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
	// 마지막 List 호출의 필터/정렬을 기록한다(핸들러 쿼리 파라미터 배선 검증용).
	lastFilter storage.ScheduleLogFilter
	lastOrder  string
}

func (m *memScheduleLog) Append(_ context.Context, rec storage.ScheduleLogRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.ID = int64(len(m.records) + 1)
	m.records = append(m.records, rec)
	return nil
}

func (m *memScheduleLog) List(_ context.Context, f storage.ScheduleLogFilter, limit, offset int, order string) ([]storage.ScheduleLogRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastFilter = f
	m.lastOrder = order
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
		if f.Target != "" && r.Target != f.Target {
			continue
		}
		if f.Action != "" && r.Action != f.Action {
			continue
		}
		if f.Result != "" && r.Result != f.Result {
			continue
		}
		filtered = append(filtered, r)
	}
	// order="asc" 면 오래된순, 그 외("" / "desc" 포함)는 최신순(기본값).
	ascending := order == "asc"
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Timestamp != filtered[j].Timestamp {
			if ascending {
				return filtered[i].Timestamp < filtered[j].Timestamp
			}
			return filtered[i].Timestamp > filtered[j].Timestamp
		}
		if ascending {
			return filtered[i].ID < filtered[j].ID
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

// Count 는 필터에 매칭되는 전체 레코드 수를 반환한다(List 와 동일 필터 의미, 페이지네이션 무관).
func (m *memScheduleLog) Count(_ context.Context, f storage.ScheduleLogFilter) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.records {
		if f.ScheduleID != "" && r.ScheduleID != f.ScheduleID {
			continue
		}
		if f.RuleName != "" && r.RuleName != f.RuleName {
			continue
		}
		if f.AgentID != "" && r.DeclaredAgentID != f.AgentID && r.ActorAgentID != f.AgentID {
			continue
		}
		n++
	}
	return n, nil
}

// Clear 는 저장된 모든 레코드를 삭제한다(전체 초기화).
func (m *memScheduleLog) Clear(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.records = nil
	return nil
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

// decodeScheduleLogsPage 는 200 응답 본문을 {items, total} 페이지 형태로 디코드한다.
func decodeScheduleLogsPage(t *testing.T, rec *httptest.ResponseRecorder) (items []ScheduleLogResponse, total int) {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data struct {
			Items []ScheduleLogResponse `json:"items"`
			Total int                   `json:"total"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	return resp.Data.Items, resp.Data.Total
}

// decodeScheduleLogs 는 200 응답의 items 슬라이스만 반환한다(total 을 확인하지 않는 테스트 편의).
func decodeScheduleLogs(t *testing.T, rec *httptest.ResponseRecorder) []ScheduleLogResponse {
	t.Helper()
	items, _ := decodeScheduleLogsPage(t, rec)
	return items
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

// scheduleLogDelete 는 주어진 role 로 인증된 DELETE 요청을 실행하는 헬퍼를 만든다.
func scheduleLogDelete(t *testing.T, repo storage.ScheduleLogRepository) func(role string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewScheduleLogHandler(repo)
	return func(role string) *httptest.ResponseRecorder {
		router := api.NewRouter()
		h.RegisterRoutes(router.Group("/api/v1"))
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/schedules/logs", nil)
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), "tester")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)
		return rec
	}
}

// TestScheduleLogs_TotalReflectsFilterNotPagination 은 total 이 페이지네이션과 무관하게
// 필터 매칭 전체 개수를 반영하는지 검증한다(items 는 limit 만큼만).
func TestScheduleLogs_TotalReflectsFilterNotPagination(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 10},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 20},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 30},
		storage.ScheduleLogRecord{ScheduleID: "s2", RecordKind: "result", Timestamp: 40},
	)
	do := scheduleLogRequest(t, repo)

	// schedule_id=s1 필터: 전체 3건 중 limit 2 만 반환하되 total 은 3.
	items, total := decodeScheduleLogsPage(t, do("/api/v1/schedules/logs?schedule_id=s1&limit=2", "admin"))
	require.Len(t, items, 2)
	assert.Equal(t, 3, total, "total 은 필터 매칭 전체 개수(페이지네이션 무관)")
	// 최신순 확인.
	assert.Equal(t, int64(30), items[0].Timestamp)
	assert.Equal(t, int64(20), items[1].Timestamp)
}

// TestScheduleLogs_NilRepoShape 는 저장소 미구성(nil) 시 {items:[], total:0} 형태를 반환하는지
// 검증한다(AC-5 — 에러 아님).
func TestScheduleLogs_NilRepoShape(t *testing.T) {
	do := scheduleLogRequest(t, nil)
	items, total := decodeScheduleLogsPage(t, do("/api/v1/schedules/logs", "admin"))
	assert.Empty(t, items)
	assert.Equal(t, 0, total)
}

// TestScheduleLogs_Clear 는 DELETE 가 전체 로그를 초기화하고 이후 GET 이 빈 목록 + total 0 을
// 반환하는지 검증한다. admin 게이팅 없이 비-admin 도 초기화할 수 있다(RD-5).
func TestScheduleLogs_Clear(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Timestamp: 100},
		storage.ScheduleLogRecord{ScheduleID: "s2", RecordKind: "result", Timestamp: 200},
	)
	get := scheduleLogRequest(t, repo)
	del := scheduleLogDelete(t, repo)

	// 초기 상태: 2건.
	_, total := decodeScheduleLogsPage(t, get("/api/v1/schedules/logs", "admin"))
	require.Equal(t, 2, total)

	// DELETE(비-admin viewer) → 200 + cleared:true.
	rec := del("viewer")
	require.Equal(t, http.StatusOK, rec.Code, "비-admin 도 초기화 가능(RD-5)")
	var clearResp struct {
		Data struct {
			Cleared bool `json:"cleared"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &clearResp))
	assert.True(t, clearResp.Data.Cleared)

	// 초기화 후: 빈 목록 + total 0.
	items, total := decodeScheduleLogsPage(t, get("/api/v1/schedules/logs", "admin"))
	assert.Empty(t, items)
	assert.Equal(t, 0, total)
}

