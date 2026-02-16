# node - Bridge Node 시스템

`internal/node/` 패키지의 Bridge Node는 xflow 엔진에서 Agent와 Flow를 연결하는 브릿지 노드를 제공한다. 4가지 통신 모드(In, Out, InOut, RequestReply)를 지원하며, AgentResolver/AgentTransport 인터페이스 기반 에이전트 통신, 메시지 변환, Correlation ID 추적, 원자적 통계 수집 기능을 통합한다.

**SPEC**: SPEC-BRIDGE-001

## 아키텍처 개요

```
    Bridge Node 통합 아키텍처 (Agent-Flow 연결)

    +-----------------------------------------------------------+
    |                      BridgeNode                           |
    |  BaseNode 임베딩 (생명주기 관리)                             |
    |  Node 인터페이스 구현 (Init/Process/Shutdown)               |
    +-----------------------------------------------------------+
            |               |              |             |
    +-------v------+ +-----v-----+ +------v------+ +---v--------+
    | AgentResolver| | Transformer| | Correlation | |   Stats    |
    | 에이전트 해석  | | 메시지 변환 | | ID 추적     | | 원자적 수집 |
    +--------------+ +-----------+ +-------------+ +------------+
            |
    +-------v--------+
    | AgentTransport  |
    | Send / Receive  |
    +----------------+

    4가지 통신 모드:

    BridgeIn:           Agent --> [수신루프] --> recvCh --> Flow
    BridgeOut:          Flow  --> [Process]  --> Agent
    BridgeInOut:        Agent <--> [양방향]  <--> Flow
    BridgeRequestReply: Flow  --> [Correlation] --> Agent --> 응답 대기
```

**핵심 구성 요소**:

1. **BridgeNode**: BaseNode 임베딩, 4가지 모드별 Process 분기, 수신 루프 goroutine
2. **BridgeConfig**: Agent 참조, 통신 방향, 변환 설정, 타임아웃, 버퍼 크기 등 통합 설정
3. **BridgeTransformer**: 에이전트 데이터와 플로우 메시지 간 변환 인터페이스
4. **DefaultTransformer**: `_raw` 키 패턴 기반 기본 변환기
5. **CorrelationTracker**: RequestReply 모드의 요청-응답 상관관계 추적
6. **BridgeStatsSnapshot**: 원자적 카운터 기반 통계 스냅샷
7. **AgentResolver/AgentTransport**: 에이전트 해석 및 통신 추상화 인터페이스

## 빠른 시작

### BridgeNode 생성 및 초기화

`NewBridgeNode()` 함수는 `flow.NodeDef`와 NodeOption으로 BridgeNode를 생성한다. `Init()`으로 초기화하면 에이전트 해석, transport 설정, 수신 루프(In/InOut 모드) 또는 클린업 루프(RequestReply 모드)가 시작된다.

```go
package main

import (
    "context"
    "time"

    "github.com/xtra/xflow/internal/node"
    "github.com/xtra/xflow/pkg/flow"
)

func main() {
    ctx := context.Background()

    // NodeDef에 AgentRef 설정
    def := flow.NodeDef{
        ID:   "bridge-1",
        Type: "bridge",
        AgentRef: &flow.AgentRef{
            AgentName: "my-agent",
            Direction: flow.BridgeOut,
        },
    }

    // BridgeNode 생성 (옵션 패턴)
    bridgeNode, err := node.NewBridgeNode(def,
        node.WithAgentResolver(myResolver),   // 에이전트 해석기
        node.WithBridgeConfig(node.BridgeConfig{  // 전체 설정 지정
            AgentRef:  *def.AgentRef,
            Direction: flow.BridgeOut,
            Transform: node.TransformConfig{Mode: "auto"},
            RequestTimeout:       30 * time.Second,
            ReconnectInterval:    5 * time.Second,
            MaxReconnectAttempts: 10,
            BufferSize:           256,
        }),
    )
    if err != nil {
        panic(err)
    }

    // 초기화 (에이전트 해석 + transport 설정 + 루프 시작)
    if err := bridgeNode.Init(ctx); err != nil {
        panic(err)
    }
}
```

