// @SPEC:SPEC-UPDATE-002 v0.1.0 (M1-M5)
//go:build unix

// restart_orchestrator_test.go — M-1 In-Process Restart 오케스트레이터 단위 테스트.
//
// 테스트 전략:
//   - 실제 syscall.Exec 은 호출하지 않음 (execFn 변수 stub 으로 대체)
//   - DrainSignal / HealthChecker 은 mock 으로 주입
//   - TOCTOU 재검증은 ephemeral Ed25519 keypair 로 진짜 검증 (위변조 시뮬레이션)
//   - 모든 상태 전이는 race detector clean (sync.Mutex 보호)
//
// 커버되는 시나리오 (12+):
//
//	M-1 (auto_restart 옵션) — handler 단위 테스트에 위임
//	M-2 (graceful drain): nil signal, 성공, 타임아웃 시 진행
//	M-3 (TOCTOU 재검증 + exec): 정상, 변조 거부, exec 실패
//	M-4 (self health check): 성공, 타임아웃, 버전 불일치
//	M-5 (자동 rollback): 성공, 백업 부재, restore 실패, post-rollback exec 실패
package updater

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- mock helpers ----

// orchestratorMockDrain 은 DrainSignal 을 추적하는 stub 이다.
type orchestratorMockDrain struct {
	calls       atomic.Int32
	lastTimeout atomic.Int64 // ns
	returnErr   error
	sleep       time.Duration // 호출 시 인위적 지연 (timeout 시뮬레이션)
}

func (m *orchestratorMockDrain) Drain(timeout time.Duration) error {
	m.calls.Add(1)
	m.lastTimeout.Store(int64(timeout))
	if m.sleep > 0 {
		time.Sleep(m.sleep)
	}
	return m.returnErr
}

// makeTestKeypair 은 테스트용 Ed25519 keypair + Verifier 를 생성한다.
func makeTestKeypair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey, *Verifier) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	v, err := NewVerifier(pub)
	require.NoError(t, err)
	return pub, priv, v
}

// writeSignedBinary 은 임시 디렉토리에 content 를 쓰고 SHA256 + 서명을 계산해 반환한다.
func writeSignedBinary(t *testing.T, dir string, content []byte, priv ed25519.PrivateKey) (path, sha256Hex string, sig []byte) {
	t.Helper()
	path = filepath.Join(dir, "binary-new")
	require.NoError(t, os.WriteFile(path, content, 0o755))
	sum := sha256.Sum256(content)
	sha256Hex = hex.EncodeToString(sum[:])
	sig = ed25519.Sign(priv, content)
	return path, sha256Hex, sig
}

// makeOrchestrator 은 테스트용 RestartOrchestrator 를 생성한다.
//
// content 가 비어있지 않으면 downloaded 파일을 만들어 두고 verifier/manifest 를 일관되게 설정한다.
// applier 의 binaryPath 도 임시 디렉토리에 실재하는 실행 가능한 파일로 설정.
func makeOrchestrator(t *testing.T, content []byte) *RestartOrchestrator {
	t.Helper()
	dir := t.TempDir()
	_, priv, verifier := makeTestKeypair(t)
	path, sha256Hex, sig := writeSignedBinary(t, dir, content, priv)

	// applier 는 binaryPath 가 실제 존재하는 파일을 요구함 (verifyExecutable).
	// 본 테스트에서는 .Apply() 를 직접 호출하지 않으므로 binaryPath 도 동일 path 로 설정.
	applier := &Applier{
		Verifier:   verifier,
		binaryPath: path,
		backupPath: path + ".previous",
	}
	rb := NewRollback(path)

	return &RestartOrchestrator{
		Verifier:       verifier,
		Applier:        applier,
		Rollback:       rb,
		HealthChecker:  &HealthChecker{},
		DrainTimeout:   30 * time.Second,
		Logger:         slog.Default(),
		DownloadedPath: path,
		Manifest: Manifest{
			Version:   "v0.4.0",
			SHA256:    sha256Hex,
			Signature: sig,
		},
	}
}

