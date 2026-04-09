package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// AgentManager 는 에이전트 작업을 위한 인터페이스이다.
type AgentManager interface {
	ListAgents(ctx context.Context, opts dto.ListOptions) ([]AgentInfo, int64, error)
	GetAgent(ctx context.Context, id string, detail string) (*AgentInfo, error)
	CreateAgent(ctx context.Context, req *dto.AgentCreateRequest) (*AgentInfo, error)
	UpdateAgent(ctx context.Context, id string, req *dto.AgentUpdateRequest) (*AgentInfo, error)
	DeleteAgent(ctx context.Context, id string) error
	StartAgent(ctx context.Context, id string) error
	StopAgent(ctx context.Context, id string) error
	RestartAgent(ctx context.Context, id string) error
	ConfigureAgent(ctx context.Context, id string, cfg map[string]any) error
	AgentStats(ctx context.Context, id string) (*AgentStatsInfo, error)
	ExecAgent(ctx context.Context, id string, data []byte) (json.RawMessage, error)
	// EnableAgent 는 에이전트를 영속적으로 활성화한다 (SPEC-AGENT-005 R3.8).
	// 현재 실행 상태에 영향을 주지 않으며, 다음 데몬 재시작 시 자동 시작 대상에 포함된다.
	EnableAgent(ctx context.Context, id string) (*AgentInfo, error)
	// DisableAgent 는 에이전트를 영속적으로 비활성화한다 (SPEC-AGENT-005 R3.7).
	// 현재 실행 중인 에이전트를 정지시키지 않으며, 다음 데몬 재시작 시 자동 시작에서 제외된다.
	DisableAgent(ctx context.Context, id string) (*AgentInfo, error)
}

// AgentHealthInfo 는 에이전트 헬스 상태 요약이다.
type AgentHealthInfo struct {
	Status    string    `json:"status"`
	LastCheck time.Time `json:"last_check"`
}

// AgentStatsResponse 는 에이전트 메시지 통계 요약이다.
type AgentStatsResponse struct {
	MessagesIn     int64 `json:"messages_in"`
	MessagesOut    int64 `json:"messages_out"`
	Errors         int64 `json:"errors"`
	BufferPending  int   `json:"buffer_pending"`
	BufferCapacity int   `json:"buffer_capacity"`
}

// AgentSharedInfo 는 에이전트 공유 참조 정보이다.
type AgentSharedInfo struct {
	RefCount int32    `json:"ref_count"`
	Flows    []string `json:"flows"`
}

// AgentInfo 는 에이전트 정보를 나타낸다.
type AgentInfo struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	Status  string         `json:"status"`
	Enabled bool           `json:"enabled"` // 에이전트 활성화 상태 (SPEC-AGENT-005). omitempty 없음: 항상 출력.
	Config  map[string]any `json:"config,omitempty"`
	// Detail view fields (Module 6) - populated based on detail level
	Health     *AgentHealthInfo    `json:"health,omitempty"`
	Stats      *AgentStatsResponse `json:"stats,omitempty"`
	Uptime     string              `json:"uptime,omitempty"`
	StartedAt  *time.Time          `json:"started_at,omitempty"`
	CreatedAt  *time.Time          `json:"created_at,omitempty"`
	Connected  *bool               `json:"connected,omitempty"`
	SharedInfo *AgentSharedInfo    `json:"shared_info,omitempty"`
	State      map[string]any      `json:"state,omitempty"`
}

// MessageCounters 는 메시지 카운터 그룹이다.
type MessageCounters struct {
	Received int64 `json:"received"`
	Sent     int64 `json:"sent"`
	Errored  int64 `json:"errored"`
}

// EnhancedMessagesStats 는 전체/외부/내부 메시지 통계이다.
type EnhancedMessagesStats struct {
	Total    MessageCounters `json:"total"`
	External MessageCounters `json:"external"`
	Internal MessageCounters `json:"internal"`
}

// BytesStats 는 바이트 I/O 통계이다.
type BytesStats struct {
	Read    int64 `json:"read"`
	Written int64 `json:"written"`
}

