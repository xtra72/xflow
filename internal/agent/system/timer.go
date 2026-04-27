package system

import (
	"context"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"github.com/xtra/xflow/pkg/lifecycle"
)

// ---------------------------------------------------------------------------
// 타입 정의
// ---------------------------------------------------------------------------

// TimerID 는 타이머의 고유 식별자 타입이다.
type TimerID string

// TimerType 은 타이머 종류를 나타내는 타입이다.
type TimerType string

const (
	// TimerTypeInterval 은 주기적으로 반복 실행되는 인터벌 타이머이다.
	TimerTypeInterval TimerType = "interval"

	// TimerTypeCron 은 cron 표현식 기반으로 실행되는 크론 타이머이다.
	TimerTypeCron TimerType = "cron"

	// TimerTypeTimeout 은 지정된 시간 후 1회 실행되는 타임아웃 타이머이다.
	TimerTypeTimeout TimerType = "timeout"
)

// TimerHandler 는 타이머 트리거 시 호출되는 핸들러 함수 타입이다.
type TimerHandler func(trigger TimerTrigger)

// TimerTrigger 는 타이머 트리거 시 핸들러에 전달되는 정보 구조체이다.
type TimerTrigger struct {
	TimerID    TimerID   // 트리거된 타이머의 ID
	TriggerAt  time.Time // 트리거 시각
	TickCount  int64     // 누적 틱 수
	ScheduleID string    // cron 스케줄 ID (cron 타이머 전용)
}

// TimerInfo 는 타이머의 상태 정보를 나타내는 구조체이다.
type TimerInfo struct {
	ID         TimerID   // 타이머 ID
	Type       TimerType // 타이머 종류
	Expression string    // 인터벌/cron/딜레이 표현식
	NextFire   time.Time // 다음 실행 예정 시각
	LastFired  time.Time // 마지막 실행 시각
	FireCount  int64     // 실행 횟수
	Active     bool      // 활성 상태 여부
}

// ---------------------------------------------------------------------------
// Timer 인터페이스
// ---------------------------------------------------------------------------

// Timer 는 타이머 등록 및 관리를 위한 인터페이스이다.
type Timer interface {
	// SetInterval 은 주기적으로 반복 실행되는 인터벌 타이머를 등록한다.
	SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error)

	// SetCron 은 cron 표현식 기반 타이머를 등록한다.
	SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error)

	// SetTimeout 은 지정된 시간 후 1회 실행되는 타임아웃 타이머를 등록한다.
	SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error)

	// Cancel 은 지정된 타이머를 취소한다.
	Cancel(id TimerID) error

	// List 는 등록된 모든 타이머의 정보를 반환한다.
	List() []TimerInfo
}

// ---------------------------------------------------------------------------
// 내부 타입
// ---------------------------------------------------------------------------

// timerEntry 는 등록된 타이머의 내부 정보를 담는 구조체이다.
type timerEntry struct {
	info      TimerInfo
	handler   TimerHandler
	ticker    *time.Ticker   // interval 타이머 전용
	cronID    cron.EntryID   // cron 타이머 전용
	timer     *time.Timer    // timeout 타이머 전용
	cancel    context.CancelFunc // interval 타이머 고루틴 취소용
	tickCount int64          // atomic: 누적 틱 수
}

// timerStats 는 타이머 에이전트의 통계 정보 구조체이다.
type timerStats struct {
	totalTriggers int64 // atomic: 전체 트리거 횟수
}

// timerConfig 는 타이머 에이전트의 내부 설정 구조체이다.
type timerConfig struct {
	minInterval time.Duration // 최소 인터벌 (기본 100ms)
	maxTimers   int           // 최대 타이머 수 (기본 1000)
	cronParser  cron.Parser   // cron 파서
}

// ---------------------------------------------------------------------------
// TimerAgent - Timer System Agent (Lifecycle + Configurable + HealthChecker)
// ---------------------------------------------------------------------------

// TimerAgent 는 Timer System Agent이다.
// Lifecycle, Configurable, HealthChecker, Timer 인터페이스를 구현한다.
type TimerAgent struct {
	*lifecycle.BaseLifecycle          // 임베딩
	config    timerConfig             // 설정
	mu        sync.RWMutex            // 상태 보호
	timers    map[TimerID]*timerEntry // 등록된 타이머 맵
	cronSched *cron.Cron              // cron 스케줄러
	wg        sync.WaitGroup          // 핸들러 완료 대기
	paused    bool                    // Pause 상태 플래그
	closed    bool                    // Stop 상태 플래그
	stats     timerStats              // 통계 정보
}

// 컴파일 타임 인터페이스 체크
var _ Timer = (*TimerAgent)(nil)
var _ lifecycle.Lifecycle = (*TimerAgent)(nil)
var _ lifecycle.Configurable = (*TimerAgent)(nil)
var _ lifecycle.HealthChecker = (*TimerAgent)(nil)

