package xsfm

import "time"

// minMonitorInterval 은 오프라인 감지 모니터 ticker 의 최소 주기이다. offline_timeout 이
// 매우 짧을 때 tick 간격이 0 이 되어 time.NewTicker 가 panic 하는 것을 막는 하한이다.
const minMonitorInterval = time.Millisecond

// startOfflineMonitor 는 offline_timeout 경과 기반 오프라인 감지 모니터 고루틴을 기동한다
// (REQ-XSFM-001-05-03, 양 모드 공통). offline_timeout<=0 이면 감지가 비활성이므로 고루틴을
// 기동하지 않는다(3-way: 0=비활성).
//
// ticker 주기는 offline_timeout 의 1/4 로 잡아, 임계값 경과를 한 tick 이내에 감지한다. 고루틴은
// stopCh 관측 시 즉시 종료하며, Stop 이 monitorWG.Wait 로 종료를 확인해 누수를 방지한다.
func (a *XSFMAgent) startOfflineMonitor() {
	if a.cfg.OfflineTimeout <= 0 {
		return
	}
	interval := a.cfg.OfflineTimeout / 4
	if interval < minMonitorInterval {
		interval = minMonitorInterval
	}

	a.monitorWG.Add(1)
	go func() {
		defer a.monitorWG.Done()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-a.stopCh:
				return
			case <-t.C:
				a.checkOfflineDevices()
			}
		}
	}()
}

// checkOfflineDevices 는 모든 online 디바이스의 LastSeen 을 검사해, offline_timeout 을 초과해
// 갱신되지 않은 디바이스를 오프라인으로 전환하고 device_offline 이벤트를 방출한다 (REQ-05-03).
//
// 동시성: 오프라인 전이는 로스터 락 하에서 수행하고, 방출은 락 해제 후 수행한다(락을 채널
// 송신에 걸쳐 잡지 않는다). never-seen(LastSeen==zero) 디바이스는 stale 판정에서 제외한다.
func (a *XSFMAgent) checkOfflineDevices() {
	timeout := a.cfg.OfflineTimeout
	if timeout <= 0 {
		return
	}
	now := time.Now()

	// 오프라인 전이 후보를 락 하에서 식별·전환하고, 방출용 메타를 스냅샷한다.
	type offEvt struct {
		deviceID   string
		groupID    string
		lastSeenMs int64
	}
	var events []offEvt

	a.mu.Lock()
	for id, dev := range a.devices {
		if !dev.Online || dev.LastSeen.IsZero() {
			continue
		}
		if now.Sub(dev.LastSeen) > timeout {
			dev.Online = false
			events = append(events, offEvt{deviceID: id, groupID: dev.GroupID, lastSeenMs: dev.LastSeen.UnixMilli()})
		}
	}
	a.mu.Unlock()

	for _, e := range events {
		a.emitOnlineTransition("device_offline", e.deviceID, e.groupID, false, e.lastSeenMs)
	}
}

// handleLWTOffline 는 LWT(Last Will and Testament) 통지로 디바이스를 오프라인으로 전환하고
// device_offline 이벤트를 방출한다 (REQ-XSFM-001-05-03 path 1, direct 모드 전용).
//
// LWT 토픽 스킴(디바이스 매뉴얼 의존)은 아직 확정되지 않아 구독 배선은 후속 배치로 유보하되,
// 오프라인 전이 로직 자체는 이 메서드로 직접 호출(또는 LWT 토픽 콜백)해 테스트 가능하게 노출한다.
// 이미 오프라인이거나 미등록 디바이스면 no-op(중복 방출 방지).
func (a *XSFMAgent) handleLWTOffline(deviceID string) {
	a.mu.Lock()
	dev := a.devices[deviceID]
	if dev == nil || !dev.Online {
		a.mu.Unlock()
		return
	}
	dev.Online = false
	groupID := dev.GroupID
	lastSeenMs := dev.LastSeen.UnixMilli()
	a.mu.Unlock()

	a.emitOnlineTransition("device_offline", deviceID, groupID, false, lastSeenMs)
}
