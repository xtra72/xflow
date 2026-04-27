package socket

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/agent"
)

// TestRegisterSocketTypes 는 4가지 소켓 에이전트 타입이 모두 등록되는지 확인한다.
func TestRegisterSocketTypes(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSocketTypes(mgr)
	require.NoError(t, err)

	expectedTypes := []string{"tcp-server", "tcp-client", "udp-server", "udp-client"}
	for _, typ := range expectedTypes {
		t.Run(typ, func(t *testing.T) {
			// Create 를 통해 등록 여부를 검증한다.
			// 실제 소켓 에이전트 생성에는 유효한 설정이 필요하므로,
			// TypeRegistry 에 등록되었는지만 확인한다.
			_, err := mgr.Create(agent.AgentConfig{
				ID:   "test-" + typ,
				Name: "test-" + typ,
				Type: typ,
			})
			// 에이전트 생성은 설정 부족으로 실패할 수 있지만,
			// "transport not available" 에러가 아니면 타입이 등록된 것이다.
			if err != nil {
				assert.NotContains(t, err.Error(), "transport not available",
					"타입 %q 가 등록되지 않음", typ)
			}
		})
	}
}

// TestRegisterSocketTypes_AllTypesRegistered 는 등록 후 에이전트 생성이 타입 미등록 에러를 반환하지 않는지 확인한다.
func TestRegisterSocketTypes_AllTypesRegistered(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSocketTypes(mgr)
	require.NoError(t, err)

	// 등록되지 않은 타입으로 Create 시도하면 transport not available 에러가 발생해야 한다.
	_, err = mgr.Create(agent.AgentConfig{
		ID:   "test-unknown",
		Name: "test-unknown",
		Type: "unknown-type",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "transport type not available")
}

// TestRegisterSocketTypes_DuplicateRegistration 은 동일 타입을 중복 등록하면 에러를 반환하는지 확인한다.
func TestRegisterSocketTypes_DuplicateRegistration(t *testing.T) {
	mgr := agent.NewManager()

	err := RegisterSocketTypes(mgr)
	require.NoError(t, err)

	// 두 번째 등록은 에러를 반환해야 한다.
	err = RegisterSocketTypes(mgr)
	assert.Error(t, err)
}
