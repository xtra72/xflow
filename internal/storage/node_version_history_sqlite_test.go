package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestVersionHistoryRepo(t *testing.T) *NodeVersionHistorySQLiteRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "vh.db")
	repo, err := NewNodeVersionHistorySQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

func TestNodeVersionHistory_AppendAndList(t *testing.T) {
	repo := newTestVersionHistoryRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Append(ctx, "node-a", "v1.0.0", 1000))
	require.NoError(t, repo.Append(ctx, "node-a", "v1.1.0", 2000))
	require.NoError(t, repo.Append(ctx, "node-a", "v1.2.0", 3000))
	// 다른 노드는 분리되어야 한다.
	require.NoError(t, repo.Append(ctx, "node-b", "v0.9.0", 1500))

	hist, err := repo.List(ctx, "node-a", 0)
	require.NoError(t, err)
	require.Len(t, hist, 3)
	// 최신순(changed_at DESC).
	assert.Equal(t, "v1.2.0", hist[0].Version)
	assert.Equal(t, int64(3000), hist[0].ChangedAt)
	assert.Equal(t, "v1.1.0", hist[1].Version)
	assert.Equal(t, "v1.0.0", hist[2].Version)
	for _, h := range hist {
		assert.Equal(t, "node-a", h.InstanceID)
	}

	other, err := repo.List(ctx, "node-b", 0)
	require.NoError(t, err)
	require.Len(t, other, 1)
	assert.Equal(t, "v0.9.0", other[0].Version)
}

func TestNodeVersionHistory_ListLimit(t *testing.T) {
	repo := newTestVersionHistoryRepo(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, repo.Append(ctx, "n", "v"+string(rune('1'+i))+".0.0", int64(1000*(i+1))))
	}
	hist, err := repo.List(ctx, "n", 2)
	require.NoError(t, err)
	require.Len(t, hist, 2)
	// 최신 2건만.
	assert.Equal(t, int64(5000), hist[0].ChangedAt)
	assert.Equal(t, int64(4000), hist[1].ChangedAt)
}

func TestNodeVersionHistory_ListEmpty(t *testing.T) {
	repo := newTestVersionHistoryRepo(t)
	hist, err := repo.List(context.Background(), "none", 0)
	require.NoError(t, err)
	assert.Empty(t, hist)
}

func TestNodeVersionHistory_MigrationIdempotent(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "vh.db")
	r1, err := NewNodeVersionHistorySQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	require.NoError(t, r1.Append(context.Background(), "n", "v1.0.0", 1000))
	require.NoError(t, r1.Close())
	// 같은 DB 재오픈 — 마이그레이션 멱등 + 기존 데이터 보존.
	r2, err := NewNodeVersionHistorySQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = r2.Close() })
	hist, err := r2.List(context.Background(), "n", 0)
	require.NoError(t, err)
	require.Len(t, hist, 1)
}
