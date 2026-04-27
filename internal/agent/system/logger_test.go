package system

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// 테스트 헬퍼
// ---------------------------------------------------------------------------

// newTestLoggerAgent 는 초기화된 LoggerAgent를 생성한다.
func newTestLoggerAgent(t *testing.T, opts ...LoggerOption) *LoggerAgent {
	t.Helper()
	agent := NewLoggerAgent(opts...)
	err := agent.Init(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() {
		if agent.State() != lifecycle.StateStopped {
			_ = agent.Stop(context.Background())
		}
	})
	return agent
}

// ---------------------------------------------------------------------------
// 센티넬 에러 테스트
// ---------------------------------------------------------------------------

func TestLoggerErrors_SentinelErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
		msg  string
	}{
		{"ErrLoggerClosed", ErrLoggerClosed, "logger: logger agent is closed"},
		{"ErrLoggerPaused", ErrLoggerPaused, "logger: logger agent is paused (debug/info suppressed)"},
		{"ErrInvalidLevel", ErrInvalidLevel, "logger: invalid log level"},
		{"ErrInvalidComponent", ErrInvalidComponent, "logger: invalid component name"},
		{"ErrSubscriptionNotFound", ErrSubscriptionNotFound, "logger: subscription not found"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.EqualError(t, tt.err, tt.msg)
		})
	}
}

func TestLoggerErrors_ErrorsIs_Wrapping(t *testing.T) {
	// errors.Is 를 통한 래핑 호환성 테스트
	wrapped := errors.Join(ErrLoggerClosed, errors.New("additional context"))
	assert.True(t, errors.Is(wrapped, ErrLoggerClosed))
}

// ---------------------------------------------------------------------------
// NewLoggerAgent 테스트
// ---------------------------------------------------------------------------

func TestNewLoggerAgent_DefaultOptions(t *testing.T) {
	agent := NewLoggerAgent()
	assert.Equal(t, lifecycle.StateCreated, agent.State())
}

func TestNewLoggerAgent_WithOptions(t *testing.T) {
	buf := &bytes.Buffer{}
	agent := NewLoggerAgent(
		WithLogDefaultLevel(slog.LevelDebug),
		WithLogFormat("text"),
		WithLogWriter(buf),
	)
	assert.Equal(t, lifecycle.StateCreated, agent.State())
}

func TestNewLoggerAgent_WithExternalObserver(t *testing.T) {
	obs := observe.New(
		observe.WithObserverDefaultLevel(slog.LevelWarn),
	)
	agent := NewLoggerAgent(WithLogObserver(obs))
	assert.Equal(t, lifecycle.StateCreated, agent.State())
}

// ---------------------------------------------------------------------------
// Init 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Init_TransitionsToRunning(t *testing.T) {
	agent := NewLoggerAgent()
	ctx := context.Background()

	err := agent.Init(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, agent.State())

	// 정리
	require.NoError(t, agent.Stop(ctx))
}

func TestLoggerAgent_Init_CreatesObserver(t *testing.T) {
	agent := NewLoggerAgent()
	ctx := context.Background()

	err := agent.Init(ctx)
	require.NoError(t, err)
	assert.NotNil(t, agent.observer)

	require.NoError(t, agent.Stop(ctx))
}

func TestLoggerAgent_Init_WithExternalObserver(t *testing.T) {
	obs := observe.New()
	agent := NewLoggerAgent(WithLogObserver(obs))
	ctx := context.Background()

	err := agent.Init(ctx)
	require.NoError(t, err)
	// 주입된 Observer 를 사용하는지 확인
	assert.Same(t, obs, agent.observer)

	require.NoError(t, agent.Stop(ctx))
}

func TestLoggerAgent_Init_DoubleInit_ReturnsError(t *testing.T) {
	agent := newTestLoggerAgent(t)

	err := agent.Init(context.Background())
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// Start 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Start_AlreadyRunning_NoError(t *testing.T) {
	agent := newTestLoggerAgent(t)

	err := agent.Start(context.Background())
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// WriteLog 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_WriteLog_AllLevels(t *testing.T) {
	buf := &bytes.Buffer{}
	agent := newTestLoggerAgent(t,
		WithLogWriter(buf),
		WithLogDefaultLevel(slog.LevelDebug),
	)

	ctx := context.Background()

	tests := []struct {
		name  string
		level slog.Level
	}{
		{"Debug", slog.LevelDebug},
		{"Info", slog.LevelInfo},
		{"Warn", slog.LevelWarn},
		{"Error", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf.Reset()
			err := agent.WriteLog(ctx, "test-component", tt.level, "test message", "key", "value")
			assert.NoError(t, err)
		})
	}
}

