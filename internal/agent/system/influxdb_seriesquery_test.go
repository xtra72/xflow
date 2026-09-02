// @spec SPEC-TSDB-002 §2.7 (U7) · §2.8 (U8)
package system

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// baseSpec 는 골든 대조에 쓰는 기준 입력이다.
// StartMs 1700000000000 = 2023-11-14T22:13:20Z, EndMs 는 그로부터 1시간 뒤다.
func baseSpec() SeriesQuerySpec {
	return SeriesQuerySpec{
		Bucket:      "metrics",
		Measurement: "cpu",
		Field:       "usage",
		Tags:        map[string]string{"host": "a"},
		StartMs:     1_700_000_000_000,
		EndMs:       1_700_003_600_000,
		IntervalMs:  60_000,
		Aggregation: SeriesAggAverage,
		Fill:        SeriesFillNone,
	}
}

// allAggregations 는 §2.7 매핑 표의 7 종 전부다.
var allAggregations = []SeriesAggregation{
	SeriesAggMin, SeriesAggMax, SeriesAggAverage, SeriesAggFirst, SeriesAggLast,
	SeriesAggSum, SeriesAggCount,
}

// allFills 는 §2.7 fill 표의 5 종 전부다(거부 대상 1 종 포함).
var allFills = []SeriesFill{
	SeriesFillNone, SeriesFillNull, SeriesFillZero, SeriesFillPrevious, SeriesFillAvg,
}

// tagCases 는 태그 0 개 / 1 개 / N 개 세 형상이다.
var tagCases = map[string]map[string]string{
	"tags=0": nil,
	"tags=1": {"host": "a"},
	"tags=N": {"host": "a", "region": "kr", "az": "kr-1"},
}

// --- AC-21: Flux 생성 계층 골든 대조 ---

func TestBuildFluxSeriesQuery_Golden(t *testing.T) {
	t.Parallel()
	got, err := BuildFluxSeriesQuery(baseSpec())
	require.NoError(t, err)

	want := strings.Join([]string{
		`from(bucket: "metrics")`,
		`  |> range(start: time(v: 1700000000000000000), stop: time(v: 1700003600000000000))`,
		`  |> filter(fn: (r) => r._measurement == "cpu")`,
		`  |> filter(fn: (r) => r._field == "usage")`,
		`  |> filter(fn: (r) => r["host"] == "a")`,
		`  |> aggregateWindow(every: 60000ms, fn: mean, createEmpty: false, timeSrc: "_start")`,
		`  |> keep(columns: ["_time", "_value"])`,
	}, "\n")
	assert.Equal(t, want, got)
}

func TestBuildFluxSeriesQuery_TagsAreSortedByKey(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Tags = map[string]string{"host": "a", "region": "kr", "az": "kr-1"}

	// 맵 순회 순서는 비결정적이므로 여러 번 만들어도 같아야 한다.
	first, err := BuildFluxSeriesQuery(spec)
	require.NoError(t, err)
	for i := 0; i < 20; i++ {
		again, err := BuildFluxSeriesQuery(spec)
		require.NoError(t, err)
		require.Equal(t, first, again)
	}
	azAt := strings.Index(first, `r["az"]`)
	hostAt := strings.Index(first, `r["host"]`)
	regionAt := strings.Index(first, `r["region"]`)
	require.Positive(t, azAt)
	assert.Less(t, azAt, hostAt)
	assert.Less(t, hostAt, regionAt)
}

// --- AC-22: InfluxQL 생성 계층 골든 대조 ---

func TestBuildInfluxQLSeriesQuery_Golden(t *testing.T) {
	t.Parallel()
	got, err := BuildInfluxQLSeriesQuery(baseSpec())
	require.NoError(t, err)

	want := strings.Join([]string{
		`SELECT MEAN("usage") FROM "cpu"`,
		` WHERE time >= '2023-11-14T22:13:20Z' AND time < '2023-11-14T23:13:20Z'`,
		`   AND "host" = 'a'`,
		` GROUP BY time(60000ms) FILL(none)`,
	}, "\n")
	assert.Equal(t, want, got)
}

