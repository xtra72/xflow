// schedule_log_memory_test.go 는 ScheduleLogMemoryRepository 를 검증한다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 — 스케줄 로그 저장소 백엔드 선택).
//
// sqlite 구현과 동일한 시나리오(fire/result, 필터 declared/actor, 페이지네이션, Count,
// Clear, 닫힘-에러)를 미러링한다. race-clean, t.Parallel().
package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleLogMemory_AppendAndListNewestFirst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewScheduleLogMemoryRepository()

	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: ScheduleLogRecordKindFire, Timestamp: 100}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: ScheduleLogRecordKindResult, Timestamp: 200}))

	got, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0)
	require.NoError(t, err)
	require.Len(t, got, 2)
	// 최신순(timestamp DESC).
	assert.Equal(t, int64(200), got[0].Timestamp)
	assert.Equal(t, int64(100), got[1].Timestamp)
	// ID 자동 증가.
	assert.NotZero(t, got[0].ID)
	assert.NotZero(t, got[1].ID)
}

func TestScheduleLogMemory_FilterScheduleAndRule(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewScheduleLogMemoryRepository()
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RuleName: "morning", RecordKind: "result", Timestamp: 100}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s2", RuleName: "evening", RecordKind: "result", Timestamp: 200}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RuleName: "morning", RecordKind: "result", Timestamp: 300}))

	byID, err := repo.List(ctx, ScheduleLogFilter{ScheduleID: "s1"}, 0, 0)
	require.NoError(t, err)
	require.Len(t, byID, 2)

	byRule, err := repo.List(ctx, ScheduleLogFilter{RuleName: "evening"}, 0, 0)
	require.NoError(t, err)
	require.Len(t, byRule, 1)
	assert.Equal(t, "s2", byRule[0].ScheduleID)
}

func TestScheduleLogMemory_FilterAgentDeclaredOrActor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewScheduleLogMemoryRepository()
	// declared 매칭(fire — actor 비어 있음).
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", DeclaredAgentID: "hvac-a", RecordKind: "fire", Timestamp: 100}))
	// actor 매칭(result).
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", ActorAgentID: "hvac-a", RecordKind: "result", Timestamp: 200}))
	// 무관.
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s2", DeclaredAgentID: "hvac-b", ActorAgentID: "hvac-b", RecordKind: "result", Timestamp: 150}))

	got, err := repo.List(ctx, ScheduleLogFilter{AgentID: "hvac-a"}, 0, 0)
	require.NoError(t, err)
	require.Len(t, got, 2, "declared 또는 actor 매칭(RD-6)")

	only, err := repo.List(ctx, ScheduleLogFilter{AgentID: "hvac-b"}, 0, 0)
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, "s2", only[0].ScheduleID)
}

func TestScheduleLogMemory_Pagination(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewScheduleLogMemoryRepository()
	for _, ts := range []int64{10, 20, 30, 40, 50} {
		require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: ts}))
	}
	// 최신순 50,40,30,20,10 → offset 1 → 40,30,20,10 → limit 2 → 40,30.
	got, err := repo.List(ctx, ScheduleLogFilter{}, 2, 1)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, int64(40), got[0].Timestamp)
	assert.Equal(t, int64(30), got[1].Timestamp)

	// offset 이 범위를 넘으면 빈 슬라이스.
	empty, err := repo.List(ctx, ScheduleLogFilter{}, 10, 100)
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestScheduleLogMemory_Count(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewScheduleLogMemoryRepository()
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 10}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 20}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s2", RecordKind: "result", Timestamp: 30}))

	all, err := repo.Count(ctx, ScheduleLogFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, all)

	byID, err := repo.Count(ctx, ScheduleLogFilter{ScheduleID: "s1"})
	require.NoError(t, err)
	assert.Equal(t, 2, byID, "Count 는 필터 매칭 전체(페이지네이션 무관)")
}

func TestScheduleLogMemory_Clear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewScheduleLogMemoryRepository()
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 10}))
	require.NoError(t, repo.Clear(ctx))

	got, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, got)
	n, err := repo.Count(ctx, ScheduleLogFilter{})
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestScheduleLogMemory_ClosedError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo := NewScheduleLogMemoryRepository()
	require.NoError(t, repo.Close())

	assert.ErrorIs(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1"}), ErrScheduleLogClosed)
	_, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0)
	assert.ErrorIs(t, err, ErrScheduleLogClosed)
	_, err = repo.Count(ctx, ScheduleLogFilter{})
	assert.ErrorIs(t, err, ErrScheduleLogClosed)
	assert.ErrorIs(t, repo.Clear(ctx), ErrScheduleLogClosed)
}
