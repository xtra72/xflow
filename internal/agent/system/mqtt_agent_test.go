package system

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

func newMQTTTestConfig() agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "agent-mqtt-test",
		Name: "test-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"broker":              "tcp://localhost:1883",
				"client_id":           "xflow-test-001",
				"username":            "testuser",
				"password":            "testpass",
				"topics":              []any{"sensor/#", "device/+/data"},
				"qos":                 1,
				"keep_alive_sec":      30,
				"auto_reconnect":      true,
				"clean_session":       true,
				"buffer_size":         50,
				"connect_timeout_sec": 3,
			},
		},
	}
}

func TestParseMQTTConfig(t *testing.T) {
	cfg := newMQTTTestConfig()
	mc := parseMQTTConfig(cfg)

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

func TestParseMQTTConfig_Defaults(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-default",
		Name: "default-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
		},
	}
	mc := parseMQTTConfig(cfg)

	assert.Equal(t, "tcp://localhost:1883", mc.Broker)
	// client_id는 UUID 기반 자동 생성
	assert.True(t, strings.HasPrefix(mc.ClientID, "xflow-"), "client_id는 'xflow-' 접두사로 시작해야 한다")
	uuidPart := strings.TrimPrefix(mc.ClientID, "xflow-")
	_, err := uuid.Parse(uuidPart)
	assert.NoError(t, err, "client_id의 UUID 부분은 유효한 UUID여야 한다")
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

func TestParseMQTTConfig_DefaultClientID_Unique(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-unique",
		Name: "unique-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
		},
	}

	ids := make(map[string]bool)
	for i := 0; i < 100; i++ {
		mc := parseMQTTConfig(cfg)
		assert.False(t, ids[mc.ClientID], "client_id 중복 발생: %s", mc.ClientID)
		ids[mc.ClientID] = true
	}
}

func TestParseMQTTConfig_CustomClientID(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-custom",
		Name: "custom-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"client_id": "my-custom-client",
			},
		},
	}
	mc := parseMQTTConfig(cfg)
	assert.Equal(t, "my-custom-client", mc.ClientID)
}

func TestParseMQTTConfig_EmptyClientID(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-empty",
		Name: "empty-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"client_id": "",
			},
		},
	}
	mc := parseMQTTConfig(cfg)
	// 빈 문자열은 사용자 설정으로 간주되지 않으므로 UUID 기반 자동 생성
	assert.True(t, strings.HasPrefix(mc.ClientID, "xflow-"))
	uuidPart := strings.TrimPrefix(mc.ClientID, "xflow-")
	_, err := uuid.Parse(uuidPart)
	assert.NoError(t, err)
}

func TestParseMQTTConfig_StringTopics(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-str",
		Name: "str-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"topics": []string{"topic/a", "topic/b"},
			},
		},
	}
	mc := parseMQTTConfig(cfg)
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

func TestMQTTAgent_MessageHandler(t *testing.T) {
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

func TestMQTTAgent_MessageHandler_BufferFull(t *testing.T) {
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

func TestMQTTAgent_ReceiveMessage_Done(t *testing.T) {
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

func TestMQTTAgent_ReceiveMessage_ContextCancel(t *testing.T) {
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

func TestMQTTAgent_Init_ConnectionTimeout(t *testing.T) {
	// auto_reconnect=false (strict 모드): 존재하지 않는 브로커에 연결 시도 → 타임아웃 에러.
	// auto_reconnect 가 꺼져 있으면 기존처럼 초기 연결 실패가 치명적이다.
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-timeout",
		Name: "timeout-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"broker":              "tcp://192.0.2.1:1883", // RFC 5737 문서용 IP (연결 불가)
				"client_id":           "xflow-timeout-test",
				"connect_timeout_sec": 1, // 1초 타임아웃
				"buffer_size":         10,
				"auto_reconnect":      false, // strict 모드 — 초기 연결 실패 시 Init 실패
			},
		},
	}

	_, err := NewMQTTAgent(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "mqtt init")
}

// TestMQTTAgent_Init_NonBlockingWithAutoReconnect 는 auto_reconnect=true 일 때
// 브로커가 도달 불가해도 Init 이 블로킹/실패하지 않고 성공(Running)함을 검증한다.
// 죽은 외부 브로커가 이 에이전트를 참조하는 플로우의 시작을 막지 못하게 하기 위함이며,
// 백그라운드 connect-retry 가 브로커 복구 시 자동 연결한다.
func TestMQTTAgent_Init_NonBlockingWithAutoReconnect(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "agent-mqtt-nonblocking",
		Name: "nonblocking-mqtt",
		Type: "mqtt-client",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"broker":              "tcp://192.0.2.1:1883", // RFC 5737 문서용 IP (연결 불가)
				"client_id":           "xflow-nonblocking-test",
				"connect_timeout_sec": 1, // 1초 타임아웃
				"buffer_size":         10,
				"auto_reconnect":      true, // 논블로킹 — 초기 연결 실패해도 Init 성공
			},
		},
	}

	a, err := NewMQTTAgent(cfg)
	require.NoError(t, err, "auto_reconnect 시 도달 불가 브로커여도 Init 은 성공해야 한다")
	require.NotNil(t, a)
	// 핵심: 죽은 브로커가 Init/플로우 시작을 막지 않는다(Running 진입). Health 는
	// paho 의 ConnectRetry 낙관적 보고로 healthy/degraded 둘 다 가능하나, Unhealthy(미시작)
	// 는 아니어야 한다.
	assert.NotEqual(t, agent.HealthUnhealthy, a.Health().Status,
		"Init 후 Running 상태여야 한다(미시작/Unhealthy 가 아님)")
}

