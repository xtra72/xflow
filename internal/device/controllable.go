package device

import "context"

// ControllableDevice extends Device with command execution capabilities.
// Devices that support remote control (e.g., HVAC units) implement this interface.
type ControllableDevice interface {
	Device

	// Execute runs a named command with the given parameters and returns the result.
	Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error)

	// Commands returns the list of available commands for this device.
	Commands() []CommandSpec
}

// CommandSpec describes a command that can be executed on a controllable device.
type CommandSpec struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Params      []ParamSpec `json:"params"`
}

// ParamSpec describes a parameter for a command.
type ParamSpec struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`     // string, int, float, bool, enum
	Required bool     `json:"required"`
	Min      *float64 `json:"min,omitempty"`
	Max      *float64 `json:"max,omitempty"`
	Enum     []string `json:"enum,omitempty"`
}
