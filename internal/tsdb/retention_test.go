package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestRetentionPolicy_ApplyTimeRetention(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		rp       RetentionPolicy
		points   []DataPoint
		expected int // 남는 포인트 수
	}{
		{
			name: "MaxAge 비활성 (0)",
			rp:   RetentionPolicy{MaxAge: 0},
			points: []DataPoint{
				{Timestamp: now.Add(-2 * time.Hour)},
				{Timestamp: now.Add(-1 * time.Hour)},
			},
			expected: 2,
		},
		{
			name:     "빈 포인트 슬라이스",
			rp:       RetentionPolicy{MaxAge: 1 * time.Hour},
			points:   []DataPoint{},
			expected: 0,
		},
		{
			name: "모든 포인트 보존",
			rp:   RetentionPolicy{MaxAge: 3 * time.Hour},
			points: []DataPoint{
				{Timestamp: now.Add(-2 * time.Hour)},
				{Timestamp: now.Add(-1 * time.Hour)},
				{Timestamp: now},
			},
			expected: 3,
		},
		{
			name: "일부 포인트 만료",
			rp:   RetentionPolicy{MaxAge: 1 * time.Hour},
			points: []DataPoint{
				{Timestamp: now.Add(-3 * time.Hour)},
				{Timestamp: now.Add(-2 * time.Hour)},
				{Timestamp: now.Add(-30 * time.Minute)},
				{Timestamp: now},
			},
			expected: 2,
		},
		{
			name: "모든 포인트 만료",
			rp:   RetentionPolicy{MaxAge: 10 * time.Minute},
			points: []DataPoint{
				{Timestamp: now.Add(-3 * time.Hour)},
				{Timestamp: now.Add(-2 * time.Hour)},
				{Timestamp: now.Add(-1 * time.Hour)},
			},
			expected: 0,
		},
		{
			name: "경계값 - 정확히 MaxAge인 포인트",
			rp:   RetentionPolicy{MaxAge: 1 * time.Hour},
			points: []DataPoint{
				{Timestamp: now.Add(-1 * time.Hour)}, // 정확히 cutoff 시점
				{Timestamp: now.Add(-30 * time.Minute)},
			},
			expected: 2, // cutoff과 같은 시점은 보존
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rp.ApplyTimeRetention(tt.points, now)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

func TestRetentionPolicy_ApplyCountRetention(t *testing.T) {
	tests := []struct {
		name     string
		rp       RetentionPolicy
		points   []DataPoint
		expected int
	}{
		{
			name:     "MaxPoints 비활성 (0)",
			rp:       RetentionPolicy{MaxPoints: 0},
			points:   makePoints(5),
			expected: 5,
		},
		{
			name:     "포인트 수가 MaxPoints 이하",
			rp:       RetentionPolicy{MaxPoints: 10},
			points:   makePoints(5),
			expected: 5,
		},
		{
			name:     "포인트 수가 MaxPoints 초과",
			rp:       RetentionPolicy{MaxPoints: 3},
			points:   makePoints(5),
			expected: 3,
		},
		{
			name:     "포인트 수가 MaxPoints와 동일",
			rp:       RetentionPolicy{MaxPoints: 5},
			points:   makePoints(5),
			expected: 5,
		},
		{
			name:     "빈 슬라이스",
			rp:       RetentionPolicy{MaxPoints: 3},
			points:   []DataPoint{},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.rp.ApplyCountRetention(tt.points)
			assert.Equal(t, tt.expected, len(result))
		})
	}
}

func TestRetentionPolicy_ApplyCountRetention_KeepsLatest(t *testing.T) {
	// 카운트 초과 시 가장 최근 데이터가 보존되는지 확인
	now := time.Now()
	points := []DataPoint{
		{Timestamp: now.Add(-4 * time.Second), Fields: map[string]any{"v": int64(1)}},
		{Timestamp: now.Add(-3 * time.Second), Fields: map[string]any{"v": int64(2)}},
		{Timestamp: now.Add(-2 * time.Second), Fields: map[string]any{"v": int64(3)}},
		{Timestamp: now.Add(-1 * time.Second), Fields: map[string]any{"v": int64(4)}},
		{Timestamp: now, Fields: map[string]any{"v": int64(5)}},
	}

	rp := RetentionPolicy{MaxPoints: 2}
	result := rp.ApplyCountRetention(points)

	assert.Equal(t, 2, len(result))
	assert.Equal(t, int64(4), result[0].Fields["v"])
	assert.Equal(t, int64(5), result[1].Fields["v"])
}

func TestRetentionPolicy_IsExpired(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		rp       RetentionPolicy
		dp       DataPoint
		expected bool
	}{
		{
			name:     "MaxAge 비활성",
			rp:       RetentionPolicy{MaxAge: 0},
			dp:       DataPoint{Timestamp: now.Add(-100 * time.Hour)},
			expected: false,
		},
		{
			name:     "만료된 포인트",
			rp:       RetentionPolicy{MaxAge: 1 * time.Hour},
			dp:       DataPoint{Timestamp: now.Add(-2 * time.Hour)},
			expected: true,
		},
		{
			name:     "만료되지 않은 포인트",
			rp:       RetentionPolicy{MaxAge: 1 * time.Hour},
			dp:       DataPoint{Timestamp: now.Add(-30 * time.Minute)},
			expected: false,
		},
		{
			name:     "정확히 경계값",
			rp:       RetentionPolicy{MaxAge: 1 * time.Hour},
			dp:       DataPoint{Timestamp: now.Add(-1 * time.Hour)},
			expected: false, // 정확히 MaxAge 이전은 만료되지 않음
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.rp.IsExpired(tt.dp, now))
		})
	}
}

func TestRetentionPolicy_IsZero(t *testing.T) {
	tests := []struct {
		name     string
		rp       RetentionPolicy
		expected bool
	}{
		{
			name:     "모든 값 0 (비활성)",
			rp:       RetentionPolicy{},
			expected: true,
		},
		{
			name:     "MaxAge만 설정",
			rp:       RetentionPolicy{MaxAge: 1 * time.Hour},
			expected: false,
		},
		{
			name:     "MaxPoints만 설정",
			rp:       RetentionPolicy{MaxPoints: 100},
			expected: false,
		},
		{
			name:     "MaxBytes만 설정",
			rp:       RetentionPolicy{MaxBytes: 1024},
			expected: false,
		},
		{
			name:     "모든 값 설정",
			rp:       RetentionPolicy{MaxAge: 1 * time.Hour, MaxPoints: 100, MaxBytes: 1024},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.rp.IsZero())
		})
	}
}

// makePoints 는 테스트용 DataPoint 슬라이스를 생성한다.
func makePoints(n int) []DataPoint {
	now := time.Now()
	points := make([]DataPoint, n)
	for i := range n {
		points[i] = DataPoint{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"v": float64(i)},
		}
	}
	return points
}
