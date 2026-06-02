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
// Century 노드 테스트용 모의 객체 (v0.18.26 inactivity 모델 정렬)
// ---------------------------------------------------------------------------

// mockCenturyAgent 는 Century 노드 테스트용 agent.Agent 구현이다.
type mockCenturyAgent struct {
	processCount int
	processData  [][]byte
	processResp  []byte
	processErr   error
	notifyCh     chan struct{}
}

func newMockCenturyAgent(resp []byte) *mockCenturyAgent {
	return &mockCenturyAgent{
		processResp: resp,
		notifyCh:    make(chan struct{}, 1),
	}
}

func (m *mockCenturyAgent) Init(_ agent.AgentConfig) error      { return nil }
func (m *mockCenturyAgent) Start(_ context.Context) error       { return nil }
func (m *mockCenturyAgent) Stop(_ context.Context) error        { return nil }
func (m *mockCenturyAgent) Pause(_ context.Context) error       { return nil }
func (m *mockCenturyAgent) Resume(_ context.Context) error      { return nil }
func (m *mockCenturyAgent) Health() agent.HealthStatus          { return agent.HealthStatus{} }
func (m *mockCenturyAgent) Configure(_ agent.AgentConfig) error { return nil }
func (m *mockCenturyAgent) ID() string                          { return "mock-century" }
func (m *mockCenturyAgent) Name() string                        { return "mock-century" }
func (m *mockCenturyAgent) Type() string                        { return "century_hvacr01" }
func (m *mockCenturyAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockCenturyAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockCenturyAgent) Process(data []byte) ([]byte, error) {
	m.processCount++
	cp := make([]byte, len(data))
	copy(cp, data)
	m.processData = append(m.processData, cp)
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.processResp, nil
}

func (m *mockCenturyAgent) FrameNotifyCh() <-chan struct{} { return m.notifyCh }

// otherProtoAgent 는 century 타입이 아닌 에이전트를 시뮬레이트한다.
type otherProtoAgent struct{ mockCenturyAgent }

func (o *otherProtoAgent) Type() string { return "lg_hvacr01" }

// centuryMockResolver 는 테스트용 AgentResolver 이다.
type centuryMockResolver struct {
	transport AgentTransport
	err       error
}

func (r *centuryMockResolver) ResolveAgent(_ context.Context, _ flow.AgentRef) (AgentTransport, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.transport, nil
}

// centuryMockTransport 는 AgentTransport + AgentAccessor 구현체이다.
type centuryMockTransport struct {
	agent agent.Agent
}

func (m *centuryMockTransport) Send(_ context.Context, _ message.Message) error { return nil }
func (m *centuryMockTransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (m *centuryMockTransport) UnderlyingAgent() agent.Agent { return m.agent }

// centuryMockTransportNoAccessor 는 AgentAccessor 미구현 transport 이다.
type centuryMockTransportNoAccessor struct{}

func (m *centuryMockTransportNoAccessor) Send(_ context.Context, _ message.Message) error { return nil }
func (m *centuryMockTransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// newCenturyNodeDef 는 테스트용 NodeDef 를 만든다.
func newCenturyNodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// makeRunningStatusNode 는 mock agent 를 주입한 CenturyHvacr01StatusNode 를 만든다 (테스트 헬퍼).
func makeRunningStatusNode(t *testing.T, ag agent.Agent, cfg map[string]any) *CenturyHvacr01StatusNode {
	t.Helper()
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	base := NewBaseNode(def)
	n := &CenturyHvacr01StatusNode{
		centuryHvacr01NodeBase: centuryHvacr01NodeBase{BaseNode: base, timeout: 5 * time.Second, agent: ag},
		sourceCh:               make(chan message.Message, 64),
		stopCh:                 make(chan struct{}),
	}
	if cfg == nil {
		cfg = map[string]any{"agent_ref": "ct-1"}
	}
	require.NoError(t, n.Configure(cfg))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// ===========================================================================
// Configure / 타입 검증
// ===========================================================================

func TestCenturyHvacr01StatusNode_Type(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	assert.Equal(t, "century_hvacr01_status", n.Type())
}

func TestCenturyHvacr01StatusNode_Configure_AgentRefRequired(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def)
	require.NoError(t, err)
	err = node.(*CenturyHvacr01StatusNode).Configure(map[string]any{})
	assert.ErrorIs(t, err, ErrCenturyHvacr01MissingAgentRef)
}

// v0.18.26: 새 config 모델 (inactivity_timeout / group_id / unit_id 추가, polling 필드 제거).
func TestCenturyHvacr01StatusNode_Configure_Defaults(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	assert.Equal(t, "ct-1", n.centuryCfg.AgentRef)
	assert.Equal(t, "90s", n.centuryCfg.InactivityTimeout)
	assert.Equal(t, "5s", n.centuryCfg.Timeout)
	assert.Equal(t, 32, n.centuryCfg.BatchSize)
	assert.Equal(t, 5*time.Second, n.timeout)
	assert.Equal(t, 90*time.Second, n.inactivityTimeout)
	assert.Empty(t, n.centuryCfg.GroupID)
	assert.Empty(t, n.centuryCfg.UnitID)
	assert.False(t, n.centuryCfg.EmitRawFrames)
}

func TestCenturyHvacr01StatusNode_Configure_CustomAddressing(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":          "ct-2",
		"inactivity_timeout": "30s",
		"timeout":            "2s",
		"batch_size":         float64(16),
		"unit_id":            "3B",
		"emit_raw_frames":    true,
	}))
	assert.Equal(t, 16, n.centuryCfg.BatchSize)
	assert.Equal(t, "3B", n.centuryCfg.UnitID)
	assert.Equal(t, 2*time.Second, n.timeout)
	assert.Equal(t, 30*time.Second, n.inactivityTimeout)
	assert.True(t, n.centuryCfg.EmitRawFrames)
}

