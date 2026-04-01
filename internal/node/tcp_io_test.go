package node

import (
	"context"
	"encoding/base64"
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
// TCP 테스트용 모의 객체 정의
// ---------------------------------------------------------------------------

// mockTCPServerAgent 는 Agent + MessageReceiver + ConnAwareReceiver 를 모두 구현하는 테스트용 TCP 서버 Agent이다.
type mockTCPServerAgent struct {
	receiveData []byte // ReceiveMessage() 호출 시 반환할 데이터
	receiveErr  error  // ReceiveMessage() 호출 시 반환할 에러
	remoteAddr  string // ReceiveMessageFrom() 호출 시 반환할 원격 주소

	processData   []byte // Process() 호출 시 기록된 데이터
	processErr    error  // Process() 호출 시 반환할 에러
	processCalled bool   // Process() 호출 여부
}

func (m *mockTCPServerAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockTCPServerAgent) Start(_ context.Context) error       { return nil }
func (m *mockTCPServerAgent) Stop(_ context.Context) error        { return nil }
func (m *mockTCPServerAgent) Pause(_ context.Context) error       { return nil }
func (m *mockTCPServerAgent) Resume(_ context.Context) error      { return nil }
func (m *mockTCPServerAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockTCPServerAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockTCPServerAgent) ID() string                          { return "mock-tcp-server" }
func (m *mockTCPServerAgent) Name() string                        { return "mock-tcp-server" }
func (m *mockTCPServerAgent) Type() string                        { return "tcp-server" }
func (m *mockTCPServerAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockTCPServerAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockTCPServerAgent) Process(data []byte) ([]byte, error) {
	m.processData = data
	m.processCalled = true
	return nil, m.processErr
}

func (m *mockTCPServerAgent) ReceiveMessage(_ context.Context) ([]byte, error) {
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	return m.receiveData, nil
}

func (m *mockTCPServerAgent) ReceiveMessageFrom(_ context.Context) (data []byte, remoteAddr string, err error) {
	if m.receiveErr != nil {
		return nil, "", m.receiveErr
	}
	return m.receiveData, m.remoteAddr, nil
}

// mockTCPClientAgent 는 Agent + MessageReceiver 만 구현하는 테스트용 TCP 클라이언트 Agent이다.
// ConnAwareReceiver 는 구현하지 않는다.
type mockTCPClientAgent struct {
	receiveData []byte // ReceiveMessage() 호출 시 반환할 데이터
	receiveErr  error  // ReceiveMessage() 호출 시 반환할 에러

	processData   []byte // Process() 호출 시 기록된 데이터
	processErr    error  // Process() 호출 시 반환할 에러
	processCalled bool   // Process() 호출 여부
}

func (m *mockTCPClientAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockTCPClientAgent) Start(_ context.Context) error       { return nil }
func (m *mockTCPClientAgent) Stop(_ context.Context) error        { return nil }
func (m *mockTCPClientAgent) Pause(_ context.Context) error       { return nil }
func (m *mockTCPClientAgent) Resume(_ context.Context) error      { return nil }
func (m *mockTCPClientAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockTCPClientAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockTCPClientAgent) ID() string                          { return "mock-tcp-client" }
func (m *mockTCPClientAgent) Name() string                        { return "mock-tcp-client" }
func (m *mockTCPClientAgent) Type() string                        { return "tcp-client" }
func (m *mockTCPClientAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockTCPClientAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockTCPClientAgent) Process(data []byte) ([]byte, error) {
	m.processData = data
	m.processCalled = true
	return nil, m.processErr
}

func (m *mockTCPClientAgent) ReceiveMessage(_ context.Context) ([]byte, error) {
	if m.receiveErr != nil {
		return nil, m.receiveErr
	}
	return m.receiveData, nil
}

// mockTCPPlainAgent 는 기본 Agent만 구현하고 MessageReceiver/ConnAwareReceiver 는 구현하지 않는 테스트용 Agent이다.
type mockTCPPlainAgent struct{}

func (m *mockTCPPlainAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockTCPPlainAgent) Start(_ context.Context) error       { return nil }
func (m *mockTCPPlainAgent) Stop(_ context.Context) error        { return nil }
func (m *mockTCPPlainAgent) Pause(_ context.Context) error       { return nil }
func (m *mockTCPPlainAgent) Resume(_ context.Context) error      { return nil }
func (m *mockTCPPlainAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockTCPPlainAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockTCPPlainAgent) ID() string                          { return "mock-tcp-plain" }
func (m *mockTCPPlainAgent) Name() string                        { return "mock-tcp-plain" }
func (m *mockTCPPlainAgent) Type() string                        { return "tcp-server" }
func (m *mockTCPPlainAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockTCPPlainAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }
func (m *mockTCPPlainAgent) Process(_ []byte) ([]byte, error)    { return nil, nil }

// mockTCPResolver 는 테스트용 AgentResolver 구현이다.
type mockTCPResolver struct {
	transport AgentTransport
	err       error
}

func (m *mockTCPResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockTCPTransport 는 AgentTransport + AgentAccessor를 구현하는 테스트용 모의 객체이다.
type mockTCPTransport struct {
	agent agent.Agent
}

func (m *mockTCPTransport) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockTCPTransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockTCPTransport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockTCPTransportNoAccessor 는 AgentAccessor를 구현하지 않는 AgentTransport이다.
type mockTCPTransportNoAccessor struct{}

func (m *mockTCPTransportNoAccessor) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockTCPTransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// ---------------------------------------------------------------------------
// TCP 테스트용 헬퍼 함수
// ---------------------------------------------------------------------------

// newTCPNodeDef 는 테스트용 NodeDef를 생성한다.
func newTCPNodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// newTestTCPInNode 는 테스트용 TCPInNode를 agent를 직접 주입하여 생성한다.
// Init()을 우회하여 인터페이스 타입 검사를 건너뛴다.
func newTestTCPInNode(mockAgent agent.Agent) *TCPInNode {
	def := newTCPNodeDef("test-tcp-in", "tcp-in")
	base := NewBaseNode(def)

	n := &TCPInNode{
		tcpNodeBase: tcpNodeBase{
			BaseNode: base,
			agent:    mockAgent,
		},
		sourceCh: make(chan message.Message, tcpDefaultBufferSize),
		stopCh:   make(chan struct{}),
	}

	// 인터페이스 캐스팅
	if cr, ok := mockAgent.(agent.ConnAwareReceiver); ok {
		n.connReceiver = cr
	}
	if recv, ok := mockAgent.(agent.MessageReceiver); ok {
		n.receiver = recv
	}

	// Running 상태로 전이
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestTCPOutNode 는 테스트용 TCPOutNode를 agent를 직접 주입하여 생성한다.
func newTestTCPOutNode(mockAgent agent.Agent) *TCPOutNode {
	def := newTCPNodeDef("test-tcp-out", "tcp-out")
	base := NewBaseNode(def)

	n := &TCPOutNode{
		tcpNodeBase: tcpNodeBase{
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
// TCPInNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewTCPInNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewTCPInNode_정상생성 은 TCPInNode가 올바르게 생성되는지 확인한다.
func TestNewTCPInNode_정상생성(t *testing.T) {
	def := newTCPNodeDef("tcp-in-1", "tcp-in")
	node, err := NewTCPInNode(def)

	require.NoError(t, err)
	assert.NotNil(t, node)
}

// TestNewTCPInNode_WithResolver 는 AgentResolver 옵션이 적용되는지 확인한다.
func TestNewTCPInNode_WithResolver(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	resolver := &mockTCPResolver{
		transport: &mockTCPTransport{agent: mockAgent},
	}

	def := newTCPNodeDef("tcp-in-2", "tcp-in")
	node, err := NewTCPInNode(def, WithAgentResolver(resolver))

	require.NoError(t, err)
	inNode, ok := node.(*TCPInNode)
	require.True(t, ok)
	assert.NotNil(t, inNode.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestTCPInNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestTCPInNode_Configure_정상 은 올바른 설정이 적용되는지 확인한다.
func TestTCPInNode_Configure_정상(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPInNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref": "tcp-server-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "tcp-server-1", n.tcpCfg.AgentRef)
}

// TestTCPInNode_Configure_AgentRef없음 은 agent_ref가 없으면 ErrTCPMissingAgentRef를 반환하는지 확인한다.
func TestTCPInNode_Configure_AgentRef없음(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPInNode(mockAgent)

	err := n.Configure(map[string]any{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTCPMissingAgentRef))
}

// ---------------------------------------------------------------------------
// 3. TestTCPInNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestTCPInNode_Init_정상_서버 는 ConnAwareReceiver를 구현하는 TCP 서버 에이전트로 Init이 정상 동작하는지 확인한다.
func TestTCPInNode_Init_정상_서버(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	resolver := &mockTCPResolver{
		transport: &mockTCPTransport{agent: mockAgent},
	}

	def := newTCPNodeDef("tcp-in-init-server", "tcp-in")
	node, err := NewTCPInNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	inNode := node.(*TCPInNode)
	err = inNode.Configure(map[string]any{
		"agent_ref": "tcp-server-1",
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = inNode.Init(ctx)
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateRunning, inNode.CurrentState())
	// ConnAwareReceiver 가 설정되었는지 확인
	assert.NotNil(t, inNode.connReceiver)
	// MessageReceiver 도 함께 설정됨 (폴백용)
	assert.NotNil(t, inNode.receiver)

	// 정리
	_ = inNode.Shutdown(ctx)
}

// TestTCPInNode_Init_정상_클라이언트 는 MessageReceiver만 구현하는 TCP 클라이언트 에이전트로 Init이 정상 동작하는지 확인한다.
func TestTCPInNode_Init_정상_클라이언트(t *testing.T) {
	mockAgent := &mockTCPClientAgent{}
	resolver := &mockTCPResolver{
		transport: &mockTCPTransport{agent: mockAgent},
	}

	def := newTCPNodeDef("tcp-in-init-client", "tcp-in")
	node, err := NewTCPInNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	inNode := node.(*TCPInNode)
	err = inNode.Configure(map[string]any{
		"agent_ref": "tcp-client-1",
	})
	require.NoError(t, err)

	ctx := context.Background()
	err = inNode.Init(ctx)
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateRunning, inNode.CurrentState())
	// ConnAwareReceiver 는 미구현이므로 nil
	assert.Nil(t, inNode.connReceiver)
	// MessageReceiver 가 설정됨
	assert.NotNil(t, inNode.receiver)

	// 정리
	_ = inNode.Shutdown(ctx)
}

// TestTCPInNode_Init_ResolverNil 은 resolver가 nil이면 ErrTCPNoResolver를 반환하는지 확인한다.
func TestTCPInNode_Init_ResolverNil(t *testing.T) {
	def := newTCPNodeDef("tcp-in-no-resolver", "tcp-in")
	node, err := NewTCPInNode(def)
	require.NoError(t, err)

	inNode := node.(*TCPInNode)
	err = inNode.Configure(map[string]any{
		"agent_ref": "tcp-server-1",
	})
	require.NoError(t, err)

	err = inNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTCPNoResolver))
}

// TestTCPInNode_Init_NotReceiver 는 Agent가 MessageReceiver도 ConnAwareReceiver도 구현하지 않으면
// ErrTCPAgentNotReceiver를 반환하는지 확인한다.
func TestTCPInNode_Init_NotReceiver(t *testing.T) {
	plainAgent := &mockTCPPlainAgent{}
	resolver := &mockTCPResolver{
		transport: &mockTCPTransport{agent: plainAgent},
	}

	def := newTCPNodeDef("tcp-in-not-receiver", "tcp-in")
	node, err := NewTCPInNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	inNode := node.(*TCPInNode)
	err = inNode.Configure(map[string]any{
		"agent_ref": "tcp-server-1",
	})
	require.NoError(t, err)

	err = inNode.Init(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTCPAgentNotReceiver))
}

// ---------------------------------------------------------------------------
// 4. TestTCPInNode_SourceCh - SourceNode 테스트
// ---------------------------------------------------------------------------

// TestTCPInNode_SourceCh 는 SourceCh가 non-nil 채널을 반환하는지 확인한다.
func TestTCPInNode_SourceCh(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPInNode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// ---------------------------------------------------------------------------
// 5. TestTCPInNode_ReceiveLoop - 수신 루프 테스트
// ---------------------------------------------------------------------------

// TestTCPInNode_ReceiveLoop_ConnAware 는 ConnAwareReceiver 경로로 수신 시
// tcp.remote_addr 메타데이터가 설정되는지 확인한다.
func TestTCPInNode_ReceiveLoop_ConnAware(t *testing.T) {
	mockAgent := &mockTCPServerAgent{
		receiveData: []byte("hello from client"),
		remoteAddr:  "192.168.1.100:5678",
	}
	n := newTestTCPInNode(mockAgent)

	// 수신 루프 시작
	go n.receiveLoop()

	// 메시지 수신 대기
	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)

		// raw 데이터 확인
		raw, ok := msg.Payload().Get("raw")
		assert.True(t, ok)
		assert.Equal(t, []byte("hello from client"), raw)

		// data 문자열 확인
		data, ok := msg.Payload().Get("data")
		assert.True(t, ok)
		assert.Equal(t, "hello from client", data)

		// tcp.remote_addr 메타데이터 확인
		addr, ok := msg.Metadata().Get("tcp.remote_addr")
		assert.True(t, ok)
		assert.Equal(t, "192.168.1.100:5678", addr)

		// tcp.node_id 메타데이터 확인
		nodeID, ok := msg.Metadata().Get("tcp.node_id")
		assert.True(t, ok)
		assert.NotEmpty(t, nodeID)

		// tcp.agent_type 메타데이터 확인
		agentType, ok := msg.Metadata().Get("tcp.agent_type")
		assert.True(t, ok)
		assert.Equal(t, "tcp-server", agentType)

	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}

	// 정리
	close(n.stopCh)
}

// TestTCPInNode_ReceiveLoop_Fallback 은 MessageReceiver만 구현된 Agent를 사용할 때
// tcp.remote_addr 메타데이터가 설정되지 않는지 확인한다.
func TestTCPInNode_ReceiveLoop_Fallback(t *testing.T) {
	mockAgent := &mockTCPClientAgent{
		receiveData: []byte("hello from server"),
	}
	n := newTestTCPInNode(mockAgent)

	// 수신 루프 시작
	go n.receiveLoop()

	// 메시지 수신 대기
	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)

		// raw 데이터 확인
		raw, ok := msg.Payload().Get("raw")
		assert.True(t, ok)
		assert.Equal(t, []byte("hello from server"), raw)

		// data 문자열 확인
		data, ok := msg.Payload().Get("data")
		assert.True(t, ok)
		assert.Equal(t, "hello from server", data)

		// tcp.remote_addr 메타데이터가 없어야 한다
		_, ok = msg.Metadata().Get("tcp.remote_addr")
		assert.False(t, ok)

		// tcp.agent_type 메타데이터 확인
		agentType, ok := msg.Metadata().Get("tcp.agent_type")
		assert.True(t, ok)
		assert.Equal(t, "tcp-client", agentType)

	case <-time.After(3 * time.Second):
		t.Fatal("메시지 수신 타임아웃")
	}

	// 정리
	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 6. TestTCPInNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestTCPInNode_Shutdown_정상 은 Shutdown이 상태를 Stopping으로 전이하는지 확인한다.
func TestTCPInNode_Shutdown_정상(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPInNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// TCPOutNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewTCPOutNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewTCPOutNode_정상생성 은 TCPOutNode가 올바르게 생성되는지 확인한다.
func TestNewTCPOutNode_정상생성(t *testing.T) {
	def := newTCPNodeDef("tcp-out-1", "tcp-out")
	node, err := NewTCPOutNode(def)

	require.NoError(t, err)
	assert.NotNil(t, node)
}

// ---------------------------------------------------------------------------
// 2. TestTCPOutNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestTCPOutNode_Configure_정상 은 올바른 설정이 적용되는지 확인한다.
func TestTCPOutNode_Configure_정상(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	err := n.Configure(map[string]any{
		"agent_ref": "tcp-server-1",
	})

	require.NoError(t, err)
	assert.Equal(t, "tcp-server-1", n.tcpCfg.AgentRef)
}

// TestTCPOutNode_Configure_AgentRef없음 은 agent_ref가 없으면 ErrTCPMissingAgentRef를 반환하는지 확인한다.
func TestTCPOutNode_Configure_AgentRef없음(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	err := n.Configure(map[string]any{})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrTCPMissingAgentRef))
}

// ---------------------------------------------------------------------------
// 3. TestTCPOutNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestTCPOutNode_Init_정상 은 Init이 정상적으로 에이전트를 resolve하고 Running 상태가 되는지 확인한다.
func TestTCPOutNode_Init_정상(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	resolver := &mockTCPResolver{
		transport: &mockTCPTransport{agent: mockAgent},
	}

	def := newTCPNodeDef("tcp-out-init", "tcp-out")
	node, err := NewTCPOutNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	outNode := node.(*TCPOutNode)
	err = outNode.Configure(map[string]any{
		"agent_ref": "tcp-server-1",
	})
	require.NoError(t, err)

	err = outNode.Init(context.Background())
	require.NoError(t, err)

	assert.Equal(t, lifecycle.StateRunning, outNode.CurrentState())
}

// ---------------------------------------------------------------------------
// 4. TestTCPOutNode_Process - 메시지 전송 테스트
// ---------------------------------------------------------------------------

// TestTCPOutNode_Process_WithRemoteAddr 는 tcp.remote_addr 메타데이터가 있을 때
// JSON 명령의 target 필드에 해당 주소가 설정되는지 확인한다.
func TestTCPOutNode_Process_WithRemoteAddr(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("data", "response data")
	msg.Metadata().Set("tcp.remote_addr", "192.168.1.100:5678")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, mockAgent.processCalled)

	// 전송된 JSON 명령 파싱
	var cmd struct {
		Command string `json:"command"`
		Target  string `json:"target"`
		Data    string `json:"data"`
	}
	err = json.Unmarshal(mockAgent.processData, &cmd)
	require.NoError(t, err)

	assert.Equal(t, "send", cmd.Command)
	assert.Equal(t, "192.168.1.100:5678", cmd.Target)

	// base64 디코딩하여 원본 데이터 확인
	decoded, err := base64.StdEncoding.DecodeString(cmd.Data)
	require.NoError(t, err)
	assert.Equal(t, "response data", string(decoded))
}

