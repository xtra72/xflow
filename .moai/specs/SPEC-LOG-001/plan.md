---
id: SPEC-LOG-001
type: plan
version: "1.0.0"
spec_ref: SPEC-LOG-001
status: draft
---

# SPEC-LOG-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 신규 파일이므로 TDD(RED-GREEN-REFACTOR) 적용
- 모든 파일이 신규 생성이므로 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `sync.RWMutex` (구독 맵 보호)
- **래핑 대상**: `internal/observe/` (Observer, LoggerFactory, LevelManager, StreamRouter)
- **외부 의존성**: `pkg/lifecycle/` (SPEC-LIFE-001), `pkg/message/` (Bridge Node 연동 시)

### 1.3 패키지 위치

- **경로**: `internal/agent/system/`
- **Tier**: internal (비공개 패키지)
- **소비자**: internal/engine/ (Flow Runtime), internal/node/ (Bridge Node)

### 1.4 핵심 설계 제약

- **래퍼 패턴 엄수**: `internal/observe` 패키지의 기능을 재구현하지 않는다. 모든 로깅, 레벨 관리, 스트림 라우팅은 Observer의 하위 시스템에 위임한다.
- **구독 관리**: Subscribe/Unsubscribe는 LoggerAgent가 자체 관리하는 구독 맵을 유지하며, StreamRouter에 위임한다.
- **Pause 로직**: LoggerAgent 레벨에서 Debug/Info 억제를 구현한다 (Observer의 LevelManager를 변경하지 않음).

---

## 2. 마일스톤

### Primary Goal: Error Types + Logger Interface + Lifecycle + Options (P0)

**범위**: Module 1, 2, 3, 5

**작업 항목**:

1. `logger_errors.go` + 에러 테스트 작성
   - 5개 sentinel error 변수 정의 (ErrLoggerClosed, ErrLoggerPaused, ErrInvalidLevel, ErrInvalidComponent, ErrSubscriptionNotFound)
   - `errors.Is()` 호환성 테스트

2. `logger.go` (인터페이스 부분) 작성
   - `Logger` 인터페이스 정의 (7개 메서드)
   - `subscription` 내부 구조체 정의
   - `LoggerAgent` 구조체 기본 골격

3. `logger_options.go` + `logger_options_test.go` 작성
   - `LoggerOption` 함수 타입
   - `loggerConfig` 구조체 및 기본값
   - `WithLogDefaultLevel()`, `WithLogFormat()`, `WithLogWriter()`, `WithLogObserver()` 옵션
   - 옵션 적용 테스트

4. `logger.go` (LoggerAgent 구현) 완성 + `logger_test.go` 작성
   - `NewLoggerAgent()` 생성자 (기본 Observer 생성 또는 외부 주입)
   - `Init()` / `Start()` / `Pause()` / `Resume()` / `Stop()` 구현
   - `HealthCheck()` 구현
   - `WriteLog()` 구현 (Observer.Loggers 위임 + Pause 레벨 필터링)
   - `SetLevel()` / `SetLevelByPattern()` / `GetLevel()` 구현 (Observer.Levels 위임)
   - `Subscribe()` / `Unsubscribe()` 구현 (Observer.Streams 위임 + 구독 맵 관리)
   - `Components()` 구현 (Observer.Loggers 위임)
   - 생명주기 전체 흐름 통합 테스트
   - Pause 시 Debug/Info 억제 + Warn/Error 허용 테스트
   - 동시성 안전 테스트 (Subscribe/Unsubscribe 동시 호출)
   - Closed 상태 에러 반환 테스트
   - 외부 Observer 주입 테스트

**산출물**: Logger Agent의 핵심 기능 완성 (인터페이스, 생명주기, 옵션, 에러)

---

### Secondary Goal: Bridge Node Integration (P1)

**범위**: Module 4

**작업 항목**:

1. `logger_bridge.go` + `logger_bridge_test.go` 작성
   - `LoggerBridgeHandler` 구조체
   - `NewLoggerBridgeHandler(agent)` 생성자
   - `HandleMessage(ctx, msg)` 메시지 디스패처
   - `write` 연산 핸들러: component, level, message 추출 후 WriteLog 호출
   - `set_level` 연산 핸들러: component, level 추출 후 SetLevel 호출
   - `set_level_pattern` 연산 핸들러: pattern, level 추출 후 SetLevelByPattern 호출
   - `get_level` 연산 핸들러: component 추출 후 GetLevel 호출, 결과를 Payload에 포함
   - `subscribe` / `unsubscribe` 연산 핸들러
   - `components` 연산 핸들러: Components 호출, 결과를 Payload에 포함
   - 잘못된 연산 유형 에러 처리 테스트
   - 잘못된 레벨 문자열 에러 처리 테스트

