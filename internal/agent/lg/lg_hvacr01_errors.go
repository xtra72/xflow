package lg

import "errors"

var (
	// ErrHvacr01SerialPortRequired 는 시리얼 포트가 지정되지 않았을 때 반환된다.
	ErrHvacr01SerialPortRequired = errors.New("lg_hvacr01: serial_port is required")

	// ErrHvacr01InvalidBaudRate 는 잘못된 보레이트 값이 지정되었을 때 반환된다.
	ErrHvacr01InvalidBaudRate = errors.New("lg_hvacr01: invalid baud_rate")

	// ErrHvacr01UnknownTransportType 은 알 수 없는 트랜스포트 타입이 지정되었을 때 반환된다.
	ErrHvacr01UnknownTransportType = errors.New("lg_hvacr01: unknown transport_type (must be serial, tcp-client, or tcp-server)")

	// ErrHvacr01TCPPortRequired 는 TCP 모드에서 tcp_port 가 지정되지 않았을 때 반환된다.
	ErrHvacr01TCPPortRequired = errors.New("lg_hvacr01: tcp_port is required for tcp-client and tcp-server transport")

	// ErrHvacr01TCPHostRequired 는 tcp-client 모드에서 tcp_host 가 지정되지 않았을 때 반환된다.
	ErrHvacr01TCPHostRequired = errors.New("lg_hvacr01: tcp_host is required for tcp-client transport")

	// ErrHvacr01ControlNotSupported 는 lg_hvacr01 에이전트에서 제어를 시도할 때 반환된다.
	ErrHvacr01ControlNotSupported = errors.New("lg_hvacr01: control is not supported for this protocol")
)
