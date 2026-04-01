package socket

import (
	"fmt"

	"github.com/xtra/xflow/internal/agent"
)

// RegisterSocketTypes 는 소켓 에이전트 타입(TCP/UDP 서버/클라이언트)을 Manager 에 등록한다.
func RegisterSocketTypes(mgr *agent.DefaultManager) error {
	types := map[string]agent.AgentFactory{
		"tcp-server": func(config agent.AgentConfig) (agent.Agent, error) {
			return NewTCPServerAgent(config)
		},
		"tcp-client": func(config agent.AgentConfig) (agent.Agent, error) {
			return NewTCPClientAgent(config)
		},
		"udp-server": func(config agent.AgentConfig) (agent.Agent, error) {
			return NewUDPServerAgent(config)
		},
		"udp-client": func(config agent.AgentConfig) (agent.Agent, error) {
			return NewUDPClientAgent(config)
		},
	}
	for typeName, factory := range types {
		if err := mgr.RegisterType(typeName, factory); err != nil {
			return fmt.Errorf("socket register %s: %w", typeName, err)
		}
	}
	return nil
}
