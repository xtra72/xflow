package chirpstack

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// rxSpec 는 테스트 업링크의 rxInfo[] 항목 1건을 기술한다.
//
// channelJSON 이 빈 문자열이면 `channel` 키 자체를 생략한다 — proto3 JSON 의 기본값
// 생략(A5)을 재현하는 지점이다.
type rxSpec struct {
	gatewayID   string
	rssi        int
	snr         float64
	channelJSON string
}

// buildUplinkJSON 은 원시 ChirpStack 업링크 JSON 을 조립한다.
//
// 문자열 조립을 쓰는 이유: `channel` 키의 **부재**를 표현해야 하는데, Go 구조체를
// 마샬하면 값 타입 필드는 항상 키가 생성되어 그 케이스를 만들 수 없다.
func buildUplinkJSON(devEui string, timeRFC3339 string, txFreq uint64, sf, bw uint32, rx []rxSpec) []byte {
	var b strings.Builder
	b.WriteString(`{"time":"` + timeRFC3339 + `",`)
	b.WriteString(`"deviceInfo":{"devEui":"` + devEui + `","deviceName":"dev-` + devEui +
		`","deviceProfileName":"WS301","applicationId":"app-1"},`)
	b.WriteString(`"object":{"temperature":21.5},`)
	b.WriteString(fmt.Sprintf(`"txInfo":{"frequency":%d,"modulation":{"lora":{"bandwidth":%d,"codeRate":"CR_4_5","spreadingFactor":%d}}},`, txFreq, bw, sf))
	b.WriteString(`"rxInfo":[`)
	for i, r := range rx {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(fmt.Sprintf(`{"gatewayId":%q,"rssi":%d,"snr":%v`, r.gatewayID, r.rssi, r.snr))
		if r.channelJSON != "" {
			b.WriteString(`,"channel":` + r.channelJSON)
		}
		b.WriteString(`}`)
	}
	b.WriteString(`]}`)
	return []byte(b.String())
}

// newGatewayTestAgent 는 지정 transport 옵션으로 chirpstack 에이전트를 만든다.
func newGatewayTestAgent(t *testing.T, name string, opts map[string]any) *ChirpStackAgent {
	t.Helper()
	resetNameRegistryForTest()
	cfg := newTestConfig("id-"+name, name)
	cfg.Transport.Options = opts
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

// deviceLinks 는 로스터의 링크 캐시 사본을 반환한다(테스트 관찰용).
func deviceLinks(t *testing.T, a *ChirpStackAgent, devEui string) map[string]gatewayLink {
	t.Helper()
	a.devicesMu.RLock()
	defer a.devicesMu.RUnlock()
	d, ok := a.devices[devEui]
	if !ok {
		t.Fatalf("로스터에 devEui=%s 없음", devEui)
	}
	out := make(map[string]gatewayLink, len(d.links))
	for k, v := range d.links {
		out[k] = v
	}
	return out
}

// ---------------------------------------------------------------------------
// AC-1 / AC-1b / AC-1c — 다중 게이트웨이 보존, 순서 무관성, 노브 비종속
// ---------------------------------------------------------------------------

// TestUpsertDevice_MultiGatewayLinksKeyedByGatewayID 는 rxInfo 3건이 gatewayId 로
// 키잉되어 전량 보존되고, 각 링크가 자기 게이트웨이의 값을 그대로 갖는지 검증한다
// (AC-1, REQ-M1-01/03, REQ-M2-01/02).
func TestUpsertDevice_MultiGatewayLinksKeyedByGatewayID(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-multi", nil)

	raw := buildUplinkJSON("aabb0001", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -80, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwB", rssi: -95, snr: 2.5, channelJSON: "5"},
		{gatewayID: "gwC", rssi: -70, snr: 11.25, channelJSON: "3"},
	})
	a.handleUplink(raw, "application/x")

	links := deviceLinks(t, a, "aabb0001")
	if len(links) != 3 {
		t.Fatalf("링크 수 = %d, want 3 (rxInfo 전량 보존)", len(links))
	}

	want := map[string]struct {
		rssi    int
		snr     float64
		channel uint32
	}{
		"gwA": {-80, 9.0, 1},
		"gwB": {-95, 2.5, 5},
		"gwC": {-70, 11.25, 3},
	}
	for id, w := range want {
		got, ok := links[id]
		if !ok {
			t.Fatalf("gatewayId=%s 링크 없음 (배열 인덱스 키잉 의심)", id)
		}
		if got.rssi != w.rssi || got.snr != w.snr || got.channel != w.channel {
			t.Errorf("%s = {rssi:%d snr:%v channel:%d}, want {rssi:%d snr:%v channel:%d} "+
				"(최대 RSSI 항목 값으로 덮이면 안 됨)",
				id, got.rssi, got.snr, got.channel, w.rssi, w.snr, w.channel)
		}
	}
}

// TestBestGateway_UnchangedByMultiGatewayLinks 는 comm 맵이 종전대로 최대 RSSI
// 1건만 반영하는지 검증한다 (AC-1 후단, REQ-M1-05 / REQ-FROZEN-C 무회귀).
func TestBestGateway_UnchangedByMultiGatewayLinks(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-best", map[string]any{"emit_comm_state": true})

	raw := buildUplinkJSON("aabb0002", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -80, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwB", rssi: -95, snr: 2.5, channelJSON: "5"},
		{gatewayID: "gwC", rssi: -70, snr: 11.25, channelJSON: "3"},
	})
	a.handleUplink(raw, "application/x")

	snaps := a.commSnapshots()
	e, ok := snaps["aabb0002"]
	if !ok {
		t.Fatal("comm 엔트리 없음")
	}
	if e.gatewayID != "gwC" || e.rssi != -70 || e.snr != 11.25 {
		t.Errorf("comm 엔트리 = {gw:%s rssi:%d snr:%v}, want {gwC -70 11.25} (best-gateway 스칼라 무변경)",
			e.gatewayID, e.rssi, e.snr)
	}
}

