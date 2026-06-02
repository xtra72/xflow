package samsung

import (
	"testing"
)

// TestParseHvacr01Config_StatusQueryEnabled 는 v0.6.1 의 신규 옵션
// status_query_enabled 파싱을 검증한다.
//
// 사용자 요구 "Nasa 상태 확인 요청 옵션" — passive sniff only 모드 지원.
func TestParseHvacr01Config_StatusQueryEnabled(t *testing.T) {
	t.Run("default is true", func(t *testing.T) {
		cfg, err := parseHvacr01Config(map[string]any{
			"transport_type": "serial",
			"serial_port":    "/dev/ttyTEST",
		})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if !cfg.StatusQueryEnabled {
			t.Errorf("default StatusQueryEnabled = false, want true (기존 동작 보존)")
		}
	})

	t.Run("explicit false (passive sniff only)", func(t *testing.T) {
		cfg, err := parseHvacr01Config(map[string]any{
			"transport_type":       "serial",
			"serial_port":          "/dev/ttyTEST",
			"status_query_enabled": false,
		})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if cfg.StatusQueryEnabled {
			t.Errorf("StatusQueryEnabled = true, want false (사용자 명시 disable)")
		}
	})

	t.Run("explicit true (확인)", func(t *testing.T) {
		cfg, err := parseHvacr01Config(map[string]any{
			"transport_type":       "serial",
			"serial_port":          "/dev/ttyTEST",
			"status_query_enabled": true,
		})
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if !cfg.StatusQueryEnabled {
			t.Errorf("StatusQueryEnabled = false, want true")
		}
	})
}
