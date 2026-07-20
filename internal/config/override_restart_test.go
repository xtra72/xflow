package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestOverride_SurvivesRestart 는 SetPersistent 후 새 Load(재시작 모사)에서 오버라이드가
// RemoteManagement() 에 반영되는지 검증한다(자동 등록 미동작 진단).
func TestOverride_SurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "xflow.yaml")
	// 실제 앱처럼 sqlite.path 를 temp 로, remote_management.mode 를 disabled 로 시작.
	yaml := "storage:\n  sqlite:\n    path: " + filepath.Join(dir, "xflow.db") +
		"\nremote_management:\n  mode: disabled\n"
	if err := os.WriteFile(cfgFile, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1) 첫 로드 후 UI 저장을 모사.
	cfg1, err := Load(WithConfigFile(cfgFile))
	if err != nil {
		t.Fatalf("Load1: %v", err)
	}
	for k, v := range map[string]any{
		"remote_management.mode":             "client",
		"remote_management.server_url":       "wss://mgmt.example",
		"remote_management.auto_register":    true,
		"remote_management.enrollment_token": "tok-123",
	} {
		if err := cfg1.SetPersistent(k, v); err != nil {
			t.Fatalf("SetPersistent(%s): %v", k, err)
		}
	}

	// override 파일 위치 로그.
	ovPath := filepath.Join(dir, overridesFileName)
	if _, err := os.Stat(ovPath); err != nil {
		t.Fatalf("override 파일 없음(%s): %v", ovPath, err)
	}

	// 2) 재시작 모사: 완전히 새 Load.
	cfg2, err := Load(WithConfigFile(cfgFile))
	if err != nil {
		t.Fatalf("Load2: %v", err)
	}
	rm := cfg2.RemoteManagement()
	if rm.Mode != "client" {
		t.Errorf("재시작 후 Mode = %q, want client", rm.Mode)
	}
	if rm.ServerURL != "wss://mgmt.example" {
		t.Errorf("재시작 후 ServerURL = %q", rm.ServerURL)
	}
	if !rm.AutoRegister {
		t.Errorf("재시작 후 AutoRegister = false, want true")
	}
	if rm.EnrollmentToken != "tok-123" {
		t.Errorf("재시작 후 EnrollmentToken = %q, want tok-123", rm.EnrollmentToken)
	}
}
