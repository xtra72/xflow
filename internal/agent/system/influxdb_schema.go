// @spec SPEC-TSDB-002 §2.10 (U10)
//
// InfluxDB 스키마 디스커버리 계층이다. TSDB 선택 UI 가 필요로 하는 4 종
// (D1 measurement · D2 tag key · D3 tag value · D4 field key) 중 D2~D4 를 v2/v3
// 양쪽에서 구현한다. D1(measurement) 은 이미 InfluxClient 에 존재하므로
// influxdb_v2.go · influxdb_v3.go 가 그대로 소유한다.
//
// v2/v3 구현을 각 어댑터 파일로 흩지 않고 한 파일에 모으는 이유는 셋이다 —
// (1) 디스커버리는 읽기 전용 한 축이며 쓰기·관리와 관심사가 다르다,
// (2) 아래 "쿼리 문자열의 출처" 주석이 한 곳에만 남는다,
// (3) 방언 차이(Flux vs InfluxQL)를 나란히 두면 D2~D4 의 대응 관계가 보인다.
//
// **쿼리 문자열의 출처.** D2·D3 의 v2/v3 쿼리 문자열은
// tsdbtags 마이그레이션 패키지(internal/migrate 하위)의 client_v2.go ·
// client_v3.go 에서 **복제**한 것이다.
// 그 패키지를 import 하지 않는 이유는 §4.5 에 있다 — 그 패키지의 존재 이유가
// "마이그레이션 도구는 write 하지 않는다" 는 컴파일 타임 보장인데, API 계층이
// 같은 인터페이스를 쓰면 API 쪽 필요(예: field key)가 그 좁힘을 넓힌다.
// 문자열 중복은 양쪽 fixture 테스트로 관리 가능한 중복이고, 인터페이스 결합은
// 그렇게 관리되지 않는다. 같은 판단을 M4 의 escapeInfluxQLIdent 도 따랐다.
//
// **D4(field key)는 신규 구현이다.** tsdbtags 는 태그 디스커버리만 덮으며
// field key 대응물이 트리 전체에 없었다(§1.2.10).
//
// 시스템은 디스커버리 응답을 캐시하지 않는다(§2.10 · UB1-12). 스키마는 쓰기에
// 따라 변하며, 오래된 목록에서 고른 시리즈는 조회 시 빈 결과가 된다.
package system

import (
	"context"
	"fmt"
)

// InfluxSchemaDiscoverer 는 스키마 디스커버리 계약이다.
// HTTP 핸들러가 타입 단언으로 에이전트를 검증한다.
//
// D1(ListMeasurements) 은 InfluxManager 가 이미 갖고 있으므로 여기 두지 않는다.
// 두 인터페이스를 합치면 디스커버리 라우트가 관리 조작 능력까지 요구하게 되고,
// 그것은 읽기 전용 경로에 필요 없는 넓힘이다.
type InfluxSchemaDiscoverer interface {
	ListTagKeys(ctx context.Context, bucket, measurement string) ([]string, error)
	ListTagValues(ctx context.Context, bucket, measurement, tagKey string) ([]string, error)
	ListFieldKeys(ctx context.Context, bucket, measurement string) ([]string, error)
}

// 컴파일 타임 인터페이스 준수 체크.
var _ InfluxSchemaDiscoverer = (*InfluxDBAgent)(nil)

// --- 에이전트 위임 ---
//
// bucket 이 비어 있으면 에이전트 설정의 기본 버킷을 쓴다. ListMeasurements 의
// 규약(influxdb_management.go)과 동일하다.

// ListTagKeys 는 client 에 위임하여 measurement 의 태그 키 목록을 반환한다(D2).
func (a *InfluxDBAgent) ListTagKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	c, err := a.managementClient()
	if err != nil {
		return nil, err
	}
	if bucket == "" {
		bucket = a.defaultBucket()
	}
	return c.ListTagKeys(ctx, bucket, measurement)
}

// ListTagValues 는 client 에 위임하여 (measurement, tagKey) 의 태그 값 목록을
// 반환한다(D3).
func (a *InfluxDBAgent) ListTagValues(ctx context.Context, bucket, measurement, tagKey string) ([]string, error) {
	c, err := a.managementClient()
	if err != nil {
		return nil, err
	}
	if bucket == "" {
		bucket = a.defaultBucket()
	}
	return c.ListTagValues(ctx, bucket, measurement, tagKey)
}

