package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/pkg/flow"

	_ "modernc.org/sqlite" // Pure Go SQLite driver registration
)

// SQLiteRepository 는 SQLite 기반의 FlowRepository 구현체이다.
type SQLiteRepository struct {
	db *sql.DB
}

// NewSQLiteRepository 는 SQLite 데이터베이스를 열고 테이블을 자동 생성한 후
// SQLiteRepository 를 반환한다.
//
// dbPath 의 부모 디렉토리가 없으면 자동으로 생성한다.
// WAL 모드를 활성화하여 동시 읽기 성능을 높인다.
func NewSQLiteRepository(ctx context.Context, dbPath string) (*SQLiteRepository, error) {
	// 부모 디렉토리 생성
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	// WAL 모드 활성화
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// 자동 마이그레이션 (flows)
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS flows (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		data       BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-1)
	// dashboards 와 users 테이블도 동일 xflow.db 에 자동 생성한다.
	// 본 호출이 flow 저장소 초기화 시점에 항상 실행되므로 부팅 순서와 무관하게 스키마가
	// 보장된다. dashboard / users 저장소가 별도 *sql.DB 핸들을 열어도 IF NOT EXISTS
	// 패턴이므로 멱등하다.
	if err := migrateDashboardSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateUsersSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	// @SPEC:SPEC-AUTH-005 (M1) — roles / role_permissions 스키마와 빌트인 역할 시드.
	// users 스키마(CHECK 제약 제거 포함) 이후에 수행되어야 한다.
	if err := migrateRolesSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &SQLiteRepository{db: db}, nil
}

// migrateDashboardSchema 는 dashboards 테이블과 (scope, owner) 부분 유니크 인덱스를
// 멱등하게 생성한다. modernc.org/sqlite 환경에서 COALESCE(owner, ”) 기반 표현식
// 인덱스는 정상 동작한다 (SPEC-DASHBOARD-001 v0.2.0 Risk Mitigation).
//
// 본 함수는 sqlite.go / dashboard_sqlite.go / 테스트 어디서 호출되어도 동일하게 동작
// 한다 (CREATE TABLE/INDEX IF NOT EXISTS).
func migrateDashboardSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dashboards (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		scope      TEXT    NOT NULL CHECK (scope IN ('global', 'user')),
		owner      TEXT,
		version    INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL,
		payload    TEXT    NOT NULL
	)`); err != nil {
		return fmt.Errorf("create dashboards table: %w", err)
	}

	// (scope, COALESCE(owner, '')) 부분 유니크: scope=global+NULL 은 빈 문자열로
	// 정규화되어 단일 row 만 허용되고, scope=user+owner 별로 1개씩 허용된다.
	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS dashboards_scope_owner_uidx
		ON dashboards(scope, COALESCE(owner, ''))`); err != nil {
		return fmt.Errorf("create dashboards index: %w", err)
	}
	return nil
}

// usersTableBody 는 users 테이블의 컬럼 정의이다 (CREATE TABLE 이후 부분).
//
// @SPEC:SPEC-AUTH-005 (M2)
// role 의 CHECK (role IN ('admin','editor','viewer')) 제약은 제거되었다.
// 관리자가 생성한 커스텀 역할을 사용자에게 부여할 수 있어야 하기 때문이다
// (spec.md §4.2). 신규 DB 는 처음부터 제약 없이 생성되고, 기존 DB 는
// migrateUsersRoleConstraint 가 테이블 재생성으로 제약을 제거한다.
const usersTableBody = ` (
	id            INTEGER PRIMARY KEY AUTOINCREMENT,
	username      TEXT    NOT NULL UNIQUE,
	password_hash TEXT    NOT NULL,
	role          TEXT    NOT NULL DEFAULT 'viewer',
	created_at    INTEGER NOT NULL,
	updated_at    INTEGER NOT NULL
)`

// usersColumns 는 users 테이블 재생성 시 복사할 컬럼 목록이다.
// SELECT * 대신 명시적 목록을 사용해 컬럼 순서 변화에 영향받지 않게 한다.
const usersColumns = `id, username, password_hash, role, created_at, updated_at`

