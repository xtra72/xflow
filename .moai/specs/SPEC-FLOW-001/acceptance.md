---
id: SPEC-FLOW-001
type: acceptance
version: "1.0.0"
created: "2026-02-12"
updated: "2026-02-12"
---

# SPEC-FLOW-001 인수 기준

## Module 1: Core Definitions - 핵심 데이터 구조

### AC-FLOW-001-01: Flow 생성 및 기본값

```gherkin
Scenario: Flow 기본 생성
  Given 플로우 이름 "sensor-pipeline"이 주어졌을 때
  When NewFlow("sensor-pipeline")를 호출하면
  Then ID는 유효한 UUID v4 형식이어야 한다
  And Name은 "sensor-pipeline"이어야 한다
  And State는 FlowStored여야 한다
  And Nodes()는 빈 슬라이스여야 한다
  And Wires()는 빈 슬라이스여야 한다
  And CreatedAt은 현재 시각 부근이어야 한다
  And UpdatedAt은 CreatedAt과 동일해야 한다
```

### AC-FLOW-001-02: Flow 옵션 적용

```gherkin
Scenario: Flow 옵션으로 생성
  Given 플로우 이름 "my-flow"와 WithDescription("test"), WithFlowMetadata("key", "val") 옵션이 주어졌을 때
  When NewFlow("my-flow", WithDescription("test"), WithFlowMetadata("key", "val"))를 호출하면
  Then Description()은 "test"여야 한다
  And Metadata()["key"]는 "val"이어야 한다
```

### AC-FLOW-001-03: 노드 추가 성공

```gherkin
Scenario: Flow에 노드 추가
  Given 빈 Flow와 NodeDef(Name: "filter-1", Type: "filter")가 주어졌을 때
  When AddNode(node)를 호출하면
  Then 에러가 nil이어야 한다
  And Nodes()의 길이가 1이어야 한다
  And Node("filter-1")이 (node, true)를 반환해야 한다
```

### AC-FLOW-001-04: 중복 노드 ID 추가 에러

```gherkin
Scenario: 중복 ID 노드 추가 시 에러
  Given Flow에 ID "abc"인 노드가 이미 존재할 때
  When 같은 ID "abc"인 새 노드를 AddNode()로 추가하면
  Then ErrDuplicateNodeID 에러를 반환해야 한다
  And Nodes()의 길이는 변하지 않아야 한다
```

### AC-FLOW-001-05: 중복 노드 이름 추가 에러

```gherkin
Scenario: 중복 Name 노드 추가 시 에러
  Given Flow에 Name "filter-1"인 노드가 이미 존재할 때
  When 같은 Name "filter-1"인 새 노드를 AddNode()로 추가하면
  Then ErrDuplicateNodeName 에러를 반환해야 한다
```

### AC-FLOW-001-06: 노드 제거 시 연결된 Wire도 제거

```gherkin
Scenario: 노드 제거 시 관련 Wire 자동 삭제
  Given Flow에 nodeA, nodeB가 있고, nodeA->nodeB Wire가 연결되어 있을 때
  When RemoveNode("nodeA")를 호출하면
  Then Nodes()에 nodeA가 없어야 한다
  And nodeA를 소스 또는 타겟으로 하는 Wire가 모두 제거되어야 한다
```

### AC-FLOW-001-07: Wire 추가 시 노드/포트 검증

```gherkin
Scenario: Wire 추가 시 소스 노드가 존재하지 않으면 에러
  Given Flow에 nodeB만 존재할 때
  When NewWire("nonexistent", "out", nodeB.ID, "in")를 AddWire()로 추가하면
  Then ErrInvalidWireSource 에러를 반환해야 한다
```

### AC-FLOW-001-08: NodeDef 기본 생성

```gherkin
Scenario: NodeDef 기본 생성
  Given 노드 이름 "my-node"과 타입 "filter"가 주어졌을 때
  When NewNodeDef("my-node", "filter")를 호출하면
  Then ID는 유효한 UUID v4 형식이어야 한다
  And Name은 "my-node"이어야 한다
  And Type은 "filter"이어야 한다
  And Inputs 길이는 1이어야 한다 (기본 "in" 포트)
  And Outputs 길이는 1이어야 한다 (기본 "out" 포트)
  And ErrorPort는 nil이어야 한다
  And AgentRef는 nil이어야 한다
```

### AC-FLOW-001-09: NodeDef 에러 포트 활성화

