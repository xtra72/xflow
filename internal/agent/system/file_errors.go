package system

import "errors"

// Sentinel errors for the File Agent.
var (
	// ErrPathOutsideSandbox is returned when a path resolves outside the sandbox directory.
	ErrPathOutsideSandbox = errors.New("system/file: path outside sandbox directory")

	// ErrSandboxNotConfigured is returned when the sandbox directory is not set.
	ErrSandboxNotConfigured = errors.New("system/file: sandbox directory not configured")

	// ErrFileNotFound is returned when the requested file does not exist.
	ErrFileNotFound = errors.New("system/file: file not found")

	// ErrWatchPathInvalid is returned when a watch path is invalid.
	ErrWatchPathInvalid = errors.New("system/file: watch path is invalid")

	// ErrFileAgentClosed is returned when operating on a closed file agent.
	ErrFileAgentClosed = errors.New("system/file: file agent is closed")

	// ErrDirNotFound is returned when the requested directory does not exist.
	ErrDirNotFound = errors.New("system/file: directory not found")

	// ErrUnsupportedCommand is returned when an unknown bridge command is received.
	ErrUnsupportedCommand = errors.New("system/file: unsupported command")

	// ErrAliasNotFound is returned when a target file alias cannot be resolved.
	ErrAliasNotFound = errors.New("system/file: alias not found")

	// ErrInvalidCommandFormat is returned when a bridge command has invalid JSON format.
	ErrInvalidCommandFormat = errors.New("system/file: invalid command format")

	// ErrTextModeRequired is returned when a text-only operation is attempted in binary mode.
	ErrTextModeRequired = errors.New("system/file: text mode required for this operation")

	// ErrLineOutOfRange is returned when a line number is outside the file's line count.
	ErrLineOutOfRange = errors.New("system/file: line number out of range")
)
