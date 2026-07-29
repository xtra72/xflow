package xsfm

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// mockMQTTClient 는 MQTTClient 인터페이스의 테스트용 목이다. Publish/Subscribe 호출을
// 기록하고, 구독 콜백을 저장하여 유입 상태를 시뮬레이션할 수 있다.
type mockMQTTClient struct {
	mu           sync.Mutex
	connected    bool
	published    []publishCall
	subscribed   map[string]MessageHandler
	unsubscribed []string            // Unsubscribe 호출 토픽 기록 (remove_device 배선 검증용).
	failTopics   map[string]struct{} // Publish 가 실패할 토픽 집합 (fan-out 부분 실패 테스트용).
}

type publishCall struct {
	topic   string
	qos     byte
	payload []byte
}

func newMockMQTTClient() *mockMQTTClient {
	return &mockMQTTClient{subscribed: make(map[string]MessageHandler)}
}

func (m *mockMQTTClient) Connect() error { m.mu.Lock(); m.connected = true; m.mu.Unlock(); return nil }
func (m *mockMQTTClient) Disconnect()    { m.mu.Lock(); m.connected = false; m.mu.Unlock() }
func (m *mockMQTTClient) IsConnected() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.connected
}

func (m *mockMQTTClient) Publish(topic string, qos byte, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, fail := m.failTopics[topic]; fail {
		return fmt.Errorf("mock publish failed: %s", topic)
	}
	cp := make([]byte, len(payload))
	copy(cp, payload)
	m.published = append(m.published, publishCall{topic: topic, qos: qos, payload: cp})
	return nil
}

// failPublish 는 지정 토픽의 Publish 가 실패하도록 설정한다 (fan-out 부분 실패 주입).
func (m *mockMQTTClient) failPublish(topic string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failTopics == nil {
		m.failTopics = make(map[string]struct{})
	}
	m.failTopics[topic] = struct{}{}
}

// hasPublished 는 지정 토픽으로 발행된 적이 있는지 락 하에 반환한다 (에코 주입 타이밍용).
func (m *mockMQTTClient) hasPublished(topic string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, p := range m.published {
		if p.topic == topic {
			return true
		}
	}
	return false
}

// publishedTopics 는 발행된 토픽 목록을 락 하에 반환한다 (fan-out 대상 검증용).
func (m *mockMQTTClient) publishedTopics() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.published))
	for _, p := range m.published {
		out = append(out, p.topic)
	}
	return out
}

func (m *mockMQTTClient) Subscribe(topic string, _ byte, cb MessageHandler) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.subscribed[topic] = cb
	return nil
}

func (m *mockMQTTClient) Unsubscribe(topic string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.subscribed, topic)
	m.unsubscribed = append(m.unsubscribed, topic)
	return nil
}

// unsubscribedTopics 는 Unsubscribe 로 해제된 토픽 목록을 락 하에 반환한다.
func (m *mockMQTTClient) unsubscribedTopics() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.unsubscribed))
	copy(out, m.unsubscribed)
	return out
}

// publishCount 는 발행된 메시지 수를 락 하에 반환한다 (request_state 무통신 검증용).
func (m *mockMQTTClient) publishCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.published)
}

// deliver 는 구독된 토픽으로 페이로드가 도착한 것을 시뮬레이션한다. 정확 일치가 없으면
// MQTT 단일 레벨 와일드카드("+") 패턴과 매칭하여 콜백을 찾는다 (M14 와일드카드 구독). 콜백은
// 실제 토픽을 인자로 받아 파싱한다.
func (m *mockMQTTClient) deliver(topic string, payload []byte) {
	m.mu.Lock()
	cb, ok := m.subscribed[topic]
	if !ok {
		for pat, c := range m.subscribed {
			if mqttTopicMatches(pat, topic) {
				cb = c
				break
			}
		}
	}
	m.mu.Unlock()
	if cb != nil {
		cb(topic, payload)
	}
}

// mqttTopicMatches 는 MQTT 단일 레벨 와일드카드("+") 패턴이 토픽과 매칭하는지 판정한다.
func mqttTopicMatches(pattern, topic string) bool {
	ps := strings.Split(pattern, "/")
	ts := strings.Split(topic, "/")
	if len(ps) != len(ts) {
		return false
	}
	for i := range ps {
		if ps[i] == "+" {
			continue
		}
		if ps[i] != ts[i] {
			return false
		}
	}
	return true
}

func (m *mockMQTTClient) subscribedTopics() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, 0, len(m.subscribed))
	for t := range m.subscribed {
		out = append(out, t)
	}
	return out
}