// GROUP BY time(d) 에 offset 인자를 주면 epoch 정렬이 깨져 Store 와 버킷 경계가
// 어긋난다(§2.8). 인자가 하나뿐임을 생성 결과에서 직접 확인한다.
func TestBuildInfluxQLSeriesQuery_GroupByHasNoOffsetArgument(t *testing.T) {
	t.Parallel()
	for name, tags := range tagCases {
		t.Run(name, func(t *testing.T) {
			spec := baseSpec()
			spec.Tags = tags
			got, err := BuildInfluxQLSeriesQuery(spec)
			require.NoError(t, err)

			at := strings.Index(got, "GROUP BY time(")
			require.GreaterOrEqual(t, at, 0)
			rest := got[at+len("GROUP BY time("):]
			closing := strings.Index(rest, ")")
			require.GreaterOrEqual(t, closing, 0)
			assert.NotContains(t, rest[:closing], ",",
				"GROUP BY time(...) 은 인터벌 인자 하나만 받아야 한다")
		})
	}
}

// --- AC-25: 집계 어휘 매핑 7 종 전수 ---

func TestSeriesQueryAggregationMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		agg      SeriesAggregation
		fluxFn   string
		influxQL string
	}{
		{"min", SeriesAggMin, "min", "MIN"},
		{"max", SeriesAggMax, "max", "MAX"},
		// 매핑 표에서 유일하게 이름이 어긋나는 항목이다. 산술평균의 Flux 이름은
		// mean, InfluxQL 이름은 MEAN 이며 둘 다 요청 어휘와 다르다.
		{"arithmetic-mean", SeriesAggAverage, "mean", "MEAN"},
		{"first", SeriesAggFirst, "first", "FIRST"},
		{"last", SeriesAggLast, "last", "LAST"},
		// 합·횟수. 이름은 양쪽 백엔드 모두 요청 어휘와 같다(대소문자만 다르다).
		{"sum", SeriesAggSum, "sum", "SUM"},
		{"count", SeriesAggCount, "count", "COUNT"},
	}
	require.Len(t, cases, len(allAggregations), "7 종 전수여야 한다")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := baseSpec()
			spec.Aggregation = tc.agg

			flux, err := BuildFluxSeriesQuery(spec)
			require.NoError(t, err)
			assert.Contains(t, flux, fmt.Sprintf("fn: %s,", tc.fluxFn))

			iql, err := BuildInfluxQLSeriesQuery(spec)
			require.NoError(t, err)
			assert.Contains(t, iql, fmt.Sprintf(`SELECT %s("usage")`, tc.influxQL))
		})
	}
}

func TestSeriesQueryAggregationMapping_UnknownRejected(t *testing.T) {
	t.Parallel()
	for _, agg := range []SeriesAggregation{seriesAggUnspecified, SeriesAggregation(99), SeriesAggregation(-1)} {
		spec := baseSpec()
		spec.Aggregation = agg

		_, err := BuildFluxSeriesQuery(spec)
		require.Error(t, err, "미지의 집계는 조용히 통과해서는 안 된다")
		assert.Contains(t, err.Error(), "unsupported series aggregation")

		_, err = BuildInfluxQLSeriesQuery(spec)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported series aggregation")
	}
}

// --- AC-24: fill 매핑 5 종 × (v2/v3) ---

func TestSeriesQueryFillMapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		fill SeriesFill
		// fluxContains 는 생성된 Flux 에 반드시 포함되어야 하는 조각이다.
		fluxContains []string
		// fluxAbsent 는 포함되어서는 안 되는 조각이다.
		fluxAbsent []string
		influxQL   string
		wantErr    bool
	}{
		{
			name:         "none",
			fill:         SeriesFillNone,
			fluxContains: []string{"createEmpty: false"},
			fluxAbsent:   []string{"|> fill("},
			influxQL:     "FILL(none)",
		},
		{
			name:         "null",
			fill:         SeriesFillNull,
			fluxContains: []string{"createEmpty: true"},
			fluxAbsent:   []string{"|> fill("},
			influxQL:     "FILL(null)",
		},
		{
			name:         "zero",
			fill:         SeriesFillZero,
			fluxContains: []string{"createEmpty: true", "|> fill(value: 0.0)"},
			influxQL:     "FILL(0)",
		},
		{
			// 직전값 채우기는 DB 에 맡기지 않는다 — 두 방언 모두 "몇 칸까지만
			// 이어라" 를 표현하지 못해 사용 기간 제한을 걸 수 없다. 빈 버킷만
			// null 로 받아 오고 이어 쓰기는 응답 후처리(applyPreviousFill)가 한다.
			name:         "previous",
			fill:         SeriesFillPrevious,
			fluxContains: []string{"createEmpty: true"},
			fluxAbsent:   []string{"|> fill("},
			influxQL:     "FILL(null)",
		},
		{
			// 프런트 계약에 남아 있는 값이지만 InfluxDB 양쪽 모두 대응물이 없다.
			// 조용히 다른 전략으로 대체하지 않고 거부한다(§2.7 · UB1-17).
			name:    "mean-fill-unsupported",
			fill:    SeriesFillAvg,
			wantErr: true,
		},
	}
	require.Len(t, cases, len(allFills), "5 종 전수여야 한다")

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := baseSpec()
			spec.Fill = tc.fill

			flux, fluxErr := BuildFluxSeriesQuery(spec)
			iql, iqlErr := BuildInfluxQLSeriesQuery(spec)

			if tc.wantErr {
				require.Error(t, fluxErr)
				require.Error(t, iqlErr)
				assert.True(t, errors.Is(fluxErr, ErrUnsupportedSeriesFill))
				assert.True(t, errors.Is(iqlErr, ErrUnsupportedSeriesFill))
				return
			}

			require.NoError(t, fluxErr)
			require.NoError(t, iqlErr)
			for _, frag := range tc.fluxContains {
				assert.Contains(t, flux, frag)
			}
			for _, frag := range tc.fluxAbsent {
				assert.NotContains(t, flux, frag)
			}
			assert.Contains(t, iql, tc.influxQL)
		})
	}
}

