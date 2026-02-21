# SPEC-SYSAGENT-002: SystemAgentManager 5종 에이전트 통합 관리

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SYSAGENT-002 |
| 상태 | draft |
| 선행 SPEC | SPEC-SYSAGENT-001 |
| 생성일 | 2026-02-21 |
| 도메인 | internal/agent/system |

---

## 1. Environment (환경)

### 1.1 현재 상태

- `SystemAgentManager`는 Event + File 에이전트 2종만 관리한다 (SPEC-SYSAGENT-001 결과물).
- Logger, Timer, Store 에이전트는 각각 독립적으로 생성/시작/종료된다.
- `cmd/xflowd/main.go`는 `SystemAgentManager`를 사용하지 않고, `agent.NewManager()`를 통해 HTTP/ConsoleLogger/MQTT 타입만 등록한다.

### 1.2 아키텍처 제약

- **BaseAgent 기반** (Event, File): `agent.BaseAgent`를 임베딩하며, `Init(config agent.AgentConfig) error` 시그니처를 사용한다.
- **BaseLifecycle 기반** (Logger, Timer, Store): `*lifecycle.BaseLifecycle`를 임베딩하며, `Init(_ context.Context) error` 시그니처를 사용한다.
- BaseLifecycle 기반 에이전트의 `Init()`는 `Created -> Initializing -> Running` 상태 전이를 수행하므로, `Start()`는 이미 Running 상태에서 no-op이다.
- Logger, Timer, Store는 함수형 옵션 패턴으로 생성한다 (`NewLoggerAgent(opts ...LoggerOption)` 등).

### 1.3 기존 브릿지 핸들러

- `NewLoggerBridgeHandler(agent *LoggerAgent) *LoggerBridgeHandler`
- `NewTimerBridgeHandler(agent *TimerAgent) *TimerBridgeHandler`
- `NewBridgeHandler(agent *StoreAgent) *BridgeHandler` (Store)

---

## 2. Assumptions (가정)

- A1: 기존 5종 에이전트 구현체(Event, Logger, File, Timer, Store)는 변경하지 않는다.
- A2: `SystemAgentManager`는 인터페이스가 아닌 구조체(concrete struct)로 유지한다.
- A3: 모든 코드는 `go test -race`를 통과해야 한다.
- A4: 테스트 커버리지 85% 이상을 달성해야 한다.
- A5: BaseLifecycle 기반 에이전트의 `Init()` 호출 후 상태가 Running으로 전이되는 동작을 전제한다.
- A6: SPEC-SYSAGENT-001에서 정의된 Event + File 관리 로직은 보존한다.

---

## 3. Requirements (요구사항)

### Module 1: SystemConfig 확장

**REQ-SYSAGENT-002-001** (SystemConfig 필드 추가)

시스템은 **항상** 다음 필드를 `SystemConfig` 구조체에 포함해야 한다:

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `EventBufferSize` | `int` | 256 | 이벤트 채널 버퍼 크기 (기존) |
| `FileSandboxRoot` | `string` | `""` | 파일 에이전트 샌드박스 루트 (기존) |
| `StoreDefaultTTL` | `time.Duration` | `0` (무제한) | Store 에이전트 기본 TTL |
| `TimerMinInterval` | `time.Duration` | `100ms` | Timer 에이전트 최소 간격 |
| `LogDefaultLevel` | `LogLevel` | `LogLevelInfo` | Logger 에이전트 기본 로그 레벨 |

**REQ-SYSAGENT-002-002** (LogLevel 타입)

시스템은 **항상** `LogDefaultLevel` 필드에 타입 안전한 `LogLevel` 타입을 사용해야 한다. 기존 `string` 타입을 `LogLevel`로 교체한다.

---

### Module 2: SystemAgentManager 확장

**REQ-SYSAGENT-002-010** (에이전트 필드 추가)

시스템은 **항상** `SystemAgentManager` 구조체에 다음 3개 에이전트 필드를 포함해야 한다:
- `logger *LoggerAgent`
- `timer  *TimerAgent`
- `store  *StoreAgent`

