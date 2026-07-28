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
// NASA 테스트용 모의 객체 (v0.18.26 inactivity 모델 정렬)
// ---------------------------------------------------------------------------

type mockNASAAgent struct {
	processCount int
	processData  [][]byte
	processResp  []byte
	processErr   error
	notifyCh     chan struct{}
}

func newMockNASAAgent(resp []byte) *mockNASAAgent {
	return &mockNASAAgent{
		processResp: resp,
		notifyCh:    make(chan struct{}, 1),
	}
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
func (m *mockNASAAgent) Type() string                        { return "samsung_hvacr01" }
func (m *mockNASAAgent) Info() agent.AgentInfo               { return agent.AgentInfo{} }
func (m *mockNASAAgent) Stats() agent.StatsSnapshot          { return agent.StatsSnapshot{} }

func (m *mockNASAAgent) Process(data []byte) ([]byte, error) {
	m.processCount++
	cp := make([]byte, len(data))
	copy(cp, data)
	m.processData = append(m.processData, cp)
	if m.processErr != nil {
		return nil, m.processErr
	}
	return m.processResp, nil
}

func (m *mockNASAAgent) FrameNotifyCh() <-chan struct{} { return m.notifyCh }

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

// mockNASATransport 는 AgentTransport + AgentAccessor 구현체이다.
type mockNASATransport struct {
	agent agent.Agent
}

func (m *mockNASATransport) Send(_ context.Context, _ message.Message) error { return nil }
func (m *mockNASATransport) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (m *mockNASATransport) UnderlyingAgent() agent.Agent { return m.agent }

// mockNASATransportNoAccessor 는 AgentAccessor 미구현 transport 이다.
type mockNASATransportNoAccessor struct{}

func (m *mockNASATransportNoAccessor) Send(_ context.Context, _ message.Message) error { return nil }
func (m *mockNASATransportNoAccessor) Receive(ctx context.Context) (message.Message, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

// newSamsungHvacr01NodeDef 는 테스트용 NodeDef를 만든다.
func newSamsungHvacr01NodeDef(name, nodeType string) flow.NodeDef {
	return flow.NewNodeDef(name, nodeType)
}

// makeRunningSamsungStatusNode 는 mock agent 를 주입한 SamsungHvacr01StatusNode 를 만든다.
func makeRunningSamsungStatusNode(t *testing.T, ag agent.Agent, cfg map[string]any) *SamsungHvacr01StatusNode {
	t.Helper()
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	base := NewBaseNode(def)
	n := &SamsungHvacr01StatusNode{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{BaseNode: base, timeout: 5 * time.Second, agent: ag},
		sourceCh:               make(chan message.Message, 64),
		stopCh:                 make(chan struct{}),
	}
	if cfg == nil {
		cfg = map[string]any{"agent_ref": "sm-1"}
	}
	require.NoError(t, n.Configure(cfg))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// ===========================================================================
// Configure / 타입 검증
// ===========================================================================

func TestSamsungHvacr01StatusNode_Type(t *testing.T) {
	t.Parallel()
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	node, err := NewSamsungHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*SamsungHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "sm-1"}))
	assert.Equal(t, "samsung_hvacr01_status", n.Type())
}

func TestSamsungHvacr01StatusNode_Configure_AgentRefRequired(t *testing.T) {
	t.Parallel()
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	node, err := NewSamsungHvacr01StatusNode(def)
	require.NoError(t, err)
	err = node.(*SamsungHvacr01StatusNode).Configure(map[string]any{})
	assert.ErrorIs(t, err, ErrNASAMissingAgentRef)
}

