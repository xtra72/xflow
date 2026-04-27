---
id: SPEC-TIMER-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-TIMER-001
---

# SPEC-TIMER-001 수락 기준

## Module 1: Timer Interface - 타이머 인터페이스

### AC-TIMER-001-01: Timer 인터페이스 메서드 시그니처

```gherkin
Given Timer 인터페이스가 정의되어 있을 때
Then SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error) 메서드가 존재해야 한다
And SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error) 메서드가 존재해야 한다
And SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error) 메서드가 존재해야 한다
And Cancel(id TimerID) error 메서드가 존재해야 한다
And List() []TimerInfo 메서드가 존재해야 한다
```

### AC-TIMER-001-02: TimerHandler 콜백 타입

```gherkin
Given TimerHandler 타입이 정의되어 있을 때
Then func(trigger TimerTrigger) 시그니처를 가져야 한다
```

### AC-TIMER-001-03: TimerTrigger 구조체

```gherkin
Given TimerTrigger가 정의되어 있을 때
Then TimerID, TriggerAt, TickCount, ScheduleID 필드가 존재해야 한다
And TimerID는 TimerID 타입이어야 한다
And TriggerAt은 time.Time 타입이어야 한다
And TickCount은 int64 타입이어야 한다
And ScheduleID는 string 타입이어야 한다
```

### AC-TIMER-001-04: TimerInfo 구조체

```gherkin
Given TimerInfo가 정의되어 있을 때
Then ID, Type, Expression, NextFire, LastFired, FireCount, Active 필드가 존재해야 한다
```

### AC-TIMER-001-05: TimerType 상수

```gherkin
Given TimerType 상수가 정의되어 있을 때
Then TimerTypeInterval = "interval" 이어야 한다
And TimerTypeCron = "cron" 이어야 한다
And TimerTypeTimeout = "timeout" 이어야 한다
```

### AC-TIMER-001-06: 빈 ID 등록 거부

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When SetInterval("", 1*time.Second, handler)를 호출하면
Then ErrTimerIDEmpty 에러를 반환해야 한다

When SetCron("", "*/5 * * * *", handler)를 호출하면
Then ErrTimerIDEmpty 에러를 반환해야 한다

When SetTimeout("", 1*time.Second, handler)를 호출하면
Then ErrTimerIDEmpty 에러를 반환해야 한다
```

### AC-TIMER-001-07: nil 핸들러 등록 거부

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When SetInterval("test", 1*time.Second, nil)를 호출하면
Then ErrNilHandler 에러를 반환해야 한다
```

### AC-TIMER-001-08: 중복 ID 등록 거부

```gherkin
Given "timer-1" ID로 Interval 타이머가 등록되어 있을 때
When SetInterval("timer-1", 2*time.Second, handler)를 호출하면
Then ErrDuplicateTimerID 에러를 반환해야 한다
```

---

## Module 2: Interval Timer - 인터벌 타이머

### AC-TIMER-001-09: SetInterval 주기적 실행

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When SetInterval("tick", 200*time.Millisecond, handler)를 호출하면
Then 200ms 간격으로 핸들러가 반복 호출되어야 한다
And 1초 후 약 5회 호출되어야 한다
```

### AC-TIMER-001-10: 최소 간격 미만 등록 거부

```gherkin
Given 기본 최소 간격(100ms)의 TimerAgent가 주어졌을 때
When SetInterval("fast", 50*time.Millisecond, handler)를 호출하면
Then ErrIntervalTooShort 에러를 반환해야 한다

