package message

import (
	"testing"
	"time"
)

// TestHistoryPayload_RecordAdd 는 historyPayload가 Add 연산을 기록하는지 검증한다.
func TestHistoryPayload_RecordAdd(t *testing.T) {
	inner := NewPayload()
	var history []ChangeRecord
	hp := newHistoryPayload(inner, &history, "node-1", 100)

	err := hp.Add("key1", "val1")
	if err != nil {
		t.Fatalf("Add 에러: %v", err)
	}

	if len(history) != 1 {
		t.Fatalf("이력 길이 = %d, 기대값 1", len(history))
	}

	rec := history[0]
	if rec.Target != "payload" {
		t.Errorf("Target = %q, 기대값 \"payload\"", rec.Target)
	}
	if rec.Operation != "add" {
		t.Errorf("Operation = %q, 기대값 \"add\"", rec.Operation)
	}
	if rec.Key != "key1" {
		t.Errorf("Key = %q, 기대값 \"key1\"", rec.Key)
	}
	if rec.OldValue != nil {
		t.Errorf("OldValue = %v, 기대값 nil", rec.OldValue)
	}
	if rec.NewValue != "val1" {
		t.Errorf("NewValue = %v, 기대값 \"val1\"", rec.NewValue)
	}
	if rec.NodeID != "node-1" {
		t.Errorf("NodeID = %q, 기대값 \"node-1\"", rec.NodeID)
	}
	if rec.Timestamp.IsZero() {
		t.Error("Timestamp가 zero 값이다")
	}
}

// TestHistoryPayload_RecordSet 은 historyPayload가 Set 연산을 기록하는지 검증한다.
func TestHistoryPayload_RecordSet(t *testing.T) {
	inner := NewPayload()
	inner.Set("key1", "old")
	var history []ChangeRecord
	hp := newHistoryPayload(inner, &history, "node-2", 100)

	hp.Set("key1", "new")

	if len(history) != 1 {
		t.Fatalf("이력 길이 = %d, 기대값 1", len(history))
	}

	rec := history[0]
	if rec.Operation != "set" {
		t.Errorf("Operation = %q, 기대값 \"set\"", rec.Operation)
	}
	if rec.OldValue != "old" {
		t.Errorf("OldValue = %v, 기대값 \"old\"", rec.OldValue)
	}
	if rec.NewValue != "new" {
		t.Errorf("NewValue = %v, 기대값 \"new\"", rec.NewValue)
	}
}

// TestHistoryPayload_RecordDelete 는 historyPayload가 Delete 연산을 기록하는지 검증한다.
func TestHistoryPayload_RecordDelete(t *testing.T) {
	inner := NewPayload()
	inner.Set("key1", "val1")
	var history []ChangeRecord
	hp := newHistoryPayload(inner, &history, "node-3", 100)

	hp.Delete("key1")

	if len(history) != 1 {
		t.Fatalf("이력 길이 = %d, 기대값 1", len(history))
	}

	rec := history[0]
	if rec.Operation != "delete" {
		t.Errorf("Operation = %q, 기대값 \"delete\"", rec.Operation)
	}
	if rec.OldValue != "val1" {
		t.Errorf("OldValue = %v, 기대값 \"val1\"", rec.OldValue)
	}
	if rec.NewValue != nil {
		t.Errorf("NewValue = %v, 기대값 nil", rec.NewValue)
	}
}

// TestHistoryMetadata_RecordSet 은 historyMetadata가 Set 연산을 기록하는지 검증한다.
func TestHistoryMetadata_RecordSet(t *testing.T) {
	inner := NewMetadata()
	var history []ChangeRecord
	hm := newHistoryMetadata(inner, &history, "node-4", 100)

	hm.Set("source", "agent-1")

	if len(history) != 1 {
		t.Fatalf("이력 길이 = %d, 기대값 1", len(history))
	}

	rec := history[0]
	if rec.Target != "metadata" {
		t.Errorf("Target = %q, 기대값 \"metadata\"", rec.Target)
	}
	if rec.Operation != "set" {
		t.Errorf("Operation = %q, 기대값 \"set\"", rec.Operation)
	}
	if rec.Key != "source" {
		t.Errorf("Key = %q, 기대값 \"source\"", rec.Key)
	}
	if rec.NewValue != "agent-1" {
		t.Errorf("NewValue = %v, 기대값 \"agent-1\"", rec.NewValue)
	}
}

