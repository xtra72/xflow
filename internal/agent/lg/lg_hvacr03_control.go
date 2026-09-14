package lg

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 제어
//
// SPEC-LG-HVACR-003 § M7 / § 4.7.
//
// lg_hvacr02 의 서모스탯 사칭과 달리 정식 레지스터 쓰기이므로 SEQ 추적·에코 판별·
// 버스 정숙 대기가 전부 불필요하다. 대신 문서 §6.2 가 경고하는 네 가지를 지킨다.
//
//	1. 쓰기 후 즉시 반영되지 않는다      → control_verify_delay 후 read-back
//	2. 잠금 코일이 걸린 항목은 조용히 무시 → 불일치 시 잠금 진단 제공
//	3. 상·하한을 넘는 온도는 클램핑/거부  → 쓰기 전 범위 검증
//	4. 미설치 N 쓰기는 조용히 무시        → 쓰기 전 연결 상태 확인
// ---------------------------------------------------------------------------

// controlTarget 은 제어 명령의 해석된 대상이다.
type controlTarget struct {
	addr    string
	unitN   uint16
	devType string
}

// processControlCommand 는 제어 명령을 처리한다.
func (a *Hvacr03Agent) processControlCommand(req *hvacr03ProcessRequest) ([]byte, error) {
	fillAddressFromParams(req)

	target, err := a.resolveControlTarget(req)
	if err != nil {
		return nil, err
	}

	a.mu.RLock()
	cfg := a.hvacr03Config
	a.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), cfg.RequestTimeout*4)
	defer cancel()

	a.logger.Info("lg_hvacr03: 제어 명령 수신",
		"command", req.Command, "address", target.addr, "params", req.Params)

	switch req.Command {
	case "set_power":
		return a.controlPower(ctx, target, req, cfg)
	case "set_mode":
		return a.controlMode(ctx, target, req, cfg)
	case "set_fan_speed":
		return a.controlFanSpeed(ctx, target, req, cfg)
	case "target_temperature":
		return a.controlTemperature(ctx, target, req, cfg)
	case "set_multiple":
		return a.controlMultiple(ctx, target, req, cfg)
	case "set_swing":
		return a.controlSwing(ctx, target, req, cfg)
	case "clear_filter_alarm":
		return a.controlCoilSimple(ctx, target, cfg, pmbusCoilFilterReset, true, "clear_filter_alarm")
	case "set_lock":
		return a.controlLock(ctx, target, req, cfg)
	case "set_temp_limit":
		return a.controlTempLimit(ctx, target, req, cfg)
	case "set_erv_mode":
		return a.controlERVMode(ctx, target, req, cfg)
	case "set_erv_rapid":
		return a.controlERVCoil(ctx, target, req, cfg, pmbusCoilERVRapid, "set_erv_rapid")
	case "set_erv_eco":
		return a.controlERVCoil(ctx, target, req, cfg, pmbusCoilERVEco, "set_erv_eco")
	default:
		return nil, fmt.Errorf("lg_hvacr03: unsupported control command %q", req.Command)
	}
}

// resolveControlTarget 은 제어 대상 디바이스를 해석하고 사전 조건을 검증한다.
//
// 문서 §6.2-4: 미설치 N 에 대한 쓰기는 예외 응답 없이 조용히 무시될 수 있다.
// 따라서 쓰기 전에 연결 상태를 확인한다.
func (a *Hvacr03Agent) resolveControlTarget(req *hvacr03ProcessRequest) (controlTarget, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if !a.hvacr03Config.ControlEnabled {
		return controlTarget{}, ErrHvacr03ControlNotEnabled
	}

	addr := req.Address
	if addr == "" && req.DeviceID != "" {
		if byUUID, ok := a.addrByDeviceUUIDLocked(req.DeviceID); ok {
			addr = byUUID
		}
	}
	if addr == "" {
		return controlTarget{}, fmt.Errorf("%w: address", ErrHvacr03MissingParam)
	}

	n, err := pmbusParseUnitAddr(addr, a.hvacr03Config.AddressBase)
	if err != nil {
		return controlTarget{}, err
	}

	dev, ok := a.devices[addr]
	if !ok {
		return controlTarget{}, ErrDeviceNotFound
	}
	// 연결 비트가 관측된 적이 있고 미연결이면 쓰기를 차단한다. 아직 스캔 전이라
	// 관측이 없으면(Connected == nil) 통과시킨다 — 기동 직후 제어를 막지 않는다.
	if dev.State != nil && dev.State.Connected != nil && !*dev.State.Connected {
		return controlTarget{}, fmt.Errorf("%w: address %s", ErrHvacr03DeviceNotConnected, addr)
	}

	return controlTarget{addr: addr, unitN: n, devType: dev.Type}, nil
}