func TestSamsungHvacr01StatusNode_Configure_Defaults(t *testing.T) {
	t.Parallel()
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	node, err := NewSamsungHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*SamsungHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "sm-1"}))
	assert.Equal(t, "sm-1", n.hvacr01Cfg.AgentRef)
	assert.Equal(t, "90s", n.hvacr01Cfg.InactivityTimeout)
	assert.Equal(t, "5s", n.hvacr01Cfg.Timeout)
	assert.Equal(t, 32, n.hvacr01Cfg.BatchSize)
	assert.Equal(t, 5*time.Second, n.timeout)
	assert.Equal(t, 90*time.Second, n.inactivityTimeout)
	assert.Empty(t, n.hvacr01Cfg.GroupID)
	assert.Empty(t, n.hvacr01Cfg.UnitID)
}

func TestSamsungHvacr01StatusNode_Configure_Addressing(t *testing.T) {
	t.Parallel()
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	node, err := NewSamsungHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*SamsungHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":          "sm-2",
		"inactivity_timeout": "30s",
		"timeout":            "2s",
		"group_id":           "10",
		"unit_id":            "0F",
		"batch_size":         float64(16),
	}))
	assert.Equal(t, "10", n.hvacr01Cfg.GroupID)
	assert.Equal(t, "0F", n.hvacr01Cfg.UnitID)
	assert.Equal(t, 16, n.hvacr01Cfg.BatchSize)
	assert.Equal(t, 2*time.Second, n.timeout)
	assert.Equal(t, 30*time.Second, n.inactivityTimeout)
}

func TestSamsungHvacr01StatusNode_Configure_MinInactivityClamp(t *testing.T) {
	t.Parallel()
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	node, err := NewSamsungHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*SamsungHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":          "sm-1",
		"inactivity_timeout": "1s",
	}))
	assert.Equal(t, nasaMinInactivityTimeout, n.inactivityTimeout)
}

// ===========================================================================
// Init 시나리오
// ===========================================================================

func TestSamsungHvacr01StatusNode_Init_NoResolver(t *testing.T) {
	t.Parallel()
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	node, err := NewSamsungHvacr01StatusNode(def)
	require.NoError(t, err)
	n := node.(*SamsungHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "sm-1"}))
	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrNASANoResolver)
}

func TestSamsungHvacr01StatusNode_Init_DeferredOnUnresolvedAgent(t *testing.T) {
	t.Parallel()
	resolver := &mockNASAResolver{err: assert.AnError}
	def := newSamsungHvacr01NodeDef("sm-status", "samsung_hvacr01_status")
	node, err := NewSamsungHvacr01StatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	n := node.(*SamsungHvacr01StatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "missing"}))
	// 에이전트 resolve 실패 시 deferred connection — Running 으로 진행.
	require.NoError(t, n.Init(context.Background()))
	_ = n.Shutdown(context.Background())
}

// ===========================================================================
// Process (단발 호출 — get_state / get_all)
// ===========================================================================

func TestSamsungHvacr01StatusNode_Process_GetAll(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{
		"devices": []any{},
	})
	mock := newMockNASAAgent(resp)
	n := makeRunningSamsungStatusNode(t, mock, nil)

	in := message.New()
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "device_state.response", out[0].Type())

	// device_id 미지정 → get_all 사용.
	require.GreaterOrEqual(t, mock.processCount, 1)
	var got map[string]any
	require.NoError(t, json.Unmarshal(mock.processData[len(mock.processData)-1], &got))
	assert.Equal(t, nasaCmdGetAllState, got["command"])
}

func TestSamsungHvacr01StatusNode_Process_GetStateWithPayloadDeviceID(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{"status": "ok"})
	mock := newMockNASAAgent(resp)
	n := makeRunningSamsungStatusNode(t, mock, nil)

	in := message.New()
	in.Payload().Set("device_id", "living-room")
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	var got map[string]any
	require.NoError(t, json.Unmarshal(mock.processData[len(mock.processData)-1], &got))
	assert.Equal(t, nasaCmdGetState, got["command"])
	assert.Equal(t, "living-room", got["device_id"])
}

// ===========================================================================
// drainNewFrames / 어드레싱 필터
// ===========================================================================

