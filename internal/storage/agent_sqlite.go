package storage

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xtra/xflow/internal/agent"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증
var _ AgentRepository = (*AgentSQLiteRepository)(nil)

// AgentSQLiteRepository 는 SQLite 기반의 AgentRepository 구현체이다.
type AgentSQLiteRepository struct {
	db *sql.DB
}

// NewAgentSQLiteRepository 는 SQLite 데이터베이스를 열고 agents 테이블을 자동 생성한 후
// AgentSQLiteRepository 를 반환한다.
//
// dbPath 의 부모 디렉토리가 없으면 자동으로 생성한다.
// WAL 모드를 활성화하여 동시 읽기 성능을 높인다.
func NewAgentSQLiteRepository(ctx context.Context, dbPath string) (*AgentSQLiteRepository, error) {
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
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS agents (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		type       TEXT NOT NULL,
		data       BLOB NOT NULL,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`); err != nil {
		db.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &AgentSQLiteRepository{db: db}, nil
}

// Save 는 에이전트 설정을 저장한다. 동일 ID 가 있으면 덮어쓴다.
func (r *AgentSQLiteRepository) Save(ctx context.Context, config agent.AgentConfig) error {
	data, err := agent.AgentConfigToJSON(config)
	if err != nil {
		return fmt.Errorf("marshal agent config: %w", err)
	}

	_, err = r.db.ExecContext(ctx, `
		INSERT INTO agents (id, name, type, data, updated_at)
		VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(id) DO UPDATE SET
			name       = excluded.name,
			type       = excluded.type,
			data       = excluded.data,
			updated_at = CURRENT_TIMESTAMP
	`, config.ID, config.Name, config.Type, data)
	if err != nil {
		return fmt.Errorf("save agent config: %w", err)
	}

	return nil
}

// Get 은 ID 로 에이전트 설정을 조회한다. 없으면 ErrAgentNotFound 를 반환한다.
func (r *AgentSQLiteRepository) Get(ctx context.Context, id string) (agent.AgentConfig, error) {
	var data []byte
	err := r.db.QueryRowContext(ctx, "SELECT data FROM agents WHERE id = ?", id).Scan(&data)
	if err == sql.ErrNoRows {
		return agent.AgentConfig{}, ErrAgentNotFound
	}
	if err != nil {
		return agent.AgentConfig{}, fmt.Errorf("get agent config: %w", err)
	}

	config, err := agent.AgentConfigFromJSON(data)
	if err != nil {
		return agent.AgentConfig{}, fmt.Errorf("unmarshal agent config: %w", err)
	}

	return config, nil
}

// List 는 저장소의 모든 에이전트 설정을 생성 시각 순서로 반환한다.
func (r *AgentSQLiteRepository) List(ctx context.Context) ([]agent.AgentConfig, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT data FROM agents ORDER BY created_at")
	if err != nil {
		return nil, fmt.Errorf("list agent configs: %w", err)
	}
	defer rows.Close()

	var configs []agent.AgentConfig
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, fmt.Errorf("scan agent row: %w", err)
		}

		config, err := agent.AgentConfigFromJSON(data)
		if err != nil {
			return nil, fmt.Errorf("unmarshal agent config: %w", err)
		}
		configs = append(configs, config)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate agent rows: %w", err)
	}

	return configs, nil
}

// Delete 는 ID 로 에이전트 설정을 삭제한다. 없으면 ErrAgentNotFound 를 반환한다.
func (r *AgentSQLiteRepository) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, "DELETE FROM agents WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete agent config: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return ErrAgentNotFound
	}

	return nil
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *AgentSQLiteRepository) Close() error {
	return r.db.Close()
}