// requireDeviceType 은 명령이 해당 기기 종류에서 지원되는지 확인한다.
func requireDeviceType(target controlTarget, want string, command string) error {
	if target.devType != want {
		return fmt.Errorf("%w: %s requires %s but device is %s",
			ErrHvacr03UnsupportedForDeviceType, command, want, target.devType)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 개별 제어 명령
// ---------------------------------------------------------------------------

// controlPower 는 전원을 켜거나 끈다 (FC05 Coil N×16+0).
func (a *Hvacr03Agent) controlPower(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	on, err := paramBool(req.Params, "power")
	if err != nil {
		return nil, err
	}
	addr := pmbusCoilAddr(target.unitN, pmbusCoilPower)
	if err := a.writeCoil(ctx, addr, on); err != nil {
		return nil, err
	}
	return a.verifyCoil(ctx, target, cfg, pmbusCoilPower, on, "set_power")
}

// controlSwing 은 자동 풍향 스윙을 제어한다 (FC05 Coil N×16+1, 에어컨 전용).
func (a *Hvacr03Agent) controlSwing(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	if err := requireDeviceType(target, pmbusDeviceTypeIDU, "set_swing"); err != nil {
		return nil, err
	}
	on, err := paramBool(req.Params, "swing")
	if err != nil {
		return nil, err
	}
	if err := a.writeCoil(ctx, pmbusCoilAddr(target.unitN, pmbusCoilSwing), on); err != nil {
		return nil, err
	}
	return a.verifyCoil(ctx, target, cfg, pmbusCoilSwing, on, "set_swing")
}

// controlCoilSimple 은 고정값 코일 쓰기를 수행한다 (필터 알람 해제 등).
func (a *Hvacr03Agent) controlCoilSimple(ctx context.Context, target controlTarget, cfg Hvacr03Config, item uint16, value bool, command string) ([]byte, error) {
	if err := a.writeCoil(ctx, pmbusCoilAddr(target.unitN, item), value); err != nil {
		return nil, err
	}
	// 필터 알람 해제는 게이트웨이가 처리 후 코일을 스스로 되돌릴 수 있어 read-back
	// 일치를 요구하지 않는다. 쓰기 성공만 보고한다.
	return json.Marshal(map[string]any{
		"status":  "ok",
		"command": command,
		"address": target.addr,
	})
}

// controlLock 은 리모컨 잠금 코일을 제어한다 (FC05 Coil N×16+3..7).
func (a *Hvacr03Agent) controlLock(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	targetName, err := paramString(req.Params, "target")
	if err != nil {
		return nil, err
	}
	item, err := pmbusLockCoilItem(targetName)
	if err != nil {
		return nil, err
	}
	locked, err := paramBool(req.Params, "locked")
	if err != nil {
		return nil, err
	}
	if err := a.writeCoil(ctx, pmbusCoilAddr(target.unitN, item), locked); err != nil {
		return nil, err
	}
	resp, err := a.verifyCoil(ctx, target, cfg, item, locked, "set_lock")
	if err != nil {
		return nil, err
	}
	return withExtra(resp, map[string]any{"target": targetName})
}

// controlERVCoil 은 ERV 전용 코일을 제어한다.
func (a *Hvacr03Agent) controlERVCoil(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config, item uint16, command string) ([]byte, error) {
	if err := requireDeviceType(target, pmbusDeviceTypeERV, command); err != nil {
		return nil, err
	}
	on, err := paramBool(req.Params, "enabled")
	if err != nil {
		return nil, err
	}
	if err := a.writeCoil(ctx, pmbusCoilAddr(target.unitN, item), on); err != nil {
		return nil, err
	}
	return a.verifyCoil(ctx, target, cfg, item, on, command)
}

// controlMode 는 운전 모드를 변경한다 (FC06 Holding N×20+0).
//
// 주의: 모드만 바꾸면 실내기가 그 모드에서 기억하던 온도·풍량으로 함께 되돌아간다
// (문서 §6.2-5). 모드·온도·풍량을 함께 지정하려면 set_multiple 을 써야 한다.
func (a *Hvacr03Agent) controlMode(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	name, err := paramString(req.Params, "mode")
	if err != nil {
		return nil, err
	}
	code, err := pmbusModeFromName(name)
	if err != nil {
		return nil, err
	}
	if err := a.writeRegister(ctx, pmbusHoldingAddr(target.unitN, pmbusHoldingMode), code); err != nil {
		return nil, err
	}
	return a.verifyHolding(ctx, target, cfg, pmbusHoldingMode, code, "set_mode")
}

// controlFanSpeed 는 풍량을 변경한다 (FC06 Holding N×20+1).
func (a *Hvacr03Agent) controlFanSpeed(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	name, err := paramString(req.Params, "fan_speed")
	if err != nil {
		return nil, err
	}
	code, err := pmbusFanFromName(name, cfg.FanAutoCode)
	if err != nil {
		return nil, err
	}
	if err := a.writeRegister(ctx, pmbusHoldingAddr(target.unitN, pmbusHoldingFanSpeed), code); err != nil {
		return nil, err
	}
	return a.verifyHolding(ctx, target, cfg, pmbusHoldingFanSpeed, code, "set_fan_speed")
}

// controlTemperature 는 설정 온도를 변경한다 (FC06 Holding N×20+2).
func (a *Hvacr03Agent) controlTemperature(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	tempC, err := paramFloat(req.Params, "temperature")
	if err != nil {
		return nil, err
	}
	if err := a.validateSetTemp(target.addr, tempC); err != nil {
		return nil, err
	}
	raw := pmbusEncodeTemp(tempC, cfg.TempScale)
	if err := a.writeRegister(ctx, pmbusHoldingAddr(target.unitN, pmbusHoldingSetTemp), raw); err != nil {
		return nil, err
	}
	return a.verifyHolding(ctx, target, cfg, pmbusHoldingSetTemp, raw, "target_temperature")
}

// controlMultiple 은 모드·풍량·온도를 단일 FC16 트랜잭션으로 쓴다.
//
// 문서 §6.2-5: 에어컨은 모드별로 마지막 온도·풍량을 기억하므로, 개별 FC06 으로
// 나눠 쓰면 모드 쓰기 직후 실내기가 기억된 값으로 되돌리는 중간 상태가 생긴다.
// 세 레지스터가 연속(N×20+0..2)이라 한 트랜잭션으로 쓸 수 있다.
func (a *Hvacr03Agent) controlMultiple(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	// 현재 값을 기준으로 삼고, 지정된 항목만 덮어쓴다.
	current, err := a.readRegisters(ctx, pmbusFCReadHolding,
		pmbusHoldingAddr(target.unitN, pmbusHoldingMode), 3)
	if err != nil {
		return nil, fmt.Errorf("lg_hvacr03: set_multiple read current: %w", err)
	}
	values := []uint16{current[0], current[1], current[2]}

	if v, ok := req.Params["mode"]; ok {
		name, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: mode must be a string", ErrHvacr03MissingParam)
		}
		code, err := pmbusModeFromName(name)
		if err != nil {
			return nil, err
		}
		values[0] = code
	}
	if v, ok := req.Params["fan_speed"]; ok {
		name, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("%w: fan_speed must be a string", ErrHvacr03MissingParam)
		}
		code, err := pmbusFanFromName(name, cfg.FanAutoCode)
		if err != nil {
			return nil, err
		}
		values[1] = code
	}
	if v, ok := req.Params["temperature"]; ok {
		tempC, err := toFloat(v, "temperature")
		if err != nil {
			return nil, err
		}
		if err := a.validateSetTemp(target.addr, tempC); err != nil {
			return nil, err
		}
		values[2] = pmbusEncodeTemp(tempC, cfg.TempScale)
	}

	if err := a.writeRegisters(ctx, pmbusHoldingAddr(target.unitN, pmbusHoldingMode), values); err != nil {
		return nil, err
	}

	a.sleepVerifyDelay(ctx, cfg.ControlVerifyDelay)

	actual, err := a.readRegisters(ctx, pmbusFCReadHolding,
		pmbusHoldingAddr(target.unitN, pmbusHoldingMode), 3)
	if err != nil {
		return json.Marshal(map[string]any{
			"status":   "ok",
			"command":  "set_multiple",
			"address":  target.addr,
			"verified": false,
			"expected": values,
			"error":    err.Error(),
		})
	}

	verified := actual[0] == values[0] && actual[1] == values[1] && actual[2] == values[2]
	resp := map[string]any{
		"status":   "ok",
		"command":  "set_multiple",
		"address":  target.addr,
		"verified": verified,
		"expected": values,
		"actual":   actual,
	}
	if !verified {
		if locks := a.lockDiagnosis(target.addr, mismatchedLockTargets(values, actual)); len(locks) > 0 {
			resp["locked_by"] = locks
		}
	}
	return json.Marshal(resp)
}

