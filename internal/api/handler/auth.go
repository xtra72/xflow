package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
)

// PermissionLister 는 역할의 권한 키 목록을 조회한다.
//
// @SPEC:SPEC-AUTH-005 (M6) — /auth/me 의 permissions 배열 산출에 사용된다.
// *auth.PermissionCache 가 본 인터페이스를 만족한다. 주입되지 않으면 permissions 는
// 빈 배열이 된다 (인증 비활성 배포 등).
type PermissionLister interface {
	// Permissions 는 역할의 권한 키 목록을 반환한다.
	Permissions(ctx context.Context, role string) ([]string, error)
	// EffectiveRole 은 인가 판정에 실제로 사용되는 현재 역할을 반환한다.
	// 조회 대상이 없으면 fallback(토큰 클레임의 역할)을 그대로 돌려준다.
	EffectiveRole(ctx context.Context, username, fallback string) (string, error)
}

// AuthHandler 는 인증 관련 API 핸들러이다.
type AuthHandler struct {
	credentials *auth.CredentialsManager
	jwtSvc      *auth.JWTService
	permissions PermissionLister
	logger      *slog.Logger
}

// NewAuthHandler 는 새 AuthHandler를 생성한다.
func NewAuthHandler(credentials *auth.CredentialsManager, jwtSvc *auth.JWTService, logger *slog.Logger) *AuthHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AuthHandler{
		credentials: credentials,
		jwtSvc:      jwtSvc,
		logger:      logger,
	}
}

// WithPermissions 는 /auth/me 의 permissions 배열 산출에 사용할 조회기를 주입한다.
//
// @SPEC:SPEC-AUTH-005 (M6)
// 생성자 시그니처를 바꾸면 기존 호출자(cmd/xflowd, 테스트)가 전부 깨지므로 선택적
// 설정자로 추가한다. 주입하지 않으면 permissions 는 빈 배열이다.
func (h *AuthHandler) WithPermissions(p PermissionLister) *AuthHandler {
	h.permissions = p
	return h
}

// RegisterRoutes 는 인증 관련 라우트를 등록한다.
func (h *AuthHandler) RegisterRoutes(g *api.RouteGroup) {
	authGroup := g.Group("/auth")
	authGroup.POST("/login", h.login)
	authGroup.POST("/logout", h.logout)
	authGroup.POST("/refresh", h.refresh)
	authGroup.GET("/me", h.me)
	authGroup.PUT("/password", h.changePassword)
}

// RegisterAuthStatusRoute 는 인증 상태 확인 라우트를 등록한다.
// 인증 활성화 여부와 무관하게 항상 등록되어야 한다.
func RegisterAuthStatusRoute(g *api.RouteGroup, authEnabled bool) {
	g.GET("/auth/status", func(ctx api.Context) error {
		return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]bool{
			"auth_enabled": authEnabled,
		}))
	})
}

// login 은 사용자 인증 후 JWT 토큰을 발급한다.
// POST /api/v1/auth/login
func (h *AuthHandler) login(ctx api.Context) error {
	var req dto.LoginRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	// 입력 검증
	if req.Username == "" || req.Password == "" {
		return api.ErrBadRequest.WithMessage("username과 password는 필수입니다")
	}

	// 자격증명 검증 (REQ-N-002: 사용자 미발견과 비밀번호 오류를 구분하지 않음)
	user, err := h.credentials.Authenticate(req.Username, req.Password)
	if err != nil {
		h.logger.Warn("로그인 실패",
			"username", req.Username,
			"remote_ip", ctx.RealIP(),
		)
		return api.ErrUnauthorized.WithMessage("인증 실패")
	}

	// JWT 토큰 생성
	accessToken, refreshToken, expiresAt, err := h.jwtSvc.GenerateTokens(user.Username, user.Role)
	if err != nil {
		h.logger.Error("토큰 생성 실패", "error", err)
		return api.ErrInternalServer.WithMessage("토큰 생성 실패")
	}

	h.logger.Info("로그인 성공",
		"username", user.Username,
		"role", user.Role,
		"remote_ip", ctx.RealIP(),
	)

	// SPEC-AUTH-004 U1: 응답을 {user, tokens} 중첩 구조로 조립한다.
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.LoginResponse{
		User: dto.UserInfoResponse{
			Username: user.Username,
			Role:     user.Role,
		},
		Tokens: dto.TokenPair{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
			ExpiresAt:    expiresAt,
			TokenType:    "Bearer",
		},
	}))
}

// logout 은 현재 토큰을 블랙리스트에 추가한다.
// POST /api/v1/auth/logout
func (h *AuthHandler) logout(ctx api.Context) error {
	// Authorization 헤더에서 토큰 추출
	token := extractBearerToken(ctx.GetHeader("Authorization"))
	if token == "" {
		return api.ErrUnauthorized.WithMessage("Authorization 헤더가 필요합니다")
	}

	// 토큰을 블랙리스트에 추가
	h.jwtSvc.Blacklist(token)

	h.logger.Info("로그아웃",
		"username", ctx.UserID(),
		"remote_ip", ctx.RealIP(),
	)

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"message": "로그아웃 성공",
	}))
}

