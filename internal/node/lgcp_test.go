package node

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/lg"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// LGCP 테스트용 모의 객체 정의
// ---------------------------------------------------------------------------

// mockLGCPAgent 는 테스트용 agent.Agent 구현이다.
// Process() 호출 시 수신한 데이터를 기록하고 미리 설정된 응답을 반환한다.
type mockLGCPAgent struct {
	processData []byte // 마지막 Process() 호출 시 전달된 데이터
	processResp []byte // Process() 호출 시 반환할 응답
	processErr  error  // Process() 호출 시 반환할 에러
}

func (m *mockLGCPAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockLGCPAgent) Start(_ context.Context) error       { return nil }
func (m *mockLGCPAgent) Stop(_ context.Context) error        { return nil }
func (m *mockLGCPAgent) Pause(_ context.Context) error       { return nil }
func (m *mockLGCPAgent) Resume(_ context.Context) error      { return nil }
func (m *mockLGCPAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockLGCPAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockLGCPAgent) ID() string                          { return "mock-lgcp" }
func (m *mockLGCPAgent) Name() string                        { return "mock-lgcp" }
func (m *mockLGCPAgent) Type() string                        { return "lg-lgcp" }
func (m *mockLGCPAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockLGCPAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockLGCPAgent) Process(data []byte) ([]byte, error) {
	m.processData = data
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.processResp, nil
}

// slowLGCPAgent 는 Process() 호출 시 지연을 발생시키는 테스트용 Agent이다.
type slowLGCPAgent struct {
	delay time.Duration
}

func (m *slowLGCPAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *slowLGCPAgent) Start(_ context.Context) error       { return nil }
func (m *slowLGCPAgent) Stop(_ context.Context) error        { return nil }
func (m *slowLGCPAgent) Pause(_ context.Context) error       { return nil }
func (m *slowLGCPAgent) Resume(_ context.Context) error      { return nil }
func (m *slowLGCPAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *slowLGCPAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *slowLGCPAgent) ID() string                          { return "slow-lgcp" }
func (m *slowLGCPAgent) Name() string                        { return "slow-lgcp" }
func (m *slowLGCPAgent) Type() string                        { return "lg-lgcp" }
func (m *slowLGCPAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *slowLGCPAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *slowLGCPAgent) Process(_ []byte) ([]byte, error) {
	time.Sleep(m.delay)
	return []byte(`{"ok": true}`), nil
}

// mockLGCPResolver 는 테스트용 AgentResolver 구현이다.
type mockLGCPResolver struct {
	transport AgentTransport
	err       error
}

