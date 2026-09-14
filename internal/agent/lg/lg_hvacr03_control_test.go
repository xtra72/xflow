package lg

import (
	"errors"
	"testing"
)

// ---------------------------------------------------------------------------
// 제어 (AC-060 ~ AC-071)
// ---------------------------------------------------------------------------

// setupControlAgent 는 제어가 활성화된 에이전트를 만든다.
// control_verify_delay 는 1ms 로 낮춰 테스트가 느려지지 않게 한다.
func setupControlAgent(t *testing.T, n uint16, extra map[string]any) (*Hvacr03Agent, *mockGateway) {
	t.Helper()
	opts := map[string]any{
		"control_enabled":      true,
		"control_verify_delay": "1ms",
	}
	for k, v := range extra {
		opts[k] = v
	}
	return setupPolledAgent(t, n, opts)
}

// TestHvacr03_ControlDisabledBlocksWrites 는 control_enabled=false 일 때 버스에
// 어떤 쓰기도 나가지 않는지 확인한다.
func TestHvacr03_ControlDisabledBlocksWrites(t *testing.T) {
	a, gw := setupPolledAgent(t, 0, nil) // control_enabled 기본 false
	gw.resetRequests()

	err := execErr(t, a, map[string]any{
		"command": "set_power",
		"address": "0",
		"params":  map[string]any{"power": true},
	})
	if !errors.Is(err, ErrHvacr03ControlNotEnabled) {
		t.Fatalf("error = %v, want ErrHvacr03ControlNotEnabled", err)
	}
	if gw.requestCount() != 0 {
		t.Errorf("issued %d transactions, want 0 (no write may reach the bus)", gw.requestCount())
	}
}

// TestHvacr03_ControlRejectsDisconnectedUnit 은 미연결 실내기에 쓰기를 차단하는지
// 확인한다. 미설치 N 쓰기는 예외 응답 없이 조용히 무시되므로 사전 차단이 필요하다.
func TestHvacr03_ControlRejectsDisconnectedUnit(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(5, false)

	a := newTestAgent(t, gw, map[string]any{
		"control_enabled":      true,
		"control_verify_delay": "1ms",
		"devices":              []any{map[string]any{"address": "5"}},
	})
	a.registerConfigDevices()
	a.scanDevices()
	gw.resetRequests()

	err := execErr(t, a, map[string]any{
		"command": "set_power",
		"address": "5",
		"params":  map[string]any{"power": true},
	})
	if !errors.Is(err, ErrHvacr03DeviceNotConnected) {
		t.Fatalf("error = %v, want ErrHvacr03DeviceNotConnected", err)
	}
	if gw.requestCount() != 0 {
		t.Errorf("issued %d transactions, want 0", gw.requestCount())
	}
}

func TestHvacr03_SetPower(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	resp := exec(t, a, map[string]any{
		"command": "set_power",
		"address": "0",
		"params":  map[string]any{"power": false},
	})

	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}
	if gw.getCoil(0, pmbusCoilPower) {
		t.Error("power coil should be off")
	}
}

func TestHvacr03_SetMode(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	resp := exec(t, a, map[string]any{
		"command": "set_mode",
		"address": "0",
		"params":  map[string]any{"mode": "heat"},
	})

	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}
	if got := gw.getHolding(0, pmbusHoldingMode); got != pmbusModeHeat {
		t.Errorf("mode register = %d, want %d", got, pmbusModeHeat)
	}
}

func TestHvacr03_SetTemperature(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	resp := exec(t, a, map[string]any{
		"command": "target_temperature",
		"address": "0",
		"params":  map[string]any{"temperature": 26.0},
	})

	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}
	if got := gw.getHolding(0, pmbusHoldingSetTemp); got != 260 {
		t.Errorf("set temp register = %d, want 260", got)
	}
}

