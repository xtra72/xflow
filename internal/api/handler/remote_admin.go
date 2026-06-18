// remote_admin.go 는 관리 노드 승인/거부/폐기/목록 REST API 를 제공한다
// (@SPEC:SPEC-REMOTE-001 M2, REQ-C03/C07, F03/F04, G01/G02 토대).
//
// 모든 엔드포인트는 admin 권한을 강제한다(node-role 토큰 거부 — REQ-F04). 운영
// 환경에서 Auth 미들웨어가 JWT 를 검증하여 user_role 컨텍스트를 채우고, 본 핸들러는
// role=admin 만 허용한다(PutChannel 패턴 준용).
//
// 라우트(api/v1 그룹 하위, /ws 모니터링과 분리 — REQ-N02):
//
//	GET  /remote/nodes                       — 전체 관리 노드 목록
//	GET  /remote/nodes/pending               — pending 큐
//	POST /remote/nodes/{instance_id}/approve — 승인(REQ-C03)
//	POST /remote/nodes/{instance_id}/reject  — 거부(REQ-C03)
//	POST /remote/nodes/{instance_id}/revoke  — 폐기(REQ-C07)
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// NodeAdminService 는 관리 노드 상태 머신 작업과 명령 디스패치를 추상화한다
// (*remote.Server 가 만족). 핸들러 테스트에서 fake 로 대체 가능하도록 인터페이스로
// 분리한다.
type NodeAdminService interface {
	// ListNodes 는 모든 관리 노드를 반환한다.
	ListNodes(ctx context.Context) ([]storage.ManagedNode, error)
	// Approve 는 노드를 승인한다(REQ-C03/C04). 미존재 시 storage.ErrManagedNodeNotFound.
	Approve(ctx context.Context, instanceID string) error
	// Reject 는 노드를 거부한다(REQ-C03). 미존재 시 storage.ErrManagedNodeNotFound.
	Reject(ctx context.Context, instanceID, reason string) error
	// Revoke 는 노드를 폐기한다(REQ-C07). 미존재 시 storage.ErrManagedNodeNotFound.
	Revoke(ctx context.Context, instanceID string) error
	// Dispatch 는 승인+온라인 노드에 원격 명령을 디스패치하고 결과를 기다린다(M3,
	// REQ-D01/D05/D06/D07/D08). 미승인/오프라인 시 remote.ErrNodeNotManaged.
	Dispatch(ctx context.Context, instanceID, domain, action string, args json.RawMessage) (json.RawMessage, error)

	// 인벤토리 미러 목록(M4, REQ-E05/E06). 노드별/통합 조회를 출처 노드 + online
	// 태그와 함께 반환한다. 알 수 없는 노드 조회는 storage.ErrManagedNodeNotFound.
	ListMirroredFlows(ctx context.Context, instanceID string) ([]remote.MirroredResourceView, error)
	ListMirroredAgents(ctx context.Context, instanceID string) ([]remote.MirroredResourceView, error)
	ListMirroredDevices(ctx context.Context, instanceID string) ([]remote.MirroredResourceView, error)
	ListAllMirroredFlows(ctx context.Context) ([]remote.MirroredResourceView, error)
	ListAllMirroredAgents(ctx context.Context) ([]remote.MirroredResourceView, error)
	ListAllMirroredDevices(ctx context.Context) ([]remote.MirroredResourceView, error)

	// NodeVersionHistory 는 노드 버전 변경 이력을 최신순으로 반환한다(버전 관리 Phase 1).
	// limit <= 0 이면 전체. VersionHistory 저장소 미구성 시 빈 슬라이스.
	NodeVersionHistory(ctx context.Context, instanceID string, limit int) ([]storage.NodeVersionHistory, error)
}

// commandRequest 는 원격 명령 발행 요청 본문이다(POST /remote/nodes/{id}/command).
type commandRequest struct {
	Domain string          `json:"domain"`
	Action string          `json:"action"`
	Args   json.RawMessage `json:"args,omitempty"`
}

