package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// influxFluxQueryer 는 InfluxDB 에이전트가 제공해야 하는 쿼리 계약이다.
// *system.InfluxDBAgent 가 이 인터페이스를 만족한다. 잘못된 타입의 에이전트가
// InfluxDB 엔드포인트에 지정되면 400 으로 거부할 수 있다.
type influxFluxQueryer interface {
	ExecuteFluxQuery(ctx context.Context, flux string) ([]system.InfluxQueryResult, error)
	ExecuteInfluxQLQuery(ctx context.Context, iql string) ([]system.InfluxQueryResult, error)
}

// defaultInfluxQueryTimeout 은 InfluxDB 쿼리 HTTP 요청의 기본 타임아웃이다.
// 추후 config 로 override 할 수 있도록 패키지 변수로 둔다.
var defaultInfluxQueryTimeout = 30 * time.Second

// InfluxDBQueryHandler 는 SPEC-CHART-001 REQ-M3-02 를 구현한다.
//
// 라우트:
//
//	POST /influxdb/{agent_name}/query
type InfluxDBQueryHandler struct {
	agents AgentLookup
	logger *slog.Logger
}

// NewInfluxDBQueryHandler 는 새 InfluxDBQueryHandler 를 생성한다.
func NewInfluxDBQueryHandler(agents AgentLookup, logger *slog.Logger) *InfluxDBQueryHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &InfluxDBQueryHandler{agents: agents, logger: logger}
}

// RegisterRoutes 는 InfluxDB 쿼리 라우트를 등록한다.
func (h *InfluxDBQueryHandler) RegisterRoutes(g *api.RouteGroup) {
	g.POST("/influxdb/{agent_name}/query", h.Query)
}

// influxDBQueryRequest 는 REQ-M3-02 요청 바디 형식이다.
type influxDBQueryRequest struct {
	QueryLanguage string         `json:"query_language"`
	Query         string         `json:"query"`
	Params        map[string]any `json:"params,omitempty"`
}

// Query 는 InfluxDB 에이전트의 Flux/InfluxQL 쿼리를 실행한다.
func (h *InfluxDBQueryHandler) Query(ctx api.Context) error {
	agentName := ctx.Param("agent_name")
	if agentName == "" {
		return api.ErrBadRequest.WithMessage("agent_name is required")
	}

	var req influxDBQueryRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.Query == "" {
		return api.ErrBadRequest.WithMessage("query is required")
	}

	lang := req.QueryLanguage
	if lang == "" {
		lang = "flux"
	}
	if lang != "flux" && lang != "influxql" {
		return api.ErrBadRequest.WithMessage("query_language must be 'flux' or 'influxql'")
	}

	ag := findAgentByName(h.agents, agentName)
	if ag == nil {
		return api.ErrNotFound.
			WithMessage("agent_not_found: " + agentName).
			WithDetails(map[string]string{"error": "agent_not_found"})
	}

	influxAgent, ok := ag.(influxFluxQueryer)
	if !ok {
		return api.ErrBadRequest.
			WithMessage("not_an_influxdb_agent: " + agentName).
			WithDetails(map[string]string{"error": "not_an_influxdb_agent"})
	}

	// 쿼리 타임아웃 적용.
	qctx, cancel := context.WithTimeout(ctx.Context(), defaultInfluxQueryTimeout)
	defer cancel()

	var rows []system.InfluxQueryResult
	var err error
	if lang == "flux" {
		rows, err = influxAgent.ExecuteFluxQuery(qctx, req.Query)
	} else {
		rows, err = influxAgent.ExecuteInfluxQLQuery(qctx, req.Query)
	}
	if err != nil {
		// context 타임아웃/취소는 408 로 매핑한다.
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return api.ErrRequestTimeout.WithMessage(err.Error())
		}
		return api.ErrInternalServer.WithMessage(err.Error())
	}

	resp := chartQueryResponse{
		Entries:   mapInfluxRowsToDTO(rows),
		Count:     len(rows),
		Truncated: false,
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// mapInfluxRowsToDTO 는 InfluxQueryResult 배열을 표준 응답의 chartQueryEntry 로 변환한다.
// Time 이 zero value 면 Timestamp = 0 이 되며, 이는 "시간 정보 없음"을 의미한다.
func mapInfluxRowsToDTO(rows []system.InfluxQueryResult) []chartQueryEntry {
	out := make([]chartQueryEntry, 0, len(rows))
	for _, r := range rows {
		var ts int64
		if !r.Time.IsZero() {
			ts = r.Time.UnixMilli()
		}
		out = append(out, chartQueryEntry{
			Timestamp: ts,
			Value:     r.Value,
			Labels:    r.Labels,
		})
	}
	return out
}
