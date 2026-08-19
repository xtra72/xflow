package handler

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/rbac"
)

// @SPEC:SPEC-AUTH-005 (M5, acceptance.md AC-08) — 보호 대상 라우트 커버리지.
//
// 라우트 부착 누락은 특정 API 가 무방비로 남는 보안 구멍이다(plan.md §3 위험 분석).
// 본 테스트는 운영과 동일한 방식으로 모든 핸들러의 RegisterRoutes 를 실행한 뒤,
// 등록된 전 라우트가 (a) 권한 키를 보유하거나 (b) 명시적 allowlist 에 사유와 함께
// 등재되어 있는지 전수 대조한다. 새 라우트를 권한 지정 없이 추가하면 실패한다.

// routeKey 는 allowlist 대조용 "METHOD PATH" 키이다.
func routeKey(method, pattern string) string { return method + " " + pattern }

// permissionAllowlist 는 권한 미부착이 의도된 라우트와 그 사유이다.
//
// 여기에 등재하지 않은 권한 미부착 라우트는 테스트 실패로 드러난다. 새 라우트를
// 무심코 등재하지 않도록, 각 항목은 "왜 권한이 필요 없는가" 를 한 줄로 남긴다.
var permissionAllowlist = map[string]string{
	// 인증 자체를 수행하는 경로 — 토큰이 없는 상태에서 호출되므로 권한 검사 대상이 아니다.
	// (api.Auth 의 exemptPaths 와 대응한다.)
	"POST /api/v1/auth/login":   "인증 전 호출 — Auth 미들웨어 면제 경로",
	"POST /api/v1/auth/refresh": "인증 전 호출 — Auth 미들웨어 면제 경로",
	"GET /api/v1/auth/status":   "인증 활성화 여부 조회 — 로그인 화면 렌더링에 필요, Auth 면제 경로",

	// 인증만 요구하는 self-service 경로 — 본인 세션·본인 계정에만 작용한다.
	"POST /api/v1/auth/logout": "본인 토큰 무효화 — 인증만 요구",
	"GET /api/v1/auth/me":      "본인 정보 조회 — 인증만 요구",
	"PUT /api/v1/auth/password": "본인 비밀번호 변경 — 현재 비밀번호를 검증하며 " +
		"타인 계정에는 작용하지 않는다 (관리자 재설정은 user.update 를 요구하는 별도 경로)",

	// 권한 카탈로그는 코드 상수이므로 비밀이 아니며, 역할 편집 UI 렌더링에 필요하다.
	"GET /api/v1/permissions": "권한 키 카탈로그 — spec.md §2.2 에서 '인증만' 으로 명시",

	// 본인 소유 대시보드 — 핸들러가 owner 를 ctx.UserID() 로 고정하므로 타 사용자
	// 데이터에 접근할 수 없다.
	//
	// @SPEC:SPEC-DASHBOARD-004 (M4, spec.md §2.3, acceptance.md AC-13)
	// PUT · DELETE /dashboards/mine 은 라우트가 제거되었으므로 항목도 함께 사라졌다
	// (묶음 단위 쓰기는 낙관적 동시성이 성립하지 않는다). GET 만 읽기 전용 shim 으로
	// 남으며, 권한 미부착 사유는 그대로다.
	"GET /api/v1/dashboards/mine": "본인 소유 리소스 — 핸들러가 owner 를 세션 사용자로 고정",

	// @SPEC:SPEC-DASHBOARD-004 (M4, acceptance.md AC-13)
	"GET /api/v1/dashboard-state": "본인 소유 UI 상태 — 핸들러가 username 을 세션 사용자로 고정",
	"PUT /api/v1/dashboard-state": "본인 소유 UI 상태 — 핸들러가 username 을 세션 사용자로 고정",
}

