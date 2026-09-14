package lg

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// LG HVACR-03 상태 방출
//
// SPEC-LG-HVACR-003 § M6. lg_hvacr02 의 emitDeviceStatePayloadLocked 와 동일한
// 페이로드 구조·dedup 규칙·이중 push 를 유지한다. 기존 플로우·대시보드가 추가 변경
// 없이 동작하려면 이 형식이 바이트 구조까지 같아야 한다.
// ---------------------------------------------------------------------------

// emitDeviceStateLocked 는 디바이스 상태를 device_state 이벤트로 방출한다.
//
// tempThreshold 가 양수이면, 실내 온도만 변경되고 변화폭이 임계 미만인 경우
// change 방출을 억제한다 (보고 폭주 방지).
//
// 호출 전제: a.mu 쓰기 락 보유.
func (a *Hvacr03Agent) emitDeviceStateLocked(dev *PmbusDevice, trigger string, tempThreshold float64) {
	if dev == nil || dev.State == nil {
		return
	}

	// report_enabled 게이트: off 인 디바이스는 device_state 를 방출하지 않는다.
	// lastEmitted 갱신 전에 반환하여 off 동안의 상태를 dedup 기준으로 남기지 않는다.
	if !dev.ReportEnabled {
		return
	}

	full := dev.State.toProperties(dev.Type, a.projectionOptsLocked())

	// 발행 정책:
	//   "report" — 주기 heartbeat. 변경 여부와 무관하게 항상 발행한다.
	//   그 외     — 직전 발행 투영과 같으면 발행하지 않는다.
	if trigger != "report" {
		prev, hadPrev := a.lastEmitted[dev.Address]
		if hadPrev {
			if propertiesEqualPmbus(prev, full) {
				return
			}
			// 실내 온도만 바뀌었고 변화폭이 임계 미만이면 억제한다.
			if tempThreshold > 0 && onlyTemperatureChanged(prev, full, tempThreshold) {
				return
			}
		}
	}
	a.lastEmitted[dev.Address] = full

	// 락 보유 중이므로 a.Name() 을 호출하지 않는다 — RWMutex 재귀 락은 자기
	// deadlock 이 된다. agentConfig.Name 을 직접 읽는다.
	agentName := a.agentConfig.Name

	metadata := map[string]any{
		"name":        dev.Label,
		"address":     dev.Address,
		"device_type": dev.Type,
	}

	// 이벤트 시각: "report" 는 발행 시점이 곧 스냅샷 시각이다. dev.LastSeen 을 쓰면
	// 조용한 디바이스의 주기 report 가 모두 같은 과거 시각으로 찍혀 보고 주기가
	// 어긋난다. 그 외 트리거는 관측 시각을 쓴다.
	eventTime := dev.LastSeen
	if trigger == "report" || eventTime.IsZero() {
		eventTime = time.Now()
	}

	payload := map[string]any{
		"unit_id":      dev.Address,
		"device_id":    agent.ResolveDeviceID(context.Background(), agentName, dev.Address),
		"trigger":      trigger,
		"state":        full,
		"metadata":     metadata,
		"last_seen_ms": eventTime.UnixMilli(),
	}

	b, err := json.Marshal(payload)
	if err != nil {
		a.logger.Warn("lg_hvacr03: device_state marshal 실패", "error", err)
		return
	}

	seq := a.eventsEmitted.Add(1)
	a.pushRecentEvent(b, eventTime, seq)
	if a.bridgeActive.Load() {
		a.sendEvent(b)
	}
}

// onlyTemperatureChanged 는 두 투영의 차이가 실내 온도 하나뿐이고 그 변화폭이
// 임계 미만인지 판별한다.
//
// 실내 온도는 센서 노이즈로 매 폴링마다 0.1 ℃ 씩 흔들리므로, 이 게이트가 없으면
// change 이벤트가 폴링 주기마다 발생한다.
func onlyTemperatureChanged(prev, curr map[string]any, threshold float64) bool {
	if len(prev) != len(curr) {
		return false
	}

	tempDiffFound := false
	for k, cv := range curr {
		pv, ok := prev[k]
		if !ok {
			return false
		}
		if k == "current_temperature" {
			pf, pok := pv.(float64)
			cf, cok := cv.(float64)
			if !pok || !cok {
				return false
			}
			if pf == cf {
				continue
			}
			d := cf - pf
			if d < 0 {
				d = -d
			}
			if d >= threshold {
				return false // 임계 이상 변화 — 억제 대상이 아니다
			}
			tempDiffFound = true
			continue
		}
		if !valuesEqual(pv, cv) {
			return false // 온도 외 항목이 바뀌었다
		}
	}
	return tempDiffFound
}

// valuesEqual 은 투영 값 두 개의 동일성을 비교한다.
func valuesEqual(a, b any) bool {
	switch av := a.(type) {
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	case float64:
		bv, ok := b.(float64)
		return ok && av == bv
	default:
		return a == b
	}
}

// notifyLoop 은 주기적으로 모든 디바이스 상태를 report 트리거로 방출한다.
func (a *Hvacr03Agent) notifyLoop() {
	a.mu.RLock()
	interval := a.hvacr03Config.ReportInterval
	a.mu.RUnlock()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			if a.isPaused() {
				continue
			}
			a.emitAllDeviceStates("report")
		}
	}
}

// emitAllDeviceStates 는 모든 디바이스의 상태를 주어진 트리거로 방출하고,
// 실제 방출된 건수를 반환한다.
func (a *Hvacr03Agent) emitAllDeviceStates(trigger string) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	before := a.eventsEmitted.Load()
	threshold := a.hvacr03Config.EventTempThreshold
	for _, dev := range a.devices {
		// report 트리거는 임계 게이트를 적용하지 않는다 — heartbeat 는 항상 나간다.
		th := threshold
		if trigger == "report" {
			th = 0
		}
		a.emitDeviceStateLocked(dev, trigger, th)
	}
	return int(a.eventsEmitted.Load() - before)
}

// asModbusException 은 errors.As 를 감싼 헬퍼이다.
func asModbusException(err error, target **ModbusException) bool {
	return errors.As(err, target)
}
