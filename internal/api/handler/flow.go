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
	UndeployFlow(ctx context.Context, id string) error
	ConfigureFlow(ctx context.Context, id string, cfg map[string]any) error
	FlowStatus(ctx context.Context, id string) (*FlowStatusInfo, error)
	ListFlowNodes(ctx context.Context, flowID string) ([]FlowNodeInfo, error)
	GetFlowNode(ctx context.Context, flowID, nodeID string) (*FlowNodeInfo, error)

	// RenameAgentInFlows 는 저장된 모든 플로우에서 oldName 에이전트 참조를 newName 으로 변경한다.
	// 업데이트된 플로우 수를 반환한다.
	RenameAgentInFlows(ctx context.Context, oldName, newName string) (int, error)
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
	Uptime      string         `json:"uptime,omitempty"`
	AutoStart   bool           `json:"auto_start"`
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
	NodeID    string `json:"node_id"`
	NodeName  string `json:"node_name"`
	NodeType  string `json:"node_type"`
	Processed int64  `json:"processed"`
	Errors    int64  `json:"errors"`
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
	ID         string `json:"id"`
	Name       string `json:"name"`
	Direction  string `json:"direction"`
	Connected  bool   `json:"connected"`
	Messages   int64  `json:"messages"`
	Throughput string `json:"throughput"` // "12.300" msg/sec (소수점 3자리)
	ActiveFor  string `json:"active_for"` // "1m30s" (비활성이면 빈 문자열)
}

