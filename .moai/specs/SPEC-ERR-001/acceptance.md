---
id: SPEC-ERR-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-ERR-001
---

# SPEC-ERR-001 수락 기준

## Module 1: ErrorMessage Type

### AC-ERR-001-01: ErrorMessage 기본 생성

```gherkin
Given 유효한 Message와 error, 노드 ID "node-filter-01"이 주어졌을 때
When NewErrorMessage(msg, err, "node-filter-01")를 호출하면
Then nil error를 반환해야 한다
And ErrorMessage의 OriginalMessage()가 전달된 msg와 동일해야 한다
And ErrorMessage의 Error()가 전달된 err와 동일해야 한다
And ErrorMessage의 SourceNodeID()가 "node-filter-01"이어야 한다
And ErrorMessage의 ErrorTimestamp()가 현재 시각에 근접해야 한다
And ErrorMessage의 Severity()가 SeverityError여야 한다 (기본값)
And ErrorMessage의 Category()가 CategoryProcessing이어야 한다 (기본값)
And ErrorMessage의 StackContext()가 빈 문자열이어야 한다 (기본값)
```

### AC-ERR-001-02: ErrorMessage 옵션 오버라이드

```gherkin
Given 유효한 Message와 error가 주어졌을 때
When NewErrorMessage(msg, err, "node-01",
    WithSeverity(SeverityCritical),
    WithCategory(CategoryConnection),
    WithStackContext("connection timeout at line 42"))를 호출하면
Then ErrorMessage의 Severity()가 SeverityCritical이어야 한다
And ErrorMessage의 Category()가 CategoryConnection이어야 한다
And ErrorMessage의 StackContext()가 "connection timeout at line 42"여야 한다
```

### AC-ERR-001-03: ErrorMessage nil 메시지 거부

```gherkin
Given msg가 nil인 경우
When NewErrorMessage(nil, errors.New("test"), "node-01")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrNilMessage)가 true여야 한다
```

### AC-ERR-001-04: ErrorMessage nil error 거부

```gherkin
Given err가 nil인 경우
When NewErrorMessage(msg, nil, "node-01")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrNilError)가 true여야 한다
```

---

## Module 2: DeadLetterMessage Type

### AC-ERR-001-05: DeadLetterMessage 기본 생성

```gherkin
Given 유효한 Message와 ReasonTTLExpired가 주어졌을 때
When NewDeadLetterMessage(msg, ReasonTTLExpired)를 호출하면
Then nil error를 반환해야 한다
And DeadLetterMessage의 OriginalMessage()가 전달된 msg와 동일해야 한다
And DeadLetterMessage의 Reason()이 ReasonTTLExpired여야 한다
And DeadLetterMessage의 DropTimestamp()가 현재 시각에 근접해야 한다
And DeadLetterMessage의 SourceNodeID()가 빈 문자열이어야 한다
And DeadLetterMessage의 SourceWireID()가 빈 문자열이어야 한다
And DeadLetterMessage의 Context()가 빈 map이어야 한다
```

### AC-ERR-001-06: DeadLetterMessage 옵션 설정

```gherkin
Given 유효한 Message가 주어졌을 때
When NewDeadLetterMessage(msg, ReasonBackpressureDrop,
    WithSourceNodeID("node-transform-01"),
    WithSourceWireID("wire-001"),
    WithContext(map[string]string{"buffer_size": "100", "queue_length": "100"}))를 호출하면
Then DeadLetterMessage의 SourceNodeID()가 "node-transform-01"이어야 한다
And DeadLetterMessage의 SourceWireID()가 "wire-001"이어야 한다
And DeadLetterMessage의 Context()["buffer_size"]가 "100"이어야 한다
And DeadLetterMessage의 Context()["queue_length"]가 "100"이어야 한다
```

### AC-ERR-001-07: DeadLetterMessage 유효하지 않은 DropReason 거부

```gherkin
Given 정의되지 않은 DropReason "invalid_reason"이 주어졌을 때
When NewDeadLetterMessage(msg, DropReason("invalid_reason"))를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidDropReason)이 true여야 한다
```

### AC-ERR-001-08: DeadLetterMessage Context 맵 독립성

```gherkin
Given 외부 context map{"key": "value1"}으로 DeadLetterMessage를 생성했을 때
When 외부 context map의 "key" 값을 "value2"로 변경하면
Then DeadLetterMessage의 Context()["key"]가 여전히 "value1"이어야 한다
```