// controlTempLimit 은 설정 온도의 상·하한을 단일 FC16 트랜잭션으로 쓴다
// (Holding N×20+3..4 연속).
func (a *Hvacr03Agent) controlTempLimit(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	high, err := paramFloat(req.Params, "high")
	if err != nil {
		return nil, err
	}
	low, err := paramFloat(req.Params, "low")
	if err != nil {
		return nil, err
	}
	if low > high {
		return nil, fmt.Errorf("%w: low (%.1f) must not exceed high (%.1f)", ErrHvacr03TemperatureOutOfRange, low, high)
	}
	for _, v := range []float64{high, low} {
		if v < pmbusMinSetTempC || v > pmbusMaxSetTempC {
			return nil, fmt.Errorf("%w: %.1f (allowed %.1f-%.1f)",
				ErrHvacr03TemperatureOutOfRange, v, pmbusMinSetTempC, pmbusMaxSetTempC)
		}
	}

	values := []uint16{
		pmbusEncodeTemp(high, cfg.TempScale),
		pmbusEncodeTemp(low, cfg.TempScale),
	}
	addr := pmbusHoldingAddr(target.unitN, pmbusHoldingTempLimitHigh)
	if err := a.writeRegisters(ctx, addr, values); err != nil {
		return nil, err
	}

	a.sleepVerifyDelay(ctx, cfg.ControlVerifyDelay)

	actual, err := a.readRegisters(ctx, pmbusFCReadHolding, addr, 2)
	if err != nil {
		return json.Marshal(map[string]any{
			"status": "ok", "command": "set_temp_limit", "address": target.addr,
			"verified": false, "expected": values, "error": err.Error(),
		})
	}
	return json.Marshal(map[string]any{
		"status":   "ok",
		"command":  "set_temp_limit",
		"address":  target.addr,
		"verified": actual[0] == values[0] && actual[1] == values[1],
		"expected": values,
		"actual":   actual,
	})
}

