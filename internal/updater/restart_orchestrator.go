// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1-M5)
// restart_orchestrator.go — In-Process Restart 오케스트레이터.
//
// SPEC M1-M5 (auto_restart 흐름):
//
//	applying 완료 → restarting (graceful drain) → exec → 새 프로세스 부팅
//	                                                       ↓
//	                                                   health_checking (self-probe)
//	                                                       ↓
//	                                          [success] completed
//	                                          [fail]    auto rollback → second probe
//	                                                       ↓
//	                                                   [success] failed (with rollback)
//	                                                   [fail]    failed (manual intervention)
//
// 보안 critical:
//   - reverifyDownloaded(): syscall.Exec 직전 SHA256 + Ed25519 재검증 (TOCTOU 방어)
//   - 단일 rollback 시도만 허용 (M5 loop 방지)
//   - drain timeout 강제 enforce (DrainSignal.Drain 의 timeout 인자)
//
// 동시성 디자인:
//   - RestartOrchestrator 는 stateless (인스턴스가 한 번 사용되고 버려짐)
//   - Orchestrate / PostExecHealthCheck / AutoRollback 은 서로 다른 프로세스 단계에서 호출됨
//     (현재 process: Orchestrate, 새 process: PostExecHealthCheck, 실패 시: AutoRollback)
//   - 따라서 별도 mutex 필요 없음 (각 호출은 단일 goroutine)
//
// 디자인 결정:
//   - syscall.Exec 자체는 v0.1.0 의 Restart() 를 재사용 (execFn indirection 활용)
//   - drain 실패는 호출자 (Orchestrate) 가 방어적 진행 결정 (M2 정책)
//   - Manifest 는 RestartOrchestrator 에 임베드 (재검증 시 일관성 보장)
package updater

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// RestartOrchestrator 는 M-1 In-Process Restart 흐름을 조율하는 컴포넌트이다.
//
// 단계별 호출 시퀀스 (process boundary 에 의해 분리됨):
//
//  1. (현재 프로세스) Orchestrate(ctx)
//     - reverifyDownloaded(): SHA256 + Ed25519 재검증
//     - gracefulDrain(ctx):   in-flight 요청/메시지 종료 대기
//     - execNewBinary:        syscall.Exec (정상 시 return 안 함)
//
//  2. (새 프로세스) PostExecHealthCheck(ctx, expectedVersion)
//     - HealthChecker.WaitHealthy(): localhost self-probe
//     - version 일치 검증
//
//  3. (새 프로세스, health check 실패 시) AutoRollback(ctx)
//     - Rollback.Restore(): .previous → main atomic rename
//     - syscall.Exec 으로 이전 바이너리 재실행 (loop 방지: 단일 시도)
//
// 모든 메서드는 race detector clean (단일 goroutine 호출 가정).
type RestartOrchestrator struct {
	// Verifier 는 SHA256 + Ed25519 검증을 수행한다 (TOCTOU 재검증).
	// nil 이면 reverifyDownloaded 가 즉시 실패.
	Verifier *Verifier

	// Applier 는 atomic 바이너리 교체를 수행한 컴포넌트이다.
	// 현재 binaryPath / backupPath 정보를 RestartOrchestrator 가 활용한다.
	Applier *Applier

	// Rollback 은 M5 자동 rollback 시 사용한다.
	Rollback *Rollback

	// HealthChecker 는 PostExecHealthCheck 단계에서 self-probe 수행.
	// nil 이면 PostExecHealthCheck 가 즉시 실패.
	HealthChecker *HealthChecker

	// DrainSignal 은 graceful drain 트리거 (선택). nil 이면 drain phase 가 즉시 통과.
	DrainSignal DrainSignal

	// DrainTimeout 은 drain 단계 최대 대기시간. 0 이면 default 30초.
	DrainTimeout time.Duration

	// Logger 는 구조화 로그용 (선택). nil 이면 slog.Default() 사용.
	Logger *slog.Logger

	// DownloadedPath 는 atomic replace 직전의 임시 파일 경로 (TOCTOU 재검증 대상).
	// applier 가 적용한 후에도 보존되어야 reverifyDownloaded 가 동작 가능.
	DownloadedPath string

	// Manifest 는 reverifyDownloaded 의 SHA256 + 서명 비교 기준이다.
	Manifest Manifest
}

