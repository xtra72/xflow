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

// mockLGHvacr02Agent 는 테스트용 agent.Agent 구현이다.
// Process() 호출 시 수신한 데이터를 기록하고 미리 설정된 응답을 반환한다.
type mockLGHvacr02Agent struct {
	processData []byte // 마지막 Process() 호출 시 전달된 데이터
	processResp []byte // Process() 호출 시 반환할 응답
	processErr  error  // Process() 호출 시 반환할 에러
}

func (m *mockLGHvacr02Agent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockLGHvacr02Agent) Start(_ context.Context) error       { return nil }
func (m *mockLGHvacr02Agent) Stop(_ context.Context) error        { return nil }
func (m *mockLGHvacr02Agent) Pause(_ context.Context) error       { return nil }
func (m *mockLGHvacr02Agent) Resume(_ context.Context) error      { return nil }
func (m *mockLGHvacr02Agent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockLGHvacr02Agent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockLGHvacr02Agent) ID() string                          { return "mock-lg_hvacr02" }
func (m *mockLGHvacr02Agent) Name() string                        { return "mock-lg_hvacr02" }
func (m *mockLGHvacr02Agent) Type() string                        { return "lg-lg_hvacr02" }
func (m *mockLGHvacr02Agent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockLGHvacr02Agent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockLGHvacr02Agent) Process(data []byte) ([]byte, error) {
	m.processData = data
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.processResp, nil
}

// slowLGHvacr02Agent 는 Process() 호출 시 지연을 발생시키는 테스트용 Agent이다.
type slowLGHvacr02Agent struct {
	delay time.Duration
}

func (m *slowLGHvacr02Agent) Init(_ agent.AgentConfig) error      { return nil }
func (m *slowLGHvacr02Agent) Start(_ context.Context) error       { return nil }
func (m *slowLGHvacr02Agent) Stop(_ context.Context) error        { return nil }
func (m *slowLGHvacr02Agent) Pause(_ context.Context) error       { return nil }
func (m *slowLGHvacr02Agent) Resume(_ context.Context) error      { return nil }
func (m *slowLGHvacr02Agent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *slowLGHvacr02Agent) Configure(_ agent.AgentConfig) error { return nil }
func (m *slowLGHvacr02Agent) ID() string                          { return "slow-lg_hvacr02" }
func (m *slowLGHvacr02Agent) Name() string                        { return "slow-lg_hvacr02" }
func (m *slowLGHvacr02Agent) Type() string                        { return "lg-lg_hvacr02" }
func (m *slowLGHvacr02Agent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *slowLGHvacr02Agent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *slowLGHvacr02Agent) Process(_ []byte) ([]byte, error) {
	time.Sleep(m.delay)
	return []byte(`{"ok": true}`), nil
}

// mockLGHvacr02Resolver 는 테스트용 AgentResolver 구현이다.
type mockLGHvacr02Resolver struct {
	transport AgentTransport
	err       error
}

