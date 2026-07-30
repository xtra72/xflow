package node

import (
	"context"
	"encoding/json"
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
// 통합 XsfmNode — 상태 수신 + 제어 송신 (LGAPNode 통합 패턴)
// ---------------------------------------------------------------------------

func TestXsfmNode_Type(t *testing.T) {
	t.Parallel()
	def := flow.NewNodeDef("ap", "xsfm")
	n, err := NewXsfmNode(def)
	require.NoError(t, err)
	require.NoError(t, n.Configure(map[string]any{"agent_ref": "ap-1"}))
	assert.Equal(t, "xsfm", n.Type())
}

func TestXsfmUnifiedNode_AgentRefRequired(t *testing.T) {
	t.Parallel()
	def := flow.NewNodeDef("ap", "xsfm")
	n, err := NewXsfmNode(def)
	require.NoError(t, err)
	assert.ErrorIs(t, n.Configure(map[string]any{}), ErrXSFMMissingAgentRef)
}

// (a) 상태 메시지 입력(command/params 없음) → FeedState 주입 반영, (d) 텔레메트리 emit.
// 통합 노드는 command/params 가 없는 raw 상태 payload 를 상태 주입으로 라우팅하며,
// 에이전트는 FeedState 후 device_state_changed 를 방출하여 병합 SourceCh 로 흐른다.
func TestXsfmNode_StateInputFeedsAndEmitsTelemetry(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")

	def := flow.NewNodeDef("ap", "xsfm")
	node, err := NewXsfmNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	un := node.(*XsfmNode)
	require.NoError(t, un.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, un.Init(context.Background()))
	defer func() { _ = un.Shutdown(context.Background()) }()

	// 상태 주입 경로: raw 상태 payload (command/params 없음).
	in := message.New()
	in.Payload().Set("device_id", "ap-101")
	in.Payload().Set("power", true)
	in.Payload().Set("fan_speed", float64(2))
	out, err := un.Process(context.Background(), in)
	require.NoError(t, err)
	assert.Empty(t, out, "상태 주입 경로는 하류로 직접 메시지를 반환하지 않는다")

	// (d) FeedState → device_state_changed 텔레메트리가 병합 SourceCh 로 방출된다.
	msg, ok := drainForType(un.SourceCh(), "device_state_changed", 2*time.Second)
	require.True(t, ok, "통합 노드는 device_state_changed 텔레메트리를 SourceCh 로 방출해야 한다")
	dev, ok := msg.Payload().Get("device_id")
	require.True(t, ok)
	assert.Equal(t, "ap-101", dev)

	// (a) 에이전트 로스터가 FeedState 로 갱신되었는지 확인.
	d, err := ap.GetDevice("ap-101")
	require.NoError(t, err)
	assert.True(t, d.Online)
	assert.True(t, d.Power)
	assert.Equal(t, 2, d.FanSpeed)
}

// (b) 제어 명령 입력 → 에이전트 제어 호출(응답 1건), (c) port 모드 ControlPort emit →
// 병합 SourceCh 로 device_command 방출.
func TestXsfmNode_ControlInputCallsAgentAndEmitsControlPort(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")

	def := flow.NewNodeDef("ap", "xsfm")
	node, err := NewXsfmNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	un := node.(*XsfmNode)
	require.NoError(t, un.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, un.Init(context.Background()))
	defer func() { _ = un.Shutdown(context.Background()) }()

	// (b) 제어 경로: 명시적 command/params.
	in := message.New()
	in.Payload().Set("device_id", "ap-101")
	in.Payload().Set("command", "set_power")
	in.Payload().Set("params", map[string]any{"power": true})
	respMsgs, err := un.Process(context.Background(), in)
	require.NoError(t, err)
	assert.Len(t, respMsgs, 1, "제어 Process 는 응답 메시지 1건을 반환한다")

	// (c) port 모드 ControlPort 방출이 병합 SourceCh 로 device_command 로 전달된다.
	msg, ok := drainForType(un.SourceCh(), "device_command", 2*time.Second)
	require.True(t, ok, "통합 노드는 제어 출력을 병합 SourceCh 로 방출해야 한다")
	dev, ok := msg.Payload().Get("device_id")
	require.True(t, ok)
	assert.Equal(t, "ap-101", dev)
	power, ok := msg.Payload().Get("power")
	require.True(t, ok)
	assert.Equal(t, true, power, "인코딩된 명령 페이로드에 power=true 가 포함되어야 한다")
}

// AgentRef 반환 + Reinit(에이전트 재시작 후 두 소스 루프 재구성) 커버리지.
func TestXsfmNode_AgentRefAndReinit(t *testing.T) {
	ap := newPortXSFMAgent(t, "ap-101")

	def := flow.NewNodeDef("ap", "xsfm")
	node, err := NewXsfmNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	un := node.(*XsfmNode)
	require.NoError(t, un.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	assert.Equal(t, "ap-agent-1", un.AgentRef().AgentName)
	require.NoError(t, un.Init(context.Background()))
	require.NoError(t, un.Reinit(context.Background()))
	_ = un.Shutdown(context.Background())
}

// ---------------------------------------------------------------------------
// 노드 레지스트리 등록 (canonical 이름 + `_` 별칭)
// ---------------------------------------------------------------------------

func TestXsfmNodes_Registered(t *testing.T) {
	t.Parallel()
	r := NewRegistry()

	for _, name := range []string{
		"xsfm-status", "xsfm-control", "xsfm",
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

	un, err := r.Create(flow.NewNodeDef("u", "xsfm"))
	require.NoError(t, err)
	_, ok = un.(*XsfmNode)
	assert.True(t, ok, "xsfm 는 *XsfmNode 를 생성해야 한다")

	// `_` 별칭으로도 동일 타입이 나온다.
	snAlias, err := r.Create(flow.NewNodeDef("s2", "xsfm_status"))
	require.NoError(t, err)
	_, ok = snAlias.(*XsfmStatusNode)
	assert.True(t, ok, "xsfm_status 별칭은 *XsfmStatusNode 를 생성해야 한다")
}

// ---------------------------------------------------------------------------
// 그룹/개별 제어 명령셋 — buildXsfmControlCommand 셀렉터 + 명령 추론 (테이블)
// ---------------------------------------------------------------------------

// newXsfmMsg 는 payload/metadata 키를 실은 테스트 메시지를 만든다.
func newXsfmMsg(payload map[string]any, meta map[string]string) message.Message {
	m := message.New()
	for k, v := range payload {
		m.Payload().Set(k, v)
	}
	for k, v := range meta {
		m.Metadata().Set(k, v)
	}
	return m
}

// buildXsfmControlCommand 의 셀렉터(개별 device_id / 그룹 group_id) + 명령 추론을 테이블로 검증한다.
// device_id 는 개별, group_id 는 그룹 일괄 제어의 대상 선정이며, 둘 다 실린 경우 드롭 없이
// 그대로 방출해 에이전트가 우선순위(device_id > group_id)를 적용하게 한다.
func TestBuildXsfmControlCommand(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		payload      map[string]any
		meta         map[string]string
		wantErr      bool
		wantCommand  string
		wantDeviceID string // "" = 명령에 device_id 필드가 없어야 함
		wantGroupID  string // "" = 명령에 group_id 필드가 없어야 함
		wantParams   map[string]any
	}{
		{
			name: "device_id + power → set_power (개별)", payload: map[string]any{"device_id": "01", "power": true},
			wantCommand: "set_power", wantDeviceID: "01", wantParams: map[string]any{"power": true},
		},
		{
			name: "device_id + fan_speed → set_fan_speed (개별)", payload: map[string]any{"device_id": "01", "fan_speed": float64(2)},
			wantCommand: "set_fan_speed", wantDeviceID: "01", wantParams: map[string]any{"fan_speed": float64(2)},
		},
		{
			name: "device_id + power + fan_speed → set_multiple (개별)", payload: map[string]any{"device_id": "01", "power": true, "fan_speed": float64(3)},
			wantCommand: "set_multiple", wantDeviceID: "01",
		},
		{
			name: "group_id + power → set_power (그룹 targeting)", payload: map[string]any{"group_id": "custom:floor2", "power": false},
			wantCommand: "set_power", wantGroupID: "custom:floor2", wantParams: map[string]any{"power": false},
		},
		{
			name: "group_id + fan_speed → set_fan_speed (그룹 targeting)", payload: map[string]any{"group_id": "station:st01", "fan_speed": float64(1)},
			wantCommand: "set_fan_speed", wantGroupID: "station:st01", wantParams: map[string]any{"fan_speed": float64(1)},
		},
		{
			name: "explicit command passthrough", payload: map[string]any{"device_id": "01", "command": "set_multiple", "params": map[string]any{"power": true, "fan_speed": float64(2)}},
			wantCommand: "set_multiple", wantDeviceID: "01",
		},
		{
			name: "group_id metadata fallback", payload: map[string]any{"power": true}, meta: map[string]string{"group_id": "line:ln1"},
			wantCommand: "set_power", wantGroupID: "line:ln1", wantParams: map[string]any{"power": true},
		},
		{
			name: "device_id + group_id 병존 → 둘 다 방출(우선순위는 에이전트)", payload: map[string]any{"device_id": "01", "group_id": "custom:g", "power": true},
			wantCommand: "set_power", wantDeviceID: "01", wantGroupID: "custom:g",
		},
		{
			name: "셀렉터도 제어키도 없음 → 에러", payload: map[string]any{}, wantErr: true,
		},
		{
			name: "group_id 만 있고 제어키/command 없음 → 에러", payload: map[string]any{"group_id": "custom:g"}, wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			msg := newXsfmMsg(tt.payload, tt.meta)
			deviceID := xsfmExtractDeviceID(msg)
			out, err := buildXsfmControlCommand(msg, deviceID, "node-1")
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)

			var cmd map[string]any
			require.NoError(t, json.Unmarshal(out, &cmd))
			assert.Equal(t, tt.wantCommand, cmd["command"], "추론/전달된 command")
			assert.Equal(t, "node-1", cmd["node_id"], "node_id 는 항상 실린다")

			if tt.wantDeviceID != "" {
				assert.Equal(t, tt.wantDeviceID, cmd["device_id"])
			} else {
				_, has := cmd["device_id"]
				assert.False(t, has, "device_id 셀렉터가 명령에 없어야 한다")
			}
			if tt.wantGroupID != "" {
				assert.Equal(t, tt.wantGroupID, cmd["group_id"], "group_id 셀렉터가 최상위에 실려야 한다")
			} else {
				_, has := cmd["group_id"]
				assert.False(t, has, "group_id 셀렉터가 명령에 없어야 한다")
			}
			if tt.wantParams != nil {
				params, ok := cmd["params"].(map[string]any)
				require.True(t, ok, "params 맵이 있어야 한다")
				for k, v := range tt.wantParams {
					assert.Equal(t, v, params[k], "params[%s]", k)
				}
			}
		})
	}
}

