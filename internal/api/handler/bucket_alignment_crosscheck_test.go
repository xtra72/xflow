// @spec SPEC-TSDB-002 §2.8 (U8) — 패널 데이터소스 간 버킷 원점 정렬 교차 검증.
//
// 패널이 소스를 갈아탈 때 버킷 경계가 달라지면 같은 데이터가 다른 시각에 찍힌
// 것처럼 보인다. Store 와 InfluxDB(v2 Flux · v3 InfluxQL)가 모든 인터벌에서
// 같은 경계를 산출한다는 사실을 여기서 고정한다.
//
// v0.1.0 은 memTSDB 가 non-divisor 인터벌에서 어긋난다는 사실을 단언했으나,
// memTSDB 는 패널 데이터소스가 아니므로 비교 대상에서 빠졌다. 남은 두 소스는
// 둘 다 epoch-zero 정렬이라 420 초 같은 non-divisor 인터벌에서도 일치한다.
//
// **3소스 단언의 구성**(M7.3). v2 와 v3 의 버킷 시작을 프로덕션의 공용
// 정규화 함수(system.SeriesBucketStartMs)로 얻으면 두 백엔드가 같은 함수로
// 수렴해 "3소스 비교"가 사실은 2소스 비교가 된다. 그래서 여기서는 각 백엔드가
// **자기 방언으로 생성한 쿼리 문자열**에서 윈도우 파라미터(폭 · offset ·
// 레이블 위치)를 읽어 내고, 그 파라미터로 InfluxDB 가 실제로 산출할 경계를
// 테스트가 직접 모델링한다. 백엔드 하나의 템플릿만 바뀌어도(예: Flux 에
// offset: 을 넣거나 timeSrc 를 "_stop" 으로 되돌리거나, InfluxQL GROUP BY
// time() 에 offset 인자를 붙이면) 그 소스만 Store 와 갈라져 실패한다.
package handler

