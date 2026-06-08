// remote_query.go 는 M8(그룹 J) 원격 READ 프록시 REST API 를 제공한다
// (@SPEC:SPEC-REMOTE-001 M8, 그룹 J, REQ-J01/J02/J05/J07/J15/J16).
//
// 로컬 디테일 API 형태를 병렬화하여(프론트엔드 8.4 가 base path 만 교체하면 되도록)
// 승인+온라인 노드의 flow/agent/device READ 데이터를 노드 경유로 프록시한다. 모든
// 엔드포인트는 admin 권한을 강제하고(REQ-F04), 자원-타깃 action 은 노출 범위를 게이트
// 한다(REQ-J05 — 범위 밖 404). 일반 READ 는 감사하지 않으며(REQ-J15), 노출 위반/오류
// 접근만 감사한다.
//
// 라우트 → (domain, query_action) 매핑 표(목록은 기존 미러 엔드포인트 사용 — remote_admin.go):
//
//	flow:
//	  GET .../flows/{flow_id}                       → flow/get
//	  GET .../flows/{flow_id}/status                → flow/status
//	  GET .../flows/{flow_id}/nodes                 → flow/nodes
//	  GET .../flows/{flow_id}/nodes/{node_id}       → flow/node
//	  GET .../flows/{flow_id}/logs                  → flow/logs
//	agent:
//	  GET .../agents/{agent_id}                     → agent/get
//	  GET .../agents/{agent_id}/stats               → agent/stats   (라이브 — 캐시 우회)
//	  GET .../agents/{agent_id}/config              → agent/config
//	  GET .../agents/{agent_id}/devices             → agent/devices
//	  GET .../agents/{agent_id}/topics              → agent/topics
//	  GET .../agents/{agent_id}/store               → agent/store
//	  GET .../agents/{agent_id}/sessions            → agent/sessions
//	  GET .../agents/{agent_id}/series              → agent/series  (라이브 — 캐시 우회)
//	device:
//	  GET .../devices/{device_id}                   → device/get
//	  GET .../devices/{device_id}/state             → device/state  (라이브 — 캐시 우회)
//	  GET .../devices/{device_id}/commands          → device/commands
//	  GET .../devices/{device_id}/metadata          → device/metadata
//
// 모든 경로의 접두사는 /remote/nodes/{instance_id}/... 이다. 본문은 노드가 이미
// redaction 한 본문을 그대로 통과시킨다(REQ-J06 — 서버는 마스킹하지 않음).
//
// 실패 매핑(REQ-J07, mapRemoteQueryError): 미관리/오프라인 → 503, 타임아웃 → 504,
// 노드 ok:false → 502, 노출 범위 밖 → 404.
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// RemoteQueryService 는 원격 READ 프록시 오케스트레이션을 추상화한다(*remote.Server 가
// 만족). 핸들러는 노출 범위(REQ-J05)를 IsResourceExposed 로 사전 게이트한 뒤 DispatchQuery
// 로 노드에 질의를 전파한다.
type RemoteQueryService interface {
	// DispatchQuery 는 승인+온라인 노드에 READ 질의를 전파하고 redacted 결과를 기다린다
	// (REQ-J01/J02). 미관리 → remote.ErrNodeNotManaged, 타임아웃 → ErrQueryTimeout,
	// 노드 실패 → ErrQueryFailed.
	DispatchQuery(ctx context.Context, instanceID, domain, queryAction string, args json.RawMessage) (json.RawMessage, error)
	// IsResourceExposed 는 kind(flow|agent|device) 자원 id 가 노드의 노출 범위 내(미러
	// 존재)인지 반환한다(REQ-J05/E07).
	IsResourceExposed(ctx context.Context, instanceID, kind, id string) (bool, error)
	// IsManaged 는 노드가 승인+온라인인지 반환한다(M10 노드-레벨 query 게이팅 — REQ-L02).
	// dashboard config·metrics 는 per-resource 노출 범위가 없는 노드-레벨 자원이므로
	// (REQ-L03), 노출 범위 평가 없이 IsManaged 만 게이트한다.
	IsManaged(instanceID string) bool
}

