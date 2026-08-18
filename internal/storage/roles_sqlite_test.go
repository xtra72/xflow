// @SPEC:SPEC-AUTH-005 (M1)
// roles_sqlite_test.go — 빌트인 역할 시드 (AC-01) 와 역할 저장소 헬퍼 검증.

package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/rbac"
	_ "modernc.org/sqlite"
)

// setupRolesDB 는 마이그레이션이 적용된 빈 DB 를 연다.
func setupRolesDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "roles-test.db")
	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db, dbPath
}

// TestMigrateRolesSchema_SeedsBuiltinRoles 는 AC-01 전반부를 검증한다:
// roles 에 admin/editor/viewer 가 builtin=1 로 생성되고 권한이 채워지며,
// admin 은 카탈로그 전체 권한을 보유한다.
func TestMigrateRolesSchema_SeedsBuiltinRoles(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	roles, err := ListRoles(ctx, db)
	require.NoError(t, err)
	require.Len(t, roles, 3, "빌트인 역할 3개가 시드되어야 한다")

	// ListRoles 는 이름 알파벳 순서 → admin, editor, viewer
	assert.Equal(t, []string{"admin", "editor", "viewer"},
		[]string{roles[0].Name, roles[1].Name, roles[2].Name})

	for _, r := range roles {
		assert.Truef(t, r.Builtin, "역할 %q 는 builtin=1 이어야 한다", r.Name)
		assert.NotEmptyf(t, r.Permissions, "역할 %q 는 권한을 보유해야 한다", r.Name)
		assert.NotZerof(t, r.CreatedAt, "역할 %q 의 created_at 이 채워져야 한다", r.Name)
		assert.NotZerof(t, r.UpdatedAt, "역할 %q 의 updated_at 이 채워져야 한다", r.Name)
	}

	assert.Equal(t, rbac.Permissions(), roles[0].Permissions,
		"admin 은 카탈로그 전체 권한을 보유해야 한다")

	// 빌트인 정의와 시드 결과 대조
	for _, want := range rbac.BuiltinRoles() {
		got, err := GetRoleByName(ctx, db, want.Name)
		require.NoError(t, err)
		assert.Equalf(t, want.Permissions, got.Permissions, "역할 %q 권한 불일치", want.Name)
		assert.Equal(t, want.Description, got.Description)
	}
}

// TestMigrateRolesSchema_Idempotent 는 AC-01 후반부를 검증한다:
// 마이그레이션 재실행 시 행이 중복 생성되지 않고 오류도 없다.
func TestMigrateRolesSchema_Idempotent(t *testing.T) {
	db, dbPath := setupRolesDB(t)
	ctx := context.Background()

	countRows := func() (int64, int64) {
		var roleCount, permCount int64
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles`).Scan(&roleCount))
		require.NoError(t, db.QueryRowContext(ctx, `SELECT COUNT(*) FROM role_permissions`).Scan(&permCount))
		return roleCount, permCount
	}

	wantRoles, wantPerms := countRows()

	// 동일 핸들에서 직접 재실행
	require.NoError(t, migrateRolesSchema(ctx, db), "마이그레이션 재실행은 오류가 없어야 한다")
	gotRoles, gotPerms := countRows()
	assert.Equal(t, wantRoles, gotRoles, "roles 행이 중복 생성되면 안 된다")
	assert.Equal(t, wantPerms, gotPerms, "role_permissions 행이 중복 생성되면 안 된다")

	// 새 핸들로 재오픈 (서버 재기동 시나리오)
	reopened, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	defer reopened.Close()

	var roleCount, permCount int64
	require.NoError(t, reopened.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles`).Scan(&roleCount))
	require.NoError(t, reopened.QueryRowContext(ctx, `SELECT COUNT(*) FROM role_permissions`).Scan(&permCount))
	assert.Equal(t, wantRoles, roleCount)
	assert.Equal(t, wantPerms, permCount)
}

