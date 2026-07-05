// remote_bridge_tap_nodes.go 는 매니저 측 라이브 브리지 엔드포인트 노드 타입(입력 forwarder/
// 출력 emitter)과 등록 헬퍼를 구현한다(@SPEC:SPEC-SUBFLOW-001 그룹 RB, P3, REQ-SUBFLOW-RB07).
//
// 이 두 노드 타입은 server-mode 엔진 레지스트리에만 등록되며(RegisterRemoteBridgeNodes),
// 살아남은 remote:// flow-node 재배선(remote_bridge_node.go)으로만 인스턴스화된다. 엔진은
// 이를 일반 노드로 취급하므로 엔진 변경이 없다(bridge_tap_nodes.go 와 동일 정신, 단 방향이
// 반대 — 매니저 측은 입력=ProcessNode, 출력=MultiSourceNode).
//
//   - 입력 forwarder(ProcessNode): flow-node INPUT 포트 와이어를 받아 Process 에서
//     controller.forwardInput(port,data) → bridge.SendInput(원격 주입 — WRITE). Init 에서
//     controller.start(OpenBridge), Shutdown 에서 controller.stop(bridge.Close)을 구동한다
//     (노드 라이프사이클 = 브리지 라이프사이클 — RB05/RB09).
//   - 출력 emitter(MultiSourceNode): controller.onOutput 이 포트별 채널로 흘린 메시지를
//     엔진이 OUTPUT 와이어로 라우팅한다(원격 출력 중계 — READ).
package service

