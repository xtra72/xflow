---
id: SPEC-LIFE-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-LIFE-001
---

# SPEC-LIFE-001 수락 기준

## Module 1: Core State - 상태 타입 및 전이 규칙

### AC-LIFE-001-01: State 타입 및 상수

```gherkin
Given State 타입이 정의되어 있을 때
Then 7개의 상수(StateCreated, StateInitializing, StateRunning, StatePaused, StateStopping, StateStopped, StateError)가 존재해야 한다
And 각 상수는 고유한 string 값을 가져야 한다
```

### AC-LIFE-001-02: State.String()

```gherkin
Given 유효한 State 값이 주어졌을 때
When String() 메서드를 호출하면
Then 사람이 읽을 수 있는 상태 이름을 반환해야 한다

Given StateRunning 값이 주어졌을 때
When String() 메서드를 호출하면
Then "running" 문자열을 반환해야 한다
```

### AC-LIFE-001-03: State.IsValid()

```gherkin
Given 유효한 State 상수(StateCreated 등)가 주어졌을 때
When IsValid() 메서드를 호출하면
Then true를 반환해야 한다

Given 임의의 문자열 State("unknown")가 주어졌을 때
When IsValid() 메서드를 호출하면
Then false를 반환해야 한다
```

### AC-LIFE-001-04: ValidTransitions 맵

```gherkin
Given ValidTransitions 맵이 정의되어 있을 때
Then StateCreated에서 StateInitializing으로의 전이가 유효해야 한다
And StateRunning에서 StatePaused로의 전이가 유효해야 한다
And StateRunning에서 StateStopping으로의 전이가 유효해야 한다
And StateRunning에서 StateError로의 전이가 유효해야 한다
And StatePaused에서 StateRunning으로의 전이가 유효해야 한다
And StateError에서 StateStopping으로의 전이가 유효해야 한다
And StateError에서 StateCreated로의 전이가 유효해야 한다
And StateStopped에서 StateCreated로의 전이가 유효해야 한다
```

### AC-LIFE-001-05: IsValidTransition()

```gherkin
Given from=StateCreated, to=StateInitializing 가 주어졌을 때
When IsValidTransition(from, to)를 호출하면
Then true를 반환해야 한다

Given from=StateCreated, to=StateRunning 가 주어졌을 때
When IsValidTransition(from, to)를 호출하면
Then false를 반환해야 한다 (직접 전이 불가)
```

### AC-LIFE-001-06: ParseState()

```gherkin
Given 유효한 문자열 "running"이 주어졌을 때
When ParseState("running")를 호출하면
Then StateRunning과 nil error를 반환해야 한다

Given 유효하지 않은 문자열 "invalid"가 주어졌을 때
When ParseState("invalid")를 호출하면
Then ErrInvalidState 에러를 반환해야 한다
```

### AC-LIFE-001-07: 유효하지 않은 전이 거부

```gherkin
Given BaseLifecycle이 StateCreated 상태일 때
When TransitionTo(StateRunning)를 호출하면 (Created -> Running 직접 전이 불가)
Then ErrInvalidStateTransition 에러를 반환해야 한다
And 상태는 StateCreated로 유지되어야 한다
```

---

## Module 2: Lifecycle Interface

### AC-LIFE-001-08: Lifecycle 인터페이스 메서드 시그니처

```gherkin
Given Lifecycle 인터페이스가 정의되어 있을 때
Then Init(ctx context.Context) error 메서드가 존재해야 한다
And Start(ctx context.Context) error 메서드가 존재해야 한다
And Pause(ctx context.Context) error 메서드가 존재해야 한다
And Resume(ctx context.Context) error 메서드가 존재해야 한다
And Stop(ctx context.Context) error 메서드가 존재해야 한다
And State() State 메서드가 존재해야 한다
```

### AC-LIFE-001-09: Init 호출 시 상태 전이

```gherkin
Given BaseLifecycle을 임베딩한 컴포넌트가 StateCreated 상태일 때
When Init(ctx)를 호출하면
Then 상태가 StateInitializing을 거쳐 StateRunning으로 전이해야 한다

Given BaseLifecycle을 임베딩한 컴포넌트가 StateRunning 상태일 때
When Init(ctx)를 호출하면
Then ErrAlreadyInitialized 에러를 반환해야 한다
```

### AC-LIFE-001-10: Stop 호출 시 Graceful Shutdown

