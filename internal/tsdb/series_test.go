package tsdb

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildSeriesKey(t *testing.T) {
	tests := []struct {
		name        string
		measurement string
		tags        map[string]string
		expected    string
		wantErr     error
	}{
		{
			name:        "measurement만 있는 경우",
			measurement: "cpu",
			tags:        nil,
			expected:    "cpu",
			wantErr:     nil,
		},
		{
			name:        "빈 태그 맵",
			measurement: "cpu",
			tags:        map[string]string{},
			expected:    "cpu",
			wantErr:     nil,
		},
		{
			name:        "태그 하나",
			measurement: "cpu",
			tags:        map[string]string{"host": "server01"},
			expected:    "cpu,host=server01",
			wantErr:     nil,
		},
		{
			name:        "태그 여러 개 - 정렬 확인",
			measurement: "cpu",
			tags:        map[string]string{"region": "us-east", "host": "server01", "dc": "dc1"},
			expected:    "cpu,dc=dc1,host=server01,region=us-east",
			wantErr:     nil,
		},
		{
			name:        "빈 measurement",
			measurement: "",
			tags:        map[string]string{"host": "server01"},
			expected:    "",
			wantErr:     ErrInvalidMeasurement,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key, err := BuildSeriesKey(tt.measurement, tt.tags)
			if tt.wantErr != nil {
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, key)
			}
		})
	}
}

func TestBuildSeriesKey_TagSortingDeterministic(t *testing.T) {
	// 동일 태그를 여러 번 호출해도 동일한 키가 생성되어야 함
	tags := map[string]string{"z": "1", "a": "2", "m": "3"}

	key1, err := BuildSeriesKey("test", tags)
	require.NoError(t, err)

	for range 100 {
		key2, err := BuildSeriesKey("test", tags)
		require.NoError(t, err)
		assert.Equal(t, key1, key2)
	}
}

func TestNewSeries(t *testing.T) {
	s := NewSeries("cpu,host=server01")
	assert.Equal(t, "cpu,host=server01", s.Key())
	assert.Equal(t, 0, s.Len())
	assert.Equal(t, int64(0), s.MemorySize())
	assert.False(t, s.createdAt.IsZero())
}

func TestSeries_Write_Append(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()
	for i := range 5 {
		err := s.Write(DataPoint{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"v": float64(i)},
		})
		require.NoError(t, err)
	}

	assert.Equal(t, 5, s.Len())
	assert.Greater(t, s.MemorySize(), int64(0))
}

func TestSeries_Write_OutOfOrder(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()

	// 먼저 시간순으로 일부 삽입
	require.NoError(t, s.Write(DataPoint{
		Timestamp: now,
		Fields:    map[string]any{"v": float64(1)},
	}))
	require.NoError(t, s.Write(DataPoint{
		Timestamp: now.Add(2 * time.Second),
		Fields:    map[string]any{"v": float64(3)},
	}))

	// out-of-order 삽입
	require.NoError(t, s.Write(DataPoint{
		Timestamp: now.Add(1 * time.Second),
		Fields:    map[string]any{"v": float64(2)},
	}))

	assert.Equal(t, 3, s.Len())

	// 시간순으로 정렬되어 있는지 확인
	result := s.Latest(3)
	assert.Equal(t, float64(1), result[0].Fields["v"])
	assert.Equal(t, float64(2), result[1].Fields["v"])
	assert.Equal(t, float64(3), result[2].Fields["v"])
}

func TestSeries_Write_InvalidField(t *testing.T) {
	s := NewSeries("test")

	err := s.Write(DataPoint{
		Timestamp: time.Now(),
		Fields:    nil,
	})
	assert.ErrorIs(t, err, ErrInvalidField)

	err = s.Write(DataPoint{
		Timestamp: time.Now(),
		Fields:    map[string]any{},
	})
	assert.ErrorIs(t, err, ErrInvalidField)
}

func TestSeries_QueryRange(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)

	// 10개의 포인트 삽입 (1초 간격)
	for i := range 10 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: base.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"v": float64(i)},
		}))
	}

	tests := []struct {
		name     string
		start    time.Time
		end      time.Time
		expected int
	}{
		{
			name:     "전체 범위",
			start:    base,
			end:      base.Add(9 * time.Second),
			expected: 10,
		},
		{
			name:     "부분 범위",
			start:    base.Add(3 * time.Second),
			end:      base.Add(6 * time.Second),
			expected: 4,
		},
		{
			name:     "범위 밖 (이전)",
			start:    base.Add(-10 * time.Second),
			end:      base.Add(-1 * time.Second),
			expected: 0,
		},
		{
			name:     "범위 밖 (이후)",
			start:    base.Add(20 * time.Second),
			end:      base.Add(30 * time.Second),
			expected: 0,
		},
		{
			name:     "단일 포인트",
			start:    base.Add(5 * time.Second),
			end:      base.Add(5 * time.Second),
			expected: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.QueryRange(tt.start, tt.end)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

func TestSeries_QueryRange_Empty(t *testing.T) {
	s := NewSeries("test")
	result := s.QueryRange(time.Now(), time.Now().Add(time.Hour))
	assert.Nil(t, result)
}

func TestSeries_Latest(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()
	for i := range 5 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"v": float64(i)},
		}))
	}

	tests := []struct {
		name     string
		n        int
		expected int
		firstVal float64
	}{
		{"마지막 3개", 3, 3, float64(2)},
		{"마지막 1개", 1, 1, float64(4)},
		{"전체보다 많이 요청", 10, 5, float64(0)},
		{"0개 요청", 0, 0, 0},
		{"음수 요청", -1, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := s.Latest(tt.n)
			if tt.expected == 0 {
				assert.Nil(t, result)
			} else {
				assert.Equal(t, tt.expected, len(result))
				assert.Equal(t, tt.firstVal, result[0].Fields["v"])
			}
		})
	}
}

