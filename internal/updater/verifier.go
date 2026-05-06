// @SPEC:SPEC-UPDATE-001 v0.1.0
// verifier.go — 다운로드된 바이너리의 무결성(SHA256) + 진위(Ed25519 서명) 검증.
//
// 보안 critical 모듈: 100% 분기 커버리지 목표 (acceptance.md TRUST 5 / Tested).
//
// 디자인 결정:
//   - SHA256 비교는 crypto/subtle.ConstantTimeCompare 로 timing-attack 방어
//   - Ed25519 검증은 crypto/ed25519 stdlib 사용 (외부 의존 없음)
//   - 입력 검증 fail-fast 후 본 검증 수행
//   - VerifyAll 은 checksum → signature 순서 (체크섬이 더 빠르고 cheap)
//
// 위협 모델:
//   - MITM: HTTPS + 공개키 핀닝 + Ed25519 서명 검증 (M13 강제)
//   - Replay: 채널 메타데이터 published_at 검증 (별도 모듈에서 처리)
//   - Timing attack: ConstantTimeCompare 사용 (SHA256 비교 시)
package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
)

// Verifier 는 SHA256 + Ed25519 검증을 수행하는 보안 critical 컴포넌트다.
//
// 사용 흐름:
//  1. NewVerifier(pubKey) 로 인스턴스 생성 (공개키 핀닝)
//  2. VerifyChecksum(content, expectedHex) 으로 무결성 확인
//  3. VerifySignature(content, signature) 으로 진위 확인
//  4. (또는) VerifyAll() 로 두 단계를 한 번에
//
// SPEC M4: 검증 단계는 다운로드 직후 + 원자적 교체 직전에 모두 수행 (TOCTOU 방어).
type Verifier struct {
	publicKey ed25519.PublicKey
}

// NewVerifier 는 공개키 핀닝 모드로 Verifier 를 생성한다.
//
// publicKey 는 Ed25519 raw 공개키 (32 bytes). 빌드 시 -ldflags 로 임베드된 키
// 또는 운영자가 명시적 public_key_path 로 지정한 PEM 파일에서 파싱된 결과를 받는다.
//
// 길이가 32 bytes 가 아니거나 nil 이면 ErrUpdateInvalidInput 반환.
func NewVerifier(publicKey ed25519.PublicKey) (*Verifier, error) {
	if len(publicKey) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: public key must be %d bytes (got %d)",
			ErrUpdateInvalidInput, ed25519.PublicKeySize, len(publicKey))
	}
	return &Verifier{publicKey: publicKey}, nil
}

// VerifyChecksum 은 content 의 SHA256 해시를 expectedHex (hex 인코딩) 와 비교한다.
//
// 입력 검증:
//   - content 가 비어 있으면 ErrUpdateInvalidInput
//   - expectedHex 가 빈 문자열이면 ErrUpdateInvalidInput
//   - expectedHex 가 hex 디코딩 실패 또는 32 bytes 가 아니면 ErrUpdateInvalidInput
//
// 비교 결과:
//   - 일치하면 nil
//   - 불일치하면 ErrUpdateChecksumMismatch
//
// 보안: ConstantTimeCompare 로 timing-attack 방어.
func (v *Verifier) VerifyChecksum(content []byte, expectedHex string) error {
	if len(content) == 0 {
		return fmt.Errorf("%w: empty content", ErrUpdateInvalidInput)
	}
	if expectedHex == "" {
		return fmt.Errorf("%w: empty expected hash", ErrUpdateInvalidInput)
	}
	expected, err := hex.DecodeString(expectedHex)
	if err != nil {
		return fmt.Errorf("%w: invalid hex hash: %v", ErrUpdateInvalidInput, err)
	}
	if len(expected) != sha256.Size {
		return fmt.Errorf("%w: expected hash must be %d bytes (got %d)",
			ErrUpdateInvalidInput, sha256.Size, len(expected))
	}

	actual := sha256.Sum256(content)
	if subtle.ConstantTimeCompare(actual[:], expected) != 1 {
		return ErrUpdateChecksumMismatch
	}
	return nil
}

// VerifySignature 는 content 의 Ed25519 서명을 검증한다.
//
// signature 는 64 bytes raw signature. 길이가 다르면 ErrUpdateInvalidInput.
// content 가 비어 있으면 ErrUpdateInvalidInput.
// 서명 검증 실패 시 ErrUpdateSignatureInvalid (보안 감사 로그 trigger).
//
// 보안: ed25519.Verify 는 자체적으로 timing-safe (stdlib).
func (v *Verifier) VerifySignature(content []byte, signature []byte) error {
	if len(content) == 0 {
		return fmt.Errorf("%w: empty content", ErrUpdateInvalidInput)
	}
	if len(signature) != ed25519.SignatureSize {
		return fmt.Errorf("%w: signature must be %d bytes (got %d)",
			ErrUpdateInvalidInput, ed25519.SignatureSize, len(signature))
	}
	if !ed25519.Verify(v.publicKey, content, signature) {
		return ErrUpdateSignatureInvalid
	}
	return nil
}

// VerifyAll 은 checksum 과 signature 를 순서대로 검증한다 (fail-fast).
//
// 순서:
//  1. VerifyChecksum (cheap, 무결성 확인)
//  2. VerifySignature (진위 확인)
//
// 어느 단계에서 실패해도 즉시 반환되며, 에러는 단계 정보를 wrapping 한다
// (errors.Is 로 sentinel 식별 가능).
func (v *Verifier) VerifyAll(content []byte, expectedChecksumHex string, signature []byte) error {
	if err := v.VerifyChecksum(content, expectedChecksumHex); err != nil {
		return fmt.Errorf("checksum: %w", err)
	}
	if err := v.VerifySignature(content, signature); err != nil {
		return fmt.Errorf("signature: %w", err)
	}
	return nil
}
