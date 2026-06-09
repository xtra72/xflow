package node

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/message"
)

// metadata_group_emit_test.go 는 P3(에이전트/디바이스 메타데이터 그룹 emit) 의
// TDD 검증을 담는다.
//
// 검증 범위:
//   - MetadataEmitOptions 의 Agent / Device 기본값 ON (parse 시)
//   - SetAgentGroupIfAllowed / SetDeviceGroupIfAllowed 헬퍼
//   - emitAgentGroup 헬퍼 (nil-guard, default ON)
//   - promote 경로의 device 그룹 emit (+ flat device_type/device_id 보존)

// ---------------------------------------------------------------------------
// 1) MetadataEmitOptions: Agent / Device 기본값 및 parse
// ---------------------------------------------------------------------------

// TestDefaultEmitOptions_AgentDeviceOn 는 DefaultEmitOptions() 가 Agent/Device 를
// true 로 반환하는지 검증한다 (P3: 기본 ON 정책).
func TestDefaultEmitOptions_AgentDeviceOn(t *testing.T) {
	o := DefaultEmitOptions()
	if !o.Agent {
		t.Errorf("DefaultEmitOptions().Agent = false; want true (기본 ON)")
	}
	if !o.Device {
		t.Errorf("DefaultEmitOptions().Device = false; want true (기본 ON)")
	}
}

// TestParseEmitMetadata_AgentDeviceDefaultOn 는 config 에 emit 설정이 전혀 없을 때
// Agent/Device 가 true 로 default 되는지 검증한다. (기타 flat 옵션은 default OFF 유지.)
func TestParseEmitMetadata_AgentDeviceDefaultOn(t *testing.T) {
	var out MetadataEmitOptions
	parseEmitMetadata(map[string]any{}, &out)

	if !out.Agent {
		t.Errorf("emit 설정 없을 때 Agent 는 기본 ON 이어야 함")
	}
	if !out.Device {
		t.Errorf("emit 설정 없을 때 Device 는 기본 ON 이어야 함")
	}
	// 기존 옵션은 여전히 default OFF.
	if out.NodeID || out.DeviceType || out.Name || out.NodeSource {
		t.Errorf("기존 옵션(NodeID/DeviceType/Name/NodeSource)은 default OFF 여야 함: %+v", out)
	}
}

// TestParseEmitMetadata_FlatDisableAgentDevice 는 flat 키 emit_agent/emit_device=false
// 가 그룹을 비활성화하는지 검증한다.
func TestParseEmitMetadata_FlatDisableAgentDevice(t *testing.T) {
	var out MetadataEmitOptions
	parseEmitMetadata(map[string]any{
		"emit_agent":  false,
		"emit_device": false,
	}, &out)

	if out.Agent {
		t.Errorf("emit_agent:false 이면 Agent OFF 여야 함")
	}
	if out.Device {
		t.Errorf("emit_device:false 이면 Device OFF 여야 함")
	}
}

// TestParseEmitMetadata_NestedDisableAgentDevice 는 nested emit_metadata.agent/device=false
// 가 그룹을 비활성화하는지 검증한다.
func TestParseEmitMetadata_NestedDisableAgentDevice(t *testing.T) {
	var out MetadataEmitOptions
	parseEmitMetadata(map[string]any{
		"emit_metadata": map[string]any{
			"agent":  false,
			"device": false,
		},
	}, &out)

	if out.Agent {
		t.Errorf("emit_metadata.agent:false 이면 Agent OFF 여야 함")
	}
	if out.Device {
		t.Errorf("emit_metadata.device:false 이면 Device OFF 여야 함")
	}
}

// ---------------------------------------------------------------------------
// 2) SetAgentGroupIfAllowed / SetDeviceGroupIfAllowed
// ---------------------------------------------------------------------------

// TestSetAgentGroupIfAllowed_EmitsGroup 는 Agent=true 일 때 agent 그룹이
// {type, id, name} 으로 설정되는지 검증한다.
func TestSetAgentGroupIfAllowed_EmitsGroup(t *testing.T) {
	msg := message.New()
	o := MetadataEmitOptions{Agent: true}
	o.SetAgentGroupIfAllowed(msg.Metadata().SetGroup, "lg-lg_hvacr02", "agent-123", "Living Room Agent")

	g, ok := msg.Metadata().GetGroup("agent")
	if !ok {
		t.Fatalf("agent 그룹이 설정되어야 함")
	}
	if g["type"] != "lg-lg_hvacr02" {
		t.Errorf("agent.type = %q; want %q", g["type"], "lg-lg_hvacr02")
	}
	if g["id"] != "agent-123" {
		t.Errorf("agent.id = %q; want %q", g["id"], "agent-123")
	}
	if g["name"] != "Living Room Agent" {
		t.Errorf("agent.name = %q; want %q", g["name"], "Living Room Agent")
	}
}

