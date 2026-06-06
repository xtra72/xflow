// remote_query_test.go 는 M8(그룹 J) 원격 READ 프록시 REST API 의 게이팅·매핑·라우팅을
// 검증한다(@SPEC:SPEC-REMOTE-001 M8, REQ-J01/J05/J07).
//
// 검증:
//   - 도메인별 happy path: 라우트 → (domain, query_action) 매핑 + redacted 본문 통과.
//   - 게이팅: 미관리 → 503, 노출 범위 밖 → 404(노출 위반 감사).
//   - 실패 매핑: 타임아웃 → 504, 노드 ok:false → 502.
//   - admin 권한 강제(node-role 거부).
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
)

// fakeQuerySvc 는 RemoteQueryService 의 테스트 구현이다.
type fakeQuerySvc struct {
	data       json.RawMessage
	err        error
	exposed    map[string]bool // key = kind+"/"+id
	dispatched []string        // "domain/action/id"
	scope      []string        // "kind/id"
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
	}
	f.dispatched = append(f.dispatched, domain+"/"+action+"/"+id)
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

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
