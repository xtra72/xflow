package agent

import (
	"fmt"
	"sync"
)

// Registry manages the registration and lookup of agent instances.
type Registry interface {
	Register(agent Agent) error
	Unregister(agentID string) error
	Get(agentID string) (Agent, bool)
	GetByName(name string) (Agent, bool)
	GetByType(agentType string) []Agent
	List() []Agent
	Count() int
}

// DefaultRegistry is the default implementation of Registry.
type DefaultRegistry struct {
	mu     sync.RWMutex
	agents map[string]Agent // keyed by ID
}

// NewRegistry creates a new DefaultRegistry.
func NewRegistry() *DefaultRegistry {
	return &DefaultRegistry{
		agents: make(map[string]Agent),
	}
}

// Register adds an agent to the registry.
// Returns ErrAgentAlreadyExists if an agent with the same ID is already registered.
func (r *DefaultRegistry) Register(agent Agent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := agent.ID()
	if _, exists := r.agents[id]; exists {
		return fmt.Errorf("registry: agent %q: %w", id, ErrAgentAlreadyExists)
	}

	r.agents[id] = agent
	return nil
}

// Unregister removes an agent from the registry by ID.
// Returns ErrAgentNotFound if no agent with the given ID exists.
func (r *DefaultRegistry) Unregister(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[agentID]; !exists {
		return fmt.Errorf("registry: agent %q: %w", agentID, ErrAgentNotFound)
	}

	delete(r.agents, agentID)
	return nil
}

// Get returns the agent with the given ID.
func (r *DefaultRegistry) Get(agentID string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[agentID]
	return agent, ok
}

// GetByName returns the first agent with the given name.
func (r *DefaultRegistry) GetByName(name string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, agent := range r.agents {
		if agent.Name() == name {
			return agent, true
		}
	}
	return nil, false
}

// GetByType returns all agents of the given type.
func (r *DefaultRegistry) GetByType(agentType string) []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []Agent
	for _, agent := range r.agents {
		if agent.Type() == agentType {
			result = append(result, agent)
		}
	}
	return result
}

// List returns all registered agents.
func (r *DefaultRegistry) List() []Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Agent, 0, len(r.agents))
	for _, agent := range r.agents {
		result = append(result, agent)
	}
	return result
}

// Count returns the number of registered agents.
func (r *DefaultRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.agents)
}

// Compile-time interface check.
var _ Registry = (*DefaultRegistry)(nil)
