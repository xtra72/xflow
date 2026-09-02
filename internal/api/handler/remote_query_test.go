// remote_query_test.go 는 M8(그룹 J) 원격 READ 프록시 REST API 의 게이팅·매핑·라우팅을
// 검증한다(@SPEC:SPEC-REMOTE-001 M8, REQ-J01/J05/J07).
//
// 검증:
//   - 도메인별 happy path: 라우트 → (domain, query_action) 매핑 + redacted 본문 통과.
//   - 게이팅: 미관리 → 503, 노출 범위 밖 → 404(노출 위반 감사).
//   - 실패 매핑: 타임아웃 → 504, 노드 ok:false → 502.
//   - admin 권한 강제(node-role 거부).
//
// @SPEC:SPEC-DASHBOARD-004 (M8 8.3, AC-12): 파일 말미에 원격 대시보드 프록시의
// 응답 형상 회귀 테스트를 추가했다. 본 SPEC 이 프록시 3개 파일을 무변경으로 남긴
// 판단이 안전했음을 증명한다.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// fakeQuerySvc 는 RemoteQueryService 의 테스트 구현이다.
//
// M10(그룹 L): managed/lastOwner 는 노드-레벨 query(dashboard/metrics) 게이팅·owner
// 전달 검증용이다(REQ-L01/L02). 기본 managed=false 이며, 노드-레벨 테스트는 명시 설정한다.
type fakeQuerySvc struct {
	data       json.RawMessage
	err        error
	exposed    map[string]bool // key = kind+"/"+id
	dispatched []string        // "domain/action/id"
	scope      []string        // "kind/id"
	managed    bool            // M10: IsManaged 반환값(노드-레벨 게이팅).
	lastOwner  string          // M10: dashboard.get_mine args.owner 기록.
}

func newFakeQuerySvc() *fakeQuerySvc {
	return &fakeQuerySvc{exposed: make(map[string]bool)}
}

func (f *fakeQuerySvc) DispatchQuery(_ context.Context, _ string, domain, action string, args json.RawMessage) (json.RawMessage, error) {
	id := ""
	var m map[string]any
	if json.Unmarshal(args, &m) == nil {
		if v, ok := m["id"].(string); ok {
			id = v
		}
		if v, ok := m["owner"].(string); ok {
			f.lastOwner = v
		}
	}
	f.dispatched = append(f.dispatched, domain+"/"+action+"/"+id)
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

// IsManaged 는 노드가 승인+온라인인지 반환한다(M10 노드-레벨 게이팅 — REQ-L02/J05).
func (f *fakeQuerySvc) IsManaged(string) bool { return f.managed }

func (f *fakeQuerySvc) IsResourceExposed(_ context.Context, _ string, kind, id string) (bool, error) {
	f.scope = append(f.scope, kind+"/"+id)
	return f.exposed[kind+"/"+id], nil
}

// doQuery 는 admin role 컨텍스트로 GET 질의를 보낸다.
func doQuery(t *testing.T, svc RemoteQueryService, role, target string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteQueryHandler(svc, nil)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodGet, target, nil)
	if role != "" {
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), "admin-user")
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// TestQuery_AgentStatsHappyPath 는 agent/stats 라우트가 매핑되고 redacted 본문을
// 반환하는지 검증한다(REQ-J01).
func TestQuery_AgentStatsHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.exposed["agent/a1"] = true
	svc.data = json.RawMessage(`{"rx":10,"tx":5}`)

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/agents/a1/stats")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"rx":10`)
	assert.Equal(t, []string{"agent/stats/a1"}, svc.dispatched)
}

// TestQuery_DeviceStateHappyPath 는 device/state 라우트 매핑을 검증한다(REQ-J01).
func TestQuery_DeviceStateHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.exposed["device/d1"] = true
	svc.data = json.RawMessage(`{"on":true}`)

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/devices/d1/state")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"device/state/d1"}, svc.dispatched)
}

// TestQuery_FlowNodesHappyPath 는 flow/nodes 라우트 매핑을 검증한다(REQ-J01).
func TestQuery_FlowNodesHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.exposed["flow/f1"] = true
	svc.data = json.RawMessage(`[{"id":"n"}]`)

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/flows/f1/nodes")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"flow/nodes/f1"}, svc.dispatched)
}

// --- M8 보강: 라이브 목록 프록시(.../agents/live, .../flows/live, .../devices/live) ---
//
// 라이브 목록은 노드-레벨 READ(per-resource 노출 범위 없음 — 목록 자체가 노출 필터된
// 자원만 운반)이므로 IsManaged 만 게이트한다(nodeQuery 오케스트레이션 재사용).

