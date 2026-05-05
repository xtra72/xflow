// @spec SPEC-STORE-003 v0.3.0
//
// store_v3_integration_test.go — Phase F 통합 (end-to-end) 테스트.
//
// 본 파일은 yaml parser → UserStoreAgent 부팅 → ForNamespace().Set → State()/StaticKeysSnapshot()
// 까지 풀스택 흐름을 따라가며 SPEC v0.3.0 시나리오 1~10 의 핵심 동작을 회귀 검출 anchor 로 보존한다.
//
// 단위 테스트 (store_data_type_test.go), 마이그레이션 테스트 (store_static_keys_test.go),
// 등록 테스트 (store_registration_test.go), metric 테스트 (store_metric_type_test.go) 와
// 내용이 일부 중복되지만, 본 파일의 가치는 "여러 layer 가 한 흐름으로 정상 결합되는지"
// 를 단일 시나리오로 검증하는 것이다.
package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// =============================================================================
// Scenario 1: 정적 키 + data_type + tags 정상 동작 (M1, M3, M7, M8)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 1
// TestV3_Scenario1_NormalFlow 는 yaml 정의 → 부팅 → 정적 키 쓰기 → state.entries 조회까지
// 전체 흐름을 검증한다.
func TestV3_Scenario1_NormalFlow(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "auto",
				"keys": []any{
					map[string]any{
						"key":         "indoor:1:room_temp",
						"data_type":   "float",
						"metric_type": "temperature",
						"tags":        map[string]any{"room": "1"},
					},
				},
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	// SET indoor:1:room_temp = 22.5 (float64).
	store := u.inner.ForNamespace("default")
	require.NoError(t, store.Set(context.Background(), "indoor:1:room_temp", 22.5))

	// State() 의 entries 에서 해당 키 + tags + value 검증.
	state := u.State()
	entries, ok := state["entries"].([]map[string]any)
	require.True(t, ok)
	var found map[string]any
	for _, e := range entries {
		if e["key"] == "indoor:1:room_temp" {
			found = e
			break
		}
	}
	require.NotNil(t, found, "indoor:1:room_temp 엔트리가 존재해야 한다")
	assert.Equal(t, 22.5, found["value"])
	tags, ok := found["tags"].(map[string]string)
	require.True(t, ok)
	assert.Equal(t, "1", tags["room"])

	// StaticKeysSnapshot 으로 메타 검증 (M9 응답에 노출되는 모든 필드).
	snap := u.StaticKeysSnapshot()
	meta := snap["indoor:1:room_temp"]
	assert.Equal(t, DataTypeFloat, meta.DataType)
	assert.Equal(t, "temperature", meta.MetricType)
	assert.Equal(t, SourceManual, meta.Source)
	assert.Equal(t, "1", meta.Tags["room"])
}

// =============================================================================
// Scenario 3: Auto 모드 미등록 키 자동 등록 + 공존 (M2, M6, M7)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 3
// TestV3_Scenario3_AutoModeAndCoexistence 는 yaml manual 키 + 런타임 자동 등록 키가
// 동시에 staticKeys 에 존재하며 각자 올바른 Source 로 표시됨을 검증한다.
func TestV3_Scenario3_AutoModeAndCoexistence(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "auto",
				"keys": []any{
					map[string]any{
						"key":         "indoor:1:room_temp",
						"data_type":   "float",
						"metric_type": "temperature",
						"tags":        map[string]any{"room": "1"},
					},
				},
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	// 미등록 키 자동 등록 (auto 모드).
	store := u.inner.ForNamespace("default")
	require.NoError(t, store.Set(context.Background(), "outdoor:temperature", 35.0))

	snap := u.StaticKeysSnapshot()
	require.Len(t, snap, 2)

	// Manual 키 보존.
	manual := snap["indoor:1:room_temp"]
	assert.Equal(t, SourceManual, manual.Source)
	assert.Equal(t, "temperature", manual.MetricType)
	assert.Equal(t, "1", manual.Tags["room"])

	// Auto 키 default.
	auto := snap["outdoor:temperature"]
	assert.Equal(t, SourceAuto, auto.Source)
	assert.Equal(t, DataTypeFloat, auto.DataType)
	assert.Equal(t, "unknown", auto.MetricType)
	assert.Empty(t, auto.Tags)
}

