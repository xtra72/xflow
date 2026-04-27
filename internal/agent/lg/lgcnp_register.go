package lg

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterLGCNPTypes 는 LG LGCNP-01 패킷 캡처 에이전트 타입을 Manager 에 등록한다.
func RegisterLGCNPTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("lgcnp", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewLGCNPAgent(config)
	})
}
