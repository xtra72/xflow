package node

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// MQTT 테스트용 모의 객체 정의
// ---------------------------------------------------------------------------

// mockMQTTAgent 는 SubscriberAgent + MessageReceiver + MessagePublisher를 모두 구현하는 테스트용 Agent이다.
type mockMQTTAgent struct {
	subscribedTopics   []string // Subscribe() 호출 시 기록된 토픽
	unsubscribedTopics []string // Unsubscribe() 호출 시 기록된 토픽
	subscribeErr       error    // Subscribe() 호출 시 반환할 에러
	unsubscribeErr     error    // Unsubscribe() 호출 시 반환할 에러

	receiveData []byte // ReceiveMessage() 호출 시 반환할 데이터
	receiveErr  error  // ReceiveMessage() 호출 시 반환할 에러

	publishedTopic    string // PublishMessage() 호출 시 기록된 토픽
	publishedQoS      byte   // PublishMessage() 호출 시 기록된 QoS
	publishedRetained bool   // PublishMessage() 호출 시 기록된 Retained
	publishedPayload  []byte // PublishMessage() 호출 시 기록된 페이로드
	publishErr        error  // PublishMessage() 호출 시 반환할 에러
}

func (m *mockMQTTAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockMQTTAgent) Start(_ context.Context) error       { return nil }
func (m *mockMQTTAgent) Stop(_ context.Context) error        { return nil }
func (m *mockMQTTAgent) Pause(_ context.Context) error       { return nil }
func (m *mockMQTTAgent) Resume(_ context.Context) error      { return nil }
func (m *mockMQTTAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockMQTTAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockMQTTAgent) ID() string                          { return "mock-mqtt" }
func (m *mockMQTTAgent) Name() string                        { return "mock-mqtt" }
func (m *mockMQTTAgent) Type() string                        { return "mqtt-client" }
func (m *mockMQTTAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockMQTTAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (m *mockMQTTAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

func (m *mockMQTTAgent) Subscribe(_ context.Context, topics []string) error {
	m.subscribedTopics = topics
	return m.subscribeErr
}

func (m *mockMQTTAgent) Unsubscribe(_ context.Context, topics []string) error {
	m.unsubscribedTopics = topics
	return m.unsubscribeErr
}

func (m *mockMQTTAgent) ReceiveMessage(_ context.Context) ([]byte, error) {
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	return m.receiveData, nil
}

func (m *mockMQTTAgent) PublishMessage(topic string, qos byte, retained bool, payload []byte) error {
	m.publishedTopic = topic
	m.publishedQoS = qos
	m.publishedRetained = retained
	m.publishedPayload = payload
	return m.publishErr
}

// mockPublishOnlyAgent 는 MessagePublisher만 구현하는 테스트용 Agent이다.
// SubscriberAgent/MessageReceiver를 구현하지 않는다.
type mockPublishOnlyAgent struct {
	publishedTopic   string
	publishedQoS     byte
	publishedPayload []byte
	publishErr       error
}

func (m *mockPublishOnlyAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockPublishOnlyAgent) Start(_ context.Context) error       { return nil }
func (m *mockPublishOnlyAgent) Stop(_ context.Context) error        { return nil }
func (m *mockPublishOnlyAgent) Pause(_ context.Context) error       { return nil }
func (m *mockPublishOnlyAgent) Resume(_ context.Context) error      { return nil }
func (m *mockPublishOnlyAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockPublishOnlyAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockPublishOnlyAgent) ID() string                          { return "mock-pub-only" }
func (m *mockPublishOnlyAgent) Name() string                        { return "mock-pub-only" }
func (m *mockPublishOnlyAgent) Type() string                        { return "mqtt-client" }
func (m *mockPublishOnlyAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockPublishOnlyAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (m *mockPublishOnlyAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

func (m *mockPublishOnlyAgent) PublishMessage(topic string, qos byte, _ bool, payload []byte) error {
	m.publishedTopic = topic
	m.publishedQoS = qos
	m.publishedPayload = payload
	return m.publishErr
}

// mockPlainAgent 는 기본 Agent만 구현하고 MQTT 관련 인터페이스는 구현하지 않는 테스트용 Agent이다.
type mockPlainAgent struct{}

func (m *mockPlainAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockPlainAgent) Start(_ context.Context) error       { return nil }
func (m *mockPlainAgent) Stop(_ context.Context) error        { return nil }
func (m *mockPlainAgent) Pause(_ context.Context) error       { return nil }
func (m *mockPlainAgent) Resume(_ context.Context) error      { return nil }
func (m *mockPlainAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockPlainAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockPlainAgent) ID() string                          { return "mock-plain" }
func (m *mockPlainAgent) Name() string                        { return "mock-plain" }
func (m *mockPlainAgent) Type() string                        { return "generic" }
func (m *mockPlainAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockPlainAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (m *mockPlainAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

// mockMQTTResolver 는 테스트용 AgentResolver 구현이다.
type mockMQTTResolver struct {
	transport AgentTransport
	err       error
}

func (m *mockMQTTResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockMQTTTransport 는 AgentTransport + AgentAccessor를 구현하는 테스트용 모의 객체이다.
type mockMQTTTransport struct {
	agent agent.Agent
}

func (m *mockMQTTTransport) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockMQTTTransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockMQTTTransport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockMQTTTransportNoAccessor 는 AgentAccessor를 구현하지 않는 AgentTransport이다.
type mockMQTTTransportNoAccessor struct{}

func (m *mockMQTTTransportNoAccessor) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockMQTTTransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// ---------------------------------------------------------------------------
// MQTT 테스트용 헬퍼 함수
// ---------------------------------------------------------------------------

// newMQTTNodeDef 는 테스트용 NodeDef를 생성한다.
func newMQTTNodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// newTestMQTTSubNode 는 테스트용 MQTTSubNode를 agent를 직접 주입하여 생성한다.
// Init()을 우회하여 인터페이스 타입 검사를 건너뛴다.
func newTestMQTTSubNode(mockAgent agent.Agent) *MQTTSubNode {
	def := newMQTTNodeDef("test-sub", "mqtt-subscriber")
	base := NewBaseNode(def)

	n := &MQTTSubNode{
		mqttNodeBase: mqttNodeBase{
			BaseNode: base,
			agent:    mockAgent,
		},
		sourceCh: make(chan message.Message, mqttDefaultBufferSize),
		stopCh:   make(chan struct{}),
	}

	// 인터페이스 캐스팅 (mockMQTTAgent가 모두 구현)
	if sub, ok := mockAgent.(agent.SubscriberAgent); ok {
		n.subscriber = sub
	}
	if recv, ok := mockAgent.(agent.MessageReceiver); ok {
		n.receiver = recv
	}

	// Running 상태로 전이
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestMQTTPublisherNode 는 테스트용 MQTTPublisherNode를 agent를 직접 주입하여 생성한다.
func newTestMQTTPublisherNode(mockAgent agent.Agent) *MQTTPublisherNode {
	def := newMQTTNodeDef("test-pub", "mqtt-publisher")
	base := NewBaseNode(def)

	n := &MQTTPublisherNode{
		mqttNodeBase: mqttNodeBase{
			BaseNode: base,
			agent:    mockAgent,
		},
	}

	// MessagePublisher 인터페이스 캐스팅
	if pub, ok := mockAgent.(agent.MessagePublisher); ok {
		n.publisher = pub
	}

	// Running 상태로 전이
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// ===========================================================================
// R18: MQTTSubNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewMQTTSubNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewMQTTSubNode_정상생성 은 MQTTSubNode가 올바르게 생성되는지 확인한다.
func TestNewMQTTSubNode_정상생성(t *testing.T) {
	def := newMQTTNodeDef("sub-1", "mqtt-subscriber")
	node, err := NewMQTTSubNode(def)

	require.NoError(t, err)
	assert.NotNil(t, node)
}

// TestNewMQTTSubNode_WithResolver 는 AgentResolver 옵션이 적용되는지 확인한다.
func TestNewMQTTSubNode_WithResolver(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	resolver := &mockMQTTResolver{
		transport: &mockMQTTTransport{agent: mockAgent},
	}

	def := newMQTTNodeDef("sub-2", "mqtt-subscriber")
	node, err := NewMQTTSubNode(def, WithAgentResolver(resolver))

	require.NoError(t, err)
	subNode, ok := node.(*MQTTSubNode)
	require.True(t, ok)
	assert.NotNil(t, subNode.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestMQTTSubNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestMQTTSubNode_Configure_정상 은 올바른 설정이 적용되는지 확인한다.
func TestMQTTSubNode_Configure_정상(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTSubNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
		"topics":    []any{"sensor/+/data", "device/#"},
		"qos":       float64(1),
	})

	require.NoError(t, err)
	assert.Equal(t, "mqtt-agent-1", n.mqttCfg.AgentRef)
	assert.Equal(t, []string{"sensor/+/data", "device/#"}, n.mqttCfg.Topics)
	assert.Equal(t, 1, n.mqttCfg.QoS)
}

// TestMQTTSubNode_Configure_AgentRef없음 은 agent_ref가 없으면 에러를 반환하는지 확인한다.
func TestMQTTSubNode_Configure_AgentRef없음(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTSubNode(mockAgent)

	err := n.Configure(map[string]any{
		"topics": []any{"test/topic"},
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMQTTMissingAgentRef))
}

// TestMQTTSubNode_Configure_Topics_StringSlice 는 []string 타입 토픽이 올바르게 파싱되는지 확인한다.
func TestMQTTSubNode_Configure_Topics_StringSlice(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTSubNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
		"topics":    []string{"topic/a", "topic/b"},
	})

	require.NoError(t, err)
	assert.Equal(t, []string{"topic/a", "topic/b"}, n.mqttCfg.Topics)
}

// ---------------------------------------------------------------------------
// 3. TestMQTTSubNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestMQTTSubNode_Init_정상 은 Init이 정상적으로 에이전트를 resolve하고 구독하는지 확인한다.
func TestMQTTSubNode_Init_정상(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	resolver := &mockMQTTResolver{
		transport: &mockMQTTTransport{agent: mockAgent},
	}

	def := newMQTTNodeDef("sub-init", "mqtt-subscriber")
	node, err := NewMQTTSubNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	subNode := node.(*MQTTSubNode)
	err = subNode.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
		"topics":    []any{"sensor/data"},
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = subNode.Init(ctx)
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateRunning, subNode.CurrentState())
	assert.Equal(t, []string{"sensor/data"}, mockAgent.subscribedTopics)

	// 정리
	_ = subNode.Shutdown(ctx)
}

// TestMQTTSubNode_Init_ResolverNil 은 resolver가 nil이면 에러를 반환하는지 확인한다.
func TestMQTTSubNode_Init_ResolverNil(t *testing.T) {
	def := newMQTTNodeDef("sub-no-resolver", "mqtt-subscriber")
	node, err := NewMQTTSubNode(def)
	require.NoError(t, err)

	subNode := node.(*MQTTSubNode)
	err = subNode.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
	})
	require.NoError(t, err)

	err = subNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMQTTNoResolver))
}

// TestMQTTSubNode_Init_NotSubscriber 는 Agent가 SubscriberAgent를 구현하지 않으면 에러를 반환하는지 확인한다.
func TestMQTTSubNode_Init_NotSubscriber(t *testing.T) {
	plainAgent := &mockPlainAgent{}
	resolver := &mockMQTTResolver{
		transport: &mockMQTTTransport{agent: plainAgent},
	}

	def := newMQTTNodeDef("sub-not-subscriber", "mqtt-subscriber")
	node, err := NewMQTTSubNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	subNode := node.(*MQTTSubNode)
	err = subNode.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
	})
	require.NoError(t, err)

	err = subNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMQTTAgentNotSubscriber))
}

