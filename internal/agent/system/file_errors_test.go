package system

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFileErrors_NonNil verifies all sentinel errors are non-nil.
func TestFileErrors_NonNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrPathOutsideSandbox", ErrPathOutsideSandbox},
		{"ErrSandboxNotConfigured", ErrSandboxNotConfigured},
		{"ErrFileNotFound", ErrFileNotFound},
		{"ErrWatchPathInvalid", ErrWatchPathInvalid},
		{"ErrFileAgentClosed", ErrFileAgentClosed},
		{"ErrDirNotFound", ErrDirNotFound},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.err, "%s must not be nil", tc.name)
		})
	}
}

// TestFileErrors_UniqueMessages verifies all sentinel error messages are distinct.
func TestFileErrors_UniqueMessages(t *testing.T) {
	sentinels := []error{
		ErrPathOutsideSandbox,
		ErrSandboxNotConfigured,
		ErrFileNotFound,
		ErrWatchPathInvalid,
		ErrFileAgentClosed,
		ErrDirNotFound,
	}

	seen := make(map[string]bool)
	for _, err := range sentinels {
		msg := err.Error()
		assert.False(t, seen[msg], "duplicate error message found: %s", msg)
		seen[msg] = true
	}
}

// TestFileErrors_ErrorsIs verifies wrapped errors can be identified via errors.Is.
func TestFileErrors_ErrorsIs(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrPathOutsideSandbox", ErrPathOutsideSandbox},
		{"ErrSandboxNotConfigured", ErrSandboxNotConfigured},
		{"ErrFileNotFound", ErrFileNotFound},
		{"ErrWatchPathInvalid", ErrWatchPathInvalid},
		{"ErrFileAgentClosed", ErrFileAgentClosed},
		{"ErrDirNotFound", ErrDirNotFound},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("context: %w", tc.err)
			assert.True(t, errors.Is(wrapped, tc.err),
				"should identify %s from wrapped error", tc.name)
		})
	}
}

// TestFileErrors_MessagePrefix verifies all error messages have "system/file:" prefix.
func TestFileErrors_MessagePrefix(t *testing.T) {
	sentinels := []error{
		ErrPathOutsideSandbox,
		ErrSandboxNotConfigured,
		ErrFileNotFound,
		ErrWatchPathInvalid,
		ErrFileAgentClosed,
		ErrDirNotFound,
	}

	for _, err := range sentinels {
		assert.Contains(t, err.Error(), "system/file:",
			"error message must contain 'system/file:' prefix: %s", err.Error())
	}
}