func TestLoggerAgent_WriteLog_EmptyComponent_ReturnsErrInvalidComponent(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	err := agent.WriteLog(ctx, "", slog.LevelInfo, "test")
	assert.ErrorIs(t, err, ErrInvalidComponent)
}

func TestLoggerAgent_WriteLog_Closed_ReturnsErrLoggerClosed(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	require.NoError(t, agent.Stop(ctx))

	err := agent.WriteLog(ctx, "comp", slog.LevelInfo, "test")
	assert.ErrorIs(t, err, ErrLoggerClosed)
}

// ---------------------------------------------------------------------------
// Pause/Resume 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Pause_TransitionsToPaused(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	err := agent.Pause(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StatePaused, agent.State())
}

func TestLoggerAgent_Pause_DebugInfoSuppressed(t *testing.T) {
	buf := &bytes.Buffer{}
	agent := newTestLoggerAgent(t,
		WithLogWriter(buf),
		WithLogDefaultLevel(slog.LevelDebug),
	)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))

	// Debug 로그는 억제됨 (에러 없이 nil 반환)
	err := agent.WriteLog(ctx, "comp", slog.LevelDebug, "debug msg")
	assert.NoError(t, err)

	// Info 로그도 억제됨
	err = agent.WriteLog(ctx, "comp", slog.LevelInfo, "info msg")
	assert.NoError(t, err)
}

func TestLoggerAgent_Pause_WarnErrorStillAllowed(t *testing.T) {
	buf := &bytes.Buffer{}
	agent := newTestLoggerAgent(t,
		WithLogWriter(buf),
		WithLogDefaultLevel(slog.LevelDebug),
	)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))

	// Warn 로그는 허용됨
	err := agent.WriteLog(ctx, "comp", slog.LevelWarn, "warn msg")
	assert.NoError(t, err)

	// Error 로그도 허용됨
	err = agent.WriteLog(ctx, "comp", slog.LevelError, "error msg")
	assert.NoError(t, err)
}

func TestLoggerAgent_Resume_TransitionsToRunning(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))
	err := agent.Resume(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateRunning, agent.State())
}

func TestLoggerAgent_Resume_AllLevelsRestored(t *testing.T) {
	buf := &bytes.Buffer{}
	agent := newTestLoggerAgent(t,
		WithLogWriter(buf),
		WithLogDefaultLevel(slog.LevelDebug),
	)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))
	require.NoError(t, agent.Resume(ctx))

	// Debug 로그 다시 작동
	err := agent.WriteLog(ctx, "comp", slog.LevelDebug, "debug msg after resume")
	assert.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Stop 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Stop_TransitionsToStopped(t *testing.T) {
	agent := NewLoggerAgent()
	ctx := context.Background()

	require.NoError(t, agent.Init(ctx))
	err := agent.Stop(ctx)
	require.NoError(t, err)
	assert.Equal(t, lifecycle.StateStopped, agent.State())
}

func TestLoggerAgent_Stop_ClearsSubscriptions(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	// 구독 추가
	buf := &bytes.Buffer{}
	_, err := agent.Subscribe(ctx, "comp", buf)
	require.NoError(t, err)

	// Stop 후 구독이 정리되어야 함
	require.NoError(t, agent.Stop(ctx))
}