// TestHistoryMetadata_RecordRemove 는 historyMetadata가 Remove 연산을 기록하는지 검증한다.
func TestHistoryMetadata_RecordRemove(t *testing.T) {
	inner := NewMetadata()
	inner.Set("source", "agent-1")
	var history []ChangeRecord
	hm := newHistoryMetadata(inner, &history, "node-5", 100)

	hm.Remove("source")

	if len(history) != 1 {
		t.Fatalf("이력 길이 = %d, 기대값 1", len(history))
	}

	rec := history[0]
	if rec.Operation != "remove" {
		t.Errorf("Operation = %q, 기대값 \"remove\"", rec.Operation)
	}
	if rec.OldValue != "agent-1" {
		t.Errorf("OldValue = %v, 기대값 \"agent-1\"", rec.OldValue)
	}
	if rec.NewValue != nil {
		t.Errorf("NewValue = %v, 기대값 nil", rec.NewValue)
	}
}

// TestHistory_FIFO 는 maxHistory 초과 시 FIFO로 가장 오래된 레코드가 제거되는지 검증한다.
func TestHistory_FIFO(t *testing.T) {
	inner := NewPayload()
	var history []ChangeRecord
	hp := newHistoryPayload(inner, &history, "node-6", 3)

	// 4번 연산 수행 (최대 3개)
	hp.Set("k1", "v1")
	hp.Set("k2", "v2")
	hp.Set("k3", "v3")
	hp.Set("k4", "v4")

	if len(history) != 3 {
		t.Fatalf("이력 길이 = %d, 기대값 3 (FIFO 적용)", len(history))
	}

	// 가장 오래된 k1 기록이 제거되어야 한다
	if history[0].Key != "k2" {
		t.Errorf("FIFO 후 첫 번째 레코드 Key = %q, 기대값 \"k2\"", history[0].Key)
	}
	if history[2].Key != "k4" {
		t.Errorf("FIFO 후 마지막 레코드 Key = %q, 기대값 \"k4\"", history[2].Key)
	}
}

// TestHistoryPayload_ReadPassthrough 는 읽기 연산이 내부 Payload로 전달되는지 검증한다.
func TestHistoryPayload_ReadPassthrough(t *testing.T) {
	inner := NewPayload()
	inner.Set("key1", "val1")
	var history []ChangeRecord
	hp := newHistoryPayload(inner, &history, "node-7", 100)

	// Get 패스스루
	got, ok := hp.Get("key1")
	if !ok || got != "val1" {
		t.Errorf("Get 패스스루 실패: (%v, %v)", got, ok)
	}

	// Keys 패스스루
	keys := hp.Keys()
	if len(keys) != 1 || keys[0] != "key1" {
		t.Errorf("Keys 패스스루 실패: %v", keys)
	}

	// ToMap 패스스루
	m := hp.ToMap()
	if m["key1"] != "val1" {
		t.Error("ToMap 패스스루 실패")
	}

	// GetPath 패스스루
	inner.Set("nested", map[string]any{"inner": "value"})
	pathVal, pathErr := hp.GetPath("$.nested.inner")
	if pathErr != nil {
		t.Errorf("GetPath 패스스루 에러: %v", pathErr)
	}
	if pathVal != "value" {
		t.Errorf("GetPath 패스스루 결과 = %v, 기대값 \"value\"", pathVal)
	}

	// ToJSON 패스스루
	data, err := hp.ToJSON()
	if err != nil {
		t.Errorf("ToJSON 패스스루 에러: %v", err)
	}
	if len(data) == 0 {
		t.Error("ToJSON 빈 결과")
	}

	// 읽기 연산은 이력에 기록되지 않아야 한다
	if len(history) != 0 {
		t.Errorf("읽기 연산이 이력에 기록되었다: %d건", len(history))
	}
}

