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
var _ DashboardRepository = (*DashboardEntitySQLiteRepository)(nil)

// dashboardColumns 는 SELECT 시 payload 를 제외한 컬럼 목록이다.
// SELECT * 대신 명시적 목록을 사용해 컬럼 순서 변화에 영향받지 않게 한다.
const dashboardColumns = `id, uid, name, owner, visibility, is_default, sort_order, version, created_at, updated_at`

// @SPEC:SPEC-DASHBOARD-004 (M2, spec.md §2.1)
// DashboardEntitySQLiteRepository 는 신규 dashboards 스키마 기반의
// DashboardRepository 구현체이다.
//
// 동시성: *sql.DB 풀 + 트랜잭션 + WAL 모드(ASM-007)에
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
//
// sort_order 는 **소유자별 시퀀스**로 서버가 부여한다(입력값 무시).
// 같은 소유자의 기존 최댓값 + 1 이며, 그 소유자의 첫 대시보드는 0 이다.
//
// 소유자별인 이유: M3 이관(insertMigratedDashboards)이 스냅샷 1건의
// dashboardPages 배열 인덱스를 그대로 sort_order 로 넣으므로, 전역 스냅샷의
// 페이지들이 0..N-1 을, 각 사용자의 페이지들이 각각 0..M-1 을 갖는다. 이관 데이터에서
// 이미 소유자별 시퀀스이므로 전역 max+1 을 쓰면 두 규칙이 섞인다.
//
// 조회와 삽입이 같은 트랜잭션 안에 있어야 두 요청이 같은 max 를 읽고 같은
// sort_order 를 부여하는 경합이 생기지 않는다 — 핸들러에서 read-then-write 로
// 구현하면 그 경합을 막을 수 없다.
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

	// 소유자별 sort_order 시퀀스. 행이 없으면 COALESCE 가 -1 을 주어 첫 대시보드는 0 이 된다.
	var nextSortOrder int64
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(sort_order), -1) + 1 FROM dashboards WHERE owner = ?`,
		d.Owner).Scan(&nextSortOrder); err != nil {
		return nil, fmt.Errorf("dashboard entity sqlite create: next sort_order: %w", err)
	}

	now := time.Now().UnixMilli()
	res, err := tx.ExecContext(ctx, `
		INSERT INTO dashboards(uid, name, owner, visibility, is_default, sort_order, version, created_at, updated_at, payload)
		VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?, ?)
	`, d.UID, d.Name, d.Owner, d.Visibility, boolToInt(d.IsDefault), nextSortOrder, now, now, d.Payload)
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
	created.SortOrder = nextSortOrder
	created.Version = 1
	created.CreatedAt = now
	created.UpdatedAt = now
	return &created, nil
}

// Update 는 uid 의 대시보드를 부분 갱신하고 새 version 을 부여한다.
//
// expectedVersion >= 0 이고 현재 version 과 다르면 ErrDashboardVersionMismatch 를
// 반환하며 트랜잭션이 롤백되어 서버 상태는 변하지 않는다(spec.md §2.13 #10).
//
// IsDefault 를 true 로 올리면 **같은 소유자의 다른 대시보드** 의 is_default 를 0 으로
// 내린다(acceptance.md 엣지 케이스 "is_default 가 2장 이상에 설정됨 → 마지막 것만
// 유지하고 나머지는 0으로 정규화"). 이 요청이 "마지막 것" 이므로 이 행만 남는다.
//
// **소유자 범위인 이유**: acceptance.md 는 정규화 범위를 명시하지 않지만, 전역
// 범위로 하면 한 사용자가 자기 기본 대시보드를 지정하는 순간 다른 사용자의 기본
// 대시보드가 조용히 해제된다 — 명백히 틀렸다. sort_order 와 같은 축이며, 이관
// 경로도 스냅샷 1건 = 소유자 1명이므로 같은 모양이다.
//
// 내려가는 행들의 version / updated_at 은 **올리지 않는다.** version 은 그 행의
// 본문·메타를 편집하는 클라이언트의 낙관적 동시성 토큰인데, 여기서 올리면 다른 탭의
// 진행 중인 저장이 무관한 이유로 409 를 받는다. is_default 는 payload 밖의 컬럼이라
// 덮어쓰기 손실도 발생하지 않는다.
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
	var currentOwner string
	err = tx.QueryRowContext(ctx,
		`SELECT id, version, owner FROM dashboards WHERE uid = ?`, uid).
		Scan(&id, &currentVersion, &currentOwner)
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

	// 기본 대시보드 정규화 — 같은 트랜잭션 안에서 수행해야 "둘 다 기본" 인 중간
	// 상태가 다른 요청에 관측되지 않는다.
	//
	// 소유권 이전(upd.Owner)이 함께 오면 **새 소유자** 기준으로 정규화한다. 옮겨간
	// 대시보드가 새 소유자의 기본이 되는 것이므로, 정리 대상도 새 소유자 쪽이다.
	if upd.IsDefault != nil && *upd.IsDefault {
		normalizeOwner := currentOwner
		if upd.Owner != nil {
			normalizeOwner = *upd.Owner
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE dashboards SET is_default = 0 WHERE owner = ? AND id != ? AND is_default != 0`,
			normalizeOwner, id); err != nil {
			return nil, fmt.Errorf("dashboard entity sqlite update: normalize is_default: %w", err)
		}
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
