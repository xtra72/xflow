package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLiveSeriesKeys_ReflectsStoredData 는 실제 저장된 시리즈만 LiveSeriesKeys 에
// 포함됨을 검증한다(레지스트리에만 있는 유령 시리즈는 제외되는 근거).
func TestLiveSeriesKeys_ReflectsStoredData(t *testing.T) {
	ctx := context.Background()
	u := makeStoreAgentForRegistration(t, map[string]any{"backend": "volatile"})
	storeResolver := func() Store { return u.inner.ForNamespace("default") }
	agentResolver := func() *StoreAgent { return u.inner }
	adapter := NewLazyNodeStoreAdapterWithAgent(storeResolver, agentResolver, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "dev1", true, StoreWriteMeta{
		MetricType: "power", DataType: "boolean", Tags: map[string]string{"name": "indoor-1"},
	}))

	seriesKey := EncodeSeriesKey(SeriesID{
		Key: "dev1", MetricType: "power", Tags: map[string]string{"name": "indoor-1"},
	})

	live := u.inner.LiveSeriesKeys()
	_, ok := live[seriesKey]
	assert.True(t, ok, "저장된 시리즈는 LiveSeriesKeys 에 포함")

	// 존재하지 않는(유령) 시리즈 키는 포함되지 않는다.
	phantom := EncodeSeriesKey(SeriesID{Key: "dev1", MetricType: "power"})
	_, ok = live[phantom]
	assert.False(t, ok, "실데이터 없는 시리즈는 LiveSeriesKeys 에 없음")
}

// TestLiveSeriesKeys_ExcludesPhantom 은 "레지스트리엔 있으나 실데이터는 없는" 유령
// 시리즈(라인차트에만 보이던 태그 없는 행)를 재현하고, LiveSeriesKeys 가 이를 제외함을
// 검증한다. GET /keys 필터가 이 차이를 이용해 유령을 걸러낸다.
func TestLiveSeriesKeys_ExcludesPhantom(t *testing.T) {
	ctx := context.Background()
	u := makeStoreAgentForRegistration(t, map[string]any{"backend": "volatile", "max_history_size": 10})
	storeResolver := func() Store { return u.inner.ForNamespace("default") }
	agentResolver := func() *StoreAgent { return u.inner }
	adapter := NewLazyNodeStoreAdapterWithAgent(storeResolver, agentResolver, "default")

	// bare 시리즈(태그 없음) 쓰기 → 레지스트리 + 데이터 생성.
	require.NoError(t, adapter.SetWithMeta(ctx, "dev", true, StoreWriteMeta{
		MetricType: "power", DataType: "boolean",
	}))
	bareKey := EncodeSeriesKey(SeriesID{Key: "dev", MetricType: "power"})

	// 데이터만 삭제(레지스트리는 유지) → 유령 상태 재현(과거 만료/삭제 시 레지스트리 미정리).
	require.NoError(t, u.inner.ForNamespace("default").Delete(ctx, bareKey))

	// 레지스트리엔 남아있다(GET /keys 원본).
	snap := u.inner.StaticKeysSnapshot()
	_, inReg := snap[bareKey]
	assert.True(t, inReg, "유령: 레지스트리엔 남아있음")

	// 그러나 LiveSeriesKeys(실데이터 기준)엔 없다 → GET /keys 필터가 제외.
	live := u.inner.LiveSeriesKeys()
	_, inLive := live[bareKey]
	assert.False(t, inLive, "유령: 실데이터 없음 → LiveSeriesKeys 제외 → GET /keys 에서 필터됨")
}
