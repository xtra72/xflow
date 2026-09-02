// @spec SPEC-STORE-003 v0.3.0
//
// store_field_test.go — M8 field 시맨틱을 풀스택(parser → storeAgent state)
// 으로 검증하는 Phase F 테스트.
//
// 본 파일은 store_data_type_test.go 의 validateField 단위 테스트를 보완하며,
// parseStoreConfig 경로(yaml 입력) 와 inner StoreAgent 의 staticKeys 메타에서
// field 이 정확히 노출되는지 통합 시점에서 검증한다.
//
// 커버 시나리오:
//   - Scenario 1, 9 의 field 정상 동작 (M1, M3, M8)
//   - Scenario 5, 8 의 auto 등록 시 field="unknown" default (M8)
//   - Edge case: 빈 문자열 / 정규식 위반 / 비문자열 / field 누락
package system

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// =============================================================================
// 테스트 헬퍼 (Phase F 공용)
// =============================================================================

// buildAutoModeKeysOption 은 keys yaml 입력의 옵션 슬라이스 형태를 반환한다.
// auto 모드 fixture 에서 사용되며, 호출자가 entry 들을 직접 구성한다.
//
// @spec SPEC-STORE-003 v0.3.0
func buildKeysYAMLOption(regType string, entries ...map[string]any) map[string]any {
	list := make([]any, 0, len(entries))
	for _, e := range entries {
		list = append(list, e)
	}
	out := map[string]any{
		"keys": list,
	}
	if regType != "" {
		out["registration_type"] = regType
	}
	return out
}

// parseConfigFromOptions 는 options 맵으로 AgentConfig 를 구성하고 parseStoreConfig 로
// storeConfig 까지 끝까지 적용한 결과를 반환한다.
//
// @spec SPEC-STORE-003 v0.3.0
func parseConfigFromOptions(t *testing.T, options map[string]any) (storeConfig, error) {
	t.Helper()
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type:    "store",
			Options: options,
		},
	}
	opts, err := parseStoreConfig(cfg)
	if err != nil {
		return storeConfig{}, err
	}
	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}
	return sc, nil
}

// =============================================================================
// A. field default ("unknown") — yaml 누락 / 빈 문자열
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M8
// TestField_Default_WhenAbsent 는 yaml entry 에 field 필드가 없을 때
// default "unknown" 이 적용됨을 검증한다.
func TestField_Default_WhenAbsent(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k_no_metric",
			"data_type": "float",
			// field 누락
		},
	)
	sc, err := parseConfigFromOptions(t, options)
	require.NoError(t, err)
	require.Contains(t, sc.staticKeys, "k_no_metric")
	assert.Equal(t, "unknown", sc.staticKeys["k_no_metric"].Field,
		"누락 시 default 'unknown' 이 적용되어야 한다 (M8)")
}

// @spec SPEC-STORE-003 v0.3.0 / M8
// TestField_Default_WhenEmptyString 은 yaml 에 field: "" 가 명시될 때도
// default "unknown" 으로 normalize 됨을 검증한다.
func TestField_Default_WhenEmptyString(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k_empty",
			"data_type": "string",
			"field":     "", // 명시적 빈 문자열
		},
	)
	sc, err := parseConfigFromOptions(t, options)
	require.NoError(t, err)
	require.Contains(t, sc.staticKeys, "k_empty")
	assert.Equal(t, "unknown", sc.staticKeys["k_empty"].Field,
		"빈 문자열은 default 'unknown' 으로 normalize 되어야 한다 (M8)")
}

// =============================================================================
// B. field 정상 케이스 — 정규식 ^[a-zA-Z0-9_-]+$ 준수
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M8
// TestField_Valid_AlphaNumDashUnderscore 은 영문/숫자/언더스코어/하이픈 조합이
// 모두 통과하고 yaml 입력 그대로 보존되는지 검증한다.
func TestField_Valid_AlphaNumDashUnderscore(t *testing.T) {
	cases := []struct {
		name  string
		field string
		dtype string
	}{
		{"temperature", "temperature", "float"},
		{"humidity_v2_underscore", "humidity_v2", "float"},
		{"pressure_dash", "pressure-meter", "float"},
		{"alphanum", "abc123", "int"},
		{"single_char", "a", "string"},
		{"only_digits", "123", "int"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			options := buildKeysYAMLOption("manual",
				map[string]any{
					"key":       "k",
					"data_type": tc.dtype,
					"field":     tc.field,
				},
			)
			sc, err := parseConfigFromOptions(t, options)
			require.NoError(t, err, "유효한 field 은 통과해야 한다")
			assert.Equal(t, tc.field, sc.staticKeys["k"].Field,
				"입력 field 그대로 보존되어야 한다")
		})
	}
}

// =============================================================================
// C. field 정규식 위반 — 부팅 실패 (ErrInvalidField)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M8 unwanted
// TestField_Invalid_DotRejected 는 점(.) 이 포함된 field 이
// ErrInvalidField 으로 부팅 실패시킴을 검증한다 (Edge case "field 정규식 위반").
func TestField_Invalid_DotRejected(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k",
			"data_type": "float",
			"field":     "room.temp", // 점 포함
		},
	)
	_, err := parseConfigFromOptions(t, options)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField),
		"ErrInvalidField 으로 wrap 되어야 한다")
}

// @spec SPEC-STORE-003 v0.3.0 / M8 unwanted
// TestField_Invalid_SpaceRejected 는 공백이 포함된 field 이 거부됨을 검증한다.
func TestField_Invalid_SpaceRejected(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k",
			"data_type": "float",
			"field":     "room temp",
		},
	)
	_, err := parseConfigFromOptions(t, options)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
}

