package flow

// FlowState 는 Flow의 생명주기 상태를 나타내는 문자열 타입이다.
type FlowState string

const (
	// FlowStored 는 Flow가 저장소에 저장된 상태를 나타낸다.
	FlowStored FlowState = "stored"

	// FlowLoaded 는 Flow가 메모리에 로드된 상태를 나타낸다.
	FlowLoaded FlowState = "loaded"

	// FlowInitializing 은 Flow가 초기화 중인 상태를 나타낸다.
	FlowInitializing FlowState = "initializing"

	// FlowRunning 은 Flow가 실행 중인 상태를 나타낸다.
	FlowRunning FlowState = "running"

	// FlowPaused 는 Flow가 일시정지된 상태를 나타낸다.
	FlowPaused FlowState = "paused"

	// FlowStopping 은 Flow가 정지 중인 상태를 나타낸다.
	FlowStopping FlowState = "stopping"

	// FlowStopped 는 Flow가 정지된 상태를 나타낸다.
	FlowStopped FlowState = "stopped"

	// FlowError 는 Flow에 오류가 발생한 상태를 나타낸다.
	FlowError FlowState = "error"
)

// String 은 FlowState의 사람이 읽을 수 있는 문자열 표현을 반환한다.
func (s FlowState) String() string {
	return string(s)
}

// ValidTransitions 는 각 FlowState에서 전이 가능한 대상 상태 목록을 정의한다.
var ValidTransitions = map[FlowState][]FlowState{
	FlowStored:       {FlowLoaded},
	FlowLoaded:       {FlowInitializing, FlowStored},
	FlowInitializing: {FlowRunning, FlowError, FlowStopped},
	FlowRunning:      {FlowPaused, FlowStopping, FlowError},
	FlowPaused:       {FlowRunning, FlowStopping, FlowError},
	FlowStopping:     {FlowStopped, FlowError},
	FlowStopped:      {FlowLoaded, FlowStored},
	FlowError:        {FlowStopped, FlowInitializing},
}

// IsValidTransition 은 from 상태에서 to 상태로의 전이가 유효한지 확인한다.
func IsValidTransition(from, to FlowState) bool {
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

// ParseFlowState 는 문자열을 FlowState로 파싱한다.
// 알 수 없는 문자열이 제공되면 ErrInvalidFlowState를 반환한다.
func ParseFlowState(s string) (FlowState, error) {
	switch FlowState(s) {
	case FlowStored, FlowLoaded, FlowInitializing, FlowRunning,
		FlowPaused, FlowStopping, FlowStopped, FlowError:
		return FlowState(s), nil
	default:
		return "", ErrInvalidFlowState
	}
}
