package handler

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/observe"
)

// MonitorManager 는 시스템 모니터링 작업을 위한 인터페이스이다.
// 핸들러를 구체적인 모니터링 구현으로부터 분리한다.
type MonitorManager interface {
	// GetMetrics 는 시스템 메트릭을 반환한다.
	GetMetrics(ctx context.Context) (*MetricsResponse, error)
	// SetLogLevel 은 로그 레벨을 변경한다.
	SetLogLevel(ctx context.Context, level string) error
	// ListLogLevels 는 컴포넌트별 로그 레벨 목록을 반환한다.
	ListLogLevels(ctx context.Context) (*LogLevelsResponse, error)
	// SetComponentLogLevel 은 특정 컴포넌트의 로그 레벨을 설정한다.
	SetComponentLogLevel(ctx context.Context, component, level string) error
	// ResetComponentLogLevel 은 특정 컴포넌트의 로그 레벨을 기본값으로 초기화한다.
	ResetComponentLogLevel(ctx context.Context, component string) error
}

// MetricsResponse 는 시스템 메트릭 응답을 나타낸다.
type MetricsResponse struct {
	CPUUsagePercent  float64 `json:"cpu_usage_percent"`
	MemoryUsagePct   float64 `json:"memory_usage_percent"`
	GoRoutines       int     `json:"go_routines"`
	GoMemAllocMB     float64 `json:"go_mem_alloc_mb"`
	GoMemSysMB       float64 `json:"go_mem_sys_mb"`
	UptimeSeconds    float64 `json:"uptime_seconds"`
}

// logLevelRequest 는 로그 레벨 변경 요청을 나타낸다.
type logLevelRequest struct {
	Level string `json:"level"`
}

// LogLevelsResponse 는 컴포넌트별 로그 레벨 목록 응답을 나타낸다.
type LogLevelsResponse struct {
	DefaultLevel string            `json:"default_level"`
	Components   map[string]string `json:"components"`
}

// componentLogLevelResponse 는 단일 컴포넌트의 로그 레벨 응답을 나타낸다.
type componentLogLevelResponse struct {
	Component string `json:"component"`
	Level     string `json:"level,omitempty"`
}

// MonitorHandler 는 모니터링 관련 API 엔드포인트를 처리한다.
type MonitorHandler struct {
	monitor MonitorManager
	logger  *slog.Logger
}

// NewMonitorHandler 는 새 MonitorHandler 를 생성한다.
func NewMonitorHandler(monitor MonitorManager, logger *slog.Logger) *MonitorHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &MonitorHandler{
		monitor: monitor,
		logger:  logger,
	}
}

// RegisterRoutes 는 주어진 라우트 그룹에 모니터링 라우트를 등록한다.
//
// Routes:
//
//	GET    /monitor/metrics                -> Metrics
//	GET    /monitor/loglevel               -> ListLogLevels
//	PUT    /monitor/loglevel               -> SetLogLevel (글로벌)
//	PUT    /monitor/loglevel/{component}   -> SetComponentLogLevel
//	DELETE /monitor/loglevel/{component}   -> ResetComponentLogLevel
func (h *MonitorHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/monitor/metrics", h.Metrics)
	g.GET("/monitor/loglevel", h.ListLogLevels)
	g.PUT("/monitor/loglevel", h.SetLogLevel)
	g.PUT("/monitor/loglevel/{component}", h.SetComponentLogLevel)
	g.DELETE("/monitor/loglevel/{component}", h.ResetComponentLogLevel)
}

// Metrics 는 시스템 메트릭을 반환한다.
// GET /monitor/metrics
func (h *MonitorHandler) Metrics(ctx api.Context) error {
	metrics, err := h.monitor.GetMetrics(ctx.Context())
	if err != nil {
		h.logger.Error("메트릭 조회 실패", "error", err)
		return ctx.JSON(http.StatusInternalServerError,
			dto.NewErrorResponse("METRICS_ERROR", "메트릭 조회에 실패했습니다"))
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(metrics))
}

