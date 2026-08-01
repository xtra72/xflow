package modbusserver

import "github.com/xtra/xflow/internal/agent"

// RegisterModbusServerTypes registers the modbus-server agent type with the manager.
func RegisterModbusServerTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("modbus-server", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewModbusServerAgent(config, mgr)
	})
}
