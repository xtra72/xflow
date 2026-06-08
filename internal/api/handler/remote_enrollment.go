// remote_enrollment.go 는 수동 enrollment(그룹 H) 관리자 REST API 를 제공한다
// (@SPEC:SPEC-REMOTE-001 v1.1, REQ-REMOTE-H01/H03/H04/H06).
//
// 두 묶음의 엔드포인트를 제공한다(모두 admin 권한 강제 — REQ-F04):
//
//	사전 등록(경로 A):
//	  POST   /remote/nodes                       — 사전 승인 노드 생성(201, 중복 409)
//	  DELETE /remote/nodes/{instance_id}         — 노드 삭제(204)
//
//	enrollment 토큰(경로 B):
//	  POST   /remote/enrollment-tokens           — 토큰 발급(원본 토큰 1회 노출)
//	  GET    /remote/enrollment-tokens           — 토큰 메타 목록(토큰/해시 미노출)
//	  DELETE /remote/enrollment-tokens/{id}      — 토큰 폐기(204)
//
// 보안: 발급한 원본 토큰은 생성 응답에서 1회만 노출되고 저장소에는 SHA-256 해시만
// 영속된다(REQ-H06). 토큰 값은 로깅하지 않는다.
package handler

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// enrollmentTokenBytes 는 발급 토큰의 엔트로피 바이트 수이다. 32바이트 = 256비트
// (REQ-H06 — >=256비트 랜덤). base64url 인코딩 시 약 43자가 된다.
const enrollmentTokenBytes = 32

// PreRegistrationService 는 사전 등록 노드 생성/삭제를 추상화한다(*remote.Server 만족).
type PreRegistrationService interface {
	// PreRegister 는 instance_id 를 approved 로 사전 생성한다(REQ-H01). 중복 시
	// remote.ErrManagedNodeExists.
	PreRegister(ctx context.Context, instanceID, name string) error
	// RemoveNode 는 노드 등록을 제거한다(연결 종료 + 토큰 폐기 — REQ-H01). 미존재 시
	// storage.ErrManagedNodeNotFound.
	RemoveNode(ctx context.Context, instanceID string) error
}

// EnrollmentTokenService 는 enrollment 토큰 발급/목록/폐기를 추상화한다.
//
// 토큰 원본 생성과 해시 저장을 캡슐화하여 핸들러가 시크릿 처리를 직접 다루지 않게 한다.
type EnrollmentTokenService interface {
	// Issue 는 새 enrollment 토큰을 생성하고(>=256비트 랜덤), 해시를 저장한 뒤 원본
	// 토큰을 1회 반환한다(REQ-H06). expiresIn=0 이면 무기한, maxUses<=0 이면 무제한.
	Issue(ctx context.Context, label string, expiresIn time.Duration, maxUses int) (issued EnrollmentTokenIssued, err error)
	// List 는 토큰 메타데이터를 반환한다(토큰/해시 미노출 — REQ-H06).
	List(ctx context.Context) ([]storage.EnrollmentToken, error)
	// Revoke 는 토큰을 폐기한다(REQ-H04). 미존재 시 storage.ErrEnrollmentTokenNotFound.
	Revoke(ctx context.Context, id string) error
}

// EnrollmentTokenIssued 는 발급 결과이다(원본 토큰 1회 노출).
type EnrollmentTokenIssued struct {
	ID        string
	Token     string // 원본 토큰(1회 노출 — 이후 조회 불가)
	Label     string
	ExpiresAt *int64 // epoch ms
	MaxUses   *int
}

// enrollmentTokenSvc 는 EnrollmentTokenService 의 기본 구현이다(storage repo 위임).
type enrollmentTokenSvc struct {
	repo storage.EnrollmentTokenRepository
}

// NewEnrollmentTokenService 는 storage repo 를 래핑한 EnrollmentTokenService 를 만든다.
func NewEnrollmentTokenService(repo storage.EnrollmentTokenRepository) EnrollmentTokenService {
	return &enrollmentTokenSvc{repo: repo}
}

