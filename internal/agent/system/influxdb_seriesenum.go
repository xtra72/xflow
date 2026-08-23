// @spec SPEC-TSDB-003 §2.3 (U3) · §2.4 (U4) · §2.7 (U7) · §2.8 (U8)
//
// 시리즈 열거(디스커버리 D5) 계층이다. "시리즈 = (measurement, 태그 집합)" 이라는
// InfluxDB 자신의 정의(§2.1)를 따라, 주어진 (measurement, 창, 사전 필터) 에서
// **실재하는** 태그 집합을 얻는다.
//
// **왜 백엔드가 열거해야 하는가.** 클라이언트가 태그 키 × 태그 값의 데카르트
// 곱으로 후보를 만들면 기록된 적 없는 조합이 후보에 섞인다(§4.2). 사용자가 그것을
// 고르면 조회는 성공하고 결과만 비어 오류로도 보이지 않는다 — 원인을 알 수 없는
// 조용한 오답이다. 실재 여부를 아는 것은 백엔드뿐이다.
//
// **파일 경계.** 쿼리 문자열 조립은 네트워크를 타지 않는 순수 함수로 분리한다
// (§2.3). 이는 BuildFluxSeriesQuery(influxdb_seriesquery.go) ·
// buildFluxTagKeysQuery(influxdb_schema.go) 가 이미 따르는 규율이며 새 규율이
// 아니다. 이스케이프·검증 헬퍼도 전부 재사용한다 — 신규 이스케이프 함수를
// 만들지 않는다(§2.3 · AC-08).
//
// **인터페이스 경계.** 열거는 기존 InfluxSchemaDiscoverer(influxdb_schema.go:41)
// 를 넓히지 않고 별도 InfluxSeriesEnumerator 로 둔다(plan.md §4 위험 R5).
// 넓히면 그 인터페이스를 만족하던 모든 구현(테스트 모의 포함)이 동시에 깨진다.
//
// v3 경로는 influxdb_seriesenum_v3.go 에 있다. 접기를 공유하지 않는 이유는
// 결손 태그 표현 · 제외 컬럼 · 시리즈당 행 수가 전부 다르기 때문이다
// (SPEC-TSDB-003 §HISTORY-0.5.0 (3)).
package system

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// ErrSeriesEnumerationNotSupported 는 클라이언트가 시리즈 열거를 지원하지 않을
// 때 반환하는 센티넬이다. HTTP 계층이 별도 상태 코드로 매핑할 수 있게 둔다.
//
// "디스커버리 실패"로 뭉뚱그리지 않는 이유는 §2.5 와 같다 — 원인을 알 수 없는
// 오류가 사용자에게 가장 비싼 실패다.
var ErrSeriesEnumerationNotSupported = errors.New("influxdb: series enumeration is not supported by this client")

// SeriesEnumSpec 는 열거 1건의 입력이다(§2.2 의 질의 파라미터에 대응).
type SeriesEnumSpec struct {
	// Bucket 은 v2 의 bucket, v3 의 database 다. 빈 값이면 에이전트/클라이언트
	// 기본값으로 채워지며, 그 채움은 호출자의 책임이다.
	Bucket string
	// Measurement 는 열거 대상이며 필수다.
	Measurement string
	// Tags 는 사전 필터다. 지정된 키/값에 일치하는 시리즈만 열거한다.
	// 생성 순서는 키 오름차순으로 고정한다 — 맵 순회는 비결정적이다.
	Tags map[string]string
	// StartMs 는 탐색 창 시작(포함)이다.
	StartMs int64
	// EndMs 는 탐색 창 끝(미포함)이다.
	EndMs int64
	// RowLimit 은 **접기 전 원시 행** 상한이다. 0 이하면 기본값을, 상한을 넘으면
	// 상한을 쓴다(§2.7). 반환 태그 집합 상한(1,000) 은 별개 축이며 HTTP 계층이
	// 강제한다 — 그쪽은 DOM 행 수를, 이쪽은 백엔드 계산량을 막는다.
	RowLimit int
}

