---
id: SPEC-LOG-001
version: "1.0.0"
status: completed
created: "2026-02-15"
updated: "2026-02-15"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-15 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-15 | 1.0.0 | 구현 완료 (61 tests, 88.0% coverage) |

---

# SPEC-LOG-001: Logger System Agent - 로깅 시스템 에이전트

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 5개 System Agent(Event, Logger, File, Timer, Store) 중 하나인 Logger Agent를 정의한다. Logger Agent는 기존 `internal/observe` 패키지를 **래핑(wrapping)** 하여 System Agent 인터페이스를 제공하는 통합 로깅 서비스이다. `internal/observe`의 Observer, LoggerFactory, LevelManager, StreamRouter를 활용하며, 생명주기 관리와 Bridge Node 연동을 추가한다.

**핵심 설계 원칙: `internal/observe` 패키지를 대체하지 않고 래핑한다.**

본 SPEC은 다음을 다룬다:
- `Logger` 인터페이스 정의 (WriteLog, SetLevel, SetLevelByPattern, GetLevel, Subscribe, Unsubscribe, Components)
- `LoggerAgent` 구조체 (Observer 래핑, 생명주기 관리, 구독 관리)
- 옵션 시스템 (WithDefaultLevel, WithFormat, WithDefaultWriter, WithObserver)
- LoggerAgent 생명주기 (Lifecycle, HealthChecker 인터페이스 구현)
- Pause 시 Debug/Info 억제, Warn/Error 허용 (안전 우선 정책)
- Bridge Node 연동 (메시지 기반 로깅 제어)
- 에러 타입 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/system/logger.go` (구현), `internal/agent/system/logger_*_test.go` (테스트)
- **의존성**: 표준 라이브러리 (`sync`, `io`, `log/slog`, `context`, `errors`, `fmt`) + `pkg/lifecycle/` + `internal/observe/`
- **선택적 의존성**: `pkg/message/` (Bridge Node 연동 시)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **Tier**: internal (비공개 패키지, 외부 임포트 불가)

### 1.3 설계 원칙

- **래퍼 패턴**: `internal/observe`의 Observer를 래핑하여 System Agent 인터페이스를 제공. observe 패키지의 기능을 그대로 위임
- **동시성 안전**: 구독 관리(subscribe/unsubscribe)는 `sync.RWMutex`로 보호
- **안전 우선 Pause**: Pause 상태에서 Debug/Info는 억제하되 Warn/Error는 항상 허용 (안전 관련 로그 보장)
- **생명주기 통합**: `pkg/lifecycle/Lifecycle`, `HealthChecker` 인터페이스 구현
- **System Agent 패턴**: Transport/Protocol 설정 불필요, 시스템 시작 시 자동 활성화
- **최소 중복**: observe 패키지의 기존 기능을 재구현하지 않고 위임을 통해 활용

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `Logger` 인터페이스 정의 (WriteLog, SetLevel, SetLevelByPattern, GetLevel, Subscribe, Unsubscribe, Components)
- `LoggerAgent` 구조체 (Observer 래핑, 구독 맵 관리, 생명주기)
- 옵션 패턴 (WithDefaultLevel, WithFormat, WithDefaultWriter, WithObserver)
- Pause 시 레벨별 억제 로직 (Debug/Info 억제, Warn/Error 허용)
- Subscribe/Unsubscribe를 통한 동적 로그 스트림 라우팅
- Bridge Node 통한 메시지 기반 로깅 제어 패턴 정의
- 에러 타입 정의 (ErrLoggerClosed, ErrLoggerPaused, ErrInvalidLevel, ErrInvalidComponent, ErrSubscriptionNotFound)

**OUT OF SCOPE (별도 SPEC)**:
- `internal/observe` 패키지 자체 (이미 구현 완료, 86 tests, 95.7% coverage)
- Observer의 Metrics, Tracer 기능 (Logger Agent는 로깅/스트림만 래핑)
- Flow Engine 통합 (internal/engine/ - 별도 SPEC)
- Bridge Node 구현 자체 (internal/node/bridge.go - SPEC-FLOW-001 범위)
- 설정 파일 로딩 및 Viper 통합 (SPEC-CFG-001: internal/config/)
- REST API 핸들러 (별도 SPEC: internal/api/)
- 다른 System Agent 구현 (Event, File, Timer, Store)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | Logger Agent가 Lifecycle, HealthChecker 인터페이스를 구현 |
| SPEC-MSG-001 | 동료 | Bridge Node 통한 메시지 기반 접근 시 Message 타입 사용 |
| SPEC-STORE-001 | 동료 | 동일한 System Agent 패턴(BaseLifecycle 임베딩, Options 패턴) 공유 |
| SPEC-TIMER-001 | 동료 | 동일한 System Agent 패턴 공유, Bridge 핸들러 패턴 참조 |
| SPEC-FLOW-001 | 소비자 | 플로우가 Bridge Node 또는 직접 API로 Logger Agent 참조 |
| SPEC-OBS-001 | 래핑 대상 | Logger Agent가 internal/observe의 Observer를 래핑 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `internal/observe/Observer`는 이미 완성된 관찰성 시스템이며, Logger Agent는 이를 래핑하여 System Agent 인터페이스를 추가한다
- A2: `observe.LoggerFactory`는 `sync.Map` 기반으로 동시성 안전하며, `NewLogger(component)`는 멱등적이다
- A3: `observe.LevelManager`는 `slog.LevelVar`(atomic) 기반으로 동시성 안전하며, `SetLevel`/`SetLevelByPattern` 호출이 즉시 반영된다
- A4: `observe.StreamRouter`는 `sync.RWMutex` 기반으로 동시성 안전하며, `AddRoute`/`RemoveRoute`로 동적 Writer 라우팅을 지원한다
- A5: Logger Agent는 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 생명주기 로직을 재사용한다
- A6: Bridge Node 연동 시 요청/응답 패턴은 `pkg/message/Message`의 메타데이터에 연산 유형을 포함하여 전달한다
- A7: 구독(Subscribe) 시 `StreamRouter.AddRoute`로 Writer를 등록하며, 구독 ID는 UUID 또는 고유 문자열로 생성한다
- A8: 하나의 LoggerAgent 인스턴스가 전역으로 공유되며, 컴포넌트명으로 개별 로거를 식별한다

### 2.2 도메인 가정

- A9: Logger Agent는 시스템 시작 시 자동 활성화되며, 외부 연결 설정이 불필요하다
- A10: 기본 로그 레벨은 `slog.LevelInfo`이며, 런타임에 컴포넌트별로 변경 가능하다
- A11: 기본 로그 포맷은 "json"이며, "text" 포맷도 지원한다 (Observer 생성 시 결정)
- A12: Pause 상태에서 Debug/Info 로그를 억제하는 것은 노이즈 감소 목적이며, Warn/Error는 시스템 안전을 위해 항상 기록한다
- A13: Logger Agent의 Graceful Shutdown 시, 모든 구독을 해제하고 Observer 참조를 정리한다
- A14: Logger Agent의 HealthCheck는 Observer 유효성과 Lifecycle 상태를 기반으로 healthy/degraded/unhealthy를 판별한다

---

## 3. Requirements (요구사항)

### Module 1: Logger Interface - 로거 인터페이스 (P0)

#### REQ-LOG-001-01-01 (Ubiquitous) Logger 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Logger` 인터페이스를 제공해야 한다:

