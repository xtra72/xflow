package auth

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/internal/storage"
)

// @SPEC:SPEC-AUTH-005 (M4) — SQLPermissionResolver 통합 테스트.

func TestSQLPermissionResolver(t *testing.T) {
	ctx := context.Background()
	db, err := storage.OpenSQLiteDB(ctx, filepath.Join(t.TempDir(), "resolver.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	resolver := NewSQLPermissionResolver(db)

	t.Run("빌트인 역할의 권한을 조회한다", func(t *testing.T) {
		perms, err := resolver.RolePermissions(ctx, rbac.RoleAdmin)
		require.NoError(t, err)
		assert.ElementsMatch(t, rbac.Permissions(), perms)

		viewerPerms, err := resolver.RolePermissions(ctx, rbac.RoleViewer)
		require.NoError(t, err)
		assert.Contains(t, viewerPerms, "agent.read")
		assert.NotContains(t, viewerPerms, "agent.delete")
		// 관리 리소스는 read 도 제외된다 (AC-07: viewer 의 GET /users → 403).
		assert.NotContains(t, viewerPerms, "user.read")
		assert.NotContains(t, viewerPerms, "role.read")
	})

	t.Run("없는 역할은 ErrRoleNotFound", func(t *testing.T) {
		_, err := resolver.RolePermissions(ctx, "ghost")
		assert.ErrorIs(t, err, ErrRoleNotFound)
	})

	t.Run("사용자의 현재 역할을 조회한다", func(t *testing.T) {
		hash, err := HashPassword("kimpassword")
		require.NoError(t, err)
		require.NoError(t, storage.InsertUser(ctx, db, "kim", hash, rbac.RoleViewer, 0, 0))

		role, err := resolver.UserRole(ctx, "kim")
		require.NoError(t, err)
		assert.Equal(t, rbac.RoleViewer, role)

		require.NoError(t, storage.UpdateUserRole(ctx, db, "kim", rbac.RoleEditor))
		role, err = resolver.UserRole(ctx, "kim")
		require.NoError(t, err)
		assert.Equal(t, rbac.RoleEditor, role)
	})

	t.Run("없는 사용자는 ErrUserNotFound", func(t *testing.T) {
		_, err := resolver.UserRole(ctx, "nobody")
		assert.ErrorIs(t, err, ErrUserNotFound)
	})

	t.Run("캐시와 결합해 역할 변경이 즉시 반영된다", func(t *testing.T) {
		cache := NewPermissionCache(resolver).WithUserRoles(resolver)

		// kim 은 editor 이므로 agent.update 를 보유한다.
		ok, err := cache.HasPermission(ctx, "kim", rbac.RoleAdmin, "agent.update")
		require.NoError(t, err)
		assert.True(t, ok, "토큰 클레임(admin)이 아니라 users 의 editor 로 판정되어야 한다")

		// editor 는 delete 를 보유하지 않는다.
		ok, err = cache.HasPermission(ctx, "kim", rbac.RoleAdmin, "agent.delete")
		require.NoError(t, err)
		assert.False(t, ok)

		// viewer 로 강등하면 캐시 무효화 없이도 즉시 반영된다 (역할 조회는 캐시하지 않음).
		require.NoError(t, storage.UpdateUserRole(ctx, db, "kim", rbac.RoleViewer))
		ok, err = cache.HasPermission(ctx, "kim", rbac.RoleAdmin, "agent.update")
		require.NoError(t, err)
		assert.False(t, ok)
	})
}
