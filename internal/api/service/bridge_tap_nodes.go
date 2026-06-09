// bridge_tap_nodes.go 는 라이브 브리지 경계 tap 노드 타입(입력 tap/출력 tap)과 등록
// 헬퍼를 구현한다(@SPEC:SPEC-SUBFLOW-001 그룹 RB, P2, REQ-SUBFLOW-RB07).
//
// 이 두 노드 타입은 client-mode 엔진 레지스트리에만 등록되며(RegisterBridgeTapNodes),
// 브리지 배포 시 경계 와이어 재배선으로만 인스턴스화된다(bridge_runner.go). 엔진은 이를
// 일반 노드로 취급하므로 엔진 변경이 없다(plan §1.3). 두 노드는 NodeDef.Config 의 tapID
// 로 process-global bridgeTapTable 에서 per-flow 컨트롤러를 찾아 채널/콜백을 결선한다.
package service

import (
	"context"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// RegisterBridgeTapNodes 는 입력/출력 tap 노드 타입을 레지스트리에 등록한다. client-mode
// 엔진 구성 시 1회 호출한다. 이미 등록되어 있으면 오류를 무시한다(멱등).
func RegisterBridgeTapNodes(reg *node.Registry) error {
	if reg == nil {
		return nil
	}
	if !reg.Has(bridgeInputTapType) {
		if err := reg.Register(bridgeInputTapType, newBridgeInputTapNode); err != nil {
			return err
		}
	}
	if !reg.Has(bridgeOutputTapType) {
		if err := reg.Register(bridgeOutputTapType, newBridgeOutputTapNode); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 입력 tap 노드 (MultiSourceNode) — bridge_input 주입 → 입력 경계 포트
// ---------------------------------------------------------------------------

// bridgeInputTapNode 는 입력 경계 포트별 source 채널을 노출하는 MultiSourceNode 이다.
// 컨트롤러의 inputChans[port] 를 그대로 출력 포트 채널로 노출하여, Inject(port,data)가
// 흘린 메시지를 엔진이 해당 출력 와이어(원래 __flow_input__→consumer 재배선)로 전달한다.
type bridgeInputTapNode struct {
	*node.BaseNode
	ctrl  *bridgeTapController
	ports []string
}

// newBridgeInputTapNode 는 입력 tap 노드를 생성한다. NodeDef.Config 의 tapID 로 컨트롤러를
// 찾고, 선언된 출력 포트(입력 경계 포트 이름)별 채널을 컨트롤러에서 보장 생성한다.
func newBridgeInputTapNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	n := &bridgeInputTapNode{BaseNode: node.NewBaseNode(def, opts...)}
	tapID, _ := def.Config[bridgeTapIDConfigKey].(string)
	if ctrl, ok := bridgeTapTable.lookup(tapID); ok {
		n.ctrl = ctrl
	}
	for _, p := range def.Outputs {
		n.ports = append(n.ports, p.Name)
	}
	return n, nil
}

// Init 는 source 채널을 컨트롤러에서 보장 생성하고 노드를 running 으로 전이한다.
func (n *bridgeInputTapNode) Init(_ context.Context) error {
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

// Shutdown 은 노드를 정지 전이한다(채널 close 는 컨트롤러 shutdown 이 담당).
func (n *bridgeInputTapNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Process 는 사용되지 않는다(SourceNode 이므로). 방어적으로 메시지를 그대로 통과시킨다.
func (n *bridgeInputTapNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// SourceCh 는 기본("out" 의미) 채널을 반환한다(SourceNode 인터페이스). 입력 경계 포트가
// 1개면 그 채널, 없으면 닫힌 빈 채널을 반환한다. 다중 포트는 ExtraSourceChannels 로 노출.
func (n *bridgeInputTapNode) SourceCh() <-chan message.Message {
	if n.ctrl == nil || len(n.ports) == 0 {
		ch := make(chan message.Message)
		close(ch)
		return ch
	}
	return n.ctrl.ensureInputChan(n.ports[0])
}

// ExtraSourceChannels 는 첫 포트를 제외한 추가 입력 경계 포트 채널을 반환한다.
// 엔진은 포트별로 출력 와이어를 그룹핑해 라우팅한다.
func (n *bridgeInputTapNode) ExtraSourceChannels() map[string]<-chan message.Message {
	out := make(map[string]<-chan message.Message)
	if n.ctrl == nil {
		return out
	}
	for i, p := range n.ports {
		if i == 0 {
			continue // SourceCh 가 첫 포트를 담당.
		}
		out[p] = n.ctrl.ensureInputChan(p)
	}
	return out
}

var (
	_ node.Node            = (*bridgeInputTapNode)(nil)
	_ node.MultiSourceNode = (*bridgeInputTapNode)(nil)
)

// ---------------------------------------------------------------------------
// 출력 tap 노드 (ProcessNode) — 출력 경계 포트 → onOutput 콜백
// ---------------------------------------------------------------------------

// bridgeOutputTapNode 는 출력 경계 포트별 입력 포트로 수신한 메시지를 컨트롤러의
// emitOutput 으로 전달하는 ProcessNode 이다. 수신 포트 이름은 엔진의 와이어 매핑이
// TargetPort 로 결정하므로, Process 에서는 메시지 메타데이터의 도착 포트를 신뢰하지 않고
// 단일 입력 포트면 그 이름을, 다중이면 메시지 라우팅 메타로 판별한다.
//
// 엔진의 mergeInputWires 는 여러 입력 와이어를 단일 채널로 머지하므로 Process 는 어느
// 포트로 도착했는지 직접 알 수 없다. 이를 해결하기 위해, 출력 경계 포트가 1개인 일반적
// 경우는 그 포트 이름을 사용하고, 다중 포트는 각 포트를 별도 출력 tap 노드 인스턴스가
// 아니라 본 노드가 보유한 ports 중 메시지 메타의 source port 를 사용한다(폴백: 첫 포트).
type bridgeOutputTapNode struct {
	*node.BaseNode
	ctrl  *bridgeTapController
	ports []string
}

// newBridgeOutputTapNode 는 출력 tap 노드를 생성한다. NodeDef.Config 의 tapID 로 컨트롤러를
// 찾고, 선언된 입력 포트(출력 경계 포트 이름)를 보관한다.
func newBridgeOutputTapNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	n := &bridgeOutputTapNode{BaseNode: node.NewBaseNode(def, opts...)}
	tapID, _ := def.Config[bridgeTapIDConfigKey].(string)
	if ctrl, ok := bridgeTapTable.lookup(tapID); ok {
		n.ctrl = ctrl
	}
	for _, p := range def.Inputs {
		n.ports = append(n.ports, p.Name)
	}
	return n, nil
}

// Init 는 노드를 running 으로 전이한다.
func (n *bridgeOutputTapNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 노드를 정지 전이한다.
func (n *bridgeOutputTapNode) Shutdown(_ context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Process 는 수신 메시지를 출력 경계 포트로 emit 한다(REQ-SUBFLOW-RB07). 메시지가 어느
// 출력 경계 포트로 도착했는지는 메시지 메타데이터의 라우팅 포트 키로 판별하며, 없으면
// 단일 포트 케이스로 첫 포트를 사용한다. emit 후 메시지를 소비한다(하류 와이어 없음).
func (n *bridgeOutputTapNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.ctrl == nil {
		return nil, nil
	}
	port := n.resolveOutputPort(msg)
	n.ctrl.emitOutput(port, messageToJSON(msg))
	return nil, nil
}

// resolveOutputPort 는 메시지가 도착한 출력 경계 포트 이름을 판별한다. 메시지 메타데이터의
// 도착 포트 키(message.MetaKeyTargetPort 류)가 있으면 사용하고, 없으면 단일 포트면 그
// 포트, 다중이면 첫 포트로 폴백한다.
func (n *bridgeOutputTapNode) resolveOutputPort(msg message.Message) string {
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

var _ node.Node = (*bridgeOutputTapNode)(nil)

// bridgeTapPortMetaKey 는 다중 출력 경계 포트 판별용 메타데이터 키이다(폴백 — 단일 포트는
// 불필요). 엔진이 TargetPort 를 메타로 싣지 않으므로, 일반적 단일 출력 포트 케이스에
// 의존하고 다중 포트는 본 키가 있으면 사용한다.
const bridgeTapPortMetaKey = "__bridge_target_port__"
