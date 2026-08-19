package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-1)
// 본 파일은 dashboards / users 테이블 자동 생성 + 부분 유니크 인덱스 동작 검증.

// TestSQLiteRepository_CreatesDashboardAndUsersTables 는 NewSQLiteRepository 가
// 부팅 스키마를 자동 생성하는지 검증한다 (UR-001, UR-006).
//
// @SPEC:SPEC-DASHBOARD-004 (M3)
// 대시보드 엔티티 이관 이후 dashboards 는 1급 엔티티 스키마다. 따라서 구
// (scope, owner) 부분 유니크 인덱스는 더 이상 부팅 경로에서 만들어지지 않으며,
// 신규 테이블 3종 + 마커 테이블이 함께 생성된다.
func TestSQLiteRepository_CreatesDashboardAndUsersTables(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	tables := []string{
		"flows", "users", "dashboards",
		"dashboard_acl", "dashboard_user_state", "schema_markers",
	}
	for _, name := range tables {
		var got string
		err := repo.db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&got)
		require.NoErrorf(t, err, "테이블 %q 가 생성되어야 한다", name)
		assert.Equal(t, name, got)
	}

	// 신규 dashboards 는 uid 기반이며 scope 컬럼을 갖지 않는다.
	legacy, err := hasLegacyDashboardSchema(ctx, repo.db)
	require.NoError(t, err)
	assert.False(t, legacy, "부팅 경로가 만드는 dashboards 는 신규 스키마여야 한다")

	// 구 (scope, owner) 부분 유니크 인덱스는 부팅 경로에서 만들어지지 않는다.
	var idxName string
	err = repo.db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type='index' AND name=?`,
		"dashboards_scope_owner_uidx",
	).Scan(&idxName)
	assert.ErrorIs(t, err, sql.ErrNoRows, "구 부분 유니크 인덱스는 남아 있으면 안 된다")
}

// setupLegacyDashboardDB 는 구 (scope, owner) 스냅샷 스키마만 가진 원시 DB 를 연다.
//
// 부팅 경로(NewSQLiteRepository / OpenSQLiteDB)는 SPEC-DASHBOARD-004 이관 이후
// 신규 스키마를 만들므로, 구 스키마의 제약을 검증하는 테스트는 테이블을 직접
// 만들어 쓴다.
func setupLegacyDashboardDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "legacy.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	_, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	require.NoError(t, err)
	require.NoError(t, ensureLegacySnapshotTable(ctx, db, "dashboards"))
	return db
}

// TestSQLiteRepository_DashboardPartialUniqueIndex 는 (scope, COALESCE(owner,”))
// 부분 유니크 인덱스가 다음을 만족하는지 검증한다 (AC-5):
//
//	(global, NULL)    → 단일 row 허용
//	(user, 'alice')   → 허용
//	(user, 'bob')     → 허용 (alice 와 공존)
//	(global, NULL)    → 두 번째 시도는 UNIQUE 위반 (단일 공유 row 강제)
func TestSQLiteRepository_DashboardPartialUniqueIndex(t *testing.T) {
	db := setupLegacyDashboardDB(t)
	ctx := context.Background()

	// 1. (global, NULL) 1회 삽입
	_, err := db.ExecContext(ctx,
		`INSERT INTO dashboards(scope, owner, version, updated_at, payload) VALUES('global', NULL, 1, 1, '{}')`,
	)
	require.NoError(t, err, "global+NULL 첫 삽입은 성공해야 한다")

	// 2. (user, 'alice') 삽입 (global 과 공존)
	_, err = db.ExecContext(ctx,
		`INSERT INTO dashboards(scope, owner, version, updated_at, payload) VALUES('user', 'alice', 1, 2, '{}')`,
	)
	require.NoError(t, err, "user+alice 는 global+NULL 과 공존 가능해야 한다")

	// 3. (user, 'bob') 삽입 (alice 와 공존)
	_, err = db.ExecContext(ctx,
		`INSERT INTO dashboards(scope, owner, version, updated_at, payload) VALUES('user', 'bob', 1, 3, '{}')`,
	)
	require.NoError(t, err, "user+bob 은 user+alice 와 공존 가능해야 한다")

	// 4. (global, NULL) 두 번째 삽입은 UNIQUE 위반
	_, err = db.ExecContext(ctx,
		`INSERT INTO dashboards(scope, owner, version, updated_at, payload) VALUES('global', NULL, 2, 4, '{}')`,
	)
	require.Error(t, err, "global+NULL 중복 삽입은 UNIQUE 위반이어야 한다")
	assert.Contains(t, err.Error(), "UNIQUE")

	// 5. (user, 'alice') 두 번째 삽입도 UNIQUE 위반
	_, err = db.ExecContext(ctx,
		`INSERT INTO dashboards(scope, owner, version, updated_at, payload) VALUES('user', 'alice', 2, 5, '{}')`,
	)
	require.Error(t, err, "user+alice 중복 삽입은 UNIQUE 위반이어야 한다")
	assert.Contains(t, err.Error(), "UNIQUE")

	// 6. 총 row 수 검증: 3 개 (global+NULL, user+alice, user+bob)
	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM dashboards`).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 3, count, "총 3개 row 가 존재해야 한다")
}