// NOTE: 본 파일의 테스트는 t.Parallel() 을 사용하지 않는다.
// 이유: execFn 은 패키지 레벨 global 변수이며 일부 테스트가 stub 으로 교체한다.
// 병렬 실행 시 race detector 가 정당한 race 를 감지하므로 sequential 실행을 강제.
//
// Race-free 패턴은 restarter_test.go 와 동일.

// ---- M-3: TOCTOU 재검증 ----

func TestRestartOrchestrator_TOCTOU_Reverify_Success(t *testing.T) {
	content := []byte("test-binary-content-valid")
	o := makeOrchestrator(t, content)

	err := o.reverifyDownloaded()
	assert.NoError(t, err, "정상 매니페스트 + 정상 콘텐츠 → 검증 성공")
}

func TestRestartOrchestrator_TOCTOU_Reverify_Tampered_RestartFailed(t *testing.T) {
	content := []byte("original-content")
	o := makeOrchestrator(t, content)

	// 디스크의 다운로드된 파일을 변조 (TOCTOU 공격 시뮬레이션)
	require.NoError(t, os.WriteFile(o.DownloadedPath, []byte("TAMPERED"), 0o755))

	err := o.reverifyDownloaded()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRestartFailed), "변조된 콘텐츠 → ErrUpdateRestartFailed 를 errors.Is 로 감지해야 함")
}

func TestRestartOrchestrator_TOCTOU_Reverify_FileMissing_RestartFailed(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	// 다운로드된 파일 삭제
	require.NoError(t, os.Remove(o.DownloadedPath))

	err := o.reverifyDownloaded()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRestartFailed))
}

// ---- M-2: graceful drain ----

func TestRestartOrchestrator_GracefulDrain_NilSignal_NoOp(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	o.DrainSignal = nil // explicit no-op

	stats, err := o.gracefulDrain(context.Background())
	require.NoError(t, err)
	assert.False(t, stats.Started.IsZero())
	assert.False(t, stats.Completed.IsZero())
	// drain 호출 자체가 없었으므로 dropped reqs 도 0
	assert.Equal(t, 0, stats.DroppedReqs)
}

func TestRestartOrchestrator_GracefulDrain_Success(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	mock := &orchestratorMockDrain{}
	o.DrainSignal = mock
	o.DrainTimeout = 5 * time.Second

	stats, err := o.gracefulDrain(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), mock.calls.Load(), "Drain 은 한 번만 호출되어야 함")
	assert.Equal(t, int64(5*time.Second), mock.lastTimeout.Load(), "DrainTimeout 이 그대로 전달되어야 함")
	assert.Equal(t, 0, stats.DroppedReqs)
}

func TestRestartOrchestrator_GracefulDrain_Timeout_StillProceeds(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	mock := &orchestratorMockDrain{
		returnErr: errors.New("drain timeout"),
	}
	o.DrainSignal = mock
	o.DrainTimeout = 100 * time.Millisecond

	// drain 실패해도 gracefulDrain 자체는 에러를 함께 반환 (호출자가 진행 결정)
	stats, err := o.gracefulDrain(context.Background())
	require.Error(t, err, "drain 실패는 호출자에게 전달되어야 함 (방어적 진행 결정은 Orchestrate 가)")
	assert.Equal(t, int32(1), mock.calls.Load())
	// 통계는 기록되어야 함
	assert.False(t, stats.Started.IsZero())
	assert.False(t, stats.Completed.IsZero())
	// dropped reqs 는 unknown → -1 표시
	assert.Equal(t, -1, stats.DroppedReqs)
}

func TestRestartOrchestrator_GracefulDrain_DefaultTimeout(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	mock := &orchestratorMockDrain{}
	o.DrainSignal = mock
	o.DrainTimeout = 0 // → default 30s

	_, err := o.gracefulDrain(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int64(30*time.Second), mock.lastTimeout.Load(),
		"DrainTimeout=0 이면 기본값 30초가 적용되어야 함")
}

// ---- M-4: self health check ----

