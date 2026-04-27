package agent

import (
	"fmt"
	"sync"
)

// AgentFactory is a function that creates an Agent from a given configuration.
type AgentFactory func(config AgentConfig) (Agent, error)

// TypeRegistry manages the registration of agent types and their factories.
type TypeRegistry interface {
	RegisterType(agentType string, factory AgentFactory) error
	CreateAgent(agentType string, config AgentConfig) (Agent, error)
	ListTypes() []string
	HasType(agentType string) bool
}

// DefaultTypeRegistry is the default implementation of TypeRegistry.
type DefaultTypeRegistry struct {
	mu        sync.RWMutex
	factories map[string]AgentFactory
}

// NewTypeRegistry creates a new DefaultTypeRegistry.
func NewTypeRegistry() *DefaultTypeRegistry {
	return &DefaultTypeRegistry{
		factories: make(map[string]AgentFactory),
	}
}

// RegisterType registers an agent type with its factory function.
// Returns an error if the type is already registered.
func (r *DefaultTypeRegistry) RegisterType(agentType string, factory AgentFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.factories[agentType]; exists {
		return fmt.Errorf("agent type %q already registered", agentType)
	}

	r.factories[agentType] = factory
	return nil
}

// CreateAgent creates an agent of the specified type using the registered factory.
// Returns an error if the type is not registered.
func (r *DefaultTypeRegistry) CreateAgent(agentType string, config AgentConfig) (Agent, error) {
	r.mu.RLock()
	factory, exists := r.factories[agentType]
	r.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("agent type %q not registered: %w", agentType, ErrTransportNotAvailable)
	}

	return factory(config)
}

// ListTypes returns a list of all registered agent type names.
func (r *DefaultTypeRegistry) ListTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.factories))
	for t := range r.factories {
		types = append(types, t)
	}
	return types
}

// HasType returns true if the given agent type is registered.
func (r *DefaultTypeRegistry) HasType(agentType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, exists := r.factories[agentType]
	return exists
}

// DefaultTypeReg is the package-level default type registry.
var DefaultTypeReg = NewTypeRegistry()