// TestMQTTSubNode_Init_NoAccessor 는 Transport가 AgentAccessor를 구현하지 않으면
// agent가 nil이 되어 SubscriberAgent 캐스팅에 실패하는지 확인한다.
func TestMQTTSubNode_Init_NoAccessor(t *testing.T) {
	resolver := &mockMQTTResolver{
		transport: &mockMQTTTransportNoAccessor{},
	}

	def := newMQTTNodeDef("sub-no-accessor", "mqtt-subscriber")
	node, err := NewMQTTSubNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	subNode := node.(*MQTTSubNode)
	err = subNode.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
	})
	require.NoError(t, err)

	err = subNode.Init(context.Background())
	require.Error(t, err)
	// agent가 nil이므로 SubscriberAgent 캐스팅 실패
	assert.True(t, errors.Is(err, ErrMQTTAgentNotSubscriber))
}

// ---------------------------------------------------------------------------
// 4. TestMQTTSubNode_SourceCh - SourceNode 테스트
// ---------------------------------------------------------------------------

// TestMQTTSubNode_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestMQTTSubNode_SourceCh(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTSubNode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// ---------------------------------------------------------------------------
// 5. TestMQTTSubNode_ReceiveLoop - 수신 루프 테스트
// ---------------------------------------------------------------------------

