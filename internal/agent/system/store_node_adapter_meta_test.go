package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newMetaAdapterWithAgent 는 SetWithMeta 의 metric_type 적용 검증을 위해
// 실제 *StoreAgent 와 연결된 NodeStoreAdapter 를 만든다. 반환된 agent 로
// StaticKeyMetaFor 를 호출해 기록 후 메타를 확인할 수 있다.
func newMetaAdapterWithAgent(t *testing.T, namespace string) (*NodeStoreAdapter, *StoreAgent) {
	t.Helper()
	sa := NewStoreAgent(WithMaxHistorySize(10))
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() {
		_ = sa.Stop(context.Background())
	})
	storeResolver := func() Store { return sa.ForNamespace(namespace) }
	agentResolver := func() *StoreAgent { return sa }
	adapter := NewLazyNodeStoreAdapterWithAgent(storeResolver, agentResolver, namespace)
	return adapter, sa
}

// TestSetWithMeta_AppliesMetricType 는 MetricType 이 지정되면 기록되는 키의
// metric_type 으로 반영되는지 검증한다.
func TestSetWithMeta_AppliesMetricType(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	err := adapter.SetWithMeta(ctx, "k", "v", StoreWriteMeta{
		MetricType: "temperature",
	})
	require.NoError(t, err)

	// @spec SPEC-STORE-004: 레지스트리 키가 시리즈 인코딩으로 승격됨.
	seriesKey := EncodeSeriesKey(SeriesID{Key: "k", MetricType: "temperature"})
	meta, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok)
	assert.Equal(t, "temperature", meta.MetricType)
}

// TestSetWithMeta_MetricTypeWithTags 는 MetricType 과 Tags 가 함께 지정되면
// 둘 다 한 번에 적용되는지 검증한다.
func TestSetWithMeta_MetricTypeWithTags(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	err := adapter.SetWithMeta(ctx, "k", "v", StoreWriteMeta{
		MetricType: "humidity",
		Tags:       map[string]string{"unit": "percent"},
	})
	require.NoError(t, err)

	// @spec SPEC-STORE-004: 시리즈 키(k, humidity, {unit:percent}) 로 메타가 적용된다.
	seriesKey := EncodeSeriesKey(SeriesID{Key: "k", MetricType: "humidity", Tags: map[string]string{"unit": "percent"}})
	meta, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok)
	assert.Equal(t, "humidity", meta.MetricType)
	assert.Equal(t, map[string]string{"unit": "percent"}, meta.Tags)
}

// @spec SPEC-STORE-004
// TestSetWithMeta_DiffMetricTags_IndependentSeries 는 같은 key 에 (metric only) 쓰기와
// (tags only) 쓰기가 서로 독립된 시리즈를 만들고 덮어쓰지 않음을 검증한다 (N1/N2/E2/E3).
// (구 모델의 "메타 덮어쓰기 보존" 테스트가 시리즈 모델에서 독립성 검증으로 진화함.)
func TestSetWithMeta_DiffMetricTags_IndependentSeries(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	// 1) (k, temperature, {}) 시리즈.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v1", StoreWriteMeta{
		MetricType: "temperature",
	}))

	// 2) metric 없이 tags 만 → (k, unknown, {unit:celsius}) 별개 시리즈.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v2", StoreWriteMeta{
		Tags: map[string]string{"unit": "celsius"},
	}))

	// 두 시리즈가 독립 존재하며 서로 덮어쓰지 않는다.
	tempKey := EncodeSeriesKey(SeriesID{Key: "k", MetricType: "temperature"})
	tagsKey := EncodeSeriesKey(SeriesID{Key: "k", Tags: map[string]string{"unit": "celsius"}})

	tempMeta, ok := sa.StaticKeyMetaFor(tempKey)
	require.True(t, ok, "temperature 시리즈가 보존되어야 한다")
	assert.Equal(t, "temperature", tempMeta.MetricType)
	assert.Empty(t, tempMeta.Tags, "temperature 시리즈는 tags-only 쓰기로 변경되지 않는다 (N1)")

	tagsMeta, ok := sa.StaticKeyMetaFor(tagsKey)
	require.True(t, ok, "tags-only 시리즈가 독립 생성되어야 한다")
	assert.Equal(t, MetricTypeUnknown, tagsMeta.MetricType)
	assert.Equal(t, map[string]string{"unit": "celsius"}, tagsMeta.Tags)
}

// @spec SPEC-STORE-004
// TestSetWithMeta_SameSeries_DataTypeIrrelevant 는 같은 (key, metric, tags) 에 대해
// data_type 만 다른 후속 쓰기가 새 시리즈를 만들지 않고 동일 시리즈를 갱신함을 검증한다
// (U3/N3/AC-4: data_type 은 식별 차원이 아니다).
func TestSetWithMeta_SameSeries_DataTypeIrrelevant(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	// 동적 string 시리즈로 쓰기.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v1", StoreWriteMeta{
		MetricType: "temperature",
		Tags:       map[string]string{"unit": "celsius"},
	}))

	// 같은 (key, metric, tags) — data_type 만 명시(float) → 동일 시리즈.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", float64(22.2), StoreWriteMeta{
		DataType:   "float",
		MetricType: "temperature",
		Tags:       map[string]string{"unit": "celsius"},
	}))

	// 시리즈가 하나만 존재해야 한다 (data_type 차이로 새 시리즈 생성 안 함).
	seriesKey := EncodeSeriesKey(SeriesID{Key: "k", MetricType: "temperature", Tags: map[string]string{"unit": "celsius"}})
	snap := sa.StaticKeysSnapshot()
	count := 0
	for sk := range snap {
		if sk == seriesKey {
			count++
		}
	}
	assert.Equal(t, 1, count, "data_type 차이는 동일 시리즈로 합산되어야 한다")
	assert.Len(t, snap, 1, "단일 시리즈만 존재해야 한다")
}

// TestSetWithMeta_DataTypeMetricTags_Combined 는 data_type + metric_type + tags 가
// 함께 지정될 때 모두 반영되는지 검증한다.
func TestSetWithMeta_DataTypeMetricTags_Combined(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	err := adapter.SetWithMeta(ctx, "k", float64(22.2), StoreWriteMeta{
		DataType:   "float",
		MetricType: "temperature",
		Tags:       map[string]string{"unit": "celsius"},
	})
	require.NoError(t, err)

	// @spec SPEC-STORE-004: 시리즈 키(k, temperature, {unit:celsius}) 로 라우팅된다.
	seriesKey := EncodeSeriesKey(SeriesID{Key: "k", MetricType: "temperature", Tags: map[string]string{"unit": "celsius"}})
	meta, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok)
	assert.Equal(t, DataTypeFloat, meta.DataType)
	assert.Equal(t, "temperature", meta.MetricType)
	assert.Equal(t, map[string]string{"unit": "celsius"}, meta.Tags)
}

// TestSetWithMeta_NoMeta_Fallback 는 메타가 전혀 없으면 일반 쓰기로 동작하고
// metric_type 은 unknown(자동 등록) 으로 남는지 확인한다 (기존 동작 보존).
func TestSetWithMeta_NoMeta_Fallback(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v", StoreWriteMeta{}))

	// @spec SPEC-STORE-004: 메타 미지정 = 기본 시리즈 (k, "unknown", {}).
	val, found, err := adapter.GetSeries(ctx, "k", "", nil)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "v", val)

	seriesKey := EncodeSeriesKey(SeriesID{Key: "k"})
	meta, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok)
	assert.Equal(t, MetricTypeUnknown, meta.MetricType, "메타 미지정 시 기본 시리즈 unknown")
}