func (m *mockLGCPResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockLGCPTransport 는 AgentTransport + AgentAccessor를 구현하는 테스트용 모의 객체이다.
type mockLGCPTransport struct {
	agent agent.Agent
}

func (m *mockLGCPTransport) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockLGCPTransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockLGCPTransport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockLGCPTransportNoAccessor 는 AgentAccessor를 구현하지 않는 AgentTransport이다.
type mockLGCPTransportNoAccessor struct{}

func (m *mockLGCPTransportNoAccessor) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockLGCPTransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// ---------------------------------------------------------------------------
// LGCP 테스트용 헬퍼 함수
// ---------------------------------------------------------------------------

// newLGCPNodeDef 는 테스트용 NodeDef를 생성한다.
func newLGCPNodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// newTestLGCPStatusNode 는 테스트용 LGCPStatusNode를 agent를 직접 주입하여 생성한다.
// Init()을 우회하여 lg.LGCPAgent 타입 검사를 건너뛴다.
func newTestLGCPStatusNode(mockAgent agent.Agent) *LGCPStatusNode {
	def := newLGCPNodeDef("test-status", "lgcp-status")
	base := NewBaseNode(def)
	n := &LGCPStatusNode{
		lgcpNodeBase: lgcpNodeBase{
			BaseNode: base,
			timeout:  5 * time.Second,
			agent:    mockAgent,
		},
		sourceCh: make(chan message.Message, 64),
		stopCh:   make(chan struct{}),
	}
	// Running 상태로 전이 (Process 호출을 위해)
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestLGCPControlNode 는 테스트용 LGCPControlNode를 agent를 직접 주입하여 생성한다.
func newTestLGCPControlNode(mockAgent agent.Agent) *LGCPControlNode {
	def := newLGCPNodeDef("test-control", "lgcp-control")
	base := NewBaseNode(def)
	n := &LGCPControlNode{
		lgcpNodeBase: lgcpNodeBase{
			BaseNode: base,
			timeout:  5 * time.Second,
			agent:    mockAgent,
		},
	}
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestLGCPNode 는 테스트용 LGCPNode를 agent를 직접 주입하여 생성한다.
func newTestLGCPNode(mockAgent agent.Agent) *LGCPNode {
	def := newLGCPNodeDef("test-lgcp", "lgcp")
	base := NewBaseNode(def)
	n := &LGCPNode{
		lgcpNodeBase: lgcpNodeBase{
			BaseNode: base,
			timeout:  5 * time.Second,
			agent:    mockAgent,
		},
		sourceCh: make(chan message.Message, 64),
		stopCh:   make(chan struct{}),
	}
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// parseLGCPProcessCommand 는 Agent.Process()에 전달된 JSON 바이트를 파싱하여 map으로 반환한다.
func parseLGCPProcessCommand(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var cmd map[string]any
	err := json.Unmarshal(data, &cmd)
	require.NoError(t, err, "LGCP Process 명령 JSON 파싱 실패")
	return cmd
}

// ===========================================================================
// R18: LGCPStatusNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewLGCPStatusNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewLGCPStatusNode_정상생성 은 LGCPStatusNode가 올바르게 생성되는지 확인한다.
func TestNewLGCPStatusNode_정상생성(t *testing.T) {
	def := newLGCPNodeDef("status-1", "lgcp-status")
	node, err := NewLGCPStatusNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "status-1", node.Name())
	assert.Equal(t, "lgcp-status", node.Type())
}

// TestNewLGCPStatusNode_Resolver옵션 은 WithAgentResolver 옵션으로 resolver가 설정되는지 확인한다.
func TestNewLGCPStatusNode_Resolver옵션(t *testing.T) {
	resolver := &mockLGCPResolver{}
	def := newLGCPNodeDef("status-resolver", "lgcp-status")
	node, err := NewLGCPStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGCPStatusNode)
	assert.NotNil(t, n.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestLGCPStatusNode_Configure - 설정 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGCPStatusNode_Configure 는 Configure 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestLGCPStatusNode_Configure(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]any
		wantErr   error
		checkFunc func(t *testing.T, n *LGCPStatusNode)
	}{
		{
			name: "agent_ref 누락 에러",
			config: map[string]any{
				"default_address": "0x10",
			},
			wantErr: ErrLGCPMissingAgentRef,
		},
		{
			name: "agent_ref 빈문자열 에러",
			config: map[string]any{
				"agent_ref": "",
			},
			wantErr: ErrLGCPMissingAgentRef,
		},
		{
			name: "기본값 적용",
			config: map[string]any{
				"agent_ref": "lgcp-agent-1",
			},
			checkFunc: func(t *testing.T, n *LGCPStatusNode) {
				assert.Equal(t, "lgcp-agent-1", n.lgcpCfg.AgentRef)
				assert.Equal(t, "", n.lgcpCfg.DefaultAddress, "default_address 기본값 빈문자열")
				assert.Equal(t, "30s", n.lgcpCfg.PollInterval, "poll_interval 기본값 30s")
				assert.Equal(t, "5s", n.lgcpCfg.Timeout, "timeout 기본값 5s")
				assert.Equal(t, lgcpCmdGetStats, n.lgcpCfg.PollCommand, "poll_command 기본값 get_stats")
				assert.Equal(t, 10, n.lgcpCfg.RecentCount, "recent_count 기본값 10")
				assert.Equal(t, 5*time.Second, n.timeout)
				assert.Equal(t, 30*time.Second, n.pollInterval)
			},
		},
		{
			name: "커스텀 값 적용",
			config: map[string]any{
				"agent_ref":       "my-lgcp",
				"default_address": "0x20",
				"poll_interval":   "10s",
				"timeout":         "15s",
				"poll_command":    "get_recent",
				"recent_count":    float64(20),
			},
			checkFunc: func(t *testing.T, n *LGCPStatusNode) {
				assert.Equal(t, "my-lgcp", n.lgcpCfg.AgentRef)
				assert.Equal(t, "0x20", n.lgcpCfg.DefaultAddress)
				assert.Equal(t, "10s", n.lgcpCfg.PollInterval)
				assert.Equal(t, "15s", n.lgcpCfg.Timeout)
				assert.Equal(t, "get_recent", n.lgcpCfg.PollCommand)
				assert.Equal(t, 20, n.lgcpCfg.RecentCount)
				assert.Equal(t, 15*time.Second, n.timeout)
				assert.Equal(t, 10*time.Second, n.pollInterval)
			},
		},
		{
			name: "recent_count int 타입",
			config: map[string]any{
				"agent_ref":    "lgcp-1",
				"recent_count": 5,
			},
			checkFunc: func(t *testing.T, n *LGCPStatusNode) {
				assert.Equal(t, 5, n.lgcpCfg.RecentCount)
			},
		},
		{
			name: "잘못된 timeout 시 기본값 적용",
			config: map[string]any{
				"agent_ref": "lgcp-agent-1",
				"timeout":   "invalid",
			},
			checkFunc: func(t *testing.T, n *LGCPStatusNode) {
				assert.Equal(t, lgcpDefaultTimeout, n.timeout, "잘못된 timeout은 기본값으로 대체")
			},
		},
		{
			name: "잘못된 poll_interval 시 기본값 적용",
			config: map[string]any{
				"agent_ref":     "lgcp-agent-1",
				"poll_interval": "not-a-duration",
			},
			checkFunc: func(t *testing.T, n *LGCPStatusNode) {
				assert.Equal(t, lgcpDefaultPollInterval, n.pollInterval, "잘못된 poll_interval은 기본값으로 대체")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newLGCPNodeDef("test-cfg", "lgcp-status")
			node, err := NewLGCPStatusNode(def)
			require.NoError(t, err)

			n := node.(*LGCPStatusNode)
			err = n.Configure(tt.config)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.checkFunc != nil {
				tt.checkFunc(t, n)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. TestLGCPStatusNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestLGCPStatusNode_Init_Resolver없음_에러 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestLGCPStatusNode_Init_Resolver없음_에러(t *testing.T) {
	def := newLGCPNodeDef("init-no-resolver", "lgcp-status")
	node, err := NewLGCPStatusNode(def)
	require.NoError(t, err)

	n := node.(*LGCPStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "lgcp-1"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPNoResolver)
}

// TestLGCPStatusNode_Init_비LGCP_Agent_에러 는 resolve된 Agent가 LG LGCP 타입이 아닐 때 에러를 반환하는지 확인한다.
func TestLGCPStatusNode_Init_비LGCP_Agent_에러(t *testing.T) {
	fakeAgent := &mockLGCPAgent{}
	transport := &mockLGCPTransport{agent: fakeAgent}
	resolver := &mockLGCPResolver{transport: transport}

	def := newLGCPNodeDef("init-non-lgcp", "lgcp-status")
	node, err := NewLGCPStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGCPStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "fake-lgcp"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPAgentNotLGCP)
}

// TestLGCPStatusNode_Init_AgentAccessor_미지원_에러 는 transport가 AgentAccessor를 구현하지 않을 때 에러를 반환하는지 확인한다.
func TestLGCPStatusNode_Init_AgentAccessor_미지원_에러(t *testing.T) {
	transport := &mockLGCPTransportNoAccessor{}
	resolver := &mockLGCPResolver{transport: transport}

	def := newLGCPNodeDef("init-no-accessor", "lgcp-status")
	node, err := NewLGCPStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGCPStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "no-accessor"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPAgentNotLGCP)
}

// TestLGCPStatusNode_Init_Resolver실패_에러 는 Agent resolve 실패 시 에러를 반환하는지 확인한다.
func TestLGCPStatusNode_Init_Resolver실패_에러(t *testing.T) {
	resolver := &mockLGCPResolver{err: assert.AnError}

	def := newLGCPNodeDef("init-resolve-err", "lgcp-status")
	node, err := NewLGCPStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGCPStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "missing-agent"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "agent resolve failed")
}

// ---------------------------------------------------------------------------
// 4. TestLGCPStatusNode_Process - 상태 조회 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGCPStatusNode_Process 는 Process 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestLGCPStatusNode_Process(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		pollCommand string
		recentCount int
		msgPayload  map[string]any
		agentResp   map[string]any
		wantCommand string
		checkCmd    func(t *testing.T, cmd map[string]any)
		checkOutput func(t *testing.T, out message.Message)
	}{
		{
			name:        "기본 get_stats 명령",
			address:     "",
			pollCommand: lgcpCmdGetStats,
			agentResp:   map[string]any{"total_frames": 100, "errors": 0},
			wantCommand: lgcpCmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				_, hasAddress := cmd["address"]
				assert.False(t, hasAddress, "address가 포함되지 않아야 한다")
			},
		},
		{
			name:        "address 설정 시 포함",
			address:     "0x10",
			pollCommand: lgcpCmdGetStats,
			agentResp:   map[string]any{"power": "on"},
			wantCommand: lgcpCmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"])
			},
		},
		{
			name:        "get_recent 명령 + count",
			address:     "0x20",
			pollCommand: lgcpCmdGetRecent,
			recentCount: 5,
			agentResp:   map[string]any{"frames": []any{"f1", "f2"}},
			wantCommand: lgcpCmdGetRecent,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x20", cmd["address"])
				assert.Equal(t, float64(5), cmd["count"])
			},
		},
		{
			name:        "payload address 오버라이드",
			address:     "0x10",
			pollCommand: lgcpCmdGetStats,
			msgPayload: map[string]any{
				"address": "0xFF",
			},
			agentResp:   map[string]any{"power": "off"},
			wantCommand: lgcpCmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0xFF", cmd["address"], "payload의 address가 우선")
			},
		},
		{
			name:        "payload에 빈 address -> config 값 유지",
			address:     "0x10",
			pollCommand: lgcpCmdGetStats,
			msgPayload: map[string]any{
				"address": "",
			},
			agentResp:   map[string]any{"status": "idle"},
			wantCommand: lgcpCmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"], "빈 문자열은 오버라이드하지 않음")
			},
		},
		{
			name:        "응답의 모든 키가 출력 payload에 포함",
			address:     "0x10",
			pollCommand: lgcpCmdGetStats,
			agentResp:   map[string]any{"power": "on", "mode": "cool", "temperature": 22.5},
			wantCommand: lgcpCmdGetStats,
			checkOutput: func(t *testing.T, out message.Message) {
				v, ok := out.Payload().Get("power")
				assert.True(t, ok)
				assert.Equal(t, "on", v)

				v, ok = out.Payload().Get("mode")
				assert.True(t, ok)
				assert.Equal(t, "cool", v)

				v, ok = out.Payload().Get("temperature")
				assert.True(t, ok)
				assert.Equal(t, 22.5, v)

				// 메타데이터 확인
				source, ok := out.Metadata().Get("lgcp_source")
				assert.True(t, ok)
				assert.Equal(t, "request", source)

				nodeID, ok := out.Metadata().Get("lgcp_node_id")
				assert.True(t, ok)
				assert.NotEmpty(t, nodeID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockLGCPAgent{processResp: respBytes}
			n := newTestLGCPStatusNode(mockAgent)

			// config 설정
			recentCount := tt.recentCount
			if recentCount == 0 {
				recentCount = 10
			}
			n.mu.Lock()
			n.lgcpCfg = LGCPNodeConfig{
				AgentRef:       "test-agent",
				DefaultAddress: tt.address,
				PollCommand:    tt.pollCommand,
				RecentCount:    recentCount,
			}
			n.mu.Unlock()

			// 입력 메시지 생성
			var msg message.Message
			if tt.msgPayload != nil {
				msg = message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			} else {
				msg = message.New()
			}

			results, err := n.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// Agent에 전달된 명령 검증
			cmd := parseLGCPProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])

			if tt.checkCmd != nil {
				tt.checkCmd(t, cmd)
			}

			if tt.checkOutput != nil {
				tt.checkOutput(t, results[0])
			}
		})
	}
}