func (m *mockLGHvacr02Resolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockLGHvacr02Transport 는 AgentTransport + AgentAccessor를 구현하는 테스트용 모의 객체이다.
type mockLGHvacr02Transport struct {
	agent agent.Agent
}

func (m *mockLGHvacr02Transport) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockLGHvacr02Transport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockLGHvacr02Transport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockLGHvacr02TransportNoAccessor 는 AgentAccessor를 구현하지 않는 AgentTransport이다.
type mockLGHvacr02TransportNoAccessor struct{}

func (m *mockLGHvacr02TransportNoAccessor) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockLGHvacr02TransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// ---------------------------------------------------------------------------
// LGCP 테스트용 헬퍼 함수
// ---------------------------------------------------------------------------

// newLGHvacr02NodeDef 는 테스트용 NodeDef를 생성한다.
func newLGHvacr02NodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// newTestLGHvacr02StatusNode 는 테스트용 LGHvacr02StatusNode를 agent를 직접 주입하여 생성한다.
// Init()을 우회하여 lg.Hvacr02Agent 타입 검사를 건너뛴다.
func newTestLGHvacr02StatusNode(mockAgent agent.Agent) *LGHvacr02StatusNode {
	def := newLGHvacr02NodeDef("test-status", "lg_hvacr02_status")
	base := NewBaseNode(def)
	n := &LGHvacr02StatusNode{
		lgHvacr02NodeBase: lgHvacr02NodeBase{
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

// newTestLGHvacr02ControlNode 는 테스트용 LGHvacr02ControlNode를 agent를 직접 주입하여 생성한다.
func newTestLGHvacr02ControlNode(mockAgent agent.Agent) *LGHvacr02ControlNode {
	def := newLGHvacr02NodeDef("test-control", "lg_hvacr02_control")
	base := NewBaseNode(def)
	n := &LGHvacr02ControlNode{
		lgHvacr02NodeBase: lgHvacr02NodeBase{
			BaseNode: base,
			timeout:  5 * time.Second,
			agent:    mockAgent,
		},
	}
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestLGHvacr02Node 는 테스트용 LGHvacr02Node를 agent를 직접 주입하여 생성한다.
func newTestLGHvacr02Node(mockAgent agent.Agent) *LGHvacr02Node {
	def := newLGHvacr02NodeDef("test-lgcp", "lg_hvacr02")
	base := NewBaseNode(def)
	n := &LGHvacr02Node{
		lgHvacr02NodeBase: lgHvacr02NodeBase{
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

// parseLGHvacr02ProcessCommand 는 Agent.Process()에 전달된 JSON 바이트를 파싱하여 map으로 반환한다.
func parseLGHvacr02ProcessCommand(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var cmd map[string]any
	err := json.Unmarshal(data, &cmd)
	require.NoError(t, err, "LGCP Process 명령 JSON 파싱 실패")
	return cmd
}

// ===========================================================================
// R18: LGHvacr02StatusNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewLGHvacr02StatusNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewLGHvacr02StatusNode_정상생성 은 LGHvacr02StatusNode가 올바르게 생성되는지 확인한다.
func TestNewLGHvacr02StatusNode_정상생성(t *testing.T) {
	def := newLGHvacr02NodeDef("status-1", "lg_hvacr02_status")
	node, err := NewLGHvacr02StatusNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "status-1", node.Name())
	assert.Equal(t, "lg_hvacr02_status", node.Type())
}

// TestNewLGHvacr02StatusNode_Resolver옵션 은 WithAgentResolver 옵션으로 resolver가 설정되는지 확인한다.
func TestNewLGHvacr02StatusNode_Resolver옵션(t *testing.T) {
	resolver := &mockLGHvacr02Resolver{}
	def := newLGHvacr02NodeDef("status-resolver", "lg_hvacr02_status")
	node, err := NewLGHvacr02StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGHvacr02StatusNode)
	assert.NotNil(t, n.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestLGHvacr02StatusNode_Configure - 설정 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGHvacr02StatusNode_Configure 는 Configure 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestLGHvacr02StatusNode_Configure(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]any
		wantErr   error
		checkFunc func(t *testing.T, n *LGHvacr02StatusNode)
	}{
		{
			name: "agent_ref 누락 에러",
			config: map[string]any{
				"default_address": "0x10",
			},
			wantErr: ErrLGHvacr02MissingAgentRef,
		},
		{
			name: "agent_ref 빈문자열 에러",
			config: map[string]any{
				"agent_ref": "",
			},
			wantErr: ErrLGHvacr02MissingAgentRef,
		},
		{
			name: "기본값 적용",
			config: map[string]any{
				"agent_ref": "lg_hvacr02-agent-1",
			},
			checkFunc: func(t *testing.T, n *LGHvacr02StatusNode) {
				assert.Equal(t, "lg_hvacr02-agent-1", n.lgHvacr02Cfg.AgentRef)
				assert.Equal(t, "", n.lgHvacr02Cfg.DefaultAddress, "default_address 기본값 빈문자열")
				assert.Equal(t, "100ms", n.lgHvacr02Cfg.PollInterval, "poll_interval 기본값 100ms")
				assert.Equal(t, "5s", n.lgHvacr02Cfg.Timeout, "timeout 기본값 5s")
				assert.Equal(t, lgHvacr02CmdGetRecent, n.lgHvacr02Cfg.PollCommand, "poll_command 기본값 get_recent")
				assert.Equal(t, 10, n.lgHvacr02Cfg.RecentCount, "recent_count 기본값 10")
				assert.Equal(t, 5*time.Second, n.timeout)
				// 2026-05-30: status 의 inactivity 모델 — 기본 90s.
				assert.Equal(t, lgHvacr02DefaultInactivityTimeout, n.inactivityTimeout)
			},
		},
		{
			name: "커스텀 값 적용",
			config: map[string]any{
				"agent_ref":          "my-lg_hvacr02",
				"default_address":    "0x20",
				"poll_interval":      "10s",
				"timeout":            "15s",
				"poll_command":       "get_recent",
				"recent_count":       float64(20),
				"inactivity_timeout": "30s",
				"unit_id":            "58",
			},
			checkFunc: func(t *testing.T, n *LGHvacr02StatusNode) {
				assert.Equal(t, "my-lg_hvacr02", n.lgHvacr02Cfg.AgentRef)
				assert.Equal(t, "0x20", n.lgHvacr02Cfg.DefaultAddress)
				assert.Equal(t, "10s", n.lgHvacr02Cfg.PollInterval)
				assert.Equal(t, "15s", n.lgHvacr02Cfg.Timeout)
				assert.Equal(t, "get_recent", n.lgHvacr02Cfg.PollCommand)
				assert.Equal(t, 20, n.lgHvacr02Cfg.RecentCount)
				assert.Equal(t, 15*time.Second, n.timeout)
				// status: inactivity_timeout 적용 + unit_id.
				assert.Equal(t, 30*time.Second, n.inactivityTimeout)
				assert.Equal(t, "58", n.lgHvacr02Cfg.UnitID)
			},
		},
		{
			name: "recent_count int 타입",
			config: map[string]any{
				"agent_ref":    "lg_hvacr02-1",
				"recent_count": 5,
			},
			checkFunc: func(t *testing.T, n *LGHvacr02StatusNode) {
				assert.Equal(t, 5, n.lgHvacr02Cfg.RecentCount)
			},
		},
		{
			name: "잘못된 timeout 시 기본값 적용",
			config: map[string]any{
				"agent_ref": "lg_hvacr02-agent-1",
				"timeout":   "invalid",
			},
			checkFunc: func(t *testing.T, n *LGHvacr02StatusNode) {
				assert.Equal(t, hvacr02DefaultTimeout, n.timeout, "잘못된 timeout은 기본값으로 대체")
			},
		},
		{
			name: "잘못된 inactivity_timeout 시 기본값 적용",
			config: map[string]any{
				"agent_ref":          "lg_hvacr02-agent-1",
				"inactivity_timeout": "not-a-duration",
			},
			checkFunc: func(t *testing.T, n *LGHvacr02StatusNode) {
				assert.Equal(t, lgHvacr02DefaultInactivityTimeout, n.inactivityTimeout, "잘못된 inactivity_timeout은 기본값으로 대체")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newLGHvacr02NodeDef("test-cfg", "lg_hvacr02_status")
			node, err := NewLGHvacr02StatusNode(def)
			require.NoError(t, err)

			n := node.(*LGHvacr02StatusNode)
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
// 3. TestLGHvacr02StatusNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestLGHvacr02StatusNode_Init_Resolver없음_에러 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestLGHvacr02StatusNode_Init_Resolver없음_에러(t *testing.T) {
	def := newLGHvacr02NodeDef("init-no-resolver", "lg_hvacr02_status")
	node, err := NewLGHvacr02StatusNode(def)
	require.NoError(t, err)

	n := node.(*LGHvacr02StatusNode)
	err = n.Configure(map[string]any{"agent_ref": "lg_hvacr02-1"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02NoResolver)
}

// TestLGHvacr02StatusNode_Init_비LGHvacr02_Agent_에러 는 resolve된 Agent가 LG LGCP 타입이 아닐 때 에러를 반환하는지 확인한다.
func TestLGHvacr02StatusNode_Init_비LGHvacr02_Agent_에러(t *testing.T) {
	fakeAgent := &mockLGHvacr02Agent{}
	transport := &mockLGHvacr02Transport{agent: fakeAgent}
	resolver := &mockLGHvacr02Resolver{transport: transport}

	def := newLGHvacr02NodeDef("init-non-lgcp", "lg_hvacr02_status")
	node, err := NewLGHvacr02StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGHvacr02StatusNode)
	err = n.Configure(map[string]any{"agent_ref": "fake-lg_hvacr02"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02AgentNotLGHvacr02)
}

// TestLGHvacr02StatusNode_Init_AgentAccessor_미지원_에러 는 transport가 AgentAccessor를 구현하지 않을 때 에러를 반환하는지 확인한다.
func TestLGHvacr02StatusNode_Init_AgentAccessor_미지원_에러(t *testing.T) {
	transport := &mockLGHvacr02TransportNoAccessor{}
	resolver := &mockLGHvacr02Resolver{transport: transport}

	def := newLGHvacr02NodeDef("init-no-accessor", "lg_hvacr02_status")
	node, err := NewLGHvacr02StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGHvacr02StatusNode)
	err = n.Configure(map[string]any{"agent_ref": "no-accessor"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02AgentNotLGHvacr02)
}

// TestLGHvacr02StatusNode_Init_Resolver실패_에러 는 Agent resolve 실패 시 플로우는 시작되지만 노드는 대기 상태가 되는지 확인한다.
// 이제 agent not found는 runtime 에러로 처리되어 플로우가 계속 진행된다.
func TestLGHvacr02StatusNode_Init_Resolver실패_에러(t *testing.T) {
	resolver := &mockLGHvacr02Resolver{err: assert.AnError}

	def := newLGHvacr02NodeDef("init-resolve-err", "lg_hvacr02_status")
	node, err := NewLGHvacr02StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*LGHvacr02StatusNode)
	err = n.Configure(map[string]any{"agent_ref": "missing-agent"})
	require.NoError(t, err)

	// 에이전트를 찾을 수 없어도 Init은 성공하고, 노드는 Running 상태로 진행
	err = n.Init(context.Background())
	require.NoError(t, err)

	// 노드는 agent nil 상태 (나중에 Reinit으로 연결됨)
	require.Nil(t, n.agent)
}

// ---------------------------------------------------------------------------
// 4. TestLGHvacr02StatusNode_Process - 상태 조회 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGHvacr02StatusNode_Process 는 Process 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestLGHvacr02StatusNode_Process(t *testing.T) {
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
			pollCommand: lgHvacr02CmdGetStats,
			agentResp:   map[string]any{"total_frames": 100, "errors": 0},
			wantCommand: lgHvacr02CmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				_, hasAddress := cmd["address"]
				assert.False(t, hasAddress, "address가 포함되지 않아야 한다")
			},
		},
		{
			name:        "address 설정 시 포함",
			address:     "0x10",
			pollCommand: lgHvacr02CmdGetStats,
			agentResp:   map[string]any{"power": "on"},
			wantCommand: lgHvacr02CmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"])
			},
		},
		{
			name:        "get_recent 명령 + count",
			address:     "0x20",
			pollCommand: lgHvacr02CmdGetRecent,
			recentCount: 5,
			agentResp:   map[string]any{"frames": []any{"f1", "f2"}},
			wantCommand: lgHvacr02CmdGetRecent,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x20", cmd["address"])
				assert.Equal(t, float64(5), cmd["count"])
			},
		},
		{
			name:        "payload address 오버라이드",
			address:     "0x10",
			pollCommand: lgHvacr02CmdGetStats,
			msgPayload: map[string]any{
				"address": "0xFF",
			},
			agentResp:   map[string]any{"power": "off"},
			wantCommand: lgHvacr02CmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0xFF", cmd["address"], "payload의 address가 우선")
			},
		},
		{
			name:        "payload에 빈 address -> config 값 유지",
			address:     "0x10",
			pollCommand: lgHvacr02CmdGetStats,
			msgPayload: map[string]any{
				"address": "",
			},
			agentResp:   map[string]any{"status": "idle"},
			wantCommand: lgHvacr02CmdGetStats,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0x10", cmd["address"], "빈 문자열은 오버라이드하지 않음")
			},
		},
		{
			name:        "응답의 모든 키가 출력 payload에 포함",
			address:     "0x10",
			pollCommand: lgHvacr02CmdGetStats,
			agentResp:   map[string]any{"power": "on", "mode": "cool", "temperature": 22.5},
			wantCommand: lgHvacr02CmdGetStats,
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

				// v0.10.0: lg_hvacr02_source="request" 제거됨 — message_type 으로 식별.
				_, srcOK := out.Metadata().Get("node_source")
				assert.False(t, srcOK, "v0.10.0: Process 응답에는 lg_hvacr02_source 가 설정되지 않아야 함")

				nodeID, ok := out.Metadata().Get("node_id")
				assert.True(t, ok)
				assert.NotEmpty(t, nodeID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockLGHvacr02Agent{processResp: respBytes}
			n := newTestLGHvacr02StatusNode(mockAgent)

			// config 설정
			recentCount := tt.recentCount
			if recentCount == 0 {
				recentCount = 10
			}
			n.mu.Lock()
			n.lgHvacr02Cfg = LGHvacr02NodeConfig{
				AgentRef:       "test-agent",
				DefaultAddress: tt.address,
				PollCommand:    tt.pollCommand,
				RecentCount:    recentCount,
				EmitMetadata:   MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true},
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
			cmd := parseLGHvacr02ProcessCommand(t, mockAgent.processData)
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

// TestLGHvacr02StatusNode_Process_AgentNil_에러 는 agent가 nil일 때 에러를 반환하는지 확인한다.
func TestLGHvacr02StatusNode_Process_AgentNil_에러(t *testing.T) {
	n := newTestLGHvacr02StatusNode(nil)
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{AgentRef: "test", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10, EmitMetadata: MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true}}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02ProcessFailed)
}

// TestLGHvacr02StatusNode_Process_유효하지않은응답_에러 는 Agent 응답이 유효하지 않은 JSON일 때 에러를 반환하는지 확인한다.
func TestLGHvacr02StatusNode_Process_유효하지않은응답_에러(t *testing.T) {
	mockAgent := &mockLGHvacr02Agent{processResp: []byte("invalid json")}
	n := newTestLGHvacr02StatusNode(mockAgent)
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{AgentRef: "test", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10, EmitMetadata: MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true}}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02ProcessFailed)
	assert.Contains(t, err.Error(), "invalid response JSON")
}

// TestLGHvacr02StatusNode_Process_AgentError_에러 는 Agent Process() 에러가 전파되는지 확인한다.
func TestLGHvacr02StatusNode_Process_AgentError_에러(t *testing.T) {
	mockAgent := &mockLGHvacr02Agent{processErr: assert.AnError}
	n := newTestLGHvacr02StatusNode(mockAgent)
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{AgentRef: "test", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10, EmitMetadata: MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true}}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02ProcessFailed)
}

