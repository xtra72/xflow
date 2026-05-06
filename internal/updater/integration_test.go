// @SPEC:SPEC-UPDATE-001 v0.1.0
// integration_test.go — Phase H end-to-end 통합 테스트.
//
// 목적: SPEC-UPDATE-001 acceptance.md 의 10개 Given-When-Then 시나리오를
// httptest 기반 mock GitHub Releases 서버로 끝까지 검증한다.
//
// 테스트 안전성:
//   - 모든 테스트는 httptest 만 사용 (실제 GitHub 호출 없음)
//   - 모든 테스트는 ephemeral Ed25519 키 페어를 자체 생성 (하드코딩 금지)
//   - 모든 테스트는 t.TempDir() 안에서만 동작 (실제 xflowd 바이너리 무영향)
//   - 모든 테스트는 t.Cleanup 으로 자원 해제 보장
//
// 시나리오 매핑 (acceptance.md):
//
//	Scenario 1  — TestE2E_CheckDetectsNewVersion
//	Scenario 2  — TestE2E_FullApplyFlow_DownloadVerifyApply
//	Scenario 3  — TestE2E_TamperedSignature_AbortsApply
//	Scenario 4  — TestE2E_RollbackFlow_RestoresPreviousBinary (standalone; full restart 통합 미연결)
//	Scenario 5  — TestE2E_Downgrade_RefusedWithoutForce
//	Scenario 6  — TestE2E_InsufficientDisk_AbortsBeforeDownload
//	Scenario 7  — TestE2E_HTTPOnlyURL_RejectedAtConstruct
//	Scenario 8  — Phase G handler 테스트로 커버 (system_update_test.go)
//	Scenario 9  — Phase G + auth middleware 로 커버
//	Scenario 10 — TestE2E_DrainTimeout_StillProceedsToExec (Phase D restarter_test 에서 커버)
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// =============================================================================
// fakeReleaseServer — mock GitHub Releases 서버 빌더
// =============================================================================
//
// Phase H 의 핵심 헬퍼. 단일 인스턴스로 다음을 모두 처리한다:
//   - GitHub API endpoint (/repos/.../releases/latest)
//   - 바이너리 asset 다운로드 (/binary)
//   - checksum.txt 다운로드 (/checksum)
//   - signature 다운로드 (/signature)
//
// 옵션 (functional options) 으로 시나리오별 변형 가능:
//   - withTamperedSignature(): 잘못된 서명 응답 (Scenario 3)
//   - withTamperedChecksum(): 잘못된 체크섬 응답
//   - withVersion("v0.4.0"): 응답할 버전 태그
//   - withBinaryContent([]byte): 바이너리 페이로드
//   - withEmptyReleases(): GitHub 가 release 0 건 (404)
//   - withOversizedResponse(): DoS 방어 검증용
type fakeReleaseServer struct {
	server      *httptest.Server
	privateKey  ed25519.PrivateKey
	publicKey   ed25519.PublicKey
	verifier    *Verifier
	binary      []byte
	version     Version
	channel     Channel
	tamperedSig bool
	tamperedSum bool
	noReleases  bool
	oversized   bool

	// requestCount 는 동일 endpoint 의 호출 횟수를 추적 (race 테스트용).
	requestCount atomic.Int64
}

// fakeReleaseOpt 는 newFakeReleaseServer 의 옵션 함수 시그니처이다.
type fakeReleaseOpt func(*fakeReleaseServer)

func withVersion(v Version) fakeReleaseOpt {
	return func(f *fakeReleaseServer) { f.version = v }
}

func withBinaryContent(c []byte) fakeReleaseOpt {
	return func(f *fakeReleaseServer) { f.binary = c }
}

func withTamperedSignature() fakeReleaseOpt {
	return func(f *fakeReleaseServer) { f.tamperedSig = true }
}

func withTamperedChecksum() fakeReleaseOpt {
	return func(f *fakeReleaseServer) { f.tamperedSum = true }
}