// RestartResult 는 Orchestrate / PostExecHealthCheck / AutoRollback 의 결과를 표현한다.
//
// 정상 흐름에서 Orchestrate 는 syscall.Exec 으로 프로세스가 교체되어 return 하지 않는다.
// 따라서 이 구조체는 주로 (a) 단위 테스트, (b) 실패 경로의 진단 정보 전달에 사용된다.
type RestartResult struct {
	// Status 는 최종 단계 분류: "restarted" | "rolled_back" | "rollback_failed".
	Status string
	// NewVersion 은 적용 시도한 신규 버전.
	NewVersion Version
	// OldVersion 은 적용 전 실행 중이던 버전.
	OldVersion Version
	// HealthCheck 는 PostExecHealthCheck 의 결과 (해당 단계 호출된 경우).
	HealthCheck HealthResult
	// DrainStats 는 graceful drain 의 진행 통계.
	DrainStats DrainStats
}

// DrainStats 는 graceful drain 의 진행 시간/통계를 기록한다.
type DrainStats struct {
	// Started 는 drain phase 시작 시각 (UTC).
	Started time.Time
	// Completed 는 drain phase 종료 시각 (성공/타임아웃 무관).
	Completed time.Time
	// DroppedReqs 는 drain timeout 초과로 손실된 요청 수의 best-effort 추정치.
	// 0  = drain 정상 종료 (nil signal 또는 success)
	// -1 = unknown (DrainSignal 구현이 카운트를 노출하지 않음)
	// >0 = 실제 손실 카운트 (구현이 노출하는 경우, v0.1.0 에서는 미구현)
	DroppedReqs int
}

// Orchestrate 는 M-1 흐름의 첫 번째 단계 (현재 프로세스) 를 수행한다.
//
// 흐름:
//  1. reverifyDownloaded(): TOCTOU 재검증
//  2. gracefulDrain(ctx):   in-flight 작업 종료 대기 (drain 실패 시에도 진행)
//  3. Restart():            syscall.Exec (성공 시 return 하지 않음)
//
// 정상 시 Orchestrate 는 return 하지 않는다 (process image 가 교체됨).
// 실패 시 ErrUpdateRestartFailed wrapping error 반환.
//
// 주의: drain 실패는 critical 이 아니다 (M2 정책: "drain timeout 초과 시 강제 종료").
// 본 구현은 drain error 를 로그로만 남기고 exec 을 그대로 진행한다.
func (r *RestartOrchestrator) Orchestrate(ctx context.Context) (*RestartResult, error) {
	logger := r.logger()

	// 1. TOCTOU 재검증: 다운로드된 파일이 manifest 와 여전히 일치하는지
	if err := r.reverifyDownloaded(); err != nil {
		logger.Error("update.restart.reverify_failed",
			slog.String("err", err.Error()),
			slog.String("downloaded_path", r.DownloadedPath))
		return nil, err
	}

	// 2. Graceful drain. 실패해도 exec 진행 (M2 방어적 정책).
	drainStats, drainErr := r.gracefulDrain(ctx)
	if drainErr != nil {
		logger.Warn("update.restart.drain_error_proceeding",
			slog.String("err", drainErr.Error()),
			slog.Duration("drain_duration", drainStats.Completed.Sub(drainStats.Started)))
	}

	// 3. exec 직전 구조화 로그 (M3)
	logger.Info("update.exec",
		slog.String("target_version", string(r.Manifest.Version)),
		slog.String("binary_path", r.Applier.binaryPath),
		slog.Duration("drain_duration_ms", drainStats.Completed.Sub(drainStats.Started)))

	// 4. syscall.Exec (정상 시 return 안 함). drain 은 이미 완료했으므로 nil DrainSignal 전달.
	err := Restart(RestartOptions{
		BinaryPath:  r.Applier.binaryPath,
		DrainSignal: nil,
		Logger:      logger,
	})

	// exec 가 return 했다는 것은 실패를 의미.
	if err == nil {
		err = errors.New("exec returned without error (unreachable)")
	}
	return &RestartResult{
		Status:     "restart_failed",
		NewVersion: r.Manifest.Version,
		DrainStats: drainStats,
	}, fmt.Errorf("%w: exec phase: %v", ErrUpdateRestartFailed, err)
}