// FlowHandler 는 플로우 관련 API 엔드포인트를 처리한다.
type FlowHandler struct {
	flows  FlowManager
	agents AgentManager       // nil 허용 (에이전트 조회 미사용 시)
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

// WithAgentManager 는 FlowHandler 에 AgentManager 를 설정한다.
func WithAgentManager(agents AgentManager) FlowHandlerOption {
	return func(h *FlowHandler) {
		h.agents = agents
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
//	POST   /flows/{id}/stop      -> Stop
//	POST   /flows/{id}/undeploy -> Undeploy
//	POST   /flows/{id}/restart  -> Restart
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
	g.POST("/flows/{id}/undeploy", h.Undeploy)
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

// Undeploy 는 플로우를 배포 해제한다.
// POST /flows/{id}/undeploy
func (h *FlowHandler) Undeploy(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("flow id is required")
	}

	if err := h.flows.UndeployFlow(ctx.Context(), id); err != nil {
		return api.MapDomainError(err)
	}

	if h.events != nil {
		name := id // 기본값: flowID
		if info, err := h.flows.GetFlow(ctx.Context(), id); err == nil {
			name = info.Name
		}
		h.events.PublishFlowEvent(ws.EventFlowUndeployed, name, id)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"id":     id,
		"status": "undeployed",
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

// nodeLayoutFields 는 노드 최상위에서 렌더링 전용으로 분류되는 필드 이름 집합이다.
// 내보내기 시 layout 키로 분리되며, 가져오기 시 자동 생성 가능하다.
var nodeLayoutFields = map[string]bool{
	"type": true, "position": true, "measured": true,
	"selected": true, "dragging": true, "width": true, "height": true,
}

// edgeLayoutFields 는 엣지에서 렌더링 전용으로 분류되는 필드 이름 집합이다.
var edgeLayoutFields = map[string]bool{
	"type": true, "selected": true, "animated": true, "style": true,
}

// nodeDataRenames 는 data 내 필드명 변환 규칙이다. (React Flow 내부명 → 내보내기 공개명)
var nodeDataRenames = map[string]string{
	"label":    "name",
	"nodeType": "type",
}

// nodeDataAgentFields 는 agent_ref 구조로 묶이는 필드 집합이다.
// XFlow 표준 스키마 (pkg/flow.AgentRef) 와 동일한 키 이름을 사용한다.
var nodeDataAgentFields = map[string]string{
	"agent_id":   "agent_id",
	"agent_name": "agent_name",
}

// nodeDataSkipFields 는 기본값이면 제거할 필드와 해당 기본값이다.
var nodeDataSkipFields = map[string]any{
	"agent_type": "",
	"status":     "draft",
	"enabled":    true,
}

// nodeTopLevelKeepFields 는 노드 export 시 최상위에 유지되는 키 집합이다.
//
// pkg/flow.NodeDef 의 표준 필드 + 렌더링 전용 layout 을 포함한다.
// 이외의 모든 키는 restoreConfigNesting 이 config 객체로 자동 재중첩하여
// re-import 시 NodeDef.UnmarshalJSON 이 손실 없이 복원할 수 있도록 한다.
var nodeTopLevelKeepFields = map[string]bool{
	"id":        true,
	"name":      true,
	"type":      true,
	"enabled":   true,
	"config":    true,
	"inputs":    true,
	"outputs":   true,
	"errors":    true,
	"agent_ref": true,
	"metadata":  true,
	"layout":    true,
}

// restoreConfigNesting 은 flattenNodeData 가 최상위로 올린 노드 속성을
// 표준 XFlow 스키마 (pkg/flow.NodeDef) 에 맞게 config 객체로 재중첩한다.
//
// 동기: flattenNodeData 는 React Flow 의 data.{condition, expression, category,
// poll_command, ...} 등을 노드 최상위로 평탄화한다. 이 결과 JSON 을 다시 import 하면
// pkg/flow.FlowFromJSON 의 unmarshal 이 NodeDef 표준 필드가 아닌 키를 silently
// drop 하여 round-trip 데이터 손실이 발생한다.
//
// 본 함수는 비표준 키를 config 로 모아 export JSON 이 canonical 형식 (예:
// examples/flows/iot-sensor.json) 과 동일한 round-trip 동작을 갖도록 보장한다.
// flattenNodeData 가 만든 agent_ref/layout 은 그대로 최상위에 보존한다.
//
// 호출 위치: flattenNodeData 직후, separateLayoutFields 의 노드 정리 단계에서 사용한다.
//
// SPEC: flow import data-loss hotfix (2026-05-13)
func restoreConfigNesting(node map[string]any) map[string]any {
	// 1) 비표준 키 수집
	var extras map[string]any
	for k := range node {
		if nodeTopLevelKeepFields[k] {
			continue
		}
		if extras == nil {
			extras = make(map[string]any)
		}
		extras[k] = node[k]
	}
	if extras == nil {
		return node // 비표준 키 없음 — 변경 불필요
	}

	// 2) 기존 config 와 병합 (기존 명시 config 값이 우선)
	configMap, _ := node["config"].(map[string]any)
	if configMap == nil {
		configMap = make(map[string]any, len(extras))
	}
	for k, v := range extras {
		if _, exists := configMap[k]; exists {
			continue // 명시 config 값 보존
		}
		configMap[k] = v
		delete(node, k)
	}
	if len(configMap) > 0 {
		node["config"] = configMap
	}
	return node
}

// flattenNodeData 는 data 맵을 풀어서 노드 최상위 필드로 올리고,
// agent_id/agent_name (및 bridge 노드의 direction) 을 표준 agent_ref 객체로 묶는다.
//
// 표준 XFlow 스키마:
//
//	"agent_ref": {"agent_id": "...", "agent_name": "...", "direction": "..."}
//
// 이전 구현은 agent: {id, name} 으로 키 이름을 변환하여 export 결과를 다시
// import 할 때 pkg/flow/serialize.go 의 json.Unmarshal 이 NodeDef.AgentRef 로
// 역직렬화하지 못해 round-trip 결함이 발생했다. 이를 수정하기 위해 export
// 시점에서도 표준 nested 객체 그대로 보존한다. client (DynamicForm) 가 flat
// agent_ref:string 형태로 보내는 경우 server 의 normalizeReactFlowDefinition
// 이 이미 nested 로 변환하므로 이 함수는 nested 형식만 처리하면 된다.
//
// SPEC: flow round-trip 결함 hotfix (2026-05-13)
// 반환하는 맵은 노드의 최상위에 직접 병합되어야 한다.
func flattenNodeData(data map[string]any) map[string]any {
	flat := make(map[string]any, len(data))
	agentRef := make(map[string]any, 3)

	// 1) data["agent_ref"] 가 이미 표준 nested 객체이면 우선 채택
	if existing, ok := data["agent_ref"].(map[string]any); ok {
		for k, v := range existing {
			if s, ok := v.(string); ok && s == "" {
				continue
			}
			agentRef[k] = v
		}
	}

	for k, v := range data {
		// agent_ref 는 위에서 별도 처리했으므로 건너뛴다
		if k == "agent_ref" {
			continue
		}
		// 기본값과 동일하면 제거
		if def, ok := nodeDataSkipFields[k]; ok && v == def {
			continue
		}
		// 빈 문자열 제거
		if s, ok := v.(string); ok && s == "" {
			continue
		}
		// agent 그룹 필드 (agent_id/agent_name) → agent_ref 표준 객체로 흡수
		if agentKey, ok := nodeDataAgentFields[k]; ok {
			if _, already := agentRef[agentKey]; !already {
				agentRef[agentKey] = v
			}
			continue
		}
		// 필드명 변환
		if newKey, ok := nodeDataRenames[k]; ok {
			flat[newKey] = v
		} else {
			flat[k] = v
		}
	}

	// bridge 노드에서 direction 이 최상위로 올라온 경우 agent_ref 로 흡수한다.
	// (NodeDef.AgentRef.Direction 과 노드 최상위 "direction" 중복 방지)
	if len(agentRef) > 0 {
		if dir, ok := flat["direction"].(string); ok && dir != "" {
			if _, already := agentRef["direction"]; !already {
				agentRef["direction"] = dir
			}
			delete(flat, "direction")
		}
		flat["agent_ref"] = agentRef
	}
	return flat
}

// toSliceOfMaps 는 []any 또는 []map[string]any 를 []map[string]any 로 변환한다.
// flowToReactFlowConfig 는 []map[string]any 를 반환하고,
// JSON 디코딩은 []any 를 반환하므로 두 타입 모두 처리해야 한다.
func toSliceOfMaps(v any) []map[string]any {
	switch s := v.(type) {
	case []map[string]any:
		return s
	case []any:
		result := make([]map[string]any, 0, len(s))
		for _, item := range s {
			if m, ok := item.(map[string]any); ok {
				result = append(result, m)
			}
		}
		return result
	}
	return nil
}

// separateLayoutFields 는 플로우 정의에서 렌더링 전용 필드를 layout 키로 분리하고
// 노드 데이터의 필드명을 정제한다. 원본 definition 을 변경하지 않는다.
func separateLayoutFields(definition map[string]any) map[string]any {
	result := make(map[string]any, len(definition))
	for k, v := range definition {
		result[k] = v
	}

	// 노드 처리
	if nodesRaw, ok := result["nodes"]; ok {
		if nodes := toSliceOfMaps(nodesRaw); len(nodes) > 0 {
			cleaned := make([]any, 0, len(nodes))
			for _, node := range nodes {
				newNode := make(map[string]any, len(node))
				layout := make(map[string]any)
				for k, v := range node {
					if nodeLayoutFields[k] {
						layout[k] = v
					} else if k == "data" {
						// data 를 풀어서 노드 최상위로 병합
						if data, ok := v.(map[string]any); ok {
							for fk, fv := range flattenNodeData(data) {
								newNode[fk] = fv
							}
						}
					} else {
						newNode[k] = v
					}
				}
				if len(layout) > 0 {
					newNode["layout"] = layout
				}
				// 비표준 키를 config 로 재중첩하여 round-trip 보장
				// (SPEC: flow import data-loss hotfix — 2026-05-13)
				newNode = restoreConfigNesting(newNode)
				cleaned = append(cleaned, newNode)
			}
			result["nodes"] = cleaned
		}
	}

	// 엣지 처리
	if edgesRaw, ok := result["edges"]; ok {
		if edges := toSliceOfMaps(edgesRaw); len(edges) > 0 {
			cleaned := make([]any, 0, len(edges))
			for _, edge := range edges {
				newEdge := make(map[string]any, len(edge))
				layout := make(map[string]any)
				for k, v := range edge {
					if edgeLayoutFields[k] {
						layout[k] = v
					} else {
						newEdge[k] = v
					}
				}
				if len(layout) > 0 {
					newEdge["layout"] = layout
				}
				cleaned = append(cleaned, newEdge)
			}
			result["edges"] = cleaned
		}
	}

	return result
}

// extractAgentNames 은 플로우 정의에서 참조된 에이전트 이름을 추출한다.
func extractAgentNames(definition map[string]any) []string {
	nodesRaw, ok := definition["nodes"]
	if !ok {
		return nil
	}
	nodes, ok := nodesRaw.([]any)
	if !ok {
		return nil
	}

	seen := make(map[string]bool)
	var names []string

	for _, n := range nodes {
		node, ok := n.(map[string]any)
		if !ok {
			continue
		}
		// agent_ref.agent_name (XFlow 포맷)
		if ref, ok := node["agent_ref"].(map[string]any); ok {
			if name, ok := ref["agent_name"].(string); ok && name != "" {
				if !seen[name] {
					seen[name] = true
					names = append(names, name)
				}
			}
		}
	}
	return names
}

// resolveAgentExports 는 에이전트 이름 목록으로 내보내기용 데이터를 생성한다.
func (h *FlowHandler) resolveAgentExports(ctx context.Context, names []string) []map[string]any {
	agents, _, err := h.agents.ListAgents(ctx, dto.ListOptions{
		PaginationParams: dto.PaginationParams{Page: 1, Size: 100},
	})
	if err != nil {
		h.logger.Warn("에이전트 목록 조회 실패", "error", err)
		return nil
	}

	agentByName := make(map[string]*AgentInfo, len(agents))
	for i := range agents {
		agentByName[agents[i].Name] = &agents[i]
	}

	result := make([]map[string]any, 0, len(names))
	for _, name := range names {
		entry := map[string]any{"name": name}
		if ag, ok := agentByName[name]; ok {
			entry["type"] = ag.Type
			if ag.Config != nil {
				entry["config"] = ag.Config
			}
		}
		result = append(result, entry)
	}
	return result
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
		exported["definition"] = separateLayoutFields(info.Config)
	}

	// 플로우가 참조하는 에이전트 정보를 포함한다
	if h.agents != nil && info.Config != nil {
		if agentNames := extractAgentNames(info.Config); len(agentNames) > 0 {
			if requiredAgents := h.resolveAgentExports(ctx.Context(), agentNames); len(requiredAgents) > 0 {
				exported["required_agents"] = requiredAgents
			}
		}
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(exported))
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

	// 에이전트 목록을 1회 조회한다
	var agentByName map[string]*AgentInfo
	if h.agents != nil {
		agents, _, err := h.agents.ListAgents(ctx.Context(), dto.ListOptions{
			PaginationParams: dto.PaginationParams{Page: 1, Size: 100},
		})
		if err == nil {
			agentByName = make(map[string]*AgentInfo, len(agents))
			for i := range agents {
				agentByName[agents[i].Name] = &agents[i]
			}
		}
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
			item["definition"] = separateLayoutFields(full.Config)
			// 플로우가 참조하는 에이전트 정보를 포함한다
			if agentByName != nil {
				if agentNames := extractAgentNames(full.Config); len(agentNames) > 0 {
					requiredAgents := make([]map[string]any, 0, len(agentNames))
					for _, name := range agentNames {
						entry := map[string]any{"name": name}
						if ag, ok := agentByName[name]; ok {
							entry["type"] = ag.Type
							if ag.Config != nil {
								entry["config"] = ag.Config
							}
						}
						requiredAgents = append(requiredAgents, entry)
					}
					if len(requiredAgents) > 0 {
						item["required_agents"] = requiredAgents
					}
				}
			}
		}
		exported = append(exported, item)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(exported))
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
