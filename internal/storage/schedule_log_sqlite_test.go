// schedule_log_sqlite_test.go 는 ScheduleLogRepository 의 SQLite 구현을 검증한다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 M1, spec §4.1).
//
// 스케줄 로그는 append-only 이며, fire↔result 를 CorrelationID 로 조인한다.
// AgentID 필터는 DeclaredAgentID 또는 ActorAgentID 중 하나라도 일치하면 매칭한다(RD-6).
package storage

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestScheduleLogRepo(t *testing.T) *ScheduleLogSQLiteRepository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "schedule_log.db")
	repo, err := NewScheduleLogSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// fireRecord 는 발화 이벤트 레코드를 구성한다(Result/ActorAgentID 는 빈 값).
func fireRecord(scheduleID, ruleName, declaredAgent string, triggerTime int64) ScheduleLogRecord {
	return ScheduleLogRecord{
		CorrelationID:   scheduleID + ":" + itoa(triggerTime),
		RecordKind:      ScheduleLogRecordKindFire,
		ScheduleID:      scheduleID,
		RuleName:        ruleName,
		DeclaredAgentID: declaredAgent,
		TriggerTime:     triggerTime,
		Timestamp:       triggerTime,
	}
}

// itoa 는 int64 를 10진 문자열로 변환한다(테스트 CorrelationID 조립용).
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestScheduleLog_AppendFireAndList 는 발화 이벤트를 추가하고 조회하는 기본 동작을
// 검증한다. RecordKind=fire, Result/ActorAgentID 가 빈 값이어야 한다.
func TestScheduleLog_AppendFireAndList(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Append(ctx, fireRecord("sched-1", "야간 절전", "agent-a", 1000)))

	recs, err := repo.List(ctx, ScheduleLogFilter{}, 100, 0)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, ScheduleLogRecordKindFire, recs[0].RecordKind)
	assert.Equal(t, "sched-1", recs[0].ScheduleID)
	assert.Equal(t, "agent-a", recs[0].DeclaredAgentID)
	assert.Empty(t, recs[0].Result, "fire 이벤트는 Result 가 비어 있어야 함")
	assert.Empty(t, recs[0].ActorAgentID, "fire 이벤트는 ActorAgentID 가 비어 있어야 함")
	assert.NotZero(t, recs[0].ID)
}

// TestScheduleLog_FireResultJoinByCorrelation 은 동일 CorrelationID 의 fire+result 가
// 둘 다 조회되고 조인 가능한지 검증한다(2 행).
func TestScheduleLog_FireResultJoinByCorrelation(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()

	corr := "sched-1:2000"
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{
		CorrelationID: corr, RecordKind: ScheduleLogRecordKindFire,
		ScheduleID: "sched-1", RuleName: "주간 냉방", DeclaredAgentID: "agent-a",
		TriggerTime: 2000, Timestamp: 2000,
	}))
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{
		CorrelationID: corr, RecordKind: ScheduleLogRecordKindResult,
		ScheduleID: "sched-1", RuleName: "주간 냉방", DeclaredAgentID: "agent-a",
		ActorAgentID: "agent-a", TriggerTime: 2000,
		Target: "group_id=station:0150", Action: "set_power",
		Result: ScheduleLogResultOK, Reason: "members=[..] ok=4/4", Timestamp: 2100,
	}))

	recs, err := repo.List(ctx, ScheduleLogFilter{}, 100, 0)
	require.NoError(t, err)
	require.Len(t, recs, 2, "fire 와 result 두 레코드가 모두 존재해야 함")

	// 동일 CorrelationID 로 조인 가능해야 한다.
	for _, r := range recs {
		assert.Equal(t, corr, r.CorrelationID)
	}
	// 최신순: result(ts=2100)가 fire(ts=2000)보다 먼저.
	assert.Equal(t, ScheduleLogRecordKindResult, recs[0].RecordKind)
	assert.Equal(t, ScheduleLogResultOK, recs[0].Result)
	assert.Equal(t, ScheduleLogRecordKindFire, recs[1].RecordKind)
}

