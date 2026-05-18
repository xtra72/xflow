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
// Century 노드 테스트용 모의 객체
// ---------------------------------------------------------------------------

// mockCenturyAgent 는 Century 노드 테스트용 agent.Agent 구현이다.
//
// Process() 호출 시 호출 횟수와 마지막 전달 데이터를 기록하고 미리 지정된 응답을 반환한다.
// FrameNotifier 도 구현하여 즉시 폴링 동작을 검증할 수 있다.
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
func (m *mockCenturyAgent) Type() string                        { return "century-hvac" }
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

// otherProtoAgent 는 century 타입이 아닌 에이전트를 시뮬레이트한다 (AC-C6 / ErrCenturyAgentNotCentury 검증).
type otherProtoAgent struct{ mockCenturyAgent }

func (o *otherProtoAgent) Type() string { return "lgcnp" }

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

// makeRunningStatusNode 는 mock agent 를 주입한 CenturyStatusNode 를 만든다 (테스트 헬퍼).
func makeRunningStatusNode(t *testing.T, ag agent.Agent, cfg map[string]any) *CenturyStatusNode {
	t.Helper()
	def := newCenturyNodeDef("ct-status", "century-status")
	base := NewBaseNode(def)
	n := &CenturyStatusNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: 5 * time.Second, agent: ag},
		sourceCh:        make(chan message.Message, 64),
		stopCh:          make(chan struct{}),
	}
	if cfg == nil {
		cfg = map[string]any{"agent_ref": "ct-1"}
	}
	// 공개 Configure 를 사용하여 pollInterval 까지 설정.
	require.NoError(t, n.Configure(cfg))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	return n
}

// ===========================================================================
// 팩토리 / Configure / Init 테스트
// ===========================================================================

func TestNewCenturyStatusNode_Default(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century-status")
	n, err := NewCenturyStatusNode(def)
	require.NoError(t, err)
	assert.Equal(t, "ct-status", n.Name())
	assert.Equal(t, "century-status", n.Type())
}

func TestCenturyStatusNode_Configure_AgentRefRequired(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century-status")
	node, err := NewCenturyStatusNode(def)
	require.NoError(t, err)
	err = node.(*CenturyStatusNode).Configure(map[string]any{})
	assert.ErrorIs(t, err, ErrCenturyMissingAgentRef)
}

func TestCenturyStatusNode_Configure_Defaults(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century-status")
	node, err := NewCenturyStatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyStatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	assert.Equal(t, "ct-1", n.centuryCfg.AgentRef)
	assert.Equal(t, "100ms", n.centuryCfg.PollInterval)
	assert.Equal(t, "5s", n.centuryCfg.Timeout)
	assert.Equal(t, "drain", n.centuryCfg.PollCommand)
	assert.Equal(t, 10, n.centuryCfg.RecentCount)
	assert.Equal(t, 32, n.centuryCfg.BatchSize)
	assert.Equal(t, 5*time.Second, n.timeout)
	assert.Equal(t, 100*time.Millisecond, n.pollInterval)
}

func TestCenturyStatusNode_Configure_CustomValues(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century-status")
	node, err := NewCenturyStatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyStatusNode)
	require.NoError(t, n.Configure(map[string]any{
		"agent_ref":     "ct-2",
		"poll_interval": "200ms",
		"timeout":       "2s",
		"poll_command":  "get_recent",
		"recent_count":  float64(5),
		"batch_size":    float64(16),
	}))
	assert.Equal(t, 5, n.centuryCfg.RecentCount)
	assert.Equal(t, 16, n.centuryCfg.BatchSize)
	assert.Equal(t, "get_recent", n.centuryCfg.PollCommand)
	assert.Equal(t, 2*time.Second, n.timeout)
	assert.Equal(t, 200*time.Millisecond, n.pollInterval)
}

func TestCenturyStatusNode_Init_NoResolver(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century-status")
	node, err := NewCenturyStatusNode(def)
	require.NoError(t, err)
	n := node.(*CenturyStatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyNoResolver)
}

