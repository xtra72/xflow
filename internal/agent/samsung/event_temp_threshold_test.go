package samsung

import (
	"testing"
)

// TestNonTempFieldsChangedHvacr01 는 비온도 필드 변경 감지 헬퍼를 검증한다 (v0.6.7).
func TestNonTempFieldsChangedHvacr01(t *testing.T) {
	t.Parallel()
	base := NasaDeviceState{
		Power:       true,
		Mode:        "cool",
		TargetTemp:  25.0,
		CurrentTemp: 23.5,
		FanSpeed:    "auto",
	}

	t.Run("CurrentTemp diff returns false (temp ignored)", func(t *testing.T) {
		curr := base
		curr.CurrentTemp = 24.0
		if nonTempFieldsChangedHvacr01(base, curr) {
			t.Error("CurrentTemp diff must not count as non-temp change")
		}
	})

	t.Run("Power diff returns true", func(t *testing.T) {
		curr := base
		curr.Power = false
		if !nonTempFieldsChangedHvacr01(base, curr) {
			t.Error("Power diff must register")
		}
	})

	t.Run("Mode diff returns true", func(t *testing.T) {
		curr := base
		curr.Mode = "heat"
		if !nonTempFieldsChangedHvacr01(base, curr) {
			t.Error("Mode diff must register")
		}
	})

	t.Run("TargetTemp diff returns true", func(t *testing.T) {
		curr := base
		curr.TargetTemp = 26.0
		if !nonTempFieldsChangedHvacr01(base, curr) {
			t.Error("TargetTemp diff must register")
		}
	})

	t.Run("FanSpeed diff returns true", func(t *testing.T) {
		curr := base
		curr.FanSpeed = "high"
		if !nonTempFieldsChangedHvacr01(base, curr) {
			t.Error("FanSpeed diff must register")
		}
	})
}

// TestMaxTempDeltaHvacr01 는 실내온도 |Δ| 계산을 검증한다 (v0.6.7).
func TestMaxTempDeltaHvacr01(t *testing.T) {
	t.Parallel()
	prev := NasaDeviceState{CurrentTemp: 23.5}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaHvacr01(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("positive delta", func(t *testing.T) {
		curr := NasaDeviceState{CurrentTemp: 24.5}
		got := maxTempDeltaHvacr01(prev, curr)
		if got < 0.99 || got > 1.01 {
			t.Errorf("= %v, want ≈1.0", got)
		}
	})

	t.Run("negative delta returns absolute value", func(t *testing.T) {
		curr := NasaDeviceState{CurrentTemp: 22.0}
		got := maxTempDeltaHvacr01(prev, curr)
		if got < 1.49 || got > 1.51 {
			t.Errorf("= %v, want ≈1.5 (abs)", got)
		}
	})
}

// TestHvacr01Config_EventTempThreshold_Default 는 옵션 미지정 시 기본 1.0℃ 인지 검증한다.
func TestHvacr01Config_EventTempThreshold_Default(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type": "serial",
		"serial_port":    "/dev/ttyTEST",
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config: %v", err)
	}
	if cfg.EventTempThreshold != 1.0 {
		t.Errorf("default = %v, want 1.0", cfg.EventTempThreshold)
	}
}

// TestHvacr01Config_EventTempThreshold_Custom 는 옵션이 적용되는지 검증한다.
func TestHvacr01Config_EventTempThreshold_Custom(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type":       "serial",
		"serial_port":          "/dev/ttyTEST",
		"event_temp_threshold": 0.5,
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config: %v", err)
	}
	if cfg.EventTempThreshold != 0.5 {
		t.Errorf("= %v, want 0.5", cfg.EventTempThreshold)
	}
}

// TestHvacr01Config_EventTempThreshold_Zero 는 0 일 때 게이트 비활성을 표현하는지 검증한다.
func TestHvacr01Config_EventTempThreshold_Zero(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr01Config(map[string]any{
		"transport_type":       "serial",
		"serial_port":          "/dev/ttyTEST",
		"event_temp_threshold": 0.0,
	})
	if err != nil {
		t.Fatalf("parseHvacr01Config: %v", err)
	}
	if cfg.EventTempThreshold != 0.0 {
		t.Errorf("= %v, want 0.0 (disabled)", cfg.EventTempThreshold)
	}
}