- `WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error` - 지정 컴포넌트로 로그 기록
- `SetLevel(ctx context.Context, component string, level slog.Level) error` - 컴포넌트별 로그 레벨 설정
- `SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error)` - 패턴 매칭 레벨 일괄 설정
- `GetLevel(ctx context.Context, component string) (slog.Level, error)` - 컴포넌트 로그 레벨 조회
- `Subscribe(ctx context.Context, component string, writer io.Writer) (string, error)` - 컴포넌트 로그 스트림 구독 (구독 ID 반환)
- `Unsubscribe(ctx context.Context, subscriptionID string) error` - 구독 해제
- `Components(ctx context.Context) ([]string, error)` - 등록된 컴포넌트 목록 조회

#### REQ-LOG-001-01-02 (Event-Driven) WriteLog 호출 시 Observer 위임

**WHEN** `Logger.WriteLog(ctx, component, level, msg, args...)` 호출 시, **THEN** Observer의 LoggerFactory에서 해당 컴포넌트의 ComponentLogger를 획득하고, 지정된 레벨의 로그 메서드(Debug/Info/Warn/Error)를 호출해야 한다.

#### REQ-LOG-001-01-03 (Event-Driven) SetLevel 호출 시 LevelManager 위임

**WHEN** `Logger.SetLevel(ctx, component, level)` 호출 시, **THEN** Observer의 LevelManager.SetLevel(component, level)을 호출하여 해당 컴포넌트의 로그 레벨을 즉시 변경해야 한다.