---

## Module 3: StatusEvent Type

### AC-ERR-001-09: StatusEvent 기본 생성

```gherkin
Given ComponentNode, "node-filter-01", StateRunning, StatePaused가 주어졌을 때
When NewStatusEvent(ComponentNode, "node-filter-01", StateRunning, StatePaused)를 호출하면
Then nil error를 반환해야 한다
And StatusEvent의 ComponentType()가 ComponentNode여야 한다
And StatusEvent의 ComponentID()가 "node-filter-01"이어야 한다
And StatusEvent의 PreviousState()가 StateRunning이어야 한다
And StatusEvent의 NewState()가 StatePaused여야 한다
And StatusEvent의 Timestamp()가 현재 시각에 근접해야 한다
And StatusEvent의 Info()가 빈 map이어야 한다
```

### AC-ERR-001-10: StatusEvent Info 옵션

```gherkin
Given Error 상태 전이 정보가 주어졌을 때
When NewStatusEvent(ComponentAgent, "agent-mqtt-01", StateRunning, StateError,
    WithInfo(map[string]string{"error": "connection refused", "retry_count": "3"}))를 호출하면
Then StatusEvent의 Info()["error"]가 "connection refused"여야 한다
And StatusEvent의 Info()["retry_count"]가 "3"이어야 한다
```

### AC-ERR-001-11: StatusEvent 동일 상태 전이 거부

```gherkin
Given prevState와 newState가 모두 StateRunning인 경우
When NewStatusEvent(ComponentNode, "node-01", StateRunning, StateRunning)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrSameStateTransition)가 true여야 한다
```

### AC-ERR-001-12: StatusEvent 모든 ComponentType 지원

```gherkin
Given ComponentFlow, ComponentNode, ComponentAgent, ComponentScriptEngine, ComponentPlugin 각각에 대해
When NewStatusEvent(compType, "comp-01", StateCreated, StateInitializing)를 호출하면
Then 모든 경우 nil error를 반환해야 한다
And ComponentType()이 전달된 타입과 일치해야 한다
```

---

## Module 4: Error Severity & Classification

### AC-ERR-001-13: ErrorSeverity 유효성 검증

```gherkin
Given SeverityCritical, SeverityError, SeverityWarning이 주어졌을 때
When IsValidSeverity()를 호출하면
Then 모든 경우 true를 반환해야 한다

Given ErrorSeverity("unknown")이 주어졌을 때
When IsValidSeverity()를 호출하면
Then false를 반환해야 한다
```

### AC-ERR-001-14: ErrorCategory 유효성 검증

```gherkin
Given CategoryProcessing, CategoryValidation, CategoryTimeout, CategoryConnection, CategoryConfiguration, CategorySystem이 주어졌을 때
When IsValidCategory()를 호출하면
Then 모든 경우 true를 반환해야 한다

Given ErrorCategory("unknown")이 주어졌을 때
When IsValidCategory()를 호출하면
Then false를 반환해야 한다
```

### AC-ERR-001-15: DropReason 유효성 검증

```gherkin
Given ReasonTTLExpired, ReasonBackpressureDrop, ReasonFilterRejected, ReasonMaxRetriesExceeded, ReasonNodeStopped, ReasonChannelFull이 주어졌을 때
When IsValidDropReason()를 호출하면
Then 모든 경우 true를 반환해야 한다

Given DropReason("unknown")이 주어졌을 때
When IsValidDropReason()를 호출하면
Then false를 반환해야 한다
```

### AC-ERR-001-16: ClassifySeverity 에러 분류

```gherkin
Given context.DeadlineExceeded 에러가 주어졌을 때
When ClassifySeverity(err)를 호출하면
Then SeverityError를 반환해야 한다

Given panic recover로 잡힌 에러가 주어졌을 때
When ClassifySeverity(err)를 호출하면
Then SeverityCritical을 반환해야 한다

Given 알 수 없는 에러가 주어졌을 때
When ClassifySeverity(err)를 호출하면
Then SeverityError를 반환해야 한다 (기본값)
```

### AC-ERR-001-17: ClassifyCategory 에러 분류

```gherkin
Given context.DeadlineExceeded 에러가 주어졌을 때
When ClassifyCategory(err)를 호출하면
Then CategoryTimeout을 반환해야 한다

Given net.Error 인터페이스를 구현하는 에러가 주어졌을 때
When ClassifyCategory(err)를 호출하면
Then CategoryConnection을 반환해야 한다

Given 알 수 없는 에러가 주어졌을 때
When ClassifyCategory(err)를 호출하면
Then CategoryProcessing을 반환해야 한다 (기본값)
```

