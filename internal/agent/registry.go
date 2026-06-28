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
	// byName 은 에이전트 이름 → ID 인덱스이다. 등록/해제 시점(에이전트 메서드 호출이
	// 안전한 시점)에 한 번만 채우고, 조회(ResolveID)에서는 에이전트의 Name()/ID() 를
	// 다시 호출하지 않는다. 이는 에이전트가 자기 락을 보유한 컨텍스트(HVAC 메시지
	// 처리 등)에서 이름→ID 해석을 호출할 때 발생하던 재귀 RLock deadlock 을 방지한다.
	// 같은 이름이 중복되면 첫 등록을 유지한다(GetByName 의 "first" 의미 보존).
	byName map[string]string
}

// NewRegistry creates a new DefaultRegistry.
func NewRegistry() *DefaultRegistry {
	return &DefaultRegistry{
		agents: make(map[string]Agent),
		byName: make(map[string]string),
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
	// name→ID 인덱스 갱신 (첫 등록 우선). 조회 시 agent.Name() 재호출을 피하기 위함.
	if name := agent.Name(); name != "" {
		if _, ok := r.byName[name]; !ok {
			r.byName[name] = id
		}
	}
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
	// 이 ID 를 가리키는 byName 엔트리를 제거한다 (agent.Name() 재호출 없이).
	for name, id := range r.byName {
		if id == agentID {
			delete(r.byName, name)
		}
	}
	return nil
}

// Get returns the agent with the given ID.
func (r *DefaultRegistry) Get(agentID string) (Agent, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[agentID]
	return agent, ok
}

// ResolveID 는 ref(에이전트 ID 또는 이름)를 정본 에이전트 ID 로 해석한다.
//
// 핵심: 해석 과정에서 에이전트 메서드(Name()/ID())를 전혀 호출하지 않는다(맵 조회만).
// 따라서 에이전트가 자기 RWMutex 를 보유한 컨텍스트(예: HVAC 수신 루프의 메시지
// 처리)에서 호출해도 재귀 RLock deadlock 이 발생하지 않는다. device_id 정규화
// (ResolveDeviceID → 에이전트 ID 정규화) 경로가 이 메서드를 사용한다.
//
//   - ref 가 등록된 ID 이면 그대로 반환(agents 는 ID 키).
//   - ref 가 등록된 이름이면 byName 인덱스로 ID 반환.
//   - 그 외에는 (",", false).
func (r *DefaultRegistry) ResolveID(ref string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if _, ok := r.agents[ref]; ok {
		return ref, true
	}
	if id, ok := r.byName[ref]; ok {
		return id, true
	}
	return "", false
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
