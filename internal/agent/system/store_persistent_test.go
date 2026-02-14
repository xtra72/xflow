package system

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// 컴파일 타임 인터페이스 체크
// ---------------------------------------------------------------------------

// PersistentStore 가 Store 인터페이스를 구현하는지 컴파일 타임에 확인한다.
var _ Store = (*PersistentStore)(nil)

// ---------------------------------------------------------------------------
// mockRepository - 테스트용 StoreRepository 구현
// ---------------------------------------------------------------------------

// mockRepository 는 테스트용 StoreRepository 구현이다.
type mockRepository struct {
	entries map[string]*StoreEntry
	mu      sync.Mutex

	// 테스트 훅: non-nil이면 해당 메서드에서 이 에러를 반환한다
	setErr    error
	getErr    error
	deleteErr error
	clearErr  error
	listErr   error
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		entries: make(map[string]*StoreEntry),
	}
}

func (m *mockRepository) GetEntry(_ context.Context, key string) (*StoreEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return nil, m.getErr
	}

	entry, ok := m.entries[key]
	if !ok {
		return nil, ErrKeyNotFound
	}
	// 복사본 반환
	cp := *entry
	return &cp, nil
}

func (m *mockRepository) SetEntry(_ context.Context, key string, entry *StoreEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.setErr != nil {
		return m.setErr
	}

	cp := *entry
	m.entries[key] = &cp
	return nil
}

func (m *mockRepository) DeleteEntry(_ context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.deleteErr != nil {
		return m.deleteErr
	}

	delete(m.entries, key)
	return nil
}

func (m *mockRepository) ListKeys(_ context.Context, _ string) ([]string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.listErr != nil {
		return nil, m.listErr
	}

	keys := make([]string, 0, len(m.entries))
	for k := range m.entries {
		keys = append(keys, k)
	}
	return keys, nil
}

func (m *mockRepository) DeleteExpired(_ context.Context, before time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for k, entry := range m.entries {
		if !entry.ExpiresAt.IsZero() && entry.ExpiresAt.Before(before) {
			delete(m.entries, k)
			count++
		}
	}
	return count, nil
}

