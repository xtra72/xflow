package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

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

// DefaultManager is the default implementation of the Manager interface.
type DefaultManager struct {
	mu       sync.RWMutex
	registry *DefaultRegistry
	agents   map[string]Agent // ordered tracking by ID
	order    []string         // creation order for shutdown
	typeReg  *DefaultTypeRegistry
}

// NewManager creates a new DefaultManager.
func NewManager() *DefaultManager {
	return &DefaultManager{
		registry: NewRegistry(),
		agents:   make(map[string]Agent),
		typeReg:  NewTypeRegistry(),
	}
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

	var agent Agent
	var err error

	// Try TypeRegistry first
	if m.typeReg.HasType(config.Type) {
		agent, err = m.typeReg.CreateAgent(config.Type, config)
		if err != nil {
			return nil, fmt.Errorf("manager create: factory error: %w", err)
		}
	} else {
		// Default: create BaseAgent
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
	return agent.Start(ctx)
}

// Stop stops the agent with the given ID.
func (m *DefaultManager) Stop(ctx context.Context, agentID string) error {
	agent, err := m.getAgent(agentID)
	if err != nil {
		return err
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

// Compile-time interface check.
var _ Manager = (*DefaultManager)(nil)
