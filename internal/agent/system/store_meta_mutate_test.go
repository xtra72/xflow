// @spec SPEC-STORE-003 v0.4.0
//
// store_meta_mutate_test.go 는 SetKeyMeta (임의 엔트리의 field/tags 설정) 와
// 동적 string 정책 관련 system 계층 동작을 검증한다.
package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// newAutoStoreAgent 는 registration_type=auto + 정적 키 없음 구성의 UserStoreAgent 를 만든다.
func newAutoStoreAgent(t *testing.T) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:   "s-meta",
		Name: "store-meta",
		Type: "store",
		Transport: agent.TransportConfig{
			Type:    "store",
			Options: map[string]any{"registration_type": "auto"},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	return ag.(*UserStoreAgent)
}

// TestSetKeyMeta_DynamicKey_AssignsMetricAndTags 는 런타임에 자동 등록된 동적 키에
// SetKeyMeta 로 field 과 tags 를 부여할 수 있고, data_type=string / Source=auto 는
// 보존됨을 검증한다 (M2: 동적 키에도 타입/태그 지정 가능).
func TestSetKeyMeta_DynamicKey_AssignsMetricAndTags(t *testing.T) {
	a := newAutoStoreAgent(t)
	ctx := context.Background()

	// 동적 키 자동 등록 (값 쓰기).
	store := a.inner.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "outdoor:humidity", 55))

	// 등록 직후: field=unknown, tags={}.
	snap := a.StaticKeysSnapshot()
	require.Contains(t, snap, "outdoor:humidity")
	assert.Equal(t, "unknown", snap["outdoor:humidity"].Field)
	assert.Empty(t, snap["outdoor:humidity"].Tags)

	// SetKeyMeta 로 타입/태그 부여.
	err := a.SetKeyMeta("outdoor:humidity", "humidity", map[string]string{"room": "kitchen"})
	require.NoError(t, err)

	snap = a.StaticKeysSnapshot()
	meta := snap["outdoor:humidity"]
	assert.Equal(t, "humidity", meta.Field, "field 갱신")
	assert.Equal(t, "kitchen", meta.Tags["room"], "tags 갱신")
	assert.Equal(t, DataTypeString, meta.DataType, "동적 키 data_type=string 보존")
	assert.Equal(t, SourceAuto, meta.Source, "Source=auto 보존")
}

// TestSetKeyMeta_UnregisteredKey_RegistersDynamicString 는 아직 값이 쓰여지지 않은
// 미등록 키에 SetKeyMeta 를 호출하면 동적 string 키로 신규 등록되고 메타가 적용됨을 검증한다.
func TestSetKeyMeta_UnregisteredKey_RegistersDynamicString(t *testing.T) {
	a := newAutoStoreAgent(t)

	err := a.SetKeyMeta("future:key", "power", map[string]string{"phase": "a"})
	require.NoError(t, err)

	snap := a.StaticKeysSnapshot()
	require.Contains(t, snap, "future:key")
	meta := snap["future:key"]
	assert.Equal(t, DataTypeString, meta.DataType)
	assert.Equal(t, SourceAuto, meta.Source)
	assert.Equal(t, "power", meta.Field)
	assert.Equal(t, "a", meta.Tags["phase"])
}

// TestSetKeyMeta_StaticKey_PreservesDataType 는 yaml 로 정의된 정적 키(명시 data_type)에
// SetKeyMeta 를 적용해도 DataType/Source 가 보존되고 field/tags 만 갱신됨을 검증한다
// (PRESERVE: 정적 키의 명시 data_type 보존).
func TestSetKeyMeta_StaticKey_PreservesDataType(t *testing.T) {
	a := newStaticKeysAgent(t, false) // manual 모드: indoor:1:room_temp = float

	err := a.SetKeyMeta("indoor:1:room_temp", "celsius", map[string]string{"floor": "2"})
	require.NoError(t, err)

	snap := a.StaticKeysSnapshot()
	meta := snap["indoor:1:room_temp"]
	assert.Equal(t, DataTypeFloat, meta.DataType, "정적 키 명시 data_type 보존")
	assert.Equal(t, SourceManual, meta.Source, "Source=manual 보존")
	assert.Equal(t, "celsius", meta.Field, "field 갱신")
	assert.Equal(t, "2", meta.Tags["floor"], "tags 갱신")
}

// TestSetKeyMeta_EmptyField_NormalizesToUnknown 은 빈 field 이 "unknown" 으로
// normalize 됨을 검증한다.
func TestSetKeyMeta_EmptyField_NormalizesToUnknown(t *testing.T) {
	a := newAutoStoreAgent(t)

	require.NoError(t, a.SetKeyMeta("k", "", nil))
	snap := a.StaticKeysSnapshot()
	assert.Equal(t, "unknown", snap["k"].Field)
}

// TestSetKeyMeta_InvalidField_Rejected 는 정규식 위반 field 이
// ErrInvalidField 으로 거부됨을 검증한다.
func TestSetKeyMeta_InvalidField_Rejected(t *testing.T) {
	a := newAutoStoreAgent(t)

	err := a.SetKeyMeta("k", "bad type!", nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidField)

	// 거부 시 등록되지 않아야 한다.
	snap := a.StaticKeysSnapshot()
	_, exists := snap["k"]
	assert.False(t, exists, "검증 실패 시 키가 등록되지 않아야 한다")
}

// TestSetKeyMeta_InvalidTagKey_Rejected 는 정규식 위반 tag key 가 ErrInvalidTagKey 로
// 거부됨을 검증한다.
func TestSetKeyMeta_InvalidTagKey_Rejected(t *testing.T) {
	a := newAutoStoreAgent(t)

	err := a.SetKeyMeta("k", "gauge", map[string]string{"bad key": "v"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidTagKey)
}

// TestSetKeyMeta_EmptyKey_Rejected 는 빈 key 가 거부됨을 검증한다.
func TestSetKeyMeta_EmptyKey_Rejected(t *testing.T) {
	a := newAutoStoreAgent(t)
	err := a.SetKeyMeta("", "gauge", nil)
	require.Error(t, err)
}

// TestStringifyValue 는 다양한 타입의 string 변환 결과를 검증한다.
func TestStringifyValue(t *testing.T) {
	cases := []struct {
		name  string
		value any
		want  string
	}{
		{"string", "hello", "hello"},
		{"int", 42, "42"},
		{"float", 3.14, "3.14"},
		{"bool_true", true, "true"},
		{"bool_false", false, "false"},
		{"bytes", []byte("raw"), "raw"},
		{"nil", nil, ""},
		{"slice", []int{1, 2}, "[1 2]"},
		{"map", map[string]int{"a": 1}, "map[a:1]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, stringifyValue(tc.value))
		})
	}
}

// TestIsStringifiableValue 는 nil/channel/func 만 거부되고 나머지는 허용됨을 검증한다.
func TestIsStringifiableValue(t *testing.T) {
	assert.False(t, isStringifiableValue(nil), "nil 거부")
	assert.False(t, isStringifiableValue(make(chan int)), "channel 거부")
	assert.False(t, isStringifiableValue(func() {}), "func 거부")
	assert.True(t, isStringifiableValue(42), "int 허용")
	assert.True(t, isStringifiableValue("s"), "string 허용")
	assert.True(t, isStringifiableValue([]byte("b")), "bytes 허용")
	assert.True(t, isStringifiableValue(map[string]any{"a": 1}), "map 허용")
}