func TestCenturyStatusNode_Init_DeferredOnUnresolvedAgent(t *testing.T) {
	t.Parallel()
	// AC-C7: resolver 가 에이전트를 찾지 못해도 Init 은 성공해야 한다 (deferred connection).
	resolver := &centuryMockResolver{err: assert.AnError}
	def := newCenturyNodeDef("ct-status", "century-status")
	node, err := NewCenturyStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	n := node.(*CenturyStatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "missing"}))
	require.NoError(t, n.Init(context.Background()))
	_ = n.Shutdown(context.Background())
	assert.Nil(t, n.agent, "agent should remain nil for deferred connection")
}

func TestCenturyStatusNode_Init_NotCenturyAgent(t *testing.T) {
	t.Parallel()
	// AC-C6: resolve 된 agent 가 century 타입이 아니면 에러.
	notCentury := &otherProtoAgent{}
	transport := &centuryMockTransport{agent: notCentury}
	resolver := &centuryMockResolver{transport: transport}

	def := newCenturyNodeDef("ct-status", "century-status")
	node, err := NewCenturyStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	n := node.(*CenturyStatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyAgentNotCentury)
}

func TestCenturyStatusNode_Init_NoAccessor(t *testing.T) {
	t.Parallel()
	transport := &centuryMockTransportNoAccessor{}
	resolver := &centuryMockResolver{transport: transport}
	def := newCenturyNodeDef("ct-status", "century-status")
	node, err := NewCenturyStatusNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	n := node.(*CenturyStatusNode)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = n.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyAgentNotCentury)
}

// ===========================================================================
// CenturyStatusNode.Process — AC-C1 변형 (단일 요청)
// ===========================================================================

func TestCenturyStatusNode_Process_BulkResponse(t *testing.T) {
	t.Parallel()
	// agent 가 drain 응답에 frames 배열을 반환하면, Process 는 그 결과를 그대로 펼친다.
	resp := centuryMustJSON(t, map[string]any{
		"count": 2,
		"frames": []map[string]any{
			{"seq": 1, "timestamp_ms": int64(10), "raw_hex": "aa", "function_code": 6},
			{"seq": 2, "timestamp_ms": int64(20), "raw_hex": "bb", "function_code": 6},
		},
	})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, map[string]any{
		"agent_ref": "ct-1", "poll_command": "drain",
	})

	in := message.New()
	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1, "Process 는 단일 응답을 반환한다 (펼친 형태)")

	// 마지막 보낸 command 가 drain 인지 확인.
	require.GreaterOrEqual(t, mock.processCount, 1)
	cmd := parseCmd(t, mock.processData[mock.processCount-1])
	assert.Equal(t, "drain", cmd["command"])
}

