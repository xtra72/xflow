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

	// ErrUnsupportedByteOrder 는 지원하지 않는 바이트순서가 지정되었을 때 반환된다(REQ-03).
	// 별칭(big_endian/little_endian) 및 4순열(ABCD/BADC/CDAB/DCBA)만 허용된다.
	ErrUnsupportedByteOrder = errors.New("modbus: unsupported byte order")

	// ErrTypeMapOverlap 는 type_map 주소가 겹칠 때 반환된다.
	ErrTypeMapOverlap = errors.New("modbus: type_map addresses overlap")

	// ErrTypeMapOutOfRange 는 type_map 주소가 범위를 초과할 때 반환된다.
	ErrTypeMapOutOfRange = errors.New("modbus: type_map address out of range")

	// ErrCRCMismatch 는 RTU 응답의 CRC-16 재계산 검증이 실패했을 때 반환된다.
	// 손상된 프레임을 유효한 결과로 상위에 반환하지 않기 위해 사용한다(AC-07).
	ErrCRCMismatch = errors.New("modbus: RTU CRC mismatch")

	// ErrRTUFrameTooShort 는 RTU ADU 가 최소 길이(unitID+FC+CRC = 4바이트)보다 짧을 때 반환된다.
	ErrRTUFrameTooShort = errors.New("modbus: RTU frame too short")

	// ErrUnitIDMismatch 는 RTU 응답의 unitID 가 요청 unitID 와 일치하지 않을 때 반환된다.
	ErrUnitIDMismatch = errors.New("modbus: RTU response unit ID mismatch")

	// ErrFunctionCodeMismatch 는 RTU 응답의 function code 가 요청 function code
	// (또는 그 예외 형태 fc|0x80)와 일치하지 않을 때 반환된다.
	ErrFunctionCodeMismatch = errors.New("modbus: RTU response function code mismatch")

	// ErrSerialConnectionFailed 는 RTU 시리얼 포트 오픈/연결에 실패했을 때 반환된다.
	ErrSerialConnectionFailed = errors.New("modbus: RTU serial connection failed")

	// ErrInvalidTransport 는 transport 값이 "tcp"/"rtu" 가 아닐 때 반환된다.
	ErrInvalidTransport = errors.New("modbus: invalid transport (must be \"tcp\" or \"rtu\")")

	// ErrMissingSerialPort 는 transport 가 "rtu" 인데 serial_port 가 없을 때 반환된다.
	ErrMissingSerialPort = errors.New("modbus: serial_port is required for rtu transport")

	// ErrInvalidSerialParam 는 RTU 시리얼 파라미터(baud_rate/data_bits/stop_bits/parity)가
	// 유효 범위를 벗어났을 때 반환된다.
	ErrInvalidSerialParam = errors.New("modbus: invalid serial parameter")

	// ErrInitOnlyField 는 런타임 set_config 로 init 전용 필드(트랜스포트 tcp↔rtu 전환,
	// RTU 시리얼 하드웨어 파라미터 port/baud/data_bits/stop_bits/parity)의 변경을
	// 시도했을 때 반환된다(M9, AC-08). 이 필드들은 ADU 프레이밍·연결 토폴로지·시리얼 포트
	// 오픈에 귀속되므로 런타임 변경을 거부하고 에이전트는 직전 설정으로 계속 동작한다.
	ErrInitOnlyField = errors.New("modbus: field is init-only and cannot be changed at runtime")

	// ErrDuplicateDevice 는 런타임 add_device 로 이미 존재하는 device ID 를 추가하려 할 때
	// 반환된다(M4, AC-07). 중복 추가는 원자적으로 거부되며(부분 적용 없음) 에이전트는
	// 직전 상태로 계속 동작한다.
	ErrDuplicateDevice = errors.New("modbus: device with this ID already exists")

	// ErrMissingDeviceID 는 add_device/remove_device 명령에 device ID 가 없을 때 반환된다(M4).
	ErrMissingDeviceID = errors.New("modbus: device id is required")
)
