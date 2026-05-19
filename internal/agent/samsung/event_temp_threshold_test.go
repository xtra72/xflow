package samsung

import (
	"testing"
)

// TestOnlyCurrentTempChangedNASA 는 헬퍼가 CurrentTemp 만 다른 경우를 정확히 식별하는지 검증한다.
func TestOnlyCurrentTempChangedNASA(t *testing.T) {
	t.Parallel()
	base := NASADeviceState{
		Power:       true,
		Mode:        "cool",
		TargetTemp:  25.0,
		CurrentTemp: 23.5,
		FanSpeed:    "auto",
	}

	t.Run("only CurrentTemp differs returns true", func(t *testing.T) {
		curr := base
		curr.CurrentTemp = 24.0
		if !onlyCurrentTempChangedNASA(base, curr) {
			t.Error("expected true when only CurrentTemp differs")
		}
	})

	t.Run("Power differs returns false", func(t *testing.T) {
		curr := base
		curr.Power = false
		curr.CurrentTemp = 24.0
		if onlyCurrentTempChangedNASA(base, curr) {
			t.Error("expected false when Power differs")
		}
	})

	t.Run("Mode differs returns false", func(t *testing.T) {
		curr := base
		curr.Mode = "heat"
		curr.CurrentTemp = 24.0
		if onlyCurrentTempChangedNASA(base, curr) {
			t.Error("expected false when Mode differs")
		}
	})

	t.Run("TargetTemp differs returns false", func(t *testing.T) {
		curr := base
		curr.TargetTemp = 26.0
		curr.CurrentTemp = 24.0
		if onlyCurrentTempChangedNASA(base, curr) {
			t.Error("expected false when TargetTemp differs")
		}
	})

	t.Run("FanSpeed differs returns false", func(t *testing.T) {
		curr := base
		curr.FanSpeed = "high"
		curr.CurrentTemp = 24.0
		if onlyCurrentTempChangedNASA(base, curr) {
			t.Error("expected false when FanSpeed differs")
		}
	})
}

// TestNASAConfig_EventTempThreshold_Default 는 옵션 미지정 시 기본 1.0℃ 인지 검증한다.
func TestNASAConfig_EventTempThreshold_Default(t *testing.T) {
	t.Parallel()
	cfg, err := parseNASAConfig(map[string]any{
		"transport_type": "serial",
		"serial_port":    "/dev/ttyTEST",
	})
	if err != nil {
		t.Fatalf("parseNASAConfig: %v", err)
	}
	if cfg.EventTempThreshold != 1.0 {
		t.Errorf("default EventTempThreshold = %v, want 1.0", cfg.EventTempThreshold)
	}
}

// TestNASAConfig_EventTempThreshold_Custom 는 옵션이 적용되는지 검증한다.
func TestNASAConfig_EventTempThreshold_Custom(t *testing.T) {
	t.Parallel()
	cfg, err := parseNASAConfig(map[string]any{
		"transport_type":       "serial",
		"serial_port":          "/dev/ttyTEST",
		"event_temp_threshold": 0.5,
	})
	if err != nil {
		t.Fatalf("parseNASAConfig: %v", err)
	}
	if cfg.EventTempThreshold != 0.5 {
		t.Errorf("EventTempThreshold = %v, want 0.5", cfg.EventTempThreshold)
	}
}

// TestNASAConfig_EventTempThreshold_Zero 는 0 일 때 게이트 비활성을 표현하는지 검증한다.
func TestNASAConfig_EventTempThreshold_Zero(t *testing.T) {
	t.Parallel()
	cfg, err := parseNASAConfig(map[string]any{
		"transport_type":       "serial",
		"serial_port":          "/dev/ttyTEST",
		"event_temp_threshold": 0.0,
	})
	if err != nil {
		t.Fatalf("parseNASAConfig: %v", err)
	}
	if cfg.EventTempThreshold != 0.0 {
		t.Errorf("EventTempThreshold = %v, want 0.0 (disabled)", cfg.EventTempThreshold)
	}
}