// ---------------------------------------------------------------------------
// 5. TestLGHvacr02StatusNode_SourceNode - 폴링 테스트
// ---------------------------------------------------------------------------

// TestLGHvacr02StatusNode_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestLGHvacr02StatusNode_SourceCh(t *testing.T) {
	mockAgent := &mockLGHvacr02Agent{}
	n := newTestLGHvacr02StatusNode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// 2026-05-30: TestLGHvacr02StatusNode_SourceNode_폴링 / Shutdown_폴링정지 삭제.
// status 노드가 pull (pollLoop) → push (receiveLoop / inactivity-fallback) 모델로
// 전환됨에 따라 옛 폴링 의존 검증은 의미를 잃었다. 새 모델의 회귀 테스트는
// LG HVACR-01 status node 테스트와 동일 패턴으로 별도 commit 에서 추가 예정.

// TestLGHvacr02StatusNode_Shutdown_이중호출 은 Shutdown을 2번 호출해도 패닉이 발생하지 않는지 확인한다.
func TestLGHvacr02StatusNode_Shutdown_이중호출(t *testing.T) {
	mockAgent := &mockLGHvacr02Agent{}
	n := newTestLGHvacr02StatusNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)

	// 두 번째 호출은 에러가 발생할 수 있지만 패닉은 아님
	_ = n.Shutdown(context.Background())
}

