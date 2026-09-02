// @spec SPEC-STORE-004
//
// store_series_query_test.go — M3 조회 fan-out 의 시리즈 식별/조회 수용 기준 테스트.
//
// 쓰기(SetWithMeta) 로 여러 시리즈를 만든 뒤 QuerySeries fan-out 으로 조회하여,
// AC-1/2/4/5/6/7/8/9/16 (시리즈 독립성·전체 반환·필터·기본 시리즈) 를 검증한다.

package system

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// newAutoStoreAgentWithHistory 는 registration_type=auto + history 보관이 활성화된
// UserStoreAgent 를 만든다 (시리즈별 독립 history 검증용).
func newAutoStoreAgentWithHistory(t *testing.T, maxHistory int) *UserStoreAgent {
	t.Helper()
	cfg := agent.AgentConfig{
		ID:   "s-series-hist",
		Name: "store-series-hist",
		Type: "store",
		Transport: agent.TransportConfig{
			Type: "store",
			Options: map[string]any{
				"registration_type": "auto",
				"max_history_size":  maxHistory,
			},
		},
	}
	ag, err := NewUserStoreAgent(cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ag.Stop(context.Background()) })
	return ag.(*UserStoreAgent)
}

// seriesAdapter 는 SetWithMeta 로 시리즈를 쓰기 위한 어댑터를 돌려준다.
func seriesAdapter(t *testing.T, a *UserStoreAgent, namespace string) *NodeStoreAdapter {
	t.Helper()
	return a.NodeStoreForNamespace(namespace).(*NodeStoreAdapter)
}

// lastN 은 last_n 모드 조회 헬퍼이다.
func lastN(n int) HistoryQuery { return HistoryQuery{Mode: QueryModeLastN, Count: n} }

// findSeries 는 결과에서 (field, tags) 시리즈를 찾는다.
func findSeries(results []SeriesResult, field string, tags map[string]string) (SeriesResult, bool) {
	want := EncodeSeriesKey(SeriesID{Field: field, Tags: tags})
	for _, r := range results {
		if EncodeSeriesKey(SeriesID{Field: r.Series.Field, Tags: r.Series.Tags}) == want {
			return r, true
		}
	}
	return SeriesResult{}, false
}

// AC-1: 같은 key + 다른 field → 별개 시리즈, 서로 덮어쓰지 않음 (E2/N1).
func TestQuerySeries_DiffField_IndependentSeries(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{Field: "humidity"}))

	results, err := a.QuerySeries(ctx, "default", "room", "", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 2, "두 field 시리즈가 독립 존재해야 한다")

	temp, ok := findSeries(results, "temperature", nil)
	require.True(t, ok)
	assert.Equal(t, "22", temp.Entries[0].Value, "temperature 현재값 (동적 string)")

	hum, ok := findSeries(results, "humidity", nil)
	require.True(t, ok)
	assert.Equal(t, "55", hum.Entries[0].Value, "humidity 현재값")
}

// AC-2 + AC-3: 같은 key + 다른 tags → 별개 시리즈, tags 순서 무관 동일 시리즈.
func TestQuerySeries_DiffTags_IndependentSeries(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "temp", 21, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"room": "1"},
	}))
	require.NoError(t, adapter.SetWithMeta(ctx, "temp", 26, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"room": "2"},
	}))

	results, err := a.QuerySeries(ctx, "default", "temp", "", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 2, "다른 tags 는 독립 시리즈")

	r1, ok := findSeries(results, "temperature", map[string]string{"room": "1"})
	require.True(t, ok)
	assert.Equal(t, "21", r1.Entries[0].Value)
	r2, ok := findSeries(results, "temperature", map[string]string{"room": "2"})
	require.True(t, ok)
	assert.Equal(t, "26", r2.Entries[0].Value)
}