func TestSeriesQueryFillMapping_UnknownRejected(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Fill = SeriesFill(42)

	_, err := BuildFluxSeriesQuery(spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported series fill")

	_, err = BuildInfluxQLSeriesQuery(spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported series fill")
}

// --- AC-26: 버킷 레이블이 시작 시각이다 ---

func TestBuildFluxSeriesQuery_TimeSrcIsStart(t *testing.T) {
	t.Parallel()
	for name, tags := range tagCases {
		for _, agg := range allAggregations {
			spec := baseSpec()
			spec.Tags = tags
			spec.Aggregation = agg
			got, err := BuildFluxSeriesQuery(spec)
			require.NoError(t, err, name)

			assert.Contains(t, got, `timeSrc: "_start"`,
				"기본값에 맡기면 버킷 끝이 레이블이 되어 전 데이터가 한 인터벌 미래로 밀린다")
			assert.NotContains(t, got, "_stop")
		}
	}
}

func TestSeriesBucketStartMs_MatchesStoreEpochZeroFormula(t *testing.T) {
	t.Parallel()
	// Store 의 정본 식과 동일해야 한다: (tsMs / intervalMs) * intervalMs.
	for _, intervalMs := range []int64{1_000, 10_000, 30_000, 60_000, 300_000, 900_000, 3_600_000, 420_000} {
		for _, tsMs := range []int64{0, 1, 999, 1_700_000_000_123, 1_700_000_000_999} {
			want := (tsMs / intervalMs) * intervalMs
			assert.Equal(t, want, SeriesBucketStartMs(tsMs, intervalMs),
				"interval=%d ts=%d", intervalMs, tsMs)
		}
	}
	// interval 이 0 이하이면 정렬하지 않고 원값을 돌려준다(0 나눗셈 방어).
	assert.Equal(t, int64(123), SeriesBucketStartMs(123, 0))
}

// --- AC-23: 식별자 이스케이프 ---

func TestBuildFluxSeriesQuery_Escaping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "큰따옴표", raw: `a"b`, want: `a\"b`},
		{name: "백슬래시", raw: `a\b`, want: `a\\b`},
		{name: "세미콜론", raw: `a;b`, want: `a;b`},
		{name: "유니코드", raw: "온도_℃", want: "온도_℃"},
		{name: "문자열보간", raw: "a${x}b", want: `a\${x}b`},
		{name: "개행", raw: "a\nb", wantErr: true},
		{name: "탭", raw: "a\tb", wantErr: true},
		{name: "NUL", raw: "a\x00b", wantErr: true},
		{name: "깨진UTF8", raw: "a\xffb", wantErr: true},
	}
	for _, tc := range cases {
		t.Run("measurement/"+tc.name, func(t *testing.T) {
			spec := baseSpec()
			spec.Measurement = tc.raw
			got, err := BuildFluxSeriesQuery(spec)
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrUnescapableIdentifier))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, got, `r._measurement == "`+tc.want+`"`)
		})
		t.Run("tagvalue/"+tc.name, func(t *testing.T) {
			spec := baseSpec()
			spec.Tags = map[string]string{"host": tc.raw}
			got, err := BuildFluxSeriesQuery(spec)
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrUnescapableIdentifier))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, got, `r["host"] == "`+tc.want+`"`)
		})
	}
}