// TestSetAgentGroupIfAllowed_OmitEmptyName 는 name 이 빈 문자열이면 name 필드를
// 포함하지 않는지 검증한다.
func TestSetAgentGroupIfAllowed_OmitEmptyName(t *testing.T) {
	msg := message.New()
	o := MetadataEmitOptions{Agent: true}
	o.SetAgentGroupIfAllowed(msg.Metadata().SetGroup, "modbus", "agent-9", "")

	g, ok := msg.Metadata().GetGroup("agent")
	if !ok {
		t.Fatalf("agent 그룹이 설정되어야 함")
	}
	if _, has := g["name"]; has {
		t.Errorf("name 이 비어있으면 name 필드를 포함하지 않아야 함: %+v", g)
	}
	if g["type"] != "modbus" || g["id"] != "agent-9" {
		t.Errorf("type/id 는 설정되어야 함: %+v", g)
	}
}

// TestSetAgentGroupIfAllowed_DisabledNoEmit 는 Agent=false 면 그룹을 emit 하지 않는지 검증한다.
func TestSetAgentGroupIfAllowed_DisabledNoEmit(t *testing.T) {
	msg := message.New()
	o := MetadataEmitOptions{Agent: false}
	o.SetAgentGroupIfAllowed(msg.Metadata().SetGroup, "modbus", "agent-9", "n")

	if _, ok := msg.Metadata().GetGroup("agent"); ok {
		t.Errorf("Agent=false 이면 agent 그룹을 emit 하지 않아야 함")
	}
}

// TestSetAgentGroupIfAllowed_SkipWhenTypeAndIDEmpty 는 type, id 둘 다 비어있으면
// 그룹을 만들지 않는지 검증한다.
func TestSetAgentGroupIfAllowed_SkipWhenTypeAndIDEmpty(t *testing.T) {
	msg := message.New()
	o := MetadataEmitOptions{Agent: true}
	o.SetAgentGroupIfAllowed(msg.Metadata().SetGroup, "", "", "")

	if _, ok := msg.Metadata().GetGroup("agent"); ok {
		t.Errorf("type/id 모두 비어있으면 agent 그룹을 만들지 않아야 함")
	}
}

// TestSetDeviceGroupIfAllowed_EmitsGroup 는 Device=true 일 때 device 그룹이
// {type, id} 로 설정되는지 검증한다.
func TestSetDeviceGroupIfAllowed_EmitsGroup(t *testing.T) {
	msg := message.New()
	o := MetadataEmitOptions{Device: true}
	o.SetDeviceGroupIfAllowed(msg.Metadata().SetGroup, "HVACR.IDU", "uuid-abc")

	g, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatalf("device 그룹이 설정되어야 함")
	}
	if g["type"] != "HVACR.IDU" {
		t.Errorf("device.type = %q; want %q", g["type"], "HVACR.IDU")
	}
	if g["id"] != "uuid-abc" {
		t.Errorf("device.id = %q; want %q", g["id"], "uuid-abc")
	}
}

// TestSetDeviceGroupIfAllowed_DisabledNoEmit 는 Device=false 면 그룹을 emit 하지 않는지 검증한다.
func TestSetDeviceGroupIfAllowed_DisabledNoEmit(t *testing.T) {
	msg := message.New()
	o := MetadataEmitOptions{Device: false}
	o.SetDeviceGroupIfAllowed(msg.Metadata().SetGroup, "HVACR.IDU", "uuid-abc")

	if _, ok := msg.Metadata().GetGroup("device"); ok {
		t.Errorf("Device=false 이면 device 그룹을 emit 하지 않아야 함")
	}
}

// TestSetDeviceGroupIfAllowed_SkipWhenBothEmpty 는 type, id 둘 다 비어있으면
// 그룹을 만들지 않는지 검증한다.
func TestSetDeviceGroupIfAllowed_SkipWhenBothEmpty(t *testing.T) {
	msg := message.New()
	o := MetadataEmitOptions{Device: true}
	o.SetDeviceGroupIfAllowed(msg.Metadata().SetGroup, "", "")

	if _, ok := msg.Metadata().GetGroup("device"); ok {
		t.Errorf("type/id 모두 비어있으면 device 그룹을 만들지 않아야 함")
	}
}

