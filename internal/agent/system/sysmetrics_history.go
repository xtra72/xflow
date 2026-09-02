package system

// 표본 이력 버퍼.
//
// 에이전트가 설정된 기간만큼 표본을 들고 있어, 대시보드 패널이 브라우저에서 점을
// 누적하는 대신 이력을 **질의**한다. 패널을 열자마자 과거 구간이 그려지고, 패널을
// 닫았다 열어도 선이 초기화되지 않는다.
//
// 보관하는 것은 원표본이 아니라 **시리즈별 값**이다. 누적 카운터(네트워크·디스크 I/O)는
// 저장 시점에 초당 증가량으로 환산한다 — 그래야 패널이 Store·TSDB 와 똑같이 "숫자
// 시계열을 버킷으로 집계" 하기만 하면 되고, 증가량 단위 같은 이 소스 전용 축이
// 화면에 남지 않는다.
//
// 프로세스 메모리이며 재시작하면 사라진다. 영속 이력은 저장 경로(`sysmetrics-in` →
// storage-write)의 몫이다.

import (
	"sort"
	"time"
)

// SysMetricsSeriesKey 는 시리즈 하나의 동일성이다.
//
// 대시보드의 Store 어휘와 1:1 이다 — measurement 는 그룹을 뗀 필드 이름이고, 그룹과
// 인스턴스 대상은 태그로 간다. 두 화면이 같은 어휘를 쓰지 않으면 같은 값이 소스마다
// 다른 이름으로 읽힌다.
type SysMetricsSeriesKey struct {
	// Measurement 는 필드 이름이다(`usage_percent` · `bytes_recv`).
	Measurement string `json:"measurement"`
	// Category 는 지표 분류다(`cpu` · `network` · `disk_io`). 배치 그룹 이름과 같다.
	Category string `json:"category"`
	// Target 은 인스턴스 대상이다(`en0` · `/data`). **빈 문자열은 종합**이다.
	Target string `json:"target,omitempty"`
	// Mode 는 누적 카운터의 표현 방식이다.
	//
	// **빈 문자열이 그 분류의 기본 표현**이다 — 누적 카운터는 초당 증가량, 나머지는
	// 그 시점의 상태값. `sysMetricsModeTotal` 은 누적 카운터의 원값(부팅 이후 누적)을
	// 뜻하며 카운터 분류에만 나타난다.
	//
	// 기본 표현에 이름을 싣지 않는 이유는 호환이다. 이미 저장된 패널의 시리즈 태그에는
	// 이 축이 없으므로, 증가량 시리즈에 `mode` 를 실으면 그 패널들이 시리즈를 찾지
	// 못해 조용히 **빈 차트**가 된다.
	Mode string `json:"mode,omitempty"`
}

// sysMetricsModeTotal 은 누적 원값 시리즈의 Mode 값이다.
//
// 값이 아니라 이름이므로 바꾸면 이미 저장된 패널의 시리즈 태그와 끊긴다.
const sysMetricsModeTotal = "total"

// sysMetricsHistoryPoint 는 한 시점의 시리즈 값 묶음이다.
type sysMetricsHistoryPoint struct {
	timeMs int64
	values map[SysMetricsSeriesKey]float64
}

// SysMetricsHistoryResult 는 이력 질의 응답이다.
//
// 열 지향(columnar)이다 — 점마다 키를 반복하면 표본 수천 개에서 응답이 키 문자열로
// 가득 찬다. `Values[i]` 는 `Series[i]` 에 대응하며, 그 시점에 값이 없으면 nil 이다.
type SysMetricsHistoryResult struct {
	Series []SysMetricsSeriesKey  `json:"series"`
	Points []SysMetricsHistoryRow `json:"points"`
	// Retention 은 실제 보관 기간(초)이다. 표본 수 상한에 걸리면 설정값보다 짧다.
	RetentionSeconds float64 `json:"retention_seconds"`
	// Truncated 는 표본 수 상한 때문에 오래된 표본을 버리고 있는지다.
	Truncated bool `json:"truncated"`
}

// SysMetricsHistoryRow 는 한 시점의 값 배열이다.
type SysMetricsHistoryRow struct {
	TimeMs int64      `json:"time_ms"`
	Values []*float64 `json:"values"`
}

