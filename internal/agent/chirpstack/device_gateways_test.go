package chirpstack

import (
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// F-4 — 디바이스 측 역방향 뷰 (state.properties.gateways)
//
// SPEC-CHIRPSTACK-003 이 v1 에서 Non-Goal 로 미룬 항목이다. 본 파일은 두 가지를
// 함께 검증한다:
//   - 제거(before): rssi / snr / gateway_id 스칼라가 properties 에서 사라졌다.
//   - 도입(after): gateways 배열이 그 자리를 대체하되, 스칼라가 주지 못하던 것
//     (게이트웨이 전량 + emit_comm_state 비종속)까지 준다.
// ---------------------------------------------------------------------------

// nowUplinkTime 은 "방금 수신한" 업링크의 time 필드 문자열을 만든다.
//
// 고정 과거 시각(픽스처의 2026-08-11...)을 쓰면 lastSeenMs 가 기본 임계(300s)를
// 훌쩍 넘겨 모든 링크가 stale 로 파생된다 — staleness 를 검증 대상으로 삼는
// 테스트에서 "항상 stale" 은 아무것도 증명하지 못하므로, 신선도가 의미 있는
// 케이스에서는 현재 시각을 쓴다.
func nowUplinkTime() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// deviceGateways 는 첫 번째 디바이스의 state.properties["gateways"] 를 꺼낸다.
func deviceGateways(t *testing.T, a *ChirpStackAgent) []deviceGatewayView {
	t.Helper()
	props := deviceProps(t, a)
	gws, ok := props["gateways"].([]deviceGatewayView)
	if !ok {
		t.Fatalf("properties[gateways] = %#v (%T), want []deviceGatewayView", props["gateways"], props["gateways"])
	}
	return gws
}

// gatewaysByID 는 게이트웨이 뷰를 gateway_id 로 색인한다.
func gatewaysByID(gws []deviceGatewayView) map[string]deviceGatewayView {
	out := make(map[string]deviceGatewayView, len(gws))
	for _, g := range gws {
		out[g.GatewayID] = g
	}
	return out
}

// setLinkLastSeen 은 로스터 링크의 lastSeenMs 를 소급 조작한다(실시간 대기 회피).
func setLinkLastSeen(t *testing.T, a *ChirpStackAgent, devEui, gatewayID string, lastSeenMs int64) {
	t.Helper()
	a.devicesMu.Lock()
	defer a.devicesMu.Unlock()
	d, ok := a.devices[devEui]
	if !ok {
		t.Fatalf("로스터에 devEui=%s 없음", devEui)
	}
	l, ok := d.links[gatewayID]
	if !ok {
		t.Fatalf("devEui=%s 에 gatewayId=%s 링크 없음", devEui, gatewayID)
	}
	l.lastSeenMs = lastSeenMs
	d.links[gatewayID] = l
}

// TestDeviceProperties_GatewaysPerGateway 는 디바이스를 들은 게이트웨이마다 항목이
// 1개씩 생기고, 각 항목이 자기 게이트웨이의 값을 그대로 갖는지 검증한다 (F-4).
//
// rssi/snr/channel 은 rxInfo 항목별(게이트웨이별) 값이고, frequency_hz/
// spreading_factor/bandwidth 는 txInfo 에서 온 프레임 레벨 값이라 모든 항목이
// 동일해야 한다 (A4, REQ-M1-04) — 두 계층이 섞이지 않았음을 함께 확인한다.
func TestDeviceProperties_GatewaysPerGateway(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-per", nil)

	upTime := nowUplinkTime()
	a.handleUplink(buildUplinkJSON("aabb1001", upTime, 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwB", rssi: -95, snr: 2.5, channelJSON: "5"},
		{gatewayID: "gwA", rssi: -70, snr: 11.25, channelJSON: "1"},
		{gatewayID: "gwC", rssi: -80, snr: 9.0, channelJSON: "3"},
	}), "application/x")

	gws := deviceGateways(t, a)
	if len(gws) != 3 {
		t.Fatalf("gateways 수 = %d, want 3 (게이트웨이마다 1건)", len(gws))
	}

	wantLastSeen := parseUplinkTimeMs(upTime)
	want := map[string]struct {
		rssi    int
		snr     float64
		channel uint32
	}{
		"gwA": {-70, 11.25, 1},
		"gwB": {-95, 2.5, 5},
		"gwC": {-80, 9.0, 3},
	}
	byID := gatewaysByID(gws)
	for id, w := range want {
		got, ok := byID[id]
		if !ok {
			t.Errorf("gateways 에 %s 없음", id)
			continue
		}
		if got.RSSI != w.rssi || got.SNR != w.snr || got.Channel != w.channel {
			t.Errorf("%s = {rssi:%d snr:%v channel:%d}, want {%d %v %d}",
				id, got.RSSI, got.SNR, got.Channel, w.rssi, w.snr, w.channel)
		}
		// 프레임 레벨 값은 게이트웨이와 무관하게 동일하다.
		if got.FrequencyHz != 922100000 || got.SpreadingFactor != 7 || got.Bandwidth != 125000 {
			t.Errorf("%s 프레임 레벨 값 = {freq:%d sf:%d bw:%d}, want {922100000 7 125000}",
				id, got.FrequencyHz, got.SpreadingFactor, got.Bandwidth)
		}
		if got.LastSeenMs != wantLastSeen {
			t.Errorf("%s last_seen_ms = %d, want %d (업링크 파생 시각)", id, got.LastSeenMs, wantLastSeen)
		}
		if got.Stale {
			t.Errorf("%s stale = true — 방금 수신한 링크는 live 여야 한다", id)
		}
	}
}