// TestHistoryMetadata_ReadPassthrough 는 읽기 연산이 내부 Metadata로 전달되는지 검증한다.
func TestHistoryMetadata_ReadPassthrough(t *testing.T) {
	inner := NewMetadata()
	inner.Set("source", "agent-1")
	var history []ChangeRecord
	hm := newHistoryMetadata(inner, &history, "node-8", 100)

	// Get 패스스루
	got, ok := hm.Get("source")
	if !ok || got != "agent-1" {
		t.Errorf("Get 패스스루 실패: (%v, %v)", got, ok)
	}

	// Has 패스스루
	if !hm.Has("source") {
		t.Error("Has 패스스루 실패")
	}

	// All 패스스루
	all := hm.All()
	if all["source"] != "agent-1" {
		t.Error("All 패스스루 실패")
	}

	// 읽기 연산은 이력에 기록되지 않아야 한다
	if len(history) != 0 {
		t.Errorf("읽기 연산이 이력에 기록되었다: %d건", len(history))
	}
}

// TestHistoryPayload_CloneReturnsInnerClone 은 Clone이 래핑되지 않은 내부 Payload 복제본을 반환하는지 검증한다.
func TestHistoryPayload_CloneReturnsInnerClone(t *testing.T) {
	inner := NewPayload()
	inner.Set("key1", "val1")
	var history []ChangeRecord
	hp := newHistoryPayload(inner, &history, "node-9", 100)

	cloned := hp.Clone()

	// 복제본에서 값 확인
	got, ok := cloned.Get("key1")
	if !ok || got != "val1" {
		t.Errorf("Clone 후 값 불일치: (%v, %v)", got, ok)
	}

	// 복제본은 historyPayload가 아닌 일반 Payload여야 한다
	if _, isHistory := cloned.(*historyPayload); isHistory {
		t.Error("Clone이 historyPayload를 반환했다, 일반 Payload여야 한다")
	}
}

// TestHistoryMetadata_CloneReturnsInnerClone 은 Clone이 래핑되지 않은 내부 Metadata 복제본을 반환하는지 검증한다.
func TestHistoryMetadata_CloneReturnsInnerClone(t *testing.T) {
	inner := NewMetadata()
	inner.Set("source", "agent-1")
	var history []ChangeRecord
	hm := newHistoryMetadata(inner, &history, "node-10", 100)

	cloned := hm.Clone()

	got, ok := cloned.Get("source")
	if !ok || got != "agent-1" {
		t.Errorf("Clone 후 값 불일치: (%v, %v)", got, ok)
	}

	if _, isHistory := cloned.(*historyMetadata); isHistory {
		t.Error("Clone이 historyMetadata를 반환했다, 일반 Metadata여야 한다")
	}
}

// TestChangeRecord_TimestampOrder 는 ChangeRecord의 타임스탬프가 시간순으로 기록되는지 검증한다.
func TestChangeRecord_TimestampOrder(t *testing.T) {
	inner := NewPayload()
	var history []ChangeRecord
	hp := newHistoryPayload(inner, &history, "node-11", 100)

	hp.Set("k1", "v1")
	time.Sleep(time.Millisecond)
	hp.Set("k2", "v2")

	if len(history) < 2 {
		t.Fatalf("이력 길이 부족: %d", len(history))
	}

	if !history[0].Timestamp.Before(history[1].Timestamp) && !history[0].Timestamp.Equal(history[1].Timestamp) {
		t.Error("타임스탬프 순서가 올바르지 않다")
	}
}
