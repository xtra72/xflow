package node

import (
	"context"
	"encoding/json"
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
// 시리얼 I/O 테스트용 모의 객체 정의
// ---------------------------------------------------------------------------

// mockSerialAgent 는 Agent + MessageReceiver를 모두 구현하는 테스트용 Agent이다.
type mockSerialAgent struct {
	receiveData []byte // ReceiveMessage() 호출 시 반환할 데이터
	receiveErr  error  // ReceiveMessage() 호출 시 반환할 에러

	processData   []byte // Process() 호출 시 기록된 데이터
	processErr    error  // Process() 호출 시 반환할 에러
	processCalled bool   // Process() 호출 여부
}

func (m *mockSerialAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockSerialAgent) Start(_ context.Context) error       { return nil }
func (m *mockSerialAgent) Stop(_ context.Context) error        { return nil }
func (m *mockSerialAgent) Pause(_ context.Context) error       { return nil }
func (m *mockSerialAgent) Resume(_ context.Context) error      { return nil }
func (m *mockSerialAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockSerialAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockSerialAgent) ID() string                          { return "mock-serial" }
func (m *mockSerialAgent) Name() string                        { return "mock-serial" }
func (m *mockSerialAgent) Type() string                        { return "serial" }
func (m *mockSerialAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockSerialAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockSerialAgent) Process(data []byte) ([]byte, error) {
	m.processCalled = true
	m.processData = data
	return nil, m.processErr
}

func (m *mockSerialAgent) ReceiveMessage(_ context.Context) ([]byte, error) {
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	return m.receiveData, nil
}

// mockSerialAgentWithRaw 는 Agent + MessageReceiver + RawMessageReceiver를 모두 구현하는 테스트용 Agent이다.
type mockSerialAgentWithRaw struct {
	mockSerialAgent
	rawCh chan []byte
}

func (m *mockSerialAgentWithRaw) ReceiveRawMessage() <-chan []byte {
	return m.rawCh
}

// mockSerialPlainAgent 는 기본 Agent만 구현하고 MessageReceiver는 구현하지 않는 테스트용 Agent이다.
type mockSerialPlainAgent struct{}

func (m *mockSerialPlainAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockSerialPlainAgent) Start(_ context.Context) error       { return nil }
func (m *mockSerialPlainAgent) Stop(_ context.Context) error        { return nil }
func (m *mockSerialPlainAgent) Pause(_ context.Context) error       { return nil }
func (m *mockSerialPlainAgent) Resume(_ context.Context) error      { return nil }
func (m *mockSerialPlainAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockSerialPlainAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockSerialPlainAgent) ID() string                          { return "mock-serial-plain" }
func (m *mockSerialPlainAgent) Name() string                        { return "mock-serial-plain" }
func (m *mockSerialPlainAgent) Type() string                        { return "serial" }
func (m *mockSerialPlainAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockSerialPlainAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (m *mockSerialPlainAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

// mockSerialResolver 는 테스트용 AgentResolver 구현이다.
type mockSerialResolver struct {
	transport AgentTransport
	err       error
}

func (m *mockSerialResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockSerialTransport 는 AgentTransport + AgentAccessor를 구현하는 테스트용 모의 객체이다.
type mockSerialTransport struct {
	agent agent.Agent
}

func (m *mockSerialTransport) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockSerialTransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockSerialTransport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockSerialTransportNoAccessor 는 AgentAccessor를 구현하지 않는 AgentTransport이다.
type mockSerialTransportNoAccessor struct{}

func (m *mockSerialTransportNoAccessor) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockSerialTransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// ---------------------------------------------------------------------------
// 시리얼 I/O 테스트용 헬퍼 함수
// ---------------------------------------------------------------------------

// newSerialNodeDef 는 테스트용 NodeDef를 생성한다.
func newSerialNodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// newTestSerialInNode 는 테스트용 SerialInNode를 agent를 직접 주입하여 생성한다.
// Init()을 우회하여 인터페이스 타입 검사를 건너뛴다.
func newTestSerialInNode(mockAgent agent.Agent) *SerialInNode {
	def := newSerialNodeDef("test-serial-in", "serial-in")
	base := NewBaseNode(def)

	n := &SerialInNode{
		serialNodeBase: serialNodeBase{
			BaseNode: base,
			agent:    mockAgent,
		},
		sourceCh: make(chan message.Message, serialDefaultBufferSize),
		stopCh:   make(chan struct{}),
	}

	// MessageReceiver 인터페이스 캐스팅
	if recv, ok := mockAgent.(agent.MessageReceiver); ok {
		n.receiver = recv
	}

	// Running 상태로 전이
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestSerialOutNode 는 테스트용 SerialOutNode를 agent를 직접 주입하여 생성한다.
func newTestSerialOutNode(mockAgent agent.Agent) *SerialOutNode {
	def := newSerialNodeDef("test-serial-out", "serial-out")
	base := NewBaseNode(def)

	n := &SerialOutNode{
		serialNodeBase: serialNodeBase{
			BaseNode: base,
			agent:    mockAgent,
		},
	}

	// Running 상태로 전이
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// ===========================================================================
// SerialInNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewSerialInNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewSerialInNode_정상생성 은 SerialInNode가 올바르게 생성되는지 확인한다.
func TestNewSerialInNode_정상생성(t *testing.T) {
	def := newSerialNodeDef("serial-in-1", "serial-in")
	node, err := NewSerialInNode(def)

	require.NoError(t, err)
	assert.NotNil(t, node)
}

// TestNewSerialInNode_WithResolver 는 AgentResolver 옵션이 적용되는지 확인한다.
func TestNewSerialInNode_WithResolver(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	resolver := &mockSerialResolver{
		transport: &mockSerialTransport{agent: mockAgent},
	}

	def := newSerialNodeDef("serial-in-2", "serial-in")
	node, err := NewSerialInNode(def, WithAgentResolver(resolver))

	require.NoError(t, err)
	inNode, ok := node.(*SerialInNode)
	require.True(t, ok)
	assert.NotNil(t, inNode.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestSerialInNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestSerialInNode_Configure_정상 은 올바른 설정이 적용되는지 확인한다.
func TestSerialInNode_Configure_정상(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialInNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "serial-agent-1", n.serialCfg.AgentRef)
}

// TestSerialInNode_Configure_AgentRef없음 은 agent_ref가 없으면 에러를 반환하는지 확인한다.
func TestSerialInNode_Configure_AgentRef없음(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialInNode(mockAgent)

	err := n.Configure(map[string]any{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSerialMissingAgentRef))
}

// ---------------------------------------------------------------------------
// 3. TestSerialInNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestSerialInNode_Init_정상 은 Init이 정상적으로 에이전트를 resolve하고 Running 상태로 전이하는지 확인한다.
func TestSerialInNode_Init_정상(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	resolver := &mockSerialResolver{
		transport: &mockSerialTransport{agent: mockAgent},
	}

	def := newSerialNodeDef("serial-in-init", "serial-in")
	node, err := NewSerialInNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	inNode := node.(*SerialInNode)
	err = inNode.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = inNode.Init(ctx)
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateRunning, inNode.CurrentState())

	// 정리
	_ = inNode.Shutdown(ctx)
}

// TestSerialInNode_Init_ResolverNil 은 resolver가 nil이면 에러를 반환하는지 확인한다.
func TestSerialInNode_Init_ResolverNil(t *testing.T) {
	def := newSerialNodeDef("serial-in-no-resolver", "serial-in")
	node, err := NewSerialInNode(def)
	require.NoError(t, err)

	inNode := node.(*SerialInNode)
	err = inNode.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	})
	require.NoError(t, err)

	err = inNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSerialNoResolver))
}

// TestSerialInNode_Init_NotReceiver 는 Agent가 MessageReceiver를 구현하지 않으면 에러를 반환하는지 확인한다.
func TestSerialInNode_Init_NotReceiver(t *testing.T) {
	plainAgent := &mockSerialPlainAgent{}
	resolver := &mockSerialResolver{
		transport: &mockSerialTransport{agent: plainAgent},
	}

	def := newSerialNodeDef("serial-in-not-receiver", "serial-in")
	node, err := NewSerialInNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	inNode := node.(*SerialInNode)
	err = inNode.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	})
	require.NoError(t, err)

	err = inNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSerialAgentNotReceiver))
}