// TestLGCPStatusNode_Process_AgentNil_에러 는 agent가 nil일 때 에러를 반환하는지 확인한다.
func TestLGCPStatusNode_Process_AgentNil_에러(t *testing.T) {
	n := newTestLGCPStatusNode(nil)
	n.lgcpCfg = LGCPNodeConfig{AgentRef: "test", PollCommand: lgcpCmdGetStats, RecentCount: 10}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPProcessFailed)
}

// TestLGCPStatusNode_Process_유효하지않은응답_에러 는 Agent 응답이 유효하지 않은 JSON일 때 에러를 반환하는지 확인한다.
func TestLGCPStatusNode_Process_유효하지않은응답_에러(t *testing.T) {
	mockAgent := &mockLGCPAgent{processResp: []byte("invalid json")}
	n := newTestLGCPStatusNode(mockAgent)
	n.lgcpCfg = LGCPNodeConfig{AgentRef: "test", PollCommand: lgcpCmdGetStats, RecentCount: 10}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPProcessFailed)
	assert.Contains(t, err.Error(), "invalid response JSON")
}

// TestLGCPStatusNode_Process_AgentError_에러 는 Agent Process() 에러가 전파되는지 확인한다.
func TestLGCPStatusNode_Process_AgentError_에러(t *testing.T) {
	mockAgent := &mockLGCPAgent{processErr: assert.AnError}
	n := newTestLGCPStatusNode(mockAgent)
	n.lgcpCfg = LGCPNodeConfig{AgentRef: "test", PollCommand: lgcpCmdGetStats, RecentCount: 10}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPProcessFailed)
}