func TestSeries_Latest_ReturnsACopy(t *testing.T) {
	// Latest가 원본 수정에 영향받지 않는 복사본을 반환하는지 확인
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()
	require.NoError(t, s.Write(DataPoint{
		Timestamp: now,
		Fields:    map[string]any{"v": float64(1)},
	}))

	result := s.Latest(1)
	require.Len(t, result, 1)

	// 반환된 슬라이스 수정이 원본에 영향 없는지 확인
	result[0].Fields["v"] = float64(999)
	original := s.Latest(1)
	assert.Equal(t, float64(1), original[0].Fields["v"])
}

func TestSeries_Evict(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()
	for i := range 10 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: now.Add(time.Duration(i-9) * time.Hour), // -9h ~ 0h
			Fields:    map[string]any{"v": float64(i)},
		}))
	}

	// 5시간 보존 정책 적용
	// 포인트: -9h, -8h, -7h, -6h, -5h, -4h, -3h, -2h, -1h, 0h
	// cutoff = now - 5h = -5h 시점
	// -9h, -8h, -7h, -6h 는 cutoff 이전 -> 만료 (4개)
	// -5h 이후는 보존 (6개)
	globalPolicy := RetentionPolicy{MaxAge: 5 * time.Hour}
	evicted := s.Evict(globalPolicy, now)

	assert.Equal(t, 4, evicted)
	assert.Equal(t, 6, s.Len())
}

func TestSeries_Evict_SeriesPolicyOverridesGlobal(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()
	for i := range 10 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: now.Add(time.Duration(i-9) * time.Hour),
			Fields:    map[string]any{"v": float64(i)},
		}))
	}

	// 시리즈별 정책 설정 (3시간)
	// 포인트: -9h, -8h, -7h, -6h, -5h, -4h, -3h, -2h, -1h, 0h
	// cutoff = now - 3h = -3h 시점
	// -9h ~ -4h 까지 만료 (6개), -3h ~ 0h 보존 (4개)
	s.SetRetention(RetentionPolicy{MaxAge: 3 * time.Hour})

	// 글로벌 정책은 10시간이지만 시리즈 정책이 우선
	globalPolicy := RetentionPolicy{MaxAge: 10 * time.Hour}
	evicted := s.Evict(globalPolicy, now)

	assert.Equal(t, 6, evicted)
	assert.Equal(t, 4, s.Len())
}

func TestSeries_Evict_CountRetention(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()
	for i := range 10 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"v": float64(i)},
		}))
	}

	globalPolicy := RetentionPolicy{MaxPoints: 5}
	evicted := s.Evict(globalPolicy, now)

	assert.Equal(t, 5, evicted)
	assert.Equal(t, 5, s.Len())
}

func TestSeries_Evict_MemoryTracking(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("test")

	now := time.Now()
	for i := range 5 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: now.Add(time.Duration(i-4) * time.Hour),
			Fields:    map[string]any{"v": float64(i)},
		}))
	}

	memBefore := CurrentMemoryUsage()
	assert.Greater(t, memBefore, int64(0))

	globalPolicy := RetentionPolicy{MaxAge: 2 * time.Hour}
	s.Evict(globalPolicy, now)

	memAfter := CurrentMemoryUsage()
	assert.Less(t, memAfter, memBefore)
}

func TestSeries_ConcurrentWrite(t *testing.T) {
	// 동시 쓰기 안전성 테스트
	ResetMemoryUsage()
	s := NewSeries("test")

	var wg sync.WaitGroup
	goroutines := 50
	pointsPerGoroutine := 20

	wg.Add(goroutines)
	for g := range goroutines {
		go func(gid int) {
			defer wg.Done()
			for i := range pointsPerGoroutine {
				now := time.Now().Add(time.Duration(gid*pointsPerGoroutine+i) * time.Millisecond)
				_ = s.Write(DataPoint{
					Timestamp: now,
					Fields:    map[string]any{"v": float64(gid*pointsPerGoroutine + i)},
				})
			}
		}(g)
	}

	wg.Wait()
	assert.Equal(t, goroutines*pointsPerGoroutine, s.Len())
}

func TestSeries_ConcurrentReadWrite(t *testing.T) {
	// 동시 읽기/쓰기 안전성 테스트
	ResetMemoryUsage()
	s := NewSeries("test")

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// 쓰기 고루틴
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
				_ = s.Write(DataPoint{
					Timestamp: time.Now().Add(time.Duration(i) * time.Millisecond),
					Fields:    map[string]any{"v": float64(i)},
				})
			}
		}
	}()

	// 읽기 고루틴
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
				_ = s.Latest(10)
				_ = s.Len()
				_ = s.MemorySize()
			}
		}
	}()

	// 100ms 후 종료
	time.Sleep(100 * time.Millisecond)
	close(stop)
	wg.Wait()

	assert.Greater(t, s.Len(), 0)
}

func TestSeries_String(t *testing.T) {
	ResetMemoryUsage()
	s := NewSeries("cpu,host=server01")
	str := s.String()
	assert.Contains(t, str, "cpu,host=server01")
	assert.Contains(t, str, "points=0")
}
