package node

import (
	"context"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// CatchNode 는 에러 메시지를 처리하는 노드이다.
// 메시지 메타데이터에서 에러 정보(_error_type, _error_message, _source_node_id)를 추출하여 처리한다.
// catchTypes가 설정되어 있으면 해당 타입의 에러만 처리하고, 나머지는 그대로 통과시킨다.
type CatchNode struct {
	*BaseNode
	catchTypes []string // 처리 대상 에러 타입 (빈 배열 = 모든 에러)
	mu         sync.RWMutex
}

// NewCatchNode 는 새로운 CatchNode를 생성하는 팩토리 함수이다.
func NewCatchNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &CatchNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 CatchNode를 초기화한다.
func (n *CatchNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 메시지의 에러 정보를 확인하고 처리한다.
// catchTypes가 설정되어 있으면 에러 타입이 목록에 있는 경우만 처리한다.
// 에러 타입이 목록에 없으면 메시지를 그대로 통과시킨다.
func (n *CatchNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	catchTypes := n.catchTypes
	n.mu.RUnlock()

	// 에러 타입을 메타데이터에서 추출
	errorType, hasErrorType := msg.Metadata().Get("_error_type")

	// catchTypes가 설정되어 있고 에러 타입이 목록에 없으면 통과
	if len(catchTypes) > 0 && hasErrorType {
		found := false
		for _, ct := range catchTypes {
			if ct == errorType {
				found = true
				break
			}
		}
		if !found {
			return []message.Message{msg}, nil
		}
	}

	// 에러 메시지를 처리하여 반환
	return []message.Message{msg}, nil
}

// Shutdown 은 CatchNode를 종료한다.
func (n *CatchNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 CatchNode의 설정을 적용한다.
// config에 "catch_types" 키가 있고 []string 타입이면 처리 대상 에러 타입을 설정한다.
func (n *CatchNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	if ct, ok := config["catch_types"]; ok {
		if types, ok := ct.([]string); ok {
			n.mu.Lock()
			n.catchTypes = types
			n.mu.Unlock()
		}
	}
	return nil
}
