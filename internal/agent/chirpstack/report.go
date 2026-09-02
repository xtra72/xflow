package chirpstack

import (
	"context"
	"encoding/json"
	"sort"
	"time"
)

// 본 파일은 **주기 집계 리포트**(measurement_report)를 구현한다 (emit_report opt-in).
//
// # device_state.report 와의 이름 충돌 주의
//
// 이미 device_state.report 가 존재하지만 **완전히 다른 것**이다:
//
//   - device_state.report (watchdog.go / comm_report_interval): commEntry 의
//     **마지막 스냅샷 1건**을 주기적으로 재방출한다. commEntry 는 최신 값만 들고
//     있으므로 집계가 아니다.
//   - measurement_report (본 파일 / report_interval): 윈도 구간의 모든 업링크를
//     누적한 **min/max/avg/count 집계**이다.
//
// 그래서 판별자를 measurement_report 로, 노드 메시지 타입을 measurement.report 로
// 분리한다. 기존 device_state 경로는 한 줄도 건드리지 않는다.
//
// # 왜 새 집계기인가 (재사용 가능한 것이 없다)
//
//   - AggregateNode(internal/node/aggregate.go)는 payload.Get(field) 만 읽으므로
//     metadata.measurement 로 그룹화할 수 없다.
//   - internal/tsdb/query.go 와 store_query.go 의 bucketAggregate 는 **이미 저장된
//     포인트에 대한 배치 집계**이며 스트리밍 누적기가 아니다.
//
// 따라서 에이전트 안에 스트리밍 누적기를 둔다. 샘플을 버퍼링하지 않고 러닝
// {min,max,sum,count} 만 유지하는 것이 핵심이다 — 샘플 버퍼는 업링크 레이트가
// 높아질수록 무한히 자란다(윈도 하나가 곧 메모리 상한이 되어 버린다).

// recordKindMeasurementReport 는 주기 집계 리포트 레코드의 판별자 값이다.
//
// device_state / measurements 와 나란히 놓이는 세 번째 판별자이며, 노드는 이 값을
// 보고 measurement.report 메시지로 빌드한다. 기존 세 분기(device_state /
// measurements / 빈 문자열=event)의 동작은 변하지 않는다.
const recordKindMeasurementReport = "measurement_report"

// report 방출 모드 (report_emit_mode).
//
// measurement_emit_mode 와 **독립된 축**이다. event 는 per_measurement 로 잘게
// 흘리면서 report 는 combined 로 한 번에 받고 싶은(또는 그 반대) 요구가 실재하므로
// 두 노브를 하나로 묶지 않는다.
//
//   - per_measurement(기본): measurement 당 리포트 레코드 1개.
//   - combined: 디바이스당 리포트 레코드 1개(모든 measurement 를 함께 담는다).
const (
	reportEmitModePerMeasurement = "per_measurement"
	reportEmitModeCombined       = "combined"
)

// 집계 누적기의 상한(cap).
//
// 누적 맵은 report_interval 마다 통째로 비워지지만, **한 윈도 안에서도** 오작동/
// 스푸핑 입력이 맵을 부풀릴 수 있으므로 상한이 필요하다. 특히 emit_report=true 인데
// report_interval 이 0 이거나 goroutine 이 아직 기동되지 않은 상태(Configure 런타임
// 토글, 아래 measurementReportLoop 주석 참조)에서는 드레인 주체가 없으므로 상한이
// 유일한 방어선이다.
//
// 값의 근거는 기존 상한을 그대로 미러링한 것이다:
//   - maxReportMeasurements = 64 → maxCachedMeasurements(provider.go)와 동일.
//   - maxReportGateways = 8 → maxCachedGatewayLinks(gateways.go)와 동일.
//   - maxReportDevices = 1024 → 위 둘과 곱해져 전역 상한이 구조적으로 성립하게 하는
//     디바이스 축 상한이다(1024 × (64 + 8) 엔트리).
//
// # 축출(eviction) 동작: 상한 도달 시 **신규 키를 무시**하고 기존 키의 누적은 계속한다
//
// 세 축 모두 동일하다. mergeMeasurements(provider.go)의 규칙과 같으며,
// mergeGatewayLinks 의 "최고령 축출" 과는 **의도적으로 다르다**. 링크 캐시는 "현재
// 상태" 라 낡은 항목을 밀어내는 것이 맞지만, 집계 윈도에서 부분 누적된 게이트웨이를
// 축출하면 그 자리에 들어온 신규 게이트웨이가 윈도 일부만 관측한 min/max/avg 를
// 마치 윈도 전체 집계인 양 내보낸다 — 조용히 틀린 숫자가 된다. 신규를 무시하면
// **보고된 항목은 항상 윈도 전 구간을 관측한 완전한 집계**임이 보장된다.
const (
	maxReportMeasurements = 64
	maxReportGateways     = 8
	maxReportDevices      = 1024
)

