// @SPEC:SPEC-AUTH-005 (M1)
// catalog_test.go — 권한 카탈로그 / 빌트인 역할 / 역할 이름 규칙 검증.

package rbac

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPermissions_MatchesSpecTable 은 카탈로그가 spec.md §2.1 표와 정확히
// 일치하는지 전수 대조한다. 표에 없는 키가 추가되거나 표의 키가 누락되면 실패한다.
func TestPermissions_MatchesSpecTable(t *testing.T) {
	want := []string{
		"agent.read", "agent.create", "agent.update", "agent.delete", "agent.execute",
		"device.read", "device.create", "device.update", "device.delete", "device.execute",
		"flow.read", "flow.create", "flow.update", "flow.delete", "flow.execute",
		"dashboard.read", "dashboard.create", "dashboard.update", "dashboard.delete",
		"node.read",
		"schedule.read", "schedule.create", "schedule.update", "schedule.delete",
		"monitoring.read",
		"store.read", "store.update",
		"remote.read", "remote.update",
		"system.read", "system.update",
		"user.read", "user.create", "user.update", "user.delete",
		"role.read", "role.create", "role.update", "role.delete",
		// SPEC-AUTH-006 E2: 메뉴 노출 축. 데이터 접근과 분리된 키다 —
		// 대시보드 패널이 agent/device/flow 를 읽으므로 read 는 줘야 하는데,
		// 같은 키로 메뉴까지 판정하면 "대시보드만 보이는 역할" 이 불가능해진다.
		// nav.dashboard 는 '대시보드 관리' 메뉴 전용이다(SPEC-DASHBOARD-004 §2.5).
		// 대시보드를 보는 사이드바 항목은 여전히 무권한이다.
		"nav.flow", "nav.agent", "nav.device", "nav.monitoring",
		"nav.schedule", "nav.node", "nav.remote", "nav.system",
		"nav.user", "nav.role", "nav.dashboard",
	}
	sort.Strings(want)

	assert.Equal(t, want, Permissions(), "카탈로그가 spec.md §2.1 표와 일치해야 한다")
}

// TestPermissions_IsSortedAndCopied 는 반환 슬라이스가 결정적으로 정렬되어 있고
// 호출자 변형이 카탈로그를 오염시키지 않음을 검증한다.
func TestPermissions_IsSortedAndCopied(t *testing.T) {
	got := Permissions()
	require.True(t, sort.StringsAreSorted(got), "권한 목록은 사전순 정렬이어야 한다")

	got[0] = "tampered"
	assert.NotEqual(t, "tampered", Permissions()[0], "반환값 변형이 카탈로그에 반영되면 안 된다")
}

// TestIsValidPermission 은 카탈로그 밖의 키가 거부되는지 검증한다 (AC-03).
func TestIsValidPermission(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"agent.read", true},
		{"role.delete", true},
		{"agent.launch", false}, // acceptance.md AC-03 의 미정의 키
		{"agent.*", false},
		{"", false},
		{"agent", false},
		{"AGENT.READ", false},
		{"node.create", false}, // node 는 read 만 정의됨
		{"dashboard.delete", true},
		{"nav.dashboard", true},
		{"dashboard.execute", false}, // dashboard 는 execute 를 정의하지 않는다
		{"monitoring.update", false},
	}
	for _, tc := range tests {
		assert.Equalf(t, tc.want, IsValidPermission(tc.key), "IsValidPermission(%q)", tc.key)
	}
}

// TestBuiltinRoles_AdminEqualsFullCatalog 는 admin 이 카탈로그 전체 권한을
// 보유하는지 검증한다 (AC-01).
func TestBuiltinRoles_AdminEqualsFullCatalog(t *testing.T) {
	roles := BuiltinRoles()
	require.Len(t, roles, 3, "빌트인 역할은 3종이어야 한다")
	require.Equal(t, RoleAdmin, roles[0].Name)

	assert.Equal(t, Permissions(), roles[0].Permissions, "admin 은 카탈로그 전체와 동일해야 한다")
}

// TestBuiltinRoles_Order 는 시드 순서가 admin → editor → viewer 로 결정적인지 검증한다.
func TestBuiltinRoles_Order(t *testing.T) {
	roles := BuiltinRoles()
	assert.Equal(t, []string{RoleAdmin, RoleEditor, RoleViewer},
		[]string{roles[0].Name, roles[1].Name, roles[2].Name})
	for _, r := range roles {
		assert.NotEmptyf(t, r.Description, "역할 %q 는 설명을 가져야 한다", r.Name)
	}
}

