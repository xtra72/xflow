package agent

import (
	"testing"
	"time"
)

// mockConnectionStatsProvider 는 테스트용 ConnectionStatsProvider 구현체이다.
type mockConnectionStatsProvider struct{}

func (m *mockConnectionStatsProvider) ConnectionStats() []ConnectionStats {
	return []ConnectionStats{
		{
			ID:               "topic/test",
			MessagesReceived: 10,
			MessagesSent:     5,
			MessagesErrored:  1,
			BytesRead:        1024,
			BytesWritten:     512,
			ConnectedAt:      time.Now().Add(-1 * time.Hour),
			LastActivityAt:   time.Now(),
		},
	}
}

// 컴파일 타임 인터페이스 검증
var _ ConnectionStatsProvider = (*mockConnectionStatsProvider)(nil)

// TestConnectionStats_StructFields 는 ConnectionStats 구조체의 필드와 타입을 검증한다.
func TestConnectionStats_StructFields(t *testing.T) {
	now := time.Now()
	cs := ConnectionStats{
		ID:               "device/001",
		MessagesReceived: 100,
		MessagesSent:     50,
		MessagesErrored:  3,
		BytesRead:        2048,
		BytesWritten:     1024,
		ConnectedAt:      now.Add(-2 * time.Hour),
		LastActivityAt:   now,
	}

	if cs.ID != "device/001" {
		t.Errorf("ID = %q, want %q", cs.ID, "device/001")
	}
	if cs.MessagesReceived != 100 {
		t.Errorf("MessagesReceived = %d, want %d", cs.MessagesReceived, 100)
	}
	if cs.MessagesSent != 50 {
		t.Errorf("MessagesSent = %d, want %d", cs.MessagesSent, 50)
	}
	if cs.MessagesErrored != 3 {
		t.Errorf("MessagesErrored = %d, want %d", cs.MessagesErrored, 3)
	}
	if cs.BytesRead != 2048 {
		t.Errorf("BytesRead = %d, want %d", cs.BytesRead, 2048)
	}
	if cs.BytesWritten != 1024 {
		t.Errorf("BytesWritten = %d, want %d", cs.BytesWritten, 1024)
	}
	if cs.ConnectedAt.IsZero() {
		t.Error("ConnectedAt should not be zero")
	}
	if cs.LastActivityAt.IsZero() {
		t.Error("LastActivityAt should not be zero")
	}
}

// TestConnectionStatsProvider_Interface 는 인터페이스 시그니처를 검증한다.
// mockConnectionStatsProvider 가 ConnectionStatsProvider 를 구현하지 않으면
// 위의 var _ 구문에서 컴파일 에러가 발생한다.
func TestConnectionStatsProvider_Interface(t *testing.T) {
	var provider ConnectionStatsProvider = &mockConnectionStatsProvider{}
	stats := provider.ConnectionStats()
	if stats == nil {
		t.Fatal("ConnectionStats() returned nil, want non-nil slice")
	}
	if len(stats) != 1 {
		t.Fatalf("ConnectionStats() returned %d items, want 1", len(stats))
	}
}

// TestConnectionStats_EmptySlice 는 인터페이스를 구현하지 않는 에이전트에 대해
// 타입 어서션이 실패하는지 검증한다.
func TestConnectionStats_EmptySlice(t *testing.T) {
	// nonProvider 는 ConnectionStatsProvider 를 구현하지 않는 타입이다.
	type nonProvider struct{}
	var v interface{} = &nonProvider{}

	provider, ok := v.(ConnectionStatsProvider)
	if ok {
		t.Error("type assertion should fail for non-implementing type")
	}
	if provider != nil {
		t.Error("provider should be nil for failed type assertion")
	}
}

// TestConnectionStatsProvider_TypeAssertion 는 mock 을 사용한 타입 어서션이
// 정상적으로 동작하는지 검증한다.
func TestConnectionStatsProvider_TypeAssertion(t *testing.T) {
	var v interface{} = &mockConnectionStatsProvider{}

	provider, ok := v.(ConnectionStatsProvider)
	if !ok {
		t.Fatal("type assertion should succeed for mockConnectionStatsProvider")
	}

	stats := provider.ConnectionStats()
	if len(stats) == 0 {
		t.Fatal("expected at least one ConnectionStats entry")
	}

	first := stats[0]
	if first.ID != "topic/test" {
		t.Errorf("ID = %q, want %q", first.ID, "topic/test")
	}
	if first.MessagesReceived != 10 {
		t.Errorf("MessagesReceived = %d, want %d", first.MessagesReceived, 10)
	}
	if first.MessagesSent != 5 {
		t.Errorf("MessagesSent = %d, want %d", first.MessagesSent, 5)
	}
	if first.MessagesErrored != 1 {
		t.Errorf("MessagesErrored = %d, want %d", first.MessagesErrored, 1)
	}
	if first.BytesRead != 1024 {
		t.Errorf("BytesRead = %d, want %d", first.BytesRead, 1024)
	}
	if first.BytesWritten != 512 {
		t.Errorf("BytesWritten = %d, want %d", first.BytesWritten, 512)
	}
}

// TestConnectionStatsProvider_NonImplementor 는 ConnectionStatsProvider 를
// 구현하지 않는 타입에 대해 타입 어서션이 실패하는지 검증한다.
func TestConnectionStatsProvider_NonImplementor(t *testing.T) {
	// BaseAgent 는 ConnectionStatsProvider 를 구현하지 않는다.
	ba := NewBaseAgent()
	var v interface{} = ba

	_, ok := v.(ConnectionStatsProvider)
	if ok {
		t.Error("BaseAgent should not implement ConnectionStatsProvider")
	}
}
