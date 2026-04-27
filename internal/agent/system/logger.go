package system

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/xtra/xflow/internal/observe"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// Logger 인터페이스
// ---------------------------------------------------------------------------

// Logger 는 로거 에이전트의 로깅 인터페이스이다.
type Logger interface {
	// WriteLog 는 지정된 컴포넌트와 레벨로 로그를 기록한다.
	WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error

	// SetLevel 은 지정된 컴포넌트의 로그 레벨을 설정한다.
	SetLevel(ctx context.Context, component string, level slog.Level) error

	// SetLevelByPattern 은 패턴에 매칭되는 모든 컴포넌트의 레벨을 설정하고 변경된 수를 반환한다.
	SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error)

	// GetLevel 은 지정된 컴포넌트의 로그 레벨을 반환한다.
	GetLevel(ctx context.Context, component string) (slog.Level, error)

	// Subscribe 는 컴포넌트의 로그 스트림을 Writer로 라우팅하는 구독을 추가한다.
	Subscribe(ctx context.Context, component string, writer io.Writer) (string, error)

	// Unsubscribe 는 구독을 해제한다.
	Unsubscribe(ctx context.Context, subscriptionID string) error

	// Components 는 등록된 모든 컴포넌트 이름 목록을 반환한다.
	Components(ctx context.Context) ([]string, error)
}

// ---------------------------------------------------------------------------
// 내부 타입
// ---------------------------------------------------------------------------

// subscription 은 로그 스트림 구독 정보를 담는 내부 구조체이다.
type subscription struct {
	id        string
	component string
	writer    io.Writer
}

// subscriptionCounter 는 구독 ID 생성을 위한 전역 원자 카운터이다.
var subscriptionCounter atomic.Int64

// ---------------------------------------------------------------------------
// LoggerAgent - Logger System Agent (Lifecycle + HealthChecker)
// ---------------------------------------------------------------------------

// LoggerAgent 는 Logger System Agent이다.
// Lifecycle, HealthChecker, Logger 인터페이스를 구현한다.
type LoggerAgent struct {
	*lifecycle.BaseLifecycle          // 임베딩
	observer *observe.Observer        // 관찰성 시스템 통합 인스턴스
	config   loggerConfig             // 설정
	subs     map[string]*subscription // 활성 구독 맵
	mu       sync.RWMutex             // 상태 보호
	paused   bool                     // Pause 상태 플래그
	closed   bool                     // Stop 상태 플래그
}

// 컴파일 타임 인터페이스 체크
var _ Logger = (*LoggerAgent)(nil)
var _ lifecycle.Lifecycle = (*LoggerAgent)(nil)
var _ lifecycle.HealthChecker = (*LoggerAgent)(nil)

// NewLoggerAgent 는 주어진 옵션으로 LoggerAgent를 생성한다.
// 초기 상태는 StateCreated이며, Observer와 구독 맵은 Init에서 생성된다.
func NewLoggerAgent(opts ...LoggerOption) *LoggerAgent {
	cfg := defaultLoggerConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &LoggerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("logger-agent")),
		config:        cfg,
	}
}

// ---------------------------------------------------------------------------
// Lifecycle 메서드
// ---------------------------------------------------------------------------

// Init 은 LoggerAgent를 초기화한다.
// Created 상태에서만 호출 가능하며, Observer와 구독 맵을 생성하고 Running 상태로 전이한다.
func (l *LoggerAgent) Init(_ context.Context) error {
	// Created -> Initializing 전이
	if err := l.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("logger-agent: init 전이 실패: %w", err)
	}

	// Observer 생성 또는 주입된 것 사용
	if l.config.observer != nil {
		l.observer = l.config.observer
	} else {
		var observerOpts []observe.Option
		observerOpts = append(observerOpts, observe.WithObserverDefaultLevel(l.config.defaultLevel))
		if l.config.format != "" {
			observerOpts = append(observerOpts, observe.WithObserverFormat(l.config.format))
		}
		if l.config.defaultWriter != nil {
			observerOpts = append(observerOpts, observe.WithObserverWriter(l.config.defaultWriter))
		}
		l.observer = observe.New(observerOpts...)
	}

	// 구독 맵 초기화
	l.subs = make(map[string]*subscription)

	// Initializing -> Running 전이
	if err := l.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("logger-agent: running 전이 실패: %w", err)
	}

	return nil
}

// Start 는 컴포넌트를 시작한다.
// Init이 이미 Running으로 전이하므로, 이미 Running 상태이면 no-op이다.
func (l *LoggerAgent) Start(_ context.Context) error {
	if l.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("logger-agent: start는 Running 상태에서만 no-op (현재: %s)", l.CurrentState())
}

// Pause 는 LoggerAgent를 일시정지한다.
// Running -> Paused 전이. Debug/Info 로그는 억제되고 Warn/Error는 허용된다.
func (l *LoggerAgent) Pause(_ context.Context) error {
	if err := l.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("logger-agent: pause 전이 실패: %w", err)
	}

	l.mu.Lock()
	l.paused = true
	l.mu.Unlock()

	return nil
}

// Resume 은 일시정지된 LoggerAgent를 재개한다.
// Paused -> Running 전이.
func (l *LoggerAgent) Resume(_ context.Context) error {
	if err := l.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("logger-agent: resume 전이 실패: %w", err)
	}

	l.mu.Lock()
	l.paused = false
	l.mu.Unlock()

	return nil
}

