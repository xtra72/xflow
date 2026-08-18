// @SPEC:SPEC-AUTH-005 (M2, acceptance.md AC-02)
// users_role_constraint_test.go — users.role CHECK 제약 제거 마이그레이션 검증.
//
// 위험 구간이므로 다음을 전수 검증한다:
//   - 제약 있는 기존 DB → 제약 제거 + 사용자 3명 전 컬럼 보존
//   - 마이그레이션 후 커스텀 역할('operator') 삽입 성공
//   - 재실행 시 no-op (데이터 불변)
//   - 신규 DB 는 처음부터 제약 없음
//   - 명시적 인덱스 보존

package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// legacyUser 는 마이그레이션 전후 대조용 사용자 픽스처이다.
type legacyUser struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    int64
	UpdatedAt    int64
}

var legacyUsers = []legacyUser{
	{1, "admin", "$2a$10$adminhashadminhashadminha", "admin", 1700000000001, 1700000000002},
	{2, "editor-kim", "$2a$10$editorhasheditorhasheditor", "editor", 1700000000003, 1700000000004},
	{3, "viewer-lee", "$2a$10$viewerhashviewerhashviewer", "viewer", 1700000000005, 1700000000006},
}

// setupLegacyUsersDB 는 CHECK 제약이 있는 구 버전 users 테이블과 사용자 3명을
// 가진 DB 를 만든다 (마이그레이션 이전 상태 재현).
func setupLegacyUsersDB(t *testing.T) (*sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "legacy-users.db")

	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// SPEC-DASHBOARD-001 v0.2.0 시점의 원본 DDL (CHECK 제약 포함).
	_, err = db.ExecContext(ctx, `CREATE TABLE users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT    NOT NULL UNIQUE,
		password_hash TEXT    NOT NULL,
		role          TEXT    NOT NULL DEFAULT 'viewer'
		              CHECK (role IN ('admin', 'editor', 'viewer')),
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	)`)
	require.NoError(t, err)

	for _, u := range legacyUsers {
		_, err := db.ExecContext(ctx, `
			INSERT INTO users(id, username, password_hash, role, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, u.ID, u.Username, u.PasswordHash, u.Role, u.CreatedAt, u.UpdatedAt)
		require.NoError(t, err)
	}
	return db, dbPath
}

// usersDDL 은 현재 users 테이블의 DDL 문자열을 반환한다.
func usersDDL(t *testing.T, db *sql.DB) string {
	t.Helper()
	var ddl string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&ddl))
	return ddl
}

// snapshotUsers 는 users 테이블 전체를 id 순서로 읽는다.
func snapshotUsers(t *testing.T, db *sql.DB) []legacyUser {
	t.Helper()
	rows, err := db.QueryContext(context.Background(), `
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users ORDER BY id ASC
	`)
	require.NoError(t, err)
	defer rows.Close()

	var out []legacyUser
	for rows.Next() {
		var u legacyUser
		require.NoError(t, rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt))
		out = append(out, u)
	}
	require.NoError(t, rows.Err())
	return out
}

// TestUsersRoleConstraint_LegacyFixtureHasConstraint 는 픽스처가 실제로 제약을
// 가지고 있음을 먼저 확인한다 (테스트가 무의미해지는 것을 방지).
func TestUsersRoleConstraint_LegacyFixtureHasConstraint(t *testing.T) {
	db, _ := setupLegacyUsersDB(t)
	ctx := context.Background()

	assert.Contains(t, strings.ToUpper(usersDDL(t, db)), "CHECK")

	_, err := db.ExecContext(ctx, `
		INSERT INTO users(username, password_hash, role, created_at, updated_at)
		VALUES ('pre', 'h', 'operator', 1, 1)
	`)
	require.Error(t, err, "마이그레이션 전에는 커스텀 역할 삽입이 CHECK 위반이어야 한다")
}

// TestUsersRoleConstraint_RemovesConstraintAndPreservesData 는 AC-02 를 검증한다.
func TestUsersRoleConstraint_RemovesConstraintAndPreservesData(t *testing.T) {
	db, _ := setupLegacyUsersDB(t)
	ctx := context.Background()

	before := snapshotUsers(t, db)
	require.Len(t, before, 3)

	require.NoError(t, migrateUsersRoleConstraint(ctx, db))

	// 1. 제약 제거
	assert.NotContains(t, strings.ToUpper(usersDDL(t, db)), "CHECK",
		"role CHECK 제약이 제거되어야 한다")

	// 2. 사용자 3명 전 컬럼 보존
	after := snapshotUsers(t, db)
	assert.Equal(t, before, after,
		"username·password_hash·role·created_at·updated_at 이 모두 보존되어야 한다")

	// 3. 커스텀 역할 삽입 성공
	require.NoError(t, InsertUser(ctx, db, "operator-park", "$2a$10$operatorhash", "operator", 0, 0))
	got, err := GetUserByUsername(ctx, db, "operator-park")
	require.NoError(t, err)
	assert.Equal(t, "operator", got.Role)

	// 4. UNIQUE(username) 제약이 재생성 후에도 유효
	err = InsertUser(ctx, db, "admin", "dup", "viewer", 0, 0)
	assert.Error(t, err, "username UNIQUE 제약이 유지되어야 한다")
}

// TestUsersRoleConstraint_RerunIsNoop 는 재실행 멱등성을 검증한다 (AC-02).
func TestUsersRoleConstraint_RerunIsNoop(t *testing.T) {
	db, _ := setupLegacyUsersDB(t)
	ctx := context.Background()

	require.NoError(t, migrateUsersRoleConstraint(ctx, db))
	firstDDL := usersDDL(t, db)
	firstSnapshot := snapshotUsers(t, db)

	for i := 0; i < 3; i++ {
		require.NoErrorf(t, migrateUsersRoleConstraint(ctx, db), "%d 회차 재실행", i+2)
	}

	assert.Equal(t, firstDDL, usersDDL(t, db), "재실행이 테이블을 다시 만들면 안 된다")
	assert.Equal(t, firstSnapshot, snapshotUsers(t, db), "재실행 후 데이터가 변하면 안 된다")

	// users_new 잔여물이 남지 않아야 한다.
	var leftover int64
	require.NoError(t, db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users_new'`).Scan(&leftover))
	assert.Zero(t, leftover, "임시 테이블 users_new 가 남으면 안 된다")
}

