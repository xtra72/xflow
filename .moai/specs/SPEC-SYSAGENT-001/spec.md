---
id: SPEC-SYSAGENT-001
version: "1.5.0"
status: completed
created: "2026-02-13"
updated: "2026-03-30"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |
| 2026-03-17 | 1.1.0 | console-logger 에이전트 확장: 출력 대상(stdout/stderr/file), 포맷(text/json), 롤링 파일(RollingWriter) 설정 추가. internal/io/rollingwriter.go 신규 |
| 2026-03-27 | 1.2.0 | Agent Type 리네이밍(console-logger → logger), MessagePublisher 인터페이스 구현(토픽별 파일 출력), Process() 로깅 레벨 Debug→Info 변경, 테스트 7건 추가 |
| 2026-03-27 | 1.3.0 | 모든 시스템 에이전트 Start() 메서드에 Stopped 상태 복구 로직 추가 (Stopped→Created→Init() 재초기화), HTTPReceiverAgent 신규 시스템 에이전트 구현 |
| 2026-03-30 | 1.4.0 | Store Agent namespace 전파 수정: namespaceWriter 인터페이스 도입(store_namespace.go), NamespacedStore Set/SetWithTTL 후 storeItem.namespace 필드 자동 설정, agentStore/VolatileStore setItemNamespace() 구현. StatefulAgent State() 강화: summary/full 양쪽에서 호출(entries는 full만), 네임스페이스 접두사 제거한 displayKey 표시, 만료 항목 필터링, 키 정렬. UserStoreAgent 웹 UI 연동 완료 |
| 2026-03-30 | 1.5.0 | Store 노드 어댑터 추가(store_node_adapter.go: NodeStoreAdapter로 StoreWriter+StoreReader 인터페이스 구현), Store 에이전트 타입 등록(store_register.go: RegisterStoreAgent 팩토리), main.go Store 에이전트 등록, agent_adapter Store 상태 조회 지원 |

---

# SPEC-SYSAGENT-001: System Agent 구현 - Event, Logger, File, Timer, Store 내장 서비스

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼에서 System Agent는 별도의 Transport/Protocol 설정 없이 노드나 다른 에이전트에서 바로 사용할 수 있는 내장 서비스이다. 시스템 시작 시 자동으로 활성화되며, Bridge Node 없이도 직접 참조 가능하다.

본 SPEC은 SPEC-AGENT-001에서 정의한 `SystemAgent` 인터페이스의 **구체 구현체 5종**을 정의한다:

