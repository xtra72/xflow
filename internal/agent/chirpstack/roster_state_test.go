package chirpstack

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// fixtureDevEui 는 testdata/packet.json 픽스처의 devEui 이다.
const fixtureDevEui = "24e124141d180806"

// backdateLastSeen 은 로스터 엔트리의 lastSeen 을 과거로 소급 조작한다.
// staleness 판정(FIX-A)을 실시간 대기 없이 검증하기 위한 테스트 전용 헬퍼이다.
func backdateLastSeen(t *testing.T, a *ChirpStackAgent, devEui string, age time.Duration) {
	t.Helper()
	a.devicesMu.Lock()
	defer a.devicesMu.Unlock()
	d, ok := a.devices[devEui]
	if !ok {
		t.Fatalf("로스터에 devEui=%s 없음", devEui)
	}
	d.lastSeen = time.Now().Add(-age)
}

// TestRosterOnline_DerivedFromLastSeen 는 로스터의 online 이 저장된 불리언이 아니라
// lastSeen + offline_threshold 로 파생되는지 검증한다 (FIX-A).
//
// 결함(수정 전): upsertDevice 가 online=true 를 저장하고 아무도 false 로 되돌리지
// 않으므로, 며칠 전 죽은 디바이스도 inventory 노드에서 online:true 로 보고된다.
func TestRosterOnline_DerivedFromLastSeen(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "roster-online")

	a.handleUplink(loadRawUplink(t), "application/x")

	// 갓 수신한 디바이스는 online.
	fresh := a.DeviceProvider().Devices()[0]
	if !fresh.Online() {
		t.Error("Online() = false 직후 업링크, want true")
	}
	if !fresh.State().Online {
		t.Error("State().Online = false 직후 업링크, want true")
	}

	// 기본 임계(300s)를 넘긴 디바이스는 offline 이어야 한다.
	backdateLastSeen(t, a, fixtureDevEui, 10*time.Minute)

	stale := a.DeviceProvider().Devices()[0]
	if stale.Online() {
		t.Error("Online() = true (lastSeen 이 임계 초과), want false")
	}
	if stale.State().Online {
		t.Error("State().Online = true (lastSeen 이 임계 초과), want false")
	}
}

// TestRosterOnline_NeverSeen 은 lastSeen 이 zero 인 엔트리를 offline 으로 보는지
// 검증한다 (업링크 전 unknown 보류 — SPEC-001 REQ-M5-06 과 일관).
func TestRosterOnline_NeverSeen(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "roster-never")

	a.handleUplink(loadRawUplink(t), "application/x")
	a.devicesMu.Lock()
	a.devices[fixtureDevEui].lastSeen = time.Time{}
	a.devicesMu.Unlock()

	if a.DeviceProvider().Devices()[0].Online() {
		t.Error("Online() = true (lastSeen zero), want false")
	}
}

// TestRosterOnline_ZeroThresholdFallsBackToDefault 는 offline_threshold 가 0/미설정
// 일 때 기본값(300s)으로 폴백하는지 검증한다 (FIX-A 의 zero-threshold 결정).
//
// 0 을 그대로 임계로 쓰면 now-lastSeen>0 이 즉시 참이 되어 방금 수신한 디바이스까지
// 조용히 전부 offline 으로 뒤집힌다 — 그 실패 양상을 금지한다.
func TestRosterOnline_ZeroThresholdFallsBackToDefault(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "roster-zero-th")

	cc := *a.cs()
	cc.OfflineThreshold = 0
	a.csConfig.Store(&cc)

	a.handleUplink(loadRawUplink(t), "application/x")

	if !a.DeviceProvider().Devices()[0].Online() {
		t.Error("Online() = false (threshold=0), want true — 기본 임계로 폴백해야 한다")
	}

	// 폴백 임계(300s)를 넘기면 여전히 offline 으로 전이한다.
	backdateLastSeen(t, a, fixtureDevEui, 10*time.Minute)
	if a.DeviceProvider().Devices()[0].Online() {
		t.Error("Online() = true (threshold=0, lastSeen 10분 경과), want false")
	}
}

// ---------------------------------------------------------------------------
// FEAT-B: rssi / snr / gateway_id 를 state.properties 로 노출
// ---------------------------------------------------------------------------

