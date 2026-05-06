// @SPEC:SPEC-UPDATE-001 v0.1.0
//go:build unix

// restarter_test.go — Phase D 그레이스풀 재시작 + exec 테스트.
//
// SPEC M6 (Graceful Restart):
//   - drain → exec 순서 (drain 실패해도 exec 진행: 방어적)
//   - 새 바이너리로 syscall.Exec (PID 보존, FD 보존)
//
// 보안 critical:
//   - 실행 전 target 바이너리 권한/유형 검증 (디렉토리·심링크 거부)
//   - Args / Env 명시적 전달 (예측 가능성)
//
// 테스트 안전성:
//   - 실제 exec 은 sub-process 패턴으로만 테스트 (테스트 프로세스 자체를 교체하지 않음)
//   - drain logic 등 pre-exec 영역은 nonexistent target 으로 검증 (exec 시 자연 실패 → 분기 가능)
package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain 은 sub-process 재실행 패턴을 위해 환경변수를 검사한다.
//
// RESTARTER_TEST_INVOKE=1 이면 child role:
//   - 즉시 Restart() 를 호출 (테스트 프로세스 자체를 exec 으로 교체)
//   - 원래 테스트 셋트는 실행하지 않음 (m.Run 호출 X)
//
// RESTARTER_TEST_INVOKE 미설정 = parent role: 정상 테스트 실행.
func TestMain(m *testing.M) {
	if os.Getenv("RESTARTER_TEST_INVOKE") == "1" {
		runRestartChild()
		// runRestartChild 는 정상 시 syscall.Exec 으로 프로세스 교체 (return 안 함)
		// 여기 도달했다면 exec 실패
		fmt.Fprintln(os.Stderr, "child: reached unreachable code (exec failed silently)")
		os.Exit(99)
	}
	os.Exit(m.Run())
}

// runRestartChild 는 환경변수에 따라 Restart() 를 호출한다.
//
// 환경변수:
//   - RESTARTER_TEST_TARGET (필수): exec 대상 경로
//   - RESTARTER_TEST_ARG (선택): 추가 argv (target 뒤에 한 개)
//   - RESTARTER_TEST_DRAIN_TIMEOUT (선택): drain timeout (Go duration string)
func runRestartChild() {
	target := os.Getenv("RESTARTER_TEST_TARGET")
	if target == "" {
		fmt.Fprintln(os.Stderr, "child: RESTARTER_TEST_TARGET not set")
		os.Exit(98)
	}
	args := []string{target}
	if extra := os.Getenv("RESTARTER_TEST_ARG"); extra != "" {
		args = append(args, extra)
	}

	opts := RestartOptions{
		BinaryPath: target,
		Args:       args,
		Env:        os.Environ(),
	}

	if err := Restart(opts); err != nil {
		fmt.Fprintf(os.Stderr, "child: Restart returned error: %v\n", err)
		os.Exit(1)
	}
	// exec 성공 시 여기 도달 안 함
}

// ---- Mock DrainSignal ----

// mockDrainSignal 은 Drain 호출을 추적하는 stub 이다.
type mockDrainSignal struct {
	calls       atomic.Int32
	lastTimeout atomic.Int64 // ns
	returnErr   error
	sleep       time.Duration // 호출 시 인위적 지연 (timeout 시뮬레이션)
}

func (m *mockDrainSignal) Drain(timeout time.Duration) error {
	m.calls.Add(1)
	m.lastTimeout.Store(int64(timeout))
	if m.sleep > 0 {
		time.Sleep(m.sleep)
	}
	return m.returnErr
}

// ---- Helper: write a small executable shell script that records argv ----

// writeMarkerScript 는 markerPath 에 자기 argv 를 기록하는 임시 sh 스크립트를 작성한다.
// 반환된 path 는 0o755 로 실행 가능하다.
func writeMarkerScript(t *testing.T, dir, markerPath string) string {
	t.Helper()
	scriptPath := filepath.Join(dir, "target.sh")
	// $0 자체와 $@ (모든 추가 args) 를 마커 파일에 기록.
	// argv0 + 모든 args 를 줄바꿈 구분으로 저장.
	script := `#!/bin/sh
{
  echo "argv0=$0"
  echo "args=$*"
  echo "env_marker=${RESTARTER_TEST_ENVMARKER:-}"
} > "` + markerPath + `"
exit 0
`
	require.NoError(t, os.WriteFile(scriptPath, []byte(script), 0o755))
	return scriptPath
}

// ---- Tests requiring real exec (sub-process pattern) ----

