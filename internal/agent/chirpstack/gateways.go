package chirpstack

import (
	"context"
	"sort"
	"time"

	"github.com/xtra/xflow/internal/agent"
)

// maxCachedGatewayLinks 는 디바이스당 유지하는 서로 다른 게이트웨이 링크의 상한이다
// (SPEC-CHIRPSTACK-003 REQ-M2-03).
//
// 로스터는 프로세스 수명 동안 유지되고 gatewayId 는 업링크가 실어 오는 외부 입력이므로,
// 상한이 없으면 오작동/스푸핑 게이트웨이 ID 가 캐시를 무한히 부풀린다. 상한을 디바이스당
// 국소값으로 두면 전역 메모리 상한이 `디바이스 수 × 8` 로 **구조적으로** 성립해, 별도
// 전역 게이트웨이 맵이나 reaper goroutine 없이도 한계가 보장된다.
//
// 8 인 근거: 실제 배치에서 한 디바이스를 듣는 게이트웨이는 보통 1~3개이므로 8은 충분한
// 여유다. 선례는 maxCachedMeasurements = 64 이다.
const maxCachedGatewayLinks = 8

// gatewayLink 는 (devEui, gatewayId) 쌍의 **최신 링크 샘플 1건**이다 (REQ-M2-02).
//
// 이력은 보관하지 않는다 — 목적이 "현재 링크 상태 조회"이고, 시계열은 별도 저장소
// (influxdb 경로)의 관심사이기 때문이다. 인메모리 링 버퍼는 상한·만료·직렬화 비용을
// 모두 새로 도입한다.
//
// 필드 출처가 두 계층으로 갈린다:
//   - rssi / snr / channel: rxInfo[] 항목 — **게이트웨이별** 값.
//   - frequencyHz / spreadingFactor / bandwidth: txInfo — **프레임 레벨** 값이며
//     같은 업링크에서 파생한 모든 링크가 동일한 값을 갖는다 (A4, REQ-M1-04).
//
// stale 은 저장하지 않는다 — lastSeenMs 와 offline 임계에서 조회 시점에 파생한다
// (linkStale). 저장된 불리언은 되돌릴 주체가 없으면 굳는다(deviceOnline 이 online
// 불리언을 버린 것과 동일한 이유).
type gatewayLink struct {
	gatewayID       string
	rssi            int
	snr             float64
	channel         uint32
	frequencyHz     uint64
	spreadingFactor uint32
	bandwidth       uint32
	lastSeenMs      int64
}

// buildGatewayLinks 는 업링크 1건에서 (device, gateway) 링크 샘플 전량을 만든다
// (REQ-M1-01/03/04).
//
// 키잉은 **반드시 gatewayId** 이다 — 배열 인덱스나 rxInfo[0] 을 식별자/대표로 쓰지
// 않는다. ChirpStack 의 중복 제거는 Redis Set 기반이라 rxInfo[] 배열 순서에 보장이
// 없고, 인덱스로 키잉하면 같은 게이트웨이가 업링크마다 다른 슬롯으로 튄다 (A2).
//
// 반환 슬라이스는 gatewayId 오름차순으로 정렬된다. 정렬은 cap 도달 시 어떤 신규
// 게이트웨이가 입장하는지를 rxInfo 배열 순서로부터 분리하기 위한 것이며, 호출자가
// 락을 잡기 전에 수행된다 (REQ-M2-06).
//
// 같은 업링크에 동일 gatewayId 가 중복으로 실려 오는 경우(정상 ChirpStack 에서는
// 발생하지 않는다)에도 순서 무관성을 구조적으로 보장하기 위해, 먼저 나온 항목을
// 취하지 않고 betterRxSample 의 전순서로 결정한다.
func buildGatewayLinks(up *uplink, timeMs int64) []gatewayLink {
	if up == nil || len(up.RxInfo) == 0 {
		return nil
	}

	byGateway := make(map[string]gatewayLink, len(up.RxInfo))
	for i := range up.RxInfo {
		rx := &up.RxInfo[i]
		if rx.GatewayID == "" {
			continue // 키잉 불가 — 익명 링크는 로스터에 담지 않는다.
		}
		cand := gatewayLink{
			gatewayID:       rx.GatewayID,
			rssi:            rx.RSSI,
			snr:             rx.SNR,
			channel:         rx.Channel,
			frequencyHz:     up.TxInfo.Frequency,
			spreadingFactor: up.TxInfo.Modulation.LoRa.SpreadingFactor,
			bandwidth:       up.TxInfo.Modulation.LoRa.Bandwidth,
			lastSeenMs:      timeMs,
		}
		if prev, dup := byGateway[rx.GatewayID]; dup && !betterRxSample(cand, prev) {
			continue
		}
		byGateway[rx.GatewayID] = cand
	}

	out := make([]gatewayLink, 0, len(byGateway))
	for _, l := range byGateway {
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].gatewayID < out[j].gatewayID })
	return out
}

