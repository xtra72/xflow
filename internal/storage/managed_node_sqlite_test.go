// managed_node_sqlite_test.go 는 ManagedNodeRepository 의 sqlite 구현을 검증한다
// (@SPEC:SPEC-REMOTE-001 M2, spec §5.4 managed_nodes). 임시 파일 sqlite 를 사용한다.
package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestManagedNodeRepo 는 임시 디렉토리에 sqlite 기반 ManagedNodeRepository 를
// 생성한다. t.Cleanup 으로 Close 를 등록한다.
func newTestManagedNodeRepo(t *testing.T) ManagedNodeRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "managed.db")
	repo, err := NewManagedNodeSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// TestManagedNode_UpsertAndGet 는 upsert 후 get 으로 동일 행을 조회하는지 검증한다.
func TestManagedNode_UpsertAndGet(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	node := ManagedNode{
		InstanceID: "node-1",
		Hostname:   "host-1",
		Version:    "1.0.0",
		Status:     "pending",
		Online:     true,
		LastSeen:   1000,
	}
	require.NoError(t, repo.Upsert(ctx, node))

	got, err := repo.Get(ctx, "node-1")
	require.NoError(t, err)
	assert.Equal(t, "node-1", got.InstanceID)
	assert.Equal(t, "host-1", got.Hostname)
	assert.Equal(t, "1.0.0", got.Version)
	assert.Equal(t, "pending", got.Status)
	assert.True(t, got.Online)
	assert.Equal(t, int64(1000), got.LastSeen)
	assert.Greater(t, got.CreatedAt, int64(0))
	assert.Greater(t, got.UpdatedAt, int64(0))
}

// TestManagedNode_GetNotFound 는 미존재 노드 조회 시 ErrManagedNodeNotFound 를
// 반환하는지 검증한다.
func TestManagedNode_GetNotFound(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	_, err := repo.Get(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_UpsertPreservesCreatedAt 는 재 upsert 시 created_at 이 보존되고
// 메타가 갱신되는지 검증한다(last-known 보존, REQ-E06 토대).
func TestManagedNode_UpsertPreservesCreatedAt(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Hostname: "old", Status: "pending"}))
	first, err := repo.Get(ctx, "n")
	require.NoError(t, err)

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Hostname: "new", Status: "pending"}))
	second, err := repo.Get(ctx, "n")
	require.NoError(t, err)

	assert.Equal(t, "new", second.Hostname, "hostname 은 갱신되어야 함")
	assert.Equal(t, first.CreatedAt, second.CreatedAt, "created_at 은 보존되어야 함")
}

// TestManagedNode_UpdateStatus 는 상태 전이를 검증한다(pending→approved→revoked).
func TestManagedNode_UpdateStatus(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "pending"}))

	require.NoError(t, repo.UpdateStatus(ctx, "n", "approved"))
	got, _ := repo.Get(ctx, "n")
	assert.Equal(t, "approved", got.Status)

	require.NoError(t, repo.UpdateStatus(ctx, "n", "revoked"))
	got, _ = repo.Get(ctx, "n")
	assert.Equal(t, "revoked", got.Status)
}

// TestManagedNode_UpdateStatusNotFound 는 미존재 노드 상태 갱신 시 에러를 반환하는지
// 검증한다.
func TestManagedNode_UpdateStatusNotFound(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	err := repo.UpdateStatus(context.Background(), "missing", "approved")
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_SetToken 는 토큰 식별자 저장을 검증한다(폐기 매핑용, REQ-C04/C07).
func TestManagedNode_SetToken(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved"}))
	require.NoError(t, repo.SetToken(ctx, "n", "token-id-xyz"))

	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.Equal(t, "token-id-xyz", got.TokenID)
}

// TestManagedNode_SetTokenNotFound 는 미존재 노드 토큰 설정 시 에러를 반환하는지
// 검증한다.
func TestManagedNode_SetTokenNotFound(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	err := repo.SetToken(context.Background(), "missing", "tid")
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_SetOnline 는 online/last_seen 갱신을 검증한다(REQ-B05/E06 토대).
func TestManagedNode_SetOnline(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "approved", Online: true, LastSeen: 100}))

	require.NoError(t, repo.SetOnline(ctx, "n", false, 200))
	got, err := repo.Get(ctx, "n")
	require.NoError(t, err)
	assert.False(t, got.Online)
	assert.Equal(t, int64(200), got.LastSeen)
}

