// @SPEC:SPEC-UPDATE-001 v0.1.0
// keys_test.go — Phase B 단위 테스트: Ed25519 공개키 로더 (PEM/hex/file).
//
// 테스트 전략:
//   - 모든 분기 (PEM/hex/file 분배 + 입력 검증 fail-fast) 커버
//   - Ed25519 외 알고리즘 (RSA) 거부 검증
//   - PKCS#8 invariant: x509.MarshalPKIXPublicKey 결과를 round-trip
package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperGenerateEd25519PEM 은 테스트용 Ed25519 공개키를 PKCS#8 PEM 으로 인코딩한다.
func helperGenerateEd25519PEM(t *testing.T) (ed25519.PublicKey, []byte) {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(pub)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
	return pub, pemBytes
}

// helperGenerateRSAPEM 은 테스트용 RSA 공개키를 PEM 으로 인코딩한다 (거부 케이스용).
func helperGenerateRSAPEM(t *testing.T) []byte {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})
}

// TestLoadPublicKeyFromPEM_Valid 는 정상 Ed25519 PEM 로드를 검증한다.
func TestLoadPublicKeyFromPEM_Valid(t *testing.T) {
	want, pemBytes := helperGenerateEd25519PEM(t)

	got, err := LoadPublicKeyFromPEM(pemBytes)
	require.NoError(t, err)
	assert.Equal(t, want, got, "round-trip mismatch")
	assert.Len(t, got, ed25519.PublicKeySize)
}

// TestLoadPublicKeyFromPEM_RSAKey_Rejected 는 비-Ed25519 알고리즘 거부를 검증한다.
func TestLoadPublicKeyFromPEM_RSAKey_Rejected(t *testing.T) {
	pemBytes := helperGenerateRSAPEM(t)

	_, err := LoadPublicKeyFromPEM(pemBytes)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput), "expected ErrUpdateInvalidInput, got %v", err)
}

// TestLoadPublicKeyFromPEM_MalformedPEM 은 잘못된 PEM 입력을 거부한다.
func TestLoadPublicKeyFromPEM_MalformedPEM(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty bytes", []byte{}},
		{"plain text", []byte("not a pem block at all")},
		{"corrupt der", []byte("-----BEGIN PUBLIC KEY-----\nXXXXXX\n-----END PUBLIC KEY-----\n")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadPublicKeyFromPEM(tc.input)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateInvalidInput), "expected ErrUpdateInvalidInput, got %v", err)
		})
	}
}

// TestLoadPublicKeyFromHex_Valid 는 정상 hex 키 로드를 검증한다.
func TestLoadPublicKeyFromHex_Valid(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hexStr := hex.EncodeToString(pub)

	got, err := LoadPublicKeyFromHex(hexStr)
	require.NoError(t, err)
	assert.Equal(t, ed25519.PublicKey(pub), got)
}

// TestLoadPublicKeyFromHex_TrimsWhitespace 는 hex 입력에 trailing newline/공백이 있어도 처리한다.
func TestLoadPublicKeyFromHex_TrimsWhitespace(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	hexStr := "  " + hex.EncodeToString(pub) + "\n  "

	got, err := LoadPublicKeyFromHex(hexStr)
	require.NoError(t, err)
	assert.Equal(t, ed25519.PublicKey(pub), got)
}

// TestLoadPublicKeyFromHex_WrongLength 는 잘못된 길이의 hex 입력을 거부한다.
func TestLoadPublicKeyFromHex_WrongLength(t *testing.T) {
	tests := []string{
		"",                                   // empty
		"ab",                                 // 1 byte
		hex.EncodeToString(make([]byte, 16)), // 16 bytes
		hex.EncodeToString(make([]byte, 64)), // 64 bytes (private key size, not pub)
	}
	for _, h := range tests {
		t.Run("len="+h, func(t *testing.T) {
			_, err := LoadPublicKeyFromHex(h)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
		})
	}
}

// TestLoadPublicKeyFromHex_InvalidChars 는 hex 디코딩 실패를 거부한다.
func TestLoadPublicKeyFromHex_InvalidChars(t *testing.T) {
	// 64자이지만 hex 가 아닌 문자 포함
	bad := "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"
	_, err := LoadPublicKeyFromHex(bad)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestLoadPublicKeyFromFile_PEM 는 파일에서 PEM 형식을 로드한다.
func TestLoadPublicKeyFromFile_PEM(t *testing.T) {
	want, pemBytes := helperGenerateEd25519PEM(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "key.pem")
	require.NoError(t, os.WriteFile(path, pemBytes, 0o600))

	got, err := LoadPublicKeyFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, got)
}

// TestLoadPublicKeyFromFile_Hex 는 파일에서 hex 형식을 로드한다.
func TestLoadPublicKeyFromFile_Hex(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	dir := t.TempDir()
	path := filepath.Join(dir, "key.hex")
	require.NoError(t, os.WriteFile(path, []byte(hex.EncodeToString(pub)+"\n"), 0o600))

	got, err := LoadPublicKeyFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, ed25519.PublicKey(pub), got)
}

// TestLoadPublicKeyFromFile_Missing 은 존재하지 않는 파일 경로를 거부한다.
func TestLoadPublicKeyFromFile_Missing(t *testing.T) {
	_, err := LoadPublicKeyFromFile("/nonexistent/path/to/key.pem")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestLoadPublicKeyFromFile_Empty 은 빈 파일을 거부한다.
func TestLoadPublicKeyFromFile_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty")
	require.NoError(t, os.WriteFile(path, []byte{}, 0o600))

	_, err := LoadPublicKeyFromFile(path)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}
