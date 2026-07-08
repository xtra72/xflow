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
// Thingplus 테스트용 모의 객체 정의
// ---------------------------------------------------------------------------

// mockThingplusAgent 는 MessageReceiver + SubscriberAgent + Process 진입점을 모두 구현하는
// 테스트용 Agent이다. thingplus-gateway 에이전트의 관련 표면을 모사한다.
type mockThingplusAgent struct {
	// Process (업링크) 관련
	processedData []byte // Process() 호출 시 캡처된 바이트
	processCalls  int    // Process() 호출 횟수
	processErr    error  // Process() 호출 시 반환할 에러

	// ReceiveMessage (다운링크) 관련
	receiveData []byte // ReceiveMessage() 호출 시 반환할 데이터
	receiveErr  error  // ReceiveMessage() 호출 시 반환할 에러

	// Subscribe/Unsubscribe 관련
	subscribedTopics   []string // Subscribe() 호출 시 기록된 토픽
	unsubscribedTopics []string // Unsubscribe() 호출 시 기록된 토픽
	subscribeErr       error    // Subscribe() 호출 시 반환할 에러
}

func (m *mockThingplusAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockThingplusAgent) Start(_ context.Context) error       { return nil }
func (m *mockThingplusAgent) Stop(_ context.Context) error        { return nil }
func (m *mockThingplusAgent) Pause(_ context.Context) error       { return nil }
func (m *mockThingplusAgent) Resume(_ context.Context) error      { return nil }
func (m *mockThingplusAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockThingplusAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockThingplusAgent) ID() string                          { return "mock-thingplus" }
func (m *mockThingplusAgent) Name() string                        { return "mock-thingplus" }
func (m *mockThingplusAgent) Type() string                        { return "thingplus-gateway" }
func (m *mockThingplusAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockThingplusAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockThingplusAgent) Process(data []byte) ([]byte, error) {
	m.processCalls++
	m.processedData = data
	return nil, m.processErr
}

func (m *mockThingplusAgent) ReceiveMessage(_ context.Context) ([]byte, error) {
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	return m.receiveData, nil
}

func (m *mockThingplusAgent) Subscribe(_ context.Context, topics []string) error {
	m.subscribedTopics = topics
	return m.subscribeErr
}

func (m *mockThingplusAgent) Unsubscribe(_ context.Context, topics []string) error {
	m.unsubscribedTopics = topics
	return nil
}

// ---------------------------------------------------------------------------
// Thingplus 테스트용 헬퍼 함수
// ---------------------------------------------------------------------------

