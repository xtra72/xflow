package api

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/engine"
	"github.com/xtra/xflow/pkg/xferr"
)

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		expected string
	}{
		{
			name:     "bad request error",
			err:      ErrBadRequest,
			expected: "api: BAD_REQUEST: bad request",
		},
		{
			name:     "unauthorized error",
			err:      ErrUnauthorized,
			expected: "api: UNAUTHORIZED: unauthorized",
		},
		{
			name:     "forbidden error",
			err:      ErrForbidden,
			expected: "api: FORBIDDEN: forbidden",
		},
		{
			name:     "not found error",
			err:      ErrNotFound,
			expected: "api: NOT_FOUND: not found",
		},
		{
			name:     "conflict error",
			err:      ErrConflict,
			expected: "api: CONFLICT: resource conflict",
		},
		{
			name:     "validation failed error",
			err:      ErrValidationFailed,
			expected: "api: VALIDATION_FAILED: validation failed",
		},
		{
			name:     "rate limit exceeded error",
			err:      ErrRateLimitExceeded,
			expected: "api: RATE_LIMIT_EXCEEDED: rate limit exceeded",
		},
		{
			name:     "internal server error",
			err:      ErrInternalServer,
			expected: "api: INTERNAL_ERROR: internal server error",
		},
		{
			name:     "service unavailable error",
			err:      ErrServiceUnavailable,
			expected: "api: SERVICE_UNAVAILABLE: service unavailable",
		},
		{
			name:     "request timeout error",
			err:      ErrRequestTimeout,
			expected: "api: REQUEST_TIMEOUT: request timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.err.Error())
		})
	}
}

func TestAPIError_HTTPCodes(t *testing.T) {
	tests := []struct {
		name     string
		err      *APIError
		httpCode int
	}{
		{"bad request", ErrBadRequest, 400},
		{"unauthorized", ErrUnauthorized, 401},
		{"forbidden", ErrForbidden, 403},
		{"not found", ErrNotFound, 404},
		{"request timeout", ErrRequestTimeout, 408},
		{"conflict", ErrConflict, 409},
		{"validation failed", ErrValidationFailed, 422},
		{"rate limit exceeded", ErrRateLimitExceeded, 429},
		{"internal server error", ErrInternalServer, 500},
		{"service unavailable", ErrServiceUnavailable, 503},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.httpCode, tt.err.HTTPCode)
		})
	}
}

func TestAPIError_WithMessage(t *testing.T) {
	tests := []struct {
		name        string
		base        *APIError
		newMessage  string
		expectedMsg string
	}{
		{
			name:        "custom bad request message",
			base:        ErrBadRequest,
			newMessage:  "invalid user ID",
			expectedMsg: "api: BAD_REQUEST: invalid user ID",
		},
		{
			name:        "custom not found message",
			base:        ErrNotFound,
			newMessage:  "flow not found",
			expectedMsg: "api: NOT_FOUND: flow not found",
		},
		{
			name:        "empty message",
			base:        ErrBadRequest,
			newMessage:  "",
			expectedMsg: "api: BAD_REQUEST: ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newErr := tt.base.WithMessage(tt.newMessage)

			// 새 인스턴스여야 한다 (원본 변경 없음)
			assert.NotSame(t, tt.base, newErr)
			assert.Equal(t, tt.base.HTTPCode, newErr.HTTPCode)
			assert.Equal(t, tt.base.Code, newErr.Code)
			assert.Equal(t, tt.newMessage, newErr.Message)
			assert.Equal(t, tt.expectedMsg, newErr.Error())

			// 원본은 변경되지 않아야 한다
			assert.NotEqual(t, tt.newMessage, tt.base.Message)
		})
	}
}