- **Event Agent** (`event.go`): Go 채널 기반 Pub/Sub 시스템 이벤트 발행/구독
- **Logger Agent** (`logger.go`): slog + observe 패키지 연동 로그 관리
- **File Agent** (`file.go`): os + fsnotify 기반 파일 읽기/쓰기/감시
- **Timer Agent** (`timer.go`): time.Ticker + robfig/cron 기반 타이머/스케줄러
- **Store Agent** (`store.go`): sync.Map + 선택적 DB 백엔드 키-값 저장소
- **System Agent Manager** (`manager.go`): 5종 System Agent의 일괄 생성/시작/중지 관리
- **Error Types** (`errors.go`): 본 패키지 전용 sentinel 에러 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/system/`
- **Tier**: Tier 2 - 내부 실행 계층 (internal)
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-001): BaseAgent, Agent 인터페이스, SystemAgent 인터페이스, Manager, Registry, TypeRegistry, AgentConfig, AgentStats, AgentInfo
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 인터페이스, 생명주기 상태 관리
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스 (이벤트 메시지 래핑)
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent 타입
  - `internal/observe/` (SPEC-OBS-001): 컴포넌트별 로거 팩토리, 메트릭
  - `pkg/config/` (SPEC-CFG-001): 설정 관리 (Store 백엔드 설정)
  - `internal/engine/` (SPEC-ENGINE-001): Flow Engine 이벤트 연동
  - `pkg/store/` (SPEC-STORE-001): 저장소 백엔드 인터페이스
- **소비 패키지**:
  - `internal/node/` (SPEC-NODE-001): 노드에서 System Agent 직접 참조
  - `internal/script/` (SPEC-SCRIPT-001): Lua 바인딩을 통한 System Agent 접근
  - `internal/engine/` (SPEC-ENGINE-001): Flow Engine이 System Agent를 자동 생성/관리
  - `internal/plugin/` (SPEC-PLUGIN-001): Plugin에서 Go 인터페이스/WASM ABI로 접근
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **외부 의존성**:
  - `github.com/fsnotify/fsnotify` (File Agent 디렉토리 감시)
  - `github.com/robfig/cron/v3` (Timer Agent cron 스케줄링)
  - `sync` 패키지 (`sync.Map` - Store Agent, `sync.RWMutex` - Event Agent 구독자 관리)

### 1.3 설계 원칙

- **SystemAgent 인터페이스 준수**: 모든 System Agent는 SPEC-AGENT-001의 `SystemAgent` 인터페이스를 구현하며, `IsSystem()` = true, `RequiresTransport()` = false를 반환
- **BaseAgent 임베딩**: 공통 로직(생명주기, 상태 관리, 통계 수집)을 재사용하기 위해 `BaseAgent`를 임베딩
- **TypeRegistry 자동 등록**: 각 System Agent는 `init()` 함수에서 `TypeRegistry.RegisterType()`으로 자동 등록
- **이중 접근 모드**: 직접 참조(Go 인터페이스, Lua 바인딩, WASM ABI)와 선택적 Bridge Node 경유를 모두 지원
- **동시성 안전**: 모든 System Agent는 goroutine-safe하게 설계 (sync.Map, sync.RWMutex, atomic, 채널 사용)
- **Graceful Shutdown**: 모든 System Agent는 context 취소 시 정상 종료를 보장
- **관찰성 내장**: 이벤트 발행/구독, 로그 작성, 파일 감시, 타이머 트리거, 스토어 접근 등 모든 활동이 추적 가능
- **제로 설정**: System Agent는 외부 연결이 없으므로 Transport/Protocol 설정이 불필요

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Event Agent 구현 (Go 채널 기반 Pub/Sub, 토픽 매칭, 다중 구독자)
- Logger Agent 구현 (slog 연동, 로그 레벨 동적 변경, 로그 스트림 구독)
- File Agent 구현 (파일 읽기/쓰기, fsnotify 디렉토리 감시, 샌드박스 경로 제한)
- Timer Agent 구현 (time.Ticker 주기 실행, robfig/cron 스케줄, 지연/반복 실행)
- Store Agent 구현 (sync.Map 기반 키-값 저장, TTL 지원, 네임스페이스 격리)
- System Agent Manager (일괄 생성/시작/중지)
- Error Types (패키지 전용 sentinel 에러 정의)
- System Agent 정보/통계 확장 (각 Agent별 특화 통계)

**OUT OF SCOPE (다른 SPEC 범위)**:
- Agent 프레임워크 (SPEC-AGENT-001): BaseAgent, Agent/SystemAgent 인터페이스, Manager, Registry
- 표준 Agent 구현 (SPEC-SAGENT-001): MQTT, HTTP, WebSocket, gRPC, Samsung NASA
- Bridge Node (SPEC-NODE-001): Bridge Node를 통한 Agent 연결
- Lua 바인딩 (SPEC-SCRIPT-001): Script Engine의 System Agent Lua 바인딩
- 관찰성 인프라 (SPEC-OBS-001): slog 로거 팩토리, 메트릭 수집기
- 저장소 백엔드 (SPEC-STORE-001): DB 기반 영속 저장소 구현

### 1.5 접근 방식

System Agent는 두 가지 방식으로 접근 가능하다:

**직접 참조 (Bridge Node 불필요)**:
- 노드 코드에서 System Agent의 Go 인터페이스를 직접 호출
- Script 노드에서 Lua 바인딩을 통해 `xflow.event.emit("name", data)` 형태로 호출
- Plugin에서 Go 인터페이스 또는 WASM ABI를 통해 접근

**Bridge Node 경유 (선택적)**:
- 플로우 에디터에서 System Agent를 Bridge Node로 연결하여 시각적 구성
- 이벤트 구독, 타이머 트리거 등을 플로우 그래프에서 명시적으로 표현

---

## 2. Assumptions (가정)

### 2.1 프레임워크 가정

- SPEC-AGENT-001의 `SystemAgent` 인터페이스, `BaseAgent` 구현체, `TypeRegistry`가 사용 가능하다
- `BaseAgent`는 `Init()`, `Start()`, `Stop()`, `Pause()`, `Resume()`, `Health()`, `Process()`, `Configure()`, `ID()`, `Name()`, `Type()`, `Info()`, `Stats()` 메서드의 기본 구현을 제공한다
- `TypeRegistry.RegisterType()`을 통해 `init()` 시점에 팩토리 함수를 등록할 수 있다

### 2.2 의존 패키지 가정

- `pkg/lifecycle/` (SPEC-LIFE-001)의 State 타입과 상태 전이가 구현되어 있다
- `pkg/message/` (SPEC-MSG-001)의 Message 인터페이스가 구현되어 있다
- `pkg/xferr/` (SPEC-ERR-001)의 ErrorMessage, StatusEvent 타입이 구현되어 있다
- `internal/observe/` (SPEC-OBS-001)의 컴포넌트별 로거 팩토리가 사용 가능하다

### 2.3 런타임 가정

- System Agent는 시스템 시작 시 자동으로 생성/시작되며, 시스템 종료 시 자동으로 중지된다
- System Agent는 싱글톤으로 동작한다 (각 타입별 하나의 인스턴스만 존재)
- 모든 System Agent는 동일 프로세스 내에서 실행되며, 프로세스 간 통신은 지원하지 않는다

### 2.4 보안 가정

- File Agent는 샌드박스 경로 내에서만 파일 접근이 허용된다
- Store Agent는 네임스페이스로 데이터를 격리한다
- Timer Agent는 최소 실행 간격(100ms)을 강제하여 리소스 남용을 방지한다

---

## 3. Requirements (요구사항)

### Module 1: Event Agent (P0)

Event Agent는 Go 채널 기반 Pub/Sub 시스템으로, 플로우 상태 변경, Agent 연결/해제, 에러 발생 등 내부 이벤트를 발행하고 구독한다.

#### REQ-SYSAGENT-001-01-01: EventAgent 인터페이스

시스템은 **항상** 다음 메서드를 가진 `EventAgent` 인터페이스를 제공해야 한다:

```go
type EventAgent interface {
    SystemAgent
    Emit(topic string, data interface{}) error
    Subscribe(topic string, handler EventHandler) (SubscriptionID, error)
    SubscribePattern(pattern string, handler EventHandler) (SubscriptionID, error)
    Unsubscribe(id SubscriptionID) error
    Topics() []string
    SubscriberCount(topic string) int
}

type EventHandler func(event Event)

type Event struct {
    Topic     string
    Data      interface{}
    Timestamp time.Time
    Source    string
}

