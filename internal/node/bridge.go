package node

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// AgentResolver 는 에이전트 참조를 실제 AgentTransport로 해석하는 인터페이스이다.
// 실제 구현은 engine/agent 패키지에서 제공한다.
type AgentResolver interface {
	ResolveAgent(ctx context.Context, ref flow.AgentRef) (AgentTransport, error)
}

// AgentTransport 는 에이전트와의 통신을 담당하는 인터페이스이다.
type AgentTransport interface {
	Send(ctx context.Context, msg message.Message) error
	Receive(ctx context.Context) (message.Message, error)
}

// BridgeNode 는 에이전트와 플로우 간의 다리 역할을 하는 노드이다.
// 4가지 모드(in, out, inout, request_reply)를 지원한다.
type BridgeNode struct {
	*BaseNode
	agentRef     flow.AgentRef
	direction    flow.BridgeDirection
	resolver     AgentResolver
	transport    AgentTransport
	replyTimeout time.Duration
	correlations sync.Map // Request-Reply: correlationID -> chan message.Message
	mu           sync.RWMutex
}

// WithAgentResolver 는 BridgeNode에 AgentResolver를 설정하는 옵션을 반환한다.
func WithAgentResolver(resolver AgentResolver) NodeOption {
	return func(b *BaseNode) {
		// resolver는 BridgeNode 생성 후 적용되므로, config에 저장
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_agent_resolver"] = resolver
	}
}

// WithReplyTimeout 은 BridgeNode의 요청-응답 타임아웃을 설정하는 옵션을 반환한다.
func WithReplyTimeout(d time.Duration) NodeOption {
	return func(b *BaseNode) {
		if b.config == nil {
			b.config = make(map[string]any)
		}
		b.config["_reply_timeout"] = d
	}
}

// NewBridgeNode 는 새로운 BridgeNode를 생성하는 팩토리 함수이다.
// NodeDef에 AgentRef가 없으면 에러를 반환한다.
func NewBridgeNode(def flow.NodeDef, opts ...NodeOption) (Node, error) {
	if def.AgentRef == nil {
		return nil, &NodeError{
			NodeID:   def.ID,
			NodeType: def.Type,
			Err:      ErrAgentNotFound,
		}
	}

	base := NewBaseNode(def, opts...)
	n := &BridgeNode{
		BaseNode:     base,
		agentRef:     *def.AgentRef,
		direction:    def.AgentRef.Direction,
		replyTimeout: 30 * time.Second,
	}

	// 옵션에서 resolver와 timeout 추출
	if base.config != nil {
		if r, ok := base.config["_agent_resolver"]; ok {
			if resolver, ok := r.(AgentResolver); ok {
				n.resolver = resolver
			}
		}
		if t, ok := base.config["_reply_timeout"]; ok {
			if timeout, ok := t.(time.Duration); ok {
				n.replyTimeout = timeout
			}
		}
	}

	return n, nil
}

// Init 은 BridgeNode를 초기화한다.
// AgentResolver가 설정되어 있으면 에이전트를 해석하여 transport를 설정한다.
func (n *BridgeNode) Init(ctx context.Context) error {
	if err := n.BaseNode.TransitionTo(lifecycle.StateInitializing); err != nil {
		return err
	}

	if n.resolver == nil {
		return &NodeError{
			NodeID:   n.ID(),
			NodeType: n.Type(),
			Err:      ErrAgentNotFound,
		}
	}

	transport, err := n.resolver.ResolveAgent(ctx, n.agentRef)
	if err != nil {
		return &NodeError{
			NodeID:   n.ID(),
			NodeType: n.Type(),
			Err:      err,
		}
	}
	n.mu.Lock()
	n.transport = transport
	n.mu.Unlock()

	return n.BaseNode.TransitionTo(lifecycle.StateRunning)
}

// Process 는 방향에 따라 메시지를 처리한다.
func (n *BridgeNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) {
	n.mu.RLock()
	transport := n.transport
	direction := n.direction
	n.mu.RUnlock()

	switch direction {
	case flow.BridgeOut:
		// 플로우 -> 에이전트: 메시지를 에이전트에 전송
		if transport != nil {
			if err := transport.Send(ctx, msg); err != nil {
				return nil, err
			}
		}
		return []message.Message{}, nil

	case flow.BridgeIn, flow.BridgeInOut:
		// 에이전트 -> 플로우: 외부에서 수신 처리, Process는 통과
		return []message.Message{msg}, nil

	case flow.BridgeRequestReply:
		// 요청-응답 패턴: 메시지 전송 후 응답 대기
		if transport == nil {
			return nil, ErrAgentNotFound
		}

		correlationID := uuid.New().String()
		msg.Metadata().Set("_correlationID", correlationID)

		replyCh := make(chan message.Message, 1)
		n.correlations.Store(correlationID, replyCh)
		defer n.correlations.Delete(correlationID)

		if err := transport.Send(ctx, msg); err != nil {
			return nil, err
		}

		// 타임아웃 컨텍스트 생성
		timeoutCtx, cancel := context.WithTimeout(ctx, n.replyTimeout)
		defer cancel()

		// 응답 수신 시도
		reply, err := transport.Receive(timeoutCtx)
		if err != nil {
			return nil, ErrRequestTimeout
		}
		return []message.Message{reply}, nil

	default:
		return []message.Message{msg}, nil
	}
}

// Shutdown 은 BridgeNode를 종료한다.
// 대기 중인 correlations을 정리한다.
func (n *BridgeNode) Shutdown(ctx context.Context) error {
	// 대기 중인 correlation 채널 정리
	n.correlations.Range(func(key, value any) bool {
		if ch, ok := value.(chan message.Message); ok {
			close(ch)
		}
		n.correlations.Delete(key)
		return true
	})
	return n.BaseNode.TransitionTo(lifecycle.StateStopping)
}

// Configure 는 BridgeNode의 설정을 적용한다.
func (n *BridgeNode) Configure(config map[string]any) error {
	return n.BaseNode.Configure(config)
}
