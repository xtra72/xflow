// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-2, M-3)
// credentials_test.go — SQLite 기반 자격증명 관리자 테스트.
//
// v0.1.x 의 yaml 파일 기반 테스트는 SQLite 백엔드로 이관되었다. 외부 API
// (Authenticate, ChangePassword, EnsureDefaultAdmin, GetUser) 시그니처는 유지되며
// 기존 테스트 시나리오가 동일하게 검증된다.

package auth

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/storage"
)

// setupCredentialsDB 는 테스트용 SQLite DB 를 생성하여 *sql.DB 를 반환한다.
// 테스트 종료 시 자동으로 db.Close().
func setupCredentialsDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "credentials-test.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err, "OpenSQLiteDB 실패")
	t.Cleanup(func() { db.Close() })
	return db
}

// insertTestUser 는 직접 SQL 로 사용자를 삽입하는 헬퍼이다 (NewCredentialsManager 와 무관).
func insertTestUser(t *testing.T, db *sql.DB, username, passwordHash, role string) {
	t.Helper()
	ctx := context.Background()
	err := storage.InsertUser(ctx, db, username, passwordHash, role, 0, 0)
	require.NoError(t, err)
}

// TestCredentialsManager_AuthenticateAndChangePassword 는 기본 인증 + 비밀번호 변경
// 시나리오를 검증한다.
//
// v0.1.x 의 TestCredentialsManager_LoadAndSave 와 동일한 시나리오를 SQLite 백엔드로
// 재작성한 형태.
func TestCredentialsManager_AuthenticateAndChangePassword(t *testing.T) {
	db := setupCredentialsDB(t)

	// bcrypt 해시로 alice / bob 두 사용자 직접 삽입
	aliceHash, err := HashPassword("alice-pass")
	require.NoError(t, err)
	insertTestUser(t, db, "alice", aliceHash, "admin")

	bobHash, err := HashPassword("bob-pass")
	require.NoError(t, err)
	insertTestUser(t, db, "bob", bobHash, "viewer")

	cm := NewCredentialsManager(db, "")
	require.NoError(t, cm.Load(), "Load 는 SQLite 백엔드에서 no-op")

	// 사용자 조회
	alice, err := cm.GetUser("alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", alice.Username)
	assert.Equal(t, "admin", alice.Role)

	bob, err := cm.GetUser("bob")
	require.NoError(t, err)
	assert.Equal(t, "viewer", bob.Role)

	// 존재하지 않는 사용자
	_, err = cm.GetUser("charlie")
	assert.ErrorIs(t, err, ErrUserNotFound)

	// 인증 → bob 비밀번호 변경 → 새 비밀번호로 인증
	_, err = cm.Authenticate("bob", "bob-pass")
	require.NoError(t, err)

	require.NoError(t, cm.ChangePassword("bob", "bob-pass", "bob-newpass"))
	require.NoError(t, cm.Save(), "Save 는 SQLite 백엔드에서 no-op")

	// 새 manager 인스턴스에서 새 비밀번호로 인증 가능
	cm2 := NewCredentialsManager(db, "")
	_, err = cm2.Authenticate("bob", "bob-newpass")
	require.NoError(t, err)

	// 옛 비밀번호는 거부
	_, err = cm2.Authenticate("bob", "bob-pass")
	assert.ErrorIs(t, err, ErrInvalidCredentials)
}

// TestCredentialsManager_Authenticate 는 다양한 인증 케이스를 검증한다.
func TestCredentialsManager_Authenticate(t *testing.T) {
	db := setupCredentialsDB(t)

	hash, err := HashPassword("secret123")
	require.NoError(t, err)
	insertTestUser(t, db, "testuser", hash, "admin")

	cm := NewCredentialsManager(db, "")

	tests := []struct {
		name     string
		username string
		password string
		wantErr  error
		wantUser bool
	}{
		{
			name:     "올바른 자격증명",
			username: "testuser",
			password: "secret123",
			wantErr:  nil,
			wantUser: true,
		},
		{
			name:     "잘못된 비밀번호",
			username: "testuser",
			password: "wrongpassword",
			wantErr:  ErrInvalidCredentials,
			wantUser: false,
		},
		{
			name:     "존재하지 않는 사용자",
			username: "nobody",
			password: "secret123",
			wantErr:  ErrInvalidCredentials,
			wantUser: false,
		},
		{
			name:     "빈 사용자명",
			username: "",
			password: "secret123",
			wantErr:  ErrInvalidCredentials,
			wantUser: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			user, err := cm.Authenticate(tc.username, tc.password)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Nil(t, user)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tc.username, user.Username)
			}
		})
	}
}