// ListFieldKeys 는 client 에 위임하여 measurement 의 필드 키 목록을 반환한다(D4).
func (a *InfluxDBAgent) ListFieldKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	c, err := a.managementClient()
	if err != nil {
		return nil, err
	}
	if bucket == "" {
		bucket = a.defaultBucket()
	}
	return c.ListFieldKeys(ctx, bucket, measurement)
}

// --- 공통 검증 ---

// validateSchemaArgs 는 디스커버리 인자를 검증한다.
//
// 식별자 검증에 M4 의 validateSeriesIdentifier 를 재사용한다. 두 경로 모두
// 사용자 데이터를 쿼리 문자열에 삽입하므로 판정이 같아야 하며, 실패는
// ErrUnescapableIdentifier 로 표면화되어 HTTP 계층이 400 으로 매핑한다(§2.7).
//
// bucket 이 빈 문자열이면 검사를 건너뛴다 — v3 는 database 가 클라이언트 생성
// 시점에 고정되어 쿼리 문자열에 삽입되지 않으므로 검증할 대상이 없다.
// extras 는 (이름 → 값) 쌍이며 전부 필수로 취급한다.
func validateSchemaArgs(bucket, measurement string, extras map[string]string) error {
	if bucket != "" {
		if err := validateSeriesIdentifier("bucket", bucket); err != nil {
			return err
		}
	}
	if measurement == "" {
		return fmt.Errorf("influxdb: measurement is required")
	}
	if err := validateSeriesIdentifier("measurement", measurement); err != nil {
		return err
	}
	for what, v := range extras {
		if v == "" {
			return fmt.Errorf("influxdb: %s is required", what)
		}
		if err := validateSeriesIdentifier(what, v); err != nil {
			return err
		}
	}
	return nil
}

// --- v2 (Flux) ---
//
// 쿼리 생성은 순수 함수로 분리한다. 실행과 섞으면 문자열 형상을 검증하는 데
// mock HTTP 서버가 필요해지고, 그러면 "따옴표 하나가 빠졌다" 같은 결함이
// 네트워크 계층 뒤에 숨는다. M4 의 BuildFluxSeriesQuery 와 같은 규율이다.

// fluxSchemaImport 는 schema.* 함수를 쓰기 위한 Flux import 문이다.
// tsdbtags/client_v2.go 와 같은 문자열이다.
const fluxSchemaImport = `import "influxdata/influxdb/schema"`

