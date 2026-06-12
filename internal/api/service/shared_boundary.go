// shared_boundary.go 는 로컬 `shared` 모드 서브플로우 연결의 in-process 경계 포트 탭(tap)
// 인프라를 구현한다(@SPEC:SPEC-SUBFLOW-002 그룹 SH/MR/L, REQ-SUBFLOW2-SH02/SH03/MR01~MR04).
//
// 배경 — 왜 별도의 공유 경계 탭이 필요한가:
//
//	`shared` flow-node 는 참조 플로우의 "플로우 리스트에 있는 그 단일 실행 인스턴스"에
//	연결되어야 한다(복사본 금지 — SH02). 그러나 단독 배포된 플로우는 DeployFlow 가
//	StripBoundaryWires 로 경계(__flow_input__/__flow_output__) 와이어를 제거하므로, 실행
//	인스턴스에는 외부에서 주입/수신할 탭 지점이 없다. 그래서 참조 플로우가 "자기 자신을
//	배포할 때" 경계 와이어를 in-process 공유 탭 노드로 재배선하고, 그 탭을 flow_id 기준
//	process-global 레지스트리(sharedBoundaryTable)에 등록한다. 이렇게 하면:
//	  - 여러 부모(또는 한 부모 안 여러 flow-node)가 같은 flow_id 컨트롤러에 attach 되어
//	    입력 fan-in / 출력 fan-out 이 자연히 성립한다(MR01~MR04).
//	  - 브리지가 0개여도 입력 채널에 생산자가 없고 출력은 0 subscriber 로 드롭되므로,
//	    StripBoundaryWires 와 동작이 동일하다(외부 카운터파트 없음 = 무출력 — 회귀 0).
//	  - 참조 플로우의 라이프사이클(배포/정지)이 컨트롤러의 존재/부재를 결정하므로, 매니저
//	    측 self-healing supervisor(managerBridgeController)가 오프라인 대기·재연결을 그대로
//	    수행한다(L02~L05). 참조 카운팅/자동 정지는 없다(L06).
//
// 노드 측 bridge_tap_nodes.go(remote 라이브 브리지의 노드 측 탭)와 구조는 유사하나, 의도적으로
// 분리한다(N01 회귀 0): 노드 측 탭은 tapID 기준이며 flow 라이프사이클을 refcount 로 소유하는
// 반면, 본 공유 경계 탭은 flow_id 기준이며 flow 라이프사이클을 소유하지 않는다(참조 플로우가
// 독립적으로 실행). 두 메커니즘을 한 테이블/타입으로 합치면 소유 의미가 충돌하므로 분리한다.
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

const (
	// sharedBoundaryInputTapType / sharedBoundaryOutputTapType 은 공유 경계 탭 노드 타입이다.
	// shared 모드를 지원하는 엔진 레지스트리에만 등록되며(RegisterSharedBoundaryNodes), 일반
	// 플로우에는 나타나지 않는다(참조 플로우 배포 시 경계 재배선으로만 생성).
	sharedBoundaryInputTapType  = "__shared_boundary_input_tap__"
	sharedBoundaryOutputTapType = "__shared_boundary_output_tap__"

	// sharedBoundaryIDConfigKey 는 탭 노드가 자신의 per-flow 컨트롤러를 찾는 NodeDef.Config 키이다.
	sharedBoundaryIDConfigKey = "__shared_boundary_id__"

	// sharedBoundaryInputTapNodeID / sharedBoundaryOutputTapNodeID 는 재배선으로 생성하는
	// 공유 경계 탭 노드의 고정 ID 이다. 각 참조 플로우는 자기 자신의 배포이므로 플로우 내에서
	// 유일하다(노드 측 탭과 별도 ID 라 동시 적용 시에도 충돌하지 않는다).
	sharedBoundaryInputTapNodeID  = "__shared_boundary_input_tap_node__"
	sharedBoundaryOutputTapNodeID = "__shared_boundary_output_tap_node__"

	// sharedBoundaryInputChanBuffer 는 입력 경계 포트별 in-process 채널의 유계 버퍼이다(N03).
	sharedBoundaryInputChanBuffer = 64
)

