package node

import "errors"

// Bridge 전용 센티널 에러 정의
// 기존 errors.go의 에러(ErrAgentNotFound, ErrRequestTimeout 등)와 중복되지 않는다.
var (
	// ErrAgentNotRunning 은 바인딩된 에이전트가 실행 상태가 아닐 때 반환된다.
	ErrAgentNotRunning = errors.New("bridge: agent not running")

	// ErrAgentDisconnected 는 에이전트 연결이 끊어졌을 때 반환된다.
	ErrAgentDisconnected = errors.New("bridge: agent disconnected")

	// ErrCorrelationNotFound 는 등록되지 않은 상관관계 ID를 조회할 때 반환된다.
	ErrCorrelationNotFound = errors.New("bridge: correlation ID not found")

	// ErrTransformFailed 는 메시지 변환에 실패했을 때 반환된다.
	ErrTransformFailed = errors.New("bridge: message transform failed")

	// ErrInvalidDirection 은 유효하지 않은 BridgeDirection이 지정되었을 때 반환된다.
	ErrInvalidDirection = errors.New("bridge: invalid bridge direction")

	// ErrMaxReconnectExceeded 는 최대 재연결 시도 횟수를 초과했을 때 반환된다.
	ErrMaxReconnectExceeded = errors.New("bridge: max reconnect attempts exceeded")

	// ErrBufferFull 은 수신 버퍼가 가득 찼을 때 반환된다.
	ErrBufferFull = errors.New("bridge: receive buffer full")
)
