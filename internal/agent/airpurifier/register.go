package airpurifier

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterAirPurifierTypes 는 공기청정기 에이전트 타입을 Manager 에 등록한다
// (samsung.RegisterSamsungHvacr01Types 패턴 미러, REQ-AIRPUR-001-01-02).
func RegisterAirPurifierTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType(agentType, func(config agent.AgentConfig) (agent.Agent, error) {
		return NewAirPurifierAgent(config)
	})
}