// migrateUsersSchema 는 users 테이블을 멱등하게 생성하고, 기존 DB 에 남아 있는
// role CHECK 제약을 제거한다.
//
// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-1)
// 자격증명 yaml 을 대체하는 단일 source-of-truth 저장소.
// password_hash 는 bcrypt 결과를 그대로 저장한다.
//
// @SPEC:SPEC-AUTH-005 (M2)
// CHECK 제약 제거를 본 함수 내부에서 호출하여, 어느 호출 경로로 진입하든
// 커스텀 역할 저장이 가능한 상태가 보장되게 한다 (호출 누락 = 기능 결함).
func migrateUsersSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS users`+usersTableBody); err != nil {
		return fmt.Errorf("create users table: %w", err)
	}
	return migrateUsersRoleConstraint(ctx, db)
}

// migrateUsersRoleConstraint 는 기존 users 테이블의 role CHECK 제약을 제거한다.
//
// @SPEC:SPEC-AUTH-005 (M2, acceptance.md AC-02)
// SQLite 는 ALTER TABLE ... DROP CONSTRAINT 를 지원하지 않으므로 테이블 재생성
// 절차를 수행한다. 전 과정이 단일 트랜잭션이라 실패 시 원본 users 가 그대로
// 남는다 (plan.md §M2 롤백 계획).
//
// 절차:
//  1. sqlite_master 의 DDL 로 CHECK 존재 여부 판정 — 없으면 즉시 반환 (멱등).
//  2. 재생성 후 복원할 명시적 인덱스 DDL 수집 (UNIQUE(username) 은 테이블 정의에 포함).
//  3. users_new 생성 → 전체 행 복사 → 행 수 대조 (불일치 시 롤백).
//  4. DROP TABLE users → ALTER TABLE users_new RENAME TO users → 인덱스 재생성.
func migrateUsersRoleConstraint(ctx context.Context, db *sql.DB) error {
	var ddl string
	err := db.QueryRowContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&ddl)
	if errors.Is(err, sql.ErrNoRows) {
		// users 테이블 자체가 없음 → 마이그레이션 대상 아님.
		return nil
	}
	if err != nil {
		return fmt.Errorf("read users table ddl: %w", err)
	}
	if !strings.Contains(strings.ToUpper(ddl), "CHECK") {
		// 이미 제약이 없다 → no-op (재실행 안전).
		return nil
	}

	// 명시적으로 생성된 인덱스만 수집한다. UNIQUE 제약이 만드는 자동 인덱스는
	// sql 이 NULL 이며 새 테이블 정의에 의해 자동 재생성된다.
	idxRows, err := db.QueryContext(ctx,
		`SELECT sql FROM sqlite_master WHERE type='index' AND tbl_name='users' AND sql IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("read users indexes: %w", err)
	}
	var indexDDLs []string
	for idxRows.Next() {
		var s string
		if err := idxRows.Scan(&s); err != nil {
			idxRows.Close()
			return fmt.Errorf("scan users index ddl: %w", err)
		}
		indexDDLs = append(indexDDLs, s)
	}
	if err := idxRows.Err(); err != nil {
		idxRows.Close()
		return fmt.Errorf("iterate users indexes: %w", err)
	}
	idxRows.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin users constraint migration: %w", err)
	}
	// 커밋에 성공하면 Rollback 은 no-op 이다.
	defer func() { _ = tx.Rollback() }()

	var before int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&before); err != nil {
		return fmt.Errorf("count users before constraint migration: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `CREATE TABLE users_new`+usersTableBody); err != nil {
		return fmt.Errorf("create users_new table: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO users_new(`+usersColumns+`) SELECT `+usersColumns+` FROM users`); err != nil {
		return fmt.Errorf("copy users rows: %w", err)
	}

	var after int64
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users_new`).Scan(&after); err != nil {
		return fmt.Errorf("count users after copy: %w", err)
	}
	if before != after {
		// defer Rollback 이 원본 users 를 보존한다.
		return fmt.Errorf("users row count mismatch during constraint migration: before=%d after=%d", before, after)
	}

	if _, err := tx.ExecContext(ctx, `DROP TABLE users`); err != nil {
		return fmt.Errorf("drop old users table: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `ALTER TABLE users_new RENAME TO users`); err != nil {
		return fmt.Errorf("rename users_new to users: %w", err)
	}
	for _, idxDDL := range indexDDLs {
		if _, err := tx.ExecContext(ctx, idxDDL); err != nil {
			return fmt.Errorf("recreate users index: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit users constraint migration: %w", err)
	}
	return nil
}

// migrateRolesSchema 는 roles / role_permissions 테이블을 멱등하게 생성하고
// 빌트인 역할 3종을 시드한다.
//
// @SPEC:SPEC-AUTH-005 (M1, acceptance.md AC-01)
// 시드는 INSERT OR IGNORE + 복합 PK 로 멱등하다. 재실행 시 행이 중복 생성되지
// 않으며 오류도 발생하지 않는다. 카탈로그에 권한이 추가되면 다음 기동 시
// 빌트인 역할에 자동으로 반영된다 (기존 행은 유지).
func migrateRolesSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS roles (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		name        TEXT    NOT NULL UNIQUE,
		description TEXT    NOT NULL DEFAULT '',
		builtin     INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create roles table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS role_permissions (
		role_id    INTEGER NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
		permission TEXT    NOT NULL,
		PRIMARY KEY (role_id, permission)
	)`); err != nil {
		return fmt.Errorf("create role_permissions table: %w", err)
	}
	return seedBuiltinRoles(ctx, db)
}

