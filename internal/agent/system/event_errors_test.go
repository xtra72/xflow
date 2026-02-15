package system

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEventErrors_NonNil verifies all sentinel errors are non-nil.
func TestEventErrors_NonNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrTopicEmpty", ErrTopicEmpty},
		{"ErrHandlerNil", ErrHandlerNil},
		{"ErrInvalidPattern", ErrInvalidPattern},
		{"ErrEventBufferFull", ErrEventBufferFull},
		{"ErrEventAgentClosed", ErrEventAgentClosed},
		{"ErrEventSubNotFound", ErrEventSubNotFound},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.err, "%s must not be nil", tc.name)
		})
	}
}

// TestEventErrors_UniqueMessages verifies all sentinel error messages are distinct.
func TestEventErrors_UniqueMessages(t *testing.T) {
	sentinels := []error{
		ErrTopicEmpty,
		ErrHandlerNil,
		ErrInvalidPattern,
		ErrEventBufferFull,
		ErrEventAgentClosed,
		ErrEventSubNotFound,
	}

	seen := make(map[string]bool)
	for _, err := range sentinels {
		msg := err.Error()
		assert.False(t, seen[msg], "duplicate error message found: %s", msg)
		seen[msg] = true
	}
}

// TestEventErrors_ErrorsIs verifies wrapped errors can be identified via errors.Is.
func TestEventErrors_ErrorsIs(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrTopicEmpty", ErrTopicEmpty},
		{"ErrHandlerNil", ErrHandlerNil},
		{"ErrInvalidPattern", ErrInvalidPattern},
		{"ErrEventBufferFull", ErrEventBufferFull},
		{"ErrEventAgentClosed", ErrEventAgentClosed},
		{"ErrEventSubNotFound", ErrEventSubNotFound},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("context: %w", tc.err)
			assert.True(t, errors.Is(wrapped, tc.err),
				"should identify %s from wrapped error", tc.name)
		})
	}
}

// TestEventErrors_MessagePrefix verifies all error messages have "system/event:" prefix.
func TestEventErrors_MessagePrefix(t *testing.T) {
	sentinels := []error{
		ErrTopicEmpty,
		ErrHandlerNil,
		ErrInvalidPattern,
		ErrEventBufferFull,
		ErrEventAgentClosed,
		ErrEventSubNotFound,
	}

	for _, err := range sentinels {
		assert.Contains(t, err.Error(), "system/event:",
			"error message must contain 'system/event:' prefix: %s", err.Error())
	}
}