// ---------------------------------------------------------------------------
// 4. TestSerialInNode_SourceCh - SourceNode 테스트
// ---------------------------------------------------------------------------

// TestSerialInNode_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestSerialInNode_SourceCh(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialInNode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// ---------------------------------------------------------------------------
// 5. TestSerialInNode_ReceiveLoop - 수신 루프 테스트
// ---------------------------------------------------------------------------

// TestSerialInNode_ReceiveLoop_정상 은 receiveLoop가 시리얼 데이터를 수신하여 sourceCh에 전달하는지 확인한다.
func TestSerialInNode_ReceiveLoop_정상(t *testing.T) {
	mockAgent := &mockSerialAgent{
		receiveData: []byte("HELLO_SERIAL_DATA"),
	}
	n := newTestSerialInNode(mockAgent)

	// 수신 루프 시작
	go n.receiveLoop()

	// 메시지 수신 대기
	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)

		// raw 바이트 확인
		raw, ok := msg.Payload().Get("raw")
		assert.True(t, ok)
		assert.Equal(t, []byte("HELLO_SERIAL_DATA"), raw)

		// data 문자열 확인
		data, ok := msg.Payload().Get("data")
		assert.True(t, ok)
		assert.Equal(t, "HELLO_SERIAL_DATA", data)

		// 메타데이터 확인
		nodeID, ok := msg.Metadata().Get("serial.node_id")
		assert.True(t, ok)
		assert.NotEmpty(t, nodeID)

	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}

	// 정리
	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 6. TestSerialInNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestSerialInNode_Shutdown_정상 은 Shutdown이 정상적으로 종료하는지 확인한다.