import (
	"strconv"
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

// crosscheckSpec 은 쿼리 생성에 쓰는 최소 spec 이다. 인터벌만 축으로 바뀐다.
func crosscheckSpec(intervalMs int64) system.SeriesQuerySpec {
	return system.SeriesQuerySpec{
		Bucket:      "metrics",
		Measurement: "cpu",
		Field:       "usage",
		StartMs:     1_700_000_000_000,
		EndMs:       1_700_003_600_000,
		IntervalMs:  intervalMs,
		Aggregation: system.SeriesAggAverage,
	}
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

// --- 백엔드별 윈도우 모델 ---

// influxWindow 는 생성된 쿼리에서 읽어 낸 백엔드별 버킷 파라미터다.
//
// 이 구조체가 존재하는 이유는 "무엇이 백엔드마다 다를 수 있는가"를 명시하기
// 위함이다 — 윈도우 폭 · 원점 offset · 버킷 레이블 위치 셋 중 하나라도 방언
// 사이에서 어긋나면 같은 타임스탬프가 다른 시각에 찍힌다.
type influxWindow struct {
	// everyMs 는 윈도우 폭이다.
	everyMs int64
	// offsetMs 는 윈도우 원점 이동이다. 0 이면 epoch 정렬이다.
	offsetMs int64
	// labelAtStart 가 false 이면 버킷 레이블이 윈도우 끝이다.
	labelAtStart bool
}

// bucketStartsMs 는 이 윈도우 파라미터로 InfluxDB 가 산출할 버킷 시작 시각을
// 계산한다.
//
// 여기서 식을 직접 쓰는 것은 의도적이다. InfluxDB 가 문서상 어떻게 윈도우를
// 자르는지(epoch 기준 고정 폭, offset 만큼 원점 이동)를 테스트가 독립 오라클로
// 들고 있어야, 프로덕션의 공용 정규화 함수가 바뀌었을 때 그 변화를 감지한다.
// 공용 함수를 부르면 v2 · v3 · Store 가 한 식으로 수렴해 아무것도 검증하지 못한다.
func (w influxWindow) bucketStartsMs(tsMs []int64) []int64 {
	seen := make(map[int64]struct{}, len(tsMs))
	out := make([]int64, 0, len(tsMs))
	for _, ms := range tsMs {
		shifted := ms - w.offsetMs
		q := shifted / w.everyMs
		if shifted%w.everyMs != 0 && shifted < 0 {
			q-- // 음수 구간에서도 바닥 나눗셈이 되게 한다.
		}
		start := q*w.everyMs + w.offsetMs
		if !w.labelAtStart {
			start += w.everyMs
		}
		if _, dup := seen[start]; dup {
			continue
		}
		seen[start] = struct{}{}
		out = append(out, start)
	}
	return out
}

// parseDurationLiteralMs 는 `420000ms` 형태의 리터럴을 밀리초로 되돌린다.
// 두 방언 모두 ms 단위 리터럴을 쓴다(seriesDurationLiteral).
func parseDurationLiteralMs(t *testing.T, lit string) int64 {
	t.Helper()
	trimmed := strings.TrimSpace(lit)
	require.True(t, strings.HasSuffix(trimmed, "ms"), "duration 리터럴이 ms 단위가 아니다: %q", trimmed)
	ms, err := strconv.ParseInt(strings.TrimSuffix(trimmed, "ms"), 10, 64)
	require.NoError(t, err, "duration 리터럴 파싱 실패: %q", trimmed)
	return ms
}

// fluxWindowFromQuery 는 **v2 가 스스로 생성한 Flux 쿼리**에서 윈도우 파라미터를
// 읽는다. aggregateWindow 의 every · offset · timeSrc 가 v2 의 경계를 결정한다.
func fluxWindowFromQuery(t *testing.T, intervalMs int64) influxWindow {
	t.Helper()
	q, err := system.BuildFluxSeriesQuery(crosscheckSpec(intervalMs))
	require.NoError(t, err)

	at := strings.Index(q, "aggregateWindow(")
	require.GreaterOrEqual(t, at, 0, "생성된 Flux 에 aggregateWindow 가 없다")
	args := q[at+len("aggregateWindow("):]
	closing := strings.Index(args, ")")
	require.GreaterOrEqual(t, closing, 0)
	args = args[:closing]

	w := influxWindow{}

	everyAt := strings.Index(args, "every:")
	require.GreaterOrEqual(t, everyAt, 0, "aggregateWindow 에 every 인자가 없다")
	everyArg := args[everyAt+len("every:"):]
	if comma := strings.Index(everyArg, ","); comma >= 0 {
		everyArg = everyArg[:comma]
	}
	w.everyMs = parseDurationLiteralMs(t, everyArg)

	// offset 인자가 없으면 Flux 기본값 0 — 즉 epoch 정렬이다.
	if offsetAt := strings.Index(args, "offset:"); offsetAt >= 0 {
		offsetArg := args[offsetAt+len("offset:"):]
		if comma := strings.Index(offsetArg, ","); comma >= 0 {
			offsetArg = offsetArg[:comma]
		}
		w.offsetMs = parseDurationLiteralMs(t, offsetArg)
	}

	// timeSrc 를 명시하지 않으면 aggregateWindow 는 윈도우 끝(_stop)을 레이블로
	// 방출한다. 즉 기본값은 labelAtStart = false 다.
	w.labelAtStart = strings.Contains(args, `timeSrc: "_start"`)

	return w
}

// influxQLWindowFromQuery 는 **v3 가 스스로 생성한 InfluxQL 쿼리**에서 윈도우
// 파라미터를 읽는다. GROUP BY time(d[, offset]) 이 v3 의 경계를 결정한다.
//
// InfluxQL 의 GROUP BY time() 은 언제나 윈도우 시작을 레이블로 쓰므로
// labelAtStart 는 방언 규약상 고정이다(쿼리 문자열이 바꿀 수 있는 축이 아니다).
func influxQLWindowFromQuery(t *testing.T, intervalMs int64) influxWindow {
	t.Helper()
	q, err := system.BuildInfluxQLSeriesQuery(crosscheckSpec(intervalMs))
	require.NoError(t, err)

	at := strings.Index(q, "GROUP BY time(")
	require.GreaterOrEqual(t, at, 0, "생성된 InfluxQL 에 GROUP BY time( 이 없다")
	args := q[at+len("GROUP BY time("):]
	closing := strings.Index(args, ")")
	require.GreaterOrEqual(t, closing, 0)
	args = args[:closing]

	w := influxWindow{labelAtStart: true}
	parts := strings.SplitN(args, ",", 2)
	w.everyMs = parseDurationLiteralMs(t, parts[0])
	if len(parts) == 2 {
		// offset 인자를 주면 기본 offset 0(=epoch 정렬)이 깨진다.
		w.offsetMs = parseDurationLiteralMs(t, parts[1])
	}
	return w
}

// assertThreeSourcesAgree 는 Store · v2 · v3 를 각각의 경로로 산출해 세 방향
// 모두를 단언한다. 세 방향을 모두 적는 이유는, 두 방향만 적으면 어느 소스가
// 갈라졌는지 실패 메시지에서 읽히지 않기 때문이다.
func assertThreeSourcesAgree(t *testing.T, intervalMs int64) {
	t.Helper()
	storeStarts := storeBucketStartsMs(t, crosscheckTimestampsMs, intervalMs)
	v2Starts := fluxWindowFromQuery(t, intervalMs).bucketStartsMs(crosscheckTimestampsMs)
	v3Starts := influxQLWindowFromQuery(t, intervalMs).bucketStartsMs(crosscheckTimestampsMs)

	require.NotEmpty(t, storeStarts, "interval=%d", intervalMs)
	assert.ElementsMatch(t, storeStarts, v2Starts,
		"Store 와 InfluxDB v2(Flux)의 버킷 경계가 interval=%d 에서 갈라졌다", intervalMs)
	assert.ElementsMatch(t, storeStarts, v3Starts,
		"Store 와 InfluxDB v3(InfluxQL)의 버킷 경계가 interval=%d 에서 갈라졌다", intervalMs)
	assert.ElementsMatch(t, v2Starts, v3Starts,
		"InfluxDB v2 와 v3 의 버킷 경계가 interval=%d 에서 갈라졌다", intervalMs)
}

func TestBucketAlignment_DivisorIntervals_AllPanelSourcesAgree(t *testing.T) {
	t.Parallel()
	for _, intervalMs := range divisorIntervalsMs {
		assertThreeSourcesAgree(t, intervalMs)
	}
}

func TestBucketAlignment_NonDivisorInterval_AllPanelSourcesAgree(t *testing.T) {
	t.Parallel()
	// 420 초는 86400 의 약수가 아니지만 세 소스 모두 epoch-zero 정렬이므로 일치한다.
	assertThreeSourcesAgree(t, nonDivisorIntervalMs)
}

// 각 백엔드가 읽어 낸 윈도우 파라미터 자체를 고정한다. 위 교차 검증이 실패했을 때
// "폭이 틀렸는가 · 원점이 밀렸는가 · 레이블이 끝인가"를 이 테스트가 갈라 준다.
func TestBucketAlignment_PerBackendWindowParameters(t *testing.T) {
	t.Parallel()
	intervals := append([]int64{nonDivisorIntervalMs}, divisorIntervalsMs...)
	for _, intervalMs := range intervals {
		v2 := fluxWindowFromQuery(t, intervalMs)
		assert.Equal(t, intervalMs, v2.everyMs, "v2 윈도우 폭, interval=%d", intervalMs)
		assert.Zero(t, v2.offsetMs, "v2 는 epoch 정렬이어야 한다, interval=%d", intervalMs)
		assert.True(t, v2.labelAtStart, "v2 버킷 레이블이 시작이어야 한다, interval=%d", intervalMs)

		v3 := influxQLWindowFromQuery(t, intervalMs)
		assert.Equal(t, intervalMs, v3.everyMs, "v3 윈도우 폭, interval=%d", intervalMs)
		assert.Zero(t, v3.offsetMs, "v3 는 epoch 정렬이어야 한다, interval=%d", intervalMs)
		assert.True(t, v3.labelAtStart, "v3 버킷 레이블이 시작이어야 한다, interval=%d", intervalMs)
	}
}

// 런타임 정규화(normalizeSeriesBuckets 가 v2 · v3 양쪽에서 부르는
// system.SeriesBucketStartMs)도 Store 와 같은 경계를 만든다.
//
// 이 단언은 위 3소스 단언을 대체하지 않는다 — 두 백엔드가 같은 함수로 수렴하는
// 지점이 여기이며, 그 수렴 자체가 Store 와 어긋나지 않는다는 사실만 고정한다.
func TestBucketAlignment_SharedRuntimeNormalizationMatchesStore(t *testing.T) {
	t.Parallel()
	intervals := append([]int64{nonDivisorIntervalMs}, divisorIntervalsMs...)
	for _, intervalMs := range intervals {
		storeStarts := storeBucketStartsMs(t, crosscheckTimestampsMs, intervalMs)

		seen := make(map[int64]struct{}, len(crosscheckTimestampsMs))
		normalized := make([]int64, 0, len(crosscheckTimestampsMs))
		for _, ms := range crosscheckTimestampsMs {
			start := system.SeriesBucketStartMs(ms, intervalMs)
			if _, dup := seen[start]; dup {
				continue
			}
			seen[start] = struct{}{}
			normalized = append(normalized, start)
		}

		require.NotEmpty(t, storeStarts, "interval=%d", intervalMs)
		assert.ElementsMatch(t, storeStarts, normalized,
			"공용 런타임 정규화가 Store 와 interval=%d 에서 갈라졌다", intervalMs)
	}
}

// 생성되는 쿼리 자체가 epoch 정렬을 요구하는지 확인한다. 정렬은 서버가 소유하며
// (§2.8) 클라이언트가 경계를 재계산하지 않는다.
func TestBucketAlignment_GeneratedQueriesRequestEpochAlignment(t *testing.T) {
	t.Parallel()
	intervals := append([]int64{nonDivisorIntervalMs}, divisorIntervalsMs...)
	for _, intervalMs := range intervals {
		spec := crosscheckSpec(intervalMs)

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
