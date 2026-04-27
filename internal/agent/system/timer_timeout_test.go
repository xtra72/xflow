package system

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Timeout 타이머 기본 동작 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_SetTimeout_DelayedExecution 은 지정된 지연 후 1회 실행되는지 검증한다.
func TestTimerAgent_SetTimeout_DelayedExecution(t *testing.T) {
	agent := newTestTimerAgent(t)

	var count int64
	done := make(chan struct{})

	handler := func(trigger TimerTrigger) {
		atomic.AddInt64(&count, 1)
		assert.Equal(t, int64(1), trigger.TickCount, "타임아웃 TickCount는 항상 1이어야 한다")
		close(done)
	}

	_, err := agent.SetTimeout("once", 200*time.Millisecond, handler)
	require.NoError(t, err)

	select {
	case <-done:
		assert.Equal(t, int64(1), atomic.LoadInt64(&count))
	case <-time.After(2 * time.Second):
		t.Fatal("타임아웃 타이머가 실행되지 않았다")
	}
}

// TestTimerAgent_SetTimeout_AutoRemoval 은 실행 후 자동으로 List에서 제거되는지 검증한다.
func TestTimerAgent_SetTimeout_AutoRemoval(t *testing.T) {
	agent := newTestTimerAgent(t)

	done := make(chan struct{})

	handler := func(_ TimerTrigger) {
		close(done)
	}

	_, err := agent.SetTimeout("auto-remove", 100*time.Millisecond, handler)
	require.NoError(t, err)

	// 등록 직후에는 List에 포함
	require.Len(t, agent.List(), 1)

	// 실행 완료 대기
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("타임아웃 타이머 실행 대기 초과")
	}

	// 안정화 대기
	time.Sleep(50 * time.Millisecond)

	// 실행 후 List에서 제거됨
	assert.Empty(t, agent.List())
}

// TestTimerAgent_SetTimeout_Cancel 은 Cancel이 실행을 방지하는지 검증한다.
func TestTimerAgent_SetTimeout_Cancel(t *testing.T) {
	agent := newTestTimerAgent(t)

	var count int64

	handler := func(_ TimerTrigger) {
		atomic.AddInt64(&count, 1)
	}

	tid, err := agent.SetTimeout("cancel-timeout", 500*time.Millisecond, handler)
	require.NoError(t, err)

	// 즉시 취소
	err = agent.Cancel(tid)
	require.NoError(t, err)

	// 원래 실행 예정 시간 + 여유 대기
	time.Sleep(700 * time.Millisecond)

	assert.Equal(t, int64(0), atomic.LoadInt64(&count),
		"Cancel된 타이머는 실행되면 안 된다")
}

// TestTimerAgent_SetTimeout_CancelAfterFired 는 실행 후 Cancel 시 ErrTimerNotFound를 반환하는지 검증한다.
func TestTimerAgent_SetTimeout_CancelAfterFired(t *testing.T) {
	agent := newTestTimerAgent(t)

	done := make(chan struct{})

	handler := func(_ TimerTrigger) {
		close(done)
	}

	tid, err := agent.SetTimeout("fire-then-cancel", 100*time.Millisecond, handler)
	require.NoError(t, err)

	// 실행 완료 대기
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("타이머 실행 대기 초과")
	}

	// 안정화 대기
	time.Sleep(50 * time.Millisecond)

	// 이미 실행 후 제거되었으므로 NotFound
	err = agent.Cancel(tid)
	assert.ErrorIs(t, err, ErrTimerNotFound)
}

// ---------------------------------------------------------------------------
// 유효성 검사 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_SetTimeout_InvalidDelay_Negative 는 음수 지연이 거부되는지 검증한다.
func TestTimerAgent_SetTimeout_InvalidDelay_Negative(t *testing.T) {
	agent := newTestTimerAgent(t)

	_, err := agent.SetTimeout("negative", -100*time.Millisecond, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrInvalidDelay)
}

// TestTimerAgent_SetTimeout_InvalidDelay_Zero 는 0 지연이 거부되는지 검증한다.
func TestTimerAgent_SetTimeout_InvalidDelay_Zero(t *testing.T) {
	agent := newTestTimerAgent(t)

	_, err := agent.SetTimeout("zero", 0, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrInvalidDelay)
}

// TestTimerAgent_SetTimeout_EmptyID 는 빈 ID가 거부되는지 검증한다.
func TestTimerAgent_SetTimeout_EmptyID(t *testing.T) {
	agent := newTestTimerAgent(t)
	_, err := agent.SetTimeout("", 200*time.Millisecond, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerIDEmpty)
}

// TestTimerAgent_SetTimeout_NilHandler 는 nil 핸들러가 거부되는지 검증한다.
func TestTimerAgent_SetTimeout_NilHandler(t *testing.T) {
	agent := newTestTimerAgent(t)
	_, err := agent.SetTimeout("nil-timeout", 200*time.Millisecond, nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

// TestTimerAgent_SetTimeout_DuplicateID 는 중복 ID가 거부되는지 검증한다.
func TestTimerAgent_SetTimeout_DuplicateID(t *testing.T) {
	agent := newTestTimerAgent(t)
	handler := func(_ TimerTrigger) {}

	_, err := agent.SetTimeout("dup-timeout", 5*time.Second, handler)
	require.NoError(t, err)

	_, err = agent.SetTimeout("dup-timeout", 5*time.Second, handler)
	assert.ErrorIs(t, err, ErrDuplicateTimerID)
}

// TestTimerAgent_SetTimeout_Closed 는 닫힌 에이전트에서 거부되는지 검증한다.
func TestTimerAgent_SetTimeout_Closed(t *testing.T) {
	agent := NewTimerAgent()
	require.NoError(t, agent.Init(context.Background()))
	require.NoError(t, agent.Stop(context.Background()))

	_, err := agent.SetTimeout("after-close", 200*time.Millisecond, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerClosed)
}

// TestTimerAgent_SetTimeout_Paused 는 일시정지 상태에서 거부되는지 검증한다.
func TestTimerAgent_SetTimeout_Paused(t *testing.T) {
	agent := newTestTimerAgent(t)
	require.NoError(t, agent.Pause(context.Background()))

	_, err := agent.SetTimeout("paused-timeout", 200*time.Millisecond, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerPaused)
}
