package node

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/pkg/message"
)

// store_write_default_datatype_test.go 는 store-write 노드에서 data_type 미지정 시
// "auto" sentinel 이 기본값으로 적용되는 동작을 특성화한다.
//
// 배경(선행 결정의 반전): 49884cb8 은 DataTypeAuto sentinel 을 추가하면서 "기존 빈
// data_type 의 string 동작은 보존(비회귀)" 를 명시적으로 선택했다. 그 결과 data_type 을
// 설정하지 않은 기존 플로우는 store 의 "동적 = string" 정책(store.go checkKeyAllowed)에
// 걸려 float64(29.8) 이 "29.8" 로 저장되고, store 기반 차트가 전부 빈 화면이 되었다.
// 본 파일은 그 반전 이후의 동작과 그 경계(무엇이 바뀌고 무엇이 안 바뀌는지)를 고정한다.

// newDefaultDataTypeHarness 는 실 StoreAgent + 메타 경로가 살아있는 어댑터를 만든다.
// 노드에는 `_store` 로 직접 주입하므로 AgentResolver 없이 SetWithMeta 경로가 동작한다.
func newDefaultDataTypeHarness(t *testing.T, opts ...system.StoreOption) (*system.StoreAgent, *system.NodeStoreAdapter) {
	t.Helper()
	ag := system.NewStoreAgent(opts...)
	require.NoError(t, ag.Init(context.Background()))
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })

	adapter := system.NewLazyNodeStoreAdapterWithAgent(
		func() system.Store { return ag.ForNamespace("") },
		func() *system.StoreAgent { return ag },
		"",
	)
	return ag, adapter
}

// 노드 생성은 store_write_dynamic_meta_test.go 의 newConfiguredStoreWriteNode 를 재사용한다.

// processValue 는 payload {"value": v} 메시지를 노드에 흘려보낸다.
func processValue(t *testing.T, n *StoreWriteNode, v any) error {
	t.Helper()
	msg := message.New(message.WithPayload(message.NewPayload(map[string]any{"value": v})))
	_, err := n.Process(context.Background(), msg)
	return err
}

// storedValue 는 시리즈 키의 저장값을 반환한다.
func storedValue(t *testing.T, ag *system.StoreAgent, key, metric string) any {
	t.Helper()
	seriesKey := system.EncodeSeriesKey(system.SeriesID{Key: key, MetricType: metric})
	entry, err := ag.ForNamespace("").Get(context.Background(), seriesKey)
	require.NoError(t, err)
	return entry.Value
}

// registeredType 은 시리즈 키에 등록된 data_type 을 반환한다.
func registeredType(t *testing.T, ag *system.StoreAgent, key, metric string) system.DataType {
	t.Helper()
	seriesKey := system.EncodeSeriesKey(system.SeriesID{Key: key, MetricType: metric})
	meta, ok := ag.StaticKeyMetaFor(seriesKey)
	require.True(t, ok, "시리즈 키가 등록되어야 한다")
	return meta.DataType
}

// === 주 특성 테스트: 미지정 data_type + 숫자값 → 숫자로 저장 =====================

// data_type 미지정 + float 값: 시리즈가 float 로 등록되고 저장값이 **숫자**여야 한다.
// 반전 이전에는 동적 string 등록 + stringifyValue 로 "29.8" 문자열이 저장되어
// store 기반 차트(storeChartValue / toFloat64)가 전부 값을 버렸다.
func TestStoreWriteNode_UnsetDataType_NumericValue_StoresNumber(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "room",
		"value_key":    "$.payload.value",
		"metric_type":  "temperature",
		// data_type 없음 — 기본값 "auto" 가 적용되어야 한다.
	})

	require.NoError(t, processValue(t, n, float64(29.8)))

	assert.Equal(t, system.DataTypeFloat, registeredType(t, ag, "room", "temperature"),
		"미지정 data_type 은 auto 로 추론되어 float 로 등록되어야 한다")
	assert.Equal(t, float64(29.8), storedValue(t, ag, "room", "temperature"),
		"저장값은 숫자여야 한다 (문자열 \"29.8\" 이면 차트가 렌더링되지 않는다)")
}

