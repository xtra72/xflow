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

// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-4, M-5)
// 본 파일은 DashboardSQLiteRepository 의 Get/Put/Delete 동작 + If-Match 충돌 +
// scope/owner 부분 유니크 인덱스를 검증한다 (AC-5, AC-10, AC-17, UR-005, UB-003).

// setupDashboardRepo 는 테스트용 DashboardSQLiteRepository 와 *sql.DB 를 생성한다.
// 테스트 종료 시 자동으로 db.Close().
func setupDashboardRepo(t *testing.T) *DashboardSQLiteRepository {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "dashboard-test.db")
	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err, "OpenSQLiteDB 실패")
	t.Cleanup(func() { db.Close() })

	repo, err := NewDashboardSQLiteRepository(ctx, db)
	require.NoError(t, err, "NewDashboardSQLiteRepository 실패")
	return repo
}

// ---------------------------------------------------------------------------
// Get: NotFound
// ---------------------------------------------------------------------------

func TestDashboardSQLite_Get_NotFound(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// global / user 모두 빈 상태에서 ErrDashboardNotFound
	_, err := repo.Get(ctx, "global", "")
	assert.ErrorIs(t, err, ErrDashboardNotFound, "빈 저장소의 global 조회는 NotFound")

	_, err = repo.Get(ctx, "user", "alice")
	assert.ErrorIs(t, err, ErrDashboardNotFound, "빈 저장소의 user 조회는 NotFound")
}

// ---------------------------------------------------------------------------
// Put: 최초 생성 version=1, 두번째 PUT version=2 (server-assigned monotonic)
// AC-3, UR-005, UB-003 (서버가 version/updatedAt 부여)
// ---------------------------------------------------------------------------

func TestDashboardSQLite_Put_VersionMonotonic(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// 1) 최초 PUT (unconditional)
	beforeFirst := time.Now().UnixMilli()
	snap1, err := repo.Put(ctx, "global", "", []byte(`{"pages":[]}`), -1)
	require.NoError(t, err, "최초 PUT 성공")
	require.NotNil(t, snap1)
	assert.Equal(t, "global", snap1.Scope)
	assert.Equal(t, "", snap1.Owner)
	assert.EqualValues(t, 1, snap1.Version, "최초 version=1")
	assert.GreaterOrEqual(t, snap1.UpdatedAt, beforeFirst, "updatedAt 은 서버 부여, 과거 시각이 아니어야 한다")
	assert.JSONEq(t, `{"pages":[]}`, string(snap1.Payload))

	// 2) 두번째 PUT (unconditional, version 자동 증가)
	snap2, err := repo.Put(ctx, "global", "", []byte(`{"pages":[{"id":"p1"}]}`), -1)
	require.NoError(t, err, "두번째 PUT 성공")
	assert.EqualValues(t, 2, snap2.Version, "두번째 version=2")
	assert.GreaterOrEqual(t, snap2.UpdatedAt, snap1.UpdatedAt, "updatedAt 은 단조 증가")

	// 3) Get 으로 확인
	got, err := repo.Get(ctx, "global", "")
	require.NoError(t, err)
	assert.EqualValues(t, 2, got.Version)
	assert.JSONEq(t, `{"pages":[{"id":"p1"}]}`, string(got.Payload))
}

// TestDashboardSQLite_Put_ServerIgnoresClientVersion 는 UR-005/UB-003 검증.
// 클라이언트가 version 값을 보내도 무시되고 서버가 결정함 — 인터페이스 자체가
// expectedVersion 만 받으므로 페이로드 바이트는 server-assigned version 과 무관하다.
func TestDashboardSQLite_Put_ServerAssignsUpdatedAt(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// "고대" updatedAt 을 페이로드에 포함시키더라도 서버가 자체 결정한 시각을 사용한다
	// (DashboardSnapshot.UpdatedAt 는 페이로드 바이트가 아니라 별도 컬럼).
	payload := []byte(`{"updatedAt": 100, "pages": []}`)
	before := time.Now().UnixMilli()
	snap, err := repo.Put(ctx, "user", "alice", payload, -1)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, snap.UpdatedAt, before, "서버 부여 updatedAt 은 현재 시각 이상")
}