// ---------------------------------------------------------------------------
// 5. TestLGCPStatusNode_SourceNode - 폴링 테스트
// ---------------------------------------------------------------------------

// TestLGCPStatusNode_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestLGCPStatusNode_SourceCh(t *testing.T) {
	mockAgent := &mockLGCPAgent{}
	n := newTestLGCPStatusNode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// TestLGCPStatusNode_SourceNode_폴링 은 pollLoop가 sourceCh에 메시지를 전달하는지 확인한다.
func TestLGCPStatusNode_SourceNode_폴링(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"power": "on", "temperature": 25.0})
	mockAgent := &mockLGCPAgent{processResp: respBytes}

	n := newTestLGCPStatusNode(mockAgent)
	n.lgcpCfg = LGCPNodeConfig{AgentRef: "test-agent", PollCommand: lgcpCmdGetStats, RecentCount: 10}
	n.pollInterval = 50 * time.Millisecond

	// 폴링 고루틴 시작
	go n.pollLoop()

	// 메시지 수신 대기 (최대 1초)
	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)
		v, ok := msg.Payload().Get("power")
		assert.True(t, ok)
		assert.Equal(t, "on", v)

		source, ok := msg.Metadata().Get("lgcp_source")
		assert.True(t, ok)
		assert.Equal(t, "poll", source)
	case <-time.After(1 * time.Second):
		t.Fatal("폴링 메시지가 1초 내에 도착하지 않았다")
	}

	// 정리
	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 6. TestLGCPStatusNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestLGCPStatusNode_Shutdown_폴링정지 는 Shutdown 시 폴링이 정지되는지 확인한다.
func TestLGCPStatusNode_Shutdown_폴링정지(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"ok": true})
	mockAgent := &mockLGCPAgent{processResp: respBytes}

	n := newTestLGCPStatusNode(mockAgent)
	n.lgcpCfg = LGCPNodeConfig{AgentRef: "test-agent", PollCommand: lgcpCmdGetStats, RecentCount: 10}
	n.pollInterval = 50 * time.Millisecond

	// 폴링 고루틴 시작
	go n.pollLoop()

	// 최소 1개의 폴링 메시지가 생성될 때까지 대기
	select {
	case <-n.sourceCh:
		// 정상: 폴링 메시지 수신 확인
	case <-time.After(2 * time.Second):
		t.Fatal("폴링 메시지가 2초 내에 도착하지 않았다")
	}

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())

	// Shutdown 후에는 새 메시지가 생성되지 않아야 한다
	time.Sleep(100 * time.Millisecond)
	for {
		select {
		case <-n.sourceCh:
			continue
		default:
			goto drained
		}
	}
drained:

	// 200ms 동안 새 메시지가 없어야 한다
	select {
	case <-n.sourceCh:
		t.Fatal("Shutdown 후에도 폴링 메시지가 생성되고 있다")
	case <-time.After(200 * time.Millisecond):
		// 정상: 폴링 중단 확인
	}
}

