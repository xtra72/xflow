// client_bridge.go 는 P2(노드 측 라이브 브리지)의 bridge_open/bridge_input/bridge_close
// 처리를 구현한다(@SPEC:SPEC-SUBFLOW-001 그룹 RB, plan §2-bis P2, REQ-SUBFLOW-RB05/RB06/
// RB07/RB08/RB09/RB10/RB11).
//
// 처리 경로(handleCommand/handleSubscribe 와 대칭, READ+WRITE 양방향):
//
//	bridge_open:
//	  1. 게이팅(RB08/RB11): 미승인(토큰 미보유) 노드 거부 + 참조 플로우가 노출 범위
//	     (Exposure.Flows) 안에 있어야 함. 위반 시 bridge_open_ack{ok:false} + 감사.
//	  2. 자동배포(OQ-RB1/RB5): BridgeRunner.OpenBridge 로 참조 플로우를 running 으로
//	     만들고(재사용 가능) 입출력 경계 포트를 tap 한다. 소유/정지 판단은 runner 가 소유.
//	  3. 경계 포트 보고(RB06): handle 의 InputPorts/OutputPorts 를 bridge_open_ack 로 반환.
//	  4. 출력 펌프 시작: onOutput → 포트별 bounded buffer(+oldest-drop/coalesce, RB10) →
//	     별도 펌프 고루틴이 redaction(§5.14) 후 bridge_output 전송. 느린 WS write 가
//	     엔진을 막지 않는다.
//
//	bridge_input(WRITE — RB07/RB11): 열린 브리지(bridge_id)를 찾아 handle.Inject 로 입력
//	  경계 포트에 주입한다. 쓰기 authz 는 open 시점에 게이팅되었으므로 per-message 는 열린
//	  브리지 범위 내에서 수행되며, 입력 주입을 감사한다(RB11). 미지 bridge_id 는 무시한다.
//
//	bridge_close / 세션 종료(RB09): 펌프 종료 + handle.Close(자동 시작분 정지 위임) +
//	  레지스트리 제거. 누수 없음(세션 wg 추적 + bounded 펌프). flow-node 별 독립 bridge_id
//	  (RB12)로 다중 브리지가 독립 teardown 된다.
//
// 적용은 별도 고루틴(handleServerMessage)에서 호출되어 읽기 루프를 막지 않으며, 신뢰
// 경계 panic 복구 가드로 데몬을 보호한다(시크릿 비노출 — REQ-SUBFLOW-RB06).
package remote

import (
	"context"
	"encoding/json"
	"runtime/debug"
	"strings"
	"sync"
)

// bridgeOutputBufferSize 는 브리지 출력 포트별 bounded buffer 의 용량이다(RB10). full
// 시 oldest-drop + 펌프 전송 직전 coalesce(drain-to-latest)로 느린 WS 소비자에서
// 메모리 폭증을 방지한다(client_stream 의 drainLatest 패턴과 동일 정신).
const bridgeOutputBufferSize = 256

// bridgeOutputFrame 는 펌프로 흐르는 단일 출력 경계 메시지(포트 + 페이로드)이다.
type bridgeOutputFrame struct {
	port string
	data json.RawMessage
}

// bridgeSession 은 단일 활성 라이브 브리지의 client 측 핸들이다(P2). 출력 펌프 고루틴과
// runner handle 을 보유하며, teardown 시 펌프를 종료하고 handle.Close()로 참조 플로우
// 정지(자동 시작분)를 위임한다.
type bridgeSession struct {
	client   *Client // 소유 client(레지스트리 자기 제거용)
	bridgeID string
	flowID   string
	handle   BridgeHandle

	// out 은 출력 경계 메시지를 펌프로 흘리는 bounded 채널이다(RB10). onOutput 콜백이
	// non-blocking 으로 enqueue(full 시 oldest-drop)하고, 펌프가 drain→WS 한다.
	out    chan bridgeOutputFrame
	cancel context.CancelFunc

	closeOnce sync.Once
}

