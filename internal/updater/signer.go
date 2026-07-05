// @SPEC:SPEC-UPDATE-001 v0.1.0
// signer.go — 릴리스 이미지 생성용 Ed25519 키 생성 + 바이너리 서명.
//
// verifier.go 의 검증 측(Verifier.VerifySignature) 과 짝을 이루는 생성 측이다.
// 서명은 바이너리의 raw content (바이트 전체) 에 대해 수행되며, checksum 이 아니다.
//
// 디자인 결정:
//   - GenerateKeyPair 는 crypto/ed25519 + crypto/rand stdlib 사용 (외부 의존 없음)
//   - 공개키 hex 출력은 EXISTING LoadPublicKeyFromHex 로 그대로 로드 가능 (64 hex chars)
//   - 개인키는 128 hex chars (64-byte ed25519 private key) 형식으로 저장/로드
//   - LoadPrivateKeyFromFile 은 "BEGIN" 토큰으로 PEM/hex 자동 분기 (keys.go 와 동일 패턴)
//   - 모든 입력 검증 실패는 ErrUpdateInvalidInput 으로 wrap (패키지 일관성)
//
// 보안:
//   - 개인키는 절대 커밋/로깅 금지. 운영자가 .key 파일로 오프라인 보관.
//   - Sign 은 ed25519.Sign 자체가 timing-safe (stdlib).
package updater

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
)

// GenerateKeyPair 는 새 Ed25519 키쌍을 생성한다.
//
// crypto/rand.Reader 를 엔트로피 소스로 사용하므로 cryptographically secure 하다.
// 반환된 pub 은 PublicKeyToHex 로 hex 화하여 노드의 update.public_key_path 에 배포하고,
// priv 은 PrivateKeyToHex 로 hex 화하여 운영자가 오프라인 보관한다.
func GenerateKeyPair() (priv ed25519.PrivateKey, pub ed25519.PublicKey, err error) {
	pub, priv, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: generate ed25519 key pair: %v", ErrUpdateInvalidInput, err)
	}
	return priv, pub, nil
}

// PrivateKeyToHex 는 Ed25519 개인키 (64 bytes) 를 128-char hex 문자열로 인코딩한다.
//
// 이 출력은 LoadPrivateKeyFromHex 로 round-trip 가능하다.
func PrivateKeyToHex(priv ed25519.PrivateKey) string {
	return hex.EncodeToString(priv)
}

// PublicKeyToHex 는 Ed25519 공개키 (32 bytes) 를 64-char hex 문자열로 인코딩한다.
//
// 이 출력은 EXISTING LoadPublicKeyFromHex 로 round-trip 가능하다 (노드 배포용).
func PublicKeyToHex(pub ed25519.PublicKey) string {
	return hex.EncodeToString(pub)
}

// LoadPrivateKeyFromHex 는 128-char hex 문자열 (64-byte ed25519 개인키) 을 로드한다.
//
// 앞뒤 공백/개행은 제거된다.
//
// 입력 검증:
//   - 빈 문자열 → ErrUpdateInvalidInput
//   - hex 디코딩 실패 → ErrUpdateInvalidInput
//   - 디코딩 결과가 ed25519.PrivateKeySize (64 bytes) 가 아님 → ErrUpdateInvalidInput
func LoadPrivateKeyFromHex(s string) (ed25519.PrivateKey, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("%w: empty hex private key", ErrUpdateInvalidInput)
	}
	raw, err := hex.DecodeString(s)
	if err != nil {
		return nil, fmt.Errorf("%w: hex decode private key: %v", ErrUpdateInvalidInput, err)
	}
	if len(raw) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: private key length %d (want %d)",
			ErrUpdateInvalidInput, len(raw), ed25519.PrivateKeySize)
	}
	return ed25519.PrivateKey(raw), nil
}

// LoadPrivateKeyFromPEM 은 PKCS#8 PEM 인코딩된 Ed25519 개인키를 파싱한다.
//
// 입력 검증:
//   - pem.Decode 실패 (block == nil) → ErrUpdateInvalidInput
//   - x509.ParsePKCS8PrivateKey 실패 → ErrUpdateInvalidInput
//   - 알고리즘이 Ed25519 가 아님 → ErrUpdateInvalidInput
func LoadPrivateKeyFromPEM(pemBytes []byte) (ed25519.PrivateKey, error) {
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, fmt.Errorf("%w: invalid PEM block (private key)", ErrUpdateInvalidInput)
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse PKCS8 private key: %v", ErrUpdateInvalidInput, err)
	}
	edKey, ok := key.(ed25519.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: private key is not Ed25519 (got %T)", ErrUpdateInvalidInput, key)
	}
	if len(edKey) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: ed25519 private key has wrong size %d", ErrUpdateInvalidInput, len(edKey))
	}
	return edKey, nil
}

// LoadPrivateKeyFromFile 은 파일 내용을 PEM 또는 hex 로 자동 분기 로드한다.
//
// 분기 기준: 파일 내용에 "BEGIN" 토큰이 포함되어 있으면 PEM, 없으면 hex
// (keys.go 의 LoadPublicKeyFromFile 과 동일 패턴).
//
// 입력 검증:
//   - 파일 열기 실패 → ErrUpdateInvalidInput
//   - 빈 파일 → ErrUpdateInvalidInput
//   - PEM/hex 단계의 모든 실패 → ErrUpdateInvalidInput (wrapping 보존)
func LoadPrivateKeyFromFile(path string) (ed25519.PrivateKey, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path 는 운영자가 설정한 신뢰 경로
	if err != nil {
		return nil, fmt.Errorf("%w: open private key file %q: %v", ErrUpdateInvalidInput, path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, fmt.Errorf("%w: private key file %q is empty", ErrUpdateInvalidInput, path)
	}
	if strings.Contains(string(data), "BEGIN") {
		return LoadPrivateKeyFromPEM(data)
	}
	return LoadPrivateKeyFromHex(string(data))
}

// Sign 은 content 의 Ed25519 서명 (64 bytes raw) 을 생성한다.
//
// 서명 대상은 content 의 바이트 전체 (바이너리 자체) 이며, checksum 이 아니다.
// 이 서명은 Verifier.VerifySignature(content, sig) 로 검증된다.
//
// 입력 검증:
//   - content 가 비어 있으면 ErrUpdateInvalidInput
//   - priv 의 길이가 ed25519.PrivateKeySize (64 bytes) 가 아니면 ErrUpdateInvalidInput
//
// 보안: ed25519.Sign 은 자체적으로 timing-safe (stdlib).
func Sign(priv ed25519.PrivateKey, content []byte) ([]byte, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("%w: empty content", ErrUpdateInvalidInput)
	}
	if len(priv) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("%w: private key must be %d bytes (got %d)",
			ErrUpdateInvalidInput, ed25519.PrivateKeySize, len(priv))
	}
	return ed25519.Sign(priv, content), nil
}
