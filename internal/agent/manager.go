package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// Manager manages the lifecycle of agents (create, start, stop, restart, delete).
type Manager interface {
	Create(config AgentConfig) (Agent, error)
	Start(ctx context.Context, agentID string) error
	Stop(ctx context.Context, agentID string) error
	Restart(ctx context.Context, agentID string) error
	Delete(agentID string) error
	Get(agentID string) (Agent, error)
	List() []Agent
	Shutdown(ctx context.Context) error
	Summary() ManagerSummary
}

// ManagerOption 은 DefaultManager 생성 시 설정을 변경하는 옵션 함수이다.
type ManagerOption func(*DefaultManager)

// WithObserver 는 Manager 에 Observer 를 주입하여 에이전트 생성 시 컴포넌트 로거를 제공한다.
func WithObserver(obs *observe.Observer) ManagerOption {
	return func(m *DefaultManager) {
		m.observer = obs
	}
}

// WithOnStart 는 에이전트 Start() 성공 후 호출되는 콜백을 등록한다.
func WithOnStart(fn func(Agent)) ManagerOption {
	return func(m *DefaultManager) {
		m.onStart = append(m.onStart, fn)
	}
}

// WithOnStop 는 에이전트 Stop() 전에 호출되는 콜백을 등록한다.
func WithOnStop(fn func(Agent)) ManagerOption {
	return func(m *DefaultManager) {
		m.onStop = append(m.onStop, fn)
	}
}

// DefaultManager is the default implementation of the Manager interface.
type DefaultManager struct {
	mu       sync.RWMutex
	registry *DefaultRegistry
	agents   map[string]Agent // ordered tracking by ID
	order    []string         // creation order for shutdown
	typeReg  *DefaultTypeRegistry
	observer *observe.Observer // Observer 기반 로거 주입용 (nil 허용)
	onStart  []func(Agent)     // 에이전트 시작 후 호출되는 훅
	onStop   []func(Agent)     // 에이전트 중지 전 호출되는 훅
}

// NewManager creates a new DefaultManager.
func NewManager(opts ...ManagerOption) *DefaultManager {
	m := &DefaultManager{
		registry: NewRegistry(),
		agents:   make(map[string]Agent),
		typeReg:  NewTypeRegistry(),
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// Create validates the config, creates a new agent instance, and registers it.
// If a type is registered in the TypeRegistry, the registered factory is used.
// Otherwise, a default BaseAgent is created.
func (m *DefaultManager) Create(config AgentConfig) (Agent, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("manager create: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check for duplicate ID
	if _, exists := m.agents[config.ID]; exists {
		return nil, fmt.Errorf("manager create: agent %q: %w", config.ID, ErrAgentAlreadyExists)
	}

	// Observer 가 있으면 컴포넌트 로거를 생성하여 config 에 주입한다.
	if m.observer != nil && config.Type != "" {
		component := fmt.Sprintf("agent.%s.%s", config.Type, config.Name)
		componentLogger := m.observer.Loggers.NewLogger(component)
		config.Logger = componentLogger.Logger()

		// 로그 레벨이 지정되었으면 Observer 에 등록한다.
		if config.LogLevel != "" {
			if lvl, ok := parseLogLevel(config.LogLevel); ok {
				m.observer.Levels.SetLevel(component, lvl)
			}
		}
	}

	var agent Agent
	var err error

	// Try TypeRegistry first
	if m.typeReg.HasType(config.Type) {
		agent, err = m.typeReg.CreateAgent(config.Type, config)
		if err != nil {
			return nil, fmt.Errorf("manager create: factory error: %w", err)
		}
	} else if config.Type != "" {
		// 타입이 지정되었지만 등록되지 않은 경우 즉시 에러를 반환한다.
		// BaseAgent로 폴백하면 transport 없이 생성되어 이후 Process() 호출 시
		// 디버깅이 어려운 ErrTransportNotAvailable 에러가 발생한다.
		return nil, fmt.Errorf("manager create: agent type %q: %w", config.Type, ErrTransportNotAvailable)
	} else {
		// 타입 미지정: BaseAgent 기본 생성
		ba := NewBaseAgent()
		if err := ba.Init(config); err != nil {
			return nil, fmt.Errorf("manager create: %w", err)
		}
		agent = ba
	}

	// Register in registry
	if err := m.registry.Register(agent); err != nil {
		return nil, fmt.Errorf("manager create: %w", err)
	}

	m.agents[config.ID] = agent
	m.order = append(m.order, config.ID)

	return agent, nil
}

// Start starts the agent with the given ID.
func (m *DefaultManager) Start(ctx context.Context, agentID string) error {
	agent, err := m.getAgent(agentID)
	if err != nil {
		return err
	}
	if err := agent.Start(ctx); err != nil {
		return err
	}
	for _, fn := range m.onStart {
		fn(agent)
	}
	return nil
}

// Stop stops the agent with the given ID.
func (m *DefaultManager) Stop(ctx context.Context, agentID string) error {
	agent, err := m.getAgent(agentID)
	if err != nil {
		return err
	}
	for _, fn := range m.onStop {
		fn(agent)
	}
	return agent.Stop(ctx)
}

// Restart stops and then re-initializes the agent with the given ID.
func (m *DefaultManager) Restart(ctx context.Context, agentID string) error {
	agent, err := m.getAgent(agentID)
	if err != nil {
		return err
	}

	// Get current config before stopping
	ba, ok := agent.(*BaseAgent)
	if !ok {
		return fmt.Errorf("manager restart: agent %q is not a BaseAgent", agentID)
	}

	ba.mu.RLock()
	cfg := ba.config
	ba.mu.RUnlock()

	// Stop the agent
	if err := agent.Stop(ctx); err != nil {
		return fmt.Errorf("manager restart: stop failed: %w", err)
	}

	// Re-create the lifecycle for fresh state
	ba.BaseLifecycle = lifecycle.NewBaseLifecycle(lifecycle.WithName(cfg.Name))
	ba.mu.Lock()
	ba.startedAt = time.Time{}
	ba.mu.Unlock()

	// Re-initialize
	if err := ba.Init(cfg); err != nil {
		return fmt.Errorf("manager restart: re-init failed: %w", err)
	}

	// Increment restart count
	ba.stats.IncrRestartCount()

	return nil
}

// Delete removes the agent from the manager.
// If the agent is running, it is stopped first.
func (m *DefaultManager) Delete(agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return fmt.Errorf("manager delete: agent %q: %w", agentID, ErrAgentNotFound)
	}

	// Stop if running or paused
	ba, ok := agent.(*BaseAgent)
	if ok {
		state := ba.CurrentState()
		if state == lifecycle.StateRunning || state == lifecycle.StatePaused {
			for _, fn := range m.onStop {
				fn(agent)
			}
			if err := agent.Stop(context.Background()); err != nil {
				// Log but continue deletion
				_ = err
			}
		}
	}

	// Remove from registry
	_ = m.registry.Unregister(agentID)

	// Remove from internal tracking
	delete(m.agents, agentID)
	m.removeFromOrder(agentID)

	return nil
}

// Get returns the agent with the given ID.
func (m *DefaultManager) Get(agentID string) (Agent, error) {
	return m.getAgent(agentID)
}

// List returns all managed agents.
func (m *DefaultManager) List() []Agent {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]Agent, 0, len(m.agents))
	for _, agent := range m.agents {
		result = append(result, agent)
	}
	return result
}

