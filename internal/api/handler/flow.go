package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/ws"
)

// FlowManager 는 플로우 작업을 위한 인터페이스이다.
// 핸들러를 구체적인 엔진 구현으로부터 분리한다.
type FlowManager interface {
	ListFlows(ctx context.Context, opts dto.ListOptions) ([]FlowInfo, int64, error)
	GetFlow(ctx context.Context, id string) (*FlowInfo, error)
	CreateFlow(ctx context.Context, req *dto.FlowCreateRequest) (*FlowInfo, error)
	UpdateFlow(ctx context.Context, id string, req *dto.FlowUpdateRequest) (*FlowInfo, error)
	DeleteFlow(ctx context.Context, id string) error
	DeployFlow(ctx context.Context, id string) error
	StartFlow(ctx context.Context, id string) error
	StopFlow(ctx context.Context, id string) error
	RestartFlow(ctx context.Context, id string) error
	ConfigureFlow(ctx context.Context, id string, cfg map[string]any) error
	FlowStatus(ctx context.Context, id string) (*FlowStatusInfo, error)
	ListFlowNodes(ctx context.Context, flowID string) ([]FlowNodeInfo, error)
	GetFlowNode(ctx context.Context, flowID, nodeID string) (*FlowNodeInfo, error)
}

// FlowInfo 는 핸들러가 반환하는 플로우 정보를 나타낸다.
type FlowInfo struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Status      string         `json:"status"`
	CreatedAt   string         `json:"created_at,omitempty"`
	UpdatedAt   string         `json:"updated_at,omitempty"`
	NodeCount   int            `json:"node_count"`
	Config      map[string]any `json:"config,omitempty"`
}

// FlowStatusInfo 는 상세 플로우 상태를 나타낸다.
type FlowStatusInfo struct {
	ID           string         `json:"id"`
	Status       string         `json:"status"`
	Uptime       string         `json:"uptime,omitempty"`
	NodeStats    []NodeStatInfo `json:"node_stats,omitempty"`
	MessageCount int64          `json:"message_count"`
	ErrorCount   int64          `json:"error_count"`
}

// NodeStatInfo 는 노드별 통계를 나타낸다.
type NodeStatInfo struct {
	NodeID   string `json:"node_id"`
	NodeName string `json:"node_name"`
	NodeType string `json:"node_type"`
	Processed int64 `json:"processed"`
	Errors    int64 `json:"errors"`
}

// FlowNodeInfo 는 플로우 내 노드 인스턴스의 런타임 정보를 나타낸다.
type FlowNodeInfo struct {
	NodeID string         `json:"node_id"`
	Name   string         `json:"name"`
	Type   string         `json:"type"`
	State  string         `json:"state"`
	Config map[string]any `json:"config,omitempty"`
	Ports  []PortInfo     `json:"ports,omitempty"`
	Extra  map[string]any `json:"extra,omitempty"`
}

// PortInfo 는 노드 포트의 런타임 정보를 나타낸다.
type PortInfo struct {
	ID         string  `json:"id"`
	Name       string  `json:"name"`
	Direction  string  `json:"direction"`
	Connected  bool    `json:"connected"`
	Messages   int64  `json:"messages"`
	Throughput string `json:"throughput"`             // "12.300" msg/sec (소수점 3자리)
	ActiveFor  string `json:"active_for"`             // "1m30s" (비활성이면 빈 문자열)
}

// FlowHandler 는 플로우 관련 API 엔드포인트를 처리한다.
type FlowHandler struct {
	flows  FlowManager
	events *ws.EventPublisher // nil 허용 (이벤트 미사용 시)
	logger *slog.Logger
}

// FlowHandlerOption 은 FlowHandler 의 선택적 설정 함수이다.
type FlowHandlerOption func(*FlowHandler)

// WithEventPublisher 는 FlowHandler 에 EventPublisher 를 설정한다.
func WithEventPublisher(ep *ws.EventPublisher) FlowHandlerOption {
	return func(h *FlowHandler) {
		h.events = ep
	}
}

