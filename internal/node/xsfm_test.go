package node

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/xsfm"
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/message"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼 — 실제 port 모드 XSFM 에이전트 + resolver/transport 배선
// ---------------------------------------------------------------------------

// newPortXSFMAgent 는 port 모드 XSFM 에이전트를 생성하고 지정 디바이스를
// add_device 로 등록한다.
func newPortXSFMAgent(t *testing.T, deviceIDs ...string) *xsfm.XSFMAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:   "ap-agent-1",
		Name: "xsfm-1",
		Type: "xsfm",
		Transport: agent.TransportConfig{
			Type: "mqtt",
			Options: map[string]any{
				"transport_mode": "port",
				// 제어 응답 대기를 비활성화하여 fire-and-forget (에코 유입 없이 테스트).
				// ControlPort 방출은 대기 이전에 발생하므로 출력 포트 검증에는 영향 없다.
				"control_response_timeout": "0s",
				"payload_mapping": map[string]any{
					"power_field":     "power",
					"fan_speed_field": "fan_speed",
				},
			},
		},
	}
	a, err := xsfm.NewXSFMAgent(cfg)
	require.NoError(t, err)
	ap, ok := a.(*xsfm.XSFMAgent)
	require.True(t, ok)

	for _, id := range deviceIDs {
		_, err := ap.Process([]byte(`{"command":"add_device","device_id":"` + id + `"}`))
		require.NoError(t, err)
	}
	return ap
}

// wireAPResolver 는 주어진 에이전트를 노드에 주입하는 resolver 를 반환한다.
func wireAPResolver(ap *xsfm.XSFMAgent) AgentResolver {
	return &mockNASAResolver{transport: &mockNASATransport{agent: ap}}
}

// drainForType 는 채널에서 timeout 내에 지정 타입 메시지가 나올 때까지 drain 한다.
func drainForType(ch <-chan message.Message, wantType string, timeout time.Duration) (message.Message, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case msg := <-ch:
			if msg.Type() == wantType {
				return msg, true
			}
		case <-deadline:
			return nil, false
		}
	}
}

// ---------------------------------------------------------------------------
// Configure / 타입 검증
// ---------------------------------------------------------------------------

func TestXsfmStatusNode_Type(t *testing.T) {
	t.Parallel()
	def := flow.NewNodeDef("ap-status", "xsfm-status")
	n, err := NewXsfmStatusNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ap-1"}))
	assert.Equal(t, "xsfm-status", n.Type())
}

func TestXsfmControlNode_Type(t *testing.T) {
	t.Parallel()
	def := flow.NewNodeDef("ap-control", "xsfm-control")
	n, err := NewXsfmControlNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ap-1"}))
	assert.Equal(t, "xsfm-control", n.Type())
}

func TestXsfmNode_AgentRefRequired(t *testing.T) {
	t.Parallel()
	sdef := flow.NewNodeDef("ap-status", "xsfm-status")
	sn, err := NewXsfmStatusNode(sdef)
	require.NoError(t, err)
	assert.ErrorIs(t, sn.Configure(map[string]any{}), ErrXSFMMissingAgentRef)

	cdef := flow.NewNodeDef("ap-control", "xsfm-control")
	cn, err := NewXsfmControlNode(cdef)
	require.NoError(t, err)
	assert.ErrorIs(t, cn.Configure(map[string]any{}), ErrXSFMMissingAgentRef)
}

// ---------------------------------------------------------------------------
// Module 6.1 / 1B.3 — 상태 텔레메트리 + 상태 입력 포트
// ---------------------------------------------------------------------------