// ManagedNodeDTO 는 관리 노드 응답 표현이다(시크릿 토큰은 노출하지 않음 — REQ-F06).
//
// GroupName 은 v1.4(M9, 그룹 K)의 단일 그룹 라벨이다(빈값="전체" 가상 버킷 —
// REQ-K01/K04). 프론트엔드 디렉토리 뷰가 단일 `/remote/nodes` 응답으로 그룹별
// 노드를 묶을 수 있도록 목록 항목에 포함한다(N+1 그룹별 조회 회피).
type ManagedNodeDTO struct {
	InstanceID string `json:"instance_id"`
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Online     bool   `json:"online"`
	GroupName  string `json:"group_name"` // 단일 그룹 라벨(빈값=전체 — REQ-K01/K04)
	LastSeen   int64  `json:"last_seen"`  // epoch ms
	// Outdated 는 관리자 지정 목표 버전 대비 이 노드가 구버전인지 여부이다(버전 관리
	// Phase 1). 목표 버전 미설정이거나 버전 문자열이 semver 가 아니면 false.
	Outdated bool `json:"outdated"`
}

// rejectRequest 는 거부 사유를 담는 선택적 요청 본문이다.
type rejectRequest struct {
	Reason string `json:"reason,omitempty"`
}

// RemoteAdminHandler 는 관리 노드 승인/거부/폐기/목록 엔드포인트를 처리한다.
type RemoteAdminHandler struct {
	svc      NodeAdminService
	audit    storage.RemoteAuditRepository
	settings storage.SettingsRepository
	logger   *slog.Logger
}

// NewRemoteAdminHandler 는 RemoteAdminHandler 를 생성한다.
func NewRemoteAdminHandler(svc NodeAdminService, logger *slog.Logger) *RemoteAdminHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteAdminHandler{svc: svc, logger: logger}
}

// WithAudit 는 원격 변경 감사 저장소를 연결한다(M6, REQ-F05). 설정되면 승인/거부/
// 폐기 mutation 을 actor 와 함께 영속 감사 레코드로 기록하고, GET /remote/audit 로
// 조회할 수 있게 한다. nil 이면 감사는 구조화 로그로만 남는다(하위 호환).
func (h *RemoteAdminHandler) WithAudit(audit storage.RemoteAuditRepository) *RemoteAdminHandler {
	h.audit = audit
	return h
}

// WithSettings 는 전역 설정 저장소를 연결한다(버전 관리 Phase 1). 설정되면 관리자가
// 지정한 목표 버전(target_version)을 GET/PUT 으로 관리하고, /remote/nodes 응답의
// outdated 플래그 계산에 사용한다. nil 이면 목표 버전 기능은 비활성(outdated 항상 false).
func (h *RemoteAdminHandler) WithSettings(settings storage.SettingsRepository) *RemoteAdminHandler {
	h.settings = settings
	return h
}

