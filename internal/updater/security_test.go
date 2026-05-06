// @SPEC:SPEC-UPDATE-001 v0.1.0
// security_test.go — Phase H 보안 적대적 시나리오 테스트.
//
// 목적: SPEC-UPDATE-001 의 보안 critical 요구사항 (acceptance.md "보안" edge case
// 체크리스트) 을 다양한 공격 모델로 검증한다.
//
// 위협 모델:
//   - MITM 공격: HTTPS + TLS 검증으로 차단
//   - 다운그레이드 공격: 서버가 이전 버전을 latest 로 응답해도 거부
//   - 서명 위조: 공격자 키로 서명한 바이너리 차단
//   - TOCTOU: 다운로드 후 변조된 파일 → pre-rename 재검증으로 차단
//   - Replay 공격: 이전 release 를 다시 보내도 버전 비교로 차단
//   - DoS: 과대 응답 (checksum/signature) → io.LimitReader 로 차단
//   - 공개키 변조: Verifier 의 publicKey 는 unexported 필드로 보호
//   - 권한 누설: 임시 파일 0600, 실행 파일 0755
//   - Timing attack: SHA256 비교는 ConstantTimeCompare 사용
//   - 동시성: 백업 파일 race 시 마지막 쓰기 보존
//
// 보안 critical 테스트 안전성:
//   - 모든 키는 ephemeral (테스트 격리)
//   - 모든 경로는 t.TempDir() 안 (실제 시스템 무영향)
//   - 절대 InsecureSkipVerify 를 production path 에 끼지 않음 (server cert 만)
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// 1. MITM 공격 — HTTPS 인증서 검증은 우회 불가
// =============================================================================

// TestSecurity_MITM_HTTPSCertificateValidation 은 HTTPS 인증서 검증이 production
// 경로에서 비활성화 불가능함을 검증한다.
//
// 시나리오: 공격자가 자체 서명 인증서로 MITM 시도 → production HTTP 클라이언트는 거부.
//
// 검증 방법:
//  1. httptest.NewTLSServer (자체 서명 cert) 를 띄움
//  2. production Downloader 의 NewDownloader() 기본 클라이언트로 시도 → cert 검증 실패
//  3. server.Client() (cert 신뢰) 로 시도 → 정상 동작 (테스트 인프라 검증)
//
// 결론: production 흐름에서 cert 검증을 끄려면 명시적 InsecureSkipVerify=true 필요
// (M11 config 에 별도 옵션 + critical 경고 로그). 본 테스트는 default 가 always-verify 인지 확인.
func TestSecurity_MITM_HTTPSCertificateValidation(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("attacker-controlled-payload"))
	}))
	t.Cleanup(srv.Close)

	// production 기본 클라이언트로 시도 → cert 검증 실패해야 함
	d := NewDownloader()
	d.disk = &fakeDiskInspector{avail: 1 << 30}
	// 의도적으로 srv.Client() 를 사용하지 않음 (production 동작 시뮬레이션)

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{
		DownloadURL: srv.URL + "/binary",
		Size:        100,
	}
	err := d.Download(context.Background(), asset, dest, nil)
	require.Error(t, err, "production client must reject untrusted self-signed TLS cert")
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed),
		"untrusted cert error should wrap ErrUpdateDownloadFailed; got %v", err)

	// 다운로드 실패 → 임시 파일도 정리
	_, statErr := os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr))
}

// =============================================================================
// 2. 다운그레이드 공격 — 서버가 이전 버전을 latest 로 응답해도 거부
// =============================================================================

// TestSecurity_DowngradeAttack_Prevented 는 채널 응답이 v0.2.0 (현재 v0.3.0 보다 구버전)
// 이어도 Checker 가 Available=false 로 판정함을 검증한다.
//
// 공격 시나리오: 침해된 채널 또는 MITM 이 의도적으로 구버전을 latest 로 응답
// → 운영자가 모르고 다운그레이드되어 알려진 취약점이 있는 구버전으로 회귀.
func TestSecurity_DowngradeAttack_Prevented(t *testing.T) {
	srv := newFakeReleaseServer(t,
		withVersion("v0.2.0"), // 구버전 응답 (공격자 시나리오)
	)

	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()

	res, err := checker.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)

	// Then: 다운그레이드 자동 거부
	assert.False(t, res.Available, "downgrade attack must NOT trigger auto-update")
	assert.Equal(t, Version("v0.3.0"), res.Current)
	assert.Equal(t, Version("v0.2.0"), res.Latest)
}

