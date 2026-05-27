package flow

import (
	"encoding/json"
	"testing"
)

// TestNodeDef_UnmarshalJSON_FlatFieldsCollectedToConfig 는 NodeDef 표준 필드 외의
// flat 필드 (예: condition, on_reject) 가 자동으로 Config 맵에 수집되는지 검증한다.
//
// 결함 시나리오: flattenNodeData export 가 만든 JSON 또는 외부 도구의 flat 노드
// 표현을 import 할 때 NodeDef.Config 가 비어있던 문제.
//
// SPEC: flow import data-loss hotfix (2026-05-13)
func TestNodeDef_UnmarshalJSON_FlatFieldsCollectedToConfig(t *testing.T) {
	data := []byte(`{
		"id": "n1",
		"name": "node1",
		"type": "filter",
		"condition": "$.x == 1",
		"on_reject": "reject_port"
	}`)

	var n NodeDef
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("flat 필드 포함 NodeDef 역직렬화 실패: %v", err)
	}

	if n.ID != "n1" || n.Name != "node1" || n.Type != "filter" {
		t.Errorf("표준 필드 손실: %+v", n)
	}
	if n.Config == nil {
		t.Fatal("Config 가 nil — flat 필드가 수집되지 않음")
	}
	if got, want := n.Config["condition"], "$.x == 1"; got != want {
		t.Errorf("Config[condition] 불일치: 기대 %q, 실제 %v", want, got)
	}
	if got, want := n.Config["on_reject"], "reject_port"; got != want {
		t.Errorf("Config[on_reject] 불일치: 기대 %q, 실제 %v", want, got)
	}
}

// TestNodeDef_UnmarshalJSON_ExplicitConfigWinsOverFlat 는 노드에 명시적 config 와
// 동일 키의 flat 필드가 동시에 존재할 때, 명시 config 값이 우선하는지 검증한다.
//
// 정책 근거: 사용자가 의도적으로 config 객체를 작성한 경우, flat 표현은 legacy
// fallback 이므로 override 대상이 될 수 없다.
func TestNodeDef_UnmarshalJSON_ExplicitConfigWinsOverFlat(t *testing.T) {
	data := []byte(`{
		"id": "n1",
		"type": "t",
		"condition": "flat",
		"config": {"condition": "explicit"}
	}`)

	var n NodeDef
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("역직렬화 실패: %v", err)
	}
	if got, want := n.Config["condition"], "explicit"; got != want {
		t.Errorf("명시 config 가 flat 값에 의해 덮어쓰여짐: 기대 %q, 실제 %v", want, got)
	}
}

// TestNodeDef_UnmarshalJSON_StandardFieldsPreserved 는 canonical iot-sensor.json
// 형식의 노드 (표준 nested config + agent_ref + 포트 shorthand) 가 손실 없이
// 역직렬화되는지 검증한다.
//
// 회귀 방지: Y 가 표준 형식의 round-trip 을 깨지 않음을 보장한다.
func TestNodeDef_UnmarshalJSON_StandardFieldsPreserved(t *testing.T) {
	data := []byte(`{
		"id": "node-mqtt-in",
		"name": "mqtt-subscriber",
		"type": "bridge",
		"config": {"topic": "sensors/temperature/#"},
		"inputs": [{"name": "in"}],
		"outputs": [{"name": "out"}],
		"agent_ref": {
			"agent_id": "agent-mqtt",
			"agent_name": "mqtt-broker",
			"direction": "in"
		},
		"metadata": {"x": "50", "y": "200"}
	}`)

	var n NodeDef
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("canonical 형식 역직렬화 실패: %v", err)
	}

	if n.ID != "node-mqtt-in" || n.Name != "mqtt-subscriber" || n.Type != "bridge" {
		t.Errorf("표준 필드 손실: %+v", n)
	}
	if got, want := n.Config["topic"], "sensors/temperature/#"; got != want {
		t.Errorf("Config[topic] 불일치: 기대 %q, 실제 %v", want, got)
	}
	if n.AgentRef == nil || n.AgentRef.AgentID != "agent-mqtt" || n.AgentRef.AgentName != "mqtt-broker" {
		t.Errorf("AgentRef 손실 또는 불일치: %+v", n.AgentRef)
	}
	if n.AgentRef.Direction != BridgeIn {
		t.Errorf("AgentRef.Direction 불일치: 기대 %q, 실제 %q", BridgeIn, n.AgentRef.Direction)
	}
	if len(n.Inputs) != 1 || n.Inputs[0].Name != "in" {
		t.Errorf("Inputs 손실: %+v", n.Inputs)
	}
	if len(n.Outputs) != 1 || n.Outputs[0].Name != "out" {
		t.Errorf("Outputs 손실: %+v", n.Outputs)
	}
	if n.Metadata["x"] != "50" {
		t.Errorf("Metadata 손실: %+v", n.Metadata)
	}
}