// NewTimerAgent 는 주어진 옵션으로 TimerAgent를 생성한다.
// 초기 상태는 StateCreated이며, cron 스케줄러와 타이머 맵은 Init에서 생성된다.
func NewTimerAgent(opts ...TimerOption) *TimerAgent {
	cfg := defaultTimerConfig()
	for _, opt := range opts {
		opt(&cfg)
	}

	return &TimerAgent{
		BaseLifecycle: lifecycle.NewBaseLifecycle(lifecycle.WithName("timer-agent")),
		config:        cfg,
	}
}

// ---------------------------------------------------------------------------
// Lifecycle 메서드
// ---------------------------------------------------------------------------

// Init 은 TimerAgent를 초기화한다.
// Created 상태에서만 호출 가능하며, cron 스케줄러와 타이머 맵을 생성하고 Running 상태로 전이한다.
func (t *TimerAgent) Init(_ context.Context) error {
	// Created -> Initializing 전이
	if err := t.TransitionTo(lifecycle.StateInitializing); err != nil {
		return fmt.Errorf("timer-agent: init 전이 실패: %w", err)
	}

	// 타이머 맵 초기화
	t.timers = make(map[TimerID]*timerEntry)

	// cron 스케줄러 생성 및 시작
	t.cronSched = cron.New(cron.WithParser(t.config.cronParser))
	t.cronSched.Start()

	// Initializing -> Running 전이
	if err := t.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("timer-agent: running 전이 실패: %w", err)
	}

	return nil
}

// Start 는 컴포넌트를 시작한다.
// Init이 이미 Running으로 전이하므로, 이미 Running 상태이면 no-op이다.
func (t *TimerAgent) Start(_ context.Context) error {
	if t.CurrentState() == lifecycle.StateRunning {
		return nil
	}
	return fmt.Errorf("timer-agent: start는 Running 상태에서만 no-op (현재: %s)", t.CurrentState())
}

// Pause 는 TimerAgent를 일시정지한다.
// Running -> Paused 전이. 새로운 타이머 등록은 비활성화되고 핸들러 호출이 스킵된다.
func (t *TimerAgent) Pause(_ context.Context) error {
	if err := t.TransitionTo(lifecycle.StatePaused); err != nil {
		return fmt.Errorf("timer-agent: pause 전이 실패: %w", err)
	}

	t.mu.Lock()
	t.paused = true
	t.mu.Unlock()

	return nil
}

// Resume 은 일시정지된 TimerAgent를 재개한다.
// Paused -> Running 전이.
func (t *TimerAgent) Resume(_ context.Context) error {
	if err := t.TransitionTo(lifecycle.StateRunning); err != nil {
		return fmt.Errorf("timer-agent: resume 전이 실패: %w", err)
	}

	t.mu.Lock()
	t.paused = false
	t.mu.Unlock()

	return nil
}

// Stop 은 TimerAgent를 정지한다.
// Running 또는 Paused -> Stopping -> Stopped 전이.
// 모든 타이머를 취소하고, cron 스케줄러를 정지하며, 진행 중인 핸들러 완료를 대기한다.
func (t *TimerAgent) Stop(_ context.Context) error {
	if err := t.TransitionTo(lifecycle.StateStopping); err != nil {
		return fmt.Errorf("timer-agent: stopping 전이 실패: %w", err)
	}

	t.mu.Lock()
	t.closed = true

	// 모든 타이머 정리
	for id, entry := range t.timers {
		t.cancelEntryLocked(entry)
		delete(t.timers, id)
	}
	t.mu.Unlock()

	// cron 스케줄러 정지
	if t.cronSched != nil {
		ctx := t.cronSched.Stop()
		<-ctx.Done()
	}

	// 진행 중인 핸들러 완료 대기
	t.wg.Wait()

	if err := t.TransitionTo(lifecycle.StateStopped); err != nil {
		return fmt.Errorf("timer-agent: stopped 전이 실패: %w", err)
	}

	return nil
}

// State 는 컴포넌트의 현재 상태를 반환한다.
func (t *TimerAgent) State() lifecycle.State {
	return t.CurrentState()
}