// sharedBoundaryController 는 한 참조 플로우(flow_id)의 실행 인스턴스에 대한 in-process 경계
// 탭 상태를 보유한다. 입력 경계 포트별 채널(fan-in 합류)과 출력 subscriber 집합(fan-out 복제)을
// 관리한다. 참조 플로우 배포 시 생성·등록되고, 정지(undeploy) 시 shutdown·등록 해제된다.
//
// 라이프사이클 소유 없음(L06): 본 컨트롤러는 참조 플로우를 시작/정지하지 않는다. 단지 이미
// 실행 중인 인스턴스의 경계 포트에 in-process 로 attach 할 뿐이다.
type sharedBoundaryController struct {
	flowID      string
	inputPorts  []string
	outputPorts []string

	mu          sync.Mutex
	inputChans  map[string]chan message.Message // 입력 경계 포트별 source 채널(입력 탭 노드가 소비)
	subscribers map[*sharedBoundarySub]struct{} // 출력 fan-out subscriber 집합
	closed      bool
}

// sharedBoundarySub 는 출력 경계 메시지 1 subscriber(=1 브리지 엔드포인트)의 콜백이다.
//   - onOutput: 출력 경계 메시지 fan-out 콜백.
//   - onClose:  컨트롤러 shutdown(참조 플로우 정지) 시 호출되어, 이 subscriber 의 브리지가
//     자신의 출력/상태 채널을 닫도록 신호한다. 매니저 측 브리지 컨트롤러가 채널 close(!ok)를
//     관측해 self-heal supervisor 가 재open 백오프로 전환하게 한다(L04 드롭 → L05 재연결).
type sharedBoundarySub struct {
	onOutput func(port string, data json.RawMessage)
	onClose  func()
}

// newSharedBoundaryController 는 컨트롤러를 생성한다(참조 플로우 배포 시).
func newSharedBoundaryController(flowID string) *sharedBoundaryController {
	return &sharedBoundaryController{
		flowID:      flowID,
		inputChans:  make(map[string]chan message.Message),
		subscribers: make(map[*sharedBoundarySub]struct{}),
	}
}

// ensureInputChan 은 입력 경계 포트 채널을 보장 생성한다(입력 탭 노드 Init 이 바인딩).
func (c *sharedBoundaryController) ensureInputChan(port string) chan message.Message {
	c.mu.Lock()
	defer c.mu.Unlock()
	ch, ok := c.inputChans[port]
	if !ok {
		ch = make(chan message.Message, sharedBoundaryInputChanBuffer)
		c.inputChans[port] = ch
	}
	return ch
}

// inject 는 입력 경계 포트 채널로 메시지를 흘린다(WRITE — 다중 부모 fan-in, MR02). 채널
// 미생성/닫힘 시 오류를 반환한다. 채널 full 이면 oldest-drop 후 재시도하여 유계를 유지한다(N03).
func (c *sharedBoundaryController) inject(ctx context.Context, port string, data json.RawMessage) error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return fmt.Errorf("shared-boundary: 닫힌 컨트롤러로의 주입(flow_id=%q, port=%q)", c.flowID, port)
	}
	ch, ok := c.inputChans[port]
	c.mu.Unlock()
	if !ok {
		return fmt.Errorf("shared-boundary: 알 수 없는 입력 경계 포트(flow_id=%q, port=%q)", c.flowID, port)
	}

	msg, err := messageFromJSON(data)
	if err != nil {
		return fmt.Errorf("shared-boundary: 입력 페이로드 디코드 실패: %w", err)
	}

	// 비블로킹 송신 우선. full 이면 oldest-drop 후 1회 재시도(유계 버퍼 — N03). ctx 취소도 존중.
	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	// oldest-drop: 가장 오래된 메시지 1개를 비우고 다시 시도한다.
	select {
	case <-ch:
	default:
	}
	select {
	case ch <- msg:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil // 여전히 경합 — 드롭(라이브 브리지의 상태 동기화 의미와 일관).
	}
}

