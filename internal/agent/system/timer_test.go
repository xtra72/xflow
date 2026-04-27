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
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// TimerAgent 생성 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_NewTimerAgent_StateCreated 는 초기 상태가 Created인지 검증한다.
func TestTimerAgent_NewTimerAgent_StateCreated(t *testing.T) {
	agent := NewTimerAgent()
	assert.Equal(t, lifecycle.StateCreated, agent.State())
}

// TestTimerAgent_NewTimerAgent_WithOptions 는 옵션이 올바르게 적용되는지 검증한다.
func TestTimerAgent_NewTimerAgent_WithOptions(t *testing.T) {
	agent := NewTimerAgent(
		WithMinInterval(50*time.Millisecond),
		WithMaxTimers(500),
	)

	cfg := agent.GetConfig()
	assert.Equal(t, 50*time.Millisecond, cfg["min_interval"])
	assert.Equal(t, 500, cfg["max_timers"])
}

// ---------------------------------------------------------------------------
// Lifecycle 전이 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_Init_TransitionsToRunning 은 Init이 Running으로 전이하는지 검증한다.
func TestTimerAgent_Init_TransitionsToRunning(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()

	err := agent.Init(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, agent.State())

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

// TestTimerAgent_Pause_TransitionsToPaused 는 Pause가 Paused로 전이하는지 검증한다.
func TestTimerAgent_Pause_TransitionsToPaused(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	err := agent.Pause(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, agent.State())
}

// TestTimerAgent_Pause_BlocksNewRegistrations 는 Pause 상태에서 새 등록이 거부되는지 검증한다.
func TestTimerAgent_Pause_BlocksNewRegistrations(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()
	require.NoError(t, agent.Pause(ctx))

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetInterval("paused-interval", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrTimerPaused)

	_, err = agent.SetCron("paused-cron", "* * * * *", handler)
	assert.ErrorIs(t, err, ErrTimerPaused)

	_, err = agent.SetTimeout("paused-timeout", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrTimerPaused)
}

// TestTimerAgent_Pause_AllowsCancelAndList 는 Pause 상태에서 Cancel과 List가 허용되는지 검증한다.
func TestTimerAgent_Pause_AllowsCancelAndList(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	// 타이머 등록
	tid, err := agent.SetInterval("cancel-in-pause", 200*time.Millisecond, func(_ TimerTrigger) {})
	require.NoError(t, err)

	// Pause
	require.NoError(t, agent.Pause(ctx))

	// List 허용
	infos := agent.List()
	assert.Len(t, infos, 1)

	// Cancel 허용
	err = agent.Cancel(tid)
	assert.NoError(t, err)
	assert.Empty(t, agent.List())
}

// TestTimerAgent_Pause_SkipsHandlerCalls 는 Pause 상태에서 핸들러 호출이 스킵되는지 검증한다.
func TestTimerAgent_Pause_SkipsHandlerCalls(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	var count int64
	started := make(chan struct{})

	handler := func(_ TimerTrigger) {
		atomic.AddInt64(&count, 1)
		select {
		case started <- struct{}{}:
		default:
		}
	}

	_, err := agent.SetInterval("pause-skip", 100*time.Millisecond, handler)
	require.NoError(t, err)

	// 최소 1회 실행 대기
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("핸들러 첫 실행 대기 초과")
	}

	// Pause
	require.NoError(t, agent.Pause(ctx))

	countAtPause := atomic.LoadInt64(&count)

	// Pause 후 대기
	time.Sleep(500 * time.Millisecond)

	countAfterPause := atomic.LoadInt64(&count)

	// Pause 동안 추가 실행이 없거나 매우 적어야 한다 (진행 중이던 1회 허용)
	assert.LessOrEqual(t, countAfterPause-countAtPause, int64(1),
		"Pause 동안 핸들러 호출이 스킵되어야 한다")
}

