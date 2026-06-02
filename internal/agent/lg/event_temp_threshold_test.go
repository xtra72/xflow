package lg

import (
	"testing"
)

// ---------------------------------------------------------------------------
// LGAP: nonTempFieldsChangedLGAP + maxTempDeltaLGAP
// ---------------------------------------------------------------------------

func TestNonTempFieldsChangedLGAP(t *testing.T) {
	t.Parallel()
	base := LGAPDeviceState{
		Power:      true,
		Mode:       "cool",
		TargetTemp: 25,
		RoomTemp:   23.5,
		FanSpeed:   "auto",
		ErrorCode:  0,
	}

	t.Run("RoomTemp diff returns false (temp ignored)", func(t *testing.T) {
		curr := base
		curr.RoomTemp = 24.0
		if nonTempFieldsChangedLGAP(base, curr) {
			t.Error("RoomTemp diff must not count")
		}
	})

	t.Run("Power diff returns true", func(t *testing.T) {
		curr := base
		curr.Power = false
		if !nonTempFieldsChangedLGAP(base, curr) {
			t.Error("Power diff must register")
		}
	})

	t.Run("ErrorCode diff returns true", func(t *testing.T) {
		curr := base
		curr.ErrorCode = 0x01
		if !nonTempFieldsChangedLGAP(base, curr) {
			t.Error("ErrorCode diff must register")
		}
	})
}