// TestTCPOutNode_Process_Broadcast 는 tcp.remote_addr 메타데이터가 없을 때
// target이 빈 문자열(브로드캐스트)로 설정되는지 확인한다.
func TestTCPOutNode_Process_Broadcast(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("data", "broadcast data")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.True(t, mockAgent.processCalled)

	// 전송된 JSON 명령 파싱
	var cmd struct {
		Command string `json:"command"`
		Target  string `json:"target"`
		Data    string `json:"data"`
	}
	err = json.Unmarshal(mockAgent.processData, &cmd)
	require.NoError(t, err)

	assert.Equal(t, "send", cmd.Command)
	assert.Empty(t, cmd.Target) // 브로드캐스트

	// base64 디코딩하여 원본 데이터 확인
	decoded, err := base64.StdEncoding.DecodeString(cmd.Data)
	require.NoError(t, err)
	assert.Equal(t, "broadcast data", string(decoded))
}

// TestTCPOutNode_Process_Raw바이트 는 payload에 raw 바이트가 있을 때 우선 사용되는지 확인한다.
func TestTCPOutNode_Process_Raw바이트(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	rawBytes := []byte{0x01, 0x02, 0x03, 0xFF}
	msg := message.New()
	msg.Payload().Set("raw", rawBytes)
	msg.Payload().Set("data", "should be ignored")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 전송된 JSON 명령 파싱
	var cmd struct {
		Command string `json:"command"`
		Target  string `json:"target"`
		Data    string `json:"data"`
	}
	err = json.Unmarshal(mockAgent.processData, &cmd)
	require.NoError(t, err)

	// raw 바이트가 base64로 인코딩되어 전송됨
	decoded, err := base64.StdEncoding.DecodeString(cmd.Data)
	require.NoError(t, err)
	assert.Equal(t, rawBytes, decoded)
}

