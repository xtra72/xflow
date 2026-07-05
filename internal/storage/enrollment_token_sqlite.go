// enrollment_token_sqlite.go 는 EnrollmentTokenRepository 의 SQLite 구현이다
// (@SPEC:SPEC-REMOTE-001 v1.1 그룹 H, spec §5.8 enrollment_tokens).
//
// ManagedNodeSQLiteRepository/RemoteAuditSQLiteRepository 패턴을 준용한다: WAL 모드,
// IF NOT EXISTS 멱등 마이그레이션, 기존 xflow.db 에 enrollment_tokens 테이블 멱등 추가.
//
// 보안(REQ-H06): token_hash 컬럼만 존재하며 원본 토큰 컬럼은 의도적으로 없다. List 는
// token_hash 를 SELECT 하지 않아 메타데이터만 노출한다.
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ EnrollmentTokenRepository = (*EnrollmentTokenSQLiteRepository)(nil)

// EnrollmentTokenSQLiteRepository 는 SQLite 기반 EnrollmentTokenRepository 구현이다.
type EnrollmentTokenSQLiteRepository struct {
	db *sql.DB
}

// NewEnrollmentTokenSQLiteRepository 는 SQLite DB 를 열고 enrollment_tokens 테이블을
// 멱등하게 생성한 후 저장소를 반환한다. WAL 모드를 활성화한다.
func NewEnrollmentTokenSQLiteRepository(ctx context.Context, dbPath string) (*EnrollmentTokenSQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// DSN 에 WAL + busy_timeout pragma 를 실어 모든 풀 연결에 적용한다(다중 노드 동시
	// register 시 uses 증가 경합 흡수 — REQ-N01/H07).
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrateEnrollmentTokensSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &EnrollmentTokenSQLiteRepository{db: db}, nil
}

// migrateEnrollmentTokensSchema 는 enrollment_tokens 테이블을 멱등하게 생성한다(spec §5.8).
//
// token_hash 에 UNIQUE 인덱스를 두어 동일 토큰 중복 등록을 방지하고 GetByHash 를
// 가속한다. expires_at/max_uses 는 NULL 허용(무기한/무제한).
func migrateEnrollmentTokensSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS enrollment_tokens (
		id          TEXT    PRIMARY KEY,
		token_hash  TEXT    NOT NULL UNIQUE,
		label       TEXT    NOT NULL DEFAULT '',
		created_at  INTEGER NOT NULL DEFAULT 0,
		expires_at  INTEGER,
		max_uses    INTEGER,
		uses        INTEGER NOT NULL DEFAULT 0,
		revoked     INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create enrollment_tokens table: %w", err)
	}
	return nil
}

// Create 는 새 토큰을 저장한다(token_hash 만 — 원본 미저장, REQ-H06).
func (r *EnrollmentTokenSQLiteRepository) Create(ctx context.Context, tok EnrollmentToken) error {
	revoked := 0
	if tok.Revoked {
		revoked = 1
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO enrollment_tokens (id, token_hash, label, created_at, expires_at, max_uses, uses, revoked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, tok.ID, tok.TokenHash, tok.Label, tok.CreatedAt,
		nullableInt64(tok.ExpiresAt), nullableInt(tok.MaxUses), tok.Uses, revoked)
	if err != nil {
		return fmt.Errorf("create enrollment token: %w", err)
	}
	return nil
}

// scanEnrollmentToken 은 NULL 가능 컬럼을 안전하게 스캔한다.
func scanEnrollmentToken(scan func(dest ...any) error, includeHash bool) (EnrollmentToken, error) {
	var (
		tok       EnrollmentToken
		hash      string
		expiresAt sql.NullInt64
		maxUses   sql.NullInt64
		revoked   int
	)
	var err error
	if includeHash {
		err = scan(&tok.ID, &hash, &tok.Label, &tok.CreatedAt, &expiresAt, &maxUses, &tok.Uses, &revoked)
	} else {
		// List 경로: token_hash 를 SELECT 하지 않으므로 hash 자리를 비운다(REQ-H06).
		err = scan(&tok.ID, &tok.Label, &tok.CreatedAt, &expiresAt, &maxUses, &tok.Uses, &revoked)
	}
	if err != nil {
		return EnrollmentToken{}, err
	}
	if includeHash {
		tok.TokenHash = hash
	}
	if expiresAt.Valid {
		tok.ExpiresAt = Int64Ptr(expiresAt.Int64)
	}
	if maxUses.Valid {
		tok.MaxUses = IntPtr(int(maxUses.Int64))
	}
	tok.Revoked = revoked != 0
	return tok, nil
}

// GetByHash 는 token_hash 로 토큰을 조회한다. 없으면 ErrEnrollmentTokenNotFound.
func (r *EnrollmentTokenSQLiteRepository) GetByHash(ctx context.Context, tokenHash string) (EnrollmentToken, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, token_hash, label, created_at, expires_at, max_uses, uses, revoked
		FROM enrollment_tokens WHERE token_hash = ?
	`, tokenHash)
	tok, err := scanEnrollmentToken(row.Scan, true)
	if errors.Is(err, sql.ErrNoRows) {
		return EnrollmentToken{}, ErrEnrollmentTokenNotFound
	}
	if err != nil {
		return EnrollmentToken{}, fmt.Errorf("get enrollment token by hash: %w", err)
	}
	return tok, nil
}

// List 는 모든 토큰의 메타데이터를 created_at 순서로 반환한다(token_hash 미노출 — REQ-H06).
func (r *EnrollmentTokenSQLiteRepository) List(ctx context.Context) ([]EnrollmentToken, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, label, created_at, expires_at, max_uses, uses, revoked
		FROM enrollment_tokens ORDER BY created_at
	`)
	if err != nil {
		return nil, fmt.Errorf("list enrollment tokens: %w", err)
	}
	defer rows.Close()

	var out []EnrollmentToken
	for rows.Next() {
		tok, scanErr := scanEnrollmentToken(rows.Scan, false)
		if scanErr != nil {
			return nil, fmt.Errorf("scan enrollment token row: %w", scanErr)
		}
		out = append(out, tok)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate enrollment token rows: %w", err)
	}
	return out, nil
}

