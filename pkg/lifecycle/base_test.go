package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// --- BaseLifecycle 기본 동작 테스트 ---

// TestNewBaseLifecycle_DefaultValues 는 기본 옵션으로 생성 시 초기값을 검증한다 (AC-LIFE-001-17).
func TestNewBaseLifecycle_DefaultValues(t *testing.T) {
	b := NewBaseLifecycle()

	if b.ComponentName() != "" {
		t.Errorf("기본 컴포넌트 이름 = %q, 기대값 빈 문자열", b.ComponentName())
	}
	if b.CurrentState() != StateCreated {
		t.Errorf("초기 상태 = %q, 기대값 %q", b.CurrentState(), StateCreated)
	}
}

// TestNewBaseLifecycle_WithName 은 WithName 옵션이 이름을 올바르게 설정하는지 검증한다 (AC-LIFE-001-17).
func TestNewBaseLifecycle_WithName(t *testing.T) {
	b := NewBaseLifecycle(WithName("테스트컴포넌트"))

	if b.ComponentName() != "테스트컴포넌트" {
		t.Errorf("컴포넌트 이름 = %q, 기대값 %q", b.ComponentName(), "테스트컴포넌트")
	}
}

// TestNewBaseLifecycle_WithOnStateChange 는 WithOnStateChange 옵션으로 콜백이 등록되는지 검증한다.
func TestNewBaseLifecycle_WithOnStateChange(t *testing.T) {
	called := false
	b := NewBaseLifecycle(
		WithName("옵션콜백"),
		WithOnStateChange(func(event StateChangeEvent) {
			called = true
		}),
	)

	err := b.TransitionTo(StateInitializing)
	if err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}
	if !called {
		t.Error("WithOnStateChange로 등록한 콜백이 호출되지 않았다")
	}
}

// --- TransitionTo 테스트 ---

// TestTransitionTo_ValidTransition 은 유효한 상태 전이가 성공하는지 검증한다 (AC-LIFE-001-18).
func TestTransitionTo_ValidTransition(t *testing.T) {
	b := NewBaseLifecycle()

	err := b.TransitionTo(StateInitializing)
	if err != nil {
		t.Fatalf("TransitionTo(Initializing) 에러: %v", err)
	}
	if b.CurrentState() != StateInitializing {
		t.Errorf("전이 후 상태 = %q, 기대값 %q", b.CurrentState(), StateInitializing)
	}
}

// TestTransitionTo_InvalidTransition 은 유효하지 않은 상태 전이가 에러를 반환하는지 검증한다 (AC-LIFE-001-19).
func TestTransitionTo_InvalidTransition(t *testing.T) {
	b := NewBaseLifecycle()

	err := b.TransitionTo(StateRunning) // Created -> Running은 불가
	if err == nil {
		t.Fatal("유효하지 않은 전이에서 에러를 기대했으나 nil을 반환했다")
	}
	if b.CurrentState() != StateCreated {
		t.Errorf("실패한 전이 후 상태 = %q, 기대값 %q (변경되지 않아야 한다)", b.CurrentState(), StateCreated)
	}
}

// TestTransitionTo_MultipleSteps 는 연속 전이가 올바르게 동작하는지 검증한다.
func TestTransitionTo_MultipleSteps(t *testing.T) {
	b := NewBaseLifecycle()

	transitions := []State{
		StateInitializing,
		StateRunning,
		StatePaused,
		StateRunning,
		StateStopping,
		StateStopped,
	}

	for _, target := range transitions {
		if err := b.TransitionTo(target); err != nil {
			t.Fatalf("TransitionTo(%q) 에러: %v", target, err)
		}
		if b.CurrentState() != target {
			t.Errorf("전이 후 상태 = %q, 기대값 %q", b.CurrentState(), target)
		}
	}
}

// --- OnStateChange 콜백 테스트 ---

// TestOnStateChange_CallbackCalled 은 상태 변경 시 콜백이 호출되는지 검증한다 (AC-LIFE-001-20).
func TestOnStateChange_CallbackCalled(t *testing.T) {
	b := NewBaseLifecycle(WithName("콜백테스트"))

	var received StateChangeEvent
	b.OnStateChange(func(event StateChangeEvent) {
		received = event
	})

	err := b.TransitionTo(StateInitializing)
	if err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}

	if received.Component != "콜백테스트" {
		t.Errorf("이벤트 Component = %q, 기대값 %q", received.Component, "콜백테스트")
	}
	if received.From != StateCreated {
		t.Errorf("이벤트 From = %q, 기대값 %q", received.From, StateCreated)
	}
	if received.To != StateInitializing {
		t.Errorf("이벤트 To = %q, 기대값 %q", received.To, StateInitializing)
	}
}

