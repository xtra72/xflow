// store_write_meta_test.go 는 store-write 노드의 data_type/tags 지정 기능을 뒷받침하는
// system 계층 동작을 검증한다:
//   - StoreAgent.SetKeyDataType 의 data_type 등록/보존 정책
//   - NodeStoreAdapter.SetWithMeta 의 통합 동작 (등록 → 값 쓰기 → 태그)
package system

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- SetKeyDataType 정책 ---

// @spec SPEC-STORE-004: 미등록 키에 비-string data_type 을 지정하면 그 타입으로 등록되되
// Source=auto(런타임 등록) 를 유지한다. 타입 검증은 DataType(isDynamicStringMeta)로
// 동작하므로 Source 승격은 불필요하며, Source 는 reset 정책(config=manual 보존/runtime=auto
// 삭제)에만 쓰인다.
func TestSetKeyDataType_Unregistered_NonString_RegistersAuto(t *testing.T) {
	a := newAutoStoreAgent(t)

	a.inner.SetKeyDataType("temp", DataTypeFloat)

	snap := a.StaticKeysSnapshot()
	require.Contains(t, snap, "temp")
	meta := snap["temp"]
	assert.Equal(t, DataTypeFloat, meta.DataType)
	assert.Equal(t, SourceAuto, meta.Source, "런타임 등록 키는 명시 타입이어도 Source=auto")
	assert.Equal(t, "unknown", meta.MetricType)
}

// 미등록 키에 string data_type 을 지정하면 동적 string 정책(Source=auto)을 따른다.
func TestSetKeyDataType_Unregistered_String_RegistersDynamic(t *testing.T) {
	a := newAutoStoreAgent(t)

	a.inner.SetKeyDataType("label", DataTypeString)

	meta := a.StaticKeysSnapshot()["label"]
	assert.Equal(t, DataTypeString, meta.DataType)
	assert.Equal(t, SourceAuto, meta.Source)
}

// 동적 string 키(런타임 자동 등록)에 비-string data_type 을 지정하면 타입은 덮어쓰되
// Source=auto(런타임 출처)를 유지한다(SPEC-STORE-004).
func TestSetKeyDataType_DynamicString_Overwritten(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()

	// 동적 string 키 자동 등록.
	store := a.inner.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "co2", "init"))
	require.Equal(t, DataTypeString, a.StaticKeysSnapshot()["co2"].DataType)

	a.inner.SetKeyDataType("co2", DataTypeInt)

	meta := a.StaticKeysSnapshot()["co2"]
	assert.Equal(t, DataTypeInt, meta.DataType, "동적 키는 지정 타입으로 덮어써진다")
	assert.Equal(t, SourceAuto, meta.Source, "런타임 키는 명시 타입 부여 후에도 Source=auto")
}

// PRESERVE: yaml 정적 키(명시 data_type)는 SetKeyDataType 으로도 보존된다.
func TestSetKeyDataType_StaticKey_Preserved(t *testing.T) {
	a := newStaticKeysAgent(t, false) // manual: indoor:1:room_temp = float, metric_type=temperature

	a.inner.SetKeyDataType("indoor:1:room_temp", DataTypeInt) // 침범 시도

	meta := a.StaticKeysSnapshot()["indoor:1:room_temp"]
	assert.Equal(t, DataTypeFloat, meta.DataType, "정적 키 data_type 은 보존되어야 한다")
	assert.Equal(t, SourceManual, meta.Source)
	assert.Equal(t, "temperature", meta.MetricType, "정적 키 metric_type 보존")
}

// --- NodeStoreAdapter.SetWithMeta 통합 ---

