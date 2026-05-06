// @SPEC:SPEC-UPDATE-001 v0.1.0 M10
// system_update.go — REST API 핸들러 + UpdateService 오케스트레이션.
//
// 본 파일은 두 컴포넌트를 함께 제공한다:
//
//  1. UpdateService — Apply / Check / Rollback / Status 오케스트레이션.
//     - Phase A-D 의 Checker / Downloader / Verifier / Applier / Rollback 을 조립한다.
//     - mu 로 직렬화된 단일 op 만 추적 (동시 호출은 ErrUpdateInProgress).
//     - applyAsync 는 goroutine 으로 실행되어 응답이 즉시 반환된다.
//
//  2. SystemHandler — UpdateService 를 HTTP 엔드포인트에 매핑.
//     - GET /api/v1/system/version
//     - POST /api/v1/system/update/check
//     - POST /api/v1/system/update/apply
//     - POST /api/v1/system/update/rollback
//     - GET /api/v1/system/update/status
//
// 보안 critical:
//   - Apply 는 1 회만 동시 진행 (mu 직렬화)
//   - Force 없는 다운그레이드는 거부 (M8)
//   - 검증 실패 (signature / checksum) 는 op.Status="failed" 로 기록 + 임시파일 정리
//
// 주의: 본 파일은 새 바이너리로 in-process exec 을 수행하지 않는다.
// "ready_to_restart" 상태에서 외부 supervisor (systemd 등) 또는 별도 restart 트리거가
// 새 바이너리로의 전환을 담당한다. 자세한 architecture 결정은 SPEC-UPDATE-001 참조.
package handler

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/updater"
)

// 작업 상태 enum (UpdateStatusResponse.Status 값과 매핑됨).
const (
	opStatusIdle           = "idle"
	opStatusStarting       = "starting"
	opStatusChecking       = "checking"
	opStatusDownloading    = "downloading"
	opStatusVerifying      = "verifying"
	opStatusApplying       = "applying"
	opStatusReadyToRestart = "ready_to_restart"
	opStatusCompleted      = "completed"
	opStatusFailed         = "failed"
)

// updateOperation 은 단일 update 작업의 상태이다.
//
// 모든 필드는 UpdateService.mu 가 보호한다 (op 갱신은 service 내부에서만 수행).
type updateOperation struct {
	ID          string
	Status      string
	FromVersion updater.Version
	ToVersion   updater.Version
	StartedAt   time.Time
	CompletedAt time.Time
	Error       string
}

// snapshot 은 락 외부에서 읽기 안전한 복사본을 반환한다.
func (op *updateOperation) snapshot() updateOperation {
	return *op
}

// UpdateServiceFactories 는 Phase A-D 의 컴포넌트 생성 함수를 모아둔 의존성 셋이다.
//
// Production 에서는 defaultUpdateServiceFactories 를 사용하며, 테스트에서는
// 가짜 함수로 대체하여 실제 다운로드/검증/교체를 건너뛸 수 있다.
type UpdateServiceFactories struct {
	NewChecker    func(url string, ch updater.Channel) (*updater.Checker, error)
	NewDownloader func() *updater.Downloader
	NewApplier    func(v *updater.Verifier, path string) *updater.Applier
	NewRollback   func(path string) *updater.Rollback
	NewVerifier   func(pubKey ed25519.PublicKey) (*updater.Verifier, error)

	// CheckerCheck 는 Checker.Check 를 wrapping (테스트에서 fake 결과 반환 가능).
	CheckerCheck func(ctx context.Context, c *updater.Checker, current updater.Version, goos, goarch, binary string) (updater.CheckResult, error)

	// DownloaderDownload 는 Downloader.Download 를 wrapping.
	DownloaderDownload func(ctx context.Context, d *updater.Downloader, asset updater.ReleaseAsset, destPath string, progress updater.ProgressFunc) error

	// DownloaderDownloadManifest 는 Downloader.DownloadManifest 를 wrapping.
	DownloaderDownloadManifest func(ctx context.Context, d *updater.Downloader, checksumAsset, signatureAsset updater.ReleaseAsset, binaryName string, version updater.Version) (updater.Manifest, error)

	// ApplierApply 는 Applier.Apply 를 wrapping.
	ApplierApply func(ctx context.Context, a *updater.Applier, opts updater.ApplyOptions) (updater.ApplyResult, error)

	// RollbackRestore 는 Rollback.Restore 를 wrapping.
	RollbackRestore func(ctx context.Context, r *updater.Rollback) (updater.RollbackResult, error)

	// RollbackBackupInfo 는 Rollback.BackupInfo 를 wrapping (FromVersion 추정용).
	RollbackBackupInfo func(r *updater.Rollback) (updater.BackupInfo, error)
}

