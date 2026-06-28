// remote_editing_test.go 는 M7 원격 자원 편집 REST API(플로우/에이전트 FULL CRUD)의
// 게이팅·전파·실패 매핑·캐시 갱신·감사를 검증한다(@SPEC:SPEC-REMOTE-001 M7, 그룹 I,
// REQ-I01~I06/I11/I12, E08).
//
// 검증 항목:
//   - 6개 엔드포인트 happy path(POST/PATCH/DELETE × flow/agent) 상태 코드(201/200/204).
//   - 게이팅: 오프라인/미승인 → 503, 노출 범위 밖 update/delete → 404.
//   - 실패 매핑: 타임아웃 → 504, 노드 적용 실패 → 502.
//   - 캐시 갱신: 성공 시에만 미러 갱신(update upsert / delete remove), create 는 강제
//     미러 채우지 않음(opt-in 보존), 실패 시 미러 불변.
//   - 감사: 편집 시 actor 가 Dispatch ctx 로 전파됨(REQ-I12 who, 시크릿 없음).
//   - admin 권한 강제(node-role 거부).
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// fakeEditSvc 는 RemoteEditService 의 테스트 구현이다.
type fakeEditSvc struct {
	// dispatch 동작 제어.
	dispatchResult json.RawMessage
	dispatchErr    error

	// 노출 범위(미러 존재) 응답. key = kind+"/"+id.
	exposed map[string]bool

	// 기록.
	dispatched    []string // "domain/action/id"
	dispatchActor string   // 마지막 Dispatch 의 ctx actor(REQ-I12 who)
	upserts       []storage.MirroredResource
	deletes       []string // "kind/id"
	scopeChecked  []string // "kind/id"
}

func newFakeEditSvc() *fakeEditSvc {
	return &fakeEditSvc{exposed: make(map[string]bool)}
}

func (f *fakeEditSvc) Dispatch(ctx context.Context, _ string, domain, action string, args json.RawMessage) (json.RawMessage, error) {
	f.dispatchActor = remote.ActorFromContext(ctx)
	// args 에서 id 를 추출(있으면) — 기록용.
	id := ""
	var m map[string]any
	if json.Unmarshal(args, &m) == nil {
		if v, ok := m["id"].(string); ok {
			id = v
		}
	}
	f.dispatched = append(f.dispatched, domain+"/"+action+"/"+id)
	if f.dispatchErr != nil {
		return nil, f.dispatchErr
	}
	return f.dispatchResult, nil
}

func (f *fakeEditSvc) IsResourceExposed(_ context.Context, _ string, kind, id string) (bool, error) {
	f.scopeChecked = append(f.scopeChecked, kind+"/"+id)
	return f.exposed[kind+"/"+id], nil
}

func (f *fakeEditSvc) UpsertMirror(_ context.Context, _ string, item storage.MirroredResource) error {
	f.upserts = append(f.upserts, item)
	return nil
}

func (f *fakeEditSvc) DeleteMirror(_ context.Context, _ string, kind, id string) error {
	f.deletes = append(f.deletes, kind+"/"+id)
	return nil
}

// doEdit 는 admin role + actor 컨텍스트와 JSON 본문으로 편집 요청을 보낸다.
func doEdit(t *testing.T, svc RemoteEditService, role, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteEditHandler(svc, nil)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if role != "" {
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), "admin-user")
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// --- happy path ----------------------------------------------------------------

// TestEdit_CreateFlow 는 POST /flows 가 201 + 노드 채번 id 를 반환하는지 검증한다(REQ-I01).
func TestEdit_CreateFlow(t *testing.T) {
	svc := newFakeEditSvc()
	svc.dispatchResult = json.RawMessage(`{"id":"node-assigned-1","name":"F"}`)

	rec := doEdit(t, svc, "admin", http.MethodPost, "/api/v1/remote/nodes/n1/flows",
		`{"name":"F","definition":{"nodes":[]}}`)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Contains(t, rec.Body.String(), "node-assigned-1")
	assert.Equal(t, []string{"flow/create/"}, svc.dispatched)
	// create 는 미러를 강제로 채우지 않는다(opt-in 보존 — OPEN QUESTION 9).
	assert.Empty(t, svc.upserts, "create 는 미러를 자동 채우지 않아야 함")
}

// TestEdit_UpdateFlow 는 PATCH /flows/{id} 가 200 + 성공 시 미러 upsert 하는지 검증한다
// (REQ-I02/E08).
func TestEdit_UpdateFlow(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["flow/f1"] = true
	svc.dispatchResult = json.RawMessage(`{"id":"f1","name":"F2"}`)

	rec := doEdit(t, svc, "admin", http.MethodPatch, "/api/v1/remote/nodes/n1/flows/f1",
		`{"definition":{"nodes":[]}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"flow/update/f1"}, svc.dispatched)
	require.Len(t, svc.upserts, 1, "update 성공 시 미러를 upsert 해야 함")
	assert.Equal(t, "f1", svc.upserts[0].ID)
}

// TestEdit_DeleteFlow 는 DELETE /flows/{id} 가 204 + 성공 시 미러 remove 하는지 검증한다
// (REQ-I03/E08).
func TestEdit_DeleteFlow(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["flow/f1"] = true

	rec := doEdit(t, svc, "admin", http.MethodDelete, "/api/v1/remote/nodes/n1/flows/f1", "")

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"flow/delete/f1"}, svc.dispatched)
	assert.Equal(t, []string{"flow/f1"}, svc.deletes)
}

// TestEdit_CreateAgent 는 POST /agents 가 201 을 반환하는지 검증한다(REQ-I04).
func TestEdit_CreateAgent(t *testing.T) {
	svc := newFakeEditSvc()
	svc.dispatchResult = json.RawMessage(`{"id":"a-new","name":"A"}`)

	rec := doEdit(t, svc, "admin", http.MethodPost, "/api/v1/remote/nodes/n1/agents",
		`{"name":"A","type":"mqtt"}`)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, []string{"agent/create/"}, svc.dispatched)
}

// TestEdit_UpdateAgent 는 PATCH /agents/{id} 가 200 + 미러 upsert 하는지 검증한다(REQ-I04).
func TestEdit_UpdateAgent(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["agent/a1"] = true
	svc.dispatchResult = json.RawMessage(`{"id":"a1","name":"A2"}`)

	rec := doEdit(t, svc, "admin", http.MethodPatch, "/api/v1/remote/nodes/n1/agents/a1",
		`{"config":{"qos":1}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"agent/update/a1"}, svc.dispatched)
	require.Len(t, svc.upserts, 1)
}

