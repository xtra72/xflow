package dto

// TSDBWriteRequest 는 TSDB 데이터 기록 요청이다.
type TSDBWriteRequest struct {
	Points []TSDBWritePoint `json:"points"`
}

// TSDBWritePoint 는 단일 쓰기 포인트이다.
type TSDBWritePoint struct {
	Measurement string            `json:"measurement"`
	Tags        map[string]string `json:"tags,omitempty"`
	Fields      map[string]any    `json:"fields"`
	Timestamp   *string           `json:"timestamp,omitempty"` // RFC3339Nano 또는 unix nano
}

// TSDBQueryRequest 는 TSDB 쿼리 요청이다.
type TSDBQueryRequest struct {
	SeriesKey   string            `json:"series_key,omitempty"`
	Measurement string            `json:"measurement,omitempty"`
	Tags        map[string]string `json:"tags,omitempty"`
	Start       *string           `json:"start,omitempty"`       // RFC3339Nano
	End         *string           `json:"end,omitempty"`         // RFC3339Nano
	Field       string            `json:"field,omitempty"`
	Aggregation string            `json:"aggregation,omitempty"` // min,max,avg,sum,count,first,last
	Bucket      string            `json:"bucket,omitempty"`      // duration 문자열: "5m", "1h"
	Limit       int               `json:"limit,omitempty"`
}

// TSDBWriteResponse 는 쓰기 결과 응답이다.
type TSDBWriteResponse struct {
	Written int `json:"written"`
}

// TSDBQueryResponse 는 쿼리 결과 응답이다.
type TSDBQueryResponse struct {
	Results []TSDBSeriesResult `json:"results"`
}

// TSDBSeriesResult 는 시리즈별 쿼리 결과이다.
type TSDBSeriesResult struct {
	SeriesKey string          `json:"series_key"`
	Points    []TSDBDataPoint `json:"points"`
	Stats     *TSDBQueryStats `json:"stats,omitempty"`
}

// TSDBDataPoint 는 단일 데이터 포인트의 응답 표현이다.
type TSDBDataPoint struct {
	Timestamp string         `json:"timestamp"` // RFC3339Nano
	Fields    map[string]any `json:"fields"`
}

// TSDBQueryStats 는 쿼리 실행 통계이다.
type TSDBQueryStats struct {
	ScannedPoints  int    `json:"scanned_points"`
	ReturnedPoints int    `json:"returned_points"`
	ExecutionTime  string `json:"execution_time"` // duration 문자열
}

// TSDBStatsResponse 는 TSDB 통계 응답이다.
type TSDBStatsResponse struct {
	SeriesCount int    `json:"series_count"`
	TotalPoints int64  `json:"total_points"`
	MemoryBytes int64  `json:"memory_bytes"`
	MemoryHuman string `json:"memory_human"`           // "123.4 MB"
	OldestPoint string `json:"oldest_point,omitempty"` // RFC3339Nano
	NewestPoint string `json:"newest_point,omitempty"` // RFC3339Nano
}

// TSDBSeriesListResponse 는 시리즈 목록 응답이다.
//
// Pagination 필드는 요청에 page/size 파라미터가 포함된 경우에만 직렬화된다.
// 파라미터가 없으면 기존 응답 포맷 ({"series": [...], "count": N}) 을 그대로 유지하여
// 하위 호환성을 보장한다 (omitempty).
//
// PaginationMeta 는 response.go 에 정의된 공용 타입을 재사용한다.
// @spec SPEC-WEB-005
type TSDBSeriesListResponse struct {
	Series     []string        `json:"series"`
	Count      int             `json:"count"`
	Pagination *PaginationMeta `json:"pagination,omitempty"`
}
