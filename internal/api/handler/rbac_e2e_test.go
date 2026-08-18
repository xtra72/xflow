package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/internal/storage"
)

// @SPEC:SPEC-AUTH-005 (M4/M5/M6) — 인가 강제 · 사용자/역할 API 인수 테스트.
//
// acceptance.md AC-03 ~ AC-10 및 엣지 케이스를 실제 라우터·SQLite 위에서 검증한다.

// rbacEnv 는 Auth + Authorization 미들웨어가 모두 붙은 실제 라우터 환경이다.
type rbacEnv struct {
	t      *testing.T
	db     *sql.DB
	router *api.Router
	jwt    *auth.JWTService
	cache  *auth.PermissionCache
	logs   *bytes.Buffer
}

// newRBACEnv 는 인증·인가가 활성화된 테스트 환경을 만든다.
//
// OpenSQLiteDB 가 roles/role_permissions 스키마 생성과 빌트인 역할 시드를 수행하므로
// (M1) 별도 준비 없이 admin/editor/viewer 가 존재한다.
func newRBACEnv(t *testing.T) *rbacEnv {
	t.Helper()
	return newRBACEnvWithAuth(t, true)
}

// newRBACEnvWithAuth 는 인증 활성화 여부를 지정해 환경을 만든다.
// authEnabled=false 는 AC-10(인증 비활성 회귀 없음) 검증용이다.
func newRBACEnvWithAuth(t *testing.T, authEnabled bool) *rbacEnv {
	t.Helper()

	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "rbac-e2e.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	jwtSvc, err := auth.NewJWTService("test-secret-for-rbac-e2e", "15m", "168h")
	require.NoError(t, err)

	resolver := auth.NewSQLPermissionResolver(db)
	cache := auth.NewPermissionCache(resolver).WithUserRoles(resolver)
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var jwtForAuth *auth.JWTService
	if authEnabled {
		jwtForAuth = jwtSvc
	}

	router := api.NewRouter()
	router.Use(api.Auth(authEnabled, jwtForAuth))
	router.Use(api.Authorization(authEnabled, cache, logger))

	g := router.Group("/api/v1")
	cm := auth.NewCredentialsManager(db, "")
	NewAuthHandler(cm, jwtSvc, logger).WithPermissions(cache).RegisterRoutes(g)
	NewUserHandler(db, cache, logger).RegisterRoutes(g)
	NewRoleHandler(db, cache, logger).RegisterRoutes(g)
	NewAgentHandler(&mockAgentManager{}, logger).RegisterRoutes(g)

	env := &rbacEnv{t: t, db: db, router: router, jwt: jwtSvc, cache: cache, logs: logs}
	env.addUser("admin", "adminpassword", rbac.RoleAdmin)
	return env
}

// addUser 는 사용자를 DB 에 직접 삽입한다 (API 를 거치지 않는 픽스처 준비용).
func (e *rbacEnv) addUser(username, password, role string) {
	e.t.Helper()
	hash, err := auth.HashPassword(password)
	require.NoError(e.t, err)
	require.NoError(e.t, storage.InsertUser(context.Background(), e.db, username, hash, role, 0, 0))
}

// token 은 username/role 에 대한 액세스 토큰을 발급한다.
func (e *rbacEnv) token(username, role string) string {
	e.t.Helper()
	access, _, _, err := e.jwt.GenerateTokens(username, role)
	require.NoError(e.t, err)
	return access
}

// do 는 토큰을 붙여 요청을 수행한다. token 이 빈 문자열이면 헤더를 붙이지 않는다.
func (e *rbacEnv) do(method, path string, body any, token string) *httptest.ResponseRecorder {
	e.t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(e.t, err)
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, r)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.Handler().ServeHTTP(rec, req)
	return rec
}

// decodeEnvelopeData 는 성공 응답 엔벨로프의 data 를 out 으로 디코딩한다.
func decodeEnvelopeData(t *testing.T, rec *httptest.ResponseRecorder, out any) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), "body=%s", rec.Body.String())
	require.True(t, env.Success, "body=%s", rec.Body.String())
	require.NoError(t, json.Unmarshal(env.Data, out))
}

