package system

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @spec SPEC-STORE-003 v0.3.0
// store_data_type_test.go — DataType, RegistrationType, RegistrationSource enum과
// inferDataType / matchesDataType / validateDataTypeEnum / validateField /
// validateRegistrationType / isJSONMarshalable helper 들에 대한 단위 테스트.
// 100% 분기 커버리지를 목표로 한다.

// =============================================================================
// A. DataType enum constants
// =============================================================================

// TestDataTypeConstants_AllSixValues 는 6종 DataType enum 상수 값을 검증한다.
func TestDataTypeConstants_AllSixValues(t *testing.T) {
	assert.Equal(t, DataType("int"), DataTypeInt, "DataTypeInt 값 검증")
	assert.Equal(t, DataType("float"), DataTypeFloat, "DataTypeFloat 값 검증")
	assert.Equal(t, DataType("string"), DataTypeString, "DataTypeString 값 검증")
	assert.Equal(t, DataType("boolean"), DataTypeBoolean, "DataTypeBoolean 값 검증")
	assert.Equal(t, DataType("bytes"), DataTypeBytes, "DataTypeBytes 값 검증")
	assert.Equal(t, DataType("json"), DataTypeJSON, "DataTypeJSON 값 검증")
}

// =============================================================================
// B. RegistrationType enum constants
// =============================================================================

// TestRegistrationTypeConstants_ManualAndAuto 는 RegistrationType enum 상수 값을 검증한다.
func TestRegistrationTypeConstants_ManualAndAuto(t *testing.T) {
	assert.Equal(t, RegistrationType("manual"), RegistrationManual, "RegistrationManual 값 검증")
	assert.Equal(t, RegistrationType("auto"), RegistrationAuto, "RegistrationAuto 값 검증")
}

// =============================================================================
// C. RegistrationSource enum constants
// =============================================================================

// TestRegistrationSourceConstants_ManualAndAuto 는 RegistrationSource enum 상수 값을 검증한다.
func TestRegistrationSourceConstants_ManualAndAuto(t *testing.T) {
	assert.Equal(t, RegistrationSource("manual"), SourceManual, "SourceManual 값 검증")
	assert.Equal(t, RegistrationSource("auto"), SourceAuto, "SourceAuto 값 검증")
}

// =============================================================================
// D. inferDataType helper
// =============================================================================

// TestInferDataType_Bool 은 bool 값이 DataTypeBoolean 으로 추론되는지 확인한다.
func TestInferDataType_Bool(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"true", true},
		{"false", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dt, err := inferDataType(tc.value)
			require.NoError(t, err)
			assert.Equal(t, DataTypeBoolean, dt)
		})
	}
}

// TestInferDataType_Int 은 모든 int 계열 타입이 DataTypeInt 으로 추론되는지 확인한다.
func TestInferDataType_Int(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"int", int(1)},
		{"int8", int8(1)},
		{"int16", int16(1)},
		{"int32", int32(1)},
		{"int64", int64(1)},
		{"uint", uint(1)},
		{"uint8", uint8(1)},
		{"uint16", uint16(1)},
		{"uint32", uint32(1)},
		{"uint64", uint64(1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dt, err := inferDataType(tc.value)
			require.NoError(t, err)
			assert.Equal(t, DataTypeInt, dt)
		})
	}
}

// TestInferDataType_Float 은 float32/float64 가 DataTypeFloat 으로 추론되는지 확인한다.
func TestInferDataType_Float(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"float32", float32(1.5)},
		{"float64", float64(1.5)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dt, err := inferDataType(tc.value)
			require.NoError(t, err)
			assert.Equal(t, DataTypeFloat, dt)
		})
	}
}

// TestInferDataType_String 은 string 값이 DataTypeString 으로 추론되는지 확인한다.
func TestInferDataType_String(t *testing.T) {
	dt, err := inferDataType("hello")
	require.NoError(t, err)
	assert.Equal(t, DataTypeString, dt)
}

// TestInferDataType_Bytes 는 []byte 값이 DataTypeBytes 로 추론되는지 확인한다.
func TestInferDataType_Bytes(t *testing.T) {
	dt, err := inferDataType([]byte{0x01, 0x02})
	require.NoError(t, err)
	assert.Equal(t, DataTypeBytes, dt)
}

// TestInferDataType_JSON_Map 은 map 값이 DataTypeJSON 으로 추론되는지 확인한다.
func TestInferDataType_JSON_Map(t *testing.T) {
	dt, err := inferDataType(map[string]any{"a": 1})
	require.NoError(t, err)
	assert.Equal(t, DataTypeJSON, dt)
}