// ---------------------------------------------------------------------------
// Put: If-Match (expectedVersion) 검증 — UR-005, AC-10
// ---------------------------------------------------------------------------

func TestDashboardSQLite_Put_IfMatch_Success(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// 최초 PUT
	snap1, err := repo.Put(ctx, "user", "alice", []byte(`{"v":1}`), -1)
	require.NoError(t, err)
	assert.EqualValues(t, 1, snap1.Version)

	// If-Match: 1 (정확 일치) → 성공, version=2
	snap2, err := repo.Put(ctx, "user", "alice", []byte(`{"v":2}`), 1)
	require.NoError(t, err)
	assert.EqualValues(t, 2, snap2.Version)
}

func TestDashboardSQLite_Put_IfMatch_Mismatch(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// 현재 version=5 로 만든다
	for i := 0; i < 5; i++ {
		_, err := repo.Put(ctx, "user", "alice", []byte(`{"v":"x"}`), -1)
		require.NoError(t, err)
	}

	got, err := repo.Get(ctx, "user", "alice")
	require.NoError(t, err)
	assert.EqualValues(t, 5, got.Version)

	// If-Match: 3 (불일치) → ErrDashboardVersionMismatch
	_, err = repo.Put(ctx, "user", "alice", []byte(`{"v":"new"}`), 3)
	require.ErrorIs(t, err, ErrDashboardVersionMismatch, "version 불일치 시 sentinel 반환")

	// 서버 상태가 변경되지 않았는지 검증
	got2, err := repo.Get(ctx, "user", "alice")
	require.NoError(t, err)
	assert.EqualValues(t, 5, got2.Version, "충돌 시 서버 상태 유지")
	assert.JSONEq(t, `{"v":"x"}`, string(got2.Payload), "충돌 시 payload 도 유지")
}

func TestDashboardSQLite_Put_IfMatch_NewRow(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// snapshot 없을 때 expectedVersion=0 (= old.version) 으로 PUT → 새 row, version=1
	snap, err := repo.Put(ctx, "user", "bob", []byte(`{}`), 0)
	require.NoError(t, err, "신규 row 의 expectedVersion=0 매칭")
	assert.EqualValues(t, 1, snap.Version)

	// snapshot 없을 때 expectedVersion=5 (불일치) → ErrDashboardVersionMismatch
	_, err = repo.Put(ctx, "user", "carol", []byte(`{}`), 5)
	require.ErrorIs(t, err, ErrDashboardVersionMismatch, "신규 row 의 비-0 expectedVersion 은 불일치")
}

// ---------------------------------------------------------------------------
// Delete: AC-17
// ---------------------------------------------------------------------------

func TestDashboardSQLite_Delete(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// alice 의 snapshot 생성
	_, err := repo.Put(ctx, "user", "alice", []byte(`{}`), -1)
	require.NoError(t, err)

	// Delete 성공
	err = repo.Delete(ctx, "user", "alice")
	require.NoError(t, err)

	// 이후 Get 은 NotFound
	_, err = repo.Get(ctx, "user", "alice")
	assert.ErrorIs(t, err, ErrDashboardNotFound)

	// 다시 Delete 해도 nil (멱등)
	err = repo.Delete(ctx, "user", "alice")
	assert.NoError(t, err, "두 번째 Delete 는 멱등하게 nil")
}

// ---------------------------------------------------------------------------
// Partial unique index: global+NULL, user+alice, user+bob 가 공존 + cross-user 격리
// AC-5
// ---------------------------------------------------------------------------

