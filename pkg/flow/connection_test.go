package flow

import (
	"testing"
	"time"
)

// TestWireMode_Constants 는 WireMode 상수 값이 올바른지 검증한다.
func TestWireMode_Constants(t *testing.T) {
	tests := []struct {
		name     string
		got      WireMode
		expected string
	}{
		{name: "WireBypass 값", got: WireBypass, expected: "bypass"},
		{name: "WireBuffer 값", got: WireBuffer, expected: "buffer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if string(tt.got) != tt.expected {
				t.Errorf("WireMode = %q, 기대값 %q", tt.got, tt.expected)
			}
		})
	}
}

// TestNewWire_Basic 은 기본 NewWire 팩토리 함수가 올바른 와이어를 생성하는지 검증한다 (AC-11).
func TestNewWire_Basic(t *testing.T) {
	wire := NewWire("node-a", "out", "node-b", "in")

	// ID가 유효한 UUID여야 한다
	if !isValidUUID(wire.ID) {
		t.Errorf("NewWire ID = %q, 유효한 UUID 형식이 아니다", wire.ID)
	}

	// 소스 필드가 올바르게 설정되어야 한다
	if wire.SourceNodeID != "node-a" {
		t.Errorf("SourceNodeID = %q, 기대값 %q", wire.SourceNodeID, "node-a")
	}
	if wire.SourcePort != "out" {
		t.Errorf("SourcePort = %q, 기대값 %q", wire.SourcePort, "out")
	}

	// 타겟 필드가 올바르게 설정되어야 한다
	if wire.TargetNodeID != "node-b" {
		t.Errorf("TargetNodeID = %q, 기대값 %q", wire.TargetNodeID, "node-b")
	}
	if wire.TargetPort != "in" {
		t.Errorf("TargetPort = %q, 기대값 %q", wire.TargetPort, "in")
	}

	// 기본 Mode가 WireBypass여야 한다
	if wire.Mode != WireBypass {
		t.Errorf("기본 Mode = %q, 기대값 %q", wire.Mode, WireBypass)
	}

	// 기본 BufferSize가 0이어야 한다
	if wire.BufferSize != 0 {
		t.Errorf("기본 BufferSize = %d, 기대값 0", wire.BufferSize)
	}

	// 기본 TTL이 0이어야 한다
	if wire.TTL != 0 {
		t.Errorf("기본 TTL = %v, 기대값 0", wire.TTL)
	}
}

// TestNewWire_BufferMode 는 버퍼 모드 옵션이 올바르게 적용되는지 검증한다 (AC-12).
func TestNewWire_BufferMode(t *testing.T) {
	wire := NewWire("src", "output", "dst", "input",
		WithWireMode(WireBuffer),
		WithBufferSize(100),
		WithTTL(5*time.Second),
	)

	// Mode가 WireBuffer여야 한다
	if wire.Mode != WireBuffer {
		t.Errorf("Mode = %q, 기대값 %q", wire.Mode, WireBuffer)
	}

	// BufferSize가 100이어야 한다
	if wire.BufferSize != 100 {
		t.Errorf("BufferSize = %d, 기대값 100", wire.BufferSize)
	}

	// TTL이 5초여야 한다
	if wire.TTL != 5*time.Second {
		t.Errorf("TTL = %v, 기대값 %v", wire.TTL, 5*time.Second)
	}
}

// TestNewWire_WithWireMode 는 WithWireMode 옵션이 모드를 올바르게 설정하는지 검증한다.
func TestNewWire_WithWireMode(t *testing.T) {
	tests := []struct {
		name     string
		mode     WireMode
		expected WireMode
	}{
		{name: "Bypass 모드", mode: WireBypass, expected: WireBypass},
		{name: "Buffer 모드", mode: WireBuffer, expected: WireBuffer},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := NewWire("a", "out", "b", "in", WithWireMode(tt.mode))
			if wire.Mode != tt.expected {
				t.Errorf("Mode = %q, 기대값 %q", wire.Mode, tt.expected)
			}
		})
	}
}

// TestNewWire_WithBufferSize 는 WithBufferSize 옵션이 버퍼 크기를 올바르게 설정하는지 검증한다.
func TestNewWire_WithBufferSize(t *testing.T) {
	wire := NewWire("a", "out", "b", "in", WithBufferSize(256))

	if wire.BufferSize != 256 {
		t.Errorf("BufferSize = %d, 기대값 256", wire.BufferSize)
	}
}

// TestNewWire_WithTTL 은 WithTTL 옵션이 TTL을 올바르게 설정하는지 검증한다.
func TestNewWire_WithTTL(t *testing.T) {
	ttl := 30 * time.Second
	wire := NewWire("a", "out", "b", "in", WithTTL(ttl))

	if wire.TTL != ttl {
		t.Errorf("TTL = %v, 기대값 %v", wire.TTL, ttl)
	}
}

// TestNewWire_UniqueIDs 는 NewWire가 호출될 때마다 고유한 ID를 생성하는지 검증한다.
func TestNewWire_UniqueIDs(t *testing.T) {
	wire1 := NewWire("a", "out", "b", "in")
	wire2 := NewWire("a", "out", "b", "in")

	if wire1.ID == wire2.ID {
		t.Errorf("두 와이어의 ID가 동일하다: %q", wire1.ID)
	}
}
