package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/updater"
)

func TestUpdateState_RoundTrip(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "xflowd")
	// 미존재 → ok=false.
	_, ok := readUpdateState(bin)
	assert.False(t, ok)

	require.NoError(t, markUpdatePending(bin, "v1.3.0"))
	st, ok := readUpdateState(bin)
	require.True(t, ok)
	assert.Equal(t, "v1.3.0", st.Version)
	assert.Equal(t, 0, st.BootAttempts)
	assert.True(t, st.Pending)

	clearUpdateState(bin)
	_, ok = readUpdateState(bin)
	assert.False(t, ok, "clear 후 상태 없음")
}

func TestDecideBootAction(t *testing.T) {
	// pending 아님/없음 → none.
	assert.Equal(t, bootNone, decideBootAction(updateState{}, false, 2))
	assert.Equal(t, bootNone, decideBootAction(updateState{Pending: false}, true, 2))
	// pending + 시도 여유 → health check.
	assert.Equal(t, bootHealthCheck, decideBootAction(updateState{Pending: true, BootAttempts: 0}, true, 2))
	assert.Equal(t, bootHealthCheck, decideBootAction(updateState{Pending: true, BootAttempts: 1}, true, 2))
	// pending + 시도 임계 도달 → rollback.
	assert.Equal(t, bootRollback, decideBootAction(updateState{Pending: true, BootAttempts: 2}, true, 2))
	assert.Equal(t, bootRollback, decideBootAction(updateState{Pending: true, BootAttempts: 5}, true, 2))
}

// runPostUpdateSelfCheck: pending 없으면 즉시 no-op(상태 파일 미생성).
func TestRunPostUpdateSelfCheck_NoPending_Noop(t *testing.T) {
	// 실행 파일 경로(os.Executable)는 go test 바이너리지만, 상태 파일이 없으므로 no-op.
	runPostUpdateSelfCheck(context.Background(), 0, slog.Default())
	// 패닉/블록 없이 즉시 반환하면 성공.
}

// 헬스 통과 시 PostExecHealthCheck 가 성공하고 상태가 정리되는 경로(orchestrator 단위).
func TestBootOrchestrator_HealthyPasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	orch := updater.NewRestartOrchestrator(updater.RestartOrchestratorParams{
		BinaryPath:     filepath.Join(t.TempDir(), "xflowd"),
		HealthEndpoint: srv.URL,
		HealthTimeout:  2 * time.Second,
		HealthInterval: 20 * time.Millisecond,
	})
	res, err := orch.PostExecHealthCheck(context.Background(), "")
	require.NoError(t, err)
	assert.True(t, res.Healthy)
}

// 백업 없이 health 실패 시 AutoRollback 은 exec 없이 오류를 반환한다(무한 루프 방지).
func TestBootOrchestrator_UnhealthyNoBackup_RollbackErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	orch := updater.NewRestartOrchestrator(updater.RestartOrchestratorParams{
		BinaryPath:     filepath.Join(t.TempDir(), "xflowd"), // .previous 없음
		HealthEndpoint: srv.URL,
		HealthTimeout:  300 * time.Millisecond,
		HealthInterval: 20 * time.Millisecond,
	})
	_, err := orch.PostExecHealthCheck(context.Background(), "")
	require.Error(t, err)
	require.Error(t, orch.AutoRollback(context.Background()))
}