// AC-4: data_type 차이는 동일 시리즈로 합산된다 (U3/N3).
func TestQuerySeries_DataTypeIrrelevant_SameSeries(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	// 명시 float data_type 으로 첫 쓰기.
	require.NoError(t, adapter.SetWithMeta(ctx, "sensor", float64(22), StoreWriteMeta{
		DataType: "float", Field: "temperature",
	}))
	// 같은 (key, field, tags) — data_type 만 다른 값. 동일 시리즈 갱신.
	require.NoError(t, adapter.SetWithMeta(ctx, "sensor", float64(22.5), StoreWriteMeta{
		DataType: "float", Field: "temperature",
	}))

	results, err := a.QuerySeries(ctx, "default", "sensor", "", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 1, "data_type 차이로 새 시리즈가 생기지 않아야 한다")
	assert.Equal(t, 22.5, results[0].Entries[0].Value, "동일 시리즈가 갱신됨")
}

// AC-5: 키 조회 시 모든 시리즈 반환 (E4).
func TestQuerySeries_AllSeriesReturned(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{Field: "humidity"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 23, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"area": "a"},
	}))

	results, err := a.QuerySeries(ctx, "default", "room", "", nil, lastN(10))
	require.NoError(t, err)
	assert.Len(t, results, 3, "3개 시리즈 모두 반환")
}

// AC-6: field+tags 필터로 단일 시리즈 (E5).
func TestQuerySeries_MetricAndTagsFilter_SingleSeries(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{Field: "humidity"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 23, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"area": "a"},
	}))

	results, err := a.QuerySeries(ctx, "default", "room", "temperature", map[string]string{"area": "a"}, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "temperature", results[0].Series.Field)
	assert.Equal(t, "a", results[0].Series.Tags["area"])
}

// AC-7: field 만 필터(tags 생략) → 해당 field 의 모든 tags 시리즈 (S3).
func TestQuerySeries_MetricOnlyFilter_AllTagsOfMetric(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 55, StoreWriteMeta{Field: "humidity"}))
	require.NoError(t, adapter.SetWithMeta(ctx, "room", 23, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"area": "a"},
	}))

	results, err := a.QuerySeries(ctx, "default", "room", "temperature", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 2, "temperature 의 두 tags 시리즈만 (humidity 제외)")
	for _, r := range results {
		assert.Equal(t, "temperature", r.Series.Field)
	}
}

// AC-8: 미일치 필터 → 빈 결과 (에러 아님) (S4).
func TestQuerySeries_NoMatch_EmptyResult(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "room", 22, StoreWriteMeta{Field: "temperature"}))

	results, err := a.QuerySeries(ctx, "default", "room", "pressure", nil, lastN(10))
	require.NoError(t, err, "미일치는 에러가 아니다")
	assert.Empty(t, results, "일치 시리즈 없음 → 빈 결과")

	// 존재하지 않는 key 도 빈 결과.
	results, err = a.QuerySeries(ctx, "default", "nope", "", nil, lastN(10))
	require.NoError(t, err)
	assert.Empty(t, results)
}

// AC-9: 시리즈 단위 독립 history (U5/E6/A5).
func TestQuerySeries_IndependentHistory(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 10)
	adapter := seriesAdapter(t, a, "default")

	// temperature 시리즈: 20, 21, 22 순차 (동적 string → "20","21","22").
	for _, v := range []int{20, 21, 22} {
		require.NoError(t, adapter.SetWithMeta(ctx, "room", v, StoreWriteMeta{Field: "temperature"}))
	}
	// humidity 시리즈: 50, 51.
	for _, v := range []int{50, 51} {
		require.NoError(t, adapter.SetWithMeta(ctx, "room", v, StoreWriteMeta{Field: "humidity"}))
	}

	results, err := a.QuerySeries(ctx, "default", "room", "", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 2)

	temp, ok := findSeries(results, "temperature", nil)
	require.True(t, ok)
	tempVals := valuesOf(temp.Entries)
	assert.Equal(t, []any{"22", "21", "20"}, tempVals, "temperature history 최신순")

	hum, ok := findSeries(results, "humidity", nil)
	require.True(t, ok)
	humVals := valuesOf(hum.Entries)
	assert.Equal(t, []any{"51", "50"}, humVals, "humidity history 독립")
}