// TestMigrateRolesSchema_PreservesCustomRoles 는 재기동 시 커스텀 역할이
// 시드에 의해 훼손되지 않음을 검증한다.
func TestMigrateRolesSchema_PreservesCustomRoles(t *testing.T) {
	db, dbPath := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "운영자",
		[]string{"agent.read", "agent.execute", "device.read"}))

	reopened, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	defer reopened.Close()

	got, err := GetRoleByName(ctx, reopened, "operator")
	require.NoError(t, err)
	assert.False(t, got.Builtin)
	assert.Equal(t, []string{"agent.execute", "agent.read", "device.read"}, got.Permissions)
}

// TestInsertRole 는 커스텀 역할 생성 경로를 검증한다 (AC-03).
func TestInsertRole(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "운영자",
		[]string{"agent.read", "agent.execute", "device.read"}))

	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err)
	assert.Equal(t, "operator", got.Name)
	assert.Equal(t, "운영자", got.Description)
	assert.False(t, got.Builtin, "커스텀 역할은 builtin=0")
	assert.Equal(t, []string{"agent.execute", "agent.read", "device.read"}, got.Permissions)
}

// TestInsertRole_RejectsUnknownPermission 는 카탈로그에 없는 권한 키가 거부되고
// 역할이 생성되지 않음을 검증한다 (AC-03 후반부).
func TestInsertRole_RejectsUnknownPermission(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	err := InsertRole(ctx, db, "operator", "", []string{"agent.read", "agent.launch"})
	require.ErrorIs(t, err, ErrInvalidPermission)

	_, err = GetRoleByName(ctx, db, "operator")
	assert.ErrorIs(t, err, ErrRoleNotFound, "거부된 역할은 생성되지 않아야 한다")
}

// TestInsertRole_RejectsInvalidName 는 이름 규칙 위반을 검증한다 (엣지 케이스).
func TestInsertRole_RejectsInvalidName(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	for _, name := range []string{"Operator", "op erator", "", "op_erator"} {
		err := InsertRole(ctx, db, name, "", []string{"agent.read"})
		assert.ErrorIsf(t, err, ErrInvalidRoleName, "이름 %q 는 거부되어야 한다", name)
	}
}

// TestInsertRole_RejectsDuplicate 는 중복 이름 생성이 거부됨을 검증한다.
func TestInsertRole_RejectsDuplicate(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read"}))
	err := InsertRole(ctx, db, "operator", "", []string{"device.read"})
	assert.ErrorIs(t, err, ErrRoleExists)

	// 빌트인 이름과의 충돌도 동일하게 거부된다.
	assert.ErrorIs(t, InsertRole(ctx, db, "viewer", "", []string{"agent.read"}), ErrRoleExists)

	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.read"}, got.Permissions, "기존 역할이 변경되면 안 된다")
}

// TestUpdateRolePermissions 는 권한 집합 전체 교체를 검증한다.
func TestUpdateRolePermissions(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read", "agent.execute"}))
	require.NoError(t, UpdateRolePermissions(ctx, db, "operator", []string{"flow.read", "flow.execute"}))

	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err)
	assert.Equal(t, []string{"flow.execute", "flow.read"}, got.Permissions,
		"기존 권한은 제거되고 새 권한으로 교체되어야 한다")
}

// TestUpdateRolePermissions_Errors 는 미존재 역할과 미정의 권한 처리를 검증한다.
func TestUpdateRolePermissions_Errors(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	assert.ErrorIs(t, UpdateRolePermissions(ctx, db, "ghost", []string{"agent.read"}), ErrRoleNotFound)

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read"}))
	err := UpdateRolePermissions(ctx, db, "operator", []string{"agent.launch"})
	require.ErrorIs(t, err, ErrInvalidPermission)

	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.read"}, got.Permissions, "거부 시 기존 권한이 유지되어야 한다")
}

// TestUpdateRolePermissions_Empty 는 빈 권한 집합으로의 교체를 검증한다.
func TestUpdateRolePermissions_Empty(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read"}))
	require.NoError(t, UpdateRolePermissions(ctx, db, "operator", nil))

	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err)
	assert.Empty(t, got.Permissions)
}

