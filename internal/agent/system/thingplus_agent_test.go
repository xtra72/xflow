package system

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// === 테스트용 fake mqtt 발행자 ===

// fakeToken 은 mqtt.Token 인터페이스를 만족하는 테스트용 토큰이다.
// 항상 즉시 완료되며 지정된 에러를 반환한다.
type fakeToken struct {
	err error
}

func (t *fakeToken) Wait() bool                     { return true }
func (t *fakeToken) WaitTimeout(time.Duration) bool { return true }
func (t *fakeToken) Done() <-chan struct{} {
	ch := make(chan struct{})
	close(ch)
	return ch
}
func (t *fakeToken) Error() error { return t.err }

// publishRecord 는 fakePublisher 가 기록한 발행 항목이다.
type publishRecord struct {
	Topic    string
	QoS      byte
	Retained bool
	Payload  []byte
}

// fakePublisher 는 mqttPublisher 인터페이스를 만족하는 테스트용 발행자이다.
// 실제 브로커 없이 발행 호출을 기록한다.
type fakePublisher struct {
	mu        sync.Mutex
	published []publishRecord
	connected bool
	pubErr    error
}

func newFakePublisher() *fakePublisher {
	return &fakePublisher{connected: true}
}

func (f *fakePublisher) Publish(topic string, qos byte, retained bool, payload interface{}) mqtt.Token {
	f.mu.Lock()
	defer f.mu.Unlock()
	var data []byte
	switch p := payload.(type) {
	case []byte:
		data = append([]byte(nil), p...)
	case string:
		data = []byte(p)
	}
	f.published = append(f.published, publishRecord{
		Topic:    topic,
		QoS:      qos,
		Retained: retained,
		Payload:  data,
	})
	return &fakeToken{err: f.pubErr}
}

func (f *fakePublisher) IsConnected() bool { return f.connected }

// subscribeCapture 는 mqttSubscriber 인터페이스를 만족하는 테스트용 구독자이다.
// 실제 브로커 없이 Subscribe 호출 토픽을 기록한다.
type subscribeCapture struct {
	mu     sync.Mutex
	topics []string
}

func newSubscribeCapture() *subscribeCapture {
	return &subscribeCapture{}
}

func (s *subscribeCapture) Subscribe(topic string, _ byte, _ mqtt.MessageHandler) mqtt.Token {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.topics = append(s.topics, topic)
	return &fakeToken{}
}

func (s *subscribeCapture) subscribed(topic string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, t := range s.topics {
		if t == topic {
			return true
		}
	}
	return false
}

func (f *fakePublisher) records() []publishRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]publishRecord, len(f.published))
	copy(out, f.published)
	return out
}

// findPublish 는 지정 토픽으로 발행된 첫 레코드를 반환한다.
func (f *fakePublisher) findPublish(topic string) (publishRecord, bool) {
	for _, r := range f.records() {
		if r.Topic == topic {
			return r, true
		}
	}
	return publishRecord{}, false
}

// === 설정 파싱 테스트 ===

func newThingplusTestConfig(opts map[string]any) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "agent-thingplus-test",
		Name: "test-thingplus",
		Type: "thingplus-gateway",
		Transport: agent.TransportConfig{
			Type:    "thingplus",
			Options: opts,
		},
	}
}

func TestParseThingplusConfig_Defaults(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker":       "localhost",
		"access_token": "SECRET_TOKEN",
	})

	tc := parseThingplusConfig(cfg)

	assert.Equal(t, 1883, tc.Port, "포트 기본값은 1883 이어야 한다")
	assert.Equal(t, "$.device", tc.DeviceNamePath, "device_name_path 기본값은 $.device 이어야 한다")
	assert.Equal(t, byte(1), tc.QoS)
	assert.Equal(t, 60, tc.KeepAliveSec)
	assert.True(t, tc.AutoReconnect)
	assert.Equal(t, 256, tc.BufferSize)
	assert.False(t, tc.TLS)
	assert.Equal(t, "SECRET_TOKEN", tc.AccessToken)
}

func TestParseThingplusConfig_Custom(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker":           "mqtt.example.com",
		"port":             8883,
		"tls":              true,
		"ca_cert":          "/etc/ca.pem",
		"access_token":     "TB_TOKEN",
		"device_name_path": "$.metadata.device_id",
		"qos":              2,
		"keep_alive_sec":   30,
		"auto_reconnect":   false,
		"buffer_size":      512,
	})

	tc := parseThingplusConfig(cfg)

	assert.Equal(t, "mqtt.example.com", tc.Broker)
	assert.Equal(t, 8883, tc.Port)
	assert.True(t, tc.TLS)
	assert.Equal(t, "/etc/ca.pem", tc.CACert)
	assert.Equal(t, "TB_TOKEN", tc.AccessToken)
	assert.Equal(t, "$.metadata.device_id", tc.DeviceNamePath)
	assert.Equal(t, byte(2), tc.QoS)
	assert.Equal(t, 30, tc.KeepAliveSec)
	assert.False(t, tc.AutoReconnect)
	assert.Equal(t, 512, tc.BufferSize)
}

// === client_id 충돌(EOF flapping) 회귀 테스트 ===

// TestParseThingplusConfig_ClientIDUnique_SameConfigID 는 client_id 충돌 버그의 핵심 재현이다.
// 동일 config.ID(또는 빈 ID)로 두 에이전트 설정을 파싱해도 각각 서로 다른
// client_id가 자동 생성되어야 한다. 결정적 client_id를 쓰면 동일 값이 되어
// 브로커가 한쪽 연결을 EOF로 끊는 flapping이 발생한다.
func TestParseThingplusConfig_ClientIDUnique_SameConfigID(t *testing.T) {
	// 두 config 모두 동일한 config.ID를 가진다 (또는 빈 ID여도 동일하게 재현된다).
	cfg := newThingplusTestConfig(map[string]any{
		"broker":       "localhost",
		"access_token": "SECRET_TOKEN",
	})

	tc1 := parseThingplusConfig(cfg)
	tc2 := parseThingplusConfig(cfg)

	assert.NotEmpty(t, tc1.ClientID, "client_id는 비어 있으면 안 된다")
	assert.NotEmpty(t, tc2.ClientID, "client_id는 비어 있으면 안 된다")
	assert.NotEqual(t, tc1.ClientID, tc2.ClientID,
		"동일 config로 파싱한 두 인스턴스의 client_id는 서로 달라야 한다 (충돌 방지)")
}

// TestParseThingplusConfig_ClientIDUnique_EmptyID 는 config.ID가 빈 문자열이어도
// 자동 생성된 client_id가 고유함을 확인한다 (config.ID에 의존하지 않는다).
func TestParseThingplusConfig_ClientIDUnique_EmptyID(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "", // 빈 ID — 결정적 방식이었다면 "xflow-thingplus-" 로 충돌했을 것이다.
		Name: "test-thingplus",
		Type: "thingplus-gateway",
		Transport: agent.TransportConfig{
			Type:    "thingplus",
			Options: map[string]any{"broker": "localhost"},
		},
	}

	tc1 := parseThingplusConfig(cfg)
	tc2 := parseThingplusConfig(cfg)

	assert.True(t, strings.HasPrefix(tc1.ClientID, "xflow-thingplus-"),
		"자동 생성 client_id는 xflow-thingplus- 프리픽스를 가져야 한다")
	assert.NotEqual(t, tc1.ClientID, tc2.ClientID,
		"빈 config.ID여도 두 인스턴스의 client_id는 서로 달라야 한다")
}