### 통신 모드별 동작

**BridgeOut (Flow -> Agent)**: `Process()`가 입력 메시지를 변환 검증 후 에이전트에 전송한다.

**BridgeIn (Agent -> Flow)**: 수신 루프 goroutine이 `transport.Receive()`를 폴링하여 `recvCh` 버퍼에 메시지를 전달한다.

**BridgeInOut (양방향)**: BridgeIn의 수신 루프와 BridgeOut의 송신 처리를 동시에 수행한다.

**BridgeRequestReply (요청-응답)**: Correlation ID(UUID v4)를 생성하여 요청을 추적하고, 타임아웃 내 응답을 대기한다.

## API 레퍼런스

### AgentResolver 인터페이스

에이전트 참조를 실제 AgentTransport로 해석하는 인터페이스이다.

```go
type AgentResolver interface {
    ResolveAgent(ctx context.Context, ref flow.AgentRef) (AgentTransport, error)
}
```

### AgentTransport 인터페이스

에이전트와의 통신을 담당하는 인터페이스이다.

```go
type AgentTransport interface {
    Send(ctx context.Context, msg message.Message) error
    Receive(ctx context.Context) (message.Message, error)
}
```

### BridgeTransformer 인터페이스

에이전트 데이터와 플로우 메시지 간 변환을 담당하는 인터페이스이다.

```go
type BridgeTransformer interface {
    AgentToFlow(data []byte) (message.Message, error)
    FlowToAgent(msg message.Message) ([]byte, error)
}
```

### BridgeConfig 구조체

| 필드 | 타입 | 기본값 | 설명 |
|------|------|--------|------|
| `AgentRef` | `flow.AgentRef` | - | 연결 대상 에이전트 참조 |
| `Direction` | `flow.BridgeDirection` | AgentRef.Direction | 통신 방향 (In/Out/InOut/RequestReply) |
| `Transform` | `TransformConfig` | Mode: "auto" | 메시지 변환 설정 |
| `RequestTimeout` | `time.Duration` | 30s | RequestReply 타임아웃 |
| `ReconnectInterval` | `time.Duration` | 5s | 재연결 시도 간격 |
| `MaxReconnectAttempts` | `int` | 10 | 최대 재연결 시도 횟수 |
| `BufferSize` | `int` | 256 | 수신 버퍼 크기 (최소 1) |
| `ResponseTarget` | `string` | "" | 응답 전달 대상 노드 ID |

### TransformConfig 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `Mode` | `string` | 변환 모드 ("auto" 또는 "lua") |
| `LuaScript` | `string` | Lua 스크립트 경로 (Mode가 "lua"일 때 필수) |

### BridgeNode 메서드

| 메서드 | 설명 |
|--------|------|
| `NewBridgeNode(def, opts...)` | NodeDef와 옵션으로 BridgeNode 생성 |
| `Init(ctx)` | 에이전트 해석, transport 설정, 루프 시작 |
| `Process(ctx, msg)` | 방향에 따라 메시지 처리 |
| `Shutdown(ctx)` | 루프 취소, Correlation 정리, 연결 해제 |
| `Configure(config)` | 런타임 설정 변경 |
| `Info()` | 런타임 상태 정보 스냅샷 반환 |
| `Stats()` | 통계 데이터 스냅샷 반환 |

### NodeOption 함수

| 옵션 | 설명 |
|------|------|
| `WithAgentResolver(resolver)` | AgentResolver 설정 |
| `WithReplyTimeout(duration)` | RequestReply 타임아웃 설정 (WithBridgeConfig 미사용 시) |
| `WithBridgeConfig(config)` | 전체 BridgeConfig 설정 (최고 우선순위) |

