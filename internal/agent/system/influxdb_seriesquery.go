// @spec SPEC-TSDB-002 §2.7 (U7) · §2.8 (U8)
//
// 구조화 시리즈 질의의 쿼리 생성 계층이다. 네트워크를 타지 않는 순수 문자열
// 조립만 담당하며, 실행은 influxdb_agent.go 의 QuerySeriesBuckets 가 맡는다.
// 둘을 분리하는 이유는 조합 폭발이다 — (v2/v3) × (집계 5) × (fill 5) × (태그 0/1/N)
// 은 150 조합이고, 실행과 섞으면 그 전수를 mock HTTP 로 돌려야 한다.
package system

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

// SeriesAggregation 은 구조화 시리즈 질의의 집계 연산이다.
//
// 값이 정수 열거형인 것은 의도적이다. HTTP 요청의 집계 어휘(와이어 어휘)는 API
// 계층이 소유하고, 이 패키지는 도메인 열거형만 다룬다. 이렇게 나누면 §2.7 의
// 백엔드별 함수 이름 매핑이 와이어 어휘와 섞이지 않는다.
type SeriesAggregation int

const (
	// seriesAggUnspecified 는 zero value 이며 항상 오류로 거부된다.
	// 열거형의 0 번을 유효한 집계에 배정하면 미지정 요청이 조용히 그 집계로
	// 처리된다.
	seriesAggUnspecified SeriesAggregation = iota
	// SeriesAggMin 은 구간 최솟값이다.
	SeriesAggMin
	// SeriesAggMax 는 구간 최댓값이다.
	SeriesAggMax
	// SeriesAggAverage 는 구간 산술평균이다.
	SeriesAggAverage
	// SeriesAggFirst 는 구간의 첫 값이다.
	SeriesAggFirst
	// SeriesAggLast 는 구간의 마지막 값이다.
	SeriesAggLast
)

// SeriesFill 은 빈 버킷 채우기 전략이다.
type SeriesFill int

const (
	// SeriesFillNone 은 빈 버킷을 방출하지 않는다(요청의 빈 문자열에 대응).
	// zero value 를 여기에 두는 것은 옳다 — 미지정의 뜻이 곧 "채우지 않음"이다.
	SeriesFillNone SeriesFill = iota
	// SeriesFillNull 은 빈 버킷을 null 값으로 방출한다.
	SeriesFillNull
	// SeriesFillZero 는 빈 버킷을 0 으로 방출한다.
	SeriesFillZero
	// SeriesFillPrevious 는 빈 버킷을 직전 값으로 방출한다.
	SeriesFillPrevious
	// SeriesFillAvg 는 InfluxDB 양쪽 백엔드 모두 대응물이 없어 항상 거부된다.
	// 열거형에 남겨 두는 이유는 프런트 계약(SeriesMatrixQuery.fill)에 이 값이
	// 존재하기 때문이며, 조용히 다른 전략으로 대체하지 않기 위함이다(§2.7).
	SeriesFillAvg
)

// ErrUnsupportedSeriesFill 은 InfluxDB 가 대응물을 갖지 않는 fill 전략이다.
// HTTP 계층은 이를 400 으로 매핑한다.
var ErrUnsupportedSeriesFill = errors.New("influxdb: fill strategy is not supported by influxdb backends")

// ErrUnescapableIdentifier 는 쿼리 문자열에 안전하게 삽입할 수 없는 식별자다.
// HTTP 계층은 이를 400 으로 매핑한다(§2.7).
var ErrUnescapableIdentifier = errors.New("influxdb: identifier cannot be safely escaped")

