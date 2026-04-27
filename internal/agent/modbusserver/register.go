package modbusserver

import "github.com/xtra/xflow/internal/agent"

// RegisterModbusServerTypes registers the modbus-tcp-server agent type with the manager.
func RegisterModbusServerTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("modbus-tcp-server", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewModbusServerAgent(config)
	})
}
