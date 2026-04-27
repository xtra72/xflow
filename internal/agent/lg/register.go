package lg

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterLGLGAPTypes 는 LG LGAP HVAC 에이전트 타입을 Manager 에 등록한다.
func RegisterLGLGAPTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("lgap", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewLGAPAgent(config)
	})
}