**산출물**: Bridge Node를 통한 메시지 기반 Logger 접근 완성

---

## 3. 기술적 접근

### 3.1 Observer 래핑 전략

LoggerAgent는 Observer의 세 가지 하위 시스템을 래핑한다:

1. **LoggerFactory** (Observer.Loggers): 컴포넌트별 ComponentLogger 생성/조회
   - `WriteLog()`: `Loggers.NewLogger(component)`로 ComponentLogger를 획득 후 레벨에 맞는 메서드 호출
   - `Components()`: `Loggers.Components()` 직접 위임

2. **LevelManager** (Observer.Levels): 컴포넌트별 로그 레벨 관리
   - `SetLevel()`: `Levels.SetLevel(component, level)` 직접 위임
   - `SetLevelByPattern()`: `Levels.SetLevelByPattern(pattern, level)` 직접 위임
   - `GetLevel()`: `Levels.GetLevel(component)` 직접 위임

3. **StreamRouter** (Observer.Streams): 컴포넌트별 로그 스트림 라우팅
   - `Subscribe()`: `Streams.AddRoute(component, writer)` 호출 + 내부 구독 맵에 등록
   - `Unsubscribe()`: 구독 맵에서 구독 정보 조회 후 `Streams.RemoveRoute(component, writer)` 호출

### 3.2 구독 관리 전략

LoggerAgent는 StreamRouter에 직접 추가된 라우트를 추적하기 위해 별도의 구독 맵을 유지한다:

- 구독 맵: `map[string]*subscription` (구독 ID -> subscription)
- 구독 ID 생성: `fmt.Sprintf("sub-%d", atomic counter)` 또는 UUID 기반
- Subscribe 시: StreamRouter.AddRoute 호출 + 구독 맵에 등록
- Unsubscribe 시: 구독 맵에서 조회 -> StreamRouter.RemoveRoute 호출 -> 구독 맵에서 삭제
- Stop 시: 구독 맵의 모든 항목에 대해 RemoveRoute 호출 후 맵 초기화

구독 맵 보호: `sync.RWMutex` 사용
- Subscribe/Unsubscribe: Write Lock
- 구독 수 조회 (HealthCheck): Read Lock

### 3.3 Pause 레벨 필터링 전략

Pause 상태에서의 WriteLog 처리:

```
WriteLog(ctx, component, level, msg, args...):
  1. closed 확인 → ErrLoggerClosed 반환
  2. paused && level < slog.LevelWarn → nil 반환 (Debug/Info 억제)
  3. component 유효성 확인 → ErrInvalidComponent 반환
  4. Observer.Loggers.NewLogger(component) → ComponentLogger 획득
  5. level에 따라 Debug/Info/Warn/Error 호출
```

**주의**: Observer의 LevelManager 레벨을 변경하지 않는다. Pause/Resume은 LoggerAgent 레벨에서만 필터링한다. 이렇게 하면 Resume 시 Observer의 원래 레벨 설정이 그대로 보존된다.

### 3.4 Observer 생성/주입 전략

Init 시 Observer 결정 로직:

```
Init(ctx):
  1. config.observer != nil → 외부 주입된 Observer 사용
  2. config.observer == nil → observe.New() 호출하여 새 Observer 생성
     - WithObserverDefaultLevel(config.defaultLevel)
     - WithObserverFormat(config.format)
     - WithObserverWriter(config.defaultWriter) (nil이 아닌 경우)
```

### 3.5 생명주기 통합

LoggerAgent의 상태 전이:

```
Created -> Init() -> Initializing -> (Observer 초기화, 구독 맵 생성) -> Running
Running -> Pause() -> Paused (Warn/Error만 허용, 관리 연산은 정상)
Paused -> Resume() -> Running (전체 레벨 복원)
Running/Paused -> Stop() -> Stopping -> (모든 구독 해제, Observer 정리) -> Stopped
```

### 3.6 HealthCheck 상세

```go
HealthCheck(ctx):
  status := HealthStatus{}

  // Observer 유효성
  if l.observer == nil {
      status.Healthy = false
      status.Status = "unhealthy"
      return status
  }

  // Lifecycle 상태 기반 판별
  switch l.State() {
  case lifecycle.StateRunning:
      status.Healthy = true
      status.Status = "healthy"
  case lifecycle.StatePaused:
      status.Healthy = true  // degraded이지만 기능은 동작
      status.Status = "degraded"
  default:
      status.Healthy = false
      status.Status = "unhealthy"
  }

  // Details
  status.Details = map[string]any{
      "active_subscriptions": len(l.subs),
      "registered_components": len(l.observer.Loggers.Components()),
  }
```

