package lg

import (
	"context"
	"time"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 폴링 엔진
//
// SPEC-LG-HVACR-003 § M6. 두 개의 독립 루프가 돈다.
//
//	scanLoop — scan_interval 마다 FC02 전체 스캔 (0~255, 1 트랜잭션)
//	pollLoop — poll_interval 마다 온라인 디바이스를 순회 (디바이스당 FC01/03/04)
//
// 9,600 bps 에서 실내기 16대를 4종 영역 모두 폴링하면 한 사이클이 2~4초이므로
// poll_interval 하한 5s 가 강제된다 (설정 파싱에서 검증).
// ---------------------------------------------------------------------------

// scanLoop 은 주기적으로 FC02 전체 스캔을 수행한다.
func (a *Hvacr03Agent) scanLoop() {
	a.mu.RLock()
	interval := a.hvacr03Config.ScanInterval
	a.mu.RUnlock()

	// 기동 직후 1회 즉시 스캔한다 — 첫 주기를 기다리면 디바이스 목록이 비어 있는
	// 시간이 scan_interval 만큼 생긴다.
	a.scanDevices()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			if !a.connected.Load() {
				return // 재연결 루프가 새 scanLoop 을 기동한다
			}
			if a.isPaused() {
				continue
			}
			a.scanDevices()
		}
	}
}

// scanDevices 는 FC02 로 Discrete 0~255 를 한 번에 읽어 설치된 실내기를 판별한다.
//
// 한 트랜잭션으로 16대 × 16bit 를 모두 읽으므로, 대수가 늘어도 스캔 비용은 일정하다.
func (a *Hvacr03Agent) scanDevices() {
	a.mu.RLock()
	timeout := a.hvacr03Config.RequestTimeout
	base := a.hvacr03Config.AddressBase
	a.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	bits, err := a.readBits(ctx, pmbusFCReadDiscreteInputs, 0, pmbusScanBitCount)
	cancel()

	a.scansTotal.Add(1)
	if err != nil {
		a.pollsFailed.Add(1)
		a.logger.Warn("lg_hvacr03: 전체 스캔 실패", "error", err)
		a.handleTransportFailure(err)
		return
	}

	now := time.Now()
	var changed []string

	a.mu.Lock()
	agentName := a.agentConfig.Name
	v2 := a.onDeviceStateChangeV2
	for n := uint16(0); n < pmbusMaxUnits; n++ {
		connected := bits[pmbusScanBitIndex(n, pmbusDiscreteConnected)]
		addr := pmbusFormatUnitAddr(n, base)

		dev, exists := a.devices[addr]
		if !exists {
			if !connected {
				continue // 미설치 + 미등록 — 관심 대상이 아니다
			}
			dev = a.ensureDeviceLocked(addr, n)
			if dev == nil {
				continue // auto_discovery 비활성
			}
			changed = append(changed, addr)
		}

		alarm := bits[pmbusScanBitIndex(n, pmbusDiscreteAlarm)]
		filterAlarm := bits[pmbusScanBitIndex(n, pmbusDiscreteFilterAlarm)]
		tempBasis := bits[pmbusScanBitIndex(n, pmbusDiscreteTempBasis)]
		errorKind := bits[pmbusScanBitIndex(n, pmbusDiscreteErrorKind)]

		if dev.State == nil {
			dev.State = &PmbusDeviceState{}
		}
		dev.State.Connected = &connected
		dev.State.Alarm = &alarm
		dev.State.FilterAlarm = &filterAlarm
		dev.State.TempBasisWater = &tempBasis
		dev.State.ErrorKindBC = &errorKind

		// 목표 온도 기준이 "물"이면 하이드로킷이다. 설정으로 종류가 고정된
		// 디바이스는 사용자 지정을 우선하여 승격하지 않는다.
		if !dev.TypePinned && tempBasis && dev.Type == pmbusDeviceTypeIDU {
			dev.Type = pmbusDeviceTypeAWHP
			a.logger.Info("lg_hvacr03: 목표 온도 기준이 물 — 하이드로킷으로 판별",
				"address", addr)
		}

		wasOnline := dev.Online
		dev.Online = connected
		if connected {
			dev.LastSeen = now
		}
		if wasOnline != connected {
			changed = append(changed, addr)
		}
	}
	a.mu.Unlock()

	a.notifyDeviceChanges(v2, agentName, changed)
}

