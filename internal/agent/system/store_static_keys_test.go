// @spec SPEC-STORE-003
package system

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
)

// ---------------------------------------------------------------------------
// parseStoreConfig: 하위호환 (기존 설정 그대로 동작)
// ---------------------------------------------------------------------------

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

	// 옵션 적용 후 기본값(allowDynamicKeys=true, staticKeys=nil) 유지 확인.
	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}
	assert.True(t, sc.allowDynamicKeys, "allowDynamicKeys는 설정 미지정 시 기본 true")
	assert.Nil(t, sc.staticKeys, "staticKeys는 설정 미지정 시 nil")
}

// ---------------------------------------------------------------------------
// parseStoreConfig: allow_dynamic_keys
// ---------------------------------------------------------------------------

func TestParseStoreConfig_allow_dynamic_keys_false(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"allow_dynamic_keys": false,
			},
		},
	}
	opts, err := parseStoreConfig(cfg)
	require.NoError(t, err)
	sc := defaultConfig()
	for _, o := range opts {
		o(&sc)
	}
	assert.False(t, sc.allowDynamicKeys)
}

func TestParseStoreConfig_allow_dynamic_keys_타입오류(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"allow_dynamic_keys": "true", // string 은 허용되지 않는다
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "allow_dynamic_keys")
}

// ---------------------------------------------------------------------------
// parseStoreConfig: keys (정적 키 + 태그)
// ---------------------------------------------------------------------------

func TestParseStoreConfig_keys_정상케이스(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{
						"key": "indoor:1:room_temp",
						"tags": map[string]any{
							"room": "1",
							"type": "temperature",
						},
					},
					map[string]any{
						"key": "outdoor:temperature",
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
	assert.Equal(t, "1", sc.staticKeys["indoor:1:room_temp"]["room"])
	assert.Equal(t, "temperature", sc.staticKeys["indoor:1:room_temp"]["type"])
	assert.Equal(t, "outside", sc.staticKeys["outdoor:temperature"]["location"])
}

func TestParseStoreConfig_keys_태그없음_허용(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{"key": "only_key"},
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
	assert.Empty(t, sc.staticKeys["only_key"], "tags 미지정은 빈 맵")
}

func TestParseStoreConfig_keys_중복_ErrDuplicateStaticKey(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{"key": "dup"},
					map[string]any{"key": "dup"},
				},
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrDuplicateStaticKey)
}

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
								"key":  "k",
								"tags": map[string]any{tc.tagKey: "v"},
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

func TestParseStoreConfig_keys_태그value_비문자열_에러(t *testing.T) {
	cfg := agent.AgentConfig{
		Transport: agent.TransportConfig{
			Options: map[string]any{
				"keys": []any{
					map[string]any{
						"key":  "k",
						"tags": map[string]any{"n": 42}, // int 는 허용되지 않음
					},
				},
			},
		},
	}
	_, err := parseStoreConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be string")
}

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
// 쓰기 경로: allow_dynamic_keys=false 일 때 정적 키 밖의 쓰기 거부
// ---------------------------------------------------------------------------

