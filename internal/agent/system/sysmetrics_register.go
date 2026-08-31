package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterSysMetricsTypes 는 시스템 모니터링 에이전트 타입을 Manager에 등록한다.
func RegisterSysMetricsTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType(SysMetricsAgentType, func(config agent.AgentConfig) (agent.Agent, error) {
		return NewSysMetricsAgent(config)
	})
}
