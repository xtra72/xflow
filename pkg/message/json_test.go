package message

import (
	"encoding/json"
	"testing"
	"time"
)

// TestMarshalJSON_AllFields 는 MarshalJSON이 모든 필드를 포함한 유효한 JSON을 생성하는지 검증한다.
func TestMarshalJSON_AllFields(t *testing.T) {
	msg := New(
		WithHistory(true),
		WithMetadata("_source", "test"),
	)
	msg.Payload().Set("name", "John")

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON 파싱 에러: %v", err)
	}

	// 필수 필드 존재 확인
	requiredFields := []string{"id", "timestamp", "payload", "metadata", "history_enabled", "history"}
	for _, field := range requiredFields {
		if _, ok := raw[field]; !ok {
			t.Errorf("JSON에 %q 필드가 없다", field)
		}
	}
}

// TestFromJSON_RestoreFields 는 FromJSON이 ID, Timestamp, Payload, Metadata를 복원하는지 검증한다.
func TestFromJSON_RestoreFields(t *testing.T) {
	original := New(WithMetadata("_source", "agent-1"))
	original.Payload().Set("key1", "val1")
	original.Payload().Set("count", float64(42))

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	restored, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON 에러: %v", err)
	}

	// ID 복원 확인
	if restored.ID() != original.ID() {
		t.Errorf("ID = %q, 기대값 %q", restored.ID(), original.ID())
	}

	// Timestamp 복원 확인 (초 단위까지 비교 - JSON은 나노초 손실 가능)
	if !restored.Timestamp().Truncate(time.Second).Equal(original.Timestamp().Truncate(time.Second)) {
		t.Errorf("Timestamp = %v, 기대값 %v", restored.Timestamp(), original.Timestamp())
	}

	// Payload 복원 확인
	got, ok := restored.Payload().Get("key1")
	if !ok || got != "val1" {
		t.Errorf("Payload key1 = (%v, %v), 기대값 (\"val1\", true)", got, ok)
	}

	gotCount, ok := restored.Payload().Get("count")
	if !ok || gotCount != float64(42) {
		t.Errorf("Payload count = (%v, %v), 기대값 (42, true)", gotCount, ok)
	}

	// Metadata 복원 확인
	src, ok := restored.Metadata().Get("_source")
	if !ok || src != "agent-1" {
		t.Errorf("Metadata _source = (%q, %v), 기대값 (\"agent-1\", true)", src, ok)
	}
}

// TestJSON_RoundTrip 은 Marshal 후 FromJSON으로 왕복 변환이 데이터를 보존하는지 검증한다.
func TestJSON_RoundTrip(t *testing.T) {
	original := New(
		WithHistory(true),
		WithMaxHistory(50),
		WithMetadata("_flowID", "flow-123"),
		WithMetadata("_nodeID", "node-456"),
	)
	original.Payload().Set("user", map[string]any{
		"name": "John",
		"age":  float64(30),
	})
	original.Payload().Set("tags", []any{"go", "tdd"})

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	restored, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON 에러: %v", err)
	}

	// ID 보존
	if restored.ID() != original.ID() {
		t.Errorf("라운드트립 ID 불일치")
	}

	// Payload 보존
	user, ok := restored.Payload().Get("user")
	if !ok {
		t.Fatal("라운드트립 후 user 키 없음")
	}
	userMap, ok := user.(map[string]any)
	if !ok {
		t.Fatal("user가 map[string]any 타입이 아니다")
	}
	if userMap["name"] != "John" {
		t.Errorf("user.name = %v, 기대값 \"John\"", userMap["name"])
	}

	// Metadata 보존
	flowID, ok := restored.Metadata().Get("_flowID")
	if !ok || flowID != "flow-123" {
		t.Errorf("라운드트립 _flowID = (%q, %v)", flowID, ok)
	}
}

// TestFromJSON_WithHistory 는 이력 레코드가 있는 JSON을 복원하는지 검증한다.
func TestFromJSON_WithHistory(t *testing.T) {
	original := New(WithHistory(true))
	original.Payload().Set("key1", "val1")

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	restored, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON 에러: %v", err)
	}

	if !restored.HistoryEnabled() {
		t.Error("복원된 메시지의 HistoryEnabled = false")
	}

	h := restored.History()
	if len(h) != 1 {
		t.Fatalf("복원된 History 길이 = %d, 기대값 1", len(h))
	}
	if h[0].Target != "payload" || h[0].Key != "key1" {
		t.Errorf("복원된 History[0]: Target=%q, Key=%q", h[0].Target, h[0].Key)
	}
}

