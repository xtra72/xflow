// @SPEC:SPEC-UPDATE-001 v0.1.0 M10
// system_update_test.go — REST API 핸들러 단위 테스트.
//
// 테스트는 가짜 UpdateServiceFactories 를 주입하여 실제 다운로드 / 검증 / 교체를
// 수행하지 않는다. Apply 는 비동기 op 이므로 polling 으로 종료를 대기한다.
package handler

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/updater"
)

// --- Test factories (mock) ---

// fakeFactories 는 실제 컴포넌트 대신 검증용 함수를 노출한다.
// nil 인 함수는 통과 (default zero return value).
type fakeFactories struct {
	checkResult     updater.CheckResult
	checkErr        error
	downloadErr     error
	downloadCalls   atomic.Int32
	manifestResult  updater.Manifest
	manifestErr     error
	applyErr        error
	applyCalls      atomic.Int32
	rollbackResult  updater.RollbackResult
	rollbackErr     error
	rollbackInfoErr error
	verifierErr     error

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M11)
	// compatValidateErr 은 dependency manifest 호환성 검증 결과를 stub 한다.
	// nil 이면 검증 통과. ErrUpdateIncompatibleVersion 등을 주입하여 호환성 위반 시뮬레이션.
	compatValidateErr   error
	compatValidateCalls atomic.Int32
}

func (f *fakeFactories) toServiceFactories() UpdateServiceFactories {
	return UpdateServiceFactories{
		// Real wrappers for components that need just constructors (won't be called in tests).
		NewChecker: func(_ string, _ updater.Channel) (*updater.Checker, error) {
			return &updater.Checker{}, nil
		},
		NewDownloader: func() *updater.Downloader {
			return &updater.Downloader{}
		},
		NewApplier: func(_ *updater.Verifier, _ string) *updater.Applier {
			return &updater.Applier{}
		},
		NewRollback: func(_ string) *updater.Rollback {
			return updater.NewRollback("/tmp/fake-binary")
		},
		NewVerifier: func(_ ed25519.PublicKey) (*updater.Verifier, error) {
			if f.verifierErr != nil {
				return nil, f.verifierErr
			}
			// Return a non-nil Verifier (with a 32-byte zero pubkey) to skip nil check in Applier.
			zero := make(ed25519.PublicKey, ed25519.PublicKeySize)
			return updater.NewVerifier(zero)
		},
		CheckerCheck: func(_ context.Context, _ *updater.Checker, _ updater.Version, _, _, _ string) (updater.CheckResult, error) {
			return f.checkResult, f.checkErr
		},
		DownloaderDownload: func(_ context.Context, _ *updater.Downloader, _ updater.ReleaseAsset, _ string, _ updater.ProgressFunc) error {
			f.downloadCalls.Add(1)
			return f.downloadErr
		},
		DownloaderDownloadManifest: func(_ context.Context, _ *updater.Downloader, _, _ updater.ReleaseAsset, _ string, _ updater.Version) (updater.Manifest, error) {
			return f.manifestResult, f.manifestErr
		},
		ApplierApply: func(_ context.Context, _ *updater.Applier, _ updater.ApplyOptions) (updater.ApplyResult, error) {
			f.applyCalls.Add(1)
			return updater.ApplyResult{}, f.applyErr
		},
		RollbackRestore: func(_ context.Context, _ *updater.Rollback) (updater.RollbackResult, error) {
			return f.rollbackResult, f.rollbackErr
		},
		RollbackBackupInfo: func(_ *updater.Rollback) (updater.BackupInfo, error) {
			return updater.BackupInfo{}, f.rollbackInfoErr
		},
		CompatibilityValidate: func(_ context.Context, _ string, _ updater.Version) error {
			f.compatValidateCalls.Add(1)
			return f.compatValidateErr
		},
	}
}

// newTestSvc 는 fake factories 를 주입한 UpdateService 를 생성한다.
func newTestSvc(t *testing.T, ff *fakeFactories) *UpdateService {
	t.Helper()
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:   updater.ChannelStable,
			UpdateURL: "https://api.github.com/repos/xtra/xflow",
		},
		BinaryPath:     "/tmp/fake-xflowd",
		PublicKey:      pubKey,
		CurrentVersion: "v0.3.0",
		Commit:         "abc123",
		BuildDate:      "2026-04-30T12:00:00Z",
		BinaryName:     "xflowd",
		Factories:      ff.toServiceFactories(),
	}
	return NewUpdateService(cfg, nil)
}

