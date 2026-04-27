package tsdb

import (
	"sync"
	"sync/atomic"
	"time"
)

// TSDB 는 인메모리 시계열 데이터베이스의 공개 인터페이스이다.
type TSDB interface {
	// Write 는 단일 데이터 포인트를 기록한다.
	Write(measurement string, tags map[string]string, fields map[string]any) error

	// WriteBatch 는 여러 데이터 포인트를 일괄 기록한다.
	WriteBatch(points []WriteRequest) error

	// QueryRange 는 시간 범위 내의 데이터 포인트를 조회한다.
	QueryRange(seriesKey string, start, end time.Time) ([]DataPoint, error)

	// Latest 는 시리즈의 최신 n개 포인트를 반환한다.
	Latest(seriesKey string, n int) ([]DataPoint, error)

	// Execute 는 쿼리를 실행한다.
	Execute(q Query) ([]QueryResult, error)

	// FilterSeries 는 measurement와 태그 필터로 시리즈 키를 검색한다.
	FilterSeries(measurement string, tags map[string]string) []string

	// SeriesKeys 는 모든 시리즈 키 목록을 반환한다.
	SeriesKeys() []string

	// DeleteSeries 는 시리즈를 삭제한다.
	DeleteSeries(key string) error

	// Stats 는 TSDB 통계를 반환한다.
	Stats() Stats

	// Subscribe 는 지정된 시리즈 키에 대한 구독을 시작한다.
	// 새 DataPoint가 기록될 때마다 채널로 전달된다.
	// 반환된 함수를 호출하면 구독이 해제된다.
	Subscribe(keys []string, ch chan DataPoint) (unsubscribe func())

	// Close 는 TSDB를 종료한다.
	Close() error
}

// WriteRequest 는 쓰기 요청을 정의한다.
type WriteRequest struct {
	Measurement string
	Tags        map[string]string
	Fields      map[string]any
	Timestamp   time.Time // zero면 time.Now()
}

// Stats 는 TSDB 통계를 정의한다.
type Stats struct {
	SeriesCount int
	TotalPoints int64
	MemoryBytes int64
	OldestPoint time.Time
	NewestPoint time.Time
}

// defaultTSDB 는 TSDB 인터페이스의 기본 구현체이다.
type defaultTSDB struct {
	series     map[string]*Series
	mu         sync.RWMutex
	config     Config
	evictor    *Evictor
	subscriber *Subscriber
	closed     atomic.Bool
}

// New 는 새로운 TSDB 인스턴스를 생성한다.
// Evictor를 시작하고 설정된 주기로 만료 데이터를 정리한다.
func New(config Config) TSDB {
	db := &defaultTSDB{
		series:     make(map[string]*Series),
		config:     config,
		subscriber: NewSubscriber(),
	}

	db.evictor = NewEvictor(db, config.EvictionInterval)
	db.evictor.Start()

	return db
}

// Write 는 단일 데이터 포인트를 기록한다.
func (db *defaultTSDB) Write(measurement string, tags map[string]string, fields map[string]any) error {
	if db.closed.Load() {
		return ErrTSDBClosed
	}

	if measurement == "" {
		return ErrInvalidMeasurement
	}

	if fields == nil || len(fields) == 0 {
		return ErrInvalidField
	}

	key, err := BuildSeriesKey(measurement, tags)
	if err != nil {
		return err
	}

	s, err := db.getOrCreateSeries(key)
	if err != nil {
		return err
	}

	dp := DataPoint{
		Timestamp: time.Now(),
		Fields:    fields,
	}

	if err := s.Write(dp); err != nil {
		return err
	}

	db.subscriber.Notify(key, dp)
	return nil
}

// WriteBatch 는 여러 데이터 포인트를 일괄 기록한다.
func (db *defaultTSDB) WriteBatch(points []WriteRequest) error {
	if db.closed.Load() {
		return ErrTSDBClosed
	}

	for i := range points {
		p := &points[i]

		if p.Measurement == "" {
			return ErrInvalidMeasurement
		}

		if p.Fields == nil || len(p.Fields) == 0 {
			return ErrInvalidField
		}

		key, err := BuildSeriesKey(p.Measurement, p.Tags)
		if err != nil {
			return err
		}

		s, err := db.getOrCreateSeries(key)
		if err != nil {
			return err
		}

		ts := p.Timestamp
		if ts.IsZero() {
			ts = time.Now()
		}

		dp := DataPoint{
			Timestamp: ts,
			Fields:    p.Fields,
		}

		if err := s.Write(dp); err != nil {
			return err
		}

		db.subscriber.Notify(key, dp)
	}

	return nil
}