// handleBridgeOpen 은 bridge_open 을 처리한다: 게이팅 → 자동배포 → 경계 tap → ack
// → 출력 펌프 시작(REQ-SUBFLOW-RB05/RB06/RB08).
func (c *Client) handleBridgeOpen(ctx context.Context, conn Conn, wg *sync.WaitGroup, payload []byte) {
	// 신뢰 경계 panic 복구 가드: runner/디코드/redaction panic 이 데몬을 죽이지 않도록
	// 복구하고 bridge_open_ack{ok:false}로 변환한다. 시크릿 비노출(REQ-SUBFLOW-RB06).
	bridgeID := ""
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("브리지 개설 중 panic 복구",
				"bridge_id", bridgeID, "panic", r, "stack", string(debug.Stack()))
			c.sendBridgeOpenAck(conn, BridgeOpenAckPayload{
				BridgeID: bridgeID, OK: false,
				Error: "internal error while opening bridge",
			})
		}
	}()

	var op BridgeOpenPayload
	if err := json.Unmarshal(payload, &op); err != nil {
		c.logger.Debug("bridge_open 디코드 실패", "error", err)
		return
	}
	if op.BridgeID == "" {
		c.logger.Warn("bridge_open 에 bridge_id 누락 — 무시")
		return
	}
	bridgeID = op.BridgeID

	// 1) 승인 게이팅(RB08/RB11). 미승인 노드는 거부(handleCommand/handleQuery 와 동일 정신).
	if !c.hasToken() {
		c.logger.Warn("미승인 노드 — 브리지 개설 거부",
			"bridge_id", op.BridgeID, "remote_flow_id", op.RemoteFlowID)
		c.recordBridgeAudit(ctx, "open_rejected", op.BridgeID, op.RemoteFlowID, "node not approved")
		c.sendBridgeOpenAck(conn, BridgeOpenAckPayload{
			BridgeID: op.BridgeID, OK: false, Error: "node not approved",
		})
		return
	}

	// 2) 노출 범위 게이팅(RB08/RB11 — 참조 플로우가 노출 범위 안이어야 함). 범위 밖은 거부.
	if !c.flowInExposureScope(op.RemoteFlowID) {
		c.logger.Warn("노출 범위 밖 참조 플로우 — 브리지 개설 거부",
			"bridge_id", op.BridgeID, "remote_flow_id", op.RemoteFlowID)
		c.recordBridgeAudit(ctx, "open_rejected", op.BridgeID, op.RemoteFlowID, "flow not in exposure scope")
		c.sendBridgeOpenAck(conn, BridgeOpenAckPayload{
			BridgeID: op.BridgeID, OK: false, Error: "flow not in exposure scope",
		})
		return
	}

	// 3) runner 미구성이면 거부(미구성 노드 보호).
	if c.cfg.BridgeRunner == nil {
		c.logger.Warn("bridge runner 미구성 — 브리지 개설 거부", "bridge_id", op.BridgeID)
		c.recordBridgeAudit(ctx, "open_rejected", op.BridgeID, op.RemoteFlowID, "bridge runner not configured")
		c.sendBridgeOpenAck(conn, BridgeOpenAckPayload{
			BridgeID: op.BridgeID, OK: false, Error: ErrBridgeRunnerUnavailable.Error(),
		})
		return
	}

	// 4) 브리지 세션 + 출력 펌프 준비. 펌프 ctx 는 세션 ctx 의 자식이므로 세션 종료 시
	//    자동 취소된다(teardown — RB09).
	pumpCtx, cancel := context.WithCancel(ctx)
	br := &bridgeSession{
		client:   c,
		bridgeID: op.BridgeID,
		flowID:   op.RemoteFlowID,
		out:      make(chan bridgeOutputFrame, bridgeOutputBufferSize),
		cancel:   cancel,
	}

	// onOutput: 엔진 tap 에서 인라인 호출되므로 non-blocking enqueue(+oldest-drop)만 한다
	//           (RB10 — 느린 WS write 가 엔진을 막지 않음).
	onOutput := func(port string, data json.RawMessage) {
		br.enqueueOutput(port, data)
	}

	// 5) 자동배포(OQ-RB1/RB5): 참조 플로우를 running 으로 만들고 경계 포트 tap.
	handle, err := c.cfg.BridgeRunner.OpenBridge(pumpCtx, op.RemoteFlowID, onOutput)
	if err != nil {
		cancel()
		c.logger.Warn("브리지 참조 플로우 실행 실패",
			"bridge_id", op.BridgeID, "remote_flow_id", op.RemoteFlowID, "error", err)
		c.recordBridgeAudit(ctx, "open_rejected", op.BridgeID, op.RemoteFlowID, "runner error")
		c.sendBridgeOpenAck(conn, BridgeOpenAckPayload{
			BridgeID: op.BridgeID, OK: false, Error: err.Error(),
		})
		return
	}
	br.handle = handle

	// 6) 레지스트리 등록(bridge_input/bridge_close/세션 종료 추적). 세션이 이미 끝났으면
	//    (bridges nil) 즉시 teardown 하고 종료한다(레이스 안전).
	c.mu.Lock()
	if c.bridges == nil {
		c.mu.Unlock()
		cancel()
		_ = handle.Close(context.Background())
		return
	}
	// 동일 bridge_id 재개설 방어: 기존 브리지가 있으면 등록 후 락 밖에서 teardown 한다
	// (teardown→removeBridgeIf 가 c.mu 를 재획득하므로 락 보유 중 호출 금지). want-instance
	// 가드(removeBridgeIf)로 교체된 신규 br 은 보존된다.
	prev := c.bridges[op.BridgeID]
	c.bridges[op.BridgeID] = br
	c.mu.Unlock()
	if prev != nil {
		prev.teardown(ctx, "replaced")
	}

	// 7) 출력 펌프 고루틴(세션 wg 추적 — 연결 종료/취소 시 정리, 누수 없음 — RB09).
	wg.Add(1)
	go func() {
		defer wg.Done()
		c.runBridgeOutputPump(pumpCtx, conn, br)
	}()

	// 8) ack: 노드가 권위인 실제 경계 포트 보고(RB06).
	c.recordBridgeAudit(ctx, "open", op.BridgeID, op.RemoteFlowID, "")
	c.sendBridgeOpenAck(conn, BridgeOpenAckPayload{
		BridgeID:    op.BridgeID,
		OK:          true,
		InputPorts:  handle.InputPorts(),
		OutputPorts: handle.OutputPorts(),
	})
}

