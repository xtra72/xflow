package system

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 컴파일 타임에 Store 인터페이스 구현을 확인한다.
var _ Store = (*VolatileStore)(nil)

// TestVolatileStore_SetAndGet 은 기본 Set/Get 동작을 검증한다.
func TestVolatileStore_SetAndGet(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.Set(ctx, "key1", "value1")
	require.NoError(t, err)

	entry, err := store.Get(ctx, "key1")
	require.NoError(t, err)
	assert.Equal(t, "value1", entry.Value)
	assert.False(t, entry.CreatedAt.IsZero(), "CreatedAt 은 설정되어야 한다")
	assert.False(t, entry.UpdatedAt.IsZero(), "UpdatedAt 은 설정되어야 한다")
	assert.True(t, entry.ExpiresAt.IsZero(), "TTL 없이 Set하면 ExpiresAt은 zero value여야 한다")
	assert.Equal(t, time.Duration(0), entry.TTL, "TTL 없이 Set하면 TTL은 0이어야 한다")
}

// TestVolatileStore_GetNotFound 는 존재하지 않는 키 조회 시 ErrKeyNotFound를 반환하는지 검증한다.
func TestVolatileStore_GetNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	_, err := store.Get(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestVolatileStore_SetNilValue 는 nil 값 저장 시 ErrNilValue를 반환하는지 검증한다.
func TestVolatileStore_SetNilValue(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.Set(ctx, "key", nil)
	assert.ErrorIs(t, err, ErrNilValue)
}

// TestVolatileStore_SetNilValueWithTTL 은 SetWithTTL에서 nil 값 저장 시 ErrNilValue를 반환하는지 검증한다.
func TestVolatileStore_SetNilValueWithTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.SetWithTTL(ctx, "key", nil, time.Second)
	assert.ErrorIs(t, err, ErrNilValue)
}

// TestVolatileStore_KeyTooLong 은 키 길이 제한을 검증한다.
func TestVolatileStore_KeyTooLong(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	// 정확히 MaxKeyLength 인 키는 허용되어야 한다
	exactKey := strings.Repeat("a", MaxKeyLength)
	err := store.Set(ctx, exactKey, "value")
	assert.NoError(t, err, "MaxKeyLength 길이의 키는 허용되어야 한다")

	// MaxKeyLength + 1 인 키는 거부되어야 한다
	longKey := strings.Repeat("a", MaxKeyLength+1)
	err = store.Set(ctx, longKey, "value")
	assert.ErrorIs(t, err, ErrKeyTooLong)
}

// TestVolatileStore_KeyTooLongWithTTL 은 SetWithTTL에서도 키 길이 제한이 적용되는지 검증한다.
func TestVolatileStore_KeyTooLongWithTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	longKey := strings.Repeat("b", MaxKeyLength+1)
	err := store.SetWithTTL(ctx, longKey, "value", time.Second)
	assert.ErrorIs(t, err, ErrKeyTooLong)
}

// TestVolatileStore_SetWithTTL 은 TTL이 적용된 저장과 조회를 검증한다.
func TestVolatileStore_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.SetWithTTL(ctx, "ttl-key", "ttl-value", 10*time.Second)
	require.NoError(t, err)

	entry, err := store.Get(ctx, "ttl-key")
	require.NoError(t, err)
	assert.Equal(t, "ttl-value", entry.Value)
	assert.False(t, entry.ExpiresAt.IsZero(), "SetWithTTL은 ExpiresAt을 설정해야 한다")
	assert.True(t, entry.TTL > 0, "남은 TTL은 양수여야 한다")
}

// TestVolatileStore_SetWithTTLNegative 는 음수 TTL 시 ErrInvalidTTL을 반환하는지 검증한다.
func TestVolatileStore_SetWithTTLNegative(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.SetWithTTL(ctx, "key", "value", -1*time.Second)
	assert.ErrorIs(t, err, ErrInvalidTTL)
}

// TestVolatileStore_SetWithTTLZero 는 TTL=0이 만료 없음을 의미하는지 검증한다.
func TestVolatileStore_SetWithTTLZero(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.SetWithTTL(ctx, "no-expire", "value", 0)
	require.NoError(t, err)

	entry, err := store.Get(ctx, "no-expire")
	require.NoError(t, err)
	assert.True(t, entry.ExpiresAt.IsZero(), "TTL=0 이면 ExpiresAt은 zero value여야 한다")
}

