package modbusserver

import "errors"

var (
	// ErrServerAlreadyRunning 는 서버가 이미 실행 중일 때 반환된다.
	ErrServerAlreadyRunning = errors.New("modbus-server: server is already running")

	// ErrListenFailed 는 TCP 리슨에 실패했을 때 반환된다.
	ErrListenFailed = errors.New("modbus-server: TCP listen failed")

	// ErrMaxConnectionsReached 는 최대 연결 수에 도달했을 때 반환된다.
	ErrMaxConnectionsReached = errors.New("modbus-server: max connections reached")

	// ErrInvalidRegisterMap 는 레지스터 맵 설정이 유효하지 않을 때 반환된다.
	ErrInvalidRegisterMap = errors.New("modbus-server: invalid register map configuration")

	// ErrAddressNotMapped 는 요청된 주소가 매핑되지 않았을 때 반환된다.
	ErrAddressNotMapped = errors.New("modbus-server: address not mapped")

	// ErrReadOnlyArea 는 읽기 전용 영역에 쓰기를 시도했을 때 반환된다.
	ErrReadOnlyArea = errors.New("modbus-server: cannot write to read-only area")

	// ErrInvalidCommand 는 유효하지 않은 명령이 사용되었을 때 반환된다.
	ErrInvalidCommand = errors.New("modbus-server: invalid command")

	// ErrTypeMapOverlap 는 type_map 주소가 겹칠 때 반환된다.
	ErrTypeMapOverlap = errors.New("modbus-server: type_map addresses overlap")

	// ErrTypeMapOutOfRange 는 type_map 주소가 범위를 초과할 때 반환된다.
	ErrTypeMapOutOfRange = errors.New("modbus-server: type_map address out of range")

	// ErrUnsupportedDataType 는 지원하지 않는 데이터 타입일 때 반환된다.
	// 공유 패키지(internal/modbus)의 ErrUnsupportedDataType와 동일한 의미이나,
	// 에이전트 계층에서 "modbus-server:" 접두어로 에러 출처를 구분하기 위해 별도로 정의한다.
	ErrUnsupportedDataType = errors.New("modbus-server: unsupported data type")

	// ErrInvalidDeviceConfig 는 디바이스 설정이 유효하지 않을 때 반환된다.
	ErrInvalidDeviceConfig = errors.New("modbus-server: invalid device configuration")

	// ErrDuplicateUnitID 는 디바이스 목록에서 Unit ID 가 중복될 때 반환된다.
	ErrDuplicateUnitID = errors.New("modbus-server: duplicate unit ID")

	// ErrDeviceNotFound 는 요청된 Unit ID 에 해당하는 디바이스를 찾을 수 없을 때 반환된다.
	ErrDeviceNotFound = errors.New("modbus-server: device not found")

	// ErrMissingSerialPort 는 RTU 트랜스포트에서 serial_port 가 지정되지 않았을 때 반환된다.
	ErrMissingSerialPort = errors.New("modbus-server: serial_port is required for RTU transport")

	// ErrInvalidSerialParam 는 RTU 시리얼 파라미터가 유효하지 않을 때 반환된다.
	ErrInvalidSerialParam = errors.New("modbus-server: invalid serial parameter")

	// ErrInvalidSharedConfig 는 공유 레지스터 맵(role/shared_from) 설정이 유효하지 않을 때 반환된다.
	ErrInvalidSharedConfig = errors.New("modbus-server: invalid shared register map configuration")

	// ErrSharedMainNotFound 는 role=sub 서버가 shared_from 이 가리키는 주 서버를
	// 찾지 못하거나, 대상이 modbus-server 가 아닐 때 반환된다.
	ErrSharedMainNotFound = errors.New("modbus-server: shared main agent not found or not a modbus-server")

	// ErrSharedMainNoDevices 는 role=sub 서버가 상속할 주 서버에 디바이스가 하나도
	// 없을 때 반환된다(공유할 대상 레지스터 맵이 없음).
	ErrSharedMainNoDevices = errors.New("modbus-server: shared main agent has no devices to share")

	// ErrSharedMapMissing 는 공유 세그먼트(shared_address)가 있으나 unit_id 0
	// 공유 컨테이너가 없을 때 반환된다(intra-server 공유).
	ErrSharedMapMissing = errors.New("modbus-server: shared segment requires a unit_id 0 container")

	// ErrSharedRangeOutOfBounds 는 공유 세그먼트의 shared_address 범위가 컨테이너의
	// 같은 영역 선언 범위를 벗어날 때 반환된다.
	ErrSharedRangeOutOfBounds = errors.New("modbus-server: shared range out of container bounds")

	// ErrSharedUnderContainer 는 unit_id 0 컨테이너의 세그먼트에 shared_address 가
	// 지정되었을 때 반환된다(컨테이너 세그먼트는 모두 로컬이어야 함).
	ErrSharedUnderContainer = errors.New("modbus-server: shared_address not allowed under unit_id 0")
)
