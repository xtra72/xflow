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

	// 자동 마이그레이션
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

	return &SQLiteRepository{db: db}, nil
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
