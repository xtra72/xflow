package lifecycle

// State 는 라이프사이클 컴포넌트의 상태를 나타내는 문자열 타입이다.
type State string

const (
	// StateCreated 는 컴포넌트가 생성된 초기 상태를 나타낸다.
	StateCreated State = "created"

	// StateInitializing 은 컴포넌트가 초기화 중인 상태를 나타낸다.
	StateInitializing State = "initializing"

	// StateRunning 은 컴포넌트가 실행 중인 상태를 나타낸다.
	StateRunning State = "running"

	// StatePaused 는 컴포넌트가 일시정지된 상태를 나타낸다.
	StatePaused State = "paused"

	// StateStopping 은 컴포넌트가 정지 중인 상태를 나타낸다.
	StateStopping State = "stopping"

	// StateStopped 는 컴포넌트가 정지된 상태를 나타낸다.
	StateStopped State = "stopped"

	// StateError 는 컴포넌트에 오류가 발생한 상태를 나타낸다.
	StateError State = "error"
)

// String 은 State의 사람이 읽을 수 있는 문자열 표현을 반환한다.
func (s State) String() string {
	return string(s)
}

// IsValid 는 해당 State가 7가지 유효한 상태 중 하나인지 확인한다.
func (s State) IsValid() bool {
	switch s {
	case StateCreated, StateInitializing, StateRunning,
		StatePaused, StateStopping, StateStopped, StateError:
		return true
	default:
		return false
	}
}

// ValidTransitions 는 각 State에서 전이 가능한 대상 상태 목록을 정의한다.
var ValidTransitions = map[State][]State{
	StateCreated:      {StateInitializing},
	StateInitializing: {StateRunning, StateError},
	StateRunning:      {StatePaused, StateStopping, StateError},
	StatePaused:       {StateRunning, StateStopping, StateError},
	StateStopping:     {StateStopped, StateError},
	StateStopped:      {StateCreated},
	StateError:        {StateStopping, StateStopped, StateCreated},
}

// IsValidTransition 은 from 상태에서 to 상태로의 전이가 유효한지 확인한다.
func IsValidTransition(from, to State) bool {
	targets, exists := ValidTransitions[from]
	if !exists {
		return false
	}
	for _, target := range targets {
		if target == to {
			return true
		}
	}
	return false
}

// ParseState 는 문자열을 State로 파싱한다.
// 알 수 없는 문자열이 제공되면 ErrInvalidState를 반환한다.
func ParseState(s string) (State, error) {
	switch State(s) {
	case StateCreated, StateInitializing, StateRunning,
		StatePaused, StateStopping, StateStopped, StateError:
		return State(s), nil
	default:
		return "", ErrInvalidState
	}
}
