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

// @SPEC:SPEC-UPDATE-002 v0.1.0 (M9, M10, M14)
// targetWhitelist 는 ApplyRequest.Target 의 허용 값 셋이다.
// whitelist 외 값은 ErrUpdateInvalidInput (400) 으로 거부된다 (path traversal 방어).
//
// 각 target 은 cfg.BinaryPaths 매핑으로 binary path 가 해석된다.
// missing key 의 경우 cfg.BinaryPath 가 fallback (xflowd 만 운영하는 환경 호환).
const (
	// targetXflowd 는 데몬 (long-running). 기본값.
	targetXflowd = "xflowd"
	// targetXflowAgent 는 에이전트 (long-running daemon).
	targetXflowAgent = "xflow-agent"
	// targetXflowCLI 는 CLI 도구 (one-shot, drain/health-check 의미 없음).
	targetXflowCLI = "xflow"
)

// isValidTarget 는 target 이 whitelist 에 속하는지 검증한다 (M9).
// 빈 문자열은 caller 가 default 로 처리하므로 isValidTarget 호출 전에 normalize 해야 한다.
func isValidTarget(target string) bool {
	switch target {
	case targetXflowd, targetXflowAgent, targetXflowCLI:
		return true
	default:
		return false
	}
}

// resolveBinaryPath 는 target 에 매핑된 binary path 를 반환한다 (M10).
// BinaryPaths 가 설정되어 있고 target 이 매핑되어 있으면 해당 경로를 사용.
// 그 외엔 cfg.BinaryPath fallback (xflowd 만 운영하는 환경 호환).
func (s *UpdateService) resolveBinaryPath(target string) string {
	if s.cfg.BinaryPaths != nil {
		if p, ok := s.cfg.BinaryPaths[target]; ok && p != "" {
			return p
		}
	}
	return s.cfg.BinaryPath
}

