package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ DashboardRepository = (*DashboardSQLiteRepository)(nil)

// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-5)
// DashboardSQLiteRepository 는 dashboards 테이블 기반의 DashboardRepository 구현체이다.
//
// 동시성: 모든 메서드는 *sql.DB 의 내부 풀과 트랜잭션 + WAL 모드 (ASM-007) 에
// 의존한다. Put 은 BEGIN IMMEDIATE 로 write lock 을 즉시 획득하여 If-Match 검증과
// UPSERT 가 race-free 하다.
type DashboardSQLiteRepository struct {
	db *sql.DB
}

// NewDashboardSQLiteRepository 는 이미 열린 *sql.DB 를 받아 dashboards 스키마를
// 멱등하게 보장한 뒤 저장소를 반환한다.
//
// db 의 수명은 호출자가 관리한다 (Close 책임은 main.go).
func NewDashboardSQLiteRepository(ctx context.Context, db *sql.DB) (*DashboardSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("dashboard sqlite: db must not be nil")
	}
	if err := migrateDashboardSchema(ctx, db); err != nil {
		return nil, err
	}
	return &DashboardSQLiteRepository{db: db}, nil
}

// Get 은 (scope, owner) 의 단일 snapshot 을 조회한다.
//
// owner="" 인 경우 DB 상 NULL 과 매칭되어야 하므로 COALESCE(owner,”) 표현식으로 비교한다.
// 이는 dashboards_scope_owner_uidx 부분 유니크 인덱스를 그대로 활용한다.
func (r *DashboardSQLiteRepository) Get(ctx context.Context, scope, owner string) (*DashboardSnapshot, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT scope, COALESCE(owner, ''), version, updated_at, payload
		FROM dashboards
		WHERE scope = ? AND COALESCE(owner, '') = ?
	`, scope, owner)

	var snap DashboardSnapshot
	if err := row.Scan(&snap.Scope, &snap.Owner, &snap.Version, &snap.UpdatedAt, &snap.Payload); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDashboardNotFound
		}
		return nil, fmt.Errorf("dashboard sqlite get: %w", err)
	}
	return &snap, nil
}

// Put 은 If-Match 검증 후 snapshot 을 저장한다.
//
// 트랜잭션 내에서 SELECT (현재 version 확인) → INSERT/UPDATE 순서로 처리한다.
// expectedVersion 값에 따라 다음 분기:
//   - < 0      : unconditional (단순 UPSERT)
//   - >= 0     : SELECT 한 현재 version 과 비교, 불일치 시 ErrDashboardVersionMismatch.
//
// 새 version = old.version + 1, updated_at = time.Now().UnixMilli() 으로 서버가 부여한다.
//
// owner="" 인 경우 NULL 로 저장하여 부분 유니크 인덱스 (COALESCE(owner,”)) 의
// 빈 문자열 정규화와 호환된다.
func (r *DashboardSQLiteRepository) Put(
	ctx context.Context,
	scope, owner string,
	payload []byte,
	expectedVersion int64,
) (*DashboardSnapshot, error) {
	// BEGIN IMMEDIATE 로 write lock 을 즉시 획득 (deferred lock 시 race 가능).
	// 표준 sql 인터페이스는 IMMEDIATE 옵션을 직접 노출하지 않으므로 BeginTx 로 충분.
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("dashboard sqlite put: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 1) 현재 row 확인
	var currentID int64
	var currentVersion int64
	err = tx.QueryRowContext(ctx, `
		SELECT id, version FROM dashboards WHERE scope = ? AND COALESCE(owner, '') = ?
	`, scope, owner).Scan(&currentID, &currentVersion)

	exists := true
	if errors.Is(err, sql.ErrNoRows) {
		exists = false
		currentVersion = 0
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("dashboard sqlite put: select: %w", err)
	}

	// 2) If-Match 검증 (expectedVersion >= 0 인 경우만)
	if expectedVersion >= 0 && currentVersion != expectedVersion {
		// 트랜잭션 롤백되어 상태 미변경. 호출자는 후속 Get 으로 latest 를 받아 응답 body 에 포함.
		return nil, ErrDashboardVersionMismatch
	}

	// 3) 새 version / updated_at 부여
	newVersion := currentVersion + 1
	newUpdatedAt := time.Now().UnixMilli()

	// 4) INSERT 또는 UPDATE
	//    owner=="" 는 DB 상 NULL 로 저장 (부분 유니크 인덱스 호환).
	var dbOwner sql.NullString
	if owner != "" {
		dbOwner = sql.NullString{String: owner, Valid: true}
	}

	if exists {
		if _, err := tx.ExecContext(ctx, `
			UPDATE dashboards
			SET version = ?, updated_at = ?, payload = ?
			WHERE id = ?
		`, newVersion, newUpdatedAt, payload, currentID); err != nil {
			return nil, fmt.Errorf("dashboard sqlite put: update: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO dashboards(scope, owner, version, updated_at, payload)
			VALUES (?, ?, ?, ?, ?)
		`, scope, dbOwner, newVersion, newUpdatedAt, payload); err != nil {
			return nil, fmt.Errorf("dashboard sqlite put: insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("dashboard sqlite put: commit: %w", err)
	}
	committed = true

	return &DashboardSnapshot{
		Scope:     scope,
		Owner:     owner,
		Version:   newVersion,
		UpdatedAt: newUpdatedAt,
		Payload:   payload,
	}, nil
}

// Delete 는 (scope, owner) snapshot 을 삭제한다. 존재하지 않으면 멱등하게 nil 반환.
//
// 명시적 ErrDashboardNotFound 반환 대신 멱등 동작을 선택한 이유:
//   - 핸들러 레이어가 DELETE → 204 No Content 응답을 단순화할 수 있다.
//   - 다중 클라이언트 환경에서 race 로 인한 "이미 삭제됨" 오류를 사용자에게 노출하지 않는다.
//
// 핸들러는 GET 으로 사전 존재 확인을 하지 않아도 안전하다.
func (r *DashboardSQLiteRepository) Delete(ctx context.Context, scope, owner string) error {
	if _, err := r.db.ExecContext(ctx, `
		DELETE FROM dashboards WHERE scope = ? AND COALESCE(owner, '') = ?
	`, scope, owner); err != nil {
		return fmt.Errorf("dashboard sqlite delete: %w", err)
	}
	return nil
}
