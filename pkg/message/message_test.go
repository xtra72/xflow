package message

import (
	"regexp"
	"testing"
	"time"
)

// uuidV4Pattern 은 UUID v4 형식의 정규식이다.
var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// TestMessage_NewDefault 는 기본 생성자가 UUID v4와 유효한 타임스탬프를 생성하는지 검증한다.
func TestMessage_NewDefault(t *testing.T) {
	before := time.Now()
	msg := New()
	after := time.Now()

	// UUID v4 형식 확인
	if !uuidV4Pattern.MatchString(msg.ID()) {
		t.Errorf("ID = %q, UUID v4 형식이 아니다", msg.ID())
	}

	// 타임스탬프 범위 확인
	ts := msg.Timestamp()
	if ts.Before(before) || ts.After(after) {
		t.Errorf("Timestamp = %v, [%v, %v] 범위를 벗어났다", ts, before, after)
	}

	// Payload와 Metadata가 비어 있어야 한다
	if keys := msg.Payload().Keys(); len(keys) != 0 {
		t.Errorf("기본 Payload 키 = %v, 비어 있어야 한다", keys)
	}
	if all := msg.Metadata().All(); len(all) != 0 {
		t.Errorf("기본 Metadata = %v, 비어 있어야 한다", all)
	}
}

// TestMessage_UniqueIDs 는 두 번의 New() 호출이 서로 다른 ID를 생성하는지 검증한다.
func TestMessage_UniqueIDs(t *testing.T) {
	msg1 := New()
	msg2 := New()
	if msg1.ID() == msg2.ID() {
		t.Errorf("두 메시지의 ID가 동일하다: %s", msg1.ID())
	}
}

// TestMessage_DefaultHistoryDisabled 는 기본 메시지의 이력이 비활성화되어 있는지 검증한다.
func TestMessage_DefaultHistoryDisabled(t *testing.T) {
	msg := New()
	if msg.HistoryEnabled() {
		t.Error("기본 메시지에서 HistoryEnabled가 true이다")
	}
	if msg.History() != nil {
		t.Errorf("기본 메시지에서 History가 nil이 아니다: %v", msg.History())
	}
}

// TestMessage_WithHistoryEnabled 는 WithHistory(true) 옵션을 검증한다.
func TestMessage_WithHistoryEnabled(t *testing.T) {
	msg := New(WithHistory(true))
	if !msg.HistoryEnabled() {
		t.Error("WithHistory(true) 후 HistoryEnabled가 false이다")
	}

	h := msg.History()
	if h == nil {
		t.Fatal("WithHistory(true) 후 History가 nil이다")
	}
	if len(h) != 0 {
		t.Errorf("초기 History 길이 = %d, 기대값 0", len(h))
	}
}

// TestMessage_WithHistoryRecording 은 이력 활성화 후 변경이 기록되는지 검증한다.
func TestMessage_WithHistoryRecording(t *testing.T) {
	msg := New(WithHistory(true))
	msg.Payload().Set("key1", "val1")
	msg.Metadata().Set("source", "test")

	h := msg.History()
	if len(h) != 2 {
		t.Fatalf("History 길이 = %d, 기대값 2", len(h))
	}

	if h[0].Target != "payload" || h[0].Key != "key1" {
		t.Errorf("첫 번째 이력: Target=%q, Key=%q", h[0].Target, h[0].Key)
	}
	if h[1].Target != "metadata" || h[1].Key != "source" {
		t.Errorf("두 번째 이력: Target=%q, Key=%q", h[1].Target, h[1].Key)
	}
}

// TestMessage_WithMaxHistory 는 WithMaxHistory 옵션을 검증한다.
func TestMessage_WithMaxHistory(t *testing.T) {
	msg := New(WithHistory(true), WithMaxHistory(2))
	msg.Payload().Set("k1", "v1")
	msg.Payload().Set("k2", "v2")
	msg.Payload().Set("k3", "v3")

	h := msg.History()
	if len(h) != 2 {
		t.Fatalf("MaxHistory=2에서 History 길이 = %d, 기대값 2", len(h))
	}
	if h[0].Key != "k2" {
		t.Errorf("FIFO 후 첫 번째 Key = %q, 기대값 \"k2\"", h[0].Key)
	}
}

// TestMessage_WithMetadata 는 WithMetadata 옵션을 검증한다.
func TestMessage_WithMetadata(t *testing.T) {
	msg := New(WithMetadata("_source", "agent-1"))

	got, ok := msg.Metadata().Get("_source")
	if !ok || got != "agent-1" {
		t.Errorf("WithMetadata 후 _source = (%q, %v), 기대값 (\"agent-1\", true)", got, ok)
	}
}