// PostExecHealthCheck 는 새 프로세스에서 호출되어 self readiness 를 검증한다.
//
// 흐름:
//  1. HealthChecker.WaitHealthy(): polling 루프 (timeout 까지)
//  2. 응답 version 이 expectedVersion 과 일치하는지 검사 (응답에 version 없으면 skip)
//
// 성공 시 (nil error, healthy=true) 호출자는 OperationStatus 를 "completed" 로 전이.
// 실패 시 ErrUpdateHealthCheckFailed wrapping error 반환 → 호출자는 AutoRollback 트리거.
//
// expectedVersion 이 빈 문자열이면 version 일치 검사를 skip (legacy mode).
func (r *RestartOrchestrator) PostExecHealthCheck(ctx context.Context, expectedVersion Version) (*HealthResult, error) {
	if r.HealthChecker == nil {
		return nil, fmt.Errorf("%w: HealthChecker is nil", ErrUpdateHealthCheckFailed)
	}

	logger := r.logger()
	result := r.HealthChecker.WaitHealthy(ctx)

	if !result.Healthy {
		logger.Error("update.health_check_failed",
			slog.Bool("healthy", false),
			slog.Int("attempts", result.Attempts),
			slog.Duration("latency", result.Latency))
		errMsg := "no healthy response within timeout"
		if result.LastError != nil {
			errMsg = result.LastError.Error()
		}
		return &result, fmt.Errorf("%w: %s", ErrUpdateHealthCheckFailed, errMsg)
	}

	// version 일치 검사 (응답에 version 이 있고 expectedVersion 이 명시된 경우만)
	if expectedVersion != "" && result.Version != "" && result.Version != expectedVersion {
		logger.Error("update.health_check_version_mismatch",
			slog.String("expected", string(expectedVersion)),
			slog.String("actual", string(result.Version)))
		return &result, fmt.Errorf("%w: version mismatch (expected %s, got %s)",
			ErrUpdateHealthCheckFailed, expectedVersion, result.Version)
	}

	logger.Info("update.health_check_passed",
		slog.String("version", string(result.Version)),
		slog.Int("attempts", result.Attempts),
		slog.Duration("latency", result.Latency))
	return &result, nil
}

// AutoRollback 은 M5 자동 rollback 을 수행한다.
//
// 흐름:
//  1. Rollback.CanRollback() 검사 → 백업 부재 시 ErrUpdateRollbackFailed
//  2. Rollback.Restore(): .previous → main atomic rename
//  3. syscall.Exec 으로 복원된 바이너리 재실행 (loop 방지: 단일 시도)
//
// 정상 시 syscall.Exec 으로 process 가 교체되어 return 하지 않는다.
// 실패 시 ErrUpdateRollbackFailed wrapping error 반환 (운영자 수동 개입 필요).
//
// SPEC M5 "두 번째 health check 도 실패 시 ErrUpdateRollbackFailed" 정책의 1차 단계.
// 두 번째 health check 자체는 새 (= 복원된) 프로세스에서 PostExecHealthCheck 로 다시 호출됨.
func (r *RestartOrchestrator) AutoRollback(ctx context.Context) error {
	if r.Rollback == nil {
		return fmt.Errorf("%w: Rollback component is nil", ErrUpdateRollbackFailed)
	}

	logger := r.logger()

	if !r.Rollback.CanRollback() {
		logger.Error("update.auto_rollback_no_backup",
			slog.String("backup_path", r.Rollback.backupPath))
		return fmt.Errorf("%w: no backup available at %s",
			ErrUpdateRollbackFailed, r.Rollback.backupPath)
	}

	// 백업 → main 복원
	restoreResult, err := r.Rollback.Restore(ctx)
	if err != nil {
		logger.Error("update.auto_rollback_restore_failed",
			slog.String("err", err.Error()))
		return fmt.Errorf("%w: restore step: %v", ErrUpdateRollbackFailed, err)
	}

	logger.Info("update.auto_rollback_restored",
		slog.String("from", restoreResult.RestoredFrom),
		slog.String("to", restoreResult.RestoredTo))

	// 복원된 바이너리로 재실행 (단일 시도, loop 방지)
	execErr := Restart(RestartOptions{
		BinaryPath:  r.Rollback.binaryPath,
		DrainSignal: nil, // 이미 종료 흐름 진입
		Logger:      logger,
	})
	if execErr == nil {
		execErr = errors.New("exec returned without error (unreachable)")
	}
	logger.Error("update.auto_rollback_exec_failed",
		slog.String("err", execErr.Error()))
	return fmt.Errorf("%w: post-rollback exec: %v", ErrUpdateRollbackFailed, execErr)
}

// reverifyDownloaded 는 syscall.Exec 직전 SHA256 + Ed25519 재검증을 수행한다.
//
// SPEC M4 (TOCTOU 방어): 다운로드 직후 1차 검증 → 디스크 휴면 → 여기서 2차 검증.
// 두 시점 사이에 디스크 변조가 있으면 ErrUpdateRestartFailed 로 거부된다.
//
// Verifier 가 nil 인 경우 ErrUpdateRestartFailed 반환 (안전 기본값).
func (r *RestartOrchestrator) reverifyDownloaded() error {
	if r.Verifier == nil {
		return fmt.Errorf("%w: Verifier is nil (cannot re-verify)", ErrUpdateRestartFailed)
	}
	if r.DownloadedPath == "" {
		return fmt.Errorf("%w: DownloadedPath is empty", ErrUpdateRestartFailed)
	}

	content, err := os.ReadFile(r.DownloadedPath)
	if err != nil {
		return fmt.Errorf("%w: read downloaded file: %v", ErrUpdateRestartFailed, err)
	}

	if err := r.Verifier.VerifyAll(content, r.Manifest.SHA256, r.Manifest.Signature); err != nil {
		return fmt.Errorf("%w: pre-exec re-verification: %v", ErrUpdateRestartFailed, err)
	}
	return nil
}

