// bridge_dispatch.go 는 P3 서버 측 라이브 브리지 라우팅/상관/teardown 과 아웃바운드 API 를
// 구현한다(@SPEC:SPEC-SUBFLOW-001 v1.3 그룹 RB, REQ-SUBFLOW-RB05/RB06/RB07/RB09/RB12).
//
// command(그룹 D)·query(그룹 J)·stream(그룹 J) 의 상관/팬아웃 패턴을 미러하되, 브리지는
// open~close 사이 다수 input/output 프레임이 흐르는 지속 채널이다(1:1 요청-응답이 아님 —
// bridge_protocol.go §상관).
//
// 흐름(서버 측):
//
//	OpenBridge:
//	  1. 승인+온라인 게이트(IsManaged — command 디스패치와 동일 정신, RB08 전제).
//	  2. 고유 bridge_id 부여 + pendingBridge[bridge_id] → ack 채널 등록(RB05).
//	  3. 노드로 bridge_open 전송. bridge_open_ack 를 제한 시간 내 대기(RB05/RB06).
//	  4. ack.ok=true → 활성 ServerBridge 등록(Outputs/Status 채널 활성화). ok=false/타임아웃
//	     → 오류 반환(502/타임아웃 매핑 입력).
//
//	라우팅(읽기 루프에서 호출 — 비블로킹 계약):
//	  - bridge_output(RB07) → 소유 ServerBridge 의 Outputs 채널로 비블로킹 송신(백프레셔
//	    흡수 — oldest-drop, 노드가 이미 redaction·oldest-drop 적용했으므로 매니저는 gap 허용).
//	  - bridge_status(RB09) → 소유 ServerBridge 의 Status 채널로 비블로킹 송신.
//
//	아웃바운드:
//	  - SendBridgeInput(bridge_id,port,data) → bridge_input(WRITE — RB07).
//	  - CloseBridge(bridge_id) → bridge_close(RB09) + 로컬 teardown.
//
//	teardown(RB09):
//	  - 노드 세션 드롭(teardownNodeBridges) → 그 노드의 전 브리지에 최종 offline Status
//	    방출 + 채널 close + 레지스트리 제거(누수 없음).
//	  - ServerBridge.Close → bridge_close 전송 + 로컬 teardown(멱등).
package remote

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// ErrBridgeOpenTimeout 은 제한 시간 내 bridge_open_ack 가 도착하지 않을 때 반환된다(RB05).
var ErrBridgeOpenTimeout = errors.New("remote: bridge open timed out (no ack from node)")

// ErrBridgeOpenRejected 은 노드가 브리지 개설을 거부(ack.ok=false)했을 때 반환된다(RB05).
// 메시지에 노드 측 사유(노출 범위 밖/실행 실패 등)가 포함된다.
var ErrBridgeOpenRejected = errors.New("remote: bridge open rejected by node")

// ErrBridgeClosed 는 닫힌 브리지로 입력을 보낼 때 반환된다(RB09).
var ErrBridgeClosed = errors.New("remote: bridge is closed")

// serverBridgeOutputBuffer 는 ServerBridge Outputs 채널의 매니저 측 포트별 유계 버퍼이다
// (RB07/RB10 — 서버 읽기 루프를 막지 않는다). full 시 oldest-drop 한다(노드도 oldest-drop
// 하므로 매니저는 gap 을 허용한다 — 라우팅이 읽기 루프에서 호출되므로 절대 블로킹 금지).
const serverBridgeOutputBuffer = 256

// serverBridgeStatusBuffer 는 ServerBridge Status 채널 버퍼이다(라이프사이클 신호).
const serverBridgeStatusBuffer = 16

// ServerBridge 는 서버 측 활성 라이브 브리지 핸들이다(bridge_id 1:1 — RB12). OpenBridge 가
// 반환하며, 매니저 측 어댑터(service.FlowBridge 구현)가 이를 래핑해 엔진 노드로 노출한다.
type ServerBridge struct {
	srv         *Server
	bridgeID    string
	instanceID  string
	inputPorts  []string
	outputPorts []string

	outputs chan BridgeOutputPayload
	status  chan BridgeStatusPayload

	closeOnce sync.Once
	mu        sync.Mutex
	closed    bool
}