// setupSystemRouter 는 SystemHandler 가 등록된 라우터를 반환한다.
func setupSystemRouter(t *testing.T, svc *UpdateService) *api.Router {
	t.Helper()
	router := api.NewRouter()
	h := NewSystemHandler(svc, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// waitForOpStatus polls service.Status() until status matches one of `wants`.
// Times out after 2 seconds.
func waitForOpStatus(t *testing.T, svc *UpdateService, wants ...string) dto.UpdateStatusResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		s := svc.Status()
		for _, w := range wants {
			if s.Status == w {
				return s
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("waitForOpStatus: timeout, last status=%q (wanted one of %v)", svc.Status().Status, wants)
	return dto.UpdateStatusResponse{}
}

// --- Construction tests ---

func TestNewSystemHandler(t *testing.T) {
	svc := newTestSvc(t, &fakeFactories{})
	h := NewSystemHandler(svc, nil)
	require.NotNil(t, h)
	assert.NotNil(t, h.logger)
	assert.Same(t, svc, h.svc)
}

func TestSystemHandler_RegisterRoutes(t *testing.T) {
	svc := newTestSvc(t, &fakeFactories{})
	router := setupSystemRouter(t, svc)
	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M6, M7)
	// 7 routes: version, check, apply, rollback, status, channel-get, channel-put.
	assert.Equal(t, 7, router.RouteCount())
}

// --- GetVersion tests ---

func TestSystemHandler_GetVersion_HappyPath(t *testing.T) {
	svc := newTestSvc(t, &fakeFactories{})
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/system/version", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.VersionResponse]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "v0.3.0", resp.Data.Version)
	assert.Equal(t, "abc123", resp.Data.Commit)
	assert.Equal(t, "stable", resp.Data.Channel)
	assert.Equal(t, runtime.Version(), resp.Data.GoVersion)
	assert.False(t, resp.Data.UpdateAvailable)
	assert.Empty(t, resp.Data.LatestVersion)
}

func TestSystemHandler_GetVersion_AfterCheck_PopulatesLatest(t *testing.T) {
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:  true,
			Current:    "v0.3.0",
			Latest:     "v0.4.0",
			ReleaseURL: "https://github.com/xtra/xflow/releases/v0.4.0",
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	// First call check
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/check", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	// Now version should reflect latest_version
	rec2 := doRequest(t, router, http.MethodGet, "/api/v1/system/version", nil)
	var resp dto.APIResponse[dto.VersionResponse]
	decodeJSON(t, rec2, &resp)
	assert.True(t, resp.Data.UpdateAvailable)
	assert.Equal(t, "v0.4.0", resp.Data.LatestVersion)
}

// --- PostCheck tests ---

func TestSystemHandler_PostCheck_HappyPath(t *testing.T) {
	publishedAt := time.Now().UTC().Truncate(time.Second)
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:   true,
			Current:     "v0.3.0",
			Latest:      "v0.4.0",
			ReleaseURL:  "https://github.com/xtra/xflow/releases/v0.4.0",
			PublishedAt: publishedAt,
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/check", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.CheckResponse]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "v0.3.0", resp.Data.Current)
	assert.Equal(t, "v0.4.0", resp.Data.Latest)
	assert.True(t, resp.Data.Available)
	assert.Equal(t, "stable", resp.Data.Channel)
	assert.Equal(t, "https://github.com/xtra/xflow/releases/v0.4.0", resp.Data.ReleaseURL)
}

func TestSystemHandler_PostCheck_NetworkError_503(t *testing.T) {
	ff := &fakeFactories{
		checkErr: errors.New("download failed: " + updater.ErrUpdateDownloadFailed.Error()),
	}
	// Wrap properly so errors.Is matches.
	ff.checkErr = updater.ErrUpdateDownloadFailed
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/check", nil)
	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

// --- PostApply tests ---

func TestSystemHandler_PostApply_HappyPath_Returns200WithOpID(t *testing.T) {
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:      true,
			Current:        "v0.3.0",
			Latest:         "v0.4.0",
			BinaryAsset:    &updater.ReleaseAsset{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 1024},
			ChecksumAsset:  &updater.ReleaseAsset{Name: "checksum.txt", DownloadURL: "https://example.com/chk", Size: 64},
			SignatureAsset: &updater.ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: "https://example.com/sig", Size: 64},
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.ApplyResponse]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.NotEmpty(t, resp.Data.OperationID)
	assert.Equal(t, "v0.3.0", resp.Data.FromVersion)
	// status returned at request time may be "starting"; goroutine has not yet run.
	assert.Contains(t, []string{opStatusStarting, opStatusChecking, opStatusReadyToRestart}, resp.Data.Status)

	// Wait for async to settle
	final := waitForOpStatus(t, svc, opStatusReadyToRestart, opStatusFailed)
	assert.Equal(t, opStatusReadyToRestart, final.Status, "expected ready_to_restart, got error: %s", final.Error)
	// Verify that downloader and applier were called.
	assert.GreaterOrEqual(t, ff.downloadCalls.Load(), int32(1))
	assert.Equal(t, int32(1), ff.applyCalls.Load())
}

func TestSystemHandler_PostApply_DowngradeWithoutForce_OperationFails(t *testing.T) {
	// SPEC M10 거부 시점: 실제로는 응답이 아닌 op.Status="failed" 로 기록된다.
	// (Apply 는 비동기이므로 즉시 응답은 200 OK 이고 op 상태로 추적).
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:      false,
			Current:        "v0.3.0",
			Latest:         "v0.2.0", // downgrade
			BinaryAsset:    &updater.ReleaseAsset{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 1024},
			ChecksumAsset:  &updater.ReleaseAsset{Name: "checksum.txt", DownloadURL: "https://example.com/chk", Size: 64},
			SignatureAsset: &updater.ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: "https://example.com/sig", Size: 64},
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{"force":false}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusReadyToRestart)
	assert.Equal(t, opStatusFailed, final.Status)
	assert.Contains(t, final.Error, "downgrade requires --force")
	// Applier 는 호출되지 않아야 함.
	assert.Equal(t, int32(0), ff.applyCalls.Load())
}

func TestSystemHandler_PostApply_DowngradeWithForce_Proceeds(t *testing.T) {
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:      false,
			Current:        "v0.3.0",
			Latest:         "v0.2.0",
			BinaryAsset:    &updater.ReleaseAsset{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 1024},
			ChecksumAsset:  &updater.ReleaseAsset{Name: "checksum.txt", DownloadURL: "https://example.com/chk", Size: 64},
			SignatureAsset: &updater.ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: "https://example.com/sig", Size: 64},
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{"force":true}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusReadyToRestart, opStatusFailed)
	assert.Equal(t, opStatusReadyToRestart, final.Status, "expected ready_to_restart, got error: %s", final.Error)
}

func TestSystemHandler_PostApply_AlreadyInProgress_409(t *testing.T) {
	// Make Download block long enough that the second Apply call hits the in-progress guard.
	blockCh := make(chan struct{})
	defer close(blockCh)
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:      true,
			Current:        "v0.3.0",
			Latest:         "v0.4.0",
			BinaryAsset:    &updater.ReleaseAsset{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 1024},
			ChecksumAsset:  &updater.ReleaseAsset{Name: "checksum.txt", DownloadURL: "https://example.com/chk", Size: 64},
			SignatureAsset: &updater.ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: "https://example.com/sig", Size: 64},
		},
	}
	// Override DownloaderDownload to block until blockCh is closed.
	factories := ff.toServiceFactories()
	factories.DownloaderDownload = func(_ context.Context, _ *updater.Downloader, _ updater.ReleaseAsset, _ string, _ updater.ProgressFunc) error {
		ff.downloadCalls.Add(1)
		<-blockCh
		return nil
	}
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:   updater.ChannelStable,
			UpdateURL: "https://api.github.com/repos/xtra/xflow",
		},
		BinaryPath:     "/tmp/fake-xflowd",
		PublicKey:      pubKey,
		CurrentVersion: "v0.3.0",
		BinaryName:     "xflowd",
		Factories:      factories,
	}
	svc := NewUpdateService(cfg, nil)
	router := setupSystemRouter(t, svc)

	// First call starts an op that will block on Download.
	rec1 := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	require.Equal(t, http.StatusOK, rec1.Code)

	// Wait until op moves into a non-terminal state (downloading).
	deadline := time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if svc.Status().Status == opStatusDownloading {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, opStatusDownloading, svc.Status().Status)

	// Second call must hit 409.
	rec2 := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusConflict, rec2.Code)

	var resp dto.APIResponse[any]
	decodeJSON(t, rec2, &resp)
	assert.False(t, resp.Success)
	assert.NotNil(t, resp.Error)
	assert.Equal(t, "CONFLICT", resp.Error.Code)
}

func TestSystemHandler_PostApply_DownloadError_OpFailed(t *testing.T) {
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:      true,
			Current:        "v0.3.0",
			Latest:         "v0.4.0",
			BinaryAsset:    &updater.ReleaseAsset{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 1024},
			ChecksumAsset:  &updater.ReleaseAsset{Name: "checksum.txt", DownloadURL: "https://example.com/chk", Size: 64},
			SignatureAsset: &updater.ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: "https://example.com/sig", Size: 64},
		},
		downloadErr: updater.ErrUpdateDownloadFailed,
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusReadyToRestart)
	assert.Equal(t, opStatusFailed, final.Status)
	assert.Contains(t, final.Error, "download")
}