// TestScheduleLog_FilterByScheduleID 는 스케줄 필터가 매칭 레코드만 최신순으로
// 반환하는지 검증한다.
func TestScheduleLog_FilterByScheduleID(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Append(ctx, fireRecord("sched-1", "r1", "agent-a", 1000)))
	require.NoError(t, repo.Append(ctx, fireRecord("sched-1", "r1", "agent-a", 3000)))
	require.NoError(t, repo.Append(ctx, fireRecord("sched-2", "r2", "agent-b", 2000)))

	recs, err := repo.List(ctx, ScheduleLogFilter{ScheduleID: "sched-1"}, 100, 0)
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, int64(3000), recs[0].Timestamp, "최신 레코드가 먼저 와야 함")
	for _, r := range recs {
		assert.Equal(t, "sched-1", r.ScheduleID)
	}
}

// TestScheduleLog_FilterByAgentID 는 AgentID 필터가 DeclaredAgentID 또는
// ActorAgentID 중 하나라도 일치하면 매칭하는지 검증한다(RD-6, 두 하위 케이스).
func TestScheduleLog_FilterByAgentID(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()

	// (1) DeclaredAgentID 로 매칭: 선언된 대상이 agent-x, 실행자는 빈 값(fire).
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{
		CorrelationID: "s1:1000", RecordKind: ScheduleLogRecordKindFire,
		ScheduleID: "s1", DeclaredAgentID: "agent-x", TriggerTime: 1000, Timestamp: 1000,
	}))
	// (2) ActorAgentID 로 매칭: 선언된 대상은 다른 에이전트, 실제 실행자가 agent-x.
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{
		CorrelationID: "s2:2000", RecordKind: ScheduleLogRecordKindResult,
		ScheduleID: "s2", DeclaredAgentID: "agent-y", ActorAgentID: "agent-x",
		TriggerTime: 2000, Result: ScheduleLogResultOK, Timestamp: 2000,
	}))
	// 매칭되지 않아야 하는 레코드.
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{
		CorrelationID: "s3:3000", RecordKind: ScheduleLogRecordKindResult,
		ScheduleID: "s3", DeclaredAgentID: "agent-y", ActorAgentID: "agent-z",
		TriggerTime: 3000, Result: ScheduleLogResultOK, Timestamp: 3000,
	}))

	recs, err := repo.List(ctx, ScheduleLogFilter{AgentID: "agent-x"}, 100, 0)
	require.NoError(t, err)
	require.Len(t, recs, 2, "declared 또는 actor 로 매칭되는 두 레코드가 반환되어야 함")

	// 최신순: ActorAgentID 매칭(ts=2000)이 DeclaredAgentID 매칭(ts=1000)보다 먼저.
	assert.Equal(t, "agent-x", recs[0].ActorAgentID)
	assert.Equal(t, "agent-x", recs[1].DeclaredAgentID)
}

// TestScheduleLog_FilterByRuleName 은 규칙 이름 필터를 검증한다.
func TestScheduleLog_FilterByRuleName(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()

	require.NoError(t, repo.Append(ctx, fireRecord("s1", "야간 절전", "agent-a", 1000)))
	require.NoError(t, repo.Append(ctx, fireRecord("s2", "주간 냉방", "agent-b", 2000)))

	recs, err := repo.List(ctx, ScheduleLogFilter{RuleName: "야간 절전"}, 100, 0)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, "야간 절전", recs[0].RuleName)
	assert.Equal(t, "s1", recs[0].ScheduleID)
}