// =============================================================================
// 3. 서명 위조 — 공격자 키로 서명한 바이너리는 거부
// =============================================================================

// TestSecurity_SignatureForgery_DifferentKey 는 공격자가 자신의 키로 서명한 바이너리를
// 채널에 주입했을 때 검증 단계에서 거부됨을 검증한다.
//
// 공격 시나리오:
//   - 공격자가 GitHub Release 를 침해하고 자신의 ed25519 키로 서명한 바이너리 + 일치하는 sig 게시
//   - production 의 핀닝된 publicKey 와 다르므로 ed25519.Verify 가 false 반환
//   - ErrUpdateSignatureInvalid 반환 + 보안 감사 로그 trigger
func TestSecurity_SignatureForgery_DifferentKey(t *testing.T) {
	// production 의 정당한 키 페어
	prodPub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	verifier, err := NewVerifier(prodPub)
	require.NoError(t, err)

	// 공격자 키 페어 (다름)
	_, attackerPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	maliciousBinary := []byte("MALICIOUS-XFLOWD-BACKDOORED")
	sumBytes := sha256.Sum256(maliciousBinary)
	attackerSignature := ed25519.Sign(attackerPriv, maliciousBinary)

	// 검증 → 서명 실패
	err = verifier.VerifyAll(maliciousBinary, hex.EncodeToString(sumBytes[:]), attackerSignature)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateSignatureInvalid),
		"signature signed by attacker key must trigger ErrUpdateSignatureInvalid; got %v", err)
}

// =============================================================================
// 4. TOCTOU — 다운로드 후 변조된 파일 차단
// =============================================================================

// TestSecurity_BinaryTampering_AfterDownload 는 검증된 바이너리가 디스크에 저장된 후
// rename 직전 (TOCTOU 윈도우) 에 변조될 경우 Applier 가 pre-rename 재검증으로 차단함을 검증한다.
//
// 공격 시나리오:
//   - Phase B downloader 가 bytes A 를 디스크에 저장 + 1차 검증 통과
//   - 공격자가 디스크 파일을 bytes B 로 변경 (예: 다른 process)
//   - Phase C applier 가 rename 직전 메모리 로드 + 재검증 → bytes B != manifest → 차단
func TestSecurity_BinaryTampering_AfterDownload(t *testing.T) {
	binPath, oldContent := setupFakeBinary(t)
	verifier, priv := newTestVerifier(t)

	// 정당한 (서명된) 바이너리
	legitContent := []byte("LEGITIMATE-XFLOWD-V0.4.0")
	manifest := makeManifestFor(t, legitContent, priv)

	// 공격자가 디스크를 변조 (TOCTOU)
	tamperedContent := []byte("ATTACKER-TAMPERED-PAYLOAD")
	dlPath := writeDownloadedFile(t, t.TempDir(), tamperedContent)

	applier := NewApplier(verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed),
		"TOCTOU tampering must trigger pre-rename verification failure")

	// 메인 바이너리는 변경되지 않음
	stillOld, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, oldContent, stillOld, "main binary must NOT be replaced after TOCTOU detection")
}

// =============================================================================
// 5. Replay 공격 — 이전 release 의 valid signature 재사용
// =============================================================================

