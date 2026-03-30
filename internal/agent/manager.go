package agent

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

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
	SetAgentLogLevel(agentID, level string) error
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

// WithOnRestart 는 에이전트 Restart() 완료 후 호출되는 콜백을 등록한다.
// 콜백은 새로 생성된 에이전트 인스턴스를 전달받는다.
func WithOnRestart(fn func(Agent)) ManagerOption {
	return func(m *DefaultManager) {
		m.onRestart = append(m.onRestart, fn)
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
	onStart   []func(Agent)     // 에이전트 시작 후 호출되는 훅
	onStop    []func(Agent)     // 에이전트 중지 전 호출되는 훅
	onRestart []func(Agent)     // 에이전트 재시작 후 호출되는 훅
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
	m.runOnStartHooks(agent)
	return nil
}

// NotifyStarted 는 매니저 외부에서 직접 시작된 에이전트에 대해
// onStart 훅을 실행한다. 엔진의 autoStartAgents 등에서 사용된다.
func (m *DefaultManager) NotifyStarted(a Agent) {
	m.runOnStartHooks(a)
}

func (m *DefaultManager) runOnStartHooks(a Agent) {
	for _, fn := range m.onStart {
		fn(a)
	}
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

// Restart stops and then re-creates the agent with the given ID.
// Agent 인터페이스만 사용하므로 BaseAgent가 아닌 구현체(NASAAgent 등)도 지원한다.
func (m *DefaultManager) Restart(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	old, exists := m.agents[agentID]
	if !exists {
		return fmt.Errorf("manager restart: agent %q: %w", agentID, ErrAgentNotFound)
	}

	// 현재 설정을 Info()에서 가져온다 (인터페이스 기반).
	cfg := old.Info().Config

	// 기존 에이전트 정지
	for _, fn := range m.onStop {
		fn(old)
	}
	if err := old.Stop(ctx); err != nil {
		return fmt.Errorf("manager restart: stop failed: %w", err)
	}

	// Registry에서 제거
	_ = m.registry.Unregister(agentID)

	// TypeRegistry로 새 인스턴스 생성 (ID 유지)
	cfg.ID = agentID
	var newAgent Agent
	var err error
	if m.typeReg.HasType(cfg.Type) {
		newAgent, err = m.typeReg.CreateAgent(cfg.Type, cfg)
	} else {
		ba := NewBaseAgent()
		err = ba.Init(cfg)
		newAgent = ba
	}
	if err != nil {
		return fmt.Errorf("manager restart: re-create failed: %w", err)
	}

	// 옵저버 주입
	if m.observer != nil {
		if configurable, ok := newAgent.(interface{ SetObserver(*observe.Observer) }); ok {
			configurable.SetObserver(m.observer)
		}
	}

	// Registry에 등록하고 agents map 교체
	if err := m.registry.Register(newAgent); err != nil {
		return fmt.Errorf("manager restart: re-register failed: %w", err)
	}
	m.agents[agentID] = newAgent

	// 시작 훅 실행
	for _, fn := range m.onStart {
		fn(newAgent)
	}

	// 재시작 훅 실행 (노드 재초기화 등)
	for _, fn := range m.onRestart {
		fn(newAgent)
	}

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

// SetAgentLogLevel 은 에이전트의 로그 레벨을 런타임에 변경한다.
// Observer 의 LevelManager 를 통해 즉시 적용된다.
func (m *DefaultManager) SetAgentLogLevel(agentID, level string) error {
	m.mu.RLock()
	ag, exists := m.agents[agentID]
	m.mu.RUnlock()

	if !exists {
		return ErrAgentNotFound
	}

	lvl, ok := parseLogLevel(level)
	if !ok {
		return fmt.Errorf("invalid log level: %q", level)
	}

	if m.observer != nil {
		info := ag.Info()
		component := fmt.Sprintf("agent.%s.%s", info.Type, info.Config.Name)
		m.observer.Levels.SetLevel(component, lvl)
	}

	return nil
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
