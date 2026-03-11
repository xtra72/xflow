# node - XFlow FBP 노드 시스템

`internal/node` 패키지는 XFlow 플랫폼의 핵심 실행 단위인 노드 시스템을 제공한다. FBP(Flow-Based Programming) 패러다임에 따라 데이터를 수신, 처리, 출력하는 독립 실행 컴포넌트를 정의하며, 공통 인터페이스, 기반 구현체, 레지스트리, 포트 시스템, 12종의 내장 노드 타입을 포함한다.

**SPEC**: SPEC-NODE-001

## 아키텍처 개요

```
    Node 시스템 계층 구조

    +------------------------------------------+
    |              Node Interface               |
    |  ID, Name, Type, Init, Process, Shutdown  |
    |  Configure, Ports                         |
    +------------------------------------------+
                       |
    +------------------------------------------+
    |              BaseNode                     |
    |  *lifecycle.BaseLifecycle 임베딩           |
    |  포트 관리, 설정 관리, 로깅/메트릭         |
    +------------------------------------------+
           |        |        |        |        |        |
    +------+  +-----+  +----+  +-----+  +-----+  +----+
    |Filter|  |Trans |  |Swit|  |Brid |  |Scri |  |Catc|
    |Node  |  |form  |  |ch  |  |ge   |  |pt   |  |h   |
    |      |  |Node  |  |Node|  |Node |  |Node |  |Node|
    +------+  +------+  +----+  +-----+  +-----+  +----+
           |        |        |        |
    +------+  +-----+  +----+  +------+
    |Aggre |  |Debug|  |Stat|  |Dead  |
    |gate  |  |Node |  |us  |  |Letter|
    |Node  |  |     |  |Node|  |Node  |
    +------+  +-----+  +----+  +------+
           |        |
    +------+  +-----+
    |Mappi |  |Modb |
    |ng    |  |us   |
    |Node  |  |RW   |
    +------+  +-----+

    +------------------------------------------+
    |              Registry                     |
    |  NodeFactory 등록/조회, 빌트인 자동 등록   |
    +------------------------------------------+
```

**핵심 구성 요소**:

1. **Node Interface**: 모든 노드가 구현하는 공통 계약 (8개 메서드)
2. **BaseNode**: `*lifecycle.BaseLifecycle` 임베딩 기반 구현체, 포트/설정/로깅 관리
3. **NodePort**: 런타임 포트 정보 (`flow.Port` 정적 정의와 분리, `Connected` 상태 포함)
4. **Registry**: `NodeFactory` 기반 노드 타입 등록/조회, 12개 빌트인 자동 등록
5. **NodeOption**: 함수형 옵션 패턴 (`WithLogger`, `WithMetrics`, `WithAgentResolver` 등)
6. **NodeError**: 노드 ID/타입 정보 포함 에러 래핑 구조체

## 핵심 인터페이스

### Node 인터페이스 (8개 메서드)

```go
type Node interface {
    ID() string
    Name() string
    Type() string
    Init(ctx context.Context) error
    Process(ctx context.Context, msg message.Message) ([]message.Message, error)
    Shutdown(ctx context.Context) error
    Configure(config map[string]any) error
    Ports() []NodePort
}
```

### 외부 의존성 분리 인터페이스

```go
// AgentResolver - 에이전트 참조를 AgentTransport로 해석 (BridgeNode용)
type AgentResolver interface {
    ResolveAgent(ctx context.Context, ref flow.AgentRef) (AgentTransport, error)
}

// AgentTransport - 에이전트와의 메시지 송수신 (BridgeNode용)
type AgentTransport interface {
    Send(ctx context.Context, msg message.Message) error
    Receive(ctx context.Context) (message.Message, error)
}

// ScriptEngine - 스크립트 컴파일/실행 엔진 (ScriptNode용, Lua 구현은 SPEC-SCRIPT-001)
type ScriptEngine interface {
    Compile(source string) error
    Execute(ctx context.Context, msg message.Message) (message.Message, error)
    Close() error
}
```

## 내장 노드 타입 (12종)