// =============================================================================
// Scenario 7: Manual 모드 명시 data_type 검증 (M7)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 7
// TestV3_Scenario7_ManualModeStrictType 는 manual 모드에서 다음을 검증한다:
//   - 명시 data_type 과 일치하는 쓰기는 성공
//   - 불일치는 ErrTypeMismatch 로 거부 + 흔적 없음
func TestV3_Scenario7_ManualModeStrictType(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "manual",
				"max_history_size":  10,
				"keys": []any{
					map[string]any{
						"key":         "indoor:1:room_temp",
						"data_type":   "float",
						"metric_type": "temperature",
						"tags":        map[string]any{"room": "1"},
					},
				},
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	store := u.inner.ForNamespace("default")

	// 정상 케이스: float 일치 → 성공.
	require.NoError(t, store.Set(context.Background(), "indoor:1:room_temp", 22.5))

	// 타입 불일치: string → ErrTypeMismatch.
	err = store.Set(context.Background(), "indoor:1:room_temp", "hot")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypeMismatch)

	// 기존 값은 보존되어야 한다 (거부된 쓰기는 흔적 없음).
	entry, gerr := store.Get(context.Background(), "indoor:1:room_temp")
	require.NoError(t, gerr)
	assert.Equal(t, 22.5, entry.Value, "거부된 쓰기는 기존 값을 변경하지 않음")
}

// =============================================================================
// Scenario 8: Auto 모드 data_type 추론 + type pinning (M7)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 8
// TestV3_Scenario8_AutoModeTypePinning 는 auto 모드에서 첫 쓰기 시 추론된 data_type 이
// 영구 고정되어 후속 다른 타입 쓰기는 거부됨을 검증한다.
func TestV3_Scenario8_AutoModeTypePinning(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "auto",
				// keys 비움 (yaml 정적 키 없음).
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	store := u.inner.ForNamespace("default")

	// 첫 쓰기: int 자동 등록.
	require.NoError(t, store.Set(context.Background(), "sensor1", 42))
	snap := u.StaticKeysSnapshot()
	require.Contains(t, snap, "sensor1")
	assert.Equal(t, DataTypeInt, snap["sensor1"].DataType)
	assert.Equal(t, SourceAuto, snap["sensor1"].Source)
	assert.Equal(t, "unknown", snap["sensor1"].MetricType)

	// 동일 타입 후속 쓰기: 통과.
	require.NoError(t, store.Set(context.Background(), "sensor1", 100))

	// 다른 타입 후속 쓰기: ErrTypeMismatch.
	err = store.Set(context.Background(), "sensor1", "broken")
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTypeMismatch)

	// 등록 메타 변경 없음.
	snap = u.StaticKeysSnapshot()
	assert.Equal(t, DataTypeInt, snap["sensor1"].DataType,
		"data_type 은 첫 쓰기 시 결정된 int 로 영구 고정")
}

// =============================================================================
// Scenario 9: metric_type default + 필터 (M8, M9)
// =============================================================================

// @spec SPEC-STORE-003 v0.3.0 / Scenario 9
// TestV3_Scenario9_MetricTypeFilter 는 yaml 에 다양한 metric_type 을 정의한 뒤
// StaticKeysSnapshot 으로 메타가 정확히 노출되는지 검증한다 (handler 필터는 별도
// store_query_listkeys_filters_test.go 에서 검증).
//
// 본 테스트의 가치는 yaml → parser → staticKeys 경로의 metric_type 보존을 통합
// 시점에서 한 번 더 확인하는 것이다.
func TestV3_Scenario9_MetricTypeFilter(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "manual",
				"keys": []any{
					map[string]any{
						"key":         "k1",
						"data_type":   "float",
						"metric_type": "temperature",
					},
					map[string]any{
						"key":       "k2",
						"data_type": "float",
						// metric_type 누락 → default "unknown"
					},
					map[string]any{
						"key":         "k3",
						"data_type":   "float",
						"metric_type": "humidity",
					},
				},
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	snap := u.StaticKeysSnapshot()
	require.Len(t, snap, 3)

	// metric_type 정확 보존 + default 적용 확인.
	assert.Equal(t, "temperature", snap["k1"].MetricType)
	assert.Equal(t, "unknown", snap["k2"].MetricType, "yaml 누락 시 default 'unknown'")
	assert.Equal(t, "humidity", snap["k3"].MetricType)
}

// =============================================================================
// Scenario 10: API 응답 객체 배열 (M9) — 핸들러 측에서 별도 검증
// =============================================================================

// 본 시나리오는 핸들러 응답 형상 (객체 배열 + 5필드 + 알파벳 정렬) 의 검증이며,
// store_query_listkeys_filters_test.go 의 TestListKeys_AllFiveFieldsPresent /
// TestListKeys_Sorting_AlphabeticalAscending / TestListKeys_AutoRegistered_AppearWithDefaults /
// TestListKeys_Filter_AND_All4Axes 가 그 역할을 분담한다.
// 본 파일에서는 별도 테스트를 추가하지 않고 위 핸들러 테스트가 검증을 수행한다.

// =============================================================================
// Scenario 6: v0.2.0 yaml 마이그레이션 실패 (M5 BREAKING) — 별도 파일에서 검증
// =============================================================================

// 본 시나리오는 store_static_keys_test.go 의 TestParseStoreConfig_AllowDynamicKeys_RemovedInV030
// 가 검증한다. 본 파일에서 별도로 추가하지 않는다.