// TestMQTTSubNode_ReceiveLoop_정상 은 receiveLoop가 메시지를 수신하여 sourceCh에 전달하는지 확인한다.
func TestMQTTSubNode_ReceiveLoop_정상(t *testing.T) {
	mockAgent := &mockMQTTAgent{
		receiveData: []byte(`{"temperature": 25.5, "humidity": 60}`),
	}
	n := newTestMQTTSubNode(mockAgent)

	// 수신 루프 시작
	go n.receiveLoop()

	// 메시지 수신 대기
	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)
		val, ok := msg.Payload().Get("temperature")
		assert.True(t, ok)
		assert.Equal(t, 25.5, val)

		// 메타데이터 확인
		nodeID, ok := msg.Metadata().Get("node_id")
		assert.True(t, ok)
		assert.NotEmpty(t, nodeID)
	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}

	// 정리
	close(n.stopCh)
}

// TestMQTTSubNode_ReceiveLoop_SetsMessageTypeEvent 는 MQTT subscription
// 수신 루프가 emit 한 메시지가 msg.Type()="event" 를 가지는지
// 확인한다 (브로커 push 는 자발적 event 이다).
// 통일 분류 표준: SPEC-MESSAGE-TYPE-001 (1급 채널).
func TestMQTTSubNode_ReceiveLoop_SetsMessageTypeEvent(t *testing.T) {
	mockAgent := &mockMQTTAgent{
		receiveData: []byte(`{"temperature": 25.5}`),
	}
	n := newTestMQTTSubNode(mockAgent)

	go n.receiveLoop()

	select {
	case msg := <-n.sourceCh:
		assert.Equal(t, "event", msg.Type(), "MQTT 구독 메시지는 event 분류여야 한다")

		// 기존 mqtt_node_id 메타데이터도 유지되는지 확인
		nodeID, ok := msg.Metadata().Get("node_id")
		require.True(t, ok)
		assert.NotEmpty(t, nodeID)
	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}

	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 6. TestMQTTSubNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestMQTTSubNode_Shutdown_정상 은 Shutdown이 토픽 구독을 해제하고 종료하는지 확인한다.
func TestMQTTSubNode_Shutdown_정상(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTSubNode(mockAgent)
	n.mqttCfg.Topics = []string{"test/topic"}

	err := n.Shutdown(context.Background())
	require.NoError(t, err)

	assert.Equal(t, []string{"test/topic"}, mockAgent.unsubscribedTopics)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// R19: MQTTPublisherNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewMQTTPublisherNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewMQTTPublisherNode_정상생성 은 MQTTPublisherNode가 올바르게 생성되는지 확인한다.
func TestNewMQTTPublisherNode_정상생성(t *testing.T) {
	def := newMQTTNodeDef("pub-1", "mqtt-publisher")
	node, err := NewMQTTPublisherNode(def)

	require.NoError(t, err)
	assert.NotNil(t, node)
}

// TestNewMQTTPublisherNode_WithResolver 는 AgentResolver 옵션이 적용되는지 확인한다.
func TestNewMQTTPublisherNode_WithResolver(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	resolver := &mockMQTTResolver{
		transport: &mockMQTTTransport{agent: mockAgent},
	}

	def := newMQTTNodeDef("pub-2", "mqtt-publisher")
	node, err := NewMQTTPublisherNode(def, WithAgentResolver(resolver))

	require.NoError(t, err)
	pubNode, ok := node.(*MQTTPublisherNode)
	require.True(t, ok)
	assert.NotNil(t, pubNode.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestMQTTPublisherNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestMQTTPublisherNode_Configure_정상 은 올바른 설정이 적용되는지 확인한다.
func TestMQTTPublisherNode_Configure_정상(t *testing.T) {
	mockAgent := &mockPublishOnlyAgent{}
	n := newTestMQTTPublisherNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref":     "mqtt-agent-1",
		"qos":           float64(2),
		"retained":      true,
		"publish_topic": "output/{device_id}/status",
	})

	require.NoError(t, err)
	assert.Equal(t, "mqtt-agent-1", n.mqttCfg.AgentRef)
	assert.Equal(t, 2, n.mqttCfg.QoS)
	assert.True(t, n.mqttCfg.Retained)
	assert.Equal(t, "output/{device_id}/status", n.mqttCfg.PublishTopic)
}

// TestMQTTPublisherNode_Configure_AgentRef없음 은 agent_ref가 없으면 에러를 반환하는지 확인한다.
func TestMQTTPublisherNode_Configure_AgentRef없음(t *testing.T) {
	mockAgent := &mockPublishOnlyAgent{}
	n := newTestMQTTPublisherNode(mockAgent)

	err := n.Configure(map[string]any{
		"qos": float64(1),
	})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMQTTMissingAgentRef))
}

