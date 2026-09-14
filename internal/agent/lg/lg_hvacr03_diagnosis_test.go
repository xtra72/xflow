package lg

import (
	"testing"

	"github.com/xtra/xflow/internal/agent/hvac"
)

// ---------------------------------------------------------------------------
// 잠금 진단 (AC-067)
// ---------------------------------------------------------------------------

// TestHvacr03_CoilLockDiagnosis 는 코일 쓰기가 리모컨 잠금으로 무시될 때
// 원인이 보고되는지 확인한다.
func TestHvacr03_CoilLockDiagnosis(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	// 리모컨 전체 잠금을 켜고, mock 이 전원 코일 쓰기를 무시하게 한다.
	gw.setCoil(0, pmbusCoilLockRemote, true)
	gw.mu.Lock()
	gw.lockedCoils[pmbusCoilAddr(0, pmbusCoilPower)] = true
	gw.mu.Unlock()
	a.pollAllDevices()

	resp := exec(t, a, map[string]any{
		"command": "set_power",
		"address": "0",
		"params":  map[string]any{"power": false},
	})

	if resp["verified"] != false {
		t.Fatalf("verified = %v, want false (the write was silently ignored)", resp["verified"])
	}
	locks, ok := resp["locked_by"].([]any)
	if !ok {
		t.Fatalf("locked_by missing or wrong type: %T (%v)", resp["locked_by"], resp)
	}
	if len(locks) == 0 || locks[0] != "remote" {
		t.Errorf("locked_by = %v, want [remote]", locks)
	}
}

// TestCoilLockTargetsFor 는 코일 항목별 잠금 후보를 확인한다.
func TestCoilLockTargetsFor(t *testing.T) {
	if got := coilLockTargetsFor(pmbusCoilPower); len(got) != 1 || got[0] != "remote" {
		t.Errorf("power coil lock targets = %v, want [remote]", got)
	}
	if got := coilLockTargetsFor(pmbusCoilSwing); len(got) != 1 || got[0] != "remote" {
		t.Errorf("swing coil lock targets = %v, want [remote]", got)
	}
	// 필터 해제는 잠금 대상이 아니다.
	if got := coilLockTargetsFor(pmbusCoilFilterReset); got != nil {
		t.Errorf("filter reset lock targets = %v, want nil", got)
	}
}

// TestHoldingLockTargetsFor 는 Holding 항목별 잠금 후보를 확인한다.
func TestHoldingLockTargetsFor(t *testing.T) {
	cases := map[uint16][]string{
		pmbusHoldingMode:     {"mode", "remote"},
		pmbusHoldingFanSpeed: {"fan", "remote"},
		pmbusHoldingSetTemp:  {"temp", "remote"},
		pmbusHoldingERVMode:  {"remote"},
	}
	for item, want := range cases {
		got := holdingLockTargetsFor(item)
		if len(got) != len(want) {
			t.Errorf("item %d targets = %v, want %v", item, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("item %d targets = %v, want %v", item, got, want)
				break
			}
		}
	}
}

// TestMismatchedLockTargets 는 FC16 불일치 항목에서 잠금 후보를 모으는지 확인한다.
func TestMismatchedLockTargets(t *testing.T) {
	want := []uint16{4, 3, 240} // mode=heat, fan=high, temp=24.0

	// 전부 일치 → 후보 없음.
	if got := mismatchedLockTargets(want, []uint16{4, 3, 240}); len(got) != 0 {
		t.Errorf("no mismatch should yield no targets, got %v", got)
	}

	// 온도만 불일치 → temp + remote.
	got := mismatchedLockTargets(want, []uint16{4, 3, 200})
	if len(got) != 2 || got[0] != "temp" || got[1] != "remote" {
		t.Errorf("temp mismatch targets = %v, want [temp remote]", got)
	}

	// 모드와 온도 불일치 → 중복 없이 mode, remote, temp.
	got = mismatchedLockTargets(want, []uint16{0, 3, 200})
	seen := map[string]int{}
	for _, g := range got {
		seen[g]++
	}
	if seen["remote"] != 1 {
		t.Errorf("remote should appear exactly once, got %d in %v", seen["remote"], got)
	}
	if seen["mode"] != 1 || seen["temp"] != 1 {
		t.Errorf("targets = %v, want mode and temp present", got)
	}
}

