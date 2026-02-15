package system

import (
	"context"
	"fmt"
	"sync"

	"github.com/xtra/xflow/internal/agent"
)

// SystemConfig holds configuration for the SystemAgentManager.
type SystemConfig struct {
	EventBufferSize int    // Event channel buffer size (default: 256)
	FileSandboxRoot string // Root directory for file agent sandbox
	LogDefaultLevel string // Default log level (reserved for future use)
}

// SystemAgentManager coordinates the lifecycle of system agents (Event, File).
// Existing agents (Store, Timer, Logger) manage their own lifecycle externally.
type SystemAgentManager struct {
	mu          sync.Mutex
	initialized bool
	started     bool
	event       *EventAgentImpl
	file        *FileAgentImpl
}

// NewSystemAgentManager creates a new SystemAgentManager.
func NewSystemAgentManager() *SystemAgentManager {
	return &SystemAgentManager{}
}

// Initialize creates and initializes Event and File agents.
func (m *SystemAgentManager) Initialize(config SystemConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.initialized {
		return ErrAlreadyInitialized
	}

	bufSize := config.EventBufferSize
	if bufSize <= 0 {
		bufSize = defaultEventBufferSize
	}

	// Create Event agent
	eventAgent := NewEventAgent(bufSize)
	eventCfg := agent.AgentConfig{
		ID:   "system-event",
		Name: "system-event-agent",
		Type: string(agent.SystemAgentEvent),
	}
	if err := eventAgent.Init(eventCfg); err != nil {
		return fmt.Errorf("system manager: event agent init failed: %w", err)
	}

	// Create File agent
	fileAgent := NewFileAgent(config.FileSandboxRoot)
	fileCfg := agent.AgentConfig{
		ID:   "system-file",
		Name: "system-file-agent",
		Type: string(agent.SystemAgentFile),
	}
	if err := fileAgent.Init(fileCfg); err != nil {
		// Rollback: stop the event agent
		_ = eventAgent.Stop(context.Background())
		return fmt.Errorf("system manager: file agent init failed: %w", err)
	}

	m.event = eventAgent
	m.file = fileAgent
	m.initialized = true

	return nil
}

// Start transitions all managed agents to an active state.
func (m *SystemAgentManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return ErrNotInitialized
	}

	// Start Event agent
	if err := m.event.Start(ctx); err != nil {
		return fmt.Errorf("system manager: event agent start failed: %w", err)
	}

	// Start File agent
	if err := m.file.Start(ctx); err != nil {
		// Rollback: stop event agent
		_ = m.event.Stop(ctx)
		return fmt.Errorf("system manager: file agent start failed: %w", err)
	}

	m.started = true
	return nil
}

// Stop stops all managed agents in reverse order (File first, then Event).
func (m *SystemAgentManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return ErrNotInitialized
	}

	var firstErr error

	// Stop File agent first (reverse order)
	if m.file != nil {
		if err := m.file.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("system manager: file agent stop failed: %w", err)
		}
	}

	// Stop Event agent
	if m.event != nil {
		if err := m.event.Stop(ctx); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("system manager: event agent stop failed: %w", err)
		}
	}

	m.started = false

	return firstErr
}

// Event returns the event agent, or nil if not initialized.
func (m *SystemAgentManager) Event() *EventAgentImpl {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.event
}

// File returns the file agent, or nil if not initialized.
func (m *SystemAgentManager) File() *FileAgentImpl {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.file
}

// IsInitialized returns whether the manager has been initialized.
func (m *SystemAgentManager) IsInitialized() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.initialized
}

// IsStarted returns whether the manager has been started.
func (m *SystemAgentManager) IsStarted() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.started
}
