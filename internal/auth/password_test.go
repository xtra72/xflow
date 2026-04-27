package auth

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashPassword(t *testing.T) {
	tests := []struct {
		name     string
		password string
		wantErr  error
	}{
		{
			name:     "유효한 비밀번호",
			password: "admin",
			wantErr:  nil,
		},
		{
			name:     "긴 비밀번호",
			password: "very-long-password-1234567890",
			wantErr:  nil,
		},
		{
			name:     "최소 길이 비밀번호",
			password: "abcd",
			wantErr:  nil,
		},
		{
			name:     "너무 짧은 비밀번호",
			password: "abc",
			wantErr:  ErrPasswordTooShort,
		},
		{
			name:     "빈 비밀번호",
			password: "",
			wantErr:  ErrPasswordTooShort,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hash, err := HashPassword(tc.password)
			if tc.wantErr != nil {
				assert.ErrorIs(t, err, tc.wantErr)
				assert.Empty(t, hash)
				return
			}
			require.NoError(t, err)
			assert.NotEmpty(t, hash)
			assert.NotEqual(t, tc.password, hash, "해시는 평문과 달라야 한다")
		})
	}
}

func TestCheckPassword(t *testing.T) {
	password := "test-password"
	hash, err := HashPassword(password)
	require.NoError(t, err)

	tests := []struct {
		name    string
		hash    string
		pass    string
		wantErr bool
	}{
		{
			name:    "올바른 비밀번호",
			hash:    hash,
			pass:    password,
			wantErr: false,
		},
		{
			name:    "잘못된 비밀번호",
			hash:    hash,
			pass:    "wrong-password",
			wantErr: true,
		},
		{
			name:    "빈 비밀번호",
			hash:    hash,
			pass:    "",
			wantErr: true,
		},
		{
			name:    "잘못된 해시",
			hash:    "invalid-hash",
			pass:    password,
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckPassword(tc.hash, tc.pass)
			if tc.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestHashPassword_DifferentHashesForSamePassword(t *testing.T) {
	password := "same-password"
	hash1, err := HashPassword(password)
	require.NoError(t, err)
	hash2, err := HashPassword(password)
	require.NoError(t, err)

	assert.NotEqual(t, hash1, hash2, "같은 비밀번호라도 해시가 달라야 한다 (salt)")
	assert.NoError(t, CheckPassword(hash1, password))
	assert.NoError(t, CheckPassword(hash2, password))
}
