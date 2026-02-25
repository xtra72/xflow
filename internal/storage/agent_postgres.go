package storage

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xtra/xflow/internal/agent"
)

// AgentPostgresRepository 는 PostgreSQL 기반의 AgentRepository 구현체이다.
// pgxpool 을 사용하여 커넥션 풀링을 지원한다.
type AgentPostgresRepository struct {
	pool *pgxpool.Pool
}

// 컴파일 타임 인터페이스 충족 검증
var _ AgentRepository = (*AgentPostgresRepository)(nil)

// NewAgentPostgresRepository 는 PostgreSQL 에 연결하고 agents 테이블을 자동 생성한 후
// AgentPostgresRepository 를 반환한다.
//
// dsn 은 PostgreSQL 연결 문자열이다 (예: "postgres://user:pass@localhost:5432/xflow").
// poolSize 가 0보다 크면 최대 커넥션 수를 해당 값으로 설정한다.
func NewAgentPostgresRepository(ctx context.Context, dsn string, poolSize int) (*AgentPostgresRepository, error) {
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}
	if poolSize > 0 {
		config.MaxConns = int32(poolSize)
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("connect postgres: %w", err)
	}

	// 연결 검증
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	// 자동 마이그레이션
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS agents (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		type       TEXT NOT NULL,
		data       JSONB NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	)`); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &AgentPostgresRepository{pool: pool}, nil
}

// Save 는 에이전트 설정을 저장한다. 동일 ID 가 있으면 덮어쓴다.
func (r *AgentPostgresRepository) Save(ctx context.Context, config agent.AgentConfig) error {
	data, err := agent.AgentConfigToJSON(config)
	if err != nil {
		return fmt.Errorf("marshal agent config: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO agents (id, name, type, data, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name       = EXCLUDED.name,
			type       = EXCLUDED.type,
			data       = EXCLUDED.data,
			updated_at = NOW()
	`, config.ID, config.Name, config.Type, data)
	if err != nil {
		return fmt.Errorf("save agent config: %w", err)
	}

	return nil
}

// Get 은 ID 로 에이전트 설정을 조회한다. 없으면 ErrAgentNotFound 를 반환한다.
func (r *AgentPostgresRepository) Get(ctx context.Context, id string) (agent.AgentConfig, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, "SELECT data FROM agents WHERE id = $1", id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
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
func (r *AgentPostgresRepository) List(ctx context.Context) ([]agent.AgentConfig, error) {
	rows, err := r.pool.Query(ctx, "SELECT data FROM agents ORDER BY created_at")
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
func (r *AgentPostgresRepository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM agents WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete agent config: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrAgentNotFound
	}

	return nil
}

// Close 는 커넥션 풀을 닫는다.
func (r *AgentPostgresRepository) Close() error {
	r.pool.Close()
	return nil
}