// TestEdit_DeleteAgent 는 DELETE /agents/{id} 가 204 + 미러 remove 하는지 검증한다(REQ-I04).
func TestEdit_DeleteAgent(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["agent/a1"] = true

	rec := doEdit(t, svc, "admin", http.MethodDelete, "/api/v1/remote/nodes/n1/agents/a1", "")

	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"agent/a1"}, svc.deletes)
}

// --- 게이팅 -----------------------------------------------------------------

// TestEdit_Offline503 은 오프라인/미승인 노드 편집이 503 으로 매핑되는지 검증한다
// (REQ-I05/I11 — ErrNodeNotManaged → 503).
func TestEdit_Offline503(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["flow/f1"] = true
	svc.dispatchErr = remote.ErrNodeNotManaged

	rec := doEdit(t, svc, "admin", http.MethodPatch, "/api/v1/remote/nodes/n1/flows/f1",
		`{"definition":{}}`)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, svc.upserts, "실패 시 미러는 갱신되지 않아야 함")
}

// TestEdit_NotExposed404 는 노출 범위 밖 update 가 404 로 거부되고 명령을 디스패치하지
// 않는지 검증한다(REQ-I05 — 범위 밖 편집 거부).
func TestEdit_NotExposed404(t *testing.T) {
	svc := newFakeEditSvc()
	// exposed 비어 있음 → 범위 밖.

	rec := doEdit(t, svc, "admin", http.MethodPatch, "/api/v1/remote/nodes/n1/flows/f1",
		`{"definition":{}}`)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, svc.dispatched, "범위 밖 자원은 명령을 디스패치하지 않아야 함")
}

// TestEdit_DeleteNotExposed404 는 노출 범위 밖 delete 도 404 로 거부되는지 검증한다.
func TestEdit_DeleteNotExposed404(t *testing.T) {
	svc := newFakeEditSvc()

	rec := doEdit(t, svc, "admin", http.MethodDelete, "/api/v1/remote/nodes/n1/flows/f1", "")

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Empty(t, svc.deletes)
}

// --- 실패 매핑 -----------------------------------------------------------------

// TestEdit_Timeout504 는 명령 타임아웃이 504 로 매핑되는지 검증한다(REQ-I11/D06).
func TestEdit_Timeout504(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["flow/f1"] = true
	svc.dispatchErr = remote.ErrCommandTimeout

	rec := doEdit(t, svc, "admin", http.MethodPatch, "/api/v1/remote/nodes/n1/flows/f1",
		`{"definition":{}}`)

	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
	assert.Empty(t, svc.upserts)
}

// TestEdit_ApplyFail502 는 노드 적용 실패가 502 로 매핑되는지 검증한다(REQ-I11/D09).
func TestEdit_ApplyFail502(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["agent/a1"] = true
	svc.dispatchErr = remote.ErrCommandFailed

	rec := doEdit(t, svc, "admin", http.MethodPatch, "/api/v1/remote/nodes/n1/agents/a1",
		`{"config":{}}`)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Empty(t, svc.upserts)
}

// TestEdit_CreateFail502 는 create 실패 시 502 + 미러 불변을 검증한다(REQ-I11/E08).
func TestEdit_CreateFail502(t *testing.T) {
	svc := newFakeEditSvc()
	svc.dispatchErr = remote.ErrCommandFailed

	rec := doEdit(t, svc, "admin", http.MethodPost, "/api/v1/remote/nodes/n1/flows",
		`{"name":"F","definition":{}}`)

	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Empty(t, svc.upserts)
}

// --- 감사/권한 -----------------------------------------------------------------

// TestEdit_ActorThreadedToDispatch 는 편집 시 actor 가 Dispatch ctx 로 전파되는지
// 검증한다(REQ-I12 who — 감사는 Dispatch 가 actor/node/domain/action 으로 영속).
func TestEdit_ActorThreadedToDispatch(t *testing.T) {
	svc := newFakeEditSvc()
	svc.exposed["flow/f1"] = true
	svc.dispatchResult = json.RawMessage(`{"id":"f1"}`)

	rec := doEdit(t, svc, "admin", http.MethodPatch, "/api/v1/remote/nodes/n1/flows/f1",
		`{"definition":{}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "admin-user", svc.dispatchActor, "actor 가 Dispatch ctx 로 전파되어야 함")
}

// TestEdit_NodeRoleRejected 는 node-role 이 편집 엔드포인트에서 거부되는지 검증한다
// (REQ-F04 — admin 만 편집).
func TestEdit_NodeRoleRejected(t *testing.T) {
	svc := newFakeEditSvc()

	rec := doEdit(t, svc, "node", http.MethodPost, "/api/v1/remote/nodes/n1/flows",
		`{"name":"F","definition":{}}`)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, svc.dispatched)
}
