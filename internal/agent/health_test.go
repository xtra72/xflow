package agent

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestHealthState_StringValues(t *testing.T) {
	// HealthState 문자열 값이 올바른지 확인한다.
	tests := []struct {
		state    HealthState
		expected string
	}{
		{HealthHealthy, "healthy"},
		{HealthDegraded, "degraded"},
		{HealthUnhealthy, "unhealthy"},
	}

	for _, tc := range tests {
		t.Run(tc.expected, func(t *testing.T) {
			assert.Equal(t, tc.expected, string(tc.state))
		})
	}
}

func TestHealthStatus_ZeroValue(t *testing.T) {
	// HealthStatus의 zero value가 올바른 기본값인지 확인한다.
	var hs HealthStatus

	assert.Equal(t, HealthState(""), hs.Status, "Status zero value는 빈 문자열이어야 한다")
	assert.True(t, hs.LastCheck.IsZero(), "LastCheck zero value는 zero time이어야 한다")
	assert.True(t, hs.LastSuccess.IsZero(), "LastSuccess zero value는 zero time이어야 한다")
	assert.Equal(t, 0, hs.ConsecutiveFailures, "ConsecutiveFailures zero value는 0이어야 한다")
	assert.Empty(t, hs.Message, "Message zero value는 빈 문자열이어야 한다")
}

func TestHealthStatus_WithValues(t *testing.T) {
	// HealthStatus에 값을 설정하고 올바르게 반환되는지 확인한다.
	now := time.Now()
	hs := HealthStatus{
		Status:              HealthHealthy,
		LastCheck:           now,
		LastSuccess:         now,
		ConsecutiveFailures: 0,
		Message:             "all systems operational",
	}

	assert.Equal(t, HealthHealthy, hs.Status)
	assert.Equal(t, now, hs.LastCheck)
	assert.Equal(t, now, hs.LastSuccess)
	assert.Equal(t, 0, hs.ConsecutiveFailures)
	assert.Equal(t, "all systems operational", hs.Message)
}

func TestHealthStatus_Unhealthy(t *testing.T) {
	// Unhealthy 상태의 HealthStatus를 생성하고 확인한다.
	lastSuccess := time.Now().Add(-5 * time.Minute)
	hs := HealthStatus{
		Status:              HealthUnhealthy,
		LastCheck:           time.Now(),
		LastSuccess:         lastSuccess,
		ConsecutiveFailures: 5,
		Message:             "transport connection lost",
	}

	assert.Equal(t, HealthUnhealthy, hs.Status)
	assert.Equal(t, 5, hs.ConsecutiveFailures)
	assert.Equal(t, "transport connection lost", hs.Message)
}