// TestParseThingplusConfig_ClientIDDefaultPrefix 는 자동 생성된 client_id가
// xflow-thingplus- 프리픽스를 가지며 비어 있지 않음을 확인한다.
func TestParseThingplusConfig_ClientIDDefaultPrefix(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker": "localhost",
	})

	tc := parseThingplusConfig(cfg)

	assert.NotEmpty(t, tc.ClientID)
	assert.True(t, strings.HasPrefix(tc.ClientID, "xflow-thingplus-"),
		"자동 생성 client_id는 xflow-thingplus- 프리픽스를 가져야 한다, got=%q", tc.ClientID)
	// "xflow-thingplus-" 뒤에 실제 UUID가 붙어 있어야 한다 (프리픽스만 있으면 안 된다).
	assert.Greater(t, len(tc.ClientID), len("xflow-thingplus-"),
		"client_id는 프리픽스 뒤에 고유 식별자를 포함해야 한다")
}

// TestParseThingplusConfig_ClientIDOverride 는 사용자가 client_id를 명시하면
// 자동 생성 대신 그 값이 사용됨을 확인한다 (override 동작).
func TestParseThingplusConfig_ClientIDOverride(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker":    "localhost",
		"client_id": "my-fixed-gateway-id",
	})

	tc := parseThingplusConfig(cfg)

	assert.Equal(t, "my-fixed-gateway-id", tc.ClientID,
		"사용자가 지정한 client_id가 그대로 사용되어야 한다")
}

// TestParseThingplusConfig_ClientIDEmptyOverrideIgnored 는 client_id가 빈 문자열로
// 주어지면 무시하고 자동 생성 값을 유지함을 확인한다.
func TestParseThingplusConfig_ClientIDEmptyOverrideIgnored(t *testing.T) {
	cfg := newThingplusTestConfig(map[string]any{
		"broker":    "localhost",
		"client_id": "",
	})

	tc := parseThingplusConfig(cfg)

	assert.True(t, strings.HasPrefix(tc.ClientID, "xflow-thingplus-"),
		"빈 client_id override는 무시되고 자동 생성 값이 유지되어야 한다")
}

func TestParseThingplusConfig_TLSDefaultPort(t *testing.T) {
	// TLS 활성 시 포트를 명시하지 않으면 8883 이 사용된다.
	cfg := newThingplusTestConfig(map[string]any{
		"broker": "secure.example.com",
		"tls":    true,
	})

	tc := parseThingplusConfig(cfg)

	assert.Equal(t, 8883, tc.Port, "TLS 활성 시 기본 포트는 8883 이어야 한다")
	assert.True(t, tc.TLS)
}

// === 브로커 URL 구성 테스트 ===

func TestThingplusBrokerURL(t *testing.T) {
	tests := []struct {
		name string
		cfg  ThingplusConfig
		want string
	}{
		{
			name: "평문 host+port",
			cfg:  ThingplusConfig{Broker: "localhost", Port: 1883, TLS: false},
			want: "tcp://localhost:1883",
		},
		{
			name: "TLS host+port",
			cfg:  ThingplusConfig{Broker: "secure.host", Port: 8883, TLS: true},
			want: "ssl://secure.host:8883",
		},
		{
			name: "이미 스킴 포함된 broker 는 그대로 사용",
			cfg:  ThingplusConfig{Broker: "tcp://broker.local:1883", Port: 1883},
			want: "tcp://broker.local:1883",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, thingplusBrokerURL(tt.cfg))
		})
	}
}

// === 타입 등록 테스트 ===

func TestRegisterThingplusTypes(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterThingplusTypes(mgr)
	require.NoError(t, err)

	// 중복 등록 시 에러 (타입이 실제로 등록되었음을 확인)
	err = RegisterThingplusTypes(mgr)
	assert.Error(t, err)
}

func TestRegisterThingplusTypes_FactoryCreatesAgent(t *testing.T) {
	mgr := agent.NewManager()
	require.NoError(t, RegisterThingplusTypes(mgr))

	// 등록된 팩토리로 에이전트가 생성 가능해야 한다.
	cfg := newThingplusTestConfig(map[string]any{
		"broker":              "192.0.2.1", // RFC 5737 문서용 IP (연결 불가)
		"access_token":        "TOKEN",
		"auto_reconnect":      true, // 죽은 브로커여도 degraded Running 으로 생성됨
		"connect_timeout_sec": 1,    // 빠른 타임아웃
	})
	a, err := mgr.Create(cfg)
	require.NoError(t, err)
	require.NotNil(t, a)
	assert.Equal(t, "thingplus-gateway", a.Type())

	// 정리
	_ = a.Stop(context.Background())
}

// === 토큰 마스킹 테스트 ===

func TestMaskSecret(t *testing.T) {
	assert.Equal(t, "", maskSecret(""))

	// 마스킹된 결과에 원본 평문이 포함되지 않아야 한다.
	masked := maskSecret("SUPER_SECRET_TOKEN")
	assert.NotContains(t, masked, "SUPER_SECRET_TOKEN")
	assert.NotEqual(t, "SUPER_SECRET_TOKEN", masked)
}

// === deviceStateMap 상태 머신 테스트 ===

func TestDeviceStateMap_Transitions(t *testing.T) {
	m := newDeviceStateMap()

	// 초기: 존재하지 않음
	_, ok := m.get("Device A")
	assert.False(t, ok)

	// ensure 로 생성 시 disconnected
	m.ensure("Device A")
	e, ok := m.get("Device A")
	require.True(t, ok)
	assert.Equal(t, deviceDisconnected, e.state)

	// connecting 전이
	m.setState("Device A", deviceConnecting)
	e, _ = m.get("Device A")
	assert.Equal(t, deviceConnecting, e.state)

	// connected 전이
	m.setState("Device A", deviceConnected)
	e, _ = m.get("Device A")
	assert.Equal(t, deviceConnected, e.state)

	assert.Equal(t, 1, m.count())

	// remove
	m.remove("Device A")
	_, ok = m.get("Device A")
	assert.False(t, ok)
	assert.Equal(t, 0, m.count())
}

func TestDeviceStateMap_ConnectedNames(t *testing.T) {
	m := newDeviceStateMap()

	m.ensure("A")
	m.setState("A", deviceConnected)
	m.ensure("B")
	m.setState("B", deviceConnecting)
	m.ensure("C")
	m.setState("C", deviceConnected)

	names := m.connectedNames()
	assert.ElementsMatch(t, []string{"A", "C"}, names, "connected 상태 디바이스만 반환해야 한다")
}

// === connectDevice / disconnectDevice / reconnectKnownDevices 테스트 ===

