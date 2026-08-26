package tsdb

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Series 는 단일 시계열 데이터를 관리하는 구조체이다.
type Series struct {
	key        string
	mu         sync.RWMutex
	points     []DataPoint
	retention  RetentionPolicy // 시리즈별 정책 (IsZero()면 글로벌 사용)
	memorySize int64           // 현재 추정 메모리 크기
	createdAt  time.Time
}

// BuildSeriesKey 는 measurement와 태그로 시리즈 키를 생성한다.
// 태그를 키 기준 정렬하여 "measurement,key1=val1,key2=val2" 형식으로 반환한다.
func BuildSeriesKey(measurement string, tags map[string]string) (string, error) {
	if measurement == "" {
		return "", ErrInvalidMeasurement
	}

	if len(tags) == 0 {
		return measurement, nil
	}

	// 태그 키를 정렬
	keys := make([]string, 0, len(tags))
	for k := range tags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// 키 생성
	var b strings.Builder
	b.WriteString(measurement)
	for _, k := range keys {
		b.WriteByte(',')
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(tags[k])
	}

	return b.String(), nil
}

// NewSeries 는 새로운 시리즈를 생성한다.
func NewSeries(key string) *Series {
	return &Series{
		key:       key,
		points:    make([]DataPoint, 0, 64),
		createdAt: time.Now(),
	}
}

// Write 는 데이터 포인트를 시리즈에 추가한다.
// 대부분의 경우 시간순 append이지만, out-of-order 데이터는 sort.Search로 삽입 위치를 탐색한다.
func (s *Series) Write(dp DataPoint) error {
	if err := dp.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	pointSize := dp.EstimateSize()

	// 빈 슬라이스이거나 마지막 포인트보다 같거나 이후인 경우 append
	if len(s.points) == 0 || !dp.Timestamp.Before(s.points[len(s.points)-1].Timestamp) {
		s.points = append(s.points, dp)
	} else {
		// out-of-order: 삽입 위치 탐색
		idx := sort.Search(len(s.points), func(i int) bool {
			return s.points[i].Timestamp.After(dp.Timestamp)
		})
		// 슬라이스에 삽입
		s.points = append(s.points, DataPoint{})
		copy(s.points[idx+1:], s.points[idx:])
		s.points[idx] = dp
	}

	s.memorySize += pointSize
	AddMemory(pointSize)

	return nil
}

// QueryRange 는 시간 범위 내의 데이터 포인트를 반환한다.
// binary search 기반 범위 조회 (읽기 잠금)
func (s *Series) QueryRange(start, end time.Time) []DataPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.points) == 0 {
		return nil
	}

	// start 이상인 첫 번째 인덱스
	startIdx := sort.Search(len(s.points), func(i int) bool {
		return !s.points[i].Timestamp.Before(start)
	})

	// end 초과인 첫 번째 인덱스
	endIdx := sort.Search(len(s.points), func(i int) bool {
		return s.points[i].Timestamp.After(end)
	})

	if startIdx >= endIdx {
		return nil
	}

	// 결과 복사본 반환 (외부에서 원본 수정 방지)
	result := make([]DataPoint, endIdx-startIdx)
	for i, dp := range s.points[startIdx:endIdx] {
		result[i] = copyDataPoint(dp)
	}
	return result
}

// QueryRangeScalar 는 [start,end] 구간에서 field 값만 뽑아 돌려준다.
//
// QueryRange 와 달리 DataPoint 를 복사하지 않는다. 집계 경로는 (시각, 값) 두
// 가지만 쓰는데, 포인트마다 Fields 맵을 통째로 복제하면 구간 크기에 비례해
// 맵 할당이 발생한다. 읽기 락을 쥔 채로 그 복사를 하므로, 구간이 넓어지면
// 대기 중인 쓰기 락 뒤로 수집이 밀린다.
//
// 반환값:
//   - pts:     값 추출에 성공한 포인트만. 필드가 없거나 숫자가 아니면 빠진다.
//   - scanned: 구간에 들어온 **원본** 포인트 수(추출 성공 여부와 무관).
//   - firstTS: 구간 첫 원본 포인트의 시각. 전체 범위 집계의 결과 시각이
//     원본 첫 포인트 시각이라는 기존 규약을 지키기 위해 함께 돌려준다.
//     scanned 가 0 이면 제로값이다.
func (s *Series) QueryRangeScalar(start, end time.Time, field string) (pts []scalarPoint, scanned int, firstTS time.Time) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.points) == 0 {
		return nil, 0, time.Time{}
	}

	// start 이상인 첫 번째 인덱스
	startIdx := sort.Search(len(s.points), func(i int) bool {
		return !s.points[i].Timestamp.Before(start)
	})

	// end 초과인 첫 번째 인덱스
	endIdx := sort.Search(len(s.points), func(i int) bool {
		return s.points[i].Timestamp.After(end)
	})

	if startIdx >= endIdx {
		return nil, 0, time.Time{}
	}

	window := s.points[startIdx:endIdx]
	out := make([]scalarPoint, 0, len(window))
	for _, dp := range window {
		out = appendScalar(out, dp, field)
	}
	return out, len(window), window[0].Timestamp
}

// Latest 는 마지막 n개의 데이터 포인트를 반환한다.
func (s *Series) Latest(n int) []DataPoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if len(s.points) == 0 || n <= 0 {
		return nil
	}

	if n > len(s.points) {
		n = len(s.points)
	}

	result := make([]DataPoint, n)
	for i, dp := range s.points[len(s.points)-n:] {
		result[i] = copyDataPoint(dp)
	}
	return result
}

// Len 은 시리즈의 포인트 수를 반환한다.
func (s *Series) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.points)
}

// MemorySize 는 현재 추정 메모리 크기를 반환한다.
func (s *Series) MemorySize() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.memorySize
}

// Evict 는 보존 정책을 적용하고 퇴거된 포인트 수를 반환한다.
// 시리즈별 정책이 설정되어 있으면 우선 적용하고, 아니면 글로벌 정책을 사용한다.
func (s *Series) Evict(global RetentionPolicy, now time.Time) int {
	s.mu.Lock()
	defer s.mu.Unlock()

	policy := global
	if !s.retention.IsZero() {
		policy = s.retention
	}

	before := len(s.points)

	// 시간 기반 퇴거
	s.points = policy.ApplyTimeRetention(s.points, now)

	// 카운트 기반 퇴거
	s.points = policy.ApplyCountRetention(s.points)

	evicted := before - len(s.points)

	if evicted > 0 {
		// 메모리 크기 재계산
		var newSize int64
		for i := range s.points {
			newSize += s.points[i].EstimateSize()
		}
		freed := s.memorySize - newSize
		s.memorySize = newSize
		if freed > 0 {
			SubMemory(freed)
		}
	}

	return evicted
}

// SetRetention 는 시리즈별 보존 정책을 설정한다.
func (s *Series) SetRetention(policy RetentionPolicy) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retention = policy
}

// copyDataPoint 는 DataPoint의 깊은 복사본을 반환한다.
func copyDataPoint(dp DataPoint) DataPoint {
	fields := make(map[string]any, len(dp.Fields))
	for k, v := range dp.Fields {
		fields[k] = v
	}
	return DataPoint{
		Timestamp: dp.Timestamp,
		Fields:    fields,
	}
}

// Key 는 시리즈의 키를 반환한다.
func (s *Series) Key() string {
	return s.key
}

// String 은 시리즈의 문자열 표현을 반환한다.
func (s *Series) String() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("Series{key=%s, points=%d, memory=%d}", s.key, len(s.points), s.memorySize)
}
