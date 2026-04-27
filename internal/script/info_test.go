package script

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// ScriptStats 테스트
// ============================================================

func TestScriptStats_AvgExecutionTime_NoExecutions(t *testing.T) {
	stats := &ScriptStats{
		ScriptID: "test",
	}
	assert.Equal(t, time.Duration(0), stats.AvgExecutionTime())
}

func TestScriptStats_AvgExecutionTime_WithExecutions(t *testing.T) {
	stats := &ScriptStats{
		ScriptID: "test",
	}
	// 3회 실행, 총 300ms
	stats.Executions.Store(3)
	stats.TotalDuration.Store(int64(300 * time.Millisecond))

	avg := stats.AvgExecutionTime()
	assert.Equal(t, 100*time.Millisecond, avg)
}

func TestScriptStats_Snapshot(t *testing.T) {
	stats := &ScriptStats{
		ScriptID:   "snap-test",
		Version:    2,
		CompiledAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	stats.Executions.Store(10)
	stats.Errors.Store(2)
	stats.TotalDuration.Store(int64(500 * time.Millisecond))
	now := time.Now()
	stats.LastExecutedAt.Store(now.UnixNano())

	snap := stats.Snapshot()
	assert.Equal(t, "snap-test", snap.ScriptID)
	assert.Equal(t, int64(10), snap.Executions)
	assert.Equal(t, int64(2), snap.Errors)
	assert.Equal(t, 50*time.Millisecond, snap.AvgExecutionTime)
	assert.Equal(t, 2, snap.Version)
	assert.Equal(t, time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), snap.CompiledAt)
	// LastExecutedAt 은 UnixNano로 저장하므로 정밀도 차이가 있을 수 있음
	assert.WithinDuration(t, now, snap.LastExecutedAt, time.Millisecond)
}

// ============================================================
// ScriptStatsTracker 테스트
// ============================================================

func TestNewScriptStatsTracker(t *testing.T) {
	tracker := NewScriptStatsTracker()
	require.NotNil(t, tracker)
}

func TestScriptStatsTracker_RecordExecution_Success(t *testing.T) {
	tracker := NewScriptStatsTracker()

	tracker.RecordExecution("script-1", 100*time.Millisecond, nil)

	snap, ok := tracker.GetScriptStats("script-1")
	require.True(t, ok)
	assert.Equal(t, int64(1), snap.Executions)
	assert.Equal(t, int64(0), snap.Errors)
	assert.True(t, !snap.LastExecutedAt.IsZero())
}

func TestScriptStatsTracker_RecordExecution_Error(t *testing.T) {
	tracker := NewScriptStatsTracker()

	tracker.RecordExecution("script-1", 50*time.Millisecond, ErrScriptExecutionFailed)

	snap, ok := tracker.GetScriptStats("script-1")
	require.True(t, ok)
	assert.Equal(t, int64(1), snap.Executions)
	assert.Equal(t, int64(1), snap.Errors)
}

func TestScriptStatsTracker_RecordExecution_MultipleExecutions(t *testing.T) {
	tracker := NewScriptStatsTracker()

	tracker.RecordExecution("script-1", 100*time.Millisecond, nil)
	tracker.RecordExecution("script-1", 200*time.Millisecond, nil)
	tracker.RecordExecution("script-1", 300*time.Millisecond, nil)

	snap, ok := tracker.GetScriptStats("script-1")
	require.True(t, ok)
	assert.Equal(t, int64(3), snap.Executions)
	assert.Equal(t, 200*time.Millisecond, snap.AvgExecutionTime)
}

func TestScriptStatsTracker_RecordCompile(t *testing.T) {
	tracker := NewScriptStatsTracker()

	tracker.RecordCompile("script-1", 1)

	snap, ok := tracker.GetScriptStats("script-1")
	require.True(t, ok)
	assert.Equal(t, 1, snap.Version)
	assert.True(t, !snap.CompiledAt.IsZero())
}

func TestScriptStatsTracker_RecordCompile_VersionUpdate(t *testing.T) {
	tracker := NewScriptStatsTracker()

	tracker.RecordCompile("script-1", 1)
	tracker.RecordCompile("script-1", 2)

	snap, ok := tracker.GetScriptStats("script-1")
	require.True(t, ok)
	assert.Equal(t, 2, snap.Version)
}

func TestScriptStatsTracker_GetScriptStats_NotFound(t *testing.T) {
	tracker := NewScriptStatsTracker()

	_, ok := tracker.GetScriptStats("nonexistent")
	assert.False(t, ok)
}

func TestScriptStatsTracker_AllScriptStats(t *testing.T) {
	tracker := NewScriptStatsTracker()

	tracker.RecordExecution("script-1", 100*time.Millisecond, nil)
	tracker.RecordExecution("script-2", 200*time.Millisecond, nil)
	tracker.RecordExecution("script-3", 300*time.Millisecond, nil)

	all := tracker.AllScriptStats()
	assert.Len(t, all, 3)
	assert.Contains(t, all, "script-1")
	assert.Contains(t, all, "script-2")
	assert.Contains(t, all, "script-3")
}

func TestScriptStatsTracker_AllScriptStats_Empty(t *testing.T) {
	tracker := NewScriptStatsTracker()

	all := tracker.AllScriptStats()
	assert.Empty(t, all)
}

func TestScriptStatsTracker_RemoveScript(t *testing.T) {
	tracker := NewScriptStatsTracker()

	tracker.RecordExecution("script-1", 100*time.Millisecond, nil)

	_, ok := tracker.GetScriptStats("script-1")
	require.True(t, ok)

	tracker.RemoveScript("script-1")

	_, ok = tracker.GetScriptStats("script-1")
	assert.False(t, ok)
}

func TestScriptStatsTracker_RemoveScript_Nonexistent(t *testing.T) {
	tracker := NewScriptStatsTracker()

	// 존재하지 않는 스크립트 제거는 에러 없이 처리
	tracker.RemoveScript("nonexistent")
}

// ============================================================
// 동시성 테스트
// ============================================================

func TestScriptStatsTracker_ConcurrentRecordExecution(t *testing.T) {
	tracker := NewScriptStatsTracker()

	const goroutines = 50
	var wg sync.WaitGroup

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tracker.RecordExecution("concurrent-script", 10*time.Millisecond, nil)
		}()
	}

	wg.Wait()

	snap, ok := tracker.GetScriptStats("concurrent-script")
	require.True(t, ok)
	assert.Equal(t, int64(goroutines), snap.Executions)
}

func TestScriptStatsTracker_ConcurrentMultipleScripts(t *testing.T) {
	tracker := NewScriptStatsTracker()

	const goroutines = 20
	var wg sync.WaitGroup

	scripts := []string{"script-a", "script-b", "script-c"}

	for _, scriptID := range scripts {
		for i := 0; i < goroutines; i++ {
			wg.Add(1)
			go func(id string) {
				defer wg.Done()
				tracker.RecordExecution(id, 10*time.Millisecond, nil)
			}(scriptID)
		}
	}

	wg.Wait()

	all := tracker.AllScriptStats()
	assert.Len(t, all, 3)
	for _, scriptID := range scripts {
		snap, ok := all[scriptID]
		require.True(t, ok, "script %s should exist", scriptID)
		assert.Equal(t, int64(goroutines), snap.Executions)
	}
}
