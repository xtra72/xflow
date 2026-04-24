// @spec SPEC-WEB-005
package handler

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"sort"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// 서버측 집계 상한: 응답 폭주를 방지하기 위한 안전장치이다.
// 프런트엔드 경고 임계치(5,000)와는 별개의 하드 리미트이다.
const maxAggregationBuckets = 100_000

// 유효한 집계 연산자 집합.
const (
	aggregationMin = "min"
	aggregationMax = "max"
	aggregationAvg = "avg"
)

// AgentLookup 은 에이전트 이름으로 에이전트를 조회하기 위한 최소 인터페이스이다.
// 실제 runtime 에서는 *agent.DefaultManager 가 이 인터페이스를 만족한다.
// 테스트에서는 경량 페이크로 대체 가능하다.
type AgentLookup interface {
	List() []agent.Agent
}

// storeHistoryQueryer 는 Store 시스템 에이전트가 제공해야 하는 쿼리 계약이다.
// system.UserStoreAgent 가 이 인터페이스를 만족한다. 핸들러는 타입 단언으로 이 인터페이스를
// 요구하므로, 호출 대상이 Store 에이전트가 아니면 즉시 400 에러로 거부할 수 있다.
type storeHistoryQueryer interface {
	QueryHistory(ctx context.Context, namespace, key string, q system.HistoryQuery) ([]system.HistoryEntry, error)
}

type storeKeyLister interface {
	ListStoreKeys(ctx context.Context, namespace, pattern string) ([]string, error)
}

// StoreQueryHandler 는 SPEC-CHART-001 REQ-M3-01 를 구현한다.
//
// 라우트:
//
//	POST /store/{agent_name}/query
type StoreQueryHandler struct {
	agents AgentLookup
	logger *slog.Logger
}

// NewStoreQueryHandler 는 새 StoreQueryHandler 를 생성한다.
func NewStoreQueryHandler(agents AgentLookup, logger *slog.Logger) *StoreQueryHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &StoreQueryHandler{agents: agents, logger: logger}
}

// RegisterRoutes 는 Store 쿼리 라우트를 등록한다.
func (h *StoreQueryHandler) RegisterRoutes(g *api.RouteGroup) {
	g.POST("/store/{agent_name}/query", h.Query)
	g.GET("/store/{agent_name}/keys", h.ListKeys)
}

// storeQueryRequest 는 REQ-M3-01 요청 바디 형식이다.
//
// mode 에 따라 다음 필드가 필요하다:
//   - latest      : (none)
//   - last_n      : count > 0
//   - duration    : duration_sec > 0
//   - time_range  : start_ms > 0, end_ms > 0 (end >= start)
//   - since_n     : start_ms > 0 (since), count > 0
//
// namespace 는 선택적이다. 생략하면 "default" 로 해석된다.
// since_version 은 향후 이벤트 기반 구독 확장을 위해 예약된 필드이며 현재는 무시된다.
//
// SPEC-WEB-005 확장: 서버측 버킷팅/집계
//   - interval_ms > 0 그리고 aggregation ∈ {"min","max","avg"} 이면 서버에서
//     버킷 단위로 집계한 결과를 반환한다. 집계는 time_range / duration 모드에서만
//     지원된다 (명확한 시작 지점이 필요하기 때문).
//   - interval_ms 또는 aggregation 둘 중 하나라도 없으면 레거시 동작(원시 엔트리 반환).
type storeQueryRequest struct {
	Key          string `json:"key"`
	Mode         string `json:"mode"`
	Count        int    `json:"count,omitempty"`
	DurationSec  int    `json:"duration_sec,omitempty"`
	StartMs      int64  `json:"start_ms,omitempty"`
	EndMs        int64  `json:"end_ms,omitempty"`
	SinceVersion int64  `json:"since_version,omitempty"`
	Namespace    string `json:"namespace,omitempty"`

	// SPEC-WEB-005: 서버측 집계 파라미터 (선택).
	IntervalMs  int64  `json:"interval_ms,omitempty"`
	Aggregation string `json:"aggregation,omitempty"`
}

// chartQueryEntry 는 표준 응답(REQ-M3-04) 의 entries 항목이다.
type chartQueryEntry struct {
	Timestamp int64             `json:"timestamp"`
	Value     any               `json:"value"`
	Labels    map[string]string `json:"labels,omitempty"`
}

// chartQueryResponse 는 표준 응답(REQ-M3-04) 의 data 부분이다.
type chartQueryResponse struct {
	Entries   []chartQueryEntry `json:"entries"`
	Count     int               `json:"count"`
	Truncated bool              `json:"truncated"`
}

