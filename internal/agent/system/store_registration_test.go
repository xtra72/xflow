// @spec SPEC-STORE-003 v0.3.0
//
// store_registration_test.go — M2 / M6 등록 출처 추적 + manual 모드 거부 정책의
// 풀스택 (parser → storeAgent → ForNamespace → Set) 검증을 담당하는 Phase F 테스트.
//
// 본 파일은 store_data_type_test.go (단위) 와 store_static_keys_test.go (마이그레이션)
// 의 빈틈을 메우며, 다음 SPEC 시나리오를 통합 시점에서 검증한다.
//
//   - Scenario 2: manual 모드 미등록 키 거부 + 흔적 없음
//   - Scenario 3: auto 모드 미등록 키 자동 등록 + manual 키 메타데이터 보존
//   - Scenario 7: manual 모드 명시 data_type 검증 + 누락 시 부팅 실패
//   - Scenario 8: auto 모드 data_type 추론 + type pinning (변경 불가)
//
// 테스트는 동일 fixture 빌더 (buildAutoYAML/buildManualYAML) 를 공유하여 중복을 줄인다.
package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// =============================================================================
// 테스트 헬퍼 (Phase F 공용)
// =============================================================================

// makeStoreAgentForRegistration 는 yaml options 로 UserStoreAgent 를 생성·시작한다.
// 테스트 종료 시 자동 stop 한다.
//
// @spec SPEC-STORE-003 v0.3.0
func makeStoreAgentForRegistration(t *testing.T, options map[string]any) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type:    "store",
			Options: options,
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err, "에이전트 부팅 성공해야 한다")
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	return ag.(*UserStoreAgent)
}

