package chirpstack

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// newCommAgent 는 emit_comm_state 활성 + 비활성화(브로커 미연결) comm-state 테스트
// 에이전트를 만든다. 비활성화이므로 Init 이 watchdog 을 자동 기동하지 않으며, 테스트가
// comm 메서드/loop 을 직접 호출/기동한다.
func newCommAgent(t *testing.T, name string, extra map[string]any) *ChirpStackAgent {
	t.Helper()
	resetNameRegistryForTest()
	disabled := false
	opts := map[string]any{"emit_comm_state": true}
	for k, v := range extra {
		opts[k] = v
	}
	cfg := agent.AgentConfig{
		ID:        "id-" + name,
		Name:      name,
		Type:      "chirpstack",
		Enabled:   &disabled,
		Transport: agent.TransportConfig{Options: opts},
	}
	raw, err := NewChirpStackAgent(cfg)
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	return raw.(*ChirpStackAgent)
}

// drainRecords 는 수신 채널에 쌓인 레코드를 비블로킹으로 꺼내 event/device_state 로
// 분류한다.
func drainRecords(t *testing.T, a *ChirpStackAgent) (events []measurementRecord, states []deviceStateRecord) {
	t.Helper()
	for {
		select {
		case b := <-a.recvCh:
			var disc struct {
				Record string `json:"record"`
			}
			_ = json.Unmarshal(b, &disc)
			if disc.Record == recordKindDeviceState {
				var s deviceStateRecord
				if err := json.Unmarshal(b, &s); err != nil {
					t.Fatalf("unmarshal device_state: %v", err)
				}
				states = append(states, s)
			} else {
				var m measurementRecord
				if err := json.Unmarshal(b, &m); err != nil {
					t.Fatalf("unmarshal event: %v", err)
				}
				events = append(events, m)
			}
		default:
			return events, states
		}
	}
}

// TestCommState_ChangeOnOnlineTransition 는 첫 업링크(online 전이) 시
// device_state.change 가 최적 게이트웨이 값과 함께 emit 되는지 검증한다
// (AC-5a, REQ-M5-02/05).
func TestCommState_ChangeOnOnlineTransition(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newCommAgent(t, "comm-a", nil)

	a.handleUplink(loadRawUplink(t), "application/x")

	events, states := drainRecords(t, a)
	if len(events) != 1 {
		t.Fatalf("event records = %d, want 1 (per-measurement 유지)", len(events))
	}
	if len(states) != 1 {
		t.Fatalf("device_state records = %d, want 1", len(states))
	}
	s := states[0]
	if s.Trigger != commTriggerChange {
		t.Errorf("trigger = %q, want change", s.Trigger)
	}
	if !s.State.Online {
		t.Error("state.online = false, want true")
	}
	// 최적(최대 rssi) 게이트웨이: rssi=-57, snr=13.5, gatewayId=24e124fffef79304.
	if s.State.RSSI != -57 {
		t.Errorf("state.rssi = %d, want -57", s.State.RSSI)
	}
	if s.State.SNR != 13.5 {
		t.Errorf("state.snr = %v, want 13.5", s.State.SNR)
	}
	if s.State.GatewayID != "24e124fffef79304" {
		t.Errorf("state.gateway_id = %q, want 24e124fffef79304", s.State.GatewayID)
	}
	if s.State.LastSeenMs <= 0 || s.LastSeenMs <= 0 || s.TimeMs <= 0 {
		t.Errorf("last_seen_ms should be > 0 (int64 UnixMilli): state=%d top=%d time=%d",
			s.State.LastSeenMs, s.LastSeenMs, s.TimeMs)
	}
	if s.UnitID != "24e124141d180806" {
		t.Errorf("unit_id = %q, want devEui", s.UnitID)
	}
}

// TestCommState_NoChangeOnSecondUplink 는 이미 online 인 디바이스의 재업링크가
// 추가 change 를 emit 하지 않는지(online 전이 없음) 검증한다.
func TestCommState_NoChangeOnSecondUplink(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newCommAgent(t, "comm-nochange", nil)

	raw := loadRawUplink(t)
	a.handleUplink(raw, "application/x")
	_, states1 := drainRecords(t, a)
	if len(states1) != 1 {
		t.Fatalf("first uplink device_state = %d, want 1", len(states1))
	}

	a.handleUplink(raw, "application/x")
	_, states2 := drainRecords(t, a)
	if len(states2) != 0 {
		t.Errorf("second uplink device_state = %d, want 0 (전이 없음)", len(states2))
	}
}