// TestHvacr03_TemperatureAbsoluteRange 는 절대 허용 범위를 벗어난 값을 거부하는지
// 확인한다.
func TestHvacr03_TemperatureAbsoluteRange(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)
	gw.resetRequests()

	for _, temp := range []float64{15.0, 31.0, -5.0, 100.0} {
		err := execErr(t, a, map[string]any{
			"command": "target_temperature",
			"address": "0",
			"params":  map[string]any{"temperature": temp},
		})
		if !errors.Is(err, ErrHvacr03TemperatureOutOfRange) {
			t.Errorf("temperature %.1f: error = %v, want ErrHvacr03TemperatureOutOfRange", temp, err)
		}
	}
	if gw.requestCount() != 0 {
		t.Errorf("issued %d transactions, want 0 (validation happens before the write)", gw.requestCount())
	}
}

// TestHvacr03_TemperatureDeviceLimits 는 디바이스의 상·하한 제한을 존중하는지
// 확인한다. 제한을 넘는 값은 게이트웨이가 조용히 클램핑하므로 미리 막는다.
func TestHvacr03_TemperatureDeviceLimits(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(0, true)
	gw.setCoil(0, pmbusCoilPower, true)
	gw.setHolding(0, pmbusHoldingTempLimitHigh, 260) // 26.0 °C
	gw.setHolding(0, pmbusHoldingTempLimitLow, 200)  // 20.0 °C

	a := newTestAgent(t, gw, map[string]any{
		"control_enabled":      true,
		"control_verify_delay": "1ms",
	})
	a.scanDevices()
	a.pollAllDevices()

	// 상한 초과.
	err := execErr(t, a, map[string]any{
		"command": "target_temperature",
		"address": "0",
		"params":  map[string]any{"temperature": 28.0},
	})
	if !errors.Is(err, ErrHvacr03TemperatureOutOfRange) {
		t.Errorf("28.0 exceeds the 26.0 upper limit: error = %v", err)
	}

	// 하한 미달.
	err = execErr(t, a, map[string]any{
		"command": "target_temperature",
		"address": "0",
		"params":  map[string]any{"temperature": 18.0},
	})
	if !errors.Is(err, ErrHvacr03TemperatureOutOfRange) {
		t.Errorf("18.0 is below the 20.0 lower limit: error = %v", err)
	}

	// 범위 안은 통과.
	resp := exec(t, a, map[string]any{
		"command": "target_temperature",
		"address": "0",
		"params":  map[string]any{"temperature": 24.0},
	})
	if resp["status"] != "ok" {
		t.Errorf("24.0 is within limits but was rejected: %v", resp)
	}
}

// TestHvacr03_SetMultipleUsesSingleFC16 은 set_multiple 이 단일 FC16 트랜잭션인지
// 확인한다. 에어컨은 모드별로 마지막 온도·풍량을 기억하므로, 개별 FC06 으로
// 나누면 모드 쓰기 직후 기억된 값으로 되돌아가는 중간 상태가 생긴다.
func TestHvacr03_SetMultipleUsesSingleFC16(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)
	gw.resetRequests()

	resp := exec(t, a, map[string]any{
		"command": "set_multiple",
		"address": "0",
		"params": map[string]any{
			"mode":        "heat",
			"fan_speed":   "high",
			"temperature": 24.0,
		},
	})

	if got := len(gw.requestsWithFC(pmbusFCWriteMultipleRegs)); got != 1 {
		t.Errorf("FC16 writes = %d, want 1", got)
	}
	if got := len(gw.requestsWithFC(pmbusFCWriteSingleReg)); got != 0 {
		t.Errorf("FC06 writes = %d, want 0 (must not split into individual writes)", got)
	}
	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}

	if got := gw.getHolding(0, pmbusHoldingMode); got != pmbusModeHeat {
		t.Errorf("mode = %d, want %d", got, pmbusModeHeat)
	}
	if got := gw.getHolding(0, pmbusHoldingFanSpeed); got != pmbusFanHigh {
		t.Errorf("fan = %d, want %d", got, pmbusFanHigh)
	}
	if got := gw.getHolding(0, pmbusHoldingSetTemp); got != 240 {
		t.Errorf("temp = %d, want 240", got)
	}
}

