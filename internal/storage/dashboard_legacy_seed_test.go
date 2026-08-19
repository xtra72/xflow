package storage

import (
	"context"
	"database/sql"
	"fmt"
)

// @SPEC:SPEC-DASHBOARD-004 (M4)
// dashboard_legacy_seed_test.go — 구 (scope, owner) 스냅샷 스키마 시드 (테스트 전용).
//
// M4 가 레거시 묶음 쓰기 경로를 제거하면서 운영 코드에서 이 스키마를 만들 이유가
// 사라졌다. 그러나 이관(migrateDashboardEntities)은 여전히 구 스키마를 입력으로
// 받으므로, 이관 테스트가 그 입력을 만들어야 한다. 운영 코드에 쓰이지 않는 DDL 을
// 운영 파일에 남기면 "아직 쓰는 데가 있나" 라는 오해를 만들므로 테스트 쪽으로 옮긴다.

// seedLegacyDashboardSchema 는 "dashboards" 이름으로 구 스냅샷 스키마를 만든다.
func seedLegacyDashboardSchema(ctx context.Context, db *sql.DB) error {
	return seedLegacyDashboardTable(ctx, db, dashboardsTable)
}

// seedLegacyDashboardTable 은 지정한 이름으로 구 스냅샷 스키마를 멱등하게 만든다.
//
// (scope, COALESCE(owner, ”)) 부분 유니크: scope=global+NULL 은 빈 문자열로
// 정규화되어 단일 row 만 허용되고, scope=user+owner 별로 1개씩 허용된다.
func seedLegacyDashboardTable(ctx context.Context, db *sql.DB, table string) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS `+table+` (
		id         INTEGER PRIMARY KEY AUTOINCREMENT,
		scope      TEXT    NOT NULL CHECK (scope IN ('global', 'user')),
		owner      TEXT,
		version    INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL,
		payload    TEXT    NOT NULL
	)`); err != nil {
		return fmt.Errorf("create %s table: %w", table, err)
	}

	if _, err := db.ExecContext(ctx, `CREATE UNIQUE INDEX IF NOT EXISTS `+table+`_scope_owner_uidx
		ON `+table+`(scope, COALESCE(owner, ''))`); err != nil {
		return fmt.Errorf("create %s index: %w", table, err)
	}
	return nil
}