// TestInferDataType_JSON_Slice 는 slice 값(byte slice 제외)이 DataTypeJSON 으로 추론되는지 확인한다.
func TestInferDataType_JSON_Slice(t *testing.T) {
	dt, err := inferDataType([]int{1, 2, 3})
	require.NoError(t, err)
	assert.Equal(t, DataTypeJSON, dt)
}

// TestInferDataType_JSON_Struct 는 json marshal 가능한 struct 가 DataTypeJSON 으로 추론되는지 확인한다.
func TestInferDataType_JSON_Struct(t *testing.T) {
	type sample struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}
	dt, err := inferDataType(sample{Name: "x", Value: 1})
	require.NoError(t, err)
	assert.Equal(t, DataTypeJSON, dt)
}

// TestInferDataType_Nil_Rejected 는 nil 입력이 ErrUnsupportedValueType 으로 거부되는지 확인한다.
func TestInferDataType_Nil_Rejected(t *testing.T) {
	dt, err := inferDataType(nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedValueType))
	assert.Equal(t, DataType(""), dt)
}

// TestInferDataType_Channel_Rejected 는 channel(json marshal 불가) 이 거부되는지 확인한다.
func TestInferDataType_Channel_Rejected(t *testing.T) {
	dt, err := inferDataType(make(chan int))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedValueType))
	assert.Equal(t, DataType(""), dt)
}

// TestInferDataType_Func_Rejected 는 함수값(json marshal 불가) 이 거부되는지 확인한다.
func TestInferDataType_Func_Rejected(t *testing.T) {
	dt, err := inferDataType(func() {})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUnsupportedValueType))
	assert.Equal(t, DataType(""), dt)
}

// =============================================================================
// E. matchesDataType helper
// =============================================================================

// TestMatchesDataType_Match 는 float64 값이 DataTypeFloat 과 매치되는지 확인한다.
func TestMatchesDataType_Match(t *testing.T) {
	assert.True(t, matchesDataType(22.5, DataTypeFloat))
}

// TestMatchesDataType_Mismatch_StringVsFloat 는 string 값이 DataTypeFloat 과 불일치하는지 확인한다.
func TestMatchesDataType_Mismatch_StringVsFloat(t *testing.T) {
	assert.False(t, matchesDataType("hot", DataTypeFloat))
}

// TestMatchesDataType_Mismatch_BoolVsInt 는 bool 값이 DataTypeInt 과 불일치하는지 확인한다.
func TestMatchesDataType_Mismatch_BoolVsInt(t *testing.T) {
	assert.False(t, matchesDataType(true, DataTypeInt))
}

// TestMatchesDataType_NilValue 는 nil 값이 항상 false 를 반환하는지 확인한다.
func TestMatchesDataType_NilValue(t *testing.T) {
	assert.False(t, matchesDataType(nil, DataTypeInt))
	assert.False(t, matchesDataType(nil, DataTypeJSON))
}

// TestMatchesDataType_AllSixTypes_Match 는 6종 enum 각각의 표준 값이 자기 타입과 매치되는지 확인한다.
func TestMatchesDataType_AllSixTypes_Match(t *testing.T) {
	cases := []struct {
		name     string
		value    any
		dataType DataType
	}{
		{"int", int(1), DataTypeInt},
		{"float", float64(1.5), DataTypeFloat},
		{"string", "x", DataTypeString},
		{"boolean", true, DataTypeBoolean},
		{"bytes", []byte{0x01}, DataTypeBytes},
		{"json", map[string]any{"k": "v"}, DataTypeJSON},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.True(t, matchesDataType(tc.value, tc.dataType))
		})
	}
}

// =============================================================================
// F. validateDataTypeEnum helper
// =============================================================================

// TestValidateDataTypeEnum_Valid 는 6종 enum 값이 모두 유효함을 확인한다.
func TestValidateDataTypeEnum_Valid(t *testing.T) {
	valid := []string{"int", "float", "string", "boolean", "bytes", "json"}
	for _, v := range valid {
		t.Run(v, func(t *testing.T) {
			assert.NoError(t, validateDataTypeEnum(v))
		})
	}
}

// TestValidateDataTypeEnum_Invalid_Double 은 enum 외 값("double") 이 거부되는지 확인한다.
func TestValidateDataTypeEnum_Invalid_Double(t *testing.T) {
	err := validateDataTypeEnum("double")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDataType))
}

// TestValidateDataTypeEnum_Invalid_Empty 는 빈 문자열이 거부되는지 확인한다.
func TestValidateDataTypeEnum_Invalid_Empty(t *testing.T) {
	err := validateDataTypeEnum("")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDataType))
}