// BridgeID 는 이 브리지의 상관 식별자를 반환한다(RB03/RB12).
func (b *ServerBridge) BridgeID() string { return b.bridgeID }

// InstanceID 는 대상 원격 노드 id 를 반환한다.
func (b *ServerBridge) InstanceID() string { return b.instanceID }

// InputPorts 는 노드가 보고한 입력 경계 포트 이름 목록이다(authoritative — RB06).
func (b *ServerBridge) InputPorts() []string { return b.inputPorts }

// OutputPorts 는 노드가 보고한 출력 경계 포트 이름 목록이다(authoritative — RB06).
func (b *ServerBridge) OutputPorts() []string { return b.outputPorts }

// Outputs 는 원격 출력 경계 메시지(bridge_output)를 수신하는 채널을 반환한다(READ — RB07).
// Close/노드 드롭 시 닫힌다.
func (b *ServerBridge) Outputs() <-chan BridgeOutputPayload { return b.outputs }

// Status 는 브리지 라이프사이클/헬스(bridge_status)를 수신하는 채널을 반환한다(RB09).
// Close/노드 드롭 시 닫힌다(드롭 시 마지막 프레임은 offline).
func (b *ServerBridge) Status() <-chan BridgeStatusPayload { return b.status }

// SendInput 은 로컬 입력 핸들 메시지를 원격 입력 경계 포트(port)로 주입한다(WRITE,
// bridge_input — RB07). 닫힌 브리지/연결 부재 시 오류를 반환한다.
func (b *ServerBridge) SendInput(port string, data json.RawMessage) error {
	b.mu.Lock()
	closed := b.closed
	b.mu.Unlock()
	if closed {
		return ErrBridgeClosed
	}
	return b.srv.sendBridgeInput(b.instanceID, b.bridgeID, port, data)
}

// Close 는 브리지를 teardown 한다(멱등 — RB09): bridge_close 전송 + 로컬 정리(채널 close +
// 레지스트리 제거). 로컬 undeploy/노드 정지 시 호출된다.
func (b *ServerBridge) Close() error {
	b.srv.closeBridge(b.bridgeID, "manager close", true)
	return nil
}

// teardown 은 로컬 측 정리만 수행한다(채널 close + closed 표시 — 멱등). bridge_close 전송
// 여부/최종 status 방출은 호출자가 결정한다.
func (b *ServerBridge) teardown(finalStatus *BridgeStatusPayload) {
	b.closeOnce.Do(func() {
		b.mu.Lock()
		b.closed = true
		b.mu.Unlock()
		if finalStatus != nil {
			// 최종 status(예: offline)를 비블로킹으로 방출한 뒤 채널을 닫는다(RB09).
			select {
			case b.status <- *finalStatus:
			default:
			}
		}
		close(b.outputs)
		close(b.status)
	})
}

