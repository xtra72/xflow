package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// @SPEC:SPEC-DASHBOARD-004 (M2, spec.md §2.1)
// dashboard_state_sqlite.go — dashboard_user_state 테이블 저장소.
//
// activeDashboardId 와 deviceGridLayout 은 개별 대시보드에 속하지 않는 사용자 UI
// 상태다. 구 모델에서는 스냅샷 안에 섞여 있어 대시보드를 공유하면 남의 활성
// 대시보드까지 따라갔다. 사용자당 1행으로 분리한다.

// 컴파일 타임 인터페이스 구현 검증.
var _ DashboardUserStateRepository = (*DashboardUserStateSQLiteRepository)(nil)

// ErrDashboardUserStateNotFound 는 해당 사용자의 UI 상태 행이 없을 때 반환된다.
//
// 호출자가 기본값을 구성할지 404 를 낼지 판단한다. 저장소가 임의로 기본값을
// 지어내지 않는 이유는, "저장한 적 없음" 과 "빈 값을 저장함" 이 version 관점에서
// 다르기 때문이다(전자는 version 0, 후자는 1 이상).
var ErrDashboardUserStateNotFound = errors.New("dashboard user state not found")

// emptyDeviceGridLayout 은 device_grid_layout 의 기본값이다 (빈 JSON 객체).
const emptyDeviceGridLayout = "{}"

// DashboardUserState 는 dashboard_user_state 테이블의 단일 행이다.
//
// DeviceGridLayout 은 JSON 객체 바이트 슬라이스이며 재직렬화 없이 그대로 보관한다.
type DashboardUserState struct {
	Username           string
	ActiveDashboardUID string
	DeviceGridLayout   []byte
	Version            int64
	UpdatedAt          int64 // epoch milliseconds
}

// DashboardUserStateRepository 는 사용자별 대시보드 UI 상태의 영속 저장소이다.
type DashboardUserStateRepository interface {
	// Get 은 username 의 UI 상태를 조회한다. 없으면 ErrDashboardUserStateNotFound.
	Get(ctx context.Context, username string) (*DashboardUserState, error)

	// Put 은 username 의 UI 상태를 upsert 하고 새 version 을 부여한다.
	//
	// expectedVersion 의 의미는 대시보드 저장과 동일하다:
	//   - < 0  : unconditional
	//   - >= 0 : 불일치 시 ErrDashboardVersionMismatch (서버 상태 미변경)
	//   - 행 부재 시 현재 version 은 0 으로 간주된다.
	//
	// st.Version / st.UpdatedAt 은 무시되고 서버가 부여한다.
	Put(ctx context.Context, st DashboardUserState, expectedVersion int64) (*DashboardUserState, error)
}

// DashboardUserStateSQLiteRepository 는 dashboard_user_state 기반 구현체이다.
type DashboardUserStateSQLiteRepository struct {
	db *sql.DB
}

// NewDashboardUserStateSQLiteRepository 는 이미 열린 *sql.DB 를 받아 신규 대시보드
// 스키마를 멱등하게 보장한 뒤 저장소를 반환한다.
//
// db 의 수명은 호출자가 관리한다 (Close 책임은 main.go).
func NewDashboardUserStateSQLiteRepository(ctx context.Context, db *sql.DB) (*DashboardUserStateSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("dashboard user state sqlite: db must not be nil")
	}
	if err := migrateDashboardSchemaV2(ctx, db); err != nil {
		return nil, err
	}
	return &DashboardUserStateSQLiteRepository{db: db}, nil
}

// Get 은 username 의 UI 상태를 조회한다.
func (r *DashboardUserStateSQLiteRepository) Get(ctx context.Context, username string) (*DashboardUserState, error) {
	var st DashboardUserState
	err := r.db.QueryRowContext(ctx, `
		SELECT username, active_dashboard_uid, device_grid_layout, version, updated_at
		FROM dashboard_user_state WHERE username = ?
	`, username).Scan(&st.Username, &st.ActiveDashboardUID, &st.DeviceGridLayout, &st.Version, &st.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDashboardUserStateNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("dashboard user state sqlite get %q: %w", username, err)
	}
	return &st, nil
}

// Put 은 username 의 UI 상태를 upsert 한다.
func (r *DashboardUserStateSQLiteRepository) Put(
	ctx context.Context,
	st DashboardUserState,
	expectedVersion int64,
) (*DashboardUserState, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("dashboard user state sqlite put: begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var currentVersion int64
	err = tx.QueryRowContext(ctx,
		`SELECT version FROM dashboard_user_state WHERE username = ?`, st.Username).Scan(&currentVersion)
	exists := true
	if errors.Is(err, sql.ErrNoRows) {
		exists = false
		currentVersion = 0
		err = nil
	}
	if err != nil {
		return nil, fmt.Errorf("dashboard user state sqlite put: select: %w", err)
	}

	if expectedVersion >= 0 && currentVersion != expectedVersion {
		return nil, ErrDashboardVersionMismatch
	}

	layout := st.DeviceGridLayout
	if len(layout) == 0 {
		layout = []byte(emptyDeviceGridLayout)
	}
	newVersion := currentVersion + 1
	newUpdatedAt := time.Now().UnixMilli()

	if exists {
		if _, err := tx.ExecContext(ctx, `
			UPDATE dashboard_user_state
			SET active_dashboard_uid = ?, device_grid_layout = ?, version = ?, updated_at = ?
			WHERE username = ?
		`, st.ActiveDashboardUID, layout, newVersion, newUpdatedAt, st.Username); err != nil {
			return nil, fmt.Errorf("dashboard user state sqlite put: update: %w", err)
		}
	} else {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO dashboard_user_state(username, active_dashboard_uid, device_grid_layout, version, updated_at)
			VALUES (?, ?, ?, ?, ?)
		`, st.Username, st.ActiveDashboardUID, layout, newVersion, newUpdatedAt); err != nil {
			return nil, fmt.Errorf("dashboard user state sqlite put: insert: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("dashboard user state sqlite put: commit: %w", err)
	}
	committed = true

	return &DashboardUserState{
		Username:           st.Username,
		ActiveDashboardUID: st.ActiveDashboardUID,
		DeviceGridLayout:   layout,
		Version:            newVersion,
		UpdatedAt:          newUpdatedAt,
	}, nil
}