// enqueueOutput 은 출력 경계 메시지를 펌프 채널에 non-blocking 으로 넣는다(RB10).
// full 이면 가장 오래된 프레임을 drop 하고 새 프레임을 넣는다(oldest-drop — 엔진 비블로킹
// 보장). FIFO 는 보존되며 펌프가 전송 직전 추가 coalesce 한다.
func (br *bridgeSession) enqueueOutput(port string, data json.RawMessage) {
	frame := bridgeOutputFrame{port: port, data: data}
	select {
	case br.out <- frame:
		return
	default:
	}
	// full → 가장 오래된 프레임 drop 후 재시도(non-blocking — 엔진 스레드 비블로킹).
	select {
	case <-br.out:
	default:
	}
	select {
	case br.out <- frame:
	default:
		// 극단적 경합에서도 절대 블로킹하지 않는다(프레임 1건 추가 drop 허용).
	}
}

// runBridgeOutputPump 은 출력 경계 프레임을 redaction 후 bridge_output 로 전송한다
// (REQ-SUBFLOW-RB07). 펌프 종료 시(ctx 취소/채널 close/전송 실패) 레지스트리에서 브리지를
// teardown 한다(handle.Close 위임 — 자동 시작분 정지).
func (c *Client) runBridgeOutputPump(ctx context.Context, conn Conn, br *bridgeSession) {
	// teardown 가드: 펌프 종료 경로 어디서든 브리지를 정리한다(RB09).
	defer br.teardown(context.Background(), "pump exit")
	// panic 복구: redaction/직렬화 panic 이 데몬을 죽이지 않도록 한다(시크릿 비노출 — RB06).
	defer func() {
		if r := recover(); r != nil {
			c.logger.Error("브리지 출력 펌프 panic 복구",
				"bridge_id", br.bridgeID, "panic", r, "stack", string(debug.Stack()))
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case frame := <-br.out:
			if err := c.sendBridgeOutput(conn, br.bridgeID, frame.port, frame.data); err != nil {
				// 연결 종료 등 전송 실패 → 펌프 종료(teardown).
				c.logger.Debug("bridge_output 전송 실패 — 펌프 종료",
					"bridge_id", br.bridgeID, "error", err)
				return
			}
		}
	}
}