// ---------------------------------------------------------------------------
// 3. TestMQTTPublisherNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestMQTTPublisherNode_Init_정상 은 Init이 정상적으로 에이전트를 resolve하는지 확인한다.
func TestMQTTPublisherNode_Init_정상(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	resolver := &mockMQTTResolver{
		transport: &mockMQTTTransport{agent: mockAgent},
	}

	def := newMQTTNodeDef("pub-init", "mqtt-publisher")
	node, err := NewMQTTPublisherNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	pubNode := node.(*MQTTPublisherNode)
	err = pubNode.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
	})
	require.NoError(t, err)

	err = pubNode.Init(context.Background())
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateRunning, pubNode.CurrentState())
}

// TestMQTTPublisherNode_Init_NotPublisher 는 Agent가 MessagePublisher를 구현하지 않으면 에러를 반환하는지 확인한다.
func TestMQTTPublisherNode_Init_NotPublisher(t *testing.T) {
	plainAgent := &mockPlainAgent{}
	resolver := &mockMQTTResolver{
		transport: &mockMQTTTransport{agent: plainAgent},
	}

	def := newMQTTNodeDef("pub-not-publisher", "mqtt-publisher")
	node, err := NewMQTTPublisherNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	pubNode := node.(*MQTTPublisherNode)
	err = pubNode.Configure(map[string]any{
		"agent_ref": "mqtt-agent-1",
	})
	require.NoError(t, err)

	err = pubNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMQTTAgentNotPublisher))
}

