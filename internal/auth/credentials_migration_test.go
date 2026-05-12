// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-3, UB-007)
// credentials_migration_test.go — yaml → SQLite 1회성 마이그레이션 시나리오 검증.
//
// 검증 대상:
//   - AC-1: SQLite 와 yaml 모두 비어있으면 admin/admin 자동 생성.
//   - AC-2: yaml 이 존재하면 SQLite 로 INSERT OR IGNORE 후 yaml 을 ".migrated" 로
//           rename. 마이그레이션된 사용자로 인증 가능.
//   - UB-007: 이관 후 yaml 미참조 (yaml.migrated 가 인증 흐름에 영향 없음).
//   - 멱등성: 이미 마이그레이션된 환경에서 재부팅 시 no-op.

package auth

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// setupMigrationDB 는 마이그레이션 테스트용 SQLite DB 를 생성한다.
func setupMigrationDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "migration-test.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

// writeYAMLUsers 는 테스트용 users.yaml 파일을 작성한다.
// users 는 (username, bcrypt-hash, role) 튜플 리스트.
func writeYAMLUsers(t *testing.T, path string, users []struct{ Username, Hash, Role string }) {
	t.Helper()

	content := "users:\n"
	for _, u := range users {
		content += "  - username: " + u.Username + "\n"
		content += "    password_hash: \"" + u.Hash + "\"\n"
		if u.Role != "" {
			content += "    role: " + u.Role + "\n"
		}
	}
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
}

// TestMigration_AC1_NoYAML_CreatesAdmin 는 AC-1 검증.
// yaml 없고 SQLite 비어있는 상태에서 EnsureDefaultAdmin → admin/admin 자동 생성.
func TestMigration_AC1_NoYAML_CreatesAdmin(t *testing.T) {
	db := setupMigrationDB(t)
	yamlPath := filepath.Join(t.TempDir(), "users.yaml") // 파일 생성 안 함

	cm := NewCredentialsManager(db, yamlPath)
	require.NoError(t, cm.EnsureDefaultAdmin())

	// admin/admin 으로 인증 가능
	user, err := cm.Authenticate("admin", "admin")
	require.NoError(t, err)
	assert.Equal(t, "admin", user.Username)
	assert.Equal(t, "admin", user.Role)

	// yaml 파일은 여전히 존재하지 않음 (생성되지 않음)
	_, statErr := os.Stat(yamlPath)
	assert.True(t, os.IsNotExist(statErr), "yaml 파일은 생성되지 않아야 한다")

	// migrated 파일도 없음
	_, statErr = os.Stat(yamlPath + ".migrated")
	assert.True(t, os.IsNotExist(statErr), "yaml.migrated 도 생성되지 않아야 한다")
}

// TestMigration_AC2_YAMLExists_MigratesAndRenames 는 AC-2 검증.
// yaml 3 사용자 (admin, alice, bob) → SQLite 이관 → yaml.migrated rename →
// alice 기존 비밀번호로 인증 성공.
func TestMigration_AC2_YAMLExists_MigratesAndRenames(t *testing.T) {
	db := setupMigrationDB(t)
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "users.yaml")

	// bcrypt 해시 생성 (테스트 가속 위해 한 번만)
	adminHash, _ := HashPassword("admin-old")
	aliceHash, _ := HashPassword("alice-pass")
	bobHash, _ := HashPassword("bob-pass")

	writeYAMLUsers(t, yamlPath, []struct{ Username, Hash, Role string }{
		{"admin", adminHash, "admin"},
		{"alice", aliceHash, "editor"},
		{"bob", bobHash, "viewer"},
	})

	cm := NewCredentialsManager(db, yamlPath)
	require.NoError(t, cm.EnsureDefaultAdmin())

	// 3 사용자가 SQLite 로 이관됨
	ctx := context.Background()
	count, err := storage.CountUsers(ctx, db)
	require.NoError(t, err)
	assert.EqualValues(t, 3, count, "yaml 의 3 사용자가 모두 이관되어야 한다")

	// alice 기존 비밀번호로 인증 성공
	user, err := cm.Authenticate("alice", "alice-pass")
	require.NoError(t, err, "alice 기존 비밀번호로 인증 성공해야 한다")
	assert.Equal(t, "alice", user.Username)
	assert.Equal(t, "editor", user.Role)

	// admin 도 기존 비밀번호 (admin-old) 로 인증 성공 — 기본 admin/admin 으로는 NOT 변경됨
	_, err = cm.Authenticate("admin", "admin-old")
	require.NoError(t, err)
	_, err = cm.Authenticate("admin", "admin")
	assert.ErrorIs(t, err, ErrInvalidCredentials, "yaml 의 admin 비밀번호가 보존되어야 한다")

	// yaml 파일이 .migrated 로 rename
	_, statErr := os.Stat(yamlPath)
	assert.True(t, os.IsNotExist(statErr), "원본 yaml 은 더 이상 존재하지 않아야 한다")

	migratedPath := yamlPath + ".migrated"
	info, err := os.Stat(migratedPath)
	require.NoError(t, err, "yaml.migrated 가 존재해야 한다")
	assert.False(t, info.IsDir())
}

