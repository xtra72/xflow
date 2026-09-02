package handler

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/internal/storage"
)

// @SPEC:SPEC-AUTH-005 (M6) — 사용자·역할 API 의 오류 경로 테스트.
//
// 성공 경로와 인수 조건은 rbac_e2e_test.go 가 담당한다. 본 파일은 404/400/409 분기와
// 정상 삭제 경로를 보강한다.

// rawRequest 는 임의의(잘못된) 본문을 그대로 보낸다.
func (e *rbacEnv) rawRequest(method, path, body, token string) *httptest.ResponseRecorder {
	e.t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	e.router.Handler().ServeHTTP(rec, req)
	return rec
}

func TestUserHandler_NotFoundPaths(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	t.Run("없는 사용자 역할 변경은 404", func(t *testing.T) {
		rec := env.do(http.MethodPut, "/api/v1/users/nobody",
			dto.UpdateUserRequest{Role: rbac.RoleViewer}, adminToken)
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})

	t.Run("없는 사용자 비밀번호 재설정은 404", func(t *testing.T) {
		rec := env.do(http.MethodPut, "/api/v1/users/nobody/password",
			dto.ResetPasswordRequest{Password: "longenoughpassword"}, adminToken)
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})

	t.Run("없는 사용자 삭제는 404", func(t *testing.T) {
		rec := env.do(http.MethodDelete, "/api/v1/users/nobody", nil, adminToken)
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})
}

func TestUserHandler_BadRequestPaths(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)
	env.addUser("kim", "kimpassword", rbac.RoleViewer)

	t.Run("username 누락은 400", func(t *testing.T) {
		rec := env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
			Password: "longenoughpassword", Role: rbac.RoleViewer,
		}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	})

	t.Run("role 누락은 400", func(t *testing.T) {
		rec := env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
			Username: "lee", Password: "longenoughpassword",
		}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	})

	t.Run("역할 변경 시 role 누락은 400", func(t *testing.T) {
		rec := env.do(http.MethodPut, "/api/v1/users/kim", dto.UpdateUserRequest{}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	})

	t.Run("잘못된 JSON 본문은 400", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest,
			env.rawRequest(http.MethodPost, "/api/v1/users", `{"username":`, adminToken).Code)
		assert.Equal(t, http.StatusBadRequest,
			env.rawRequest(http.MethodPut, "/api/v1/users/kim", `{"unknown":1}`, adminToken).Code)
		assert.Equal(t, http.StatusBadRequest,
			env.rawRequest(http.MethodPut, "/api/v1/users/kim/password", `nope`, adminToken).Code)
	})
}

func TestUserHandler_DeleteAndResetSucceed(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)
	env.addUser("kim", "kimpassword", rbac.RoleViewer)

	// 비밀번호 재설정 후 새 비밀번호로 로그인된다.
	rec := env.do(http.MethodPut, "/api/v1/users/kim/password",
		dto.ResetPasswordRequest{Password: "brandnewpassword"}, adminToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	loginRec := env.do(http.MethodPost, "/api/v1/auth/login",
		dto.LoginRequest{Username: "kim", Password: "brandnewpassword"}, "")
	assert.Equal(t, http.StatusOK, loginRec.Code, loginRec.Body.String())

	// 관리 권한 보유자가 아니므로 삭제된다.
	delRec := env.do(http.MethodDelete, "/api/v1/users/kim", nil, adminToken)
	require.Equal(t, http.StatusNoContent, delRec.Code, delRec.Body.String())

	_, err := storage.GetUserByUsername(context.Background(), env.db, "kim")
	assert.ErrorIs(t, err, storage.ErrUserNotFound)

	// 목록에도 더 이상 나타나지 않는다.
	listRec := env.do(http.MethodGet, "/api/v1/users", nil, adminToken)
	require.Equal(t, http.StatusOK, listRec.Code)
	var users []dto.UserResponse
	decodeEnvelopeData(t, listRec, &users)
	for _, u := range users {
		assert.NotEqual(t, "kim", u.Username)
		assert.NotEmpty(t, u.CreatedAt)
	}
}