// newThingplusNodeDef 는 테스트용 NodeDef를 생성한다.
func newThingplusNodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// newTestThingplusUplinkNode 는 테스트용 ThingplusUplinkNode를 agent를 직접 주입하여 생성한다.
// Init()을 우회하여 인터페이스 타입 검사를 건너뛴다.
func newTestThingplusUplinkNode(mockAgent agent.Agent) *ThingplusUplinkNode {
	def := newThingplusNodeDef("test-uplink", "thingplus-uplink")
	base := NewBaseNode(def)

	n := &ThingplusUplinkNode{
		thingplusNodeBase: thingplusNodeBase{
			BaseNode: base,
			agent:    mockAgent,
		},
	}
	if proc, ok := mockAgent.(agentProcessor); ok {
		n.processor = proc
	}

	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestThingplusDownlinkNode 는 테스트용 ThingplusDownlinkNode를 agent를 직접 주입하여 생성한다.
func newTestThingplusDownlinkNode(mockAgent agent.Agent) *ThingplusDownlinkNode {
	def := newThingplusNodeDef("test-downlink", "thingplus-downlink")
	base := NewBaseNode(def)

	n := &ThingplusDownlinkNode{
		thingplusNodeBase: thingplusNodeBase{
			BaseNode: base,
			agent:    mockAgent,
		},
		sourceCh: make(chan message.Message, thingplusDefaultBufferSize),
		stopCh:   make(chan struct{}),
	}
	if recv, ok := mockAgent.(agent.MessageReceiver); ok {
		n.receiver = recv
	}

	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// ===========================================================================
// 1. 설정 파싱 테스트
// ===========================================================================

// TestThingplusUplink_Configure 는 업링크 노드의 설정 파싱을 검증한다.
func TestThingplusUplink_Configure(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]any
		wantErr    error
		assertFunc func(t *testing.T, n *ThingplusUplinkNode)
	}{
		{
			name:    "정상 설정",
			config:  map[string]any{"agent_ref": "tp-agent"},
			wantErr: nil,
			assertFunc: func(t *testing.T, n *ThingplusUplinkNode) {
				assert.Equal(t, "tp-agent", n.thingplusCfg.AgentRef)
				// 업링크도 기본 버퍼 크기가 설정되나 사용하지 않는다.
				assert.Equal(t, thingplusDefaultBufferSize, n.thingplusCfg.BufferSize)
				// emit_agent 기본 ON.
				assert.True(t, n.thingplusCfg.EmitMetadata.Agent)
			},
		},
		{
			name:    "agent_ref 없음",
			config:  map[string]any{"qos": float64(1)},
			wantErr: ErrThingplusMissingAgentRef,
		},
		{
			name:    "emit_agent 비활성화",
			config:  map[string]any{"agent_ref": "tp-agent", "emit_agent": false},
			wantErr: nil,
			assertFunc: func(t *testing.T, n *ThingplusUplinkNode) {
				assert.False(t, n.thingplusCfg.EmitMetadata.Agent)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newTestThingplusUplinkNode(&mockThingplusAgent{})
			err := n.Configure(tt.config)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
			if tt.assertFunc != nil {
				tt.assertFunc(t, n)
			}
		})
	}
}

// TestThingplusDownlink_Configure 는 다운링크 노드의 설정 파싱을 검증한다.
func TestThingplusDownlink_Configure(t *testing.T) {
	tests := []struct {
		name       string
		config     map[string]any
		wantErr    error
		assertFunc func(t *testing.T, n *ThingplusDownlinkNode)
	}{
		{
			name:    "정상 설정 (topics + buffer_size)",
			config:  map[string]any{"agent_ref": "tp-agent", "topics": []any{"v1/gateway/rpc"}, "buffer_size": float64(128)},
			wantErr: nil,
			assertFunc: func(t *testing.T, n *ThingplusDownlinkNode) {
				assert.Equal(t, "tp-agent", n.thingplusCfg.AgentRef)
				assert.Equal(t, []string{"v1/gateway/rpc"}, n.thingplusCfg.Topics)
				assert.Equal(t, 128, n.thingplusCfg.BufferSize)
				assert.Equal(t, 128, cap(n.sourceCh))
			},
		},
		{
			name:    "topics 없음 → 기본 버퍼",
			config:  map[string]any{"agent_ref": "tp-agent"},
			wantErr: nil,
			assertFunc: func(t *testing.T, n *ThingplusDownlinkNode) {
				assert.Nil(t, n.thingplusCfg.Topics)
				assert.Equal(t, thingplusDefaultBufferSize, n.thingplusCfg.BufferSize)
			},
		},
		{
			name:    "agent_ref 없음",
			config:  map[string]any{"topics": []any{"v1/gateway/rpc"}},
			wantErr: ErrThingplusMissingAgentRef,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := newTestThingplusDownlinkNode(&mockThingplusAgent{})
			err := n.Configure(tt.config)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.True(t, errors.Is(err, tt.wantErr))
				return
			}
			require.NoError(t, err)
			if tt.assertFunc != nil {
				tt.assertFunc(t, n)
			}
		})
	}
}

// ===========================================================================
// 2. 팩토리 테스트
// ===========================================================================

// TestNewThingplusUplinkNode 는 업링크 노드 팩토리를 검증한다.
func TestNewThingplusUplinkNode(t *testing.T) {
	t.Run("정상 생성", func(t *testing.T) {
		def := newThingplusNodeDef("up-1", "thingplus-uplink")
		node, err := NewThingplusUplinkNode(def)
		require.NoError(t, err)
		assert.NotNil(t, node)
	})

	t.Run("WithResolver", func(t *testing.T) {
		mockAgent := &mockThingplusAgent{}
		resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: mockAgent}}
		def := newThingplusNodeDef("up-2", "thingplus-uplink")
		node, err := NewThingplusUplinkNode(def, WithAgentResolver(resolver))
		require.NoError(t, err)
		upNode, ok := node.(*ThingplusUplinkNode)
		require.True(t, ok)
		assert.NotNil(t, upNode.resolver)
	})
}