// DefaultUpdateServiceFactories 는 production 기본 의존성 셋을 반환한다.
func DefaultUpdateServiceFactories() UpdateServiceFactories {
	return UpdateServiceFactories{
		NewChecker:    updater.NewChecker,
		NewDownloader: updater.NewDownloader,
		NewApplier:    updater.NewApplier,
		NewRollback:   updater.NewRollback,
		NewVerifier:   updater.NewVerifier,
		CheckerCheck: func(ctx context.Context, c *updater.Checker, current updater.Version, goos, goarch, binary string) (updater.CheckResult, error) {
			return c.Check(ctx, current, goos, goarch, binary)
		},
		DownloaderDownload: func(ctx context.Context, d *updater.Downloader, asset updater.ReleaseAsset, destPath string, progress updater.ProgressFunc) error {
			return d.Download(ctx, asset, destPath, progress)
		},
		DownloaderDownloadManifest: func(ctx context.Context, d *updater.Downloader, checksumAsset, signatureAsset updater.ReleaseAsset, binaryName string, version updater.Version) (updater.Manifest, error) {
			return d.DownloadManifest(ctx, checksumAsset, signatureAsset, binaryName, version)
		},
		ApplierApply: func(ctx context.Context, a *updater.Applier, opts updater.ApplyOptions) (updater.ApplyResult, error) {
			return a.Apply(ctx, opts)
		},
		RollbackRestore: func(ctx context.Context, r *updater.Rollback) (updater.RollbackResult, error) {
			return r.Restore(ctx)
		},
		RollbackBackupInfo: func(r *updater.Rollback) (updater.BackupInfo, error) {
			return r.BackupInfo()
		},
	}
}

// UpdateServiceConfig 는 UpdateService 생성 시 필수 외부 의존성을 모아둔 설정이다.
type UpdateServiceConfig struct {
	// UpdateConfig 는 운영자 설정 (channel, update_url, drain_timeout 등).
	Config updater.UpdateConfig

	// BinaryPath 는 현재 실행 중 바이너리 경로 (os.Executable 결과).
	BinaryPath string

	// PublicKey 는 Ed25519 공개키 (32 bytes). nil 이면 Apply 가 즉시 실패.
	PublicKey ed25519.PublicKey

	// CurrentVersion 은 현재 빌드 버전 ("v0.3.0" 등). 빈 문자열도 허용 (display 만).
	CurrentVersion string

	// Commit 은 빌드 시 git commit SHA.
	Commit string

	// BuildDate 은 빌드 시각.
	BuildDate string

	// BinaryName 은 asset 매칭에 사용 (대개 "xflowd").
	BinaryName string

	// Factories 는 Phase A-D 컴포넌트 생성 함수 셋. 빈 값이면 production 기본값 사용.
	Factories UpdateServiceFactories
}

// UpdateService 는 update 작업의 오케스트레이션을 담당한다.
//
// 동시성:
//   - Apply / Rollback 은 mu 로 직렬화된다 (한 번에 한 op).
//   - Status 는 mu.RLock 으로 안전하게 snapshot 을 반환.
//   - applyAsync goroutine 은 mu.Lock 을 자체 관리한다.
type UpdateService struct {
	cfg UpdateServiceConfig

	mu        sync.RWMutex
	currentOp *updateOperation

	logger *slog.Logger
}

// NewUpdateService 는 UpdateService 를 생성한다.
//
// cfg.Factories 가 zero value 이면 DefaultUpdateServiceFactories 가 적용된다.
// logger 가 nil 이면 slog.Default() 가 사용된다.
func NewUpdateService(cfg UpdateServiceConfig, logger *slog.Logger) *UpdateService {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Factories.NewChecker == nil {
		cfg.Factories = DefaultUpdateServiceFactories()
	}
	return &UpdateService{
		cfg:    cfg,
		logger: logger,
	}
}