// ===========================================================================
// R19: LGHvacr02ControlNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewLGHvacr02ControlNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewLGHvacr02ControlNode_정상생성 은 LGHvacr02ControlNode가 올바르게 생성되는지 확인한다.
func TestNewLGHvacr02ControlNode_정상생성(t *testing.T) {
	def := newLGHvacr02NodeDef("control-1", "lg_hvacr02_control")
	node, err := NewLGHvacr02ControlNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "control-1", node.Name())
	assert.Equal(t, "lg_hvacr02_control", node.Type())
}

// ---------------------------------------------------------------------------
// 2. TestLGHvacr02ControlNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestLGHvacr02ControlNode_Configure_기본값 은 기본값이 올바르게 적용되는지 확인한다.
func TestLGHvacr02ControlNode_Configure_기본값(t *testing.T) {
	def := newLGHvacr02NodeDef("ctrl-cfg", "lg_hvacr02_control")
	node, err := NewLGHvacr02ControlNode(def)
	require.NoError(t, err)

	n := node.(*LGHvacr02ControlNode)
	err = n.Configure(map[string]any{
		"agent_ref":       "ctrl-agent",
		"default_address": "0xAB",
	})
	require.NoError(t, err)
	assert.Equal(t, "ctrl-agent", n.lgHvacr02Cfg.AgentRef)
	assert.Equal(t, "0xAB", n.lgHvacr02Cfg.DefaultAddress)
}

