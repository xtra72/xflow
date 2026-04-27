package lifecycle

import "errors"

// 패키지 수준 에러 정의
var (
	// ErrInvalidState 는 유효하지 않은 상태 값이 제공되었을 때 반환된다.
	ErrInvalidState = errors.New("lifecycle: invalid state")

	// ErrInvalidStateTransition 은 허용되지 않은 상태 전이를 시도할 때 반환된다.
	ErrInvalidStateTransition = errors.New("lifecycle: invalid state transition")

	// ErrInvalidStateForConfigure 는 설정 변경이 불가능한 상태에서 Configure를 호출할 때 반환된다.
	ErrInvalidStateForConfigure = errors.New("lifecycle: invalid state for configure")

	// ErrAlreadyInitialized 는 이미 초기화된 컴포넌트에 Init을 재호출할 때 반환된다.
	ErrAlreadyInitialized = errors.New("lifecycle: already initialized")

	// ErrNotRunning 은 실행 중이 아닌 상태에서 실행 전제 연산을 시도할 때 반환된다.
	ErrNotRunning = errors.New("lifecycle: not running")

	// ErrNotPaused 는 일시정지 상태가 아닌데 Resume을 호출할 때 반환된다.
	ErrNotPaused = errors.New("lifecycle: not paused")
)
