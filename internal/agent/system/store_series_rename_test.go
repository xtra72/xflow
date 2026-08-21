package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newRenameFixture 는 UserStoreAgent + 그에 연결된 SetWithMeta 어댑터를 만든다.
func newRenameFixture(t *testing.T) (*UserStoreAgent, *NodeStoreAdapter) {
	t.Helper()
	u := makeStoreAgentForRegistration(t, map[string]any{"backend": "volatile", "max_history_size": 10})
	storeResolver := func() Store { return u.inner.ForNamespace("default") }
	agentResolver := func() *StoreAgent { return u.inner }
	adapter := NewLazyNodeStoreAdapterWithAgent(storeResolver, agentResolver, "default")
	return u, adapter
}

// TestRenameKey_MovesAllSeriesWithValueMetaAndHistory 는 키의 모든 시리즈(값+메타)가
// 새 키로 이동하고 기존 키가 사라짐을 검증한다.
func TestRenameKey_MovesAllSeriesWithValueMetaAndHistory(t *testing.T) {
	ctx := context.Background()
	u, adapter := newRenameFixture(t)

	// oldkey 아래 두 시리즈(field 다름) 생성 + 히스토리 축적.
	require.NoError(t, adapter.SetWithMeta(ctx, "oldkey", float64(21), StoreWriteMeta{Field: "temp", DataType: "float"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "oldkey", float64(22), StoreWriteMeta{Field: "temp", DataType: "float"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "oldkey", float64(40), StoreWriteMeta{Field: "humidity", DataType: "float"}))

	moved, err := u.RenameKey(ctx, "default", "oldkey", "newkey")
	require.NoError(t, err)
	assert.Equal(t, 2, moved, "두 시리즈(temp, humidity)가 이동")

	// 새 키 시리즈가 등록되어 있고 값/메타 보존.
	newTemp := EncodeSeriesKey(SeriesID{Measurement: "newkey", Field: "temp"})
	meta, ok := u.inner.StaticKeyMetaFor(newTemp)
	require.True(t, ok, "새 키 시리즈 메타 존재")
	assert.Equal(t, DataType("float"), meta.DataType, "data_type 보존")

	got, found, err := adapter.GetSeries(ctx, "newkey", "temp", nil)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, float64(22), got, "최신값 보존")

	// 히스토리 보존 확인(temp 시리즈: 21 → 22, 이전값 21 이 히스토리에 남음).
	hist, err := u.inner.ForNamespace("default").GetHistory(ctx, newTemp)
	require.NoError(t, err)
	assert.NotEmpty(t, hist, "히스토리 보존")

	// 기존 키 시리즈는 사라짐.
	oldTemp := EncodeSeriesKey(SeriesID{Measurement: "oldkey", Field: "temp"})
	_, ok = u.inner.StaticKeyMetaFor(oldTemp)
	assert.False(t, ok, "기존 키 메타 제거")
	has, _ := u.inner.ForNamespace("default").Has(ctx, oldTemp)
	assert.False(t, has, "기존 키 값 제거")
}

// TestRenameKey_RejectsWhenDestinationExists 는 대상 키에 동일 시리즈가 이미 있으면
// ErrKeyExists 로 거부하고 아무것도 바꾸지 않음을 검증한다.
func TestRenameKey_RejectsWhenDestinationExists(t *testing.T) {
	ctx := context.Background()
	u, adapter := newRenameFixture(t)

	require.NoError(t, adapter.SetWithMeta(ctx, "oldkey", float64(1), StoreWriteMeta{Field: "temp", DataType: "float"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "newkey", float64(9), StoreWriteMeta{Field: "temp", DataType: "float"}))

	moved, err := u.RenameKey(ctx, "default", "oldkey", "newkey")
	require.ErrorIs(t, err, ErrKeyExists)
	assert.Equal(t, 0, moved)

	// 기존 키는 그대로 남아있어야 한다(변경 없음).
	oldTemp := EncodeSeriesKey(SeriesID{Measurement: "oldkey", Field: "temp"})
	_, ok := u.inner.StaticKeyMetaFor(oldTemp)
	assert.True(t, ok, "거부 시 기존 키 보존")
}

// TestRenameKey_NoMatchingSeries 는 대상 키에 시리즈가 없으면 (0, nil) 반환을 검증한다.
func TestRenameKey_NoMatchingSeries(t *testing.T) {
	ctx := context.Background()
	u, _ := newRenameFixture(t)

	moved, err := u.RenameKey(ctx, "default", "does-not-exist", "newkey")
	require.NoError(t, err)
	assert.Equal(t, 0, moved)
}

// TestRenameKey_ValidatesInput 는 빈 newKey / oldKey==newKey 를 거부함을 검증한다.
func TestRenameKey_ValidatesInput(t *testing.T) {
	ctx := context.Background()
	u, _ := newRenameFixture(t)

	_, err := u.RenameKey(ctx, "default", "k", "")
	require.Error(t, err, "빈 newKey 거부")

	_, err = u.RenameKey(ctx, "default", "k", "k")
	require.Error(t, err, "oldKey==newKey 거부")
}