func newTestAgent(t *testing.T) *ThingplusGatewayAgent {
	t.Helper()
	a := &ThingplusGatewayAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("thingplus-gateway")),
		cfg:           parseThingplusConfig(newThingplusTestConfig(map[string]any{"broker": "localhost"})),
		devices:       newDeviceStateMap(),
		mapping:       newNameIDMap(),
		recvCh:        make(chan []byte, 16),
		done:          make(chan struct{}),
		stats:         agent.NewAgentStats(),
		logger:        slog.Default(),
		createdAt:     time.Now(),
		upBuf:         newBoundedUplinkBuffer(16),
		pendingRPC:    newPendingRPCMap(16),
	}
	return a
}

func TestConnectDevice_PublishesAndTransitions(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	err := a.connectDevice(pub, "Device A")
	require.NoError(t, err)

	// v1/gateway/connect 로 {"device":"Device A"} 발행 검증
	rec, ok := pub.findPublish(topicGatewayConnect)
	require.True(t, ok, "v1/gateway/connect 로 발행되어야 한다")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Payload, &payload))
	assert.Equal(t, "Device A", payload["device"])

	// PUBACK(token.Wait) 완료 후 connected 전이
	e, ok := a.devices.get("Device A")
	require.True(t, ok)
	assert.Equal(t, deviceConnected, e.state)
}

func TestConnectDevice_PublishError_StaysConnecting(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()
	pub.pubErr = assert.AnError

	err := a.connectDevice(pub, "Device A")
	require.Error(t, err)

	// 발행 실패 시 connected 로 전이되지 않는다.
	e, ok := a.devices.get("Device A")
	require.True(t, ok)
	assert.NotEqual(t, deviceConnected, e.state)
}

func TestDisconnectDevice_PublishesAndRemoves(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	require.NoError(t, a.connectDevice(pub, "Device A"))
	require.NoError(t, a.disconnectDevice(pub, "Device A"))

	rec, ok := pub.findPublish(topicGatewayDisconnect)
	require.True(t, ok, "v1/gateway/disconnect 로 발행되어야 한다")

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Payload, &payload))
	assert.Equal(t, "Device A", payload["device"])

	// 상태 맵에서 제거
	_, ok = a.devices.get("Device A")
	assert.False(t, ok)
}

func TestReconnectKnownDevices_ReconnectsConnected(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	require.NoError(t, a.connectDevice(pub, "Device A"))
	require.NoError(t, a.connectDevice(pub, "Device B"))

	// 재연결 시나리오: connect 발행 기록을 리셋한 뒤 재connect 수행
	pub.mu.Lock()
	pub.published = nil
	pub.mu.Unlock()

	a.reconnectKnownDevices(pub)

	// 이전에 connected 였던 두 디바이스 모두 재connect 되어야 한다.
	var connectDevices []string
	for _, r := range pub.records() {
		if r.Topic == topicGatewayConnect {
			var p map[string]any
			require.NoError(t, json.Unmarshal(r.Payload, &p))
			connectDevices = append(connectDevices, p["device"].(string))
		}
	}
	assert.ElementsMatch(t, []string{"Device A", "Device B"}, connectDevices)
}

// === State() 테스트 ===

func TestThingplusState_MasksToken(t *testing.T) {
	a := newTestAgent(t)
	a.cfg.AccessToken = "PLAINTEXT_SECRET"

	state := a.State()

	// State 스냅샷을 JSON 으로 직렬화해도 평문 토큰이 노출되지 않아야 한다 (AC-06).
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	assert.False(t, strings.Contains(string(raw), "PLAINTEXT_SECRET"),
		"State() 스냅샷에 access token 평문이 노출되면 안 된다")

	// 기본 상태 필드 존재 확인
	assert.Contains(t, state, "broker")
	assert.Contains(t, state, "connected")
	assert.Contains(t, state, "device_count")
}

func TestThingplusState_DeviceCount(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()
	require.NoError(t, a.connectDevice(pub, "Device A"))
	require.NoError(t, a.connectDevice(pub, "Device B"))

	state := a.State()
	assert.Equal(t, 2, state["device_count"])
}

// === 접근자 / 라이프사이클(브로커 불필요) 테스트 ===

func TestThingplusAgent_Accessors(t *testing.T) {
	a := newTestAgent(t)
	a.agentConfig = agent.AgentConfig{ID: "id-1", Name: "gw-1"}

	assert.Equal(t, "id-1", a.ID())
	assert.Equal(t, "gw-1", a.Name())
	assert.Equal(t, "thingplus-gateway", a.Type())
}

func TestThingplusAgent_Configure(t *testing.T) {
	a := newTestAgent(t)

	newCfg := newThingplusTestConfig(map[string]any{
		"broker":           "new.broker",
		"device_name_path": "$.metadata.device_id",
	})
	newCfg.ID = "id-2"
	newCfg.Name = "gw-2"

	require.NoError(t, a.Configure(newCfg))
	assert.Equal(t, "$.metadata.device_id", a.cfg.DeviceNamePath, "Configure 는 cfg 를 재파싱해야 한다")
	assert.Equal(t, "id-2", a.ID())
}

func TestThingplusAgent_Health_NotConnected(t *testing.T) {
	a := newTestAgent(t)
	// client 가 nil 이므로 Running 이 아니면 Unhealthy 이다.
	hs := a.Health()
	assert.Equal(t, agent.HealthUnhealthy, hs.Status)
}

func TestThingplusAgent_BufferInfo(t *testing.T) {
	a := newTestAgent(t)
	pending, capacity := a.BufferInfo()
	assert.Equal(t, 0, pending)
	assert.Equal(t, cap(a.recvCh), capacity)
}

func TestThingplusAgent_ConnectionStats(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()
	require.NoError(t, a.connectDevice(pub, "Device A"))

	stats := a.ConnectionStats()
	require.Len(t, stats, 1)
	assert.Equal(t, "Device A", stats[0].ID)
}

func TestThingplusAgent_Info(t *testing.T) {
	a := newTestAgent(t)
	a.agentConfig = agent.AgentConfig{ID: "id-3", Name: "gw-3", Type: "thingplus-gateway"}

	info := a.Info()
	assert.Equal(t, "id-3", info.ID)
	assert.Equal(t, "gw-3", info.Name)
	assert.Equal(t, "thingplus-gateway", info.Type)
}

func TestThingplusAgent_Stats(t *testing.T) {
	a := newTestAgent(t)
	s := a.Stats()
	assert.Equal(t, cap(a.recvCh), s.MsgBufferCapacity)
}

func TestThingplusAgent_Process_EmptyIsNoOp(t *testing.T) {
	a := newTestAgent(t)
	out, err := a.Process(nil)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestThingplusAgent_Process_InvalidJSON(t *testing.T) {
	a := newTestAgent(t)
	// 비-JSON 바이트는 파싱 실패로 관찰 가능한 에러를 반환한다 (silent 무시 금지).
	_, err := a.Process([]byte("not-json"))
	assert.Error(t, err)
}

// === 매핑 통합: setDeviceID / snapshot ===

func TestDeviceStateMap_SetDeviceIDAndSnapshot(t *testing.T) {
	m := newDeviceStateMap()
	m.ensure("Device A")
	m.setDeviceID("Device A", "uuid-a")
	m.setState("Device A", deviceConnected)

	snap := m.snapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, "uuid-a", snap[0].deviceID)
	assert.Equal(t, deviceConnected, snap[0].state)
}