// RestartOrchestratorParams 는 RestartOrchestrator 를 운영 환경에서 생성하기 위한 입력 묶음이다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1)
// handler 가 applying 완료 후 orchestrator 를 빌드할 때 사용한다.
// 모든 필드는 명시적 (zero-value 도 허용되지만 의미 명확).
type RestartOrchestratorParams struct {
	// Verifier 는 TOCTOU 재검증용 (M3).
	Verifier *Verifier
	// Applier 는 이미 적용된 applier 인스턴스 (binaryPath/backupPath 정보 활용).
	Applier *Applier
	// BinaryPath 는 현재 실행 중 (=교체된 후) 바이너리 경로.
	BinaryPath string
	// DrainSignal 은 graceful drain 트리거 (선택).
	DrainSignal DrainSignal
	// DrainTimeout 은 drain 단계 최대 대기. 0 이면 default.
	DrainTimeout time.Duration
	// HealthEndpoint 는 self-probe URL (예: http://127.0.0.1:8080/api/v1/system/version).
	HealthEndpoint string
	// HealthTimeout 은 health check 폴링 윈도우. 0 이면 default 30초.
	HealthTimeout time.Duration
	// HealthInterval 은 health check 폴링 주기. 0 이면 default 1초.
	HealthInterval time.Duration
	// DownloadedPath 는 atomic replace 직전의 임시 파일 경로 (재검증 대상).
	DownloadedPath string
	// Manifest 는 SHA256 + 서명 비교 기준.
	Manifest Manifest
	// Logger 는 구조화 로그용. nil 이면 default.
	Logger *slog.Logger
}

// NewRestartOrchestrator 는 params 로부터 운영 환경용 RestartOrchestrator 인스턴스를 생성한다.
//
// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1)
// 본 함수는 default factory 로 사용된다 (handler 의 RestartOrchestratorFactory 미설정 시 fallback).
// 테스트는 자체 mock factory 를 주입하여 본 함수를 우회한다.
func NewRestartOrchestrator(params RestartOrchestratorParams) *RestartOrchestrator {
	return &RestartOrchestrator{
		Verifier:       params.Verifier,
		Applier:        params.Applier,
		Rollback:       NewRollback(params.BinaryPath),
		HealthChecker:  &HealthChecker{Endpoint: params.HealthEndpoint, Timeout: params.HealthTimeout, Interval: params.HealthInterval},
		DrainSignal:    params.DrainSignal,
		DrainTimeout:   params.DrainTimeout,
		Logger:         params.Logger,
		DownloadedPath: params.DownloadedPath,
		Manifest:       params.Manifest,
	}
}

// gracefulDrain 은 DrainSignal 호출 + timeout 강제 enforce 를 담당한다.
//
// 흐름:
//   - DrainSignal == nil → 즉시 통과 (no-op, DroppedReqs=0)
//   - DrainSignal != nil → Drain(timeout) 호출
//
// timeout 의 default 는 30 초 (DrainTimeout==0 인 경우).
// drain error 는 호출자에게 전달되며, Orchestrate 가 방어적 진행을 결정한다 (M2 정책).
//
// DroppedReqs 는 best-effort 통계:
//   - 0  = drain 정상 종료
//   - -1 = drain timeout/error (실제 카운트는 DrainSignal 구현이 노출하지 않음)
func (r *RestartOrchestrator) gracefulDrain(ctx context.Context) (DrainStats, error) {
	stats := DrainStats{Started: time.Now().UTC()}

	if r.DrainSignal == nil {
		stats.Completed = stats.Started
		return stats, nil
	}

	timeout := r.DrainTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	// ctx 가 이미 취소되었으면 즉시 fail-fast
	if err := ctx.Err(); err != nil {
		stats.Completed = time.Now().UTC()
		stats.DroppedReqs = -1
		return stats, fmt.Errorf("drain ctx cancelled: %w", err)
	}

	err := r.DrainSignal.Drain(timeout)
	stats.Completed = time.Now().UTC()
	if err != nil {
		stats.DroppedReqs = -1
		return stats, err
	}
	return stats, nil
}

// logger 는 nil-safe logger 접근자.
func (r *RestartOrchestrator) logger() *slog.Logger {
	if r.Logger == nil {
		return slog.Default()
	}
	return r.Logger
}