// TestLockBitFor 는 잠금 이름과 코일 항목의 매핑을 확인한다.
func TestLockBitFor(t *testing.T) {
	cases := map[string]uint16{
		"remote":  pmbusCoilLockRemote,
		"mode":    pmbusCoilLockMode,
		"fan":     pmbusCoilLockFan,
		"temp":    pmbusCoilLockTemp,
		"address": pmbusCoilLockAddress,
	}
	for name, want := range cases {
		got, ok := lockBitFor(name)
		if !ok || got != want {
			t.Errorf("lockBitFor(%q) = %d, %v; want %d, true", name, got, ok, want)
		}
	}
	if _, ok := lockBitFor("nonexistent"); ok {
		t.Error("unknown lock name should not resolve")
	}
}

// TestLockDiagnosisFromBits 는 방금 읽은 코일 비트에서 잠금을 판정하는지 확인한다.
func TestLockDiagnosisFromBits(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil)

	bits := make([]bool, pmbusCoilCount)
	bits[pmbusCoilLockRemote] = true
	bits[pmbusCoilLockTemp] = true

	got := a.lockDiagnosisFromBits(bits, []string{"remote", "mode", "temp"})
	if len(got) != 2 {
		t.Fatalf("active locks = %v, want 2 entries", got)
	}
	if got[0] != "remote" || got[1] != "temp" {
		t.Errorf("active locks = %v, want [remote temp]", got)
	}

	// 짧은 비트 배열은 조용히 건너뛴다 (범위 밖 접근 방지).
	if got := a.lockDiagnosisFromBits([]bool{}, []string{"remote"}); len(got) != 0 {
		t.Errorf("empty bits should yield no locks, got %v", got)
	}
}

// TestLockDiagnosisUnknownDevice 는 없는 디바이스에 대해 빈 결과를 내는지 확인한다.
func TestLockDiagnosisUnknownDevice(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil)

	if got := a.lockDiagnosis("99", []string{"remote"}); got != nil {
		t.Errorf("unknown device should yield nil, got %v", got)
	}
	if got := a.lockDiagnosis("0", nil); got != nil {
		t.Errorf("no candidates should yield nil, got %v", got)
	}
}

// ---------------------------------------------------------------------------
// 기기 종류별 투영 (AC-048, AC-049)
// ---------------------------------------------------------------------------

// TestPmbusProjectERV 는 환기 장치 투영을 확인한다.
func TestPmbusProjectERV(t *testing.T) {
	on := true
	mode := uint16(pmbusERVModeAuto)
	rapid := true
	eco := false
	temp := 24.5

	s := &PmbusDeviceState{
		Power:       &on,
		ERVModeCode: &mode,
		ERVRapid:    &rapid,
		ERVEco:      &eco,
		RoomTempC:   &temp, // ERV 투영에는 나타나지 않아야 한다
	}
	props := s.toProperties(pmbusDeviceTypeERV, pmbusProjectionOpts{fanAutoCode: 4})

	if props["erv_mode"] != int(pmbusERVModeAuto) {
		t.Errorf("erv_mode = %v, want %d", props["erv_mode"], pmbusERVModeAuto)
	}
	if props["erv_rapid"] != true {
		t.Errorf("erv_rapid = %v, want true", props["erv_rapid"])
	}
	if props["erv_eco"] != false {
		t.Errorf("erv_eco = %v, want false", props["erv_eco"])
	}
	for _, k := range []string{"current_temperature", "target_temperature", "fan_speed", "mode"} {
		if _, ok := props[k]; ok {
			t.Errorf("%s should not appear on a ventilator projection", k)
		}
	}
}

// TestPmbusProjectAWHP 는 하이드로킷 투영을 확인한다.
// 배관 온도가 입수/출수 온도 키로 바뀌고, 급탕·태양열이 노출된다.
func TestPmbusProjectAWHP(t *testing.T) {
	on := true
	mode := uint16(pmbusModeHeat)
	room, in, out, tank, solar := 24.5, 35.0, 40.0, 55.0, 60.0

	s := &PmbusDeviceState{
		Power:      &on,
		ModeCode:   &mode,
		RoomTempC:  &room,
		PipeInC:    &in,
		PipeOutC:   &out,
		WaterTankC: &tank,
		SolarC:     &solar,
	}
	props := s.toProperties(pmbusDeviceTypeAWHP, pmbusProjectionOpts{fanAutoCode: 4})

	checks := map[string]any{
		"current_temperature":      24.5,
		"water_in_temperature_c":   35.0,
		"water_out_temperature_c":  40.0,
		"water_tank_temperature_c": 55.0,
		"solar_temperature_c":      60.0,
		"mode":                     hvac.ModeHeat,
	}
	for k, want := range checks {
		if got := props[k]; got != want {
			t.Errorf("%s = %v, want %v", k, got, want)
		}
	}
	// 하이드로킷에는 풍량·스윙 개념이 없다.
	for _, k := range []string{"fan_speed", "swing", "pipe_in_temperature_c"} {
		if _, ok := props[k]; ok {
			t.Errorf("%s should not appear on a hydro-kit projection", k)
		}
	}
}

