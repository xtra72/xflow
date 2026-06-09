// bridge_runner.go 는 노드 측 라이브 브리지 실행 어댑터(BridgeFlowRunnerAdapter)와 경계
// 포트 tap 메커니즘을 구현한다(@SPEC:SPEC-SUBFLOW-001 그룹 RB, P2, REQ-SUBFLOW-RB05/RB06/
// RB07, OQ-RB1/RB5).
//
// 엔진 메커니즘(발견·사용):
//
//	엔진은 노드 간 메시지를 RuntimeWire 채널로 전달하며, 와이어는 SourceNodeID/
//	TargetNodeID 로 노드를 연결한다. 플로우 레벨 입출력 포트는 노드가 아니므로 예약
//	센티넬 노드 ID(__flow_input__/__flow_output__, pkg/flow/boundary.go)를 와이어
//	엔드포인트로 사용한다. 단독 배포 경로(FlowServiceAdapter.DeployFlow)는 이 경계
//	와이어를 StripBoundaryWires 로 제거한다(외부 카운터파트 없음 — 수신자 없는 채널
//	블로킹 방지).
//
//	브리지는 이 경계 와이어를 "외부 카운터파트(브리지 엔드포인트)"에 연결해야 한다.
//	엔진을 침습하지 않고(plan §1.3 — 엔진 subflow-agnostic 유지) 이를 달성하기 위해,
//	두 개의 실제 tap 노드 타입을 client-mode 노드 레지스트리에 등록하고 경계 와이어를
//	이 tap 노드로 재배선한다(subflow_expand 의 "직접 재배선" 철학과 동일):
//	  - 입력 tap(MultiSourceNode): 입력 경계 포트별 채널을 노출한다. Inject(port,data)가
//	    해당 포트 채널로 메시지를 흘리면 엔진이 출력 와이어(원래 __flow_input__→consumer)로
//	    전달한다. 즉 `__flow_input__:portX → consumer` 를 `inputTap:portX → consumer` 로 재배선.
//	  - 출력 tap(ProcessNode): 출력 경계 포트별 입력 포트를 갖고, 수신 메시지를 onOutput
//	    콜백으로 전달한다. 즉 `producer → __flow_output__:portY` 를 `producer → outputTap:portY`
//	    로 재배선. onOutput 은 엔진 스레드에서 인라인 호출되므로 비블로킹 계약이다(RB10 —
//	    호출 측 client 가 bounded buffer 로 흡수).
//
//	엔진은 이 두 타입을 일반 노드로 취급하며 변경이 전혀 없다. tap 노드는 NodeDef.Config
//	의 tapID 로 process-global bridgeTapTable 에서 per-flow 컨트롤러를 찾아 채널/콜백을
//	바인딩한다(registry.Create(def) 가 NodeDef 만 받으므로 side-channel 결선).
//
// 소유 모델(OQ-RB1/RB5 — 매니저 관리형 자동 배포, 재사용):
//
//	OpenBridge 는 flowID 별 컨트롤러를 refcount 한다. 첫 open 이 flow 를 tap-배포+start
//	하고, 같은 flowID 로의 추가 open 은 running 인스턴스를 재사용(subscriber 추가)한다.
//	각 BridgeHandle.Close 는 subscriber 를 제거하고, 마지막 subscriber 가 빠지면 이 노드가
//	auto-start 한 경우 flow 를 정지한다(다른 브리지가 쓰면 정지하지 않음).
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

const (
	// bridgeInputTapType / bridgeOutputTapType 은 경계 tap 노드 타입 식별자이다.
	// client-mode 엔진 레지스트리에만 등록되며(RegisterBridgeTapNodes), 일반 플로우에는
	// 나타나지 않는다(브리지 배포 시 재배선으로만 생성).
	bridgeInputTapType  = "__bridge_input_tap__"
	bridgeOutputTapType = "__bridge_output_tap__"

	// bridgeTapIDConfigKey 는 tap 노드가 자신의 per-flow 컨트롤러를 찾는 NodeDef.Config 키이다.
	bridgeTapIDConfigKey = "__bridge_tap_id__"
)