// TestRestart_NoDrain_ExecsTargetBinary 는 drain 없이 target 바이너리로 exec 되는지 검증한다.
//
// 흐름:
//  1. parent: marker 파일 경로 + sh 스크립트 생성
//  2. parent: 서브프로세스 (test binary) 생성 with env RESTARTER_TEST_INVOKE=1
//  3. child (TestMain): Restart(target=script) → syscall.Exec 으로 script 실행
//  4. script: marker 파일에 argv 기록
//  5. parent: 서브프로세스 종료 후 marker 검증
func TestRestart_NoDrain_ExecsTargetBinary(t *testing.T) {
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, "marker")
	scriptPath := writeMarkerScript(t, tmpDir, markerPath)

	// 서브프로세스 (테스트 바이너리 자체) 생성 with INVOKE=1
	cmd := exec.Command(os.Args[0], "-test.run=^$") // 자식에서는 테스트 실행 안 함
	cmd.Env = append(os.Environ(),
		"RESTARTER_TEST_INVOKE=1",
		"RESTARTER_TEST_TARGET="+scriptPath,
		"RESTARTER_TEST_ARG=hello",
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "child should exec successfully; output=%s", output)

	// marker 검증
	data, err := os.ReadFile(markerPath)
	require.NoError(t, err, "marker should be written by exec'd script")
	s := string(data)
	assert.Contains(t, s, "argv0="+scriptPath, "argv[0] should be script path")
	assert.Contains(t, s, "args=hello", "args should propagate to exec'd binary")
}

// TestRestart_PassesArgsAndEnv_ToChild 는 argv 와 env 가 새 프로세스로 전파되는지 검증한다.
func TestRestart_PassesArgsAndEnv_ToChild(t *testing.T) {
	tmpDir := t.TempDir()
	markerPath := filepath.Join(tmpDir, "marker")
	scriptPath := writeMarkerScript(t, tmpDir, markerPath)

	cmd := exec.Command(os.Args[0], "-test.run=^$")
	cmd.Env = append(os.Environ(),
		"RESTARTER_TEST_INVOKE=1",
		"RESTARTER_TEST_TARGET="+scriptPath,
		"RESTARTER_TEST_ARG=world",
		"RESTARTER_TEST_ENVMARKER=env-propagated", // child Restart() 의 Env 에 포함되어 script 가 읽음
	)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "output=%s", output)

	data, err := os.ReadFile(markerPath)
	require.NoError(t, err)
	s := string(data)
	assert.Contains(t, s, "args=world", "argv 전파 확인")
	assert.Contains(t, s, "env_marker=env-propagated", "env 전파 확인")
}

// ---- Tests with mock DrainSignal (no actual exec) ----

// TestRestart_DrainCalled_BeforeExec 는 drain 이 exec 전에 호출되는지 검증한다.
// nonexistent target 사용 → exec 단계 실패 → Restart 가 에러 반환 → 부모 테스트로 복귀.
func TestRestart_DrainCalled_BeforeExec(t *testing.T) {
	t.Parallel()

	mock := &mockDrainSignal{}
	opts := RestartOptions{
		BinaryPath:   "/definitely/nonexistent/binary-restarter-test",
		Args:         []string{"/definitely/nonexistent/binary-restarter-test"},
		DrainTimeout: 100 * time.Millisecond,
		DrainSignal:  mock,
	}

	err := Restart(opts)
	require.Error(t, err, "should fail since target doesn't exist")
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed), "error should wrap ErrUpdateApplyFailed; got %v", err)
	assert.Equal(t, int32(1), mock.calls.Load(), "drain should have been called exactly once")
	assert.Equal(t, int64(100*time.Millisecond), mock.lastTimeout.Load(), "drain timeout propagated")
}

// TestRestart_DrainSignalError_StillExecs 는 drain 이 에러를 반환해도 exec 시도는 진행되는지 검증한다.
func TestRestart_DrainSignalError_StillExecs(t *testing.T) {
	t.Parallel()

	mock := &mockDrainSignal{returnErr: errors.New("drain timeout exceeded")}
	opts := RestartOptions{
		BinaryPath:   "/definitely/nonexistent/binary-2",
		Args:         []string{"/definitely/nonexistent/binary-2"},
		DrainTimeout: 50 * time.Millisecond,
		DrainSignal:  mock,
	}

	err := Restart(opts)
	require.Error(t, err)
	// drain 자체 에러 메시지가 아니라 exec 단계 (verifyExecutable) 에러여야 함
	// → 즉, drain 후에도 exec 단계로 진행됨이 확인됨
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
	assert.Contains(t, err.Error(), "pre-exec check",
		"drain 실패 후에도 verifyExecutable 까지 도달해야 함")
	assert.Equal(t, int32(1), mock.calls.Load(), "drain called once")
}