func TestDashboardSQLite_PartialUniqueIndex_AllowsCoexistence(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	// 세 row 가 공존
	_, err := repo.Put(ctx, "global", "", []byte(`{"shared":true}`), -1)
	require.NoError(t, err, "global+NULL 생성")

	_, err = repo.Put(ctx, "user", "alice", []byte(`{"alice":true}`), -1)
	require.NoError(t, err, "user+alice 생성")

	_, err = repo.Put(ctx, "user", "bob", []byte(`{"bob":true}`), -1)
	require.NoError(t, err, "user+bob 생성")

	// 각자 정확한 row 가 조회됨
	g, err := repo.Get(ctx, "global", "")
	require.NoError(t, err)
	assert.JSONEq(t, `{"shared":true}`, string(g.Payload))

	a, err := repo.Get(ctx, "user", "alice")
	require.NoError(t, err)
	assert.JSONEq(t, `{"alice":true}`, string(a.Payload))

	b, err := repo.Get(ctx, "user", "bob")
	require.NoError(t, err)
	assert.JSONEq(t, `{"bob":true}`, string(b.Payload))

	// alice 의 PUT 이 bob 에 영향 없음
	_, err = repo.Put(ctx, "user", "alice", []byte(`{"alice":"updated"}`), -1)
	require.NoError(t, err)

	bAfter, err := repo.Get(ctx, "user", "bob")
	require.NoError(t, err)
	assert.JSONEq(t, `{"bob":true}`, string(bAfter.Payload), "bob 의 row 는 영향 없음 (cross-user 격리)")
}

// TestDashboardSQLite_GlobalOwnerStoredAsNULL 는 scope=global 일 때 DB 상 owner 컬럼이
// NULL 로 저장되는지 확인한다 (부분 유니크 인덱스 호환).
func TestDashboardSQLite_GlobalOwnerStoredAsNULL(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	_, err := repo.Put(ctx, "global", "", []byte(`{}`), -1)
	require.NoError(t, err)

	var ownerNull bool
	err = repo.db.QueryRowContext(ctx,
		`SELECT owner IS NULL FROM `+repo.table+` WHERE scope='global'`,
	).Scan(&ownerNull)
	require.NoError(t, err)
	assert.True(t, ownerNull, "global scope 의 owner 컬럼은 NULL 로 저장")
}

// TestDashboardSQLite_Delete_GlobalDoesNotAffectUser 는 AC-17 의 cross-scope
// 격리 (admin 의 shared DELETE 가 user 의 row 에 영향 주지 않음) 를 검증한다.
func TestDashboardSQLite_Delete_GlobalDoesNotAffectUser(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	_, err := repo.Put(ctx, "global", "", []byte(`{}`), -1)
	require.NoError(t, err)
	_, err = repo.Put(ctx, "user", "alice", []byte(`{"alice":true}`), -1)
	require.NoError(t, err)

	// shared 삭제
	err = repo.Delete(ctx, "global", "")
	require.NoError(t, err)

	// alice 는 영향 없음
	a, err := repo.Get(ctx, "user", "alice")
	require.NoError(t, err)
	assert.JSONEq(t, `{"alice":true}`, string(a.Payload))
}

// TestDashboardSQLite_New_NilDB 는 nil *sql.DB 입력 방어 검증.
func TestDashboardSQLite_New_NilDB(t *testing.T) {
	_, err := NewDashboardSQLiteRepository(context.Background(), nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil")
}

// TestDashboardSQLite_PutConcurrent 는 동일 (scope, owner) 에 대한 동시 PUT 이
// 트랜잭션으로 직렬화되어 race condition 없이 일관된 상태를 유지하는지 검증한다.
//
// SQLite WAL 모드에서 동시 writer 중 일부는 "database is locked" 으로 실패할 수
// 있으나, 성공한 PUT 수와 최종 version 이 일치해야 한다 (각 성공이 정확히 +1).
func TestDashboardSQLite_PutConcurrent(t *testing.T) {
	repo := setupDashboardRepo(t)
	ctx := context.Background()

	const N = 10
	var (
		wg      sync.WaitGroup
		mu      sync.Mutex
		success int
	)
	wg.Add(N)
	for i := 0; i < N; i++ {
		go func() {
			defer wg.Done()
			if _, err := repo.Put(ctx, "user", "concurrent", []byte(`{}`), -1); err == nil {
				mu.Lock()
				success++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	got, err := repo.Get(ctx, "user", "concurrent")
	require.NoError(t, err)
	// 최소 1개는 성공해야 하며, 성공한 PUT 수와 최종 version 이 정확히 일치해야 race-free.
	assert.GreaterOrEqual(t, success, 1, "최소 1회 PUT 은 성공해야 한다")
	assert.EqualValues(t, success, got.Version,
		"성공한 PUT 수와 최종 version 이 일치 (각 성공이 정확히 +1 → race-free)")
}