// 6.1 + 1B.3: 상태 입력 포트(Process)로 device-STATE 를 주입하면 에이전트가
// device_state_changed 를 방출하고, status 노드의 SourceCh 로 텔레메트리가 흐른다
// (influxdb 경로로 향하는 telemetry 메시지 검증).
func TestXsfmStatusNode_StateInputEmitsTelemetry(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")

	def := flow.NewNodeDef("ap-status", "xsfm-status")
	node, err := NewXsfmStatusNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	sn := node.(*XsfmStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, sn.Init(context.Background()))
	defer func() { _ = sn.Shutdown(context.Background()) }()

	// 상태 입력 포트: 상류 device-STATE 메시지를 주입한다 (REQ-05-01 port 경로).
	in := message.New()
	in.Payload().Set("device_id", "ap-101")
	in.Payload().Set("power", true)
	in.Payload().Set("fan_speed", float64(2))
	out, err := sn.Process(context.Background(), in)
	require.NoError(t, err)
	assert.Empty(t, out, "상태 입력 포트는 하류로 직접 메시지를 반환하지 않는다")

	// 에이전트가 FeedState 로 로스터를 갱신하고 device_state_changed 를 방출 → SourceCh.
	msg, ok := drainForType(sn.SourceCh(), "device_state_changed", 2*time.Second)
	require.True(t, ok, "status 노드는 device_state_changed 텔레메트리를 SourceCh 로 방출해야 한다")

	dev, ok := msg.Payload().Get("device_id")
	require.True(t, ok)
	assert.Equal(t, "ap-101", dev)
	power, ok := msg.Payload().Get("power")
	require.True(t, ok)
	assert.Equal(t, true, power)

	// 에이전트 로스터도 갱신되었는지 확인 (FeedState 경로 검증).
	d, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, d.Online)
	assert.True(t, d.Power)
	assert.Equal(t, 2, d.FanSpeed)
}

// ---------------------------------------------------------------------------
// Module 1B.4 — 제어 노드 Process → ControlPort → 제어 출력 포트 방출
// ---------------------------------------------------------------------------

// 1B.4: control 노드 Process(set_power) → 에이전트가 ControlPort 로 명령 방출 →
// control 노드의 출력 포트(SourceCh)가 하류 message.Message 1건을 방출한다.
func TestXsfmControlNode_ControlOutputPort(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")

	def := flow.NewNodeDef("ap-control", "xsfm-control")
	node, err := NewXsfmControlNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	cn := node.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, cn.Init(context.Background()))
	defer func() { _ = cn.Shutdown(context.Background()) }()

	// 제어 입력: set_power 명령.
	in := message.New()
	in.Payload().Set("device_id", "ap-101")
	in.Payload().Set("command", "set_power")
	in.Payload().Set("params", map[string]any{"power": true})
	respMsgs, err := cn.Process(context.Background(), in)
	require.NoError(t, err)
	assert.Len(t, respMsgs, 1, "제어 Process 는 응답 메시지 1건을 반환한다")

	// 제어 출력 포트: 에이전트 ControlPort 방출이 하류 message 로 전달되었는지 확인.
	msg, ok := drainForType(cn.SourceCh(), "device_command", 2*time.Second)
	require.True(t, ok, "control 노드는 제어 출력 포트로 명령 메시지 1건을 방출해야 한다")

	dev, ok := msg.Payload().Get("device_id")
	require.True(t, ok)
	assert.Equal(t, "ap-101", dev)
	power, ok := msg.Payload().Get("power")
	require.True(t, ok)
	assert.Equal(t, true, power, "인코딩된 명령 페이로드에 power=true 가 포함되어야 한다")
}

// 제어 키(command 미지정)로부터 명령 추론: power 만 → set_power.
func TestXsfmControlNode_InferSetPowerFromKey(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-201")

	def := flow.NewNodeDef("ap-control", "xsfm-control")
	node, err := NewXsfmControlNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	cn := node.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, cn.Init(context.Background()))
	defer func() { _ = cn.Shutdown(context.Background()) }()

	in := message.New()
	in.Payload().Set("device_id", "ap-201")
	in.Payload().Set("power", true)
	_, err = cn.Process(context.Background(), in)
	require.NoError(t, err)

	msg, ok := drainForType(cn.SourceCh(), "device_command", 2*time.Second)
	require.True(t, ok)
	dev, _ := msg.Payload().Get("device_id")
	assert.Equal(t, "ap-201", dev)
}

