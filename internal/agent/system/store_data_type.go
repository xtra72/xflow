package system

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
)

// @spec SPEC-STORE-003 v0.4.0
// MetricTypeUnknown 은 metric_type 미지정 시 적용되는 기본값이다.
// auto 등록 키 및 yaml 에서 metric_type 을 생략한 키에 일관되게 부여된다.
// 정규식 ^[a-zA-Z0-9_-]+$ 를 만족하므로 ErrInvalidMetricType 과 충돌하지 않는다.
const MetricTypeUnknown = "unknown"

// @spec SPEC-STORE-003 v0.3.0
// store_data_type.go — Store 의 v0.3.0 진화에서 도입된 타입 시스템 기반.
// DataType / RegistrationType / RegistrationSource enum 과 inferDataType,
// matchesDataType, validateDataTypeEnum, validateMetricType, validateRegistrationType,
// isJSONMarshalable 같은 헬퍼들이 응집되어 있다. 단위 테스트 격리가 쉽도록 분리되었다.

// =============================================================================
// 타입 정의 (Type Definitions)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// DataType 은 정적 키에 등록되는 6종 타입 enum 이다.
// yaml 의 `data_type` 필드와 매핑되며, manual 모드에서는 명시 필수, auto 모드에서는 추론된다.
type DataType string

// @spec SPEC-STORE-003 v0.3.0
// 6종 DataType enum 상수 정의.
const (
	DataTypeInt     DataType = "int"
	DataTypeFloat   DataType = "float"
	DataTypeString  DataType = "string"
	DataTypeBoolean DataType = "boolean"
	DataTypeBytes   DataType = "bytes"
	DataTypeJSON    DataType = "json"
)

// @spec SPEC-STORE-003 v0.3.0
// RegistrationType 은 store 의 키 등록 정책을 표현하는 enum 이다.
// yaml 의 `registration_type` 필드와 매핑되며, v0.2.0 의 `allow_dynamic_keys` 의
// clean rename (no shim) 이다.
type RegistrationType string

// @spec SPEC-STORE-003 v0.3.0
// RegistrationType enum 상수 정의.
const (
	RegistrationManual RegistrationType = "manual"
	RegistrationAuto   RegistrationType = "auto"
)

// @spec SPEC-STORE-003 v0.3.0
// RegistrationSource 는 정적 키가 어떤 경로로 등록되었는지를 표현한다.
// "manual" = yaml 명시, "auto" = 런타임 자동 등록.
type RegistrationSource string

// @spec SPEC-STORE-003 v0.3.0
// RegistrationSource enum 상수 정의.
const (
	SourceManual RegistrationSource = "manual"
	SourceAuto   RegistrationSource = "auto"
)

// @spec SPEC-STORE-003 v0.3.0
// StaticKeyMeta 는 정적 키에 대한 메타데이터 묶음이다.
// v0.2.0 의 단순 `map[string]map[string]string` (key → tags) 모델을 대체한다.
//
// 필드:
//   - DataType: 6종 enum 중 하나 (manual: yaml 명시, auto: 추론)
//   - MetricType: free string, ^[a-zA-Z0-9_-]+$ 정규식, default "unknown"
//   - Tags: 기존 v0.2.0 태그 맵 (^[a-zA-Z0-9_-]+$)
//   - Source: yaml manual 등록 vs 런타임 auto 등록 구분
type StaticKeyMeta struct {
	DataType   DataType
	MetricType string
	Tags       map[string]string
	Source     RegistrationSource
}

// =============================================================================
// 정규식 패턴 (Regex Patterns)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// metricTypePattern 은 metric_type 의 허용 정규식이다.
// 영문 대소문자, 숫자, 언더스코어, 하이픈만 허용한다.
// 주의: 정규식 자체는 leading dash ("-foo") 를 허용한다 — 이는 의도된 design choice 이다.
var metricTypePattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// =============================================================================
// inferDataType: auto 모드의 핵심 추론 함수
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// inferDataType 은 Go 값으로부터 DataType 을 추론한다.
// auto 모드의 첫 쓰기에서 호출되어 정적 키 메타데이터의 DataType 필드를 결정한다.
//
// 매핑:
//   - bool → DataTypeBoolean
//   - int 계열 (int, int8~64, uint, uint8~64) → DataTypeInt
//   - float32, float64 → DataTypeFloat
//   - string → DataTypeString
//   - []byte → DataTypeBytes
//   - map / slice / struct (json marshal 가능) → DataTypeJSON
//
// nil 또는 channel, func 등 json marshal 불가 타입은 ErrUnsupportedValueType 으로 거부된다.
func inferDataType(value any) (DataType, error) {
	if value == nil {
		return "", ErrUnsupportedValueType
	}
	switch v := value.(type) {
	case bool:
		return DataTypeBoolean, nil
	case int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64:
		return DataTypeInt, nil
	case float32, float64:
		return DataTypeFloat, nil
	case string:
		return DataTypeString, nil
	case []byte:
		return DataTypeBytes, nil
	default:
		// map, slice, struct 등 → json 추론 (json marshal 가능 여부로 판별)
		if isJSONMarshalable(v) {
			return DataTypeJSON, nil
		}
		return "", ErrUnsupportedValueType
	}
}

