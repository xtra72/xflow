package auth

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCredentialsManager_LoadAndSave(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "users.yaml")

	// 초기 파일 생성
	content := `users:
  - username: alice
    password_hash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ01234"
    role: admin
  - username: bob
    password_hash: "$2a$10$abcdefghijklmnopqrstuuABCDEFGHIJKLMNOPQRSTUVWXYZ56789"
    role: viewer
`
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0600))

	cm := NewCredentialsManager(filePath)
	require.NoError(t, cm.Load())

	// 로드된 사용자 확인
	alice, err := cm.GetUser("alice")
	require.NoError(t, err)
	assert.Equal(t, "alice", alice.Username)
	assert.Equal(t, "admin", alice.Role)

	bob, err := cm.GetUser("bob")
	require.NoError(t, err)
	assert.Equal(t, "bob", bob.Username)
	assert.Equal(t, "viewer", bob.Role)

	// 존재하지 않는 사용자 조회
	_, err = cm.GetUser("charlie")
	assert.ErrorIs(t, err, ErrUserNotFound)

	// 저장 후 다시 로드
	require.NoError(t, cm.Save())

	cm2 := NewCredentialsManager(filePath)
	require.NoError(t, cm2.Load())

	alice2, err := cm2.GetUser("alice")
	require.NoError(t, err)
	assert.Equal(t, "admin", alice2.Role)
}

func TestCredentialsManager_LoadError(t *testing.T) {
	cm := NewCredentialsManager("/nonexistent/path/users.yaml")
	err := cm.Load()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "자격증명 파일 읽기 실패")
}

func TestCredentialsManager_Authenticate(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "users.yaml")

	// bcrypt 해시로 사용자 생성
	hash, err := HashPassword("secret123")
	require.NoError(t, err)

	cm := NewCredentialsManager(filePath)
	cm.users["testuser"] = &CredentialUser{
		Username:     "testuser",
		PasswordHash: hash,
		Role:         "admin",
	}

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

func TestCredentialsManager_Authenticate_TimingAttack(t *testing.T) {
	// 존재하지 않는 사용자와 잘못된 비밀번호 모두 동일한 에러를 반환해야 한다 (REQ-N-002)
	dir := t.TempDir()
	filePath := filepath.Join(dir, "users.yaml")

	hash, err := HashPassword("password")
	require.NoError(t, err)

	cm := NewCredentialsManager(filePath)
	cm.users["existing"] = &CredentialUser{
		Username:     "existing",
		PasswordHash: hash,
		Role:         "admin",
	}

	// 존재하지 않는 사용자
	_, err1 := cm.Authenticate("nonexistent", "password")
	// 잘못된 비밀번호
	_, err2 := cm.Authenticate("existing", "wrongpassword")

	// 두 경우 모두 동일한 에러 타입이어야 한다
	assert.ErrorIs(t, err1, ErrInvalidCredentials)
	assert.ErrorIs(t, err2, ErrInvalidCredentials)
}

func TestCredentialsManager_ChangePassword(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "users.yaml")

	hash, err := HashPassword("oldpass")
	require.NoError(t, err)

	cm := NewCredentialsManager(filePath)
	cm.users["testuser"] = &CredentialUser{
		Username:     "testuser",
		PasswordHash: hash,
		Role:         "admin",
	}

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

func TestCredentialsManager_EnsureDefaultAdmin(t *testing.T) {
	t.Run("파일이 없으면 기본 admin 생성", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "users.yaml")

		cm := NewCredentialsManager(filePath)
		require.NoError(t, cm.EnsureDefaultAdmin())

		// admin 사용자 존재 확인
		admin, err := cm.GetUser("admin")
		require.NoError(t, err)
		assert.Equal(t, "admin", admin.Username)
		assert.Equal(t, "admin", admin.Role)

		// admin/admin 으로 인증 가능 확인
		user, err := cm.Authenticate("admin", "admin")
		require.NoError(t, err)
		assert.Equal(t, "admin", user.Username)

		// 파일이 생성되었는지 확인
		_, statErr := os.Stat(filePath)
		assert.NoError(t, statErr)

		// 파일 권한 확인 (0600)
		info, _ := os.Stat(filePath)
		assert.Equal(t, os.FileMode(0600), info.Mode().Perm())
	})

	t.Run("파일이 있으면 로드만 수행", func(t *testing.T) {
		dir := t.TempDir()
		filePath := filepath.Join(dir, "users.yaml")

		// 기존 파일 생성
		hash, err := HashPassword("custompass")
		require.NoError(t, err)

		content := "users:\n  - username: admin\n    password_hash: \"" + hash + "\"\n    role: superadmin\n"
		require.NoError(t, os.WriteFile(filePath, []byte(content), 0600))

		cm := NewCredentialsManager(filePath)
		require.NoError(t, cm.EnsureDefaultAdmin())

		// 기존 사용자 보존 확인
		admin, err := cm.GetUser("admin")
		require.NoError(t, err)
		assert.Equal(t, "superadmin", admin.Role)

		// 기존 비밀번호로 인증
		user, err := cm.Authenticate("admin", "custompass")
		require.NoError(t, err)
		assert.Equal(t, "admin", user.Username)
	})
}

func TestCredentialsManager_SavePermissions(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "users.yaml")

	cm := NewCredentialsManager(filePath)
	cm.users["test"] = &CredentialUser{
		Username:     "test",
		PasswordHash: "hash",
		Role:         "viewer",
	}

	require.NoError(t, cm.Save())

	info, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0600), info.Mode().Perm(), "자격증명 파일은 0600 권한이어야 한다")
}
