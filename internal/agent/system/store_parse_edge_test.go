// @spec SPEC-STORE-003 v0.3.0
//
// store_parse_edge_test.go — parseStaticKeysRaw / parseTagsRaw 의 잔여 분기 커버리지를
// 채우기 위한 Phase F 보조 테스트.
//
// 본 파일은 다른 Phase F 테스트가 자연스럽게 다루지 않는 corner case 를 명시적으로
// 테스트한다.
//
//   - parseTagsRaw: 강타입 map[string]string 입력 분기 (yaml decode 결과는 보통
//     map[string]any 이지만, 호출자가 직접 map[string]string 을 넘기는 경우)
//   - parseStaticKeysRaw: []map[string]any 입력 분기 (드물지만 호환을 위해 보존된 경로)
//   - parseStaticKeysRaw: 빈 문자열 key 거부, key 가 string 이 아닌 거부
package system

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// parseTagsRaw 분기 커버리지
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// TestParseTagsRaw_StronglyTypedMap 은 호출자가 이미 map[string]string 강타입을 넘겼을 때
// 정상 처리되는 경로를 검증한다 (parseTagsRaw 의 case map[string]string 분기).
func TestParseTagsRaw_StronglyTypedMap(t *testing.T) {
	out, err := parseTagsRaw(map[string]string{"room": "1", "type": "x"}, "k")
	require.NoError(t, err)
	assert.Equal(t, "1", out["room"])
	assert.Equal(t, "x", out["type"])
}

// @spec SPEC-STORE-003 v0.3.0
// TestParseTagsRaw_StronglyTypedMap_InvalidKey 은 강타입 입력에서도 태그 key 정규식
// 검증이 동작함을 검증한다 (security/통일성 — 입력 형식과 무관하게 동일 정책 적용).
func TestParseTagsRaw_StronglyTypedMap_InvalidKey(t *testing.T) {
	_, err := parseTagsRaw(map[string]string{"room.bad": "v"}, "k")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidTagKey),
		"강타입 입력에서도 ErrInvalidTagKey 가 반환되어야 한다")
}

// @spec SPEC-STORE-003 v0.3.0
// TestParseTagsRaw_UnsupportedType 은 tags 가 map 도 아닌 임의 타입(예: string) 일 때
// 명시적 에러가 반환됨을 검증한다 (parseTagsRaw 의 default 분기).
func TestParseTagsRaw_UnsupportedType(t *testing.T) {
	_, err := parseTagsRaw("not-a-map", "k")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a map",
		"에러 메시지는 tags 가 map 이어야 함을 명시해야 한다")
}

// =============================================================================
// parseStaticKeysRaw 분기 커버리지
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// TestParseStaticKeysRaw_AlreadyTypedSlice 는 입력이 [] 가 아닌 []map[string]any 강타입일
// 때 정상 처리되는 경로를 검증한다 (parseStaticKeysRaw 의 case []map[string]any 분기).
//
// 본 분기는 yaml decode 결과의 표준 형식 ([]any) 과 다른 호출자 (예: 테스트, 프로그램매틱
// config 빌더) 에 대비한 호환 경로이다.
func TestParseStaticKeysRaw_AlreadyTypedSlice(t *testing.T) {
	raw := []map[string]any{
		{"key": "k1", "data_type": "int"},
		{"key": "k2", "data_type": "string"},
	}
	out, err := parseStaticKeysRaw(raw, RegistrationManual)
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, DataTypeInt, out["k1"].DataType)
	assert.Equal(t, DataTypeString, out["k2"].DataType)
}

// @spec SPEC-STORE-003 v0.3.0
// TestParseStaticKeysRaw_EmptyKeyString 은 keys 엔트리의 key 필드가 빈 문자열일 때
// 명시적 에러로 거부됨을 검증한다.
func TestParseStaticKeysRaw_EmptyKeyString(t *testing.T) {
	raw := []any{
		map[string]any{"key": "", "data_type": "string"},
	}
	_, err := parseStaticKeysRaw(raw, RegistrationManual)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be non-empty")
}

// @spec SPEC-STORE-003 v0.3.0
// TestParseStaticKeysRaw_KeyNotString 은 keys 엔트리의 key 필드가 string 이 아닌 타입일 때
// 명시적 에러로 거부됨을 검증한다.
func TestParseStaticKeysRaw_KeyNotString(t *testing.T) {
	raw := []any{
		map[string]any{"key": 42, "data_type": "string"},
	}
	_, err := parseStaticKeysRaw(raw, RegistrationManual)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be string")
}

// @spec SPEC-STORE-003 v0.3.0
// TestParseStaticKeysRaw_DataTypeNotString 은 keys 엔트리의 data_type 필드가 string 이 아닌
// 타입일 때 명시적 에러로 거부됨을 검증한다.
func TestParseStaticKeysRaw_DataTypeNotString(t *testing.T) {
	raw := []any{
		map[string]any{"key": "k", "data_type": 42}, // int — string 이 아님
	}
	_, err := parseStaticKeysRaw(raw, RegistrationManual)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "data_type")
	assert.Contains(t, err.Error(), "must be string")
}

// @spec SPEC-STORE-003 v0.3.0
// TestParseStaticKeysRaw_ItemNotMap 은 keys 배열의 한 entry 가 map 이 아닐 때
// 명시적 에러로 거부됨을 검증한다.
func TestParseStaticKeysRaw_ItemNotMap(t *testing.T) {
	raw := []any{
		"not-a-map", // string entry
	}
	_, err := parseStaticKeysRaw(raw, RegistrationManual)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a map")
}

// @spec SPEC-STORE-003 v0.3.0
// TestParseStaticKeysRaw_NilInput 은 입력이 nil 일 때 nil 결과 + nil 에러를 반환함을 검증한다.
func TestParseStaticKeysRaw_NilInput(t *testing.T) {
	out, err := parseStaticKeysRaw(nil, RegistrationAuto)
	require.NoError(t, err)
	assert.Nil(t, out)
}
