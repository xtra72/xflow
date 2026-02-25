package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/config"
)

// ---------------------------------------------------------------------------
// File 타입 팩토리
// ---------------------------------------------------------------------------

func TestNewRepository_File(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	cfg := config.StorageConfig{
		Type:          "file",
		FileDirectory: dir,
	}

	repo, err := NewRepository(ctx, cfg)
	require.NoError(t, err, "NewRepository(file) 실패")
	defer repo.Close()

	// 타입 확인
	_, ok := repo.(*FileRepository)
	assert.True(t, ok, "file 타입은 *FileRepository 이어야 한다")

	// CRUD 동작 확인
	f := newTestFlow("factory-file-test")
	err = repo.Save(ctx, f)
	require.NoError(t, err, "Save 실패")

	got, err := repo.Get(ctx, f.ID())
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, f.Name(), got.Name(), "Name 일치")
}

// ---------------------------------------------------------------------------
// SQLite 타입 팩토리
// ---------------------------------------------------------------------------

func TestNewRepository_SQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "factory-test.db")
	ctx := context.Background()

	cfg := config.StorageConfig{
		Type:       "sqlite",
		SQLitePath: dbPath,
	}

	repo, err := NewRepository(ctx, cfg)
	require.NoError(t, err, "NewRepository(sqlite) 실패")
	defer repo.Close()

	// 타입 확인
	_, ok := repo.(*SQLiteRepository)
	assert.True(t, ok, "sqlite 타입은 *SQLiteRepository 이어야 한다")

	// CRUD 동작 확인
	f := newTestFlow("factory-sqlite-test")
	err = repo.Save(ctx, f)
	require.NoError(t, err, "Save 실패")

	got, err := repo.Get(ctx, f.ID())
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, f.Name(), got.Name(), "Name 일치")
}

// ---------------------------------------------------------------------------
// Unknown 타입
// ---------------------------------------------------------------------------

func TestNewRepository_Unknown(t *testing.T) {
	ctx := context.Background()

	cfg := config.StorageConfig{
		Type: "mongodb",
	}

	repo, err := NewRepository(ctx, cfg)
	assert.Nil(t, repo, "unknown 타입은 nil 을 반환해야 한다")
	assert.Error(t, err, "unknown 타입은 에러를 반환해야 한다")
	assert.Contains(t, err.Error(), "mongodb", "에러 메시지에 타입명이 포함되어야 한다")
}

// ---------------------------------------------------------------------------
// Agent File 타입 팩토리
// ---------------------------------------------------------------------------

func TestNewAgentRepository_File(t *testing.T) {
	dir := t.TempDir()
	ctx := context.Background()

	cfg := config.StorageConfig{
		Type:          "file",
		FileDirectory: dir,
	}

	repo, err := NewAgentRepository(ctx, cfg)
	require.NoError(t, err, "NewAgentRepository(file) 실패")
	defer repo.Close()

	// 타입 확인
	_, ok := repo.(*AgentFileRepository)
	assert.True(t, ok, "file 타입은 *AgentFileRepository 이어야 한다")

	// CRUD 동작 확인
	ac := testAgentConfig("factory-agent-file", "Factory Agent File", "custom")
	err = repo.Save(ctx, ac)
	require.NoError(t, err, "Save 실패")

	got, err := repo.Get(ctx, ac.ID)
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, ac.Name, got.Name, "Name 일치")
}

// ---------------------------------------------------------------------------
// Agent SQLite 타입 팩토리
// ---------------------------------------------------------------------------

func TestNewAgentRepository_SQLite(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "factory-agent-test.db")
	ctx := context.Background()

	cfg := config.StorageConfig{
		Type:       "sqlite",
		SQLitePath: dbPath,
	}

	repo, err := NewAgentRepository(ctx, cfg)
	require.NoError(t, err, "NewAgentRepository(sqlite) 실패")
	defer repo.Close()

	// 타입 확인
	_, ok := repo.(*AgentSQLiteRepository)
	assert.True(t, ok, "sqlite 타입은 *AgentSQLiteRepository 이어야 한다")

	// CRUD 동작 확인
	ac := testAgentConfig("factory-agent-sqlite", "Factory Agent SQLite", "mqtt")
	err = repo.Save(ctx, ac)
	require.NoError(t, err, "Save 실패")

	got, err := repo.Get(ctx, ac.ID)
	require.NoError(t, err, "Get 실패")
	assert.Equal(t, ac.Name, got.Name, "Name 일치")
}

// ---------------------------------------------------------------------------
// Agent Unknown 타입
// ---------------------------------------------------------------------------

func TestNewAgentRepository_UnknownType(t *testing.T) {
	ctx := context.Background()

	cfg := config.StorageConfig{
		Type: "dynamodb",
	}

	repo, err := NewAgentRepository(ctx, cfg)
	assert.Nil(t, repo, "unknown 타입은 nil 을 반환해야 한다")
	assert.Error(t, err, "unknown 타입은 에러를 반환해야 한다")
	assert.Contains(t, err.Error(), "dynamodb", "에러 메시지에 타입명이 포함되어야 한다")
}