```gherkin
Scenario: NodeDef 에러 포트 옵션
  Given WithErrorPort() 옵션이 주어졌을 때
  When NewNodeDef("my-node", "filter", WithErrorPort())를 호출하면
  Then ErrorPort가 nil이 아니어야 한다
  And ErrorPort.Direction이 PortError여야 한다
```

### AC-FLOW-001-10: NodeDef Bridge 노드 생성

```gherkin
Scenario: Bridge NodeDef 생성
  Given AgentRef{AgentName: "mqtt-broker", Direction: BridgeIn} 참조가 주어졌을 때
  When NewNodeDef("mqtt-bridge", "bridge", WithAgentRef(ref))를 호출하면
  Then AgentRef가 nil이 아니어야 한다
  And AgentRef.AgentName이 "mqtt-broker"여야 한다
  And AgentRef.Direction이 BridgeIn이어야 한다
```

### AC-FLOW-001-11: Wire 기본 생성

```gherkin
Scenario: Wire 기본 생성
  Given 소스("node-a", "out")와 타겟("node-b", "in")이 주어졌을 때
  When NewWire("node-a", "out", "node-b", "in")를 호출하면
  Then ID는 유효한 UUID v4 형식이어야 한다
  And Mode는 WireBypass여야 한다
  And BufferSize는 0이어야 한다
  And TTL은 0이어야 한다
```

### AC-FLOW-001-12: Wire 버퍼 모드 생성

```gherkin
Scenario: Wire 버퍼 모드 생성
  Given WithWireMode(WireBuffer), WithBufferSize(100), WithTTL(5*time.Second) 옵션이 주어졌을 때
  When NewWire("a", "out", "b", "in", WithWireMode(WireBuffer), WithBufferSize(100), WithTTL(5*time.Second))를 호출하면
  Then Mode는 WireBuffer여야 한다
  And BufferSize는 100이어야 한다
  And TTL은 5초여야 한다
```

### AC-FLOW-001-13: 방어적 복사 보장

```gherkin
Scenario: Nodes() 반환값 수정이 원본에 영향 없음
  Given Flow에 노드 1개가 존재할 때
  When nodes := flow.Nodes()를 호출하고, nodes에 새 요소를 append하면
  Then flow.Nodes()의 길이는 여전히 1이어야 한다
```

---

## Module 2: Identification & Addressing - 식별 및 주소 지정

### AC-FLOW-001-14: 기본 Dot 표기법 파싱

```gherkin
Scenario: 기본 dot 표기법 파싱
  Given 경로 문자열 "myflow.mynode"가 주어졌을 때
  When ParseNodePath("myflow.mynode")를 호출하면
  Then FlowRef는 "myflow"여야 한다
  And NodeRef는 "mynode"여야 한다
  And 에러는 nil이어야 한다
```

### AC-FLOW-001-15: 대괄호 이스케이프 파싱

```gherkin
Scenario: FlowRef에 마침표가 포함된 경우
  Given 경로 문자열 "[flow.name].nodeName"이 주어졌을 때
  When ParseNodePath("[flow.name].nodeName")를 호출하면
  Then FlowRef는 "flow.name"이어야 한다
  And NodeRef는 "nodeName"이어야 한다
```

### AC-FLOW-001-16: 양쪽 대괄호 이스케이프 파싱

```gherkin
Scenario: 양쪽 모두 마침표 포함
  Given 경로 문자열 "[flow.a].[node.b]"가 주어졌을 때
  When ParseNodePath("[flow.a].[node.b]")를 호출하면
  Then FlowRef는 "flow.a"여야 한다
  And NodeRef는 "node.b"여야 한다
```

### AC-FLOW-001-17: 잘못된 경로 파싱 에러

```gherkin
Scenario: 빈 문자열 파싱 에러
  Given 빈 문자열 ""이 주어졌을 때
  When ParseNodePath("")를 호출하면
  Then ErrInvalidPath 에러를 반환해야 한다

Scenario: 구분자 없는 단일 문자열
  Given "singlename"이 주어졌을 때
  When ParseNodePath("singlename")를 호출하면
  Then ErrInvalidPath 에러를 반환해야 한다

Scenario: 닫히지 않은 대괄호
  Given "[unclosed.bracket"이 주어졌을 때
  When ParseNodePath("[unclosed.bracket")를 호출하면
  Then ErrInvalidPath 에러를 반환해야 한다
```