// TestManagedNode_List 는 전체 목록 반환을 검증한다.
func TestManagedNode_List(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "a", Status: "approved"}))
	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "b", Status: "pending"}))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)

	ids := map[string]string{}
	for _, n := range list {
		ids[n.InstanceID] = n.Status
	}
	assert.Equal(t, "approved", ids["a"])
	assert.Equal(t, "pending", ids["b"])
}

// TestManagedNode_Delete 는 삭제를 검증한다.
func TestManagedNode_Delete(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "n", Status: "pending"}))
	require.NoError(t, repo.Delete(ctx, "n"))

	_, err := repo.Get(ctx, "n")
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_DeleteNotFound 는 미존재 노드 삭제 시 에러를 반환하는지 검증한다.
func TestManagedNode_DeleteNotFound(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	err := repo.Delete(context.Background(), "missing")
	assert.ErrorIs(t, err, ErrManagedNodeNotFound)
}

// TestManagedNode_FactoryWiring 는 storage 팩토리에서 ManagedNodeRepository 를
// 생성할 수 있는지 검증한다(sqlite).
func TestManagedNode_FactoryWiring(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "factory.db")
	repo, err := NewManagedNodeRepository(context.Background(), "sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	require.NoError(t, repo.Upsert(context.Background(), ManagedNode{InstanceID: "f", Status: "pending"}))
	got, err := repo.Get(context.Background(), "f")
	require.NoError(t, err)
	assert.Equal(t, "f", got.InstanceID)
}

// TestManagedNode_AfterClose 는 Close 후 작업이 에러를 반환하는지 검증한다(에러 경로).
func TestManagedNode_AfterClose(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "closed.db")
	repo, err := NewManagedNodeSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	ctx := context.Background()
	assert.Error(t, repo.Upsert(ctx, ManagedNode{InstanceID: "x", Status: "pending"}))
	_, getErr := repo.Get(ctx, "x")
	assert.Error(t, getErr)
	_, listErr := repo.List(ctx)
	assert.Error(t, listErr)
	assert.Error(t, repo.UpdateStatus(ctx, "x", "approved"))
	assert.Error(t, repo.SetToken(ctx, "x", "t"))
	assert.Error(t, repo.SetOnline(ctx, "x", true, 1))
	assert.Error(t, repo.Delete(ctx, "x"))
}

// TestManagedNode_FactoryUnknownTypeFallsBack 는 알 수 없는 storage type 도 sqlite
// 로 폴백되는지 검증한다(서버 캐시는 항상 sqlite — §5.4).
func TestManagedNode_FactoryUnknownTypeFallsBack(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fallback.db")
	repo, err := NewManagedNodeRepository(context.Background(), "postgres", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	require.NoError(t, repo.Upsert(context.Background(), ManagedNode{InstanceID: "f", Status: "pending"}))
	got, err := repo.Get(context.Background(), "f")
	require.NoError(t, err)
	assert.Equal(t, "f", got.InstanceID)
}

// TestManagedNode_InvalidStatusRejected 는 CHECK 제약이 잘못된 status 를 거부하는지
// 검증한다(상태 머신 무결성).
func TestManagedNode_InvalidStatusRejected(t *testing.T) {
	repo := newTestManagedNodeRepo(t)
	err := repo.Upsert(context.Background(), ManagedNode{InstanceID: "n", Status: "bogus"})
	assert.Error(t, err, "CHECK 제약은 잘못된 status 를 거부해야 함")
}

// TestManagedNode_MigrationOnExistingDB 는 기존 xflow.db(다른 테이블 보유)에
// managed_nodes 스키마를 멱등하게 추가하는지 검증한다(기존 DB 마이그레이션 안전성).
func TestManagedNode_MigrationOnExistingDB(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "existing.db")

	// 먼저 flows/users 스키마를 가진 DB 를 만든다.
	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	// 동일 경로로 managed node repo 를 열어도 멱등하게 동작해야 한다.
	repo, err := NewManagedNodeSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })

	require.NoError(t, repo.Upsert(ctx, ManagedNode{InstanceID: "m", Status: "pending"}))
	got, err := repo.Get(ctx, "m")
	require.NoError(t, err)
	assert.Equal(t, "m", got.InstanceID)
}