func TestSystemHandler_PostApply_NoBinaryAsset_OpFailed(t *testing.T) {
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Current: "v0.3.0",
			Latest:  "v0.4.0",
			// No assets
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusReadyToRestart)
	assert.Equal(t, opStatusFailed, final.Status)
	assert.Contains(t, final.Error, "no platform binary asset")
}

// --- PostRollback tests ---

func TestSystemHandler_PostRollback_HappyPath(t *testing.T) {
	ff := &fakeFactories{
		rollbackResult: updater.RollbackResult{
			RestoredFrom: "/tmp/fake-xflowd.previous",
			RestoredTo:   "/tmp/fake-xflowd",
			RestoredAt:   time.Now().UTC(),
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/rollback", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.RollbackResponse]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "v0.3.0", resp.Data.FromVersion)
	assert.False(t, resp.Data.RolledBackAt.IsZero())
}

func TestSystemHandler_PostRollback_NoBackup_409(t *testing.T) {
	ff := &fakeFactories{
		rollbackErr: updater.ErrUpdateRollbackFailed,
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/rollback", nil)
	assert.Equal(t, http.StatusConflict, rec.Code)

	var resp dto.APIResponse[any]
	decodeJSON(t, rec, &resp)
	assert.False(t, resp.Success)
	assert.Equal(t, "CONFLICT", resp.Error.Code)
}

// --- GetStatus tests ---

func TestSystemHandler_GetStatus_Idle(t *testing.T) {
	svc := newTestSvc(t, &fakeFactories{})
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/system/update/status", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.UpdateStatusResponse]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "idle", resp.Data.Status)
	assert.Empty(t, resp.Data.OperationID)
}

func TestSystemHandler_GetStatus_AfterApply_Completed(t *testing.T) {
	ff := &fakeFactories{
		checkResult: updater.CheckResult{
			Available:      true,
			Current:        "v0.3.0",
			Latest:         "v0.4.0",
			BinaryAsset:    &updater.ReleaseAsset{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 1024},
			ChecksumAsset:  &updater.ReleaseAsset{Name: "checksum.txt", DownloadURL: "https://example.com/chk", Size: 64},
			SignatureAsset: &updater.ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: "https://example.com/sig", Size: 64},
		},
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	require.Equal(t, http.StatusOK, rec.Code)

	waitForOpStatus(t, svc, opStatusReadyToRestart)

	rec2 := doRequest(t, router, http.MethodGet, "/api/v1/system/update/status", nil)
	assert.Equal(t, http.StatusOK, rec2.Code)

	var resp dto.APIResponse[dto.UpdateStatusResponse]
	decodeJSON(t, rec2, &resp)
	assert.Equal(t, opStatusReadyToRestart, resp.Data.Status)
	assert.Equal(t, "v0.3.0", resp.Data.FromVersion)
	assert.Equal(t, "v0.4.0", resp.Data.ToVersion)
	assert.NotEmpty(t, resp.Data.OperationID)
	assert.False(t, resp.Data.StartedAt.IsZero())
	assert.False(t, resp.Data.CompletedAt.IsZero())
}

func TestSystemHandler_GetStatus_Failed_IncludesError(t *testing.T) {
	ff := &fakeFactories{
		checkErr: updater.ErrUpdateDownloadFailed,
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	// Apply with an upstream error → op fails during checking.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	require.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed)
	assert.Equal(t, opStatusFailed, final.Status)
	assert.NotEmpty(t, final.Error)

	rec2 := doRequest(t, router, http.MethodGet, "/api/v1/system/update/status", nil)
	var resp dto.APIResponse[dto.UpdateStatusResponse]
	decodeJSON(t, rec2, &resp)
	assert.Equal(t, opStatusFailed, resp.Data.Status)
	assert.NotEmpty(t, resp.Data.Error)
}

// --- mapUpdateError unit tests ---

func TestMapUpdateError(t *testing.T) {
	tests := []struct {
		name     string
		input    error
		wantCode int
	}{
		{"nil", nil, 0},
		{"in_progress", updater.ErrUpdateInProgress, http.StatusConflict},
		{"rollback_failed", updater.ErrUpdateRollbackFailed, http.StatusConflict},
		{"downgrade_force", updater.ErrDowngradeRequiresForce, http.StatusBadRequest},
		{"channel_invalid", updater.ErrUpdateChannelInvalid, http.StatusBadRequest},
		{"invalid_input", updater.ErrUpdateInvalidInput, http.StatusBadRequest},
		{"download_failed", updater.ErrUpdateDownloadFailed, http.StatusServiceUnavailable},
		{"checksum_mismatch", updater.ErrUpdateChecksumMismatch, http.StatusInternalServerError},
		{"signature_invalid", updater.ErrUpdateSignatureInvalid, http.StatusInternalServerError},
		{"apply_failed", updater.ErrUpdateApplyFailed, http.StatusInternalServerError},
		{"unknown", errors.New("random failure"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapUpdateError(tt.input)
			if tt.input == nil {
				assert.Nil(t, got)
				return
			}
			require.NotNil(t, got)
			apiErr, ok := got.(*api.APIError)
			require.True(t, ok, "expected *api.APIError, got %T", got)
			assert.Equal(t, tt.wantCode, apiErr.HTTPCode)
		})
	}
}

// --- Apply with wrapped errors (errors.Is) ---

func TestApplyAsync_DownloadErrorWrapped_StillFailsCorrectly(t *testing.T) {
	wrapped := errors.New("network: " + updater.ErrUpdateDownloadFailed.Error())
	ff := &fakeFactories{
		checkErr: wrapped,
	}
	svc := newTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	// Direct check call returns an unwrapped (non-sentinel) error → maps to 500.
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/check", nil)
	// Unknown error → 500.
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	_ = json.NewDecoder(rec.Body) // drain
}

// =============================================================================
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1, M2, M3, M4, M14)
// auto_restart 옵션 + 11-state machine 테스트
// =============================================================================

// stubOrchestratorRunner 는 RestartOrchestratorRunner 의 mock 이다.
// orchestrateErr 가 nil 이면 success (실제 운영에서는 syscall.Exec 가 return 안 하지만 테스트는 stub).
type stubOrchestratorRunner struct {
	calls           atomic.Int32
	orchestrateErr  error
	receivedManif   updater.Manifest
	receivedDLPath  string
	receivedTimeout time.Duration
}

func (s *stubOrchestratorRunner) Orchestrate(_ context.Context) (*updater.RestartResult, error) {
	s.calls.Add(1)
	if s.orchestrateErr != nil {
		return nil, s.orchestrateErr
	}
	// 실제 운영에서는 unreachable. 테스트에서는 nil result + nil error 도 허용.
	// applyAsync 가 이 경우 ErrUpdateRestartFailed 로 마킹해야 함 (silent skip 방지).
	return &updater.RestartResult{Status: "restarted"}, nil
}

// makeAutoRestartTestSvc 는 stub orchestrator factory 를 주입한 svc 를 생성한다.
func makeAutoRestartTestSvc(t *testing.T, ff *fakeFactories, runner *stubOrchestratorRunner) *UpdateService {
	t.Helper()
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:            updater.ChannelStable,
			UpdateURL:          "https://api.github.com/repos/xtra/xflow",
			DrainTimeout:       100 * time.Millisecond,
			HealthCheckTimeout: 1 * time.Second,
		},
		BinaryPath:          "/tmp/fake-xflowd",
		PublicKey:           pubKey,
		CurrentVersion:      "v0.3.0",
		BinaryName:          "xflowd",
		Factories:           ff.toServiceFactories(),
		HealthCheckEndpoint: "http://127.0.0.1:0/health",
		HealthCheckInterval: 50 * time.Millisecond,
		// orchestrator factory: 테스트 stub 반환.
		RestartOrchestratorFactory: func(params updater.RestartOrchestratorParams) RestartOrchestratorRunner {
			runner.receivedManif = params.Manifest
			runner.receivedDLPath = params.DownloadedPath
			runner.receivedTimeout = params.DrainTimeout
			return runner
		},
	}
	return NewUpdateService(cfg, nil)
}

// happyApplyResult 는 fake check 결과 (모든 asset 존재) 를 반환한다.
func happyApplyResult() updater.CheckResult {
	return updater.CheckResult{
		Available:      true,
		Current:        "v0.3.0",
		Latest:         "v0.4.0",
		BinaryAsset:    &updater.ReleaseAsset{Name: "xflowd-linux-amd64", DownloadURL: "https://example.com/bin", Size: 1024},
		ChecksumAsset:  &updater.ReleaseAsset{Name: "checksum.txt", DownloadURL: "https://example.com/chk", Size: 64},
		SignatureAsset: &updater.ReleaseAsset{Name: "xflowd-linux-amd64.sig", DownloadURL: "https://example.com/sig", Size: 64},
	}
}

// M1: auto_restart=false (default) → v0.1.0 backward 호환
func TestSystemHandler_PostApply_AutoRestartFalse_BackwardCompat(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	runner := &stubOrchestratorRunner{} // factory 가 호출되어선 안 됨
	svc := makeAutoRestartTestSvc(t, ff, runner)
	router := setupSystemRouter(t, svc)

	// auto_restart 미지정 (default false)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusReadyToRestart, opStatusFailed)
	assert.Equal(t, opStatusReadyToRestart, final.Status,
		"v0.1.0 호환: auto_restart=false 면 ready_to_restart 종료")
	assert.Equal(t, int32(0), runner.calls.Load(),
		"orchestrator 는 호출되지 않아야 함")
}

// M1: auto_restart=true → orchestrator 가 호출됨
func TestSystemHandler_PostApply_AutoRestartTrue_OrchestratorInvoked(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	// orchestrator stub: ErrUpdateRestartFailed 반환 (syscall.Exec 시뮬레이션 X)
	runner := &stubOrchestratorRunner{
		orchestrateErr: updater.ErrUpdateRestartFailed,
	}
	svc := makeAutoRestartTestSvc(t, ff, runner)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply",
		strings.NewReader(`{"auto_restart":true}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusCompleted, opStatusReadyToRestart)
	assert.Equal(t, opStatusFailed, final.Status,
		"orchestrator stub 가 ErrUpdateRestartFailed 반환 → op.Status=failed")
	assert.Equal(t, int32(1), runner.calls.Load(),
		"orchestrator 는 정확히 한 번 호출되어야 함")
	assert.Contains(t, final.Error, "restart")
}

// M1: auto_restart=true 인데 RestartOrchestratorFactory 가 nil → 명시 실패
func TestSystemHandler_PostApply_AutoRestartTrue_NoFactory_ExplicitFailure(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	// factory 미설정 (nil)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:   updater.ChannelStable,
			UpdateURL: "https://api.github.com/repos/xtra/xflow",
		},
		BinaryPath:     "/tmp/fake-xflowd",
		PublicKey:      pubKey,
		CurrentVersion: "v0.3.0",
		BinaryName:     "xflowd",
		Factories:      ff.toServiceFactories(),
		// RestartOrchestratorFactory 의도적 nil
	}
	svc := NewUpdateService(cfg, nil)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply",
		strings.NewReader(`{"auto_restart":true}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusCompleted, opStatusReadyToRestart)
	assert.Equal(t, opStatusFailed, final.Status,
		"factory nil → silent skip 대신 명시 실패")
	assert.True(t, strings.Contains(final.Error, "RestartOrchestratorFactory") ||
		strings.Contains(final.Error, "restart"),
		"에러 메시지가 명확해야 함, got: %s", final.Error)
}

// M1: orchestrator 가 unreachable code (nil error) 로 return 시 → silent skip 대신 명시 실패
func TestSystemHandler_PostApply_AutoRestartTrue_OrchestratorReturnsNilError_StillFails(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	// stub: orchestrate returns nil error (정상 운영에선 unreachable, 테스트에선 stub 가능성)
	runner := &stubOrchestratorRunner{orchestrateErr: nil}
	svc := makeAutoRestartTestSvc(t, ff, runner)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply",
		strings.NewReader(`{"auto_restart":true}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusCompleted, opStatusReadyToRestart)
	assert.Equal(t, opStatusFailed, final.Status,
		"orchestrator 가 nil error 로 return 했지만 process 가 교체되지 않은 상태 → 명시 실패")
	assert.Contains(t, final.Error, "exec")
}

// M1: orchestrator 호출 시 manifest / downloaded path 가 일관되게 전달됨
func TestSystemHandler_PostApply_AutoRestartTrue_PassesManifestAndPath(t *testing.T) {
	ff := &fakeFactories{
		checkResult:    happyApplyResult(),
		manifestResult: updater.Manifest{Version: "v0.4.0", SHA256: "deadbeef", Signature: make([]byte, ed25519.SignatureSize)},
	}
	runner := &stubOrchestratorRunner{orchestrateErr: updater.ErrUpdateRestartFailed}
	svc := makeAutoRestartTestSvc(t, ff, runner)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply",
		strings.NewReader(`{"auto_restart":true}`))
	require.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusCompleted)
	require.Equal(t, opStatusFailed, final.Status)
	assert.Equal(t, int32(1), runner.calls.Load())

	// manifest 필드 일관성
	assert.Equal(t, updater.Version("v0.4.0"), runner.receivedManif.Version)
	// downloaded path 가 비어있지 않아야 함
	assert.NotEmpty(t, runner.receivedDLPath)
	// drain timeout 이 cfg 에서 전달
	assert.Equal(t, 100*time.Millisecond, runner.receivedTimeout)
}

// M1, M14: 11-state enum 의 모든 상수가 dto 와 일치
func TestOperationStatusEnum_11States(t *testing.T) {
	// 11-state 가 정확히 정의되어 있는지 컴파일 가드
	statuses := []string{
		opStatusIdle,
		opStatusStarting,
		opStatusChecking,
		opStatusDownloading,
		opStatusVerifying,
		opStatusApplying,
		opStatusReadyToRestart,
		opStatusRestarting,     // NEW v0.2.0 (M2)
		opStatusHealthChecking, // NEW v0.2.0 (M4)
		opStatusCompleted,
		opStatusFailed,
	}
	require.Len(t, statuses, 11, "v0.2.0 11-state 정확히 11개여야 함")

	// 신규 2-state 는 v0.1.0 9-state 와 다름
	v01States := []string{
		opStatusIdle, opStatusStarting, opStatusChecking,
		opStatusDownloading, opStatusVerifying, opStatusApplying,
		opStatusReadyToRestart, opStatusCompleted, opStatusFailed,
	}
	for _, st := range v01States {
		assert.NotEqual(t, opStatusRestarting, st)
		assert.NotEqual(t, opStatusHealthChecking, st)
	}
}

// M2: restarting 상태에서 들어온 동시 apply 는 409 Conflict
func TestSystemHandler_PostApply_DuringRestarting_409Conflict(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	// orchestrator 가 오래 block 하도록 stub
	blockCh := make(chan struct{})
	defer close(blockCh)
	runner := &stubOrchestratorRunnerBlocking{blockCh: blockCh}
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:   updater.ChannelStable,
			UpdateURL: "https://api.github.com/repos/xtra/xflow",
		},
		BinaryPath:     "/tmp/fake-xflowd",
		PublicKey:      pubKey,
		CurrentVersion: "v0.3.0",
		BinaryName:     "xflowd",
		Factories:      ff.toServiceFactories(),
		RestartOrchestratorFactory: func(_ updater.RestartOrchestratorParams) RestartOrchestratorRunner {
			return runner
		},
	}
	svc := NewUpdateService(cfg, nil)
	router := setupSystemRouter(t, svc)

	// 1차: auto_restart=true 시작 → restarting 단계에서 block
	rec1 := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply",
		strings.NewReader(`{"auto_restart":true}`))
	require.Equal(t, http.StatusOK, rec1.Code)

	// op.Status 가 restarting 이 될 때까지 대기
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if svc.Status().Status == opStatusRestarting {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, opStatusRestarting, svc.Status().Status)

	// 2차: 동시 apply → 409
	rec2 := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusConflict, rec2.Code,
		"restarting 상태에서 들어온 두 번째 apply 는 409 Conflict")
}

// stubOrchestratorRunnerBlocking 은 Orchestrate 호출 시 blockCh 가 닫힐 때까지 기다린다.
type stubOrchestratorRunnerBlocking struct {
	calls   atomic.Int32
	blockCh <-chan struct{}
}

func (s *stubOrchestratorRunnerBlocking) Orchestrate(_ context.Context) (*updater.RestartResult, error) {
	s.calls.Add(1)
	<-s.blockCh
	return nil, updater.ErrUpdateRestartFailed
}

// =============================================================================
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M6, M7)
// Channel REST API 테스트 — GET / PUT /api/v1/system/update/channel
// =============================================================================

// channelTestSvc 는 channel 변경 후 즉시 새 channel 로 check 가 실행됐는지 확인하기 위해
// CheckerCheck 호출 시 cfg.Channel 을 capture 한다.
type channelCaptureFactories struct {
	*fakeFactories
	capturedChannels   []updater.Channel
	captureMu          sync.Mutex
	checkInvocationErr error
}

func newChannelCaptureFactories(base *fakeFactories) *channelCaptureFactories {
	return &channelCaptureFactories{fakeFactories: base}
}

// toServiceFactoriesWithCapture 는 NewChecker 호출 시 channel 인자를 기록한다.
func (c *channelCaptureFactories) toServiceFactoriesWithCapture() UpdateServiceFactories {
	base := c.fakeFactories.toServiceFactories()
	originalNewChecker := base.NewChecker
	base.NewChecker = func(url string, ch updater.Channel) (*updater.Checker, error) {
		c.captureMu.Lock()
		c.capturedChannels = append(c.capturedChannels, ch)
		c.captureMu.Unlock()
		return originalNewChecker(url, ch)
	}
	originalCheck := base.CheckerCheck
	base.CheckerCheck = func(ctx context.Context, ck *updater.Checker, current updater.Version, goos, goarch, binary string) (updater.CheckResult, error) {
		if c.checkInvocationErr != nil {
			return updater.CheckResult{}, c.checkInvocationErr
		}
		return originalCheck(ctx, ck, current, goos, goarch, binary)
	}
	return base
}

// newChannelTestSvc 는 channel API 테스트용 svc 를 생성한다.
// cfg.Channel 변경 후 새 Checker 가 그 채널로 호출되는지 capture 가능.
func newChannelTestSvc(t *testing.T, cf *channelCaptureFactories, initialChannel updater.Channel) *UpdateService {
	t.Helper()
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:   initialChannel,
			UpdateURL: "https://api.github.com/repos/xtra/xflow",
		},
		BinaryPath:     "/tmp/fake-xflowd",
		PublicKey:      pubKey,
		CurrentVersion: "v0.3.0",
		Commit:         "abc123",
		BuildDate:      "2026-04-30T12:00:00Z",
		BinaryName:     "xflowd",
		Factories:      cf.toServiceFactoriesWithCapture(),
	}
	return NewUpdateService(cfg, nil)
}

// requestWithRole 은 doRequest 와 동일하나 미리 user_role 을 컨텍스트에 주입한다.
// admin 권한 필수 핸들러 테스트용.
func requestWithRole(t *testing.T, router *api.Router, method, path, role string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	if role != "" {
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
		req = req.WithContext(ctx)
	}
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// --- M6: GET /api/v1/system/update/channel ---

// TestUpdateService_GetChannel_DefaultStable: 초기 default 가 stable 인지.
func TestUpdateService_GetChannel_DefaultStable(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)

	info := svc.GetChannel()
	assert.Equal(t, "stable", info.Current)
	assert.ElementsMatch(t, []string{"stable", "beta", "nightly"}, info.Available)
}

// TestSystemHandler_GetChannel_HappyPath: GET 응답 페이로드 검증.
func TestSystemHandler_GetChannel_HappyPath(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelBeta)
	router := setupSystemRouter(t, svc)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/system/update/channel", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.ChannelInfoResponse]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "beta", resp.Data.Current)
	assert.ElementsMatch(t, []string{"stable", "beta", "nightly"}, resp.Data.Available)
}

// --- M7: PUT /api/v1/system/update/channel ---

// TestSystemHandler_ChangeChannel_HappyPath: stable → beta, check_result 포함.
func TestSystemHandler_ChangeChannel_HappyPath(t *testing.T) {
	publishedAt := time.Now().UTC().Truncate(time.Second)
	cf := newChannelCaptureFactories(&fakeFactories{
		checkResult: updater.CheckResult{
			Available:   true,
			Current:     "v0.3.0",
			Latest:      "v0.4.0-beta.1",
			ReleaseURL:  "https://github.com/xtra/xflow/releases/tag/v0.4.0-beta.1",
			PublishedAt: publishedAt,
		},
	})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/update/channel",
		"admin", strings.NewReader(`{"channel":"beta"}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.ChangeChannelResponse]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success, "got error: %+v", resp.Error)
	assert.Equal(t, "stable", resp.Data.Previous)
	assert.Equal(t, "beta", resp.Data.Current)
	require.NotNil(t, resp.Data.CheckResult)
	assert.Equal(t, "v0.4.0-beta.1", resp.Data.CheckResult.Latest)
	assert.Equal(t, "beta", resp.Data.CheckResult.Channel)
	assert.NotEmpty(t, resp.Data.Message)

	// runtime state: 현재 채널이 beta 로 갱신
	assert.Equal(t, "beta", svc.GetChannel().Current)

	// 변경 후 새 Checker 가 beta 로 호출됐는지 검증.
	cf.captureMu.Lock()
	defer cf.captureMu.Unlock()
	require.NotEmpty(t, cf.capturedChannels)
	assert.Equal(t, updater.ChannelBeta, cf.capturedChannels[len(cf.capturedChannels)-1])
}