// BufferStatsInfo 는 메시지 버퍼 상태이다.
type BufferStatsInfo struct {
	Pending  int `json:"pending"`
	Capacity int `json:"capacity"`
}

// ConnectionStatsResponse 는 에이전트 타입별 외부 연결 통계이다.
type ConnectionStatsResponse struct {
	ID               string `json:"id"`
	MessagesReceived int64  `json:"messages_received"`
	MessagesSent     int64  `json:"messages_sent"`
	MessagesErrored  int64  `json:"messages_errored"`
	BytesRead        int64  `json:"bytes_read"`
	BytesWritten     int64  `json:"bytes_written"`
	ConnectedAt      string `json:"connected_at"`
	LastActivityAt   string `json:"last_activity_at"`
}

// NodeRefStatsResponse 는 노드 참조별 내부 통계이다.
type NodeRefStatsResponse struct {
	NodeID           string `json:"node_id"`
	NodeName         string `json:"node_name,omitempty"`
	FlowID           string `json:"flow_id"`
	FlowName         string `json:"flow_name,omitempty"`
	MessagesReceived int64  `json:"messages_received"`
	MessagesSent     int64  `json:"messages_sent"`
	MessagesErrored  int64  `json:"messages_errored"`
	LastActivityAt   string `json:"last_activity_at"`
}

// AgentStatsInfo 는 에이전트 통계를 나타낸다.
type AgentStatsInfo struct {
	// 기존 flat 필드 유지 (하위 호환성)
	ID             string `json:"id"`
	Status         string `json:"status"`
	Uptime         string `json:"uptime,omitempty"`
	MessagesIn     int64  `json:"messages_in"`
	MessagesOut    int64  `json:"messages_out"`
	ErrorCount     int64  `json:"error_count"`
	Connected      bool   `json:"connected"`
	BufferPending  int    `json:"buffer_pending"`
	BufferCapacity int    `json:"buffer_capacity"`

	// 새 중첩 구조 (SPEC-AGENT-004)
	Messages          *EnhancedMessagesStats    `json:"messages,omitempty"`
	Bytes             *BytesStats               `json:"bytes,omitempty"`
	Buffer            *BufferStatsInfo          `json:"buffer,omitempty"`
	DroppedMessages   int64                     `json:"dropped_messages"`
	LoadTime          string                    `json:"load_time,omitempty"`
	AvgProcessLatency string                    `json:"avg_processing_latency,omitempty"`
	RestartCount      int64                     `json:"restart_count"`
	LastActivityAt    string                    `json:"last_activity_at,omitempty"`
	Connections       []ConnectionStatsResponse `json:"connections"`
	NodeRefs          []NodeRefStatsResponse    `json:"node_refs"`
}

// AgentHandler 는 에이전트 관련 API 엔드포인트를 처리한다.
type AgentHandler struct {
	agents AgentManager
	flows  FlowManager // 에이전트 이름 변경 시 플로우 참조 cascade 업데이트에 사용
	logger *slog.Logger
}

// AgentHandlerOption 은 AgentHandler 의 선택적 설정 함수이다.
type AgentHandlerOption func(*AgentHandler)

// WithFlowManager 는 AgentHandler 에 FlowManager 를 설정한다.
// 에이전트 이름 변경 시 플로우 정의의 agent_ref 참조를 cascade 업데이트하는 데 사용된다.
func WithFlowManager(flows FlowManager) AgentHandlerOption {
	return func(h *AgentHandler) {
		h.flows = flows
	}
}