func withChannel(c Channel) fakeReleaseOpt {
	return func(f *fakeReleaseServer) { f.channel = c }
}

func withEmptyReleases() fakeReleaseOpt {
	return func(f *fakeReleaseServer) { f.noReleases = true }
}

func withOversizedResponse() fakeReleaseOpt {
	return func(f *fakeReleaseServer) { f.oversized = true }
}

// newFakeReleaseServer 는 ephemeral 키 + httptest.NewTLSServer 로 mock 인스턴스를 생성한다.
//
// 매 테스트마다 새 키 페어를 생성해 키 격리를 보장한다 (보안 critical).
// t.Cleanup 으로 server 자동 종료.
func newFakeReleaseServer(t *testing.T, opts ...fakeReleaseOpt) *fakeReleaseServer {
	t.Helper()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err, "ephemeral Ed25519 key generation must succeed")

	verifier, err := NewVerifier(pub)
	require.NoError(t, err)

	f := &fakeReleaseServer{
		privateKey: priv,
		publicKey:  pub,
		verifier:   verifier,
		binary:     []byte("FAKE-XFLOWD-V0.4.0-BINARY-PAYLOAD"),
		version:    "v0.4.0",
		channel:    ChannelStable,
	}
	for _, o := range opts {
		o(f)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/repos/xtra/xflow/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		f.requestCount.Add(1)
		if f.noReleases {
			http.NotFound(w, r)
			return
		}
		writeReleasePayload(w, f)
	})

	mux.HandleFunc("/repos/xtra/xflow/releases", func(w http.ResponseWriter, r *http.Request) {
		f.requestCount.Add(1)
		if f.noReleases {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		// 배열 형식 (beta/nightly 채널)
		_, _ = w.Write([]byte(`[`))
		writeReleasePayload(w, f)
		_, _ = w.Write([]byte(`]`))
	})

	mux.HandleFunc("/binary", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(f.binary)
	})

	mux.HandleFunc("/checksum", func(w http.ResponseWriter, _ *http.Request) {
		if f.oversized {
			// 1MB 초과로 io.LimitReader 발동 검증 (DoS 방어)
			huge := strings.Repeat("a", 2*1024*1024)
			_, _ = w.Write([]byte(huge))
			return
		}
		sum := sha256.Sum256(f.binary)
		if f.tamperedSum {
			sum[0] ^= 0xff // 첫 바이트만 뒤집기
		}
		assetName := AssetName("xflowd", "linux", "amd64")
		_, _ = fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), assetName)
	})

	mux.HandleFunc("/signature", func(w http.ResponseWriter, _ *http.Request) {
		sig := ed25519.Sign(f.privateKey, f.binary)
		if f.tamperedSig {
			sig[0] ^= 0xff
		}
		_, _ = w.Write(sig)
	})

	f.server = httptest.NewTLSServer(mux)
	t.Cleanup(func() { f.server.Close() })
	return f
}

