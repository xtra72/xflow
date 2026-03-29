package lg

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterLGLGCPTypes 는 LG LGCP 패킷 캡처 에이전트 타입을 Manager 에 등록한다.
func RegisterLGLGCPTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("lgcp", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewLGCPAgent(config)
	})
}
