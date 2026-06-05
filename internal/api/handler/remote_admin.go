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
}

// commandRequest 는 원격 명령 발행 요청 본문이다(POST /remote/nodes/{id}/command).
type commandRequest struct {
	Domain string          `json:"domain"`
	Action string          `json:"action"`
	Args   json.RawMessage `json:"args,omitempty"`
}

// ManagedNodeDTO 는 관리 노드 응답 표현이다(시크릿 토큰은 노출하지 않음 — REQ-F06).
type ManagedNodeDTO struct {
	InstanceID string `json:"instance_id"`
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Online     bool   `json:"online"`
	LastSeen   int64  `json:"last_seen"` // epoch ms
}

// rejectRequest 는 거부 사유를 담는 선택적 요청 본문이다.
type rejectRequest struct {
	Reason string `json:"reason,omitempty"`
}

// RemoteAdminHandler 는 관리 노드 승인/거부/폐기/목록 엔드포인트를 처리한다.
type RemoteAdminHandler struct {
	svc    NodeAdminService
	logger *slog.Logger
}

// NewRemoteAdminHandler 는 RemoteAdminHandler 를 생성한다.
func NewRemoteAdminHandler(svc NodeAdminService, logger *slog.Logger) *RemoteAdminHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteAdminHandler{svc: svc, logger: logger}
}

// RegisterRoutes 는 관리 노드 admin 라우트를 그룹에 등록한다.
func (h *RemoteAdminHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/remote/nodes", h.ListNodes)
	g.GET("/remote/nodes/pending", h.ListPending)
	g.POST("/remote/nodes/{instance_id}/approve", h.Approve)
	g.POST("/remote/nodes/{instance_id}/reject", h.Reject)
	g.POST("/remote/nodes/{instance_id}/revoke", h.Revoke)
	g.POST("/remote/nodes/{instance_id}/command", h.Command)
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
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toManagedNodeDTOs(nodes)))
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
		return mapRemoteAdminError(err)
	}
	// M6 seam: 여기에 감사 로그(누가/언제/어느 노드/approve)를 기록한다(REQ-F05).
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
		return mapRemoteAdminError(err)
	}
	// M6 seam: 감사 로그(REQ-F05).
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
		return mapRemoteAdminError(err)
	}
	// M6 seam: 감사 로그(REQ-F05).
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
	// M6 seam: 감사 저장소 — 영속 감사 레코드(actor/ts/target/domain/action) 기록.
	h.logger.Info("원격 명령 발행",
		"instance_id", id, "domain", req.Domain, "action", req.Action,
		"actor", ctx.UserID())

	result, err := h.svc.Dispatch(ctx.Context(), id, req.Domain, req.Action, req.Args)
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

// toManagedNodeDTOs 는 저장소 모델을 응답 DTO 로 변환한다(토큰 식별자 제외 — REQ-F06).
func toManagedNodeDTOs(nodes []storage.ManagedNode) []ManagedNodeDTO {
	out := make([]ManagedNodeDTO, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, ManagedNodeDTO{
			InstanceID: n.InstanceID,
			Hostname:   n.Hostname,
			Version:    n.Version,
			Status:     n.Status,
			Online:     n.Online,
			LastSeen:   n.LastSeen,
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
