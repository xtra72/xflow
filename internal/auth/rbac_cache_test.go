package auth

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @SPEC:SPEC-AUTH-005 (M4) — 역할→권한 캐시 단위 테스트.
//
// DB 없이 PermissionResolver / UserRoleResolver 를 대체 구현하여 캐시 적재·무효화·
// 오류 전파 동작을 검증한다.

// fakeResolver 는 호출 횟수를 세는 PermissionResolver 구현이다.
type fakeResolver struct {
	mu    sync.Mutex
	roles map[string][]string
	err   error
	calls map[string]int
}

func newFakeResolver(roles map[string][]string) *fakeResolver {
	return &fakeResolver{roles: roles, calls: map[string]int{}}
}

func (f *fakeResolver) RolePermissions(_ context.Context, role string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls[role]++
	if f.err != nil {
		return nil, f.err
	}
	perms, ok := f.roles[role]
	if !ok {
		return nil, ErrRoleNotFound
	}
	return append([]string(nil), perms...), nil
}

func (f *fakeResolver) callCount(role string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[role]
}

// fakeUserRoles 는 UserRoleResolver 대체 구현이다.
type fakeUserRoles struct {
	users map[string]string
	err   error
}

func (f *fakeUserRoles) UserRole(_ context.Context, username string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	role, ok := f.users[username]
	if !ok {
		return "", ErrUserNotFound
	}
	return role, nil
}

func TestPermissionCache_ColdMissLoadsAndCaches(t *testing.T) {
	res := newFakeResolver(map[string][]string{"operator": {"agent.read", "agent.execute"}})
	cache := NewPermissionCache(res)
	ctx := context.Background()

	// 캐시 적재 전 첫 요청도 정상 처리된다 (acceptance.md 엣지 케이스).
	ok, err := cache.HasPermission(ctx, "", "operator", "agent.read")
	require.NoError(t, err)
	assert.True(t, ok)

	// 두 번째 조회는 캐시 적중이므로 resolver 를 다시 호출하지 않는다.
	ok, err = cache.HasPermission(ctx, "", "operator", "agent.execute")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 1, res.callCount("operator"))

	// 보유하지 않은 권한은 false 이다.
	ok, err = cache.HasPermission(ctx, "", "operator", "agent.delete")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestPermissionCache_MissingRoleIsDeniedNotError(t *testing.T) {
	res := newFakeResolver(map[string][]string{})
	cache := NewPermissionCache(res)

	// 삭제된 역할을 가리키는 토큰은 403 으로 처리되어야 하므로 error 가 아니라
	// (false, nil) 이다 (acceptance.md 엣지 케이스: 500 아님).
	ok, err := cache.HasPermission(context.Background(), "", "ghost", "agent.read")
	assert.NoError(t, err)
	assert.False(t, ok)

	// 음성 결과도 캐시되어 반복 요청이 DB 를 반복해서 때리지 않는다.
	_, _ = cache.HasPermission(context.Background(), "", "ghost", "agent.read")
	assert.Equal(t, 1, res.callCount("ghost"))
}

func TestPermissionCache_ResolverErrorPropagatesAndIsNotCached(t *testing.T) {
	res := newFakeResolver(map[string][]string{})
	res.err = errors.New("db down")
	cache := NewPermissionCache(res)

	_, err := cache.HasPermission(context.Background(), "", "operator", "agent.read")
	require.Error(t, err)

	// 일시적 장애가 캐시에 고착되지 않는다 (다음 호출이 다시 시도한다).
	_, _ = cache.HasPermission(context.Background(), "", "operator", "agent.read")
	assert.Equal(t, 2, res.callCount("operator"))
}

func TestPermissionCache_Invalidate(t *testing.T) {
	res := newFakeResolver(map[string][]string{"operator": {"agent.read"}})
	cache := NewPermissionCache(res)
	ctx := context.Background()

	ok, err := cache.HasPermission(ctx, "", "operator", "agent.execute")
	require.NoError(t, err)
	require.False(t, ok)

	// 권한 확대 후 무효화하면 다음 요청부터 반영된다.
	res.roles["operator"] = []string{"agent.read", "agent.execute"}
	cache.Invalidate("operator")

	ok, err = cache.HasPermission(ctx, "", "operator", "agent.execute")
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, 2, res.callCount("operator"))
}

