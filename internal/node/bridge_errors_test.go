package node

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// --- Bridge 센티널 에러 테스트 ---

// TestBridgeErrors_고유성 은 모든 bridge 센티널 에러가 서로 다른지 확인한다.
func TestBridgeErrors_고유성(t *testing.T) {
	bridgeErrors := []error{
		ErrAgentNotRunning,
		ErrAgentDisconnected,
		ErrCorrelationNotFound,
		ErrTransformFailed,
		ErrInvalidDirection,
		ErrMaxReconnectExceeded,
		ErrBufferFull,
	}

	for i, e1 := range bridgeErrors {
		for j, e2 := range bridgeErrors {
			if i != j {
				assert.NotEqual(t, e1.Error(), e2.Error(),
					"에러 %d와 %d가 동일한 메시지를 가짐", i, j)
			}
		}
	}
}

// TestBridgeErrors_Is호환성 은 errors.Is가 정상 동작하는지 확인한다.
func TestBridgeErrors_Is호환성(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		target error
	}{
		{"ErrAgentNotRunning", ErrAgentNotRunning, ErrAgentNotRunning},
		{"ErrAgentDisconnected", ErrAgentDisconnected, ErrAgentDisconnected},
		{"ErrCorrelationNotFound", ErrCorrelationNotFound, ErrCorrelationNotFound},
		{"ErrTransformFailed", ErrTransformFailed, ErrTransformFailed},
		{"ErrInvalidDirection", ErrInvalidDirection, ErrInvalidDirection},
		{"ErrMaxReconnectExceeded", ErrMaxReconnectExceeded, ErrMaxReconnectExceeded},
		{"ErrBufferFull", ErrBufferFull, ErrBufferFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, errors.Is(tt.err, tt.target))
		})
	}
}

// TestBridgeErrors_래핑호환성 은 fmt.Errorf로 래핑된 에러도 errors.Is로 확인할 수 있는지 검증한다.
func TestBridgeErrors_래핑호환성(t *testing.T) {
	tests := []struct {
		name   string
		err    error
	}{
		{"ErrAgentNotRunning", ErrAgentNotRunning},
		{"ErrAgentDisconnected", ErrAgentDisconnected},
		{"ErrCorrelationNotFound", ErrCorrelationNotFound},
		{"ErrTransformFailed", ErrTransformFailed},
		{"ErrInvalidDirection", ErrInvalidDirection},
		{"ErrMaxReconnectExceeded", ErrMaxReconnectExceeded},
		{"ErrBufferFull", ErrBufferFull},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wrapped := fmt.Errorf("상위 에러: %w", tt.err)
			assert.True(t, errors.Is(wrapped, tt.err))
		})
	}
}

// TestBridgeErrors_기존에러와_중복없음 은 bridge 에러가 기존 errors.go의 에러와 겹치지 않는지 확인한다.
func TestBridgeErrors_기존에러와_중복없음(t *testing.T) {
	existingErrors := []error{
		ErrNodeTypeAlreadyRegistered,
		ErrNodeTypeNotFound,
		ErrPortNotFound,
		ErrInvalidConfig,
		ErrAgentNotFound,
		ErrRequestTimeout,
		ErrScriptCompileFailed,
		ErrScriptExecutionFailed,
		ErrScriptTimeout,
		ErrNodeNotInitialized,
		ErrNodeAlreadyInitialized,
		ErrAggregateWindowInvalid,
	}

	bridgeErrors := []error{
		ErrAgentNotRunning,
		ErrAgentDisconnected,
		ErrCorrelationNotFound,
		ErrTransformFailed,
		ErrInvalidDirection,
		ErrMaxReconnectExceeded,
		ErrBufferFull,
	}

	for _, be := range bridgeErrors {
		for _, ee := range existingErrors {
			assert.NotEqual(t, be.Error(), ee.Error(),
				"bridge 에러 %q가 기존 에러 %q와 중복됨", be.Error(), ee.Error())
		}
	}
}

// TestBridgeErrors_메시지접두사 는 모든 bridge 에러가 "bridge:" 접두사를 가지는지 확인한다.
func TestBridgeErrors_메시지접두사(t *testing.T) {
	bridgeErrors := []error{
		ErrAgentNotRunning,
		ErrAgentDisconnected,
		ErrCorrelationNotFound,
		ErrTransformFailed,
		ErrInvalidDirection,
		ErrMaxReconnectExceeded,
		ErrBufferFull,
	}

	for _, err := range bridgeErrors {
		assert.Contains(t, err.Error(), "bridge:",
			"에러 %q에 'bridge:' 접두사가 없음", err.Error())
	}
}

// TestBridgeErrors_NodeError래핑 은 NodeError로 래핑된 bridge 에러도 errors.Is로 확인할 수 있는지 검증한다.
func TestBridgeErrors_NodeError래핑(t *testing.T) {
	nodeErr := &NodeError{
		NodeID:   "bridge-1",
		NodeType: "bridge",
		Err:      ErrAgentNotRunning,
	}

	assert.True(t, errors.Is(nodeErr, ErrAgentNotRunning))
	assert.Contains(t, nodeErr.Error(), "bridge-1")
	assert.Contains(t, nodeErr.Error(), "bridge")
}