| 타입 이름 | 구조체 | 용도 |
|-----------|--------|------|
| `filter` | `FilterNode` | 조건 기반 메시지 필터링 (조건 불일치 시 폐기) |
| `transform` | `TransformNode` | 메시지 Payload 변환 (필드 추가/삭제/변환) |
| `switch` | `SwitchNode` | 조건별 출력 포트 라우팅 (First-Match, 기본 라우트) |
| `bridge` | `BridgeNode` | Agent-Flow 간 브릿지 (In/Out/InOut/RequestReply 4모드) |
| `script` | `ScriptNode` | ScriptEngine 기반 스크립트 메시지 처리 |
| `catch` | `CatchNode` | 에러 포트 메시지 수신 및 심각도/범주 기반 필터링 |
| `aggregate` | `AggregateNode` | 다중 메시지 집계 (count/time 윈도우, 7개 집계 함수) |
| `debug` | `DebugNode` | 메시지 로깅 pass-through (debug/info/warn 3단계) |
| `status` | `StatusNode` | 노드 생명주기 상태 변경 모니터링 (watchNodes 필터링) |
| `deadletter` | `DeadLetterNode` | TTL 만료/배달 불가/재시도 초과 메시지 수집 (log/store/forward) |
| `mapping` | `MappingNode` | 키 기반 값 매핑 (입력 값을 매핑 테이블로 변환) |
| `modbus_rw` | `ModbusRWNode` | MODBUS Agent 레지스터 읽기/쓰기 (Server/Client 자동 감지, 4영역 지원) |

## 파일 구조

```
internal/node/
  base.go               # Node 인터페이스 + BaseNode + NodePort + NodeOption
  base_test.go           # BaseNode 단위 테스트
  registry.go            # Registry + NodeFactory + 빌트인 등록 (12종)
  registry_test.go       # Registry 단위 테스트
  filter.go              # FilterNode 구현
  filter_test.go         # FilterNode 단위 테스트
  transform.go           # TransformNode 구현
  transform_test.go      # TransformNode 단위 테스트
  switch.go              # SwitchNode 구현
  switch_test.go         # SwitchNode 단위 테스트
  bridge.go              # BridgeNode + AgentResolver + AgentTransport
  bridge_test.go         # BridgeNode 단위 테스트
  script.go              # ScriptNode + ScriptEngine 인터페이스
  script_test.go         # ScriptNode 단위 테스트
  catch.go               # CatchNode 구현
  catch_test.go          # CatchNode 단위 테스트
  aggregate.go           # AggregateNode 구현 (count/time 윈도우 집계)
  aggregate_test.go      # AggregateNode 단위 테스트
  debug.go               # DebugNode 구현 (메시지 로깅 pass-through)
  debug_test.go          # DebugNode 단위 테스트
  status.go              # StatusNode 구현 (상태 변경 모니터링)
  status_test.go         # StatusNode 단위 테스트
  deadletter.go          # DeadLetterNode 구현 (폐기 메시지 수집)
  deadletter_test.go     # DeadLetterNode 단위 테스트
  mapping.go             # MappingNode 구현 (키 기반 값 매핑)
  mapping_test.go        # MappingNode 단위 테스트
  modbus_rw.go           # ModbusRWNode 구현 (MODBUS Agent 레지스터 읽기/쓰기)
  modbus_rw_test.go      # ModbusRWNode 단위 테스트 (30+ 테스트 케이스)
  errors.go              # 센티넬 에러 + NodeError 구조체
  errors_test.go         # 에러 타입 단위 테스트
```