// SetLogLevel 은 로그 레벨을 변경한다.
// PUT /monitor/loglevel
func (h *MonitorHandler) SetLogLevel(ctx api.Context) error {
	var req logLevelRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest,
			dto.NewErrorResponse("INVALID_REQUEST", "요청 본문을 파싱할 수 없습니다"))
	}

	if err := h.monitor.SetLogLevel(ctx.Context(), req.Level); err != nil {
		h.logger.Warn("로그 레벨 변경 실패", "level", req.Level, "error", err)
		return ctx.JSON(http.StatusBadRequest,
			dto.NewErrorResponse("INVALID_LOG_LEVEL", err.Error()))
	}

	h.logger.Info("로그 레벨 변경 완료", "level", req.Level)
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"level": req.Level,
	}))
}

// ListLogLevels 는 컴포넌트별 로그 레벨 목록을 반환한다.
// GET /monitor/loglevel
func (h *MonitorHandler) ListLogLevels(ctx api.Context) error {
	resp, err := h.monitor.ListLogLevels(ctx.Context())
	if err != nil {
		h.logger.Error("로그 레벨 목록 조회 실패", "error", err)
		return ctx.JSON(http.StatusInternalServerError,
			dto.NewErrorResponse("LOG_LEVEL_ERROR", err.Error()))
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// SetComponentLogLevel 은 특정 컴포넌트의 로그 레벨을 설정한다.
// PUT /monitor/loglevel/{component}
func (h *MonitorHandler) SetComponentLogLevel(ctx api.Context) error {
	component := ctx.Param("component")
	if component == "" {
		return ctx.JSON(http.StatusBadRequest,
			dto.NewErrorResponse("INVALID_REQUEST", "컴포넌트 이름이 필요합니다"))
	}

	var req logLevelRequest
	if err := ctx.Bind(&req); err != nil {
		return ctx.JSON(http.StatusBadRequest,
			dto.NewErrorResponse("INVALID_REQUEST", "요청 본문을 파싱할 수 없습니다"))
	}

	if err := h.monitor.SetComponentLogLevel(ctx.Context(), component, req.Level); err != nil {
		h.logger.Warn("컴포넌트 로그 레벨 변경 실패",
			"component", component, "level", req.Level, "error", err)
		return ctx.JSON(http.StatusBadRequest,
			dto.NewErrorResponse("INVALID_LOG_LEVEL", err.Error()))
	}

	h.logger.Info("컴포넌트 로그 레벨 변경 완료", "component", component, "level", req.Level)
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(componentLogLevelResponse{
		Component: component,
		Level:     strings.ToLower(strings.TrimSpace(req.Level)),
	}))
}

// ResetComponentLogLevel 은 특정 컴포넌트의 로그 레벨을 기본값으로 초기화한다.
// DELETE /monitor/loglevel/{component}
func (h *MonitorHandler) ResetComponentLogLevel(ctx api.Context) error {
	component := ctx.Param("component")
	if component == "" {
		return ctx.JSON(http.StatusBadRequest,
			dto.NewErrorResponse("INVALID_REQUEST", "컴포넌트 이름이 필요합니다"))
	}

	if err := h.monitor.ResetComponentLogLevel(ctx.Context(), component); err != nil {
		h.logger.Warn("컴포넌트 로그 레벨 초기화 실패",
			"component", component, "error", err)
		return ctx.JSON(http.StatusInternalServerError,
			dto.NewErrorResponse("LOG_LEVEL_ERROR", err.Error()))
	}

	h.logger.Info("컴포넌트 로그 레벨 초기화 완료", "component", component)
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(componentLogLevelResponse{
		Component: component,
	}))
}

// --- defaultMonitorManager: 기본 모니터링 매니저 구현 ---

// validLogLevels 는 허용되는 로그 레벨 목록이다.
var validLogLevels = map[string]bool{
	"debug": true,
	"info":  true,
	"warn":  true,
	"error": true,
}

// defaultMonitorManager 는 Go runtime 기반의 기본 모니터링 매니저이다.
type defaultMonitorManager struct {
	startedAt time.Time
	logger    *slog.Logger
	levels    observe.LevelManager // nil 일 수 있음
}

// NewDefaultMonitorManager 는 기본 모니터링 매니저를 생성한다.
// levels 가 nil 이면 로그 레벨 변경 기능이 비활성화된다.
func NewDefaultMonitorManager(logger *slog.Logger, levels observe.LevelManager) *defaultMonitorManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &defaultMonitorManager{
		startedAt: time.Now(),
		logger:    logger,
		levels:    levels,
	}
}

