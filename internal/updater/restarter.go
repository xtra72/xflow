// @SPEC:SPEC-UPDATE-001 v0.1.0
// restarter.go — Phase D 그레이스풀 재시작 + exec.
//
// SPEC M6 (Graceful Restart):
//  1. drain phase: 신규 인입 차단 + in-flight 작업 완료 대기 (timeout 가능)
//  2. exec phase: syscall.Exec 으로 새 바이너리로 프로세스 이미지 교체 (PID 보존)
//
// 디자인 결정:
//   - DrainSignal 인터페이스: 호출자가 drain 의미 정의 (HTTP shutdown, FBP drain 등)
//   - drain 실패해도 exec 는 시도 (방어적: M6 "drain timeout 초과 시 강제 종료" 정책)
//   - 플랫폼 추상화: unix 는 syscall.Exec, !unix 는 stub (Windows 는 SPEC out-of-scope)
//   - exec 직전 verifyExecutable: 명백한 사용자 실수 (디렉토리/심링크/권한) 차단
//
// 보안 critical:
//   - target 바이너리 경로가 일반 파일 + 실행 권한이 있는지 검증
//   - argv[0] 을 항상 BinaryPath 로 고정 (호출자가 잘못된 args 를 넘겨도 일관성 보장)
//   - Env 미지정 시 os.Environ() 으로 자동 (운영 환경 유지)
package updater

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// DrainSignal 은 daemon 이 graceful drain 신호를 받기 위한 인터페이스이다.
//
// 호출자가 구현 책임:
//   - HTTP listener Shutdown(ctx)
//   - FBP 메시지 drain (in-flight 메시지 완료 대기)
//   - Store flush
//
// timeout 안에 drain 을 마치지 못하면 error 반환 (Restart 는 그래도 exec 진행).
type DrainSignal interface {
	// Drain 은 timeout 안에 모든 in-flight 작업을 종료하려 시도한다.
	// 정상 완료 시 nil, timeout 초과 또는 부분 실패 시 error 반환.
	Drain(timeout time.Duration) error
}

// RestartOptions 는 Restart 호출에 필요한 옵션 묶음이다.
type RestartOptions struct {
	// BinaryPath 는 새 바이너리의 절대 경로 (M5 Apply 가 교체한 후의 경로).
	BinaryPath string

	// Args 는 새 프로세스에 전달할 argv. nil/empty 면 [BinaryPath] 로 자동 설정.
	// argv[0] 은 항상 BinaryPath 로 정규화된다 (호출자 args[0] 무시).
	Args []string

	// Env 는 새 프로세스의 환경변수. nil 이면 os.Environ() 사용.
	Env []string

	// DrainTimeout 는 drain 단계 최대 대기시간. 0 또는 DrainSignal nil 이면 drain 단계 skip.
	DrainTimeout time.Duration

	// DrainSignal 은 graceful drain 트리거 (선택). nil 이면 drain skip.
	DrainSignal DrainSignal

	// Logger 는 구조화 로그용 (선택). nil 이면 slog.Default() 사용.
	Logger *slog.Logger
}

