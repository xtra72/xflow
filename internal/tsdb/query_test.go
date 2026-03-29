package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- parseSeriesKey 테스트 ---

func TestParseSeriesKey(t *testing.T) {
	tests := []struct {
		name            string
		key             string
		wantMeasurement string
		wantTags        map[string]string
	}{
		{
			name:            "measurement만 있는 경우",
			key:             "cpu",
			wantMeasurement: "cpu",
			wantTags:        map[string]string{},
		},
		{
			name:            "measurement + 태그 1개",
			key:             "cpu,host=server01",
			wantMeasurement: "cpu",
			wantTags:        map[string]string{"host": "server01"},
		},
		{
			name:            "measurement + 태그 여러 개",
			key:             "cpu,host=server01,region=us-east",
			wantMeasurement: "cpu",
			wantTags:        map[string]string{"host": "server01", "region": "us-east"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, tags := parseSeriesKey(tt.key)
			assert.Equal(t, tt.wantMeasurement, m)
			assert.Equal(t, tt.wantTags, tags)
		})
	}
}

// --- toFloat64 테스트 ---

func TestToFloat64(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		want    float64
		wantOk  bool
	}{
		{"float64", float64(3.14), 3.14, true},
		{"int64", int64(42), 42.0, true},
		{"int", int(10), 10.0, true},
		{"float32", float32(1.5), float64(float32(1.5)), true},
		{"string은 실패", "hello", 0, false},
		{"bool은 실패", true, 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := toFloat64(tt.input)
			assert.Equal(t, tt.wantOk, ok)
			if ok {
				assert.InDelta(t, tt.want, got, 0.001)
			}
		})
	}
}

// --- aggregate 테스트 ---

