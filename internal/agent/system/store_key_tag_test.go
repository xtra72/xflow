package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// newKeyTagAdapter 는 key_tag 가 설정된 *StoreAgent 와 연결된 NodeStoreAdapter 를 만든다.
func newKeyTagAdapter(t *testing.T, keyTag string) (*NodeStoreAdapter, *StoreAgent) {
	t.Helper()
	sa := NewStoreAgent(WithMaxHistorySize(10), WithKeyTag(keyTag))
	require.NoError(t, sa.Init(context.Background()))
	t.Cleanup(func() { _ = sa.Stop(context.Background()) })
	storeResolver := func() Store { return sa.ForNamespace("default") }
	agentResolver := func() *StoreAgent { return sa }
	adapter := NewLazyNodeStoreAdapterWithAgent(storeResolver, agentResolver, "default")
	return adapter, sa
}

// TestKeyTag_OverridesKeyWithTagValue 는 key_tag 지정 시 해당 태그 값이 시리즈 키로
// 사용되고, 원래 키(생성된 id)는 "id" 태그로 보존됨을 검증한다.
func TestKeyTag_OverridesKeyWithTagValue(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newKeyTagAdapter(t, "name")

	// 호출자 제공 key = 생성된 device_id, tags 에 name 포함.
	err := adapter.SetWithMeta(ctx, "dev-uuid-123", "v", StoreWriteMeta{
		Tags: map[string]string{"name": "livingroom"},
	})
	require.NoError(t, err)

	// 시리즈 키는 name 값("livingroom")으로 재구성되며, id 태그가 보존된다.
	seriesKey := EncodeSeriesKey(SeriesID{
		Measurement: "livingroom",
		Tags:        map[string]string{"name": "livingroom", "id": "dev-uuid-123"},
	})
	meta, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok, "name 값을 키로 한 시리즈가 등록되어야 한다")
	assert.Equal(t, "dev-uuid-123", meta.Tags["id"], "원래 id 는 id 태그로 보존")
	assert.Equal(t, "livingroom", meta.Tags["name"])

	// 값도 해당 시리즈에서 조회되어야 한다.
	got, found, err := adapter.GetSeries(ctx, "livingroom", "",
		map[string]string{"name": "livingroom", "id": "dev-uuid-123"})
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, "v", got)
}

// TestKeyTag_FallsBackToProvidedKeyWhenTagAbsent 는 지정 태그가 없으면 기존 키
// (생성된 id)로 폴백함을 검증한다.
func TestKeyTag_FallsBackToProvidedKeyWhenTagAbsent(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newKeyTagAdapter(t, "name")

	// name 태그 없음 → 기존 키 유지.
	err := adapter.SetWithMeta(ctx, "dev-uuid-999", "v", StoreWriteMeta{
		Tags: map[string]string{"room": "1"},
	})
	require.NoError(t, err)

	// 시리즈 키는 원래 키(dev-uuid-999)로 유지되고 id 태그는 주입되지 않는다.
	seriesKey := EncodeSeriesKey(SeriesID{
		Measurement: "dev-uuid-999",
		Tags:        map[string]string{"room": "1"},
	})
	meta, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok, "폴백 시 원래 키로 시리즈가 등록되어야 한다")
	assert.Equal(t, map[string]string{"room": "1"}, meta.Tags, "id 태그 주입 없음")
}

// TestKeyTag_EmptyTagValueFallsBack 는 지정 태그가 있으나 값이 비면 폴백함을 검증한다.
func TestKeyTag_EmptyTagValueFallsBack(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newKeyTagAdapter(t, "name")

	err := adapter.SetWithMeta(ctx, "dev-uuid-777", "v", StoreWriteMeta{
		Tags: map[string]string{"name": ""},
	})
	require.NoError(t, err)

	seriesKey := EncodeSeriesKey(SeriesID{Measurement: "dev-uuid-777", Tags: map[string]string{"name": ""}})
	_, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok, "빈 name 값이면 원래 키로 폴백")
}

// TestKeyTag_Disabled 는 key_tag 미설정(빈 문자열)이면 오버라이드가 없음을 검증한다.
func TestKeyTag_Disabled(t *testing.T) {
	ctx := context.Background()
	adapter, sa := newKeyTagAdapter(t, "")

	err := adapter.SetWithMeta(ctx, "dev-uuid-000", "v", StoreWriteMeta{
		Tags: map[string]string{"name": "shouldNotBeKey"},
	})
	require.NoError(t, err)

	// name 오버라이드 없이 원래 키로 등록.
	seriesKey := EncodeSeriesKey(SeriesID{Measurement: "dev-uuid-000", Tags: map[string]string{"name": "shouldNotBeKey"}})
	_, ok := sa.StaticKeyMetaFor(seriesKey)
	require.True(t, ok, "key_tag 미설정 시 원래 키 유지")
}

// TestKeyTag_ConfigParse 는 yaml key_tag 옵션이 파싱되어 에이전트에 반영됨을 검증한다.
func TestKeyTag_ConfigParse(t *testing.T) {
	u := makeStoreAgentForRegistration(t, map[string]any{
		"backend": "volatile",
		"key_tag": "name",
	})
	assert.Equal(t, "name", u.inner.KeyTag())
}

// TestKeyTag_ConfigParse_Invalid 는 형식에 맞지 않는 key_tag 가 거부됨을 검증한다.
func TestKeyTag_ConfigParse_Invalid(t *testing.T) {
	cfg := agent.AgentConfig{
		ID: "s1", Name: "store-a", Type: "store",
		Transport: agent.TransportConfig{
			Type:    "store",
			Options: map[string]any{"key_tag": "bad tag!"},
		},
	}
	_, err := NewUserStoreAgent(cfg)
	require.Error(t, err, "형식 위반 key_tag 는 부팅 실패해야 한다")
}