// ---------------------------------------------------------------------------
// 3. TestLGHvacr02ControlNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestLGHvacr02ControlNode_Init_Resolver없음_에러 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestLGHvacr02ControlNode_Init_Resolver없음_에러(t *testing.T) {
	def := newLGHvacr02NodeDef("init-no-resolver", "lg_hvacr02_control")
	node, err := NewLGHvacr02ControlNode(def)
	require.NoError(t, err)

	n := node.(*LGHvacr02ControlNode)
	err = n.Configure(map[string]any{"agent_ref": "lg_hvacr02-1"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02NoResolver)
}

// ---------------------------------------------------------------------------
// 4. TestLGHvacr02ControlNode_Process - 제어 명령 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGHvacr02ControlNode_Process 는 Process 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestLGHvacr02ControlNode_Process(t *testing.T) {
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
			wantCommand: lgHvacr02CmdSetMultiple,
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
			wantCommand: lgHvacr02CmdSetMultiple,
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
			wantCommand: lgHvacr02CmdSetMultiple,
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
			wantErr: ErrLGHvacr02ProcessFailed,
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
			wantErr: ErrLGHvacr02ProcessFailed,
		},
		{
			name:           "제어 키 없으면 상태 조회로 폴백",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"some_other_key": "value",
			},
			agentResp:   map[string]any{"stats": "ok"},
			wantCommand: lgHvacr02CmdGetStats,
		},
		{
			name:           "출력 메타데이터 확인",
			defaultAddress: "0x10",
			msgPayload: map[string]any{
				"power": "on",
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgHvacr02CmdSetMultiple,
			checkOutput: func(t *testing.T, out message.Message) {
				cmd, ok := out.Metadata().Get("lg_hvacr02_command")
				assert.True(t, ok)
				assert.Equal(t, "control", cmd)

				nodeID, ok := out.Metadata().Get("node_id")
				assert.True(t, ok)
				assert.NotEmpty(t, nodeID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockLGHvacr02Agent{processResp: respBytes}
			n := newTestLGHvacr02ControlNode(mockAgent)

			n.mu.Lock()
			n.lgHvacr02Cfg = LGHvacr02NodeConfig{
				AgentRef:       "test-agent",
				DefaultAddress: tt.defaultAddress,
				PollCommand:    lgHvacr02CmdGetStats,
				RecentCount:    10,
				EmitMetadata:   MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true},
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
			cmd := parseLGHvacr02ProcessCommand(t, mockAgent.processData)
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

// TestLGHvacr02ControlNode_Process_AgentNil_에러 는 agent가 nil일 때 에러를 반환하는지 확인한다.
func TestLGHvacr02ControlNode_Process_AgentNil_에러(t *testing.T) {
	n := newTestLGHvacr02ControlNode(nil)
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{
		AgentRef:       "test",
		DefaultAddress: "0x10",
		PollCommand:    lgHvacr02CmdGetStats,
		RecentCount:    10,
	}

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"power": "on",
	})))
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02ProcessFailed)
}