// TestLGCPStatusNode_Shutdown_이중호출 은 Shutdown을 2번 호출해도 패닉이 발생하지 않는지 확인한다.
func TestLGCPStatusNode_Shutdown_이중호출(t *testing.T) {
	mockAgent := &mockLGCPAgent{}
	n := newTestLGCPStatusNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)

	// 두 번째 호출은 에러가 발생할 수 있지만 패닉은 아님
	_ = n.Shutdown(context.Background())
}

// ===========================================================================
// R19: LGCPControlNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewLGCPControlNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewLGCPControlNode_정상생성 은 LGCPControlNode가 올바르게 생성되는지 확인한다.
func TestNewLGCPControlNode_정상생성(t *testing.T) {
	def := newLGCPNodeDef("control-1", "lgcp-control")
	node, err := NewLGCPControlNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "control-1", node.Name())
	assert.Equal(t, "lgcp-control", node.Type())
}

// ---------------------------------------------------------------------------
// 2. TestLGCPControlNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestLGCPControlNode_Configure_기본값 은 기본값이 올바르게 적용되는지 확인한다.
func TestLGCPControlNode_Configure_기본값(t *testing.T) {
	def := newLGCPNodeDef("ctrl-cfg", "lgcp-control")
	node, err := NewLGCPControlNode(def)
	require.NoError(t, err)

	n := node.(*LGCPControlNode)
	err = n.Configure(map[string]any{
		"agent_ref":       "ctrl-agent",
		"default_address": "0xAB",
	})
	require.NoError(t, err)
	assert.Equal(t, "ctrl-agent", n.lgcpCfg.AgentRef)
	assert.Equal(t, "0xAB", n.lgcpCfg.DefaultAddress)
}

// ---------------------------------------------------------------------------
// 3. TestLGCPControlNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestLGCPControlNode_Init_Resolver없음_에러 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestLGCPControlNode_Init_Resolver없음_에러(t *testing.T) {
	def := newLGCPNodeDef("init-no-resolver", "lgcp-control")
	node, err := NewLGCPControlNode(def)
	require.NoError(t, err)

	n := node.(*LGCPControlNode)
	err = n.Configure(map[string]any{"agent_ref": "lgcp-1"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPNoResolver)
}

// ---------------------------------------------------------------------------
// 4. TestLGCPControlNode_Process - 제어 명령 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGCPControlNode_Process 는 Process 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestLGCPControlNode_Process(t *testing.T) {
	tests := []struct {
		name           string
		defaultAddress string
		msgPayload     map[string]any
		agentResp      map[string]any
		wantCommand    string
		wantErr        error
		checkCmd       func(t *testing.T, cmd map[string]any)
		checkOutput    func(t *testing.T, out message.Message)
	}{
		{
			name:           "set_multiple 제어 (power)",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"power": "on",
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgcpCmdSetMultiple,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"])
				params, ok := cmd["params"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "on", params["power"])
			},
		},
		{
			name:           "set_multiple 복합 제어",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"power":       "on",
				"mode":        "cool",
				"temperature": 22.0,
				"fan_speed":   "high",
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgcpCmdSetMultiple,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				params, ok := cmd["params"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "on", params["power"])
				assert.Equal(t, "cool", params["mode"])
				assert.Equal(t, 22.0, params["temperature"])
				assert.Equal(t, "high", params["fan_speed"])
			},
		},
		{
			name:           "payload address 오버라이드",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"address": "0xFF",
				"power":   "off",
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgcpCmdSetMultiple,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0xFF", cmd["address"], "payload의 address가 우선")
			},
		},
		{
			name:           "address 미설정 시 에러 (제어 키 있음)",
			defaultAddress: "",
			msgPayload: map[string]any{
				"power": "on",
			},
			wantErr: ErrLGCPProcessFailed,
		},
		{
			name:           "직접 command 전달",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"command": "custom_cmd",
				"params":  map[string]any{"key": "value"},
			},
			agentResp:   map[string]any{"result": "custom_ok"},
			wantCommand: "custom_cmd",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"])
				assert.NotNil(t, cmd["params"])
			},
		},
		{
			name:           "직접 command + address 없음 -> 에러",
			defaultAddress: "",
			msgPayload: map[string]any{
				"command": "custom_cmd",
			},
			wantErr: ErrLGCPProcessFailed,
		},
		{
			name:           "제어 키 없으면 상태 조회로 폴백",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"some_other_key": "value",
			},
			agentResp:   map[string]any{"stats": "ok"},
			wantCommand: lgcpCmdGetStats,
		},
		{
			name:           "출력 메타데이터 확인",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"power": "on",
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgcpCmdSetMultiple,
			checkOutput: func(t *testing.T, out message.Message) {
				cmd, ok := out.Metadata().Get("lgcp_command")
				assert.True(t, ok)
				assert.Equal(t, "control", cmd)

				nodeID, ok := out.Metadata().Get("lgcp_node_id")
				assert.True(t, ok)
				assert.NotEmpty(t, nodeID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockLGCPAgent{processResp: respBytes}
			n := newTestLGCPControlNode(mockAgent)

			n.mu.Lock()
			n.lgcpCfg = LGCPNodeConfig{
				AgentRef:       "test-agent",
				DefaultAddress: tt.defaultAddress,
				PollCommand:    lgcpCmdGetStats,
				RecentCount:    10,
			}
			n.mu.Unlock()

			msg := message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))

			results, err := n.Process(context.Background(), msg)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Len(t, results, 1)

			// Agent에 전달된 명령 검증
			cmd := parseLGCPProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])

			if tt.checkCmd != nil {
				tt.checkCmd(t, cmd)
			}

			if tt.checkOutput != nil {
				tt.checkOutput(t, results[0])
			}
		})
	}
}