// validPayloadMapping 은 테스트용 유효 payload_mapping 이다.
func validPayloadMapping() map[string]any {
	return map[string]any{
		"power_field":     "power",
		"fan_speed_field": "fan_speed",
	}
}

// directOpts 는 유효한 direct 모드 옵션을 반환한다.
func directOpts() map[string]any {
	return map[string]any{
		"broker":                 "tcp://broker:1883",
		"state_topic_template":   "xsfm/{device_id}/state",
		"command_topic_template": "xsfm/{device_id}/cmd",
		"payload_mapping":        validPayloadMapping(),
	}
}

func baseAgentConfig(opts map[string]any) agent.AgentConfig {
	return agent.AgentConfig{
		ID:        "ap-agent-1",
		Name:      "xsfm-1",
		Type:      agentType,
		Transport: agent.TransportConfig{Type: "mqtt", Options: opts},
	}
}

// asAP 는 agent.Agent 를 *XSFMAgent 로 캐스팅한다 (in-package 테스트).
func asAP(t *testing.T, a agent.Agent) *XSFMAgent {
	t.Helper()
	ap, ok := a.(*XSFMAgent)
	require.True(t, ok, "expected *XSFMAgent")
	return ap
}

// Scenario 1.1: 설정 파싱 및 초기화 (direct 기본, Running, MQTT client 생성, config 디바이스 등록).
func TestNewAgent_DirectInitRunning(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101", "name": "플랫폼-1", "group_id": "line2"},
	}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	assert.Equal(t, lifecycle.StateRunning, ap.CurrentState())
	assert.NotNil(t, ap.client, "direct 모드는 MQTT 클라이언트를 생성해야 한다")
	assert.Equal(t, transportModeDirect, ap.cfg.TransportMode)

	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.Equal(t, "config", dev.Source)
	assert.False(t, dev.Online, "config 디바이스는 Online=false 로 초기화되어야 한다")
	assert.Equal(t, "line2", dev.GroupID)
}

// Scenario 1.7: transport_mode 미지정 시 기본 direct.
func TestParseConfig_TransportModeDefaultDirect(t *testing.T) {
	cfg, err := parseXSFMConfig(directOpts())
	require.NoError(t, err)
	assert.Equal(t, "direct", cfg.TransportMode)
}

// Scenario 1.2: 토픽 템플릿 placeholder 누락 거부.
func TestParseConfig_MissingPlaceholder(t *testing.T) {
	opts := directOpts()
	opts["state_topic_template"] = "xsfm/state" // {device_id} 없음
	_, err := parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrInvalidTopicTemplate)
}

// Scenario 1.3 / 1.10: direct 모드 broker 빈 문자열 거부.
func TestParseConfig_BrokerRequiredDirect(t *testing.T) {
	opts := directOpts()
	opts["broker"] = ""
	_, err := parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrBrokerRequired)

	// broker 키 자체가 없는 경우도 동일.
	opts2 := directOpts()
	delete(opts2, "broker")
	_, err = parseXSFMConfig(opts2)
	assert.ErrorIs(t, err, ErrBrokerRequired)
}

// Scenario 1.4: payload_mapping 누락 / power·fan 필드 누락 거부.
func TestParseConfig_InvalidPayloadMapping(t *testing.T) {
	// payload_mapping 키 자체 누락
	opts := directOpts()
	delete(opts, "payload_mapping")
	_, err := parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrInvalidPayloadMapping)

	// power_field 누락
	opts = directOpts()
	opts["payload_mapping"] = map[string]any{"fan_speed_field": "fan_speed"}
	_, err = parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrInvalidPayloadMapping)

	// fan_speed_field 누락
	opts = directOpts()
	opts["payload_mapping"] = map[string]any{"power_field": "power"}
	_, err = parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrInvalidPayloadMapping)
}

// Scenario 1.8: 유효하지 않은 transport_mode 거부.
func TestParseConfig_InvalidTransportMode(t *testing.T) {
	opts := directOpts()
	opts["transport_mode"] = "bridge"
	_, err := parseXSFMConfig(opts)
	assert.ErrorIs(t, err, ErrInvalidTransportMode)
}

