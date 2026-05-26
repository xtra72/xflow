package system

import (
	"github.com/xtra/xflow/internal/agent"
)

// RegisterInfluxDBTypes 는 InfluxDB 관련 에이전트 타입을 Manager 에 등록한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D § D-T17: dual-tag 부착 기능이 제거되어
// DeviceResolver 주입 경로 (RegisterInfluxDBTypesWithResolver) 와 옵션 패턴이
// 불필요해졌다. 단일 함수로 단순화됨.
func RegisterInfluxDBTypes(mgr *agent.DefaultManager) error {
	return mgr.RegisterType("influxdb", func(config agent.AgentConfig) (agent.Agent, error) {
		return NewInfluxDBAgent(config)
	})
}
