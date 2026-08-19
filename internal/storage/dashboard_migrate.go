package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/xtra/xflow/internal/rbac"
)

// @SPEC:SPEC-DASHBOARD-004 (M3, spec.md §2.4)
// dashboard_migrate.go — 구 스냅샷 모델(scope, owner) → 대시보드 1급 엔티티 모델
// 1회성 데이터 이관.
//
// **롤백 불가 지점이다.** 방어는 4겹이다.
//
//  1. 사전 백업 — 이관 직전 VACUUM INTO 로 DB 전체 사본을 <dbpath>.pre-dashboard004
//     에 남긴다. WAL 모드에서 단순 파일 복사는 -wal 파일을 놓쳐 불완전하므로
//     VACUUM INTO 를 쓴다. 백업이 실패하면 이관을 수행하지 않는다 — 복구 수단
//     없이 되돌릴 수 없는 변경을 실행하지 않는다.
//  2. 구 테이블 보존 — dashboard_snapshots_v1 은 **어떤 경로에서도 DROP 하지 않는다.**
//  3. 단일 트랜잭션 — 4단계 전체가 하나의 트랜잭션이다. 부분 실패 시 신규
//     dashboards 는 비어 있고 구 테이블은 온전하며 마커도 없으므로 다음 기동에서
//     그대로 재시도된다.
//  4. 마커 행 — 재실행 판정은 **오직 schema_markers 의 마커 행 존재 여부**다.
//     dashboards 행 수로 판정하면 "관리자가 의도적으로 전부 삭제한 상태" 와
//     "아직 이관하지 않은 상태" 를 구분할 수 없어, 삭제한 대시보드가 재기동마다
//     되살아난다(spec.md §2.4 3상태 표).

const (
	// dashboardEntityMarkerKey 는 대시보드 엔티티 이관 완료를 표시하는 마커 키다.
	dashboardEntityMarkerKey = "dashboard_entity_migrated"

	// dashboardSnapshotsV1Table 은 개명된 구 스냅샷 테이블(= 복구 원본)이다.
	dashboardSnapshotsV1Table = "dashboard_snapshots_v1"

	// dashboardsTable 은 신규 1급 엔티티 테이블이자 구 스냅샷 테이블의 원래 이름이다.
	dashboardsTable = "dashboards"

	// dashboardBackupSuffix 는 이관 사전 백업 파일의 접미사다.
	dashboardBackupSuffix = ".pre-dashboard004"

	// dashboardMigrationVersion 은 이관으로 생성되는 대시보드의 초기 version 이다.
	// 신규 생성(spec.md §2.7)과 동일하게 1에서 시작한다.
	dashboardMigrationVersion = 1
)

// dashboardReservedUIDs 는 라우트 리터럴과 충돌해 uid 로 발급할 수 없는 예약어다
// (spec.md §2.1). 구 데이터에 같은 id 의 페이지가 있으면 접미사로 회피한다.
var dashboardReservedUIDs = []string{"shared", "mine", "state"}

// legacyDashboardPage 는 구 payload 의 dashboardPages 원소
// (web/src/stores/uiStore.ts DashboardPageConfig) 이다.
//
// panels / layout 은 재직렬화 없이 원본 바이트를 그대로 옮긴다 — 패널 설정은
// 타입이 열려 있어(config: Record<string, unknown>) 구조체로 받으면 알 수 없는
// 필드가 조용히 사라진다.
type legacyDashboardPage struct {
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	IsDefault bool            `json:"isDefault"`
	Panels    json.RawMessage `json:"panels"`
	Layout    json.RawMessage `json:"layout"`
}

// legacyDashboardSnapshotPayload 는 구 payload 전체
// (web/src/types/dashboard.ts DashboardPayload) 이다.
//
// 그리드 설정 3종은 원본 키가 dashboard* 접두사를 갖고(dashboardGridCols 등),
// 신규 대시보드 payload 에서는 접두사 없는 이름(gridCols 등)으로 바뀐다
// (spec.md §2.1 vs §2.4). 값은 RawMessage 로 받아 타입 가정 없이 그대로 옮긴다.
type legacyDashboardSnapshotPayload struct {
	DashboardPages    []legacyDashboardPage `json:"dashboardPages"`
	ActiveDashboardID string                `json:"activeDashboardId"`
	GridCols          json.RawMessage       `json:"dashboardGridCols"`
	ShowGridLines     json.RawMessage       `json:"dashboardShowGridLines"`
	RefreshInterval   json.RawMessage       `json:"dashboardRefreshInterval"`
	DeviceGridLayout  json.RawMessage       `json:"deviceGridLayout"`
}