func TestCenturyStatusNode_Process_AgentNil(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-status", "century-status")
	base := NewBaseNode(def)
	n := &CenturyStatusNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: time.Second},
		sourceCh:        make(chan message.Message, 8),
		stopCh:          make(chan struct{}),
	}
	require.NoError(t, n.centuryNodeBase.configure(map[string]any{"agent_ref": "ct-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	_, err := n.Process(context.Background(), message.New())
	assert.ErrorIs(t, err, ErrCenturyProcessFailed)
}

// ===========================================================================
// CenturyStatusNode 폴링 — AC-C1, AC-C8 (FrameNotifyCh 즉시 반응)
// ===========================================================================

func TestCenturyStatusNode_PollLoop_EmitsDecodedFrame(t *testing.T) {
	t.Parallel()
	decoded := mustJSONRaw(t, map[string]any{
		"type":       "century_reg02_response",
		"sub_dev_id": 0x3B,
		"register":   2,
	})
	resp := centuryMustJSON(t, map[string]any{
		"count": 1,
		"frames": []map[string]any{
			{
				"seq":           uint64(101),
				"timestamp_ms":  int64(1000),
				"raw_hex":       "deadbeef",
				"function_code": 6,
				"decoded":       decoded,
			},
		},
	})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, map[string]any{
		"agent_ref":     "ct-1",
		"poll_interval": "20ms",
		"poll_command":  "drain",
	})
	defer func() { _ = n.Shutdown(context.Background()) }()

	go n.pollLoop()
	// notify 를 한 번 트리거.
	mock.notifyCh <- struct{}{}

	select {
	case msg := <-n.sourceCh:
		v, ok := msg.Payload().Get("type")
		require.True(t, ok)
		assert.Equal(t, "century_reg02_response", v)
		seq, _ := msg.Payload().Get("seq")
		assert.EqualValues(t, 101, seq)
		// metadata 검증
		src, _ := msg.Metadata().Get("century_source")
		assert.Equal(t, "poll_bulk", src)
		nid, _ := msg.Metadata().Get("century_node_id")
		assert.Equal(t, n.ID(), nid)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for status message")
	}
}

func TestCenturyStatusNode_PollLoop_SkipsFramesWithoutDecoded(t *testing.T) {
	t.Parallel()
	// decoded 가 없으면 status 노드는 송출하지 않는다.
	resp := centuryMustJSON(t, map[string]any{
		"count": 1,
		"frames": []map[string]any{
			{"seq": uint64(1), "timestamp_ms": int64(10), "raw_hex": "aa", "function_code": 11},
		},
	})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, map[string]any{
		"agent_ref":     "ct-1",
		"poll_interval": "20ms",
		"poll_command":  "drain",
	})
	defer func() { _ = n.Shutdown(context.Background()) }()

	go n.pollLoop()
	mock.notifyCh <- struct{}{}

	select {
	case <-n.sourceCh:
		t.Fatal("status node should NOT emit frames without decoded payload")
	case <-time.After(100 * time.Millisecond):
		// 기대된 동작 — 무 송출.
	}
}

// ===========================================================================
// CenturyControlNode — AC-C3 (항상 not_supported)
// ===========================================================================

