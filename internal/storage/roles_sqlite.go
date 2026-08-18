package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/rbac"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// @SPEC:SPEC-AUTH-005 (M1)
// roles_sqlite.go — roles / role_permissions 테이블 CRUD 헬퍼.
//
// 스키마 생성과 빌트인 역할 시드는 sqlite.go 의 migrateRolesSchema 가 담당한다.
// 본 파일은 조회·변경 헬퍼만 제공하며, 잠금 방지 불변식(UB1) 은 상위 API 계층에서
// 강제한다 (본 계층은 데이터 정합성만 책임진다).

// 역할 저장소 sentinel 에러.
var (
	// ErrRoleNotFound 는 이름에 해당하는 역할이 없을 때 반환된다.
	ErrRoleNotFound = errors.New("role not found")
	// ErrRoleExists 는 이미 존재하는 역할 이름으로 생성을 시도할 때 반환된다.
	ErrRoleExists = errors.New("role already exists")
	// ErrInvalidRoleName 은 역할 이름 규칙(소문자·숫자·하이픈, 1~32자) 위반 시 반환된다.
	ErrInvalidRoleName = errors.New("invalid role name")
	// ErrInvalidPermission 은 카탈로그에 없는 권한 키를 저장하려 할 때 반환된다.
	//
	// spec.md §2.1: "정의되지 않은 권한 키는 역할에 저장될 수 없다" — 영속 계층에서
	// 최종 방어한다.
	ErrInvalidPermission = errors.New("invalid permission key")
)

// RoleRow 는 roles 테이블의 단일 row 이다 (storage 헬퍼 전용 DTO).
//
// Permissions 는 ListRoles / GetRoleByName 이 함께 채운다. 권한이 없는 역할은
// 빈 슬라이스가 아니라 nil 이 된다.
type RoleRow struct {
	ID          int64
	Name        string
	Description string
	Builtin     bool
	CreatedAt   int64 // epoch ms
	UpdatedAt   int64 // epoch ms
	Permissions []string
}

// ListRoles 는 모든 역할을 이름 알파벳 순서로, 각 역할의 권한과 함께 반환한다.
func ListRoles(ctx context.Context, db *sql.DB) ([]RoleRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, name, description, builtin, created_at, updated_at
		FROM roles ORDER BY name ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer rows.Close()

	var out []RoleRow
	byID := make(map[int64]int) // role_id → out 인덱스
	for rows.Next() {
		var r RoleRow
		var builtin int64
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &builtin, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan role: %w", err)
		}
		r.Builtin = builtin != 0
		byID[r.ID] = len(out)
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate roles: %w", err)
	}
	if len(out) == 0 {
		return out, nil
	}

	// 권한은 역할 수와 무관하게 단일 쿼리로 적재한다 (N+1 방지).
	permRows, err := db.QueryContext(ctx, `
		SELECT role_id, permission FROM role_permissions ORDER BY role_id ASC, permission ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list role permissions: %w", err)
	}
	defer permRows.Close()

	for permRows.Next() {
		var roleID int64
		var perm string
		if err := permRows.Scan(&roleID, &perm); err != nil {
			return nil, fmt.Errorf("scan role permission: %w", err)
		}
		if idx, ok := byID[roleID]; ok {
			out[idx].Permissions = append(out[idx].Permissions, perm)
		}
	}
	if err := permRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate role permissions: %w", err)
	}
	return out, nil
}

// GetRoleByName 은 이름으로 역할을 권한과 함께 조회한다. 없으면 ErrRoleNotFound.
func GetRoleByName(ctx context.Context, db *sql.DB, name string) (*RoleRow, error) {
	var r RoleRow
	var builtin int64
	err := db.QueryRowContext(ctx, `
		SELECT id, name, description, builtin, created_at, updated_at
		FROM roles WHERE name = ?
	`, name).Scan(&r.ID, &r.Name, &r.Description, &builtin, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get role %q: %w", name, err)
	}
	r.Builtin = builtin != 0

	perms, err := listPermissionsByRoleID(ctx, db, r.ID)
	if err != nil {
		return nil, err
	}
	r.Permissions = perms
	return &r, nil
}

// ListRolePermissions 는 역할 이름으로 권한 목록을 사전순으로 반환한다.
// 역할이 없으면 ErrRoleNotFound (인가 미들웨어가 403 으로 처리한다).
func ListRolePermissions(ctx context.Context, db *sql.DB, name string) ([]string, error) {
	var roleID int64
	err := db.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = ?`, name).Scan(&roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRoleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get role id %q: %w", name, err)
	}
	return listPermissionsByRoleID(ctx, db, roleID)
}

