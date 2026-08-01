package modbus

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterModbusTypes 는 MODBUS 클라이언트 에이전트 타입을 Manager 에 등록한다.
func RegisterModbusTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("modbus-client", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewModbusAgent(config)
	})
}
