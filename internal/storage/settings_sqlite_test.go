// settings_sqlite_test.go 는 SettingsSQLiteRepository 의 테이블 기반 테스트이다.
//
// 검증 범위:
//   - SetSetting → GetSetting 라운드트립(저장/조회)
//   - 없는 key 조회 시 ErrSettingNotFound
//   - 동일 key 재저장 시 UPSERT(덮어쓰기)
//   - 불투명 value(임의 JSON/문자열) 무손실 보존
//   - 멱등 마이그레이션(재오픈 시 데이터 유지)
//   - 공유 *sql.DB 진입점(WithDB)
package storage

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newSettingsRepo 는 임시 디렉토리에 SQLite settings 저장소를 생성한다.
func newSettingsRepo(t *testing.T) (*SettingsSQLiteRepository, string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "settings.db")
	repo, err := NewSettingsSQLiteRepository(context.Background(), dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })
	return repo, dbPath
}

func TestSettingsSQLiteRepository_SetGet(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "단순 JSON 객체",
			key:   "device-list-columns",
			value: `{"columns":["name","online","last_seen"]}`,
		},
		{
			name:  "JSON 배열",
			key:   "ordered-cols",
			value: `["a","b","c"]`,
		},
		{
			name:  "빈 문자열 value",
			key:   "empty",
			value: "",
		},
		{
			name:  "유니코드/특수문자 보존",
			key:   "ui-pref",
			value: `{"label":"온도 °C","emoji":"x"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newSettingsRepo(t)

			err := repo.SetSetting(ctx, tt.key, tt.value)
			require.NoError(t, err)

			got, err := repo.GetSetting(ctx, tt.key)
			require.NoError(t, err)
			assert.Equal(t, tt.value, got)
		})
	}
}

func TestSettingsSQLiteRepository_GetNotFound(t *testing.T) {
	repo, _ := newSettingsRepo(t)

	_, err := repo.GetSetting(context.Background(), "does-not-exist")
	assert.ErrorIs(t, err, ErrSettingNotFound)
}

func TestSettingsSQLiteRepository_Upsert(t *testing.T) {
	ctx := context.Background()
	repo, _ := newSettingsRepo(t)

	require.NoError(t, repo.SetSetting(ctx, "k", `{"v":1}`))
	require.NoError(t, repo.SetSetting(ctx, "k", `{"v":2}`))

	got, err := repo.GetSetting(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, `{"v":2}`, got, "재저장 시 최신 값으로 덮어써져야 한다")
}

// TestSettingsSQLiteRepository_PersistAcrossReopen 은 멱등 마이그레이션과 영속성을
// 검증한다: 저장 후 닫고 다시 열어도 값이 유지되며 재마이그레이션이 무해해야 한다.
func TestSettingsSQLiteRepository_PersistAcrossReopen(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "settings.db")

	repo1, err := NewSettingsSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	require.NoError(t, repo1.SetSetting(ctx, "persist", `{"x":true}`))
	require.NoError(t, repo1.Close())

	repo2, err := NewSettingsSQLiteRepository(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { repo2.Close() })

	got, err := repo2.GetSetting(ctx, "persist")
	require.NoError(t, err)
	assert.Equal(t, `{"x":true}`, got)
}

// TestSettingsSQLiteRepository_WithSharedDB 는 공유 *sql.DB 진입점을 검증한다.
func TestSettingsSQLiteRepository_WithSharedDB(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "shared.db")

	db, err := OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	repo, err := NewSettingsSQLiteRepositoryWithDB(ctx, db)
	require.NoError(t, err)

	require.NoError(t, repo.SetSetting(ctx, "k", "v"))
	got, err := repo.GetSetting(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

// TestSettingsSQLiteRepository_FactoryDefaults 는 factory 가 알 수 없는 type 에 대해
// sqlite 로 폴백하는지 확인한다.
func TestSettingsSQLiteRepository_FactoryDefaults(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "factory.db")

	repo, err := NewSettingsRepository(ctx, "unknown-type", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { repo.Close() })

	require.NoError(t, repo.SetSetting(ctx, "k", "v"))
	got, err := repo.GetSetting(ctx, "k")
	require.NoError(t, err)
	assert.Equal(t, "v", got)
}

// TestNewSettingsSQLiteRepositoryWithDB_NilDB 는 nil DB 가드를 검증한다.
func TestNewSettingsSQLiteRepositoryWithDB_NilDB(t *testing.T) {
	_, err := NewSettingsSQLiteRepositoryWithDB(context.Background(), nil)
	assert.Error(t, err)
}
