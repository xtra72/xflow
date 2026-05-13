// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-6, M-7)
// dashboard.go — /api/dashboards/{shared,mine} REST 핸들러.
//
// 책임:
//   - URL 로 scope 결정 (shared → global, mine → user)
//   - JWT Claims.Username 으로 owner 결정 (UB-003 spoofing 차단, UB-005 cross-user 격리)
//   - admin 권한 검증 (UB-004 — PUT/DELETE /shared 만)
//   - body scope vs URL scope 불일치 시 400 (UB-006)
//   - payload 크기 256KB 한도 (UR-003 → 413)
//   - If-Match 헤더 파싱 (낙관적 동시성 → 409)
//   - 응답 코드: 200/204/400/401/403/404/409/413/500 (UB-001 명시적)

package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/storage"
)

// maxDashboardPayloadBytes 는 PUT 페이로드의 최대 크기이다 (UR-003).
//
// SPEC ASM-004: 단일 snapshot JSON 크기는 256 KB 이하로 가정.
// 초과 시 413 Payload Too Large 로 거부.
const maxDashboardPayloadBytes = 256 * 1024

// DashboardHandler 는 공유/개인 대시보드 REST 엔드포인트를 처리한다.
//
// JWT 검증은 api.Auth 미들웨어가 담당하므로 (전역 적용) 본 핸들러는 401 을 직접
// 만들지 않는다. 대신 ctx.UserID() 가 빈 문자열이면 인증 컨텍스트가 누락된 것으로
// 간주하여 방어적으로 401 을 반환한다.
type DashboardHandler struct {
	repo   storage.DashboardRepository
	jwtSvc *auth.JWTService // nil 허용 (테스트용)
	logger *slog.Logger
}

// NewDashboardHandler 는 새 DashboardHandler 를 생성한다.
//
// jwtSvc 는 향후 추가 검증 (예: 토큰 재검증) 에 사용될 수 있다. 현재 구현은 미들웨어
// 가 주입한 ctx.UserID() / ctx.UserRole() 만 사용한다.
func NewDashboardHandler(repo storage.DashboardRepository, jwtSvc *auth.JWTService, logger *slog.Logger) *DashboardHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DashboardHandler{
		repo:   repo,
		jwtSvc: jwtSvc,
		logger: logger,
	}
}

// RegisterRoutes 는 6 개 라우트를 등록한다 (spec.md 라인 277 표 참조).
//
// Routes:
//
//	GET    /dashboards/shared  - 인증된 모든 사용자
//	PUT    /dashboards/shared  - admin only
//	DELETE /dashboards/shared  - admin only
//	GET    /dashboards/mine    - JWT username 으로 owner 결정
//	PUT    /dashboards/mine    - JWT username 으로 owner 결정
//	DELETE /dashboards/mine    - JWT username 으로 owner 결정
func (h *DashboardHandler) RegisterRoutes(g *api.RouteGroup) {
	// /shared: GET 은 인증된 모든 사용자, PUT/DELETE 는 admin 전용 (핸들러 레벨 검증).
	g.GET("/dashboards/shared", h.getShared)
	g.PUT("/dashboards/shared", h.putShared)
	g.DELETE("/dashboards/shared", h.deleteShared)

	// /mine: 모두 인증된 사용자 (본인 owner).
	g.GET("/dashboards/mine", h.getMine)
	g.PUT("/dashboards/mine", h.putMine)
	g.DELETE("/dashboards/mine", h.deleteMine)
}

// -----------------------------------------------------------------------------
// Shared (scope=global, owner="")
// -----------------------------------------------------------------------------

// getShared 는 공유 snapshot 을 조회한다. 인증된 모든 사용자 접근 가능.
func (h *DashboardHandler) getShared(ctx api.Context) error {
	if err := h.requireAuthenticated(ctx); err != nil {
		return err
	}
	return h.handleGet(ctx, "global", "")
}

