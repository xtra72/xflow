package xferr

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/pkg/lifecycle"
	"github.com/xtra/xflow/pkg/message"
)

// === 모의 수신자 구현 ===

// mockErrorReceiver 는 ErrorReceiver 인터페이스의 테스트용 모의 구현체이다.
type mockErrorReceiver struct {
	mu       sync.Mutex
	received []ErrorMessage
	err      error
}

func newMockErrorReceiver() *mockErrorReceiver {
	return &mockErrorReceiver{}
}

func (r *mockErrorReceiver) ReceiveError(ctx context.Context, errMsg ErrorMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.received = append(r.received, errMsg)
	return r.err
}

// mockDeadLetterReceiver 는 DeadLetterReceiver 인터페이스의 테스트용 모의 구현체이다.
type mockDeadLetterReceiver struct {
	mu       sync.Mutex
	received []DeadLetterMessage
	err      error
}

func newMockDeadLetterReceiver() *mockDeadLetterReceiver {
	return &mockDeadLetterReceiver{}
}

func (r *mockDeadLetterReceiver) ReceiveDeadLetter(ctx context.Context, dlMsg DeadLetterMessage) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.received = append(r.received, dlMsg)
	return r.err
}

// mockStatusReceiver 는 StatusReceiver 인터페이스의 테스트용 모의 구현체이다.
type mockStatusReceiver struct {
	mu       sync.Mutex
	received []StatusEvent
	err      error
}

func newMockStatusReceiver() *mockStatusReceiver {
	return &mockStatusReceiver{}
}

func (r *mockStatusReceiver) ReceiveStatus(ctx context.Context, evt StatusEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.received = append(r.received, evt)
	return r.err
}

// === DefaultErrorRouter 테스트 ===

// TestDefaultErrorRouter_RouteError_ExactMatch 는 정확한 스코프 매칭으로 에러를 라우팅하는 것을 검증한다.
func TestDefaultErrorRouter_RouteError_ExactMatch(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver := newMockErrorReceiver()
	router.RegisterErrorReceiver("node-1", receiver)

	msg := message.New()
	em, err := NewErrorMessage(msg, errors.New("test"), "node-1")
	require.NoError(t, err)

	routeErr := router.RouteError(context.Background(), em)
	assert.NoError(t, routeErr)

	assert.Len(t, receiver.received, 1, "수신자가 에러 메시지를 받아야 한다")
}

// TestDefaultErrorRouter_RouteError_WildcardMatch 는 와일드카드 스코프 매칭을 검증한다.
func TestDefaultErrorRouter_RouteError_WildcardMatch(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver := newMockErrorReceiver()
	router.RegisterErrorReceiver("*", receiver)

	msg := message.New()
	em, err := NewErrorMessage(msg, errors.New("test"), "any-node")
	require.NoError(t, err)

	routeErr := router.RouteError(context.Background(), em)
	assert.NoError(t, routeErr)

	assert.Len(t, receiver.received, 1, "와일드카드 수신자가 에러 메시지를 받아야 한다")
}

// TestDefaultErrorRouter_RouteError_ExactAndWildcard 는 정확한 매칭과 와일드카드 모두에 전달되는 팬아웃을 검증한다.
func TestDefaultErrorRouter_RouteError_ExactAndWildcard(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	exactReceiver := newMockErrorReceiver()
	wildcardReceiver := newMockErrorReceiver()
	router.RegisterErrorReceiver("node-1", exactReceiver)
	router.RegisterErrorReceiver("*", wildcardReceiver)

	msg := message.New()
	em, err := NewErrorMessage(msg, errors.New("test"), "node-1")
	require.NoError(t, err)

	routeErr := router.RouteError(context.Background(), em)
	assert.NoError(t, routeErr)

	assert.Len(t, exactReceiver.received, 1, "정확한 매칭 수신자가 메시지를 받아야 한다")
	assert.Len(t, wildcardReceiver.received, 1, "와일드카드 수신자도 메시지를 받아야 한다")
}

// TestDefaultErrorRouter_RouteError_NoReceiver 는 수신자가 없을 때 폐기 정책이 적용되는지 검증한다.
func TestDefaultErrorRouter_RouteError_NoReceiver(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	msg := message.New()
	em, err := NewErrorMessage(msg, errors.New("test"), "node-1")
	require.NoError(t, err)

	routeErr := router.RouteError(context.Background(), em)
	assert.NoError(t, routeErr)

	assert.Equal(t, 1, metrics.counters["xferr.error.discarded"],
		"폐기 정책의 메트릭이 증가해야 한다")
}

// TestDefaultErrorRouter_RouteError_FanOut 은 동일 스코프에 여러 수신자가 등록된 팬아웃을 검증한다.
func TestDefaultErrorRouter_RouteError_FanOut(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver1 := newMockErrorReceiver()
	receiver2 := newMockErrorReceiver()
	router.RegisterErrorReceiver("node-1", receiver1)
	router.RegisterErrorReceiver("node-1", receiver2)

	msg := message.New()
	em, err := NewErrorMessage(msg, errors.New("test"), "node-1")
	require.NoError(t, err)

	routeErr := router.RouteError(context.Background(), em)
	assert.NoError(t, routeErr)

	assert.Len(t, receiver1.received, 1, "첫 번째 수신자가 메시지를 받아야 한다")
	assert.Len(t, receiver2.received, 1, "두 번째 수신자도 메시지를 받아야 한다")
}

// TestDefaultErrorRouter_RouteDeadLetter 는 데드레터 메시지 라우팅을 검증한다.
func TestDefaultErrorRouter_RouteDeadLetter(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver := newMockDeadLetterReceiver()
	router.RegisterDeadLetterReceiver(receiver)

	msg := message.New()
	dlm, err := NewDeadLetterMessage(msg, ReasonTTLExpired)
	require.NoError(t, err)

	routeErr := router.RouteDeadLetter(context.Background(), dlm)
	assert.NoError(t, routeErr)

	assert.Len(t, receiver.received, 1, "수신자가 데드레터 메시지를 받아야 한다")
}