// TestHvacr03_SetMultiplePartialPreservesOthers 는 일부 항목만 지정했을 때
// 나머지가 현재 값을 유지하는지 확인한다.
func TestHvacr03_SetMultiplePartialPreservesOthers(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)
	// 초기값: mode=cool(0), fan=high(3), temp=240

	exec(t, a, map[string]any{
		"command": "set_multiple",
		"address": "0",
		"params":  map[string]any{"temperature": 28.0},
	})

	if got := gw.getHolding(0, pmbusHoldingMode); got != pmbusModeCool {
		t.Errorf("mode = %d, want %d (unchanged)", got, pmbusModeCool)
	}
	if got := gw.getHolding(0, pmbusHoldingFanSpeed); got != pmbusFanHigh {
		t.Errorf("fan = %d, want %d (unchanged)", got, pmbusFanHigh)
	}
	if got := gw.getHolding(0, pmbusHoldingSetTemp); got != 280 {
		t.Errorf("temp = %d, want 280", got)
	}
}

// TestHvacr03_LockDiagnosis 는 잠금으로 쓰기가 무시될 때 원인을 알려주는지
// 확인한다. 잠금이 걸리면 게이트웨이는 정상 에코를 돌려주고 값만 조용히 무시한다.
func TestHvacr03_LockDiagnosis(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	// 온도 잠금 코일을 켜고, mock 게이트웨이가 온도 쓰기를 무시하게 한다.
	gw.setCoil(0, pmbusCoilLockTemp, true)
	gw.lockHolding(0, pmbusHoldingSetTemp)
	a.pollAllDevices() // 잠금 상태를 에이전트 캐시에 반영

	resp := exec(t, a, map[string]any{
		"command": "target_temperature",
		"address": "0",
		"params":  map[string]any{"temperature": 28.0},
	})

	if resp["verified"] != false {
		t.Fatalf("verified = %v, want false (the write was silently ignored)", resp["verified"])
	}
	locks, ok := resp["locked_by"].([]any)
	if !ok {
		t.Fatalf("locked_by missing or wrong type: %T (%v)", resp["locked_by"], resp)
	}
	found := false
	for _, l := range locks {
		if l == "temp" {
			found = true
		}
	}
	if !found {
		t.Errorf("locked_by = %v, should include \"temp\"", locks)
	}
}

// TestHvacr03_NoLockDiagnosisWhenUnlocked 는 잠금이 없으면 진단이 붙지 않는지
// 확인한다 — 엉뚱한 원인을 제시하면 디버깅을 오히려 방해한다.
func TestHvacr03_NoLockDiagnosisWhenUnlocked(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	// 잠금 코일 없이 쓰기만 무시되게 한다 (다른 원인으로 반영 실패한 상황).
	gw.lockHolding(0, pmbusHoldingSetTemp)
	a.pollAllDevices()

	resp := exec(t, a, map[string]any{
		"command": "target_temperature",
		"address": "0",
		"params":  map[string]any{"temperature": 28.0},
	})

	if resp["verified"] != false {
		t.Fatalf("verified = %v, want false", resp["verified"])
	}
	if _, ok := resp["locked_by"]; ok {
		t.Errorf("locked_by should be absent when no lock is set: %v", resp["locked_by"])
	}
}

// ---------------------------------------------------------------------------
// 확장 제어 명령 (AC-068 ~ AC-071)
// ---------------------------------------------------------------------------

func TestHvacr03_SetSwing(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	resp := exec(t, a, map[string]any{
		"command": "set_swing",
		"address": "0",
		"params":  map[string]any{"swing": true},
	})
	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}
	if !gw.getCoil(0, pmbusCoilSwing) {
		t.Error("swing coil should be on")
	}
}