// SeriesQuerySpec 는 구조화 시리즈 질의 1건의 입력이다.
// 요청 1건이 시리즈 1개를 처리하므로(§2.6) measurement/field 는 단수다.
type SeriesQuerySpec struct {
	// Bucket 은 v2 의 bucket, v3 의 database 이다. 빈 값이면 에이전트 기본값을
	// 쓰며, 그 채움은 QuerySeriesBuckets 가 수행한다.
	Bucket string
	// Measurement 는 시리즈 키다(중립 어휘 series_key 에 대응).
	Measurement string
	// Field 는 값 필드다. TSDB 소스에서 필수이며 폴백을 두지 않는다(§2.16 #4).
	Field string
	// Tags 는 시리즈 태그 필터다. 생성 순서는 키 오름차순으로 고정한다.
	Tags map[string]string
	// StartMs 는 조회 시작(포함)이다.
	StartMs int64
	// EndMs 는 조회 끝(미포함)이다.
	EndMs int64
	// IntervalMs 는 버킷 폭이며 0 보다 커야 한다.
	IntervalMs int64
	// Aggregation 은 버킷 집계 연산이다.
	Aggregation SeriesAggregation
	// Fill 은 빈 버킷 채우기 전략이다.
	Fill SeriesFill
}

// --- 집계 어휘 매핑 (§2.7 정본 표) ---
//
// 5 종을 명시적으로 나열하고 default 에서 오류를 반환한다. map 리터럴로 두면
// 항목을 빠뜨려도 컴파일되기 때문이다(§4.6). Go 에는 exhaustive switch 강제가
// 없으므로 전수성은 테스트가 함께 검증한다.

// fluxAggregationFn 은 집계 연산을 Flux 함수 이름으로 매핑한다.
func fluxAggregationFn(agg SeriesAggregation) (string, error) {
	switch agg {
	case SeriesAggMin:
		return "min", nil
	case SeriesAggMax:
		return "max", nil
	case SeriesAggAverage:
		// Flux 의 산술평균 함수 이름은 mean 이다. 매핑 표에서 유일하게 이름이
		// 어긋나는 항목이며, 빠뜨리면 존재하지 않는 함수로 쿼리가 만들어진다.
		return "mean", nil
	case SeriesAggFirst:
		return "first", nil
	case SeriesAggLast:
		return "last", nil
	default:
		return "", fmt.Errorf("influxdb: unsupported series aggregation (%d)", int(agg))
	}
}

// influxQLAggregationFn 은 집계 연산을 InfluxQL 함수 이름으로 매핑한다.
func influxQLAggregationFn(agg SeriesAggregation) (string, error) {
	switch agg {
	case SeriesAggMin:
		return "MIN", nil
	case SeriesAggMax:
		return "MAX", nil
	case SeriesAggAverage:
		// InfluxQL 의 산술평균 함수 이름도 MEAN 이다. 다른 SQL 방언의 습관대로
		// 적으면 파서 오류가 난다.
		return "MEAN", nil
	case SeriesAggFirst:
		return "FIRST", nil
	case SeriesAggLast:
		return "LAST", nil
	default:
		return "", fmt.Errorf("influxdb: unsupported series aggregation (%d)", int(agg))
	}
}

// --- fill 매핑 (§2.7) ---

// fluxFillOptions 는 fill 전략을 aggregateWindow 의 createEmpty 인자와
// 후처리 파이프 한 줄로 매핑한다. postPipe 가 빈 문자열이면 후처리가 없다.
func fluxFillOptions(fill SeriesFill) (createEmpty bool, postPipe string, err error) {
	switch fill {
	case SeriesFillNone:
		return false, "", nil
	case SeriesFillNull:
		return true, "", nil
	case SeriesFillZero:
		return true, fluxFillZeroPipe, nil
	case SeriesFillPrevious:
		return true, fluxFillPreviousPipe, nil
	case SeriesFillAvg:
		return false, "", ErrUnsupportedSeriesFill
	default:
		return false, "", fmt.Errorf("influxdb: unsupported series fill (%d)", int(fill))
	}
}

// influxQLFillArg 는 fill 전략을 InfluxQL FILL(...) 의 인자로 매핑한다.
func influxQLFillArg(fill SeriesFill) (string, error) {
	switch fill {
	case SeriesFillNone:
		return "none", nil
	case SeriesFillNull:
		return "null", nil
	case SeriesFillZero:
		return "0", nil
	case SeriesFillPrevious:
		return "previous", nil
	case SeriesFillAvg:
		return "", ErrUnsupportedSeriesFill
	default:
		return "", fmt.Errorf("influxdb: unsupported series fill (%d)", int(fill))
	}
}

