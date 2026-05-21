package node

import (
	"context"
	"encoding/json"
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
// NASA 테스트용 모의 객체 정의
// ---------------------------------------------------------------------------

// mockNASAAgent 는 테스트용 agent.Agent 구현이다.
// Process() 호출 시 수신한 데이터를 기록하고 미리 설정된 응답을 반환한다.
type mockNASAAgent struct {
	processData []byte // 마지막 Process() 호출 시 전달된 데이터
	processResp []byte // Process() 호출 시 반환할 응답
	processErr  error  // Process() 호출 시 반환할 에러
}

func (m *mockNASAAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockNASAAgent) Start(_ context.Context) error       { return nil }
func (m *mockNASAAgent) Stop(_ context.Context) error        { return nil }
func (m *mockNASAAgent) Pause(_ context.Context) error       { return nil }
func (m *mockNASAAgent) Resume(_ context.Context) error      { return nil }
func (m *mockNASAAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockNASAAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockNASAAgent) ID() string                          { return "mock-nasa" }
func (m *mockNASAAgent) Name() string                        { return "mock-nasa" }
func (m *mockNASAAgent) Type() string                        { return "samsung-nasa" }
func (m *mockNASAAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockNASAAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockNASAAgent) Process(data []byte) ([]byte, error) {
	m.processData = data
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.processResp, nil
}

// slowNASAAgent 는 Process() 호출 시 지연을 발생시키는 테스트용 Agent이다.
type slowNASAAgent struct {
	delay time.Duration
}

func (m *slowNASAAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *slowNASAAgent) Start(_ context.Context) error       { return nil }
func (m *slowNASAAgent) Stop(_ context.Context) error        { return nil }
func (m *slowNASAAgent) Pause(_ context.Context) error       { return nil }
func (m *slowNASAAgent) Resume(_ context.Context) error      { return nil }
func (m *slowNASAAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *slowNASAAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *slowNASAAgent) ID() string                          { return "slow-nasa" }
func (m *slowNASAAgent) Name() string                        { return "slow-nasa" }
func (m *slowNASAAgent) Type() string                        { return "samsung-nasa" }
func (m *slowNASAAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *slowNASAAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *slowNASAAgent) Process(_ []byte) ([]byte, error) {
	time.Sleep(m.delay)
	return []byte(`{"ok": true}`), nil
}

// mockNASAResolver 는 테스트용 AgentResolver 구현이다.
type mockNASAResolver struct {
	transport AgentTransport
	err       error
}

func (m *mockNASAResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.transport, nil
}

// mockNASATransport 는 AgentTransport + AgentAccessor를 구현하는 테스트용 모의 객체이다.
type mockNASATransport struct {
	agent agent.Agent
}

func (m *mockNASATransport) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockNASATransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (m *mockNASATransport) UnderlyingAgent() agent.Agent {
	return m.agent
}

// mockNASATransportNoAccessor 는 AgentAccessor를 구현하지 않는 AgentTransport이다.
type mockNASATransportNoAccessor struct{}

func (m *mockNASATransportNoAccessor) Send(_ context.Context, _ message.Message) error {
	return nil
}

func (m *mockNASATransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// ---------------------------------------------------------------------------
// NASA 테스트용 헬퍼 함수
// ---------------------------------------------------------------------------

// newNASANodeDef 는 테스트용 NodeDef를 생성한다.
func newNASANodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// newTestNASAStatusNode 는 테스트용 NASAStatusNode를 agent를 직접 주입하여 생성한다.
// Init()을 우회하여 samsung.NASAAgent 타입 검사를 건너뛴다.
func newTestNASAStatusNode(mockAgent agent.Agent) *NASAStatusNode {
	def := newNASANodeDef("test-status", "nasa-status")
	base := NewBaseNode(def)
	n := &NASAStatusNode{
		nasaNodeBase: nasaNodeBase{
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

// newTestNASAControlNode 는 테스트용 NASAControlNode를 agent를 직접 주입하여 생성한다.
func newTestNASAControlNode(mockAgent agent.Agent) *NASAControlNode {
	def := newNASANodeDef("test-control", "nasa-control")
	base := NewBaseNode(def)
	n := &NASAControlNode{
		nasaNodeBase: nasaNodeBase{
			BaseNode: base,
			timeout:  5 * time.Second,
			agent:    mockAgent,
		},
	}
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// newTestNASANode 는 테스트용 NASANode를 agent를 직접 주입하여 생성한다.
func newTestNASANode(mockAgent agent.Agent) *NASANode {
	def := newNASANodeDef("test-nasa", "nasa")
	base := NewBaseNode(def)
	n := &NASANode{
		nasaNodeBase: nasaNodeBase{
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

// parseNASAProcessCommand 는 Agent.Process()에 전달된 JSON 바이트를 파싱하여 map으로 반환한다.
func parseNASAProcessCommand(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var cmd map[string]any
	err := json.Unmarshal(data, &cmd)
	require.NoError(t, err, "NASA Process 명령 JSON 파싱 실패")
	return cmd
}

// ===========================================================================
// R18: NASAStatusNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewNASAStatusNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewNASAStatusNode_정상생성 은 NASAStatusNode가 올바르게 생성되는지 확인한다.
func TestNewNASAStatusNode_정상생성(t *testing.T) {
	def := newNASANodeDef("status-1", "nasa-status")
	node, err := NewNASAStatusNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "status-1", node.Name())
	assert.Equal(t, "nasa-status", node.Type())
}

// TestNewNASAStatusNode_Resolver옵션 은 WithAgentResolver 옵션으로 resolver가 설정되는지 확인한다.
func TestNewNASAStatusNode_Resolver옵션(t *testing.T) {
	resolver := &mockNASAResolver{}
	def := newNASANodeDef("status-resolver", "nasa-status")
	node, err := NewNASAStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASAStatusNode)
	assert.NotNil(t, n.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestNASAStatusNode_Configure - 설정 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestNASAStatusNode_Configure 는 Configure 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestNASAStatusNode_Configure(t *testing.T) {
	tests := []struct {
		name      string
		config    map[string]any
		wantErr   error
		checkFunc func(t *testing.T, n *NASAStatusNode)
	}{
		{
			name: "agent_ref 누락 에러",
			config: map[string]any{
				"device_id": "dev-1",
			},
			wantErr: ErrNASAMissingAgentRef,
		},
		{
			name: "agent_ref 빈문자열 에러",
			config: map[string]any{
				"agent_ref": "",
			},
			wantErr: ErrNASAMissingAgentRef,
		},
		{
			name: "기본값 적용",
			config: map[string]any{
				"agent_ref": "nasa-agent-1",
			},
			checkFunc: func(t *testing.T, n *NASAStatusNode) {
				assert.Equal(t, "nasa-agent-1", n.nasaCfg.AgentRef)
				assert.Equal(t, "", n.nasaCfg.DeviceID, "device_id 기본값 빈문자열")
				assert.Equal(t, "30s", n.nasaCfg.PollInterval, "poll_interval 기본값 30s")
				assert.Equal(t, "5s", n.nasaCfg.Timeout, "timeout 기본값 5s")
				assert.Equal(t, 5*time.Second, n.timeout)
				assert.Equal(t, 30*time.Second, n.pollInterval)
			},
		},
		{
			name: "커스텀 값 적용",
			config: map[string]any{
				"agent_ref":     "my-nasa",
				"device_id":     "hvac-001",
				"poll_interval": "10s",
				"timeout":       "15s",
			},
			checkFunc: func(t *testing.T, n *NASAStatusNode) {
				assert.Equal(t, "my-nasa", n.nasaCfg.AgentRef)
				assert.Equal(t, "hvac-001", n.nasaCfg.DeviceID)
				assert.Equal(t, "10s", n.nasaCfg.PollInterval)
				assert.Equal(t, "15s", n.nasaCfg.Timeout)
				assert.Equal(t, 15*time.Second, n.timeout)
				assert.Equal(t, 10*time.Second, n.pollInterval)
			},
		},
		{
			name: "잘못된 timeout 시 기본값 적용",
			config: map[string]any{
				"agent_ref": "nasa-agent-1",
				"timeout":   "invalid",
			},
			checkFunc: func(t *testing.T, n *NASAStatusNode) {
				assert.Equal(t, nasaDefaultTimeout, n.timeout, "잘못된 timeout은 기본값으로 대체")
			},
		},
		{
			name: "잘못된 poll_interval 시 기본값 적용",
			config: map[string]any{
				"agent_ref":     "nasa-agent-1",
				"poll_interval": "not-a-duration",
			},
			checkFunc: func(t *testing.T, n *NASAStatusNode) {
				assert.Equal(t, nasaDefaultPollInterval, n.pollInterval, "잘못된 poll_interval은 기본값으로 대체")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			def := newNASANodeDef("test-cfg", "nasa-status")
			node, err := NewNASAStatusNode(def)
			require.NoError(t, err)

			n := node.(*NASAStatusNode)
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
// 3. TestNASAStatusNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestNASAStatusNode_Init_Resolver없음_에러 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestNASAStatusNode_Init_Resolver없음_에러(t *testing.T) {
	def := newNASANodeDef("init-no-resolver", "nasa-status")
	node, err := NewNASAStatusNode(def) // resolver 없이 생성
	require.NoError(t, err)

	n := node.(*NASAStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "nasa-1"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASANoResolver)
}

// TestNASAStatusNode_Init_비NASA_Agent_에러 는 resolve된 Agent가 Samsung NASA 타입이 아닐 때 에러를 반환하는지 확인한다.
func TestNASAStatusNode_Init_비NASA_Agent_에러(t *testing.T) {
	// mockNASAAgent는 *samsung.NASAAgent 타입이 아니므로 에러가 발생한다.
	// 이는 타입 불일치로 인한 구성 오류이며, 런타임 agent not found 가 아니다.
	// 하지만 현재 코드에서는 이를 구성 오류로 분류하지 않으므로 Init이 성공한다.
	// (타입 체크는 initAgent 이후에 발생하므로)
	fakeAgent := &mockNASAAgent{}
	transport := &mockNASATransport{agent: fakeAgent}
	resolver := &mockNASAResolver{transport: transport}

	def := newNASANodeDef("init-non-nasa", "nasa-status")
	node, err := NewNASAStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASAStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "fake-nasa"})
	require.NoError(t, err)

	// 타입 불일치는 여전히 에러
	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASAAgentNotNASA)
}

// TestNASAStatusNode_Init_AgentAccessor_미지원_에러 는 transport가 AgentAccessor를 구현하지 않을 때 에러를 반환하는지 확인한다.
func TestNASAStatusNode_Init_AgentAccessor_미지원_에러(t *testing.T) {
	transport := &mockNASATransportNoAccessor{}
	resolver := &mockNASAResolver{transport: transport}

	def := newNASANodeDef("init-no-accessor", "nasa-status")
	node, err := NewNASAStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASAStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "no-accessor"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASAAgentNotNASA)
}

// TestNASAStatusNode_Init_Resolver실패_에러 는 Agent resolve 실패 시 플로우는 시작되지만 노드는 대기 상태가 되는지 확인한다.
// 이제 agent not found는 runtime 에러로 처리되어 플로우가 계속 진행된다.
func TestNASAStatusNode_Init_Resolver실패_에러(t *testing.T) {
	resolver := &mockNASAResolver{err: assert.AnError}

	def := newNASANodeDef("init-resolve-err", "nasa-status")
	node, err := NewNASAStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASAStatusNode)
	err = n.Configure(map[string]any{"agent_ref": "missing-agent"})
	require.NoError(t, err)

	// 에이전트를 찾을 수 없어도 Init은 성공하고, 노드는 Running 상태로 진행
	err = n.Init(context.Background())
	require.NoError(t, err)

	// 노드는 agent nil 상태 (나중에 Reinit으로 연결됨)
	require.Nil(t, n.agent)
}

// ---------------------------------------------------------------------------
// 4. TestNASAStatusNode_Process - 상태 조회 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestNASAStatusNode_Process 는 Process 메서드의 다양한 시나리오를 테이블 기반으로 테스트한다.
func TestNASAStatusNode_Process(t *testing.T) {
	tests := []struct {
		name        string
		deviceID    string
		msgPayload  map[string]any
		agentResp   map[string]any
		wantCommand string
		checkCmd    func(t *testing.T, cmd map[string]any)
		checkOutput func(t *testing.T, out message.Message)
	}{
		{
			name:        "device_id 미설정 -> get_all_states",
			deviceID:    "",
			agentResp:   map[string]any{"devices": []any{"dev-1", "dev-2"}},
			wantCommand: nasaCmdGetAllState,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				_, hasDeviceID := cmd["device_id"]
				assert.False(t, hasDeviceID, "device_id가 포함되지 않아야 한다")
			},
		},
		{
			name:        "device_id 설정 -> get_state",
			deviceID:    "hvac-001",
			agentResp:   map[string]any{"power": "on", "temperature": 24.0},
			wantCommand: nasaCmdGetState,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "hvac-001", cmd["device_id"])
			},
		},
		{
			name:     "payload device_id 오버라이드",
			deviceID: "original-dev",
			msgPayload: map[string]any{
				"device_id": "override-dev",
			},
			agentResp:   map[string]any{"power": "off"},
			wantCommand: nasaCmdGetState,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "override-dev", cmd["device_id"], "payload의 device_id가 우선")
			},
		},
		{
			name:     "payload에 빈 device_id -> config 값 유지",
			deviceID: "config-dev",
			msgPayload: map[string]any{
				"device_id": "",
			},
			agentResp:   map[string]any{"status": "idle"},
			wantCommand: nasaCmdGetState,
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "config-dev", cmd["device_id"], "빈 문자열은 오버라이드하지 않음")
			},
		},
		{
			name:        "응답의 모든 키가 출력 payload에 포함",
			deviceID:    "dev-1",
			agentResp:   map[string]any{"power": "on", "mode": "cool", "temperature": 22.5},
			wantCommand: nasaCmdGetState,
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
				source, ok := out.Metadata().Get("nasa_source")
				assert.True(t, ok)
				assert.Equal(t, "request", source)

				nodeID, ok := out.Metadata().Get("nasa_node_id")
				assert.True(t, ok)
				assert.NotEmpty(t, nodeID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockNASAAgent{processResp: respBytes}
			n := newTestNASAStatusNode(mockAgent)

			// config 설정
			n.mu.Lock()
			n.nasaCfg = NASANodeConfig{
				AgentRef: "test-agent",
				DeviceID: tt.deviceID,
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
			cmd := parseNASAProcessCommand(t, mockAgent.processData)
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

// TestNASAStatusNode_Process_AgentNil_에러 는 agent가 nil일 때 에러를 반환하는지 확인한다.
func TestNASAStatusNode_Process_AgentNil_에러(t *testing.T) {
	n := newTestNASAStatusNode(nil)
	n.nasaCfg = NASANodeConfig{AgentRef: "test"}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASAProcessFailed)
}

// TestNASAStatusNode_Process_유효하지않은응답_에러 는 Agent 응답이 유효하지 않은 JSON일 때 에러를 반환하는지 확인한다.
func TestNASAStatusNode_Process_유효하지않은응답_에러(t *testing.T) {
	mockAgent := &mockNASAAgent{processResp: []byte("invalid json")}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test"}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASAProcessFailed)
	assert.Contains(t, err.Error(), "invalid response JSON")
}

// TestNASAStatusNode_Process_AgentError_에러 는 Agent Process() 에러가 전파되는지 확인한다.
func TestNASAStatusNode_Process_AgentError_에러(t *testing.T) {
	mockAgent := &mockNASAAgent{processErr: assert.AnError}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test"}

	msg := message.New()
	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASAProcessFailed)
}

// ---------------------------------------------------------------------------
// 5. TestNASAStatusNode_SourceNode - 폴링 테스트
// ---------------------------------------------------------------------------

// TestNASAStatusNode_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestNASAStatusNode_SourceCh(t *testing.T) {
	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// TestNASAStatusNode_SourceNode_폴링 은 pollLoop가 sourceCh에 메시지를 전달하는지 확인한다.
func TestNASAStatusNode_SourceNode_폴링(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"power": "on", "temperature": 25.0})
	mockAgent := &mockNASAAgent{processResp: respBytes}

	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent"}
	n.pollInterval = 50 * time.Millisecond // 빠른 폴링으로 테스트

	// 폴링 고루틴 시작
	go n.pollLoop()

	// 메시지 수신 대기 (최대 1초)
	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)
		v, ok := msg.Payload().Get("power")
		assert.True(t, ok)
		assert.Equal(t, "on", v)

		source, ok := msg.Metadata().Get("nasa_source")
		assert.True(t, ok)
		assert.Equal(t, "poll", source)
	case <-time.After(1 * time.Second):
		t.Fatal("폴링 메시지가 1초 내에 도착하지 않았다")
	}

	// 정리
	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 6. TestNASAStatusNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestNASAStatusNode_Shutdown_폴링정지 는 Shutdown 시 폴링이 정지되는지 확인한다.
