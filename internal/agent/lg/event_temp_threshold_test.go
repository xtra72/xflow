package lg

import (
	"testing"
)

// ---------------------------------------------------------------------------
// LGAP: onlyRoomTempChangedLGAP
// ---------------------------------------------------------------------------

func TestOnlyRoomTempChangedLGAP(t *testing.T) {
	t.Parallel()
	base := LGAPDeviceState{
		Power:      true,
		Mode:       "cool",
		TargetTemp: 25.0,
		RoomTemp:   23.5,
		FanSpeed:   "auto",
		ErrorCode:  0,
	}

	t.Run("only RoomTemp differs returns true", func(t *testing.T) {
		curr := base
		curr.RoomTemp = 24.0
		if !onlyRoomTempChangedLGAP(base, curr) {
			t.Error("expected true when only RoomTemp differs")
		}
	})

	t.Run("Power differs returns false", func(t *testing.T) {
		curr := base
		curr.Power = false
		if onlyRoomTempChangedLGAP(base, curr) {
			t.Error("expected false when Power differs")
		}
	})

	t.Run("Mode differs returns false", func(t *testing.T) {
		curr := base
		curr.Mode = "heat"
		if onlyRoomTempChangedLGAP(base, curr) {
			t.Error("expected false when Mode differs")
		}
	})

	t.Run("TargetTemp differs returns false", func(t *testing.T) {
		curr := base
		curr.TargetTemp = 26.0
		if onlyRoomTempChangedLGAP(base, curr) {
			t.Error("expected false when TargetTemp differs")
		}
	})

	t.Run("FanSpeed differs returns false", func(t *testing.T) {
		curr := base
		curr.FanSpeed = "high"
		if onlyRoomTempChangedLGAP(base, curr) {
			t.Error("expected false when FanSpeed differs")
		}
	})

	t.Run("ErrorCode differs returns false", func(t *testing.T) {
		curr := base
		curr.ErrorCode = 0x01
		if onlyRoomTempChangedLGAP(base, curr) {
			t.Error("expected false when ErrorCode differs")
		}
	})
}

func TestLGAPConfig_EventTempThreshold_Default(t *testing.T) {
	t.Parallel()
	cfg, err := parseLGAPConfig(map[string]any{"serial_port": "/dev/ttyTEST"})
	if err != nil {
		t.Fatalf("parseLGAPConfig: %v", err)
	}
	if cfg.EventTempThreshold != 1.0 {
		t.Errorf("default = %v, want 1.0", cfg.EventTempThreshold)
	}
}

func TestLGAPConfig_EventTempThreshold_Custom(t *testing.T) {
	t.Parallel()
	cfg, err := parseLGAPConfig(map[string]any{
		"serial_port":          "/dev/ttyTEST",
		"event_temp_threshold": 0.5,
	})
	if err != nil {
		t.Fatalf("parseLGAPConfig: %v", err)
	}
	if cfg.EventTempThreshold != 0.5 {
		t.Errorf("= %v, want 0.5", cfg.EventTempThreshold)
	}
}

// ---------------------------------------------------------------------------
// LGCP: onlyIndoorTempChangedLGCP
// ---------------------------------------------------------------------------

func TestOnlyIndoorTempChangedLGCP(t *testing.T) {
	t.Parallel()
	powerOn := "on"
	modeCool := "cooling"
	fan := "auto"
	target := 25.0
	indoor := 23.5

	base := LGCPDeviceState{
		PowerState:  &powerOn,
		Power:       &powerOn,
		Mode:        &modeCool,
		FanSpeed:    &fan,
		SetTempC:    &target,
		IndoorTempC: &indoor,
	}

	t.Run("only IndoorTempC differs returns true", func(t *testing.T) {
		newTemp := 24.0
		curr := base
		curr.IndoorTempC = &newTemp
		if !onlyIndoorTempChangedLGCP(base, curr) {
			t.Error("expected true when only IndoorTempC differs")
		}
	})

	t.Run("Mode differs returns false", func(t *testing.T) {
		heat := "heating"
		curr := base
		curr.Mode = &heat
		if onlyIndoorTempChangedLGCP(base, curr) {
			t.Error("expected false when Mode differs")
		}
	})

	t.Run("Power differs returns false", func(t *testing.T) {
		off := "off"
		curr := base
		curr.Power = &off
		if onlyIndoorTempChangedLGCP(base, curr) {
			t.Error("expected false when Power differs")
		}
	})

	t.Run("SetTempC differs returns false", func(t *testing.T) {
		newTarget := 26.0
		curr := base
		curr.SetTempC = &newTarget
		if onlyIndoorTempChangedLGCP(base, curr) {
			t.Error("expected false when SetTempC differs")
		}
	})
}