// 컴파일 타임 검증: 본 어댑터/핸들이 remote 의 브리지 계약을 구현한다(remote 는 service 를
// import 하지 않으므로 사이클 없음 — service→remote 단방향).
var (
	_ remote.BridgeFlowRunner = (*BridgeFlowRunnerAdapter)(nil)
	_ remote.BridgeHandle     = (*BridgeFlowHandle)(nil)
	// BridgeFlowRunnerAdapter 는 노드 측 라이브 브리지 tap 소스로서 FlowServiceAdapter 의
	// (재)배포가 활성 tap 을 재바인딩하도록 한다(참조 플로우 재시작 투명성).
	_ bridgeTapSource = (*BridgeFlowRunnerAdapter)(nil)
)

// BridgeFlowRunnerAdapter 는 참조 플로우를 경계 포트 tap 과 함께 실행하는 어댑터이다.
// flowID 별 tap 컨트롤러를 refcount 로 관리하여 자동배포/재사용/정지를 수행한다.
type BridgeFlowRunnerAdapter struct {
	adapter *FlowServiceAdapter
	engine  *engine.Engine
	logger  *slog.Logger

	mu          sync.Mutex
	controllers map[string]*bridgeTapController // flowID → 컨트롤러(running tap-flow)
}

// NewBridgeFlowRunnerAdapter 는 어댑터를 생성한다. adapter 는 참조 플로우 정의 조회/
// 확장에, engine 은 tap-배포/start/stop 에 사용된다.
func NewBridgeFlowRunnerAdapter(adapter *FlowServiceAdapter, eng *engine.Engine, logger *slog.Logger) *BridgeFlowRunnerAdapter {
	if logger == nil {
		logger = slog.Default()
	}
	return &BridgeFlowRunnerAdapter{
		adapter:     adapter,
		engine:      eng,
		logger:      logger,
		controllers: make(map[string]*bridgeTapController),
	}
}

// OpenBridge 는 flowID 참조 플로우가 running 이 되도록 보장하고(미실행이면 tap-배포+start,
// 실행 중이면 재사용 — OQ-RB1/RB5), 입출력 경계 포트를 tap 한다. onOutput 은 출력 경계
// 메시지마다 호출된다(비블로킹 계약 — RB10).
func (r *BridgeFlowRunnerAdapter) OpenBridge(ctx context.Context, flowID string, onOutput remote.BridgeOutputFunc) (remote.BridgeHandle, error) {
	r.mu.Lock()
	ctrl, ok := r.controllers[flowID]
	if !ok {
		// 첫 브리지 — tap-배포 + start.
		newCtrl, err := r.deployTapped(ctx, flowID)
		if err != nil {
			r.mu.Unlock()
			return nil, err
		}
		r.controllers[flowID] = newCtrl
		ctrl = newCtrl
	}
	sub := ctrl.addSubscriber(onOutput)
	r.mu.Unlock()

	return &BridgeFlowHandle{
		runner:  r,
		flowID:  flowID,
		ctrl:    ctrl,
		sub:     sub,
		inPorts: ctrl.inputPorts,
		outPort: ctrl.outputPorts,
	}, nil
}