// entityDashboardPayload 는 신규 대시보드 1장의 payload 다 (spec.md §2.1).
type entityDashboardPayload struct {
	Panels          json.RawMessage `json:"panels"`
	Layout          json.RawMessage `json:"layout"`
	GridCols        json.RawMessage `json:"gridCols,omitempty"`
	ShowGridLines   json.RawMessage `json:"showGridLines,omitempty"`
	RefreshInterval json.RawMessage `json:"refreshInterval,omitempty"`
}

// legacyDashboardSnapshotRow 는 구 스냅샷 테이블의 1행이다.
type legacyDashboardSnapshotRow struct {
	ID        int64
	Scope     string
	Owner     string
	UpdatedAt int64
	Payload   []byte
}

// migrateDashboardEntities 는 구 스냅샷 모델을 대시보드 1급 엔티티 모델로 1회
// 이관한다. 재실행 안전(멱등)하다.
//
// 절차는 spec.md §2.4 의 4단계를 그대로 수행하되, 멱등 게이트(3단계)와 사전 백업을
// 개명(1단계)보다 **앞으로** 당긴다.
//
//	0단계. schema_markers 테이블 확보 — 마커를 읽으려면 테이블이 먼저 있어야 한다.
//	3단계. 마커 행이 있으면 신규 테이블만 보장하고 **즉시 반환**
//	       (행 수가 아니라 마커의 존재 여부로 판정 — spec.md §2.4 3상태 표).
//	백업.  VACUUM INTO 로 <dbpath>.pre-dashboard004 생성. 실패하면 중단한다.
//	1단계. dashboards 가 구 스키마(scope 컬럼 보유)이면
//	       ALTER TABLE dashboards RENAME TO dashboard_snapshots_v1.
//	2단계. 신규 테이블 4종을 CREATE TABLE IF NOT EXISTS 로 생성.
//	4단계. 단일 트랜잭션에서 데이터를 옮기고 마커를 삽입한다.
//
// 순서를 당기는 이유는 두 가지다.
//   - 백업이 개명보다 앞서야 백업 파일이 **손대지 않은 이관 이전 DB** 가 된다.
//     개명 이후에 뜨면 복구할 때 테이블 이름을 되돌리는 절차가 추가로 필요하다.
//   - 마커 확인이 개명보다 앞서야 이미 이관을 마친 DB 를 매 기동마다 다시 훑지 않는다.
//
// 신규 설치(구 dashboards 테이블 자체가 없음)에서도 정상 동작한다 — 신규 테이블을
// 만들고 마커만 기록한 뒤 아무것도 이관하지 않는다.
func migrateDashboardEntities(ctx context.Context, db *sql.DB) error {
	// --- 0단계: 마커 테이블 확보 -------------------------------------------
	if err := migrateSchemaMarkersSchema(ctx, db); err != nil {
		return err
	}

	// --- 3단계: 멱등 게이트 (마커 행의 존재 여부만으로 판정) ----------------
	migrated, err := dashboardEntityMarkerExists(ctx, db)
	if err != nil {
		return err
	}
	if migrated {
		// 이관은 끝났다. 신규 테이블만 멱등하게 보장하고 데이터는 건드리지 않는다.
		return migrateDashboardSchemaV2(ctx, db)
	}

	// --- 사전 백업: 복구 수단 없이 롤백 불가 변경을 실행하지 않는다 --------
	if err := backupDatabaseOnce(ctx, db, dashboardBackupSuffix); err != nil {
		return fmt.Errorf("migrate dashboard entities: pre-migration backup failed, aborting: %w", err)
	}

	// --- 1단계: 구 스키마 판정 후 개명 -------------------------------------
	legacy, err := hasLegacyDashboardSchema(ctx, db)
	if err != nil {
		return err
	}
	if legacy {
		archived, err := tableExists(ctx, db, dashboardSnapshotsV1Table)
		if err != nil {
			return err
		}
		if archived {
			// 구 dashboards 와 보존본이 동시에 존재하는 상태는 설계상 나올 수 없다.
			// 자동으로 합치거나 덮어쓰면 어느 쪽이 원본인지 판단할 수 없으므로
			// 조용히 진행하지 않고 멈춘다 — 운영자가 확인해야 한다.
			return fmt.Errorf(
				"migrate dashboard entities: both %q (legacy schema) and %q exist; refusing to overwrite the preserved snapshot table",
				dashboardsTable, dashboardSnapshotsV1Table)
		}
		if _, err := db.ExecContext(ctx,
			`ALTER TABLE `+dashboardsTable+` RENAME TO `+dashboardSnapshotsV1Table); err != nil {
			return fmt.Errorf("migrate dashboard entities: rename legacy dashboards table: %w", err)
		}
	}

	// --- 2단계: 신규 테이블 생성 -------------------------------------------
	if err := migrateDashboardSchemaV2(ctx, db); err != nil {
		return err
	}

	// --- 4단계: 단일 트랜잭션 이관 -----------------------------------------
	return runDashboardEntityMigration(ctx, db)
}