// Version 은 현재 실행 중 바이너리의 메타데이터 + 마지막 Check 결과를 반환한다.
//
// 락 비용을 최소화하기 위해 RLock 만 사용한다.
func (s *UpdateService) Version() dto.VersionResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	resp := dto.VersionResponse{
		Version:   s.cfg.CurrentVersion,
		Commit:    s.cfg.Commit,
		BuildDate: s.cfg.BuildDate,
		GoVersion: runtime.Version(),
		Channel:   string(s.cfg.Config.Channel),
	}
	if s.currentOp != nil && s.currentOp.ToVersion != "" {
		resp.LatestVersion = string(s.currentOp.ToVersion)
		// completed/ready_to_restart 면 update 가 적용된 상태이므로 false (이미 최신).
		// 그 외 op (예: check 만 수행) 에서는 latest > current 일 때만 true 가 되도록 단순화.
		if s.currentOp.Status != opStatusCompleted && s.currentOp.Status != opStatusReadyToRestart {
			cur := updater.Version(s.cfg.CurrentVersion)
			if s.currentOp.ToVersion.Compare(cur) > 0 {
				resp.UpdateAvailable = true
			}
		}
	}
	return resp
}

// Check 는 채널에서 최신 버전을 조회하고 결과를 반환한다.
//
// 작업이 진행 중이어도 Check 는 영향을 주지 않는다 (read-only operation).
// 결과는 currentOp.ToVersion 에 캐시되어 다음 Version() 호출에서 사용된다.
func (s *UpdateService) Check(ctx context.Context) (dto.CheckResponse, error) {
	checker, err := s.cfg.Factories.NewChecker(s.cfg.Config.UpdateURL, s.cfg.Config.Channel)
	if err != nil {
		return dto.CheckResponse{}, err
	}

	current := updater.Version(s.cfg.CurrentVersion)
	result, err := s.cfg.Factories.CheckerCheck(ctx, checker, current, runtime.GOOS, runtime.GOARCH, s.cfg.BinaryName)
	if err != nil {
		return dto.CheckResponse{}, err
	}

	// Latest 결과를 op 에 기록 (Version() 응답에 반영).
	s.mu.Lock()
	if s.currentOp == nil {
		s.currentOp = &updateOperation{
			ID:          uuid.NewString(),
			Status:      opStatusIdle,
			FromVersion: current,
			ToVersion:   result.Latest,
			StartedAt:   time.Now().UTC(),
			CompletedAt: time.Now().UTC(),
		}
	} else if !s.opInProgressLocked() {
		s.currentOp.ToVersion = result.Latest
	}
	s.mu.Unlock()

	return dto.CheckResponse{
		Current:     string(current),
		Latest:      string(result.Latest),
		Available:   result.Available,
		Channel:     string(s.cfg.Config.Channel),
		ReleaseURL:  result.ReleaseURL,
		PublishedAt: result.PublishedAt.UTC(),
	}, nil
}

// Apply 는 비동기로 update 작업을 시작하고 op 식별자를 즉시 반환한다.
//
// 흐름:
//  1. mu.Lock 후 진행 중 op 가 있으면 ErrUpdateInProgress 반환 (HTTP 409).
//  2. 새 op 를 생성하고 currentOp 에 저장.
//  3. mu.Unlock 하고 applyAsync 를 goroutine 으로 실행.
//  4. ApplyResponse 즉시 반환 (status="starting").
//
// requestedVersion 이 빈 문자열이면 channel 의 latest 사용.
// force=false 인데 latest < current 면 op 가 failed 로 마킹된다 (응답 자체는 200).
//
// 비동기 동작: 호출자 ctx 가 cancel 되어도 update 자체는 완료까지 진행되어야 하므로
// 내부적으로 분리된 백그라운드 context 를 사용한다 (HTTP request lifecycle 과 분리).
func (s *UpdateService) Apply(_ context.Context, requestedVersion string, force bool) (dto.ApplyResponse, error) {
	current := updater.Version(s.cfg.CurrentVersion)

	s.mu.Lock()
	if s.opInProgressLocked() {
		s.mu.Unlock()
		return dto.ApplyResponse{}, updater.ErrUpdateInProgress
	}

	op := &updateOperation{
		ID:          uuid.NewString(),
		Status:      opStatusStarting,
		FromVersion: current,
		ToVersion:   updater.Version(requestedVersion),
		StartedAt:   time.Now().UTC(),
	}
	s.currentOp = op
	// 락 보유 중에 응답 필드를 캡처 (goroutine 이 op.Status 갱신 전에 race 방지).
	respID := op.ID
	respStatus := op.Status
	s.mu.Unlock()

	// 백그라운드 goroutine 은 새로운 context 를 사용하여 HTTP request 종료와 무관하게 진행.
	bgCtx := context.Background()
	go s.applyAsync(bgCtx, op, requestedVersion, force)

	return dto.ApplyResponse{
		OperationID: respID,
		Status:      respStatus,
		FromVersion: string(current),
		ToVersion:   requestedVersion,
	}, nil
}