func TestPermissionCache_InvalidateAll(t *testing.T) {
	res := newFakeResolver(map[string][]string{
		"operator": {"agent.read"},
		"viewer":   {"agent.read"},
	})
	cache := NewPermissionCache(res)
	ctx := context.Background()

	_, _ = cache.HasPermission(ctx, "", "operator", "agent.read")
	_, _ = cache.HasPermission(ctx, "", "viewer", "agent.read")
	require.Equal(t, 1, res.callCount("operator"))
	require.Equal(t, 1, res.callCount("viewer"))

	cache.InvalidateAll()
	_, _ = cache.HasPermission(ctx, "", "operator", "agent.read")
	_, _ = cache.HasPermission(ctx, "", "viewer", "agent.read")
	assert.Equal(t, 2, res.callCount("operator"))
	assert.Equal(t, 2, res.callCount("viewer"))
}

func TestPermissionCache_Permissions(t *testing.T) {
	res := newFakeResolver(map[string][]string{"operator": {"agent.read", "agent.execute"}})
	cache := NewPermissionCache(res)
	ctx := context.Background()

	perms, err := cache.Permissions(ctx, "operator")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"agent.read", "agent.execute"}, perms)

	// 반환 슬라이스를 변형해도 캐시가 오염되지 않는다.
	perms[0] = "mutated"
	again, err := cache.Permissions(ctx, "operator")
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"agent.read", "agent.execute"}, again)

	// 존재하지 않는 역할은 빈 슬라이스이며 에러가 아니다.
	empty, err := cache.Permissions(ctx, "ghost")
	require.NoError(t, err)
	assert.Empty(t, empty)
}

func TestPermissionCache_NilResolverDeniesWithoutPanic(t *testing.T) {
	cache := NewPermissionCache(nil)

	ok, err := cache.HasPermission(context.Background(), "", "admin", "agent.read")
	assert.NoError(t, err)
	assert.False(t, ok)
}

func TestPermissionCache_EffectiveRole(t *testing.T) {
	res := newFakeResolver(map[string][]string{
		"operator": {"agent.read", "agent.execute"},
		"viewer":   {"agent.read"},
	})
	users := &fakeUserRoles{users: map[string]string{"kim": "viewer"}}
	cache := NewPermissionCache(res).WithUserRoles(users)
	ctx := context.Background()

	// users 테이블의 역할이 토큰 클레임보다 우선한다 (AC-05).
	role, err := cache.EffectiveRole(ctx, "kim", "operator")
	require.NoError(t, err)
	assert.Equal(t, "viewer", role)

	ok, err := cache.HasPermission(ctx, "kim", "operator", "agent.execute")
	require.NoError(t, err)
	assert.False(t, ok, "강등된 사용자가 옛 토큰 클레임으로 권한을 유지했다")

	// users 에 없는 주체는 토큰 클레임을 그대로 사용한다 (원격 노드 토큰 등).
	role, err = cache.EffectiveRole(ctx, "unknown", "operator")
	require.NoError(t, err)
	assert.Equal(t, "operator", role)

	ok, err = cache.HasPermission(ctx, "unknown", "operator", "agent.execute")
	require.NoError(t, err)
	assert.True(t, ok)

	// username 이 비어있으면 조회하지 않는다.
	role, err = cache.EffectiveRole(ctx, "", "operator")
	require.NoError(t, err)
	assert.Equal(t, "operator", role)
}

func TestPermissionCache_EffectiveRoleErrorPropagates(t *testing.T) {
	res := newFakeResolver(map[string][]string{"viewer": {"agent.read"}})
	users := &fakeUserRoles{err: errors.New("db down")}
	cache := NewPermissionCache(res).WithUserRoles(users)

	_, err := cache.EffectiveRole(context.Background(), "kim", "viewer")
	assert.Error(t, err)

	_, err = cache.HasPermission(context.Background(), "kim", "viewer", "agent.read")
	assert.Error(t, err)
}

func TestPermissionCache_ConcurrentAccess(t *testing.T) {
	res := newFakeResolver(map[string][]string{"operator": {"agent.read"}})
	cache := NewPermissionCache(res)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = cache.HasPermission(ctx, "", "operator", "agent.read")
			if n%10 == 0 {
				cache.Invalidate("operator")
			}
		}(i)
	}
	wg.Wait()

	ok, err := cache.HasPermission(ctx, "", "operator", "agent.read")
	require.NoError(t, err)
	assert.True(t, ok)
}