func TestBuildInfluxQLSeriesQuery_Escaping(t *testing.T) {
	t.Parallel()
	identCases := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "큰따옴표", raw: `a"b`, want: `a\"b`},
		{name: "백슬래시", raw: `a\b`, want: `a\\b`},
		{name: "세미콜론", raw: `a;b`, want: `a;b`},
		{name: "유니코드", raw: "온도_℃", want: "온도_℃"},
		{name: "개행", raw: "a\nb", wantErr: true},
		{name: "깨진UTF8", raw: "a\xffb", wantErr: true},
	}
	for _, tc := range identCases {
		t.Run("measurement/"+tc.name, func(t *testing.T) {
			spec := baseSpec()
			spec.Measurement = tc.raw
			got, err := BuildInfluxQLSeriesQuery(spec)
			if tc.wantErr {
				require.Error(t, err)
				assert.True(t, errors.Is(err, ErrUnescapableIdentifier))
				return
			}
			require.NoError(t, err)
			assert.Contains(t, got, `FROM "`+tc.want+`"`)
		})
	}

	// 태그 값은 작은따옴표 문자열 리터럴이므로 이스케이프 대상이 다르다.
	literalCases := []struct {
		name string
		raw  string
		want string
	}{
		{name: "작은따옴표", raw: `a'b`, want: `a\'b`},
		{name: "백슬래시", raw: `a\b`, want: `a\\b`},
		{name: "세미콜론", raw: `a;b`, want: `a;b`},
	}
	for _, tc := range literalCases {
		t.Run("tagvalue/"+tc.name, func(t *testing.T) {
			spec := baseSpec()
			spec.Tags = map[string]string{"host": tc.raw}
			got, err := BuildInfluxQLSeriesQuery(spec)
			require.NoError(t, err)
			assert.Contains(t, got, `AND "host" = '`+tc.want+`'`)
		})
	}
}

func TestSeriesQuerySpec_ValidateIdentifiers_CoversEveryUserSuppliedAxis(t *testing.T) {
	t.Parallel()
	bad := "a\nb"
	cases := map[string]func(*SeriesQuerySpec){
		"bucket":      func(s *SeriesQuerySpec) { s.Bucket = bad },
		"measurement": func(s *SeriesQuerySpec) { s.Measurement = bad },
		"field":       func(s *SeriesQuerySpec) { s.Field = bad },
		"tag key":     func(s *SeriesQuerySpec) { s.Tags = map[string]string{bad: "v"} },
		"tag value":   func(s *SeriesQuerySpec) { s.Tags = map[string]string{"k": bad} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := baseSpec()
			mutate(&spec)
			err := spec.ValidateIdentifiers()
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUnescapableIdentifier))
			assert.Contains(t, err.Error(), name)
		})
	}
}

// --- 요청 형상 검증 ---

func TestBuildSeriesQuery_ShapeValidation(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		mutate func(*SeriesQuerySpec)
		msg    string
	}{
		{"measurement 빈 문자열", func(s *SeriesQuerySpec) { s.Measurement = "" }, "measurement is required"},
		{"field 빈 문자열", func(s *SeriesQuerySpec) { s.Field = "" }, "field is required"},
		{"interval 0", func(s *SeriesQuerySpec) { s.IntervalMs = 0 }, "interval_ms must be > 0"},
		{"interval 음수", func(s *SeriesQuerySpec) { s.IntervalMs = -1 }, "interval_ms must be > 0"},
		{"end == start", func(s *SeriesQuerySpec) { s.EndMs = s.StartMs }, "end_ms must be greater than start_ms"},
		{"end < start", func(s *SeriesQuerySpec) { s.EndMs = s.StartMs - 1 }, "end_ms must be greater than start_ms"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			spec := baseSpec()
			tc.mutate(&spec)

			_, err := BuildFluxSeriesQuery(spec)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.msg)

			_, err = BuildInfluxQLSeriesQuery(spec)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.msg)
		})
	}
}