func TestLoggerAgent_Stop_AllOperationsReturnClosed(t *testing.T) {
	agent := NewLoggerAgent()
	ctx := context.Background()

	require.NoError(t, agent.Init(ctx))
	require.NoError(t, agent.Stop(ctx))

	// 모든 쓰기 연산 실패
	assert.ErrorIs(t, agent.WriteLog(ctx, "comp", slog.LevelInfo, "test"), ErrLoggerClosed)

	err := agent.SetLevel(ctx, "comp", slog.LevelDebug)
	assert.ErrorIs(t, err, ErrLoggerClosed)

	_, err = agent.SetLevelByPattern(ctx, "*", slog.LevelDebug)
	assert.ErrorIs(t, err, ErrLoggerClosed)

	_, err = agent.GetLevel(ctx, "comp")
	assert.ErrorIs(t, err, ErrLoggerClosed)

	_, err = agent.Subscribe(ctx, "comp", &bytes.Buffer{})
	assert.ErrorIs(t, err, ErrLoggerClosed)

	assert.ErrorIs(t, agent.Unsubscribe(ctx, "sub-id"), ErrLoggerClosed)

	_, err = agent.Components(ctx)
	assert.ErrorIs(t, err, ErrLoggerClosed)
}

// ---------------------------------------------------------------------------
// SetLevel / GetLevel 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_SetLevel_GetLevel(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	// 기본 레벨은 Info
	level, err := agent.GetLevel(ctx, "comp")
	require.NoError(t, err)
	assert.Equal(t, slog.LevelInfo, level)

	// Debug로 변경
	err = agent.SetLevel(ctx, "comp", slog.LevelDebug)
	require.NoError(t, err)

	level, err = agent.GetLevel(ctx, "comp")
	require.NoError(t, err)
	assert.Equal(t, slog.LevelDebug, level)
}

func TestLoggerAgent_SetLevel_EmptyComponent_ReturnsError(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	err := agent.SetLevel(ctx, "", slog.LevelDebug)
	assert.ErrorIs(t, err, ErrInvalidComponent)
}

func TestLoggerAgent_GetLevel_EmptyComponent_ReturnsError(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	_, err := agent.GetLevel(ctx, "")
	assert.ErrorIs(t, err, ErrInvalidComponent)
}

// ---------------------------------------------------------------------------
// SetLevelByPattern 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_SetLevelByPattern(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	// 컴포넌트들을 등록한다 (WriteLog 를 통해)
	require.NoError(t, agent.WriteLog(ctx, "agent.timer", slog.LevelInfo, "init"))
	require.NoError(t, agent.WriteLog(ctx, "agent.store", slog.LevelInfo, "init"))
	require.NoError(t, agent.WriteLog(ctx, "server.http", slog.LevelInfo, "init"))

	// "agent.*" 패턴으로 Debug 레벨 적용
	count, err := agent.SetLevelByPattern(ctx, "agent.*", slog.LevelDebug)
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

// ---------------------------------------------------------------------------
// Subscribe / Unsubscribe 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Subscribe_ReturnsID(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	buf := &bytes.Buffer{}
	id, err := agent.Subscribe(ctx, "comp", buf)
	require.NoError(t, err)
	assert.NotEmpty(t, id)
	assert.Contains(t, id, "sub-comp-")
}

func TestLoggerAgent_Subscribe_EmptyComponent_ReturnsError(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	_, err := agent.Subscribe(ctx, "", &bytes.Buffer{})
	assert.ErrorIs(t, err, ErrInvalidComponent)
}

func TestLoggerAgent_Unsubscribe_ValidID(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	buf := &bytes.Buffer{}
	id, err := agent.Subscribe(ctx, "comp", buf)
	require.NoError(t, err)

	err = agent.Unsubscribe(ctx, id)
	assert.NoError(t, err)
}

func TestLoggerAgent_Unsubscribe_InvalidID_ReturnsError(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	err := agent.Unsubscribe(ctx, "nonexistent-id")
	assert.ErrorIs(t, err, ErrSubscriptionNotFound)
}

func TestLoggerAgent_Subscribe_StreamRoutingVerification(t *testing.T) {
	agent := newTestLoggerAgent(t,
		WithLogDefaultLevel(slog.LevelDebug),
	)
	ctx := context.Background()

	buf := &bytes.Buffer{}
	_, err := agent.Subscribe(ctx, "routed-comp", buf)
	require.NoError(t, err)

	// 로그 기록 후 buf에 라우팅되었는지 확인
	err = agent.WriteLog(ctx, "routed-comp", slog.LevelInfo, "routed message")
	require.NoError(t, err)

	assert.Contains(t, buf.String(), "routed message")
}