// Issue 는 256비트 랜덤 토큰을 생성하고 해시를 저장한다(REQ-H06).
func (s *enrollmentTokenSvc) Issue(ctx context.Context, label string, expiresIn time.Duration, maxUses int) (EnrollmentTokenIssued, error) {
	raw, err := generateEnrollmentToken()
	if err != nil {
		return EnrollmentTokenIssued{}, err
	}

	now := time.Now().UnixMilli()
	rec := storage.EnrollmentToken{
		ID:        uuid.NewString(),
		TokenHash: remote.HashEnrollmentToken(raw),
		Label:     label,
		CreatedAt: now,
	}
	if expiresIn > 0 {
		rec.ExpiresAt = storage.Int64Ptr(time.Now().Add(expiresIn).UnixMilli())
	}
	if maxUses > 0 {
		rec.MaxUses = storage.IntPtr(maxUses)
	}

	if err := s.repo.Create(ctx, rec); err != nil {
		return EnrollmentTokenIssued{}, err
	}
	return EnrollmentTokenIssued{
		ID:        rec.ID,
		Token:     raw,
		Label:     rec.Label,
		ExpiresAt: rec.ExpiresAt,
		MaxUses:   rec.MaxUses,
	}, nil
}

func (s *enrollmentTokenSvc) List(ctx context.Context) ([]storage.EnrollmentToken, error) {
	return s.repo.List(ctx)
}

func (s *enrollmentTokenSvc) Revoke(ctx context.Context, id string) error {
	return s.repo.Revoke(ctx, id)
}

// generateEnrollmentToken 은 256비트 암호학적 랜덤 토큰을 base64url(패딩 없음)로 만든다.
func generateEnrollmentToken() (string, error) {
	b := make([]byte, enrollmentTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// RemoteEnrollmentHandler 는 수동 enrollment 관리자 엔드포인트를 처리한다.
type RemoteEnrollmentHandler struct {
	preReg PreRegistrationService
	tokens EnrollmentTokenService
	logger *slog.Logger
}

// NewRemoteEnrollmentHandler 는 RemoteEnrollmentHandler 를 생성한다.
func NewRemoteEnrollmentHandler(preReg PreRegistrationService, tokens EnrollmentTokenService, logger *slog.Logger) *RemoteEnrollmentHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteEnrollmentHandler{preReg: preReg, tokens: tokens, logger: logger}
}

// RegisterRoutes 는 수동 enrollment 라우트를 그룹에 등록한다.
func (h *RemoteEnrollmentHandler) RegisterRoutes(g *api.RouteGroup) {
	g.POST("/remote/nodes", h.CreateNode)
	g.DELETE("/remote/nodes/{instance_id}", h.DeleteNode)

	g.POST("/remote/enrollment-tokens", h.CreateToken)
	g.GET("/remote/enrollment-tokens", h.ListTokens)
	g.DELETE("/remote/enrollment-tokens/{id}", h.RevokeToken)
}

// preRegisterRequest 는 사전 등록 노드 생성 요청 본문이다.
type preRegisterRequest struct {
	InstanceID string `json:"instance_id"`
	Name       string `json:"name,omitempty"`
}

// CreateNode 는 사전 승인 노드를 생성한다(REQ-H01). POST /remote/nodes
func (h *RemoteEnrollmentHandler) CreateNode(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	var req preRegisterRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.InstanceID == "" {
		return api.ErrBadRequest.WithMessage("instance_id 는 필수입니다")
	}

	if err := h.preReg.PreRegister(ctx.Context(), req.InstanceID, req.Name); err != nil {
		return mapEnrollmentError(err)
	}
	h.logger.Info("관리 노드 사전 등록", "instance_id", req.InstanceID, "actor", ctx.UserID())
	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(ManagedNodeDTO{
		InstanceID: req.InstanceID,
		Hostname:   req.Name,
		Status:     "approved",
		Online:     false,
	}))
}

// DeleteNode 는 노드 등록을 제거한다(REQ-H01). DELETE /remote/nodes/{instance_id}
func (h *RemoteEnrollmentHandler) DeleteNode(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	if err := h.preReg.RemoveNode(ctx.Context(), id); err != nil {
		return mapEnrollmentError(err)
	}
	h.logger.Info("관리 노드 삭제", "instance_id", id, "actor", ctx.UserID())
	return ctx.NoContent(http.StatusNoContent)
}

// createTokenRequest 는 enrollment 토큰 발급 요청 본문이다.
type createTokenRequest struct {
	Label     string `json:"label,omitempty"`
	ExpiresIn string `json:"expires_in,omitempty"` // duration 문자열(예: "24h")
	MaxUses   int    `json:"max_uses,omitempty"`
}

