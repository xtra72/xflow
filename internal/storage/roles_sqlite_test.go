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

// @SPEC:SPEC-AUTH-005 (M1) — 빌트인 역할 시드의 자기 복구 경계.
//
// 부팅 시드는 admin 만 코드 정의로 되맞춘다. editor/viewer 의 권한 집합은
// 관리자의 소유물이므로(서버가 수정을 허용한다) 부팅이 덮어쓰지 않는다.
// 이 경계가 무너지면 둘 중 하나가 깨진다 — admin 이 축소된 채 방치되거나,
// 관리자의 editor/viewer 수정이 재시작 때 조용히 사라진다.
func TestSeedBuiltinRoles_ReconcilesAdminOnly(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	// 1) admin 축소 → 부팅 시드가 전체 권한으로 복구한다.
	require.NoError(t, UpdateRolePermissions(ctx, db, "admin", []string{"agent.read"}))
	// 2) viewer 축소 → 부팅 시드가 그대로 둔다(관리자의 의도된 수정으로 본다).
	require.NoError(t, UpdateRolePermissions(ctx, db, "viewer", []string{"agent.read"}))

	require.NoError(t, seedBuiltinRoles(ctx, db))

	adminPerms, err := ListRolePermissions(ctx, db, "admin")
	require.NoError(t, err)
	assert.ElementsMatch(t, rbac.Permissions(), adminPerms,
		"admin 이 코드 정의로 복구되지 않았다")

	viewerPerms, err := ListRolePermissions(ctx, db, "viewer")
	require.NoError(t, err)
	assert.Equal(t, []string{"agent.read"}, viewerPerms,
		"viewer 의 관리자 수정이 부팅 시드에 덮어써졌다")
}

// TestSeedBuiltinRoles_PrunesStaleAdminPermission 는 카탈로그에서 사라진 권한 키가
// admin 에 남아 있으면 제거됨을 확인한다(더하기만 하던 이전 동작의 결함).
func TestSeedBuiltinRoles_PrunesStaleAdminPermission(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	// UpdateRolePermissions 는 카탈로그 검증을 통과시키지 않으므로 직접 주입한다.
	var adminID int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT id FROM roles WHERE name = 'admin'`).Scan(&adminID))
	_, err := db.ExecContext(ctx,
		`INSERT OR IGNORE INTO role_permissions(role_id, permission) VALUES (?, 'ghost.read')`,
		adminID)
	require.NoError(t, err)

	require.NoError(t, seedBuiltinRoles(ctx, db))

	perms, lerr := ListRolePermissions(ctx, db, "admin")
	require.NoError(t, lerr)
	assert.NotContains(t, perms, "ghost.read", "카탈로그에 없는 권한이 남았다")
	assert.ElementsMatch(t, rbac.Permissions(), perms)
}

// TestMigrateRoleNavPermissions_OneTime 는 메뉴 축 이관이 1회성임을 고정한다.
//
// 이관이 매 부팅 반복되면 "관리자가 모든 메뉴를 껐다" 는 상태를 만들 수 없다 —
// 껐다가 재부팅하면 read 기준으로 다시 채워지기 때문이다. nav_migrated 표시로
// 1회만 수행하고, 이후 관리자의 결정을 그대로 둔다.
func TestMigrateRoleNavPermissions_OneTime(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	// 메뉴 축 이전 역할을 재현한다 — read 는 있고 nav 는 없으며 미이관 상태다.
	require.NoError(t, InsertRole(ctx, db, "legacy", "", []string{"agent.read", "flow.read"}))
	_, err := db.ExecContext(ctx, `UPDATE roles SET nav_migrated = 0 WHERE name = 'legacy'`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		DELETE FROM role_permissions
		WHERE role_id = (SELECT id FROM roles WHERE name = 'legacy')
		  AND permission LIKE 'nav.%'`)
	require.NoError(t, err)

	// 1회차 — 보유한 read 에 맞춰 nav 를 채운다(보이는 메뉴가 변하지 않는다).
	require.NoError(t, migrateRoleNavPermissions(ctx, db))
	perms, err := ListRolePermissions(ctx, db, "legacy")
	require.NoError(t, err)
	assert.Contains(t, perms, "nav.agent")
	assert.Contains(t, perms, "nav.flow")
	assert.NotContains(t, perms, "nav.device", "보유하지 않은 read 의 메뉴까지 주면 안 된다")

	// 관리자가 모든 메뉴를 끈다.
	require.NoError(t, UpdateRolePermissions(ctx, db, "legacy",
		[]string{"agent.read", "flow.read"}))

	// 2회차 — 다시 채우지 않는다. 이것이 "대시보드 전용 역할" 이 성립하는 조건이다.
	require.NoError(t, migrateRoleNavPermissions(ctx, db))
	after, err := ListRolePermissions(ctx, db, "legacy")
	require.NoError(t, err)
	assert.NotContains(t, after, "nav.agent", "이관이 반복되어 관리자의 설정이 되돌아갔다")
	assert.ElementsMatch(t, []string{"agent.read", "flow.read"}, after)
}