// errorCode 는 에러 응답의 code 를 반환한다.
func errorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Error *dto.ErrorDetail `json:"error"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env), "body=%s", rec.Body.String())
	require.NotNil(t, env.Error, "에러 응답이 아니다: %s", rec.Body.String())
	return env.Error.Code
}

// --- AC-03: 역할 생성과 권한 조합 ---

func TestAC03_RoleCreationAndPermissionComposition(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	rec := env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name:        "operator",
		Description: "운영자",
		Permissions: []string{"agent.read", "agent.execute", "device.read"},
	}, adminToken)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// GET /roles 응답에 권한 3개와 함께 나타난다.
	listRec := env.do(http.MethodGet, "/api/v1/roles", nil, adminToken)
	require.Equal(t, http.StatusOK, listRec.Code)
	var roles []dto.RoleResponse
	decodeEnvelopeData(t, listRec, &roles)

	var operator *dto.RoleResponse
	for i := range roles {
		if roles[i].Name == "operator" {
			operator = &roles[i]
		}
	}
	require.NotNil(t, operator, "operator 역할이 목록에 없다")
	assert.False(t, operator.Builtin)
	assert.ElementsMatch(t, []string{"agent.read", "agent.execute", "device.read"}, operator.Permissions)

	// 카탈로그에 없는 권한 키는 400 이며 역할이 생성되지 않는다.
	badRec := env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name:        "bad-role",
		Permissions: []string{"agent.read", "agent.launch"},
	}, adminToken)
	assert.Equal(t, http.StatusBadRequest, badRec.Code, badRec.Body.String())

	_, err := storage.GetRoleByName(context.Background(), env.db, "bad-role")
	assert.ErrorIs(t, err, storage.ErrRoleNotFound, "거부된 역할이 생성되었다")
}

// --- AC-04: 사용자 등록과 역할 부여 ---

func TestAC04_UserCreationAndRoleAssignment(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name:        "operator",
		Permissions: []string{"agent.read", "agent.execute", "device.read"},
	}, adminToken).Code)

	rec := env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
		Username: "kim",
		Password: "kimpassword",
		Role:     "operator",
	}, adminToken)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "password_hash",
		"응답 본문에 password_hash 가 노출되었다")

	// kim 계정으로 로그인이 성공한다.
	loginRec := env.do(http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{
		Username: "kim", Password: "kimpassword",
	}, "")
	require.Equal(t, http.StatusOK, loginRec.Code, loginRec.Body.String())
	var login dto.LoginResponse
	decodeEnvelopeData(t, loginRec, &login)
	require.NotEmpty(t, login.Tokens.AccessToken)

	// /auth/me 의 permissions 가 operator 역할의 권한 3개와 일치한다.
	meRec := env.do(http.MethodGet, "/api/v1/auth/me", nil, login.Tokens.AccessToken)
	require.Equal(t, http.StatusOK, meRec.Code)
	var me dto.MeResponse
	decodeEnvelopeData(t, meRec, &me)
	assert.Equal(t, "kim", me.Username)
	assert.Equal(t, "operator", me.Role)
	assert.ElementsMatch(t, []string{"agent.read", "agent.execute", "device.read"}, me.Permissions)
}

// --- AC-05: 역할 변경 즉시 반영 (토큰 재발급 불필요) ---

func TestAC05_RoleChangeTakesEffectWithoutTokenReissue(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name:        "operator",
		Permissions: []string{"agent.read", "agent.execute", "device.read"},
	}, adminToken).Code)
	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
		Username: "kim", Password: "kimpassword", Role: "operator",
	}, adminToken).Code)

	// kim 이 operator 로 발급받은 토큰. 이후 재발급하지 않는다.
	kimToken := env.token("kim", "operator")
	require.Equal(t, http.StatusOK,
		env.do(http.MethodPost, "/api/v1/agents/a1/start", map[string]any{}, kimToken).Code)

	// 관리자가 kim 을 viewer 로 강등한다.
	updRec := env.do(http.MethodPut, "/api/v1/users/kim", dto.UpdateUserRequest{Role: rbac.RoleViewer}, adminToken)
	require.Equal(t, http.StatusOK, updRec.Code, updRec.Body.String())

	// 기존 토큰 그대로(재발급 없음): start 는 403, 목록 조회는 여전히 200.
	//
	// kimToken 의 클레임은 여전히 role=operator 이다. 인가가 토큰 클레임이 아니라
	// users 테이블의 현재 역할로 판정되어야만 이 단언이 성립한다.
	assert.Equal(t, http.StatusForbidden,
		env.do(http.MethodPost, "/api/v1/agents/a1/start", map[string]any{}, kimToken).Code)
	assert.Equal(t, http.StatusOK,
		env.do(http.MethodGet, "/api/v1/agents", nil, kimToken).Code)

	// /auth/me 도 강등된 역할과 그 권한을 보고한다 (UI 게이팅 불일치 방지).
	meRec := env.do(http.MethodGet, "/api/v1/auth/me", nil, kimToken)
	require.Equal(t, http.StatusOK, meRec.Code)
	var me dto.MeResponse
	decodeEnvelopeData(t, meRec, &me)
	assert.Equal(t, rbac.RoleViewer, me.Role)
	assert.NotContains(t, me.Permissions, "agent.execute")
}

// TestAC05_PermissionRevocationIsImmediate 는 역할의 권한 자체를 축소했을 때
// 토큰 재발급 없이 다음 요청부터 반영되는지 검증한다 (캐시 무효화 경로).
func TestAC05_PermissionRevocationIsImmediate(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name:        "operator",
		Permissions: []string{"agent.read", "agent.execute"},
	}, adminToken).Code)

	kimToken := env.token("kim", "operator")
	require.Equal(t, http.StatusOK,
		env.do(http.MethodPost, "/api/v1/agents/a1/start", map[string]any{}, kimToken).Code)

	// agent.execute 를 회수한다 → 동일 토큰으로 즉시 403.
	perms := []string{"agent.read"}
	updRec := env.do(http.MethodPut, "/api/v1/roles/operator", dto.UpdateRoleRequest{Permissions: &perms}, adminToken)
	require.Equal(t, http.StatusOK, updRec.Code, updRec.Body.String())

	assert.Equal(t, http.StatusForbidden,
		env.do(http.MethodPost, "/api/v1/agents/a1/start", map[string]any{}, kimToken).Code)
	assert.Equal(t, http.StatusOK, env.do(http.MethodGet, "/api/v1/agents", nil, kimToken).Code)
}

// --- AC-06: /auth/me 하위 호환 ---

func TestAC06_AuthMeBackwardCompatible(t *testing.T) {
	env := newRBACEnv(t)
	viewerToken := env.token("v1", rbac.RoleViewer)

	rec := env.do(http.MethodGet, "/api/v1/auth/me", nil, viewerToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 기존 필드가 이전과 동일한 위치·형식(문자열)으로 존재한다.
	var env2 struct {
		Success bool `json:"success"`
		Data    struct {
			Username    string    `json:"username"`
			Role        string    `json:"role"`
			Permissions *[]string `json:"permissions"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env2))
	assert.True(t, env2.Success)
	assert.Equal(t, "v1", env2.Data.Username)
	assert.Equal(t, rbac.RoleViewer, env2.Data.Role)

	// permissions 배열이 추가되어 있고 null 이 아니다.
	require.NotNil(t, env2.Data.Permissions, "permissions 가 누락되었거나 null 이다")
	assert.NotEmpty(t, *env2.Data.Permissions)

	// data 객체의 키 순서상 username, role 이 permissions 보다 앞에 온다 (위치 보존).
	body := rec.Body.String()
	assert.Less(t, strings.Index(body, `"username"`), strings.Index(body, `"permissions"`))
	assert.Less(t, strings.Index(body, `"role"`), strings.Index(body, `"permissions"`))
}

