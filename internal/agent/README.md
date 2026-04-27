# agent - XFlow Agent 프레임워크

`internal/agent` 패키지는 XFlow 플랫폼의 Agent 시스템 프레임워크를 제공한다. Agent 인터페이스, BaseAgent 기본 구현, Manager 생명주기 관리, Registry 조회, TypeRegistry 팩토리 패턴, AgentConfig 설정 검증, AgentStats 통계 수집을 통합적으로 지원한다.

**SPEC**: SPEC-AGENT-001

## 아키텍처 개요

```
    Agent 프레임워크 계층 구조

    +----------------------------------------------------+
    |                  Manager                            |
    |  Agent 생명주기 관리 (생성, 시작, 중지, 재시작, 삭제)  |
    |  생성순서 추적, 역순 종료 (Graceful Shutdown)         |
    +----------------------------------------------------+
            |                         |
    +-------v--------+       +-------v--------+
    |    Registry     |       |  TypeRegistry  |
    |  ID/Name/Type   |       |  AgentFactory  |
    |  조회 및 목록    |       |  팩토리 등록    |
    +----------------+       +----------------+
            |
    +-------v---------+
    |     Agent        |  인터페이스 (13개 메서드)
    +-----------------+
            |
    +-------v---------+
    |    BaseAgent     |  기본 구현
    |  lifecycle 임베딩 |  Transport 래핑
    |  AgentStats 관리 |  동시성 보호
    +-----------------+
            |
    +-------+----------+
    |                   |
    v                   v
Agent 구현체         SystemAgent
(Custom, MQTT 등)   (Event, File, Timer 등)
```

**핵심 구성 요소**:

1. **Agent 인터페이스**: 13개 메서드 (Init/Start/Stop/Pause/Resume/Health/Process/Configure/ID/Name/Type/Info/Stats)
2. **BaseAgent**: `lifecycle.BaseLifecycle` 임베딩, Transport 래핑, AgentStats atomic 카운터 관리
3. **Manager**: Agent 생성/시작/중지/재시작/삭제, 생성순서 추적, Graceful Shutdown (역순 종료)
4. **Registry**: ID/Name/Type 기반 Agent 조회, 동시성 안전 (`sync.RWMutex`)
5. **TypeRegistry**: AgentFactory 팩토리 패턴, 타입별 Agent 인스턴스 생성
6. **AgentConfig**: Agent 설정 구조체, `Validate()` 유효성 검증
7. **AgentInfo/AgentStats**: Agent 종합 상태 스냅샷, atomic 카운터 기반 통계

## 핵심 인터페이스

### Agent 인터페이스 (13개 메서드)

```go
type Agent interface {
    Init(config AgentConfig) error       // Agent 초기화
    Start(ctx context.Context) error     // Agent 실행 시작
    Stop(ctx context.Context) error      // Agent 정상 종료
    Pause(ctx context.Context) error     // Agent 일시정지
    Resume(ctx context.Context) error    // Agent 재개
    Health() HealthStatus                // 현재 헬스 상태 반환
    Process(data []byte) ([]byte, error) // 수신 데이터 처리
    Configure(config AgentConfig) error  // 런타임 설정 변경
    ID() string                          // Agent 고유 식별자
    Name() string                        // Agent 표시 이름
    Type() string                        // Agent 타입
    Info() AgentInfo                     // 종합 상태 스냅샷
    Stats() StatsSnapshot                // 처리 통계
}
```

### Transport 인터페이스

통신 계층 추상화이다. System Agent는 Transport를 사용하지 않는다 (`RequiresTransport() = false`).

```go
type Transport interface {
    Open(config TransportConfig) error
    Close() error
    Read(buf []byte) (int, error)
    Write(data []byte) (int, error)
    Available() bool
}
```

### SystemAgent 인터페이스

Transport/Protocol 없이 직접 참조 가능한 내장 서비스 인터페이스이다.

```go
type SystemAgent interface {
    Agent
    IsSystem() bool           // 항상 true 반환
    RequiresTransport() bool  // 항상 false 반환
}
```

## BaseAgent 구조

BaseAgent는 Agent 인터페이스의 기본 구현이다. `lifecycle.BaseLifecycle`을 임베딩하여 7상태 생명주기 관리를 상속한다.

```go
type BaseAgent struct {
    *lifecycle.BaseLifecycle          // 7상태 생명주기 관리
    config    AgentConfig             // 현재 설정
    transport Transport               // 통신 계층 (System Agent는 nil)
    stats     *AgentStats             // atomic 카운터 기반 통계
    mu        sync.RWMutex            // 내부 상태 동시성 보호
    startedAt time.Time               // 마지막 시작 시각
    createdAt time.Time               // Agent 생성 시각
}
```

**생명주기 상태 전이**:

