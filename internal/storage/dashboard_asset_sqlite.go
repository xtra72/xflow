package storage

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ DashboardAssetRepository = (*DashboardAssetSQLiteRepository)(nil)

// DashboardAssetSQLiteRepository 는 dashboard_assets 테이블 기반 구현체이다.
//
// dashboards 와 같은 *sql.DB(xflow.db)를 공유한다 — 별도 파일 핸들을 늘리지 않고
// WAL 모드에서 공존한다(dashboards / settings 와 동일 규율).
type DashboardAssetSQLiteRepository struct {
	db *sql.DB
}

// NewDashboardAssetSQLiteRepository 는 열린 *sql.DB 를 받아 스키마를 멱등 보장한 뒤
// 저장소를 반환한다. db 의 수명은 호출자가 관리한다.
func NewDashboardAssetSQLiteRepository(ctx context.Context, db *sql.DB) (*DashboardAssetSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("dashboard asset sqlite: db must not be nil")
	}
	if err := migrateDashboardAssetSchema(ctx, db); err != nil {
		return nil, err
	}
	return &DashboardAssetSQLiteRepository{db: db}, nil
}

// migrateDashboardAssetSchema 는 dashboard_assets 테이블을 멱등하게 생성한다.
//
// id 가 내용 해시이므로 PRIMARY KEY 가 곧 중복 제거 장치다 — 같은 도면을 여러 패널이
// 참조해도 BLOB 은 한 벌만 저장된다.
func migrateDashboardAssetSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS dashboard_assets (
		id         TEXT    PRIMARY KEY,
		mime       TEXT    NOT NULL,
		size       INTEGER NOT NULL,
		created_at INTEGER NOT NULL,
		data       BLOB    NOT NULL
	)`); err != nil {
		return fmt.Errorf("create dashboard_assets table: %w", err)
	}
	return nil
}

// Put 은 내용 해시를 id 로 삼아 자산을 저장한다.
//
// 같은 내용이 이미 있으면 INSERT 를 건너뛰고(ON CONFLICT DO NOTHING) 기존 created_at 을
// 보존한다 — 재업로드가 타임스탬프를 흔들지 않는다.
func (r *DashboardAssetSQLiteRepository) Put(
	ctx context.Context,
	mime string,
	data []byte,
	nowMs int64,
) (*DashboardAsset, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("dashboard asset put: empty data")
	}
	sum := sha256.Sum256(data)
	id := hex.EncodeToString(sum[:])

	if _, err := r.db.ExecContext(ctx, `
		INSERT INTO dashboard_assets (id, mime, size, created_at, data)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`, id, mime, int64(len(data)), nowMs, data); err != nil {
		return nil, fmt.Errorf("dashboard asset put: %w", err)
	}

	// created_at 은 최초 저장 시각이어야 하므로 삽입 여부와 무관하게 저장된 값을 읽어 돌려준다.
	row := r.db.QueryRowContext(ctx,
		`SELECT id, mime, size, created_at FROM dashboard_assets WHERE id = ?`, id)
	var rec DashboardAsset
	if err := row.Scan(&rec.ID, &rec.MIME, &rec.Size, &rec.CreatedAt); err != nil {
		return nil, fmt.Errorf("dashboard asset put readback: %w", err)
	}
	return &rec, nil
}

// Get 은 id 로 자산을 조회한다(바이트 포함). 없으면 ErrDashboardAssetNotFound.
func (r *DashboardAssetSQLiteRepository) Get(ctx context.Context, id string) (*DashboardAsset, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT id, mime, size, created_at, data FROM dashboard_assets WHERE id = ?`, id)
	var rec DashboardAsset
	if err := row.Scan(&rec.ID, &rec.MIME, &rec.Size, &rec.CreatedAt, &rec.Data); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrDashboardAssetNotFound
		}
		return nil, fmt.Errorf("dashboard asset get: %w", err)
	}
	return &rec, nil
}
