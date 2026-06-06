// enrollment_token_sqlite_test.go 는 EnrollmentTokenRepository 의 SQLite 구현을
// 검증한다(@SPEC:SPEC-REMOTE-001 v1.1 그룹 H, REQ-REMOTE-H03~H07).
//
// 핵심 보안 불변식:
//   - token_hash 만 저장하고 원본 토큰은 저장하지 않는다(REQ-H06).
//   - List 는 메타데이터만 반환하고 hash 를 노출하지 않는다(REQ-H06).
//   - IncrementUses 는 max_uses 조건부 원자 증가로 동시 register 경합을 안전화한다(REQ-H07).
package storage

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestEnrollmentRepo(t *testing.T) *EnrollmentTokenSQLiteRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "enroll.db")
	repo, err := NewEnrollmentTokenSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// TestEnrollmentToken_CreateAndGetByHash 는 생성 후 hash 로 조회되는지 검증한다(REQ-H03/H05).
func TestEnrollmentToken_CreateAndGetByHash(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()

	rec := EnrollmentToken{
		ID:        "et-1",
		TokenHash: "hash-abc",
		Label:     "lab",
		CreatedAt: time.Now().UnixMilli(),
		MaxUses:   IntPtr(3),
	}
	require.NoError(t, repo.Create(ctx, rec))

	got, err := repo.GetByHash(ctx, "hash-abc")
	require.NoError(t, err)
	assert.Equal(t, "et-1", got.ID)
	assert.Equal(t, "lab", got.Label)
	require.NotNil(t, got.MaxUses)
	assert.Equal(t, 3, *got.MaxUses)
	assert.Equal(t, 0, got.Uses)
	assert.False(t, got.Revoked)
}

// TestEnrollmentToken_GetByHashNotFound 는 미존재 hash 가 센티널을 반환하는지 검증한다.
func TestEnrollmentToken_GetByHashNotFound(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	_, err := repo.GetByHash(context.Background(), "nope")
	assert.ErrorIs(t, err, ErrEnrollmentTokenNotFound)
}

// TestEnrollmentToken_ListHidesSecret 는 List 가 메타만 반환하고 token_hash 를
// 노출하지 않는지 검증한다(REQ-H06).
func TestEnrollmentToken_ListHidesSecret(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "a", TokenHash: "secret-hash-a", Label: "A", CreatedAt: 1}))
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "b", TokenHash: "secret-hash-b", Label: "B", CreatedAt: 2}))

	list, err := repo.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 2)
	for _, e := range list {
		assert.Empty(t, e.TokenHash, "List 는 token_hash 를 노출하면 안 된다(REQ-H06)")
	}
}

// TestEnrollmentToken_Revoke 는 revoke 후 revoked=true 가 되는지 검증한다(REQ-H04).
func TestEnrollmentToken_Revoke(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "r1", TokenHash: "h", CreatedAt: 1}))

	require.NoError(t, repo.Revoke(ctx, "r1"))
	got, err := repo.GetByHash(ctx, "h")
	require.NoError(t, err)
	assert.True(t, got.Revoked)

	// 미존재 revoke 는 센티널.
	assert.ErrorIs(t, repo.Revoke(ctx, "missing"), ErrEnrollmentTokenNotFound)
}

// TestEnrollmentToken_Delete 는 삭제 동작과 미존재 센티널을 검증한다.
func TestEnrollmentToken_Delete(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "d1", TokenHash: "h", CreatedAt: 1}))

	require.NoError(t, repo.Delete(ctx, "d1"))
	_, err := repo.GetByHash(ctx, "h")
	assert.ErrorIs(t, err, ErrEnrollmentTokenNotFound)
	assert.ErrorIs(t, repo.Delete(ctx, "d1"), ErrEnrollmentTokenNotFound)
}