// TestBuildGatewayLinks_ShuffleInvariant 는 rxInfo 배열 순서를 섞어도 링크 결과가
// 완전히 동일한지 검증한다 (AC-1b, REQ-M1-03, A2/R8).
func TestBuildGatewayLinks_ShuffleInvariant(t *testing.T) {
	base := []rxSpec{
		{gatewayID: "gwA", rssi: -80, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwB", rssi: -95, snr: 2.5, channelJSON: "5"},
		{gatewayID: "gwC", rssi: -70, snr: 11.25, channelJSON: "3"},
	}
	// 3! 순열 전량 — 어떤 순서로 와도 동일한 결과여야 한다.
	perms := [][]int{
		{0, 1, 2}, {0, 2, 1}, {1, 0, 2}, {1, 2, 0}, {2, 0, 1}, {2, 1, 0},
	}

	var reference []gatewayLink
	for _, p := range perms {
		shuffled := []rxSpec{base[p[0]], base[p[1]], base[p[2]]}
		up, err := decodeUplink(buildUplinkJSON("aabb0003", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, shuffled))
		if err != nil {
			t.Fatalf("decodeUplink: %v", err)
		}
		got := buildGatewayLinks(up, 1765432100000)
		if reference == nil {
			reference = got
			continue
		}
		if len(got) != len(reference) {
			t.Fatalf("순열 %v: 링크 수 = %d, want %d", p, len(got), len(reference))
		}
		for i := range got {
			if got[i] != reference[i] {
				t.Errorf("순열 %v: 링크[%d] = %+v, want %+v (rxInfo 순서에 불변이어야 함)", p, i, got[i], reference[i])
			}
		}
	}
}

// TestBuildGatewayLinks_DuplicateGatewayIDOrderInvariant 는 같은 업링크에 동일
// gatewayId 가 중복으로 실려 와도(정상 ChirpStack 에서는 발생하지 않음) 결과가
// rxInfo 순서에 불변인지 검증한다 (REQ-M1-03 순서 무관성의 엣지 케이스).
//
// "먼저 나온 항목 채택" 은 순서 의존이므로 쓰지 않는다 — betterRxSample 의 전순서로
// 결정되어 어느 순서로 와도 같은 항목이 남는다.
func TestBuildGatewayLinks_DuplicateGatewayIDOrderInvariant(t *testing.T) {
	forward := []rxSpec{
		{gatewayID: "gwDup", rssi: -90, snr: 3.0, channelJSON: "1"},
		{gatewayID: "gwDup", rssi: -70, snr: 8.0, channelJSON: "2"},
		{gatewayID: "gwOther", rssi: -80, snr: 5.0, channelJSON: "3"},
	}
	reversed := []rxSpec{forward[2], forward[1], forward[0]}

	decode := func(rx []rxSpec) []gatewayLink {
		t.Helper()
		up, err := decodeUplink(buildUplinkJSON("aabb0009", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, rx))
		if err != nil {
			t.Fatalf("decodeUplink: %v", err)
		}
		return buildGatewayLinks(up, 1765432100000)
	}

	a, b := decode(forward), decode(reversed)
	if len(a) != 2 || len(b) != 2 {
		t.Fatalf("링크 수 = %d/%d, want 2/2 (중복 gatewayId 는 1건으로 접힌다)", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("링크[%d]: forward=%+v, reversed=%+v (순서 불변이어야 함)", i, a[i], b[i])
		}
	}
	// 전순서상 최대 RSSI 항목이 남는다.
	if a[0].gatewayID != "gwDup" || a[0].rssi != -70 || a[0].channel != 2 {
		t.Errorf("gwDup = %+v, want {rssi:-70 channel:2}", a[0])
	}
}

// TestBetterRxSample_TotalOrder 는 중복 항목 타이브레이커가 완전한 전순서를
// 이루는지(비대칭 + 값만으로 결정) 검증한다 — 순서 무관성의 기반이다.
func TestBetterRxSample_TotalOrder(t *testing.T) {
	cases := []struct {
		name string
		a, b gatewayLink
		want bool
	}{
		{"rssi 우선", gatewayLink{rssi: -70, snr: 0}, gatewayLink{rssi: -80, snr: 99}, true},
		{"rssi 열세", gatewayLink{rssi: -80, snr: 99}, gatewayLink{rssi: -70, snr: 0}, false},
		{"rssi 동률 → snr", gatewayLink{rssi: -70, snr: 9}, gatewayLink{rssi: -70, snr: 3}, true},
		{"rssi 동률 → snr 열세", gatewayLink{rssi: -70, snr: 3}, gatewayLink{rssi: -70, snr: 9}, false},
		{"rssi/snr 동률 → channel", gatewayLink{rssi: -70, snr: 3, channel: 7}, gatewayLink{rssi: -70, snr: 3, channel: 2}, true},
		{"완전 동률 → false(비대칭)", gatewayLink{rssi: -70, snr: 3, channel: 2}, gatewayLink{rssi: -70, snr: 3, channel: 2}, false},
	}
	for _, tc := range cases {
		if got := betterRxSample(tc.a, tc.b); got != tc.want {
			t.Errorf("%s: betterRxSample = %v, want %v", tc.name, got, tc.want)
		}
		// 비대칭성: a>b 이면 b>a 는 반드시 거짓이어야 한다.
		if tc.want && betterRxSample(tc.b, tc.a) {
			t.Errorf("%s: 양방향 모두 참 — 전순서 위반", tc.name)
		}
	}
}

// TestBuildGatewayLinks_EmptyInputs 는 rxInfo 부재/빈 gatewayId 의 방어적 처리를
// 검증한다 (링크 캐시는 gatewayId 로만 키잉 가능하므로 익명 항목은 담지 않는다).
func TestBuildGatewayLinks_EmptyInputs(t *testing.T) {
	if got := buildGatewayLinks(nil, 1); got != nil {
		t.Errorf("nil uplink → %v, want nil", got)
	}
	if got := buildGatewayLinks(&uplink{}, 1); got != nil {
		t.Errorf("rxInfo 없음 → %v, want nil", got)
	}

	up, err := decodeUplink(buildUplinkJSON("aabb0010", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "", rssi: -80, snr: 1.0, channelJSON: "1"},
		{gatewayID: "gwReal", rssi: -70, snr: 2.0, channelJSON: "2"},
	}))
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}
	links := buildGatewayLinks(up, 1765432100000)
	if len(links) != 1 || links[0].gatewayID != "gwReal" {
		t.Errorf("링크 = %+v, want gwReal 1건만 (빈 gatewayId 는 스킵)", links)
	}
}

