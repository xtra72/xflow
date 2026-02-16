package engine

import (
	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// MapFlowStateToLifecycleState 는 flow.FlowState를 lifecycle.State로 매핑한다.
// FlowStored 상태는 lifecycle에 대응하는 상태가 없으므로 ErrStateMapping을 반환한다.
func MapFlowStateToLifecycleState(fs flow.FlowState) (lifecycle.State, error) {
	switch fs {
	case flow.FlowLoaded:
		return lifecycle.StateCreated, nil
	case flow.FlowInitializing:
		return lifecycle.StateInitializing, nil
	case flow.FlowRunning:
		return lifecycle.StateRunning, nil
	case flow.FlowPaused:
		return lifecycle.StatePaused, nil
	case flow.FlowStopping:
		return lifecycle.StateStopping, nil
	case flow.FlowStopped:
		return lifecycle.StateStopped, nil
	case flow.FlowError:
		return lifecycle.StateError, nil
	default:
		return "", ErrStateMapping
	}
}

// MapLifecycleStateToFlowState 는 lifecycle.State를 flow.FlowState로 매핑한다.
func MapLifecycleStateToFlowState(s lifecycle.State) flow.FlowState {
	switch s {
	case lifecycle.StateCreated:
		return flow.FlowLoaded
	case lifecycle.StateInitializing:
		return flow.FlowInitializing
	case lifecycle.StateRunning:
		return flow.FlowRunning
	case lifecycle.StatePaused:
		return flow.FlowPaused
	case lifecycle.StateStopping:
		return flow.FlowStopping
	case lifecycle.StateStopped:
		return flow.FlowStopped
	case lifecycle.StateError:
		return flow.FlowError
	default:
		return flow.FlowError
	}
}