// ---------------------------------------------------------------------------
// 3) emitAgentGroup 헬퍼 (nil-guard)
// ---------------------------------------------------------------------------

// TestEmitAgentGroup_DefaultOn 는 emitAgentGroup 이 기본 ON 옵션에서 agent 그룹을
// {type, id} 로 emit 하는지 검증한다 (config 없는 노드 기본 동작).
func TestEmitAgentGroup_DefaultOn(t *testing.T) {
	msg := message.New()
	a := &mockLGHvacr02Agent{}
	emitAgentGroup(msg, a, DefaultEmitOptions())

	g, ok := msg.Metadata().GetGroup("agent")
	if !ok {
		t.Fatalf("agent 그룹이 emit 되어야 함")
	}
	if g["type"] != a.Type() {
		t.Errorf("agent.type = %q; want %q", g["type"], a.Type())
	}
	if g["id"] != a.ID() {
		t.Errorf("agent.id = %q; want %q", g["id"], a.ID())
	}
}

// TestEmitAgentGroup_NilAgentNoOp 는 agent 가 nil 이면 no-op 인지 검증한다.
func TestEmitAgentGroup_NilAgentNoOp(t *testing.T) {
	msg := message.New()
	emitAgentGroup(msg, nil, DefaultEmitOptions())

	if _, ok := msg.Metadata().GetGroup("agent"); ok {
		t.Errorf("agent 가 nil 이면 agent 그룹을 emit 하지 않아야 함")
	}
}

// TestEmitAgentGroup_DisabledNoOp 는 Agent=false 옵션에서 no-op 인지 검증한다.
func TestEmitAgentGroup_DisabledNoOp(t *testing.T) {
	msg := message.New()
	a := &mockLGHvacr02Agent{}
	emitAgentGroup(msg, a, MetadataEmitOptions{Agent: false})

	if _, ok := msg.Metadata().GetGroup("agent"); ok {
		t.Errorf("Agent=false 이면 agent 그룹을 emit 하지 않아야 함")
	}
}

// ---------------------------------------------------------------------------
// 4) promote 경로의 device 그룹 emit (+ flat 보존)
// ---------------------------------------------------------------------------

// TestPromoteDevIDWithUUID_EmitsDeviceGroup 는 promoteDevIDWithUUID 가 device 그룹
// {id} 를 emit 하면서 동시에 flat device_id 를 보존하는지 검증한다.
//
// 보존 근거: MQTT 토픽 템플릿 ($.metadata.device_id) 이 flat 키를 소비한다.
func TestPromoteDevIDWithUUID_EmitsDeviceGroup(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"device_id": "uuid-xyz",
	}
	// agentName="" 이면 registry 조회를 건너뛰고 payload.device_id 만 promote.
	opts := MetadataEmitOptions{Device: true}
	promoteDevIDWithUUID(msg, payload, "", opts)

	// device 그룹에 id 가 들어가야 함.
	g, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatalf("device 그룹이 emit 되어야 함")
	}
	if g["id"] != "uuid-xyz" {
		t.Errorf("device.id = %q; want %q", g["id"], "uuid-xyz")
	}

	// flat device_id 도 보존되어야 함 (MQTT 템플릿 소비자).
	if v, ok := msg.Metadata().Get("device_id"); !ok || v != "uuid-xyz" {
		t.Errorf("flat device_id 가 보존되어야 함 (MQTT 템플릿 호환): got %q, ok=%v", v, ok)
	}

	// payload 에서 device_id 는 제거되어야 함.
	if _, exists := payload["device_id"]; exists {
		t.Errorf("payload.device_id 는 promote 후 제거되어야 함")
	}
}