// numAgg 는 러닝 집계 상태이다 (샘플 버퍼 없음).
//
// count 와 numericCount 를 분리하는 이유는 비숫자 스칼라(문자열/불리언) 때문이다.
// 예컨대 magnet_status="close" 는 스칼라이지만 min/max/avg 를 계산할 수 없다.
// 이런 값을 (a) 통째로 버리면 "그 구간에 업링크가 없었다" 와 구분되지 않고,
// (b) 0 으로 강제 변환하면 존재하지 않는 숫자를 발명한다. 그래서 **개수는 세되
// 숫자 통계는 생략**한다 — count 는 나오고 min/max/avg 는 키 자체가 사라진다.
//
// 한 measurement 가 윈도 안에서 숫자와 비숫자를 섞어 보내는 병리적 경우
// (numericCount < count)에는 min/max/avg 가 **숫자 부분집합에 대해서만** 계산되며
// count 는 전체 샘플 수를 유지한다. avg = sum/numericCount 이므로 avg 를 count 로
// 되곱해도 sum 이 나오지 않는다는 뜻이다 — 의도된 선택이며, 반대로 avg 를
// sum/count 로 계산하면 존재하지 않는 0 샘플을 섞은 값이 되어 더 나쁘다.
type numAgg struct {
	count        int64
	numericCount int64
	min          float64
	max          float64
	sum          float64
}

// add 는 스칼라 샘플 1건을 누적한다. 비숫자 스칼라는 count 만 올린다.
func (a *numAgg) add(v any) {
	a.count++
	f, ok := numericValue(v)
	if !ok {
		return
	}
	if a.numericCount == 0 {
		a.min, a.max = f, f
	} else {
		if f < a.min {
			a.min = f
		}
		if f > a.max {
			a.max = f
		}
	}
	a.sum += f
	a.numericCount++
}

// numericValue 는 스칼라 값이 집계 가능한 숫자인지 판정하고 float64 로 변환한다.
//
// JSON 숫자는 언제나 float64 로 도착하지만, 테스트/프로그램 경로에서 Go 정수 타입이
// 직접 실려 올 수 있으므로 정수 계열도 받는다.
//
// **문자열은 숫자로 강제 변환하지 않는다** — coerceInt/coerceDuration 이 문자열을
// 받아 주는 것은 사람이 타이핑하는 **설정 값**이기 때문이고, measurement 값은 장비가
// 보낸 **데이터**이다. "12" 를 숫자로 접으면 문자열 센서가 조용히 숫자 통계에 섞여
// 들어가고, 그 측정치가 왜 갑자기 평균을 갖는지 추적할 단서가 사라진다.
//
// 불리언도 숫자가 아니다(true=1 로 접지 않는다). 같은 이유이며, 필요하면 소비자가
// count 로 등장 횟수를 볼 수 있다.
func numericValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int8:
		return float64(n), true
	case int16:
		return float64(n), true
	case int32:
		return float64(n), true
	case int64:
		return float64(n), true
	case uint:
		return float64(n), true
	case uint8:
		return float64(n), true
	case uint16:
		return float64(n), true
	case uint32:
		return float64(n), true
	case uint64:
		return float64(n), true
	default:
		return 0, false
	}
}

