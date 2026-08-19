package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ DashboardSnapshotRepository = (*DashboardSQLiteRepository)(nil)

// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-5)
// DashboardSQLiteRepository 는 레거시 dashboards 스키마 기반의 DashboardSnapshotRepository
// 구현체이다.
//
// 동시성: 모든 메서드는 *sql.DB 의 내부 풀과 트랜잭션 + WAL 모드 (ASM-007) 에
// 의존한다. Put 은 BEGIN IMMEDIATE 로 write lock 을 즉시 획득하여 If-Match 검증과
// UPSERT 가 race-free 하다.
type DashboardSQLiteRepository struct {
	db *sql.DB

	// table 은 이 저장소가 읽고 쓰는 레거시 스냅샷 테이블 이름이다.
	//
	// @SPEC:SPEC-DASHBOARD-004 (M3)
	// 대시보드 엔티티 이관 이후에는 "dashboards" 가 신규 1급 엔티티 테이블의
	// 이름이므로 레거시 저장소는 다른 이름을 쓴다(legacySnapshotTableName).
	// 보존 원본 dashboard_snapshots_v1 은 이 저장소가 건드리지 않는다.
	table string
}

// NewDashboardSQLiteRepository 는 이미 열린 *sql.DB 를 받아 dashboards 스키마를
// 멱등하게 보장한 뒤 저장소를 반환한다.
//
// db 의 수명은 호출자가 관리한다 (Close 책임은 main.go).
func NewDashboardSQLiteRepository(ctx context.Context, db *sql.DB) (*DashboardSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("dashboard sqlite: db must not be nil")
	}
	table, err := legacySnapshotTableName(ctx, db)
	if err != nil {
		return nil, err
	}
	if err := ensureLegacySnapshotTable(ctx, db, table); err != nil {
		return nil, err
	}
	return &DashboardSQLiteRepository{db: db, table: table}, nil
}

// Get 은 (scope, owner) 의 단일 snapshot 을 조회한다.
//
// owner="" 인 경우 DB 상 NULL 과 매칭되어야 하므로 COALESCE(owner,”) 표현식으로 비교한다.
// 이는 dashboards_scope_owner_uidx 부분 유니크 인덱스를 그대로 활용한다.
func (r *DashboardSQLiteRepository) Get(ctx context.Context, scope, owner string) (*DashboardSnapshot, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT scope, COALESCE(owner, ''), version, updated_at, payload
		FROM `+r.table+`
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
		SELECT id, version FROM `+r.table+` WHERE scope = ? AND COALESCE(owner, '') = ?
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
			UPDATE `+r.table+`
			SET version = ?, updated_at = ?, payload = ?
			WHERE id = ?
		`, newVersion, newUpdatedAt, payload, currentID); err != nil {
			return nil, fmt.Errorf("dashboard sqlite put: update: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO `+r.table+`(scope, owner, version, updated_at, payload)
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
		DELETE FROM `+r.table+` WHERE scope = ? AND COALESCE(owner, '') = ?
	`, scope, owner); err != nil {
		return fmt.Errorf("dashboard sqlite delete: %w", err)
	}
	return nil
}

// -----------------------------------------------------------------------------
// 대시보드 1급 엔티티 저장소 (SPEC-DASHBOARD-004)
// -----------------------------------------------------------------------------

// 컴파일 타임 인터페이스 구현 검증.
var _ DashboardRepository = (*DashboardEntitySQLiteRepository)(nil)

// dashboardColumns 는 SELECT 시 payload 를 제외한 컬럼 목록이다.
// SELECT * 대신 명시적 목록을 사용해 컬럼 순서 변화에 영향받지 않게 한다.
const dashboardColumns = `id, uid, name, owner, visibility, is_default, sort_order, version, created_at, updated_at`

// @SPEC:SPEC-DASHBOARD-004 (M2, spec.md §2.1)
// DashboardEntitySQLiteRepository 는 신규 dashboards 스키마 기반의
// DashboardRepository 구현체이다.
//
// 동시성: 레거시 구현과 동일하게 *sql.DB 풀 + 트랜잭션 + WAL 모드(ASM-007)에
// 의존한다. Create / Update / Delete 는 단일 트랜잭션에서 존재 확인과 변경을
// 수행하므로 If-Match 검증과 uid 중복 검사가 race-free 하다.
type DashboardEntitySQLiteRepository struct {
	db *sql.DB
}