// ---------------------------------------------------------------------------
// 4. TestMQTTPublisherNode_Process - 메시지 발행 테스트
// ---------------------------------------------------------------------------

// TestMQTTPublisherNode_Process_정상 은 Process가 메시지를 정상적으로 발행하는지 확인한다.
func TestMQTTPublisherNode_Process_정상(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTPublisherNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("temperature", 25.5)
	msg.Metadata().Set("mqtt.topic", "sensor/1/data")
	msg.Metadata().Set("mqtt.qos", "1")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 발행된 데이터 확인
	assert.Equal(t, "sensor/1/data", mockAgent.publishedTopic)
	assert.Equal(t, byte(1), mockAgent.publishedQoS)
	assert.NotEmpty(t, mockAgent.publishedPayload)

	// 출력 메시지 메타데이터 확인
	topic, ok := results[0].Metadata().Get("mqtt_published_topic")
	assert.True(t, ok)
	assert.Equal(t, "sensor/1/data", topic)
}

// TestMQTTPublisherNode_Process_PublishError 는 발행 실패 시 에러를 반환하는지 확인한다.
func TestMQTTPublisherNode_Process_PublishError(t *testing.T) {
	mockAgent := &mockMQTTAgent{
		publishErr: errors.New("connection lost"),
	}
	n := newTestMQTTPublisherNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("data", "test")
	msg.Metadata().Set("mqtt.topic", "test/topic")

	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrMQTTPublishFailed))
}

// TestMQTTPublisherNode_Process_WithPublishTopic 은 publish_topic 템플릿이 적용되는지 확인한다.
func TestMQTTPublisherNode_Process_WithPublishTopic(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTPublisherNode(mockAgent)
	n.mqttCfg.QoS = 1
	n.mqttCfg.PublishTopic = "output/{device_id}/status"

	msg := message.New()
	msg.Payload().Set("device_id", "dev-001")
	msg.Payload().Set("temperature", 30.0)

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 템플릿 보간된 토픽 확인
	assert.Equal(t, "output/dev-001/status", mockAgent.publishedTopic)
	assert.Equal(t, byte(1), mockAgent.publishedQoS)
}