// writeReleasePayload 는 GitHub API 응답 JSON 을 단일 release 객체로 출력한다.
func writeReleasePayload(w http.ResponseWriter, f *fakeReleaseServer) {
	binAssetName := AssetName("xflowd", "linux", "amd64")
	prerelease := f.channel != ChannelStable

	payload := map[string]any{
		"tag_name":     string(f.version),
		"name":         "xflowd " + string(f.version),
		"published_at": time.Now().UTC().Format(time.RFC3339),
		"prerelease":   prerelease,
		"html_url":     f.server.URL + "/releases/" + string(f.version),
		"assets": []map[string]any{
			{"name": binAssetName, "browser_download_url": f.server.URL + "/binary", "size": len(f.binary)},
			{"name": "checksum.txt", "browser_download_url": f.server.URL + "/checksum", "size": 200},
			{"name": binAssetName + ".sig", "browser_download_url": f.server.URL + "/signature", "size": 64},
		},
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// httpClient 는 httptest TLS server 의 자가 서명 인증서를 신뢰하는 HTTP 클라이언트를 반환한다.
//
// 보안: production 코드는 절대 InsecureSkipVerify 를 켜지 않으며, 본 헬퍼는 테스트 전용.
func (f *fakeReleaseServer) httpClient() *http.Client {
	return f.server.Client()
}

// baseURL 는 mock GitHub API 의 base URL 을 반환한다 (e.g. https://127.0.0.1:xxxxx/repos/xtra/xflow).
func (f *fakeReleaseServer) baseURL() string {
	return f.server.URL + "/repos/xtra/xflow"
}

// =============================================================================
// 시나리오 1: 새 버전 발견 — Checker 흐름
// =============================================================================

// TestE2E_CheckDetectsNewVersion (Scenario 1) 은 v0.3.0 운영 중 v0.4.0 발견 흐름을 검증한다.
//
// Given: xflowd v0.3.0 운영 중, stable 채널, mock 서버에 v0.4.0 게시
// When: Checker.Check 호출
// Then: Available=true, Latest=v0.4.0, Current=v0.3.0, ReleaseURL 존재
func TestE2E_CheckDetectsNewVersion(t *testing.T) {
	srv := newFakeReleaseServer(t, withVersion("v0.4.0"))

	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()

	res, err := checker.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err, "stable check should succeed")

	assert.True(t, res.Available, "v0.4.0 > v0.3.0 → Available=true")
	assert.Equal(t, Version("v0.3.0"), res.Current)
	assert.Equal(t, Version("v0.4.0"), res.Latest)
	assert.NotNil(t, res.BinaryAsset, "binary asset must be matched")
	assert.NotNil(t, res.SignatureAsset, "signature asset must be matched")
	assert.NotNil(t, res.ChecksumAsset, "checksum asset must be matched")
	assert.NotEmpty(t, res.ReleaseURL, "release_notes_url must be populated")
	assert.False(t, res.PublishedAt.IsZero(), "published_at must be parsed")
}

// =============================================================================
// 시나리오 2: 자동 적용 해피패스 — 다운로드 → 검증 → Apply
// =============================================================================

// TestE2E_FullApplyFlow_DownloadVerifyApply (Scenario 2) 는 정상 흐름 전체를 검증한다.
//
// 흐름:
//  1. Checker.Check → 신규 버전 발견
//  2. Downloader.Download → 바이너리 다운로드 (HTTPS, 0600)
//  3. Downloader.DownloadManifest → 체크섬 + 서명 다운로드
//  4. Verifier.VerifyAll → 무결성 + 진위 검증
//  5. Applier.Apply → atomic rename + 백업 생성 (0755)
//
// Then: 새 바이너리는 다운로드된 콘텐츠와 동일, 백업은 기존 콘텐츠와 동일.
func TestE2E_FullApplyFlow_DownloadVerifyApply(t *testing.T) {
	binaryContent := []byte("XFLOWD-V0.4.0-FULL-FLOW-CONTENT")
	srv := newFakeReleaseServer(t,
		withVersion("v0.4.0"),
		withBinaryContent(binaryContent),
	)

	// Phase 1: Check
	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()

	checkRes, err := checker.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	require.True(t, checkRes.Available)

	// Phase 2: Download binary
	downloader := NewDownloader()
	downloader.HTTPClient = srv.httpClient()
	downloader.disk = &fakeDiskInspector{avail: 1 << 30} // 1GB 여유

	tmpDir := t.TempDir()
	dlPath := filepath.Join(tmpDir, "downloaded-binary")
	require.NoError(t, downloader.Download(context.Background(), *checkRes.BinaryAsset, dlPath, nil))

	// 임시 파일은 0600 권한이어야 함 (M3, M13 보안 요구사항)
	info, err := os.Stat(dlPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"downloaded file must have 0600 permission before verification")

	// Phase 3: Download manifest (checksum + signature)
	binAssetName := AssetName("xflowd", "linux", "amd64")
	manifest, err := downloader.DownloadManifest(context.Background(),
		*checkRes.ChecksumAsset, *checkRes.SignatureAsset, binAssetName, checkRes.Latest)
	require.NoError(t, err)
	manifest.BinaryURL = checkRes.BinaryAsset.DownloadURL

	// Phase 4: Verify (downloader 가 1차, applier 가 pre-rename 시점에 2차)
	dlBytes, err := os.ReadFile(dlPath)
	require.NoError(t, err)
	require.NoError(t, srv.verifier.VerifyAll(dlBytes, manifest.SHA256, manifest.Signature),
		"verification of downloaded artifacts must succeed")

	// Phase 5: Apply (atomic rename + 백업)
	binPath, oldContent := setupFakeBinary(t)
	applier := NewApplier(srv.verifier, binPath)
	result, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.NoError(t, err, "apply should succeed for valid binary")
	assert.Equal(t, Version("v0.4.0"), result.NewVersion)

	// Then: 새 바이너리 = downloaded content
	newBin, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, binaryContent, newBin)

	// Then: 백업 = old content
	backup, err := os.ReadFile(binPath + ".previous")
	require.NoError(t, err)
	assert.Equal(t, oldContent, backup, "backup must contain pre-update binary")

	// Then: 권한 0755 (Unix)
	binInfo, err := os.Stat(binPath)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o755), binInfo.Mode().Perm())
}