// TestNodeDef_RoundTrip_CapturedUserJSON 는 사용자가 보고한 실제 결함 시나리오 —
// flat 필드를 가진 broken export JSON 의 round-trip 일관성을 검증한다.
//
// 입력: filter 노드의 condition/on_reject 가 flat 으로 노출된 노드.
// 기대: Unmarshal 결과 Config 에 condition/on_reject 가 포함되어야 한다.
// 추가: Marshal → Unmarshal 후에도 데이터가 유지되어야 한다.
func TestNodeDef_RoundTrip_CapturedUserJSON(t *testing.T) {
	rawFlat := []byte(`{
		"id": "lg_hvacr01_status-1776148011929",
		"name": "lg_hvacr01_status",
		"type": "lg_hvacr01_status",
		"category": "io",
		"poll_command": "drain",
		"agent_ref": {
			"agent_id": "agent-lg_hvacr01",
			"agent_name": "lg_hvacr01-broker"
		},
		"layout": {"type": "custom"}
	}`)

	var n NodeDef
	if err := json.Unmarshal(rawFlat, &n); err != nil {
		t.Fatalf("flat broken export 형식 역직렬화 실패: %v", err)
	}
	if n.Config == nil {
		t.Fatal("Config 가 nil — flat 필드가 수집되지 않음")
	}
	if got, want := n.Config["category"], "io"; got != want {
		t.Errorf("Config[category] 불일치: 기대 %q, 실제 %v", want, got)
	}
	if got, want := n.Config["poll_command"], "drain"; got != want {
		t.Errorf("Config[poll_command] 불일치: 기대 %q, 실제 %v", want, got)
	}
	if _, hasLayout := n.Config["layout"]; hasLayout {
		t.Error("layout 키는 Config 로 수집되어서는 안 됨 (렌더링 전용 메타)")
	}
	if n.AgentRef == nil || n.AgentRef.AgentName != "lg_hvacr01-broker" {
		t.Errorf("AgentRef 손실: %+v", n.AgentRef)
	}

	// 2nd round-trip: Marshal 후 다시 Unmarshal 해도 데이터 보존
	encoded, err := json.Marshal(&n)
	if err != nil {
		t.Fatalf("Marshal 실패: %v", err)
	}
	var n2 NodeDef
	if err := json.Unmarshal(encoded, &n2); err != nil {
		t.Fatalf("재 Unmarshal 실패: %v", err)
	}
	if got, want := n2.Config["category"], "io"; got != want {
		t.Errorf("재 Marshal/Unmarshal 후 Config[category] 손실: %v", got)
	}
	if got, want := n2.Config["poll_command"], "drain"; got != want {
		t.Errorf("재 Marshal/Unmarshal 후 Config[poll_command] 손실: %v", got)
	}
}

// TestNodeDef_UnmarshalJSON_AgentRefStringFallback_Integration 는 70d5c3e 가
// 도입한 AgentRef bare-string 호환 unmarshal 이 Y 의 unknown-field 수집과 함께
// 동시에 동작하는지 통합 검증한다.
//
// 결합 시나리오: 노드에 flat config-style 필드 (condition) 와 bare string
// agent_ref 가 동시에 존재한다.
func TestNodeDef_UnmarshalJSON_AgentRefStringFallback_Integration(t *testing.T) {
	data := []byte(`{
		"id": "node-x",
		"name": "x-node",
		"type": "bridge",
		"condition": "$.ok",
		"agent_ref": "legacy-agent-name"
	}`)

	var n NodeDef
	if err := json.Unmarshal(data, &n); err != nil {
		t.Fatalf("flat + bare-string agent_ref 결합 역직렬화 실패: %v", err)
	}

	if n.AgentRef == nil || n.AgentRef.AgentName != "legacy-agent-name" {
		t.Errorf("AgentRef bare-string fallback 회귀: %+v", n.AgentRef)
	}
	if n.Config == nil || n.Config["condition"] != "$.ok" {
		t.Errorf("flat condition 이 Config 로 수집되지 않음: %+v", n.Config)
	}
}
