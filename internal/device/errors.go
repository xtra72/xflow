package device

import "errors"

var (
	// ErrDeviceNotFound is returned when a device cannot be found by its ID.
	ErrDeviceNotFound = errors.New("device not found")

	// ErrNotControllable is returned when a command is sent to a non-controllable device.
	ErrNotControllable = errors.New("device is not controllable")

	// ErrAgentStopped is returned when an operation targets a device whose agent is stopped.
	ErrAgentStopped = errors.New("agent is stopped")

	// ErrDuplicateDevice is returned when a device with the same ID is already registered.
	ErrDuplicateDevice = errors.New("device already registered")

	// ErrProviderNotFound is returned when a device provider cannot be found.
	ErrProviderNotFound = errors.New("device provider not found")

	// ErrCommandNotFound is returned when the requested command does not exist on the device.
	ErrCommandNotFound = errors.New("command not found")

	// ErrInvalidParams is returned when command parameters are invalid.
	ErrInvalidParams = errors.New("invalid command parameters")
)