#### REQ-LOG-001-01-04 (Event-Driven) SetLevelByPattern 호출 시 패턴 매칭 위임

**WHEN** `Logger.SetLevelByPattern(ctx, pattern, level)` 호출 시, **THEN** Observer의 LevelManager.SetLevelByPattern(pattern, level)을 호출하여 패턴에 매칭되는 모든 컴포넌트의 레벨을 변경하고, 변경된 컴포넌트 수를 반환해야 한다.

#### REQ-LOG-001-01-05 (Event-Driven) Subscribe 호출 시 StreamRouter 라우트 추가

**WHEN** `Logger.Subscribe(ctx, component, writer)` 호출 시, **THEN** Observer의 StreamRouter.AddRoute(component, writer)를 호출하여 해당 컴포넌트의 로그 출력을 추가 Writer로 라우팅하고, 고유한 구독 ID를 반환해야 한다.

#### REQ-LOG-001-01-06 (Event-Driven) Unsubscribe 호출 시 StreamRouter 라우트 제거

**WHEN** `Logger.Unsubscribe(ctx, subscriptionID)` 호출 시, **THEN** 해당 구독 ID에 연결된 Writer를 StreamRouter.RemoveRoute로 제거해야 한다. 존재하지 않는 구독 ID에 대해서는 `ErrSubscriptionNotFound`를 반환해야 한다.

#### REQ-LOG-001-01-07 (Event-Driven) Components 호출 시 LoggerFactory 위임

**WHEN** `Logger.Components(ctx)` 호출 시, **THEN** Observer의 LoggerFactory.Components()를 호출하여 등록된 모든 컴포넌트 이름의 정렬된 목록을 반환해야 한다.

---

### Module 2: LoggerAgent Lifecycle - 로거 에이전트 생명주기 (P0)

#### REQ-LOG-001-02-01 (Ubiquitous) LoggerAgent 구조체

시스템은 **항상** `LoggerAgent` 구조체를 제공해야 한다. `LoggerAgent`는 다음 인터페이스를 구현한다:

- `pkg/lifecycle/Lifecycle` - 생명주기 관리 (Init, Start, Pause, Resume, Stop, State)
- `pkg/lifecycle/HealthChecker` - 헬스 체크 (HealthCheck)

#### REQ-LOG-001-02-02 (Ubiquitous) BaseLifecycle 임베딩

시스템은 **항상** `LoggerAgent`가 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 전이 로직을 재사용해야 한다.

#### REQ-LOG-001-02-03 (Event-Driven) Init 시 Observer 초기화

**WHEN** `LoggerAgent.Init(ctx)` 호출 시, **THEN** 옵션에 따라 Observer를 생성(또는 외부 주입된 Observer 사용)하고, 구독 맵을 초기화하고, Running 상태로 전이해야 한다.

#### REQ-LOG-001-02-04 (Event-Driven) Stop 시 Graceful Shutdown

**WHEN** `LoggerAgent.Stop(ctx)` 호출 시, **THEN** 모든 활성 구독을 해제(StreamRouter.RemoveRoute)하고, 구독 맵을 비우고, Observer 참조를 정리한 뒤, Stopped 상태로 전이해야 한다.

#### REQ-LOG-001-02-05 (Event-Driven) Pause 시 Debug/Info 억제

**WHEN** `LoggerAgent.Pause(ctx)` 호출 시, **THEN** Debug 및 Info 레벨 로그의 WriteLog 호출을 억제(nil 반환)해야 한다. Warn 및 Error 레벨 로그는 정상적으로 기록해야 한다.

#### REQ-LOG-001-02-06 (Event-Driven) Resume 시 전체 레벨 복원

**WHEN** `LoggerAgent.Resume(ctx)` 호출 시, **THEN** Pause 상태에서 억제되었던 Debug/Info 레벨 로그를 다시 허용하여 전체 레벨로 복원해야 한다.

#### REQ-LOG-001-02-07 (Ubiquitous) 자동 활성화

시스템은 **항상** 시스템 시작 시 LoggerAgent를 자동으로 생성하고 Init/Start하여 활성화해야 한다. 별도의 사용자 등록이 불필요하다.

#### REQ-LOG-001-02-08 (Ubiquitous) HealthCheck 구현

시스템은 **항상** `LoggerAgent.HealthCheck(ctx)` 호출 시 다음을 확인하여 `HealthStatus`를 반환해야 한다:

