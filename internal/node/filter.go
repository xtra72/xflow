package node

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ErrFilterRejected 는 필터 조건에 의해 메시지가 거부되었음을 나타내는 에러이다.
var ErrFilterRejected = errors.New("filter: message rejected by condition")

// FilterCondition 은 메시지를 필터링하는 조건 함수 타입이다.
// true를 반환하면 메시지가 통과하고, false를 반환하면 메시지가 드롭된다.
type FilterCondition func(msg message.Message) bool

// onRejectMode 는 필터 조건 불일치 시 처리 방식을 나타낸다.
type onRejectMode int

const (
	// rejectToPort 는 reject 메시지를 "reject" 출력 포트로 전송한다 (연결 없으면 폐기).
	rejectToPort onRejectMode = iota
	// rejectToError 는 reject 메시지를 엔진 에러 포트로 전송한다.
	rejectToError
)

// FilterNode 는 조건에 따라 메시지를 필터링하는 노드이다.
// 조건이 nil이면 모든 메시지를 통과시킨다 (pass-through).
type FilterNode struct {
	*BaseNode
	condition FilterCondition
	onReject  onRejectMode   // reject 처리 모드
	mu        sync.RWMutex   // 조건 함수 보호
}

// NewFilterNode 는 새로운 FilterNode를 생성하는 팩토리 함수이다.
func NewFilterNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	base := NewBaseNode(def, opts...)
	n := &FilterNode{
		BaseNode: base,
	}
	return n, nil
}

// Init 은 FilterNode를 초기화한다.
func (n *FilterNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}
	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 조건에 따라 메시지를 필터링한다.
// 조건이 nil이면 메시지를 그대로 통과시킨다.
// 조건이 true면 "out" 포트로 통과.
// false면 on_reject 설정에 따라:
//   - reject_port (기본): "reject" 포트로 전송 (연결 없으면 폐기)
//   - error_port: ErrFilterRejected 에러 반환 → 엔진 에러 포트
func (n *FilterNode) Process(_ context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	cond := n.condition
	mode := n.onReject
	n.mu.RUnlock()

	if cond == nil {
		return []message.Message{msg}, nil
	}
	if cond(msg) {
		return []message.Message{msg}, nil
	}

	// reject 처리
	switch mode {
	case rejectToError:
		return nil, ErrFilterRejected
	default:
		// reject 포트로 전송 (_target_port 메타데이터 설정)
		out := msg.Clone()
		out.Metadata().Set("_target_port", "reject")
		return []message.Message{out}, nil
	}
}

// Shutdown 은 FilterNode를 종료한다.
func (n *FilterNode) Shutdown(ctx context.Context) error {
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 FilterNode의 설정을 적용한다.
// config에 "condition" 키가 있으면 조건을 설정한다.
// 1순위: FilterCondition Go 함수 타입 (기존 동작 보존)
// 2순위: string 조건식 (compileCondition으로 컴파일)
func (n *FilterNode) Configure(config map[string]any) error {
	if err := n.BaseNode.Configure(config); err != nil {
		return err
	}

	// on_reject 설정: "reject_port" (기본) 또는 "error_port"
	if v, ok := config["on_reject"]; ok {
		if s, ok := v.(string); ok {
			n.mu.Lock()
			switch s {
			case "error_port":
				n.onReject = rejectToError
			default:
				n.onReject = rejectToPort
			}
			n.mu.Unlock()
		}
	}

	cond, ok := config["condition"]
	if !ok {
		return nil // condition 미설정 → pass-through
	}

	// 1순위: Go 함수 타입 (기존 동작 보존)
	if fn, ok := cond.(FilterCondition); ok {
		n.mu.Lock()
		n.condition = fn
		n.mu.Unlock()
		return nil
	}

	// 2순위: 문자열 조건식 (SPEC-FILTER-001 신규)
	if expr, ok := cond.(string); ok {
		if expr == "" {
			return fmt.Errorf("filter configure: %w: empty condition expression", ErrInvalidExpression)
		}
		fn, err := compileCondition(expr)
		if err != nil {
			return fmt.Errorf("filter configure: %w", err)
		}
		n.mu.Lock()
		n.condition = fn
		n.mu.Unlock()
		return nil
	}

	return nil
}