// metrics[] 항목의 data_type 미지정도 동일하게 auto 로 기본 적용되어야 한다.
func TestStoreWriteNode_UnsetDataType_MetricsLevel_StoresNumber(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "room",
		"metrics": []any{
			// data_type 없음 — metrics 레벨 기본값도 "auto" 여야 한다.
			map[string]any{"metric_type": "humidity", "value_key": "$.payload.value"},
		},
	})

	require.NoError(t, processValue(t, n, float64(41.2)))

	assert.Equal(t, system.DataTypeFloat, registeredType(t, ag, "room", "humidity"))
	assert.Equal(t, float64(41.2), storedValue(t, ag, "room", "humidity"))
}

// === 비회귀: 문자열 값은 여전히 string ==========================================

// data_type 미지정 + 문자열 값: auto 추론 결과가 string 이므로 기존과 동일하게 string.
// ChirpStack 의 magnet_status("close") 같은 비숫자 시리즈가 깨지지 않음을 고정한다.
func TestStoreWriteNode_UnsetDataType_StringValue_StaysString(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "door",
		"value_key":    "$.payload.value",
		"metric_type":  "magnet_status",
	})

	require.NoError(t, processValue(t, n, "close"))

	assert.Equal(t, system.DataTypeString, registeredType(t, ag, "door", "magnet_status"))
	assert.Equal(t, "close", storedValue(t, ag, "door", "magnet_status"))
}

// boolean 값도 auto 추론으로 bool 그대로 보존된다(문자열화되지 않음).
func TestStoreWriteNode_UnsetDataType_BoolValue_StaysBool(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "unit",
		"value_key":    "$.payload.value",
		"metric_type":  "power",
	})

	require.NoError(t, processValue(t, n, true))

	assert.Equal(t, system.DataTypeBoolean, registeredType(t, ag, "unit", "power"))
	assert.Equal(t, true, storedValue(t, ag, "unit", "power"))
}

// === 명시 data_type 은 의미가 바뀌지 않는다 =====================================

// 명시 data_type 6종 + "auto" 는 반전의 영향을 받지 않는다.
// 각 타입에 맞는 값을 써서 등록 타입이 설정 그대로임을 확인한다.
func TestStoreWriteNode_ExplicitDataType_Unchanged(t *testing.T) {
	cases := []struct {
		dataType string
		value    any
		want     system.DataType
	}{
		{"int", 7, system.DataTypeInt},
		{"float", 1.5, system.DataTypeFloat},
		{"string", "s", system.DataTypeString},
		{"boolean", true, system.DataTypeBoolean},
		{"bytes", []byte("b"), system.DataTypeBytes},
		{"json", map[string]any{"a": float64(1)}, system.DataTypeJSON},
		// 명시 "auto" 는 값에서 추론 — 기본값 auto 와 동일한 결과지만 경로가 명시적이다.
		{"auto", 2.25, system.DataTypeFloat},
	}
	for _, tc := range cases {
		t.Run(tc.dataType, func(t *testing.T) {
			ag, adapter := newDefaultDataTypeHarness(t)
			n := newConfiguredStoreWriteNode(t, map[string]any{
				"_store":       adapter,
				"key_template": "k",
				"value_key":    "$.payload.value",
				"metric_type":  "m",
				"data_type":    tc.dataType,
			})

			require.NoError(t, processValue(t, n, tc.value))
			assert.Equal(t, tc.want, registeredType(t, ag, "k", "m"),
				"명시 data_type %q 의 의미는 바뀌지 않아야 한다", tc.dataType)
		})
	}
}

// metrics[] 의 명시 data_type 도 그대로 유지된다.
func TestStoreWriteNode_ExplicitDataType_MetricsLevel_Unchanged(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "k",
		"metrics": []any{
			map[string]any{"metric_type": "m", "value_key": "$.payload.value", "data_type": "string"},
		},
	})

	// 숫자 값이지만 명시 string 이므로 string 시리즈로 등록된다(설정 우선).
	require.NoError(t, processValue(t, n, float64(3.5)))
	assert.Equal(t, system.DataTypeString, registeredType(t, ag, "k", "m"))
}

// === 위험: int 선행 → float 후행 =================================================