```
Created -> Initializing -> Running <-> Paused -> Stopping -> Stopped
                                                     |
                                                  Error
```

**사용 예시**:

```go
// BaseAgent 생성 및 초기화
ba := agent.NewBaseAgent()
config := agent.AgentConfig{
    ID:   "my-agent-001",
    Name: "My Custom Agent",
    Type: "custom",
}
if err := ba.Init(config); err != nil {
    log.Fatal(err)
}
defer ba.Stop(ctx)
```

## AgentConfig와 Validate()

Agent 설정을 정의하는 구조체이다. `Validate()` 메서드로 필수 필드와 값 범위를 검증한다.

```go
type AgentConfig struct {
    ID                  string            // Agent 고유 식별자 (필수)
    Name                string            // Agent 표시 이름 (필수)
    Type                string            // Agent 타입
    Transport           TransportConfig   // Transport 설정
    HealthCheckInterval time.Duration     // 헬스 체크 주기
    MaxRestarts         int               // 최대 재시작 횟수
    StopOnZeroRef       bool              // 참조 카운트 0 시 자동 중지
    BufferSize          int               // 수신 버퍼 크기
    Metadata            map[string]string // 사용자 메타데이터
}
```

**Validate() 검증 항목**:

- `ID`가 비어있지 않은지 확인
- `Name`이 비어있지 않은지 확인
- `HealthCheckInterval`이 0보다 큰지 확인
- `MaxRestarts`가 0 이상인지 확인
- `BufferSize`가 0보다 큰지 확인

## AgentStats (atomic 카운터) - Snapshot 패턴

AgentStats는 내부에서 `sync/atomic` 연산으로 카운터를 관리하며, 외부에는 `StatsSnapshot` 구조체로 스냅샷을 제공한다.

```go
// 내부 atomic 카운터 (AgentStats)
type AgentStats struct {
    messagesReceived atomic.Int64
    messagesSent     atomic.Int64
    messagesErrored  atomic.Int64
    bytesRead        atomic.Int64
    bytesWritten     atomic.Int64
    restartCount     atomic.Int64
    // ...
}

// 외부 스냅샷 (StatsSnapshot)
type StatsSnapshot struct {
    MessagesReceived int64
    MessagesSent     int64
    MessagesErrored  int64
    BytesRead        int64
    BytesWritten     int64
    RestartCount     int64
    LastActivityAt   time.Time
}
```

**사용 패턴**:

```go
// Stats() 호출 시 현재 atomic 값의 스냅샷 반환
snapshot := myAgent.Stats()
fmt.Printf("수신: %d, 송신: %d, 에러: %d\n",
    snapshot.MessagesReceived,
    snapshot.MessagesSent,
    snapshot.MessagesErrored)
```

## TypeRegistry와 AgentFactory 패턴

TypeRegistry는 Agent 타입별 팩토리 함수를 등록하고, 타입명으로 Agent 인스턴스를 생성하는 패턴이다.

```go
// AgentFactory 함수 타입
type AgentFactory func(config AgentConfig) (Agent, error)

// TypeRegistry 인터페이스
type TypeRegistry interface {
    RegisterType(agentType string, factory AgentFactory) error
    CreateAgent(agentType string, config AgentConfig) (Agent, error)
    ListTypes() []string
    HasType(agentType string) bool
}
```

**사용 예시**:

```go
// 커스텀 Agent 타입 등록
registry := agent.NewTypeRegistry()
registry.RegisterType("custom", func(config agent.AgentConfig) (agent.Agent, error) {
    a := NewCustomAgent()
    if err := a.Init(config); err != nil {
        return nil, err
    }
    return a, nil
})

// 타입명으로 Agent 생성
config := agent.AgentConfig{ID: "custom-001", Name: "Custom Agent", Type: "custom"}
a, err := registry.CreateAgent("custom", config)
```

## Registry (ID/Name/Type 조회)

Registry는 실행 중인 Agent의 등록, 조회, 목록 관리를 담당한다.

```go
type Registry interface {
    Register(agent Agent) error              // Agent 등록
    Unregister(agentID string) error         // Agent 등록 해제
    Get(agentID string) (Agent, bool)        // ID 기반 조회
    GetByName(name string) (Agent, bool)     // 이름 기반 조회
    GetByType(agentType string) []Agent      // 타입 기반 필터링
    List() []Agent                           // 전체 목록
    Count() int                              // 등록 수
}
```

**특징**:

- `sync.RWMutex` 기반 동시성 안전
- 중복 Agent ID 등록 거부 (`ErrAgentAlreadyExists`)
- ID, Name, Type 세 가지 축으로 조회 가능

## Manager (생성순서 추적, 역순 종료)

Manager는 Agent의 전체 생명주기를 관리한다. 생성 순서를 추적하며, Graceful Shutdown 시 역순으로 종료한다.