func TestNASAStatusNode_Shutdown_폴링정지(t *testing.T) {
	// slowNASAAgent를 사용하여 폴링 중 응답이 느리게 오도록 함
	respBytes, _ := json.Marshal(map[string]any{"ok": true})
	mockAgent := &mockNASAAgent{processResp: respBytes}

	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent"}
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
	// 인플라이트 메시지 포함하여 sourceCh를 모두 드레인한 뒤,
	// 폴링이 멈추었음을 확인한다.
	// 드레인: Shutdown 직후 인플라이트 메시지가 있을 수 있으므로 짧은 대기 후 드레인
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
	// 드레인 후 충분히 대기하여 폴링이 실제로 멈추었는지 확인
	time.Sleep(300 * time.Millisecond)
	select {
	case <-n.sourceCh:
		t.Fatal("Shutdown 후 새 메시지가 생성되면 안 된다")
	default:
		// 기대하는 동작: 새 메시지 없음
	}
}

// TestNASAStatusNode_Shutdown_중복호출 은 Shutdown을 여러 번 호출해도 패닉하지 않는지 확인한다.
func TestNASAStatusNode_Shutdown_중복호출(t *testing.T) {
	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)

	// 두 번째 Shutdown은 stopCh가 이미 닫혀 있지만 pollOnce로 패닉하지 않아야 한다
	// 단, lifecycle 상태 전이가 이미 Stopping이므로 에러가 발생할 수 있다
	_ = n.Shutdown(context.Background())
}