// TestBuiltinRoles_Viewer 는 viewer 가 read 전용이며 관리 리소스(user/role) 는
// read 조차 보유하지 않음을 검증한다 (AC-07: viewer 의 GET /users → 403).
func TestBuiltinRoles_Viewer(t *testing.T) {
	viewer := builtinByName(t, RoleViewer)

	for _, p := range viewer {
		isNav := strings.HasPrefix(p, ResourceNav+".")
		assert.Truef(t, isNav || strings.HasSuffix(p, "."+ActionRead),
			"viewer 는 read 와 메뉴 키만 보유해야 한다: %q", p)
		assert.NotContainsf(t, []string{"user.read", "role.read", "nav.user", "nav.role"}, p,
			"viewer 는 관리 리소스의 read·메뉴를 보유하면 안 된다: %q", p)
	}

	assert.Contains(t, viewer, "agent.read")
	assert.Contains(t, viewer, "system.read")
	assert.NotContains(t, viewer, "user.read")
	assert.NotContains(t, viewer, "role.read")

	// 메뉴 축: 읽을 수 있는 리소스의 메뉴는 기본 부여, 관리 메뉴는 제외.
	assert.Contains(t, viewer, "nav.agent")
	assert.NotContains(t, viewer, "nav.user")
	assert.NotContains(t, viewer, "nav.role")

	// 메뉴 11종 중 viewer 는 관리 메뉴(user/role) 와 대시보드 관리 메뉴를 제외한 8종.
	assert.Len(t, viewer, 18, "viewer = 관리 제외 전 리소스 read(10) + 메뉴(8)")
}

// TestBuiltinRoles_Editor 는 editor 권한 집합을 spec.md §2.1 editor 행과 대조한다.
func TestBuiltinRoles_Editor(t *testing.T) {
	editor := builtinByName(t, RoleEditor)
	viewer := builtinByName(t, RoleViewer)

	// viewer 의 모든 권한을 포함한다.
	for _, p := range viewer {
		assert.Containsf(t, editor, p, "editor 는 viewer 권한 %q 를 포함해야 한다", p)
	}

	// 쓰기 권한 (delete 제외).
	for _, p := range []string{
		"agent.create", "agent.update", "agent.execute",
		"device.create", "device.update", "device.execute",
		"flow.create", "flow.update", "flow.execute",
		"schedule.create", "schedule.update",
		"dashboard.update", "store.update",
	} {
		assert.Containsf(t, editor, p, "editor 는 %q 를 보유해야 한다", p)
	}

	// delete 는 어떤 리소스에서도 보유하지 않는다.
	for _, p := range editor {
		assert.Falsef(t, strings.HasSuffix(p, "."+ActionDelete), "editor 는 delete 를 보유하면 안 된다: %q", p)
	}

	// user/role 관리 및 system.update 는 제외.
	for _, p := range []string{
		"user.read", "user.create", "user.update", "user.delete",
		"role.read", "role.create", "role.update", "role.delete",
		"system.update", "remote.update",
	} {
		assert.NotContainsf(t, editor, p, "editor 는 %q 를 보유하면 안 된다", p)
	}

	assert.True(t, sort.StringsAreSorted(editor), "editor 권한은 정렬되어야 한다")
}

// TestBuiltinRoles_PermissionsAreValid 는 시드되는 모든 권한이 카탈로그에
// 존재하는지 검증한다 (역할 정의와 카탈로그의 drift 방지).
func TestBuiltinRoles_PermissionsAreValid(t *testing.T) {
	for _, r := range BuiltinRoles() {
		for _, p := range r.Permissions {
			assert.Truef(t, IsValidPermission(p), "역할 %q 의 권한 %q 가 카탈로그에 없다", r.Name, p)
		}
	}
}

// TestBuiltinRoles_ReturnsIndependentCopy 는 호출자의 변형이 다음 호출에 영향을
// 주지 않음을 검증한다 (시드 반복 호출 안전성).
func TestBuiltinRoles_ReturnsIndependentCopy(t *testing.T) {
	first := BuiltinRoles()
	first[0].Permissions[0] = "tampered"
	assert.NotEqual(t, "tampered", BuiltinRoles()[0].Permissions[0])
}

// TestIsBuiltinRole 은 빌트인 판정을 검증한다 (UB1 불변식 3, 4 에서 사용).
func TestIsBuiltinRole(t *testing.T) {
	assert.True(t, IsBuiltinRole(RoleAdmin))
	assert.True(t, IsBuiltinRole(RoleEditor))
	assert.True(t, IsBuiltinRole(RoleViewer))
	assert.False(t, IsBuiltinRole("operator"))
	assert.False(t, IsBuiltinRole(""))
}