// handleBridgeInput 은 bridge_input 을 처리하여 입력 경계 포트로 메시지를 주입한다
// (WRITE — REQ-SUBFLOW-RB07/RB11). 미지 bridge_id 는 무시한다.
func (c *Client) handleBridgeInput(ctx context.Context, payload []byte) {
	var in BridgeInputPayload
	if err := json.Unmarshal(payload, &in); err != nil {
		c.logger.Debug("bridge_input 디코드 실패", "error", err)
		return
	}
	if in.BridgeID == "" {
		return
	}

	c.mu.Lock()
	br, ok := c.bridges[in.BridgeID]
	c.mu.Unlock()
	if !ok {
		// 미지/이미 닫힌 브리지로의 입력 — 무시(stale 주입 방지).
		c.logger.Debug("미지 bridge_id 입력 — 무시", "bridge_id", in.BridgeID)
		return
	}

	// 쓰기 authz 는 open 시점에 게이팅됨(RB11). per-message 는 열린 브리지 범위 내. 입력
	// 주입 감사(RB11 — who/when/node/flow + 포트, 페이로드 값 제외 — RB06).
	c.recordBridgeAudit(ctx, "input", in.BridgeID, br.flowID, "port="+in.Port)

	// 주입(Data 는 시크릿 가능성으로 로깅 제외 — RB06).
	if err := br.handle.Inject(ctx, in.Port, in.Data); err != nil {
		c.logger.Warn("브리지 입력 주입 실패",
			"bridge_id", in.BridgeID, "port", in.Port, "error", err)
		// 주입 실패는 상태 신호로 보고할 수 있으나(선택), 브리지를 닫지는 않는다.
		c.sendBridgeStatus(c.currentConn(), in.BridgeID, BridgeStateError, "input inject failed")
	}
}

// handleBridgeClose 는 bridge_close 를 처리하여 대상 브리지를 teardown 한다(REQ-SUBFLOW-RB09).
func (c *Client) handleBridgeClose(payload []byte) {
	var cl BridgeClosePayload
	if err := json.Unmarshal(payload, &cl); err != nil {
		c.logger.Debug("bridge_close 디코드 실패", "error", err)
		return
	}
	if cl.BridgeID == "" {
		return
	}
	c.mu.Lock()
	br, ok := c.bridges[cl.BridgeID]
	c.mu.Unlock()
	if !ok {
		return
	}
	reason := cl.Reason
	if reason == "" {
		reason = "manager close"
	}
	c.recordBridgeAudit(context.Background(), "close", cl.BridgeID, br.flowID, reason)
	br.teardown(context.Background(), reason)
}

// teardown 은 브리지를 정리한다(멱등 — RB09): 펌프 종료(cancel) → handle.Close(자동
// 시작분 정지 위임) → 레지스트리 제거 + 감사. 펌프 종료 경로와 close/세션 종료 경로 모두
// 안전하게 한 번만 정리한다.
func (br *bridgeSession) teardown(ctx context.Context, reason string) {
	br.closeOnce.Do(func() {
		if br.cancel != nil {
			br.cancel()
		}
		if br.handle != nil {
			_ = br.handle.Close(ctx)
		}
		// 레지스트리에서 자기 자신을 제거한다(이 bridgeID 가 여전히 이 인스턴스일 때만 —
		// 재개설 교체 경합 방어). client 가 nil(테스트 경로 일부)이면 건너뛴다.
		if br.client != nil {
			br.client.removeBridgeIf(br.bridgeID, br)
		}
	})
	_ = reason
}