// aggView 는 집계 결과의 직렬화 형태이다.
//
// Min/Max/Avg 가 포인터인 이유는 "숫자 통계 없음"(비숫자 스칼라만 관측)과 "값이
// 0"을 구분하기 위해서이다. 값 타입 + omitempty 였다면 min=0 인 정당한 집계가 통째로
// 사라진다(uplinkRxInfo.Channel 이 값 타입이어야 하는 것과 정확히 반대 방향의 이유).
// nil 포인터만 omitempty 로 생략되므로 0.0 은 그대로 실린다.
//
// Count 는 항상 방출된다 — 집계가 존재한다는 것 자체가 count>=1 을 뜻하며, 0 은
// 애초에 레코드가 만들어지지 않는다(emitMeasurementReports 주석 참조).
type aggView struct {
	Min   *float64 `json:"min,omitempty"`
	Max   *float64 `json:"max,omitempty"`
	Avg   *float64 `json:"avg,omitempty"`
	Count int64    `json:"count"`
}

// view 는 누적 상태를 직렬화 형태로 변환한다. count==0 이면 ok=false.
func (a numAgg) view() (aggView, bool) {
	if a.count == 0 {
		return aggView{}, false
	}
	v := aggView{Count: a.count}
	if a.numericCount > 0 {
		// 로컬 복사본의 주소를 취한다 — 수신자가 값 타입이므로 a 의 필드를 직접
		// 가리켜도 안전하지만, 복사본이 의도를 더 분명히 한다.
		min, max := a.min, a.max
		avg := a.sum / float64(a.numericCount)
		v.Min, v.Max, v.Avg = &min, &max, &avg
	}
	return v, true
}

// gatewayAgg 는 게이트웨이 1대의 rssi/snr 러닝 집계이다.
type gatewayAgg struct {
	rssi numAgg
	snr  numAgg
}

// radioAggView 는 리포트의 게이트웨이별 무선 품질 집계 1건이다.
//
// 게이트웨이마다 따로 집계하는 것은 Feature 1(radio 그룹)의 "최적 1개로 접지 않고
// 전량을 싣는다" 선택과 같은 결정이다. 여러 게이트웨이의 rssi 를 하나로 뭉치면
// 서로 다른 물리 링크의 값이 섞여 평균이 아무 의미도 갖지 않는다.
type radioAggView struct {
	GatewayID string  `json:"gateway_id"`
	RSSI      aggView `json:"rssi"`
	SNR       aggView `json:"snr"`
}

// deviceWindow 는 디바이스 1대의 **현재 윈도** 누적 상태이다 (reportsMu 보호).
//
// tags 는 마지막 업링크의 태그를 그대로 들고 있는다(verbatim, 동결 계약과 동일 규약).
// 윈도 중간에 태그가 바뀌면 마지막 값이 이긴다 — 태그는 시계열 데이터가 아니라
// 디바이스 속성이므로 집계 대상이 아니다.
type deviceWindow struct {
	tags         map[string]string
	measurements map[string]*numAgg
	gateways     map[string]*gatewayAgg
	uplinks      int64
}

