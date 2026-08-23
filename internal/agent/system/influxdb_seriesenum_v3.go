// @spec SPEC-TSDB-003 §2.3 (U3) · §2.5 (U5) · §2.7 (U7) · §2.8 (U8)
//
// 시리즈 열거(디스커버리 D5) 의 **v3 경로**다. v2 경로는
// influxdb_seriesenum.go 에 있으며, 타입(SeriesEnumSpec · EnumeratedSeries ·
// SeriesEnumResult)과 상한·창 기본값은 그 파일의 것을 그대로 쓴다.
//
// **v3 에는 시리즈를 직접 여는 메타쿼리가 없다.** InfluxQL 의 SHOW 계열 중
// 그 한 문장은 공식 문서 두 곳이 미지원을 명시하고(§1.2.4), 실측에서도
// 런타임이 아니라 **파싱 단계**에서 거부되었다(§HISTORY-0.5.0 (1)). 그래서
// 대체 경로가 필요하고, 그 선택이 이 SPEC 의 주요 설계 결정이었다.
//
// 그 미지원 메타쿼리의 이름을 이 파일에 **문자열로 적지 않는다.** UB1-1 은
// grep 으로 기계 검증되며(plan.md M3.5), 주석에 적으면 그 검증이 주석에
// 걸린다. M2 가 distinct 축약 함수에 대해 같은 규율을 따랐다.
//
// **채택된 경로는 InfluxQL 2질의다**(OQ2 확정 — §2.5).
//
//	1단계  SHOW TAG KEYS FROM "<m>"                    ← 태그 키 집합의 정본
//	2단계  SELECT * FROM "<m>" WHERE ... GROUP BY * LIMIT n
//
// 1단계가 없으면 안 되는 이유는 하나다 — **태그와 필드는 응답 타입으로
// 구분되지 않는다.** 문자열 필드가 있으면 태그와 JSON 타입이 같아진다
// (실측 measurement `ambig`: host 는 태그, status 는 문자열 필드 —
// §HISTORY-0.5.0 (4)). "문자열이면 태그" 로 판정하면 존재하지 않는 시리즈가
// 만들어지고, 그것은 §4.2 가 막으려는 조용한 오답이다.
//
// **폐기된 대안**: SQL `SELECT DISTINCT` 3질의 경로. 질의가 하나 더 들고,
// 필드 축이 measurement 전역이라 시리즈별 정확도가 낮다(§2.5 · §4.3).
//
// **v2 의 접기 함수를 재사용할 수 없다.** 세 지점에서 갈린다
// (§HISTORY-0.5.0 (3)):
//
//	        결손 태그        제외할 비-밑줄 컬럼        시리즈당 행 수
//	v2      빈 문자열        result · table            first() 가 1행 보장
//	v3      키 부재          iox::measurement · time   보장 없음(필드 시각별 분리)
//
// 그래서 foldEnumRowsV3 를 따로 둔다. 공용화하면 두 백엔드의 서로 다른 함정을
// 한 함수의 분기로 감추게 되고, 그 분기는 다음 백엔드에서 다시 깨진다.
package system

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// 컴파일 타임 인터페이스 준수 체크.
//
// v2 쪽 단언은 influxdb_seriesenum.go 가 갖는다. 이 파일이 v3 항목만 더하는
// 이유는 M2 산출물을 한 글자도 건드리지 않기 위함이다.
var _ InfluxSeriesEnumerator = (*influxV3Client)(nil)

// --- v3 InfluxQL 생성 (§2.5 2단계 템플릿) ---

const (
	// influxQLEnumSelectTmpl 은 전 컬럼을 회수한다. 태그가 각 행의 **평탄한
	// 컬럼**으로 나타나므로(§HISTORY-0.5.0 (2)) 별도 tags 객체 파싱이 없다.
	influxQLEnumSelectTmpl = `SELECT * FROM "%s"` + "\n"

	// influxQLEnumWhereTmpl 의 시간 술어는 **나노초 정수**다.
	//
	// RFC3339Nano 문자열 술어도 실측에서 동작했으나 정수를 택했다 — v2 경로의
	// range(start: time(v: <ns>)) 와 단위가 같아져 두 백엔드의 창 해석이
	// 어긋날 여지가 없어지고, 타임존·포맷 왕복이 사라진다.
	influxQLEnumWhereTmpl = ` WHERE time >= %d AND time < %d` + "\n"

	// influxQLEnumGroupByTmpl 의 GROUP BY * 가 이 경로의 하중 지지점이다.
	// LIMIT 은 원시 행 상한이며 접기 전 행에 걸린다(§2.7).
	influxQLEnumGroupByTmpl = ` GROUP BY * LIMIT %d`
)

