package engine

import (
	"errors"
	"testing"
)

func TestSentinelErrors_Exist(t *testing.T) {
	// 모든 sentinel 에러가 존재하며 nil이 아닌지 검증한다.
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrFlowAlreadyDeployed", ErrFlowAlreadyDeployed},
		{"ErrFlowNotFound", ErrFlowNotFound},
		{"ErrFlowNotRunning", ErrFlowNotRunning},
		{"ErrFlowNotPaused", ErrFlowNotPaused},
		{"ErrFlowNotStopped", ErrFlowNotStopped},
		{"ErrFlowNotLoaded", ErrFlowNotLoaded},
		{"ErrFlowValidationFailed", ErrFlowValidationFailed},
		{"ErrCycleDetected", ErrCycleDetected},
		{"ErrStateMapping", ErrStateMapping},
		{"ErrChannelClosed", ErrChannelClosed},
		{"ErrNodeStartFailed", ErrNodeStartFailed},
		{"ErrShutdownTimeout", ErrShutdownTimeout},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			if tc.err == nil {
				t.Errorf("%s should not be nil", tc.name)
			}
		})
	}
}

func TestSentinelErrors_ErrorsIs(t *testing.T) {
	// errors.Is()로 sentinel 에러를 매칭할 수 있는지 검증한다.
	sentinels := []error{
		ErrFlowAlreadyDeployed,
		ErrFlowNotFound,
		ErrFlowNotRunning,
		ErrFlowNotPaused,
		ErrFlowNotStopped,
		ErrFlowNotLoaded,
		ErrFlowValidationFailed,
		ErrCycleDetected,
		ErrStateMapping,
		ErrChannelClosed,
		ErrNodeStartFailed,
		ErrShutdownTimeout,
	}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			if !errors.Is(sentinel, sentinel) {
				t.Errorf("errors.Is(%v, %v) should return true", sentinel, sentinel)
			}
		})
	}
}

func TestSentinelErrors_UniqueMessages(t *testing.T) {
	// 모든 에러 메시지가 고유한지 검증한다.
	sentinels := []error{
		ErrFlowAlreadyDeployed,
		ErrFlowNotFound,
		ErrFlowNotRunning,
		ErrFlowNotPaused,
		ErrFlowNotStopped,
		ErrFlowNotLoaded,
		ErrFlowValidationFailed,
		ErrCycleDetected,
		ErrStateMapping,
		ErrChannelClosed,
		ErrNodeStartFailed,
		ErrShutdownTimeout,
	}

	seen := make(map[string]bool)
	for _, err := range sentinels {
		msg := err.Error()
		if seen[msg] {
			t.Errorf("duplicate error message: %s", msg)
		}
		seen[msg] = true
	}
}

func TestSentinelErrors_HavePrefix(t *testing.T) {
	// 모든 에러가 "engine:" 접두사를 가지는지 검증한다.
	sentinels := []error{
		ErrFlowAlreadyDeployed,
		ErrFlowNotFound,
		ErrFlowNotRunning,
		ErrFlowNotPaused,
		ErrFlowNotStopped,
		ErrFlowNotLoaded,
		ErrFlowValidationFailed,
		ErrCycleDetected,
		ErrStateMapping,
		ErrChannelClosed,
		ErrNodeStartFailed,
		ErrShutdownTimeout,
	}

	for _, err := range sentinels {
		msg := err.Error()
		if len(msg) < 7 || msg[:7] != "engine:" {
			t.Errorf("error %q should have 'engine:' prefix", msg)
		}
	}
}
