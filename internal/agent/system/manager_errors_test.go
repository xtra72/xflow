package system

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestManagerErrors_NonNil verifies all sentinel errors are non-nil.
func TestManagerErrors_NonNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrAlreadyInitialized", ErrAlreadyInitialized},
		{"ErrNotInitialized", ErrNotInitialized},
		{"ErrAgentTypeUnknown", ErrAgentTypeUnknown},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.err, "%s must not be nil", tc.name)
		})
	}
}

// TestManagerErrors_UniqueMessages verifies all sentinel error messages are distinct.
func TestManagerErrors_UniqueMessages(t *testing.T) {
	sentinels := []error{
		ErrAlreadyInitialized,
		ErrNotInitialized,
		ErrAgentTypeUnknown,
	}

	seen := make(map[string]bool)
	for _, err := range sentinels {
		msg := err.Error()
		assert.False(t, seen[msg], "duplicate error message found: %s", msg)
		seen[msg] = true
	}
}

// TestManagerErrors_ErrorsIs verifies wrapped errors can be identified via errors.Is.
func TestManagerErrors_ErrorsIs(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrAlreadyInitialized", ErrAlreadyInitialized},
		{"ErrNotInitialized", ErrNotInitialized},
		{"ErrAgentTypeUnknown", ErrAgentTypeUnknown},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("context: %w", tc.err)
			assert.True(t, errors.Is(wrapped, tc.err),
				"should identify %s from wrapped error", tc.name)
		})
	}
}

// TestManagerErrors_MessagePrefix verifies all error messages have "system:" prefix.
func TestManagerErrors_MessagePrefix(t *testing.T) {
	sentinels := []error{
		ErrAlreadyInitialized,
		ErrNotInitialized,
		ErrAgentTypeUnknown,
	}

	for _, err := range sentinels {
		assert.Contains(t, err.Error(), "system:",
			"error message must contain 'system:' prefix: %s", err.Error())
	}
}