// NewAgentHandler 는 새 AgentHandler를 생성한다.
func NewAgentHandler(agents AgentManager, logger *slog.Logger, opts ...AgentHandlerOption) *AgentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &AgentHandler{
		agents: agents,
		logger: logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterRoutes 는 에이전트 라우트를 등록한다.
//
// Routes:
//
//	GET    /agents               -> List
//	GET    /agents/export        -> ExportAll (주의: /agents/{id} 보다 먼저 등록해야 함)
//	GET    /agents/{id}          -> Get
//	GET    /agents/{id}/export   -> Export
//	POST   /agents               -> Create
//	PUT    /agents/{id}          -> Update
//	DELETE /agents/{id}          -> Delete
//	POST   /agents/{id}/start    -> Start
//	POST   /agents/{id}/stop     -> Stop
//	POST   /agents/{id}/restart  -> Restart
//	POST   /agents/{id}/enable   -> Enable  (SPEC-AGENT-005)
//	POST   /agents/{id}/disable  -> Disable (SPEC-AGENT-005)
//	PUT    /agents/{id}/config   -> Configure
//	GET    /agents/{id}/stats    -> Stats
func (h *AgentHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/agents", h.List)
	// /agents/export 는 /agents/{id} 보다 먼저 등록하여 라우트 충돌을 방지한다
	g.GET("/agents/export", h.ExportAll)
	g.GET("/agents/{id}", h.Get)
	g.GET("/agents/{id}/export", h.Export)
	g.POST("/agents", h.Create)
	g.PUT("/agents/{id}", h.Update)
	g.DELETE("/agents/{id}", h.Delete)
	g.POST("/agents/{id}/start", h.Start)
	g.POST("/agents/{id}/stop", h.Stop)
	g.POST("/agents/{id}/restart", h.Restart)
	g.POST("/agents/{id}/enable", h.Enable)
	g.POST("/agents/{id}/disable", h.Disable)
	g.PUT("/agents/{id}/config", h.Configure)
	g.GET("/agents/{id}/stats", h.Stats)
	g.POST("/agents/{id}/exec", h.Exec)
}

// List 는 페이지네이션을 적용하여 에이전트 목록을 반환한다.
// GET /agents?page=1&size=20&status=running
func (h *AgentHandler) List(ctx api.Context) error {
	opts := parseListOptions(ctx)

	agents, total, err := h.agents.ListAgents(ctx.Context(), opts)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewPaginatedResponse(agents, opts.Page, opts.Size, total))
}

// Get 은 ID로 단일 에이전트를 반환한다.
// GET /agents/{id}
func (h *AgentHandler) Get(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	detail := ctx.QueryDefault("detail", "summary")

	info, err := h.agents.GetAgent(ctx.Context(), id, detail)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}

// Create 는 새 에이전트를 생성한다.
// POST /agents
func (h *AgentHandler) Create(ctx api.Context) error {
	var req dto.AgentCreateRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if errs := dto.ValidateAgentCreate(&req); errs != nil {
		return api.ErrValidationFailed.WithDetails(errs)
	}

	info, err := h.agents.CreateAgent(ctx.Context(), &req)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(info))
}

// Update 는 기존 에이전트를 업데이트한다.
// PUT /agents/{id}
// 이름 변경 시, 모든 플로우 정의의 agent_ref.agent_name 참조를 cascade 업데이트한다.
func (h *AgentHandler) Update(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	var req dto.AgentUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	// 이름 변경이 요청된 경우 기존 이름을 저장한다
	var oldName string
	if req.Name != nil {
		existing, err := h.agents.GetAgent(ctx.Context(), id, "")
		if err != nil {
			return api.MapDomainError(err)
		}
		oldName = existing.Name
	}

	info, err := h.agents.UpdateAgent(ctx.Context(), id, &req)
	if err != nil {
		return api.MapDomainError(err)
	}

	// 이름이 실제로 변경된 경우 플로우의 agent_ref 참조를 cascade 업데이트한다
	if req.Name != nil && oldName != "" && oldName != *req.Name && h.flows != nil {
		h.cascadeAgentRename(ctx.Context(), oldName, *req.Name)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}

// cascadeAgentRename 는 저장된 모든 플로우에서 이전 에이전트 이름을 새 이름으로 업데이트한다.
func (h *AgentHandler) cascadeAgentRename(ctx context.Context, oldName, newName string) {
	count, err := h.flows.RenameAgentInFlows(ctx, oldName, newName)
	if err != nil {
		h.logger.Warn("cascade rename: 실패", "oldName", oldName, "newName", newName, "error", err)
		return
	}
	if count > 0 {
		h.logger.Info("cascade rename: 완료", "oldName", oldName, "newName", newName, "updatedFlows", count)
	}
}

// Delete 는 에이전트를 삭제한다.
// DELETE /agents/{id}
func (h *AgentHandler) Delete(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	if err := h.agents.DeleteAgent(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.NoContent(http.StatusNoContent)
}

// Start 는 에이전트를 시작한다.
// POST /agents/{id}/start
func (h *AgentHandler) Start(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	if err := h.agents.StartAgent(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "started",
	}))
}