func TestSamsungHvacr01StatusNode_DrainNewFrames_Emits(t *testing.T) {
	t.Parallel()
	// 주의: map[string]any 에 []byte 를 넣으면 json.Marshal 시 base64 인코딩되므로
	// json.RawMessage 를 사용해야 한다.
	device := map[string]any{
		"address":      "10.0F.00",
		"trigger":      "change",
		"state":        map[string]any{"power": true},
		"last_seen_ms": int64(1716800000000),
	}
	devJSON, _ := json.Marshal(device)
	snap, _ := json.Marshal(map[string]any{"seq": 1, "device": json.RawMessage(devJSON)})
	resp, _ := json.Marshal(map[string]any{
		"count":     1,
		"snapshots": []json.RawMessage{snap},
	})
	mock := newMockNASAAgent(resp)
	n := makeRunningSamsungStatusNode(t, mock, nil)

	n.mu.RLock()
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	select {
	case msg := <-n.sourceCh:
		assert.Equal(t, "device_state.change", msg.Type())
	case <-time.After(time.Second):
		t.Fatal("expected message on sourceCh")
	}
}

// TestSamsungHvacr01StatusNode_DrainNewFrames_EmitsConnectionFields 는 연결 정보가
// device_state 로 일원화된 뒤(별도 device_connection 스트림 제거), 연결 진단 필드
// (online/error_count/offline_threshold/transport_connected)를 state 그룹에 담은 스냅샷이
// device_state.<trigger> 메시지로 방출되고 해당 필드가 payload 로 평탄화되는지 검증한다.
func TestSamsungHvacr01StatusNode_DrainNewFrames_EmitsConnectionFields(t *testing.T) {
	t.Parallel()
	device := map[string]any{
		"address":   "20.00.00",
		"unit_id":   "20.00.00",
		"device_id": "20.00.00",
		"trigger":   "report",
		"state": map[string]any{
			"online":              false,
			"error_count":         float64(2),
			"offline_threshold":   float64(3),
			"transport_connected": true,
		},
		"last_seen_ms": int64(1716800000000),
	}
	devJSON, _ := json.Marshal(device)
	snap, _ := json.Marshal(map[string]any{"seq": 1, "device": json.RawMessage(devJSON)})
	resp, _ := json.Marshal(map[string]any{
		"count":     1,
		"snapshots": []json.RawMessage{snap},
	})
	mock := newMockNASAAgent(resp)
	n := makeRunningSamsungStatusNode(t, mock, nil)

	n.mu.RLock()
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

	select {
	case msg := <-n.sourceCh:
		assert.Equal(t, "device_state.report", msg.Type(),
			"연결 정보 스냅샷도 device_state 로 방출되어야 함")
		online, _ := msg.Payload().Get("online")
		assert.Equal(t, false, online, "online 필드가 평탄화되어야 함")
		ec, _ := msg.Payload().Get("error_count")
		assert.Equal(t, float64(2), ec, "error_count 필드 보존")
		ot, _ := msg.Payload().Get("offline_threshold")
		assert.Equal(t, float64(3), ot, "offline_threshold 필드 보존")
		tc, _ := msg.Payload().Get("transport_connected")
		assert.Equal(t, true, tc, "transport_connected 필드 보존")
	case <-time.After(time.Second):
		t.Fatal("expected device_state message on sourceCh")
	}
}

func TestSamsungHvacr01StatusNode_DrainNewFrames_AddressingFilter(t *testing.T) {
	t.Parallel()
	// 두 디바이스: address 10.0F.00 (매칭) / 10.10.00 (제외).
	mk := func(addr string, seq int) json.RawMessage {
		dev, _ := json.Marshal(map[string]any{
			"address": addr,
			"trigger": "change",
			"state":   map[string]any{"power": true},
		})
		b, _ := json.Marshal(map[string]any{"seq": seq, "device": json.RawMessage(dev)})
		return b
	}
	resp, _ := json.Marshal(map[string]any{
		"count":     2,
		"snapshots": []json.RawMessage{mk("10.0F.00", 1), mk("10.10.00", 2)},
	})
	mock := newMockNASAAgent(resp)
	n := makeRunningSamsungStatusNode(t, mock, map[string]any{
		"agent_ref": "sm-1",
		"group_id":  "10",
		"unit_id":   "0F",
	})

	n.mu.RLock()
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()
	n.drainNewFrames(cfg)

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
	assert.Equal(t, 1, got, "group_id/unit_id 매칭되는 1개만 통과해야 한다")
}