// TestEnrollmentToken_IncrementUses 는 조건부 증가가 max_uses 를 넘지 않는지 검증한다(REQ-H07).
func TestEnrollmentToken_IncrementUses(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "u1", TokenHash: "h", CreatedAt: 1, MaxUses: IntPtr(2)}))

	require.NoError(t, repo.IncrementUses(ctx, "u1")) // 1/2
	require.NoError(t, repo.IncrementUses(ctx, "u1")) // 2/2
	// 3 번째는 max_uses 초과 → ErrEnrollmentTokenExhausted.
	assert.ErrorIs(t, repo.IncrementUses(ctx, "u1"), ErrEnrollmentTokenExhausted)

	got, err := repo.GetByHash(ctx, "h")
	require.NoError(t, err)
	assert.Equal(t, 2, got.Uses)
}

// TestEnrollmentToken_IncrementUsesUnlimited 는 max_uses=nil 일 때 무제한 증가를 검증한다.
func TestEnrollmentToken_IncrementUsesUnlimited(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "inf", TokenHash: "h", CreatedAt: 1, MaxUses: nil}))

	for i := 0; i < 5; i++ {
		require.NoError(t, repo.IncrementUses(ctx, "inf"))
	}
	got, _ := repo.GetByHash(ctx, "h")
	assert.Equal(t, 5, got.Uses)
}

// TestEnrollmentToken_IncrementUsesConcurrent 는 동시 증가에서 max_uses 가 정확히
// 강제되는지(over-issue 방지) 검증한다(REQ-H07 — uses 증가 경합).
func TestEnrollmentToken_IncrementUsesConcurrent(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	const maxUses = 10
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "c1", TokenHash: "h", CreatedAt: 1, MaxUses: IntPtr(maxUses)}))

	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
	)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := repo.IncrementUses(ctx, "c1"); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	assert.Equal(t, maxUses, success, "성공 증가 횟수는 정확히 max_uses 여야 한다(over-issue 금지)")
	got, _ := repo.GetByHash(ctx, "h")
	assert.Equal(t, maxUses, got.Uses)
}

// TestEnrollmentToken_FactoryWiring 는 팩토리가 sqlite 구현을 반환하는지 검증한다.
func TestEnrollmentToken_FactoryWiring(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "enroll.db")
	for _, typ := range []string{"sqlite", "file", "other"} {
		repo, err := NewEnrollmentTokenRepository(context.Background(), typ, dbPath)
		require.NoError(t, err)
		_, ok := repo.(*EnrollmentTokenSQLiteRepository)
		assert.True(t, ok)
		_ = repo.Close()
	}
}

// TestEnrollmentToken_IncrementUsesNotFound 는 미존재 토큰 증가가 NotFound 를 반환하는지
// 검증한다(소진과 구분 — REQ-H07).
func TestEnrollmentToken_IncrementUsesNotFound(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	assert.ErrorIs(t, repo.IncrementUses(context.Background(), "ghost"), ErrEnrollmentTokenNotFound)
}

// TestEnrollmentToken_Int64Ptr 는 nullable 헬퍼와 expires_at 왕복을 검증한다.
func TestEnrollmentToken_Int64Ptr(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	exp := time.Now().Add(time.Hour).UnixMilli()
	require.NoError(t, repo.Create(ctx, EnrollmentToken{
		ID: "e1", TokenHash: "h", CreatedAt: 1, ExpiresAt: Int64Ptr(exp),
	}))
	got, err := repo.GetByHash(ctx, "h")
	require.NoError(t, err)
	require.NotNil(t, got.ExpiresAt)
	assert.Equal(t, exp, *got.ExpiresAt)
	assert.Nil(t, got.MaxUses, "max_uses 미설정은 nil 이어야 한다")
}

// TestEnrollmentToken_CreateRevoked 는 revoked=true 로 생성된 토큰의 왕복을 검증한다.
func TestEnrollmentToken_CreateRevoked(t *testing.T) {
	repo := newTestEnrollmentRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Create(ctx, EnrollmentToken{ID: "rv", TokenHash: "h", CreatedAt: 1, Revoked: true}))
	got, err := repo.GetByHash(ctx, "h")
	require.NoError(t, err)
	assert.True(t, got.Revoked)
}