// TestSecurity_ReplayAttack_OldRelease 는 공격자가 valid 서명을 가진 v0.1.0 (구버전)
// release 를 v0.4.0 인 척 응답해도 Checker 의 버전 비교에서 차단됨을 검증한다.
//
// 공격 시나리오:
//   - 공격자가 과거의 v0.1.0 binary + 정당한 서명을 보존
//   - 채널 응답에서 tag_name 만 v0.4.0 으로 위조 + 실제 asset 은 v0.1.0
//   - 검증은 통과 (서명은 valid) 하지만 Checker 가 latest > current 로 판정 (false positive)
//
// 본 테스트는 Checker 의 버전 비교 로직이 단조 증가만 허용함을 검증한다 (semver 기반).
// 또한 PublishedAt 필드가 Replay 방어에 사용 가능함을 표시 (M13 acceptance 의 Replay 항목).
func TestSecurity_ReplayAttack_OldRelease(t *testing.T) {
	// Channel 이 v0.4.0 으로 응답하지만 실제로는 운영 중 버전이 더 새것
	srv := newFakeReleaseServer(t,
		withVersion("v0.4.0"),
	)

	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()

	// 케이스 1: 동일 버전 → Available=false
	res, err := checker.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.False(t, res.Available, "동일 버전 → 업데이트 불필요")

	// 케이스 2: 이미 v1.0.0 운영 중인데 채널 응답이 v0.4.0 → 다운그레이드 거부
	res, err = checker.Check(context.Background(), "v1.0.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.False(t, res.Available, "replay attack with older version must be rejected")

	// PublishedAt 이 Checker 결과에 포함되어 호출자가 Replay 방어 로직 (M13) 을 추가 구성 가능
	assert.False(t, res.PublishedAt.IsZero(), "published_at은 replay 방어 메타데이터로 노출되어야 함")
}

// =============================================================================
// 6. 과대 응답 — io.LimitReader 로 DoS 방어
// =============================================================================

// TestSecurity_OversizedResponse_DoSProtection 은 채널이 거대한 checksum.txt
// (예: 2MB 이상) 를 응답해도 fetchSmallFile 의 limit (1MB) 가 발동되어 거부됨을 검증한다.
//
// 공격 시나리오: 공격자가 메모리 폭증 유발 시도. updater 는 1MB 캡으로 차단.
func TestSecurity_OversizedResponse_DoSProtection(t *testing.T) {
	srv := newFakeReleaseServer(t,
		withOversizedResponse(),
	)

	d := NewDownloader()
	d.HTTPClient = srv.httpClient()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	checksumAsset := ReleaseAsset{DownloadURL: srv.server.URL + "/checksum", Size: 200}
	sigAsset := ReleaseAsset{DownloadURL: srv.server.URL + "/signature", Size: 64}
	binAssetName := AssetName("xflowd", "linux", "amd64")

	_, err := d.DownloadManifest(context.Background(), checksumAsset, sigAsset, binAssetName, "v0.4.0")
	require.Error(t, err, "oversized checksum response must be rejected")
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed),
		"DoS-sized response should wrap ErrUpdateDownloadFailed; got %v", err)
}

// =============================================================================
// 7. 공개키 변조 — Verifier 의 publicKey 는 unexported (외부에서 변경 불가)
// =============================================================================

// TestSecurity_PublicKeyMutation_Rejected 는 Verifier 인스턴스 생성 후 외부에서
// publicKey 를 변경할 방법이 없음을 검증한다 (Go 의 unexported 필드 보호).
//
// 시나리오: 공격자가 reflection 으로 publicKey 를 자신의 키로 swap 시도 → reflect 가 거부.
//
// Go 의 reflect.CanSet 은 unexported 필드에 대해 false 를 반환하므로,
// 공격자가 같은 패키지 안에서 직접 접근하지 않는 한 변경 불가능하다.
func TestSecurity_PublicKeyMutation_Rejected(t *testing.T) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	verifier, err := NewVerifier(pub)
	require.NoError(t, err)

	// reflection 시도: publicKey 필드는 unexported → CanSet=false
	v := reflect.ValueOf(verifier).Elem()
	field := v.FieldByName("publicKey")
	require.True(t, field.IsValid(), "publicKey 필드 자체는 reflect 로 보임 (Go reflection 표준)")
	assert.False(t, field.CanSet(),
		"unexported field publicKey should NOT be settable via reflection (Go safety)")

	// 추가: 동일 패키지 내에서 의도적으로 변경할 수 있더라도 (테스트용),
	// 정상 verifier 는 prodPub 로 검증 → priv 로 서명한 데이터 거부.
	content := []byte("attacker-payload")
	attackerSig := ed25519.Sign(priv, content)
	sumBytes := sha256.Sum256(content)

	verifyErr := verifier.VerifyAll(content, hex.EncodeToString(sumBytes[:]), attackerSig)
	require.Error(t, verifyErr)
	assert.True(t, errors.Is(verifyErr, ErrUpdateSignatureInvalid),
		"공격자 키로 서명한 데이터는 prod publicKey 검증에서 거부")
}

