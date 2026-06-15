// @spec SPEC-STORE-004
//
// store_series_reset_test.go — M4 IMPROVE 단계의 시리즈 reset/정합 수용 기준 테스트.
//
// ResetSeries(단일/부분/전체 시리즈 reset, E8/AC-15, 식별자 누락 정책) 와 IsStaticKey 의
// 사용자 관점 fallback 의미를 검증한다. 회귀 보장은 store_series_reset_char_test.go 의
// characterization 테스트가 담당한다.

package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// AC-15: metric 식별자로 단일 시리즈만 reset, 다른 시리즈 무영향.
func TestResetSeries_SingleSeries_OthersUntouched(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{MetricType: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{MetricType: "humidity"}))

	cleared, deleted, err := a.ResetSeries(ctx, "default", "room", "humidity", nil)
	require.NoError(t, err)
	// 동적(auto) 시리즈지만 레지스트리 등록 → 정적 취급 → ClearHistory.
	assert.Equal(t, 1, cleared, "humidity 단일 시리즈만 ClearHistory")
	assert.Equal(t, 0, deleted)

	// temperature 시리즈는 영향받지 않는다.
	results, err := a.QuerySeries(ctx, "default", "room", "temperature", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "22", results[0].Entries[0].Value, "temperature 현재값 보존(무영향)")
}

// 식별자 누락 정책: metric/tags 없이 → 그 key 의 모든 시리즈가 대상.
func TestResetSeries_NoIdentifier_AllSeriesOfKey(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{MetricType: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{MetricType: "humidity"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 23, StoreWriteMeta{
		MetricType: "temperature", Tags: map[string]string{"area": "a"},
	}))
	// 다른 key.
	require.NoError(t, adapter.SetWithMeta(ctx, "other", 9, StoreWriteMeta{MetricType: "temperature"}))

	cleared, deleted, err := a.ResetSeries(ctx, "default", "room", "", nil)
	require.NoError(t, err)
	assert.Equal(t, 3, cleared+deleted, "room 의 3개 시리즈 전부 대상")

	// other key 의 시리즈는 영향받지 않는다.
	results, err := a.QuerySeries(ctx, "default", "other", "", nil, lastN(10))
	require.NoError(t, err)
	assert.Len(t, results, 1, "다른 key 의 시리즈는 보존")
}

// metric+tags 로 더 좁힌 단일 시리즈 reset (부분집합이 아닌 정확 일치 1개).
func TestResetSeries_MetricAndTags_NarrowsToOne(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{MetricType: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 23, StoreWriteMeta{
		MetricType: "temperature", Tags: map[string]string{"area": "a"},
	}))

	cleared, deleted, err := a.ResetSeries(ctx, "default", "room",
		"temperature", map[string]string{"area": "a"})
	require.NoError(t, err)
	assert.Equal(t, 1, cleared+deleted, "(temperature, area=a) 단일 시리즈만")

	// 태그 없는 temperature 시리즈는 보존.
	results, err := a.QuerySeries(ctx, "default", "room", "temperature", nil, lastN(10))
	require.NoError(t, err)
	assert.Len(t, results, 2, "ClearHistory 는 엔트리 보존 — 두 시리즈 모두 현재값 유지")
}

// 미일치 → (0,0,nil), 어떤 시리즈도 건드리지 않음.
func TestResetSeries_NoMatch_Zero(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{MetricType: "temperature"}))

	cleared, deleted, err := a.ResetSeries(ctx, "default", "room", "pressure", nil)
	require.NoError(t, err)
	assert.Equal(t, 0, cleared)
	assert.Equal(t, 0, deleted)
}

// IsStaticKey fallback: 사용자 관점 key 로 "그 key 의 어떤 시리즈라도 정적이면 true".
func TestIsStaticKey_UserKeyFallback(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")
	// 인코딩 키로 레지스트리에 등록되는 시리즈.
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{MetricType: "temperature"}))

	encoded := EncodeSeriesKey(SeriesID{Key: "room", MetricType: "temperature"})

	// primary: 인코딩 키 직접 조회 적중.
	assert.True(t, a.IsStaticKey(encoded), "인코딩 키 직접 조회로 true")
	// fallback: 사용자 관점 key "room" 으로도 true (그 key 의 시리즈가 정적).
	assert.True(t, a.IsStaticKey("room"), "사용자 key 로도 그 key 의 시리즈가 정적이면 true")
	// 미존재 사용자 key 는 false.
	assert.False(t, a.IsStaticKey("nonexistent"))
	assert.False(t, a.IsStaticKey(""))
}
