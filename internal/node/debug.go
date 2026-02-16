package node

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// DebugNode 는 메시지 내용을 로깅하는 디버그 노드이다.
// 메시지를 로그에 기록한 뒤 그대로 통과시킨다 (pass-through).
type DebugNode struct {
	*BaseNode
	logLevel string // "debug", "info", "warn"
	mu       sync.RWMutex
}

// NewDebugNode 는 새로운 DebugNode를 생성하는 팩토리 함수이다.
func NewDebugNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &DebugNode{
		BaseNode: base,
		logLevel: "debug", // 기본 레벨
	}
	return n, nil
}

// Init 은 DebugNode를 초기화한다.
func (n *DebugNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 메시지 내용을 로깅한 뒤 그대로 통과시킨다.
// 로거가 nil이면 로깅을 건너뛰고 메시지만 통과시킨다.
func (n *DebugNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	level := n.logLevel
	n.mu.RUnlock()

	logger := n.BaseNode.Logger()
	if logger != nil {
		logMsg := fmt.Sprintf("message id=%s payload=%v metadata=%v",
			msg.ID(), msg.Payload().ToMap(), msg.Metadata().All())

		switch level {
		case "info":
			logger.Info(logMsg)
		case "warn":
			logger.Warn(logMsg)
		default:
			logger.Debug(logMsg)
		}
	}

	return []message.Message{msg}, nil
}

// Shutdown 은 DebugNode를 종료한다.
func (n *DebugNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 DebugNode의 설정을 적용한다.
// config에 "level" 키가 있고 string 타입이면 로그 레벨을 설정한다.
func (n *DebugNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	if lvl, ok := config["level"]; ok {
		if levelStr, ok := lvl.(string); ok {
			n.mu.Lock()
			n.logLevel = levelStr
			n.mu.Unlock()
		}
	}
	return nil
}