// Rollback 은 백업 바이너리를 동기적으로 복원한다.
//
// Apply 와 동일하게 mu 직렬화. 백업이 없으면 ErrUpdateRollbackFailed 반환 (HTTP 409).
func (s *UpdateService) Rollback(ctx context.Context) (dto.RollbackResponse, error) {
	s.mu.Lock()
	if s.opInProgressLocked() {
		s.mu.Unlock()
		return dto.RollbackResponse{}, updater.ErrUpdateInProgress
	}
	op := &updateOperation{
		ID:          uuid.NewString(),
		Status:      opStatusApplying,
		FromVersion: updater.Version(s.cfg.CurrentVersion),
		StartedAt:   time.Now().UTC(),
	}
	s.currentOp = op
	s.mu.Unlock()

	rb := s.cfg.Factories.NewRollback(s.cfg.BinaryPath)
	result, err := s.cfg.Factories.RollbackRestore(ctx, rb)
	if err != nil {
		s.markOpFailed(op, err)
		return dto.RollbackResponse{}, err
	}

	s.markOpCompleted(op, opStatusCompleted)

	return dto.RollbackResponse{
		FromVersion:  string(op.FromVersion),
		ToVersion:    "",
		RolledBackAt: result.RestoredAt.UTC(),
	}, nil
}

// Status 는 마지막 작업의 snapshot 을 반환한다 (idle 이면 status="idle").
func (s *UpdateService) Status() dto.UpdateStatusResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.currentOp == nil {
		return dto.UpdateStatusResponse{Status: opStatusIdle}
	}
	op := s.currentOp.snapshot()
	return dto.UpdateStatusResponse{
		OperationID: op.ID,
		Status:      op.Status,
		FromVersion: string(op.FromVersion),
		ToVersion:   string(op.ToVersion),
		StartedAt:   op.StartedAt,
		CompletedAt: op.CompletedAt,
		Error:       op.Error,
	}
}

// opInProgressLocked 는 현재 op 가 종료 상태가 아닌지 확인한다 (mu 보유 가정).
func (s *UpdateService) opInProgressLocked() bool {
	if s.currentOp == nil {
		return false
	}
	switch s.currentOp.Status {
	case opStatusIdle, opStatusCompleted, opStatusFailed, opStatusReadyToRestart:
		return false
	default:
		return true
	}
}

// markOpFailed 는 op 를 실패 상태로 마킹한다 (mu 자체 관리).
func (s *UpdateService) markOpFailed(op *updateOperation, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op.Status = opStatusFailed
	op.Error = err.Error()
	op.CompletedAt = time.Now().UTC()
}

// markOpStatus 는 진행 단계 상태만 갱신한다 (mu 자체 관리).
func (s *UpdateService) markOpStatus(op *updateOperation, status string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op.Status = status
}

// markOpToVersion 은 op 의 ToVersion 을 갱신한다 (Check 단계 후 사용).
func (s *UpdateService) markOpToVersion(op *updateOperation, v updater.Version) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op.ToVersion = v
}

// markOpCompleted 는 op 를 종료 상태로 마킹한다 (mu 자체 관리).
func (s *UpdateService) markOpCompleted(op *updateOperation, finalStatus string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	op.Status = finalStatus
	op.CompletedAt = time.Now().UTC()
}

