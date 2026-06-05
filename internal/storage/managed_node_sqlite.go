// managed_node_sqlite.go 는 ManagedNodeRepository 의 SQLite 구현이다
// (@SPEC:SPEC-REMOTE-001 M2, spec §5.4 managed_nodes).
//
// FlowRepository/SQLiteRepository 패턴을 준용한다: WAL 모드, IF NOT EXISTS 멱등
// 마이그레이션. 기존 xflow.db 에 managed_nodes 테이블을 멱등하게 추가하므로
// 기존 DB(다른 테이블 보유)에서도 안전하다.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ ManagedNodeRepository = (*ManagedNodeSQLiteRepository)(nil)

// ManagedNodeSQLiteRepository 는 SQLite 기반 ManagedNodeRepository 구현이다.
type ManagedNodeSQLiteRepository struct {
	db *sql.DB
}

// NewManagedNodeSQLiteRepository 는 SQLite DB 를 열고 managed_nodes 테이블을
// 멱등하게 생성한 후 저장소를 반환한다.
//
// dbPath 의 부모 디렉토리가 없으면 자동 생성한다. WAL 모드를 활성화한다.
func NewManagedNodeSQLiteRepository(ctx context.Context, dbPath string) (*ManagedNodeSQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// DSN 에 WAL + busy_timeout pragma 를 실어 모든 풀 연결에 적용한다(다수 노드
	// online/last_seen 갱신 경합 흡수 — @SPEC:SPEC-REMOTE-001 M6, REQ-N01).
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrateManagedNodesSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &ManagedNodeSQLiteRepository{db: db}, nil
}

// migrateManagedNodesSchema 는 managed_nodes 테이블을 멱등하게 생성한다(spec §5.4).
//
// status 는 CHECK 제약으로 상태 머신 값만 허용한다(pending/approved/rejected/revoked).
// 시각 컬럼은 epoch milliseconds(int64) 이다.
func migrateManagedNodesSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS managed_nodes (
		instance_id TEXT    PRIMARY KEY,
		hostname    TEXT    NOT NULL DEFAULT '',
		version     TEXT    NOT NULL DEFAULT '',
		status      TEXT    NOT NULL DEFAULT 'pending'
		            CHECK (status IN ('pending', 'approved', 'rejected', 'revoked')),
		token_id    TEXT    NOT NULL DEFAULT '',
		last_seen   INTEGER NOT NULL DEFAULT 0,
		online      INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL,
		updated_at  INTEGER NOT NULL
	)`); err != nil {
		return fmt.Errorf("create managed_nodes table: %w", err)
	}
	return nil
}

// Upsert 는 노드를 저장한다. 동일 instance_id 가 있으면 메타/상태를 갱신하되
// created_at 은 보존한다(REQ-E06 last-known).
func (r *ManagedNodeSQLiteRepository) Upsert(ctx context.Context, node ManagedNode) error {
	now := time.Now().UnixMilli()
	createdAt := node.CreatedAt
	if createdAt == 0 {
		createdAt = now
	}
	status := node.Status
	if status == "" {
		status = "pending"
	}
	online := 0
	if node.Online {
		online = 1
	}

	_, err := r.db.ExecContext(ctx, `
		INSERT INTO managed_nodes (instance_id, hostname, version, status, token_id, last_seen, online, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(instance_id) DO UPDATE SET
			hostname   = excluded.hostname,
			version    = excluded.version,
			status     = excluded.status,
			token_id   = excluded.token_id,
			last_seen  = excluded.last_seen,
			online     = excluded.online,
			updated_at = excluded.updated_at
	`, node.InstanceID, node.Hostname, node.Version, status, node.TokenID,
		node.LastSeen, online, createdAt, now)
	if err != nil {
		return fmt.Errorf("upsert managed node: %w", err)
	}
	return nil
}

// Get 은 instance_id 로 노드를 조회한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) Get(ctx context.Context, instanceID string) (ManagedNode, error) {
	var (
		node   ManagedNode
		online int
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT instance_id, hostname, version, status, token_id, last_seen, online, created_at, updated_at
		FROM managed_nodes WHERE instance_id = ?
	`, instanceID).Scan(&node.InstanceID, &node.Hostname, &node.Version, &node.Status,
		&node.TokenID, &node.LastSeen, &online, &node.CreatedAt, &node.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ManagedNode{}, ErrManagedNodeNotFound
	}
	if err != nil {
		return ManagedNode{}, fmt.Errorf("get managed node: %w", err)
	}
	node.Online = online != 0
	return node, nil
}

// List 는 모든 관리 노드를 created_at 순서로 반환한다.
func (r *ManagedNodeSQLiteRepository) List(ctx context.Context) ([]ManagedNode, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT instance_id, hostname, version, status, token_id, last_seen, online, created_at, updated_at
		FROM managed_nodes ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list managed nodes: %w", err)
	}
	defer rows.Close()

	var out []ManagedNode
	for rows.Next() {
		var (
			node   ManagedNode
			online int
		)
		if err := rows.Scan(&node.InstanceID, &node.Hostname, &node.Version, &node.Status,
			&node.TokenID, &node.LastSeen, &online, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan managed node row: %w", err)
		}
		node.Online = online != 0
		out = append(out, node)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed node rows: %w", err)
	}
	return out, nil
}

// UpdateStatus 는 등록 상태를 전이한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) UpdateStatus(ctx context.Context, instanceID, status string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET status = ?, updated_at = ? WHERE instance_id = ?
	`, status, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "update managed node status")
}

// SetToken 은 노드 토큰 식별자를 저장한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) SetToken(ctx context.Context, instanceID, tokenID string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET token_id = ?, updated_at = ? WHERE instance_id = ?
	`, tokenID, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "set managed node token")
}

// SetOnline 은 online 상태와 last_seen 을 갱신한다(행 삭제 없음 — REQ-E06).
func (r *ManagedNodeSQLiteRepository) SetOnline(ctx context.Context, instanceID string, online bool, lastSeenMs int64) error {
	onlineInt := 0
	if online {
		onlineInt = 1
	}
	res, err := r.db.ExecContext(ctx, `
		UPDATE managed_nodes SET online = ?, last_seen = ?, updated_at = ? WHERE instance_id = ?
	`, onlineInt, lastSeenMs, time.Now().UnixMilli(), instanceID)
	return checkAffected(res, err, "set managed node online")
}

// Delete 는 instance_id 로 노드를 삭제한다. 없으면 ErrManagedNodeNotFound.
func (r *ManagedNodeSQLiteRepository) Delete(ctx context.Context, instanceID string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM managed_nodes WHERE instance_id = ?`, instanceID)
	return checkAffected(res, err, "delete managed node")
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *ManagedNodeSQLiteRepository) Close() error {
	return r.db.Close()
}

// checkAffected 는 ExecContext 결과를 검사하여 영향받은 행이 없으면
// ErrManagedNodeNotFound 를 반환한다.
func checkAffected(res sql.Result, execErr error, op string) error {
	if execErr != nil {
		return fmt.Errorf("%s: %w", op, execErr)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", op, err)
	}
	if affected == 0 {
		return ErrManagedNodeNotFound
	}
	return nil
}