// =============================================================================
// stringifyValue: 동적(auto) string 키의 값 변환 helper
// =============================================================================

// @spec SPEC-STORE-003 v0.4.0
// isStringifiableValue 는 값이 동적 string 키로 저장될 수 있는지(의미 있는 문자열
// 표현을 가지는지) 판별한다. nil, channel, func 은 false 를 반환한다.
//
// 이는 기존 auto 모드의 ErrUnsupportedValueType 거부 동작을 보존하기 위함이다.
// v0.3.0 에서는 inferDataType 이 channel/func 을 거부했으나, v0.4.0 에서 동적 키가
// 무조건 string 으로 저장되도록 바뀌면서 거부 책임이 이 함수로 이동했다.
func isStringifiableValue(value any) bool {
	if value == nil {
		return false
	}
	switch reflect.TypeOf(value).Kind() {
	case reflect.Chan, reflect.Func:
		return false
	default:
		return true
	}
}

// @spec SPEC-STORE-003 v0.4.0
// stringifyValue 는 임의의 Go 값을 string 으로 변환한다.
// auto 모드에서 미등록 키가 처음 쓰일 때, 해당 키는 data_type=string 으로 등록되고
// 이후 모든 쓰기 값이 이 함수로 string 화되어 저장된다 (동적=string 정책).
//
// 변환 규칙:
//   - string  → 그대로 반환 (불필요한 따옴표 회피)
//   - []byte  → string(b) (바이트 슬라이스를 그대로 문자열로)
//   - 그 외   → fmt.Sprintf("%v", v) (숫자/불리언/맵/슬라이스/구조체 등)
//
// nil 은 호출 전 단계(checkKeyAllowed)에서 이미 거부되므로 여기 도달하지 않지만,
// 방어적으로 빈 문자열을 반환한다.
func stringifyValue(value any) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

// =============================================================================
// matchesDataType: 쓰기 검증 helper
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// matchesDataType 은 Go 값의 추론 타입이 등록된 DataType 과 일치하는지 검사한다.
// nil 이거나 추론 불가능한 타입이면 항상 false 를 반환한다.
// Set/SetWithTTL 의 manual 검증 경로에서 ErrTypeMismatch 판별에 사용된다.
func matchesDataType(value any, dt DataType) bool {
	inferred, err := inferDataType(value)
	if err != nil {
		return false
	}
	return inferred == dt
}

// =============================================================================
// validateDataTypeEnum: yaml 파싱 helper
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// validateDataTypeEnum 은 yaml 의 data_type 문자열이 6종 enum 중 하나인지 검증한다.
// 6종 enum 외 값(빈 문자열 포함, 대소문자 차이 포함) 은 ErrInvalidDataType 으로 거부된다.
func validateDataTypeEnum(s string) error {
	switch DataType(s) {
	case DataTypeInt, DataTypeFloat, DataTypeString,
		DataTypeBoolean, DataTypeBytes, DataTypeJSON:
		return nil
	default:
		return ErrInvalidDataType
	}
}

// =============================================================================
// validateMetricType: yaml 파싱 helper (M8)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// validateMetricType 은 yaml 의 metric_type 문자열을 검증/normalize 한다.
//
// 동작:
//   - 빈 문자열 → ("unknown", nil) — default 적용
//   - 정규식 ^[a-zA-Z0-9_-]+$ 만족 → (s, nil)
//   - 정규식 위반 → ("", ErrInvalidMetricType)
func validateMetricType(s string) (string, error) {
	if s == "" {
		return "unknown", nil
	}
	if !metricTypePattern.MatchString(s) {
		return "", ErrInvalidMetricType
	}
	return s, nil
}

// =============================================================================
// validateRegistrationType: yaml 파싱 helper
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// validateRegistrationType 은 yaml 의 registration_type 문자열이
// "manual" 또는 "auto" 인지 검증한다.
// 빈 문자열 default 처리는 호출자(parseStoreConfig)의 책임이며,
// 본 함수 자체는 명시적 enum 값만 허용한다.
func validateRegistrationType(s string) error {
	switch RegistrationType(s) {
	case RegistrationManual, RegistrationAuto:
		return nil
	default:
		return ErrInvalidDataType
	}
}

// =============================================================================
// isJSONMarshalable: json 추론 helper
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0
// isJSONMarshalable 은 값이 encoding/json 으로 marshal 가능한지 검사한다.
// auto 모드에서 map/slice/struct 등을 DataTypeJSON 으로 받아들일지 판별하는 데 사용된다.
// channel, func 같이 marshal 불가능한 타입은 false 를 반환한다.
func isJSONMarshalable(v any) bool {
	_, err := json.Marshal(v)
	return err == nil
}