// putShared 는 공유 snapshot 을 저장한다. admin only (UB-004).
func (h *DashboardHandler) putShared(ctx api.Context) error {
	if err := h.requireAuthenticated(ctx); err != nil {
		return err
	}
	if err := h.requireAdmin(ctx); err != nil {
		return err
	}
	return h.handlePut(ctx, "global", "")
}

// deleteShared 는 공유 snapshot 을 삭제한다. admin only (AC-17).
func (h *DashboardHandler) deleteShared(ctx api.Context) error {
	if err := h.requireAuthenticated(ctx); err != nil {
		return err
	}
	if err := h.requireAdmin(ctx); err != nil {
		return err
	}
	return h.handleDelete(ctx, "global", "")
}

// -----------------------------------------------------------------------------
// Mine (scope=user, owner=JWT username)
// -----------------------------------------------------------------------------

// getMine 는 본인 개인 snapshot 을 조회한다.
func (h *DashboardHandler) getMine(ctx api.Context) error {
	username, err := h.requireAuthenticatedUsername(ctx)
	if err != nil {
		return err
	}
	return h.handleGet(ctx, "user", username)
}

// putMine 는 본인 개인 snapshot 을 저장한다. owner 는 JWT username 으로만 결정됨
// (UB-005 cross-user 접근 차단).
func (h *DashboardHandler) putMine(ctx api.Context) error {
	username, err := h.requireAuthenticatedUsername(ctx)
	if err != nil {
		return err
	}
	return h.handlePut(ctx, "user", username)
}

// deleteMine 은 본인 개인 snapshot 을 삭제한다.
func (h *DashboardHandler) deleteMine(ctx api.Context) error {
	username, err := h.requireAuthenticatedUsername(ctx)
	if err != nil {
		return err
	}
	return h.handleDelete(ctx, "user", username)
}

// -----------------------------------------------------------------------------
// Common handlers
// -----------------------------------------------------------------------------

// handleGet 은 GET 동작 공통 로직: NotFound → 404, 성공 → 200.
func (h *DashboardHandler) handleGet(ctx api.Context, scope, owner string) error {
	snap, err := h.repo.Get(ctx.Context(), scope, owner)
	if err != nil {
		if errors.Is(err, storage.ErrDashboardNotFound) {
			return api.ErrNotFound.WithMessage("dashboard snapshot not found")
		}
		h.logger.Error("dashboard get 실패", "scope", scope, "owner", owner, "error", err)
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	// 표준 APIResponse envelope 으로 래핑 (UR-002, client.ts interceptor 호환).
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toDTO(snap)))
}