type SubscriptionID string
```

#### REQ-SYSAGENT-001-01-02: 이벤트 발행

**WHEN** `Emit(topic, data)`가 호출되면 **THEN** 해당 토픽의 모든 구독자에게 Event를 비동기 전달해야 한다.

- 구독자가 없는 토픽에 발행 시 에러 없이 무시한다
- Event에 Timestamp와 Source를 자동으로 포함한다
- 발행은 non-blocking으로 동작한다 (채널 버퍼 활용)

#### REQ-SYSAGENT-001-01-03: 토픽 구독

**WHEN** `Subscribe(topic, handler)`가 호출되면 **THEN** 해당 토픽에 핸들러를 등록하고 고유한 `SubscriptionID`를 반환해야 한다.

- 동일 토픽에 다중 구독자를 등록할 수 있다
- 구독은 goroutine-safe해야 한다

#### REQ-SYSAGENT-001-01-04: 패턴 구독

**WHEN** `SubscribePattern(pattern, handler)`가 호출되면 **THEN** 패턴과 매칭되는 모든 토픽의 이벤트를 수신해야 한다.

- 와일드카드 패턴 지원: `*` (단일 레벨), `**` (다중 레벨)
- 예: `flow.*` 패턴은 `flow.started`, `flow.stopped` 등에 매칭
- 예: `agent.**` 패턴은 `agent.connected`, `agent.mqtt.error` 등에 매칭

#### REQ-SYSAGENT-001-01-05: 구독 해제

**WHEN** `Unsubscribe(id)`가 호출되면 **THEN** 해당 구독을 제거하고 더 이상 이벤트를 수신하지 않아야 한다.

- 존재하지 않는 SubscriptionID로 해제 시 `ErrSubscriptionNotFound` 에러를 반환한다

#### REQ-SYSAGENT-001-01-06: 이벤트 버퍼링

시스템은 **항상** 이벤트 전달에 버퍼링된 채널을 사용해야 한다.

- 기본 버퍼 크기: 256
- 버퍼가 가득 차면 오래된 이벤트를 드롭하고 메트릭을 기록한다
- 버퍼 크기는 System Agent Manager를 통해 설정 가능하다

#### REQ-SYSAGENT-001-01-07: 내장 이벤트 토픽

시스템은 **항상** 다음 내장 이벤트 토픽을 제공해야 한다:

| 토픽 | 발생 시점 | Data 타입 |
|------|----------|----------|
| `flow.started` | 플로우 시작 | FlowID string |
| `flow.stopped` | 플로우 중지 | FlowID string |
| `flow.error` | 플로우 에러 | FlowError struct |
| `agent.connected` | Agent 연결 | AgentID string |
| `agent.disconnected` | Agent 연결 해제 | AgentID string |
| `agent.error` | Agent 에러 | AgentError struct |
| `node.status` | 노드 상태 변경 | NodeStatus struct |
| `system.error` | 시스템 에러 | error |

#### REQ-SYSAGENT-001-01-08: EventAgent 통계

**WHEN** `Stats()`가 호출되면 **THEN** 기본 AgentStats에 추가로 이벤트 특화 통계를 제공해야 한다:

- `EventsEmitted`: 발행된 총 이벤트 수
- `EventsDelivered`: 전달 성공한 총 이벤트 수
- `EventsDropped`: 버퍼 초과로 드롭된 이벤트 수
- `ActiveSubscriptions`: 현재 활성 구독 수
- `ActiveTopics`: 현재 구독자가 있는 토픽 수

---

### Module 2: Store Agent (P0)

Store Agent는 sync.Map 기반 키-값 저장소로, 플로우 간 공유 데이터를 저장하고 조회한다.

#### REQ-SYSAGENT-001-02-01: StoreAgent 인터페이스

시스템은 **항상** 다음 메서드를 가진 `StoreAgent` 인터페이스를 제공해야 한다:

```go
type StoreAgent interface {
    SystemAgent
    Get(namespace, key string) (interface{}, bool)
    Set(namespace, key string, value interface{}) error
    SetWithTTL(namespace, key string, value interface{}, ttl time.Duration) error
    Delete(namespace, key string) error
    Keys(namespace string) []string
    Clear(namespace string) error
    Watch(namespace, key string, handler WatchHandler) (WatchID, error)
    Unwatch(id WatchID) error
}

type WatchHandler func(event WatchEvent)

type WatchEvent struct {
    Namespace string
    Key       string
    OldValue  interface{}
    NewValue  interface{}
    Type      WatchEventType
    Timestamp time.Time
}

type WatchEventType string

const (
    WatchEventSet    WatchEventType = "set"
    WatchEventDelete WatchEventType = "delete"
    WatchEventExpire WatchEventType = "expire"
)

type WatchID string
```

#### REQ-SYSAGENT-001-02-02: 키-값 CRUD

**WHEN** `Set(namespace, key, value)`가 호출되면 **THEN** 해당 네임스페이스의 키에 값을 저장해야 한다.

- `Get()`은 키가 존재하면 `(value, true)`, 없으면 `(nil, false)`를 반환한다
- `Delete()`는 키를 삭제하며, 존재하지 않는 키 삭제 시 에러 없이 무시한다
- `Keys()`는 네임스페이스 내 모든 키 목록을 반환한다
- `Clear()`는 네임스페이스 내 모든 키를 삭제한다

#### REQ-SYSAGENT-001-02-03: TTL 지원

**WHEN** `SetWithTTL(namespace, key, value, ttl)`이 호출되면 **THEN** TTL 만료 후 자동으로 키를 삭제해야 한다.

- TTL이 0이면 영구 저장 (만료 없음)
- 만료된 키는 백그라운드 goroutine에서 주기적으로 정리한다 (lazy + periodic 방식)
- 만료 시 Watch 구독자에게 `WatchEventExpire` 이벤트를 발행한다

#### REQ-SYSAGENT-001-02-04: 네임스페이스 격리

시스템은 **항상** 네임스페이스별로 데이터를 격리해야 한다.

- 서로 다른 네임스페이스의 동일 키는 독립적인 값을 가진다
- 빈 네임스페이스 문자열(`""`)은 기본 네임스페이스 `"default"`로 매핑한다
- 네임스페이스명 유효성 검증: 영문, 숫자, 하이픈, 밑줄만 허용

#### REQ-SYSAGENT-001-02-05: Watch (변경 감시)

**WHEN** `Watch(namespace, key, handler)`가 호출되면 **THEN** 해당 키의 값 변경 시 핸들러를 호출해야 한다.

- `Set`, `Delete`, TTL 만료 시 각각 `WatchEventSet`, `WatchEventDelete`, `WatchEventExpire` 이벤트 발행
- `OldValue`와 `NewValue`를 포함하여 변경 전후 값을 추적 가능
- key에 와일드카드 `"*"` 사용 시 네임스페이스 내 모든 키 변경을 감시

#### REQ-SYSAGENT-001-02-06: 동시성 안전

시스템은 **항상** `sync.Map`을 사용하여 동시성 안전을 보장해야 한다.

- 읽기/쓰기 경합 없이 다중 goroutine에서 동시 접근 가능
- Watch 핸들러 호출은 별도 goroutine에서 비동기 실행

#### REQ-SYSAGENT-001-02-07: StoreAgent 통계

**WHEN** `Stats()`가 호출되면 **THEN** 기본 AgentStats에 추가로 스토어 특화 통계를 제공해야 한다:

- `TotalKeys`: 전체 키 수
- `NamespaceCount`: 네임스페이스 수
- `GetHits`: Get 성공 횟수
- `GetMisses`: Get 실패 횟수
- `ExpiredKeys`: TTL 만료로 삭제된 키 수
- `ActiveWatchers`: 활성 Watch 구독 수

---

### Module 3: Timer Agent (P0)

Timer Agent는 time.Ticker와 robfig/cron을 사용하여 주기적 실행, cron 스케줄, 지연 실행, 반복 실행을 지원한다.

#### REQ-SYSAGENT-001-03-01: TimerAgent 인터페이스

시스템은 **항상** 다음 메서드를 가진 `TimerAgent` 인터페이스를 제공해야 한다:

```go
type TimerAgent interface {
    SystemAgent
    SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error)
    SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error)
    SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error)
    Cancel(id TimerID) error
    List() []TimerInfo
}

type TimerHandler func(trigger TimerTrigger)

type TimerTrigger struct {
    TimerID    TimerID
    TriggerAt  time.Time
    TickCount  int64
    ScheduleID string
}

type TimerID string

type TimerInfo struct {
    ID         TimerID
    Type       TimerType
    Expression string
    NextFire   time.Time
    LastFired  time.Time
    TickCount  int64
    Active     bool
}