## 에러 타입 목록 (12개 센티넬 에러)

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrNodeTypeAlreadyRegistered` | node: type already registered | 이미 등록된 노드 타입 재등록 |
| `ErrNodeTypeNotFound` | node: type not found | 등록되지 않은 노드 타입 조회 |
| `ErrPortNotFound` | node: port not found | 존재하지 않는 포트 조회 |
| `ErrInvalidConfig` | node: invalid configuration | 유효하지 않은 설정 전달 |
| `ErrAgentNotFound` | node: agent not found | 에이전트를 찾을 수 없음 |
| `ErrRequestTimeout` | node: request-reply timeout | 요청-응답 타임아웃 |
| `ErrScriptCompileFailed` | node: script compile failed | 스크립트 컴파일 실패 |
| `ErrScriptExecutionFailed` | node: script execution failed | 스크립트 실행 실패 |
| `ErrScriptTimeout` | node: script execution timeout | 스크립트 실행 타임아웃 |
| `ErrNodeNotInitialized` | node: not initialized | 초기화되지 않은 노드에서 연산 수행 |
| `ErrNodeAlreadyInitialized` | node: already initialized | 이미 초기화된 노드 재초기화 |
| `ErrAggregateWindowInvalid` | node: invalid aggregate window | 유효하지 않은 집계 윈도우 설정 |

모든 에러는 `errors.Is()` 함수로 비교 가능하다.

## 의존성

- **표준 라이브러리**: `context`, `sync`, `time`, `errors`, `fmt`, `sort`
- **외부 라이브러리**: `github.com/google/uuid` (BridgeNode correlation ID 생성)
- **내부 의존성**:
  - `pkg/lifecycle` (SPEC-LIFE-001) - `BaseLifecycle` 임베딩, 상태 관리
  - `pkg/flow` (SPEC-FLOW-001) - `NodeDef`, `PortDirection`, `AgentRef`, `BridgeDirection`
  - `pkg/message` (SPEC-MSG-001) - `Message` 인터페이스 (Process 입출력)
  - `internal/observe` (SPEC-OBS-001) - `ComponentLogger`, `MetricsCollector`

## 사용 예시

### Registry를 통한 노드 생성

```go
// 레지스트리 생성 (빌트인 10종 자동 등록)
registry := node.NewRegistry()

