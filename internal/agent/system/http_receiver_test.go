package system

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

func newReceiverConfig(port int) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "agent-http-recv",
		Name: "test-http-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr":  fmt.Sprintf(":%d", port),
				"path":         "/data",
				"method":       "POST",
				"timeout_sec":  5,
				"content_type": "application/json",
				"buffer_size":  10,
			},
		},
	}
}

func TestHTTPReceiverAgent_Init(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-001",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0", // OS가 빈 포트를 할당
				"path":        "/test",
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)

	assert.Equal(t, "recv-001", a.ID())
	assert.Equal(t, "test-receiver", a.Name())
	assert.Equal(t, "http", a.Type())

	recv := a.(*HTTPReceiverAgent)
	assert.Equal(t, lifecycle.StateRunning, recv.CurrentState())
	assert.NotEmpty(t, recv.ListenAddr())

	// 정리
	require.NoError(t, a.Stop(context.Background()))
}

func TestHTTPReceiverAgent_ReceiveMessage(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-002",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr":  ":0",
				"path":         "/data",
				"method":       "POST",
				"content_type": "application/json",
				"buffer_size":  10,
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	recv := a.(*HTTPReceiverAgent)
	addr := recv.ListenAddr()

	// HTTP POST 요청 전송
	url := fmt.Sprintf("http://%s/data", addr)
	body := `{"message":"hello"}`
	resp, err := http.Post(url, "application/json", bytes.NewBufferString(body))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusAccepted, resp.StatusCode)

	// ReceiveMessage로 데이터 수신
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := recv.ReceiveMessage(ctx)
	require.NoError(t, err)
	assert.Equal(t, body, string(data))
}

func TestHTTPReceiverAgent_MethodNotAllowed(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-003",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
				"method":      "POST",
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	recv := a.(*HTTPReceiverAgent)
	url := fmt.Sprintf("http://%s/data", recv.ListenAddr())

	// GET 요청은 거부되어야 한다
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestHTTPReceiverAgent_ContentTypeCheck(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-004",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr":  ":0",
				"path":         "/data",
				"method":       "POST",
				"content_type": "application/json",
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	recv := a.(*HTTPReceiverAgent)
	url := fmt.Sprintf("http://%s/data", recv.ListenAddr())

	// text/plain 은 거부되어야 한다
	resp, err := http.Post(url, "text/plain", bytes.NewBufferString("hello"))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnsupportedMediaType, resp.StatusCode)
}

func TestHTTPReceiverAgent_BufferFull(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-005",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
				"buffer_size": 1, // 버퍼 크기 1
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	recv := a.(*HTTPReceiverAgent)
	url := fmt.Sprintf("http://%s/data", recv.ListenAddr())

	// 첫 번째 요청: 성공
	resp1, err := http.Post(url, "application/json", bytes.NewBufferString(`{"n":1}`))
	require.NoError(t, err)
	resp1.Body.Close()
	assert.Equal(t, http.StatusAccepted, resp1.StatusCode)

	// 두 번째 요청: 버퍼 가득 참 → 503
	resp2, err := http.Post(url, "application/json", bytes.NewBufferString(`{"n":2}`))
	require.NoError(t, err)
	resp2.Body.Close()
	assert.Equal(t, http.StatusServiceUnavailable, resp2.StatusCode)
}

