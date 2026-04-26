// @spec SPEC-WEB-005
// @spec SPEC-STORE-003
package handler

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strings"
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

// @spec SPEC-STORE-003
// storeKeyTagLister 는 정적 키 태그 메타데이터를 노출하는 에이전트 계약이다.
// system.UserStoreAgent 가 이 인터페이스를 만족한다. 핸들러는 type 단언으로
// 옵셔널하게 태그 정보를 수집한다. 미구현 에이전트는 기존 동작(태그 없음)으로 폴백된다.
type storeKeyTagLister interface {
	storeKeyLister
	// KeyTags 는 사용자 관점 key → 태그 맵 전체를 반환한다 (복사본).
	// 정적 키가 없으면 빈 맵을 반환한다.
	KeyTags(ctx context.Context) (map[string]map[string]string, error)
	// StaticTagPairs 는 태그 key → 정렬된 unique value 목록을 반환한다.
	// /tags 엔드포인트 전용 집계 헬퍼이다.
	StaticTagPairs() map[string][]string
}

// @spec SPEC-STORE-003
// storeResetter 는 reset 엔드포인트(DELETE /keys, DELETE /keys/{key}) 가 요구하는 에이전트 계약이다.
// 정책(정적 키 → 히스토리만 / 동적 키 → 엔트리 삭제) 는 핸들러가 IsStaticKey 결과로 분기한다.
// system.UserStoreAgent 가 이 인터페이스를 만족한다.
type storeResetter interface {
	storeKeyLister
	// ClearHistory 는 정적 키의 히스토리만 비우고 엔트리는 보존한다.
	// 키가 존재하지 않으면 system.ErrKeyNotFound 를 반환한다.
	ClearHistory(ctx context.Context, namespace, key string) error
	// DeleteEntry 는 동적 키의 엔트리(값+히스토리) 를 모두 삭제한다.
	// 키가 존재하지 않더라도 에러를 반환하지 않는다.
	DeleteEntry(ctx context.Context, namespace, key string) error
	// IsStaticKey 는 사용자 관점 key 가 정적 키 목록에 정의되어 있는지 검사한다.
	IsStaticKey(key string) bool
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
	// @spec SPEC-STORE-003
	g.GET("/store/{agent_name}/tags", h.ListTags)
	// @spec SPEC-STORE-003: reset 엔드포인트.
	//   DELETE /store/{agent_name}/keys/{key} → 단일 키 reset (정책 분기)
	//   DELETE /store/{agent_name}/keys       → 전체 키 reset (벌크)
	g.DELETE("/store/{agent_name}/keys/{key}", h.ResetKey)
	g.DELETE("/store/{agent_name}/keys", h.ResetAll)
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
//	GET /store/{agent_name}/keys?tag=room:1&tag=type:temperature
//
// @spec SPEC-STORE-003
//   - 응답에 정적 키의 태그 맵을 포함한다 (정적 키가 하나도 없으면 tags 필드 생략).
//   - ?tag=key:value 쿼리(다중 허용, AND 조건)로 정적 키를 필터링한다.
//   - 잘못된 tag 형식은 HTTP 400 과 'invalid tag format: expected key:value' 메시지.
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

	// @spec SPEC-STORE-003: 태그 필터 쿼리 파싱 및 검증 (에이전트 조회 전에 해도 무방하지만
	// not_a_store_agent 분류를 유지하기 위해 타입 단언 뒤에서 수행한다).
	tagFilters, ferr := parseTagFilters(ctx.QueryValues("tag"))
	if ferr != nil {
		return api.ErrBadRequest.WithMessage(ferr.Error())
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

	// 정적 태그 메타데이터가 사용 가능한 경우에만 tags/필터를 적용한다.
	var staticTags map[string]map[string]string
	if tagLister, okt := ag.(storeKeyTagLister); okt {
		kt, kerr := tagLister.KeyTags(ctx.Context())
		if kerr != nil {
			return api.MapDomainError(kerr)
		}
		staticTags = kt
	}

	// 태그 필터 적용 (tagFilters 가 비어있지 않을 때만).
	if len(tagFilters) > 0 {
		keys = filterKeysByTags(keys, staticTags, tagFilters)
	}

	// 응답 tags 맵 구성: 결과 keys 중 정적 태그가 있는 키만 포함.
	out := map[string]any{
		"keys":  keys,
		"count": len(keys),
	}
	if tagsPayload := buildTagsPayload(keys, staticTags); tagsPayload != nil {
		out["tags"] = tagsPayload
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(out))
}

// @spec SPEC-STORE-003
// ListTags 는 정적 키 태그의 (key → 정렬된 unique value 목록) 페어를 반환한다.
//
//	GET /store/{agent_name}/tags
//
// 응답: {"pairs": [{"key": "room", "values": ["1","2"]}, ...]}
// pairs 배열의 순서는 key 오름차순이다. 정적 키가 없으면 빈 배열을 반환한다.
func (h *StoreQueryHandler) ListTags(ctx api.Context) error {
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

	tagLister, ok := ag.(storeKeyTagLister)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_a_store_agent: " + agentName)
	}

	pairs := tagLister.StaticTagPairs()
	// 정렬된 출력: key 오름차순.
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	type pair struct {
		Key    string   `json:"key"`
		Values []string `json:"values"`
	}
	out := make([]pair, 0, len(keys))
	for _, k := range keys {
		out = append(out, pair{Key: k, Values: pairs[k]})
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"pairs": out,
	}))
}