// TestDeviceProperties_TwoGatewaysDistinctMetrics 는 **같은 디바이스를 두 게이트웨이가
// 들었을 때** 항목이 2건 생기고 각 항목의 rssi/snr/channel 이 서로 다른지 검증한다.
//
// 제거된 스칼라 표현이 구조적으로 표현할 수 없던 바로 그 케이스이다: comm 맵은
// bestGateway 가 고른 최대 RSSI 1건만 담으므로 여기서 gwWeak 의 링크는 통째로
// 사라졌다.
func TestDeviceProperties_TwoGatewaysDistinctMetrics(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-two", nil)

	a.handleUplink(buildUplinkJSON("aabb1002", nowUplinkTime(), 922300000, 9, 250000, []rxSpec{
		{gatewayID: "gwStrong", rssi: -57, snr: 13.5, channelJSON: "2"},
		{gatewayID: "gwWeak", rssi: -113, snr: -9.5, channelJSON: "7"},
	}), "application/x")

	gws := deviceGateways(t, a)
	if len(gws) != 2 {
		t.Fatalf("gateways 수 = %d, want 2", len(gws))
	}
	byID := gatewaysByID(gws)
	strong, weak := byID["gwStrong"], byID["gwWeak"]

	if strong.RSSI != -57 || weak.RSSI != -113 {
		t.Errorf("rssi = %d/%d, want -57/-113", strong.RSSI, weak.RSSI)
	}
	if strong.SNR != 13.5 || weak.SNR != -9.5 {
		t.Errorf("snr = %v/%v, want 13.5/-9.5", strong.SNR, weak.SNR)
	}
	if strong.Channel != 2 || weak.Channel != 7 {
		t.Errorf("channel = %d/%d, want 2/7", strong.Channel, weak.Channel)
	}
	// 세 지표가 실제로 서로 다른 값을 갖는지(= 한쪽 값이 복제되지 않았는지) 확인한다.
	if strong.RSSI == weak.RSSI || strong.SNR == weak.SNR || strong.Channel == weak.Channel {
		t.Errorf("두 게이트웨이의 지표가 동일하다 — 게이트웨이별 값이 아니라 복제된 값이다: %+v / %+v", strong, weak)
	}
}

