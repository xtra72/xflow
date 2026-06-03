package flow

import (
	"encoding/json"
	"strings"
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

// TestNewWire_VirtualDefaultFalse 는 NewWire 기본 Virtual 값이 false 인지 검증한다 (REQ-LINK-001).
func TestNewWire_VirtualDefaultFalse(t *testing.T) {
	wire := NewWire("a", "out", "b", "in")

	if wire.Virtual {
		t.Errorf("기본 Virtual = %v, 기대값 false", wire.Virtual)
	}
}

// TestNewWire_WithWireVirtual 은 WithWireVirtual 옵션이 Virtual 플래그를 설정하는지 검증한다 (REQ-LINK-001).
func TestNewWire_WithWireVirtual(t *testing.T) {
	tests := []struct {
		name    string
		virtual bool
	}{
		{name: "virtual true", virtual: true},
		{name: "virtual false", virtual: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := NewWire("a", "out", "b", "in", WithWireVirtual(tt.virtual))
			if wire.Virtual != tt.virtual {
				t.Errorf("Virtual = %v, 기대값 %v", wire.Virtual, tt.virtual)
			}
		})
	}
}

// TestWire_JSONRoundTrip_VirtualAndName 은 Wire 직렬화/역직렬화에서 virtual/name 이
// JSON 키 그대로 보존되는지 검증한다 (REQ-LINK-001, AC-B1).
func TestWire_JSONRoundTrip_VirtualAndName(t *testing.T) {
	tests := []struct {
		name    string
		virtual bool
	}{
		{name: "virtual true", virtual: true},
		{name: "virtual false", virtual: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			orig := NewWire("node-a", "out", "node-b", "in",
				WithWireVirtual(tt.virtual),
			)
			orig.Name = "sensor"

			data, err := json.Marshal(orig)
			if err != nil {
				t.Fatalf("Marshal 실패: %v", err)
			}

			// JSON 키가 virtual / name 으로 직렬화되어야 한다.
			if !strings.Contains(string(data), `"virtual"`) {
				t.Errorf("직렬화 JSON 에 \"virtual\" 키가 없다: %s", data)
			}
			if !strings.Contains(string(data), `"name"`) {
				t.Errorf("직렬화 JSON 에 \"name\" 키가 없다: %s", data)
			}

			var got Wire
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal 실패: %v", err)
			}

			if got.Virtual != tt.virtual {
				t.Errorf("라운드트립 Virtual = %v, 기대값 %v", got.Virtual, tt.virtual)
			}
			if got.Name != "sensor" {
				t.Errorf("라운드트립 Name = %q, 기대값 %q", got.Name, "sensor")
			}
		})
	}
}

// TestWire_JSONUnmarshal_MissingVirtualDefaultsFalse 는 virtual 필드가 없는
// 기존 플로우 정의가 기본 false 로 역직렬화되는지 검증한다 (REQ-LINK-031, AC-05).
func TestWire_JSONUnmarshal_MissingVirtualDefaultsFalse(t *testing.T) {
	// virtual 키가 전혀 없는 레거시 JSON.
	legacy := `{"id":"w1","name":"legacy","source_node_id":"a","source_port":"out","target_node_id":"b","target_port":"in"}`

	var got Wire
	if err := json.Unmarshal([]byte(legacy), &got); err != nil {
		t.Fatalf("Unmarshal 실패: %v", err)
	}

	if got.Virtual {
		t.Errorf("virtual 키 없는 레거시 와이어 Virtual = %v, 기대값 false", got.Virtual)
	}
	if got.Name != "legacy" {
		t.Errorf("Name = %q, 기대값 %q", got.Name, "legacy")
	}
}
