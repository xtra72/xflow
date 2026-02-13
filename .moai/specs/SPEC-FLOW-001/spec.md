---
id: SPEC-FLOW-001
version: "1.0.0"
status: draft
created: "2026-02-12"
updated: "2026-02-12"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-12 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-FLOW-001: Flow System - 인터페이스 기반 플로우 정의, 노드/와이어 구성, 상태 모델 및 경로 지정

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 엔진의 실행 단위인 Flow 시스템을 정의한다. Flow는 엔진에서 독립적으로 실행 관리되는 노드의 그룹으로, 노드 간 연결(Wire)과 노드-에이전트 간 연결(Bridge Node 참조)을 관리한다. 노드와 와이어는 해당 플로우 내에서만 유효하며, ID 또는 이름으로 구분된다. Flow는 다양한 상태(Stored, Loaded, Running, Paused, Stopped 등)를 가지며, CLI 등의 관리 도구에서 dot 표기법(`flowRef.nodeRef`)으로 특정 노드에 접근할 수 있다.

본 SPEC은 `pkg/flow/` 패키지의 공개 API 데이터 구조와 유틸리티를 다루며, 런타임 실행 로직(`internal/engine/`)은 별도 SPEC-ENGINE-001에서 다룬다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `pkg/flow/`
- **의존성**: 표준 라이브러리 + `gopkg.in/yaml.v3` (YAML 직렬화)
- **테스트 프레임워크**: Go 표준 `testing` 패키지
- **관련 패키지**: `pkg/message/` (SPEC-MSG-001, Message 인터페이스 참조)
- **의존 대상**: Tier 1 - 데이터 기반 계층 (SPEC-MSG-001과 동일 계층, 상호 의존 없음)

### 1.3 설계 원칙

- **인터페이스 우선**: Flow의 공개 API는 인터페이스로 정의, 구현체는 unexported
- **캡슐화**: 구조체 필드 직접 접근 금지, 메서드를 통한 접근만 허용
- **스코프 격리**: 노드와 와이어는 소속 플로우 내에서만 유효
- **경로 지정**: dot 표기법으로 플로우 내 노드를 주소 지정
- **직렬화 호환**: JSON/YAML 양방향 직렬화로 설정 로딩 및 내보내기 지원
- **상태 정의 분리**: 상태 열거와 전이 규칙은 정의, 실행은 엔진에 위임

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Flow, NodeDef, Port, Wire 데이터 구조 정의
- FlowState 열거형 및 유효 상태 전이 맵
- Dot 표기법 경로 해석 (PathResolver)
- JSON/YAML 직렬화/역직렬화
- Flow 그래프 유효성 검증
- 팩토리 함수 및 Options Pattern

**OUT OF SCOPE (별도 SPEC)**:
- 런타임 플로우 실행 (SPEC-ENGINE-001: `internal/engine/`)
- 실제 상태 전이 실행 (SPEC-ENGINE-001: `internal/engine/state.go`)
- Wire 메시지 전달 (SPEC-ENGINE-001: `internal/engine/wire.go`)
- 노드 프로세스 실행 (별도 SPEC: `internal/node/`)
- 백프레셔 메커니즘 (SPEC-ENGINE-001: `internal/engine/backpressure.go`)
- 스케줄러 (SPEC-ENGINE-001: `internal/engine/scheduler.go`)
- Bridge Node 런타임 동작 (별도 SPEC: `internal/node/bridge.go`)

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: Flow ID는 UUID v4로 생성되며 시스템 전체에서 고유하다
- A2: Flow Name은 시스템 전체에서 고유하다 (외부 레지스트리에서 강제, `pkg/flow/`는 검증하지 않음)
- A3: NodeDef ID는 Flow 내에서 고유하다 (UUID v4)
- A4: NodeDef Name은 Flow 내에서 고유하다
- A5: Wire ID는 Flow 내에서 고유하다 (UUID v4)
- A6: `pkg/flow/` 패키지는 동시성 안전(thread-safety)을 보장하지 않는다 (단일 goroutine 가정)
- A7: YAML 직렬화를 위해 `gopkg.in/yaml.v3` 외부 패키지를 허용한다
- A8: JSON 직렬화는 표준 라이브러리 `encoding/json`만 사용한다

### 2.2 도메인 가정

- A9: 하나의 Flow는 0개 이상의 NodeDef와 0개 이상의 Wire를 포함한다
- A10: Wire는 반드시 같은 Flow 내 노드들 간의 연결이어야 한다 (크로스 플로우 불가)
- A11: 하나의 출력 포트에서 여러 입력 포트로의 연결(fan-out)이 가능하다
- A12: 하나의 입력 포트에 여러 출력 포트로부터의 연결(fan-in)이 가능하다
- A13: NodeDef는 노드의 "정의"이며, 런타임 인스턴스가 아니다 (타입, 설정, 포트 정보)
- A14: Bridge Node는 NodeDef의 한 종류이며, AgentRef 필드로 외부 Agent를 참조한다
- A15: Dot 표기법의 구분자는 `.`(마침표)이며, 이름에 마침표가 포함된 경우 대괄호 표기법(`[name.with.dots]`)으로 이스케이프한다
- A16: FlowState는 정의만 포함하며, 실제 상태 전이 실행은 `internal/engine/`에서 수행한다

