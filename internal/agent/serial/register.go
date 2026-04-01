package serial

import (
	"fmt"

	"github.com/xtra/xflow/internal/agent"
)

// RegisterSerialTypes 는 시리얼 에이전트 타입을 Manager 에 등록한다.
func RegisterSerialTypes(mgr *agent.DefaultManager) error {
	types := map[string]agent.AgentFactory{
		"serial": func(config agent.AgentConfig) (agent.Agent, error) {
			return NewSerialAgent(config)
		},
	}
	for typeName, factory := range types {
		if err := mgr.RegisterType(typeName, factory); err != nil {
			return fmt.Errorf("serial register %s: %w", typeName, err)
		}
	}
	return nil
}