func TestAggregate(t *testing.T) {
	now := time.Now()
	points := []DataPoint{
		{Timestamp: now, Fields: map[string]any{"value": float64(10)}},
		{Timestamp: now.Add(1 * time.Second), Fields: map[string]any{"value": float64(20)}},
		{Timestamp: now.Add(2 * time.Second), Fields: map[string]any{"value": float64(30)}},
		{Timestamp: now.Add(3 * time.Second), Fields: map[string]any{"value": float64(5)}},
		{Timestamp: now.Add(4 * time.Second), Fields: map[string]any{"value": float64(15)}},
	}

	tests := []struct {
		name    string
		fn      AggregateFunc
		want    float64
		wantErr bool
	}{
		{"min", AggMin, 5, false},
		{"max", AggMax, 30, false},
		{"avg", AggAvg, 16, false},
		{"sum", AggSum, 80, false},
		{"count", AggCount, 5, false},
		{"first", AggFirst, 10, false},
		{"last", AggLast, 15, false},
		{"unknown", AggregateFunc("unknown"), 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := aggregate(points, "value", tt.fn)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

func TestAggregate_EmptyPoints(t *testing.T) {
	_, err := aggregate(nil, "value", AggMin)
	assert.ErrorIs(t, err, ErrEmptyResult)

	_, err = aggregate([]DataPoint{}, "value", AggAvg)
	assert.ErrorIs(t, err, ErrEmptyResult)
}

// --- downsample 테스트 ---

func TestDownsample(t *testing.T) {
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 1시간 동안 5분 간격 데이터 (12포인트)
	var points []DataPoint
	for i := 0; i < 12; i++ {
		points = append(points, DataPoint{
			Timestamp: base.Add(time.Duration(i) * 5 * time.Minute),
			Fields:    map[string]any{"temp": float64(20 + i)},
		})
	}

	// 15분 버킷으로 다운샘플링 (avg)
	result := downsample(points, "temp", AggAvg, 15*time.Minute)

	// 0-14분: 20,21,22 -> avg=21
	// 15-29분: 23,24,25 -> avg=24
	// 30-44분: 26,27,28 -> avg=27
	// 45-59분: 29,30,31 -> avg=30
	require.Len(t, result, 4)

	assert.InDelta(t, 21.0, result[0].Fields["temp"].(float64), 0.001)
	assert.InDelta(t, 24.0, result[1].Fields["temp"].(float64), 0.001)
	assert.InDelta(t, 27.0, result[2].Fields["temp"].(float64), 0.001)
	assert.InDelta(t, 30.0, result[3].Fields["temp"].(float64), 0.001)

	// 버킷 시작 시간 확인
	assert.Equal(t, base, result[0].Timestamp)
	assert.Equal(t, base.Add(15*time.Minute), result[1].Timestamp)
	assert.Equal(t, base.Add(30*time.Minute), result[2].Timestamp)
	assert.Equal(t, base.Add(45*time.Minute), result[3].Timestamp)
}

func TestDownsample_EmptyPoints(t *testing.T) {
	result := downsample(nil, "value", AggAvg, 15*time.Minute)
	assert.Nil(t, result)
}

// --- Execute 테스트 ---

func newQueryTestTSDB(t *testing.T) *defaultTSDB {
	t.Helper()
	ResetMemoryUsage()

	cfg := DefaultConfig()
	cfg.EvictionInterval = 1 * time.Hour
	cfg.QueryTimeout = 5 * time.Second
	db := New(cfg).(*defaultTSDB)
	return db
}

func writeTestPoints(t *testing.T, db TSDB, measurement string, tags map[string]string, field string, values []float64, baseTime time.Time) {
	t.Helper()
	for i, v := range values {
		key, err := BuildSeriesKey(measurement, tags)
		require.NoError(t, err)

		s, err := db.(*defaultTSDB).getOrCreateSeries(key)
		require.NoError(t, err)

		dp := DataPoint{
			Timestamp: baseTime.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{field: v},
		}
		require.NoError(t, s.Write(dp))
	}
}

func TestExecute_RawQuery(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s1"}, "usage", []float64{10, 20, 30}, base)

	results, err := db.Execute(Query{
		SeriesKey: "cpu,host=s1",
		Start:     base,
		End:       base.Add(5 * time.Second),
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Points, 3)
	assert.Equal(t, "cpu,host=s1", results[0].SeriesKey)
	assert.Equal(t, 3, results[0].Stats.ScannedPoints)
}

func TestExecute_Aggregation(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s1"}, "usage", []float64{10, 20, 30, 40, 50}, base)

	results, err := db.Execute(Query{
		SeriesKey:   "cpu,host=s1",
		Start:       base,
		End:         base.Add(10 * time.Second),
		Field:       "usage",
		Aggregation: AggAvg,
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Points, 1)
	assert.InDelta(t, 30.0, results[0].Points[0].Fields["usage"].(float64), 0.001)
}

func TestExecute_Downsample(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 6개 포인트: 0s,1s,2s,3s,4s,5s
	writeTestPoints(t, db, "temp", map[string]string{"room": "a"}, "value",
		[]float64{10, 20, 30, 40, 50, 60}, base)

	results, err := db.Execute(Query{
		SeriesKey:      "temp,room=a",
		Start:          base,
		End:            base.Add(10 * time.Second),
		Field:          "value",
		Aggregation:    AggSum,
		BucketInterval: 3 * time.Second,
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	// 버킷 0-2s: 10+20+30=60, 버킷 3-5s: 40+50+60=150
	require.Len(t, results[0].Points, 2)
	assert.InDelta(t, 60.0, results[0].Points[0].Fields["value"].(float64), 0.001)
	assert.InDelta(t, 150.0, results[0].Points[1].Fields["value"].(float64), 0.001)
}

func TestExecute_FilterByMeasurement(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s1"}, "usage", []float64{10}, base)
	writeTestPoints(t, db, "mem", map[string]string{"host": "s1"}, "usage", []float64{80}, base)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s2"}, "usage", []float64{20}, base)

	results, err := db.Execute(Query{
		Measurement: "cpu",
		Start:       base,
		End:         base.Add(5 * time.Second),
	})

	require.NoError(t, err)
	assert.Len(t, results, 2) // cpu,host=s1 과 cpu,host=s2
}

func TestExecute_FilterByTags(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s1", "region": "us"}, "usage", []float64{10}, base)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s2", "region": "eu"}, "usage", []float64{20}, base)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s3", "region": "us"}, "usage", []float64{30}, base)

	results, err := db.Execute(Query{
		Measurement: "cpu",
		TagFilters:  map[string]string{"region": "us"},
		Start:       base,
		End:         base.Add(5 * time.Second),
	})

	require.NoError(t, err)
	assert.Len(t, results, 2) // s1, s3 (region=us)
}

func TestFilterSeries(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s1"}, "usage", []float64{1}, base)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s2"}, "usage", []float64{2}, base)
	writeTestPoints(t, db, "mem", map[string]string{"host": "s1"}, "free", []float64{100}, base)

	tests := []struct {
		name        string
		measurement string
		tags        map[string]string
		wantCount   int
	}{
		{
			name:        "measurement 필터",
			measurement: "cpu",
			tags:        nil,
			wantCount:   2,
		},
		{
			name:        "태그 필터",
			measurement: "",
			tags:        map[string]string{"host": "s1"},
			wantCount:   2, // cpu,host=s1 + mem,host=s1
		},
		{
			name:        "measurement + 태그 필터",
			measurement: "cpu",
			tags:        map[string]string{"host": "s1"},
			wantCount:   1,
		},
		{
			name:        "매칭 없음",
			measurement: "disk",
			tags:        nil,
			wantCount:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := db.FilterSeries(tt.measurement, tt.tags)
			assert.Len(t, keys, tt.wantCount)
		})
	}
}

func TestExecute_MaxPointsLimit(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 100개 포인트 기록
	values := make([]float64, 100)
	for i := range values {
		values[i] = float64(i)
	}
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s1"}, "usage", values, base)

	// Limit을 10으로 설정하여 초과 테스트
	_, err := db.Execute(Query{
		SeriesKey: "cpu,host=s1",
		Start:     base,
		End:       base.Add(200 * time.Second),
		Limit:     10,
	})
	assert.ErrorIs(t, err, ErrMaxPointsExceeded)

	// Limit이 충분하면 성공
	results, err := db.Execute(Query{
		SeriesKey: "cpu,host=s1",
		Start:     base,
		End:       base.Add(200 * time.Second),
		Limit:     200,
	})
	require.NoError(t, err)
	assert.Len(t, results[0].Points, 100)
}

func TestExecute_ClosedDB(t *testing.T) {
	db := newQueryTestTSDB(t)
	db.Close()

	_, err := db.Execute(Query{
		SeriesKey: "cpu,host=s1",
		Start:     time.Now().Add(-1 * time.Hour),
		End:       time.Now(),
	})
	assert.ErrorIs(t, err, ErrTSDBClosed)
}

func TestExecute_SeriesNotFound(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	// 존재하지 않는 시리즈 키로 쿼리 - 에러 없이 빈 결과 반환
	results, err := db.Execute(Query{
		SeriesKey: "nonexistent,host=s1",
		Start:     time.Now().Add(-1 * time.Hour),
		End:       time.Now(),
	})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestAggregate_NoMatchingField(t *testing.T) {
	now := time.Now()
	points := []DataPoint{
		{Timestamp: now, Fields: map[string]any{"other": float64(10)}},
	}

	_, err := aggregate(points, "nonexistent", AggMin)
	assert.ErrorIs(t, err, ErrEmptyResult)
}

func TestAggregate_Int64Values(t *testing.T) {
	now := time.Now()
	points := []DataPoint{
		{Timestamp: now, Fields: map[string]any{"count": int64(10)}},
		{Timestamp: now.Add(time.Second), Fields: map[string]any{"count": int64(20)}},
	}

	val, err := aggregate(points, "count", AggSum)
	require.NoError(t, err)
	assert.InDelta(t, 30.0, val, 0.001)
}

func TestExecute_AggregationWithNoField(t *testing.T) {
	db := newQueryTestTSDB(t)
	defer db.Close()

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	writeTestPoints(t, db, "cpu", map[string]string{"host": "s1"}, "usage", []float64{10, 20, 30}, base)

	// Field를 비워두면 첫 번째 숫자 필드를 사용
	results, err := db.Execute(Query{
		SeriesKey:   "cpu,host=s1",
		Start:       base,
		End:         base.Add(10 * time.Second),
		Aggregation: AggAvg,
	})

	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Len(t, results[0].Points, 1)
	// 결과는 "value" 필드에 저장됨
	val, ok := results[0].Points[0].Fields["value"].(float64)
	assert.True(t, ok)
	assert.InDelta(t, 20.0, val, 0.001)
}