type TimerType string

const (
    TimerTypeInterval TimerType = "interval"
    TimerTypeCron     TimerType = "cron"
    TimerTypeTimeout  TimerType = "timeout"
)
```

#### REQ-SYSAGENT-001-03-02: 주기적 실행 (Interval)

**WHEN** `SetInterval(id, interval, handler)`가 호출되면 **THEN** 지정된 간격으로 핸들러를 반복 호출해야 한다.

- `time.Ticker`를 사용하여 정확한 주기 보장
- 최소 간격: 100ms (이하 시 `ErrIntervalTooShort` 에러)
- 각 트리거에 `TickCount`를 증가시켜 실행 횟수를 추적

#### REQ-SYSAGENT-001-03-03: Cron 스케줄

**WHEN** `SetCron(id, cronExpr, handler)`가 호출되면 **THEN** cron 표현식에 따라 핸들러를 호출해야 한다.

- `robfig/cron/v3` 라이브러리 사용
- 표준 5필드 cron 표현식 지원 (`분 시 일 월 요일`)
- 초 단위 확장 6필드 표현식도 지원 (옵션)
- 잘못된 cron 표현식 시 `ErrInvalidCronExpression` 에러

#### REQ-SYSAGENT-001-03-04: 지연 실행 (Timeout)

**WHEN** `SetTimeout(id, delay, handler)`가 호출되면 **THEN** 지정된 시간 후 핸들러를 1회만 호출해야 한다.

- `time.AfterFunc`을 사용하여 구현
- 실행 후 자동으로 타이머 목록에서 제거
- 최소 지연: 1ms

#### REQ-SYSAGENT-001-03-05: 타이머 취소

**WHEN** `Cancel(id)`가 호출되면 **THEN** 해당 타이머를 중지하고 제거해야 한다.

- 존재하지 않는 TimerID로 취소 시 `ErrTimerNotFound` 에러
- 취소된 타이머의 핸들러는 더 이상 호출되지 않는다
- 이미 실행된 Timeout 타이머 취소 시 에러 없이 무시

#### REQ-SYSAGENT-001-03-06: Graceful Shutdown

**WHEN** System Agent가 Stop되면 **THEN** 모든 활성 타이머를 취소하고 리소스를 정리해야 한다.

- 실행 중인 핸들러는 완료까지 대기 (context 기반 타임아웃)
- cron 스케줄러 종료 (`cron.Stop()`)
- 모든 Ticker 정리

#### REQ-SYSAGENT-001-03-07: TimerAgent 통계

**WHEN** `Stats()`가 호출되면 **THEN** 기본 AgentStats에 추가로 타이머 특화 통계를 제공해야 한다:

- `ActiveTimers`: 현재 활성 타이머 수
- `TotalTriggers`: 총 트리거 실행 횟수
- `IntervalTimers`: Interval 타입 타이머 수
- `CronTimers`: Cron 타입 타이머 수
- `TimeoutTimers`: Timeout 타입 타이머 수

---

### Module 4: Error Types (P0)

본 패키지 전용 sentinel 에러를 정의한다.

#### REQ-SYSAGENT-001-04-01: Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러를 제공해야 한다:

```go
var (
    // Event Agent
    ErrSubscriptionNotFound   = errors.New("system/event: subscription not found")
    ErrTopicEmpty             = errors.New("system/event: topic cannot be empty")
    ErrHandlerNil             = errors.New("system/event: handler cannot be nil")
    ErrInvalidPattern         = errors.New("system/event: invalid subscription pattern")
    ErrBufferFull             = errors.New("system/event: event buffer is full")

    // Store Agent
    ErrNamespaceInvalid       = errors.New("system/store: invalid namespace name")
    ErrKeyEmpty               = errors.New("system/store: key cannot be empty")
    ErrWatchNotFound          = errors.New("system/store: watch not found")
    ErrTTLNegative            = errors.New("system/store: TTL cannot be negative")

    // Timer Agent
    ErrIntervalTooShort       = errors.New("system/timer: interval below minimum (100ms)")
    ErrInvalidCronExpression  = errors.New("system/timer: invalid cron expression")
    ErrTimerNotFound          = errors.New("system/timer: timer not found")
    ErrTimerIDEmpty           = errors.New("system/timer: timer ID cannot be empty")

    // File Agent
    ErrPathOutsideSandbox     = errors.New("system/file: path outside sandbox directory")
    ErrSandboxNotConfigured   = errors.New("system/file: sandbox directory not configured")
    ErrFileNotFound           = errors.New("system/file: file not found")
    ErrWatchPathInvalid       = errors.New("system/file: watch path is invalid")

    // Logger Agent
    ErrInvalidLogLevel        = errors.New("system/logger: invalid log level")
    ErrComponentEmpty         = errors.New("system/logger: component name cannot be empty")
    ErrStreamClosed           = errors.New("system/logger: log stream is closed")

    // Manager
    ErrAgentTypeUnknown       = errors.New("system: unknown system agent type")
    ErrAlreadyInitialized     = errors.New("system: system agents already initialized")
    ErrNotInitialized         = errors.New("system: system agents not initialized")
)
```

#### REQ-SYSAGENT-001-04-02: errors.Is() 호환성

시스템은 **항상** 모든 sentinel 에러가 `errors.Is()` 및 `fmt.Errorf("%w")` 래핑과 호환되어야 한다.

---

### Module 5: File Agent (P1)

File Agent는 로컬 파일 읽기/쓰기와 fsnotify 기반 디렉토리 감시를 제공한다.

#### REQ-SYSAGENT-001-05-01: FileAgent 인터페이스

시스템은 **항상** 다음 메서드를 가진 `FileAgent` 인터페이스를 제공해야 한다:

```go
type FileAgent interface {
    SystemAgent
    Read(path string) ([]byte, error)
    Write(path string, data []byte, perm os.FileMode) error
    Append(path string, data []byte) error
    Exists(path string) bool
    Remove(path string) error
    List(dir string) ([]FileInfo, error)
    WatchDir(dir string, handler FileEventHandler) (FileWatchID, error)
    UnwatchDir(id FileWatchID) error
}

type FileEventHandler func(event FileEvent)

type FileEvent struct {
    Path      string
    Op        FileOp
    Timestamp time.Time
}

type FileOp string

const (
    FileOpCreate FileOp = "create"
    FileOpWrite  FileOp = "write"
    FileOpRemove FileOp = "remove"
    FileOpRename FileOp = "rename"
    FileOpChmod  FileOp = "chmod"
)

type FileWatchID string