// dashboardEntityMarkerExists 는 이관 완료 마커 행의 존재 여부를 반환한다.
//
// **행 수로 판정하지 않는다.** spec.md §2.4 의 3상태 표가 요구하는 구분
// ("이관 안 됨" vs "이관 후 관리자가 전부 삭제") 은 마커로만 표현된다.
func dashboardEntityMarkerExists(ctx context.Context, db *sql.DB) (bool, error) {
	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_markers WHERE key = ?`, dashboardEntityMarkerKey).Scan(&n); err != nil {
		return false, fmt.Errorf("read dashboard entity marker: %w", err)
	}
	return n > 0, nil
}

// tableExists 는 name 테이블의 존재 여부를 반환한다.
func tableExists(ctx context.Context, db *sql.DB, name string) (bool, error) {
	var n int64
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&n); err != nil {
		return false, fmt.Errorf("inspect table %q: %w", name, err)
	}
	return n > 0, nil
}

// mainDatabaseFilePath 는 연결이 실제로 사용 중인 main 데이터베이스 파일 경로를
// 반환한다. 인메모리 DB 이면 빈 문자열이다.
//
// 호출자가 넘긴 경로 대신 연결에게 직접 묻는 이유: 백업은 반드시 "지금 이관하려는
// 그 파일" 의 사본이어야 한다. 경로를 인자로 받으면 호출부가 늘어날수록 어긋날
// 여지가 생긴다.
func mainDatabaseFilePath(ctx context.Context, db *sql.DB) (string, error) {
	rows, err := db.QueryContext(ctx, `PRAGMA database_list`)
	if err != nil {
		return "", fmt.Errorf("read database list: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var seq int64
		var name string
		var file sql.NullString
		if err := rows.Scan(&seq, &name, &file); err != nil {
			return "", fmt.Errorf("scan database list: %w", err)
		}
		if name == "main" {
			return file.String, nil
		}
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("iterate database list: %w", err)
	}
	return "", nil
}

// backupDatabaseOnce 는 이관 직전 DB 전체 사본을 <dbpath><suffix> 로 남긴다.
//
// VACUUM INTO 를 쓰는 이유: WAL 모드에서는 최신 커밋이 -wal 파일에만 있을 수 있어
// 단순 파일 복사가 불완전하다. VACUUM INTO 는 일관된 스냅샷을 단일 파일로 쓴다.
//
// 이미 백업 파일이 있으면 **덮어쓰지 않는다.** 이전 시도의 백업이 지금 만드는
// 백업보다 원본에 가깝다 — 재시도 상황에서 앞선 백업을 지우면 복구 원본을 잃는다.
//
// 인메모리 DB(파일 경로 없음)는 보호할 파일이 없으므로 건너뛴다.
func backupDatabaseOnce(ctx context.Context, db *sql.DB, suffix string) error {
	path, err := mainDatabaseFilePath(ctx, db)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	target := path + suffix
	switch info, statErr := os.Stat(target); {
	case statErr == nil && info.Mode().IsRegular():
		// 이전 시도의 백업을 보존한다.
		return nil
	case statErr == nil:
		// 디렉토리 등 파일이 아닌 것이 자리를 차지하고 있으면 백업을 만들 수 없다.
		// 조용히 넘어가면 복구 원본 없이 이관이 진행된다.
		return fmt.Errorf("backup target %q exists but is not a regular file", target)
	case !errors.Is(statErr, os.ErrNotExist):
		return fmt.Errorf("stat backup target %q: %w", target, statErr)
	}

	if _, err := db.ExecContext(ctx, `VACUUM INTO ?`, target); err != nil {
		return fmt.Errorf("vacuum into %q: %w", target, err)
	}
	return nil
}

// runDashboardEntityMigration 은 spec.md §2.4 4단계를 단일 트랜잭션으로 수행한다.
//
// 트랜잭션 경계가 이 함수 전체다. 중간에 실패하면 신규 dashboards 는 비고,
// dashboard_snapshots_v1 은 온전하며, 마커도 기록되지 않아 다음 기동에서 재시도된다.
func runDashboardEntityMigration(ctx context.Context, db *sql.DB) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin dashboard entity migration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	snapshots, err := readLegacyDashboardSnapshots(ctx, tx)
	if err != nil {
		return err
	}
	adminOwner, err := firstAdminUsername(ctx, tx)
	if err != nil {
		return err
	}

	used, err := reservedAndExistingUIDs(ctx, tx)
	if err != nil {
		return err
	}

	// 전역 스냅샷을 먼저 처리한다. uid 충돌 시 접미사를 받는 쪽이 개인 대시보드여야
	// 하므로(spec.md §2.4), 전역이 원본 uid 를 선점해야 한다.
	for _, snap := range snapshots {
		if snap.Scope != "global" {
			continue
		}
		payload, err := decodeLegacySnapshotPayload(snap)
		if err != nil {
			return err
		}
		// 전역 스냅샷의 activeDashboardId / deviceGridLayout 은 버린다
		// (spec.md §2.4 — 어느 사용자의 UI 상태인지 정의되지 않는다).
		if _, err := insertMigratedDashboards(ctx, tx, snap, payload,
			adminOwner, dashboardaclVisibilityShared, "", used); err != nil {
			return err
		}
	}

	for _, snap := range snapshots {
		if snap.Scope == "global" {
			continue
		}
		payload, err := decodeLegacySnapshotPayload(snap)
		if err != nil {
			return err
		}
		// 개인 대시보드가 전역과 uid 를 공유하면 "-<username>" 접미사를 받는다.
		remap, err := insertMigratedDashboards(ctx, tx, snap, payload,
			snap.Owner, dashboardaclVisibilityPrivate, snap.Owner, used)
		if err != nil {
			return err
		}
		if err := insertMigratedUserState(ctx, tx, snap, payload, remap); err != nil {
			return err
		}
	}

	// 마커는 같은 트랜잭션의 마지막에 삽입한다. key 가 PRIMARY KEY 이므로 경쟁
	// 상황에서 두 번 기록되면 트랜잭션이 통째로 실패해 중복 이관이 차단된다.
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO schema_markers(key, value, applied_at) VALUES (?, ?, ?)`,
		dashboardEntityMarkerKey, "1", time.Now().UnixMilli()); err != nil {
		return fmt.Errorf("insert dashboard entity marker: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit dashboard entity migration: %w", err)
	}
	committed = true
	return nil
}