func TestThingplusAgent_ReceiveMessage_FromBuffer(t *testing.T) {
	a := newTestAgent(t)
	a.recvCh <- []byte("downlink-payload")

	data, err := a.ReceiveMessage(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []byte("downlink-payload"), data)
}

func TestThingplusAgent_ReceiveMessage_ContextCancel(t *testing.T) {
	a := newTestAgent(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := a.ReceiveMessage(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestThingplusAgent_ReceiveMessage_Done(t *testing.T) {
	a := newTestAgent(t)
	close(a.done)

	_, err := a.ReceiveMessage(context.Background())
	assert.Error(t, err)
}

func TestThingplusAgent_PauseResume(t *testing.T) {
	a := newTestAgent(t)
	// created -> initializing -> running 으로 전이해 두고 pause/resume 을 검증한다.
	require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
	require.NoError(t, a.TransitionTo(lifecycle.StateRunning))

	require.NoError(t, a.Pause(context.Background()))
	assert.Equal(t, lifecycle.StatePaused, a.CurrentState())

	require.NoError(t, a.Resume(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
}

func TestThingplusAgent_Start_NoOpWhenRunning(t *testing.T) {
	a := newTestAgent(t)
	require.NoError(t, a.TransitionTo(lifecycle.StateInitializing))
	require.NoError(t, a.TransitionTo(lifecycle.StateRunning))

	// 이미 Running 이면 no-op
	require.NoError(t, a.Start(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, a.CurrentState())
}

func TestThingplusAgent_PublishMessage_NotConnected(t *testing.T) {
	a := newTestAgent(t)
	// client 가 nil 이므로 발행은 미연결 에러를 반환한다.
	err := a.PublishMessage("v1/gateway/telemetry", 1, false, []byte("{}"))
	assert.Error(t, err)
}

func TestThingplusBuildTLSConfig_Disabled(t *testing.T) {
	a := newTestAgent(t)
	a.cfg.TLS = false
	cfg, err := a.buildTLSConfig()
	require.NoError(t, err)
	assert.Nil(t, cfg, "TLS 비활성 시 nil tls.Config 를 반환해야 한다")
}

func TestThingplusBuildTLSConfig_EnabledNoCA(t *testing.T) {
	a := newTestAgent(t)
	a.cfg.TLS = true
	a.cfg.CACert = ""
	cfg, err := a.buildTLSConfig()
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, uint16(0x0303), cfg.MinVersion) // TLS 1.2
}

func TestThingplusBuildTLSConfig_InvalidCA(t *testing.T) {
	a := newTestAgent(t)
	a.cfg.TLS = true
	a.cfg.CACert = "-----BEGIN CERTIFICATE-----\nGARBAGE\n-----END CERTIFICATE-----"
	_, err := a.buildTLSConfig()
	assert.Error(t, err, "잘못된 CA PEM 은 에러를 반환해야 한다")
}

// === M3/M4 통합 테스트 (channel-mocked broker) ===

// drainRecv 는 recvCh 에서 메시지 바이트 하나를 non-blocking 으로 가져온다.
func drainRecv(t *testing.T, a *ThingplusGatewayAgent) []byte {
	t.Helper()
	select {
	case data := <-a.recvCh:
		return data
	default:
		t.Fatal("recvCh 에 방출된 메시지가 없다")
		return nil
	}
}

// TestUplink_DeviceTelemetryNoConnect 는 Device API 업링크의 핵심 계약을 검증한다:
//   - 텔레메트리가 v1/devices/me/telemetry 로 Device API 형식으로 발행된다:
//     {"ts":<UnixMilli>,"values":{"unit-1":{...}}}
//   - v1/gateway/connect 발행이 전혀 없어야 한다(Device API 는 sub-device connect 없음).
func TestUplink_DeviceTelemetryNoConnect(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher() // connected==true

	before := time.Now().UnixMilli()
	payload := map[string]any{
		"device":              "unit-1",
		"current_temperature": 24,
		"target_temperature":  20,
	}
	require.NoError(t, a.handleUplink(pub, payload))
	after := time.Now().UnixMilli()

	// CRITICAL: v1/gateway/connect 발행이 없어야 한다.
	_, hasConnect := pub.findPublish(topicGatewayConnect)
	assert.False(t, hasConnect, "Device API 는 gateway connect 를 발행하면 안 된다")

	// 텔레메트리가 v1/devices/me/telemetry 로 발행되어야 한다.
	telRec, ok := pub.findPublish(topicDeviceTelemetry)
	require.True(t, ok, "텔레메트리가 v1/devices/me/telemetry 로 발행되어야 한다")

	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))

	// ts 는 최상위, int64 epoch milliseconds (UnixMilli 범위 내).
	tsVal := int64(tel["ts"].(float64))
	assert.GreaterOrEqual(t, tsVal, before)
	assert.LessOrEqual(t, tsVal, after)

	// values 는 디바이스 NAME 을 키로 가진다.
	values := tel["values"].(map[string]any)
	unit, ok := values["unit-1"].(map[string]any)
	require.True(t, ok, "values 는 unit-1 을 키로 가져야 한다")
	assert.EqualValues(t, 24, unit["current_temperature"])
	assert.EqualValues(t, 20, unit["target_temperature"])
	// device 식별 키는 텔레메트리 values 에서 제외되어야 한다.
	_, hasDevice := unit["device"]
	assert.False(t, hasDevice)
}

// TestUplink_DeviceClientAttributes 는 클라이언트 속성이 Device API flat 형식으로
// v1/devices/me/attributes 로 발행됨을 검증한다.
func TestUplink_DeviceClientAttributes(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	payload := map[string]any{
		"device":     "unit-1",
		"attributes": map[string]any{"fw": "1.0"},
	}
	require.NoError(t, a.handleUplink(pub, payload))

	// connect 발행이 없어야 한다.
	_, hasConnect := pub.findPublish(topicGatewayConnect)
	assert.False(t, hasConnect, "Device API 는 gateway connect 를 발행하면 안 된다")

	attrRec, ok := pub.findPublish(topicDeviceAttributes)
	require.True(t, ok, "클라이언트 속성이 v1/devices/me/attributes 로 발행되어야 한다")

	// flat {"fw":"1.0"} — NAME 래핑 없음.
	var parsed map[string]any
	require.NoError(t, json.Unmarshal(attrRec.Payload, &parsed))
	assert.Equal(t, "1.0", parsed["fw"])
}

// AC-THINGPLUS-001-02: 다운링크 RPC → thingplus.rpc.request 방출 → 플로우 응답 → RPC 발행
func TestDownlink_RPCRequestEmitAndReply(t *testing.T) {
	a := newTestAgent(t)

	// (1) 브로커가 v1/gateway/rpc 로 RPC 요청 주입
	rpcBytes := []byte(`{"device":"Device A","data":{"id":1,"method":"setValue","params":{"v":10}}}`)
	a.routeDownlink(topicGatewayRPC, rpcBytes)

	// 방출된 메시지는 Type "thingplus.rpc.request" 여야 한다.
	emitted := drainRecv(t, a)
	msg, err := message.FromJSON(emitted)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.rpc.request", msg.Type(), "RPC 요청은 thingplus.rpc.request Type 으로 방출되어야 한다")

	pm := msg.Payload().ToMap()
	assert.EqualValues(t, 1, pm["id"], "RPC id 1 을 포함해야 한다")
	assert.Equal(t, "Device A", pm["device"])
	assert.Equal(t, "setValue", pm["method"])
	// device_id fallback: repo 미설정 시 NAME 자체.
	assert.Equal(t, "Device A", pm["device_id"])

	// (2) 디바이스를 connected 로 만들고 플로우 RPC 응답 주입
	pub := newFakePublisher()
	require.NoError(t, a.connectDevice(pub, "Device A"))

	replyMsg := message.New(
		message.WithType("thingplus.rpc.response"),
		message.WithPayload(message.NewPayload(map[string]any{
			"device": "Device A",
			"id":     1,
			"data":   map[string]any{"success": true},
		})),
	)
	replyBytes, err := replyMsg.MarshalJSON()
	require.NoError(t, err)

	require.NoError(t, a.handleRPCReply(pub, mustDecodePayload(t, replyBytes)))

	// v1/gateway/rpc 로 {"device":"Device A","id":1,"data":{"success":true}} 발행
	rpcRec, ok := pub.findPublish(topicGatewayRPC)
	require.True(t, ok, "RPC 응답이 v1/gateway/rpc 로 발행되어야 한다")
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rpcRec.Payload, &resp))
	assert.Equal(t, "Device A", resp["device"])
	assert.EqualValues(t, 1, resp["id"])
	assert.Equal(t, true, resp["data"].(map[string]any)["success"])
}

// RPC 응답 게이팅: connecting 상태(미PUBACK)에서는 발행하지 않는다 (AC-08 / A6/A7)
func TestDownlink_RPCReplyGatedWhenNotConnected(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	// Device A 를 connecting 상태로만 둔다 (PUBACK 미수신 시뮬레이션).
	a.devices.ensure("Device A")
	a.devices.setState("Device A", deviceConnecting)

	payload := map[string]any{"device": "Device A", "id": 1, "data": map[string]any{"ok": true}}
	err := a.handleRPCReply(pub, payload)
	assert.Error(t, err, "connected 가 아니면 게이팅되어 발행하지 않아야 한다")

	_, ok := pub.findPublish(topicGatewayRPC)
	assert.False(t, ok, "게이팅 시 RPC 발행이 없어야 한다")
}

// TestDownlink_DeviceSharedAttributesEmit 는 Device API 공유 속성 다운링크
// (v1/devices/me/attributes, flat 형식)가 thingplus.attr.update 로 방출됨을 검증한다.
func TestDownlink_DeviceSharedAttributesEmit(t *testing.T) {
	a := newTestAgent(t)

	// Device API 공유 속성: flat {"key":val} — 최상위 device 필드 없음("me" 대상).
	attrBytes := []byte(`{"target_temperature":21,"mode":"cool"}`)
	a.routeDownlink(topicDeviceAttributes, attrBytes)

	emitted := drainRecv(t, a)
	msg, err := message.FromJSON(emitted)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.attr.update", msg.Type(), "공유 속성은 thingplus.attr.update Type 으로 방출되어야 한다")

	pm := msg.Payload().ToMap()
	// 방출 페이로드에 속성 데이터 포함
	data := pm["data"].(map[string]any)
	assert.EqualValues(t, 21, data["target_temperature"])
	assert.Equal(t, "cool", data["mode"])
}

// TestDownlink_DeviceSharedAttributesSharedWrapper 는 {"shared":{...}} 래핑 형식도
// tolerant 하게 처리됨을 검증한다.
func TestDownlink_DeviceSharedAttributesSharedWrapper(t *testing.T) {
	a := newTestAgent(t)

	attrBytes := []byte(`{"shared":{"target_temperature":19}}`)
	a.routeDownlink(topicDeviceAttributes, attrBytes)

	emitted := drainRecv(t, a)
	msg, err := message.FromJSON(emitted)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.attr.update", msg.Type())

	data := msg.Payload().ToMap()["data"].(map[string]any)
	assert.EqualValues(t, 19, data["target_temperature"])
}

// AC-THINGPLUS-001-04 (dormant gateway): 게이트웨이 공유 속성 push → thingplus.attr.update 방출.
// gateway 다운링크 경로는 dormant 이나 routeDownlink 코드/테스트 일관성을 위해 유지한다.
func TestDownlink_SharedAttributesEmit(t *testing.T) {
	a := newTestAgent(t)

	attrBytes := []byte(`{"device":"Device A","data":{"fw":"1.0"}}`)
	a.routeDownlink(topicGatewayAttributes, attrBytes)

	emitted := drainRecv(t, a)
	msg, err := message.FromJSON(emitted)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.attr.update", msg.Type(), "공유 속성은 thingplus.attr.update Type 으로 방출되어야 한다")

	pm := msg.Payload().ToMap()
	assert.Equal(t, "Device A", pm["device"])
	// 방출 페이로드에 속성 데이터 포함
	data := pm["data"].(map[string]any)
	assert.Equal(t, "1.0", data["fw"])
}

// === Device API RPC 테스트 ===

// TestDownlink_DeviceRPCRequestEmit 는 Device API RPC 요청 다운링크를 검증한다:
//   - 토픽 v1/devices/me/rpc/request/{requestId} 로 {"method":..,"params":..} 인입
//   - requestId 는 토픽 suffix 에서 추출되어 방출 페이로드 id 로 담긴다(문자열 원본 보존).
//   - Type "thingplus.rpc.request" 로 방출된다.
func TestDownlink_DeviceRPCRequestEmit(t *testing.T) {
	a := newTestAgent(t)

	// 브로커가 v1/devices/me/rpc/request/42 로 RPC 요청 주입 (requestId 는 토픽에 있음).
	rpcBytes := []byte(`{"method":"setValue","params":{"v":10}}`)
	a.routeDownlink(topicDeviceRPCRequestPrefix+"42", rpcBytes)

	emitted := drainRecv(t, a)
	msg, err := message.FromJSON(emitted)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.rpc.request", msg.Type(), "Device RPC 요청은 thingplus.rpc.request Type 으로 방출되어야 한다")

	pm := msg.Payload().ToMap()
	// id 는 토픽 suffix "42" 를 문자열로 보존해야 응답이 올바른 토픽으로 라우팅된다.
	assert.Equal(t, "42", pm["id"], "requestId 는 토픽 suffix 문자열 42 여야 한다")
	assert.Equal(t, "setValue", pm["method"])
	params, ok := pm["params"].(map[string]any)
	require.True(t, ok, "params 가 방출 페이로드에 포함되어야 한다")
	assert.EqualValues(t, 10, params["v"])
}

// TestProcess_DeviceRPCResponsePublishesToTopic 는 플로우가 보낸
// thingplus.rpc.response 메시지가 v1/devices/me/rpc/response/{id} 로 발행됨을 검증한다.
// requestId 는 토픽에 담기며 본문에는 포함되지 않는다.
func TestProcess_DeviceRPCResponsePublishesToTopic(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher() // connected==true

	require.NoError(t, a.handleDeviceRPCResponse(pub, map[string]any{
		"id":     "42",
		"result": true,
	}))

	rec, ok := pub.findPublish(topicDeviceRPCResponsePrefix + "42")
	require.True(t, ok, "RPC 응답이 v1/devices/me/rpc/response/42 로 발행되어야 한다")

	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Payload, &resp))
	assert.Equal(t, true, resp["result"])
	// id 는 토픽에만 존재하고 본문에는 없어야 한다.
	_, hasID := resp["id"]
	assert.False(t, hasID, "응답 본문에 id 가 포함되면 안 된다(토픽에만 존재)")
}

