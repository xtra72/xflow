// @SPEC:SPEC-AUTH-005 (M1)
// rbac.go — 권한 카탈로그 / 빌트인 역할의 auth 패키지 facade.
//
// 실제 정의는 의존성 없는 leaf 패키지 internal/rbac 에 있다.
// auth 패키지가 storage 를 import 하고 (credentials.go), storage 의 역할 시드
// 마이그레이션이 카탈로그를 필요로 하므로, 카탈로그를 auth 에 직접 두면
// storage → auth → storage 순환 참조가 발생한다. 본 파일은 별칭만 제공하여
// 정의를 중복하지 않으면서 auth 패키지 소비자에게 동일한 API 를 노출한다.
//
// M4 의 역할→권한 캐시는 storage 조회가 필요하므로 leaf 가 아닌 본 파일에 추가된다.

package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/internal/storage"
)

// 빌트인 역할 이름. 기존 users.role 값과 동일하다.
const (
	RoleAdmin  = rbac.RoleAdmin
	RoleEditor = rbac.RoleEditor
	RoleViewer = rbac.RoleViewer
)

// BuiltinRole 은 빌트인 역할 정의이다 (rbac.BuiltinRole 의 별칭).
type BuiltinRole = rbac.BuiltinRole

// 카탈로그 API. 정의는 internal/rbac 참조.
var (
	// Permissions 는 카탈로그 전체 권한 키를 사전순으로 반환한다.
	Permissions = rbac.Permissions
	// IsValidPermission 은 권한 키가 카탈로그에 정의되어 있는지 판정한다.
	IsValidPermission = rbac.IsValidPermission
	// BuiltinRoles 는 빌트인 역할 3종을 admin → editor → viewer 순서로 반환한다.
	BuiltinRoles = rbac.BuiltinRoles
	// IsBuiltinRole 은 이름이 빌트인 역할인지 판정한다.
	IsBuiltinRole = rbac.IsBuiltinRole
	// IsValidRoleName 은 역할 이름 규칙(소문자·숫자·하이픈, 1~32자)을 검증한다.
	IsValidRoleName = rbac.IsValidRoleName
)

// @SPEC:SPEC-AUTH-005 (M4) — 역할→권한 캐시.
//
// spec.md §2.3: 권한은 토큰이 아니라 요청 시점에 역할 이름으로 조회한다. 매 요청마다
// DB 를 때리지 않도록 역할→권한 매핑을 인메모리에 보관하고, 역할·권한 쓰기 경로에서
// 무효화한다.

// ErrRoleNotFound 는 조회한 역할이 저장소에 없을 때 반환된다.
//
// 토큰이 이미 삭제된 역할을 가리키는 경우가 여기에 해당한다. 호출자(인가 미들웨어)는
// 이를 "권한 없음"(403)으로 처리해야 하며 500 으로 승격시키면 안 된다
// (acceptance.md 엣지 케이스).
var ErrRoleNotFound = errors.New("auth: 역할을 찾을 수 없습니다")

// PermissionResolver 는 역할 이름으로 권한 키 목록을 조회하는 백엔드이다.
//
// 역할이 없으면 ErrRoleNotFound 를 반환해야 한다. 테스트는 본 인터페이스를 직접
// 구현하여 DB 없이 캐시 동작을 검증한다.
type PermissionResolver interface {
	RolePermissions(ctx context.Context, role string) ([]string, error)
}

// UserRoleResolver 는 username 의 현재 역할을 조회하는 백엔드이다.
//
// 사용자가 없으면 ErrUserNotFound 를 반환해야 한다.
type UserRoleResolver interface {
	UserRole(ctx context.Context, username string) (string, error)
}

// SQLPermissionResolver 는 SQLite 를 백엔드로 하는 PermissionResolver +
// UserRoleResolver 구현이다.
type SQLPermissionResolver struct {
	db *sql.DB
}

// NewSQLPermissionResolver 는 새 SQLPermissionResolver 를 생성한다.
func NewSQLPermissionResolver(db *sql.DB) *SQLPermissionResolver {
	return &SQLPermissionResolver{db: db}
}

// RolePermissions 는 역할의 권한 키 목록을 사전순으로 반환한다.
// 역할이 없으면 ErrRoleNotFound.
func (r *SQLPermissionResolver) RolePermissions(ctx context.Context, role string) ([]string, error) {
	perms, err := storage.ListRolePermissions(ctx, r.db, role)
	if errors.Is(err, storage.ErrRoleNotFound) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("auth: 역할 권한 조회 실패 (%s): %w", role, err)
	}
	return perms, nil
}

// UserRole 은 username 의 현재 역할을 users 테이블에서 조회한다.
// 사용자가 없으면 ErrUserNotFound.
func (r *SQLPermissionResolver) UserRole(ctx context.Context, username string) (string, error) {
	row, err := storage.GetUserByUsername(ctx, r.db, username)
	if errors.Is(err, storage.ErrUserNotFound) {
		return "", ErrUserNotFound
	}
	if err != nil {
		return "", fmt.Errorf("auth: 사용자 역할 조회 실패 (%s): %w", username, err)
	}
	return row.Role, nil
}

// roleEntry 는 캐시에 적재된 단일 역할의 권한 스냅샷이다.
//
// exists=false 는 "저장소에 없는 역할"을 뜻하는 음성 캐시이다. 삭제된 역할을 가리키는
// 토큰이 반복 요청될 때 매번 DB 를 때리지 않도록 음성 결과도 캐시하며, 역할 쓰기 경로의
// 무효화가 이를 되돌린다.
type roleEntry struct {
	exists bool
	set    map[string]struct{}
	list   []string // 사전순 정렬 (Permissions 반환용)
}