// TestTimerAgent_Resume_TransitionsToRunning 은 Resume이 Running으로 전이하는지 검증한다.
func TestTimerAgent_Resume_TransitionsToRunning(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))
	err := agent.Resume(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, agent.State())
}

// TestTimerAgent_Resume_ReallowsRegistrations 는 Resume 후 등록이 다시 허용되는지 검증한다.
func TestTimerAgent_Resume_ReallowsRegistrations(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))
	require.NoError(t, agent.Resume(ctx))

	_, err := agent.SetInterval("after-resume", 200*time.Millisecond, func(_ TimerTrigger) {})
	assert.NoError(t, err)
}

// TestTimerAgent_Stop_TransitionsToStopped 는 Stop이 Stopped로 전이하는지 검증한다.
func TestTimerAgent_Stop_TransitionsToStopped(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	err := agent.Stop(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, agent.State())
}

// TestTimerAgent_Stop_CancelsAllTimers 는 Stop이 모든 타이머를 취소하는지 검증한다.
func TestTimerAgent_Stop_CancelsAllTimers(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetInterval("t1", 200*time.Millisecond, handler)
	require.NoError(t, err)

	_, err = agent.SetCron("t2", "* * * * *", handler)
	require.NoError(t, err)

	_, err = agent.SetTimeout("t3", 5*time.Second, handler)
	require.NoError(t, err)

	assert.Len(t, agent.List(), 3)

	// Stop
	require.NoError(t, agent.Stop(ctx))

	// Stop 후 모든 타이머 맵이 비어야 한다
	agent.mu.RLock()
	count := len(agent.timers)
	agent.mu.RUnlock()
	assert.Equal(t, 0, count)
}

// TestTimerAgent_Stop_SubsequentOperationsReturnClosed 는 Stop 후 연산이 ErrTimerClosed를 반환하는지 검증한다.
func TestTimerAgent_Stop_SubsequentOperationsReturnClosed(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))
	require.NoError(t, agent.Stop(ctx))

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetInterval("closed-interval", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrTimerClosed)

	_, err = agent.SetCron("closed-cron", "* * * * *", handler)
	assert.ErrorIs(t, err, ErrTimerClosed)

	_, err = agent.SetTimeout("closed-timeout", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrTimerClosed)

	err = agent.Cancel(TimerID("any"))
	assert.ErrorIs(t, err, ErrTimerClosed)
}

// ---------------------------------------------------------------------------
// Configure 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_Configure_Running_ChangeMinInterval 은 Running 상태에서 min_interval 변경을 검증한다.
func TestTimerAgent_Configure_Running_ChangeMinInterval(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	err := agent.Configure(ctx, map[string]any{
		"min_interval": "50ms",
	})
	require.NoError(t, err)

	cfg := agent.GetConfig()
	assert.Equal(t, 50*time.Millisecond, cfg["min_interval"])
}

// TestTimerAgent_Configure_Running_ChangeMaxTimers 는 Running 상태에서 max_timers 변경을 검증한다.
func TestTimerAgent_Configure_Running_ChangeMaxTimers(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	err := agent.Configure(ctx, map[string]any{
		"max_timers": 500,
	})
	require.NoError(t, err)

	cfg := agent.GetConfig()
	assert.Equal(t, 500, cfg["max_timers"])
}

// TestTimerAgent_Configure_Created_ReturnsError 는 Created 상태에서 Configure가 에러를 반환하는지 검증한다.
func TestTimerAgent_Configure_Created_ReturnsError(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()

	err := agent.Configure(ctx, map[string]any{
		"min_interval": "50ms",
	})
	assert.Error(t, err)
	assert.ErrorIs(t, err, lifecycle.ErrInvalidStateForConfigure)
}

