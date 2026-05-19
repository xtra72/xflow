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
// LGCP: nonTempFieldsChangedLGCP + maxTempDeltaLGCP
// ---------------------------------------------------------------------------

func TestNonTempFieldsChangedLGCP(t *testing.T) {
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

	t.Run("IndoorTempC diff returns false (temp ignored)", func(t *testing.T) {
		newTemp := 24.0
		curr := base
		curr.IndoorTempC = &newTemp
		if nonTempFieldsChangedLGCP(base, curr) {
			t.Error("IndoorTempC diff must not count")
		}
	})

	t.Run("Mode diff returns true", func(t *testing.T) {
		heat := "heating"
		curr := base
		curr.Mode = &heat
		if !nonTempFieldsChangedLGCP(base, curr) {
			t.Error("Mode diff must register")
		}
	})

	t.Run("SetTempC diff returns true", func(t *testing.T) {
		newTarget := 26.0
		curr := base
		curr.SetTempC = &newTarget
		if !nonTempFieldsChangedLGCP(base, curr) {
			t.Error("SetTempC diff must register")
		}
	})
}

func TestMaxTempDeltaLGCP(t *testing.T) {
	t.Parallel()
	indoor := 23.5
	pipe1 := 20.0
	pipe2 := 18.0
	prev := LGCPDeviceState{
		IndoorTempC: &indoor,
		PipeTemp1C:  &pipe1,
		PipeTemp2C:  &pipe2,
	}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaLGCP(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("PipeTemp1C diff dominates", func(t *testing.T) {
		newIndoor := 23.6
		newPipe1 := 22.0
		curr := prev
		curr.IndoorTempC = &newIndoor // 0.1
		curr.PipeTemp1C = &newPipe1   // 2.0
		got := maxTempDeltaLGCP(prev, curr)
		if got < 1.99 || got > 2.01 {
			t.Errorf("= %v, want ≈2.0", got)
		}
	})

	t.Run("PipeTemp2C 0.5 diff", func(t *testing.T) {
		newPipe2 := 18.5
		curr := prev
		curr.PipeTemp2C = &newPipe2
		got := maxTempDeltaLGCP(prev, curr)
		if got < 0.49 || got > 0.51 {
			t.Errorf("= %v, want ≈0.5", got)
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
// LGCNP IDU: nonTempFieldsChangedLGCNPIDU + maxTempDeltaLGCNPIDU
// ---------------------------------------------------------------------------

func TestNonTempFieldsChangedLGCNPIDU(t *testing.T) {
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

	t.Run("CurrentTemp diff returns false", func(t *testing.T) {
		curr := base
		curr.CurrentTemp = 24.0
		if nonTempFieldsChangedLGCNPIDU(base, curr) {
			t.Error("CurrentTemp diff must not count")
		}
	})

	t.Run("InletTemp diff returns false (temp sensor)", func(t *testing.T) {
		curr := base
		curr.InletTemp = 22.0
		if nonTempFieldsChangedLGCNPIDU(base, curr) {
			t.Error("InletTemp diff must not count as non-temp")
		}
	})

	t.Run("OutletTemp diff returns false (temp sensor)", func(t *testing.T) {
		curr := base
		curr.OutletTemp = 19.0
		if nonTempFieldsChangedLGCNPIDU(base, curr) {
			t.Error("OutletTemp diff must not count as non-temp")
		}
	})

	t.Run("Power diff returns true", func(t *testing.T) {
		curr := base
		curr.Power = false
		if !nonTempFieldsChangedLGCNPIDU(base, curr) {
			t.Error("Power diff must register")
		}
	})

	t.Run("Mode diff returns true", func(t *testing.T) {
		curr := base
		curr.Mode = 2
		if !nonTempFieldsChangedLGCNPIDU(base, curr) {
			t.Error("Mode diff must register")
		}
	})

	t.Run("FanSpeed diff returns true", func(t *testing.T) {
		curr := base
		curr.FanSpeed = 5
		if !nonTempFieldsChangedLGCNPIDU(base, curr) {
			t.Error("FanSpeed diff must register")
		}
	})

	t.Run("TargetTemp diff returns true", func(t *testing.T) {
		curr := base
		curr.TargetTemp = 26.0
		if !nonTempFieldsChangedLGCNPIDU(base, curr) {
			t.Error("TargetTemp diff must register")
		}
	})
}

func TestMaxTempDeltaLGCNPIDU(t *testing.T) {
	t.Parallel()
	prev := LGCNPIDUParsed{CurrentTemp: 23.5, InletTemp: 20.0, OutletTemp: 18.0}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaLGCNPIDU(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("InletTemp 0.5 diff (CurrentTemp same)", func(t *testing.T) {
		curr := prev
		curr.InletTemp = 20.5
		got := maxTempDeltaLGCNPIDU(prev, curr)
		if got < 0.49 || got > 0.51 {
			t.Errorf("= %v, want ≈0.5 (reproduces user-reported bug)", got)
		}
	})

	t.Run("OutletTemp dominates", func(t *testing.T) {
		curr := prev
		curr.CurrentTemp = 23.6 // 0.1
		curr.OutletTemp = 19.5  // 1.5
		got := maxTempDeltaLGCNPIDU(prev, curr)
		if got < 1.49 || got > 1.51 {
			t.Errorf("= %v, want ≈1.5", got)
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

// ---------------------------------------------------------------------------
// LGCNP ODU: maxTempDeltaLGCNPODU
// ---------------------------------------------------------------------------

func TestMaxTempDeltaLGCNPODU(t *testing.T) {
	t.Parallel()
	out := 30.0
	suc := 15.0
	dis := 60.0
	condA := 40.0
	condB := 41.0
	prev := LGCNPODUParsed{
		OutdoorTemp:       &out,
		CompSuctionTemp:   &suc,
		CompDischargeTemp: &dis,
		CondenserTempA:    &condA,
		CondenserTempB:    &condB,
	}

	t.Run("identical returns 0", func(t *testing.T) {
		if got := maxTempDeltaLGCNPODU(prev, prev); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})

	t.Run("CompDischargeTemp 5 diff dominates", func(t *testing.T) {
		newOut := 30.1
		newDis := 65.0
		curr := prev
		curr.OutdoorTemp = &newOut       // 0.1
		curr.CompDischargeTemp = &newDis // 5.0
		got := maxTempDeltaLGCNPODU(prev, curr)
		if got < 4.99 || got > 5.01 {
			t.Errorf("= %v, want ≈5.0", got)
		}
	})

	t.Run("nil-to-non-nil returns large value", func(t *testing.T) {
		curr := prev
		curr.OutdoorTemp = nil
		got := maxTempDeltaLGCNPODU(prev, curr)
		if got < 1e6 {
			t.Errorf("= %v, want ≥ 1e6 (gate bypass)", got)
		}
	})

	t.Run("both nil returns 0", func(t *testing.T) {
		a := LGCNPODUParsed{}
		b := LGCNPODUParsed{}
		if got := maxTempDeltaLGCNPODU(a, b); got != 0 {
			t.Errorf("= %v, want 0", got)
		}
	})
}