// auto 는 **첫 쓰기 값**으로 타입을 고정한다. 첫 값이 Go int 면 시리즈가 int 로
// 고정되고, 이후 비정수 float 쓰기는 ErrTypeMismatch 로 **거부**된다(절삭이 아니라 에러).
// 이는 이번 반전이 도입하는 실제 위험이므로 테스트로 가시화한다.
//
// 주의: JSON 을 거쳐 온 숫자는 모두 float64 이므로 float 로 추론된다(아래 별도 테스트).
// 이 위험은 값이 Go int 계열로 도착하는 경로에서만 발생한다.
func TestStoreWriteNode_UnsetDataType_IntThenFloat_RejectsFloat(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "sensor",
		"value_key":    "$.payload.value",
		"metric_type":  "temp",
	})

	// 첫 값이 Go int → 시리즈가 int 로 고정된다.
	require.NoError(t, processValue(t, n, int(21)))
	assert.Equal(t, system.DataTypeInt, registeredType(t, ag, "sensor", "temp"))

	// 정수값 float(22.0)은 int 로 정규화되어 허용된다(matchesDataType 의 JSON 숫자 호환).
	require.NoError(t, processValue(t, n, float64(22)))
	assert.Equal(t, int64(22), storedValue(t, ag, "sensor", "temp"),
		"정수값 float 는 int64 로 정규화되어 저장된다")

	// 비정수 float(21.5)은 거부된다 — 값이 유실되고 Process 가 에러를 반환한다.
	err := processValue(t, n, float64(21.5))
	require.Error(t, err, "int 로 고정된 시리즈에 비정수 float 쓰기는 거부된다")
	assert.ErrorIs(t, err, system.ErrTypeMismatch)

	// 거부되었으므로 마지막 성공값이 그대로 남는다(부분 절삭 없음).
	assert.Equal(t, int64(22), storedValue(t, ag, "sensor", "temp"))
	assert.Equal(t, system.DataTypeInt, registeredType(t, ag, "sensor", "temp"),
		"거부되어도 등록 타입은 int 로 유지된다(재추론 없음)")
}

// JSON 경유(모든 숫자가 float64) 경로에서는 첫 값이 정수여도 float 로 추론되므로
// 위 int-lock 위험이 발생하지 않음을 특성화한다.
func TestStoreWriteNode_UnsetDataType_JSONNumbers_LockToFloat(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "sensor",
		"value_key":    "$.payload.value",
		"metric_type":  "temp",
	})

	// JSON 디코딩 결과 모사: 정수처럼 보여도 float64.
	require.NoError(t, processValue(t, n, float64(21)))
	assert.Equal(t, system.DataTypeFloat, registeredType(t, ag, "sensor", "temp"))

	// 이후 소수값도 문제없이 저장된다.
	require.NoError(t, processValue(t, n, float64(21.5)))
	assert.Equal(t, float64(21.5), storedValue(t, ag, "sensor", "temp"))
}

// === 이미 등록된 동적 string 시리즈에 대한 영향 ==================================

