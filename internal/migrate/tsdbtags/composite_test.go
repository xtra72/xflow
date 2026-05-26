// composite_test.go — UUID/composite 분류와 Classify 알고리즘 단위 테스트.
package tsdbtags

import (
	"reflect"
	"testing"
)

func TestIsUUID(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", true},
		{"A58BA668-5741-4B3C-9D2E-7F3C8A1B2C3D", true}, // 대문자 허용
		{"lgcnp:81", false},
		{"", false},
		{"not-a-uuid", false},
		{"a58ba668-5741-1b3c-9d2e-7f3c8a1b2c3d", false}, // version != 4
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := IsUUID(tc.in); got != tc.want {
				t.Errorf("IsUUID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestIsCompositeShape(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want bool
	}{
		{"lgcnp:81", true},
		{"samsung:0.0.16", true},
		{"century:7", true},
		{"agent_v2:device-1", true},
		{"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", false}, // UUID 는 composite 아님
		{"plain-string", false},                         // 콜론 없음
		{"too:many:colons", false},                      // 콜론 2개 이상
		{"", false},
		{"north-1", false}, // room name (콜론 없음)
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.in, func(t *testing.T) {
			t.Parallel()
			if got := IsCompositeShape(tc.in); got != tc.want {
				t.Errorf("IsCompositeShape(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestClassify_AllCategories(t *testing.T) {
	t.Parallel()

	uidShared := "e9f80aac-9185-4f70-d162-b07fce5f6071"
	ids := IDMapping{
		"lgcnp:81":  "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:82":  "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
		"lgcnp:99a": uidShared,
		"lgcnp:99b": uidShared, // 다대일 → ambiguous
	}
	scanned := []ScannedTagValue{
		{Value: "lgcnp:81", TagKey: "device_id", Measurements: []string{"indoor_temp", "outdoor_temp"}},
		{Value: "lgcnp:82", TagKey: "device_id", Measurements: []string{"indoor_temp"}},
		{Value: "lgcnp:99a", TagKey: "device_id", Measurements: []string{"hvac_status"}},
		{Value: "lgcnp:99b", TagKey: "device_id", Measurements: []string{"hvac_status"}},
		{Value: "samsung:0.0.16", TagKey: "device_id", Measurements: []string{"indoor_temp"}},                       // orphan (매핑 없음)
		{Value: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", TagKey: "device_id", Measurements: []string{"hvac_status"}}, // UUID already
		{Value: "lgcnp", TagKey: "agent", Measurements: []string{"indoor_temp"}},                                    // 일반 tag — 무시
		{Value: "north-1", TagKey: "room", Measurements: []string{"indoor_temp"}},                                   // 일반 tag — 무시
	}

	res := Classify(scanned, ids)

	if res.MappedCount() != 2 {
		t.Errorf("Mapped 개수 = %d, want 2 (lgcnp:81, lgcnp:82): %+v", res.MappedCount(), res.Mapped)
	}
	if res.AmbiguousCount() != 2 {
		t.Errorf("Ambiguous 개수 = %d, want 2 (lgcnp:99a, lgcnp:99b): %+v", res.AmbiguousCount(), res.Ambiguous)
	}
	if res.OrphanCount() != 1 {
		t.Errorf("Orphan 개수 = %d, want 1 (samsung:0.0.16): %+v", res.OrphanCount(), res.Orphan)
	}
	if res.UUIDAlreadyCount() != 1 {
		t.Errorf("UUIDAlready 개수 = %d, want 1: %+v", res.UUIDAlreadyCount(), res.UUIDAlready)
	}

	// Mapped 의 measurements 가 정렬되어 있는지.
	for _, m := range res.Mapped {
		if m.Composite == "lgcnp:81" {
			want := []string{"indoor_temp", "outdoor_temp"}
			if !reflect.DeepEqual(m.Measurements, want) {
				t.Errorf("lgcnp:81 의 measurements 정렬 안 됨: got %v, want %v", m.Measurements, want)
			}
		}
	}

	// Ambiguous 의 conflicts 가 채워졌는지.
	for _, a := range res.Ambiguous {
		if len(a.Conflicts) != 1 {
			t.Errorf("ambiguous %q 의 conflicts 가 1 이어야 함: %v", a.Composite, a.Conflicts)
		}
	}
}

func TestClassify_DeterministicOrder(t *testing.T) {
	t.Parallel()

	ids := IDMapping{
		"agent-a:1": "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"agent-b:2": "b6c5d779-6852-4c4d-ae3f-8f4d9b2c3d4e",
		"agent-c:3": "c7d6e88a-7963-4d5e-bf40-9f5eac3d4e5f",
	}
	// 입력 순서를 일부러 뒤섞음.
	scanned := []ScannedTagValue{
		{Value: "agent-c:3", TagKey: "device_id", Measurements: []string{"m1"}},
		{Value: "agent-a:1", TagKey: "device_id", Measurements: []string{"m1"}},
		{Value: "agent-b:2", TagKey: "device_id", Measurements: []string{"m1"}},
	}
	res1 := Classify(scanned, ids)
	res2 := Classify(scanned, ids)
	if !reflect.DeepEqual(res1.Mapped, res2.Mapped) {
		t.Errorf("Classify 결과가 결정적이지 않음:\n  1=%+v\n  2=%+v", res1.Mapped, res2.Mapped)
	}
	// 알파벳 순으로 정렬되어 있는지.
	if res1.Mapped[0].Composite != "agent-a:1" {
		t.Errorf("Mapped 정렬 안 됨: %+v", res1.Mapped)
	}
}

func TestClassify_EmptyInputs(t *testing.T) {
	t.Parallel()

	res := Classify(nil, IDMapping{})
	if res.MappedCount() != 0 || res.AmbiguousCount() != 0 || res.OrphanCount() != 0 || res.UUIDAlreadyCount() != 0 {
		t.Errorf("빈 입력에 대해 모든 카운트가 0 이어야 함: %+v", res)
	}
}