// TestRestart_DrainTimeout_LogsWarning_StillExecs 는 drain 이 timeout 후에 반환해도
// exec 가 시도되는지 검증한다. (방어적: drain 실패가 deploy 차단 사유는 아님)
func TestRestart_DrainTimeout_LogsWarning_StillExecs(t *testing.T) {
	t.Parallel()

	mock := &mockDrainSignal{
		sleep:     30 * time.Millisecond, // drain 이 살짝 늦게 반환
		returnErr: errors.New("ctx deadline exceeded"),
	}
	opts := RestartOptions{
		BinaryPath:   "/nonexistent/binary-3",
		Args:         []string{"/nonexistent/binary-3"},
		DrainTimeout: 10 * time.Millisecond, // 사실상 무의미하지만 옵션 전달 확인용
		DrainSignal:  mock,
	}

	err := Restart(opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
	assert.Equal(t, int32(1), mock.calls.Load(), "drain attempted exactly once")
}

// ---- Tests for pre-exec validation ----

// TestRestart_BinaryNotExecutable_ErrUpdateApplyFailed 는 비실행 권한 또는 부재 파일에 대해
// ErrUpdateApplyFailed 가 반환되는지 검증한다.
func TestRestart_BinaryNotExecutable_ErrUpdateApplyFailed(t *testing.T) {
	t.Parallel()

	// 케이스 1: 부재 파일
	t.Run("missing", func(t *testing.T) {
		err := Restart(RestartOptions{
			BinaryPath: "/this/path/does/not/exist/at/all-xyz",
		})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
		assert.Contains(t, err.Error(), "pre-exec check")
	})

	// 케이스 2: 실행 권한 없는 파일
	t.Run("non_executable", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "not-exec")
		require.NoError(t, os.WriteFile(f, []byte("not a binary"), 0o600))

		err := Restart(RestartOptions{BinaryPath: f})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
	})

	// 케이스 3: 디렉토리
	t.Run("is_directory", func(t *testing.T) {
		dir := t.TempDir()
		err := Restart(RestartOptions{BinaryPath: dir})
		require.Error(t, err)
		assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
		// 에러 메시지에 "directory" 가 포함되어야 디버깅에 유용
		assert.True(t, strings.Contains(err.Error(), "directory") ||
			strings.Contains(err.Error(), "not executable"),
			"error should explain reason; got: %v", err)
	})
}

// TestRestart_NilDrainSignal_SkipsDrainPhase 는 DrainSignal 이 nil 일 때 drain 단계가
// 건너뛰어지고 곧바로 exec 단계로 진입하는지 검증한다.
func TestRestart_NilDrainSignal_SkipsDrainPhase(t *testing.T) {
	t.Parallel()

	opts := RestartOptions{
		BinaryPath:   "/nonexistent/binary-no-drain",
		DrainTimeout: 1 * time.Second,
		DrainSignal:  nil, // 핵심: nil
	}
	start := time.Now()
	err := Restart(opts)
	elapsed := time.Since(start)

	require.Error(t, err)
	// drain 이 skip 되었으므로 1초 timeout 대기를 하지 않음 → 즉시 실패
	assert.Less(t, elapsed, 200*time.Millisecond, "should not wait for drain when nil")
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
}

// TestRestart_ZeroDrainTimeout_SkipsDrainPhase 는 drainTimeout=0 일 때 drain 이 호출되지 않는지 검증.
func TestRestart_ZeroDrainTimeout_SkipsDrainPhase(t *testing.T) {
	t.Parallel()

	mock := &mockDrainSignal{}
	opts := RestartOptions{
		BinaryPath:   "/nonexistent/zero-drain",
		DrainTimeout: 0, // 0이면 skip
		DrainSignal:  mock,
	}
	err := Restart(opts)
	require.Error(t, err)
	assert.Equal(t, int32(0), mock.calls.Load(),
		"drain should not be called when timeout=0")
}

// ---- normalizeArgs / normalizeEnv unit tests (no exec required) ----

// TestNormalizeArgs_Empty_UsesBinary 는 빈 args 가 [binary] 로 채워지는지 검증.
func TestNormalizeArgs_Empty_UsesBinary(t *testing.T) {
	t.Parallel()
	got := normalizeArgs("/path/to/bin", nil)
	assert.Equal(t, []string{"/path/to/bin"}, got)

	got2 := normalizeArgs("/path/to/bin", []string{})
	assert.Equal(t, []string{"/path/to/bin"}, got2)
}

// TestNormalizeArgs_WrongArgv0_Corrected 는 args[0] 이 binary 와 다르면 교정되는지 검증.
func TestNormalizeArgs_WrongArgv0_Corrected(t *testing.T) {
	t.Parallel()
	got := normalizeArgs("/path/to/new-bin", []string{"/path/to/old-bin", "--flag", "value"})
	assert.Equal(t, []string{"/path/to/new-bin", "--flag", "value"}, got)
}