// TestPmbusProjectPowerOffPerType 은 종류별 전원 OFF 투영을 확인한다.
func TestPmbusProjectPowerOffPerType(t *testing.T) {
	off := false
	temp := 24.5
	mode := uint16(pmbusModeHeat)
	s := &PmbusDeviceState{Power: &off, RoomTempC: &temp, ModeCode: &mode}
	opts := pmbusProjectionOpts{fanAutoCode: 4}

	t.Run("indoor", func(t *testing.T) {
		props := s.toProperties(pmbusDeviceTypeIDU, opts)
		if props["fan_speed"] != hvac.FanOff || props["mode"] != hvac.ModeOffOrAuto {
			t.Errorf("power-off indoor projection = %v", props)
		}
		if _, ok := props["current_temperature"]; ok {
			t.Error("current_temperature should be omitted when off")
		}
	})

	t.Run("awhp", func(t *testing.T) {
		props := s.toProperties(pmbusDeviceTypeAWHP, opts)
		if props["mode"] != hvac.ModeOffOrAuto {
			t.Errorf("mode = %v, want 0 when off", props["mode"])
		}
		if _, ok := props["current_temperature"]; ok {
			t.Error("current_temperature should be omitted when off")
		}
	})

	t.Run("erv", func(t *testing.T) {
		props := s.toProperties(pmbusDeviceTypeERV, opts)
		if _, ok := props["erv_mode"]; ok {
			t.Error("erv_mode should be omitted when off")
		}
	})
}

// TestPmbusStateSnapshotIsDeepCopy 는 스냅샷이 원본과 포인터를 공유하지 않는지
// 확인한다. 공유하면 호출자가 에이전트 내부 상태를 바꿀 수 있다.
func TestPmbusStateSnapshotIsDeepCopy(t *testing.T) {
	on := true
	temp := 24.0
	mode := uint16(pmbusModeCool)
	s := &PmbusDeviceState{Power: &on, SetTempC: &temp, ModeCode: &mode}

	cp := s.snapshot()
	if cp.Power == s.Power {
		t.Error("Power pointer is shared between the snapshot and the source")
	}
	if cp.SetTempC == s.SetTempC {
		t.Error("SetTempC pointer is shared")
	}
	if cp.ModeCode == s.ModeCode {
		t.Error("ModeCode pointer is shared")
	}

	*cp.Power = false
	if !*s.Power {
		t.Error("mutating the snapshot changed the source")
	}
}

func TestPmbusStateSnapshotHandlesNils(t *testing.T) {
	s := &PmbusDeviceState{}
	cp := s.snapshot()
	if cp.Power != nil || cp.SetTempC != nil || cp.ModeCode != nil {
		t.Error("nil fields should stay nil in the snapshot")
	}
	if cp.isPowerOn() {
		t.Error("a state with no observations should not report power on")
	}
}

// TestIsValidPmbusDeviceType 은 기기 종류 검증을 확인한다.
func TestIsValidPmbusDeviceType(t *testing.T) {
	for _, dt := range []string{pmbusDeviceTypeIDU, pmbusDeviceTypeERV, pmbusDeviceTypeAWHP} {
		if !isValidPmbusDeviceType(dt) {
			t.Errorf("%q should be valid", dt)
		}
	}
	for _, dt := range []string{"", "controller", "HVACR.ODU", "idu"} {
		if isValidPmbusDeviceType(dt) {
			t.Errorf("%q should be invalid", dt)
		}
	}
}

// TestModbusExceptionMessage 는 예외 코드 이름이 메시지에 담기는지 확인한다.
func TestModbusExceptionMessage(t *testing.T) {
	cases := map[byte]string{
		0x01: "ILLEGAL_FUNCTION",
		0x02: "ILLEGAL_DATA_ADDRESS",
		0x03: "ILLEGAL_DATA_VALUE",
		0x04: "SLAVE_DEVICE_FAILURE",
		0x05: "ACKNOWLEDGE",
		0x06: "SLAVE_DEVICE_BUSY",
		0x08: "MEMORY_PARITY_ERROR",
		0x0A: "GATEWAY_PATH_UNAVAILABLE",
		0x0B: "GATEWAY_TARGET_NO_RESPONSE",
		0x7F: "UNKNOWN",
	}
	for code, want := range cases {
		e := &ModbusException{FunctionCode: 0x03, Code: code}
		if got := modbusExceptionName(code); got != want {
			t.Errorf("exception name for 0x%02X = %q, want %q", code, got, want)
		}
		if e.Error() == "" {
			t.Errorf("error message for 0x%02X should not be empty", code)
		}
	}
}