// TestLGHvacr02ControlNode_Shutdown 은 Shutdown이 정상 동작하는지 확인한다.
func TestLGHvacr02ControlNode_Shutdown(t *testing.T) {
	mockAgent := &mockLGHvacr02Agent{}
	n := newTestLGHvacr02ControlNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// R20: LGHvacr02Node (통합) 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewLGHvacr02Node - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewLGHvacr02Node_정상생성 은 LGHvacr02Node가 올바르게 생성되는지 확인한다.
func TestNewLGHvacr02Node_정상생성(t *testing.T) {
	def := newLGHvacr02NodeDef("lg_hvacr02-1", "lg_hvacr02")
	node, err := NewLGHvacr02Node(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "lg_hvacr02-1", node.Name())
	assert.Equal(t, "lg_hvacr02", node.Type())
}

// ---------------------------------------------------------------------------
// 2. TestLGHvacr02Node_Process - 자동 감지 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestLGHvacr02Node_Process_자동감지 는 LGHvacr02Node가 payload에 따라 상태 조회와 제어를 자동 감지하는지 테스트한다.
func TestLGHvacr02Node_Process_자동감지(t *testing.T) {
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
			wantCommand: lgHvacr02CmdGetStats,
			wantCmdType: "status",
		},
		{
			name:    "power 키 -> 제어 모드",
			address: "0x10",
			msgPayload: map[string]any{
				"power": "on",
			},
			agentResp:   map[string]any{"result": "ok"},
			wantCommand: lgHvacr02CmdSetMultiple,
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
			wantCommand: lgHvacr02CmdSetMultiple,
			wantCmdType: "control",
		},
		{
			name:    "제어 키 있는데 address 없으면 에러",
			address: "",
			msgPayload: map[string]any{
				"power": "on",
			},
			wantErr: ErrLGHvacr02ProcessFailed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockLGHvacr02Agent{processResp: respBytes}
			n := newTestLGHvacr02Node(mockAgent)

			n.mu.Lock()
			n.lgHvacr02Cfg = LGHvacr02NodeConfig{
				AgentRef:       "test-agent",
				DefaultAddress: tt.address,
				PollCommand:    lgHvacr02CmdGetStats,
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

			cmd := parseLGHvacr02ProcessCommand(t, mockAgent.processData)
			assert.Equal(t, tt.wantCommand, cmd["command"])

			// 메타데이터의 lg_hvacr02_command 확인
			cmdType, ok := results[0].Metadata().Get("lg_hvacr02_command")
			assert.True(t, ok)
			assert.Equal(t, tt.wantCmdType, cmdType)

			if tt.checkCmd != nil {
				tt.checkCmd(t, cmd)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 3. TestLGHvacr02Node_SourceNode - 폴링 테스트
// ---------------------------------------------------------------------------

// TestLGHvacr02Node_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestLGHvacr02Node_SourceCh(t *testing.T) {
	mockAgent := &mockLGHvacr02Agent{}
	n := newTestLGHvacr02Node(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// TestLGHvacr02Node_SourceNode_폴링 은 pollLoop가 sourceCh에 메시지를 전달하는지 확인한다.
func TestLGHvacr02Node_SourceNode_폴링(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"power": "on"})
	mockAgent := &mockLGHvacr02Agent{processResp: respBytes}

	n := newTestLGHvacr02Node(mockAgent)
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{AgentRef: "test-agent", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10, EmitMetadata: MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true}}
	n.pollInterval = 50 * time.Millisecond

	go n.pollLoop()

	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)
		source, ok := msg.Metadata().Get("node_source")
		assert.True(t, ok)
		assert.Equal(t, "poll", source)
	case <-time.After(1 * time.Second):
		t.Fatal("폴링 메시지가 1초 내에 도착하지 않았다")
	}

	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 4. TestLGHvacr02Node_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestLGHvacr02Node_Shutdown 은 Shutdown이 정상 동작하는지 확인한다.
func TestLGHvacr02Node_Shutdown(t *testing.T) {
	mockAgent := &mockLGHvacr02Agent{}
	n := newTestLGHvacr02Node(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// 헬퍼 함수 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// TestHasLGHvacr02ControlKeys - 제어 키 감지 테스트
// ---------------------------------------------------------------------------

func TestHasLGHvacr02ControlKeys(t *testing.T) {
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
			got := hasLGHvacr02ControlKeys(msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// TestBuildLGHvacr02StatusCommand - 상태 조회 명령 빌드 테스트
// ---------------------------------------------------------------------------

func TestBuildLGHvacr02StatusCommand(t *testing.T) {
	tests := []struct {
		name     string
		cfg      LGHvacr02NodeConfig
		checkCmd func(t *testing.T, cmd map[string]any)
	}{
		{
			name: "get_stats 기본 명령",
			cfg: LGHvacr02NodeConfig{
				PollCommand: lgHvacr02CmdGetStats,
				RecentCount: 10,
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgHvacr02CmdGetStats, cmd["command"])
				_, hasCount := cmd["count"]
				assert.False(t, hasCount, "get_stats에는 count가 없어야 한다")
				_, hasAddress := cmd["address"]
				assert.False(t, hasAddress, "address가 비어있으면 포함되지 않아야 한다")
			},
		},
		{
			name: "get_recent 명령 + count",
			cfg: LGHvacr02NodeConfig{
				PollCommand: lgHvacr02CmdGetRecent,
				RecentCount: 20,
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgHvacr02CmdGetRecent, cmd["command"])
				assert.Equal(t, float64(20), cmd["count"])
			},
		},
		{
			name: "address 포함",
			cfg: LGHvacr02NodeConfig{
				PollCommand:    lgHvacr02CmdGetStats,
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
			cmdBytes, err := buildLGHvacr02StatusCommand(tt.cfg)
			require.NoError(t, err)

			var cmd map[string]any
			err = json.Unmarshal(cmdBytes, &cmd)
			require.NoError(t, err)

			tt.checkCmd(t, cmd)
		})
	}
}

// ---------------------------------------------------------------------------
// TestBuildLGHvacr02ControlCommand - 제어 명령 빌드 테스트
// ---------------------------------------------------------------------------

func TestBuildLGHvacr02ControlCommand(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]any
		cfg      LGHvacr02NodeConfig
		wantErr  error
		checkCmd func(t *testing.T, cmd map[string]any)
	}{
		{
			name:    "set_multiple 생성",
			payload: map[string]any{"power": "on", "temperature": 22.0},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: "0x10"},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgHvacr02CmdSetMultiple, cmd["command"])
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
			cfg:     LGHvacr02NodeConfig{DefaultAddress: "0x10"},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "0xFF", cmd["address"])
			},
		},
		{
			name:    "address 없으면 에러",
			payload: map[string]any{"power": "on"},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: ""},
			wantErr: ErrLGHvacr02MissingAddress,
		},
		{
			name:    "직접 command 전달",
			payload: map[string]any{"command": "custom_cmd", "params": map[string]any{"k": "v"}},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: "0x10"},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "custom_cmd", cmd["command"])
				assert.Equal(t, "0x10", cmd["address"])
				assert.NotNil(t, cmd["params"])
			},
		},
		{
			name:    "직접 command + address 없음 -> 에러",
			payload: map[string]any{"command": "custom_cmd"},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: ""},
			wantErr: ErrLGHvacr02MissingAddress,
		},
		{
			name:    "제어 키 없으면 상태 조회로 폴백",
			payload: map[string]any{"some_key": "value"},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: "0x10", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, lgHvacr02CmdGetStats, cmd["command"], "제어 키가 없으면 상태 조회로 폴백")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))

			cmdBytes, err := buildLGHvacr02ControlCommand(msg, tt.cfg)

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
// TestApplyLGHvacr02Overrides - 오버라이드 테스트
// ---------------------------------------------------------------------------

