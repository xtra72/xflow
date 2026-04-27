package xferr

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
)

// === ErrorSeverity 테스트 ===

// TestErrorSeverity_Constants 는 ErrorSeverity 상수 값을 검증한다.
func TestErrorSeverity_Constants(t *testing.T) {
	tests := []struct {
		name     string
		severity ErrorSeverity
		expected string
	}{
		{"SeverityCritical", SeverityCritical, "critical"},
		{"SeverityError", SeverityError, "error"},
		{"SeverityWarning", SeverityWarning, "warning"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, ErrorSeverity(tt.expected), tt.severity)
		})
	}
}

// TestIsValidSeverity 는 심각도 유효성 검증을 테스트한다.
func TestIsValidSeverity(t *testing.T) {
	tests := []struct {
		name     string
		severity ErrorSeverity
		expected bool
	}{
		{"유효_critical", SeverityCritical, true},
		{"유효_error", SeverityError, true},
		{"유효_warning", SeverityWarning, true},
		{"무효_빈문자열", ErrorSeverity(""), false},
		{"무효_unknown", ErrorSeverity("unknown"), false},
		{"무효_info", ErrorSeverity("info"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsValidSeverity(tt.severity))
		})
	}
}

// === ErrorCategory 테스트 ===

// TestErrorCategory_Constants 는 ErrorCategory 상수 값을 검증한다.
func TestErrorCategory_Constants(t *testing.T) {
	tests := []struct {
		name     string
		category ErrorCategory
		expected string
	}{
		{"CategoryProcessing", CategoryProcessing, "processing"},
		{"CategoryValidation", CategoryValidation, "validation"},
		{"CategoryTimeout", CategoryTimeout, "timeout"},
		{"CategoryConnection", CategoryConnection, "connection"},
		{"CategoryConfiguration", CategoryConfiguration, "configuration"},
		{"CategorySystem", CategorySystem, "system"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, ErrorCategory(tt.expected), tt.category)
		})
	}
}

// TestIsValidCategory 는 카테고리 유효성 검증을 테스트한다.
func TestIsValidCategory(t *testing.T) {
	tests := []struct {
		name     string
		category ErrorCategory
		expected bool
	}{
		{"유효_processing", CategoryProcessing, true},
		{"유효_validation", CategoryValidation, true},
		{"유효_timeout", CategoryTimeout, true},
		{"유효_connection", CategoryConnection, true},
		{"유효_configuration", CategoryConfiguration, true},
		{"유효_system", CategorySystem, true},
		{"무효_빈문자열", ErrorCategory(""), false},
		{"무효_unknown", ErrorCategory("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsValidCategory(tt.category))
		})
	}
}

// === DropReason 테스트 ===

// TestDropReason_Constants 는 DropReason 상수 값을 검증한다.
func TestDropReason_Constants(t *testing.T) {
	tests := []struct {
		name     string
		reason   DropReason
		expected string
	}{
		{"ReasonTTLExpired", ReasonTTLExpired, "ttl_expired"},
		{"ReasonBackpressureDrop", ReasonBackpressureDrop, "backpressure_drop"},
		{"ReasonFilterRejected", ReasonFilterRejected, "filter_rejected"},
		{"ReasonMaxRetriesExceeded", ReasonMaxRetriesExceeded, "max_retries_exceeded"},
		{"ReasonNodeStopped", ReasonNodeStopped, "node_stopped"},
		{"ReasonChannelFull", ReasonChannelFull, "channel_full"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, DropReason(tt.expected), tt.reason)
		})
	}
}

// TestIsValidDropReason 는 폐기 사유 유효성 검증을 테스트한다.
func TestIsValidDropReason(t *testing.T) {
	tests := []struct {
		name     string
		reason   DropReason
		expected bool
	}{
		{"유효_ttl_expired", ReasonTTLExpired, true},
		{"유효_backpressure_drop", ReasonBackpressureDrop, true},
		{"유효_filter_rejected", ReasonFilterRejected, true},
		{"유효_max_retries_exceeded", ReasonMaxRetriesExceeded, true},
		{"유효_node_stopped", ReasonNodeStopped, true},
		{"유효_channel_full", ReasonChannelFull, true},
		{"무효_빈문자열", DropReason(""), false},
		{"무효_unknown", DropReason("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsValidDropReason(tt.reason))
		})
	}
}

// === ComponentType 테스트 ===

// TestComponentType_Constants 는 ComponentType 상수 값을 검증한다.
func TestComponentType_Constants(t *testing.T) {
	tests := []struct {
		name      string
		compType  ComponentType
		expected  string
	}{
		{"ComponentFlow", ComponentFlow, "flow"},
		{"ComponentNode", ComponentNode, "node"},
		{"ComponentAgent", ComponentAgent, "agent"},
		{"ComponentScriptEngine", ComponentScriptEngine, "script_engine"},
		{"ComponentPlugin", ComponentPlugin, "plugin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, ComponentType(tt.expected), tt.compType)
		})
	}
}

// TestIsValidComponentType 는 컴포넌트 타입 유효성 검증을 테스트한다.
func TestIsValidComponentType(t *testing.T) {
	tests := []struct {
		name     string
		compType ComponentType
		expected bool
	}{
		{"유효_flow", ComponentFlow, true},
		{"유효_node", ComponentNode, true},
		{"유효_agent", ComponentAgent, true},
		{"유효_script_engine", ComponentScriptEngine, true},
		{"유효_plugin", ComponentPlugin, true},
		{"무효_빈문자열", ComponentType(""), false},
		{"무효_unknown", ComponentType("unknown"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, IsValidComponentType(tt.compType))
		})
	}
}

// === ClassifySeverity 테스트 ===

// TestClassifySeverity 는 에러의 심각도 분류를 테스트한다.
func TestClassifySeverity(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorSeverity
	}{
		{"context_DeadlineExceeded는_error", context.DeadlineExceeded, SeverityError},
		{"일반_에러는_error", assert.AnError, SeverityError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ClassifySeverity(tt.err))
		})
	}
}

// === ClassifyCategory 테스트 ===

// mockNetError 는 net.Error 인터페이스를 구현하는 모의 에러이다.
type mockNetError struct {
	timeout   bool
	temporary bool
}

func (e *mockNetError) Error() string   { return "mock net error" }
func (e *mockNetError) Timeout() bool   { return e.timeout }
func (e *mockNetError) Temporary() bool { return e.temporary }

// mockNetError가 net.Error 인터페이스를 구현하는지 컴파일 타임 검증
var _ net.Error = (*mockNetError)(nil)

// TestClassifyCategory 는 에러의 카테고리 분류를 테스트한다.
func TestClassifyCategory(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected ErrorCategory
	}{
		{"net.Error는_connection", &mockNetError{}, CategoryConnection},
		{"context_DeadlineExceeded는_timeout", context.DeadlineExceeded, CategoryTimeout},
		{"context_Canceled는_timeout", context.Canceled, CategoryTimeout},
		{"일반_에러는_processing", assert.AnError, CategoryProcessing},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, ClassifyCategory(tt.err))
		})
	}
}
