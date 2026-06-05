// remote_admin_audit_test.go 는 M6 감사 로그 배선(승인/거부/폐기 감사 기록 +
// GET /remote/audit 조회 + 명령 actor 컨텍스트 전파)을 검증한다
// (@SPEC:SPEC-REMOTE-001 M6, REQ-F05/F06).
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"strings"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// memAudit 는 RemoteAuditRepository 의 인메모리 테스트 구현이다.
type memAudit struct {
	mu      sync.Mutex
	records []storage.RemoteAuditRecord
}

func (m *memAudit) Append(_ context.Context, rec storage.RemoteAuditRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.ID = int64(len(m.records) + 1)
	m.records = append(m.records, rec)
	return nil
}

func (m *memAudit) List(_ context.Context, instanceID string, limit, offset int) ([]storage.RemoteAuditRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	filtered := make([]storage.RemoteAuditRecord, 0)
	for i := len(m.records) - 1; i >= 0; i-- { // 최신순.
		r := m.records[i]
		if instanceID == "" || r.InstanceID == instanceID {
			filtered = append(filtered, r)
		}
	}
	if offset > len(filtered) {
		return nil, nil
	}
	filtered = filtered[offset:]
	if limit > 0 && limit < len(filtered) {
		filtered = filtered[:limit]
	}
	return filtered, nil
}

func (m *memAudit) Close() error { return nil }

func (m *memAudit) all() []storage.RemoteAuditRecord {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]storage.RemoteAuditRecord(nil), m.records...)
}

// adminAuditRequest 는 admin role + user_id 가 주입된 요청을 만든다.
func adminAuditRequest(t *testing.T, audit storage.RemoteAuditRepository, svc NodeAdminService) func(method, target, actor string, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteAdminHandler(svc, nil).WithAudit(audit)
	return func(method, target, actor, body string) *httptest.ResponseRecorder {
		router := api.NewRouter()
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, target, strings.NewReader(body))
		} else {
			req = httptest.NewRequest(method, target, nil)
		}
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), "admin")
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), actor)
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		h.RegisterRoutes(router.Group("/api/v1"))
		router.Handler().ServeHTTP(rec, req)
		return rec
	}
}

// TestAudit_ApproveRejectRevokeRecorded 는 승인/거부/폐기 시 감사 레코드가 actor 와
// 함께 기록되는지 검증한다(REQ-F05).
func TestAudit_ApproveRejectRevokeRecorded(t *testing.T) {
	audit := &memAudit{}
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1"}
	svc.nodes["n2"] = storage.ManagedNode{InstanceID: "n2"}
	svc.nodes["n3"] = storage.ManagedNode{InstanceID: "n3"}
	do := adminAuditRequest(t, audit, svc)

	require.Equal(t, http.StatusOK, do(http.MethodPost, "/api/v1/remote/nodes/n1/approve", "alice", "").Code)
	require.Equal(t, http.StatusOK, do(http.MethodPost, "/api/v1/remote/nodes/n2/reject", "alice", "").Code)
	require.Equal(t, http.StatusOK, do(http.MethodPost, "/api/v1/remote/nodes/n3/revoke", "bob", "").Code)

	recs := audit.all()
	require.Len(t, recs, 3)
	byAction := map[string]storage.RemoteAuditRecord{}
	for _, r := range recs {
		byAction[r.Action] = r
	}
	assert.Equal(t, "n1", byAction[storage.AuditActionApprove].InstanceID)
	assert.Equal(t, "alice", byAction[storage.AuditActionApprove].Actor)
	assert.Equal(t, storage.AuditResultOK, byAction[storage.AuditActionApprove].Result)
	assert.Equal(t, "bob", byAction[storage.AuditActionRevoke].Actor)
}

// TestAudit_ListEndpoint 는 GET /remote/audit 가 감사 레코드를 admin 에게 반환하고
// instance_id 필터를 적용하는지 검증한다(REQ-F05 관측성).
func TestAudit_ListEndpoint(t *testing.T) {
	audit := &memAudit{}
	svc := newFakeNodeAdmin()
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1"}
	svc.nodes["n2"] = storage.ManagedNode{InstanceID: "n2"}
	do := adminAuditRequest(t, audit, svc)
	do(http.MethodPost, "/api/v1/remote/nodes/n1/approve", "alice", "")
	do(http.MethodPost, "/api/v1/remote/nodes/n2/approve", "alice", "")

	rec := do(http.MethodGet, "/api/v1/remote/audit", "alice", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Len(t, resp.Data, 2)

	// instance_id 필터.
	rec2 := do(http.MethodGet, "/api/v1/remote/audit?instance_id=n1", "alice", "")
	require.Equal(t, http.StatusOK, rec2.Code)
	var resp2 struct {
		Data []map[string]any `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp2))
	require.Len(t, resp2.Data, 1)
	assert.Equal(t, "n1", resp2.Data[0]["instance_id"])
}

// TestAudit_ListRequiresAdmin 은 비-admin 의 감사 조회가 403 인지 검증한다(REQ-F04).
func TestAudit_ListRequiresAdmin(t *testing.T) {
	audit := &memAudit{}
	h := NewRemoteAdminHandler(newFakeNodeAdmin(), nil).WithAudit(audit)
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/remote/audit", nil)
	req = req.WithContext(context.WithValue(req.Context(), api.ContextKeyUserRole(), "viewer"))
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestAudit_CommandSetsActorContext 는 명령 핸들러가 actor 를 컨텍스트에 실어
// Dispatch 로 전달하는지 검증한다(서버 측 명령 감사 actor 출처).
func TestAudit_CommandSetsActorContext(t *testing.T) {
	audit := &memAudit{}
	svc := &actorCapturingAdmin{fakeNodeAdmin: newFakeNodeAdmin()}
	svc.nodes["n1"] = storage.ManagedNode{InstanceID: "n1"}
	do := adminAuditRequest(t, audit, svc)

	rec := do(http.MethodPost, "/api/v1/remote/nodes/n1/command", "carol", `{"domain":"flow","action":"deploy"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "carol", svc.capturedActor, "Dispatch 컨텍스트에 actor 가 전파되어야 함")
}

// actorCapturingAdmin 은 Dispatch 컨텍스트의 actor 를 캡처한다.
type actorCapturingAdmin struct {
	*fakeNodeAdmin
	capturedActor string
}

func (a *actorCapturingAdmin) Dispatch(ctx context.Context, instanceID, domain, action string, args json.RawMessage) (json.RawMessage, error) {
	a.capturedActor = remote.ActorFromContext(ctx)
	return a.fakeNodeAdmin.Dispatch(ctx, instanceID, domain, action, args)
}