func TestHvacr03_ClearFilterAlarm(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	resp := exec(t, a, map[string]any{"command": "clear_filter_alarm", "address": "0"})
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	if !gw.getCoil(0, pmbusCoilFilterReset) {
		t.Error("filter reset coil should have been written")
	}
}

func TestHvacr03_SetLockTargets(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	cases := map[string]uint16{
		"remote":  pmbusCoilLockRemote,
		"mode":    pmbusCoilLockMode,
		"fan":     pmbusCoilLockFan,
		"temp":    pmbusCoilLockTemp,
		"address": pmbusCoilLockAddress,
	}
	for target, item := range cases {
		t.Run(target, func(t *testing.T) {
			resp := exec(t, a, map[string]any{
				"command": "set_lock",
				"address": "0",
				"params":  map[string]any{"target": target, "locked": true},
			})
			if resp["verified"] != true {
				t.Errorf("verified = %v, want true", resp["verified"])
			}
			if resp["target"] != target {
				t.Errorf("target = %v, want %s", resp["target"], target)
			}
			if !gw.getCoil(0, item) {
				t.Errorf("lock coil for %s should be set", target)
			}
		})
	}
}

func TestHvacr03_SetLockRejectsUnknownTarget(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil)

	err := execErr(t, a, map[string]any{
		"command": "set_lock",
		"address": "0",
		"params":  map[string]any{"target": "everything", "locked": true},
	})
	if err == nil {
		t.Fatal("unknown lock target should be rejected")
	}
}

// TestHvacr03_SetTempLimitUsesFC16 은 상·하한이 단일 FC16 트랜잭션으로 쓰이는지
// 확인한다 (Holding N×20+3..4 연속).
func TestHvacr03_SetTempLimitUsesFC16(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)
	gw.resetRequests()

	resp := exec(t, a, map[string]any{
		"command": "set_temp_limit",
		"address": "0",
		"params":  map[string]any{"high": 28.0, "low": 20.0},
	})

	if got := len(gw.requestsWithFC(pmbusFCWriteMultipleRegs)); got != 1 {
		t.Errorf("FC16 writes = %d, want 1", got)
	}
	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}
	if got := gw.getHolding(0, pmbusHoldingTempLimitHigh); got != 280 {
		t.Errorf("high limit = %d, want 280", got)
	}
	if got := gw.getHolding(0, pmbusHoldingTempLimitLow); got != 200 {
		t.Errorf("low limit = %d, want 200", got)
	}
}

