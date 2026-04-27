package system

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"

	"context"
)

func TestHTTPSenderAgent_Init(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "sender-001",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
			Options: map[string]any{
				"url":          "http://localhost:9999/target",
				"method":       "POST",
				"content_type": "application/json",
				"timeout_sec":  5,
			},
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, "sender-001", a.ID())
	assert.Equal(t, "test-sender", a.Name())
	assert.Equal(t, "http-sender", a.Type())

	sender := a.(*HTTPSenderAgent)
	assert.Equal(t, lifecycle.StateRunning, sender.CurrentState())

	require.NoError(t, a.Stop(context.Background()))
}

func TestHTTPSenderAgent_Process(t *testing.T) {
	// 테스트용 HTTP 서버
	var receivedBody []byte
	var receivedContentType string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		receivedBody = body
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer ts.Close()

	cfg := agent.AgentConfig{
		ID:   "sender-002",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
			Options: map[string]any{
				"url":          ts.URL + "/target",
				"method":       "POST",
				"content_type": "application/json",
			},
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	// Process로 데이터 전송
	result, err := a.Process([]byte(`{"data":"test"}`))
	require.NoError(t, err)

	assert.Equal(t, `{"data":"test"}`, string(receivedBody))
	assert.Equal(t, "application/json", receivedContentType)
	assert.Equal(t, `{"status":"ok"}`, string(result))

	// 통계 확인
	stats := a.Stats()
	assert.Equal(t, int64(1), stats.MessagesSent)
	assert.True(t, stats.BytesWritten > 0)
}

func TestHTTPSenderAgent_Process_WithHeaders(t *testing.T) {
	var receivedHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedHeaders = r.Header
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := agent.AgentConfig{
		ID:   "sender-003",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
			Options: map[string]any{
				"url": ts.URL + "/target",
				"headers": map[string]any{
					"X-Custom":      "test-value",
					"Authorization": "Bearer token123",
				},
			},
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	_, err = a.Process([]byte(`{}`))
	require.NoError(t, err)

	assert.Equal(t, "test-value", receivedHeaders.Get("X-Custom"))
	assert.Equal(t, "Bearer token123", receivedHeaders.Get("Authorization"))
}

func TestHTTPSenderAgent_Process_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer ts.Close()

	cfg := agent.AgentConfig{
		ID:   "sender-004",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
			Options: map[string]any{
				"url": ts.URL + "/target",
			},
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	_, err = a.Process([]byte(`{"data":"test"}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "500")

	// 에러 통계 확인
	stats := a.Stats()
	assert.Equal(t, int64(1), stats.MessagesErrored)
}

func TestHTTPSenderAgent_Process_NoURL(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "sender-005",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	_, err = a.Process([]byte(`{"data":"test"}`))
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "URL is not configured")
}

func TestHTTPSenderAgent_Lifecycle(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "sender-006",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
			Options: map[string]any{
				"url": "http://localhost:9999/target",
			},
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)

	sender := a.(*HTTPSenderAgent)

	// Running 상태 확인
	assert.Equal(t, lifecycle.StateRunning, sender.CurrentState())
	assert.Equal(t, agent.HealthHealthy, a.Health().Status)

	// Pause
	require.NoError(t, a.Pause(context.Background()))
	assert.Equal(t, lifecycle.StatePaused, sender.CurrentState())

	// Resume
	require.NoError(t, a.Resume(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, sender.CurrentState())

	// Stop
	require.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, lifecycle.StateStopped, sender.CurrentState())
}

func TestHTTPSenderAgent_MultipleRequests(t *testing.T) {
	var count atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	cfg := agent.AgentConfig{
		ID:   "sender-007",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
			Options: map[string]any{
				"url": ts.URL + "/target",
			},
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	// 5건 전송
	for i := 0; i < 5; i++ {
		data, _ := json.Marshal(map[string]int{"i": i})
		_, err := a.Process(data)
		require.NoError(t, err)
	}

	assert.Equal(t, int64(5), count.Load())

	stats := a.Stats()
	assert.Equal(t, int64(5), stats.MessagesSent)
}

func TestHTTPSenderAgent_Info(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "sender-008",
		Name: "test-sender",
		Type: "http-sender",
		Transport: agent.TransportConfig{
			Type: "http-sender",
			Options: map[string]any{
				"url": "http://localhost:9999/target",
			},
		},
	}

	a, err := NewHTTPSenderAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	info := a.Info()
	assert.Equal(t, "sender-008", info.ID)
	assert.Equal(t, "test-sender", info.Name)
	assert.Equal(t, "http-sender", info.Type)
	assert.Equal(t, lifecycle.StateRunning, info.State)
	assert.True(t, info.Uptime > 0)
}
