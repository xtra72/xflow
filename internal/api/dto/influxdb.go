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
//
// **예외는 GroupBy 다**(SPEC-TSDB-004). 그룹 축이 지정되면 요청 1건이 그룹마다
// 시리즈 하나씩, 여러 개를 돌려준다. 응답 형상은 그대로다 — 다중 시리즈는
// entries 를 평탄화하고 labels 로 구분하는 기존 규약(SPEC-STORE-004)이 이미
// 표현한다.
type InfluxSeriesQueryRequest struct {
	// Bucket 은 v2 의 bucket, v3 의 database 다. 빈 값이면 에이전트 기본값.
	Bucket string `json:"bucket,omitempty"`
	// Measurement 는 시리즈 키다. 필수.
	Measurement string `json:"measurement"`
	// Field 는 값 필드다. 필수 — "첫 번째 숫자 필드" 류의 폴백을 두지 않는다(§2.16 #4).
	Field string `json:"field"`
	// Tags 는 시리즈 태그 필터다.
	Tags map[string]string `json:"tags,omitempty"`
	// GroupBy 는 시리즈를 나눌 태그 키 목록이다(SPEC-TSDB-004 §2.1).
	//
	// 비어 있으면 정확 일치 모드이며 요청·응답이 본 축 도입 이전과 같다.
	// 비어 있지 않으면 지정한 키들의 값 조합마다 시리즈가 하나씩 생기고,
	// 각 엔트리의 labels 가 그 그룹의 실제 태그 값을 담는다.
	//
	// Tags 와 직교한다 — Tags 는 사전 필터, GroupBy 는 분할 축이다. 같은 키가
	// 양쪽에 오면 400 으로 거부한다(§2.8 UB1-3).
	//
	// **그룹 수에 상한이 없다**(§2.7). 응답은 절단되지 않으며 많아서 읽기 어려운
	// 문제는 열거 표면의 페이지네이션이 담당한다.
	GroupBy []string `json:"group_by,omitempty"`
	// GroupFilter 는 반환할 그룹을 **태그 값 조합 목록**으로 제한한다
	// (SPEC-TSDB-004 §2.7.1). 시리즈축 페이지네이션의 수단이다.
	//
	//	group_filter: [{"host":"a"}, {"host":"b"}]
	//
	// 각 페이지는 여전히 완결된 시간창이며 버킷 경계는 요청마다 동일하다 —
	// 시간축이 아니라 시리즈축을 나누기 때문이다.
	//
	// 조합 목록인 이유는 다중 키 때문이다. 키별 허용값 맵으로 두면 데카르트
	// 곱이 되어 실재하지 않는 조합까지 선택한다.
	//
	// 키가 하나도 없는 항목은 400 으로 거부한다 — 빈 조합은 "모든 그룹" 을
	// 뜻하게 되어 페이지 선택을 조용히 무효화한다.
	GroupFilter []map[string]string `json:"group_filter,omitempty"`
	// StartMs 는 조회 시작(포함)이다.
	StartMs int64 `json:"start_ms"`
	// EndMs 는 조회 끝(미포함)이다.
	EndMs int64 `json:"end_ms"`
	// IntervalMs 는 버킷 폭이며 0 보다 커야 한다.
	IntervalMs int64 `json:"interval_ms"`
	// Aggregation 은 집계 어휘다: min|max|average|first|last|sum|count.
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
	// SeriesAggregationSum 은 구간 값들의 합이다.
	SeriesAggregationSum = "sum"
	// SeriesAggregationCount 는 구간의 표본 개수다. 결과 단위가 원본 필드의
	// 단위가 아니라 "개" 인 유일한 집계다.
	SeriesAggregationCount = "count"
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

// --- 시리즈 열거(디스커버리 D5) 응답 (@spec SPEC-TSDB-003 §2.2 (U2)) ---
//
// 여기에는 응답 타입이 있다. 구조화 질의(위)와 반대인 이유는 열거가 **기존 응답
// 형상에 대응물이 없기** 때문이다 — 평탄 entries 는 (시각, 값) 열이고, 열거는
// (태그 집합, field 목록) 열이다. 있는 형상에 억지로 태우면 소비자가 두 뜻을
// 구분할 수 없다.
//
// system.EnumeratedSeries 를 그대로 직렬화하지 않고 여기에 다시 두는 이유는
// 계층 경계다 — dto 는 agent 계층을 import 하지 않으며, 와이어 형상이 에이전트
// 내부 타입의 json 태그 변경에 딸려 흔들려서는 안 된다.

// InfluxSeriesEnumWindow 는 서버가 **실제로 사용한** 탐색 창이다(§2.8).
//
// 요청이 창을 생략했을 때 무엇이 적용됐는지 드러내는 것이 이 필드의 목적이다.
// 창을 코드에 숨기면(UB1-6) 사용자는 "시리즈가 삭제됨"과 "탐색 창 밖"을 구분할
// 수 없고, 전자로 오해하면 잘못된 조치를 한다.
type InfluxSeriesEnumWindow struct {
	// StartMs 는 탐색 창 시작(포함)이다.
	StartMs int64 `json:"start_ms"`
	// EndMs 는 탐색 창 끝(미포함)이다.
	EndMs int64 `json:"end_ms"`
}

// InfluxEnumeratedSeries 는 열거된 시리즈 1개다.
type InfluxEnumeratedSeries struct {
	// Tags 는 실재하는 태그 집합 1벌이다. 키 없는 시리즈는 빈 객체({})이며
	// null 이 아니다 — 프런트가 분기 없이 태그 맵을 읽을 수 있어야 한다.
	Tags map[string]string `json:"tags"`
	// Fields 는 그 태그 집합에서 관측된 field 키 목록이며 사전순이다.
	Fields []string `json:"fields"`
}

// InfluxSeriesEnumResponse 는 GET /influxdb/{agent_name}/series 의 응답 데이터다.
//
// dto.NewSuccessResponse 봉투 안에 담긴다 — 디스커버리 D1~D4 와 같은 규약이며
// 신규 봉투를 만들지 않는다.
type InfluxSeriesEnumResponse struct {
	// Series 는 태그 직렬화 오름차순으로 정렬된 시리즈 목록이다(UB1-16).
	// 절단이 발생할 때 폴링마다 다른 부분집합이 잘리면 사용자가 고른 시리즈가
	// 목록에서 사라졌다 나타났다 한다.
	Series []InfluxEnumeratedSeries `json:"series"`
	// FieldExact 는 Fields 가 정확한 관측치인지(true) 하한/근사인지(false) 다.
	// 백엔드마다 다르며(v2 true · v3 false), 프런트 능력 표가 아니라 **이 값**이
	// 정본이다(§2.6).
	FieldExact bool `json:"field_exact"`
	// Count 는 Series 의 길이다.
	Count int `json:"count"`
	// Truncated 는 상한에 걸려 잘렸는지다(§2.7). 절단을 조용히 수행하지 않는다 —
	// UI 가 이 신호로 배너와 좁히는 방법을 함께 제시한다.
	Truncated bool `json:"truncated"`
	// Window 는 서버가 실제로 사용한 탐색 창이다.
	Window InfluxSeriesEnumWindow `json:"window"`
}