// TestTimerAgent_Configure_Paused_Works 는 Paused 상태에서 Configure가 동작하는지 검증한다.
func TestTimerAgent_Configure_Paused_Works(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()
	require.NoError(t, agent.Pause(ctx))

	err := agent.Configure(ctx, map[string]any{
		"min_interval": "200ms",
	})
	require.NoError(t, err)

	cfg := agent.GetConfig()
	assert.Equal(t, 200*time.Millisecond, cfg["min_interval"])
}

// ---------------------------------------------------------------------------
// HealthCheck 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_HealthCheck_Running_Healthy 는 Running 상태에서 건강 상태가 정상인지 검증한다.
func TestTimerAgent_HealthCheck_Running_Healthy(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	status := agent.HealthCheck(ctx)
	assert.True(t, status.Healthy)
	assert.Contains(t, status.Message, "healthy")
	assert.Equal(t, string(lifecycle.StateRunning), status.Details["state"])
	assert.Equal(t, 0, status.Details["active_timers"])
	assert.Equal(t, int64(0), status.Details["total_triggers"])
}

// TestTimerAgent_HealthCheck_WithTimers 는 타이머가 등록된 상태에서 통계를 검증한다.
func TestTimerAgent_HealthCheck_WithTimers(t *testing.T) {
	agent := newTestTimerAgent(t)
	ctx := context.Background()

	handler := func(_ TimerTrigger) {}

	_, err := agent.SetInterval("h-interval", 200*time.Millisecond, handler)
	require.NoError(t, err)

	_, err = agent.SetCron("h-cron", "* * * * *", handler)
	require.NoError(t, err)

	_, err = agent.SetTimeout("h-timeout", 5*time.Second, handler)
	require.NoError(t, err)

	status := agent.HealthCheck(ctx)
	assert.True(t, status.Healthy)
	assert.Equal(t, 3, status.Details["active_timers"])
	assert.Equal(t, 1, status.Details["interval_timers"])
	assert.Equal(t, 1, status.Details["cron_timers"])
	assert.Equal(t, 1, status.Details["timeout_timers"])
}

// TestTimerAgent_HealthCheck_Stopped_NotHealthy 는 Stopped 상태에서 비정상인지 검증한다.
func TestTimerAgent_HealthCheck_Stopped_NotHealthy(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))
	require.NoError(t, agent.Stop(ctx))

	status := agent.HealthCheck(ctx)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not healthy")
}

// ---------------------------------------------------------------------------
// 전체 라이프사이클 통합 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_FullLifecycle 은 전체 라이프사이클 전이를 검증한다.
func TestTimerAgent_FullLifecycle(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()

	// Created -> Init(Running)
	assert.Equal(t, lifecycle.StateCreated, agent.State())
	require.NoError(t, agent.Init(ctx))
	assert.Equal(t, lifecycle.StateRunning, agent.State())

	// Running -> Pause
	require.NoError(t, agent.Pause(ctx))
	assert.Equal(t, lifecycle.StatePaused, agent.State())

	// Pause -> Resume(Running)
	require.NoError(t, agent.Resume(ctx))
	assert.Equal(t, lifecycle.StateRunning, agent.State())

	// Running -> Stop(Stopped)
	require.NoError(t, agent.Stop(ctx))
	assert.Equal(t, lifecycle.StateStopped, agent.State())
}

