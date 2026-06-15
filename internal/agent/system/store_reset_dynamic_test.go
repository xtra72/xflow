package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 본 파일은 "전체 초기화(ResetAll) 시 동적 키 삭제" 요구를 검증한다.
// 회귀(수정 전): IsStaticKey 가 Source 무관하게 레지스트리 존재로 판정해 auto(동적)
// 키도 static 취급 → reset 시 ClearHistory(보존)되고, DeleteEntry 도 값만 지우고
// 레지스트리 메타(staticKeys)를 남겨 /keys 에 동적 키가 잔존했다.

// newAutoUserStoreAgentWithManualKey 는 auto 등록 모드 + manual 정적 키 "manual_k"
// 를 가진 UserStoreAgent 를 만든다.
func newAutoUserStoreAgentWithManualKey(t *testing.T) *UserStoreAgent {
	t.Helper()
	cfg := buildStoreConfig(RegistrationAuto, []any{
		map[string]any{"key": "manual_k", "data_type": "string"},
	})
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	u := ag.(*UserStoreAgent)
	require.NoError(t, u.Start(context.Background()))
	return u
}

// writeDynamicSeries 는 동적(auto) 시리즈를 하나 써서 그 저장 키를 반환한다.
func writeDynamicSeries(t *testing.T, u *UserStoreAgent, key, metric string) string {
	t.Helper()
	raw := u.NodeStoreForNamespace("")
	adapter := raw.(*NodeStoreAdapter)
	require.NoError(t, adapter.SetWithMeta(context.Background(), key, "1", StoreWriteMeta{MetricType: metric}))
	return EncodeSeriesKey(SeriesID{Key: key, MetricType: metric}.Normalize())
}

// TestIsStaticKey_OnlyManualIsStatic 는 IsStaticKey 가 Source=manual 만 정적으로
// 판정하고 동적(auto) 키는 false 를 반환함을 검증한다.
func TestIsStaticKey_OnlyManualIsStatic(t *testing.T) {
	u := newAutoUserStoreAgentWithManualKey(t)
	dynKey := writeDynamicSeries(t, u, "dev.temp", "current_temperature")

	assert.True(t, u.IsStaticKey("manual_k"), "manual 정적 키는 static 이어야 한다")
	assert.False(t, u.IsStaticKey(dynKey), "동적(auto) 키는 static 이 아니어야 한다(reset 시 삭제 대상)")
}

// TestDeleteEntry_RemovesDynamicRegistryMeta 는 DeleteEntry 가 값뿐 아니라 동적 키의
// 레지스트리 메타(staticKeys)도 제거하여 키가 완전히 사라짐을 검증한다.
func TestDeleteEntry_RemovesDynamicRegistryMeta(t *testing.T) {
	u := newAutoUserStoreAgentWithManualKey(t)
	dynKey := writeDynamicSeries(t, u, "dev.temp", "current_temperature")

	// 쓰기 직후엔 레지스트리에 등록돼 있어야 한다.
	_, ok := u.inner.StaticKeyMetaFor(dynKey)
	require.True(t, ok, "동적 키는 쓰기 후 레지스트리에 등록된다")

	require.NoError(t, u.DeleteEntry(context.Background(), "", dynKey))

	_, ok = u.inner.StaticKeyMetaFor(dynKey)
	assert.False(t, ok, "DeleteEntry 후 동적 키의 레지스트리 메타가 제거되어야 한다")
}

// TestState_EntryKeyIsDecodedUserKey 는 State() 의 엔트리 key 가 인코딩 시리즈 키
// (metric|tags|key)가 아니라 디코드된 사용자 key 임을 검증한다(저장소 탭 키 컬럼 표시).
// 회귀(수정 전): displayKey 가 인코딩 키 그대로라 "metric|key" 가 노출됐다.
func TestState_EntryKeyIsDecodedUserKey(t *testing.T) {
	u := newAutoUserStoreAgentWithManualKey(t)
	_ = writeDynamicSeries(t, u, "dev.temp", "current_temperature")

	st := u.State()
	entries, _ := st["entries"].([]map[string]any)
	require.NotEmpty(t, entries)

	var found bool
	for _, e := range entries {
		if e["key"] == "dev.temp" {
			found = true
			assert.Equal(t, "current_temperature", e["metric_type"], "metric 은 별도 필드로 노출")
		}
		// 인코딩 키(metric|...|key)가 그대로 노출되면 안 된다.
		k, _ := e["key"].(string)
		assert.NotContains(t, k, "|", "엔트리 key 에 시리즈 인코딩 구분자가 없어야 한다(디코드된 사용자 key)")
	}
	assert.True(t, found, "디코드된 사용자 key 'dev.temp' 엔트리가 있어야 한다")
}

// TestIsStaticKey_ExplicitDataTypeStillDynamic 는 명시 data_type(float/int/boolean)으로
// store-write 된 런타임 키가 여전히 동적(IsStaticKey=false)임을 검증한다.
// 회귀(수정 전): SetKeyDataType 가 명시 타입 신규 키를 Source=manual 로 등록해 정적 취급
// → 전체 초기화에서 보존되어 자동 등록 키가 남았다.
func TestIsStaticKey_ExplicitDataTypeStillDynamic(t *testing.T) {
	u := newAutoUserStoreAgentWithManualKey(t)
	raw := u.NodeStoreForNamespace("")
	adapter := raw.(*NodeStoreAdapter)
	// store-write 노드와 동일: data_type 명시 + metric (HVAC current_temperature=float).
	require.NoError(t, adapter.SetWithMeta(context.Background(), "dev.temp", 21.5,
		StoreWriteMeta{DataType: "float", MetricType: "current_temperature"}))

	seriesKey := EncodeSeriesKey(SeriesID{Key: "dev.temp", MetricType: "current_temperature"}.Normalize())
	meta, ok := u.inner.StaticKeyMetaFor(seriesKey)
	require.True(t, ok)
	assert.Equal(t, SourceAuto, meta.Source, "명시 data_type 이어도 런타임 등록 키는 Source=auto")
	assert.False(t, u.IsStaticKey(seriesKey), "명시 data_type 의 런타임 키도 동적이어야 한다(reset 삭제 대상)")
}