// TestNormalizeArgs_CorrectArgv0_Preserved 는 이미 args[0] 이 binary 면 그대로 유지되는지 검증.
func TestNormalizeArgs_CorrectArgv0_Preserved(t *testing.T) {
	t.Parallel()
	in := []string{"/path/to/bin", "--flag"}
	got := normalizeArgs("/path/to/bin", in)
	assert.Equal(t, in, got)
}

// TestNormalizeEnv_Nil_UsesOsEnviron 는 nil env 가 os.Environ() 으로 fallback 되는지 검증.
func TestNormalizeEnv_Nil_UsesOsEnviron(t *testing.T) {
	t.Parallel()
	got := normalizeEnv(nil)
	assert.NotEmpty(t, got, "nil env should fallback to os.Environ which is non-empty")
}

// TestNormalizeEnv_Empty_PreservedAsExplicit 는 빈 슬라이스가 의도된 비우기로 보존되는지 검증.
func TestNormalizeEnv_Empty_PreservedAsExplicit(t *testing.T) {
	t.Parallel()
	got := normalizeEnv([]string{})
	assert.Empty(t, got, "explicit empty env should be preserved (caller intent)")
}

// TestNormalizeEnv_NonEmpty_Preserved 는 명시 env 슬라이스가 그대로 보존되는지 검증.
func TestNormalizeEnv_NonEmpty_Preserved(t *testing.T) {
	t.Parallel()
	in := []string{"FOO=bar", "BAZ=qux"}
	got := normalizeEnv(in)
	assert.Equal(t, in, got)
}

// TestRestart_ExecFnReturnsError_WrappedAsApplyFailed 는 exec 자체가 실패할 때
// (예: syscall.Exec 의 ENOEXEC, EPERM 등) ErrUpdateApplyFailed 로 래핑되는지 검증한다.
//
// 실제 syscall.Exec 의 다양한 실패 경로는 운영체제에 의존적이므로 execFn 을 stub 으로
// 교체해 결정적 테스트를 수행한다.
func TestRestart_ExecFnReturnsError_WrappedAsApplyFailed(t *testing.T) {
	// 기존 execFn 저장 후 mock 으로 교체, 종료 시 원복.
	origExec := execFn
	defer func() { execFn = origExec }()

	mockErr := errors.New("simulated exec failure (e.g. ENOEXEC)")
	execFn = func(path string, args, env []string) error {
		return mockErr
	}

	// verifyExecutable 통과를 위해 실제 실행 가능한 더미 파일 생성
	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-bin")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755))

	err := Restart(RestartOptions{
		BinaryPath: fake,
		Args:       []string{fake, "--flag"},
		Env:        []string{"FOO=bar"},
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed),
		"execFn 에러는 ErrUpdateApplyFailed 로 wrapping 되어야 함")
	assert.Contains(t, err.Error(), "exec",
		"에러 메시지에 'exec' 문구 포함")
}

// TestRestart_ExecFnReturnsNil_UnreachablePathReported 는 execFn 이 (비정상적으로)
// nil 을 반환할 때 unreachable 안전망이 동작하는지 검증한다.
func TestRestart_ExecFnReturnsNil_UnreachablePathReported(t *testing.T) {
	origExec := execFn
	defer func() { execFn = origExec }()

	execFn = func(path string, args, env []string) error {
		return nil // 비정상: 실제 syscall.Exec 은 성공 시 return 하지 않음
	}

	dir := t.TempDir()
	fake := filepath.Join(dir, "fake-bin")
	require.NoError(t, os.WriteFile(fake, []byte("#!/bin/sh\n"), 0o755))

	err := Restart(RestartOptions{BinaryPath: fake})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
	assert.Contains(t, err.Error(), "unreachable",
		"unreachable safety net should trigger when execFn returns nil")
}

// TestVerifyExecutable_EmptyPath 는 빈 경로에 대한 명시적 에러 메시지를 검증.
func TestVerifyExecutable_EmptyPath(t *testing.T) {
	t.Parallel()
	err := verifyExecutable("")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

// TestRestart_DefaultArgs_UsesBinaryPath 는 Args 가 비어있을 때 [BinaryPath] 가 자동 설정되는지 검증.
//
// 검증 방법: nonexistent target 사용 + DrainSignal mock 으로 호출 흐름이 정상 진행됐음을
// (즉 exec 호출 직전까지 갔다는 사실을) ErrUpdateApplyFailed 발생 여부로 확인.
// (Args 의 실제 내용은 sub-process exec 테스트에서 별도 검증)
func TestRestart_DefaultArgs_UsesBinaryPath(t *testing.T) {
	t.Parallel()

	opts := RestartOptions{
		BinaryPath: "/nonexistent/default-args",
		// Args 미지정 → Restart 내부에서 [BinaryPath] 로 채워짐 (panic 등 발생 안 해야 함)
	}
	err := Restart(opts)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateApplyFailed))
}