// ---------------------------------------------------------------------------
// Module 1B.7 / 7.5 — 상태 입력 포트 ≠ 제어 출력 포트 (분리 불변식)
// ---------------------------------------------------------------------------

// 1B.7: status 노드의 상태 입력 포트(Process)와 control 노드의 제어 출력 포트(SourceCh)는
// 서로 다른 노드의 서로 다른 포트이며 절대 결합되지 않는다 (REQ-01-12).
func TestXsfmNodes_StateInputAndControlOutputAreSeparate(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")

	sdef := flow.NewNodeDef("ap-status", "xsfm-status")
	snode, err := NewXsfmStatusNode(sdef, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	sn := snode.(*XsfmStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, sn.Init(context.Background()))
	defer func() { _ = sn.Shutdown(context.Background()) }()

	cdef := flow.NewNodeDef("ap-control", "xsfm-control")
	cnode, err := NewXsfmControlNode(cdef, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	cn := cnode.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, cn.Init(context.Background()))
	defer func() { _ = cn.Shutdown(context.Background()) }()

	// 서로 다른 노드 타입 (상태 입력 포트와 제어 출력 포트가 서로 다른 노드에 있음).
	assert.NotEqual(t, sn.Type(), cn.Type(), "status 와 control 은 서로 다른 노드 타입이어야 한다")

	// 제어 명령을 control 노드로 보낸다.
	in := message.New()
	in.Payload().Set("device_id", "ap-101")
	in.Payload().Set("command", "set_power")
	in.Payload().Set("params", map[string]any{"power": true})
	_, err = cn.Process(context.Background(), in)
	require.NoError(t, err)

	// 제어 출력은 control 노드의 출력 포트에만 나타난다.
	cmdMsg, ok := drainForType(cn.SourceCh(), "device_command", 2*time.Second)
	require.True(t, ok, "제어 출력은 control 노드의 출력 포트로 방출되어야 한다")
	assert.Equal(t, "device_command", cmdMsg.Type())

	// 제어 출력은 status 노드의 텔레메트리 포트로는 절대 흐르지 않는다.
	if leaked, ok := drainForType(sn.SourceCh(), "device_command", 300*time.Millisecond); ok {
		t.Fatalf("제어 명령이 status 노드 텔레메트리 포트로 누출되었다: %v", leaked)
	}
}

// AgentRef 반환 + Reinit(에이전트 재시작 후 재구성) 커버리지.
func TestXsfmNodes_AgentRefAndReinit(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")

	sdef := flow.NewNodeDef("ap-status", "xsfm-status")
	sn0, err := NewXsfmStatusNode(sdef, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	sn := sn0.(*XsfmStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	assert.Equal(t, "ap-agent-1", sn.AgentRef().AgentName)
	require.NoError(t, sn.Init(context.Background()))
	require.NoError(t, sn.Reinit(context.Background()))
	_ = sn.Shutdown(context.Background())

	cdef := flow.NewNodeDef("ap-control", "xsfm-control")
	cn0, err := NewXsfmControlNode(cdef, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	cn := cn0.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	assert.Equal(t, "ap-agent-1", cn.AgentRef().AgentID)
	require.NoError(t, cn.Init(context.Background()))
	require.NoError(t, cn.Reinit(context.Background()))
	_ = cn.Shutdown(context.Background())
}

// resolve 실패(NoResolver/NotXSFM 이외)는 deferred connection 으로 Running 진행.
func TestXsfmNodes_InitDeferredOnResolveFailure(t *testing.T) {
	resolver := &mockNASAResolver{err: assert.AnError}

	sdef := flow.NewNodeDef("ap-status", "xsfm-status")
	sn0, err := NewXsfmStatusNode(sdef, WithAgentResolver(resolver))
	require.NoError(t, err)
	sn := sn0.(*XsfmStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "missing"}))
	require.NoError(t, sn.Init(context.Background()))
	_ = sn.Shutdown(context.Background())

	cdef := flow.NewNodeDef("ap-control", "xsfm-control")
	cn0, err := NewXsfmControlNode(cdef, WithAgentResolver(resolver))
	require.NoError(t, err)
	cn := cn0.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "missing"}))
	require.NoError(t, cn.Init(context.Background()))
	_ = cn.Shutdown(context.Background())
}