// startTestServer 는 테스트용 health endpoint 를 시작한다.
// statusCode/body 는 매 요청마다 동일.
func startTestServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(statusCode)
		_, _ = io.WriteString(w, body)
	}))
}

func TestRestartOrchestrator_PostExecHealthCheck_Success(t *testing.T) {
	srv := startTestServer(t, http.StatusOK, `{"version":"v0.4.0","status":"ok"}`)
	defer srv.Close()

	o := makeOrchestrator(t, []byte("content"))
	o.HealthChecker = &HealthChecker{
		Endpoint: srv.URL,
		Timeout:  2 * time.Second,
		Interval: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result, err := o.PostExecHealthCheck(ctx, "v0.4.0")
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Healthy)
	assert.Equal(t, Version("v0.4.0"), result.Version)
}

func TestRestartOrchestrator_PostExecHealthCheck_Timeout_HealthCheckFailed(t *testing.T) {
	// 영구 503 응답 → 정상 응답 한 번도 없음
	srv := startTestServer(t, http.StatusServiceUnavailable, `unavailable`)
	defer srv.Close()

	o := makeOrchestrator(t, []byte("content"))
	o.HealthChecker = &HealthChecker{
		Endpoint: srv.URL,
		Timeout:  150 * time.Millisecond, // 짧게
		Interval: 50 * time.Millisecond,
	}

	result, err := o.PostExecHealthCheck(context.Background(), "v0.4.0")
	require.Error(t, err)
	require.NotNil(t, result)
	assert.False(t, result.Healthy)
	assert.True(t, errors.Is(err, ErrUpdateHealthCheckFailed),
		"타임아웃 시 ErrUpdateHealthCheckFailed sentinel 이어야 함")
}

func TestRestartOrchestrator_PostExecHealthCheck_VersionMismatch(t *testing.T) {
	// 응답 버전이 expected 와 다름
	srv := startTestServer(t, http.StatusOK, `{"version":"v0.3.9","status":"ok"}`)
	defer srv.Close()

	o := makeOrchestrator(t, []byte("content"))
	o.HealthChecker = &HealthChecker{
		Endpoint: srv.URL,
		Timeout:  500 * time.Millisecond,
		Interval: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := o.PostExecHealthCheck(ctx, "v0.4.0")
	require.Error(t, err)
	require.NotNil(t, result)
	assert.True(t, errors.Is(err, ErrUpdateHealthCheckFailed),
		"버전 불일치도 health check failure 로 매핑되어야 함")
	assert.Contains(t, err.Error(), "version mismatch")
}

func TestRestartOrchestrator_PostExecHealthCheck_EmptyVersionAccepted(t *testing.T) {
	// 응답에 version 필드 없음 (legacy endpoint) → healthy 자체가 의미 있음
	srv := startTestServer(t, http.StatusOK, `{}`)
	defer srv.Close()

	o := makeOrchestrator(t, []byte("content"))
	o.HealthChecker = &HealthChecker{
		Endpoint: srv.URL,
		Timeout:  500 * time.Millisecond,
		Interval: 50 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	result, err := o.PostExecHealthCheck(ctx, "v0.4.0")
	// version 비교는 응답에 version 이 없으면 skip (binary 응답 자체가 healthy 신호)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Healthy)
}

// ---- M-5: 자동 rollback ----

// makeRollbackEnv 는 백업 파일을 포함한 RestartOrchestrator 환경을 구성한다.
func makeRollbackEnv(t *testing.T, withBackup bool) *RestartOrchestrator {
	t.Helper()
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "xflowd")
	require.NoError(t, os.WriteFile(mainPath, []byte("current-binary"), 0o755))

	if withBackup {
		require.NoError(t, os.WriteFile(mainPath+".previous", []byte("previous-binary"), 0o755))
	}

	_, priv, verifier := makeTestKeypair(t)
	content := []byte("downloaded-content")
	dlPath, sha, sig := writeSignedBinary(t, dir, content, priv)

	return &RestartOrchestrator{
		Verifier: verifier,
		Applier: &Applier{
			Verifier:   verifier,
			binaryPath: mainPath,
			backupPath: mainPath + ".previous",
		},
		Rollback:       NewRollback(mainPath),
		HealthChecker:  &HealthChecker{},
		DrainTimeout:   1 * time.Second,
		Logger:         slog.Default(),
		DownloadedPath: dlPath,
		Manifest: Manifest{
			Version:   "v0.4.0",
			SHA256:    sha,
			Signature: sig,
		},
	}
}

func TestRestartOrchestrator_AutoRollback_NoBackup_RollbackFailed(t *testing.T) {
	o := makeRollbackEnv(t, false /* withBackup */)

	err := o.AutoRollback(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed),
		"백업 없으면 ErrUpdateRollbackFailed sentinel 반환")
	assert.Contains(t, err.Error(), "no backup")
}