func TestHTTPReceiverAgent_Lifecycle(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-006",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)

	recv := a.(*HTTPReceiverAgent)

	// Running 상태 확인
	assert.Equal(t, lifecycle.StateRunning, recv.CurrentState())
	assert.Equal(t, agent.HealthHealthy, a.Health().Status)

	// Start는 no-op
	require.NoError(t, a.Start(context.Background()))

	// Pause
	require.NoError(t, a.Pause(context.Background()))
	assert.Equal(t, lifecycle.StatePaused, recv.CurrentState())
	assert.Equal(t, agent.HealthDegraded, a.Health().Status)

	// Resume
	require.NoError(t, a.Resume(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, recv.CurrentState())

	// Stop
	require.NoError(t, a.Stop(context.Background()))
	assert.Equal(t, lifecycle.StateStopped, recv.CurrentState())
	assert.Equal(t, agent.HealthUnhealthy, a.Health().Status)
}

func TestHTTPReceiverAgent_Stats(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-007",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr":  ":0",
				"path":         "/data",
				"content_type": "application/json",
				"buffer_size":  10,
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	recv := a.(*HTTPReceiverAgent)
	url := fmt.Sprintf("http://%s/data", recv.ListenAddr())

	// 2건 전송
	for i := 0; i < 2; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewBufferString(`{"i":1}`))
		require.NoError(t, err)
		resp.Body.Close()
	}

	stats := a.Stats()
	assert.Equal(t, int64(2), stats.MessagesReceived)
	assert.True(t, stats.BytesRead > 0)

	// Info 확인
	info := a.Info()
	assert.Equal(t, "recv-007", info.ID)
	assert.Equal(t, "test-receiver", info.Name)
	assert.True(t, info.Uptime > 0)
}

func TestHTTPReceiverAgent_ReceiveMessage_ContextCancel(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-008",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	recv := a.(*HTTPReceiverAgent)

	// 즉시 취소되는 context로 ReceiveMessage 호출
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err = recv.ReceiveMessage(ctx)
	assert.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestHTTPReceiverAgent_Process_NoOp(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-009",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	// Process는 수신 전용이므로 nil 반환
	result, err := a.Process([]byte(`{"test":true}`))
	assert.NoError(t, err)
	assert.Nil(t, result)
}

func TestHTTPReceiverAgent_MultipleMessages(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-010",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr":  ":0",
				"path":         "/data",
				"content_type": "application/json",
				"buffer_size":  100,
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)
	defer a.Stop(context.Background())

	recv := a.(*HTTPReceiverAgent)
	url := fmt.Sprintf("http://%s/data", recv.ListenAddr())

	// 5건 전송
	messages := []string{
		`{"id":1}`, `{"id":2}`, `{"id":3}`, `{"id":4}`, `{"id":5}`,
	}
	for _, msg := range messages {
		resp, err := http.Post(url, "application/json", bytes.NewBufferString(msg))
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	// 5건 모두 수신 확인 (순서 보장)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	for _, expected := range messages {
		data, err := recv.ReceiveMessage(ctx)
		require.NoError(t, err)
		assert.Equal(t, expected, string(data))
	}
}

func TestHTTPReceiverAgent_StopDrainsBuffer(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-011",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr":  ":0",
				"path":         "/data",
				"content_type": "application/json",
				"buffer_size":  100,
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)

	recv := a.(*HTTPReceiverAgent)
	url := fmt.Sprintf("http://%s/data", recv.ListenAddr())

	// 3건 전송 (수신하지 않고 버퍼에 쌓아둔다)
	for i := 0; i < 3; i++ {
		resp, err := http.Post(url, "application/json", bytes.NewBufferString(fmt.Sprintf(`{"n":%d}`, i)))
		require.NoError(t, err)
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}

	// Stop 호출 → 버퍼 드레인
	require.NoError(t, a.Stop(context.Background()))

	// DroppedOnStop 확인
	assert.Equal(t, int64(3), recv.DroppedOnStop())
}

func TestHTTPReceiverAgent_ReceiveMessage_AfterStop(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "recv-012",
		Name: "test-receiver",
		Type: "http",
		Transport: agent.TransportConfig{
			Type: "http",
			Options: map[string]any{
				"listen_addr": ":0",
				"path":        "/data",
				"buffer_size": 10,
			},
		},
	}

	a, err := NewHTTPReceiverAgent(cfg)
	require.NoError(t, err)

	recv := a.(*HTTPReceiverAgent)

	// Stop 호출
	require.NoError(t, a.Stop(context.Background()))

	// Stop 후 ReceiveMessage는 즉시 에러를 반환해야 한다
	_, err = recv.ReceiveMessage(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "stopped")
}