func TestBuildFluxSeriesQuery_BucketRequired(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Bucket = ""
	_, err := BuildFluxSeriesQuery(spec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bucket is required")

	// InfluxQL 은 database 가 클라이언트 연결에 묶여 있어 bucket 을 요구하지 않는다.
	_, err = BuildInfluxQLSeriesQuery(spec)
	assert.NoError(t, err)
}

// --- 전수 조합: (v2/v3) × 집계 5 × fill 5 × 태그 0/1/N ---

func TestBuildSeriesQuery_Exhaustive(t *testing.T) {
	t.Parallel()
	type builder struct {
		name string
		fn   func(SeriesQuerySpec) (string, error)
	}
	builders := []builder{
		{"v2-flux", BuildFluxSeriesQuery},
		{"v3-influxql", BuildInfluxQLSeriesQuery},
	}

	combos := 0
	for _, b := range builders {
		for _, agg := range allAggregations {
			for _, fill := range allFills {
				for tagName, tags := range tagCases {
					combos++
					name := fmt.Sprintf("%s/%d/%d/%s", b.name, agg, fill, tagName)
					t.Run(name, func(t *testing.T) {
						spec := baseSpec()
						spec.Aggregation = agg
						spec.Fill = fill
						spec.Tags = tags

						got, err := b.fn(spec)
						if fill == SeriesFillAvg {
							require.Error(t, err, "미지원 fill 은 양쪽 백엔드 모두에서 거부된다")
							assert.True(t, errors.Is(err, ErrUnsupportedSeriesFill))
							return
						}
						require.NoError(t, err)

						// 태그 필터 줄 수 = 태그 수.
						assert.Equal(t, len(tags), countTagPredicates(b.name, got), got)
						// 생성 결과에 요청 어휘가 그대로 새어나가서는 안 된다.
						assert.NotContains(t, got, "fn: average")
						assert.NotContains(t, got, "SELECT AVG(")
					})
				}
			}
		}
	}
	// 백엔드 2 × 집계 7 × fill 5 × 태그 형상 3.
	assert.Equal(t, 2*len(allAggregations)*len(allFills)*len(tagCases), combos, "전수 조합 개수")
}

// countTagPredicates 는 생성된 쿼리에서 태그 술어 줄 수를 센다.
func countTagPredicates(builderName, query string) int {
	n := 0
	for _, line := range strings.Split(query, "\n") {
		if builderName == "v2-flux" {
			// measurement/field 필터는 r._ 로 시작하므로 태그 필터와 구분된다.
			if strings.Contains(line, "|> filter(fn: (r) => r[") {
				n++
			}
			continue
		}
		if strings.HasPrefix(line, `   AND "`) {
			n++
		}
	}
	return n
}

// ===== group by (SPEC-TSDB-004 §2.2 ~ §2.4) =====

// TestBuildFluxSeriesQuery_GroupBy_Golden 은 v2 그룹 질의의 전체 형상을 고정한다(AC-04).
//
// 세 가지가 동시에 성립해야 한다 — group 파이프가 존재하고, 그것이
// aggregateWindow **앞**에 있고(§4.2), keep 목록이 그룹 키를 남긴다.
func TestBuildFluxSeriesQuery_GroupBy_Golden(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Tags = map[string]string{"region": "kr"}
	spec.GroupBy = []string{"rack", "host"} // 정렬 전 순서로 넣는다

	got, err := BuildFluxSeriesQuery(spec)
	require.NoError(t, err)

	want := strings.Join([]string{
		`from(bucket: "metrics")`,
		`  |> range(start: time(v: 1700000000000000000), stop: time(v: 1700003600000000000))`,
		`  |> filter(fn: (r) => r._measurement == "cpu")`,
		`  |> filter(fn: (r) => r._field == "usage")`,
		`  |> filter(fn: (r) => r["region"] == "kr")`,
		`  |> group(columns: ["host", "rack"])`,
		`  |> aggregateWindow(every: 60000ms, fn: mean, createEmpty: false, timeSrc: "_start")`,
		`  |> keep(columns: ["_time", "_value", "host", "rack"])`,
	}, "\n")
	assert.Equal(t, want, got)
}

// TestBuildFluxSeriesQuery_GroupPipePrecedesAggregateWindow 는 §4.2 의 순서
// 계약을 인덱스로 직접 고정한다.
//
// 골든 테스트만으로는 부족하다 — 템플릿 문자열이 함께 바뀌면 골든도 같이
// 갱신되어 순서 역전이 통과할 수 있다. 순서는 독립 단언으로 남긴다.
func TestBuildFluxSeriesQuery_GroupPipePrecedesAggregateWindow(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.GroupBy = []string{"host2"}

	got, err := BuildFluxSeriesQuery(spec)
	require.NoError(t, err)

	groupAt := strings.Index(got, "|> group(columns:")
	aggAt := strings.Index(got, "|> aggregateWindow(")
	keepAt := strings.Index(got, "|> keep(columns:")
	require.Positive(t, groupAt, "group 파이프가 존재해야 한다")
	require.Positive(t, aggAt)
	assert.Less(t, groupAt, aggAt, "group 은 aggregateWindow 앞에 와야 한다(§4.2)")
	assert.Less(t, aggAt, keepAt, "keep 은 마지막이다")
}

// TestBuildInfluxQLSeriesQuery_GroupBy_Golden 은 v3 그룹 절의 형상을 고정한다(AC-05).
func TestBuildInfluxQLSeriesQuery_GroupBy_Golden(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Tags = map[string]string{"region": "kr"}
	spec.GroupBy = []string{"rack", "host"}

	got, err := BuildInfluxQLSeriesQuery(spec)
	require.NoError(t, err)

	assert.Contains(t, got, ` GROUP BY time(60000ms), "host", "rack" FILL(none)`)
}

// TestBuildInfluxQLSeriesQuery_GroupBy_TimeStaysFirst 는 버킷 경계 계약을
// 지킨다(AC-05 · AC-08). time(d) 가 첫 자리이고 offset 인자가 없어야 한다.
//
// 이 두 성질이 SPEC-TSDB-002 §2.8 의 epoch 정렬을 성립시킨다. 그룹 키 추가가
// 경계를 흔들지 않는다는 것이 본 SPEC 이 지켜야 할 불변식이다.
func TestBuildInfluxQLSeriesQuery_GroupBy_TimeStaysFirst(t *testing.T) {
	t.Parallel()
	for _, groupBy := range [][]string{nil, {"host2"}, {"host2", "rack"}} {
		spec := baseSpec()
		spec.GroupBy = groupBy

		got, err := BuildInfluxQLSeriesQuery(spec)
		require.NoError(t, err)

		gb := got[strings.Index(got, " GROUP BY "):]
		assert.True(t, strings.HasPrefix(gb, ` GROUP BY time(`),
			"time() 이 GROUP BY 첫 자리여야 한다: %q", gb)
		// offset 인자는 `time(d, off)` 형태로 나타난다. 쉼표가 time(...) **안**에
		// 있으면 안 된다 — 그룹 키 쉼표는 닫는 괄호 뒤에 온다.
		timeArgs := gb[len(` GROUP BY time(`):strings.Index(gb, ")")]
		assert.NotContains(t, timeArgs, ",", "time() 에 offset 인자가 없어야 한다")
	}
}

// TestBuildSeriesQuery_GroupByKeysAreSortedAndDeduped 는 결정성을 고정한다(AC-03).
func TestBuildSeriesQuery_GroupByKeysAreSortedAndDeduped(t *testing.T) {
	t.Parallel()

	shuffled := baseSpec()
	shuffled.Tags = nil
	shuffled.GroupBy = []string{"rack", "host2", "az", "host2"} // 역순 + 중복

	sorted := baseSpec()
	sorted.Tags = nil
	sorted.GroupBy = []string{"az", "host2", "rack"}

	for _, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux":     BuildFluxSeriesQuery,
		"influxql": BuildInfluxQLSeriesQuery,
	} {
		a, err := build(shuffled)
		require.NoError(t, err)
		b, err := build(sorted)
		require.NoError(t, err)
		assert.Equal(t, b, a, "입력 순서와 중복이 생성 결과에 영향을 주면 안 된다")

		// 반복 호출에도 동일해야 한다(맵이 아니라 슬라이스지만 회귀 방지).
		for i := 0; i < 10; i++ {
			again, err := build(shuffled)
			require.NoError(t, err)
			require.Equal(t, a, again)
		}
	}
}