func TestLGCPConfig_EventTempThreshold_Default(t *testing.T) {
	t.Parallel()
	cfg, err := parseLGCPConfig(map[string]any{
		"transport_type": "serial",
		"serial_port":    "/dev/ttyTEST",
	})
	if err != nil {
		t.Fatalf("parseLGCPConfig: %v", err)
	}
	if cfg.EventTempThreshold != 1.0 {
		t.Errorf("default = %v, want 1.0", cfg.EventTempThreshold)
	}
}

// ---------------------------------------------------------------------------
// LGCNP: onlyCurrentTempChangedLGCNP
// ---------------------------------------------------------------------------

func TestOnlyCurrentTempChangedLGCNP(t *testing.T) {
	t.Parallel()
	base := LGCNPIDUParsed{
		Power:       true,
		TargetTemp:  25.0,
		CurrentTemp: 23.5,
		InletTemp:   20.0,
		OutletTemp:  18.0,
		FanSpeed:    2,
		Mode:        1,
	}

	t.Run("only CurrentTemp differs returns true", func(t *testing.T) {
		curr := base
		curr.CurrentTemp = 24.0
		if !onlyCurrentTempChangedLGCNP(base, curr) {
			t.Error("expected true when only CurrentTemp differs")
		}
	})

	t.Run("Power differs returns false", func(t *testing.T) {
		curr := base
		curr.Power = false
		if onlyCurrentTempChangedLGCNP(base, curr) {
			t.Error("expected false when Power differs")
		}
	})

	t.Run("TargetTemp differs returns false", func(t *testing.T) {
		curr := base
		curr.TargetTemp = 26.0
		if onlyCurrentTempChangedLGCNP(base, curr) {
			t.Error("expected false when TargetTemp differs")
		}
	})

	t.Run("Mode differs returns false", func(t *testing.T) {
		curr := base
		curr.Mode = 2
		if onlyCurrentTempChangedLGCNP(base, curr) {
			t.Error("expected false when Mode differs")
		}
	})

	t.Run("FanSpeed differs returns false", func(t *testing.T) {
		curr := base
		curr.FanSpeed = 5
		if onlyCurrentTempChangedLGCNP(base, curr) {
			t.Error("expected false when FanSpeed differs")
		}
	})

	t.Run("InletTemp differs returns false", func(t *testing.T) {
		curr := base
		curr.InletTemp = 22.0
		if onlyCurrentTempChangedLGCNP(base, curr) {
			t.Error("expected false when InletTemp differs")
		}
	})
}

func TestLGCNPConfig_EventTempThreshold_Default(t *testing.T) {
	t.Parallel()
	cfg, err := parseLGCNPConfig(map[string]any{
		"transport_type": "serial",
		"serial_port":    "/dev/ttyTEST",
	})
	if err != nil {
		t.Fatalf("parseLGCNPConfig: %v", err)
	}
	if cfg.EventTempThreshold != 1.0 {
		t.Errorf("default = %v, want 1.0", cfg.EventTempThreshold)
	}
}

func TestLGCNPConfig_EventTempThreshold_Custom(t *testing.T) {
	t.Parallel()
	cfg, err := parseLGCNPConfig(map[string]any{
		"transport_type":       "serial",
		"serial_port":          "/dev/ttyTEST",
		"event_temp_threshold": 0.5,
	})
	if err != nil {
		t.Fatalf("parseLGCNPConfig: %v", err)
	}
	if cfg.EventTempThreshold != 0.5 {
		t.Errorf("= %v, want 0.5", cfg.EventTempThreshold)
	}
}
