// local_bridge_opener.go 는 로컬 `shared` 모드 flow-node 를 위한 in-process FlowBridgeOpener
// 구현이다(@SPEC:SPEC-SUBFLOW-002 그룹 SH/L, REQ-SUBFLOW2-SH02/SH04/L01~L05).
//
// 원격 opener(server_bridge_opener.go)와 동일한 FlowBridgeOpener/FlowBridge 계약을 구현하되,
// 전송(transport)만 in-process 로 바꾼다(plan §1.1 — 원격 브리지 인프라의 in-process 일반화):
//   - 원격: OpenBridge → remote.Server.OpenBridge → WS 세션 위 bridge_input/bridge_output.
//   - 로컬: OpenBridge → 엔진 실행 상태 확인 + sharedBoundaryTable 의 컨트롤러에 in-process attach.
//
// managerBridgeController(self-healing supervisor)는 opener 인터페이스에만 의존하므로 코드
// 변경 없이 재사용된다(opener 만 교체). 따라서 "오프라인 시작 → 도착 시 자동 open → 드롭 시
// 백오프 재open" 단일 경로가 로컬 shared 의 생명주기(L02~L05)에 그대로 적용된다.
//
// 자동 시작 없음(L01): OpenBridge 는 참조 플로우를 시작하지 않는다. 참조 플로우가 running 이
// 아니거나 공유 경계 컨트롤러가 없으면 transient 오류를 반환하여, 컨트롤러가 오프라인 대기 후
// 자가치유하도록 한다(L02/L03). 참조 카운팅 없음(L06): Close 는 subscriber 만 제거하며 참조
// 플로우 인스턴스에는 영향을 주지 않는다(L07 — 부모 teardown 격리).
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/flow"
)

// localBridgeInstanceID 는 로컬 in-process 브리지의 instance_id 자리(원격과 달리 노드가 없으므로
// 빈 의미). FlowBridgeOpener.OpenBridge 의 instanceID 인자는 로컬에서 무시되고 remoteFlowID 가
// 실제 참조 flow_id 로 쓰인다(rewireRemoteBridges 일반화가 로컬은 instanceID="" 로 호출).

// engineFlowStatusProvider 는 로컬 opener 가 참조 플로우의 실행 상태를 조회하는 데 필요한
// 최소 표면이다(*engine.Engine 가 만족). 테스트 더블 주입을 위해 인터페이스로 둔다.
type engineFlowStatusProvider interface {
	GetFlowStatus(flowID string) (engine.FlowStatus, error)
}

// sharedBoundaryLookup 은 로컬 opener 가 flow_id 로 공유 경계 컨트롤러를 조회하는 표면이다
// (globalSharedBoundaryTable 이 만족 — 테스트 주입용).
type sharedBoundaryLookup interface {
	lookup(flowID string) (*sharedBoundaryController, bool)
}

// localBridgeOpener 는 in-process FlowBridgeOpener 구현이다.
type localBridgeOpener struct {
	status engineFlowStatusProvider
	table  sharedBoundaryLookup
	logger *slog.Logger
}

// newLocalBridgeOpener 는 opener 를 생성한다(내부/테스트 — 인터페이스 주입).
func newLocalBridgeOpener(status engineFlowStatusProvider, table sharedBoundaryLookup, logger *slog.Logger) *localBridgeOpener {
	if logger == nil {
		logger = slog.Default()
	}
	return &localBridgeOpener{status: status, table: table, logger: logger}
}

// NewLocalBridgeOpener 는 *engine.Engine 위의 로컬 in-process FlowBridgeOpener 를 생성한다
// (cmd/xflowd 와이어링). 공유 경계 컨트롤러는 process-global globalSharedBoundaryTable 에서 조회한다.
func NewLocalBridgeOpener(eng *engine.Engine, logger *slog.Logger) FlowBridgeOpener {
	return newLocalBridgeOpener(eng, globalSharedBoundaryTable, logger)
}

// ErrSharedBridgeOffline 은 참조 플로우가 미실행이거나 공유 경계 컨트롤러가 아직 없을 때
// 반환되는 transient 오류이다. managerBridgeController 가 이를 받아 오프라인 대기 후 백오프
// 재open 한다(L02/L03 — deploy 를 실패시키지 않음).
var ErrSharedBridgeOffline = fmt.Errorf("shared bridge offline: referenced flow not running")