import (
	"context"
	"encoding/json"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// RegisterRemoteBridgeNodes 는 입력 forwarder/출력 emitter 노드 타입을 레지스트리에 등록한다.
// server-mode 엔진 구성 시 1회 호출한다. 이미 등록되어 있으면 무시한다(멱등).
func RegisterRemoteBridgeNodes(reg *node.Registry) error {
	if reg == nil {
		return nil
	}
	if !reg.Has(bridgeFwdType) {
		if err := reg.Register(bridgeFwdType, newBridgeFwdNode); err != nil {
			return err
		}
	}
	if !reg.Has(bridgeEmitType) {
		if err := reg.Register(bridgeEmitType, newBridgeEmitNode); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 입력 forwarder (ProcessNode) — flow-node INPUT 포트 → bridge.SendInput
// ---------------------------------------------------------------------------

// bridgeFwdNode 는 flow-node 입력 포트로 수신한 메시지를 controller.forwardInput 으로 원격
// 주입하는 ProcessNode 이다. Init 에서 브리지를 열고(start), Shutdown 에서 닫는다(stop).
type bridgeFwdNode struct {
	*node.BaseNode
	ctrl  *managerBridgeController
	ports []string
}

// newBridgeFwdNode 는 입력 forwarder 노드를 생성한다. NodeDef.Config 의 bridgeNodeID 로
// 컨트롤러를 찾고, 선언된 입력 포트(flow-node 입력 경계 포트 이름)를 보관한다.
func newBridgeFwdNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	n := &bridgeFwdNode{BaseNode: node.NewBaseNode(def, opts...)}
	bridgeID, _ := def.Config[bridgeNodeIDConfigKey].(string)
	if ctrl, ok := globalManagerBridgeTable.lookup(bridgeID); ok {
		n.ctrl = ctrl
	}
	for _, p := range def.Inputs {
		n.ports = append(n.ports, p.Name)
	}
	return n, nil
}

// Init 은 컨트롤러를 통해 라이브 브리지를 열고(OpenBridge — RB05) 노드를 running 으로 전이한다.
// 브리지 open 실패 시 노드 Init 실패로 배포를 거부한다(명확한 오류 — 게이팅/오프라인/노드 오류).
func (n *bridgeFwdNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if n.ctrl != nil {
		if err := n.ctrl.start(ctx); err != nil {
			return err
		}
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 수신 메시지를 controller.forwardInput 으로 원격 입력 경계 포트에 주입한다
// (WRITE — RB07). 입력 포트가 1개인 일반적 경우는 그 포트 이름을 사용한다(이름 기반 매핑 —
// RB06). 메시지는 소비되며(하류 와이어 없음), 빈 결과를 반환한다.
func (n *bridgeFwdNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	if n.ctrl == nil {
		return nil, nil
	}
	port := n.resolveInputPort(msg)
	_ = n.ctrl.forwardInput(port, messageToJSON(msg))
	return nil, nil
}

// resolveInputPort 는 메시지가 도착한 입력 경계 포트 이름을 판별한다. 단일 포트면 그 포트,
// 다중이면 메시지 메타의 도착 포트 키(있으면)로 판별하고 없으면 첫 포트로 폴백한다(엔진이
// 입력 와이어를 단일 채널로 머지하므로 — bridge_tap_nodes 의 출력 포트 판별과 동일 한계).
func (n *bridgeFwdNode) resolveInputPort(msg message.Message) string {
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
	return "in"
}

// Shutdown 은 컨트롤러를 통해 브리지를 닫고(bridge.Close — RB09) 노드를 정지 전이한다.
func (n *bridgeFwdNode) Shutdown(ctx context.Context) error {
	if n.ctrl != nil {
		n.ctrl.stop(ctx)
	}
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

var _ node.Node = (*bridgeFwdNode)(nil)

// ---------------------------------------------------------------------------
// 출력 emitter (MultiSourceNode) — bridge.Outputs → flow-node OUTPUT 포트
// ---------------------------------------------------------------------------

// bridgeEmitNode 는 출력 경계 포트별 채널을 노출하는 MultiSourceNode 이다. controller.onOutput
// 이 포트별 채널로 메시지를 흘리면 엔진이 해당 출력 와이어로 전달한다(READ — RB07).
type bridgeEmitNode struct {
	*node.BaseNode
	ctrl  *managerBridgeController
	ports []string
	chans map[string]chan message.Message
}

// newBridgeEmitNode 는 출력 emitter 노드를 생성한다. NodeDef.Config 의 bridgeNodeID 로
// 컨트롤러를 찾고, 선언된 출력 포트별 채널을 만들어 controller.onOutput 에 결선한다.
func newBridgeEmitNode(def flow.NodeDef, opts ...node.NodeOption) (node.Node, error) {
	n := &bridgeEmitNode{
		BaseNode: node.NewBaseNode(def, opts...),
		chans:    make(map[string]chan message.Message),
	}
	bridgeID, _ := def.Config[bridgeNodeIDConfigKey].(string)
	if ctrl, ok := globalManagerBridgeTable.lookup(bridgeID); ok {
		n.ctrl = ctrl
	}
	for _, p := range def.Outputs {
		n.ports = append(n.ports, p.Name)
		n.chans[p.Name] = make(chan message.Message, 64)
	}
	return n, nil
}

// Init 은 controller.onOutput 을 포트별 채널 흘림 콜백으로 결선하고 노드를 running 으로 전이한다.
func (n *bridgeEmitNode) Init(_ context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	if n.ctrl != nil {
		n.ctrl.setOnOutput(func(port string, data json.RawMessage) {
			ch, ok := n.chans[port]
			if !ok {
				return // 미지의 출력 포트 — 폐기(노드 선언 포트 외).
			}
			msg, err := messageFromJSON(data)
			if err != nil {
				return
			}
			// 비블로킹 송신: 소비가 느리면 oldest-drop(엔진/브리지 비블로킹 일관).
			select {
			case ch <- msg:
			default:
				select {
				case <-ch:
				default:
				}
				select {
				case ch <- msg:
				default:
				}
			}
		})
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Shutdown 은 onOutput 결선을 해제하고 노드를 정지 전이한다(채널 close 는 컨트롤러 stop 이
// 펌프를 멈추므로 별도 close 불필요 — 엔진 ctx 취소가 source 고루틴을 종료).
func (n *bridgeEmitNode) Shutdown(_ context.Context) error {
	if n.ctrl != nil {
		n.ctrl.setOnOutput(nil)
	}
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Process 는 사용되지 않는다(SourceNode 이므로). 방어적으로 메시지를 통과시킨다.
func (n *bridgeEmitNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	return []message.Message{msg}, nil
}

// SourceCh 는 기본("out" 의미) 채널을 반환한다. 출력 포트가 1개면 그 채널, 없으면 닫힌 빈
// 채널을 반환한다. 다중 포트는 ExtraSourceChannels 로 노출한다.
func (n *bridgeEmitNode) SourceCh() <-chan message.Message {
	if len(n.ports) == 0 {
		ch := make(chan message.Message)
		close(ch)
		return ch
	}
	return n.chans[n.ports[0]]
}

// ExtraSourceChannels 는 첫 포트를 제외한 추가 출력 경계 포트 채널을 반환한다.
func (n *bridgeEmitNode) ExtraSourceChannels() map[string]<-chan message.Message {
	out := make(map[string]<-chan message.Message)
	for i, p := range n.ports {
		if i == 0 {
			continue // SourceCh 가 첫 포트를 담당.
		}
		out[p] = n.chans[p]
	}
	return out
}

var (
	_ node.Node            = (*bridgeEmitNode)(nil)
	_ node.MultiSourceNode = (*bridgeEmitNode)(nil)
)