// hasXsfmControlCommand 의 라우팅 판정을 테이블로 검증한다: command/params/group_id 셀렉터가
// 있으면 제어 경로, device_id+제어키 상태 스냅샷은 상태 주입 경로(무회귀).
func TestHasXsfmControlCommand_Routing(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		payload map[string]any
		meta    map[string]string
		want    bool
	}{
		{name: "명시적 command → 제어", payload: map[string]any{"command": "set_power"}, want: true},
		{name: "params → 제어", payload: map[string]any{"params": map[string]any{"power": true}}, want: true},
		{name: "group_id 셀렉터 → 제어", payload: map[string]any{"group_id": "custom:g"}, want: true},
		{name: "group_id + power → 제어", payload: map[string]any{"group_id": "custom:g", "power": true}, want: true},
		{name: "group_id metadata → 제어", meta: map[string]string{"group_id": "station:st01"}, want: true},
		{name: "device_id + power + fan_speed 상태 스냅샷 → 상태 주입", payload: map[string]any{"device_id": "01", "power": true, "fan_speed": float64(2)}, want: false},
		{name: "device_id 단독 → 상태 주입", payload: map[string]any{"device_id": "01"}, want: false},
		{name: "빈 payload → 상태 주입", payload: map[string]any{}, want: false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, hasXsfmControlCommand(newXsfmMsg(tt.payload, tt.meta)))
		})
	}
}