// RetapDeployedBoundaries 는 flowID 에 대한 활성(비종료) 노드 측 브리지 tap 컨트롤러가
// 있으면 expanded 플로우의 경계 와이어를 동일 tapID 로 tap 노드에 재배선한 새 플로우와
// true 를 반환한다(없거나 종료된 컨트롤러면 expanded, false). 컨트롤러는 그대로
// controllers/bridgeTapTable 에 남겨, 재배포된 플로우의 새 tap 노드가 Init 에서 동일 LIVE
// 컨트롤러(출력 subscriber + 입력 채널 보존)에 재바인딩되도록 한다(참조 플로우 재시작 투명성).
//
// r.mu 를 전 구간 보유하여 closeSubscriber(컨트롤러 제거 + shutdown + tapID unregister 를
// r.mu 보유 중 수행)와의 경합을 차단한다. 따라서 closed 판정과 tap 재배선/재등록이 원자적이다.
// bridgeTapSource 인터페이스 구현(flow_adapter.go).
func (r *BridgeFlowRunnerAdapter) RetapDeployedBoundaries(flowID string, expanded flow.Flow) (flow.Flow, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	ctrl, ok := r.controllers[flowID]
	if !ok {
		return expanded, false // 활성 노드 측 브리지 없음 — 무변경.
	}
	// 컨트롤러 종료 여부를 동일 r.mu 보유 하에 확인한다(closeSubscriber 가 r.mu 보유 중
	// shutdown 하므로 원자적). 종료되었으면 채널이 닫혔으므로 재바인딩하지 않는다.
	ctrl.mu.Lock()
	closed := ctrl.closed
	ctrl.mu.Unlock()
	if closed {
		return expanded, false
	}

	tapID := ctrl.tapID
	// 동일 tapID 재등록(멱등). closeSubscriber 가 아직 unregister 하지 않았다면 no-op 이고,
	// 어떤 경우든 새 tap 노드 Init 이 tapID 로 LIVE 컨트롤러를 찾도록 보장한다.
	bridgeTapTable.register(tapID, ctrl)

	tapped, inPorts, outPorts := rewireBoundariesToTap(expanded, tapID)

	// 엣지 케이스: 재시작 중 경계 포트가 바뀌면 매니저의 이름 매핑 핸들과 어긋날 수 있다.
	// 재배선은 현재 존재하는 포트로 그대로 수행하고 경고만 남긴다(over-engineering 금지).
	if !stringSetsEqual(inPorts, ctrl.inputPorts) || !stringSetsEqual(outPorts, ctrl.outputPorts) {
		r.logger.Warn("브리지 재배포: 참조 플로우 경계 포트 변경 감지 — 매니저 핸들과 어긋날 수 있음",
			"flow_id", flowID,
			"old_inputs", ctrl.inputPorts, "new_inputs", inPorts,
			"old_outputs", ctrl.outputPorts, "new_outputs", outPorts)
	}

	return tapped, true
}

// stringSetsEqual 은 두 문자열 슬라이스가 같은 집합인지 반환한다(순서 무시, 경계 포트는
// 정렬·중복 없음이므로 길이+원소 비교로 충분).
func stringSetsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	m := make(map[string]struct{}, len(a))
	for _, s := range a {
		m[s] = struct{}{}
	}
	for _, s := range b {
		if _, ok := m[s]; !ok {
			return false
		}
	}
	return true
}

// closeSubscriber 는 BridgeHandle.Close 경로에서 호출되어 subscriber 를 제거하고, 마지막
// subscriber 가 빠지면 auto-start 한 flow 를 정지·tap 정리한다(refcount 라이프사이클).
func (r *BridgeFlowRunnerAdapter) closeSubscriber(ctx context.Context, flowID string, ctrl *bridgeTapController, sub *bridgeOutputSub) {
	r.mu.Lock()
	defer r.mu.Unlock()
	remaining := ctrl.removeSubscriber(sub)
	if remaining > 0 {
		return // 다른 브리지가 아직 사용 중 — 정지하지 않음.
	}
	// 마지막 subscriber — 컨트롤러 제거 + flow 정지(이 노드가 auto-start 한 경우).
	if cur, ok := r.controllers[flowID]; ok && cur == ctrl {
		delete(r.controllers, flowID)
	}
	ctrl.shutdown()
	if ctrl.startedByBridge {
		if err := r.engine.StopFlow(ctx, flowID); err != nil {
			r.logger.Warn("브리지 참조 플로우 정지 실패", "flow_id", flowID, "error", err)
		}
		if err := r.engine.UndeployFlow(ctx, flowID); err != nil {
			r.logger.Debug("브리지 참조 플로우 배포 해제 실패", "flow_id", flowID, "error", err)
		}
	}
	// tap 테이블에서 컨트롤러 등록 해제.
	bridgeTapTable.unregister(ctrl.tapID)
}