// controlERVMode 는 환기 운전 모드를 변경한다 (FC06 Holding N×20+5, ERV 전용).
func (a *Hvacr03Agent) controlERVMode(ctx context.Context, target controlTarget, req *hvacr03ProcessRequest, cfg Hvacr03Config) ([]byte, error) {
	if err := requireDeviceType(target, pmbusDeviceTypeERV, "set_erv_mode"); err != nil {
		return nil, err
	}
	name, err := paramString(req.Params, "erv_mode")
	if err != nil {
		return nil, err
	}
	code, err := pmbusERVModeFromName(name)
	if err != nil {
		return nil, err
	}
	if err := a.writeRegister(ctx, pmbusHoldingAddr(target.unitN, pmbusHoldingERVMode), code); err != nil {
		return nil, err
	}
	return a.verifyHolding(ctx, target, cfg, pmbusHoldingERVMode, code, "set_erv_mode")
}

// ---------------------------------------------------------------------------
// read-back 검증
// ---------------------------------------------------------------------------

// sleepVerifyDelay 는 read-back 전 대기한다.
//
// 문서 §6.2-1: 게이트웨이가 명령을 LGCP 버스로 중계하고 실내기가 수락하기까지 수 초가
// 걸리므로, 쓰기 직후의 read-back 은 이전 값을 돌려줄 수 있다.
func (a *Hvacr03Agent) sleepVerifyDelay(ctx context.Context, d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctx.Done():
	case <-a.stopCh:
	}
}

