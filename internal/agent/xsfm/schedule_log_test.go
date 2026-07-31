package xsfm

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/storage"
)

// ---------------------------------------------------------------------------
// SPEC-SCHEDULE-VIEW-001 M2 — 스케줄 로그 result 이벤트 (xsfm 측)
// ---------------------------------------------------------------------------

// mockScheduleLogRepo 는 storage.ScheduleLogRepository 의 테스트용 목이다.
type mockScheduleLogRepo struct {
	mu         sync.Mutex
	appended   []storage.ScheduleLogRecord
	failAppend bool
}

var _ storage.ScheduleLogRepository = (*mockScheduleLogRepo)(nil)

func newMockScheduleLogRepo() *mockScheduleLogRepo { return &mockScheduleLogRepo{} }

func (m *mockScheduleLogRepo) Append(_ context.Context, rec storage.ScheduleLogRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.failAppend {
		return fmt.Errorf("mock schedule log append failed")
	}
	m.appended = append(m.appended, rec)
	return nil
}

func (m *mockScheduleLogRepo) List(_ context.Context, _ storage.ScheduleLogFilter, _, _ int) ([]storage.ScheduleLogRecord, error) {
	return m.records(), nil
}

func (m *mockScheduleLogRepo) Count(_ context.Context, _ storage.ScheduleLogFilter) (int, error) {
	return len(m.records()), nil
}

func (m *mockScheduleLogRepo) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appended = nil
	return nil
}

func (m *mockScheduleLogRepo) Close() error { return nil }

func (m *mockScheduleLogRepo) records() []storage.ScheduleLogRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]storage.ScheduleLogRecord, len(m.appended))
	copy(out, m.appended)
	return out
}

// corrJSON 은 테스트 명령에 붙일 _correlation 블록 문자열을 만든다.
const testTriggerTime = "2026-07-31T12:00:00Z"

func wantCorrelationID(scheduleID string) string {
	ms, _ := time.Parse(time.RFC3339Nano, testTriggerTime)
	return scheduleID + ":" + strconv.FormatInt(ms.UnixMilli(), 10)
}

// 시나리오 (a): 예약 개별 제어 → result 레코드 1건. correlation_id·ActorAgentID(실제 실행)·
// DeclaredAgentID(선언 대상)·Targets 임베드를 검증한다. fire 이벤트(trigger 측)와 동일 공식의
// correlation_id 로 조인된다(AC-16).
func TestScheduleLog_IndividualResultRecorded(t *testing.T) {
	repo := newMockScheduleLogRepo()
	SetScheduleLogRepository(repo)
	defer SetScheduleLogRepository(nil)

	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil)) // ap-101, timeout 0

	cmd := `{"command":"set_power","device_id":"ap-101","params":{"power":true},` +
		`"_correlation":{"schedule_id":"sched-1","rule_name":"야간 소등","declared_agent_id":"hvac-1","trigger_time":"` + testTriggerTime + `"}}`
	_, err := ap.Process([]byte(cmd))
	require.NoError(t, err)

	recs := repo.records()
	require.Len(t, recs, 1, "예약 개별 제어는 result 레코드 1건")
	r := recs[0]
	assert.Equal(t, storage.ScheduleLogRecordKindResult, r.RecordKind)
	assert.Equal(t, wantCorrelationID("sched-1"), r.CorrelationID, "fire 와 동일 공식의 correlation_id")
	assert.Equal(t, "sched-1", r.ScheduleID)
	assert.Equal(t, "야간 소등", r.RuleName)
	assert.Equal(t, "hvac-1", r.DeclaredAgentID, "선언된 대상 에이전트")
	assert.NotEmpty(t, r.ActorAgentID, "실제 실행 에이전트(RD-6)")
	assert.Equal(t, storage.ScheduleLogResultOK, r.Result)
	assert.Equal(t, "device_id=ap-101", r.Target)
	assert.Equal(t, "set_power", r.Action)

	// Targets 임베드: 개별 제어는 1원소(ok).
	var targets []scheduleTargetEntry
	require.NoError(t, json.Unmarshal([]byte(r.Targets), &targets))
	require.Len(t, targets, 1)
	assert.Equal(t, "ap-101", targets[0].Target)
	assert.Equal(t, storage.ScheduleLogResultOK, targets[0].Result)
}

