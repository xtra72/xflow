package handler

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
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
type storeQueryRequest struct {
	Key          string `json:"key"`
	Mode         string `json:"mode"`
	Count        int    `json:"count,omitempty"`
	DurationSec  int    `json:"duration_sec,omitempty"`
	StartMs      int64  `json:"start_ms,omitempty"`
	EndMs        int64  `json:"end_ms,omitempty"`
	SinceVersion int64  `json:"since_version,omitempty"`
	Namespace    string `json:"namespace,omitempty"`
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

	resp := chartQueryResponse{
		Entries:   mapHistoryEntriesToDTO(entries),
		Count:     len(entries),
		Truncated: false,
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
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
