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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	_, err := store.Get(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestVolatileStore_SetNilValue 는 nil 값 저장 시 ErrNilValue를 반환하는지 검증한다.
func TestVolatileStore_SetNilValue(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	err := store.Set(ctx, "key", nil)
	assert.ErrorIs(t, err, ErrNilValue)
}

// TestVolatileStore_SetNilValueWithTTL 은 SetWithTTL에서 nil 값 저장 시 ErrNilValue를 반환하는지 검증한다.
func TestVolatileStore_SetNilValueWithTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	err := store.SetWithTTL(ctx, "key", nil, time.Second)
	assert.ErrorIs(t, err, ErrNilValue)
}

// TestVolatileStore_KeyTooLong 은 키 길이 제한을 검증한다.
func TestVolatileStore_KeyTooLong(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	longKey := strings.Repeat("b", MaxKeyLength+1)
	err := store.SetWithTTL(ctx, longKey, "value", time.Second)
	assert.ErrorIs(t, err, ErrKeyTooLong)
}

// TestVolatileStore_SetWithTTL 은 TTL이 적용된 저장과 조회를 검증한다.
func TestVolatileStore_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	err := store.SetWithTTL(ctx, "key", "value", -1*time.Second)
	assert.ErrorIs(t, err, ErrInvalidTTL)
}

// TestVolatileStore_SetWithTTLZero 는 TTL=0이 만료 없음을 의미하는지 검증한다.
func TestVolatileStore_SetWithTTLZero(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	err := store.SetWithTTL(ctx, "no-expire", "value", 0)
	require.NoError(t, err)

	entry, err := store.Get(ctx, "no-expire")
	require.NoError(t, err)
	assert.True(t, entry.ExpiresAt.IsZero(), "TTL=0 이면 ExpiresAt은 zero value여야 한다")
}

// TestVolatileStore_SetPreservesExistingTTL 은 Set()이 기존 키의 TTL을 보존하는지 검증한다.
func TestVolatileStore_SetPreservesExistingTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	err := store.Delete(ctx, "nonexistent")
	assert.NoError(t, err, "존재하지 않는 키 삭제는 에러를 반환하지 않아야 한다")
}

// TestVolatileStore_Has 는 키 존재 확인을 검증한다.
func TestVolatileStore_Has(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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
	store := NewVolatileStore(MaxKeyLength, 0, 0)

	err := store.Set(ctx, "key", "original")
	require.NoError(t, err)

	err = store.Set(ctx, "key", "updated")
	require.NoError(t, err)

	entry, err := store.Get(ctx, "key")
	require.NoError(t, err)
	assert.Equal(t, "updated", entry.Value)
}

// --- 히스토리 추적 테스트 ---

// TestVolatileStore_History_Accumulation 은 Set 호출 시 이전 값이 히스토리에 누적되는지 검증한다.
func TestVolatileStore_History_Accumulation(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0) // maxHistorySize=10

	// 3번 Set: v1 → v2 → v3
	require.NoError(t, store.Set(ctx, "k", "v1"))
	time.Sleep(time.Millisecond)
	require.NoError(t, store.Set(ctx, "k", "v2"))
	time.Sleep(time.Millisecond)
	require.NoError(t, store.Set(ctx, "k", "v3"))

	// 현재 값은 v3
	entry, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v3", entry.Value)
	assert.Equal(t, 2, entry.HistoryCount, "v1, v2 두 개의 히스토리가 있어야 한다")
	assert.Equal(t, 10, entry.MaxHistorySize)

	// 히스토리는 최신순: [v2, v1]
	history, err := store.GetHistory(ctx, "k")
	require.NoError(t, err)
	require.Len(t, history, 2)
	assert.Equal(t, "v2", history[0].Value, "최신 히스토리가 먼저 와야 한다")
	assert.Equal(t, "v1", history[1].Value)
	assert.True(t, history[0].Timestamp.After(history[1].Timestamp), "최신 항목의 타임스탬프가 더 커야 한다")
}

// TestVolatileStore_History_CountTrimming 은 maxHistorySize 초과 시 가장 오래된 항목이 제거되는지 검증한다.
func TestVolatileStore_History_CountTrimming(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 3, 0) // maxHistorySize=3

	// 5번 Set: v1 → v2 → v3 → v4 → v5
	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Set(ctx, "k", i))
		time.Sleep(time.Millisecond)
	}

	// 현재 값은 5, 히스토리는 최대 3개: [4, 3, 2]
	history, err := store.GetHistory(ctx, "k")
	require.NoError(t, err)
	require.Len(t, history, 3, "maxHistorySize=3이므로 최대 3개만 보관해야 한다")
	assert.Equal(t, 4, history[0].Value)
	assert.Equal(t, 3, history[1].Value)
	assert.Equal(t, 2, history[2].Value)
}