// PermissionCache 는 역할→권한 매핑의 RWMutex 보호 인메모리 캐시이다.
//
// 캐시 미스 시 resolver 로 적재하므로 적재 전 첫 요청도 정상 처리된다
// (acceptance.md 엣지 케이스). 역할·권한이 변경되면 Invalidate / InvalidateAll 로
// 무효화해야 한다 — 무효화 누락은 권한 변경이 반영되지 않는 결함이 되므로,
// 역할 쓰기 경로(핸들러)에서 단일 지점으로 호출한다.
type PermissionCache struct {
	mu        sync.RWMutex
	entries   map[string]*roleEntry
	resolver  PermissionResolver
	userRoles UserRoleResolver
}

// NewPermissionCache 는 resolver 를 백엔드로 하는 캐시를 생성한다.
func NewPermissionCache(resolver PermissionResolver) *PermissionCache {
	return &PermissionCache{
		entries:  make(map[string]*roleEntry),
		resolver: resolver,
	}
}

// WithUserRoles 는 유효 역할 판정에 사용할 사용자 역할 조회기를 주입한다.
//
// 주입되지 않으면 EffectiveRole 은 토큰 클레임의 역할을 그대로 사용한다.
func (c *PermissionCache) WithUserRoles(r UserRoleResolver) *PermissionCache {
	c.userRoles = r
	return c
}

// EffectiveRole 은 인가 판정에 사용할 "현재" 역할을 반환한다.
//
// @SPEC:SPEC-AUTH-005 (acceptance.md AC-05, spec.md §6.4)
// JWT 클레임의 역할은 토큰 발급 시점의 스냅샷이라 강등 이후에도 옛 역할을 주장한다.
// AC-05 는 "기존 토큰 그대로" 강등이 반영될 것을 요구하고 §6.4 도 "강등은 다음
// 요청부터 적용" 이라고 명시하므로, 역할은 요청 시점에 users 테이블에서 다시 읽는다.
// 권한 집합 자체는 여전히 역할 이름으로 조회되므로 §2.3 의 설계와 어긋나지 않는다.
//
// users 에 없는 주체(원격 노드 토큰 등)는 fallback(토큰 클레임)을 그대로 사용한다.
// 본 조회는 캐시하지 않는다 — 즉시 반영이 목적인데 캐시하면 별도 무효화 경로가
// 필요해지고, username 은 UNIQUE 인덱스 조회라 비용이 무시할 수준이다.
func (c *PermissionCache) EffectiveRole(ctx context.Context, username, fallback string) (string, error) {
	if c.userRoles == nil || username == "" {
		return fallback, nil
	}
	role, err := c.userRoles.UserRole(ctx, username)
	if errors.Is(err, ErrUserNotFound) {
		return fallback, nil
	}
	if err != nil {
		return "", err
	}
	return role, nil
}

// HasPermission 은 username/role 주체가 permission 을 보유했는지 판정한다.
//
// role 은 토큰 클레임의 역할이며, users 테이블에 해당 username 이 있으면 그쪽 역할이
// 우선한다 (EffectiveRole 참조).
//
// 존재하지 않는 역할(삭제된 역할을 가리키는 토큰 포함)은 (false, nil) 이다 — 호출자가
// 403 으로 처리하고 500 으로 승격시키지 않도록 에러가 아닌 거부로 표현한다.
// 저장소 장애 등 그 외 에러만 error 로 전파된다.
func (c *PermissionCache) HasPermission(ctx context.Context, username, role, permission string) (bool, error) {
	effective, err := c.EffectiveRole(ctx, username, role)
	if err != nil {
		return false, err
	}
	entry, err := c.lookup(ctx, effective)
	if err != nil {
		return false, err
	}
	if !entry.exists {
		return false, nil
	}
	_, ok := entry.set[permission]
	return ok, nil
}

// Permissions 는 role 의 권한 키 목록을 사전순으로 반환한다.
// 존재하지 않는 역할은 빈 슬라이스이다 (에러 아님).
func (c *PermissionCache) Permissions(ctx context.Context, role string) ([]string, error) {
	entry, err := c.lookup(ctx, role)
	if err != nil {
		return nil, err
	}
	if !entry.exists {
		return []string{}, nil
	}
	return append([]string(nil), entry.list...), nil
}

// Invalidate 는 단일 역할의 캐시 항목을 제거한다 (다음 조회 시 재적재).
func (c *PermissionCache) Invalidate(role string) {
	c.mu.Lock()
	delete(c.entries, role)
	c.mu.Unlock()
}

// InvalidateAll 은 캐시 전체를 비운다.
//
// 역할 이름 변경처럼 여러 역할·사용자 행이 동시에 바뀌는 경로에서 사용한다.
func (c *PermissionCache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]*roleEntry)
	c.mu.Unlock()
}

// lookup 은 캐시에서 역할 항목을 얻고, 미스이면 resolver 로 적재한다.
func (c *PermissionCache) lookup(ctx context.Context, role string) (*roleEntry, error) {
	c.mu.RLock()
	entry, ok := c.entries[role]
	c.mu.RUnlock()
	if ok {
		return entry, nil
	}

	if c.resolver == nil {
		return &roleEntry{}, nil
	}

	perms, err := c.resolver.RolePermissions(ctx, role)
	switch {
	case errors.Is(err, ErrRoleNotFound):
		entry = &roleEntry{exists: false}
	case err != nil:
		// 저장소 장애는 캐시하지 않는다 (일시적 오류가 고착되지 않도록).
		return nil, err
	default:
		set := make(map[string]struct{}, len(perms))
		for _, p := range perms {
			set[p] = struct{}{}
		}
		entry = &roleEntry{exists: true, set: set, list: perms}
	}

	c.mu.Lock()
	c.entries[role] = entry
	c.mu.Unlock()
	return entry, nil
}