---

## 3. Requirements (요구사항)

### Module 1: Core Definitions - 핵심 데이터 구조 정의

#### REQ-FLOW-001-01-01 (Ubiquitous) Flow 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Flow` 인터페이스를 제공해야 한다:

- `ID() string` - 플로우 고유 식별자 반환 (UUID v4)
- `Name() string` - 플로우 이름 반환
- `Description() string` - 플로우 설명 반환
- `State() FlowState` - 현재 상태 반환
- `Nodes() []NodeDef` - 포함된 노드 정의 목록 반환 (방어적 복사)
- `Node(idOrName string) (NodeDef, bool)` - ID 또는 이름으로 노드 조회
- `Wires() []Wire` - 포함된 와이어 목록 반환 (방어적 복사)
- `Wire(id string) (Wire, bool)` - ID로 와이어 조회
- `Config() FlowConfig` - 플로우 설정 반환
- `Metadata() map[string]string` - 사용자 정의 메타데이터 반환 (방어적 복사)
- `CreatedAt() time.Time` - 생성 시각 반환
- `UpdatedAt() time.Time` - 최종 수정 시각 반환

#### REQ-FLOW-001-01-02 (Ubiquitous) Flow 기본 구현체

시스템은 **항상** `defaultFlow`(unexported struct)를 `Flow` 인터페이스의 기본 구현체로 사용해야 한다.

#### REQ-FLOW-001-01-03 (Ubiquitous) Flow 생성자

시스템은 **항상** `NewFlow(name string, opts ...FlowOption) Flow` 팩토리 함수를 제공해야 한다.

- 반환 타입은 `Flow` 인터페이스이다
- ID는 UUID v4로 자동 생성한다
- Name은 필수 인자이다
- CreatedAt과 UpdatedAt은 `time.Now()`로 자동 설정한다
- State 초기값은 `FlowStored`이다
- 옵션이 없으면 빈 노드/와이어 목록과 기본 FlowConfig으로 생성한다

#### REQ-FLOW-001-01-04 (Ubiquitous) Flow 변경 메서드

시스템은 **항상** Flow에 대한 다음 변경 메서드를 제공해야 한다:

- `AddNode(node NodeDef) error` - 노드 추가 (ID/Name 중복 시 에러)
- `RemoveNode(idOrName string) error` - 노드 제거 (연결된 Wire도 함께 제거)
- `AddWire(wire Wire) error` - 와이어 추가 (참조 노드/포트 유효성 검증)
- `RemoveWire(id string) error` - 와이어 제거
- `SetDescription(desc string)` - 설명 변경
- `SetConfig(config FlowConfig)` - 설정 변경
- `SetMetadata(key, value string)` - 메타데이터 설정
- `RemoveMetadata(key string)` - 메타데이터 삭제
- `SetState(state FlowState) error` - 상태 변경 (유효 전이만 허용)

#### REQ-FLOW-001-01-05 (Ubiquitous) NodeDef 구조체 정의

시스템은 **항상** 다음 필드를 가진 `NodeDef` exported 구조체를 제공해야 한다:

- `ID string` - 노드 고유 식별자 (UUID v4)
- `Name string` - 노드 이름 (플로우 내 고유)
- `Type string` - 노드 타입 (예: "filter", "transform", "bridge", "script")
- `Config map[string]any` - 노드별 설정 (타입에 따라 상이)
- `Inputs []Port` - 입력 포트 목록
- `Outputs []Port` - 출력 포트 목록
- `ErrorPort *Port` - 에러 출력 포트 (nil이면 에러 포트 없음)
- `AgentRef *AgentRef` - Bridge Node인 경우 Agent 참조 (nil이면 일반 노드)
- `Metadata map[string]string` - 노드 메타데이터 (UI 위치 등)

#### REQ-FLOW-001-01-06 (Ubiquitous) NodeDef 생성자

시스템은 **항상** `NewNodeDef(name, nodeType string, opts ...NodeOption) NodeDef` 생성자를 제공해야 한다.

- ID는 UUID v4로 자동 생성한다
- Name과 Type은 필수 인자이다
- 기본적으로 하나의 입력 포트("in")와 하나의 출력 포트("out")를 자동 생성한다
- ErrorPort는 기본값 nil이다 (옵션으로 활성화)