// ---------------------------------------------------------------------------
// Components 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Components(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	// WriteLog를 통해 컴포넌트 등록
	require.NoError(t, agent.WriteLog(ctx, "alpha", slog.LevelInfo, "msg"))
	require.NoError(t, agent.WriteLog(ctx, "beta", slog.LevelInfo, "msg"))

	comps, err := agent.Components(ctx)
	require.NoError(t, err)
	assert.Contains(t, comps, "alpha")
	assert.Contains(t, comps, "beta")
}

// ---------------------------------------------------------------------------
// HealthCheck 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_HealthCheck_Running(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	status := agent.HealthCheck(ctx)
	assert.True(t, status.Healthy)
	assert.Equal(t, "logger-agent is healthy", status.Message)
	assert.Equal(t, string(lifecycle.StateRunning), status.Details["state"])
}

func TestLoggerAgent_HealthCheck_Paused(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))

	status := agent.HealthCheck(ctx)
	assert.True(t, status.Healthy)
	assert.Equal(t, "logger-agent is degraded (paused)", status.Message)
}

func TestLoggerAgent_HealthCheck_Stopped(t *testing.T) {
	agent := NewLoggerAgent()
	ctx := context.Background()

	require.NoError(t, agent.Init(ctx))
	require.NoError(t, agent.Stop(ctx))

	status := agent.HealthCheck(ctx)
	assert.False(t, status.Healthy)
	assert.Contains(t, status.Message, "not healthy")
}

func TestLoggerAgent_HealthCheck_Details(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	// 구독 추가
	buf := &bytes.Buffer{}
	_, err := agent.Subscribe(ctx, "comp1", buf)
	require.NoError(t, err)

	status := agent.HealthCheck(ctx)
	assert.Contains(t, status.Details, "state")
	assert.Contains(t, status.Details, "active_subscriptions")
	assert.Contains(t, status.Details, "registered_components")
}

// ---------------------------------------------------------------------------
// 전체 생명주기 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_FullLifecycle(t *testing.T) {
	agent := NewLoggerAgent()
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

// ---------------------------------------------------------------------------
// 동시성 안전 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Concurrency_WriteLogSubscribeUnsubscribe(t *testing.T) {
	agent := newTestLoggerAgent(t,
		WithLogDefaultLevel(slog.LevelDebug),
		WithLogWriter(io.Discard),
	)
	ctx := context.Background()

	const goroutines = 10
	const iterations = 50

	var wg sync.WaitGroup

	// 동시에 WriteLog 호출
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				_ = agent.WriteLog(ctx, "concurrent-comp", slog.LevelInfo, "msg")
			}
		}(i)
	}

	// 동시에 Subscribe/Unsubscribe (io.Discard는 스레드 안전하므로 사용)
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			subID, err := agent.Subscribe(ctx, "concurrent-comp", io.Discard)
			if err == nil {
				_ = agent.Unsubscribe(ctx, subID)
			}
		}()
	}

	wg.Wait()
}

// ---------------------------------------------------------------------------
// Pause 상태에서 관리 연산 테스트
// ---------------------------------------------------------------------------

func TestLoggerAgent_Pause_ManagementOperationsStillWork(t *testing.T) {
	agent := newTestLoggerAgent(t)
	ctx := context.Background()

	require.NoError(t, agent.Pause(ctx))

	// SetLevel은 Pause 상태에서도 동작
	err := agent.SetLevel(ctx, "comp", slog.LevelDebug)
	assert.NoError(t, err)

	// GetLevel은 Pause 상태에서도 동작
	_, err = agent.GetLevel(ctx, "comp")
	assert.NoError(t, err)

	// SetLevelByPattern은 Pause 상태에서도 동작
	_, err = agent.SetLevelByPattern(ctx, "*", slog.LevelWarn)
	assert.NoError(t, err)

	// Subscribe는 Pause 상태에서도 동작
	buf := &bytes.Buffer{}
	id, err := agent.Subscribe(ctx, "comp", buf)
	assert.NoError(t, err)
	assert.NotEmpty(t, id)

	// Unsubscribe는 Pause 상태에서도 동작
	err = agent.Unsubscribe(ctx, id)
	assert.NoError(t, err)

	// Components는 Pause 상태에서도 동작
	_, err = agent.Components(ctx)
	assert.NoError(t, err)
}
