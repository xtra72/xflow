// @spec SPEC-STORE-004
//
// store_series_reset_char_test.go — M4 PRESERVE 단계.
//
// reset/삭제 및 시리즈 정합(IsStaticKey / StaticTagPairs) 의 "현재 동작" 을 먼저
// characterization 테스트로 포착한다. M2 이후 레지스트리(staticKeys)/저장 키는
// 시리즈 인코딩(EncodeSeriesKey) 으로 키잉되므로, 본 파일은 시리즈 모델에서의 현재
// reset 관련 빌딩블록(ClearHistory/DeleteEntry/IsStaticKey/StaticTagPairs)이 어떻게
// 동작하는지 기록하여, M4 의 시리즈 reset(ResetSeries) 도입이 회귀를 일으키지 않음을
// 보장한다.

package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// CHAR: SetWithMeta 로 쓰여진 시리즈는 EncodeSeriesKey 인코딩 키로 저장된다.
// ListStoreKeys 는 (네임스페이스 접두사 제거된) 인코딩 키를 반환한다.
func TestChar_SeriesWrite_StorageKeyIsEncoded(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{Field: "humidity"}))

	keys, err := a.ListStoreKeys(ctx, "default", "*")
	require.NoError(t, err)

	wantTemp := EncodeSeriesKey(SeriesID{Measurement: "room", Field: "temperature"})
	wantHum := EncodeSeriesKey(SeriesID{Measurement: "room", Field: "humidity"})
	assert.ElementsMatch(t, []string{wantTemp, wantHum}, keys,
		"저장 키는 시리즈 인코딩(field|tags|key) 형태여야 한다 (현재 동작)")
}

// CHAR: 시리즈 쓰기 후 레지스트리(staticKeys)는 인코딩 키로 키잉된다.
//
// 중요한 현재 동작(characterization): IsStaticKey 는 Source(manual/auto) 를 보지 않고
// "그 키가 레지스트리(staticKeys)에 존재하는가" 만을 본다. SetWithMeta 는 SetKeyMeta 로
// 인코딩 키를 레지스트리에 등록하므로, auto 로 등록된 시리즈라도 IsStaticKey(인코딩키)=true 다.
// (즉 reset 핸들러 관점에서 "값이 쓰여 레지스트리에 올라온 시리즈" 는 정적 취급 → ClearHistory.)
// M4 의 ResetSeries 정합은 이 동작을 보존해야 한다.
func TestChar_SeriesWrite_RegistryKeyedByEncoded(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))

	encoded := EncodeSeriesKey(SeriesID{Measurement: "room", Field: "temperature"})
	snap := a.StaticKeysSnapshot()
	meta, exists := snap[encoded]
	assert.True(t, exists, "레지스트리는 인코딩 키로 메타를 보관한다")
	assert.Equal(t, SourceAuto, meta.Source, "런타임 자동 등록 시리즈는 Source=auto")

	// @spec SPEC-STORE-004: IsStaticKey 는 Source=manual 만 정적으로 본다.
	// 동적(auto) 시리즈는 레지스트리에 있어도 정적이 아니다(reset 시 삭제 대상).
	assert.False(t, a.IsStaticKey(encoded), "동적(auto) 인코딩 키는 정적이 아니다")
}

// CHAR: yaml 정적 키는 bare key 로 등록되며 IsStaticKey(bareKey)=true (직접 조회 적중).
func TestChar_YamlStaticKey_IsStaticKeyByBareKey(t *testing.T) {
	a := newStaticKeysAgentWithHistory(t)
	assert.True(t, a.IsStaticKey("indoor:1:room_temp"),
		"yaml 정적 키는 bare key 로 등록되어 직접 조회로 적중한다 (현재 동작)")
	assert.False(t, a.IsStaticKey("nonexistent"))
}

// CHAR: ClearHistory/DeleteEntry 는 전달된 키(인코딩이든 bare 든) 를 그대로 사용한다.
// 즉 호출자가 인코딩 키를 넘기면 그 시리즈의 저장 엔트리에 정확히 작용한다.
func TestChar_ClearAndDelete_OperateOnGivenKey(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{Field: "humidity"}))

	tempKey := EncodeSeriesKey(SeriesID{Measurement: "room", Field: "temperature"})
	humKey := EncodeSeriesKey(SeriesID{Measurement: "room", Field: "humidity"})

	// 동적 시리즈는 DeleteEntry 로 엔트리+히스토리 제거. humidity 만 지운다.
	require.NoError(t, a.DeleteEntry(ctx, "default", humKey))

	keys, err := a.ListStoreKeys(ctx, "default", "*")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{tempKey}, keys,
		"humidity 시리즈만 삭제되고 temperature 시리즈는 보존되어야 한다")
}

// CHAR: StaticTagPairs 는 레지스트리(인코딩 키든 bare 든) 의 메타 Tags 를 가로질러 집계한다.
// 시리즈 모델에서 각 레지스트리 엔트리는 하나의 시리즈이며, 그 메타에 실제 tags 가 보관된다.
func TestChar_StaticTagPairs_AggregatesAcrossSeries(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "temp", 21, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"room": "1"},
	}))
	require.NoError(t, adapter.SetWithMeta(ctx, "temp", 26, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"room": "2"},
	}))

	pairs := a.StaticTagPairs()
	assert.Equal(t, []string{"1", "2"}, pairs["room"],
		"두 시리즈의 room 태그 값이 정렬된 union 으로 집계되어야 한다 (현재 동작)")
}