func TestSerialInNode_Shutdown_정상(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialInNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// SerialOutNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewSerialOutNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewSerialOutNode_정상생성 은 SerialOutNode가 올바르게 생성되는지 확인한다.
func TestNewSerialOutNode_정상생성(t *testing.T) {
	def := newSerialNodeDef("serial-out-1", "serial-out")
	node, err := NewSerialOutNode(def)

	require.NoError(t, err)
	assert.NotNil(t, node)
}

// ---------------------------------------------------------------------------
// 2. TestSerialOutNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestSerialOutNode_Configure_정상 은 올바른 설정이 적용되는지 확인한다.
func TestSerialOutNode_Configure_정상(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "serial-agent-1", n.serialCfg.AgentRef)
}

// TestSerialOutNode_Configure_AgentRef없음 은 agent_ref가 없으면 에러를 반환하는지 확인한다.
func TestSerialOutNode_Configure_AgentRef없음(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	err := n.Configure(map[string]any{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrSerialMissingAgentRef))
}

// ---------------------------------------------------------------------------
// 3. TestSerialOutNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestSerialOutNode_Init_정상 은 Init이 정상적으로 에이전트를 resolve하고 Running 상태로 전이하는지 확인한다.
func TestSerialOutNode_Init_정상(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	resolver := &mockSerialResolver{
		transport: &mockSerialTransport{agent: mockAgent},
	}

	def := newSerialNodeDef("serial-out-init", "serial-out")
	node, err := NewSerialOutNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	outNode := node.(*SerialOutNode)
	err = outNode.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	})
	require.NoError(t, err)

	err = outNode.Init(context.Background())
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateRunning, outNode.CurrentState())
}

// ---------------------------------------------------------------------------
// 4. TestSerialOutNode_Process - 시리얼 송신 테스트
// ---------------------------------------------------------------------------

// TestSerialOutNode_Process_Raw바이트 는 raw 바이트 페이로드가 agent.Process로 전달되는지 확인한다.
func TestSerialOutNode_Process_Raw바이트(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("raw", []byte{0x01, 0x02, 0x03, 0xFF})

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// agent.Process에 raw 바이트가 전달되었는지 확인
	assert.True(t, mockAgent.processCalled)
	assert.Equal(t, []byte{0x01, 0x02, 0x03, 0xFF}, mockAgent.processData)

	// 출력 메시지 메타데이터 확인
	nodeID, ok := results[0].Metadata().Get("serial.node_id")
	assert.True(t, ok)
	assert.NotEmpty(t, nodeID)
}

