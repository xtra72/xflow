package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/xtra/xflow/pkg/lifecycle"
)

// Transport is the communication layer abstraction.
// System Agents do not use Transport (they return RequiresTransport() = false).
type Transport interface {
	Open(config TransportConfig) error
	Close() error
	Read(buf []byte) (int, error)
	Write(data []byte) (int, error)
	Available() bool
}

// MessageReceiver 는 비동기 메시지 수신을 지원하는 에이전트의 선택적 인터페이스이다.
// HTTP Receiver 등 외부에서 데이터를 수신하는 에이전트가 구현한다.
// agentTransportAdapter.Receive()에서 이 인터페이스 존재 여부를 확인하여 사용한다.
type MessageReceiver interface {
	ReceiveMessage(ctx context.Context) ([]byte, error)
}

// SubscriberAgent 는 동적 토픽 구독/해제를 지원하는 에이전트의 선택적 인터페이스이다.
// MQTT Subscriber 등 메시지 브로커 기반 에이전트가 구현한다.
// Bridge Node에서 이 인터페이스 존재 여부를 확인하여 토픽 관리에 사용한다.
type SubscriberAgent interface {
	Subscribe(ctx context.Context, topics []string) error
	Unsubscribe(ctx context.Context, topics []string) error
}

// StatefulAgent 는 타입별 커스텀 런타임 상태를 노출하는 에이전트가 구현하는 선택적 인터페이스이다.
// detail=full 요청 시 상태 데이터가 API 응답의 state 필드로 포함된다.
type StatefulAgent interface {
	// State 는 타입별 런타임 상태를 map 으로 반환한다.
	State() map[string]any
}

// PollingConfigurable 은 런타임 폴링 간격 변경을 지원하는 에이전트의 선택적 인터페이스이다.
// Bridge 노드 설정의 polling_interval_ms 값으로 에이전트의 폴링 주기를 오버라이드할 때 사용된다.
type PollingConfigurable interface {
	SetPollInterval(d time.Duration) error
}

// BufferInfoProvider is an optional interface for agents that have an internal message buffer.
// Implementing this interface allows the system to expose buffer utilization metrics.
type BufferInfoProvider interface {
	BufferInfo() (pending int, capacity int)
}

// Agent is the core interface for all agents in the system.
type Agent interface {
	Init(config AgentConfig) error
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Pause(ctx context.Context) error
	Resume(ctx context.Context) error
	Health() HealthStatus
	Process(data []byte) ([]byte, error)
	Configure(config AgentConfig) error
	ID() string
	Name() string
	Type() string
	Info() AgentInfo
	Stats() StatsSnapshot
}

// BaseAgent is the default implementation of the Agent interface.
// It provides lifecycle management via lifecycle.BaseLifecycle,
// optional Transport wrapping, and stats tracking.
type BaseAgent struct {
	*lifecycle.BaseLifecycle
	config    AgentConfig
	transport Transport    // may be nil for System Agents
	stats     *AgentStats  // internal stats (use atomic helpers)
	mu        sync.RWMutex
	startedAt time.Time
	createdAt time.Time
}

// NewBaseAgent creates a new BaseAgent with default configuration.
func NewBaseAgent() *BaseAgent {
	return &BaseAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("base-agent")),
		stats:         NewAgentStats(),
		createdAt:     time.Now(),
	}
}

// Init validates the config, stores it, and transitions Created -> Initializing -> Running.
func (ba *BaseAgent) Init(config AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("agent init: %w", err)
	}

	// Created -> Initializing
	if err := ba.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("agent init: %w", err)
	}

	ba.mu.Lock()
	ba.config = config
	ba.mu.Unlock()

	// Open transport if available
	if ba.transport != nil {
		if err := ba.transport.Open(config.Transport); err != nil {
			// Transition to error state on transport failure
			_ = ba.TransitionTo(lifecycle.StateError)
			return fmt.Errorf("agent init: transport open failed: %w", err)
		}
	}

	// Initializing -> Running
	if err := ba.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("agent init: %w", err)
	}

	ba.mu.Lock()
	ba.startedAt = time.Now()
	ba.mu.Unlock()

	return nil
}

// Start is a no-op if the agent is already running.
func (ba *BaseAgent) Start(_ context.Context) error {
	if ba.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("agent start: agent is not in running state (current: %s)", ba.CurrentState())
}