---

## Module 5: Discard Policy

### AC-ERR-001-18: LogAndDiscardPolicy 에러 메시지 처리

```gherkin
Given LogAndDiscardPolicy가 mock Logger와 mock Metrics로 생성되었을 때
When HandleError(ctx, errorMessage)를 호출하면
Then mock Logger의 Error()가 1회 호출되어야 한다
And mock Metrics의 IncrCounter("xferr_errors_discarded", ...)가 1회 호출되어야 한다
```

### AC-ERR-001-19: LogAndDiscardPolicy 폐기 메시지 처리

```gherkin
Given LogAndDiscardPolicy가 mock Logger와 mock Metrics로 생성되었을 때
When HandleDeadLetter(ctx, deadLetterMessage)를 호출하면
Then mock Metrics의 IncrCounter("xferr_dead_letters_discarded", ...)가 1회 호출되어야 한다
```

### AC-ERR-001-20: SilentDiscardPolicy 로그 미기록

```gherkin
Given SilentDiscardPolicy가 mock Metrics로 생성되었을 때
When HandleError(ctx, errorMessage)를 호출하면
Then mock Metrics의 IncrCounter()가 1회 호출되어야 한다
And Logger는 전혀 호출되지 않아야 한다 (Logger 의존성 없음)
```

### AC-ERR-001-21: PanicOnCriticalPolicy Critical panic

```gherkin
Given PanicOnCriticalPolicy가 생성되었을 때
And ErrorMessage의 Severity()가 SeverityCritical인 경우
When HandleError(ctx, errorMessage)를 호출하면
Then panic이 발생해야 한다

Given PanicOnCriticalPolicy가 생성되었을 때
And ErrorMessage의 Severity()가 SeverityError인 경우
When HandleError(ctx, errorMessage)를 호출하면
Then panic이 발생하지 않아야 한다
And LogAndDiscardPolicy와 동일하게 로그/메트릭 기록 후 폐기되어야 한다
```

---

## Module 6: Error Router

### AC-ERR-001-22: ErrorRouter 정확한 scope 매칭

```gherkin
Given DefaultErrorRouter에 scope "node-filter-01"로 ErrorReceiver가 등록되어 있을 때
When SourceNodeID()가 "node-filter-01"인 ErrorMessage로 RouteError()를 호출하면
Then 등록된 ErrorReceiver의 ReceiveError()가 1회 호출되어야 한다
And nil error를 반환해야 한다
```

### AC-ERR-001-23: ErrorRouter 와일드카드 매칭

```gherkin
Given DefaultErrorRouter에 scope "*"로 ErrorReceiver가 등록되어 있을 때
When 임의의 SourceNodeID를 가진 ErrorMessage로 RouteError()를 호출하면
Then 등록된 ErrorReceiver의 ReceiveError()가 1회 호출되어야 한다
```

### AC-ERR-001-24: ErrorRouter Fan-out 전달

```gherkin
Given DefaultErrorRouter에 scope "node-01"로 ErrorReceiver A가 등록되고,
    scope "*"로 ErrorReceiver B가 등록되어 있을 때
When SourceNodeID()가 "node-01"인 ErrorMessage로 RouteError()를 호출하면
Then ErrorReceiver A의 ReceiveError()가 1회 호출되어야 한다
And ErrorReceiver B의 ReceiveError()가 1회 호출되어야 한다
```

### AC-ERR-001-25: ErrorRouter 수신자 미매칭 시 DiscardPolicy 호출

```gherkin
Given DefaultErrorRouter에 어떤 ErrorReceiver도 등록되지 않았을 때
And LogAndDiscardPolicy가 설정되어 있을 때
When ErrorMessage로 RouteError()를 호출하면
Then DiscardPolicy의 HandleError()가 1회 호출되어야 한다
```

### AC-ERR-001-26: ErrorRouter scope 미매칭 시 DiscardPolicy 호출

```gherkin
Given DefaultErrorRouter에 scope "node-01"로만 ErrorReceiver가 등록되어 있을 때
When SourceNodeID()가 "node-02"인 ErrorMessage로 RouteError()를 호출하면
Then DiscardPolicy의 HandleError()가 1회 호출되어야 한다
And 등록된 ErrorReceiver는 호출되지 않아야 한다
```