// TestListGateways_FilledWhenCommStateDisabled 는 emit_comm_state=false(기본값)
// 에서도 링크 캐시가 채워지고 list_gateways 가 게이트웨이를 반환하는지 검증한다
// (AC-1c, REQ-M2-01 — 노브 종속 침묵 실패 금지).
func TestListGateways_FilledWhenCommStateDisabled(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-nocomm", nil) // emit_comm_state 기본 false.

	if a.cs().EmitCommState {
		t.Fatal("전제 위반: emit_comm_state 기본값은 false 여야 한다")
	}

	raw := buildUplinkJSON("aabb0004", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -80, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwB", rssi: -95, snr: 2.5, channelJSON: "5"},
	})
	a.handleUplink(raw, "application/x")

	// comm 맵은 비어 있어야 한다(노브 off).
	if got := len(a.commSnapshots()); got != 0 {
		t.Errorf("comm 스냅샷 수 = %d, want 0 (emit_comm_state=false)", got)
	}
	// 그럼에도 게이트웨이 로스터는 채워져야 한다.
	gws := a.listGateways()
	if len(gws) != 2 {
		t.Fatalf("게이트웨이 수 = %d, want 2 (로스터 경로는 노브와 무관)", len(gws))
	}
}

// ---------------------------------------------------------------------------
// AC-2 / AC-2b — channel 값 타입 파싱, 프레임 레벨 txInfo
// ---------------------------------------------------------------------------

// TestDecodeUplink_ChannelAbsentKeyIsZero 는 channel 키의 명시/0명시/부재 3종이
// 모두 값 타입 uint32 로 파싱되는지 검증한다 (AC-2, REQ-M1-02, A5).
//
// (b) `"channel": 0` 과 (c) 키 부재는 **구분 불가능**하며 둘 다 0 이다 — 이것이
// 값 타입을 강제하는 이유이다(uplinkRxInfo 주석 참조).
func TestDecodeUplink_ChannelAbsentKeyIsZero(t *testing.T) {
	up, err := decodeUplink(buildUplinkJSON("aabb0005", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwExplicit3", rssi: -80, snr: 9.0, channelJSON: "3"},
		{gatewayID: "gwExplicit0", rssi: -81, snr: 9.0, channelJSON: "0"},
		{gatewayID: "gwAbsent", rssi: -82, snr: 9.0, channelJSON: ""}, // 키 자체 없음.
	}))
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}
	if len(up.RxInfo) != 3 {
		t.Fatalf("rxInfo 수 = %d, want 3", len(up.RxInfo))
	}

	cases := []struct {
		name string
		idx  int
		want uint32
	}{
		{"명시 3", 0, 3},
		{"명시 0", 1, 0},
		{"키 부재 → 0 (미상 아님)", 2, 0},
	}
	for _, tc := range cases {
		if got := up.RxInfo[tc.idx].Channel; got != tc.want {
			t.Errorf("%s: channel = %d, want %d", tc.name, got, tc.want)
		}
	}

	// 키 부재와 명시 0 이 구분 불가능함을 명시적으로 고정한다.
	if up.RxInfo[1].Channel != up.RxInfo[2].Channel {
		t.Error("명시 0 과 키 부재가 서로 다른 값으로 파싱됨 — nullable 표현 의심")
	}
}

// TestBuildGatewayLinks_FrameLevelTxInfoAttachedToAll 은 프레임 레벨 txInfo 가
// 그 업링크의 **모든** 링크에 동일하게 부착되는지 검증한다 (AC-2b, REQ-M1-04).
func TestBuildGatewayLinks_FrameLevelTxInfoAttachedToAll(t *testing.T) {
	up, err := decodeUplink(buildUplinkJSON("aabb0006", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -80, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwB", rssi: -95, snr: 2.5, channelJSON: "5"},
		{gatewayID: "gwC", rssi: -70, snr: 11.25, channelJSON: "3"},
	}))
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}
	links := buildGatewayLinks(up, 1765432100000)
	if len(links) != 3 {
		t.Fatalf("링크 수 = %d, want 3", len(links))
	}
	for _, l := range links {
		if l.frequencyHz != 922100000 || l.spreadingFactor != 7 || l.bandwidth != 125000 {
			t.Errorf("%s = {freq:%d sf:%d bw:%d}, want {922100000 7 125000} (프레임 레벨 공통값)",
				l.gatewayID, l.frequencyHz, l.spreadingFactor, l.bandwidth)
		}
	}
}