// TestNewThingplusDownlinkNode 는 다운링크 노드 팩토리를 검증한다.
func TestNewThingplusDownlinkNode(t *testing.T) {
	t.Run("정상 생성", func(t *testing.T) {
		def := newThingplusNodeDef("dn-1", "thingplus-downlink")
		node, err := NewThingplusDownlinkNode(def)
		require.NoError(t, err)
		assert.NotNil(t, node)
	})

	t.Run("WithResolver", func(t *testing.T) {
		mockAgent := &mockThingplusAgent{}
		resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: mockAgent}}
		def := newThingplusNodeDef("dn-2", "thingplus-downlink")
		node, err := NewThingplusDownlinkNode(def, WithAgentResolver(resolver))
		require.NoError(t, err)
		dnNode, ok := node.(*ThingplusDownlinkNode)
		require.True(t, ok)
		assert.NotNil(t, dnNode.resolver)
	})
}

// ===========================================================================
// 3. 업링크 Init 테스트
// ===========================================================================

// TestThingplusUplink_Init_정상 은 Init이 에이전트를 resolve하고 processor를 설정하는지 확인한다.
func TestThingplusUplink_Init_정상(t *testing.T) {
	mockAgent := &mockThingplusAgent{}
	resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: mockAgent}}

	def := newThingplusNodeDef("up-init", "thingplus-uplink")
	node, err := NewThingplusUplinkNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	upNode := node.(*ThingplusUplinkNode)
	require.NoError(t, upNode.Configure(map[string]any{"agent_ref": "tp-agent"}))

	require.NoError(t, upNode.Init(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, upNode.CurrentState())
	assert.NotNil(t, upNode.processor)
}

// TestThingplusUplink_Init_ResolverNil 은 resolver가 nil이면 에러를 반환하는지 확인한다.
func TestThingplusUplink_Init_ResolverNil(t *testing.T) {
	def := newThingplusNodeDef("up-no-resolver", "thingplus-uplink")
	node, err := NewThingplusUplinkNode(def)
	require.NoError(t, err)

	upNode := node.(*ThingplusUplinkNode)
	require.NoError(t, upNode.Configure(map[string]any{"agent_ref": "tp-agent"}))

	err = upNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThingplusNoResolver))
}

// TestThingplusUplink_Init_NoAccessor 는 Transport가 AgentAccessor를 구현하지 않으면
// agent가 nil이 되어 processor 캐스팅에 실패하는지 확인한다.
func TestThingplusUplink_Init_NoAccessor(t *testing.T) {
	resolver := &mockMQTTResolver{transport: &mockMQTTTransportNoAccessor{}}

	def := newThingplusNodeDef("up-no-accessor", "thingplus-uplink")
	node, err := NewThingplusUplinkNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	upNode := node.(*ThingplusUplinkNode)
	require.NoError(t, upNode.Configure(map[string]any{"agent_ref": "tp-agent"}))

	err = upNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThingplusAgentUnsupported))
}

// ===========================================================================
// 4. 업링크 Process 테스트 (핵심: FULL message 를 Type 보존하여 agent.Process 로 전달)
// ===========================================================================