```go
type Manager interface {
    Create(config AgentConfig) (Agent, error)          // Agent 생성
    Start(ctx context.Context, agentID string) error   // Agent 시작
    Stop(ctx context.Context, agentID string) error    // Agent 중지
    Restart(ctx context.Context, agentID string) error // Agent 재시작
    Delete(agentID string) error                       // Agent 삭제
    Get(agentID string) (Agent, error)                 // Agent 조회
    List() []Agent                                     // 전체 목록
    Shutdown(ctx context.Context) error                // 전체 Graceful Shutdown
    Summary() ManagerSummary                           // 관리 데이터 요약
}
```

**Graceful Shutdown 동작**:

1. 모든 Running/Paused 상태 Agent에 Stop 전파
2. Context deadline까지 정상 종료 대기
3. Deadline 초과 시 강제 종료 및 에러 로깅
4. 생성 역순으로 종료하여 의존성 안전 보장

## 에러 타입 목록 (14개 센티넬 에러)

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrAgentNotFound` | agent: agent not found | ID로 Agent 조회 실패 |
| `ErrAgentAlreadyExists` | agent: agent already exists | 중복 Agent ID 생성 시도 |
| `ErrAgentNotRunning` | agent: agent not running | Running 상태가 아닌 Agent 조작 |
| `ErrAgentAlreadyStopped` | agent: agent already stopped | 이미 Stopped인 Agent 중지 시도 |
| `ErrTransportNotAvailable` | agent: transport type not available | 미등록 Transport 타입 |
| `ErrTransportClosed` | agent: transport connection closed | 닫힌 Transport에 I/O 시도 |
| `ErrProtocolParseError` | agent: protocol parse error | 프로토콜 파싱 실패 |
| `ErrChecksumMismatch` | agent: checksum mismatch | 체크섬 검증 실패 |
| `ErrHealthCheckFailed` | agent: health check failed | 헬스 체크 실패 |
| `ErrMaxRestartsExceeded` | agent: max restarts exceeded | 최대 재시작 횟수 초과 |
| `ErrInvalidConfig` | agent: invalid configuration | 유효하지 않은 설정 |
| `ErrConfigImmutable` | agent: immutable config field cannot be changed at runtime | 불변 설정 변경 시도 |
| `ErrInvalidStateTransition` | agent: invalid state transition | 유효하지 않은 상태 전이 |
| `ErrFlowAlreadyReferenced` | agent: flow already referenced | 동일 플로우 중복 참조 |

모든 에러는 `errors.Is()` 함수로 비교 가능하다.

## 파일 구조

```
internal/agent/
  agent.go              # Agent 인터페이스 + BaseAgent 구현
  agent_test.go
  manager.go            # Manager 인터페이스 + 구현
  manager_test.go
  registry.go           # Registry 인터페이스 + 구현
  registry_test.go
  type_registry.go      # TypeRegistry + AgentFactory
  type_registry_test.go
  config.go             # AgentConfig + Validate()
  config_test.go
  info.go               # AgentInfo, StatsSnapshot, ManagerSummary 등
  info_test.go
  health.go             # HealthStatus, HealthState 정의
  health_test.go
  system.go             # SystemAgent 인터페이스 + SystemAgentType 열거
  system_test.go
  errors.go             # 14개 센티넬 에러
  errors_test.go

  system/               # System Agent 구현체 (SPEC-SYSAGENT-001)
    event.go            # EventAgent (Pub/Sub)
    file.go             # FileAgent (샌드박스 파일 I/O)
    system_manager.go   # SystemAgentManager
    store.go            # StoreAgent (키-값 저장소)
    timer.go            # TimerAgent (타이머/스케줄러)
    logger.go           # LoggerAgent (로그 관리)
    ...
```

## 의존성

- **표준 라이브러리**: `sync`, `sync/atomic`, `time`, `context`, `fmt`, `errors`
- **내부 의존성**:
  - `pkg/lifecycle` (SPEC-LIFE-001) - 7상태 생명주기 관리 (`BaseLifecycle` 임베딩)

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/agent/...

# Race Detector 포함 테스트
go test -race ./internal/agent/...

# 커버리지 확인 (system/ 서브패키지 제외)
go test -cover ./internal/agent/

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/agent/
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트: 87개 전체 통과
- 커버리지: 92.1%
- Race Detector: 이상 없음 (go test -race)
- Go Vet: 이상 없음 (go vet)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | State 인터페이스, 생명주기 상태 전이 |
| SPEC-SYSAGENT-001 | 소비자 | System Agent 구현체가 BaseAgent 임베딩 |
| SPEC-NODE-001 | 소비자 | Bridge Node가 Agent를 참조하여 메시지 교환 |
| SPEC-ENGINE-001 | 소비자 | 플로우 엔진이 Manager를 통해 Agent 관리 |