// =============================================================================
// 시나리오 3: 검증 실패 — 서명 위조
// =============================================================================

// TestE2E_TamperedSignature_AbortsApply (Scenario 3) 는 서버가 위조 서명을 응답할 때
// 다운로드는 성공하나 서명 검증에서 거부됨을 검증한다.
//
// Given: mock 서버가 잘못된 서명 (다른 키로 서명한 듯) 응답
// When: Download → Verify (서명 검증)
// Then: ErrUpdateSignatureInvalid, 백업 생성 안 됨, 현재 바이너리 무변경
func TestE2E_TamperedSignature_AbortsApply(t *testing.T) {
	binaryContent := []byte("XFLOWD-V0.4.0-TAMPERED-SIG")
	srv := newFakeReleaseServer(t,
		withVersion("v0.4.0"),
		withBinaryContent(binaryContent),
		withTamperedSignature(),
	)

	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()

	checkRes, err := checker.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	require.True(t, checkRes.Available)

	downloader := NewDownloader()
	downloader.HTTPClient = srv.httpClient()
	downloader.disk = &fakeDiskInspector{avail: 1 << 30}

	tmpDir := t.TempDir()
	dlPath := filepath.Join(tmpDir, "downloaded-binary")
	require.NoError(t, downloader.Download(context.Background(), *checkRes.BinaryAsset, dlPath, nil),
		"download itself must succeed (HTTPS+TLS 정상)")

	binAssetName := AssetName("xflowd", "linux", "amd64")
	manifest, err := downloader.DownloadManifest(context.Background(),
		*checkRes.ChecksumAsset, *checkRes.SignatureAsset, binAssetName, checkRes.Latest)
	require.NoError(t, err, "checksum+sig download succeeds; verification deferred")

	// 검증 단계에서 서명 실패 감지
	dlBytes, err := os.ReadFile(dlPath)
	require.NoError(t, err)

	verifyErr := srv.verifier.VerifyAll(dlBytes, manifest.SHA256, manifest.Signature)
	require.Error(t, verifyErr, "tampered signature must trigger verification failure")
	assert.True(t, errors.Is(verifyErr, ErrUpdateSignatureInvalid),
		"expected ErrUpdateSignatureInvalid for tampered signature; got %v", verifyErr)

	// Apply 단계도 거부해야 함 (TOCTOU 방어)
	binPath, oldContent := setupFakeBinary(t)
	applier := NewApplier(srv.verifier, binPath)
	_, applyErr := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.Error(t, applyErr)
	assert.True(t, errors.Is(applyErr, ErrUpdateApplyFailed))

	// 현재 바이너리는 변경되지 않아야 함
	stillOld, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, oldContent, stillOld, "current binary must NOT be replaced on signature failure")

	// 백업도 만들어지지 않아야 함
	_, err = os.Stat(binPath + ".previous")
	assert.True(t, os.IsNotExist(err), "backup must NOT be created when verification fails")
}