```gherkin
Given 컴포넌트가 StateRunning 상태일 때
When Stop(ctx)를 호출하면
Then 상태가 StateStopping을 거쳐 StateStopped로 전이해야 한다

Given 컴포넌트가 StatePaused 상태일 때
When Stop(ctx)를 호출하면
Then 상태가 StateStopping을 거쳐 StateStopped로 전이해야 한다
```

### AC-LIFE-001-11: Pause/Resume 전이

```gherkin
Given 컴포넌트가 StateRunning 상태일 때
When Pause(ctx)를 호출하면
Then 상태가 StatePaused로 전이해야 한다

Given 컴포넌트가 StatePaused 상태일 때
When Resume(ctx)를 호출하면
Then 상태가 StateRunning으로 전이해야 한다
```

### AC-LIFE-001-12: 잘못된 상태에서의 호출 거부

```gherkin
Given 컴포넌트가 StatePaused 상태일 때
When Pause(ctx)를 재호출하면
Then ErrNotRunning 에러를 반환해야 한다
And 상태는 StatePaused로 유지되어야 한다

Given 컴포넌트가 StateRunning 상태일 때
When Resume(ctx)를 호출하면
Then ErrNotPaused 에러를 반환해야 한다
```

---

## Module 3: Configurable Interface

### AC-LIFE-001-13: Configurable 인터페이스 시그니처

```gherkin
Given Configurable 인터페이스가 정의되어 있을 때
Then Configure(ctx context.Context, cfg map[string]any) error 메서드가 존재해야 한다
And GetConfig() map[string]any 메서드가 존재해야 한다
```

### AC-LIFE-001-14: Running/Paused 상태에서만 Configure 허용

```gherkin
Given 컴포넌트가 StateRunning 상태일 때
When Configure(ctx, cfg)를 호출하면
Then 설정 변경이 성공해야 한다

Given 컴포넌트가 StatePaused 상태일 때
When Configure(ctx, cfg)를 호출하면
Then 설정 변경이 성공해야 한다

Given 컴포넌트가 StateCreated 상태일 때
When Configure(ctx, cfg)를 호출하면
Then ErrInvalidStateForConfigure 에러를 반환해야 한다
```

### AC-LIFE-001-15: Configure 실패 시 롤백

```gherkin
Given 컴포넌트가 {"key": "old_value"} 설정으로 Running 상태일 때
When 유효하지 않은 설정으로 Configure(ctx, cfg)를 호출하여 실패하면
Then GetConfig()가 {"key": "old_value"}를 반환해야 한다 (변경되지 않음)
```

### AC-LIFE-001-16: GetConfig 방어적 복사

```gherkin
Given 컴포넌트가 설정을 보유하고 있을 때
When GetConfig()를 호출하여 반환된 맵을 수정하면
Then 다시 GetConfig()를 호출했을 때 원래 값이 유지되어야 한다
```

---

## Module 4: BaseLifecycle

### AC-LIFE-001-17: NewBaseLifecycle 생성자

```gherkin
Given NewBaseLifecycle()를 호출할 때
Then 초기 상태가 StateCreated인 BaseLifecycle 인스턴스를 반환해야 한다

Given NewBaseLifecycle(WithName("agent.mqtt"))를 호출할 때
Then ComponentName()이 "agent.mqtt"를 반환해야 한다
```

### AC-LIFE-001-18: TransitionTo 유효 전이

```gherkin
Given BaseLifecycle이 StateCreated 상태일 때
When TransitionTo(StateInitializing)를 호출하면
Then nil error를 반환해야 한다
And CurrentState()가 StateInitializing을 반환해야 한다
```

### AC-LIFE-001-19: TransitionTo 무효 전이

```gherkin
Given BaseLifecycle이 StateCreated 상태일 때
When TransitionTo(StateStopped)를 호출하면
Then ErrInvalidStateTransition 에러를 반환해야 한다
And CurrentState()가 StateCreated를 반환해야 한다 (변경 없음)
```

### AC-LIFE-001-20: OnStateChange 콜백 호출

```gherkin
Given BaseLifecycle에 StateChangeCallback이 등록되어 있을 때
When TransitionTo()가 성공적으로 상태를 변경하면
Then 등록된 콜백이 StateChangeEvent와 함께 호출되어야 한다
And StateChangeEvent.From은 이전 상태여야 한다
And StateChangeEvent.To는 새 상태여야 한다
And StateChangeEvent.Timestamp는 전이 시점이어야 한다
```

### AC-LIFE-001-21: OnStateChange 등록 해제

