package system

import (
	"encoding/json"
	"regexp"
)

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