// TestInsertRole_IsNotNavMigrationTarget 은 새로 만든 역할이 이관 대상이 아님을
// 확인한다. 관리자가 nav 없이 만든 역할에 시스템이 nav 를 주입하면 안 된다.
func TestInsertRole_IsNotNavMigrationTarget(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	require.NoError(t, InsertRole(ctx, db, "kiosk", "", []string{"agent.read", "dashboard.read"}))
	require.NoError(t, migrateRoleNavPermissions(ctx, db))

	perms, err := ListRolePermissions(ctx, db, "kiosk")
	require.NoError(t, err)
	assert.NotContains(t, perms, "nav.agent", "신규 역할에 nav 가 주입되었다")
}

// @SPEC:SPEC-DASHBOARD-004 (M1, spec.md §2.5, acceptance.md AC-09)

// makeLegacyDashboardRole 는 대시보드 엔티티 도입 이전 역할을 재현한다 —
// 지정한 권한만 보유하고 신규 키는 없으며 미이관 상태다.
func makeLegacyDashboardRole(t *testing.T, ctx context.Context, db *sql.DB, name string, perms []string) {
	t.Helper()
	require.NoError(t, InsertRole(ctx, db, name, "", perms))
	_, err := db.ExecContext(ctx,
		`UPDATE roles SET dashboard_migrated = 0 WHERE name = ?`, name)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `
		DELETE FROM role_permissions
		WHERE role_id = (SELECT id FROM roles WHERE name = ?)
		  AND permission IN ('dashboard.create', 'dashboard.delete', 'nav.dashboard')`, name)
	require.NoError(t, err)
}

// dashboardMigratedFlag 는 역할의 이관 표시값을 읽는다.
func dashboardMigratedFlag(t *testing.T, ctx context.Context, db *sql.DB, name string) int {
	t.Helper()
	var flag int
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT dashboard_migrated FROM roles WHERE name = ?`, name).Scan(&flag))
	return flag
}

// TestMigrateRoleDashboardPermissions_GrantsToEditableRole 는 read + update 를
// 모두 보유한 역할이 create 와 nav.dashboard 를 받고 delete 는 받지 않음을 고정한다.
//
// delete 를 함께 주면 이관이 사용자 몰래 삭제 권한을 늘린다 — spec.md §2.5 가
// 명시적으로 금지하는 동작이다.
func TestMigrateRoleDashboardPermissions_GrantsToEditableRole(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	makeLegacyDashboardRole(t, ctx, db, "legacy-editor",
		[]string{"dashboard.read", "dashboard.update", "agent.read"})

	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))

	perms, err := ListRolePermissions(ctx, db, "legacy-editor")
	require.NoError(t, err)
	assert.Contains(t, perms, "dashboard.create", "편집 가능 역할이 create 를 받지 못했다")
	assert.Contains(t, perms, "nav.dashboard", "편집 가능 역할이 관리 메뉴를 받지 못했다")
	assert.NotContains(t, perms, "dashboard.delete", "이관이 삭제 권한을 조용히 늘렸다")
	assert.Equal(t, 1, dashboardMigratedFlag(t, ctx, db, "legacy-editor"))
}

// TestMigrateRoleDashboardPermissions_SkipsReadOnlyRole 는 read 만 보유한 역할이
// 아무것도 받지 않음을 고정한다. viewer 계열 역할이 여기에 해당한다.
func TestMigrateRoleDashboardPermissions_SkipsReadOnlyRole(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	makeLegacyDashboardRole(t, ctx, db, "legacy-viewer",
		[]string{"dashboard.read", "agent.read"})
	// update 만 보유한 경계 케이스도 함께 확인한다 (둘 다 있어야 한다는 AND 조건).
	makeLegacyDashboardRole(t, ctx, db, "legacy-odd",
		[]string{"dashboard.update", "agent.read"})

	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))

	for _, name := range []string{"legacy-viewer", "legacy-odd"} {
		perms, err := ListRolePermissions(ctx, db, name)
		require.NoError(t, err)
		assert.NotContainsf(t, perms, "dashboard.create", "%s 가 create 를 받았다", name)
		assert.NotContainsf(t, perms, "dashboard.delete", "%s 가 delete 를 받았다", name)
		assert.NotContainsf(t, perms, "nav.dashboard", "%s 가 관리 메뉴를 받았다", name)
		assert.Equalf(t, 1, dashboardMigratedFlag(t, ctx, db, name),
			"%s 가 이관 완료로 표시되지 않아 다음 부팅에 재시도된다", name)
	}
}

// TestMigrateRoleDashboardPermissions_Idempotent 는 3회 연속 실행이 행을 늘리지
// 않고, 이관 표시가 한 번만 세워지며, 이후 관리자의 제거가 유지됨을 고정한다.
//
// 이관이 반복되면 "대시보드 관리 메뉴를 끈 역할" 을 만들 수 없다 — 껐다가
// 재부팅하면 다시 채워지기 때문이다 (migrateRoleNavPermissions 와 같은 이유).
func TestMigrateRoleDashboardPermissions_Idempotent(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	makeLegacyDashboardRole(t, ctx, db, "legacy-editor",
		[]string{"dashboard.read", "dashboard.update"})

	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))
	first, err := ListRolePermissions(ctx, db, "legacy-editor")
	require.NoError(t, err)

	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))
	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))

	after, err := ListRolePermissions(ctx, db, "legacy-editor")
	require.NoError(t, err)
	assert.Equal(t, first, after, "재실행이 권한 집합을 바꿨다")

	// 중복 행이 생기지 않았는지 직접 확인한다 (복합 PK 가 있어도 계약으로 고정).
	var rowCount int
	require.NoError(t, db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM role_permissions
		WHERE role_id = (SELECT id FROM roles WHERE name = 'legacy-editor')
	`).Scan(&rowCount))
	assert.Equal(t, len(after), rowCount, "권한 행이 중복 삽입되었다")
	assert.Equal(t, 1, dashboardMigratedFlag(t, ctx, db, "legacy-editor"))

	// 관리자가 관리 메뉴를 끈다 → 재실행이 되돌리지 않아야 한다.
	require.NoError(t, UpdateRolePermissions(ctx, db, "legacy-editor",
		[]string{"dashboard.read", "dashboard.update", "dashboard.create"}))
	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))

	final, err := ListRolePermissions(ctx, db, "legacy-editor")
	require.NoError(t, err)
	assert.NotContains(t, final, "nav.dashboard", "이관이 반복되어 관리자의 설정이 되돌아갔다")
}