// --- 식별자 검증과 이스케이프 (§2.7) ---

// validateSeriesIdentifier 는 값이 쿼리 문자열에 삽입 가능한지 검사한다.
//
// 거부 대상:
//   - 유효하지 않은 UTF-8 — 생성된 쿼리 자체가 깨진다.
//   - 제어 문자(개행 · 탭 · NUL 등) — Flux 문자열 리터럴과 InfluxQL 식별자는
//     모두 한 줄 안에 놓이므로 개행이 끼면 구문이 갈라진다. 이스케이프 시퀀스로
//     표현할 수 있는 경우에도 거부한다. 시계열 식별자에 제어 문자가 정당하게
//     들어갈 이유가 없고, 허용하면 검증 표면만 넓어진다.
func validateSeriesIdentifier(what, s string) error {
	if !utf8.ValidString(s) {
		return fmt.Errorf("%w: %s is not valid UTF-8", ErrUnescapableIdentifier, what)
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%w: %s contains control character U+%04X", ErrUnescapableIdentifier, what, r)
		}
	}
	return nil
}

// ValidateIdentifiers 는 spec 의 모든 사용자 유래 식별자를 검증한다.
// HTTP 핸들러가 질의 전에 호출해 400 으로 거부할 수 있게 공개한다.
func (s SeriesQuerySpec) ValidateIdentifiers() error {
	if err := validateSeriesIdentifier("bucket", s.Bucket); err != nil {
		return err
	}
	if err := validateSeriesIdentifier("measurement", s.Measurement); err != nil {
		return err
	}
	if err := validateSeriesIdentifier("field", s.Field); err != nil {
		return err
	}
	for _, k := range sortedTagKeys(s.Tags) {
		if err := validateSeriesIdentifier("tag key", k); err != nil {
			return err
		}
		if err := validateSeriesIdentifier("tag value", s.Tags[k]); err != nil {
			return err
		}
	}
	return nil
}

// validateShape 는 식별자 내용과 무관한 요청 형상을 검사한다.
func (s SeriesQuerySpec) validateShape() error {
	if s.Measurement == "" {
		return errors.New("influxdb: measurement is required")
	}
	if s.Field == "" {
		return errors.New("influxdb: field is required")
	}
	if s.IntervalMs <= 0 {
		return errors.New("influxdb: interval_ms must be > 0")
	}
	if s.EndMs <= s.StartMs {
		return errors.New("influxdb: end_ms must be greater than start_ms")
	}
	return nil
}

