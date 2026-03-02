package api

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// HealthChecker 는 의존성의 건강 상태를 확인하는 인터페이스이다.
type HealthChecker interface {
	HealthCheck(ctx context.Context) error
}

// Server 는 HTTP 서버의 라이프사이클을 관리한다.
type Server struct {
	config     *config.ServerConfig
	router     *Router
	httpServer *http.Server
	logger     *slog.Logger
	stats      *statsCollector
	listener   net.Listener
	healthDeps map[string]HealthChecker
	mu         sync.RWMutex
	state      lifecycle.State
	startedAt  time.Time
}

// ServerOption 은 Server 구성을 위한 함수 옵션이다.
type ServerOption func(*Server)

// WithLogger 는 서버에 사용할 로거를 설정한다.
func WithLogger(logger *slog.Logger) ServerOption {
	return func(s *Server) {
		s.logger = logger
	}
}

// WithHealthDependency 는 레디니스 체크에 사용할 의존성을 추가한다.
func WithHealthDependency(name string, checker HealthChecker) ServerOption {
	return func(s *Server) {
		s.healthDeps[name] = checker
	}
}

// NewServer 는 새 API 서버를 생성한다.
func NewServer(cfg *config.ServerConfig, opts ...ServerOption) *Server {
	s := &Server{
		config:     cfg,
		router:     NewRouter(),
		logger:     slog.Default(),
		stats:      newStatsCollector(),
		healthDeps: make(map[string]HealthChecker),
		state:      lifecycle.StateCreated,
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

// SetupRoutes 는 모든 API 라우트를 구성한다.
// Start 전에 호출되어야 한다.
// P1에서는 health/ready 엔드포인트만 등록한다. 핸들러는 P2에서 추가된다.
func (s *Server) SetupRoutes() {
	// 미들웨어 체인 구성 (SPEC 5.4 순서):
	// Recovery -> RequestID -> Logger -> Compress -> CORS -> RateLimit -> Timeout -> Auth
	s.router.Use(
		Recovery(s.logger),
		RequestID(),
		Logger(s.logger),
		Compress(),
	)

	// CORS 미들웨어 (설정이 활성화된 경우에만)
	if s.config.CORS.Enabled {
		s.router.Use(CORS(s.config.CORS))
	}

	// RateLimit 미들웨어 (설정이 활성화된 경우에만)
	if s.config.RateLimit.Enabled {
		s.router.Use(RateLimit(s.config.RateLimit))
	}

	// 타임아웃 미들웨어 (30초 기본값)
	s.router.Use(Timeout(30 * time.Second))

	// 인증 미들웨어 (P1에서는 패스스루)
	s.router.Use(Auth())

	// 헬스/레디 엔드포인트
	s.router.GET("/health", s.healthCheck)
	s.router.GET("/ready", s.readyCheck)

	// API v1 그룹
	_ = s.router.Group("/api/v1")
}

// RegisterRoutes 는 API v1 그룹에 추가 라우트를 등록한다.
// import cycle 방지를 위해 handler 패키지에서 라우트 등록 함수를 받는다.
// SetupRoutes 이후에 호출되어야 한다.
func (s *Server) RegisterRoutes(register func(g *RouteGroup)) {
	g := s.router.Group("/api/v1")
	register(g)
}

// RegisterRawHandler 는 raw HTTP 핸들러를 라우터에 직접 등록한다.
// WebSocket 업그레이드 등 Context 래퍼를 바이패스해야 하는 경우에 사용한다.
// SetupRoutes 이후에 호출되어야 한다.
func (s *Server) RegisterRawHandler(pattern string, handler http.HandlerFunc) {
	s.router.HandleFunc(pattern, handler)
}

// Start 는 HTTP 리스너를 시작하고 서빙을 시작한다.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()

	if s.state != lifecycle.StateCreated && s.state != lifecycle.StateStopped {
		s.mu.Unlock()
		return fmt.Errorf("server: cannot start from state %s", s.state)
	}

	addr := fmt.Sprintf("%s:%d", s.config.Host, s.config.Port)

	ln, err := net.Listen("tcp", addr)
	if err != nil {
		s.state = lifecycle.StateError
		s.mu.Unlock()
		return fmt.Errorf("server: failed to listen on %s: %w", addr, err)
	}

	s.listener = ln
	s.httpServer = &http.Server{
		Handler:      s.router.Handler(),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	s.state = lifecycle.StateRunning
	s.startedAt = time.Now()
	s.stats = newStatsCollector()

	s.mu.Unlock()

	s.logger.Info("서버 시작",
		slog.String("addr", ln.Addr().String()),
		slog.String("mode", s.config.Mode),
	)

	// Serve는 블로킹이므로 고루틴에서 실행한다
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.Serve(ln); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	// 컨텍스트 취소 또는 에러를 기다린다
	select {
	case err := <-errCh:
		if err != nil {
			s.mu.Lock()
			s.state = lifecycle.StateError
			s.mu.Unlock()
			return fmt.Errorf("server: serve error: %w", err)
		}
		return nil
	case <-ctx.Done():
		return s.Stop(context.Background())
	}
}

// Stop 은 설정된 타임아웃으로 그레이스풀 셧다운을 수행한다 (기본 30초).
// 이미 정지된 경우 nil을 반환한다 (멱등성).
func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	if s.state == lifecycle.StateStopping || s.state == lifecycle.StateStopped {
		s.mu.Unlock()
		return nil
	}
	if s.state != lifecycle.StateRunning {
		s.mu.Unlock()
		return fmt.Errorf("server: cannot stop from state %s", s.state)
	}
	s.state = lifecycle.StateStopping
	httpSrv := s.httpServer
	s.mu.Unlock()

	s.logger.Info("서버 정지 시작")

	// 셧다운 타임아웃 컨텍스트 생성
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		s.mu.Lock()
		s.state = lifecycle.StateError
		s.mu.Unlock()
		return fmt.Errorf("server: shutdown error: %w", err)
	}

	s.mu.Lock()
	s.state = lifecycle.StateStopped
	s.mu.Unlock()

	s.logger.Info("서버 정지 완료")
	return nil
}