// buildProductionRoutes 는 cmd/xflowd/main.go 가 등록하는 모든 RouteGroup 핸들러를
// 동일한 순서·조건으로 등록한 라우터를 만든다.
//
// 핸들러 구조체는 zero value 로 만든다. RegisterRoutes 는 라우트 등록만 수행하고
// 의존성을 역참조하지 않으므로 안전하다. 조건부 라우트(flow tap, device history)는
// 미부착이 은폐되지 않도록 해당 필드를 비-nil 로 채워 전부 등록시킨다.
//
// RegisterRawHandler 로 등록되는 raw 핸들러(WebSocket, SSE, 릴리스 피드/업로드)는
// Context 래퍼와 미들웨어 체인을 통째로 우회하며 자체적으로 JWT 를 검증하므로
// RouteGroup 커버리지 대상이 아니다.
func buildProductionRoutes(t *testing.T) []api.RouteRecord {
	t.Helper()

	router := api.NewRouter()
	g := router.Group("/api/v1")

	// 인증 상태 + 인증 핸들러 (main.go 9.4).
	RegisterAuthStatusRoute(g, true)
	(&AuthHandler{}).RegisterRoutes(g)

	// 사용자·역할 관리 (main.go 9.4, @SPEC:SPEC-AUTH-005 M6).
	(&UserHandler{}).RegisterRoutes(g)
	(&RoleHandler{}).RegisterRoutes(g)

	// 코어 리소스.
	flowH := &FlowHandler{}
	flowH.taps = stubTapController{} // 조건부 tap 라우트까지 등록시킨다
	flowH.RegisterRoutes(g)

	(&AgentHandler{}).RegisterRoutes(g)
	(&NodeHandler{}).RegisterRoutes(g)
	(&MonitorHandler{}).RegisterRoutes(g)

	deviceH := &DeviceHandler{}
	deviceH.history = stubDeviceHistoryProvider{} // 조건부 history 라우트까지 등록시킨다
	deviceH.RegisterRoutes(g)

	// 차트 / 스토어 / InfluxDB.
	(&ChartHandler{}).RegisterRoutes(g)
	(&StoreQueryHandler{}).RegisterRoutes(g)
	(&InfluxDBQueryHandler{}).RegisterRoutes(g)
	(&InfluxDBManagementHandler{}).RegisterRoutes(g)

	// 시스템 / 설정 / 대시보드.
	(&SystemHandler{}).RegisterRoutes(g)
	(&RemoteConfigHandler{}).RegisterRoutes(g)
	(&ScheduleLogConfigHandler{}).RegisterRoutes(g)
	(&DashboardHandler{}).RegisterRoutes(g)
	(&DashboardAssetHandler{}).RegisterRoutes(g)
	(&SettingsHandler{}).RegisterRoutes(g)
	(&TSDBHandler{}).RegisterRoutes(g)

	// 조건부 등록(원격 server 모드 등, main.go 9.5b~9.5d). 커버리지 검사에서는
	// 누락이 은폐되지 않도록 전부 등록한다.
	(&RemoteAdminHandler{}).RegisterRoutes(g)
	(&ScheduleLogHandler{}).RegisterRoutes(g)
	(&RemoteEnrollmentHandler{}).RegisterRoutes(g)
	(&RemoteEditHandler{}).RegisterRoutes(g)
	(&RemoteQueryHandler{}).RegisterRoutes(g)
	(&RemoteGroupingHandler{}).RegisterRoutes(g)
	(&ReleaseAdminHandler{}).RegisterRoutes(g)
	(&RemoteModeHandler{}).RegisterRoutes(g)

	routes := router.Routes()
	require.NotEmpty(t, routes, "라우트가 하나도 등록되지 않았다")
	return routes
}