// TestUpdateRoleName_UpdatesUsers 는 역할 이름 변경 시 해당 역할을 쓰던
// 사용자의 users.role 이 함께 갱신됨을 검증한다 (spec.md §4.1, 엣지 케이스).
func TestUpdateRoleName_UpdatesUsers(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read"}))
	require.NoError(t, InsertUser(ctx, db, "kim", "hash-kim", "operator", 0, 0))
	require.NoError(t, InsertUser(ctx, db, "lee", "hash-lee", "viewer", 0, 0))

	require.NoError(t, UpdateRoleName(ctx, db, "operator", "controller"))

	_, err := GetRoleByName(ctx, db, "operator")
	assert.ErrorIs(t, err, ErrRoleNotFound)

	renamed, err := GetRoleByName(ctx, db, "controller")
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.read"}, renamed.Permissions, "권한은 보존되어야 한다")

	kim, err := GetUserByUsername(ctx, db, "kim")
	require.NoError(t, err)
	assert.Equal(t, "controller", kim.Role, "역할을 쓰던 사용자가 갱신되어야 한다")

	lee, err := GetUserByUsername(ctx, db, "lee")
	require.NoError(t, err)
	assert.Equal(t, "viewer", lee.Role, "다른 역할 사용자는 영향받지 않아야 한다")
}

// TestUpdateRoleName_Errors 는 이름 변경 실패 경로를 검증한다.
func TestUpdateRoleName_Errors(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read"}))

	assert.ErrorIs(t, UpdateRoleName(ctx, db, "ghost", "other"), ErrRoleNotFound)
	assert.ErrorIs(t, UpdateRoleName(ctx, db, "operator", "Operator"), ErrInvalidRoleName)
	assert.ErrorIs(t, UpdateRoleName(ctx, db, "operator", "viewer"), ErrRoleExists)

	// 동일 이름은 no-op
	require.NoError(t, UpdateRoleName(ctx, db, "operator", "operator"))
	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.read"}, got.Permissions)
}

