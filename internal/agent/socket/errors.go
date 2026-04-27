package socket

import "errors"

// 소켓 에이전트 패키지의 센티넬 에러 정의.
var (
	// ErrMaxConnections 는 최대 연결 수에 도달했을 때 반환된다.
	ErrMaxConnections = errors.New("socket: max connections reached")

	// ErrConnectionBlocked 는 차단된 주소에서의 연결 시 반환된다.
	ErrConnectionBlocked = errors.New("socket: connection from blocked address")

	// ErrReconnectFailed 는 재연결 시도가 모두 소진되었을 때 반환된다.
	ErrReconnectFailed = errors.New("socket: reconnection attempts exhausted")

	// ErrFramingError 는 프레이밍/파싱 오류 시 반환된다.
	ErrFramingError = errors.New("socket: framing error")

	// ErrConnectionTimeout 는 연결 타임아웃 시 반환된다.
	ErrConnectionTimeout = errors.New("socket: connection timeout")

	// ErrInvalidConfig 는 잘못된 설정 시 반환된다.
	ErrInvalidConfig = errors.New("socket: invalid configuration")

	// ErrMaxMessageSize 는 메시지가 최대 크기를 초과했을 때 반환된다.
	ErrMaxMessageSize = errors.New("socket: message exceeds max size")

	// ErrPortRequired 는 포트가 지정되지 않았을 때 반환된다.
	ErrPortRequired = errors.New("socket: port not specified")

	// ErrInvalidFraming 는 지원되지 않는 프레이밍 타입일 때 반환된다.
	ErrInvalidFraming = errors.New("socket: unsupported framing type")

	// ErrFixedSizeRequired 는 fixed_size 프레이밍에서 크기가 지정되지 않았을 때 반환된다.
	ErrFixedSizeRequired = errors.New("socket: fixed_size required when framing is fixed_size")
)
