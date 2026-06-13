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
//
// v0.3.0 주의: KeyTags 는 GET /tags 전용 호환 shim 으로 유지된다. GET /keys 는
// 더 풍부한 메타데이터(StaticKeyMeta)를 요구하므로 별도의 storeKeyMetaLister
// 인터페이스를 사용한다.
type storeKeyTagLister interface {
	storeKeyLister
	// KeyTags 는 사용자 관점 key → 태그 맵 전체를 반환한다 (복사본).
	// 정적 키가 없으면 빈 맵을 반환한다.
	KeyTags(ctx context.Context) (map[string]map[string]string, error)
	// StaticTagPairs 는 태그 key → 정렬된 unique value 목록을 반환한다.
	// /tags 엔드포인트 전용 집계 헬퍼이다.
	StaticTagPairs() map[string][]string
}

// @spec SPEC-STORE-003 v0.3.0
// storeKeyMetaLister 는 정적 키의 전체 메타데이터(StaticKeyMeta) 스냅샷을 노출하는
// 에이전트 계약이다. system.UserStoreAgent 가 이 인터페이스를 만족한다.
//
// GET /store/{name}/keys (v0.3.0 BREAKING) 핸들러가 응답 객체 배열을 빌드할 때
// data_type, metric_type, registration, tags 를 한 번에 가져오기 위해 사용한다.
// 반환 맵은 호출자 전용 깊은 복사본이며, 핸들러가 임의로 수정해도 안전하다.
type storeKeyMetaLister interface {
	StaticKeysSnapshot() map[string]system.StaticKeyMeta
}