// handlePut 은 PUT 동작 공통 로직.
//
// 처리 순서:
//  1. Content-Length 헤더로 빠른 거부 (413).
//  2. body 를 LimitReader 로 읽어 256KB 초과 시 413.
//  3. JSON 파싱 (400 if invalid).
//  4. body 의 scope 가 명시되어 있고 URL 의 scope 와 다르면 400 (UB-006).
//  5. If-Match 헤더 파싱 (없으면 -1, 잘못된 형식이면 400).
//  6. Repository.Put 호출.
//  7. ErrDashboardVersionMismatch → 409 + 서버측 최신 snapshot.
//  8. 성공 → 200 + 갱신된 snapshot.
func (h *DashboardHandler) handlePut(ctx api.Context, scope, owner string) error {
	// 1. Content-Length 헤더 기반 사전 거부 (정확하지 않을 수 있으므로 reader 단계도 보강).
	if cl := ctx.GetHeader("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > maxDashboardPayloadBytes {
			return errPayloadTooLargeAPI()
		}
	}

	// 2. Body 를 limit reader 로 읽기 (256KB+1 까지 → 초과 검출).
	body, err := readLimitedBody(ctx, maxDashboardPayloadBytes)
	if err != nil {
		if errors.Is(err, errPayloadTooLarge) {
			return errPayloadTooLargeAPI()
		}
		return api.ErrBadRequest.WithMessage("read body: " + err.Error())
	}

	// 3. JSON 파싱.
	var req dto.DashboardPutRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}

	// 4. UB-006: body 의 scope 가 명시되어 있고 URL 과 불일치하면 400.
	if req.Scope != "" && req.Scope != scope {
		return api.ErrBadRequest.WithMessage(fmt.Sprintf(
			"scope mismatch: URL implies %q but body has %q", scope, req.Scope))
	}

	// 5. Payload 누락 시 빈 객체 허용 ({} payload는 PUT 의 일반적 reset 시나리오).
	payload := []byte(req.Payload)
	if len(payload) == 0 {
		payload = []byte(`{}`)
	}

	// 6. If-Match 헤더 파싱.
	expectedVersion := parseIfMatch(ctx.GetHeader("If-Match"))

	// 7. Put 호출.
	snap, err := h.repo.Put(ctx.Context(), scope, owner, payload, expectedVersion)
	if err != nil {
		if errors.Is(err, storage.ErrDashboardVersionMismatch) {
			// 409 + 서버측 최신 snapshot 을 body 에 포함시켜야 한다 (spec 라인 207, AC-10).
			latest, getErr := h.repo.Get(ctx.Context(), scope, owner)
			if getErr != nil {
				// 충돌이 났는데 latest 조회도 실패한 경우 — 가능성 낮으나 명시 처리.
				return api.ErrConflict.WithMessage("version mismatch (latest unavailable)")
			}
			// 409 도 envelope 으로 감싼다 — dashboardService.unwrapEnvelope 가 409 분기에서
			// response.data 를 unwrap 하므로, server snapshot 도 동일 envelope 규약을 따라야 한다.
			return ctx.JSON(http.StatusConflict, dto.NewSuccessResponse(toDTO(latest)))
		}
		h.logger.Error("dashboard put 실패", "scope", scope, "owner", owner, "error", err)
		return api.ErrInternalServer.WithMessage(err.Error())
	}

	h.logger.Info("dashboard snapshot saved",
		"scope", scope, "owner", owner, "version", snap.Version)
	// 표준 APIResponse envelope 으로 래핑 (UR-002, client.ts interceptor 호환).
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toDTO(snap)))
}

