// @SPEC:SPEC-UPDATE-002 v0.1.0 (M11, M12, M13)
// manifest.go — 의존성 매트릭스 (multi-binary compatibility manifest).
//
// SPEC-UPDATE-001 의 Manifest (binary checksum 메타데이터) 와는 별개로,
// 본 모듈은 멀티 바이너리 환경에서 어느 버전이 어떤 환경 바이너리들과 호환되는지를
// 선언하는 dependency manifest 를 다룬다.
//
// 본 manifest 는 release asset 옆에 manifest.json + manifest.json.sig 로 동봉된다:
//
//	{
//	  "version": "v0.4.0",
//	  "binary": "xflow-agent",
//	  "compat": {
//	    "xflowd": ">=v0.4.0",
//	    "xflow":  ">=v0.3.0"
//	  }
//	}
//
// 보안 critical:
//   - manifest 자체도 Ed25519 서명 검증 (위조 방지) — 단일 공개키 사용 (M12)
//   - HTTPS 강제 (manifest URL + sig URL)
//   - 64KB 크기 cap (DoS 방어)
//   - constraint 문자열은 anchored regex 사용 (ReDoS 방어)
//   - 호환성 위반 시 ErrUpdateIncompatibleVersion (M13)
package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// DependencyManifest 는 release asset 옆에 동봉된 의존성 manifest 이다.
//
// SPEC-UPDATE-002 v0.1.0 (M11):
//   - Version: 본 release 의 버전 (semver)
//   - Binary: 본 manifest 가 설명하는 바이너리 이름 ("xflowd" / "xflow-agent" / "xflow")
//   - Compat: 환경 내 다른 바이너리들에 대한 semver 제약 매핑
//
// 비교: SPEC-UPDATE-001 의 Manifest (types.go) 는 단일 바이너리의 SHA256 + Ed25519 서명을
// 담는 데 비해, 본 DependencyManifest 는 multi-binary 호환성을 명시한다.
type DependencyManifest struct {
	// Version 은 본 manifest 가 설명하는 release 의 버전.
	Version Version `json:"version"`
	// Binary 는 본 manifest 가 설명하는 바이너리 이름 (whitelist 검증은 호출자 책임).
	Binary string `json:"binary"`
	// Compat 은 환경 내 다른 바이너리에 대한 semver 제약 매핑.
	// key: 바이너리 이름 (예: "xflowd"), value: 제약 (예: ">=v0.4.0").
	// nil 또는 empty map 은 제약 없음 → 항상 호환.
	Compat map[string]string `json:"compat,omitempty"`
}

// IsValid 는 DependencyManifest 의 기본 구조 무결성을 검증한다.
//
// 검사 항목:
//   - Version 이 semver 형식
//   - Binary 가 빈 문자열 아님
//
// Compat 의 개별 제약 형식 검증은 CheckConstraint 호출 시점에 수행된다.
func (m DependencyManifest) IsValid() bool {
	if !m.Version.IsValid() {
		return false
	}
	if m.Binary == "" {
		return false
	}
	return true
}

// semverConstraintRe 는 "<op><version>" 형식의 제약을 매칭한다 (anchored, ReDoS 방어).
//
// 지원 연산자: >=, <=, ==, !=, >, < (긴 것 먼저 매칭하기 위한 alternation 순서 유지)
// version 부분은 Version 의 semver regex 와 동일 (v 접두사 + 3-part + optional prerelease)
var semverConstraintRe = regexp.MustCompile(
	`^(>=|<=|==|!=|>|<)v(\d+)\.(\d+)\.(\d+)(?:-([\w][\w.-]*))?$`)