// refresh 는 리프레시 토큰으로 새 액세스 토큰을 발급한다.
// POST /api/v1/auth/refresh
func (h *AuthHandler) refresh(ctx api.Context) error {
	var req dto.RefreshRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if req.RefreshToken == "" {
		return api.ErrBadRequest.WithMessage("refresh_token은 필수입니다")
	}

	// 블랙리스트 확인
	if h.jwtSvc.IsBlacklisted(req.RefreshToken) {
		return api.ErrUnauthorized.WithMessage("만료된 리프레시 토큰입니다")
	}

	// 리프레시 토큰 검증
	claims, err := h.jwtSvc.ValidateToken(req.RefreshToken)
	if err != nil {
		return api.ErrUnauthorized.WithMessage("유효하지 않은 리프레시 토큰입니다")
	}

	// 기존 리프레시 토큰 블랙리스트 처리 (토큰 회전)
	h.jwtSvc.Blacklist(req.RefreshToken)

	// 새 토큰 쌍 생성
	accessToken, refreshToken, expiresAt, err := h.jwtSvc.GenerateTokens(claims.Username, claims.Role)
	if err != nil {
		h.logger.Error("토큰 갱신 실패", "error", err)
		return api.ErrInternalServer.WithMessage("토큰 갱신 실패")
	}

	h.logger.Debug("토큰 갱신",
		"username", claims.Username,
	)

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.RefreshResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresAt:    expiresAt,
		TokenType:    "Bearer",
	}))
}

// me 는 현재 인증된 사용자 정보를 반환한다.
// GET /api/v1/auth/me
func (h *AuthHandler) me(ctx api.Context) error {
	username := ctx.UserID()
	role := ctx.UserRole()

	if username == "" {
		return api.ErrUnauthorized.WithMessage("인증이 필요합니다")
	}

	// @SPEC:SPEC-AUTH-005 (M6, acceptance.md AC-06)
	// 기존 필드(username, role)는 임베드로 위치·형식을 그대로 유지하고 permissions 만
	// 덧붙인다. 조회 실패는 로그만 남기고 빈 배열로 응답한다 — /auth/me 는 세션 유지에
	// 쓰이므로 권한 조회 장애로 로그인 상태가 끊기면 안 된다.
	//
	// 역할은 인가 미들웨어와 동일한 유효 역할(users 테이블 기준)로 보고한다. 토큰
	// 클레임을 그대로 쓰면 강등 직후 "권한이 있다고 표시되는데 요청은 403" 인
	// 불일치가 생겨 UI 게이팅이 어긋난다.
	perms := []string{}
	if h.permissions != nil {
		effective, err := h.permissions.EffectiveRole(ctx.Context(), username, role)
		if err != nil {
			h.logger.Error("유효 역할 조회 실패", "username", username, "role", role, "error", err)
		} else {
			role = effective
		}
		if role != "" {
			got, err := h.permissions.Permissions(ctx.Context(), role)
			if err != nil {
				h.logger.Error("권한 목록 조회 실패", "username", username, "role", role, "error", err)
			} else if got != nil {
				perms = got
			}
		}
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.MeResponse{
		UserInfoResponse: dto.UserInfoResponse{
			Username: username,
			Role:     role,
		},
		Permissions: perms,
	}))
}

// changePassword 는 현재 사용자의 비밀번호를 변경한다.
// PUT /api/v1/auth/password
func (h *AuthHandler) changePassword(ctx api.Context) error {
	username := ctx.UserID()
	if username == "" {
		return api.ErrUnauthorized.WithMessage("인증이 필요합니다")
	}

	var req dto.ChangePasswordRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if req.CurrentPassword == "" || req.NewPassword == "" {
		return api.ErrBadRequest.WithMessage("current_password와 new_password는 필수입니다")
	}

	// 비밀번호 변경
	if err := h.credentials.ChangePassword(username, req.CurrentPassword, req.NewPassword); err != nil {
		if err == auth.ErrInvalidCredentials {
			return api.ErrUnauthorized.WithMessage("현재 비밀번호가 일치하지 않습니다")
		}
		if err == auth.ErrPasswordTooShort {
			return api.ErrBadRequest.WithMessage("새 비밀번호는 최소 4자 이상이어야 합니다")
		}
		h.logger.Error("비밀번호 변경 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("비밀번호 변경 실패")
	}

	h.logger.Info("비밀번호 변경 성공",
		"username", username,
		"remote_ip", ctx.RealIP(),
	)

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"message": "비밀번호가 변경되었습니다",
	}))
}

// extractBearerToken 은 Authorization 헤더에서 Bearer 토큰을 추출한다.
func extractBearerToken(authHeader string) string {
	if authHeader == "" {
		return ""
	}
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(authHeader, "Bearer ")
}