// TestDeviceProperties_CommScalarsRemoved 는 rssi/snr/gateway_id 스칼라가
// properties 에서 **제거**되었음을 두 노브 상태 모두에서 검증한다 (F-4 before/after).
//
// emit_comm_state=true 는 제거 전 코드라면 세 스칼라가 실렸을 조건이다 — comm
// 엔트리가 실제로 존재함을 함께 확인해, "노브가 꺼져서 없는 것"과 "제거되어서
// 없는 것"을 구분한다.
func TestDeviceProperties_CommScalarsRemoved(t *testing.T) {
	withMemDeviceIDRepo(t)

	for _, tc := range []struct {
		name     string
		commOn   bool
		agentTag string
	}{
		{"emit_comm_state=false", false, "dgw-rm-off"},
		{"emit_comm_state=true", true, "dgw-rm-on"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var opts map[string]any
			if tc.commOn {
				opts = map[string]any{"emit_comm_state": true}
			}
			a := newGatewayTestAgent(t, tc.agentTag, opts)
			a.handleUplink(buildUplinkJSON("aabb1003", nowUplinkTime(), 922100000, 7, 125000, []rxSpec{
				{gatewayID: "gwA", rssi: -70, snr: 9.0, channelJSON: "1"},
			}), "application/x")

			// comm 엔트리 존재 여부를 먼저 관측한다(전제 확인).
			_, hasComm := a.CommSnapshot("aabb1003")
			if hasComm != tc.commOn {
				t.Fatalf("comm 엔트리 존재 = %v, want %v — 전제가 성립하지 않는다", hasComm, tc.commOn)
			}

			props := deviceProps(t, a)
			for _, key := range []string{"rssi", "snr", "gateway_id"} {
				if v, ok := props[key]; ok {
					t.Errorf("properties[%s] 존재(=%v) — 스칼라는 gateways 배열로 대체되어 제거되었다", key, v)
				}
			}
			if _, ok := props["gateways"]; !ok {
				t.Error("properties[gateways] 부재 — 스칼라를 대체할 배열이 있어야 한다")
			}
		})
	}
}

// TestDeviceProperties_PopulatedWithoutCommState 는 emit_comm_state=false 에서도
// gateways 가 채워지는지 검증한다 (F-4 의 순증 근거).
//
// 이것이 제거된 스칼라가 **서비스할 수 없던** 케이스이다: comm 맵 갱신은
// emit_comm_state(기본 false) 게이트 뒤에 있어 기본 구성 에이전트에서 세 스칼라는
// 애초에 부재였다. 반면 링크 캐시는 upsertDevice 가 모든 업링크마다 무조건 채운다
// (REQ-M2-01).
func TestDeviceProperties_PopulatedWithoutCommState(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-nocomm", nil) // emit_comm_state 기본값 false.

	if a.CommStateEnabled() {
		t.Fatal("emit_comm_state = true — 기본값은 false 여야 한다(전제)")
	}

	a.handleUplink(buildUplinkJSON("aabb1004", nowUplinkTime(), 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -70, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwB", rssi: -88, snr: 4.25, channelJSON: "6"},
	}), "application/x")

	// comm 맵은 비어 있다(노브가 꺼져 있으므로).
	if _, ok := a.CommSnapshot("aabb1004"); ok {
		t.Fatal("comm 엔트리 존재 — emit_comm_state=false 에서는 채워지지 않아야 한다(전제)")
	}

	props := deviceProps(t, a)
	for _, key := range []string{"rssi", "snr", "gateway_id"} {
		if v, ok := props[key]; ok {
			t.Errorf("properties[%s] 존재(=%v) — comm 파생 스칼라는 부재여야 한다", key, v)
		}
	}

	gws := deviceGateways(t, a)
	if len(gws) != 2 {
		t.Fatalf("gateways 수 = %d, want 2 — 노브와 무관하게 채워져야 한다", len(gws))
	}
	if gws[0].GatewayID != "gwA" || gws[1].GatewayID != "gwB" {
		t.Errorf("gateways = %s/%s, want gwA/gwB", gws[0].GatewayID, gws[1].GatewayID)
	}
	if gws[0].RSSI != -70 || gws[1].RSSI != -88 {
		t.Errorf("rssi = %d/%d, want -70/-88", gws[0].RSSI, gws[1].RSSI)
	}
}