#### REQ-FLOW-001-01-07 (Ubiquitous) Port 구조체 정의

시스템은 **항상** 다음 필드를 가진 `Port` exported 구조체를 제공해야 한다:

- `ID string` - 포트 고유 식별자
- `Name string` - 포트 이름 (노드 내 고유)
- `Direction PortDirection` - 포트 방향 (Input, Output, Error)

#### REQ-FLOW-001-01-08 (Ubiquitous) PortDirection 타입 정의

시스템은 **항상** `PortDirection` 타입과 다음 상수를 제공해야 한다:

- `PortInput` - 입력 포트
- `PortOutput` - 출력 포트
- `PortError` - 에러 출력 포트

#### REQ-FLOW-001-01-09 (Ubiquitous) Wire 구조체 정의

시스템은 **항상** 다음 필드를 가진 `Wire` exported 구조체를 제공해야 한다:

- `ID string` - 와이어 고유 식별자 (UUID v4)
- `SourceNodeID string` - 출발 노드 ID
- `SourcePort string` - 출발 포트 이름
- `TargetNodeID string` - 도착 노드 ID
- `TargetPort string` - 도착 포트 이름
- `Mode WireMode` - 전송 모드 (bypass/buffer)
- `BufferSize int` - 버퍼 크기 (buffer 모드 시, 기본값 0 = bypass)
- `TTL time.Duration` - 메시지 유효시간 (0이면 TTL 미적용)

#### REQ-FLOW-001-01-10 (Ubiquitous) WireMode 타입 정의

시스템은 **항상** `WireMode` 타입과 다음 상수를 제공해야 한다:

- `WireBypass` - 바이패스 모드 (기본값, Go unbuffered channel에 대응)
- `WireBuffer` - 버퍼 모드 (Go buffered channel에 대응)

#### REQ-FLOW-001-01-11 (Ubiquitous) Wire 생성자

시스템은 **항상** `NewWire(sourceNodeID, sourcePort, targetNodeID, targetPort string, opts ...WireOption) Wire` 생성자를 제공해야 한다.

- ID는 UUID v4로 자동 생성한다
- Mode 기본값은 `WireBypass`이다
- BufferSize 기본값은 0이다
- TTL 기본값은 0 (미적용)이다

#### REQ-FLOW-001-01-12 (Ubiquitous) AgentRef 구조체 정의

시스템은 **항상** 다음 필드를 가진 `AgentRef` exported 구조체를 제공해야 한다:

- `AgentID string` - 참조 Agent의 ID
- `AgentName string` - 참조 Agent의 이름 (ID가 없을 때 이름으로 검색)
- `Direction BridgeDirection` - 브릿지 방향 (In, Out, InOut, RequestReply)

#### REQ-FLOW-001-01-13 (Ubiquitous) BridgeDirection 타입 정의

시스템은 **항상** `BridgeDirection` 타입과 다음 상수를 제공해야 한다:

- `BridgeIn` - Agent -> Flow (수신 전용)
- `BridgeOut` - Flow -> Agent (송신 전용)
- `BridgeInOut` - 양방향
- `BridgeRequestReply` - 요청/응답 패턴

#### REQ-FLOW-001-01-14 (Ubiquitous) FlowConfig 구조체 정의

시스템은 **항상** 다음 필드를 가진 `FlowConfig` exported 구조체를 제공해야 한다:

- `TrackHistory bool` - 메시지 변경 이력 추적 여부 (기본: false)
- `MaxHistorySize int` - 최대 변경 이력 수 (기본: 100)
- `ErrorHandling ErrorPolicy` - 에러 처리 정책

#### REQ-FLOW-001-01-15 (Ubiquitous) FlowOption, NodeOption, WireOption 함수 타입

시스템은 **항상** 다음 Option 타입들을 제공해야 한다:

- `type FlowOption func(*flowConfig)` - Flow 설정 함수
- `type NodeOption func(*NodeDef)` - NodeDef 설정 함수
- `type WireOption func(*Wire)` - Wire 설정 함수

주요 FlowOption:
- `WithDescription(desc string) FlowOption`
- `WithFlowConfig(config FlowConfig) FlowOption`
- `WithFlowMetadata(key, value string) FlowOption`
- `WithNodes(nodes ...NodeDef) FlowOption`
- `WithWires(wires ...Wire) FlowOption`

주요 NodeOption:
- `WithInputPorts(ports ...Port) NodeOption` - 커스텀 입력 포트 설정
- `WithOutputPorts(ports ...Port) NodeOption` - 커스텀 출력 포트 설정
- `WithErrorPort() NodeOption` - 에러 포트 활성화
- `WithNodeConfig(key string, value any) NodeOption` - 노드 설정 추가
- `WithAgentRef(ref AgentRef) NodeOption` - Agent 참조 설정 (Bridge Node)
- `WithNodeMetadata(key, value string) NodeOption` - 노드 메타데이터 설정

