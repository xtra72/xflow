package chirpstack

import (
	"encoding/json"
	"sync"
	"testing"
)

// TestCommSnapshot_CachedEntry 는 캐시된 comm 엔트리가 값 복사로 반환되고, 반환값
// 변조가 맵 내부 상태에 영향을 주지 않는지 검증한다 (REQ-M3-01/03).
func TestCommSnapshot_CachedEntry(t *testing.T) {
	a := newCommAgent(t, "snap-cs", nil)
	const devEui = "24e124141d180806"

	a.commMu.Lock()
	a.comm[devEui] = &commEntry{
		lastSeenMs: 1_754_954_721_129,
		online:     true,
		rssi:       -90,
		snr:        7.5,
		gatewayID:  "gw1",
	}
	a.commMu.Unlock()

	snap, ok := a.CommSnapshot(devEui)
	if !ok {
		t.Fatal("CommSnapshot ok=false, want true")
	}
	if !snap.online || snap.rssi != -90 || snap.snr != 7.5 || snap.gatewayID != "gw1" {
		t.Errorf("snapshot = %+v, 캐시 값과 불일치", snap)
	}
	if snap.lastSeenMs != 1_754_954_721_129 {
		t.Errorf("lastSeenMs = %d, want 1754954721129", snap.lastSeenMs)
	}

	// 값 복사 확인: 반환된 스냅샷 변조가 맵에 반영되어서는 안 된다.
	snap.online = false
	again, _ := a.CommSnapshot(devEui)
	if !again.online {
		t.Error("반환 스냅샷 변조가 comm 맵에 반영되었다 — 값 복사가 아니다")
	}
}

// TestCommSnapshot_MissingEntry 는 엔트리 부재/빈 devEui 가 zero-value + ok=false 로
// 처리되는지 검증한다 (REQ-M3-04).
func TestCommSnapshot_MissingEntry(t *testing.T) {
	a := newCommAgent(t, "snap-miss-cs", nil)

	for _, devEui := range []string{"", "unknown-dev"} {
		snap, ok := a.CommSnapshot(devEui)
		if ok {
			t.Errorf("devEui=%q: ok=true, want false", devEui)
		}
		if snap.online {
			t.Errorf("devEui=%q: 엔트리 부재인데 online=true", devEui)
		}
		if snap.lastSeenMs != 0 {
			t.Errorf("devEui=%q: lastSeenMs = %d, want 0", devEui, snap.lastSeenMs)
		}
	}
}

// TestCommStateRecordJSON_Shape 는 경계 접근자가 수신 경로와 동일한 device_state
// 레코드 shape 를 만드는지 검증한다 (REQ-M3-01, AC-2).
func TestCommStateRecordJSON_Shape(t *testing.T) {
	a := newCommAgent(t, "snapjson-cs", nil)
	const devEui = "24e124141d180806"
	const lastSeen = int64(1_754_954_721_129)

	a.commMu.Lock()
	a.comm[devEui] = &commEntry{
		lastSeenMs: lastSeen, online: true, rssi: -90, snr: 7.5, gatewayID: "gw1",
	}
	a.commMu.Unlock()

	b, err := a.CommStateRecordJSON(devEui, commTriggerReport)
	if err != nil {
		t.Fatalf("CommStateRecordJSON: %v", err)
	}

	var rec deviceStateRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("레코드 JSON 파싱 실패: %v (%s)", err, b)
	}

	want := buildDeviceStateRecord(devEui, commTriggerReport, commEntry{
		lastSeenMs: lastSeen, online: true, rssi: -90, snr: 7.5, gatewayID: "gw1",
	})
	if rec != want {
		t.Errorf("record = %+v, want %+v (수신 경로 빌더와 동일해야 한다)", rec, want)
	}
	if rec.Record != recordKindDeviceState {
		t.Errorf("record 판별자 = %q, want %q", rec.Record, recordKindDeviceState)
	}
	if rec.LastSeenMs != lastSeen || rec.State.LastSeenMs != lastSeen {
		t.Errorf("last_seen_ms 노출 불일치: top=%d state=%d", rec.LastSeenMs, rec.State.LastSeenMs)
	}
}

// TestCommStateRecordJSON_MissingEntryOffline 는 엔트리 부재 시 offline/unknown
// 레코드가 만들어지는지 검증한다 (REQ-M3-04, AC-2b).
func TestCommStateRecordJSON_MissingEntryOffline(t *testing.T) {
	a := newCommAgent(t, "snapjson-miss-cs", nil)

	b, err := a.CommStateRecordJSON("never-seen", commTriggerReport)
	if err != nil {
		t.Fatalf("CommStateRecordJSON: %v", err)
	}
	var rec deviceStateRecord
	if err := json.Unmarshal(b, &rec); err != nil {
		t.Fatalf("레코드 JSON 파싱 실패: %v", err)
	}
	if rec.State.Online {
		t.Fatal("엔트리 부재인데 online=true — 조기 보고 금지 (REQ-M3-04)")
	}
	if rec.LastSeenMs != 0 || rec.State.LastSeenMs != 0 {
		t.Errorf("엔트리 부재 last_seen_ms = %d/%d, want 0", rec.LastSeenMs, rec.State.LastSeenMs)
	}
	if rec.UnitID != "never-seen" {
		t.Errorf("unit_id = %q, want never-seen", rec.UnitID)
	}
}

// TestCommStateEnabled 는 emit_comm_state 노브 노출을 검증한다 (status 노드의 Init
// 전제조건 경고 소스).
func TestCommStateEnabled(t *testing.T) {
	on := newCommAgent(t, "common-cs", nil) // newCommAgent 는 emit_comm_state=true.
	if !on.CommStateEnabled() {
		t.Error("emit_comm_state=true 인데 CommStateEnabled()=false")
	}

	off := newCommAgent(t, "commoff-cs", map[string]any{"emit_comm_state": false})
	if off.CommStateEnabled() {
		t.Error("emit_comm_state=false 인데 CommStateEnabled()=true")
	}
}

// TestCommSnapshot_ConcurrentWithUplink 는 스냅샷 조회가 업링크 comm 갱신과 동시에
// 수행되어도 데이터 레이스/데드락이 없는지 검증한다 (REQ-FROZEN-B, R4).
//
// -race 실행 시 commMu 보유 중 a.Name()(a.mu) 재획득으로 생기는 락 순서 엣지가
// 있었다면 여기서 드러난다.
func TestCommSnapshot_ConcurrentWithUplink(t *testing.T) {
	a := newCommAgent(t, "snapconc-cs", nil)
	const devEui = "24e124141d180806"

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			a.onUplinkCommState(&uplink{
				DeviceInfo: uplinkDeviceInfo{DevEui: devEui},
			})
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_, _ = a.CommSnapshot(devEui)
			_, _ = a.CommStateRecordJSON(devEui, commTriggerReport)
			_ = a.Name() // 락 미보유 상태에서의 정상 호출 경로.
		}
	}()

	wg.Wait()
}
