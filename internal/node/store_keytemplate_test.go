package node

import (
	"testing"

	"github.com/xtra/xflow/pkg/message"
)

// TestResolveKeyTemplate_LegacySyntax 는 {field} 구문이 backward compatible
// 한지 검증한다 (v0.7.9 이전 동작).
func TestResolveKeyTemplate_LegacySyntax(t *testing.T) {
	t.Parallel()
	msg := message.New()
	msg.Payload().Set("location", "A")
	msg.Payload().Set("sensor", "temp01")

	got, err := resolveKeyTemplate("{location}:{sensor}", msg)
	if err != nil {
		t.Fatalf("resolveKeyTemplate: %v", err)
	}
	if got != "A:temp01" {
		t.Errorf("= %q, want %q", got, "A:temp01")
	}
}

// TestResolveKeyTemplate_PayloadPath 는 $.payload.x JSONPath 구문을 검증한다.
func TestResolveKeyTemplate_PayloadPath(t *testing.T) {
	t.Parallel()
	msg := message.New()
	msg.Payload().Set("device_id", "idu-1")

	got, err := resolveKeyTemplate("device:{$.payload.device_id}", msg)
	if err != nil {
		t.Fatalf("resolveKeyTemplate: %v", err)
	}
	if got != "device:idu-1" {
		t.Errorf("= %q, want %q", got, "device:idu-1")
	}
}

// TestResolveKeyTemplate_MetadataPath 는 $.metadata.x 구문을 검증한다.
func TestResolveKeyTemplate_MetadataPath(t *testing.T) {
	t.Parallel()
	msg := message.New()
	msg.Metadata().Set("device_id", "0x3B")
	msg.Payload().Set("kind", "temp")

	got, err := resolveKeyTemplate("{$.metadata.device_id}:{kind}", msg)
	if err != nil {
		t.Fatalf("resolveKeyTemplate: %v", err)
	}
	if got != "0x3B:temp" {
		t.Errorf("= %q, want %q", got, "0x3B:temp")
	}
}

// TestResolveKeyTemplate_NestedPayload 는 $.payload.state.mode 같은 중첩
// 경로를 검증한다.
func TestResolveKeyTemplate_NestedPayload(t *testing.T) {
	t.Parallel()
	msg := message.New()
	msg.Payload().Set("state", map[string]any{
		"mode":     1,
		"sub_mode": "cool",
	})

	got, err := resolveKeyTemplate("m:{$.payload.state.mode}:{$.payload.state.sub_mode}", msg)
	if err != nil {
		t.Fatalf("resolveKeyTemplate: %v", err)
	}
	if got != "m:1:cool" {
		t.Errorf("= %q, want %q", got, "m:1:cool")
	}
}

// TestResolveKeyTemplate_MixedSyntax 는 legacy 와 JSONPath 구문 혼용을 검증한다.
func TestResolveKeyTemplate_MixedSyntax(t *testing.T) {
	t.Parallel()
	msg := message.New()
	msg.Metadata().Set("agent", "century")
	msg.Payload().Set("device_id", "0x3B")
	msg.Payload().Set("metric", "temp")

	got, err := resolveKeyTemplate("{$.metadata.agent}/{device_id}/{metric}", msg)
	if err != nil {
		t.Fatalf("resolveKeyTemplate: %v", err)
	}
	if got != "century/0x3B/temp" {
		t.Errorf("= %q, want %q", got, "century/0x3B/temp")
	}
}

// TestResolveKeyTemplate_Errors 는 잘못된 입력의 에러 케이스를 검증한다.
func TestResolveKeyTemplate_Errors(t *testing.T) {
	t.Parallel()
	msg := message.New()
	msg.Payload().Set("known", "x")

	cases := []struct {
		name     string
		template string
		want     string // expected substring in error
	}{
		{"unknown legacy field", "{missing}", `"missing" not found`},
		{"unknown payload path", "{$.payload.nope}", `"nope" not found`},
		{"unknown metadata", "{$.metadata.nope}", `"nope" not found`},
		{"unknown root", "{$.unknown.x}", `unknown key template root`},
		{"too short path", "{$.payload}", `invalid key template path`},
		{"metadata nested", "{$.metadata.a.b}", `nested access not supported`},
		{"non-map traversal", "{$.payload.known.deeper}", `not a nested object`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveKeyTemplate(tc.template, msg)
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.want)
			}
			if !containsString(err.Error(), tc.want) {
				t.Errorf("error = %q, want substring %q", err.Error(), tc.want)
			}
		})
	}
}

func containsString(s, sub string) bool {
	return len(s) >= len(sub) && indexOf(s, sub) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
