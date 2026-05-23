package century

import (
	"testing"
)

// helper: 5 core fields 가 모두 관측된 합법 snapshot 생성.
func makeTempSnap(power bool, modeRaw byte, fan uint8, target, current float32) CenturyDeviceStateSnapshot {
	return CenturyDeviceStateSnapshot{
		Power:       power,
		Mode:        "cool",
		ModeRaw:     modeRaw,
		FanSpeed:    fan,
		TargetTemp:  target,
		CurrentTemp: current,
		Online:      true,
	}
}

// TestNonTempFieldsChangedCentury 는 비온도 필드 변경 감지 헬퍼를 검증한다 (v0.6.7).
func TestNonTempFieldsChangedCentury(t *testing.T) {
	t.Parallel()
	base := makeTempSnap(true, 0x01, 2, 25.0, 23.5)

	t.Run("identical returns false", func(t *testing.T) {
		other := base
		if base.NonTempFieldsChanged(other) {
			t.Error("identical snaps must not differ")
		}
	})

	t.Run("CurrentTemp diff returns false (temp ignored)", func(t *testing.T) {
		other := base
		other.CurrentTemp = 26.0
		if base.NonTempFieldsChanged(other) {
			t.Error("CurrentTemp diff must not count as non-temp change")
		}
	})

	t.Run("Power diff returns true", func(t *testing.T) {
		other := base
		other.Power = false
		if !base.NonTempFieldsChanged(other) {
			t.Error("Power diff must register")
		}
	})

	t.Run("TargetTemp diff returns true", func(t *testing.T) {
		other := base
		other.TargetTemp = 26.0
		if !base.NonTempFieldsChanged(other) {
			t.Error("TargetTemp diff must register (TargetTemp is control field)")
		}
	})

	t.Run("ModeRaw diff returns true", func(t *testing.T) {
		other := base
		other.ModeRaw = 0x02
		if !base.NonTempFieldsChanged(other) {
			t.Error("ModeRaw diff must register")
		}
	})

	t.Run("FanSpeed diff returns true", func(t *testing.T) {
		other := base
		other.FanSpeed = 3
		if !base.NonTempFieldsChanged(other) {
			t.Error("FanSpeed diff must register")
		}
	})
}

// TestMaxTempDeltaCentury 는 모든 온도 센서값의 최대 |Δ| 를 반환하는지 검증한다 (v0.6.7).
func TestMaxTempDeltaCentury(t *testing.T) {
	t.Parallel()

	t.Run("identical returns zero", func(t *testing.T) {
		a := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
		if got := a.MaxTempDelta(a); got != 0 {
			t.Errorf("identical = %v, want 0", got)
		}
	})

	t.Run("CurrentTemp diff", func(t *testing.T) {
		a := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
		b := makeTempSnap(true, 0x01, 2, 25.0, 24.5)
		got := a.MaxTempDelta(b)
		if got < 0.99 || got > 1.01 {
			t.Errorf("CurrentTemp 1.0℃ diff = %v, want ≈1.0", got)
		}
	})

	t.Run("TempEvap diff dominates", func(t *testing.T) {
		evapA := float32(15.0)
		evapB := float32(17.0)
		a := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
		a.EvaporatorTemperatureA = &evapA
		b := makeTempSnap(true, 0x01, 2, 25.0, 23.6) // current 0.1
		b.EvaporatorTemperatureA = &evapB            // evap 2.0
		got := a.MaxTempDelta(b)
		if got < 1.99 || got > 2.01 {
			t.Errorf("EvaporatorTemperatureA 2.0℃ diff (current 0.1) = %v, want ≈2.0", got)
		}
	})

	t.Run("EvaporatorTemperatureB diff", func(t *testing.T) {
		evapA := float32(15.0)
		evapB := float32(15.7)
		a := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
		a.EvaporatorTemperatureB = &evapA
		b := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
		b.EvaporatorTemperatureB = &evapB
		got := a.MaxTempDelta(b)
		if got < 0.69 || got > 0.71 {
			t.Errorf("EvaporatorTemperatureB 0.7℃ diff = %v, want ≈0.7", got)
		}
	})

	t.Run("nil to non-nil returns large value", func(t *testing.T) {
		evapB := float32(15.0)
		a := makeTempSnap(true, 0x01, 2, 25.0, 23.5) // EvaporatorTemperatureA = nil
		b := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
		b.EvaporatorTemperatureA = &evapB
		if got := a.MaxTempDelta(b); got < 1e6 {
			t.Errorf("nil → non-nil = %v, want ≥ 1e6 (gate bypass)", got)
		}
	})
}

// TestEventTempThreshold_NonTempChangeAlwaysEmits 는 비온도 필드가 함께 변경되면
// 임계값과 무관하게 emit 되는지 검증한다.
func TestEventTempThreshold_NonTempChangeAlwaysEmits(t *testing.T) {
	t.Parallel()
	prev := makeTempSnap(true, 0x01, 2, 25.0, 23.5)

	// 온도 0.1℃ 변경 + Mode 변경 → NonTempFieldsChanged=true.
	curr := makeTempSnap(true, 0x02, 2, 25.0, 23.6)
	if !prev.NonTempFieldsChanged(curr) {
		t.Error("Mode change must trigger NonTempFieldsChanged")
	}

	// 온도 0.1℃ 변경 + Power 변경 → 동일.
	curr2 := makeTempSnap(false, 0x01, 2, 25.0, 23.6)
	if !prev.NonTempFieldsChanged(curr2) {
		t.Error("Power change must trigger NonTempFieldsChanged")
	}

	// 온도 0.1℃ 변경 + TargetTemp 변경 → 동일.
	curr3 := makeTempSnap(true, 0x01, 2, 26.0, 23.6)
	if !prev.NonTempFieldsChanged(curr3) {
		t.Error("TargetTemp change must trigger NonTempFieldsChanged")
	}

	// 온도 0.1℃ 변경 + FanSpeed 변경 → 동일.
	curr4 := makeTempSnap(true, 0x01, 5, 25.0, 23.6)
	if !prev.NonTempFieldsChanged(curr4) {
		t.Error("FanSpeed change must trigger NonTempFieldsChanged")
	}
}

// TestEventTempThreshold_DisabledWhenZero 는 threshold=0 일 때 gate 가 비활성인지 검증한다.
func TestEventTempThreshold_DisabledWhenZero(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, map[string]any{
		"event_temp_threshold": 0.0,
	}, nil)
	defer cleanup()

	if got := a.snapshotConfig().EventTempThreshold; got != 0 {
		t.Errorf("threshold = %v, want 0 (disabled)", got)
	}
}

// TestEventTempThreshold_DefaultIsOne 는 옵션 미지정 시 기본 1.0℃ 인지 검증한다.
func TestEventTempThreshold_DefaultIsOne(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, nil, nil)
	defer cleanup()

	if got := a.snapshotConfig().EventTempThreshold; got != 1.0 {
		t.Errorf("default threshold = %v, want 1.0", got)
	}
}
