package lifecycle

import (
	"context"
	"testing"
	"time"
)

// --- HealthChecker 인터페이스 테스트 ---

// mockHealthChecker 는 HealthChecker 인터페이스를 구현하는 테스트용 구조체이다.
type mockHealthChecker struct {
	healthy bool
	message string
}

// HealthCheck 는 모의 건강 상태를 반환한다.
func (m *mockHealthChecker) HealthCheck(ctx context.Context) HealthStatus {
	return HealthStatus{
		Healthy:     m.healthy,
		Message:     m.message,
		LastChecked: time.Now(),
		Details:     map[string]any{"mock": true},
	}
}

// 컴파일 타임에 HealthChecker 인터페이스 구현을 확인한다 (AC-LIFE-001-26).
var _ HealthChecker = (*mockHealthChecker)(nil)

// TestHealthChecker_InterfaceImplementation 은 HealthChecker 인터페이스가 존재하고 구현 가능한지 검증한다 (AC-LIFE-001-26).
func TestHealthChecker_InterfaceImplementation(t *testing.T) {
	checker := &mockHealthChecker{healthy: true, message: "정상"}
	ctx := context.Background()

	status := checker.HealthCheck(ctx)
	if !status.Healthy {
		t.Errorf("Healthy = %v, 기대값 true", status.Healthy)
	}
	if status.Message != "정상" {
		t.Errorf("Message = %q, 기대값 %q", status.Message, "정상")
	}
}

// TestHealthChecker_UnhealthyStatus 는 비정상 상태를 반환할 수 있는지 검증한다 (AC-LIFE-001-26).
func TestHealthChecker_UnhealthyStatus(t *testing.T) {
	checker := &mockHealthChecker{healthy: false, message: "비정상"}
	ctx := context.Background()

	status := checker.HealthCheck(ctx)
	if status.Healthy {
		t.Errorf("Healthy = %v, 기대값 false", status.Healthy)
	}
	if status.Message != "비정상" {
		t.Errorf("Message = %q, 기대값 %q", status.Message, "비정상")
	}
}

// --- HealthStatus 구조체 테스트 ---

// TestHealthStatus_Fields 는 HealthStatus 구조체의 필드가 올바른 타입으로 존재하는지 검증한다 (AC-LIFE-001-27).
func TestHealthStatus_Fields(t *testing.T) {
	now := time.Now()
	details := map[string]any{"cpu": 80.5, "memory": "1GB"}

	status := HealthStatus{
		Healthy:     true,
		Message:     "모든 컴포넌트 정상",
		LastChecked: now,
		Details:     details,
	}

	// Healthy 필드 확인 (bool 타입)
	if status.Healthy != true {
		t.Errorf("Healthy = %v, 기대값 true", status.Healthy)
	}

	// Message 필드 확인 (string 타입)
	if status.Message != "모든 컴포넌트 정상" {
		t.Errorf("Message = %q, 기대값 %q", status.Message, "모든 컴포넌트 정상")
	}

	// LastChecked 필드 확인 (time.Time 타입)
	if !status.LastChecked.Equal(now) {
		t.Errorf("LastChecked = %v, 기대값 %v", status.LastChecked, now)
	}

	// Details 필드 확인 (map[string]any 타입)
	if status.Details["cpu"] != 80.5 {
		t.Errorf("Details[cpu] = %v, 기대값 80.5", status.Details["cpu"])
	}
	if status.Details["memory"] != "1GB" {
		t.Errorf("Details[memory] = %v, 기대값 %q", status.Details["memory"], "1GB")
	}
}

// TestHealthStatus_ZeroValue 는 HealthStatus의 제로값이 올바른지 검증한다 (AC-LIFE-001-27).
func TestHealthStatus_ZeroValue(t *testing.T) {
	var status HealthStatus

	if status.Healthy != false {
		t.Errorf("제로값 Healthy = %v, 기대값 false", status.Healthy)
	}
	if status.Message != "" {
		t.Errorf("제로값 Message = %q, 기대값 빈 문자열", status.Message)
	}
	if !status.LastChecked.IsZero() {
		t.Errorf("제로값 LastChecked = %v, 기대값 제로 시각", status.LastChecked)
	}
	if status.Details != nil {
		t.Errorf("제로값 Details = %v, 기대값 nil", status.Details)
	}
}

// --- RecoveryPolicy 구조체 테스트 ---