func TestWriteGate_strict모드_미등록키_거부(t *testing.T) {
	sa := NewStoreAgent(
		WithAllowDynamicKeys(false),
		WithStaticKeys(map[string]map[string]string{
			"indoor:1:room_temp": {"room": "1", "type": "temperature"},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	store := sa.ForNamespace("default")

	// 등록된 키는 통과
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

func TestWriteGate_strict모드_SetWithTTL_도_거부(t *testing.T) {
	sa := NewStoreAgent(
		WithAllowDynamicKeys(false),
		WithStaticKeys(map[string]map[string]string{"k": {}}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	store := sa.ForNamespace("default")

	// 미등록 키 SetWithTTL 도 거부되어야 한다.
	err := store.SetWithTTL(context.Background(), "not_allowed", 1, 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrKeyNotAllowed)
}

func TestWriteGate_permissive모드_미등록키_성공_태그없음(t *testing.T) {
	sa := NewStoreAgent(
		// 기본 allowDynamicKeys=true
		WithStaticKeys(map[string]map[string]string{
			"known": {"type": "x"},
		}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	store := sa.ForNamespace("default")

	// 미등록 키 성공
	require.NoError(t, store.Set(context.Background(), "dynamic", "value"))

	// 등록된 키도 성공
	require.NoError(t, store.Set(context.Background(), "known", "value"))

	keys, err := store.Keys(context.Background(), "*")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"dynamic", "known"}, keys)
}

// ---------------------------------------------------------------------------
// TagsFor / StaticTagsFor / StaticKeyTags
// ---------------------------------------------------------------------------

func TestAgentStore_TagsFor_정적키_태그반환(t *testing.T) {
	sa := NewStoreAgent(
		WithStaticKeys(map[string]map[string]string{
			"k1": {"room": "1", "type": "temperature"},
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

func TestAgentStore_TagsFor_동적키_빈맵(t *testing.T) {
	sa := NewStoreAgent(
		WithStaticKeys(map[string]map[string]string{"k1": {"a": "b"}}),
	)
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })

	as := &agentStore{agent: sa}
	assert.Equal(t, map[string]string{}, as.TagsFor("unknown"))
}

func TestStoreAgent_StaticKeyTags_복사본반환(t *testing.T) {
	sa := NewStoreAgent(
		WithStaticKeys(map[string]map[string]string{
			"k1": {"room": "1"},
			"k2": {"room": "2"},
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

func newStaticKeysAgent(t *testing.T, allowDynamic bool) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:   "s1",
		Name: "store-a",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":            "volatile",
				"allow_dynamic_keys": allowDynamic,
				"keys": []any{
					map[string]any{
						"key": "indoor:1:room_temp",
						"tags": map[string]any{
							"room": "1",
							"type": "temperature",
						},
					},
					map[string]any{
						"key": "outdoor:temperature",
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

func TestUserStoreAgent_KeyTags_정적키_반환(t *testing.T) {
	a := newStaticKeysAgent(t, true)
	out, err := a.KeyTags(context.Background())
	require.NoError(t, err)
	require.Len(t, out, 2)
	assert.Equal(t, "1", out["indoor:1:room_temp"]["room"])
}

func TestUserStoreAgent_StaticTagPairs_정렬(t *testing.T) {
	a := newStaticKeysAgent(t, true)
	pairs := a.StaticTagPairs()

	assert.Equal(t, []string{"1"}, pairs["room"])
	assert.Equal(t, []string{"outside"}, pairs["location"])
	assert.Equal(t, []string{"temperature"}, pairs["type"])
}

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

func TestUserStoreAgent_State_정적키_tags_포함(t *testing.T) {
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
	require.NotNil(t, found, "정적 키 엔트리가 존재해야 한다")
	tags, ok := found["tags"].(map[string]string)
	require.True(t, ok, "정적 키 엔트리는 tags 맵을 포함해야 한다")
	assert.Equal(t, "1", tags["room"])
	assert.Equal(t, "temperature", tags["type"])

	require.NotNil(t, foundDyn, "동적 키 엔트리가 존재해야 한다")
	_, hasTags := foundDyn["tags"]
	assert.False(t, hasTags, "동적 키 엔트리는 tags 필드를 포함하지 않아야 한다")
}

// ---------------------------------------------------------------------------
// UserStoreAgent.Configure(): 정책 필드 런타임 반영
// ---------------------------------------------------------------------------
// @spec SPEC-STORE-003
// Configure 는 allow_dynamic_keys / keys (정적 키+태그) 변경을 inner StoreAgent 에
// 런타임으로 전파해야 한다. 운영 필드(scan_interval 등)는 재시작 시에만 반영된다.

// buildStoreConfig 는 테스트용 AgentConfig 빌더다.
func buildStoreConfig(allowDynamic bool, keys []any) agent.AgentConfig {
	return agent.AgentConfig{
		ID:   "s1",
		Name: "store-a",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":            "volatile",
				"allow_dynamic_keys": allowDynamic,
				"keys":               keys,
			},
		},
	}
}

func TestUserStoreAgent_Configure_UpdatesPolicy_permissive_to_strict(t *testing.T) {
	// 초기: allow_dynamic_keys=true, 정적 키 없음.
	initial := agent.AgentConfig{
		ID:   "s1",
		Name: "store-a",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"backend":            "volatile",
				"allow_dynamic_keys": true,
			},
		},
	}
	ag, err := NewUserStoreAgent(initial)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	store := u.inner.ForNamespace("default")

	// 초기 상태: 동적 키 쓰기 허용 확인.
	require.NoError(t, store.Set(context.Background(), "dyn:1", "v1"),
		"초기 permissive 모드에서 동적 키 쓰기 성공해야 한다")

	// Configure 호출로 strict 모드 + 정적 키 1개 주입.
	newCfg := buildStoreConfig(false, []any{
		map[string]any{
			"key":  "static:1",
			"tags": map[string]any{"room": "1", "type": "temperature"},
		},
	})
	require.NoError(t, u.Configure(newCfg))

	// 이제 동적 키 쓰기 거부되어야 한다.
	err = store.Set(context.Background(), "dyn:2", "v2")
	require.Error(t, err, "Configure 이후 strict 모드에서 동적 키 쓰기는 거부되어야 한다")
	assert.ErrorIs(t, err, ErrKeyNotAllowed)

	// 정적 키 쓰기는 성공해야 한다.
	require.NoError(t, store.Set(context.Background(), "static:1", 22.5),
		"Configure 이후 정적 키 쓰기 성공해야 한다")

	// TagsFor 가 새 태그를 반환해야 한다.
	tags := u.inner.StaticTagsFor("static:1")
	assert.Equal(t, "1", tags["room"])
	assert.Equal(t, "temperature", tags["type"])
}

func TestUserStoreAgent_Configure_UpdatesPolicy_strict_to_permissive(t *testing.T) {
	// 초기: allow_dynamic_keys=false, 정적 키 1개.
	initial := buildStoreConfig(false, []any{
		map[string]any{"key": "static:A", "tags": map[string]any{"v": "1"}},
	})
	ag, err := NewUserStoreAgent(initial)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)

	store := u.inner.ForNamespace("default")

	// 초기 상태: 동적 키 쓰기 거부 확인.
	err = store.Set(context.Background(), "dyn:1", "v1")
	require.Error(t, err, "초기 strict 모드에서 동적 키 쓰기는 거부되어야 한다")
	assert.ErrorIs(t, err, ErrKeyNotAllowed)

	// Configure 호출로 permissive 모드 전환.
	newCfg := buildStoreConfig(true, []any{
		map[string]any{"key": "static:A", "tags": map[string]any{"v": "1"}},
	})
	require.NoError(t, u.Configure(newCfg))

	// 이제 동적 키 쓰기 허용되어야 한다.
	require.NoError(t, store.Set(context.Background(), "dyn:2", "v2"),
		"Configure 이후 permissive 모드에서 동적 키 쓰기 성공해야 한다")
}

func TestUserStoreAgent_Configure_UpdatesPolicy_static_keys_mutation(t *testing.T) {
	// 초기: 정적 키 A={room:1}.
	initial := buildStoreConfig(true, []any{
		map[string]any{
			"key":  "A",
			"tags": map[string]any{"room": "1"},
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
	newCfg := buildStoreConfig(true, []any{
		map[string]any{
			"key":  "A",
			"tags": map[string]any{"room": "2"},
		},
		map[string]any{
			"key":  "B",
			"tags": map[string]any{"room": "3"},
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

func TestStoreErrors_정의(t *testing.T) {
	assert.NotNil(t, ErrKeyNotAllowed)
	assert.NotNil(t, ErrDuplicateStaticKey)
	assert.NotNil(t, ErrInvalidTagKey)

	// 센티넬 에러는 errors.Is 로 비교 가능해야 한다.
	wrapped := errors.New("wrapping: " + ErrKeyNotAllowed.Error())
	assert.NotErrorIs(t, wrapped, ErrKeyNotAllowed, "문자열 래핑은 Is 와 동일하지 않다")
}
