package flow

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestAgentRef_UnmarshalJSON_StandardObject 는 표준 nested 객체 형식이
// 모든 필드를 손실 없이 역직렬화하는지 검증한다.
//
// SPEC: flow round-trip 결함 hotfix (2026-05-13)
func TestAgentRef_UnmarshalJSON_StandardObject(t *testing.T) {
	data := []byte(`{"agent_id":"a","agent_name":"b","direction":"in"}`)

	var ref AgentRef
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("표준 객체 역직렬화 실패: %v", err)
	}

	if ref.AgentID != "a" {
		t.Errorf("AgentID 불일치: 기대 %q, 실제 %q", "a", ref.AgentID)
	}
	if ref.AgentName != "b" {
		t.Errorf("AgentName 불일치: 기대 %q, 실제 %q", "b", ref.AgentName)
	}
	if ref.Direction != BridgeIn {
		t.Errorf("Direction 불일치: 기대 %q, 실제 %q", BridgeIn, ref.Direction)
	}
}

// TestAgentRef_UnmarshalJSON_FallbackString 는 호환 형식 (bare string) 이
// AgentName 으로 매핑되고 AgentID/Direction 은 zero 값으로 유지되는지 검증한다.
//
// 이 경로는 client DynamicForm 또는 외부 도구가 agent_ref:"my-agent" 형태로
// 보낼 때 round-trip 결함을 방지한다.
func TestAgentRef_UnmarshalJSON_FallbackString(t *testing.T) {
	data := []byte(`"my-agent"`)

	var ref AgentRef
	if err := json.Unmarshal(data, &ref); err != nil {
		t.Fatalf("bare string 역직렬화 실패 (호환 형식이어야 함): %v", err)
	}

	if ref.AgentName != "my-agent" {
		t.Errorf("AgentName 불일치: 기대 %q, 실제 %q", "my-agent", ref.AgentName)
	}
	if ref.AgentID != "" {
		t.Errorf("AgentID 는 비어있어야 함: 실제 %q", ref.AgentID)
	}
	if ref.Direction != "" {
		t.Errorf("Direction 은 비어있어야 함: 실제 %q", ref.Direction)
	}
}

// TestAgentRef_RoundTrip 는 표준 객체를 marshal → unmarshal 했을 때
// 데이터 손실이 없음을 검증한다 (정상 흐름에서의 호환성 회귀 방지).
func TestAgentRef_RoundTrip(t *testing.T) {
	original := AgentRef{
		AgentID:   "i",
		AgentName: "n",
		Direction: BridgeIn,
	}

	encoded, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal 실패: %v", err)
	}

	var decoded AgentRef
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("Unmarshal 실패: %v", err)
	}

	if decoded != original {
		t.Errorf("round-trip 후 데이터 손실: 기대 %+v, 실제 %+v", original, decoded)
	}
}

// TestAgentRef_UnmarshalJSON_InvalidShape 는 지원하지 않는 JSON 형식
// (array, number 등) 에 대해 명시적 에러를 반환하는지 검증한다.
func TestAgentRef_UnmarshalJSON_InvalidShape(t *testing.T) {
	cases := []struct {
		name string
		data string
	}{
		{"array", `[1,2,3]`},
		{"number", `42`},
		{"boolean", `true`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var ref AgentRef
			err := json.Unmarshal([]byte(tc.data), &ref)
			if err == nil {
				t.Fatalf("입력 %s 에 대해 에러를 기대했으나 nil 반환됨", tc.data)
			}
			if !strings.Contains(err.Error(), "flow.AgentRef") {
				t.Errorf("에러 메시지에 'flow.AgentRef' 가 포함되어야 함: %v", err)
			}
		})
	}
}

// TestAgentRef_UnmarshalJSON_NodeDefStringFallback 는 사용자의 실제 결함
// 시나리오 — NodeDef.AgentRef 필드에 string 이 들어오는 경우 — 가
// 관대 unmarshal 로 복구되는지 검증한다.
//
// 결함 시나리오: "cannot unmarshal string into Go struct field
// NodeDef.nodes.agent_ref of type flow.AgentRef"
func TestAgentRef_UnmarshalJSON_NodeDefStringFallback(t *testing.T) {
	// nodes[0].agent_ref 가 bare string 인 결함 케이스를 시뮬레이션한다.
	jsonData := []byte(`{
		"id": "node-1",
		"name": "test-node",
		"type": "bridge",
		"inputs": [],
		"outputs": [],
		"agent_ref": "legacy-agent-name"
	}`)

	var node NodeDef
	if err := json.Unmarshal(jsonData, &node); err != nil {
		t.Fatalf("NodeDef 역직렬화 실패 (관대 unmarshal 로 복구되어야 함): %v", err)
	}

	if node.AgentRef == nil {
		t.Fatal("AgentRef 가 nil — 호환 형식이 매핑되지 않음")
	}
	if node.AgentRef.AgentName != "legacy-agent-name" {
		t.Errorf("AgentName 불일치: 기대 %q, 실제 %q", "legacy-agent-name", node.AgentRef.AgentName)
	}
}