// newCommStateAgent 는 emit_comm_state=true 인 러닝 테스트 에이전트를 만든다.
func newCommStateAgent(t *testing.T, name string) *ChirpStackAgent {
	t.Helper()
	resetNameRegistryForTest()
	cfg := newTestConfig("id-"+name, name)
	cfg.Transport.Options = map[string]any{"emit_comm_state": true}
	raw, err := NewChirpStackAgent(cfg)
	if err != nil {
		t.Fatalf("NewChirpStackAgent: %v", err)
	}
	a, ok := raw.(*ChirpStackAgent)
	if !ok {
		t.Fatalf("unexpected agent type %T", raw)
	}
	return a
}

// deviceProps 는 첫 번째 디바이스의 state.properties 를 반환한다.
func deviceProps(t *testing.T, a *ChirpStackAgent) map[string]any {
	t.Helper()
	devs := a.DeviceProvider().Devices()
	if len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	return devs[0].State().Properties
}

// TestStateProperties_CommFieldsPresent 는 emit_comm_state 활성 + comm 엔트리 존재 시
// rssi/snr/gateway_id 가 정확한 값으로 노출되는지 검증한다 (FEAT-B).
//
// 픽스처 rxInfo 는 게이트웨이 2개이며 bestGateway 는 최대 rssi 를 택한다
// (-57dBm / snr 13.5 / 24e124fffef79304).
func TestStateProperties_CommFieldsPresent(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newCommStateAgent(t, "props-on")

	a.handleUplink(loadRawUplink(t), "application/x")

	props := deviceProps(t, a)
	if got, ok := props["rssi"].(int); !ok || got != -57 {
		t.Errorf("properties[rssi] = %v (%T), want -57 (int)", props["rssi"], props["rssi"])
	}
	if got, ok := props["snr"].(float64); !ok || got != 13.5 {
		t.Errorf("properties[snr] = %v (%T), want 13.5 (float64)", props["snr"], props["snr"])
	}
	if got, ok := props["gateway_id"].(string); !ok || got != "24e124fffef79304" {
		t.Errorf("properties[gateway_id] = %v, want 24e124fffef79304", props["gateway_id"])
	}
}

// TestStateProperties_CommFieldsAbsentWhenDisabled 는 emit_comm_state 가 꺼져 있으면
// rssi/snr/gateway_id 키가 zero-value 가 아니라 아예 부재인지 검증한다 (FEAT-B).
//
// rssi:0 은 "미상"과 "실제 0dBm"을 구분할 수 없는 데이터 품질 함정이므로, 부재는
// 반드시 키 부재로 표현되어야 한다.
func TestStateProperties_CommFieldsAbsentWhenDisabled(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "props-off") // emit_comm_state 기본값 false.

	a.handleUplink(loadRawUplink(t), "application/x")

	props := deviceProps(t, a)
	for _, key := range []string{"rssi", "snr", "gateway_id"} {
		if v, ok := props[key]; ok {
			t.Errorf("properties[%s] 존재(=%v) — emit_comm_state=false 이면 부재여야 한다", key, v)
		}
	}
}

// TestStateProperties_CommFieldsAbsentWithoutEntry 는 emit_comm_state 가 켜져 있어도
// 해당 디바이스의 comm 엔트리가 아직 없으면 키가 부재인지 검증한다 (FEAT-B).
func TestStateProperties_CommFieldsAbsentWithoutEntry(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newCommStateAgent(t, "props-noentry")

	a.handleUplink(loadRawUplink(t), "application/x")

	// comm 엔트리만 제거한다 (로스터는 유지) — "엔트리 부재" 상황 재현.
	a.commMu.Lock()
	delete(a.comm, fixtureDevEui)
	a.commMu.Unlock()

	props := deviceProps(t, a)
	for _, key := range []string{"rssi", "snr", "gateway_id"} {
		if v, ok := props[key]; ok {
			t.Errorf("properties[%s] 존재(=%v) — comm 엔트리 부재 시 키도 부재여야 한다", key, v)
		}
	}
}

// ---------------------------------------------------------------------------
// FEAT-C: measurement 캐시
// ---------------------------------------------------------------------------