// ===========================================================================
// R19: NASAControlNode 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewNASAControlNode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewNASAControlNode_정상생성 은 NASAControlNode가 올바르게 생성되는지 확인한다.
func TestNewNASAControlNode_정상생성(t *testing.T) {
	def := newNASANodeDef("control-1", "nasa-control")
	node, err := NewNASAControlNode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "control-1", node.Name())
	assert.Equal(t, "nasa-control", node.Type())
}

// TestNewNASAControlNode_Resolver옵션 은 WithAgentResolver 옵션으로 resolver가 설정되는지 확인한다.
func TestNewNASAControlNode_Resolver옵션(t *testing.T) {
	resolver := &mockNASAResolver{}
	def := newNASANodeDef("control-resolver", "nasa-control")
	node, err := NewNASAControlNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASAControlNode)
	assert.NotNil(t, n.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestNASAControlNode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestNASAControlNode_Configure_AgentRef필수 는 agent_ref 누락 시 에러를 반환하는지 확인한다.
func TestNASAControlNode_Configure_AgentRef필수(t *testing.T) {
	def := newNASANodeDef("ctrl-cfg", "nasa-control")
	node, err := NewNASAControlNode(def)
	require.NoError(t, err)

	n := node.(*NASAControlNode)
	err = n.Configure(map[string]any{"device_id": "dev-1"})
	assert.ErrorIs(t, err, ErrNASAMissingAgentRef)
}

// TestNASAControlNode_Configure_정상 은 Configure가 정상적으로 동작하는지 확인한다.
func TestNASAControlNode_Configure_정상(t *testing.T) {
	def := newNASANodeDef("ctrl-cfg-ok", "nasa-control")
	node, err := NewNASAControlNode(def)
	require.NoError(t, err)

	n := node.(*NASAControlNode)
	err = n.Configure(map[string]any{
		"agent_ref": "my-nasa",
		"device_id": "hvac-001",
		"timeout":   "10s",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-nasa", n.nasaCfg.AgentRef)
	assert.Equal(t, "hvac-001", n.nasaCfg.DeviceID)
	assert.Equal(t, 10*time.Second, n.timeout)
}

// ---------------------------------------------------------------------------
// 3. TestNASAControlNode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestNASAControlNode_Init_Resolver없음_에러 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestNASAControlNode_Init_Resolver없음_에러(t *testing.T) {
	def := newNASANodeDef("ctrl-init-no-resolver", "nasa-control")
	node, err := NewNASAControlNode(def)
	require.NoError(t, err)

	n := node.(*NASAControlNode)
	err = n.Configure(map[string]any{"agent_ref": "nasa-1"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASANoResolver)
}

// TestNASAControlNode_Init_비NASA_Agent_에러 는 비-NASA Agent 시 에러를 반환하는지 확인한다.
func TestNASAControlNode_Init_비NASA_Agent_에러(t *testing.T) {
	fakeAgent := &mockNASAAgent{}
	transport := &mockNASATransport{agent: fakeAgent}
	resolver := &mockNASAResolver{transport: transport}

	def := newNASANodeDef("ctrl-init-non-nasa", "nasa-control")
	node, err := NewNASAControlNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASAControlNode)
	err = n.Configure(map[string]any{"agent_ref": "fake-nasa"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASAAgentNotNASA)
}

// ---------------------------------------------------------------------------
// 4. TestNASAControlNode_Process - 제어 명령 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestNASAControlNode_Process 는 다양한 제어 시나리오를 테이블 기반으로 테스트한다.
func TestNASAControlNode_Process(t *testing.T) {
	tests := []struct {
		name       string
		deviceID   string
		msgPayload map[string]any
		agentResp  map[string]any
		checkCmd   func(t *testing.T, cmd map[string]any)
		checkMeta  func(t *testing.T, out message.Message)
	}{
		{
			name:     "직접 command -> 패스스루",
			deviceID: "dev-1",
			msgPayload: map[string]any{
				"command": "set_power",
				"params":  map[string]any{"power": "on"},
			},
			agentResp: map[string]any{"ok": true},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "set_power", cmd["command"])
				assert.Equal(t, "dev-1", cmd["device_id"])
				params, ok := cmd["params"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "on", params["power"])
			},
		},
		{
			name:     "간소화된 제어키 -> set_multiple",
			deviceID: "dev-2",
			msgPayload: map[string]any{
				"power":       "on",
				"mode":        "cool",
				"temperature": 24.0,
			},
			agentResp: map[string]any{"ok": true},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdSetMultiple, cmd["command"])
				assert.Equal(t, "dev-2", cmd["device_id"])
				settings, ok := cmd["settings"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "on", settings["power"])
				assert.Equal(t, "cool", settings["mode"])
				assert.Equal(t, 24.0, settings["temperature"])
			},
		},
		{
			name:     "fan_speed 단일 제어키 -> set_multiple",
			deviceID: "",
			msgPayload: map[string]any{
				"fan_speed": "high",
			},
			agentResp: map[string]any{"ok": true},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdSetMultiple, cmd["command"])
				_, hasDeviceID := cmd["device_id"]
				assert.False(t, hasDeviceID, "device_id가 빈 문자열이면 포함되지 않아야 한다")
				settings, ok := cmd["settings"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "high", settings["fan_speed"])
			},
		},
		{
			name:     "config device_id 사용 (payload에 없을 때)",
			deviceID: "config-dev-001",
			msgPayload: map[string]any{
				"power": "off",
			},
			agentResp: map[string]any{"ok": true},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "config-dev-001", cmd["device_id"])
			},
		},
		{
			name:     "payload device_id 오버라이드",
			deviceID: "original",
			msgPayload: map[string]any{
				"device_id": "overridden",
				"power":     "on",
			},
			agentResp: map[string]any{"ok": true},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "overridden", cmd["device_id"])
			},
		},
		{
			name:     "메타데이터 확인",
			deviceID: "",
			msgPayload: map[string]any{
				"command": "set_mode",
			},
			agentResp: map[string]any{"ok": true},
			checkMeta: func(t *testing.T, out message.Message) {
				cmdType, ok := out.Metadata().Get("nasa_command")
				assert.True(t, ok)
				assert.Equal(t, "control", cmdType)

				nodeID, ok := out.Metadata().Get("nasa_node_id")
				assert.True(t, ok)
				assert.NotEmpty(t, nodeID)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockNASAAgent{processResp: respBytes}
			n := newTestNASAControlNode(mockAgent)

			n.mu.Lock()
			n.nasaCfg = NASANodeConfig{
				AgentRef: "test-agent",
				DeviceID: tt.deviceID,
			}
			n.mu.Unlock()

			msg := message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			results, err := n.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			cmd := parseNASAProcessCommand(t, mockAgent.processData)

			if tt.checkCmd != nil {
				tt.checkCmd(t, cmd)
			}

			if tt.checkMeta != nil {
				tt.checkMeta(t, results[0])
			}
		})
	}
}