// escapeFluxStringLiteral 은 값을 Flux 큰따옴표 문자열 리터럴 본문으로 변환한다.
//
// 호출 전에 validateSeriesIdentifier 를 통과했다고 가정한다(제어 문자 없음).
// `${` 를 함께 이스케이프하는 이유는 Flux 문자열이 그 형태를 보간(interpolation)
// 으로 해석하기 때문이다 — 이스케이프하지 않으면 사용자 데이터가 식이 된다.
func escapeFluxStringLiteral(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 8)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '$':
			if i+1 < len(s) && s[i+1] == '{' {
				b.WriteString(`\${`)
				i++ // '{' 까지 소비한다.
				continue
			}
			b.WriteByte('$')
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// escapeInfluxQLIdent 는 값을 InfluxQL 큰따옴표 식별자 본문으로 변환한다.
// measurement · field · tag key 가 대상이다.
//
// internal/migrate/tsdbtags/client_v3.go 에 같은 뜻의 함수가 있으나 그 패키지를
// import 하지 않는다(§4.5) — 마이그레이션 도구의 좁은 인터페이스 보장을 API
// 계층의 필요가 넓히지 않게 하기 위함이다. 대신 백슬래시까지 이스케이프한다.
func escapeInfluxQLIdent(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

// escapeInfluxQLStringLiteral 은 값을 InfluxQL 작은따옴표 문자열 리터럴 본문으로
// 변환한다. tag value 가 대상이다.
func escapeInfluxQLStringLiteral(s string) string {
	return strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(s)
}

// sortedTagKeys 는 태그 키를 오름차순으로 반환한다.
// 맵 순회 순서는 비결정적이므로 생성 결과를 고정하려면 정렬이 필요하다.
func sortedTagKeys(tags map[string]string) []string {
	if len(tags) == 0 {
		return nil
	}
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// seriesDurationLiteral 은 밀리초를 Flux/InfluxQL 공통 duration 리터럴로 만든다.
// 두 방언 모두 ms 단위를 인식하므로 단위 변환 없이 그대로 쓴다.
func seriesDurationLiteral(ms int64) string {
	return fmt.Sprintf("%dms", ms)
}

// --- Flux 생성 (§2.7 v2 템플릿) ---

// 템플릿 조각을 raw string 으로 두는 이유는 생성될 쿼리 문자열이 소스에 그대로
// 보이게 하기 위함이다. 특히 timeSrc 인자는 AC-26 이 소스를 직접 검사한다.
const (
	fluxFromTmpl        = `from(bucket: "%s")` + "\n"
	fluxRangeTmpl       = `  |> range(start: time(v: %d), stop: time(v: %d))` + "\n"
	fluxMeasurementTmpl = `  |> filter(fn: (r) => r._measurement == "%s")` + "\n"
	fluxFieldTmpl       = `  |> filter(fn: (r) => r._field == "%s")` + "\n"
	fluxTagTmpl         = `  |> filter(fn: (r) => r["%s"] == "%s")` + "\n"

	// timeSrc 인자를 명시하는 것은 선택 사항이 아니다. aggregateWindow 는
	// 기본적으로 버킷 끝(_stop 컬럼)을 _time 으로 방출한다.
	// chartQueryEntry.timestamp 는 버킷 시작이어야 하므로(§2.8) 기본값에
	// 맡기면 모든 값이 한 인터벌만큼 미래로 밀린다. 증상이 "값이 틀리다"가
	// 아니라 "값이 약간 늦는다"이므로 리뷰에서 잡히지 않는다.
	fluxAggregateWindowTmpl = `  |> aggregateWindow(every: %s, fn: %s, createEmpty: %t, timeSrc: "_start")` + "\n"

	fluxFillZeroPipe     = `  |> fill(value: 0.0)` + "\n"
	fluxFillPreviousPipe = `  |> fill(usePrevious: true)` + "\n"

	fluxKeepTmpl = `  |> keep(columns: ["_time", "_value"])`
)

// nsPerMs 는 epoch ms → epoch ns 변환 계수이다.
const nsPerMs = int64(1_000_000)

// BuildFluxSeriesQuery 는 InfluxDB 2.x 용 Flux 쿼리를 생성한다(§2.7).
//
// 순수 함수다 — 네트워크도, 시각도, 전역 상태도 읽지 않는다.
func BuildFluxSeriesQuery(spec SeriesQuerySpec) (string, error) {
	if err := spec.validateShape(); err != nil {
		return "", err
	}
	if spec.Bucket == "" {
		// Flux 의 from() 은 bucket 을 구조적으로 요구한다. 에이전트 기본값 채움은
		// 호출자(QuerySeriesBuckets)의 책임이며, 여기까지 빈 값이 왔다면 버그다.
		return "", errors.New("influxdb: bucket is required for flux queries")
	}
	if err := spec.ValidateIdentifiers(); err != nil {
		return "", err
	}
	fn, err := fluxAggregationFn(spec.Aggregation)
	if err != nil {
		return "", err
	}
	createEmpty, postPipe, err := fluxFillOptions(spec.Fill)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, fluxFromTmpl, escapeFluxStringLiteral(spec.Bucket))
	fmt.Fprintf(&b, fluxRangeTmpl, spec.StartMs*nsPerMs, spec.EndMs*nsPerMs)
	fmt.Fprintf(&b, fluxMeasurementTmpl, escapeFluxStringLiteral(spec.Measurement))
	fmt.Fprintf(&b, fluxFieldTmpl, escapeFluxStringLiteral(spec.Field))
	for _, k := range sortedTagKeys(spec.Tags) {
		fmt.Fprintf(&b, fluxTagTmpl,
			escapeFluxStringLiteral(k),
			escapeFluxStringLiteral(spec.Tags[k]))
	}
	fmt.Fprintf(&b, fluxAggregateWindowTmpl,
		seriesDurationLiteral(spec.IntervalMs), fn, createEmpty)
	b.WriteString(postPipe)
	b.WriteString(fluxKeepTmpl)
	return b.String(), nil
}

// --- InfluxQL 생성 (§2.7 v3 템플릿) ---

const (
	influxQLSelectTmpl = `SELECT %s("%s") FROM "%s"` + "\n"
	influxQLWhereTmpl  = ` WHERE time >= '%s' AND time < '%s'` + "\n"
	influxQLTagTmpl    = `   AND "%s" = '%s'` + "\n"

	// GROUP BY time(d) 에 offset 인자를 지정하지 않는다. 기본 offset 0 이 곧
	// epoch 정렬 + 시작 레이블이며, Store 의 (tsMs / intervalMs) * intervalMs 와
	// 같은 경계를 만든다(§2.8). offset 을 주면 그 일치가 깨진다.
	influxQLGroupByTmpl = ` GROUP BY time(%s) FILL(%s)`
)

// BuildInfluxQLSeriesQuery 는 InfluxDB 3.x 용 InfluxQL 쿼리를 생성한다(§2.7).
//
// v3 는 Flux 를 지원하지 않고 SQL 은 HTTP 계층에서 도달 불가이므로(§1.2.9)
// InfluxQL 이 유일한 경로다.
//
// spec.Bucket 은 이 템플릿에서 쓰이지 않는다 — v3 의 database 는 클라이언트
// 연결에 묶여 있고 §2.7 의 정본 템플릿이 FROM 절에 measurement 만 둔다.
func BuildInfluxQLSeriesQuery(spec SeriesQuerySpec) (string, error) {
	if err := spec.validateShape(); err != nil {
		return "", err
	}
	if err := spec.ValidateIdentifiers(); err != nil {
		return "", err
	}
	fn, err := influxQLAggregationFn(spec.Aggregation)
	if err != nil {
		return "", err
	}
	fillArg, err := influxQLFillArg(spec.Fill)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, influxQLSelectTmpl, fn,
		escapeInfluxQLIdent(spec.Field),
		escapeInfluxQLIdent(spec.Measurement))
	fmt.Fprintf(&b, influxQLWhereTmpl,
		formatInfluxQLTime(spec.StartMs),
		formatInfluxQLTime(spec.EndMs))
	for _, k := range sortedTagKeys(spec.Tags) {
		fmt.Fprintf(&b, influxQLTagTmpl,
			escapeInfluxQLIdent(k),
			escapeInfluxQLStringLiteral(spec.Tags[k]))
	}
	fmt.Fprintf(&b, influxQLGroupByTmpl, seriesDurationLiteral(spec.IntervalMs), fillArg)
	return b.String(), nil
}

// formatInfluxQLTime 은 epoch ms 를 InfluxQL 시간 리터럴(RFC3339Nano, UTC)로
// 변환한다.
func formatInfluxQLTime(ms int64) string {
	return time.UnixMilli(ms).UTC().Format(time.RFC3339Nano)
}

// SeriesBucketStartMs 는 타임스탬프를 버킷 시작 시각으로 정렬한다.
//
// 정본은 Store 의 epoch-zero 정렬이다(store_query.go 의
// bucketStartMs := (tsMs / intervalMs) * intervalMs). 두 소스가 같은 식을 쓰기
// 때문에 모든 인터벌에서 버킷 경계가 일치한다(§2.8).
func SeriesBucketStartMs(tsMs, intervalMs int64) int64 {
	if intervalMs <= 0 {
		return tsMs
	}
	return (tsMs / intervalMs) * intervalMs
}
