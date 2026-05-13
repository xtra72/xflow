package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-2)
// users_sqlite.go — users 테이블 CRUD 헬퍼.
//
// credentials.go 가 본 헬퍼들을 사용하여 SQLite 기반 자격증명 저장소를 구현한다.
// yaml 파일 의존성은 EnsureDefaultAdmin 의 1회성 마이그레이션에만 남는다 (UB-007).

// ErrUserNotFound 는 username 에 해당하는 사용자가 users 테이블에 없을 때 반환된다.
//
// 본 sentinel 은 storage 패키지의 내부 헬퍼 레벨에서 사용되며, 외부 인증 흐름은
// auth.ErrUserNotFound 를 매핑하여 사용한다 (auth/credentials.go).
var ErrUserNotFound = errors.New("user not found")

// UserRow 는 users 테이블의 단일 row 를 표현한다 (storage 헬퍼 전용 DTO).
//
// 외부 (auth 패키지) 는 본 구조체를 직접 노출하지 않고, CredentialUser 로 매핑한다.
type UserRow struct {
	ID           int64
	Username     string
	PasswordHash string
	Role         string
	CreatedAt    int64 // epoch ms
	UpdatedAt    int64 // epoch ms
}

// InsertUser 는 새 사용자를 추가한다. username 중복 시 sql 드라이버의 UNIQUE 위반 에러를
// 반환한다 (호출자가 errors.Is 로 매칭하거나 onConflict 분기에서 사용).
//
// createdAtMs/updatedAtMs 가 0 이면 호출 시점의 time.Now().UnixMilli() 가 부여된다.
func InsertUser(ctx context.Context, db *sql.DB, username, passwordHash, role string, createdAtMs, updatedAtMs int64) error {
	if role == "" {
		role = "viewer"
	}
	now := time.Now().UnixMilli()
	if createdAtMs == 0 {
		createdAtMs = now
	}
	if updatedAtMs == 0 {
		updatedAtMs = now
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO users(username, password_hash, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, username, passwordHash, role, createdAtMs, updatedAtMs); err != nil {
		return fmt.Errorf("insert user %q: %w", username, err)
	}
	return nil
}

// InsertUserIgnore 는 username 중복 시 silently skip 한다 (INSERT OR IGNORE).
//
// yaml → SQLite 1회성 마이그레이션 (AC-2) 에서 사용된다. 이미 존재하는 사용자는 덮어쓰지
// 않는다.
//
// 반환값: 실제로 삽입된 row 수 (0 = 이미 존재, 1 = 신규 삽입).
func InsertUserIgnore(ctx context.Context, db *sql.DB, username, passwordHash, role string, createdAtMs, updatedAtMs int64) (int64, error) {
	if role == "" {
		role = "viewer"
	}
	now := time.Now().UnixMilli()
	if createdAtMs == 0 {
		createdAtMs = now
	}
	if updatedAtMs == 0 {
		updatedAtMs = now
	}
	res, err := db.ExecContext(ctx, `
		INSERT OR IGNORE INTO users(username, password_hash, role, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`, username, passwordHash, role, createdAtMs, updatedAtMs)
	if err != nil {
		return 0, fmt.Errorf("insert or ignore user %q: %w", username, err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return rows, nil
}

// GetUserByUsername 은 username 으로 사용자를 조회한다. 없으면 ErrUserNotFound.
func GetUserByUsername(ctx context.Context, db *sql.DB, username string) (*UserRow, error) {
	var u UserRow
	err := db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %q: %w", username, err)
	}
	return &u, nil
}

// UpdatePasswordHash 는 username 의 password_hash 와 updated_at 을 갱신한다.
// 사용자가 없으면 ErrUserNotFound.
func UpdatePasswordHash(ctx context.Context, db *sql.DB, username, newHash string) error {
	now := time.Now().UnixMilli()
	res, err := db.ExecContext(ctx, `
		UPDATE users SET password_hash = ?, updated_at = ? WHERE username = ?
	`, newHash, now, username)
	if err != nil {
		return fmt.Errorf("update password hash %q: %w", username, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// CountUsers 는 users 테이블의 총 row 수를 반환한다 (마이그레이션 트리거 판정용).
func CountUsers(ctx context.Context, db *sql.DB) (int64, error) {
	var n int64
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count users: %w", err)
	}
	return n, nil
}

// ListUsers 는 모든 사용자를 username 알파벳 순서로 반환한다 (관리용).
//
// 본 SPEC 범위에서는 마이그레이션 검증용으로만 사용되며, 사용자 관리 API 는 OI-004
// 로 분리된다.
func ListUsers(ctx context.Context, db *sql.DB) ([]UserRow, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT id, username, password_hash, role, created_at, updated_at
		FROM users ORDER BY username ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var out []UserRow
	for rows.Next() {
		var u UserRow
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate users: %w", err)
	}
	return out, nil
}