// TestScheduleLog_Pagination 은 limit/offset 페이지네이션을 검증한다.
func TestScheduleLog_Pagination(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()
	for i := int64(1); i <= 5; i++ {
		require.NoError(t, repo.Append(ctx, fireRecord("s", "r", "agent-a", i*100)))
	}

	page1, err := repo.List(ctx, ScheduleLogFilter{}, 2, 0)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, int64(500), page1[0].Timestamp)
	assert.Equal(t, int64(400), page1[1].Timestamp)

	page2, err := repo.List(ctx, ScheduleLogFilter{}, 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 2)
	assert.Equal(t, int64(300), page2[0].Timestamp)

	// limit<=0/offset<0 은 보정되어 에러 없이 동작한다.
	all, err := repo.List(ctx, ScheduleLogFilter{}, 0, -5)
	require.NoError(t, err)
	require.Len(t, all, 5)
}

// TestScheduleLog_AppendAfterClose 는 닫힌 저장소에 Append 시 ErrScheduleLogClosed
// 계열 에러가 반환되는지 검증한다.
func TestScheduleLog_AppendAfterClose(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "schedule_log.db")
	repo, err := NewScheduleLogSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	require.NoError(t, repo.Close())

	err = repo.Append(context.Background(), fireRecord("s", "r", "agent-a", 1000))
	require.Error(t, err, "닫힌 저장소에 대한 Append 는 실패해야 함")
}

// TestScheduleLog_TargetsJSONRoundTrip 은 Targets(JSON 배열) 문자열이 저장 후
// 그대로 복원되는지 검증한다(fan-out 대상별 ok/error 임베드).
func TestScheduleLog_TargetsJSONRoundTrip(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()

	targets := `[{"target":"agent-a","result":"ok","reason":""},` +
		`{"target":"agent-b","result":"error","reason":"timeout"}]`
	require.NoError(t, repo.Append(ctx, ScheduleLogRecord{
		CorrelationID: "s:1000", RecordKind: ScheduleLogRecordKindResult,
		ScheduleID: "s", ActorAgentID: "agent-a", TriggerTime: 1000,
		Target: "group_id=station:0150", Action: "set_multiple",
		Result: ScheduleLogResultError, Targets: targets,
		Reason: "members=[a,b] ok=1/2", Timestamp: 1000,
	}))

	recs, err := repo.List(ctx, ScheduleLogFilter{}, 100, 0)
	require.NoError(t, err)
	require.Len(t, recs, 1)
	assert.Equal(t, targets, recs[0].Targets, "Targets JSON 이 그대로 복원되어야 함")
	assert.Equal(t, ScheduleLogResultError, recs[0].Result)
	assert.Equal(t, "set_multiple", recs[0].Action)
}

// TestScheduleLog_FactoryConstruction 은 factory 가 sqlite/file/default 모두에서
// sqlite 저장소를 생성하는지 확인한다.
func TestScheduleLog_FactoryConstruction(t *testing.T) {
	t.Parallel()
	for _, typ := range []string{"sqlite", "file", "unknown"} {
		dbPath := filepath.Join(t.TempDir(), "schedule_log.db")
		repo, err := NewScheduleLogRepository(context.Background(), typ, dbPath)
		require.NoError(t, err)
		require.NotNil(t, repo)
		require.NoError(t, repo.Append(context.Background(), fireRecord("s", "r", "agent-a", 1)))
		require.NoError(t, repo.Close())
	}
}

// TestScheduleLog_ConstructionError 는 DB 디렉터리를 만들 수 없을 때(경로 구성요소가
// 파일) 생성이 에러를 반환하는지 검증한다.
func TestScheduleLog_ConstructionError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// 파일을 만들고, 그 파일을 디렉터리처럼 사용하는 경로를 넘긴다 → MkdirAll 실패.
	blocker := filepath.Join(dir, "notadir")
	require.NoError(t, os.WriteFile(blocker, []byte("x"), 0o600))

	badPath := filepath.Join(blocker, "sub", "schedule_log.db")
	repo, err := NewScheduleLogSQLiteRepository(context.Background(), badPath)
	require.Error(t, err, "디렉터리 생성 실패 시 에러를 반환해야 함")
	assert.Nil(t, repo)
}

