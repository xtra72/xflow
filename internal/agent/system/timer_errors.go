package system

import "errors"

// Timer 에러 정의 - 타이머 연산에서 발생할 수 있는 센티넬 에러들이다.
var (
	// ErrIntervalTooShort 는 지정된 인터벌이 최소값(기본 100ms) 미만일 때 반환된다.
	ErrIntervalTooShort = errors.New("timer: interval below minimum (100ms)")

	// ErrInvalidCronExpression 은 유효하지 않은 cron 표현식이 제공되었을 때 반환된다.
	ErrInvalidCronExpression = errors.New("timer: invalid cron expression")

	// ErrTimerNotFound 는 요청한 타이머 ID가 존재하지 않을 때 반환된다.
	ErrTimerNotFound = errors.New("timer: timer not found")

	// ErrTimerIDEmpty 는 타이머 ID가 빈 문자열일 때 반환된다.
	ErrTimerIDEmpty = errors.New("timer: timer ID cannot be empty")

	// ErrTimerClosed 는 닫힌 타이머 에이전트에 연산을 시도할 때 반환된다.
	ErrTimerClosed = errors.New("timer: timer agent is closed")

	// ErrTimerPaused 는 일시정지된 타이머 에이전트에 등록을 시도할 때 반환된다.
	ErrTimerPaused = errors.New("timer: timer agent is paused (registration disabled)")

	// ErrNilHandler 는 nil 핸들러가 제공되었을 때 반환된다.
	ErrNilHandler = errors.New("timer: nil handler not allowed")

	// ErrDuplicateTimerID 는 이미 존재하는 타이머 ID로 등록을 시도할 때 반환된다.
	ErrDuplicateTimerID = errors.New("timer: duplicate timer ID")

	// ErrMaxTimersReached 는 최대 타이머 수에 도달했을 때 반환된다.
	ErrMaxTimersReached = errors.New("timer: maximum number of timers reached")

	// ErrInvalidDelay 는 유효하지 않은 지연 시간(0 이하)이 제공되었을 때 반환된다.
	ErrInvalidDelay = errors.New("timer: invalid delay (must be > 0)")
)
