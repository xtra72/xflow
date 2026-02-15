package agent

import (
	"fmt"
	"time"
)

// TransportConfig holds the configuration for an agent's transport layer.
type TransportConfig struct {
	Type    string         // Transport type name (e.g., "serial", "tcp", "udp")
	Options map[string]any // Transport-specific configuration options
}

// AgentConfig holds the configuration for an agent instance.
type AgentConfig struct {
	ID                  string            // Unique agent identifier
	Name                string            // Human-readable agent name
	Type                string            // Agent type (e.g., "custom", "mqtt", "system")
	Transport           TransportConfig   // Transport layer configuration
	ProtocolFile        string            // Path to protocol definition file
	HealthCheckInterval time.Duration     // Health check interval (default: 30s)
	MaxRestarts         int               // Maximum restart count (default: 10)
	StopOnZeroRef       bool              // Stop agent when reference count reaches 0
	BufferSize          int               // Pause buffer size in bytes (default: 1024)
	Metadata            map[string]string // Agent metadata key-value pairs
}

// Validate checks the AgentConfig for required fields and valid values.
func (c *AgentConfig) Validate() error {
	if c.ID == "" {
		return fmt.Errorf("%w: ID is required", ErrInvalidConfig)
	}
	if c.Name == "" {
		return fmt.Errorf("%w: Name is required", ErrInvalidConfig)
	}
	if c.HealthCheckInterval < 0 {
		return fmt.Errorf("%w: HealthCheckInterval must be >= 0", ErrInvalidConfig)
	}
	if c.MaxRestarts < 0 {
		return fmt.Errorf("%w: MaxRestarts must be >= 0", ErrInvalidConfig)
	}
	if c.BufferSize < 0 {
		return fmt.Errorf("%w: BufferSize must be >= 0", ErrInvalidConfig)
	}
	// Apply defaults for zero values
	if c.HealthCheckInterval == 0 {
		c.HealthCheckInterval = 30 * time.Second
	}
	if c.BufferSize == 0 {
		c.BufferSize = 1024
	}
	return nil
}

// DefaultAgentConfig returns an AgentConfig with sensible default values.
func DefaultAgentConfig() AgentConfig {
	return AgentConfig{
		HealthCheckInterval: 30 * time.Second,
		MaxRestarts:         10,
		StopOnZeroRef:       true,
		BufferSize:          1024,
	}
}
