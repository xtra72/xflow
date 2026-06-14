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

	meta, ok := sa.StaticKeyMetaFor("k")
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

	meta, ok := sa.StaticKeyMetaFor("k")
	require.True(t, ok)
	assert.Equal(t, "humidity", meta.MetricType)
	assert.Equal(t, map[string]string{"unit": "percent"}, meta.Tags)
}

// TestSetWithMeta_EmptyMetricType_PreservesExisting 는 MetricType 이 빈 문자열일 때
// 기존 metric_type 이 보존되는지 검증한다 (PRESERVE).
func TestSetWithMeta_EmptyMetricType_PreservesExisting(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	// 1) 먼저 metric_type 을 지정해 둔다.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v1", StoreWriteMeta{
		MetricType: "temperature",
	}))

	// 2) metric_type 없이 tags 만 갱신 → 기존 metric_type 이 보존되어야 한다.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v2", StoreWriteMeta{
		Tags: map[string]string{"unit": "celsius"},
	}))

	meta, ok := sa.StaticKeyMetaFor("k")
	require.True(t, ok)
	assert.Equal(t, "temperature", meta.MetricType, "metric_type 미지정 시 기존값 보존")
	assert.Equal(t, map[string]string{"unit": "celsius"}, meta.Tags)
}

// TestSetWithMeta_MetricTypeOnly_PreservesExistingTags 는 MetricType 만 갱신할 때
// 기존 tags 가 보존되는지 검증한다 (SetKeyMeta 가 tags 를 덮어쓰지 않도록).
func TestSetWithMeta_MetricTypeOnly_PreservesExistingTags(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	// 1) tags 를 먼저 지정해 둔다.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v1", StoreWriteMeta{
		Tags: map[string]string{"unit": "celsius"},
	}))

	// 2) metric_type 만 갱신 → 기존 tags 가 보존되어야 한다.
	require.NoError(t, adapter.SetWithMeta(ctx, "k", "v2", StoreWriteMeta{
		MetricType: "temperature",
	}))

	meta, ok := sa.StaticKeyMetaFor("k")
	require.True(t, ok)
	assert.Equal(t, "temperature", meta.MetricType)
	assert.Equal(t, map[string]string{"unit": "celsius"}, meta.Tags, "metric_type 만 갱신 시 기존 tags 보존")
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

	meta, ok := sa.StaticKeyMetaFor("k")
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

	val, found, err := adapter.Get(ctx, "k")
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "v", val)

	meta, ok := sa.StaticKeyMetaFor("k")
	require.True(t, ok)
	assert.Equal(t, MetricTypeUnknown, meta.MetricType, "메타 미지정 시 자동 등록 unknown")
}