// TestQuery_AgentsLiveHappyPath 는 .../agents/live 가 agent/list 로 매핑되고 redacted
// 라이브 목록 본문을 통과시키는지 검증한다(REQ-J04 보강).
func TestQuery_AgentsLiveHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.data = json.RawMessage(`{"data":[{"id":"a1","connected":true,"uptime":"1m"}]}`)

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/agents/live")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"connected":true`)
	assert.Equal(t, []string{"agent/list/"}, svc.dispatched)
	assert.Empty(t, svc.scope, "라이브 목록은 per-resource 노출 범위를 평가하지 않음")
}

// TestQuery_FlowsLiveHappyPath 는 .../flows/live 가 flow/list 로 매핑되는지 검증한다.
func TestQuery_FlowsLiveHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.data = json.RawMessage(`{"data":[{"id":"f1","status":"running"}]}`)

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/flows/live")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"flow/list/"}, svc.dispatched)
}

// TestQuery_DevicesLiveHappyPath 는 .../devices/live 가 device/list 로 매핑되는지 검증한다.
func TestQuery_DevicesLiveHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.data = json.RawMessage(`{"data":[{"id":"d1","online":true}]}`)

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/devices/live")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"device/list/"}, svc.dispatched)
}

// TestQuery_AgentsLiveRequiresAdmin 은 node-role 이 거부(403)되는지 검증한다(REQ-F04).
func TestQuery_AgentsLiveRequiresAdmin(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	rec := doQuery(t, svc, "node", "/api/v1/remote/nodes/n1/agents/live")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, svc.dispatched)
}

// TestQuery_AgentsLiveNotManaged503 은 미관리(미승인/오프라인) 노드가 503 으로
// 매핑되는지 검증한다(REQ-J07/L02).
func TestQuery_AgentsLiveNotManaged503(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = false
	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/agents/live")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, svc.dispatched, "미관리면 디스패치하지 않아야 함")
}

