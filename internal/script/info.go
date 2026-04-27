package script

import (
	"sync"
	"sync/atomic"
	"time"
)

// ScriptStats 는 개별 스크립트 실행 통계이다.
type ScriptStats struct {
	ScriptID      string
	Executions    atomic.Int64
	Errors        atomic.Int64
	TotalDuration atomic.Int64 // 나노초 단위 (평균 계산용)
	LastExecutedAt atomic.Int64 // 유닉스 나노초
	Version       int
	CompiledAt    time.Time
}

// AvgExecutionTime 은 평균 실행 시간을 반환한다.
func (s *ScriptStats) AvgExecutionTime() time.Duration {
	execs := s.Executions.Load()
	if execs == 0 {
		return 0
	}
	total := s.TotalDuration.Load()
	return time.Duration(total / execs)
}

// ScriptStatsSnapshot 은 ScriptStats의 읽기 전용 스냅샷이다.
type ScriptStatsSnapshot struct {
	ScriptID         string
	Executions       int64
	Errors           int64
	AvgExecutionTime time.Duration
	LastExecutedAt   time.Time
	Version          int
	CompiledAt       time.Time
}

// Snapshot 은 현재 통계의 스냅샷을 생성한다.
func (s *ScriptStats) Snapshot() ScriptStatsSnapshot {
	lastExec := s.LastExecutedAt.Load()
	var lastExecTime time.Time
	if lastExec > 0 {
		lastExecTime = time.Unix(0, lastExec)
	}

	return ScriptStatsSnapshot{
		ScriptID:         s.ScriptID,
		Executions:       s.Executions.Load(),
		Errors:           s.Errors.Load(),
		AvgExecutionTime: s.AvgExecutionTime(),
		LastExecutedAt:   lastExecTime,
		Version:          s.Version,
		CompiledAt:       s.CompiledAt,
	}
}

// ScriptStatsTracker 는 스크립트별 통계를 추적하는 구조체이다.
type ScriptStatsTracker struct {
	scripts sync.Map // map[string]*ScriptStats
}

// NewScriptStatsTracker 는 새 ScriptStatsTracker를 생성한다.
func NewScriptStatsTracker() *ScriptStatsTracker {
	return &ScriptStatsTracker{}
}

// getOrCreateStats 는 스크립트 통계를 조회하거나 새로 생성한다.
func (t *ScriptStatsTracker) getOrCreateStats(scriptID string) *ScriptStats {
	val, loaded := t.scripts.LoadOrStore(scriptID, &ScriptStats{
		ScriptID: scriptID,
	})
	if !loaded {
		// 새로 생성된 경우 ScriptID가 이미 설정됨
		return val.(*ScriptStats)
	}
	return val.(*ScriptStats)
}

// RecordExecution 은 스크립트 실행을 기록한다.
func (t *ScriptStatsTracker) RecordExecution(scriptID string, duration time.Duration, err error) {
	stats := t.getOrCreateStats(scriptID)
	stats.Executions.Add(1)
	stats.TotalDuration.Add(int64(duration))
	stats.LastExecutedAt.Store(time.Now().UnixNano())

	if err != nil {
		stats.Errors.Add(1)
	}
}

// RecordCompile 은 스크립트 컴파일을 기록한다.
func (t *ScriptStatsTracker) RecordCompile(scriptID string, version int) {
	stats := t.getOrCreateStats(scriptID)
	stats.Version = version
	stats.CompiledAt = time.Now()
}

// GetScriptStats 는 특정 스크립트의 통계 스냅샷을 반환한다.
func (t *ScriptStatsTracker) GetScriptStats(scriptID string) (*ScriptStatsSnapshot, bool) {
	val, ok := t.scripts.Load(scriptID)
	if !ok {
		return nil, false
	}
	snap := val.(*ScriptStats).Snapshot()
	return &snap, true
}

// AllScriptStats 는 모든 스크립트의 통계 스냅샷을 반환한다.
func (t *ScriptStatsTracker) AllScriptStats() map[string]*ScriptStatsSnapshot {
	result := make(map[string]*ScriptStatsSnapshot)
	t.scripts.Range(func(key, value any) bool {
		stats := value.(*ScriptStats)
		snap := stats.Snapshot()
		result[key.(string)] = &snap
		return true
	})
	return result
}

// RemoveScript 는 스크립트 통계를 제거한다.
func (t *ScriptStatsTracker) RemoveScript(scriptID string) {
	t.scripts.Delete(scriptID)
}