// NewFlowHandler 는 새 FlowHandler를 생성한다.
func NewFlowHandler(flows FlowManager, logger *slog.Logger, opts ...FlowHandlerOption) *FlowHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &FlowHandler{
		flows:  flows,
		logger: logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterRoutes 는 주어진 라우트 그룹에 플로우 라우트를 등록한다.
//
// Routes:
//
//	GET    /flows              -> List
//	GET    /flows/export       -> ExportAll (주의: /flows/{id} 보다 먼저 등록해야 함)
//	GET    /flows/{id}         -> Get
//	GET    /flows/{id}/export  -> Export
//	POST   /flows              -> Create
//	PUT    /flows/{id}         -> Update
//	DELETE /flows/{id}         -> Delete
//	POST   /flows/{id}/deploy  -> Deploy
//	POST   /flows/{id}/start   -> Start
//	POST   /flows/{id}/stop    -> Stop
//	POST   /flows/{id}/restart -> Restart
//	PUT    /flows/{id}/config  -> Configure
//	GET    /flows/{id}/status  -> Status
func (h *FlowHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/flows", h.List)
	// /flows/export 는 /flows/{id} 보다 먼저 등록하여 라우트 충돌을 방지한다
	g.GET("/flows/export", h.ExportAll)
	g.GET("/flows/{id}", h.Get)
	g.GET("/flows/{id}/export", h.Export)
	g.POST("/flows", h.Create)
	g.PUT("/flows/{id}", h.Update)
	g.DELETE("/flows/{id}", h.Delete)
	g.POST("/flows/{id}/deploy", h.Deploy)
	g.POST("/flows/{id}/start", h.Start)
	g.POST("/flows/{id}/stop", h.Stop)
	g.POST("/flows/{id}/restart", h.Restart)
	g.PUT("/flows/{id}/config", h.Configure)
	g.GET("/flows/{id}/status", h.Status)
	g.GET("/flows/{id}/nodes", h.ListNodes)
	g.GET("/flows/{id}/nodes/{nodeID}", h.GetNode)
}

// List 는 페이지네이션을 적용하여 플로우 목록을 반환한다.
// GET /flows?page=1&size=20&status=running
func (h *FlowHandler) List(ctx api.Context) error {
	opts := parseListOptions(ctx)

	flows, total, err := h.flows.ListFlows(ctx.Context(), opts)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewPaginatedResponse(flows, opts.Page, opts.Size, total))
}

// Get 은 ID로 단일 플로우를 반환한다.
// GET /flows/{id}
func (h *FlowHandler) Get(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	info, err := h.flows.GetFlow(ctx.Context(), id)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}

// Create 는 새 플로우를 생성한다.
// POST /flows
func (h *FlowHandler) Create(ctx api.Context) error {
	var req dto.FlowCreateRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if errs := dto.ValidateFlowCreate(&req); errs != nil {
		return api.ErrValidationFailed.WithDetails(errs)
	}

	info, err := h.flows.CreateFlow(ctx.Context(), &req)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(info))
}

// Update 는 기존 플로우를 업데이트한다.
// PUT /flows/{id}
func (h *FlowHandler) Update(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	var req dto.FlowUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	info, err := h.flows.UpdateFlow(ctx.Context(), id, &req)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}

// Delete 는 플로우를 삭제한다.
// DELETE /flows/{id}
func (h *FlowHandler) Delete(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	if err := h.flows.DeleteFlow(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.NoContent(http.StatusNoContent)
}

// Deploy 는 플로우를 배포한다.
// POST /flows/{id}/deploy
func (h *FlowHandler) Deploy(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	h.logger.Debug("Deploy handler called", "flowID", id)

	if err := h.flows.DeployFlow(ctx.Context(), id); err != nil {
		h.logger.Warn("Deploy handler: DeployFlow failed", "flowID", id, "error", err)
		return api.MapDomainError(err)
	}

	if h.events != nil {
		name := id // 기본값: flowID
		if info, err := h.flows.GetFlow(ctx.Context(), id); err == nil {
			name = info.Name
		}
		h.events.PublishFlowEvent(ws.EventFlowDeployed, name, id)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "deployed",
	}))
}

// Start 는 플로우를 시작한다.
// POST /flows/{id}/start
func (h *FlowHandler) Start(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	h.logger.Debug("Start handler called", "flowID", id)

	if err := h.flows.StartFlow(ctx.Context(), id); err != nil {
		h.logger.Warn("Start handler: StartFlow failed", "flowID", id, "error", err)
		return api.MapDomainError(err)
	}

	if h.events != nil {
		name := id // 기본값: flowID
		if info, err := h.flows.GetFlow(ctx.Context(), id); err == nil {
			name = info.Name
		}
		h.events.PublishFlowEvent(ws.EventFlowStarted, name, id)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "started",
	}))
}

