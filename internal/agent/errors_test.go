package agent

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSentinelErrors_Defined(t *testing.T) {
	// 14개의 sentinel 에러가 모두 정의되어 있는지 확인한다.
	sentinels := []struct {
		name string
		err  error
	}{
		{"ErrAgentNotFound", ErrAgentNotFound},
		{"ErrAgentAlreadyExists", ErrAgentAlreadyExists},
		{"ErrAgentNotRunning", ErrAgentNotRunning},
		{"ErrAgentAlreadyStopped", ErrAgentAlreadyStopped},
		{"ErrTransportNotAvailable", ErrTransportNotAvailable},
		{"ErrTransportClosed", ErrTransportClosed},
		{"ErrProtocolParseError", ErrProtocolParseError},
		{"ErrChecksumMismatch", ErrChecksumMismatch},
		{"ErrHealthCheckFailed", ErrHealthCheckFailed},
		{"ErrMaxRestartsExceeded", ErrMaxRestartsExceeded},
		{"ErrInvalidConfig", ErrInvalidConfig},
		{"ErrConfigImmutable", ErrConfigImmutable},
		{"ErrInvalidStateTransition", ErrInvalidStateTransition},
		{"ErrFlowAlreadyReferenced", ErrFlowAlreadyReferenced},
	}

	for _, tc := range sentinels {
		t.Run(tc.name, func(t *testing.T) {
			require.NotNil(t, tc.err, "%s는 nil이 아니어야 한다", tc.name)
			assert.NotEmpty(t, tc.err.Error(), "%s의 에러 메시지가 비어있으면 안 된다", tc.name)
		})
	}
}

func TestSentinelErrors_ErrorsIs(t *testing.T) {
	// 모든 sentinel 에러가 errors.Is()로 비교 가능한지 확인한다.
	sentinels := []error{
		ErrAgentNotFound,
		ErrAgentAlreadyExists,
		ErrAgentNotRunning,
		ErrAgentAlreadyStopped,
		ErrTransportNotAvailable,
		ErrTransportClosed,
		ErrProtocolParseError,
		ErrChecksumMismatch,
		ErrHealthCheckFailed,
		ErrMaxRestartsExceeded,
		ErrInvalidConfig,
		ErrConfigImmutable,
		ErrInvalidStateTransition,
		ErrFlowAlreadyReferenced,
	}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			// 직접 비교
			assert.True(t, errors.Is(sentinel, sentinel),
				"errors.Is(%v, %v)는 true여야 한다", sentinel, sentinel)
		})
	}
}

func TestSentinelErrors_WrappedErrorsIs(t *testing.T) {
	// fmt.Errorf로 래핑된 에러도 errors.Is()로 원본을 찾을 수 있어야 한다.
	sentinels := []error{
		ErrAgentNotFound,
		ErrAgentAlreadyExists,
		ErrAgentNotRunning,
		ErrAgentAlreadyStopped,
		ErrTransportNotAvailable,
		ErrTransportClosed,
		ErrProtocolParseError,
		ErrChecksumMismatch,
		ErrHealthCheckFailed,
		ErrMaxRestartsExceeded,
		ErrInvalidConfig,
		ErrConfigImmutable,
		ErrInvalidStateTransition,
		ErrFlowAlreadyReferenced,
	}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			wrapped := fmt.Errorf("추가 컨텍스트: %w", sentinel)
			assert.True(t, errors.Is(wrapped, sentinel),
				"래핑된 에러에서 errors.Is()로 원본 sentinel을 찾을 수 있어야 한다")
		})
	}
}

func TestSentinelErrors_Uniqueness(t *testing.T) {
	// 모든 sentinel 에러는 서로 다른 고유한 에러여야 한다.
	sentinels := []error{
		ErrAgentNotFound,
		ErrAgentAlreadyExists,
		ErrAgentNotRunning,
		ErrAgentAlreadyStopped,
		ErrTransportNotAvailable,
		ErrTransportClosed,
		ErrProtocolParseError,
		ErrChecksumMismatch,
		ErrHealthCheckFailed,
		ErrMaxRestartsExceeded,
		ErrInvalidConfig,
		ErrConfigImmutable,
		ErrInvalidStateTransition,
		ErrFlowAlreadyReferenced,
	}

	for i := 0; i < len(sentinels); i++ {
		for j := i + 1; j < len(sentinels); j++ {
			assert.False(t, errors.Is(sentinels[i], sentinels[j]),
				"%v와 %v는 서로 다른 에러여야 한다", sentinels[i], sentinels[j])
		}
	}
}

func TestSentinelErrors_MessagePrefix(t *testing.T) {
	// 모든 sentinel 에러 메시지가 "agent:" 접두사를 갖는지 확인한다.
	sentinels := []error{
		ErrAgentNotFound,
		ErrAgentAlreadyExists,
		ErrAgentNotRunning,
		ErrAgentAlreadyStopped,
		ErrTransportNotAvailable,
		ErrTransportClosed,
		ErrProtocolParseError,
		ErrChecksumMismatch,
		ErrHealthCheckFailed,
		ErrMaxRestartsExceeded,
		ErrInvalidConfig,
		ErrConfigImmutable,
		ErrInvalidStateTransition,
		ErrFlowAlreadyReferenced,
	}

	for _, sentinel := range sentinels {
		t.Run(sentinel.Error(), func(t *testing.T) {
			assert.Contains(t, sentinel.Error(), "agent:",
				"에러 메시지에 'agent:' 접두사가 포함되어야 한다")
		})
	}
}