// data_type 지정 쓰기: 값이 그 타입으로 저장되고 검증을 통과한다 (float 그대로 보존).
func TestNodeStoreAdapter_SetWithMeta_DataType(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()

	adapter := a.NodeStoreForNamespace("default").(*NodeStoreAdapter)
	err := adapter.SetWithMeta(ctx, "temp", 21.5, StoreWriteMeta{DataType: "float"})
	require.NoError(t, err)

	// @spec SPEC-STORE-004: 기본 시리즈 (temp, "unknown", {}) 로 라우팅된다.
	seriesKey := EncodeSeriesKey(SeriesID{Key: "temp"})
	meta := a.StaticKeysSnapshot()[seriesKey]
	assert.Equal(t, DataTypeFloat, meta.DataType)

	// 값이 float 으로 그대로 저장됐는지 (동적 string coercion 우회 확인).
	val, ok, getErr := adapter.GetSeries(ctx, "temp", "", nil)
	require.NoError(t, getErr)
	require.True(t, ok)
	assert.Equal(t, 21.5, val, "float 값이 string 으로 변환되지 않아야 한다")
}

// tags 지정 쓰기: 엔트리(키)에 태그가 부여되고 metric_type 은 unknown 으로 유지된다.
func TestNodeStoreAdapter_SetWithMeta_Tags(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()

	adapter := a.NodeStoreForNamespace("default").(*NodeStoreAdapter)
	err := adapter.SetWithMeta(ctx, "hum", float64(55), StoreWriteMeta{
		Tags: map[string]string{"room": "kitchen"},
	})
	require.NoError(t, err)

	// @spec SPEC-STORE-004: 시리즈 (hum, "unknown", {room:kitchen}) 로 라우팅된다.
	seriesKey := EncodeSeriesKey(SeriesID{Key: "hum", Tags: map[string]string{"room": "kitchen"}})
	meta := a.StaticKeysSnapshot()[seriesKey]
	assert.Equal(t, "kitchen", meta.Tags["room"], "태그가 부여되어야 한다")
	assert.Equal(t, "unknown", meta.MetricType, "tags 부여가 metric_type 을 망가뜨리지 않아야 한다")
}

// data_type + tags + TTL 조합 쓰기.
func TestNodeStoreAdapter_SetWithMeta_DataType_Tags_TTL(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()

	adapter := a.NodeStoreForNamespace("default").(*NodeStoreAdapter)
	err := adapter.SetWithMeta(ctx, "co2", 420, StoreWriteMeta{
		DataType: "int",
		Tags:     map[string]string{"unit": "ppm"},
		TTL:      10 * time.Minute,
	})
	require.NoError(t, err)

	// @spec SPEC-STORE-004: 시리즈 (co2, "unknown", {unit:ppm}) 로 라우팅된다.
	seriesKey := EncodeSeriesKey(SeriesID{Key: "co2", Tags: map[string]string{"unit": "ppm"}})
	meta := a.StaticKeysSnapshot()[seriesKey]
	assert.Equal(t, DataTypeInt, meta.DataType)
	assert.Equal(t, "ppm", meta.Tags["unit"])

	val, ok, getErr := adapter.GetSeries(ctx, "co2", "", map[string]string{"unit": "ppm"})
	require.NoError(t, getErr)
	require.True(t, ok)
	assert.Equal(t, 420, val)
}

// PRESERVE: agentResolver 가 없는 어댑터(NewNodeStoreAdapter)는 SetWithMeta 를
// 일반 쓰기로 폴백한다 (메타 미적용, 하위 호환).
func TestNodeStoreAdapter_SetWithMeta_NoAgent_FallsBack(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()

	store := a.inner.ForNamespace("default")
	adapter := NewNodeStoreAdapter(store) // agentResolver 없음

	err := adapter.SetWithMeta(ctx, "k", "v", StoreWriteMeta{DataType: "float"})
	require.NoError(t, err)

	// 값은 기록되지만, data_type 은 동적 string(auto) 으로만 등록된다 (메타 미적용).
	val, ok, getErr := adapter.Get(ctx, "k")
	require.NoError(t, getErr)
	require.True(t, ok)
	assert.Equal(t, "v", val)
	meta := a.StaticKeysSnapshot()["k"]
	assert.Equal(t, DataTypeString, meta.DataType, "agentResolver 없으면 data_type 미적용")
}

// 컴파일 타임: *NodeStoreAdapter 가 SetWithMeta 시그니처를 만족하는지 보장.
var _ interface {
	SetWithMeta(ctx context.Context, key string, value any, opts StoreWriteMeta) error
} = (*NodeStoreAdapter)(nil)
