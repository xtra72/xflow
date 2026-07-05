// @spec SPEC-STORE-003 v0.3.0
// 본 파일은 v0.2.0 의 정적 키 테스트를 v0.3.0 모델 (registration_type/data_type/metric_type) 로
// 마이그레이션한 결과이다. v0.2.0 의 검증된 동작 (M1, M2, M4, M5) 은 그대로 보존되며,
// allow_dynamic_keys 의 경우 부팅 실패 동작 (M5/Scenario 6) 으로 진화한다.
package system

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// parseStoreConfig: 하위호환 (기존 설정 그대로 동작)
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 의 동일 테스트와 의도가 동일하다: 정책 필드 (registrationType, staticKeys) 가 yaml 에
// 명시되지 않았을 때 기본값이 유지되는지 확인한다. v0.3.0 에서 기본값은 RegistrationAuto
// (모든 키 허용 + 첫 쓰기 시 data_type 자동 추론) + staticKeys=nil 이다.
func TestParseStoreConfig_기존필드만_하위호환(t *testing.T) {
	cfg := agent.AgentConfig{
		ID:   "s1",
		Name: "store-a",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":          "volatile",
				"max_history_size": 500,
				"history_ttl":      "1h",
				"max_key_length":   256,
				"scan_interval":    "15s",
			},
		},
	}

	opts, err := parseStoreConfig(cfg)
	require.NoError(t, err)

	// 옵션 적용 후 기본값(registrationType=Auto, staticKeys=nil) 유지 확인.
	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}
	assert.Equal(t, RegistrationAuto, sc.registrationType,
		"registrationType 은 설정 미지정 시 기본 RegistrationAuto")
	assert.Nil(t, sc.staticKeys, "staticKeys는 설정 미지정 시 nil")
}

// ---------------------------------------------------------------------------
// parseStoreConfig: registration_type
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 의 TestParseStoreConfig_allow_dynamic_keys_false 가 진화한 형태. v0.2.0 에서는
// allow_dynamic_keys=false 가 manual 모드를 활성화했지만, v0.3.0 에서는 registration_type=manual
// 로 동일한 의미를 표현한다.
func TestParseStoreConfig_registration_type_manual(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"registration_type": "manual",
				// manual 모드에서는 모든 keys 엔트리에 data_type 이 필수이지만,
				// 본 테스트는 registration_type 만 검증하므로 keys 를 생략한다.
			},
		},
	}
	opts, err := parseStoreConfig(cfg)
	require.NoError(t, err)
	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}
	assert.Equal(t, RegistrationManual, sc.registrationType)
}

// @spec SPEC-STORE-003 v0.3.0
// registration_type 의 enum 위반은 명시적 에러로 거부되어야 한다.
func TestParseStoreConfig_registration_type_invalid_enum(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"registration_type": "neither", // "manual" / "auto" 외 값
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registration_type")
}

// @spec SPEC-STORE-003 v0.3.0
// registration_type 에 string 이 아닌 타입을 넣으면 명시적 에러가 발생해야 한다.
func TestParseStoreConfig_registration_type_타입오류(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"registration_type": 42, // int 는 허용되지 않는다
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registration_type")
}

// ---------------------------------------------------------------------------
// parseStoreConfig: keys (정적 키 + 태그)
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 다중 정적 키 + 태그가 정확히 파싱된다. v0.3.0 추가:
// data_type 명시는 auto 모드에서 optional 이지만, 본 테스트는 fixture 의 명시성을 위해
// 항상 명시한다. 태그는 변경 없이 그대로 전달되어야 한다.
func TestParseStoreConfig_keys_정상케이스(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{
						"key":         "indoor:1:room_temp",
						"data_type":   "float",
						"metric_type": "temperature",
						"tags": map[string]any{
							"room": "1",
							"type": "temperature",
						},
					},
					map[string]any{
						"key":         "outdoor:temperature",
						"data_type":   "float",
						"metric_type": "temperature",
						"tags": map[string]any{
							"location": "outside",
							"type":     "temperature",
						},
					},
				},
			},
		},
	}
	opts, err := parseStoreConfig(cfg)
	require.NoError(t, err)

	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}

	require.Len(t, sc.staticKeys, 2)
	assert.Equal(t, "1", sc.staticKeys["indoor:1:room_temp"].Tags["room"])
	assert.Equal(t, "temperature", sc.staticKeys["indoor:1:room_temp"].Tags["type"])
	assert.Equal(t, "outside", sc.staticKeys["outdoor:temperature"].Tags["location"])
	// v0.3.0 추가 검증: data_type 과 source 도 함께 노출되어야 한다.
	assert.Equal(t, DataTypeFloat, sc.staticKeys["indoor:1:room_temp"].DataType)
	assert.Equal(t, "temperature", sc.staticKeys["indoor:1:room_temp"].MetricType)
	assert.Equal(t, SourceManual, sc.staticKeys["indoor:1:room_temp"].Source)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: tags 미지정은 빈 맵으로 정규화되어야 한다.