// OpenBridge 는 remoteFlowID(=로컬 참조 flow_id) 의 실행 인스턴스에 in-process 로 attach 한다
// (SH02). 참조 플로우가 running 이 아니거나 공유 경계 컨트롤러가 없으면 ErrSharedBridgeOffline
// (transient)을 반환한다(L02). instanceID 인자는 로컬에서 무시된다(원격과의 계약 직교성).
func (o *localBridgeOpener) OpenBridge(_ context.Context, _ string, remoteFlowID string, inputPorts, outputPorts []string) (FlowBridge, error) {
	flowID := remoteFlowID
	if flowID == "" {
		return nil, fmt.Errorf("local bridge: 빈 flow_id")
	}

	// L01 — 자동 시작 없음: 참조 플로우가 이미 running 이어야 한다.
	st, err := o.status.GetFlowStatus(flowID)
	if err != nil || (st.State != flow.FlowRunning && st.State != flow.FlowPaused) {
		return nil, ErrSharedBridgeOffline // transient — 오프라인 대기 후 자가치유.
	}

	// 공유 경계 컨트롤러 조회(참조 플로우 배포 시 등록됨). 없거나 닫혔으면 오프라인.
	ctrl, ok := o.table.lookup(flowID)
	if !ok || ctrl == nil || ctrl.isClosed() {
		return nil, ErrSharedBridgeOffline
	}

	fb := &localFlowBridge{
		ctrl:    ctrl,
		outputs: make(chan BridgeOutput, managerBridgeOutputBuffer),
		status:  make(chan BridgeStatus, managerBridgeOutputBuffer),
		logger:  o.logger,
	}

	// 출력 subscriber 등록(fan-out 대상 추가 — MR03). onClose 는 참조 플로우 정지 시 컨트롤러
	// shutdown 이 호출하여 이 브리지의 채널을 닫고 매니저 self-heal 을 트리거한다(L04/L05).
	// 컨트롤러가 막 닫혔으면 오프라인.
	sub := ctrl.addSubscriber(
		func(port string, data json.RawMessage) { fb.deliverOutput(port, data) },
		func() { fb.closeChannels() },
	)
	if sub == nil {
		return nil, ErrSharedBridgeOffline
	}
	fb.sub = sub

	// 연결 상태 1회 통지(running). 이후 상태 전이(정지)는 채널 close 로 신호한다.
	sendManagerBridgeFrame(fb.status, BridgeStatus{State: "running"})

	o.logger.Debug("로컬 in-process 브리지 연결", "flow_id", flowID,
		"input_ports", inputPorts, "output_ports", outputPorts)
	return fb, nil
}

// localFlowBridge 는 공유 경계 컨트롤러 위의 in-process FlowBridge 핸들이다.
type localFlowBridge struct {
	ctrl    *sharedBoundaryController
	sub     *sharedBoundarySub
	outputs chan BridgeOutput
	status  chan BridgeStatus
	logger  *slog.Logger

	// closeMu 는 채널 close 와 deliverOutput 송신의 경합을 차단한다. RLock 으로 송신하고
	// Lock 으로 닫으므로, "닫힌 채널로 송신"(panic)이 발생하지 않는다(race-free).
	closeMu   sync.RWMutex
	closed    bool
	closeOnce sync.Once
}

// SendInput 은 입력 메시지를 참조 플로우의 입력 경계 포트로 주입한다(WRITE — fan-in, MR02).
func (b *localFlowBridge) SendInput(port string, data json.RawMessage) error {
	// inject 의 ctx 는 비블로킹 경로(유계 버퍼 oldest-drop)이므로 Background 로 충분하다.
	return b.ctrl.inject(context.Background(), port, data)
}

// Outputs 는 출력 경계 메시지 채널을 반환한다(READ — RB07 미러). Close/참조 플로우 정지 시 닫힌다.
func (b *localFlowBridge) Outputs() <-chan BridgeOutput { return b.outputs }

// Status 는 상태 채널을 반환한다. Close/참조 플로우 정지 시 닫힌다.
func (b *localFlowBridge) Status() <-chan BridgeStatus { return b.status }

// Close 는 브리지를 teardown 한다(멱등). subscriber 를 제거하고 채널을 닫는다 — 참조 플로우
// 인스턴스에는 영향을 주지 않는다(L06/L07 — 참조 카운팅·자동 정지 없음). 부모 undeploy 시
// fwd 노드 Shutdown → 매니저 컨트롤러 stop → bridge.Close 경로로 호출된다.
func (b *localFlowBridge) Close() error {
	// 먼저 subscriber 를 제거하여 새 emitOutput 이 이 핸들을 호출하지 않게 한다(컨트롤러가
	// 이미 shutdown 으로 subscriber 를 비웠으면 no-op).
	b.ctrl.removeSubscriber(b.sub)
	b.closeChannels()
	return nil
}

// closeChannels 는 출력/상태 채널을 정확히 한 번 닫는다(closeOnce). Close(부모 undeploy)와
// 컨트롤러 shutdown 의 onClose(참조 플로우 정지) 양쪽에서 호출되며, 어느 쪽이 먼저든 단일
// close 를 보장한다. closeMu 쓰기 잠금으로 진행 중인 deliverOutput(RLock)과 경합하지 않는다.
func (b *localFlowBridge) closeChannels() {
	b.closeOnce.Do(func() {
		b.closeMu.Lock()
		b.closed = true
		close(b.outputs)
		close(b.status)
		b.closeMu.Unlock()
	})
}

// deliverOutput 은 컨트롤러 emitOutput 콜백에서 호출되어 출력 메시지를 유계 버퍼로 비블로킹
// 전달한다(N03 — full 시 oldest-drop). closeMu RLock 으로 Close 와의 경합을 차단한다.
func (b *localFlowBridge) deliverOutput(port string, data json.RawMessage) {
	b.closeMu.RLock()
	defer b.closeMu.RUnlock()
	if b.closed {
		return // 이미 닫힘 — 드롭(Close 와의 경합).
	}
	sendManagerBridgeFrame(b.outputs, BridgeOutput{Port: port, Data: data})
}

// 컴파일 타임 계약.
var (
	_ FlowBridgeOpener         = (*localBridgeOpener)(nil)
	_ FlowBridge               = (*localFlowBridge)(nil)
	_ engineFlowStatusProvider = (*engine.Engine)(nil)
	_ sharedBoundaryLookup     = (*sharedBoundaryRegistry)(nil)
)