// ---------------------------------------------------------------------------
// 그룹 일괄 제어 통합 — 셀렉터 fan-out (노드 → 에이전트 GroupMembers)
// ---------------------------------------------------------------------------

// newPortXSFMAgentGrouped 는 port 모드 에이전트를 만들고 각 디바이스를 동일 group_id(레거시
// primary 그룹 태그)로 등록한다. group_id 셀렉터 fan-out(devicesByPrimaryGroup 경로)을 검증한다.
func newPortXSFMAgentGrouped(t *testing.T, groupID string, deviceIDs ...string) *xsfm.XSFMAgent {
	t.Helper()
	ap := newPortXSFMAgent(t)
	for _, id := range deviceIDs {
		_, err := ap.Process([]byte(`{"command":"add_device","device_id":"` + id + `","group_id":"` + groupID + `"}`))
		require.NoError(t, err)
	}
	return ap
}

// 통합 노드: {group_id, power} (command/params 없음) 입력 → 셀렉터 라우팅으로 제어 경로 →
// 에이전트가 그룹 멤버로 fan-out → 병합 SourceCh 로 멤버별 device_command 방출.
func TestXsfmNode_GroupControlRoutesAndFansOut(t *testing.T) {
	ap := newPortXSFMAgentGrouped(t, "floor2", "g1", "g2")

	def := flow.NewNodeDef("ap", "xsfm")
	node, err := NewXsfmNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	un := node.(*XsfmNode)
	require.NoError(t, un.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, un.Init(context.Background()))
	defer func() { _ = un.Shutdown(context.Background()) }()

	// command/params 없이 group_id + power 만 → 셀렉터 라우팅으로 제어 경로(set_power 추론).
	in := message.New()
	in.Payload().Set("group_id", "floor2")
	in.Payload().Set("power", true)
	resp, err := un.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, resp, 1, "그룹 제어 Process 는 fan-out 집계 응답 1건을 반환한다")

	// fan-out 집계 응답: status=ok (전 멤버 성공, fire-and-forget).
	status, ok := resp[0].Payload().Get("status")
	require.True(t, ok, "fan-out 응답에 status 가 있어야 한다")
	assert.Equal(t, "ok", status)

	// 병합 SourceCh 로 멤버별 device_command 2건이 방출된다.
	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		msg, ok := drainForType(un.SourceCh(), "device_command", 2*time.Second)
		require.True(t, ok, "그룹 fan-out 은 멤버별 device_command 를 방출해야 한다")
		dev, _ := msg.Payload().Get("device_id")
		if s, ok := dev.(string); ok {
			got[s] = true
		}
	}
	assert.True(t, got["g1"] && got["g2"], "그룹 두 멤버(g1,g2) 모두 제어 출력으로 방출되어야 한다: %v", got)
}

