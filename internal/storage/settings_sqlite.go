// settings_sqlite.go 는 SettingsRepository 의 SQLite 구현이다.
//
// EnrollmentTokenSQLiteRepository 패턴을 준용한다: WAL 모드 DSN, IF NOT EXISTS 멱등
// 마이그레이션으로 기존 xflow.db 에 settings 테이블을 추가한다(기존 DB 호환).
//
// 스키마(settings):
//
//	key        TEXT PRIMARY KEY  — 설정 키(프론트가 정함, 예: "device-list-columns")
//	value      TEXT NOT NULL     — 불투명 JSON 문자열(서버는 스키마 검증 안 함)
//	updated_at INTEGER NOT NULL  — 마지막 갱신 시각(epoch ms)
package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite" // Pure Go SQLite 드라이버 등록
)

// 컴파일 타임 인터페이스 구현 검증.
var _ SettingsRepository = (*SettingsSQLiteRepository)(nil)

// SettingsSQLiteRepository 는 settings 테이블 기반의 SettingsRepository 구현이다.
type SettingsSQLiteRepository struct {
	db *sql.DB
}

// NewSettingsSQLiteRepository 는 SQLite DB 를 열고 settings 테이블을 멱등하게 생성한 후
// 저장소를 반환한다. WAL 모드를 활성화한다.
func NewSettingsSQLiteRepository(ctx context.Context, dbPath string) (*SettingsSQLiteRepository, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// DSN 에 WAL + busy_timeout pragma 를 실어 동시 접근 경합을 흡수한다.
	db, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrateSettingsSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return &SettingsSQLiteRepository{db: db}, nil
}

// NewSettingsSQLiteRepositoryWithDB 는 이미 열린 *sql.DB 를 받아 settings 스키마를
// 멱등하게 보장한 뒤 저장소를 반환한다. db 의 수명은 호출자가 관리한다(Close 책임은 호출자).
//
// main.go 가 공유 xflow.db 핸들(authDashboardDB)을 재사용해 별도 파일 핸들을 늘리지
// 않도록 하는 진입점이다.
func NewSettingsSQLiteRepositoryWithDB(ctx context.Context, db *sql.DB) (*SettingsSQLiteRepository, error) {
	if db == nil {
		return nil, fmt.Errorf("settings sqlite: db must not be nil")
	}
	if err := migrateSettingsSchema(ctx, db); err != nil {
		return nil, err
	}
	return &SettingsSQLiteRepository{db: db}, nil
}

// migrateSettingsSchema 는 settings 테이블을 멱등하게 생성한다(CREATE TABLE IF NOT EXISTS).
// 기존 DB 에 안전하게 추가되며 재실행해도 무해하다.
func migrateSettingsSchema(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS settings (
		key        TEXT    PRIMARY KEY,
		value      TEXT    NOT NULL,
		updated_at INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create settings table: %w", err)
	}
	return nil
}

// GetSetting 은 key 의 value 를 반환한다. 없으면 ErrSettingNotFound.
func (r *SettingsSQLiteRepository) GetSetting(ctx context.Context, key string) (string, error) {
	var value string
	err := r.db.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key = ?`, key).Scan(&value)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrSettingNotFound
	}
	if err != nil {
		return "", fmt.Errorf("get setting: %w", err)
	}
	return value, nil
}

// SetSetting 은 key 에 value 를 저장한다(UPSERT). updated_at 은 서버가 부여한다.
func (r *SettingsSQLiteRepository) SetSetting(ctx context.Context, key, value string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO settings (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value      = excluded.value,
			updated_at = excluded.updated_at
	`, key, value, time.Now().UnixMilli())
	if err != nil {
		return fmt.Errorf("set setting: %w", err)
	}
	return nil
}

// Close 는 데이터베이스 연결을 닫는다.
func (r *SettingsSQLiteRepository) Close() error {
	return r.db.Close()
}