// buildFluxTagKeysQuery 는 D2 의 v2 쿼리를 만든다.
//
// tsdbtags/client_v2.go 의 ListTagKeys 를 복제했다. 다만 %q 대신
// escapeFluxStringLiteral 을 쓴다 — %q 는 Flux 문자열의 ${ 보간을
// 이스케이프하지 않아 사용자 데이터가 식으로 해석될 여지를 남긴다(§2.7 · UB1-9).
func buildFluxTagKeysQuery(bucket, measurement string) (string, error) {
	if err := validateSchemaArgs(bucket, measurement, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\nschema.measurementTagKeys(bucket: \"%s\", measurement: \"%s\")",
		fluxSchemaImport,
		escapeFluxStringLiteral(bucket),
		escapeFluxStringLiteral(measurement),
	), nil
}

// buildFluxTagValuesQuery 는 D3 의 v2 쿼리를 만든다.
// tsdbtags/client_v2.go 의 ListTagValues 복제본이다.
func buildFluxTagValuesQuery(bucket, measurement, tagKey string) (string, error) {
	if err := validateSchemaArgs(bucket, measurement, map[string]string{"tag key": tagKey}); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\nschema.measurementTagValues(bucket: \"%s\", measurement: \"%s\", tag: \"%s\")",
		fluxSchemaImport,
		escapeFluxStringLiteral(bucket),
		escapeFluxStringLiteral(measurement),
		escapeFluxStringLiteral(tagKey),
	), nil
}

// buildFluxFieldKeysQuery 는 D4 의 v2 쿼리를 만든다.
//
// **신규 구현이다** — tsdbtags 는 태그 디스커버리만 덮으므로 복제 대상이 없다.
// schema.measurementFieldKeys 는 다른 schema.* 함수와 같이 _value 컬럼에 결과를
// 담으므로 파싱은 그대로 재사용된다.
func buildFluxFieldKeysQuery(bucket, measurement string) (string, error) {
	if err := validateSchemaArgs(bucket, measurement, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf("%s\nschema.measurementFieldKeys(bucket: \"%s\", measurement: \"%s\")",
		fluxSchemaImport,
		escapeFluxStringLiteral(bucket),
		escapeFluxStringLiteral(measurement),
	), nil
}

// collectFluxSchemaValues 는 schema.* 결과 행에서 _value 컬럼을 모은다.
// schema.* 함수는 모두 결과를 _value 에 담는다.
func collectFluxSchemaValues(rows []map[string]any) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		v, ok := row[fluxValueColumn]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// filterInternalTagKeys 는 밑줄로 시작하는 내부 컬럼을 제외한다.
//
// schema.measurementTagKeys 는 _start · _stop · _measurement · _field 를 함께
// 돌려준다. 이들은 사용자가 고를 태그가 아니다. tsdbtags/client_v2.go 의
// ListTagKeys 가 같은 필터를 갖는다.
func filterInternalTagKeys(keys []string) []string {
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if len(k) > 0 && k[0] == '_' {
			continue
		}
		out = append(out, k)
	}
	return out
}

// ListTagKeys 는 schema.measurementTagKeys() 로 태그 키 목록을 조회한다(D2).
func (c *influxV2Client) ListTagKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	bucket, err := c.resolveSchemaBucket(bucket)
	if err != nil {
		return nil, err
	}
	flux, err := buildFluxTagKeysQuery(bucket, measurement)
	if err != nil {
		return nil, err
	}
	rows, err := c.queryFlux(ctx, flux)
	if err != nil {
		return nil, fmt.Errorf("influxdb v2 list tag keys (bucket=%q, measurement=%q): %w",
			bucket, measurement, err)
	}
	return filterInternalTagKeys(collectFluxSchemaValues(rows)), nil
}

// ListTagValues 는 schema.measurementTagValues() 로 태그 값 목록을 조회한다(D3).
func (c *influxV2Client) ListTagValues(ctx context.Context, bucket, measurement, tagKey string) ([]string, error) {
	bucket, err := c.resolveSchemaBucket(bucket)
	if err != nil {
		return nil, err
	}
	flux, err := buildFluxTagValuesQuery(bucket, measurement, tagKey)
	if err != nil {
		return nil, err
	}
	rows, err := c.queryFlux(ctx, flux)
	if err != nil {
		return nil, fmt.Errorf("influxdb v2 list tag values (bucket=%q, measurement=%q, tag_key=%q): %w",
			bucket, measurement, tagKey, err)
	}
	return collectFluxSchemaValues(rows), nil
}

// ListFieldKeys 는 schema.measurementFieldKeys() 로 필드 키 목록을 조회한다(D4).
func (c *influxV2Client) ListFieldKeys(ctx context.Context, bucket, measurement string) ([]string, error) {
	bucket, err := c.resolveSchemaBucket(bucket)
	if err != nil {
		return nil, err
	}
	flux, err := buildFluxFieldKeysQuery(bucket, measurement)
	if err != nil {
		return nil, err
	}
	rows, err := c.queryFlux(ctx, flux)
	if err != nil {
		return nil, fmt.Errorf("influxdb v2 list field keys (bucket=%q, measurement=%q): %w",
			bucket, measurement, err)
	}
	return collectFluxSchemaValues(rows), nil
}

// resolveSchemaBucket 은 빈 bucket 을 클라이언트 기본 버킷으로 채운다.
func (c *influxV2Client) resolveSchemaBucket(bucket string) (string, error) {
	if bucket == "" {
		bucket = c.bucket
	}
	if bucket == "" {
		return "", fmt.Errorf("influxdb v2 schema: bucket 은 필수입니다")
	}
	return bucket, nil
}

// --- v3 (InfluxQL) ---
//
// v3 는 Flux 를 지원하지 않는다. SQL 의 information_schema 도 이론상 가능하나
// 환경에 따라 미지원이므로, tsdbtags/client_v3.go 와 같이 InfluxQL 의 SHOW 계열을
// 쓴다. 응답 컬럼 이름은 SHOW 문마다 다르다.

