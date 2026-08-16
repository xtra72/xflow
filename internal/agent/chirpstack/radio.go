package chirpstack

// 본 파일은 event 레코드에 덧붙는 **무선 품질(radio) 메타데이터 그룹**을 만든다
// (emit_radio opt-in 경로).
//
// # 왜 새 그룹인가 (동결 계약 보존)
//
// 오늘의 event 경로(buildMeasurementRecords / buildCombinedMeasurementRecord)는
// up.Object 만 읽으므로 무선 품질이 **전혀** 실리지 않는다. up.RxInfo 는 comm 맵
// (device_state)과 로스터 링크 캐시로만 흘러 flow message 에는 도달하지 않는다.
//
// radio 는 그 빈자리에 **새로 추가되는 형제 그룹**이다. $.payload.value /
// $.metadata.measurement / $.metadata.tags.* / $.metadata.device.* / $.timestamp
// (REQ-FROZEN-A / REQ-FROZEN-02 / REQ-FROZEN-04)는 한 글자도 이동하거나 변형되지
// 않는다 — emit_radio=false(기본)에서는 레코드 JSON 이 오늘과 바이트 동일하다
// (Radio 는 포인터 + omitempty 이므로 nil 이면 키 자체가 사라진다).

// radioGatewayView 는 업링크를 수신한 게이트웨이 1개의 무선 품질 샘플이다.
//
// 필드는 rxInfo[] 항목에서 오는 **게이트웨이별** 값만 담는다. 프레임 레벨 값
// (frequency / spreadingFactor / bandwidth)은 담지 않는다 — 같은 업링크의 모든
// 게이트웨이가 동일한 값을 가지므로 게이트웨이마다 반복하면 메시지 크기만 늘고
// 정보량은 늘지 않는다(gatewayLink 주석의 2계층 구분 참조).
//
// Channel 은 **수신 게이트웨이의 concentrator IF 채널 인덱스**이며 주파수가 아니다
// (uplinkRxInfo 주석 A4). 소비자/UI 는 이 값을 주파수로 제시해서는 안 된다.
// 값 타입 uint32 이므로 proto3 가 생략한 `channel: 0` 은 자동으로 0 이 되며,
// 0 은 "미상" 이 아니라 **정당한 채널 0** 으로 그대로 보존된다.
type radioGatewayView struct {
	GatewayID string  `json:"gateway_id"`
	RSSI      int     `json:"rssi"`
	SNR       float64 `json:"snr"`
	Channel   uint32  `json:"channel"`
}

// radioGroup 은 업링크 1건의 무선 품질 그룹이다.
//
// Gateways 는 **업링크를 수신한 게이트웨이 전량**이며 최적 1개로 접지 않는다.
// 하나의 업링크를 여러 게이트웨이가 동시에 듣는 것이 LoRaWAN 의 정상 동작이므로,
// bestGateway 식의 스칼라 표현은 나머지 게이트웨이의 링크를 구조적으로 버린다
// (properties 주석의 동일한 논거).
//
// Count 는 len(Gateways) 이다. 중복 필드처럼 보이지만, 소비자가 배열을 순회하지
// 않고 커버리지(몇 대가 들었는가)만 보려는 경우가 흔하고 JSONPath / 대시보드 표현식
// 에서 배열 길이를 얻는 방법이 백엔드마다 갈리기 때문에 명시 필드로 둔다.
type radioGroup struct {
	Gateways []radioGatewayView `json:"gateways"`
	Count    int                `json:"count"`
}

// buildRadioGroup 은 업링크 1건에서 무선 품질 그룹을 만든다 (emit_radio 경로).
//
// 추출은 buildGatewayLinks 를 **재사용**한다. 직접 up.RxInfo 를 다시 순회하지 않는
// 이유는 그 함수가 이미 이 경로에 필요한 세 가지 성질을 모두 확립해 두었기 때문이다:
//
//	(1) gatewayId 키잉 — 배열 인덱스나 rxInfo[0] 을 식별자로 쓰지 않는다.
//	    ChirpStack 의 중복 제거는 Redis Set 기반이라 rxInfo[] 배열 순서에 보장이 없다.
//	(2) 동일 gatewayId 중복 항목의 전순서 축약(betterRxSample) — 배열을 어떻게 섞어도
//	    같은 결과가 나온다.
//	(3) gatewayId 오름차순 정렬 — 상태가 같으면 반복 호출이 바이트 동일한 JSON 을 낸다.
//	    테스트 flakiness 와 diff 소음을 구조적으로 제거한다.
//
// 두 번째 추출기를 만들면 위 세 성질을 각각 다시 구현해야 하고, 두 표면(로스터 링크
// 캐시 / flow message)이 같은 rxInfo 를 다르게 해석하기 시작한다.
//
// 반환값이 gatewayLink 슬라이스가 아니라 별도 뷰인 이유는 프레임 레벨 필드
// (frequencyHz/spreadingFactor/bandwidth)와 lastSeenMs 를 싣지 않기 위해서이다 —
// 링크 캐시의 관심사(현재 링크 상태 조회)와 event 의 관심사(이 측정치를 누가 들었나)
// 가 다르다.
//
// 게이트웨이가 하나도 없으면(rxInfo 부재, 또는 모든 항목에 gatewayId 가 없음) nil 을
// 반환한다. 호출자는 nil 을 그대로 레코드에 넣으므로 radio 키 자체가 생략된다 —
// 빈 배열을 흘리지 않는다. 빈 배열은 "게이트웨이가 하나도 없다" 는 적극적 사실처럼
// 읽히지만 실제 의미는 "이 업링크에는 rxInfo 가 없었다" 이다(properties 주석의
// 부재 표현 규약과 동일).
func buildRadioGroup(up *uplink, timeMs int64) *radioGroup {
	links := buildGatewayLinks(up, timeMs)
	if len(links) == 0 {
		return nil
	}
	gws := make([]radioGatewayView, 0, len(links))
	for _, l := range links {
		gws = append(gws, radioGatewayView{
			GatewayID: l.gatewayID,
			RSSI:      l.rssi,
			SNR:       l.snr,
			Channel:   l.channel,
		})
	}
	return &radioGroup{Gateways: gws, Count: len(gws)}
}