func TestCenturyHvacr01StatusNode_Configure_MinInactivityClamp(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":          "ct-1",
		"inactivity_timeout": "1s",
	}))
	// 1s 는 최소 5s 로 clamp.
	assert.Equal(t, centuryHvacr01MinInactivityTimeout, n.inactivityTimeout)
}

// ===========================================================================
// Init / Reinit
// ===========================================================================

func TestCenturyHvacr01StatusNode_Init_NoResolver(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyHvacr01NoResolver)
}

func TestCenturyHvacr01StatusNode_Init_DeferredOnUnresolvedAgent(t *testing.T) {
	t.Parallel()
	resolver := &centuryMockResolver{err: assert.AnError}
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "missing"}))
	require.NoError(t, n.Init(context.Background()))
	_ = n.Shutdown(context.Background())
	assert.Nil(t, n.agent, "agent should remain nil for deferred connection")
}

func TestCenturyHvacr01StatusNode_Init_NotCenturyAgent(t *testing.T) {
	t.Parallel()
	notCentury := &otherProtoAgent{}
	transport := &centuryMockTransport{agent: notCentury}
	resolver := &centuryMockResolver{transport: transport}

	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyHvacr01AgentNotCentury)
}

func TestCenturyHvacr01StatusNode_Init_NoAccessor(t *testing.T) {
	t.Parallel()
	transport := &centuryMockTransportNoAccessor{}
	resolver := &centuryMockResolver{transport: transport}
	def := newCenturyNodeDef("ct-status", "century_hvacr01_status")
	node, err := NewCenturyHvacr01StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	n := node.(*CenturyHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyHvacr01AgentNotCentury)
}

// ===========================================================================
// Process (단발 호출)
// ===========================================================================

func TestCenturyHvacr01StatusNode_Process_StatsResponse(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{
		"frames_captured": 10,
	})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, nil)

	in := message.New()
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "device_state.response", out[0].Type())
}

// ===========================================================================
// drainDeviceStateEvents (inactivity 모델 핵심 경로)
// ===========================================================================

func TestCenturyHvacr01StatusNode_DrainDeviceStateEvents_Emits(t *testing.T) {
	t.Parallel()
	// drain_device_state 응답을 시뮬레이트.
	deviceState := map[string]any{
		"type":         "device_state",
		"unit_id":      "0x3B",
		"trigger":      "change",
		"last_seen_ms": int64(1716800000000),
		"state": map[string]any{
			"power": true,
		},
	}
	devJSON, _ := json.Marshal(deviceState)
	resp, _ := json.Marshal(map[string]any{
		"count":  1,
		"events": []json.RawMessage{devJSON},
	})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, nil)

	// 직접 호출 (receiveLoop 의존성 없이 검증).
	n.centuryHvacr01NodeBase.drainDeviceStateEvents(n.ID(), n.sourceCh)

	select {
	case msg := <-n.sourceCh:
		// trigger=change → message_type=device_state.change
		assert.Equal(t, "device_state.change", msg.Type())
	case <-time.After(time.Second):
		t.Fatal("expected message on sourceCh")
	}
}