// TestCredentialsManager_Authenticate_TimingAttack 는 REQ-N-002 검증.
// 존재하지 않는 사용자와 잘못된 비밀번호 모두 동일한 에러 타입을 반환한다.
func TestCredentialsManager_Authenticate_TimingAttack(t *testing.T) {
	db := setupCredentialsDB(t)

	hash, err := HashPassword("password")
	require.NoError(t, err)
	insertTestUser(t, db, "existing", hash, "admin")

	cm := NewCredentialsManager(db, "")

	// 존재하지 않는 사용자
	_, err1 := cm.Authenticate("nonexistent", "password")
	// 잘못된 비밀번호
	_, err2 := cm.Authenticate("existing", "wrongpassword")

	// 두 경우 모두 동일한 에러 타입 (구분 불가)
	assert.ErrorIs(t, err1, ErrInvalidCredentials)
	assert.ErrorIs(t, err2, ErrInvalidCredentials)
}

// TestCredentialsManager_ChangePassword 는 비밀번호 변경의 성공/실패 케이스를 검증한다.
func TestCredentialsManager_ChangePassword(t *testing.T) {
	db := setupCredentialsDB(t)

	hash, err := HashPassword("oldpass")
	require.NoError(t, err)
	insertTestUser(t, db, "testuser", hash, "admin")

	cm := NewCredentialsManager(db, "")

	tests := []struct {
		name        string
		username    string
		currentPass string
		newPass     string
		wantErr     bool
	}{
		{
			name:        "성공적인 비밀번호 변경",
			username:    "testuser",
			currentPass: "oldpass",
			newPass:     "newpass123",
			wantErr:     false,
		},
		{
			name:        "잘못된 현재 비밀번호",
			username:    "testuser",
			currentPass: "wrongpass",
			newPass:     "newpass123",
			wantErr:     true,
		},
		{
			name:        "존재하지 않는 사용자",
			username:    "nobody",
			currentPass: "oldpass",
			newPass:     "newpass123",
			wantErr:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := cm.ChangePassword(tc.username, tc.currentPass, tc.newPass)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				// 새 비밀번호로 인증 확인
				user, authErr := cm.Authenticate(tc.username, tc.newPass)
				require.NoError(t, authErr)
				assert.Equal(t, tc.username, user.Username)
			}
		})
	}
}

// TestCredentialsManager_EnsureDefaultAdmin 은 부팅 시 동작을 검증한다.
//
// v0.2.0: yaml 파일 없는 환경에서 SQLite 가 비어 있으면 admin/admin 을 자동 생성
// (기존 동작 유지, AC-1).
func TestCredentialsManager_EnsureDefaultAdmin(t *testing.T) {
	t.Run("DB가_비어있고_yaml도_없으면_admin_생성", func(t *testing.T) {
		db := setupCredentialsDB(t)
		cm := NewCredentialsManager(db, "")
		require.NoError(t, cm.EnsureDefaultAdmin())

		// admin 사용자 존재 확인
		admin, err := cm.GetUser("admin")
		require.NoError(t, err)
		assert.Equal(t, "admin", admin.Username)
		assert.Equal(t, "admin", admin.Role)

		// admin/admin 으로 인증 가능
		user, err := cm.Authenticate("admin", "admin")
		require.NoError(t, err)
		assert.Equal(t, "admin", user.Username)
	})

	t.Run("DB에_사용자가_이미_있으면_no-op", func(t *testing.T) {
		db := setupCredentialsDB(t)

		// 기존 admin 직접 삽입 (custompass 해시)
		hash, err := HashPassword("custompass")
		require.NoError(t, err)
		insertTestUser(t, db, "admin", hash, "admin")

		cm := NewCredentialsManager(db, "")
		require.NoError(t, cm.EnsureDefaultAdmin())

		// 기존 사용자 보존 (덮어쓰지 않음)
		_, err = cm.Authenticate("admin", "custompass")
		require.NoError(t, err)

		// 기본 admin/admin 으로는 인증 안 됨
		_, err = cm.Authenticate("admin", "admin")
		assert.ErrorIs(t, err, ErrInvalidCredentials)
	})

	t.Run("멱등성_두번_호출해도_admin은_한_번만_생성", func(t *testing.T) {
		db := setupCredentialsDB(t)
		cm := NewCredentialsManager(db, "")
		require.NoError(t, cm.EnsureDefaultAdmin())
		require.NoError(t, cm.EnsureDefaultAdmin())

		// 사용자 수 = 1
		ctx := context.Background()
		count, err := storage.CountUsers(ctx, db)
		require.NoError(t, err)
		assert.EqualValues(t, 1, count, "EnsureDefaultAdmin 멱등")
	})
}
