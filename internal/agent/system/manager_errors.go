package system

import "errors"

// Sentinel errors for the SystemAgentManager.
var (
	// ErrAlreadyInitialized is returned when Initialize is called on an already initialized manager.
	ErrAlreadyInitialized = errors.New("system: system agents already initialized")

	// ErrNotInitialized is returned when an operation requires initialization.
	ErrNotInitialized = errors.New("system: system agents not initialized")

	// ErrAgentTypeUnknown is returned when an unknown system agent type is referenced.
	ErrAgentTypeUnknown = errors.New("system: unknown system agent type")

	// ErrAgentInitFailed is returned when an agent fails to initialize.
	ErrAgentInitFailed = errors.New("system: agent initialization failed")

	// ErrAgentStartFailed is returned when an agent fails to start.
	ErrAgentStartFailed = errors.New("system: agent start failed")

	// ErrAgentStopFailed is returned when an agent fails to stop.
	ErrAgentStopFailed = errors.New("system: agent stop failed")
)
