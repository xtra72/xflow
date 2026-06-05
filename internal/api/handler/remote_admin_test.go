// remote_admin_test.go 는 관리 노드 승인/거부/폐기/목록 REST API 의 권한 및 동작을
// 검증한다(@SPEC:SPEC-REMOTE-001 M2, REQ-C03/C07, F03/F04).
//
// admin 권한 강제(node-role 토큰 거부), 미존재 instance_id 404, 정상 흐름을 검증한다.
package handler

import (
	"context"
	"encoding/json"
	"errors"
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

// fakeNodeAdmin 은 NodeAdminService 의 테스트 구현이다.
type fakeNodeAdmin struct {
	nodes          map[string]storage.ManagedNode
	approved       []string
	rejected       []string
	revoked        []string
	getErr         error
	dispatched     []string // domain/action 기록
	dispatchResult json.RawMessage
	dispatchErr    error
	mirror         *mirrorStore // M4 미러 목록(노드 push 로만 변경 — E08)
}

func newFakeNodeAdmin() *fakeNodeAdmin {
	return &fakeNodeAdmin{nodes: make(map[string]storage.ManagedNode), mirror: newMirrorStore()}
}

func (f *fakeNodeAdmin) ListNodes(_ context.Context) ([]storage.ManagedNode, error) {
	out := make([]storage.ManagedNode, 0, len(f.nodes))
	for _, n := range f.nodes {
		out = append(out, n)
	}
	return out, nil
}

func (f *fakeNodeAdmin) Approve(_ context.Context, instanceID string) error {
	if _, ok := f.nodes[instanceID]; !ok {
		return storage.ErrManagedNodeNotFound
	}
	f.approved = append(f.approved, instanceID)
	return nil
}

func (f *fakeNodeAdmin) Reject(_ context.Context, instanceID, _ string) error {
	if _, ok := f.nodes[instanceID]; !ok {
		return storage.ErrManagedNodeNotFound
	}
	f.rejected = append(f.rejected, instanceID)
	return nil
}

func (f *fakeNodeAdmin) Revoke(_ context.Context, instanceID string) error {
	if _, ok := f.nodes[instanceID]; !ok {
		return storage.ErrManagedNodeNotFound
	}
	f.revoked = append(f.revoked, instanceID)
	return nil
}

func (f *fakeNodeAdmin) Dispatch(_ context.Context, _, domain, action string, _ json.RawMessage) (json.RawMessage, error) {
	if f.dispatchErr != nil {
		return nil, f.dispatchErr
	}
	f.dispatched = append(f.dispatched, domain+"/"+action)
	return f.dispatchResult, nil
}

// newAdminRequest 는 role=admin 컨텍스트가 주입된 요청을 만든다.
func newAdminRequest(method, target string, role string) (*api.Router, *httptest.ResponseRecorder, *http.Request) {
	router := api.NewRouter()
	req := httptest.NewRequest(method, target, nil)
	if role != "" {
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
		req = req.WithContext(ctx)
	}
	return router, httptest.NewRecorder(), req
}

func registerAdminRoutes(t *testing.T, svc NodeAdminService, role string) func(method, target string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteAdminHandler(svc, nil)
	return func(method, target string) *httptest.ResponseRecorder {
		router, rec, req := newAdminRequest(method, target, role)
		h.RegisterRoutes(router.Group("/api/v1"))
		router.Handler().ServeHTTP(rec, req)
		return rec
	}
}

// TestRemoteAdmin_ListNodes 는 목록 엔드포인트가 노드를 반환하는지 검증한다(REQ-G01 토대).
func TestRemoteAdmin_ListNodes(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved", Online: true}
	svc.nodes["n2"] = storage.ManagedNode{InstanceID: "n2", Status: "pending"}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodGet, "/api/v1/remote/nodes")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "n1")
	assert.Contains(t, body, "n2")
}

// TestRemoteAdmin_ListPending 는 pending 필터 엔드포인트가 pending 노드만 반환하는지
// 검증한다(REQ-G02 토대).
func TestRemoteAdmin_ListPending(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved"}
	svc.nodes["n2"] = storage.ManagedNode{InstanceID: "n2", Status: "pending"}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodGet, "/api/v1/remote/nodes/pending")

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "n2")
	assert.NotContains(t, body, "n1", "pending 필터는 approved 노드를 제외해야 함")
}