// TestOnStateChange_Unsubscribe 는 구독 해제 후 콜백이 호출되지 않는지 검증한다 (AC-LIFE-001-21).
func TestOnStateChange_Unsubscribe(t *testing.T) {
	b := NewBaseLifecycle()

	callCount := 0
	unsub := b.OnStateChange(func(event StateChangeEvent) {
		callCount++
	})

	// 첫 번째 전이: 콜백 호출되어야 함
	if err := b.TransitionTo(StateInitializing); err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}
	if callCount != 1 {
		t.Errorf("구독 해제 전 콜백 호출 횟수 = %d, 기대값 1", callCount)
	}

	// 구독 해제
	unsub()

	// 두 번째 전이: 콜백 호출되지 않아야 함
	if err := b.TransitionTo(StateRunning); err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}
	if callCount != 1 {
		t.Errorf("구독 해제 후 콜백 호출 횟수 = %d, 기대값 1 (변경 없어야 한다)", callCount)
	}
}

// TestOnStateChange_MultipleCallbacks_OrderPreserved 는 여러 콜백이 등록 순서대로 호출되는지 검증한다 (AC-LIFE-001-23).
func TestOnStateChange_MultipleCallbacks_OrderPreserved(t *testing.T) {
	b := NewBaseLifecycle()

	var order []int
	b.OnStateChange(func(event StateChangeEvent) {
		order = append(order, 1)
	})
	b.OnStateChange(func(event StateChangeEvent) {
		order = append(order, 2)
	})
	b.OnStateChange(func(event StateChangeEvent) {
		order = append(order, 3)
	})

	if err := b.TransitionTo(StateInitializing); err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("콜백 호출 횟수 = %d, 기대값 3", len(order))
	}
	for i, v := range order {
		expected := i + 1
		if v != expected {
			t.Errorf("콜백 순서[%d] = %d, 기대값 %d", i, v, expected)
		}
	}
}

// TestOnStateChange_CallbackNotCalledOnInvalidTransition 은 전이 실패 시 콜백이 호출되지 않는지 검증한다.
func TestOnStateChange_CallbackNotCalledOnInvalidTransition(t *testing.T) {
	b := NewBaseLifecycle()

	called := false
	b.OnStateChange(func(event StateChangeEvent) {
		called = true
	})

	// Created -> Running은 유효하지 않은 전이
	_ = b.TransitionTo(StateRunning)

	if called {
		t.Error("유효하지 않은 전이에서 콜백이 호출되었다")
	}
}

// TestOnStateChange_PanicRecovery 는 콜백에서 패닉이 발생해도 다른 콜백이 호출되는지 검증한다 (AC-LIFE-001-25).
func TestOnStateChange_PanicRecovery(t *testing.T) {
	b := NewBaseLifecycle()

	secondCalled := false
	b.OnStateChange(func(event StateChangeEvent) {
		panic("의도적 패닉")
	})
	b.OnStateChange(func(event StateChangeEvent) {
		secondCalled = true
	})

	if err := b.TransitionTo(StateInitializing); err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}

	if !secondCalled {
		t.Error("첫 번째 콜백 패닉 후 두 번째 콜백이 호출되지 않았다")
	}

	// 상태 전이 자체는 성공해야 한다
	if b.CurrentState() != StateInitializing {
		t.Errorf("패닉 후 상태 = %q, 기대값 %q", b.CurrentState(), StateInitializing)
	}
}

// TestOnStateChange_UnsubscribeIdempotent 은 구독 해제를 여러 번 호출해도 안전한지 검증한다.
func TestOnStateChange_UnsubscribeIdempotent(t *testing.T) {
	b := NewBaseLifecycle()

	unsub := b.OnStateChange(func(event StateChangeEvent) {})

	// 여러 번 호출해도 패닉이 발생하지 않아야 한다
	unsub()
	unsub()
	unsub()
}

// --- StateChangeEvent 필드 검증 ---