주요 WireOption:
- `WithWireMode(mode WireMode) WireOption` - 전송 모드 설정
- `WithBufferSize(size int) WireOption` - 버퍼 크기 설정
- `WithTTL(ttl time.Duration) WireOption` - 메시지 TTL 설정

---

### Module 2: Identification & Addressing - 식별 및 주소 지정

#### REQ-FLOW-001-02-01 (Ubiquitous) Flow 식별

시스템은 **항상** Flow를 ID(UUID) 또는 Name(문자열)으로 식별할 수 있어야 한다.

#### REQ-FLOW-001-02-02 (Ubiquitous) NodeDef 식별

시스템은 **항상** NodeDef를 소속 Flow 내에서 ID(UUID) 또는 Name(문자열)으로 식별할 수 있어야 한다.

#### REQ-FLOW-001-02-03 (Ubiquitous) NodePath 구조체 정의

시스템은 **항상** 다음 필드를 가진 `NodePath` exported 구조체를 제공해야 한다:

- `FlowRef string` - 플로우 식별자 (ID 또는 Name)
- `NodeRef string` - 노드 식별자 (ID 또는 Name)

#### REQ-FLOW-001-02-04 (Event-Driven) Dot 표기법 파싱

**WHEN** `ParseNodePath(path string)` 함수가 `"flowRef.nodeRef"` 형식의 문자열을 받으면, **THEN** `NodePath{FlowRef: "flowRef", NodeRef: "nodeRef"}`를 반환해야 한다.

#### REQ-FLOW-001-02-05 (Event-Driven) Dot 표기법 대괄호 이스케이프

**WHEN** `ParseNodePath(path string)` 함수가 이름에 마침표를 포함하는 경우 대괄호 표기법을 받으면, **THEN** 대괄호 내부를 하나의 식별자로 파싱해야 한다.

예시:
- `"[flow.name].nodeName"` -> `NodePath{FlowRef: "flow.name", NodeRef: "nodeName"}`
- `"flowName.[node.name]"` -> `NodePath{FlowRef: "flowName", NodeRef: "node.name"}`
- `"[flow.a].[node.b]"` -> `NodePath{FlowRef: "flow.a", NodeRef: "node.b"}`

#### REQ-FLOW-001-02-06 (Event-Driven) Dot 표기법 파싱 에러

**WHEN** `ParseNodePath(path string)` 함수가 유효하지 않은 형식을 받으면, **THEN** `ErrInvalidPath` 에러를 반환해야 한다.

유효하지 않은 형식:
- 빈 문자열
- 구분자(`.`) 없는 단일 문자열
- FlowRef 또는 NodeRef가 빈 경우
- 닫히지 않은 대괄호

#### REQ-FLOW-001-02-07 (Ubiquitous) NodePath 문자열 변환

시스템은 **항상** `NodePath.String() string` 메서드를 제공하여, NodePath를 dot 표기법 문자열로 변환해야 한다. 이름에 마침표가 포함되면 자동으로 대괄호 표기법을 사용한다.

#### REQ-FLOW-001-02-08 (Ubiquitous) Flow 내 노드 조회

시스템은 **항상** `Flow.Node(idOrName string)` 메서드를 통해 ID 우선, Name 차선 순서로 노드를 조회해야 한다.

#### REQ-FLOW-001-02-09 (Unwanted) 크로스 플로우 노드 참조 금지

시스템은 Wire의 SourceNodeID와 TargetNodeID가 같은 Flow에 소속되지 않은 노드를 참조하는 것을 **허용하지 않아야 한다**.

---

### Module 3: State Model - 상태 모델

#### REQ-FLOW-001-03-01 (Ubiquitous) FlowState 타입 정의

시스템은 **항상** `FlowState` 타입(string 기반)과 다음 상수를 제공해야 한다:

- `FlowStored` - 저장됨 (영속화 상태, 메모리에 미로드)
- `FlowLoaded` - 로드됨 (메모리에 로드, 미초기화)
- `FlowInitializing` - 초기화 중 (리소스 할당 중)
- `FlowRunning` - 실행 중 (데이터 처리 가능)
- `FlowPaused` - 일시정지 (데이터 처리 중단, 채널/상태 유지)
- `FlowStopping` - 중지 중 (정상 종료 진행)
- `FlowStopped` - 중지됨 (리소스 해제 완료)
- `FlowError` - 에러 (오류 발생, 복구 대기)

#### REQ-FLOW-001-03-02 (Ubiquitous) FlowState String 메서드

시스템은 **항상** `FlowState.String() string` 메서드를 제공하여 사람이 읽을 수 있는 상태명을 반환해야 한다.

#### REQ-FLOW-001-03-03 (Ubiquitous) 유효 상태 전이 맵