// BuildInfluxQLSeriesEnumQuery 는 InfluxDB 3.x 용 시리즈 열거 InfluxQL 쿼리를
// 만든다(§2.5 2단계).
//
// 순수 함수다 — 네트워크도, 시각도, 전역 상태도 읽지 않는다.
//
// **미지원 시리즈 메타쿼리를 발행하지 않는다**(UB1-1). 이 함수의 출력에 그
// 문자열이 등장하지 않음을 테스트가 기계 검증한다 — 단언문은 검증 대상
// 문자열을 써야 하므로 _test.go 에만 있다.
//
// spec.Bucket 은 이 템플릿에서 쓰이지 않는다 — v3 의 database 는 클라이언트
// 연결에 묶여 있다. BuildInfluxQLSeriesQuery 와 같은 규약이며, 그래서 v2 와
// 달리 bucket 이 비어도 오류가 아니다.
func BuildInfluxQLSeriesEnumQuery(spec SeriesEnumSpec) (string, error) {
	if err := spec.validateShape(); err != nil {
		return "", err
	}
	if err := spec.ValidateIdentifiers(); err != nil {
		return "", err
	}

	var b strings.Builder
	fmt.Fprintf(&b, influxQLEnumSelectTmpl, escapeInfluxQLIdent(spec.Measurement))
	fmt.Fprintf(&b, influxQLEnumWhereTmpl, spec.StartMs*nsPerMs, spec.EndMs*nsPerMs)
	for _, k := range sortedTagKeys(spec.Tags) {
		// 사전 필터는 기존 구조화 질의의 태그 술어 템플릿을 그대로 쓴다.
		// 식별자는 큰따옴표, 값은 작은따옴표 리터럴이므로 이스케이프 함수가
		// 다르다 — 신규 이스케이프 함수를 만들지 않는다(§2.3 · AC-08).
		fmt.Fprintf(&b, influxQLTagTmpl,
			escapeInfluxQLIdent(k),
			escapeInfluxQLStringLiteral(spec.Tags[k]))
	}
	fmt.Fprintf(&b, influxQLEnumGroupByTmpl, resolveSeriesEnumRowLimit(spec.RowLimit))
	return b.String(), nil
}

// --- v3 행 접기 (§2.5) ---

// influxQLMeasurementMetaColumn 은 v3 응답이 모든 행에 붙이는 measurement
// 메타 컬럼이다.
//
// **밑줄이 없다.** v2 의 result·table 과 정확히 같은 함정 부류이며, §2.5 의
// "밑줄 접두" 규칙만으로는 걸러지지 않는다. 걸러내지 않으면 모든 시리즈에
// 존재하지 않는 필드가 붙는다.
//
// 함께 제외해야 하는 시각 컬럼은 기존 influxQLTimeColumn(influxdb_agent.go)
// 을 재사용한다 — 값이 같고("time") 뜻도 같으므로 상수를 새로 두지 않는다.
// 그쪽도 v2 의 _time 과 달리 밑줄이 없다는 점이 요지다.
const influxQLMeasurementMetaColumn = "iox::measurement"