// TestVolatileStore_SetPreservesExistingTTL 은 Set()이 기존 키의 TTL을 보존하는지 검증한다.
func TestVolatileStore_SetPreservesExistingTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	// TTL과 함께 초기 값 저장
	err := store.SetWithTTL(ctx, "key", "v1", 10*time.Second)
	require.NoError(t, err)

	entry1, err := store.Get(ctx, "key")
	require.NoError(t, err)
	originalExpiresAt := entry1.ExpiresAt
	originalCreatedAt := entry1.CreatedAt

	// Set()으로 값만 업데이트 - TTL은 보존되어야 한다
	time.Sleep(5 * time.Millisecond)
	err = store.Set(ctx, "key", "v2")
	require.NoError(t, err)

	entry2, err := store.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "v2", entry2.Value, "값은 갱신되어야 한다")
	assert.Equal(t, originalCreatedAt, entry2.CreatedAt, "CreatedAt은 보존되어야 한다")
	assert.Equal(t, originalExpiresAt, entry2.ExpiresAt, "ExpiresAt은 보존되어야 한다")
	assert.True(t, entry2.UpdatedAt.After(entry1.UpdatedAt), "UpdatedAt은 갱신되어야 한다")
}

// TestVolatileStore_SetWithTTLUpdatesExpiresAt 은 SetWithTTL()이 기존 키의 ExpiresAt을 갱신하는지 검증한다.
func TestVolatileStore_SetWithTTLUpdatesExpiresAt(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.SetWithTTL(ctx, "key", "v1", 5*time.Second)
	require.NoError(t, err)

	entry1, err := store.Get(ctx, "key")
	require.NoError(t, err)
	originalCreatedAt := entry1.CreatedAt

	// SetWithTTL로 새 TTL 적용
	time.Sleep(5 * time.Millisecond)
	err = store.SetWithTTL(ctx, "key", "v2", 30*time.Second)
	require.NoError(t, err)

	entry2, err := store.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "v2", entry2.Value)
	assert.Equal(t, originalCreatedAt, entry2.CreatedAt, "CreatedAt은 보존되어야 한다")
	assert.True(t, entry2.ExpiresAt.After(entry1.ExpiresAt), "ExpiresAt은 새로운 TTL로 갱신되어야 한다")
}

// TestVolatileStore_GetExpiredKey 는 만료된 키 조회 시 ErrKeyNotFound를 반환하고 키를 삭제하는지 검증한다.
func TestVolatileStore_GetExpiredKey(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	// 매우 짧은 TTL로 저장
	err := store.SetWithTTL(ctx, "expire-key", "value", 1*time.Millisecond)
	require.NoError(t, err)

	// 만료 대기
	time.Sleep(10 * time.Millisecond)

	// 만료된 키 조회 시 ErrKeyNotFound
	_, err = store.Get(ctx, "expire-key")
	assert.ErrorIs(t, err, ErrKeyNotFound, "만료된 키는 ErrKeyNotFound를 반환해야 한다")

	// lazy expiration으로 키가 삭제되었는지 확인
	has, err := store.Has(ctx, "expire-key")
	require.NoError(t, err)
	assert.False(t, has, "만료 후 Get 호출로 키가 삭제되어야 한다")
}

