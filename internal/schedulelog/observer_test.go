// observer_test.go 는 fire 이벤트 관측 어댑터 FireObserver 를 검증한다
// (SPEC-SCHEDULE-VIEW-001 M2, RD-8 / AC-14 / AC-15).
//
// FireObserver 는 storage 를 알지 않는 제네릭 trigger 노드(node.ScheduleFireObserver)와
// 스케줄 로그 저장소(storage.ScheduleLogRepository) 사이의 구체 어댑터이다. 발화마다 fire
// 레코드 1건을 best-effort 로 append 하며, repo 미설정(nil)이면 no-op, append 실패는 로깅만
// 한다(발화 무영향).
package schedulelog

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/node"
	"github.com/xtra/xflow/internal/storage"
)

// fakeScheduleLogRepo 는 storage.ScheduleLogRepository 의 테스트용 목이다
// (xsfm.mockScheduleLogRepo 와 동일한 패턴). failAppend 로 append 실패 경로를 강제한다.
type fakeScheduleLogRepo struct {
	mu         sync.Mutex
	appended   []storage.ScheduleLogRecord
	failAppend bool
}

var _ storage.ScheduleLogRepository = (*fakeScheduleLogRepo)(nil)

func (f *fakeScheduleLogRepo) Append(_ context.Context, rec storage.ScheduleLogRecord) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failAppend {
		return assert.AnError
	}
	f.appended = append(f.appended, rec)
	return nil
}

func (f *fakeScheduleLogRepo) List(_ context.Context, _ storage.ScheduleLogFilter, _, _ int, _ string) ([]storage.ScheduleLogRecord, error) {
	return f.records(), nil
}

func (f *fakeScheduleLogRepo) Count(_ context.Context, _ storage.ScheduleLogFilter) (int, error) {
	return len(f.records()), nil
}

func (f *fakeScheduleLogRepo) Clear(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.appended = nil
	return nil
}

func (f *fakeScheduleLogRepo) Close() error { return nil }

func (f *fakeScheduleLogRepo) records() []storage.ScheduleLogRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]storage.ScheduleLogRecord, len(f.appended))
	copy(out, f.appended)
	return out
}

// countingLogger 는 warnLogger 의 테스트용 구현으로 Warn 호출 횟수를 센다.
type countingLogger struct {
	mu    sync.Mutex
	warns int
}

func (l *countingLogger) Warn(_ string, _ ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warns++
}

func (l *countingLogger) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.warns
}

// 정상 경로: 발화 1회 → fire 레코드 1건 append. 상관/스케줄/규칙/선언대상/발화시각이 매핑되고
// ActorAgentID·Result(대상 실행 결과)는 비어 있어야 한다(fire 이벤트는 실행 결과를 담지 않음).
func TestFireObserver_OnScheduleFire_AppendsFireRecord(t *testing.T) {
	t.Parallel()

	repo := &fakeScheduleLogRepo{}
	obs := NewFireObserver(repo, nil) // logger nil → slog.Default() 사용(폴백 분기 커버)

	obs.OnScheduleFire(node.ScheduleFireContext{
		CorrelationID:   "sched-1:1000",
		ScheduleID:      "sched-1",
		RuleName:        "야간 소등",
		DeclaredAgentID: "hvac-1",
		TriggerTime:     1000,
	})

	recs := repo.records()
	require.Len(t, recs, 1, "발화 1회는 fire 레코드 1건")
	r := recs[0]
	assert.Equal(t, storage.ScheduleLogRecordKindFire, r.RecordKind)
	assert.Equal(t, "sched-1:1000", r.CorrelationID)
	assert.Equal(t, "sched-1", r.ScheduleID)
	assert.Equal(t, "야간 소등", r.RuleName)
	assert.Equal(t, "hvac-1", r.DeclaredAgentID)
	assert.Equal(t, int64(1000), r.TriggerTime)
	assert.NotZero(t, r.Timestamp, "기록 시각은 채워져야 한다")

	// fire 이벤트는 대상 실행 결과를 담지 않는다(별도 result 이벤트가 동일 상관으로 조인).
	assert.Empty(t, r.ActorAgentID, "fire 이벤트에는 실제 실행 에이전트가 없다")
	assert.Empty(t, r.Result, "fire 이벤트에는 집계 결과가 없다")
	assert.Empty(t, r.Target)
	assert.Empty(t, r.Targets)
}

// repo 미설정(nil): NewFireObserver(nil, ...) → OnScheduleFire 는 no-op(crash 없음, AC-14).
func TestFireObserver_OnScheduleFire_NilRepoNoOp(t *testing.T) {
	t.Parallel()

	obs := NewFireObserver(nil, &countingLogger{})
	assert.NotPanics(t, func() {
		obs.OnScheduleFire(node.ScheduleFireContext{ScheduleID: "sched-x", TriggerTime: 1})
	}, "저장소 미설정이면 no-op 이어야 한다")
}

// nil 수신자: (*FireObserver)(nil).OnScheduleFire → no-op(방어적 nil 가드 분기 커버).
func TestFireObserver_OnScheduleFire_NilReceiverNoOp(t *testing.T) {
	t.Parallel()

	var obs *FireObserver
	assert.NotPanics(t, func() {
		obs.OnScheduleFire(node.ScheduleFireContext{ScheduleID: "sched-x"})
	}, "nil 수신자에서도 no-op 이어야 한다")
}

// append 실패: best-effort — 로깅만 하고 패닉/전파하지 않는다(AC-14). 레코드는 남지 않는다.
func TestFireObserver_OnScheduleFire_AppendErrorBestEffort(t *testing.T) {
	t.Parallel()

	repo := &fakeScheduleLogRepo{failAppend: true}
	logger := &countingLogger{}
	obs := NewFireObserver(repo, logger)

	assert.NotPanics(t, func() {
		obs.OnScheduleFire(node.ScheduleFireContext{
			CorrelationID: "sched-1:1000", ScheduleID: "sched-1", TriggerTime: 1000,
		})
	})
	assert.Empty(t, repo.records(), "append 실패 시 레코드는 남지 않는다")
	assert.Equal(t, 1, logger.count(), "append 실패는 경고 로깅 1회")
}