// sysMetricsHistory 는 표본 이력 링 버퍼이다. 호출부가 잠금을 쥔다.
type sysMetricsHistory struct {
	points []sysMetricsHistoryPoint
	// prev 는 증가량 환산의 기준점이다(누적 카운터의 직전 원값).
	prev map[SysMetricsSeriesKey]float64
	// prevAt 은 기준점의 시각이다.
	prevAt int64
	// truncated 는 표본 수 상한에 걸려 오래된 표본을 버린 적이 있는지다.
	truncated bool
}

// newSysMetricsHistory 는 빈 이력을 만든다.
func newSysMetricsHistory() *sysMetricsHistory {
	return &sysMetricsHistory{prev: make(map[SysMetricsSeriesKey]float64)}
}

// counterCategories 는 누적 카운터 분류다.
//
// 이 분류의 값은 부팅 이후 누적이라 그대로 두면 언제나 커지기만 하는 수다. 저장
// 시점에 초당 증가량으로 환산한다. cpu·memory·storage 는 그 시점의 상태값이므로
// 환산하지 않는다.
var counterCategories = map[string]bool{
	groupDiskIO:  true,
	groupNetwork: true,
}

// flattenSample 은 표본을 시리즈별 **원값** 맵으로 편다.
//
// 인스턴스 축이 있는 그룹은 대상별 값과 **종합**(전체 합)을 함께 낸다. 종합은 개별과
// 다른 시리즈이며, 대상을 고르지 않은 패널이 보는 값이다.
//
// 스토리지 종합 사용률만 특별하다 — 마운트별 퍼센트를 더하면 200% 가 나오므로 합계
// 용량 기준으로 다시 계산한다.
func flattenSample(sample SysMetricsSample) map[SysMetricsSeriesKey]float64 {
	out := make(map[SysMetricsSeriesKey]float64, 32)

	for group, value := range buildBatch(sample).Fields {
		switch v := value.(type) {
		case MetricGroup:
			// 인스턴스 축이 없다 — 값 하나가 곧 시리즈 하나다.
			for field, num := range v {
				out[SysMetricsSeriesKey{Measurement: field, Category: group}] = num
			}
		case InstanceGroup:
			totals := make(map[string]float64, 8)
			for target, metrics := range v {
				for field, num := range metrics {
					out[SysMetricsSeriesKey{Measurement: field, Category: group, Target: target}] = num
					totals[field] += num
				}
			}
			for field, num := range totals {
				out[SysMetricsSeriesKey{Measurement: field, Category: group}] = num
			}
			if group == groupStorage {
				// 퍼센트의 합은 뜻이 없다. 합계 용량 기준으로 다시 낸다.
				if total := totals["total_bytes"]; total > 0 {
					out[SysMetricsSeriesKey{Measurement: "usage_percent", Category: group}] =
						totals["used_bytes"] / total * 100
				} else {
					delete(out, SysMetricsSeriesKey{Measurement: "usage_percent", Category: group})
				}
			}
		}
	}
	return out
}

// append 는 표본 하나를 이력에 넣는다.
//
// 누적 카운터는 **두 갈래로** 싣는다:
//
//   - 증가량(Mode 빈 값): 직전 표본과의 차이를 초당으로 환산한다. 환산할 수 없는 경우
//     (첫 표본 · 카운터 되감김 · 경과 0)에는 **그 시리즈의 값을 싣지 않는다** — 0 으로
//     대신하면 재부팅 직후 트래픽이 끊긴 것처럼 보인다.
//   - 누적 원값(Mode `total`): 그 시점의 카운터 그대로. 기준점이 필요 없으므로 첫
//     표본부터 값이 있다.
//
// 두 갈래를 다 싣는 이유는 소비자가 줄마다 다르게 고르기 때문이다. 질의 시점에
// 환산하도록 미루면 원값을 따로 보관해야 하는데, 그러면 지금 이 함수가 하는 일을
// 질의가 표본 수만큼 반복하게 된다.
func (h *sysMetricsHistory) append(sample SysMetricsSample, retention time.Duration) {
	raw := flattenSample(sample)
	at := sample.Timestamp

	values := make(map[SysMetricsSeriesKey]float64, len(raw))
	elapsed := float64(at-h.prevAt) / 1000
	for key, num := range raw {
		if !counterCategories[key.Category] {
			values[key] = num
			continue
		}
		// 누적 원값은 언제나 실린다 — 환산이 없으니 실패할 구간도 없다.
		totalKey := key
		totalKey.Mode = sysMetricsModeTotal
		values[totalKey] = num

		prev, ok := h.prev[key]
		// 기준점이 없거나, 되감겼거나, 경과가 0 이면 그릴 수 없다.
		if !ok || h.prevAt == 0 || elapsed <= 0 || num < prev {
			continue
		}
		values[key] = (num - prev) / elapsed
	}

	h.prev = raw
	h.prevAt = at
	h.points = append(h.points, sysMetricsHistoryPoint{timeMs: at, values: values})
	h.trim(at, retention)
}