// emitOutput 은 출력 경계 메시지를 모든 subscriber 로 fan-out 한다(출력 탭 노드가 호출, MR03).
// 콜백은 비블로킹 계약(매니저 컨트롤러가 유계 버퍼로 흡수 — N03)이므로 인라인 호출한다.
func (c *sharedBoundaryController) emitOutput(port string, data json.RawMessage) {
	c.mu.Lock()
	subs := make([]*sharedBoundarySub, 0, len(c.subscribers))
	for s := range c.subscribers {
		subs = append(subs, s)
	}
	c.mu.Unlock()
	for _, s := range subs {
		s.onOutput(port, data)
	}
}

// addSubscriber 는 출력 subscriber 를 추가한다(fan-out 대상 1 증가). onClose 는 컨트롤러
// shutdown 시 호출되어 브리지가 채널을 닫고 self-heal 을 트리거하게 한다(L04/L05). 컨트롤러가
// 이미 닫혔으면 nil 을 반환한다(참조 플로우가 막 정지된 경합 — 호출자가 오프라인으로 처리).
func (c *sharedBoundaryController) addSubscriber(onOutput func(port string, data json.RawMessage), onClose func()) *sharedBoundarySub {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	sub := &sharedBoundarySub{onOutput: onOutput, onClose: onClose}
	c.subscribers[sub] = struct{}{}
	return sub
}

// removeSubscriber 는 subscriber 를 제거한다(브리지 Close 시 — 참조 플로우는 정지하지 않음,
// L06/L07). 참조 카운팅에 따른 자동 정지는 수행하지 않는다.
func (c *sharedBoundaryController) removeSubscriber(sub *sharedBoundarySub) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subscribers, sub)
}

// shutdown 은 입력 채널을 닫고 컨트롤러를 종료 표시한 뒤, 각 subscriber 의 onClose 를 호출해
// 브리지가 출력/상태 채널을 닫도록 신호한다(참조 플로우 정지/undeploy 시 — L04). 매니저 측
// 브리지 컨트롤러는 채널 close(!ok)를 관측해 self-heal supervisor 가 재open 백오프로 전환한다
// (참조 플로우 재시작 시 새 컨트롤러로 자동 재연결 — L05).
func (c *sharedBoundaryController) shutdown() {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	for _, ch := range c.inputChans {
		close(ch)
	}
	// onClose 콜백을 잠금 밖에서 호출하기 위해 스냅샷을 뜬다(브리지 Close 와의 재진입 회피).
	subs := make([]*sharedBoundarySub, 0, len(c.subscribers))
	for s := range c.subscribers {
		subs = append(subs, s)
	}
	c.subscribers = make(map[*sharedBoundarySub]struct{})
	c.mu.Unlock()

	for _, s := range subs {
		if s.onClose != nil {
			s.onClose() // 브리지가 자신의 출력/상태 채널을 닫음(self-heal 트리거).
		}
	}
}

// isClosed 는 컨트롤러 종료 여부를 반환한다(opener 가 attach 가능 여부 판단).
func (c *sharedBoundaryController) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// ---------------------------------------------------------------------------
// process-global 테이블 (flow_id → 컨트롤러)
// ---------------------------------------------------------------------------

// sharedBoundaryRegistry 는 flow_id → 공유 경계 컨트롤러 매핑의 레지스트리이다. 탭 노드
// Init 이 NodeDef.Config 의 flow_id 로 컨트롤러를 찾아 채널/콜백을 결선하고(side-channel —
// registry.Create 가 NodeDef 만 받으므로), 로컬 opener 가 flow_id 로 컨트롤러를 조회한다.
type sharedBoundaryRegistry struct {
	mu sync.RWMutex
	m  map[string]*sharedBoundaryController
}

func newSharedBoundaryRegistry() *sharedBoundaryRegistry {
	return &sharedBoundaryRegistry{m: make(map[string]*sharedBoundaryController)}
}

func (t *sharedBoundaryRegistry) register(flowID string, c *sharedBoundaryController) {
	t.mu.Lock()
	t.m[flowID] = c
	t.mu.Unlock()
}