- Observer가 nil이 아닌지 확인
- Lifecycle 상태: Running = "healthy", Paused = "degraded", 기타 = "unhealthy"
- Details에 활성 구독 수, 등록된 컴포넌트 수 포함

---

### Module 3: Options System - 옵션 시스템 (P0)

#### REQ-LOG-001-03-01 (Ubiquitous) LoggerOption 함수 타입

시스템은 **항상** `LoggerOption func(*loggerConfig)` 함수 옵션 타입을 제공하고, `NewLoggerAgent(opts ...LoggerOption)` 생성자를 지원해야 한다.

#### REQ-LOG-001-03-02 (Ubiquitous) WithLogDefaultLevel 옵션

시스템은 **항상** `WithLogDefaultLevel(level slog.Level) LoggerOption`을 제공하여, Observer 생성 시 기본 로그 레벨을 설정할 수 있어야 한다. 기본값은 `slog.LevelInfo`이다.

#### REQ-LOG-001-03-03 (Ubiquitous) WithLogFormat 옵션

시스템은 **항상** `WithLogFormat(format string) LoggerOption`을 제공하여, 로그 출력 포맷을 "json" 또는 "text"로 설정할 수 있어야 한다. 기본값은 "json"이다.

#### REQ-LOG-001-03-04 (Ubiquitous) WithLogWriter 옵션

시스템은 **항상** `WithLogWriter(writer io.Writer) LoggerOption`을 제공하여, 기본 로그 출력 대상을 설정할 수 있어야 한다. 기본값은 `os.Stdout`이다.

#### REQ-LOG-001-03-05 (Ubiquitous) WithLogObserver 옵션

시스템은 **항상** `WithLogObserver(observer *observe.Observer) LoggerOption`을 제공하여, 외부에서 생성된 Observer를 주입할 수 있어야 한다. 주입된 Observer가 있으면 Init 시 새로 생성하지 않고 주입된 것을 사용한다.

---

### Module 4: Bridge Node Integration - 브릿지 노드 연동 (P1)

#### REQ-LOG-001-04-01 (Optional) Bridge Node 메시지 기반 로깅 제어

**가능하면** Logger Agent는 Bridge Node를 통해 메시지 기반으로 접근할 수 있어야 한다. 메시지의 메타데이터에 연산 유형을 포함한다:

- `logger.operation`: `write`, `set_level`, `set_level_pattern`, `get_level`, `subscribe`, `unsubscribe`, `components`
- `logger.component`: 대상 컴포넌트 이름
- `logger.level`: 로그 레벨 문자열 ("debug", "info", "warn", "error")
- `logger.message`: 로그 메시지 텍스트 (write 연산 시)
- `logger.pattern`: 패턴 문자열 (set_level_pattern 연산 시)

응답 메시지:
- `logger.status`: "ok" 또는 "error"
- `logger.error`: 에러 메시지 (status가 "error"일 때)

#### REQ-LOG-001-04-02 (Event-Driven) Write 메시지 처리

**WHEN** Bridge Node를 통해 `write` 연산 메시지를 수신하면, **THEN** 메시지에서 component, level, message를 추출하여 `WriteLog`를 호출하고, 결과를 응답 메시지로 반환해야 한다.

#### REQ-LOG-001-04-03 (Event-Driven) SetLevel 메시지 처리

**WHEN** Bridge Node를 통해 `set_level` 연산 메시지를 수신하면, **THEN** 메시지에서 component와 level을 추출하여 `SetLevel`을 호출하고, 결과를 응답 메시지로 반환해야 한다.

#### REQ-LOG-001-04-04 (Event-Driven) GetLevel 요청-응답

**WHEN** Bridge Node를 통해 `get_level` 연산 메시지를 수신하면, **THEN** 해당 컴포넌트의 로그 레벨을 조회하여 응답 메시지의 Payload에 레벨 문자열을 포함하여 반환해야 한다.

#### REQ-LOG-001-04-05 (Event-Driven) Components 요청-응답

**WHEN** Bridge Node를 통해 `components` 연산 메시지를 수신하면, **THEN** 등록된 컴포넌트 목록을 조회하여 응답 메시지의 Payload에 문자열 배열로 포함하여 반환해야 한다.

---

### Module 5: Error Types - 에러 타입 (P0)

#### REQ-LOG-001-05-01 (Ubiquitous) 표준 에러 변수

시스템은 **항상** 다음 에러 변수를 제공해야 한다:

| 에러 변수 | 용도 |
|-----------|------|
| `ErrLoggerClosed` | 중지된 Logger Agent에 대한 연산 시도 시 |
| `ErrLoggerPaused` | 일시정지된 Logger Agent에 대한 Debug/Info WriteLog 시도 시 |
| `ErrInvalidLevel` | 유효하지 않은 로그 레벨 지정 시 |
| `ErrInvalidComponent` | 빈 문자열 등 유효하지 않은 컴포넌트 이름 지정 시 |
| `ErrSubscriptionNotFound` | 존재하지 않는 구독 ID로 Unsubscribe 시도 시 |

#### REQ-LOG-001-05-02 (Ubiquitous) 에러 래핑 지원

시스템은 **항상** 모든 에러 변수가 `errors.Is()` 및 `errors.As()`와 호환되도록 sentinel error 패턴을 사용해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/agent/system/
  logger_errors.go       # 센티넬 에러 정의
  logger.go              # LoggerAgent 구조체, Logger 인터페이스
  logger_options.go      # LoggerOption 함수 타입 및 옵션 함수
  logger_bridge.go       # Bridge Node 메시지 핸들러

  logger_test.go         # LoggerAgent 통합 테스트
  logger_bridge_test.go  # Bridge Node 연동 테스트
  logger_options_test.go # Options 단위 테스트
```

### 4.2 타입 시그니처

```go
// Logger 는 로깅 시스템 에이전트 인터페이스이다.
type Logger interface {
    WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error
    SetLevel(ctx context.Context, component string, level slog.Level) error
    SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error)
    GetLevel(ctx context.Context, component string) (slog.Level, error)
    Subscribe(ctx context.Context, component string, writer io.Writer) (string, error)
    Unsubscribe(ctx context.Context, subscriptionID string) error
    Components(ctx context.Context) ([]string, error)
}

// subscription 은 로그 스트림 구독 정보를 나타내는 내부 구조체이다.
type subscription struct {
    id        string
    component string
    writer    io.Writer
}

// LoggerAgent 는 Logger System Agent이다.
// internal/observe 패키지의 Observer를 래핑하여 System Agent 인터페이스를 제공한다.
type LoggerAgent struct {
    *lifecycle.BaseLifecycle          // 임베딩
    observer *observe.Observer        // 래핑 대상
    config   loggerConfig             // 설정
    subs     map[string]*subscription // 구독 ID -> subscription
    mu       sync.RWMutex             // 구독 맵 보호
    paused   bool                     // Pause 상태 플래그
    closed   bool                     // Stop 상태 플래그
}

func NewLoggerAgent(opts ...LoggerOption) *LoggerAgent
func (l *LoggerAgent) Init(ctx context.Context) error
func (l *LoggerAgent) Start(ctx context.Context) error
func (l *LoggerAgent) Pause(ctx context.Context) error
func (l *LoggerAgent) Resume(ctx context.Context) error
func (l *LoggerAgent) Stop(ctx context.Context) error
func (l *LoggerAgent) State() lifecycle.State
func (l *LoggerAgent) HealthCheck(ctx context.Context) lifecycle.HealthStatus

// Logger 인터페이스 구현
func (l *LoggerAgent) WriteLog(ctx context.Context, component string, level slog.Level, msg string, args ...any) error
func (l *LoggerAgent) SetLevel(ctx context.Context, component string, level slog.Level) error
func (l *LoggerAgent) SetLevelByPattern(ctx context.Context, pattern string, level slog.Level) (int, error)
func (l *LoggerAgent) GetLevel(ctx context.Context, component string) (slog.Level, error)
func (l *LoggerAgent) Subscribe(ctx context.Context, component string, writer io.Writer) (string, error)
func (l *LoggerAgent) Unsubscribe(ctx context.Context, subscriptionID string) error
func (l *LoggerAgent) Components(ctx context.Context) ([]string, error)

// loggerConfig 는 LoggerAgent의 내부 설정을 담는 구조체이다.
type loggerConfig struct {
    defaultLevel  slog.Level       // 기본 로그 레벨 (default: slog.LevelInfo)
    format        string           // "json" 또는 "text" (default: "json")
    defaultWriter io.Writer        // 기본 출력 (default: os.Stdout)
    observer      *observe.Observer // 외부 주입 Observer (선택)
}

// Options Pattern
type LoggerOption func(*loggerConfig)

func WithLogDefaultLevel(level slog.Level) LoggerOption
func WithLogFormat(format string) LoggerOption
func WithLogWriter(writer io.Writer) LoggerOption
func WithLogObserver(observer *observe.Observer) LoggerOption

