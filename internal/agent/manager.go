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
	mu        sync.RWMutex
	registry  *DefaultRegistry
	agents    map[string]Agent // ordered tracking by ID
	order     []string         // creation order for shutdown
	typeReg   *DefaultTypeRegistry
	observer  *observe.Observer // Observer 기반 로거 주입용 (nil 허용)
	onStart   []func(Agent)     // 에이전트 시작 후 호출되는 훅
	onStop    []func(Agent)     // 에이전트 중지 전 호출되는 훅
	onRestart []func(Agent)     // 에이전트 재시작 후 호출되는 훅

	// v0.7.6: Restart 직렬화 전용 mutex. 동일 agentID 에 대한 동시 Restart 를 방지하되,
	// 본 lock 은 mu 와 분리되어 있어 Stop/Init/Start 의 긴 호출 동안 mu 를 holding
	// 하지 않는다. 결과적으로 ListAgents 등 read API 가 block 되지 않는다.
	restartMu sync.Mutex
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
// 이미 Stopped 상태이면 no-op (2026-05-14 hotfix: lifecycle 의 Stopped → Stopping
// 전이가 invalid 이므로 idempotent Stop 보장. 두번째 Stop API 호출 또는 Restart
// 내부의 Stop 호출에서 회귀 방지).
func (m *DefaultManager) Stop(ctx context.Context, agentID string) error {
	agent, err := m.getAgent(agentID)
	if err != nil {
		return err
	}
	// StateStopped 만으로 early-return 하면, Paho 가 이전 Stop 이후 재연결에 성공해
	// 트랜스포트는 살아있는데 라이프사이클만 Stopped 로 남은 desync 상황에서 UI Stop 이
	// no-op 이 되어 에이전트를 끊을 수 없게 된다. 따라서 트랜스포트가 여전히 연결되어
	// 있는지 TransportChecker 로 확인하여, 진짜 정지(State==Stopped && !연결)일 때만
	// early-return 하고, 연결이 남아 있으면 agent.Stop() 을 호출해 강제 disconnect 한다.
	if agent.Info().State == lifecycle.StateStopped {
		stillConnected := false
		if tc, ok := agent.(TransportChecker); ok {
			stillConnected = tc.TransportConnected()
		}
		if !stillConnected {
			return nil
		}
	}
	for _, fn := range m.onStop {
		fn(agent)
	}
	return agent.Stop(ctx)
}

// Restart stops and then re-creates the agent with the given ID.
// Agent 인터페이스만 사용하므로 BaseAgent가 아닌 구현체(Hvacr01Agent 등)도 지원한다.
func (m *DefaultManager) Restart(ctx context.Context, agentID string) error {
	// v0.7.6: 동시 Restart 는 restartMu 로 직렬화. m.mu 와 분리하여 다른 read API
	// (ListAgents 등) 가 Restart 진행 중에도 응답할 수 있도록 한다.
	//
	// 이전 구현은 m.mu.Lock() 을 함수 끝까지 holding 하여, old.Stop / newAgent.Start
	// 가 외부 I/O (InfluxDB Health, transport close 등) 에서 5-10초 hang 하는 동안
	// /api/v1/agents 같은 read 가 모두 block 되는 문제가 있었다.
	m.restartMu.Lock()
	defer m.restartMu.Unlock()

	// 1. m.mu 짧게 잡고 old agent 만 가져온다.
	m.mu.RLock()
	old, exists := m.agents[agentID]
	m.mu.RUnlock()
	if !exists {
		return fmt.Errorf("manager restart: agent %q: %w", agentID, ErrAgentNotFound)
	}

	// 현재 설정을 Info()에서 가져온다 (인터페이스 기반).
	cfg := old.Info().Config
	cfg.ID = agentID

	// 2. lock 없이 onStop 훅 + Stop 호출 (외부 I/O 발생 가능).
	for _, fn := range m.onStop {
		fn(old)
	}
	// 기존 에이전트 정지 (이미 Stopped 상태면 Stop 호출 생략 — invalid lifecycle 전이 방지)
	// 2026-05-14 hotfix: Stopped → Stopping 전이가 invalid 라서 사용자가 설정 변경 후
	// 정지된 에이전트를 Restart 시 stop 단계에서 회귀 발생하던 문제 해소.
	if old.Info().State != lifecycle.StateStopped {
		if err := old.Stop(ctx); err != nil {
			return fmt.Errorf("manager restart: stop failed: %w", err)
		}
	}

	// 3. lock 없이 새 인스턴스 생성 (Init 의 헬스체크 등 외부 I/O 발생 가능).
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

	// 4. lock 없이 새 에이전트 Start (트랜스포트 열기 등 외부 I/O).
	if err := newAgent.Start(ctx); err != nil {
		return fmt.Errorf("manager restart: start failed: %w", err)
	}

	// 5. m.mu 짧게 잡고 registry/agents map swap.
	m.mu.Lock()
	_ = m.registry.Unregister(agentID)
	if err := m.registry.Register(newAgent); err != nil {
		m.mu.Unlock()
		// rollback: 새 에이전트 정지 (외부 I/O 발생 가능하므로 lock 밖에서).
		_ = newAgent.Stop(ctx)
		return fmt.Errorf("manager restart: re-register failed: %w", err)
	}
	m.agents[agentID] = newAgent
	m.mu.Unlock()

	// 6. lock 없이 onStart / onRestart 훅 실행 (Bridge 재초기화 등).
	for _, fn := range m.onStart {
		fn(newAgent)
	}
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

// ResolveAgentID 는 ref(에이전트 이름 또는 ID)를 정본 에이전트 ID 로 변환한다.
// ref 가 등록된 에이전트 "이름" 이면 그 에이전트의 ID 와 true 를 반환하고,
// 이미 ID 이거나 못 찾으면 ("", false) 를 반환한다 (호출자가 ref 폴백).
//
// device_id / device_info 정규화(SetAgentIDResolver)에 와이어링하기 위한 진입점.
//
// registry.ResolveID 는 ID/이름 인덱스 맵 조회만 수행하고 에이전트 메서드
// (Name()/ID())를 호출하지 않는다. 이는 HVAC 에이전트가 자기 RWMutex 를 보유한
// 컨텍스트(수신 루프의 메시지 처리 등)에서 device_id 정규화를 호출할 때 발생하던
// 재귀 RLock deadlock 을 방지한다. (이전 구현은 GetByName 이 에이전트 Name() 을
// 순회 호출하여 호출 에이전트의 RLock 을 재귀적으로 요구했다.)
func (m *DefaultManager) ResolveAgentID(ref string) (string, bool) {
	if ref == "" {
		return "", false
	}
	return m.registry.ResolveID(ref)
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