// 시나리오 (c): 수동 제어(상관 없음) → 스케줄 로그 0건(AC-6). 감사(recordControlAudit)는 그대로.
func TestScheduleLog_ManualControl_NoRecord(t *testing.T) {
	repo := newMockScheduleLogRepo()
	SetScheduleLogRepository(repo)
	defer SetScheduleLogRepository(nil)

	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil))
	_, err := ap.Process([]byte(`{"command":"set_power","device_id":"ap-101","params":{"power":true}}`))
	require.NoError(t, err)

	assert.Empty(t, repo.records(), "수동 제어(상관 없음)는 스케줄 로그를 남기지 않아야 한다(AC-6)")
}

// 시나리오 (d): 예약 fan-out → 집계 result 레코드 1건 + 대상별 Targets 임베드(RD-7).
func TestScheduleLog_FanOutAggregateRecord(t *testing.T) {
	repo := newMockScheduleLogRepo()
	SetScheduleLogRepository(repo)
	defer SetScheduleLogRepository(nil)

	opts := directOpts()
	opts["control_response_timeout"] = "0s"
	opts["devices"] = groupDevices(
		map[string]any{"device_id": "ap-101", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-102", "group_id": "concourse-b1"},
		map[string]any{"device_id": "ap-103", "group_id": "concourse-b1"},
	)
	ap, _ := directAgentWithMock(t, opts)

	cmd := `{"command":"set_power","group_id":"concourse-b1","params":{"power":true},` +
		`"_correlation":{"schedule_id":"sched-2","rule_name":"","declared_agent_id":"hvac-1","trigger_time":"` + testTriggerTime + `"}}`
	_, err := ap.Process([]byte(cmd))
	require.NoError(t, err)

	recs := repo.records()
	require.Len(t, recs, 1, "fan-out 이라도 result 레코드는 1건이어야 한다(RD-7)")
	r := recs[0]
	assert.Equal(t, wantCorrelationID("sched-2"), r.CorrelationID)
	assert.Equal(t, "group_id=concourse-b1", r.Target)
	assert.Equal(t, storage.ScheduleLogResultOK, r.Result, "전 멤버 ok → 집계 ok")

	var targets []scheduleTargetEntry
	require.NoError(t, json.Unmarshal([]byte(r.Targets), &targets))
	require.Len(t, targets, 3, "대상별 결과가 Targets 에 임베드되어야 한다")
	ids := map[string]bool{}
	for _, tg := range targets {
		ids[tg.Target] = true
		assert.Equal(t, storage.ScheduleLogResultOK, tg.Result)
	}
	for _, id := range []string{"ap-101", "ap-102", "ap-103"} {
		assert.True(t, ids[id], "멤버 %s 가 Targets 에 있어야 한다", id)
	}
}

// 저장소 미설정(nil)에서도 예약 제어가 crash 없이 정상 동작한다(graceful no-op, AC-5).
func TestScheduleLog_NilRepoGracefulNoOp(t *testing.T) {
	SetScheduleLogRepository(nil)
	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil))
	cmd := `{"command":"set_power","device_id":"ap-101","params":{"power":true},` +
		`"_correlation":{"schedule_id":"sched-1","declared_agent_id":"hvac-1","trigger_time":"` + testTriggerTime + `"}}`
	_, err := ap.Process([]byte(cmd))
	require.NoError(t, err, "저장소 미설정에서도 제어는 성공")
}

