// post_update.go 는 원격 자가 업데이트(버전 관리 Phase 2)의 "재시작 후 자동 검증 +
// 자동 롤백"을 구현한다. 상태 파일 기반이라 systemd 등 외부 supervisor 의 재시작에도
// 부팅 실패를 누적 추적해, 새 바이너리가 health 도달 전 즉시 크래시하는 경우까지 다룬다.
//
// 흐름:
//  1. 업데이트 적용(바이너리 교체) 직후, <binary>.update-state 에 pending 상태를 기록한다
//     (markUpdatePending). 백업(.previous)은 Applier 가 이미 만들어 둔다.
//  2. 모든 부팅에서 runPostUpdateSelfCheck 가 상태 파일을 읽는다. pending 이면:
//     - boot_attempts 가 임계 초과면 → 검증 생략하고 즉시 자동 롤백(반복 크래시 차단).
//     - 아니면 boot_attempts 를 증가·영속한 뒤 로컬 /health 를 폴링(PostExecHealthCheck).
//     정상 → pending 해제(성공). 실패 → 자동 롤백.
//  3. 자동 롤백: AutoRollback 이 .previous 를 복원하고 이전 바이너리로 재-exec 한다.
//
// boot_attempts 를 부팅 시작 시점에 증가·영속하므로, 새 프로세스가 health 도달 전에
// 크래시해도 systemd 가 재시작하면 다음 부팅에서 카운트가 누적되어 결국 롤백된다.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/xtra/xflow/internal/updater"
)

// maxBootAttempts 는 health 도달 실패를 허용하는 부팅 횟수다. 이를 넘으면 자동 롤백한다.
const maxBootAttempts = 2

// updateState 는 <binary>.update-state 파일의 내용이다.
type updateState struct {
	Version      string `json:"version"`
	BootAttempts int    `json:"boot_attempts"`
	Pending      bool   `json:"pending"`
}

func stateFilePath(binaryPath string) string {
	return binaryPath + ".update-state"
}

// markUpdatePending 은 업데이트 적용 직후 호출되어, 다음 부팅이 검증 대상임을 표시한다.
func markUpdatePending(binaryPath, version string) error {
	return writeUpdateState(binaryPath, updateState{Version: version, BootAttempts: 0, Pending: true})
}

func writeUpdateState(binaryPath string, st updateState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return os.WriteFile(stateFilePath(binaryPath), data, 0o600)
}

// readUpdateState 는 상태 파일을 읽는다. 파일이 없거나 파싱 실패면 ok=false.
func readUpdateState(binaryPath string) (updateState, bool) {
	data, err := os.ReadFile(stateFilePath(binaryPath))
	if err != nil {
		return updateState{}, false
	}
	var st updateState
	if json.Unmarshal(data, &st) != nil {
		return updateState{}, false
	}
	return st, true
}

// clearUpdateState 는 상태 파일을 제거한다(검증 성공 또는 롤백 직전).
func clearUpdateState(binaryPath string) {
	_ = os.Remove(stateFilePath(binaryPath))
}

// bootAction 은 부팅 시점에 취할 동작이다.
type bootAction int

const (
	bootNone        bootAction = iota // pending 아님 — 일반 부팅
	bootHealthCheck                   // 헬스체크 후 성공/실패 판정
	bootRollback                      // 검증 생략, 즉시 롤백(반복 실패)
)

// decideBootAction 은 상태로부터 부팅 동작을 결정한다(순수 함수 — 테스트 용이).
func decideBootAction(st updateState, ok bool, maxAttempts int) bootAction {
	if !ok || !st.Pending {
		return bootNone
	}
	if st.BootAttempts >= maxAttempts {
		return bootRollback
	}
	return bootHealthCheck
}

// runPostUpdateSelfCheck 는 부팅 시 pending 업데이트를 검증하고 실패 시 자동 롤백한다.
// 일반 부팅(pending 없음)은 즉시 no-op. 별도 고루틴에서 호출한다(서버 리스닝과 병행 —
// WaitHealthy 가 서버 기동을 폴링 대기). 롤백 성공 시 프로세스 이미지가 교체되어 반환하지 않는다.
func runPostUpdateSelfCheck(ctx context.Context, port int, logger *slog.Logger) {
	binaryPath, err := os.Executable()
	if err != nil {
		logger.Error("post-update: 실행 파일 경로 조회 실패 — 자가 검증 생략", "error", err)
		return
	}
	st, ok := readUpdateState(binaryPath)
	switch decideBootAction(st, ok, maxBootAttempts) {
	case bootNone:
		return
	case bootRollback:
		logger.Error("post-update: 부팅 반복 실패 — 즉시 자동 롤백",
			"attempts", st.BootAttempts, "version", st.Version)
		rollback(ctx, binaryPath, port, logger)
		return
	case bootHealthCheck:
		// 증가 후 영속: health 도달 전 크래시해도 다음 부팅에서 누적된다.
		st.BootAttempts++
		if werr := writeUpdateState(binaryPath, st); werr != nil {
			logger.Warn("post-update: 부팅 카운터 기록 실패", "error", werr)
		}
	}

	orch := newBootOrchestrator(binaryPath, port, logger)
	logger.Info("post-update: 자가 검증 시작",
		"expected_version", st.Version, "attempt", st.BootAttempts)
	res, hErr := orch.PostExecHealthCheck(ctx, updater.Version(st.Version))
	if hErr != nil {
		logger.Error("post-update: 헬스체크 실패 — 자동 롤백", "error", hErr)
		rollback(ctx, binaryPath, port, logger)
		return
	}
	logger.Info("post-update: 자가 검증 통과", "version", string(res.Version), "attempts", res.Attempts)
	clearUpdateState(binaryPath)
}

// rollback 은 상태를 정리한 뒤 자동 롤백(.previous 복원 + 이전 바이너리 재-exec)을 수행한다.
// 상태를 먼저 지워 롤백된 이전 바이너리가 깨끗하게 부팅하도록 한다.
func rollback(ctx context.Context, binaryPath string, port int, logger *slog.Logger) {
	clearUpdateState(binaryPath)
	orch := newBootOrchestrator(binaryPath, port, logger)
	if rbErr := orch.AutoRollback(ctx); rbErr != nil {
		// 백업 없음(CanRollback=false) 또는 복원 실패 — 수동 개입 필요.
		logger.Error("post-update: 자동 롤백 실패 — 수동 개입 필요", "error", rbErr)
	}
}

// newBootOrchestrator 는 부팅 측 헬스체크/롤백용 orchestrator 를 만든다(무인증 /health 사용).
func newBootOrchestrator(binaryPath string, port int, logger *slog.Logger) *updater.RestartOrchestrator {
	return updater.NewRestartOrchestrator(updater.RestartOrchestratorParams{
		BinaryPath:     binaryPath,
		HealthEndpoint: fmt.Sprintf("http://127.0.0.1:%d/health", port),
		HealthTimeout:  60 * time.Second,
		HealthInterval: time.Second,
		Logger:         logger,
	})
}
