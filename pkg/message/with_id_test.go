package message

import (
	"testing"
)

// TestMessage_WithID 는 WithID 옵션이 제공된 id 를 사용함을 검증한다.
func TestMessage_WithID(t *testing.T) {
	msg := New(WithID("fixed-id-123"))
	if msg.ID() != "fixed-id-123" {
		t.Fatalf("WithID 가 제공된 id 를 사용해야 함: got=%q", msg.ID())
	}
}

// TestMessage_WithID_EmptyFallsBackToUUID 는 빈 id 전달 시 New() 가 uuid 를 생성함을 검증한다.
func TestMessage_WithID_EmptyFallsBackToUUID(t *testing.T) {
	msg := New(WithID(""))
	if msg.ID() == "" {
		t.Fatal("빈 WithID 는 uuid 생성으로 폴백해야 함")
	}
}

// TestMessage_WithID_DefaultGeneratesUUID 는 WithID 미지정 시 매번 다른 uuid 가 생성됨을 검증한다.
func TestMessage_WithID_DefaultGeneratesUUID(t *testing.T) {
	a := New()
	b := New()
	if a.ID() == "" || b.ID() == "" {
		t.Fatal("기본 생성 id 는 비어 있으면 안 됨")
	}
	if a.ID() == b.ID() {
		t.Fatal("기본 생성 id 는 매번 달라야 함")
	}
}