// deployTapped 는 flowID 참조 플로우를 조회·확장하고 경계 와이어를 tap 노드로 재배선하여
// 엔진에 배포+start 한다. 컨트롤러(입출력 경계 포트 + tap 결선)를 반환한다.
//
// r.mu 보유 중 호출된다(OpenBridge). 엔진 배포/start 는 r.mu 와 무관하므로 안전하다.
func (r *BridgeFlowRunnerAdapter) deployTapped(ctx context.Context, flowID string) (*bridgeTapController, error) {
	f, err := r.adapter.repo.Get(ctx, flowID)
	if err != nil {
		return nil, fmt.Errorf("bridge: 참조 플로우 조회 실패(flow_id=%q): %w", flowID, err)
	}

	// LOCAL 서브플로우 확장(중첩 참조 평탄화 — 원격 참조는 본 노드 범위 밖이라 미사용).
	expanded, expErr := ExpandSubflows(ctx, f, r.adapter.repo)
	if expErr != nil {
		return nil, fmt.Errorf("bridge: 서브플로우 확장 실패(flow_id=%q): %w", flowID, expErr)
	}

	// 경계 포트 수집 + tap 노드 재배선.
	tapID := newTapID()
	ctrl := newBridgeTapController(tapID)
	tapped, inPorts, outPorts := rewireBoundariesToTap(expanded, tapID)
	ctrl.inputPorts = inPorts
	ctrl.outputPorts = outPorts

	// tap 테이블에 등록(노드 Init 이 tapID 로 컨트롤러를 찾도록).
	bridgeTapTable.register(tapID, ctrl)

	// 이미 배포/실행 중인지 확인(재사용 — OQ-RB1/RB5). 미배포면 이 브리지가 시작한다.
	startedByBridge := true
	if status, sErr := r.engine.GetFlowStatus(flowID); sErr == nil {
		if status.State == flow.FlowRunning || status.State == flow.FlowPaused {
			// 이미 running(브리지 외 경로로 배포). tap 이 없는 인스턴스이므로 재배포가
			// 필요하나, 외부 소유 인스턴스를 중단시키지 않기 위해 이 경우는 기존 인스턴스를
			// 그대로 두고 tap 없는 재사용은 하지 않는다 — 명확성을 위해 재배포한다.
			startedByBridge = false
		}
		// 기존 배포를 해제하고 tap-배포로 교체한다(undeploy → deploy).
		if status.State == flow.FlowRunning || status.State == flow.FlowPaused {
			_ = r.engine.StopFlow(ctx, flowID)
		}
		_ = r.engine.UndeployFlow(ctx, flowID)
	}

	if err := r.engine.DeployFlow(ctx, tapped); err != nil {
		bridgeTapTable.unregister(tapID)
		return nil, fmt.Errorf("bridge: tap 배포 실패(flow_id=%q): %w", flowID, err)
	}
	if err := r.engine.StartFlow(ctx, flowID); err != nil {
		_ = r.engine.UndeployFlow(ctx, flowID)
		bridgeTapTable.unregister(tapID)
		return nil, fmt.Errorf("bridge: tap start 실패(flow_id=%q): %w", flowID, err)
	}
	ctrl.startedByBridge = startedByBridge
	return ctrl, nil
}

// BridgeFlowHandle 은 단일 브리지(참조 플로우 1 인스턴스의 1 subscriber)의 핸들이다.
type BridgeFlowHandle struct {
	runner  *BridgeFlowRunnerAdapter
	flowID  string
	ctrl    *bridgeTapController
	sub     *bridgeOutputSub
	inPorts []string
	outPort []string

	closed atomic.Bool
}

// InputPorts 는 참조 플로우의 입력 경계 포트 이름 목록이다(RB06).
func (h *BridgeFlowHandle) InputPorts() []string { return h.inPorts }

// OutputPorts 는 참조 플로우의 출력 경계 포트 이름 목록이다(RB06).
func (h *BridgeFlowHandle) OutputPorts() []string { return h.outPort }

// Inject 는 입력 경계 포트(port)로 메시지(data JSON)를 주입한다(WRITE — RB07).
func (h *BridgeFlowHandle) Inject(ctx context.Context, port string, data json.RawMessage) error {
	return h.ctrl.inject(ctx, port, data)
}