### AC-FLOW-001-18: NodePath 문자열 변환

```gherkin
Scenario: 기본 NodePath 문자열 변환
  Given NodePath{FlowRef: "flow1", NodeRef: "node1"}이 주어졌을 때
  When String()를 호출하면
  Then "flow1.node1"을 반환해야 한다

Scenario: 마침표 포함 시 자동 대괄호 적용
  Given NodePath{FlowRef: "flow.name", NodeRef: "node1"}이 주어졌을 때
  When String()를 호출하면
  Then "[flow.name].node1"을 반환해야 한다
```

### AC-FLOW-001-19: Flow 내 노드 조회 (ID 우선)

```gherkin
Scenario: ID로 노드 조회
  Given Flow에 ID "abc-123", Name "filter-1"인 노드가 있을 때
  When Node("abc-123")를 호출하면
  Then (node, true)를 반환해야 한다
  And node.Name은 "filter-1"이어야 한다

Scenario: Name으로 노드 조회
  Given Flow에 Name "filter-1"인 노드가 있을 때
  When Node("filter-1")를 호출하면
  Then (node, true)를 반환해야 한다

Scenario: 존재하지 않는 노드 조회
  Given Flow에 해당 ID/Name의 노드가 없을 때
  When Node("nonexistent")를 호출하면
  Then (NodeDef{}, false)를 반환해야 한다
```

---

## Module 3: State Model - 상태 모델

### AC-FLOW-001-20: 유효 상태 전이

```gherkin
Scenario: Stored -> Loaded 전이
  Given State가 FlowStored인 Flow가 있을 때
  When SetState(FlowLoaded)를 호출하면
  Then 에러는 nil이어야 한다
  And State()는 FlowLoaded여야 한다
  And UpdatedAt이 갱신되어야 한다

Scenario: Running -> Paused 전이
  Given State가 FlowRunning인 Flow가 있을 때
  When SetState(FlowPaused)를 호출하면
  Then 에러는 nil이어야 한다
  And State()는 FlowPaused여야 한다

Scenario: Paused -> Running 전이 (재개)
  Given State가 FlowPaused인 Flow가 있을 때
  When SetState(FlowRunning)를 호출하면
  Then 에러는 nil이어야 한다
  And State()는 FlowRunning이어야 한다
```

### AC-FLOW-001-21: 유효하지 않은 상태 전이

```gherkin
Scenario: Stored -> Running 직접 전이 불가
  Given State가 FlowStored인 Flow가 있을 때
  When SetState(FlowRunning)를 호출하면
  Then ErrInvalidStateTransition 에러를 반환해야 한다
  And State()는 여전히 FlowStored여야 한다

Scenario: Running -> Stored 직접 전이 불가
  Given State가 FlowRunning인 Flow가 있을 때
  When SetState(FlowStored)를 호출하면
  Then ErrInvalidStateTransition 에러를 반환해야 한다
```

### AC-FLOW-001-22: IsValidTransition 함수

```gherkin
Scenario: 유효 전이 확인
  When IsValidTransition(FlowStored, FlowLoaded)를 호출하면
  Then true를 반환해야 한다

Scenario: 유효하지 않은 전이 확인
  When IsValidTransition(FlowStored, FlowRunning)를 호출하면
  Then false를 반환해야 한다
```

### AC-FLOW-001-23: FlowState 파싱

```gherkin
Scenario: 유효한 상태 문자열 파싱
  When ParseFlowState("running")를 호출하면
  Then FlowRunning을 반환해야 한다
  And 에러는 nil이어야 한다

Scenario: 유효하지 않은 상태 문자열 파싱
  When ParseFlowState("invalid")를 호출하면
  Then ErrInvalidFlowState 에러를 반환해야 한다
```

### AC-FLOW-001-24: 전체 상태 전이 맵 테스트

```gherkin
Scenario: 모든 유효 전이 경로 검증 (테이블 드리븐)
  Given ValidTransitions 맵의 모든 항목에 대해
  When 각 (from, to) 쌍으로 IsValidTransition을 호출하면
  Then 모두 true를 반환해야 한다

Scenario: 유효하지 않은 전이 샘플 검증
  Given 다음 (from, to) 쌍에 대해:
    | from         | to           |
    | Stored       | Running      |
    | Loaded       | Running      |
    | Running      | Stored       |
    | Paused       | Stored       |
    | Stopped      | Running      |
  When 각 쌍으로 IsValidTransition을 호출하면
  Then 모두 false를 반환해야 한다
```

