---
id: SPEC-BRIDGE-001
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-BRIDGE-001: Bridge Node System - Agent-Flow 연결 브릿지 노드, 메시지 변환, Correlation 관리

## 1. Environment (환경)

### 1.1 시스템 개요

Bridge Node는 Agent와 Flow를 연결하는 전용 노드이다. Agent는 플로우와 독립적으로 실행되므로, Bridge Node가 두 시스템 간 메시지 교환을 중재한다. 4가지 통신 모드(In, Out, InOut, Request-Reply)를 지원하며, Agent의 프로토콜 데이터와 플로우의 Message 구조체 간 변환을 수행한다.

**SPEC-NODE-001과의 관계**: SPEC-NODE-001의 Module 8에서 BridgeNode의 기본 인터페이스(Init, Process, Shutdown, 4개 모드)를 정의하였다. 본 SPEC-BRIDGE-001은 Bridge Node의 **상세 구현 요구사항**을 정의한다:

- Agent 바인딩 및 연결 관리 (해상도, 생명주기 연동)
- 메시지 변환 레이어 (자동 매핑, Lua 커스텀 변환)
- Request-Reply Correlation ID 관리 (생성, 매칭, 타임아웃)
- 멀티 브릿지 (하나의 Agent에 여러 Bridge Node 연결)
- Bridge 설정 및 통계
- 에러 처리 및 복구

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/node/` (bridge.go, bridge_transform.go, bridge_correlation.go)
- **Tier**: Tier 2 - 내부 실행 계층 (internal)
- **의존 패키지**:
  - `internal/node/` (SPEC-NODE-001): BaseNode, Node 인터페이스, NodeOption, Registry
  - `internal/agent/` (SPEC-AGENT-001): Agent 인터페이스, Registry, SharedRef
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스
  - `pkg/flow/` (SPEC-FLOW-001): NodeDef, Port, PortDirection, AgentRef, BridgeDirection
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent 타입
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 인터페이스
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**:
  - `sync.Map` (Correlation ID 추적)
  - `sync.RWMutex` (Agent 바인딩 상태 보호)
  - `sync/atomic` (BridgeStats 카운터)

### 1.3 설계 원칙

- **SPEC-NODE-001 준수**: BridgeNode는 Node 인터페이스를 구현하며, BaseNode을 임베딩하여 공통 생명주기 로직을 재사용
- **Agent 독립성**: Agent의 생명주기에 의존하지 않되, Agent 상태 변경에 반응하여 Bridge 동작을 조정
- **변환 추상화**: BridgeTransformer 인터페이스로 Agent 프로토콜 데이터와 Flow Message 간 변환 로직을 분리
- **Correlation 안전성**: Request-Reply 모드에서 Correlation ID의 생성, 매칭, 타임아웃, 정리를 체계적으로 관리
- **멀티캐스트 지원**: 하나의 Agent에 여러 Bridge Node가 연결되어 동시 수신 가능
- **관찰성 내장**: 모든 Bridge 동작에 대해 메트릭(중계 수, 변환 에러, Correlation 타임아웃)을 자동 기록

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- BridgeNode 상세 구현 (Agent 바인딩, 4개 모드 처리 로직)
- BridgeConfig 구조체 (Agent 참조 설정, 통신 모드, 변환 설정, 타임아웃)
- Agent 바인딩 및 연결 관리 (해상도, SharedRef.Acquire/Release, 재바인딩)
- BridgeTransformer 인터페이스 및 구현체 (DefaultTransformer, LuaTransformer)
- CorrelationTracker 구조체 (Request-Reply Correlation ID 관리)
- BridgeInfo 및 BridgeStats 구조체 (상태/통계 스냅샷)
- Bridge 전용 sentinel 에러 타입

**OUT OF SCOPE (별도 SPEC)**:
- Node 인터페이스, BaseNode 기반 구조체 정의 (SPEC-NODE-001: Module 1)
- Node Registry에 bridge 팩토리 등록 (SPEC-NODE-001: Module 2)
- Agent 인터페이스, Manager, SharedRef 구조체 정의 (SPEC-AGENT-001)
- Engine의 노드 goroutine 실행 및 Wire 시스템 (SPEC-ENGINE-001)
- Message 데이터 구조 정의 (SPEC-MSG-001)
- Flow 데이터 구조 정의 (SPEC-FLOW-001)
- Lua 스크립트 엔진 구현 (별도 SPEC: `internal/script/`)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-NODE-001 | 상위 | BridgeNode의 기본 인터페이스 정의 (Module 8), BaseNode 임베딩 |
| SPEC-AGENT-001 | 의존 | Agent 인터페이스, Registry, SharedRef (참조 카운팅) |
| SPEC-MSG-001 | 의존 | Message 인터페이스 (Bridge 입출력 데이터) |
| SPEC-FLOW-001 | 의존 | NodeDef, AgentRef, BridgeDirection 데이터 구조 |
| SPEC-ERR-001 | 의존 | ErrorMessage, StatusEvent 타입 |
| SPEC-LIFE-001 | 의존 | State 인터페이스 (Agent 상태 감시) |
| SPEC-ENGINE-001 | 소비자 | Engine이 BridgeNode를 포함한 노드 그래프 실행 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: SPEC-NODE-001의 BaseNode, Node 인터페이스, NodeOption 패턴이 구현되어 있다
- A2: SPEC-AGENT-001의 Agent 인터페이스와 Registry.Get(name string) 메서드가 구현되어 있다
- A3: SPEC-AGENT-001의 SharedRef 구조체와 Acquire/Release 메서드가 구현되어 있다
- A4: Agent.Process(data []byte) ([]byte, error) 메서드가 Agent 프로토콜 데이터를 처리한다
- A5: flow.BridgeDirection 열거형(BridgeIn, BridgeOut, BridgeInOut, BridgeRequestReply)이 SPEC-FLOW-001에서 정의되어 있다
- A6: flow.AgentRef 구조체(AgentID, AgentName 필드)가 SPEC-FLOW-001에서 정의되어 있다
- A7: `internal/script/` 패키지가 Lua VM 인터페이스를 제공하며, LuaTransformer가 이를 소비한다

### 2.2 도메인 가정

- A8: 하나의 Agent에 여러 Bridge Node가 연결될 수 있으며, Agent는 모든 연결된 Bridge에 동일 데이터를 전달한다
- A9: BridgeIn 모드에서 Agent의 수신 데이터는 Agent.Process() 결과를 기반으로 하며, Bridge Node가 별도 goroutine에서 폴링한다
- A10: Request-Reply 모드의 Correlation ID는 UUID v4를 사용하며, 충돌 확률은 무시 가능하다
- A11: DefaultTransformer는 Agent의 []byte 데이터를 Message.Payload에 직접 할당하는 패스스루 방식이다
- A12: LuaTransformer의 변환 스크립트는 Bridge 설정 시 1회 로드되며, Configure()로 변경 가능하다
- A13: Agent 상태 변경(Running -> Paused -> Stopped) 시 Bridge Node는 해당 모드의 수신/송신을 일시 중지 또는 중단한다
- A14: Agent 연결 끊김 시 재바인딩은 설정된 간격과 최대 횟수 제한 내에서 시도한다

---

## 3. Requirements (요구사항)

### Module 1: BridgeNode Core - 브릿지 노드 코어 (P0)

#### REQ-BRIDGE-001-01-01 (Ubiquitous) BridgeNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 다음 필드를 포함하는 `BridgeNode` 구조체를 제공해야 한다:

- Agent 바인딩 참조 (Agent 인터페이스)
- SharedRef 참조 (참조 카운팅)
- BridgeConfig (설정)
- BridgeTransformer (메시지 변환기)
- CorrelationTracker (Request-Reply 모드 전용)
- BridgeStats (처리 통계)
- 수신 루프 관리용 컨텍스트 및 취소 함수

#### REQ-BRIDGE-001-01-02 (Ubiquitous) NewBridgeNode 생성자

시스템은 **항상** `NewBridgeNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` 생성자를 제공해야 한다:

- `flow.NodeDef`에서 AgentRef, BridgeDirection, Config 정보를 추출
- BridgeConfig 초기화 및 검증
- BridgeTransformer 초기화 (설정에 따라 DefaultTransformer 또는 LuaTransformer)
- Request-Reply 모드인 경우 CorrelationTracker 초기화

#### REQ-BRIDGE-001-01-03 (Event-Driven) Bridge 초기화

**WHEN** `BridgeNode.Init(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. Agent Registry에서 AgentRef 기반 Agent 해상도
2. Agent SharedRef.Acquire(flowID) 호출 (참조 카운트 증가)
3. Agent 상태 확인 (Running 상태가 아니면 대기 또는 에러)
4. BridgeIn/BridgeInOut 모드인 경우 수신 루프 goroutine 시작
5. Request-Reply 모드인 경우 Correlation 만료 정리 goroutine 시작
6. 상태를 StateRunning으로 전이

#### REQ-BRIDGE-001-01-04 (State-Driven) 통신 모드별 Process 분기

**IF** BridgeDirection이 BridgeIn이면, **THEN** Process()는 Agent 수신 루프에서 전달된 메시지를 출력 포트로 반환해야 한다.

**IF** BridgeDirection이 BridgeOut이면, **THEN** Process()는 입력 메시지를 Agent로 전달해야 한다.

**IF** BridgeDirection이 BridgeInOut이면, **THEN** Process()는 입력 메시지를 Agent로 전달하고, 동시에 수신 루프에서 Agent 데이터를 출력 포트로 전달해야 한다.

**IF** BridgeDirection이 BridgeRequestReply이면, **THEN** Process()는 Correlation ID를 생성하여 요청을 전달하고, 응답 대기 후 반환해야 한다.

#### REQ-BRIDGE-001-01-05 (Event-Driven) Bridge 종료

**WHEN** `BridgeNode.Shutdown(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 수신 루프 goroutine 중지 (컨텍스트 취소)
2. Request-Reply 모드의 대기 중인 모든 Correlation 타임아웃 처리
3. Correlation 만료 정리 goroutine 중지
4. Agent SharedRef.Release(flowID) 호출 (참조 카운트 감소)
5. Agent 바인딩 해제
6. 상태를 StateStopped로 전이

---

### Module 2: Agent Binding - Agent 바인딩 (P0)

#### REQ-BRIDGE-001-02-01 (Event-Driven) Agent 이름 기반 해상도

**WHEN** BridgeConfig.AgentRef에 AgentName이 설정되어 있으면, **THEN** Agent Registry에서 이름으로 Agent를 조회해야 한다. Agent를 찾을 수 없으면 `ErrAgentNotFound` 에러를 반환해야 한다.

#### REQ-BRIDGE-001-02-02 (Event-Driven) Agent ID 기반 해상도

**WHEN** BridgeConfig.AgentRef에 AgentID가 설정되어 있으면, **THEN** Agent Registry에서 ID로 Agent를 조회해야 한다. Agent를 찾을 수 없으면 `ErrAgentNotFound` 에러를 반환해야 한다.

#### REQ-BRIDGE-001-02-03 (Event-Driven) SharedRef 참조 획득

**WHEN** Agent 해상도가 성공하면, **THEN** Agent의 SharedRef.Acquire(flowID)를 호출하여 참조 카운트를 증가시켜야 한다.

#### REQ-BRIDGE-001-02-04 (Event-Driven) SharedRef 참조 해제

**WHEN** Bridge Node가 Shutdown되면, **THEN** Agent의 SharedRef.Release(flowID)를 호출하여 참조 카운트를 감소시켜야 한다.

#### REQ-BRIDGE-001-02-05 (State-Driven) Agent 상태 변경 감지

**IF** 바인딩된 Agent의 상태가 Running에서 Paused로 변경되면, **THEN** Bridge Node는 수신/송신을 일시 중지해야 한다.

**IF** 바인딩된 Agent의 상태가 Paused에서 Running으로 변경되면, **THEN** Bridge Node는 수신/송신을 재개해야 한다.

**IF** 바인딩된 Agent의 상태가 Stopped로 변경되면, **THEN** Bridge Node는 에러 포트로 `ErrAgentDisconnected`를 전달하고 재바인딩을 시도해야 한다.

#### REQ-BRIDGE-001-02-06 (Event-Driven) Agent 재바인딩

**WHEN** Agent 연결이 끊기면, **THEN** 다음 절차로 재바인딩을 시도해야 한다:

1. ReconnectInterval 간격으로 Agent Registry 재조회
2. Agent 발견 시 SharedRef.Acquire 및 연결 재설정
3. MaxReconnectAttempts 초과 시 `ErrMaxReconnectExceeded` 에러를 에러 포트로 전달하고 재바인딩 중단

#### REQ-BRIDGE-001-02-07 (Ubiquitous) 멀티 브릿지 지원

시스템은 **항상** 하나의 Agent에 여러 Bridge Node가 동시에 연결되는 것을 허용해야 한다. 각 Bridge Node는 독립적으로 Agent의 SharedRef.Acquire/Release를 관리한다.

---

### Module 3: Communication Modes - 통신 모드 (P0)

#### REQ-BRIDGE-001-03-01 (State-Driven) BridgeIn 모드 수신 루프

**IF** BridgeDirection이 BridgeIn이면, **THEN** Bridge Node는 별도 goroutine에서 Agent의 수신 데이터를 폴링하는 수신 루프를 실행해야 한다:

1. Agent.Process()로 수신 데이터 획득
2. BridgeTransformer.AgentToFlow()로 Message 변환
3. 변환된 Message를 출력 포트로 전달
4. MessagesFromAgent 카운터 증가

#### REQ-BRIDGE-001-03-02 (State-Driven) BridgeOut 모드 송신 처리

**IF** BridgeDirection이 BridgeOut이면, **THEN** Bridge Node의 Process()는 입력 메시지를 다음과 같이 처리해야 한다:

1. BridgeTransformer.FlowToAgent()로 Agent 프로토콜 데이터 변환
2. Agent에 변환된 데이터 전달
3. MessagesToAgent 카운터 증가

#### REQ-BRIDGE-001-03-03 (State-Driven) BridgeInOut 모드 양방향 처리

**IF** BridgeDirection이 BridgeInOut이면, **THEN** Bridge Node는 다음을 동시에 수행해야 한다:

1. 수신 루프: BridgeIn 모드와 동일한 Agent 수신 처리
2. Process(): BridgeOut 모드와 동일한 Agent 송신 처리
3. 입력/출력 포트 동시 활성화

#### REQ-BRIDGE-001-03-04 (State-Driven) BridgeRequestReply 모드 요청-응답 처리

**IF** BridgeDirection이 BridgeRequestReply이면, **THEN** Bridge Node의 Process()는 다음을 수행해야 한다:

1. Correlation ID(UUID v4) 생성
2. 메시지 Metadata에 `_correlationID` 설정
3. CorrelationTracker.Track(correlationID, responseCh) 호출
4. BridgeTransformer.FlowToAgent()로 변환 후 Agent에 요청 전달
5. responseCh에서 응답 대기 (select + timeout)
6. 응답 수신 시: BridgeTransformer.AgentToFlow()로 변환 후 반환
7. 타임아웃 시: ErrRequestTimeout 에러 포트 전달

#### REQ-BRIDGE-001-03-05 (Event-Driven) 수신 버퍼 관리

**WHEN** BridgeIn 또는 BridgeInOut 모드에서 수신 루프가 메시지를 생성하면, **THEN** BufferSize 설정에 따른 크기의 채널 버퍼에 메시지를 저장해야 한다.

#### REQ-BRIDGE-001-03-06 (Unwanted) 수신 버퍼 초과

시스템은 수신 버퍼가 가득 찬 경우 `ErrBufferFull` 에러를 에러 포트로 전달하고 **해당 메시지를 폐기해야 한다**. 버퍼 풀 메트릭 카운터를 증가시켜야 한다.

---

### Module 4: Message Transformation - 메시지 변환 (P1)

#### REQ-BRIDGE-001-04-01 (Ubiquitous) BridgeTransformer 인터페이스

시스템은 **항상** 다음 메서드를 포함하는 `BridgeTransformer` 인터페이스를 제공해야 한다:

- `AgentToFlow(data []byte) (message.Message, error)`: Agent 프로토콜 데이터를 Flow Message로 변환
- `FlowToAgent(msg message.Message) ([]byte, error)`: Flow Message를 Agent 프로토콜 데이터로 변환

#### REQ-BRIDGE-001-04-02 (Ubiquitous) DefaultTransformer 구현

시스템은 **항상** 기본 변환기인 `DefaultTransformer`를 제공해야 한다:

- AgentToFlow: `[]byte` 데이터를 새 Message의 Payload에 `_raw` 키로 설정
- FlowToAgent: Message의 Payload에서 `_raw` 키의 `[]byte` 값을 추출하여 반환

#### REQ-BRIDGE-001-04-03 (Optional) LuaTransformer 구현

**가능하면** Lua 스크립트 기반 커스텀 변환을 수행하는 `LuaTransformer`를 제공해야 한다:

- `internal/script/` 패키지의 Lua VM 참조
- AgentToFlow: Lua 스크립트에 `[]byte` 입력을 전달하고, 반환된 Lua 테이블을 Message Payload로 변환
- FlowToAgent: Lua 스크립트에 Message Payload를 Lua 테이블로 전달하고, 반환된 `[]byte`를 추출
- TransformConfig.LuaScript 경로에서 스크립트 로드

#### REQ-BRIDGE-001-04-04 (Event-Driven) 변환 에러 처리

**WHEN** BridgeTransformer의 AgentToFlow 또는 FlowToAgent 실행 중 에러가 발생하면, **THEN** `ErrTransformFailed` 에러를 에러 포트로 전달하고, TransformErrors 카운터를 증가시켜야 한다.

#### REQ-BRIDGE-001-04-05 (Event-Driven) 변환기 Hot Reload

**WHEN** `BridgeNode.Configure(config)` 호출 시 `"transform"` 설정이 변경되면, **THEN** BridgeTransformer를 새 설정에 맞게 재초기화해야 한다. LuaTransformer의 경우 Lua VM을 재생성하고 새 스크립트를 로드해야 한다.

---

### Module 5: Request-Reply Correlation - 요청-응답 Correlation 관리 (P1)

#### REQ-BRIDGE-001-05-01 (Ubiquitous) CorrelationTracker 구조체

시스템은 **항상** 다음 메서드를 포함하는 `CorrelationTracker` 구조체를 제공해야 한다:

- `Track(correlationID string, responseCh chan message.Message)`: 요청 추적 등록
- `Resolve(correlationID string, response message.Message) bool`: 응답 매칭 및 전달
- `Timeout(d time.Duration)`: 전역 타임아웃 설정
- `Cleanup()`: 만료된 추적 항목 정리
- `PendingCount() int`: 대기 중인 요청 수 반환
- `Close()`: 모든 대기 항목 정리 및 트래커 종료

#### REQ-BRIDGE-001-05-02 (Ubiquitous) Correlation ID 생성

시스템은 **항상** Correlation ID를 UUID v4로 생성해야 한다.

#### REQ-BRIDGE-001-05-03 (Ubiquitous) 동시성 안전 관리

시스템은 **항상** CorrelationTracker 내부의 Correlation ID 매핑을 `sync.Map`으로 관리하여 동시성 안전을 보장해야 한다.

#### REQ-BRIDGE-001-05-04 (Event-Driven) 응답 매칭

**WHEN** Agent로부터 응답이 수신되고, 메시지 Metadata의 `_correlationID`가 등록된 Correlation ID와 일치하면, **THEN** 해당 responseCh에 응답을 전달하고 매핑을 삭제해야 한다.

#### REQ-BRIDGE-001-05-05 (Unwanted) Correlation ID 타임아웃

시스템은 Track()으로 등록된 항목이 설정된 타임아웃(기본 30초) 내에 Resolve되지 않으면, responseCh를 닫고 매핑을 삭제하며, CorrelationTimeouts 카운터를 증가시키고, `ErrRequestTimeout`을 에러 포트로 전달**해야 한다**.

#### REQ-BRIDGE-001-05-06 (Unwanted) 존재하지 않는 Correlation ID 매칭 시도

시스템은 Resolve() 호출 시 등록되지 않은 correlationID가 전달되면 `ErrCorrelationNotFound` 에러를 반환하고 **매칭을 수행하지 않아야 한다**.

#### REQ-BRIDGE-001-05-07 (Event-Driven) 정기적 만료 항목 정리

**WHEN** Bridge Node가 Request-Reply 모드로 초기화되면, **THEN** 별도 goroutine에서 주기적(RequestTimeout / 2 간격)으로 만료된 Correlation 항목을 정리해야 한다.

---

### Module 6: Bridge Configuration - 브릿지 설정 (P1)

#### REQ-BRIDGE-001-06-01 (Ubiquitous) BridgeConfig 구조체

시스템은 **항상** 다음 필드를 포함하는 `BridgeConfig` 구조체를 제공해야 한다:

- `AgentRef flow.AgentRef` - Agent 이름 또는 ID
- `Direction flow.BridgeDirection` - 통신 모드 (In, Out, InOut, RequestReply)
- `Transform TransformConfig` - 변환 모드 및 설정
- `RequestTimeout time.Duration` - Request-Reply 타임아웃 (기본 30초)
- `ReconnectInterval time.Duration` - Agent 재연결 시도 간격 (기본 5초)
- `MaxReconnectAttempts int` - 최대 재연결 시도 횟수 (기본 10)
- `BufferSize int` - 수신 버퍼 크기 (기본 256)
- `ResponseTarget string` - Request-Reply 응답 전달 대상 노드 ID (비어있으면 요청 노드로 반환)

#### REQ-BRIDGE-001-06-02 (Ubiquitous) TransformConfig 구조체

시스템은 **항상** 다음 필드를 포함하는 `TransformConfig` 구조체를 제공해야 한다:

- `Mode string` - 변환 모드 ("auto" 또는 "lua")
- `LuaScript string` - Lua 스크립트 파일 경로 (Mode가 "lua"일 때 사용)

#### REQ-BRIDGE-001-06-03 (Event-Driven) BridgeConfig 검증

**WHEN** `BridgeConfig.Validate()` 호출 시, **THEN** 다음을 검증해야 한다:

1. AgentRef에 AgentName 또는 AgentID가 설정되어 있는지
2. Direction이 유효한 BridgeDirection 값인지
3. RequestTimeout이 양수인지 (Request-Reply 모드일 때)
4. MaxReconnectAttempts가 0 이상인지
5. BufferSize가 1 이상인지
6. Transform.Mode가 "auto" 또는 "lua"인지
7. Transform.Mode가 "lua"이고 LuaScript가 비어있지 않은지

#### REQ-BRIDGE-001-06-04 (Unwanted) 유효하지 않은 설정 거부

시스템은 `BridgeConfig.Validate()`에서 검증 실패 시 `ErrInvalidDirection` 또는 SPEC-NODE-001의 `ErrInvalidConfig` 에러를 반환하고 **Bridge Node 생성을 거부해야 한다**.

---

### Module 7: Bridge Info & Stats - 브릿지 정보 및 통계 (P1)

#### REQ-BRIDGE-001-07-01 (Ubiquitous) BridgeInfo 구조체

시스템은 **항상** Bridge 상태 스냅샷을 제공하는 `BridgeInfo` 구조체를 제공해야 한다:

- `AgentID string` - 바인딩된 Agent ID
- `AgentName string` - 바인딩된 Agent 이름
- `Direction flow.BridgeDirection` - 현재 통신 모드
- `Connected bool` - Agent 연결 상태
- `Stats BridgeStats` - 처리 통계

#### REQ-BRIDGE-001-07-02 (Ubiquitous) BridgeStats 구조체

시스템은 **항상** Bridge 처리 통계를 제공하는 `BridgeStats` 구조체를 제공해야 한다:

- `MessagesRelayed int64` - 중계된 총 메시지 수
- `MessagesFromAgent int64` - Agent에서 수신한 메시지 수
- `MessagesToAgent int64` - Agent로 송신한 메시지 수
- `TransformErrors int64` - 변환 에러 수
- `CorrelationTimeouts int64` - Request-Reply 타임아웃 수
- `PendingCorrelations int` - 현재 대기 중인 Correlation 수
- `AvgRelayLatency time.Duration` - 평균 중계 지연시간
- `LastActivityAt time.Time` - 마지막 활동 시각

#### REQ-BRIDGE-001-07-03 (Ubiquitous) Info() 및 Stats() 메서드

시스템은 **항상** `BridgeNode.Info() BridgeInfo` 및 `BridgeNode.Stats() BridgeStats` 메서드를 제공해야 한다.

#### REQ-BRIDGE-001-07-04 (Ubiquitous) 원자적 통계 업데이트

시스템은 **항상** BridgeStats의 카운터 필드를 `sync/atomic`으로 업데이트하여 동시성 안전을 보장해야 한다.

---

### Module 8: Error Types - 에러 타입 (P0)

#### REQ-BRIDGE-001-08-01 (Ubiquitous) Bridge 전용 Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러를 정의해야 한다:

- `ErrAgentNotFound` - Agent 바인딩 해상도 실패
- `ErrAgentNotRunning` - 바인딩된 Agent가 Running 상태가 아님
- `ErrAgentDisconnected` - Agent 연결 끊김
- `ErrRequestTimeout` - Request-Reply 타임아웃
- `ErrCorrelationNotFound` - Correlation ID 매칭 실패
- `ErrTransformFailed` - 메시지 변환 실패
- `ErrInvalidDirection` - 유효하지 않은 BridgeDirection
- `ErrMaxReconnectExceeded` - 최대 재연결 시도 초과
- `ErrBufferFull` - 수신 버퍼 가득 참

#### REQ-BRIDGE-001-08-02 (Ubiquitous) errors.Is() 호환성

시스템은 **항상** 모든 Bridge sentinel 에러가 `errors.Is()` 및 `fmt.Errorf("%w", ...)` 래핑과 호환되어야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/node/
├── bridge.go                // BridgeNode 구조체, NewBridgeNode, Init, Process, Shutdown
├── bridge_transform.go      // BridgeTransformer 인터페이스, DefaultTransformer, LuaTransformer
├── bridge_correlation.go    // CorrelationTracker 구조체
├── bridge_config.go         // BridgeConfig, TransformConfig, Validate()
├── bridge_info.go           // BridgeInfo, BridgeStats 구조체, Info(), Stats() 메서드
├── bridge_errors.go         // Bridge 전용 sentinel 에러 정의
├── bridge_test.go           // BridgeNode 통합 테스트
├── bridge_transform_test.go // 변환 테스트
├── bridge_correlation_test.go // Correlation 테스트
└── bridge_config_test.go    // 설정 검증 테스트
```

### 4.2 타입 시그니처

```go
package node

import (
    "context"
    "sync"
    "sync/atomic"
    "time"

    "xflow/internal/agent"
    "xflow/pkg/flow"
    "xflow/pkg/message"
)

// ── BridgeNode Core ──

type BridgeNode struct {
    *BaseNode
    agent       agent.Agent       // 바인딩된 Agent
    sharedRef   *agent.SharedRef  // 참조 카운팅
    config      BridgeConfig      // Bridge 설정
    transformer BridgeTransformer // 메시지 변환기
    correlation *CorrelationTracker // Request-Reply 전용
    stats       BridgeStats       // 처리 통계
    recvCh      chan message.Message // 수신 버퍼 채널
    cancelFn    context.CancelFunc   // 수신 루프 취소
    mu          sync.RWMutex         // Agent 바인딩 보호
    flowID      string               // 소속 Flow ID (SharedRef 관리용)
}

func NewBridgeNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *BridgeNode) Init(ctx context.Context) error
func (n *BridgeNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *BridgeNode) Shutdown(ctx context.Context) error
func (n *BridgeNode) Configure(config map[string]any) error
func (n *BridgeNode) Info() BridgeInfo
func (n *BridgeNode) Stats() BridgeStats

// ── BridgeConfig ──

type BridgeConfig struct {
    AgentRef             flow.AgentRef
    Direction            flow.BridgeDirection
    Transform            TransformConfig
    RequestTimeout       time.Duration
    ReconnectInterval    time.Duration
    MaxReconnectAttempts int
    BufferSize           int
    ResponseTarget       string
}

type TransformConfig struct {
    Mode      string // "auto", "lua"
    LuaScript string // Lua 스크립트 파일 경로
}

func (c BridgeConfig) Validate() error

// ── BridgeTransformer ──

type BridgeTransformer interface {
    AgentToFlow(data []byte) (message.Message, error)
    FlowToAgent(msg message.Message) ([]byte, error)
}

type DefaultTransformer struct{}

func NewDefaultTransformer() *DefaultTransformer
func (t *DefaultTransformer) AgentToFlow(data []byte) (message.Message, error)
func (t *DefaultTransformer) FlowToAgent(msg message.Message) ([]byte, error)

type LuaTransformer struct {
    // vm은 internal/script 패키지 타입 참조
    agentToFlowScript string
    flowToAgentScript string
}

func NewLuaTransformer(config TransformConfig) (*LuaTransformer, error)
func (t *LuaTransformer) AgentToFlow(data []byte) (message.Message, error)
func (t *LuaTransformer) FlowToAgent(msg message.Message) ([]byte, error)

// ── CorrelationTracker ──

type correlationEntry struct {
    responseCh chan message.Message
    createdAt  time.Time
    timeout    time.Duration
}

type CorrelationTracker struct {
    entries    sync.Map      // correlationID -> *correlationEntry
    timeout    time.Duration // 전역 타임아웃
    cancelFn   context.CancelFunc // 정리 goroutine 취소
    timeoutCnt atomic.Int64  // 타임아웃 카운터
}

func NewCorrelationTracker(timeout time.Duration) *CorrelationTracker
func (t *CorrelationTracker) Track(correlationID string, responseCh chan message.Message)
func (t *CorrelationTracker) Resolve(correlationID string, response message.Message) bool
func (t *CorrelationTracker) Timeout(d time.Duration)
func (t *CorrelationTracker) Cleanup()
func (t *CorrelationTracker) PendingCount() int
func (t *CorrelationTracker) Close()
func (t *CorrelationTracker) StartCleanupLoop(ctx context.Context)

// ── BridgeInfo & BridgeStats ──

type BridgeInfo struct {
    AgentID    string
    AgentName  string
    Direction  flow.BridgeDirection
    Connected  bool
    Stats      BridgeStats
}

type BridgeStats struct {
    MessagesRelayed     int64
    MessagesFromAgent   int64
    MessagesToAgent     int64
    TransformErrors     int64
    CorrelationTimeouts int64
    PendingCorrelations int
    AvgRelayLatency     time.Duration
    LastActivityAt      time.Time
}

// ── Errors ──

var (
    ErrAgentNotFound        = errors.New("bridge: agent not found")
    ErrAgentNotRunning      = errors.New("bridge: agent not running")
    ErrAgentDisconnected    = errors.New("bridge: agent disconnected")
    ErrRequestTimeout       = errors.New("bridge: request-reply timeout")
    ErrCorrelationNotFound  = errors.New("bridge: correlation ID not found")
    ErrTransformFailed      = errors.New("bridge: message transform failed")
    ErrInvalidDirection     = errors.New("bridge: invalid bridge direction")
    ErrMaxReconnectExceeded = errors.New("bridge: max reconnect attempts exceeded")
    ErrBufferFull           = errors.New("bridge: receive buffer full")
)
```

### 4.3 BridgeNode 통신 모드 상세 다이어그램

```
BridgeIn (Agent -> Flow):
  Agent ──[Process() []byte]──> BridgeNode ──[AgentToFlow()]──> Message ──[출력포트]──> Flow
                                    │
                           [수신 루프 goroutine]
                           [recvCh 버퍼 관리]

BridgeOut (Flow -> Agent):
  Flow ──[입력포트]──> Message ──> BridgeNode ──[FlowToAgent()]──> []byte ──[Agent.Write]──> Agent

BridgeInOut (Agent <-> Flow):
  Agent ──[수신 루프]──> BridgeNode ──[AgentToFlow()]──> Flow (출력 포트)
  Flow ──[입력 포트]──> BridgeNode ──[FlowToAgent()]──> Agent (송신)

BridgeRequestReply (Correlation ID 기반):
  Flow ──[요청]──> BridgeNode
                      │
                      ├── correlationID = uuid.New()
                      ├── msg.Metadata.Set("_correlationID", correlationID)
                      ├── tracker.Track(correlationID, responseCh)
                      ├── FlowToAgent(msg) -> Agent
                      │
                      ├── select {
                      │     case resp := <-responseCh:
                      │         return AgentToFlow(resp)
                      │     case <-time.After(timeout):
                      │         return ErrRequestTimeout
                      │     case <-ctx.Done():
                      │         return ctx.Err()
                      │   }
                      │
  Agent ──[응답]──> BridgeNode ──[tracker.Resolve(correlationID, data)]──> responseCh
```

### 4.4 Agent 바인딩 및 재바인딩 시퀀스

```
Init() 시퀀스:
  1. AgentRegistry.Get(agentRef.Name) -> agent
  2. agent.SharedRef.Acquire(flowID) -> refCount++
  3. agent.Health() == Running? -> 확인
  4. BridgeIn/InOut? -> go recvLoop(ctx)
  5. RequestReply? -> go tracker.StartCleanupLoop(ctx)
  6. state = StateRunning

재바인딩 시퀀스 (Agent Disconnected):
  1. 에러 포트로 ErrAgentDisconnected 전달
  2. for attempt := 0; attempt < MaxReconnectAttempts; attempt++ {
         time.Sleep(ReconnectInterval)
         agent = AgentRegistry.Get(agentRef.Name)
         if agent != nil && agent.Health() == Running {
             agent.SharedRef.Acquire(flowID)
             reconnect 성공 -> break
         }
     }
  3. 모든 시도 실패 시 ErrMaxReconnectExceeded 에러 포트 전달

Shutdown() 시퀀스:
  1. cancelFn() -> 수신 루프 / 정리 루프 중지
  2. tracker.Close() -> 모든 대기 Correlation 타임아웃 처리
  3. agent.SharedRef.Release(flowID) -> refCount--
  4. agent = nil
  5. state = StateStopped
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-BRIDGE-001-01-01 ~ 01-05 | BridgeNode Core | bridge.go | P0 |
| REQ-BRIDGE-001-02-01 ~ 02-07 | Agent Binding | bridge.go | P0 |
| REQ-BRIDGE-001-03-01 ~ 03-06 | Communication Modes | bridge.go | P0 |
| REQ-BRIDGE-001-04-01 ~ 04-05 | Message Transformation | bridge_transform.go | P1 |
| REQ-BRIDGE-001-05-01 ~ 05-07 | Request-Reply Correlation | bridge_correlation.go | P1 |
| REQ-BRIDGE-001-06-01 ~ 06-04 | Bridge Configuration | bridge_config.go | P1 |
| REQ-BRIDGE-001-07-01 ~ 07-04 | Bridge Info & Stats | bridge_info.go | P1 |
| REQ-BRIDGE-001-08-01 ~ 08-02 | Error Types | bridge_errors.go | P0 |