// TestThingplusUplink_Process_ForwardsFullMessage 는 Process가 인입 메시지를
// FULL message.Message JSON(Type 포함)으로 agent.Process 에 전달하고, 통과 출력을 생성하는지 확인한다.
func TestThingplusUplink_Process_ForwardsFullMessage(t *testing.T) {
	mockAgent := &mockThingplusAgent{}
	n := newTestThingplusUplinkNode(mockAgent)

	// 텔레메트리 업링크 메시지.
	in := message.New(message.WithType("thingplus.telemetry"))
	in.Payload().Set("device", "sensor-01")
	in.Payload().Set("temperature", 22.5)

	results, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 에이전트 Process 가 정확히 1회 호출되었고 바이트가 캡처되었는지 확인.
	assert.Equal(t, 1, mockAgent.processCalls)
	require.NotEmpty(t, mockAgent.processedData)

	// 캡처된 바이트가 FULL message.Message JSON 이며 Type 이 보존되는지 확인.
	decoded, err := message.FromJSON(mockAgent.processedData)
	require.NoError(t, err, "agent.Process 는 FULL message.Message JSON 을 받아야 한다")
	assert.Equal(t, "thingplus.telemetry", decoded.Type(), "메시지 Type 이 보존되어야 한다")
	temp, ok := decoded.Payload().Get("temperature")
	require.True(t, ok)
	assert.Equal(t, 22.5, temp)

	// 통과 출력 검증.
	out := results[0]
	assert.Equal(t, "response", out.Type())
	nodeID, ok := out.Metadata().Get("node_id")
	require.True(t, ok)
	assert.NotEmpty(t, nodeID)
}

// TestThingplusUplink_Process_PreservesRPCResponseType 는 RPC 응답 메시지의 Type
// "thingplus.rpc.response" 가 agent.Process 로 전달되는 바이트에 보존되는지 확인한다.
// (Type 이 소실되면 에이전트가 텔레메트리로 오분류한다.)
func TestThingplusUplink_Process_PreservesRPCResponseType(t *testing.T) {
	mockAgent := &mockThingplusAgent{}
	n := newTestThingplusUplinkNode(mockAgent)

	in := message.New(message.WithType("thingplus.rpc.response"))
	in.Payload().Set("device", "sensor-01")
	in.Payload().Set("id", float64(42))
	in.Payload().Set("data", map[string]any{"result": "ok"})

	_, err := n.Process(context.Background(), in)
	require.NoError(t, err)

	require.NotEmpty(t, mockAgent.processedData)
	decoded, err := message.FromJSON(mockAgent.processedData)
	require.NoError(t, err)
	assert.Equal(t, "thingplus.rpc.response", decoded.Type(),
		"RPC 응답 Type 이 보존되어야 에이전트가 응답으로 분기한다")
}

// TestThingplusUplink_Process_AgentError 는 agent.Process 실패 시 에러를 래핑하여 반환하는지 확인한다.
func TestThingplusUplink_Process_AgentError(t *testing.T) {
	mockAgent := &mockThingplusAgent{processErr: errors.New("broker disconnected")}
	n := newTestThingplusUplinkNode(mockAgent)

	in := message.New()
	in.Payload().Set("device", "sensor-01")

	_, err := n.Process(context.Background(), in)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThingplusPublishFailed))
}

// ===========================================================================
// 5. 다운링크 Init 테스트
// ===========================================================================

// TestThingplusDownlink_Init_정상_NoTopics 는 topics 미지정 시 Subscribe 없이 초기화되는지 확인한다.
func TestThingplusDownlink_Init_정상_NoTopics(t *testing.T) {
	mockAgent := &mockThingplusAgent{}
	resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: mockAgent}}

	def := newThingplusNodeDef("dn-init", "thingplus-downlink")
	node, err := NewThingplusDownlinkNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	dnNode := node.(*ThingplusDownlinkNode)
	require.NoError(t, dnNode.Configure(map[string]any{"agent_ref": "tp-agent"}))

	require.NoError(t, dnNode.Init(context.Background()))
	assert.Equal(t, lifecycle.StateRunning, dnNode.CurrentState())
	// topics 미지정 → Subscribe 미호출.
	assert.Nil(t, mockAgent.subscribedTopics)
	assert.False(t, dnNode.subscribed)

	_ = dnNode.Shutdown(context.Background())
}