type FileInfo struct {
    Name    string
    Size    int64
    IsDir   bool
    ModTime time.Time
    Perm    os.FileMode
}
```

#### REQ-SYSAGENT-001-05-02: 샌드박스 경로 제한

시스템은 **항상** 파일 접근을 샌드박스 디렉토리 내로 제한해야 한다.

- 모든 경로는 `filepath.Clean()` + `filepath.Abs()`로 정규화 후 샌드박스 루트 하위인지 검증
- 샌드박스 외부 접근 시 `ErrPathOutsideSandbox` 에러
- 심볼릭 링크를 따라간 후의 실제 경로도 샌드박스 내에 있어야 한다 (`filepath.EvalSymlinks()`)
- 샌드박스 루트는 System Agent Manager를 통해 설정

#### REQ-SYSAGENT-001-05-03: 파일 읽기/쓰기

**WHEN** `Read(path)`가 호출되면 **THEN** 파일 전체 내용을 바이트 슬라이스로 반환해야 한다.
**WHEN** `Write(path, data, perm)`이 호출되면 **THEN** 파일을 생성하거나 덮어쓰기해야 한다.
**WHEN** `Append(path, data)`가 호출되면 **THEN** 기존 파일 끝에 데이터를 추가해야 한다.

- 파일이 존재하지 않으면 `ErrFileNotFound` 에러 (Read, Append 시)
- 쓰기 시 중간 디렉토리가 없으면 자동 생성 (`os.MkdirAll`)

#### REQ-SYSAGENT-001-05-04: 디렉토리 감시

**WHEN** `WatchDir(dir, handler)`가 호출되면 **THEN** fsnotify를 사용하여 디렉토리 내 파일 변경을 감시해야 한다.

- `github.com/fsnotify/fsnotify` 사용
- Create, Write, Remove, Rename, Chmod 이벤트를 감지
- 재귀적 하위 디렉토리 감시는 지원하지 않음 (지정 디렉토리만)
- 감시 goroutine은 context 취소 시 정상 종료

#### REQ-SYSAGENT-001-05-05: FileAgent 통계

**WHEN** `Stats()`가 호출되면 **THEN** 기본 AgentStats에 추가로 파일 특화 통계를 제공해야 한다:

- `FilesRead`: 읽기 실행 횟수
- `FilesWritten`: 쓰기 실행 횟수
- `BytesRead`: 읽은 총 바이트 수
- `BytesWritten`: 쓴 총 바이트 수
- `ActiveWatches`: 활성 디렉토리 감시 수
- `FileEventsReceived`: 수신된 파일 이벤트 수

---

### Module 6: Logger Agent (P1)

Logger Agent는 slog와 observe 패키지를 연동하여 컴포넌트별 로그 관리를 제공한다.

#### REQ-SYSAGENT-001-06-01: LoggerAgent 인터페이스

시스템은 **항상** 다음 메서드를 가진 `LoggerAgent` 인터페이스를 제공해야 한다:

```go
type LoggerAgent interface {
    SystemAgent
    Log(component string, level LogLevel, msg string, attrs ...any)
    Debug(component string, msg string, attrs ...any)
    Info(component string, msg string, attrs ...any)
    Warn(component string, msg string, attrs ...any)
    Error(component string, msg string, attrs ...any)
    SetLevel(component string, level LogLevel) error
    GetLevel(component string) LogLevel
    Subscribe(handler LogStreamHandler) (LogStreamID, error)
    Unsubscribe(id LogStreamID) error
}

type LogLevel string

const (
    LogLevelDebug LogLevel = "debug"
    LogLevelInfo  LogLevel = "info"
    LogLevelWarn  LogLevel = "warn"
    LogLevelError LogLevel = "error"
)

type LogStreamHandler func(entry LogEntry)

type LogEntry struct {
    Component string
    Level     LogLevel
    Message   string
    Attrs     map[string]any
    Timestamp time.Time
}

type LogStreamID string
```

#### REQ-SYSAGENT-001-06-02: 컴포넌트별 로그 작성

**WHEN** `Log(component, level, msg, attrs...)`가 호출되면 **THEN** slog를 통해 해당 컴포넌트의 로그를 기록해야 한다.

- `internal/observe/` 패키지의 컴포넌트별 로거 팩토리와 연동
- 컴포넌트별 로그 레벨이 설정되어 있으면 해당 레벨 이하의 로그는 기록하지 않는다
- `Debug()`, `Info()`, `Warn()`, `Error()`는 `Log()`의 편의 메서드이다

#### REQ-SYSAGENT-001-06-03: 로그 레벨 동적 변경

**WHEN** `SetLevel(component, level)`이 호출되면 **THEN** 해당 컴포넌트의 로그 레벨을 런타임에 변경해야 한다.

- 컴포넌트가 존재하지 않으면 새로 등록한다
- 잘못된 로그 레벨 시 `ErrInvalidLogLevel` 에러
- 변경은 즉시 적용되며, 진행 중인 로그 기록에는 영향을 주지 않는다

#### REQ-SYSAGENT-001-06-04: 로그 스트림 구독

**WHEN** `Subscribe(handler)`가 호출되면 **THEN** 모든 로그 엔트리를 실시간으로 핸들러에 전달해야 한다.

- 주로 디버그/모니터링 목적으로 사용
- 구독자에게 비동기적으로 LogEntry를 전달 (채널 기반)
- 로그 스트림은 버퍼링되며, 소비가 느린 구독자는 드롭 처리

#### REQ-SYSAGENT-001-06-05: LoggerAgent 통계

**WHEN** `Stats()`가 호출되면 **THEN** 기본 AgentStats에 추가로 로거 특화 통계를 제공해야 한다:

- `TotalLogs`: 기록된 총 로그 수
- `LogsByLevel`: 레벨별 로그 수 (`map[LogLevel]int64`)
- `Components`: 등록된 컴포넌트 수
- `ActiveStreams`: 활성 로그 스트림 구독 수
- `DroppedLogs`: 소비 지연으로 드롭된 로그 수

---

### Module 7: System Agent Manager (P1)

System Agent Manager는 5종 System Agent의 일괄 생성/시작/중지를 관리한다.

#### REQ-SYSAGENT-001-07-01: SystemAgentManager 인터페이스

시스템은 **항상** 다음 메서드를 가진 `SystemAgentManager` 인터페이스를 제공해야 한다:

```go
type SystemAgentManager interface {
    Initialize(config SystemConfig) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Event() EventAgent
    Logger() LoggerAgent
    File() FileAgent
    Timer() TimerAgent
    Store() StoreAgent
    IsInitialized() bool
}