// Close 는 이 브리지의 subscriber 를 제거하고, 마지막이면 auto-start 한 flow 를 정지한다
// (멱등 — refcount 라이프사이클).
func (h *BridgeFlowHandle) Close(ctx context.Context) error {
	if !h.closed.CompareAndSwap(false, true) {
		return nil
	}
	h.runner.closeSubscriber(ctx, h.flowID, h.ctrl, h.sub)
	return nil
}

// ---------------------------------------------------------------------------
// 경계 재배선
// ---------------------------------------------------------------------------

// rewireBoundariesToTap 는 플로우의 경계(센티넬) 와이어를 tap 노드로 재배선한 새 플로우와
// 입출력 경계 포트 이름 목록을 반환한다. 비-경계 와이어/노드는 보존한다.
func rewireBoundariesToTap(f flow.Flow, tapID string) (tappedFlow flow.Flow, inputPorts []string, outputPorts []string) {
	srcNodes := f.Nodes()
	srcWires := f.Wires()

	inSet := make(map[string]struct{})
	outSet := make(map[string]struct{})
	resultWires := make([]flow.Wire, 0, len(srcWires))

	for _, w := range srcWires {
		switch {
		case w.SourceNodeID == flow.FlowInputBoundaryID && w.TargetNodeID == flow.FlowOutputBoundaryID:
			// 입력 포트 → 출력 포트 직결(passthrough): 입력 tap → 출력 tap 로 재배선.
			inSet[w.SourcePort] = struct{}{}
			outSet[w.TargetPort] = struct{}{}
			resultWires = append(resultWires, retargetWire(w, bridgeInputTapNodeID, w.SourcePort, bridgeOutputTapNodeID, w.TargetPort))
		case w.SourceNodeID == flow.FlowInputBoundaryID:
			// 입력 경계: 입력 tap(SourcePort) → 내부 소비자.
			inSet[w.SourcePort] = struct{}{}
			resultWires = append(resultWires, retargetWire(w, bridgeInputTapNodeID, w.SourcePort, w.TargetNodeID, w.TargetPort))
		case w.TargetNodeID == flow.FlowOutputBoundaryID:
			// 출력 경계: 내부 생산자 → 출력 tap(TargetPort).
			outSet[w.TargetPort] = struct{}{}
			resultWires = append(resultWires, retargetWire(w, w.SourceNodeID, w.SourcePort, bridgeOutputTapNodeID, w.TargetPort))
		default:
			// 비-경계 와이어: 그대로 보존.
			resultWires = append(resultWires, w)
		}
	}

	inputPorts = setToSortedSlice(inSet)
	outputPorts = setToSortedSlice(outSet)

	resultNodes := make([]flow.NodeDef, 0, len(srcNodes)+2)
	resultNodes = append(resultNodes, srcNodes...)

	// 입력 tap 노드: 입력 경계 포트별 출력 포트를 선언한다(엔진이 출력 와이어로 라우팅).
	if len(inputPorts) > 0 {
		resultNodes = append(resultNodes, tapNodeDef(bridgeInputTapNodeID, bridgeInputTapType, tapID, nil, inputPorts))
	}
	// 출력 tap 노드: 출력 경계 포트별 입력 포트를 선언한다(엔진이 입력 와이어를 매핑).
	if len(outputPorts) > 0 {
		resultNodes = append(resultNodes, tapNodeDef(bridgeOutputTapNodeID, bridgeOutputTapType, tapID, outputPorts, nil))
	}

	return flow.RebuildFlow(f, resultNodes, resultWires), inputPorts, outputPorts
}

const (
	bridgeInputTapNodeID  = "__bridge_input_tap_node__"
	bridgeOutputTapNodeID = "__bridge_output_tap_node__"
)

// retargetWire 는 와이어의 양 끝을 지정 엔드포인트로 재작성한 복사본을 반환한다(센티넬→tap).
func retargetWire(w flow.Wire, srcNode, srcPort, tgtNode, tgtPort string) flow.Wire {
	w.SourceNodeID = srcNode
	w.SourcePort = srcPort
	w.TargetNodeID = tgtNode
	w.TargetPort = tgtPort
	return w
}