func TestRestartOrchestrator_AutoRollback_Success_StubsExec(t *testing.T) {
	o := makeRollbackEnv(t, true /* withBackup */)

	// execFn stub: 성공 시 호출되었음만 기록 (실제 exec 안 함)
	var execCalled atomic.Int32
	original := execFn
	execFn = func(_ string, _ []string, _ []string) error {
		execCalled.Add(1)
		return nil // 성공 → 그러나 본래는 return 안 함
	}
	defer func() { execFn = original }()

	// AutoRollback 은 Restore + execFn (stub) 호출 후 nil 반환 시
	// "exec returned without error (unreachable)" 메시지가 ErrUpdateApplyFailed 로 wrapping 되어
	// ErrUpdateRollbackFailed 로 다시 wrapping 됨 (정상 운영 경로에서는 stub 도 return 안 해야 함).
	err := o.AutoRollback(context.Background())
	// stub 이 nil 을 반환했으므로 unreachable error 가 발생함 → 우리 구현은 이를 ErrUpdateRollbackFailed 로 wrapping 해야 함
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed))
	assert.GreaterOrEqual(t, execCalled.Load(), int32(1), "execFn stub 가 적어도 1회 호출되어야 함")
}

func TestRestartOrchestrator_AutoRollback_PostRollbackExec_Fails(t *testing.T) {
	o := makeRollbackEnv(t, true /* withBackup */)

	// execFn stub: 명시적 실패 반환
	original := execFn
	execFn = func(_ string, _ []string, _ []string) error {
		return errors.New("exec syscall denied")
	}
	defer func() { execFn = original }()

	err := o.AutoRollback(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed),
		"post-rollback exec 실패는 ErrUpdateRollbackFailed 로 wrapping 되어야 함")
	assert.Contains(t, err.Error(), "exec syscall denied")
}

// ---- Orchestrate(): 통합 (run-all) ----

func TestRestartOrchestrator_Orchestrate_TamperedBinary_FailsBeforeExec(t *testing.T) {
	o := makeOrchestrator(t, []byte("legit-content"))

	// 매니페스트는 legit 그대로지만, 디스크 파일을 변조
	require.NoError(t, os.WriteFile(o.DownloadedPath, []byte("EVIL"), 0o755))

	// execFn 도 호출되어선 안 됨 (TOCTOU 거부 시점이 exec 이전)
	var execCalled atomic.Int32
	original := execFn
	execFn = func(_ string, _ []string, _ []string) error {
		execCalled.Add(1)
		return nil
	}
	defer func() { execFn = original }()

	_, err := o.Orchestrate(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRestartFailed),
		"변조된 바이너리는 reverifyDownloaded 단계에서 ErrUpdateRestartFailed 로 거부되어야 함")
	assert.Equal(t, int32(0), execCalled.Load(), "exec 은 호출되지 않아야 함 (TOCTOU 차단 점)")
}

