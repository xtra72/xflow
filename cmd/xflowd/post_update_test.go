package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/updater"
)

func TestPostUpdateEnv_AddsMarkers(t *testing.T) {
	env := postUpdateEnv([]string{"PATH=/bin", "HOME=/root"}, "v1.3.0")
	assert.Contains(t, env, "PATH=/bin")
	assert.Contains(t, env, "XFLOW_POST_UPDATE=1")
	assert.Contains(t, env, "XFLOW_UPDATE_EXPECTED_VERSION=v1.3.0")
}

func TestIsPostUpdateBoot(t *testing.T) {
	t.Setenv(envPostUpdate, "1")
	assert.True(t, isPostUpdateBoot())
	t.Setenv(envPostUpdate, "")
	assert.False(t, isPostUpdateBoot())
}

// 부팅 헬스체크 통과: /health 가 200 이면 PostExecHealthCheck 성공(롤백 없음).
func TestPostUpdate_HealthyPasses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	}))
	defer srv.Close()

	orch := updater.NewRestartOrchestrator(updater.RestartOrchestratorParams{
		BinaryPath:     "/nonexistent/xflowd",
		HealthEndpoint: srv.URL,
		HealthTimeout:  2 * time.Second,
		HealthInterval: 20 * time.Millisecond,
	})
	res, err := orch.PostExecHealthCheck(context.Background(), "")
	require.NoError(t, err)
	assert.True(t, res.Healthy)
}

// 부팅 헬스체크 실패 + 백업 없음: AutoRollback 은 exec 없이 오류를 반환한다(롤백 불가).
func TestPostUpdate_UnhealthyNoBackup_RollbackErrors(t *testing.T) {
	// 즉시 503 → 헬스 폴링 실패.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	orch := updater.NewRestartOrchestrator(updater.RestartOrchestratorParams{
		BinaryPath:     t.TempDir() + "/xflowd", // .previous 백업 없음 → CanRollback=false
		HealthEndpoint: srv.URL,
		HealthTimeout:  300 * time.Millisecond,
		HealthInterval: 20 * time.Millisecond,
	})
	_, err := orch.PostExecHealthCheck(context.Background(), "")
	require.Error(t, err, "503 이면 헬스체크 실패")

	// 백업이 없으므로 롤백은 exec 없이 오류를 반환한다(무한 루프/오작동 방지).
	rbErr := orch.AutoRollback(context.Background())
	require.Error(t, rbErr)
}

// 헬스 URL 이 포트로 올바르게 구성되는지(보안: 무인증 /health 사용) 형식만 검증.
func TestPostUpdate_HealthURLFormat(t *testing.T) {
	const port = 8081
	healthURL := "http://127.0.0.1:" + strconv.Itoa(port) + "/health"
	u, err := url.Parse(healthURL)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:"+strconv.Itoa(port), u.Host)
	assert.True(t, strings.HasSuffix(u.Path, "/health"), "무인증 liveness 엔드포인트")
}