```gherkin
Given StateChangeCallback이 등록되어 unsub 함수를 받았을 때
When unsub()를 호출한 후 상태를 변경하면
Then 해제된 콜백은 호출되지 않아야 한다
```

### AC-LIFE-001-22: 동시성 안전

```gherkin
Given BaseLifecycle 인스턴스가 하나 주어졌을 때
When 10개의 goroutine이 동시에 TransitionTo()를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 최종 상태는 유효한 상태여야 한다
```

### AC-LIFE-001-23: 콜백 등록 순서 보장

```gherkin
Given 3개의 콜백(cb1, cb2, cb3)이 순서대로 등록되어 있을 때
When 상태 전이가 발생하면
Then cb1, cb2, cb3 순서대로 호출되어야 한다
```

---

## Module 5: State Change Event

### AC-LIFE-001-24: StateChangeEvent 구조체

```gherkin
Given StateChangeEvent가 정의되어 있을 때
Then Component, From, To, Timestamp, Error 필드가 존재해야 한다
And Component 필드는 string 타입이어야 한다
And From, To 필드는 State 타입이어야 한다
And Timestamp 필드는 time.Time 타입이어야 한다
And Error 필드는 error 타입이어야 한다 (nil 허용)
```

### AC-LIFE-001-25: 콜백 panic recovery

```gherkin
Given panic을 발생시키는 콜백(cb1)과 정상 콜백(cb2)이 순서대로 등록되어 있을 때
When 상태 전이가 발생하면
Then cb1의 panic이 recover되어야 한다
And cb2는 정상적으로 호출되어야 한다
And TransitionTo()는 정상 반환해야 한다 (panic 전파 없음)
```

---

## Module 6: Health Check & Recovery

### AC-LIFE-001-26: HealthChecker 인터페이스

```gherkin
Given HealthChecker 인터페이스가 정의되어 있을 때
Then HealthCheck(ctx context.Context) HealthStatus 메서드가 존재해야 한다
```

### AC-LIFE-001-27: HealthStatus 구조체

```gherkin
Given HealthStatus가 정의되어 있을 때
Then Healthy, Message, LastChecked, Details 필드가 존재해야 한다
And Healthy는 bool 타입이어야 한다
And Details는 map[string]any 타입이어야 한다
```

### AC-LIFE-001-28: RecoveryPolicy 구조체

```gherkin
Given RecoveryPolicy가 정의되어 있을 때
Then MaxRetries, InitialBackoff, MaxBackoff, BackoffMultiplier, OnMaxRetriesExceeded 필드가 존재해야 한다
```

### AC-LIFE-001-29: DefaultRecoveryPolicy()

```gherkin
Given DefaultRecoveryPolicy()를 호출할 때
Then MaxRetries는 3이어야 한다
And InitialBackoff는 1초여야 한다
And MaxBackoff는 30초여야 한다
And BackoffMultiplier는 2.0이어야 한다
And OnMaxRetriesExceeded는 RecoveryStop이어야 한다
```

### AC-LIFE-001-30: RecoveryAction 상수

```gherkin
Given RecoveryAction 타입이 정의되어 있을 때
Then RecoveryStop과 RecoveryKeepError 상수가 존재해야 한다
```

---

## Module 7: Error Types

### AC-LIFE-001-31: Sentinel 에러 정의

```gherkin
Given errors 패키지가 임포트되어 있을 때
Then ErrInvalidState가 정의되어 있어야 한다
And ErrInvalidStateTransition이 정의되어 있어야 한다
And ErrInvalidStateForConfigure가 정의되어 있어야 한다
And ErrAlreadyInitialized가 정의되어 있어야 한다
And ErrNotRunning이 정의되어 있어야 한다
And ErrNotPaused가 정의되어 있어야 한다
```

### AC-LIFE-001-32: errors.Is() 호환성

```gherkin
Given ErrInvalidStateTransition 에러가 주어졌을 때
When fmt.Errorf("wrap: %w", ErrInvalidStateTransition)로 래핑한 후
Then errors.Is(wrappedErr, ErrInvalidStateTransition)이 true를 반환해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-LIFE-001-01 ~ 32) 테스트 통과
- [ ] `go test ./pkg/lifecycle/...` 전체 통과
- [ ] `go test -race ./pkg/lifecycle/...` 경쟁 상태 없음
- [ ] `go vet ./pkg/lifecycle/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] 외부 의존성 없음 (표준 라이브러리만 사용) 확인
- [ ] `pkg/flow/`와 상호 import 없음 확인
- [ ] `internal/` 패키지와의 의존 없음 확인

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