// TestScheduleLogs_ClearNilRepo 는 저장소 미구성(nil) 시 DELETE 가 no-op 성공(200 + cleared)을
// 반환하는지 검증한다(AC-5 준용).
func TestScheduleLogs_ClearNilRepo(t *testing.T) {
	del := scheduleLogDelete(t, nil)
	rec := del("admin")
	require.Equal(t, http.StatusOK, rec.Code)
}

// TestScheduleLogs_TargetActionResultOrderWiring 은 새 쿼리 파라미터
// (target/action/result/order)가 필터/List 호출로 올바르게 배선되는지 검증한다.
func TestScheduleLogs_TargetActionResultOrderWiring(t *testing.T) {
	repo := &memScheduleLog{}
	seedScheduleLogs(repo,
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Target: "group_id=g1", Action: "set_power", Result: "ok", Timestamp: 100},
		storage.ScheduleLogRecord{ScheduleID: "s1", RecordKind: "result", Target: "group_id=g2", Action: "set_fan_speed", Result: "error", Timestamp: 200},
	)
	do := scheduleLogRequest(t, repo)

	rec := do("/api/v1/schedules/logs?target=group_id=g1&action=set_power&result=ok&order=asc", "admin")
	require.Equal(t, http.StatusOK, rec.Code)

	// 핸들러가 쿼리 파라미터를 필터/order 로 배선했는지 확인.
	repo.mu.Lock()
	gotFilter := repo.lastFilter
	gotOrder := repo.lastOrder
	repo.mu.Unlock()
	assert.Equal(t, "group_id=g1", gotFilter.Target)
	assert.Equal(t, "set_power", gotFilter.Action)
	assert.Equal(t, "ok", gotFilter.Result)
	assert.Equal(t, "asc", gotOrder)

	// 필터가 실제로 적용되어 매칭 레코드만 반환.
	got := decodeScheduleLogs(t, rec)
	require.Len(t, got, 1)
	assert.Equal(t, "group_id=g1", got[0].Target)
	assert.Equal(t, "set_power", got[0].Action)
	assert.Equal(t, "ok", got[0].Result)
}