// seedBuiltinRoles 는 빌트인 역할과 권한을 멱등하게 시드한다 (builtin = 1).
func seedBuiltinRoles(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin builtin role seed: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	now := time.Now().UnixMilli()
	for _, role := range rbac.BuiltinRoles() {
		res, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO roles(name, description, builtin, created_at, updated_at)
			VALUES (?, ?, 1, ?, ?)
		`, role.Name, role.Description, now, now)
		if err != nil {
			return fmt.Errorf("seed role %q: %w", role.Name, err)
		}
		created, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected for role %q: %w", role.Name, err)
		}

		var roleID int64
		if err := tx.QueryRowContext(ctx,
			`SELECT id FROM roles WHERE name = ?`, role.Name).Scan(&roleID); err != nil {
			return fmt.Errorf("resolve seeded role id %q: %w", role.Name, err)
		}

		// 신규 생성한 역할에만 권한을 시드한다. 이미 존재하던 역할의 권한 집합은
		// 관리자의 소유물이며(spec.md §2.1 — 서버가 editor/viewer 의 권한 수정을
		// 허용한다), 부팅마다 덮어쓰면 관리자의 수정이 재시작 때 조용히 사라진다.
		if created == 1 {
			for _, perm := range role.Permissions {
				if _, err := tx.ExecContext(ctx, `
					INSERT OR IGNORE INTO role_permissions(role_id, permission) VALUES (?, ?)
				`, roleID, perm); err != nil {
					return fmt.Errorf("seed permission %q for role %q: %w", perm, role.Name, err)
				}
			}
			continue
		}

		// 예외: admin 은 항상 카탈로그 전체 권한을 보유해야 한다(spec.md §2.4 UB1-4).
		// 관리자도 수정할 수 없는 불변식이므로 부팅 시 코드 정의로 되맞춘다. 이 자동
		// 복구가 없으면 어떤 이유로든 축소된 admin 이 스스로 회복하지 못하고, 관리
		// 권한을 가진 사용자가 조용히 기능을 잃는다.
		if role.Name == rbac.RoleAdmin {
			if err := reconcileRolePermissions(ctx, tx, roleID, role.Permissions); err != nil {
				return fmt.Errorf("reconcile role %q: %w", role.Name, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit builtin role seed: %w", err)
	}
	return nil
}

// reconcileRolePermissions 는 role_id 의 권한 집합을 want 와 정확히 일치시킨다.
// 누락분은 추가하고 카탈로그에 없는 잔여분은 제거한다. 매 부팅 전량 삭제·재삽입을
// 피하기 위해 차집합만 건드린다.
func reconcileRolePermissions(ctx context.Context, tx *sql.Tx, roleID int64, want []string) error {
	for _, perm := range want {
		if _, err := tx.ExecContext(ctx, `
			INSERT OR IGNORE INTO role_permissions(role_id, permission) VALUES (?, ?)
		`, roleID, perm); err != nil {
			return fmt.Errorf("add permission %q: %w", perm, err)
		}
	}

	if len(want) == 0 {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
			return fmt.Errorf("clear permissions: %w", err)
		}
		return nil
	}

	args := make([]any, 0, len(want)+1)
	args = append(args, roleID)
	placeholders := make([]string, len(want))
	for i, perm := range want {
		placeholders[i] = "?"
		args = append(args, perm)
	}
	query := `DELETE FROM role_permissions WHERE role_id = ? AND permission NOT IN (` +
		strings.Join(placeholders, ",") + `)`
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("prune permissions: %w", err)
	}
	return nil
}


// OpenSQLiteDB 는 SQLite 데이터베이스를 WAL 모드로 열고, dashboards / users 스키마를
// 멱등하게 마이그레이션한다. 호출자가 *sql.DB 의 수명을 책임진다 (Close 필요).
//
// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-8)
// main.go 가 CredentialsManager / DashboardRepository 에 *sql.DB 를 주입하기 위해 사용한다.
// 기존 NewSQLiteRepository / NewAgentSQLiteRepository 는 자체 *sql.DB 를 열지만,
// WAL 모드 하에서 동일 파일에 다중 핸들이 안전하게 공존한다 (ASM-007).
func OpenSQLiteDB(ctx context.Context, dbPath string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	if err := migrateDashboardSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	if err := migrateUsersSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	// @SPEC:SPEC-AUTH-005 (M1) — roles / role_permissions 스키마와 빌트인 역할 시드.
	if err := migrateRolesSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Save 는 플로우를 저장한다. 동일 ID 가 있으면 덮어쓴다.
func (r *SQLiteRepository) Save(ctx context.Context, f flow.Flow) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal flow: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO flows (id, name, data, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name       = excluded.name,
			data       = excluded.data,
			updated_at = CURRENT_TIMESTAMP
	`, f.ID(), f.Name(), data)
	if err != nil {
		return fmt.Errorf("save flow: %w", err)
	}

	return nil
}

