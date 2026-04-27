package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterInfluxDBTypes 는 InfluxDB 관련 에이전트 타입을 Manager 에 등록한다.
func RegisterInfluxDBTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("influxdb", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewInfluxDBAgent(config)
	})
}