// Stop 은 플로우를 정지한다.
// POST /flows/{id}/stop
func (h *FlowHandler) Stop(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	if err := h.flows.StopFlow(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	if h.events != nil {
		name := id // 기본값: flowID
		if info, err := h.flows.GetFlow(ctx.Context(), id); err == nil {
			name = info.Name
		}
		h.events.PublishFlowEvent(ws.EventFlowStopped, name, id)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "stopped",
	}))
}

// Restart 는 플로우를 재시작한다.
// POST /flows/{id}/restart
func (h *FlowHandler) Restart(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	if err := h.flows.RestartFlow(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "restarted",
	}))
}

// Configure 는 플로우 설정을 업데이트한다.
// PUT /flows/{id}/config
func (h *FlowHandler) Configure(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	var req dto.ConfigUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if errs := dto.ValidateConfigUpdate(&req); errs != nil {
		return api.ErrValidationFailed.WithDetails(errs)
	}

	if err := h.flows.ConfigureFlow(ctx.Context(), id, req.Config); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "configured",
	}))
}

// Status 는 플로우의 상세 상태를 반환한다.
// GET /flows/{id}/status
func (h *FlowHandler) Status(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	status, err := h.flows.FlowStatus(ctx.Context(), id)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(status))
}

// Export 는 단일 플로우를 내보내기용 데이터로 반환한다.
// GET /flows/{id}/export
// 런타임 필드(id, status, created_at, updated_at, node_count)를 제거하고
// name, description, definition 만 반환한다.
func (h *FlowHandler) Export(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	info, err := h.flows.GetFlow(ctx.Context(), id)
	if err != nil {
		return api.MapDomainError(err)
	}

	// 런타임 필드 제거 — name, description, definition 만 포함
	exported := map[string]any{
		"name": info.Name,
	}
	if info.Description != "" {
		exported["description"] = info.Description
	}
	if info.Config != nil {
		exported["definition"] = info.Config
	}

	return ctx.JSON(http.StatusOK, exported)
}

// ExportAll 은 모든 플로우를 내보내기용 데이터 배열로 반환한다.
// GET /flows/export
// 각 플로우에서 런타임 필드를 제거하고 반환한다.
func (h *FlowHandler) ExportAll(ctx api.Context) error {
	opts := dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 100},
	}

	flows, _, err := h.flows.ListFlows(ctx.Context(), opts)
	if err != nil {
		return api.MapDomainError(err)
	}

	exported := make([]map[string]any, 0, len(flows))
	for _, f := range flows {
		// 개별 플로우를 조회하여 전체 Config(definition)를 확보한다
		full, err := h.flows.GetFlow(ctx.Context(), f.ID)
		if err != nil {
			continue // 조회 실패 시 건너뛴다
		}
		item := map[string]any{
			"name": full.Name,
		}
		if full.Description != "" {
			item["description"] = full.Description
		}
		if full.Config != nil {
			item["definition"] = full.Config
		}
		exported = append(exported, item)
	}

	return ctx.JSON(http.StatusOK, exported)
}

// ListNodes 는 플로우 내 모든 노드 인스턴스의 목록을 반환한다.
// GET /flows/{id}/nodes
func (h *FlowHandler) ListNodes(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	nodes, err := h.flows.ListFlowNodes(ctx.Context(), id)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(nodes))
}

// GetNode 는 플로우 내 특정 노드 인스턴스의 상세 정보를 반환한다.
// GET /flows/{id}/nodes/{nodeID}
func (h *FlowHandler) GetNode(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}
	nodeID := ctx.Param("nodeID")
	if nodeID == "" {
		return api.ErrBadRequest.WithMessage("node id is required")
	}

	info, err := h.flows.GetFlowNode(ctx.Context(), id, nodeID)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}

// parsePagination 은 쿼리 파라미터에서 페이지네이션 정보를 추출한다.
func parsePagination(ctx api.Context) dto.PaginationParams {
	page, _ := strconv.Atoi(ctx.QueryDefault("page", "1"))
	size, _ := strconv.Atoi(ctx.QueryDefault("size", "20"))
	p := dto.PaginationParams{Page: page, Size: size}
	p.Normalize()
	return p
}

// parseListOptions 는 쿼리 파라미터에서 목록 조회 옵션을 추출한다.
func parseListOptions(ctx api.Context) dto.ListOptions {
	return dto.ListOptions{
		PaginationParams: parsePagination(ctx),
		Sort:             ctx.Query("sort"),
		Filter:           ctx.Query("filter"),
		Status:           ctx.Query("status"),
		Detail:           ctx.Query("detail"),
	}
}