type SystemConfig struct {
    EventBufferSize   int
    FileSandboxRoot   string
    StoreDefaultTTL   time.Duration
    TimerMinInterval  time.Duration
    LogDefaultLevel   LogLevel
}
```

#### REQ-SYSAGENT-001-07-02: 자동 초기화

**WHEN** `Initialize(config)`가 호출되면 **THEN** 5종 System Agent를 모두 생성하고 초기화해야 한다.

- 이미 초기화된 상태에서 재호출 시 `ErrAlreadyInitialized` 에러
- 각 System Agent는 TypeRegistry에 등록된 팩토리를 통해 생성
- 초기화 순서: Event -> Store -> Timer -> Logger -> File (의존성 순)

#### REQ-SYSAGENT-001-07-03: 일괄 시작/중지

**WHEN** `Start(ctx)`가 호출되면 **THEN** 모든 System Agent를 순서대로 시작해야 한다.
**WHEN** `Stop(ctx)`가 호출되면 **THEN** 모든 System Agent를 역순으로 중지해야 한다.

- 시작 순서: Event -> Store -> Timer -> Logger -> File
- 중지 순서: File -> Logger -> Timer -> Store -> Event (역순)
- 하나의 Agent 시작 실패 시 이미 시작된 Agent들을 역순으로 중지하고 에러 반환

#### REQ-SYSAGENT-001-07-04: 싱글톤 접근자

시스템은 **항상** `Event()`, `Logger()`, `File()`, `Timer()`, `Store()` 메서드를 통해 각 System Agent 인스턴스에 접근 가능해야 한다.

- 초기화 전 호출 시 `nil`을 반환하거나 panic하지 않고 `ErrNotInitialized` 에러를 감싸서 반환

---

### Module 8: System Agent Info & Stats (P1)

각 System Agent의 특화된 정보/통계를 제공한다.

#### REQ-SYSAGENT-001-08-01: SystemAgentInfo 확장

**WHEN** System Agent의 `Info()`가 호출되면 **THEN** 기본 AgentInfo에 추가로 시스템 에이전트 특화 정보를 제공해야 한다:

```go
type SystemAgentInfo struct {
    AgentInfo
    SystemType SystemAgentType
    AutoStart  bool
    Config     interface{} // Agent별 설정 구조체
}
```

#### REQ-SYSAGENT-001-08-02: Agent별 특화 통계

시스템은 **항상** 각 System Agent의 `Stats()` 메서드가 해당 Agent에 특화된 통계 필드를 포함해야 한다.

- Event Agent: REQ-SYSAGENT-001-01-08에 정의된 통계
- Store Agent: REQ-SYSAGENT-001-02-07에 정의된 통계
- Timer Agent: REQ-SYSAGENT-001-03-07에 정의된 통계
- File Agent: REQ-SYSAGENT-001-05-05에 정의된 통계
- Logger Agent: REQ-SYSAGENT-001-06-05에 정의된 통계

### Module 9: Agent Start() Stopped 상태 복구 (P0)

#### REQ-SYSAGENT-001-09-01 (Event-Driven) Stopped→Running 재시작

**WHEN** 시스템 에이전트의 `Start(ctx)` 메서드가 호출될 때 현재 상태가 `Stopped`이면, **THEN** 시스템은 `Created` 상태로 전이한 후 `Init(cfg)`를 호출하여 에이전트를 재초기화해야 한다. 이를 통해 에이전트 인스턴스를 새로 생성하지 않고도 재시작이 가능하다.

**적용 대상**: ConsoleLoggerAgent, MQTTAgent, TSDBAgent, HTTPReceiverAgent 및 향후 추가되는 모든 시스템 에이전트.

#### REQ-SYSAGENT-001-09-02 (Event-Driven) Running 상태 no-op

**WHEN** 시스템 에이전트의 `Start(ctx)` 메서드가 호출될 때 현재 상태가 `Running`이면, **THEN** 시스템은 아무 작업도 수행하지 않고 `nil`을 반환해야 한다 (멱등성).

### Module 10: HTTPReceiverAgent - HTTP 수신 에이전트 (P1)

#### REQ-SYSAGENT-001-10-01 (Ubiquitous) HTTPReceiverAgent 구현

시스템은 **항상** `http-receiver` 타입의 시스템 에이전트를 제공해야 한다. 이 에이전트는 지정된 HTTP 엔드포인트에서 데이터를 수신하여 내부 채널 버퍼에 저장하고, `MessageReceiver` 인터페이스를 통해 브릿지 노드가 메시지를 가져갈 수 있도록 한다.

#### REQ-SYSAGENT-001-10-02 (Ubiquitous) HTTP 수신 설정

시스템은 **항상** 다음 설정을 지원해야 한다:
- `listen_addr`: 수신 주소 (기본: `:8080`)
- `path`: 수신 경로 (기본: `/`)
- `method`: 허용 HTTP 메서드 (기본: `POST`)
- `timeout_sec`: 요청 타임아웃 (기본: 30초)
- `buffer_size`: 수신 버퍼 크기 (기본: 256)
- `max_body_bytes`: 최대 요청 본문 크기 (기본: 1MB)

#### REQ-SYSAGENT-001-10-03 (Event-Driven) 버퍼 가득 참 처리

**WHEN** 수신 버퍼가 가득 찬 상태에서 HTTP 요청이 도착하면, **THEN** 시스템은 HTTP 503 (Service Unavailable)을 반환해야 한다.

#### REQ-SYSAGENT-001-10-04 (Event-Driven) Graceful Shutdown

**WHEN** `Stop(ctx)` 메서드가 호출되면, **THEN** 시스템은 HTTP 서버를 graceful shutdown하고, `ReceiveMessage` 대기자에게 종료 시그널을 전달하고, 버퍼에 남은 메시지를 드레인해야 한다.

---

## 4. Specifications (설계)

### 4.1 패키지 구조

```
internal/agent/system/
    event.go                # EventAgent 구현
    event_test.go
    store.go                # StoreAgent 구현
    store_test.go
    timer.go                # TimerAgent 구현
    timer_test.go
    file.go                 # FileAgent 구현
    file_test.go
    logger.go               # LoggerAgent 구현
    logger_test.go
    manager.go              # SystemAgentManager 구현
    manager_test.go
    errors.go               # sentinel 에러 정의
    errors_test.go
    doc.go                  # 패키지 문서
```

### 4.2 주요 타입 시그니처

```go
package system

import (
    "context"
    "os"
    "sync"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/robfig/cron/v3"

    "github.com/xtra/xflow/internal/agent"
    "github.com/xtra/xflow/pkg/lifecycle"
)