// TestDeviceProperties_GatewaysDeterministicOrder 는 반복 호출이 바이트 동일한
// gateways 배열을 내는지, 그리고 정렬 키가 gateway_id 오름차순인지 검증한다.
//
// 정렬 키 선택을 실제로 구분하기 위해 rxInfo 를 rssi 내림차순과 **어긋나게** 만든다:
// gateway_id 오름차순은 gw01,gw02,... 이지만 rssi 순서는 그 역이다. rssi 정렬이었다면
// 여기서 순서가 뒤집힌다.
//
// offline_threshold 를 크게 잡아 5회 호출 사이에 stale 이 뒤집히지 않게 한다
// (검증 대상은 정렬 결정성이다).
func TestDeviceProperties_GatewaysDeterministicOrder(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-order", map[string]any{"offline_threshold": "24h"})

	// 삽입 순서: gw07 → gw06 → ... → gw00 (맵 순회 무작위성을 실제로 흔든다).
	rx := make([]rxSpec, 0, maxCachedGatewayLinks)
	for i := maxCachedGatewayLinks - 1; i >= 0; i-- {
		rx = append(rx, rxSpec{
			gatewayID:   fmt.Sprintf("gw%02d", i),
			rssi:        -60 - i, // gw00 이 가장 강하다 — rssi 순서 == id 순서의 역이 아니도록 의도적으로 설계.
			snr:         float64(i),
			channelJSON: fmt.Sprintf("%d", i),
		})
	}
	a.handleUplink(buildUplinkJSON("aabb1005", nowUplinkTime(), 922100000, 7, 125000, rx), "application/x")

	first := ""
	for i := 0; i < 5; i++ {
		gws := deviceGateways(t, a)
		if len(gws) != maxCachedGatewayLinks {
			t.Fatalf("반복 %d: gateways 수 = %d, want %d", i, len(gws), maxCachedGatewayLinks)
		}
		// gateway_id 오름차순인지 확인한다.
		for j := 1; j < len(gws); j++ {
			if gws[j-1].GatewayID >= gws[j].GatewayID {
				t.Fatalf("반복 %d: 정렬 위반 %s >= %s (gateway_id 오름차순이어야 한다)",
					i, gws[j-1].GatewayID, gws[j].GatewayID)
			}
		}
		b, err := json.Marshal(gws)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if i == 0 {
			first = string(b)
			continue
		}
		if string(b) != first {
			t.Fatalf("반복 %d 의 직렬화가 1회차와 다르다 — 순회 순서에 의존한다\n1회차: %s\n%d회차: %s",
				i, first, i, string(b))
		}
	}
}

// TestDeviceProperties_GatewaysDeepCopy 는 반환된 gateways 슬라이스를 변조해도
// 로스터와 다음 호출 결과가 오염되지 않는지 검증한다.
//
// 슬라이스는 참조 타입이라 스냅샷의 것을 재사용하면 호출자의 쓰기가 그대로 내부로
// 흘러 들어간다 — listDevices 의 "깊은 복사" 계약이 links 에도 적용되는지의 확인이다.
func TestDeviceProperties_GatewaysDeepCopy(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-copy", map[string]any{"offline_threshold": "24h"})

	a.handleUplink(buildUplinkJSON("aabb1006", nowUplinkTime(), 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -70, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwB", rssi: -88, snr: 4.25, channelJSON: "6"},
	}), "application/x")

	// 1차 조회 결과를 변조한다: 원소 치환 + 슬라이스 확장.
	first := deviceGateways(t, a)
	first[0].GatewayID = "CLOBBERED"
	first[0].RSSI = 9999
	first[1].Stale = true
	_ = append(first, deviceGatewayView{GatewayID: "INJECTED"}) //nolint:staticcheck // 변조 시도 자체가 검증 대상.

	// 2차 조회는 영향을 받지 않아야 한다.
	second := deviceGateways(t, a)
	if len(second) != 2 {
		t.Fatalf("2차 gateways 수 = %d, want 2", len(second))
	}
	if second[0].GatewayID != "gwA" || second[0].RSSI != -70 {
		t.Errorf("2차 gateways[0] = {%s, %d}, want {gwA, -70} — 반환 구조 변조가 전파되었다",
			second[0].GatewayID, second[0].RSSI)
	}
	if second[1].Stale {
		t.Error("2차 gateways[1].stale = true — 변조가 전파되었다")
	}

	// 로스터 원본(스냅샷이 아닌 실체)도 확인한다.
	links := deviceLinks(t, a, "aabb1006")
	if len(links) != 2 {
		t.Fatalf("로스터 링크 수 = %d, want 2", len(links))
	}
	if l, ok := links["gwA"]; !ok || l.rssi != -70 {
		t.Errorf("로스터 gwA = %+v (exists=%v), want rssi -70", l, ok)
	}
	if _, poisoned := links["CLOBBERED"]; poisoned {
		t.Error("로스터에 변조 키가 반영되었다 — 얕은 복사")
	}
}

