package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// @SPEC:SPEC-DASHBOARD-004 (M4, spec.md §2.13 UB1 #7, acceptance.md AC-20)
// dashboard_owner.go — 사용자 삭제 시 대시보드 소유권 정리.
//
// 사용자 삭제 경로(internal/api/handler/user.go)가 소비한다. 저장소 계층에 두는
// 이유는 users 삭제와 대시보드 소유권 승계가 같은 DB 핸들 위의 SQL 이기 때문이다 —
// 핸들러에 raw SQL 을 두면 users/roles 경로가 storage.* 함수를 쓰는 기존 규칙이
// 대시보드만 예외가 된다.
//
// dashboards.owner 는 users 를 참조하는 FK 가 아니다(인증 비활성 배포의 owner=''
// 를 허용해야 하므로). 따라서 고아 방지는 삭제 경로의 명시적 승계로만 성립한다.

// CountDashboardsByOwner 는 owner 가 소유한 대시보드 수를 반환한다.
func CountDashboardsByOwner(ctx context.Context, db *sql.DB, owner string) (int, error) {
	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM dashboards WHERE owner = ?`, owner).Scan(&n); err != nil {
		return 0, fmt.Errorf("count dashboards by owner %q: %w", owner, err)
	}
	return n, nil
}

// TransferDashboardOwnership 은 from 이 소유한 대시보드를 to 에게 승계하고,
// from 을 대상으로 하던 ACL 행(`user:<from>`)을 제거한다.
//
// 두 변경은 단일 트랜잭션이다. 승계만 되고 ACL 이 남으면 삭제된 사용자의 유령
// 권한 행이 영속하고, ACL 만 지워지고 승계가 실패하면 대시보드가 고아가 된다.
//
// 반환값은 승계된 대시보드 수이다.
func TransferDashboardOwnership(ctx context.Context, db *sql.DB, from, to string) (int64, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("transfer dashboard ownership: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	res, err := tx.ExecContext(ctx,
		`UPDATE dashboards SET owner = ? WHERE owner = ?`, to, from)
	if err != nil {
		return 0, fmt.Errorf("transfer dashboard ownership %q -> %q: %w", from, to, err)
	}
	moved, err := res.RowsAffected()
	if err != nil {
		moved = 0
	}

	if _, err := tx.ExecContext(ctx,
		`DELETE FROM dashboard_acl WHERE subject = ?`, "user:"+from); err != nil {
		return 0, fmt.Errorf("delete dashboard acl for %q: %w", from, err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("transfer dashboard ownership: commit: %w", err)
	}
	committed = true
	return moved, nil
}

// DeleteDashboardACLByRole 은 삭제된 역할을 대상으로 하던 ACL 행을 제거한다.
//
// 판정 시 존재하지 않는 역할은 매치되지 않아 무해하지만(internal/dashboardacl
// matchesSubject), 같은 이름의 역할이 나중에 다시 만들어지면 의도치 않게 되살아난다.
func DeleteDashboardACLByRole(ctx context.Context, db *sql.DB, role string) error {
	if _, err := db.ExecContext(ctx,
		`DELETE FROM dashboard_acl WHERE subject = ?`, "role:"+role); err != nil {
		return fmt.Errorf("delete dashboard acl for role %q: %w", role, err)
	}
	return nil
}