// CheckConstraint 는 currentVersion 이 constraint 를 만족하는지 검증한다.
//
// constraint 형식: "<op><version>" (예: ">=v0.4.0", "<v1.0.0", "==v0.4.0")
//
// 반환값:
//   - 만족: nil
//   - 위반: ErrUpdateIncompatibleVersion (with context: 어떤 비교 실패했는지)
//   - 형식 오류: ErrUpdateInvalidInput (constraint 자체 또는 currentVersion 이 invalid)
//
// 보안:
//   - regex 는 anchored (^...$) → ReDoS 방어
//   - alternation 순서가 긴 연산자 우선 (>= 가 > 보다 먼저)
//
// SPEC-UPDATE-002 v0.1.0 (M11):
//
//	각 manifest.compat[binary] 값이 본 함수로 검증된다.
func CheckConstraint(currentVersion Version, constraint string) error {
	trimmed := strings.TrimSpace(constraint)
	if trimmed == "" {
		return fmt.Errorf("%w: empty constraint string", ErrUpdateInvalidInput)
	}

	matches := semverConstraintRe.FindStringSubmatch(trimmed)
	if matches == nil {
		return fmt.Errorf("%w: invalid constraint format %q (expected forms: >=v0.4.0, <v1.0.0, ==v0.4.0)",
			ErrUpdateInvalidInput, constraint)
	}

	op := matches[1]
	// matches[2..5] 은 version 부분. 재구성하여 Version 으로 변환.
	target := Version("v" + matches[2] + "." + matches[3] + "." + matches[4])
	if matches[5] != "" {
		target = target + "-" + Version(matches[5])
	}

	if !currentVersion.IsValid() {
		return fmt.Errorf("%w: invalid current version %q", ErrUpdateInvalidInput, currentVersion)
	}

	cmp := currentVersion.Compare(target)

	var ok bool
	switch op {
	case ">=":
		ok = cmp >= 0
	case "<=":
		ok = cmp <= 0
	case "==":
		ok = cmp == 0
	case "!=":
		ok = cmp != 0
	case ">":
		ok = cmp > 0
	case "<":
		ok = cmp < 0
	default:
		// regex 가 op 를 alternation 으로 제한하므로 도달 불가.
		return fmt.Errorf("%w: unknown operator %q", ErrUpdateInvalidInput, op)
	}

	if !ok {
		return fmt.Errorf("%w: %s %s %s",
			ErrUpdateIncompatibleVersion, currentVersion, op, target)
	}
	return nil
}

// ManifestFetcher 는 dependency manifest 를 다운로드 + Ed25519 서명 검증 + 파싱한다.
//
// 보안:
//   - HTTPS 전용 (manifest URL + sig URL 둘 다 검증)
//   - manifest 본문에 대한 Ed25519 서명 검증 (단일 공개키, M12)
//   - 64KB 크기 cap (DoS 방어)
//
// 사용 예:
//
//	verifier, _ := NewVerifier(pubKey)
//	fetcher := &ManifestFetcher{HTTPClient: http.DefaultClient, Verifier: verifier}
//	dm, err := fetcher.FetchAndVerify(ctx, manifestURL, sigURL)
type ManifestFetcher struct {
	// HTTPClient 는 manifest / sig 다운로드용 클라이언트.
	// nil 이면 http.DefaultClient 가 사용된다.
	HTTPClient *http.Client
	// Verifier 는 manifest 본문에 대한 Ed25519 서명을 검증한다.
	// nil 이면 FetchAndVerify 가 즉시 ErrUpdateInvalidInput 을 반환한다.
	Verifier *Verifier
}

// manifestSizeCap 은 manifest.json 의 최대 허용 크기 (DoS 방어).
//
// 64KB 는 일반적인 manifest (수십 의존성) 를 충분히 수용하면서도 abuse 시
// 메모리 폭증을 막는다.
const manifestSizeCap = 64 * 1024

// signatureSizeCap 은 .sig 파일의 최대 허용 크기.
// Ed25519 서명은 정확히 64 bytes 이지만 트레일링 newline 등을 허용해 256 으로 여유.
const signatureSizeCap = 256