// RemoteQueryHandler 는 원격 READ 프록시 엔드포인트를 처리한다.
type RemoteQueryHandler struct {
	svc    RemoteQueryService
	audit  storage.RemoteAuditRepository
	logger *slog.Logger
}

// NewRemoteQueryHandler 는 RemoteQueryHandler 를 생성한다.
func NewRemoteQueryHandler(svc RemoteQueryService, logger *slog.Logger) *RemoteQueryHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteQueryHandler{svc: svc, logger: logger}
}

// WithAudit 는 노출 위반/오류 접근 감사 저장소를 연결한다(REQ-J15). nil 이면 감사는
// 구조화 로그로만 남는다. 일반 READ 성공은 감사하지 않는다.
func (h *RemoteQueryHandler) WithAudit(audit storage.RemoteAuditRepository) *RemoteQueryHandler {
	h.audit = audit
	return h
}

// RegisterRoutes 는 원격 READ 프록시 라우트를 그룹에 등록한다(목록은 remote_admin.go 의
// 미러 엔드포인트 사용 — 여기서는 자원-타깃 디테일/라이브 READ 만).
func (h *RemoteQueryHandler) RegisterRoutes(g *api.RouteGroup) {
	// flow
	g.GET("/remote/nodes/{instance_id}/flows/{flow_id}", h.flowGet)
	g.GET("/remote/nodes/{instance_id}/flows/{flow_id}/status", h.flowStatus)
	g.GET("/remote/nodes/{instance_id}/flows/{flow_id}/nodes", h.flowNodes)
	g.GET("/remote/nodes/{instance_id}/flows/{flow_id}/nodes/{node_id}", h.flowNode)
	g.GET("/remote/nodes/{instance_id}/flows/{flow_id}/logs", h.flowLogs)
	// agent
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}", h.agentGet)
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}/stats", h.agentStats)
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}/config", h.agentConfig)
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}/devices", h.agentDevices)
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}/topics", h.agentTopics)
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}/store", h.agentStore)
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}/sessions", h.agentSessions)
	g.GET("/remote/nodes/{instance_id}/agents/{agent_id}/series", h.agentSeries)
	// device
	g.GET("/remote/nodes/{instance_id}/devices/{device_id}", h.deviceGet)
	g.GET("/remote/nodes/{instance_id}/devices/{device_id}/state", h.deviceState)
	g.GET("/remote/nodes/{instance_id}/devices/{device_id}/commands", h.deviceCommands)
	g.GET("/remote/nodes/{instance_id}/devices/{device_id}/metadata", h.deviceMetadata)

	// M10 그룹 L: 노드-레벨 READ(대시보드 config + 시스템 메트릭). per-resource 노출
	// 범위가 없으므로(REQ-L03) IsManaged 만 게이트한다(노출 범위 미평가). 라우트는
	// 자원-타깃 경로(.../flows/{id} 등)보다 path 세그먼트가 짧거나 리터럴이므로 충돌 없음.
	g.GET("/remote/nodes/{instance_id}/dashboards/shared", h.dashboardShared)
	g.GET("/remote/nodes/{instance_id}/dashboards/mine", h.dashboardMine)
	g.GET("/remote/nodes/{instance_id}/metrics", h.monitorMetrics)
}

// --- flow -------------------------------------------------------------------

func (h *RemoteQueryHandler) flowGet(ctx api.Context) error {
	return h.query(ctx, remote.DomainFlow, remote.QueryActionGet, ctx.Param("flow_id"), nil)
}
func (h *RemoteQueryHandler) flowStatus(ctx api.Context) error {
	return h.query(ctx, remote.DomainFlow, remote.QueryActionStatus, ctx.Param("flow_id"), nil)
}
func (h *RemoteQueryHandler) flowNodes(ctx api.Context) error {
	return h.query(ctx, remote.DomainFlow, remote.QueryActionNodes, ctx.Param("flow_id"), nil)
}
func (h *RemoteQueryHandler) flowNode(ctx api.Context) error {
	return h.query(ctx, remote.DomainFlow, remote.QueryActionNode, ctx.Param("flow_id"),
		map[string]string{"node_id": ctx.Param("node_id")})
}
func (h *RemoteQueryHandler) flowLogs(ctx api.Context) error {
	return h.query(ctx, remote.DomainFlow, remote.QueryActionLogs, ctx.Param("flow_id"), nil)
}

