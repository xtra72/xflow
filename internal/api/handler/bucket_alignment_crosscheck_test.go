// @spec SPEC-TSDB-002 §2.8 (U8) — 패널 데이터소스 간 버킷 원점 정렬 교차 검증.
//
// 패널이 소스를 갈아탈 때 버킷 경계가 달라지면 같은 데이터가 다른 시각에 찍힌
// 것처럼 보인다. Store 와 InfluxDB(v2 Flux · v3 InfluxQL)가 모든 인터벌에서
// 같은 경계를 산출한다는 사실을 여기서 고정한다.
//
// v0.1.0 은 memTSDB 가 non-divisor 인터벌에서 어긋난다는 사실을 단언했으나,
// memTSDB 는 패널 데이터소스가 아니므로 비교 대상에서 빠졌다. 남은 두 소스는
// 둘 다 epoch-zero 정렬이라 420 초 같은 non-divisor 인터벌에서도 일치한다.
package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
)

// divisorIntervalsMs 는 86400 을 나누어떨어지게 하는 인터벌들이다.
var divisorIntervalsMs = []int64{
	1_000, 10_000, 30_000, 60_000, 300_000, 900_000, 3_600_000,
}

// nonDivisorIntervalMs 는 86400 을 나누어떨어지게 하지 않는 인터벌이다(420 초).
const nonDivisorIntervalMs = int64(420_000)

// crosscheckTimestampsMs 는 경계 · 경계 직전 · 경계 직후를 섞은 표본이다.
var crosscheckTimestampsMs = []int64{
	0,
	1,
	999,
	86_399_999,
	86_400_000,
	1_700_000_000_000,
	1_700_000_000_001,
	1_700_000_419_999,
	1_700_000_420_000,
}

// storeBucketStartsMs 는 Store 의 실제 집계 경로를 태워 버킷 시작 시각을 얻는다.
// 식을 테스트에 복제하지 않고 프로덕션 코드를 그대로 부르는 것이 요점이다.
func storeBucketStartsMs(t *testing.T, tsMs []int64, intervalMs int64) []int64 {
	t.Helper()
	entries := make([]system.HistoryEntry, 0, len(tsMs))
	for _, ms := range tsMs {
		entries = append(entries, system.HistoryEntry{
			Value:     1.0,
			Timestamp: time.UnixMilli(ms).UTC(),
		})
	}
	// originMs = 0 이면 범위 하한 필터가 아무것도 거르지 않는다.
	aggregated := bucketAggregate(entries, 0, intervalMs, aggregationAvg)
	out := make([]int64, 0, len(aggregated))
	for _, e := range aggregated {
		out = append(out, e.Timestamp)
	}
	return out
}

// influxBucketStartsMs 는 InfluxDB 어댑터의 버킷 정렬을 태운다.
// v2/v3 모두 QuerySeriesBuckets 의 정규화에서 이 함수를 통과한다.
func influxBucketStartsMs(tsMs []int64, intervalMs int64) []int64 {
	seen := make(map[int64]struct{}, len(tsMs))
	out := make([]int64, 0, len(tsMs))
	for _, ms := range tsMs {
		start := system.SeriesBucketStartMs(ms, intervalMs)
		if _, dup := seen[start]; dup {
			continue
		}
		seen[start] = struct{}{}
		out = append(out, start)
	}
	return out
}

func TestBucketAlignment_DivisorIntervals_AllPanelSourcesAgree(t *testing.T) {
	t.Parallel()
	for _, intervalMs := range divisorIntervalsMs {
		storeStarts := storeBucketStartsMs(t, crosscheckTimestampsMs, intervalMs)
		influxStarts := influxBucketStartsMs(crosscheckTimestampsMs, intervalMs)
		require.NotEmpty(t, storeStarts, "interval=%d", intervalMs)
		assert.ElementsMatch(t, storeStarts, influxStarts,
			"Store 와 InfluxDB 의 버킷 경계가 interval=%d 에서 갈라졌다", intervalMs)
	}
}

func TestBucketAlignment_NonDivisorInterval_AllPanelSourcesAgree(t *testing.T) {
	t.Parallel()
	storeStarts := storeBucketStartsMs(t, crosscheckTimestampsMs, nonDivisorIntervalMs)
	influxStarts := influxBucketStartsMs(crosscheckTimestampsMs, nonDivisorIntervalMs)
	require.NotEmpty(t, storeStarts)
	// 420 초는 86400 의 약수가 아니지만 두 소스 모두 epoch-zero 정렬이므로 일치한다.
	assert.ElementsMatch(t, storeStarts, influxStarts)
}

// 생성되는 쿼리 자체가 epoch 정렬을 요구하는지 확인한다. 정렬은 서버가 소유하며
// (§2.8) 클라이언트가 경계를 재계산하지 않는다.
func TestBucketAlignment_GeneratedQueriesRequestEpochAlignment(t *testing.T) {
	t.Parallel()
	intervals := append([]int64{nonDivisorIntervalMs}, divisorIntervalsMs...)
	for _, intervalMs := range intervals {
		spec := system.SeriesQuerySpec{
			Bucket:      "metrics",
			Measurement: "cpu",
			Field:       "usage",
			StartMs:     1_700_000_000_000,
			EndMs:       1_700_003_600_000,
			IntervalMs:  intervalMs,
			Aggregation: system.SeriesAggAverage,
		}

		flux, err := system.BuildFluxSeriesQuery(spec)
		require.NoError(t, err)
		// 버킷 레이블이 시작이어야 Store 와 같은 시각에 찍힌다.
		assert.Contains(t, flux, `timeSrc: "_start"`, "interval=%d", intervalMs)
		// offset 인자를 주면 epoch 정렬이 깨진다.
		assert.NotContains(t, flux, "offset:", "interval=%d", intervalMs)

		iql, err := system.BuildInfluxQLSeriesQuery(spec)
		require.NoError(t, err)
		at := strings.Index(iql, "GROUP BY time(")
		require.GreaterOrEqual(t, at, 0)
		rest := iql[at+len("GROUP BY time("):]
		closing := strings.Index(rest, ")")
		require.GreaterOrEqual(t, closing, 0)
		assert.NotContains(t, rest[:closing], ",",
			"GROUP BY time(d) 에 offset 을 주면 기본 offset 0(=epoch 정렬)이 깨진다")
	}
}