// TestDecodeUplink_FixtureTxInfoAndChannel 은 **실제 캡처 픽스처**(testdata/packet.json)
// 로 txInfo / channel 파싱을 검증한다 (AC-2b, REQ-M5-04 실측 대조).
//
// 이 픽스처의 channel 은 6 과 7 이며 **0 인 항목이 없다** — 따라서 A5(`channel: 0`
// 이 실제 ChirpStack JSON 에서 생략되는지)는 이 픽스처로 확인되지 않는다. 해당 가정은
// 여전히 미검증이며 R1 로 남는다.
func TestDecodeUplink_FixtureTxInfoAndChannel(t *testing.T) {
	up, err := decodeUplink(loadRawUplink(t))
	if err != nil {
		t.Fatalf("decodeUplink: %v", err)
	}
	if up.TxInfo.Frequency != 923300000 {
		t.Errorf("txInfo.frequency = %d, want 923300000", up.TxInfo.Frequency)
	}
	if up.TxInfo.Modulation.LoRa.SpreadingFactor != 10 {
		t.Errorf("spreadingFactor = %d, want 10", up.TxInfo.Modulation.LoRa.SpreadingFactor)
	}
	if up.TxInfo.Modulation.LoRa.Bandwidth != 125000 {
		t.Errorf("bandwidth = %d, want 125000", up.TxInfo.Modulation.LoRa.Bandwidth)
	}

	links := buildGatewayLinks(up, 1765432100000)
	if len(links) != 2 {
		t.Fatalf("링크 수 = %d, want 2 (픽스처 rxInfo 2건)", len(links))
	}
	// gatewayId 오름차순: 24e124fffef5dccc < 24e124fffef79304.
	if links[0].gatewayID != "24e124fffef5dccc" || links[0].channel != 6 || links[0].rssi != -113 {
		t.Errorf("links[0] = %+v, want {gw:24e124fffef5dccc channel:6 rssi:-113}", links[0])
	}
	if links[1].gatewayID != "24e124fffef79304" || links[1].channel != 7 || links[1].rssi != -57 {
		t.Errorf("links[1] = %+v, want {gw:24e124fffef79304 channel:7 rssi:-57}", links[1])
	}
}

// ---------------------------------------------------------------------------
// AC-4 — cap + 결정적 eviction
// ---------------------------------------------------------------------------

// TestMergeGatewayLinks_CapEvictsOldest 는 cap 도달 시 최고령 last_seen_ms 항목이
// 축출되고 링크 수가 cap 으로 유지되는지, 그리고 반복 실행이 항상 같은 게이트웨이를
// 축출하는지 검증한다 (AC-4, REQ-M2-03).
func TestMergeGatewayLinks_CapEvictsOldest(t *testing.T) {
	// 반복 실행 결정성: Go 맵 순회는 무작위이므로 여러 번 돌려 흔든다.
	for iter := 0; iter < 50; iter++ {
		d := &deviceState{devEui: "capdev"}

		// cap(8) 만큼 채운다. gwOld 의 lastSeenMs 만 유일하게 최소.
		seed := make([]gatewayLink, 0, maxCachedGatewayLinks)
		seed = append(seed, gatewayLink{gatewayID: "gwOld", lastSeenMs: 1000})
		for i := 1; i < maxCachedGatewayLinks; i++ {
			seed = append(seed, gatewayLink{gatewayID: fmt.Sprintf("gw%02d", i), lastSeenMs: int64(2000 + i)})
		}
		d.mergeGatewayLinks(seed)
		if len(d.links) != maxCachedGatewayLinks {
			t.Fatalf("iter %d: seed 후 링크 수 = %d, want %d", iter, len(d.links), maxCachedGatewayLinks)
		}

		// 9번째 신규 게이트웨이 입장.
		d.mergeGatewayLinks([]gatewayLink{{gatewayID: "gwNew", lastSeenMs: 9000}})

		if len(d.links) != maxCachedGatewayLinks {
			t.Fatalf("iter %d: 링크 수 = %d, want %d (cap 유지)", iter, len(d.links), maxCachedGatewayLinks)
		}
		if _, evicted := d.links["gwOld"]; evicted {
			t.Fatalf("iter %d: gwOld 가 남아 있음 — 최고령 eviction 실패", iter)
		}
		if _, admitted := d.links["gwNew"]; !admitted {
			t.Fatalf("iter %d: gwNew 가 입장하지 못함", iter)
		}
	}
}

// TestMergeGatewayLinks_ExistingKeyUpdateBypassesCap 는 cap 도달 상태에서 기존
// gatewayId 갱신이 eviction 없이 항상 허용되는지 검증한다 (AC-4 후단, REQ-M2-03).
func TestMergeGatewayLinks_ExistingKeyUpdateBypassesCap(t *testing.T) {
	d := &deviceState{devEui: "capdev2"}
	seed := make([]gatewayLink, 0, maxCachedGatewayLinks)
	for i := 0; i < maxCachedGatewayLinks; i++ {
		seed = append(seed, gatewayLink{gatewayID: fmt.Sprintf("gw%02d", i), rssi: -100, lastSeenMs: int64(1000 + i)})
	}
	d.mergeGatewayLinks(seed)

	before := make(map[string]gatewayLink, len(d.links))
	for k, v := range d.links {
		before[k] = v
	}

	// 기존 키 하나만 담긴 업링크.
	d.mergeGatewayLinks([]gatewayLink{{gatewayID: "gw03", rssi: -55, lastSeenMs: 9999}})

	if len(d.links) != maxCachedGatewayLinks {
		t.Fatalf("링크 수 = %d, want %d (eviction 발생하면 안 됨)", len(d.links), maxCachedGatewayLinks)
	}
	if got := d.links["gw03"]; got.rssi != -55 || got.lastSeenMs != 9999 {
		t.Errorf("gw03 = %+v, want {rssi:-55 lastSeenMs:9999} (갱신 실패)", got)
	}
	for id := range before {
		if _, ok := d.links[id]; !ok {
			t.Errorf("%s 가 축출됨 — 기존 키 갱신은 cap 과 무관해야 한다", id)
		}
	}
}