// TestDeviceProperties_NoLinksOmitsKey 는 링크가 하나도 없는 디바이스가 gateways
// 키를 **아예 생략**하는지(빈 배열이 아니라) 검증한다.
//
// 빈 배열은 "게이트웨이가 하나도 없다"는 적극적 사실처럼 읽히지만 실제 의미는
// "아직 모른다"이다 — measurements 의 부재 표현과 동일한 규약이다.
func TestDeviceProperties_NoLinksOmitsKey(t *testing.T) {
	withMemDeviceIDRepo(t)

	t.Run("rxInfo 없음", func(t *testing.T) {
		a := newGatewayTestAgent(t, "dgw-norx", nil)
		a.handleUplink(buildUplinkJSON("aabb1007", nowUplinkTime(), 922100000, 7, 125000, nil), "application/x")

		props := deviceProps(t, a)
		if v, ok := props["gateways"]; ok {
			t.Errorf("properties[gateways] 존재(=%#v) — 링크가 없으면 키 자체가 부재여야 한다", v)
		}
		// 디바이스 자체는 로스터에 있고 measurement 는 정상 수집된다(부분 실패 아님).
		if _, ok := props["measurements"]; !ok {
			t.Error("properties[measurements] 부재 — 링크 부재가 다른 수집을 막으면 안 된다")
		}
	})

	t.Run("gatewayId 빈 문자열", func(t *testing.T) {
		a := newGatewayTestAgent(t, "dgw-anonrx", nil)
		a.handleUplink(buildUplinkJSON("aabb1008", nowUplinkTime(), 922100000, 7, 125000, []rxSpec{
			{gatewayID: "", rssi: -70, snr: 9.0, channelJSON: "1"},
		}), "application/x")

		if v, ok := deviceProps(t, a)["gateways"]; ok {
			t.Errorf("properties[gateways] 존재(=%#v) — 키잉 불가 링크는 담기지 않는다", v)
		}
	})
}

// TestDeviceProperties_StaleDerivedAtReadTime 는 stale 이 저장값이 아니라 조회 시점
// 파생값인지, 그리고 threshold<=0 이 300s 로 폴백하는지 검증한다 (REQ-M2-05).
func TestDeviceProperties_StaleDerivedAtReadTime(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-stale", map[string]any{"offline_threshold": "300s"})

	a.handleUplink(buildUplinkJSON("aabb1009", nowUplinkTime(), 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwFresh", rssi: -70, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwStale", rssi: -95, snr: 2.5, channelJSON: "5"},
	}), "application/x")

	nowMs := time.Now().UnixMilli()
	setLinkLastSeen(t, a, "aabb1009", "gwFresh", nowMs-10_000)
	setLinkLastSeen(t, a, "aabb1009", "gwStale", nowMs-600_000)

	byID := gatewaysByID(deviceGateways(t, a))
	if byID["gwFresh"].Stale {
		t.Error("gwFresh stale = true, want false")
	}
	if !byID["gwStale"].Stale {
		t.Error("gwStale stale = false, want true")
	}
	// stale 이어도 링크는 삭제되지 않는다(하드 제거는 cap-pressure 로만).
	if got := len(deviceLinks(t, a, "aabb1009")); got != 2 {
		t.Errorf("캐시 링크 수 = %d, want 2 — staleness 만으로 제거하면 안 된다", got)
	}

	// threshold=0 은 defaultOfflineThreshold(300s)로 폴백한다. 0 을 그대로 쓰면
	// 방금 수신한 gwFresh 까지 stale 로 뒤집힌다.
	cc := *a.cs()
	cc.OfflineThreshold = 0
	a.csConfig.Store(&cc)

	byID = gatewaysByID(deviceGateways(t, a))
	if byID["gwFresh"].Stale {
		t.Error("threshold=0: gwFresh stale = true — 기본 임계로 폴백해야 한다")
	}
	if !byID["gwStale"].Stale {
		t.Error("threshold=0: gwStale stale = false, want true")
	}
}