// TestBuildSeriesQuery_GroupByEmptyIsByteIdentical 은 §2.9 U9 를 고정한다(AC-06).
//
// nil 과 빈 슬라이스가 서로 같을 뿐 아니라, **그룹 축을 아예 설정하지 않은**
// spec 과도 바이트 단위로 같아야 한다. 저장된 config 의 렌더 불변이 여기에 걸린다.
func TestBuildSeriesQuery_GroupByEmptyIsByteIdentical(t *testing.T) {
	t.Parallel()

	unset := baseSpec()
	nilGroup := baseSpec()
	nilGroup.GroupBy = nil
	emptyGroup := baseSpec()
	emptyGroup.GroupBy = []string{}

	for name, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux":     BuildFluxSeriesQuery,
		"influxql": BuildInfluxQLSeriesQuery,
	} {
		base, err := build(unset)
		require.NoError(t, err, name)

		gotNil, err := build(nilGroup)
		require.NoError(t, err, name)
		assert.Equal(t, base, gotNil, "%s: nil GroupBy 는 미설정과 같아야 한다", name)

		gotEmpty, err := build(emptyGroup)
		require.NoError(t, err, name)
		assert.Equal(t, base, gotEmpty, "%s: 빈 GroupBy 는 미설정과 같아야 한다", name)

		// 그룹 관련 토큰이 하나도 새지 않아야 한다.
		assert.NotContains(t, base, "group(columns:", "%s", name)
	}
}

