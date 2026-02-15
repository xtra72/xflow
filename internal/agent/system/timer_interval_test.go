package system

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestTimerAgent 는 테스트용 초기화된 TimerAgent를 생성한다.
func newTestTimerAgent(t *testing.T, opts ...TimerOption) *TimerAgent {
	t.Helper()
	agent := NewTimerAgent(opts...)
	err := agent.Init(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = agent.Stop(context.Background())
	})
	return agent
}

// ---------------------------------------------------------------------------
// 인터벌 타이머 기본 동작 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_SetInterval_PeriodicExecution 은 인터벌 타이머가 주기적으로 실행되는지 검증한다.
func TestTimerAgent_SetInterval_PeriodicExecution(t *testing.T) {
	agent := newTestTimerAgent(t)

	var count int64
	done := make(chan struct{})

	handler := func(_ TimerTrigger) {
		if atomic.AddInt64(&count, 1) >= 5 {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}

	_, err := agent.SetInterval("tick", 100*time.Millisecond, handler)
	require.NoError(t, err)

	// 최대 2초 대기 (100ms * 5 = 500ms 이내에 완료 예상)
	select {
	case <-done:
		// 성공: 5회 이상 실행됨
	case <-time.After(2 * time.Second):
		t.Fatal("인터벌 타이머가 5회 실행되지 않았다")
	}

	finalCount := atomic.LoadInt64(&count)
	assert.GreaterOrEqual(t, finalCount, int64(5),
		"최소 5회 이상 실행되어야 한다")
}

// TestTimerAgent_SetInterval_MinInterval_Rejected 는 최소 인터벌 미만이면 거부되는지 검증한다.
func TestTimerAgent_SetInterval_MinInterval_Rejected(t *testing.T) {
	agent := newTestTimerAgent(t)

	handler := func(_ TimerTrigger) {}

	// 50ms 는 기본 최소값(100ms) 미만
	_, err := agent.SetInterval("too-fast", 50*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrIntervalTooShort)
}

// TestTimerAgent_SetInterval_MinInterval_Exact 는 최소 인터벌과 정확히 같으면 허용되는지 검증한다.
func TestTimerAgent_SetInterval_MinInterval_Exact(t *testing.T) {
	agent := newTestTimerAgent(t)

	handler := func(_ TimerTrigger) {}

	// 100ms 는 기본 최소값과 같음
	tid, err := agent.SetInterval("exact-min", 100*time.Millisecond, handler)
	assert.NoError(t, err)
	assert.Equal(t, TimerID("exact-min"), tid)
}

// TestTimerAgent_SetInterval_TickCount 은 TickCount가 올바르게 추적되는지 검증한다.
func TestTimerAgent_SetInterval_TickCount(t *testing.T) {
	agent := newTestTimerAgent(t)

	var lastTick int64
	done := make(chan struct{})

	handler := func(trigger TimerTrigger) {
		atomic.StoreInt64(&lastTick, trigger.TickCount)
		if trigger.TickCount >= 3 {
			select {
			case done <- struct{}{}:
			default:
			}
		}
	}

	_, err := agent.SetInterval("counter", 100*time.Millisecond, handler)
	require.NoError(t, err)

	select {
	case <-done:
		tick := atomic.LoadInt64(&lastTick)
		assert.GreaterOrEqual(t, tick, int64(3))
	case <-time.After(2 * time.Second):
		t.Fatal("틱 카운트 3 도달 실패")
	}
}

// TestTimerAgent_SetInterval_Cancel 은 Cancel 후 실행이 중지되는지 검증한다.
func TestTimerAgent_SetInterval_Cancel(t *testing.T) {
	agent := newTestTimerAgent(t)

	var count int64
	started := make(chan struct{})

	handler := func(_ TimerTrigger) {
		atomic.AddInt64(&count, 1)
		select {
		case started <- struct{}{}:
		default:
		}
	}

	tid, err := agent.SetInterval("cancel-me", 100*time.Millisecond, handler)
	require.NoError(t, err)

	// 최소 1회 실행 대기
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("핸들러 첫 실행 대기 초과")
	}

	// 취소
	err = agent.Cancel(tid)
	require.NoError(t, err)

	countAtCancel := atomic.LoadInt64(&count)

	// 취소 후 안정화 대기
	time.Sleep(300 * time.Millisecond)

	countAfter := atomic.LoadInt64(&count)

	// 취소 후 추가 실행이 없거나 매우 적어야 한다
	assert.LessOrEqual(t, countAfter-countAtCancel, int64(1),
		"Cancel 후 추가 실행은 최대 1회 이하여야 한다")
}

// TestTimerAgent_SetInterval_IndependentGoroutines 는 느린 핸들러가 다른 타이머를 차단하지 않는지 검증한다.
func TestTimerAgent_SetInterval_IndependentGoroutines(t *testing.T) {
	agent := newTestTimerAgent(t)

	var fastCount int64
	fastDone := make(chan struct{})

	// 느린 핸들러 A: 500ms 블록
	slowHandler := func(_ TimerTrigger) {
		time.Sleep(500 * time.Millisecond)
	}

	// 빠른 핸들러 B
	fastHandler := func(_ TimerTrigger) {
		if atomic.AddInt64(&fastCount, 1) >= 3 {
			select {
			case fastDone <- struct{}{}:
			default:
			}
		}
	}

	_, err := agent.SetInterval("slow", 200*time.Millisecond, slowHandler)
	require.NoError(t, err)

	_, err = agent.SetInterval("fast", 100*time.Millisecond, fastHandler)
	require.NoError(t, err)

	// 빠른 핸들러는 느린 핸들러에 관계없이 3회 이상 실행되어야 한다
	select {
	case <-fastDone:
		// 성공
	case <-time.After(2 * time.Second):
		t.Fatal("빠른 핸들러가 느린 핸들러에 의해 차단됨")
	}
}

