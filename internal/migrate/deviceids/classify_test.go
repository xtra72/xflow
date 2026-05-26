package deviceids

import (
	"encoding/json"
	"testing"
)

func TestIsUUID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"valid v4", "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", true},
		{"valid v4 uppercase", "A58BA668-5741-4B3C-9D2E-7F3C8A1B2C3D", true},
		{"v1 not v4", "a58ba668-5741-1b3c-9d2e-7f3c8a1b2c3d", false},
		{"composite", "lgcnp:81", false},
		{"empty", "", false},
		{"too short", "a58ba668-5741-4b3c-9d2e", false},
		{"agent/name", "lgcnp/indoor-1", false},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isUUID(tt.in); got != tt.want {
				t.Errorf("isUUID(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestClassify_AllConvert(t *testing.T) {
	t.Parallel()

	meta := rawMetadata{
		"lgcnp:81":       json.RawMessage(`{"name":"a"}`),
		"lgcnp:82":       json.RawMessage(`{"name":"b"}`),
		"samsung:0.0.16": json.RawMessage(`{"name":"c"}`),
	}
	ids := idMapping{
		"lgcnp:81":       "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82":       "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
		"samsung:0.0.16": "c7d6e88a-7963-4d5e-bf40-9f5eac3d4e5f",
	}

	res := classify(meta, ids)

	if len(res.convert) != 3 {
		t.Errorf("convert count = %d, want 3", len(res.convert))
	}
	if len(res.ambiguous) != 0 {
		t.Errorf("ambiguous count = %d, want 0", len(res.ambiguous))
	}
	if len(res.orphan) != 0 {
		t.Errorf("orphan count = %d, want 0", len(res.orphan))
	}
	if len(res.alreadyUUID) != 0 {
		t.Errorf("alreadyUUID count = %d, want 0", len(res.alreadyUUID))
	}
}

func TestClassify_Orphan(t *testing.T) {
	t.Parallel()

	meta := rawMetadata{
		"lgcnp:81":  json.RawMessage(`{"name":"a"}`),
		"orphan:xx": json.RawMessage(`{"name":"o"}`),
	}
	ids := idMapping{
		"lgcnp:81": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
	}

	res := classify(meta, ids)

	if len(res.convert) != 1 {
		t.Errorf("convert count = %d, want 1", len(res.convert))
	}
	if len(res.orphan) != 1 || res.orphan[0] != "orphan:xx" {
		t.Errorf("orphan = %v, want [orphan:xx]", res.orphan)
	}
}

func TestClassify_AmbiguousDueToManyToOne(t *testing.T) {
	t.Parallel()

	// 두 composite 가 동일 UUID 와 매핑 (다대일) → 둘 다 ambiguous.
	uid := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	meta := rawMetadata{
		"lgcnp:81":  json.RawMessage(`{"name":"a"}`),
		"lgcnp:81b": json.RawMessage(`{"name":"b"}`),
	}
	ids := idMapping{
		"lgcnp:81":  uid,
		"lgcnp:81b": uid,
	}

	res := classify(meta, ids)

	if len(res.ambiguous) != 2 {
		t.Errorf("ambiguous count = %d, want 2", len(res.ambiguous))
	}
	if len(res.convert) != 0 {
		t.Errorf("convert count = %d, want 0 (multi-mapped UUID)", len(res.convert))
	}
}

func TestClassify_AmbiguousDueToCollisionWithExistingUUIDKey(t *testing.T) {
	t.Parallel()

	uid := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	meta := rawMetadata{
		"lgcnp:81": json.RawMessage(`{"name":"a"}`),
		uid:        json.RawMessage(`{"name":"already-migrated"}`),
	}
	ids := idMapping{
		"lgcnp:81": uid,
	}

	res := classify(meta, ids)

	if len(res.ambiguous) != 1 {
		t.Errorf("ambiguous count = %d, want 1", len(res.ambiguous))
	}
	if len(res.alreadyUUID) != 1 {
		t.Errorf("alreadyUUID count = %d, want 1", len(res.alreadyUUID))
	}
	if len(res.convert) != 0 {
		t.Errorf("convert count = %d, want 0 (collision)", len(res.convert))
	}
}

func TestClassify_AlreadyUUIDIsIdempotent(t *testing.T) {
	t.Parallel()

	uid := "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	meta := rawMetadata{
		uid: json.RawMessage(`{"name":"a"}`),
	}
	ids := idMapping{} // ID repository 가 비어 있어도 영향 없음.

	res := classify(meta, ids)

	if len(res.alreadyUUID) != 1 {
		t.Errorf("alreadyUUID count = %d, want 1", len(res.alreadyUUID))
	}
	if len(res.convert) != 0 || len(res.ambiguous) != 0 || len(res.orphan) != 0 {
		t.Errorf("expected only alreadyUUID category populated, got: %+v", res)
	}
}
