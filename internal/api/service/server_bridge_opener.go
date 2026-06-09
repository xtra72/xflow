// server_bridge_opener.go 는 service.FlowBridgeOpener/FlowBridge 를 remote.Server 의 라이브
// 브리지 API(internal/remote/bridge_dispatch.go) 위에 구현한다(@SPEC:SPEC-SUBFLOW-001 v1.3
// 그룹 RB, P3, REQ-SUBFLOW-RB05/RB06/RB07/RB09).
//
// 위치/사이클: service 는 remote 를 import 한다(bridge_runner.go 와 동일 단방향). 본 어댑터는
// remote.Server(serverBridgeAPI)/remote.ServerBridge(serverBridgeHandle) 위에서 동작하며,
// cmd/xflowd(server 모드)가 엔진 remote-bridge 노드에 주입한다(import-cycle 회피 — service
// 패키지가 양쪽을 알고 와이어링).
//
// 매핑:
//   - OpenBridge → server.OpenBridge(...) → ServerBridge 핸들을 FlowBridge 로 래핑.
//   - FlowBridge.SendInput → ServerBridge.SendInput(bridge_input — WRITE).
//   - FlowBridge.Outputs → ServerBridge.Outputs 를 매니저 측 포트별 유계 버퍼로 펌프(아래)
//     하여 service 측 BridgeOutput 으로 변환한다(서버 읽기 루프를 막지 않음 — RB07/RB10).
//   - FlowBridge.Status → ServerBridge.Status 를 BridgeStatus 로 변환 펌프.
//   - FlowBridge.Close → ServerBridge.Close(bridge_close + teardown).
//
// 매니저 측 버퍼링(RB07/RB10): ServerBridge 의 Outputs/Status 채널은 이미 서버 측에서
// oldest-drop 유계 버퍼이지만, 어댑터는 한 단계 더 유계 버퍼 + 펌프 고루틴을 둬서 엔진
// 노드의 소비 속도와 서버 라우팅을 디커플링한다. full 시 oldest-drop(노드도 oldest-drop
// 하므로 매니저는 gap 을 허용 — 라이브 브리지의 상태 동기화 의미와 일관).
package service

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/xtra/xflow/internal/remote"
)

// managerBridgeOutputBuffer 는 어댑터 측 Outputs/Status 유계 버퍼 크기이다(RB10).
const managerBridgeOutputBuffer = 256

// serverBridgeHandle 은 remote.ServerBridge 의 표면을 추상화한다(테스트 더블 주입용).
// *remote.ServerBridge 가 이를 만족한다.
type serverBridgeHandle interface {
	BridgeID() string
	InputPorts() []string
	OutputPorts() []string
	SendInput(port string, data json.RawMessage) error
	Outputs() <-chan remote.BridgeOutputPayload
	Status() <-chan remote.BridgeStatusPayload
	Close() error
}

// serverBridgeAPI 는 remote.Server 의 OpenBridge 표면을 추상화한다(테스트 더블 주입용).
// *remote.Server 가 이를 만족한다(serverBridgeAdapter 경유 — 반환 타입 일치).
type serverBridgeAPI interface {
	OpenBridge(ctx context.Context, instanceID, remoteFlowID string, inputPorts, outputPorts []string) (serverBridgeHandle, error)
}

// RemoteServerBridgeAPI 는 *remote.Server 를 serverBridgeAPI 로 어댑트한다(반환 타입을
// 인터페이스로 승격). cmd/xflowd 가 NewServerBridgeOpener 주입 시 사용한다.
type RemoteServerBridgeAPI struct {
	Server *remote.Server
}