// --- agent ------------------------------------------------------------------

func (h *RemoteQueryHandler) agentGet(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionGet, ctx.Param("agent_id"), nil)
}
func (h *RemoteQueryHandler) agentStats(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionStats, ctx.Param("agent_id"), nil)
}
func (h *RemoteQueryHandler) agentConfig(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionConfig, ctx.Param("agent_id"), nil)
}
func (h *RemoteQueryHandler) agentDevices(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionDevices, ctx.Param("agent_id"), nil)
}
func (h *RemoteQueryHandler) agentTopics(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionTopics, ctx.Param("agent_id"), nil)
}
func (h *RemoteQueryHandler) agentStore(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionStore, ctx.Param("agent_id"), nil)
}
func (h *RemoteQueryHandler) agentSessions(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionSessions, ctx.Param("agent_id"), nil)
}
func (h *RemoteQueryHandler) agentSeries(ctx api.Context) error {
	return h.query(ctx, remote.DomainAgent, remote.QueryActionSeries, ctx.Param("agent_id"), nil)
}

// --- device -----------------------------------------------------------------

func (h *RemoteQueryHandler) deviceGet(ctx api.Context) error {
	return h.query(ctx, remote.DomainDevice, remote.QueryActionGet, ctx.Param("device_id"), nil)
}
func (h *RemoteQueryHandler) deviceState(ctx api.Context) error {
	return h.query(ctx, remote.DomainDevice, remote.QueryActionState, ctx.Param("device_id"), nil)
}
func (h *RemoteQueryHandler) deviceCommands(ctx api.Context) error {
	return h.query(ctx, remote.DomainDevice, remote.QueryActionCommands, ctx.Param("device_id"), nil)
}
func (h *RemoteQueryHandler) deviceMetadata(ctx api.Context) error {
	return h.query(ctx, remote.DomainDevice, remote.QueryActionMetadata, ctx.Param("device_id"), nil)
}

// --- M10 그룹 L: 노드-레벨 READ(대시보드 config + 메트릭) ----------------------

// dashboardShared 는 노드의 공유(global) 대시보드 config 를 프록시한다(REQ-L01,
// dashboard/get_shared). 노드-레벨 자원이므로 노출 범위 게이트 없이 IsManaged 만 평가.
func (h *RemoteQueryHandler) dashboardShared(ctx api.Context) error {
	return h.nodeQuery(ctx, remote.DomainDashboard, remote.QueryActionGetShared, nil)
}

// dashboardMine 은 노드의 개인(user) 대시보드 config 를 프록시한다(REQ-L01/A17,
// dashboard/get_mine). owner 는 viewing admin(ctx.UserID())로 전달하여 그 노드의
// 동일 사용자 스코프 config 를 노드-로컬로 취득한다(deviceId 네임스페이싱 — REQ-L03).
func (h *RemoteQueryHandler) dashboardMine(ctx api.Context) error {
	args, _ := json.Marshal(map[string]string{"owner": ctx.UserID()})
	return h.nodeQuery(ctx, remote.DomainDashboard, remote.QueryActionGetMine, args)
}

// monitorMetrics 는 노드의 시스템 메트릭 스냅샷을 프록시한다(REQ-L05, monitor/metrics).
// 완만 변동이므로 서버는 단기 TTL 캐시한다(REQ-J16 — DispatchQuery 가 처리).
func (h *RemoteQueryHandler) monitorMetrics(ctx api.Context) error {
	return h.nodeQuery(ctx, remote.DomainMonitor, remote.QueryActionMetrics, nil)
}