// TestLGCPControlNode_Process_AgentNil_에러 는 agent가 nil일 때 에러를 반환하는지 확인한다.
func TestLGCPControlNode_Process_AgentNil_에러(t *testing.T) {
	n := newTestLGCPControlNode(nil)
	n.lgcpCfg = LGCPNodeConfig{
		AgentRef:       "test",
		DefaultAddress: "0x10",
		PollCommand:    lgcpCmdGetStats,
		RecentCount:    10,
	}

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"power": "on",
	})))
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPProcessFailed)
}

// TestLGCPControlNode_Shutdown 은 Shutdown이 정상 동작하는지 확인한다.
func TestLGCPControlNode_Shutdown(t *testing.T) {
	mockAgent := &mockLGCPAgent{}
	n := newTestLGCPControlNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// R20: LGCPNode (통합) 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewLGCPNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewLGCPNode_정상생성 은 LGCPNode가 올바르게 생성되는지 확인한다.
func TestNewLGCPNode_정상생성(t *testing.T) {
	def := newLGCPNodeDef("lgcp-1", "lgcp")
	node, err := NewLGCPNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "lgcp-1", node.Name())
	assert.Equal(t, "lgcp", node.Type())
}

// ---------------------------------------------------------------------------
// 2. TestLGCPNode_Process - 자동 감지 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGCPNode_Process_자동감지 는 LGCPNode가 payload에 따라 상태 조회와 제어를 자동 감지하는지 테스트한다.
func TestLGCPNode_Process_자동감지(t *testing.T) {
	tests := []struct {
		name        string
		address     string
		msgPayload  map[string]any
		agentResp   map[string]any
		wantCommand string
		wantCmdType string
		wantErr     error
		checkCmd    func(t *testing.T, cmd map[string]any)
	}{
		{
			name:        "제어 키 없으면 상태 조회 (get_stats)",
			address:     "",
			msgPayload:  map[string]any{},
			agentResp:   map[string]any{"stats": "ok"},
			wantCommand: lgcpCmdGetStats,
			wantCmdType: "status",
		},
		{
			name:    "power 키 -> 제어 모드",
			address: "0x10",
			msgPayload: map[string]any{
				"power": "on",
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgcpCmdSetMultiple,
			wantCmdType: "control",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"])
			},
		},
		{
			name:    "temperature 키 -> 제어 모드",
			address: "0x20",
			msgPayload: map[string]any{
				"temperature": 24.0,
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgcpCmdSetMultiple,
			wantCmdType: "control",
		},
		{
			name:    "제어 키 있는데 address 없으면 에러",
			address: "",
			msgPayload: map[string]any{
				"power": "on",
			},
			wantErr: ErrLGCPProcessFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockLGCPAgent{processResp: respBytes}
			n := newTestLGCPNode(mockAgent)

			n.mu.Lock()
			n.lgcpCfg = LGCPNodeConfig{
				AgentRef:       "test-agent",
				DefaultAddress: tt.address,
				PollCommand:    lgcpCmdGetStats,
				RecentCount:    10,
			}
			n.mu.Unlock()

			msg := message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			results, err := n.Process(context.Background(), msg)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			require.Len(t, results, 1)

			cmd := parseLGCPProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])

			// 메타데이터의 lgcp_command 확인
			cmdType, ok := results[0].Metadata().Get("lgcp_command")
			assert.True(t, ok)
			assert.Equal(t, tt.wantCmdType, cmdType)

			if tt.checkCmd != nil {
				tt.checkCmd(t, cmd)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. TestLGCPNode_SourceNode - 폴링 테스트
// ---------------------------------------------------------------------------

// TestLGCPNode_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestLGCPNode_SourceCh(t *testing.T) {
	mockAgent := &mockLGCPAgent{}
	n := newTestLGCPNode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// TestLGCPNode_SourceNode_폴링 은 pollLoop가 sourceCh에 메시지를 전달하는지 확인한다.
func TestLGCPNode_SourceNode_폴링(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"power": "on"})
	mockAgent := &mockLGCPAgent{processResp: respBytes}

	n := newTestLGCPNode(mockAgent)
	n.lgcpCfg = LGCPNodeConfig{AgentRef: "test-agent", PollCommand: lgcpCmdGetStats, RecentCount: 10}
	n.pollInterval = 50 * time.Millisecond

	go n.pollLoop()

	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)
		source, ok := msg.Metadata().Get("lgcp_source")
		assert.True(t, ok)
		assert.Equal(t, "poll", source)
	case <-time.After(1 * time.Second):
		t.Fatal("폴링 메시지가 1초 내에 도착하지 않았다")
	}

	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 4. TestLGCPNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestLGCPNode_Shutdown 은 Shutdown이 정상 동작하는지 확인한다.