// 이미 동적 string 으로 등록된 시리즈(= 이번 버그로 망가진 기존 시리즈)에 대해
// 기본 auto 쓰기가 어떤 영향을 주는지 특성화한다.
//
// SetKeyDataType 정책상 "동적 string 키(Source=auto && data_type=string)" 는 지정
// 타입으로 **덮어써진다**. 따라서 기존에 망가진 시리즈는 다음 쓰기에서 숫자 타입으로
// 자동 전환된다. 다만 이미 저장된 과거 문자열 값(히스토리)은 그대로 남는다.
func TestStoreWriteNode_UnsetDataType_ExistingDynamicStringSeries_IsRetyped(t *testing.T) {
	// 히스토리 보존 여부까지 확인해야 하므로 히스토리를 활성화한다(기본값은 0 = 비활성).
	ag, adapter := newDefaultDataTypeHarness(t, system.WithMaxHistorySize(10))
	ctx := context.Background()

	// 사전 조건: 반전 이전 동작(빈 data_type)으로 동적 string 시리즈를 만든다.
	require.NoError(t, adapter.SetWithMeta(ctx, "legacy", float64(10.5),
		system.StoreWriteMeta{DataType: "", MetricType: "temp"}))
	require.Equal(t, system.DataTypeString, registeredType(t, ag, "legacy", "temp"))
	require.Equal(t, "10.5", storedValue(t, ag, "legacy", "temp"),
		"사전 조건: 기존 시리즈는 문자열로 저장되어 있다")

	// 기본 auto 로 동작하는 노드가 같은 시리즈에 쓴다.
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "legacy",
		"value_key":    "$.payload.value",
		"metric_type":  "temp",
	})
	require.NoError(t, processValue(t, n, float64(11.5)))

	assert.Equal(t, system.DataTypeFloat, registeredType(t, ag, "legacy", "temp"),
		"동적 string 시리즈는 auto 추론 타입으로 덮어써진다")
	assert.Equal(t, float64(11.5), storedValue(t, ag, "legacy", "temp"),
		"신규 값은 숫자로 저장된다")

	// 그러나 과거 문자열 값은 히스토리에 그대로 남는다(자동 소급 변환 없음).
	hist, err := adapter.GetHistory(ctx,
		system.EncodeSeriesKey(system.SeriesID{Key: "legacy", MetricType: "temp"}))
	require.NoError(t, err)
	require.NotEmpty(t, hist)
	assert.Equal(t, "10.5", hist[0],
		"과거 문자열 히스토리는 소급 변환되지 않는다")
}

// yaml 로 명시 선언된(정적) 타입 계약은 기본 auto 가 침범하지 못한다.
// SetKeyDataType 은 동적 string 키만 덮어쓰므로, 명시 타입 시리즈는 보존된다.
func TestStoreWriteNode_UnsetDataType_ExplicitlyTypedSeries_Preserved(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	ctx := context.Background()

	// 사전 조건: 명시 string 타입으로 시리즈를 고정한다.
	require.NoError(t, adapter.SetWithMeta(ctx, "pinned", "on",
		system.StoreWriteMeta{DataType: "string", MetricType: "state"}))
	require.Equal(t, system.DataTypeString, registeredType(t, ag, "pinned", "state"))

	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "pinned",
		"value_key":    "$.payload.value",
		"metric_type":  "state",
	})
	require.NoError(t, processValue(t, n, "off"))

	assert.Equal(t, system.DataTypeString, registeredType(t, ag, "pinned", "state"),
		"명시 타입 시리즈는 기본 auto 로 재타이핑되지 않는다")
}

// === 경계: 메타가 전혀 없는 노드는 bare key 경로 그대로 =========================

// data_type / metric_type / tags 가 **모두** 없는 노드는 writeOne 의 useMeta 판정에서
// 기존과 동일하게 일반 Set 경로(bare key)를 탄다. 기본 auto 는 이 판정을 바꾸지 않는다.
//
// 이 경계는 의도적이다: useMeta 를 켜면 저장 키가 bare key 에서 시리즈 인코딩 키
// ("unknown||k") 로 바뀌어, 같은 키를 읽는 store-read 노드와 기존 저장 데이터가
// 끊긴다. 따라서 이 시리즈는 여전히 동적 string 이며, 이번 변경으로 고쳐지지 않는다.
func TestStoreWriteNode_NoMetaAtAll_KeepsBareKeyAndStringPolicy(t *testing.T) {
	ag, adapter := newDefaultDataTypeHarness(t)
	n := newConfiguredStoreWriteNode(t, map[string]any{
		"_store":       adapter,
		"key_template": "plain",
		"value_key":    "$.payload.value",
		// data_type / metric_type / tags 모두 없음.
	})

	require.NoError(t, processValue(t, n, float64(5.5)))

	// bare key 로 저장된다(시리즈 인코딩 키가 아니다).
	entry, err := ag.ForNamespace("").Get(context.Background(), "plain")
	require.NoError(t, err)
	assert.Equal(t, "5.5", entry.Value,
		"메타가 전혀 없는 노드는 기존 동적 string 정책을 그대로 따른다")

	seriesKey := system.EncodeSeriesKey(system.SeriesID{Key: "plain", MetricType: ""})
	_, registered := ag.StaticKeyMetaFor(seriesKey)
	assert.False(t, registered, "시리즈 인코딩 키로 옮겨가지 않아야 한다(저장 키 불변)")
}
