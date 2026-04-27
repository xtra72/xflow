package node

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSentinelErrors_정의확인 는 모든 센티널 에러가 정의되어 있는지 확인한다.
func TestSentinelErrors_정의확인(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrNodeTypeAlreadyRegistered", ErrNodeTypeAlreadyRegistered, "node: type already registered"},
		{"ErrNodeTypeNotFound", ErrNodeTypeNotFound, "node: type not found"},
		{"ErrPortNotFound", ErrPortNotFound, "node: port not found"},
		{"ErrInvalidConfig", ErrInvalidConfig, "node: invalid configuration"},
		{"ErrAgentNotFound", ErrAgentNotFound, "node: agent not found"},
		{"ErrRequestTimeout", ErrRequestTimeout, "node: request-reply timeout"},
		{"ErrScriptCompileFailed", ErrScriptCompileFailed, "node: script compile failed"},
		{"ErrScriptExecutionFailed", ErrScriptExecutionFailed, "node: script execution failed"},
		{"ErrScriptTimeout", ErrScriptTimeout, "node: script execution timeout"},
		{"ErrNodeNotInitialized", ErrNodeNotInitialized, "node: not initialized"},
		{"ErrNodeAlreadyInitialized", ErrNodeAlreadyInitialized, "node: already initialized"},
		{"ErrAggregateWindowInvalid", ErrAggregateWindowInvalid, "node: invalid aggregate window"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotNil(t, tt.err)
			assert.Equal(t, tt.msg, tt.err.Error())
		})
	}
}

// TestSentinelErrors_ErrorsIs호환 은 errors.Is()와 호환되는지 확인한다.
func TestSentinelErrors_ErrorsIs호환(t *testing.T) {
	wrapped := fmt.Errorf("래핑: %w", ErrNodeTypeNotFound)
	assert.True(t, errors.Is(wrapped, ErrNodeTypeNotFound))
	assert.False(t, errors.Is(wrapped, ErrPortNotFound))
}

// TestNodeError_Error포맷 은 NodeError의 Error() 메서드 포맷을 확인한다.
func TestNodeError_Error포맷(t *testing.T) {
	tests := []struct {
		name     string
		nodeErr  *NodeError
		expected string
	}{
		{
			name: "기본 포맷",
			nodeErr: &NodeError{
				NodeID:   "node-1",
				NodeType: "filter",
				Err:      ErrPortNotFound,
			},
			expected: "node [node-1] (filter): node: port not found",
		},
		{
			name: "빈 NodeID",
			nodeErr: &NodeError{
				NodeID:   "",
				NodeType: "transform",
				Err:      ErrInvalidConfig,
			},
			expected: "node [] (transform): node: invalid configuration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.nodeErr.Error())
		})
	}
}

// TestNodeError_Unwrap 은 NodeError의 Unwrap() 메서드를 확인한다.
func TestNodeError_Unwrap(t *testing.T) {
	inner := ErrPortNotFound
	nodeErr := &NodeError{
		NodeID:   "node-1",
		NodeType: "filter",
		Err:      inner,
	}

	// Unwrap으로 내부 에러를 꺼낼 수 있어야 한다
	assert.Equal(t, inner, nodeErr.Unwrap())

	// errors.Is()로 내부 에러와 비교 가능해야 한다
	assert.True(t, errors.Is(nodeErr, ErrPortNotFound))
	assert.False(t, errors.Is(nodeErr, ErrInvalidConfig))
}

// TestNodeError_ErrorsAs호환 은 errors.As()로 NodeError를 추출할 수 있는지 확인한다.
func TestNodeError_ErrorsAs호환(t *testing.T) {
	nodeErr := &NodeError{
		NodeID:   "node-1",
		NodeType: "filter",
		Err:      ErrPortNotFound,
	}

	wrapped := fmt.Errorf("상위 에러: %w", nodeErr)

	var target *NodeError
	assert.True(t, errors.As(wrapped, &target))
	assert.Equal(t, "node-1", target.NodeID)
	assert.Equal(t, "filter", target.NodeType)
	assert.Equal(t, ErrPortNotFound, target.Err)
}