// Revoke 는 토큰을 폐기한다(REQ-H04). 없으면 ErrEnrollmentTokenNotFound.
func (r *EnrollmentTokenSQLiteRepository) Revoke(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE enrollment_tokens SET revoked = 1 WHERE id = ?`, id)
	return checkEnrollmentAffected(res, err, "revoke enrollment token")
}

// IncrementUses 는 사용 횟수를 원자적으로 1 증가시킨다(REQ-H07).
//
// max_uses 가 설정되어 있으면 `uses < max_uses` 인 경우에만 증가하는 조건부 UPDATE 로,
// 동시 register 경합에서도 max_uses 를 초과 발급하지 않는다(over-issue 방지). 영향받은
// 행이 0 이면 — 토큰이 존재하지만 소진된 경우(ErrEnrollmentTokenExhausted)와 토큰이
// 아예 없는 경우(ErrEnrollmentTokenNotFound)를 구분하기 위해 존재 여부를 재확인한다.
func (r *EnrollmentTokenSQLiteRepository) IncrementUses(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE enrollment_tokens
		SET uses = uses + 1
		WHERE id = ? AND (max_uses IS NULL OR uses < max_uses)
	`, id)
	if err != nil {
		return fmt.Errorf("increment enrollment token uses: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("increment enrollment token uses rows affected: %w", err)
	}
	if affected == 0 {
		// 행이 없거나(미존재) 소진(uses >= max_uses)이다 — 존재 여부로 구분한다.
		var exists int
		if scanErr := r.db.QueryRowContext(ctx,
			`SELECT 1 FROM enrollment_tokens WHERE id = ?`, id).Scan(&exists); errors.Is(scanErr, sql.ErrNoRows) {
			return ErrEnrollmentTokenNotFound
		}
		return ErrEnrollmentTokenExhausted
	}
	return nil
}

// Delete 는 토큰을 삭제한다. 없으면 ErrEnrollmentTokenNotFound.
func (r *EnrollmentTokenSQLiteRepository) Delete(ctx context.Context, id string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM enrollment_tokens WHERE id = ?`, id)
	return checkEnrollmentAffected(res, err, "delete enrollment token")
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *EnrollmentTokenSQLiteRepository) Close() error {
	return r.db.Close()
}

// checkEnrollmentAffected 는 ExecContext 결과를 검사하여 영향받은 행이 없으면
// ErrEnrollmentTokenNotFound 를 반환한다.
func checkEnrollmentAffected(res sql.Result, execErr error, op string) error {
	if execErr != nil {
		return fmt.Errorf("%s: %w", op, execErr)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("%s rows affected: %w", op, err)
	}
	if affected == 0 {
		return ErrEnrollmentTokenNotFound
	}
	return nil
}

// nullableInt64 는 *int64 를 SQL NULL 가능 값으로 변환한다.
func nullableInt64(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

// nullableInt 는 *int 를 SQL NULL 가능 값으로 변환한다.
func nullableInt(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