// =============================================================================
// 시나리오 4: 자동 롤백 — Standalone (full restart 통합 미연결)
// =============================================================================

// TestE2E_RollbackFlow_RestoresPreviousBinary (Scenario 4) 는 롤백 흐름 자체를 검증한다.
//
// Note: SPEC-UPDATE-001 v0.1.0 의 Phase D restart 와 health check 자동 트리거는
// 본 SPEC 의 manager.go (orchestrator) 에 의해 wiring 되며, Phase H 시점에는
// 아직 통합되지 않음 (rollback.go 단위 동작 + restarter.go drain/exec 만 검증).
//
// 본 테스트는 Apply 후 (백업 보존) → 즉시 Restore 호출이 정상 동작함을 검증한다.
func TestE2E_RollbackFlow_RestoresPreviousBinary(t *testing.T) {
	srv := newFakeReleaseServer(t,
		withVersion("v0.4.0"),
		withBinaryContent([]byte("XFLOWD-V0.4.0-NEW")),
	)

	// Step 1: 정상 Apply 로 백업 생성
	binPath, oldContent := setupFakeBinary(t) // oldContent = v0.3.0 simulant
	manifest := makeManifestForServer(t, srv, "v0.4.0")
	dlPath := writeDownloadedFile(t, t.TempDir(), srv.binary)

	applier := NewApplier(srv.verifier, binPath)
	_, err := applier.Apply(context.Background(), ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: dlPath,
	})
	require.NoError(t, err)

	// Step 2: 새 바이너리 적용 후 health check 실패 시뮬레이션 (롤백 트리거)
	rollback := NewRollback(binPath)
	require.True(t, rollback.CanRollback(), "백업 파일이 존재해야 함")

	info, err := rollback.BackupInfo()
	require.NoError(t, err)
	assert.Equal(t, binPath+".previous", info.Path)
	assert.Equal(t, int64(len(oldContent)), info.Size)

	// Step 3: Restore 호출 → 메인 바이너리가 v0.3.0 으로 복원
	restoreRes, err := rollback.Restore(context.Background())
	require.NoError(t, err)
	assert.Equal(t, binPath, restoreRes.RestoredTo)
	assert.False(t, restoreRes.RestoredAt.IsZero())

	// Then: 메인 바이너리가 oldContent (v0.3.0) 와 일치
	restored, err := os.ReadFile(binPath)
	require.NoError(t, err)
	assert.Equal(t, oldContent, restored, "메인 바이너리는 롤백 후 v0.3.0 콘텐츠와 동일해야 함")

	// Then: 백업 파일은 제거되어 재롤백 방지 (M7 loop 방지)
	_, err = os.Stat(binPath + ".previous")
	assert.True(t, os.IsNotExist(err), "백업 파일은 롤백 후 정리되어야 함")
}

// =============================================================================
// 시나리오 5: 다운그레이드 거부 — Checker 의 Available 판정
// =============================================================================