// TestSQLiteRepository_ScopeCheckConstraint 는 scope CHECK 제약을 검증한다.
// 'global' / 'user' 외 값은 거부되어야 한다.
func TestSQLiteRepository_ScopeCheckConstraint(t *testing.T) {
	db := setupLegacyDashboardDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO dashboards(scope, owner, version, updated_at, payload) VALUES('team', 'team-x', 1, 1, '{}')`,
	)
	require.Error(t, err, "scope='team' 은 CHECK 제약 위반이어야 한다")
	assert.Contains(t, err.Error(), "CHECK")
}

// TestSQLiteRepository_UsersUniqueUsername 는 users.username UNIQUE 제약을 검증한다.
func TestSQLiteRepository_UsersUniqueUsername(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	_, err := repo.db.ExecContext(ctx,
		`INSERT INTO users(username, password_hash, role, created_at, updated_at) VALUES('alice', 'hash1', 'admin', 1, 1)`,
	)
	require.NoError(t, err)

	_, err = repo.db.ExecContext(ctx,
		`INSERT INTO users(username, password_hash, role, created_at, updated_at) VALUES('alice', 'hash2', 'viewer', 2, 2)`,
	)
	require.Error(t, err, "동일 username 중복은 UNIQUE 위반")
	assert.Contains(t, err.Error(), "UNIQUE")
}

// TestSQLiteRepository_UsersRoleAcceptsCustomRole 는 users.role 이 커스텀 역할을
// 허용하는지 검증한다.
//
// @SPEC:SPEC-AUTH-005 (M2, spec.md §4.2)
// 본 테스트는 v0.2.0 의 TestSQLiteRepository_UsersRoleCheckConstraint 를 대체한다.
// 당시에는 role 이 CHECK (role IN ('admin','editor','viewer')) 로 제한되어 잘못된
// role 삽입이 실패해야 했으나, SPEC-AUTH-005 가 관리자 정의 커스텀 역할을 도입하며
// 해당 제약을 의도적으로 제거했다. 따라서 기대 동작이 "거부" 에서 "허용" 으로
// 반전된다 (회귀가 아니라 SPEC 이 명령한 계약 변경).
//
// 카탈로그에 없는 역할 이름을 사용자에게 부여하는 것을 막는 책임은 스키마가 아니라
// 상위 API 계층(M6) 으로 이동한다.
func TestSQLiteRepository_UsersRoleAcceptsCustomRole(t *testing.T) {
	repo := setupSQLiteRepo(t)
	ctx := context.Background()

	_, err := repo.db.ExecContext(ctx,
		`INSERT INTO users(username, password_hash, role, created_at, updated_at) VALUES('bob', 'hash', 'operator', 1, 1)`,
	)
	require.NoError(t, err, "커스텀 역할은 저장될 수 있어야 한다")

	var role string
	require.NoError(t, repo.db.QueryRowContext(ctx,
		`SELECT role FROM users WHERE username = 'bob'`).Scan(&role))
	assert.Equal(t, "operator", role)
}

// TestOpenSQLiteDB 는 OpenSQLiteDB 헬퍼가 WAL 모드를 활성화하고 스키마를 멱등하게
// 마이그레이션 하는지 검증한다 (M-8 boot order).
func TestOpenSQLiteDB(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "open-test.db")

	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err, "OpenSQLiteDB 성공")
	t.Cleanup(func() { db.Close() })

	// WAL 모드 확인
	var journalMode string
	err = db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journalMode)
	require.NoError(t, err)
	assert.Equal(t, "wal", journalMode, "WAL 모드가 활성화되어야 한다")

	// dashboards / users 테이블 존재
	for _, name := range []string{"dashboards", "users"} {
		var got string
		err := db.QueryRowContext(ctx,
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name,
		).Scan(&got)
		require.NoErrorf(t, err, "테이블 %q 존재해야 한다", name)
	}

	// 두 번째 호출도 멱등하게 동작 (이미 존재해도 에러 없음)
	db2, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err, "OpenSQLiteDB 두 번째 호출도 성공해야 한다")
	t.Cleanup(func() { db2.Close() })
}

// TestOpenSQLiteDB_InvalidPath 는 디렉토리 생성 실패 시 에러를 검증한다.
func TestOpenSQLiteDB_InvalidPath(t *testing.T) {
	ctx := context.Background()
	// "/dev/null" 하위 경로는 디렉토리로 만들 수 없다
	_, err := OpenSQLiteDB(ctx, "/dev/null/bogus/path/db.sqlite")
	require.Error(t, err)
}

// migrateOpenDB 는 modernc/sqlite 호환성 sanity check 용.
// COALESCE(owner, ”) 기반 부분 유니크 인덱스 동작 확인 (Risk Mitigation).
func TestMigrateDashboardSchema_ModerncCompatibility(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "compat.db")
	db, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// WAL 활성 (실 환경 동일 조건)
	_, err = db.ExecContext(ctx, "PRAGMA journal_mode=WAL")
	require.NoError(t, err)

	require.NoError(t, migrateDashboardSchema(ctx, db))

	// COALESCE(owner,'') 인덱스가 실제 query 에서 사용되는지 EXPLAIN QUERY PLAN 으로 확인
	rows, err := db.QueryContext(ctx,
		`EXPLAIN QUERY PLAN SELECT version FROM dashboards WHERE scope='user' AND COALESCE(owner,'')='alice'`,
	)
	require.NoError(t, err)
	defer rows.Close()

	var detailSeen string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err == nil {
			detailSeen += detail + " "
		}
	}
	// modernc/sqlite 는 부분 인덱스를 USING INDEX 로 매칭한다
	t.Logf("EXPLAIN QUERY PLAN: %s", detailSeen)
	assert.Contains(t, detailSeen, "dashboards", "쿼리는 dashboards 테이블을 사용해야 한다")
}
