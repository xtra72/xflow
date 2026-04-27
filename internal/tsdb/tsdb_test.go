package tsdb

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestTSDB(t *testing.T) TSDB {
	t.Helper()
	ResetMemoryUsage()

	cfg := DefaultConfig()
	cfg.EvictionInterval = 1 * time.Hour // 테스트 중 자동 퇴거 방지
	return New(cfg)
}

func TestTSDB_WriteAndQuery(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	// 쓰기
	err := db.Write("cpu", map[string]string{"host": "server01"}, map[string]any{"usage": float64(75.5)})
	require.NoError(t, err)

	// 키 확인
	keys := db.SeriesKeys()
	assert.Equal(t, 1, len(keys))
	assert.Equal(t, "cpu,host=server01", keys[0])

	// QueryRange
	start := time.Now().Add(-1 * time.Minute)
	end := time.Now().Add(1 * time.Minute)
	points, err := db.QueryRange("cpu,host=server01", start, end)
	require.NoError(t, err)
	assert.Equal(t, 1, len(points))
	assert.Equal(t, float64(75.5), points[0].Fields["usage"])
}

func TestTSDB_Latest(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	// 여러 포인트 쓰기
	for i := range 5 {
		err := db.Write("temp", nil, map[string]any{"value": float64(i)})
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond) // 타임스탬프 차이 보장
	}

	points, err := db.Latest("temp", 3)
	require.NoError(t, err)
	assert.Equal(t, 3, len(points))
}

func TestTSDB_DeleteSeries(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	err := db.Write("cpu", map[string]string{"host": "a"}, map[string]any{"v": float64(1)})
	require.NoError(t, err)

	err = db.Write("cpu", map[string]string{"host": "b"}, map[string]any{"v": float64(2)})
	require.NoError(t, err)

	assert.Equal(t, 2, len(db.SeriesKeys()))

	// 삭제
	err = db.DeleteSeries("cpu,host=a")
	require.NoError(t, err)

	assert.Equal(t, 1, len(db.SeriesKeys()))

	// 삭제된 시리즈 조회 시 에러
	_, err = db.QueryRange("cpu,host=a", time.Time{}, time.Now())
	assert.ErrorIs(t, err, ErrSeriesNotFound)
}

func TestTSDB_DeleteSeries_NotFound(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	err := db.DeleteSeries("nonexistent")
	assert.ErrorIs(t, err, ErrSeriesNotFound)
}

func TestTSDB_Stats(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	// 빈 상태
	stats := db.Stats()
	assert.Equal(t, 0, stats.SeriesCount)
	assert.Equal(t, int64(0), stats.TotalPoints)

	// 데이터 쓰기
	for i := range 3 {
		err := db.Write("cpu", map[string]string{"host": "s1"}, map[string]any{"v": float64(i)})
		require.NoError(t, err)
	}
	err := db.Write("mem", nil, map[string]any{"v": float64(99)})
	require.NoError(t, err)

	stats = db.Stats()
	assert.Equal(t, 2, stats.SeriesCount)
	assert.Equal(t, int64(4), stats.TotalPoints)
	assert.Greater(t, stats.MemoryBytes, int64(0))
	assert.False(t, stats.OldestPoint.IsZero())
	assert.False(t, stats.NewestPoint.IsZero())
}

func TestTSDB_Close(t *testing.T) {
	db := newTestTSDB(t)

	err := db.Write("test", nil, map[string]any{"v": float64(1)})
	require.NoError(t, err)

	// 닫기
	err = db.Close()
	require.NoError(t, err)

	// 닫힌 후 Write 시도
	err = db.Write("test", nil, map[string]any{"v": float64(2)})
	assert.ErrorIs(t, err, ErrTSDBClosed)

	// 닫힌 후 QueryRange 시도
	_, err = db.QueryRange("test", time.Time{}, time.Now())
	assert.ErrorIs(t, err, ErrTSDBClosed)

	// 닫힌 후 Latest 시도
	_, err = db.Latest("test", 10)
	assert.ErrorIs(t, err, ErrTSDBClosed)

	// 닫힌 후 DeleteSeries 시도
	err = db.DeleteSeries("test")
	assert.ErrorIs(t, err, ErrTSDBClosed)

	// 두 번 닫기
	err = db.Close()
	assert.ErrorIs(t, err, ErrTSDBClosed)
}

func TestTSDB_WriteBatch(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	now := time.Now()
	requests := []WriteRequest{
		{
			Measurement: "cpu",
			Tags:        map[string]string{"host": "s1"},
			Fields:      map[string]any{"usage": float64(50)},
			Timestamp:   now,
		},
		{
			Measurement: "cpu",
			Tags:        map[string]string{"host": "s2"},
			Fields:      map[string]any{"usage": float64(60)},
			Timestamp:   now,
		},
		{
			Measurement: "mem",
			Fields:      map[string]any{"free": int64(1024)},
			// zero timestamp -> time.Now() 사용
		},
	}

	err := db.WriteBatch(requests)
	require.NoError(t, err)

	keys := db.SeriesKeys()
	assert.Equal(t, 3, len(keys))

	stats := db.Stats()
	assert.Equal(t, int64(3), stats.TotalPoints)
}