// recordAudit 는 mutation 감사 레코드를 기록한다(REQ-F05). audit 미구성이면 no-op.
// 시크릿은 기록하지 않는다(REQ-F06 — action/result/reason 만).
func (h *RemoteAdminHandler) recordAudit(ctx api.Context, instanceID, action, result, reason string) {
	if h.audit == nil {
		return
	}
	rec := storage.RemoteAuditRecord{
		InstanceID: instanceID,
		Actor:      ctx.UserID(),
		Action:     action,
		Result:     result,
		Reason:     reason,
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := h.audit.Append(ctx.Context(), rec); err != nil {
		h.logger.Warn("감사 레코드 기록 실패",
			"instance_id", instanceID, "action", action, "error", err)
	}
}

// RegisterRoutes 는 관리 노드 admin 라우트를 그룹에 등록한다.
func (h *RemoteAdminHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/remote/nodes", h.ListNodes)
	g.GET("/remote/nodes/pending", h.ListPending)
	g.POST("/remote/nodes/{instance_id}/approve", h.Approve)
	g.POST("/remote/nodes/{instance_id}/reject", h.Reject)
	g.POST("/remote/nodes/{instance_id}/revoke", h.Revoke)
	g.POST("/remote/nodes/{instance_id}/command", h.Command)

	// 인벤토리 미러 목록(M4, REQ-E05/E06). 노드별 + 통합(전 노드) 엔드포인트.
	g.GET("/remote/nodes/{instance_id}/flows", h.NodeFlows)
	g.GET("/remote/nodes/{instance_id}/agents", h.NodeAgents)
	g.GET("/remote/nodes/{instance_id}/devices", h.NodeDevices)
	g.GET("/remote/flows", h.AllFlows)
	g.GET("/remote/agents", h.AllAgents)
	g.GET("/remote/devices", h.AllDevices)

	// 원격 변경 감사 로그 조회(M6, REQ-F05). admin-gated, 선택적 instance_id 필터 +
	// limit/offset 페이지네이션. 감사 영속 관측성을 제공한다.
	g.GET("/remote/audit", h.Audit)

	// 버전 관리(Phase 1): 노드 버전 이력 조회 + 관리자 수동 목표 버전 GET/PUT.
	g.GET("/remote/nodes/{instance_id}/version-history", h.VersionHistory)
	g.GET("/remote/target-version", h.GetTargetVersion)
	g.PUT("/remote/target-version", h.PutTargetVersion)

	// 버전 관리 Phase 2: 노드 자가 업데이트 명령(system/update 디스패치).
	g.POST("/remote/nodes/{instance_id}/update", h.UpdateNode)
}

// requireAdmin 은 admin 권한을 강제한다. node/viewer/editor 등은 403(REQ-F04).
func requireAdmin(ctx api.Context) error {
	if ctx.UserRole() != "admin" {
		return api.ErrForbidden.WithMessage("admin 권한이 필요합니다")
	}
	return nil
}

// ListNodes 는 전체 관리 노드 목록을 반환한다. GET /remote/nodes
func (h *RemoteAdminHandler) ListNodes(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	nodes, err := h.svc.ListNodes(ctx.Context())
	if err != nil {
		return mapRemoteAdminError(err)
	}
	target := h.targetVersion(ctx.Context())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toManagedNodeDTOsWithTarget(nodes, target)))
}

// ListPending 는 pending 상태 노드만 반환한다. GET /remote/nodes/pending
func (h *RemoteAdminHandler) ListPending(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	nodes, err := h.svc.ListNodes(ctx.Context())
	if err != nil {
		return mapRemoteAdminError(err)
	}
	pending := make([]storage.ManagedNode, 0)
	for _, n := range nodes {
		if n.Status == "pending" {
			pending = append(pending, n)
		}
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toManagedNodeDTOs(pending)))
}

// Approve 는 노드를 승인한다. POST /remote/nodes/{instance_id}/approve
func (h *RemoteAdminHandler) Approve(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	if err := h.svc.Approve(ctx.Context(), id); err != nil {
		h.recordAudit(ctx, id, storage.AuditActionApprove, storage.AuditResultError, "")
		return mapRemoteAdminError(err)
	}
	// 원격 변경 감사(REQ-F05): 누가/언제/어느 노드/approve/result 를 영속 기록한다.
	h.recordAudit(ctx, id, storage.AuditActionApprove, storage.AuditResultOK, "")
	h.logger.Info("관리 노드 승인", "instance_id", id, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"instance_id": id,
		"status":      "approved",
	}))
}

// Reject 는 노드를 거부한다. POST /remote/nodes/{instance_id}/reject
func (h *RemoteAdminHandler) Reject(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	var req rejectRequest
	_ = ctx.Bind(&req) // 본문은 선택적(사유 없음 허용).

	if err := h.svc.Reject(ctx.Context(), id, req.Reason); err != nil {
		h.recordAudit(ctx, id, storage.AuditActionReject, storage.AuditResultError, req.Reason)
		return mapRemoteAdminError(err)
	}
	// 원격 변경 감사(REQ-F05). 거부 사유는 비밀이 아니므로 reason 에 기록한다.
	h.recordAudit(ctx, id, storage.AuditActionReject, storage.AuditResultOK, req.Reason)
	h.logger.Info("관리 노드 거부", "instance_id", id, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"instance_id": id,
		"status":      "rejected",
	}))
}

