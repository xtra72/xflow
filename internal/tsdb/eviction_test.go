package tsdb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvictor_StartStop(t *testing.T) {
	// Evictor가 정상적으로 시작/종료되는지 확인
	ResetMemoryUsage()

	cfg := DefaultConfig()
	cfg.EvictionInterval = 10 * time.Millisecond
	db := newDefaultTSDB(cfg)

	evictor := NewEvictor(db, cfg.EvictionInterval)

	// 시작
	evictor.Start()

	// 약간 대기
	time.Sleep(50 * time.Millisecond)

	// 종료 (데드락 없이 완료되어야 함)
	evictor.Stop()
}

func TestEvictor_PeriodicEviction(t *testing.T) {
	// 주기적으로 만료 데이터가 정리되는지 확인
	ResetMemoryUsage()

	cfg := DefaultConfig()
	cfg.MaxAge = 50 * time.Millisecond
	cfg.EvictionInterval = 20 * time.Millisecond
	db := newDefaultTSDB(cfg)

	// 데이터 삽입
	now := time.Now()
	key := "test_series"
	s := NewSeries(key)
	for i := range 10 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: now.Add(-time.Duration(i) * 100 * time.Millisecond),
			Fields:    map[string]any{"v": float64(i)},
		}))
	}

	db.mu.Lock()
	db.series[key] = s
	db.mu.Unlock()

	// Evictor 시작
	evictor := NewEvictor(db, cfg.EvictionInterval)
	evictor.Start()

	// MaxAge + EvictionInterval 만큼 대기하여 퇴거가 실행되도록 함
	time.Sleep(150 * time.Millisecond)

	evictor.Stop()

	// 만료된 데이터가 정리되었는지 확인
	assert.Less(t, s.Len(), 10)
}

func TestEvictor_MemoryBasedEviction(t *testing.T) {
	// 메모리 기반 퇴거가 동작하는지 확인
	ResetMemoryUsage()

	cfg := DefaultConfig()
	cfg.MaxMemoryBytes = 200 // 매우 작은 메모리 제한
	cfg.MaxAge = 0           // 시간 기반 퇴거 비활성
	cfg.EvictionInterval = 10 * time.Millisecond
	db := newDefaultTSDB(cfg)

	// 데이터 삽입
	key := "test_series"
	s := NewSeries(key)
	now := time.Now()
	for i := range 50 {
		require.NoError(t, s.Write(DataPoint{
			Timestamp: now.Add(time.Duration(i) * time.Second),
			Fields:    map[string]any{"value": float64(i)},
		}))
	}

	db.mu.Lock()
	db.series[key] = s
	db.mu.Unlock()

	beforeLen := s.Len()

	// Evictor 시작
	evictor := NewEvictor(db, cfg.EvictionInterval)
	evictor.Start()

	// 충분히 대기
	time.Sleep(100 * time.Millisecond)

	evictor.Stop()

	// 메모리 제한으로 인해 일부 데이터가 제거되었는지 확인
	assert.Less(t, s.Len(), beforeLen)
}

// newDefaultTSDB 는 테스트용으로 defaultTSDB를 생성한다. (evictor 없이)
func newDefaultTSDB(cfg Config) *defaultTSDB {
	return &defaultTSDB{
		series: make(map[string]*Series),
		config: cfg,
	}
}
