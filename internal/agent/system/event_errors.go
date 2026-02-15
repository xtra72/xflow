package system

import "errors"

// Sentinel errors for the Event Agent.
var (
	// ErrTopicEmpty is returned when an empty topic string is provided.
	ErrTopicEmpty = errors.New("system/event: topic cannot be empty")

	// ErrHandlerNil is returned when a nil handler function is provided.
	ErrHandlerNil = errors.New("system/event: handler cannot be nil")

	// ErrInvalidPattern is returned when an invalid subscription pattern is provided.
	ErrInvalidPattern = errors.New("system/event: invalid subscription pattern")

	// ErrEventBufferFull is returned when the event delivery buffer is full.
	ErrEventBufferFull = errors.New("system/event: event buffer is full")

	// ErrEventAgentClosed is returned when operating on a closed event agent.
	ErrEventAgentClosed = errors.New("system/event: event agent is closed")

	// ErrEventSubNotFound is returned when an unknown subscription ID is provided.
	ErrEventSubNotFound = errors.New("system/event: subscription not found")
)