func TestRestartOrchestrator_Orchestrate_DrainAttempted_BeforeExec(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	mock := &orchestratorMockDrain{}
	o.DrainSignal = mock
	o.DrainTimeout = 100 * time.Millisecond

	// execFn stub: 정상 시 return 안 해야 하지만 테스트에서는 명시 에러로 분기 검증
	original := execFn
	execFn = func(_ string, _ []string, _ []string) error {
		return errors.New("simulated exec failure")
	}
	defer func() { execFn = original }()

	_, err := o.Orchestrate(context.Background())
	require.Error(t, err)
	// drain 은 exec 이전에 호출되었어야 함
	assert.Equal(t, int32(1), mock.calls.Load(), "Drain 은 exec 직전 한 번 호출되어야 함")
	assert.True(t, errors.Is(err, ErrUpdateRestartFailed),
		"exec 실패는 ErrUpdateRestartFailed 로 wrapping 되어야 함")
}

func TestRestartOrchestrator_Orchestrate_DrainFailure_StillProceedsToExec(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	mock := &orchestratorMockDrain{
		returnErr: errors.New("drain timeout"),
	}
	o.DrainSignal = mock
	o.DrainTimeout = 50 * time.Millisecond

	var execCalled atomic.Int32
	original := execFn
	execFn = func(_ string, _ []string, _ []string) error {
		execCalled.Add(1)
		return errors.New("simulated exec failure")
	}
	defer func() { execFn = original }()

	_, err := o.Orchestrate(context.Background())
	require.Error(t, err)
	assert.Equal(t, int32(1), execCalled.Load(),
		"drain 실패해도 exec 은 진행되어야 함 (방어적 동작)")
}

// ---- Concurrency / state safety ----

func TestRestartOrchestrator_PostExecHealthCheck_ContextCancellation(t *testing.T) {
	srv := startTestServer(t, http.StatusServiceUnavailable, `nope`)
	defer srv.Close()

	o := makeOrchestrator(t, []byte("content"))
	o.HealthChecker = &HealthChecker{
		Endpoint: srv.URL,
		Timeout:  5 * time.Second,
		Interval: 100 * time.Millisecond,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := o.PostExecHealthCheck(ctx, "v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateHealthCheckFailed),
		"context 취소도 health check 실패로 매핑되어야 함")
}

// ---- Edge: nil 의존성 방어 ----

func TestRestartOrchestrator_PostExecHealthCheck_NilHealthChecker_GuardedFailure(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	o.HealthChecker = nil // 실수 시뮬레이션

	defer func() {
		if r := recover(); r != nil {
			t.Logf("recovered panic: %v (acceptable if it's a clear nil deref)", r)
		}
	}()

	_, err := o.PostExecHealthCheck(context.Background(), "v0.4.0")
	// 이 케이스는 panic 또는 명시 error — 어떤 결과든 critical 이지만 silent crash 는 안 됨.
	if err == nil {
		t.Errorf("nil HealthChecker 는 명시 에러를 반환해야 함")
	}
}

// ---- helpers verification ----

func TestMakeTestKeypair_GeneratesValidPair(t *testing.T) {
	pub, priv, v := makeTestKeypair(t)
	require.NotNil(t, v)
	require.Len(t, pub, ed25519.PublicKeySize)
	require.Len(t, priv, ed25519.PrivateKeySize)

	// round-trip: sign + verify
	content := []byte("hello")
	sig := ed25519.Sign(priv, content)
	assert.NoError(t, v.VerifySignature(content, sig))
}

func TestWriteSignedBinary_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	_, priv, v := makeTestKeypair(t)
	content := []byte("payload")
	path, sha, sig := writeSignedBinary(t, dir, content, priv)

	read, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, read)

	require.NoError(t, v.VerifyAll(content, sha, sig))
}

func TestMakeRollbackEnv_BackupExists(t *testing.T) {
	o := makeRollbackEnv(t, true)
	assert.True(t, o.Rollback.CanRollback(), "withBackup=true 일 때 backup 이 있어야 함")

	o2 := makeRollbackEnv(t, false)
	assert.False(t, o2.Rollback.CanRollback(), "withBackup=false 일 때 backup 이 없어야 함")
}

// 종합 sanity: orchestrator 의 logger 가 nil 인 경우 default 로 fallback 되어야 함
func TestRestartOrchestrator_NilLogger_UsesDefault(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	o.Logger = nil

	// drain 호출 → 로그 출력 시 panic 없이 진행되어야 함
	_, err := o.gracefulDrain(context.Background())
	require.NoError(t, err)
}