// visibility 리터럴. internal/dashboardacl 을 import 하면 storage → dashboardacl
// 의존이 생기므로(현재는 api 계층만 참조) 값만 로컬 상수로 둔다. 값 자체는
// dashboards.visibility CHECK 제약과 동일하다.
const (
	dashboardaclVisibilityPrivate = "private"
	dashboardaclVisibilityShared  = "shared"
)

// readLegacyDashboardSnapshots 는 보존된 구 스냅샷 테이블을 id 오름차순으로 읽는다.
// 테이블이 없으면 빈 결과다 (신규 설치).
func readLegacyDashboardSnapshots(ctx context.Context, tx *sql.Tx) ([]legacyDashboardSnapshotRow, error) {
	var n int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`,
		dashboardSnapshotsV1Table).Scan(&n); err != nil {
		return nil, fmt.Errorf("inspect %s: %w", dashboardSnapshotsV1Table, err)
	}
	if n == 0 {
		return nil, nil
	}

	rows, err := tx.QueryContext(ctx, `
		SELECT id, scope, COALESCE(owner, ''), updated_at, payload
		FROM `+dashboardSnapshotsV1Table+`
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dashboardSnapshotsV1Table, err)
	}
	defer rows.Close()

	var out []legacyDashboardSnapshotRow
	for rows.Next() {
		var r legacyDashboardSnapshotRow
		if err := rows.Scan(&r.ID, &r.Scope, &r.Owner, &r.UpdatedAt, &r.Payload); err != nil {
			return nil, fmt.Errorf("scan %s row: %w", dashboardSnapshotsV1Table, err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", dashboardSnapshotsV1Table, err)
	}
	return out, nil
}

// firstAdminUsername 은 전역 스냅샷의 소유자로 삼을 "최초 admin 사용자" 를 결정한다.
//
// **정렬을 명시한다.** users.id 는 AUTOINCREMENT 이므로 오름차순 첫 행이 가장 먼저
// 만들어진 admin 이다. SQLite 가 돌려주는 순서에 기대면 같은 DB 에서도 결과가
// 달라질 수 있어 이관이 재현 불가능해진다.
//
// admin 사용자가 없으면 빈 문자열을 반환한다 — 부팅을 실패시키지 않는다.
// 빈 소유자는 인증 비활성 배포의 소유자 표기(spec.md §2.10)와 같은 값이며,
// 해당 대시보드는 visibility='shared' 로 남아 계속 조회 가능하다
// (acceptance.md 엣지 케이스 "이관 시 admin 사용자가 존재하지 않음").
//
// users 테이블 자체가 없는 DB 도 admin 없음으로 취급한다.
func firstAdminUsername(ctx context.Context, tx *sql.Tx) (string, error) {
	var n int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='users'`).Scan(&n); err != nil {
		return "", fmt.Errorf("inspect users table: %w", err)
	}
	if n == 0 {
		return "", nil
	}

	var username string
	err := tx.QueryRowContext(ctx,
		`SELECT username FROM users WHERE role = ? ORDER BY id ASC LIMIT 1`, rbac.RoleAdmin).Scan(&username)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve first admin user: %w", err)
	}
	return username, nil
}

// reservedAndExistingUIDs 는 새로 발급할 수 없는 uid 집합을 만든다.
//
// 예약어(shared/mine/state)는 라우트 리터럴과 충돌하고(spec.md §2.1), 이미
// dashboards 에 있는 uid 는 UNIQUE 제약에 걸린다. 둘 다 선점된 것으로 취급하면
// 접미사 규칙이 자동으로 회피한다.
func reservedAndExistingUIDs(ctx context.Context, tx *sql.Tx) (map[string]struct{}, error) {
	used := make(map[string]struct{}, len(dashboardReservedUIDs)+8)
	for _, uid := range dashboardReservedUIDs {
		used[uid] = struct{}{}
	}

	rows, err := tx.QueryContext(ctx, `SELECT uid FROM `+dashboardsTable)
	if err != nil {
		return nil, fmt.Errorf("read existing dashboard uids: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan existing dashboard uid: %w", err)
		}
		used[uid] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate existing dashboard uids: %w", err)
	}
	return used, nil
}

// decodeLegacySnapshotPayload 는 구 payload JSON 을 해석한다.
//
// 해석에 실패하면 이관 전체를 중단한다. 해당 스냅샷만 건너뛰면 그 사용자의
// 대시보드가 조용히 사라지고, 마커가 기록되어 다시는 이관되지 않는다 —
// 롤백 불가 지점에서 가장 나쁜 결과다. 부팅을 멈추고 운영자가 원본을 확인하게 한다.
func decodeLegacySnapshotPayload(snap legacyDashboardSnapshotRow) (legacyDashboardSnapshotPayload, error) {
	var payload legacyDashboardSnapshotPayload
	if len(snap.Payload) == 0 {
		return payload, nil
	}
	if err := json.Unmarshal(snap.Payload, &payload); err != nil {
		return payload, fmt.Errorf(
			"migrate dashboard entities: decode payload of %s row id=%d (scope=%q owner=%q): %w",
			dashboardSnapshotsV1Table, snap.ID, snap.Scope, snap.Owner, err)
	}
	return payload, nil
}

// insertMigratedDashboards 는 스냅샷 1건의 dashboardPages 를 대시보드 N 행으로
// 삽입하고, 원본 페이지 id → 확정 uid 매핑을 반환한다.
//
// uidSuffix 가 비어 있지 않으면 uid 충돌 시 "-<uidSuffix>" 를 붙인다.
func insertMigratedDashboards(
	ctx context.Context,
	tx *sql.Tx,
	snap legacyDashboardSnapshotRow,
	payload legacyDashboardSnapshotPayload,
	owner string,
	visibility string,
	uidSuffix string,
	used map[string]struct{},
) (map[string]string, error) {
	remap := make(map[string]string, len(payload.DashboardPages))

	// created_at / updated_at 은 원본 스냅샷의 updated_at 을 승계한다. 이관 시각으로
	// 덮으면 "언제부터 있던 대시보드인가" 라는 정보가 사라진다. 원본이 0(미기록)
	// 이면 현재 시각으로 대체한다 — NOT NULL 컬럼에 의미 없는 0을 남기지 않는다.
	ts := snap.UpdatedAt
	if ts <= 0 {
		ts = time.Now().UnixMilli()
	}

	for idx, page := range payload.DashboardPages {
		uid := allocateDashboardUID(page.ID, uidSuffix, used)
		remap[page.ID] = uid

		body, err := buildEntityPayload(page, payload)
		if err != nil {
			return nil, fmt.Errorf(
				"migrate dashboard entities: build payload for %s row id=%d page %d: %w",
				dashboardSnapshotsV1Table, snap.ID, idx, err)
		}

		isDefault := int64(0)
		if page.IsDefault {
			isDefault = 1
		}

		if _, err := tx.ExecContext(ctx, `
			INSERT INTO `+dashboardsTable+`
				(uid, name, owner, visibility, is_default, sort_order, version, created_at, updated_at, payload)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, uid, page.Name, owner, visibility, isDefault, int64(idx),
			int64(dashboardMigrationVersion), ts, ts, string(body)); err != nil {
			return nil, fmt.Errorf(
				"migrate dashboard entities: insert dashboard uid=%q (from %s row id=%d): %w",
				uid, dashboardSnapshotsV1Table, snap.ID, err)
		}
	}
	return remap, nil
}