// TestAC06_LoginResponseUnchanged 는 로그인 응답이 permissions 필드 추가의 영향을
// 받지 않음을 확인한다 (인증 흐름 무변경).
func TestAC06_LoginResponseUnchanged(t *testing.T) {
	env := newRBACEnv(t)

	rec := env.do(http.MethodPost, "/api/v1/auth/login", dto.LoginRequest{
		Username: "admin", Password: "adminpassword",
	}, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.NotContains(t, rec.Body.String(), "permissions",
		"로그인 응답 구조가 변경되었다")

	var login dto.LoginResponse
	decodeEnvelopeData(t, login2Recorder(rec), &login)
	assert.Equal(t, "admin", login.User.Username)
	assert.Equal(t, rbac.RoleAdmin, login.User.Role)
	assert.Equal(t, "Bearer", login.Tokens.TokenType)
}

// login2Recorder 는 가독성을 위한 항등 헬퍼이다.
func login2Recorder(rec *httptest.ResponseRecorder) *httptest.ResponseRecorder { return rec }

// --- AC-07: 권한 없는 요청 차단 ---

func TestAC07_ViewerIsDeniedWriteOperations(t *testing.T) {
	env := newRBACEnv(t)
	viewerToken := env.token("v1", rbac.RoleViewer)

	cases := []struct {
		method string
		path   string
		body   any
		want   int
	}{
		{http.MethodGet, "/api/v1/agents", nil, http.StatusOK},
		{http.MethodPost, "/api/v1/agents", map[string]any{"name": "x", "type": "y"}, http.StatusForbidden},
		{http.MethodPut, "/api/v1/agents/a1", map[string]any{"name": "x"}, http.StatusForbidden},
		{http.MethodDelete, "/api/v1/agents/a1", nil, http.StatusForbidden},
		{http.MethodPost, "/api/v1/agents/a1/start", map[string]any{}, http.StatusForbidden},
		{http.MethodGet, "/api/v1/users", nil, http.StatusForbidden},
	}
	for _, tc := range cases {
		rec := env.do(tc.method, tc.path, tc.body, viewerToken)
		assert.Equal(t, tc.want, rec.Code, "%s %s → %s", tc.method, tc.path, rec.Body.String())

		if tc.want == http.StatusForbidden {
			// 403 응답 본문에 부족한 권한 키가 노출되지 않는다.
			body := rec.Body.String()
			assert.NotContains(t, body, "agent.")
			assert.NotContains(t, body, "user.")
			assert.NotContains(t, body, "permission")
		}
	}

	// 서버 로그에 username·permission·method·path 가 남는다.
	logs := env.logs.String()
	assert.Contains(t, logs, `"username":"v1"`)
	assert.Contains(t, logs, `"permission":"agent.create"`)
	assert.Contains(t, logs, `"method":"POST"`)
	assert.Contains(t, logs, `"path":"/api/v1/agents"`)
	assert.Contains(t, logs, `"permission":"user.read"`)
}

// --- AC-09: 잠금 방지 불변식 ---

func TestAC09_LockoutPreventionInvariants(t *testing.T) {
	t.Run("마지막 관리자 삭제는 409", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		rec := env.do(http.MethodDelete, "/api/v1/users/admin", nil, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "LAST_ADMIN_USER", errorCode(t, rec))

		_, err := storage.GetUserByUsername(context.Background(), env.db, "admin")
		assert.NoError(t, err, "거부되었는데 사용자가 삭제되었다")
	})

	t.Run("마지막 관리자 강등은 409", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		rec := env.do(http.MethodPut, "/api/v1/users/admin",
			dto.UpdateUserRequest{Role: rbac.RoleViewer}, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "LAST_ADMIN_USER", errorCode(t, rec))

		row, err := storage.GetUserByUsername(context.Background(), env.db, "admin")
		require.NoError(t, err)
		assert.Equal(t, rbac.RoleAdmin, row.Role, "거부되었는데 역할이 변경되었다")
	})

	t.Run("자기 자신 삭제는 409", func(t *testing.T) {
		env := newRBACEnv(t)
		// 관리 권한 보유자를 2명으로 만들어 마지막-관리자 불변식과 분리한다.
		env.addUser("admin2", "admin2password", rbac.RoleAdmin)
		adminToken := env.token("admin", rbac.RoleAdmin)

		rec := env.do(http.MethodDelete, "/api/v1/users/admin", nil, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "SELF_DELETION", errorCode(t, rec))

		_, err := storage.GetUserByUsername(context.Background(), env.db, "admin")
		assert.NoError(t, err, "거부되었는데 사용자가 삭제되었다")
	})

	t.Run("빌트인 역할 viewer 삭제는 409", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		rec := env.do(http.MethodDelete, "/api/v1/roles/viewer", nil, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "BUILTIN_ROLE_IMMUTABLE", errorCode(t, rec))

		_, err := storage.GetRoleByName(context.Background(), env.db, rbac.RoleViewer)
		assert.NoError(t, err, "거부되었는데 역할이 삭제되었다")
	})

	t.Run("admin 역할 권한 축소는 409", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		perms := []string{"agent.read"}
		rec := env.do(http.MethodPut, "/api/v1/roles/admin",
			dto.UpdateRoleRequest{Permissions: &perms}, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "ADMIN_ROLE_IMMUTABLE", errorCode(t, rec))

		row, err := storage.GetRoleByName(context.Background(), env.db, rbac.RoleAdmin)
		require.NoError(t, err)
		assert.ElementsMatch(t, rbac.Permissions(), row.Permissions, "admin 권한이 축소되었다")
	})

	t.Run("사용 중인 역할 삭제는 409", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
			Name: "operator", Permissions: []string{"agent.read"},
		}, adminToken).Code)
		require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
			Username: "kim", Password: "kimpassword", Role: "operator",
		}, adminToken).Code)

		rec := env.do(http.MethodDelete, "/api/v1/roles/operator", nil, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "ROLE_IN_USE", errorCode(t, rec))

		_, err := storage.GetRoleByName(context.Background(), env.db, "operator")
		assert.NoError(t, err, "거부되었는데 역할이 삭제되었다")
	})
}

