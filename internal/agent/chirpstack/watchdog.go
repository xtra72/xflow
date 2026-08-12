package chirpstack

import (
	"context"
	"encoding/json"
	"time"
)

// commEntry 는 devEui 별 comm-state 추적 스냅샷이다 (commMu 보호).
//
// ChirpStack 은 패시브/푸시이므로 online 은 업링크 staleness 로만 추론한다
// (폴 루프/error_count 없음, REQ-M5-03). 재시작 시 이 맵은 비어 있으므로 첫 업링크
// 전까지 어떤 디바이스도 online 으로 보고되지 않는다(unknown 보류, REQ-M5-06).
type commEntry struct {
	lastSeenMs int64   // 마지막 업링크 수신 시각(UnixMilli)
	online     bool    // watchdog 판정 online 여부
	rssi       int     // 최적 게이트웨이 rssi
	snr        float64 // 최적 게이트웨이 snr
	gatewayID  string  // 최적 게이트웨이 gatewayId
}

// startCommWatchdog 은 emit_comm_state 활성화 시 watchLoop/reportLoop 을 기동한다.
// idempotent 하며(중복 기동 방지), Init(Running 진입) 경로에서 호출된다.
//
// context 는 에이전트 생명주기에 종속되며 stopCommWatchdog 이 취소한다 (REQ-M6-03).
func (a *ChirpStackAgent) startCommWatchdog() {
	if !a.csConfig.EmitCommState {
		return
	}
	a.mu.Lock()
	if a.wdStarted {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.wdCancel = cancel
	a.wdStarted = true
	a.mu.Unlock()

	a.wdWg.Add(2)
	go a.watchLoop(ctx)
	go a.reportLoop(ctx)
}

// stopCommWatchdog 은 watchdog goroutine 을 취소하고 종료를 대기한다 (REQ-M6-03).
// idempotent — 미기동 상태면 no-op.
func (a *ChirpStackAgent) stopCommWatchdog() {
	a.mu.Lock()
	cancel := a.wdCancel
	started := a.wdStarted
	a.wdStarted = false
	a.wdCancel = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if started {
		a.wdWg.Wait()
	}
}

// watchLoop 은 주기 tick 으로 각 디바이스의 staleness 를 검사해 offline 전이 시
// device_state.change 를 emit 한다 (REQ-M5-03).
//
// @MX:WARN: context 취소로 반드시 종료되어야 하는 백그라운드 goroutine 이다.
// @MX:REASON: 종료 누락 시 에이전트 Stop 후에도 goroutine 누수 발생 (REQ-M6-03).
func (a *ChirpStackAgent) watchLoop(ctx context.Context) {
	defer a.wdWg.Done()

	// tick 은 offline_threshold 의 절반(최소 1s)으로, 임계 초과를 적시에 감지한다.
	interval := a.csConfig.OfflineThreshold / 2
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkStaleness()
		}
	}
}

// reportLoop 은 comm_report_interval>0 시 주기적으로 device_state.report 를 emit 한다
// (REQ-M5-04). interval==0 이면 즉시 종료한다(주기 report off, change 는 유지).
//
// @MX:WARN: context 취소로 반드시 종료되어야 하는 백그라운드 goroutine 이다.
// @MX:REASON: 종료 누락 시 에이전트 Stop 후에도 goroutine 누수 발생 (REQ-M6-03).
func (a *ChirpStackAgent) reportLoop(ctx context.Context) {
	defer a.wdWg.Done()

	interval := a.csConfig.CommReportInterval
	if interval <= 0 {
		return // 주기 report off — goroutine 즉시 종료.
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.emitReports()
		}
	}
}

// onUplinkCommState 는 업링크 수신 시 last-seen 을 갱신하고, unknown→online 또는
// offline→online 전이 시 device_state.change 를 emit 한다 (REQ-M5-02).
//
// HVAC 락 함정 회피: commMu 보유 중에는 a.Name() 등 재-lock 을 유발하는 메서드를
// 호출하지 않는다. 레코드 enqueue 는 락 해제 후 수행한다.
func (a *ChirpStackAgent) onUplinkCommState(up *uplink) {
	devEui := up.DeviceInfo.DevEui
	if devEui == "" {
		return
	}
	nowMs := time.Now().UnixMilli()
	rssi, snr, gw, _ := bestGateway(up.RxInfo)

	a.commMu.Lock()
	e, existed := a.comm[devEui]
	if !existed {
		e = &commEntry{}
		a.comm[devEui] = e
	}
	wasOnline := existed && e.online
	e.lastSeenMs = nowMs
	e.online = true
	e.rssi, e.snr, e.gatewayID = rssi, snr, gw
	transition := !existed || !wasOnline // unknown→online 또는 offline→online.
	snap := *e
	a.commMu.Unlock()

	if transition {
		a.enqueueRecord(buildDeviceStateRecord(devEui, commTriggerChange, snap))
	}
}

// checkStaleness 는 online 디바이스 중 offline_threshold 초과분을 offline 으로 전이하고
// device_state.change 를 emit 한다 (REQ-M5-03). emit 은 락 해제 후 수행한다.
func (a *ChirpStackAgent) checkStaleness() {
	nowMs := time.Now().UnixMilli()
	thresholdMs := a.csConfig.OfflineThreshold.Milliseconds()

	var toEmit []deviceStateRecord
	a.commMu.Lock()
	for devEui, e := range a.comm {
		if e.online && nowMs-e.lastSeenMs > thresholdMs {
			e.online = false
			toEmit = append(toEmit, buildDeviceStateRecord(devEui, commTriggerChange, *e))
		}
	}
	a.commMu.Unlock()

	for i := range toEmit {
		a.enqueueRecord(toEmit[i])
	}
}

// emitReports 는 알려진 모든 디바이스에 대해 device_state.report 를 emit 한다
// (REQ-M5-04). emit 은 락 해제 후 수행한다.
func (a *ChirpStackAgent) emitReports() {
	var toEmit []deviceStateRecord
	a.commMu.Lock()
	for devEui, e := range a.comm {
		toEmit = append(toEmit, buildDeviceStateRecord(devEui, commTriggerReport, *e))
	}
	a.commMu.Unlock()

	for i := range toEmit {
		a.enqueueRecord(toEmit[i])
	}
}

// enqueueRecord 는 device_state 레코드를 직렬화해 수신 채널에 넣는다. 노드는 record
// 판별자로 event/device_state 를 구분해 빌드한다. 직렬화 실패는 에러 통계로 계상한다.
func (a *ChirpStackAgent) enqueueRecord(rec deviceStateRecord) {
	b, err := json.Marshal(rec)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("chirpstack: device_state 레코드 직렬화 실패",
			"trigger", rec.Trigger, "unit_id", rec.UnitID, "error", err)
		return
	}
	a.enqueue(b, "device_state")
}
