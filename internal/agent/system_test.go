package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSystemAgentType_StringValues(t *testing.T) {
	// SystemAgentType 문자열 값이 올바른지 확인한다.
	tests := []struct {
		agentType SystemAgentType
		expected  string
	}{
		{SystemAgentEvent, "event"},
		{SystemAgentLogger, "logger"},
		{SystemAgentFile, "file"},
		{SystemAgentTimer, "timer"},
		{SystemAgentStore, "store"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, string(tc.agentType))
		})
	}
}

func TestSystemAgentType_AllDefined(t *testing.T) {
	// 5개의 SystemAgentType이 모두 정의되어 있는지 확인한다.
	types := []SystemAgentType{
		SystemAgentEvent,
		SystemAgentLogger,
		SystemAgentFile,
		SystemAgentTimer,
		SystemAgentStore,
	}

	assert.Len(t, types, 5)
	for _, sat := range types {
		assert.NotEmpty(t, string(sat))
	}
}

func TestSystemAgent_InterfaceDefinition(t *testing.T) {
	// SystemAgent 인터페이스가 Agent를 임베딩하는지 확인한다.
	// 컴파일 타임 체크를 위한 타입 확인
	var _ SystemAgent = (*mockSystemAgent)(nil)
}

// mockSystemAgent는 SystemAgent 인터페이스의 컴파일 타임 체크를 위한 목 구현체이다.
type mockSystemAgent struct {
	*BaseAgent
}

func (m *mockSystemAgent) IsSystem() bool         { return true }
func (m *mockSystemAgent) RequiresTransport() bool { return false }