// --- AC-10: 인증 비활성 회귀 없음 ---

func TestAC10_NoAuthorizationWhenAuthDisabled(t *testing.T) {
	env := newRBACEnvWithAuth(t, false)

	// 토큰 없이 쓰기 작업을 호출해도 권한 검사로 인한 403 이 발생하지 않는다.
	create := env.do(http.MethodPost, "/api/v1/agents",
		map[string]any{"name": "a", "type": "b"}, "")
	assert.NotEqual(t, http.StatusForbidden, create.Code, create.Body.String())

	del := env.do(http.MethodDelete, "/api/v1/agents/a1", nil, "")
	assert.NotEqual(t, http.StatusForbidden, del.Code, del.Body.String())

	// 관리 API 도 마찬가지로 403 이 되지 않는다.
	users := env.do(http.MethodGet, "/api/v1/users", nil, "")
	assert.Equal(t, http.StatusOK, users.Code, users.Body.String())
}

// --- 엣지 케이스 (acceptance.md) ---

func TestEdgeCases_UserAndRoleValidation(t *testing.T) {
	t.Run("존재하지 않는 역할 부여는 400 이며 사용자 상태 불변", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)
		env.addUser("kim", "kimpassword", rbac.RoleViewer)

		rec := env.do(http.MethodPut, "/api/v1/users/kim",
			dto.UpdateUserRequest{Role: "nonexistent"}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

		row, err := storage.GetUserByUsername(context.Background(), env.db, "kim")
		require.NoError(t, err)
		assert.Equal(t, rbac.RoleViewer, row.Role)
	})

	t.Run("존재하지 않는 역할로 생성은 400 이며 사용자 미생성", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		rec := env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
			Username: "lee", Password: "leepassword", Role: "nonexistent",
		}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

		_, err := storage.GetUserByUsername(context.Background(), env.db, "lee")
		assert.ErrorIs(t, err, storage.ErrUserNotFound)
	})

	t.Run("중복 username 은 409", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		rec := env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
			Username: "admin", Password: "anotherpassword", Role: rbac.RoleViewer,
		}, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "USER_EXISTS", errorCode(t, rec))
	})

	t.Run("8자 미만 비밀번호는 400", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		rec := env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
			Username: "short", Password: "1234567", Role: rbac.RoleViewer,
		}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

		env.addUser("kim", "kimpassword", rbac.RoleViewer)
		resetRec := env.do(http.MethodPut, "/api/v1/users/kim/password",
			dto.ResetPasswordRequest{Password: "1234567"}, adminToken)
		assert.Equal(t, http.StatusBadRequest, resetRec.Code, resetRec.Body.String())
	})

	t.Run("역할 이름 대문자/공백은 400", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		for _, name := range []string{"Operator", "field operator", "op_erator", ""} {
			rec := env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
				Name: name, Permissions: []string{"agent.read"},
			}, adminToken)
			assert.Equal(t, http.StatusBadRequest, rec.Code, "name=%q body=%s", name, rec.Body.String())
		}
	})

	t.Run("역할 이름 변경은 users.role 도 함께 갱신한다", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
			Name: "operator", Permissions: []string{"agent.read"},
		}, adminToken).Code)
		require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
			Username: "kim", Password: "kimpassword", Role: "operator",
		}, adminToken).Code)

		newName := "field-operator"
		rec := env.do(http.MethodPut, "/api/v1/roles/operator",
			dto.UpdateRoleRequest{Name: &newName}, adminToken)
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		row, err := storage.GetUserByUsername(context.Background(), env.db, "kim")
		require.NoError(t, err)
		assert.Equal(t, "field-operator", row.Role, "역할 이름 변경이 users.role 에 반영되지 않았다")
	})

	t.Run("삭제된 역할을 가리키는 토큰은 403 (500 아님)", func(t *testing.T) {
		env := newRBACEnv(t)
		adminToken := env.token("admin", rbac.RoleAdmin)

		require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
			Name: "ghost", Permissions: []string{"agent.read"},
		}, adminToken).Code)

		ghostToken := env.token("ghost-user", "ghost")
		require.Equal(t, http.StatusOK, env.do(http.MethodGet, "/api/v1/agents", nil, ghostToken).Code)

		require.Equal(t, http.StatusNoContent,
			env.do(http.MethodDelete, "/api/v1/roles/ghost", nil, adminToken).Code)

		rec := env.do(http.MethodGet, "/api/v1/agents", nil, ghostToken)
		assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	})

	t.Run("권한 캐시 적재 전 첫 요청도 정상 처리된다", func(t *testing.T) {
		env := newRBACEnv(t)
		// 캐시를 비운 직후의 첫 요청 (cold miss → DB 조회 → 적재).
		env.cache.InvalidateAll()
		rec := env.do(http.MethodGet, "/api/v1/agents", nil, env.token("v1", rbac.RoleViewer))
		assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})
}