// auto 모드 + data_type 미명시는 v0.3.0 에서 허용되지만, 첫 쓰기까지 DataType="" 으로 둔다.
// 본 테스트는 tags 만 검증하므로 data_type 을 의도적으로 명시하지 않는다.
func TestParseStoreConfig_keys_태그없음_허용(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{"key": "only_key", "data_type": "string"},
				},
			},
		},
	}
	opts, err := parseStoreConfig(cfg)
	require.NoError(t, err)
	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}
	require.Contains(t, sc.staticKeys, "only_key")
	assert.Empty(t, sc.staticKeys["only_key"].Tags, "tags 미지정은 빈 맵")
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 동일 key 가 두 번 등장하면 ErrDuplicateStaticKey.
func TestParseStoreConfig_keys_중복_ErrDuplicateStaticKey(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{"key": "dup", "data_type": "string"},
					map[string]any{"key": "dup", "data_type": "string"},
				},
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateStaticKey)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 태그 key 가 정규식 ^[a-zA-Z0-9_-]+$ 위반 시 ErrInvalidTagKey.
func TestParseStoreConfig_keys_태그key_정규식위반_ErrInvalidTagKey(t *testing.T) {
	cases := []struct {
		name   string
		tagKey string
	}{
		{"dot", "room.1"},
		{"space", "room 1"},
		{"colon", "room:sub"},
		{"slash", "room/sub"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := agent.AgentConfig{
				Transport: agent.TransportConfig{
					Options: map[string]any{
						"keys": []any{
							map[string]any{
								"key":       "k",
								"data_type": "string",
								"tags":      map[string]any{tc.tagKey: "v"},
							},
						},
					},
				},
			}
			_, err := parseStoreConfig(cfg)
			require.Error(t, err)
			assert.ErrorIs(t, err, ErrInvalidTagKey)
		})
	}
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 태그 value 가 string 이 아니면 명시적 에러.
func TestParseStoreConfig_keys_태그value_비문자열_에러(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{
						"key":       "k",
						"data_type": "string",
						"tags":      map[string]any{"n": 42}, // int 는 허용되지 않음
					},
				},
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be string")
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: key 필드 누락 시 명시적 에러.
func TestParseStoreConfig_keys_key_누락_에러(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{"tags": map[string]any{"t": "v"}},
				},
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing required field 'key'")
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 빈 keys 배열은 허용되며 staticKeys 는 빈 맵이다.
func TestParseStoreConfig_keys_빈배열_허용(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{},
			},
		},
	}
	opts, err := parseStoreConfig(cfg)
	require.NoError(t, err)
	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}
	assert.Empty(t, sc.staticKeys, "빈 배열은 빈 맵")
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: keys 가 list 타입이 아니면 명시적 에러.
func TestParseStoreConfig_keys_타입_비리스트_에러(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": "not-a-list",
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a list")
}

