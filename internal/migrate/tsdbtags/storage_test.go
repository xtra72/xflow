// storage_test.go — IDMapping IO + Reverse 헬퍼 단위 테스트.
package tsdbtags

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestLoadIDMapping_NonExistent(t *testing.T) {
	t.Parallel()

	m, err := LoadIDMapping(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("missing 파일에서 에러 반환: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("missing 파일에서 빈 mapping 반환해야 함: %v", m)
	}
}

func TestLoadIDMapping_Empty(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(p, []byte{}, 0o644); err != nil {
		t.Fatalf("write empty: %v", err)
	}
	m, err := LoadIDMapping(p)
	if err != nil {
		t.Fatalf("빈 파일에서 에러 반환: %v", err)
	}
	if len(m) != 0 {
		t.Errorf("빈 파일에서 빈 mapping 반환해야 함: %v", m)
	}
}

func TestLoadIDMapping_Valid(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "ids.json")
	if err := os.WriteFile(p, []byte(`{"lgcnp:81":"a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	m, err := LoadIDMapping(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if m["lgcnp:81"] != "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d" {
		t.Errorf("mapping 불일치: %v", m)
	}
}

func TestLoadIDMapping_InvalidJSON(t *testing.T) {
	t.Parallel()

	p := filepath.Join(t.TempDir(), "bad.json")
	if err := os.WriteFile(p, []byte("not json"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadIDMapping(p); err == nil {
		t.Errorf("잘못된 JSON 에서 에러 반환해야 함")
	}
}

func TestLoadIDMapping_FixtureFile(t *testing.T) {
	t.Parallel()

	m, err := LoadIDMapping("testdata/device-ids.json")
	if err != nil {
		t.Fatalf("fixture 로드 실패: %v", err)
	}
	if m["lgcnp:81"] == "" {
		t.Errorf("fixture 에 lgcnp:81 매핑 없음: %v", m)
	}
}

func TestIDMapping_Reverse(t *testing.T) {
	t.Parallel()

	uidShared := "e9f80aac-9185-4f70-d162-b07fce5f6071"
	m := IDMapping{
		"lgcnp:81":  "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d",
		"lgcnp:99a": uidShared,
		"lgcnp:99b": uidShared,
	}
	rev := m.Reverse()
	if len(rev["a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"]) != 1 {
		t.Errorf("1대1 매핑이 1 entry 여야 함: %v", rev)
	}
	conflicts := rev[uidShared]
	sort.Strings(conflicts)
	want := []string{"lgcnp:99a", "lgcnp:99b"}
	if !reflect.DeepEqual(conflicts, want) {
		t.Errorf("다대일 매핑 reverse: got %v, want %v", conflicts, want)
	}
}