// TestNASAControlNode_Process_제어키없음_상태조회폴백 은 제어 키도 command 키도 없을 때 상태 조회로 폴백하는지 확인한다.
func TestNASAControlNode_Process_제어키없음_상태조회폴백(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"devices": []any{"dev-1"}})
	mockAgent := &mockNASAAgent{processResp: respBytes}
	n := newTestNASAControlNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent"}

	// 제어 키도 command도 없는 payload
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"some_other_key": "value",
	})))

	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	// 상태 조회 명령으로 폴백되어야 한다
	cmd := parseNASAProcessCommand(t, mockAgent.processData)
	assert.Equal(t, nasaCmdGetAllState, cmd["command"])
}

// TestNASAControlNode_Process_타임아웃 은 Agent 타임아웃이 정상 동작하는지 확인한다.
func TestNASAControlNode_Process_타임아웃(t *testing.T) {
	slowAgent := &slowNASAAgent{delay: 2 * time.Second}

	n := newTestNASAControlNode(slowAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent"}
	n.timeout = 100 * time.Millisecond // 짧은 타임아웃

	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{
		"command": "set_power",
	})))

	_, err := n.Process(context.Background(), msg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

// ---------------------------------------------------------------------------
// 5. TestNASAControlNode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestNASAControlNode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestNASAControlNode_Shutdown_상태전이(t *testing.T) {
	mockAgent := &mockNASAAgent{}
	n := newTestNASAControlNode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// ===========================================================================
// R20: NASANode (통합) 테스트
// ===========================================================================

// ---------------------------------------------------------------------------
// 1. TestNewNASANode - 팩토리 테스트
// ---------------------------------------------------------------------------

// TestNewNASANode_정상생성 은 NASANode가 올바르게 생성되는지 확인한다.
func TestNewNASANode_정상생성(t *testing.T) {
	def := newNASANodeDef("nasa-1", "nasa")
	node, err := NewNASANode(def)
	require.NoError(t, err)
	assert.NotNil(t, node)
	assert.Equal(t, "nasa-1", node.Name())
	assert.Equal(t, "nasa", node.Type())
}

// TestNewNASANode_Resolver옵션 은 WithAgentResolver 옵션으로 resolver가 설정되는지 확인한다.
func TestNewNASANode_Resolver옵션(t *testing.T) {
	resolver := &mockNASAResolver{}
	def := newNASANodeDef("nasa-resolver", "nasa")
	node, err := NewNASANode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASANode)
	assert.NotNil(t, n.resolver)
}

// ---------------------------------------------------------------------------
// 2. TestNASANode_Configure - 설정 테스트
// ---------------------------------------------------------------------------

// TestNASANode_Configure_AgentRef필수 는 agent_ref 누락 시 에러를 반환하는지 확인한다.
func TestNASANode_Configure_AgentRef필수(t *testing.T) {
	def := newNASANodeDef("nasa-cfg", "nasa")
	node, err := NewNASANode(def)
	require.NoError(t, err)

	n := node.(*NASANode)
	err = n.Configure(map[string]any{})
	assert.ErrorIs(t, err, ErrNASAMissingAgentRef)
}

// TestNASANode_Configure_정상 은 Configure가 정상적으로 동작하는지 확인한다.
func TestNASANode_Configure_정상(t *testing.T) {
	def := newNASANodeDef("nasa-cfg-ok", "nasa")
	node, err := NewNASANode(def)
	require.NoError(t, err)

	n := node.(*NASANode)
	err = n.Configure(map[string]any{
		"agent_ref":     "my-nasa",
		"device_id":     "hvac-001",
		"poll_interval": "5s",
		"timeout":       "3s",
	})
	require.NoError(t, err)
	assert.Equal(t, "my-nasa", n.nasaCfg.AgentRef)
	assert.Equal(t, "hvac-001", n.nasaCfg.DeviceID)
	assert.Equal(t, 3*time.Second, n.timeout)
	assert.Equal(t, 5*time.Second, n.pollInterval)
}

// ---------------------------------------------------------------------------
// 3. TestNASANode_Init - 초기화 테스트
// ---------------------------------------------------------------------------

// TestNASANode_Init_Resolver없음_에러 는 AgentResolver가 없을 때 에러를 반환하는지 확인한다.
func TestNASANode_Init_Resolver없음_에러(t *testing.T) {
	def := newNASANodeDef("nasa-init-no-resolver", "nasa")
	node, err := NewNASANode(def)
	require.NoError(t, err)

	n := node.(*NASANode)
	err = n.Configure(map[string]any{"agent_ref": "nasa-1"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASANoResolver)
}

// TestNASANode_Init_비NASA_Agent_에러 는 비-NASA Agent 시 에러를 반환하는지 확인한다.
func TestNASANode_Init_비NASA_Agent_에러(t *testing.T) {
	fakeAgent := &mockNASAAgent{}
	transport := &mockNASATransport{agent: fakeAgent}
	resolver := &mockNASAResolver{transport: transport}

	def := newNASANodeDef("nasa-init-non-nasa", "nasa")
	node, err := NewNASANode(def, WithAgentResolver(resolver))
	require.NoError(t, err)

	n := node.(*NASANode)
	err = n.Configure(map[string]any{"agent_ref": "fake-nasa"})
	require.NoError(t, err)

	err = n.Init(context.Background())
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASAAgentNotNASA)
}

// ---------------------------------------------------------------------------
// 4. TestNASANode_Process - 자동감지 테스트 (테이블 기반)
// ---------------------------------------------------------------------------