// measurementReportRecord 는 에이전트가 노드로 전달하는 주기 집계 리포트 레코드이다.
//
// 두 방출 모드가 하나의 구조체를 공유하며 상호 배타적인 필드로 구분된다:
//
//   - per_measurement: Measurement + Stats 를 채우고 Measurements 는 비운다.
//   - combined: Measurements 를 채우고 Measurement/Stats 는 비운다.
//
// 판별자(Record)는 두 모드가 동일하다. 별도 판별자를 만들지 않은 이유는 노드가
// "measurements 키가 있는가" 로 이미 명확히 갈라낼 수 있고, 판별자를 늘리면 노드의
// switch 가 같은 개념을 두 값으로 나눠 갖게 되기 때문이다.
//
// TimeMs 는 **윈도 종료 시각**이다 (WindowEndMs 와 동일 값). 상세한 근거는
// emitMeasurementReports 주석 참조.
type measurementReportRecord struct {
	Record string `json:"record"`

	// per_measurement 모드 전용.
	Measurement string   `json:"measurement,omitempty"`
	Stats       *aggView `json:"stats,omitempty"`

	// combined 모드 전용. 맵 키는 measurement 이름이며, encoding/json 이 맵 키를
	// 정렬해 직렬화하므로 출력이 결정적이다.
	Measurements map[string]aggView `json:"measurements,omitempty"`

	// 게이트웨이별 rssi/snr 집계. gateway_id 오름차순 정렬(결정성).
	Radio []radioAggView `json:"radio,omitempty"`

	Uplinks  int64 `json:"uplinks"`  // 윈도 구간에 수신한 업링크 건수
	Gateways int   `json:"gateways"` // 윈도 구간에 관측된 서로 다른 게이트웨이 수

	UnitID        string            `json:"unit_id"`
	TimeMs        int64             `json:"time_ms"`
	WindowStartMs int64             `json:"window_start_ms"`
	WindowEndMs   int64             `json:"window_end_ms"`
	Tags          map[string]string `json:"tags,omitempty"`
}

// onUplinkReport 는 업링크 1건을 현재 윈도에 누적한다 (emit_report 게이트 뒤).
//
// # 락 규율 (REQ-FROZEN-B)
//
// 신규 mutex(reportsMu)도 기존 규율의 적용 대상이다:
//
//	(1) 락 보유 중 a.Name() 등 a.mu 를 다시 잡는 메서드를 호출하지 않는다
//	    (v0.18.6 HVAC 재귀 RLock 트랩).
//	(2) devicesMu / commMu 와 **절대 중첩하지 않는다**. 이 함수는 두 락 중 어느
//	    것도 잡지 않으며, 반대로 두 락을 잡는 경로(upsertDevice / onUplinkCommState)
//	    도 reportsMu 를 잡지 않는다 — 락 순서 엣지 자체가 존재하지 않는다.
//	(3) 스칼라 키 선별/정렬과 게이트웨이 링크 산출은 **락 밖**에서 끝낸다. 락 보유
//	    구간에서 하는 일은 맵 갱신뿐이다(upsertDevice 와 동형).
//
// # buildGatewayLinks 중복 호출에 관하여
//
// upsertDevice 도 같은 업링크에 대해 buildGatewayLinks 를 호출한다. 두 호출을 하나로
// 합치려면 upsertDevice 의 시그니처를 넓혀야 하는데, 그 함수는 모든 업링크가 무조건
// 지나는 경로이므로 opt-in 기능을 위해 건드리지 않는다. 중복 비용은 emit_report 를
// 켠 배포에서만 발생하며, rxInfo 항목 수(보통 1~3)에 비례하는 작은 값이다.
func (a *ChirpStackAgent) onUplinkReport(up *uplink, timeMs int64) {
	devEui := up.DeviceInfo.DevEui
	if devEui == "" {
		return
	}

	// (3) 락 밖 준비: 스칼라 키 선별(emit 경로와 동일한 isScalar 술어를 공유하는
	// scalarMeasurementKeys 재사용)과 게이트웨이 링크 산출.
	keys := scalarMeasurementKeys(up.Object)
	links := buildGatewayLinks(up, timeMs)
	tags := up.DeviceInfo.Tags

	var dropped bool // 디바이스 상한 도달 — 로그는 락 해제 후에 낸다.

	a.reportsMu.Lock()
	// 윈도 시작 시각 지연 초기화. startMeasurementReporter 가 기동 시점에 확정하지만,
	// 그보다 먼저 업링크가 도착하거나(기동 직전 경합) tick 루프가 아예 없는 구성에서는
	// 0 으로 남는다 — 0 은 1970 으로 읽혀 window_start_ms 의 의미가 무너진다.
	// 여기서 서버 시각으로 채우면 window_end_ms(마감 tick 의 서버 시각)와 같은 시계를
	// 쓰므로 두 값이 어긋나지 않는다.
	if a.reportWindowStartMs == 0 {
		a.reportWindowStartMs = time.Now().UnixMilli()
	}
	w, exists := a.reports[devEui]
	if !exists {
		if len(a.reports) >= maxReportDevices {
			dropped = true
		} else {
			w = &deviceWindow{
				measurements: make(map[string]*numAgg, len(keys)),
				gateways:     make(map[string]*gatewayAgg, len(links)),
			}
			a.reports[devEui] = w
		}
	}
	if w != nil {
		w.tags = tags
		w.uplinks++
		for _, k := range keys {
			agg, ok := w.measurements[k]
			if !ok {
				if len(w.measurements) >= maxReportMeasurements {
					continue // 상한 도달 — 신규 키 무시, 기존 키 누적은 계속.
				}
				agg = &numAgg{}
				w.measurements[k] = agg
			}
			agg.add(up.Object[k])
		}
		for _, l := range links {
			g, ok := w.gateways[l.gatewayID]
			if !ok {
				if len(w.gateways) >= maxReportGateways {
					continue // 상한 도달 — 신규 게이트웨이 무시(부분 집계 방지).
				}
				g = &gatewayAgg{}
				w.gateways[l.gatewayID] = g
			}
			g.rssi.add(l.rssi)
			g.snr.add(l.snr)
		}
	}
	a.reportsMu.Unlock()

	if dropped && a.logger != nil {
		a.logger.Warn("chirpstack: 리포트 디바이스 상한 도달 — 이번 윈도에서 신규 디바이스 집계 생략",
			"devEui", devEui, "cap", maxReportDevices)
	}
}