// rawUplinkWithObject 는 지정한 object 를 담은 원시 ChirpStack 업링크 JSON 을 만든다.
func rawUplinkWithObject(t *testing.T, obj map[string]any) []byte {
	t.Helper()
	up := map[string]any{
		"time": "2026-08-11T23:32:01.129+00:00",
		"deviceInfo": map[string]any{
			"devEui":            fixtureDevEui,
			"deviceName":        "WS301-180806",
			"deviceProfileName": "WS301",
			"applicationId":     "96b4d719-f23f-40aa-9f94-a0f2d0354342",
			// group/location 은 전용 필드로 승격되고 point 는 Labels 에 남는다
			// (Metadata 승격 경로를 measurement/동시성 테스트에서도 함께 구동한다).
			"tags": map[string]any{"location": "실습실", "group": "3층", "point": "앞문"},
		},
		"object": obj,
		"rxInfo": []any{
			map[string]any{"gatewayId": "gw-1", "rssi": -57, "snr": 13.5},
		},
	}
	b, err := json.Marshal(up)
	if err != nil {
		t.Fatalf("marshal uplink: %v", err)
	}
	return b
}

// measurementsOf 는 첫 디바이스의 state.properties["measurements"] 를 반환한다.
// 각 항목은 {value, time_ms} 객체이다.
func measurementsOf(t *testing.T, a *ChirpStackAgent) map[string]any {
	t.Helper()
	props := deviceProps(t, a)
	m, ok := props["measurements"].(map[string]any)
	if !ok {
		t.Fatalf("properties[measurements] = %v (%T), want map[string]any", props["measurements"], props["measurements"])
	}
	return m
}

// measSample 은 measurements[key] 의 {value, time_ms} 쌍을 꺼낸다.
func measSample(t *testing.T, m map[string]any, key string) (any, int64) {
	t.Helper()
	obj, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("measurements[%s] = %#v, want map[string]any{value,time_ms}", key, m[key])
	}
	ts, ok := obj["time_ms"].(int64)
	if !ok {
		t.Fatalf("measurements[%s].time_ms = %#v (%T), want int64", key, obj["time_ms"], obj["time_ms"])
	}
	return obj["value"], ts
}

// measValue 는 measurements[key].value 만 꺼낸다.
func measValue(t *testing.T, m map[string]any, key string) any {
	t.Helper()
	v, _ := measSample(t, m, key)
	return v
}

// TestMeasurementCache_MergeSemantics 는 새 업링크가 실어온 키만 갱신하고 나머지 키는
// 보존하는 병합 의미를 검증한다 (FEAT-C).
//
// 매 업링크마다 temperature 를 보내지만 battery 는 드물게 보내는 디바이스가 battery
// 값을 잃지 않아야 한다.
func TestMeasurementCache_MergeSemantics(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meas-merge")

	// 업링크 1: temperature + battery.
	a.handleUplink(rawUplinkWithObject(t, map[string]any{
		"temperature": 29.8,
		"battery":     88,
	}), "application/x")

	// 업링크 2: temperature(중첩) 갱신 + humidity(신규). battery 는 실어 오지 않는다.
	a.handleUplink(rawUplinkWithObject(t, map[string]any{
		"temperature": 30.1,
		"humidity":    55.2,
	}), "application/x")

	m := measurementsOf(t, a)
	want := map[string]float64{"temperature": 30.1, "humidity": 55.2, "battery": 88}
	if len(m) != len(want) {
		t.Fatalf("measurements = %v, want %d keys", m, len(want))
	}
	for k, wantV := range want {
		got, ok := measValue(t, m, k).(float64)
		if !ok || got != wantV {
			t.Errorf("measurements[%s].value = %v (%T), want %v", k, m[k], m[k], wantV)
		}
	}
}

// TestMeasurementCache_SkipsNonScalar 는 비스칼라 값(중첩 객체/배열)이 emit 경로와
// 동일하게 skip 되는지 검증한다 (FEAT-C, isScalar 재사용).
func TestMeasurementCache_SkipsNonScalar(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meas-scalar")

	a.handleUplink(rawUplinkWithObject(t, map[string]any{
		"temperature": 21.5,
		"magnet":      "close",
		"ok":          true,
		"nested":      map[string]any{"a": 1},
		"arr":         []any{1, 2, 3},
	}), "application/x")

	m := measurementsOf(t, a)
	if len(m) != 3 {
		t.Errorf("measurements = %v, want 3 스칼라 키만", m)
	}
	for _, k := range []string{"nested", "arr"} {
		if v, ok := m[k]; ok {
			t.Errorf("measurements[%s] 존재(=%v) — 비스칼라는 skip 되어야 한다", k, v)
		}
	}
	if measValue(t, m, "magnet") != "close" || measValue(t, m, "ok") != true {
		t.Errorf("스칼라 값 손실: %v", m)
	}
}