// @spec SPEC-STORE-003
// tagFilter 는 단일 tag=key:value 쿼리 파라미터이다.
type tagFilter struct {
	key   string
	value string
}

// parseTagFilters 는 ?tag= 반복 파라미터를 tagFilter 슬라이스로 변환한다.
// 각 값은 SplitN(":", 2) 로 해석되어 `key:value:extra` 같은 value 내 콜론을 허용한다.
// 콜론이 없는 입력은 "invalid tag format: expected key:value" 에러를 반환한다.
// 빈 슬라이스 입력은 nil 을 반환한다 (필터 미적용 신호).
func parseTagFilters(raw []string) ([]tagFilter, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]tagFilter, 0, len(raw))
	for _, v := range raw {
		idx := strings.IndexByte(v, ':')
		if idx < 0 {
			return nil, errInvalidField("invalid tag format: expected key:value")
		}
		out = append(out, tagFilter{key: v[:idx], value: v[idx+1:]})
	}
	return out, nil
}

// @spec SPEC-STORE-003
// filterKeysByTags 는 모든 tagFilter 쌍을 포함(AND)하는 key 만 남긴다.
// 정적 태그가 없는 key 는 필터 기준을 만족할 수 없으므로 제외된다.
// 입력 keys 순서는 보존된다.
func filterKeysByTags(
	keys []string,
	staticTags map[string]map[string]string,
	filters []tagFilter,
) []string {
	if len(filters) == 0 {
		return keys
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		tags, ok := staticTags[k]
		if !ok {
			continue
		}
		match := true
		for _, f := range filters {
			if tags[f.key] != f.value {
				match = false
				break
			}
		}
		if match {
			out = append(out, k)
		}
	}
	return out
}