// applyAsync 는 goroutine 으로 실행되어 전체 update 파이프라인을 수행한다.
//
// 단계: checking → downloading → verifying → applying → ready_to_restart.
// 각 단계 실패 시 op.Status = "failed" + Error 메시지 기록.
func (s *UpdateService) applyAsync(ctx context.Context, op *updateOperation, requestedVersion string, force bool) {
	// 1. Checker 생성 + 채널 조회
	s.markOpStatus(op, opStatusChecking)

	checker, err := s.cfg.Factories.NewChecker(s.cfg.Config.UpdateURL, s.cfg.Config.Channel)
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("create checker: %w", err))
		return
	}

	current := op.FromVersion
	result, err := s.cfg.Factories.CheckerCheck(ctx, checker, current, runtime.GOOS, runtime.GOARCH, s.cfg.BinaryName)
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("check: %w", err))
		return
	}

	// requestedVersion 처리: 빈 문자열 → latest 사용. 명시 버전이면 다운그레이드 검사.
	target := result.Latest
	if requestedVersion != "" {
		target = updater.Version(requestedVersion)
	}
	s.markOpToVersion(op, target)

	// 다운그레이드 보호 (M8): force 없으면 거부.
	if target.Compare(current) < 0 && !force {
		s.markOpFailed(op, updater.ErrDowngradeRequiresForce)
		return
	}

	// asset 가용성 확인: result 는 latest 를 기준으로 매칭한 asset 셋이므로,
	// requestedVersion != latest 인 경우 asset 매칭이 다를 수 있다.
	// SPEC v0.1.0 는 latest 만 지원 (requestedVersion=latest 가 아니면 asset 매칭 실패 가능).
	if result.BinaryAsset == nil {
		s.markOpFailed(op, fmt.Errorf("%w: no platform binary asset for %s/%s",
			updater.ErrUpdateDownloadFailed, runtime.GOOS, runtime.GOARCH))
		return
	}
	if result.ChecksumAsset == nil || result.SignatureAsset == nil {
		s.markOpFailed(op, fmt.Errorf("%w: missing checksum or signature asset",
			updater.ErrUpdateDownloadFailed))
		return
	}

	// 2. Download
	s.markOpStatus(op, opStatusDownloading)

	tmpDir, err := os.MkdirTemp("", "xflow-update-*")
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("create tmp dir: %w", err))
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, s.cfg.BinaryName+"-new")
	d := s.cfg.Factories.NewDownloader()
	if err := s.cfg.Factories.DownloaderDownload(ctx, d, *result.BinaryAsset, binPath, nil); err != nil {
		s.markOpFailed(op, fmt.Errorf("download binary: %w", err))
		return
	}

	binaryName := updater.AssetName(s.cfg.BinaryName, runtime.GOOS, runtime.GOARCH)
	manifest, err := s.cfg.Factories.DownloaderDownloadManifest(
		ctx, d, *result.ChecksumAsset, *result.SignatureAsset, binaryName, target,
	)
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("download manifest: %w", err))
		return
	}
	manifest.BinaryURL = result.BinaryAsset.DownloadURL

	// 3. Verify
	s.markOpStatus(op, opStatusVerifying)

	if s.cfg.PublicKey == nil {
		s.markOpFailed(op, fmt.Errorf("%w: public key not configured", updater.ErrUpdateInvalidInput))
		return
	}

	verifier, err := s.cfg.Factories.NewVerifier(s.cfg.PublicKey)
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("create verifier: %w", err))
		return
	}

	// Phase C 의 Applier.Apply 가 내부적으로 VerifyAll 을 다시 수행하므로 (TOCTOU 방어),
	// 여기서 별도의 사전 검증은 생략한다. Phase B downloader 가 이미 1차 검증 수행.

	// 4. Apply
	s.markOpStatus(op, opStatusApplying)

	applier := s.cfg.Factories.NewApplier(verifier, s.cfg.BinaryPath)
	_, err = s.cfg.Factories.ApplierApply(ctx, applier, updater.ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: binPath,
	})
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("apply: %w", err))
		return
	}

	// 5. Mark as ready (외부 supervisor / restart endpoint 가 이후 처리).
	s.markOpCompleted(op, opStatusReadyToRestart)
	s.logger.Info("update.applied_pending_restart",
		slog.String("op", op.ID),
		slog.String("from_version", string(op.FromVersion)),
		slog.String("to_version", string(target)),
	)
}

// =============================================================================
// SystemHandler — HTTP endpoint dispatch
// =============================================================================

// SystemHandler 는 system 관련 (version, update) 엔드포인트를 처리한다.
type SystemHandler struct {
	svc    *UpdateService
	logger *slog.Logger
}

// NewSystemHandler 는 새 SystemHandler 를 생성한다.
func NewSystemHandler(svc *UpdateService, logger *slog.Logger) *SystemHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SystemHandler{
		svc:    svc,
		logger: logger,
	}
}

