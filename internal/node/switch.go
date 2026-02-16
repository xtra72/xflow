package node

import (
	"context"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// SwitchRoute 는 스위치 노드의 라우팅 규칙을 정의하는 구조체이다.
// Condition이 true를 반환하면 메시지가 TargetPort로 전달된다.
type SwitchRoute struct {
	Condition  func(msg message.Message) bool
	TargetPort string
}

// SwitchNode 는 조건에 따라 메시지를 다른 포트로 라우팅하는 노드이다.
// 라우트를 순서대로 평가하여 첫 번째 매칭되는 라우트의 TargetPort로 전달한다.
type SwitchNode struct {
	*BaseNode
	routes      []SwitchRoute
	defaultPort string
	mu          sync.RWMutex
}

// NewSwitchNode 는 새로운 SwitchNode를 생성하는 팩토리 함수이다.
func NewSwitchNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &SwitchNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 SwitchNode를 초기화한다.
func (n *SwitchNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 라우트를 순서대로 평가하여 메시지를 라우팅한다.
// 매칭되는 라우트가 있으면 메시지 메타데이터에 "_target_port"를 설정하여 반환한다.
// 매칭되는 라우트가 없고 defaultPort가 설정되어 있으면 기본 포트로 라우팅한다.
// 매칭도 없고 기본 포트도 없으면 빈 슬라이스를 반환한다 (드롭).
func (n *SwitchNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	routes := n.routes
	defPort := n.defaultPort
	n.mu.RUnlock()

	for _, route := range routes {
		if route.Condition(msg) {
			out := msg.Clone()
			out.Metadata().Set("_target_port", route.TargetPort)
			return []message.Message{out}, nil
		}
	}

	if defPort != "" {
		out := msg.Clone()
		out.Metadata().Set("_target_port", defPort)
		return []message.Message{out}, nil
	}

	return []message.Message{}, nil
}

// Shutdown 은 SwitchNode를 종료한다.
func (n *SwitchNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 SwitchNode의 설정을 적용한다.
// config에 "routes" 키가 있고 []SwitchRoute 타입이면 라우트를 설정한다.
// config에 "default_port" 키가 있고 string 타입이면 기본 포트를 설정한다.
func (n *SwitchNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()

	if routes, ok := config["routes"]; ok {
		if r, ok := routes.([]SwitchRoute); ok {
			n.routes = r
		}
	}
	if dp, ok := config["default_port"]; ok {
		if s, ok := dp.(string); ok {
			n.defaultPort = s
		}
	}
	return nil
}
