package engine

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/flow"
)

func TestAgentManagerResolver_ResolveAgent(t *testing.T) {
	mgr := agent.NewManager()

	// 에이전트 생성
	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "agent-001",
		Name: "test-agent",
		Type: "custom",
	})
	require.NoError(t, err)

	resolver := NewAgentManagerResolver(mgr)

	// 존재하는 에이전트 해석
	transport, err := resolver.ResolveAgent(context.Background(), flow.AgentRef{
		AgentID:   ag.ID(),
		AgentName: "test-agent",
	})
	require.NoError(t, err)
	assert.NotNil(t, transport)
}

func TestAgentManagerResolver_ResolveAgent_NotFound(t *testing.T) {
	mgr := agent.NewManager()
	resolver := NewAgentManagerResolver(mgr)

	// 존재하지 않는 에이전트 해석
	_, err := resolver.ResolveAgent(context.Background(), flow.AgentRef{
		AgentID:   "nonexistent",
		AgentName: "ghost",
	})
	assert.Error(t, err)
}

func TestAgentManagerResolver_ResolveByName(t *testing.T) {
	mgr := agent.NewManager()

	_, err := mgr.Create(agent.AgentConfig{
		ID:   "agent-name-test",
		Name: "my-agent",
		Type: "custom",
	})
	require.NoError(t, err)

	resolver := NewAgentManagerResolver(mgr)

	// 존재하지 않는 ID + 존재하는 이름으로 검색 → 이름으로 폴백
	transport, err := resolver.ResolveAgent(context.Background(), flow.AgentRef{
		AgentID:   "wrong-id",
		AgentName: "my-agent",
	})
	require.NoError(t, err)
	assert.NotNil(t, transport)
}

func TestAgentTransportAdapter_Receive_NoMessageReceiver(t *testing.T) {
	mgr := agent.NewManager()

	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "agent-plain",
		Name: "plain-agent",
		Type: "custom",
	})
	require.NoError(t, err)

	adapter := &agentTransportAdapter{agent: ag}

	// MessageReceiver를 구현하지 않으면 context 취소까지 차단한다 (CPU 스핀 방지).
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	msg, err := adapter.Receive(ctx)
	assert.Nil(t, msg)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestAgentTransportAdapter_Receive_WithMessageReceiver(t *testing.T) {
	mgr := agent.NewManager()

	// HTTP Receiver 에이전트 타입 등록
	require.NoError(t, system.RegisterHTTPTypes(mgr))

	// HTTP Receiver 에이전트 생성
	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "agent-http-recv",
		Name: "http-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
				"buffer_size": 10,
			},
		},
	})
	require.NoError(t, err)
	defer ag.Stop(context.Background())

	// MessageReceiver 인터페이스 확인
	_, ok := ag.(agent.MessageReceiver)
	assert.True(t, ok, "HTTPReceiverAgent는 MessageReceiver를 구현해야 한다")

	adapter := &agentTransportAdapter{agent: ag}

	recv := ag.(*system.HTTPReceiverAgent)
	addr := recv.ListenAddr()

	// HTTP 요청으로 데이터 전송
	go func() {
		url := fmt.Sprintf("http://%s/data", addr)
		resp, err := http.Post(url, "application/json", bytes.NewBufferString(`{"test":"hello"}`))
		if err == nil {
			resp.Body.Close()
		}
	}()

	// Adapter.Receive로 메시지 수신
	msg, err := adapter.Receive(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, msg)

	// Payload에 test 키가 있는지 확인
	payload := msg.Payload()
	val, ok := payload.Get("test")
	require.True(t, ok)
	assert.Equal(t, "hello", val)
}

func TestAgentTransportAdapter_Receive_JSONPayload(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, system.RegisterHTTPTypes(mgr))

	ag, err := mgr.Create(agent.AgentConfig{
		ID:   "agent-json-test",
		Name: "json-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
				"buffer_size": 10,
			},
		},
	})
	require.NoError(t, err)
	defer ag.Stop(context.Background())

	adapter := &agentTransportAdapter{agent: ag}

	recv := ag.(*system.HTTPReceiverAgent)
	addr := recv.ListenAddr()

	// Goroutine으로 HTTP 요청 전송
	go func() {
		url := fmt.Sprintf("http://%s/data", addr)
		resp, err := http.Post(url, "application/json", bytes.NewBufferString(`{"key":"value","number":42}`))
		if err == nil {
			resp.Body.Close()
		}
	}()

	msg, err := adapter.Receive(context.Background())
	require.NoError(t, err)

	payload := msg.Payload()
	keys := payload.Keys()
	assert.Contains(t, keys, "key")
	assert.Contains(t, keys, "number")
}