**REQ-SYSAGENT-002-011** (Initialize 확장 - 에이전트 생성 및 초기화)

**WHEN** `Initialize(config SystemConfig)`이 호출되면 **THEN** 시스템은 다음 순서로 5종 에이전트를 생성 및 초기화해야 한다:

1. Event 에이전트: `NewEventAgent(config.EventBufferSize)` -> `Init(agent.AgentConfig{...})`
2. Store 에이전트: `NewStoreAgent(WithDefaultTTL(config.StoreDefaultTTL))` -> `Init(context.Background())`
3. Timer 에이전트: `NewTimerAgent(WithMinInterval(config.TimerMinInterval))` -> `Init(context.Background())`
4. Logger 에이전트: `NewLoggerAgent(WithLogLevel(config.LogDefaultLevel))` -> `Init(context.Background())`
5. File 에이전트: `NewFileAgent(config.FileSandboxRoot)` -> `Init(agent.AgentConfig{...})`

**REQ-SYSAGENT-002-012** (Initialize 롤백)

**IF** Initialize 과정에서 N번째 에이전트 초기화가 실패하면, **THEN** 시스템은 이미 초기화된 1~(N-1)번째 에이전트를 역순으로 `Stop()`하고 에러를 반환해야 한다.

**REQ-SYSAGENT-002-013** (Start 확장)

**WHEN** `Start(ctx context.Context)`가 호출되면 **THEN** 시스템은 다음 순서로 5종 에이전트를 시작해야 한다:

1. Event
2. Store
3. Timer
4. Logger
5. File

> 참고: BaseLifecycle 기반 에이전트(Store, Timer, Logger)의 `Start()`는 Init()에서 이미 Running 상태이므로 no-op이지만, 일관성을 위해 호출한다.

**REQ-SYSAGENT-002-014** (Start 롤백)

**IF** Start 과정에서 N번째 에이전트 시작이 실패하면, **THEN** 시스템은 이미 시작된 1~(N-1)번째 에이전트를 역순으로 `Stop()`하고 에러를 반환해야 한다.

**REQ-SYSAGENT-002-015** (Stop 역순)

**WHEN** `Stop(ctx context.Context)`가 호출되면 **THEN** 시스템은 다음 역순으로 에이전트를 종료해야 한다:

1. File
2. Logger
3. Timer
4. Store
5. Event

**REQ-SYSAGENT-002-016** (Stop 에러 수집)

**WHEN** Stop 과정에서 하나 이상의 에이전트 종료가 실패하면 **THEN** 시스템은 나머지 에이전트도 종료를 시도하고, 첫 번째 에러를 반환해야 한다.

**REQ-SYSAGENT-002-017** (접근자 메서드)

시스템은 **항상** 다음 접근자 메서드를 제공해야 한다:
- `Logger() *LoggerAgent` - 초기화 전이면 nil 반환
- `Timer() *TimerAgent` - 초기화 전이면 nil 반환
- `Store() *StoreAgent` - 초기화 전이면 nil 반환

각 접근자는 `sync.Mutex`로 보호되어야 한다.

---

### Module 3: 브릿지 핸들러 접근

**REQ-SYSAGENT-002-020** (브릿지 핸들러 팩토리)

**WHEN** 5종 에이전트가 초기화된 상태에서 **THEN** 시스템은 다음 메서드를 통해 브릿지 핸들러에 접근할 수 있어야 한다:
- `LoggerBridge() *LoggerBridgeHandler`
- `TimerBridge() *TimerBridgeHandler`
- `StoreBridge() *BridgeHandler`

**REQ-SYSAGENT-002-021** (브릿지 미초기화 방어)

**IF** 에이전트가 초기화되지 않은 상태에서 브릿지 메서드가 호출되면 **THEN** 시스템은 nil을 반환해야 한다.

---

### Module 4: main.go 통합

**REQ-SYSAGENT-002-030** (xflowd 서버 통합)

**WHEN** `xflowd` 서버가 시작되면 **THEN** `cmd/xflowd/main.go`는 다음 순서로 `SystemAgentManager`를 통합해야 한다:

1. `NewSystemAgentManager()` 생성
2. `SystemConfig` 구성 (설정 파일 또는 기본값)
3. `Initialize(config)` 호출
4. `Start(ctx)` 호출
5. 서버 종료 시 `Stop(ctx)` 호출

**REQ-SYSAGENT-002-031** (Observer 연결)

**WHEN** SystemAgentManager가 초기화되면 **THEN** Logger 에이전트는 Observer 인스턴스를 수신할 수 있어야 한다. SystemConfig에 `Observer *observe.Observer` 필드를 추가하거나, LoggerOption을 통해 전달한다.

---

### Module 5: 에러 처리

**REQ-SYSAGENT-002-040** (센티넬 에러 추가)

시스템은 **항상** 다음 센티넬 에러를 `manager_errors.go`에 정의해야 한다:

| 에러 | 설명 |
|------|------|
| `ErrAlreadyInitialized` | 이미 초기화된 상태 (기존) |
| `ErrNotInitialized` | 초기화 안 된 상태 (기존) |
| `ErrAgentTypeUnknown` | 알 수 없는 에이전트 타입 (기존) |
| `ErrAgentInitFailed` | 에이전트 초기화 실패 (신규) |
| `ErrAgentStartFailed` | 에이전트 시작 실패 (신규) |
| `ErrAgentStopFailed` | 에이전트 종료 실패 (신규) |

**REQ-SYSAGENT-002-041** (에러 래핑)

**WHEN** 에이전트 초기화/시작/종료가 실패하면 **THEN** 시스템은 센티넬 에러로 래핑하여 `fmt.Errorf("... %w", ErrAgentInitFailed)`와 같이 반환해야 한다. 이를 통해 `errors.Is()` 검사가 가능해야 한다.

---

## 4. Specifications (상세 설계)

### 4.1 수정 대상 파일

| 파일 | 변경 내용 |
|------|-----------|
| `internal/agent/system/system_manager.go` | SystemConfig 확장, SystemAgentManager 확장, Initialize/Start/Stop 5종 통합, 접근자 메서드 추가 |
| `internal/agent/system/system_manager_test.go` | 5종 에이전트 통합 테스트, 롤백 테스트, 브릿지 테스트 |
| `internal/agent/system/manager_errors.go` | 센티넬 에러 추가 |
| `cmd/xflowd/main.go` | SystemAgentManager 통합 |

### 4.2 Init 시그니처 차이 처리 전략

```
BaseAgent 기반 (Event, File):
  agent.Init(agent.AgentConfig{ID: "system-xxx", Name: "system-xxx-agent", Type: "xxx"})

BaseLifecycle 기반 (Logger, Timer, Store):
  agent.Init(context.Background())
```

SystemAgentManager의 `Initialize()` 내부에서 각 에이전트 타입에 맞는 Init 호출을 직접 수행한다. 별도 추상화 레이어 없이, Initialize() 메서드 내부에서 조건 분기 없이 순서대로 호출한다.

### 4.3 에이전트 순서 테이블

| 단계 | Initialize 순서 | Start 순서 | Stop 순서 |
|------|-----------------|------------|-----------|
| 1 | Event | Event | File |
| 2 | Store | Store | Logger |
| 3 | Timer | Timer | Timer |
| 4 | Logger | Logger | Store |
| 5 | File | File | Event |

### 4.4 Traceability

| 요구사항 ID | 관련 파일 | 관련 테스트 |
|-------------|-----------|-------------|
| REQ-SYSAGENT-002-001 | system_manager.go | TestSystemConfig_* |
| REQ-SYSAGENT-002-010~017 | system_manager.go | TestSystemAgentManager_* |
| REQ-SYSAGENT-002-020~021 | system_manager.go | TestSystemAgentManager_Bridge_* |
| REQ-SYSAGENT-002-030~031 | cmd/xflowd/main.go | (통합 테스트) |
| REQ-SYSAGENT-002-040~041 | manager_errors.go | TestErrors_* |