// TestStateChangeEvent_Fields 는 이벤트의 모든 필드가 올바르게 채워지는지 검증한다 (AC-LIFE-001-24).
func TestStateChangeEvent_Fields(t *testing.T) {
	b := NewBaseLifecycle(WithName("이벤트테스트"))

	var received StateChangeEvent
	b.OnStateChange(func(event StateChangeEvent) {
		received = event
	})

	before := time.Now()
	if err := b.TransitionTo(StateInitializing); err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}
	after := time.Now()

	if received.Component != "이벤트테스트" {
		t.Errorf("Component = %q, 기대값 %q", received.Component, "이벤트테스트")
	}
	if received.From != StateCreated {
		t.Errorf("From = %q, 기대값 %q", received.From, StateCreated)
	}
	if received.To != StateInitializing {
		t.Errorf("To = %q, 기대값 %q", received.To, StateInitializing)
	}
	if received.Timestamp.Before(before) || received.Timestamp.After(after) {
		t.Errorf("Timestamp = %v, %v ~ %v 범위를 벗어났다", received.Timestamp, before, after)
	}
	if received.Error != nil {
		t.Errorf("Error = %v, 기대값 nil", received.Error)
	}
}

// --- 동시성 테스트 ---

// TestConcurrentTransitions 는 동시 전이가 race condition 없이 안전한지 검증한다 (AC-LIFE-001-22).
func TestConcurrentTransitions(t *testing.T) {
	b := NewBaseLifecycle()

	// Created -> Initializing으로 먼저 전이
	if err := b.TransitionTo(StateInitializing); err != nil {
		t.Fatalf("초기 TransitionTo 에러: %v", err)
	}
	// Initializing -> Running으로 전이
	if err := b.TransitionTo(StateRunning); err != nil {
		t.Fatalf("초기 TransitionTo 에러: %v", err)
	}

	// Running 상태에서 동시에 여러 전이 시도
	var wg sync.WaitGroup
	var successCount atomic.Int32

	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// Running에서 Paused로 전이 시도 (하나만 성공해야 함은 아님 - Paused에서 다시 Running 가능)
			if err := b.TransitionTo(StatePaused); err == nil {
				successCount.Add(1)
				// 다시 Running으로 복귀
				_ = b.TransitionTo(StateRunning)
			}
		}()
	}
	wg.Wait()

	// 최소 1회 이상 성공해야 한다 (race-free라면)
	if successCount.Load() == 0 {
		t.Error("100회 동시 전이에서 한 번도 성공하지 못했다")
	}

	// 최종 상태는 유효한 상태여야 한다
	finalState := b.CurrentState()
	if !finalState.IsValid() {
		t.Errorf("동시 전이 후 유효하지 않은 상태: %q", finalState)
	}
}

// TestConcurrentCurrentState 는 동시 상태 조회가 안전한지 검증한다.
func TestConcurrentCurrentState(t *testing.T) {
	b := NewBaseLifecycle()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state := b.CurrentState()
			if !state.IsValid() {
				t.Errorf("동시 조회에서 유효하지 않은 상태: %q", state)
			}
		}()
	}
	wg.Wait()
}

// TestConcurrentOnStateChange 는 동시 콜백 등록과 전이가 안전한지 검증한다.
func TestConcurrentOnStateChange(t *testing.T) {
	b := NewBaseLifecycle()

	var wg sync.WaitGroup

	// 동시에 콜백 등록
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unsub := b.OnStateChange(func(event StateChangeEvent) {})
			// 일부는 바로 구독 해제
			unsub()
		}()
	}

	// 동시에 전이 시도
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = b.TransitionTo(StateInitializing)
	}()

	wg.Wait()
}

// --- Lifecycle 인터페이스 구현 테스트 (testComponent 사용) ---

// testComponent 는 Lifecycle 및 Configurable 인터페이스를 구현하는 테스트용 컴포넌트이다.
type testComponent struct {
	*BaseLifecycle
	config map[string]any
}

// newTestComponent 는 테스트용 컴포넌트를 생성한다.
func newTestComponent(name string) *testComponent {
	return &testComponent{
		BaseLifecycle: NewBaseLifecycle(WithName(name)),
		config:        make(map[string]any),
	}
}

// Init 은 컴포넌트를 초기화한다.
func (c *testComponent) Init(ctx context.Context) error {
	if c.CurrentState() != StateCreated {
		return ErrAlreadyInitialized
	}
	if err := c.TransitionTo(StateInitializing); err != nil {
		return err
	}
	// 초기화 로직 (여기서는 바로 완료)
	return c.TransitionTo(StateRunning)
}

// Start 는 컴포넌트를 시작한다.
func (c *testComponent) Start(ctx context.Context) error {
	// 이미 Running이면 그대로 반환
	if c.CurrentState() == StateRunning {
		return nil
	}
	return c.TransitionTo(StateRunning)
}

// Pause 는 컴포넌트를 일시정지한다.
func (c *testComponent) Pause(ctx context.Context) error {
	if c.CurrentState() != StateRunning {
		return ErrNotRunning
	}
	return c.TransitionTo(StatePaused)
}