// TestVolatileStore_Delete 는 키 삭제를 검증한다.
func TestVolatileStore_Delete(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.Set(ctx, "del-key", "value")
	require.NoError(t, err)

	err = store.Delete(ctx, "del-key")
	assert.NoError(t, err)

	_, err = store.Get(ctx, "del-key")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestVolatileStore_DeleteNonExistent 는 존재하지 않는 키 삭제가 에러 없이 동작하는지 검증한다.
func TestVolatileStore_DeleteNonExistent(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.Delete(ctx, "nonexistent")
	assert.NoError(t, err, "존재하지 않는 키 삭제는 에러를 반환하지 않아야 한다")
}

// TestVolatileStore_Has 는 키 존재 확인을 검증한다.
func TestVolatileStore_Has(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	has, err := store.Has(ctx, "no-key")
	require.NoError(t, err)
	assert.False(t, has)

	err = store.Set(ctx, "yes-key", "value")
	require.NoError(t, err)

	has, err = store.Has(ctx, "yes-key")
	require.NoError(t, err)
	assert.True(t, has)
}

// TestVolatileStore_HasExpiredKey 는 만료된 키에 대해 Has가 false를 반환하는지 검증한다.
func TestVolatileStore_HasExpiredKey(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.SetWithTTL(ctx, "has-expire", "value", 1*time.Millisecond)
	require.NoError(t, err)

	time.Sleep(10 * time.Millisecond)

	has, err := store.Has(ctx, "has-expire")
	require.NoError(t, err)
	assert.False(t, has, "만료된 키는 Has가 false를 반환해야 한다")
}

// TestVolatileStore_KeysAll 은 빈 패턴이나 "*"로 모든 키를 반환하는지 검증한다.
func TestVolatileStore_KeysAll(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	_ = store.Set(ctx, "a", 1)
	_ = store.Set(ctx, "b", 2)
	_ = store.Set(ctx, "c", 3)

	// 빈 문자열: 모든 키
	keys, err := store.Keys(ctx, "")
	require.NoError(t, err)
	assert.Len(t, keys, 3)

	// "*": 모든 키
	keys, err = store.Keys(ctx, "*")
	require.NoError(t, err)
	assert.Len(t, keys, 3)
}

// TestVolatileStore_KeysPattern 은 패턴 매칭으로 키를 필터링하는지 검증한다.
func TestVolatileStore_KeysPattern(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	_ = store.Set(ctx, "user:1", "alice")
	_ = store.Set(ctx, "user:2", "bob")
	_ = store.Set(ctx, "session:1", "s1")

	keys, err := store.Keys(ctx, "user:*")
	require.NoError(t, err)
	assert.Len(t, keys, 2)
	assert.Contains(t, keys, "user:1")
	assert.Contains(t, keys, "user:2")
}

// TestVolatileStore_KeysSkipsExpired 는 Keys가 만료된 키를 제외하는지 검증한다.
func TestVolatileStore_KeysSkipsExpired(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	_ = store.Set(ctx, "alive", "yes")
	_ = store.SetWithTTL(ctx, "expired", "no", 1*time.Millisecond)

	time.Sleep(10 * time.Millisecond)

	keys, err := store.Keys(ctx, "*")
	require.NoError(t, err)
	assert.Len(t, keys, 1)
	assert.Contains(t, keys, "alive")
}

// TestVolatileStore_Clear 는 모든 키를 삭제하는지 검증한다.
func TestVolatileStore_Clear(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	_ = store.Set(ctx, "a", 1)
	_ = store.Set(ctx, "b", 2)

	err := store.Clear(ctx)
	require.NoError(t, err)

	keys, err := store.Keys(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, keys, "Clear 후 키가 없어야 한다")
}

// TestVolatileStore_ConcurrentAccess 는 동시 접근 시 데이터 레이스가 없는지 검증한다.
func TestVolatileStore_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	var wg sync.WaitGroup
	const goroutines = 100

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()

			key := "key"
			// 쓰기
			_ = store.Set(ctx, key, id)
			// 읽기
			_, _ = store.Get(ctx, key)
			// 존재 확인
			_, _ = store.Has(ctx, key)
			// 키 목록
			_, _ = store.Keys(ctx, "*")
		}(i)
	}

	wg.Wait()
}

// TestVolatileStore_SetOverwrite 는 기존 키에 Set 호출 시 값이 덮어써지는지 검증한다.
func TestVolatileStore_SetOverwrite(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	err := store.Set(ctx, "key", "original")
	require.NoError(t, err)

	err = store.Set(ctx, "key", "updated")
	require.NoError(t, err)

	entry, err := store.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "updated", entry.Value)
}

// TestVolatileStore_VariousValueTypes 는 다양한 타입의 값을 저장하고 조회할 수 있는지 검증한다.
func TestVolatileStore_VariousValueTypes(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength)

	tests := []struct {
		name  string
		key   string
		value any
	}{
		{"정수", "int", 42},
		{"문자열", "string", "hello"},
		{"불리언", "bool", true},
		{"슬라이스", "slice", []int{1, 2, 3}},
		{"맵", "map", map[string]int{"a": 1}},
		{"구조체", "struct", struct{ Name string }{"test"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Set(ctx, tc.key, tc.value)
			require.NoError(t, err)

			entry, err := store.Get(ctx, tc.key)
			require.NoError(t, err)
			assert.Equal(t, tc.value, entry.Value)
		})
	}
}