// TestNASANode_Process_자동감지 는 payload에 따라 상태 조회/제어를 자동 감지하는지 테스트한다.
func TestNASANode_Process_자동감지(t *testing.T) {
	tests := []struct {
		name        string
		deviceID    string
		msgPayload  map[string]any
		agentResp   map[string]any
		wantCmdType string // "status" or "control"
		checkCmd    func(t *testing.T, cmd map[string]any)
	}{
		{
			name:       "command 키만 있음 -> 상태 조회 (status, 제어 키 아님)",
			deviceID:   "dev-1",
			msgPayload: map[string]any{"command": "set_power", "params": map[string]any{"power": "on"}},
			agentResp:  map[string]any{"power": "on"},
			// "command" 키는 nasaControlKeys에 포함되지 않으므로 상태 조회로 라우팅됨
			wantCmdType: "status",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdGetState, cmd["command"])
				assert.Equal(t, "dev-1", cmd["device_id"])
			},
		},
		{
			name:       "command + 제어 키 함께 있음 -> 제어 (control, command 패스스루)",
			deviceID:   "dev-1",
			msgPayload: map[string]any{"command": "set_power", "power": "on", "params": map[string]any{"extra": true}},
			agentResp:  map[string]any{"ok": true},
			// power 키가 있으므로 hasNASAControlKeys가 true → buildControlCommand 호출
			// "command" 키가 있으면 직접 커맨드가 전달됨 (패스스루)
			wantCmdType: "control",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "set_power", cmd["command"])
			},
		},
		{
			name:        "power 키 있음 -> 제어 (control)",
			deviceID:    "dev-2",
			msgPayload:  map[string]any{"power": "on"},
			agentResp:   map[string]any{"ok": true},
			wantCmdType: "control",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdSetMultiple, cmd["command"])
			},
		},
		{
			name:        "mode 키 있음 -> 제어 (control)",
			deviceID:    "",
			msgPayload:  map[string]any{"mode": "heat"},
			agentResp:   map[string]any{"ok": true},
			wantCmdType: "control",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdSetMultiple, cmd["command"])
				settings, ok := cmd["settings"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "heat", settings["mode"])
			},
		},
		{
			name:        "temperature 키 있음 -> 제어 (control)",
			deviceID:    "dev-3",
			msgPayload:  map[string]any{"temperature": 22.5},
			agentResp:   map[string]any{"ok": true},
			wantCmdType: "control",
		},
		{
			name:        "fan_speed 키 있음 -> 제어 (control)",
			deviceID:    "dev-4",
			msgPayload:  map[string]any{"fan_speed": "low"},
			agentResp:   map[string]any{"ok": true},
			wantCmdType: "control",
		},
		{
			name:        "제어 키 없음 -> 상태 조회 (status)",
			deviceID:    "dev-5",
			msgPayload:  map[string]any{"some_other": "value"},
			agentResp:   map[string]any{"power": "on", "temperature": 24.0},
			wantCmdType: "status",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdGetState, cmd["command"])
				assert.Equal(t, "dev-5", cmd["device_id"])
			},
		},
		{
			name:        "빈 payload -> 상태 조회 (status)",
			deviceID:    "",
			msgPayload:  nil,
			agentResp:   map[string]any{"devices": []any{}},
			wantCmdType: "status",
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdGetAllState, cmd["command"])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			respBytes, err := json.Marshal(tt.agentResp)
			require.NoError(t, err)

			mockAgent := &mockNASAAgent{processResp: respBytes}
			n := newTestNASANode(mockAgent)

			n.mu.Lock()
			n.nasaCfg = NASANodeConfig{
				AgentRef: "test-agent",
				DeviceID: tt.deviceID,
			}
			n.mu.Unlock()

			var msg message.Message
			if tt.msgPayload != nil {
				msg = message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			} else {
				msg = message.New()
			}

			results, err := n.Process(context.Background(), msg)
			require.NoError(t, err)
			require.Len(t, results, 1)

			// 메타데이터에서 명령 타입 확인
			cmdType, ok := results[0].Metadata().Get("nasa_command")
			assert.True(t, ok)
			assert.Equal(t, tt.wantCmdType, cmdType)

			if tt.checkCmd != nil {
				cmd := parseNASAProcessCommand(t, mockAgent.processData)
				tt.checkCmd(t, cmd)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. TestNASANode_SourceNode - 폴링 테스트
// ---------------------------------------------------------------------------

// TestNASANode_SourceCh 는 SourceCh가 채널을 반환하는지 확인한다.
func TestNASANode_SourceCh(t *testing.T) {
	mockAgent := &mockNASAAgent{}
	n := newTestNASANode(mockAgent)

	ch := n.SourceCh()
	assert.NotNil(t, ch)
}

// TestNASANode_SourceNode_폴링 은 NASANode의 pollLoop가 sourceCh에 메시지를 전달하는지 확인한다.
func TestNASANode_SourceNode_폴링(t *testing.T) {
	respBytes, _ := json.Marshal(map[string]any{"power": "off"})
	mockAgent := &mockNASAAgent{processResp: respBytes}

	n := newTestNASANode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", DeviceID: "dev-1"}
	n.pollInterval = 50 * time.Millisecond

	go n.pollLoop()

	select {
	case msg := <-n.sourceCh:
		assert.NotNil(t, msg)
		v, ok := msg.Payload().Get("power")
		assert.True(t, ok)
		assert.Equal(t, "off", v)

		source, ok := msg.Metadata().Get("nasa_source")
		assert.True(t, ok)
		assert.Equal(t, "poll", source)
	case <-time.After(1 * time.Second):
		t.Fatal("NASANode 폴링 메시지가 1초 내에 도착하지 않았다")
	}

	close(n.stopCh)
}

// ---------------------------------------------------------------------------
// 6. TestNASANode_Shutdown - 종료 테스트
// ---------------------------------------------------------------------------

// TestNASANode_Shutdown_상태전이 는 Shutdown 시 Stopping 상태로 전이하는지 확인한다.
func TestNASANode_Shutdown_상태전이(t *testing.T) {
	mockAgent := &mockNASAAgent{}
	n := newTestNASANode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopping, n.CurrentState())
}

// TestNASANode_Shutdown_중복호출 은 Shutdown을 여러 번 호출해도 패닉하지 않는지 확인한다.
func TestNASANode_Shutdown_중복호출(t *testing.T) {
	mockAgent := &mockNASAAgent{}
	n := newTestNASANode(mockAgent)

	err := n.Shutdown(context.Background())
	require.NoError(t, err)

	_ = n.Shutdown(context.Background())
}

// ===========================================================================
// 헬퍼 함수 테스트
// ===========================================================================

// TestBuildStatusCommand 는 buildStatusCommand 함수를 테스트한다.
func TestBuildStatusCommand(t *testing.T) {
	tests := []struct {
		name        string
		cfg         NASANodeConfig
		wantCommand string
		hasDeviceID bool
	}{
		{
			name:        "device_id 없음 -> get_all_states",
			cfg:         NASANodeConfig{},
			wantCommand: nasaCmdGetAllState,
			hasDeviceID: false,
		},
		{
			name:        "device_id 있음 -> get_state",
			cfg:         NASANodeConfig{DeviceID: "hvac-001"},
			wantCommand: nasaCmdGetState,
			hasDeviceID: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := buildStatusCommand(tt.cfg, "test-node")
			require.NoError(t, err)

			var cmd map[string]any
			err = json.Unmarshal(data, &cmd)
			require.NoError(t, err)

			assert.Equal(t, tt.wantCommand, cmd["command"])

			if tt.hasDeviceID {
				assert.Equal(t, tt.cfg.DeviceID, cmd["device_id"])
			} else {
				_, ok := cmd["device_id"]
				assert.False(t, ok)
			}
		})
	}
}

// TestBuildControlCommand 는 buildControlCommand 함수를 테스트한다.
func TestBuildControlCommand(t *testing.T) {
	tests := []struct {
		name       string
		cfg        NASANodeConfig
		msgPayload map[string]any
		checkCmd   func(t *testing.T, cmd map[string]any)
	}{
		{
			name: "직접 command 전달",
			cfg:  NASANodeConfig{DeviceID: "dev-1"},
			msgPayload: map[string]any{
				"command": "set_power",
				"params":  map[string]any{"power": "on"},
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, "set_power", cmd["command"])
				assert.Equal(t, "dev-1", cmd["device_id"])
				params, ok := cmd["params"].(map[string]any)
				assert.True(t, ok)
				assert.Equal(t, "on", params["power"])
			},
		},
		{
			name: "빈 command 무시 -> 제어키 사용",
			cfg:  NASANodeConfig{},
			msgPayload: map[string]any{
				"command": "",
				"power":   "off",
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdSetMultiple, cmd["command"])
				settings, ok := cmd["settings"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, "off", settings["power"])
			},
		},
		{
			name:       "제어키 없음 -> 상태 조회 폴백",
			cfg:        NASANodeConfig{},
			msgPayload: map[string]any{"other": "value"},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				assert.Equal(t, nasaCmdGetAllState, cmd["command"])
			},
		},
		{
			name: "device_id 없으면 제어 명령에 미포함",
			cfg:  NASANodeConfig{DeviceID: ""},
			msgPayload: map[string]any{
				"power": "on",
			},
			checkCmd: func(t *testing.T, cmd map[string]any) {
				_, ok := cmd["device_id"]
				assert.False(t, ok, "빈 device_id는 명령에 포함되지 않아야 한다")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			data, err := buildControlCommand(msg, tt.cfg, "test-node")
			require.NoError(t, err)

			var cmd map[string]any
			err = json.Unmarshal(data, &cmd)
			require.NoError(t, err)

			tt.checkCmd(t, cmd)
		})
	}
}