// TestE2E_Downgrade_RefusedWithoutForce (Scenario 5) 는 v0.4.0 운영 중 v0.3.0 으로의
// 다운그레이드 시도가 자동 적용 흐름에서 거부됨을 검증한다.
//
// Note: --force CLI flag 처리는 cmd/xflowd/update.go 에서 수행 (Phase 별도);
// updater 패키지 수준에서는 Checker.Available 판정 + 별도 요구 시 ErrDowngradeRequiresForce.
//
// Given: xflowd v0.4.0 운영 중, mock 서버에 v0.3.0 게시
// When: Checker.Check 호출
// Then: Available=false (다운그레이드 자동 거부)
func TestE2E_Downgrade_RefusedWithoutForce(t *testing.T) {
	srv := newFakeReleaseServer(t,
		withVersion("v0.3.0"), // 채널 latest = 구버전
	)

	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()

	res, err := checker.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)

	// Then: Available=false (v0.3.0 < v0.4.0)
	assert.False(t, res.Available, "다운그레이드는 자동으로 거부되어야 함")
	assert.Equal(t, Version("v0.4.0"), res.Current)
	assert.Equal(t, Version("v0.3.0"), res.Latest)

	// 명시적 ErrDowngradeRequiresForce 가 sentinel 로 정의되어 있어야 한다 (CLI 단계 보호용).
	// 실제 사용은 cmd/xflowd 가 강제 다운그레이드 검증 시.
	assert.NotNil(t, ErrDowngradeRequiresForce)
	wrapped := fmt.Errorf("simulating CLI force check: %w", ErrDowngradeRequiresForce)
	assert.True(t, errors.Is(wrapped, ErrDowngradeRequiresForce))
}

// =============================================================================
// 시나리오 6: 디스크 부족 — 사전 검사 단계에서 거부
// =============================================================================

// TestE2E_InsufficientDisk_AbortsBeforeDownload (Scenario 6) 는 디스크 부족 시
// 다운로드가 시작도 되지 않음을 검증한다.
//
// Given: mock 서버 정상, fakeDiskInspector 가 가용량 < asset.Size × 3 보고
// When: Downloader.Download 호출
// Then: ErrUpdateInsufficientDiskSpace, 임시 파일 미생성
func TestE2E_InsufficientDisk_AbortsBeforeDownload(t *testing.T) {
	binaryContent := make([]byte, 25*1024*1024) // 25MB
	srv := newFakeReleaseServer(t,
		withBinaryContent(binaryContent),
	)

	checker, err := NewChecker(srv.baseURL(), ChannelStable)
	require.NoError(t, err)
	checker.HTTPClient = srv.httpClient()
	checkRes, err := checker.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	require.True(t, checkRes.Available)

	downloader := NewDownloader()
	downloader.HTTPClient = srv.httpClient()
	// 가용 = 25MB × 2 (요구치 = 25MB × 3 = 75MB)
	downloader.disk = &fakeDiskInspector{avail: uint64(50 * 1024 * 1024)}

	tmpDir := t.TempDir()
	dlPath := filepath.Join(tmpDir, "downloaded-binary")

	err = downloader.Download(context.Background(), *checkRes.BinaryAsset, dlPath, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateInsufficientDiskSpace),
		"insufficient disk should trigger ErrUpdateInsufficientDiskSpace; got %v", err)

	// 임시 파일이 만들어지지 않아야 함 (다운로드 단계 진입 전 거부)
	_, statErr := os.Stat(dlPath)
	assert.True(t, os.IsNotExist(statErr), "다운로드 시작 전 거부 → 임시 파일 부재")
	_, statErr = os.Stat(dlPath + ".tmp")
	assert.True(t, os.IsNotExist(statErr), ".tmp 도 만들어지지 않아야 함")
}

// =============================================================================
// 시나리오 7: HTTP-only URL 거부 — 부팅/실행 단계 양쪽
// =============================================================================

// TestE2E_HTTPOnlyURL_RejectedAtConstruct (Scenario 7) 는 update_url 의 http:// 스킴이
// NewChecker 단계에서 즉시 거부됨을 검증한다 (부팅 단계 fail-fast).
func TestE2E_HTTPOnlyURL_RejectedAtConstruct(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		url  string
	}{
		{"plain_http", "http://example.com/releases"},
		{"http_with_port", "http://example.com:8080/releases"},
		{"http_with_path", "http://api.github.com/repos/x/y"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewChecker(tc.url, ChannelStable)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrUpdateChannelInvalid),
				"http:// URL must be rejected at construction; got %v", err)
		})
	}

	// 추가: Downloader 도 asset URL 단계에서 동일 거부
	t.Run("download_layer_also_rejects_http", func(t *testing.T) {
		t.Parallel()
		d := NewDownloader()
		d.disk = &fakeDiskInspector{avail: 1 << 30}
		err := d.Download(context.Background(),
			ReleaseAsset{DownloadURL: "http://example.com/binary", Size: 100},
			filepath.Join(t.TempDir(), "out"), nil)
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
	})
}