// TestInsertRole_IsNotDashboardMigrationTarget 은 새로 만든 역할이 이관 대상이
// 아님을 확인한다. 관리자가 create 없이 만든 역할에 시스템이 create 를 주입하면
// 안 된다 (TestInsertRole_IsNotNavMigrationTarget 와 같은 경계).
func TestInsertRole_IsNotDashboardMigrationTarget(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	require.NoError(t, InsertRole(ctx, db, "kiosk", "",
		[]string{"dashboard.read", "dashboard.update"}))
	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))

	perms, err := ListRolePermissions(ctx, db, "kiosk")
	require.NoError(t, err)
	assert.NotContains(t, perms, "dashboard.create", "신규 역할에 create 가 주입되었다")
	assert.NotContains(t, perms, "nav.dashboard", "신규 역할에 관리 메뉴가 주입되었다")
}

// TestMigrateRoleDashboardPermissions_BuiltinRolesMatchSeed 는 이관이 빌트인 시드와
// 충돌하지 않음을 고정한다 (spec.md §2.5 표, acceptance.md AC-09).
//
// 신규 설치는 시드가 곧바로 코드 정의를 채우고, 업그레이드는 시드(admin 되맞춤) 후
// 이관이 editor 만 끌어올린다. 두 경로의 결과가 같아야 한다.
func TestMigrateRoleDashboardPermissions_BuiltinRolesMatchSeed(t *testing.T) {
	ctx := context.Background()
	db, _ := setupRolesDB(t)

	// 신규 설치 경로 — OpenSQLiteDB 가 이미 시드 + 이관을 마쳤다.
	assertBuiltinDashboardMatrix(t, ctx, db)

	// 업그레이드 경로 재현 — 신규 키를 걷어내고 미이관 상태로 되돌린다.
	_, err := db.ExecContext(ctx, `
		DELETE FROM role_permissions
		WHERE permission IN ('dashboard.create', 'dashboard.delete', 'nav.dashboard')`)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `UPDATE roles SET dashboard_migrated = 0`)
	require.NoError(t, err)

	require.NoError(t, seedBuiltinRoles(ctx, db))
	require.NoError(t, migrateRoleDashboardPermissions(ctx, db))

	assertBuiltinDashboardMatrix(t, ctx, db)
}

// assertBuiltinDashboardMatrix 는 빌트인 역할 3종의 신규 키 보유를 spec.md §2.5
// 표와 대조한다.
func assertBuiltinDashboardMatrix(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	tests := []struct {
		role       string
		wantHave   []string
		wantAbsent []string
	}{
		{role: "admin", wantHave: []string{"dashboard.create", "dashboard.delete", "nav.dashboard"}},
		{role: "editor", wantHave: []string{"dashboard.create", "nav.dashboard"}, wantAbsent: []string{"dashboard.delete"}},
		{role: "viewer", wantAbsent: []string{"dashboard.create", "dashboard.delete", "nav.dashboard"}},
	}
	for _, tc := range tests {
		perms, err := ListRolePermissions(ctx, db, tc.role)
		require.NoError(t, err)
		for _, p := range tc.wantHave {
			assert.Containsf(t, perms, p, "%s 는 %q 를 보유해야 한다", tc.role, p)
		}
		for _, p := range tc.wantAbsent {
			assert.NotContainsf(t, perms, p, "%s 는 %q 를 보유하면 안 된다", tc.role, p)
		}
	}
}