const (
	// influxQLTagKeyColumn 은 SHOW TAG KEYS 결과의 컬럼 이름이다.
	influxQLTagKeyColumn = "tagKey"
	// influxQLTagValueColumn 은 SHOW TAG VALUES 결과의 컬럼 이름이다.
	influxQLTagValueColumn = "value"
	// influxQLFieldKeyColumn 은 SHOW FIELD KEYS 결과의 컬럼 이름이다.
	// SHOW FIELD KEYS 는 fieldKey 와 fieldType 두 컬럼을 돌려준다.
	influxQLFieldKeyColumn = "fieldKey"
)

// buildInfluxQLTagKeysQuery 는 D2 의 v3 쿼리를 만든다.
// tsdbtags/client_v3.go 의 ListTagKeys 복제본이다.
func buildInfluxQLTagKeysQuery(measurement string) (string, error) {
	if err := validateSchemaArgs("", measurement, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf(`SHOW TAG KEYS FROM "%s"`, escapeInfluxQLIdent(measurement)), nil
}

// buildInfluxQLTagValuesQuery 는 D3 의 v3 쿼리를 만든다.
// tsdbtags/client_v3.go 의 ListTagValues 복제본이다.
func buildInfluxQLTagValuesQuery(measurement, tagKey string) (string, error) {
	if err := validateSchemaArgs("", measurement, map[string]string{"tag key": tagKey}); err != nil {
		return "", err
	}
	return fmt.Sprintf(`SHOW TAG VALUES FROM "%s" WITH KEY = "%s"`,
		escapeInfluxQLIdent(measurement),
		escapeInfluxQLIdent(tagKey),
	), nil
}

// buildInfluxQLFieldKeysQuery 는 D4 의 v3 쿼리를 만든다.
// **신규 구현이다** — tsdbtags 에 대응물이 없다.
func buildInfluxQLFieldKeysQuery(measurement string) (string, error) {
	if err := validateSchemaArgs("", measurement, nil); err != nil {
		return "", err
	}
	return fmt.Sprintf(`SHOW FIELD KEYS FROM "%s"`, escapeInfluxQLIdent(measurement)), nil
}

// collectInfluxQLColumn 은 결과 행에서 지정 컬럼의 문자열을 모은다.
func collectInfluxQLColumn(rows []map[string]any, column string) []string {
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		v, ok := row[column]
		if !ok {
			continue
		}
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

// ListTagKeys 는 SHOW TAG KEYS FROM 으로 태그 키 목록을 조회한다(D2).
//
// bucket 인자는 v3 에서 무시된다 — database 는 클라이언트 생성 시 고정되며,
// 이는 기존 ListMeasurements 의 규약과 같다.
func (c *influxV3Client) ListTagKeys(ctx context.Context, _ string, measurement string) ([]string, error) {
	q, err := buildInfluxQLTagKeysQuery(measurement)
	if err != nil {
		return nil, err
	}
	rows, err := c.queryInfluxQL(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("influxdb v3 list tag keys (measurement=%q): %w", measurement, err)
	}
	return collectInfluxQLColumn(rows, influxQLTagKeyColumn), nil
}

// ListTagValues 는 SHOW TAG VALUES FROM ... WITH KEY = 로 태그 값 목록을 조회한다(D3).
func (c *influxV3Client) ListTagValues(ctx context.Context, _ string, measurement, tagKey string) ([]string, error) {
	q, err := buildInfluxQLTagValuesQuery(measurement, tagKey)
	if err != nil {
		return nil, err
	}
	rows, err := c.queryInfluxQL(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("influxdb v3 list tag values (measurement=%q, tag_key=%q): %w",
			measurement, tagKey, err)
	}
	return collectInfluxQLColumn(rows, influxQLTagValueColumn), nil
}

// ListFieldKeys 는 SHOW FIELD KEYS FROM 으로 필드 키 목록을 조회한다(D4).
func (c *influxV3Client) ListFieldKeys(ctx context.Context, _ string, measurement string) ([]string, error) {
	q, err := buildInfluxQLFieldKeysQuery(measurement)
	if err != nil {
		return nil, err
	}
	rows, err := c.queryInfluxQL(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("influxdb v3 list field keys (measurement=%q): %w", measurement, err)
	}
	return collectInfluxQLColumn(rows, influxQLFieldKeyColumn), nil
}
