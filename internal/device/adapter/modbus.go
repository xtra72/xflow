package adapter

import (
	"context"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// ModbusDeviceInfo holds pre-extracted data from a ModbusDevice.
// This breaks the import dependency on the modbus agent package.
type ModbusDeviceInfo struct {
	DeviceID       string         // From DeviceConfig.ID
	Host           string
	Port           int
	UnitID         byte
	Online         bool
	LastSeen       time.Time
	RegisterGroups []string       // Names of register groups (for capabilities)
	CacheData      map[string]any // Cached register values (from RegisterCache)
	Writable       bool           // Whether device supports write operations
}

// ModbusDeviceAdapter wraps Modbus device data into the unified Device interface.
type ModbusDeviceAdapter struct {
	info      ModbusDeviceInfo
	agentName string
	metadata  device.DeviceMetadata
	executor  CommandExecutor      // nil for read-only devices
	commands  []device.CommandSpec // nil for read-only devices
}

// NewModbusDevice creates a read-only ModbusDeviceAdapter.
func NewModbusDevice(agentName string, info ModbusDeviceInfo) *ModbusDeviceAdapter {
	return &ModbusDeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

// NewControllableModbusDevice creates a writable ModbusDeviceAdapter
// that implements ControllableDevice.
func NewControllableModbusDevice(agentName string, info ModbusDeviceInfo, executor CommandExecutor) *ModbusDeviceAdapter {
	return &ModbusDeviceAdapter{
		info:      info,
		agentName: agentName,
		executor:  executor,
		commands:  modbusCommandSpecs(),
	}
}

// ID returns the globally unique device ID in the format "agentName:deviceID".
func (a *ModbusDeviceAdapter) ID() string {
	return fmt.Sprintf("%s:%s", a.agentName, a.info.DeviceID)
}

// Name returns a human-readable name for this device.
func (a *ModbusDeviceAdapter) Name() string {
	return fmt.Sprintf("Modbus Device %s", a.info.DeviceID)
}

// Type returns DeviceTypeSensor as the default type for Modbus devices.
func (a *ModbusDeviceAdapter) Type() device.DeviceType {
	return device.DeviceTypeSensor
}

// Protocol returns "modbus".
func (a *ModbusDeviceAdapter) Protocol() string {
	return "modbus"
}

// AgentName returns the name of the owning agent.
func (a *ModbusDeviceAdapter) AgentName() string {
	return a.agentName
}

// Online returns whether the device is currently online.
func (a *ModbusDeviceAdapter) Online() bool {
	return a.info.Online
}

// LastSeen returns the last communication time.
func (a *ModbusDeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

// State returns the current device state with Modbus-specific properties.
func (a *ModbusDeviceAdapter) State() device.DeviceState {
	props := map[string]any{
		"host":    a.info.Host,
		"port":    a.info.Port,
		"unit_id": a.info.UnitID,
	}

	// Merge cached register data into properties
	for k, v := range a.info.CacheData {
		props[k] = v
	}

	return device.DeviceState{
		Online:     a.info.Online,
		LastSeen:   a.info.LastSeen,
		Properties: props,
	}
}

// Metadata returns user-defined metadata for this device.
func (a *ModbusDeviceAdapter) Metadata() device.DeviceMetadata {
	return a.metadata
}

func (a *ModbusDeviceAdapter) Source() string {
	return "config"
}

// Capabilities returns the list of supported capabilities based on
// register groups and writable flag.
func (a *ModbusDeviceAdapter) Capabilities() []string {
	caps := []string{"read_registers"}

	// Add register group names as capabilities
	caps = append(caps, a.info.RegisterGroups...)

	// Add write capabilities if the device is writable
	if a.info.Writable {
		caps = append(caps, "write_register", "write_coil")
	}

	return caps
}

// Execute runs a named command with the given parameters.
// Returns ErrNotControllable if the device is read-only.
func (a *ModbusDeviceAdapter) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if a.executor == nil {
		return nil, device.ErrNotControllable
	}
	return a.executor(ctx, command, params)
}

// Commands returns the list of available commands for this device.
// Returns nil for read-only devices.
func (a *ModbusDeviceAdapter) Commands() []device.CommandSpec {
	return a.commands
}

// modbusCommandSpecs generates the command specifications for writable Modbus devices.
func modbusCommandSpecs() []device.CommandSpec {
	return []device.CommandSpec{
		{
			Name:        "write_register",
			Description: "Write a value to a holding register",
			Params: []device.ParamSpec{
				{
					Name:     "address",
					Type:     "int",
					Required: true,
				},
				{
					Name:     "value",
					Type:     "int",
					Required: true,
				},
			},
		},
		{
			Name:        "write_coil",
			Description: "Write a boolean value to a coil",
			Params: []device.ParamSpec{
				{
					Name:     "address",
					Type:     "int",
					Required: true,
				},
				{
					Name:     "value",
					Type:     "bool",
					Required: true,
				},
			},
		},
	}
}