// TestValidateDataTypeEnum_Invalid_CaseSensitive 는 대소문자 차이 ("Float") 가 거부되는지 확인한다.
func TestValidateDataTypeEnum_Invalid_CaseSensitive(t *testing.T) {
	err := validateDataTypeEnum("Float")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidDataType))
}

// =============================================================================
// G. validateField helper (M8)
// =============================================================================

// TestValidateField_ValidAlphaNum 은 정규식을 만족하는 field 들이 그대로 통과하는지 확인한다.
func TestValidateField_ValidAlphaNum(t *testing.T) {
	cases := []string{"temperature", "humidity", "pressure_v2", "abc-123"}
	for _, v := range cases {
		t.Run(v, func(t *testing.T) {
			out, err := validateField(v)
			require.NoError(t, err)
			assert.Equal(t, v, out)
		})
	}
}

// TestValidateField_DefaultUnknown_EmptyToUnknown 는 빈 문자열이 "unknown" 으로 normalize 되는지 확인한다.
func TestValidateField_DefaultUnknown_EmptyToUnknown(t *testing.T) {
	out, err := validateField("")
	require.NoError(t, err)
	assert.Equal(t, "unknown", out)
}

// TestValidateField_InvalidDot 는 점(.) 이 포함된 문자열이 거부되는지 확인한다.
func TestValidateField_InvalidDot(t *testing.T) {
	out, err := validateField("room.temp")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
	assert.Equal(t, "", out)
}

// TestValidateField_InvalidSpace 는 공백이 포함된 문자열이 거부되는지 확인한다.
func TestValidateField_InvalidSpace(t *testing.T) {
	out, err := validateField("room temp")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
	assert.Equal(t, "", out)
}

// TestValidateField_InvalidColon 은 콜론(:) 이 포함된 문자열이 거부되는지 확인한다.
func TestValidateField_InvalidColon(t *testing.T) {
	out, err := validateField("room:temp")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
	assert.Equal(t, "", out)
}

// TestValidateField_InvalidUnicode 는 유니코드(한글) 가 거부되는지 확인한다.
func TestValidateField_InvalidUnicode(t *testing.T) {
	out, err := validateField("온도")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
	assert.Equal(t, "", out)
}

// TestValidateField_InvalidLeadingDash 는 정규식 ^[a-zA-Z0-9_-]+$ 가
// 선행 dash 를 허용하므로 "-leading" 도 통과한다는 점을 명문화한다.
// 이는 design choice 이며 의도된 동작이다.
func TestValidateField_InvalidLeadingDash(t *testing.T) {
	out, err := validateField("-leading")
	require.NoError(t, err, "정규식이 leading dash 를 허용함 (design choice)")
	assert.Equal(t, "-leading", out)
}

// =============================================================================
// H. validateRegistrationType helper
// =============================================================================

// TestValidateRegistrationType_Manual 은 "manual" 이 유효함을 확인한다.
func TestValidateRegistrationType_Manual(t *testing.T) {
	assert.NoError(t, validateRegistrationType("manual"))
}

// TestValidateRegistrationType_Auto 는 "auto" 가 유효함을 확인한다.
func TestValidateRegistrationType_Auto(t *testing.T) {
	assert.NoError(t, validateRegistrationType("auto"))
}

// TestValidateRegistrationType_Invalid_Automatic 은 enum 외 값("automatic") 이 거부되는지 확인한다.
func TestValidateRegistrationType_Invalid_Automatic(t *testing.T) {
	err := validateRegistrationType("automatic")
	require.Error(t, err)
}

// TestValidateRegistrationType_Invalid_Empty 는 빈 문자열이 거부되는지 확인한다.
// 빈 문자열은 parseStoreConfig 에서 default 적용 (RegistrationAuto) 전 단계로 위임되며,
// validateRegistrationType 자체는 명시적 enum 만 허용한다.
func TestValidateRegistrationType_Invalid_Empty(t *testing.T) {
	err := validateRegistrationType("")
	require.Error(t, err)
}

// =============================================================================
// I. isJSONMarshalable helper
// =============================================================================

// TestIsJSONMarshalable_Map 은 map 이 json marshal 가능함을 확인한다.
func TestIsJSONMarshalable_Map(t *testing.T) {
	assert.True(t, isJSONMarshalable(map[string]any{"k": "v"}))
}

// TestIsJSONMarshalable_Slice 는 slice 가 json marshal 가능함을 확인한다.
func TestIsJSONMarshalable_Slice(t *testing.T) {
	assert.True(t, isJSONMarshalable([]int{1, 2, 3}))
}