// TestSerialOutNode_Process_Data문자열 은 data 문자열 페이로드가 agent.Process로 전달되는지 확인한다.
func TestSerialOutNode_Process_Data문자열(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("data", "AT+RST\r\n")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// agent.Process에 문자열 바이트가 전달되었는지 확인
	assert.True(t, mockAgent.processCalled)
	assert.Equal(t, []byte("AT+RST\r\n"), mockAgent.processData)
}

// TestSerialOutNode_Process_JSON 은 raw와 data가 없을 때 JSON 직렬화하여 전송하는지 확인한다.
func TestSerialOutNode_Process_JSON(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("temperature", 25.5)
	msg.Payload().Set("humidity", 60.0)

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// agent.Process에 JSON 바이트가 전달되었는지 확인
	assert.True(t, mockAgent.processCalled)
	assert.NotEmpty(t, mockAgent.processData)

	// JSON 파싱 검증
	var parsed map[string]any
	err = json.Unmarshal(mockAgent.processData, &parsed)
	require.NoError(t, err)
	assert.Equal(t, 25.5, parsed["temperature"])
	assert.Equal(t, 60.0, parsed["humidity"])
}

// TestSerialOutNode_Process_SendError 는 agent.Process가 에러를 반환하면 에러가 전파되는지 확인한다.
func TestSerialOutNode_Process_SendError(t *testing.T) {
	mockAgent := &mockSerialAgent{
		processErr: errors.New("serial port disconnected"),
	}
	n := newTestSerialOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("data", "test-data")

	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "serial-out: send failed")
}