// TestPromoteDevIDWithUUID_DeviceDisabledNoGroup 는 Device=false 면 device 그룹을
// emit 하지 않지만 flat device_id 는 여전히 보존되는지 검증한다.
func TestPromoteDevIDWithUUID_DeviceDisabledNoGroup(t *testing.T) {
	msg := message.New()
	payload := map[string]any{"device_id": "uuid-xyz"}
	promoteDevIDWithUUID(msg, payload, "", MetadataEmitOptions{Device: false})

	if _, ok := msg.Metadata().GetGroup("device"); ok {
		t.Errorf("Device=false 이면 device 그룹을 emit 하지 않아야 함")
	}
	// flat device_id 는 여전히 보존 (기존 동작 + MQTT 호환).
	if v, ok := msg.Metadata().Get("device_id"); !ok || v != "uuid-xyz" {
		t.Errorf("flat device_id 는 보존되어야 함: got %q, ok=%v", v, ok)
	}
}

// TestPromotePayloadMetadata_DeviceTypeToGroupAndFlat 는 payload.metadata.device_type
// 가 device 그룹의 type 으로도 들어가면서 flat device_type 도 보존되는지 검증한다.
func TestPromotePayloadMetadata_DeviceTypeToGroupAndFlat(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"metadata": map[string]any{
			"device_type": "HVACR.IDU",
		},
	}
	// DeviceType=true 로 flat device_type 유지, Device=true 로 그룹 emit.
	promotePayloadMetadata(msg, payload, MetadataEmitOptions{DeviceType: true, Device: true})

	// flat device_type 보존 (MQTT 템플릿 $.metadata.device_type 소비자).
	if v, ok := msg.Metadata().Get("device_type"); !ok || v != "HVACR.IDU" {
		t.Errorf("flat device_type 보존되어야 함: got %q, ok=%v", v, ok)
	}
	// device 그룹의 type 으로도 들어가야 함.
	g, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatalf("device 그룹이 emit 되어야 함")
	}
	if g["type"] != "HVACR.IDU" {
		t.Errorf("device.type = %q; want %q", g["type"], "HVACR.IDU")
	}
}

// TestPromote_DeviceGroupMergeTypeAndID 는 promotePayloadMetadata(type) 후
// promoteDevIDWithUUID(id) 가 device 그룹을 {type, id} 로 병합(clobber 없이)하는지 검증한다.
func TestPromote_DeviceGroupMergeTypeAndID(t *testing.T) {
	msg := message.New()
	payload := map[string]any{
		"device_id": "uuid-merged",
		"metadata": map[string]any{
			"device_type": "HVACR.ODU",
		},
	}
	opts := MetadataEmitOptions{Device: true}
	promotePayloadMetadata(msg, payload, opts)
	promoteDevIDWithUUID(msg, payload, "", opts)

	g, ok := msg.Metadata().GetGroup("device")
	if !ok {
		t.Fatalf("device 그룹이 emit 되어야 함")
	}
	if g["type"] != "HVACR.ODU" {
		t.Errorf("device.type = %q; want %q (병합 시 type 보존)", g["type"], "HVACR.ODU")
	}
	if g["id"] != "uuid-merged" {
		t.Errorf("device.id = %q; want %q (병합 시 id 추가)", g["id"], "uuid-merged")
	}
}

// ---------------------------------------------------------------------------
// 5) 노드 end-to-end: LGHvacr02 (디바이스 노드) device + agent 그룹
// ---------------------------------------------------------------------------

// runLGHvacr02StatusProcess 는 device_state 응답을 주입해 Process 결과 메시지를 반환하는 헬퍼다.
func runLGHvacr02StatusProcess(t *testing.T, agentResp map[string]any, opts MetadataEmitOptions) message.Message {
	t.Helper()
	respBytes, err := json.Marshal(agentResp)
	require.NoError(t, err)

	mockAgent := &mockLGHvacr02Agent{processResp: respBytes}
	n := newTestLGHvacr02StatusNode(mockAgent)
	n.mu.Lock()
	n.lgHvacr02Cfg = LGHvacr02NodeConfig{
		AgentRef:     "test-agent",
		PollCommand:  lgHvacr02CmdGetStats,
		RecentCount:  10,
		EmitMetadata: opts,
	}
	n.mu.Unlock()

	results, err := n.Process(context.Background(), message.New())
	require.NoError(t, err)
	require.Len(t, results, 1)
	return results[0]
}