// TestMigration_UB007_YAMLMigratedIsIgnored 는 UB-007 검증.
//
// 이관 후 yaml.migrated 가 잔존해도 다음 부팅에서 인증 흐름이 절대로 yaml.migrated
// 를 참조하지 않음을 검증한다.
//
// 시나리오:
//  1. yaml → SQLite 이관 (yaml.migrated 생성)
//  2. SQLite 의 alice 비밀번호를 새 비밀번호로 변경
//  3. yaml.migrated 의 옛 해시는 그대로 (검증 대상)
//  4. 새 CredentialsManager 인스턴스 (재부팅 시뮬레이션) 에서:
//     - 새 비밀번호로 인증 성공 (SQLite 참조)
//     - yaml.migrated 의 옛 비밀번호로는 인증 실패
//     - users 테이블이 비어있지 않으므로 마이그레이션 재실행 안 됨
func TestMigration_UB007_YAMLMigratedIsIgnored(t *testing.T) {
	db := setupMigrationDB(t)
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "users.yaml")

	aliceOldHash, _ := HashPassword("alice-old-pass")
	writeYAMLUsers(t, yamlPath, []struct{ Username, Hash, Role string }{
		{"alice", aliceOldHash, "admin"},
	})

	// 1. 첫 부팅 → 이관
	cm1 := NewCredentialsManager(db, yamlPath)
	require.NoError(t, cm1.EnsureDefaultAdmin())

	// 2. SQLite 의 alice 비밀번호 변경
	require.NoError(t, cm1.ChangePassword("alice", "alice-old-pass", "alice-new-pass"))

	// 3. yaml.migrated 파일은 옛 해시를 그대로 가지고 있음 (재부팅 시 참조되면 안 됨)
	migratedContent, err := os.ReadFile(yamlPath + ".migrated")
	require.NoError(t, err)
	assert.Contains(t, string(migratedContent), aliceOldHash, "yaml.migrated 에 옛 해시가 보존됨")

	// 4. 새 manager 인스턴스 (재부팅 시뮬레이션) — yamlPath 를 그대로 전달
	cm2 := NewCredentialsManager(db, yamlPath)
	require.NoError(t, cm2.EnsureDefaultAdmin())

	// 4-1. 새 비밀번호로 인증 성공 (SQLite source-of-truth)
	user, err := cm2.Authenticate("alice", "alice-new-pass")
	require.NoError(t, err)
	assert.Equal(t, "alice", user.Username)

	// 4-2. yaml.migrated 의 옛 비밀번호로는 인증 실패 (yaml.migrated 미참조)
	_, err = cm2.Authenticate("alice", "alice-old-pass")
	assert.ErrorIs(t, err, ErrInvalidCredentials, "yaml.migrated 의 옛 비밀번호는 인증되지 않아야 한다")
}

