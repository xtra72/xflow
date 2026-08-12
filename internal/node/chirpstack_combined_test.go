package node

import (
	"encoding/json"
	"testing"
	"time"
)

// combinedTestTS 는 실측 픽스처와 동일한 업링크 시각이다 (2026-08-11T23:32:01.129Z).
var combinedTestTS = time.Date(2026, 8, 11, 23, 32, 1, 129_000_000, time.UTC)

// combinedTestTags 는 per-measurement 경로와 동일한 verbatim tags 그룹이다.
func combinedTestTags() map[string]string {
	return map[string]string{"location": "실습실", "point": "앞문", "spot": "앞문"}
}

// TestBuildChirpStackMessage_Combined 는 combined 판별자 레코드가 모든 measurement 를
// payload 최상위 flat 키로 담은 메시지 1개로 빌드되는지 검증한다.
//
// 계약: type="event", timestamp=UnixMilli, payload=flat measurement 키,
// metadata.measurement 부재, metadata.tags verbatim, unit_id 는 device 승격 후 제거.
func TestBuildChirpStackMessage_Combined(t *testing.T) {
	rec := map[string]any{
		"record": "measurements",
		"values": map[string]any{
			"temperature": 29.8,
			"humidity":    55.2,
		},
		"unit_id": "24e124141d180806",
		"time_ms": combinedTestTS.UnixMilli(),
		"tags":    combinedTestTags(),
	}
	data, _ := json.Marshal(rec)

	msg, ok := buildChirpStackCombinedMessage(data, "node-1", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("buildChirpStackCombinedMessage returned ok=false")
	}

	// type 은 event 로 유지된다 — 다운스트림 switch/filter 가 $.type == "event" 로 분기한다.
	if msg.Type() != "event" {
		t.Errorf("type = %q, want event", msg.Type())
	}
	if !msg.Timestamp().Equal(combinedTestTS) {
		t.Errorf("timestamp = %v, want %v", msg.Timestamp().UTC(), combinedTestTS)
	}
	if v, _ := msg.Payload().Get("temperature"); v != 29.8 {
		t.Errorf("payload.temperature = %v, want 29.8", v)
	}
	if v, _ := msg.Payload().Get("humidity"); v != 55.2 {
		t.Errorf("payload.humidity = %v, want 55.2", v)
	}
	// per-measurement 전용 키는 combined 에 존재하지 않는다.
	if _, ok := msg.Payload().Get("value"); ok {
		t.Error("payload.value 는 combined 모드에 존재하면 안 된다")
	}
	if _, ok := msg.Payload().Get("unit_id"); ok {
		t.Error("payload.unit_id 는 device 승격 후 제거되어야 한다")
	}
	if _, ok := msg.Metadata().Get("measurement"); ok {
		t.Error("metadata.measurement 는 combined 모드에 존재하면 안 된다")
	}

	tags, ok := msg.Metadata().GetGroup("tags")
	if !ok {
		t.Fatal("metadata.tags 그룹 누락")
	}
	if tags["location"] != "실습실" || tags["point"] != "앞문" || tags["spot"] != "앞문" {
		t.Errorf("tags verbatim 불일치: %v", tags)
	}
}

// TestBuildChirpStackMessage_CombinedDispatch 는 record="measurements" 판별자가
// buildChirpStackMessage 에서 combined 빌더로 라우팅되는지 검증한다.
func TestBuildChirpStackMessage_CombinedDispatch(t *testing.T) {
	rec := map[string]any{
		"record":  "measurements",
		"values":  map[string]any{"temperature": 29.8},
		"unit_id": "eui",
		"time_ms": combinedTestTS.UnixMilli(),
	}
	data, _ := json.Marshal(rec)

	msg, ok := buildChirpStackMessage(data, "node-1", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("buildChirpStackMessage returned ok=false")
	}
	if v, _ := msg.Payload().Get("temperature"); v != 29.8 {
		t.Errorf("payload.temperature = %v, want 29.8 (combined 빌더로 라우팅되어야 함)", v)
	}
	if _, ok := msg.Metadata().Get("measurement"); ok {
		t.Error("combined 경로에서 metadata.measurement 가 설정되었다")
	}
}