// verifyCoil 은 코일 쓰기 결과를 read-back 으로 확인한다.
func (a *Hvacr03Agent) verifyCoil(ctx context.Context, target controlTarget, cfg Hvacr03Config, item uint16, want bool, command string) ([]byte, error) {
	a.sleepVerifyDelay(ctx, cfg.ControlVerifyDelay)

	bits, err := a.readBits(ctx, pmbusFCReadCoils, pmbusCoilAddr(target.unitN, 0), pmbusCoilCount)
	if err != nil {
		return json.Marshal(map[string]any{
			"status": "ok", "command": command, "address": target.addr,
			"verified": false, "expected": want, "error": err.Error(),
		})
	}

	actual := bits[item]
	resp := map[string]any{
		"status":   "ok",
		"command":  command,
		"address":  target.addr,
		"verified": actual == want,
		"expected": want,
		"actual":   actual,
	}
	if actual != want {
		if locks := a.lockDiagnosisFromBits(bits, coilLockTargetsFor(item)); len(locks) > 0 {
			resp["locked_by"] = locks
		}
	}
	return json.Marshal(resp)
}

// verifyHolding 은 Holding 레지스터 쓰기 결과를 read-back 으로 확인한다.
func (a *Hvacr03Agent) verifyHolding(ctx context.Context, target controlTarget, cfg Hvacr03Config, item, want uint16, command string) ([]byte, error) {
	a.sleepVerifyDelay(ctx, cfg.ControlVerifyDelay)

	regs, err := a.readRegisters(ctx, pmbusFCReadHolding, pmbusHoldingAddr(target.unitN, 0), pmbusHoldingCount)
	if err != nil {
		return json.Marshal(map[string]any{
			"status": "ok", "command": command, "address": target.addr,
			"verified": false, "expected": want, "error": err.Error(),
		})
	}

	actual := regs[item]
	resp := map[string]any{
		"status":   "ok",
		"command":  command,
		"address":  target.addr,
		"verified": actual == want,
		"expected": want,
		"actual":   actual,
	}
	if actual != want {
		if locks := a.lockDiagnosis(target.addr, holdingLockTargetsFor(item)); len(locks) > 0 {
			resp["locked_by"] = locks
		}
	}
	return json.Marshal(resp)
}

// ---------------------------------------------------------------------------
// 잠금 진단
//
// 문서 §6.2-2: 잠금 코일이 설정된 상태에서는 해당 항목의 쓰기가 조용히 무시된다.
// read-back 불일치의 원인이 잠금인지 알려주면 "제어가 안 먹는다"는 상황에서
// 사용자가 바로 원인을 짚을 수 있다.
// ---------------------------------------------------------------------------

// coilLockTargetsFor 는 코일 항목에 영향을 주는 잠금 대상 이름을 반환한다.
func coilLockTargetsFor(item uint16) []string {
	switch item {
	case pmbusCoilPower, pmbusCoilSwing:
		return []string{"remote"}
	default:
		return nil
	}
}

// holdingLockTargetsFor 는 Holding 항목에 영향을 주는 잠금 대상 이름을 반환한다.
func holdingLockTargetsFor(item uint16) []string {
	switch item {
	case pmbusHoldingMode:
		return []string{"mode", "remote"}
	case pmbusHoldingFanSpeed:
		return []string{"fan", "remote"}
	case pmbusHoldingSetTemp:
		return []string{"temp", "remote"}
	default:
		return []string{"remote"}
	}
}

// mismatchedLockTargets 는 FC16 일괄 쓰기에서 불일치한 항목의 잠금 대상을 모은다.
func mismatchedLockTargets(want, actual []uint16) []string {
	var targets []string
	seen := map[string]bool{}
	items := []uint16{pmbusHoldingMode, pmbusHoldingFanSpeed, pmbusHoldingSetTemp}
	for i, item := range items {
		if i >= len(want) || i >= len(actual) || want[i] == actual[i] {
			continue
		}
		for _, t := range holdingLockTargetsFor(item) {
			if !seen[t] {
				seen[t] = true
				targets = append(targets, t)
			}
		}
	}
	return targets
}

// lockDiagnosis 는 캐시된 상태에서 실제로 걸려 있는 잠금만 골라낸다.
func (a *Hvacr03Agent) lockDiagnosis(addr string, candidates []string) []string {
	if len(candidates) == 0 {
		return nil
	}
	a.mu.RLock()
	defer a.mu.RUnlock()

	dev, ok := a.devices[addr]
	if !ok || dev.State == nil {
		return nil
	}
	s := dev.State

	var active []string
	for _, c := range candidates {
		var flag *bool
		switch c {
		case "remote":
			flag = s.LockRemote
		case "mode":
			flag = s.LockMode
		case "fan":
			flag = s.LockFan
		case "temp":
			flag = s.LockTemp
		case "address":
			flag = s.LockAddress
		}
		if flag != nil && *flag {
			active = append(active, c)
		}
	}
	return active
}

