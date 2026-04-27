package system

import (
	"context"
	"fmt"
	"time"
)

// InfluxQueryResult 는 Flux/InfluxQL 쿼리 결과의 단일 행을 나타낸다.
// SPEC-CHART-001 REQ-M3-02 에서 정의한 표준 응답 형식(entries[].{timestamp, value, labels})
// 으로의 변환 중간 단계에서 사용된다.
//
// Time 은 Flux 결과의 `_time` 컬럼을 직접 매핑한 것이다. `_time` 이 없으면
// Time.IsZero() 가 true 가 된다 (호출자가 fallback 을 적용할 수 있다).
//
// Labels 는 `_time` 과 `_value` 를 제외한 모든 컬럼을 문자열로 변환하여 담는다.
// Flux 의 `_measurement`, `_field`, `_start`, `_stop` 등 메타 컬럼과 태그 컬럼이 모두
// 이 맵에 포함되며, JSON 응답 단계에서 그대로 전달된다.
type InfluxQueryResult struct {
	Time   time.Time
	Value  any
	Labels map[string]string
}

// fluxTimeColumn 은 Flux 결과에서 시간 값을 담은 컬럼 이름이다.
const fluxTimeColumn = "_time"

// fluxValueColumn 은 Flux 결과에서 값 컬럼 이름이다.
const fluxValueColumn = "_value"

// InfluxFluxQueryer 는 InfluxDB 에이전트가 제공하는 쿼리 계약이다.
// HTTP 핸들러가 타입 단언을 통해 이 인터페이스로 에이전트를 검증한다.
type InfluxFluxQueryer interface {
	ExecuteFluxQuery(ctx context.Context, flux string) ([]InfluxQueryResult, error)
	ExecuteInfluxQLQuery(ctx context.Context, iql string) ([]InfluxQueryResult, error)
}

// 컴파일 타임 인터페이스 준수 체크.
var _ InfluxFluxQueryer = (*InfluxDBAgent)(nil)

// ExecuteFluxQuery 는 Flux 쿼리를 실행하고 결과를 InfluxQueryResult 배열로 반환한다.
// 내부적으로 InfluxClient.Query(ctx, q, "flux") 를 호출하고, 각 행 맵을
// InfluxQueryResult 로 변환한다.
//
// 에러 경로: InfluxClient 가 반환한 에러는 wrap 하여 전파한다.
// 빈 결과: 비-nil 이지만 길이 0 인 슬라이스를 반환한다.
func (a *InfluxDBAgent) ExecuteFluxQuery(ctx context.Context, flux string) ([]InfluxQueryResult, error) {
	return a.executeQuery(ctx, flux, "flux")
}

// ExecuteInfluxQLQuery 는 InfluxQL 쿼리를 실행한다.
// InfluxDB 2.x 에서는 내부적으로 Flux 와 동일한 QueryAPI 를 사용하지만,
// 명시적으로 `influxql` 언어 식별자를 전달하여 향후 버전별 분기에 대비한다.
func (a *InfluxDBAgent) ExecuteInfluxQLQuery(ctx context.Context, iql string) ([]InfluxQueryResult, error) {
	return a.executeQuery(ctx, iql, "influxql")
}

// executeQuery 는 Flux/InfluxQL 공용 실행 진입점이다.
// nil 이 반환되면 안되므로 빈 슬라이스로 정규화한다.
func (a *InfluxDBAgent) executeQuery(ctx context.Context, query, lang string) ([]InfluxQueryResult, error) {
	if a.client == nil {
		return nil, fmt.Errorf("influxdb: client is not initialized")
	}

	rows, err := a.client.Query(ctx, query, lang)
	if err != nil {
		return nil, fmt.Errorf("influxdb %s query: %w", lang, err)
	}

	results := make([]InfluxQueryResult, 0, len(rows))
	for _, row := range rows {
		results = append(results, mapFluxRow(row))
	}
	return results, nil
}

// mapFluxRow 는 Flux/InfluxQL 결과의 단일 행(map[string]any) 을
// InfluxQueryResult 로 변환한다.
//   - `_time` → Time (없으면 zero value)
//   - `_value` → Value (없으면 nil)
//   - 나머지 모든 키 → Labels (문자열로 변환)
func mapFluxRow(row map[string]any) InfluxQueryResult {
	var r InfluxQueryResult
	if t, ok := row[fluxTimeColumn]; ok {
		if tt, ok := t.(time.Time); ok {
			r.Time = tt
		}
	}
	if v, ok := row[fluxValueColumn]; ok {
		r.Value = v
	}

	r.Labels = make(map[string]string, len(row))
	for k, v := range row {
		if k == fluxTimeColumn || k == fluxValueColumn {
			continue
		}
		r.Labels[k] = stringifyLabelValue(v)
	}
	return r
}

// stringifyLabelValue 는 임의의 값을 레이블 문자열로 변환한다.
// 문자열은 그대로, 나머지는 fmt.Sprintf("%v", ...) 로 변환한다.
func stringifyLabelValue(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