// TestRemoteAdmin_Approve 는 승인 엔드포인트를 검증한다(REQ-C03).
func TestRemoteAdmin_Approve(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "pending"}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodPost, "/api/v1/remote/nodes/n1/approve")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"n1"}, svc.approved)
}

// TestRemoteAdmin_Reject 는 거부 엔드포인트를 검증한다(REQ-C03).
func TestRemoteAdmin_Reject(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "pending"}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodPost, "/api/v1/remote/nodes/n1/reject")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"n1"}, svc.rejected)
}

// TestRemoteAdmin_Revoke 는 폐기 엔드포인트를 검증한다(REQ-C07).
func TestRemoteAdmin_Revoke(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved"}

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodPost, "/api/v1/remote/nodes/n1/revoke")

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"n1"}, svc.revoked)
}

// TestRemoteAdmin_NodeRoleRejected 는 node-role 토큰이 admin 엔드포인트에서 거부되는지
// 검증한다(REQ-F03/F04 — admin 만 명령 발행).
func TestRemoteAdmin_NodeRoleRejected(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "pending"}

	do := registerAdminRoutes(t, svc, "node")
	rec := do(http.MethodPost, "/api/v1/remote/nodes/n1/approve")

	assert.Equal(t, http.StatusForbidden, rec.Code, "node-role 은 admin 엔드포인트에서 거부되어야 함")
	assert.Empty(t, svc.approved, "거부된 요청은 승인을 실행하지 않아야 함")
}

// TestRemoteAdmin_ViewerRoleRejected 는 viewer-role 이 거부되는지 검증한다.
func TestRemoteAdmin_ViewerRoleRejected(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "pending"}

	do := registerAdminRoutes(t, svc, "viewer")
	rec := do(http.MethodPost, "/api/v1/remote/nodes/n1/reject")

	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteAdmin_UnknownNode404 는 미존재 instance_id 에 대해 404 를 반환하는지
// 검증한다.
func TestRemoteAdmin_UnknownNode404(t *testing.T) {
	svc := newFakeNodeAdmin()

	do := registerAdminRoutes(t, svc, "admin")
	rec := do(http.MethodPost, "/api/v1/remote/nodes/missing/approve")

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestRemoteAdmin_InternalError 는 일반 에러가 500 으로 매핑되는지 검증한다.
func TestRemoteAdmin_InternalError(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved"}
	svc.getErr = errors.New("boom")

	h := NewRemoteAdminHandler(&erroringAdmin{}, nil)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/remote/nodes/n1/revoke", nil)
	req = req.WithContext(context.WithValue(req.Context(), api.ContextKeyUserRole(), "admin"))
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// erroringAdmin 은 항상 일반 에러를 반환하는 NodeAdminService 이다.
type erroringAdmin struct{}

func (erroringAdmin) ListNodes(context.Context) ([]storage.ManagedNode, error) {
	return nil, errors.New("boom")
}
func (erroringAdmin) Approve(context.Context, string) error        { return errors.New("boom") }
func (erroringAdmin) Reject(context.Context, string, string) error { return errors.New("boom") }
func (erroringAdmin) Revoke(context.Context, string) error         { return errors.New("boom") }
func (erroringAdmin) Dispatch(context.Context, string, string, string, json.RawMessage) (json.RawMessage, error) {
	return nil, errors.New("boom")
}

func (erroringAdmin) ListMirroredFlows(context.Context, string) ([]remote.MirroredResourceView, error) {
	return nil, errors.New("boom")
}

func (erroringAdmin) ListMirroredAgents(context.Context, string) ([]remote.MirroredResourceView, error) {
	return nil, errors.New("boom")
}

func (erroringAdmin) ListMirroredDevices(context.Context, string) ([]remote.MirroredResourceView, error) {
	return nil, errors.New("boom")
}

func (erroringAdmin) ListAllMirroredFlows(context.Context) ([]remote.MirroredResourceView, error) {
	return nil, errors.New("boom")
}

func (erroringAdmin) ListAllMirroredAgents(context.Context) ([]remote.MirroredResourceView, error) {
	return nil, errors.New("boom")
}

func (erroringAdmin) ListAllMirroredDevices(context.Context) ([]remote.MirroredResourceView, error) {
	return nil, errors.New("boom")
}

// doCommand 는 admin 역할로 command 엔드포인트에 JSON 본문을 POST 한다.
func doCommand(t *testing.T, svc NodeAdminService, role, instanceID, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteAdminHandler(svc, nil)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/remote/nodes/"+instanceID+"/command", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		req = req.WithContext(context.WithValue(req.Context(), api.ContextKeyUserRole(), role))
	}
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// TestRemoteAdmin_Command 는 admin 이 명령을 발행하고 디스패치가 호출되는지
// 검증한다(REQ-D01/F04).
func TestRemoteAdmin_Command(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved", Online: true}
	svc.dispatchResult = json.RawMessage(`{"status":"running"}`)

	rec := doCommand(t, svc, "admin", "n1", `{"domain":"flow","action":"start","args":{"id":"f1"}}`)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, []string{"flow/start"}, svc.dispatched)
	assert.Contains(t, rec.Body.String(), "running")
}

// TestRemoteAdmin_CommandRequiresAdmin 는 node-role 이 명령 발행에서 거부되는지
// 검증한다(REQ-F04 — admin 만 명령 발행).
func TestRemoteAdmin_CommandRequiresAdmin(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1", Status: "approved", Online: true}

	rec := doCommand(t, svc, "node", "n1", `{"domain":"flow","action":"start"}`)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, svc.dispatched, "거부된 요청은 디스패치하지 않아야 함")
}