// TestMessage_WithPayload 는 WithPayload 옵션을 검증한다.
func TestMessage_WithPayload(t *testing.T) {
	custom := NewPayload()
	custom.Set("custom_key", "custom_value")

	msg := New(WithPayload(custom))

	got, ok := msg.Payload().Get("custom_key")
	if !ok || got != "custom_value" {
		t.Errorf("WithPayload 후 custom_key = (%v, %v), 기대값 (\"custom_value\", true)", got, ok)
	}
}

// TestMessage_CombinedOptions 는 여러 옵션 조합이 간섭 없이 동작하는지 검증한다.
func TestMessage_CombinedOptions(t *testing.T) {
	custom := NewPayload()
	custom.Set("data", "test")

	msg := New(
		WithHistory(true),
		WithMaxHistory(50),
		WithMetadata("_source", "combined"),
		WithPayload(custom),
	)

	// 이력 활성화 확인
	if !msg.HistoryEnabled() {
		t.Error("CombinedOptions: HistoryEnabled = false")
	}

	// 메타데이터 확인
	src, ok := msg.Metadata().Get("_source")
	if !ok || src != "combined" {
		t.Errorf("CombinedOptions: _source = (%q, %v)", src, ok)
	}

	// 페이로드 확인
	data, ok := msg.Payload().Get("data")
	if !ok || data != "test" {
		t.Errorf("CombinedOptions: data = (%v, %v)", data, ok)
	}

	// WithPayload + WithHistory: 커스텀 페이로드가 히스토리 래핑되어야 한다
	msg.Payload().Set("new_key", "new_val")
	h := msg.History()
	if len(h) != 1 || h[0].Key != "new_key" {
		t.Errorf("CombinedOptions: 이력 기록 실패: %v", h)
	}
}

// TestMessage_CloneNewID 는 Clone이 새로운 UUID를 생성하는지 검증한다.
func TestMessage_CloneNewID(t *testing.T) {
	msg := New()
	cloned := msg.Clone()

	if msg.ID() == cloned.ID() {
		t.Error("Clone의 ID가 원본과 동일하다")
	}
	if !uuidV4Pattern.MatchString(cloned.ID()) {
		t.Errorf("Clone의 ID = %q, UUID v4 형식이 아니다", cloned.ID())
	}
}

// TestMessage_CloneSameTimestamp 는 Clone이 동일한 타임스탬프를 유지하는지 검증한다.
func TestMessage_CloneSameTimestamp(t *testing.T) {
	msg := New()
	cloned := msg.Clone()

	if !msg.Timestamp().Equal(cloned.Timestamp()) {
		t.Errorf("Clone 타임스탬프 불일치: %v != %v", msg.Timestamp(), cloned.Timestamp())
	}
}

// TestMessage_CloneDeepCopy 는 Clone의 Payload/Metadata가 독립적인지 검증한다.
func TestMessage_CloneDeepCopy(t *testing.T) {
	msg := New()
	msg.Payload().Set("key1", "val1")
	msg.Metadata().Set("source", "original")

	cloned := msg.Clone()

	// 복제본 수정
	cloned.Payload().Set("key1", "modified")
	cloned.Metadata().Set("source", "cloned")

	// 원본 확인
	origPayload, _ := msg.Payload().Get("key1")
	if origPayload != "val1" {
		t.Error("Clone 페이로드 수정이 원본에 영향을 주었다")
	}
	origMeta, _ := msg.Metadata().Get("source")
	if origMeta != "original" {
		t.Error("Clone 메타데이터 수정이 원본에 영향을 주었다")
	}
}

// TestMessage_CloneWithHistory 는 이력 활성화된 메시지의 Clone이 빈 이력을 가지는지 검증한다.
func TestMessage_CloneWithHistory(t *testing.T) {
	msg := New(WithHistory(true), WithMaxHistory(50))
	msg.Payload().Set("key1", "val1")

	cloned := msg.Clone()

	// 원본에 이력이 있어야 한다
	if len(msg.History()) != 1 {
		t.Errorf("원본 History 길이 = %d, 기대값 1", len(msg.History()))
	}

	// 복제본은 빈 이력이어야 한다
	if !cloned.HistoryEnabled() {
		t.Error("Clone에서 HistoryEnabled = false")
	}
	if len(cloned.History()) != 0 {
		t.Errorf("Clone History 길이 = %d, 기대값 0", len(cloned.History()))
	}

	// 복제본의 이력 기록이 독립적으로 동작하는지 확인
	cloned.Payload().Set("new", "data")
	if len(cloned.History()) != 1 {
		t.Errorf("Clone 이력 기록 후 길이 = %d, 기대값 1", len(cloned.History()))
	}
	if len(msg.History()) != 1 {
		t.Errorf("Clone 이력 기록이 원본에 영향: 길이 = %d, 기대값 1", len(msg.History()))
	}
}
