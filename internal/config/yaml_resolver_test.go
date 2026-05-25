// yaml_resolver_test.go (SPEC-DEVICE-IDENTITY-001 Phase B — B-T7)

package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/xtra/xflow/internal/device"
)

func TestParseDeviceRef_UUID(t *testing.T) {
	t.Parallel()

	const uid = "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	ref, err := ParseDeviceRef(uid, "flow.yaml", 42)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Raw != uid {
		t.Errorf("Raw = %q, want %q", ref.Raw, uid)
	}
	if ref.Kind != device.DeviceRefUUID {
		t.Errorf("Kind = %v, want DeviceRefUUID", ref.Kind)
	}
	if ref.UUID != uid {
		t.Errorf("UUID = %q, want %q", ref.UUID, uid)
	}
	if ref.IsLegacyComposite() {
		t.Error("expected IsLegacyComposite() == false")
	}
	if ref.SourceFile != "flow.yaml" || ref.SourceLine != 42 {
		t.Errorf("diagnostics lost: %+v", ref)
	}
}

func TestParseDeviceRef_AgentName(t *testing.T) {
	t.Parallel()

	ref, err := ParseDeviceRef("lgcnp/indoor-1", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Kind != device.DeviceRefAgentName {
		t.Errorf("Kind = %v, want DeviceRefAgentName", ref.Kind)
	}
	if ref.Agent != "lgcnp" {
		t.Errorf("Agent = %q, want lgcnp", ref.Agent)
	}
	if ref.Name != "indoor-1" {
		t.Errorf("Name = %q, want indoor-1", ref.Name)
	}
	if ref.IsLegacyComposite() {
		t.Error("expected IsLegacyComposite() == false")
	}
}

func TestParseDeviceRef_Composite(t *testing.T) {
	t.Parallel()

	ref, err := ParseDeviceRef("lgcnp:81", "", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ref.Kind != device.DeviceRefComposite {
		t.Errorf("Kind = %v, want DeviceRefComposite", ref.Kind)
	}
	if ref.Agent != "lgcnp" {
		t.Errorf("Agent = %q, want lgcnp", ref.Agent)
	}
	if ref.Local != "81" {
		t.Errorf("Local = %q, want 81", ref.Local)
	}
	if !ref.IsLegacyComposite() {
		t.Error("expected IsLegacyComposite() == true for composite")
	}
}

func TestParseDeviceRef_InvalidFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
	}{
		{"empty string", ""},
		{"plain word", "invalid-format-xyz"},
		{"agent only no separator", "lgcnp"},
		{"slash at start", "/indoor-1"},
		{"slash at end", "lgcnp/"},
		{"colon at start", ":81"},
		{"colon at end", "lgcnp:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParseDeviceRef(tt.ref, "flow.yaml", 7)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !errors.Is(err, ErrInvalidDeviceReference) {
				t.Errorf("got %v, want wrapped ErrInvalidDeviceReference", err)
			}
			// 위치 정보가 메시지에 포함되어야 한다.
			if tt.ref != "" && !strings.Contains(err.Error(), "flow.yaml:7") {
				t.Errorf("error missing source location: %v", err)
			}
		})
	}
}

func TestParseDeviceRefs_BatchFailFast(t *testing.T) {
	t.Parallel()

	t.Run("all valid", func(t *testing.T) {
		t.Parallel()
		refs, err := ParseDeviceRefs([]string{
			"lgcnp/indoor-1",
			"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
			"lgcnp:81",
		}, "flow.yaml")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(refs) != 3 {
			t.Fatalf("got %d refs, want 3", len(refs))
		}
		wantKinds := []device.DeviceRefKind{
			device.DeviceRefAgentName,
			device.DeviceRefUUID,
			device.DeviceRefComposite,
		}
		for i, w := range wantKinds {
			if refs[i].Kind != w {
				t.Errorf("refs[%d].Kind = %v, want %v", i, refs[i].Kind, w)
			}
		}
	})

	t.Run("fail fast on first invalid", func(t *testing.T) {
		t.Parallel()
		_, err := ParseDeviceRefs([]string{
			"lgcnp/indoor-1",
			"invalid-xyz",
			"lgcnp/indoor-2",
		}, "flow.yaml")
		if !errors.Is(err, ErrInvalidDeviceReference) {
			t.Errorf("got %v, want wrapped ErrInvalidDeviceReference", err)
		}
		// 인덱스가 메시지에 포함되어야 한다.
		if !strings.Contains(err.Error(), "entry 1") {
			t.Errorf("error missing entry index: %v", err)
		}
	})
}

func TestParseDeviceRef_NoSourceLocation(t *testing.T) {
	t.Parallel()

	// 진단 메타데이터 없이 호출한 경우 에러 메시지에 위치 정보 없음.
	_, err := ParseDeviceRef("invalid", "", 0)
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), ".yaml") || strings.Contains(err.Error(), "at line") {
		t.Errorf("unexpected source location in error: %v", err)
	}
}
