package xferr

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// mockLogger 는 Logger 인터페이스의 테스트용 모의 구현체이다.
type mockLogger struct {
	mu       sync.Mutex
	errors   []string
	warnings []string
	infos    []string
}

func newMockLogger() *mockLogger {
	return &mockLogger{}
}

func (l *mockLogger) Error(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errors = append(l.errors, msg)
}

func (l *mockLogger) Warn(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.warnings = append(l.warnings, msg)
}

func (l *mockLogger) Info(msg string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.infos = append(l.infos, msg)
}

// mockMetrics 는 Metrics 인터페이스의 테스트용 모의 구현체이다.
type mockMetrics struct {
	mu       sync.Mutex
	counters map[string]int
}

func newMockMetrics() *mockMetrics {
	return &mockMetrics{
		counters: make(map[string]int),
	}
}

func (m *mockMetrics) IncrCounter(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.counters[name]++
}

// 테스트 헬퍼: ErrorMessage 생성
func newTestErrorMessage(t *testing.T, severity ErrorSeverity) ErrorMessage {
	t.Helper()
	msg := message.New()
	em, err := NewErrorMessage(msg, errors.New("test error"), "node-1",
		WithSeverity(severity),
	)
	require.NoError(t, err)
	return em
}

// 테스트 헬퍼: DeadLetterMessage 생성
func newTestDeadLetterMessage(t *testing.T) DeadLetterMessage {
	t.Helper()
	msg := message.New()
	dlm, err := NewDeadLetterMessage(msg, ReasonTTLExpired)
	require.NoError(t, err)
	return dlm
}

// 테스트 헬퍼: StatusEvent 생성
func newTestStatusEvent(t *testing.T) StatusEvent {
	t.Helper()
	evt, err := NewStatusEvent(ComponentNode, "node-1", lifecycle.StateCreated, lifecycle.StateRunning)
	require.NoError(t, err)
	return evt
}

// === LogAndDiscardPolicy 테스트 ===

// TestLogAndDiscardPolicy_HandleError 는 에러 처리 시 로깅과 메트릭 증가를 검증한다.
func TestLogAndDiscardPolicy_HandleError(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewLogAndDiscardPolicy(logger, metrics)

	em := newTestErrorMessage(t, SeverityError)
	policy.HandleError(context.Background(), em)

	assert.Len(t, logger.errors, 1, "에러 로그가 1건 기록되어야 한다")
	assert.Equal(t, 1, metrics.counters["xferr.error.discarded"], "에러 메트릭이 증가해야 한다")
}

// TestLogAndDiscardPolicy_HandleDeadLetter 는 데드레터 처리 시 로깅과 메트릭 증가를 검증한다.
func TestLogAndDiscardPolicy_HandleDeadLetter(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewLogAndDiscardPolicy(logger, metrics)

	dlm := newTestDeadLetterMessage(t)
	policy.HandleDeadLetter(context.Background(), dlm)

	assert.Len(t, logger.warnings, 1, "경고 로그가 1건 기록되어야 한다")
	assert.Equal(t, 1, metrics.counters["xferr.deadletter.discarded"], "데드레터 메트릭이 증가해야 한다")
}

// TestLogAndDiscardPolicy_HandleStatus 는 상태 이벤트 처리 시 로깅과 메트릭 증가를 검증한다.
func TestLogAndDiscardPolicy_HandleStatus(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewLogAndDiscardPolicy(logger, metrics)

	evt := newTestStatusEvent(t)
	policy.HandleStatus(context.Background(), evt)

	assert.Len(t, logger.infos, 1, "정보 로그가 1건 기록되어야 한다")
	assert.Equal(t, 1, metrics.counters["xferr.status.discarded"], "상태 메트릭이 증가해야 한다")
}

// === SilentDiscardPolicy 테스트 ===

// TestSilentDiscardPolicy_HandleError 는 에러 처리 시 메트릭만 증가하고 로깅하지 않는 것을 검증한다.
func TestSilentDiscardPolicy_HandleError(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)

	em := newTestErrorMessage(t, SeverityError)
	policy.HandleError(context.Background(), em)

	assert.Equal(t, 1, metrics.counters["xferr.error.discarded"], "에러 메트릭이 증가해야 한다")
}

