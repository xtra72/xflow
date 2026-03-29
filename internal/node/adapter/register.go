package adapter

import "github.com/xtra/xflow/internal/node"

func init() {
	// 모든 시스템 에이전트 서브타입을 전역 레지스트리에 등록한다.
	for subtype := range validSubtypes {
		node.RegisterAdapter(subtype, NewSystemAdapter(subtype))
	}
	// 일반적인 "system" 타입도 등록한다 (기본 서브타입: store).
	node.RegisterAdapter("system", NewSystemAdapter("store"))

	// MQTT 프로토콜 어댑터 등록
	node.RegisterAdapter("mqtt-client", NewMQTTAdapter())

	// HTTP 프로토콜 어댑터 등록
	node.RegisterAdapter("http", NewHTTPAdapter())

	// Modbus 프로토콜 어댑터 등록
	node.RegisterAdapter("modbus-tcp", NewModbusAdapter(WithUnitID(1)))
	node.RegisterAdapter("modbus-rtu", NewModbusAdapter(WithUnitID(1)))
	node.RegisterAdapter("modbus-tcp-server", NewModbusAdapter(WithUnitID(1)))

	// Samsung NASA 프로토콜 어댑터 등록
	node.RegisterAdapter("samsung-nasa", NewNASAAdapter())
}