// TestTCPOutNode_Process_Data문자열 은 raw가 없고 data 문자열만 있을 때 폴백되는지 확인한다.
func TestTCPOutNode_Process_Data문자열(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("data", "hello tcp")

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 전송된 JSON 명령 파싱
	var cmd struct {
		Command string `json:"command"`
		Data    string `json:"data"`
	}
	err = json.Unmarshal(mockAgent.processData, &cmd)
	require.NoError(t, err)

	decoded, err := base64.StdEncoding.DecodeString(cmd.Data)
	require.NoError(t, err)
	assert.Equal(t, "hello tcp", string(decoded))
}

// TestTCPOutNode_Process_SendError 는 agent.Process() 실패 시 에러가 전파되는지 확인한다.
func TestTCPOutNode_Process_SendError(t *testing.T) {
	mockAgent := &mockTCPServerAgent{
		processErr: errors.New("connection reset"),
	}
	n := newTestTCPOutNode(mockAgent)

	msg := message.New()
	msg.Payload().Set("data", "test")

	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tcp-out: send failed")
}

// ---------------------------------------------------------------------------
// 5. TestTCPOutNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestTCPOutNode_Shutdown_정상 은 Shutdown이 상태를 Stopping으로 전이하는지 확인한다.
func TestTCPOutNode_Shutdown_정상(t *testing.T) {
	mockAgent := &mockTCPServerAgent{}
	n := newTestTCPOutNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// 레지스트리 등록 테스트
// ===========================================================================

// TestTCPRegistry_TCPIn 은 "tcp-in"이 레지스트리에 등록되어 있는지 확인한다.
func TestTCPRegistry_TCPIn(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("tcp-in"))

	meta, ok := r.TypeMeta("tcp-in")
	assert.True(t, ok)
	assert.Equal(t, "io", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
}

// TestTCPRegistry_TCPOut 은 "tcp-out"이 레지스트리에 등록되어 있는지 확인한다.
func TestTCPRegistry_TCPOut(t *testing.T) {
	r := NewRegistry()
	assert.True(t, r.Has("tcp-out"))

	meta, ok := r.TypeMeta("tcp-out")
	assert.True(t, ok)
	assert.Equal(t, "io", meta.Category)
	assert.Equal(t, "builtin", meta.Source)
}