// NewDashboardEntitySQLiteRepository 는 이미 열린 *sql.DB 를 받아 신규 대시보드
// 스키마를 멱등하게 보장한 뒤 저장소를 반환한다.
//
// db 가 아직 구 스키마(scope 컬럼 보유)를 갖고 있으면 migrateDashboardSchemaV2 가
// 오류를 반환한다 — 데이터 이관(M3)이 선행되어야 한다.
//
// db 의 수명은 호출자가 관리한다 (Close 책임은 main.go).
func NewDashboardEntitySQLiteRepository(ctx context.Context, db *sql.DB) (*DashboardEntitySQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("dashboard entity sqlite: db must not be nil")
	}
	if err := migrateDashboardSchemaV2(ctx, db); err != nil {
		return nil, err
	}
	return &DashboardEntitySQLiteRepository{db: db}, nil
}

// List 는 전체 대시보드를 sort_order → uid 순으로 반환한다.
//
// includePayload 가 false 이면 payload 컬럼을 아예 SELECT 하지 않는다. 목록
// 응답은 payload 를 포함하지 않으므로(spec.md §2.3) 수백 장 × 최대 256KB 를
// 불필요하게 읽지 않게 한다. 어느 쪽이든 질의는 1회다(spec.md §5 목록 성능).
func (r *DashboardEntitySQLiteRepository) List(ctx context.Context, includePayload bool) ([]Dashboard, error) {
	query := `SELECT ` + dashboardColumns + ` FROM dashboards ORDER BY sort_order ASC, uid ASC`
	if includePayload {
		query = `SELECT ` + dashboardColumns + `, payload FROM dashboards ORDER BY sort_order ASC, uid ASC`
	}

	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite list: %w", err)
	}
	defer rows.Close()

	var out []Dashboard
	for rows.Next() {
		var d Dashboard
		var isDefault int64
		dest := []any{
			&d.ID, &d.UID, &d.Name, &d.Owner, &d.Visibility,
			&isDefault, &d.SortOrder, &d.Version, &d.CreatedAt, &d.UpdatedAt,
		}
		if includePayload {
			dest = append(dest, &d.Payload)
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("dashboard entity sqlite list: scan: %w", err)
		}
		d.IsDefault = isDefault != 0
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite list: iterate: %w", err)
	}
	return out, nil
}

// Get 은 uid 의 대시보드를 payload 와 함께 조회한다. 없으면 ErrDashboardNotFound.
func (r *DashboardEntitySQLiteRepository) Get(ctx context.Context, uid string) (*Dashboard, error) {
	var d Dashboard
	var isDefault int64
	err := r.db.QueryRowContext(ctx,
		`SELECT `+dashboardColumns+`, payload FROM dashboards WHERE uid = ?`, uid).
		Scan(&d.ID, &d.UID, &d.Name, &d.Owner, &d.Visibility,
			&isDefault, &d.SortOrder, &d.Version, &d.CreatedAt, &d.UpdatedAt, &d.Payload)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDashboardNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite get %q: %w", uid, err)
	}
	d.IsDefault = isDefault != 0
	return &d, nil
}

// Create 는 새 대시보드를 생성한다. version 은 1, created_at / updated_at 은
// 서버 시각으로 부여되며 입력의 해당 값은 무시된다(spec.md §2.7).
//
// uid 중복은 트랜잭션 내 사전 조회로 판정해 ErrDashboardUIDExists 를 반환한다.
// UNIQUE 제약을 드라이버 오류 문자열로 식별하지 않는 이유는, 문자열 형식이
// 드라이버 구현 세부사항이라 버전 변경에 취약하기 때문이다. 제약 자체는 최종
// 방어선으로 그대로 남는다.
func (r *DashboardEntitySQLiteRepository) Create(ctx context.Context, d Dashboard) (*Dashboard, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite create: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var existing int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM dashboards WHERE uid = ?`, d.UID).Scan(&existing)
	if err == nil {
		return nil, ErrDashboardUIDExists
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("dashboard entity sqlite create: probe uid: %w", err)
	}

	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO dashboards(uid, name, owner, visibility, is_default, sort_order, version, created_at, updated_at, payload)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	`, d.UID, d.Name, d.Owner, d.Visibility, boolToInt(d.IsDefault), d.SortOrder, now, now, d.Payload)
	if err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite create: insert: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite create: last insert id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite create: commit: %w", err)
	}
	committed = true

	created := d
	created.ID = id
	created.Version = 1
	created.CreatedAt = now
	created.UpdatedAt = now
	return &created, nil
}

