package lifecycle

import (
	"errors"
	"fmt"
	"testing"
)

// TestSentinelErrors_NotNil 은 모든 센티넬 에러가 nil이 아닌지 검증한다.
func TestSentinelErrors_NotNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{name: "ErrInvalidState", err: ErrInvalidState},
		{name: "ErrInvalidStateTransition", err: ErrInvalidStateTransition},
		{name: "ErrInvalidStateForConfigure", err: ErrInvalidStateForConfigure},
		{name: "ErrAlreadyInitialized", err: ErrAlreadyInitialized},
		{name: "ErrNotRunning", err: ErrNotRunning},
		{name: "ErrNotPaused", err: ErrNotPaused},
	}

	for _, tt := range sentinels {
		t.Run(tt.name+"_nil이_아님", func(t *testing.T) {
			if tt.err == nil {
				t.Errorf("%s 가 nil이다", tt.name)
			}
		})
	}
}

// TestSentinelErrors_Message 는 각 센티넬 에러의 메시지가 올바른지 검증한다.
func TestSentinelErrors_Message(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{name: "ErrInvalidState 메시지", err: ErrInvalidState, expected: "lifecycle: invalid state"},
		{name: "ErrInvalidStateTransition 메시지", err: ErrInvalidStateTransition, expected: "lifecycle: invalid state transition"},
		{name: "ErrInvalidStateForConfigure 메시지", err: ErrInvalidStateForConfigure, expected: "lifecycle: invalid state for configure"},
		{name: "ErrAlreadyInitialized 메시지", err: ErrAlreadyInitialized, expected: "lifecycle: already initialized"},
		{name: "ErrNotRunning 메시지", err: ErrNotRunning, expected: "lifecycle: not running"},
		{name: "ErrNotPaused 메시지", err: ErrNotPaused, expected: "lifecycle: not paused"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.expected {
				t.Errorf("%s.Error() = %q, 기대값 %q", tt.name, got, tt.expected)
			}
		})
	}
}

// TestSentinelErrors_Uniqueness 는 모든 센티넬 에러가 서로 다른 인스턴스인지 검증한다.
func TestSentinelErrors_Uniqueness(t *testing.T) {
	sentinels := []error{
		ErrInvalidState,
		ErrInvalidStateTransition,
		ErrInvalidStateForConfigure,
		ErrAlreadyInitialized,
		ErrNotRunning,
		ErrNotPaused,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			if errors.Is(sentinels[i], sentinels[j]) {
				t.Errorf("센티넬 에러 [%d]과 [%d]가 동일하다: %v == %v",
					i, j, sentinels[i], sentinels[j])
			}
		}
	}
}

// TestSentinelErrors_IsCompatibility 는 fmt.Errorf로 래핑된 에러에서 errors.Is가 정상 동작하는지 검증한다 (AC-LIFE-001-31, AC-LIFE-001-32).
func TestSentinelErrors_IsCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		sentinel error
	}{
		{name: "ErrInvalidState 래핑", sentinel: ErrInvalidState},
		{name: "ErrInvalidStateTransition 래핑", sentinel: ErrInvalidStateTransition},
		{name: "ErrInvalidStateForConfigure 래핑", sentinel: ErrInvalidStateForConfigure},
		{name: "ErrAlreadyInitialized 래핑", sentinel: ErrAlreadyInitialized},
		{name: "ErrNotRunning 래핑", sentinel: ErrNotRunning},
		{name: "ErrNotPaused 래핑", sentinel: ErrNotPaused},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// fmt.Errorf %w 로 래핑한 에러에서 원본을 찾을 수 있어야 한다
			wrapped := fmt.Errorf("컨텍스트 추가: %w", tt.sentinel)
			if !errors.Is(wrapped, tt.sentinel) {
				t.Errorf("errors.Is(wrapped, %v) = false, 래핑된 에러에서 원본을 찾을 수 있어야 한다", tt.sentinel)
			}

			// 이중 래핑에서도 동작해야 한다
			doubleWrapped := fmt.Errorf("외부 래핑: %w", wrapped)
			if !errors.Is(doubleWrapped, tt.sentinel) {
				t.Errorf("errors.Is(doubleWrapped, %v) = false, 이중 래핑에서도 원본을 찾을 수 있어야 한다", tt.sentinel)
			}
		})
	}
}