// @spec SPEC-STORE-003 v0.4.0
// storeKeyMetaSetter 는 임의 엔트리(정적 + 동적)의 metric_type/tags 를 설정하는
// 에이전트 계약이다. system.UserStoreAgent 가 이 인터페이스를 만족한다.
//
// PUT /store/{name}/keys/{key}/meta 핸들러가 사용하며, 동적으로 등록된 키에도
// 사용자가 나중에 타입/태그를 부여할 수 있게 한다. 미등록 키이면 동적 string 키로
// 신규 등록된다. 입력 검증(metric_type 정규식, tag key 정규식)은 구현체가 수행하며,
// 검증 실패 시 system.ErrInvalidMetricType / system.ErrInvalidTagKey 를 반환한다.
type storeKeyMetaSetter interface {
	SetKeyMeta(key string, metricType string, tags map[string]string) error
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
	// @spec SPEC-STORE-003 v0.4.0: 임의 엔트리(동적 포함)의 metric_type/tags 설정.
	g.PUT("/store/{agent_name}/keys/{key}/meta", h.SetKeyMeta)
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

// resolveAggregationOriginMs 는 버킷 집계 시 범위 하한 필터로 쓸 시각(epoch ms)을 결정한다.
//
// epoch-zero 정렬 도입 후 이 값은 버킷 경계 결정에는 사용되지 않으며, 사용자 요청
// 범위 밖 (시작 시각 이전) 의 엔트리를 떨어뜨리는 용도로만 쓰인다.
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
// 버킷 정렬: epoch zero 기준 (벽시계 경계).
// 사용자 startMs 가 인터벌 경계와 어긋나도 버킷은 항상 epoch 0 기준 벽시계 경계에
// 정렬된다. 예) intervalMs=60000 → 모든 버킷의 초 = 0.
// 1d 간격은 UTC 자정에 정렬됨 (로컬 자정 아님 — sub-day 간격에는 영향 없음).
//
// 규칙:
//   - 각 엔트리의 버킷 시작은 floor(ts_ms / intervalMs) * intervalMs
//   - ts_ms < originMs 인 엔트리는 범위 하한 필터로 제외된다
//   - 비숫자/NaN/Infinity 값은 스킵한다 (평균의 분모에 포함되지 않음)
//   - 유효 숫자가 0 개인 버킷은 응답에서 생략된다 (페이로드 축소)
//   - 각 버킷의 timestamp 는 bucketStartMs 이다
//   - 결과는 timestamp 오름차순이다
//
// originMs 는 더 이상 버킷 정렬에 사용되지 않고 범위 하한 필터로만 쓰인다.
// 시그니처는 호출자 호환성을 위해 유지한다.
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
	// map 키는 bucket 시작 시각 (epoch ms). 일반적으로 연속되지만 희소할 수도 있으므로 map 사용.
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
		// epoch-zero 정렬: 사용자 시작 시각과 무관하게 벽시계 경계에 맞춘다.
		bucketStartMs := (tsMs / intervalMs) * intervalMs
		st, exists := buckets[bucketStartMs]
		if !exists {
			st = &bucketState{sum: f, count: 1, min: f, max: f}
			buckets[bucketStartMs] = st
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

	// 시작 시각 오름차순 정렬 후 결과 구성.
	starts := make([]int64, 0, len(buckets))
	for k := range buckets {
		starts = append(starts, k)
	}
	// 작은 수의 버킷에 대한 단순 삽입 정렬 대신 표준 sort 사용.
	sortInt64Asc(starts)

	out := make([]chartQueryEntry, 0, len(starts))
	for _, bucketStartMs := range starts {
		st := buckets[bucketStartMs]
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
			Timestamp: bucketStartMs,
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

// @spec SPEC-STORE-003 v0.3.0
// StoreKeyResponse 는 GET /store/{name}/keys 응답 배열의 단일 키 객체이다.
// v0.2.0 의 단순 string 배열에서 객체 배열로 BREAKING 변경되었다 (M9).
//
// 필드 의미:
//   - Key:          사용자 관점 key (예: "indoor:1:room_temp")
//   - Registration: "manual" (yaml 정의) 또는 "auto" (런타임 자동 등록)
//   - DataType:     6종 enum 중 하나 ("int", "float", "string", "boolean", "bytes", "json")
//   - MetricType:   free string (기본 "unknown")
//   - Tags:         태그 맵. 빈 맵이라도 항상 포함되며 null 이 되지 않는다 (M9).
type StoreKeyResponse struct {
	Key          string            `json:"key"`
	Registration string            `json:"registration"`
	DataType     string            `json:"data_type"`
	MetricType   string            `json:"metric_type"`
	Tags         map[string]string `json:"tags"`
}

// @spec SPEC-STORE-003 v0.3.0
// StoreKeysListResponse 는 GET /store/{name}/keys 응답의 envelope 이다.
// Keys 배열은 알파벳순(Key 오름차순) 으로 정렬된다 (M9 안정성 요구).
type StoreKeysListResponse struct {
	Count int                `json:"count"`
	Keys  []StoreKeyResponse `json:"keys"`
}

// @spec SPEC-STORE-003 v0.3.0
// keyFilter 는 GET /store/{name}/keys 의 다축 필터 (tag, data_type, metric_type,
// registration) 의 AND 결합을 표현한다.
//
// 필드별 빈 문자열 시맨틱 (plan §7):
//   - DataType, MetricType, Registration 이 빈 문자열이면 해당 축은 통과 (no-op).
//   - 비어있지 않으면 정확히 일치해야 한다 (case-sensitive).
//
// `?metric_type=` (URL 파라미터가 존재하지만 값이 빈 문자열) 의 경우, 본 구조체는
// 빈 문자열을 "필터 미적용" 으로 처리한다. 이는 plan §7 의 명시적 design choice 로,
// 실제 staticKeys 의 MetricType 은 항상 normalize 되어 빈 문자열이 될 수 없으므로
// 동작상 차이가 없다 (auto: "unknown", manual yaml: validateMetricType 으로 보정).
type keyFilter struct {
	Tags         []tagFilter // 다중 ?tag= AND 결합
	DataType     string      // "" = no filter
	MetricType   string      // "" = no filter
	Registration string      // "" = no filter; else "manual" | "auto"
}

// matches 는 StaticKeyMeta 가 모든 필터 조건을 만족하는지 검사한다 (AND).
// 각 축은 빈 문자열이면 통과, 아니면 정확히 일치해야 한다.
//
// @spec SPEC-STORE-003 v0.3.0
func (f keyFilter) matches(meta system.StaticKeyMeta) bool {
	if f.DataType != "" && string(meta.DataType) != f.DataType {
		return false
	}
	if f.MetricType != "" && meta.MetricType != f.MetricType {
		return false
	}
	if f.Registration != "" && string(meta.Source) != f.Registration {
		return false
	}
	for _, tf := range f.Tags {
		v, ok := meta.Tags[tf.key]
		if !ok || v != tf.value {
			return false
		}
	}
	return true
}

// parseKeyFilter 는 ?tag= ?data_type= ?metric_type= ?registration= 쿼리 파라미터를
// keyFilter 로 변환한다. ?tag= 형식이 잘못되면 (콜론 없음) 400 에러를 반환한다.
//
// 다중 값 처리 규칙:
//   - ?tag= 는 다중 허용 (AND 결합) — parseTagFilters 위임.
//   - ?data_type=, ?metric_type=, ?registration= 는 동일 이름이 여러 번 와도
//     첫 번째 값만 사용한다 (단일 axis 필터). 이는 net/url 의 default behavior 와 일치한다.
//
// @spec SPEC-STORE-003 v0.3.0
func parseKeyFilter(ctx api.Context) (keyFilter, error) {
	var f keyFilter

	// ?tag= 다중 (AND). 잘못된 형식은 400.
	tags, err := parseTagFilters(ctx.QueryValues("tag"))
	if err != nil {
		return f, err
	}
	f.Tags = tags

	// ?data_type=, ?metric_type=, ?registration= 단일 axis (첫 값만).
	// ctx.Query 는 첫 번째 값을 반환하므로 그대로 사용한다.
	f.DataType = ctx.Query("data_type")
	f.MetricType = ctx.Query("metric_type")
	f.Registration = ctx.Query("registration")
	return f, nil
}

// ListKeys 는 Store 에이전트의 키 목록을 반환한다.
//
//	GET /store/{agent_name}/keys
//	GET /store/{agent_name}/keys?tag=room:1&tag=type:temperature
//	GET /store/{agent_name}/keys?data_type=float
//	GET /store/{agent_name}/keys?metric_type=temperature
//	GET /store/{agent_name}/keys?registration=manual
//	GET /store/{agent_name}/keys?registration=manual&metric_type=temperature&tag=room:1   (AND)
//
// @spec SPEC-STORE-003 v0.3.0 (BREAKING — M9)
//   - 응답 형식이 v0.2.0 의 `{count, keys: []string, tags: map}` 에서
//     `{count, keys: [{key, registration, data_type, metric_type, tags}, ...]}` 로 변경되었다.
//   - 키 정렬: 항상 Key 오름차순 (알파벳).
//   - 태그 필터 (M4): `?tag=key:value` 다중 허용 (AND), 잘못된 형식은 HTTP 400.
//   - 신규 필터: `?data_type=`, `?metric_type=`, `?registration=` (각 단일 값, AND 결합).
//   - 자동 등록 키: registration="auto", metric_type 보통 "unknown", tags={}.
//   - tags 필드는 항상 객체 (빈 맵 `{}` 포함, null 아님).
//
// 동작 흐름:
//  1. 에이전트 조회 → 404 if not found.
//  2. 타입 단언: storeKeyMetaLister 를 만족해야 함 (UserStoreAgent), 아니면 400.
//  3. 필터 파싱 → 잘못된 ?tag= 는 400.
//  4. StaticKeysSnapshot() 으로 정적 + 자동 등록된 모든 키의 메타 스냅샷 획득.
//  5. 필터 적용 후 알파벳순 정렬, tags 가 nil 이면 {} 로 normalize 하여 응답.
//
// 주의: ?namespace= 와 ?pattern= 는 v0.3.0 에서 응답에 영향을 주지 않는다.
// staticKeys 는 namespace 무관하게 에이전트 단위로 관리되며, pattern glob 매칭은
// v0.3.0 응답 모델(객체 배열) 의 design 단순화를 위해 제거되었다.
// (UserStoreAgent.ListStoreKeys 는 reset 등 다른 경로에서 계속 사용된다.)
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

	// v0.3.0: 응답 객체 배열 빌드를 위해 풍부 메타 인터페이스를 요구한다.
	metaLister, ok := ag.(storeKeyMetaLister)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_a_store_agent: " + agentName)
	}

	// 필터 파싱 (?tag= 형식 검증 포함). 에이전트 조회 뒤에 수행하여
	// 잘못된 agent_name → 404, 잘못된 필터 → 400 의 분류를 유지한다.
	filter, ferr := parseKeyFilter(ctx)
	if ferr != nil {
		return api.ErrBadRequest.WithMessage(ferr.Error())
	}

	// 일관된 스냅샷 (manual + auto-registered 모두 포함, deep copy).
	snapshot := metaLister.StaticKeysSnapshot()

	// 필터 적용 + 응답 객체 빌드.
	objects := make([]StoreKeyResponse, 0, len(snapshot))
	for key, meta := range snapshot {
		if !filter.matches(meta) {
			continue
		}
		// Tags 는 항상 non-nil 보장 (M9: "빈 tags 객체로 표시").
		// snapshot 이 깊은 복사를 보장하므로 그대로 사용해도 안전하지만, nil 가능성을
		// 차단해 JSON 인코딩 결과를 결정적(`{}` vs `null`)으로 만든다.
		tags := meta.Tags
		if tags == nil {
			tags = map[string]string{}
		}
		objects = append(objects, StoreKeyResponse{
			Key:          key,
			Registration: string(meta.Source),
			DataType:     string(meta.DataType),
			MetricType:   meta.MetricType,
			Tags:         tags,
		})
	}

	// 알파벳순 정렬 (M9 안정성 요구). 결정적 응답 순서를 보장한다.
	sort.Slice(objects, func(i, j int) bool { return objects[i].Key < objects[j].Key })

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(StoreKeysListResponse{
		Count: len(objects),
		Keys:  objects,
	}))
}

