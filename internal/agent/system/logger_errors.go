package system

import "errors"

// Logger 에러 정의 - 로거 에이전트 연산에서 발생할 수 있는 센티넬 에러들이다.
var (
	// ErrLoggerClosed 는 정지된 로거 에이전트에 연산을 시도할 때 반환된다.
	ErrLoggerClosed = errors.New("logger: logger agent is closed")

	// ErrLoggerPaused 는 일시정지된 로거 에이전트에서 Debug/Info 로그가 억제될 때 반환된다.
	ErrLoggerPaused = errors.New("logger: logger agent is paused (debug/info suppressed)")

	// ErrInvalidLevel 은 유효하지 않은 로그 레벨이 지정되었을 때 반환된다.
	ErrInvalidLevel = errors.New("logger: invalid log level")

	// ErrInvalidComponent 는 빈 문자열 등 유효하지 않은 컴포넌트 이름이 지정되었을 때 반환된다.
	ErrInvalidComponent = errors.New("logger: invalid component name")

	// ErrSubscriptionNotFound 는 존재하지 않는 구독 ID로 해제를 시도할 때 반환된다.
	ErrSubscriptionNotFound = errors.New("logger: subscription not found")
)