// TestDefaultErrorRouter_RouteDeadLetter_NoReceiver 는 데드레터 수신자가 없을 때 폐기 정책을 검증한다.
func TestDefaultErrorRouter_RouteDeadLetter_NoReceiver(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	msg := message.New()
	dlm, err := NewDeadLetterMessage(msg, ReasonChannelFull)
	require.NoError(t, err)

	routeErr := router.RouteDeadLetter(context.Background(), dlm)
	assert.NoError(t, routeErr)

	assert.Equal(t, 1, metrics.counters["xferr.deadletter.discarded"],
		"폐기 정책의 데드레터 메트릭이 증가해야 한다")
}

// TestDefaultErrorRouter_RouteStatus 는 상태 이벤트 라우팅을 검증한다.
func TestDefaultErrorRouter_RouteStatus(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver := newMockStatusReceiver()
	router.RegisterStatusReceiver(receiver)

	evt, err := NewStatusEvent(ComponentNode, "node-1", lifecycle.StateCreated, lifecycle.StateRunning)
	require.NoError(t, err)

	routeErr := router.RouteStatus(context.Background(), evt)
	assert.NoError(t, routeErr)

	assert.Len(t, receiver.received, 1, "수신자가 상태 이벤트를 받아야 한다")
}

// TestDefaultErrorRouter_RouteStatus_NoReceiver 는 상태 수신자가 없을 때 폐기 정책을 검증한다.
func TestDefaultErrorRouter_RouteStatus_NoReceiver(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	evt, err := NewStatusEvent(ComponentAgent, "agent-1", lifecycle.StateRunning, lifecycle.StateStopped)
	require.NoError(t, err)

	routeErr := router.RouteStatus(context.Background(), evt)
	assert.NoError(t, routeErr)

	assert.Equal(t, 1, metrics.counters["xferr.status.discarded"],
		"폐기 정책의 상태 메트릭이 증가해야 한다")
}

// TestDefaultErrorRouter_Concurrent 은 동시 등록 및 라우팅의 안전성을 검증한다.
func TestDefaultErrorRouter_Concurrent(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	var routeCount atomic.Int64
	var wg sync.WaitGroup

	// 동시에 수신자 등록
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			receiver := newMockErrorReceiver()
			router.RegisterErrorReceiver("*", receiver)
		}(i)
	}

	// 동시에 에러 라우팅
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			msg := message.New()
			em, err := NewErrorMessage(msg, errors.New("concurrent test"), "node-1")
			if err != nil {
				return
			}
			if routeErr := router.RouteError(context.Background(), em); routeErr == nil {
				routeCount.Add(1)
			}
		}()
	}

	wg.Wait()
	assert.Greater(t, routeCount.Load(), int64(0), "동시 라우팅이 최소 1회 이상 성공해야 한다")
}

// TestDefaultErrorRouter_RouteError_ReceiverError 는 수신자가 에러를 반환할 때의 동작을 검증한다.
func TestDefaultErrorRouter_RouteError_ReceiverError(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver := newMockErrorReceiver()
	receiver.err = errors.New("receiver error")
	router.RegisterErrorReceiver("node-1", receiver)

	msg := message.New()
	em, err := NewErrorMessage(msg, errors.New("test"), "node-1")
	require.NoError(t, err)

	routeErr := router.RouteError(context.Background(), em)
	assert.Error(t, routeErr, "수신자의 에러가 반환되어야 한다")
}

// TestDefaultErrorRouter_RouteDeadLetter_MultipleReceivers 는 다수의 데드레터 수신자에게 팬아웃하는 것을 검증한다.
func TestDefaultErrorRouter_RouteDeadLetter_MultipleReceivers(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver1 := newMockDeadLetterReceiver()
	receiver2 := newMockDeadLetterReceiver()
	router.RegisterDeadLetterReceiver(receiver1)
	router.RegisterDeadLetterReceiver(receiver2)

	msg := message.New()
	dlm, err := NewDeadLetterMessage(msg, ReasonBackpressureDrop)
	require.NoError(t, err)

	routeErr := router.RouteDeadLetter(context.Background(), dlm)
	assert.NoError(t, routeErr)

	assert.Len(t, receiver1.received, 1, "첫 번째 수신자가 메시지를 받아야 한다")
	assert.Len(t, receiver2.received, 1, "두 번째 수신자도 메시지를 받아야 한다")
}

// TestDefaultErrorRouter_RouteStatus_MultipleReceivers 는 다수의 상태 수신자에게 팬아웃하는 것을 검증한다.
func TestDefaultErrorRouter_RouteStatus_MultipleReceivers(t *testing.T) {
	metrics := newMockMetrics()
	policy := NewSilentDiscardPolicy(metrics)
	router := NewDefaultErrorRouter(policy)

	receiver1 := newMockStatusReceiver()
	receiver2 := newMockStatusReceiver()
	router.RegisterStatusReceiver(receiver1)
	router.RegisterStatusReceiver(receiver2)

	evt, err := NewStatusEvent(ComponentFlow, "flow-1", lifecycle.StatePaused, lifecycle.StateRunning)
	require.NoError(t, err)

	routeErr := router.RouteStatus(context.Background(), evt)
	assert.NoError(t, routeErr)

	assert.Len(t, receiver1.received, 1, "첫 번째 수신자가 이벤트를 받아야 한다")
	assert.Len(t, receiver2.received, 1, "두 번째 수신자도 이벤트를 받아야 한다")
}
