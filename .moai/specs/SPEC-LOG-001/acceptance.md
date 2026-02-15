---
id: SPEC-LOG-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-LOG-001
---

# SPEC-LOG-001 수락 기준

## Module 1: Logger Interface - 로거 인터페이스

### AC-LOG-001-01: Logger 인터페이스 메서드 시그니처

```gherkin
Given Logger 인터페이스가 정의되어 있을 때
Then WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error 메서드가 존재해야 한다
And SetLevel(ctx context.Context, component string, level slog.Level) error 메서드가 존재해야 한다
And SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error) 메서드가 존재해야 한다
And GetLevel(ctx context.Context, component string) (slog.Level, error) 메서드가 존재해야 한다
And Subscribe(ctx context.Context, component string, writer io.Writer) (string, error) 메서드가 존재해야 한다
And Unsubscribe(ctx context.Context, subscriptionID string) error 메서드가 존재해야 한다
And Components(ctx context.Context) ([]string, error) 메서드가 존재해야 한다
```

### AC-LOG-001-02: WriteLog를 통한 로그 기록

```gherkin
Given 초기화된 LoggerAgent가 주어졌을 때
When WriteLog(ctx, "engine", slog.LevelInfo, "시작 완료")를 호출하면
Then nil error를 반환해야 한다
And Observer의 LoggerFactory에 "engine" 컴포넌트가 등록되어야 한다

Given 초기화된 LoggerAgent에 "engine" 컴포넌트에 bytes.Buffer를 Subscribe한 상태일 때
When WriteLog(ctx, "engine", slog.LevelInfo, "테스트 메시지", "key", "value")를 호출하면
Then Subscribe된 Buffer에 로그가 기록되어야 한다
And 로그에 "component" 속성이 포함되어야 한다
```

### AC-LOG-001-03: SetLevel을 통한 레벨 변경

```gherkin
Given 초기화된 LoggerAgent가 주어졌을 때
When SetLevel(ctx, "engine", slog.LevelDebug)를 호출하면
Then nil error를 반환해야 한다
And GetLevel(ctx, "engine")가 slog.LevelDebug를 반환해야 한다
```

### AC-LOG-001-04: SetLevelByPattern을 통한 패턴 매칭 레벨 변경

```gherkin
Given LoggerAgent에 "agent.store", "agent.timer", "engine" 컴포넌트가 등록되어 있을 때
When SetLevelByPattern(ctx, "agent.*", slog.LevelDebug)를 호출하면
Then 반환값이 2여야 한다 (agent.store, agent.timer 매칭)
And GetLevel(ctx, "agent.store")가 slog.LevelDebug를 반환해야 한다
And GetLevel(ctx, "agent.timer")가 slog.LevelDebug를 반환해야 한다
And GetLevel(ctx, "engine")의 레벨은 변경되지 않아야 한다
```

### AC-LOG-001-05: GetLevel 조회

```gherkin
Given LoggerAgent에 "engine" 컴포넌트가 레벨 slog.LevelWarn으로 설정되어 있을 때
When GetLevel(ctx, "engine")를 호출하면
Then slog.LevelWarn을 반환해야 한다
And nil error를 반환해야 한다

Given 등록되지 않은 컴포넌트 "unknown"이 주어졌을 때
When GetLevel(ctx, "unknown")를 호출하면
Then 기본 레벨(slog.LevelInfo)을 반환해야 한다
```

### AC-LOG-001-06: Subscribe/Unsubscribe 스트림 구독

```gherkin
Given 초기화된 LoggerAgent가 주어졌을 때
When Subscribe(ctx, "engine", &buf)를 호출하면
Then 빈 문자열이 아닌 구독 ID를 반환해야 한다
And nil error를 반환해야 한다

When 반환된 구독 ID로 Unsubscribe(ctx, subID)를 호출하면
Then nil error를 반환해야 한다

When 동일한 구독 ID로 다시 Unsubscribe(ctx, subID)를 호출하면
Then ErrSubscriptionNotFound 에러를 반환해야 한다
```

### AC-LOG-001-07: Components 목록 조회

```gherkin
Given LoggerAgent에서 WriteLog로 "engine", "store", "timer" 컴포넌트에 로그를 기록한 후
When Components(ctx)를 호출하면
Then ["engine", "store", "timer"]를 반환해야 한다 (알파벳 순 정렬)
And nil error를 반환해야 한다
```

### AC-LOG-001-08: 빈 컴포넌트명 검증

