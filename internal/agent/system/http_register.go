package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterHTTPTypes 는 HTTP 관련 에이전트 타입을 Manager에 등록한다.
func RegisterHTTPTypes(mgr *agent.DefaultManager) error {
	if err := mgr.RegisterType("http", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewHTTPReceiverAgent(config)
	}); err != nil {
		return err
	}

	if err := mgr.RegisterType("http-sender", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewHTTPSenderAgent(config)
	}); err != nil {
		return err
	}

	return nil
}
