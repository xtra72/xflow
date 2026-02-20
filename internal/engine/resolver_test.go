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

// --- PayloadFormat 변환 테스트 ---

// TestConvertPayload_Auto_JSON 은 auto 모드에서 유효한 JSON이 객체로 파싱되는지 확인한다.
func TestConvertPayload_Auto_JSON(t *testing.T) {
	adapter := &agentTransportAdapter{}
	data := []byte(`{"key":"value","num":42}`)

	msg, err := adapter.convertPayload(data)
	require.NoError(t, err)

	val, ok := msg.Payload().Get("key")
	require.True(t, ok)
	assert.Equal(t, "value", val)
}

// TestConvertPayload_Auto_NonJSON 은 auto 모드에서 JSON이 아닌 데이터가 raw 문자열로 래핑되는지 확인한다.
func TestConvertPayload_Auto_NonJSON(t *testing.T) {
	adapter := &agentTransportAdapter{payloadFormat: "auto"}
	data := []byte("plain text data")

	msg, err := adapter.convertPayload(data)
	require.NoError(t, err)

	val, ok := msg.Payload().Get("raw")
	require.True(t, ok)
	assert.Equal(t, "plain text data", val)
}

// TestConvertPayload_JSON_유효 는 json 모드에서 유효한 JSON이 객체로 파싱되는지 확인한다.
func TestConvertPayload_JSON_유효(t *testing.T) {
	adapter := &agentTransportAdapter{payloadFormat: "json"}
	data := []byte(`{"temperature":25.5}`)

	msg, err := adapter.convertPayload(data)
	require.NoError(t, err)

	val, ok := msg.Payload().Get("temperature")
	require.True(t, ok)
	assert.Equal(t, 25.5, val)
}

// TestConvertPayload_JSON_무효 는 json 모드에서 무효한 JSON이 에러를 반환하는지 확인한다.
func TestConvertPayload_JSON_무효(t *testing.T) {
	adapter := &agentTransportAdapter{payloadFormat: "json"}
	data := []byte("not json")

	msg, err := adapter.convertPayload(data)
	assert.Error(t, err)
	assert.Nil(t, msg)
	assert.Contains(t, err.Error(), "invalid JSON")
}

// TestConvertPayload_Raw 는 raw 모드에서 항상 "raw" 키에 문자열로 저장되는지 확인한다.
func TestConvertPayload_Raw(t *testing.T) {
	adapter := &agentTransportAdapter{payloadFormat: "raw"}

	tests := []struct {
		name string
		data []byte
	}{
		{"JSON_데이터도_raw로", []byte(`{"key":"value"}`)},
		{"일반_문자열", []byte("hello world")},
		{"바이너리_데이터", []byte{0x00, 0x01, 0x02}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, err := adapter.convertPayload(tt.data)
			require.NoError(t, err)

			val, ok := msg.Payload().Get("raw")
			require.True(t, ok)
			assert.Equal(t, string(tt.data), val)
		})
	}
}

// TestConvertPayload_Binary 는 binary 모드에서 "_raw" 키에 []byte로 저장되는지 확인한다.
func TestConvertPayload_Binary(t *testing.T) {
	adapter := &agentTransportAdapter{payloadFormat: "binary"}
	data := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	msg, err := adapter.convertPayload(data)
	require.NoError(t, err)

	val, ok := msg.Payload().Get("_raw")
	require.True(t, ok)
	assert.Equal(t, data, val)
}

// TestSetPayloadFormat 은 SetPayloadFormat이 payloadFormat 필드를 설정하는지 확인한다.
func TestSetPayloadFormat(t *testing.T) {
	adapter := &agentTransportAdapter{}
	assert.Empty(t, adapter.payloadFormat)

	adapter.SetPayloadFormat("json")
	assert.Equal(t, "json", adapter.payloadFormat)

	adapter.SetPayloadFormat("binary")
	assert.Equal(t, "binary", adapter.payloadFormat)
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