// nodeQuery 는 노드-레벨 READ(per-resource 노출 범위 없음 — REQ-L03)의 오케스트레이션
// 이다: admin 게이트 → IsManaged 게이트(503) → DispatchQuery → redacted 본문 통과.
//
// query() 와 달리 IsResourceExposed 를 호출하지 않는다(대시보드 config·메트릭은 노드
// 단위 자원이므로 노출 범위 개념이 없음 — REQ-L02/L03). 실패 의미는 그룹 J 와 동일
// (503/504/502 — mapRemoteQueryError 재사용). 일반 read 는 감사하지 않으며(REQ-L13),
// 오류 접근만 감사한다(REQ-J15 일관).
func (h *RemoteQueryHandler) nodeQuery(ctx api.Context, domain, queryAction string, args json.RawMessage) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	instanceID := ctx.Param("instance_id")

	// 노드-레벨 게이팅(REQ-L02/J05): 미관리(미승인/오프라인) → 503. 노출 범위는 미평가.
	if !h.svc.IsManaged(instanceID) {
		return api.ErrServiceUnavailable.WithMessage("노드가 관리 대상이 아닙니다(미승인/오프라인)")
	}

	data, qErr := h.svc.DispatchQuery(ctx.Context(), instanceID, domain, queryAction, args)
	if qErr != nil {
		// 오류 접근 감사(REQ-L13/J15) — 일반 read 성공은 감사하지 않는다.
		h.recordError(ctx, instanceID, domain, queryAction, "", qErr)
		return mapRemoteQueryError(qErr)
	}
	// 노드가 redaction 한 본문을 그대로 통과시킨다(REQ-J06/L02).
	return h.writeRaw(ctx, data)
}

// --- 공통 오케스트레이션 -------------------------------------------------------

// query 는 admin 게이트 → 노출 범위 게이트 → DispatchQuery → redacted 본문 통과를
// 수행한다. extra 는 args 에 병합할 추가 식별 인자(예: node_id)이다.
//
// args 는 항상 {"id": resourceID, ...extra} 형태로 구성한다(노드 QuerySource 가 id 로
// 자원을 식별). 게이팅은 캐시 적중과 무관하게 매 요청 평가된다(DispatchQuery 가 캐시를
// 다루나 노출 범위 평가는 본 핸들러가 항상 선행 — REQ-J05).
func (h *RemoteQueryHandler) query(ctx api.Context, domain, queryAction, resourceID string, extra map[string]string) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	instanceID := ctx.Param("instance_id")
	if resourceID == "" {
		return api.ErrBadRequest.WithMessage("자원 id 는 필수입니다")
	}

	// 노출 범위 게이트(REQ-J05). 범위 밖이면 디스패치하지 않고 404 + 위반 감사.
	exposed, err := h.svc.IsResourceExposed(ctx.Context(), instanceID, kindFor(domain), resourceID)
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	if !exposed {
		h.recordViolation(ctx, instanceID, domain, queryAction, resourceID)
		return api.ErrNotFound.WithMessage("대상 자원이 노드의 노출 범위에 없습니다")
	}

	// args 구성: {"id": resourceID, ...extra}.
	argMap := map[string]string{"id": resourceID}
	for k, v := range extra {
		argMap[k] = v
	}
	args, _ := json.Marshal(argMap)

	data, qErr := h.svc.DispatchQuery(ctx.Context(), instanceID, domain, queryAction, args)
	if qErr != nil {
		// 오류 접근 감사(REQ-J15) — 일반 READ 성공은 감사하지 않는다.
		h.recordError(ctx, instanceID, domain, queryAction, resourceID, qErr)
		return mapRemoteQueryError(qErr)
	}

	// 노드가 redaction 한 본문을 그대로 통과시킨다(REQ-J06). 빈 본문은 null 로 응답.
	return h.writeRaw(ctx, data)
}

