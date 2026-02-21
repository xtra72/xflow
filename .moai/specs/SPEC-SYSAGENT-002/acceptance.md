# SPEC-SYSAGENT-002: 인수 기준

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SYSAGENT-002 |
| 형식 | Given-When-Then (Gherkin) |

---

## Module 1: SystemConfig 확장

### AC-001: SystemConfig 새 필드 기본값

```gherkin
Given SystemConfig가 zero value로 생성되었을 때
When Initialize()에 전달되면
Then StoreDefaultTTL은 0 (무제한)으로 적용된다
And TimerMinInterval은 100ms로 적용된다
And LogDefaultLevel은 LogLevelInfo로 적용된다
```

### AC-002: SystemConfig 사용자 지정 값

```gherkin
Given SystemConfig에 StoreDefaultTTL=5m, TimerMinInterval=1s, LogDefaultLevel=LogLevelDebug가 설정되었을 때
When Initialize()에 전달되면
Then 각 에이전트는 지정된 값으로 초기화된다
```

### AC-003: LogLevel 타입 안전성

```gherkin
Given LogDefaultLevel 필드가 LogLevel 타입일 때
When 잘못된 문자열을 직접 대입하려 하면
Then 컴파일 에러가 발생한다
```

---

## Module 2: SystemAgentManager 5종 통합

### AC-010: Initialize 5종 에이전트 생성

```gherkin
Given SystemAgentManager가 새로 생성되었을 때
When Initialize(validConfig)를 호출하면
Then IsInitialized()가 true를 반환한다
And Event()가 nil이 아니다
And Store()가 nil이 아니다
And Timer()가 nil이 아니다
And Logger()가 nil이 아니다
And File()가 nil이 아니다
```

### AC-011: Initialize 중복 호출 방어

```gherkin
Given SystemAgentManager가 이미 초기화되었을 때
When Initialize()를 다시 호출하면
Then ErrAlreadyInitialized 에러가 반환된다
```

### AC-012: Initialize 롤백 - Store 실패

```gherkin
Given SystemConfig에 유효하지 않은 Store 설정이 있을 때
When Initialize()가 Store 에이전트 초기화에 실패하면
Then Event 에이전트는 Stop()이 호출되어 정리된다
And 에러 메시지에 "store" 또는 해당 에이전트 이름이 포함된다
And IsInitialized()가 false이다
```

### AC-013: Initialize 롤백 - Logger 실패

```gherkin
Given SystemConfig에 유효하지 않은 Logger 설정이 있을 때
When Initialize()가 Logger 에이전트 초기화에 실패하면
Then Event, Store, Timer 에이전트가 역순으로 Stop()된다
And IsInitialized()가 false이다
```

### AC-014: Initialize 롤백 - File 실패

```gherkin
Given SystemConfig에 유효하지 않은 File 설정이 있을 때
When Initialize()가 File 에이전트 초기화에 실패하면
Then Event, Store, Timer, Logger 에이전트가 역순으로 Stop()된다
And IsInitialized()가 false이다
```

### AC-015: Start 5종 에이전트

```gherkin
Given SystemAgentManager가 초기화된 상태일 때
When Start(ctx)를 호출하면
Then IsStarted()가 true를 반환한다
And 에러가 없다
```

### AC-016: Start 미초기화 상태 방어

```gherkin
Given SystemAgentManager가 초기화되지 않았을 때
When Start(ctx)를 호출하면
Then ErrNotInitialized 에러가 반환된다
```

### AC-017: Start 롤백

```gherkin
Given SystemAgentManager가 초기화된 상태일 때
When Start() 과정에서 N번째 에이전트가 실패하면
Then 1~(N-1)번째 에이전트가 역순으로 Stop()된다
And IsStarted()가 false이다
```

### AC-018: Stop 역순 종료

```gherkin
Given SystemAgentManager가 시작된 상태일 때
When Stop(ctx)를 호출하면
Then IsStarted()가 false를 반환한다
And 에러가 없다
```

### AC-019: Stop 미초기화 상태 방어

```gherkin
Given SystemAgentManager가 초기화되지 않았을 때
When Stop(ctx)를 호출하면
Then ErrNotInitialized 에러가 반환된다
```

### AC-020: Stop 부분 실패 시 나머지 종료 계속

```gherkin
Given SystemAgentManager가 시작된 상태일 때
When Stop() 과정에서 하나의 에이전트 종료가 실패하면
Then 나머지 에이전트도 종료가 시도된다
And 첫 번째 에러가 반환된다
```

### AC-021: 접근자 초기화 전 nil 반환

```gherkin
Given SystemAgentManager가 새로 생성되었을 때 (초기화 전)
When Logger(), Timer(), Store()를 호출하면
Then 모두 nil을 반환한다
```

### AC-022: 전체 라이프사이클 통합

```gherkin
Given SystemAgentManager가 새로 생성되었을 때
When Initialize -> Start -> (에이전트 사용) -> Stop 순서로 호출하면
Then 모든 단계가 에러 없이 완료된다
And 각 에이전트의 기능이 정상 동작한다 (Event emit/subscribe, File read/write, Store get/set, Timer 등록, Logger 로깅)
```

### AC-023: 동시성 안전

