package storage

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// @SPEC:SPEC-DASHBOARD-004 (M2, spec.md §2.1, §4.5)
// dashboard_acl_sqlite.go — dashboard_acl 테이블 저장소.
//
// 스키마 생성은 sqlite.go 의 migrateDashboardSchemaV2 가 담당한다. 본 파일은
// 조회·치환만 제공하며, subject 형식·실재 검증(존재하지 않는 사용자·역할 거부,
// 소유자 자기 등재 거부)은 상위 API 계층이 강제한다 — roles_sqlite.go 가 잠금
// 방지 불변식을 상위에 맡기는 것과 같은 역할 분리다.

// 컴파일 타임 인터페이스 구현 검증.
var _ DashboardACLRepository = (*DashboardACLSQLiteRepository)(nil)

// DashboardACLEntry 는 dashboard_acl 테이블의 단일 행이다.
//
// Subject 형식은 "user:<username>" 또는 "role:<rolename>" 이다
// (internal/dashboardacl 의 SubjectPrefixUser / SubjectPrefixRole).
// GrantedBy / GrantedAt 은 Replace 시 서버가 부여하므로, 조회 결과에서만
// 의미 있는 값이다.
type DashboardACLEntry struct {
	DashboardID int64
	Subject     string
	Level       string // "view" | "edit"
	GrantedBy   string
	GrantedAt   int64 // epoch milliseconds
}

// DashboardACLRepository 는 대시보드 권한 부여 대상의 영속 저장소이다.
type DashboardACLRepository interface {
	// ListByDashboard 는 단일 대시보드의 ACL 을 subject 사전순으로 반환한다.
	ListByDashboard(ctx context.Context, dashboardID int64) ([]DashboardACLEntry, error)

	// ListBySubjects 는 주어진 subject 들에 매치되는 모든 ACL 행을 반환한다.
	//
	// 목록 조회 시 대시보드 행당 추가 질의 없이 인가를 판정하기 위한 단일 질의다
	// (spec.md §5 — 질의 2회 이내). subjects 가 비어 있으면 빈 결과를 반환한다.
	ListBySubjects(ctx context.Context, subjects []string) ([]DashboardACLEntry, error)

	// Replace 는 대시보드의 ACL 을 entries 로 **전량 치환**한다.
	//
	// 부분 갱신은 제공하지 않는다 — 클라이언트가 화면 목록과 서버 상태의 차이를
	// 계산해야 하고, 두 관리자가 동시에 편집하면 중간 상태가 커밋된다(spec.md §4.5).
	//
	// GrantedAt 은 요청 값과 무관하게 서버 시각으로 기록된다(spec.md §2.9).
	// entries 가 비어 있으면 전량 삭제와 같다.
	Replace(ctx context.Context, dashboardID int64, entries []DashboardACLEntry) error

	// DeleteByDashboard 는 대시보드의 ACL 을 전부 삭제한다 (멱등).
	DeleteByDashboard(ctx context.Context, dashboardID int64) error
}

// DashboardACLSQLiteRepository 는 dashboard_acl 테이블 기반의 구현체이다.
type DashboardACLSQLiteRepository struct {
	db *sql.DB
}

// NewDashboardACLSQLiteRepository 는 이미 열린 *sql.DB 를 받아 신규 대시보드
// 스키마를 멱등하게 보장한 뒤 저장소를 반환한다.
//
// db 의 수명은 호출자가 관리한다 (Close 책임은 main.go).
func NewDashboardACLSQLiteRepository(ctx context.Context, db *sql.DB) (*DashboardACLSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("dashboard acl sqlite: db must not be nil")
	}
	if err := migrateDashboardSchemaV2(ctx, db); err != nil {
		return nil, err
	}
	return &DashboardACLSQLiteRepository{db: db}, nil
}

// ListByDashboard 는 단일 대시보드의 ACL 을 subject 사전순으로 반환한다.
func (r *DashboardACLSQLiteRepository) ListByDashboard(ctx context.Context, dashboardID int64) ([]DashboardACLEntry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT dashboard_id, subject, level, granted_by, granted_at
		FROM dashboard_acl WHERE dashboard_id = ? ORDER BY subject ASC
	`, dashboardID)
	if err != nil {
		return nil, fmt.Errorf("dashboard acl sqlite list by dashboard: %w", err)
	}
	defer rows.Close()
	return scanDashboardACLRows(rows)
}

// ListBySubjects 는 subjects 중 하나에 매치되는 모든 ACL 행을 반환한다.
func (r *DashboardACLSQLiteRepository) ListBySubjects(ctx context.Context, subjects []string) ([]DashboardACLEntry, error) {
	if len(subjects) == 0 {
		return nil, nil
	}

	placeholders := make([]string, len(subjects))
	args := make([]any, len(subjects))
	for i, s := range subjects {
		placeholders[i] = "?"
		args[i] = s
	}
	query := `SELECT dashboard_id, subject, level, granted_by, granted_at
		FROM dashboard_acl WHERE subject IN (` + strings.Join(placeholders, ",") + `)
		ORDER BY dashboard_id ASC, subject ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard acl sqlite list by subjects: %w", err)
	}
	defer rows.Close()
	return scanDashboardACLRows(rows)
}

// Replace 는 단일 트랜잭션에서 기존 행 전량 삭제 후 entries 를 삽입한다.
//
// 삭제와 삽입이 같은 트랜잭션에 있으므로 중간 상태(권한이 전부 사라진 순간)가
// 다른 요청에 관측되지 않는다.
func (r *DashboardACLSQLiteRepository) Replace(ctx context.Context, dashboardID int64, entries []DashboardACLEntry) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("dashboard acl sqlite replace: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM dashboard_acl WHERE dashboard_id = ?`, dashboardID); err != nil {
		return fmt.Errorf("dashboard acl sqlite replace: clear: %w", err)
	}

	now := time.Now().UnixMilli()
	for _, e := range entries {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO dashboard_acl(dashboard_id, subject, level, granted_by, granted_at)
			VALUES (?, ?, ?, ?, ?)
		`, dashboardID, e.Subject, e.Level, e.GrantedBy, now); err != nil {
			return fmt.Errorf("dashboard acl sqlite replace: insert %q: %w", e.Subject, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dashboard acl sqlite replace: commit: %w", err)
	}
	committed = true
	return nil
}

// DeleteByDashboard 는 대시보드의 ACL 을 전부 삭제한다 (멱등).
func (r *DashboardACLSQLiteRepository) DeleteByDashboard(ctx context.Context, dashboardID int64) error {
	if _, err := r.db.ExecContext(ctx,
		`DELETE FROM dashboard_acl WHERE dashboard_id = ?`, dashboardID); err != nil {
		return fmt.Errorf("dashboard acl sqlite delete by dashboard: %w", err)
	}
	return nil
}

// scanDashboardACLRows 는 조회 결과를 DashboardACLEntry 슬라이스로 변환한다.
func scanDashboardACLRows(rows *sql.Rows) ([]DashboardACLEntry, error) {
	var out []DashboardACLEntry
	for rows.Next() {
		var e DashboardACLEntry
		if err := rows.Scan(&e.DashboardID, &e.Subject, &e.Level, &e.GrantedBy, &e.GrantedAt); err != nil {
			return nil, fmt.Errorf("scan dashboard acl row: %w", err)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard acl rows: %w", err)
	}
	return out, nil
}
