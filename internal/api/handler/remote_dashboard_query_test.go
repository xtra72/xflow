// remote_dashboard_query_test.go 는 M10(그룹 L) 원격 대시보드 config + 메트릭 READ
// 프록시 REST API 의 게이팅·매핑·실패 의미를 검증한다
// (@SPEC:SPEC-REMOTE-001 M10, REQ-L01/L02/L05).
//
// 대시보드 config·메트릭은 노드-레벨 자원(per-resource 노출 범위 없음 — REQ-L03)이므로
// 게이팅은 IsManaged(승인+온라인)만 평가하고 IsResourceExposed 는 호출하지 않는다.
// 실패 의미는 그룹 J 와 동일(503/504/502 — mapRemoteQueryError 재사용).
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
)

// doNodeQuery 는 fakeQuerySvc(IsManaged 포함)로 노드-레벨 GET 질의를 보낸다.
func doNodeQuery(t *testing.T, svc RemoteQueryService, role, target string) *httptest.ResponseRecorder {
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

// TestDashboardQuery_SharedHappyPath 는 GET .../dashboards/shared 가 dashboard/get_shared
// 로 매핑되고 노드 config 를 통과시키는지 검증한다(REQ-L01). 노출 범위 게이트 미호출.
func TestDashboardQuery_SharedHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.data = []byte(`{"scope":"global","payload":{"dashboardPages":[]}}`)

	rec := doNodeQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/dashboards/shared")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"scope":"global"`)
	assert.Equal(t, []string{"dashboard/get_shared/"}, svc.dispatched)
	assert.Empty(t, svc.scope, "노드-레벨 query 는 노출 범위를 평가하지 않아야 함")
}

// TestDashboardQuery_MineHappyPath 는 GET .../dashboards/mine 가 dashboard/get_mine 로
// 매핑되고 viewing admin 을 owner 로 전달하는지 검증한다(REQ-L01/A17 — 노드-로컬 사용자).
func TestDashboardQuery_MineHappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.data = []byte(`{"scope":"user"}`)

	rec := doNodeQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/dashboards/mine")
	require.Equal(t, http.StatusOK, rec.Code)
	require.Len(t, svc.dispatched, 1)
	assert.Contains(t, svc.dispatched[0], "dashboard/get_mine")
	// owner 는 viewing admin(admin-user)로 전달되어야 한다(노드-로컬 사용자 스코프).
	assert.Equal(t, "admin-user", svc.lastOwner)
}

// TestDashboardQuery_NotManaged503 은 미관리 노드(오프라인/미승인)가 503 으로 매핑되는지
// 검증한다(REQ-L02/J05 — 게이팅).
func TestDashboardQuery_NotManaged503(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = false // 미관리.

	rec := doNodeQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/dashboards/shared")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Empty(t, svc.dispatched, "미관리 노드에는 디스패치하지 않아야 함")
}

// TestDashboardQuery_NotAdmin403 은 비-admin 접근이 거부되는지 검증한다(REQ-F04).
func TestDashboardQuery_NotAdmin403(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true

	rec := doNodeQuery(t, svc, "node", "/api/v1/remote/nodes/n1/dashboards/shared")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestDashboardQuery_NodeError502 은 노드 측 오류(config 미설정 등)가 502 로 매핑되는지
// 검증한다(REQ-L02 — node-error).
func TestDashboardQuery_NodeError502(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.err = remote.ErrQueryFailed

	rec := doNodeQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/dashboards/shared")
	assert.Equal(t, http.StatusBadGateway, rec.Code)
}

// TestDashboardQuery_Timeout504 은 타임아웃이 504 로 매핑되는지 검증한다(REQ-L02).
func TestDashboardQuery_Timeout504(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.err = remote.ErrQueryTimeout

	rec := doNodeQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/dashboards/shared")
	assert.Equal(t, http.StatusGatewayTimeout, rec.Code)
}

// TestMetricsQuery_HappyPath 는 GET .../metrics 가 monitor/metrics 로 매핑되는지
// 검증한다(REQ-L05). 노출 범위 게이트 미호출(노드-레벨).
func TestMetricsQuery_HappyPath(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = true
	svc.data = []byte(`{"memory_usage_percent":42}`)

	rec := doNodeQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/metrics")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"memory_usage_percent":42`)
	assert.Equal(t, []string{"monitor/metrics/"}, svc.dispatched)
	assert.Empty(t, svc.scope)
}

// TestMetricsQuery_NotManaged503 은 미관리 노드의 메트릭 질의가 503 인지 검증한다(REQ-L02).
func TestMetricsQuery_NotManaged503(t *testing.T) {
	svc := newFakeQuerySvc()
	svc.managed = false

	rec := doNodeQuery(t, svc, "admin", "/api/v1/remote/nodes/n1/metrics")
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}