// EnumeratedSeries 는 열거된 시리즈 1개다.
type EnumeratedSeries struct {
	// Tags 는 실재하는 태그 집합 1벌이다. 키 없는 시리즈는 빈 맵이다.
	Tags map[string]string `json:"tags"`
	// Fields 는 그 태그 집합에서 관측된 field 키 목록이며 사전순이다.
	Fields []string `json:"fields"`
}

// SeriesEnumResult 는 열거 1건의 산출이다.
//
// FieldExact 를 결과에 담는 이유는 §2.6 이다 — 프런트의 능력 표는 렌더 시점의
// 낙관적 표시값이고 정본은 서버 응답이다. 백엔드별로 값이 다르므로(v2 true ·
// v3 false) 열거를 수행한 계층이 함께 보고해야 한다.
type SeriesEnumResult struct {
	// Series 는 태그 직렬화 오름차순으로 정렬된 시리즈 목록이다.
	Series []EnumeratedSeries
	// FieldExact 는 Fields 가 정확한 관측치인지(true) measurement 전체 field
	// 목록의 근사인지(false) 를 나타낸다.
	FieldExact bool
}

// InfluxSeriesEnumerator 는 시리즈 열거 계약이다.
// HTTP 핸들러가 타입 단언으로 에이전트를 검증한다.
//
// InfluxSchemaDiscoverer 와 합치지 않는다(plan.md §4 위험 R5).
type InfluxSeriesEnumerator interface {
	EnumerateSeries(ctx context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error)
}

// 컴파일 타임 인터페이스 준수 체크.
var (
	_ InfluxSeriesEnumerator = (*InfluxDBAgent)(nil)
	_ InfluxSeriesEnumerator = (*influxV2Client)(nil)
)

// --- 상한과 창 기본값 ---

const (
	// maxSeriesEnumRawRows 는 접기 전 원시 행 상한이다(§2.7).
	// 반환 태그 집합 상한(1,000) 보다 커야 한다 — 접기 전 행이기 때문이다.
	maxSeriesEnumRawRows = 20000

	// seriesEnumDefaultWindowMs 는 탐색 창 기본 폭 30일이다(§2.8 · §4.6).
	//
	// 임의로 고른 값이 아니다. v2 의 schema.measurementTagKeys ·
	// measurementTagValues · measurementFieldKeys 셋 모두 start 기본값이 -30d 이며
	// (공식 문서 확인 — SPEC-TSDB-003 §7 OQ5), 현재 D2~D4 가 이미 그 값으로
	// 동작한다. 다른 값을 고르면 본 SPEC 이 사용자가 보던 목록의 범위를 조용히
	// 바꾸게 된다.
	seriesEnumDefaultWindowMs = int64(30 * 24 * 60 * 60 * 1000)
)

// ResolveSeriesEnumWindow 는 미지정 탐색 창을 기본값으로 채운다(§2.8).
//
// now 를 인자로 받는 것은 의도적이다 — 내부에서 time.Now() 를 읽으면 순수성이
// 깨지고 테스트가 시계에 의존한다. HTTP 계층이 time.Now() 를 넘긴다.
//
// 서버가 실제로 사용한 창을 응답에 담아야 하므로(§2.8) 이 함수의 반환값이 곧
// 응답의 window 다.
func ResolveSeriesEnumWindow(startMs, endMs int64, now time.Time) (int64, int64) {
	if endMs <= 0 {
		endMs = now.UnixMilli()
	}
	if startMs <= 0 {
		startMs = endMs - seriesEnumDefaultWindowMs
	}
	return startMs, endMs
}

// resolveSeriesEnumRowLimit 은 원시 행 상한을 [1, maxSeriesEnumRawRows] 로
// 접는다. 클라이언트가 보낸 값이 백엔드 계산량을 무제한으로 늘릴 수 없어야
// 한다 — 상한을 서버에 두는 이유가 그것이다(§2.7).
func resolveSeriesEnumRowLimit(limit int) int {
	if limit <= 0 || limit > maxSeriesEnumRawRows {
		return maxSeriesEnumRawRows
	}
	return limit
}

// validateShape 는 식별자 내용과 무관한 요청 형상을 검사한다.
func (s SeriesEnumSpec) validateShape() error {
	if s.Measurement == "" {
		return errors.New("influxdb: measurement is required")
	}
	if s.EndMs <= s.StartMs {
		return errors.New("influxdb: end_ms must be greater than start_ms")
	}
	return nil
}

