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
	"net/http"
	"runtime"
	"strings"
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
	// 5 routes: version, check, apply, rollback, status
	assert.Equal(t, 5, router.RouteCount())
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