// TestHasNASAControlKeys 는 hasNASAControlKeys 함수를 테스트한다.
func TestHasNASAControlKeys(t *testing.T) {
	tests := []struct {
		name    string
		payload map[string]any
		want    bool
	}{
		{"power 키", map[string]any{"power": "on"}, true},
		{"mode 키", map[string]any{"mode": "cool"}, true},
		{"temperature 키", map[string]any{"temperature": 24.0}, true},
		{"fan_speed 키", map[string]any{"fan_speed": "high"}, true},
		{"복합 제어 키", map[string]any{"power": "on", "mode": "cool"}, true},
		{"제어 키 없음", map[string]any{"other": "value"}, false},
		{"빈 payload", map[string]any{}, false},
		{"command만 있음 (제어 키 아님)", map[string]any{"command": "set_power"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.payload)))
			got := hasNASAControlKeys(msg)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestApplyNASAOverrides 는 applyNASAOverrides 함수를 테스트한다.
func TestApplyNASAOverrides(t *testing.T) {
	tests := []struct {
		name       string
		cfg        NASANodeConfig
		msgPayload map[string]any
		checkCfg   func(t *testing.T, cfg NASANodeConfig)
	}{
		{
			name: "device_id 오버라이드",
			cfg:  NASANodeConfig{DeviceID: "original"},
			msgPayload: map[string]any{
				"device_id": "overridden",
			},
			checkCfg: func(t *testing.T, cfg NASANodeConfig) {
				assert.Equal(t, "overridden", cfg.DeviceID)
			},
		},
		{
			name: "timeout 오버라이드",
			cfg:  NASANodeConfig{Timeout: "5s"},
			msgPayload: map[string]any{
				"timeout": "10s",
			},
			checkCfg: func(t *testing.T, cfg NASANodeConfig) {
				assert.Equal(t, "10s", cfg.Timeout)
			},
		},
		{
			name: "빈 device_id 무시",
			cfg:  NASANodeConfig{DeviceID: "keep-me"},
			msgPayload: map[string]any{
				"device_id": "",
			},
			checkCfg: func(t *testing.T, cfg NASANodeConfig) {
				assert.Equal(t, "keep-me", cfg.DeviceID, "빈 문자열은 무시해야 한다")
			},
		},
		{
			name:       "오버라이드 키 없음 -> 원본 유지",
			cfg:        NASANodeConfig{DeviceID: "original", Timeout: "5s"},
			msgPayload: map[string]any{"other": "value"},
			checkCfg: func(t *testing.T, cfg NASANodeConfig) {
				assert.Equal(t, "original", cfg.DeviceID)
				assert.Equal(t, "5s", cfg.Timeout)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := message.New(message.WithPayload(message.NewPayload(tt.msgPayload)))
			result := applyNASAOverrides(msg, tt.cfg)
			tt.checkCfg(t, result)
		})
	}
}

// ===========================================================================
// callAgentProcess 테스트
// ===========================================================================

// TestNASANodeBase_CallAgentProcess_AgentNil 은 agent가 nil일 때 ErrNASANoResolver를 반환하는지 확인한다.
func TestNASANodeBase_CallAgentProcess_AgentNil(t *testing.T) {
	n := newTestNASAStatusNode(nil)

	_, err := n.callAgentProcess(context.Background(), []byte("test"))
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNASANoResolver)
}