// TestTimerAgent_DoubleInit_ReturnsError 는 두 번째 Init이 에러를 반환하는지 검증한다.
func TestTimerAgent_DoubleInit_ReturnsError(t *testing.T) {
	agent := NewTimerAgent()
	ctx := context.Background()
	require.NoError(t, agent.Init(ctx))

	err := agent.Init(ctx)
	assert.Error(t, err)

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

// TestTimerAgent_Start_AlreadyRunning_NoError 는 이미 Running 상태에서 Start가 no-op인지 검증한다.
func TestTimerAgent_Start_AlreadyRunning_NoError(t *testing.T) {
	agent := newTestTimerAgent(t)

	err := agent.Start(context.Background())
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// 패닉 복구 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_PanicRecovery 는 핸들러 패닉이 에이전트를 크래시시키지 않는지 검증한다.
func TestTimerAgent_PanicRecovery(t *testing.T) {
	agent := newTestTimerAgent(t)

	panicDone := make(chan struct{})
	normalDone := make(chan struct{})

	// 패닉하는 핸들러
	panicHandler := func(_ TimerTrigger) {
		select {
		case panicDone <- struct{}{}:
		default:
		}
		panic("테스트 패닉!")
	}

	// 정상 핸들러
	normalHandler := func(_ TimerTrigger) {
		select {
		case normalDone <- struct{}{}:
		default:
		}
	}

	_, err := agent.SetInterval("panic-timer", 100*time.Millisecond, panicHandler)
	require.NoError(t, err)

	_, err = agent.SetInterval("normal-timer", 100*time.Millisecond, normalHandler)
	require.NoError(t, err)

	// 패닉 핸들러가 실행됨
	select {
	case <-panicDone:
	case <-time.After(2 * time.Second):
		t.Fatal("패닉 핸들러 실행 대기 초과")
	}

	// 정상 핸들러도 계속 실행됨
	select {
	case <-normalDone:
	case <-time.After(2 * time.Second):
		t.Fatal("정상 핸들러가 패닉에 의해 영향을 받았다")
	}
}

// ---------------------------------------------------------------------------
// 동시성 안전 테스트
// ---------------------------------------------------------------------------

// TestTimerAgent_ConcurrentAccess 는 100개 고루틴에서 동시 접근이 안전한지 검증한다.
func TestTimerAgent_ConcurrentAccess(t *testing.T) {
	agent := newTestTimerAgent(t, WithMaxTimers(500))

	var wg sync.WaitGroup
	handler := func(_ TimerTrigger) {}

	// 100개 고루틴에서 동시에 등록, 목록 조회, 취소
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()

			id := fmt.Sprintf("race-%d", idx)

			// 등록
			tid, err := agent.SetInterval(id, 200*time.Millisecond, handler)
			if err != nil {
				return
			}

			// 목록 조회
			_ = agent.List()

			// 헬스 체크
			_ = agent.HealthCheck(context.Background())

			// 취소
			_ = agent.Cancel(tid)
		}(i)
	}

	wg.Wait()
}

// TestTimerAgent_MaxTimersReached_Interval 은 최대 타이머 수 제한이 전체적으로 동작하는지 검증한다.
func TestTimerAgent_MaxTimersReached_Interval(t *testing.T) {
	agent := newTestTimerAgent(t, WithMaxTimers(3))

	handler := func(_ TimerTrigger) {}

	// 인터벌 1개
	_, err := agent.SetInterval("max-1", 200*time.Millisecond, handler)
	require.NoError(t, err)

	// 크론 1개
	_, err = agent.SetCron("max-2", "* * * * *", handler)
	require.NoError(t, err)

	// 타임아웃 1개
	_, err = agent.SetTimeout("max-3", 5*time.Second, handler)
	require.NoError(t, err)

	// 4번째는 거부
	_, err = agent.SetInterval("max-4", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrMaxTimersReached)

	_, err = agent.SetCron("max-5", "* * * * *", handler)
	assert.ErrorIs(t, err, ErrMaxTimersReached)

	_, err = agent.SetTimeout("max-6", 200*time.Millisecond, handler)
	assert.ErrorIs(t, err, ErrMaxTimersReached)
}

// TestTimerAgent_GetConfig 는 GetConfig가 올바른 설정을 반환하는지 검증한다.
func TestTimerAgent_GetConfig(t *testing.T) {
	agent := NewTimerAgent(
		WithMinInterval(50*time.Millisecond),
		WithMaxTimers(100),
	)

	cfg := agent.GetConfig()
	assert.Equal(t, 50*time.Millisecond, cfg["min_interval"])
	assert.Equal(t, 100, cfg["max_timers"])
}
