package engine

import "errors"

var (
	// ErrFlowAlreadyDeployed 는 이미 배포된 Flow를 다시 배포하려 할 때 반환된다.
	ErrFlowAlreadyDeployed = errors.New("engine: flow already deployed")

	// ErrFlowNotFound 는 존재하지 않는 Flow를 참조할 때 반환된다.
	ErrFlowNotFound = errors.New("engine: flow not found")

	// ErrFlowNotRunning 은 실행 중이지 않은 Flow에 대해 실행 중 상태를 요구하는 작업을 시도할 때 반환된다.
	ErrFlowNotRunning = errors.New("engine: flow not running")

	// ErrFlowNotPaused 는 일시정지 상태가 아닌 Flow에 대해 재개를 시도할 때 반환된다.
	ErrFlowNotPaused = errors.New("engine: flow not paused")

	// ErrFlowNotStopped 는 정지 상태가 아닌 Flow에 대해 배포 해제를 시도할 때 반환된다.
	ErrFlowNotStopped = errors.New("engine: flow not stopped")

	// ErrFlowNotLoaded 는 로드되지 않은 Flow에 대해 시작을 시도할 때 반환된다.
	ErrFlowNotLoaded = errors.New("engine: flow not loaded")

	// ErrFlowValidationFailed 는 Flow 유효성 검사에 실패했을 때 반환된다.
	ErrFlowValidationFailed = errors.New("engine: flow validation failed")

	// ErrCycleDetected 는 Flow 그래프에서 순환이 발견되었을 때 반환된다.
	ErrCycleDetected = errors.New("engine: cycle detected in flow graph")

	// ErrStateMapping 은 FlowState와 lifecycle.State 간 매핑에 실패했을 때 반환된다.
	ErrStateMapping = errors.New("engine: flow state to lifecycle state mapping failed")

	// ErrChannelClosed 는 닫힌 채널에 메시지를 전송하려 할 때 반환된다.
	ErrChannelClosed = errors.New("engine: channel closed")

	// ErrNodeStartFailed 는 노드 시작에 실패했을 때 반환된다.
	ErrNodeStartFailed = errors.New("engine: node start failed")

	// ErrShutdownTimeout 은 종료 대기 시간이 초과되었을 때 반환된다.
	ErrShutdownTimeout = errors.New("engine: shutdown timeout exceeded")

	// ErrNodeNotFound 는 플로우 내에 존재하지 않는 노드를 참조할 때 반환된다.
	ErrNodeNotFound = errors.New("engine: node not found in flow")

	// ErrAgentRefNotFound 는 플로우 배포 시 노드가 참조하는 에이전트가 존재하지 않을 때 반환된다.
	ErrAgentRefNotFound = errors.New("engine: agent reference not found")
)
