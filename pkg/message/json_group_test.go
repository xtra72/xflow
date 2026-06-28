package message

import (
	"encoding/json"
	"testing"
)

// TestJSON_MarshalNestedGroups 는 MarshalJSON이 group을 중첩 객체로, flat 키를 문자열로
// 직렬화함을 검증한다.
func TestJSON_MarshalNestedGroups(t *testing.T) {
	msg := New(
		WithMetadata("node_source", "poll"),
	)
	msg.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal 실패: %v", err)
	}

	// metadata 부분만 추출하여 정확한 형태를 검증한다
	var decoded struct {
		Metadata map[string]json.RawMessage `json:"metadata"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("디코드 실패: %v", err)
	}

	// flat 키는 문자열로 직렬화
	var src string
	if err := json.Unmarshal(decoded.Metadata["node_source"], &src); err != nil {
		t.Fatalf("node_source 디코드 실패: %v", err)
	}
	if src != "poll" {
		t.Errorf("node_source = %q, 기대값 \"poll\"", src)
	}

	// group 키는 중첩 객체로 직렬화
	var agent map[string]string
	if err := json.Unmarshal(decoded.Metadata["agent"], &agent); err != nil {
		t.Fatalf("agent 디코드 실패 (중첩 객체가 아님): %v", err)
	}
	if agent["type"] != "serial" || agent["id"] != "node-1" {
		t.Errorf("agent 그룹 불일치: %v", agent)
	}
}

// TestJSON_MarshalExactShape 는 정확한 JSON 형태를 검증한다.
func TestJSON_MarshalExactShape(t *testing.T) {
	msg := New(
		WithMetadata("node_source", "poll"),
	)
	msg.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal 실패: %v", err)
	}

	// metadata 필드를 다시 정규화하여 정확한 키/값을 비교한다
	var top map[string]json.RawMessage
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("top 디코드 실패: %v", err)
	}

	var meta map[string]any
	if err := json.Unmarshal(top["metadata"], &meta); err != nil {
		t.Fatalf("metadata 디코드 실패: %v", err)
	}

	// agent는 객체, node_source는 문자열
	agent, ok := meta["agent"].(map[string]any)
	if !ok {
		t.Fatalf("agent가 객체가 아니다: %T", meta["agent"])
	}
	if agent["type"] != "serial" || agent["id"] != "node-1" {
		t.Errorf("agent = %v", agent)
	}
	if meta["node_source"] != "poll" {
		t.Errorf("node_source = %v, 기대값 \"poll\"", meta["node_source"])
	}
}

// TestJSON_FromJSONRestoresGroups 는 FromJSON이 group을 group으로, string을 string으로
// 복원함을 검증한다.
func TestJSON_FromJSONRestoresGroups(t *testing.T) {
	input := `{
		"id": "msg-1",
		"timestamp": "2026-01-01T00:00:00Z",
		"payload": {},
		"metadata": {
			"agent": {"type": "serial", "id": "node-1"},
			"device": {"type": "HVACR.IDU", "id": "dev-9"},
			"node_source": "poll"
		},
		"history_enabled": false,
		"history": null
	}`

	msg, err := FromJSON([]byte(input))
	if err != nil {
		t.Fatalf("FromJSON 실패: %v", err)
	}

	// string 키는 Get으로 조회 가능
	if v, ok := msg.Metadata().Get("node_source"); !ok || v != "poll" {
		t.Errorf("node_source Get = (%q, %v)", v, ok)
	}

	// group 키는 GetGroup으로 조회 가능
	agent, ok := msg.Metadata().GetGroup("agent")
	if !ok || agent["type"] != "serial" || agent["id"] != "node-1" {
		t.Errorf("agent GetGroup = (%v, %v)", agent, ok)
	}
	device, ok := msg.Metadata().GetGroup("device")
	if !ok || device["type"] != "HVACR.IDU" || device["id"] != "dev-9" {
		t.Errorf("device GetGroup = (%v, %v)", device, ok)
	}

	// group 키는 Get(string)으로는 조회 불가
	if _, ok := msg.Metadata().Get("agent"); ok {
		t.Error("group 키 'agent'가 Get(string)으로 조회되었다")
	}
}

// TestJSON_RoundTripGroups 는 Marshal -> FromJSON 왕복에서 group과 string이
// 정확히 보존됨을 검증한다.
func TestJSON_RoundTripGroups(t *testing.T) {
	orig := New(
		WithMetadata("node_source", "poll"),
	)
	orig.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})
	orig.Metadata().SetGroup("device", map[string]string{"type": "HVACR.IDU", "id": "dev-9"})

	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatalf("Marshal 실패: %v", err)
	}

	restored, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON 실패: %v", err)
	}

	if v, ok := restored.Metadata().Get("node_source"); !ok || v != "poll" {
		t.Errorf("왕복 후 node_source = (%q, %v)", v, ok)
	}
	agent, ok := restored.Metadata().GetGroup("agent")
	if !ok || agent["type"] != "serial" || agent["id"] != "node-1" {
		t.Errorf("왕복 후 agent = (%v, %v)", agent, ok)
	}
	device, ok := restored.Metadata().GetGroup("device")
	if !ok || device["type"] != "HVACR.IDU" || device["id"] != "dev-9" {
		t.Errorf("왕복 후 device = (%v, %v)", device, ok)
	}
}

// TestJSON_FromJSONNonStringGroupFields 는 group 내 비문자열 필드를 안전하게
// 문자열로 강제 변환함을 검증한다.
func TestJSON_FromJSONNonStringGroupFields(t *testing.T) {
	input := `{
		"id": "msg-1",
		"timestamp": "2026-01-01T00:00:00Z",
		"payload": {},
		"metadata": {
			"mixed": {"type": "x", "count": 5, "active": true}
		},
		"history_enabled": false,
		"history": null
	}`

	msg, err := FromJSON([]byte(input))
	if err != nil {
		t.Fatalf("FromJSON 실패: %v", err)
	}

	g, ok := msg.Metadata().GetGroup("mixed")
	if !ok {
		t.Fatal("mixed group을 복원하지 못했다")
	}
	if g["type"] != "x" {
		t.Errorf("mixed.type = %q, 기대값 \"x\"", g["type"])
	}
	// 비문자열 필드는 문자열 표현으로 강제 변환된다 (데이터 손실 방지)
	if g["count"] != "5" {
		t.Errorf("mixed.count = %q, 기대값 \"5\"", g["count"])
	}
	if g["active"] != "true" {
		t.Errorf("mixed.active = %q, 기대값 \"true\"", g["active"])
	}
}

// TestJSON_FromJSONNonObjectNonStringIsString 는 array/number 등 group도 string도 아닌
// 값을 문자열 표현으로 저장함을 검증한다 (데이터 손실 방지).
func TestJSON_FromJSONNonObjectNonStringIsString(t *testing.T) {
	input := `{
		"id": "msg-1",
		"timestamp": "2026-01-01T00:00:00Z",
		"payload": {},
		"metadata": {
			"num": 42,
			"flag": true
		},
		"history_enabled": false,
		"history": null
	}`

	msg, err := FromJSON([]byte(input))
	if err != nil {
		t.Fatalf("FromJSON 실패: %v", err)
	}

	// 숫자/불리언은 string으로 저장되어 Get으로 조회 가능
	if v, ok := msg.Metadata().Get("num"); !ok || v != "42" {
		t.Errorf("num Get = (%q, %v), 기대값 (\"42\", true)", v, ok)
	}
	if v, ok := msg.Metadata().Get("flag"); !ok || v != "true" {
		t.Errorf("flag Get = (%q, %v), 기대값 (\"true\", true)", v, ok)
	}
}

// TestJSON_HistoryEnabledSupportsGroups 는 이력 활성화 메시지가 SetGroup/GetGroup을
// 지원함을 검증한다.
func TestJSON_HistoryEnabledSupportsGroups(t *testing.T) {
	msg := New(WithHistory(true))
	msg.Metadata().SetGroup("agent", map[string]string{"type": "serial", "id": "node-1"})

	g, ok := msg.Metadata().GetGroup("agent")
	if !ok || g["type"] != "serial" || g["id"] != "node-1" {
		t.Errorf("history-enabled GetGroup = (%v, %v)", g, ok)
	}

	// Raw도 group을 포함해야 한다
	raw := msg.Metadata().Raw()
	if _, ok := raw["agent"].(map[string]string); !ok {
		t.Errorf("history-enabled Raw()[agent] 타입 = %T, 기대값 map[string]string", raw["agent"])
	}
}