// FetchAndVerify 는 manifest 와 signature 를 HTTPS 로 다운로드하고 검증 + 파싱한다.
//
// 단계:
//  1. 두 URL 모두 https:// 강제
//  2. manifest 다운로드 (64KB cap)
//  3. signature 다운로드 (256B cap)
//  4. Ed25519 서명 검증 (manifest 본문 vs signature)
//  5. JSON 파싱 후 IsValid() 검증
//
// 반환:
//   - 정상: DependencyManifest, nil
//   - HTTPS 위반: ErrUpdateChannelInvalid
//   - 다운로드 실패 / 크기 초과: ErrUpdateDownloadFailed
//   - 서명 검증 실패: ErrUpdateSignatureInvalid
//   - JSON 파싱 / 구조 위반: ErrUpdateInvalidInput
func (f *ManifestFetcher) FetchAndVerify(ctx context.Context, manifestURL, signatureURL string) (DependencyManifest, error) {
	if f.Verifier == nil {
		return DependencyManifest{}, fmt.Errorf("%w: ManifestFetcher.Verifier is nil", ErrUpdateInvalidInput)
	}
	if err := requireHTTPS(manifestURL); err != nil {
		return DependencyManifest{}, fmt.Errorf("manifest URL: %w", err)
	}
	if err := requireHTTPS(signatureURL); err != nil {
		return DependencyManifest{}, fmt.Errorf("signature URL: %w", err)
	}

	client := f.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	manifestBytes, err := fetchSizeCappedBytes(ctx, client, manifestURL, manifestSizeCap)
	if err != nil {
		return DependencyManifest{}, fmt.Errorf("fetch manifest: %w", err)
	}
	sigBytes, err := fetchSizeCappedBytes(ctx, client, signatureURL, signatureSizeCap)
	if err != nil {
		return DependencyManifest{}, fmt.Errorf("fetch signature: %w", err)
	}

	// 일부 도구는 .sig 파일에 트레일링 newline 을 추가한다.
	// Ed25519 서명은 정확히 64 bytes 인데 random byte 가 0x0A/0x0D 일 수 있으므로 무조건 trim 하면 위양성 발생.
	// 따라서 길이가 64 보다 큰 경우에만 trailing newline 을 1-2 byte 까지 제거한다 (보수적).
	sigBytes = trimTrailingNewlinesIfTooLong(sigBytes, 64)

	if err := f.Verifier.VerifySignature(manifestBytes, sigBytes); err != nil {
		return DependencyManifest{}, fmt.Errorf("manifest signature: %w", err)
	}

	var dm DependencyManifest
	if err := json.Unmarshal(manifestBytes, &dm); err != nil {
		return DependencyManifest{}, fmt.Errorf("%w: parse manifest JSON: %v", ErrUpdateInvalidInput, err)
	}
	if !dm.IsValid() {
		return DependencyManifest{}, fmt.Errorf("%w: invalid manifest structure (version=%q binary=%q)",
			ErrUpdateInvalidInput, dm.Version, dm.Binary)
	}
	return dm, nil
}

// fetchSizeCappedBytes 는 작은 메타데이터 파일을 메모리로 읽어들이며 cap 초과 시 거부한다.
//
// HTTP error / network error / 크기 초과는 모두 ErrUpdateDownloadFailed 로 매핑된다.
func fetchSizeCappedBytes(ctx context.Context, client *http.Client, urlStr string, cap int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("%w: build request: %v", ErrUpdateDownloadFailed, err)
	}
	req.Header.Set("User-Agent", "xflow-updater/1.0")

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("%w: do request: %v", ErrUpdateDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("%w: HTTP %d", ErrUpdateDownloadFailed, resp.StatusCode)
	}

	// cap+1 까지 읽어서 cap 초과 여부 판별
	buf := make([]byte, cap+1)
	n, readErr := readUntilFull(resp.Body, buf)
	if int64(n) > cap {
		return nil, fmt.Errorf("%w: response exceeds %d bytes", ErrUpdateDownloadFailed, cap)
	}
	if readErr != nil && readErr.Error() != "EOF" {
		// io.EOF 가 아닌 다른 에러는 진짜 실패
		// (note: readUntilFull 은 EOF 시 nil 반환하므로 이 분기는 실 네트워크 에러용)
		return nil, fmt.Errorf("%w: read body: %v", ErrUpdateDownloadFailed, readErr)
	}
	return buf[:n], nil
}