// ---------------------------------------------------------------------------
// 쓰기 경로: registration_type=manual 일 때 정적 키 밖의 쓰기 거부
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도 (Scenario 2): manual 모드에서 미등록 키 쓰기는 ErrKeyNotAllowed 로 거부되며
// 엔트리/히스토리에 흔적이 남지 않는다. v0.3.0 에서는 WithAllowDynamicKeys(false) 가
// WithRegistrationType(RegistrationManual) 로, WithStaticKeys 의 value 가 StaticKeyMeta 로
// 진화했다.
func TestWriteGate_strict모드_미등록키_거부(t *testing.T) {
	sa := NewStoreAgent(
		WithRegistrationType(RegistrationManual),
		WithStaticKeys(map[string]StaticKeyMeta{
			"indoor:1:room_temp": {
				DataType:   DataTypeFloat,
				MetricType: "temperature",
				Tags:       map[string]string{"room": "1", "type": "temperature"},
				Source:     SourceManual,
			},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	store := sa.ForNamespace("default")

	// 등록된 키는 통과 (data_type=float 와 22.5 일치).
	require.NoError(t, store.Set(context.Background(), "indoor:1:room_temp", 22.5))

	// 미등록 키는 ErrKeyNotAllowed
	err := store.Set(context.Background(), "outdoor:temperature", 35.0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyNotAllowed)

	// 히스토리에도 기록되지 않아야 한다 (키 자체가 존재하지 않음).
	keys, err := store.Keys(context.Background(), "*")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"indoor:1:room_temp"}, keys)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: SetWithTTL 도 manual 모드의 거부 정책을 동일하게 적용해야 한다.
func TestWriteGate_strict모드_SetWithTTL_도_거부(t *testing.T) {
	sa := NewStoreAgent(
		WithRegistrationType(RegistrationManual),
		WithStaticKeys(map[string]StaticKeyMeta{
			"k": {
				DataType:   DataTypeString,
				MetricType: "unknown",
				Tags:       map[string]string{},
				Source:     SourceManual,
			},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	store := sa.ForNamespace("default")

	// 미등록 키 SetWithTTL 도 거부되어야 한다.
	err := store.SetWithTTL(context.Background(), "not_allowed", 1, 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyNotAllowed)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: auto 모드 (permissive) 에서는 미등록 키 쓰기가 자동 등록과 함께
// 성공한다 (Scenario 3 / M6). 등록된 키도 data_type 일치 하에 성공한다.
func TestWriteGate_permissive모드_미등록키_성공_태그없음(t *testing.T) {
	sa := NewStoreAgent(
		// 기본 RegistrationAuto
		WithStaticKeys(map[string]StaticKeyMeta{
			"known": {
				DataType:   DataTypeString,
				MetricType: "x",
				Tags:       map[string]string{"type": "x"},
				Source:     SourceManual,
			},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	store := sa.ForNamespace("default")

	// 미등록 키 성공 (auto 등록).
	require.NoError(t, store.Set(context.Background(), "dynamic", "value"))

	// 등록된 키도 성공 (data_type=string 과 "value" 일치).
	require.NoError(t, store.Set(context.Background(), "known", "value"))

	keys, err := store.Keys(context.Background(), "*")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dynamic", "known"}, keys)
}

// ---------------------------------------------------------------------------
// TagsFor / StaticTagsFor / StaticKeyTags
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: agentStore.TagsFor 는 정적 키의 태그 복사본을 반환하며, 반환 맵 변조가
// 내부 상태에 영향을 주지 않는다.
func TestAgentStore_TagsFor_정적키_태그반환(t *testing.T) {
	sa := NewStoreAgent(
		WithStaticKeys(map[string]StaticKeyMeta{
			"k1": {
				DataType:   DataTypeFloat,
				MetricType: "temperature",
				Tags:       map[string]string{"room": "1", "type": "temperature"},
				Source:     SourceManual,
			},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}
	tags := as.TagsFor("k1")
	assert.Equal(t, map[string]string{"room": "1", "type": "temperature"}, tags)

	// 반환 맵은 복사본: 수정해도 에이전트 내부에 영향 없음.
	tags["room"] = "999"
	assert.Equal(t, "1", sa.StaticTagsFor("k1")["room"])
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 정적 키 목록에 없는 key 의 TagsFor 는 빈 맵 (nil 아님).
func TestAgentStore_TagsFor_동적키_빈맵(t *testing.T) {
	sa := NewStoreAgent(
		WithStaticKeys(map[string]StaticKeyMeta{
			"k1": {
				DataType:   DataTypeString,
				MetricType: "unknown",
				Tags:       map[string]string{"a": "b"},
				Source:     SourceManual,
			},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}
	assert.Equal(t, map[string]string{}, as.TagsFor("unknown"))
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: StaticKeyTags 는 (사용자 키 → 태그 맵) 복사본을 반환하며, 변조가
// 내부 상태에 영향을 주지 않는다. v0.3.0 에서 staticKeys 의 value 가 StaticKeyMeta 로
// 진화했지만 StaticKeyTags shim 은 tags-only view 를 빌드하므로 반환 형식은 v0.2.0 과 동일.
func TestStoreAgent_StaticKeyTags_복사본반환(t *testing.T) {
	sa := NewStoreAgent(
		WithStaticKeys(map[string]StaticKeyMeta{
			"k1": {
				DataType:   DataTypeString,
				MetricType: "unknown",
				Tags:       map[string]string{"room": "1"},
				Source:     SourceManual,
			},
			"k2": {
				DataType:   DataTypeString,
				MetricType: "unknown",
				Tags:       map[string]string{"room": "2"},
				Source:     SourceManual,
			},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	all := sa.StaticKeyTags()
	require.Len(t, all, 2)

	// 반환 맵 변조가 원본에 반영되지 않음을 확인.
	all["k1"]["room"] = "mutated"
	assert.Equal(t, "1", sa.StaticKeyTags()["k1"]["room"])
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 정적 키가 없으면 StaticKeyTags 는 nil 이 아닌 빈 맵을 반환한다.
func TestStoreAgent_StaticKeyTags_정적키없음_빈맵(t *testing.T) {
	sa := NewStoreAgent()
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	all := sa.StaticKeyTags()
	assert.Empty(t, all)
	assert.NotNil(t, all, "nil 이 아니라 빈 맵이어야 한다")
}

// ---------------------------------------------------------------------------
// UserStoreAgent 통합 경로: KeyTags / StaticTagPairs
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// newStaticKeysAgent 는 정적 키 2개 (indoor:1:room_temp, outdoor:temperature) 가 등록된
// UserStoreAgent 를 만든다. autoMode 가 true 면 RegistrationAuto, false 면 RegistrationManual.
// v0.3.0 마이그레이션: data_type=float 을 모든 키에 명시한다 (manual 모드 필수, auto 모드
// optional 이지만 일관성을 위해 명시).
func newStaticKeysAgent(t *testing.T, autoMode bool) *UserStoreAgent {
	t.Helper()
	regType := "auto"
	if !autoMode {
		regType = "manual"
	}
	cfg := agent.AgentConfig{
		ID:   "s1",
		Name: "store-a",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":           "volatile",
				"registration_type": regType,
				"keys": []any{
					map[string]any{
						"key":         "indoor:1:room_temp",
						"data_type":   "float",
						"metric_type": "temperature",
						"tags": map[string]any{
							"room": "1",
							"type": "temperature",
						},
					},
					map[string]any{
						"key":         "outdoor:temperature",
						"data_type":   "float",
						"metric_type": "temperature",
						"tags": map[string]any{
							"location": "outside",
							"type":     "temperature",
						},
					},
				},
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	return ag.(*UserStoreAgent)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: KeyTags 는 정적 키 2개의 태그를 모두 반환한다.
func TestUserStoreAgent_KeyTags_정적키_반환(t *testing.T) {
	a := newStaticKeysAgent(t, true)
	out, err := a.KeyTags(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "1", out["indoor:1:room_temp"]["room"])
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: StaticTagPairs 는 모든 정적 키의 태그를 (key → 정렬된 unique value)
// 형태로 집계한다.
func TestUserStoreAgent_StaticTagPairs_정렬(t *testing.T) {
	a := newStaticKeysAgent(t, true)
	pairs := a.StaticTagPairs()

	assert.Equal(t, []string{"1"}, pairs["room"])
	assert.Equal(t, []string{"outside"}, pairs["location"])
	assert.Equal(t, []string{"temperature"}, pairs["type"])
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 정적 키가 없으면 StaticTagPairs 는 빈 맵을 반환한다.
func TestUserStoreAgent_StaticTagPairs_정적키없음_빈맵(t *testing.T) {
	// 정적 키 없음.
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{Type: "store", Options: map[string]any{}},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)
	assert.Empty(t, u.StaticTagPairs())
}

// ---------------------------------------------------------------------------
// UserStoreAgent.State(): 정적 키 엔트리에 tags 첨부 확인
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.4.0 (모든 엔트리 type/tags 노출 정책에 따른 갱신)
// State() 의 entries 는 정적 키와 동적 키 모두에 metric_type 과 tags 를 포함한다.
// 정적 키는 yaml 에 정의된 tags/metric_type 을, 동적(자동 등록) 키는 기본값
// metric_type="unknown" + 빈 tags 객체를 노출한다. 이로써 프론트가 모든 엔트리를
// 일관되게 필터/표시할 수 있다 (v0.3.0 의 "동적 키 tags 생략" 동작에서 변경).
func TestUserStoreAgent_State_모든엔트리_type_tags_포함(t *testing.T) {
	a := newStaticKeysAgent(t, true)

	// 정적 키 + 동적 키 각각 쓰기.
	store := a.inner.ForNamespace("default")
	require.NoError(t, store.Set(context.Background(), "indoor:1:room_temp", 22.5))
	require.NoError(t, store.Set(context.Background(), "dynamic_key", "x"))

	state := a.State()
	entries, ok := state["entries"].([]map[string]any)
	require.True(t, ok)

	var found, foundDyn map[string]any
	for _, e := range entries {
		switch e["key"] {
		case "indoor:1:room_temp":
			found = e
		case "dynamic_key":
			foundDyn = e
		}
	}

	// 정적 키: yaml tags + metric_type 노출.
	require.NotNil(t, found, "정적 키 엔트리가 존재해야 한다")
	tags, ok := found["tags"].(map[string]string)
	require.True(t, ok, "정적 키 엔트리는 tags 맵을 포함해야 한다")
	assert.Equal(t, "1", tags["room"])
	assert.Equal(t, "temperature", tags["type"])
	_, hasMetric := found["metric_type"]
	assert.True(t, hasMetric, "정적 키 엔트리는 metric_type 을 포함해야 한다")

	// 동적 키: metric_type="unknown" + 빈 tags 객체 노출.
	require.NotNil(t, foundDyn, "동적 키 엔트리가 존재해야 한다")
	dynTags, ok := foundDyn["tags"].(map[string]string)
	require.True(t, ok, "동적 키 엔트리도 tags 맵(빈 객체)을 포함해야 한다")
	assert.Empty(t, dynTags, "동적 키의 tags 는 빈 맵")
	assert.Equal(t, "unknown", foundDyn["metric_type"],
		"동적 키의 metric_type 은 'unknown'")
}

// ---------------------------------------------------------------------------
// UserStoreAgent.Configure(): 정책 필드 런타임 반영
// ---------------------------------------------------------------------------
// @spec SPEC-STORE-003 v0.3.0
// Configure 는 registration_type / keys (정적 키 + DataType + MetricType + Tags) 변경을
// inner StoreAgent 에 런타임으로 전파해야 한다. 운영 필드 (scan_interval 등) 는 재시작 시에만
// 반영된다.

// @spec SPEC-STORE-003 v0.3.0
// buildStoreConfig 는 테스트용 AgentConfig 빌더다. v0.2.0 의 (allowDynamic, keys) 시그니처에서
// (registrationType, keys) 로 진화했다. keys 의 각 엔트리는 호출자가 data_type 을 명시해야
// 한다 (manual 모드에서는 필수, auto 모드에서는 일관성 위해 권장).
func buildStoreConfig(rt RegistrationType, keys []any) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "s1",
		Name: "store-a",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":           "volatile",
				"registration_type": string(rt),
				"keys":              keys,
			},
		},
	}
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도 (Scenario 4 변형): Configure 로 permissive (auto) → strict (manual) 전환 시
// 미등록 동적 키 쓰기가 거부되고 정적 키 쓰기는 통과한다.
func TestUserStoreAgent_Configure_UpdatesPolicy_permissive_to_strict(t *testing.T) {
	// 초기: registration_type=auto, 정적 키 없음.
	initial := agent.AgentConfig{
		ID:   "s1",
		Name: "store-a",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":           "volatile",
				"registration_type": "auto",
			},
		},
	}
	ag, err := NewUserStoreAgent(initial)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	store := u.inner.ForNamespace("default")

	// 초기 상태: 동적 키 쓰기 허용 확인 (auto 등록 + DataType=string 으로 추론됨).
	require.NoError(t, store.Set(context.Background(), "dyn:1", "v1"),
		"초기 permissive 모드에서 동적 키 쓰기 성공해야 한다")

	// Configure 호출로 manual 모드 + 정적 키 1개 주입 (data_type=float).
	newCfg := buildStoreConfig(RegistrationManual, []any{
		map[string]any{
			"key":         "static:1",
			"data_type":   "float",
			"metric_type": "temperature",
			"tags":        map[string]any{"room": "1", "type": "temperature"},
		},
	})
	require.NoError(t, u.Configure(newCfg))

	// 이제 동적 키 쓰기 거부되어야 한다.
	err = store.Set(context.Background(), "dyn:2", "v2")
	require.Error(t, err, "Configure 이후 manual 모드에서 동적 키 쓰기는 거부되어야 한다")
	assert.ErrorIs(t, err, ErrKeyNotAllowed)

	// 정적 키 쓰기는 성공해야 한다 (data_type=float 와 22.5 일치).
	require.NoError(t, store.Set(context.Background(), "static:1", 22.5),
		"Configure 이후 정적 키 쓰기 성공해야 한다")

	// TagsFor 가 새 태그를 반환해야 한다.
	tags := u.inner.StaticTagsFor("static:1")
	assert.Equal(t, "1", tags["room"])
	assert.Equal(t, "temperature", tags["type"])
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도 (Scenario 4 역방향): Configure 로 strict (manual) → permissive (auto) 전환 시
// 미등록 동적 키 쓰기가 허용되어야 한다.
func TestUserStoreAgent_Configure_UpdatesPolicy_strict_to_permissive(t *testing.T) {
	// 초기: registration_type=manual, 정적 키 1개 (data_type=string 명시).
	initial := buildStoreConfig(RegistrationManual, []any{
		map[string]any{
			"key":       "static:A",
			"data_type": "string",
			"tags":      map[string]any{"v": "1"},
		},
	})
	ag, err := NewUserStoreAgent(initial)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	store := u.inner.ForNamespace("default")

	// 초기 상태: 동적 키 쓰기 거부 확인.
	err = store.Set(context.Background(), "dyn:1", "v1")
	require.Error(t, err, "초기 manual 모드에서 동적 키 쓰기는 거부되어야 한다")
	assert.ErrorIs(t, err, ErrKeyNotAllowed)

	// Configure 호출로 auto 모드 전환.
	newCfg := buildStoreConfig(RegistrationAuto, []any{
		map[string]any{
			"key":       "static:A",
			"data_type": "string",
			"tags":      map[string]any{"v": "1"},
		},
	})
	require.NoError(t, u.Configure(newCfg))

	// 이제 동적 키 쓰기 허용되어야 한다 (auto 등록).
	require.NoError(t, store.Set(context.Background(), "dyn:2", "v2"),
		"Configure 이후 auto 모드에서 동적 키 쓰기 성공해야 한다")
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 정적 키 mutation (기존 키 태그 변경 + 신규 키 추가) 이 Configure 로
// 런타임에 반영되며, 이미 저장된 값은 보존된다.
func TestUserStoreAgent_Configure_UpdatesPolicy_static_keys_mutation(t *testing.T) {
	// 초기: 정적 키 A={room:1}, data_type=string.
	initial := buildStoreConfig(RegistrationAuto, []any{
		map[string]any{
			"key":       "A",
			"data_type": "string",
			"tags":      map[string]any{"room": "1"},
		},
	})
	ag, err := NewUserStoreAgent(initial)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	store := u.inner.ForNamespace("default")

	// 초기 상태: A 에 값 쓰기 (이후 보존 확인용).
	require.NoError(t, store.Set(context.Background(), "A", "initial-value"))
	assert.Equal(t, "1", u.inner.StaticTagsFor("A")["room"])

	// Configure 로 A 태그 변경 + B 신규 추가.
	newCfg := buildStoreConfig(RegistrationAuto, []any{
		map[string]any{
			"key":       "A",
			"data_type": "string",
			"tags":      map[string]any{"room": "2"},
		},
		map[string]any{
			"key":       "B",
			"data_type": "string",
			"tags":      map[string]any{"room": "3"},
		},
	})
	require.NoError(t, u.Configure(newCfg))

	// A 의 태그가 업데이트되었는지 확인.
	assert.Equal(t, "2", u.inner.StaticTagsFor("A")["room"],
		"Configure 이후 A 의 태그가 업데이트되어야 한다")

	// B 가 신규 정적 키로 인식되는지 확인.
	assert.Equal(t, "3", u.inner.StaticTagsFor("B")["room"],
		"Configure 이후 B 가 정적 키로 추가되어야 한다")

	// A 에 저장된 기존 값은 유지되어야 한다 (정책 변경은 값 삭제와 무관).
	entry, err := store.Get(context.Background(), "A")
	require.NoError(t, err)
	assert.Equal(t, "initial-value", entry.Value,
		"정책 변경은 기존 저장 값을 삭제하지 않아야 한다")
}

// ---------------------------------------------------------------------------
// 에러 센티넬 정의 자체 확인 (TRUST Readable 목적).
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 핵심 에러 센티넬이 정의되어 있고 errors.Is 비교 가능하다.
func TestStoreErrors_정의(t *testing.T) {
	assert.NotNil(t, ErrKeyNotAllowed)
	assert.NotNil(t, ErrDuplicateStaticKey)
	assert.NotNil(t, ErrInvalidTagKey)
	// v0.3.0 신규 에러 센티넬도 함께 검증.
	assert.NotNil(t, ErrInvalidDataType)
	assert.NotNil(t, ErrInvalidMetricType)
	assert.NotNil(t, ErrTypeMismatch)
	assert.NotNil(t, ErrUnsupportedValueType)

	// 센티넬 에러는 errors.Is 로 비교 가능해야 한다.
	wrapped := errors.New("wrapping: " + ErrKeyNotAllowed.Error())
	assert.NotErrorIs(t, wrapped, ErrKeyNotAllowed, "문자열 래핑은 Is 와 동일하지 않다")
}

// ---------------------------------------------------------------------------
// @spec SPEC-STORE-003 v0.3.0: UserStoreAgent reset 메서드 (ClearHistory / DeleteEntry / IsStaticKey)
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: IsStaticKey 는 정적 키 정의를 정확히 반영한다.
func TestUserStoreAgent_IsStaticKey(t *testing.T) {
	a := newStaticKeysAgent(t, true)
	assert.True(t, a.IsStaticKey("indoor:1:room_temp"))
	assert.True(t, a.IsStaticKey("outdoor:temperature"))
	assert.False(t, a.IsStaticKey("dynamic_key"))
	assert.False(t, a.IsStaticKey(""))
}

// @spec SPEC-STORE-003 v0.3.0
// newStaticKeysAgentWithHistory 는 정적 키 + 히스토리 활성화된 UserStoreAgent 를 만든다.
// reset 메서드 테스트에서 사용된다.
func newStaticKeysAgentWithHistory(t *testing.T) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":           "volatile",
				"registration_type": "auto",
				"max_history_size":  10,
				"keys": []any{
					map[string]any{
						"key":         "indoor:1:room_temp",
						"data_type":   "int",
						"metric_type": "temperature",
						"tags":        map[string]any{"room": "1", "type": "temperature"},
					},
				},
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	return ag.(*UserStoreAgent)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 정적 키에 대해 ClearHistory 가 히스토리만 비우고 엔트리를 보존한다.
func TestUserStoreAgent_ClearHistory_정적키(t *testing.T) {
	a := newStaticKeysAgentWithHistory(t)
	ctx := context.Background()
	store := a.inner.ForNamespace("default")

	// 정적 키에 5번 Set → 4개의 히스토리 (data_type=int 와 i 일치).
	for i := 1; i <= 5; i++ {
		require.NoError(t, store.Set(ctx, "indoor:1:room_temp", i))
		time.Sleep(time.Millisecond)
	}
	before, err := store.Get(ctx, "indoor:1:room_temp")
	require.NoError(t, err)
	require.Equal(t, 4, before.HistoryCount)

	// Act
	require.NoError(t, a.ClearHistory(ctx, "default", "indoor:1:room_temp"))

	// Assert: 엔트리는 보존, 히스토리만 비워짐.
	after, err := store.Get(ctx, "indoor:1:room_temp")
	require.NoError(t, err)
	assert.Equal(t, 5, after.Value)
	assert.Equal(t, 0, after.HistoryCount)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: namespace 가 빈 문자열이면 "default" 로 치환된다.
func TestUserStoreAgent_ClearHistory_NamespaceDefault(t *testing.T) {
	a := newStaticKeysAgentWithHistory(t)
	ctx := context.Background()
	store := a.inner.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "indoor:1:room_temp", 1))
	require.NoError(t, store.Set(ctx, "indoor:1:room_temp", 2))

	require.NoError(t, a.ClearHistory(ctx, "" /* default */, "indoor:1:room_temp"))

	entry, err := store.Get(ctx, "indoor:1:room_temp")
	require.NoError(t, err)
	assert.Equal(t, 2, entry.Value)
	assert.Equal(t, 0, entry.HistoryCount)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 존재하지 않는 키에 ClearHistory 호출 시 ErrKeyNotFound 를 반환한다.
func TestUserStoreAgent_ClearHistory_KeyNotFound(t *testing.T) {
	a := newStaticKeysAgent(t, true)
	err := a.ClearHistory(context.Background(), "default", "missing")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: 동적 키에 대해 DeleteEntry 가 엔트리+히스토리를 모두 삭제한다.
func TestUserStoreAgent_DeleteEntry_동적키(t *testing.T) {
	// auto 모드 에이전트로 동적 키를 쓸 수 있게 한다.
	a := newStaticKeysAgent(t, true)
	ctx := context.Background()
	store := a.inner.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "dynamic_key", "v1"))
	require.NoError(t, store.Set(ctx, "dynamic_key", "v2"))

	// Act
	require.NoError(t, a.DeleteEntry(ctx, "default", "dynamic_key"))

	// Assert: 엔트리가 사라짐.
	_, err := store.Get(ctx, "dynamic_key")
	assert.ErrorIs(t, err, ErrKeyNotFound)
}

// @spec SPEC-STORE-003 v0.3.0
// v0.2.0 와 동일 의도: Delete 와 동일하게 키가 없어도 에러를 반환하지 않는다.
func TestUserStoreAgent_DeleteEntry_없는키_에러없음(t *testing.T) {
	a := newStaticKeysAgent(t, true)
	err := a.DeleteEntry(context.Background(), "default", "ghost")
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// @spec SPEC-STORE-003 v0.3.0 / Scenario 6 — BREAKING migration failure
// v0.2.0 의 allow_dynamic_keys 가 yaml 에 잔존하면 부팅이 실패하고 명시적 마이그레이션
// 메시지가 노출되어야 한다. 이는 clean rename 의도이며 shim 은 제공하지 않는다.
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0 / Scenario 6
// allow_dynamic_keys 의 어떤 형태든 (bool true/false, string, 등) 잔존하면
// parseStoreConfig 가 명시적 마이그레이션 에러로 거부해야 한다. 에러 메시지는
// "allow_dynamic_keys", "removed in v0.3.0", "registration_type" 모두 포함해야 한다.
func TestParseStoreConfig_AllowDynamicKeys_RemovedInV030(t *testing.T) {
	cases := []struct {
		name string
		val  any
	}{
		{"bool_true", true},
		{"bool_false", false},
		{"string_false", "false"},
		{"string_true", "true"},
		{"nil", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := agent.AgentConfig{
				Transport: agent.TransportConfig{
					Options: map[string]any{
						"allow_dynamic_keys": tc.val,
					},
				},
			}
			opts, err := parseStoreConfig(cfg)
			assert.Nil(t, opts, "options should be nil on migration error")
			require.Error(t, err)
			assert.Contains(t, err.Error(), "allow_dynamic_keys")
			assert.Contains(t, err.Error(), "removed in v0.3.0")
			assert.Contains(t, err.Error(), "registration_type")
		})
	}
}

// ---------------------------------------------------------------------------
// @spec SPEC-STORE-003 v0.3.0 / M7 — manual mode missing data_type
// manual 모드에서 keys 의 한 엔트리라도 data_type 을 누락하면 부팅이 실패해야 한다.
// ---------------------------------------------------------------------------

// @spec SPEC-STORE-003 v0.3.0 / M7
// manual 모드에서 data_type 누락 시 ErrInvalidDataType 으로 부팅이 거부되어야 한다.
func TestParseStoreConfig_Manual_MissingDataType_BootFailure(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"registration_type": "manual",
				"keys": []any{
					map[string]any{
						"key": "indoor:1:room_temp",
						// data_type MISSING — must fail
					},
				},
			},
		},
	}
	opts, err := parseStoreConfig(cfg)
	assert.Nil(t, opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDataType)
}

// @spec SPEC-STORE-003 v0.3.0 / M7
// manual 모드에서 data_type 이 enum 외 값이면 ErrInvalidDataType 으로 부팅이 거부되어야 한다.
func TestParseStoreConfig_Manual_InvalidDataType_BootFailure(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"registration_type": "manual",
				"keys": []any{
					map[string]any{
						"key":       "indoor:1:room_temp",
						"data_type": "decimal", // enum 6종 외 값
					},
				},
			},
		},
	}
	opts, err := parseStoreConfig(cfg)
	assert.Nil(t, opts)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidDataType)
}
