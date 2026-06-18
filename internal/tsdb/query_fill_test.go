package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// first/last 집계 검증.
func TestAggregate_FirstLast(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	points := []DataPoint{
		{Timestamp: base, Fields: map[string]any{"v": float64(10)}},
		{Timestamp: base.Add(time.Minute), Fields: map[string]any{"v": float64(20)}},
		{Timestamp: base.Add(2 * time.Minute), Fields: map[string]any{"v": float64(30)}},
	}
	first, err := aggregate(points, "v", AggFirst)
	require.NoError(t, err)
	assert.Equal(t, 10.0, first)

	last, err := aggregate(points, "v", AggLast)
	require.NoError(t, err)
	assert.Equal(t, 30.0, last)
}

// downsampleFilled: 빈 버킷이 fill 전략대로 채워지는지 검증.
// 데이터: 0분(=10), 30분(=40). 15분 버킷 → 버킷 0/15/30/45 중 15·45는 비어 있음.
func TestDownsampleFilled(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	start := base
	end := base.Add(60 * time.Minute)
	points := []DataPoint{
		{Timestamp: base, Fields: map[string]any{"v": float64(10)}},
		{Timestamp: base.Add(30 * time.Minute), Fields: map[string]any{"v": float64(40)}},
	}
	interval := 15 * time.Minute
	// 버킷: 0(10), 15(빈), 30(40), 45(빈)

	t.Run("previous", func(t *testing.T) {
		r := downsampleFilled(points, "v", AggLast, interval, start, end, FillPrevious)
		require.Len(t, r, 4)
		assert.Equal(t, 10.0, r[0].Fields["v"]) // 0
		assert.Equal(t, 10.0, r[1].Fields["v"]) // 15 ← 이전값 10
		assert.Equal(t, 40.0, r[2].Fields["v"]) // 30
		assert.Equal(t, 40.0, r[3].Fields["v"]) // 45 ← 이전값 40
	})

	t.Run("avg", func(t *testing.T) {
		r := downsampleFilled(points, "v", AggLast, interval, start, end, FillAvg)
		require.Len(t, r, 4)
		assert.Equal(t, 10.0, r[0].Fields["v"])
		assert.Equal(t, 25.0, r[1].Fields["v"]) // 15 ← (10+40)/2
		assert.Equal(t, 40.0, r[2].Fields["v"])
		assert.Equal(t, 40.0, r[3].Fields["v"]) // 45 ← 이후값 없음 → 이전값 40
	})

	t.Run("zero", func(t *testing.T) {
		r := downsampleFilled(points, "v", AggLast, interval, start, end, FillZero)
		require.Len(t, r, 4)
		assert.Equal(t, 0.0, r[1].Fields["v"])
		assert.Equal(t, 0.0, r[3].Fields["v"])
	})

	t.Run("null", func(t *testing.T) {
		r := downsampleFilled(points, "v", AggLast, interval, start, end, FillNull)
		require.Len(t, r, 4)
		assert.Nil(t, r[1].Fields["v"]) // 비우기 → null
		assert.Nil(t, r[3].Fields["v"])
	})
}

// 선두 빈 버킷은 previous 시 null(이전값 없음).
func TestDownsampleFilled_LeadingEmpty_Previous(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	start := base
	end := base.Add(30 * time.Minute)
	// 데이터는 15분 버킷에만 존재 → 0분 버킷은 선두 빈 버킷.
	points := []DataPoint{
		{Timestamp: base.Add(15 * time.Minute), Fields: map[string]any{"v": float64(50)}},
	}
	r := downsampleFilled(points, "v", AggLast, 15*time.Minute, start, end, FillPrevious)
	require.Len(t, r, 2)
	assert.Nil(t, r[0].Fields["v"]) // 선두 빈 버킷 → 이전값 없음 → null
	assert.Equal(t, 50.0, r[1].Fields["v"])
}

// FillNone(기본)은 Execute 경로에서 기존 downsample(빈 버킷 생략)을 쓴다.
func TestExecute_Downsample_FillPrevious(t *testing.T) {
	cfg := DefaultConfig()
	db := New(cfg).(*defaultTSDB)
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	s, err := db.getOrCreateSeries("k")
	require.NoError(t, err)
	require.NoError(t, s.Write(DataPoint{Timestamp: base, Fields: map[string]any{"value": float64(10)}}))
	require.NoError(t, s.Write(DataPoint{Timestamp: base.Add(30 * time.Minute), Fields: map[string]any{"value": float64(40)}}))

	results, err := db.Execute(Query{
		SeriesKey:      "k",
		Start:          base,
		End:            base.Add(60 * time.Minute),
		Aggregation:    AggLast,
		BucketInterval: 15 * time.Minute,
		Fill:           FillPrevious,
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	pts := results[0].Points
	require.Len(t, pts, 4) // 빈 버킷 포함 4개
	assert.Equal(t, 10.0, pts[1].Fields["value"])
}