// EnrollmentTokenCreatedDTO 는 토큰 발급 응답이다(원본 토큰 1회 노출 — REQ-H06).
type EnrollmentTokenCreatedDTO struct {
	ID        string `json:"id"`
	Token     string `json:"token"` // 원본 토큰(1회 노출, 이후 조회 불가)
	Label     string `json:"label,omitempty"`
	ExpiresAt *int64 `json:"expires_at,omitempty"` // epoch ms
	MaxUses   *int   `json:"max_uses,omitempty"`
}

// EnrollmentTokenDTO 는 토큰 메타데이터 목록 항목이다(토큰/해시 미노출 — REQ-H06).
type EnrollmentTokenDTO struct {
	ID        string `json:"id"`
	Label     string `json:"label,omitempty"`
	CreatedAt int64  `json:"created_at"`
	ExpiresAt *int64 `json:"expires_at,omitempty"`
	MaxUses   *int   `json:"max_uses,omitempty"`
	Uses      int    `json:"uses"`
	Revoked   bool   `json:"revoked"`
}

// CreateToken 은 enrollment 토큰을 발급한다(REQ-H03). POST /remote/enrollment-tokens
func (h *RemoteEnrollmentHandler) CreateToken(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	var req createTokenRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	var expiresIn time.Duration
	if req.ExpiresIn != "" {
		d, perr := time.ParseDuration(req.ExpiresIn)
		if perr != nil || d <= 0 {
			return api.ErrBadRequest.WithMessage("expires_in 은 유효한 duration 이어야 합니다(예: \"24h\")")
		}
		expiresIn = d
	}

	issued, err := h.tokens.Issue(ctx.Context(), req.Label, expiresIn, req.MaxUses)
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	// 토큰 값은 로깅하지 않는다(REQ-H06) — id/label 만 기록.
	h.logger.Info("enrollment 토큰 발급",
		"token_id", issued.ID, "label", issued.Label, "actor", ctx.UserID())
	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(EnrollmentTokenCreatedDTO{
		ID:        issued.ID,
		Token:     issued.Token,
		Label:     issued.Label,
		ExpiresAt: issued.ExpiresAt,
		MaxUses:   issued.MaxUses,
	}))
}

// ListTokens 는 토큰 메타데이터 목록을 반환한다(REQ-H06). GET /remote/enrollment-tokens
func (h *RemoteEnrollmentHandler) ListTokens(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	tokens, err := h.tokens.List(ctx.Context())
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toEnrollmentTokenDTOs(tokens)))
}

// RevokeToken 은 토큰을 폐기한다(REQ-H04). DELETE /remote/enrollment-tokens/{id}
func (h *RemoteEnrollmentHandler) RevokeToken(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("id")
	if err := h.tokens.Revoke(ctx.Context(), id); err != nil {
		return mapEnrollmentError(err)
	}
	h.logger.Info("enrollment 토큰 폐기", "token_id", id, "actor", ctx.UserID())
	return ctx.NoContent(http.StatusNoContent)
}

// toEnrollmentTokenDTOs 는 저장소 모델을 메타데이터 DTO 로 변환한다(토큰/해시 제외 — REQ-H06).
func toEnrollmentTokenDTOs(tokens []storage.EnrollmentToken) []EnrollmentTokenDTO {
	out := make([]EnrollmentTokenDTO, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, EnrollmentTokenDTO{
			ID:        t.ID,
			Label:     t.Label,
			CreatedAt: t.CreatedAt,
			ExpiresAt: t.ExpiresAt,
			MaxUses:   t.MaxUses,
			Uses:      t.Uses,
			Revoked:   t.Revoked,
		})
	}
	return out
}

// mapEnrollmentError 는 도메인 에러를 APIError 로 매핑한다.
//   - remote.ErrManagedNodeExists        → 409 (중복 사전 등록)
//   - storage.ErrManagedNodeNotFound     → 404
//   - storage.ErrEnrollmentTokenNotFound → 404
//   - 그 외                              → 500
func mapEnrollmentError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, remote.ErrManagedNodeExists):
		return api.ErrConflict.WithMessage(err.Error())
	case errors.Is(err, storage.ErrManagedNodeNotFound),
		errors.Is(err, storage.ErrEnrollmentTokenNotFound):
		return api.ErrNotFound.WithMessage(err.Error())
	default:
		return api.ErrInternalServer.WithMessage(err.Error())
	}
}