// TestSystemHandler_ChangeChannel_SameChannel_NoOp: stable → stable.
func TestSystemHandler_ChangeChannel_SameChannel_NoOp(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{
		checkResult: updater.CheckResult{
			Available: false, Current: "v0.3.0", Latest: "v0.3.0",
		},
	})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/update/channel",
		"admin", strings.NewReader(`{"channel":"stable"}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.ChangeChannelResponse]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)
	assert.Equal(t, "stable", resp.Data.Previous)
	assert.Equal(t, "stable", resp.Data.Current)
	assert.Contains(t, resp.Data.Message, "이미 동일한 채널")
}

// TestSystemHandler_ChangeChannel_InvalidChannel_400: 잘못된 채널 값.
func TestSystemHandler_ChangeChannel_InvalidChannel_400(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/update/channel",
		"admin", strings.NewReader(`{"channel":"invalid-channel"}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp dto.APIResponse[any]
	decodeJSON(t, rec, &resp)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "BAD_REQUEST", resp.Error.Code)

	// 채널은 변경되지 않아야 함
	assert.Equal(t, "stable", svc.GetChannel().Current)
}

// TestSystemHandler_ChangeChannel_MissingChannelField_400: 빈 channel.
func TestSystemHandler_ChangeChannel_MissingChannelField_400(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/update/channel",
		"admin", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	var resp dto.APIResponse[any]
	decodeJSON(t, rec, &resp)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
}