// ValidateIdentifiers 는 spec 의 모든 사용자 유래 식별자를 검증한다.
// HTTP 핸들러가 질의 전에 호출해 400 으로 거부할 수 있게 공개한다.
//
// 판정은 SeriesQuerySpec.ValidateIdentifiers 와 같은 헬퍼를 쓴다 — 두 경로 모두
// 사용자 데이터를 쿼리 문자열에 삽입하므로 판정이 달라서는 안 된다(§2.3).
func (s SeriesEnumSpec) ValidateIdentifiers() error {
	if err := validateSeriesIdentifier("bucket", s.Bucket); err != nil {
		return err
	}
	if err := validateSeriesIdentifier("measurement", s.Measurement); err != nil {
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

// --- v2 Flux 생성 (§2.4 정본 템플릿) ---

// 템플릿 조각을 raw string 으로 두는 이유는 생성될 쿼리 문자열이 소스에 그대로
// 보이게 하기 위함이다. from · range · measurement filter · tag filter 네 조각은
// influxdb_seriesquery.go 의 것을 그대로 재사용한다.
const (
	// fluxEnumFirstPipe 는 각 테이블을 1행으로 줄인다.
	//
	// first() 이며 last() 가 아니다(SPEC-TSDB-003 §7 OQ10 확정). 둘 다 푸시다운
	// 가능하나 하나로 고정해야 생성 결과가 결정적이다.
	fluxEnumFirstPipe = `  |> first()` + "\n"

	// fluxEnumGroupPipe 는 테이블당 1행이 된 결과를 한 테이블로 모은다.
	fluxEnumGroupPipe = `  |> group()` + "\n"

	// fluxEnumLimitTmpl 은 원시 행 상한이다. 기본값이 적용되면
	// `  |> limit(n: 20000)` 이 된다(§2.7 · AC-21).
	fluxEnumLimitTmpl = `  |> limit(n: %d)`
)

// BuildFluxSeriesEnumQuery 는 InfluxDB 2.x 용 시리즈 열거 Flux 쿼리를 만든다(§2.4).
//
// 순수 함수다 — 네트워크도, 시각도, 전역 상태도 읽지 않는다.
//
// **동작 원리.** filter 이후 그룹 키가 [_start, _stop, _measurement, _field, <태그>]
// 이므로 테이블 1개가 곧 (field, 태그 집합) 1벌이다. first() 가 각 테이블을 1행으로
// 줄이고, group() 이 그것을 한 테이블로 모은다. 서버는 그 행들의 _field 와
// 밑줄로 시작하지 않는 컬럼에서 시리즈를 읽는다.
//
// **Flux 의 distinct 축약 함수를 쓰지 않는다.** 푸시다운 표에 first() 와
// group() |> first() 는 있으나 그 함수는 없다. 축약이 스토리지 계층으로
// 내려가지 않으면 열거 비용이 창 안의 전체 포인트 수에 비례한다(§2.4 · UB1-13).
// 이 파일에 해당 호출 형태가 문자열로도 등장하지 않아야 한다(AC-10) — 그래서
// 주석에서도 괄호를 붙여 쓰지 않는다.
func BuildFluxSeriesEnumQuery(spec SeriesEnumSpec) (string, error) {
	if err := spec.validateShape(); err != nil {
		return "", err
	}
	if spec.Bucket == "" {
		// Flux 의 from() 은 bucket 을 구조적으로 요구한다. 기본값 채움은 호출자의
		// 책임이며, 여기까지 빈 값이 왔다면 버그다. BuildFluxSeriesQuery 와 같다.
		return "", errors.New("influxdb: bucket is required for flux queries")
	}
	if err := spec.ValidateIdentifiers(); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, fluxFromTmpl, escapeFluxStringLiteral(spec.Bucket))
	fmt.Fprintf(&b, fluxRangeTmpl, spec.StartMs*nsPerMs, spec.EndMs*nsPerMs)
	fmt.Fprintf(&b, fluxMeasurementTmpl, escapeFluxStringLiteral(spec.Measurement))
	for _, k := range sortedTagKeys(spec.Tags) {
		fmt.Fprintf(&b, fluxTagTmpl,
			escapeFluxStringLiteral(k),
			escapeFluxStringLiteral(spec.Tags[k]))
	}
	b.WriteString(fluxEnumFirstPipe)
	b.WriteString(fluxEnumGroupPipe)
	fmt.Fprintf(&b, fluxEnumLimitTmpl, resolveSeriesEnumRowLimit(spec.RowLimit))
	return b.String(), nil
}

// --- 행 접기 ---

// fluxFieldColumn 은 Flux 결과에서 field 이름을 담은 컬럼이다.
const fluxFieldColumn = "_field"

// fluxResultColumn · fluxTableColumn 은 Flux 주석 CSV 의 구조 컬럼이다.
//
// 이 둘은 밑줄로 시작하지 않으므로 §2.4 의 "밑줄 접두" 규칙만으로는 걸러지지
// 않는다. 그러나 influxdb-client-go 의 FluxRecord.Values() 는 CSV 의 모든 컬럼을
// 그대로 담으므로(api/query.go 의 행 파싱), 걸러내지 않으면 **모든 시리즈에**
// result="_result" · table=0 이라는 존재하지 않는 태그가 붙는다.
// 그 결과는 오류가 아니라 조용한 오답이므로(§4.2) 여기서 막는다.
const (
	fluxResultColumn = "result"
	fluxTableColumn  = "table"
)

// foldEnumRows 는 열거 쿼리의 원시 행을 태그 집합 기준으로 접는다(§2.4).
//
// 판정 규칙 셋:
//
//  1. 밑줄로 시작하는 컬럼은 태그가 아니다. filterInternalTagKeys
//     (influxdb_schema.go) 가 D2 에서 하는 판정과 같으며 신규 규칙이 아니다.
//     그중 _field 만 field 이름으로 승격한다.
//  2. Flux 구조 컬럼(result · table) 도 태그가 아니다(위 상수 주석).
//  3. 빈 문자열 태그 값은 태그가 아니다. InfluxDB 는 빈 태그 값을 저장하지
//     않으며, group() 이 서로 다른 스키마의 테이블을 모을 때 없는 컬럼이 빈 값으로
//     채워지므로 걸러내지 않으면 같은 시리즈가 둘로 쪼개진다.
//
// 반환은 태그 직렬화 문자열의 오름차순이다(§2.2 · UB1-16). 절단이 발생할 때
// 폴링마다 다른 부분집합이 잘리면 사용자가 고른 시리즈가 목록에서 사라졌다
// 나타났다 한다.
func foldEnumRows(rows []map[string]any) []EnumeratedSeries {
	type accumulator struct {
		tags   map[string]string
		fields map[string]struct{}
	}

	byKey := make(map[string]*accumulator, len(rows))
	for _, row := range rows {
		tags := make(map[string]string, len(row))
		field := ""
		for k, v := range row {
			if k == "" {
				continue
			}
			if k[0] == '_' {
				if k == fluxFieldColumn {
					field = enumColumnString(v)
				}
				continue
			}
			if k == fluxResultColumn || k == fluxTableColumn {
				continue
			}
			if s := enumColumnString(v); s != "" {
				tags[k] = s
			}
		}

		key := serializeTagSet(tags)
		acc, ok := byKey[key]
		if !ok {
			acc = &accumulator{tags: tags, fields: make(map[string]struct{})}
			byKey[key] = acc
		}
		if field != "" {
			acc.fields[field] = struct{}{}
		}
	}

	keys := make([]string, 0, len(byKey))
	for k := range byKey {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]EnumeratedSeries, 0, len(keys))
	for _, k := range keys {
		acc := byKey[k]
		fields := make([]string, 0, len(acc.fields))
		for f := range acc.fields {
			fields = append(fields, f)
		}
		sort.Strings(fields)
		out = append(out, EnumeratedSeries{Tags: acc.tags, Fields: fields})
	}
	return out
}

