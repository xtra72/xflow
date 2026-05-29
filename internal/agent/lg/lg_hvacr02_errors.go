package lg

import "errors"

var (
	// ErrHvacr02SerialPortRequired 는 시리얼 포트가 지정되지 않았을 때 반환된다.
	ErrHvacr02SerialPortRequired = errors.New("lg_hvacr02: serial_port is required")

	// ErrHvacr02InvalidBaudRate 는 잘못된 보레이트 값이 지정되었을 때 반환된다.
	ErrHvacr02InvalidBaudRate = errors.New("lg_hvacr02: invalid baud_rate")

	// ErrHvacr02NotConnected 는 트랜스포트가 연결되지 않은 상태에서 동작을 시도할 때 반환된다.
	ErrHvacr02NotConnected = errors.New("lg_hvacr02: transport not connected")

	// ErrHvacr02AlreadyRunning 은 에이전트가 이미 실행 중일 때 반환된다.
	ErrHvacr02AlreadyRunning = errors.New("lg_hvacr02: agent already running")

	// ErrHvacr02ControlNotEnabled 는 제어 기능이 비활성화된 에이전트에서 제어를 시도할 때 반환된다.
	ErrHvacr02ControlNotEnabled = errors.New("lg_hvacr02: control not enabled for this agent")

	// ErrHvacr02InvalidAddress 는 주소 형식이 올바르지 않을 때 반환된다.
	ErrHvacr02InvalidAddress = errors.New("lg_hvacr02: invalid address format")

	// ErrHvacr02TemperatureOutOfRange 는 설정 온도가 허용 범위를 벗어났을 때 반환된다.
	ErrHvacr02TemperatureOutOfRange = errors.New("lg_hvacr02: temperature out of range")

	// ErrHvacr02InvalidFanSpeed 는 유효하지 않은 팬 속도가 지정되었을 때 반환된다.
	ErrHvacr02InvalidFanSpeed = errors.New("lg_hvacr02: invalid fan_speed")

	// ErrHvacr02InvalidMode 는 유효하지 않은 운전 모드가 지정되었을 때 반환된다.
	ErrHvacr02InvalidMode = errors.New("lg_hvacr02: invalid mode")

	// ErrHvacr02MissingParam 은 필수 파라미터가 누락되었을 때 반환된다.
	ErrHvacr02MissingParam = errors.New("lg_hvacr02: missing required parameter")

	// ErrHvacr02SerialWriteFailed 는 시리얼 포트 쓰기가 실패했을 때 반환된다.
	ErrHvacr02SerialWriteFailed = errors.New("lg_hvacr02: serial write failed")

	// ErrHvacr02UnknownTransportType 은 알 수 없는 트랜스포트 타입이 지정되었을 때 반환된다.
	ErrHvacr02UnknownTransportType = errors.New("lg_hvacr02: unknown transport_type (must be serial, tcp-client, or tcp-server)")

	// ErrHvacr02TCPPortRequired 는 TCP 모드에서 tcp_port 가 지정되지 않았을 때 반환된다.
	ErrHvacr02TCPPortRequired = errors.New("lg_hvacr02: tcp_port is required for tcp-client and tcp-server transport")

	// ErrHvacr02TCPHostRequired 는 tcp-client 모드에서 tcp_host 가 지정되지 않았을 때 반환된다.
	ErrHvacr02TCPHostRequired = errors.New("lg_hvacr02: tcp_host is required for tcp-client transport")
)