// ===========================================================================
// request_state 발송
// ===========================================================================

func TestSamsungHvacr01StatusNode_RequestStateRefresh_SendsCommand(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{"status": "ok"})
	mock := newMockNASAAgent(resp)
	n := makeRunningSamsungStatusNode(t, mock, map[string]any{
		"agent_ref": "sm-1",
		"group_id":  "10",
		"unit_id":   "0F",
	})

	n.mu.RLock()
	cfg := n.hvacr01Cfg
	n.mu.RUnlock()
	n.requestStateRefresh(cfg)

	require.GreaterOrEqual(t, mock.processCount, 1)
	last := mock.processData[len(mock.processData)-1]
	var got map[string]any
	require.NoError(t, json.Unmarshal(last, &got))
	assert.Equal(t, "request_state", got["command"])
	assert.Equal(t, "10", got["group_id"])
	assert.Equal(t, "0F", got["unit_id"])
}

// ===========================================================================
// 어드레싱 매칭 단위 테스트
// ===========================================================================

func TestSamsungHvacr01MatchAddressing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		payload map[string]any
		cfg     SamsungHvacr01NodeConfig
		want    bool
	}{
		{
			name:    "cfg 비어있으면 매칭",
			payload: map[string]any{"address": "10.0F.00"},
			cfg:     SamsungHvacr01NodeConfig{},
			want:    true,
		},
		{
			name:    "group+unit 매칭 — 점 형식",
			payload: map[string]any{"address": "10.0F.00"},
			cfg:     SamsungHvacr01NodeConfig{GroupID: "10", UnitID: "0F"},
			want:    true,
		},
		{
			name:    "group+unit 매칭 — 연속 형식",
			payload: map[string]any{"address": "100F00"},
			cfg:     SamsungHvacr01NodeConfig{GroupID: "10", UnitID: "0F"},
			want:    true,
		},
		{
			name:    "unit 만 매칭 — group 미지정",
			payload: map[string]any{"address": "10.0F.00"},
			cfg:     SamsungHvacr01NodeConfig{UnitID: "0F"},
			want:    true,
		},
		{
			name:    "group 다름 거부",
			payload: map[string]any{"address": "20.0F.00"},
			cfg:     SamsungHvacr01NodeConfig{GroupID: "10", UnitID: "0F"},
			want:    false,
		},
		{
			name:    "unit 다름 거부",
			payload: map[string]any{"address": "10.10.00"},
			cfg:     SamsungHvacr01NodeConfig{GroupID: "10", UnitID: "0F"},
			want:    false,
		},
		{
			name:    "address 누락 거부",
			payload: map[string]any{"other": "x"},
			cfg:     SamsungHvacr01NodeConfig{UnitID: "0F"},
			want:    false,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, samsungHvacr01MatchAddressing(tc.payload, tc.cfg))
		})
	}
}

// ===========================================================================
// Control 노드 — 기본 동작 검증
// ===========================================================================

