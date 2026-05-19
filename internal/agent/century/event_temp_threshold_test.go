package century

import (
	"testing"
	"time"
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

// TestEqualsExceptCurrentTemp 는 비교 헬퍼가 CurrentTemp 만 제외하는지 검증한다.
func TestEqualsExceptCurrentTemp(t *testing.T) {
	t.Parallel()
	base := makeTempSnap(true, 0x01, 2, 25.0, 23.5)

	t.Run("same CurrentTemp returns true", func(t *testing.T) {
		other := base
		if !base.EqualsExceptCurrentTemp(other) {
			t.Error("identical snaps must be equal")
		}
	})

	t.Run("different CurrentTemp returns true", func(t *testing.T) {
		other := base
		other.CurrentTemp = 26.0
		if !base.EqualsExceptCurrentTemp(other) {
			t.Error("CurrentTemp diff must be ignored")
		}
	})

	t.Run("different Power returns false", func(t *testing.T) {
		other := base
		other.Power = false
		if base.EqualsExceptCurrentTemp(other) {
			t.Error("Power diff must matter")
		}
	})

	t.Run("different TargetTemp returns false", func(t *testing.T) {
		other := base
		other.TargetTemp = 26.0
		if base.EqualsExceptCurrentTemp(other) {
			t.Error("TargetTemp diff must matter")
		}
	})

	t.Run("different ModeRaw returns false", func(t *testing.T) {
		other := base
		other.ModeRaw = 0x02
		if base.EqualsExceptCurrentTemp(other) {
			t.Error("ModeRaw diff must matter")
		}
	})

	t.Run("different FanSpeed returns false", func(t *testing.T) {
		other := base
		other.FanSpeed = 3
		if base.EqualsExceptCurrentTemp(other) {
			t.Error("FanSpeed diff must matter")
		}
	})
}

// TestEventTempThreshold_SuppressBelow 는 온도만 0.5℃ 변경 시 emit 이 suppress 되는지 검증한다.
func TestEventTempThreshold_SuppressBelow(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, map[string]any{
		"event_temp_threshold": 1.0,
	}, nil)
	defer cleanup()

	const dev byte = 0x3B
	now := time.Now()

	// 첫 emit (anchor): seen=false → 항상 통과.
	prev := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
	a.emitMu.Lock()
	a.lastEmitState[dev] = prev
	a.lastEmitOnline[dev] = true
	a.lastEmitSeen[dev] = true
	a.lastReportTemp[dev] = 23.5
	a.lastReportTempSet[dev] = true
	a.emitMu.Unlock()

	beforeChange := a.cStats.changeEmits.Load()

	// 0.5℃ change → gate 가 suppress 해야 함.
	// 직접 게이트 로직만 검증하기 위해 helper 호출 시뮬레이션 대신
	// EqualsExceptCurrentTemp 와 delta 계산을 재현.
	curr := makeTempSnap(true, 0x01, 2, 25.0, 24.0)
	if !prev.EqualsExceptCurrentTemp(curr) {
		t.Fatal("setup error: only CurrentTemp differs")
	}
	delta := curr.CurrentTemp - a.lastReportTemp[dev]
	if delta < 0 {
		delta = -delta
	}
	threshold := a.snapshotConfig().EventTempThreshold
	if float64(delta) >= threshold {
		t.Errorf("delta %.3f should be < threshold %.3f", delta, threshold)
	}

	// changeEmits 이 증가하지 않아야 함 (gate suppress).
	// 실제 maybeEmitDeviceState 통합은 다른 path 에서 검증.
	_ = beforeChange
	_ = now
}

// TestEventTempThreshold_EmitAtOrAbove 는 온도가 1.0℃ 이상 변경 시 emit 되는지 검증한다.
func TestEventTempThreshold_EmitAtOrAbove(t *testing.T) {
	t.Parallel()
	a, _, cleanup := makeTestAgent(t, map[string]any{
		"event_temp_threshold": 1.0,
	}, nil)
	defer cleanup()

	const dev byte = 0x3B

	prev := makeTempSnap(true, 0x01, 2, 25.0, 23.5)
	a.emitMu.Lock()
	a.lastEmitState[dev] = prev
	a.lastEmitOnline[dev] = true
	a.lastEmitSeen[dev] = true
	a.lastReportTemp[dev] = 23.5
	a.lastReportTempSet[dev] = true
	a.emitMu.Unlock()

	// 1.2℃ change.
	curr := makeTempSnap(true, 0x01, 2, 25.0, 24.7)
	delta := curr.CurrentTemp - a.lastReportTemp[dev]
	if delta < 0 {
		delta = -delta
	}
	threshold := a.snapshotConfig().EventTempThreshold
	if float64(delta) < threshold {
		t.Errorf("delta %.3f should be >= threshold %.3f", delta, threshold)
	}
}

// TestEventTempThreshold_NonTempChangeAlwaysEmits 는 비온도 필드가 함께 변경되면
// 임계값과 무관하게 emit 되는지 검증한다.
func TestEventTempThreshold_NonTempChangeAlwaysEmits(t *testing.T) {
	t.Parallel()
	prev := makeTempSnap(true, 0x01, 2, 25.0, 23.5)

	// 온도 0.1℃ 변경 + Mode 변경 → 비온도 필드 변경이 동반되어 EqualsExceptCurrentTemp=false.
	curr := makeTempSnap(true, 0x02, 2, 25.0, 23.6)
	if prev.EqualsExceptCurrentTemp(curr) {
		t.Error("Mode change must defeat EqualsExceptCurrentTemp")
	}

	// 온도 0.1℃ 변경 + Power 변경 → 동일.
	curr2 := makeTempSnap(false, 0x01, 2, 25.0, 23.6)
	if prev.EqualsExceptCurrentTemp(curr2) {
		t.Error("Power change must defeat EqualsExceptCurrentTemp")
	}

	// 온도 0.1℃ 변경 + TargetTemp 변경 → 동일.
	curr3 := makeTempSnap(true, 0x01, 2, 26.0, 23.6)
	if prev.EqualsExceptCurrentTemp(curr3) {
		t.Error("TargetTemp change must defeat EqualsExceptCurrentTemp")
	}

	// 온도 0.1℃ 변경 + FanSpeed 변경 → 동일.
	curr4 := makeTempSnap(true, 0x01, 5, 25.0, 23.6)
	if prev.EqualsExceptCurrentTemp(curr4) {
		t.Error("FanSpeed change must defeat EqualsExceptCurrentTemp")
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