// TestBuildSeriesQuery_GroupByConflictsWithTagFilter 는 UB1-3 을 고정한다(AC-14).
func TestBuildSeriesQuery_GroupByConflictsWithTagFilter(t *testing.T) {
	t.Parallel()

	spec := baseSpec()
	spec.Tags = map[string]string{"host": "a", "region": "kr"}
	spec.GroupBy = []string{"host"} // Tags 와 충돌

	for name, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux":     BuildFluxSeriesQuery,
		"influxql": BuildInfluxQLSeriesQuery,
	} {
		_, err := build(spec)
		require.Error(t, err, name)
		assert.ErrorIs(t, err, ErrGroupByConflictsWithTagFilter, name)
		assert.Contains(t, err.Error(), "host", "%s: 충돌한 키를 알려 줘야 한다", name)
	}

	// 충돌하지 않는 조합은 통과한다.
	ok := baseSpec()
	ok.Tags = map[string]string{"region": "kr"}
	ok.GroupBy = []string{"host2"}
	_, err := BuildFluxSeriesQuery(ok)
	assert.NoError(t, err)
}

// TestBuildSeriesQuery_GroupByIdentifierValidation 은 그룹 키가 태그 키와 같은
// 식별자 검증을 받는지 고정한다. 검증을 건너뛰면 제어문자가 쿼리에 삽입된다.
func TestBuildSeriesQuery_GroupByIdentifierValidation(t *testing.T) {
	t.Parallel()

	spec := baseSpec()
	spec.Tags = nil
	spec.GroupBy = []string{"bad\nkey"}

	for name, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux":     BuildFluxSeriesQuery,
		"influxql": BuildInfluxQLSeriesQuery,
	} {
		_, err := build(spec)
		require.Error(t, err, name)
		assert.ErrorIs(t, err, ErrUnescapableIdentifier, name)
		assert.Contains(t, err.Error(), "group key", "%s", name)
	}
}

// ===== group_filter — 시리즈축 페이지네이션 (SPEC-TSDB-004 §2.7.1) =====

// TestBuildSeriesQuery_GroupFilter_Golden 은 페이지 선택 술어의 형상을 고정한다.
//
// 조합 안은 and, 조합 사이는 or 다. 이 결합을 뒤집으면 술어가 항상 참이 되어
// 페이지 선택이 조용히 무효화된다.
func TestBuildSeriesQuery_GroupFilter_Golden(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Tags = nil
	spec.GroupBy = []string{"host2"}
	spec.GroupFilter = []map[string]string{{"host2": "b"}, {"host2": "a"}} // 역순 입력

	flux, err := BuildFluxSeriesQuery(spec)
	require.NoError(t, err)
	assert.Contains(t, flux, `  |> filter(fn: (r) => (r["host2"] == "a") or (r["host2"] == "b"))`)
	// 술어는 group() **앞**에 온다 — 걸러낸 뒤 나눠야 불필요한 테이블이 안 생긴다.
	assert.Less(t, strings.Index(flux, `(r["host2"] == "a")`), strings.Index(flux, "|> group(columns:"))

	iql, err := BuildInfluxQLSeriesQuery(spec)
	require.NoError(t, err)
	assert.Contains(t, iql, `   AND (("host2" = 'a') OR ("host2" = 'b'))`)
}

// TestBuildSeriesQuery_GroupFilter_다중키조합 은 조합 술어를 고정한다.
// 키별 허용값 맵이었다면 데카르트 곱이 되어 (a,r2)·(b,r1) 까지 선택했을 것이다.
func TestBuildSeriesQuery_GroupFilter_다중키조합(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Tags = nil
	spec.GroupBy = []string{"host2", "rack"}
	spec.GroupFilter = []map[string]string{
		{"host2": "a", "rack": "r1"},
		{"host2": "b", "rack": "r2"},
	}

	flux, err := BuildFluxSeriesQuery(spec)
	require.NoError(t, err)
	assert.Contains(t, flux, `(r["host2"] == "a" and r["rack"] == "r1")`)
	assert.Contains(t, flux, `(r["host2"] == "b" and r["rack"] == "r2")`)
	// 실재하지 않는 조합은 술어에 없다.
	assert.NotContains(t, flux, `(r["host2"] == "a" and r["rack"] == "r2")`)

	iql, err := BuildInfluxQLSeriesQuery(spec)
	require.NoError(t, err)
	assert.Contains(t, iql, `("host2" = 'a' AND "rack" = 'r1')`)
	assert.NotContains(t, iql, `("host2" = 'a' AND "rack" = 'r2')`)
}