func (m *mockRepository) ClearNamespace(_ context.Context, namespace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.clearErr != nil {
		return m.clearErr
	}

	if namespace == "" {
		// 빈 문자열이면 모든 엔트리 삭제
		m.entries = make(map[string]*StoreEntry)
		return nil
	}

	for k, entry := range m.entries {
		if entry.Namespace == namespace {
			delete(m.entries, k)
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestPersistentStore 는 테스트용 PersistentStore를 생성한다.
func newTestPersistentStore() (*PersistentStore, *mockRepository) {
	repo := newMockRepository()
	store := NewPersistentStore(repo, MaxKeyLength)
	return store, repo
}

// ---------------------------------------------------------------------------
// 기본 CRUD 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_SetAndGet 은 기본 Set/Get 동작을 검증한다.
func TestPersistentStore_SetAndGet(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.Set(ctx, "key1", "value1")
	require.NoError(t, err)

	entry, err := store.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", entry.Value)
	assert.False(t, entry.CreatedAt.IsZero(), "CreatedAt 은 설정되어야 한다")
	assert.False(t, entry.UpdatedAt.IsZero(), "UpdatedAt 은 설정되어야 한다")
	assert.True(t, entry.ExpiresAt.IsZero(), "TTL 없이 Set하면 ExpiresAt은 zero value여야 한다")
}

// TestPersistentStore_GetNotFound 는 존재하지 않는 키에 대해 ErrKeyNotFound를 반환하는지 검증한다.
func TestPersistentStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	_, err := store.Get(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestPersistentStore_Delete 는 키 삭제를 검증한다.
func TestPersistentStore_Delete(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.Set(ctx, "del-key", "value")
	require.NoError(t, err)

	err = store.Delete(ctx, "del-key")
	require.NoError(t, err)

	_, err = store.Get(ctx, "del-key")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestPersistentStore_Has 는 키 존재 확인을 검증한다.
func TestPersistentStore_Has(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	has, err := store.Has(ctx, "no-key")
	require.NoError(t, err)
	assert.False(t, has)

	err = store.Set(ctx, "yes-key", "value")
	require.NoError(t, err)

	has, err = store.Has(ctx, "yes-key")
	require.NoError(t, err)
	assert.True(t, has)
}

// ---------------------------------------------------------------------------
// 캐시 미스 → 리포지토리 로드 → 캐시 적재 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_CacheMissLoadsFromRepo 는 캐시 미스 시 리포지토리에서 로드하여 캐시에 적재하는지 검증한다.
func TestPersistentStore_CacheMissLoadsFromRepo(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	// 리포지토리에 직접 엔트리를 넣는다 (캐시 우회)
	now := time.Now()
	jsonVal, _ := json.Marshal("repo-value")
	repo.mu.Lock()
	repo.entries["repo-key"] = &StoreEntry{
		Value:     string(jsonVal),
		CreatedAt: now,
		UpdatedAt: now,
	}
	repo.mu.Unlock()

	// Get 호출 시 캐시 미스 → 리포지토리 조회 → 캐시 적재
	entry, err := store.Get(ctx, "repo-key")
	require.NoError(t, err)
	assert.Equal(t, "repo-value", entry.Value)

	// 두 번째 Get은 캐시에서 조회되어야 한다 (리포지토리 에러를 설정해도 성공해야 한다)
	repo.mu.Lock()
	repo.getErr = assert.AnError
	repo.mu.Unlock()

	entry2, err := store.Get(ctx, "repo-key")
	require.NoError(t, err)
	assert.Equal(t, "repo-value", entry2.Value)
}

// ---------------------------------------------------------------------------
// Set 실패 시 캐시 롤백 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_SetRepoFailureRollback 은 리포지토리 Set 실패 시 캐시가 이전 상태로 롤백되는지 검증한다.
func TestPersistentStore_SetRepoFailureRollback(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	// 먼저 정상적으로 값 저장
	err := store.Set(ctx, "key", "original")
	require.NoError(t, err)

	// 리포지토리 에러 설정
	repo.mu.Lock()
	repo.setErr = assert.AnError
	repo.mu.Unlock()

	// Set 실패 시 에러 반환
	err = store.Set(ctx, "key", "new-value")
	assert.Error(t, err)

	// 캐시 롤백 확인 - 원래 값이 유지되어야 한다
	repo.mu.Lock()
	repo.setErr = nil
	repo.mu.Unlock()

	entry, err := store.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "original", entry.Value, "리포지토리 Set 실패 시 캐시가 원래 값으로 롤백되어야 한다")
}

// TestPersistentStore_SetRepoFailureRollbackNewKey 는 새 키에 대한 리포지토리 실패 시 캐시에서 키가 삭제되는지 검증한다.
func TestPersistentStore_SetRepoFailureRollbackNewKey(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	// 리포지토리 에러 설정
	repo.mu.Lock()
	repo.setErr = assert.AnError
	repo.mu.Unlock()

	// 새 키에 대한 Set 실패
	err := store.Set(ctx, "new-key", "value")
	assert.Error(t, err)

	// 캐시에서 키가 없어야 한다
	repo.mu.Lock()
	repo.setErr = nil
	repo.mu.Unlock()

	has, err := store.Has(ctx, "new-key")
	require.NoError(t, err)
	assert.False(t, has, "리포지토리 Set 실패 시 새 키는 캐시에서 삭제되어야 한다")
}

// ---------------------------------------------------------------------------
// JSON 직렬화 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_JSONSerializationComplex 는 복잡한 타입의 JSON 직렬화를 검증한다.
func TestPersistentStore_JSONSerializationComplex(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{"맵", "map-key", map[string]any{"nested": "value", "count": float64(42)}},
		{"구조체", "struct-key", map[string]any{"Name": "test", "Age": float64(30)}},
		{"슬라이스", "slice-key", []any{"a", "b", "c"}},
		{"정수", "int-key", float64(42)},
		{"문자열", "string-key", "hello"},
		{"불리언", "bool-key", true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Set(ctx, tc.key, tc.value)
			require.NoError(t, err)

			// 리포지토리에 JSON 형태로 저장되었는지 확인
			repo.mu.Lock()
			repoEntry := repo.entries[tc.key]
			repo.mu.Unlock()

			require.NotNil(t, repoEntry, "리포지토리에 엔트리가 저장되어야 한다")

			// JSON 문자열로 저장되어야 한다
			jsonStr, ok := repoEntry.Value.(string)
			require.True(t, ok, "리포지토리 값은 JSON 문자열이어야 한다")

			var decoded any
			err = json.Unmarshal([]byte(jsonStr), &decoded)
			require.NoError(t, err, "리포지토리 값은 유효한 JSON이어야 한다")
		})
	}
}

// TestPersistentStore_ErrNotSerializableFunc 는 함수 타입 저장 시 ErrNotSerializable을 반환하는지 검증한다.
func TestPersistentStore_ErrNotSerializableFunc(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.Set(ctx, "func-key", func() {})
	assert.ErrorIs(t, err, ErrNotSerializable)
}

// TestPersistentStore_ErrNotSerializableChannel 은 채널 타입 저장 시 ErrNotSerializable을 반환하는지 검증한다.
func TestPersistentStore_ErrNotSerializableChannel(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.Set(ctx, "chan-key", make(chan int))
	assert.ErrorIs(t, err, ErrNotSerializable)
}

// ---------------------------------------------------------------------------
// LoadFromRepo 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_LoadFromRepo 는 리포지토리에서 만료되지 않은 엔트리를 캐시에 로드하는지 검증한다.
func TestPersistentStore_LoadFromRepo(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	now := time.Now()

	// 만료되지 않은 엔트리
	aliveJSON, _ := json.Marshal("alive-value")
	repo.mu.Lock()
	repo.entries["alive"] = &StoreEntry{
		Value:     string(aliveJSON),
		CreatedAt: now,
		UpdatedAt: now,
	}

	// 만료된 엔트리
	expiredJSON, _ := json.Marshal("expired-value")
	repo.entries["expired"] = &StoreEntry{
		Value:     string(expiredJSON),
		CreatedAt: now.Add(-2 * time.Hour),
		UpdatedAt: now.Add(-2 * time.Hour),
		ExpiresAt: now.Add(-1 * time.Hour), // 1시간 전 만료
	}

	// TTL이 있지만 아직 만료되지 않은 엔트리
	futureJSON, _ := json.Marshal("future-value")
	repo.entries["future"] = &StoreEntry{
		Value:     string(futureJSON),
		CreatedAt: now,
		UpdatedAt: now,
		ExpiresAt: now.Add(1 * time.Hour), // 1시간 후 만료
	}
	repo.mu.Unlock()

	// LoadFromRepo 호출
	err := store.LoadFromRepo(ctx)
	require.NoError(t, err)

	// 만료되지 않은 엔트리는 캐시에 있어야 한다
	entry, err := store.Get(ctx, "alive")
	require.NoError(t, err)
	assert.Equal(t, "alive-value", entry.Value)

	// 만료된 엔트리는 캐시에 없어야 한다
	_, err = store.Get(ctx, "expired")
	assert.ErrorIs(t, err, ErrKeyNotFound)

	// TTL이 남은 엔트리는 캐시에 있어야 한다
	entry, err = store.Get(ctx, "future")
	require.NoError(t, err)
	assert.Equal(t, "future-value", entry.Value)
}

// ---------------------------------------------------------------------------
// Keys 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_KeysCombinesCacheAndRepo 는 캐시와 리포지토리 키를 합치고 중복을 제거하는지 검증한다.
func TestPersistentStore_KeysCombinesCacheAndRepo(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	// 캐시에만 존재하는 키
	err := store.Set(ctx, "cache-only", "v1")
	require.NoError(t, err)

	// 리포지토리에만 존재하는 키 (캐시 우회)
	repoJSON, _ := json.Marshal("repo-only-value")
	repo.mu.Lock()
	repo.entries["repo-only"] = &StoreEntry{
		Value:     string(repoJSON),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	repo.mu.Unlock()

	// 양쪽 모두에 존재하는 키 (Set으로 생성되면 양쪽에 있음)
	// cache-only는 이미 양쪽에 있으므로 repo-only만 추가된 상태

	keys, err := store.Keys(ctx, "*")
	require.NoError(t, err)
	assert.Contains(t, keys, "cache-only")
	assert.Contains(t, keys, "repo-only")
}

// ---------------------------------------------------------------------------
// Clear 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_Clear 는 캐시와 리포지토리 모두 클리어하는지 검증한다.
func TestPersistentStore_Clear(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	err := store.Set(ctx, "a", "1")
	require.NoError(t, err)
	err = store.Set(ctx, "b", "2")
	require.NoError(t, err)

	err = store.Clear(ctx)
	require.NoError(t, err)

	// 캐시에서 키가 없어야 한다
	keys, err := store.Keys(ctx, "*")
	require.NoError(t, err)
	assert.Empty(t, keys)

	// 리포지토리에서도 키가 없어야 한다
	repo.mu.Lock()
	assert.Empty(t, repo.entries)
	repo.mu.Unlock()
}

// ---------------------------------------------------------------------------
// nil 값 거부 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_NilValueRejection 은 nil 값 저장 시 ErrNilValue를 반환하는지 검증한다.
func TestPersistentStore_NilValueRejection(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.Set(ctx, "key", nil)
	assert.ErrorIs(t, err, ErrNilValue)
}

// ---------------------------------------------------------------------------
// 키 길이 검증 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_KeyLengthValidation 은 키 길이 제한을 검증한다.
func TestPersistentStore_KeyLengthValidation(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	// 최대 길이 키는 허용
	exactKey := strings.Repeat("x", MaxKeyLength)
	err := store.Set(ctx, exactKey, "value")
	assert.NoError(t, err)

	// 최대 길이 초과 키는 거부
	longKey := strings.Repeat("x", MaxKeyLength+1)
	err = store.Set(ctx, longKey, "value")
	assert.ErrorIs(t, err, ErrKeyTooLong)
}

// ---------------------------------------------------------------------------
// SetWithTTL 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_SetWithTTLPositive 는 양수 TTL 설정을 검증한다.
func TestPersistentStore_SetWithTTLPositive(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.SetWithTTL(ctx, "ttl-key", "ttl-value", 10*time.Second)
	require.NoError(t, err)

	entry, err := store.Get(ctx, "ttl-key")
	require.NoError(t, err)
	assert.Equal(t, "ttl-value", entry.Value)
	assert.False(t, entry.ExpiresAt.IsZero(), "ExpiresAt이 설정되어야 한다")
	assert.True(t, entry.TTL > 0, "남은 TTL은 양수여야 한다")
}

// TestPersistentStore_SetWithTTLNegative 는 음수 TTL 시 ErrInvalidTTL을 반환하는지 검증한다.
func TestPersistentStore_SetWithTTLNegative(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.SetWithTTL(ctx, "key", "value", -1*time.Second)
	assert.ErrorIs(t, err, ErrInvalidTTL)
}

// TestPersistentStore_SetWithTTLZero 는 TTL=0이 만료 없음을 의미하는지 검증한다.
func TestPersistentStore_SetWithTTLZero(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.SetWithTTL(ctx, "no-expire", "value", 0)
	require.NoError(t, err)

	entry, err := store.Get(ctx, "no-expire")
	require.NoError(t, err)
	assert.True(t, entry.ExpiresAt.IsZero(), "TTL=0 이면 ExpiresAt은 zero value여야 한다")
}

// ---------------------------------------------------------------------------
// 만료된 키 Lazy Expiration 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_GetExpiredKeyLazyDeletion 은 만료된 키 조회 시 삭제하고 ErrKeyNotFound를 반환하는지 검증한다.
func TestPersistentStore_GetExpiredKeyLazyDeletion(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	// 매우 짧은 TTL로 저장
	err := store.SetWithTTL(ctx, "expire-key", "value", 1*time.Millisecond)
	require.NoError(t, err)

	// 만료 대기
	time.Sleep(10 * time.Millisecond)

	// 만료된 키 조회 시 ErrKeyNotFound
	_, err = store.Get(ctx, "expire-key")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestPersistentStore_HasExpiredKey 는 만료된 키에 대해 Has가 false를 반환하는지 검증한다.
func TestPersistentStore_HasExpiredKey(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.SetWithTTL(ctx, "has-expire", "value", 1*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	has, err := store.Has(ctx, "has-expire")
	require.NoError(t, err)
	assert.False(t, has, "만료된 키는 Has가 false를 반환해야 한다")
}

// ---------------------------------------------------------------------------
// 동시 접근 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_ConcurrentAccess 는 동시 접근 시 데이터 레이스가 없는지 검증한다.
func TestPersistentStore_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	var wg sync.WaitGroup
	const goroutines = 50

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()

			key := "key"
			_ = store.Set(ctx, key, id)
			_, _ = store.Get(ctx, key)
			_, _ = store.Has(ctx, key)
			_, _ = store.Keys(ctx, "*")
		}(i)
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// SetWithTTL 리포지토리 실패 롤백 테스트
// ---------------------------------------------------------------------------

// TestPersistentStore_SetWithTTLRepoFailureRollback 은 SetWithTTL 리포지토리 실패 시 캐시 롤백을 검증한다.
func TestPersistentStore_SetWithTTLRepoFailureRollback(t *testing.T) {
	ctx := context.Background()
	store, repo := newTestPersistentStore()

	// 먼저 정상적으로 저장
	err := store.Set(ctx, "key", "original")
	require.NoError(t, err)

	// 리포지토리 에러 설정
	repo.mu.Lock()
	repo.setErr = assert.AnError
	repo.mu.Unlock()

	// SetWithTTL 실패
	err = store.SetWithTTL(ctx, "key", "new-value", 10*time.Second)
	assert.Error(t, err)

	// 캐시 롤백 확인
	repo.mu.Lock()
	repo.setErr = nil
	repo.mu.Unlock()

	entry, err := store.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "original", entry.Value, "리포지토리 SetWithTTL 실패 시 캐시가 원래 값으로 롤백되어야 한다")
}

// TestPersistentStore_DeleteNonExistent 는 존재하지 않는 키 삭제가 에러 없이 동작하는지 검증한다.
func TestPersistentStore_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.Delete(ctx, "nonexistent")
	assert.NoError(t, err)
}

// TestPersistentStore_SetWithTTLNilValue 는 SetWithTTL에서 nil 값을 거부하는지 검증한다.
func TestPersistentStore_SetWithTTLNilValue(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.SetWithTTL(ctx, "key", nil, time.Second)
	assert.ErrorIs(t, err, ErrNilValue)
}

// TestPersistentStore_SetWithTTLKeyTooLong 은 SetWithTTL에서 키 길이 제한을 검증한다.
func TestPersistentStore_SetWithTTLKeyTooLong(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	longKey := strings.Repeat("z", MaxKeyLength+1)
	err := store.SetWithTTL(ctx, longKey, "value", time.Second)
	assert.ErrorIs(t, err, ErrKeyTooLong)
}

// TestPersistentStore_SetWithTTLNotSerializable 는 SetWithTTL에서 직렬화 불가능한 값을 거부하는지 검증한다.
func TestPersistentStore_SetWithTTLNotSerializable(t *testing.T) {
	ctx := context.Background()
	store, _ := newTestPersistentStore()

	err := store.SetWithTTL(ctx, "func-key", func() {}, time.Second)
	assert.ErrorIs(t, err, ErrNotSerializable)
}