// readUntilFull 은 buf 가 채워질 때까지 또는 EOF 까지 읽는다.
//
// 반환값:
//   - n: 실제 읽은 바이트 수
//   - err: EOF 인 경우 nil (정상), 그 외 에러
func readUntilFull(r interface {
	Read(p []byte) (int, error)
}, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			// io.EOF 도 성공 (정상 종료)
			if err.Error() == "EOF" {
				return total, nil
			}
			return total, err
		}
	}
	return total, nil
}

// trimTrailingNewlinesIfTooLong 은 b 의 길이가 expected 보다 큰 경우에만
// trailing \n / \r 을 제거한 슬라이스를 반환한다.
//
// Ed25519 서명은 정확히 64 bytes random data 이므로 마지막 byte 가 우연히 0x0A 또는 0x0D 일 수 있다.
// 이 경우 무조건 trim 하면 valid signature 의 길이를 63 bytes 로 만들어 위양성 (ErrUpdateInvalidInput)
// 을 발생시킨다. 따라서 입력 길이가 expected 를 이미 초과한 경우에만 trim 을 시도한다.
//
// 예시:
//   - 입력 64 bytes (정상 서명) → 트림 안 함
//   - 입력 65 bytes ("...\n") → 64 bytes 로 트림
//   - 입력 66 bytes ("...\r\n") → 64 bytes 로 트림
func trimTrailingNewlinesIfTooLong(b []byte, expected int) []byte {
	for len(b) > expected {
		last := b[len(b)-1]
		if last == '\n' || last == '\r' {
			b = b[:len(b)-1]
			continue
		}
		// 마지막 byte 가 newline 이 아니면 더 이상 trim 불가 (노이즈가 있는 .sig 는 그대로 검증 실패)
		break
	}
	return b
}

// CompatibilityChecker 는 환경 내 바이너리 버전을 manifest 의 제약과 비교한다.
//
// SPEC-UPDATE-002 v0.1.0 (M11):
//   - 환경에 설치된 바이너리만 검사 (미설치 바이너리는 제약을 건너뜀)
//   - 모든 제약을 만족해야 nil 반환
//   - 하나라도 위반 → ErrUpdateIncompatibleVersion
//
// 의존성 주입:
//   - GetVersion: 환경 바이너리의 현재 버전을 반환하는 함수.
//     not-installed 는 error 반환 (해당 제약 자동 skip).
type CompatibilityChecker struct {
	// GetVersion 은 환경 내 binary 의 현재 버전을 반환한다.
	//
	// 운영 환경 구현 예: process discovery 또는 설치된 바이너리의 --version 출력 파싱.
	// not-installed 시 non-nil error 반환 → Validate() 가 해당 제약을 skip.
	//
	// nil 이면 Validate() 가 즉시 ErrUpdateInvalidInput 반환.
	GetVersion func(binary string) (Version, error)
}

// Validate 는 manifest 의 모든 Compat 제약을 환경과 비교한다.
//
// 동작:
//   - Compat 이 nil/empty 면 즉시 nil (제약 없음)
//   - 각 binary 별로 GetVersion() 호출
//   - not-installed (error) → 해당 제약 skip
//   - installed → CheckConstraint() 검증
//   - 위반 시 ErrUpdateIncompatibleVersion (with context)
//
// 보안: GetVersion 에러 자체는 호환성 위반으로 분류하지 않는다 (false positive 회피).
// 환경에 바이너리가 없는 것이 정상 케이스 (예: agent 만 설치된 노드).
func (c *CompatibilityChecker) Validate(m DependencyManifest) error {
	if c.GetVersion == nil {
		return fmt.Errorf("%w: CompatibilityChecker.GetVersion is nil", ErrUpdateInvalidInput)
	}
	if len(m.Compat) == 0 {
		return nil
	}

	for binary, constraint := range m.Compat {
		currentVersion, err := c.GetVersion(binary)
		if err != nil {
			// 미설치 → 제약 skip (intentional: not all environments have all binaries).
			continue
		}
		if err := CheckConstraint(currentVersion, constraint); err != nil {
			return fmt.Errorf("%w: %s requires %s%s (current: %s)",
				ErrUpdateIncompatibleVersion, m.Binary, binary, constraint, currentVersion)
		}
	}
	return nil
}