// pollLoop 은 주기적으로 온라인 디바이스의 상태 레지스터를 읽는다.
func (a *Hvacr03Agent) pollLoop() {
	a.mu.RLock()
	interval := a.hvacr03Config.PollInterval
	a.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			if !a.connected.Load() {
				return // 재연결 루프가 새 pollLoop 을 기동한다
			}
			if a.isPaused() {
				continue
			}
			a.pollAllDevices()
		}
	}
}

// pollAllDevices 는 온라인 디바이스를 순회하며 상태를 갱신한다.
//
// 한 디바이스의 실패가 사이클 전체를 중단시키지 않는다 — 다음 디바이스로 진행한다.
// 오프라인 디바이스는 건너뛰어 버스 대역을 절약한다.
func (a *Hvacr03Agent) pollAllDevices() {
	type target struct {
		addr string
		n    uint16
	}

	a.mu.RLock()
	targets := make([]target, 0, len(a.devices))
	for addr, dev := range a.devices {
		if dev.Online {
			targets = append(targets, target{addr: addr, n: dev.UnitN})
		}
	}
	a.mu.RUnlock()

	for _, t := range targets {
		select {
		case <-a.stopCh:
			return
		default:
		}
		a.pollDevice(t.addr, t.n)
	}
}

// pollDevice 는 디바이스 하나의 코일·Holding·Input 을 읽고 상태를 갱신한다.
func (a *Hvacr03Agent) pollDevice(addr string, n uint16) {
	a.mu.RLock()
	timeout := a.hvacr03Config.RequestTimeout
	scale := a.hvacr03Config.TempScale
	logDecodeErrors := a.hvacr03Config.LogDecodeErrors
	a.mu.RUnlock()

	a.pollsTotal.Add(1)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	coils, err := a.readBits(ctx, pmbusFCReadCoils, pmbusCoilAddr(n, 0), pmbusCoilCount)
	if err != nil {
		a.onPollError(addr, "coils", err, logDecodeErrors)
		return
	}
	holding, err := a.readRegisters(ctx, pmbusFCReadHolding, pmbusHoldingAddr(n, 0), pmbusHoldingCount)
	if err != nil {
		a.onPollError(addr, "holding", err, logDecodeErrors)
		return
	}
	input, err := a.readRegisters(ctx, pmbusFCReadInput, pmbusInputAddr(n, 0), pmbusInputCount)
	if err != nil {
		a.onPollError(addr, "input", err, logDecodeErrors)
		return
	}

	a.applyPollResult(addr, coils, holding, input, scale)
}

// onPollError 는 폴링 실패를 기록하고, 연결 단절이면 재연결을 개시한다.
func (a *Hvacr03Agent) onPollError(addr, area string, err error, logDecodeErrors bool) {
	a.pollsFailed.Add(1)
	if logDecodeErrors {
		a.logger.Warn("lg_hvacr03: 폴링 실패", "address", addr, "area", area, "error", err)
	}
	// Modbus 예외는 게이트웨이가 응답한 것이므로 연결은 살아 있다. 그 외
	// (타임아웃·I/O 오류)는 연결 단절 가능성이 있다.
	if isModbusException(err) {
		a.decodeErrors.Add(1)
		return
	}
	a.handleTransportFailure(err)
}

// isModbusException 은 에러가 게이트웨이의 Modbus 예외 응답인지 판별한다.
func isModbusException(err error) bool {
	var me *ModbusException
	return asModbusException(err, &me)
}