```gherkin
Given SystemAgentManager가 시작된 상태일 때
When 10개 이상의 goroutine에서 동시에 접근자 메서드를 호출하면
Then race condition이 발생하지 않는다
And go test -race가 통과한다
```

---

## Module 3: 브릿지 핸들러 접근

### AC-030: Logger 브릿지 핸들러 접근

```gherkin
Given SystemAgentManager가 초기화된 상태일 때
When LoggerBridge()를 호출하면
Then nil이 아닌 *LoggerBridgeHandler가 반환된다
```

### AC-031: Timer 브릿지 핸들러 접근

```gherkin
Given SystemAgentManager가 초기화된 상태일 때
When TimerBridge()를 호출하면
Then nil이 아닌 *TimerBridgeHandler가 반환된다
```

### AC-032: Store 브릿지 핸들러 접근

```gherkin
Given SystemAgentManager가 초기화된 상태일 때
When StoreBridge()를 호출하면
Then nil이 아닌 *BridgeHandler가 반환된다
```

### AC-033: 미초기화 상태 브릿지 nil 반환

```gherkin
Given SystemAgentManager가 초기화되지 않았을 때
When LoggerBridge(), TimerBridge(), StoreBridge()를 호출하면
Then 모두 nil을 반환한다
```

---

## Module 4: main.go 통합

### AC-040: xflowd 서버 시작 시 SystemAgentManager 통합

```gherkin
Given xflowd 서버 설정이 유효할 때
When runServer()가 실행되면
Then SystemAgentManager가 생성되고 초기화된다
And SystemAgentManager가 시작된다
And 서버 종료 시 SystemAgentManager.Stop()이 호출된다
```

### AC-041: SystemAgentManager 초기화 실패 시 서버 시작 중단

```gherkin
Given SystemConfig에 유효하지 않은 값이 있을 때
When runServer()가 실행되면
Then SystemAgentManager 초기화 실패 에러가 반환된다
And 서버가 시작되지 않는다
```

### AC-042: Observer 연결

```gherkin
Given xflowd 서버에 Observer가 설정되어 있을 때
When SystemAgentManager가 초기화되면
Then Logger 에이전트가 Observer를 통해 로그를 출력할 수 있다
```

---

## Module 5: 에러 처리

### AC-050: 센티넬 에러 정의 확인

```gherkin
Given manager_errors.go 파일이 있을 때
When 에러 상수를 확인하면
Then ErrAgentInitFailed가 정의되어 있다
And ErrAgentStartFailed가 정의되어 있다
And ErrAgentStopFailed가 정의되어 있다
```

### AC-051: 에러 래핑 검증

```gherkin
Given Initialize()가 에이전트 초기화에 실패했을 때
When 반환된 에러에 errors.Is(err, ErrAgentInitFailed)를 검사하면
Then true가 반환된다
```

### AC-052: Start 에러 래핑 검증

```gherkin
Given Start()가 에이전트 시작에 실패했을 때
When 반환된 에러에 errors.Is(err, ErrAgentStartFailed)를 검사하면
Then true가 반환된다
```

---

## Quality Gate

| 항목 | 기준 |
|------|------|
| 테스트 커버리지 | 85% 이상 (`system_manager.go` 대상) |
| Race Condition | `go test -race ./internal/agent/system/...` 통과 |
| 기존 테스트 호환 | SPEC-SYSAGENT-001 관련 기존 테스트 모두 통과 |
| 에러 처리 | 모든 에러 경로에 대한 테스트 존재 |
| 롤백 검증 | Initialize/Start 롤백 경로 테스트 존재 |
| 동시성 테스트 | 10+ goroutine 동시 접근 테스트 존재 |

---

## Definition of Done

- [ ] `SystemConfig`에 3개 신규 필드 추가 완료
- [ ] `LogDefaultLevel` 타입이 `string`에서 `LogLevel`로 변경 완료
- [ ] `SystemAgentManager`에 `logger`, `timer`, `store` 필드 추가 완료
- [ ] `Initialize()`가 5종 에이전트를 순서대로 생성/초기화
- [ ] `Start()`가 5종 에이전트를 순서대로 시작
- [ ] `Stop()`이 5종 에이전트를 역순으로 종료
- [ ] Initialize/Start 롤백 로직 구현 및 테스트
- [ ] `Logger()`, `Timer()`, `Store()` 접근자 메서드 구현
- [ ] `LoggerBridge()`, `TimerBridge()`, `StoreBridge()` 메서드 구현
- [ ] `cmd/xflowd/main.go` 통합 완료
- [ ] 센티넬 에러 3종 추가 완료
- [ ] `go test -race ./internal/agent/system/...` 통과
- [ ] 테스트 커버리지 85% 이상
- [ ] 기존 SPEC-SYSAGENT-001 테스트 모두 통과

---

## Traceability

| 인수 기준 | 요구사항 |
|-----------|----------|
| AC-001~003 | REQ-SYSAGENT-002-001, 002 |
| AC-010~023 | REQ-SYSAGENT-002-010~017 |
| AC-030~033 | REQ-SYSAGENT-002-020, 021 |
| AC-040~042 | REQ-SYSAGENT-002-030, 031 |
| AC-050~052 | REQ-SYSAGENT-002-040, 041 |