// TestCommState_StalenessToOffline 는 offline_threshold 초과 시 watchdog 검사가
// offline 전이 + device_state.change(online=false) 를 emit 하는지 검증한다
// (AC-5b, REQ-M5-03).
func TestCommState_StalenessToOffline(t *testing.T) {
	a := newCommAgent(t, "comm-stale", map[string]any{"offline_threshold": 300})

	// 301s 전 마지막 업링크로 seed → 임계(300s) 초과.
	devEui := "24e124141d180806"
	a.commMu.Lock()
	a.comm[devEui] = &commEntry{
		lastSeenMs: time.Now().Add(-301 * time.Second).UnixMilli(),
		online:     true,
		rssi:       -57,
		snr:        13.5,
		gatewayID:  "24e124fffef79304",
	}
	a.commMu.Unlock()

	a.checkStaleness()

	_, states := drainRecords(t, a)
	if len(states) != 1 {
		t.Fatalf("device_state records = %d, want 1 (offline 전이)", len(states))
	}
	if states[0].Trigger != commTriggerChange {
		t.Errorf("trigger = %q, want change", states[0].Trigger)
	}
	if states[0].State.Online {
		t.Error("state.online = true, want false (staleness offline)")
	}

	// 재검사 시 이미 offline 이므로 추가 emit 없음.
	a.checkStaleness()
	_, states2 := drainRecords(t, a)
	if len(states2) != 0 {
		t.Errorf("second check device_state = %d, want 0 (이미 offline)", len(states2))
	}
}

// TestCommState_FreshUplinkNotStale 는 최근 업링크 디바이스는 offline 전이하지
// 않는지 검증한다.
func TestCommState_FreshUplinkNotStale(t *testing.T) {
	a := newCommAgent(t, "comm-fresh", map[string]any{"offline_threshold": 300})
	a.commMu.Lock()
	a.comm["dev"] = &commEntry{lastSeenMs: time.Now().UnixMilli(), online: true}
	a.commMu.Unlock()

	a.checkStaleness()
	_, states := drainRecords(t, a)
	if len(states) != 0 {
		t.Errorf("fresh device offline emits = %d, want 0", len(states))
	}
}

// TestCommState_PeriodicReport 는 emitReports 가 알려진 디바이스에 대해
// device_state.report 를 emit 하는지 검증한다 (AC-5c, REQ-M5-04).
func TestCommState_PeriodicReport(t *testing.T) {
	a := newCommAgent(t, "comm-report", map[string]any{"comm_report_interval": 60})
	a.commMu.Lock()
	a.comm["dev"] = &commEntry{
		lastSeenMs: time.Now().UnixMilli(),
		online:     true,
		rssi:       -57,
		snr:        13.5,
		gatewayID:  "gw",
	}
	a.commMu.Unlock()

	a.emitReports()

	_, states := drainRecords(t, a)
	if len(states) != 1 {
		t.Fatalf("report records = %d, want 1", len(states))
	}
	if states[0].Trigger != commTriggerReport {
		t.Errorf("trigger = %q, want report", states[0].Trigger)
	}
	if !states[0].State.Online || states[0].State.RSSI != -57 {
		t.Errorf("report state mismatch: %+v", states[0].State)
	}
}

// TestCommState_ReportOffButChangeEmitted 는 comm_report_interval==0 시 reportLoop 이
// 즉시 종료(주기 report 미방출)하지만 online 전이 change 는 정상 방출되는지 검증한다
// (AC-5d, REQ-M5-04).
func TestCommState_ReportOffButChangeEmitted(t *testing.T) {
	a := newCommAgent(t, "comm-reportoff", map[string]any{"comm_report_interval": 0})

	// reportLoop(interval=0) 은 즉시 종료해야 한다 → Wait 가 블로킹되지 않음.
	a.wdWg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go a.reportLoop(ctx)

	done := make(chan struct{})
	go func() { a.wdWg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("reportLoop(interval=0) did not exit immediately")
	}

	// 주기 report 는 방출되지 않았지만, online 전이 change 는 정상 방출된다.
	withMemDeviceIDRepo(t)
	a.handleUplink(loadRawUplink(t), "application/x")
	_, states := drainRecords(t, a)
	if len(states) != 1 || states[0].Trigger != commTriggerChange {
		t.Fatalf("expected 1 change record with report off, got %+v", states)
	}
}

