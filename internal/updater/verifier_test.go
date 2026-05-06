// @SPEC:SPEC-UPDATE-001 v0.1.0
// verifier_test.go — Phase A 보안 critical 검증자 테스트.
// 목표: 100% 분기 커버리지 (Ed25519 서명 + SHA256 체크섬 + 입력 검증).
package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: ephemeral Ed25519 keypair 생성.
func newKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	require.Len(t, pub, ed25519.PublicKeySize)
	require.Len(t, priv, ed25519.PrivateKeySize)
	return pub, priv
}

// ---- NewVerifier 입력 검증 ----

// TestNewVerifier_ValidKey 는 정상 공개키로 Verifier 생성을 검증한다.
func TestNewVerifier_ValidKey(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)
	require.NotNil(t, v)
}

// TestNewVerifier_NilKey 는 nil 공개키 거부를 검증한다.
func TestNewVerifier_NilKey(t *testing.T) {
	v, err := NewVerifier(nil)
	assert.Nil(t, v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestNewVerifier_WrongKeySize 는 잘못된 길이의 공개키 거부를 검증한다.
func TestNewVerifier_WrongKeySize(t *testing.T) {
	short := ed25519.PublicKey(make([]byte, 16))
	v, err := NewVerifier(short)
	assert.Nil(t, v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))

	long := ed25519.PublicKey(make([]byte, 64))
	v, err = NewVerifier(long)
	assert.Nil(t, v)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// ---- VerifySignature ----

// TestVerifier_ValidEd25519Signature 는 정상 서명 검증 성공을 확인한다.
func TestVerifier_ValidEd25519Signature(t *testing.T) {
	pub, priv := newKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	content := []byte("legitimate xflowd binary content")
	sig := ed25519.Sign(priv, content)

	err = v.VerifySignature(content, sig)
	assert.NoError(t, err)
}

// TestVerifier_TamperedBinary_ErrSignatureInvalid 는 변조된 바이너리 거부를 검증한다.
func TestVerifier_TamperedBinary_ErrSignatureInvalid(t *testing.T) {
	pub, priv := newKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	original := []byte("original binary")
	sig := ed25519.Sign(priv, original)

	tampered := []byte("TAMPERED binary")
	err = v.VerifySignature(tampered, sig)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateSignatureInvalid))
}

// TestVerifier_TamperedSignature_ErrSignatureInvalid 는 변조된 서명 거부를 검증한다.
func TestVerifier_TamperedSignature_ErrSignatureInvalid(t *testing.T) {
	pub, priv := newKeyPair(t)
	v, err := NewVerifier(pub)
	require.NoError(t, err)

	content := []byte("binary content")
	sig := ed25519.Sign(priv, content)
	// 첫 바이트 변조
	sig[0] ^= 0xFF

	err = v.VerifySignature(content, sig)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateSignatureInvalid))
}

// TestVerifier_WrongPublicKey_ErrSignatureInvalid 는 공개키 불일치 거부를 검증한다.
func TestVerifier_WrongPublicKey_ErrSignatureInvalid(t *testing.T) {
	_, priv := newKeyPair(t)
	pubB, _ := newKeyPair(t) // 다른 키쌍
	v, err := NewVerifier(pubB)
	require.NoError(t, err)

	content := []byte("binary signed by key A, verified with key B")
	sig := ed25519.Sign(priv, content)

	err = v.VerifySignature(content, sig)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateSignatureInvalid))
}

// TestVerifier_VerifySignature_EmptyContent 는 빈 콘텐츠 거부를 검증한다.
func TestVerifier_VerifySignature_EmptyContent(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	err := v.VerifySignature([]byte{}, make([]byte, ed25519.SignatureSize))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))

	err = v.VerifySignature(nil, make([]byte, ed25519.SignatureSize))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestVerifier_InvalidSignatureLength_ErrInvalidInput 는 잘못된 서명 길이 거부를 검증한다.
func TestVerifier_InvalidSignatureLength_ErrInvalidInput(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	tests := []struct {
		name string
		sig  []byte
	}{
		{"empty", []byte{}},
		{"short", make([]byte, 32)},
		{"long", make([]byte, 128)},
		{"nil", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.VerifySignature([]byte("content"), tc.sig)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
		})
	}
}

// ---- VerifyChecksum ----

// TestVerifier_VerifyChecksum_Match 는 SHA256 매칭 정상 케이스를 검증한다.
func TestVerifier_VerifyChecksum_Match(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	content := []byte("binary content for checksum test")
	hash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(hash[:])

	err := v.VerifyChecksum(content, expectedHex)
	assert.NoError(t, err)
}