// 기록 실패는 제어 명령을 실패시키지 않는다(best-effort logging, AC-14).
func TestScheduleLog_AppendErrorBestEffort(t *testing.T) {
	repo := newMockScheduleLogRepo()
	repo.failAppend = true
	SetScheduleLogRepository(repo)
	defer SetScheduleLogRepository(nil)

	ap, _ := directAgentWithMock(t, oneDeviceOpts(nil))
	cmd := `{"command":"set_power","device_id":"ap-101","params":{"power":true},` +
		`"_correlation":{"schedule_id":"sched-1","declared_agent_id":"hvac-1","trigger_time":"` + testTriggerTime + `"}}`
	_, err := ap.Process([]byte(cmd))
	require.NoError(t, err, "스케줄 로그 Append 실패해도 제어는 성공(best-effort)")
}

// ---------------------------------------------------------------------------
// 헬퍼 단위 테스트 (분기 정밀 커버리지)
// ---------------------------------------------------------------------------

// scheduleSelectorRef 는 요청에서 대상 셀렉터의 종류·값을 우선순위대로 도출한다:
// device_id > device_name > station > line > group_id > group_name > (없음).
func TestScheduleSelectorRef(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		req       processRequest
		wantKind  string
		wantValue string
	}{
		{"device_id 우선", processRequest{DeviceID: "ap-1", DeviceName: "n", Station: "s"}, "device_id", "ap-1"},
		{"device_name", processRequest{DeviceName: "복도-AP", Station: "s"}, "device_name", "복도-AP"},
		{"station", processRequest{Station: "0150", Line: "L2"}, "station", "0150"},
		{"line", processRequest{Line: "line-2", GroupID: "g"}, "line", "line-2"},
		{"group_id", processRequest{GroupID: "concourse-b1", GroupName: "gn"}, "group_id", "concourse-b1"},
		{"group_name", processRequest{GroupName: "동관 전체"}, "group_name", "동관 전체"},
		{"셀렉터 없음", processRequest{Command: "set_power"}, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			kind, value := scheduleSelectorRef(tt.req)
			assert.Equal(t, tt.wantKind, kind)
			assert.Equal(t, tt.wantValue, value)
		})
	}
}

// parseScheduleTriggerMs 는 RFC3339(Nano) 우선 파싱, 실패 시 epoch ms 정수 관용 수용,
// 어느 쪽도 아니면 0(best-effort)을 반환한다.
func TestParseScheduleTriggerMs(t *testing.T) {
	t.Parallel()
	rfc, _ := time.Parse(time.RFC3339Nano, "2026-07-31T12:00:00Z")
	tests := []struct {
		name string
		in   string
		want int64
	}{
		{"RFC3339", "2026-07-31T12:00:00Z", rfc.UnixMilli()},
		{"RFC3339Nano", "2026-07-31T12:00:00.123456789Z", func() int64 {
			t, _ := time.Parse(time.RFC3339Nano, "2026-07-31T12:00:00.123456789Z")
			return t.UnixMilli()
		}()},
		{"epoch ms 정수 문자열", "1780000000000", 1780000000000},
		{"빈 문자열 → 0", "", 0},
		{"파싱 불가 → 0", "not-a-time", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, parseScheduleTriggerMs(tt.in))
		})
	}
}

