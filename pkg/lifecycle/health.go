package lifecycle

import (
	"context"
	"time"
)

// HealthChecker 는 컴포넌트의 건강 상태를 확인하는 인터페이스이다.
type HealthChecker interface {
	HealthCheck(ctx context.Context) HealthStatus
}

// HealthStatus 는 헬스 체크 결과를 나타내는 구조체이다.
type HealthStatus struct {
	Healthy     bool           // 건강 여부
	Message     string         // 상태 메시지
	LastChecked time.Time      // 마지막 체크 시각
	Details     map[string]any // 추가 상세 정보
}

// RecoveryAction 은 최대 재시도 초과 시 수행할 동작을 정의하는 타입이다.
type RecoveryAction string

const (
	// RecoveryStop 은 최대 재시도 초과 시 컴포넌트를 중지한다.
	RecoveryStop RecoveryAction = "stop"
	// RecoveryKeepError 는 최대 재시도 초과 시 Error 상태를 유지한다.
	RecoveryKeepError RecoveryAction = "keep_error"
)

// RecoveryPolicy 는 자동 복구 전략을 정의하는 구조체이다.
type RecoveryPolicy struct {
	MaxRetries           int            // 최대 재시도 횟수 (0이면 비활성화)
	InitialBackoff       time.Duration  // 초기 백오프 간격
	MaxBackoff           time.Duration  // 최대 백오프 간격
	BackoffMultiplier    float64        // 백오프 증가 배수
	OnMaxRetriesExceeded RecoveryAction // 최대 재시도 초과 시 동작
}

// DefaultRecoveryPolicy 는 합리적인 기본값을 가진 RecoveryPolicy를 반환한다.
func DefaultRecoveryPolicy() RecoveryPolicy {
	return RecoveryPolicy{
		MaxRetries:           3,
		InitialBackoff:       1 * time.Second,
		MaxBackoff:           30 * time.Second,
		BackoffMultiplier:    2.0,
		OnMaxRetriesExceeded: RecoveryStop,
	}
}