// TestLGHvacr02Node_DeviceStateEmitsDeviceAndAgentGroups 는 device-state 메시지가
// device:{type,id} 그룹과 agent:{type,id} 그룹을 모두 emit 하는지 검증한다 (기본 ON).
func TestLGHvacr02Node_DeviceStateEmitsDeviceAndAgentGroups(t *testing.T) {
	out := runLGHvacr02StatusProcess(t, map[string]any{
		"device_id": "uuid-idu-1",
		"power":     "on",
		"metadata": map[string]any{
			"device_type": "HVACR.IDU",
		},
	}, DefaultEmitOptions())

	// device 그룹: {type, id}
	dg, ok := out.Metadata().GetGroup("device")
	if !ok {
		t.Fatalf("device 그룹이 emit 되어야 함; metadata=%v", out.Metadata().Raw())
	}
	if dg["type"] != "HVACR.IDU" {
		t.Errorf("device.type = %q; want %q", dg["type"], "HVACR.IDU")
	}
	if dg["id"] != "uuid-idu-1" {
		t.Errorf("device.id = %q; want %q", dg["id"], "uuid-idu-1")
	}

	// agent 그룹: {type, id} (mockLGHvacr02Agent)
	ag, ok := out.Metadata().GetGroup("agent")
	if !ok {
		t.Fatalf("agent 그룹이 emit 되어야 함")
	}
	if ag["type"] != "lg-lg_hvacr02" {
		t.Errorf("agent.type = %q; want %q", ag["type"], "lg-lg_hvacr02")
	}
	if ag["id"] != "mock-lg_hvacr02" {
		t.Errorf("agent.id = %q; want %q", ag["id"], "mock-lg_hvacr02")
	}

	// flat device_id 보존 (MQTT 템플릿 소비자).
	if v, ok := out.Metadata().Get("device_id"); !ok || v != "uuid-idu-1" {
		t.Errorf("flat device_id 보존되어야 함: got %q, ok=%v", v, ok)
	}
}

// TestLGHvacr02Node_EmitAgentFalseDisablesAgentGroup 는 emit_agent:false 가 agent 그룹을
// 비활성화하지만 device 그룹은 유지하는지 검증한다.
func TestLGHvacr02Node_EmitAgentFalseDisablesAgentGroup(t *testing.T) {
	opts := DefaultEmitOptions()
	opts.Agent = false // emit_agent:false 동등
	out := runLGHvacr02StatusProcess(t, map[string]any{
		"device_id": "uuid-idu-2",
		"metadata":  map[string]any{"device_type": "HVACR.IDU"},
	}, opts)

	if _, ok := out.Metadata().GetGroup("agent"); ok {
		t.Errorf("emit_agent:false 이면 agent 그룹을 emit 하지 않아야 함")
	}
	if _, ok := out.Metadata().GetGroup("device"); !ok {
		t.Errorf("device 그룹은 여전히 emit 되어야 함")
	}
}

// TestLGHvacr02Node_EmitDeviceFalseDisablesDeviceGroup 는 emit_device:false 가 device 그룹을
// 비활성화하지만 agent 그룹은 유지하는지 검증한다. flat device_id 는 여전히 보존.
func TestLGHvacr02Node_EmitDeviceFalseDisablesDeviceGroup(t *testing.T) {
	opts := DefaultEmitOptions()
	opts.Device = false // emit_device:false 동등
	out := runLGHvacr02StatusProcess(t, map[string]any{
		"device_id": "uuid-idu-3",
		"metadata":  map[string]any{"device_type": "HVACR.IDU"},
	}, opts)

	if _, ok := out.Metadata().GetGroup("device"); ok {
		t.Errorf("emit_device:false 이면 device 그룹을 emit 하지 않아야 함")
	}
	if _, ok := out.Metadata().GetGroup("agent"); !ok {
		t.Errorf("agent 그룹은 여전히 emit 되어야 함")
	}
	// flat device_id 는 보존.
	if v, ok := out.Metadata().Get("device_id"); !ok || v != "uuid-idu-3" {
		t.Errorf("flat device_id 보존되어야 함: got %q, ok=%v", v, ok)
	}
}

// TestLGHvacr02Node_NodeIDFlatWhenToggled 는 NodeID 토글이 ON 이면 node_id 가 flat 으로
// 유지되는지 검증한다 (그룹화 대상 아님).
func TestLGHvacr02Node_NodeIDFlatWhenToggled(t *testing.T) {
	opts := DefaultEmitOptions()
	opts.NodeID = true
	out := runLGHvacr02StatusProcess(t, map[string]any{
		"device_id": "uuid-idu-4",
	}, opts)

	if _, ok := out.Metadata().Get("node_id"); !ok {
		t.Errorf("NodeID 토글 ON 이면 node_id 가 flat 으로 emit 되어야 함")
	}
}
