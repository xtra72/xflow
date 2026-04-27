package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// testHealthChecker 는 테스트용 HealthChecker 구현이다.
type testHealthChecker struct {
	err error
}

func (h *testHealthChecker) HealthCheck(_ context.Context) error {
	return h.err
}

func newTestServerConfig() *config.ServerConfig {
	return &config.ServerConfig{
		Port: 0, // 임의 포트
		Host: "127.0.0.1",
		Mode: "test",
		CORS: config.CORSConfig{
			Enabled:        false,
			AllowedOrigins: []string{"*"},
		},
		RateLimit: config.RateLimitConfig{
			Enabled:           false,
			RequestsPerSecond: 100,
		},
	}
}

func newTestServer(opts ...ServerOption) *Server {
	cfg := newTestServerConfig()
	defaultOpts := []ServerOption{
		WithLogger(slog.Default()),
	}
	allOpts := append(defaultOpts, opts...)
	s := NewServer(cfg, allOpts...)
	s.SetupRoutes()
	return s
}

func startTestServer(t *testing.T, s *Server) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(func() {
		cancel()
		// 서버가 이미 중지된 경우 에러 무시
		_ = s.Stop(context.Background())
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start(ctx)
	}()

	// 서버가 시작될 때까지 대기
	for i := 0; i < 100; i++ {
		if s.ListenAddr() != "" {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("서버 시작 타임아웃")
}

func TestNewServer(t *testing.T) {
	cfg := newTestServerConfig()
	s := NewServer(cfg)

	require.NotNil(t, s)
	assert.Equal(t, lifecycle.StateCreated, s.State())
	assert.NotNil(t, s.router)
	assert.NotNil(t, s.stats)
	assert.NotNil(t, s.logger)
	assert.NotNil(t, s.healthDeps)
}

func TestNewServer_WithOptions(t *testing.T) {
	cfg := newTestServerConfig()
	logger := slog.Default()
	checker := &testHealthChecker{}

	s := NewServer(cfg,
		WithLogger(logger),
		WithHealthDependency("db", checker),
	)

	require.NotNil(t, s)
	assert.Same(t, logger, s.logger)
	assert.Contains(t, s.healthDeps, "db")
}

func TestServer_StartStop(t *testing.T) {
	s := newTestServer()
	startTestServer(t, s)

	assert.Equal(t, lifecycle.StateRunning, s.State())
	assert.NotEmpty(t, s.ListenAddr())

	err := s.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, s.State())
}

func TestServer_StartFromInvalidState(t *testing.T) {
	s := newTestServer()
	startTestServer(t, s)

	// Running 상태에서 다시 Start 시도
	err := s.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot start from state")
}

func TestServer_StopFromInvalidState(t *testing.T) {
	s := newTestServer()
	// Created 상태에서 Stop 시도
	err := s.Stop(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot stop from state")
}

func TestServer_ListenAddr_BeforeStart(t *testing.T) {
	s := newTestServer()
	assert.Empty(t, s.ListenAddr())
}

func TestServer_HealthEndpoint(t *testing.T) {
	s := newTestServer()
	startTestServer(t, s)

	addr := s.ListenAddr()
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, true, body["success"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", data["status"])
}

func TestServer_ReadyEndpoint_AllHealthy(t *testing.T) {
	healthyChecker := &testHealthChecker{err: nil}
	s := newTestServer(
		WithHealthDependency("db", healthyChecker),
	)
	startTestServer(t, s)

	addr := s.ListenAddr()
	resp, err := http.Get(fmt.Sprintf("http://%s/ready", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)
	assert.Equal(t, true, body["success"])

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", data["status"])
}

func TestServer_ReadyEndpoint_Degraded(t *testing.T) {
	unhealthyChecker := &testHealthChecker{err: errors.New("connection refused")}
	s := newTestServer(
		WithHealthDependency("db", unhealthyChecker),
	)
	startTestServer(t, s)

	addr := s.ListenAddr()
	resp, err := http.Get(fmt.Sprintf("http://%s/ready", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "degraded", data["status"])

	checks, ok := data["checks"].(map[string]any)
	require.True(t, ok)

	dbCheck, ok := checks["db"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "unhealthy", dbCheck["status"])
	assert.Equal(t, "connection refused", dbCheck["message"])
}

func TestServer_ReadyEndpoint_NoDependencies(t *testing.T) {
	s := newTestServer()
	startTestServer(t, s)

	addr := s.ListenAddr()
	resp, err := http.Get(fmt.Sprintf("http://%s/ready", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "ok", data["status"])
}

func TestServer_ReadyEndpoint_MixedDependencies(t *testing.T) {
	s := newTestServer(
		WithHealthDependency("db", &testHealthChecker{err: nil}),
		WithHealthDependency("cache", &testHealthChecker{err: errors.New("timeout")}),
	)
	startTestServer(t, s)

	addr := s.ListenAddr()
	resp, err := http.Get(fmt.Sprintf("http://%s/ready", addr))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)

	var body map[string]any
	err = json.NewDecoder(resp.Body).Decode(&body)
	require.NoError(t, err)

	data, ok := body["data"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "degraded", data["status"])

	checks, ok := data["checks"].(map[string]any)
	require.True(t, ok)

	dbCheck, ok := checks["db"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "healthy", dbCheck["status"])

	cacheCheck, ok := checks["cache"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "unhealthy", cacheCheck["status"])
}

func TestServer_Info(t *testing.T) {
	s := newTestServer()
	startTestServer(t, s)

	info := s.Info()
	assert.Equal(t, "0.1.0", info.Version)
	assert.NotEmpty(t, info.Uptime)
	assert.False(t, info.StartedAt.IsZero())
	assert.NotEmpty(t, info.GoVersion)
	assert.Greater(t, info.RegisteredRoutes, 0) // health, ready 라우트
}

func TestServer_Info_BeforeStart(t *testing.T) {
	s := newTestServer()
	info := s.Info()
	assert.Equal(t, "0.1.0", info.Version)
	assert.Empty(t, info.Uptime)
	assert.True(t, info.StartedAt.IsZero())
}

func TestServer_Stats(t *testing.T) {
	s := newTestServer()
	startTestServer(t, s)

	stats := s.Stats()
	assert.Equal(t, int64(0), stats.TotalRequests)
	assert.Equal(t, int64(0), stats.ErrorCount)
	assert.Equal(t, 0.0, stats.ErrorRate)
}

func TestServer_State(t *testing.T) {
	s := newTestServer()
	assert.Equal(t, lifecycle.StateCreated, s.State())

	startTestServer(t, s)
	assert.Equal(t, lifecycle.StateRunning, s.State())

	err := s.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, s.State())
}

func TestServer_Router(t *testing.T) {
	s := newTestServer()
	r := s.Router()
	require.NotNil(t, r)
	assert.IsType(t, &Router{}, r)
}

func TestServer_SetupRoutes(t *testing.T) {
	s := newTestServer()
	// SetupRoutes는 newTestServer에서 이미 호출됨
	routeCount := s.Router().RouteCount()
	assert.GreaterOrEqual(t, routeCount, 2) // health + ready
}

func TestServer_GracefulShutdown(t *testing.T) {
	s := newTestServer()
	startTestServer(t, s)

	addr := s.ListenAddr()

	// 서버가 요청을 처리할 수 있는지 확인
	resp, err := http.Get(fmt.Sprintf("http://%s/health", addr))
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	// 그레이스풀 셧다운
	err = s.Stop(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, s.State())
}

func TestServer_ContextCancellation(t *testing.T) {
	cfg := newTestServerConfig()
	s := NewServer(cfg, WithLogger(slog.Default()))
	s.SetupRoutes()

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start(ctx)
	}()

	// 서버 시작 대기
	for i := 0; i < 100; i++ {
		if s.ListenAddr() != "" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	assert.Equal(t, lifecycle.StateRunning, s.State())

	// 컨텍스트 취소로 서버 중지
	cancel()

	select {
	case err := <-errCh:
		// Stop 성공 또는 이미 중지 됨
		if err != nil {
			// 에러가 있더라도 서버가 중지되면 OK
			assert.True(t, s.State() == lifecycle.StateStopped || s.State() == lifecycle.StateError)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("서버 종료 타임아웃")
	}
}

func TestWithLogger(t *testing.T) {
	logger := slog.Default()
	opt := WithLogger(logger)

	s := &Server{healthDeps: make(map[string]HealthChecker)}
	opt(s)
	assert.Same(t, logger, s.logger)
}

func TestWithHealthDependency(t *testing.T) {
	checker := &testHealthChecker{}
	opt := WithHealthDependency("test", checker)

	s := &Server{healthDeps: make(map[string]HealthChecker)}
	opt(s)
	assert.Contains(t, s.healthDeps, "test")
	assert.Same(t, checker, s.healthDeps["test"])
}

func TestServer_StartListenError(t *testing.T) {
	// 이미 사용 중인 포트에서 시작 시 에러
	s1 := newTestServer()
	startTestServer(t, s1)

	addr := s1.ListenAddr()
	// s1이 사용하는 포트 추출
	var port int
	fmt.Sscanf(addr, "127.0.0.1:%d", &port)

	cfg := &config.ServerConfig{
		Port: port,
		Host: "127.0.0.1",
		Mode: "test",
	}
	s2 := NewServer(cfg, WithLogger(slog.Default()))
	s2.SetupRoutes()

	err := s2.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to listen")
}