### BridgeInfo 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `AgentID` | `string` | 연결된 에이전트 ID |
| `AgentName` | `string` | 연결된 에이전트 이름 |
| `Direction` | `flow.BridgeDirection` | 브릿지 방향 |
| `Connected` | `bool` | 에이전트 연결 상태 |
| `Stats` | `BridgeStatsSnapshot` | 통계 스냅샷 |

### BridgeStatsSnapshot 구조체

| 필드 | 타입 | 설명 |
|------|------|------|
| `MessagesRelayed` | `int64` | 릴레이된 총 메시지 수 |
| `MessagesFromAgent` | `int64` | 에이전트로부터 수신한 메시지 수 |
| `MessagesToAgent` | `int64` | 에이전트로 전송한 메시지 수 |
| `TransformErrors` | `int64` | 변환 에러 누적 횟수 |
| `CorrelationTimeouts` | `int64` | 상관관계 타임아웃 누적 횟수 |
| `PendingCorrelations` | `int` | 현재 대기 중인 상관관계 수 |
| `AvgRelayLatency` | `time.Duration` | 평균 릴레이 지연시간 |
| `LastActivityAt` | `time.Time` | 마지막 활동 시각 |

### CorrelationTracker 메서드

| 메서드 | 설명 |
|--------|------|
| `NewCorrelationTracker(timeout)` | 기본 타임아웃으로 생성 |
| `Track(correlationID, responseCh)` | 요청 추적 등록 |
| `Resolve(correlationID, response)` | 응답 매칭 및 전달 (성공 시 true) |
| `SetTimeout(duration)` | 새 항목의 기본 타임아웃 변경 |
| `Cleanup()` | 만료된 항목 제거 및 채널 닫기 |
| `PendingCount()` | 대기 중인 항목 수 반환 |
| `TimeoutCount()` | 누적 타임아웃 횟수 반환 |
| `Close()` | 모든 대기 항목 정리 및 종료 (중복 호출 안전) |
| `StartCleanupLoop(ctx)` | 주기적 클린업 goroutine 시작 (간격: timeout/2, 최소 10ms) |

## 센티널 에러

| 에러 변수 | 메시지 | 발생 조건 |
|-----------|--------|----------|
| `ErrAgentNotRunning` | bridge: agent not running | 바인딩된 에이전트가 실행 상태가 아님 |
| `ErrAgentDisconnected` | bridge: agent disconnected | 에이전트 연결 끊김 |
| `ErrCorrelationNotFound` | bridge: correlation ID not found | 등록되지 않은 상관관계 ID 조회 |
| `ErrTransformFailed` | bridge: message transform failed | 메시지 변환 실패 |
| `ErrInvalidDirection` | bridge: invalid bridge direction | 유효하지 않은 BridgeDirection |
| `ErrMaxReconnectExceeded` | bridge: max reconnect attempts exceeded | 최대 재연결 시도 초과 |
| `ErrBufferFull` | bridge: receive buffer full | 수신 버퍼 가득 참 |

## 설계 결정

### AgentResolver/AgentTransport 인터페이스 패턴

BridgeNode는 Agent 패키지에 직접 의존하지 않고, `AgentResolver`와 `AgentTransport` 인터페이스를 통해 에이전트와 통신한다. 이 설계는 다음과 같은 이점을 제공한다:

- **순환 의존 방지**: `internal/node/` -> `internal/agent/` 직접 참조 없이 인터페이스 기반 연결
- **테스트 용이성**: mock AgentResolver/AgentTransport로 에이전트 없이 단위 테스트 가능
- **확장성**: 다양한 에이전트 구현(로컬, 원격, gRPC 등)을 동일 인터페이스로 지원

### SharedRef 미사용 결정

SPEC에서 정의한 `SharedRef.Acquire/Release` 패턴 대신, `AgentResolver.ResolveAgent()`가 참조 관리를 추상화한다. AgentResolver 구현체가 내부적으로 참조 카운팅을 처리하므로, BridgeNode는 참조 관리 세부사항에 관여하지 않는다.

### 원자적 통계 수집