// LoggerBridgeHandler 는 Bridge Node를 통한 메시지 기반 Logger 접근을 처리한다.
type LoggerBridgeHandler struct {
    agent *LoggerAgent
}

func NewLoggerBridgeHandler(agent *LoggerAgent) *LoggerBridgeHandler
func (h *LoggerBridgeHandler) HandleMessage(ctx context.Context, msg message.Message) (message.Message, error)

// 에러 변수
var (
    ErrLoggerClosed        = errors.New("logger: logger agent is closed")
    ErrLoggerPaused        = errors.New("logger: logger agent is paused (debug/info suppressed)")
    ErrInvalidLevel        = errors.New("logger: invalid log level")
    ErrInvalidComponent    = errors.New("logger: invalid component name")
    ErrSubscriptionNotFound = errors.New("logger: subscription not found")
)
```

### 4.3 SPEC-LIFE-001과의 관계

Logger Agent는 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 상태 머신 로직을 재사용한다:

- `Init()` 호출 시 `BaseLifecycle.TransitionTo(StateInitializing)` 후 Observer 초기화(또는 외부 주입 사용), 구독 맵 생성, 성공 시 `TransitionTo(StateRunning)`
- `Stop()` 호출 시 `TransitionTo(StateStopping)` 후 모든 구독 해제, Observer 참조 정리, 완료 시 `TransitionTo(StateStopped)`
- `HealthCheck()` 호출은 상태와 무관하게 항상 가능

### 4.4 internal/observe와의 관계 (래핑 전략)

LoggerAgent는 Observer의 하위 시스템을 다음과 같이 위임한다:

| LoggerAgent 메서드 | 위임 대상 | 설명 |
|-------------------|----------|------|
| `WriteLog` | `Observer.Loggers.NewLogger(component).{Debug/Info/Warn/Error}` | 컴포넌트별 로그 기록 |
| `SetLevel` | `Observer.Levels.SetLevel(component, level)` | 컴포넌트별 레벨 설정 |
| `SetLevelByPattern` | `Observer.Levels.SetLevelByPattern(pattern, level)` | 패턴 매칭 레벨 설정 |
| `GetLevel` | `Observer.Levels.GetLevel(component)` | 컴포넌트 레벨 조회 |
| `Subscribe` | `Observer.Streams.AddRoute(component, writer)` | 로그 스트림 구독 |
| `Unsubscribe` | `Observer.Streams.RemoveRoute(component, writer)` | 로그 스트림 해제 |
| `Components` | `Observer.Loggers.Components()` | 등록된 컴포넌트 목록 |

### 4.5 SPEC-MSG-001과의 관계

Bridge Node 연동 시 `pkg/message/Message`의 Metadata를 활용하여 Logger 연산을 인코딩한다:

- 요청 메시지: `Metadata["logger.operation"]`, `Metadata["logger.component"]`, `Metadata["logger.level"]`, `Metadata["logger.message"]`, `Metadata["logger.pattern"]`
- 응답 메시지: `Payload["level"]` (GetLevel 결과), `Payload["components"]` (Components 결과), `Payload["count"]` (SetLevelByPattern 결과), `Metadata["logger.status"]` ("ok" 또는 "error")
- Correlation ID를 통한 요청-응답 매칭은 Bridge Node의 책임 (SPEC-FLOW-001)

### 4.6 Pause 동작 상세

```
Pause 상태에서의 WriteLog 동작:

  WriteLog(ctx, component, slog.LevelDebug, msg) → return nil (억제)
  WriteLog(ctx, component, slog.LevelInfo, msg)  → return nil (억제)
  WriteLog(ctx, component, slog.LevelWarn, msg)  → 정상 기록 (안전 로그)
  WriteLog(ctx, component, slog.LevelError, msg) → 정상 기록 (안전 로그)

SetLevel, GetLevel, Subscribe, Unsubscribe, Components → 정상 동작 (관리 연산은 Pause 영향 없음)
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-LOG-001-01-01 ~ 01-07 | Logger Interface | logger.go | P0 |
| REQ-LOG-001-02-01 ~ 02-08 | LoggerAgent Lifecycle | logger.go | P0 |
| REQ-LOG-001-03-01 ~ 03-05 | Options System | logger_options.go | P0 |
| REQ-LOG-001-04-01 ~ 04-05 | Bridge Node Integration | logger_bridge.go | P1 |
| REQ-LOG-001-05-01 ~ 05-02 | Error Types | logger_errors.go | P0 |