// Revoke 는 노드를 폐기한다. POST /remote/nodes/{instance_id}/revoke
func (h *RemoteAdminHandler) Revoke(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	if err := h.svc.Revoke(ctx.Context(), id); err != nil {
		h.recordAudit(ctx, id, storage.AuditActionRevoke, storage.AuditResultError, "")
		return mapRemoteAdminError(err)
	}
	// 원격 변경 감사(REQ-F05/F07).
	h.recordAudit(ctx, id, storage.AuditActionRevoke, storage.AuditResultOK, "")
	h.logger.Info("관리 노드 폐기", "instance_id", id, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"instance_id": id,
		"status":      "revoked",
	}))
}

// Command 는 승인+온라인 노드에 원격 명령을 발행한다(M3, REQ-D01/D08, F04).
// POST /remote/nodes/{instance_id}/command  본문: {domain, action, args}
//
// admin 권한만 명령을 발행할 수 있다(REQ-F04). 대상이 미승인/오프라인이면 503,
// 명령 타임아웃이면 504, 노드 적용 실패이면 502 로 매핑한다.
func (h *RemoteAdminHandler) Command(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	var req commandRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.Domain == "" || req.Action == "" {
		return api.ErrBadRequest.WithMessage("domain 과 action 은 필수입니다")
	}

	// 감사(F05): 명령 발행 — args 는 시크릿 가능성으로 로깅 제외(REQ-F06).
	h.logger.Info("원격 명령 발행",
		"instance_id", id, "domain", req.Domain, "action", req.Action,
		"actor", ctx.UserID())

	// actor(관리자 username)를 컨텍스트에 실어 Dispatch 로 전달한다. 서버(dispatch.go)
	// 가 명령 터미널 결과 시 actor 와 함께 영속 감사 레코드를 1행 기록한다(REQ-F05 —
	// 중복 방지: 명령 감사는 서버 측 1곳에서만 기록). 미러 편집→명령(E08) 경로에서도
	// 동일하게 actor 가 전파된다.
	dispatchCtx := remote.ContextWithActor(ctx.Context(), ctx.UserID())
	result, err := h.svc.Dispatch(dispatchCtx, id, req.Domain, req.Action, req.Args)
	if err != nil {
		return mapRemoteCommandError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"instance_id": id,
		"domain":      req.Domain,
		"action":      req.Action,
		"result":      result,
	}))
}

// RemoteAuditDTO 는 감사 레코드 응답 표현이다(M6, REQ-F05). 시크릿은 포함하지
// 않는다(REQ-F06 — action/domain/result 만).
type RemoteAuditDTO struct {
	ID            int64  `json:"id"`
	InstanceID    string `json:"instance_id"`
	Actor         string `json:"actor"`
	Action        string `json:"action"`
	Domain        string `json:"domain,omitempty"`
	CommandAction string `json:"command_action,omitempty"`
	Result        string `json:"result"`
	Reason        string `json:"reason,omitempty"`
	Timestamp     int64  `json:"timestamp"` // epoch ms
}

// Audit 는 원격 변경 감사 로그를 반환한다(M6, REQ-F05). GET /remote/audit
//
// admin 권한만 조회할 수 있다(REQ-F04). 선택적 ?instance_id= 필터, ?limit=/?offset=
// 페이지네이션을 지원한다(기본 limit=100). audit 미구성이면 빈 목록을 반환한다.
func (h *RemoteAdminHandler) Audit(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	if h.audit == nil {
		return ctx.JSON(http.StatusOK, dto.NewSuccessResponse([]RemoteAuditDTO{}))
	}
	instanceID := ctx.Query("instance_id")
	limit := parsePositiveInt(ctx.Query("limit"), 100)
	offset := parsePositiveInt(ctx.Query("offset"), 0)

	records, err := h.audit.List(ctx.Context(), instanceID, limit, offset)
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toRemoteAuditDTOs(records)))
}