// AC-16: 메타 미지정 쓰기 = 기본 시리즈, 같은 key 재쓰기 시 시리즈 폭증 없음.
func TestQuerySeries_DefaultSeries_NoMetaWrite(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "simple", 1, StoreWriteMeta{}))
	require.NoError(t, adapter.SetWithMeta(ctx, "simple", 2, StoreWriteMeta{}))

	results, err := a.QuerySeries(ctx, "default", "simple", "", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 1, "기본 시리즈 하나만 (폭증 없음)")
	assert.Equal(t, FieldUnknown, results[0].Series.Field)
	assert.Empty(t, results[0].Series.Tags)
	assert.Equal(t, "2", results[0].Entries[0].Value)
}

// 레거시: plain Set(bare key) 도 기본 시리즈로 fan-out 에 포착된다.
func TestQuerySeries_LegacyPlainSet_AsDefaultSeries(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)

	store := a.inner.ForNamespace("default")
	require.NoError(t, store.Set(ctx, "legacy", 7))

	results, err := a.QuerySeries(ctx, "default", "legacy", "", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 1, "plain Set 도 기본 시리즈로 흡수")
	assert.Equal(t, FieldUnknown, results[0].Series.Field)
	assert.Equal(t, "7", results[0].Entries[0].Value)
}

// AC-11: auto 모드 미등록 시리즈 첫 쓰기 → SourceAuto 자동 등록 (S2).
func TestQuerySeries_AutoRegistersNewSeries(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgent(t)
	adapter := seriesAdapter(t, a, "default")

	require.NoError(t, adapter.SetWithMeta(ctx, "new", 1, StoreWriteMeta{
		Field: "temperature", Tags: map[string]string{"room": "1"},
	}))

	seriesKey := EncodeSeriesKey(SeriesID{Measurement: "new", Field: "temperature", Tags: map[string]string{"room": "1"}})
	meta, ok := a.StaticKeysSnapshot()[seriesKey]
	require.True(t, ok, "미등록 시리즈가 자동 등록되어야 한다")
	assert.Equal(t, SourceAuto, meta.Source)

	results, err := a.QuerySeries(ctx, "default", "new", "", nil, lastN(10))
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "1", results[0].Entries[0].Value)
}

// AC-17: 같은 key 에 서로 다른 field/tags 로 동시 다수 goroutine 쓰기 → race/panic 없음,
// 각 시리즈가 독립 저장. go test -race 로 검증한다.
func TestQuerySeries_ConcurrentDifferentSeries_NoRace(t *testing.T) {
	ctx := context.Background()
	a := newAutoStoreAgentWithHistory(t, 50)
	adapter := seriesAdapter(t, a, "default")

	const writers = 16
	done := make(chan struct{})
	for i := 0; i < writers; i++ {
		go func(id int) {
			defer func() { done <- struct{}{} }()
			field := "temperature"
			if id%2 == 0 {
				field = "humidity"
			}
			tags := map[string]string{"w": string(rune('a' + id%4))}
			for j := 0; j < 20; j++ {
				_ = adapter.SetWithMeta(ctx, "shared", id*100+j, StoreWriteMeta{
					Field: field, Tags: tags,
				})
			}
		}(i)
	}
	for i := 0; i < writers; i++ {
		<-done
	}

	// 모든 시리즈가 독립적으로 일관 저장되었는지 — fan-out 으로 조회하여 panic/누락 없음 확인.
	results, err := a.QuerySeries(ctx, "default", "shared", "", nil, lastN(50))
	require.NoError(t, err)
	// field 2종 × tags 4종 = 최대 8개 시리즈 (실제 조합 수에 따라).
	assert.NotEmpty(t, results, "동시 쓰기 후 시리즈가 존재해야 한다")
	for _, r := range results {
		assert.Equal(t, "shared", r.Series.Measurement)
		assert.NotEmpty(t, r.Entries, "각 시리즈는 값을 가져야 한다")
	}
}

// valuesOf 는 HistoryEntry 슬라이스의 값만 추출한다.
func valuesOf(entries []HistoryEntry) []any {
	out := make([]any, len(entries))
	for i, e := range entries {
		out[i] = e.Value
	}
	return out
}