```gherkin
Given 초기화된 LoggerAgent가 주어졌을 때
When WriteLog(ctx, "", slog.LevelInfo, "메시지")를 호출하면
Then ErrInvalidComponent 에러를 반환해야 한다

When SetLevel(ctx, "", slog.LevelDebug)를 호출하면
Then ErrInvalidComponent 에러를 반환해야 한다

When Subscribe(ctx, "", &buf)를 호출하면
Then ErrInvalidComponent 에러를 반환해야 한다
```

---

## Module 2: LoggerAgent Lifecycle - 로거 에이전트 생명주기

### AC-LOG-001-09: NewLoggerAgent 생성자

```gherkin
Given NewLoggerAgent()를 호출할 때
Then State()가 lifecycle.StateCreated를 반환해야 한다

Given NewLoggerAgent(WithLogDefaultLevel(slog.LevelDebug), WithLogFormat("text"))를 호출할 때
Then 생성된 LoggerAgent의 설정이 반영되어야 한다
```

### AC-LOG-001-10: Init -> Running 생명주기

```gherkin
Given StateCreated 상태의 LoggerAgent가 주어졌을 때
When Init(ctx)를 호출하면
Then State()가 lifecycle.StateRunning을 반환해야 한다
And 내부 Observer가 nil이 아니어야 한다
And 구독 맵이 초기화되어야 한다 (빈 맵)
```

### AC-LOG-001-11: Pause 시 Debug/Info 억제

```gherkin
Given StateRunning 상태의 LoggerAgent에 "test" 컴포넌트로 Buffer가 Subscribe되어 있을 때
When Pause(ctx)를 호출한 후 WriteLog(ctx, "test", slog.LevelDebug, "디버그 메시지")를 호출하면
Then nil error를 반환해야 한다 (에러 없이 억제)
And Buffer에 로그가 기록되지 않아야 한다

When Pause(ctx) 후 WriteLog(ctx, "test", slog.LevelInfo, "정보 메시지")를 호출하면
Then nil error를 반환해야 한다 (에러 없이 억제)
And Buffer에 로그가 기록되지 않아야 한다
```

### AC-LOG-001-12: Pause 시 Warn/Error 허용

```gherkin
Given StatePaused 상태의 LoggerAgent에 "test" 컴포넌트로 Buffer가 Subscribe되어 있을 때
When WriteLog(ctx, "test", slog.LevelWarn, "경고 메시지")를 호출하면
Then nil error를 반환해야 한다
And Buffer에 경고 로그가 기록되어야 한다

When WriteLog(ctx, "test", slog.LevelError, "에러 메시지")를 호출하면
Then nil error를 반환해야 한다
And Buffer에 에러 로그가 기록되어야 한다
```

### AC-LOG-001-13: Pause 시 관리 연산 정상 동작

```gherkin
Given StatePaused 상태의 LoggerAgent가 주어졌을 때
When SetLevel(ctx, "engine", slog.LevelDebug)를 호출하면
Then nil error를 반환해야 한다

When GetLevel(ctx, "engine")를 호출하면
Then slog.LevelDebug를 반환해야 한다

When Subscribe(ctx, "engine", &buf)를 호출하면
Then 유효한 구독 ID와 nil error를 반환해야 한다

When Components(ctx)를 호출하면
Then nil error를 반환해야 한다
```

### AC-LOG-001-14: Resume 시 전체 레벨 복원

```gherkin
Given StatePaused 상태의 LoggerAgent에 "test" 컴포넌트로 Buffer가 Subscribe되어 있을 때
When Resume(ctx)를 호출한 후 WriteLog(ctx, "test", slog.LevelDebug, "디버그 복원")를 호출하면
Then Buffer에 디버그 로그가 기록되어야 한다 (Pause 해제)

When Resume(ctx) 후 WriteLog(ctx, "test", slog.LevelInfo, "정보 복원")를 호출하면
Then Buffer에 정보 로그가 기록되어야 한다
```

### AC-LOG-001-15: Stop 시 Graceful Shutdown

```gherkin
Given StateRunning 상태의 LoggerAgent에 2개의 구독이 활성화되어 있을 때
When Stop(ctx)를 호출하면
Then State()가 lifecycle.StateStopped를 반환해야 한다
And 모든 구독이 해제되어야 한다

When Stop(ctx) 후 WriteLog(ctx, "test", slog.LevelInfo, "메시지")를 호출하면
Then ErrLoggerClosed 에러를 반환해야 한다

When Stop(ctx) 후 SetLevel(ctx, "test", slog.LevelDebug)를 호출하면
Then ErrLoggerClosed 에러를 반환해야 한다

When Stop(ctx) 후 Subscribe(ctx, "test", &buf)를 호출하면
Then ErrLoggerClosed 에러를 반환해야 한다
```