시스템은 **항상** `ValidTransitions` 맵을 제공하여, 각 FlowState에서 전이 가능한 상태 목록을 정의해야 한다:

| 현재 상태 | 전이 가능 상태 |
|-----------|--------------|
| Stored | Loaded |
| Loaded | Initializing, Stored |
| Initializing | Running, Error, Stopped |
| Running | Paused, Stopping, Error |
| Paused | Running, Stopping, Error |
| Stopping | Stopped, Error |
| Stopped | Loaded, Stored |
| Error | Stopped, Initializing |

#### REQ-FLOW-001-03-04 (Event-Driven) 상태 전이 검증

**WHEN** `Flow.SetState(newState)` 호출 시 현재 상태에서 `newState`로의 전이가 유효하지 않으면, **THEN** `ErrInvalidStateTransition` 에러를 반환해야 한다.

#### REQ-FLOW-001-03-05 (Event-Driven) 유효 상태 전이 성공

**WHEN** `Flow.SetState(newState)` 호출 시 전이가 유효하면, **THEN** 상태를 변경하고 `UpdatedAt`을 갱신해야 한다.

#### REQ-FLOW-001-03-06 (Ubiquitous) FlowState 유효성 검증 함수

시스템은 **항상** `IsValidTransition(from, to FlowState) bool` 함수를 제공하여 상태 전이 가능 여부를 확인할 수 있어야 한다.

#### REQ-FLOW-001-03-07 (Ubiquitous) FlowState 파싱 함수

시스템은 **항상** `ParseFlowState(s string) (FlowState, error)` 함수를 제공하여 문자열에서 FlowState를 파싱할 수 있어야 한다. 유효하지 않은 문자열은 에러를 반환한다.

---

### Module 4: Serialization & Loading - 직렬화 및 로딩

#### REQ-FLOW-001-04-01 (Ubiquitous) JSON 직렬화

시스템은 **항상** Flow를 JSON으로 직렬화할 수 있어야 한다. `MarshalJSON` 커스텀 메서드를 제공한다.

직렬화 구조:
```json
{
  "id": "uuid-string",
  "name": "flow-name",
  "description": "flow description",
  "state": "stored",
  "config": {
    "track_history": false,
    "max_history_size": 100,
    "error_handling": "propagate"
  },
  "nodes": [...],
  "wires": [...],
  "metadata": {...},
  "created_at": "RFC3339",
  "updated_at": "RFC3339"
}
```

#### REQ-FLOW-001-04-02 (Ubiquitous) JSON 역직렬화

시스템은 **항상** `FlowFromJSON(data []byte) (Flow, error)` 함수를 제공하여 JSON 데이터로부터 Flow를 복원해야 한다.

- 유효하지 않은 JSON은 에러를 반환한다
- 누락된 필수 필드(name)는 에러를 반환한다
- ID가 누락되면 새 UUID를 자동 생성한다

#### REQ-FLOW-001-04-03 (Ubiquitous) YAML 직렬화

시스템은 **항상** `FlowToYAML(f Flow) ([]byte, error)` 함수를 제공하여 Flow를 YAML로 직렬화해야 한다.

#### REQ-FLOW-001-04-04 (Ubiquitous) YAML 역직렬화

시스템은 **항상** `FlowFromYAML(data []byte) (Flow, error)` 함수를 제공하여 YAML 데이터로부터 Flow를 복원해야 한다.

#### REQ-FLOW-001-04-05 (Event-Driven) 파일 로딩

**WHEN** `LoadFlowFromFile(path string)` 함수가 파일 경로를 받으면, **THEN** 확장자에 따라 JSON(.json) 또는 YAML(.yaml, .yml)로 파싱하여 Flow를 반환해야 한다.

#### REQ-FLOW-001-04-06 (Event-Driven) 파일 로딩 에러

**WHEN** `LoadFlowFromFile(path string)` 함수가 존재하지 않는 파일 또는 지원하지 않는 확장자를 받으면, **THEN** 적절한 에러를 반환해야 한다.

#### REQ-FLOW-001-04-07 (Ubiquitous) 파일 저장

시스템은 **항상** `SaveFlowToFile(f Flow, path string) error` 함수를 제공하여 Flow를 확장자에 따라 JSON 또는 YAML로 저장해야 한다.

#### REQ-FLOW-001-04-08 (Ubiquitous) NodeDef JSON 직렬화

시스템은 **항상** `NodeDef`가 `encoding/json` 표준 직렬화/역직렬화를 지원하도록 JSON 태그를 설정해야 한다.

#### REQ-FLOW-001-04-09 (Ubiquitous) Wire JSON 직렬화

시스템은 **항상** `Wire`가 `encoding/json` 표준 직렬화/역직렬화를 지원하도록 JSON 태그를 설정해야 한다.