// enumColumnString 은 컬럼 값을 문자열로 만든다.
//
// 문자열이 아닌 값도 버리지 않고 보존한다. 태그를 조용히 버리면 서로 다른 두
// 시리즈가 하나로 합쳐지고, 그것은 §4.2 가 막으려는 조용한 오답과 같은 종류다.
// nil 은 "없음"이므로 빈 문자열로 접는다.
func enumColumnString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprint(t)
	}
}

// serializeTagSetEscaper 는 직렬화의 구분자 충돌을 막는다.
// 이스케이프가 없으면 {a: "b,c=d"} 와 {a: "b", c: "d"} 가 같은 키가 되어 서로
// 다른 두 시리즈가 하나로 접힌다.
var serializeTagSetEscaper = strings.NewReplacer(`\`, `\\`, `,`, `\,`, `=`, `\=`)

// serializeTagSet 은 태그 집합을 정렬 가능한 단일 문자열로 만든다(§2.2).
// 키 사전순으로 직렬화하므로 같은 태그 집합은 항상 같은 문자열이 된다.
func serializeTagSet(tags map[string]string) string {
	if len(tags) == 0 {
		return ""
	}
	parts := make([]string, 0, len(tags))
	for _, k := range sortedTagKeys(tags) {
		parts = append(parts,
			serializeTagSetEscaper.Replace(k)+"="+serializeTagSetEscaper.Replace(tags[k]))
	}
	return strings.Join(parts, ",")
}

// --- v2 클라이언트 ---

// fluxQueryRunner 는 Flux 질의 실행자다.
//
// 실행을 인자로 받는 이유는 조립·접기 로직을 네트워크 없이 검증하기 위함이다.
// influxV2Client.queryFlux 가 유일한 실제 구현이다.
type fluxQueryRunner func(ctx context.Context, query string) ([]map[string]any, error)

// enumerateSeriesWithFlux 는 v2 열거의 네트워크 비의존 코어다.
//
// v2 경로의 FieldExact 는 항상 true 다 — 그룹 키에 _field 가 포함되므로 각 태그
// 집합의 field 목록이 근사가 아니라 관측치다(§2.4 · §2.6).
func enumerateSeriesWithFlux(ctx context.Context, spec SeriesEnumSpec, run fluxQueryRunner) (SeriesEnumResult, error) {
	flux, err := BuildFluxSeriesEnumQuery(spec)
	if err != nil {
		return SeriesEnumResult{}, err
	}
	rows, err := run(ctx, flux)
	if err != nil {
		return SeriesEnumResult{}, fmt.Errorf("influxdb v2 enumerate series (bucket=%q, measurement=%q): %w",
			spec.Bucket, spec.Measurement, err)
	}
	return SeriesEnumResult{Series: foldEnumRows(rows), FieldExact: true}, nil
}

// EnumerateSeries 는 그룹 키에서 시리즈를 도출한다(D5 의 v2 경로 · §2.4).
func (c *influxV2Client) EnumerateSeries(ctx context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error) {
	bucket, err := c.resolveSchemaBucket(spec.Bucket)
	if err != nil {
		return SeriesEnumResult{}, err
	}
	spec.Bucket = bucket
	return enumerateSeriesWithFlux(ctx, spec, c.queryFlux)
}

// --- 에이전트 위임 ---

// EnumerateSeries 는 client 에 위임하여 시리즈를 열거한다(D5).
//
// bucket 이 비어 있으면 에이전트 설정의 기본 버킷을 쓴다 — ListTagKeys 등
// D2~D4 와 같은 규약이다.
//
// 열거를 구현하지 않는 클라이언트는 ErrSeriesEnumerationNotSupported
// 로 표면화한다. InfluxClient 인터페이스를 넓히지 않는 이유는 위험 R5 와 같다 —
// 넓히면 그것을 만족하던 모든 구현과 테스트 모의가 동시에 깨진다.
func (a *InfluxDBAgent) EnumerateSeries(ctx context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error) {
	c, err := a.managementClient()
	if err != nil {
		return SeriesEnumResult{}, err
	}
	enumerator, ok := c.(InfluxSeriesEnumerator)
	if !ok {
		return SeriesEnumResult{}, ErrSeriesEnumerationNotSupported
	}
	if spec.Bucket == "" {
		spec.Bucket = a.defaultBucket()
	}
	return enumerator.EnumerateSeries(ctx, spec)
}
