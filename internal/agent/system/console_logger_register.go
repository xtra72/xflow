package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterConsoleLoggerType 은 logger 에이전트 타입을 Manager에 등록한다.
func RegisterConsoleLoggerType(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("logger", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewConsoleLoggerAgent(config)
	})
}