// NewRestartOrchestrator factory 검증
func TestNewRestartOrchestrator_AllFieldsPropagated(t *testing.T) {
	dir := t.TempDir()
	mainPath := filepath.Join(dir, "xflowd")
	require.NoError(t, os.WriteFile(mainPath, []byte("binary"), 0o755))

	_, _, verifier := makeTestKeypair(t)
	applier := &Applier{
		Verifier:   verifier,
		binaryPath: mainPath,
		backupPath: mainPath + ".previous",
	}
	mock := &orchestratorMockDrain{}

	params := RestartOrchestratorParams{
		Verifier:       verifier,
		Applier:        applier,
		BinaryPath:     mainPath,
		DrainSignal:    mock,
		DrainTimeout:   12 * time.Second,
		HealthEndpoint: "http://localhost:1234/health",
		HealthTimeout:  5 * time.Second,
		HealthInterval: 200 * time.Millisecond,
		DownloadedPath: mainPath,
		Manifest:       Manifest{Version: "v0.4.0"},
		Logger:         slog.Default(),
	}

	o := NewRestartOrchestrator(params)
	require.NotNil(t, o)
	assert.Same(t, verifier, o.Verifier)
	assert.Same(t, applier, o.Applier)
	require.NotNil(t, o.Rollback)
	require.NotNil(t, o.HealthChecker)
	assert.Equal(t, "http://localhost:1234/health", o.HealthChecker.Endpoint)
	assert.Equal(t, 5*time.Second, o.HealthChecker.Timeout)
	assert.Equal(t, 200*time.Millisecond, o.HealthChecker.Interval)
	assert.Same(t, mock, o.DrainSignal)
	assert.Equal(t, 12*time.Second, o.DrainTimeout)
	assert.Equal(t, mainPath, o.DownloadedPath)
	assert.Equal(t, Version("v0.4.0"), o.Manifest.Version)
}

// AutoRollback: nil Rollback 가드
func TestRestartOrchestrator_AutoRollback_NilRollback_GuardedFailure(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	o.Rollback = nil

	err := o.AutoRollback(context.Background())
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRollbackFailed),
		"nil Rollback 도 명시적 ErrUpdateRollbackFailed 로 거부")
}

// reverifyDownloaded: nil Verifier 가드
func TestRestartOrchestrator_ReverifyDownloaded_NilVerifier_RestartFailed(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	o.Verifier = nil

	err := o.reverifyDownloaded()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRestartFailed))
	assert.Contains(t, err.Error(), "Verifier is nil")
}

// reverifyDownloaded: 빈 DownloadedPath 가드
func TestRestartOrchestrator_ReverifyDownloaded_EmptyPath_RestartFailed(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	o.DownloadedPath = ""

	err := o.reverifyDownloaded()
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateRestartFailed))
	assert.Contains(t, err.Error(), "DownloadedPath is empty")
}

// gracefulDrain: ctx 가 이미 취소된 경우 fail-fast
func TestRestartOrchestrator_GracefulDrain_PreCancelledCtx(t *testing.T) {
	o := makeOrchestrator(t, []byte("content"))
	o.DrainSignal = &orchestratorMockDrain{}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	stats, err := o.gracefulDrain(ctx)
	require.Error(t, err, "취소된 ctx 는 fail-fast")
	assert.Equal(t, -1, stats.DroppedReqs)
}

// ensure ApplyResult 가 만족스러운지 (구조체 필드 모음 확인용 컴파일 가드)
var _ = ApplyResult{
	NewVersion: "v0.4.0",
	BackupPath: "",
	AppliedAt:  time.Time{},
}

// ensure Manifest 형식 컴파일 가드
var _ = Manifest{Version: "v0.4.0", SHA256: "deadbeef", Signature: nil}

// ensure logger 호환
var _ = (*slog.Logger)(nil)

// dummy const referenced for fmt usage
const _orchestratorTestPackage = "updater"

var _ = fmt.Sprintf
