package node

import (
	"context"
	"encoding/hex"
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

		// data 확인 (hex 문자열)
		data, ok := msg.Payload().Get("data")
		assert.True(t, ok)
		assert.Equal(t, "48454c4c4f5f53455249414c5f44415441", data) // HELLO_SERIAL_DATA의 hex

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
	nodeID, ok := results[0].Metadata().Get("node_id")
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
// 4b. TestSerialOutNode_InputEncoding - input_encoding 설정 테스트
// ---------------------------------------------------------------------------

// TestSerialOutNode_InputEncoding_Hex 는 input_encoding=hex 설정 시 data 문자열이
// 항상 hex 디코딩되며, 유효하지 않은 hex 는 에러를 반환하는지 확인한다.
func TestSerialOutNode_InputEncoding_Hex(t *testing.T) {
	// 유효한 hex
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":      "serial-agent-1",
		"input_encoding": "hex",
	}))

	msg := message.New()
	msg.Payload().Set("data", "5555aa")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, mockAgent.processCalled)
	assert.Equal(t, []byte{0x55, 0x55, 0xaa}, mockAgent.processData)

	// 유효하지 않은 hex → 에러
	mockAgent2 := &mockSerialAgent{}
	n2 := newTestSerialOutNode(mockAgent2)
	require.NoError(t, n2.Configure(map[string]any{
		"agent_ref":      "serial-agent-1",
		"input_encoding": "hex",
	}))

	msg2 := message.New()
	msg2.Payload().Set("data", "xyz")

	_, err = n2.Process(context.Background(), msg2)
	require.Error(t, err)
	assert.False(t, mockAgent2.processCalled, "유효하지 않은 hex 는 전송되면 안 된다")
}

// TestSerialOutNode_InputEncoding_Text 는 input_encoding=text 설정 시 data 문자열이
// hex 로 보이더라도 평문 바이트로 전송되는지 확인한다 (핵심 모호성 해소 테스트).
func TestSerialOutNode_InputEncoding_Text(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":      "serial-agent-1",
		"input_encoding": "text",
	}))

	msg := message.New()
	// "abcdef" 는 유효한 hex 이지만 text 모드에서는 평문 6바이트여야 한다.
	msg.Payload().Set("data", "abcdef")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, mockAgent.processCalled)
	assert.Equal(t, []byte("abcdef"), mockAgent.processData,
		"text 모드에서는 hex 디코딩하지 않고 평문 6바이트여야 한다")
}

// TestSerialOutNode_InputEncoding_Base64 는 input_encoding=base64 설정 시 data 문자열이
// base64 디코딩되는지 확인한다.
func TestSerialOutNode_InputEncoding_Base64(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":      "serial-agent-1",
		"input_encoding": "base64",
	}))

	msg := message.New()
	// "VVWq" 는 0x55,0x55,0xaa 의 base64 표현이다.
	msg.Payload().Set("data", "VVWq")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, mockAgent.processCalled)
	assert.Equal(t, []byte{0x55, 0x55, 0xaa}, mockAgent.processData)
}

// TestSerialOutNode_InputEncoding_Auto_BackwardCompat 는 input_encoding 미설정(또는 auto)
// 시 기존 hex 추론 동작이 그대로 유지되는지 확인한다 (하위호환 회귀 방지).
func TestSerialOutNode_InputEncoding_Auto_BackwardCompat(t *testing.T) {
	// 미설정: hex 로 보이는 문자열은 hex 디코딩 (기존 동작)
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	}))

	msg := message.New()
	msg.Payload().Set("data", "5555aa")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []byte{0x55, 0x55, 0xaa}, mockAgent.processData)

	// 미설정: hex 가 아닌 문자열은 평문 바이트 (기존 동작)
	mockAgent2 := &mockSerialAgent{}
	n2 := newTestSerialOutNode(mockAgent2)
	require.NoError(t, n2.Configure(map[string]any{
		"agent_ref": "serial-agent-1",
	}))

	msg2 := message.New()
	msg2.Payload().Set("data", "hello world")

	_, err = n2.Process(context.Background(), msg2)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello world"), mockAgent2.processData)
}