// TestQuery_AgentsLiveTimeout504 는 타임아웃이 504 로 매핑되는지 검증한다(REQ-J07).
func TestQuery_AgentsLiveTimeout504(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.err = remote.ErrQueryTimeout
	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/agents/live")
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

// TestQuery_DevicesLiveNodeError502 는 노드 질의 실패가 502 로 매핑되는지 검증한다(REQ-J07).
func TestQuery_DevicesLiveNodeError502(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.err = remote.ErrQueryFailed
	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/devices/live")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// TestQuery_NotExposed404 은 노출 범위 밖 자원이 404 로 거부되는지 검증한다(REQ-J05).
func TestQuery_NotExposed404(t *testing.T) {
	svc := newFakeQuerySvc()
	// exposed 비어 있음 → 범위 밖.
	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/agents/a1/stats")
	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, svc.dispatched, "범위 밖이면 디스패치하지 않아야 함")
}

// TestQuery_NotManaged503 은 미관리 노드가 503 으로 매핑되는지 검증한다(REQ-J07).
func TestQuery_NotManaged503(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.exposed["agent/a1"] = true
	svc.err = remote.ErrNodeNotManaged

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/agents/a1/stats")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestQuery_Timeout504 은 타임아웃이 504 로 매핑되는지 검증한다(REQ-J07).
func TestQuery_Timeout504(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.exposed["device/d1"] = true
	svc.err = remote.ErrQueryTimeout

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/devices/d1/state")
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

// TestQuery_NodeError502 은 노드 질의 실패가 502 로 매핑되는지 검증한다(REQ-J07).
func TestQuery_NodeError502(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.exposed["flow/f1"] = true
	svc.err = remote.ErrQueryFailed

	rec := doQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/flows/f1")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// TestQuery_RequiresAdmin 은 node-role 이 거부(403)되는지 검증한다(REQ-F04).
func TestQuery_RequiresAdmin(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.exposed["agent/a1"] = true
	rec := doQuery(t, svc, "node", "/api/v1/remote/nodes/n1/agents/a1/stats")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// -----------------------------------------------------------------------------
// @SPEC:SPEC-DASHBOARD-004 (M8 8.3, acceptance.md AC-12) — 원격 대시보드 프록시 회귀
// -----------------------------------------------------------------------------
//
// 본 SPEC 은 원격 프록시 3개 파일(internal/api/handler/remote_query.go,
// cmd/xflowd/remote_query.go, web/src/services/api/remoteService.ts)을 **의도적으로
// 건드리지 않았다**. 아래 테스트는 그 판단이 안전했음을 증명한다 — 즉 프록시가
// 직렬화하는 DTO 가 클라이언트가 소비하는 형상 그대로인지 고정한다.
//
// 기존 remote_dashboard_query_test.go 는 손으로 쓴 가짜 JSON 으로 라우팅·게이팅만
// 검증하므로 형상 회귀를 잡지 못한다. 여기서는 **실제 노드 측 생산 경로**
// (DashboardSnapshotShim → DashboardSnapshotToDTO → json.Marshal,
//  cmd/xflowd/remote_query.go queryDashboard 와 동일한 합성)의 산출 바이트를
// 프록시에 통과시켜 응답을 검사한다.

// clientDashboardSnapshotKeys 는 web/src/types/dashboard.ts 의 `DashboardSnapshot`
// 인터페이스 필드 집합이다. 클라이언트 계약의 사본이므로 임의로 늘리지 않는다.
var clientDashboardSnapshotKeys = []string{"scope", "owner", "version", "updatedAt", "payload"}

// clientDashboardPayloadKeys 는 `DashboardPayload` 인터페이스 필드 집합이다.
// 그리드 설정 3종은 원본에 값이 있을 때만 직렬화되므로(shim 의 omitempty),
// 값이 있는 픽스처에서 전량 존재해야 한다.
var clientDashboardPayloadKeys = []string{
	"dashboardPages", "activeDashboardId",
	"dashboardGridCols", "dashboardShowGridLines", "dashboardRefreshInterval",
	"deviceGridLayout",
}

// clientDashboardPageKeys 는 `DashboardPageConfig` 중 레거시 스냅샷이 싣는 필드이다.
var clientDashboardPageKeys = []string{"id", "name", "isDefault", "panels", "layout"}

// nodeDashboardBytes 는 노드가 dashboard/get_{shared,mine} 에 응답하는 바이트를
// 실제 생산 경로로 만든다 — cmd/xflowd/remote_query.go queryDashboard 와 동일하다.
func nodeDashboardBytes(t *testing.T, repo storage.DashboardRepository, scope, owner string) json.RawMessage {
	t.Helper()
	snap, err := NewDashboardSnapshotShim(repo).Get(context.Background(), scope, owner)
	require.NoError(t, err)
	raw, err := json.Marshal(DashboardSnapshotToDTO(snap))
	require.NoError(t, err)
	return raw
}

// proxyDashboardData 는 노드 바이트를 실제 프록시 라우트에 통과시키고 엔벨로프의
// data 를 돌려준다.
func proxyDashboardData(t *testing.T, node json.RawMessage, path string) json.RawMessage {
	t.Helper()
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.data = node

	rec := doNodeQuery(t, svc, "admin", path)
	require.Equal(t, http.StatusOK, rec.Code, "본문: %s", rec.Body.String())

	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.True(t, env.Success)
	return env.Data
}

// objectKeys 는 JSON object 의 최상위 키를 정렬해 돌려준다.
func objectKeys(t *testing.T, raw json.RawMessage) []string {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &m), "object 가 아니다: %s", raw)
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestRemoteQueryDashboard_ShapeUnchanged 는 shim 합성 결과가 프록시를 지나 도착한
// 응답의 형상이 클라이언트 계약과 **정확히** 일치함을 고정한다 (AC-12).
//
// 키 집합을 부분 포함(Contains)이 아니라 **완전 일치(ElementsMatch)** 로 비교한다.
// 누락은 클라이언트를 깨뜨리고, 추가는 계약에 없는 필드가 조용히 새어 나가는 것이며,
// 둘 다 원격 노드 버전 혼재 상황에서 진단이 어렵다.
func TestRemoteQueryDashboard_ShapeUnchanged(t *testing.T) {
	repo := newShimRepo(t)
	mustCreate(t, repo, "sh1", "공유 대시보드", "root", "shared",
		`{"panels":[{"id":"p1","type":"stat"}],"layout":[{"i":"p1","x":0,"y":0,"w":2,"h":2}],`+
			`"gridCols":12,"showGridLines":true,"refreshInterval":5000}`)
	mustCreate(t, repo, "pv1", "root 개인", "root", "private",
		`{"panels":[],"layout":[],"gridCols":10,"showGridLines":false,"refreshInterval":3000}`)

	t.Run("shared — 최상위 키가 클라이언트 계약과 완전히 일치한다", func(t *testing.T) {
		node := nodeDashboardBytes(t, repo, "global", "")
		data := proxyDashboardData(t, node, "/api/v1/remote/nodes/n1/dashboards/shared")

		assert.ElementsMatch(t, clientDashboardSnapshotKeys, objectKeys(t, data),
			"DashboardSnapshot 필드 집합이 바뀌면 remoteService.getRemoteDashboard 소비자가 깨진다")
	})

	t.Run("shared — payload 키가 클라이언트 계약과 완전히 일치한다", func(t *testing.T) {
		node := nodeDashboardBytes(t, repo, "global", "")
		data := proxyDashboardData(t, node, "/api/v1/remote/nodes/n1/dashboards/shared")

		var snap struct {
			Scope   string          `json:"scope"`
			Owner   *string         `json:"owner"`
			Version int64           `json:"version"`
			Payload json.RawMessage `json:"payload"`
		}
		require.NoError(t, json.Unmarshal(data, &snap))

		assert.Equal(t, "global", snap.Scope)
		assert.Nil(t, snap.Owner, "scope=global 은 owner=null (클라이언트 타입이 string|null)")
		assert.ElementsMatch(t, clientDashboardPayloadKeys, objectKeys(t, snap.Payload))
	})

	t.Run("shared — dashboardPages 원소 키가 계약과 일치한다", func(t *testing.T) {
		node := nodeDashboardBytes(t, repo, "global", "")
		data := proxyDashboardData(t, node, "/api/v1/remote/nodes/n1/dashboards/shared")

		var snap struct {
			Payload struct {
				Pages []json.RawMessage `json:"dashboardPages"`
			} `json:"payload"`
		}
		require.NoError(t, json.Unmarshal(data, &snap))
		require.Len(t, snap.Payload.Pages, 1, "private 은 global 묶음에서 빠진다")
		assert.ElementsMatch(t, clientDashboardPageKeys, objectKeys(t, snap.Payload.Pages[0]))
	})

	t.Run("mine — owner 가 문자열로 채워진다", func(t *testing.T) {
		node := nodeDashboardBytes(t, repo, "user", "root")
		data := proxyDashboardData(t, node, "/api/v1/remote/nodes/n1/dashboards/mine")

		var snap struct {
			Scope string  `json:"scope"`
			Owner *string `json:"owner"`
		}
		require.NoError(t, json.Unmarshal(data, &snap))
		assert.Equal(t, "user", snap.Scope)
		require.NotNil(t, snap.Owner, "scope=user 는 owner 가 null 이 아니다")
		assert.Equal(t, "root", *snap.Owner)
		assert.ElementsMatch(t, clientDashboardSnapshotKeys, objectKeys(t, data))
	})

	t.Run("프록시는 노드 바이트를 변형하지 않는다", func(t *testing.T) {
		node := nodeDashboardBytes(t, repo, "global", "")
		data := proxyDashboardData(t, node, "/api/v1/remote/nodes/n1/dashboards/shared")

		// writeRaw 가 json.RawMessage 를 그대로 싣는지 — 재직렬화·필드 재정렬이
		// 일어나면 노드/서버 버전 혼재 시 진단 불가능한 차이가 생긴다.
		assert.JSONEq(t, string(node), string(data))
	})

	t.Run("대시보드 0장이어도 계약 형상을 유지한다", func(t *testing.T) {
		// 원격 노드에 대시보드가 없어도 클라이언트는 payload.dashboardPages 를
		// 배열로 기대한다. null 이 되면 .map 이 터진다.
		empty := newShimRepo(t)
		node := nodeDashboardBytes(t, empty, "global", "")
		data := proxyDashboardData(t, node, "/api/v1/remote/nodes/n1/dashboards/shared")

		var snap struct {
			Payload struct {
				Pages             []json.RawMessage `json:"dashboardPages"`
				ActiveDashboardID string            `json:"activeDashboardId"`
				DeviceGridLayout  json.RawMessage   `json:"deviceGridLayout"`
			} `json:"payload"`
		}
		require.NoError(t, json.Unmarshal(data, &snap))
		assert.NotNil(t, snap.Payload.Pages, "dashboardPages 는 null 이 아니라 빈 배열")
		assert.Empty(t, snap.Payload.Pages)
		assert.JSONEq(t, `{}`, string(snap.Payload.DeviceGridLayout))
		assert.ElementsMatch(t, clientDashboardSnapshotKeys, objectKeys(t, data))
	})
}