// TestVolatileStore_History_TimeTrimming 은 historyTTL 초과 히스토리 항목이 제거되는지 검증한다.
func TestVolatileStore_History_TimeTrimming(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 100, 50*time.Millisecond) // historyTTL=50ms

	// v1 저장 후 TTL보다 긴 시간 대기
	require.NoError(t, store.Set(ctx, "k", "v1"))
	time.Sleep(80 * time.Millisecond)

	// v2 저장 → v1의 히스토리 항목이 TTL 초과로 트리밍됨
	require.NoError(t, store.Set(ctx, "k", "v2"))

	history, err := store.GetHistory(ctx, "k")
	require.NoError(t, err)
	assert.Empty(t, history, "historyTTL 초과 항목은 트리밍되어야 한다")

	// 빠르게 v3 저장 → v2는 아직 TTL 이내이므로 히스토리에 남음
	require.NoError(t, store.Set(ctx, "k", "v3"))

	history, err = store.GetHistory(ctx, "k")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "v2", history[0].Value)
}

// TestVolatileStore_History_DisabledByDefault 는 maxHistorySize=0일 때 히스토리가 기록되지 않는지 검증한다.
func TestVolatileStore_History_DisabledByDefault(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0) // 히스토리 비활성

	require.NoError(t, store.Set(ctx, "k", "v1"))
	require.NoError(t, store.Set(ctx, "k", "v2"))
	require.NoError(t, store.Set(ctx, "k", "v3"))

	// 히스토리가 비어 있어야 한다
	history, err := store.GetHistory(ctx, "k")
	require.NoError(t, err)
	assert.Empty(t, history, "maxHistorySize=0이면 히스토리가 기록되지 않아야 한다")

	entry, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 0, entry.HistoryCount)
	assert.Equal(t, 0, entry.MaxHistorySize)
}

// TestVolatileStore_History_GetHistoryNotFound 는 존재하지 않는 키에 대해 ErrKeyNotFound를 반환하는지 검증한다.
func TestVolatileStore_History_GetHistoryNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	_, err := store.GetHistory(ctx, "nonexistent")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestVolatileStore_History_GetHistoryEmpty 는 히스토리가 없는 키에 대해 빈 슬라이스를 반환하는지 검증한다.
func TestVolatileStore_History_GetHistoryEmpty(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	// 한 번만 Set → 히스토리 없음
	require.NoError(t, store.Set(ctx, "k", "v1"))

	history, err := store.GetHistory(ctx, "k")
	require.NoError(t, err)
	assert.Empty(t, history, "한 번만 Set한 키는 히스토리가 비어 있어야 한다")
}

// TestVolatileStore_History_GetHistoryExpiredKey 는 만료된 키에 대해 GetHistory가 ErrKeyNotFound를 반환하는지 검증한다.
func TestVolatileStore_History_GetHistoryExpiredKey(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	require.NoError(t, store.SetWithTTL(ctx, "k", "v1", 1*time.Millisecond))
	time.Sleep(10 * time.Millisecond)

	_, err := store.GetHistory(ctx, "k")
	assert.ErrorIs(t, err, ErrKeyNotFound, "만료된 키는 ErrKeyNotFound를 반환해야 한다")
}

// TestVolatileStore_History_SetWithTTL 은 SetWithTTL도 히스토리를 기록하는지 검증한다.
func TestVolatileStore_History_SetWithTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	require.NoError(t, store.SetWithTTL(ctx, "k", "v1", 10*time.Second))
	time.Sleep(time.Millisecond)
	require.NoError(t, store.SetWithTTL(ctx, "k", "v2", 10*time.Second))

	history, err := store.GetHistory(ctx, "k")
	require.NoError(t, err)
	require.Len(t, history, 1)
	assert.Equal(t, "v1", history[0].Value)
}

// TestVolatileStore_History_ConcurrentAccess 는 동시 Set + GetHistory 호출 시 데이터 레이스가 없는지 검증한다.
func TestVolatileStore_History_ConcurrentAccess(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 50, 0)

	var wg sync.WaitGroup
	const goroutines = 100

	wg.Add(goroutines)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			_ = store.Set(ctx, "k", id)
			_, _ = store.GetHistory(ctx, "k")
			_, _ = store.Get(ctx, "k")
		}(i)
	}

	wg.Wait()

	// 패닉 없이 완료되면 성공
	entry, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.NotNil(t, entry.Value)
}

// TestVolatileStore_VariousValueTypes 는 다양한 타입의 값을 저장하고 조회할 수 있는지 검증한다.
func TestVolatileStore_VariousValueTypes(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 0, 0)

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

// ---------------------------------------------------------------------------
// @spec SPEC-STORE-003
// ClearHistory 테스트: 히스토리만 비우고 엔트리(value/ttl/createdAt 등)는 보존한다.
// 정책(static vs dynamic) 은 핸들러 계층의 책임이므로 저장소 레벨에서는 분기하지 않는다.
// ---------------------------------------------------------------------------