When SetInterval("edge", 100*time.Millisecond, handler)를 호출하면
Then nil error를 반환해야 한다 (최소 간격은 허용)
```

### AC-TIMER-001-11: TickCount 추적

```gherkin
Given SetInterval("counter", 100*time.Millisecond, handler)가 등록되어 있을 때
When 핸들러가 3번 호출된 후
Then 마지막 TimerTrigger.TickCount가 3이어야 한다
```

### AC-TIMER-001-12: Interval Cancel

```gherkin
Given "interval-1" Interval 타이머가 등록되어 있을 때
When Cancel("interval-1")를 호출하면
Then nil error를 반환해야 한다
And 이후 핸들러가 호출되지 않아야 한다
And List()에 "interval-1"이 포함되지 않아야 한다
```

### AC-TIMER-001-13: 독립 goroutine 실행

```gherkin
Given 200ms 간격 타이머 A와 200ms 간격 타이머 B가 등록되어 있을 때
When 타이머 A의 핸들러가 500ms 블로킹되면
Then 타이머 B의 핸들러는 여전히 200ms 간격으로 호출되어야 한다
```

---

## Module 3: Cron Timer - 크론 타이머

### AC-TIMER-001-14: SetCron 5필드 표준 크론

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When SetCron("daily", "0 9 * * *", handler)를 호출하면
Then nil error를 반환해야 한다
And List()에 Type="cron", Expression="0 9 * * *"인 타이머가 포함되어야 한다
```

### AC-TIMER-001-15: 유효하지 않은 크론 표현식 거부

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When SetCron("bad", "invalid expression", handler)를 호출하면
Then ErrInvalidCronExpression 에러를 반환해야 한다
And List()에 "bad" 타이머가 포함되지 않아야 한다
```

### AC-TIMER-001-16: Cron Cancel

```gherkin
Given "cron-1" Cron 타이머가 등록되어 있을 때
When Cancel("cron-1")를 호출하면
Then nil error를 반환해야 한다
And cron 스케줄러에서 해당 Entry가 제거되어야 한다
And List()에 "cron-1"이 포함되지 않아야 한다
```

### AC-TIMER-001-17: ScheduleID 추적

```gherkin
Given SetCron("sched", "*/1 * * * *", handler)가 등록되어 있을 때
When 핸들러가 호출되면
Then TimerTrigger.ScheduleID가 빈 문자열이 아니어야 한다
```

---

## Module 4: Timeout Timer - 타임아웃 타이머

### AC-TIMER-001-18: SetTimeout 지연 실행

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When SetTimeout("delayed", 200*time.Millisecond, handler)를 호출하면
Then 약 200ms 후 핸들러가 정확히 1회 호출되어야 한다
And TimerTrigger.TickCount가 1이어야 한다
```

### AC-TIMER-001-19: Timeout 실행 후 자동 제거

```gherkin
Given SetTimeout("once", 100*time.Millisecond, handler)가 등록되어 있을 때
When 200ms 후 List()를 호출하면
Then "once" 타이머가 포함되지 않아야 한다 (자동 제거됨)
```

### AC-TIMER-001-20: Timeout Cancel

```gherkin
Given SetTimeout("cancel-me", 5*time.Second, handler)가 등록되어 있을 때
When 즉시 Cancel("cancel-me")를 호출하면
Then nil error를 반환해야 한다
And 5초 후에도 핸들러가 호출되지 않아야 한다
```

### AC-TIMER-001-21: 이미 실행된 Timeout Cancel

```gherkin
Given SetTimeout("fired", 100*time.Millisecond, handler)가 등록되어 있을 때
When 200ms 후 Cancel("fired")를 호출하면
Then ErrTimerNotFound 에러를 반환해야 한다 (이미 자동 제거됨)
```

### AC-TIMER-001-22: 음수/0 지연 거부

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When SetTimeout("neg", -1*time.Second, handler)를 호출하면
Then ErrInvalidDelay 에러를 반환해야 한다

When SetTimeout("zero", 0, handler)를 호출하면
Then ErrInvalidDelay 에러를 반환해야 한다
```

---

## Module 5: Timer Agent Lifecycle - 타이머 에이전트 생명주기

### AC-TIMER-001-23: NewTimerAgent 생성자

```gherkin
Given NewTimerAgent()를 호출할 때
Then State()가 lifecycle.StateCreated를 반환해야 한다