// listPermissionsByRoleIDTx 는 listPermissionsByRoleID 의 트랜잭션 판이다.
// 마이그레이션이 같은 트랜잭션 안에서 기존 권한을 읽어야 하므로 분리했다.
func listPermissionsByRoleIDTx(ctx context.Context, tx *sql.Tx, roleID int64) ([]string, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT permission FROM role_permissions WHERE role_id = ? ORDER BY permission ASC
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list permissions of role %d: %w", roleID, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}
	return out, nil
}

// listPermissionsByRoleID 는 role_id 로 권한 목록을 사전순으로 조회한다.
func listPermissionsByRoleID(ctx context.Context, db *sql.DB, roleID int64) ([]string, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT permission FROM role_permissions WHERE role_id = ? ORDER BY permission ASC
	`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list permissions of role %d: %w", roleID, err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, fmt.Errorf("scan permission: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate permissions: %w", err)
	}
	return out, nil
}

// InsertRole 은 커스텀 역할을 권한과 함께 생성한다 (builtin = 0).
//
// 이름 규칙 위반 시 ErrInvalidRoleName, 카탈로그에 없는 권한 포함 시
// ErrInvalidPermission, 중복 이름이면 ErrRoleExists 를 반환하며 어느 경우에도
// 행이 생성되지 않는다 (전체가 단일 트랜잭션).
func InsertRole(ctx context.Context, db *sql.DB, name, description string, permissions []string) error {
	if !rbac.IsValidRoleName(name) {
		return fmt.Errorf("insert role %q: %w", name, ErrInvalidRoleName)
	}
	if err := validatePermissions(permissions); err != nil {
		return fmt.Errorf("insert role %q: %w", name, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert role %q: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var exists int64
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM roles WHERE name = ?`, name).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check role %q: %w", name, err)
	}
	if exists > 0 {
		return fmt.Errorf("insert role %q: %w", name, ErrRoleExists)
	}

	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO roles(name, description, builtin, nav_migrated, created_at, updated_at)
		VALUES (?, ?, 0, 1, ?, ?)
	`, name, description, now, now)
	if err != nil {
		return fmt.Errorf("insert role %q: %w", name, err)
	}
	roleID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("last insert id for role %q: %w", name, err)
	}

	if err := replacePermissionsTx(ctx, tx, roleID, permissions, false); err != nil {
		return fmt.Errorf("insert role %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert role %q: %w", name, err)
	}
	return nil
}

// UpdateRolePermissions 는 역할의 권한 집합을 통째로 교체한다 (delete → insert).
//
// 역할이 없으면 ErrRoleNotFound, 카탈로그에 없는 권한이 있으면
// ErrInvalidPermission 이며 기존 권한은 변경되지 않는다.
func UpdateRolePermissions(ctx context.Context, db *sql.DB, name string, permissions []string) error {
	if err := validatePermissions(permissions); err != nil {
		return fmt.Errorf("update permissions of role %q: %w", name, err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin update permissions of role %q: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var roleID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = ?`, name).Scan(&roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRoleNotFound
	}
	if err != nil {
		return fmt.Errorf("get role id %q: %w", name, err)
	}

	if err := replacePermissionsTx(ctx, tx, roleID, permissions, true); err != nil {
		return fmt.Errorf("update permissions of role %q: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE roles SET updated_at = ? WHERE id = ?`, time.Now().UnixMilli(), roleID); err != nil {
		return fmt.Errorf("touch role %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit update permissions of role %q: %w", name, err)
	}
	return nil
}

