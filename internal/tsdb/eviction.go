package tsdb

import (
	"time"
)

// Evictor 는 주기적으로 만료된 데이터를 정리하는 구조체이다.
type Evictor struct {
	db       *defaultTSDB
	interval time.Duration
	stopCh   chan struct{}
	doneCh   chan struct{}
}

// NewEvictor 는 새로운 Evictor를 생성한다.
func NewEvictor(db *defaultTSDB, interval time.Duration) *Evictor {
	return &Evictor{
		db:       db,
		interval: interval,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start 는 고루틴에서 interval 주기로 모든 시리즈를 순회하며 만료 데이터를 정리한다.
func (e *Evictor) Start() {
	go func() {
		defer close(e.doneCh)
		ticker := time.NewTicker(e.interval)
		defer ticker.Stop()

		for {
			select {
			case <-e.stopCh:
				return
			case <-ticker.C:
				e.evict()
			}
		}
	}()
}

// Stop 는 Evictor 고루틴을 종료하고 완료를 대기한다.
func (e *Evictor) Stop() {
	close(e.stopCh)
	<-e.doneCh
}

// evict 는 실제 퇴거 작업을 수행한다.
func (e *Evictor) evict() {
	now := time.Now()

	// 글로벌 보존 정책
	globalPolicy := RetentionPolicy{
		MaxAge:    e.db.config.MaxAge,
		MaxPoints: e.db.config.MaxPointsPerSeries,
	}

	// 모든 시리즈에 보존 정책 적용
	e.db.mu.RLock()
	keys := make([]string, 0, len(e.db.series))
	for k := range e.db.series {
		keys = append(keys, k)
	}
	e.db.mu.RUnlock()

	for _, key := range keys {
		e.db.mu.RLock()
		s, ok := e.db.series[key]
		e.db.mu.RUnlock()
		if !ok {
			continue
		}
		s.Evict(globalPolicy, now)
	}

	// 용량 기반 퇴거: MaxMemoryBytes 초과 시 가장 오래된 시리즈의 가장 오래된 데이터부터 제거
	if e.db.config.MaxMemoryBytes > 0 {
		e.evictByMemory()
	}
}

// evictByMemory 는 메모리 사용량이 MaxMemoryBytes를 초과할 때
// 가장 오래된 데이터부터 제거한다.
func (e *Evictor) evictByMemory() {
	for CurrentMemoryUsage() > e.db.config.MaxMemoryBytes {
		// 가장 오래된 데이터를 가진 시리즈 탐색
		e.db.mu.RLock()
		var oldestKey string
		var oldestTime time.Time
		first := true

		for key, s := range e.db.series {
			s.mu.RLock()
			if len(s.points) > 0 {
				if first || s.points[0].Timestamp.Before(oldestTime) {
					oldestKey = key
					oldestTime = s.points[0].Timestamp
					first = false
				}
			}
			s.mu.RUnlock()
		}
		e.db.mu.RUnlock()

		if first {
			// 모든 시리즈가 비어있음
			break
		}

		// 가장 오래된 시리즈에서 포인트 제거 (10% 또는 최소 1개)
		e.db.mu.RLock()
		s, ok := e.db.series[oldestKey]
		e.db.mu.RUnlock()
		if !ok {
			break
		}

		s.mu.Lock()
		if len(s.points) == 0 {
			s.mu.Unlock()
			break
		}

		removeCount := len(s.points) / 10
		if removeCount < 1 {
			removeCount = 1
		}

		var freed int64
		for i := range removeCount {
			freed += s.points[i].EstimateSize()
		}
		s.points = s.points[removeCount:]
		s.memorySize -= freed
		s.mu.Unlock()

		SubMemory(freed)
	}
}
