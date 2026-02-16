package engine

import (
	"errors"
	"testing"

	"github.com/xtra/xflow/pkg/flow"
	"github.com/xtra/xflow/pkg/lifecycle"
)

func TestMapFlowStateToLifecycleState(t *testing.T) {
	tests := []struct {
		name      string
		flowState flow.FlowState
		want      lifecycle.State
		wantErr   error
	}{
		{
			name:      "FlowLoaded -> StateCreated",
			flowState: flow.FlowLoaded,
			want:      lifecycle.StateCreated,
		},
		{
			name:      "FlowInitializing -> StateInitializing",
			flowState: flow.FlowInitializing,
			want:      lifecycle.StateInitializing,
		},
		{
			name:      "FlowRunning -> StateRunning",
			flowState: flow.FlowRunning,
			want:      lifecycle.StateRunning,
		},
		{
			name:      "FlowPaused -> StatePaused",
			flowState: flow.FlowPaused,
			want:      lifecycle.StatePaused,
		},
		{
			name:      "FlowStopping -> StateStopping",
			flowState: flow.FlowStopping,
			want:      lifecycle.StateStopping,
		},
		{
			name:      "FlowStopped -> StateStopped",
			flowState: flow.FlowStopped,
			want:      lifecycle.StateStopped,
		},
		{
			name:      "FlowError -> StateError",
			flowState: flow.FlowError,
			want:      lifecycle.StateError,
		},
		{
			name:      "FlowStored -> error (no mapping)",
			flowState: flow.FlowStored,
			wantErr:   ErrStateMapping,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := MapFlowStateToLifecycleState(tc.flowState)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error %v, got nil", tc.wantErr)
				}
				if !errors.Is(err, tc.wantErr) {
					t.Errorf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestMapLifecycleStateToFlowState(t *testing.T) {
	tests := []struct {
		name           string
		lifecycleState lifecycle.State
		want           flow.FlowState
	}{
		{
			name:           "StateCreated -> FlowLoaded",
			lifecycleState: lifecycle.StateCreated,
			want:           flow.FlowLoaded,
		},
		{
			name:           "StateInitializing -> FlowInitializing",
			lifecycleState: lifecycle.StateInitializing,
			want:           flow.FlowInitializing,
		},
		{
			name:           "StateRunning -> FlowRunning",
			lifecycleState: lifecycle.StateRunning,
			want:           flow.FlowRunning,
		},
		{
			name:           "StatePaused -> FlowPaused",
			lifecycleState: lifecycle.StatePaused,
			want:           flow.FlowPaused,
		},
		{
			name:           "StateStopping -> FlowStopping",
			lifecycleState: lifecycle.StateStopping,
			want:           flow.FlowStopping,
		},
		{
			name:           "StateStopped -> FlowStopped",
			lifecycleState: lifecycle.StateStopped,
			want:           flow.FlowStopped,
		},
		{
			name:           "StateError -> FlowError",
			lifecycleState: lifecycle.StateError,
			want:           flow.FlowError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MapLifecycleStateToFlowState(tc.lifecycleState)
			if got != tc.want {
				t.Errorf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestMapFlowStateToLifecycleState_UnknownState(t *testing.T) {
	// 알 수 없는 FlowState에 대해 에러를 반환하는지 검증한다.
	_, err := MapFlowStateToLifecycleState(flow.FlowState("unknown"))
	if err == nil {
		t.Fatal("expected error for unknown FlowState, got nil")
	}
	if !errors.Is(err, ErrStateMapping) {
		t.Errorf("expected ErrStateMapping, got %v", err)
	}
}