### AC-LOG-001-16: HealthCheck

```gherkin
Given StateRunning 상태의 LoggerAgent에 활성 구독 2개, 등록 컴포넌트 3개가 있을 때
When HealthCheck(ctx)를 호출하면
Then Healthy=true인 HealthStatus를 반환해야 한다
And Status가 "healthy"여야 한다
And Details에 "active_subscriptions": 2가 포함되어야 한다
And Details에 "registered_components": 3이 포함되어야 한다

Given StatePaused 상태의 LoggerAgent가 주어졌을 때
When HealthCheck(ctx)를 호출하면
Then Healthy=true인 HealthStatus를 반환해야 한다
And Status가 "degraded"여야 한다

Given StateStopped 상태의 LoggerAgent가 주어졌을 때
When HealthCheck(ctx)를 호출하면
Then Healthy=false인 HealthStatus를 반환해야 한다
And Status가 "unhealthy"여야 한다
```

### AC-LOG-001-17: 외부 Observer 주입

```gherkin
Given 외부에서 생성한 observe.Observer가 주어졌을 때
When NewLoggerAgent(WithLogObserver(observer))를 생성하고 Init(ctx)를 호출하면
Then LoggerAgent가 주입된 Observer를 사용해야 한다
And 새로운 Observer를 생성하지 않아야 한다

When 주입된 Observer의 LevelManager에서 SetLevel("ext", slog.LevelDebug)를 호출한 후
Then LoggerAgent.GetLevel(ctx, "ext")가 slog.LevelDebug를 반환해야 한다
```

### AC-LOG-001-18: 동시성 안전

```gherkin
Given 초기화된 LoggerAgent가 주어졌을 때
When 100개의 goroutine이 동시에 WriteLog, Subscribe, Unsubscribe를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 연산이 정상 완료되어야 한다
```

---

## Module 3: Options System - 옵션 시스템

### AC-LOG-001-19: WithLogDefaultLevel 옵션

```gherkin
Given NewLoggerAgent(WithLogDefaultLevel(slog.LevelDebug))로 생성한 LoggerAgent가 주어졌을 때
When Init(ctx)를 호출하고 GetLevel(ctx, "new-component")를 조회하면
Then slog.LevelDebug를 반환해야 한다 (기본 레벨 반영)
```

### AC-LOG-001-20: WithLogFormat 옵션

```gherkin
Given NewLoggerAgent(WithLogFormat("text"))로 생성한 LoggerAgent가 주어졌을 때
When Init(ctx) 후 Subscribe(ctx, "test", &buf)하고 WriteLog(ctx, "test", slog.LevelInfo, "hello")를 호출하면
Then Buffer에 text 포맷 로그가 기록되어야 한다 (JSON이 아닌 key=value 형식)
```

### AC-LOG-001-21: WithLogWriter 옵션

```gherkin
Given bytes.Buffer를 기본 Writer로 지정한 NewLoggerAgent(WithLogWriter(&buf))가 주어졌을 때
When Init(ctx) 후 WriteLog(ctx, "test", slog.LevelInfo, "hello")를 호출하면
Then 지정된 Buffer에 로그가 기록되어야 한다 (os.Stdout 대신)
```

### AC-LOG-001-22: WithLogObserver 옵션

```gherkin
Given 외부 Observer가 주어졌을 때
When NewLoggerAgent(WithLogObserver(observer))를 생성하면
Then loggerConfig.observer에 외부 Observer가 설정되어야 한다
```

### AC-LOG-001-23: 기본 설정값

```gherkin
Given 옵션 없이 NewLoggerAgent()를 호출할 때
Then 내부 config.defaultLevel이 slog.LevelInfo여야 한다
And 내부 config.format이 "json"이어야 한다
And 내부 config.defaultWriter가 nil이어야 한다 (os.Stdout은 Observer 기본값)
And 내부 config.observer가 nil이어야 한다 (Init 시 새로 생성)
```

---

## Module 4: Bridge Node Integration - 브릿지 노드 연동

### AC-LOG-001-24: Write 메시지 처리

```gherkin
Given 초기화된 LoggerAgent와 LoggerBridgeHandler가 주어졌을 때
When logger.operation="write", logger.component="engine", logger.level="info", logger.message="시작" 메시지를 수신하면
Then 응답 메시지의 Metadata["logger.status"]가 "ok"여야 한다
```

