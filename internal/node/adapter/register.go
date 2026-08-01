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

	// Thingplus(ThingsBoard Gateway) 프로토콜 어댑터 등록
	node.RegisterAdapter("thingplus-gateway", NewThingplusAdapter())

	// HTTP 프로토콜 어댑터 등록
	node.RegisterAdapter("http", NewHTTPAdapter())

	// Modbus 프로토콜 어댑터 등록
	node.RegisterAdapter("modbus-tcp", NewModbusAdapter(WithUnitID(1)))
	node.RegisterAdapter("modbus-rtu", NewModbusAdapter(WithUnitID(1)))
	node.RegisterAdapter("modbus-server", NewModbusAdapter(WithUnitID(1)))

	// Samsung NASA 프로토콜 어댑터 등록
	node.RegisterAdapter("samsung_hvacr01", NewSamsungHvacr01Adapter())

	// Socket 프로토콜 어댑터 등록
	node.RegisterAdapter("tcp-server", NewSocketAdapter())
	node.RegisterAdapter("tcp-client", NewSocketAdapter())
	node.RegisterAdapter("udp-server", NewSocketAdapter())
	node.RegisterAdapter("udp-client", NewSocketAdapter())

	// Serial 프로토콜 어댑터 등록
	node.RegisterAdapter("serial", NewSerialAdapter())
}
