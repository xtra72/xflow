package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xtra/xflow/pkg/flow"
)

// PostgresRepository 는 PostgreSQL 기반의 FlowRepository 구현체이다.
// pgxpool 을 사용하여 커넥션 풀링을 지원한다.
type PostgresRepository struct {
	pool *pgxpool.Pool
}

// 컴파일 타임 인터페이스 충족 검증
var _ FlowRepository = (*PostgresRepository)(nil)

// NewPostgresRepository 는 PostgreSQL 에 연결하고 테이블을 자동 생성한 후
// PostgresRepository 를 반환한다.
//
// dsn 은 PostgreSQL 연결 문자열이다 (예: "postgres://user:pass@localhost:5432/xflow").
// poolSize 가 0보다 크면 최대 커넥션 수를 해당 값으로 설정한다.
func NewPostgresRepository(ctx context.Context, dsn string, poolSize int) (*PostgresRepository, error) {
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
	if _, err := pool.Exec(ctx, `CREATE TABLE IF NOT EXISTS flows (
		id         TEXT PRIMARY KEY,
		name       TEXT NOT NULL,
		data       JSONB NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		updated_at TIMESTAMPTZ DEFAULT NOW()
	)`); err != nil {
		pool.Close()
		return nil, fmt.Errorf("create table: %w", err)
	}

	return &PostgresRepository{pool: pool}, nil
}

// Save 는 플로우를 저장한다. 동일 ID 가 있으면 덮어쓴다.
func (r *PostgresRepository) Save(ctx context.Context, f flow.Flow) error {
	data, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal flow: %w", err)
	}

	_, err = r.pool.Exec(ctx, `
		INSERT INTO flows (id, name, data, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (id) DO UPDATE SET
			name       = EXCLUDED.name,
			data       = EXCLUDED.data,
			updated_at = NOW()
	`, f.ID(), f.Name(), data)
	if err != nil {
		return fmt.Errorf("save flow: %w", err)
	}

	return nil
}

// Get 은 ID 로 플로우를 조회한다. 없으면 ErrFlowNotFound 를 반환한다.
func (r *PostgresRepository) Get(ctx context.Context, id string) (flow.Flow, error) {
	var data []byte
	err := r.pool.QueryRow(ctx, "SELECT data FROM flows WHERE id = $1", id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
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
func (r *PostgresRepository) List(ctx context.Context) ([]flow.Flow, error) {
	rows, err := r.pool.Query(ctx, "SELECT data FROM flows ORDER BY created_at")
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
func (r *PostgresRepository) Delete(ctx context.Context, id string) error {
	result, err := r.pool.Exec(ctx, "DELETE FROM flows WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete flow: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrFlowNotFound
	}

	return nil
}

// Close 는 커넥션 풀을 닫는다.
func (r *PostgresRepository) Close() error {
	r.pool.Close()
	return nil
}