// @spec SPEC-STORE-003 v0.3.0 / M8 unwanted
// TestField_Invalid_ColonRejected 는 콜론(:) 이 포함된 field 이 거부됨을 검증한다.
// (콜론은 태그 형식 separator 와 충돌 가능하므로 명시적으로 금지된다.)
func TestField_Invalid_ColonRejected(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k",
			"data_type": "float",
			"field":     "room:temp",
		},
	)
	_, err := parseConfigFromOptions(t, options)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
}

// @spec SPEC-STORE-003 v0.3.0 / M8 unwanted
// TestField_Invalid_UnicodeRejected 는 유니코드(한글) 가 거부됨을 검증한다.
// 정규식 ^[a-zA-Z0-9_-]+$ 는 ASCII 범위만 허용한다 (security: injection 방어).
func TestField_Invalid_UnicodeRejected(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k",
			"data_type": "float",
			"field":     "온도", // 한글
		},
	)
	_, err := parseConfigFromOptions(t, options)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
}

// @spec SPEC-STORE-003 v0.3.0 / M8 unwanted
// TestField_Invalid_SlashRejected 는 슬래시(/) 가 포함된 field 이 거부됨을 검증한다.
func TestField_Invalid_SlashRejected(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k",
			"data_type": "float",
			"field":     "ab/cd",
		},
	)
	_, err := parseConfigFromOptions(t, options)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidField))
}

// =============================================================================
// D. field 비문자열 타입 거부
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M8
// TestField_NonStringType_Rejected 는 field 이 string 이 아닌 경우
// (예: int) 명시적 에러를 반환함을 검증한다.
func TestField_NonStringType_Rejected(t *testing.T) {
	options := buildKeysYAMLOption("manual",
		map[string]any{
			"key":       "k",
			"data_type": "float",
			"field":     42, // int — string 이 아닌 타입
		},
	)
	_, err := parseConfigFromOptions(t, options)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field")
	assert.Contains(t, err.Error(), "must be string",
		"에러 메시지는 field 이 string 타입이어야 함을 명시해야 한다")
}

// =============================================================================
// E. Auto 등록 키의 field — 항상 "unknown"
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M8 / Scenario 3, 5, 8
// TestField_AutoRegistered_AlwaysUnknown 은 auto 모드에서 미등록 키 첫 쓰기로
// 자동 등록된 키의 field 이 항상 "unknown" 으로 설정됨을 검증한다.
//
// 이는 SPEC M8 의 default 정책과 일관성을 유지하며, 운영자가 명시적으로 field 을
// 부여하기 전까지는 분류 미상으로 표시한다.
func TestField_AutoRegistered_AlwaysUnknown(t *testing.T) {
	cases := []struct {
		name  string
		value any
	}{
		{"int_value", 42},
		{"float_value", 3.14},
		{"string_value", "hello"},
		{"bool_value", true},
		{"bytes_value", []byte{0x01, 0x02}},
		{"json_value", map[string]any{"k": "v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sa := NewStoreAgent(WithRegistrationType(RegistrationAuto))
			require.NoError(t, sa.Init(context.Background()))
			t.Cleanup(func() { _ = sa.Stop(context.Background()) })

			store := sa.ForNamespace("default")
			require.NoError(t, store.Set(context.Background(), "auto_key", tc.value),
				"auto 모드에서 자동 등록 + field=unknown 이어야 한다")

			snap := sa.StaticKeysSnapshot()
			require.Contains(t, snap, "auto_key")
			assert.Equal(t, "unknown", snap["auto_key"].Field,
				"auto 등록 키의 field 은 항상 'unknown' (M8 default)")
			assert.Equal(t, SourceAuto, snap["auto_key"].Source)
		})
	}
}

// =============================================================================
// F. 통합 검증 — yaml manual 키 field 명시 + 자동 등록 키와 공존
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 3 / M8
// TestField_ManualAndAuto_Coexist_DifferentFields 는 manual 키들이 yaml 에서
// 명시한 field 을 보존하고, auto 등록 키는 "unknown" 으로 표시되어 공존함을 검증한다.
func TestField_ManualAndAuto_Coexist_DifferentFields(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "auto",
				"keys": []any{
					map[string]any{
						"key":       "indoor:temperature",
						"data_type": "float",
						"field":     "temperature",
					},
					map[string]any{
						"key":       "indoor:humidity",
						"data_type": "float",
						"field":     "humidity",
					},
					// field 누락 → default "unknown"
					map[string]any{
						"key":       "indoor:default",
						"data_type": "string",
					},
				},
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	// 자동 등록 키 추가.
	store := u.inner.ForNamespace("default")
	require.NoError(t, store.Set(context.Background(), "outdoor:auto", 100))

	snap := u.StaticKeysSnapshot()
	require.Len(t, snap, 4, "manual 3개 + auto 1개")

	// Manual 키 field 검증.
	assert.Equal(t, "temperature", snap["indoor:temperature"].Field)
	assert.Equal(t, "humidity", snap["indoor:humidity"].Field)
	assert.Equal(t, "unknown", snap["indoor:default"].Field,
		"yaml 누락 시 default 'unknown'")

	// Auto 등록 키 field 검증.
	assert.Equal(t, "unknown", snap["outdoor:auto"].Field,
		"auto 등록 시 항상 'unknown'")
}
