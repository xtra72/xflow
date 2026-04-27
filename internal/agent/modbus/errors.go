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

	// ErrUnsupportedDataType 는 지원하지 않는 데이터 타입이 지정되었을 때 반환된다.
	// 공유 패키지(internal/modbus)에도 동일 이름의 에러가 있으나,
	// 에러 접두사("modbus:")가 동일하여 클라이언트 에이전트 내부에서 독립적으로 정의한다.
	// config 검증 시 이 에러를 반환하며, 공유 패키지의 타입 변환 에러와 구분된다.
	ErrUnsupportedDataType = errors.New("modbus: unsupported data type")

	// ErrTypeMapOverlap 는 type_map 주소가 겹칠 때 반환된다.
	ErrTypeMapOverlap = errors.New("modbus: type_map addresses overlap")

	// ErrTypeMapOutOfRange 는 type_map 주소가 범위를 초과할 때 반환된다.
	ErrTypeMapOutOfRange = errors.New("modbus: type_map address out of range")
)