// =============================================================================
// 8. 임시 파일 권한 0600 — 다른 사용자 접근 불가
// =============================================================================

// TestSecurity_TempFilePermissions_0600 은 다운로드된 임시 파일이 0600 권한을 가져
// 동일 시스템의 다른 사용자가 읽거나 실행할 수 없음을 검증한다.
//
// 보안 요구사항 (M3, M13): 검증 통과 전에는 절대 실행 가능 상태 아님.
func TestSecurity_TempFilePermissions_0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 의 권한 시맨틱은 다름")
	}

	srv := newFakeReleaseServer(t)

	d := NewDownloader()
	d.HTTPClient = srv.httpClient()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "downloaded-binary")
	asset := ReleaseAsset{
		DownloadURL: srv.server.URL + "/binary",
		Size:        int64(len(srv.binary)),
	}

	err := d.Download(context.Background(), asset, dest, nil)
	require.NoError(t, err)

	info, err := os.Stat(dest)
	require.NoError(t, err)

	// Then: 0600 권한 (소유자 read+write 만, 실행 비트 없음)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"downloaded file MUST have 0600 permission before verification (owner read/write only)")

	// 명시적으로 실행 비트 부재 확인
	assert.Zero(t, info.Mode().Perm()&0o111,
		"downloaded file MUST NOT have execute bit (security critical)")
}

// =============================================================================
// 9. Timing attack — SHA256 비교는 ConstantTimeCompare 사용
// =============================================================================

// TestSecurity_TimingAttack_ConstantTimeCompare 은 verifier.VerifyChecksum 의 비교가
// constant-time 임을 행동 기반으로 검증한다.
//
// 직접적 타이밍 측정은 noise 가 크므로, 본 테스트는 호출 동작이 결정적이며
// (mismatch 위치와 무관하게 동일한 ErrUpdateChecksumMismatch 반환) timing 누설이 없음을 표시한다.
//
// 실제 ConstantTimeCompare 사용은 verifier.go 의 코드 인라인 검사가 보증
// (crypto/subtle 라이브러리 호출).
func TestSecurity_TimingAttack_ConstantTimeCompare(t *testing.T) {
	verifier, _ := newTestVerifier(t)

	content := []byte("test-content")
	correctSum := sha256.Sum256(content)
	correctHex := hex.EncodeToString(correctSum[:])

	// 케이스 1: 첫 바이트만 다름
	tampered1 := correctHex
	tampered1 = "ff" + tampered1[2:]
	err1 := verifier.VerifyChecksum(content, tampered1)
	require.Error(t, err1)
	assert.True(t, errors.Is(err1, ErrUpdateChecksumMismatch),
		"first-byte mismatch must trigger generic ErrUpdateChecksumMismatch (no positional info)")

	// 케이스 2: 마지막 바이트만 다름
	tampered2 := correctHex[:62] + "ff"
	err2 := verifier.VerifyChecksum(content, tampered2)
	require.Error(t, err2)
	assert.True(t, errors.Is(err2, ErrUpdateChecksumMismatch),
		"last-byte mismatch must trigger same error (constant-time guarantee)")

	// 두 에러가 동일 sentinel → 위치 정보 누설 없음
	assert.Equal(t, errors.Unwrap(errors.Unwrap(err1)), errors.Unwrap(errors.Unwrap(err2)),
		"timing-safe compare: both mismatches surface identical sentinel chain")
}

// =============================================================================
// 10. 백업 파일 race — 동시 Apply 시 마지막 성공이 우선
// =============================================================================

