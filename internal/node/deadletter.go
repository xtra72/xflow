package node

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// DeadLetterStrategy 는 데드레터 처리 전략을 나타내는 문자열 타입이다.
type DeadLetterStrategy string

const (
	// DeadLetterLog 는 데드레터를 로그로 기록하는 전략이다.
	DeadLetterLog DeadLetterStrategy = "log"
	// DeadLetterStore 는 데드레터를 저장하는 전략이다.
	DeadLetterStore DeadLetterStrategy = "store"
	// DeadLetterForward 는 데드레터를 전달하는 전략이다.
	DeadLetterForward DeadLetterStrategy = "forward"
)

// DeadLetterNode 는 TTL 만료, 전달 불가, 최대 재시도 초과 메시지를 처리하는 노드이다.
// 전략에 따라 로그 기록, 저장, 또는 전달 처리를 수행한다.
type DeadLetterNode struct {
	*BaseNode
	strategy DeadLetterStrategy
	mu       sync.RWMutex
}

// NewDeadLetterNode 는 새로운 DeadLetterNode를 생성하는 팩토리 함수이다.
func NewDeadLetterNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &DeadLetterNode{
		BaseNode: base,
		strategy: DeadLetterLog, // 기본 전략
	}
	return n, nil
}

// Init 은 DeadLetterNode를 초기화한다.
func (n *DeadLetterNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 데드레터 메시지를 처리한다.
// _deadletter_reason 메타데이터를 확인하고, _processed_at 타임스탬프를 추가한 뒤,
// 전략에 따른 처리를 수행하고 메시지를 통과시킨다.
func (n *DeadLetterNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	strategy := n.strategy
	n.mu.RUnlock()

	// _deadletter_reason 확인
	reason, hasReason := msg.Metadata().Get("_deadletter_reason")

	// _processed_at 타임스탬프 추가
	if hasReason {
		msg.Metadata().Set("_processed_at", time.Now().UTC().Format(time.RFC3339))
	}

	// 메트릭 기록
	if hasReason && n.BaseNode.MetricsCollector() != nil {
		metricName := fmt.Sprintf("deadletter_%s_total", reason)
		n.BaseNode.MetricsCollector().Counter(metricName, "deadletter").Inc()
	}

	// 전략별 처리
	if hasReason {
		switch strategy {
		case DeadLetterLog:
			if n.BaseNode.Logger() != nil {
				n.BaseNode.Logger().Info("dead letter received",
					"reason", reason,
					"message_id", msg.ID(),
				)
			}
		case DeadLetterStore, DeadLetterForward:
			msg.Metadata().Set("_deadletter_strategy", string(strategy))
		}
	}

	return []message.Message{msg}, nil
}

// Shutdown 은 DeadLetterNode를 종료한다.
func (n *DeadLetterNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 DeadLetterNode의 설정을 적용한다.
// config에 "strategy" 키가 있고 string 타입이면 처리 전략을 설정한다.
func (n *DeadLetterNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}
	if s, ok := config["strategy"]; ok {
		if strategyStr, ok := s.(string); ok {
			n.mu.Lock()
			n.strategy = DeadLetterStrategy(strategyStr)
			n.mu.Unlock()
		}
	}
	return nil
}