// TestSerialOutNode_InputEncoding_RawBytesUnaffected 는 raw 가 실제 []byte 로 도착하면
// input_encoding 설정과 무관하게 그대로 전송되는지 확인한다.
func TestSerialOutNode_InputEncoding_RawBytesUnaffected(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":      "serial-agent-1",
		"input_encoding": "text",
	}))

	msg := message.New()
	msg.Payload().Set("raw", []byte{0x55, 0x55})

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, mockAgent.processCalled)
	assert.Equal(t, []byte{0x55, 0x55}, mockAgent.processData,
		"raw []byte 는 input_encoding 과 무관하게 그대로 전송되어야 한다")
}

// TestSerialOutNode_Configure_RejectsUnknownEncoding 는 알 수 없는 input_encoding 값이
// Configure 단계에서 에러를 반환하는지 확인한다.
func TestSerialOutNode_Configure_RejectsUnknownEncoding(t *testing.T) {
	mockAgent := &mockSerialAgent{}
	n := newTestSerialOutNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref":      "serial-agent-1",
		"input_encoding": "bogus",
	})

	require.Error(t, err)
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

		port, ok := msg.Metadata().Get("port")
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

// ---------------------------------------------------------------------------
// 버퍼 aliasing 회귀 테스트 (2026-05-14 hotfix)
// ---------------------------------------------------------------------------

// mockSerialReusedBufferAgent 는 receiver/framer 가 재사용 buffer 를 반환하는
// 상황을 재현하는 테스트용 시리얼 Agent 이다. 하나의 backing 배열을 보유하고,
// 매 ReceiveMessage 호출마다 같은 배열을 다음 프레임으로 덮어쓴 뒤 반환한다.
type mockSerialReusedBufferAgent struct {
	buf    []byte   // 모든 호출이 공유하는 단일 backing 배열
	frames [][]byte // 호출 순서대로 buf 에 채워질 프레임들
	idx    int
}