func TestLGCPNode_Shutdown(t *testing.T) {
	mockAgent := &mockLGCPAgent{}
	n := newTestLGCPNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// 헬퍼 함수 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// TestHasLGCPControlKeys - 제어 키 감지 테스트
// ---------------------------------------------------------------------------

func TestHasLGCPControlKeys(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    bool
	}{
		{
			name:    "power 키 있음",
			payload: map[string]any{"power": "on"},
			want:    true,
		},
		{
			name:    "mode 키 있음",
			payload: map[string]any{"mode": "cool"},
			want:    true,
		},
		{
			name:    "temperature 키 있음",
			payload: map[string]any{"temperature": 24.0},
			want:    true,
		},
		{
			name:    "fan_speed 키 있음",
			payload: map[string]any{"fan_speed": "high"},
			want:    true,
		},
		{
			name:    "제어 키 없음",
			payload: map[string]any{"some_key": "value"},
			want:    false,
		},
		{
			name:    "빈 payload",
			payload: map[string]any{},
			want:    false,
		},
		{
			name:    "복합 제어 키",
			payload: map[string]any{"power": "on", "temperature": 22.0},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			got := hasLGCPControlKeys(msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// TestBuildLGCPStatusCommand - 상태 조회 명령 빌드 테스트
// ---------------------------------------------------------------------------

func TestBuildLGCPStatusCommand(t *testing.T) {
	tests := []struct {
		name     string
		cfg      LGCPNodeConfig
		checkCmd func(t *testing.T, cmd map[string]any)
	}{
		{
			name: "get_stats 기본 명령",
			cfg: LGCPNodeConfig{
				PollCommand: lgcpCmdGetStats,
				RecentCount: 10,
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgcpCmdGetStats, cmd["command"])
				_, hasCount := cmd["count"]
				assert.False(t, hasCount, "get_stats에는 count가 없어야 한다")
				_, hasAddress := cmd["address"]
				assert.False(t, hasAddress, "address가 비어있으면 포함되지 않아야 한다")
			},
		},
		{
			name: "get_recent 명령 + count",
			cfg: LGCPNodeConfig{
				PollCommand: lgcpCmdGetRecent,
				RecentCount: 20,
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgcpCmdGetRecent, cmd["command"])
				assert.Equal(t, float64(20), cmd["count"])
			},
		},
		{
			name: "address 포함",
			cfg: LGCPNodeConfig{
				PollCommand:    lgcpCmdGetStats,
				DefaultAddress: "0x10",
				RecentCount:    10,
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmdBytes, err := buildLGCPStatusCommand(tt.cfg)
			require.NoError(t, err)

			var cmd map[string]any
			err = json.Unmarshal(cmdBytes, &cmd)
			require.NoError(t, err)

			tt.checkCmd(t, cmd)
		})
	}
}

// ---------------------------------------------------------------------------
// TestBuildLGCPControlCommand - 제어 명령 빌드 테스트
// ---------------------------------------------------------------------------

func TestBuildLGCPControlCommand(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]any
		cfg      LGCPNodeConfig
		wantErr  error
		checkCmd func(t *testing.T, cmd map[string]any)
	}{
		{
			name:    "set_multiple 생성",
			payload: map[string]any{"power": "on", "temperature": 22.0},
			cfg:     LGCPNodeConfig{DefaultAddress: "0x10"},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgcpCmdSetMultiple, cmd["command"])
				assert.Equal(t, "0x10", cmd["address"])
				params, ok := cmd["params"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "on", params["power"])
				assert.Equal(t, 22.0, params["temperature"])
			},
		},
		{
			name:    "payload address 우선",
			payload: map[string]any{"address": "0xFF", "power": "on"},
			cfg:     LGCPNodeConfig{DefaultAddress: "0x10"},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0xFF", cmd["address"])
			},
		},
		{
			name:    "address 없으면 에러",
			payload: map[string]any{"power": "on"},
			cfg:     LGCPNodeConfig{DefaultAddress: ""},
			wantErr: ErrLGCPMissingAddress,
		},
		{
			name:    "직접 command 전달",
			payload: map[string]any{"command": "custom_cmd", "params": map[string]any{"k": "v"}},
			cfg:     LGCPNodeConfig{DefaultAddress: "0x10"},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "custom_cmd", cmd["command"])
				assert.Equal(t, "0x10", cmd["address"])
				assert.NotNil(t, cmd["params"])
			},
		},
		{
			name:    "직접 command + address 없음 -> 에러",
			payload: map[string]any{"command": "custom_cmd"},
			cfg:     LGCPNodeConfig{DefaultAddress: ""},
			wantErr: ErrLGCPMissingAddress,
		},
		{
			name:    "제어 키 없으면 상태 조회로 폴백",
			payload: map[string]any{"some_key": "value"},
			cfg:     LGCPNodeConfig{DefaultAddress: "0x10", PollCommand: lgcpCmdGetStats, RecentCount: 10},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgcpCmdGetStats, cmd["command"], "제어 키가 없으면 상태 조회로 폴백")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))

			cmdBytes, err := buildLGCPControlCommand(msg, tt.cfg)

			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)

			var cmd map[string]any
			err = json.Unmarshal(cmdBytes, &cmd)
			require.NoError(t, err)

			tt.checkCmd(t, cmd)
		})
	}
}

// ---------------------------------------------------------------------------
// TestApplyLGCPOverrides - 오버라이드 테스트
// ---------------------------------------------------------------------------

