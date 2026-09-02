package xsfm

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterXSFMTypes 는 설비 에이전트 타입을 Manager 에 등록한다
// (samsung.RegisterSamsungHvacr01Types 패턴 미러, REQ-XSFM-001-01-02).
func RegisterXSFMTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType(agentType, func(config agent.AgentConfig) (agent.Agent, error) {
		return NewXSFMAgent(config)
	})
}