### AC-LOG-001-25: SetLevel 메시지 처리

```gherkin
Given 초기화된 LoggerAgent와 LoggerBridgeHandler가 주어졌을 때
When logger.operation="set_level", logger.component="engine", logger.level="debug" 메시지를 수신하면
Then 응답 메시지의 Metadata["logger.status"]가 "ok"여야 한다
And LoggerAgent.GetLevel(ctx, "engine")가 slog.LevelDebug를 반환해야 한다
```

### AC-LOG-001-26: SetLevelByPattern 메시지 처리

```gherkin
Given LoggerAgent에 "agent.store", "agent.timer" 컴포넌트가 등록되어 있을 때
When logger.operation="set_level_pattern", logger.pattern="agent.*", logger.level="warn" 메시지를 수신하면
Then 응답 메시지의 Metadata["logger.status"]가 "ok"여야 한다
And Payload["count"]가 2여야 한다
```

### AC-LOG-001-27: GetLevel 요청-응답

```gherkin
Given LoggerAgent에 "engine" 컴포넌트가 slog.LevelWarn으로 설정되어 있을 때
When logger.operation="get_level", logger.component="engine" 메시지를 수신하면
Then 응답 메시지의 Metadata["logger.status"]가 "ok"여야 한다
And Payload["level"]이 "WARN"이어야 한다
```

### AC-LOG-001-28: Components 요청-응답

```gherkin
Given LoggerAgent에 "engine", "store" 컴포넌트가 등록되어 있을 때
When logger.operation="components" 메시지를 수신하면
Then 응답 메시지의 Metadata["logger.status"]가 "ok"여야 한다
And Payload["components"]가 ["engine", "store"]를 포함해야 한다
```

### AC-LOG-001-29: 잘못된 연산 메시지

```gherkin
Given LoggerAgent와 LoggerBridgeHandler가 주어졌을 때
When logger.operation="invalid_op" 메시지를 수신하면
Then 응답 메시지의 Metadata["logger.status"]가 "error"여야 한다
And 에러 메시지가 포함되어야 한다
```

### AC-LOG-001-30: 잘못된 레벨 문자열

```gherkin
Given LoggerBridgeHandler가 주어졌을 때
When logger.operation="set_level", logger.component="engine", logger.level="invalid" 메시지를 수신하면
Then 응답 메시지의 Metadata["logger.status"]가 "error"여야 한다
And Metadata["logger.error"]에 "invalid log level" 정보가 포함되어야 한다
```

---

## Module 5: Error Types - 에러 타입

### AC-LOG-001-31: Sentinel 에러 정의

```gherkin
Given logger_errors.go가 로드되어 있을 때
Then ErrLoggerClosed가 정의되어 있어야 한다
And ErrLoggerPaused가 정의되어 있어야 한다
And ErrInvalidLevel이 정의되어 있어야 한다
And ErrInvalidComponent가 정의되어 있어야 한다
And ErrSubscriptionNotFound가 정의되어 있어야 한다
```

### AC-LOG-001-32: errors.Is() 호환성

```gherkin
Given ErrLoggerClosed 에러가 주어졌을 때
When fmt.Errorf("operation failed: %w", ErrLoggerClosed)로 래핑한 후
Then errors.Is(wrappedErr, ErrLoggerClosed)가 true를 반환해야 한다

Given ErrSubscriptionNotFound 에러가 주어졌을 때
When fmt.Errorf("unsubscribe failed: %w", ErrSubscriptionNotFound)로 래핑한 후
Then errors.Is(wrappedErr, ErrSubscriptionNotFound)가 true를 반환해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-LOG-001-01 ~ 32) 테스트 통과
- [ ] `go test ./internal/agent/system/...` 전체 통과
- [ ] `go test -race ./internal/agent/system/...` 경쟁 상태 없음
- [ ] `go vet ./internal/agent/system/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] `pkg/lifecycle/` 인터페이스 구현 확인 (Lifecycle, HealthChecker)
- [ ] sentinel error가 `errors.Is()` 호환 확인
- [ ] Pause 상태에서 Debug/Info 억제 + Warn/Error 허용 검증
- [ ] Graceful Shutdown 검증 (모든 구독 해제, Observer 정리)
- [ ] Observer 래핑 위임 검증 (기존 observe 기능 재사용 확인)
- [ ] 외부 Observer 주입 검증

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
| `testify/assert` | 테스트 어설션 |
| `testify/require` | 치명적 어설션 (테스트 중단) |
| `bytes.Buffer` | 로그 출력 캡처 및 검증 |
