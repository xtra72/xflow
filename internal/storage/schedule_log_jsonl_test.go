// schedule_log_jsonl_test.go 는 ScheduleLogJSONLRepository 를 검증한다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 — 스케줄 로그 저장소 백엔드 선택).
//
// sqlite/memory 와 동일한 시나리오 + 손상된 줄 건너뛰기(malformed-line-skipped) +
// 파일 영속(재오픈 시 nextID 복원)을 검증한다. race-clean.
package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTempJSONL(t *testing.T) (*ScheduleLogJSONLRepository, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sub", "schedule_log.jsonl")
	repo, err := NewScheduleLogJSONLRepository(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo, path
}

func TestScheduleLogJSONL_AppendAndListNewestFirst(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, path := newTempJSONL(t)

	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: ScheduleLogRecordKindFire, Timestamp: 100}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: ScheduleLogRecordKindResult, Timestamp: 200}))

	// 부모 디렉터리가 멱등 생성되었는지(dir-create 패턴).
	_, statErr := os.Stat(filepath.Dir(path))
	require.NoError(t, statErr)

	got, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, int64(200), got[0].Timestamp)
	assert.Equal(t, int64(100), got[1].Timestamp)
	assert.Equal(t, int64(1), got[1].ID)
	assert.Equal(t, int64(2), got[0].ID)
}

func TestScheduleLogJSONL_FilterAgentDeclaredOrActor(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newTempJSONL(t)
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", DeclaredAgentID: "hvac-a", RecordKind: "fire", Timestamp: 100}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", ActorAgentID: "hvac-a", RecordKind: "result", Timestamp: 200}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s2", DeclaredAgentID: "hvac-b", ActorAgentID: "hvac-b", RecordKind: "result", Timestamp: 150}))

	got, err := repo.List(ctx, ScheduleLogFilter{AgentID: "hvac-a"}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, got, 2, "declared 또는 actor 매칭(RD-6)")

	only, err := repo.List(ctx, ScheduleLogFilter{AgentID: "hvac-b"}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, only, 1)
	assert.Equal(t, "s2", only[0].ScheduleID)
}

func TestScheduleLogJSONL_FilterTargetActionResult(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newTempJSONL(t)
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Target: "group_id=g1", Action: "set_power", Result: "ok", Timestamp: 100}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s2", RecordKind: "result", Target: "group_id=g2", Action: "set_fan_speed", Result: "error", Timestamp: 200}))

	byTarget, err := repo.List(ctx, ScheduleLogFilter{Target: "group_id=g1"}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, byTarget, 1)
	assert.Equal(t, "s1", byTarget[0].ScheduleID)

	byAction, err := repo.List(ctx, ScheduleLogFilter{Action: "set_fan_speed"}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, byAction, 1)
	assert.Equal(t, "s2", byAction[0].ScheduleID)

	byResult, err := repo.List(ctx, ScheduleLogFilter{Result: "error"}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, byResult, 1)
	assert.Equal(t, "s2", byResult[0].ScheduleID)
}

func TestScheduleLogJSONL_OrderAscDesc(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newTempJSONL(t)
	for _, ts := range []int64{100, 200, 300} {
		require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: ts}))
	}

	asc, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0, "asc")
	require.NoError(t, err)
	require.Len(t, asc, 3)
	assert.Equal(t, int64(100), asc[0].Timestamp)
	assert.Equal(t, int64(300), asc[2].Timestamp)

	for _, ord := range []string{"", "desc"} {
		desc, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0, ord)
		require.NoError(t, err)
		require.Len(t, desc, 3)
		assert.Equal(t, int64(300), desc[0].Timestamp, "order=%q 는 최신순", ord)
		assert.Equal(t, int64(100), desc[2].Timestamp)
	}
}

func TestScheduleLogJSONL_PaginationAndCount(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newTempJSONL(t)
	for _, ts := range []int64{10, 20, 30, 40, 50} {
		require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: ts}))
	}
	got, err := repo.List(ctx, ScheduleLogFilter{}, 2, 1, "")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, int64(40), got[0].Timestamp)
	assert.Equal(t, int64(30), got[1].Timestamp)

	n, err := repo.Count(ctx, ScheduleLogFilter{})
	require.NoError(t, err)
	assert.Equal(t, 5, n, "Count 는 페이지네이션 무관 전체")
}

func TestScheduleLogJSONL_Clear(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newTempJSONL(t)
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 10}))
	require.NoError(t, repo.Clear(ctx))

	got, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0, "")
	require.NoError(t, err)
	assert.Empty(t, got)
	n, err := repo.Count(ctx, ScheduleLogFilter{})
	require.NoError(t, err)
	assert.Equal(t, 0, n)
}

func TestScheduleLogJSONL_MalformedLineSkipped(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, path := newTempJSONL(t)
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 100}))

	// 파일에 손상된(비-JSON) 줄과 빈 줄을 끼워 넣는다.
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("this-is-not-json\n\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// 손상된 줄 뒤에 정상 레코드를 다시 append.
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s2", RecordKind: "result", Timestamp: 200}))

	got, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, got, 2, "손상된 줄은 건너뛰고 정상 레코드만 반환")
	assert.Equal(t, int64(200), got[0].Timestamp)
	assert.Equal(t, int64(100), got[1].Timestamp)
}

func TestScheduleLogJSONL_PersistAcrossReopen(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "schedule_log.jsonl")

	repo1, err := NewScheduleLogJSONLRepository(path)
	require.NoError(t, err)
	require.NoError(t, repo1.Append(ctx, ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 100}))
	require.NoError(t, repo1.Close())

	// 재오픈: 기존 레코드가 보이고 nextID 가 이어져야 한다.
	repo2, err := NewScheduleLogJSONLRepository(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo2.Close() })
	require.NoError(t, repo2.Append(ctx, ScheduleLogRecord{ScheduleID: "s2", RecordKind: "result", Timestamp: 200}))

	got, err := repo2.List(ctx, ScheduleLogFilter{}, 0, 0, "")
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, int64(2), got[0].ID, "재오픈 후 ID 가 단조 증가로 이어짐")
}

func TestScheduleLogJSONL_ClosedError(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	repo, _ := newTempJSONL(t)
	require.NoError(t, repo.Close())

	assert.ErrorIs(t, repo.Append(ctx, ScheduleLogRecord{ScheduleID: "s1"}), ErrScheduleLogClosed)
	_, err := repo.List(ctx, ScheduleLogFilter{}, 0, 0, "")
	assert.ErrorIs(t, err, ErrScheduleLogClosed)
	_, err = repo.Count(ctx, ScheduleLogFilter{})
	assert.ErrorIs(t, err, ErrScheduleLogClosed)
	assert.ErrorIs(t, repo.Clear(ctx), ErrScheduleLogClosed)
}