// buildScheduleResult 의 나머지 분기(제어 실패 / fan-out 부분성공 / device_id 폴백)를
// 직접 커버한다. 수신자 상태를 쓰지 않으므로 zero 값 에이전트로 호출한다.
func TestBuildScheduleResult(t *testing.T) {
	t.Parallel()
	a := &XSFMAgent{}

	// (1) 제어 자체 실패: 셀렉터 단위 error 1건, Targets 는 셀렉터 값 1원소.
	t.Run("제어 실패 → 셀렉터 단위 error", func(t *testing.T) {
		t.Parallel()
		req := processRequest{Command: "set_power", GroupID: "concourse-b1"}
		target, action, result, targets, reason := a.buildScheduleResult(req, nil, fmt.Errorf("no target matched"))
		assert.Equal(t, "group_id=concourse-b1", target)
		assert.Equal(t, "set_power", action)
		assert.Equal(t, storage.ScheduleLogResultError, result)
		assert.Equal(t, "no target matched", reason)
		var entries []scheduleTargetEntry
		require.NoError(t, json.Unmarshal([]byte(targets), &entries))
		require.Len(t, entries, 1)
		assert.Equal(t, "concourse-b1", entries[0].Target)
		assert.Equal(t, storage.ScheduleLogResultError, entries[0].Result)
		assert.Equal(t, "no target matched", entries[0].Reason)
	})

	// (2) fan-out 부분성공: 한 멤버 error → 집계 error, 멤버별 ok/error 임베드.
	t.Run("fan-out 부분성공 → 집계 error", func(t *testing.T) {
		t.Parallel()
		resp := `{"selector":{"type":"group_id","value":"concourse-b1"},` +
			`"results":[{"device_id":"ap-101","status":"ok"},` +
			`{"device_id":"ap-102","status":"timeout"}]}`
		req := processRequest{Command: "set_power", GroupID: "concourse-b1"}
		target, _, result, targets, reason := a.buildScheduleResult(req, []byte(resp), nil)
		assert.Equal(t, "group_id=concourse-b1", target)
		assert.Equal(t, storage.ScheduleLogResultError, result, "전 멤버 ok 가 아니면 집계 error")
		assert.Equal(t, "group_id=concourse-b1 ok=1/2", reason)
		var entries []scheduleTargetEntry
		require.NoError(t, json.Unmarshal([]byte(targets), &entries))
		require.Len(t, entries, 2)
		assert.Equal(t, storage.ScheduleLogResultOK, entries[0].Result)
		assert.Equal(t, storage.ScheduleLogResultError, entries[1].Result)
		assert.Equal(t, "timeout", entries[1].Reason)
	})

	// (3) fan-out 응답이나 selector 필드 없음 + 빈 results: 집계 ok(ok==len==0), 빈 셀렉터 표기.
	t.Run("fan-out selector 없음 + 빈 results", func(t *testing.T) {
		t.Parallel()
		req := processRequest{Command: "set_fan_speed", Station: "0150"}
		target, action, result, targets, reason := a.buildScheduleResult(req, []byte(`{"results":[]}`), nil)
		assert.Equal(t, "=", target, "selector 미제공 → 빈 종류=빈 값")
		assert.Equal(t, "set_fan_speed", action)
		assert.Equal(t, storage.ScheduleLogResultOK, result)
		assert.Equal(t, "= ok=0/0", reason)
		assert.Equal(t, "[]", targets, "빈 results → 빈 임베드 배열")
	})

	// (4) 개별 제어 성공(응답 device_id): Target=device_id=<응답값>, Targets 1원소 ok.
	t.Run("개별 성공 → 응답 device_id 사용", func(t *testing.T) {
		t.Parallel()
		req := processRequest{Command: "set_power", DeviceName: "복도-AP"}
		target, _, result, targets, reason := a.buildScheduleResult(req, []byte(`{"device_id":"ap-resolved"}`), nil)
		assert.Equal(t, "device_id=ap-resolved", target, "이름 해소 결과 device_id 를 사용")
		assert.Equal(t, storage.ScheduleLogResultOK, result)
		assert.Empty(t, reason)
		var entries []scheduleTargetEntry
		require.NoError(t, json.Unmarshal([]byte(targets), &entries))
		require.Len(t, entries, 1)
		assert.Equal(t, "ap-resolved", entries[0].Target)
	})

	// (5) 개별 제어 성공(응답에 device_id 없음): 요청 device_id 로 폴백.
	t.Run("개별 성공 → 요청 device_id 폴백", func(t *testing.T) {
		t.Parallel()
		req := processRequest{Command: "set_power", DeviceID: "ap-101"}
		target, _, result, _, _ := a.buildScheduleResult(req, []byte(`{}`), nil)
		assert.Equal(t, "device_id=ap-101", target, "응답 device_id 없으면 요청 device_id 폴백")
		assert.Equal(t, storage.ScheduleLogResultOK, result)
	})
}