// Get 은 ID 로 플로우를 조회한다. 없으면 ErrFlowNotFound 를 반환한다.
func (r *SQLiteRepository) Get(ctx context.Context, id string) (flow.Flow, error) {
	var data []byte
	err := r.db.QueryRowContext(ctx, "SELECT data FROM flows WHERE id = ?", id).Scan(&data)
	if err == sql.ErrNoRows {
		return nil, ErrFlowNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get flow: %w", err)
	}

	f, err := flow.FlowFromJSON(data)
	if err != nil {
		return nil, fmt.Errorf("unmarshal flow: %w", err)
	}

	return f, nil
}

// List 는 저장소의 모든 플로우를 생성 시각 순서로 반환한다.
func (r *SQLiteRepository) List(ctx context.Context) ([]flow.Flow, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT data FROM flows ORDER BY created_at")
	if err != nil {
		return nil, fmt.Errorf("list flows: %w", err)
	}
	defer rows.Close()

	var flows []flow.Flow
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan flow row: %w", err)
		}

		f, err := flow.FlowFromJSON(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshal flow: %w", err)
		}
		flows = append(flows, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flow rows: %w", err)
	}

	return flows, nil
}

// Delete 는 ID 로 플로우를 삭제한다. 없으면 ErrFlowNotFound 를 반환한다.
func (r *SQLiteRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM flows WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete flow: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return ErrFlowNotFound
	}

	return nil
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *SQLiteRepository) Close() error {
	return r.db.Close()
}
