// node_version_history_sqlite.go 는 NodeVersionHistoryRepository 의 SQLite 구현이다
// (@SPEC:SPEC-REMOTE-001 버전 관리 Phase 1).
//
// managed_node_sqlite.go 패턴을 준용한다: WAL 모드 DSN, IF NOT EXISTS 멱등 마이그레이션.
// 기존 xflow.db 에 node_version_history 테이블을 멱등하게 추가하므로 기존 DB 에서도 안전하다.
package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ NodeVersionHistoryRepository = (*NodeVersionHistorySQLiteRepository)(nil)

// NodeVersionHistorySQLiteRepository 는 SQLite 기반 구현이다.
type NodeVersionHistorySQLiteRepository struct {
	db *sql.DB
}

// NewNodeVersionHistorySQLiteRepository 는 SQLite DB 를 열고 node_version_history
// 테이블을 멱등하게 생성한 후 저장소를 반환한다.
func NewNodeVersionHistorySQLiteRepository(ctx context.Context, dbPath string) (*NodeVersionHistorySQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := migrateNodeVersionHistorySchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}
	return &NodeVersionHistorySQLiteRepository{db: db}, nil
}

// migrateNodeVersionHistorySchema 는 테이블 + 인덱스를 멱등하게 생성한다.
func migrateNodeVersionHistorySchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS node_version_history (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		instance_id TEXT    NOT NULL,
		version     TEXT    NOT NULL,
		changed_at  INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create node_version_history table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_nvh_instance_changed
		ON node_version_history(instance_id, changed_at)`); err != nil {
		return fmt.Errorf("create node_version_history index: %w", err)
	}
	return nil
}

// Append 는 한 노드의 새 버전 관측을 이력에 추가한다.
func (r *NodeVersionHistorySQLiteRepository) Append(ctx context.Context, instanceID, version string, changedAtMs int64) error {
	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO node_version_history (instance_id, version, changed_at)
		VALUES (?, ?, ?)
	`, instanceID, version, changedAtMs); err != nil {
		return fmt.Errorf("append node version history: %w", err)
	}
	return nil
}

// List 는 instance_id 의 버전 이력을 최신순(changed_at DESC)으로 반환한다.
// limit <= 0 이면 전체를 반환한다(id DESC 보조 정렬로 동시각 결정적 순서 보장).
func (r *NodeVersionHistorySQLiteRepository) List(ctx context.Context, instanceID string, limit int) ([]NodeVersionHistory, error) {
	query := `SELECT instance_id, version, changed_at FROM node_version_history
		WHERE instance_id = ? ORDER BY changed_at DESC, id DESC`
	args := []any{instanceID}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list node version history: %w", err)
	}
	defer rows.Close()

	var out []NodeVersionHistory
	for rows.Next() {
		var h NodeVersionHistory
		if err := rows.Scan(&h.InstanceID, &h.Version, &h.ChangedAt); err != nil {
			return nil, fmt.Errorf("scan node version history: %w", err)
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate node version history: %w", err)
	}
	return out, nil
}

// Close 는 DB 연결을 닫는다.
func (r *NodeVersionHistorySQLiteRepository) Close() error {
	return r.db.Close()
}
