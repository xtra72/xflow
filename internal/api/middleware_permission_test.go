package api

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @SPEC:SPEC-AUTH-005 (M4) — RequirePermission / Authorization 미들웨어 단위 테스트.

// stubAuthorizer 는 Authorizer 대체 구현이다.
type stubAuthorizer struct {
	granted map[string]bool
	err     error

	gotUsername   string
	gotRole       string
	gotPermission string
}

func (s *stubAuthorizer) HasPermission(_ context.Context, username, role, permission string) (bool, error) {
	s.gotUsername, s.gotRole, s.gotPermission = username, role, permission
	if s.err != nil {
		return false, s.err
	}
	return s.granted[permission], nil
}

// permTestRouter 는 인가 미들웨어가 붙은 최소 라우터를 만든다.
func permTestRouter(enabled bool, az Authorizer, logs *bytes.Buffer) *Router {
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := NewRouter()
	r.Use(Authorization(enabled, az, logger))
	g := r.Group("/api/v1")
	g.GETPerm("/things", "agent.read", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})
	g.DELETEPerm("/things/{id}", "agent.delete", func(ctx Context) error {
		return ctx.NoContent(http.StatusNoContent)
	})
	return r
}

func serve(r *Router, method, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	return rec
}

func TestRequirePermission_AllowsWhenGranted(t *testing.T) {
	az := &stubAuthorizer{granted: map[string]bool{"agent.read": true}}
	r := permTestRouter(true, az, &bytes.Buffer{})

	assert.Equal(t, http.StatusOK, serve(r, http.MethodGet, "/api/v1/things").Code)
	assert.Equal(t, "agent.read", az.gotPermission)
}

func TestRequirePermission_DeniesWithoutLeakingPermissionKey(t *testing.T) {
	logs := &bytes.Buffer{}
	az := &stubAuthorizer{granted: map[string]bool{}}
	r := permTestRouter(true, az, logs)

	rec := serve(r, http.MethodDelete, "/api/v1/things/t1")
	require.Equal(t, http.StatusForbidden, rec.Code)

	// 응답 본문은 부족한 권한 키를 노출하지 않는다 (spec.md §5).
	body := rec.Body.String()
	assert.NotContains(t, body, "agent.delete")
	assert.NotContains(t, body, "permission")
	assert.Contains(t, body, "FORBIDDEN")

	// 구조화 로그에는 username·permission·method·path 가 남는다 (spec.md §2.3).
	logged := logs.String()
	assert.Contains(t, logged, `"permission":"agent.delete"`)
	assert.Contains(t, logged, `"method":"DELETE"`)
	assert.Contains(t, logged, `"path":"/api/v1/things/t1"`)
	assert.Contains(t, logged, `"username"`)
}

// TestRequirePermission_PassThroughWhenDisabled 는 AC-10(인증 비활성 회귀 없음)을
// 미들웨어 수준에서 고정한다.
func TestRequirePermission_PassThroughWhenDisabled(t *testing.T) {
	az := &stubAuthorizer{granted: map[string]bool{}} // 전부 거부하도록 설정
	r := permTestRouter(false, az, &bytes.Buffer{})

	assert.Equal(t, http.StatusOK, serve(r, http.MethodGet, "/api/v1/things").Code)
	assert.Equal(t, http.StatusNoContent, serve(r, http.MethodDelete, "/api/v1/things/t1").Code)
	assert.Empty(t, az.gotPermission, "비활성 상태에서 인가 판정기가 호출되었다")
}

func TestRequirePermission_PassThroughWhenAuthorizerMissing(t *testing.T) {
	// 활성이지만 판정기가 주입되지 않은 경우 (WithAuthorizer 미사용).
	r := permTestRouter(true, nil, &bytes.Buffer{})
	assert.Equal(t, http.StatusOK, serve(r, http.MethodGet, "/api/v1/things").Code)
}