func (m *mockSerialReusedBufferAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockSerialReusedBufferAgent) Start(_ context.Context) error       { return nil }
func (m *mockSerialReusedBufferAgent) Stop(_ context.Context) error        { return nil }
func (m *mockSerialReusedBufferAgent) Pause(_ context.Context) error       { return nil }
func (m *mockSerialReusedBufferAgent) Resume(_ context.Context) error      { return nil }
func (m *mockSerialReusedBufferAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockSerialReusedBufferAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockSerialReusedBufferAgent) ID() string                          { return "mock-serial-reused" }
func (m *mockSerialReusedBufferAgent) Name() string                        { return "mock-serial-reused" }
func (m *mockSerialReusedBufferAgent) Type() string                        { return "serial" }
func (m *mockSerialReusedBufferAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockSerialReusedBufferAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (m *mockSerialReusedBufferAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

func (m *mockSerialReusedBufferAgent) ReceiveMessage(_ context.Context) ([]byte, error) {
	if m.idx >= len(m.frames) {
		return nil, context.DeadlineExceeded
	}
	frame := m.frames[m.idx]
	m.idx++
	// 공유 배열을 다음 프레임으로 덮어쓴다 (재사용 buffer 시맨틱).
	for i := range m.buf {
		m.buf[i] = 0
	}
	copy(m.buf, frame)
	return m.buf[:len(frame)], nil
}

// TestSerialInNode_ReceiveLoop_RawNotAliased 는 receiver 가 재사용 buffer 를
// 반환하더라도 이미 전달된 메시지의 "raw" 페이로드가 이후 read 에 의해
// 변조되지 않는지 검증한다.
//
// 수정 전(버그): receiveLoop 가 data 슬라이스를 그대로 "raw" 에 저장하므로
// 두 번째 read 가 같은 backing 배열을 덮어쓰면서 첫 메시지의 raw 가 변조된다.
// 수정 후: raw 를 방어적으로 복사하므로 첫 메시지의 raw 는 frame A 를 유지한다.
func TestSerialInNode_ReceiveLoop_RawNotAliased(t *testing.T) {
	frameA := []byte{0x55, 0x55, 0x01, 0x02, 0x03}
	frameB := []byte{0xAA, 0xBB, 0xCC, 0xDD, 0xEE}

	mockAgent := &mockSerialReusedBufferAgent{
		buf:    make([]byte, 8),
		frames: [][]byte{frameA, frameB},
	}
	n := newTestSerialInNode(mockAgent)
	go n.receiveLoop()
	defer close(n.stopCh)

	var msgs []message.Message
	for i := 0; i < 2; i++ {
		select {
		case msg := <-n.sourceCh:
			msgs = append(msgs, msg)
		case <-time.After(3 * time.Second):
			t.Fatalf("메시지 %d 수신 타임아웃", i)
		}
	}

	// 첫 메시지의 raw 는 frame A 여야 한다 (frame B 로 변조되면 안 된다).
	raw0, ok := msgs[0].Payload().Get("raw")
	require.True(t, ok, "메시지 0 에 raw 페이로드가 있어야 한다")
	assert.Equal(t, frameA, raw0,
		"메시지 0 의 raw 는 frame A 를 유지해야 한다 (재사용 buffer aliasing 금지)")

	// data(hex 문자열) 와 raw 는 동일 프레임을 가리켜야 한다.
	data0, ok := msgs[0].Payload().Get("data")
	require.True(t, ok)
	assert.Equal(t, hex.EncodeToString(frameA), data0)
	raw0Bytes, _ := raw0.([]byte)
	assert.Equal(t, hex.EncodeToString(raw0Bytes), data0,
		"raw 와 data 는 동일 프레임이어야 한다")

	// 두 번째 메시지는 frame B 여야 한다.
	raw1, ok := msgs[1].Payload().Get("raw")
	require.True(t, ok)
	assert.Equal(t, frameB, raw1, "메시지 1 의 raw 는 frame B 여야 한다")
}

// TestSerialInNode_RawReceiveLoop_RawNotAliased 는 rawReceiveLoop 경로에서도
// 재사용 buffer 가 채널로 전달될 때 이미 전달된 메시지의 raw 가 변조되지
// 않는지 검증한다.
//
// 수정 전(버그): rawReceiveLoop 가 채널에서 받은 data 를 그대로 "raw" 에
// 저장하므로, 송신측이 같은 backing 배열을 재사용하면 변조된다.
// 수정 후: raw 를 방어적으로 복사하므로 첫 메시지의 raw 는 유지된다.
func TestSerialInNode_RawReceiveLoop_RawNotAliased(t *testing.T) {
	frameA := []byte{0x55, 0x55, 0x11, 0x22}
	frameB := []byte{0xAA, 0xAA, 0x33, 0x44}

	rawCh := make(chan []byte, 1)
	mockAgent := &mockSerialAgentWithRaw{
		mockSerialAgent: mockSerialAgent{},
		rawCh:           rawCh,
	}
	n := newTestSerialInNode(mockAgent)
	n.rawSourceCh = make(chan message.Message, serialDefaultBufferSize)
	go n.rawReceiveLoop(rawCh)
	defer close(n.stopCh)

	// 송신측이 재사용하는 공유 backing 배열.
	shared := make([]byte, 8)

	// frame A 를 공유 배열에 채워 전송.
	copy(shared, frameA)
	rawCh <- shared[:len(frameA)]

	var msg0 message.Message
	select {
	case msg0 = <-n.rawSourceCh:
	case <-time.After(3 * time.Second):
		t.Fatal("메시지 0 수신 타임아웃")
	}

	// 같은 공유 배열을 frame B 로 덮어쓰고 다시 전송.
	for i := range shared {
		shared[i] = 0
	}
	copy(shared, frameB)
	rawCh <- shared[:len(frameB)]

	select {
	case <-n.rawSourceCh:
	case <-time.After(3 * time.Second):
		t.Fatal("메시지 1 수신 타임아웃")
	}

	// 첫 메시지의 raw 는 여전히 frame A 여야 한다.
	raw0, ok := msg0.Payload().Get("raw")
	require.True(t, ok)
	assert.Equal(t, frameA, raw0,
		"rawReceiveLoop: 메시지 0 의 raw 는 frame A 를 유지해야 한다")
}
