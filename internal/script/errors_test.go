package script

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// RED Phase: Sentinel error 테스트
// ============================================================

func TestSentinelErrors_NotNil(t *testing.T) {
	// 모든 sentinel error가 정의되어 있어야 한다
	sentinels := []error{
		ErrVMPoolExhausted,
		ErrScriptCompileFailed,
		ErrScriptExecutionFailed,
		ErrScriptTimeout,
		ErrSandboxViolation,
		ErrScriptNotFound,
		ErrInvalidScriptSource,
		ErrHotReloadFailed,
	}
	for _, err := range sentinels {
		assert.NotNil(t, err, "sentinel error should not be nil")
	}
}

func TestSentinelErrors_Distinct(t *testing.T) {
	// 모든 sentinel error는 고유해야 한다
	sentinels := []error{
		ErrVMPoolExhausted,
		ErrScriptCompileFailed,
		ErrScriptExecutionFailed,
		ErrScriptTimeout,
		ErrSandboxViolation,
		ErrScriptNotFound,
		ErrInvalidScriptSource,
		ErrHotReloadFailed,
	}
	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			assert.NotEqual(t, sentinels[i].Error(), sentinels[j].Error(),
				"sentinel errors must be distinct: %v vs %v", sentinels[i], sentinels[j])
		}
	}
}

func TestSentinelErrors_ErrorsIs(t *testing.T) {
	// 각 sentinel error는 errors.Is로 비교 가능해야 한다
	tests := []struct {
		name string
		err  error
	}{
		{"ErrVMPoolExhausted", ErrVMPoolExhausted},
		{"ErrScriptCompileFailed", ErrScriptCompileFailed},
		{"ErrScriptExecutionFailed", ErrScriptExecutionFailed},
		{"ErrScriptTimeout", ErrScriptTimeout},
		{"ErrSandboxViolation", ErrSandboxViolation},
		{"ErrScriptNotFound", ErrScriptNotFound},
		{"ErrInvalidScriptSource", ErrInvalidScriptSource},
		{"ErrHotReloadFailed", ErrHotReloadFailed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 직접 비교
			assert.True(t, errors.Is(tt.err, tt.err))
			// fmt.Errorf 래핑 후 비교
			wrapped := fmt.Errorf("context: %w", tt.err)
			assert.True(t, errors.Is(wrapped, tt.err))
		})
	}
}

func TestSentinelErrors_ContainPrefix(t *testing.T) {
	// 모든 sentinel error 메시지는 "script:" 접두사를 포함해야 한다
	sentinels := []error{
		ErrVMPoolExhausted,
		ErrScriptCompileFailed,
		ErrScriptExecutionFailed,
		ErrScriptTimeout,
		ErrSandboxViolation,
		ErrScriptNotFound,
		ErrInvalidScriptSource,
		ErrHotReloadFailed,
	}
	for _, err := range sentinels {
		assert.Contains(t, err.Error(), "script:", "error message should contain 'script:' prefix: %v", err)
	}
}

// ============================================================
// RED Phase: ScriptError 구조체 테스트
// ============================================================

func TestScriptError_Error_WithScriptIDOnly(t *testing.T) {
	// ScriptID만 있고 Line이 0인 경우
	se := &ScriptError{
		Err:      ErrScriptCompileFailed,
		ScriptID: "my-script",
		Line:     0,
		Detail:   "syntax error near 'end'",
	}
	expected := "[script:my-script] syntax error near 'end'"
	assert.Equal(t, expected, se.Error())
}

func TestScriptError_Error_WithScriptIDAndLine(t *testing.T) {
	// ScriptID와 Line이 모두 있는 경우
	se := &ScriptError{
		Err:      ErrScriptExecutionFailed,
		ScriptID: "my-script",
		Line:     42,
		Detail:   "attempt to index a nil value",
	}
	expected := "[script:my-script:42] attempt to index a nil value"
	assert.Equal(t, expected, se.Error())
}

func TestScriptError_Unwrap(t *testing.T) {
	// Unwrap은 원래 에러를 반환해야 한다
	se := &ScriptError{
		Err:      ErrScriptTimeout,
		ScriptID: "timeout-script",
		Detail:   "execution exceeded 5s",
	}
	assert.Equal(t, ErrScriptTimeout, se.Unwrap())
}

func TestScriptError_ErrorsIs(t *testing.T) {
	// errors.Is로 래핑된 에러를 식별할 수 있어야 한다
	se := &ScriptError{
		Err:      ErrScriptNotFound,
		ScriptID: "missing-script",
		Detail:   "script not in cache",
	}
	assert.True(t, errors.Is(se, ErrScriptNotFound))
	assert.False(t, errors.Is(se, ErrScriptTimeout))
}

func TestScriptError_ErrorsAs(t *testing.T) {
	// errors.As로 ScriptError 구조체를 추출할 수 있어야 한다
	se := &ScriptError{
		Err:      ErrVMPoolExhausted,
		ScriptID: "pool-test",
		Line:     10,
		Detail:   "no available VMs",
	}
	var wrapped error = fmt.Errorf("execution failed: %w", se)

	var target *ScriptError
	require.True(t, errors.As(wrapped, &target))
	assert.Equal(t, "pool-test", target.ScriptID)
	assert.Equal(t, 10, target.Line)
	assert.Equal(t, "no available VMs", target.Detail)
	assert.Equal(t, ErrVMPoolExhausted, target.Err)
}

func TestScriptError_Error_EmptyScriptID(t *testing.T) {
	// ScriptID가 비어있는 경우
	se := &ScriptError{
		Err:      ErrSandboxViolation,
		ScriptID: "",
		Detail:   "attempted to load os module",
	}
	expected := "[script:] attempted to load os module"
	assert.Equal(t, expected, se.Error())
}

func TestScriptError_Error_EmptyDetail(t *testing.T) {
	// Detail이 비어있는 경우
	se := &ScriptError{
		Err:      ErrHotReloadFailed,
		ScriptID: "test-script",
		Detail:   "",
	}
	expected := "[script:test-script] "
	assert.Equal(t, expected, se.Error())
}