// applyPollResult 는 읽은 레지스터를 디바이스 상태에 반영하고 변경 시 방출한다.
func (a *Hvacr03Agent) applyPollResult(addr string, coils []bool, holding, input []uint16, scale int) {
	now := time.Now()

	a.mu.Lock()
	dev, ok := a.devices[addr]
	if !ok {
		a.mu.Unlock()
		return
	}
	if dev.State == nil {
		dev.State = &PmbusDeviceState{}
	}
	s := dev.State

	// --- Coil ---
	setBool(&s.Power, coils, pmbusCoilPower)
	setBool(&s.Swing, coils, pmbusCoilSwing)
	setBool(&s.LockRemote, coils, pmbusCoilLockRemote)
	setBool(&s.LockMode, coils, pmbusCoilLockMode)
	setBool(&s.LockFan, coils, pmbusCoilLockFan)
	setBool(&s.LockTemp, coils, pmbusCoilLockTemp)
	setBool(&s.LockAddress, coils, pmbusCoilLockAddress)
	setBool(&s.ERVRapid, coils, pmbusCoilERVRapid)
	setBool(&s.ERVEco, coils, pmbusCoilERVEco)

	// --- Holding ---
	setU16(&s.ModeCode, holding, pmbusHoldingMode)
	setU16(&s.FanCode, holding, pmbusHoldingFanSpeed)
	setTemp(&s.SetTempC, holding, pmbusHoldingSetTemp, scale)
	setTemp(&s.TempLimitHigh, holding, pmbusHoldingTempLimitHigh, scale)
	setTemp(&s.TempLimitLow, holding, pmbusHoldingTempLimitLow, scale)
	setU16(&s.ERVModeCode, holding, pmbusHoldingERVMode)

	// --- Input ---
	setU16(&s.ErrorCode, input, pmbusInputErrorCode)
	setTemp(&s.RoomTempC, input, pmbusInputRoomTemp, scale)
	setTemp(&s.PipeInC, input, pmbusInputPipeIn, scale)
	setTemp(&s.PipeOutC, input, pmbusInputPipeOut, scale)
	setTemp(&s.WaterTankC, input, pmbusInputWaterTank, scale)
	setTemp(&s.SolarC, input, pmbusInputSolar, scale)

	dev.LastSeen = now
	dev.Online = true

	logStateUpdates := a.hvacr03Config.LogStateUpdates
	threshold := a.hvacr03Config.EventTempThreshold
	a.emitDeviceStateLocked(dev, "change", threshold)
	a.mu.Unlock()

	if logStateUpdates {
		a.logStateUpdate(addr, coils, holding, input)
	}
}

// setBool 은 비트 배열에서 값을 읽어 포인터 필드에 반영한다.
func setBool(dst **bool, bits []bool, idx int) {
	if idx >= len(bits) {
		return
	}
	v := bits[idx]
	*dst = &v
}

// setU16 은 워드 배열에서 값을 읽어 포인터 필드에 반영한다.
func setU16(dst **uint16, regs []uint16, idx int) {
	if idx >= len(regs) {
		return
	}
	v := regs[idx]
	*dst = &v
}

// setTemp 는 워드 배열의 값을 °C 로 변환해 포인터 필드에 반영한다.
func setTemp(dst **float64, regs []uint16, idx, scale int) {
	if idx >= len(regs) {
		return
	}
	v := pmbusDecodeTemp(regs[idx], scale)
	*dst = &v
}

// logStateUpdate 는 진단용 상태 갱신 로그를 남긴다.
func (a *Hvacr03Agent) logStateUpdate(addr string, coils []bool, holding, input []uint16) {
	a.logger.Info("lg_hvacr03: 상태 갱신",
		"address", addr,
		"coils", coils,
		"holding", holding,
		"input", input,
	)
}