// betterRxSample 은 동일 gatewayId 중복 항목 사이의 전순서(total order)이다.
//
// 목적은 "더 좋은 샘플" 판정이 아니라 **순서 무관성**이다: 비교가 항목 값만으로
// 결정되므로, rxInfo 배열을 어떻게 섞어도 같은 결과가 나온다. 신호 세기 우선
// (bestGateway 와 동일한 방향)에 snr/channel 을 타이브레이커로 붙여 전순서를 완성한다.
func betterRxSample(a, b gatewayLink) bool {
	if a.rssi != b.rssi {
		return a.rssi > b.rssi
	}
	if a.snr != b.snr {
		return a.snr > b.snr
	}
	return a.channel > b.channel
}

// mergeGatewayLinks 는 이번 업링크의 링크 샘플을 캐시에 병합한다 (REQ-M2-02/03).
//
// 병합 의미(덮어쓰기 아님): 이번 업링크가 실어온 gatewayId 만 갱신하고 나머지 키는
// 그대로 둔다 — 이번에 못 들은 게이트웨이의 마지막 링크 정보를 잃지 않기 위함이다
// (mergeMeasurements 와 동일한 의미론).
//
// cap 규칙:
//   - 기존 gatewayId 의 값 갱신은 cap 과 **무관하게 항상 허용**된다.
//   - 신규 gatewayId 는 cap 미만이면 그대로 입장한다.
//   - cap 도달 상태에서 신규 gatewayId 가 오면 최고령 lastSeenMs 항목을 축출하고
//     입장시킨다 (oldestLinkKey — 맵 순회 순서에 비의존한 결정적 선택).
//
// 호출자는 devicesMu 를 보유한 상태이며, samples 는 락 밖에서 이미 정렬된 값
// 슬라이스이다 — 락 보유 구간에서 하는 일은 맵 갱신뿐이다 (REQ-M2-06).
func (d *deviceState) mergeGatewayLinks(samples []gatewayLink) {
	if len(samples) == 0 {
		return
	}
	if d.links == nil {
		d.links = make(map[string]gatewayLink, len(samples))
	}
	for _, s := range samples {
		if _, exists := d.links[s.gatewayID]; !exists && len(d.links) >= maxCachedGatewayLinks {
			victim, ok := oldestLinkKey(d.links)
			if !ok {
				continue
			}
			delete(d.links, victim)
		}
		d.links[s.gatewayID] = s
	}
}

// oldestLinkKey 는 최고령 lastSeenMs 링크의 gatewayId 를 반환한다 (cap eviction 대상).
//
// 결정성이 요구사항이다: 동일 시나리오를 반복 실행하면 항상 같은 게이트웨이가 축출되어야
// 한다. Go 맵 순회는 무작위이므로 "먼저 만난 최소값" 은 결정적이지 않다 — lastSeenMs
// 동률 시 gatewayId 오름차순을 타이브레이커로 써서 순회 순서 의존을 제거한다.
func oldestLinkKey(links map[string]gatewayLink) (string, bool) {
	var (
		victim string
		oldest int64
		found  bool
	)
	for id, l := range links {
		if !found || l.lastSeenMs < oldest || (l.lastSeenMs == oldest && id < victim) {
			victim, oldest, found = id, l.lastSeenMs, true
		}
	}
	return victim, found
}