// ---------------------------------------------------------------------------
// 5. TestMQTTPublisherNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestMQTTPublisherNode_Shutdown_정상 은 Shutdown이 정상적으로 종료하는지 확인한다.
func TestMQTTPublisherNode_Shutdown_정상(t *testing.T) {
	mockAgent := &mockMQTTAgent{}
	n := newTestMQTTPublisherNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// R20: 레지스트리 등록 테스트
// ===========================================================================

// TestMQTTRegistry_MQTTSub 는 mqtt-subscriber가 레지스트리에 등록되어 있는지 확인한다.
func TestMQTTRegistry_MQTTSub(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("mqtt-subscriber"))

	meta, ok := r.TypeMeta("mqtt-subscriber")
	assert.True(t, ok)
	assert.Equal(t, "io", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
}

// TestMQTTRegistry_MQTTPublisher 는 mqtt-publisher가 레지스트리에 등록되어 있는지 확인한다.
func TestMQTTRegistry_MQTTPublisher(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("mqtt-publisher"))

	meta, ok := r.TypeMeta("mqtt-publisher")
	assert.True(t, ok)
	assert.Equal(t, "io", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
}

// TestMQTTRegistry_TotalBuiltins 는 빌트인 노드 타입이 63개인지 확인한다.
//
// 61개 이전 빌트인 + thingplus-uplink / thingplus-downlink (SPEC-THINGPLUS-001, io 카테고리) = 63.
// canonical 추가 항목: chart-emitter + century-hvacr01-status / century-hvacr01-control / century-hvacr01 (3종, raw-frame 통합됨)
// + inventory (SPEC-INVENTORY-001, processing 카테고리)
// + select-field (SPEC-SELECT-FIELD, processing 카테고리)
// + flow-node (SPEC-SUBFLOW-001, composition 카테고리 — 배포 시 확장됨)
// + enrich (message-slim-metadata / enrich, processing 카테고리)
// + thingplus-uplink / thingplus-downlink (SPEC-THINGPLUS-001, io 카테고리)
func TestMQTTRegistry_TotalBuiltins(t *testing.T) {
	r := NewRegistry()
	types := r.Types()
	assert.Equal(t, 63, len(types))
}

// ===========================================================================
// R21: toInt 헬퍼 함수 테스트
// ===========================================================================

// TestToInt 는 toInt 헬퍼 함수가 다양한 타입을 올바르게 변환하는지 확인한다.
func TestToInt(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected int
	}{
		{"int", 42, 42},
		{"float64", float64(3), 3},
		{"int64", int64(100), 100},
		{"string", "not_a_number", 0},
		{"nil", nil, 0},
		{"bool", true, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// ===========================================================================
// R22: mqttInterpolateTemplate 헬퍼 함수 테스트
// ===========================================================================

// TestMQTTInterpolateTemplate 은 템플릿 보간이 올바르게 동작하는지 확인한다.
func TestMQTTInterpolateTemplate(t *testing.T) {
	msg := message.New()
	msg.Payload().Set("device_id", "dev-001")
	msg.Payload().Set("room", "living")
	msg.Payload().Set("state", map[string]any{"power": true, "mode": "cool"})
	msg.Metadata().Set("device_type", "HVACR.IDU")
	msg.Metadata().SetGroup("device", map[string]string{
		"type": "controller",
		"id":   "uuid-abc",
		"name": "Living Room",
	})
	msg.SetType("device_state.change")

	tests := []struct {
		name     string
		template string
		expected string
	}{
		// Legacy: payload 직접 키
		{"단일 치환", "output/{device_id}/status", "output/dev-001/status"},
		{"복수 치환", "{room}/{device_id}", "living/dev-001"},
		{"없는 키", "output/{unknown}/data", "output/{unknown}/data"},
		{"치환 없음", "static/topic", "static/topic"},
		// v0.18.2: JSONPath 지원
		{"$.payload.field", "out/{$.payload.device_id}/x", "out/dev-001/x"},
		{"$.metadata.key", "xflow/{$.metadata.device_type}/status", "xflow/HVACR.IDU/status"},
		{"$.type", "ev/{$.type}", "ev/device_state.change"},
		{"$.payload nested", "x/{$.payload.state.mode}", "x/cool"},
		{"JSONPath 키 없음", "out/{$.metadata.absent}/x", "out/{$.metadata.absent}/x"},
		// nested metadata group path (flat 키 제거 후 대체 경로)
		{"$.metadata.device.type", "xflow/{$.metadata.device.type}/{$.metadata.device.id}/status", "xflow/controller/uuid-abc/status"},
		{"$.metadata.device.name", "out/{$.metadata.device.name}/x", "out/Living Room/x"},
		{"nested group 키 없음", "out/{$.metadata.device.absent}/x", "out/{$.metadata.device.absent}/x"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mqttInterpolateTemplate(tt.template, msg)
			assert.Equal(t, tt.expected, result)
		})
	}
}