func TestApplyLGCPOverrides(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]any
		cfg      LGCPNodeConfig
		checkCfg func(t *testing.T, cfg LGCPNodeConfig)
	}{
		{
			name:    "address 오버라이드",
			payload: map[string]any{"address": "0xFF"},
			cfg:     LGCPNodeConfig{DefaultAddress: "0x10"},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, "0xFF", cfg.DefaultAddress)
			},
		},
		{
			name:    "빈 address는 오버라이드 안 함",
			payload: map[string]any{"address": ""},
			cfg:     LGCPNodeConfig{DefaultAddress: "0x10"},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, "0x10", cfg.DefaultAddress)
			},
		},
		{
			name:    "timeout 오버라이드",
			payload: map[string]any{"timeout": "10s"},
			cfg:     LGCPNodeConfig{Timeout: "5s"},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, "10s", cfg.Timeout)
			},
		},
		{
			name:    "poll_command 오버라이드",
			payload: map[string]any{"poll_command": "get_recent"},
			cfg:     LGCPNodeConfig{PollCommand: lgcpCmdGetStats},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, "get_recent", cfg.PollCommand)
			},
		},
		{
			name:    "count 오버라이드 (float64)",
			payload: map[string]any{"count": float64(5)},
			cfg:     LGCPNodeConfig{RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, 5, cfg.RecentCount)
			},
		},
		{
			name:    "count 오버라이드 (int)",
			payload: map[string]any{"count": 7},
			cfg:     LGCPNodeConfig{RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, 7, cfg.RecentCount)
			},
		},
		{
			name:    "count 0 이하는 오버라이드 안 함",
			payload: map[string]any{"count": float64(0)},
			cfg:     LGCPNodeConfig{RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, 10, cfg.RecentCount)
			},
		},
		{
			name:    "관련 없는 키는 무시",
			payload: map[string]any{"unknown_key": "value"},
			cfg:     LGCPNodeConfig{DefaultAddress: "0x10", PollCommand: lgcpCmdGetStats, RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGCPNodeConfig) {
				assert.Equal(t, "0x10", cfg.DefaultAddress)
				assert.Equal(t, lgcpCmdGetStats, cfg.PollCommand)
				assert.Equal(t, 10, cfg.RecentCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			result := applyLGCPOverrides(msg, tt.cfg)
			tt.checkCfg(t, result)
		})
	}
}

// ---------------------------------------------------------------------------
// 인터페이스 컴파일 타임 체크
// ---------------------------------------------------------------------------

// 컴파일 타임에 인터페이스 구현을 확인하는 보호 변수
var (
	_ Node       = (*LGCPStatusNode)(nil)
	_ SourceNode = (*LGCPStatusNode)(nil)
	_ Node       = (*LGCPControlNode)(nil)
	_ Node       = (*LGCPNode)(nil)
	_ SourceNode = (*LGCPNode)(nil)
)

// ---------------------------------------------------------------------------
// TestLGCPNode_Timeout - 타임아웃 테스트
// ---------------------------------------------------------------------------

// TestLGCPStatusNode_Process_타임아웃 은 Agent Process가 느릴 때 타임아웃이 발생하는지 확인한다.
func TestLGCPStatusNode_Process_타임아웃(t *testing.T) {
	slow := &slowLGCPAgent{delay: 2 * time.Second}
	n := newTestLGCPStatusNode(slow)
	n.timeout = 100 * time.Millisecond
	n.lgcpCfg = LGCPNodeConfig{AgentRef: "test", PollCommand: lgcpCmdGetStats, RecentCount: 10}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPProcessFailed)
}

// ---------------------------------------------------------------------------
// TestLGCPControlNode_Process_타임아웃 은 Agent Process가 느릴 때 타임아웃이 발생하는지 확인한다.
// ---------------------------------------------------------------------------

func TestLGCPControlNode_Process_타임아웃(t *testing.T) {
	slow := &slowLGCPAgent{delay: 2 * time.Second}
	n := newTestLGCPControlNode(slow)
	n.timeout = 100 * time.Millisecond
	n.lgcpCfg = LGCPNodeConfig{
		AgentRef:       "test",
		DefaultAddress: "0x10",
		PollCommand:    lgcpCmdGetStats,
		RecentCount:    10,
	}

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"power": "on",
	})))
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGCPProcessFailed)
}

// ---------------------------------------------------------------------------
// Registry 등록 테스트
// ---------------------------------------------------------------------------

// TestRegistry_LGCP노드등록 은 LGCP 노드가 Registry에 등록되어 있는지 확인한다.
func TestRegistry_LGCP노드등록(t *testing.T) {
	r := NewRegistry()

	lgcpTypes := []string{"lgcp-status", "lgcp-control", "lgcp"}
	for _, typeName := range lgcpTypes {
		t.Run(typeName, func(t *testing.T) {
			assert.True(t, r.Has(typeName), "%s가 레지스트리에 등록되어 있어야 한다", typeName)

			meta, ok := r.TypeMeta(typeName)
			assert.True(t, ok)
			assert.Equal(t, "io", meta.Category)
			assert.Equal(t, "builtin", meta.Source)
		})
	}
}

// 사용하지 않는 import 방지를 위한 변수
var _ = lg.LGCPAgent{}