---

## Module 4: Serialization & Loading - 직렬화 및 로딩

### AC-FLOW-001-25: JSON 라운드트립

```gherkin
Scenario: Flow JSON 직렬화 후 역직렬화 동일성
  Given 노드 2개와 Wire 1개가 포함된 Flow가 있을 때
  When MarshalJSON()으로 직렬화하고, FlowFromJSON()으로 역직렬화하면
  Then 복원된 Flow의 ID, Name, Description이 원본과 동일해야 한다
  And 복원된 Flow의 Nodes 수가 동일해야 한다
  And 복원된 Flow의 Wires 수가 동일해야 한다
  And 각 NodeDef의 ID, Name, Type, Config가 동일해야 한다
  And 각 Wire의 소스/타겟 정보가 동일해야 한다
```

### AC-FLOW-001-26: YAML 라운드트립

```gherkin
Scenario: Flow YAML 직렬화 후 역직렬화 동일성
  Given 노드 2개와 Wire 1개가 포함된 Flow가 있을 때
  When FlowToYAML()로 직렬화하고, FlowFromYAML()로 역직렬화하면
  Then 복원된 Flow가 원본과 동일해야 한다 (JSON 라운드트립과 동일한 검증 기준)
```

### AC-FLOW-001-27: 잘못된 JSON 역직렬화 에러

```gherkin
Scenario: 유효하지 않은 JSON
  Given 잘못된 JSON 바이트가 주어졌을 때
  When FlowFromJSON(data)를 호출하면
  Then 에러를 반환해야 한다

Scenario: Name 누락 JSON
  Given name 필드가 없는 JSON이 주어졌을 때
  When FlowFromJSON(data)를 호출하면
  Then ErrFlowNameRequired 에러를 반환해야 한다

Scenario: ID 누락 JSON은 자동 생성
  Given id 필드가 없는 유효한 JSON이 주어졌을 때
  When FlowFromJSON(data)를 호출하면
  Then 에러는 nil이어야 한다
  And 반환된 Flow의 ID는 유효한 UUID v4여야 한다
```

### AC-FLOW-001-28: 파일 로딩

```gherkin
Scenario: JSON 파일 로딩
  Given "/tmp/test-flow.json" 파일에 유효한 Flow JSON이 저장되어 있을 때
  When LoadFlowFromFile("/tmp/test-flow.json")를 호출하면
  Then 에러는 nil이어야 한다
  And 반환된 Flow가 유효해야 한다

Scenario: YAML 파일 로딩
  Given "/tmp/test-flow.yaml" 파일에 유효한 Flow YAML이 저장되어 있을 때
  When LoadFlowFromFile("/tmp/test-flow.yaml")를 호출하면
  Then 에러는 nil이어야 한다
  And 반환된 Flow가 유효해야 한다

Scenario: 지원하지 않는 확장자
  Given "/tmp/test-flow.xml" 파일이 존재할 때
  When LoadFlowFromFile("/tmp/test-flow.xml")를 호출하면
  Then ErrUnsupportedFormat 에러를 반환해야 한다
```

### AC-FLOW-001-29: 파일 저장 후 로딩 라운드트립

```gherkin
Scenario: 저장 후 로딩 동일성
  Given 유효한 Flow가 있을 때
  When SaveFlowToFile(flow, "/tmp/roundtrip.json")으로 저장하고
  And LoadFlowFromFile("/tmp/roundtrip.json")으로 로딩하면
  Then 복원된 Flow가 원본과 동일해야 한다
```

---

## Module 5: Validation - 유효성 검증

### AC-FLOW-001-30: 유효한 Flow 검증 통과

```gherkin
Scenario: 정상 Flow 검증
  Given nodeA(outputs: ["out"]), nodeB(inputs: ["in"])이 있고
  And nodeA.out -> nodeB.in Wire가 연결된 Flow가 있을 때
  When Validate(flow)를 호출하면
  Then 반환된 ValidationError 목록이 비어있어야 한다
```

### AC-FLOW-001-31: Wire 소스 노드 미존재

```gherkin
Scenario: 존재하지 않는 소스 노드
  Given Wire의 SourceNodeID가 Flow 내 어떤 노드 ID와도 일치하지 않을 때
  When Validate(flow)를 호출하면
  Then WIRE_ORPHAN_SOURCE 코드의 ValidationError가 포함되어야 한다
  And Severity는 SeverityError여야 한다
```

