// @spec SPEC-TSDB-002 §2.6 (U6)
//
// InfluxDB 구조화 시리즈 질의의 요청 형상과 와이어 어휘를 정의한다.
//
// 응답 타입은 여기에 없다. 구조화 질의는 기존 평탄 응답(chartQueryResponse)을
// 그대로 재사용하며 세 번째 응답 형상을 도입하지 않는다(§2.16 #23 · §4.3).
package dto

// InfluxSeriesQueryRequest 는 POST /influxdb/{agent_name}/series/query 의 요청
// 바디다.
//
// 요청 1건이 시리즈 1개를 처리한다 — Store 의 요청 축과 같다(§2.6). N 개 시리즈를
// 가진 패널은 N 회 요청한다.
type InfluxSeriesQueryRequest struct {
	// Bucket 은 v2 의 bucket, v3 의 database 다. 빈 값이면 에이전트 기본값.
	Bucket string `json:"bucket,omitempty"`
	// Measurement 는 시리즈 키다. 필수.
	Measurement string `json:"measurement"`
	// Field 는 값 필드다. 필수 — "첫 번째 숫자 필드" 류의 폴백을 두지 않는다(§2.16 #4).
	Field string `json:"field"`
	// Tags 는 시리즈 태그 필터다.
	Tags map[string]string `json:"tags,omitempty"`
	// StartMs 는 조회 시작(포함)이다.
	StartMs int64 `json:"start_ms"`
	// EndMs 는 조회 끝(미포함)이다.
	EndMs int64 `json:"end_ms"`
	// IntervalMs 는 버킷 폭이며 0 보다 커야 한다.
	IntervalMs int64 `json:"interval_ms"`
	// Aggregation 은 집계 어휘다: min|max|average|first|last.
	Aggregation string `json:"aggregation"`
	// Fill 은 빈 버킷 채우기 전략이다: ""|null|zero|previous.
	Fill string `json:"fill,omitempty"`
}

// 집계 어휘(§2.7 정본 표의 왼쪽 열). 백엔드 함수 이름이 아니라 HTTP 요청이 쓰는
// 어휘이며, 백엔드 함수 이름으로의 매핑은 agent 계층이 소유한다.
const (
	// SeriesAggregationMin 은 구간 최솟값이다.
	SeriesAggregationMin = "min"
	// SeriesAggregationMax 는 구간 최댓값이다.
	SeriesAggregationMax = "max"
	// SeriesAggregationAverage 는 구간 산술평균이다. 매핑 표에서 유일하게
	// 백엔드 함수 이름과 어휘가 어긋나는 항목이다.
	SeriesAggregationAverage = "average"
	// SeriesAggregationFirst 는 구간의 첫 값이다.
	SeriesAggregationFirst = "first"
	// SeriesAggregationLast 는 구간의 마지막 값이다.
	SeriesAggregationLast = "last"
)

// fill 어휘(§2.7 fill 표의 왼쪽 열).
const (
	// SeriesFillNone 은 생략(빈 문자열)이며 빈 버킷을 방출하지 않는다.
	SeriesFillNone = ""
	// SeriesFillNull 은 빈 버킷을 null 로 방출한다.
	SeriesFillNull = "null"
	// SeriesFillZero 는 빈 버킷을 0 으로 방출한다.
	SeriesFillZero = "zero"
	// SeriesFillPrevious 는 빈 버킷을 직전 값으로 방출한다.
	SeriesFillPrevious = "previous"
	// SeriesFillAvg 는 프런트 계약(SeriesMatrixQuery.fill)에 남아 있으나 InfluxDB
	// 양쪽 백엔드 모두 대응물이 없다. 조용히 대체하지 않고 400 으로 거부한다.
	SeriesFillAvg = "avg"
)