---

### Module 5: Validation - 유효성 검증

#### REQ-FLOW-001-05-01 (Ubiquitous) 통합 검증 함수

시스템은 **항상** `Validate(f Flow) []ValidationError` 함수를 제공하여 Flow의 모든 유효성 검사를 수행하고, 발견된 모든 문제를 목록으로 반환해야 한다.

#### REQ-FLOW-001-05-02 (Ubiquitous) ValidationError 구조체

시스템은 **항상** 다음 필드를 가진 `ValidationError` exported 구조체를 제공해야 한다:

- `Code string` - 에러 코드 (예: "WIRE_ORPHAN_SOURCE")
- `Severity ValidationSeverity` - 심각도 (Error, Warning)
- `Message string` - 사람이 읽을 수 있는 에러 메시지
- `Path string` - 에러 발생 위치 (예: "wires[0].source_node_id")

#### REQ-FLOW-001-05-03 (Event-Driven) Wire 소스 노드 존재 검증

**WHEN** Validate 실행 시 Wire의 SourceNodeID가 Flow 내 노드에 존재하지 않으면, **THEN** `WIRE_ORPHAN_SOURCE` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-04 (Event-Driven) Wire 타겟 노드 존재 검증

**WHEN** Validate 실행 시 Wire의 TargetNodeID가 Flow 내 노드에 존재하지 않으면, **THEN** `WIRE_ORPHAN_TARGET` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-05 (Event-Driven) Wire 소스 포트 존재 검증

**WHEN** Validate 실행 시 Wire의 SourcePort가 소스 노드의 출력 포트 또는 에러 포트에 존재하지 않으면, **THEN** `WIRE_INVALID_SOURCE_PORT` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-06 (Event-Driven) Wire 타겟 포트 존재 검증

**WHEN** Validate 실행 시 Wire의 TargetPort가 타겟 노드의 입력 포트에 존재하지 않으면, **THEN** `WIRE_INVALID_TARGET_PORT` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-07 (Event-Driven) 중복 Wire 검증

**WHEN** Validate 실행 시 동일한 SourceNodeID+SourcePort+TargetNodeID+TargetPort 조합의 Wire가 중복되면, **THEN** `WIRE_DUPLICATE` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-08 (Event-Driven) 자기 참조 Wire 검증

**WHEN** Validate 실행 시 Wire의 SourceNodeID와 TargetNodeID가 동일하면, **THEN** `WIRE_SELF_REFERENCE` Warning을 생성해야 한다 (허용하되 경고).

#### REQ-FLOW-001-05-09 (Event-Driven) 노드 ID 중복 검증

**WHEN** Validate 실행 시 동일 Flow 내 두 개 이상의 NodeDef가 같은 ID를 가지면, **THEN** `NODE_DUPLICATE_ID` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-10 (Event-Driven) 노드 이름 중복 검증

**WHEN** Validate 실행 시 동일 Flow 내 두 개 이상의 NodeDef가 같은 Name을 가지면, **THEN** `NODE_DUPLICATE_NAME` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-11 (Event-Driven) Bridge Node AgentRef 검증

**WHEN** Validate 실행 시 NodeDef의 Type이 "bridge"이고 AgentRef가 nil이면, **THEN** `NODE_BRIDGE_NO_AGENT` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-12 (Event-Driven) Buffer 모드 버퍼 크기 검증

**WHEN** Validate 실행 시 Wire의 Mode가 `WireBuffer`이고 BufferSize가 0 이하이면, **THEN** `WIRE_BUFFER_INVALID_SIZE` ValidationError를 생성해야 한다.

#### REQ-FLOW-001-05-13 (Event-Driven) 연결되지 않은 노드 경고

**WHEN** Validate 실행 시 NodeDef에 연결된 Wire가 하나도 없으면, **THEN** `NODE_DISCONNECTED` Warning을 생성해야 한다.

#### REQ-FLOW-001-05-14 (Ubiquitous) ValidationSeverity 타입

시스템은 **항상** `ValidationSeverity` 타입과 다음 상수를 제공해야 한다:

- `SeverityError` - 에러 (Flow 실행 불가)
- `SeverityWarning` - 경고 (Flow 실행 가능하나 잠재적 문제)

---

## 4. Specifications (사양)

### 4.1 패키지 구조