// TestRoutePermissionCoverage 는 권한 부착 누락이 0 건임을 검증한다 (AC-08).
func TestRoutePermissionCoverage(t *testing.T) {
	routes := buildProductionRoutes(t)

	var missing []string
	for _, r := range routes {
		if r.Permission != "" {
			continue
		}
		key := routeKey(r.Method, r.Pattern)
		if _, ok := permissionAllowlist[key]; !ok {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)

	assert.Empty(t, missing,
		"권한 미부착 라우트가 있다. 권한을 부착하거나(*Perm 계열 등록) "+
			"permissionAllowlist 에 사유와 함께 등재하라:\n%v", missing)
}

// TestRoutePermissionKeysAreInCatalog 는 부착된 권한 키가 모두 카탈로그에 정의되어
// 있는지 검증한다 (오타로 인한 "영구 거부" 라우트 방지).
func TestRoutePermissionKeysAreInCatalog(t *testing.T) {
	routes := buildProductionRoutes(t)

	var unknown []string
	for _, r := range routes {
		if r.Permission == "" {
			continue
		}
		if !rbac.IsValidPermission(r.Permission) {
			unknown = append(unknown, routeKey(r.Method, r.Pattern)+" -> "+r.Permission)
		}
	}
	sort.Strings(unknown)

	assert.Empty(t, unknown, "카탈로그에 없는 권한 키가 부착되었다:\n%v", unknown)
}

// TestPermissionAllowlistHasNoStaleEntries 는 allowlist 가 실제 라우트와 대응하는지
// 검증한다. 라우트가 사라지거나 권한이 부착되면 해당 allowlist 항목은 제거되어야 한다.
func TestPermissionAllowlistHasNoStaleEntries(t *testing.T) {
	routes := buildProductionRoutes(t)

	unattached := make(map[string]bool, len(routes))
	for _, r := range routes {
		if r.Permission == "" {
			unattached[routeKey(r.Method, r.Pattern)] = true
		}
	}

	var stale []string
	for key := range permissionAllowlist {
		if !unattached[key] {
			stale = append(stale, key)
		}
	}
	sort.Strings(stale)

	assert.Empty(t, stale,
		"allowlist 에 남아있으나 더 이상 권한 미부착이 아닌(또는 존재하지 않는) 라우트:\n%v", stale)
}

// TestAgentRoutesPermissionMapping 은 SPEC 이 직접 명시한 agents 매핑을 고정한다
// (plan.md §M5 매핑 원칙 표).
func TestAgentRoutesPermissionMapping(t *testing.T) {
	routes := buildProductionRoutes(t)
	got := make(map[string]string, len(routes))
	for _, r := range routes {
		got[routeKey(r.Method, r.Pattern)] = r.Permission
	}

	want := map[string]string{
		"GET /api/v1/agents":                "agent.read",
		"GET /api/v1/agents/export":         "agent.read",
		"GET /api/v1/agents/{id}":           "agent.read",
		"GET /api/v1/agents/{id}/export":    "agent.read",
		"GET /api/v1/agents/{id}/stats":     "agent.read",
		"POST /api/v1/agents":               "agent.create",
		"PUT /api/v1/agents/{id}":           "agent.update",
		"PUT /api/v1/agents/{id}/config":    "agent.update",
		"DELETE /api/v1/agents/{id}":        "agent.delete",
		"POST /api/v1/agents/{id}/start":    "agent.execute",
		"POST /api/v1/agents/{id}/stop":     "agent.execute",
		"POST /api/v1/agents/{id}/restart":  "agent.execute",
		"POST /api/v1/agents/{id}/enable":   "agent.execute",
		"POST /api/v1/agents/{id}/disable":  "agent.execute",
		"POST /api/v1/agents/{id}/exec":     "agent.execute",
		"GET /api/v1/flows":                 "flow.read",
		"POST /api/v1/flows":                "flow.create",
		"PUT /api/v1/flows/{id}":            "flow.update",
		"DELETE /api/v1/flows/{id}":         "flow.delete",
		"POST /api/v1/flows/{id}/deploy":    "flow.execute",
		"GET /api/v1/devices":               "device.read",
		"POST /api/v1/devices/{id}/execute": "device.execute",
		"PUT /api/v1/devices/{id}/metadata": "device.update",
		"GET /api/v1/users":                 "user.read",
		"POST /api/v1/users":                "user.create",
		"PUT /api/v1/users/{username}":      "user.update",
		"DELETE /api/v1/users/{username}":   "user.delete",
		"GET /api/v1/roles":                 "role.read",
		"POST /api/v1/roles":                "role.create",
		"PUT /api/v1/roles/{name}":          "role.update",
		"DELETE /api/v1/roles/{name}":       "role.delete",
	}
	for key, perm := range want {
		assert.Equal(t, perm, got[key], "라우트 %s 의 권한 매핑", key)
	}
}

// --- 조건부 라우트를 등록시키기 위한 최소 스텁 ---

type stubTapController struct{}

func (stubTapController) SetTap(flowID, nodeID string, enabled bool) {}
func (stubTapController) IsTapped(flowID, nodeID string) bool        { return false }
func (stubTapController) TappedNodes(flowID string) []string         { return nil }

type stubDeviceHistoryProvider struct{}

func (stubDeviceHistoryProvider) History(deviceID string, limit int) []device.HistorySnapshot {
	return nil
}
func (stubDeviceHistoryProvider) MaxEntries() int { return 0 }