// TestNASANodeBase_CallAgentProcess_타임아웃 은 Agent Process() 호출이 타임아웃되는지 확인한다.
func TestNASANodeBase_CallAgentProcess_타임아웃(t *testing.T) {
	slowAgent := &slowNASAAgent{delay: 2 * time.Second}
	n := newTestNASAStatusNode(slowAgent)
	n.timeout = 100 * time.Millisecond

	_, err := n.callAgentProcess(context.Background(), []byte(`{"command":"get_all_states"}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "deadline exceeded")
}

// ===========================================================================
// 인터페이스 컴파일 타임 검증
// ===========================================================================

// NASAStatusNode이 Node 및 SourceNode 인터페이스를 구현하는지 검증
var (
	_ Node       = (*NASAStatusNode)(nil)
	_ SourceNode = (*NASAStatusNode)(nil)
)

// NASAControlNode이 Node 인터페이스를 구현하는지 검증
var _ Node = (*NASAControlNode)(nil)

// NASANode이 Node 및 SourceNode 인터페이스를 구현하는지 검증
var (
	_ Node       = (*NASANode)(nil)
	_ SourceNode = (*NASANode)(nil)
)

// ===========================================================================
// splitNASAPollResult 테스트
// ===========================================================================

// TestSplitNASAPollResult_MultiDevice 는 다중 디바이스 응답을 개별 메시지로 분리하는지 확인한다.
func TestSplitNASAPollResult_MultiDevice(t *testing.T) {
	result := map[string]any{
		"status": "ok",
		"devices": []any{
			map[string]any{
				"device_id": "living-room",
				"state": map[string]any{
					"Power":      true,
					"Mode":       "cool",
					"TargetTemp": 24.0,
				},
			},
			map[string]any{
				"device_id": "bedroom",
				"state": map[string]any{
					"Power": false,
					"Mode":  "heat",
				},
			},
		},
	}

	msgs := splitNASAPollResult(result, "test-node")
	assert.Len(t, msgs, 2)

	// 각 메시지에 device_id와 state가 있는지 확인
	for _, msg := range msgs {
		id, ok := msg.Payload().Get("device_id")
		assert.True(t, ok, "device_id 필드가 있어야 한다")
		assert.NotNil(t, id)

		stateVal, ok := msg.Payload().Get("state")
		assert.True(t, ok, "state 필드가 있어야 한다")
		assert.NotNil(t, stateVal)

		source, ok := msg.Metadata().Get("nasa_source")
		assert.True(t, ok)
		assert.Equal(t, "poll", source)

		nodeID, ok := msg.Metadata().Get("nasa_node_id")
		assert.True(t, ok)
		assert.Equal(t, "test-node", nodeID)
	}

	// 디바이스 ID 확인
	id0, _ := msgs[0].Payload().Get("device_id")
	id1, _ := msgs[1].Payload().Get("device_id")
	ids := []string{id0.(string), id1.(string)}
	assert.ElementsMatch(t, []string{"living-room", "bedroom"}, ids)
}

// TestSplitNASAPollResult_SingleDevice 는 단일 디바이스도 개별 메시지로 분리되는지 확인한다.
func TestSplitNASAPollResult_SingleDevice(t *testing.T) {
	result := map[string]any{
		"status": "ok",
		"devices": []any{
			map[string]any{
				"device_id": "living-room",
				"state":     map[string]any{"Power": true},
			},
		},
	}

	msgs := splitNASAPollResult(result, "test-node")
	assert.Len(t, msgs, 1)

	id, ok := msgs[0].Payload().Get("device_id")
	assert.True(t, ok)
	assert.Equal(t, "living-room", id)
}

// TestSplitNASAPollResult_EmptyDevices 는 빈 devices 배열 시 전체 응답이 단일 메시지로 반환되는지 확인한다.
func TestSplitNASAPollResult_EmptyDevices(t *testing.T) {
	result := map[string]any{
		"status":  "ok",
		"devices": []any{},
	}

	msgs := splitNASAPollResult(result, "test-node")
	assert.Len(t, msgs, 1)

	status, ok := msgs[0].Payload().Get("status")
	assert.True(t, ok)
	assert.Equal(t, "ok", status)
}

// TestSplitNASAPollResult_NoDevices 는 devices 키가 없을 때 전체 응답이 단일 메시지로 반환되는지 확인한다.
func TestSplitNASAPollResult_NoDevices(t *testing.T) {
	result := map[string]any{
		"status":    "ok",
		"device_id": "living-room",
		"state":     map[string]any{"Power": true},
	}

	msgs := splitNASAPollResult(result, "test-node")
	assert.Len(t, msgs, 1)

	id, ok := msgs[0].Payload().Get("device_id")
	assert.True(t, ok)
	assert.Equal(t, "living-room", id)
}

// ===========================================================================
// pollRecentBulk 콘텐츠 기반 중복 제거 테스트
// ===========================================================================
//
// 배경: 사용자 보고에 따르면 NASAStatusNode 가 동일 device_id 와 거의 동일한
// payload (last_seen 만 갱신) 를 가진 메시지를 초당 ~10건 폭주시키는 문제가 있다.
// 특히 device_id 가 빈 문자열인 NASA 컨트롤러 자체 프레임이 가장 큰 잡음원이다.
// 본 테스트 그룹은 pollRecentBulk 에 콘텐츠 기반 dedup 필터 두 가지를 추가하기
// 위한 명세 테스트이다.

// buildBulkResp 는 pollRecentBulk 가 기대하는 응답 형식을 생성하는 헬퍼이다.
// pollRecentBulk 는 {"count": N, "snapshots": [{"seq": S, "device": {...}}, ...]}
// 형식을 unmarshal 한다.
func buildBulkResp(t *testing.T, snapshots []map[string]any) []byte {
	t.Helper()
	raw := make([]map[string]any, 0, len(snapshots))
	for i, snap := range snapshots {
		seq, ok := snap["seq"]
		if !ok {
			seq = int64(i + 1)
		}
		dev, ok := snap["device"]
		if !ok {
			t.Fatalf("buildBulkResp: snapshot %d 에 device 필드가 없다", i)
		}
		raw = append(raw, map[string]any{
			"seq":    seq,
			"device": dev,
		})
	}
	resp := map[string]any{
		"count":     len(raw),
		"snapshots": raw,
	}
	b, err := json.Marshal(resp)
	require.NoError(t, err)
	return b
}

// drainBulkSourceCh 는 NASAStatusNode 의 sourceCh 에서 짧은 대기 동안
// 도착한 모든 메시지를 수집한다. trigger_test 의 drainSourceCh(Node, ...)
// 와는 시그니처가 다르므로 별도 헬퍼로 정의한다.
func drainBulkSourceCh(n *NASAStatusNode, wait time.Duration) []message.Message {
	deadline := time.After(wait)
	var msgs []message.Message
	for {
		select {
		case m := <-n.sourceCh:
			msgs = append(msgs, m)
		case <-deadline:
			return msgs
		}
	}
}

// TestNASAStatusNode_PollRecentBulk_DedupsIdenticalPayload 는 동일한 device_id 와
// 동일한 payload 가 연속 두 번 들어오면 한 번만 emit 되는지 확인한다.
func TestNASAStatusNode_PollRecentBulk_DedupsIdenticalPayload(t *testing.T) {
	devicePayload := map[string]any{
		"device_id":   "dev-01",
		"address":     "10.00.01",
		"device_type": "outdoor",
		"online":      true,
		"last_seen":   "2026-05-13T17:37:25+09:00",
	}

	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", PollCommand: nasaCmdGetRecentStates, BatchSize: 32}

	// 첫 번째 poll: seq=1 에 동일 device payload
	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(1), "device": devicePayload},
	})
	n.pollRecentBulk(n.nasaCfg)

	// 두 번째 poll: seq=2 에 동일 device payload (last_seen 까지 모두 동일)
	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(2), "device": devicePayload},
	})
	n.pollRecentBulk(n.nasaCfg)

	msgs := drainBulkSourceCh(n, 50*time.Millisecond)
	assert.Len(t, msgs, 1, "동일 payload 는 한 번만 emit 되어야 한다")
}

// TestNASAStatusNode_PollRecentBulk_EmitsChangedPayload 는 동일 device_id 라도
// payload 가 달라지면 두 번 모두 emit 되는지 확인한다.
func TestNASAStatusNode_PollRecentBulk_EmitsChangedPayload(t *testing.T) {
	first := map[string]any{
		"device_id":   "dev-01",
		"address":     "10.00.01",
		"device_type": "outdoor",
		"online":      true,
		"last_seen":   "2026-05-13T17:37:25+09:00",
	}
	second := map[string]any{
		"device_id":   "dev-01",
		"address":     "10.00.01",
		"device_type": "outdoor",
		"online":      false, // 상태 변경
		"last_seen":   "2026-05-13T17:37:26+09:00",
	}

	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", PollCommand: nasaCmdGetRecentStates, BatchSize: 32}

	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(1), "device": first},
	})
	n.pollRecentBulk(n.nasaCfg)

	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(2), "device": second},
	})
	n.pollRecentBulk(n.nasaCfg)

	msgs := drainBulkSourceCh(n, 50*time.Millisecond)
	require.Len(t, msgs, 2, "payload 변경 시 두 번 모두 emit 되어야 한다")

	online0, _ := msgs[0].Payload().Get("online")
	online1, _ := msgs[1].Payload().Get("online")
	assert.Equal(t, true, online0)
	assert.Equal(t, false, online1)
}

// TestNASAStatusNode_PollRecentBulk_EmptyDeviceID_DedupsByAddress 는 device_id 가
// 빈 문자열인 스냅샷 (NASA 컨트롤러 self-frame) 이 노드로 emit 되고, 동일 address
// 의 동일 payload 반복은 dedup 되는지 확인한다 (2026-05-13 hotfix: 이전 구현은
// 빈 device_id 를 무조건 skip 하여 모든 controller-self 메시지가 차단되는 회귀를
// 유발했음).
func TestNASAStatusNode_PollRecentBulk_EmptyDeviceID_DedupsByAddress(t *testing.T) {
	heartbeatPayload := map[string]any{
		"device_id":   "", // NASA 컨트롤러 자체 프레임 — device_id 가 비어있음
		"address":     "10.00.00",
		"device_type": "outdoor",
		"online":      true,
		"last_seen":   "2026-05-13T17:37:25+09:00",
	}

	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", PollCommand: nasaCmdGetRecentStates, BatchSize: 32}

	// 1차 폴링 — emit 되어야 한다
	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(1), "device": heartbeatPayload},
	})
	n.pollRecentBulk(n.nasaCfg)
	first := drainBulkSourceCh(n, 50*time.Millisecond)
	require.Len(t, first, 1, "빈 device_id 메시지도 1회는 emit 되어야 한다")

	// 2차 폴링 — 동일 address + 동일 payload → address 기반 dedup 으로 skip
	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(2), "device": heartbeatPayload},
	})
	n.pollRecentBulk(n.nasaCfg)
	second := drainBulkSourceCh(n, 50*time.Millisecond)
	assert.Empty(t, second, "동일 address+payload 반복은 dedup 되어야 한다")
}

// TestNASAStatusNode_PollRecentBulk_IgnoresLastSeenInDedup 는 last_seen 필드만
// 갱신된 동일 상태 스냅샷이 dedup 되는지 확인한다 (2026-05-14 hotfix).
func TestNASAStatusNode_PollRecentBulk_IgnoresLastSeenInDedup(t *testing.T) {
	first := map[string]any{
		"device_id":   "dev-01",
		"address":     "20.00.00",
		"device_type": "indoor",
		"online":      true,
		"last_seen":   "2026-05-14T00:29:18+09:00",
		"state":       map[string]any{"CurrentTemp": 23.4, "Mode": "cool"},
	}
	second := map[string]any{
		"device_id":   "dev-01",
		"address":     "20.00.00",
		"device_type": "indoor",
		"online":      true,
		"last_seen":   "2026-05-14T00:29:19+09:00", // ← 1초 후, 그 외 동일
		"state":       map[string]any{"CurrentTemp": 23.4, "Mode": "cool"},
	}

	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", PollCommand: nasaCmdGetRecentStates, BatchSize: 32}

	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(1), "device": first},
	})
	n.pollRecentBulk(n.nasaCfg)
	emits1 := drainBulkSourceCh(n, 50*time.Millisecond)
	require.Len(t, emits1, 1, "1차 emit 1건")

	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(2), "device": second},
	})
	n.pollRecentBulk(n.nasaCfg)
	emits2 := drainBulkSourceCh(n, 50*time.Millisecond)
	assert.Empty(t, emits2, "last_seen 만 갱신된 동일 상태는 dedup 되어야 한다")
}

// TestNASAStatusNode_PollRecentBulk_PerDeviceIsolation 는 두 디바이스의 dedup 이
// 서로 독립적으로 동작하는지 확인한다.
// 두 디바이스가 각각 한 번씩 emit 된 뒤 동일 payload 가 반복되어도
// 추가 emit 은 없어야 한다 (초기 emit 만 2건).
func TestNASAStatusNode_PollRecentBulk_PerDeviceIsolation(t *testing.T) {
	dev01 := map[string]any{
		"device_id":   "dev-01",
		"address":     "10.00.01",
		"device_type": "outdoor",
		"online":      true,
		"last_seen":   "2026-05-13T17:37:25+09:00",
	}
	dev02 := map[string]any{
		"device_id":   "dev-02",
		"address":     "10.00.02",
		"device_type": "indoor",
		"online":      true,
		"last_seen":   "2026-05-13T17:37:25+09:00",
	}

	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", PollCommand: nasaCmdGetRecentStates, BatchSize: 32}

	// 첫 poll: 두 디바이스 모두 초기 emit
	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(1), "device": dev01},
		{"seq": int64(2), "device": dev02},
	})
	n.pollRecentBulk(n.nasaCfg)

	// 두 번째 poll: 동일 payload 반복 → 양쪽 모두 dedup
	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(3), "device": dev01},
		{"seq": int64(4), "device": dev02},
	})
	n.pollRecentBulk(n.nasaCfg)

	msgs := drainBulkSourceCh(n, 50*time.Millisecond)
	assert.Len(t, msgs, 2, "각 디바이스별로 초기 1건씩만 emit 되어야 한다")

	// 두 emit 의 device_id 가 dev-01, dev-02 임을 확인
	seen := map[string]bool{}
	for _, m := range msgs {
		if v, ok := m.Payload().Get("device_id"); ok {
			if s, ok := v.(string); ok {
				seen[s] = true
			}
		}
	}
	assert.True(t, seen["dev-01"], "dev-01 emit 누락")
	assert.True(t, seen["dev-02"], "dev-02 emit 누락")
}

// TestNASAStatusNode_PollRecentBulk_PreservesMetadata 는 emit 된 메시지에
// 기존 메타데이터 필드 (nasa_source, nasa_node_id, nasa_seq) 가 유지되는지 확인한다.
// 다운스트림 테스트가 이 필드들을 assert 하므로 dedup 추가가 영향을 주면 안 된다.
func TestNASAStatusNode_PollRecentBulk_PreservesMetadata(t *testing.T) {
	dev := map[string]any{
		"device_id":   "dev-01",
		"address":     "10.00.01",
		"device_type": "outdoor",
		"online":      true,
	}

	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", PollCommand: nasaCmdGetRecentStates, BatchSize: 32}

	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(42), "device": dev},
	})
	n.pollRecentBulk(n.nasaCfg)

	msgs := drainBulkSourceCh(n, 50*time.Millisecond)
	require.Len(t, msgs, 1)

	source, ok := msgs[0].Metadata().Get("nasa_source")
	require.True(t, ok, "nasa_source 메타데이터 누락")
	assert.Equal(t, "poll_bulk", source)

	nodeID, ok := msgs[0].Metadata().Get("nasa_node_id")
	require.True(t, ok, "nasa_node_id 메타데이터 누락")
	assert.NotEmpty(t, nodeID)

	seq, ok := msgs[0].Metadata().Get("nasa_seq")
	require.True(t, ok, "nasa_seq 메타데이터 누락")
	assert.Equal(t, "42", seq)
}

// ===========================================================================
// metadata.message_type 통일 분류 표준 테스트 (2026-05-14 SPEC)
// ===========================================================================
//
// 모든 agent 노드는 emit 하는 메시지에 metadata.message_type 을 설정한다:
//   - "event":    poll / subscription / frame notify 등으로 자발적 emit
//   - "response": Process(req) 호출에 대한 응답으로 emit
//
// 본 그룹은 NASA 노드의 두 경로 (pollRecentBulk → event, Process → response) 를
// 검증한다. 기존 nasa_source 키와 함께 설정되며 (alongside, not replacement) 이를
// 확인하여 회귀를 방지한다.

// TestNASAStatusNode_PollRecentBulk_SetsMessageTypeDeviceStatePoll 는 pollRecentBulk 가
// emit 한 메시지가 metadata.message_type="device_state.poll" (trigger fallback) 와
// nasa_source="poll_bulk" 를 모두 가지는지 확인한다 (v0.8.0 계층형 분류).
func TestNASAStatusNode_PollRecentBulk_SetsMessageTypeDeviceStatePoll(t *testing.T) {
	dev := map[string]any{
		"device_id": "dev-evt",
		"address":   "10.00.99",
		"online":    true,
	}

	mockAgent := &mockNASAAgent{}
	n := newTestNASAStatusNode(mockAgent)
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", PollCommand: nasaCmdGetRecentStates, BatchSize: 32}

	mockAgent.processResp = buildBulkResp(t, []map[string]any{
		{"seq": int64(7), "device": dev},
	})
	n.pollRecentBulk(n.nasaCfg)

	msgs := drainBulkSourceCh(n, 50*time.Millisecond)
	require.Len(t, msgs, 1)

	mt, ok := msgs[0].Metadata().Get("message_type")
	require.True(t, ok, "message_type 메타데이터 누락 — agent 노드 통일 표준 위반")
	assert.Equal(t, "device_state.poll", mt, "poll_bulk emit (trigger 없음) 은 device_state.poll 분류여야 한다")

	// 기존 source 키도 그대로 유지되는지 확인 (alongside, not replacement)
	source, ok := msgs[0].Metadata().Get("nasa_source")
	require.True(t, ok)
	assert.Equal(t, "poll_bulk", source)
}

// TestNASAStatusNode_Process_SetsMessageTypeDeviceStateResponse 는 Process 응답이
// metadata.message_type="device_state.response" 와 nasa_source="request" 를 모두
// 가지는지 확인한다 (v0.8.0 계층형 분류).
func TestNASAStatusNode_Process_SetsMessageTypeDeviceStateResponse(t *testing.T) {
	respBytes, err := json.Marshal(map[string]any{"power": "on", "temperature": 22.5})
	require.NoError(t, err)

	mockAgent := &mockNASAAgent{processResp: respBytes}
	n := newTestNASAStatusNode(mockAgent)

	n.mu.Lock()
	n.nasaCfg = NASANodeConfig{AgentRef: "test-agent", DeviceID: "hvac-001"}
	n.mu.Unlock()

	msg := message.New()
	results, err := n.Process(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, results, 1)

	mt, ok := results[0].Metadata().Get("message_type")
	require.True(t, ok, "message_type 메타데이터 누락 — agent 노드 통일 표준 위반")
	assert.Equal(t, "device_state.response", mt, "Process 응답은 device_state.response 분류여야 한다")

	source, ok := results[0].Metadata().Get("nasa_source")
	require.True(t, ok)
	assert.Equal(t, "request", source)
}
