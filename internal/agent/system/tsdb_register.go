package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterTSDBTypes 는 TSDB 에이전트 타입을 Manager에 등록한다.
func RegisterTSDBTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("tsdb", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewTSDBAgent(config)
	})
}