// TestOldestLinkKey_TieBreakDeterministic 는 lastSeenMs 동률 시 gatewayId 오름차순
// 타이브레이커로 축출 대상이 결정되어, 맵 순회 순서에 비의존함을 검증한다 (AC-4).
func TestOldestLinkKey_TieBreakDeterministic(t *testing.T) {
	links := map[string]gatewayLink{
		"gwZ": {gatewayID: "gwZ", lastSeenMs: 100},
		"gwA": {gatewayID: "gwA", lastSeenMs: 100},
		"gwM": {gatewayID: "gwM", lastSeenMs: 100},
		"gwB": {gatewayID: "gwB", lastSeenMs: 500},
	}
	for i := 0; i < 100; i++ {
		got, ok := oldestLinkKey(links)
		if !ok || got != "gwA" {
			t.Fatalf("oldestLinkKey = %q (ok=%v), want gwA — 순회 순서 의존 의심", got, ok)
		}
	}
}

// ---------------------------------------------------------------------------
// AC-5 — staleness 파생
// ---------------------------------------------------------------------------

// TestLinkStale_DerivedNotStored 는 staleness 판정과 zero-threshold 폴백을 검증한다
// (AC-5, REQ-M2-05).
func TestLinkStale_DerivedNotStored(t *testing.T) {
	const nowMs = 1_765_432_100_000
	threshold := 300 * time.Second

	cases := []struct {
		name       string
		lastSeenMs int64
		threshold  time.Duration
		want       bool
	}{
		{"10초 전 → live", nowMs - 10_000, threshold, false},
		{"600초 전 → stale", nowMs - 600_000, threshold, true},
		{"임계 정확히 도달 → live (경과 > 임계 만 stale)", nowMs - 300_000, threshold, false},
		{"임계 1ms 초과 → stale", nowMs - 300_001, threshold, true},
		{"threshold=0 → defaultOfflineThreshold 폴백, 10초 전은 live", nowMs - 10_000, 0, false},
		{"threshold<0 → 폴백, 600초 전은 stale", nowMs - 600_000, -1, true},
		{"lastSeen 미상 → stale", 0, threshold, true},
	}
	for _, tc := range cases {
		if got := linkStale(nowMs, tc.lastSeenMs, tc.threshold); got != tc.want {
			t.Errorf("%s: linkStale = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestListGateways_StaleLinkNotDeleted 는 stale 링크가 조회 응답에서 stale=true 로
// 표시되되 **삭제되지 않음**을 검증한다 (AC-5, REQ-M2-05).
func TestListGateways_StaleLinkNotDeleted(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-stale", map[string]any{"offline_threshold": "300s"})

	raw := buildUplinkJSON("aabb0007", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwFresh", rssi: -70, snr: 9.0, channelJSON: "1"},
		{gatewayID: "gwStale", rssi: -95, snr: 2.5, channelJSON: "5"},
	})
	a.handleUplink(raw, "application/x")

	// lastSeenMs 를 소급 조작해 실시간 대기 없이 staleness 를 만든다.
	nowMs := time.Now().UnixMilli()
	a.devicesMu.Lock()
	d := a.devices["aabb0007"]
	fresh := d.links["gwFresh"]
	fresh.lastSeenMs = nowMs - 10_000
	d.links["gwFresh"] = fresh
	stale := d.links["gwStale"]
	stale.lastSeenMs = nowMs - 600_000
	d.links["gwStale"] = stale
	a.devicesMu.Unlock()

	gws := a.listGateways()
	if len(gws) != 2 {
		t.Fatalf("게이트웨이 수 = %d, want 2 (stale 이라도 삭제되지 않는다)", len(gws))
	}
	byID := map[string]gatewayView{}
	for _, g := range gws {
		byID[g.GatewayID] = g
	}
	if byID["gwFresh"].Devices[0].Stale {
		t.Error("gwFresh 링크가 stale=true — want false")
	}
	if !byID["gwStale"].Devices[0].Stale {
		t.Error("gwStale 링크가 stale=false — want true")
	}

	// 캐시에도 그대로 남아 있어야 한다(하드 제거는 cap-pressure 로만).
	if got := len(deviceLinks(t, a, "aabb0007")); got != 2 {
		t.Errorf("캐시 링크 수 = %d, want 2 (staleness 만으로 제거 금지)", got)
	}
}

// ---------------------------------------------------------------------------
// AC-3 / AC-3b / AC-3c — exec 계약, 결정적 정렬, 미지 커맨드
// ---------------------------------------------------------------------------

// seedTwoGatewayRoster 는 gwA(디바이스 2대) / gwB(디바이스 1대) 상태를 만든다.
func seedTwoGatewayRoster(t *testing.T, a *ChirpStackAgent) {
	t.Helper()
	// dev-2 를 먼저 넣어 삽입 순서와 정렬 순서를 어긋나게 만든다.
	a.handleUplink(buildUplinkJSON("bbbb0002", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -88, snr: 6.75, channelJSON: "2"},
	}), "application/x")
	a.handleUplink(buildUplinkJSON("aaaa0001", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwB", rssi: -60, snr: 12.0, channelJSON: "4"},
		{gatewayID: "gwA", rssi: -87, snr: 9.25, channelJSON: "3"},
	}), "application/x")
}

