// @SPEC:SPEC-UPDATE-001 v0.1.0
// keys.go — Ed25519 공개키 로더 (PEM/hex/file 입력 형식 지원).
//
// 보안 critical: 공개키 핀닝의 입구. RSA 등 비-Ed25519 알고리즘은 거부.
//
// 디자인:
//   - PKCS#8 PEM 형식 (-----BEGIN PUBLIC KEY-----) 또는
//   - 64-char hex (32-byte raw key) 또는
//   - 파일 경로 (확장자 무관, 내용으로 분기)
//
// 모든 입력 검증 실패는 ErrUpdateInvalidInput 으로 wrap.
package updater

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

// LoadPublicKeyFromPEM 은 PKCS#8 PEM 인코딩된 Ed25519 공개키를 파싱한다.
//
// 입력 검증:
//   - pem.Decode 실패 (block == nil) → ErrUpdateInvalidInput
//   - x509.ParsePKIXPublicKey 실패 → ErrUpdateInvalidInput
//   - 알고리즘이 Ed25519 가 아님 (RSA/ECDSA 등) → ErrUpdateInvalidInput
//
// 보안: 공개키 핀닝의 마지막 게이트; 알고리즘 sanity check 필수.
func LoadPublicKeyFromPEM(pemBytes []byte) (ed25519.PublicKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("%w: invalid PEM block", ErrUpdateInvalidInput)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse PKIX public key: %v", ErrUpdateInvalidInput, err)
	}
	edKey, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: public key is not Ed25519 (got %T)", ErrUpdateInvalidInput, pub)
	}
	if len(edKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: ed25519 key has wrong size %d", ErrUpdateInvalidInput, len(edKey))
	}
	return edKey, nil
}

// LoadPublicKeyFromHex 은 64-char hex 문자열 (32 bytes 디코딩) 을 Ed25519 공개키로 로드한다.
//
// 앞뒤 공백/개행은 제거된다.
//
// 입력 검증:
//   - hex 디코딩 실패 → ErrUpdateInvalidInput
//   - 디코딩 결과가 ed25519.PublicKeySize (32 bytes) 가 아님 → ErrUpdateInvalidInput
func LoadPublicKeyFromHex(s string) (ed25519.PublicKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: empty hex key", ErrUpdateInvalidInput)
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: hex decode: %v", ErrUpdateInvalidInput, err)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: key length %d (want %d)",
			ErrUpdateInvalidInput, len(raw), ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(raw), nil
}

// LoadPublicKeyFromFile 은 파일 내용을 PEM 또는 hex 로 자동 분기 로드한다.
//
// 분기 기준: 파일 내용에 "BEGIN" 토큰이 포함되어 있으면 PEM, 없으면 hex.
//
// 입력 검증:
//   - 파일 열기 실패 → ErrUpdateInvalidInput
//   - 빈 파일 → ErrUpdateInvalidInput
//   - PEM/hex 단계의 모든 실패 → ErrUpdateInvalidInput (wrapping 보존)
func LoadPublicKeyFromFile(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path 는 운영자가 설정한 신뢰 경로
	if err != nil {
		return nil, fmt.Errorf("%w: open key file %q: %v", ErrUpdateInvalidInput, path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: key file %q is empty", ErrUpdateInvalidInput, path)
	}
	if strings.Contains(string(data), "BEGIN") {
		return LoadPublicKeyFromPEM(data)
	}
	return LoadPublicKeyFromHex(string(data))
}