```
pkg/flow/
  flow.go              # Flow 인터페이스, NewFlow(), FlowOption, FlowConfig
  node.go              # NodeDef, Port, PortDirection, AgentRef, BridgeDirection, NewNodeDef()
  connection.go        # Wire, WireMode, NewWire(), WireOption
  state.go             # FlowState 상수, ValidTransitions, IsValidTransition(), ParseFlowState()
  path.go              # NodePath, ParseNodePath(), String()
  serialize.go         # MarshalJSON, FlowFromJSON, FlowToYAML, FlowFromYAML, LoadFlowFromFile, SaveFlowToFile
  validate.go          # Validate(), ValidationError, ValidationSeverity
  errors.go            # 패키지 에러 정의 (ErrInvalidPath, ErrInvalidStateTransition 등)
  flow_test.go         # Flow 인터페이스 및 생성자 테스트
  node_test.go         # NodeDef, Port 테스트
  connection_test.go   # Wire 테스트
  state_test.go        # FlowState, 상태 전이 테스트
  path_test.go         # NodePath, dot 표기법 파싱 테스트
  serialize_test.go    # JSON/YAML 직렬화 테스트
  validate_test.go     # Flow 유효성 검증 테스트
```

### 4.2 에러 정의

| 에러 변수 | 설명 |
|-----------|------|
| `ErrInvalidPath` | dot 표기법 경로가 유효하지 않을 때 |
| `ErrInvalidStateTransition` | 유효하지 않은 상태 전이 시도 시 |
| `ErrInvalidFlowState` | 알 수 없는 FlowState 문자열 파싱 시 |
| `ErrDuplicateNodeID` | 동일 ID의 노드가 이미 존재할 때 |
| `ErrDuplicateNodeName` | 동일 이름의 노드가 이미 존재할 때 |
| `ErrNodeNotFound` | 지정된 ID/이름의 노드를 찾을 수 없을 때 |
| `ErrWireNotFound` | 지정된 ID의 와이어를 찾을 수 없을 때 |
| `ErrDuplicateWireID` | 동일 ID의 와이어가 이미 존재할 때 |
| `ErrInvalidWireSource` | 와이어 소스 노드/포트가 유효하지 않을 때 |
| `ErrInvalidWireTarget` | 와이어 타겟 노드/포트가 유효하지 않을 때 |
| `ErrUnsupportedFormat` | 지원하지 않는 파일 확장자일 때 |
| `ErrFlowNameRequired` | Flow 이름이 비어있을 때 |

### 4.3 상태 전이 다이어그램

```
Stored --> Loaded --> Initializing --> Running <--> Paused
  ^          |             |              |           |
  |          v             v              v           v
  |       Stored         Error         Stopping    Stopping
  |                        |              |           |
  |                        v              v           v
  +------ Stopped <----- Stopped <----- Stopped <-- Stopped
              |
              v
           Loaded (재시작)
```

### 4.4 Dot 표기법 구문

| 구문 | 예시 | 설명 |
|------|------|------|
| 기본 | `myflow.mynode` | 이름으로 접근 |
| UUID | `550e8400...4400.a1b2c3d4...` | ID로 접근 |
| 대괄호 이스케이프 | `[flow.name].nodeName` | 마침표 포함 이름 |
| 양쪽 이스케이프 | `[flow.a].[node.b]` | 양쪽 모두 마침표 포함 |