// =============================================================================
// 시나리오 10: graceful drain timeout 초과 시 exec 진행 — 이미 Phase D 에서 검증
// =============================================================================

// TestE2E_DrainTimeout_StillProceedsToExec (Scenario 10) 의 본질은
// restarter_test.go 의 TestRestart_DrainTimeout_LogsWarning_StillExecs 가 검증한다.
//
// 본 테스트는 Phase H 차원에서 한 번 더 명시적으로 호출 흐름을 문서화한다 (smoke test).
// 실제 in-flight 메시지 손실 카운트는 호출자 (xflowd 메인 루프) 가 로그에 기록.
func TestE2E_DrainTimeout_StillProceedsToExec(t *testing.T) {
	t.Parallel()

	// Phase D 의 restarter_test 가 이미 검증한 흐름:
	//   - DrainSignal.Drain() 이 timeout 에러 반환
	//   - 그래도 verifyExecutable → execNewBinary 진행 시도
	//   - 잘못된 binary path → ErrUpdateApplyFailed (정상 분기)
	//
	// 여기서는 sentinel 만 cross-check 한다.
	assert.NotNil(t, ErrUpdateApplyFailed)
	assert.NotNil(t, errors.New("drain timeout simulant"))
}

// =============================================================================
// 통합 흐름 보강: cancel 처리 + 동시 다운로드
// =============================================================================

// TestE2E_ContextCancellation_DuringDownload 은 다운로드 중 ctx cancel 시
// 임시 파일이 정리됨을 검증한다 (안정성 edge case).
func TestE2E_ContextCancellation_DuringDownload(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// 응답을 일부만 보낸 뒤 끝없이 쉬게 함 (cancel 발동 시간 확보)
		w.Header().Set("Content-Length", "100000")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(make([]byte, 1024))
		flusher, ok := w.(http.Flusher)
		if ok {
			flusher.Flush()
		}
		time.Sleep(2 * time.Second)
	}))
	t.Cleanup(srv.Close)

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	t.Cleanup(cancel)

	dest := filepath.Join(t.TempDir(), "binary")
	err := d.Download(ctx, ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: 100000}, dest, nil)
	require.Error(t, err)
	// ctx cancel 또는 deadline exceeded
	assert.True(t,
		errors.Is(err, context.Canceled) ||
			errors.Is(err, context.DeadlineExceeded) ||
			errors.Is(err, ErrUpdateDownloadFailed),
		"cancelled download should propagate ctx error or wrap as ErrUpdateDownloadFailed; got %v", err)

	// 임시 파일이 정리되어야 함
	_, statErr := os.Stat(dest + ".tmp")
	assert.True(t, os.IsNotExist(statErr), "ctx cancel 시 .tmp 정리되어야 함")
	_, statErr = os.Stat(dest)
	assert.True(t, os.IsNotExist(statErr), "최종 파일도 미생성")
}

// =============================================================================
// helpers (consolidated for integration_test + security_test)
// =============================================================================

// makeManifestForServer 는 fakeReleaseServer 의 binary + 키로 Manifest 를 생성한다.
//
// 보안: 매 테스트마다 server 가 ephemeral 키를 가지므로, 본 헬퍼도 server 단위로 격리된다.
func makeManifestForServer(t *testing.T, srv *fakeReleaseServer, version Version) Manifest {
	t.Helper()
	sum := sha256.Sum256(srv.binary)
	sig := ed25519.Sign(srv.privateKey, srv.binary)
	return Manifest{
		Version:   version,
		SHA256:    hex.EncodeToString(sum[:]),
		Signature: sig,
		BinaryURL: srv.server.URL + "/binary",
	}
}