Given NewTimerAgent(WithMinInterval(200*time.Millisecond), WithMaxTimers(500))를 호출할 때
Then 생성된 TimerAgent의 설정이 반영되어야 한다
```

### AC-TIMER-001-24: Init -> Start 생명주기

```gherkin
Given StateCreated 상태의 TimerAgent가 주어졌을 때
When Init(ctx)를 호출하면
Then State()가 lifecycle.StateRunning을 반환해야 한다
And cron 스케줄러가 초기화되어야 한다
And 타이머 맵이 빈 상태로 생성되어야 한다
```

### AC-TIMER-001-25: Pause 시 등록 거부

```gherkin
Given StateRunning 상태의 TimerAgent가 주어졌을 때
When Pause(ctx)를 호출한 후 SetInterval("new", 1*time.Second, handler)를 시도하면
Then ErrTimerPaused 에러를 반환해야 한다

When Pause(ctx) 후 List()를 호출하면
Then 정상적으로 타이머 목록을 반환해야 한다 (읽기는 허용)

When Pause(ctx) 후 Cancel("existing")를 호출하면
Then 정상적으로 타이머를 취소해야 한다 (취소는 허용)
```

### AC-TIMER-001-26: Resume 시 등록 재개

```gherkin
Given StatePaused 상태의 TimerAgent가 주어졌을 때
When Resume(ctx)를 호출한 후 SetInterval("new", 1*time.Second, handler)를 시도하면
Then nil error를 반환해야 한다 (등록 허용)
```

### AC-TIMER-001-27: Stop 시 Graceful Shutdown

```gherkin
Given StateRunning 상태의 TimerAgent에 3개의 타이머가 등록되어 있을 때
When Stop(ctx)를 호출하면
Then State()가 lifecycle.StateStopped를 반환해야 한다
And 모든 활성 타이머가 취소되어야 한다
And List()가 빈 슬라이스를 반환해야 한다
And 이후 SetInterval 호출 시 ErrTimerClosed를 반환해야 한다
```

### AC-TIMER-001-28: Configure 런타임 설정 변경

```gherkin
Given StateRunning 상태의 TimerAgent가 주어졌을 때
When Configure(ctx, map[string]any{"min_interval": "200ms"})를 호출하면
Then 최소 인터벌 간격이 200ms로 변경되어야 한다
And GetConfig()가 변경된 설정을 반환해야 한다

Given StateCreated 상태의 TimerAgent가 주어졌을 때
When Configure(ctx, cfg)를 호출하면
Then ErrInvalidStateForConfigure 에러를 반환해야 한다
```

### AC-TIMER-001-29: HealthCheck

```gherkin
Given StateRunning 상태의 TimerAgent에 Interval 2개, Cron 1개, Timeout 1개가 등록되어 있을 때
When HealthCheck(ctx)를 호출하면
Then Healthy=true인 HealthStatus를 반환해야 한다
And Details에 ActiveTimers=4가 포함되어야 한다
And Details에 IntervalTimers=2가 포함되어야 한다
And Details에 CronTimers=1이 포함되어야 한다
And Details에 TimeoutTimers=1이 포함되어야 한다