// emitMeasurementReports 는 현재 윈도를 마감하고 디바이스별 리포트를 방출한다.
//
// # 윈도 리셋
//
// 누적 맵을 **통째로 새 맵으로 교체**한다(스냅샷 복사가 아니다). 그 결과:
//
//   - 다음 윈도는 완전히 0 에서 시작한다 — 두 번째 윈도가 첫 윈도의 값을 누적해
//     들고 가지 않는다.
//   - 업링크가 없었던 디바이스는 새 맵에 아예 나타나지 않으므로 **아무것도 방출되지
//     않는다**. 0 건 리포트를 흘리지 않는 것은 buildCombinedMeasurementRecord 가
//     스칼라 0개일 때 ok=false 를 반환하는 것과 같은 규약이며, count==0 으로 avg 를
//     계산하는 경로(0 나눗셈 / NaN)가 구조적으로 존재할 수 없게 만든다.
//   - 교체된 맵은 이 goroutine 이 단독 소유하므로 락 밖에서 자유롭게 순회한다.
//
// # 타임스탬프가 timestamp_source 의 지배를 받지 않는 이유
//
// timestamp_source 는 **업링크 1건**의 시각을 두 시계 중 어디에서 읽을지 고르는
// 노브이다(장비/게이트웨이가 찍은 uplink.time vs 서버 수신 시각). 리포트는 특정
// 업링크가 아니라 **구간**에 대응하므로 읽을 uplink.time 자체가 없다 — uplink 모드를
// 적용하면 "어느 업링크의 시각을 고를 것인가" 라는 답 없는 질문이 되고, server 모드를
// 적용하면 지금 하는 일과 정확히 같아진다. 즉 이 노브를 끌어들이면 한쪽은 무의미하고
// 다른 쪽은 항등(no-op)이다. 그래서 리포트 시각은 **항상 윈도 종료 시각**(마감 tick
// 의 서버 시각)으로 고정하고, 구간 자체는 window_start_ms/window_end_ms 로 명시한다.
func (a *ChirpStackAgent) emitMeasurementReports() {
	nowMs := time.Now().UnixMilli()
	mode := a.cs().ReportEmitMode // atomic 스냅샷 — 락 없음.

	a.reportsMu.Lock()
	windows := a.reports
	startMs := a.reportWindowStartMs
	a.reports = make(map[string]*deviceWindow)
	a.reportWindowStartMs = nowMs
	a.reportsMu.Unlock()

	if len(windows) == 0 {
		return
	}

	// 락 밖 조립. devEui 오름차순으로 순회해 방출 순서를 결정적으로 만든다
	// (맵 순회는 무작위이므로 그대로 두면 같은 시나리오가 매번 다른 순서를 낸다).
	devEuis := make([]string, 0, len(windows))
	for devEui := range windows {
		devEuis = append(devEuis, devEui)
	}
	sort.Strings(devEuis)

	for _, devEui := range devEuis {
		for _, rec := range buildMeasurementReportRecords(devEui, windows[devEui], mode, startMs, nowMs) {
			a.enqueueMeasurementReport(rec)
		}
	}
}

