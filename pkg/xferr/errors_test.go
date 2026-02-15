package xferr

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSentinelErrors_NonNil 은 모든 센티널 에러가 nil이 아닌지 검증한다.
func TestSentinelErrors_NonNil(t *testing.T) {
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrNilMessage", ErrNilMessage},
		{"ErrNilError", ErrNilError},
		{"ErrNoReceiver", ErrNoReceiver},
		{"ErrAlertThresholdExceeded", ErrAlertThresholdExceeded},
		{"ErrInvalidSeverity", ErrInvalidSeverity},
		{"ErrInvalidCategory", ErrInvalidCategory},
		{"ErrInvalidDropReason", ErrInvalidDropReason},
		{"ErrInvalidComponentType", ErrInvalidComponentType},
		{"ErrSameStateTransition", ErrSameStateTransition},
	}

	for _, s := range sentinels {
		t.Run(s.name, func(t *testing.T) {
			assert.NotNil(t, s.err, "%s는 nil이 아니어야 한다", s.name)
			assert.NotEmpty(t, s.err.Error(), "%s의 에러 메시지가 비어있으면 안 된다", s.name)
		})
	}
}

// TestSentinelErrors_Distinct 는 모든 센티널 에러가 서로 다른지 검증한다.
func TestSentinelErrors_Distinct(t *testing.T) {
	sentinels := []error{
		ErrNilMessage,
		ErrNilError,
		ErrNoReceiver,
		ErrAlertThresholdExceeded,
		ErrInvalidSeverity,
		ErrInvalidCategory,
		ErrInvalidDropReason,
		ErrInvalidComponentType,
		ErrSameStateTransition,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			assert.NotEqual(t, sentinels[i].Error(), sentinels[j].Error(),
				"에러 [%d]와 [%d]는 서로 달라야 한다", i, j)
		}
	}
}

// TestSentinelErrors_ErrorsIs 는 errors.Is()와의 호환성을 검증한다.
func TestSentinelErrors_ErrorsIs(t *testing.T) {
	sentinels := []error{
		ErrNilMessage,
		ErrNilError,
		ErrNoReceiver,
		ErrAlertThresholdExceeded,
		ErrInvalidSeverity,
		ErrInvalidCategory,
		ErrInvalidDropReason,
		ErrInvalidComponentType,
		ErrSameStateTransition,
	}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			// 자기 자신과 일치해야 한다
			assert.True(t, errors.Is(sentinel, sentinel),
				"errors.Is(%v, %v)는 true여야 한다", sentinel, sentinel)

			// 다른 에러와 일치하면 안 된다
			otherErr := errors.New("other error")
			assert.False(t, errors.Is(sentinel, otherErr),
				"errors.Is(%v, otherErr)는 false여야 한다", sentinel)
		})
	}
}

// TestSentinelErrors_ErrorMessages 는 모든 에러 메시지가 "xferr:" 접두사를 가지는지 검증한다.
func TestSentinelErrors_ErrorMessages(t *testing.T) {
	sentinels := []error{
		ErrNilMessage,
		ErrNilError,
		ErrNoReceiver,
		ErrAlertThresholdExceeded,
		ErrInvalidSeverity,
		ErrInvalidCategory,
		ErrInvalidDropReason,
		ErrInvalidComponentType,
		ErrSameStateTransition,
	}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			assert.Contains(t, sentinel.Error(), "xferr:",
				"에러 메시지는 'xferr:' 접두사를 포함해야 한다")
		})
	}
}