// Stop closes the transport if present and transitions to Stopping -> Stopped.
func (ba *BaseAgent) Stop(_ context.Context) error {
	if err := ba.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("agent stop: %w", err)
	}

	// Close transport if present
	if ba.transport != nil {
		if err := ba.transport.Close(); err != nil {
			// Log but continue stopping
			_ = err
		}
	}

	if err := ba.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("agent stop: %w", err)
	}

	return nil
}

// Pause transitions Running -> Paused.
func (ba *BaseAgent) Pause(_ context.Context) error {
	if err := ba.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("agent pause: %w", err)
	}
	return nil
}

// Resume transitions Paused -> Running.
func (ba *BaseAgent) Resume(_ context.Context) error {
	if err := ba.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("agent resume: %w", err)
	}
	return nil
}

// Health returns a HealthStatus based on the current agent state.
func (ba *BaseAgent) Health() HealthStatus {
	now := time.Now()
	state := ba.CurrentState()

	switch state {
	case lifecycle.StateRunning:
		return HealthStatus{
			Status:    HealthHealthy,
			LastCheck: now,
			Message:   "agent is running",
		}
	case lifecycle.StatePaused:
		return HealthStatus{
			Status:    HealthDegraded,
			LastCheck: now,
			Message:   "agent is paused",
		}
	default:
		return HealthStatus{
			Status:    HealthUnhealthy,
			LastCheck: now,
			Message:   fmt.Sprintf("agent is in %s state", state),
		}
	}
}

// Process delegates to the transport if available. If no transport is set,
// returns ErrTransportNotAvailable.
func (ba *BaseAgent) Process(data []byte) ([]byte, error) {
	if ba.transport == nil {
		return nil, ErrTransportNotAvailable
	}

	// Write data to transport
	n, err := ba.transport.Write(data)
	if err != nil {
		ba.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("agent process: write failed: %w", err)
	}
	ba.stats.AddBytesWritten(int64(n))
	ba.stats.IncrMessagesSent()

	// Read response from transport
	buf := make([]byte, ba.config.BufferSize)
	n, err = ba.transport.Read(buf)
	if err != nil {
		ba.stats.IncrMessagesErrored()
		return nil, fmt.Errorf("agent process: read failed: %w", err)
	}
	ba.stats.AddBytesRead(int64(n))
	ba.stats.IncrMessagesReceived()
	ba.stats.UpdateLastActivity()

	return buf[:n], nil
}

// Configure validates and updates the agent configuration.
// Refuses Transport.Type change when the agent is Running.
func (ba *BaseAgent) Configure(config AgentConfig) error {
	if err := config.Validate(); err != nil {
		return fmt.Errorf("agent configure: %w", err)
	}

	ba.mu.Lock()
	defer ba.mu.Unlock()

	// Check immutable fields when running
	state := ba.CurrentState()
	if state == lifecycle.StateRunning || state == lifecycle.StatePaused {
		if ba.config.Transport.Type != "" && config.Transport.Type != ba.config.Transport.Type {
			return fmt.Errorf("agent configure: transport type cannot be changed while running: %w", ErrConfigImmutable)
		}
	}

	ba.config = config
	return nil
}

// ID returns the agent's unique identifier.
func (ba *BaseAgent) ID() string {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return ba.config.ID
}

// Name returns the agent's human-readable name.
func (ba *BaseAgent) Name() string {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return ba.config.Name
}

// Type returns the agent's type.
func (ba *BaseAgent) Type() string {
	ba.mu.RLock()
	defer ba.mu.RUnlock()
	return ba.config.Type
}

// Info builds and returns an AgentInfo snapshot.
func (ba *BaseAgent) Info() AgentInfo {
	ba.mu.RLock()
	cfg := ba.config
	startedAt := ba.startedAt
	createdAt := ba.createdAt
	ba.mu.RUnlock()

	state := ba.CurrentState()
	var uptime time.Duration
	if state == lifecycle.StateRunning && !startedAt.IsZero() {
		uptime = time.Since(startedAt)
	}

	return AgentInfo{
		ID:        cfg.ID,
		Name:      cfg.Name,
		Type:      cfg.Type,
		State:     state,
		Health:    ba.Health(),
		Config:    cfg,
		Stats:     ba.stats.Snapshot(),
		StartedAt: startedAt,
		Uptime:    uptime,
		CreatedAt: createdAt,
	}
}

// Stats returns a snapshot of the current processing statistics.
func (ba *BaseAgent) Stats() StatsSnapshot {
	return ba.stats.Snapshot()
}

// Compile-time interface check.
var _ Agent = (*BaseAgent)(nil)