// TestDeviceGatewayView_TagParityWithListGateways 는 디바이스 측 뷰와 list_gateways
// 의 디바이스 링크 뷰가 **동일한 링크 지표 키 이름**을 쓰는지 검증한다.
//
// 프론트엔드가 두 표면에서 같은 타입을 재사용할 수 있어야 한다는 요구의 기계적
// 확인이다. 두 구조체를 따로 두고 있으므로, 한쪽의 태그만 바뀌는 드리프트는 이
// 테스트로만 잡힌다.
func TestDeviceGatewayView_TagParityWithListGateways(t *testing.T) {
	devJSON, err := json.Marshal(deviceGatewayView{})
	if err != nil {
		t.Fatalf("marshal deviceGatewayView: %v", err)
	}
	gwJSON, err := json.Marshal(gatewayLinkView{})
	if err != nil {
		t.Fatalf("marshal gatewayLinkView: %v", err)
	}

	var devKeys, gwKeys map[string]any
	if err := json.Unmarshal(devJSON, &devKeys); err != nil {
		t.Fatalf("unmarshal deviceGatewayView: %v", err)
	}
	if err := json.Unmarshal(gwJSON, &gwKeys); err != nil {
		t.Fatalf("unmarshal gatewayLinkView: %v", err)
	}

	// 링크 지표 8개는 양쪽에 동일한 이름으로 존재해야 한다.
	shared := []string{
		"rssi", "snr", "channel", "frequency_hz",
		"spreading_factor", "bandwidth", "last_seen_ms", "stale",
	}
	for _, k := range shared {
		if _, ok := devKeys[k]; !ok {
			t.Errorf("deviceGatewayView 에 %q 키 없음", k)
		}
		if _, ok := gwKeys[k]; !ok {
			t.Errorf("gatewayLinkView 에 %q 키 없음", k)
		}
	}

	// 식별 축만 다르다: 디바이스 측은 gateway_id 를, 게이트웨이 측은 dev_eui 등을 갖는다.
	if _, ok := devKeys["gateway_id"]; !ok {
		t.Error("deviceGatewayView 에 gateway_id 키 없음")
	}
	for _, k := range []string{"dev_eui", "device_id", "device_name", "device_profile_name"} {
		if _, ok := devKeys[k]; ok {
			t.Errorf("deviceGatewayView 에 %q 키 존재 — 디바이스 식별 필드는 이 축에서 항상 자기 자신이라 무의미하다", k)
		}
	}
	if len(devKeys) != len(shared)+1 {
		t.Errorf("deviceGatewayView 키 수 = %d, want %d (%v)", len(devKeys), len(shared)+1, devKeys)
	}
}

