package serial

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// TestRegisterSerialTypes 는 "serial" 에이전트 타입이 등록되는지 확인한다.
func TestRegisterSerialTypes(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSerialTypes(mgr)
	require.NoError(t, err)

	// "serial" 타입으로 Create 시도 — 설정 부족으로 실패하더라도
	// "transport not available" 에러가 아니면 타입이 등록된 것이다.
	_, err = mgr.Create(agent.AgentConfig{
		ID:   "test-serial",
		Name: "test-serial",
		Type: "serial",
	})
	if err != nil {
		assert.NotContains(t, err.Error(), "transport not available",
			"타입 \"serial\" 이 등록되지 않음")
	}
}

// TestRegisterSerialTypes_DuplicateRegistration 은 동일 타입을 중복 등록하면 에러를 반환하는지 확인한다.
func TestRegisterSerialTypes_DuplicateRegistration(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSerialTypes(mgr)
	require.NoError(t, err)

	// 두 번째 등록은 에러를 반환해야 한다.
	err = RegisterSerialTypes(mgr)
	assert.Error(t, err)
}

// TestRegisterSerialTypes_UnknownTypeStillFails 는 등록되지 않은 타입으로 Create 시 에러를 확인한다.
func TestRegisterSerialTypes_UnknownTypeStillFails(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSerialTypes(mgr)
	require.NoError(t, err)

	// 등록되지 않은 타입은 실패해야 한다.
	_, err = mgr.Create(agent.AgentConfig{
		ID:   "test-unknown",
		Name: "test-unknown",
		Type: "unknown-type",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport type not available")
}
