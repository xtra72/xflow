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
	"errors"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

// NodeAdminService 는 관리 노드 상태 머신 작업을 추상화한다(*remote.Server 가 만족).
// 핸들러 테스트에서 fake 로 대체 가능하도록 인터페이스로 분리한다.
type NodeAdminService interface {
	// ListNodes 는 모든 관리 노드를 반환한다.
	ListNodes(ctx context.Context) ([]storage.ManagedNode, error)
	// Approve 는 노드를 승인한다(REQ-C03/C04). 미존재 시 storage.ErrManagedNodeNotFound.
	Approve(ctx context.Context, instanceID string) error
	// Reject 는 노드를 거부한다(REQ-C03). 미존재 시 storage.ErrManagedNodeNotFound.
	Reject(ctx context.Context, instanceID, reason string) error
	// Revoke 는 노드를 폐기한다(REQ-C07). 미존재 시 storage.ErrManagedNodeNotFound.
	Revoke(ctx context.Context, instanceID string) error
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