// OpenBridge 는 remote.Server.OpenBridge 를 호출하고 *remote.ServerBridge 를 인터페이스로
// 반환한다(nil 핸들 안전 변환).
func (a RemoteServerBridgeAPI) OpenBridge(ctx context.Context, instanceID, remoteFlowID string, inputPorts, outputPorts []string) (serverBridgeHandle, error) {
	b, err := a.Server.OpenBridge(ctx, instanceID, remoteFlowID, inputPorts, outputPorts)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// serverBridgeOpener 는 serverBridgeAPI 위의 FlowBridgeOpener 구현이다.
type serverBridgeOpener struct {
	api    serverBridgeAPI
	logger *slog.Logger
}

// newServerBridgeOpener 는 어댑터를 생성한다(내부/테스트 — 인터페이스 주입).
func newServerBridgeOpener(api serverBridgeAPI, logger *slog.Logger) *serverBridgeOpener {
	if logger == nil {
		logger = slog.Default()
	}
	return &serverBridgeOpener{api: api, logger: logger}
}

// NewServerBridgeOpener 는 *remote.Server 위의 FlowBridgeOpener 를 생성한다(cmd/xflowd 와이어링).
func NewServerBridgeOpener(srv *remote.Server, logger *slog.Logger) FlowBridgeOpener {
	return newServerBridgeOpener(RemoteServerBridgeAPI{Server: srv}, logger)
}

// OpenBridge 는 server.OpenBridge 를 호출하고 FlowBridge 핸들을 반환한다(RB05/RB06).
func (o *serverBridgeOpener) OpenBridge(ctx context.Context, instanceID, remoteFlowID string, inputPorts, outputPorts []string) (FlowBridge, error) {
	h, err := o.api.OpenBridge(ctx, instanceID, remoteFlowID, inputPorts, outputPorts)
	if err != nil {
		return nil, err
	}
	fb := &serverFlowBridge{
		handle:  h,
		outputs: make(chan BridgeOutput, managerBridgeOutputBuffer),
		status:  make(chan BridgeStatus, managerBridgeOutputBuffer),
		done:    make(chan struct{}),
		logger:  o.logger,
	}
	fb.wg.Add(2)
	go fb.pumpOutputs(h.Outputs())
	go fb.pumpStatus(h.Status())
	return fb, nil
}

// serverFlowBridge 는 serverBridgeHandle 을 service.FlowBridge 로 래핑한다.
type serverFlowBridge struct {
	handle  serverBridgeHandle
	outputs chan BridgeOutput
	status  chan BridgeStatus
	done    chan struct{}
	wg      sync.WaitGroup
	logger  *slog.Logger

	closeOnce sync.Once
}

// SendInput 은 입력을 하부 브리지로 전달한다(WRITE — RB07).
func (b *serverFlowBridge) SendInput(port string, data json.RawMessage) error {
	return b.handle.SendInput(port, data)
}

// Outputs 는 변환된 출력 채널을 반환한다(READ — RB07). Close/하부 채널 close 시 닫힌다.
func (b *serverFlowBridge) Outputs() <-chan BridgeOutput { return b.outputs }

// Status 는 변환된 상태 채널을 반환한다(RB09). Close/하부 채널 close 시 닫힌다.
func (b *serverFlowBridge) Status() <-chan BridgeStatus { return b.status }

// Close 는 하부 브리지를 닫고 펌프를 종료한다(멱등 — RB09).
func (b *serverFlowBridge) Close() error {
	b.closeOnce.Do(func() {
		_ = b.handle.Close() // 하부 채널 close → 펌프 고루틴 종료.
		b.wg.Wait()          // 펌프 종료 대기(채널 close 순서 보장).
		close(b.outputs)
		close(b.status)
		close(b.done)
	})
	return nil
}

// pumpOutputs 는 하부 bridge_output 을 매니저 측 유계 버퍼로 비블로킹 펌프한다(RB07/RB10).
func (b *serverFlowBridge) pumpOutputs(src <-chan remote.BridgeOutputPayload) {
	defer b.wg.Done()
	for p := range src {
		sendManagerBridgeFrame(b.outputs, BridgeOutput{Port: p.Port, Data: p.Data})
	}
}

// pumpStatus 는 하부 bridge_status 를 매니저 측 유계 버퍼로 비블로킹 펌프한다(RB09).
func (b *serverFlowBridge) pumpStatus(src <-chan remote.BridgeStatusPayload) {
	defer b.wg.Done()
	for p := range src {
		sendManagerBridgeFrame(b.status, BridgeStatus{State: p.Status, Detail: p.Error})
	}
}

// sendManagerBridgeFrame 은 유계 채널로 비블로킹 송신한다(full 시 oldest-drop — gap 허용).
func sendManagerBridgeFrame[T any](ch chan T, v T) {
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

// 컴파일 타임 계약.
var (
	_ FlowBridgeOpener   = (*serverBridgeOpener)(nil)
	_ FlowBridge         = (*serverFlowBridge)(nil)
	_ serverBridgeAPI    = RemoteServerBridgeAPI{}
	_ serverBridgeHandle = (*remote.ServerBridge)(nil)
)