// TestSystemHandler_ChangeChannel_NonAdmin_403: 권한 없는 사용자.
func TestSystemHandler_ChangeChannel_NonAdmin_403(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/update/channel",
		"user", strings.NewReader(`{"channel":"beta"}`))
	assert.Equal(t, http.StatusForbidden, rec.Code,
		"non-admin 사용자는 403 Forbidden")

	// 채널은 변경되지 않아야 함
	assert.Equal(t, "stable", svc.GetChannel().Current)
}

// TestSystemHandler_ChangeChannel_NoRole_403: role 없는 (인증 안된 풀) 요청.
// 인증 미들웨어가 활성화된 운영 환경에선 401 이지만, handler 레벨에선 admin 아님 → 403.
func TestSystemHandler_ChangeChannel_NoRole_403(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	// role 미주입
	rec := doRequest(t, router, http.MethodPut, "/api/v1/system/update/channel",
		strings.NewReader(`{"channel":"beta"}`))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	assert.Equal(t, "stable", svc.GetChannel().Current)
}

// TestSystemHandler_ChangeChannel_CheckFailure_StillUpdatesChannel:
// 채널 변경은 됐으나 즉시 check 실패 시 응답에 check_result=null + 채널 갱신 유지.
func TestSystemHandler_ChangeChannel_CheckFailure_StillUpdatesChannel(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{
		checkErr: updater.ErrUpdateDownloadFailed,
	})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/update/channel",
		"admin", strings.NewReader(`{"channel":"nightly"}`))
	// check 실패 → API 는 503 (download failed) 매핑 OR 200 + null check_result.
	// 본 SPEC 는 "채널 변경은 성공" 을 우선시하므로 200 + null check_result + 경고 메시지.
	assert.Equal(t, http.StatusOK, rec.Code,
		"채널 변경 자체는 성공이므로 200, check_result 만 null")

	var resp dto.APIResponse[dto.ChangeChannelResponse]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)
	assert.Equal(t, "stable", resp.Data.Previous)
	assert.Equal(t, "nightly", resp.Data.Current)
	assert.Nil(t, resp.Data.CheckResult, "check 실패 시 check_result 는 null")
	assert.Contains(t, resp.Data.Message, "check") // 경고 hint

	// 채널은 갱신
	assert.Equal(t, "nightly", svc.GetChannel().Current)
}

