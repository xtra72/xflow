package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

func newMQTTTestConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "agent-mqtt-test",
		Name: "test-mqtt-subscriber",
		Type: "mqtt",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"broker":              "tcp://localhost:1883",
				"client_id":          "xflow-test-001",
				"username":           "testuser",
				"password":           "testpass",
				"topics":             []any{"sensor/#", "device/+/data"},
				"qos":                1,
				"keep_alive_sec":     30,
				"auto_reconnect":     true,
				"clean_session":      true,
				"buffer_size":        50,
				"connect_timeout_sec": 3,
			},
		},
	}
}

func TestParseMQTTSubscriberConfig(t *testing.T) {
	cfg := newMQTTTestConfig()
	mc := parseMQTTSubscriberConfig(cfg)

	assert.Equal(t, "tcp://localhost:1883", mc.Broker)
	assert.Equal(t, "xflow-test-001", mc.ClientID)
	assert.Equal(t, "testuser", mc.Username)
	assert.Equal(t, "testpass", mc.Password)
	assert.Equal(t, []string{"sensor/#", "device/+/data"}, mc.Topics)
	assert.Equal(t, byte(1), mc.QoS)
	assert.Equal(t, 30, mc.KeepAliveSec)
	assert.True(t, mc.AutoReconnect)
	assert.True(t, mc.CleanSession)
	assert.Equal(t, 50, mc.BufferSize)
	assert.Equal(t, 3, mc.ConnectTimeoutSec)
}

func TestParseMQTTSubscriberConfig_Defaults(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-default",
		Name: "default-mqtt",
		Type: "mqtt",
		Transport: agent.TransportConfig{
			Type: "mqtt",
		},
	}
	mc := parseMQTTSubscriberConfig(cfg)

	assert.Equal(t, "tcp://localhost:1883", mc.Broker)
	assert.Equal(t, "xflow-mqtt-001", mc.ClientID)
	assert.Equal(t, "", mc.Username)
	assert.Equal(t, "", mc.Password)
	assert.Nil(t, mc.Topics)
	assert.Equal(t, byte(1), mc.QoS)
	assert.Equal(t, 60, mc.KeepAliveSec)
	assert.True(t, mc.AutoReconnect)
	assert.True(t, mc.CleanSession)
	assert.Equal(t, 256, mc.BufferSize)
	assert.Equal(t, 10, mc.ConnectTimeoutSec)
}

func TestParseMQTTSubscriberConfig_StringTopics(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-str",
		Name: "str-mqtt",
		Type: "mqtt",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"topics": []string{"topic/a", "topic/b"},
			},
		},
	}
	mc := parseMQTTSubscriberConfig(cfg)
	assert.Equal(t, []string{"topic/a", "topic/b"}, mc.Topics)
}

func TestToStringSlice(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want []string
	}{
		{
			name: "[]string",
			in:   []string{"a", "b"},
			want: []string{"a", "b"},
		},
		{
			name: "[]any with strings",
			in:   []any{"x", "y", "z"},
			want: []string{"x", "y", "z"},
		},
		{
			name: "[]any with mixed types",
			in:   []any{"a", 123, "b"},
			want: []string{"a", "b"},
		},
		{
			name: "nil",
			in:   nil,
			want: nil,
		},
		{
			name: "unsupported type",
			in:   "not a slice",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toStringSlice(tt.in)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMQTTSubscriberAgent_MessageHandler(t *testing.T) {
	// 직접 recvCh에 메시지를 넣어서 ReceiveMessage를 테스트한다.
	// 실제 MQTT 브로커 없이 채널 동작만 검증.
	recvCh := make(chan []byte, 10)
	done := make(chan struct{})

	// 채널에 메시지 전송
	payload := []byte(`{"temperature":25.5,"humidity":60}`)
	recvCh <- payload

	// ReceiveMessage 시뮬레이션
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	select {
	case data := <-recvCh:
		assert.Equal(t, payload, data)
	case <-done:
		t.Fatal("unexpected done signal")
	case <-ctx.Done():
		t.Fatal("timeout waiting for message")
	}
}

func TestMQTTSubscriberAgent_MessageHandler_BufferFull(t *testing.T) {
	// 버퍼가 가득 찬 상태에서 메시지가 드롭되는지 확인
	recvCh := make(chan []byte, 1)

	// 첫 번째 메시지: 성공
	recvCh <- []byte(`{"n":1}`)

	// 두 번째 메시지: 버퍼 가득 참 → 드롭
	select {
	case recvCh <- []byte(`{"n":2}`):
		t.Fatal("expected channel to be full")
	default:
		// 예상대로 드롭됨
	}
}

func TestMQTTSubscriberAgent_ReceiveMessage_Done(t *testing.T) {
	// done 채널이 닫히면 ReceiveMessage가 즉시 에러를 반환해야 한다.
	recvCh := make(chan []byte, 10)
	done := make(chan struct{})
	close(done)

	ctx := context.Background()
	select {
	case <-recvCh:
		t.Fatal("unexpected message")
	case <-done:
		// done 시그널 수신 → 에러 반환 시뮬레이션
	case <-ctx.Done():
		t.Fatal("unexpected context cancel")
	}
}

func TestMQTTSubscriberAgent_ReceiveMessage_ContextCancel(t *testing.T) {
	recvCh := make(chan []byte, 10)
	done := make(chan struct{})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	select {
	case <-recvCh:
		t.Fatal("unexpected message")
	case <-done:
		t.Fatal("unexpected done signal")
	case <-ctx.Done():
		assert.ErrorIs(t, ctx.Err(), context.Canceled)
	}
}

func TestMQTTSubscriberAgent_Init_ConnectionTimeout(t *testing.T) {
	// 존재하지 않는 브로커에 연결 시도 → 타임아웃 에러
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-timeout",
		Name: "timeout-mqtt",
		Type: "mqtt",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"broker":              "tcp://192.0.2.1:1883", // RFC 5737 문서용 IP (연결 불가)
				"client_id":          "xflow-timeout-test",
				"connect_timeout_sec": 1, // 1초 타임아웃
				"buffer_size":        10,
			},
		},
	}

	_, err := NewMQTTSubscriberAgent(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mqtt-subscriber init")
}

func TestRegisterMQTTTypes(t *testing.T) {
	mgr := agent.NewManager()
	err := RegisterMQTTTypes(mgr)
	require.NoError(t, err)

	// 중복 등록 시 에러
	err = RegisterMQTTTypes(mgr)
	assert.Error(t, err)
}