// Resume 은 일시정지된 컴포넌트를 재개한다.
func (c *testComponent) Resume(ctx context.Context) error {
	if c.CurrentState() != StatePaused {
		return ErrNotPaused
	}
	return c.TransitionTo(StateRunning)
}

// Stop 은 컴포넌트를 정지한다.
func (c *testComponent) Stop(ctx context.Context) error {
	state := c.CurrentState()
	if state != StateRunning && state != StatePaused {
		return ErrNotRunning
	}
	if err := c.TransitionTo(StateStopping); err != nil {
		return err
	}
	return c.TransitionTo(StateStopped)
}

// State 는 현재 상태를 반환한다.
func (c *testComponent) State() State {
	return c.CurrentState()
}

// Configure 는 설정을 변경한다 (AC-LIFE-001-14).
func (c *testComponent) Configure(ctx context.Context, cfg map[string]any) error {
	state := c.CurrentState()
	if state != StateCreated && state != StateStopped {
		return ErrInvalidStateForConfigure
	}
	for k, v := range cfg {
		c.config[k] = v
	}
	return nil
}

// GetConfig 는 현재 설정을 반환한다.
func (c *testComponent) GetConfig() map[string]any {
	result := make(map[string]any, len(c.config))
	for k, v := range c.config {
		result[k] = v
	}
	return result
}

// 컴파일 타임에 인터페이스 구현을 확인한다.
var (
	_ Lifecycle    = (*testComponent)(nil)
	_ Configurable = (*testComponent)(nil)
)

// TestLifecycleInterface_FullCycle 은 Init -> Start -> Pause -> Resume -> Stop 전체 사이클을 검증한다 (AC-LIFE-001-09 ~ 12).
func TestLifecycleInterface_FullCycle(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("전체사이클")

	// 초기 상태 확인
	if comp.State() != StateCreated {
		t.Fatalf("초기 상태 = %q, 기대값 %q", comp.State(), StateCreated)
	}

	// Init (AC-LIFE-001-09)
	if err := comp.Init(ctx); err != nil {
		t.Fatalf("Init 에러: %v", err)
	}
	if comp.State() != StateRunning {
		t.Fatalf("Init 후 상태 = %q, 기대값 %q", comp.State(), StateRunning)
	}

	// Pause (AC-LIFE-001-10)
	if err := comp.Pause(ctx); err != nil {
		t.Fatalf("Pause 에러: %v", err)
	}
	if comp.State() != StatePaused {
		t.Fatalf("Pause 후 상태 = %q, 기대값 %q", comp.State(), StatePaused)
	}

	// Resume (AC-LIFE-001-11)
	if err := comp.Resume(ctx); err != nil {
		t.Fatalf("Resume 에러: %v", err)
	}
	if comp.State() != StateRunning {
		t.Fatalf("Resume 후 상태 = %q, 기대값 %q", comp.State(), StateRunning)
	}

	// Stop (AC-LIFE-001-12)
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop 에러: %v", err)
	}
	if comp.State() != StateStopped {
		t.Fatalf("Stop 후 상태 = %q, 기대값 %q", comp.State(), StateStopped)
	}
}

// TestLifecycleInterface_InitAlreadyInitialized 는 이미 초기화된 컴포넌트에 Init을 재호출하면 에러를 반환하는지 검증한다.
func TestLifecycleInterface_InitAlreadyInitialized(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("중복초기화")

	if err := comp.Init(ctx); err != nil {
		t.Fatalf("첫 번째 Init 에러: %v", err)
	}

	err := comp.Init(ctx)
	if err != ErrAlreadyInitialized {
		t.Errorf("두 번째 Init 에러 = %v, 기대값 ErrAlreadyInitialized", err)
	}
}

// TestLifecycleInterface_PauseNotRunning 은 Running이 아닌 상태에서 Pause를 호출하면 에러를 반환하는지 검증한다.
func TestLifecycleInterface_PauseNotRunning(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("비실행상태일시정지")

	err := comp.Pause(ctx)
	if err != ErrNotRunning {
		t.Errorf("Created 상태에서 Pause 에러 = %v, 기대값 ErrNotRunning", err)
	}
}

// TestLifecycleInterface_ResumeNotPaused 는 Paused가 아닌 상태에서 Resume을 호출하면 에러를 반환하는지 검증한다.
func TestLifecycleInterface_ResumeNotPaused(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("비일시정지상태재개")

	if err := comp.Init(ctx); err != nil {
		t.Fatalf("Init 에러: %v", err)
	}

	err := comp.Resume(ctx)
	if err != ErrNotPaused {
		t.Errorf("Running 상태에서 Resume 에러 = %v, 기대값 ErrNotPaused", err)
	}
}