// tapNodeDef 는 tap 노드 정의를 만든다. inputs/outputs 는 경계 포트 이름으로 구성하고,
// Config 에 tapID 를 실어 노드 Init 이 컨트롤러를 찾게 한다.
func tapNodeDef(id, typ, tapID string, inputs, outputs []string) flow.NodeDef {
	nd := flow.NodeDef{
		ID:     id,
		Name:   id,
		Type:   typ,
		Config: map[string]any{bridgeTapIDConfigKey: tapID},
	}
	for _, p := range inputs {
		nd.Inputs = append(nd.Inputs, flow.Port{ID: p, Name: p, Direction: flow.PortInput})
	}
	for _, p := range outputs {
		nd.Outputs = append(nd.Outputs, flow.Port{ID: p, Name: p, Direction: flow.PortOutput})
	}
	return nd
}

// setToSortedSlice 는 set 을 정렬된 슬라이스로 변환한다(결정적 ack 포트 순서).
func setToSortedSlice(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	// 작은 N(경계 포트)이므로 단순 삽입 정렬로 충분하나 sort 사용.
	sortStrings(out)
	return out
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}

// ---------------------------------------------------------------------------
// tap 컨트롤러 + process-global 테이블
// ---------------------------------------------------------------------------

// bridgeOutputSub 는 출력 경계 메시지 1 subscriber(=1 브리지)의 콜백이다.
type bridgeOutputSub struct {
	onOutput func(port string, data json.RawMessage)
}

// bridgeTapController 는 한 running tap-flow 의 입력 채널 + 출력 subscriber 집합을 보유한다.
type bridgeTapController struct {
	tapID       string
	inputPorts  []string
	outputPorts []string

	mu          sync.Mutex
	inputChans  map[string]chan message.Message // 입력 포트별 source 채널(입력 tap 노드가 소비)
	subscribers map[*bridgeOutputSub]struct{}
	closed      bool

	startedByBridge bool // 이 브리지가 flow 를 start 했는지(정지 판단)
}

func newBridgeTapController(tapID string) *bridgeTapController {
	return &bridgeTapController{
		tapID:       tapID,
		inputChans:  make(map[string]chan message.Message),
		subscribers: make(map[*bridgeOutputSub]struct{}),
	}
}

// ensureInputChan 은 입력 포트 채널을 보장 생성한다(입력 tap 노드 Init 이 바인딩).
func (c *bridgeTapController) ensureInputChan(port string) chan message.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.inputChans[port]
	if !ok {
		ch = make(chan message.Message, 64)
		c.inputChans[port] = ch
	}
	return ch
}

// inject 는 입력 포트 채널로 메시지를 흘린다(WRITE — RB07). 채널 미생성/닫힘 시 오류.
func (c *bridgeTapController) inject(ctx context.Context, port string, data json.RawMessage) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("bridge: 닫힌 브리지로의 주입(port=%q)", port)
	}
	ch, ok := c.inputChans[port]
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("bridge: 알 수 없는 입력 경계 포트(port=%q)", port)
	}

	msg, err := messageFromJSON(data)
	if err != nil {
		return fmt.Errorf("bridge: 입력 페이로드 디코드 실패: %w", err)
	}
	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// emitOutput 은 출력 경계 메시지를 모든 subscriber 로 fan-out 한다(출력 tap 노드가 호출).
// 콜백은 비블로킹 계약(client 가 bounded buffer 로 흡수 — RB10)이므로 인라인 호출한다.
func (c *bridgeTapController) emitOutput(port string, data json.RawMessage) {
	c.mu.Lock()
	subs := make([]*bridgeOutputSub, 0, len(c.subscribers))
	for s := range c.subscribers {
		subs = append(subs, s)
	}
	c.mu.Unlock()
	for _, s := range subs {
		s.onOutput(port, data)
	}
}