// linkStale 은 링크의 staleness 를 lastSeenMs 와 offline 임계로 **파생**한다
// (REQ-M2-05).
//
// 저장 필드가 아닌 이유는 deviceOnline 과 동일하다 — 저장된 불리언은 되돌릴 주체가
// 없으면 굳는다. staleness 는 하드 제거 트리거도 아니다: 잠깐 안 들리던 게이트웨이가
// 사라졌다 나타났다 깜빡이지 않도록, 제거는 오직 cap-pressure eviction 으로만 일어난다.
//
// 판정 방향은 watchdog 의 checkStaleness / deviceOnline 과 동일하다(경과 > 임계).
// zero-threshold 정책도 동일하다: threshold<=0 이면 defaultOfflineThreshold 로
// 폴백한다 — 0 을 그대로 쓰면 방금 수신한 링크까지 전부 즉시 stale 로 뒤집힌다.
func linkStale(nowMs, lastSeenMs int64, threshold time.Duration) bool {
	if lastSeenMs <= 0 {
		return true // 수신 시각 미상 — live 로 보고하지 않는다.
	}
	if threshold <= 0 {
		threshold = defaultOfflineThreshold
	}
	return nowMs-lastSeenMs > threshold.Milliseconds()
}

// deviceGatewayView 는 **디바이스 측 역방향 뷰**의 게이트웨이 링크 1건이다
// (SPEC-CHIRPSTACK-003 F-4 — v1 에서 Non-Goal 로 미룬 항목).
//
// list_gateways 가 (게이트웨이 → 디바이스[]) 방향이라면 이쪽은 (디바이스 →
// 게이트웨이[]) 방향이며, 같은 링크 샘플을 반대 축에서 본 것이다. 그래서 링크
// 지표 필드의 이름과 JSON 태그는 gatewayLinkView 와 **한 글자도 다르지 않게**
// 유지한다 — 프론트엔드가 두 표면에서 같은 타입을 재사용할 수 있어야 하고, 두
// 표면이 같은 값을 다른 이름으로 부르면 조용히 갈라지기 때문이다.
//
// 두 뷰의 차이는 식별 축뿐이다:
//   - gatewayLinkView: 게이트웨이 아래에 붙으므로 디바이스 식별 필드를 갖는다
//     (dev_eui/device_id/device_name/device_profile_name).
//   - deviceGatewayView: 디바이스 아래에 붙으므로 게이트웨이 식별 필드를 갖는다
//     (gateway_id).
//
// 구조체를 그대로 재사용하지 않고 별도로 둔 이유가 여기에 있다 — 한쪽에서만
// 의미가 있는 식별 필드를 공유하면 반대쪽에서 항상 빈 값이 실려 나간다.
//
// stale 은 저장 필드가 아니라 조회 시점 파생값이다(linkStale) — gatewayLinkView 와
// 동일 규약.
type deviceGatewayView struct {
	GatewayID       string  `json:"gateway_id"`
	RSSI            int     `json:"rssi"`
	SNR             float64 `json:"snr"`
	Channel         uint32  `json:"channel"`
	FrequencyHz     uint64  `json:"frequency_hz"`
	SpreadingFactor uint32  `json:"spreading_factor"`
	Bandwidth       uint32  `json:"bandwidth"`
	LastSeenMs      int64   `json:"last_seen_ms"`
	Stale           bool    `json:"stale"`
}