// 작업 상태 enum (UpdateStatusResponse.Status 값과 매핑됨).
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M2, M4, M14)
// v0.1.0 9-state + v0.2.0 신규 2-state ("restarting", "health_checking") = 11-state.
// 신규 2-state 는 auto_restart=true 인 apply 흐름에서만 사용된다.
const (
	opStatusIdle           = "idle"
	opStatusStarting       = "starting"
	opStatusChecking       = "checking"
	opStatusDownloading    = "downloading"
	opStatusVerifying      = "verifying"
	opStatusApplying       = "applying"
	opStatusReadyToRestart = "ready_to_restart"
	// opStatusRestarting 은 auto_restart=true 인 apply 의 graceful drain → exec 진행 단계 (M2).
	opStatusRestarting = "restarting"
	// opStatusHealthChecking 은 새 바이너리 self-probe 진행 단계 (M4).
	opStatusHealthChecking = "health_checking"
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

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M11)
	// CompatibilityValidate 는 dependency manifest 의 호환성 검증을 수행한다.
	//
	// 운영 환경에서는 ManifestFetcher.FetchAndVerify + CompatibilityChecker.Validate 를 조합한다.
	// 테스트는 직접 결과 (nil 또는 ErrUpdateIncompatibleVersion) 를 반환하는 stub 으로 대체.
	//
	// nil 이면 dependency manifest 검증을 건너뛴다 (v0.1.0 backward 호환).
	// 운영 환경에서는 항상 non-nil 으로 주입되어야 한다 (M11 강제).
	CompatibilityValidate func(ctx context.Context, target string, version updater.Version) error
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
	// target 미지정 (default xflowd) 인 경우 기본값으로 사용된다.
	BinaryPath string

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M9, M10)
	// BinaryPaths 는 target 별 바이너리 경로 매핑이다.
	// 키는 whitelist 멤버 ("xflowd" / "xflow-agent" / "xflow").
	// 값은 절대 경로 (예: "/usr/local/bin/xflow-agent").
	// nil 또는 missing key 의 경우 BinaryPath 가 fallback 으로 사용된다 (xflowd 만 운영하는 환경의 호환).
	BinaryPaths map[string]string

	// PublicKey 는 Ed25519 공개키 (32 bytes). nil 이면 Apply 가 즉시 실패.
	PublicKey ed25519.PublicKey

	// CurrentVersion 은 현재 빌드 버전 ("v0.3.0" 등). 빈 문자열도 허용 (display 만).
	CurrentVersion string

	// Commit 은 빌드 시 git commit SHA.
	Commit string

	// BuildDate 은 빌드 시각.
	BuildDate string

	// BinaryName 은 asset 매칭에 사용 (대개 "xflowd").
	// target 별 binary name 은 ApplyRequest.Target 으로 동적으로 override 된다 (M9, M10).
	BinaryName string

	// Factories 는 Phase A-D 컴포넌트 생성 함수 셋. 빈 값이면 production 기본값 사용.
	Factories UpdateServiceFactories

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1, M2, M4)
	// RestartOrchestratorFactory 는 auto_restart=true 인 apply 흐름에서 호출된다.
	// 미설정 시 auto_restart 요청은 ErrUpdateRestartFailed 로 거부된다 (silent skip 방지).
	// 운영 환경: updater.NewRestartOrchestrator 주입.
	// 테스트: stub factory 주입 (실제 syscall.Exec 우회).
	RestartOrchestratorFactory func(params updater.RestartOrchestratorParams) RestartOrchestratorRunner

	// DrainSignal 은 RestartOrchestrator 에 전달될 graceful drain 트리거.
	// nil 이면 drain phase 가 즉시 통과 (운영 환경에서는 HTTP server.Shutdown 등을 wrap).
	DrainSignal updater.DrainSignal

	// HealthCheckEndpoint 는 새 바이너리 self-probe 의 URL.
	// 빈 문자열이면 기본값 "http://127.0.0.1/api/v1/system/version" 사용 (port 는 호출자가 적절히 설정).
	HealthCheckEndpoint string

	// HealthCheckInterval 은 self-probe 폴링 주기 (default 1초).
	HealthCheckInterval time.Duration

	// @SPEC:SPEC-WEB-007
	// Mode 는 remote_management.mode ("server" | "client" | "disabled").
	// Version() 응답의 self identity 표출에 사용된다. 빈 문자열도 허용 (display 만).
	Mode string

	// @SPEC:SPEC-WEB-007
	// StartedAt 은 프로세스 시작 시각이다. zero 가 아니면 Version() 응답의
	// UptimeSeconds 를 time.Since(StartedAt) 로 계산한다. zero 면 0 을 반환한다.
	StartedAt time.Time
}

