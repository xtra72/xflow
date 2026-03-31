package lg

import "errors"

var (
	// ErrLGCPSerialPortRequired 는 시리얼 포트가 지정되지 않았을 때 반환된다.
	ErrLGCPSerialPortRequired = errors.New("lgcp: serial_port is required")

	// ErrLGCPInvalidBaudRate 는 잘못된 보레이트 값이 지정되었을 때 반환된다.
	ErrLGCPInvalidBaudRate = errors.New("lgcp: invalid baud_rate")

	// ErrLGCPInvalidFrameLen 은 프레임 길이가 유효하지 않을 때 반환된다.
	ErrLGCPInvalidFrameLen = errors.New("lgcp: invalid frame length")

	// ErrLGCPPartialFrame 은 불완전한 프레임이 수신되었을 때 반환된다.
	ErrLGCPPartialFrame = errors.New("lgcp: partial frame received")

	// ErrLGCPCRCMismatch 는 CRC 검증이 실패했을 때 반환된다.
	ErrLGCPCRCMismatch = errors.New("lgcp: CRC mismatch")

	// ErrLGCPNotConnected 는 트랜스포트가 연결되지 않은 상태에서 동작을 시도할 때 반환된다.
	ErrLGCPNotConnected = errors.New("lgcp: transport not connected")

	// ErrLGCPAlreadyRunning 은 에이전트가 이미 실행 중일 때 반환된다.
	ErrLGCPAlreadyRunning = errors.New("lgcp: agent already running")

	// ErrLGCPControlNotEnabled 는 제어 기능이 비활성화된 에이전트에서 제어를 시도할 때 반환된다.
	ErrLGCPControlNotEnabled = errors.New("lgcp: control not enabled for this agent")

	// ErrLGCPInvalidAddress 는 주소 형식이 올바르지 않을 때 반환된다.
	ErrLGCPInvalidAddress = errors.New("lgcp: invalid address format")

	// ErrLGCPTemperatureOutOfRange 는 설정 온도가 허용 범위를 벗어났을 때 반환된다.
	ErrLGCPTemperatureOutOfRange = errors.New("lgcp: temperature out of range")

	// ErrLGCPInvalidFanSpeed 는 유효하지 않은 팬 속도가 지정되었을 때 반환된다.
	ErrLGCPInvalidFanSpeed = errors.New("lgcp: invalid fan_speed")

	// ErrLGCPInvalidMode 는 유효하지 않은 운전 모드가 지정되었을 때 반환된다.
	ErrLGCPInvalidMode = errors.New("lgcp: invalid mode")

	// ErrLGCPMissingParam 은 필수 파라미터가 누락되었을 때 반환된다.
	ErrLGCPMissingParam = errors.New("lgcp: missing required parameter")

	// ErrLGCPSerialWriteFailed 는 시리얼 포트 쓰기가 실패했을 때 반환된다.
	ErrLGCPSerialWriteFailed = errors.New("lgcp: serial write failed")
)