// Stop 은 에이전트를 정지한다.
// POST /agents/{id}/stop
func (h *AgentHandler) Stop(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	if err := h.agents.StopAgent(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "stopped",
	}))
}

// Restart 는 에이전트를 재시작한다.
// POST /agents/{id}/restart
func (h *AgentHandler) Restart(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	if err := h.agents.RestartAgent(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "restarted",
	}))
}

// Enable 은 에이전트의 자동 시작을 활성화한다 (SPEC-AGENT-005 R3.8).
// 현재 실행 상태에 영향을 주지 않으며, 다음 데몬 재시작 시 자동 시작 대상에 포함된다.
// POST /agents/{id}/enable
func (h *AgentHandler) Enable(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	info, err := h.agents.EnableAgent(ctx.Context(), id)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}

// Disable 은 에이전트의 자동 시작을 비활성화한다 (SPEC-AGENT-005 R3.7).
// 현재 실행 중인 에이전트를 정지시키지 않으며, 다음 데몬 재시작 시 자동 시작에서 제외된다.
// POST /agents/{id}/disable
func (h *AgentHandler) Disable(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	info, err := h.agents.DisableAgent(ctx.Context(), id)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(info))
}

// Configure 는 에이전트 설정을 업데이트한다.
// PUT /agents/{id}/config
func (h *AgentHandler) Configure(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	var req dto.ConfigUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if errs := dto.ValidateConfigUpdate(&req); errs != nil {
		return api.ErrValidationFailed.WithDetails(errs)
	}

	if err := h.agents.ConfigureAgent(ctx.Context(), id, req.Config); err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "configured",
	}))
}

// Stats 는 에이전트의 상세 통계를 반환한다.
// GET /agents/{id}/stats
func (h *AgentHandler) Stats(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	stats, err := h.agents.AgentStats(ctx.Context(), id)
	if err != nil {
		return api.MapDomainError(err)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(stats))
}

// Exec 는 에이전트에 Process 커맨드를 전송한다.
// POST /agents/{id}/exec
func (h *AgentHandler) Exec(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	var req dto.AgentExecRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if req.Command == "" {
		return api.ErrBadRequest.WithMessage("command is required")
	}

	data, err := json.Marshal(req)
	if err != nil {
		return api.ErrBadRequest.WithMessage("failed to marshal request")
	}

	result, err := h.agents.ExecAgent(ctx.Context(), id, data)
	if err != nil {
		return api.MapDomainError(err)
	}

	var resultMap any
	if err := json.Unmarshal(result, &resultMap); err != nil {
		resultMap = string(result)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resultMap))
}

// Export 는 단일 에이전트를 내보내기용 데이터로 반환한다.
// GET /agents/{id}/export
// 런타임 필드(ID, Status)를 제거하고 Name, Type, Config 만 반환한다.
func (h *AgentHandler) Export(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("agent id is required")
	}

	info, err := h.agents.GetAgent(ctx.Context(), id, "")
	if err != nil {
		return api.MapDomainError(err)
	}

	// 런타임 필드 제거 (ID, Status)
	exported := map[string]any{
		"name": info.Name,
		"type": info.Type,
	}
	if info.Config != nil {
		exported["config"] = info.Config
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(exported))
}

// ExportAll 은 모든 에이전트를 내보내기용 데이터 배열로 반환한다.
// GET /agents/export
// 각 에이전트에서 런타임 필드를 제거하고 반환한다.
func (h *AgentHandler) ExportAll(ctx api.Context) error {
	opts := dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 100},
	}

	agents, _, err := h.agents.ListAgents(ctx.Context(), opts)
	if err != nil {
		return api.MapDomainError(err)
	}

	exported := make([]map[string]any, 0, len(agents))
	for _, a := range agents {
		item := map[string]any{
			"name": a.Name,
			"type": a.Type,
		}
		if a.Config != nil {
			item["config"] = a.Config
		}
		exported = append(exported, item)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(exported))
}