// Shutdown stops all running agents in reverse creation order.
func (m *DefaultManager) Shutdown(ctx context.Context) error {
	m.mu.RLock()
	// Copy the order in reverse
	reversed := make([]string, len(m.order))
	for i, id := range m.order {
		reversed[len(m.order)-1-i] = id
	}
	m.mu.RUnlock()

	var lastErr error
	for _, id := range reversed {
		agent, err := m.getAgent(id)
		if err != nil {
			continue
		}

		ba, ok := agent.(*BaseAgent)
		if !ok {
			continue
		}

		state := ba.CurrentState()
		if state == lifecycle.StateRunning || state == lifecycle.StatePaused {
			for _, fn := range m.onStop {
				fn(agent)
			}
			if err := agent.Stop(ctx); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// Summary returns an aggregate overview of all managed agents.
func (m *DefaultManager) Summary() ManagerSummary {
	m.mu.RLock()
	defer m.mu.RUnlock()

	summary := ManagerSummary{
		TotalAgents: len(m.agents),
		Agents:      make([]AgentInfo, 0, len(m.agents)),
	}

	for _, agent := range m.agents {
		info := agent.Info()
		summary.Agents = append(summary.Agents, info)

		switch info.State {
		case lifecycle.StateRunning:
			summary.RunningAgents++
		case lifecycle.StatePaused:
			summary.PausedAgents++
		case lifecycle.StateStopped:
			summary.StoppedAgents++
		case lifecycle.StateError:
			summary.ErrorAgents++
		}

		if info.Health.Status == HealthHealthy {
			summary.HealthyAgents++
		} else if info.Health.Status == HealthUnhealthy {
			summary.UnhealthyAgents++
		}

		summary.TotalMessagesProcessed += info.Stats.MessagesReceived + info.Stats.MessagesSent
		summary.TotalErrors += info.Stats.MessagesErrored
	}

	return summary
}

// getAgent retrieves an agent by ID with proper locking.
func (m *DefaultManager) getAgent(agentID string) (Agent, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("manager: agent %q: %w", agentID, ErrAgentNotFound)
	}
	return agent, nil
}

// removeFromOrder removes an ID from the creation order slice.
func (m *DefaultManager) removeFromOrder(agentID string) {
	for i, id := range m.order {
		if id == agentID {
			m.order = append(m.order[:i], m.order[i+1:]...)
			return
		}
	}
}

// RegisterType 은 에이전트 타입과 팩토리를 내부 TypeRegistry에 등록한다.
func (m *DefaultManager) RegisterType(agentType string, factory AgentFactory) error {
	return m.typeReg.RegisterType(agentType, factory)
}

// parseLogLevel 은 문자열 로그 레벨을 slog.Level 로 변환한다.
func parseLogLevel(level string) (slog.Level, bool) {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn", "warning":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	default:
		return slog.LevelInfo, false
	}
}

// Compile-time interface check.
var _ Manager = (*DefaultManager)(nil)