// 노드 정의로부터 노드 생성
filterNode, err := registry.Create(flow.NodeDef{
    ID:   "filter-1",
    Name: "입력 필터",
    Type: "filter",
    Config: map[string]any{
        "condition": "payload.level == 'error'",
    },
})
```

### BaseNode 옵션 패턴

```go
n, err := registry.Create(nodeDef,
    node.WithLogger(logger),
    node.WithMetrics(metrics),
)
```

### BridgeNode 에이전트 연결

```go
n, err := registry.Create(bridgeDef,
    node.WithAgentResolver(resolver),
)
```

## P2 노드 상세

### AggregateNode (집계 노드)

다중 메시지를 윈도우 단위로 모아 집계 결과를 출력한다.

- **윈도우 방식**: `count` (메시지 개수 기반), `time` (시간 간격 기반)
- **집계 함수** (7종): `sum`, `avg`, `count`, `min`, `max`, `first`, `last`
- **종료 시 플러시**: Shutdown 호출 시 잔여 버퍼의 메시지를 집계 후 출력
- **동시성 보호**: `sync.Mutex`로 버퍼 접근 보호, `time.Timer` 기반 시간 윈도우
- **설정 키**: `window_type`, `window_size`, `aggregate_fn`

### DebugNode (디버그 노드)

메시지 내용을 로깅하고 그대로 출력 포트로 전달하는 pass-through 노드이다.

- **로그 레벨**: `debug` (기본값), `info`, `warn`
- **로깅 내용**: 메시지 ID, Payload, Metadata를 설정된 로그 레벨로 출력
- **pass-through**: 메시지를 변형 없이 출력 포트로 전달
- **설정 키**: `level`

### StatusNode (상태 노드)

Flow 내 노드의 생명주기 상태 변경 이벤트를 모니터링한다.

- **상태 이벤트 수신**: `StatusCallback` 함수형 인터페이스로 상태 변경 이벤트 수신
- **이벤트 내용**: 노드 ID, 이전 상태, 새 상태, 타임스탬프
- **모니터링 대상 필터링**: `watch_nodes` 설정으로 특정 노드 ID만 모니터링 (빈 목록이면 전체)
- **설정 키**: `watch_nodes`

### DeadLetterNode (데드 레터 노드)

TTL 만료, 배달 불가, 최대 재시도 초과 메시지를 수집하고 처리한다.

- **데드 레터 사유** (3종): `ttl_expired`, `undeliverable`, `max_retries`
- **처리 전략** (3종): `log` (로깅만, 기본값), `store` (스토어 저장), `forward` (다른 Flow로 전달)
- **메트릭 기록**: 사유별 메트릭 카운터 자동 증가
- **원인 정보 보강**: Metadata에 `_deadletter_reason`, `_deadletter_node`, `_deadletter_timestamp` 첨부
- **설정 키**: `strategy`

### ModbusRWNode (MODBUS 읽기/쓰기 노드)

MODBUS Agent(Server/Client)의 레지스터를 플로우 내에서 직접 읽기/쓰기하는 전용 처리 노드이다.

- **단일 노드 설계**: `operation` 설정(read/write)에 따라 읽기 또는 쓰기로 동작
- **Agent 타입 자동 감지**: `AgentAccessor`를 통해 원본 Agent 객체를 획득하고, 타입 어서션으로 Server(`*modbusserver.MODBUSServerAgent`) / Client(`*modbus.MODBUSAgent`) 자동 감지
- **4개 레지스터 영역**: `coils`, `discrete_inputs`, `holding_registers`, `input_registers`
- **다중 데이터 타입**: `uint16`, `int16`, `float32`, `uint32`, `int32` (Server Agent `get_register_typed` 활용)
- **Server Agent 명령 매핑**: `get_coils`, `get_discrete_inputs`, `get_holding_registers`, `get_register_typed`, `set_coil`, `set_coils`, `set_register`, `set_registers`
- **Client Agent 명령 매핑**: `read_registers` (FC 코드 매핑), `write_coil`, `write_coils`, `write_register`, `write_registers`
- **payload 보존**: 읽기/쓰기 결과를 원본 메시지의 payload에 병합
- **에러 포트**: 타임아웃, Agent 상태 이상, 패닉 등을 에러 포트로 안전하게 전달
- **런타임 오버라이드**: 입력 메시지의 payload에서 `device_id`, `read_mode` 동적 오버라이드 지원
- **설정 키**: `agent_ref`, `operation`, `register_area`, `address`, `count`, `data_type`, `byte_order`, `device_id`
- **센티넬 에러** (9개): `ErrInvalidOperation`, `ErrReadOnlyArea`, `ErrInvalidRegisterArea`, `ErrInvalidCount`, `ErrInvalidAddress`, `ErrMissingWriteValue`, `ErrUnsupportedAgentType`, `ErrMissingAgentRef`, `ErrAgentProcessFailed`

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/node/...

# Race Detector 포함 테스트
go test -race ./internal/node/...

# 커버리지 확인
go test -cover ./internal/node/

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/node/
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트 수: 약 159개
- 커버리지: 92.8% (전체)
- Race Detector: 이상 없음 (go test -race)
- Go Vet: 이상 없음 (go vet)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-NODE-001 | 본 SPEC | Node 시스템 전체 명세 |
| SPEC-LIFE-001 | 의존 | `BaseLifecycle` 상태 관리 (임베딩) |
| SPEC-FLOW-001 | 의존 | `NodeDef`, `PortDirection`, `AgentRef` 정적 타입 |
| SPEC-MSG-001 | 의존 | `Message` 인터페이스 (Process 입출력) |
| SPEC-OBS-001 | 의존 | `ComponentLogger`, `MetricsCollector` 관측 인터페이스 |
| SPEC-ERR-001 | 소비자 | CatchNode가 `ErrorMessage` 타입 소비 |
| SPEC-ENGINE-001 | 소비자 | Engine이 `Node` 인터페이스와 `Registry` 소비 |
| SPEC-SCRIPT-001 | 이연 | `ScriptEngine` Lua 구현 (ScriptNode에서 인터페이스만 정의) |
| SPEC-MODBUS-004 | 확장 | ModbusRWNode - MODBUS Agent 레지스터 읽기/쓰기 전용 노드 |