// @spec SPEC-STORE-003 v0.4.0
// setKeyMetaRequest 는 PUT /store/{name}/keys/{key}/meta 요청 바디이다.
//
// 필드:
//   - MetricType: 설정할 metric_type (생략/빈 문자열 → "unknown" 으로 normalize).
//   - Tags:       설정할 태그 맵 전체(replace 시맨틱). 생략하면 빈 맵으로 간주되어 기존 태그가 비워진다.
//
// 주의: Tags 는 부분 갱신(merge)이 아니라 전체 교체(replace)이다. 일부만 바꾸려면
// 클라이언트가 기존 태그를 포함한 전체 맵을 보내야 한다 (단순하고 예측 가능한 시맨틱).
type setKeyMetaRequest struct {
	MetricType string            `json:"metric_type"`
	Tags       map[string]string `json:"tags"`
}

// @spec SPEC-STORE-003 v0.4.0
// SetKeyMeta 는 임의 엔트리(정적 또는 동적)의 metric_type 과 tags 를 설정한다.
//
//	PUT /store/{agent_name}/keys/{key}/meta
//	body: {"metric_type": "temperature", "tags": {"room": "1"}}
//
// 동작:
//   - 정적 키: DataType/Source 보존, metric_type/tags 만 갱신.
//   - 동적 키: data_type=string/Source=auto 보존, metric_type/tags 갱신.
//   - 미등록 키: 동적 string 키로 신규 등록 후 메타 적용(사전 타입/태그 지정).
//
// 검증 실패 매핑:
//   - metric_type 정규식 위반 → 400 (system.ErrInvalidMetricType)
//   - tag key 정규식 위반     → 400 (system.ErrInvalidTagKey)
//
// 키 경로 파라미터는 url.PathUnescape 로 디코딩되어 콜론(%3A) 등 특수문자를 안전히 처리한다.
// 응답: 200 OK 와 함께 적용된 {key, metric_type, tags} 를 반환한다.
func (h *StoreQueryHandler) SetKeyMeta(ctx api.Context) error {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return api.ErrBadRequest.WithMessage("agent_name is required")
	}

	rawKey := ctx.Param("key")
	if rawKey == "" {
		return api.ErrBadRequest.WithMessage("key is required")
	}
	decodedKey, derr := url.PathUnescape(rawKey)
	if derr != nil {
		return api.ErrBadRequest.WithMessage("invalid key encoding")
	}

	var req setKeyMetaRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	setter, ok := ag.(storeKeyMetaSetter)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_a_store_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_a_store_agent"})
	}

	if err := setter.SetKeyMeta(decodedKey, req.MetricType, req.Tags); err != nil {
		// 검증 에러는 400 으로 매핑한다 (그 외는 그대로 메시지 노출).
		if errors.Is(err, system.ErrInvalidMetricType) || errors.Is(err, system.ErrInvalidTagKey) {
			return api.ErrBadRequest.WithMessage(err.Error())
		}
		return api.ErrBadRequest.WithMessage(err.Error())
	}

	// 응답: 적용 결과(normalize 된 metric_type/tags) 를 일관되게 반환한다.
	metricType := req.MetricType
	if metricType == "" {
		metricType = system.MetricTypeUnknown
	}
	tags := req.Tags
	if tags == nil {
		tags = map[string]string{}
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"key":         decodedKey,
		"metric_type": metricType,
		"tags":        tags,
	}))
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

// @spec SPEC-STORE-003 v0.3.0
//
// 주: v0.2.0 의 filterKeysByTags/buildTagsPayload 헬퍼는 GET /keys 응답이
// 객체 배열(StoreKeyResponse)로 BREAKING 변경됨에 따라 제거되었다. 새로운 필터
// 로직은 keyFilter.matches 에 통합되어 있다 (위 ListKeys 구현 참조).
// /tags 엔드포인트는 별도 헬퍼 없이 KeyTags/StaticTagPairs 만 사용하므로 영향 없음.

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
