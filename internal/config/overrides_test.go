package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOverrideStore_SaveLoad 는 Set 으로 저장한 값이 새 스토어에서 로드되는지 검증한다.
func TestOverrideStore_SaveLoad(t *testing.T) {
	dir := t.TempDir()
	s := NewOverrideStore(dir, nil)
	if err := s.Set("remote_management.mode", "client"); err != nil {
		t.Fatalf("Set error: %v", err)
	}
	if err := s.Set("remote_management.server_url", "wss://x"); err != nil {
		t.Fatalf("Set error: %v", err)
	}

	// 새 스토어가 파일에서 값을 로드해야 한다.
	s2 := NewOverrideStore(dir, nil)
	vals := s2.Values()
	rm, ok := vals["remote_management"].(map[string]any)
	if !ok {
		t.Fatalf("remote_management nested map 없음: %#v", vals)
	}
	if rm["mode"] != "client" {
		t.Errorf("mode = %v, want client", rm["mode"])
	}
	if rm["server_url"] != "wss://x" {
		t.Errorf("server_url = %v, want wss://x", rm["server_url"])
	}
}

// TestOverrideStore_CorruptionIgnored 는 손상된 오버라이드 파일이 무시되고(원본 복원 효과)
// 빈 값을 반환하는지 검증한다.
func TestOverrideStore_CorruptionIgnored(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, overridesFileName), []byte("{{{ not yaml"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewOverrideStore(dir, nil)
	if len(s.Values()) != 0 {
		t.Errorf("손상 파일은 무시되어 빈 오버라이드여야 함: %#v", s.Values())
	}
}

// TestOverrideStore_HistoryRotation 는 저장 시마다 이전 버전이 히스토리로 보관되는지 검증한다.
func TestOverrideStore_HistoryRotation(t *testing.T) {
	dir := t.TempDir()
	s := NewOverrideStore(dir, nil)
	// 첫 저장(히스토리 없음) + 두 번째 저장(첫 버전이 히스토리로).
	_ = s.Set("remote_management.mode", "client")
	_ = s.Set("remote_management.mode", "disabled")

	histDir := filepath.Join(dir, overridesHistoryDir)
	entries, err := os.ReadDir(histDir)
	if err != nil {
		t.Fatalf("history dir 읽기 실패: %v", err)
	}
	if len(entries) < 1 {
		t.Errorf("두 번째 저장 후 히스토리 파일이 최소 1개 있어야 함, got %d", len(entries))
	}
}

// TestSetPersistent_MutableAppliesImmediately 는 mutable 키가 즉시 Get 에 반영되고
// 변경 콜백을 발화하는지 검증한다.
func TestSetPersistent_MutableAppliesImmediately(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithConfigName("nonexistent"))
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	vc := cfg.(*viperConfig)
	// 오버라이드 스토어를 temp dir 로 교체(격리).
	vc.overrides = NewOverrideStore(dir, nil)

	var fired bool
	unsub := cfg.OnChange("remote_management.mode", func(ChangeEvent) { fired = true })
	defer unsub()

	if err := cfg.SetPersistent("remote_management.mode", "client"); err != nil {
		t.Fatalf("SetPersistent error: %v", err)
	}
	if got := cfg.Get("remote_management.mode"); got != "client" {
		t.Errorf("Get(mode) = %v, want client", got)
	}
	if !fired {
		t.Error("mutable 키 변경 콜백이 발화되어야 함")
	}
	// 파일에 영속화되었는지 확인.
	if _, err := os.Stat(filepath.Join(dir, overridesFileName)); err != nil {
		t.Errorf("오버라이드 파일이 저장되어야 함: %v", err)
	}
}

// TestSetPersistent_NonMutableReflectsWithoutCallback 는 비-mutable 키가 Get 에 반영되되
// 변경 콜백은 발화하지 않는지(재시작 후 적용) 검증한다.
func TestSetPersistent_NonMutableReflectsWithoutCallback(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithConfigName("nonexistent"))
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	vc := cfg.(*viperConfig)
	vc.overrides = NewOverrideStore(dir, nil)

	var fired bool
	unsub := cfg.OnChange("remote_management.enrollment_token", func(ChangeEvent) { fired = true })
	defer unsub()

	if err := cfg.SetPersistent("remote_management.enrollment_token", "tok-123"); err != nil {
		t.Fatalf("SetPersistent error: %v", err)
	}
	if got := cfg.Get("remote_management.enrollment_token"); got != "tok-123" {
		t.Errorf("Get = %v, want tok-123", got)
	}
	if fired {
		t.Error("비-mutable 키는 변경 콜백을 발화하지 않아야 함(재시작 후 적용)")
	}
}

// TestSetPersistent_RejectsNonOverridable 는 allowlist 밖 키를 거부하는지 검증한다.
func TestSetPersistent_RejectsNonOverridable(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(WithConfigPaths(dir), WithConfigName("nonexistent"))
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if err := cfg.SetPersistent("server.port", 9999); err == nil {
		t.Error("allowlist 밖 키(server.port)는 거부되어야 함")
	}
}
