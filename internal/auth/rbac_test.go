// @SPEC:SPEC-AUTH-005 (M1)
// rbac_test.go — auth 패키지 facade 가 leaf 카탈로그와 동일한 값을 노출하는지 검증.
//
// 카탈로그 자체의 상세 검증은 internal/rbac/catalog_test.go 가 담당한다.
// 본 테스트는 facade 가 정의를 중복하지 않고 위임하고 있음을 고정한다.

package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/rbac"
)

func TestRBACFacade_DelegatesToLeafPackage(t *testing.T) {
	assert.Equal(t, rbac.Permissions(), Permissions())
	assert.Equal(t, rbac.BuiltinRoles(), BuiltinRoles())

	assert.True(t, IsValidPermission("agent.read"))
	assert.False(t, IsValidPermission("agent.launch"))

	assert.True(t, IsBuiltinRole(RoleAdmin))
	assert.False(t, IsBuiltinRole("operator"))

	assert.True(t, IsValidRoleName("operator"))
	assert.False(t, IsValidRoleName("Operator"))
}

// TestRBACFacade_RoleNamesMatchExistingUserRoles 는 빌트인 역할 이름이 기존
// users.role 값과 정확히 일치함을 고정한다 (사용자 데이터 마이그레이션 불필요 근거).
func TestRBACFacade_RoleNamesMatchExistingUserRoles(t *testing.T) {
	roles := BuiltinRoles()
	require.Len(t, roles, 3)
	assert.Equal(t, []string{"admin", "editor", "viewer"},
		[]string{roles[0].Name, roles[1].Name, roles[2].Name})
	assert.Equal(t, "admin", RoleAdmin)
	assert.Equal(t, "editor", RoleEditor)
	assert.Equal(t, "viewer", RoleViewer)
}