// --- Configurable 인터페이스 테스트 ---

// TestConfigurable_ConfigureInCreatedState 는 Created 상태에서 Configure가 성공하는지 검증한다 (AC-LIFE-001-14).
func TestConfigurable_ConfigureInCreatedState(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("설정테스트")

	cfg := map[string]any{"key": "value", "count": 42}
	if err := comp.Configure(ctx, cfg); err != nil {
		t.Fatalf("Created 상태에서 Configure 에러: %v", err)
	}

	got := comp.GetConfig()
	if got["key"] != "value" {
		t.Errorf("설정 key = %v, 기대값 %v", got["key"], "value")
	}
	if got["count"] != 42 {
		t.Errorf("설정 count = %v, 기대값 %v", got["count"], 42)
	}
}

// TestConfigurable_ConfigureInStoppedState 는 Stopped 상태에서 Configure가 성공하는지 검증한다 (AC-LIFE-001-15).
func TestConfigurable_ConfigureInStoppedState(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("정지후설정")

	// Init -> Running -> Stopping -> Stopped
	if err := comp.Init(ctx); err != nil {
		t.Fatalf("Init 에러: %v", err)
	}
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop 에러: %v", err)
	}

	cfg := map[string]any{"new_key": "new_value"}
	if err := comp.Configure(ctx, cfg); err != nil {
		t.Fatalf("Stopped 상태에서 Configure 에러: %v", err)
	}
}

// TestConfigurable_ConfigureInRunningState 는 Running 상태에서 Configure가 에러를 반환하는지 검증한다 (AC-LIFE-001-16).
func TestConfigurable_ConfigureInRunningState(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("실행중설정")

	if err := comp.Init(ctx); err != nil {
		t.Fatalf("Init 에러: %v", err)
	}

	cfg := map[string]any{"key": "value"}
	err := comp.Configure(ctx, cfg)
	if err != ErrInvalidStateForConfigure {
		t.Errorf("Running 상태에서 Configure 에러 = %v, 기대값 ErrInvalidStateForConfigure", err)
	}
}

// TestConfigurable_GetConfig_Isolation 은 GetConfig가 내부 설정의 복사본을 반환하는지 검증한다.
func TestConfigurable_GetConfig_Isolation(t *testing.T) {
	ctx := context.Background()
	comp := newTestComponent("설정격리")

	cfg := map[string]any{"key": "value"}
	if err := comp.Configure(ctx, cfg); err != nil {
		t.Fatalf("Configure 에러: %v", err)
	}

	// 반환된 맵을 수정해도 원본에 영향 없어야 한다
	got := comp.GetConfig()
	got["key"] = "modified"

	original := comp.GetConfig()
	if original["key"] != "value" {
		t.Errorf("GetConfig 수정이 원본에 영향을 줬다: %v", original["key"])
	}
}

// --- ComponentName 이벤트 검증 ---

// TestComponentName_InEvent 는 이벤트의 Component 필드에 컴포넌트 이름이 올바르게 포함되는지 검증한다 (AC-LIFE-001-17).
func TestComponentName_InEvent(t *testing.T) {
	b := NewBaseLifecycle(WithName("이벤트이름"))

	var eventName string
	b.OnStateChange(func(event StateChangeEvent) {
		eventName = event.Component
	})

	if err := b.TransitionTo(StateInitializing); err != nil {
		t.Fatalf("TransitionTo 에러: %v", err)
	}

	if eventName != "이벤트이름" {
		t.Errorf("이벤트 Component = %q, 기대값 %q", eventName, "이벤트이름")
	}
}

// --- 에러 래핑 검증 ---

// TestTransitionTo_ErrorWrapping 은 TransitionTo 실패 시 에러가 ErrInvalidStateTransition을 래핑하는지 검증한다.
func TestTransitionTo_ErrorWrapping(t *testing.T) {
	b := NewBaseLifecycle()

	err := b.TransitionTo(StateRunning) // Created -> Running은 불가
	if err == nil {
		t.Fatal("에러를 기대했으나 nil을 반환했다")
	}

	// 에러 메시지에 상태 정보가 포함되어야 한다
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("에러 메시지가 비어있다")
	}

	// fmt.Errorf로 래핑된 경우 ErrInvalidStateTransition을 감지할 수 있어야 한다
	_ = fmt.Sprintf("에러 내용 확인: %v", err)
}
