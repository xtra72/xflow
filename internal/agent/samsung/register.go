package samsung

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterSamsungHvacr01Types 는 Samsung NASA HVAC 에이전트 타입을 Manager 에 등록한다.
func RegisterSamsungHvacr01Types(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("samsung_hvacr01", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewHvacr01Agent(config)
	})
}
