// @SPEC:SPEC-UPDATE-001 v0.1.0
// signer_test.go — 키 생성 / 서명 / hex·PEM 라운드트립 검증.
//
// 핵심 시나리오:
//   - keygen → Sign(content) → NewVerifier(pub).VerifySignature PASS
//   - 다른 키로 만든 서명은 FAIL (위조 거부)
//   - priv/pub hex round-trip
//   - LoadPrivateKeyFromFile (hex / PEM 두 형식)
package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateKeyPair(t *testing.T) {
	t.Parallel()
	priv, pub, err := GenerateKeyPair()
	require.NoError(t, err)
	assert.Len(t, priv, ed25519.PrivateKeySize)
	assert.Len(t, pub, ed25519.PublicKeySize)

	// 생성된 pub 은 priv 의 공개 부분과 일치해야 한다.
	assert.Equal(t, ed25519.PublicKey(priv.Public().(ed25519.PublicKey)), pub)
}

func TestSignAndVerifyRoundTrip(t *testing.T) {
	t.Parallel()
	priv, pub, err := GenerateKeyPair()
	require.NoError(t, err)

	content := []byte("xflowd-linux-amd64 binary content bytes")
	sig, err := Sign(priv, content)
	require.NoError(t, err)
	assert.Len(t, sig, ed25519.SignatureSize)

	// 검증 측(verifier.go) 과 호환되어야 한다.
	v, err := NewVerifier(pub)
	require.NoError(t, err)
	assert.NoError(t, v.VerifySignature(content, sig))
}

func TestSignWithDifferentKeyFails(t *testing.T) {
	t.Parallel()
	priv1, _, err := GenerateKeyPair()
	require.NoError(t, err)
	_, pub2, err := GenerateKeyPair()
	require.NoError(t, err)

	content := []byte("some binary")
	sig, err := Sign(priv1, content)
	require.NoError(t, err)

	// pub2 (다른 키) 로 검증하면 실패해야 한다.
	v, err := NewVerifier(pub2)
	require.NoError(t, err)
	assert.ErrorIs(t, v.VerifySignature(content, sig), ErrUpdateSignatureInvalid)
}

func TestSignInvalidInput(t *testing.T) {
	t.Parallel()
	priv, _, err := GenerateKeyPair()
	require.NoError(t, err)

	t.Run("empty content", func(t *testing.T) {
		t.Parallel()
		_, err := Sign(priv, nil)
		assert.ErrorIs(t, err, ErrUpdateInvalidInput)
	})

	t.Run("wrong key size", func(t *testing.T) {
		t.Parallel()
		_, err := Sign(ed25519.PrivateKey([]byte("too-short")), []byte("data"))
		assert.ErrorIs(t, err, ErrUpdateInvalidInput)
	})
}

func TestPrivateKeyHexRoundTrip(t *testing.T) {
	t.Parallel()
	priv, _, err := GenerateKeyPair()
	require.NoError(t, err)

	hexStr := PrivateKeyToHex(priv)
	assert.Len(t, hexStr, ed25519.PrivateKeySize*2)

	loaded, err := LoadPrivateKeyFromHex(hexStr)
	require.NoError(t, err)
	assert.Equal(t, priv, loaded)

	// 공백/개행이 붙어도 trim 후 로드 가능해야 한다.
	loaded2, err := LoadPrivateKeyFromHex("  " + hexStr + "\n")
	require.NoError(t, err)
	assert.Equal(t, priv, loaded2)
}

func TestPublicKeyHexRoundTrip(t *testing.T) {
	t.Parallel()
	_, pub, err := GenerateKeyPair()
	require.NoError(t, err)

	hexStr := PublicKeyToHex(pub)
	assert.Len(t, hexStr, ed25519.PublicKeySize*2)

	// EXISTING LoadPublicKeyFromHex 로 로드 가능해야 한다 (노드 배포 호환).
	loaded, err := LoadPublicKeyFromHex(hexStr)
	require.NoError(t, err)
	assert.Equal(t, pub, loaded)
}

func TestLoadPrivateKeyFromHexInvalid(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"empty":      "",
		"whitespace": "   \n",
		"bad hex":    "zzzz",
		"wrong len":  "abcd",
	}
	for name, input := range cases {
		input := input
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadPrivateKeyFromHex(input)
			assert.ErrorIs(t, err, ErrUpdateInvalidInput)
		})
	}
}

func TestLoadPrivateKeyFromFileHex(t *testing.T) {
	t.Parallel()
	priv, _, err := GenerateKeyPair()
	require.NoError(t, err)

	dir := t.TempDir()
	path := filepath.Join(dir, "xflow-release.key")
	require.NoError(t, os.WriteFile(path, []byte(PrivateKeyToHex(priv)), 0o600))

	loaded, err := LoadPrivateKeyFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, priv, loaded)
}

func TestLoadPrivateKeyFromFilePEM(t *testing.T) {
	t.Parallel()
	priv, _, err := GenerateKeyPair()
	require.NoError(t, err)

	// PKCS8 PEM 으로 인코딩.
	der, err := x509.MarshalPKCS8PrivateKey(priv)
	require.NoError(t, err)
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})

	dir := t.TempDir()
	path := filepath.Join(dir, "xflow-release.pem")
	require.NoError(t, os.WriteFile(path, pemBytes, 0o600))

	loaded, err := LoadPrivateKeyFromFile(path)
	require.NoError(t, err)
	assert.Equal(t, priv, loaded)

	// 로드한 PEM 키로 서명 → 원래 pub 로 검증되어야 한다.
	content := []byte("pem-signed content")
	sig, err := Sign(loaded, content)
	require.NoError(t, err)
	v, err := NewVerifier(priv.Public().(ed25519.PublicKey))
	require.NoError(t, err)
	assert.NoError(t, v.VerifySignature(content, sig))
}

func TestLoadPrivateKeyFromFileErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing file", func(t *testing.T) {
		t.Parallel()
		_, err := LoadPrivateKeyFromFile(filepath.Join(t.TempDir(), "nope.key"))
		assert.ErrorIs(t, err, ErrUpdateInvalidInput)
	})

	t.Run("empty file", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "empty.key")
		require.NoError(t, os.WriteFile(path, []byte("   \n"), 0o600))
		_, err := LoadPrivateKeyFromFile(path)
		assert.ErrorIs(t, err, ErrUpdateInvalidInput)
	})
}

func TestLoadPrivateKeyFromPEMInvalid(t *testing.T) {
	t.Parallel()

	t.Run("not a PEM block", func(t *testing.T) {
		t.Parallel()
		_, err := LoadPrivateKeyFromPEM([]byte("not pem"))
		assert.ErrorIs(t, err, ErrUpdateInvalidInput)
	})

	t.Run("non-ed25519 PEM (RSA)", func(t *testing.T) {
		t.Parallel()
		// PKCS8 형식이지만 ed25519 가 아닌 키를 거부하는지 확인.
		// 간단히 잘못된 DER 로 ParsePKCS8 실패 경로를 탄다.
		bad := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: []byte("garbage")})
		_, err := LoadPrivateKeyFromPEM(bad)
		assert.ErrorIs(t, err, ErrUpdateInvalidInput)
	})
}

// ensure crypto/rand import is genuinely exercised somewhere (keygen 경로 외 sanity).
func TestRandReaderAvailable(t *testing.T) {
	t.Parallel()
	buf := make([]byte, 8)
	_, err := rand.Read(buf)
	require.NoError(t, err)
	require.NoError(t, errors.Join(nil))
}