// TestThingplusDownlink_Init_정상_WithTopics 는 topics 지정 시 Subscribe가 호출되는지 확인한다.
func TestThingplusDownlink_Init_정상_WithTopics(t *testing.T) {
	mockAgent := &mockThingplusAgent{}
	resolver := &mockMQTTResolver{transport: &mockMQTTTransport{agent: mockAgent}}

	def := newThingplusNodeDef("dn-init-topics", "thingplus-downlink")
	node, err := NewThingplusDownlinkNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	dnNode := node.(*ThingplusDownlinkNode)
	require.NoError(t, dnNode.Configure(map[string]any{
		"agent_ref": "tp-agent",
		"topics":    []any{"v1/gateway/rpc"},
	}))

	require.NoError(t, dnNode.Init(context.Background()))
	assert.Equal(t, []string{"v1/gateway/rpc"}, mockAgent.subscribedTopics)
	assert.True(t, dnNode.subscribed)

	// Shutdown 시 Unsubscribe 검증.
	_ = dnNode.Shutdown(context.Background())
	assert.Equal(t, []string{"v1/gateway/rpc"}, mockAgent.unsubscribedTopics)
}

// TestThingplusDownlink_Init_ResolverNil 은 resolver가 nil이면 에러를 반환하는지 확인한다.
func TestThingplusDownlink_Init_ResolverNil(t *testing.T) {
	def := newThingplusNodeDef("dn-no-resolver", "thingplus-downlink")
	node, err := NewThingplusDownlinkNode(def)
	require.NoError(t, err)

	dnNode := node.(*ThingplusDownlinkNode)
	require.NoError(t, dnNode.Configure(map[string]any{"agent_ref": "tp-agent"}))

	err = dnNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThingplusNoResolver))
}

// TestThingplusDownlink_Init_NotReceiver 는 Agent가 MessageReceiver를 구현하지 않으면
// (NoAccessor 로 agent=nil) 에러를 반환하는지 확인한다.
func TestThingplusDownlink_Init_NotReceiver(t *testing.T) {
	resolver := &mockMQTTResolver{transport: &mockMQTTTransportNoAccessor{}}

	def := newThingplusNodeDef("dn-not-receiver", "thingplus-downlink")
	node, err := NewThingplusDownlinkNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	dnNode := node.(*ThingplusDownlinkNode)
	require.NoError(t, dnNode.Configure(map[string]any{"agent_ref": "tp-agent"}))

	err = dnNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrThingplusAgentNotReceiver))
}

// ===========================================================================
// 6. 다운링크 receiveLoop 테스트 (핵심: Type 보존)
// ===========================================================================

// TestThingplusDownlink_ReceiveLoop_PreservesType 는 수신 루프가 방출하는 메시지가
// 원본 Type("thingplus.rpc.request"/"thingplus.attr.update")을 보존하는지 확인한다.
// (mqtt-subscriber 처럼 "event" 로 덮어쓰지 않는다.)
func TestThingplusDownlink_ReceiveLoop_PreservesType(t *testing.T) {
	tests := []struct {
		name     string
		msgType  string
		payload  map[string]any
		metadata map[string]string
	}{
		{
			name:     "RPC 요청 Type 보존",
			msgType:  "thingplus.rpc.request",
			payload:  map[string]any{"device": "sensor-01", "id": float64(7), "method": "setValue"},
			metadata: map[string]string{"device": "sensor-01"},
		},
		{
			name:     "공유 속성 업데이트 Type 보존",
			msgType:  "thingplus.attr.update",
			payload:  map[string]any{"device": "sensor-01", "data": map[string]any{"threshold": float64(30)}},
			metadata: map[string]string{"device": "sensor-01"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 에이전트가 방출하는 FULLY MARSHALED message.Message JSON 을 구성한다.
			opts := []message.Option{message.WithType(tt.msgType), message.WithPayload(message.NewPayload(tt.payload))}
			for k, v := range tt.metadata {
				opts = append(opts, message.WithMetadata(k, v))
			}
			original := message.New(opts...)
			data, err := original.MarshalJSON()
			require.NoError(t, err)

			mockAgent := &mockThingplusAgent{receiveData: data}
			n := newTestThingplusDownlinkNode(mockAgent)

			go n.receiveLoop()

			select {
			case msg := <-n.sourceCh:
				// 핵심: Type 이 보존되어야 한다 ("event" 가 아님).
				assert.Equal(t, tt.msgType, msg.Type(), "다운링크 메시지 Type 이 보존되어야 한다")
				assert.NotEqual(t, "event", msg.Type())

				// payload 보존 확인.
				dev, ok := msg.Payload().Get("device")
				require.True(t, ok)
				assert.Equal(t, "sensor-01", dev)

				// node_id 메타데이터 부여 확인.
				nodeID, ok := msg.Metadata().Get("node_id")
				require.True(t, ok)
				assert.NotEmpty(t, nodeID)
			case <-time.After(3 * time.Second):
				t.Fatal("메시지 수신 타임아웃")
			}

			n.stopOnce.Do(func() { close(n.stopCh) })
		})
	}
}

