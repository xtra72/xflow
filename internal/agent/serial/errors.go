package serial

import "errors"

// 시리얼 에이전트 패키지의 센티넬 에러 정의.
var (
	// ErrPortRequired 는 포트 경로가 지정되지 않았을 때 반환된다.
	ErrPortRequired = errors.New("serial: port path is required")

	// ErrInvalidBaudRate 는 지원되지 않는 보드레이트일 때 반환된다.
	ErrInvalidBaudRate = errors.New("serial: invalid baud_rate")

	// ErrInvalidDataBits 는 지원되지 않는 데이터 비트일 때 반환된다.
	ErrInvalidDataBits = errors.New("serial: invalid data_bits")

	// ErrInvalidStopBits 는 지원되지 않는 스톱 비트일 때 반환된다.
	ErrInvalidStopBits = errors.New("serial: invalid stop_bits")

	// ErrInvalidParity 는 지원되지 않는 패리티일 때 반환된다.
	ErrInvalidParity = errors.New("serial: invalid parity")

	// ErrInvalidFraming 는 지원되지 않는 프레이밍 타입일 때 반환된다.
	ErrInvalidFraming = errors.New("serial: unsupported framing type")

	// ErrFixedSizeRequired 는 fixed_size 프레이밍에서 크기가 지정되지 않았을 때 반환된다.
	ErrFixedSizeRequired = errors.New("serial: fixed_size required when framing is fixed_size")

	// ErrPortNotFound 는 시리얼 포트를 찾을 수 없을 때 반환된다.
	ErrPortNotFound = errors.New("serial: port not found")

	// ErrPermissionDenied 는 시리얼 포트 접근 권한이 없을 때 반환된다.
	ErrPermissionDenied = errors.New("serial: permission denied")

	// ErrDeviceDisconnected 는 장치가 연결 해제되었을 때 반환된다.
	ErrDeviceDisconnected = errors.New("serial: device disconnected")

	// ErrPortClosed 는 포트가 이미 닫혀있을 때 반환된다.
	ErrPortClosed = errors.New("serial: port is closed")

	// ErrNotRunning 는 에이전트가 실행 중이지 않을 때 반환된다.
	ErrNotRunning = errors.New("serial: agent is not running")

	// ErrMaxMessageSize 는 메시지가 최대 크기를 초과했을 때 반환된다.
	ErrMaxMessageSize = errors.New("serial: message exceeds max size")
)