// TestIsValidRoleName 은 역할 이름 규칙 (소문자·숫자·하이픈, 1~32자) 을 검증한다.
func TestIsValidRoleName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"operator", true},
		{"op-1", true},
		{"a", true},
		{"0", true},
		{"-", true},
		{strings.Repeat("a", 32), true},
		{strings.Repeat("a", 33), false},
		{"", false},
		{"Operator", false},  // 대문자
		{"op erator", false}, // 공백
		{"op_erator", false}, // 언더스코어
		{"오퍼레이터", false},     // 비ASCII
		{"op.erator", false},
	}
	for _, tc := range tests {
		assert.Equalf(t, tc.want, IsValidRoleName(tc.name), "IsValidRoleName(%q)", tc.name)
	}
}

// builtinByName 은 빌트인 역할의 권한 목록을 이름으로 조회한다.
func builtinByName(t *testing.T, name string) []string {
	t.Helper()
	for _, r := range BuiltinRoles() {
		if r.Name == name {
			return r.Permissions
		}
	}
	t.Fatalf("빌트인 역할 %q 를 찾을 수 없다", name)
	return nil
}

// TestBuiltinRoles_DashboardPermissions 는 대시보드 엔티티 도입으로 추가된 키 3종의
// 빌트인 역할 부여를 spec.md §2.5 표와 대조한다 (acceptance.md AC-09).
//
// admin=전량, editor=create+nav(삭제 제외), viewer=미부여. viewer 가 nav.dashboard 를
// 얻으면 아무것도 할 수 없는 빈 관리 화면만 보게 되므로 파생에서 제외된다.
func TestBuiltinRoles_DashboardPermissions(t *testing.T) {
	tests := []struct {
		role       string
		wantHave   []string
		wantAbsent []string
	}{
		{
			role:     RoleAdmin,
			wantHave: []string{"dashboard.create", "dashboard.delete", "nav.dashboard"},
		},
		{
			role:       RoleEditor,
			wantHave:   []string{"dashboard.create", "nav.dashboard"},
			wantAbsent: []string{"dashboard.delete"},
		},
		{
			role:       RoleViewer,
			wantAbsent: []string{"dashboard.create", "dashboard.delete", "nav.dashboard"},
		},
	}
	for _, tc := range tests {
		perms := builtinByName(t, tc.role)
		for _, p := range tc.wantHave {
			assert.Containsf(t, perms, p, "%s 는 %q 를 보유해야 한다", tc.role, p)
		}
		for _, p := range tc.wantAbsent {
			assert.NotContainsf(t, perms, p, "%s 는 %q 를 보유하면 안 된다", tc.role, p)
		}
	}
}

// TestPermissions_ContainsDashboardEntityKeys 는 카탈로그에 신규 키 3종이 존재하는지
// 직접 확인한다. 이 키들이 없으면 이후 마일스톤의 라우트 권한 부착이 카탈로그
// 검증에서 거부된다 (plan.md M1 배치 근거).
func TestPermissions_ContainsDashboardEntityKeys(t *testing.T) {
	all := Permissions()
	for _, key := range []string{"dashboard.create", "dashboard.delete", "nav.dashboard"} {
		assert.Containsf(t, all, key, "카탈로그에 %q 가 없다", key)
		assert.Truef(t, IsValidPermission(key), "IsValidPermission(%q) 가 false 다", key)
	}
}

// TestBuiltinRoles_ViewerExcludesDashboardNavOnly 는 viewer 의 nav 제외가
// 대시보드 관리 메뉴에만 적용되고 다른 메뉴 파생은 그대로임을 고정한다.
// 제외 규칙이 넓어지면 viewer 의 사이드바가 조용히 비어 간다.
func TestBuiltinRoles_ViewerExcludesDashboardNavOnly(t *testing.T) {
	viewer := builtinByName(t, RoleViewer)

	assert.NotContains(t, viewer, "nav.dashboard")
	// 대시보드 조회 자체는 유지된다 — 데이터 축과 메뉴 축은 별개다.
	assert.Contains(t, viewer, "dashboard.read")

	for _, key := range NavPermissions() {
		menu := strings.TrimPrefix(key, ResourceNav+".")
		if menu == ResourceUser || menu == ResourceRole || menu == ResourceDashboard {
			continue
		}
		assert.Containsf(t, viewer, key, "viewer 가 메뉴 %q 를 잃었다", key)
	}
}
