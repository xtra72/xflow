package system

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	xflowio "github.com/xtra/xflow/internal/io"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ConsoleLoggerConfig 는 ConsoleLoggerAgent의 설정을 담는 구조체이다.
type ConsoleLoggerConfig struct {
	// Prefix 는 로그 출력 시 앞에 붙는 접두어이다.
	Prefix string
	// Level 은 최소 로그 레벨이다 (기본: INFO).
	Level slog.Level
	// Output 은 출력 대상이다. "stdout"(기본), "stderr", 또는 파일 경로.
	Output string
	// Format 은 로그 출력 형식이다. "text"(기본) 또는 "json".
	Format string
	// MaxSize 는 롤링 파일의 최대 크기(바이트). 기본값: 10MB.
	MaxSize int64
	// MaxAge 는 백업 파일 최대 보관 일수. 0이면 무제한.
	MaxAge int
	// MaxBackups 는 백업 파일 최대 개수. 0이면 무제한.
	MaxBackups int
	// Compress 는 백업 파일을 gzip 압축할지 여부이다.
	Compress bool
}

// parseConsoleLoggerConfig 는 AgentConfig에서 ConsoleLoggerConfig를 추출한다.
func parseConsoleLoggerConfig(cfg agent.AgentConfig) ConsoleLoggerConfig {
	cc := ConsoleLoggerConfig{
		Prefix: "[console-logger]",
		Level:  slog.LevelInfo,
	}

	opts := cfg.Transport.Options
	if opts == nil {
		return cc
	}

	if v, ok := opts["prefix"].(string); ok && v != "" {
		cc.Prefix = v
	}

	if v, ok := opts["output"].(string); ok && v != "" {
		cc.Output = v
	}

	if v, ok := opts["format"].(string); ok && v != "" {
		cc.Format = v
	}

	// max_size: 사용자가 MB 단위로 지정, 내부에서는 바이트로 변환
	if v, ok := opts["max_size"]; ok {
		switch n := v.(type) {
		case int:
			cc.MaxSize = int64(n) * 1024 * 1024
		case int64:
			cc.MaxSize = n * 1024 * 1024
		case float64:
			cc.MaxSize = int64(n) * 1024 * 1024
		}
	}

	// output 이 파일 경로이고 max_size 가 미지정이면 기본 10MB 설정
	if cc.Output != "" && cc.Output != "stdout" && cc.Output != "stderr" && cc.MaxSize == 0 {
		cc.MaxSize = 10 * 1024 * 1024
	}

	if v, ok := opts["max_age"]; ok {
		switch n := v.(type) {
		case int:
			cc.MaxAge = n
		case float64:
			cc.MaxAge = int(n)
		}
	}

	if v, ok := opts["max_backups"]; ok {
		switch n := v.(type) {
		case int:
			cc.MaxBackups = n
		case float64:
			cc.MaxBackups = int(n)
		}
	}

	if v, ok := opts["compress"].(bool); ok {
		cc.Compress = v
	}

	return cc
}

// resolveWriter 는 설정에 따라 적절한 writer 를 생성한다.
func resolveWriter(cc ConsoleLoggerConfig) (io.Writer, io.Closer, error) {
	switch cc.Output {
	case "", "stdout":
		return os.Stdout, nil, nil
	case "stderr":
		return os.Stderr, nil, nil
	default:
		// 파일 경로 — MaxSize > 0 이면 RollingWriter 사용
		if cc.MaxSize > 0 {
			rw, err := xflowio.NewRollingWriter(xflowio.RollingWriterConfig{
				FilePath:   cc.Output,
				MaxSize:    cc.MaxSize,
				MaxAge:     cc.MaxAge,
				MaxBackups: cc.MaxBackups,
				Compress:   cc.Compress,
			})
			return rw, rw, err
		}
		// 단순 파일 추가 모드
		dir := filepath.Dir(cc.Output)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, nil, fmt.Errorf("console-logger: 디렉터리 생성 실패: %w", err)
		}
		f, err := os.OpenFile(cc.Output, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, fmt.Errorf("console-logger: 파일 열기 실패: %w", err)
		}
		return f, f, nil
	}
}

// createLogger 는 지정된 형식과 writer 로 slog.Logger 를 생성한다.
func createLogger(w io.Writer, cc ConsoleLoggerConfig) *slog.Logger {
	opts := &slog.HandlerOptions{Level: cc.Level}
	var handler slog.Handler
	if cc.Format == "json" {
		handler = slog.NewJSONHandler(w, opts)
	} else {
		handler = slog.NewTextHandler(w, opts)
	}
	return slog.New(handler)
}

// ConsoleLoggerAgent 는 수신한 메시지를 표준 출력에 기록하는 싱크 에이전트이다.
// BridgeOut 방향의 플로우 종단에서 사용된다.
type ConsoleLoggerAgent struct {
	*lifecycle.BaseLifecycle
	agentConfig agent.AgentConfig
	logConfig   ConsoleLoggerConfig
	logger      *slog.Logger
	stats       *agent.AgentStats
	mu          sync.RWMutex
	startedAt   time.Time
	createdAt   time.Time
	writer      io.Writer // 출력 대상
	closer      io.Closer // 파일 정리용
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*ConsoleLoggerAgent)(nil)