// TestFromJSON_InvalidJSON 은 잘못된 JSON에 대해 에러를 반환하는지 검증한다.
func TestFromJSON_InvalidJSON(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{name: "빈 바이트", data: []byte{}},
		{name: "잘못된 JSON", data: []byte("{invalid}")},
		{name: "배열 JSON", data: []byte("[]")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := FromJSON(tt.data)
			if err == nil {
				t.Error("잘못된 JSON에 대해 에러가 반환되지 않았다")
			}
		})
	}
}

// TestJSON_EmptyMessage 는 빈 메시지의 직렬화/역직렬화를 검증한다.
func TestJSON_EmptyMessage(t *testing.T) {
	original := New()

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("빈 메시지 MarshalJSON 에러: %v", err)
	}

	restored, err := FromJSON(data)
	if err != nil {
		t.Fatalf("빈 메시지 FromJSON 에러: %v", err)
	}

	if restored.ID() != original.ID() {
		t.Error("빈 메시지 라운드트립 ID 불일치")
	}

	if len(restored.Payload().Keys()) != 0 {
		t.Error("빈 메시지 복원 후 Payload가 비어 있지 않다")
	}

	if len(restored.Metadata().All()) != 0 {
		t.Error("빈 메시지 복원 후 Metadata가 비어 있지 않다")
	}
}

// SPEC-MESSAGE-TYPE-001 AC6-2: 값 있는 Type() 의 JSON 출력.
// top-level "type" 필드로 노출되어야 한다.
func TestMarshalJSON_TypeFieldExposed(t *testing.T) {
	msg := New(WithType("event"))

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON 파싱 에러: %v", err)
	}

	typeRaw, ok := raw["type"]
	if !ok {
		t.Fatalf("AC6-2: JSON 에 top-level 'type' 필드가 부재")
	}

	var typeStr string
	if err := json.Unmarshal(typeRaw, &typeStr); err != nil {
		t.Fatalf("type 값 파싱 에러: %v", err)
	}
	if typeStr != "event" {
		t.Errorf("AC6-2: type 값 불일치 — got %q, want %q", typeStr, "event")
	}
}

// SPEC-MESSAGE-TYPE-001 AC6-3: 빈 Type() 의 JSON 출력.
// omitempty 정책에 따라 "type" 키 부재 (top-level 노이즈 제거).
func TestMarshalJSON_EmptyTypeOmitempty(t *testing.T) {
	msg := New() // type 미설정

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("JSON 파싱 에러: %v", err)
	}

	if _, ok := raw["type"]; ok {
		t.Fatalf("AC6-3: 빈 Type() 의 JSON 에 'type' 키가 노출되었다 (omitempty 미적용)")
	}
}

// SPEC-MESSAGE-TYPE-001 AC6-2: Type 라운드트립 검증.
// MarshalJSON → FromJSON 후 Type() 값이 보존되어야 한다.
func TestJSON_TypeRoundTrip(t *testing.T) {
	original := New(WithType("device_state.change"))

	data, err := original.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	restored, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON 에러: %v", err)
	}

	if got := restored.Type(); got != "device_state.change" {
		t.Errorf("Type 라운드트립 실패 — got %q, want %q", got, "device_state.change")
	}
}

// TestMarshalJSON_HistoryRecordFields 는 이력 레코드의 JSON 필드가 올바른지 검증한다.
func TestMarshalJSON_HistoryRecordFields(t *testing.T) {
	msg := New(WithHistory(true))
	msg.Payload().Set("key1", "val1")

	data, err := msg.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON 에러: %v", err)
	}

	var result struct {
		History []struct {
			Target    string `json:"target"`
			Operation string `json:"operation"`
			Key       string `json:"key"`
			NewValue  any    `json:"new_value"`
			Timestamp string `json:"timestamp"`
		} `json:"history"`
	}

	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("JSON 파싱 에러: %v", err)
	}

	if len(result.History) != 1 {
		t.Fatalf("History 길이 = %d, 기대값 1", len(result.History))
	}

	rec := result.History[0]
	if rec.Target != "payload" {
		t.Errorf("History target = %q, 기대값 \"payload\"", rec.Target)
	}
	if rec.Operation != "set" {
		t.Errorf("History operation = %q, 기대값 \"set\"", rec.Operation)
	}
	if rec.Key != "key1" {
		t.Errorf("History key = %q, 기대값 \"key1\"", rec.Key)
	}
	if rec.Timestamp == "" {
		t.Error("History timestamp가 비어 있다")
	}
}
