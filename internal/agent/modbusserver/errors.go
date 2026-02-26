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
)