// lockDiagnosisFromBits 는 방금 읽은 코일 비트에서 잠금 상태를 직접 판정한다.
// 캐시보다 신선하므로 코일 read-back 경로에서는 이쪽을 쓴다.
func lockBitFor(target string) (uint16, bool) {
	switch target {
	case "remote":
		return pmbusCoilLockRemote, true
	case "mode":
		return pmbusCoilLockMode, true
	case "fan":
		return pmbusCoilLockFan, true
	case "temp":
		return pmbusCoilLockTemp, true
	case "address":
		return pmbusCoilLockAddress, true
	default:
		return 0, false
	}
}

func (a *Hvacr03Agent) lockDiagnosisFromBits(bits []bool, candidates []string) []string {
	var active []string
	for _, c := range candidates {
		item, ok := lockBitFor(c)
		if !ok || int(item) >= len(bits) {
			continue
		}
		if bits[item] {
			active = append(active, c)
		}
	}
	return active
}

// ---------------------------------------------------------------------------
// 온도 범위 검증
// ---------------------------------------------------------------------------

// validateSetTemp 는 설정 온도가 절대 범위와 현재 상·하한 제한 안에 있는지 확인한다.
//
// 문서 §6.2-3: 상·하한을 넘는 값을 쓰면 클램핑되거나 거부된다. 미리 막으면 사용자가
// 조용한 클램핑 대신 명확한 오류를 받는다.
func (a *Hvacr03Agent) validateSetTemp(addr string, tempC float64) error {
	if tempC < pmbusMinSetTempC || tempC > pmbusMaxSetTempC {
		return fmt.Errorf("%w: %.1f (allowed %.1f-%.1f)",
			ErrHvacr03TemperatureOutOfRange, tempC, pmbusMinSetTempC, pmbusMaxSetTempC)
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	dev, ok := a.devices[addr]
	if !ok || dev.State == nil {
		return nil
	}
	if h := dev.State.TempLimitHigh; h != nil && *h > 0 && tempC > *h {
		return fmt.Errorf("%w: %.1f exceeds the device upper limit %.1f",
			ErrHvacr03TemperatureOutOfRange, tempC, *h)
	}
	if l := dev.State.TempLimitLow; l != nil && *l > 0 && tempC < *l {
		return fmt.Errorf("%w: %.1f is below the device lower limit %.1f",
			ErrHvacr03TemperatureOutOfRange, tempC, *l)
	}
	return nil
}

// ---------------------------------------------------------------------------
// 파라미터 헬퍼
// ---------------------------------------------------------------------------

func paramBool(params map[string]any, key string) (bool, error) {
	v, ok := params[key]
	if !ok {
		return false, fmt.Errorf("%w: %s", ErrHvacr03MissingParam, key)
	}
	b, ok := v.(bool)
	if !ok {
		return false, fmt.Errorf("%w: %s must be a boolean, got %T", ErrHvacr03MissingParam, key, v)
	}
	return b, nil
}

func paramString(params map[string]any, key string) (string, error) {
	v, ok := params[key]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrHvacr03MissingParam, key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: %s must be a string, got %T", ErrHvacr03MissingParam, key, v)
	}
	return s, nil
}

func paramFloat(params map[string]any, key string) (float64, error) {
	v, ok := params[key]
	if !ok {
		return 0, fmt.Errorf("%w: %s", ErrHvacr03MissingParam, key)
	}
	return toFloat(v, key)
}

func toFloat(v any, key string) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case float32:
		return float64(n), nil
	case int:
		return float64(n), nil
	case int64:
		return float64(n), nil
	default:
		return 0, fmt.Errorf("%w: %s must be a number, got %T", ErrHvacr03MissingParam, key, v)
	}
}

// withExtra 는 JSON 응답에 추가 필드를 병합한다.
func withExtra(raw []byte, extra map[string]any) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw, nil // 병합 실패 시 원본을 그대로 돌려준다
	}
	for k, v := range extra {
		m[k] = v
	}
	return json.Marshal(m)
}