func TestCenturyHvacr01StatusNode_DrainDeviceStateEvents_UnitIDFilter(t *testing.T) {
	t.Parallel()
	// unit_id="0x3B" 인 이벤트와 "0x40" 인 이벤트.
	mk := func(unitID string) json.RawMessage {
		b, _ := json.Marshal(map[string]any{
			"type":    "device_state",
			"unit_id": unitID,
			"trigger": "change",
			"state":   map[string]any{"power": true},
		})
		return b
	}
	resp, _ := json.Marshal(map[string]any{
		"count":  2,
		"events": []json.RawMessage{mk("0x3B"), mk("0x40")},
	})
	mock := newMockCenturyAgent(resp)
	// cfg.UnitID="3B" 필터 적용.
	n := makeRunningStatusNode(t, mock, map[string]any{
		"agent_ref": "ct-1",
		"unit_id":   "3B",
	})

	n.centuryHvacr01NodeBase.drainDeviceStateEvents(n.ID(), n.sourceCh)

	// 매칭되는 메시지 하나만 통과해야 한다.
	got := 0
loop:
	for {
		select {
		case <-n.sourceCh:
			got++
		case <-time.After(200 * time.Millisecond):
			break loop
		}
	}
	assert.Equal(t, 1, got, "unit_id 필터로 1개만 통과해야 한다")
}

// ===========================================================================
// emit_raw_frames=true 경로 (기존 raw-frame 통합 동작 보존)
// ===========================================================================

func TestCenturyHvacr01StatusNode_EmitRawFrames_BypassesInactivity(t *testing.T) {
	t.Parallel()
	// get_recent 응답: 한 frame.
	resp, _ := json.Marshal(map[string]any{
		"count": 1,
		"frames": []map[string]any{{
			"seq":           1,
			"timestamp_ms":  1716800000000,
			"raw_hex":       "DEAD",
			"function_code": 0x10,
		}},
	})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, map[string]any{
		"agent_ref":       "ct-1",
		"emit_raw_frames": true,
	})

	// drainNewFrames → drainRawFrames 호경로 시뮬레이트.
	n.mu.RLock()
	cfg := n.centuryCfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	select {
	case msg := <-n.sourceCh:
		assert.Equal(t, "raw_frame.event", msg.Type())
		v, _ := msg.Payload().Get("raw_hex")
		assert.Equal(t, "DEAD", v)
	case <-time.After(time.Second):
		t.Fatal("expected raw frame message")
	}
}

// ===========================================================================
// request_state 발송 (inactivity-fallback 호경로)
// ===========================================================================

func TestCenturyHvacr01StatusNode_RequestStateRefresh_SendsCommand(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{"status": "ok"})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, map[string]any{
		"agent_ref": "ct-1",
		"unit_id":   "3B",
	})

	n.mu.RLock()
	cfg := n.centuryCfg
	n.mu.RUnlock()
	n.requestStateRefresh(cfg)

	require.GreaterOrEqual(t, mock.processCount, 1)
	last := mock.processData[len(mock.processData)-1]
	var got map[string]any
	require.NoError(t, json.Unmarshal(last, &got))
	assert.Equal(t, "request_state", got["command"])
	assert.Equal(t, "3B", got["unit_id"])
}

// ===========================================================================
// Control 노드 - not_supported
// ===========================================================================

func TestCenturyHvacr01ControlNode_AlwaysNotSupported(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-ctl", "century_hvacr01_control")
	node, err := NewCenturyHvacr01ControlNode(def)
	require.NoError(t, err)
	n := node.(*CenturyHvacr01ControlNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))

	in := message.New()
	in.Payload().Set("power", true)
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)
	status, _ := out[0].Payload().Get("status")
	assert.Equal(t, "not_supported", status)
}

// ===========================================================================
// 어드레싱 헬퍼 단위 테스트
// ===========================================================================

func TestCenturyHvacr01MatchAddressing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		payload map[string]any
		cfg     CenturyHvacr01NodeConfig
		want    bool
	}{
		{
			name:    "cfg 비어있으면 매칭",
			payload: map[string]any{"unit_id": "0x3B"},
			cfg:     CenturyHvacr01NodeConfig{},
			want:    true,
		},
		{
			name:    "동일 hex byte 매칭",
			payload: map[string]any{"unit_id": "0x3B"},
			cfg:     CenturyHvacr01NodeConfig{UnitID: "3B"},
			want:    true,
		},
		{
			name:    "0x 접두 무관",
			payload: map[string]any{"unit_id": "3B"},
			cfg:     CenturyHvacr01NodeConfig{UnitID: "0x3B"},
			want:    true,
		},
		{
			name:    "다른 unit_id 거부",
			payload: map[string]any{"unit_id": "0x40"},
			cfg:     CenturyHvacr01NodeConfig{UnitID: "3B"},
			want:    false,
		},
		{
			name:    "payload 에 unit_id 없으면 거부",
			payload: map[string]any{"other": "x"},
			cfg:     CenturyHvacr01NodeConfig{UnitID: "3B"},
			want:    false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, centuryHvacr01MatchAddressing(tc.payload, tc.cfg))
		})
	}
}
