package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterThingplusTypes 는 Thingplus(ThingsBoard Gateway) 관련 에이전트 타입을
// Manager에 등록한다. 웹 표시 레이블은 "Thingplus Gateway"이다.
func RegisterThingplusTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("thingplus-gateway", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewThingplusGatewayAgent(config)
	})
}