func TestHvacr03_SetTempLimitRejectsInverted(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil)

	err := execErr(t, a, map[string]any{
		"command": "set_temp_limit",
		"address": "0",
		"params":  map[string]any{"high": 20.0, "low": 28.0},
	})
	if !errors.Is(err, ErrHvacr03TemperatureOutOfRange) {
		t.Errorf("inverted limits should be rejected, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// 기기 종류 게이트 (AC-071)
// ---------------------------------------------------------------------------

// TestHvacr03_ERVCommandsRejectedOnAirConditioner 는 에어컨에 ERV 명령이
// 거부되는지 확인한다.
func TestHvacr03_ERVCommandsRejectedOnAirConditioner(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil) // 기본 종류는 HVACR.IDU

	cases := []struct {
		command string
		params  map[string]any
	}{
		{"set_erv_mode", map[string]any{"erv_mode": "auto"}},
		{"set_erv_rapid", map[string]any{"enabled": true}},
		{"set_erv_eco", map[string]any{"enabled": true}},
	}
	for _, c := range cases {
		t.Run(c.command, func(t *testing.T) {
			err := execErr(t, a, map[string]any{
				"command": c.command, "address": "0", "params": c.params,
			})
			if !errors.Is(err, ErrHvacr03UnsupportedForDeviceType) {
				t.Errorf("error = %v, want ErrHvacr03UnsupportedForDeviceType", err)
			}
		})
	}
}

// TestHvacr03_SwingRejectedOnERV 는 ERV 에 스윙 명령이 거부되는지 확인한다.
func TestHvacr03_SwingRejectedOnERV(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(1, true)
	gw.setCoil(1, pmbusCoilPower, true)

	a := newTestAgent(t, gw, map[string]any{
		"control_enabled":      true,
		"control_verify_delay": "1ms",
		"devices":              []any{map[string]any{"address": "1", "device_type": "HVACR.ERV"}},
	})
	a.registerConfigDevices()
	a.scanDevices()

	err := execErr(t, a, map[string]any{
		"command": "set_swing",
		"address": "1",
		"params":  map[string]any{"swing": true},
	})
	if !errors.Is(err, ErrHvacr03UnsupportedForDeviceType) {
		t.Errorf("error = %v, want ErrHvacr03UnsupportedForDeviceType", err)
	}
}

// TestHvacr03_ERVCommandsOnERV 는 ERV 디바이스에서 ERV 명령이 동작하는지 확인한다.
//
// 미실측: 실제 ERV 장비로는 검증하지 못했다 (프로토콜 문서 §8). 여기서는 문서 기준의
// 레지스터 매핑이 올바른지만 확인한다.
func TestHvacr03_ERVCommandsOnERV(t *testing.T) {
	gw := newMockGateway()
	gw.setUnitConnected(1, true)
	gw.setCoil(1, pmbusCoilPower, true)

	a := newTestAgent(t, gw, map[string]any{
		"control_enabled":      true,
		"control_verify_delay": "1ms",
		"devices":              []any{map[string]any{"address": "1", "device_type": "HVACR.ERV"}},
	})
	a.registerConfigDevices()
	a.scanDevices()

	resp := exec(t, a, map[string]any{
		"command": "set_erv_mode",
		"address": "1",
		"params":  map[string]any{"erv_mode": "auto"},
	})
	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}
	if got := gw.getHolding(1, pmbusHoldingERVMode); got != pmbusERVModeAuto {
		t.Errorf("erv mode register = %d, want %d", got, pmbusERVModeAuto)
	}

	resp = exec(t, a, map[string]any{
		"command": "set_erv_rapid",
		"address": "1",
		"params":  map[string]any{"enabled": true},
	})
	if resp["verified"] != true {
		t.Errorf("verified = %v, want true", resp["verified"])
	}
	if !gw.getCoil(1, pmbusCoilERVRapid) {
		t.Error("erv rapid coil should be on")
	}
}

// ---------------------------------------------------------------------------
// 파라미터 검증
// ---------------------------------------------------------------------------

func TestHvacr03_ControlMissingParams(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil)

	cases := []struct {
		name    string
		command string
		params  map[string]any
	}{
		{"power missing", "set_power", map[string]any{}},
		{"power wrong type", "set_power", map[string]any{"power": "on"}},
		{"mode missing", "set_mode", map[string]any{}},
		{"mode unknown", "set_mode", map[string]any{"mode": "turbo-cool"}},
		{"fan unknown", "set_fan_speed", map[string]any{"fan_speed": "hurricane"}},
		{"temperature missing", "target_temperature", map[string]any{}},
		{"temperature wrong type", "target_temperature", map[string]any{"temperature": "24"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := execErr(t, a, map[string]any{
				"command": c.command, "address": "0", "params": c.params,
			})
			if err == nil {
				t.Errorf("%s should be rejected", c.name)
			}
		})
	}
}

func TestHvacr03_ControlMissingAddress(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil)

	err := execErr(t, a, map[string]any{
		"command": "set_power",
		"params":  map[string]any{"power": true},
	})
	if !errors.Is(err, ErrHvacr03MissingParam) {
		t.Errorf("error = %v, want ErrHvacr03MissingParam", err)
	}
}

