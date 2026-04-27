package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterStoreTypes 는 Store 에이전트 타입을 Manager에 등록한다.
func RegisterStoreTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("store", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewUserStoreAgent(config)
	})
}