// TestScheduleLog_MigrationError 는 인덱스 이름과 충돌하는 객체가 이미 존재할 때
// 마이그레이션(스키마 생성)이 실패하고 생성이 에러를 반환하는지 검증한다.
func TestScheduleLog_MigrationError(t *testing.T) {
	t.Parallel()
	dbPath := filepath.Join(t.TempDir(), "schedule_log.db")

	// 우리 인덱스 이름(idx_schedule_log_corr)과 충돌하는 테이블을 미리 만든다.
	// 이후 CREATE INDEX IF NOT EXISTS 가 동일 이름의 다른 객체 종류와 충돌해 실패한다.
	seed, err := sql.Open("sqlite", sqliteDSN(dbPath))
	require.NoError(t, err)
	_, err = seed.Exec(`CREATE TABLE idx_schedule_log_corr (x INTEGER)`)
	require.NoError(t, err)
	require.NoError(t, seed.Close())

	repo, err := NewScheduleLogSQLiteRepository(context.Background(), dbPath)
	require.Error(t, err, "인덱스 이름 충돌 시 마이그레이션이 실패해야 함")
	assert.Nil(t, repo)
}

// TestScheduleLog_EmptyResult 는 빈 저장소 조회가 빈 결과를 반환하는지 확인한다.
func TestScheduleLog_EmptyResult(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	recs, err := repo.List(context.Background(), ScheduleLogFilter{ScheduleID: "missing"}, 10, 0)
	require.NoError(t, err)
	assert.Empty(t, recs)
}

// TestScheduleLog_Count 는 Count 가 List 와 동일한 필터 의미(AgentID 는 declared 또는 actor
// 매칭)로 전체 개수를 반환하는지 검증한다(페이지네이션 무관).
func TestScheduleLog_Count(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Append(ctx, fireRecord("s1", "r", "hvac-a", 10)))
	require.NoError(t, repo.Append(ctx, fireRecord("s1", "r", "hvac-a", 20)))
	require.NoError(t, repo.Append(ctx, fireRecord("s2", "r", "hvac-b", 30)))

	all, err := repo.Count(ctx, ScheduleLogFilter{})
	require.NoError(t, err)
	assert.Equal(t, 3, all)

	byID, err := repo.Count(ctx, ScheduleLogFilter{ScheduleID: "s1"})
	require.NoError(t, err)
	assert.Equal(t, 2, byID, "필터 매칭 전체(페이지네이션 무관)")

	byAgent, err := repo.Count(ctx, ScheduleLogFilter{AgentID: "hvac-a"})
	require.NoError(t, err)
	assert.Equal(t, 2, byAgent, "declared 또는 actor 매칭(RD-6)")
}

// TestScheduleLog_Clear 는 Clear 가 전체 레코드를 삭제하고 테이블은 유지(이후 Append/List
// 정상)하는지 검증한다.
func TestScheduleLog_Clear(t *testing.T) {
	t.Parallel()
	repo := newTestScheduleLogRepo(t)
	ctx := context.Background()
	require.NoError(t, repo.Append(ctx, fireRecord("s1", "r", "hvac-a", 10)))
	require.NoError(t, repo.Append(ctx, fireRecord("s2", "r", "hvac-b", 20)))

	require.NoError(t, repo.Clear(ctx))

	recs, err := repo.List(ctx, ScheduleLogFilter{}, 100, 0)
	require.NoError(t, err)
	assert.Empty(t, recs)
	n, err := repo.Count(ctx, ScheduleLogFilter{})
	require.NoError(t, err)
	assert.Equal(t, 0, n)

	// 테이블 유지 확인: Clear 후에도 Append 가 정상 동작한다.
	require.NoError(t, repo.Append(ctx, fireRecord("s3", "r", "hvac-c", 30)))
	after, err := repo.List(ctx, ScheduleLogFilter{}, 100, 0)
	require.NoError(t, err)
	require.Len(t, after, 1)
}