Given StateStopped 상태의 TimerAgent가 주어졌을 때
When HealthCheck(ctx)를 호출하면
Then Healthy=false인 HealthStatus를 반환해야 한다
```

### AC-TIMER-001-30: 핸들러 패닉 보호

```gherkin
Given SetInterval("panicky", 100*time.Millisecond, panicHandler)가 등록되어 있을 때
When panicHandler가 panic("test")를 실행하면
Then TimerAgent가 중단되지 않아야 한다
And 다른 타이머의 핸들러는 정상 호출되어야 한다
```

### AC-TIMER-001-31: 최대 타이머 수 제한

```gherkin
Given WithMaxTimers(3)으로 생성된 TimerAgent에 3개의 타이머가 등록되어 있을 때
When SetInterval("overflow", 1*time.Second, handler)를 호출하면
Then ErrMaxTimersReached 에러를 반환해야 한다
```

### AC-TIMER-001-32: 동시성 안전

```gherkin
Given 초기화된 TimerAgent가 주어졌을 때
When 100개의 goroutine이 동시에 SetInterval/Cancel/List를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 연산이 정상 완료되어야 한다
```

---

## Module 6: Bridge Node Integration - 브릿지 노드 연동

### AC-TIMER-001-33: List 요청-응답 메시지

```gherkin
Given TimerAgent에 2개의 활성 타이머가 등록되어 있을 때
When Bridge Node를 통해 timer.operation="list" 메시지를 수신하면
Then 응답 메시지의 Payload에 2개의 타이머 정보가 JSON 배열로 포함되어야 한다
And 응답 메시지의 Metadata["timer.status"]가 "ok"여야 한다
```

### AC-TIMER-001-34: Cancel 메시지 처리

```gherkin
Given "my-timer" 타이머가 등록되어 있을 때
When Bridge Node를 통해 timer.operation="cancel", timer.id="my-timer" 메시지를 수신하면
Then 해당 타이머가 취소되어야 한다
And 응답 메시지의 Metadata["timer.status"]가 "ok"여야 한다
```

### AC-TIMER-001-35: 잘못된 연산 메시지

```gherkin
Given TimerAgent가 주어졌을 때
When Bridge Node를 통해 timer.operation="invalid_op" 메시지를 수신하면
Then 응답 메시지의 Metadata["timer.status"]가 "error"여야 한다
And 에러 메시지가 포함되어야 한다
```

---

## Module 7: Error Types - 에러 타입

### AC-TIMER-001-36: Sentinel 에러 정의

```gherkin
Given timer_errors.go가 로드되어 있을 때
Then ErrIntervalTooShort가 정의되어 있어야 한다
And ErrInvalidCronExpression이 정의되어 있어야 한다
And ErrTimerNotFound가 정의되어 있어야 한다
And ErrTimerIDEmpty가 정의되어 있어야 한다
And ErrTimerClosed가 정의되어 있어야 한다
And ErrTimerPaused가 정의되어 있어야 한다
And ErrNilHandler가 정의되어 있어야 한다
And ErrDuplicateTimerID가 정의되어 있어야 한다
And ErrMaxTimersReached가 정의되어 있어야 한다
And ErrInvalidDelay가 정의되어 있어야 한다
```

### AC-TIMER-001-37: errors.Is() 호환성

```gherkin
Given ErrTimerNotFound 에러가 주어졌을 때
When fmt.Errorf("cancel failed: %w", ErrTimerNotFound)로 래핑한 후
Then errors.Is(wrappedErr, ErrTimerNotFound)가 true를 반환해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-TIMER-001-01 ~ 37) 테스트 통과
- [ ] `go test ./internal/agent/system/...` 전체 통과
- [ ] `go test -race ./internal/agent/system/...` 경쟁 상태 없음
- [ ] `go vet ./internal/agent/system/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] `pkg/lifecycle/` 인터페이스 구현 확인 (Lifecycle, Configurable, HealthChecker)
- [ ] sentinel error가 `errors.Is()` 호환 확인
- [ ] 핸들러 패닉 보호(recover) 검증 완료
- [ ] 최소 간격 제한 검증 완료 (100ms)
- [ ] 최대 타이머 수 제한 검증 완료 (1000)
- [ ] Pause 상태에서 등록 거부 / List·Cancel 허용 검증
- [ ] Graceful Shutdown 검증 (핸들러 완료 대기, cron 정지, goroutine 종료)

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
| `testify/assert` | 테스트 어설션 |