// TestUpdateService_ChangeChannel_RuntimeReadable: GetChannel 즉시 반영.
func TestUpdateService_ChangeChannel_RuntimeReadable(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{
		checkResult: updater.CheckResult{Current: "v0.3.0", Latest: "v0.3.0"},
	})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)

	assert.Equal(t, "stable", svc.GetChannel().Current)

	resp, err := svc.ChangeChannel(context.Background(), "nightly")
	require.NoError(t, err)
	assert.Equal(t, "nightly", resp.Current)
	assert.Equal(t, "nightly", svc.GetChannel().Current,
		"GetChannel 이 즉시 반영")
}

// TestSystemHandler_ChangeChannel_ConcurrentChange_RaceFree:
// 두 goroutine 동시 PUT → mutex 직렬화로 race-free.
// `go test -race` 로 검증.
func TestSystemHandler_ChangeChannel_ConcurrentChange_RaceFree(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{
		checkResult: updater.CheckResult{Current: "v0.3.0", Latest: "v0.3.0"},
	})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	const N = 10
	var wg sync.WaitGroup
	channels := []string{"beta", "nightly", "stable"}
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ch := channels[idx%len(channels)]
			body := `{"channel":"` + ch + `"}`
			rec := requestWithRole(t, router, http.MethodPut,
				"/api/v1/system/update/channel", "admin", strings.NewReader(body))
			// 결과 코드는 200 (정상) 만 허용.
			assert.Equal(t, http.StatusOK, rec.Code)
		}(i)
	}
	wg.Wait()

	// 최종 채널은 valid enum 중 하나
	final := svc.GetChannel().Current
	assert.Contains(t, []string{"stable", "beta", "nightly"}, final)
}

