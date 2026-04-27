package system

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/observe"
)

// SystemConfig holds configuration for the SystemAgentManager.
type SystemConfig struct {
	EventBufferSize  int           // Event channel buffer size (default: 256)
	FileSandboxRoot  string        // Root directory for file agent sandbox
	LogDefaultLevel  slog.Level    // Default log level (default: slog.LevelInfo)
	StoreDefaultTTL  time.Duration // Default TTL for store entries (default: 0, unlimited)
	TimerMinInterval time.Duration    // Minimum interval for timer agent (default: 100ms)
	Observer         *observe.Observer // Observer instance for logger agent (optional)
}

// SystemAgentManager coordinates the lifecycle of all five system agents:
// Event, Store, Timer, Logger, and File.
type SystemAgentManager struct {
	mu          sync.Mutex
	initialized bool
	started     bool
	event       *EventAgentImpl
	store       *StoreAgent
	timer       *TimerAgent
	logger      *LoggerAgent
	file        *FileAgentImpl
}

// NewSystemAgentManager creates a new SystemAgentManager.
func NewSystemAgentManager() *SystemAgentManager {
	return &SystemAgentManager{}
}

// initStep describes one agent's init action and its corresponding rollback (stop).
type initStep struct {
	name string
	init func() error
	stop func(context.Context) error
}

// Initialize creates and initializes all five system agents in order:
// Event -> Store -> Timer -> Logger -> File.
// If the Nth agent fails, agents 1..(N-1) are stopped in reverse order.
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

	timerInterval := config.TimerMinInterval
	if timerInterval <= 0 {
		timerInterval = 100 * time.Millisecond
	}

	// Create agents (creation does not fail)
	eventAgent := NewEventAgent(bufSize)
	storeAgent := NewStoreAgent(WithDefaultTTL(config.StoreDefaultTTL))
	timerAgent := NewTimerAgent(WithMinInterval(timerInterval))
	loggerOpts := []LoggerOption{WithLogDefaultLevel(config.LogDefaultLevel)}
	if config.Observer != nil {
		loggerOpts = append(loggerOpts, WithLogObserver(config.Observer))
	}
	loggerAgent := NewLoggerAgent(loggerOpts...)
	fileAgent := NewFileAgent(config.FileSandboxRoot)

	steps := []initStep{
		{
			name: "event",
			init: func() error {
				return eventAgent.Init(agent.AgentConfig{
					ID:   "system-event",
					Name: "system-event-agent",
					Type: string(agent.SystemAgentEvent),
				})
			},
			stop: func(ctx context.Context) error { return eventAgent.Stop(ctx) },
		},
		{
			name: "store",
			init: func() error { return storeAgent.Init(context.Background()) },
			stop: func(ctx context.Context) error { return storeAgent.Stop(ctx) },
		},
		{
			name: "timer",
			init: func() error { return timerAgent.Init(context.Background()) },
			stop: func(ctx context.Context) error { return timerAgent.Stop(ctx) },
		},
		{
			name: "logger",
			init: func() error { return loggerAgent.Init(context.Background()) },
			stop: func(ctx context.Context) error { return loggerAgent.Stop(ctx) },
		},
		{
			name: "file",
			init: func() error {
				return fileAgent.Init(agent.AgentConfig{
					ID:   "system-file",
					Name: "system-file-agent",
					Type: string(agent.SystemAgentFile),
				})
			},
			stop: func(ctx context.Context) error { return fileAgent.Stop(ctx) },
		},
	}

	for i, step := range steps {
		if err := step.init(); err != nil {
			// Rollback: stop agents 0..(i-1) in reverse order
			for j := i - 1; j >= 0; j-- {
				_ = steps[j].stop(context.Background())
			}
			return fmt.Errorf("system manager: %s agent init failed: %w", step.name, ErrAgentInitFailed)
		}
	}

	m.event = eventAgent
	m.store = storeAgent
	m.timer = timerAgent
	m.logger = loggerAgent
	m.file = fileAgent
	m.initialized = true

	return nil
}

// startStep describes one agent's start action and its corresponding rollback (stop).
type startStep struct {
	name  string
	start func(context.Context) error
	stop  func(context.Context) error
}

// Start transitions all five managed agents to an active state in order:
// Event -> Store -> Timer -> Logger -> File.
// If the Nth agent fails, agents 1..(N-1) are stopped in reverse order.
// BaseLifecycle agents (Store, Timer, Logger) treat Start() as a no-op since
// Init() already transitions them to Running.
func (m *SystemAgentManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return ErrNotInitialized
	}

	steps := []startStep{
		{name: "event", start: m.event.Start, stop: m.event.Stop},
		{name: "store", start: m.store.Start, stop: m.store.Stop},
		{name: "timer", start: m.timer.Start, stop: m.timer.Stop},
		{name: "logger", start: m.logger.Start, stop: m.logger.Stop},
		{name: "file", start: m.file.Start, stop: m.file.Stop},
	}

	for i, step := range steps {
		if err := step.start(ctx); err != nil {
			// Rollback: stop agents 0..(i-1) in reverse order
			for j := i - 1; j >= 0; j-- {
				_ = steps[j].stop(ctx)
			}
			return fmt.Errorf("system manager: %s agent start failed: %w", step.name, ErrAgentStartFailed)
		}
	}

	m.started = true
	return nil
}

// Stop stops all managed agents in reverse order: File -> Logger -> Timer -> Store -> Event.
// If one agent fails to stop, the remaining agents are still stopped and the first error is returned.
func (m *SystemAgentManager) Stop(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.initialized {
		return ErrNotInitialized
	}

	type stopEntry struct {
		name  string
		agent interface{ Stop(context.Context) error }
	}

	// Reverse order: File -> Logger -> Timer -> Store -> Event
	entries := []stopEntry{
		{"file", m.file},
		{"logger", m.logger},
		{"timer", m.timer},
		{"store", m.store},
		{"event", m.event},
	}

	var firstErr error
	for _, e := range entries {
		if e.agent != nil {
			if err := e.agent.Stop(ctx); err != nil && firstErr == nil {
				firstErr = fmt.Errorf("system manager: %s agent stop failed: %w", e.name, ErrAgentStopFailed)
			}
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

// Logger returns the logger agent, or nil if not initialized.
func (m *SystemAgentManager) Logger() *LoggerAgent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.logger
}

// Timer returns the timer agent, or nil if not initialized.
func (m *SystemAgentManager) Timer() *TimerAgent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.timer
}

// Store returns the store agent, or nil if not initialized.
func (m *SystemAgentManager) Store() *StoreAgent {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.store
}

// LoggerBridge returns a new LoggerBridgeHandler, or nil if the logger agent is not initialized.
func (m *SystemAgentManager) LoggerBridge() *LoggerBridgeHandler {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.logger == nil {
		return nil
	}
	return NewLoggerBridgeHandler(m.logger)
}

// TimerBridge returns a new TimerBridgeHandler, or nil if the timer agent is not initialized.
func (m *SystemAgentManager) TimerBridge() *TimerBridgeHandler {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.timer == nil {
		return nil
	}
	return NewTimerBridgeHandler(m.timer)
}

// StoreBridge returns a new BridgeHandler for the store agent, or nil if not initialized.
func (m *SystemAgentManager) StoreBridge() *BridgeHandler {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		return nil
	}
	return NewBridgeHandler(m.store)
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
