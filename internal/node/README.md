# node - XFlow FBP 노드 시스템

`internal/node` 패키지는 XFlow 플랫폼의 핵심 실행 단위인 노드 시스템을 제공한다. FBP(Flow-Based Programming) 패러다임에 따라 데이터를 수신, 처리, 출력하는 독립 실행 컴포넌트를 정의하며, 공통 인터페이스, 기반 구현체, 레지스트리, 포트 시스템, 6종의 내장 노드 타입을 포함한다.

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

    +------------------------------------------+
    |              Registry                     |
    |  NodeFactory 등록/조회, 빌트인 자동 등록   |
    +------------------------------------------+
```

**핵심 구성 요소**:

1. **Node Interface**: 모든 노드가 구현하는 공통 계약 (8개 메서드)
2. **BaseNode**: `*lifecycle.BaseLifecycle` 임베딩 기반 구현체, 포트/설정/로깅 관리
3. **NodePort**: 런타임 포트 정보 (`flow.Port` 정적 정의와 분리, `Connected` 상태 포함)
4. **Registry**: `NodeFactory` 기반 노드 타입 등록/조회, 6개 빌트인 자동 등록
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

## 내장 노드 타입 (6종)

| 타입 이름 | 구조체 | 용도 |
|-----------|--------|------|
| `filter` | `FilterNode` | 조건 기반 메시지 필터링 (조건 불일치 시 폐기) |
| `transform` | `TransformNode` | 메시지 Payload 변환 (필드 추가/삭제/변환) |
| `switch` | `SwitchNode` | 조건별 출력 포트 라우팅 (First-Match, 기본 라우트) |
| `bridge` | `BridgeNode` | Agent-Flow 간 브릿지 (In/Out/InOut/RequestReply 4모드) |
| `script` | `ScriptNode` | ScriptEngine 기반 스크립트 메시지 처리 |
| `catch` | `CatchNode` | 에러 포트 메시지 수신 및 심각도/범주 기반 필터링 |

## 파일 구조

```
internal/node/
  base.go               # Node 인터페이스 + BaseNode + NodePort + NodeOption
  base_test.go           # BaseNode 단위 테스트
  registry.go            # Registry + NodeFactory + 빌트인 등록
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
  errors.go              # 12개 sentinel 에러 + NodeError 구조체
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
// 레지스트리 생성 (빌트인 6종 자동 등록)
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

## 이연 모듈 (P2)

다음 모듈은 향후 별도 구현으로 이연되었다:

| 모듈 | 타입 | 설명 |
|------|------|------|
| Module 7 | `AggregateNode` | 시간/카운트 기반 메시지 집계 |
| Module 10 | `DebugNode` | 메시지 로깅 및 디버깅 |
| Module 12 | `StatusNode` | 상태 이벤트 모니터링 |
| Module 13 | `DeadLetterNode` | 폐기 메시지 수집 |

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

- 커버리지: 93.7%
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