// Stop 은 LoggerAgent를 정지한다.
// Running 또는 Paused -> Stopping -> Stopped 전이.
// 모든 구독을 해제하고 리소스를 정리한다.
func (l *LoggerAgent) Stop(_ context.Context) error {
	if err := l.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("logger-agent: stopping 전이 실패: %w", err)
	}

	l.mu.Lock()
	l.closed = true

	// 모든 구독 정리
	for id, sub := range l.subs {
		l.observer.Streams.RemoveRoute(sub.component, sub.writer)
		delete(l.subs, id)
	}
	l.mu.Unlock()

	if err := l.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("logger-agent: stopped 전이 실패: %w", err)
	}

	return nil
}

// State 는 컴포넌트의 현재 상태를 반환한다.
func (l *LoggerAgent) State() lifecycle.State {
	return l.CurrentState()
}

// HealthCheck 는 LoggerAgent의 건강 상태를 반환한다.
func (l *LoggerAgent) HealthCheck(_ context.Context) lifecycle.HealthStatus {
	state := l.CurrentState()

	details := map[string]any{
		"state": string(state),
	}

	var msg string
	var healthy bool

	switch state {
	case lifecycle.StateRunning:
		healthy = true
		msg = "logger-agent is healthy"
	case lifecycle.StatePaused:
		healthy = true
		msg = "logger-agent is degraded (paused)"
	default:
		healthy = false
		msg = fmt.Sprintf("logger-agent is not healthy (state: %s)", state)
	}

	// Running/Paused 상태에서 추가 상세 정보를 포함한다
	if healthy {
		l.mu.RLock()
		details["active_subscriptions"] = len(l.subs)
		l.mu.RUnlock()

		if l.observer != nil {
			details["registered_components"] = len(l.observer.Loggers.Components())
		}
	}

	return lifecycle.HealthStatus{
		Healthy:     healthy,
		Message:     msg,
		LastChecked: time.Now(),
		Details:     details,
	}
}

// ---------------------------------------------------------------------------
// Logger 인터페이스 메서드
// ---------------------------------------------------------------------------

// WriteLog 는 지정된 컴포넌트와 레벨로 로그를 기록한다.
func (l *LoggerAgent) WriteLog(_ context.Context, component string, level slog.Level, msg string, args ...any) error {
	l.mu.RLock()
	closed := l.closed
	paused := l.paused
	l.mu.RUnlock()

	if closed {
		return ErrLoggerClosed
	}
	if component == "" {
		return ErrInvalidComponent
	}
	// Pause 상태에서 Debug/Info 레벨은 억제 (에러 없이 nil 반환)
	if paused && level < slog.LevelWarn {
		return nil
	}

	// ComponentLogger를 통해 로그 기록
	cl := l.observer.Loggers.NewLogger(component)
	switch {
	case level >= slog.LevelError:
		cl.Error(msg, args...)
	case level >= slog.LevelWarn:
		cl.Warn(msg, args...)
	case level >= slog.LevelInfo:
		cl.Info(msg, args...)
	default:
		cl.Debug(msg, args...)
	}

	return nil
}

// SetLevel 은 지정된 컴포넌트의 로그 레벨을 설정한다.
func (l *LoggerAgent) SetLevel(_ context.Context, component string, level slog.Level) error {
	l.mu.RLock()
	closed := l.closed
	l.mu.RUnlock()

	if closed {
		return ErrLoggerClosed
	}
	if component == "" {
		return ErrInvalidComponent
	}

	l.observer.Levels.SetLevel(component, level)
	return nil
}

// SetLevelByPattern 은 패턴에 매칭되는 모든 컴포넌트의 레벨을 설정하고 변경된 수를 반환한다.
func (l *LoggerAgent) SetLevelByPattern(_ context.Context, pattern string, level slog.Level) (int, error) {
	l.mu.RLock()
	closed := l.closed
	l.mu.RUnlock()

	if closed {
		return 0, ErrLoggerClosed
	}

	count := l.observer.Levels.SetLevelByPattern(pattern, level)
	return count, nil
}

// GetLevel 은 지정된 컴포넌트의 로그 레벨을 반환한다.
func (l *LoggerAgent) GetLevel(_ context.Context, component string) (slog.Level, error) {
	l.mu.RLock()
	closed := l.closed
	l.mu.RUnlock()

	if closed {
		return 0, ErrLoggerClosed
	}
	if component == "" {
		return 0, ErrInvalidComponent
	}

	level := l.observer.Levels.GetLevel(component)
	return level, nil
}

// Subscribe 는 컴포넌트의 로그 스트림을 Writer로 라우팅하는 구독을 추가한다.
// 생성된 구독 ID를 반환한다.
func (l *LoggerAgent) Subscribe(_ context.Context, component string, writer io.Writer) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return "", ErrLoggerClosed
	}
	if component == "" {
		return "", ErrInvalidComponent
	}

	// 구독 ID 생성
	counter := subscriptionCounter.Add(1)
	id := fmt.Sprintf("sub-%s-%d", component, counter)

	// StreamRouter에 라우트 추가
	l.observer.Streams.AddRoute(component, writer)

	// 구독 맵에 저장
	l.subs[id] = &subscription{
		id:        id,
		component: component,
		writer:    writer,
	}

	return id, nil
}

// Unsubscribe 는 구독을 해제한다.
func (l *LoggerAgent) Unsubscribe(_ context.Context, subscriptionID string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.closed {
		return ErrLoggerClosed
	}

	sub, exists := l.subs[subscriptionID]
	if !exists {
		return ErrSubscriptionNotFound
	}

	l.observer.Streams.RemoveRoute(sub.component, sub.writer)
	delete(l.subs, subscriptionID)

	return nil
}

// Components 는 등록된 모든 컴포넌트 이름 목록을 반환한다.
func (l *LoggerAgent) Components(_ context.Context) ([]string, error) {
	l.mu.RLock()
	closed := l.closed
	l.mu.RUnlock()

	if closed {
		return nil, ErrLoggerClosed
	}

	return l.observer.Loggers.Components(), nil
}
