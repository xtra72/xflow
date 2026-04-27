package node

import (
	"context"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// StatusNode 는 노드 라이프사이클 상태 변경을 모니터링하는 노드이다.
// watchNodes가 설정되어 있으면 해당 노드 ID의 이벤트만 처리하고,
// 비어있으면 모든 이벤트를 통과시킨다.
type StatusNode struct {
	*BaseNode
	watchNodes []string // 감시 대상 노드 ID (빈 배열 = 모든 노드)
	mu         sync.RWMutex
}

// NewStatusNode 는 새로운 StatusNode를 생성하는 팩토리 함수이다.
func NewStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &StatusNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 StatusNode를 초기화한다.
func (n *StatusNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 상태 변경 이벤트 메시지를 처리한다.
// watchNodes가 설정되어 있으면 해당 노드 ID의 이벤트만 통과시키고,
// 나머지는 빈 슬라이스를 반환하여 필터링한다.
func (n *StatusNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	watchNodes := n.watchNodes
	n.mu.RUnlock()

	// watchNodes가 비어있으면 모든 이벤트 통과
	if len(watchNodes) == 0 {
		return []message.Message{msg}, nil
	}

	// 메타데이터에서 _node_id 추출
	nodeID, _ := msg.Metadata().Get("_node_id")

	// watchNodes에 포함된 노드 ID인지 확인
	for _, wn := range watchNodes {
		if wn == nodeID {
			return []message.Message{msg}, nil
		}
	}

	// 감시 대상이 아니면 필터링
	return []message.Message{}, nil
}

// Shutdown 은 StatusNode를 종료한다.
func (n *StatusNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 StatusNode의 설정을 적용한다.
// config에 "watch_nodes" 키가 있고 []string 타입이면 감시 대상 노드를 설정한다.
func (n *StatusNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	if wn, ok := config["watch_nodes"]; ok {
		if nodes, ok := wn.([]string); ok {
			n.mu.Lock()
			n.watchNodes = nodes
			n.mu.Unlock()
		}
	}
	return nil
}
