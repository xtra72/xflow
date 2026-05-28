package node

import (
	"testing"
)

// TestAgentReinitializer_InterfaceCompliance 는 모든 에이전트 백엔드 노드가
// AgentReinitializer 인터페이스를 구현하는지 컴파일 시점에 검증한다.
// 이 변수 선언이 컴파일되면 인터페이스 구현이 보장된다.
//
// 수정 전 (bridgeReinitializer 만 BridgeNode 가 구현하던 시점) 에는 본 파일이
// 컴파일되지 않았으므로 회귀를 방지한다.
func TestAgentReinitializer_InterfaceCompliance(t *testing.T) {
	// 컴파일 타임 인터페이스 체크
	var _ AgentReinitializer = (*BridgeNode)(nil)

	// NASA 계열
	var _ AgentReinitializer = (*SamsungHvacr01StatusNode)(nil)
	var _ AgentReinitializer = (*SamsungHvacr01ControlNode)(nil)
	var _ AgentReinitializer = (*SamsungHvacr01Node)(nil)

	// LGCP 계열
	var _ AgentReinitializer = (*LGCPStatusNode)(nil)
	var _ AgentReinitializer = (*LGCPControlNode)(nil)
	var _ AgentReinitializer = (*LGCPNode)(nil)

	// lg_hvacr01 (LG HVACR-01) 계열
	var _ AgentReinitializer = (*LGHvacr01StatusNode)(nil)
	var _ AgentReinitializer = (*LGHvacr01ControlNode)(nil)
	var _ AgentReinitializer = (*LGHvacr01Node)(nil)

	// LGAP 계열
	var _ AgentReinitializer = (*LGAPStatusNode)(nil)
	var _ AgentReinitializer = (*LGAPControlNode)(nil)
	var _ AgentReinitializer = (*LGAPNode)(nil)

	// MQTT 계열
	var _ AgentReinitializer = (*MQTTSubNode)(nil)
	var _ AgentReinitializer = (*MQTTPublisherNode)(nil)

	// Modbus 계열
	var _ AgentReinitializer = (*ModbusNode)(nil)
	var _ AgentReinitializer = (*ModbusPollerNode)(nil)
	var _ AgentReinitializer = (*ModbusWriterNode)(nil)

	// InfluxDB 계열
	var _ AgentReinitializer = (*InfluxDBReadNode)(nil)
	var _ AgentReinitializer = (*InfluxDBQueryNode)(nil)

	// TSDB 계열
	var _ AgentReinitializer = (*TSDBQueryNode)(nil)

	// Serial 계열
	var _ AgentReinitializer = (*SerialInNode)(nil)
	var _ AgentReinitializer = (*SerialOutNode)(nil)

	// TCP 계열
	var _ AgentReinitializer = (*TCPInNode)(nil)
	var _ AgentReinitializer = (*TCPOutNode)(nil)

	t.Log("모든 에이전트 백엔드 노드가 AgentReinitializer 인터페이스를 구현한다")
}