// Scenario 1.9: port 모드는 브로커/토픽 없이 Running, client==nil.
func TestNewAgent_PortModeNoBroker(t *testing.T) {
	opts := map[string]any{
		"transport_mode":  "port",
		"payload_mapping": validPayloadMapping(),
	}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	assert.Equal(t, lifecycle.StateRunning, ap.CurrentState())
	assert.Nil(t, ap.client, "port 모드는 MQTT 클라이언트를 생성하지 않아야 한다")
	assert.NotNil(t, ap.ControlPort(), "port 모드는 제어 출력 포트를 노출해야 한다")

	// port 모드 Start 는 브로커 연결 없이 Running 유지.
	require.NoError(t, ap.Start(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, ap.CurrentState())
}

// Scenario 1.5: 인터페이스 준수 (컴파일 타임 + 런타임 캐스트).
func TestInterfaceCompliance(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)

	_, ok := a.(agent.Agent)
	assert.True(t, ok, "agent.Agent 구현")
	_, ok = a.(agent.MessageReceiver)
	assert.True(t, ok, "agent.MessageReceiver 구현")
}

// Start(direct) 가 목 클라이언트로 모든 디바이스 state 토픽을 구독하고, 유입 페이로드가
// 공유 디코딩 경로로 로스터를 갱신하는지 검증한다 (상태 유입 시임).
func TestStartDirect_SubscribeAndIngest(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101"},
		map[string]any{"device_id": "ap-102"},
	}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	// 목 클라이언트 주입 (실제 pahoMQTTClient 대체).
	mock := newMockMQTTClient()
	ap.client = mock
	ap.cmdSink = &brokerCommandSink{client: mock, topicTmpl: ap.cfg.CommandTopicTemplate, qos: ap.cfg.QoS}

	require.NoError(t, ap.Start(context.Background()))
	// M14: 디바이스별 렌더 구독 대신 단일 와일드카드 구독.
	assert.ElementsMatch(t,
		[]string{"xsfm/+/state"},
		mock.subscribedTopics(),
	)

	// 유입 상태 페이로드 → 로스터 갱신 (와일드카드 콜백이 실제 토픽을 파싱하여 대상 디바이스 도출).
	mock.deliver("xsfm/ap-101/state", []byte(`{"power":true,"fan_speed":2}`))
	dev, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, dev.Power)
	assert.Equal(t, 2, dev.FanSpeed)
	assert.False(t, dev.LastSeen.IsZero())
}

// brokerCommandSink 는 미연결 시 ErrNotConnected, 연결 시 command 토픽으로 발행.
func TestBrokerCommandSink(t *testing.T) {
	mock := newMockMQTTClient()
	sink := &brokerCommandSink{client: mock, topicTmpl: "xsfm/{device_id}/cmd", qos: 1}
	did := map[string]string{"device_id": "ap-101"}

	assert.ErrorIs(t, sink.SendCommand(outboundCommand{DeviceID: "ap-101", Fields: did, Payload: []byte(`{"power":true}`)}), ErrNotConnected)

	require.NoError(t, mock.Connect())
	require.NoError(t, sink.SendCommand(outboundCommand{DeviceID: "ap-101", Fields: did, Payload: []byte(`{"power":true}`)}))
	require.Len(t, mock.published, 1)
	assert.Equal(t, "xsfm/ap-101/cmd", mock.published[0].topic)
	assert.JSONEq(t, `{"power":true}`, string(mock.published[0].payload))
}

// M14: brokerCommandSink 는 attribute-per-topic 모드에서 {attribute} 를 채운 축별 토픽으로 발행.
func TestBrokerCommandSink_AttributeMode(t *testing.T) {
	mock := newMockMQTTClient()
	require.NoError(t, mock.Connect())
	sink := &brokerCommandSink{
		client:    mock,
		topicTmpl: "cmd/ui-line/{station_code}/{place_code}/bse9000/{device_index}/{attribute}",
		qos:       1,
	}
	addr := map[string]string{"station_code": "ST1", "place_code": "P1", "device_index": "3"}
	require.NoError(t, sink.SendCommand(outboundCommand{DeviceID: "ST1:P1:3", Fields: addr, Attribute: "power", Payload: []byte("on")}))
	require.Len(t, mock.published, 1)
	assert.Equal(t, "cmd/ui-line/ST1/P1/bse9000/3/power", mock.published[0].topic)
	assert.Equal(t, "on", string(mock.published[0].payload))
}

// portCommandSink 는 제어 출력 포트로 ControlMessage 를 방출한다 (주소 필드/attribute 포함).
func TestPortCommandSink(t *testing.T) {
	ch := make(chan ControlMessage, 4)
	sink := &portCommandSink{ch: ch}
	addr := map[string]string{"station_code": "ST1"}
	require.NoError(t, sink.SendCommand(outboundCommand{DeviceID: "ap-101", Fields: addr, Attribute: "power", Payload: []byte("on")}))
	msg := <-ch
	assert.Equal(t, "ap-101", msg.DeviceID)
	assert.Equal(t, "ST1", msg.Fields["station_code"])
	assert.Equal(t, "power", msg.Attribute)
	assert.Equal(t, "on", string(msg.Payload))
}