// TestNoPasswordHashInAnyUserResponse 는 사용자 관련 어떤 응답에도 비밀번호 해시가
// 포함되지 않음을 검증한다 (spec.md §5 비기능 요구사항).
func TestNoPasswordHashInAnyUserResponse(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
		Username: "kim", Password: "kimpassword", Role: rbac.RoleViewer,
	}, adminToken).Code)

	recs := []*httptest.ResponseRecorder{
		env.do(http.MethodGet, "/api/v1/users", nil, adminToken),
		env.do(http.MethodPut, "/api/v1/users/kim",
			dto.UpdateUserRequest{Role: rbac.RoleEditor}, adminToken),
		env.do(http.MethodPut, "/api/v1/users/kim/password",
			dto.ResetPasswordRequest{Password: "newkimpassword"}, adminToken),
		env.do(http.MethodGet, "/api/v1/auth/me", nil, adminToken),
		env.do(http.MethodPost, "/api/v1/auth/login",
			dto.LoginRequest{Username: "kim", Password: "newkimpassword"}, ""),
	}
	for i, rec := range recs {
		body := rec.Body.String()
		assert.NotContains(t, body, "password_hash", "응답 %d", i)
		assert.NotContains(t, body, "$2a$", "응답 %d 에 bcrypt 해시가 포함되었다", i)
	}
}

// TestPermissionCatalogRequiresAuthOnly 는 GET /permissions 가 role.* 권한 없이도
// 인증만으로 조회 가능함을 검증한다 (spec.md §2.2).
func TestPermissionCatalogRequiresAuthOnly(t *testing.T) {
	env := newRBACEnv(t)

	rec := env.do(http.MethodGet, "/api/v1/permissions", nil, env.token("v1", rbac.RoleViewer))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var catalog dto.PermissionCatalogResponse
	decodeEnvelopeData(t, rec, &catalog)
	assert.ElementsMatch(t, rbac.Permissions(), catalog.Permissions)

	// 토큰이 없으면 인증 단계에서 401 이다.
	assert.Equal(t, http.StatusUnauthorized,
		env.do(http.MethodGet, "/api/v1/permissions", nil, "").Code)
}