// GetMetrics 는 Go runtime 기반 시스템 메트릭을 반환한다.
func (m *defaultMonitorManager) GetMetrics(_ context.Context) (*MetricsResponse, error) {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	uptime := time.Since(m.startedAt).Seconds()

	return &MetricsResponse{
		CPUUsagePercent:  0, // CPU 사용률은 샘플링이 필요하므로 v1 에서는 미지원
		MemoryUsagePct:   float64(mem.Alloc) / float64(mem.Sys) * 100,
		GoRoutines:       runtime.NumGoroutine(),
		GoMemAllocMB:     float64(mem.Alloc) / 1024 / 1024,
		GoMemSysMB:       float64(mem.Sys) / 1024 / 1024,
		UptimeSeconds:    uptime,
	}, nil
}

// SetLogLevel 은 로그 레벨을 변경한다.
// LevelManager 가 설정되지 않은 경우 에러를 반환한다.
func (m *defaultMonitorManager) SetLogLevel(_ context.Context, level string) error {
	level = strings.ToLower(strings.TrimSpace(level))

	if !validLogLevels[level] {
		return fmt.Errorf("유효하지 않은 로그 레벨: %q (허용: debug, info, warn, error)", level)
	}

	if m.levels == nil {
		return fmt.Errorf("로그 레벨 변경이 지원되지 않습니다")
	}

	parsed, err := observe.ParseLogLevel(level)
	if err != nil {
		return fmt.Errorf("로그 레벨 파싱 실패: %w", err)
	}

	m.levels.SetDefaultLevel(parsed)
	m.logger.Info("기본 로그 레벨 변경됨", "level", level)
	return nil
}

// levelToString 은 slog.Level 을 소문자 문자열로 변환한다.
func levelToString(level slog.Level) string {
	switch level {
	case slog.LevelDebug:
		return "debug"
	case slog.LevelInfo:
		return "info"
	case slog.LevelWarn:
		return "warn"
	case slog.LevelError:
		return "error"
	default:
		return "info"
	}
}

// ListLogLevels 는 컴포넌트별 로그 레벨 목록을 반환한다.
// LevelManager 가 설정되지 않은 경우 에러를 반환한다.
func (m *defaultMonitorManager) ListLogLevels(_ context.Context) (*LogLevelsResponse, error) {
	if m.levels == nil {
		return nil, fmt.Errorf("로그 레벨 관리가 지원되지 않습니다")
	}

	components := make(map[string]string)
	for comp, lvl := range m.levels.Levels() {
		components[comp] = levelToString(lvl)
	}

	return &LogLevelsResponse{
		DefaultLevel: levelToString(m.levels.DefaultLevel()),
		Components:   components,
	}, nil
}

// SetComponentLogLevel 은 특정 컴포넌트의 로그 레벨을 설정한다.
// LevelManager 가 설정되지 않은 경우 에러를 반환한다.
func (m *defaultMonitorManager) SetComponentLogLevel(_ context.Context, component, level string) error {
	level = strings.ToLower(strings.TrimSpace(level))

	if !validLogLevels[level] {
		return fmt.Errorf("유효하지 않은 로그 레벨: %q (허용: debug, info, warn, error)", level)
	}

	if m.levels == nil {
		return fmt.Errorf("로그 레벨 변경이 지원되지 않습니다")
	}

	parsed, err := observe.ParseLogLevel(level)
	if err != nil {
		return fmt.Errorf("로그 레벨 파싱 실패: %w", err)
	}

	m.levels.SetLevel(component, parsed)
	m.logger.Info("컴포넌트 로그 레벨 변경됨", "component", component, "level", level)
	return nil
}

// ResetComponentLogLevel 은 특정 컴포넌트의 로그 레벨을 기본값으로 초기화한다.
// LevelManager 가 설정되지 않은 경우 에러를 반환한다.
func (m *defaultMonitorManager) ResetComponentLogLevel(_ context.Context, component string) error {
	if m.levels == nil {
		return fmt.Errorf("로그 레벨 변경이 지원되지 않습니다")
	}

	m.levels.SetLevel(component, m.levels.DefaultLevel())
	m.logger.Info("컴포넌트 로그 레벨 초기화됨", "component", component)
	return nil
}
