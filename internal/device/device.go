// Package device provides a unified, protocol-agnostic device model and interfaces
// for managing IoT devices across multiple protocol agents.
package device

import "time"

// DeviceType represents the type of device.
type DeviceType string

const (
	// DeviceTypeIndoor represents an indoor device (e.g., indoor HVAC unit).
	DeviceTypeIndoor DeviceType = "indoor"

	// DeviceTypeOutdoor represents an outdoor device (e.g., outdoor HVAC unit).
	DeviceTypeOutdoor DeviceType = "outdoor"

	// DeviceTypeController represents a controller device.
	DeviceTypeController DeviceType = "controller"

	// DeviceTypeSensor represents a sensor device.
	DeviceTypeSensor DeviceType = "sensor"

	// DeviceTypeActuator represents an actuator device.
	DeviceTypeActuator DeviceType = "actuator"

	// DeviceTypeUnknown represents a device with an unknown type.
	DeviceTypeUnknown DeviceType = "unknown"
)

// Device is the unified interface for all protocol-agnostic devices.
// Every protocol-specific device (NASA, Modbus, MQTT, etc.) must be
// adaptable to this interface.
type Device interface {
	// ID returns the globally unique device ID in the format "agent_name:device_id".
	ID() string

	// Name returns the user-defined name for this device.
	Name() string

	// Type returns the device type (indoor, outdoor, controller, sensor, actuator).
	Type() DeviceType

	// Protocol returns the protocol name (nasa, modbus, mqtt, etc.).
	Protocol() string

	// AgentName returns the name of the owning agent.
	AgentName() string

	// Online returns whether the device is currently online.
	Online() bool

	// LastSeen returns the last communication time.
	LastSeen() time.Time

	// State returns the current device state.
	State() DeviceState

	// Metadata returns user-defined metadata for this device.
	Metadata() DeviceMetadata

	// Source returns the device origin: "config" (from config file) or "auto" (discovered).
	Source() string

	// Capabilities returns the list of supported capabilities.
	Capabilities() []string
}

// DeviceState holds the current state of a device. It separates common fields
// from protocol-specific properties stored in the Properties map.
type DeviceState struct {
	Online     bool           `json:"online"`
	Ready      bool           `json:"ready"`
	LastSeen   time.Time      `json:"last_seen"`
	ErrorCount int            `json:"error_count"`
	Properties map[string]any `json:"properties"` // Protocol-specific properties
}

// DeviceMetadata holds user-defined metadata such as name, tags, location, and labels.
type DeviceMetadata struct {
	Name     string            `json:"name"`              // 사용자 정의 디바이스 이름
	Tags     []string          `json:"tags"`
	Location string            `json:"location"`
	Group    string            `json:"group"`
	Labels   map[string]string `json:"labels"`
	Pinned   *bool             `json:"pinned,omitempty"` // 고정 설치 여부 (nil = 미설정)
}

// DeviceProvider is implemented by agents that manage devices.
// Each protocol agent (NASA, Modbus, etc.) implements this interface
// to expose its devices to the central registry.
type DeviceProvider interface {
	// Devices returns all devices managed by this provider.
	Devices() []Device

	// Device returns a specific device by its ID.
	Device(id string) (Device, error)
}