// foldEnumRowsV3 는 v3 열거 쿼리의 원시 행을 태그 집합 기준으로 접는다(§2.5).
//
// tagKeys 는 1단계 `SHOW TAG KEYS` 의 결과이며 **행 키 분류의 정본**이다.
// 이 인자 없이는 태그와 문자열 필드를 구분할 수 없다.
//
// 판정 규칙 넷:
//
//  1. iox::measurement · time 은 제외한다. 둘 다 밑줄이 없어 규칙 2 로는
//     걸러지지 않는다.
//  2. 밑줄 접두 컬럼은 제외한다(방어적). v3 응답에서 관측되지는 않았으나,
//     걸러 두면 백엔드가 내부 컬럼을 추가했을 때의 사고를 막는다.
//  3. tagKeys 에 속하면 태그다. nil · 빈 문자열은 태그가 아니다 —
//     InfluxDB 는 빈 태그 값을 저장하지 않는다. **다만 v3 에서 결손의 실제
//     표현은 키 부재이며**(§HISTORY-0.5.0 (3)), 빈 값 처리는 방어다.
//  4. 그 외는 필드 이름이다. nil 값은 그 행에서 관측되지 않은 것이므로 필드
//     목록에 넣지 않는다 — 넣으면 존재하지 않는 (field, 태그) 조합이 후보가
//     된다(§4.2).
//
// **시리즈당 1행이 보장되지 않는다.** 필드가 서로 다른 시각에 기록된 시리즈는
// 시각마다 별도 행으로 온다(실측: multi 의 host=c → 2행). 그래서 태그 집합
// 기준으로 중복을 합치고 **필드를 합집합**한다. 합치지 않으면 같은 시리즈가
// 둘로 쪼개져 선택 표에 중복 행이 나타난다.
//
// 반환은 태그 직렬화 문자열의 오름차순이다(§2.2 · UB1-16) — v2 와 같은 규약이며
// serializeTagSet 을 공유한다. 절단이 발생할 때 폴링마다 다른 부분집합이 잘리면
// 사용자가 고른 시리즈가 목록에서 사라졌다 나타났다 한다.
func foldEnumRowsV3(rows []map[string]any, tagKeys []string) []EnumeratedSeries {
	tagKeySet := make(map[string]struct{}, len(tagKeys))
	for _, k := range tagKeys {
		tagKeySet[k] = struct{}{}
	}

	type accumulator struct {
		tags   map[string]string
		fields map[string]struct{}
	}

	byKey := make(map[string]*accumulator, len(rows))
	for _, row := range rows {
		tags := make(map[string]string, len(row))
		fields := make([]string, 0, len(row))

		for k, v := range row {
			if k == "" {
				continue
			}
			if k == influxQLMeasurementMetaColumn || k == influxQLTimeColumn {
				continue
			}
			if k[0] == '_' {
				continue
			}
			if _, isTag := tagKeySet[k]; isTag {
				if s := enumColumnString(v); s != "" {
					tags[k] = s
				}
				continue
			}
			if v != nil {
				fields = append(fields, k)
			}
		}

		key := serializeTagSet(tags)
		acc, ok := byKey[key]
		if !ok {
			acc = &accumulator{tags: tags, fields: make(map[string]struct{})}
			byKey[key] = acc
		}
		for _, f := range fields {
			acc.fields[f] = struct{}{}
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

// --- v3 클라이언트 ---

// influxQLTagKeyLister 는 1단계(SHOW TAG KEYS) 실행자다.
// 시그니처는 기존 D2 ListTagKeys(influxdb_schema.go) 와 같다 — 재사용이
// 목적이며(§2.5), bucket 인자는 v3 에서 무시된다.
type influxQLTagKeyLister func(ctx context.Context, bucket, measurement string) ([]string, error)

// influxQLQueryRunner 는 2단계(GROUP BY *) 실행자다.
//
// 실행을 인자로 받는 이유는 조립·접기 로직을 네트워크 없이 검증하기 위함이다.
// v2 의 fluxQueryRunner 와 같은 규율이며, v3 에서는 이 분리가 특히 필요하다 —
// influxdb3-go 는 Arrow Flight(gRPC) 로 질의하므로 httptest 로 대체할 수 없다.
type influxQLQueryRunner func(ctx context.Context, query string) ([]map[string]any, error)

// enumerateSeriesWithInfluxQL 은 v3 열거의 네트워크 비의존 코어다(§2.5).
//
// 순서가 중요하다. 순수 생성이 먼저이므로 잘못된 입력은 네트워크를 타지 않고,
// 그 다음 태그 키 집합을 얻는다 — 그 집합 없이는 행의 키를 분류할 수 없으므로
// 1단계가 실패하면 2단계를 시도하지 않는다.
//
// FieldExact 는 항상 false 다(OQ3 확정). 행에서 시리즈별 필드를 실제로
// 관측하므로 정확도는 높지만, LIMIT 이 표본을 자르면 필드가 누락될 수 있어
// 관측된 필드 집합은 **하한**이다(§HISTORY-0.5.0 (5) · §2.6).
func enumerateSeriesWithInfluxQL(
	ctx context.Context,
	spec SeriesEnumSpec,
	listTagKeys influxQLTagKeyLister,
	run influxQLQueryRunner,
) (SeriesEnumResult, error) {
	q, err := BuildInfluxQLSeriesEnumQuery(spec)
	if err != nil {
		return SeriesEnumResult{}, err
	}

	tagKeys, err := listTagKeys(ctx, spec.Bucket, spec.Measurement)
	if err != nil {
		return SeriesEnumResult{}, fmt.Errorf("influxdb v3 enumerate series tag keys (measurement=%q): %w",
			spec.Measurement, err)
	}

	rows, err := run(ctx, q)
	if err != nil {
		return SeriesEnumResult{}, fmt.Errorf("influxdb v3 enumerate series (measurement=%q): %w",
			spec.Measurement, err)
	}

	return SeriesEnumResult{Series: foldEnumRowsV3(rows, tagKeys), FieldExact: false}, nil
}

// EnumerateSeries 는 SHOW TAG KEYS + GROUP BY * 2질의로 시리즈를 열거한다
// (D5 의 v3 경로 · §2.5).
//
// **의도된 반전이다.** M2 시점에는 v3 클라이언트가 이 인터페이스를 만족하지
// 않아 에이전트가 ErrSeriesEnumerationNotSupported 를 반환했다. OQ2 가 실측으로
// 닫히면서 그 미지원이 해제된다 — SPEC-TSDB-002 의 v3 ListMeasurements 501 해제와
// 같은 형태의 반전이다.
func (c *influxV3Client) EnumerateSeries(ctx context.Context, spec SeriesEnumSpec) (SeriesEnumResult, error) {
	return enumerateSeriesWithInfluxQL(ctx, spec, c.ListTagKeys, c.queryInfluxQL)
}
