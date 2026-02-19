package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// AgentManagerResolver 는 agent.Manager를 node.AgentResolver 인터페이스에 맞게 어댑팅한다.
// AgentRef의 AgentID로 에이전트를 조회하고 AgentTransport를 반환한다.
type AgentManagerResolver struct {
	manager agent.Manager
}

// NewAgentManagerResolver 는 새 AgentManagerResolver를 생성한다.
func NewAgentManagerResolver(mgr agent.Manager) *AgentManagerResolver {
	return &AgentManagerResolver{manager: mgr}
}

// ResolveAgent 는 AgentRef를 사용하여 에이전트를 찾고 AgentTransport를 반환한다.
// AgentID로 먼저 검색하고, 실패하면 AgentName으로 폴백 검색한다.
func (r *AgentManagerResolver) ResolveAgent(_ context.Context, ref flow.AgentRef) (node.AgentTransport, error) {
	// 1. AgentID로 정확히 검색
	ag, err := r.manager.Get(ref.AgentID)
	if err == nil {
		return &agentTransportAdapter{agent: ag}, nil
	}

	// 2. ID 검색 실패 시 이름으로 폴백 검색
	if ref.AgentName != "" {
		for _, a := range r.manager.List() {
			if a.Name() == ref.AgentName {
				return &agentTransportAdapter{agent: a}, nil
			}
		}
	}

	return nil, fmt.Errorf("agent %q (name=%q) not found: %w", ref.AgentID, ref.AgentName, err)
}

// agentTransportAdapter 는 agent.Agent를 node.AgentTransport에 맞게 어댑팅한다.
type agentTransportAdapter struct {
	agent agent.Agent
}

// Send 는 메시지를 에이전트에게 전송한다.
func (t *agentTransportAdapter) Send(_ context.Context, msg message.Message) error {
	data, err := msg.Payload().ToJSON()
	if err != nil {
		return fmt.Errorf("message payload marshal failed: %w", err)
	}
	_, err = t.agent.Process(data)
	return err
}

// Receive 는 에이전트로부터 메시지를 수신한다.
// 에이전트가 MessageReceiver 인터페이스를 구현하면 비동기 수신을 사용하고,
// 그렇지 않으면 context가 취소될 때까지 차단한다 (CPU 스핀 방지).
func (t *agentTransportAdapter) Receive(ctx context.Context) (message.Message, error) {
	receiver, ok := t.agent.(agent.MessageReceiver)
	if !ok {
		// MessageReceiver를 구현하지 않는 에이전트는 수신할 데이터가 없으므로
		// context가 끝날 때까지 차단하여 startReceiveLoop의 CPU 스핀을 방지한다.
		<-ctx.Done()
		return nil, ctx.Err()
	}

	data, err := receiver.ReceiveMessage(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent receive: %w", err)
	}

	// 수신 데이터를 메시지 Payload로 변환한다.
	var payload map[string]any
	if jsonErr := json.Unmarshal(data, &payload); jsonErr != nil {
		// JSON이 아니면 raw 데이터로 래핑한다.
		payload = map[string]any{"raw": string(data)}
	}

	msg := message.New(message.WithPayload(message.NewPayload(payload)))
	return msg, nil
}