// 제어 노드: group_id 셀렉터 set_fan_speed → fan-out → 멤버별 device_command 방출.
func TestXsfmControlNode_GroupControl(t *testing.T) {
	ap := newPortXSFMAgentGrouped(t, "floor3", "g10", "g11")

	def := flow.NewNodeDef("ap-control", "xsfm-control")
	node, err := NewXsfmControlNode(def, WithAgentResolver(wireAPResolver(ap)))
	require.NoError(t, err)
	cn := node.(*XsfmControlNode)
	require.NoError(t, cn.Configure(map[string]any{"agent_ref": "ap-agent-1"}))
	require.NoError(t, cn.Init(context.Background()))
	defer func() { _ = cn.Shutdown(context.Background()) }()

	// group_id + fan_speed → set_fan_speed 추론으로 그룹 일괄 제어.
	in := message.New()
	in.Payload().Set("group_id", "floor3")
	in.Payload().Set("fan_speed", float64(2))
	resp, err := cn.Process(context.Background(), in)
	require.NoError(t, err)
	require.Len(t, resp, 1)
	status, ok := resp[0].Payload().Get("status")
	require.True(t, ok)
	assert.Equal(t, "ok", status)

	got := map[string]bool{}
	for i := 0; i < 2; i++ {
		msg, ok := drainForType(cn.SourceCh(), "device_command", 2*time.Second)
		require.True(t, ok)
		dev, _ := msg.Payload().Get("device_id")
		if s, ok := dev.(string); ok {
			got[s] = true
		}
	}
	assert.True(t, got["g10"] && got["g11"], "그룹 두 멤버 모두 방출되어야 한다: %v", got)
}
