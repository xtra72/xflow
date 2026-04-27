package lg

import "errors"

var (
	// ErrLGCNPSerialPortRequired 는 시리얼 포트가 지정되지 않았을 때 반환된다.
	ErrLGCNPSerialPortRequired = errors.New("lgcnp: serial_port is required")

	// ErrLGCNPInvalidBaudRate 는 잘못된 보레이트 값이 지정되었을 때 반환된다.
	ErrLGCNPInvalidBaudRate = errors.New("lgcnp: invalid baud_rate")

	// ErrLGCNPUnknownTransportType 은 알 수 없는 트랜스포트 타입이 지정되었을 때 반환된다.
	ErrLGCNPUnknownTransportType = errors.New("lgcnp: unknown transport_type (must be serial, tcp-client, or tcp-server)")

	// ErrLGCNPTCPPortRequired 는 TCP 모드에서 tcp_port 가 지정되지 않았을 때 반환된다.
	ErrLGCNPTCPPortRequired = errors.New("lgcnp: tcp_port is required for tcp-client and tcp-server transport")

	// ErrLGCNPTCPHostRequired 는 tcp-client 모드에서 tcp_host 가 지정되지 않았을 때 반환된다.
	ErrLGCNPTCPHostRequired = errors.New("lgcnp: tcp_host is required for tcp-client transport")

	// ErrLGCNPControlNotSupported 는 LGCNP 에이전트에서 제어를 시도할 때 반환된다.
	ErrLGCNPControlNotSupported = errors.New("lgcnp: control is not supported for this protocol")
)