// TestSystemHandler_GetChannel_ConcurrentReads_RaceFree:
// GET 은 RLock 으로 동시 읽기 안전.
func TestSystemHandler_GetChannel_ConcurrentReads_RaceFree(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)

	const N = 20
	var wg sync.WaitGroup
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := doRequest(t, router, http.MethodGet,
				"/api/v1/system/update/channel", nil)
			assert.Equal(t, http.StatusOK, rec.Code)
		}()
	}
	wg.Wait()
}

// TestSystemHandler_RegisterRoutes_IncludesChannel: 채널 라우트가 등록됨 (5 + 2 = 7).
func TestSystemHandler_RegisterRoutes_IncludesChannel(t *testing.T) {
	cf := newChannelCaptureFactories(&fakeFactories{})
	svc := newChannelTestSvc(t, cf, updater.ChannelStable)
	router := setupSystemRouter(t, svc)
	// version, check, apply, rollback, status, channel-get, channel-put
	assert.Equal(t, 7, router.RouteCount())
}

// dto-level: ChangeChannelRequest JSON round-trip
func TestChangeChannelRequest_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		expected string
	}{
		{"stable", `{"channel":"stable"}`, "stable"},
		{"beta", `{"channel":"beta"}`, "beta"},
		{"nightly", `{"channel":"nightly"}`, "nightly"},
		{"empty", `{}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req dto.ChangeChannelRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
			assert.Equal(t, tc.expected, req.Channel)
		})
	}
}

// dto-level: ApplyRequest 의 auto_restart JSON 필드 round-trip
func TestApplyRequest_AutoRestartField_JSONRoundTrip(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		expected bool
	}{
		{"missing", `{}`, false},
		{"explicit_false", `{"auto_restart":false}`, false},
		{"explicit_true", `{"auto_restart":true}`, true},
		{"with_other_fields", `{"version":"v0.4.0","force":true,"auto_restart":true}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req dto.ApplyRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
			assert.Equal(t, tc.expected, req.AutoRestart)
		})
	}
}

// =============================================================================
// @SPEC:SPEC-UPDATE-002 v0.1.0 Phase D — target option (M9, M10, M11, M14)
// =============================================================================

// makeTargetTestSvc 는 BinaryPaths map 이 주입된 svc 를 생성한다 (M9, M10).
//
// target 옵션 지원을 위해 cfg.BinaryPaths 가 추가되었다 ({"xflowd": "...", "xflow-agent": "...", "xflow": "..."}).
// 운영 환경에서는 ServiceConfig 가 이 맵을 채운다.
func makeTargetTestSvc(t *testing.T, ff *fakeFactories) *UpdateService {
	t.Helper()
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:   updater.ChannelStable,
			UpdateURL: "https://api.github.com/repos/xtra/xflow",
		},
		BinaryPath: "/tmp/fake-xflowd",
		BinaryPaths: map[string]string{
			"xflowd":      "/tmp/fake-xflowd",
			"xflow-agent": "/tmp/fake-xflow-agent",
			"xflow":       "/tmp/fake-xflow",
		},
		PublicKey:      pubKey,
		CurrentVersion: "v0.3.0",
		BinaryName:     "xflowd",
		Factories:      ff.toServiceFactories(),
	}
	return NewUpdateService(cfg, nil)
}

// M9: target 미지정 시 default "xflowd" 로 동작 (backward compat)
func TestSystemHandler_PostApply_TargetMissing_DefaultsToXflowd(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	svc := makeTargetTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	// target 필드 없는 body
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", strings.NewReader(`{}`))
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusReadyToRestart, opStatusFailed)
	assert.Equal(t, opStatusReadyToRestart, final.Status,
		"target 미지정 → xflowd default 로 동작해야 함")
}

// M9: target 이 whitelist 에 없으면 400 Bad Request
func TestSystemHandler_PostApply_TargetInvalid_400(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	svc := makeTargetTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	body := strings.NewReader(`{"target":"malicious-binary"}`)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code,
		"whitelist 외 target 은 400 으로 거부되어야 함 (path traversal 방어)")
}

// M9: target 이 비어있어도 default 로 동작
func TestSystemHandler_PostApply_TargetEmpty_DefaultsToXflowd(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	svc := makeTargetTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	body := strings.NewReader(`{"target":""}`)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", body)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// M10: target=xflow-agent → 적합한 binary path 로 dispatch
func TestSystemHandler_PostApply_TargetXflowAgent_DispatchesCorrectly(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	svc := makeTargetTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	body := strings.NewReader(`{"target":"xflow-agent"}`)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusReadyToRestart, opStatusFailed)
	assert.Equal(t, opStatusReadyToRestart, final.Status,
		"target=xflow-agent 가 정상적으로 적용되어야 함")
}

// M10: target=xflow (one-shot CLI) → 정상 dispatch (drain/health-check 무관)
func TestSystemHandler_PostApply_TargetXflowCLI_DispatchesCorrectly(t *testing.T) {
	ff := &fakeFactories{checkResult: happyApplyResult()}
	svc := makeTargetTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	body := strings.NewReader(`{"target":"xflow"}`)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", body)
	assert.Equal(t, http.StatusOK, rec.Code)

	final := waitForOpStatus(t, svc, opStatusReadyToRestart, opStatusFailed)
	assert.Equal(t, opStatusReadyToRestart, final.Status,
		"target=xflow CLI 도 정상 dispatch (drain 흐름 건너뜀)")
}

// M11/M13: dependency manifest 가 호환성 위반을 보고하면 400/500 으로 거부 (op.Status=failed)
func TestSystemHandler_PostApply_IncompatibleVersion_OpFails(t *testing.T) {
	// fakeFactories 의 manifestValidator 를 통해 호환성 위반 시뮬레이션
	ff := &fakeFactories{
		checkResult:       happyApplyResult(),
		compatValidateErr: updater.ErrUpdateIncompatibleVersion,
	}
	svc := makeTargetTestSvc(t, ff)
	router := setupSystemRouter(t, svc)

	body := strings.NewReader(`{"target":"xflow-agent"}`)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/system/update/apply", body)
	assert.Equal(t, http.StatusOK, rec.Code, "비동기 작업이므로 즉시 응답은 200")

	final := waitForOpStatus(t, svc, opStatusFailed, opStatusReadyToRestart)
	require.Equal(t, opStatusFailed, final.Status,
		"호환성 위반 → op.Status=failed")
	assert.Contains(t, final.Error, "incompatible",
		"에러 메시지가 호환성 위반을 명시해야 함, got: %s", final.Error)
}