// TestDeviceProperties_ObservedJSON 은 ChirpStack 디바이스가 실제로 방출하는
// state.properties JSON 을 **관측**해 로그로 남긴다 (주장이 아니라 근거).
//
// 프론트엔드는 이 모양을 기준으로 만들어지므로, 키 집합을 함께 고정한다.
func TestDeviceProperties_ObservedJSON(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-observe", map[string]any{"offline_threshold": "24h"})

	// 시각을 고정해 관측 JSON 을 결정적으로 만든다(24h 임계 안이라 stale=false).
	fixedTime := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	a.handleUplink(buildUplinkJSON("24e124141d180806", fixedTime, 922100000, 7, 125000, []rxSpec{
		{gatewayID: "24e124fffef5dccc", rssi: -113, snr: -9.5, channelJSON: "7"},
		{gatewayID: "24e124fffef79304", rssi: -57, snr: 13.5, channelJSON: "2"},
	}), "application/x")

	props := deviceProps(t, a)
	pretty, err := json.MarshalIndent(props, "", "  ")
	if err != nil {
		t.Fatalf("marshal properties: %v", err)
	}
	t.Logf("ChirpStack 디바이스의 state.properties (관측):\n%s", string(pretty))

	// 최상위 키 집합 고정.
	if len(props) != 2 {
		t.Errorf("properties 키 수 = %d, want 2 (gateways, measurements): %v", len(props), props)
	}
	for _, k := range []string{"gateways", "measurements"} {
		if _, ok := props[k]; !ok {
			t.Errorf("properties[%s] 부재", k)
		}
	}

	// gateways 항목의 키 집합 고정(직렬화 결과 기준).
	var round struct {
		Gateways []map[string]any `json:"gateways"`
	}
	if err := json.Unmarshal(pretty, &round); err != nil {
		t.Fatalf("unmarshal properties: %v", err)
	}
	if len(round.Gateways) != 2 {
		t.Fatalf("gateways 수 = %d, want 2", len(round.Gateways))
	}
	wantKeys := []string{
		"gateway_id", "rssi", "snr", "channel", "frequency_hz",
		"spreading_factor", "bandwidth", "last_seen_ms", "stale",
	}
	for _, k := range wantKeys {
		if _, ok := round.Gateways[0][k]; !ok {
			t.Errorf("gateways[0] 에 %q 키 없음: %v", k, round.Gateways[0])
		}
	}
	if len(round.Gateways[0]) != len(wantKeys) {
		t.Errorf("gateways[0] 키 수 = %d, want %d (%v)", len(round.Gateways[0]), len(wantKeys), round.Gateways[0])
	}
	// 정렬은 gateway_id 오름차순이다.
	if round.Gateways[0]["gateway_id"] != "24e124fffef5dccc" || round.Gateways[1]["gateway_id"] != "24e124fffef79304" {
		t.Errorf("gateways 순서 = %v/%v", round.Gateways[0]["gateway_id"], round.Gateways[1]["gateway_id"])
	}
}

// TestDeviceProperties_ConcurrentUplinksAndReads 는 동시 업링크와 Devices()/
// properties() 조회가 데이터 레이스 없이 동작하는지 검증한다 (go test -race).
//
// reader 는 반환된 gateways 슬라이스를 원소까지 순회한다 — 슬라이스가 스냅샷의
// 별칭이면 여기서 -race 가 잡는다.
func TestDeviceProperties_ConcurrentUplinksAndReads(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "dgw-race", map[string]any{"emit_comm_state": true})
	p := a.DeviceProvider()

	const workers = 8
	const iterations = 50

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				// 동일 devEui 에 집중시켜 링크 맵 경합을 최대화한다.
				a.handleUplink(buildUplinkJSON("aabb2001", nowUplinkTime(), 922100000, 7, 125000, []rxSpec{
					{gatewayID: fmt.Sprintf("gw%02d", (w+i)%4), rssi: -60 - i, snr: float64(w), channelJSON: "1"},
					{gatewayID: fmt.Sprintf("gw%02d", (w+i+1)%4), rssi: -90 + i, snr: float64(i), channelJSON: "3"},
				}), "application/x")
			}
		}(w)
	}
	for r := 0; r < workers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				for _, d := range p.Devices() {
					st := d.State()
					gws, ok := st.Properties["gateways"].([]deviceGatewayView)
					if !ok {
						continue
					}
					for j := range gws {
						_ = gws[j].GatewayID
						_ = gws[j].RSSI
						_ = gws[j].Stale
					}
				}
				_ = a.listGateways() // 역인덱싱 경로도 함께 구동한다.
			}
		}()
	}
	wg.Wait()

	if devs := p.Devices(); len(devs) != 1 {
		t.Fatalf("Devices() len = %d, want 1", len(devs))
	}
	if got := len(deviceGateways(t, a)); got == 0 || got > maxCachedGatewayLinks {
		t.Errorf("gateways 수 = %d, want 1..%d", got, maxCachedGatewayLinks)
	}
}
