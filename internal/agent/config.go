package agent

import (
	"fmt"
	"log/slog"
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
	Type                string            // Agent type (e.g., "custom", "mqtt-client", "system")
	Transport           TransportConfig   // Transport layer configuration
	ProtocolFile        string            // Path to protocol definition file
	HealthCheckInterval time.Duration     // Health check interval (default: 30s)
	MaxRestarts         int               // Maximum restart count (default: 10)
	StopOnZeroRef       bool              // Stop agent when reference count reaches 0
	BufferSize          int               // Pause buffer size in bytes (default: 1024)
	LogLevel            string            // Log level override (debug, info, warn, error); empty = daemon default
	Metadata            map[string]string // Agent metadata key-value pairs
	Logger              *slog.Logger      `json:"-"` // Observer 기반 로거. nil 이면 slog.Default() 폴백.
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

// DeviceEntry 는 에이전트 설정에서 디바이스 등록 항목을 나타낸다.
// 모든 에이전트(NASA, LGCP, LGAP)에서 공통으로 사용한다.
type DeviceEntry struct {
	Address string // 프로토콜별 주소 (NASA: "200001", LGCP: "44550067", LGAP: "0x10")
	Name    string // 사람이 읽을 수 있는 이름 (선택)
}

// ParseDevices 는 에이전트 설정 옵션에서 "devices" 배열을 파싱한다.
func ParseDevices(opts map[string]any) []DeviceEntry {
	v, ok := opts["devices"]
	if !ok {
		return nil
	}
	items, ok := v.([]any)
	if !ok {
		return nil
	}
	var devices []DeviceEntry
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		entry := DeviceEntry{}
		if addr, ok := m["address"]; ok {
			entry.Address = fmt.Sprintf("%v", addr)
		}
		if name, ok := m["name"]; ok {
			entry.Name = fmt.Sprintf("%v", name)
		}
		if entry.Address != "" {
			devices = append(devices, entry)
		}
	}
	return devices
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
