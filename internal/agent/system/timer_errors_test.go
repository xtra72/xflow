package system

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimerErrors_NonNil 은 모든 센티넬 에러가 nil이 아닌지 확인한다.
func TestTimerErrors_NonNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrIntervalTooShort", ErrIntervalTooShort},
		{"ErrInvalidCronExpression", ErrInvalidCronExpression},
		{"ErrTimerNotFound", ErrTimerNotFound},
		{"ErrTimerIDEmpty", ErrTimerIDEmpty},
		{"ErrTimerClosed", ErrTimerClosed},
		{"ErrTimerPaused", ErrTimerPaused},
		{"ErrNilHandler", ErrNilHandler},
		{"ErrDuplicateTimerID", ErrDuplicateTimerID},
		{"ErrMaxTimersReached", ErrMaxTimersReached},
		{"ErrInvalidDelay", ErrInvalidDelay},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.err, "%s 는 nil이 아니어야 한다", tc.name)
		})
	}
}

// TestTimerErrors_UniqueMessages 는 모든 센티넬 에러의 메시지가 서로 다른지 확인한다.
func TestTimerErrors_UniqueMessages(t *testing.T) {
	sentinels := []error{
		ErrIntervalTooShort,
		ErrInvalidCronExpression,
		ErrTimerNotFound,
		ErrTimerIDEmpty,
		ErrTimerClosed,
		ErrTimerPaused,
		ErrNilHandler,
		ErrDuplicateTimerID,
		ErrMaxTimersReached,
		ErrInvalidDelay,
	}

	seen := make(map[string]bool)
	for _, err := range sentinels {
		msg := err.Error()
		assert.False(t, seen[msg], "중복된 에러 메시지 발견: %s", msg)
		seen[msg] = true
	}
}

// TestTimerErrors_ErrorsIs 는 fmt.Errorf로 래핑한 에러가 errors.Is로 식별되는지 확인한다.
func TestTimerErrors_ErrorsIs(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrIntervalTooShort", ErrIntervalTooShort},
		{"ErrInvalidCronExpression", ErrInvalidCronExpression},
		{"ErrTimerNotFound", ErrTimerNotFound},
		{"ErrTimerIDEmpty", ErrTimerIDEmpty},
		{"ErrTimerClosed", ErrTimerClosed},
		{"ErrTimerPaused", ErrTimerPaused},
		{"ErrNilHandler", ErrNilHandler},
		{"ErrDuplicateTimerID", ErrDuplicateTimerID},
		{"ErrMaxTimersReached", ErrMaxTimersReached},
		{"ErrInvalidDelay", ErrInvalidDelay},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			wrapped := fmt.Errorf("추가 컨텍스트: %w", tc.err)
			assert.True(t, errors.Is(wrapped, tc.err),
				"래핑된 에러에서 %s를 식별할 수 있어야 한다", tc.name)
		})
	}
}

// TestTimerErrors_MessagePrefix 는 모든 에러 메시지가 "timer:" 접두사를 갖는지 확인한다.
func TestTimerErrors_MessagePrefix(t *testing.T) {
	sentinels := []error{
		ErrIntervalTooShort,
		ErrInvalidCronExpression,
		ErrTimerNotFound,
		ErrTimerIDEmpty,
		ErrTimerClosed,
		ErrTimerPaused,
		ErrNilHandler,
		ErrDuplicateTimerID,
		ErrMaxTimersReached,
		ErrInvalidDelay,
	}

	for _, err := range sentinels {
		assert.Contains(t, err.Error(), "timer:",
			"에러 메시지는 'timer:' 접두사를 포함해야 한다: %s", err.Error())
	}
}