// QueryRange 는 시간 범위 내의 데이터 포인트를 조회한다.
func (db *defaultTSDB) QueryRange(seriesKey string, start, end time.Time) ([]DataPoint, error) {
	if db.closed.Load() {
		return nil, ErrTSDBClosed
	}

	s, err := db.getSeries(seriesKey)
	if err != nil {
		return nil, err
	}

	result := s.QueryRange(start, end)

	if db.config.MaxQueryPoints > 0 && len(result) > db.config.MaxQueryPoints {
		return nil, ErrMaxPointsExceeded
	}

	return result, nil
}

// Latest 는 시리즈의 최신 n개 포인트를 반환한다.
func (db *defaultTSDB) Latest(seriesKey string, n int) ([]DataPoint, error) {
	if db.closed.Load() {
		return nil, ErrTSDBClosed
	}

	s, err := db.getSeries(seriesKey)
	if err != nil {
		return nil, err
	}

	return s.Latest(n), nil
}

// SeriesKeys 는 모든 시리즈 키 목록을 반환한다.
func (db *defaultTSDB) SeriesKeys() []string {
	db.mu.RLock()
	defer db.mu.RUnlock()

	keys := make([]string, 0, len(db.series))
	for k := range db.series {
		keys = append(keys, k)
	}
	return keys
}

// DeleteSeries 는 시리즈를 삭제한다.
func (db *defaultTSDB) DeleteSeries(key string) error {
	if db.closed.Load() {
		return ErrTSDBClosed
	}

	db.mu.Lock()
	defer db.mu.Unlock()

	s, ok := db.series[key]
	if !ok {
		return ErrSeriesNotFound
	}

	// 메모리 해제
	s.mu.Lock()
	SubMemory(s.memorySize)
	s.memorySize = 0
	s.points = nil
	s.mu.Unlock()

	delete(db.series, key)
	return nil
}

// Stats 는 TSDB 통계를 반환한다.
func (db *defaultTSDB) Stats() Stats {
	db.mu.RLock()
	defer db.mu.RUnlock()

	stats := Stats{
		SeriesCount: len(db.series),
		MemoryBytes: CurrentMemoryUsage(),
	}

	for _, s := range db.series {
		s.mu.RLock()
		stats.TotalPoints += int64(len(s.points))
		if len(s.points) > 0 {
			oldest := s.points[0].Timestamp
			newest := s.points[len(s.points)-1].Timestamp

			if stats.OldestPoint.IsZero() || oldest.Before(stats.OldestPoint) {
				stats.OldestPoint = oldest
			}
			if stats.NewestPoint.IsZero() || newest.After(stats.NewestPoint) {
				stats.NewestPoint = newest
			}
		}
		s.mu.RUnlock()
	}

	return stats
}

// Subscribe 는 지정된 시리즈 키에 대한 구독을 시작한다.
func (db *defaultTSDB) Subscribe(keys []string, ch chan DataPoint) func() {
	return db.subscriber.Subscribe(keys, ch)
}

// Close 는 TSDB를 종료한다.
func (db *defaultTSDB) Close() error {
	if db.closed.Swap(true) {
		return ErrTSDBClosed
	}

	// 구독자 정리
	db.subscriber.Close()

	// Evictor 종료
	db.evictor.Stop()

	// 모든 시리즈 정리
	db.mu.Lock()
	defer db.mu.Unlock()

	for key, s := range db.series {
		s.mu.Lock()
		SubMemory(s.memorySize)
		s.memorySize = 0
		s.points = nil
		s.mu.Unlock()
		delete(db.series, key)
	}

	return nil
}

// getOrCreateSeries 는 키에 해당하는 시리즈를 가져오거나 새로 생성한다.
func (db *defaultTSDB) getOrCreateSeries(key string) (*Series, error) {
	// 먼저 읽기 잠금으로 시도
	db.mu.RLock()
	s, ok := db.series[key]
	db.mu.RUnlock()
	if ok {
		return s, nil
	}

	// 쓰기 잠금으로 생성
	db.mu.Lock()
	defer db.mu.Unlock()

	// 더블 체크
	if s, ok = db.series[key]; ok {
		return s, nil
	}

	// 최대 시리즈 수 확인
	if db.config.MaxSeries > 0 && len(db.series) >= db.config.MaxSeries {
		return nil, ErrMaxSeriesExceeded
	}

	s = NewSeries(key)
	db.series[key] = s
	return s, nil
}

// getSeries 는 키에 해당하는 시리즈를 반환한다.
func (db *defaultTSDB) getSeries(key string) (*Series, error) {
	db.mu.RLock()
	defer db.mu.RUnlock()

	s, ok := db.series[key]
	if !ok {
		return nil, ErrSeriesNotFound
	}
	return s, nil
}