// writeRaw 는 노드 redacted 본문을 표준 성공 응답으로 감싸 반환한다. 본문은 이미 JSON
// 이므로 RawMessage 로 전달해 이중 인코딩을 피한다.
func (h *RemoteQueryHandler) writeRaw(ctx api.Context, data json.RawMessage) error {
	if len(data) == 0 {
		data = json.RawMessage("null")
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(data))
}

// recordViolation 은 노출 범위 위반 접근을 감사한다(REQ-J15). audit 미구성이면 로그만.
func (h *RemoteQueryHandler) recordViolation(ctx api.Context, instanceID, domain, action, resourceID string) {
	h.logger.Warn("원격 READ 노출 범위 위반",
		"instance_id", instanceID, "domain", domain, "query_action", action,
		"resource_id", resourceID, "actor", ctx.UserID())
	h.appendAudit(ctx, instanceID, domain, action, storage.AuditResultError, "exposure scope violation")
}

// recordError 는 오류 접근을 감사한다(REQ-J15). 노드 오류 사유는 시크릿일 수 있으므로
// 분류만 기록한다(REQ-F06).
func (h *RemoteQueryHandler) recordError(ctx api.Context, instanceID, domain, action, resourceID string, err error) {
	h.logger.Warn("원격 READ 오류 접근",
		"instance_id", instanceID, "domain", domain, "query_action", action,
		"resource_id", resourceID, "error", err)
	h.appendAudit(ctx, instanceID, domain, action, storage.AuditResultError, classifyQueryError(err))
}

// appendAudit 는 감사 레코드를 기록한다(REQ-J15). audit 미구성이면 no-op. 시크릿은
// 기록하지 않는다(REQ-F06 — domain/action/result/reason 분류만).
func (h *RemoteQueryHandler) appendAudit(ctx api.Context, instanceID, domain, action, result, reason string) {
	if h.audit == nil {
		return
	}
	rec := storage.RemoteAuditRecord{
		InstanceID:    instanceID,
		Actor:         ctx.UserID(),
		Action:        storage.AuditActionCommand,
		Domain:        domain,
		CommandAction: action,
		Result:        result,
		Reason:        reason,
		Timestamp:     time.Now().UnixMilli(),
	}
	if err := h.audit.Append(ctx.Context(), rec); err != nil {
		h.logger.Warn("원격 READ 감사 기록 실패", "instance_id", instanceID, "error", err)
	}
}

// classifyQueryError 는 디스패치 오류를 시크릿 없는 분류 문자열로 환원한다(REQ-F06).
func classifyQueryError(err error) string {
	switch {
	case errors.Is(err, remote.ErrNodeNotManaged), errors.Is(err, remote.ErrNoConn):
		return "node not managed"
	case errors.Is(err, remote.ErrQueryTimeout):
		return "query timeout"
	case errors.Is(err, remote.ErrQueryFailed):
		return "node query failed"
	default:
		return "query error"
	}
}

// mapRemoteQueryError 는 READ 프록시 도메인 오류를 APIError 로 매핑한다(REQ-J07).
//   - remote.ErrNodeNotManaged / ErrNoConn → 503 (미승인/오프라인 — REQ-J05)
//   - remote.ErrQueryTimeout              → 504 (결과 미수신)
//   - remote.ErrQueryFailed               → 502 (노드 질의 실패)
//   - context.Canceled 등 그 외           → 500
func mapRemoteQueryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, remote.ErrNodeNotManaged), errors.Is(err, remote.ErrNoConn):
		return api.ErrServiceUnavailable.WithMessage(err.Error())
	case errors.Is(err, remote.ErrQueryTimeout):
		return &api.APIError{HTTPCode: http.StatusGatewayTimeout, Code: "QUERY_TIMEOUT", Message: err.Error()}
	case errors.Is(err, remote.ErrQueryFailed):
		return &api.APIError{HTTPCode: http.StatusBadGateway, Code: "QUERY_FAILED", Message: err.Error()}
	default:
		return api.ErrInternalServer.WithMessage(err.Error())
	}
}