// OpenBridge 는 instanceID 노드에서 remoteFlowID 플로우를 running 으로 만들고 라이브
// 브리지를 연다(RB05). inputPorts/outputPorts 는 매니저 측 flow-node 가 보유한 핸들 이름
// 힌트이며, 실제 경계 포트는 노드의 ack 가 권위(RB06)이다. 게이팅 실패/타임아웃/노드
// 거부 시 오류를 반환한다.
func (s *Server) OpenBridge(ctx context.Context, instanceID, remoteFlowID string, inputPorts, outputPorts []string) (*ServerBridge, error) {
	// 1) 승인+온라인 게이트(command 디스패치와 동일 — RB08 전제).
	if !s.IsManaged(instanceID) {
		s.logger.Warn("브리지 개설 거절 — 대상 노드 미관리",
			"instance_id", instanceID, "remote_flow_id", remoteFlowID)
		return nil, ErrNodeNotManaged
	}
	nc, ok := s.connFor(instanceID)
	if !ok {
		return nil, ErrNoConn
	}

	// 2) 고유 bridge_id + open ack 채널 등록(RB05).
	bridgeID := uuid.NewString()
	ackCh := make(chan BridgeOpenAckPayload, 1)
	s.bridgeMu.Lock()
	s.pendingBridge[bridgeID] = ackCh
	s.bridgeMu.Unlock()
	defer func() {
		s.bridgeMu.Lock()
		delete(s.pendingBridge, bridgeID)
		s.bridgeMu.Unlock()
	}()

	// 3) bridge_open 전송.
	msg, err := NewBridgeOpenMessage(BridgeOpenPayload{
		BridgeID:     bridgeID,
		InstanceID:   instanceID,
		RemoteFlowID: remoteFlowID,
	})
	if err != nil {
		return nil, fmt.Errorf("remote: encode bridge_open: %w", err)
	}
	s.logger.Info("라이브 브리지 개설 디스패치",
		"bridge_id", bridgeID, "instance_id", instanceID, "remote_flow_id", remoteFlowID)
	if werr := writeEnvelope(nc.conn, msg); werr != nil {
		return nil, fmt.Errorf("remote: send bridge_open: %w", werr)
	}

	// 4) ack 대기(타임아웃/취소 — RB05).
	timeout := s.bridgeOpenTimeout
	if timeout <= 0 {
		timeout = DefaultBridgeOpenTimeout
	}
	select {
	case ack := <-ackCh:
		if !ack.OK {
			s.logger.Warn("라이브 브리지 개설 거부(노드)",
				"bridge_id", bridgeID, "instance_id", instanceID, "error", ack.Error)
			return nil, fmt.Errorf("%w: %s", ErrBridgeOpenRejected, ack.Error)
		}
		// 노드가 보고한 경계 포트가 권위(RB06). open 요청 힌트는 무시하고 ack 값을 사용한다.
		_ = inputPorts
		_ = outputPorts
		b := &ServerBridge{
			srv:         s,
			bridgeID:    bridgeID,
			instanceID:  instanceID,
			inputPorts:  ack.InputPorts,
			outputPorts: ack.OutputPorts,
			outputs:     make(chan BridgeOutputPayload, serverBridgeOutputBuffer),
			status:      make(chan BridgeStatusPayload, serverBridgeStatusBuffer),
		}
		s.bridgeMu.Lock()
		s.bridges[bridgeID] = b
		s.bridgeMu.Unlock()
		s.logger.Info("라이브 브리지 활성",
			"bridge_id", bridgeID, "instance_id", instanceID,
			"input_ports", ack.InputPorts, "output_ports", ack.OutputPorts)
		return b, nil

	case <-time.After(timeout):
		s.logger.Warn("라이브 브리지 개설 타임아웃",
			"bridge_id", bridgeID, "instance_id", instanceID, "remote_flow_id", remoteFlowID)
		return nil, ErrBridgeOpenTimeout

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// sendBridgeInput 은 bridge_input 을 대상 노드로 전송한다(WRITE — RB07).
func (s *Server) sendBridgeInput(instanceID, bridgeID, port string, data json.RawMessage) error {
	nc, ok := s.connFor(instanceID)
	if !ok {
		return ErrNoConn
	}
	msg, err := NewBridgeInputMessage(BridgeInputPayload{
		BridgeID: bridgeID, Port: port, Data: data,
	})
	if err != nil {
		return fmt.Errorf("remote: encode bridge_input: %w", err)
	}
	return writeEnvelope(nc.conn, msg)
}

// closeBridge 는 bridge_id 브리지를 정리한다(멱등 — RB09). sendClose=true 이면 노드로
// bridge_close 를 전송한다(매니저 발신 teardown). 레지스트리에서 제거하고 채널을 닫는다.
func (s *Server) closeBridge(bridgeID, reason string, sendClose bool) {
	s.bridgeMu.Lock()
	b, ok := s.bridges[bridgeID]
	if ok {
		delete(s.bridges, bridgeID)
	}
	s.bridgeMu.Unlock()
	if !ok {
		return
	}
	if sendClose {
		if nc, connOK := s.connFor(b.instanceID); connOK {
			if msg, err := NewBridgeCloseMessage(BridgeClosePayload{BridgeID: bridgeID, Reason: reason}); err == nil {
				_ = writeEnvelope(nc.conn, msg)
			}
		}
	}
	b.teardown(nil)
	s.logger.Debug("라이브 브리지 teardown", "bridge_id", bridgeID, "reason", reason)
}

// teardownNodeBridges 는 한 노드의 모든 라이브 브리지를 teardown 한다(노드 오프라인/세션
// 종료 — RB09). 각 브리지에 최종 offline status 를 방출하고 채널을 닫는다(bridge_close
// 전송 불필요 — 연결 소멸). 누수 없음.
func (s *Server) teardownNodeBridges(instanceID string) {
	s.bridgeMu.Lock()
	var victims []*ServerBridge
	for id, b := range s.bridges {
		if b.instanceID == instanceID {
			victims = append(victims, b)
			delete(s.bridges, id)
		}
	}
	s.bridgeMu.Unlock()

	for _, b := range victims {
		b.teardown(&BridgeStatusPayload{
			BridgeID: b.bridgeID, Status: BridgeStateOffline, Error: "node offline",
		})
	}
	if len(victims) > 0 {
		s.logger.Debug("노드 브리지 전체 teardown",
			"instance_id", instanceID, "bridges", len(victims))
	}
}

// routeBridgeOpenAck 는 수신한 bridge_open_ack 를 대기 중인 OpenBridge 로 상관한다(RB05/RB06).
func (s *Server) routeBridgeOpenAck(payload []byte) {
	var ack BridgeOpenAckPayload
	if err := json.Unmarshal(payload, &ack); err != nil {
		s.logger.Debug("bridge_open_ack 디코드 실패", "error", err)
		return
	}
	if ack.BridgeID == "" {
		return
	}
	s.bridgeMu.Lock()
	ch, ok := s.pendingBridge[ack.BridgeID]
	if ok {
		delete(s.pendingBridge, ack.BridgeID)
	}
	s.bridgeMu.Unlock()
	if !ok {
		// 늦게 도착(타임아웃 후) 또는 중복 — 안전하게 폐기한다.
		s.logger.Debug("매칭되는 pending 브리지 없음 — open_ack 폐기", "bridge_id", ack.BridgeID)
		return
	}
	ch <- ack // 버퍼 1 — 블로킹되지 않는다.
}

// routeBridgeOutput 는 bridge_output 을 소유 브리지의 Outputs 채널로 라우팅한다(RB07).
// 읽기 루프에서 호출되므로 비블로킹 송신이다(full 시 oldest-drop — 매니저는 gap 허용).
func (s *Server) routeBridgeOutput(payload []byte) {
	var p BridgeOutputPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		s.logger.Debug("bridge_output 디코드 실패", "error", err)
		return
	}
	if p.BridgeID == "" {
		return
	}
	s.bridgeMu.Lock()
	b, ok := s.bridges[p.BridgeID]
	s.bridgeMu.Unlock()
	if !ok {
		return // 미지/닫힌 브리지 — 폐기.
	}
	sendBridgeFrameNonBlocking(b.outputs, p)
}

// routeBridgeStatus 는 bridge_status 를 소유 브리지의 Status 채널로 라우팅한다(RB09).
// 비블로킹 송신(full 시 oldest-drop — 읽기 루프 비블로킹).
func (s *Server) routeBridgeStatus(payload []byte) {
	var p BridgeStatusPayload
	if err := json.Unmarshal(payload, &p); err != nil {
		s.logger.Debug("bridge_status 디코드 실패", "error", err)
		return
	}
	if p.BridgeID == "" {
		return
	}
	s.bridgeMu.Lock()
	b, ok := s.bridges[p.BridgeID]
	s.bridgeMu.Unlock()
	if !ok {
		return
	}
	sendBridgeFrameNonBlocking(b.status, p)
}

// sendBridgeFrameNonBlocking 은 유계 채널로 비블로킹 송신한다(백프레셔 — RB07/RB10). full
// 이면 가장 오래된 프레임을 drop 하고 최신값을 넣는다. 읽기 루프가 절대 블로킹되지 않는다.
func sendBridgeFrameNonBlocking[T any](ch chan T, v T) {
	select {
	case ch <- v:
		return
	default:
	}
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- v:
	default:
	}
}