// Configure 는 런타임에 설정을 변경한다.
// Running 또는 Paused 상태에서만 호출 가능하다.
// 지원 키: "min_interval" (string duration), "max_timers" (int)
func (t *TimerAgent) Configure(_ context.Context, cfg map[string]any) error {
	state := t.CurrentState()
	if state != lifecycle.StateRunning && state != lifecycle.StatePaused {
		return fmt.Errorf("timer-agent: configure는 Running/Paused에서만 가능 (현재: %s): %w",
			state, lifecycle.ErrInvalidStateForConfigure)
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if v, ok := cfg["min_interval"]; ok {
		str, ok := v.(string)
		if !ok {
			return fmt.Errorf("timer-agent: min_interval은 string이어야 한다")
		}
		d, err := time.ParseDuration(str)
		if err != nil {
			return fmt.Errorf("timer-agent: min_interval 파싱 실패: %w", err)
		}
		t.config.minInterval = d
	}

	if v, ok := cfg["max_timers"]; ok {
		n, ok := v.(int)
		if !ok {
			return fmt.Errorf("timer-agent: max_timers는 int이어야 한다")
		}
		t.config.maxTimers = n
	}

	return nil
}

// GetConfig 는 현재 설정을 map으로 반환한다.
func (t *TimerAgent) GetConfig() map[string]any {
	t.mu.RLock()
	defer t.mu.RUnlock()

	return map[string]any{
		"min_interval": t.config.minInterval,
		"max_timers":   t.config.maxTimers,
	}
}

// HealthCheck 는 TimerAgent의 건강 상태를 반환한다.
func (t *TimerAgent) HealthCheck(_ context.Context) lifecycle.HealthStatus {
	state := t.CurrentState()
	healthy := state == lifecycle.StateRunning || state == lifecycle.StatePaused

	details := map[string]any{
		"state": string(state),
	}

	// Running/Paused 상태에서만 타이머 통계를 집계한다
	if healthy {
		t.mu.RLock()
		intervalCount := 0
		cronCount := 0
		timeoutCount := 0
		for _, entry := range t.timers {
			switch entry.info.Type {
			case TimerTypeInterval:
				intervalCount++
			case TimerTypeCron:
				cronCount++
			case TimerTypeTimeout:
				timeoutCount++
			}
		}
		activeTimers := len(t.timers)
		t.mu.RUnlock()

		details["active_timers"] = activeTimers
		details["interval_timers"] = intervalCount
		details["cron_timers"] = cronCount
		details["timeout_timers"] = timeoutCount
		details["total_triggers"] = atomic.LoadInt64(&t.stats.totalTriggers)
	}

	message := "timer-agent is healthy"
	if !healthy {
		message = fmt.Sprintf("timer-agent is not healthy (state: %s)", state)
	}

	return lifecycle.HealthStatus{
		Healthy:     healthy,
		Message:     message,
		LastChecked: time.Now(),
		Details:     details,
	}
}

// ---------------------------------------------------------------------------
// Timer 인터페이스 메서드
// ---------------------------------------------------------------------------

// Cancel 은 지정된 타이머를 취소한다.
// 타이머 ID가 존재하지 않으면 ErrTimerNotFound를 반환한다.
func (t *TimerAgent) Cancel(id TimerID) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return ErrTimerClosed
	}

	entry, exists := t.timers[id]
	if !exists {
		return ErrTimerNotFound
	}

	t.cancelEntryLocked(entry)
	delete(t.timers, id)

	return nil
}

// List 는 등록된 모든 타이머의 정보를 반환한다.
func (t *TimerAgent) List() []TimerInfo {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make([]TimerInfo, 0, len(t.timers))
	for _, entry := range t.timers {
		info := entry.info
		info.FireCount = atomic.LoadInt64(&entry.tickCount)
		result = append(result, info)
	}

	return result
}

// ---------------------------------------------------------------------------
// 내부 헬퍼 메서드
// ---------------------------------------------------------------------------

// checkRegistration 은 타이머 등록이 가능한지 확인한다 (closed + paused + 공통 유효성 검사).
// 뮤텍스 외부에서 호출하며, 내부에서 RLock을 사용한다.
func (t *TimerAgent) checkRegistration(id string, handler TimerHandler) error {
	if id == "" {
		return ErrTimerIDEmpty
	}
	if handler == nil {
		return ErrNilHandler
	}

	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return ErrTimerClosed
	}
	if t.paused {
		return ErrTimerPaused
	}
	if _, exists := t.timers[TimerID(id)]; exists {
		return ErrDuplicateTimerID
	}
	if len(t.timers) >= t.config.maxTimers {
		return ErrMaxTimersReached
	}

	return nil
}

// cancelEntryLocked 는 타이머 엔트리를 취소한다. 뮤텍스가 이미 잠긴 상태에서 호출해야 한다.
func (t *TimerAgent) cancelEntryLocked(entry *timerEntry) {
	entry.info.Active = false

	switch entry.info.Type {
	case TimerTypeInterval:
		if entry.ticker != nil {
			entry.ticker.Stop()
		}
		if entry.cancel != nil {
			entry.cancel()
		}
	case TimerTypeCron:
		if t.cronSched != nil {
			t.cronSched.Remove(entry.cronID)
		}
	case TimerTypeTimeout:
		if entry.timer != nil {
			entry.timer.Stop()
		}
	}
}

// safeCall 은 핸들러를 안전하게 호출한다. 패닉이 발생해도 복구하여 에이전트에 영향을 주지 않는다.
// 호출자가 wg.Add/Done을 관리해야 한다.
func (t *TimerAgent) safeCall(handler TimerHandler, trigger TimerTrigger) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "timer-agent: 핸들러 패닉 복구: %v\n", r)
		}
	}()
	handler(trigger)
}