// removeBridgeIf 는 레지스트리에서 bridgeID 항목이 정확히 want 인스턴스일 때만 제거한다
// (재개설 교체 경합 방어 — 다른 인스턴스가 이미 등록되어 있으면 건드리지 않음). 세션이
// 이미 끝났으면(bridges nil) no-op.
func (c *Client) removeBridgeIf(bridgeID string, want *bridgeSession) {
	c.mu.Lock()
	if c.bridges != nil {
		if cur, ok := c.bridges[bridgeID]; ok && cur == want {
			delete(c.bridges, bridgeID)
		}
	}
	c.mu.Unlock()
}

// flowInExposureScope 는 참조 플로우가 노드의 노출 범위(Exposure.Flows) 안인지 판정한다
// (RB08/RB11). applyExposure 와 동일 의미: "all" → 허용, ""/"none" → 거부, 그 외 →
// 쉼표 구분 목록에 flowID 가 포함되어야 허용. 노출 범위 평가는 노드 권위이다(A04/E07).
func (c *Client) flowInExposureScope(flowID string) bool {
	policy := c.currentExposure().Flows
	switch strings.TrimSpace(strings.ToLower(policy)) {
	case ExposeAll:
		return true
	case "", ExposeNone:
		return false
	default:
		allow := parseExposureList(policy)
		_, ok := allow[flowID]
		return ok
	}
}

// recordBridgeAudit 는 브리지 라이프사이클/입력 이벤트를 감사한다(RB06/RB11). 시크릿
// 페이로드 값은 절대 기록하지 않는다(detail 은 포트/사유 등 비시크릿 메타). 싱크가
// 구성되지 않았으면 구조화 로그로만 기록한다.
func (c *Client) recordBridgeAudit(ctx context.Context, event, bridgeID, flowID, detail string) {
	if c.cfg.BridgeAudit != nil {
		c.cfg.BridgeAudit.RecordBridge(ctx, event, bridgeID, flowID, detail)
	}
	c.logger.Info("브리지 감사",
		"event", event, "bridge_id", bridgeID, "flow_id", flowID, "detail", detail,
		"instance_id", c.cfg.InstanceID)
}

// currentConn 은 현재 세션 연결을 반환한다(상태 신호 전송용). 없으면 nil.
func (c *Client) currentConn() Conn {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.conn
}

// sendBridgeOpenAck 는 bridge_open_ack 를 연결로 전송한다(REQ-SUBFLOW-RB05/RB06).
func (c *Client) sendBridgeOpenAck(conn Conn, ack BridgeOpenAckPayload) {
	msg, err := NewBridgeOpenAckMessage(ack)
	if err != nil {
		c.logger.Error("bridge_open_ack 인코딩 실패", "error", err)
		return
	}
	if err := writeEnvelope(conn, msg); err != nil {
		c.logger.Debug("bridge_open_ack 전송 실패", "bridge_id", ack.BridgeID, "error", err)
	}
}

// sendBridgeOutput 는 출력 경계 메시지를 redaction 후 bridge_output 로 전송한다
// (REQ-SUBFLOW-RB07 — 노드 측 redaction §5.14).
func (c *Client) sendBridgeOutput(conn Conn, bridgeID, port string, data json.RawMessage) error {
	msg, err := NewBridgeOutputMessage(BridgeOutputPayload{
		BridgeID: bridgeID,
		Port:     port,
		Data:     c.redact(data),
	})
	if err != nil {
		return err
	}
	return writeEnvelope(conn, msg)
}

// sendBridgeStatus 는 bridge_status 를 연결로 전송한다(REQ-SUBFLOW-RB09). conn 이 nil
// 이거나 전송 실패 시 로깅만 한다.
func (c *Client) sendBridgeStatus(conn Conn, bridgeID, status, errMsg string) {
	if conn == nil {
		return
	}
	msg, err := NewBridgeStatusMessage(BridgeStatusPayload{
		BridgeID: bridgeID, Status: status, Error: errMsg,
	})
	if err != nil {
		c.logger.Error("bridge_status 인코딩 실패", "error", err)
		return
	}
	if werr := writeEnvelope(conn, msg); werr != nil {
		c.logger.Debug("bridge_status 전송 실패", "bridge_id", bridgeID, "error", werr)
	}
}