// TestProcess_DeviceRPCResponseViaProcess 는 Process 진입점이 thingplus.rpc.response
// 타입 메시지를 device RPC 응답으로 분기하여 올바른 토픽에 발행함을 검증한다.
func TestProcess_DeviceRPCResponseViaProcess(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()
	a.mu.Lock()
	a.rpcResponsePublisher = pub // 테스트 발행자 주입(실제 client 는 nil).
	a.mu.Unlock()

	replyMsg := message.New(
		message.WithType("thingplus.rpc.response"),
		message.WithPayload(message.NewPayload(map[string]any{
			"id":     "7",
			"result": map[string]any{"ok": true},
		})),
	)
	replyBytes, err := replyMsg.MarshalJSON()
	require.NoError(t, err)

	_, procErr := a.Process(replyBytes)
	require.NoError(t, procErr)

	rec, ok := pub.findPublish(topicDeviceRPCResponsePrefix + "7")
	require.True(t, ok, "Process 가 device RPC 응답을 v1/devices/me/rpc/response/7 로 발행해야 한다")
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Payload, &resp))
	inner := resp["result"].(map[string]any)
	assert.Equal(t, true, inner["ok"])
}

// TestSubscribeDownlink_SubscribesRPCRequest 는 subscribeDownlink 가 공유 속성과 함께
// v1/devices/me/rpc/request/+ 를 구독함을 검증한다.
func TestSubscribeDownlink_SubscribesRPCRequest(t *testing.T) {
	a := newTestAgent(t)
	sc := newSubscribeCapture()

	a.subscribeDownlink(sc)

	assert.True(t, sc.subscribed(topicDeviceAttributes), "공유 속성 토픽을 구독해야 한다")
	assert.True(t, sc.subscribed(topicDeviceRPCRequestSub), "v1/devices/me/rpc/request/+ 를 구독해야 한다")
}