func TestApplyLGHvacr02Overrides(t *testing.T) {
	tests := []struct {
		name     string
		payload  map[string]any
		cfg      LGHvacr02NodeConfig
		checkCfg func(t *testing.T, cfg LGHvacr02NodeConfig)
	}{
		{
			name:    "address 오버라이드",
			payload: map[string]any{"address": "0xFF"},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: "0x10"},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, "0xFF", cfg.DefaultAddress)
			},
		},
		{
			name:    "빈 address는 오버라이드 안 함",
			payload: map[string]any{"address": ""},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: "0x10"},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, "0x10", cfg.DefaultAddress)
			},
		},
		{
			name:    "timeout 오버라이드",
			payload: map[string]any{"timeout": "10s"},
			cfg:     LGHvacr02NodeConfig{Timeout: "5s"},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, "10s", cfg.Timeout)
			},
		},
		{
			name:    "poll_command 오버라이드",
			payload: map[string]any{"poll_command": "get_recent"},
			cfg:     LGHvacr02NodeConfig{PollCommand: lgHvacr02CmdGetStats},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, "get_recent", cfg.PollCommand)
			},
		},
		{
			name:    "count 오버라이드 (float64)",
			payload: map[string]any{"count": float64(5)},
			cfg:     LGHvacr02NodeConfig{RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, 5, cfg.RecentCount)
			},
		},
		{
			name:    "count 오버라이드 (int)",
			payload: map[string]any{"count": 7},
			cfg:     LGHvacr02NodeConfig{RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, 7, cfg.RecentCount)
			},
		},
		{
			name:    "count 0 이하는 오버라이드 안 함",
			payload: map[string]any{"count": float64(0)},
			cfg:     LGHvacr02NodeConfig{RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, 10, cfg.RecentCount)
			},
		},
		{
			name:    "관련 없는 키는 무시",
			payload: map[string]any{"unknown_key": "value"},
			cfg:     LGHvacr02NodeConfig{DefaultAddress: "0x10", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10},
			checkCfg: func(t *testing.T, cfg LGHvacr02NodeConfig) {
				assert.Equal(t, "0x10", cfg.DefaultAddress)
				assert.Equal(t, lgHvacr02CmdGetStats, cfg.PollCommand)
				assert.Equal(t, 10, cfg.RecentCount)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			result := applyLGHvacr02Overrides(msg, tt.cfg)
			tt.checkCfg(t, result)
		})
	}
}

// ---------------------------------------------------------------------------
// 인터페이스 컴파일 타임 체크
// ---------------------------------------------------------------------------

// 컴파일 타임에 인터페이스 구현을 확인하는 보호 변수
var (
	_ Node       = (*LGHvacr02StatusNode)(nil)
	_ SourceNode = (*LGHvacr02StatusNode)(nil)
	_ Node       = (*LGHvacr02ControlNode)(nil)
	_ Node       = (*LGHvacr02Node)(nil)
	_ SourceNode = (*LGHvacr02Node)(nil)
)

// ---------------------------------------------------------------------------
// TestLGHvacr02Node_Timeout - 타임아웃 테스트
// ---------------------------------------------------------------------------

// TestLGHvacr02StatusNode_Process_타임아웃 은 Agent Process가 느릴 때 타임아웃이 발생하는지 확인한다.
func TestLGHvacr02StatusNode_Process_타임아웃(t *testing.T) {
	slow := &slowLGHvacr02Agent{delay: 2 * time.Second}
	n := newTestLGHvacr02StatusNode(slow)
	n.timeout = 100 * time.Millisecond
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{AgentRef: "test", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10, EmitMetadata: MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true}}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02ProcessFailed)
}

// ---------------------------------------------------------------------------
// TestLGHvacr02ControlNode_Process_타임아웃 은 Agent Process가 느릴 때 타임아웃이 발생하는지 확인한다.
// ---------------------------------------------------------------------------

func TestLGHvacr02ControlNode_Process_타임아웃(t *testing.T) {
	slow := &slowLGHvacr02Agent{delay: 2 * time.Second}
	n := newTestLGHvacr02ControlNode(slow)
	n.timeout = 100 * time.Millisecond
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{
		AgentRef:       "test",
		DefaultAddress: "0x10",
		PollCommand:    lgHvacr02CmdGetStats,
		RecentCount:    10,
	}

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"power": "on",
	})))
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrLGHvacr02ProcessFailed)
}

// ---------------------------------------------------------------------------
// Registry 등록 테스트
// ---------------------------------------------------------------------------

// TestRegistry_LGHvacr02노드등록 은 LGCP 노드가 Registry에 등록되어 있는지 확인한다.
func TestRegistry_LGHvacr02노드등록(t *testing.T) {
	r := NewRegistry()

	lgHvacr02Types := []string{"lg_hvacr02_status", "lg_hvacr02_control", "lg_hvacr02"}
	for _, typeName := range lgHvacr02Types {
		t.Run(typeName, func(t *testing.T) {
			assert.True(t, r.Has(typeName), "%s가 레지스트리에 등록되어 있어야 한다", typeName)

			meta, ok := r.TypeMeta(typeName)
			assert.True(t, ok)
			assert.Equal(t, "io", meta.Category)
			assert.Equal(t, "builtin", meta.Source)
		})
	}
}

// ===========================================================================
// 벌크 폴링 테스트 (BatchSize, pollRecentBulk, lastSeq 중복 제거)
// ===========================================================================