// Update 는 uid 의 대시보드를 부분 갱신하고 새 version 을 부여한다.
//
// expectedVersion >= 0 이고 현재 version 과 다르면 ErrDashboardVersionMismatch 를
// 반환하며 트랜잭션이 롤백되어 서버 상태는 변하지 않는다(spec.md §2.13 #10).
func (r *DashboardEntitySQLiteRepository) Update(
	ctx context.Context,
	uid string,
	upd DashboardUpdate,
	expectedVersion int64,
) (*Dashboard, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite update: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var id, currentVersion int64
	err = tx.QueryRowContext(ctx,
		`SELECT id, version FROM dashboards WHERE uid = ?`, uid).Scan(&id, &currentVersion)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDashboardNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite update: select: %w", err)
	}

	if expectedVersion >= 0 && currentVersion != expectedVersion {
		return nil, ErrDashboardVersionMismatch
	}

	newVersion := currentVersion + 1
	newUpdatedAt := time.Now().UnixMilli()

	// nil 이 아닌 필드만 SET 절에 넣는다. 부분 갱신이므로 미지정 필드는 보존된다.
	sets := []string{"version = ?", "updated_at = ?"}
	args := []any{newVersion, newUpdatedAt}
	if upd.Name != nil {
		sets = append(sets, "name = ?")
		args = append(args, *upd.Name)
	}
	if upd.Visibility != nil {
		sets = append(sets, "visibility = ?")
		args = append(args, *upd.Visibility)
	}
	if upd.IsDefault != nil {
		sets = append(sets, "is_default = ?")
		args = append(args, boolToInt(*upd.IsDefault))
	}
	if upd.SortOrder != nil {
		sets = append(sets, "sort_order = ?")
		args = append(args, *upd.SortOrder)
	}
	if upd.Owner != nil {
		sets = append(sets, "owner = ?")
		args = append(args, *upd.Owner)
	}
	if upd.Payload != nil {
		sets = append(sets, "payload = ?")
		args = append(args, upd.Payload)
	}
	args = append(args, id)

	if _, err := tx.ExecContext(ctx,
		`UPDATE dashboards SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite update: exec: %w", err)
	}

	var d Dashboard
	var isDefault int64
	if err := tx.QueryRowContext(ctx,
		`SELECT `+dashboardColumns+`, payload FROM dashboards WHERE id = ?`, id).
		Scan(&d.ID, &d.UID, &d.Name, &d.Owner, &d.Visibility,
			&isDefault, &d.SortOrder, &d.Version, &d.CreatedAt, &d.UpdatedAt, &d.Payload); err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite update: reload: %w", err)
	}
	d.IsDefault = isDefault != 0

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite update: commit: %w", err)
	}
	committed = true
	return &d, nil
}

// Delete 는 uid 의 대시보드와 그에 딸린 ACL 행을 같은 트랜잭션에서 삭제한다.
// 존재하지 않으면 멱등하게 nil 을 반환한다.
//
// ACL 을 명시적으로 지우는 이유: dashboard_acl 의 ON DELETE CASCADE 는
// PRAGMA foreign_keys=ON 일 때만 동작하는데 본 프로젝트는 그 PRAGMA 를 켜지
// 않는다. 삭제된 대시보드의 ACL 행이 남으면 이후 같은 id 가 재사용될 때 유령
// 권한이 되므로, PRAGMA 설정과 무관하게 정리를 보장한다.
func (r *DashboardEntitySQLiteRepository) Delete(ctx context.Context, uid string) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("dashboard entity sqlite delete: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var id int64
	err = tx.QueryRowContext(ctx, `SELECT id FROM dashboards WHERE uid = ?`, uid).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("dashboard entity sqlite delete: select: %w", err)
	}

	if _, err := tx.ExecContext(ctx, `DELETE FROM dashboard_acl WHERE dashboard_id = ?`, id); err != nil {
		return fmt.Errorf("dashboard entity sqlite delete: acl: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM dashboards WHERE id = ?`, id); err != nil {
		return fmt.Errorf("dashboard entity sqlite delete: dashboard: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("dashboard entity sqlite delete: commit: %w", err)
	}
	committed = true
	return nil
}

// boolToInt 는 bool 을 SQLite INTEGER 컬럼 값으로 변환한다.
func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}