// TestProcess_ListGatewaysContract 는 list_gateways 응답 shape 와 device_count,
// 그리고 게이트웨이 이름/위치 필드 부재를 검증한다 (AC-3, REQ-M3-02).
func TestProcess_ListGatewaysContract(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-exec", nil)
	seedTwoGatewayRoster(t, a)

	out, err := a.Process([]byte(`{"command":"list_gateways"}`))
	if err != nil {
		t.Fatalf("Process(list_gateways): %v", err)
	}

	// 관찰된 실제 JSON 을 로그로 남긴다(UI 계약 근거).
	t.Logf("list_gateways 응답: %s", string(out))

	var resp struct {
		Gateways []struct {
			GatewayID   string           `json:"gateway_id"`
			DeviceCount int              `json:"device_count"`
			LastSeenMs  int64            `json:"last_seen_ms"`
			Devices     []map[string]any `json:"devices"`
		} `json:"gateways"`
	}
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("응답 언마샬: %v", err)
	}
	if len(resp.Gateways) != 2 {
		t.Fatalf("게이트웨이 수 = %d, want 2", len(resp.Gateways))
	}
	if resp.Gateways[0].GatewayID != "gwA" || resp.Gateways[0].DeviceCount != 2 {
		t.Errorf("gateways[0] = {%s, %d}, want {gwA, 2}", resp.Gateways[0].GatewayID, resp.Gateways[0].DeviceCount)
	}
	if resp.Gateways[1].GatewayID != "gwB" || resp.Gateways[1].DeviceCount != 1 {
		t.Errorf("gateways[1] = {%s, %d}, want {gwB, 1}", resp.Gateways[1].GatewayID, resp.Gateways[1].DeviceCount)
	}
	for _, g := range resp.Gateways {
		if g.DeviceCount != len(g.Devices) {
			t.Errorf("%s: device_count(%d) != len(devices)(%d)", g.GatewayID, g.DeviceCount, len(g.Devices))
		}
		if g.LastSeenMs <= 0 {
			t.Errorf("%s: last_seen_ms = %d, want > 0", g.GatewayID, g.LastSeenMs)
		}
	}

	// 디바이스 링크 필드 전량 존재 확인 + 게이트웨이 이름/위치 부재 확인(A6).
	wantKeys := []string{
		"dev_eui", "device_id", "device_name", "device_profile_name",
		"rssi", "snr", "channel", "frequency_hz", "spreading_factor",
		"bandwidth", "last_seen_ms", "stale",
	}
	dev := resp.Gateways[0].Devices[0]
	for _, k := range wantKeys {
		if _, ok := dev[k]; !ok {
			t.Errorf("devices[0] 에 %q 키 없음", k)
		}
	}
	if len(dev) != len(wantKeys) {
		t.Errorf("devices[0] 키 수 = %d, want %d (%v)", len(dev), len(wantKeys), dev)
	}
	for _, forbidden := range []string{"name", "gateway_name", "location", "description", "state", "last_seen_at"} {
		if bytes.Contains(out, []byte(`"`+forbidden+`"`)) {
			t.Errorf("응답에 %q 키가 존재 — 게이트웨이 이름/위치는 Non-Goal(A6)", forbidden)
		}
	}

	// gwA 의 디바이스는 dev_eui 오름차순이며 각자 자기 링크 값을 갖는다.
	gwADevices := resp.Gateways[0].Devices
	if gwADevices[0]["dev_eui"] != "aaaa0001" || gwADevices[1]["dev_eui"] != "bbbb0002" {
		t.Errorf("gwA devices dev_eui 순서 = %v/%v, want aaaa0001/bbbb0002",
			gwADevices[0]["dev_eui"], gwADevices[1]["dev_eui"])
	}
	if gwADevices[0]["rssi"] != float64(-87) || gwADevices[1]["rssi"] != float64(-88) {
		t.Errorf("gwA devices rssi = %v/%v, want -87/-88", gwADevices[0]["rssi"], gwADevices[1]["rssi"])
	}
}

// TestProcess_ListGatewaysNoSideEffects 는 list_gateways 가 MQTT publish 0건,
// 수신 채널 방출 0건, 로스터 변경 0건임을 검증한다 (AC-3, REQ-M3-04).
//
// MQTT 클라이언트는 nil 이므로 발행을 시도하면 즉시 panic/nil-deref 가 된다 —
// 구조적으로 "0 publish" 를 관찰하는 지점이다. 저장소 write 는 exec 어댑터의
// 영속화 분기(add_device/remove_device/set_device 전용)에 걸리지 않으므로
// 커맨드 이름 자체가 구조적 보장이다.
func TestProcess_ListGatewaysNoSideEffects(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-noside", nil)
	seedTwoGatewayRoster(t, a)

	// 수신 채널을 비운다(업링크 fan-out 잔여 제거).
	drained := 0
	for drained < 100 {
		select {
		case <-a.recvCh:
			drained++
		default:
			drained = 100
		}
	}

	before := a.Stats()
	if _, err := a.Process([]byte(`{"command":"list_gateways"}`)); err != nil {
		t.Fatalf("Process: %v", err)
	}
	after := a.Stats()

	if len(a.recvCh) != 0 {
		t.Errorf("수신 채널 항목 수 = %d, want 0 (조회는 방출하지 않는다)", len(a.recvCh))
	}
	if before.MessagesSent != after.MessagesSent {
		t.Errorf("MessagesSent %d → %d, want 무변경 (0 publish)", before.MessagesSent, after.MessagesSent)
	}
	if got := len(deviceLinks(t, a, "aaaa0001")); got != 2 {
		t.Errorf("조회 후 링크 수 = %d, want 2 (조회가 상태를 변경하면 안 됨)", got)
	}
}