// === Module 1: Event Agent ===

type EventAgent interface {
    agent.SystemAgent
    Emit(topic string, data interface{}) error
    Subscribe(topic string, handler EventHandler) (SubscriptionID, error)
    SubscribePattern(pattern string, handler EventHandler) (SubscriptionID, error)
    Unsubscribe(id SubscriptionID) error
    Topics() []string
    SubscriberCount(topic string) int
}

type eventAgent struct {
    agent.BaseAgent
    mu           sync.RWMutex
    subscribers  map[string][]subscription      // topic -> []subscription
    patterns     []patternSubscription
    bufferSize   int
    stats        eventStats
}

func (a *eventAgent) IsSystem() bool           { return true }
func (a *eventAgent) RequiresTransport() bool   { return false }

// === Module 2: Store Agent ===

type StoreAgent interface {
    agent.SystemAgent
    Get(namespace, key string) (interface{}, bool)
    Set(namespace, key string, value interface{}) error
    SetWithTTL(namespace, key string, value interface{}, ttl time.Duration) error
    Delete(namespace, key string) error
    Keys(namespace string) []string
    Clear(namespace string) error
    Watch(namespace, key string, handler WatchHandler) (WatchID, error)
    Unwatch(id WatchID) error
}

type storeAgent struct {
    agent.BaseAgent
    namespaces sync.Map         // map[string]*namespace
    watchers   sync.Map         // map[WatchID]*watcher
    stats      storeStats
}

func (a *storeAgent) IsSystem() bool           { return true }
func (a *storeAgent) RequiresTransport() bool   { return false }

// === Module 3: Timer Agent ===

type TimerAgent interface {
    agent.SystemAgent
    SetInterval(id string, interval time.Duration, handler TimerHandler) (TimerID, error)
    SetCron(id string, cronExpr string, handler TimerHandler) (TimerID, error)
    SetTimeout(id string, delay time.Duration, handler TimerHandler) (TimerID, error)
    Cancel(id TimerID) error
    List() []TimerInfo
}

type timerAgent struct {
    agent.BaseAgent
    mu         sync.RWMutex
    timers     map[TimerID]*timerEntry
    cronSched  *cron.Cron
    minInterval time.Duration
    stats      timerStats
}

func (a *timerAgent) IsSystem() bool           { return true }
func (a *timerAgent) RequiresTransport() bool   { return false }

// === Module 5: File Agent ===

type FileAgent interface {
    agent.SystemAgent
    Read(path string) ([]byte, error)
    Write(path string, data []byte, perm os.FileMode) error
    Append(path string, data []byte) error
    Exists(path string) bool
    Remove(path string) error
    List(dir string) ([]FileInfo, error)
    WatchDir(dir string, handler FileEventHandler) (FileWatchID, error)
    UnwatchDir(id FileWatchID) error
}

type fileAgent struct {
    agent.BaseAgent
    sandboxRoot string
    watcher     *fsnotify.Watcher
    watches     sync.Map             // map[FileWatchID]*fileWatch
    stats       fileStats
}

func (a *fileAgent) IsSystem() bool           { return true }
func (a *fileAgent) RequiresTransport() bool   { return false }

// === Module 6: Logger Agent ===

type LoggerAgent interface {
    agent.SystemAgent
    Log(component string, level LogLevel, msg string, attrs ...any)
    Debug(component string, msg string, attrs ...any)
    Info(component string, msg string, attrs ...any)
    Warn(component string, msg string, attrs ...any)
    Error(component string, msg string, attrs ...any)
    SetLevel(component string, level LogLevel) error
    GetLevel(component string) LogLevel
    Subscribe(handler LogStreamHandler) (LogStreamID, error)
    Unsubscribe(id LogStreamID) error
}

type loggerAgent struct {
    agent.BaseAgent
    levels     sync.Map              // map[string]*slog.LevelVar
    streams    sync.Map              // map[LogStreamID]*logStream
    defaultLvl LogLevel
    stats      loggerStats
}

func (a *loggerAgent) IsSystem() bool           { return true }
func (a *loggerAgent) RequiresTransport() bool   { return false }

// === Module 7: System Agent Manager ===

type SystemAgentManager interface {
    Initialize(config SystemConfig) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
    Event() EventAgent
    Logger() LoggerAgent
    File() FileAgent
    Timer() TimerAgent
    Store() StoreAgent
    IsInitialized() bool
}

type systemAgentManager struct {
    mu          sync.Mutex
    initialized bool
    event       EventAgent
    logger      LoggerAgent
    file        FileAgent
    timer       TimerAgent
    store       StoreAgent
    config      SystemConfig
}
```

### 4.3 TypeRegistry 등록 패턴

각 System Agent는 `init()` 함수에서 TypeRegistry에 팩토리를 등록한다:

```go
// event.go
func init() {
    agent.DefaultTypeRegistry.RegisterType(string(agent.SystemAgentEvent), newEventAgent)
}

func newEventAgent(config agent.AgentConfig) (agent.Agent, error) {
    a := &eventAgent{
        subscribers: make(map[string][]subscription),
        bufferSize:  256,
    }
    if err := a.BaseAgent.Init(config); err != nil {
        return nil, err
    }
    return a, nil
}
```

### 4.4 Lua 바인딩 예시 (SPEC-SCRIPT-001에서 구현)

```lua
-- Event Agent
xflow.event.emit("sensor.reading", {temperature = 25.5, humidity = 60})
xflow.event.subscribe("flow.*", function(event)
    print("Flow event:", event.topic, event.data)
end)

-- Store Agent
xflow.store.set("cache", "sensor_1", {value = 25.5, unit = "C"})
local val = xflow.store.get("cache", "sensor_1")

-- Timer Agent
xflow.timer.set_interval("heartbeat", 5000, function(trigger)
    print("Tick:", trigger.tick_count)
end)

-- Logger Agent
xflow.log.info("my_node", "Processing started", {count = 42})
xflow.log.set_level("my_node", "debug")

-- File Agent
local data = xflow.file.read("/data/config.json")
xflow.file.write("/data/output.txt", "Hello, World!")
```

### 4.5 Bridge Node 경유 패턴 (SPEC-NODE-001에서 구현)

System Agent는 Bridge Node를 통해 플로우 그래프에 참여할 수 있다:

```
[Timer Agent] --trigger--> [Bridge Node] --msg--> [Transform Node] --msg--> [MQTT Agent]
                               (interval: 5s)

[Event Agent] --event--> [Bridge Node] --msg--> [Logger Node]
                            (topic: flow.*)