func TestSamsungHvacr01ControlNode_Process_SetMultiple(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{"status": "ok"})
	mock := newMockNASAAgent(resp)
	def := newSamsungHvacr01NodeDef("sm-ctl", "samsung_hvacr01_control")
	base := NewBaseNode(def)
	n := &SamsungHvacr01ControlNode{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{BaseNode: base, timeout: 5 * time.Second, agent: mock},
	}
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "sm-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	in := message.New()
	in.Payload().Set("device_id", "living-room")
	in.Payload().Set("power", true)
	in.Payload().Set("mode", "cool")

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	require.GreaterOrEqual(t, mock.processCount, 1)
	last := mock.processData[len(mock.processData)-1]
	var got map[string]any
	require.NoError(t, json.Unmarshal(last, &got))
	assert.Equal(t, nasaCmdSetMultiple, got["command"])
	assert.Equal(t, "living-room", got["device_id"])
}

func TestSamsungHvacr01ControlNode_Process_DirectCommand(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{"status": "ok"})
	mock := newMockNASAAgent(resp)
	def := newSamsungHvacr01NodeDef("sm-ctl", "samsung_hvacr01_control")
	base := NewBaseNode(def)
	n := &SamsungHvacr01ControlNode{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{BaseNode: base, timeout: 5 * time.Second, agent: mock},
	}
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "sm-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	in := message.New()
	in.Payload().Set("device_id", "lr")
	in.Payload().Set("command", nasaCmdSetPower)
	in.Payload().Set("params", map[string]any{"power": true})

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	last := mock.processData[len(mock.processData)-1]
	var got map[string]any
	require.NoError(t, json.Unmarshal(last, &got))
	assert.Equal(t, nasaCmdSetPower, got["command"])
}

// ===========================================================================
// Combined node — has control keys → control path
// ===========================================================================

func TestSamsungHvacr01Node_Process_DetectsControl(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{"status": "ok"})
	mock := newMockNASAAgent(resp)
	def := newSamsungHvacr01NodeDef("sm-combined", "samsung_hvacr01")
	base := NewBaseNode(def)
	n := &SamsungHvacr01Node{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{BaseNode: base, timeout: 5 * time.Second, agent: mock},
		sourceCh:               make(chan message.Message, 8),
		stopCh:                 make(chan struct{}),
	}
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "sm-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	in := message.New()
	in.Payload().Set("device_id", "lr")
	in.Payload().Set("power", true)
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	cmdType, _ := out[0].Metadata().Get("nasa_command")
	assert.Equal(t, "control", cmdType)
}

func TestSamsungHvacr01Node_Process_DetectsStatus(t *testing.T) {
	t.Parallel()
	resp, _ := json.Marshal(map[string]any{"status": "ok"})
	mock := newMockNASAAgent(resp)
	def := newSamsungHvacr01NodeDef("sm-combined", "samsung_hvacr01")
	base := NewBaseNode(def)
	n := &SamsungHvacr01Node{
		samsungHvacr01NodeBase: samsungHvacr01NodeBase{BaseNode: base, timeout: 5 * time.Second, agent: mock},
		sourceCh:               make(chan message.Message, 8),
		stopCh:                 make(chan struct{}),
	}
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "sm-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	in := message.New()
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	cmdType, _ := out[0].Metadata().Get("nasa_command")
	assert.Equal(t, "status", cmdType)
}

// ===========================================================================
// 어드레싱 hex 파서 단위 테스트
// ===========================================================================

func TestSamsungParseAddressBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		grp   byte
		unt   byte
		ok    bool
	}{
		{input: "10.0F.00", grp: 0x10, unt: 0x0F, ok: true},
		{input: "FF.AB.00", grp: 0xFF, unt: 0xAB, ok: true},
		{input: "100F00", grp: 0x10, unt: 0x0F, ok: true},
		{input: "ABCD", grp: 0, unt: 0, ok: false},     // 4 chars 미지원
		{input: "GG.00.00", grp: 0, unt: 0, ok: false}, // 잘못된 hex
		{input: "", grp: 0, unt: 0, ok: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			g, u, ok := samsungParseAddressBytes(tc.input)
			assert.Equal(t, tc.ok, ok)
			if ok {
				assert.Equal(t, tc.grp, g)
				assert.Equal(t, tc.unt, u)
			}
		})
	}
}