// TestProcess_ListGatewaysByteIdentical 은 상태가 같으면 연속 호출이 바이트 동일한
// 응답을 내는지 검증한다 (AC-3b, REQ-M3-03).
//
// stale 은 조회 시점 파생값이므로 offline_threshold 를 크게 잡아 5회 호출 사이에
// 뒤집히지 않도록 한다(정렬 결정성만 검증 대상).
func TestProcess_ListGatewaysByteIdentical(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-deterministic", map[string]any{"offline_threshold": "24h"})

	// 게이트웨이/디바이스를 다수 만들어 맵 순회 무작위성을 실제로 흔든다.
	for i := 0; i < 6; i++ {
		devEui := fmt.Sprintf("dev%04d", 9-i) // 삽입 순서 ≠ 정렬 순서.
		rx := make([]rxSpec, 0, 4)
		for g := 0; g < 4; g++ {
			rx = append(rx, rxSpec{
				gatewayID:   fmt.Sprintf("gw%02d", (g+i)%5),
				rssi:        -60 - g,
				snr:         float64(g) + 0.25,
				channelJSON: fmt.Sprintf("%d", g),
			})
		}
		a.handleUplink(buildUplinkJSON(devEui, "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, rx), "application/x")
	}

	first, err := a.Process([]byte(`{"command":"list_gateways"}`))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	for i := 1; i < 5; i++ {
		next, err := a.Process([]byte(`{"command":"list_gateways"}`))
		if err != nil {
			t.Fatalf("Process #%d: %v", i, err)
		}
		if !bytes.Equal(first, next) {
			t.Fatalf("호출 #%d 응답이 바이트 불일치\nfirst=%s\nnext =%s", i, first, next)
		}
	}

	// 정렬 계약 자체도 확인한다.
	var resp listGatewaysResponse
	if err := json.Unmarshal(first, &resp); err != nil {
		t.Fatalf("언마샬: %v", err)
	}
	for i := 1; i < len(resp.Gateways); i++ {
		if resp.Gateways[i-1].GatewayID >= resp.Gateways[i].GatewayID {
			t.Errorf("gateways 가 gateway_id 오름차순이 아님: %q >= %q",
				resp.Gateways[i-1].GatewayID, resp.Gateways[i].GatewayID)
		}
	}
	for _, g := range resp.Gateways {
		for i := 1; i < len(g.Devices); i++ {
			if g.Devices[i-1].DevEui >= g.Devices[i].DevEui {
				t.Errorf("%s: devices 가 dev_eui 오름차순이 아님: %q >= %q",
					g.GatewayID, g.Devices[i-1].DevEui, g.Devices[i].DevEui)
			}
		}
	}
}

// TestProcess_EmptyRosterReturnsEmptyArray 는 업링크 수신 전 응답이 null 이 아니라
// 빈 배열인지 검증한다 (UI 방어 — AC-8b 빈 상태의 백엔드 전제).
func TestProcess_EmptyRosterReturnsEmptyArray(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-empty", nil)

	out, err := a.Process([]byte(`{"command":"list_gateways"}`))
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := string(out); got != `{"gateways":[]}` {
		t.Errorf("빈 로스터 응답 = %s, want {\"gateways\":[]}", got)
	}
}

// TestProcess_UnknownCommandRejected 는 미지/빈 커맨드가 조용한 nil 이 아니라
// ErrInvalidCommand 로 거부되고 상태가 변하지 않는지 검증한다 (AC-3c, REQ-M3-01).
func TestProcess_UnknownCommandRejected(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-unknown", nil)
	seedTwoGatewayRoster(t, a)
	beforeLinks := len(deviceLinks(t, a, "aaaa0001"))

	cases := []struct {
		name string
		body string
	}{
		{"미지 커맨드", `{"command":"nonexistent"}`},
		{"빈 커맨드", `{}`},
		{"오타(list_gateway)", `{"command":"list_gateway"}`},
	}
	for _, tc := range cases {
		out, err := a.Process([]byte(tc.body))
		if err == nil {
			t.Errorf("%s: err = nil, want ErrInvalidCommand (무음 nil 금지)", tc.name)
			continue
		}
		if !errors.Is(err, ErrInvalidCommand) {
			t.Errorf("%s: err = %v, want ErrInvalidCommand 래핑", tc.name, err)
		}
		if out != nil {
			t.Errorf("%s: out = %s, want nil", tc.name, out)
		}
	}

	// 잘못된 JSON 도 에러이다.
	if _, err := a.Process([]byte(`not-json`)); err == nil {
		t.Error("잘못된 JSON: err = nil, want 파싱 에러")
	}

	if got := len(deviceLinks(t, a, "aaaa0001")); got != beforeLinks {
		t.Errorf("거부 후 링크 수 = %d, want %d (상태 무변경)", got, beforeLinks)
	}
}

// ---------------------------------------------------------------------------
// AC-6 — 락 안전 / 무레이스 / 깊은 복사
// ---------------------------------------------------------------------------

// TestListGateways_ConcurrentUplinks 는 업링크 수신과 list_gateways 조회가 동시에
// 실행될 때 레이스가 없음을 검증한다 (AC-6, REQ-M2-06 / REQ-FROZEN-B).
//
// `go test -race` 로 실행할 때 의미가 있다. devicesMu 보유 중 a.Name() 재-lock 이
// 발생하면 이 테스트가 데드락으로 멈추므로, 락 규율 위반의 기계적 검출 지점이기도 하다.
func TestListGateways_ConcurrentUplinks(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-race", map[string]any{"emit_comm_state": true})

	const (
		writers = 4
		readers = 4
		rounds  = 60
	)
	var wg sync.WaitGroup

	for w := 0; w < writers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				devEui := fmt.Sprintf("race%02d%02d", w, i%3)
				rx := []rxSpec{
					{gatewayID: fmt.Sprintf("gw%02d", i%9), rssi: -70 - i%10, snr: 5.5, channelJSON: "3"},
					{gatewayID: fmt.Sprintf("gw%02d", (i+1)%9), rssi: -80 - i%10, snr: 2.5, channelJSON: ""},
				}
				a.handleUplink(buildUplinkJSON(devEui, "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, rx), "application/x")
				// 수신 채널이 가득 차 로그가 범람하지 않도록 드레인.
				select {
				case <-a.recvCh:
				default:
				}
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < rounds; i++ {
				out, err := a.Process([]byte(`{"command":"list_gateways"}`))
				if err != nil {
					t.Errorf("Process: %v", err)
					return
				}
				var resp listGatewaysResponse
				if err := json.Unmarshal(out, &resp); err != nil {
					t.Errorf("언마샬: %v", err)
					return
				}
				// 반환 구조를 변조해도 다음 호출이 오염되지 않아야 한다(깊은 복사).
				for gi := range resp.Gateways {
					for di := range resp.Gateways[gi].Devices {
						resp.Gateways[gi].Devices[di].RSSI = 0
					}
				}
			}
		}()
	}

	wg.Wait()

	// 변조 후에도 캐시가 온전한지 확인.
	for _, g := range a.listGateways() {
		for _, d := range g.Devices {
			if d.RSSI == 0 {
				t.Errorf("%s/%s: rssi=0 — 호출자 변조가 내부 상태를 오염시켰다(깊은 복사 실패)", g.GatewayID, d.DevEui)
			}
		}
	}
}

// TestListDevices_LinksDeepCopied 는 listDevices 스냅샷의 links 맵이 로스터 내부
// 맵과 분리되어 있는지 검증한다 (AC-6, clone() 계약 확장).
func TestListDevices_LinksDeepCopied(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-deepcopy", nil)
	a.handleUplink(buildUplinkJSON("aabb0008", "2026-08-11T23:32:01.129+00:00", 922100000, 7, 125000, []rxSpec{
		{gatewayID: "gwA", rssi: -80, snr: 9.0, channelJSON: "1"},
	}), "application/x")

	snaps := a.listDevices()
	if len(snaps) != 1 {
		t.Fatalf("스냅샷 수 = %d, want 1", len(snaps))
	}
	// 스냅샷을 변조한다.
	snaps[0].links["gwA"] = gatewayLink{gatewayID: "gwA", rssi: 12345}
	snaps[0].links["gwInjected"] = gatewayLink{gatewayID: "gwInjected"}

	live := deviceLinks(t, a, "aabb0008")
	if len(live) != 1 {
		t.Errorf("로스터 링크 수 = %d, want 1 (스냅샷 변조가 새어 들어옴)", len(live))
	}
	if live["gwA"].rssi != -80 {
		t.Errorf("로스터 gwA.rssi = %d, want -80 (스냅샷 변조가 새어 들어옴)", live["gwA"].rssi)
	}
}

// ---------------------------------------------------------------------------
// AC-7 — 프로즌 계약 무회귀 characterization
// ---------------------------------------------------------------------------

// TestBestGateway_SemanticsUnchanged 는 bestGateway 의 의미론(최대 RSSI, 동률 시
// 선행 항목 유지, 빈 rxInfo → ok=false)이 무변경임을 고정한다 (AC-7, REQ-M1-05).
func TestBestGateway_SemanticsUnchanged(t *testing.T) {
	cases := []struct {
		name    string
		rx      []uplinkRxInfo
		wantGw  string
		wantOk  bool
		wantRSS int
	}{
		{"빈 rxInfo", nil, "", false, 0},
		{"단일", []uplinkRxInfo{{GatewayID: "gwA", RSSI: -80, SNR: 1}}, "gwA", true, -80},
		{
			"최대 RSSI 선택",
			[]uplinkRxInfo{{GatewayID: "gwA", RSSI: -80}, {GatewayID: "gwB", RSSI: -95}, {GatewayID: "gwC", RSSI: -70}},
			"gwC", true, -70,
		},
		{
			"동률 시 선행 항목 유지",
			[]uplinkRxInfo{{GatewayID: "gwFirst", RSSI: -70}, {GatewayID: "gwSecond", RSSI: -70}},
			"gwFirst", true, -70,
		},
		{
			"channel 필드 추가가 선택에 영향 없음",
			[]uplinkRxInfo{{GatewayID: "gwA", RSSI: -70, Channel: 9}, {GatewayID: "gwB", RSSI: -60, Channel: 0}},
			"gwB", true, -60,
		},
	}
	for _, tc := range cases {
		rssi, _, gw, ok := bestGateway(tc.rx)
		if ok != tc.wantOk || gw != tc.wantGw || (ok && rssi != tc.wantRSS) {
			t.Errorf("%s: bestGateway = (%d, %q, %v), want (%d, %q, %v)",
				tc.name, rssi, gw, ok, tc.wantRSS, tc.wantGw, tc.wantOk)
		}
	}
}

// TestUplinkDecode_FrozenReadPathsUnchanged 는 SPEC-001 의 emit 레코드 읽기 경로가
// txInfo/channel 추가로 변하지 않았음을 고정한다 (AC-7, REQ-FROZEN-A).
func TestUplinkDecode_FrozenReadPathsUnchanged(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-frozen", nil)
	a.handleUplink(loadRawUplink(t), "application/x")

	select {
	case b := <-a.recvCh:
		var rec map[string]any
		if err := json.Unmarshal(b, &rec); err != nil {
			t.Fatalf("레코드 언마샬: %v", err)
		}
		if rec["measurement"] != "magnet_status" {
			t.Errorf("measurement = %v, want magnet_status", rec["measurement"])
		}
		if rec["value"] != "close" {
			t.Errorf("value = %v, want close", rec["value"])
		}
		// 프레임 레벨 txInfo 는 emit 레코드에 새어 들어가지 않는다(수신 계약 동결).
		for _, k := range []string{"frequency", "txInfo", "tx_info", "channel", "links"} {
			if _, leaked := rec[k]; leaked {
				t.Errorf("emit 레코드에 %q 가 추가됨 — REQ-FROZEN-A 위반", k)
			}
		}
	case <-time.After(2 * time.Second):
		t.Fatal("emit 레코드 수신 타임아웃")
	}
}

// TestAgentInterfaces_Unchanged 는 SPEC-002 접근자 시그니처가 컴파일 타임에 그대로
// 유지되는지 고정한다 (AC-7, REQ-FROZEN-C).
func TestAgentInterfaces_Unchanged(t *testing.T) {
	withMemDeviceIDRepo(t)
	a := newGatewayTestAgent(t, "gw-iface", nil)

	var _ agent.MessagePublisher = a
	var _ agent.StatefulAgent = a

	// 시그니처 고정(컴파일 타임 검증).
	var (
		_ func(string) (commEntry, bool)                       = a.CommSnapshot
		_ func() bool                                          = a.CommStateEnabled
		_ func(string) (string, string, bool)                  = a.DownlinkTarget
		_ func(rx []uplinkRxInfo) (int, float64, string, bool) = bestGateway
	)
}