// TestUsersRoleConstraint_PreservesExplicitIndex 는 명시적으로 생성된 인덱스가
// 테이블 재생성 후에도 복원됨을 검증한다 (plan.md §M2 5단계).
func TestUsersRoleConstraint_PreservesExplicitIndex(t *testing.T) {
	db, _ := setupLegacyUsersDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx, `CREATE INDEX users_role_idx ON users(role)`)
	require.NoError(t, err)

	require.NoError(t, migrateUsersRoleConstraint(ctx, db))

	var idxName string
	err = db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='index' AND name='users_role_idx'`).Scan(&idxName)
	require.NoError(t, err, "명시적 인덱스가 복원되어야 한다")
	assert.Equal(t, "users_role_idx", idxName)
}

// TestUsersRoleConstraint_RollsBackOnFailure 는 plan.md §M2 롤백 계획을 검증한다:
// 재생성 도중 실패하면 원본 users 테이블과 데이터가 손상되지 않아야 한다.
//
// 실패를 강제하기 위해 users_new 이름을 미리 점유한다 (CREATE TABLE 충돌).
func TestUsersRoleConstraint_RollsBackOnFailure(t *testing.T) {
	db, _ := setupLegacyUsersDB(t)
	ctx := context.Background()

	before := snapshotUsers(t, db)
	beforeDDL := usersDDL(t, db)

	_, err := db.ExecContext(ctx, `CREATE TABLE users_new (blocker INTEGER)`)
	require.NoError(t, err)

	require.Error(t, migrateUsersRoleConstraint(ctx, db), "테이블 생성 충돌 시 실패해야 한다")

	// 원본 보존 — 로그인 불가 상태가 되면 안 된다.
	assert.Equal(t, beforeDDL, usersDDL(t, db), "실패 시 원본 users DDL 이 유지되어야 한다")
	assert.Equal(t, before, snapshotUsers(t, db), "실패 시 사용자 데이터가 보존되어야 한다")

	// 충돌 요인을 제거하면 정상적으로 마이그레이션된다 (재시도 가능).
	_, err = db.ExecContext(ctx, `DROP TABLE users_new`)
	require.NoError(t, err)
	require.NoError(t, migrateUsersRoleConstraint(ctx, db))
	assert.NotContains(t, strings.ToUpper(usersDDL(t, db)), "CHECK")
	assert.Equal(t, before, snapshotUsers(t, db))
}

// TestUsersRoleConstraint_ClosedDatabase 는 DB 장애 시 panic 없이 에러를 반환하는지
// 검증한다 (기동 실패로 이어져야 하며, 손상된 스키마로 진행되면 안 된다).
func TestUsersRoleConstraint_ClosedDatabase(t *testing.T) {
	db, _ := setupLegacyUsersDB(t)
	ctx := context.Background()
	require.NoError(t, db.Close())

	assert.Error(t, migrateUsersRoleConstraint(ctx, db))
	assert.Error(t, migrateUsersSchema(ctx, db))
}

// TestMigrateRolesSchema_ErrorPaths 는 역할 스키마 생성·시드 실패 경로를 검증한다.
func TestMigrateRolesSchema_ErrorPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("role_permissions 이름 충돌", func(t *testing.T) {
		db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "conflict.db"))
		require.NoError(t, err)
		defer db.Close()

		// 동일 이름의 뷰가 있으면 CREATE TABLE IF NOT EXISTS 가 실패한다.
		_, err = db.ExecContext(ctx, `CREATE VIEW role_permissions AS SELECT 1 AS x`)
		require.NoError(t, err)

		assert.Error(t, migrateRolesSchema(ctx, db))
	})

	t.Run("roles 테이블 없음", func(t *testing.T) {
		db, _ := setupRolesDB(t)
		_, err := db.ExecContext(ctx, `DROP TABLE roles`)
		require.NoError(t, err)

		assert.Error(t, seedBuiltinRoles(ctx, db), "roles 테이블이 없으면 시드가 실패해야 한다")
	})

	t.Run("role_permissions 테이블 없음", func(t *testing.T) {
		db, _ := setupRolesDB(t)
		_, err := db.ExecContext(ctx, `DROP TABLE role_permissions`)
		require.NoError(t, err)

		assert.Error(t, seedBuiltinRoles(ctx, db), "권한 테이블이 없으면 시드가 실패해야 한다")
	})
}

// TestUsersRoleConstraint_NoUsersTable 는 users 테이블이 없을 때 no-op 임을 검증한다.
func TestUsersRoleConstraint_NoUsersTable(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "empty.db"))
	require.NoError(t, err)
	defer db.Close()

	assert.NoError(t, migrateUsersRoleConstraint(ctx, db))
}

// TestOpenSQLiteDB_MigratesLegacyDatabase 는 서버 기동 경로(OpenSQLiteDB) 로
// 진입해도 제약이 제거되고 역할 스키마가 시드됨을 검증한다.
func TestOpenSQLiteDB_MigratesLegacyDatabase(t *testing.T) {
	legacyDB, dbPath := setupLegacyUsersDB(t)
	before := snapshotUsers(t, legacyDB)
	require.NoError(t, legacyDB.Close())

	ctx := context.Background()
	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	defer db.Close()

	assert.NotContains(t, strings.ToUpper(usersDDL(t, db)), "CHECK")
	assert.Equal(t, before, snapshotUsers(t, db), "기동 마이그레이션이 사용자 데이터를 보존해야 한다")

	require.NoError(t, InsertUser(ctx, db, "operator-park", "$2a$10$operatorhash", "operator", 0, 0))

	roles, err := ListRoles(ctx, db)
	require.NoError(t, err)
	assert.Len(t, roles, 3, "기존 DB 에도 빌트인 역할이 시드되어야 한다")
}

// TestMigrateUsersSchema_FreshDatabaseHasNoConstraint 는 신규 DB 가 처음부터
// CHECK 제약 없이 생성됨을 검증한다.
func TestMigrateUsersSchema_FreshDatabaseHasNoConstraint(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	assert.NotContains(t, strings.ToUpper(usersDDL(t, db)), "CHECK")
	require.NoError(t, InsertUser(ctx, db, "operator-park", "$2a$10$operatorhash", "operator", 0, 0))

	// 기본값과 UNIQUE 제약은 유지된다.
	_, err := db.ExecContext(ctx, `
		INSERT INTO users(username, password_hash, created_at, updated_at) VALUES ('nodefault', 'h', 1, 1)
	`)
	require.NoError(t, err)
	got, err := GetUserByUsername(ctx, db, "nodefault")
	require.NoError(t, err)
	assert.Equal(t, "viewer", got.Role, "role 기본값 'viewer' 가 유지되어야 한다")
}