### AC-FLOW-001-32: Wire 타겟 노드 미존재

```gherkin
Scenario: 존재하지 않는 타겟 노드
  Given Wire의 TargetNodeID가 Flow 내 어떤 노드 ID와도 일치하지 않을 때
  When Validate(flow)를 호출하면
  Then WIRE_ORPHAN_TARGET 코드의 ValidationError가 포함되어야 한다
```

### AC-FLOW-001-33: 중복 Wire 검증

```gherkin
Scenario: 동일 소스-타겟 Wire 중복
  Given 동일한 (SourceNodeID, SourcePort, TargetNodeID, TargetPort) 조합의 Wire가 2개 존재할 때
  When Validate(flow)를 호출하면
  Then WIRE_DUPLICATE 코드의 ValidationError가 포함되어야 한다
```

### AC-FLOW-001-34: 자기 참조 Wire 경고

```gherkin
Scenario: 노드 자기 자신에 연결
  Given Wire의 SourceNodeID와 TargetNodeID가 동일할 때
  When Validate(flow)를 호출하면
  Then WIRE_SELF_REFERENCE 코드의 ValidationError가 포함되어야 한다
  And Severity는 SeverityWarning이어야 한다
```

### AC-FLOW-001-35: 노드 ID/이름 중복 검증

```gherkin
Scenario: 노드 ID 중복
  Given 동일 ID를 가진 두 NodeDef가 Flow에 존재할 때
  When Validate(flow)를 호출하면
  Then NODE_DUPLICATE_ID 코드의 ValidationError가 포함되어야 한다

Scenario: 노드 이름 중복
  Given 동일 Name을 가진 두 NodeDef가 Flow에 존재할 때
  When Validate(flow)를 호출하면
  Then NODE_DUPLICATE_NAME 코드의 ValidationError가 포함되어야 한다
```

### AC-FLOW-001-36: Bridge 노드 AgentRef 검증

```gherkin
Scenario: Bridge 노드에 AgentRef 없음
  Given Type이 "bridge"이고 AgentRef가 nil인 NodeDef가 있을 때
  When Validate(flow)를 호출하면
  Then NODE_BRIDGE_NO_AGENT 코드의 ValidationError가 포함되어야 한다
```

### AC-FLOW-001-37: 버퍼 모드 크기 검증

```gherkin
Scenario: 버퍼 모드에서 크기 0
  Given Wire의 Mode가 WireBuffer이고 BufferSize가 0일 때
  When Validate(flow)를 호출하면
  Then WIRE_BUFFER_INVALID_SIZE 코드의 ValidationError가 포함되어야 한다
```

### AC-FLOW-001-38: 연결되지 않은 노드 경고

```gherkin
Scenario: Wire에 연결되지 않은 노드
  Given Flow에 nodeA, nodeB, nodeC가 있고 nodeA->nodeB Wire만 존재할 때
  When Validate(flow)를 호출하면
  Then NODE_DISCONNECTED 코드의 ValidationError가 포함되어야 한다
  And 해당 경고의 대상은 nodeC여야 한다
  And Severity는 SeverityWarning이어야 한다
```

### AC-FLOW-001-39: 복합 검증 (다중 에러 반환)

```gherkin
Scenario: 여러 문제가 있는 Flow 검증
  Given 다음 문제가 있는 Flow:
    - 노드 ID 중복 1건
    - 존재하지 않는 소스 노드 Wire 1건
    - 연결 안 된 노드 1건
  When Validate(flow)를 호출하면
  Then 반환된 ValidationError 목록의 길이가 3 이상이어야 한다
  And Error severity가 2건, Warning severity가 1건이어야 한다
```

---

## Quality Gate

### Definition of Done

- [ ] Module 1-5의 모든 요구사항이 구현됨
- [ ] 모든 acceptance criteria 테스트가 통과함
- [ ] 테스트 커버리지 85% 이상
- [ ] `go vet ./pkg/flow/...` 경고 없음
- [ ] `golangci-lint run ./pkg/flow/...` 경고 없음
- [ ] `go test -race ./pkg/flow/...` 통과 (데이터 레이스 없음)
- [ ] 모든 exported 타입과 함수에 GoDoc 주석 작성
- [ ] JSON/YAML 라운드트립 테스트 통과
- [ ] SPEC-MSG-001과의 패턴 일관성 확인 (인터페이스, Options, 방어적 복사)