// Process 는 아직 미구현인 명령(후속 배치)과 알 수 없는 명령을 ErrInvalidCommand 로 반환한다.
func TestProcess_UnimplementedAndUnknown(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)

	// get_state 는 후속 배치에서 구현되므로 아직 ErrInvalidCommand.
	_, err = a.Process([]byte(`{"command":"get_state","device_id":"ap-101"}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)

	_, err = a.Process([]byte(`{"command":"nonsense"}`))
	assert.ErrorIs(t, err, ErrInvalidCommand)

	_, err = a.Process([]byte(`not json`))
	assert.Error(t, err)
}

// 로스터 조회 메서드 (ListDevices/GetDevice/GroupMembers).
func TestRosterQueries(t *testing.T) {
	opts := directOpts()
	opts["devices"] = []any{
		map[string]any{"device_id": "ap-101", "group_id": "g1"},
		map[string]any{"device_id": "ap-102", "group_id": "g1"},
		map[string]any{"device_id": "ap-103", "group_id": "g2"},
	}
	a, err := NewXSFMAgent(baseAgentConfig(opts))
	require.NoError(t, err)
	ap := asAP(t, a)

	assert.Len(t, ap.ListDevices(), 3)
	assert.Equal(t, []string{"ap-101", "ap-102"}, ap.GroupMembers("g1"))
	assert.Equal(t, []string{"ap-103"}, ap.GroupMembers("g2"))
	assert.Empty(t, ap.GroupMembers("none"))

	_, err = ap.GetDevice("missing")
	assert.ErrorIs(t, err, ErrDeviceNotFound)
}

// 라이프사이클 전이 (Pause/Resume/Stop) + Health.
func TestLifecycleTransitions(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)
	ctx := context.Background()

	assert.Equal(t, agent.HealthHealthy, ap.Health().Status)

	require.NoError(t, ap.Pause(ctx))
	assert.Equal(t, lifecycle.StatePaused, ap.CurrentState())
	assert.Equal(t, agent.HealthDegraded, ap.Health().Status)

	require.NoError(t, ap.Resume(ctx))
	assert.Equal(t, lifecycle.StateRunning, ap.CurrentState())

	require.NoError(t, ap.Stop(ctx))
	assert.Equal(t, lifecycle.StateStopped, ap.CurrentState())
	assert.Equal(t, agent.HealthUnhealthy, ap.Health().Status)
}

// ReceiveMessage 는 stopCh 종료 시 에러를 반환한다.
func TestReceiveMessage_StopAndContext(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	// ctx 취소 시 ctx.Err.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ap.ReceiveMessage(ctx)
	assert.True(t, errors.Is(err, context.Canceled))

	// Stop 후 stopCh 종료 → 에러.
	require.NoError(t, ap.Stop(context.Background()))
	_, err = ap.ReceiveMessage(context.Background())
	assert.Error(t, err)
}

// Info/Stats/ID/Name/Type 접근자.
func TestAccessors(t *testing.T) {
	a, err := NewXSFMAgent(baseAgentConfig(directOpts()))
	require.NoError(t, err)
	ap := asAP(t, a)

	assert.Equal(t, "ap-agent-1", ap.ID())
	assert.Equal(t, "xsfm-1", ap.Name())
	assert.Equal(t, "xsfm", ap.Type())

	info := ap.Info()
	assert.Equal(t, "ap-agent-1", info.ID)
	assert.Equal(t, lifecycle.StateRunning, info.State)

	s := ap.Stats()
	assert.Equal(t, msgChannelSize, s.MsgBufferCapacity)
}

// StateForJSON 은 관측된 축만 emit 하고 power=off 시 fan_speed 를 생략한다.
func TestDeviceStateForJSON_ObservedGating(t *testing.T) {
	d := &Device{}
	assert.Empty(t, d.StateForJSON(), "미관측 디바이스는 빈 map")

	d.Power = true
	d.markObserved(observedPower)
	d.FanSpeed = 3
	d.markObserved(observedFanSpeed)
	out := d.StateForJSON()
	assert.Equal(t, true, out["power"])
	assert.Equal(t, 3, out["fan_speed"])

	// power=off 이면 fan_speed 생략 (정규화).
	d.Power = false
	out = d.StateForJSON()
	assert.Equal(t, false, out["power"])
	_, hasFan := out["fan_speed"]
	assert.False(t, hasFan)
}