// unregister 는 컨트롤러를 등록 해제한다(현재 등록된 것이 c 와 동일할 때만 — 재배포 경합 방어).
func (t *sharedBoundaryRegistry) unregister(flowID string, c *sharedBoundaryController) {
	t.mu.Lock()
	if cur, ok := t.m[flowID]; ok && (c == nil || cur == c) {
		delete(t.m, flowID)
	}
	t.mu.Unlock()
}

func (t *sharedBoundaryRegistry) lookup(flowID string) (*sharedBoundaryController, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	c, ok := t.m[flowID]
	return c, ok
}

// globalSharedBoundaryTable 은 탭 노드·배포 경로·로컬 opener 가 공유하는 process-global
// 레지스트리이다.
var globalSharedBoundaryTable = newSharedBoundaryRegistry()

// ---------------------------------------------------------------------------
// 경계 재배선 (참조 플로우 자기 배포 시)
// ---------------------------------------------------------------------------

// installSharedBoundaryTaps 는 플로우 f 의 경계(센티넬) 와이어를 공유 경계 탭 노드로 재배선한
// 새 플로우를 반환하고, 등록한 컨트롤러(없으면 nil)를 함께 반환한다. f 에 경계 와이어가 없으면
// 무변경으로 (f, nil) 을 반환한다(잎 플로우 — 회귀 0). 컨트롤러는 flowID 기준으로 테이블에
// 등록된다(이전 배포 컨트롤러는 호출자가 정리).
//
// StripBoundaryWires 의 대체(shared 지원 경로): 경계 와이어가 있는 플로우는 더 이상 경계를
// 제거하지 않고 탭으로 재배선한다. attach 된 브리지가 없으면 입력 채널 생산자 부재 + 출력
// 0-subscriber 드롭이므로 동작이 동일하다(외부 카운터파트 없음 = 무출력).
func installSharedBoundaryTaps(f flow.Flow, flowID string) (flow.Flow, *sharedBoundaryController) {
	if !flowHasBoundaryWire(f) {
		return f, nil
	}

	ctrl := newSharedBoundaryController(flowID)
	tapped, inPorts, outPorts := rewireBoundariesToSharedTap(f, flowID)
	ctrl.inputPorts = inPorts
	ctrl.outputPorts = outPorts
	globalSharedBoundaryTable.register(flowID, ctrl)
	return tapped, ctrl
}

// flowHasBoundaryWire 는 플로우에 경계(센티넬) 와이어가 하나라도 있는지 반환한다.
func flowHasBoundaryWire(f flow.Flow) bool {
	for _, w := range f.Wires() {
		if flow.IsBoundaryWire(w) {
			return true
		}
	}
	return false
}