// allocateDashboardUID 는 아직 쓰이지 않은 uid 를 확정하고 used 에 등록한다.
//
// 순서:
//  1. 원본 id 를 그대로 쓴다.
//  2. 이미 선점되었고 suffix 가 있으면 "<id>-<suffix>" (spec.md §2.4 충돌 규칙).
//  3. 그래도 겹치면 뒤에 일련번호를 붙인다. 여기까지 오는 것은 비정상이지만,
//     UNIQUE 제약으로 이관 전체를 실패시키는 것보다 대시보드를 보존하는 편이 낫다.
//
// 원본 id 가 비어 있으면 "dashboard" 를 기준으로 발급한다. 빈 uid 는 라우팅이
// 불가능해 대시보드가 사실상 사라지기 때문이다.
func allocateDashboardUID(base, suffix string, used map[string]struct{}) string {
	if base == "" {
		base = "dashboard"
	}

	take := func(candidate string) (string, bool) {
		if _, taken := used[candidate]; taken {
			return "", false
		}
		used[candidate] = struct{}{}
		return candidate, true
	}

	if uid, ok := take(base); ok {
		return uid
	}
	if suffix != "" {
		if uid, ok := take(base + "-" + suffix); ok {
			return uid
		}
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if suffix != "" {
			candidate = fmt.Sprintf("%s-%s-%d", base, suffix, n)
		}
		if uid, ok := take(candidate); ok {
			return uid
		}
	}
}