func TestHvacr03_ControlUnknownDevice(t *testing.T) {
	a, _ := setupControlAgent(t, 0, nil)

	err := execErr(t, a, map[string]any{
		"command": "set_power",
		"address": "9",
		"params":  map[string]any{"power": true},
	})
	if !errors.Is(err, ErrDeviceNotFound) {
		t.Errorf("error = %v, want ErrDeviceNotFound", err)
	}
}

// TestHvacr03_FanAutoCodeAffectsControl 은 fan_auto_code 교정이 제어 경로에도
// 반영되는지 확인한다.
func TestHvacr03_FanAutoCodeAffectsControl(t *testing.T) {
	a, gw := setupControlAgent(t, 0, map[string]any{"fan_auto_code": 5})

	exec(t, a, map[string]any{
		"command": "set_fan_speed",
		"address": "0",
		"params":  map[string]any{"fan_speed": "auto"},
	})
	if got := gw.getHolding(0, pmbusHoldingFanSpeed); got != 5 {
		t.Errorf("fan register = %d, want 5 (fan_auto_code override)", got)
	}

	exec(t, a, map[string]any{
		"command": "set_fan_speed",
		"address": "0",
		"params":  map[string]any{"fan_speed": "turbo"},
	})
	if got := gw.getHolding(0, pmbusHoldingFanSpeed); got != 4 {
		t.Errorf("fan register = %d, want 4 (turbo available when auto is 5)", got)
	}
}

// TestHvacr03_WriteFailurePropagates 는 쓰기 실패가 에러로 전파되는지 확인한다.
func TestHvacr03_WriteFailurePropagates(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	gw.mu.Lock()
	gw.failNext = 1
	gw.failErr = errors.New("serial: write timeout")
	gw.mu.Unlock()

	err := execErr(t, a, map[string]any{
		"command": "set_power",
		"address": "0",
		"params":  map[string]any{"power": true},
	})
	if err == nil {
		t.Fatal("write failure should propagate as an error")
	}
	if a.writesFailed.Load() != 1 {
		t.Errorf("writes_failed = %d, want 1", a.writesFailed.Load())
	}
}

// TestHvacr03_ModbusExceptionOnWrite 는 게이트웨이 예외 응답이 에러로 전파되는지
// 확인한다.
func TestHvacr03_ModbusExceptionOnWrite(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)

	gw.interceptor = func(pdu []byte) ([]byte, error, bool) {
		if pdu[0] == pmbusFCWriteSingleCoil {
			return []byte{pmbusFCWriteSingleCoil | 0x80, 0x02}, nil, true
		}
		return nil, nil, false
	}

	err := execErr(t, a, map[string]any{
		"command": "set_power",
		"address": "0",
		"params":  map[string]any{"power": true},
	})
	var me *ModbusException
	if !errors.As(err, &me) {
		t.Fatalf("error = %v (%T), want *ModbusException", err, err)
	}
	if me.Code != 0x02 {
		t.Errorf("exception code = 0x%02X, want 0x02", me.Code)
	}
}

// TestHvacr03_ControlSerializedWithPolling 은 제어와 폴링이 동일한 직렬화 경로를
// 지나는지 확인한다. 두 경로가 같은 버스를 쓰므로 트랜잭션이 겹치면 프레임이 깨진다.
func TestHvacr03_ControlSerializedWithPolling(t *testing.T) {
	a, gw := setupControlAgent(t, 0, nil)
	gw.resetRequests()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 20; i++ {
			a.pollAllDevices()
		}
	}()

	for i := 0; i < 20; i++ {
		exec(t, a, map[string]any{
			"command": "set_power",
			"address": "0",
			"params":  map[string]any{"power": i%2 == 0},
		})
	}
	<-done

	// mock 게이트웨이는 요청을 순차 기록한다. 경합이 있었다면 -race 가 잡는다.
	if gw.requestCount() == 0 {
		t.Error("expected transactions to be recorded")
	}
}