func TestAPIError_WithDetails(t *testing.T) {
	tests := []struct {
		name    string
		base    *APIError
		details any
	}{
		{
			name:    "string details",
			base:    ErrBadRequest,
			details: "field 'name' is required",
		},
		{
			name:    "map details",
			base:    ErrValidationFailed,
			details: map[string]string{"field": "name", "reason": "required"},
		},
		{
			name:    "nil details",
			base:    ErrInternalServer,
			details: nil,
		},
		{
			name:    "slice details",
			base:    ErrBadRequest,
			details: []string{"error1", "error2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newErr := tt.base.WithDetails(tt.details)

			// 새 인스턴스여야 한다
			assert.NotSame(t, tt.base, newErr)
			assert.Equal(t, tt.base.HTTPCode, newErr.HTTPCode)
			assert.Equal(t, tt.base.Code, newErr.Code)
			assert.Equal(t, tt.base.Message, newErr.Message)
			assert.Equal(t, tt.details, newErr.Details)

			// 원본에는 details가 없어야 한다
			assert.Nil(t, tt.base.Details)
		})
	}
}

func TestAPIError_WithMessageAndDetails(t *testing.T) {
	err := ErrBadRequest.
		WithMessage("invalid input").
		WithDetails(map[string]string{"field": "name"})

	assert.Equal(t, 400, err.HTTPCode)
	assert.Equal(t, "BAD_REQUEST", err.Code)
	assert.Equal(t, "invalid input", err.Message)
	assert.Equal(t, map[string]string{"field": "name"}, err.Details)
	assert.Equal(t, "api: BAD_REQUEST: invalid input", err.Error())
}

func TestAPIError_ImplementsErrorInterface(t *testing.T) {
	var err error = ErrBadRequest
	assert.NotNil(t, err)
	assert.Equal(t, "api: BAD_REQUEST: bad request", err.Error())
}

func TestMapDomainError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected *APIError
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: nil,
		},
		{
			name:     "nil message error maps to bad request",
			err:      xferr.ErrNilMessage,
			expected: ErrBadRequest,
		},
		{
			name:     "nil error error maps to bad request",
			err:      xferr.ErrNilError,
			expected: ErrBadRequest,
		},
		{
			name:     "no receiver error maps to service unavailable",
			err:      xferr.ErrNoReceiver,
			expected: ErrServiceUnavailable,
		},
		{
			name:     "invalid severity maps to bad request",
			err:      xferr.ErrInvalidSeverity,
			expected: ErrBadRequest,
		},
		{
			name:     "invalid category maps to bad request",
			err:      xferr.ErrInvalidCategory,
			expected: ErrBadRequest,
		},
		{
			name:     "invalid drop reason maps to bad request",
			err:      xferr.ErrInvalidDropReason,
			expected: ErrBadRequest,
		},
		{
			name:     "invalid component type maps to bad request",
			err:      xferr.ErrInvalidComponentType,
			expected: ErrBadRequest,
		},
		{
			name:     "same state transition maps to conflict",
			err:      xferr.ErrSameStateTransition,
			expected: ErrConflict,
		},
		{
			name:     "alert threshold exceeded maps to rate limit",
			err:      xferr.ErrAlertThresholdExceeded,
			expected: ErrRateLimitExceeded,
		},
		{
			name:     "node start failed maps to validation failed",
			err:      fmt.Errorf("%w: node init failed", engine.ErrNodeStartFailed),
			expected: ErrValidationFailed,
		},
		{
			name:     "shutdown timeout maps to request timeout",
			err:      engine.ErrShutdownTimeout,
			expected: ErrRequestTimeout,
		},
		{
			name:     "transport not available maps to validation failed",
			err:      agent.ErrTransportNotAvailable,
			expected: ErrValidationFailed,
		},
		{
			name:     "wrapped transport not available maps to validation failed",
			err:      fmt.Errorf("manager create: agent type %q: %w", "custom", agent.ErrTransportNotAvailable),
			expected: ErrValidationFailed,
		},
		{
			name:     "unknown error maps to internal server error",
			err:      errors.New("unknown error"),
			expected: ErrInternalServer,
		},
		{
			name:     "wrapped nil message error maps to bad request",
			err:      fmt.Errorf("wrapped: %w", xferr.ErrNilMessage),
			expected: ErrBadRequest,
		},
		{
			name:     "wrapped same state transition maps to conflict",
			err:      fmt.Errorf("flow error: %w", xferr.ErrSameStateTransition),
			expected: ErrConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MapDomainError(tt.err)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, tt.expected.HTTPCode, result.HTTPCode)
				assert.Equal(t, tt.expected.Code, result.Code)
			}
		})
	}
}