// TestSilentDiscardPolicy_HandleDeadLetter 는 데드레터 처리 시 메트릭만 증가하고 로깅하지 않는 것을 검증한다.
func TestSilentDiscardPolicy_HandleDeadLetter(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)

	dlm := newTestDeadLetterMessage(t)
	policy.HandleDeadLetter(context.Background(), dlm)

	assert.Equal(t, 1, metrics.counters["xferr.deadletter.discarded"], "데드레터 메트릭이 증가해야 한다")
}

// TestSilentDiscardPolicy_HandleStatus 는 상태 이벤트 처리 시 메트릭만 증가하고 로깅하지 않는 것을 검증한다.
func TestSilentDiscardPolicy_HandleStatus(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)

	evt := newTestStatusEvent(t)
	policy.HandleStatus(context.Background(), evt)

	assert.Equal(t, 1, metrics.counters["xferr.status.discarded"], "상태 메트릭이 증가해야 한다")
}

// === PanicOnCriticalPolicy 테스트 ===

// TestPanicOnCriticalPolicy_HandleError_Critical 은 치명적 에러 시 패닉을 발생시키는지 검증한다.
func TestPanicOnCriticalPolicy_HandleError_Critical(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewPanicOnCriticalPolicy(logger, metrics)

	em := newTestErrorMessage(t, SeverityCritical)

	assert.Panics(t, func() {
		policy.HandleError(context.Background(), em)
	}, "치명적 에러 시 패닉이 발생해야 한다")
}

// TestPanicOnCriticalPolicy_HandleError_NonCritical 은 비치명적 에러 시 패닉 없이 처리되는지 검증한다.
func TestPanicOnCriticalPolicy_HandleError_NonCritical(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewPanicOnCriticalPolicy(logger, metrics)

	em := newTestErrorMessage(t, SeverityError)

	assert.NotPanics(t, func() {
		policy.HandleError(context.Background(), em)
	}, "비치명적 에러 시 패닉이 발생하면 안 된다")

	assert.Len(t, logger.errors, 1, "에러 로그가 기록되어야 한다")
	assert.Equal(t, 1, metrics.counters["xferr.error.discarded"], "에러 메트릭이 증가해야 한다")
}

// TestPanicOnCriticalPolicy_HandleDeadLetter 는 데드레터 처리가 LogAndDiscard와 동일한지 검증한다.
func TestPanicOnCriticalPolicy_HandleDeadLetter(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewPanicOnCriticalPolicy(logger, metrics)

	dlm := newTestDeadLetterMessage(t)
	policy.HandleDeadLetter(context.Background(), dlm)

	assert.Len(t, logger.warnings, 1, "경고 로그가 기록되어야 한다")
	assert.Equal(t, 1, metrics.counters["xferr.deadletter.discarded"], "데드레터 메트릭이 증가해야 한다")
}

// TestPanicOnCriticalPolicy_HandleStatus 는 상태 이벤트 처리가 LogAndDiscard와 동일한지 검증한다.
func TestPanicOnCriticalPolicy_HandleStatus(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewPanicOnCriticalPolicy(logger, metrics)

	evt := newTestStatusEvent(t)
	policy.HandleStatus(context.Background(), evt)

	assert.Len(t, logger.infos, 1, "정보 로그가 기록되어야 한다")
	assert.Equal(t, 1, metrics.counters["xferr.status.discarded"], "상태 메트릭이 증가해야 한다")
}

// TestPanicOnCriticalPolicy_HandleError_Warning 은 경고 에러 시 패닉 없이 처리되는지 검증한다.
func TestPanicOnCriticalPolicy_HandleError_Warning(t *testing.T) {
	logger := newMockLogger()
	metrics := newMockMetrics()
	policy := NewPanicOnCriticalPolicy(logger, metrics)

	em := newTestErrorMessage(t, SeverityWarning)

	assert.NotPanics(t, func() {
		policy.HandleError(context.Background(), em)
	}, "경고 에러 시 패닉이 발생하면 안 된다")
}