// TestBuildSeriesQuery_GroupFilter_결정성 은 입력 순서가 결과를 바꾸지 않음을
// 고정한다. 같은 페이지 요청이 다른 쿼리 문자열을 만들면 캐시·로그 대조가 어긋난다.
func TestBuildSeriesQuery_GroupFilter_결정성(t *testing.T) {
	t.Parallel()
	mk := func(combos []map[string]string) SeriesQuerySpec {
		sp := baseSpec()
		sp.Tags = nil
		sp.GroupBy = []string{"host2"}
		sp.GroupFilter = combos
		return sp
	}
	a := mk([]map[string]string{{"host2": "c"}, {"host2": "a"}, {"host2": "b"}})
	b := mk([]map[string]string{{"host2": "a"}, {"host2": "b"}, {"host2": "c"}})

	for name, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux": BuildFluxSeriesQuery, "influxql": BuildInfluxQLSeriesQuery,
	} {
		qa, err := build(a)
		require.NoError(t, err, name)
		qb, err := build(b)
		require.NoError(t, err, name)
		assert.Equal(t, qb, qa, "%s: 입력 순서가 결과를 바꾸면 안 된다", name)
	}
}

// TestBuildSeriesQuery_GroupFilter_빈경우_바이트무변경 은 §2.9 U9 를 고정한다.
func TestBuildSeriesQuery_GroupFilter_빈경우_바이트무변경(t *testing.T) {
	t.Parallel()
	unset := baseSpec()
	unset.GroupBy = []string{"host2"}
	nilFilter := unset
	nilFilter.GroupFilter = nil
	emptyFilter := unset
	emptyFilter.GroupFilter = []map[string]string{}

	for name, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux": BuildFluxSeriesQuery, "influxql": BuildInfluxQLSeriesQuery,
	} {
		base, err := build(unset)
		require.NoError(t, err, name)
		for label, sp := range map[string]SeriesQuerySpec{"nil": nilFilter, "empty": emptyFilter} {
			got, err := build(sp)
			require.NoError(t, err, name)
			assert.Equal(t, base, got, "%s/%s", name, label)
		}
	}
}

// TestBuildSeriesQuery_GroupFilter_빈조합거부 는 §2.7.1 을 고정한다.
// 빈 조합은 "모든 그룹" 이 되어 페이지 선택을 조용히 무효화한다.
func TestBuildSeriesQuery_GroupFilter_빈조합거부(t *testing.T) {
	t.Parallel()
	spec := baseSpec()
	spec.Tags = nil
	spec.GroupBy = []string{"host2"}
	spec.GroupFilter = []map[string]string{{"host2": "a"}, {}}

	for name, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux": BuildFluxSeriesQuery, "influxql": BuildInfluxQLSeriesQuery,
	} {
		_, err := build(spec)
		require.Error(t, err, name)
		assert.ErrorIs(t, err, ErrEmptyGroupFilterEntry, name)
	}
}

// TestBuildSeriesQuery_GroupFilter_버킷경계불변 은 AC-08 을 페이지 축으로
// 확장한다. 페이지가 달라도 윈도우 파라미터는 같아야 한다 — 시리즈축을 나누는
// 것이지 시간축을 나누는 것이 아니다(UB1-10).
func TestBuildSeriesQuery_GroupFilter_버킷경계불변(t *testing.T) {
	t.Parallel()
	windowOf := func(q string) string {
		i := strings.Index(q, "aggregateWindow(")
		if i < 0 {
			i = strings.Index(q, "GROUP BY time(")
		}
		require.Positive(t, i)
		return q[i:]
	}
	page := func(vals ...string) SeriesQuerySpec {
		sp := baseSpec()
		sp.Tags = nil
		sp.GroupBy = []string{"host2"}
		for _, v := range vals {
			sp.GroupFilter = append(sp.GroupFilter, map[string]string{"host2": v})
		}
		return sp
	}
	for name, build := range map[string]func(SeriesQuerySpec) (string, error){
		"flux": BuildFluxSeriesQuery, "influxql": BuildInfluxQLSeriesQuery,
	} {
		p1, err := build(page("a", "b"))
		require.NoError(t, err, name)
		p2, err := build(page("c", "d"))
		require.NoError(t, err, name)
		all, err := build(page())
		require.NoError(t, err, name)
		assert.Equal(t, windowOf(all), windowOf(p1), "%s: 페이지1 윈도우가 전체와 같아야 한다", name)
		assert.Equal(t, windowOf(p1), windowOf(p2), "%s: 페이지 간 윈도우가 같아야 한다", name)
	}
}