func TestTSDB_WriteBatch_InvalidMeasurement(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	requests := []WriteRequest{
		{Measurement: "cpu", Fields: map[string]any{"v": float64(1)}},
		{Measurement: "", Fields: map[string]any{"v": float64(2)}}, // 유효하지 않은 measurement
	}

	err := db.WriteBatch(requests)
	assert.ErrorIs(t, err, ErrInvalidMeasurement)
}

func TestTSDB_WriteBatch_InvalidFields(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	requests := []WriteRequest{
		{Measurement: "cpu", Fields: nil},
	}

	err := db.WriteBatch(requests)
	assert.ErrorIs(t, err, ErrInvalidField)
}

func TestTSDB_Write_InvalidMeasurement(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	err := db.Write("", nil, map[string]any{"v": float64(1)})
	assert.ErrorIs(t, err, ErrInvalidMeasurement)
}

func TestTSDB_Write_InvalidFields(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	err := db.Write("cpu", nil, nil)
	assert.ErrorIs(t, err, ErrInvalidField)

	err = db.Write("cpu", nil, map[string]any{})
	assert.ErrorIs(t, err, ErrInvalidField)
}

func TestTSDB_MaxSeriesExceeded(t *testing.T) {
	ResetMemoryUsage()
	cfg := DefaultConfig()
	cfg.MaxSeries = 2
	cfg.EvictionInterval = 1 * time.Hour
	db := New(cfg)
	defer db.Close()

	err := db.Write("cpu", map[string]string{"host": "s1"}, map[string]any{"v": float64(1)})
	require.NoError(t, err)

	err = db.Write("cpu", map[string]string{"host": "s2"}, map[string]any{"v": float64(2)})
	require.NoError(t, err)

	// 세 번째 시리즈는 초과
	err = db.Write("cpu", map[string]string{"host": "s3"}, map[string]any{"v": float64(3)})
	assert.ErrorIs(t, err, ErrMaxSeriesExceeded)
}

func TestTSDB_MaxQueryPointsExceeded(t *testing.T) {
	ResetMemoryUsage()
	cfg := DefaultConfig()
	cfg.MaxQueryPoints = 5
	cfg.EvictionInterval = 1 * time.Hour
	db := New(cfg)
	defer db.Close()

	// 10개 포인트 쓰기
	for i := range 10 {
		err := db.Write("test", nil, map[string]any{"v": float64(i)})
		require.NoError(t, err)
		time.Sleep(1 * time.Millisecond)
	}

	// MaxQueryPoints 초과하는 범위 조회
	_, err := db.QueryRange("test", time.Time{}, time.Now().Add(time.Hour))
	assert.ErrorIs(t, err, ErrMaxPointsExceeded)
}

func TestTSDB_QueryRange_SeriesNotFound(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	_, err := db.QueryRange("nonexistent", time.Time{}, time.Now())
	assert.ErrorIs(t, err, ErrSeriesNotFound)
}

func TestTSDB_Latest_SeriesNotFound(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	_, err := db.Latest("nonexistent", 10)
	assert.ErrorIs(t, err, ErrSeriesNotFound)
}

func TestTSDB_ConcurrentAccess(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	var wg sync.WaitGroup
	writers := 10
	pointsPerWriter := 50

	// 동시 쓰기
	wg.Add(writers)
	for w := range writers {
		go func(wid int) {
			defer wg.Done()
			for i := range pointsPerWriter {
				_ = db.Write("cpu",
					map[string]string{"host": "s1"},
					map[string]any{"v": float64(wid*pointsPerWriter + i)},
				)
			}
		}(w)
	}

	// 동시 읽기
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 100 {
			_ = db.SeriesKeys()
			_ = db.Stats()
			_, _ = db.Latest("cpu,host=s1", 5)
		}
	}()

	wg.Wait()

	stats := db.Stats()
	assert.Equal(t, int64(writers*pointsPerWriter), stats.TotalPoints)
}

func TestTSDB_WriteBatch_Closed(t *testing.T) {
	db := newTestTSDB(t)
	require.NoError(t, db.Close())

	err := db.WriteBatch([]WriteRequest{
		{Measurement: "cpu", Fields: map[string]any{"v": float64(1)}},
	})
	assert.ErrorIs(t, err, ErrTSDBClosed)
}

func TestTSDB_MultipleSeriesWrite(t *testing.T) {
	db := newTestTSDB(t)
	defer db.Close()

	// 다양한 시리즈에 쓰기
	require.NoError(t, db.Write("cpu", map[string]string{"host": "s1", "region": "us"}, map[string]any{"v": float64(1)}))
	require.NoError(t, db.Write("cpu", map[string]string{"host": "s1", "region": "eu"}, map[string]any{"v": float64(2)}))
	require.NoError(t, db.Write("mem", map[string]string{"host": "s1"}, map[string]any{"v": float64(3)}))
	require.NoError(t, db.Write("disk", nil, map[string]any{"v": float64(4)}))

	keys := db.SeriesKeys()
	assert.Equal(t, 4, len(keys))

	stats := db.Stats()
	assert.Equal(t, 4, stats.SeriesCount)
	assert.Equal(t, int64(4), stats.TotalPoints)
}