// addSubscriber 는 출력 subscriber 를 추가한다(refcount 증가).
func (c *bridgeTapController) addSubscriber(onOutput func(port string, data json.RawMessage)) *bridgeOutputSub {
	sub := &bridgeOutputSub{onOutput: onOutput}
	c.mu.Lock()
	c.subscribers[sub] = struct{}{}
	c.mu.Unlock()
	return sub
}

// removeSubscriber 는 subscriber 를 제거하고 남은 수를 반환한다(refcount 감소).
func (c *bridgeTapController) removeSubscriber(sub *bridgeOutputSub) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subscribers, sub)
	return len(c.subscribers)
}

// shutdown 은 입력 채널을 닫고 컨트롤러를 종료 표시한다(teardown).
func (c *bridgeTapController) shutdown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return
	}
	c.closed = true
	for _, ch := range c.inputChans {
		close(ch)
	}
}

// bridgeTapTable 은 tapID → 컨트롤러 매핑의 process-global 레지스트리이다. tap 노드
// Init 이 NodeDef.Config 의 tapID 로 컨트롤러를 찾아 채널/콜백을 결선한다(registry.Create
// 가 NodeDef 만 받으므로 side-channel).
var bridgeTapTable = &tapTable{m: make(map[string]*bridgeTapController)}

type tapTable struct {
	mu sync.RWMutex
	m  map[string]*bridgeTapController
}

func (t *tapTable) register(tapID string, c *bridgeTapController) {
	t.mu.Lock()
	t.m[tapID] = c
	t.mu.Unlock()
}

func (t *tapTable) unregister(tapID string) {
	t.mu.Lock()
	delete(t.m, tapID)
	t.mu.Unlock()
}

func (t *tapTable) lookup(tapID string) (*bridgeTapController, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	c, ok := t.m[tapID]
	return c, ok
}

var tapIDCounter atomic.Uint64

// newTapID 는 프로세스 내 유일한 tapID 를 생성한다.
func newTapID() string {
	return fmt.Sprintf("tap-%d", tapIDCounter.Add(1))
}

// messageFromJSON 는 JSON 으로부터 메시지를 구성한다(입력 주입 / 출력 역직렬화).
//
// 봉투(envelope) 형태 — 최상위 객체가 "payload" 키 AND 형제 봉투 키
// {id,type,metadata,timestamp} 중 하나 이상을 가지면 — 는 id/type/timestamp/metadata/
// payload 를 모두 복원한다(출력/디버그 노드 buildMessageMap 와 동일 코어 형태이므로 브리지
// 통과 메시지가 로컬 메시지와 구분 불가하게 됨 — RB06 회귀 수정). messageToJSON 은 항상
// 다섯 키를 모두 방출하므로 이 판별식은 안전하다.
//
// 봉투가 아니면(형제 봉투 키 없는 객체/비객체/스칼라/배열) 기존 graceful 동작을 유지한다:
//   - 객체이면 그 객체를 payload 로 사용(예: 원시 주입 {"payload": {...}} 는 payload 키만
//     있고 형제 봉투 키가 없으므로 봉투가 아닌 원시 페이로드로 취급)
//   - 비객체(스칼라/배열)이면 {"value": <raw>} 로 감싼다
//   - 빈 입력이면 빈 페이로드 메시지
func messageFromJSON(data json.RawMessage) (message.Message, error) {
	if len(data) == 0 {
		return message.New(message.WithPayload(message.NewPayload())), nil
	}

	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		// 비-객체 페이로드(스칼라/배열): value 로 감싼다(graceful).
		var raw any
		if err2 := json.Unmarshal(data, &raw); err2 != nil {
			return nil, err
		}
		return message.New(message.WithPayload(message.NewPayload(map[string]any{"value": raw}))), nil
	}

	// 봉투 감지(강화): "payload" 키 AND 형제 봉투 키 1개 이상.
	if pv, ok := m["payload"]; ok && hasEnvelopeSiblingKey(m) {
		return messageFromEnvelope(m, pv), nil
	}

	// 봉투가 아닌 일반 객체: 객체 전체를 payload 로 사용(기존 동작 유지).
	return message.New(message.WithPayload(message.NewPayload(m))), nil
}

