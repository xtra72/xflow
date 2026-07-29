package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/xtra/xflow/internal/device"
)

// Compile-time interface checks.
var (
	_ device.Device             = (*AirPurifierDeviceAdapter)(nil)
	_ device.ControllableDevice = (*AirPurifierDeviceAdapter)(nil)
)

// airPurifierProtocol is the protocol / adapter-registry key for air purifier devices.
const airPurifierProtocol = "airpurifier"

// AirPurifierProcessor is the minimal agent surface the executor bridges to.
// It is satisfied by *airpurifier.AirPurifierAgent (Process([]byte) ([]byte, error)).
// Declaring a local interface keeps this adapter package decoupled from the
// airpurifier agent package (preventing the circular import the package doc warns of).
type AirPurifierProcessor interface {
	Process(data []byte) ([]byte, error)
}

// AirPurifierDeviceInfo holds pre-extracted data from an air purifier device.
// This breaks any import dependency on the airpurifier agent package.
type AirPurifierDeviceInfo struct {
	DeviceID string // Device identifier (roster key, stable identity/address)
	Name     string // User-defined device name (may be empty)
	GroupID  string // Optional group identifier (may be empty)
	Online   bool
	LastSeen time.Time
	// State properties (nil-safe: nil means "not observed", omitted from State()).
	Power    *bool
	FanSpeed *int   // 1/2/3 (valid only when power is on)
	Source   string // "config", "bridge", or "auto"
}

// AirPurifierDeviceAdapter wraps air purifier device data into the unified Device interface.
type AirPurifierDeviceAdapter struct {
	info      AirPurifierDeviceInfo
	agentName string
	metadata  device.DeviceMetadata
	executor  CommandExecutor      // nil for non-controllable devices
	commands  []device.CommandSpec // nil for non-controllable devices
}

// NewAirPurifierDevice creates a Device from air purifier device info.
func NewAirPurifierDevice(agentName string, info AirPurifierDeviceInfo) *AirPurifierDeviceAdapter {
	return &AirPurifierDeviceAdapter{
		info:      info,
		agentName: agentName,
	}
}

// NewControllableAirPurifierDevice creates a ControllableDevice from air purifier device info.
func NewControllableAirPurifierDevice(agentName string, info AirPurifierDeviceInfo, executor CommandExecutor) *AirPurifierDeviceAdapter {
	return &AirPurifierDeviceAdapter{
		info:      info,
		agentName: agentName,
		executor:  executor,
		commands:  airPurifierCommandSpecs(),
	}
}

// ID returns the globally unique UUID v4 for this device.
func (a *AirPurifierDeviceAdapter) ID() string {
	return ResolveAdapterUID(a.agentName, a.info.DeviceID)
}

// UID returns the globally unique UUID v4 for this device.
func (a *AirPurifierDeviceAdapter) UID() string {
	return ResolveAdapterUID(a.agentName, a.info.DeviceID)
}

// Name returns the device name with priority: metadata.Name > info.Name > DeviceID.
func (a *AirPurifierDeviceAdapter) Name() string {
	if a.metadata.Name != "" {
		return a.metadata.Name
	}
	if a.info.Name != "" {
		return a.info.Name
	}
	return a.info.DeviceID
}

// Type returns the device type. Air purifiers map to the actuator type.
func (a *AirPurifierDeviceAdapter) Type() device.DeviceType {
	return device.DeviceTypeActuator
}

// Protocol returns the protocol / adapter-registry key ("airpurifier").
func (a *AirPurifierDeviceAdapter) Protocol() string {
	return airPurifierProtocol
}

// AgentName returns the name of the owning agent.
func (a *AirPurifierDeviceAdapter) AgentName() string {
	return a.agentName
}

// Online returns whether the device is currently online.
func (a *AirPurifierDeviceAdapter) Online() bool {
	return a.info.Online
}

// LastSeen returns the last communication time.
func (a *AirPurifierDeviceAdapter) LastSeen() time.Time {
	return a.info.LastSeen
}

// State returns the current device state with observation-gated properties.
func (a *AirPurifierDeviceAdapter) State() device.DeviceState {
	props := make(map[string]any)
	if a.info.Power != nil {
		props["power"] = *a.info.Power
	}
	// fan_speed is only meaningful when power is on (observation-gating parity with the agent).
	if a.info.FanSpeed != nil && (a.info.Power == nil || *a.info.Power) {
		props["fan_speed"] = *a.info.FanSpeed
	}

	var propsResult map[string]any
	if len(props) > 0 {
		propsResult = props
	}

	return device.DeviceState{
		Online:     a.info.Online,
		Ready:      a.info.Online,
		LastSeen:   a.info.LastSeen,
		Properties: propsResult,
	}
}

// Metadata returns user-defined metadata for this device.
func (a *AirPurifierDeviceAdapter) Metadata() device.DeviceMetadata {
	return a.metadata
}

// Source returns the device origin ("config", "bridge", or "auto").
func (a *AirPurifierDeviceAdapter) Source() string {
	if a.info.Source != "" {
		return a.info.Source
	}
	return "auto"
}

// Capabilities returns the list of supported capabilities.
func (a *AirPurifierDeviceAdapter) Capabilities() []string {
	return []string{"set_power", "set_fan_speed"}
}

// Execute runs a named command with the given parameters and returns the result.
// Returns ErrNotControllable if the device has no executor.
func (a *AirPurifierDeviceAdapter) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if a.executor == nil {
		return nil, device.ErrNotControllable
	}
	return a.executor(ctx, command, params)
}

// Commands returns the list of available commands for this device.
func (a *AirPurifierDeviceAdapter) Commands() []device.CommandSpec {
	return a.commands
}

// airPurifierCommandSpecs generates the command specifications for an air purifier device
// (REQ-AIRPUR-001-08-01): 2-axis control — set_power (bool) + set_fan_speed (enum 1/2/3).
func airPurifierCommandSpecs() []device.CommandSpec {
	return []device.CommandSpec{
		{
			Name:        "set_power",
			Description: "Set the power state",
			Params: []device.ParamSpec{
				{
					Name:     "power",
					Type:     "bool",
					Required: true,
				},
			},
		},
		{
			Name:        "set_fan_speed",
			Description: "Set the fan speed (1, 2, or 3)",
			Params: []device.ParamSpec{
				{
					Name:     "fan_speed",
					Type:     "enum",
					Required: true,
					Enum:     []string{"1", "2", "3"},
				},
			},
		},
	}
}

// NewAirPurifierExecutor creates a CommandExecutor that translates unified device
// commands into AirPurifierAgent.Process JSON requests (REQ-AIRPUR-001-08-02).
//
// It mirrors newHvacr01Executor: marshal {command, device_id, params} → Process →
// unmarshal the JSON response. The air purifier agent keys control on device_id
// (not a NASA address).
func NewAirPurifierExecutor(proc AirPurifierProcessor, deviceID string) CommandExecutor {
	return func(_ context.Context, command string, params map[string]any) (map[string]any, error) {
		req := map[string]any{
			"command":   command,
			"device_id": deviceID,
			"params":    params,
		}

		data, err := json.Marshal(req)
		if err != nil {
			return nil, fmt.Errorf("airpurifier executor: marshal request: %w", err)
		}

		resp, err := proc.Process(data)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, nil
		}

		var result map[string]any
		if err := json.Unmarshal(resp, &result); err != nil {
			return nil, fmt.Errorf("airpurifier executor: unmarshal response: %w", err)
		}
		return result, nil
	}
}
