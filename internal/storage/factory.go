package storage

import (
	"context"
	"fmt"

	"github.com/xtra/xflow/internal/config"
)

// NewRepository 는 설정에 따라 적절한 FlowRepository 구현을 생성한다.
//
//   - "file":     FileRepository (YAML 파일 기반)
//   - "sqlite":   SQLiteRepository (Pure Go SQLite)
//   - "postgres": PostgresRepository (pgx 커넥션 풀)
func NewRepository(ctx context.Context, cfg config.StorageConfig) (FlowRepository, error) {
	switch cfg.Type {
	case "file":
		return NewFileRepository(cfg.FileDirectory)
	case "sqlite":
		return NewSQLiteRepository(ctx, cfg.SQLitePath)
	case "postgres":
		return NewPostgresRepository(ctx, cfg.PostgresDSN, cfg.PoolSize)
	default:
		return nil, fmt.Errorf("unknown storage type: %q", cfg.Type)
	}
}

// NewAgentRepository 는 설정에 따라 적절한 AgentRepository 구현을 생성한다.
//
//   - "file":     AgentFileRepository (YAML 파일 기반)
//   - "sqlite":   AgentSQLiteRepository (Pure Go SQLite)
//   - "postgres": AgentPostgresRepository (pgx 커넥션 풀)
func NewAgentRepository(ctx context.Context, cfg config.StorageConfig) (AgentRepository, error) {
	switch cfg.Type {
	case "file":
		return NewAgentFileRepository(cfg.FileDirectory)
	case "sqlite":
		return NewAgentSQLiteRepository(ctx, cfg.SQLitePath)
	case "postgres":
		return NewAgentPostgresRepository(ctx, cfg.PostgresDSN, cfg.PoolSize)
	default:
		return nil, fmt.Errorf("unknown storage type: %q", cfg.Type)
	}
}
