package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @spec SPEC-STORE-004
// store_persistent_series_test.go — 영속 정합(M6) characterization 테스트.
//
// 목적: PersistentStore/StoreRepository 가 시리즈 인코딩 키와 정합하는지 검증·보정한다.
//
// 핵심 관찰(ANALYZE): 시리즈 인코딩은 system 계층의 SetWithMeta 가 책임지며, Store 인터페이스
// (Set/Get/Delete/Has/Keys) 는 시리즈를 전혀 모르는 채 "불투명한 string 키"만 다룬다. 즉
// SetWithMeta 가 이미 EncodeSeriesKey 로 산출한 시리즈 키를 Store.Set 에 넘기면, 영속 계층은
// 그 키를 그대로 저장/조회하면 된다 (시그니처 변경 0).
//
// 본 테스트는 다음을 보존(characterize)한다:
//   - PersistentStore 는 시리즈 인코딩 키를 byte-perfect 로 저장/조회/삭제한다 (불투명 키 계약).
//   - namespace prefix + 시리즈 인코딩 결합 키(`namespace:metric|tags|key`)도 동일하게 처리.
//   - 서로 다른 시리즈(같은 key, 다른 metric/tags) 는 서로 다른 영속 키로 독립 보관된다.
//   - 동일 시리즈(tags 순서만 다름) 는 같은 영속 키로 정합되어 동일 엔트리를 갱신한다.
//
// 이로써 "영속 백엔드는 시리즈를 모르는 채 string 키만 다룬다" 는 SPEC §영속 백엔드 영향의
// 설계 결정이 코드로 보장됨을 회귀 방지한다.

// TestPersistentStore_SeriesEncodedKey_OpaqueRoundTrip 는 시리즈 인코딩 키가 영속 계층에서
// 불투명 string 키로 정확히 round-trip 되는지 확인한다 (영속 정합 핵심).
func TestPersistentStore_SeriesEncodedKey_OpaqueRoundTrip(t *testing.T) {
	t.Parallel()

	store, repo := newTestPersistentStore()
	ctx := context.Background()

	// SetWithMeta 가 산출하는 것과 동일한 시리즈 인코딩 키.
	seriesKey := EncodeSeriesKey(SeriesID{
		Key:        "room",
		MetricType: "temperature",
		Tags:       map[string]string{"area": "a"},
	})
	require.Equal(t, "temperature|area=a|room", seriesKey,
		"시리즈 인코딩 형식(metric|tags|key)이 기대와 일치해야 한다")

	// 영속 저장/조회 — 키는 불투명 string 으로 취급된다.
	require.NoError(t, store.Set(ctx, seriesKey, 22.5))

	got, err := store.Get(ctx, seriesKey)
	require.NoError(t, err)
	assert.InDelta(t, 22.5, got.Value, 1e-9)

	// 리포지토리에도 동일한 시리즈 키가 그대로 저장되어야 한다 (byte-perfect).
	repo.mu.Lock()
	_, ok := repo.entries[seriesKey]
	repo.mu.Unlock()
	assert.True(t, ok, "리포지토리는 시리즈 인코딩 키를 변형 없이 저장해야 한다")

	// 삭제도 동일 키로 정확히 동작한다.
	require.NoError(t, store.Delete(ctx, seriesKey))
	_, err = store.Get(ctx, seriesKey)
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// TestPersistentStore_NamespacedSeriesKey 는 namespace prefix 와 시리즈 인코딩이 결합된
// 최종 키(`namespace:metric|tags|key`)도 영속 계층이 불투명하게 처리하는지 확인한다.
func TestPersistentStore_NamespacedSeriesKey(t *testing.T) {
	t.Parallel()

	store, _ := newTestPersistentStore()
	ctx := context.Background()

	seriesKey := EncodeSeriesKey(SeriesID{Key: "sensor", MetricType: "humidity"})
	// NamespacedStore 는 prefix 를 시리즈 인코딩 바깥에 붙인다 (A2): namespace + ":" + seriesKey.
	finalKey := "default:" + seriesKey

	require.NoError(t, store.Set(ctx, finalKey, 55))
	got, err := store.Get(ctx, finalKey)
	require.NoError(t, err)
	assert.EqualValues(t, 55, toInt(got.Value))
}

// TestPersistentStore_DistinctSeriesAreIndependent 는 같은 key 라도 metric/tags 가 다른
// 시리즈가 영속 계층에서 서로 독립된 키로 보관되어 덮어쓰지 않음을 확인한다 (N1/N2 영속 정합).
func TestPersistentStore_DistinctSeriesAreIndependent(t *testing.T) {
	t.Parallel()

	store, _ := newTestPersistentStore()
	ctx := context.Background()

	tempKey := EncodeSeriesKey(SeriesID{Key: "room", MetricType: "temperature"})
	humKey := EncodeSeriesKey(SeriesID{Key: "room", MetricType: "humidity"})
	require.NotEqual(t, tempKey, humKey, "다른 metric_type 은 다른 시리즈 키여야 한다")

	require.NoError(t, store.Set(ctx, tempKey, 22))
	require.NoError(t, store.Set(ctx, humKey, 55))

	// 두 시리즈가 독립 보관되어 서로 덮어쓰지 않는다.
	tempEntry, err := store.Get(ctx, tempKey)
	require.NoError(t, err)
	assert.EqualValues(t, 22, toInt(tempEntry.Value))

	humEntry, err := store.Get(ctx, humKey)
	require.NoError(t, err)
	assert.EqualValues(t, 55, toInt(humEntry.Value))
}

// TestPersistentStore_TagOrderInvariantKey 는 tags 순서만 다른 동일 시리즈가 같은 영속 키로
// 정합되어 동일 엔트리를 갱신함을 확인한다 (U2 영속 정합).
func TestPersistentStore_TagOrderInvariantKey(t *testing.T) {
	t.Parallel()

	store, repo := newTestPersistentStore()
	ctx := context.Background()

	key1 := EncodeSeriesKey(SeriesID{Key: "t", MetricType: "temp", Tags: map[string]string{"room": "1", "floor": "2"}})
	key2 := EncodeSeriesKey(SeriesID{Key: "t", MetricType: "temp", Tags: map[string]string{"floor": "2", "room": "1"}})
	require.Equal(t, key1, key2, "tags 순서가 달라도 동일 시리즈 키여야 한다")

	require.NoError(t, store.Set(ctx, key1, 20))
	require.NoError(t, store.Set(ctx, key2, 21)) // 같은 시리즈 → 갱신

	got, err := store.Get(ctx, key1)
	require.NoError(t, err)
	assert.EqualValues(t, 21, toInt(got.Value), "동일 시리즈는 새 엔트리가 아니라 갱신되어야 한다")

	// 리포지토리에 시리즈 엔트리가 정확히 1개여야 한다 (시리즈 폭증 없음).
	repo.mu.Lock()
	count := len(repo.entries)
	repo.mu.Unlock()
	assert.Equal(t, 1, count, "tags 순서 차이로 별개 엔트리가 생기면 안 된다")
}