// NewConsoleLoggerAgent 는 ConsoleLoggerAgent 팩토리 함수이다.
func NewConsoleLoggerAgent(config agent.AgentConfig) (agent.Agent, error) {
	cc := parseConsoleLoggerConfig(config)

	// writer/closer 설정
	w, c, err := resolveWriter(cc)
	if err != nil {
		return nil, fmt.Errorf("console-logger: writer 설정 실패: %w", err)
	}

	// 로거 생성: config.Logger 가 명시적으로 설정된 경우 해당 로거를 사용,
	// 그렇지 않으면 writer 기반 로거 생성
	var logger *slog.Logger
	if config.Logger != nil {
		logger = agent.ResolveLogger(config)
	} else {
		logger = createLogger(w, cc)
	}

	a := &ConsoleLoggerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("console-logger")),
		logConfig:     cc,
		logger:        logger,
		stats:         agent.NewAgentStats(),
		createdAt:     time.Now(),
		writer:        w,
		closer:        c,
	}

	if err := a.Init(config); err != nil {
		// 초기화 실패 시 파일 정리
		if c != nil {
			c.Close()
		}
		return nil, err
	}

	return a, nil
}

// Init 은 에이전트를 초기화한다.
func (a *ConsoleLoggerAgent) Init(config agent.AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("console-logger init: %w", err)
	}

	if err := a.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("console-logger init: %w", err)
	}

	a.mu.Lock()
	a.agentConfig = config
	a.mu.Unlock()

	if err := a.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("console-logger init: %w", err)
	}

	a.mu.Lock()
	a.startedAt = time.Now()
	a.mu.Unlock()

	return nil
}

// Start 는 에이전트를 시작한다. 이미 Running이면 no-op.
func (a *ConsoleLoggerAgent) Start(_ context.Context) error {
	if a.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("console-logger: not in running state (current: %s)", a.CurrentState())
}

// Stop 은 에이전트를 정지한다. closer 가 있으면 닫는다.
func (a *ConsoleLoggerAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("console-logger stop: %w", err)
	}

	// 파일 closer 정리
	a.mu.Lock()
	if a.closer != nil {
		a.closer.Close()
		a.closer = nil
	}
	a.mu.Unlock()

	return a.TransitionTo(lifecycle.StateStopped)
}

// Pause 는 에이전트를 일시정지한다.
func (a *ConsoleLoggerAgent) Pause(_ context.Context) error {
	return a.TransitionTo(lifecycle.StatePaused)
}

// Resume 은 에이전트를 재개한다.
func (a *ConsoleLoggerAgent) Resume(_ context.Context) error {
	return a.TransitionTo(lifecycle.StateRunning)
}

// Health 는 에이전트의 헬스 상태를 반환한다.
func (a *ConsoleLoggerAgent) Health() agent.HealthStatus {
	state := a.CurrentState()
	if state == lifecycle.StateRunning || state == lifecycle.StatePaused {
		return agent.HealthStatus{Status: agent.HealthHealthy}
	}
	return agent.HealthStatus{Status: agent.HealthUnhealthy}
}

// Process 는 수신한 데이터를 콘솔에 출력한다.
func (a *ConsoleLoggerAgent) Process(data []byte) ([]byte, error) {
	a.stats.IncrMessagesReceived()

	a.logger.Debug("message received",
		"prefix", a.logConfig.Prefix,
		"payload", string(data),
	)

	a.stats.IncrMessagesSent()
	return nil, nil
}

// Configure 는 에이전트 설정을 변경한다.
// 기존 closer 가 있으면 닫고, 새 writer/logger 를 설정한다.
func (a *ConsoleLoggerAgent) Configure(config agent.AgentConfig) error {
	newCC := parseConsoleLoggerConfig(config)

	// 새 writer/closer 생성
	w, c, err := resolveWriter(newCC)
	if err != nil {
		return fmt.Errorf("console-logger configure: %w", err)
	}

	// 새 로거 생성
	var logger *slog.Logger
	if config.Logger != nil {
		logger = agent.ResolveLogger(config)
	} else {
		logger = createLogger(w, newCC)
	}

	a.mu.Lock()
	// 기존 closer 정리
	if a.closer != nil {
		a.closer.Close()
	}
	a.agentConfig = config
	a.logConfig = newCC
	a.writer = w
	a.closer = c
	a.logger = logger
	a.mu.Unlock()

	return nil
}

// ID 는 에이전트의 고유 식별자를 반환한다.
func (a *ConsoleLoggerAgent) ID() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.ID
}

// Name 은 에이전트의 이름을 반환한다.
func (a *ConsoleLoggerAgent) Name() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.agentConfig.Name
}

// Type 은 에이전트의 타입을 반환한다.
func (a *ConsoleLoggerAgent) Type() string {
	return "console-logger"
}

// Info 는 에이전트의 상세 정보를 반환한다.
func (a *ConsoleLoggerAgent) Info() agent.AgentInfo {
	a.mu.RLock()
	defer a.mu.RUnlock()

	var uptime time.Duration
	if !a.startedAt.IsZero() {
		uptime = time.Since(a.startedAt)
	}

	return agent.AgentInfo{
		ID:     a.agentConfig.ID,
		Name:   a.agentConfig.Name,
		Type:   "console-logger",
		State:  a.CurrentState(),
		Config: a.agentConfig,
		Stats:  a.stats.Snapshot(),
		Uptime: uptime,
	}
}

// Stats 는 에이전트의 처리 통계를 반환한다.
func (a *ConsoleLoggerAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}
