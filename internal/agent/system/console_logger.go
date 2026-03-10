package system

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ConsoleLoggerConfig 는 ConsoleLoggerAgent의 설정을 담는 구조체이다.
type ConsoleLoggerConfig struct {
	// Prefix 는 로그 출력 시 앞에 붙는 접두어이다.
	Prefix string
	// Level 은 최소 로그 레벨이다 (기본: INFO).
	Level slog.Level
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

	return cc
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
}

// 컴파일 타임 인터페이스 체크
var _ agent.Agent = (*ConsoleLoggerAgent)(nil)

// NewConsoleLoggerAgent 는 ConsoleLoggerAgent 팩토리 함수이다.
func NewConsoleLoggerAgent(config agent.AgentConfig) (agent.Agent, error) {
	cc := parseConsoleLoggerConfig(config)

	a := &ConsoleLoggerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("console-logger")),
		logConfig:     cc,
		logger:        agent.ResolveLogger(config),
		stats:         agent.NewAgentStats(),
		createdAt:     time.Now(),
	}

	if err := a.Init(config); err != nil {
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

// Stop 은 에이전트를 정지한다.
func (a *ConsoleLoggerAgent) Stop(_ context.Context) error {
	if err := a.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("console-logger stop: %w", err)
	}
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

	a.logger.Info("message received",
		"prefix", a.logConfig.Prefix,
		"payload", string(data),
	)

	a.stats.IncrMessagesSent()
	return nil, nil
}

// Configure 는 에이전트 설정을 변경한다.
func (a *ConsoleLoggerAgent) Configure(config agent.AgentConfig) error {
	a.mu.Lock()
	a.agentConfig = config
	a.logConfig = parseConsoleLoggerConfig(config)
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
		Uptime: uptime,
	}
}

// Stats 는 에이전트의 처리 통계를 반환한다.
func (a *ConsoleLoggerAgent) Stats() agent.StatsSnapshot {
	return a.stats.Snapshot()
}