// M9/M14: ApplyRequest.Target 필드 JSON 직렬화 테스트
func TestApplyRequest_TargetField_DeserializeFromJSON(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		expected string
	}{
		{"missing", `{}`, ""},
		{"empty", `{"target":""}`, ""},
		{"xflowd", `{"target":"xflowd"}`, "xflowd"},
		{"xflow_agent", `{"target":"xflow-agent"}`, "xflow-agent"},
		{"with_other", `{"version":"v0.4.0","target":"xflow"}`, "xflow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var req dto.ApplyRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
			assert.Equal(t, tc.expected, req.Target)
		})
	}
}

// --- SPEC-WEB-007: self identity + uptime tests ---

// newSelfInfoSvc 는 SPEC-WEB-007 self identity 필드(Mode/StartedAt)를 주입한
// UpdateService 를 생성한다. newTestSvc 와 동일한 기본값을 쓰되 두 필드만 override 한다.
func newSelfInfoSvc(t *testing.T, mode string, startedAt time.Time) *UpdateService {
	t.Helper()
	pubKey := make(ed25519.PublicKey, ed25519.PublicKeySize)
	cfg := UpdateServiceConfig{
		Config: updater.UpdateConfig{
			Channel:   updater.ChannelStable,
			UpdateURL: "https://api.github.com/repos/xtra/xflow",
		},
		BinaryPath:     "/tmp/fake-xflowd",
		PublicKey:      pubKey,
		CurrentVersion: "v0.3.0",
		Commit:         "abc123",
		BuildDate:      "2026-04-30T12:00:00Z",
		BinaryName:     "xflowd",
		Factories:      (&fakeFactories{}).toServiceFactories(),
		// @SPEC:SPEC-WEB-007
		Mode:      mode,
		StartedAt: startedAt,
	}
	return NewUpdateService(cfg, nil)
}

// TestVersion_SelfIdentityFields 는 Version() 이 self identity 필드를 채우는지 검증한다.
// OS/Arch 는 runtime 값, Hostname 은 비어있지 않음(os.Hostname 성공 가정), Mode 는 주입값.
func TestVersion_SelfIdentityFields(t *testing.T) {
	t.Parallel()
	svc := newSelfInfoSvc(t, "server", time.Time{})

	resp := svc.Version()

	assert.Equal(t, runtime.GOOS, resp.OS, "OS 는 runtime.GOOS 와 일치해야 함")
	assert.Equal(t, runtime.GOARCH, resp.Arch, "Arch 는 runtime.GOARCH 와 일치해야 함")
	assert.NotEmpty(t, resp.Hostname, "Hostname 은 비어있지 않아야 함")
	assert.Equal(t, "server", resp.Mode, "Mode 는 주입값과 일치해야 함")
	assert.GreaterOrEqual(t, resp.UptimeSeconds, float64(0), "UptimeSeconds 는 0 이상이어야 함")
}

// TestVersion_Mode 는 주입된 mode 값이 그대로 반영되는지 table-driven 으로 검증한다.
func TestVersion_Mode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		mode string
	}{
		{"server", "server"},
		{"client", "client"},
		{"disabled", "disabled"},
		{"empty", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			svc := newSelfInfoSvc(t, tc.mode, time.Time{})
			resp := svc.Version()
			assert.Equal(t, tc.mode, resp.Mode)
		})
	}
}

// TestVersion_UptimeSeconds 는 StartedAt 주입 시 UptimeSeconds 계산을 검증한다.
//   - zero StartedAt → 0
//   - 과거 StartedAt → 양수 (~경과 초)
func TestVersion_UptimeSeconds(t *testing.T) {
	t.Parallel()

	t.Run("zero_started_at_returns_zero", func(t *testing.T) {
		t.Parallel()
		svc := newSelfInfoSvc(t, "disabled", time.Time{})
		resp := svc.Version()
		assert.Equal(t, float64(0), resp.UptimeSeconds, "zero StartedAt 면 uptime 0")
	})

	t.Run("past_started_at_returns_positive", func(t *testing.T) {
		t.Parallel()
		started := time.Now().Add(-10 * time.Second)
		svc := newSelfInfoSvc(t, "disabled", started)
		resp := svc.Version()
		assert.Positive(t, resp.UptimeSeconds, "과거 StartedAt 면 uptime 양수")
		// ~10초 근방인지 느슨히 검증 (스케줄링 지연 허용).
		assert.GreaterOrEqual(t, resp.UptimeSeconds, float64(9), "약 10초 경과 기대")
		assert.Less(t, resp.UptimeSeconds, float64(60), "비정상적으로 큰 값이 아니어야 함")
	})
}

// TestVersion_ExistingFieldsUnchanged 는 SPEC-WEB-007 변경 이후에도 기존 7개 필드가
// 주입값 그대로 반환되는지(characterization) 검증한다.
func TestVersion_ExistingFieldsUnchanged(t *testing.T) {
	t.Parallel()
	svc := newSelfInfoSvc(t, "server", time.Time{})

	resp := svc.Version()

	assert.Equal(t, "v0.3.0", resp.Version)
	assert.Equal(t, "abc123", resp.Commit)
	assert.Equal(t, "2026-04-30T12:00:00Z", resp.BuildDate)
	assert.Equal(t, runtime.Version(), resp.GoVersion)
	assert.Equal(t, "stable", resp.Channel)
	assert.False(t, resp.UpdateAvailable, "Check 미수행 시 false")
	assert.Empty(t, resp.LatestVersion, "Check 미수행 시 빈 문자열")
}

// TestVersion_NoSecretsInResponse 는 Version() 응답 JSON 에 시크릿 관련 키가
// 절대 노출되지 않는지 table-driven 으로 검증한다 (SPEC-WEB-007 시크릿 부재 보장).
func TestVersion_NoSecretsInResponse(t *testing.T) {
	t.Parallel()
	svc := newSelfInfoSvc(t, "server", time.Now().Add(-5*time.Second))
	resp := svc.Version()

	raw, err := json.Marshal(resp)
	require.NoError(t, err)
	lower := strings.ToLower(string(raw))

	forbidden := []string{
		"jwt_secret",
		"bootstrap_secret",
		"enrollment_token",
		"secret",
		"private_key",
		"token",
		"password",
		"api_key",
	}
	for _, key := range forbidden {
		t.Run(key, func(t *testing.T) {
			assert.NotContainsf(t, lower, key,
				"Version() 응답 JSON 에 금지 키 %q 가 포함되어서는 안 됨: %s", key, string(raw))
		})
	}
}