// =============================================================================
// A. registration_type 기본값 (RegistrationAuto)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M2 / Scenario 5
// TestRegistration_AutoMode_Default 는 yaml 에 registration_type 이 없을 때 default
// "auto" 가 적용되어 미등록 키 쓰기가 자동 등록과 함께 성공함을 검증한다.
func TestRegistration_AutoMode_Default(t *testing.T) {
	// registration_type 미지정 → default RegistrationAuto.
	options := map[string]any{
		"backend": "volatile",
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	require.NoError(t, store.Set(context.Background(), "any_key", "value"),
		"default RegistrationAuto 에서 미등록 키 쓰기 성공")

	snap := u.StaticKeysSnapshot()
	require.Contains(t, snap, "any_key")
	assert.Equal(t, SourceAuto, snap["any_key"].Source,
		"자동 등록 키의 Source 는 SourceAuto")
	assert.Equal(t, DataTypeString, snap["any_key"].DataType)
}

// =============================================================================
// B. Manual 모드 거부 정책 (Scenario 2 / M2 unwanted)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 2 / M2 unwanted
// TestRegistration_ManualMode_RejectsUnregisteredKey 는 manual 모드에서 미등록 키
// 쓰기가 ErrKeyNotAllowed 로 거부됨을 검증한다.
func TestRegistration_ManualMode_RejectsUnregisteredKey(t *testing.T) {
	options := map[string]any{
		"registration_type": "manual",
		"keys": []any{
			map[string]any{
				"key":         "indoor:1:room_temp",
				"data_type":   "float",
				"metric_type": "temperature",
				"tags":        map[string]any{"room": "1"},
			},
		},
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	// 등록된 키는 통과 (data_type=float 와 22.5 일치).
	require.NoError(t, store.Set(context.Background(), "indoor:1:room_temp", 22.5))

	// 미등록 키는 거부.
	err := store.Set(context.Background(), "outdoor:temperature", 35.0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyNotAllowed,
		"manual 모드 + 미등록 키 → ErrKeyNotAllowed")
}

// @spec SPEC-STORE-003 v0.3.0 / Scenario 2 / M2 unwanted
// TestRegistration_NoTraceOnRejection 은 manual 모드에서 거부된 쓰기가 엔트리에도
// 히스토리에도 흔적을 남기지 않음을 검증한다 ("Unwanted: 거부된 쓰기는 엔트리에도 히스토리에도
// 기록되지 않아야 한다").
func TestRegistration_NoTraceOnRejection(t *testing.T) {
	options := map[string]any{
		"registration_type": "manual",
		"max_history_size":  10,
		"keys": []any{
			map[string]any{
				"key":       "static_only",
				"data_type": "string",
			},
		},
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	// 미등록 키 쓰기 거부.
	err := store.Set(context.Background(), "rejected_key", "value")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyNotAllowed)

	// 엔트리 부재 확인.
	_, err = store.Get(context.Background(), "rejected_key")
	assert.ErrorIs(t, err, ErrKeyNotFound,
		"거부된 키는 엔트리에 존재하지 않아야 한다")

	// 키 목록에도 부재 확인 (히스토리 entry 도 자동 생성되지 않음).
	keys, err := store.Keys(context.Background(), "*")
	require.NoError(t, err)
	assert.NotContains(t, keys, "rejected_key",
		"거부된 키는 키 목록에도 부재해야 한다")

	// staticKeys 스냅샷에도 부재 확인 (manual 모드는 자동 등록을 차단).
	snap := u.StaticKeysSnapshot()
	assert.NotContains(t, snap, "rejected_key",
		"manual 모드 거부는 staticKeys 에도 흔적 없음")
}

// @spec SPEC-STORE-003 v0.3.0 / M6 state-driven
// TestRegistration_ManualMode_AllSourceManual 은 manual 모드에서 yaml 에 정의된
// 모든 키가 Source=SourceManual 로 표시됨을 검증한다.
func TestRegistration_ManualMode_AllSourceManual(t *testing.T) {
	options := map[string]any{
		"registration_type": "manual",
		"keys": []any{
			map[string]any{"key": "k1", "data_type": "int"},
			map[string]any{"key": "k2", "data_type": "float"},
			map[string]any{"key": "k3", "data_type": "string"},
		},
	}
	u := makeStoreAgentForRegistration(t, options)
	snap := u.StaticKeysSnapshot()
	require.Len(t, snap, 3)
	for _, k := range []string{"k1", "k2", "k3"} {
		assert.Equal(t, SourceManual, snap[k].Source,
			"manual 모드의 모든 yaml 키는 Source=SourceManual")
	}
}

// =============================================================================
// C. Auto 모드 등록 정책 (Scenario 3 / M6)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 3 / M6 / M7
// TestRegistration_AutoMode_RegistersNewKey 는 auto 모드에서 미등록 키 첫 쓰기 시
// Source=SourceAuto, MetricType="unknown", Tags={} 로 등록됨을 검증한다.
func TestRegistration_AutoMode_RegistersNewKey(t *testing.T) {
	options := map[string]any{
		"registration_type": "auto",
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	require.NoError(t, store.Set(context.Background(), "outdoor:temperature", 35.0))

	snap := u.StaticKeysSnapshot()
	require.Contains(t, snap, "outdoor:temperature")
	meta := snap["outdoor:temperature"]
	assert.Equal(t, SourceAuto, meta.Source, "Source 는 SourceAuto")
	assert.Equal(t, DataTypeFloat, meta.DataType, "data_type 은 추론된 float")
	assert.Equal(t, "unknown", meta.MetricType, "metric_type default 는 'unknown'")
	require.NotNil(t, meta.Tags, "Tags 는 nil 이 아닌 빈 맵")
	assert.Empty(t, meta.Tags, "auto 등록 키의 Tags 는 빈 맵")
}

// @spec SPEC-STORE-003 v0.3.0 / Scenario 3 / M6 state-driven
// TestRegistration_AutoMode_PreservesManualKeyMetadata 는 auto 모드에서 yaml 에 명시된
// manual 키의 메타데이터(Source=SourceManual, 명시한 metric_type, tags) 가 보존되며,
// auto 등록된 키와 동시에 staticKeys 에 공존함을 검증한다.
func TestRegistration_AutoMode_PreservesManualKeyMetadata(t *testing.T) {
	options := map[string]any{
		"registration_type": "auto",
		"keys": []any{
			map[string]any{
				"key":         "indoor:1:room_temp",
				"data_type":   "float",
				"metric_type": "temperature",
				"tags":        map[string]any{"room": "1"},
			},
		},
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	// 미등록 키 자동 등록.
	require.NoError(t, store.Set(context.Background(), "outdoor:temperature", 35.0))

	snap := u.StaticKeysSnapshot()
	require.Len(t, snap, 2, "manual 1개 + auto 1개 = 총 2개")

	// Manual 키 메타 보존.
	manual := snap["indoor:1:room_temp"]
	assert.Equal(t, SourceManual, manual.Source, "yaml 정의 키는 Source=SourceManual")
	assert.Equal(t, DataTypeFloat, manual.DataType)
	assert.Equal(t, "temperature", manual.MetricType)
	assert.Equal(t, "1", manual.Tags["room"])

	// Auto 키 메타.
	auto := snap["outdoor:temperature"]
	assert.Equal(t, SourceAuto, auto.Source, "자동 등록 키는 Source=SourceAuto")
	assert.Equal(t, DataTypeFloat, auto.DataType)
	assert.Equal(t, "unknown", auto.MetricType)
	assert.Empty(t, auto.Tags)
}

// =============================================================================
// D. Type pinning (Scenario 8 / M7 unwanted)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 8 / M7 unwanted
// TestRegistration_TypeMismatchAfterAutoRegister 는 auto 모드 자동 등록 후
// 다른 타입 쓰기가 ErrTypeMismatch 로 거부됨을 검증한다 (M7 type pinning).
func TestRegistration_TypeMismatchAfterAutoRegister(t *testing.T) {
	options := map[string]any{
		"registration_type": "auto",
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	// 첫 쓰기: int 자동 등록.
	require.NoError(t, store.Set(context.Background(), "sensor1", 42))

	// 후속 쓰기: 다른 타입 (string) 거부.
	err := store.Set(context.Background(), "sensor1", "broken")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypeMismatch,
		"등록된 후 다른 타입 쓰기는 ErrTypeMismatch")

	// 등록 메타는 변경되지 않음.
	snap := u.StaticKeysSnapshot()
	assert.Equal(t, DataTypeInt, snap["sensor1"].DataType,
		"data_type 은 첫 쓰기 시 결정된 int 로 영구 고정")
}

// @spec SPEC-STORE-003 v0.3.0 / Scenario 8 / M7 unwanted
// TestRegistration_TypePinning_Permanent 는 auto 등록 후 동일 키에 대한 모든 후속
// 타입 변경이 거부됨을 검증한다 (data_type pinning per M7).
func TestRegistration_TypePinning_Permanent(t *testing.T) {
	options := map[string]any{
		"registration_type": "auto",
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	// 첫 쓰기: int 등록.
	require.NoError(t, store.Set(context.Background(), "k", 1))
	snap := u.StaticKeysSnapshot()
	assert.Equal(t, DataTypeInt, snap["k"].DataType)

	// 동일 타입 후속 쓰기 → 통과.
	require.NoError(t, store.Set(context.Background(), "k", 100))

	// 다른 enum 타입 시도들 → 모두 ErrTypeMismatch.
	cases := []struct {
		name  string
		value any
	}{
		{"float", 3.14},
		{"string", "hello"},
		{"bool", true},
		{"bytes", []byte{0x01}},
		{"json", map[string]any{"a": 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := store.Set(context.Background(), "k", tc.value)
			assert.ErrorIs(t, err, ErrTypeMismatch,
				"%s 타입 후속 쓰기는 ErrTypeMismatch", tc.name)
		})
	}

	// 등록된 DataType 은 변경 없음.
	snap = u.StaticKeysSnapshot()
	assert.Equal(t, DataTypeInt, snap["k"].DataType,
		"data_type 은 첫 쓰기 후 영구 고정")
}

// =============================================================================
// E. Auto 모드 unsupported value (Edge case "Auto 모드 nil 값 쓰기")
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M7 unwanted
// TestRegistration_AutoNilValueRejected 는 auto 모드에서 nil 값 쓰기가
// ErrUnsupportedValueType 으로 거부되고 staticKeys 에 흔적이 남지 않음을 검증한다.
func TestRegistration_AutoNilValueRejected(t *testing.T) {
	options := map[string]any{
		"registration_type": "auto",
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	err := store.Set(context.Background(), "nil_key", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrUnsupportedValueType,
		"auto 모드 nil 값은 ErrUnsupportedValueType 으로 거부")

	// staticKeys 에 흔적 없음.
	snap := u.StaticKeysSnapshot()
	assert.NotContains(t, snap, "nil_key")
}

// =============================================================================
// F. 정책 전환 (Configure runtime change)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / M5 event-driven / Configure
// TestRegistration_PolicyChangeAtRuntime 은 SetRegistrationType 으로 auto → manual
// 전환 시 다음 동작을 검증한다.
//
//   - 이전에 auto 등록된 키는 staticKeys 에 보존된다 (entries 유지)
//   - 보존된 키에 대한 동일 타입 쓰기는 계속 통과한다 (등록되어 있으므로)
//   - 새로운 미등록 키 쓰기는 ErrKeyNotAllowed 로 거부된다 (manual 모드)
func TestRegistration_PolicyChangeAtRuntime(t *testing.T) {
	options := map[string]any{
		"registration_type": "auto",
	}
	u := makeStoreAgentForRegistration(t, options)
	store := u.inner.ForNamespace("default")

	// 1) auto 모드에서 미등록 키 자동 등록.
	require.NoError(t, store.Set(context.Background(), "auto_registered", 42))
	snap := u.StaticKeysSnapshot()
	require.Contains(t, snap, "auto_registered")
	assert.Equal(t, SourceAuto, snap["auto_registered"].Source)

	// 2) auto → manual 정책 전환.
	u.inner.SetRegistrationType(RegistrationManual)

	// 3) 이전에 등록된 키에 대한 동일 타입 쓰기 → 통과.
	require.NoError(t, store.Set(context.Background(), "auto_registered", 100),
		"이전에 auto 등록된 키는 manual 전환 후에도 동일 타입 쓰기 통과")

	// 4) 새 미등록 키 쓰기 → ErrKeyNotAllowed.
	err := store.Set(context.Background(), "new_key", "value")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyNotAllowed,
		"manual 전환 후 미등록 키 쓰기는 거부")

	// 5) 이전 등록 메타는 변경되지 않음.
	snap = u.StaticKeysSnapshot()
	require.Contains(t, snap, "auto_registered")
	assert.Equal(t, SourceAuto, snap["auto_registered"].Source,
		"정책 전환은 기존 등록 키의 Source 를 변경하지 않음")
}

// =============================================================================
// G. 등록 정책 fault — yaml 잔존 vs enum 위반
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Edge case "registration_type enum 외 값"
// TestRegistration_InvalidEnum_BootFailure 는 registration_type 이 enum 외 값일 때
// 부팅이 거부됨을 검증한다 (Edge case "registration_type enum 외 값").
func TestRegistration_InvalidEnum_BootFailure(t *testing.T) {
	cases := []string{"automatic", "MANUAL", "Auto", "  manual"}
	for _, val := range cases {
		t.Run(val, func(t *testing.T) {
			cfg := agent.AgentConfig{
				ID: "s1", Name: "store-a", Type: "store",
				Transport: agent.TransportConfig{
					Type: "store",
					Options: map[string]any{
						"registration_type": val,
					},
				},
			}
			_, err := NewUserStoreAgent(cfg)
			require.Error(t, err, "enum 외 값 %q 는 부팅 실패", val)
			assert.Contains(t, err.Error(), "registration_type",
				"에러 메시지는 registration_type 을 명시해야 한다")
		})
	}
}
