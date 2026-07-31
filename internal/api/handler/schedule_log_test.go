// schedule_log_test.go 는 스케줄 로그 조회 API(GET /schedules/logs)를 검증한다
// (@SPEC:SPEC-SCHEDULE-VIEW-001 M3, RD-5/RD-6, AC-4/AC-5/AC-17).
//
// remote_admin_audit_test.go 의 인메모리 fake + 컨텍스트 role 주입 패턴을 준용하되,
// 본 엔드포인트는 admin 게이팅이 없으므로 비-admin role 로도 접근이 성공함을 검증한다.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/storage"
)

// memScheduleLog 는 ScheduleLogRepository 의 인메모리 테스트 구현이다. List 는 필터
// 적용 후 최신순(timestamp 내림차순, id tie-break)으로 반환하고 limit/offset 을 적용한다.
type memScheduleLog struct {
	mu      sync.Mutex
	records []storage.ScheduleLogRecord
}

func (m *memScheduleLog) Append(_ context.Context, rec storage.ScheduleLogRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.ID = int64(len(m.records) + 1)
	m.records = append(m.records, rec)
	return nil
}

func (m *memScheduleLog) List(_ context.Context, f storage.ScheduleLogFilter, limit, offset int) ([]storage.ScheduleLogRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := make([]storage.ScheduleLogRecord, 0)
	for _, r := range m.records {
		if f.ScheduleID != "" && r.ScheduleID != f.ScheduleID {
			continue
		}
		if f.RuleName != "" && r.RuleName != f.RuleName {
			continue
		}
		// AgentID 는 선언(Declared) 또는 실행(Actor) 중 하나라도 일치하면 매칭(RD-6).
		if f.AgentID != "" && r.DeclaredAgentID != f.AgentID && r.ActorAgentID != f.AgentID {
			continue
		}
		filtered = append(filtered, r)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Timestamp != filtered[j].Timestamp {
			return filtered[i].Timestamp > filtered[j].Timestamp
		}
		return filtered[i].ID > filtered[j].ID
	})
	if offset > len(filtered) {
		return nil, nil
	}
	filtered = filtered[offset:]
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (m *memScheduleLog) Close() error { return nil }

// seedScheduleLogs 는 테스트 레코드를 저장소에 추가한다.
func seedScheduleLogs(m *memScheduleLog, recs ...storage.ScheduleLogRecord) {
	for _, r := range recs {
		_ = m.Append(context.Background(), r)
	}
}

// scheduleLogRequest 는 주어진 role 로 인증된 GET 요청을 실행하는 헬퍼를 만든다.
// repo 가 nil 이면 미구성 저장소 경로(AC-5)를 재현한다.
func scheduleLogRequest(t *testing.T, repo storage.ScheduleLogRepository) func(target, role string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewScheduleLogHandler(repo)
	return func(target, role string) *httptest.ResponseRecorder {
		router := api.NewRouter()
		h.RegisterRoutes(router.Group("/api/v1"))
		req := httptest.NewRequest(http.MethodGet, target, nil)
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), "tester")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)
		return rec
	}
}