```

### 4.6 의존성 다이어그램

```
internal/agent/system/
    ├── 의존 ──► internal/agent/ (SPEC-AGENT-001)
    │               ├── BaseAgent (생명주기, 상태 관리 재사용)
    │               ├── SystemAgent 인터페이스 (IsSystem, RequiresTransport)
    │               ├── TypeRegistry (init() 자동 등록)
    │               ├── AgentConfig, AgentStats, AgentInfo
    │               └── Manager, Registry
    │
    ├── 의존 ──► pkg/lifecycle/ (SPEC-LIFE-001)
    ├── 의존 ──► pkg/message/ (SPEC-MSG-001)
    ├── 의존 ──► pkg/xferr/ (SPEC-ERR-001)
    ├── 의존 ──► internal/observe/ (SPEC-OBS-001)
    │
    ├── 외부 ──► github.com/fsnotify/fsnotify (File Agent)
    ├── 외부 ──► github.com/robfig/cron/v3 (Timer Agent)
    │
    ├── 소비자 ◄── internal/node/ (SPEC-NODE-001)
    │                └── Bridge Node: System Agent를 Bridge로 연결
    ├── 소비자 ◄── internal/script/ (SPEC-SCRIPT-001)
    │                └── Lua 바인딩: xflow.event, xflow.store, xflow.timer 등
    ├── 소비자 ◄── internal/engine/ (SPEC-ENGINE-001)
    │                └── Flow Engine: SystemAgentManager 자동 초기화/시작
    └── 소비자 ◄── internal/plugin/ (SPEC-PLUGIN-001)
                     └── Plugin: Go 인터페이스/WASM ABI 접근
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 우선순위 | product.md 근거 |
|------------|------|---------|-----------------|
| REQ-SYSAGENT-001-01-01 ~ 08 | Event Agent | P0 | "시스템 이벤트 발행/구독, 플로우 상태 변경, Agent 연결/해제, 에러 발생 등 내부 이벤트" |
| REQ-SYSAGENT-001-02-01 ~ 07 | Store Agent | P0 | "키-값 저장소, 플로우 간 공유 데이터 저장/조회, 영속적/휘발성 선택" |
| REQ-SYSAGENT-001-03-01 ~ 07 | Timer Agent | P0 | "타이머/스케줄러, 주기적 실행, cron 스케줄, 지연 실행, 반복 실행" |
| REQ-SYSAGENT-001-04-01 ~ 02 | Error Types | P0 | "패키지 전용 sentinel 에러 정의" |
| REQ-SYSAGENT-001-05-01 ~ 05 | File Agent | P1 | "파일 시스템 Agent, 로컬 파일 읽기/쓰기, 디렉토리 감시(fsnotify)" |
| REQ-SYSAGENT-001-06-01 ~ 05 | Logger Agent | P1 | "로그 관리 Agent, 컴포넌트별 로그 작성, 로그 레벨 동적 제어, 로그 스트림 구독" |
| REQ-SYSAGENT-001-07-01 ~ 04 | System Agent Manager | P1 | "시스템 시작 시 자동으로 활성화, 별도 등록 불필요" (REQ-AGENT-001-14-03) |
| REQ-SYSAGENT-001-08-01 ~ 02 | System Agent Info & Stats | P1 | "에이전트 상태 정보, 관리 데이터 조회" (REQ-AGENT-001-16-*) |

### 교차 참조

| SPEC-AGENT-001 요구사항 | SPEC-SYSAGENT-001 구현 |
|------------------------|----------------------|
| REQ-AGENT-001-14-01 (SystemAgent 인터페이스) | 모든 System Agent가 SystemAgent 구현 |
| REQ-AGENT-001-14-02 (RequiresTransport = false) | 모든 System Agent에서 false 반환 |
| REQ-AGENT-001-14-03 (자동 활성화) | Module 7: SystemAgentManager |
| REQ-AGENT-001-15-01 (TypeRegistry 등록) | init() 함수에서 자동 등록 |
| REQ-AGENT-001-16-* (Info & Stats) | Module 8: SystemAgentInfo 확장 |

---

## 6. Implementation Notes (구현 노트)

- 신규 모듈 3종 구현: Event Agent, File Agent, SystemAgentManager
- Event Agent: Go 채널 기반 Pub/Sub, 패턴 매칭(*/**), 비동기 전달, 버퍼 드롭 전략
- File Agent: 샌드박스 경로 제한(symlink 해석 포함), 파일 CRUD, fsnotify 감시
- SystemAgentManager: Event/File 에이전트 일괄 생성/시작/중지 조정
- 기존 Store/Timer/Logger 에이전트는 변경하지 않음 (BaseAgent 마이그레이션 별도 진행)
- 센티넬 에러: Event 6개, File 6개, Manager 3개 정의
- 테스트: 55개, 커버리지 87.4%, race-free
- 커밋: 26e9487

### v1.2.0 구현 노트 (Console Logger → Logger)

**1. Agent Type 리네이밍 (console-logger → logger)**
- `ConsoleLoggerAgent.Type()` 반환값을 `"console-logger"`에서 `"logger"`로 변경
- `TypeRegistry` 등록 키도 `"logger"`로 통일
- 에러 메시지, 로깅 접두사, 상태명 등 모든 내부 문자열을 `"logger"`로 일관 변경
- 프론트엔드 `web/src/config/agentSchemas.ts`의 라벨도 함께 업데이트

**2. MessagePublisher 인터페이스 구현 (주요 기능 추가)**
- `PublishMessage(topic, qos, retained, payload)` 메서드 구현
- topic을 파일 경로로 해석하여 파일별 독립 출력을 지원
- `managedFileWriter` 구조체로 토픽별 파일 라이터를 관리
- 빈 topic 전달 시 기본 로거로 폴백 처리
- double-checked locking 패턴으로 파일 라이터의 안전한 지연 생성 보장
- 파일 라이터가 에이전트의 롤링 설정(max_size, max_age 등)을 상속
- `Stop()` 및 `Configure()` 호출 시 관리 중인 파일 라이터를 정리

**3. Process() 로깅 레벨 수정**
- `Process()` 내부 로깅 레벨을 Debug에서 Info로 변경

**4. 테스트 추가 (7건)**
- PublishMessage 기본 파일 쓰기 테스트
- 멀티 파일 동시 쓰기 테스트
- 빈 토픽 폴백 테스트
- 롤링 설정 상속 테스트
- Stop 시 파일 정리 테스트
- Configure 트리거 정리 테스트
- 하위 디렉토리 자동 생성 테스트