// TestMigration_AlreadyMigrated_NoOp 는 두 번째 부팅이 멱등하게 동작함을 검증한다.
//
// yaml.migrated 만 존재하고 yaml 원본은 없는 상태에서 EnsureDefaultAdmin 호출 시
// 마이그레이션이 재실행되지 않으며 (yaml 미존재), 이미 존재하는 사용자는 보존된다.
func TestMigration_AlreadyMigrated_NoOp(t *testing.T) {
	db := setupMigrationDB(t)
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "users.yaml")

	// 첫 부팅: yaml → SQLite 이관
	aliceHash, _ := HashPassword("alice-pass")
	writeYAMLUsers(t, yamlPath, []struct{ Username, Hash, Role string }{
		{"alice", aliceHash, "admin"},
	})
	cm1 := NewCredentialsManager(db, yamlPath)
	require.NoError(t, cm1.EnsureDefaultAdmin())

	// yaml 원본은 사라지고 yaml.migrated 가 생성됨
	require.NoFileExists(t, yamlPath)
	require.FileExists(t, yamlPath+".migrated")

	ctx := context.Background()
	beforeCount, _ := storage.CountUsers(ctx, db)
	assert.EqualValues(t, 1, beforeCount)

	// 두 번째 부팅 (yaml 원본 없음, yaml.migrated 만 잔존)
	cm2 := NewCredentialsManager(db, yamlPath)
	require.NoError(t, cm2.EnsureDefaultAdmin())

	// 사용자 수 변동 없음 (yaml.migrated 는 참조되지 않음 — 추가 admin 도 생성되지 않음)
	afterCount, _ := storage.CountUsers(ctx, db)
	assert.EqualValues(t, 1, afterCount, "두 번째 부팅은 no-op (사용자 수 변동 없음)")

	// alice 인증 여전히 가능
	_, err := cm2.Authenticate("alice", "alice-pass")
	require.NoError(t, err)
}

// TestMigration_YAMLMigration_PreservesExisting 는 INSERT OR IGNORE 가 기존 사용자를
// 덮어쓰지 않음을 검증한다 (yaml 과 SQLite 가 동시 존재하는 경계 케이스).
func TestMigration_YAMLMigration_PreservesExisting(t *testing.T) {
	db := setupMigrationDB(t)
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "users.yaml")

	// SQLite 에 alice 를 먼저 직접 삽입 (다른 비밀번호)
	directHash, _ := HashPassword("sqlite-direct")
	ctx := context.Background()
	require.NoError(t, storage.InsertUser(ctx, db, "alice", directHash, "viewer", 0, 0))

	// yaml 에 alice (다른 비밀번호) + bob 작성
	yamlAliceHash, _ := HashPassword("yaml-alice")
	bobHash, _ := HashPassword("bob-pass")
	writeYAMLUsers(t, yamlPath, []struct{ Username, Hash, Role string }{
		{"alice", yamlAliceHash, "admin"},
		{"bob", bobHash, "editor"},
	})

	cm := NewCredentialsManager(db, yamlPath)
	// SQLite 가 이미 비어있지 않으므로 yaml 마이그레이션 자체가 트리거되지 않는다.
	// (EnsureDefaultAdmin 의 조건: count == 0 일 때만 yaml 검사).
	require.NoError(t, cm.EnsureDefaultAdmin())

	// alice 는 SQLite 직접 삽입한 비밀번호로 인증 (yaml 덮어쓰기 안 됨)
	_, err := cm.Authenticate("alice", "sqlite-direct")
	require.NoError(t, err)

	// bob 은 yaml 에만 있고 SQLite 에는 없음 → 인증 불가
	_, err = cm.Authenticate("bob", "bob-pass")
	assert.ErrorIs(t, err, ErrInvalidCredentials, "yaml 만 있고 SQLite 가 비어있지 않으면 마이그레이션 안 됨")

	// yaml 원본 여전히 존재 (이관되지 않음)
	require.FileExists(t, yamlPath)
}