### AC-ERR-001-27: ErrorRouter DeadLetter 라우팅

```gherkin
Given DefaultErrorRouter에 DeadLetterReceiver가 등록되어 있을 때
When DeadLetterMessage로 RouteDeadLetter()를 호출하면
Then 등록된 DeadLetterReceiver의 ReceiveDeadLetter()가 1회 호출되어야 한다

Given DefaultErrorRouter에 DeadLetterReceiver가 등록되지 않았을 때
When DeadLetterMessage로 RouteDeadLetter()를 호출하면
Then DiscardPolicy의 HandleDeadLetter()가 1회 호출되어야 한다
```

### AC-ERR-001-28: ErrorRouter Status 라우팅

```gherkin
Given DefaultErrorRouter에 StatusReceiver가 등록되어 있을 때
When StatusEvent로 RouteStatus()를 호출하면
Then 등록된 StatusReceiver의 ReceiveStatus()가 1회 호출되어야 한다

Given DefaultErrorRouter에 StatusReceiver가 등록되지 않았을 때
When StatusEvent로 RouteStatus()를 호출하면
Then DiscardPolicy의 HandleStatus()가 1회 호출되어야 한다
```

### AC-ERR-001-29: ErrorRouter 동시성 안전

```gherkin
Given DefaultErrorRouter가 생성되어 있을 때
When 10개의 goroutine이 동시에 RegisterErrorReceiver()와 RouteError()를 호출하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
And 모든 등록과 라우팅이 정상적으로 완료되어야 한다
```

---

## Module 7: Alert Thresholds

### AC-ERR-001-30: AlertThreshold 구조체

```gherkin
Given AlertThreshold{Name: "high_error_rate", ThresholdType: AlertThresholdRate, Value: 10.0, WindowDuration: time.Minute, Scope: "*"}가 주어졌을 때
Then Name이 "high_error_rate"여야 한다
And ThresholdType이 AlertThresholdRate여야 한다
And Value가 10.0이어야 한다
And WindowDuration이 1분이어야 한다
And Scope가 "*"여야 한다
```

### AC-ERR-001-31: AlertConfig 비활성화

```gherkin
Given AlertConfig{Enabled: false}가 주어졌을 때
Then Enabled가 false여야 한다
And Thresholds가 nil이어야 한다
And Callback이 nil이어야 한다
```

---

## Module 8: Error Types

### AC-ERR-001-32: Sentinel 에러 errors.Is() 호환성

```gherkin
Given ErrNilMessage, ErrNilError, ErrNoReceiver, ErrAlertThresholdExceeded, ErrInvalidSeverity, ErrInvalidCategory, ErrInvalidDropReason, ErrInvalidComponentType, ErrSameStateTransition 각각에 대해
When errors.Is(err, sentinel)를 호출하면
Then 동일한 sentinel 에러와 비교 시 true를 반환해야 한다
And 다른 sentinel 에러와 비교 시 false를 반환해야 한다
```

### AC-ERR-001-33: Sentinel 에러 fmt.Errorf 래핑 호환성

```gherkin
Given ErrNilMessage를 fmt.Errorf("wrapper: %w", ErrNilMessage)로 래핑했을 때
When errors.Is(wrappedErr, ErrNilMessage)를 호출하면
Then true를 반환해야 한다
```

---

## 품질 게이트

### QG-ERR-001-01: 테스트 커버리지

```gherkin
Given pkg/xferr/ 패키지의 모든 소스 파일에 대해
When go test -cover ./pkg/xferr/를 실행하면
Then 테스트 커버리지가 85% 이상이어야 한다
```

### QG-ERR-001-02: Race Condition

```gherkin
Given pkg/xferr/ 패키지의 모든 테스트에 대해
When go test -race ./pkg/xferr/를 실행하면
Then data race가 감지되지 않아야 한다
```

### QG-ERR-001-03: 린트

```gherkin
Given pkg/xferr/ 패키지의 모든 소스 파일에 대해
When golangci-lint run ./pkg/xferr/를 실행하면
Then 린트 에러가 0개여야 한다
```

### QG-ERR-001-04: vet

```gherkin
Given pkg/xferr/ 패키지의 모든 소스 파일에 대해
When go vet ./pkg/xferr/를 실행하면
Then 경고가 0개여야 한다
```
