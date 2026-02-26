package modbus

import "errors"

var (
	// ErrReadOnlyRegister 는 읽기 전용 레지스터에 쓰기를 시도할 때 반환된다.
	ErrReadOnlyRegister = errors.New("modbus: cannot write to read-only register")

	// ErrAddressOutOfRange 는 레지스터 주소가 유효 범위를 벗어났을 때 반환된다.
	ErrAddressOutOfRange = errors.New("modbus: register address out of range (0-65535)")

	// ErrQuantityExceeded 는 요청 수량이 최대 허용치를 초과했을 때 반환된다.
	ErrQuantityExceeded = errors.New("modbus: quantity exceeds maximum allowed")

	// ErrWriteTimeout 는 쓰기 응답 대기 시간이 초과했을 때 반환된다.
	ErrWriteTimeout = errors.New("modbus: write response timeout")

	// ErrModbusException 는 MODBUS 디바이스가 예외 응답을 반환했을 때 반환된다.
	ErrModbusException = errors.New("modbus: device returned exception")

	// ErrDeviceOffline 는 디바이스가 오프라인 상태일 때 반환된다.
	ErrDeviceOffline = errors.New("modbus: device is offline")

	// ErrDeviceNotFound 는 지정된 디바이스를 찾을 수 없을 때 반환된다.
	ErrDeviceNotFound = errors.New("modbus: device not found")

	// ErrInvalidFunctionCode 는 유효하지 않은 기능 코드가 사용되었을 때 반환된다.
	ErrInvalidFunctionCode = errors.New("modbus: invalid function code")

	// ErrInvalidCommand 는 유효하지 않은 명령이 사용되었을 때 반환된다.
	ErrInvalidCommand = errors.New("modbus: invalid command")

	// ErrConnectionFailed 는 TCP 연결에 실패했을 때 반환된다.
	ErrConnectionFailed = errors.New("modbus: TCP connection failed")

	// ErrFrameTooShort 는 응답 프레임이 최소 길이보다 짧을 때 반환된다.
	ErrFrameTooShort = errors.New("modbus: response frame too short")

	// ErrCacheNotFound 는 디바이스 캐시를 찾을 수 없을 때 반환된다.
	ErrCacheNotFound = errors.New("modbus: device cache not found")
)
