package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

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

// migrateUsersSchema 는 users 테이블을 멱등하게 생성한다.
//
// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-1)
// 자격증명 yaml 을 대체하는 단일 source-of-truth 저장소.
// password_hash 는 bcrypt 결과를 그대로 저장한다.
func migrateUsersSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS users (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		username      TEXT    NOT NULL UNIQUE,
		password_hash TEXT    NOT NULL,
		role          TEXT    NOT NULL DEFAULT 'viewer'
		              CHECK (role IN ('admin', 'editor', 'viewer')),
		created_at    INTEGER NOT NULL,
		updated_at    INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create users table: %w", err)
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