// TestMeasurementCache_DeepCopy 는 반환된 맵을 변조해도 로스터 상태가 오염되지 않는지
// 검증한다 (FEAT-C, listDevices 의 깊은 복사 계약).
func TestMeasurementCache_DeepCopy(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meas-copy")

	a.handleUplink(rawUplinkWithObject(t, map[string]any{"temperature": 21.5}), "application/x")

	// 1차 조회 결과를 두 깊이 모두에서 변조한다: 바깥 맵(키 추가/치환)과 measurement
	// 하나의 안쪽 {value,time_ms} 객체. 안쪽이 별칭이면 여기서 로스터가 오염된다.
	first := measurementsOf(t, a)
	inner, ok := first["temperature"].(map[string]any)
	if !ok {
		t.Fatalf("measurements[temperature] = %#v, want map[string]any", first["temperature"])
	}
	inner["value"] = 999.0
	inner["time_ms"] = int64(1)
	first["temperature"] = "clobbered"
	first["injected"] = "poison"

	// 2차 조회는 변조의 영향을 받지 않아야 한다.
	second := measurementsOf(t, a)
	gotVal, gotTime := measSample(t, second, "temperature")
	if gotVal != 21.5 {
		t.Errorf("measurements[temperature].value = %v, want 21.5 — 반환 구조 변조가 로스터를 오염시켰다", gotVal)
	}
	if gotTime == 1 {
		t.Error("measurements[temperature].time_ms 가 변조값(1) — 안쪽 객체가 별칭이다")
	}
	if _, ok := second["injected"]; ok {
		t.Error("주입된 키가 로스터에 반영되었다 — 얕은 복사")
	}

	// 로스터 내부 맵을 직접 확인한다 (스냅샷이 아니라 원본).
	a.devicesMu.RLock()
	rosterSample := a.devices[fixtureDevEui].measurements["temperature"]
	_, poisoned := a.devices[fixtureDevEui].measurements["injected"]
	a.devicesMu.RUnlock()
	if rosterSample.value != 21.5 || rosterSample.timeMs == 1 || poisoned {
		t.Errorf("로스터 원본 오염: temperature=%+v injected=%v", rosterSample, poisoned)
	}
}

// TestMeasurementCache_KeyCap 은 디바이스당 measurement 키 상한(maxCachedMeasurements)
// 동작을 검증한다 (FEAT-C).
//
// 상한 도달 후: 새 키는 무시되고, 이미 캐시된 키의 값 갱신은 계속 허용된다.
// 채택되는 키는 정렬 순서로 결정적이다(k000..k063).
func TestMeasurementCache_KeyCap(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meas-cap")

	obj := make(map[string]any, maxCachedMeasurements+16)
	for i := 0; i < maxCachedMeasurements+16; i++ {
		obj[fmt.Sprintf("k%03d", i)] = float64(i)
	}
	a.handleUplink(rawUplinkWithObject(t, obj), "application/x")

	m := measurementsOf(t, a)
	if len(m) != maxCachedMeasurements {
		t.Fatalf("measurements 키 수 = %d, want %d (상한)", len(m), maxCachedMeasurements)
	}
	if _, ok := m["k000"]; !ok {
		t.Error("k000 이 없다 — 정렬 순서상 채택되어야 한다")
	}
	if _, ok := m[fmt.Sprintf("k%03d", maxCachedMeasurements)]; ok {
		t.Errorf("k%03d 가 존재 — 상한 초과 키는 무시되어야 한다", maxCachedMeasurements)
	}

	// 상한 도달 후에도 기존 키의 값 갱신은 계속된다. 새 키는 여전히 무시된다.
	a.handleUplink(rawUplinkWithObject(t, map[string]any{
		"k000":      777.0,
		"brand_new": 1.0,
	}), "application/x")

	m2 := measurementsOf(t, a)
	if got := measValue(t, m2, "k000"); got != 777.0 {
		t.Errorf("measurements[k000].value = %v, want 777 — 기존 키 갱신은 허용되어야 한다", got)
	}
	if _, ok := m2["brand_new"]; ok {
		t.Error("brand_new 가 존재 — 상한 도달 후 새 키는 무시되어야 한다")
	}
	if len(m2) != maxCachedMeasurements {
		t.Errorf("measurements 키 수 = %d, want %d (상한 유지)", len(m2), maxCachedMeasurements)
	}
}

