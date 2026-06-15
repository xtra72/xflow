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