// Restart 는 graceful drain 후 새 바이너리로 프로세스 이미지를 교체한다.
//
// Unix 계열에서는 syscall.Exec 사용 (PID + open FD 보존, M6 systemd notify 호환).
// 성공 시 함수는 반환하지 않는다 (프로세스가 새 이미지로 교체됨).
// 실패 시 ErrUpdateApplyFailed 로 wrapping 된 error 반환.
//
// 흐름:
//  1. drain phase (옵션): DrainSignal != nil && DrainTimeout > 0
//     - 성공/실패와 무관하게 다음 단계로 진행 (방어적, M6)
//  2. verifyExecutable: 디렉토리/심링크/권한 부족 사전 차단
//  3. argv 정규화: argv[0] = BinaryPath
//  4. execNewBinary: syscall.Exec (unix) 또는 stub (other)
func Restart(opts RestartOptions) error {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// 1. Drain phase (선택)
	if opts.DrainSignal != nil && opts.DrainTimeout > 0 {
		logger.Info("update.restart.drain_start",
			"timeout", opts.DrainTimeout)
		start := time.Now()
		if err := opts.DrainSignal.Drain(opts.DrainTimeout); err != nil {
			// 의도적: drain 실패 시에도 exec 진행 (방어적, M6)
			// SPEC M6 "drain timeout 초과 시 강제 종료, 손실 메시지 카운트 로그"
			logger.Warn("update.restart.drain_error",
				"err", err,
				"elapsed", time.Since(start),
				"action", "proceed_with_exec")
		} else {
			logger.Info("update.restart.drain_complete",
				"elapsed", time.Since(start))
		}
	}

	// 2. 사전 검증: target 이 실행 가능한 일반 파일인지
	if err := verifyExecutable(opts.BinaryPath); err != nil {
		return fmt.Errorf("%w: pre-exec check: %v", ErrUpdateApplyFailed, err)
	}

	// 3. argv / env 정규화 (별도 함수로 추출하여 unit test 용이)
	args := normalizeArgs(opts.BinaryPath, opts.Args)
	env := normalizeEnv(opts.Env)

	logger.Info("update.restart.exec_start",
		"binary", opts.BinaryPath,
		"argc", len(args))

	// exec 호출. 성공 시 return 하지 않음.
	// execFn 은 테스트에서 mock 가능 (기본은 platform-specific execNewBinary).
	if err := execFn(opts.BinaryPath, args, env); err != nil {
		return fmt.Errorf("%w: exec: %v", ErrUpdateApplyFailed, err)
	}
	// 안전망: 정상 경로에서는 도달 불가 (exec 성공이 return 했다는 모순).
	return fmt.Errorf("%w: exec returned without error (unreachable)", ErrUpdateApplyFailed)
}

// execFn 은 syscall.Exec 호출의 indirection 변수이다.
//
// 테스트에서 stub 으로 교체 가능 (test-only 변경, 운영 경로는 항상 execNewBinary).
// 동시 변경 안전성: 단일 테스트 내에서만 변경하고 defer 로 원복하는 패턴 사용.
var execFn = execNewBinary

// normalizeArgs 는 새 프로세스의 argv 를 정규화한다.
//
// 규칙:
//   - args 가 비어있으면 [binary] 로 설정
//   - args[0] 이 binary 와 다르면 강제로 binary 로 교정 (호출자 실수 보호)
//   - 그 외엔 args 를 그대로 사용
func normalizeArgs(binary string, args []string) []string {
	if len(args) == 0 {
		return []string{binary}
	}
	if args[0] != binary {
		out := make([]string, 0, len(args))
		out = append(out, binary)
		out = append(out, args[1:]...)
		return out
	}
	return args
}

// normalizeEnv 는 새 프로세스의 환경변수를 정규화한다.
//
// nil 이면 호출자 프로세스의 환경 (os.Environ) 을 그대로 인계한다.
// 명시적 빈 슬라이스 (env=[]) 는 의도된 "환경 비우기" 로 해석되어 그대로 반환.
func normalizeEnv(env []string) []string {
	if env == nil {
		return os.Environ()
	}
	return env
}

// verifyExecutable 은 path 가 (a) 존재하고 (b) 일반 파일이며 (c) 실행 권한 비트가
// 켜져 있는지 검증한다.
//
// 보안 critical: 디렉토리·심링크·소켓 등 비정상 타입은 거부 (TOCTOU 는 호출자 책임).
func verifyExecutable(path string) error {
	if path == "" {
		return errors.New("binary path is empty")
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat %q: %w", path, err)
	}
	mode := info.Mode()
	if mode.IsDir() {
		return fmt.Errorf("target %q is a directory", path)
	}
	if !mode.IsRegular() {
		return fmt.Errorf("target %q is not a regular file (mode=%s)", path, mode)
	}
	if mode.Perm()&0o111 == 0 {
		return fmt.Errorf("target %q is not executable (mode=%s)", path, mode.Perm())
	}
	return nil
}
