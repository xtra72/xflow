// post_update.go 는 원격 자가 업데이트(버전 관리 Phase 2)의 "재시작 후 자동 검증 +
// 자동 롤백"을 구현한다.
//
// 흐름:
//  1. 업데이트 적용 후 재시작 시, 새 바이너리를 exec 하면서 환경변수 마커
//     (XFLOW_POST_UPDATE=1, XFLOW_UPDATE_EXPECTED_VERSION=vX)를 전달한다.
//  2. 부팅한 새 프로세스는 마커를 감지하면(isPostUpdateBoot), 서버가 뜨는 동안 로컬
//     /health 를 폴링(PostExecHealthCheck)한다.
//  3. 타임아웃 내 정상 응답하면 검증 통과(마커 제거). 실패하면 자동 롤백
//     (AutoRollback: .previous 백업 복원 후 이전 바이너리로 재-exec).
//
// 한계: 새 바이너리가 health 서버를 띄우기도 전에 즉시 크래시하면 이 프로세스(및 본
// 고루틴)도 함께 사라져 롤백이 불가하다 — 그 경우는 systemd 등 외부 supervisor 의
// 자동 재시작/롤백에 의존한다. 본 메커니즘은 "기동은 되나 비정상/행/버전불일치" 케이스를 다룬다.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/xtra/xflow/internal/updater"
)

const (
	envPostUpdate      = "XFLOW_POST_UPDATE"
	envExpectedVersion = "XFLOW_UPDATE_EXPECTED_VERSION"
)

// postUpdateEnv 는 재시작(exec) 시 자식 프로세스에 넘길 환경에 post-update 마커를
// 추가해 반환한다. base 는 보통 os.Environ() 이다.
func postUpdateEnv(base []string, expectedVersion string) []string {
	out := make([]string, 0, len(base)+2)
	out = append(out, base...)
	out = append(out, envPostUpdate+"=1")
	out = append(out, envExpectedVersion+"="+expectedVersion)
	return out
}

// isPostUpdateBoot 는 현재 프로세스가 업데이트 직후 부팅인지(마커 보유) 판정한다.
func isPostUpdateBoot() bool {
	return os.Getenv(envPostUpdate) == "1"
}

// runPostUpdateSelfCheck 는 post-update 부팅 시 로컬 헬스를 폴링하고, 실패하면 자동
// 롤백한다. 마커가 없으면 즉시 반환한다(일반 부팅). 본 함수는 별도 고루틴에서 호출한다
// (서버 리스닝과 병행 — WaitHealthy 가 서버 기동을 폴링으로 대기).
//
// AutoRollback 성공 시 프로세스 이미지가 교체되어 본 함수는 반환하지 않는다.
func runPostUpdateSelfCheck(ctx context.Context, port int, logger *slog.Logger) {
	if !isPostUpdateBoot() {
		return
	}
	expected := os.Getenv(envExpectedVersion)
	binaryPath, err := os.Executable()
	if err != nil {
		logger.Error("post-update: 실행 파일 경로 조회 실패 — 자가 검증 생략", "error", err)
		return
	}

	// 무인증 liveness 엔드포인트(/health)로 새 바이너리 기동을 확인한다. /api/v1/* 는
	// basic_auth 가 켜지면 401 이 되어 거짓 실패를 유발할 수 있으므로 사용하지 않는다.
	healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	orch := updater.NewRestartOrchestrator(updater.RestartOrchestratorParams{
		BinaryPath:     binaryPath,
		HealthEndpoint: healthURL,
		HealthTimeout:  60 * time.Second,
		HealthInterval: time.Second,
		Logger:         logger,
	})

	logger.Info("post-update: 자가 검증 시작", "expected_version", expected, "health", healthURL)
	res, hErr := orch.PostExecHealthCheck(ctx, updater.Version(expected))
	if hErr != nil {
		logger.Error("post-update: 헬스체크 실패 — 자동 롤백 시도", "error", hErr)
		if rbErr := orch.AutoRollback(ctx); rbErr != nil {
			// CanRollback=false(백업 없음) 또는 복원 실패 — 수동 개입 필요.
			logger.Error("post-update: 자동 롤백 실패 — 수동 개입 필요", "error", rbErr)
		}
		// AutoRollback 성공 시 이전 바이너리로 exec 되어 여기 도달하지 않는다.
		return
	}

	logger.Info("post-update: 자가 검증 통과",
		"version", string(res.Version), "attempts", res.Attempts)
	// 마커 제거 — 이후 자식 exec 로 전파되지 않도록 한다.
	_ = os.Unsetenv(envPostUpdate)
	_ = os.Unsetenv(envExpectedVersion)
}