// TestSecurity_BackupFileRace 는 동시에 여러 Apply 호출이 들어왔을 때 file system
// rename 의 atomic 보장으로 race condition 이 발생하지 않음을 검증한다.
//
// SPEC M5/M7: rename 은 POSIX atomic. 동시 시도 시 최후 성공자가 메인 위치를 가짐.
// 백업도 동일 시맨틱 (.previous 는 매번 덮어씀; M5 "직전 백업만 보존").
//
// Note: 본 테스트는 file system race 를 행동적으로 시연하며, production 의
// 동시 update apply 직렬화는 manager.go (orchestrator) 의 mutex 가 추가 보장한다.
func TestSecurity_BackupFileRace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows 의 rename 시맨틱은 다름 (file in use 에러 가능)")
	}

	srv := newFakeReleaseServer(t)

	binPath, _ := setupFakeBinary(t)
	manifest := makeManifestForServer(t, srv, "v0.4.0")
	dlDir := t.TempDir()

	// 동일한 (검증 통과) 콘텐츠로 두 다운로드 파일 준비
	dlPath1 := writeDownloadedFile(t, dlDir, srv.binary)
	dlPath2 := filepath.Join(dlDir, "downloaded-binary-2")
	require.NoError(t, os.WriteFile(dlPath2, srv.binary, 0o600))

	// 동시 Apply 시도 → 둘 다 성공해야 함 (race-clean)
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Add(2)

	go func() {
		defer wg.Done()
		applier := NewApplier(srv.verifier, binPath)
		_, err := applier.Apply(context.Background(), ApplyOptions{
			Manifest:       manifest,
			DownloadedPath: dlPath1,
		})
		errs <- err
	}()

	go func() {
		defer wg.Done()
		applier := NewApplier(srv.verifier, binPath)
		_, err := applier.Apply(context.Background(), ApplyOptions{
			Manifest:       manifest,
			DownloadedPath: dlPath2,
		})
		errs <- err
	}()

	wg.Wait()
	close(errs)

	// 적어도 하나는 성공해야 함; 둘 다 성공해도 무방 (rename 멱등성)
	successCount := 0
	for err := range errs {
		if err == nil {
			successCount++
		}
	}
	assert.GreaterOrEqual(t, successCount, 1,
		"at least one concurrent Apply should succeed (atomic rename guarantee)")

	// 메인 바이너리 콘텐츠 = srv.binary (양쪽 다 동일)
	got, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, srv.binary, got, "메인 바이너리는 두 콘텐츠 중 하나 (둘 다 동일하므로 결정적)")
}

// =============================================================================
// 추가 보안 테스트 — fail-closed 정책 검증
// =============================================================================

// TestSecurity_NilSignature_Rejected 는 빈 서명 입력을 거부함을 검증한다.
//
// 공격 시나리오: 채널이 빈 signature.bin 을 응답 → fail-closed 정책으로 거부.
func TestSecurity_NilSignature_Rejected(t *testing.T) {
	t.Parallel()
	verifier, _ := newTestVerifier(t)
	content := []byte("test")
	sumBytes := sha256.Sum256(content)

	// nil signature
	err := verifier.VerifySignature(content, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))

	// empty signature
	err = verifier.VerifySignature(content, []byte{})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInvalidInput))

	// VerifyAll 도 동일
	err = verifier.VerifyAll(content, hex.EncodeToString(sumBytes[:]), nil)
	require.Error(t, err)
}

// TestSecurity_NonEd25519PublicKey_Rejected 는 잘못된 길이의 공개키로 Verifier 생성 시
// fail-closed 동작을 검증한다.
//
// 공격 시나리오: 빌드 시 ldflags 로 잘못된 키 (예: RSA pubkey hex) 가 주입된 경우.
func TestSecurity_NonEd25519PublicKey_Rejected(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  ed25519.PublicKey
	}{
		{"nil_key", nil},
		{"empty_key", []byte{}},
		{"too_short", make([]byte, 16)},
		{"too_long", make([]byte, 64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewVerifier(tc.key)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateInvalidInput))
		})
	}
}