// UpdateRoleName 은 역할 이름을 변경하고, 해당 역할을 사용 중인 users.role 을
// 동일 트랜잭션에서 함께 갱신한다.
//
// @SPEC:SPEC-AUTH-005 (spec.md §4.1)
// users.role 은 역할 id 가 아니라 이름 문자열이므로, 이름 변경 시 사용자 행을
// 갱신하지 않으면 존재하지 않는 역할을 가리키게 된다.
//
// 이름 규칙 위반 시 ErrInvalidRoleName, 원본 없음 ErrRoleNotFound,
// 새 이름 중복 시 ErrRoleExists.
func UpdateRoleName(ctx context.Context, db *sql.DB, oldName, newName string) error {
	if !rbac.IsValidRoleName(newName) {
		return fmt.Errorf("rename role %q: %w", oldName, ErrInvalidRoleName)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin rename role %q: %w", oldName, err)
	}
	defer func() { _ = tx.Rollback() }()

	var roleID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = ?`, oldName).Scan(&roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRoleNotFound
	}
	if err != nil {
		return fmt.Errorf("get role id %q: %w", oldName, err)
	}

	if oldName == newName {
		// 동일 이름 변경은 no-op 으로 처리한다 (UNIQUE 위반 회피).
		return nil
	}

	var exists int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM roles WHERE name = ?`, newName).Scan(&exists); err != nil {
		return fmt.Errorf("check role %q: %w", newName, err)
	}
	if exists > 0 {
		return fmt.Errorf("rename role to %q: %w", newName, ErrRoleExists)
	}

	now := time.Now().UnixMilli()
	if _, err := tx.ExecContext(ctx,
		`UPDATE roles SET name = ?, updated_at = ? WHERE id = ?`, newName, now, roleID); err != nil {
		return fmt.Errorf("rename role %q: %w", oldName, err)
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET role = ?, updated_at = ? WHERE role = ?`, newName, now, oldName); err != nil {
		return fmt.Errorf("update users of renamed role %q: %w", oldName, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit rename role %q: %w", oldName, err)
	}
	return nil
}

// DeleteRole 은 역할과 그 권한을 삭제한다. 없으면 ErrRoleNotFound.
//
// role_permissions 는 ON DELETE CASCADE 로 선언되어 있으나, SQLite 의
// foreign_keys PRAGMA 는 커넥션 단위 기본 OFF 이므로 명시적으로 함께 삭제한다.
//
// 빌트인 역할 보호와 사용 중인 역할 보호(UB1) 는 상위 API 계층의 책임이다.
func DeleteRole(ctx context.Context, db *sql.DB, name string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete role %q: %w", name, err)
	}
	defer func() { _ = tx.Rollback() }()

	var roleID int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM roles WHERE name = ?`, name).Scan(&roleID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrRoleNotFound
	}
	if err != nil {
		return fmt.Errorf("get role id %q: %w", name, err)
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
		return fmt.Errorf("delete permissions of role %q: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM roles WHERE id = ?`, roleID); err != nil {
		return fmt.Errorf("delete role %q: %w", name, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete role %q: %w", name, err)
	}
	return nil
}

// validatePermissions 는 모든 권한 키가 카탈로그에 정의되어 있는지 검증한다.
func validatePermissions(permissions []string) error {
	for _, p := range permissions {
		if !rbac.IsValidPermission(p) {
			return fmt.Errorf("%q: %w", p, ErrInvalidPermission)
		}
	}
	return nil
}

// replacePermissionsTx 는 트랜잭션 안에서 역할의 권한 집합을 설정한다.
// clear 가 true 면 기존 권한을 먼저 삭제한다. 중복 키는 무시된다.
func replacePermissionsTx(ctx context.Context, tx *sql.Tx, roleID int64, permissions []string, clear bool) error {
	if clear {
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
			return fmt.Errorf("clear permissions: %w", err)
		}
	}
	for _, p := range permissions {
		if _, err := tx.ExecContext(ctx,
			`INSERT OR IGNORE INTO role_permissions(role_id, permission) VALUES (?, ?)`,
			roleID, p); err != nil {
			return fmt.Errorf("insert permission %q: %w", p, err)
		}
	}
	return nil
}
