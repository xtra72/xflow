package lg

import (
	"context"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// SPEC-HVACR-CONNSTATE-001: Device Connection-State Reporting (LGAP)
//
// 신규 message_type "device_connection.<trigger>" 를 device(zone) 당 개별 메시지로
// 방출한다. trigger ∈ {initial, change, report}, connection_state ∈ {online, offline}.
// 기존 device_state 봉투 규약을 재사용하며 최상위 type 필드는 두지 않는다.
// ---------------------------------------------------------------------------

const (
	connTriggerInitial = "initial"
	connTriggerChange  = "change"
	connTriggerReport  = "report"

	connStateOnline  = "online"
	connStateOffline = "offline"

	connMsgTypePrefix = "device_connection."

	// probeCheckInterval 은 startup probe 루프가 device online 확정을 폴링하는 간격이다.
	probeCheckInterval = 25 * time.Millisecond
)

// buildConnectionPayloadLocked 는 device_connection.<trigger> payload 를 구성한다.
// 호출 전제: a.mu 보유. RWMutex 비재진입 트랩(§7.1/N1)을 피하기 위해 a.Name()/a.ID()
// 를 절대 호출하지 않고 a.agentConfig.Name 을 직접 읽는다.
func (a *LGAPAgent) buildConnectionPayloadLocked(dev *LGAPDevice, trigger string, eventMs int64) map[string]any {
	connState := connStateOffline
	if dev.Online {
		connState = connStateOnline
	}
	var lastSeenMs int64
	if !dev.LastSeen.IsZero() {
		lastSeenMs = dev.LastSeen.UnixMilli()
	}
	return map[string]any{
		"unit_id":             dev.UnitID,
		"device_id":           agent.ResolveDeviceID(context.Background(), a.agentConfig.Name, dev.UnitID),
		"trigger":             trigger,
		"connected":           dev.Online,
		"connection_state":    connState,
		"error_count":         dev.ErrorCount,
		"offline_threshold":   a.lgapConfig.OfflineThreshold,
		"transport_connected": a.transport.Available(),
		"last_seen_ms":        lastSeenMs,
		"event_ms":            eventMs,
		"metadata": map[string]any{
			"message_type": connMsgTypePrefix + trigger,
		},
	}
}

// emitConnectionLocked 는 device_connection 메시지를 msgCh 로 push 한다.
// 호출 전제: a.mu 보유. sendEventLocked("") 는 type 필드를 주입하지 않고 payload 를
// 그대로 marshal 하여 sendToMsgCh(ring buffer drop-oldest) 로 보낸다.
func (a *LGAPAgent) emitConnectionLocked(dev *LGAPDevice, trigger string, eventMs int64) {
	payload := a.buildConnectionPayloadLocked(dev, trigger, eventMs)
	a.sendEventLocked("", payload)
}

// resolveStartupProbes 는 아직 initial 을 방출하지 않은 device 를 처리한다.
// force=false: online 확정은 handleResponse 경로가 담당하므로 여기서는 미방출 device
// 존재 여부만 확인한다. force=true(타임아웃): 남은 미방출 device 를 initial=offline 으로
// 확정한다(단일 실패/타임아웃/transport 미가용 → 즉시 offline, threshold 우회, N8).
// 모든 device 가 initial 방출을 마쳤으면 true 를 반환한다.
//
// N10: Stop 이 시작되었으면 어떤 메시지도 방출하지 않고 종료로 간주한다.
func (a *LGAPAgent) resolveStartupProbes(force bool) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	select {
	case <-a.stopCh:
		return true
	default:
	}

	now := time.Now().UnixMilli()
	allDone := true
	for _, dev := range a.devices {
		if dev.connInitialEmitted {
			continue
		}
		if force {
			// 통신 확인 실패 → offline baseline 확정. payload 일관성을 위해 Online 도
			// false 로 확정한다(이후 첫 성공 poll 이 E3 online change 를 방출).
			dev.Online = false
			dev.connInitialEmitted = true
			a.emitConnectionLocked(dev, connTriggerInitial, now)
			continue
		}
		allDone = false
	}
	return allDone
}

// startupProbeLoop 은 비동기 startup probe 를 수행한다 (E6, OQ-B).
// Start 는 이 goroutine 을 띄우고 즉시 반환한다. 각 device 는 첫 성공 통신 도착 시
// handleResponse 에서 initial=online 을, startup_probe_timeout 내 미확정 시 여기서
// initial=offline 을 방출한다. connWg 로 join 되어 Stop 시 leak 이 없다(§7.2/N10).
func (a *LGAPAgent) startupProbeLoop() {
	defer a.connWg.Done()

	if a.resolveStartupProbes(false) {
		return
	}

	timeout := a.lgapConfig.StartupProbeTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	deadline := time.Now().Add(timeout)

	interval := probeCheckInterval
	if timeout < interval {
		interval = timeout
	}
	if interval <= 0 {
		interval = time.Millisecond
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			if time.Now().After(deadline) {
				a.resolveStartupProbes(true)
				return
			}
			if a.resolveStartupProbes(false) {
				return
			}
		}
	}
}

// connectionReportLoop 은 connection_report_interval 마다 등록된 모든 device 에 대해
// device_connection.report 를 방출한다 (E5, S1). ConnectionReportInterval>0 일 때만
// Start 에서 기동된다. connWg 로 join 된다.
func (a *LGAPAgent) connectionReportLoop() {
	defer a.connWg.Done()

	ticker := time.NewTicker(a.lgapConfig.ConnectionReportInterval)
	defer ticker.Stop()

	for {
		select {
		case <-a.stopCh:
			return
		case <-ticker.C:
			a.emitConnectionReports()
		}
	}
}

// emitConnectionReports 는 등록된 모든 device 에 대해 report 를 방출한다.
// offline device 도 포함하며(S2/N5), 아직 initial 이 방출되지 않은 device 는 skip
// 한다(S5) — per-device 순서(E9) 보장. event_ms 는 tick 시점 time.Now()(S3).
func (a *LGAPAgent) emitConnectionReports() {
	a.mu.Lock()
	defer a.mu.Unlock()

	select {
	case <-a.stopCh:
		return
	default:
	}

	now := time.Now().UnixMilli()
	for _, dev := range a.devices {
		if !dev.connInitialEmitted {
			continue
		}
		a.emitConnectionLocked(dev, connTriggerReport, now)
	}
}