### 4.5 JSON 직렬화 스키마 (전체)

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "name": "sensor-pipeline",
  "description": "Temperature sensor data processing",
  "state": "stored",
  "config": {
    "track_history": true,
    "max_history_size": 100,
    "error_handling": "propagate"
  },
  "nodes": [
    {
      "id": "node-001",
      "name": "mqtt-bridge",
      "type": "bridge",
      "config": { "topic": "sensors/+/temperature" },
      "inputs": [{ "id": "p1", "name": "in", "direction": "input" }],
      "outputs": [{ "id": "p2", "name": "out", "direction": "output" }],
      "error_port": { "id": "p3", "name": "error", "direction": "error" },
      "agent_ref": {
        "agent_id": "agent-mqtt-001",
        "agent_name": "mqtt-broker",
        "direction": "in"
      },
      "metadata": { "x": "100", "y": "200" }
    },
    {
      "id": "node-002",
      "name": "temp-filter",
      "type": "filter",
      "config": { "condition": "$.temperature > 30" },
      "inputs": [{ "id": "p4", "name": "in", "direction": "input" }],
      "outputs": [{ "id": "p5", "name": "out", "direction": "output" }],
      "error_port": null,
      "agent_ref": null,
      "metadata": { "x": "300", "y": "200" }
    }
  ],
  "wires": [
    {
      "id": "wire-001",
      "source_node_id": "node-001",
      "source_port": "out",
      "target_node_id": "node-002",
      "target_port": "in",
      "mode": "bypass",
      "buffer_size": 0,
      "ttl": 0
    }
  ],
  "metadata": {
    "author": "xtra",
    "version": "1.0"
  },
  "created_at": "2026-02-12T10:00:00Z",
  "updated_at": "2026-02-12T10:30:00Z"
}
```

### 4.6 Validation 코드 체계

| 코드 | 심각도 | 설명 |
|------|--------|------|
| `WIRE_ORPHAN_SOURCE` | Error | 소스 노드가 존재하지 않음 |
| `WIRE_ORPHAN_TARGET` | Error | 타겟 노드가 존재하지 않음 |
| `WIRE_INVALID_SOURCE_PORT` | Error | 소스 포트가 유효하지 않음 |
| `WIRE_INVALID_TARGET_PORT` | Error | 타겟 포트가 유효하지 않음 |
| `WIRE_DUPLICATE` | Error | 중복 와이어 |
| `WIRE_SELF_REFERENCE` | Warning | 자기 참조 와이어 |
| `WIRE_BUFFER_INVALID_SIZE` | Error | 버퍼 모드에서 크기가 0 이하 |
| `NODE_DUPLICATE_ID` | Error | 노드 ID 중복 |
| `NODE_DUPLICATE_NAME` | Error | 노드 이름 중복 |
| `NODE_BRIDGE_NO_AGENT` | Error | Bridge 노드에 AgentRef 없음 |
| `NODE_DISCONNECTED` | Warning | 연결되지 않은 노드 |

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 카테고리 | 검증 방법 |
|-------------|------|----------|-----------|
| REQ-FLOW-001-01-01 ~ 01-04 | Core | Ubiquitous | 단위 테스트 (Flow 인터페이스 및 생성/변경) |
| REQ-FLOW-001-01-05 ~ 01-06 | Core | Ubiquitous | 단위 테스트 (NodeDef 생성) |
| REQ-FLOW-001-01-07 ~ 01-08 | Core | Ubiquitous | 컴파일 타임 검증 (Port, PortDirection) |
| REQ-FLOW-001-01-09 ~ 01-11 | Core | Ubiquitous | 단위 테스트 (Wire 생성) |
| REQ-FLOW-001-01-12 ~ 01-13 | Core | Ubiquitous | 컴파일 타임 검증 (AgentRef, BridgeDirection) |
| REQ-FLOW-001-01-14 ~ 01-15 | Core | Ubiquitous | 단위 테스트 (FlowConfig, Options) |
| REQ-FLOW-001-02-01 ~ 02-02 | Identification | Ubiquitous | 단위 테스트 (ID/Name 조회) |
| REQ-FLOW-001-02-03 ~ 02-05 | Identification | Event-Driven | 단위 테스트 (dot 표기법 파싱) |
| REQ-FLOW-001-02-06 | Identification | Event-Driven | 단위 테스트 (파싱 에러) |
| REQ-FLOW-001-02-07 ~ 02-08 | Identification | Ubiquitous | 단위 테스트 (NodePath 변환, 조회) |
| REQ-FLOW-001-02-09 | Identification | Unwanted | 단위 테스트 (크로스 플로우 금지) |
| REQ-FLOW-001-03-01 ~ 03-03 | State | Ubiquitous | 단위 테스트 (상태 정의, 전이 맵) |
| REQ-FLOW-001-03-04 ~ 03-05 | State | Event-Driven | 단위 테스트 (상태 전이 성공/실패) |
| REQ-FLOW-001-03-06 ~ 03-07 | State | Ubiquitous | 단위 테스트 (유효성, 파싱) |
| REQ-FLOW-001-04-01 ~ 04-02 | Serialization | Ubiquitous | 단위 테스트 (JSON 직렬화/역직렬화) |
| REQ-FLOW-001-04-03 ~ 04-04 | Serialization | Ubiquitous | 단위 테스트 (YAML 직렬화/역직렬화) |
| REQ-FLOW-001-04-05 ~ 04-06 | Serialization | Event-Driven | 단위 테스트 (파일 로딩 성공/에러) |
| REQ-FLOW-001-04-07 | Serialization | Ubiquitous | 단위 테스트 (파일 저장) |
| REQ-FLOW-001-04-08 ~ 04-09 | Serialization | Ubiquitous | 컴파일 타임 검증 (JSON 태그) |
| REQ-FLOW-001-05-01 ~ 05-02 | Validation | Ubiquitous | 단위 테스트 (검증 함수, 에러 구조체) |
| REQ-FLOW-001-05-03 ~ 05-08 | Validation | Event-Driven | 단위 테스트 (Wire 검증) |
| REQ-FLOW-001-05-09 ~ 05-10 | Validation | Event-Driven | 단위 테스트 (노드 중복 검증) |
| REQ-FLOW-001-05-11 | Validation | Event-Driven | 단위 테스트 (Bridge 검증) |
| REQ-FLOW-001-05-12 | Validation | Event-Driven | 단위 테스트 (버퍼 크기 검증) |
| REQ-FLOW-001-05-13 | Validation | Event-Driven | 단위 테스트 (연결 안 된 노드 경고) |
| REQ-FLOW-001-05-14 | Validation | Ubiquitous | 컴파일 타임 검증 (ValidationSeverity) |