// parsePositiveInt 는 쿼리 문자열을 음이 아닌 정수로 파싱한다. 빈 값/오류/음수는
// fallback 을 반환한다.
func parsePositiveInt(s string, fallback int) int {
	if s == "" {
		return fallback
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return fallback
	}
	return n
}

// toRemoteAuditDTOs 는 감사 레코드를 응답 DTO 로 변환한다.
func toRemoteAuditDTOs(records []storage.RemoteAuditRecord) []RemoteAuditDTO {
	out := make([]RemoteAuditDTO, 0, len(records))
	for _, r := range records {
		out = append(out, RemoteAuditDTO{
			ID:            r.ID,
			InstanceID:    r.InstanceID,
			Actor:         r.Actor,
			Action:        r.Action,
			Domain:        r.Domain,
			CommandAction: r.CommandAction,
			Result:        r.Result,
			Reason:        r.Reason,
			Timestamp:     r.Timestamp,
		})
	}
	return out
}

// MirroredResourceDTO 는 미러 자원 목록 응답 표현이다(M4, REQ-E04/E05/E06).
//
// SourceInstanceID 로 출처 노드를 태깅하고(REQ-E04/E05), Online 으로 출처 노드의
// 라이브 연결 상태를 표시한다(Online=false 는 last-known/offline 표식 — REQ-E06).
// Definition 은 노드가 redaction(F06)한 정의이므로 시크릿이 포함되지 않는다.
type MirroredResourceDTO struct {
	ID               string `json:"id"`
	SourceInstanceID string `json:"source_instance_id"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Status           string `json:"status,omitempty"`
	Definition       string `json:"definition,omitempty"`
	UpdatedAt        int64  `json:"updated_at"` // epoch ms
	Online           bool   `json:"online"`     // 출처 노드 라이브 상태(false=last-known)
}

// mirrorLister 는 노드별/통합 미러 조회 함수 시그니처이다.
type (
	nodeMirrorFn func(ctx context.Context, instanceID string) ([]remote.MirroredResourceView, error)
	allMirrorFn  func(ctx context.Context) ([]remote.MirroredResourceView, error)
)

// NodeFlows 는 한 노드의 flow 미러를 반환한다. GET /remote/nodes/{instance_id}/flows
func (h *RemoteAdminHandler) NodeFlows(ctx api.Context) error {
	return h.serveNodeMirror(ctx, h.svc.ListMirroredFlows)
}

// NodeAgents 는 한 노드의 agent 미러를 반환한다. GET /remote/nodes/{instance_id}/agents
func (h *RemoteAdminHandler) NodeAgents(ctx api.Context) error {
	return h.serveNodeMirror(ctx, h.svc.ListMirroredAgents)
}

// NodeDevices 는 한 노드의 device 미러를 반환한다. GET /remote/nodes/{instance_id}/devices
func (h *RemoteAdminHandler) NodeDevices(ctx api.Context) error {
	return h.serveNodeMirror(ctx, h.svc.ListMirroredDevices)
}

// AllFlows 는 전 노드의 flow 미러를 출처 태그와 함께 반환한다. GET /remote/flows
func (h *RemoteAdminHandler) AllFlows(ctx api.Context) error {
	return h.serveAllMirror(ctx, h.svc.ListAllMirroredFlows)
}

// AllAgents 는 전 노드의 agent 미러를 반환한다. GET /remote/agents
func (h *RemoteAdminHandler) AllAgents(ctx api.Context) error {
	return h.serveAllMirror(ctx, h.svc.ListAllMirroredAgents)
}

// AllDevices 는 전 노드의 device 미러를 반환한다. GET /remote/devices
func (h *RemoteAdminHandler) AllDevices(ctx api.Context) error {
	return h.serveAllMirror(ctx, h.svc.ListAllMirroredDevices)
}

// serveNodeMirror 는 노드별 미러 조회를 admin 게이트 후 응답한다(알 수 없는 노드 404).
func (h *RemoteAdminHandler) serveNodeMirror(ctx api.Context, list nodeMirrorFn) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	views, err := list(ctx.Context(), id)
	if err != nil {
		return mapRemoteAdminError(err) // ErrManagedNodeNotFound → 404.
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toMirroredDTOs(views)))
}

// serveAllMirror 는 통합 미러 조회를 admin 게이트 후 응답한다(REQ-E05).
func (h *RemoteAdminHandler) serveAllMirror(ctx api.Context, list allMirrorFn) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	views, err := list(ctx.Context())
	if err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toMirroredDTOs(views)))
}

// toMirroredDTOs 는 미러 뷰를 응답 DTO 로 변환한다(출처/online 태깅 — REQ-E04/E06).
func toMirroredDTOs(views []remote.MirroredResourceView) []MirroredResourceDTO {
	out := make([]MirroredResourceDTO, 0, len(views))
	for _, v := range views {
		out = append(out, MirroredResourceDTO{
			ID:               v.ID,
			SourceInstanceID: v.SourceInstanceID,
			Name:             v.Name,
			Kind:             v.Kind,
			Status:           v.Status,
			Definition:       v.Definition,
			UpdatedAt:        v.UpdatedAt,
			Online:           v.Online,
		})
	}
	return out
}

// toManagedNodeDTOs 는 저장소 모델을 응답 DTO 로 변환한다(토큰 식별자 제외 — REQ-F06).
func toManagedNodeDTOs(nodes []storage.ManagedNode) []ManagedNodeDTO {
	return toManagedNodeDTOsWithTarget(nodes, "")
}

// toManagedNodeDTOsWithTarget 는 목표 버전 대비 outdated 플래그를 계산하여 DTO 로
// 변환한다(버전 관리 Phase 1). target 이 빈 문자열이면 outdated 는 항상 false.
func toManagedNodeDTOsWithTarget(nodes []storage.ManagedNode, target string) []ManagedNodeDTO {
	out := make([]ManagedNodeDTO, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, ManagedNodeDTO{
			InstanceID: n.InstanceID,
			Hostname:   n.Hostname,
			Version:    n.Version,
			Status:     n.Status,
			Online:     n.Online,
			GroupName:  n.GroupName,
			LastSeen:   n.LastSeen,
			Outdated:   isOutdated(n.Version, target),
		})
	}
	return out
}

// mapRemoteAdminError 는 도메인 에러를 APIError 로 매핑한다.
//   - storage.ErrManagedNodeNotFound → 404
//   - 그 외 → 500
func mapRemoteAdminError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrManagedNodeNotFound) {
		return api.ErrNotFound.WithMessage(err.Error())
	}
	return api.ErrInternalServer.WithMessage(err.Error())
}

// mapRemoteCommandError 는 디스패치 도메인 에러를 APIError 로 매핑한다(M3).
//   - remote.ErrNodeNotManaged → 503 (미승인/오프라인 — 적용 불가, REQ-D08/B07)
//   - remote.ErrCommandTimeout → 504 (결과 미수신 — 미적용, REQ-D06)
//   - remote.ErrCommandFailed  → 502 (노드 적용 실패, REQ-D09)
//   - remote.ErrNoConn         → 503 (라이브 연결 부재)
//   - context.Canceled 등 그 외 → 500
func mapRemoteCommandError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, remote.ErrNodeNotManaged), errors.Is(err, remote.ErrNoConn):
		return api.ErrServiceUnavailable.WithMessage(err.Error())
	case errors.Is(err, remote.ErrCommandTimeout):
		return &api.APIError{HTTPCode: http.StatusGatewayTimeout, Code: "COMMAND_TIMEOUT", Message: err.Error()}
	case errors.Is(err, remote.ErrCommandFailed):
		return &api.APIError{HTTPCode: http.StatusBadGateway, Code: "COMMAND_FAILED", Message: err.Error()}
	default:
		return api.ErrInternalServer.WithMessage(err.Error())
	}
}
