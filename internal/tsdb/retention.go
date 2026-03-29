package tsdb

import (
	"sort"
	"time"
)

// RetentionPolicy 는 데이터 보존 정책을 정의한다.
type RetentionPolicy struct {
	MaxAge    time.Duration // 0이면 비활성
	MaxPoints int           // 0이면 비활성
	MaxBytes  int64         // 0이면 비활성
}

// ApplyTimeRetention 는 MaxAge 기반으로 만료된 데이터를 제거한다.
// binary search로 cutoff 위치를 탐색하여 효율적으로 처리한다.
// points는 시간순으로 정렬되어 있다고 가정한다.
func (rp RetentionPolicy) ApplyTimeRetention(points []DataPoint, now time.Time) []DataPoint {
	if rp.MaxAge <= 0 || len(points) == 0 {
		return points
	}

	cutoff := now.Add(-rp.MaxAge)

	// binary search로 cutoff 이후 첫 번째 포인트 위치 탐색
	idx := sort.Search(len(points), func(i int) bool {
		return !points[i].Timestamp.Before(cutoff)
	})

	if idx >= len(points) {
		return points[:0]
	}

	return points[idx:]
}

// ApplyCountRetention 는 MaxPoints를 초과하는 가장 오래된 데이터를 앞에서 제거한다.
func (rp RetentionPolicy) ApplyCountRetention(points []DataPoint) []DataPoint {
	if rp.MaxPoints <= 0 || len(points) <= rp.MaxPoints {
		return points
	}

	return points[len(points)-rp.MaxPoints:]
}

// IsExpired 는 개별 포인트의 만료 여부를 반환한다.
func (rp RetentionPolicy) IsExpired(dp DataPoint, now time.Time) bool {
	if rp.MaxAge <= 0 {
		return false
	}
	return dp.Timestamp.Before(now.Add(-rp.MaxAge))
}

// IsZero 는 모든 정책이 0인지(비활성 여부)를 반환한다.
func (rp RetentionPolicy) IsZero() bool {
	return rp.MaxAge == 0 && rp.MaxPoints == 0 && rp.MaxBytes == 0
}