// buildMeasurementReportRecords 는 디바이스 1대의 윈도를 리포트 레코드로 만든다.
//
// 스칼라 measurement 를 하나도 관측하지 못한 윈도는 빈 슬라이스를 반환한다 —
// 업링크는 받았지만 전부 비스칼라였던 경우이며, event 경로가 같은 상황에서 아무것도
// 방출하지 않는 것과 동일한 규약이다(빈 payload 메시지 금지).
//
// startMs 는 윈도 시작 시각(직전 마감 tick), endMs 는 이번 마감 tick 이다. 첫 샘플의
// 시각이 아니라 tick 경계를 쓰는 이유는 그것이 실제 집계 구간이기 때문이다 — 디바이스가
// 윈도 후반에만 말했더라도 "이 구간에 이만큼 관측했다" 가 맞는 서술이다.
func buildMeasurementReportRecords(devEui string, w *deviceWindow, mode string, startMs, endMs int64) []measurementReportRecord {
	if w == nil || len(w.measurements) == 0 {
		return nil
	}

	radio := buildRadioAggViews(w.gateways)

	base := measurementReportRecord{
		Record:        recordKindMeasurementReport,
		Radio:         radio,
		Uplinks:       w.uplinks,
		Gateways:      len(w.gateways),
		UnitID:        devEui,
		TimeMs:        endMs,
		WindowStartMs: startMs,
		WindowEndMs:   endMs,
		Tags:          w.tags,
	}

	if mode == reportEmitModeCombined {
		values := make(map[string]aggView, len(w.measurements))
		for k, agg := range w.measurements {
			if v, ok := agg.view(); ok {
				values[k] = v
			}
		}
		if len(values) == 0 {
			return nil
		}
		rec := base
		rec.Measurements = values
		return []measurementReportRecord{rec}
	}

	// per_measurement: measurement 이름 오름차순으로 fan-out 한다(결정적 순서).
	keys := make([]string, 0, len(w.measurements))
	for k := range w.measurements {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]measurementReportRecord, 0, len(keys))
	for _, k := range keys {
		v, ok := w.measurements[k].view()
		if !ok {
			continue
		}
		rec := base
		rec.Measurement = k
		stats := v
		rec.Stats = &stats
		out = append(out, rec)
	}
	return out
}

// buildRadioAggViews 는 게이트웨이별 집계를 gateway_id 오름차순 슬라이스로 만든다.
//
// 정렬 키가 gateway_id 인 이유는 buildGatewayLinks / deviceGatewayViews 와 동일하다:
// 맵 키라 컬렉션 안에서 유일하므로 전순서가 성립하고, 상태가 같으면 반복 호출이 바이트
// 동일한 출력을 낸다. rssi 세기순은 업링크마다 흔들려 행 순서가 계속 뒤바뀐다.
func buildRadioAggViews(gateways map[string]*gatewayAgg) []radioAggView {
	if len(gateways) == 0 {
		return nil
	}
	out := make([]radioAggView, 0, len(gateways))
	for id, g := range gateways {
		rssi, rok := g.rssi.view()
		snr, sok := g.snr.view()
		if !rok || !sok {
			continue
		}
		out = append(out, radioAggView{GatewayID: id, RSSI: rssi, SNR: snr})
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GatewayID < out[j].GatewayID })
	return out
}

