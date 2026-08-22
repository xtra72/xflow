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

// allAggregations 는 §2.7 매핑 표의 5 종 전부다.
var allAggregations = []SeriesAggregation{
	SeriesAggMin, SeriesAggMax, SeriesAggAverage, SeriesAggFirst, SeriesAggLast,
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

// --- AC-25: 집계 어휘 매핑 5 종 전수 ---

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
	}
	require.Len(t, cases, len(allAggregations), "5 종 전수여야 한다")

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
			name:         "previous",
			fill:         SeriesFillPrevious,
			fluxContains: []string{"createEmpty: true", "|> fill(usePrevious: true)"},
			influxQL:     "FILL(previous)",
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
	assert.Equal(t, 2*5*5*3, combos, "전수 조합 개수")
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