// TestLGHvacr02StatusNode_Configure_BatchSize 는 batch_size 설정이 올바르게 파싱되는지 확인한다.
func TestLGHvacr02StatusNode_Configure_BatchSize(t *testing.T) {
	tests := []struct {
		name          string
		config        map[string]any
		wantBatchSize int
	}{
		{
			name: "기본값 32",
			config: map[string]any{
				"agent_ref": "lg_hvacr02-1",
			},
			wantBatchSize: 32,
		},
		{
			name: "int 타입",
			config: map[string]any{
				"agent_ref":  "lg_hvacr02-1",
				"batch_size": 64,
			},
			wantBatchSize: 64,
		},
		{
			name: "float64 타입 (JSON 디코딩)",
			config: map[string]any{
				"agent_ref":  "lg_hvacr02-1",
				"batch_size": float64(16),
			},
			wantBatchSize: 16,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newLGHvacr02NodeDef("test-batch", "lg_hvacr02_status")
			node, err := NewLGHvacr02StatusNode(def)
			require.NoError(t, err)

			n := node.(*LGHvacr02StatusNode)
			err = n.Configure(tt.config)
			require.NoError(t, err)
			assert.Equal(t, tt.wantBatchSize, n.lgHvacr02Cfg.BatchSize)
		})
	}
}

// 2026-05-30: Status pollRecentBulk / pollLoop 의존 테스트 5종 삭제.
// status 노드가 receiveLoop / drainNewFrames / inactivity-fallback 모델로 전환됨에
// 따라 pollRecentBulk / pollLoop 메서드가 status 노드에서 제거되었다. combined
// 노드 (LGHvacr02Node) 의 동등 테스트 (TestLGHvacr02Node_pollRecentBulk_벌크수신)
// 는 그대로 유지된다. 새 모델의 회귀 테스트는 별도 commit 에서 추가.
//
// 삭제된 status 테스트:
//   - TestLGHvacr02StatusNode_pollRecentBulk_새프레임전송
//   - TestLGHvacr02StatusNode_pollRecentBulk_중복제거
//   - TestLGHvacr02StatusNode_pollRecentBulk_채널풀_중단
//   - TestLGHvacr02StatusNode_pollLoop_벌크디스패치
//   - TestLGHvacr02StatusNode_pollRecentBulk_빈응답

// (Combined node 의 pollRecentBulk 테스트는 아래에서 그대로 유지.)

// TestLGHvacr02Node_pollRecentBulk_벌크수신 은 통합 노드(LGHvacr02Node)에서도
// 벌크 수신이 동일하게 동작하는지 확인한다.
func TestLGHvacr02Node_pollRecentBulk_벌크수신(t *testing.T) {
	frames := []json.RawMessage{
		json.RawMessage(`{"seq": 2, "mode": "cool"}`),
		json.RawMessage(`{"seq": 1, "mode": "heat"}`),
	}
	respBytes, _ := json.Marshal(map[string]any{
		"count":  2,
		"frames": frames,
	})
	mockAgent := &mockLGHvacr02Agent{processResp: respBytes}

	n := newTestLGHvacr02Node(mockAgent)
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{
		AgentRef:    "test-agent",
		PollCommand: lgHvacr02CmdGetRecent,
		BatchSize:   32,
	}
	n.lastSeq = 0

	n.pollRecentBulk(n.lgHvacr02Cfg)

	// 시간순 (seq 1, 2)으로 전송
	assert.Equal(t, 2, len(n.sourceCh))

	msg1 := <-n.sourceCh
	seq1, _ := msg1.Payload().Get("seq")
	assert.Equal(t, float64(1), seq1)

	msg2 := <-n.sourceCh
	seq2, _ := msg2.Payload().Get("seq")
	assert.Equal(t, float64(2), seq2)

	assert.Equal(t, int64(2), n.lastSeq)
}

// ===========================================================================
// msg.Type() 통일 분류 표준 테스트 (SPEC-MESSAGE-TYPE-001)
// ===========================================================================
//
// 모든 agent 노드는 emit 하는 메시지에 1급 Message.Type() 을 설정한다.
// 본 그룹은 LGCP 노드의 poll → event, Process → response 두 경로를 검증한다.

// 2026-05-30: TestLGHvacr02StatusNode_Poll_SetsMessageTypeDeviceStatePoll 삭제.
// status 노드가 receiveLoop 모델로 전환됨에 따라 pollLoop 의존 검증 불가.
// receiveLoop 가 emit 하는 메시지의 msg.Type()="device_state.poll" / node_source="push"
// 검증은 새 회귀 테스트로 별도 commit 추가 예정.

// TestLGHvacr02StatusNode_Process_SetsMessageTypeDeviceStateResponse 는 Process 응답이
// msg.Type()="device_state.response" 와 lg_hvacr02_source="request" 를 모두
// 가지는지 확인한다 (v0.8.0 계층형 분류).
func TestLGHvacr02StatusNode_Process_SetsMessageTypeDeviceStateResponse(t *testing.T) {
	respBytes, err := json.Marshal(map[string]any{"power": "on"})
	require.NoError(t, err)

	mockAgent := &mockLGHvacr02Agent{processResp: respBytes}
	n := newTestLGHvacr02StatusNode(mockAgent)

	n.mu.Lock()
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{AgentRef: "test-agent", PollCommand: lgHvacr02CmdGetStats, RecentCount: 10, EmitMetadata: MetadataEmitOptions{NodeID: true, DeviceType: true, Label: true, NodeSource: true}}
	n.mu.Unlock()

	results, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "device_state.response", results[0].Type(), "Process 응답은 device_state.response 분류여야 한다")

	// v0.10.0: lg_hvacr02_source="request" 제거됨 — message_type="device_state.response" 가 단일 식별자.
	_, srcOK := results[0].Metadata().Get("node_source")
	assert.False(t, srcOK, "v0.10.0: Process 응답에는 lg_hvacr02_source 가 설정되지 않아야 함")
}

// 사용하지 않는 import 방지를 위한 변수
var _ = lg.Hvacr02Agent{}
