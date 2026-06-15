// @spec SPEC-STORE-004
//
// store_series_write_char_test.go — M2 PRESERVE 단계의 characterization 테스트.
//
// 목적: 시리즈 라우팅(IMPROVE) 으로 전환하기 전, 쓰기 경로의 "시리즈 식별과
// 무관한 내부 동작"이 시리즈 인코딩 키 위에서도 그대로 보존되는지를 고정한다.
// 즉 아래 동작들은 시리즈 모델 도입 후에도 (시리즈 단위로) 변하지 않아야 한다:
//   - 동적(auto) string 키 정책: 미등록 키 첫 쓰기 → data_type=string 자동 등록,
//     이후 어떤 값이든 string 으로 coercion (동적=string).
//   - 명시 data_type(SetKeyDataType) 키: 숫자 coercion(int/float) 보존, string 우회.
//   - gatekeeper 거부(manual 미등록) 시 엔트리/history 에 흔적 없음 (N4).
//
// 이 테스트들은 시리즈 인코딩 키를 명시적으로 사용하여, IMPROVE 이후 SetWithMeta 가
// 시리즈 키로 라우팅하더라도 동일한 내부 계약이 유지됨을 보장한다.

package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestChar_SeriesWrite_DynamicStringPolicy 는 메타 없는 SetWithMeta(기본 시리즈)가
// 동적 string 정책을 따르는지 고정한다: 첫 쓰기 시 data_type=string 으로 자동 등록되고,
// 후속 숫자 쓰기는 string 으로 변환되어 저장된다.
func TestChar_SeriesWrite_DynamicStringPolicy(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newMetaAdapterWithAgent(t, "default")

	// 메타 없는 쓰기 = 기본 시리즈 (key, "unknown", {}).
	require.NoError(t, adapter.SetWithMeta(ctx, "dyn", 42, StoreWriteMeta{}))

	seriesKey := EncodeSeriesKey(SeriesID{Key: "dyn"})
	meta, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok, "기본 시리즈가 시리즈 인코딩 키로 등록되어야 한다")
	assert.Equal(t, DataTypeString, meta.DataType, "동적 키는 string 으로 자동 등록")
	assert.Equal(t, SourceAuto, meta.Source)

	// 후속 숫자 쓰기 → string 으로 변환.
	require.NoError(t, adapter.SetWithMeta(ctx, "dyn", 99, StoreWriteMeta{}))
	val, found, err := adapter.GetSeries(ctx, "dyn", "", nil)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "99", val, "동적 string 키는 어떤 값이든 string 으로 저장")
}

// TestChar_SeriesWrite_ExplicitDataTypeCoercion 는 명시 data_type(float) 시리즈에서
// 숫자 coercion(int → float64) 이 시리즈 키 위에서도 보존됨을 고정한다.
func TestChar_SeriesWrite_ExplicitDataTypeCoercion(t *testing.T) {
	ctx := context.Background()
	adapter, _ := newMetaAdapterWithAgent(t, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "temp", 22, StoreWriteMeta{
		DataType:   "float",
		MetricType: "temperature",
	}))

	val, found, err := adapter.GetSeries(ctx, "temp", "temperature", nil)
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, float64(22), val, "float 선언 시리즈에 int 쓰기는 float64 로 coercion")
}

// TestChar_SeriesWrite_ManualRejectionNoTrace 는 manual 모드에서 미등록 시리즈 쓰기가
// 거부되며 엔트리/history 에 흔적이 남지 않음을 고정한다 (N4, S1).
func TestChar_SeriesWrite_ManualRejectionNoTrace(t *testing.T) {
	ctx := context.Background()

	// manual 모드 + 정적 시리즈 (room, temperature, {}) 하나만 등록.
	allowed := EncodeSeriesKey(SeriesID{Key: "room", MetricType: "temperature"})
	sa := NewStoreAgent(
		WithRegistrationType(RegistrationManual),
		WithStaticKeys(map[string]StaticKeyMeta{
			allowed: {DataType: DataTypeInt, MetricType: "temperature", Tags: map[string]string{}, Source: SourceManual},
		}),
		WithMaxHistorySize(10),
	)
	require.NoError(t, sa.Init(ctx))
	t.Cleanup(func() { _ = sa.Stop(ctx) })

	storeResolver := func() Store { return sa.ForNamespace("default") }
	agentResolver := func() *StoreAgent { return sa }
	adapter := NewLazyNodeStoreAdapterWithAgent(storeResolver, agentResolver, "default")

	// 미등록 시리즈 (room, humidity, {}) 쓰기 → 거부.
	err := adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{MetricType: "humidity"})
	require.ErrorIs(t, err, ErrKeyNotAllowed, "manual 미등록 시리즈 쓰기는 거부되어야 한다")

	// 거부된 시리즈에 흔적이 없어야 한다.
	rejected := EncodeSeriesKey(SeriesID{Key: "room", MetricType: "humidity"})
	store := sa.ForNamespace("default")
	_, getErr := store.Get(ctx, rejected)
	require.ErrorIs(t, getErr, ErrKeyNotFound, "거부된 시리즈는 엔트리가 없어야 한다")
}
