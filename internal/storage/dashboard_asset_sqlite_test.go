package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newAssetTestRepo 는 임시 파일 SQLite 위에 자산 저장소를 만든다.
func newAssetTestRepo(t *testing.T) *DashboardAssetSQLiteRepository {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "assets.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo, err := NewDashboardAssetSQLiteRepository(context.Background(), db)
	require.NoError(t, err)
	return repo
}

func TestDashboardAssetSQLite_PutGet_RoundTrip(t *testing.T) {
	repo := newAssetTestRepo(t)
	ctx := context.Background()
	data := []byte{0x89, 0x50, 0x4e, 0x47, 0x01, 0x02}

	rec, err := repo.Put(ctx, "image/png", data, 1000)
	require.NoError(t, err)

	// id 는 내용의 SHA-256 hex 여야 한다(내용 주소화).
	sum := sha256.Sum256(data)
	assert.Equal(t, hex.EncodeToString(sum[:]), rec.ID)
	assert.Equal(t, "image/png", rec.MIME)
	assert.Equal(t, int64(len(data)), rec.Size)

	got, err := repo.Get(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, data, got.Data, "저장한 바이트가 그대로 나와야 한다")
	assert.Equal(t, int64(1000), got.CreatedAt)
}

func TestDashboardAssetSQLite_Put_IsIdempotentByContent(t *testing.T) {
	repo := newAssetTestRepo(t)
	ctx := context.Background()
	data := []byte("same-bytes")

	first, err := repo.Put(ctx, "image/png", data, 1000)
	require.NoError(t, err)
	// 같은 내용을 다른 시각에 다시 올려도 같은 id 이고 created_at 은 최초 값을 보존한다.
	second, err := repo.Put(ctx, "image/png", data, 9999)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.Equal(t, int64(1000), second.CreatedAt, "재업로드가 최초 저장 시각을 흔들지 않아야 한다")

	// 행이 하나만 있어야 한다(중복 저장 없음).
	var count int
	require.NoError(t, repo.db.QueryRow(`SELECT COUNT(*) FROM dashboard_assets`).Scan(&count))
	assert.Equal(t, 1, count)
}

func TestDashboardAssetSQLite_Put_DifferentContentDifferentID(t *testing.T) {
	repo := newAssetTestRepo(t)
	ctx := context.Background()

	a, err := repo.Put(ctx, "image/png", []byte("aaa"), 1)
	require.NoError(t, err)
	b, err := repo.Put(ctx, "image/png", []byte("bbb"), 1)
	require.NoError(t, err)
	assert.NotEqual(t, a.ID, b.ID)
}

func TestDashboardAssetSQLite_Get_NotFound(t *testing.T) {
	repo := newAssetTestRepo(t)
	_, err := repo.Get(context.Background(), "deadbeef")
	assert.ErrorIs(t, err, ErrDashboardAssetNotFound)
}

func TestDashboardAssetSQLite_Put_RejectsEmpty(t *testing.T) {
	repo := newAssetTestRepo(t)
	_, err := repo.Put(context.Background(), "image/png", nil, 1)
	assert.Error(t, err, "빈 자산은 저장하지 않는다")
}

func TestNewDashboardAssetSQLiteRepository_NilDB(t *testing.T) {
	_, err := NewDashboardAssetSQLiteRepository(context.Background(), nil)
	assert.Error(t, err)
}