// RegisterRoutes 는 system 라우트를 그룹에 등록한다.
//
// Routes:
//
//	GET  /system/version
//	POST /system/update/check
//	POST /system/update/apply
//	POST /system/update/rollback
//	GET  /system/update/status
func (h *SystemHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/system/version", h.GetVersion)
	g.POST("/system/update/check", h.PostCheck)
	g.POST("/system/update/apply", h.PostApply)
	g.POST("/system/update/rollback", h.PostRollback)
	g.GET("/system/update/status", h.GetStatus)
}

// GetVersion 은 현재 바이너리 메타데이터 + 마지막 check 결과를 반환한다.
// GET /system/version
func (h *SystemHandler) GetVersion(ctx api.Context) error {
	resp := h.svc.Version()
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// PostCheck 는 채널에서 신규 버전을 조회한다.
// POST /system/update/check
func (h *SystemHandler) PostCheck(ctx api.Context) error {
	resp, err := h.svc.Check(ctx.Context())
	if err != nil {
		return mapUpdateError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// PostApply 는 비동기 update 작업을 시작한다.
// POST /system/update/apply
func (h *SystemHandler) PostApply(ctx api.Context) error {
	// Body 파싱 실패는 무시 (빈 body 도 허용 — latest 자동 사용).
	// io.EOF / "request body is empty" 는 정상 (요청자가 latest 를 위임한 경우).
	var req dto.ApplyRequest
	_ = ctx.Bind(&req)

	resp, err := h.svc.Apply(ctx.Context(), req.Version, req.Force)
	if err != nil {
		return mapUpdateError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// PostRollback 은 백업 바이너리를 복원한다.
// POST /system/update/rollback
func (h *SystemHandler) PostRollback(ctx api.Context) error {
	resp, err := h.svc.Rollback(ctx.Context())
	if err != nil {
		return mapUpdateError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// GetStatus 는 마지막 작업의 상태를 반환한다.
// GET /system/update/status
func (h *SystemHandler) GetStatus(ctx api.Context) error {
	resp := h.svc.Status()
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// mapUpdateError 는 updater 패키지의 sentinel error 를 APIError 로 매핑한다.
//
// SPEC M10 응답 코드 매핑:
//   - ErrUpdateInProgress           → 409 Conflict
//   - ErrUpdateRollbackFailed       → 409 Conflict (백업 부재)
//   - ErrDowngradeRequiresForce     → 400 Bad Request
//   - ErrUpdateChannelInvalid       → 400 Bad Request
//   - ErrUpdateInvalidInput         → 400 Bad Request
//   - ErrUpdateDownloadFailed       → 503 Service Unavailable (네트워크)
//   - ErrUpdateInsufficientDiskSpace→ 500 Internal Server Error
//   - ErrUpdateChecksumMismatch     → 500 Internal Server Error (보안)
//   - ErrUpdateSignatureInvalid     → 500 Internal Server Error (보안)
//   - ErrUpdateApplyFailed          → 500 Internal Server Error
//   - 그 외                          → 500 Internal Server Error
func mapUpdateError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, updater.ErrUpdateInProgress):
		return api.ErrConflict.WithMessage(err.Error())
	case errors.Is(err, updater.ErrUpdateRollbackFailed):
		return api.ErrConflict.WithMessage(err.Error())
	case errors.Is(err, updater.ErrDowngradeRequiresForce):
		return api.ErrBadRequest.WithMessage(err.Error())
	case errors.Is(err, updater.ErrUpdateChannelInvalid):
		return api.ErrBadRequest.WithMessage(err.Error())
	case errors.Is(err, updater.ErrUpdateInvalidInput):
		return api.ErrBadRequest.WithMessage(err.Error())
	case errors.Is(err, updater.ErrUpdateDownloadFailed):
		return api.ErrServiceUnavailable.WithMessage(err.Error())
	case errors.Is(err, updater.ErrUpdateInsufficientDiskSpace):
		return api.ErrInternalServer.WithMessage(err.Error())
	case errors.Is(err, updater.ErrUpdateChecksumMismatch),
		errors.Is(err, updater.ErrUpdateSignatureInvalid):
		return api.ErrInternalServer.WithMessage(err.Error())
	case errors.Is(err, updater.ErrUpdateApplyFailed):
		return api.ErrInternalServer.WithMessage(err.Error())
	default:
		return api.ErrInternalServer.WithMessage(err.Error())
	}
}