// trim 은 보관 범위를 벗어난 표본을 버린다. 기간과 개수 두 축으로 죈다.
func (h *sysMetricsHistory) trim(nowMs int64, retention time.Duration) {
	cutoff := nowMs - retention.Milliseconds()
	keep := 0
	for keep < len(h.points) && h.points[keep].timeMs < cutoff {
		keep++
	}
	if keep > 0 {
		h.points = append(h.points[:0], h.points[keep:]...)
	}
	if over := len(h.points) - maxSysMetricsHistorySamples; over > 0 {
		h.points = append(h.points[:0], h.points[over:]...)
		h.truncated = true
	}
}

// query 는 시간 구간의 이력을 열 지향 형태로 낸다.
//
// `startMs`/`endMs` 가 0 이면 그 경계를 두지 않는다. 시리즈 순서는 **결정적**이다
// (분류 → measurement → 대상 → 표현) — 순서가 호출마다 바뀌면 열 위치가 흔들려
// 소비자가 컬럼을 잘못 귀속한다.
func (h *sysMetricsHistory) query(startMs, endMs int64, retention time.Duration) SysMetricsHistoryResult {
	rows := make([]sysMetricsHistoryPoint, 0, len(h.points))
	seen := make(map[SysMetricsSeriesKey]bool, 32)
	for _, p := range h.points {
		if startMs > 0 && p.timeMs < startMs {
			continue
		}
		if endMs > 0 && p.timeMs >= endMs {
			continue
		}
		rows = append(rows, p)
		for key := range p.values {
			seen[key] = true
		}
	}

	series := make([]SysMetricsSeriesKey, 0, len(seen))
	for key := range seen {
		series = append(series, key)
	}
	sort.Slice(series, func(i, j int) bool {
		a, b := series[i], series[j]
		if a.Category != b.Category {
			return a.Category < b.Category
		}
		if a.Measurement != b.Measurement {
			return a.Measurement < b.Measurement
		}
		if a.Target != b.Target {
			return a.Target < b.Target
		}
		return a.Mode < b.Mode
	})

	points := make([]SysMetricsHistoryRow, 0, len(rows))
	for _, p := range rows {
		vals := make([]*float64, len(series))
		for i, key := range series {
			if v, ok := p.values[key]; ok {
				num := v
				vals[i] = &num
			}
		}
		points = append(points, SysMetricsHistoryRow{TimeMs: p.timeMs, Values: vals})
	}

	return SysMetricsHistoryResult{
		Series:           series,
		Points:           points,
		RetentionSeconds: h.retention(retention).Seconds(),
		Truncated:        h.truncated,
	}
}

// retention 은 **실제** 보관 기간이다.
//
// 표본 수 상한에 걸리면 설정 기간보다 짧다. 설정값을 그대로 돌려주면 사용자는 왜
// 화면이 설정만큼 거슬러 올라가지 않는지 알 수 없다.
func (h *sysMetricsHistory) retention(configured time.Duration) time.Duration {
	if len(h.points) < 2 {
		return configured
	}
	span := time.Duration(h.points[len(h.points)-1].timeMs-h.points[0].timeMs) * time.Millisecond
	if h.truncated && span < configured {
		return span
	}
	return configured
}