func TestMaxTempDeltaLGAP(t *testing.T) {
	t.Parallel()
	prev := LGAPDeviceState{RoomTemp: 23.5, PipeInTemp: 20.0, PipeOutTemp: 18.0}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaLGAP(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("PipeInTemp diff dominates", func(t *testing.T) {
		curr := prev
		curr.RoomTemp = 23.6   // 0.1
		curr.PipeInTemp = 22.0 // 2.0
		got := maxTempDeltaLGAP(prev, curr)
		if got < 1.99 || got > 2.01 {
			t.Errorf("= %v, want ≈2.0", got)
		}
	})

	t.Run("PipeOutTemp diff", func(t *testing.T) {
		curr := prev
		curr.PipeOutTemp = 18.5
		got := maxTempDeltaLGAP(prev, curr)
		if got < 0.49 || got > 0.51 {
			t.Errorf("= %v, want ≈0.5", got)
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
// LG ICP-02: nonTempFieldsChangedIcp02 + maxTempDeltaIcp02
// ---------------------------------------------------------------------------

func TestNonTempFieldsChangedIcp02(t *testing.T) {
	t.Parallel()
	powerOn := "on"
	modeCool := "cooling"
	fan := "auto"
	target := 25.0
	indoor := 23.5

	base := Icp02DeviceState{
		PowerState:  &powerOn,
		Power:       &powerOn,
		Mode:        &modeCool,
		FanSpeed:    &fan,
		SetTempC:    &target,
		IndoorTempC: &indoor,
	}

	t.Run("IndoorTempC diff returns false (temp ignored)", func(t *testing.T) {
		newTemp := 24.0
		curr := base
		curr.IndoorTempC = &newTemp
		if nonTempFieldsChangedIcp02(base, curr) {
			t.Error("IndoorTempC diff must not count")
		}
	})

	t.Run("Mode diff returns true", func(t *testing.T) {
		heat := "heating"
		curr := base
		curr.Mode = &heat
		if !nonTempFieldsChangedIcp02(base, curr) {
			t.Error("Mode diff must register")
		}
	})

	t.Run("SetTempC diff returns true", func(t *testing.T) {
		newTarget := 26.0
		curr := base
		curr.SetTempC = &newTarget
		if !nonTempFieldsChangedIcp02(base, curr) {
			t.Error("SetTempC diff must register")
		}
	})
}

func TestMaxTempDeltaIcp02(t *testing.T) {
	t.Parallel()
	indoor := 23.5
	pipe1 := 20.0
	pipe2 := 18.0
	prev := Icp02DeviceState{
		IndoorTempC: &indoor,
		PipeTemp1C:  &pipe1,
		PipeTemp2C:  &pipe2,
	}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaIcp02(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("PipeTemp1C diff dominates", func(t *testing.T) {
		newIndoor := 23.6
		newPipe1 := 22.0
		curr := prev
		curr.IndoorTempC = &newIndoor // 0.1
		curr.PipeTemp1C = &newPipe1   // 2.0
		got := maxTempDeltaIcp02(prev, curr)
		if got < 1.99 || got > 2.01 {
			t.Errorf("= %v, want ≈2.0", got)
		}
	})

	t.Run("PipeTemp2C 0.5 diff", func(t *testing.T) {
		newPipe2 := 18.5
		curr := prev
		curr.PipeTemp2C = &newPipe2
		got := maxTempDeltaIcp02(prev, curr)
		if got < 0.49 || got > 0.51 {
			t.Errorf("= %v, want ≈0.5", got)
		}
	})
}

func TestHvacr02Config_EventTempThreshold_Default(t *testing.T) {
	t.Parallel()
	cfg, err := parseHvacr02Config(map[string]any{
		"transport_type": "serial",
		"serial_port":    "/dev/ttyTEST",
	})
	if err != nil {
		t.Fatalf("parseHvacr02Config: %v", err)
	}
	if cfg.EventTempThreshold != 1.0 {
		t.Errorf("default = %v, want 1.0", cfg.EventTempThreshold)
	}
}

// ---------------------------------------------------------------------------
// HVACR-01 IDU: nonTempFieldsChangedHvacr01IDU + maxTempDeltaHvacr01IDU
// ---------------------------------------------------------------------------

func TestNonTempFieldsChangedHvacr01IDU(t *testing.T) {
	t.Parallel()
	base := Hvacr01IDUParsed{
		Power:       true,
		TargetTemp:  25.0,
		CurrentTemp: 23.5,
		InletTemp:   20.0,
		OutletTemp:  18.0,
		FanSpeed:    3, // hvac.FanLow
		Mode:        3, // hvac.ModeDry
	}

	t.Run("CurrentTemp diff returns false", func(t *testing.T) {
		curr := base
		curr.CurrentTemp = 24.0
		if nonTempFieldsChangedHvacr01IDU(base, curr) {
			t.Error("CurrentTemp diff must not count")
		}
	})

	t.Run("InletTemp diff returns false (temp sensor)", func(t *testing.T) {
		curr := base
		curr.InletTemp = 22.0
		if nonTempFieldsChangedHvacr01IDU(base, curr) {
			t.Error("InletTemp diff must not count as non-temp")
		}
	})

	t.Run("OutletTemp diff returns false (temp sensor)", func(t *testing.T) {
		curr := base
		curr.OutletTemp = 19.0
		if nonTempFieldsChangedHvacr01IDU(base, curr) {
			t.Error("OutletTemp diff must not count as non-temp")
		}
	})

	t.Run("Power diff returns true", func(t *testing.T) {
		curr := base
		curr.Power = false
		if !nonTempFieldsChangedHvacr01IDU(base, curr) {
			t.Error("Power diff must register")
		}
	})

	t.Run("Mode diff returns true", func(t *testing.T) {
		curr := base
		curr.Mode = 4 // hvac.ModeFan
		if !nonTempFieldsChangedHvacr01IDU(base, curr) {
			t.Error("Mode diff must register")
		}
	})

	t.Run("FanSpeed diff returns true", func(t *testing.T) {
		curr := base
		curr.FanSpeed = 6 // hvac.FanTurbo
		if !nonTempFieldsChangedHvacr01IDU(base, curr) {
			t.Error("FanSpeed diff must register")
		}
	})

	t.Run("TargetTemp diff returns true", func(t *testing.T) {
		curr := base
		curr.TargetTemp = 26.0
		if !nonTempFieldsChangedHvacr01IDU(base, curr) {
			t.Error("TargetTemp diff must register")
		}
	})
}

func TestMaxTempDeltaHvacr01IDU(t *testing.T) {
	t.Parallel()
	prev := Hvacr01IDUParsed{CurrentTemp: 23.5, InletTemp: 20.0, OutletTemp: 18.0}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaHvacr01IDU(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("InletTemp 0.5 diff (CurrentTemp same)", func(t *testing.T) {
		curr := prev
		curr.InletTemp = 20.5
		got := maxTempDeltaHvacr01IDU(prev, curr)
		if got < 0.49 || got > 0.51 {
			t.Errorf("= %v, want ≈0.5 (reproduces user-reported bug)", got)
		}
	})

	t.Run("OutletTemp dominates", func(t *testing.T) {
		curr := prev
		curr.CurrentTemp = 23.6 // 0.1
		curr.OutletTemp = 19.5  // 1.5
		got := maxTempDeltaHvacr01IDU(prev, curr)
		if got < 1.49 || got > 1.51 {
			t.Errorf("= %v, want ≈1.5", got)
		}
	})
}

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

// ---------------------------------------------------------------------------
// HVACR-01 ODU: maxTempDeltaHvacr01ODU
// ---------------------------------------------------------------------------

func TestMaxTempDeltaHvacr01ODU(t *testing.T) {
	t.Parallel()
	out := 30.0
	suc := 15.0
	dis := 60.0
	condA := 40.0
	condB := 41.0
	prev := Hvacr01ODUParsed{
		OutdoorTemp:       &out,
		CompSuctionTemp:   &suc,
		CompDischargeTemp: &dis,
		CondenserTempA:    &condA,
		CondenserTempB:    &condB,
	}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaHvacr01ODU(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("CompDischargeTemp 5 diff dominates", func(t *testing.T) {
		newOut := 30.1
		newDis := 65.0
		curr := prev
		curr.OutdoorTemp = &newOut       // 0.1
		curr.CompDischargeTemp = &newDis // 5.0
		got := maxTempDeltaHvacr01ODU(prev, curr)
		if got < 4.99 || got > 5.01 {
			t.Errorf("= %v, want ≈5.0", got)
		}
	})

	t.Run("nil-to-non-nil returns large value", func(t *testing.T) {
		curr := prev
		curr.OutdoorTemp = nil
		got := maxTempDeltaHvacr01ODU(prev, curr)
		if got < 1e6 {
			t.Errorf("= %v, want ≥ 1e6 (gate bypass)", got)
		}
	})

	t.Run("both nil returns 0", func(t *testing.T) {
		a := Hvacr01ODUParsed{}
		b := Hvacr01ODUParsed{}
		if got := maxTempDeltaHvacr01ODU(a, b); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})
}