// @spec SPEC-STORE-003
// buildTagsPayload 는 응답 tags 필드를 구성한다.
// 결과 keys 중 정적 태그가 존재하는 key 만 포함한 맵을 반환한다.
// 포함할 항목이 하나도 없으면 nil 을 반환 (핸들러가 응답 필드를 생략).
func buildTagsPayload(
	keys []string,
	staticTags map[string]map[string]string,
) map[string]map[string]string {
	if len(staticTags) == 0 {
		return nil
	}
	out := make(map[string]map[string]string)
	for _, k := range keys {
		tags, ok := staticTags[k]
		if !ok || len(tags) == 0 {
			continue
		}
		copied := make(map[string]string, len(tags))
		for tk, tv := range tags {
			copied[tk] = tv
		}
		out[k] = copied
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// @spec SPEC-STORE-003
// ResetKey 는 단일 키 reset 을 처리한다.
//
//	DELETE /store/{agent_name}/keys/{key}?namespace=default
//
// 정책:
//   - 정적 키(IsStaticKey=true)  → ClearHistory(엔트리 보존, 히스토리만 비움)
//     응답: action=history_cleared
//   - 동적 키(IsStaticKey=false) → DeleteEntry(엔트리+히스토리 모두 삭제)
//     응답: action=entry_deleted
//
// 키 경로 파라미터는 url.PathUnescape 로 디코딩되어 콜론(%3A) 등 특수문자가 안전히 처리된다.
// 정적 키에 대해 ClearHistory 가 ErrKeyNotFound 를 반환하면 404 로 응답한다.
// (동적 키에 대해 DeleteEntry 는 키 부재를 에러로 보고하지 않으므로 항상 200 entry_deleted.)
func (h *StoreQueryHandler) ResetKey(ctx api.Context) error {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return api.ErrBadRequest.WithMessage("agent_name is required")
	}

	rawKey := ctx.Param("key")
	if rawKey == "" {
		return api.ErrBadRequest.WithMessage("key is required")
	}
	// 콜론/슬래시 등을 안전하게 처리하기 위해 명시적으로 디코딩한다.
	// %2F 등이 path 세그먼트에 포함될 수 없으므로 chi 라우터 단계에서 이미 막혀있지만,
	// %3A(콜론) 같은 안전한 인코딩은 여기서 풀어준다.
	decodedKey, derr := url.PathUnescape(rawKey)
	if derr != nil {
		return api.ErrBadRequest.WithMessage("invalid key encoding")
	}

	namespace := ctx.Query("namespace")

	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	resetter, ok := ag.(storeResetter)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_a_store_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_a_store_agent"})
	}

	var action string
	if resetter.IsStaticKey(decodedKey) {
		if err := resetter.ClearHistory(ctx.Context(), namespace, decodedKey); err != nil {
			if errors.Is(err, system.ErrKeyNotFound) {
				return api.ErrNotFound.
					WithMessage("key_not_found: " + decodedKey).
					WithDetails(map[string]string{"error": "key_not_found"})
			}
			return api.MapDomainError(err)
		}
		action = "history_cleared"
	} else {
		if err := resetter.DeleteEntry(ctx.Context(), namespace, decodedKey); err != nil {
			// Delete 는 키가 없어도 에러를 내지 않지만, 다른 도메인 에러는 가능하다.
			if errors.Is(err, system.ErrKeyNotFound) {
				return api.ErrNotFound.
					WithMessage("key_not_found: " + decodedKey).
					WithDetails(map[string]string{"error": "key_not_found"})
			}
			return api.MapDomainError(err)
		}
		action = "entry_deleted"
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"action": action,
		"key":    decodedKey,
	}))
}

// @spec SPEC-STORE-003
// ResetAll 은 네임스페이스의 모든 키를 reset 한다.
//
//	DELETE /store/{agent_name}/keys?namespace=default
//
// 동작:
//  1. ListStoreKeys 로 키 목록을 조회한다.
//  2. 각 키에 대해 IsStaticKey 로 분기하여
//     - 정적 키 → ClearHistory (history_cleared 카운트 증가)
//     - 동적 키 → DeleteEntry (entries_deleted 카운트 증가)
//  3. 개별 키 처리 중 에러가 발생하면 로그에 남기고 다음 키로 계속 진행한다 (best-effort).
//
// 원자성: 본 엔드포인트는 비-원자적이다. 키 목록 조회와 개별 reset 사이에 새 키가
// 쓰여지면 그 키는 처리 대상에 포함되지 않는다. 운영 환경에서 reset 중 동시 쓰기가
// 발생할 가능성이 낮다는 가정 하에 단순한 구현을 채택했다.
//
// 응답: 항상 200 OK 와 함께 {history_cleared, entries_deleted} 카운트를 반환한다.
// 키가 없으면 두 카운트 모두 0 이다.
func (h *StoreQueryHandler) ResetAll(ctx api.Context) error {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return api.ErrBadRequest.WithMessage("agent_name is required")
	}

	namespace := ctx.Query("namespace")

	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	resetter, ok := ag.(storeResetter)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_a_store_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_a_store_agent"})
	}

	keys, err := resetter.ListStoreKeys(ctx.Context(), namespace, "*")
	if err != nil {
		return api.MapDomainError(err)
	}

	var historyCleared, entriesDeleted int
	for _, k := range keys {
		if resetter.IsStaticKey(k) {
			if cerr := resetter.ClearHistory(ctx.Context(), namespace, k); cerr != nil {
				// 동시 삭제 등으로 키가 사라졌을 수 있다 → 로그 후 진행.
				h.logger.Warn("store reset all: clear history failed",
					"agent", agentName, "namespace", namespace, "key", k, "err", cerr)
				continue
			}
			historyCleared++
		} else {
			if derr := resetter.DeleteEntry(ctx.Context(), namespace, k); derr != nil {
				h.logger.Warn("store reset all: delete entry failed",
					"agent", agentName, "namespace", namespace, "key", k, "err", derr)
				continue
			}
			entriesDeleted++
		}
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"history_cleared": historyCleared,
		"entries_deleted": entriesDeleted,
	}))
}