// rewireBoundariesToSharedTap 는 경계(센티넬) 와이어를 공유 경계 탭 노드로 재배선한 새 플로우와
// 입출력 경계 포트 이름 목록을 반환한다(bridge_runner.go 의 rewireBoundariesToTap 와 동일 구조,
// flow_id 기반 공유 탭 노드로 향함). 비-경계 와이어/노드는 보존한다.
func rewireBoundariesToSharedTap(f flow.Flow, flowID string) (tappedFlow flow.Flow, inputPorts, outputPorts []string) {
	srcNodes := f.Nodes()
	srcWires := f.Wires()

	inSet := make(map[string]struct{})
	outSet := make(map[string]struct{})
	resultWires := make([]flow.Wire, 0, len(srcWires))

	for _, w := range srcWires {
		switch {
		case w.SourceNodeID == flow.FlowInputBoundaryID && w.TargetNodeID == flow.FlowOutputBoundaryID:
			// 입력 포트 → 출력 포트 직결(passthrough): 입력 탭 → 출력 탭 으로 재배선.
			inSet[w.SourcePort] = struct{}{}
			outSet[w.TargetPort] = struct{}{}
			resultWires = append(resultWires, retargetWire(w, sharedBoundaryInputTapNodeID, w.SourcePort, sharedBoundaryOutputTapNodeID, w.TargetPort))
		case w.SourceNodeID == flow.FlowInputBoundaryID:
			// 입력 경계: 입력 탭(SourcePort) → 내부 소비자.
			inSet[w.SourcePort] = struct{}{}
			resultWires = append(resultWires, retargetWire(w, sharedBoundaryInputTapNodeID, w.SourcePort, w.TargetNodeID, w.TargetPort))
		case w.TargetNodeID == flow.FlowOutputBoundaryID:
			// 출력 경계: 내부 생산자 → 출력 탭(TargetPort).
			outSet[w.TargetPort] = struct{}{}
			resultWires = append(resultWires, retargetWire(w, w.SourceNodeID, w.SourcePort, sharedBoundaryOutputTapNodeID, w.TargetPort))
		default:
			resultWires = append(resultWires, w)
		}
	}

	inputPorts = setToSortedSlice(inSet)
	outputPorts = setToSortedSlice(outSet)

	resultNodes := make([]flow.NodeDef, 0, len(srcNodes)+2)
	resultNodes = append(resultNodes, srcNodes...)
	if len(inputPorts) > 0 {
		resultNodes = append(resultNodes, sharedBoundaryNodeDef(sharedBoundaryInputTapNodeID, sharedBoundaryInputTapType, flowID, nil, inputPorts))
	}
	if len(outputPorts) > 0 {
		resultNodes = append(resultNodes, sharedBoundaryNodeDef(sharedBoundaryOutputTapNodeID, sharedBoundaryOutputTapType, flowID, outputPorts, nil))
	}

	return flow.RebuildFlow(f, resultNodes, resultWires), inputPorts, outputPorts
}

// sharedBoundaryNodeDef 는 공유 경계 탭 노드 정의를 만든다. Config 에 flow_id 를 실어 노드
// Init 이 컨트롤러를 찾게 한다.
func sharedBoundaryNodeDef(id, typ, flowID string, inputs, outputs []string) flow.NodeDef {
	nd := flow.NodeDef{
		ID:     id,
		Name:   id,
		Type:   typ,
		Config: map[string]any{sharedBoundaryIDConfigKey: flowID},
	}
	for _, p := range inputs {
		nd.Inputs = append(nd.Inputs, flow.Port{ID: p, Name: p, Direction: flow.PortInput})
	}
	for _, p := range outputs {
		nd.Outputs = append(nd.Outputs, flow.Port{ID: p, Name: p, Direction: flow.PortOutput})
	}
	return nd
}

// ---------------------------------------------------------------------------
// 공유 경계 탭 노드 타입 (입력 탭 = MultiSourceNode, 출력 탭 = ProcessNode)
// ---------------------------------------------------------------------------

// RegisterSharedBoundaryNodes 는 공유 경계 입력/출력 탭 노드 타입을 레지스트리에 등록한다.
// shared 모드를 지원하는 엔진 구성 시 1회 호출한다. 이미 등록되어 있으면 건너뛴다(멱등).
func RegisterSharedBoundaryNodes(reg *node.Registry) error {
	if reg == nil {
		return nil
	}
	if !reg.Has(sharedBoundaryInputTapType) {
		if err := reg.Register(sharedBoundaryInputTapType, newSharedBoundaryInputTapNode); err != nil {
			return err
		}
	}
	if !reg.Has(sharedBoundaryOutputTapType) {
		if err := reg.Register(sharedBoundaryOutputTapType, newSharedBoundaryOutputTapNode); err != nil {
			return err
		}
	}
	return nil
}

// sharedBoundaryInputTapNode 는 입력 경계 포트별 source 채널을 노출하는 MultiSourceNode 이다.
// 컨트롤러의 inputChans[port] 를 출력 포트 채널로 노출하여, inject(port,data) 가 흘린 메시지를
// 엔진이 해당 출력 와이어(원래 __flow_input__→consumer 재배선)로 전달한다.
type sharedBoundaryInputTapNode struct {
	*node.BaseNode
	ctrl  *sharedBoundaryController
	ports []string
}

func newSharedBoundaryInputTapNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	n := &sharedBoundaryInputTapNode{BaseNode: node.NewBaseNode(def, opts...)}
	flowID, _ := def.Config[sharedBoundaryIDConfigKey].(string)
	if ctrl, ok := globalSharedBoundaryTable.lookup(flowID); ok {
		n.ctrl = ctrl
	}
	for _, p := range def.Outputs {
		n.ports = append(n.ports, p.Name)
	}
	return n, nil
}

func (n *sharedBoundaryInputTapNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if n.ctrl != nil {
		for _, p := range n.ports {
			n.ctrl.ensureInputChan(p)
		}
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

func (n *sharedBoundaryInputTapNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Process 는 사용되지 않는다(SourceNode 이므로). 방어적으로 메시지를 그대로 통과시킨다.
func (n *sharedBoundaryInputTapNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// SourceCh 는 첫 입력 경계 포트 채널을 반환한다(SourceNode 인터페이스). 포트가 없으면 닫힌
// 빈 채널을 반환한다. 다중 포트는 ExtraSourceChannels 로 노출한다.
func (n *sharedBoundaryInputTapNode) SourceCh() <-chan message.Message {
	if n.ctrl == nil || len(n.ports) == 0 {
		ch := make(chan message.Message)
		close(ch)
		return ch
	}
	return n.ctrl.ensureInputChan(n.ports[0])
}

// ExtraSourceChannels 는 첫 포트를 제외한 추가 입력 경계 포트 채널을 반환한다.
func (n *sharedBoundaryInputTapNode) ExtraSourceChannels() map[string]<-chan message.Message {
	out := make(map[string]<-chan message.Message)
	if n.ctrl == nil {
		return out
	}
	for i, p := range n.ports {
		if i == 0 {
			continue
		}
		out[p] = n.ctrl.ensureInputChan(p)
	}
	return out
}

// sharedBoundaryOutputTapNode 는 출력 경계 포트별 입력 포트로 수신한 메시지를 컨트롤러의
// emitOutput 으로 전달하는 ProcessNode 이다(bridge_tap_nodes.go 의 출력 탭과 동일 패턴).
type sharedBoundaryOutputTapNode struct {
	*node.BaseNode
	ctrl  *sharedBoundaryController
	ports []string
}

func newSharedBoundaryOutputTapNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	n := &sharedBoundaryOutputTapNode{BaseNode: node.NewBaseNode(def, opts...)}
	flowID, _ := def.Config[sharedBoundaryIDConfigKey].(string)
	if ctrl, ok := globalSharedBoundaryTable.lookup(flowID); ok {
		n.ctrl = ctrl
	}
	for _, p := range def.Inputs {
		n.ports = append(n.ports, p.Name)
	}
	return n, nil
}

func (n *sharedBoundaryOutputTapNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

func (n *sharedBoundaryOutputTapNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Process 는 수신 메시지를 출력 경계 포트로 emit 한다. 단일 포트면 그 포트, 다중이면 메시지
// 메타데이터의 도착 포트(없으면 첫 포트)로 판별한다(bridge_tap_nodes.go 와 동일 폴백 규칙).
func (n *sharedBoundaryOutputTapNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.ctrl == nil {
		return nil, nil
	}
	port := n.resolveOutputPort(msg)
	n.ctrl.emitOutput(port, messageToJSON(msg))
	return nil, nil
}

func (n *sharedBoundaryOutputTapNode) resolveOutputPort(msg message.Message) string {
	if len(n.ports) == 1 {
		return n.ports[0]
	}
	if msg != nil {
		if v, ok := msg.Metadata().Get(bridgeTapPortMetaKey); ok && v != "" {
			return v
		}
	}
	if len(n.ports) > 0 {
		return n.ports[0]
	}
	return "out"
}

var (
	_ node.Node            = (*sharedBoundaryInputTapNode)(nil)
	_ node.MultiSourceNode = (*sharedBoundaryInputTapNode)(nil)
	_ node.Node            = (*sharedBoundaryOutputTapNode)(nil)
)