// TestBuildChirpStackMessage_CombinedTopLevelMatchesPerMeasurement 는 동일 업링크에
// 대해 combined 와 per-measurement 의 top-level 계약(timestamp / tags / unit_id 제거)이
// 일치하는지 검증한다.
func TestBuildChirpStackMessage_CombinedTopLevelMatchesPerMeasurement(t *testing.T) {
	perData, _ := json.Marshal(map[string]any{
		"measurement": "temperature",
		"value":       29.8,
		"unit_id":     "24e124141d180806",
		"time_ms":     combinedTestTS.UnixMilli(),
		"tags":        combinedTestTags(),
	})
	combinedData, _ := json.Marshal(map[string]any{
		"record":  "measurements",
		"values":  map[string]any{"temperature": 29.8, "humidity": 55.2},
		"unit_id": "24e124141d180806",
		"time_ms": combinedTestTS.UnixMilli(),
		"tags":    combinedTestTags(),
	})

	perMsg, ok := buildChirpStackMessage(perData, "node-1", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("per-measurement 빌드 실패")
	}
	combMsg, ok := buildChirpStackMessage(combinedData, "node-1", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("combined 빌드 실패")
	}

	if !combMsg.Timestamp().Equal(perMsg.Timestamp()) {
		t.Errorf("timestamp 불일치: combined=%v per=%v", combMsg.Timestamp(), perMsg.Timestamp())
	}
	if combMsg.Type() != perMsg.Type() {
		t.Errorf("type 불일치: combined=%q per=%q", combMsg.Type(), perMsg.Type())
	}
	perTags, _ := perMsg.Metadata().GetGroup("tags")
	combTags, _ := combMsg.Metadata().GetGroup("tags")
	if len(perTags) != len(combTags) {
		t.Fatalf("tags 크기 불일치: combined=%v per=%v", combTags, perTags)
	}
	for k, v := range perTags {
		if combTags[k] != v {
			t.Errorf("tags[%q] 불일치: combined=%q per=%q", k, combTags[k], v)
		}
	}
	// device 승격 결과(payload 에서 unit_id 제거)도 동일하다.
	if _, ok := combMsg.Payload().Get("unit_id"); ok {
		t.Error("combined payload 에 unit_id 가 남아 있다")
	}
	if _, ok := perMsg.Payload().Get("unit_id"); ok {
		t.Error("per-measurement payload 에 unit_id 가 남아 있다")
	}
}

// TestBuildChirpStackMessage_CombinedBadJSON 은 잘못된 JSON 이 ok=false 로 안전하게
// 처리되는지 검증한다.
func TestBuildChirpStackMessage_CombinedBadJSON(t *testing.T) {
	if _, ok := buildChirpStackCombinedMessage([]byte("not-json"), "n", nil, "", DefaultEmitOptions()); ok {
		t.Error("bad JSON 은 ok=false 여야 한다")
	}
}

// TestBuildChirpStackMessage_PerMeasurementStillDefault 는 record 판별자가 없는
// (오늘의 기본 경로) 레코드가 여전히 per-measurement 계약으로 빌드되는지 검증한다
// (REQ-FROZEN-01/02 특성 테스트).
func TestBuildChirpStackMessage_PerMeasurementStillDefault(t *testing.T) {
	data, _ := json.Marshal(map[string]any{
		"measurement": "magnet_status",
		"value":       "close",
		"unit_id":     "24e124141d180806",
		"time_ms":     combinedTestTS.UnixMilli(),
		"tags":        combinedTestTags(),
	})

	msg, ok := buildChirpStackMessage(data, "node-1", nil, "", DefaultEmitOptions())
	if !ok {
		t.Fatal("buildChirpStackMessage returned ok=false")
	}
	if msg.Type() != "event" {
		t.Errorf("type = %q, want event", msg.Type())
	}
	// 동결된 다운스트림 읽기 경로.
	if v, _ := msg.Payload().Get("value"); v != "close" {
		t.Errorf("$.payload.value = %v, want close", v)
	}
	if m, _ := msg.Metadata().Get("measurement"); m != "magnet_status" {
		t.Errorf("$.metadata.measurement = %q, want magnet_status", m)
	}
	if !msg.Timestamp().Equal(combinedTestTS) {
		t.Errorf("$.timestamp = %v, want %v", msg.Timestamp().UTC(), combinedTestTS)
	}
}
