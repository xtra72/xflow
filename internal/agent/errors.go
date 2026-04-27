package agent

import "errors"

// Sentinel errors for the agent package.
var (
	// ErrAgentNotFound is returned when an agent is not found by ID.
	ErrAgentNotFound = errors.New("agent: agent not found")

	// ErrAgentAlreadyExists is returned when creating an agent with a duplicate ID.
	ErrAgentAlreadyExists = errors.New("agent: agent already exists")

	// ErrAgentNotRunning is returned when an operation requires a running agent.
	ErrAgentNotRunning = errors.New("agent: agent not running")

	// ErrAgentAlreadyStopped is returned when stopping an already stopped agent.
	ErrAgentAlreadyStopped = errors.New("agent: agent already stopped")

	// ErrTransportNotAvailable is returned when a transport type is not registered.
	ErrTransportNotAvailable = errors.New("agent: transport type not available")

	// ErrTransportClosed is returned when reading/writing on a closed transport.
	ErrTransportClosed = errors.New("agent: transport connection closed")

	// ErrProtocolParseError is returned when protocol parsing fails.
	ErrProtocolParseError = errors.New("agent: protocol parse error")

	// ErrChecksumMismatch is returned when checksum verification fails.
	ErrChecksumMismatch = errors.New("agent: checksum mismatch")

	// ErrHealthCheckFailed is returned when a health check fails.
	ErrHealthCheckFailed = errors.New("agent: health check failed")

	// ErrMaxRestartsExceeded is returned when the maximum restart count is exceeded.
	ErrMaxRestartsExceeded = errors.New("agent: max restarts exceeded")

	// ErrInvalidConfig is returned when an agent configuration is invalid.
	ErrInvalidConfig = errors.New("agent: invalid configuration")

	// ErrConfigImmutable is returned when attempting to change an immutable config field at runtime.
	ErrConfigImmutable = errors.New("agent: immutable config field cannot be changed at runtime")

	// ErrInvalidStateTransition is returned when an invalid state transition is attempted.
	ErrInvalidStateTransition = errors.New("agent: invalid state transition")

	// ErrFlowAlreadyReferenced is returned when a flow is already referenced by the agent.
	ErrFlowAlreadyReferenced = errors.New("agent: flow already referenced")
)
