// @SPEC:SPEC-AUTH-005 (M3)
// users_role_test.go — 사용자 저장소 확장 헬퍼 검증
// (UpdateUserRole / DeleteUser / CountUsersByRole).

package storage

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

// TestUpdateUserRole 는 역할 변경과 updated_at 갱신을 검증한다.
func TestUpdateUserRole(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertUser(ctx, db, "kim", "hash-kim", "viewer", 1700000000001, 1700000000001))

	before, err := GetUserByUsername(ctx, db, "kim")
	require.NoError(t, err)

	require.NoError(t, UpdateUserRole(ctx, db, "kim", "operator"))

	after, err := GetUserByUsername(ctx, db, "kim")
	require.NoError(t, err)
	assert.Equal(t, "operator", after.Role, "커스텀 역할로 변경 가능해야 한다")
	assert.Greater(t, after.UpdatedAt, before.UpdatedAt, "updated_at 이 갱신되어야 한다")
	assert.Equal(t, before.CreatedAt, after.CreatedAt, "created_at 은 유지되어야 한다")
	assert.Equal(t, before.PasswordHash, after.PasswordHash, "비밀번호 해시는 유지되어야 한다")
}

// TestUpdateUserRole_NotFound 는 미존재 사용자 처리 sentinel 을 검증한다.
func TestUpdateUserRole_NotFound(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	assert.ErrorIs(t, UpdateUserRole(ctx, db, "ghost", "viewer"), ErrUserNotFound)
}

// TestDeleteUser 는 삭제와 미존재 사용자 처리를 검증한다.
func TestDeleteUser(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertUser(ctx, db, "kim", "hash-kim", "viewer", 0, 0))
	require.NoError(t, InsertUser(ctx, db, "lee", "hash-lee", "viewer", 0, 0))

	require.NoError(t, DeleteUser(ctx, db, "kim"))

	_, err := GetUserByUsername(ctx, db, "kim")
	assert.ErrorIs(t, err, ErrUserNotFound)

	remaining, err := ListUsers(ctx, db)
	require.NoError(t, err)
	require.Len(t, remaining, 1, "다른 사용자는 영향받지 않아야 한다")
	assert.Equal(t, "lee", remaining[0].Username)

	assert.ErrorIs(t, DeleteUser(ctx, db, "kim"), ErrUserNotFound, "재삭제는 ErrUserNotFound")
}

// TestCountUsersByRole 는 역할별 사용자 수 집계를 검증한다 (UB1 불변식 판정용).
func TestCountUsersByRole(t *testing.T) {
	db, _ := setupRolesDB(t)
	ctx := context.Background()

	require.NoError(t, InsertUser(ctx, db, "admin", "h", "admin", 0, 0))
	require.NoError(t, InsertUser(ctx, db, "kim", "h", "operator", 0, 0))
	require.NoError(t, InsertUser(ctx, db, "lee", "h", "operator", 0, 0))

	tests := []struct {
		role string
		want int64
	}{
		{"admin", 1},
		{"operator", 2},
		{"viewer", 0},
		{"ghost-role", 0},
		{"", 0},
	}
	for _, tc := range tests {
		got, err := CountUsersByRole(ctx, db, tc.role)
		require.NoErrorf(t, err, "role=%q", tc.role)
		assert.Equalf(t, tc.want, got, "role=%q", tc.role)
	}

	// 강등 후 집계가 즉시 반영된다 (마지막 admin 강등 차단 판정 경로).
	require.NoError(t, UpdateUserRole(ctx, db, "admin", "viewer"))
	got, err := CountUsersByRole(ctx, db, "admin")
	require.NoError(t, err)
	assert.Zero(t, got)
}
