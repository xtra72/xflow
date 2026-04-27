// Package adapter provides protocol-specific device adapters that bridge
// agent-internal device types to the unified device.Device interface.
// Each adapter decouples the device package from protocol agent packages
// to prevent circular imports.
package adapter

import "context"

// CommandExecutor is a function that executes a command on a device.
// The agent's provider will supply this from the agent's Process method.
type CommandExecutor func(ctx context.Context, command string, params map[string]any) (map[string]any, error)