// TestCommState_DisabledEmitsNoDeviceState 는 emit_comm_state=false 시 다수 업링크
// 처리에도 어떤 device_state 도 방출되지 않는지 검증한다 (AC-6, REQ-M5-01).
func TestCommState_DisabledEmitsNoDeviceState(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "comm-off") // emit_comm_state 기본 false.

	raw := loadRawUplink(t)
	a.handleUplink(raw, "application/x")
	a.handleUplink(raw, "application/x")

	events, states := drainRecords(t, a)
	if len(states) != 0 {
		t.Errorf("device_state records = %d, want 0 (emit_comm_state=false)", len(states))
	}
	if len(events) != 2 {
		t.Errorf("event records = %d, want 2 (per-measurement 은 유지)", len(events))
	}
}

// TestCommState_RestartUnknownHold 는 재시작 후 last-seen 부재 시(comm 맵 empty)
// online 을 조기 보고하지 않는지 검증한다 (AC 없음 명시 REQ-M5-06).
func TestCommState_RestartUnknownHold(t *testing.T) {
	a := newCommAgent(t, "comm-restart", map[string]any{"comm_report_interval": 60})

	// 첫 업링크 전 — watchdog 검사/report 는 아무 것도 방출하지 않아야 한다.
	a.checkStaleness()
	a.emitReports()
	_, states := drainRecords(t, a)
	if len(states) != 0 {
		t.Errorf("unknown-hold 위반: device_state = %d, want 0 (첫 업링크 전)", len(states))
	}
}

// TestCommWatchdog_GoroutineTermination 은 watchLoop/reportLoop 이 context 취소 시
// 반드시 종료되는지(누수 없음) 검증한다 (REQ-M6-03, AC goroutine-leak).
func TestCommWatchdog_GoroutineTermination(t *testing.T) {
	a := newCommAgent(t, "comm-leak", map[string]any{
		"offline_threshold":    2, // watchLoop tick = 1s.
		"comm_report_interval": 1, // reportLoop tick = 1s.
	})

	base := runtime.NumGoroutine()
	a.startCommWatchdog()
	// 재기동은 idempotent — 추가 goroutine 없음.
	a.startCommWatchdog()

	// 루프가 실제로 tick 을 돌게 잠시 대기.
	time.Sleep(50 * time.Millisecond)

	done := make(chan struct{})
	go func() { a.stopCommWatchdog(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("watchdog goroutines did not terminate on context cancel (leak)")
	}

	// 종료 후 goroutine 수가 baseline 부근으로 회귀해야 한다.
	// (스케줄러 지연 여유로 약간의 슬랙 허용.)
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > base+1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > base+1 {
		t.Errorf("goroutine leak: after stop = %d, baseline = %d", got, base)
	}
}

// TestCommWatchdog_DisabledNoStart 는 emit_comm_state=false 시 startCommWatchdog 이
// no-op(미기동) 인지 검증한다.
func TestCommWatchdog_DisabledNoStart(t *testing.T) {
	a := newRunningTestAgent(t, "comm-nostart") // emit_comm_state=false.
	a.startCommWatchdog()
	a.mu.RLock()
	started := a.wdStarted
	a.mu.RUnlock()
	if started {
		t.Error("watchdog should not start when emit_comm_state=false")
	}
	// stopCommWatchdog 은 미기동 상태에서 no-op 이어야 한다(블로킹 금지).
	a.stopCommWatchdog()
}

// TestCommState_StopStopsWatchdog 는 Stop 이 watchdog 을 종료시키는지 검증한다.
func TestCommState_StopStopsWatchdog(t *testing.T) {
	a := newCommAgent(t, "comm-stopwd", map[string]any{
		"offline_threshold":    2,
		"comm_report_interval": 1,
	})
	a.startCommWatchdog()

	done := make(chan struct{})
	go func() { _ = a.Stop(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Stop did not terminate watchdog goroutines")
	}
}