// TestRecoveryPolicy_Fields 는 RecoveryPolicy 구조체의 필드가 올바른 타입으로 존재하는지 검증한다 (AC-LIFE-001-28).
func TestRecoveryPolicy_Fields(t *testing.T) {
	policy := RecoveryPolicy{
		MaxRetries:           5,
		InitialBackoff:       2 * time.Second,
		MaxBackoff:           60 * time.Second,
		BackoffMultiplier:    3.0,
		OnMaxRetriesExceeded: RecoveryKeepError,
	}

	// MaxRetries 필드 확인 (int 타입)
	if policy.MaxRetries != 5 {
		t.Errorf("MaxRetries = %d, 기대값 5", policy.MaxRetries)
	}

	// InitialBackoff 필드 확인 (time.Duration 타입)
	if policy.InitialBackoff != 2*time.Second {
		t.Errorf("InitialBackoff = %v, 기대값 %v", policy.InitialBackoff, 2*time.Second)
	}

	// MaxBackoff 필드 확인 (time.Duration 타입)
	if policy.MaxBackoff != 60*time.Second {
		t.Errorf("MaxBackoff = %v, 기대값 %v", policy.MaxBackoff, 60*time.Second)
	}

	// BackoffMultiplier 필드 확인 (float64 타입)
	if policy.BackoffMultiplier != 3.0 {
		t.Errorf("BackoffMultiplier = %v, 기대값 3.0", policy.BackoffMultiplier)
	}

	// OnMaxRetriesExceeded 필드 확인 (RecoveryAction 타입)
	if policy.OnMaxRetriesExceeded != RecoveryKeepError {
		t.Errorf("OnMaxRetriesExceeded = %q, 기대값 %q", policy.OnMaxRetriesExceeded, RecoveryKeepError)
	}
}

// --- DefaultRecoveryPolicy 함수 테스트 ---

// TestDefaultRecoveryPolicy_Values 는 DefaultRecoveryPolicy가 올바른 기본값을 반환하는지 검증한다 (AC-LIFE-001-29).
func TestDefaultRecoveryPolicy_Values(t *testing.T) {
	policy := DefaultRecoveryPolicy()

	// MaxRetries: 3
	if policy.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, 기대값 3", policy.MaxRetries)
	}

	// InitialBackoff: 1초
	if policy.InitialBackoff != 1*time.Second {
		t.Errorf("InitialBackoff = %v, 기대값 %v", policy.InitialBackoff, 1*time.Second)
	}

	// MaxBackoff: 30초
	if policy.MaxBackoff != 30*time.Second {
		t.Errorf("MaxBackoff = %v, 기대값 %v", policy.MaxBackoff, 30*time.Second)
	}

	// BackoffMultiplier: 2.0
	if policy.BackoffMultiplier != 2.0 {
		t.Errorf("BackoffMultiplier = %v, 기대값 2.0", policy.BackoffMultiplier)
	}

	// OnMaxRetriesExceeded: RecoveryStop
	if policy.OnMaxRetriesExceeded != RecoveryStop {
		t.Errorf("OnMaxRetriesExceeded = %q, 기대값 %q", policy.OnMaxRetriesExceeded, RecoveryStop)
	}
}

// --- RecoveryAction 상수 테스트 ---

// TestRecoveryAction_Constants 는 RecoveryAction 상수들이 올바른 값을 가지는지 검증한다 (AC-LIFE-001-30).
func TestRecoveryAction_Constants(t *testing.T) {
	// RecoveryStop 상수 확인
	if RecoveryStop != "stop" {
		t.Errorf("RecoveryStop = %q, 기대값 %q", RecoveryStop, "stop")
	}

	// RecoveryKeepError 상수 확인
	if RecoveryKeepError != "keep_error" {
		t.Errorf("RecoveryKeepError = %q, 기대값 %q", RecoveryKeepError, "keep_error")
	}
}

// TestRecoveryAction_Type 은 RecoveryAction이 문자열 기반 타입인지 검증한다 (AC-LIFE-001-30).
func TestRecoveryAction_Type(t *testing.T) {
	// RecoveryAction은 string 기반 타입이므로 string으로 변환 가능해야 한다
	var action RecoveryAction = "custom_action"
	if string(action) != "custom_action" {
		t.Errorf("RecoveryAction 문자열 변환 = %q, 기대값 %q", string(action), "custom_action")
	}
}