// TestRemoteAdmin_CommandMissingFields 는 domain/action 누락 시 400 을 반환하는지
// 검증한다.
func TestRemoteAdmin_CommandMissingFields(t *testing.T) {
	svc := newFakeNodeAdmin()
	rec := doCommand(t, svc, "admin", "n1", `{"action":"start"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoteAdmin_CommandOfflineNode 는 미승인/오프라인 노드 명령이 503 으로
// 매핑되는지 검증한다(REQ-D08/B07).
func TestRemoteAdmin_CommandOfflineNode(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.dispatchErr = remote.ErrNodeNotManaged

	rec := doCommand(t, svc, "admin", "off", `{"domain":"flow","action":"start"}`)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestRemoteAdmin_CommandTimeout 는 타임아웃이 504 로 매핑되는지 검증한다(REQ-D06).
func TestRemoteAdmin_CommandTimeout(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.dispatchErr = remote.ErrCommandTimeout

	rec := doCommand(t, svc, "admin", "n1", `{"domain":"flow","action":"deploy"}`)
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

// TestRemoteAdmin_CommandFailed 는 노드 적용 실패가 502 로 매핑되는지 검증한다
// (REQ-D09).
func TestRemoteAdmin_CommandFailed(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.dispatchErr = remote.ErrCommandFailed

	rec := doCommand(t, svc, "admin", "n1", `{"domain":"device","action":"update"}`)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// TestRemoteAdmin_CommandNoConn 는 라이브 연결 부재(ErrNoConn)가 503 으로 매핑되는지
// 검증한다.
func TestRemoteAdmin_CommandNoConn(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.dispatchErr = remote.ErrNoConn
	rec := doCommand(t, svc, "admin", "n1", `{"domain":"flow","action":"start"}`)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// TestRemoteAdmin_CommandInternalError 는 알 수 없는 디스패치 오류가 500 으로
// 매핑되는지 검증한다.
func TestRemoteAdmin_CommandInternalError(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.dispatchErr = errors.New("unexpected")
	rec := doCommand(t, svc, "admin", "n1", `{"domain":"flow","action":"start"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestRemoteAdmin_CommandBadBody 는 잘못된 JSON 본문이 400 으로 매핑되는지 검증한다.
func TestRemoteAdmin_CommandBadBody(t *testing.T) {
	svc := newFakeNodeAdmin()
	rec := doCommand(t, svc, "admin", "n1", `{not json`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// TestRemoteAdmin_RoutePatterns 는 라우트가 /api/remote/* 아래에 별도로 등록되는지
// 간단 검증한다(REQ-N02 — 모니터링과 분리).
func TestRemoteAdmin_RoutePatterns(t *testing.T) {
	svc := newFakeNodeAdmin()
	do := registerAdminRoutes(t, svc, "admin")
	// 알 수 없는 하위 경로는 404 (라우트 미등록).
	rec := do(http.MethodGet, "/api/v1/remote/unknown")
	assert.True(t, rec.Code == http.StatusNotFound || strings.Contains(rec.Body.String(), "NOT_FOUND"))
}
