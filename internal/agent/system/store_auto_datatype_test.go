// @spec SPEC-STORE-003 v0.4.0
//
// store_auto_datatype_test.go 는 store-write 의 data_type: "auto" (값 기반 타입 추론)
// 경로를 검증한다. 단일 store-write 노드가 측정별로 서로 다른 값 타입(boolean/int/float)을
// 저장할 때, "auto" 가 쓰기 값의 Go 타입을 추론해 키별로 올바른 타입을 고정함을 특성화한다.
//
// 회귀 배경: 단일 노드에 고정 리터럴 data_type(예: float)을 주면 boolean(power) 쓰기가
// ErrTypeMismatch 로 거부됐다. "auto" 는 이 충돌을 값 타입 추론으로 해소한다.
package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resolveWriteDataType 단위: "auto" 는 값 타입을 추론하고, 리터럴은 그대로, 추론 불가/빈값은 (_, false).
func TestResolveWriteDataType(t *testing.T) {
	cases := []struct {
		name       string
		configured string
		value      any
		wantType   DataType
		wantOK     bool
	}{
		{"auto_bool", DataTypeAuto, false, DataTypeBoolean, true},
		{"auto_int", DataTypeAuto, 42, DataTypeInt, true},
		{"auto_float", DataTypeAuto, 21.5, DataTypeFloat, true},
		{"auto_string", DataTypeAuto, "on", DataTypeString, true},
		{"auto_bytes", DataTypeAuto, []byte("x"), DataTypeBytes, true},
		{"auto_json_map", DataTypeAuto, map[string]any{"a": 1}, DataTypeJSON, true},
		{"auto_nil_fails", DataTypeAuto, nil, "", false},
		{"auto_chan_fails", DataTypeAuto, make(chan int), "", false},
		{"literal_float", "float", false /*무시*/, DataTypeFloat, true},
		{"literal_int", "int", "무시", DataTypeInt, true},
		{"empty_falls", "", 1, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dt, ok := resolveWriteDataType(tc.configured, tc.value)
			assert.Equal(t, tc.wantOK, ok)
			assert.Equal(t, tc.wantType, dt)
		})
	}
}

// "auto" 로 boolean 값 쓰기: 키가 boolean 으로 고정되고 값이 bool 로 보존된다(string 변환 없음).
func TestSetWithMeta_Auto_Boolean(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()
	adapter := a.NodeStoreForNamespace("default").(*NodeStoreAdapter)

	err := adapter.SetWithMeta(ctx, "dev-power", false, StoreWriteMeta{DataType: DataTypeAuto, MetricType: "power"})
	require.NoError(t, err)

	seriesKey := EncodeSeriesKey(SeriesID{Key: "dev-power", MetricType: "power"})
	assert.Equal(t, DataTypeBoolean, a.StaticKeysSnapshot()[seriesKey].DataType)

	val, ok, getErr := adapter.GetSeries(ctx, "dev-power", "power", nil)
	require.NoError(t, getErr)
	require.True(t, ok)
	assert.Equal(t, false, val, "boolean 값이 string 으로 변환되지 않아야 한다")
}

// "auto" 로 int 값 쓰기: 키가 int 로 고정되고 숫자로 보존된다.
func TestSetWithMeta_Auto_Int(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()
	adapter := a.NodeStoreForNamespace("default").(*NodeStoreAdapter)

	err := adapter.SetWithMeta(ctx, "dev-mode", 1, StoreWriteMeta{DataType: DataTypeAuto, MetricType: "mode"})
	require.NoError(t, err)

	seriesKey := EncodeSeriesKey(SeriesID{Key: "dev-mode", MetricType: "mode"})
	assert.Equal(t, DataTypeInt, a.StaticKeysSnapshot()[seriesKey].DataType)

	val, ok, getErr := adapter.GetSeries(ctx, "dev-mode", "mode", nil)
	require.NoError(t, getErr)
	require.True(t, ok)
	assert.Equal(t, 1, val)
}

// 회귀 재현: 단일 어댑터가 측정별로 boolean / float / int 값을 "auto" 로 각각 저장한다.
// 고정 리터럴 float 였다면 power(boolean) 에서 ErrTypeMismatch 가 났을 시나리오가 모두 성공해야 한다.
func TestSetWithMeta_Auto_MixedMeasurements_NoMismatch(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()
	adapter := a.NodeStoreForNamespace("default").(*NodeStoreAdapter)

	writes := []struct {
		key      string
		metric   string
		value    any
		wantType DataType
	}{
		{"집중회의실-current_temperature", "current_temperature", 28.4, DataTypeFloat},
		{"집중회의실-target_temperature", "target_temperature", 24.0, DataTypeFloat},
		{"집중회의실-mode", "mode", 1, DataTypeInt},
		{"집중회의실-fan_speed", "fan_speed", 1, DataTypeInt},
		{"집중회의실-power", "power", false, DataTypeBoolean},
	}
	for _, w := range writes {
		err := adapter.SetWithMeta(ctx, w.key, w.value, StoreWriteMeta{DataType: DataTypeAuto, MetricType: w.metric})
		require.NoErrorf(t, err, "measurement %q 쓰기는 성공해야 한다(ErrTypeMismatch 없음)", w.metric)

		seriesKey := EncodeSeriesKey(SeriesID{Key: w.key, MetricType: w.metric})
		assert.Equalf(t, w.wantType, a.StaticKeysSnapshot()[seriesKey].DataType,
			"measurement %q 는 %s 로 고정되어야 한다", w.metric, w.wantType)
	}

	// 같은 power 키에 대한 후속 boolean 쓰기도 계속 성공한다(키 단위 타입 일정).
	require.NoError(t, adapter.SetWithMeta(ctx, "집중회의실-power", true,
		StoreWriteMeta{DataType: DataTypeAuto, MetricType: "power"}))
	val, ok, getErr := adapter.GetSeries(ctx, "집중회의실-power", "power", nil)
	require.NoError(t, getErr)
	require.True(t, ok)
	assert.Equal(t, true, val)
}

// "auto" + 추론 불가 값(nil): 타입 고정을 생략한다(SetKeyDataType 미호출). nil 자체는
// 스토어가 auto 여부와 무관하게 독립적으로 거부하므로, 새로운 실패 모드를 만들지 않고
// 일반 nil 쓰기와 동일하게 에러가 난다(auto 가 nil 거부를 우회하지 않음을 특성화).
func TestSetWithMeta_Auto_Nil_RejectedLikeNormalWrite(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()
	adapter := a.NodeStoreForNamespace("default").(*NodeStoreAdapter)

	err := adapter.SetWithMeta(ctx, "dev-x", nil, StoreWriteMeta{DataType: DataTypeAuto, MetricType: "x"})
	require.Error(t, err, "nil 은 auto 여도 저장되지 않는다(일반 쓰기와 동일)")

	// 고정도 되지 않았고 값도 없으므로 키가 등록되지 않는다.
	seriesKey := EncodeSeriesKey(SeriesID{Key: "dev-x", MetricType: "x"})
	_, registered := a.StaticKeysSnapshot()[seriesKey]
	assert.False(t, registered, "추론/쓰기 모두 실패 시 키가 등록되지 않아야 한다")
}
