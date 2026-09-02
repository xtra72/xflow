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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/internal/storage"
)

// @SPEC:SPEC-AUTH-005 (M6) — 저장소 장애 시 500 경로 테스트.
//
// 닫힌 DB 핸들로 핸들러를 구동하여 모든 저장소 조회가 실패하게 만든다. 인가는
// 통과시켜(패스스루) 미들웨어가 아니라 핸들러의 오류 처리가 관측되게 한다.

// allowAllPerms 는 항상 권한을 허용하고 사용자 목록 조회만 DB 에 의존하게 하는
// RolePermissionService 스텁이다.
type allowAllPerms struct{}

func (allowAllPerms) Permissions(_ context.Context, _ string) ([]string, error) {
	return []string{"user.delete", "role.update"}, nil
}
func (allowAllPerms) Invalidate(string) {}
func (allowAllPerms) InvalidateAll()    {}

// newClosedDBRouter 는 닫힌 DB 를 물린 사용자·역할 핸들러 라우터를 만든다.
func newClosedDBRouter(t *testing.T) *api.Router {
	t.Helper()

	ctx := context.Background()
	db, err := storage.OpenSQLiteDB(ctx, filepath.Join(t.TempDir(), "closed.db"))
	require.NoError(t, err)

	hash, err := hashForTest()
	require.NoError(t, err)
	require.NoError(t, storage.InsertUser(ctx, db, "admin", hash, rbac.RoleAdmin, 0, 0))
	require.NoError(t, db.Close()) // 이후 모든 쿼리는 sql.ErrConnDone 으로 실패한다

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := api.NewRouter()
	// 인가는 비활성(패스스루) — 핸들러 자체의 오류 처리를 관측한다.
	router.Use(api.Authorization(false, nil, logger))

	g := router.Group("/api/v1")
	NewUserHandler(db, allowAllPerms{}, logger).RegisterRoutes(g)
	NewRoleHandler(db, allowAllPerms{}, logger).RegisterRoutes(g)
	return router
}

// hashForTest 는 짧은 고정 해시를 만든다 (bcrypt 비용 절감을 위해 재사용).
func hashForTest() (string, error) {
	return "$2a$10$abcdefghijklmnopqrstuvwxyz0123456789ABCDEFGHIJKLMNOPQR", nil
}

func serveClosed(t *testing.T, r *api.Router, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.Handler().ServeHTTP(rec, req)
	return rec
}

func TestUserRoleHandlers_StorageFailuresReturn500(t *testing.T) {
	r := newClosedDBRouter(t)

	cases := []struct {
		name   string
		method string
		path   string
		body   any
	}{
		{"사용자 목록", http.MethodGet, "/api/v1/users", nil},
		{"사용자 생성", http.MethodPost, "/api/v1/users",
			dto.CreateUserRequest{Username: "kim", Password: "longenoughpassword", Role: rbac.RoleViewer}},
		{"역할 변경", http.MethodPut, "/api/v1/users/admin",
			dto.UpdateUserRequest{Role: rbac.RoleViewer}},
		{"비밀번호 재설정", http.MethodPut, "/api/v1/users/admin/password",
			dto.ResetPasswordRequest{Password: "longenoughpassword"}},
		{"사용자 삭제", http.MethodDelete, "/api/v1/users/kim", nil},
		{"역할 목록", http.MethodGet, "/api/v1/roles", nil},
		{"역할 생성", http.MethodPost, "/api/v1/roles",
			dto.CreateRoleRequest{Name: "operator", Permissions: []string{"agent.read"}}},
		{"역할 삭제", http.MethodDelete, "/api/v1/roles/operator", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := serveClosed(t, r, tc.method, tc.path, tc.body)
			assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
		})
	}

	t.Run("역할 권한 수정", func(t *testing.T) {
		perms := []string{"agent.read"}
		rec := serveClosed(t, r, http.MethodPut, "/api/v1/roles/operator",
			dto.UpdateRoleRequest{Permissions: &perms})
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("역할 이름 변경", func(t *testing.T) {
		newName := "field-operator"
		rec := serveClosed(t, r, http.MethodPut, "/api/v1/roles/operator",
			dto.UpdateRoleRequest{Name: &newName})
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	// 권한 카탈로그는 DB 를 쓰지 않으므로 장애와 무관하게 200 이다.
	t.Run("권한 카탈로그는 DB 무관", func(t *testing.T) {
		rec := serveClosed(t, r, http.MethodGet, "/api/v1/permissions", nil)
		assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	})
}

// TestGuardAdminCapacity_PermissionLookupFailure 는 역할 권한 조회 실패가 500 으로
// 전파되는지 확인한다.
func TestGuardAdminCapacity_PermissionLookupFailure(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "guard.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	hash, err := hashForTest()
	require.NoError(t, err)
	require.NoError(t, storage.InsertUser(ctx, db, "admin", hash, rbac.RoleAdmin, 0, 0))
	require.NoError(t, storage.InsertUser(ctx, db, "kim", hash, rbac.RoleViewer, 0, 0))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := api.NewRouter()
	router.Use(api.Authorization(false, nil, logger))
	g := router.Group("/api/v1")
	NewUserHandler(db, failingPerms{}, logger).RegisterRoutes(g)

	rec := serveClosed(t, router, http.MethodDelete, "/api/v1/users/kim", nil)
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
}

// failingPerms 는 권한 조회가 항상 실패하는 스텁이다.
type failingPerms struct{}

func (failingPerms) Permissions(_ context.Context, _ string) ([]string, error) {
	return nil, sql.ErrConnDone
}
func (failingPerms) Invalidate(string) {}
func (failingPerms) InvalidateAll()    {}

// TestUserHandler_NilPermsSkipsAdminGuard 는 인가가 주입되지 않은 배포에서
// 잠금 방지 검사가 건너뛰어지는지 확인한다 (인증 비활성 배포 호환).
func TestUserHandler_NilPermsSkipsAdminGuard(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLiteDB(ctx, filepath.Join(t.TempDir(), "nilperms.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	hash, err := hashForTest()
	require.NoError(t, err)
	require.NoError(t, storage.InsertUser(ctx, db, "solo", hash, rbac.RoleAdmin, 0, 0))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := api.NewRouter()
	router.Use(api.Authorization(false, nil, logger))
	g := router.Group("/api/v1")
	NewUserHandler(db, nil, logger).RegisterRoutes(g)

	// 유일한 admin 이지만 perms 가 없으므로 잠금 방지 검사가 수행되지 않는다.
	rec := serveClosed(t, router, http.MethodDelete, "/api/v1/users/solo", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}