// handleDelete 는 DELETE 동작 공통 로직 → 204 No Content.
//
// Repository.Delete 는 멱등이므로 ErrDashboardNotFound 분기 불필요.
func (h *DashboardHandler) handleDelete(ctx api.Context, scope, owner string) error {
	if err := h.repo.Delete(ctx.Context(), scope, owner); err != nil {
		h.logger.Error("dashboard delete 실패", "scope", scope, "owner", owner, "error", err)
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	return ctx.NoContent(http.StatusNoContent)
}

// -----------------------------------------------------------------------------
// Auth helpers
// -----------------------------------------------------------------------------

// requireAuthenticated 는 ctx.UserID() 가 비어있지 않은지 확인한다.
//
// 운영 환경에서는 api.Auth 미들웨어가 이미 401 을 처리하지만, 본 핸들러는 미들웨어
// 누락 시 잘못된 동작을 방지하기 위해 방어적으로 검증한다.
func (h *DashboardHandler) requireAuthenticated(ctx api.Context) error {
	if ctx.UserID() == "" {
		return api.ErrUnauthorized.WithMessage("authentication required")
	}
	return nil
}

// requireAuthenticatedUsername 은 인증된 username 을 반환한다. 없으면 401.
func (h *DashboardHandler) requireAuthenticatedUsername(ctx api.Context) (string, error) {
	username := ctx.UserID()
	if username == "" {
		return "", api.ErrUnauthorized.WithMessage("authentication required")
	}
	return username, nil
}

// requireAdmin 은 ctx.UserRole() == "admin" 인지 확인한다 (UB-004).
func (h *DashboardHandler) requireAdmin(ctx api.Context) error {
	if ctx.UserRole() != "admin" {
		return api.ErrForbidden.WithMessage("admin 권한이 필요합니다")
	}
	return nil
}

// -----------------------------------------------------------------------------
// Body / header parsing
// -----------------------------------------------------------------------------

// errPayloadTooLarge 는 readLimitedBody 의 내부 sentinel.
var errPayloadTooLarge = errors.New("payload too large")

// errPayloadTooLargeAPI 는 256KB 초과 시 반환되는 APIError 를 생성한다 (HTTP 413).
//
// api 패키지의 사전 정의 sentinel 에는 413 이 없으므로 핸들러 레벨에서 직접 생성.
// router.handleError 가 APIError.HTTPCode 를 그대로 사용하여 응답 코드를 결정한다.
func errPayloadTooLargeAPI() *api.APIError {
	return &api.APIError{
		HTTPCode: http.StatusRequestEntityTooLarge,
		Code:     "PAYLOAD_TOO_LARGE",
		Message:  fmt.Sprintf("payload exceeds %d bytes", maxDashboardPayloadBytes),
	}
}

// readLimitedBody 는 ctx 의 요청 본문을 maxBytes 까지만 읽어 반환한다.
//
// LimitReader 로 maxBytes+1 만큼 읽어 정확히 한도 검출이 가능하다. maxBytes 초과 시
// errPayloadTooLarge 반환.
//
// 본 함수는 api.Context 인터페이스의 hidden interface assertion 으로 *http.Request
// 를 얻는다. api.httpContext 가 그 인터페이스를 구현하지 않으면 fallback 으로
// Bind 후 marshalled 바이트를 사용 (테스트에서는 fallback 경로 사용).
func readLimitedBody(ctx api.Context, maxBytes int64) ([]byte, error) {
	// httpContext 의 raw request 에 접근하는 우회 인터페이스.
	type requester interface {
		Request() *http.Request
	}
	if req, ok := ctx.(requester); ok && req.Request() != nil && req.Request().Body != nil {
		r := io.LimitReader(req.Request().Body, maxBytes+1)
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > maxBytes {
			return nil, errPayloadTooLarge
		}
		return data, nil
	}
	// Fallback: ctx.Bind 가 io.EOF 를 반환하면 빈 body 로 처리.
	var raw json.RawMessage
	if err := ctx.Bind(&raw); err != nil {
		// "request body is empty" → 빈 객체로 처리. 그 외 에러 전파.
		if errors.Is(err, io.EOF) {
			return []byte(`{}`), nil
		}
		// api.ErrBadRequest 에서 "request body is empty" 메시지로 들어오는 경우.
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, errPayloadTooLarge
	}
	return raw, nil
}

// parseIfMatch 는 If-Match 헤더를 int64 로 파싱한다.
//
// 헤더가 없거나 잘못된 형식이면 -1 (unconditional).
//
// 표준 If-Match 는 ETag 문자열을 사용하지만, 본 SPEC 은 단순 정수 version 을 사용
// 한다. 따옴표/W/ 접두사가 있을 경우 무시한다.
func parseIfMatch(header string) int64 {
	if header == "" {
		return -1
	}
	// 따옴표 제거, W/ 접두사 제거 (방어적)
	s := header
	if len(s) >= 2 && s[0] == 'W' && s[1] == '/' {
		s = s[2:]
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1
	}
	if n < 0 {
		return -1
	}
	return n
}

// -----------------------------------------------------------------------------
// DTO conversion
// -----------------------------------------------------------------------------

// toDTO 는 storage.DashboardSnapshot → dto.DashboardSnapshot 변환.
//
// scope=global 인 경우 Owner 필드를 nil 로 만들어 JSON null 직렬화를 보장한다.
// scope=user 인 경우 *string 으로 username 을 감싼다.
//
// Payload 는 저장된 JSON 원본을 그대로 통과시킨다 (re-encoding 없음, byte-stable).
func toDTO(s *storage.DashboardSnapshot) dto.DashboardSnapshot {
	var owner *string
	if s.Scope == "user" {
		v := s.Owner
		owner = &v
	}
	return dto.DashboardSnapshot{
		Scope:     s.Scope,
		Owner:     owner,
		Version:   s.Version,
		UpdatedAt: s.UpdatedAt,
		Payload:   s.Payload,
	}
}