// TestMeasurementCache_AbsentWhenNoScalars 는 스칼라 measurement 가 하나도 없으면
// measurements 키 자체가 부재인지 검증한다 (빈 맵을 흘리지 않는다).
func TestMeasurementCache_AbsentWhenNoScalars(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "meas-empty")

	a.handleUplink(rawUplinkWithObject(t, map[string]any{
		"nested": map[string]any{"a": 1},
	}), "application/x")

	props := deviceProps(t, a)
	if v, ok := props["measurements"]; ok {
		t.Errorf("properties[measurements] 존재(=%v) — 스칼라가 없으면 부재여야 한다", v)
	}

	// object 자체가 빈/누락인 업링크도 캐시를 만들지 않는다(그리고 기존 캐시를 지우지도
	// 않는다 — 여기서는 아직 캐시가 없으므로 계속 부재).
	a.handleUplink(rawUplinkWithObject(t, map[string]any{}), "application/x")
	if v, ok := deviceProps(t, a)["measurements"]; ok {
		t.Errorf("properties[measurements] 존재(=%v) — 빈 object 는 캐시를 만들지 않는다", v)
	}
}

// TestDeviceProvider_DeviceLookupNotFound 는 매칭되지 않는 UID/빈 문자열 조회가
// ErrDeviceNotFound 를 반환하는지 검증한다 (deviceAdapters 리팩터링 후 회귀 방지).
func TestDeviceProvider_DeviceLookupNotFound(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newRunningTestAgent(t, "dev-lookup")
	a.handleUplink(loadRawUplink(t), "application/x")
	p := a.DeviceProvider()

	if _, err := p.Device(""); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("Device(\"\") err = %v, want ErrDeviceNotFound", err)
	}
	if _, err := p.Device("00000000-0000-4000-8000-000000000000"); !errors.Is(err, device.ErrDeviceNotFound) {
		t.Errorf("Device(unknown) err = %v, want ErrDeviceNotFound", err)
	}

	// 존재하는 UID 는 여전히 조회되며, 확장 필드도 함께 실린다.
	uid := p.Devices()[0].UID()
	got, err := p.Device(uid)
	if err != nil {
		t.Fatalf("Device(uid) err = %v", err)
	}
	if !got.Online() {
		t.Error("Device(uid).Online() = false, want true")
	}
}

// ---------------------------------------------------------------------------
// 동시성 (-race)
// ---------------------------------------------------------------------------

// TestRoster_ConcurrentUplinksAndReads 는 동시 업링크 처리와 로스터 조회가 데이터
// 레이스 없이 동작하는지 검증한다 (go test -race).
//
// 락 규율(REQ-FROZEN-B) 검증도 겸한다: deviceAdapters 는 devicesMu 와 commMu 를
// 순차적으로만 잡으므로 어떤 인터리빙에서도 교착이 발생하지 않아야 한다.
func TestRoster_ConcurrentUplinksAndReads(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newCommStateAgent(t, "race-cs") // comm 맵도 함께 채워지도록 활성화.
	p := a.DeviceProvider()

	const workers = 8
	const iterations = 50

	var wg sync.WaitGroup
	// 업링크 writer: 동일 devEui 에 집중시켜 로스터 엔트리 경합을 최대화한다.
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				a.handleUplink(rawUplinkWithObject(t, map[string]any{
					"temperature": float64(i),
					"battery":     float64(w),
				}), "application/x")
			}
		}(w)
	}
	// reader: Devices() / listDevices() / Device(uid) 동시 호출.
	for r := 0; r < workers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, d := range p.Devices() {
					_ = d.Online()
					st := d.State()
					// 반환된 맵을 순회하며 읽는다 — 얕은 복사면 여기서 레이스가 잡힌다.
					for k, v := range st.Properties {
						_, _ = k, v
					}
					if m, ok := st.Properties["measurements"].(map[string]any); ok {
						for k, v := range m {
							_ = k
							// 안쪽 {value,time_ms} 객체까지 순회한다 — 중첩 맵이
							// 별칭이면 여기서 -race 가 잡는다.
							if inner, ok := v.(map[string]any); ok {
								for ik, iv := range inner {
									_, _ = ik, iv
								}
							}
						}
					}
					// Metadata 도 함께 읽는다 (Group/Location/Labels 승격 경로).
					md := d.Metadata()
					_, _, _ = md.Group, md.Location, md.Labels[labelKeyDevEui]
					_, _ = p.Device(d.UID())
				}
				_ = a.listDevices()
			}
		}()
	}
	wg.Wait()

	if devs := p.Devices(); len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
}