// buildEntityPayload 는 페이지 1장의 신규 payload 를 만든다 (spec.md §2.1).
//
// 그리드 설정 3종은 **출처 스냅샷의 값** 을 각 대시보드에 복사한다. 구 모델에서는
// 묶음 단위로 1벌만 있었으므로, 같은 스냅샷에서 나온 대시보드들은 같은 값을 갖는다.
func buildEntityPayload(page legacyDashboardPage, src legacyDashboardSnapshotPayload) ([]byte, error) {
	body := entityDashboardPayload{
		Panels:          emptyArrayIfNull(page.Panels),
		Layout:          emptyArrayIfNull(page.Layout),
		GridCols:        nullToNil(src.GridCols),
		ShowGridLines:   nullToNil(src.ShowGridLines),
		RefreshInterval: nullToNil(src.RefreshInterval),
	}
	return json.Marshal(body)
}

// emptyArrayIfNull 은 없거나 JSON null 인 값을 빈 배열로 정규화한다.
// panels / layout 은 클라이언트가 항상 배열로 기대한다.
func emptyArrayIfNull(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(`[]`)
	}
	return raw
}

// nullToNil 은 없거나 JSON null 인 값을 nil 로 만들어 결과 JSON 에서 키를 생략한다
// (omitempty). 원본에 없던 설정을 이관이 지어내지 않는다.
func nullToNil(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	return raw
}

