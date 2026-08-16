package chirpstack

import (
	"reflect"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// 디바이스 이력 힌트 (device.HistoryComparable / device.HistoryEventTimed)
//
// 이력 레코더는 주기 샘플러이므로 두 가지 프로바이더 고유 사실을 필요로 한다:
//   - 어떤 필드가 **조회 시각 파생**이라 변화로 세면 안 되는가 (gateways[].stale)
//   - 이 properties 가 대표하는 **실제 수신 시각**은 언제인가 (업링크 파생 시각)
// 둘 다 프로바이더만 아는 사실이라 선택적 인터페이스로 답한다.
// ---------------------------------------------------------------------------

// histSnap 은 링크 1개 + measurement 2개를 가진 로스터 스냅샷을 만든다.
func histSnap(lastSeenMs, measTimeMs int64) deviceState {
	return deviceState{
		devEui:     "0011223344556677",
		deviceName: "sensor-1",
		lastSeen:   time.UnixMilli(lastSeenMs),
		measurements: map[string]measurementSample{
			"temperature": {value: 21.5, timeMs: measTimeMs},
			"humidity":    {value: 41.0, timeMs: measTimeMs - 5_000},
		},
		links: map[string]gatewayLink{
			"gw-a": {gatewayID: "gw-a", rssi: -97, lastSeenMs: lastSeenMs},
		},
	}
}

// TestHistoryComparisonProperties_NeutralizesStale 는 비교 표면에서 stale 이
// 중립화되되, payload(properties)에서는 제거되지 않음을 검증한다.
func TestHistoryComparisonProperties_NeutralizesStale(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	// 임계를 아주 짧게 두어 링크가 stale 로 파생되게 한다(대기 없이 결정적).
	a := newChirpDeviceAdapter("agent", histSnap(nowMs-60_000, nowMs), time.Second)

	// payload 에는 stale 이 그대로 실려야 한다 — UI 가 staleness 를 표시한다.
	gws, ok := a.properties()["gateways"].([]deviceGatewayView)
	if !ok {
		t.Fatalf("properties[gateways] 타입 불일치")
	}
	if !gws[0].Stale {
		t.Fatalf("payload 의 stale 은 파생값 그대로여야 한다(want true)")
	}

	// 비교 표면에서는 중립화되어야 한다.
	cmp, has := a.HistoryComparisonProperties()
	if !has {
		t.Fatalf("HistoryComparisonProperties 는 표면을 제공해야 한다")
	}
	cmpGws, ok := cmp["gateways"].([]deviceGatewayView)
	if !ok {
		t.Fatalf("비교 표면 gateways 타입 불일치")
	}
	if cmpGws[0].Stale {
		t.Fatalf("비교 표면의 stale 은 중립화되어야 한다(want false)")
	}

	// 중립화가 payload 를 오염시키지 않아야 한다(깊은 복사 규약).
	again, _ := a.properties()["gateways"].([]deviceGatewayView)
	if !again[0].Stale {
		t.Fatalf("비교 표면 생성이 payload 를 변조했다")
	}
}

// TestHistoryComparisonProperties_StableAcrossStaleFlip 은 stale 이 뒤집혀도
// 비교 표면이 바이트 동일함을 검증한다 — 이것이 주기 스팸을 막는 핵심이다.
//
// 시간 경과는 offline 임계를 달리한 두 어댑터로 대체한다(동일 스냅샷 + 다른 임계
// = stale 만 다른 두 시점).
func TestHistoryComparisonProperties_StableAcrossStaleFlip(t *testing.T) {
	nowMs := time.Now().UnixMilli()
	snap := histSnap(nowMs-60_000, nowMs)

	fresh := newChirpDeviceAdapter("agent", snap, time.Hour)  // stale=false 로 파생
	aged := newChirpDeviceAdapter("agent", snap, time.Second) // stale=true 로 파생

	freshGws, _ := fresh.properties()["gateways"].([]deviceGatewayView)
	agedGws, _ := aged.properties()["gateways"].([]deviceGatewayView)
	if freshGws[0].Stale == agedGws[0].Stale {
		t.Fatalf("전제 실패: 두 시점의 payload stale 이 달라야 한다")
	}

	freshCmp, _ := fresh.HistoryComparisonProperties()
	agedCmp, _ := aged.HistoryComparisonProperties()
	if !reflect.DeepEqual(freshCmp, agedCmp) {
		t.Fatalf("stale 만 뒤집힌 두 시점의 비교 표면은 동일해야 한다\n fresh=%#v\n aged=%#v", freshCmp, agedCmp)
	}
}

// TestHistoryEventTimeMs 는 이벤트 시각이 업링크 파생 시각의 최댓값임을 검증한다.
func TestHistoryEventTimeMs(t *testing.T) {
	t.Run("measurement 가 가장 최신", func(t *testing.T) {
		snap := histSnap(1_000, 9_000)
		a := newChirpDeviceAdapter("agent", snap, time.Minute)
		ms, ok := a.HistoryEventTimeMs()
		if !ok || ms != 9_000 {
			t.Fatalf("HistoryEventTimeMs() = (%d, %v), want (9000, true)", ms, ok)
		}
	})

	t.Run("링크가 가장 최신(디코딩 object 없는 업링크)", func(t *testing.T) {
		snap := histSnap(20_000, 9_000)
		a := newChirpDeviceAdapter("agent", snap, time.Minute)
		ms, ok := a.HistoryEventTimeMs()
		if !ok || ms != 20_000 {
			t.Fatalf("HistoryEventTimeMs() = (%d, %v), want (20000, true)", ms, ok)
		}
	})

	t.Run("아무 시각도 모름 → false", func(t *testing.T) {
		a := newChirpDeviceAdapter("agent", deviceState{devEui: "aa"}, time.Minute)
		ms, ok := a.HistoryEventTimeMs()
		if ok || ms != 0 {
			t.Fatalf("HistoryEventTimeMs() = (%d, %v), want (0, false)", ms, ok)
		}
	})

	t.Run("measurement 만 있고 링크 없음", func(t *testing.T) {
		snap := deviceState{
			devEui:       "aa",
			measurements: map[string]measurementSample{"t": {value: 1, timeMs: 4_242}},
		}
		a := newChirpDeviceAdapter("agent", snap, time.Minute)
		ms, ok := a.HistoryEventTimeMs()
		if !ok || ms != 4_242 {
			t.Fatalf("HistoryEventTimeMs() = (%d, %v), want (4242, true)", ms, ok)
		}
	})
}

// BenchmarkHistoryHints 는 이력 힌트가 스냅샷 주기마다 디바이스당 추가하는 비용을
// 측정한다. 상한 크기(measurements 64개, gateways 8개) 기준이며, 비교 대상은 레코더가
// 어차피 호출하는 properties() 이다 — 비교 표면 생성이 그 위에 얹는 몫이 관심사이다.
func BenchmarkHistoryHints(b *testing.B) {
	nowMs := time.Now().UnixMilli()
	snap := deviceState{
		devEui:       "0011223344556677",
		measurements: make(map[string]measurementSample, 64),
		links:        make(map[string]gatewayLink, 8),
	}
	for i := 0; i < 64; i++ {
		snap.measurements[string(rune('a'+i%26))+string(rune('a'+i/26))] =
			measurementSample{value: float64(i), timeMs: nowMs}
	}
	for i := 0; i < 8; i++ {
		id := "gw-" + string(rune('a'+i))
		snap.links[id] = gatewayLink{gatewayID: id, rssi: -90 - i, lastSeenMs: nowMs}
	}
	a := newChirpDeviceAdapter("agent", snap, time.Minute)

	b.Run("properties(기준선)", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = a.properties()
		}
	})
	b.Run("HistoryComparisonProperties", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = a.HistoryComparisonProperties()
		}
	})
	b.Run("HistoryEventTimeMs", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = a.HistoryEventTimeMs()
		}
	})
}
