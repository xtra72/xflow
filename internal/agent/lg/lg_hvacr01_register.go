package lg

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterHvacr01Types 는 LG HVACR-01 (LG ICP-01 프로토콜 기반) 패킷 캡처 에이전트 타입을 Manager 에 등록한다.
func RegisterHvacr01Types(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("lg_hvacr01", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewHvacr01Agent(config)
	})
}