---

## 4. 리스크 및 대응

### Risk 1: Observer 내부 상태와 LoggerAgent 상태 불일치

- **위험**: LoggerAgent가 Stopped 상태이지만 Observer 내부 리소스가 정리되지 않을 수 있음
- **대응**: Stop 시 Observer 참조를 nil로 설정하여 GC 대상으로 만듦. 외부 주입된 Observer의 경우 참조만 해제하고 소유권은 주입자에게 위임

### Risk 2: 구독 맵과 StreamRouter 불일치

- **위험**: StreamRouter에 직접 AddRoute한 외부 Writer와 LoggerAgent 구독 맵이 불일치할 수 있음
- **대응**: LoggerAgent.Subscribe/Unsubscribe로만 관리되는 구독만 추적. 외부에서 직접 StreamRouter를 조작하는 경우는 LoggerAgent의 관리 범위 밖

### Risk 3: Pause 상태에서 Warn/Error 로그 누락

- **위험**: Pause 필터링 로직의 레벨 비교 오류로 Warn/Error까지 억제될 수 있음
- **대응**: 명확한 레벨 비교 조건 (`level < slog.LevelWarn`이면 억제)으로 구현하고, 테스트에서 모든 레벨 조합을 검증

### Risk 4: 동시성 경합 (구독 맵 + Pause 플래그)

- **위험**: WriteLog에서 paused 플래그를 읽는 중에 Pause/Resume이 호출되면 데이터 레이스 발생 가능
- **대응**: paused/closed 플래그를 `sync.RWMutex` 또는 `atomic.Bool`로 보호. `go test -race`로 검증

### Risk 5: Bridge 메시지의 레벨 문자열 파싱 실패

- **위험**: "debug", "info", "warn", "error" 외의 문자열이 전달될 수 있음
- **대응**: Bridge 핸들러에서 레벨 문자열을 파싱하는 헬퍼 함수를 제공하고, 유효하지 않은 레벨은 `ErrInvalidLevel` 에러로 응답

---

## 5. 의존성 그래프

```
internal/agent/system/logger.go (본 SPEC)
  ├── 래핑: internal/observe/         (Observer, LoggerFactory, LevelManager, StreamRouter)
  ├── 의존: pkg/lifecycle/            (SPEC-LIFE-001: BaseLifecycle, HealthChecker)
  ├── 의존: pkg/message/              (SPEC-MSG-001: Bridge Node 연동 시 Message 타입)
  ├── 의존: 표준 라이브러리           (sync, io, log/slog, context, errors, fmt)
  ├── 소비자: internal/engine/        (Flow Runtime에서 LoggerAgent 초기화)
  ├── 소비자: internal/node/bridge.go (Bridge Node에서 Logger 연산 메시지 처리)
  └── 동료: internal/agent/system/event.go, store.go, file.go, timer.go (다른 System Agent)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | logger_errors.go | Sentinel 에러 정의 | 없음 |
| 2 | logger.go (인터페이스) | Logger 인터페이스, subscription 구조체 | logger_errors.go |
| 3 | logger_options.go | LoggerOption 타입, loggerConfig, 옵션 함수 | logger.go |
| 4 | logger.go (LoggerAgent) | LoggerAgent 생명주기 + Logger 구현 | 전체 (1-3) + pkg/lifecycle/ + internal/observe/ |
| 5 | logger_bridge.go | LoggerBridgeHandler 메시지 핸들러 | logger.go + pkg/message/ |

모든 파일에 대해 TDD 방식으로 테스트 파일(`*_test.go`)을 먼저 작성한다.

---

## 7. Store/Timer Agent와의 패턴 비교

| 관점 | Store Agent | Timer Agent | Logger Agent |
|------|-----------|-----------|-------------|
| 핵심 패턴 | 키-값 저장소 직접 구현 | 타이머 직접 관리 | **기존 observe 래핑** |
| 내부 상태 | sync.Map (데이터) | map + cron.Cron | **Observer 위임** |
| Pause 동작 | 쓰기 거부, 읽기 허용 | 등록 거부, 트리거 정지 | **Debug/Info 억제, Warn/Error 허용** |
| 구독 개념 | 없음 | 없음 | **스트림 구독 관리** |
| Bridge 패턴 | BridgeHandler | TimerBridgeHandler | **LoggerBridgeHandler** |
| 에러 수 | 8개 | 10개 | **5개** |
| Options 수 | 5개 | 3개 | **4개** |