// 미지원/미인식 토픽은 원시 fallback 으로 방출된다.
func TestDownlink_UnknownTopicRawFallback(t *testing.T) {
	a := newTestAgent(t)
	raw := []byte("raw-bytes")
	a.routeDownlink("v1/gateway/unknown", raw)

	emitted := drainRecv(t, a)
	assert.Equal(t, raw, emitted, "미인식 토픽은 원시 페이로드로 fallback 되어야 한다")
}

// AC-THINGPLUS-001-05: 연결 끊김 시 업링크 무손실 버퍼링 → 재연결 flush
func TestUplink_BufferOnDisconnectAndFlushOnReconnect(t *testing.T) {
	a := newTestAgent(t)

	// (1) 미연결(pub==nil) 상태에서 텔레메트리 인입 → 버퍼링(드롭 금지)
	payload := map[string]any{"device": "Device A", "temperature": 7}
	require.NoError(t, a.handleUplink(nil, payload))

	assert.Equal(t, 1, a.upBuf.len(), "연결 끊김 시 업링크가 버퍼에 저장되어야 한다")
	assert.EqualValues(t, 0, a.upBufDropped.Load(), "silent drop 이 아니어야 한다")

	// (2) 재연결 시 버퍼 flush → 발행
	pub := newFakePublisher()
	a.flushUplinkBuffer(pub)

	assert.Equal(t, 0, a.upBuf.len(), "flush 후 버퍼가 비어야 한다")
	telRec, ok := pub.findPublish(topicDeviceTelemetry)
	require.True(t, ok, "버퍼링된 텔레메트리가 재연결 시 v1/devices/me/telemetry 로 발행되어야 한다")

	// Device API 형식: {"ts","values":{"Device A":{"temperature":7}}}
	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))
	values := tel["values"].(map[string]any)
	assert.EqualValues(t, 7, values["Device A"].(map[string]any)["temperature"])
}

// 버퍼 초과 시 관찰 가능한 드롭 (silent drop 금지)
func TestUplink_BufferFullObservableDrop(t *testing.T) {
	a := newTestAgent(t)
	a.upBuf = newBoundedUplinkBuffer(1) // 용량 1 로 축소

	// 첫 항목은 버퍼링 성공
	require.NoError(t, a.handleUplink(nil, map[string]any{"device": "A", "x": 1}))
	// 두 번째 항목은 버퍼 초과 → 관찰 가능한 에러
	err := a.handleUplink(nil, map[string]any{"device": "B", "x": 2})
	assert.Error(t, err, "버퍼 초과는 관찰 가능한 에러여야 한다")
	assert.EqualValues(t, 1, a.upBufDropped.Load(), "드롭 카운터가 증가해야 한다")
}

// Process 진입점: RPC 응답 message.Message 를 인식하여 발행 (연결 상태)
func TestProcess_RPCResponseRoutedToPublish(t *testing.T) {
	a := newTestAgent(t)
	// 실제 client 가 nil 이므로 handleRPCReply 는 미연결 에러를 반환한다.
	// Process 가 RPC 응답으로 올바르게 분기하는지(업링크로 오분류하지 않는지)를 검증한다.
	replyMsg := message.New(
		message.WithType("thingplus.rpc.response"),
		message.WithPayload(message.NewPayload(map[string]any{
			"device": "Device A", "id": 1, "data": map[string]any{"ok": true},
		})),
	)
	replyBytes, err := replyMsg.MarshalJSON()
	require.NoError(t, err)

	_, procErr := a.Process(replyBytes)
	// client==nil 이므로 게이팅/미연결 에러가 발생해야 한다(업링크로 오분류되지 않음).
	assert.Error(t, procErr)
}

// looksLikeRPCReply 판별 로직 테스트 (rpc_id / id+data / 업링크 구분).
func TestLooksLikeRPCReply(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    bool
	}{
		{"rpc_id 존재", map[string]any{"rpc_id": 1}, true},
		{"id+data 존재", map[string]any{"id": 1, "data": map[string]any{"ok": true}}, true},
		{"id 만 존재(data 없음)", map[string]any{"id": 1}, false},
		{"업링크 텔레메트리", map[string]any{"device": "A", "temperature": 42}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, looksLikeRPCReply(tt.payload))
		})
	}
}