// TestIsJSONMarshalable_Struct 는 단순 struct 가 json marshal 가능함을 확인한다.
func TestIsJSONMarshalable_Struct(t *testing.T) {
	type sample struct {
		Name string `json:"name"`
	}
	assert.True(t, isJSONMarshalable(sample{Name: "x"}))
}

// TestIsJSONMarshalable_Channel 은 channel 이 json marshal 불가능함을 확인한다.
func TestIsJSONMarshalable_Channel(t *testing.T) {
	assert.False(t, isJSONMarshalable(make(chan int)))
}

// TestIsJSONMarshalable_Func 는 함수값이 json marshal 불가능함을 확인한다.
func TestIsJSONMarshalable_Func(t *testing.T) {
	assert.False(t, isJSONMarshalable(func() {}))
}

// =============================================================================
// J. Auto 등록 동시성 (Phase C smoke test)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// TestCheckKeyAllowed_AutoRegister_Concurrent_SameType 은 auto 모드에서 동일 키 + 동일 타입을
// 100 goroutine 이 동시에 쓰는 경우 모두 성공하고, 정확히 한 번만 SourceAuto 로 등록됨을 검증한다.
//
// 검증 항목:
//   - 모든 goroutine 이 nil 에러로 성공 (race winner 가 등록한 후 나머지는 type 일치 경로로 통과)
//   - 최종 staticKeys 에는 단 하나의 엔트리 (key=concurrent_int)
//   - 등록된 메타의 Source == SourceAuto, DataType == DataTypeInt
func TestCheckKeyAllowed_AutoRegister_Concurrent_SameType(t *testing.T) {
	sa := NewStoreAgent(WithRegistrationType(RegistrationAuto))
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}

	const goroutines = 100
	const key = "concurrent_int"

	var wg sync.WaitGroup
	var successCount atomic.Int32
	var errCount atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := as.checkKeyAllowed(key, 42); err != nil {
				errCount.Add(1)
				return
			}
			successCount.Add(1)
		}()
	}
	wg.Wait()

	assert.Equal(t, int32(goroutines), successCount.Load(),
		"동일 타입 동시 쓰기는 모두 성공해야 한다")
	assert.Equal(t, int32(0), errCount.Load(),
		"동일 타입 동시 쓰기는 에러 없어야 한다")

	// 정확히 한 번만 등록되었는지 확인.
	// @spec v0.4.0: 동적 키는 항상 data_type=string 으로 등록된다.
	snap := sa.StaticKeysSnapshot()
	require.Len(t, snap, 1, "정적 키는 정확히 1개만 등록되어야 한다")
	meta, ok := snap[key]
	require.True(t, ok)
	assert.Equal(t, DataTypeString, meta.DataType)
	assert.Equal(t, SourceAuto, meta.Source)
	assert.Equal(t, "unknown", meta.Field)
}

// @spec SPEC-STORE-003 v0.4.0 (동적=string 정책 도입에 따른 갱신)
// TestCheckKeyAllowed_AutoRegister_Concurrent_MixedTypes 는 auto 모드에서 동일 키에 대해
// 100 goroutine 이 int 와 string 을 섞어 쓰는 경우 모두 성공함을 검증한다.
//
// v0.4.0: 동적 키는 data_type=string 으로 등록되어 어떤 타입이든 string 으로 변환되므로
// ErrTypeMismatch 가 발생하지 않는다. 정확히 1개 키만 등록되고 DataType=string 이다.
func TestCheckKeyAllowed_AutoRegister_Concurrent_MixedTypes(t *testing.T) {
	sa := NewStoreAgent(WithRegistrationType(RegistrationAuto))
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}

	const goroutines = 100
	const key = "concurrent_mixed"

	var wg sync.WaitGroup
	var success atomic.Int32
	var failure atomic.Int32

	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			var err error
			if i%2 == 0 {
				err = as.checkKeyAllowed(key, 42) // int
			} else {
				err = as.checkKeyAllowed(key, "hello") // string
			}
			if err == nil {
				success.Add(1)
			} else {
				failure.Add(1)
			}
		}()
	}
	wg.Wait()

	// 동적 string 정책: int/string 혼합이어도 모두 성공해야 한다.
	assert.Equal(t, int32(goroutines), success.Load(),
		"동적 string 키는 타입 혼합이어도 모두 허용")
	assert.Equal(t, int32(0), failure.Load(), "ErrTypeMismatch 등 실패는 없어야 한다")

	// 정확히 1개 키 등록 + data_type=string.
	snap := sa.StaticKeysSnapshot()
	require.Len(t, snap, 1)
	meta := snap[key]
	assert.Equal(t, SourceAuto, meta.Source)
	assert.Equal(t, DataTypeString, meta.DataType, "동적 키는 data_type=string")
}