// hasEnvelopeSiblingKey 는 맵에 봉투 형제 키(id/type/metadata/timestamp) 가 하나 이상
// 존재하는지 반환한다. payload 키만 단독으로 있는 원시 주입을 봉투로 오인하지 않게 한다.
func hasEnvelopeSiblingKey(m map[string]any) bool {
	for _, k := range []string{"id", "type", "metadata", "timestamp"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

// messageFromEnvelope 는 봉투 맵(m)과 그 payload 값(pv)으로부터 메시지를 완전 복원한다.
// id/type 은 비어있지 않으면 복원하고, timestamp 는 epoch ms(float64/int64 허용),
// metadata 는 문자열 값만 채택한다(비문자열 값은 무시 — 메타데이터 계약).
func messageFromEnvelope(m map[string]any, pv any) message.Message {
	opts := make([]message.Option, 0, 6)

	// payload: 객체이면 그대로, 아니면 value 로 감싼다(봉투 안에서도 graceful).
	if pm, ok := pv.(map[string]any); ok {
		opts = append(opts, message.WithPayload(message.NewPayload(pm)))
	} else {
		opts = append(opts, message.WithPayload(message.NewPayload(map[string]any{"value": pv})))
	}

	// id: 비어있지 않으면 복원(빈/누락 시 New() 가 uuid 생성).
	if id, ok := m["id"].(string); ok && id != "" {
		opts = append(opts, message.WithID(id))
	}

	// type: 비어있지 않으면 복원.
	if typ, ok := m["type"].(string); ok && typ != "" {
		opts = append(opts, message.WithType(typ))
	}

	// timestamp: epoch ms. JSON 디코드는 float64, 직접 주입은 int64 일 수 있다.
	if ts, ok := envelopeEpochMillis(m["timestamp"]); ok {
		opts = append(opts, message.WithTimestamp(time.UnixMilli(ts)))
	}

	// metadata: map 의 문자열 값만 복원(점 표기 키 포함).
	if md, ok := m["metadata"].(map[string]any); ok {
		for k, v := range md {
			if sv, ok := v.(string); ok {
				opts = append(opts, message.WithMetadata(k, sv))
			}
		}
	}

	return message.New(opts...)
}

// envelopeEpochMillis 는 봉투의 timestamp 값을 epoch ms(int64)로 정규화한다.
// JSON 숫자(float64), 직접 주입(int64/int), 문자열 숫자 등을 허용한다.
func envelopeEpochMillis(v any) (int64, bool) {
	switch t := v.(type) {
	case float64:
		return int64(t), true
	case int64:
		return t, true
	case int:
		return int64(t), true
	case json.Number:
		if n, err := t.Int64(); err == nil {
			return n, true
		}
	}
	return 0, false
}

// messageToJSON 는 메시지를 봉투(envelope) JSON 으로 직렬화한다(출력 전송 / 입력 직렬화).
//
// 형태는 출력/디버그 노드(buildMessageMap)의 코어 필드와 정확히 일치한다:
//
//	{"id":..,"type":..,"timestamp":<epoch ms>,"payload":{..},"metadata":{..}}
//
// 이로써 브리지를 통과한 메시지가 로컬 출력과 구분 불가하게 된다(RB06).
// 마샬 실패 시 기존 payload-only(Payload().ToJSON())로, 그것마저 실패하면 {} 로 폴백한다.
// redaction 은 호출 측 client 가 수행한다(노드 측 — §5.14). 리댁터는 봉투의 payload 내
// 중첩 시크릿도 재귀 제거하므로 봉투 도입으로 redaction 이 약화되지 않는다(RB06).
func messageToJSON(msg message.Message) json.RawMessage {
	env := map[string]any{
		"id":        msg.ID(),
		"type":      msg.Type(),
		"timestamp": msg.Timestamp().UnixMilli(),
		"payload":   msg.Payload().ToMap(),
		"metadata":  msg.Metadata().All(),
	}
	if data, err := json.Marshal(env); err == nil {
		return data
	}
	// 폴백: payload-only(기존 동작).
	if data, err := msg.Payload().ToJSON(); err == nil {
		return data
	}
	return json.RawMessage(`{}`)
}