// Init: resolver 미설정 시 NoResolver 에러; wrong 에이전트 타입 시 NotXSFM.
func TestXsfmNodes_InitErrors(t *testing.T) {
	// resolver 미설정.
	def := flow.NewNodeDef("ap-status", "xsfm-status")
	sn0, err := NewXsfmStatusNode(def)
	require.NoError(t, err)
	sn := sn0.(*XsfmStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "ap-1"}))
	assert.ErrorIs(t, sn.Init(context.Background()), ErrXSFMNoResolver)

	// 잘못된 에이전트 타입 (AgentAccessor 미구현 transport).
	badResolver := &mockNASAResolver{transport: &mockNASATransportNoAccessor{}}
	def2 := flow.NewNodeDef("ap-control", "xsfm-control")
	cn0, err := NewXsfmControlNode(def2, WithAgentResolver(badResolver))
	require.NoError(t, err)
	cn := cn0.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ap-1"}))
	assert.ErrorIs(t, cn.Init(context.Background()), ErrXSFMAgentNotXSFM)
}

// 제어 키/command 가 없는 메시지는 명령을 구성할 수 없어 에러.
func TestXsfmControlNode_NoCommandError(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")
	def := flow.NewNodeDef("ap-control", "xsfm-control")
	cn0, err := NewXsfmControlNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	cn := cn0.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, cn.Init(context.Background()))
	defer func() { _ = cn.Shutdown(context.Background()) }()

	in := message.New()
	in.Payload().Set("device_id", "ap-101")
	_, err = cn.Process(context.Background(), in)
	assert.ErrorIs(t, err, ErrXSFMProcessFailed)
}

// 상태 입력 포트: device_id 없는 메시지는 no-op (주입하지 않음).
func TestXsfmStatusNode_ProcessNoDeviceID(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")
	def := flow.NewNodeDef("ap-status", "xsfm-status")
	sn0, err := NewXsfmStatusNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	sn := sn0.(*XsfmStatusNode)
	require.NoError(t, sn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, sn.Init(context.Background()))
	defer func() { _ = sn.Shutdown(context.Background()) }()

	in := message.New()
	in.Payload().Set("power", true)
	out, err := sn.Process(context.Background(), in)
	require.NoError(t, err)
	assert.Empty(t, out)
}

// ---------------------------------------------------------------------------
// 노드 레지스트리 등록 (canonical 이름 + `_` 별칭)
// ---------------------------------------------------------------------------

func TestXsfmNodes_Registered(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	for _, name := range []string{
		"xsfm-status", "xsfm-control",
		"xsfm_status", "xsfm_control", // `_` 별칭
	} {
		assert.True(t, r.Has(name), "레지스트리는 %q 를 포함해야 한다", name)
	}

	// canonical 이름으로 노드 생성 시 올바른 타입이 나온다.
	sn, err := r.Create(flow.NewNodeDef("s", "xsfm-status"))
	require.NoError(t, err)
	_, ok := sn.(*XsfmStatusNode)
	assert.True(t, ok, "xsfm-status 는 *XsfmStatusNode 를 생성해야 한다")

	cn, err := r.Create(flow.NewNodeDef("c", "xsfm-control"))
	require.NoError(t, err)
	_, ok = cn.(*XsfmControlNode)
	assert.True(t, ok, "xsfm-control 은 *XsfmControlNode 를 생성해야 한다")

	// `_` 별칭으로도 동일 타입이 나온다.
	snAlias, err := r.Create(flow.NewNodeDef("s2", "xsfm_status"))
	require.NoError(t, err)
	_, ok = snAlias.(*XsfmStatusNode)
	assert.True(t, ok, "xsfm_status 별칭은 *XsfmStatusNode 를 생성해야 한다")
}