// deviceGatewayViews 는 디바이스 1대의 링크 캐시를 게이트웨이 뷰 슬라이스로 만든다.
//
// 정렬 키는 **gateway_id 오름차순**이다 (list_gateways 의 REQ-M3-03 과 동일 선택).
// 이유는 결정성이다: gatewayId 는 맵의 키라 컬렉션 안에서 유일하므로 전순서가
// 성립하고, 상태가 바뀌지 않으면 반복 호출이 바이트 동일한 출력을 낸다.
// rssi 내림차순(+ gatewayId 타이브레이크)도 전순서이긴 하지만, 신호 세기는 업링크
// 마다 흔들리므로 상태가 "사실상 그대로"인데도 행 순서가 계속 뒤바뀐다 — 목록
// 표시와 diff 양쪽에 나쁘다. 세기순 정렬이 필요하면 표시 계층에서 하면 된다.
//
// 입력 맵은 호출자가 이미 락 밖에서 확보한 스냅샷 사본이며(deviceState.clone),
// 반환 슬라이스는 매 호출마다 새로 만든 값 복사본이다 — 반환 구조를 호출자가
// 어떻게 변조하든 로스터나 다음 호출 결과가 오염되지 않는다.
//
// links 가 비어 있으면 nil 을 반환한다. 호출자는 이 경우 키 자체를 생략한다
// (properties 주석의 부재 표현 규약 참조).
func deviceGatewayViews(links map[string]gatewayLink, nowMs int64, threshold time.Duration) []deviceGatewayView {
	if len(links) == 0 {
		return nil
	}
	out := make([]deviceGatewayView, 0, len(links))
	for _, l := range links {
		out = append(out, deviceGatewayView{
			GatewayID:       l.gatewayID,
			RSSI:            l.rssi,
			SNR:             l.snr,
			Channel:         l.channel,
			FrequencyHz:     l.frequencyHz,
			SpreadingFactor: l.spreadingFactor,
			Bandwidth:       l.bandwidth,
			LastSeenMs:      l.lastSeenMs,
			Stale:           linkStale(nowMs, l.lastSeenMs, threshold),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GatewayID < out[j].GatewayID })
	return out
}

// gatewayLinkView 는 list_gateways 응답의 디바이스 링크 1건이다 (REQ-M3-02).
//
// 필드 순서가 곧 JSON 키 순서이므로, 정렬만 결정적이면 응답 전체가 바이트 동일해진다
// (REQ-M3-03).
type gatewayLinkView struct {
	DevEui            string  `json:"dev_eui"`
	DeviceID          string  `json:"device_id"`
	DeviceName        string  `json:"device_name"`
	DeviceProfileName string  `json:"device_profile_name"`
	RSSI              int     `json:"rssi"`
	SNR               float64 `json:"snr"`
	Channel           uint32  `json:"channel"`
	FrequencyHz       uint64  `json:"frequency_hz"`
	SpreadingFactor   uint32  `json:"spreading_factor"`
	Bandwidth         uint32  `json:"bandwidth"`
	LastSeenMs        int64   `json:"last_seen_ms"`
	Stale             bool    `json:"stale"`
}

// gatewayView 는 list_gateways 응답의 게이트웨이 1건이다 (REQ-M3-02).
//
// 이름/설명/위치/state 필드는 **없다**: 업링크가 제공하는 것은 gatewayId 뿐이고,
// 이름/위치는 gRPC GatewayService.List 가 있어야 얻을 수 있다 (A6, Non-Goal).
type gatewayView struct {
	GatewayID   string            `json:"gateway_id"`
	DeviceCount int               `json:"device_count"`
	LastSeenMs  int64             `json:"last_seen_ms"`
	Devices     []gatewayLinkView `json:"devices"`
}

// listGatewaysResponse 는 list_gateways 의 응답 봉투이다.
type listGatewaysResponse struct {
	Gateways []gatewayView `json:"gateways"`
}

// listGateways 는 디바이스별 링크 캐시를 **역인덱싱**해 게이트웨이 로스터를 파생한다
// (REQ-M2-04, REQ-M3-02/03).
//
// 게이트웨이는 저장된 엔티티가 아니라 파생 뷰이다. 두 번째 권위 맵을 두지 않으므로
// 두 표면이 조용히 갈라질 수 없고, 신규 mutex 도 필요 없다.
//
// 락 규율(REQ-FROZEN-B / REQ-M2-06) — deviceAdapters() 와 동형:
//
//	(1) 락 보유 전에 agentName(a.mu) 과 offline 임계(atomic)를 선캡처한다.
//	    → 락 보유 중 a.Name() 재-lock 을 유발하지 않는다 (v0.18.6 HVAC 재귀 RLock 트랩).
//	(2) devicesMu 를 잡아 로스터를 깊은 복사하고 즉시 해제한다 (listDevices).
//	(3) 역인덱싱/정렬/저장소 조회는 어떤 락도 보유하지 않은 채 수행한다.
//
// commMu 는 전혀 관여하지 않으므로 devicesMu ↔ commMu 락 순서 엣지가 생기지 않는다.
// 반환 구조는 스냅샷의 값 복사본만 담으므로 내부 맵 포인터가 노출되지 않는다.
//
// 구조적 한계(A7, 문서화된 것): 로스터가 업링크 파생이므로 (a) 담당 디바이스가 모두
// 침묵 중인 게이트웨이는 보이지 않고, (b) 커버 디바이스가 없는 게이트웨이는 애초에
// 발견되지 않는다. 근본 해소는 gRPC GatewayService.List 통합이며 본 SPEC 범위 밖이다.
func (a *ChirpStackAgent) listGateways() []gatewayView {
	agentName := a.Name()                // (1) 락 보유 전 캡처.
	threshold := a.cs().OfflineThreshold // atomic 스냅샷 — 락 없음.
	nowMs := time.Now().UnixMilli()

	snaps := a.listDevices() // (2) devicesMu 획득 → 깊은 복사 → 해제.

	// (3) 락 밖 역인덱싱: (devEui → gatewayId) 를 (gatewayId → devEui[]) 로 뒤집는다.
	byGateway := make(map[string][]gatewayLinkView)
	lastSeenByGateway := make(map[string]int64)
	for i := range snaps {
		s := &snaps[i]
		if len(s.links) == 0 {
			continue
		}
		// 저장소 조회(ResolveDeviceID)도 락 밖에서 수행한다. upsertDevice 가 모든
		// 업링크에서 이미 발급했으므로 여기서는 조회로 귀결되며, 저장소 미설정 시
		// 빈 문자열이다(chirpDeviceAdapter.UID 와 동일 규약).
		deviceID := agent.ResolveDeviceID(context.Background(), agentName, s.devEui)
		for gatewayID, l := range s.links {
			byGateway[gatewayID] = append(byGateway[gatewayID], gatewayLinkView{
				DevEui:            s.devEui,
				DeviceID:          deviceID,
				DeviceName:        s.deviceName,
				DeviceProfileName: s.deviceProfileName,
				RSSI:              l.rssi,
				SNR:               l.snr,
				Channel:           l.channel,
				FrequencyHz:       l.frequencyHz,
				SpreadingFactor:   l.spreadingFactor,
				Bandwidth:         l.bandwidth,
				LastSeenMs:        l.lastSeenMs,
				Stale:             linkStale(nowMs, l.lastSeenMs, threshold),
			})
			if l.lastSeenMs > lastSeenByGateway[gatewayID] {
				lastSeenByGateway[gatewayID] = l.lastSeenMs
			}
		}
	}

	// 결정적 정렬: 게이트웨이는 gateway_id, 디바이스는 dev_eui 오름차순 (REQ-M3-03).
	// 두 정렬 키 모두 각 컬렉션 내에서 유일하므로 전순서가 성립하고, 상태가 같으면
	// 반복 호출이 바이트 동일한 JSON 을 낸다.
	gatewayIDs := make([]string, 0, len(byGateway))
	for id := range byGateway {
		gatewayIDs = append(gatewayIDs, id)
	}
	sort.Strings(gatewayIDs)

	out := make([]gatewayView, 0, len(gatewayIDs))
	for _, id := range gatewayIDs {
		devices := byGateway[id]
		sort.Slice(devices, func(i, j int) bool { return devices[i].DevEui < devices[j].DevEui })
		out = append(out, gatewayView{
			GatewayID:   id,
			DeviceCount: len(devices),
			LastSeenMs:  lastSeenByGateway[id],
			Devices:     devices,
		})
	}
	return out
}