// decodeScheduleLogs 는 200 응답 본문을 ScheduleLogResponse 슬라이스로 디코드한다.
func decodeScheduleLogs(t *testing.T, rec *httptest.ResponseRecorder) []ScheduleLogResponse {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data []ScheduleLogResponse `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data
}

// TestScheduleLogs_FilterByScheduleID 는 schedule_id 필터가 매칭 레코드만 최신순으로
// 반환하는지 검증한다.
func TestScheduleLogs_FilterByScheduleID(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		storage.ScheduleLogRecord{ScheduleID: "s1", RuleName: "r1", RecordKind: "result", Timestamp: 100},
		storage.ScheduleLogRecord{ScheduleID: "s1", RuleName: "r1", RecordKind: "result", Timestamp: 200},
		storage.ScheduleLogRecord{ScheduleID: "s2", RuleName: "r2", RecordKind: "result", Timestamp: 150},
	)
	do := scheduleLogRequest(t, repo)

	got := decodeScheduleLogs(t, do("/api/v1/schedules/logs?schedule_id=s1", "admin"))
	require.Len(t, got, 2)
	// 최신순(timestamp 내림차순).
	assert.Equal(t, int64(200), got[0].Timestamp)
	assert.Equal(t, int64(100), got[1].Timestamp)
	for _, g := range got {
		assert.Equal(t, "s1", g.ScheduleID)
	}
}

// TestScheduleLogs_FilterByAgentID 는 agent_id 필터가 선언 에이전트 또는 실행 에이전트
// 중 하나라도 일치하는 레코드를 매칭하는지 검증한다(RD-6/AC-4, 두 서브케이스).
func TestScheduleLogs_FilterByAgentID(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		// declared 매칭(fire 이벤트에서는 actor 가 비어 있음).
		storage.ScheduleLogRecord{ScheduleID: "s1", DeclaredAgentID: "hvac-a", RecordKind: "fire", Timestamp: 100},
		// actor 매칭(result 이벤트의 실제 실행 에이전트).
		storage.ScheduleLogRecord{ScheduleID: "s1", ActorAgentID: "hvac-a", RecordKind: "result", Timestamp: 200},
		// 무관 에이전트.
		storage.ScheduleLogRecord{ScheduleID: "s2", DeclaredAgentID: "hvac-b", ActorAgentID: "hvac-b", RecordKind: "result", Timestamp: 150},
	)
	do := scheduleLogRequest(t, repo)

	t.Run("declared 또는 actor 매칭", func(t *testing.T) {
		got := decodeScheduleLogs(t, do("/api/v1/schedules/logs?agent_id=hvac-a", "admin"))
		require.Len(t, got, 2)
		kinds := map[string]bool{}
		for _, g := range got {
			kinds[g.RecordKind] = true
		}
		assert.True(t, kinds["fire"], "declared-agent 매칭(fire) 레코드 포함")
		assert.True(t, kinds["result"], "actor-agent 매칭(result) 레코드 포함")
	})

	t.Run("declared+actor 동일", func(t *testing.T) {
		got := decodeScheduleLogs(t, do("/api/v1/schedules/logs?agent_id=hvac-b", "admin"))
		require.Len(t, got, 1)
		assert.Equal(t, "s2", got[0].ScheduleID)
	})
}

// TestScheduleLogs_FilterByRuleName 는 rule_name 필터를 검증한다.
func TestScheduleLogs_FilterByRuleName(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		storage.ScheduleLogRecord{ScheduleID: "s1", RuleName: "morning", RecordKind: "result", Timestamp: 100},
		storage.ScheduleLogRecord{ScheduleID: "s2", RuleName: "evening", RecordKind: "result", Timestamp: 200},
		storage.ScheduleLogRecord{ScheduleID: "s3", RuleName: "morning", RecordKind: "result", Timestamp: 300},
	)
	do := scheduleLogRequest(t, repo)

	got := decodeScheduleLogs(t, do("/api/v1/schedules/logs?rule_name=morning", "admin"))
	require.Len(t, got, 2)
	for _, g := range got {
		assert.Equal(t, "morning", g.RuleName)
	}
	assert.Equal(t, int64(300), got[0].Timestamp, "최신순")
}

// TestScheduleLogs_Pagination 은 limit/offset 이 최신순 결과에 적용되는지 검증한다.
func TestScheduleLogs_Pagination(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 10},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 20},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 30},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 40},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 50},
	)
	do := scheduleLogRequest(t, repo)

	// 최신순: 50,40,30,20,10 → offset 1 → 40,30,20,10 → limit 2 → 40,30.
	got := decodeScheduleLogs(t, do("/api/v1/schedules/logs?limit=2&offset=1", "admin"))
	require.Len(t, got, 2)
	assert.Equal(t, int64(40), got[0].Timestamp)
	assert.Equal(t, int64(30), got[1].Timestamp)
}

// TestScheduleLogs_NilRepoEmptyList 는 저장소 미구성(nil) 시 200 + 빈 목록을 반환하는지
// 검증한다(AC-5 — 에러가 아니라 빈 목록).
func TestScheduleLogs_NilRepoEmptyList(t *testing.T) {
	do := scheduleLogRequest(t, nil)
	rec := do("/api/v1/schedules/logs", "admin")
	require.Equal(t, http.StatusOK, rec.Code)
	got := decodeScheduleLogs(t, rec)
	assert.Empty(t, got)
}

// TestScheduleLogs_NonAdminAccess 는 비-admin 인증 사용자가 403 없이 로그를 조회할 수
// 있는지 검증한다(RD-5, AC-17 — admin 게이팅 없음).
func TestScheduleLogs_NonAdminAccess(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo, storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 100})
	do := scheduleLogRequest(t, repo)

	rec := do("/api/v1/schedules/logs", "viewer")
	require.Equal(t, http.StatusOK, rec.Code, "비-admin 인증 사용자도 접근 가능(AC-17)")
	got := decodeScheduleLogs(t, rec)
	assert.Len(t, got, 1)
}

// TestScheduleLogs_TargetsSerialization 은 targets 직렬화(유효 JSON 통과, 빈 값→[])를
// 검증한다.
func TestScheduleLogs_TargetsSerialization(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 200, Targets: `[{"target":"g1","result":"ok","reason":"3/3"}]`},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "fire", Timestamp: 100, Targets: ""},
	)
	do := scheduleLogRequest(t, repo)

	got := decodeScheduleLogs(t, do("/api/v1/schedules/logs?schedule_id=s1", "admin"))
	require.Len(t, got, 2)
	// 최신순: result(200) 먼저 — 저장된 JSON 그대로 통과.
	assert.JSONEq(t, `[{"target":"g1","result":"ok","reason":"3/3"}]`, string(got[0].Targets))
	// 빈 값 → 빈 배열(유효 JSON 보장).
	assert.Equal(t, "[]", string(got[1].Targets))
}