// insertMigratedUserState 는 개인 스냅샷의 activeDashboardId / deviceGridLayout 을
// dashboard_user_state 로 옮긴다 (spec.md §2.1, acceptance.md AC-14).
//
// active_dashboard_uid 는 remap 을 거친다 — uid 충돌로 접미사가 붙었으면 그 사용자의
// 활성 대시보드 참조도 같은 트랜잭션에서 함께 보정되어야 한다(spec.md §2.4).
// 매핑에 없는 값(예: 전역 대시보드를 보고 있었던 경우)은 그대로 둔다. 존재하지 않는
// uid 의 정규화는 조회 시점의 책임이다(spec.md §2.13 #11).
func insertMigratedUserState(
	ctx context.Context,
	tx *sql.Tx,
	snap legacyDashboardSnapshotRow,
	payload legacyDashboardSnapshotPayload,
	remap map[string]string,
) error {
	activeUID := payload.ActiveDashboardID
	if mapped, ok := remap[activeUID]; ok {
		activeUID = mapped
	}

	layout := emptyObjectIfNull(payload.DeviceGridLayout)

	ts := snap.UpdatedAt
	if ts <= 0 {
		ts = time.Now().UnixMilli()
	}

	// 구 유니크 인덱스가 사용자당 1행을 보장하므로 충돌은 나오지 않아야 한다.
	// 그럼에도 ON CONFLICT 를 두는 이유는, 인덱스가 없는 손상된 DB 에서 UI 상태
	// 1건 때문에 대시보드 이관 전체가 실패하는 것을 막기 위해서다.
	if _, err := tx.ExecContext(ctx, `
		INSERT INTO dashboard_user_state
			(username, active_dashboard_uid, device_grid_layout, version, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET
			active_dashboard_uid = excluded.active_dashboard_uid,
			device_grid_layout   = excluded.device_grid_layout,
			version              = dashboard_user_state.version + 1,
			updated_at           = excluded.updated_at
	`, snap.Owner, activeUID, string(layout), int64(dashboardMigrationVersion), ts); err != nil {
		return fmt.Errorf(
			"migrate dashboard entities: insert user state for %q (from %s row id=%d): %w",
			snap.Owner, dashboardSnapshotsV1Table, snap.ID, err)
	}
	return nil
}

// emptyObjectIfNull 은 없거나 JSON null 인 값을 빈 객체로 정규화한다.
// device_grid_layout 컬럼은 NOT NULL DEFAULT '{}' 이다.
func emptyObjectIfNull(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage(emptyDeviceGridLayout)
	}
	return raw
}