// TestVerifier_VerifyChecksum_Mismatch 는 SHA256 불일치 거부를 검증한다.
func TestVerifier_VerifyChecksum_Mismatch(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	content := []byte("real content")
	wrongHash := sha256.Sum256([]byte("different content"))
	wrongHex := hex.EncodeToString(wrongHash[:])

	err := v.VerifyChecksum(content, wrongHex)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChecksumMismatch))
}

// TestVerifier_VerifyChecksum_EmptyContent 는 빈 콘텐츠 입력 거부를 검증한다.
func TestVerifier_VerifyChecksum_EmptyContent(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	hash := sha256.Sum256([]byte("any"))
	expectedHex := hex.EncodeToString(hash[:])

	err := v.VerifyChecksum([]byte{}, expectedHex)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))

	err = v.VerifyChecksum(nil, expectedHex)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestVerifier_VerifyChecksum_EmptyExpected 는 빈 expected hash 입력 거부를 검증한다.
func TestVerifier_VerifyChecksum_EmptyExpected(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	err := v.VerifyChecksum([]byte("content"), "")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// TestVerifier_VerifyChecksum_InvalidHex 는 hex 디코딩 실패 케이스를 검증한다.
func TestVerifier_VerifyChecksum_InvalidHex(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	tests := []struct {
		name string
		hex  string
	}{
		{"odd length", "abc"},
		{"non-hex char", "ZZZZ" + strings.Repeat("0", 60)}, // 64 chars, but Z invalid
		{"with spaces", "ab cd" + strings.Repeat("0", 59)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := v.VerifyChecksum([]byte("content"), tc.hex)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
		})
	}
}

// TestVerifier_VerifyChecksum_WrongHashLength 는 32 byte 외 hash 길이 거부를 검증한다.
func TestVerifier_VerifyChecksum_WrongHashLength(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	// 16 byte hash (valid hex but wrong length)
	short := hex.EncodeToString(make([]byte, 16))
	err := v.VerifyChecksum([]byte("content"), short)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))

	// 64 byte hash (valid hex but wrong length)
	long := hex.EncodeToString(make([]byte, 64))
	err = v.VerifyChecksum([]byte("content"), long)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}

// ---- VerifyAll (조합 검증, fail-fast 순서) ----

// TestVerifier_VerifyAll_BothPass 는 checksum + signature 모두 통과하는 정상 케이스.
func TestVerifier_VerifyAll_BothPass(t *testing.T) {
	pub, priv := newKeyPair(t)
	v, _ := NewVerifier(pub)

	content := []byte("xflowd binary v0.4.0")
	hash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(hash[:])
	sig := ed25519.Sign(priv, content)

	err := v.VerifyAll(content, expectedHex, sig)
	assert.NoError(t, err)
}

// TestVerifier_VerifyAll_ChecksumFails 는 checksum 단계에서 fail-fast 동작을 검증한다.
func TestVerifier_VerifyAll_ChecksumFails(t *testing.T) {
	pub, priv := newKeyPair(t)
	v, _ := NewVerifier(pub)

	content := []byte("real content")
	wrongHash := sha256.Sum256([]byte("different"))
	wrongHex := hex.EncodeToString(wrongHash[:])
	sig := ed25519.Sign(priv, content) // 정상 서명이지만 checksum 먼저 fail

	err := v.VerifyAll(content, wrongHex, sig)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChecksumMismatch),
		"checksum 이 먼저 실패해야 함 (fail-fast)")
}

// TestVerifier_VerifyAll_SignatureFails 는 checksum 통과 후 signature 실패 케이스.
func TestVerifier_VerifyAll_SignatureFails(t *testing.T) {
	pub, _ := newKeyPair(t)
	_, otherPriv := newKeyPair(t)
	v, _ := NewVerifier(pub)

	content := []byte("binary")
	hash := sha256.Sum256(content)
	expectedHex := hex.EncodeToString(hash[:])
	// 다른 키로 서명 (checksum 은 통과, signature 만 fail)
	sig := ed25519.Sign(otherPriv, content)

	err := v.VerifyAll(content, expectedHex, sig)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateSignatureInvalid))
	// checksum 에러가 아닌지 확인
	assert.False(t, errors.Is(err, ErrUpdateChecksumMismatch))
}

// TestVerifier_VerifyAll_InvalidInput 은 입력 검증 단계에서 fail-fast 를 확인한다.
func TestVerifier_VerifyAll_InvalidInput(t *testing.T) {
	pub, _ := newKeyPair(t)
	v, _ := NewVerifier(pub)

	// 빈 콘텐츠 → checksum 단계에서 InvalidInput
	err := v.VerifyAll(nil, "abc", make([]byte, ed25519.SignatureSize))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
}