// enqueueMeasurementReport 는 리포트 레코드를 직렬화해 수신 채널에 넣는다.
// 직렬화 실패는 device_state 경로(enqueueRecord)와 동일하게 에러 통계로 계상한다.
func (a *ChirpStackAgent) enqueueMeasurementReport(rec measurementReportRecord) {
	b, err := json.Marshal(rec)
	if err != nil {
		a.stats.IncrExternalMessagesErrored()
		a.logger.Warn("chirpstack: measurement_report 레코드 직렬화 실패",
			"unit_id", rec.UnitID, "measurement", rec.Measurement, "error", err)
		return
	}
	a.enqueue(b, "measurement_report")
}

// startMeasurementReporter 는 emit_report + report_interval>0 일 때 집계 리포트
// goroutine 을 기동한다. idempotent 하며 Init(Running 진입) 경로에서 호출된다.
//
// startCommWatchdog 과 동형이다 — 별도 cancel/waitgroup 을 쓰는 이유는 두 기능의
// 수명이 서로 독립이기 때문이다(emit_comm_state 만 켠 배포에서 리포트 goroutine 이
// 생기거나, 그 반대가 되어서는 안 된다).
func (a *ChirpStackAgent) startMeasurementReporter() {
	cc := a.cs()
	if !cc.EmitReport || cc.ReportInterval <= 0 {
		return
	}

	a.mu.Lock()
	if a.mrStarted {
		a.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.mrCancel = cancel
	a.mrStarted = true
	a.mu.Unlock()

	// 첫 윈도의 시작 시각을 확정한다. a.mu 를 해제한 뒤에 잡는다 — 락 중첩 금지
	// (REQ-FROZEN-B).
	a.reportsMu.Lock()
	a.reportWindowStartMs = time.Now().UnixMilli()
	a.reportsMu.Unlock()

	a.mrWg.Add(1)
	go a.measurementReportLoop(ctx)
}

// stopMeasurementReporter 는 리포트 goroutine 을 취소하고 종료를 대기한다.
// idempotent — 미기동 상태면 no-op (stopCommWatchdog 과 동형).
func (a *ChirpStackAgent) stopMeasurementReporter() {
	a.mu.Lock()
	cancel := a.mrCancel
	started := a.mrStarted
	a.mrStarted = false
	a.mrCancel = nil
	a.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if started {
		a.mrWg.Wait()
	}
}

// measurementReportLoop 는 report_interval 마다 윈도를 마감해 리포트를 방출한다.
//
// interval<=0 이면 즉시 종료한다 (reportLoop 과 동일한 규약).
//
// # 런타임 토글의 한계 (기존 제약과 동일, 악화시키지 않음)
//
// comm watchdog 과 **정확히 같은 한계**를 갖는다: 이 goroutine 은 Init 에서만
// 기동되므로 Configure 로 emit_report 를 false→true 로 바꿔도 기동되지 않고,
// true→false 로 바꿔도 정지되지 않는다(agent.go Configure 주석이 emit_comm_state
// 에 대해 문서화한 것과 동일한 사전 제약이다). 본 변경은 그 제약을 해소하지도,
// 악화시키지도 않는다 — 다만 사용자가 조용히 당황하지 않도록 Configure 가 경고
// 로그를 낸다(logRestartRequiredChanges).
//
// 그 상태에서 누적만 계속되는 경우(업링크 게이트는 즉시 반영되므로 발생 가능)에도
// 메모리는 상한(maxReportDevices × maxReportMeasurements)으로 묶여 있다.
//
// @MX:WARN: context 취소로 반드시 종료되어야 하는 백그라운드 goroutine 이다.
// @MX:REASON: 종료 누락 시 에이전트 Stop 후에도 goroutine 누수 발생.
func (a *ChirpStackAgent) measurementReportLoop(ctx context.Context) {
	defer a.mrWg.Done()

	interval := a.cs().ReportInterval
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.emitMeasurementReports()
		}
	}
}