// ListenAddr 은 현재 리스닝 주소를 반환한다 (예: ":8080").
func (s *Server) ListenAddr() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return ""
	}
	return s.listener.Addr().String()
}

// Info 는 ServerInfo 스냅샷을 반환한다.
func (s *Server) Info() ServerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var uptime string
	if !s.startedAt.IsZero() {
		uptime = time.Since(s.startedAt).Truncate(time.Second).String()
	}

	return ServerInfo{
		Version:           "0.1.0",
		Uptime:            uptime,
		StartedAt:         s.startedAt,
		ActiveConnections: s.stats.ActiveConnections(),
		RegisteredRoutes:  s.router.RouteCount(),
		GoVersion:         runtime.Version(),
	}
}

// Stats 는 ServerStats 스냅샷을 반환한다.
func (s *Server) Stats() ServerStats {
	return s.stats.Snapshot()
}

// State 는 현재 서버 상태를 반환한다.
func (s *Server) State() lifecycle.State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

// Router 는 서버의 라우터를 반환한다.
func (s *Server) Router() *Router {
	return s.router
}

// healthCheck 는 /health 핸들러이다 (라이브니스 체크).
func (s *Server) healthCheck(ctx Context) error {
	type healthResponse struct {
		Status string `json:"status"`
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(healthResponse{
		Status: "ok",
	}))
}

// readyCheck 는 /ready 핸들러이다 (레디니스 체크 - 의존성 확인).
func (s *Server) readyCheck(ctx Context) error {
	type checkResult struct {
		Status  string `json:"status"`
		Message string `json:"message,omitempty"`
	}
	type readyResponse struct {
		Status string                 `json:"status"`
		Checks map[string]checkResult `json:"checks,omitempty"`
	}

	checks := make(map[string]checkResult)
	allHealthy := true

	s.mu.RLock()
	deps := make(map[string]HealthChecker, len(s.healthDeps))
	for k, v := range s.healthDeps {
		deps[k] = v
	}
	s.mu.RUnlock()

	for name, checker := range deps {
		if err := checker.HealthCheck(ctx.Context()); err != nil {
			allHealthy = false
			checks[name] = checkResult{
				Status:  "unhealthy",
				Message: err.Error(),
			}
		} else {
			checks[name] = checkResult{
				Status: "healthy",
			}
		}
	}

	status := "ok"
	httpCode := http.StatusOK
	if !allHealthy {
		status = "degraded"
		httpCode = http.StatusServiceUnavailable
	}

	resp := readyResponse{
		Status: status,
	}
	if len(checks) > 0 {
		resp.Checks = checks
	}

	return ctx.JSON(httpCode, dto.NewSuccessResponse(resp))
}