// extractRPCReplyData 는 data 맵이 없으면 식별 키를 제외한 나머지를 응답 data 로 사용한다.
func TestExtractRPCReplyData_FallbackWithoutDataKey(t *testing.T) {
	// data 키가 없는 경우: id/device/rpc_id/device_id 를 제외한 나머지가 응답 data.
	payload := map[string]any{
		"id":      1,
		"device":  "A",
		"success": true,
		"code":    200,
	}
	data := extractRPCReplyData(payload)
	assert.Equal(t, true, data["success"])
	assert.EqualValues(t, 200, data["code"])
	_, hasID := data["id"]
	assert.False(t, hasID, "식별 키는 응답 data 에서 제외되어야 한다")
	_, hasDevice := data["device"]
	assert.False(t, hasDevice)
}

// handleRPCReply: pendingRPC 상관으로 NAME 복원 (payload 에 device 가 없을 때).
func TestHandleRPCReply_ResolvesNameFromPendingCorrelation(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	// device 를 connected 로 만들고, (Device A, id=9) 를 상관 기록.
	require.NoError(t, a.connectDevice(pub, "Device A"))
	a.pendingRPC.put("Device A", 9, "Device A")

	// payload 에 device 없이 id 만 있는 응답.
	payload := map[string]any{"id": 9, "data": map[string]any{"ok": true}}
	require.NoError(t, a.handleRPCReply(pub, payload))

	rec, ok := pub.findPublish(topicGatewayRPC)
	require.True(t, ok)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Payload, &resp))
	assert.Equal(t, "Device A", resp["device"], "pendingRPC 상관으로 NAME 이 복원되어야 한다")
}

// handleUplink: values 명시 형태 ({"device","values":{...}}) 처리.
func TestUplink_ExplicitValuesKey(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	payload := map[string]any{
		"device": "Device A",
		"values": map[string]any{"power": 100},
	}
	require.NoError(t, a.handleUplink(pub, payload))

	telRec, ok := pub.findPublish(topicDeviceTelemetry)
	require.True(t, ok)
	// Device API 형식: {"ts","values":{"Device A":{"power":100}}}
	var tel map[string]any
	require.NoError(t, json.Unmarshal(telRec.Payload, &tel))
	values := tel["values"].(map[string]any)
	assert.EqualValues(t, 100, values["Device A"].(map[string]any)["power"])
}

// handleUplink: NAME 추출 실패 시 에러 (device 키 없음).
func TestUplink_MissingDeviceNameError(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()
	err := a.handleUplink(pub, map[string]any{"temperature": 1})
	assert.Error(t, err, "device NAME 추출 실패 시 에러여야 한다")
}

// topLevelKey 는 JSONPath 에서 최상위 키를 추출한다.
func TestTopLevelKey(t *testing.T) {
	assert.Equal(t, "device", topLevelKey("$.device"))
	assert.Equal(t, "metadata", topLevelKey("$.metadata.device_id"))
	assert.Equal(t, "device", topLevelKey(""))
}

// toIntKey 는 float64/int/int64 및 미존재를 처리한다.
func TestToIntKey(t *testing.T) {
	v, ok := toIntKey(map[string]any{"id": float64(3)}, "id")
	assert.True(t, ok)
	assert.Equal(t, 3, v)

	v, ok = toIntKey(map[string]any{"rpc_id": int64(7)}, "id", "rpc_id")
	assert.True(t, ok)
	assert.Equal(t, 7, v)

	v, ok = toIntKey(map[string]any{"id": 5}, "id")
	assert.True(t, ok)
	assert.Equal(t, 5, v)

	_, ok = toIntKey(map[string]any{"other": 1}, "id")
	assert.False(t, ok)
}

// buildClientAttributes: nil attrs 는 빈 객체로 발행된다.
func TestUplink_NilAttributes_NoAttrPublish(t *testing.T) {
	// attributes 키가 아예 없으면 속성 발행이 없어야 한다(값만 텔레메트리).
	a := newTestAgent(t)
	pub := newFakePublisher()
	require.NoError(t, a.handleUplink(pub, map[string]any{"device": "A", "x": 1}))
	_, ok := pub.findPublish(topicDeviceAttributes)
	assert.False(t, ok, "속성이 없으면 attributes 발행이 없어야 한다")
}

// newBoundedUplinkBuffer 는 cap<1 을 1 로 보정한다.
func TestNewBoundedUplinkBuffer_MinCap(t *testing.T) {
	b := newBoundedUplinkBuffer(0)
	assert.True(t, b.add(bufferedUplink{topic: "t"}))
	assert.False(t, b.add(bufferedUplink{topic: "t2"}), "cap 은 최소 1 로 보정되어야 한다")
}

// handleRPCReply: id 누락 시 에러.
func TestHandleRPCReply_MissingID(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()
	err := a.handleRPCReply(pub, map[string]any{"device": "A", "data": map[string]any{}})
	assert.Error(t, err, "rpc id 누락 시 에러여야 한다")
}

// handleRPCReply: 미연결 pub 이면 에러.
func TestHandleRPCReply_NilPublisher(t *testing.T) {
	a := newTestAgent(t)
	// connected 디바이스지만 pub 이 nil 이면 발행 불가 에러.
	pub := newFakePublisher()
	require.NoError(t, a.connectDevice(pub, "A"))
	err := a.handleRPCReply(nil, map[string]any{"device": "A", "id": 1, "data": map[string]any{}})
	assert.Error(t, err)
}

// emitDownlink: recvCh 가 가득 차면 관찰 가능한 드롭(카운터 증가), silent 아님.
func TestEmitDownlink_BufferFullObservable(t *testing.T) {
	a := newTestAgent(t)
	// recvCh 를 용량 1 로 축소하고 미리 채운다.
	a.recvCh = make(chan []byte, 1)
	a.recvCh <- []byte("filled")

	before := a.stats.Snapshot().ExternalMessagesErrored
	a.emitDownlink("v1/gateway/rpc", []byte("dropped"))
	after := a.stats.Snapshot().ExternalMessagesErrored
	assert.Greater(t, after, before, "버퍼 가득 참은 관찰 가능한 에러여야 한다")
}

// flushUplinkBuffer: 발행 에러 항목도 계속 진행한다.
func TestFlushUplinkBuffer_PublishError(t *testing.T) {
	a := newTestAgent(t)
	require.NoError(t, a.handleUplink(nil, map[string]any{"device": "A", "x": 1}))
	require.Equal(t, 1, a.upBuf.len())

	pub := newFakePublisher()
	pub.pubErr = assert.AnError // 발행 실패 유도
	a.flushUplinkBuffer(pub)
	assert.Equal(t, 0, a.upBuf.len(), "flush 는 발행 실패에도 버퍼를 비운다")
}

// === FIX #2: 런타임 Configure() 데이터 레이스 회귀 테스트 (-race 로 검증) ===