// ---------------------------------------------------------------------------
// 5. TestSerialOutNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestSerialOutNode_Shutdown_정상 은 Shutdown이 정상적으로 종료하는지 확인한다.
func TestSerialOutNode_Shutdown_정상(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// 레지스트리 등록 테스트
// ===========================================================================

// TestSerialRegistry_SerialIn 는 serial-in이 레지스트리에 등록되어 있는지 확인한다.
func TestSerialRegistry_SerialIn(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("serial-in"))

	meta, ok := r.TypeMeta("serial-in")
	assert.True(t, ok)
	assert.Equal(t, "io", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
}

// TestSerialRegistry_SerialOut 는 serial-out이 레지스트리에 등록되어 있는지 확인한다.
func TestSerialRegistry_SerialOut(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("serial-out"))

	meta, ok := r.TypeMeta("serial-out")
	assert.True(t, ok)
	assert.Equal(t, "io", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
}

// ===========================================================================
// SerialInNode raw_out 포트 테스트
// ===========================================================================

// TestSerialInNode_MultiSourceNode_인터페이스 는 SerialInNode가 MultiSourceNode를 구현하는지 확인한다.
func TestSerialInNode_MultiSourceNode_인터페이스(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialInNode(mockAgent)

	var _ MultiSourceNode = n // 컴파일 타임 체크
}

// TestSerialInNode_ExtraSourceChannels_비활성 은 Agent가 RawMessageReceiver를 구현하지 않으면
// ExtraSourceChannels가 nil을 반환하는지 확인한다.
func TestSerialInNode_ExtraSourceChannels_비활성(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialInNode(mockAgent)

	channels := n.ExtraSourceChannels()
	assert.Nil(t, channels)
}

// TestSerialInNode_ExtraSourceChannels_활성 은 Agent가 RawMessageReceiver를 구현하면
// ExtraSourceChannels가 "raw_out" 채널을 포함하는지 확인한다.
func TestSerialInNode_ExtraSourceChannels_활성(t *testing.T) {
	rawCh := make(chan []byte, 10)
	mockAgent := &mockSerialAgentWithRaw{
		mockSerialAgent: mockSerialAgent{
			receiveData: []byte("DATA"),
		},
		rawCh: rawCh,
	}
	n := newTestSerialInNode(mockAgent)
	// rawSourceCh 수동 활성화 (Init 우회 시)
	n.rawSourceCh = make(chan message.Message, serialDefaultBufferSize)

	channels := n.ExtraSourceChannels()
	assert.NotNil(t, channels)
	_, ok := channels["raw_out"]
	assert.True(t, ok, "raw_out 채널이 있어야 한다")
}

// TestSerialInNode_RawReceiveLoop 는 rawReceiveLoop가 원시 바이트를 메시지로 변환하여
// rawSourceCh에 전달하는지 확인한다.
func TestSerialInNode_RawReceiveLoop(t *testing.T) {
	rawCh := make(chan []byte, 10)
	mockAgent := &mockSerialAgentWithRaw{
		mockSerialAgent: mockSerialAgent{
			receiveData: []byte("FRAMED"),
		},
		rawCh: rawCh,
	}
	n := newTestSerialInNode(mockAgent)
	n.rawSourceCh = make(chan message.Message, serialDefaultBufferSize)

	// rawReceiveLoop 시작
	go n.rawReceiveLoop(rawCh)

	// 원시 데이터 전송
	rawCh <- []byte{0x02, 0x05, 0x01, 0x02, 0x03, 0x03}

	// 메시지 수신 대기
	select {
	case msg := <-n.rawSourceCh:
		assert.NotNil(t, msg)

		raw, ok := msg.Payload().Get("raw")
		assert.True(t, ok)
		assert.Equal(t, []byte{0x02, 0x05, 0x01, 0x02, 0x03, 0x03}, raw)

		port, ok := msg.Metadata().Get("serial.port")
		assert.True(t, ok)
		assert.Equal(t, "raw_out", port)

	case <-time.After(3 * time.Second):
		t.Fatal("rawReceiveLoop 메시지 수신 타임아웃")
	}

	// 종료
	close(n.stopCh)
}

// TestSerialInNode_RawReceiveLoop_종료 는 stopCh가 닫히면 rawReceiveLoop가 종료되는지 확인한다.
func TestSerialInNode_RawReceiveLoop_종료(t *testing.T) {
	rawCh := make(chan []byte, 10)
	mockAgent := &mockSerialAgentWithRaw{
		mockSerialAgent: mockSerialAgent{},
		rawCh:           rawCh,
	}
	n := newTestSerialInNode(mockAgent)
	n.rawSourceCh = make(chan message.Message, serialDefaultBufferSize)

	done := make(chan struct{})
	go func() {
		n.rawReceiveLoop(rawCh)
		close(done)
	}()

	// stopCh 닫아서 루프 종료
	close(n.stopCh)

	select {
	case <-done:
		// 정상 종료
	case <-time.After(2 * time.Second):
		t.Fatal("rawReceiveLoop가 종료되지 않았다")
	}
}

// TestSerialInNode_Init_RawMessageReceiver 는 Init 시 Agent가 RawMessageReceiver를 구현하면
// rawSourceCh가 활성화되는지 확인한다.
func TestSerialInNode_Init_RawMessageReceiver(t *testing.T) {
	rawCh := make(chan []byte, 10)
	mockAgent := &mockSerialAgentWithRaw{
		mockSerialAgent: mockSerialAgent{
			receiveData: []byte("DATA"),
		},
		rawCh: rawCh,
	}
	resolver := &mockSerialResolver{
		transport: &mockSerialTransport{agent: mockAgent},
	}

	def := newSerialNodeDef("test-raw-in", "serial-in")
	node, err := NewSerialInNode(def)
	require.NoError(t, err)

	inNode := node.(*SerialInNode)
	inNode.resolver = resolver
	err = inNode.Configure(map[string]any{
		"agent_ref": "serial-agent-raw",
	})
	require.NoError(t, err)

	err = inNode.Init(context.Background())
	require.NoError(t, err)

	// rawSourceCh가 활성화되었는지 확인
	assert.NotNil(t, inNode.rawSourceCh)

	// ExtraSourceChannels가 raw_out 채널을 반환하는지 확인
	channels := inNode.ExtraSourceChannels()
	assert.NotNil(t, channels)
	_, ok := channels["raw_out"]
	assert.True(t, ok, "raw_out 채널이 활성화되어야 한다")

	// 원시 데이터를 보내면 rawSourceCh로 도착하는지 확인
	rawCh <- []byte{0xAA, 0xBB}

	select {
	case msg := <-inNode.rawSourceCh:
		raw, ok := msg.Payload().Get("raw")
		assert.True(t, ok)
		assert.Equal(t, []byte{0xAA, 0xBB}, raw)
	case <-time.After(3 * time.Second):
		t.Fatal("Init 후 raw_out 수신 타임아웃")
	}

	// 종료
	_ = inNode.Shutdown(context.Background())
}