// RestartOrchestratorRunner 는 handler 가 의존하는 orchestrator 의 최소 인터페이스이다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1)
// 실 구현은 *updater.RestartOrchestrator. 테스트는 mock 으로 대체 가능.
type RestartOrchestratorRunner interface {
	Orchestrate(ctx context.Context) (*updater.RestartResult, error)
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
	// @SPEC:SPEC-WEB-007
	// 락 불필요 값(self identity + uptime)은 RLock 진입 전에 미리 계산한다.
	// project memory(project_hvac_agent_lock_pattern.md): 락 보유 구간을 최소화하고
	// RWMutex 재귀/대기 위험을 줄이기 위해 cfg 읽기만 RLock 안에서 수행한다.
	// os.Hostname() 은 가벼운 syscall 이며 락과 무관하다.
	hostname, hostErr := os.Hostname()
	if hostErr != nil {
		// BuildDate 의 "unknown" 컨벤션을 따른다 (빈 문자열 대신 표식).
		hostname = "unknown"
	}
	osName := runtime.GOOS
	arch := runtime.GOARCH
	goVersion := runtime.Version()

	s.mu.RLock()
	defer s.mu.RUnlock()

	// UptimeSeconds: StartedAt 이 zero 가 아니면 경과 초, zero 면 0.
	var uptimeSeconds float64
	if !s.cfg.StartedAt.IsZero() {
		uptimeSeconds = time.Since(s.cfg.StartedAt).Seconds()
	}

	resp := dto.VersionResponse{
		Version:       s.cfg.CurrentVersion,
		Commit:        s.cfg.Commit,
		BuildDate:     s.cfg.BuildDate,
		GoVersion:     goVersion,
		Channel:       string(s.cfg.Config.Channel),
		OS:            osName,
		Arch:          arch,
		Hostname:      hostname,
		Mode:          s.cfg.Mode,
		UptimeSeconds: uptimeSeconds,
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

// GetChannel 은 현재 활성 채널과 선택 가능한 모든 채널 enum 을 반환한다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M6)
//
// 동시성: RLock 만 사용하므로 여러 GET 이 병렬 처리 가능.
// 진행 중 op 가 있어도 영향 없음 (read-only).
func (s *UpdateService) GetChannel() dto.ChannelInfoResponse {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return dto.ChannelInfoResponse{
		Current: string(s.cfg.Config.Channel),
		Available: []string{
			string(updater.ChannelStable),
			string(updater.ChannelBeta),
			string(updater.ChannelNightly),
		},
	}
}

// ChangeChannel 은 활성 채널을 변경하고 새 채널로 즉시 Check 를 실행한다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M7)
//
// 동작:
//  1. channel 값 검증 (stable / beta / nightly).
//  2. 잘못된 값이면 ErrUpdateChannelInvalid 반환 (400 Bad Request).
//  3. 기존 채널과 동일하면 no-op + 즉시 check 결과 반환 (Message: 동일 채널 안내).
//  4. 새 채널로 cfg.Channel 갱신 (mu.Lock 으로 직렬화).
//  5. 새 채널 기준 Check 실행 (실패해도 채널 변경은 유지, CheckResult=nil + 경고 message).
//
// 영구 저장 정책 (in-memory only):
//   - cfg 만 변경되며 yaml 파일은 손대지 않는다.
//   - 다음 daemon 시작 시 yaml 의 원래 값이 복원된다.
//   - 영구 변경은 `xflowd update channel <name>` CLI 또는 yaml 직접 수정 필요 (응답 Message 에 안내).
//
// 동시성: mu.Lock 으로 cfg.Channel 갱신을 보호하므로 race-free.
// Apply 와 같이 동시 op 차단은 하지 않는다 — channel 변경은 read-mostly 이며,
// 진행 중 apply op 가 사용하는 cfg.Channel 은 op 시작 시점에 capture 되어 영향 없음.
func (s *UpdateService) ChangeChannel(ctx context.Context, channel string) (dto.ChangeChannelResponse, error) {
	newCh := updater.Channel(channel)
	if !newCh.IsValid() {
		return dto.ChangeChannelResponse{}, fmt.Errorf("%w: %q (allowed: stable, beta, nightly)",
			updater.ErrUpdateChannelInvalid, channel)
	}

	// 락 짧게 잡고 previous capture + 갱신.
	s.mu.Lock()
	previous := s.cfg.Config.Channel
	sameChannel := previous == newCh
	if !sameChannel {
		s.cfg.Config.Channel = newCh
	}
	s.mu.Unlock()

	// 새 채널 기준 즉시 Check (실패해도 채널 변경은 유지).
	checkResult, checkErr := s.runCheckWithCurrentChannel(ctx)

	resp := dto.ChangeChannelResponse{
		Previous:    string(previous),
		Current:     string(newCh),
		CheckResult: checkResult,
	}

	switch {
	case sameChannel && checkErr == nil:
		resp.Message = "이미 동일한 채널입니다 (yaml 영구 저장은 'xflowd update channel " + channel + "' CLI 사용)"
	case sameChannel && checkErr != nil:
		resp.Message = "이미 동일한 채널입니다. check 실패: " + checkErr.Error()
	case !sameChannel && checkErr == nil:
		resp.Message = "채널 변경 완료 (yaml 영구 저장은 'xflowd update channel " + channel + "' CLI 사용)"
	default:
		// 채널 변경 성공 + check 실패
		resp.Message = "채널 변경 완료. 즉시 check 실패: " + checkErr.Error() +
			" (yaml 영구 저장은 'xflowd update channel " + channel + "' CLI 사용)"
	}

	s.logger.Info("update.channel_changed",
		slog.String("previous", string(previous)),
		slog.String("current", string(newCh)),
		slog.Bool("same", sameChannel),
		slog.Bool("check_ok", checkErr == nil),
	)

	return resp, nil
}

// runCheckWithCurrentChannel 은 현재 cfg.Channel 기준으로 Checker 를 새로 만들어 Check 를 실행한다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M7)
//
// 채널 변경 직후 호출되어 새 채널의 latest 정보를 응답에 포함시킨다.
// Check() 와 달리 op 상태는 갱신하지 않는다 (channel 변경은 op 가 아님).
// 실패 시 nil + error 를 반환하여 호출자가 graceful 처리.
func (s *UpdateService) runCheckWithCurrentChannel(ctx context.Context) (*dto.CheckResponse, error) {
	// cfg snapshot — race 방지.
	s.mu.RLock()
	updateCfg := s.cfg.Config
	factories := s.cfg.Factories
	binaryName := s.cfg.BinaryName
	currentVersion := s.cfg.CurrentVersion
	s.mu.RUnlock()

	checker, err := factories.NewChecker(updateCfg.UpdateURL, updateCfg.Channel)
	if err != nil {
		return nil, fmt.Errorf("create checker: %w", err)
	}

	current := updater.Version(currentVersion)
	result, err := factories.CheckerCheck(ctx, checker, current, runtime.GOOS, runtime.GOARCH, binaryName)
	if err != nil {
		return nil, fmt.Errorf("check: %w", err)
	}

	return &dto.CheckResponse{
		Current:     string(current),
		Latest:      string(result.Latest),
		Available:   result.Available,
		Channel:     string(updateCfg.Channel),
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
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1)
// autoRestart=true 면 applying 완료 직후 graceful drain → exec → self health check
// → (실패 시) 자동 rollback 흐름이 추가로 진행된다 (op.Status 가 restarting → health_checking 으로 전이).
// autoRestart=false (default) 는 v0.1.0 동작 그대로 (ready_to_restart 종료).
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M9, M10, M14)
// target 은 업데이트 대상 바이너리 ("xflowd" / "xflow-agent" / "xflow"). 빈 문자열이면 default "xflowd".
// whitelist 외 값은 즉시 ErrUpdateInvalidInput (400) 으로 거부 (path traversal 방어).
//
// 비동기 동작: 호출자 ctx 가 cancel 되어도 update 자체는 완료까지 진행되어야 하므로
// 내부적으로 분리된 백그라운드 context 를 사용한다 (HTTP request lifecycle 과 분리).
func (s *UpdateService) Apply(_ context.Context, requestedVersion string, force bool, autoRestart bool, target string) (dto.ApplyResponse, error) {
	current := updater.Version(s.cfg.CurrentVersion)

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M9, M14): target normalize + validation
	// 빈 문자열 → default "xflowd" (v0.1.0 backward 호환).
	// whitelist 외 값 → 즉시 거부 (op 생성 전, 서버 상태 오염 방지).
	if target == "" {
		target = targetXflowd
	}
	if !isValidTarget(target) {
		return dto.ApplyResponse{}, fmt.Errorf("%w: target %q not in whitelist (allowed: xflowd, xflow-agent, xflow)",
			updater.ErrUpdateInvalidInput, target)
	}

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
	go s.applyAsync(bgCtx, op, requestedVersion, force, autoRestart, target)

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
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M2, M4)
// 종료 상태: idle, completed, failed, ready_to_restart.
// 비종료 (= in progress): starting, checking, downloading, verifying, applying,
// restarting, health_checking.
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
// 단계 (auto_restart=false, v0.1.0 동작):
//
//	checking → downloading → verifying → applying → ready_to_restart
//
// 단계 (auto_restart=true, M1-M5):
//
//	checking → downloading → verifying → applying → restarting → (process replaced via exec)
//	                                                                ↓
//	                                                           (새 process 에서 health_checking)
//
// 각 단계 실패 시 op.Status = "failed" + Error 메시지 기록.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1, M2, M3, M9, M10, M11)
// autoRestart=true 인 경우 applying 완료 후 op.Status 를 "restarting" 으로 전이하고
// RestartOrchestrator 를 호출한다. Orchestrate() 가 정상 진행하면 syscall.Exec 으로
// 프로세스가 교체되어 본 goroutine 은 return 하지 않는다 (process image 가 새 바이너리).
// 실패 시 op.Status = "failed".
//
// target 은 업데이트 대상 바이너리 (whitelist 검증은 Apply 단계에서 완료됨).
// target 별로 binaryPath / binaryName / asset 매칭이 분기된다.
// xflow CLI (one-shot) 는 graceful drain / health check 가 무의미하므로 autoRestart 가 무시된다.
func (s *UpdateService) applyAsync(ctx context.Context, op *updateOperation, requestedVersion string, force bool, autoRestart bool, target string) {
	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M10): target 별 분기 결정.
	// xflow CLI 는 long-running daemon 이 아니므로 autoRestart 가 의미 없음 → 무시.
	if target == targetXflowCLI {
		autoRestart = false
	}
	targetBinaryPath := s.resolveBinaryPath(target)
	// asset 매칭에 사용되는 바이너리 이름은 target 으로 override 한다 (M10).
	// 운영 환경에서는 cfg.BinaryName 가 default ("xflowd") 이지만 target 으로 분기.
	targetBinaryShortName := target

	// 1. Checker 생성 + 채널 조회
	s.markOpStatus(op, opStatusChecking)

	checker, err := s.cfg.Factories.NewChecker(s.cfg.Config.UpdateURL, s.cfg.Config.Channel)
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("create checker: %w", err))
		return
	}

	current := op.FromVersion
	result, err := s.cfg.Factories.CheckerCheck(ctx, checker, current, runtime.GOOS, runtime.GOARCH, targetBinaryShortName)
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("check: %w", err))
		return
	}

	// requestedVersion 처리: 빈 문자열 → latest 사용. 명시 버전이면 다운그레이드 검사.
	targetVersion := result.Latest
	if requestedVersion != "" {
		targetVersion = updater.Version(requestedVersion)
	}
	s.markOpToVersion(op, targetVersion)

	// 다운그레이드 보호 (M8): force 없으면 거부.
	if targetVersion.Compare(current) < 0 && !force {
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

	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M11): dependency manifest 호환성 검증.
	// CompatibilityValidate factory 가 주입되어 있으면 호환성 위반 시 즉시 거부.
	// nil 이면 검증 단계 자체가 skip (v0.1.0 backward 호환 + 운영 환경에서는 항상 주입되어야 함).
	if s.cfg.Factories.CompatibilityValidate != nil {
		if err := s.cfg.Factories.CompatibilityValidate(ctx, target, targetVersion); err != nil {
			s.markOpFailed(op, fmt.Errorf("dependency manifest: %w", err))
			return
		}
	}

	// 2. Download
	s.markOpStatus(op, opStatusDownloading)

	tmpDir, err := os.MkdirTemp("", "xflow-update-*")
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("create tmp dir: %w", err))
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	binPath := filepath.Join(tmpDir, targetBinaryShortName+"-new")
	d := s.cfg.Factories.NewDownloader()
	if err := s.cfg.Factories.DownloaderDownload(ctx, d, *result.BinaryAsset, binPath, nil); err != nil {
		s.markOpFailed(op, fmt.Errorf("download binary: %w", err))
		return
	}

	assetBinaryName := updater.AssetName(targetBinaryShortName, runtime.GOOS, runtime.GOARCH)
	manifest, err := s.cfg.Factories.DownloaderDownloadManifest(
		ctx, d, *result.ChecksumAsset, *result.SignatureAsset, assetBinaryName, targetVersion,
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

	applier := s.cfg.Factories.NewApplier(verifier, targetBinaryPath)
	_, err = s.cfg.Factories.ApplierApply(ctx, applier, updater.ApplyOptions{
		Manifest:       manifest,
		DownloadedPath: binPath,
	})
	if err != nil {
		s.markOpFailed(op, fmt.Errorf("apply: %w", err))
		return
	}

	// 5a. autoRestart=false (v0.1.0 backward 호환): ready_to_restart 종료, 운영자 수동 재시작.
	if !autoRestart {
		s.markOpCompleted(op, opStatusReadyToRestart)
		s.logger.Info("update.applied_pending_restart",
			slog.String("op", op.ID),
			slog.String("from_version", string(op.FromVersion)),
			slog.String("to_version", string(targetVersion)),
			slog.String("target_binary", target),
		)
		return
	}

	// 5b. autoRestart=true (M1-M5): graceful drain → exec → self health check → 자동 rollback
	//
	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1, M2, M3)
	// op.Status 를 "restarting" 으로 전이한 뒤 RestartOrchestrator.Orchestrate() 호출.
	// Orchestrate() 정상 시 syscall.Exec 으로 프로세스가 교체되어 본 goroutine 은 return 하지 않음.
	// 실패 시 op.Status = "failed" 로 마킹.
	s.markOpStatus(op, opStatusRestarting)
	s.logger.Info("update.restart_starting",
		slog.String("op", op.ID),
		slog.String("target_version", string(targetVersion)),
		slog.String("target_binary", target),
	)

	if s.cfg.RestartOrchestratorFactory == nil {
		// 기본 운영 환경에서는 orchestrator factory 가 주입되어야 함.
		// 미설정 시 명시 실패 (silent skip 방지).
		s.markOpFailed(op, fmt.Errorf("%w: RestartOrchestratorFactory not configured",
			updater.ErrUpdateRestartFailed))
		return
	}

	orch := s.cfg.RestartOrchestratorFactory(updater.RestartOrchestratorParams{
		Verifier:       verifier,
		Applier:        applier,
		BinaryPath:     targetBinaryPath,
		DrainSignal:    s.cfg.DrainSignal,
		DrainTimeout:   s.cfg.Config.DrainTimeout,
		HealthEndpoint: s.cfg.HealthCheckEndpoint,
		HealthTimeout:  s.cfg.Config.HealthCheckTimeout,
		HealthInterval: s.cfg.HealthCheckInterval,
		DownloadedPath: binPath,
		Manifest:       manifest,
		Logger:         s.logger,
	})

	if _, err := orch.Orchestrate(ctx); err != nil {
		s.markOpFailed(op, fmt.Errorf("restart: %w", err))
		return
	}
	// 정상 시 unreachable: syscall.Exec 으로 프로세스 교체됨.
	// 도달 시 (테스트 또는 stub) 실패로 마킹.
	s.markOpFailed(op, fmt.Errorf("%w: orchestrator returned without exec",
		updater.ErrUpdateRestartFailed))
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
//	GET  /system/update/channel    (인증된 모든 사용자 — M6)
//	PUT  /system/update/channel    (admin 권한 필수 — M7)
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M6, M7)
// 채널 라우트는 핸들러 레벨에서 admin 권한을 확인한다 (PUT 만).
func (h *SystemHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/system/version", h.GetVersion)
	g.POST("/system/update/check", h.PostCheck)
	g.POST("/system/update/apply", h.PostApply)
	g.POST("/system/update/rollback", h.PostRollback)
	g.GET("/system/update/status", h.GetStatus)
	g.GET("/system/update/channel", h.GetChannel)
	g.PUT("/system/update/channel", h.PutChannel)
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
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1, M9)
// 요청 바디의 auto_restart / target 필드를 svc.Apply 에 전달한다 (default false / "xflowd").
// target 이 whitelist 외 값이면 svc.Apply 가 ErrUpdateInvalidInput 을 반환하고
// mapUpdateError 가 400 Bad Request 로 매핑한다.
func (h *SystemHandler) PostApply(ctx api.Context) error {
	// Body 파싱 실패는 무시 (빈 body 도 허용 — latest 자동 사용).
	// io.EOF / "request body is empty" 는 정상 (요청자가 latest 를 위임한 경우).
	var req dto.ApplyRequest
	_ = ctx.Bind(&req)

	resp, err := h.svc.Apply(ctx.Context(), req.Version, req.Force, req.AutoRestart, req.Target)
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

// GetChannel 은 현재 활성 채널과 선택 가능한 모든 채널 enum 을 반환한다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M6)
//
// GET /system/update/channel
//
// 인증된 모든 사용자 접근 가능 (admin 권한 불필요).
// 운영 환경에서 Auth 미들웨어가 인증을 검증하며, 본 핸들러는 추가 권한 체크를 하지 않는다.
func (h *SystemHandler) GetChannel(ctx api.Context) error {
	resp := h.svc.GetChannel()
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// PutChannel 은 활성 채널을 변경하고 새 채널로 즉시 Check 결과를 반환한다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M7)
//
// PUT /system/update/channel
// Body: { "channel": "stable|beta|nightly" }
// Auth: admin 권한 필수
//
// 응답:
//   - 200 OK: 채널 변경 성공 + check_result (CheckResult 가 nil 이면 check 실패).
//   - 400 Bad Request: 채널 값 invalid 또는 body 파싱 실패.
//   - 403 Forbidden: admin 아님.
//
// 영구 저장: in-memory 만 적용. 응답 message 에 CLI 안내 포함.
func (h *SystemHandler) PutChannel(ctx api.Context) error {
	// 1. admin 권한 체크 — handler-level guard.
	// Auth 미들웨어가 활성화되면 user_role 컨텍스트에 role 이 주입된다.
	// admin 아니면 403 (인증 자체는 Auth 미들웨어가 처리하므로 401 은 미들웨어 책임).
	if ctx.UserRole() != "admin" {
		return api.ErrForbidden.WithMessage("admin 권한이 필요합니다")
	}

	// 2. body 파싱.
	var req dto.ChangeChannelRequest
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}

	if req.Channel == "" {
		return api.ErrBadRequest.WithMessage("channel field is required")
	}

	// 3. service 호출.
	resp, err := h.svc.ChangeChannel(ctx.Context(), req.Channel)
	if err != nil {
		return mapUpdateError(err)
	}

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
	// @SPEC:SPEC-UPDATE-002 v0.1.0 (M3, M4, M13)
	// 신규 sentinel 매핑: ErrUpdateRestartFailed / ErrUpdateHealthCheckFailed → 500.
	// REST API 단계에서 발생할 수 있는 시점은 op.Status="failed" 의 Error 필드이므로
	// 본 함수가 호출되는 빈도는 낮지만 일관성 매핑.
	case errors.Is(err, updater.ErrUpdateRestartFailed),
		errors.Is(err, updater.ErrUpdateHealthCheckFailed):
		return api.ErrInternalServer.WithMessage(err.Error())
	default:
		return api.ErrInternalServer.WithMessage(err.Error())
	}
}