// TestConfigure_ConcurrentWithUplink 는 Configure() 를 반복 호출하면서 동시에
// handleUplink/Process/State 를 구동하여 a.cfg 접근에 데이터 레이스가 없음을 검증한다.
// 수정 전(RLock 스냅샷 미적용)이라면 `go test -race` 가 이 테스트에서 레이스를 검출했을 것이다.
func TestConfigure_ConcurrentWithUplink(t *testing.T) {
	a := newTestAgent(t)
	a.agentConfig = agent.AgentConfig{ID: "id-race", Name: "gw-race"}
	pub := newFakePublisher()

	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(3)

	// (1) Configure 를 반복 호출하여 a.cfg 를 계속 재작성한다.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			cfg := newThingplusTestConfig(map[string]any{
				"broker":           "race.broker",
				"device_name_path": "$.device",
				"qos":              i % 3,
			})
			cfg.ID = "id-race"
			cfg.Name = "gw-race"
			_ = a.Configure(cfg)
		}
	}()

	// (2) handleUplink 를 반복 구동하여 a.cfg.DeviceNamePath / QoS 를 읽는다.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = a.handleUplink(pub, map[string]any{"device": "Device A", "temperature": i})
		}
	}()

	// (3) State 를 반복 호출하여 a.cfg / a.client 스냅샷 읽기를 병행한다.
	go func() {
		defer wg.Done()
		for i := 0; i < iterations; i++ {
			_ = a.State()
		}
	}()

	wg.Wait()
}

// === FIX #3: pendingRPC 복합 키 / 상한 eviction 테스트 ===

// TestPendingRPC_NoCrossDeviceMisroute 는 서로 다른 디바이스가 동일한 RPC id 를 사용할 때
// 응답이 올바른 디바이스로 라우팅되어 오배송이 없음을 검증한다.
func TestPendingRPC_NoCrossDeviceMisroute(t *testing.T) {
	a := newTestAgent(t)
	pub := newFakePublisher()

	// 두 디바이스가 동일한 RPC id(1) 요청을 상관 기록한다.
	require.NoError(t, a.connectDevice(pub, "Device A"))
	require.NoError(t, a.connectDevice(pub, "Device B"))
	a.pendingRPC.put("Device A", 1, "Device A")
	a.pendingRPC.put("Device B", 1, "Device B")

	// Device B 에 대한 응답(device 명시) → Device B 로만 발행되어야 한다.
	require.NoError(t, a.handleRPCReply(pub, map[string]any{
		"device": "Device B", "id": 1, "data": map[string]any{"ok": true},
	}))
	// Device A 에 대한 응답(device 명시) → Device A 로만 발행되어야 한다.
	require.NoError(t, a.handleRPCReply(pub, map[string]any{
		"device": "Device A", "id": 1, "data": map[string]any{"ok": true},
	}))

	// v1/gateway/rpc 로 발행된 두 레코드의 device 필드가 각각 정확해야 한다(오배송 금지).
	var devices []string
	for _, r := range pub.records() {
		if r.Topic == topicGatewayRPC {
			var resp map[string]any
			require.NoError(t, json.Unmarshal(r.Payload, &resp))
			devices = append(devices, resp["device"].(string))
		}
	}
	assert.ElementsMatch(t, []string{"Device A", "Device B"}, devices,
		"동일 id 라도 각 디바이스로 정확히 라우팅되어야 한다")
}

// TestPendingRPC_TakeByIDAmbiguousFails 는 device 미지정 응답에서 동일 id 가
// 여러 디바이스에 존재하면 안전하게 라우팅할 수 없어 실패함을 검증한다(오배송 방지).
func TestPendingRPC_TakeByIDAmbiguousFails(t *testing.T) {
	m := newPendingRPCMap(16)
	m.put("A", 1, "A")
	m.put("B", 1, "B")

	_, ok := m.takeByID(1)
	assert.False(t, ok, "동일 id 가 다수 디바이스에 존재하면 id-only 조회는 실패해야 한다")

	// 하나만 남기면 id-only 조회가 성공한다.
	_, _ = m.take("A", 1)
	name, ok := m.takeByID(1)
	assert.True(t, ok)
	assert.Equal(t, "B", name)
}

// TestPendingRPC_EvictionPastCap 는 상한 초과 시 가장 오래된 항목이 evict 됨을 검증한다.
func TestPendingRPC_EvictionPastCap(t *testing.T) {
	m := newPendingRPCMap(2) // 상한 2

	_, ev1 := m.put("A", 1, "A")
	_, ev2 := m.put("B", 2, "B")
	assert.False(t, ev1)
	assert.False(t, ev2)
	assert.Equal(t, 2, m.len())

	// 세 번째 put 은 상한 초과 → 가장 오래된 (A,1) 이 evict 되어야 한다.
	evictedName, evicted := m.put("C", 3, "C")
	assert.True(t, evicted, "상한 초과 시 eviction 이 발생해야 한다")
	assert.Equal(t, "A", evictedName, "가장 오래된 항목이 evict 되어야 한다")
	assert.Equal(t, 2, m.len(), "상한이 유지되어야 한다")

	// evict 된 (A,1) 은 더 이상 조회되지 않는다.
	_, ok := m.take("A", 1)
	assert.False(t, ok)
	// 최신 두 항목은 조회 가능하다.
	_, okB := m.take("B", 2)
	_, okC := m.take("C", 3)
	assert.True(t, okB)
	assert.True(t, okC)
}

// TestPendingRPC_EvictionCounterInState 는 buildRPCDownlinkMessage 경로에서 eviction 이
// 발생하면 pendingRPCEvicted 카운터가 증가하고 State 에 노출됨을 검증한다.
func TestPendingRPC_EvictionCounterInState(t *testing.T) {
	a := newTestAgent(t)
	a.pendingRPC = newPendingRPCMap(1) // 상한 1 로 축소하여 eviction 유도

	// 첫 RPC 다운링크: (Device A, id=1) 상관 기록.
	_, err := a.buildRPCDownlinkMessage([]byte(`{"device":"Device A","data":{"id":1,"method":"m","params":{}}}`))
	require.NoError(t, err)
	// 두 번째 RPC 다운링크: 상한 초과 → 가장 오래된 항목 evict.
	_, err = a.buildRPCDownlinkMessage([]byte(`{"device":"Device B","data":{"id":2,"method":"m","params":{}}}`))
	require.NoError(t, err)

	assert.EqualValues(t, 1, a.pendingRPCEvicted.Load(), "eviction 카운터가 증가해야 한다")
	state := a.State()
	assert.EqualValues(t, 1, state["pending_rpc_evicted"], "State 에 evict 카운터가 노출되어야 한다")
}

// mustDecodePayload 는 message.Message JSON 바이트에서 payload map 을 복원한다 (테스트 헬퍼).
func mustDecodePayload(t *testing.T, data []byte) map[string]any {
	t.Helper()
	m, err := message.FromJSON(data)
	require.NoError(t, err)
	return m.Payload().ToMap()
}

// === 인터페이스 컴파일 타임 체크 ===

func TestThingplusGatewayAgent_InterfaceChecks(t *testing.T) {
	var _ agent.Agent = (*ThingplusGatewayAgent)(nil)
	var _ agent.MessageReceiver = (*ThingplusGatewayAgent)(nil)
	var _ agent.MessagePublisher = (*ThingplusGatewayAgent)(nil)
	var _ agent.StatefulAgent = (*ThingplusGatewayAgent)(nil)
	var _ agent.SubscriberAgent = (*ThingplusGatewayAgent)(nil)
	var _ agent.BufferInfoProvider = (*ThingplusGatewayAgent)(nil)
	var _ agent.ConnectionStatsProvider = (*ThingplusGatewayAgent)(nil)
}
