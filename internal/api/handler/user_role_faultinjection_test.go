package handler

import (
	"context"
	"database/sql"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/internal/storage"
)

// @SPEC:SPEC-AUTH-005 (M6) — 부분 저장소 장애 경로 테스트.
//
// 닫힌 DB 는 모든 쿼리를 한꺼번에 실패시키므로 "역할 조회는 되는데 사용자 조회만
// 실패" 같은 분기에 도달할 수 없다. 특정 테이블만 제거해 그런 부분 장애를 만든다.

// faultEnv 는 테이블 단위 장애를 주입할 수 있는 사용자·역할 라우터 환경이다.
type faultEnv struct {
	db     *sql.DB
	router *api.Router
}

func newFaultEnv(t *testing.T) *faultEnv {
	t.Helper()

	ctx := context.Background()
	db, err := storage.OpenSQLiteDB(ctx, filepath.Join(t.TempDir(), "fault.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	hash, err := hashForTest()
	require.NoError(t, err)
	require.NoError(t, storage.InsertUser(ctx, db, "admin", hash, rbac.RoleAdmin, 0, 0))

	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	router := api.NewRouter()
	router.Use(api.Authorization(false, nil, logger)) // 인가 패스스루
	g := router.Group("/api/v1")
	NewUserHandler(db, allowAllPerms{}, logger).RegisterRoutes(g)
	NewRoleHandler(db, allowAllPerms{}, logger).RegisterRoutes(g)

	return &faultEnv{db: db, router: router}
}

func (e *faultEnv) drop(t *testing.T, table string) {
	t.Helper()
	_, err := e.db.ExecContext(context.Background(), "DROP TABLE "+table)
	require.NoError(t, err)
}

// TestUserHandler_UsersTableMissing 는 roles 는 정상이고 users 조회만 실패하는
// 부분 장애에서 500 이 반환되는지 확인한다.
func TestUserHandler_UsersTableMissing(t *testing.T) {
	env := newFaultEnv(t)
	env.drop(t, "users")

	t.Run("생성 시 중복 검사 실패는 500", func(t *testing.T) {
		rec := serveClosed(t, env.router, http.MethodPost, "/api/v1/users",
			dto.CreateUserRequest{Username: "kim", Password: "longenoughpassword", Role: rbac.RoleViewer})
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("역할 변경 시 사용자 조회 실패는 500", func(t *testing.T) {
		rec := serveClosed(t, env.router, http.MethodPut, "/api/v1/users/admin",
			dto.UpdateUserRequest{Role: rbac.RoleViewer})
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("삭제 시 사용자 조회 실패는 500", func(t *testing.T) {
		rec := serveClosed(t, env.router, http.MethodDelete, "/api/v1/users/admin", nil)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("역할 이름 변경 시 users 갱신 실패는 500", func(t *testing.T) {
		// roles 는 살아있으므로 사전 검사는 통과하고, UpdateRoleName 의 users 갱신에서
		// 실패한다 (역할 이름 변경은 users.role 을 동일 트랜잭션에서 갱신한다).
		require.Equal(t, http.StatusCreated, serveClosed(t, env.router, http.MethodPost, "/api/v1/roles",
			dto.CreateRoleRequest{Name: "operator", Permissions: []string{"agent.read"}}).Code)

		newName := "field-operator"
		rec := serveClosed(t, env.router, http.MethodPut, "/api/v1/roles/operator",
			dto.UpdateRoleRequest{Name: &newName})
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("역할 삭제 시 사용 중 검사 실패는 500", func(t *testing.T) {
		rec := serveClosed(t, env.router, http.MethodDelete, "/api/v1/roles/operator", nil)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})
}

// TestUserHandler_InsertFailureReturns500 는 조회는 정상이고 INSERT 만 실패하는
// 상황을 트리거로 만들어 사용자 생성 실패 경로를 검증한다.
func TestUserHandler_InsertFailureReturns500(t *testing.T) {
	env := newFaultEnv(t)

	_, err := env.db.ExecContext(context.Background(), `
		CREATE TRIGGER users_block_insert BEFORE INSERT ON users
		BEGIN SELECT RAISE(ABORT, 'insert blocked'); END;
	`)
	require.NoError(t, err)

	rec := serveClosed(t, env.router, http.MethodPost, "/api/v1/users",
		dto.CreateUserRequest{Username: "kim", Password: "longenoughpassword", Role: rbac.RoleViewer})
	assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())

	// 거부되었으므로 사용자는 생성되지 않는다.
	_, err = storage.GetUserByUsername(context.Background(), env.db, "kim")
	assert.ErrorIs(t, err, storage.ErrUserNotFound)
}

// TestRoleHandler_RolePermissionsTableMissing 는 roles 는 있으나 role_permissions 만
// 없는 부분 장애에서 500 이 반환되는지 확인한다.
func TestRoleHandler_RolePermissionsTableMissing(t *testing.T) {
	env := newFaultEnv(t)
	env.drop(t, "role_permissions")

	t.Run("역할 목록 조회 실패는 500", func(t *testing.T) {
		rec := serveClosed(t, env.router, http.MethodGet, "/api/v1/roles", nil)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("역할 생성 실패는 500", func(t *testing.T) {
		rec := serveClosed(t, env.router, http.MethodPost, "/api/v1/roles",
			dto.CreateRoleRequest{Name: "operator", Permissions: []string{"agent.read"}})
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("권한 수정 실패는 500", func(t *testing.T) {
		perms := []string{"agent.read"}
		rec := serveClosed(t, env.router, http.MethodPut, "/api/v1/roles/viewer",
			dto.UpdateRoleRequest{Permissions: &perms})
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})

	t.Run("역할 삭제 실패는 500", func(t *testing.T) {
		// viewer 는 빌트인이므로 커스텀 역할을 roles 에 직접 넣어 삭제 경로를 탄다.
		_, err := env.db.ExecContext(context.Background(),
			`INSERT INTO roles(name, description, builtin, created_at, updated_at) VALUES ('doomed','',0,0,0)`)
		require.NoError(t, err)

		rec := serveClosed(t, env.router, http.MethodDelete, "/api/v1/roles/doomed", nil)
		assert.Equal(t, http.StatusInternalServerError, rec.Code, rec.Body.String())
	})
}

// TestHandlersDefaultLoggerWhenNil 는 logger 를 주입하지 않아도 기본 로거로
// 동작하는지 확인한다.
func TestHandlersDefaultLoggerWhenNil(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLiteDB(ctx, filepath.Join(t.TempDir(), "nillogger.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	assert.NotNil(t, NewUserHandler(db, nil, nil))
	assert.NotNil(t, NewRoleHandler(db, nil, nil))

	router := api.NewRouter()
	g := router.Group("/api/v1")
	NewRoleHandler(db, nil, nil).RegisterRoutes(g)

	rec := serveClosed(t, router, http.MethodGet, "/api/v1/permissions", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}
