package samsung

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterSamsungNASATypes 는 Samsung NASA HVAC 에이전트 타입을 Manager 에 등록한다.
func RegisterSamsungNASATypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("samsung-nasa", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewNASAAgent(config)
	})
}
