package node

import (
	"context"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// TransformFunc 는 메시지를 변환하는 함수 타입이다.
// 변환 실패 시 에러를 반환한다.
type TransformFunc func(msg message.Message) (message.Message, error)

// TransformNode 는 메시지를 변환하는 노드이다.
// 변환 함수가 nil이면 모든 메시지를 그대로 통과시킨다 (pass-through).
type TransformNode struct {
	*BaseNode
	transformFn TransformFunc
	mu          sync.RWMutex
}

// NewTransformNode 는 새로운 TransformNode를 생성하는 팩토리 함수이다.
func NewTransformNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &TransformNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 TransformNode를 초기화한다.
func (n *TransformNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 변환 함수를 적용하여 메시지를 변환한다.
// 변환 함수가 nil이면 메시지를 그대로 통과시킨다.
// 변환 함수가 에러를 반환하면 nil과 에러를 반환한다 (호출자가 에러 포트 처리).
func (n *TransformNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	fn := n.transformFn
	n.mu.RUnlock()

	if fn == nil {
		return []message.Message{msg}, nil
	}

	result, err := fn(msg)
	if err != nil {
		return nil, err
	}
	return []message.Message{result}, nil
}

// Shutdown 은 TransformNode를 종료한다.
func (n *TransformNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 TransformNode의 설정을 적용한다.
// config에 "transform" 키가 있고 TransformFunc 타입이면 변환 함수를 설정한다.
func (n *TransformNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	if tf, ok := config["transform"]; ok {
		if fn, ok := tf.(TransformFunc); ok {
			n.mu.Lock()
			n.transformFn = fn
			n.mu.Unlock()
		}
	}
	return nil
}