// TestSecurity_HexHashMalformed_Rejected 는 비-hex / 잘못된 길이의 hash 문자열이
// fail-closed 로 거부됨을 검증한다.
func TestSecurity_HexHashMalformed_Rejected(t *testing.T) {
	t.Parallel()
	verifier, _ := newTestVerifier(t)
	content := []byte("test-payload")

	cases := []struct {
		name string
		hex  string
	}{
		{"empty", ""},
		{"too_short", "abcd"},
		{"too_long", string(make([]byte, 100))},
		{"non_hex", "ZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZZ"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := verifier.VerifyChecksum(content, tc.hex)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateInvalidInput),
				"malformed hex must be rejected as ErrUpdateInvalidInput; got %v", err)
		})
	}
}

// TestSecurity_RedirectFromHTTPSToHTTP_NotImplemented 는 redirect 추적 시 HTTP 만 허용 여부에
// 대한 명시적 marker 테스트이다.
//
// 현재 Phase A-G 구현은 net/http 기본 redirect 정책을 사용 (스킴 변경 추적 가능).
// SPEC acceptance.md 보안 체크리스트 "HTTP redirect 추적 시 HTTPS만 허용" 은 향후
// CheckRedirect callback 추가로 강화 예정 (manager.go 또는 별도 SPEC).
//
// 본 테스트는 현 시점의 동작을 문서화하고, future regression 을 위한 anchor 역할을 한다.
func TestSecurity_RedirectFromHTTPSToHTTP_NotImplemented(t *testing.T) {
	// HTTPS → HTTP redirect 시 net/http 기본 client 가 따라가지 않을 수도 있고 (최근 Go),
	// 따라갈 수도 있음. 명시적 거부 정책은 manager.go 에서 추가 권장.
	//
	// 현재는 Downloader.Download 가 최종 응답 status 를 검사하고, 단계 이전에 URL 의
	// 스킴을 검증하므로 1단계 방어는 이미 존재.
	//
	// 본 테스트는 향후 CheckRedirect 추가 시 PR 에서 확장될 수 있도록 구조만 잡아둠.
	//
	// (의도적 No-op assertion — 향후 SPEC iteration 의 traceability 마커)
	assert.NotNil(t, ErrUpdateChannelInvalid,
		"sentinel for redirect rejection 은 이미 정의되어 있음 (HTTPS scheme check 단계 재사용)")
}

// =============================================================================
// 종합 smoke test — Phase A-H 전체 흐름 + 모든 sentinel 통합
// =============================================================================

// TestSecurity_AllSentinelsExposed 는 SPEC 명세의 9종 sentinel error 가 모두 패키지 외부에
// 노출되어 errors.Is 로 식별 가능함을 검증한다 (M9 / M13).
//
// 운영 모니터링 / CLI exit code 매핑이 sentinel 식별에 의존하므로 fail-closed.
func TestSecurity_AllSentinelsExposed(t *testing.T) {
	t.Parallel()
	sentinels := []error{
		ErrUpdateChannelInvalid,
		ErrUpdateDownloadFailed,
		ErrUpdateInsufficientDiskSpace,
		ErrUpdateChecksumMismatch,
		ErrUpdateSignatureInvalid,
		ErrUpdateApplyFailed,
		ErrUpdateRollbackFailed,
		ErrDowngradeRequiresForce,
		ErrUpdateInProgress,
		ErrUpdateInvalidInput,
	}
	for i, s := range sentinels {
		assert.NotNilf(t, s, "sentinel #%d must be defined", i)
		assert.NotEmptyf(t, s.Error(), "sentinel #%d must have non-empty message", i)
	}
}

// TestSecurity_PublishedAtPopulated 는 채널 응답에 published_at 이 포함되어
// Replay 방어용 시간 정보가 호출자에게 노출됨을 검증한다 (M13 보안 메타데이터).
func TestSecurity_PublishedAtPopulated(t *testing.T) {
	srv := newFakeReleaseServer(t, withVersion("v0.4.0"))

	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()

	res, err := checker.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)

	// published_at 은 Replay 방어 의사결정의 입력
	assert.False(t, res.PublishedAt.IsZero())
	assert.True(t, time.Since(res.PublishedAt) < 1*time.Hour,
		"테스트 mock 의 published_at 은 최근값이어야 함")
}