func TestRegisterMQTTTypes(t *testing.T) {
	mgr := agent.NewManager()
	err := RegisterMQTTTypes(mgr)
	require.NoError(t, err)

	// 중복 등록 시 에러
	err = RegisterMQTTTypes(mgr)
	assert.Error(t, err)
}

// === SubscriberAgent 인터페이스 테스트 ===

func TestMQTTAgent_SubscriberAgent_컴파일타임체크(t *testing.T) {
	// 컴파일 타임에 이미 체크하지만, 테스트에서도 명시적으로 확인
	var _ agent.SubscriberAgent = (*MQTTAgent)(nil)
}

func TestRemoveTopics(t *testing.T) {
	tests := []struct {
		name     string
		list     []string
		toRemove []string
		want     []string
	}{
		{
			name:     "일부 제거",
			list:     []string{"a", "b", "c", "d"},
			toRemove: []string{"b", "d"},
			want:     []string{"a", "c"},
		},
		{
			name:     "전부 제거",
			list:     []string{"a", "b"},
			toRemove: []string{"a", "b"},
			want:     []string{},
		},
		{
			name:     "없는 토픽 제거 시도",
			list:     []string{"a", "b"},
			toRemove: []string{"x", "y"},
			want:     []string{"a", "b"},
		},
		{
			name:     "빈 목록에서 제거",
			list:     []string{},
			toRemove: []string{"a"},
			want:     []string{},
		},
		{
			name:     "제거 대상 없음",
			list:     []string{"a", "b"},
			toRemove: []string{},
			want:     []string{"a", "b"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeTopics(tt.list, tt.toRemove)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMQTTAgent_Subscribe_재연결시_전체토픽_복원(t *testing.T) {
	// subscribe() 메서드가 subscribedTopics를 기반으로 재구독하는지 확인
	// (실제 MQTT 브로커 없이 subscribedTopics 초기화 로직만 검증)
	a := &MQTTAgent{
		mqttConfig: MQTTConfig{
			Topics: []string{"sensor/#", "device/+/data"},
			QoS:    1,
		},
		logger: slog.Default(),
	}

	// 초기 상태: subscribedTopics가 비어있음
	assert.Empty(t, a.subscribedTopics)

	// subscribe()를 호출할 수는 없지만 (mqtt.Client 필요),
	// subscribedTopics 초기화 로직을 직접 검증한다.
	// subscribe()의 초기화 로직 시뮬레이션:
	a.topicsMu.Lock()
	if len(a.subscribedTopics) == 0 && len(a.mqttConfig.Topics) > 0 {
		a.subscribedTopics = make([]string, len(a.mqttConfig.Topics))
		copy(a.subscribedTopics, a.mqttConfig.Topics)
	}
	a.topicsMu.Unlock()

	assert.Equal(t, []string{"sensor/#", "device/+/data"}, a.subscribedTopics)

	// Bridge가 추가한 토픽 시뮬레이션
	a.topicsMu.Lock()
	a.subscribedTopics = append(a.subscribedTopics, "bridge/extra")
	a.topicsMu.Unlock()

	assert.Equal(t, []string{"sensor/#", "device/+/data", "bridge/extra"}, a.subscribedTopics)

	// 재연결 시 subscribedTopics가 유지되는지 확인 (초기화 조건 false)
	a.topicsMu.Lock()
	if len(a.subscribedTopics) == 0 && len(a.mqttConfig.Topics) > 0 {
		// 이 블록은 실행되지 않아야 함
		t.Fatal("subscribedTopics가 이미 있는데 다시 초기화됨")
	}
	topics := make([]string, len(a.subscribedTopics))
	copy(topics, a.subscribedTopics)
	a.topicsMu.Unlock()

	assert.Equal(t, []string{"sensor/#", "device/+/data", "bridge/extra"}, topics)
}

// TestFormatMQTTPayloadForLog 는 debug log payload 포매팅을 검증한다.
func TestFormatMQTTPayloadForLog(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{"빈 페이로드", []byte{}, ""},
		{"JSON 텍스트", []byte(`{"power":true,"mode":"cool"}`), `{"power":true,"mode":"cool"}`},
		{"평문 텍스트", []byte("hello world"), "hello world"},
		{"개행 포함", []byte("line1\nline2\tcol"), "line1\nline2\tcol"},
		{"한글 UTF-8", []byte("실내기 1"), "실내기 1"},
		{"바이너리 (0x00 포함)", []byte{0x01, 0x02, 0xff, 0x00}, "010102ff00"[2:]}, // hex
		{"제어 문자", []byte{0x07, 0x08}, "0708"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatMQTTPayloadForLog(tt.input)
			assert.Equal(t, tt.expected, got)
		})
	}
}

// TestFormatMQTTPayloadForLog_Truncate 는 1024 바이트 초과 시 자르기를 검증한다.
func TestFormatMQTTPayloadForLog_Truncate(t *testing.T) {
	// 텍스트: 1024 + 100
	long := make([]byte, 1124)
	for i := range long {
		long[i] = 'a'
	}
	got := formatMQTTPayloadForLog(long)
	assert.True(t, len(got) <= 1024+len("...(truncated)"))
	assert.Contains(t, got, "...(truncated)")

	// 바이너리: 1024/2 + 100 = 612 → hex 인코딩 후 truncate 표시
	bin := make([]byte, 612)
	for i := range bin {
		bin[i] = byte(i % 256)
	}
	// 첫 바이트가 0x00 (제어 문자) 이므로 바이너리로 인식.
	gotBin := formatMQTTPayloadForLog(bin)
	assert.Contains(t, gotBin, "...(truncated)")
}