// TestThingplusDownlink_ReceiveLoop_RawFallback 는 FromJSON 실패 시 raw fallback 타입이
// 부여되는지 확인한다.
func TestThingplusDownlink_ReceiveLoop_RawFallback(t *testing.T) {
	// message.Message JSON 이 아닌 원시 페이로드 (id 필드 없음 → FromJSON 실패).
	mockAgent := &mockThingplusAgent{receiveData: []byte(`{"foo": "bar"}`)}
	n := newTestThingplusDownlinkNode(mockAgent)

	go n.receiveLoop()

	select {
	case msg := <-n.sourceCh:
		assert.Equal(t, thingplusRawMsgType, msg.Type())
		raw, ok := msg.Payload().Get("_raw")
		assert.True(t, ok)
		assert.NotNil(t, raw)
	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}

	n.stopOnce.Do(func() { close(n.stopCh) })
}

// TestThingplusDownlink_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestThingplusDownlink_SourceCh(t *testing.T) {
	n := newTestThingplusDownlinkNode(&mockThingplusAgent{})
	assert.NotNil(t, n.SourceCh())
}

// TestThingplusDownlink_Process_Passthrough 는 SourceNode의 Process가 입력을 그대로 통과시키는지 확인한다.
func TestThingplusDownlink_Process_Passthrough(t *testing.T) {
	n := newTestThingplusDownlinkNode(&mockThingplusAgent{})
	in := message.New(message.WithType("thingplus.rpc.request"))
	results, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, in, results[0])
}

// ===========================================================================
// 8. 공통 기반 (Shutdown / AgentRef) 테스트
// ===========================================================================

// TestThingplusUplink_Shutdown 은 업링크 노드가 Stopping 상태로 종료되는지 확인한다.
func TestThingplusUplink_Shutdown(t *testing.T) {
	n := newTestThingplusUplinkNode(&mockThingplusAgent{})
	require.NoError(t, n.Shutdown(context.Background()))
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// TestThingplus_AgentRef 는 두 노드가 설정된 agent_ref 를 AgentRef()로 노출하는지 확인한다
// (AgentReinitializer 계약).
func TestThingplus_AgentRef(t *testing.T) {
	up := newTestThingplusUplinkNode(&mockThingplusAgent{})
	require.NoError(t, up.Configure(map[string]any{"agent_ref": "tp-agent"}))
	assert.Equal(t, flow.AgentRef{AgentID: "tp-agent", AgentName: "tp-agent"}, up.AgentRef())

	dn := newTestThingplusDownlinkNode(&mockThingplusAgent{})
	require.NoError(t, dn.Configure(map[string]any{"agent_ref": "tp-agent"}))
	assert.Equal(t, flow.AgentRef{AgentID: "tp-agent", AgentName: "tp-agent"}, dn.AgentRef())
}

// ===========================================================================
// 7. 레지스트리 등록 테스트
// ===========================================================================

// TestThingplusRegistry 는 thingplus 노드가 레지스트리에 등록되어 있는지 확인한다.
func TestThingplusRegistry(t *testing.T) {
	r := NewRegistry()

	for _, typeName := range []string{"thingplus-uplink", "thingplus-downlink"} {
		t.Run(typeName, func(t *testing.T) {
			assert.True(t, r.Has(typeName))
			meta, ok := r.TypeMeta(typeName)
			require.True(t, ok)
			assert.Equal(t, "io", meta.Category)
			assert.Equal(t, "builtin", meta.Source)
		})
	}
}