// TestDeleteRole 는 역할 삭제 시 권한도 함께 제거됨을 검증한다.
func TestDeleteRole(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read", "device.read"}))
	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err)
	roleID := got.ID

	require.NoError(t, DeleteRole(ctx, db, "operator"))

	_, err = GetRoleByName(ctx, db, "operator")
	assert.ErrorIs(t, err, ErrRoleNotFound)

	var orphans int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM role_permissions WHERE role_id = ?`, roleID).Scan(&orphans))
	assert.Zero(t, orphans, "역할 삭제 시 권한 행도 제거되어야 한다")

	assert.ErrorIs(t, DeleteRole(ctx, db, "operator"), ErrRoleNotFound)
}

// TestListRolePermissions 는 이름 기반 권한 조회를 검증한다 (인가 미들웨어 경로).
func TestListRolePermissions(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	perms, err := ListRolePermissions(ctx, db, "viewer")
	require.NoError(t, err)
	assert.Contains(t, perms, "agent.read")
	assert.NotContains(t, perms, "agent.delete")

	// 삭제된 역할을 가리키는 토큰 처리: 500 이 아니라 식별 가능한 sentinel 이어야 한다.
	_, err = ListRolePermissions(ctx, db, "ghost")
	assert.ErrorIs(t, err, ErrRoleNotFound)
}

// TestRoleStore_ClosedDatabaseReturnsWrappedError 는 DB 장애 시 모든 헬퍼가
// panic 하지 않고 래핑된 에러를 반환하는지 검증한다.
//
// sentinel(ErrRoleNotFound 등) 로 오인되면 상위 계층이 404/409 로 잘못 응답하므로,
// 장애 에러가 sentinel 과 구분되는지도 함께 고정한다.
func TestRoleStore_ClosedDatabaseReturnsWrappedError(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()
	require.NoError(t, db.Close())

	t.Run("ListRoles", func(t *testing.T) {
		_, err := ListRoles(ctx, db)
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRoleNotFound)
	})
	t.Run("GetRoleByName", func(t *testing.T) {
		_, err := GetRoleByName(ctx, db, "admin")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRoleNotFound)
	})
	t.Run("ListRolePermissions", func(t *testing.T) {
		_, err := ListRolePermissions(ctx, db, "admin")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRoleNotFound)
	})
	t.Run("InsertRole", func(t *testing.T) {
		err := InsertRole(ctx, db, "operator", "", []string{"agent.read"})
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRoleExists)
	})
	t.Run("UpdateRolePermissions", func(t *testing.T) {
		err := UpdateRolePermissions(ctx, db, "admin", []string{"agent.read"})
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRoleNotFound)
	})
	t.Run("UpdateRoleName", func(t *testing.T) {
		err := UpdateRoleName(ctx, db, "admin", "root")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRoleNotFound)
	})
	t.Run("DeleteRole", func(t *testing.T) {
		err := DeleteRole(ctx, db, "admin")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrRoleNotFound)
	})
	t.Run("migrateRolesSchema", func(t *testing.T) {
		assert.Error(t, migrateRolesSchema(ctx, db))
	})
	t.Run("migrateUsersRoleConstraint", func(t *testing.T) {
		assert.Error(t, migrateUsersRoleConstraint(ctx, db))
	})
	t.Run("UpdateUserRole", func(t *testing.T) {
		err := UpdateUserRole(ctx, db, "kim", "viewer")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrUserNotFound)
	})
	t.Run("DeleteUser", func(t *testing.T) {
		err := DeleteUser(ctx, db, "kim")
		require.Error(t, err)
		assert.NotErrorIs(t, err, ErrUserNotFound)
	})
	t.Run("CountUsersByRole", func(t *testing.T) {
		_, err := CountUsersByRole(ctx, db, "admin")
		assert.Error(t, err)
	})
}

// TestRoleStore_MissingTablesReturnError 는 role_permissions 테이블이 없는
// 손상된 DB 에서도 헬퍼가 에러를 반환하는지 검증한다 (권한 조회 경로).
func TestRoleStore_MissingTablesReturnError(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `DROP TABLE role_permissions`)
	require.NoError(t, err)

	_, err = ListRoles(ctx, db)
	assert.Error(t, err, "권한 테이블이 없으면 에러여야 한다")

	_, err = GetRoleByName(ctx, db, "admin")
	assert.Error(t, err)

	_, err = ListRolePermissions(ctx, db, "admin")
	assert.Error(t, err)

	assert.Error(t, InsertRole(ctx, db, "operator", "", []string{"agent.read"}))
	assert.Error(t, UpdateRolePermissions(ctx, db, "admin", []string{"agent.read"}))
	assert.Error(t, DeleteRole(ctx, db, "admin"))
}

// TestUpdateRoleName_MissingUsersTable 는 users 갱신 실패 시 역할 이름 변경 전체가
// 롤백되는지 검증한다 (spec.md §4.1 트랜잭션 요구).
func TestUpdateRoleName_MissingUsersTable(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertRole(ctx, db, "operator", "", []string{"agent.read"}))
	_, err := db.ExecContext(ctx, `DROP TABLE users`)
	require.NoError(t, err)

	require.Error(t, UpdateRoleName(ctx, db, "operator", "controller"))

	// 롤백되어 원래 이름이 유지되어야 한다.
	got, err := GetRoleByName(ctx, db, "operator")
	require.NoError(t, err, "users 갱신 실패 시 역할 이름 변경도 롤백되어야 한다")
	assert.Equal(t, "operator", got.Name)

	_, err = GetRoleByName(ctx, db, "controller")
	assert.ErrorIs(t, err, ErrRoleNotFound)
}

// TestListRoles_IgnoresOrphanPermissions 는 존재하지 않는 role_id 를 가리키는
// 권한 행이 있어도 조회가 안전하게 동작함을 검증한다.
func TestListRoles_IgnoresOrphanPermissions(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO role_permissions(role_id, permission) VALUES (9999, 'agent.read')`)
	require.NoError(t, err)

	roles, err := ListRoles(ctx, db)
	require.NoError(t, err)
	assert.Len(t, roles, 3, "고아 권한 행이 역할 목록을 오염시키면 안 된다")
}

// TestListRoles_EmptyTable 는 역할이 하나도 없을 때의 반환을 검증한다.
func TestListRoles_EmptyTable(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `DELETE FROM role_permissions`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `DELETE FROM roles`)
	require.NoError(t, err)

	roles, err := ListRoles(ctx, db)
	require.NoError(t, err)
	assert.Empty(t, roles)
}