// Query 는 Store 시스템 에이전트의 QueryHistory 를 HTTP 로 노출한다.
func (h *StoreQueryHandler) Query(ctx api.Context) error {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return api.ErrBadRequest.WithMessage("agent_name is required")
	}

	var req storeQueryRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.Key == "" {
		return api.ErrBadRequest.WithMessage("key is required")
	}

	q, err := buildHistoryQuery(&req)
	if err != nil {
		return api.ErrBadRequest.WithMessage(err.Error())
	}

	// SPEC-WEB-005: 집계 파라미터 검증. 레거시 경로와 집계 경로를 여기서 분기한다.
	aggEnabled, err := validateAggregationParams(&req)
	if err != nil {
		return api.ErrBadRequest.WithMessage(err.Error())
	}

	// 에이전트 조회 (name 기반, 실패 시 404).
	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	// 타입 단언: Store 계열 에이전트만 QueryHistory 를 구현한다.
	storeAgent, ok := ag.(storeHistoryQueryer)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_a_store_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_a_store_agent"})
	}

	entries, err := storeAgent.QueryHistory(ctx.Context(), req.Namespace, req.Key, q)
	if err != nil {
		return api.MapDomainError(err)
	}

	// 집계 경로: time_range / duration 에서만 도달한다 (validateAggregationParams 가 보장).
	if aggEnabled {
		originMs, aerr := resolveAggregationOriginMs(&req, q)
		if aerr != nil {
			return api.ErrBadRequest.WithMessage(aerr.Error())
		}
		bucketed := bucketAggregate(entries, originMs, req.IntervalMs, req.Aggregation)
		resp := chartQueryResponse{
			Entries:   bucketed,
			Count:     len(bucketed),
			Truncated: false,
		}
		return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
	}

	resp := chartQueryResponse{
		Entries:   mapHistoryEntriesToDTO(entries),
		Count:     len(entries),
		Truncated: false,
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// validateAggregationParams 는 SPEC-WEB-005 집계 파라미터를 검증한다.
//
// 반환값:
//   - enabled=true : 유효한 (interval_ms, aggregation) 조합이 주어졌고 집계를 수행해야 한다
//   - enabled=false: 둘 중 하나라도 없거나 둘 다 비어있음 (레거시 경로)
//   - err != nil   : 값이 잘못됨 (400 으로 반환해야 함)
//
// 규칙:
//   - aggregation 이 빈 문자열이 아닐 때 반드시 "min"/"max"/"avg" 여야 한다
//   - interval_ms 가 0 이 아닐 때 반드시 양수여야 한다
//   - 집계를 수행하려면 mode 가 time_range 또는 duration 이어야 한다
//   - 계산된 버킷 수가 상한(maxAggregationBuckets)을 넘으면 거부한다
func validateAggregationParams(req *storeQueryRequest) (bool, error) {
	// aggregation 값 자체가 잘못된 경우 (interval_ms 와 무관하게 거부).
	if req.Aggregation != "" &&
		req.Aggregation != aggregationMin &&
		req.Aggregation != aggregationMax &&
		req.Aggregation != aggregationAvg {
		return false, errInvalidField("invalid aggregation: " + req.Aggregation)
	}

	// interval_ms 가 명시적으로 주어졌는데 (0이 아님) 양수가 아니면 거부.
	// 0 은 "지정 안됨" 으로 취급한다 (omitempty 규약).
	if req.IntervalMs < 0 {
		return false, errInvalidField("interval_ms must be > 0")
	}

	// 한쪽만 주어지면 레거시 경로. (interval_ms 가 0 이거나 aggregation 이 비었을 때)
	if req.IntervalMs <= 0 || req.Aggregation == "" {
		return false, nil
	}

	// 집계 수행 결정. 모드 제약 검사.
	mode := system.QueryMode(req.Mode)
	if mode != system.QueryModeTimeRange && mode != system.QueryModeDuration {
		return false, errInvalidField("aggregation requires mode=time_range or duration")
	}

	// 버킷 수 폭주 방지.
	if mode == system.QueryModeTimeRange {
		span := req.EndMs - req.StartMs
		if span > 0 {
			numBuckets := span / req.IntervalMs
			if span%req.IntervalMs != 0 {
				numBuckets++
			}
			if numBuckets > maxAggregationBuckets {
				return false, errInvalidField("too many buckets (max 100000)")
			}
		}
	} else { // duration
		spanMs := int64(req.DurationSec) * 1000
		if spanMs > 0 {
			numBuckets := spanMs / req.IntervalMs
			if spanMs%req.IntervalMs != 0 {
				numBuckets++
			}
			if numBuckets > maxAggregationBuckets {
				return false, errInvalidField("too many buckets (max 100000)")
			}
		}
	}

	return true, nil
}

// resolveAggregationOriginMs 는 버킷 원점(epoch ms)을 결정한다.
//   - time_range : req.StartMs 를 사용
//   - duration   : buildHistoryQuery 에서 설정된 q.From 이 있으면 그것을 사용하고,
//     없으면 (now - duration) 을 계산한다. 현재 system.HistoryQuery 는 duration 모드에서
//     From 을 채우지 않지만 향후 변경에 대비하여 q.From 이 존재하면 우선한다.
func resolveAggregationOriginMs(req *storeQueryRequest, q system.HistoryQuery) (int64, error) {
	mode := system.QueryMode(req.Mode)
	switch mode {
	case system.QueryModeTimeRange:
		return req.StartMs, nil
	case system.QueryModeDuration:
		if !q.From.IsZero() {
			return q.From.UnixMilli(), nil
		}
		return time.Now().Add(-q.Duration).UnixMilli(), nil
	default:
		// validateAggregationParams 에서 이미 거부되었어야 한다.
		return 0, errInvalidField("aggregation requires mode=time_range or duration")
	}
}

// toFloat64 는 임의의 값(interface{})을 float64 로 변환한다.
// 숫자가 아니거나 NaN/Infinity 인 경우 ok=false 를 반환한다.
// JSON 디코딩 결과는 일반적으로 float64 이지만, 내부 호출자가 int/int64/float32 등을
// 직접 넣을 수도 있으므로 대표적인 숫자 타입을 모두 받아준다.
func toFloat64(v any) (float64, bool) {
	var f float64
	switch x := v.(type) {
	case float64:
		f = x
	case float32:
		f = float64(x)
	case int:
		return float64(x), true
	case int8:
		return float64(x), true
	case int16:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint8:
		return float64(x), true
	case uint16:
		return float64(x), true
	case uint32:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return 0, false
	}
	return f, true
}

// bucketAggregate 는 entries 를 intervalMs 크기의 버킷으로 묶어 aggregation 연산을 적용한다.
//
// 규칙:
//   - 각 엔트리는 floor((ts_ms - originMs) / intervalMs) 버킷으로 배치된다
//   - ts_ms < originMs 인 엔트리는 버킷 인덱스가 음수이므로 제외된다
//   - 비숫자/NaN/Infinity 값은 스킵한다 (평균의 분모에 포함되지 않음)
//   - 유효 숫자가 0 개인 버킷은 응답에서 생략된다 (페이로드 축소)
//   - 각 버킷의 timestamp 는 bucket_start_ms = originMs + bucketIdx*intervalMs 이다
//   - 결과는 timestamp 오름차순이다
func bucketAggregate(
	entries []system.HistoryEntry,
	originMs int64,
	intervalMs int64,
	aggregation string,
) []chartQueryEntry {
	if intervalMs <= 0 || len(entries) == 0 {
		return []chartQueryEntry{}
	}

	// 버킷별 집계 상태. avg 는 sum+count, min/max 는 단일 값.
	type bucketState struct {
		sum   float64
		count int
		min   float64
		max   float64
	}
	// map 키는 버킷 인덱스 (음수 제외). 일반적으로 연속되지만 희소할 수도 있으므로 map 사용.
	buckets := make(map[int64]*bucketState)

	for _, e := range entries {
		tsMs := e.Timestamp.UnixMilli()
		if tsMs < originMs {
			continue
		}
		f, ok := toFloat64(e.Value)
		if !ok {
			continue
		}
		idx := (tsMs - originMs) / intervalMs
		st, exists := buckets[idx]
		if !exists {
			st = &bucketState{sum: f, count: 1, min: f, max: f}
			buckets[idx] = st
			continue
		}
		st.sum += f
		st.count++
		if f < st.min {
			st.min = f
		}
		if f > st.max {
			st.max = f
		}
	}

	if len(buckets) == 0 {
		return []chartQueryEntry{}
	}

	// 인덱스 오름차순 정렬 후 결과 구성.
	idxs := make([]int64, 0, len(buckets))
	for k := range buckets {
		idxs = append(idxs, k)
	}
	// 작은 수의 버킷에 대한 단순 삽입 정렬 대신 표준 sort 사용.
	sortInt64Asc(idxs)

	out := make([]chartQueryEntry, 0, len(idxs))
	for _, idx := range idxs {
		st := buckets[idx]
		if st.count == 0 {
			continue
		}
		var value float64
		switch aggregation {
		case aggregationMin:
			value = st.min
		case aggregationMax:
			value = st.max
		case aggregationAvg:
			value = st.sum / float64(st.count)
		default:
			// validateAggregationParams 에서 이미 거부되었어야 하므로 방어 코드.
			continue
		}
		out = append(out, chartQueryEntry{
			Timestamp: originMs + idx*intervalMs,
			Value:     value,
		})
	}
	return out
}

// sortInt64Asc 는 int64 슬라이스를 오름차순 정렬한다.
// 집계 버킷의 시간순 정렬에 사용된다.
func sortInt64Asc(s []int64) {
	sort.Slice(s, func(i, j int) bool { return s[i] < s[j] })
}

// buildHistoryQuery 는 요청 DTO 를 system.HistoryQuery 로 변환한다.
// 모드별 필수 필드가 누락되면 명시적 에러를 반환한다.
func buildHistoryQuery(req *storeQueryRequest) (system.HistoryQuery, error) {
	mode := system.QueryMode(req.Mode)
	q := system.HistoryQuery{Mode: mode, Count: req.Count}

	switch mode {
	case system.QueryModeLatest:
		// 추가 필드 없음
	case system.QueryModeLastN:
		if req.Count <= 0 {
			return q, errInvalidField("count must be > 0 for mode last_n")
		}
	case system.QueryModeDuration:
		if req.DurationSec <= 0 {
			return q, errInvalidField("duration_sec must be > 0 for mode duration")
		}
		q.Duration = time.Duration(req.DurationSec) * time.Second
	case system.QueryModeTimeRange:
		if req.StartMs <= 0 || req.EndMs <= 0 {
			return q, errInvalidField("start_ms and end_ms must be > 0 for mode time_range")
		}
		if req.EndMs < req.StartMs {
			return q, errInvalidField("end_ms must be >= start_ms for mode time_range")
		}
		q.From = time.UnixMilli(req.StartMs)
		q.To = time.UnixMilli(req.EndMs)
	case system.QueryModeSinceN:
		if req.StartMs <= 0 {
			return q, errInvalidField("start_ms must be > 0 for mode since_n (represents 'since' time)")
		}
		if req.Count <= 0 {
			return q, errInvalidField("count must be > 0 for mode since_n")
		}
		q.Since = time.UnixMilli(req.StartMs)
	default:
		return q, errInvalidField("invalid mode: " + req.Mode)
	}
	return q, nil
}

// errInvalidField 는 400 BadRequest 원인을 나타내는 sentinel error 래퍼이다.
type invalidFieldError struct{ msg string }

func (e *invalidFieldError) Error() string { return e.msg }
func errInvalidField(msg string) error     { return &invalidFieldError{msg: msg} }

// mapHistoryEntriesToDTO 는 HistoryEntry 를 표준 응답의 chartQueryEntry 로 변환한다.
// Timestamp 는 epoch ms (int64) 로 변환되며, HistoryEntry 에는 Labels 정보가 없으므로
// Labels 는 nil 로 둔다 (omitempty 에 의해 JSON 에서 누락된다).
func mapHistoryEntriesToDTO(entries []system.HistoryEntry) []chartQueryEntry {
	out := make([]chartQueryEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, chartQueryEntry{
			Timestamp: e.Timestamp.UnixMilli(),
			Value:     e.Value,
		})
	}
	return out
}

// ListKeys 는 Store 에이전트의 키 목록을 반환한다.
//
//	GET /store/{agent_name}/keys?namespace=default&pattern=*
func (h *StoreQueryHandler) ListKeys(ctx api.Context) error {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return api.ErrBadRequest.WithMessage("agent_name is required")
	}

	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	keyLister, ok := ag.(storeKeyLister)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_a_store_agent: " + agentName)
	}

	namespace := ctx.Query("namespace")
	pattern := ctx.Query("pattern")

	keys, err := keyLister.ListStoreKeys(ctx.Context(), namespace, pattern)
	if err != nil {
		return api.MapDomainError(err)
	}
	if keys == nil {
		keys = []string{}
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"keys":  keys,
		"count": len(keys),
	}))
}

// findAgentByName 은 AgentLookup.List() 를 순회하여 이름이 일치하는 첫 에이전트를 반환한다.
// 없으면 nil 을 반환한다.
func findAgentByName(lookup AgentLookup, name string) agent.Agent {
	if lookup == nil {
		return nil
	}
	for _, a := range lookup.List() {
		if a.Name() == name {
			return a
		}
	}
	return nil
}