`bridgeStatsCollector`는 모든 카운터를 `sync/atomic` 타입으로 관리하여, 별도의 뮤텍스 없이 동시성 안전한 통계 수집을 보장한다. `Snapshot()` 메서드로 읽기 전용 스냅샷을 생성하여 외부에 제공한다.

### DefaultTransformer의 `_raw` 키 패턴

DefaultTransformer는 에이전트 `[]byte` 데이터를 메시지 Payload의 `_raw` 키에 저장한다. 역변환 시 `_raw` 키의 `[]byte` 또는 `string` 값을 추출하고, 해당 키가 없으면 전체 Payload를 JSON으로 직렬화하여 반환한다. 이 패턴은 바이트 데이터의 무손실 전달을 보장하면서 JSON 폴백을 제공한다.

## 파일 구조

```
internal/node/
  bridge_errors.go           # Bridge 전용 센티널 에러 정의 (7개)
  bridge_config.go           # BridgeConfig, TransformConfig, DefaultBridgeConfig(), Validate()
  bridge_transform.go        # BridgeTransformer 인터페이스, DefaultTransformer 구현
  bridge_correlation.go      # CorrelationTracker 구조체 (Track/Resolve/Cleanup/Close)
  bridge_info.go             # BridgeInfo, BridgeStatsSnapshot, bridgeStatsCollector
  bridge.go                  # BridgeNode 구조체, AgentResolver/AgentTransport 인터페이스,
                             # NewBridgeNode, Init, Process, Shutdown, Info, Stats
  bridge_errors_test.go      # 센티널 에러 테스트
  bridge_config_test.go      # BridgeConfig 검증 테스트
  bridge_transform_test.go   # DefaultTransformer 변환 테스트
  bridge_correlation_test.go # CorrelationTracker 테스트
  bridge_info_test.go        # BridgeInfo/BridgeStats 테스트
  bridge_test.go             # BridgeNode 통합 테스트
```

## 의존성

- **표준 라이브러리**: `context`, `sync`, `sync/atomic`, `time`, `fmt`, `errors`
- **외부 의존성**:
  - `github.com/google/uuid` - Correlation ID 생성 (UUID v4)
- **내부 의존성**:
  - `internal/node/` (SPEC-NODE-001) - BaseNode 임베딩, Node 인터페이스, NodeOption
  - `pkg/flow/` (SPEC-FLOW-001) - NodeDef, AgentRef, BridgeDirection
  - `pkg/message/` (SPEC-MSG-001) - Message 인터페이스
  - `pkg/lifecycle/` (SPEC-LIFE-001) - 생명주기 상태 전이

## 테스트

```bash
# Bridge 관련 테스트 실행
go test ./internal/node/ -run "Bridge|Correlation|DefaultTransformer" -v

# Race Detector 포함 테스트
go test -race ./internal/node/ -run "Bridge|Correlation|DefaultTransformer"

# 커버리지 확인
go test -cover ./internal/node/ -run "Bridge|Correlation|DefaultTransformer"

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/node/
go tool cover -func=cover.out | grep bridge
```

### 테스트 결과

- 테스트: 117개 전체 통과
- 주요 함수 커버리지: NewBridgeNode 95.7%, Init 94.7%, Process 83.3%, Shutdown 100%, Validate 100%, CorrelationTracker 85.7%~100%
- Race Detector: 이상 없음 (go test -race)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-NODE-001 | 상위 | BridgeNode의 기본 인터페이스 정의, BaseNode 임베딩 |
| SPEC-AGENT-001 | 의존 | Agent 인터페이스 (AgentResolver/AgentTransport로 추상화) |
| SPEC-MSG-001 | 의존 | Message 인터페이스 (Bridge 입출력 데이터) |
| SPEC-FLOW-001 | 의존 | NodeDef, AgentRef, BridgeDirection 데이터 구조 |
| SPEC-LIFE-001 | 의존 | 생명주기 상태 전이 (StateInitializing, StateRunning, StateStopping) |
| SPEC-ENGINE-001 | 소비자 | Engine이 BridgeNode를 포함한 노드 그래프 실행 |