// @spec SPEC-STORE-003 v0.4.0 (동적=string 정책 도입에 따른 갱신)
// TestCheckKeyAllowed_PostAutoRegister_AcceptsAnyType 는 auto 모드에서 한 번 자동
// 등록된 동적 string 키에 대해 이후 어떤 타입의 쓰기도 ErrTypeMismatch 없이
// 허용됨을 검증한다 (동적=string 정책: 모든 값이 string 으로 변환됨).
func TestCheckKeyAllowed_PostAutoRegister_AcceptsAnyType(t *testing.T) {
	sa := NewStoreAgent(WithRegistrationType(RegistrationAuto))
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}

	// 1차 쓰기: int → 동적 string 키 자동 등록.
	require.NoError(t, as.checkKeyAllowed("pinned", 100))

	// 2차 쓰기: 같은 키에 string → 허용 (mismatch 없음).
	assert.NoError(t, as.checkKeyAllowed("pinned", "different type"))

	// 3차 쓰기: bool → 허용.
	assert.NoError(t, as.checkKeyAllowed("pinned", true))

	// 등록 메타: data_type=string 으로 고정, Source=auto 보존.
	snap := sa.StaticKeysSnapshot()
	require.Len(t, snap, 1)
	assert.Equal(t, DataTypeString, snap["pinned"].DataType)
	assert.Equal(t, SourceAuto, snap["pinned"].Source)
}

// @spec SPEC-STORE-003 v0.3.0
// TestCheckKeyAllowed_ManualMode_Reject 는 manual 모드에서 미등록 키 쓰기가
// ErrKeyNotAllowed 로 거부됨을 검증한다 (Scenario 2 / M2 unwanted).
func TestCheckKeyAllowed_ManualMode_Reject(t *testing.T) {
	sa := NewStoreAgent(
		WithRegistrationType(RegistrationManual),
		WithStaticKeys(map[string]StaticKeyMeta{
			"allowed_key": {
				DataType: DataTypeInt,
				Field:    "gauge",
				Tags:     map[string]string{},
				Source:   SourceManual,
			},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}

	// 등록된 키 + 일치 타입 → 통과.
	assert.NoError(t, as.checkKeyAllowed("allowed_key", 42))

	// 등록된 키 + 불일치 타입 → ErrTypeMismatch.
	assert.ErrorIs(t, as.checkKeyAllowed("allowed_key", "wrong"), ErrTypeMismatch)

	// 미등록 키 → ErrKeyNotAllowed (manual 모드에서는 자동 등록 안 함).
	assert.ErrorIs(t, as.checkKeyAllowed("unregistered", 42), ErrKeyNotAllowed)

	// manual 모드에서는 미등록 키에 대해 staticKeys 가 변경되면 안 된다.
	snap := sa.StaticKeysSnapshot()
	require.Len(t, snap, 1)
	_, exists := snap["unregistered"]
	assert.False(t, exists, "manual 모드 거부는 staticKeys 에 흔적을 남기지 않아야 한다")
}

// @spec SPEC-STORE-003 v0.3.0
// TestCheckKeyAllowed_AutoMode_UnsupportedValue 는 auto 모드에서 nil 또는 추론 불가능한
// 타입(channel) 이 ErrUnsupportedValueType 으로 거부되고 staticKeys 에 등록되지 않음을 검증한다.
func TestCheckKeyAllowed_AutoMode_UnsupportedValue(t *testing.T) {
	sa := NewStoreAgent(WithRegistrationType(RegistrationAuto))
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}

	// nil 값 → ErrUnsupportedValueType.
	assert.ErrorIs(t, as.checkKeyAllowed("nil_key", nil), ErrUnsupportedValueType)

	// channel → ErrUnsupportedValueType.
	assert.ErrorIs(t, as.checkKeyAllowed("chan_key", make(chan int)), ErrUnsupportedValueType)

	// func → ErrUnsupportedValueType.
	assert.ErrorIs(t, as.checkKeyAllowed("func_key", func() {}), ErrUnsupportedValueType)

	// 거부된 쓰기는 staticKeys 에 흔적을 남기지 않아야 한다.
	snap := sa.StaticKeysSnapshot()
	assert.Len(t, snap, 0, "거부된 쓰기는 등록되지 않아야 한다")
}