func TestCenturyControlNode_AlwaysNotSupported(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-control", "century-control")
	base := NewBaseNode(def)
	n := &CenturyControlNode{centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: time.Second}}
	require.NoError(t, n.centuryNodeBase.configure(map[string]any{"agent_ref": "ct-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	// 제어 키 메시지를 보내도 항상 not_supported.
	in := message.New()
	in.Payload().Set("power", true)
	in.Payload().Set("mode", "cooling")
	in.Payload().Set("temperature", 24.0)

	out, err := n.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, out, 1)

	status, _ := out[0].Payload().Get("status")
	assert.Equal(t, "not_supported", status)
	reason, _ := out[0].Payload().Get("reason")
	assert.Equal(t, "century_passive_only", reason)
	// AC-B9: agent.Process 는 절대 호출하지 않는다 — n.agent 는 nil 이므로
	// 호출 시 error 가 나야 하지만 not_supported 응답이 반환되었으므로 호출되지 않았다.
	assert.Nil(t, n.agent, "control node must not call agent.Process()")
}

func TestCenturyControlNode_EmptyMessage_StillNotSupported(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-control", "century-control")
	base := NewBaseNode(def)
	n := &CenturyControlNode{centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: time.Second}}
	require.NoError(t, n.centuryNodeBase.configure(map[string]any{"agent_ref": "ct-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	require.Len(t, out, 1)
	status, _ := out[0].Payload().Get("status")
	assert.Equal(t, "not_supported", status)
}

// ===========================================================================
// CenturyNode 통합 — AC-C4 (제어 키 분기)
// ===========================================================================

func TestCenturyNode_Process_ControlKeyReturnsNotSupported(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-combined", "century")
	base := NewBaseNode(def)
	mock := newMockCenturyAgent([]byte(`{"ok": true}`))
	n := &CenturyNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: time.Second, agent: mock},
		sourceCh:        make(chan message.Message, 8),
		stopCh:          make(chan struct{}),
	}
	require.NoError(t, n.centuryNodeBase.configure(map[string]any{"agent_ref": "ct-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	for _, key := range []string{"power", "mode", "temperature", "setpoint", "fan_speed"} {
		t.Run(key, func(t *testing.T) {
			in := message.New()
			in.Payload().Set(key, "test-value")
			out, err := n.Process(context.Background(), in)
			require.NoError(t, err)
			require.Len(t, out, 1)
			status, _ := out[0].Payload().Get("status")
			assert.Equal(t, "not_supported", status)
		})
	}
	// 제어 키 분기로 인해 agent.Process 가 호출되지 않았음을 확인.
	assert.Zero(t, mock.processCount, "control-key branch must short-circuit agent.Process")
}

func TestCenturyNode_Process_NoControlKey_QueriesStatus(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-combined", "century")
	base := NewBaseNode(def)
	resp := centuryMustJSON(t, map[string]any{"frames_captured": 99})
	mock := newMockCenturyAgent(resp)
	n := &CenturyNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: time.Second, agent: mock},
		sourceCh:        make(chan message.Message, 8),
		stopCh:          make(chan struct{}),
	}
	require.NoError(t, n.centuryNodeBase.configure(map[string]any{
		"agent_ref":    "ct-1",
		"poll_command": "get_stats",
	}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	out, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	require.Len(t, out, 1)
	v, ok := out[0].Payload().Get("frames_captured")
	require.True(t, ok)
	assert.EqualValues(t, 99, v)
	assert.Equal(t, 1, mock.processCount)
}

// ===========================================================================
// CenturyRawFrameNode — AC-C5, AC-F1
// ===========================================================================

func TestCenturyRawFrameNode_PollEmitsAllFrames(t *testing.T) {
	t.Parallel()
	// 두 frame: 하나는 decoded 성공, 하나는 decoded 없음 (예: read request).
	// raw 노드는 둘 다 emit 해야 한다.
	resp := centuryMustJSON(t, map[string]any{
		"count": 2,
		"frames": []map[string]any{
			{
				"seq":           uint64(1),
				"timestamp_ms":  int64(100),
				"raw_hex":       "010030001400000663b000020001",
				"function_code": 6,
				"register":      byte(2),
				"decoded":       mustJSONRaw(t, map[string]any{"type": "century_reg02_response"}),
			},
			{
				"seq":           uint64(2),
				"timestamp_ms":  int64(200),
				"raw_hex":       "30000100050000000b3b0002",
				"function_code": 11,
				"decode_error":  "unsupported direction: read request frames have no decoded payload (use raw frame node instead)",
			},
		},
	})
	mock := newMockCenturyAgent(resp)
	def := newCenturyNodeDef("ct-raw", "century-raw-frame")
	base := NewBaseNode(def)
	n := &CenturyRawFrameNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: time.Second, agent: mock},
		sourceCh:        make(chan message.Message, 8),
		stopCh:          make(chan struct{}),
		pollInterval:    20 * time.Millisecond,
	}
	require.NoError(t, n.centuryNodeBase.configure(map[string]any{"agent_ref": "ct-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)

	defer func() { _ = n.Shutdown(context.Background()) }()
	go n.pollLoop()
	mock.notifyCh <- struct{}{}

	got := drainSource(t, n.sourceCh, 2, 500*time.Millisecond)
	require.Len(t, got, 2, "raw node should emit both frames")

	// 첫 frame: decoded 성공 → validation_stage="ok"
	stage1, _ := got[0].Payload().Get("validation_stage")
	assert.Equal(t, "ok", stage1)
	conf1, _ := got[0].Payload().Get("confirmation_status")
	assert.Equal(t, "raw", conf1)
	rawHex1, _ := got[0].Payload().Get("raw_hex")
	assert.Equal(t, "010030001400000663b000020001", rawHex1)

	// 두 번째 frame: decode error 있음 → validation_stage != "ok"
	stage2, _ := got[1].Payload().Get("validation_stage")
	assert.NotEqual(t, "ok", stage2)
}

// AC-F1: dedupe 가 일어나도 raw 노드는 모든 WRITE 를 emit 해야 한다.
// 합성 시나리오: agent 가 두 개의 동일한 WRITE frame 을 반환한다 (dedup 은 decoded 만 영향).
func TestCenturyRawFrameNode_AC_F1_EmitsBothDuplicateWrites(t *testing.T) {
	t.Parallel()
	// 동일 raw_hex 의 WRITE frame 두 개. agent 의 ring buffer 는 양쪽 모두 보관.
	resp := centuryMustJSON(t, map[string]any{
		"count": 2,
		"frames": []map[string]any{
			{
				"seq":           uint64(10),
				"timestamp_ms":  int64(1000),
				"raw_hex":       "3000010013000000c3b00040002040000000100000000000000000c00f",
				"function_code": 0x0C,
				"register":      byte(4),
				// 첫 번째 WRITE 만 decoded — dedupe 가 두 번째 decoded 를 제거함을 시뮬레이트.
				"decoded": mustJSONRaw(t, map[string]any{"type": "century_reg04_write_request"}),
			},
			{
				"seq":           uint64(11),
				"timestamp_ms":  int64(1010),
				"raw_hex":       "3000010013000000c3b00040002040000000100000000000000000c00f",
				"function_code": 0x0C,
				"register":      byte(4),
				// 두 번째 WRITE 는 dedup 으로 decoded 가 빠진 상태.
			},
		},
	})
	mock := newMockCenturyAgent(resp)
	def := newCenturyNodeDef("ct-raw", "century-raw-frame")
	base := NewBaseNode(def)
	n := &CenturyRawFrameNode{
		centuryNodeBase: centuryNodeBase{BaseNode: base, timeout: time.Second, agent: mock},
		sourceCh:        make(chan message.Message, 8),
		stopCh:          make(chan struct{}),
		pollInterval:    20 * time.Millisecond,
	}
	require.NoError(t, n.centuryNodeBase.configure(map[string]any{"agent_ref": "ct-1"}))
	_ = base.TransitionTo(lifecycle.StateInitializing)
	_ = base.TransitionTo(lifecycle.StateRunning)
	defer func() { _ = n.Shutdown(context.Background()) }()
	go n.pollLoop()
	mock.notifyCh <- struct{}{}

	got := drainSource(t, n.sourceCh, 2, 500*time.Millisecond)
	require.Len(t, got, 2, "AC-F1: raw node must emit both duplicate WRITEs (dedupe must not affect raw stream)")

	// 동시에 status node 로 같은 응답을 보냈을 때는 decoded 없는 frame 은 skip 되어야 한다.
	statusResp := resp
	statusMock := newMockCenturyAgent(statusResp)
	stat := makeRunningStatusNode(t, statusMock, map[string]any{
		"agent_ref":     "ct-1",
		"poll_interval": "20ms",
		"poll_command":  "drain",
	})
	defer func() { _ = stat.Shutdown(context.Background()) }()
	go stat.pollLoop()
	statusMock.notifyCh <- struct{}{}

	gotStatus := drainSource(t, stat.sourceCh, 1, 500*time.Millisecond)
	assert.Len(t, gotStatus, 1, "AC-F1: status node must see only 1 decoded WRITE (dedupe removed second)")
}

// ===========================================================================
// 헬퍼
// ===========================================================================

func centuryMustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func mustJSONRaw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func parseCmd(t *testing.T, data []byte) map[string]any {
	t.Helper()
	var cmd map[string]any
	require.NoError(t, json.Unmarshal(data, &cmd))
	return cmd
}

// ===========================================================================
// 추가 팩토리 / Init / Reinit 커버리지 (각 4 노드 타입)
// ===========================================================================

func TestNewCenturyControlNode_Factory(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-c", "century-control")
	n, err := NewCenturyControlNode(def)
	require.NoError(t, err)
	assert.Equal(t, "century-control", n.Type())
	cn := n.(*CenturyControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-1"}))
}

func TestNewCenturyControlNode_Init_Lifecycle(t *testing.T) {
	t.Parallel()
	mock := newMockCenturyAgent([]byte(`{}`))
	resolver := &centuryMockResolver{transport: &centuryMockTransport{agent: mock}}
	// agent 가 century 가 아니라 mock 이므로 ErrCenturyAgentNotCentury 가 나야 한다.
	def := newCenturyNodeDef("ct-c", "century-control")
	n, err := NewCenturyControlNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	cn := n.(*CenturyControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = cn.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyAgentNotCentury)
}

func TestNewCenturyControlNode_Init_Deferred(t *testing.T) {
	t.Parallel()
	// resolver 가 nil 인 경우 hard-fail. deferred connection 은 ResolveAgent 가 err 반환할 때.
	resolver := &centuryMockResolver{err: assert.AnError}
	def := newCenturyNodeDef("ct-c", "century-control")
	n, err := NewCenturyControlNode(def, WithAgentResolver(resolver))
	require.NoError(t, err)
	cn := n.(*CenturyControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-1"}))
	require.NoError(t, cn.Init(context.Background()), "deferred connection should succeed when ResolveAgent fails")
	_ = cn.Shutdown(context.Background())
}

func TestNewCenturyControlNode_Reinit(t *testing.T) {
	t.Parallel()
	// Reinit 만 단독으로 호출 가능해야 한다 (no resolver — 즉시 에러).
	def := newCenturyNodeDef("ct-c", "century-control")
	n, err := NewCenturyControlNode(def)
	require.NoError(t, err)
	cn := n.(*CenturyControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = cn.Reinit(context.Background())
	assert.ErrorIs(t, err, ErrCenturyNoResolver)
}

func TestNewCenturyControlNode_AgentRef_Reflects(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-c", "century-control")
	n, err := NewCenturyControlNode(def)
	require.NoError(t, err)
	cn := n.(*CenturyControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-custom"}))
	ref := cn.AgentRef()
	assert.Equal(t, "ct-custom", ref.AgentID)
	assert.Equal(t, "ct-custom", ref.AgentName)
}

func TestNewCenturyNode_Factory(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct", "century")
	n, err := NewCenturyNode(def)
	require.NoError(t, err)
	cn := n.(*CenturyNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-1"}))
	assert.Equal(t, "century", n.Type())
	// SourceCh 도 정상 노출.
	assert.NotNil(t, cn.SourceCh())
}

func TestNewCenturyNode_Init_NoResolver(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct", "century")
	n, err := NewCenturyNode(def)
	require.NoError(t, err)
	cn := n.(*CenturyNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = cn.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyNoResolver)
}

func TestNewCenturyRawFrameNode_Factory(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-raw", "century-raw-frame")
	n, err := NewCenturyRawFrameNode(def)
	require.NoError(t, err)
	rn := n.(*CenturyRawFrameNode)
	require.NoError(t, rn.Configure(map[string]any{
		"agent_ref":     "ct-1",
		"poll_interval": "50ms",
	}))
	assert.Equal(t, "century-raw-frame", n.Type())
	// raw 노드의 PollCommand 는 항상 drain 으로 강제됨.
	assert.Equal(t, centuryCmdDrain, rn.centuryCfg.PollCommand)
	assert.Equal(t, 50*time.Millisecond, rn.pollInterval)
	// Process 는 source-only 노드이므로 항상 nil 반환.
	out, perr := rn.Process(context.Background(), message.New())
	assert.NoError(t, perr)
	assert.Nil(t, out)
}

func TestNewCenturyRawFrameNode_Init_NoResolver(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-raw", "century-raw-frame")
	n, err := NewCenturyRawFrameNode(def)
	require.NoError(t, err)
	rn := n.(*CenturyRawFrameNode)
	require.NoError(t, rn.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = rn.Init(context.Background())
	assert.ErrorIs(t, err, ErrCenturyNoResolver)
}

func TestNewCenturyStatusNode_AgentRef_Reflects(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-s", "century-status")
	n, err := NewCenturyStatusNode(def)
	require.NoError(t, err)
	sn := n.(*CenturyStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "ct-ref-x"}))
	ref := sn.AgentRef()
	assert.Equal(t, "ct-ref-x", ref.AgentID)
	assert.NotNil(t, sn.SourceCh())
}

// pollSingle 경로 + Status.Reinit / Status.SourceCh 커버.
func TestCenturyStatusNode_PollLoop_GetStatsBranch(t *testing.T) {
	t.Parallel()
	resp := centuryMustJSON(t, map[string]any{"frames_captured": 5})
	mock := newMockCenturyAgent(resp)
	n := makeRunningStatusNode(t, mock, map[string]any{
		"agent_ref":     "ct-1",
		"poll_interval": "20ms",
		"poll_command":  "get_stats",
	})
	defer func() { _ = n.Shutdown(context.Background()) }()
	go n.pollLoop()
	mock.notifyCh <- struct{}{}

	select {
	case msg := <-n.sourceCh:
		v, _ := msg.Payload().Get("frames_captured")
		assert.EqualValues(t, 5, v)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for stats message")
	}
}

// CenturyStatusNode.Reinit happy path.
func TestCenturyStatusNode_Reinit(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-s", "century-status")
	n, err := NewCenturyStatusNode(def)
	require.NoError(t, err)
	sn := n.(*CenturyStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = sn.Reinit(context.Background())
	assert.ErrorIs(t, err, ErrCenturyNoResolver)
}

// CenturyNode pollLoop + Shutdown + Reinit 커버리지.
func TestCenturyNode_PollLoop_DrainEmit(t *testing.T) {
	t.Parallel()
	decoded := mustJSONRaw(t, map[string]any{"type": "century_reg02_response"})
	resp := centuryMustJSON(t, map[string]any{
		"count": 1,
		"frames": []map[string]any{
			{
				"seq":           uint64(1),
				"timestamp_ms":  int64(100),
				"raw_hex":       "aabb",
				"function_code": 6,
				"decoded":       decoded,
			},
		},
	})
	mock := newMockCenturyAgent(resp)
	def := newCenturyNodeDef("ct", "century")
	n, err := NewCenturyNode(def)
	require.NoError(t, err)
	cn := n.(*CenturyNode)
	require.NoError(t, cn.Configure(map[string]any{
		"agent_ref":     "ct-1",
		"poll_interval": "20ms",
		"poll_command":  "drain",
	}))
	// agent 를 직접 주입 (테스트용)
	cn.centuryNodeBase.agent = mock
	_ = cn.BaseNode.TransitionTo(lifecycle.StateInitializing)
	_ = cn.BaseNode.TransitionTo(lifecycle.StateRunning)

	defer func() { _ = cn.Shutdown(context.Background()) }()
	go cn.pollLoop()
	mock.notifyCh <- struct{}{}

	select {
	case msg := <-cn.sourceCh:
		v, _ := msg.Payload().Get("type")
		assert.Equal(t, "century_reg02_response", v)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for century combined node emit")
	}
}

func TestCenturyNode_PollLoop_GetStatsBranch(t *testing.T) {
	t.Parallel()
	resp := centuryMustJSON(t, map[string]any{"frames_captured": 9})
	mock := newMockCenturyAgent(resp)
	def := newCenturyNodeDef("ct", "century")
	n, err := NewCenturyNode(def)
	require.NoError(t, err)
	cn := n.(*CenturyNode)
	require.NoError(t, cn.Configure(map[string]any{
		"agent_ref":     "ct-1",
		"poll_interval": "20ms",
		"poll_command":  "get_stats",
	}))
	cn.centuryNodeBase.agent = mock
	_ = cn.BaseNode.TransitionTo(lifecycle.StateInitializing)
	_ = cn.BaseNode.TransitionTo(lifecycle.StateRunning)
	defer func() { _ = cn.Shutdown(context.Background()) }()
	go cn.pollLoop()
	mock.notifyCh <- struct{}{}

	select {
	case msg := <-cn.sourceCh:
		v, _ := msg.Payload().Get("frames_captured")
		assert.EqualValues(t, 9, v)
	case <-time.After(500 * time.Millisecond):
		t.Fatal("timed out waiting for stats message")
	}
}

func TestCenturyNode_Reinit_NoResolver(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct", "century")
	n, err := NewCenturyNode(def)
	require.NoError(t, err)
	cn := n.(*CenturyNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = cn.Reinit(context.Background())
	assert.ErrorIs(t, err, ErrCenturyNoResolver)
}

func TestCenturyRawFrameNode_Reinit_NoResolver(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-raw", "century-raw-frame")
	n, err := NewCenturyRawFrameNode(def)
	require.NoError(t, err)
	rn := n.(*CenturyRawFrameNode)
	require.NoError(t, rn.Configure(map[string]any{"agent_ref": "ct-1"}))
	err = rn.Reinit(context.Background())
	assert.ErrorIs(t, err, ErrCenturyNoResolver)
}

func TestCenturyRawFrameNode_SourceCh(t *testing.T) {
	t.Parallel()
	def := newCenturyNodeDef("ct-raw", "century-raw-frame")
	n, err := NewCenturyRawFrameNode(def)
	require.NoError(t, err)
	rn := n.(*CenturyRawFrameNode)
	assert.NotNil(t, rn.SourceCh())
}

// validation_stage 헬퍼 단위 테스트.
func TestCenturyValidationStage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  string
		want string
	}{
		{"", "ok"},
		{"register length mismatch", "register_length"},
		{"payload prefix invalid", "payload_prefix"},
		{"function_code 0x0B", "header"},
		{"unknown register 0x05", "payload_prefix"},
		{"random other error", "decode_error"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := centuryValidationStage(tt.err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// containsSubstr 단위 테스트.
func TestCenturyContainsSubstr(t *testing.T) {
	t.Parallel()
	assert.True(t, containsSubstr("hello world", "world"))
	assert.True(t, containsSubstr("hello world", ""))
	assert.False(t, containsSubstr("hello", "world"))
	assert.True(t, containsSubstr("abc", "abc"))
}

// buildCenturyStatusCommand 변형 검증.
func TestBuildCenturyStatusCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		cfg         CenturyNodeConfig
		wantCommand string
		wantCount   any
	}{
		{"drain default", CenturyNodeConfig{PollCommand: "drain", BatchSize: 32}, "drain", nil},
		{"get_recent count", CenturyNodeConfig{PollCommand: "get_recent", RecentCount: 7}, "get_recent", 7},
		{"unknown -> get_stats", CenturyNodeConfig{PollCommand: "weird"}, "get_stats", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := buildCenturyStatusCommand(tt.cfg)
			require.NoError(t, err)
			var got map[string]any
			require.NoError(t, json.Unmarshal(b, &got))
			assert.Equal(t, tt.wantCommand, got["command"])
			if tt.wantCount != nil {
				assert.EqualValues(t, tt.wantCount, got["count"])
			}
		})
	}
}

// drainSource 는 최대 max 개의 메시지를 deadline 까지 sourceCh 로부터 수집한다.
func drainSource(t *testing.T, ch <-chan message.Message, max int, deadline time.Duration) []message.Message {
	t.Helper()
	out := make([]message.Message, 0, max)
	timer := time.NewTimer(deadline)
	defer timer.Stop()
	for len(out) < max {
		select {
		case m := <-ch:
			out = append(out, m)
		case <-timer.C:
			return out
		}
	}
	return out
}