func TestRequirePermission_PassThroughWhenMiddlewareNotRegistered(t *testing.T) {
	// Authorization 글로벌 미들웨어가 없으면 컨텍스트에 설정이 없어 패스스루한다.
	r := NewRouter()
	g := r.Group("/api/v1")
	g.GETPerm("/things", "agent.read", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})
	assert.Equal(t, http.StatusOK, serve(r, http.MethodGet, "/api/v1/things").Code)
}

func TestRequirePermission_AuthorizerErrorIsInternalError(t *testing.T) {
	logs := &bytes.Buffer{}
	az := &stubAuthorizer{err: errors.New("db down")}
	r := permTestRouter(true, az, logs)

	rec := serve(r, http.MethodGet, "/api/v1/things")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, logs.String(), "인가 검사 실패")
}

// TestRequirePermission_PassesUsernameAndRole 는 인가 판정기가 username 과 토큰
// 클레임 역할을 모두 전달받는지 확인한다 (유효 역할 재조회의 전제).
func TestRequirePermission_PassesUsernameAndRole(t *testing.T) {
	az := &stubAuthorizer{granted: map[string]bool{"agent.read": true}}
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))

	r := NewRouter()
	// 인증 미들웨어 대신 컨텍스트에 주체를 직접 주입한다.
	r.Use(func(next HandlerFunc) HandlerFunc {
		return func(ctx Context) error {
			hctx := ctx.(*httpContext)
			c := context.WithValue(hctx.r.Context(), ctxKeyUserID, "kim")
			c = context.WithValue(c, ctxKeyUserRole, "operator")
			hctx.setRequest(hctx.r.WithContext(c))
			return next(hctx)
		}
	})
	r.Use(Authorization(true, az, logger))
	g := r.Group("/api/v1")
	g.GETPerm("/things", "agent.read", func(ctx Context) error {
		return ctx.NoContent(http.StatusOK)
	})

	require.Equal(t, http.StatusOK, serve(r, http.MethodGet, "/api/v1/things").Code)
	assert.Equal(t, "kim", az.gotUsername)
	assert.Equal(t, "operator", az.gotRole)
}

// TestRouteRegistryRecordsPermissions 는 *Perm 계열 등록이 권한 키를 기록하고
// 기존 계열은 빈 문자열을 남기는지 확인한다 (AC-08 커버리지 검사의 전제).
func TestRouteRegistryRecordsPermissions(t *testing.T) {
	r := NewRouter()
	g := r.Group("/api/v1")
	g.GETPerm("/things", "agent.read", func(ctx Context) error { return nil })
	g.POSTPerm("/things", "agent.create", func(ctx Context) error { return nil })
	g.PUTPerm("/things/{id}", "agent.update", func(ctx Context) error { return nil })
	g.PATCHPerm("/things/{id}", "agent.update", func(ctx Context) error { return nil })
	g.DELETEPerm("/things/{id}", "agent.delete", func(ctx Context) error { return nil })
	g.GET("/public", func(ctx Context) error { return nil })
	r.GET("/health", func(ctx Context) error { return nil })

	got := map[string]string{}
	for _, rec := range r.Routes() {
		got[rec.Method+" "+rec.Pattern] = rec.Permission
	}

	assert.Equal(t, "agent.read", got["GET /api/v1/things"])
	assert.Equal(t, "agent.create", got["POST /api/v1/things"])
	assert.Equal(t, "agent.update", got["PUT /api/v1/things/{id}"])
	assert.Equal(t, "agent.update", got["PATCH /api/v1/things/{id}"])
	assert.Equal(t, "agent.delete", got["DELETE /api/v1/things/{id}"])
	assert.Equal(t, "", got["GET /api/v1/public"])
	assert.Equal(t, "", got["GET /health"])
	assert.Equal(t, 7, r.RouteCount())
	assert.Len(t, r.Routes(), 7)
}