// TestVolatileStore_ClearHistory_PreservesEntry 는 ClearHistory 가 히스토리만 비우고
// value/createdAt/expiresAt 등 엔트리 메타데이터를 보존하는지 검증한다.
func TestVolatileStore_ClearHistory_PreservesEntry(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	// 5번 갱신하여 4개의 히스토리 항목을 만든다 (현재값 v5, history=[v4,v3,v2,v1]).
	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Set(ctx, "k", i))
		time.Sleep(time.Millisecond) // updatedAt 변별을 위함
	}

	before, err := store.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, 4, before.HistoryCount, "사전 조건: 히스토리가 4개여야 한다")

	// Act
	err = store.ClearHistory(ctx, "k")
	require.NoError(t, err)

	// Assert: 엔트리는 그대로
	after, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, 5, after.Value, "현재값(value)은 보존되어야 한다")
	assert.Equal(t, before.CreatedAt, after.CreatedAt, "CreatedAt 은 보존되어야 한다")
	assert.Equal(t, before.UpdatedAt, after.UpdatedAt, "UpdatedAt 은 보존되어야 한다")
	assert.Equal(t, before.ExpiresAt, after.ExpiresAt, "ExpiresAt 은 보존되어야 한다")
	assert.Equal(t, 0, after.HistoryCount, "히스토리 개수는 0이어야 한다")

	// GetHistory 도 빈 슬라이스를 반환한다.
	hs, err := store.GetHistory(ctx, "k")
	require.NoError(t, err)
	assert.Empty(t, hs, "ClearHistory 후 GetHistory 는 빈 슬라이스여야 한다")
}

// TestVolatileStore_ClearHistory_PreservesTTL 는 ClearHistory 가 TTL 만료 예정시각을
// 보존하는지 검증한다.
func TestVolatileStore_ClearHistory_PreservesTTL(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 5, 0)

	require.NoError(t, store.SetWithTTL(ctx, "k", "v1", 1*time.Hour))
	time.Sleep(time.Millisecond)
	require.NoError(t, store.SetWithTTL(ctx, "k", "v2", 1*time.Hour))

	before, err := store.Get(ctx, "k")
	require.NoError(t, err)
	require.False(t, before.ExpiresAt.IsZero())

	require.NoError(t, store.ClearHistory(ctx, "k"))

	after, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, before.ExpiresAt, after.ExpiresAt, "ExpiresAt 은 보존되어야 한다")
	assert.Equal(t, "v2", after.Value)
	assert.Equal(t, 0, after.HistoryCount)
}

// TestVolatileStore_ClearHistory_KeyNotFound 는 존재하지 않는 키에 대해
// ErrKeyNotFound 를 반환하는지 검증한다 (Delete/GetHistory 와 일관).
func TestVolatileStore_ClearHistory_KeyNotFound(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	err := store.ClearHistory(ctx, "missing")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestVolatileStore_ClearHistory_EmptyHistory_NoOp 는 이미 히스토리가 없는 키에 대해
// no-op 으로 nil 을 반환하는지 검증한다.
func TestVolatileStore_ClearHistory_EmptyHistory_NoOp(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	require.NoError(t, store.Set(ctx, "k", "v1")) // 한번만 Set → 히스토리 없음
	before, err := store.Get(ctx, "k")
	require.NoError(t, err)
	require.Equal(t, 0, before.HistoryCount)

	err = store.ClearHistory(ctx, "k")
	require.NoError(t, err, "히스토리가 없어도 no-op 으로 nil 을 반환해야 한다")

	// 엔트리는 그대로.
	after, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v1", after.Value)
	assert.Equal(t, 0, after.HistoryCount)
}

// TestVolatileStore_ClearHistory_ExpiredKey 는 만료된 키에 대해 ErrKeyNotFound 를
// 반환하고 lazy expiration 으로 키를 삭제하는지 검증한다.
func TestVolatileStore_ClearHistory_ExpiredKey(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 10, 0)

	require.NoError(t, store.SetWithTTL(ctx, "k", "v1", 1*time.Millisecond))
	time.Sleep(10 * time.Millisecond)

	err := store.ClearHistory(ctx, "k")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestVolatileStore_ClearHistory_ConcurrentSet 는 ClearHistory 와 Set 이 동시에 호출되어도
// 데이터 레이스나 panic 이 발생하지 않는지 검증한다 (-race 로 실행 시 실효).
func TestVolatileStore_ClearHistory_ConcurrentSet(t *testing.T) {
	ctx := context.Background()
	store := NewVolatileStore(MaxKeyLength, 50, 0)

	// 사전 조건: 키 존재.
	require.NoError(t, store.Set(ctx, "k", 0))

	var wg sync.WaitGroup
	const goroutines = 50

	wg.Add(goroutines * 2)
	for i := range goroutines {
		go func(id int) {
			defer wg.Done()
			_ = store.Set(ctx, "k", id)
		}(i)
		go func() {
			defer wg.Done()
			_ = store.ClearHistory(ctx, "k")
		}()
	}
	wg.Wait()

	// 패닉 없이 완료되면 성공. 키는 여전히 존재해야 한다.
	entry, err := store.Get(ctx, "k")
	require.NoError(t, err)
	assert.NotNil(t, entry.Value)
}