// ---------------------------------------------------------------------------
// 유효성 검사 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_SetInterval_EmptyID 는 빈 ID가 거부되는지 검증한다.
func TestTimerAgent_SetInterval_EmptyID(t *testing.T) {
	agent := newTestTimerAgent(t)
	_, err := agent.SetInterval("", 200*time.Millisecond, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerIDEmpty)
}

// TestTimerAgent_SetInterval_NilHandler 는 nil 핸들러가 거부되는지 검증한다.
func TestTimerAgent_SetInterval_NilHandler(t *testing.T) {
	agent := newTestTimerAgent(t)
	_, err := agent.SetInterval("nil-handler", 200*time.Millisecond, nil)
	assert.ErrorIs(t, err, ErrNilHandler)
}

// TestTimerAgent_SetInterval_DuplicateID 는 중복 ID가 거부되는지 검증한다.
func TestTimerAgent_SetInterval_DuplicateID(t *testing.T) {
	agent := newTestTimerAgent(t)

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetInterval("dup", 200*time.Millisecond, handler)
	require.NoError(t, err)

	_, err = agent.SetInterval("dup", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrDuplicateTimerID)
}

// TestTimerAgent_SetInterval_MaxTimers 는 최대 타이머 수 제한이 동작하는지 검증한다.
func TestTimerAgent_SetInterval_MaxTimers(t *testing.T) {
	agent := newTestTimerAgent(t, WithMaxTimers(2))

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetInterval("t1", 200*time.Millisecond, handler)
	require.NoError(t, err)

	_, err = agent.SetInterval("t2", 200*time.Millisecond, handler)
	require.NoError(t, err)

	_, err = agent.SetInterval("t3", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrMaxTimersReached)
}

// TestTimerAgent_SetInterval_Closed 는 닫힌 에이전트에서 거부되는지 검증한다.
func TestTimerAgent_SetInterval_Closed(t *testing.T) {
	agent := NewTimerAgent()
	require.NoError(t, agent.Init(context.Background()))
	require.NoError(t, agent.Stop(context.Background()))

	_, err := agent.SetInterval("after-close", 200*time.Millisecond, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerClosed)
}

// TestTimerAgent_SetInterval_Paused 는 일시정지 상태에서 거부되는지 검증한다.
func TestTimerAgent_SetInterval_Paused(t *testing.T) {
	agent := newTestTimerAgent(t)
	require.NoError(t, agent.Pause(context.Background()))

	_, err := agent.SetInterval("paused-reg", 200*time.Millisecond, func(_ TimerTrigger) {})
	assert.ErrorIs(t, err, ErrTimerPaused)
}

// TestTimerAgent_SetInterval_ListShowsTimer 는 List에 등록된 타이머가 포함되는지 검증한다.
func TestTimerAgent_SetInterval_ListShowsTimer(t *testing.T) {
	agent := newTestTimerAgent(t)

	_, err := agent.SetInterval("listed", 200*time.Millisecond, func(_ TimerTrigger) {})
	require.NoError(t, err)

	infos := agent.List()
	require.Len(t, infos, 1)
	assert.Equal(t, TimerID("listed"), infos[0].ID)
	assert.Equal(t, TimerTypeInterval, infos[0].Type)
	assert.True(t, infos[0].Active)
}

// TestTimerAgent_SetInterval_CancelNotFound 는 존재하지 않는 타이머 취소 시 에러를 반환하는지 검증한다.
func TestTimerAgent_SetInterval_CancelNotFound(t *testing.T) {
	agent := newTestTimerAgent(t)

	err := agent.Cancel(TimerID("nonexistent"))
	assert.ErrorIs(t, err, ErrTimerNotFound)
}

// TestTimerAgent_SetInterval_CancelRemovesFromList 는 Cancel 후 List에서 제거되는지 검증한다.
func TestTimerAgent_SetInterval_CancelRemovesFromList(t *testing.T) {
	agent := newTestTimerAgent(t)

	tid, err := agent.SetInterval("remove-me", 200*time.Millisecond, func(_ TimerTrigger) {})
	require.NoError(t, err)

	require.Len(t, agent.List(), 1)

	err = agent.Cancel(tid)
	require.NoError(t, err)

	assert.Empty(t, agent.List())
}

// TestTimerAgent_SetInterval_CustomMinInterval 은 커스텀 최소 인터벌 옵션이 동작하는지 검증한다.
func TestTimerAgent_SetInterval_CustomMinInterval(t *testing.T) {
	agent := newTestTimerAgent(t, WithMinInterval(50*time.Millisecond))

	handler := func(_ TimerTrigger) {}

	// 50ms는 커스텀 최소값과 같으므로 허용
	_, err := agent.SetInterval("fast", 50*time.Millisecond, handler)
	assert.NoError(t, err)

	// 30ms는 커스텀 최소값 미만이므로 거부
	_, err = agent.SetInterval("too-fast", 30*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrIntervalTooShort)
}

// TestTimerAgent_SetInterval_ConcurrentRegistration 은 동시 등록이 안전한지 검증한다.
func TestTimerAgent_SetInterval_ConcurrentRegistration(t *testing.T) {
	agent := newTestTimerAgent(t, WithMaxTimers(200))

	var wg sync.WaitGroup
	handler := func(_ TimerTrigger) {}

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("concurrent-%d", idx)
			_, _ = agent.SetInterval(id, 200*time.Millisecond, handler)
		}(i)
	}

	wg.Wait()

	infos := agent.List()
	assert.Equal(t, 100, len(infos), "100개의 타이머가 모두 등록되어야 한다")
}
