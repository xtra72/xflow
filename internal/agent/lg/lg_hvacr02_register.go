package lg

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterLGHvacr02Types 는 LG HVACR-02 (LG ICP-02) 패킷 캡처 에이전트 타입을 Manager 에 등록한다.
func RegisterLGHvacr02Types(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("lg_hvacr02", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewHvacr02Agent(config)
	})
}