func TestRoleHandler_NotFoundAndBadRequestPaths(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name: "operator", Permissions: []string{"agent.read"},
	}, adminToken).Code)

	t.Run("없는 역할 수정은 404", func(t *testing.T) {
		perms := []string{"agent.read"}
		rec := env.do(http.MethodPut, "/api/v1/roles/ghost",
			dto.UpdateRoleRequest{Permissions: &perms}, adminToken)
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})

	t.Run("없는 역할 삭제는 404", func(t *testing.T) {
		rec := env.do(http.MethodDelete, "/api/v1/roles/ghost", nil, adminToken)
		assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	})

	t.Run("name 과 permissions 모두 없으면 400", func(t *testing.T) {
		rec := env.do(http.MethodPut, "/api/v1/roles/operator", dto.UpdateRoleRequest{}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	})

	t.Run("수정 시 카탈로그 밖 권한은 400", func(t *testing.T) {
		perms := []string{"agent.read", "agent.teleport"}
		rec := env.do(http.MethodPut, "/api/v1/roles/operator",
			dto.UpdateRoleRequest{Permissions: &perms}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

		row, err := storage.GetRoleByName(context.Background(), env.db, "operator")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"agent.read"}, row.Permissions, "거부되었는데 권한이 변경되었다")
	})

	t.Run("이름 변경 시 규칙 위반은 400", func(t *testing.T) {
		bad := "Operator X"
		rec := env.do(http.MethodPut, "/api/v1/roles/operator",
			dto.UpdateRoleRequest{Name: &bad}, adminToken)
		assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	})

	t.Run("중복 이름으로 변경은 409", func(t *testing.T) {
		require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
			Name: "auditor", Permissions: []string{"agent.read"},
		}, adminToken).Code)

		existing := "auditor"
		rec := env.do(http.MethodPut, "/api/v1/roles/operator",
			dto.UpdateRoleRequest{Name: &existing}, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "ROLE_EXISTS", errorCode(t, rec))
	})

	t.Run("빌트인 역할 이름 변경은 409", func(t *testing.T) {
		newName := "watcher"
		rec := env.do(http.MethodPut, "/api/v1/roles/viewer",
			dto.UpdateRoleRequest{Name: &newName}, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "BUILTIN_ROLE_IMMUTABLE", errorCode(t, rec))
	})

	t.Run("중복 역할 생성은 409", func(t *testing.T) {
		rec := env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
			Name: "operator", Permissions: []string{"agent.read"},
		}, adminToken)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Equal(t, "ROLE_EXISTS", errorCode(t, rec))
	})

	t.Run("잘못된 JSON 본문은 400", func(t *testing.T) {
		assert.Equal(t, http.StatusBadRequest,
			env.rawRequest(http.MethodPost, "/api/v1/roles", `{"name":`, adminToken).Code)
		assert.Equal(t, http.StatusBadRequest,
			env.rawRequest(http.MethodPut, "/api/v1/roles/operator", `{"nope":1}`, adminToken).Code)
	})
}

func TestRoleHandler_EmptyPermissionRoleSerializesAsArray(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	rec := env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name: "empty-role", Permissions: []string{},
	}, adminToken)
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	// 권한이 없는 역할도 null 이 아닌 빈 배열로 직렬화된다.
	assert.Contains(t, rec.Body.String(), `"permissions":[]`)

	// 삭제도 정상 동작한다 (사용자가 없으므로 UB1-5 에 걸리지 않는다).
	delRec := env.do(http.MethodDelete, "/api/v1/roles/empty-role", nil, adminToken)
	assert.Equal(t, http.StatusNoContent, delRec.Code, delRec.Body.String())
}

// TestRoleUpdateCombinesPermissionsAndRename 은 권한 교체와 이름 변경을 한 요청에서
// 함께 수행하는 경로를 검증한다.
func TestRoleUpdateCombinesPermissionsAndRename(t *testing.T) {
	env := newRBACEnv(t)
	adminToken := env.token("admin", rbac.RoleAdmin)

	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/roles", dto.CreateRoleRequest{
		Name: "operator", Permissions: []string{"agent.read"},
	}, adminToken).Code)
	require.Equal(t, http.StatusCreated, env.do(http.MethodPost, "/api/v1/users", dto.CreateUserRequest{
		Username: "kim", Password: "kimpassword", Role: "operator",
	}, adminToken).Code)

	newName := "field-operator"
	perms := []string{"agent.read", "agent.execute"}
	rec := env.do(http.MethodPut, "/api/v1/roles/operator",
		dto.UpdateRoleRequest{Name: &newName, Permissions: &perms}, adminToken)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var role dto.RoleResponse
	decodeEnvelopeData(t, rec, &role)
	assert.Equal(t, "field-operator", role.Name)
	assert.ElementsMatch(t, perms, role.Permissions)

	// users.role 이 함께 갱신되고, kim 의 기존 토큰으로 새 권한이 적용된다.
	row, err := storage.GetUserByUsername(context.Background(), env.db, "kim")
	require.NoError(t, err)
	assert.Equal(t, "field-operator", row.Role)

	kimToken := env.token("kim", "operator")
	assert.Equal(t, http.StatusOK,
		env.do(http.MethodPost, "/api/v1/agents/a1/start", map[string]any{}, kimToken).Code)
}
