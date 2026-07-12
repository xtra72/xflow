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
	Enabled             *bool             // 에이전트 활성화 상태 (nil = 기본 true). JSON/YAML 직렬화 시 omitempty 적용.
	Logger              *slog.Logger      `json:"-"` // Observer 기반 로거. nil 이면 slog.Default() 폴백.
}

// IsEnabled 는 에이전트의 활성화 상태를 반환한다.
// Enabled 가 nil 이면 기본값 true (활성화)를 반환한다.
// 이는 pkg/flow.NodeDef.IsEnabled 와 동일한 패턴을 사용한다.
func (c *AgentConfig) IsEnabled() bool {
	if c.Enabled == nil {
		return true
	}
	return *c.Enabled
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
	// Source 는 디바이스 등록 출처("config"/"bridge"/"auto")이다. 비어 있으면
	// 로더가 "config"(yaml 선언 디바이스) 로 간주한다. 런타임 등록("bridge") 디바이스가
	// 영속화 왕복 후에도 출처를 유지해 삭제 가능성이 보존되도록 한다(삭제 보호는
	// "config" 에만 적용). 후방호환: source 키가 없는 기존 항목은 "config" 로 로드된다.
	Source string
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
		if src, ok := m["source"]; ok {
			entry.Source = fmt.Sprintf("%v", src)
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
