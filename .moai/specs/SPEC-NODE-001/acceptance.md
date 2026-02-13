---
id: SPEC-NODE-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-NODE-001
---

# SPEC-NODE-001 수락 기준

## Module 1: Node Interface & BaseNode - 노드 인터페이스 및 기반 구현체

### AC-NODE-001-01: Node 인터페이스 컴파일 검증

```gherkin
Given Node 인터페이스가 정의되어 있을 때
Then ID() string 메서드가 포함되어야 한다
And Name() string 메서드가 포함되어야 한다
And Type() string 메서드가 포함되어야 한다
And Init(ctx context.Context) error 메서드가 포함되어야 한다
And Process(ctx context.Context, msg message.Message) ([]message.Message, error) 메서드가 포함되어야 한다
And Shutdown(ctx context.Context) error 메서드가 포함되어야 한다
And Configure(config map[string]any) error 메서드가 포함되어야 한다
And Ports() []Port 메서드가 포함되어야 한다
```

### AC-NODE-001-02: NewBaseNode 생성자

```gherkin
Given 입력 2개, 출력 1개, 에러 포트 1개가 정의된 flow.NodeDef가 주어졌을 때
When NewBaseNode(def)를 호출하면
Then BaseLifecycle이 StateCreated 상태인 BaseNode을 반환해야 한다
And ID()가 def.ID와 동일해야 한다
And Name()이 def.Name과 동일해야 한다
And Type()이 def.Type과 동일해야 한다
And Ports()가 4개(입력 2, 출력 1, 에러 1)를 반환해야 한다
```

### AC-NODE-001-03: NewBaseNode Options Pattern

```gherkin
Given flow.NodeDef와 WithNodeLogger(mockLogger) 옵션이 주어졌을 때
When NewBaseNode(def, WithNodeLogger(mockLogger))를 호출하면
Then BaseNode의 내부 logger가 mockLogger와 동일해야 한다

Given flow.NodeDef와 WithNodeMetrics(mockMetrics) 옵션이 주어졌을 때
When NewBaseNode(def, WithNodeMetrics(mockMetrics))를 호출하면
Then BaseNode의 내부 metrics가 mockMetrics와 동일해야 한다
```

### AC-NODE-001-04: 에러 출력 포트 전달

```gherkin
Given BaseNode를 임베딩한 TestNode가 Process에서 에러를 반환하도록 설정되어 있을 때
When TestNode.Process(ctx, msg)를 호출하면
Then 에러 메시지가 반환되어야 한다
And 에러 메시지의 Metadata에 원본 메시지 ID가 포함되어야 한다
And 에러 메시지의 Metadata에 에러 내용이 포함되어야 한다
And 에러 메시지의 Metadata에 노드 ID가 포함되어야 한다
```

### AC-NODE-001-05: Hot Configuration 성공

```gherkin
Given BaseNode가 초기화된 상태일 때
When Configure(map[string]any{"key": "value"})를 호출하면
Then nil error를 반환해야 한다
And GetConfig()["key"]가 "value"여야 한다
```

### AC-NODE-001-06: 잘못된 설정 거부

```gherkin
Given BaseNode가 Configure(validConfig)로 설정된 상태일 때
When Configure(invalidConfig)를 호출하여 에러가 반환되면
Then GetConfig()가 이전 validConfig를 유지해야 한다
```

### AC-NODE-001-07: 기본 에러 포트 자동 생성

```gherkin
Given flow.NodeDef.ErrorPort가 nil인 NodeDef가 주어졌을 때
When NewBaseNode(def)를 호출하면
Then Ports() 중 Direction이 PortError인 포트가 1개 존재해야 한다
And 해당 포트의 Name이 "_error"여야 한다
```

### AC-NODE-001-08: 노드 초기화 상태 전이

```gherkin
Given NewBaseNode(def)로 생성된 BaseNode가 StateCreated 상태일 때
When Init(ctx)를 호출하면 (성공 시)
Then 상태가 StateRunning이어야 한다

Given NewBaseNode(def)로 생성된 BaseNode가 StateCreated 상태일 때
When Init(ctx)를 호출하면 (실패 시)
Then 상태가 StateError여야 한다
And 에러를 반환해야 한다
```

### AC-NODE-001-09: 노드 종료 상태 전이

```gherkin
Given BaseNode가 StateRunning 상태일 때
When Shutdown(ctx)를 호출하면
Then 상태가 StateStopped여야 한다
```

---

## Module 2: Node Registry - 노드 레지스트리

### AC-NODE-001-10: NewRegistry 내장 타입 자동 등록

```gherkin
Given NewRegistry()를 호출할 때
Then Types()가 10개 타입을 반환해야 한다
And "filter", "transform", "switch", "aggregate", "bridge", "script", "debug", "catch", "status", "deadletter"이 포함되어야 한다
```

### AC-NODE-001-11: WithoutBuiltins 옵션

```gherkin
Given NewRegistry(WithoutBuiltins())를 호출할 때
Then Types()가 0개 타입을 반환해야 한다 (빈 레지스트리)
```

### AC-NODE-001-12: Register 성공

```gherkin
Given 빈 Registry가 있을 때
When Register("custom", customFactory)를 호출하면
Then nil error를 반환해야 한다
And Has("custom")이 true여야 한다
And Types()에 "custom"이 포함되어야 한다
```

### AC-NODE-001-13: Register 중복 거부

```gherkin
Given "filter" 타입이 이미 등록된 Registry가 있을 때
When Register("filter", anotherFactory)를 재호출하면
Then ErrNodeTypeAlreadyRegistered 에러를 반환해야 한다
And 기존 factory가 유지되어야 한다
```

### AC-NODE-001-14: Create 성공

```gherkin
Given "filter" 타입이 등록된 Registry가 있을 때
When Create(filterNodeDef)를 호출하면
Then Node 인터페이스를 구현한 인스턴스를 반환해야 한다
And 반환된 노드의 Type()이 "filter"여야 한다
```

### AC-NODE-001-15: Create 미등록 타입 거부

```gherkin
Given Registry에 "unknown" 타입이 등록되어 있지 않을 때
When Create(unknownNodeDef)를 호출하면
Then ErrNodeTypeNotFound 에러를 반환해야 한다
```

### AC-NODE-001-16: Registry 동시성 안전

```gherkin
Given Registry가 초기화된 상태일 때
When 10개 goroutine에서 동시에 Register()와 Create()를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 등록과 생성이 정상 처리되어야 한다
```

---

## Module 3: Port System - 포트 시스템

### AC-NODE-001-17: 포트 바인딩

```gherkin
Given Inputs: [{ID: "in1", Name: "input1"}], Outputs: [{ID: "out1", Name: "output1"}]인 NodeDef가 주어졌을 때
When NewBaseNode(def)를 호출하면
Then Ports()에 Direction이 PortInput인 포트가 1개 있어야 한다
And Ports()에 Direction이 PortOutput인 포트가 1개 있어야 한다
And Ports()에 Direction이 PortError인 포트가 1개 있어야 한다 (자동 생성)
```

### AC-NODE-001-18: PortDirection 상수 검증

```gherkin
Given PortDirection 타입이 정의되어 있을 때
Then PortInput이 정의되어 있어야 한다
And PortOutput이 정의되어 있어야 한다
And PortError가 정의되어 있어야 한다
And 세 값이 모두 다른 값이어야 한다
```

### AC-NODE-001-19: 존재하지 않는 포트 접근

```gherkin
Given BaseNode에 "out1" 출력 포트만 있을 때
When "out2" 포트로 메시지 전달을 시도하면
Then ErrPortNotFound 에러를 반환해야 한다
```

---

## Module 4: Filter Node - 필터 노드

### AC-NODE-001-20: 필터 조건 통과

```gherkin
Given FilterNode에 "Payload.Get('status') == 'active'" 조건이 설정되어 있을 때
When Process(ctx, msg)를 호출하고, msg의 Payload에 status: "active"가 있으면
Then 1개의 메시지를 반환해야 한다
And 반환된 메시지가 원본 msg와 동일해야 한다
```

### AC-NODE-001-21: 필터 조건 미통과

```gherkin
Given FilterNode에 "Payload.Get('status') == 'active'" 조건이 설정되어 있을 때
When Process(ctx, msg)를 호출하고, msg의 Payload에 status: "inactive"가 있으면
Then 빈 메시지 슬라이스(길이 0)를 반환해야 한다
And error는 nil이어야 한다
```

### AC-NODE-001-22: 필터 조건 Hot Reload

```gherkin
Given FilterNode가 조건 A로 동작 중일 때
When Configure(map[string]any{"condition": conditionB})를 호출하면
Then 다음 Process 호출부터 조건 B가 적용되어야 한다
```

---

## Module 5: Transform Node - 변환 노드

### AC-NODE-001-23: 메시지 변환 성공

```gherkin
Given TransformNode에 "temperature" 필드를 섭씨 -> 화씨로 변환하는 규칙이 설정되어 있을 때
When Process(ctx, msg)를 호출하고, msg.Payload에 temperature: 100이 있으면
Then 1개의 메시지를 반환해야 한다
And 반환된 메시지의 Payload에 temperature: 212가 있어야 한다
```

### AC-NODE-001-24: 변환 실패 시 에러 포트

```gherkin
Given TransformNode에 숫자 타입을 기대하는 변환 규칙이 설정되어 있을 때
When Process(ctx, msg)를 호출하고, msg.Payload에 temperature: "not_a_number"가 있으면
Then 에러 메시지가 반환되어야 한다
And error가 nil이 아니어야 한다
```

### AC-NODE-001-25: 다중 변환 규칙 순차 적용

```gherkin
Given TransformNode에 규칙 A(필드 추가)와 규칙 B(필드 변환) 2개가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then 규칙 A가 먼저 적용되고, 그 결과에 규칙 B가 적용되어야 한다
```

---

## Module 6: Switch Node - 스위치 노드

### AC-NODE-001-26: First-Match 라우팅

```gherkin
Given SwitchNode에 route1(status=="error" -> "error_port")과 route2(status=="warn" -> "warn_port") 2개 라우트가 설정되어 있을 때
When Process(ctx, msg)를 호출하고, msg의 status가 "error"이면
Then "error_port" 대상 메시지 1개를 반환해야 한다
```

### AC-NODE-001-27: 기본 라우트

```gherkin
Given SwitchNode에 route1과 default("default_port") 라우트가 설정되어 있을 때
When Process(ctx, msg)를 호출하고, route1 조건에 매칭되지 않으면
Then "default_port" 대상 메시지 1개를 반환해야 한다
```

### AC-NODE-001-28: 매칭 실패 시 폐기

```gherkin
Given SwitchNode에 route1만 있고 기본 라우트가 없을 때
When Process(ctx, msg)를 호출하고, route1 조건에 매칭되지 않으면
Then 빈 메시지 슬라이스를 반환해야 한다
And 드롭 메트릭이 1 증가해야 한다
```

### AC-NODE-001-29: 다중 출력 포트

```gherkin
Given SwitchNode에 3개 라우트(각기 다른 출력 포트)가 설정되어 있을 때
When Ports()를 호출하면
Then PortOutput 방향의 포트가 3개 이상 존재해야 한다
```

---

## Module 7: Aggregate Node - 집계 노드

### AC-NODE-001-30: 카운트 기반 윈도우

```gherkin
Given AggregateNode에 window_type: "count", window_size: 3, aggregate_fn: "count"가 설정되어 있을 때
When Process()를 메시지 3개에 대해 순차 호출하면
Then 1번째, 2번째 호출은 빈 결과를 반환해야 한다 (버퍼 축적)
And 3번째 호출은 집계 결과 메시지 1개를 반환해야 한다
And 집계 결과 Payload에 count: 3이 포함되어야 한다
```

### AC-NODE-001-31: 시간 기반 윈도우

```gherkin
Given AggregateNode에 window_type: "time", window_dur: 100ms, aggregate_fn: "count"가 설정되어 있을 때
When 100ms 이내에 Process()를 메시지 5개에 대해 호출하면
Then 각 호출은 빈 결과를 반환해야 한다 (시간 미도달)

When 100ms가 경과하면
Then 집계 결과 메시지 1개가 출력되어야 한다
And 집계 결과 Payload에 count: 5가 포함되어야 한다
```

### AC-NODE-001-32: 집계 함수 - sum

```gherkin
Given AggregateNode에 aggregate_fn: "sum"이 설정되어 있을 때
When Payload에 value: 10, value: 20, value: 30인 메시지 3개를 처리하면
Then 집계 결과 Payload에 sum: 60이 포함되어야 한다
```

### AC-NODE-001-33: 종료 시 잔여 버퍼 플러시

```gherkin
Given AggregateNode에 window_size: 5가 설정되어 있고, 3개 메시지만 축적된 상태일 때
When Shutdown(ctx)를 호출하면
Then 잔여 3개 메시지에 대한 집계 결과가 출력되어야 한다
And 내부 버퍼가 비어야 한다
```

### AC-NODE-001-34: 유효하지 않은 윈도우 설정 거부

```gherkin
Given AggregateNode가 초기화된 상태일 때
When Configure(map[string]any{"window_type": "invalid"})를 호출하면
Then ErrAggregateWindowInvalid 에러를 반환해야 한다
```

---

## Module 8: Bridge Node - 브릿지 노드

### AC-NODE-001-35: BridgeIn 모드 (Agent -> Flow)

```gherkin
Given BridgeNode가 BridgeIn 모드로 설정되고, Mock Agent가 연결되어 있을 때
When Agent가 메시지를 전송하면
Then BridgeNode의 출력 포트에서 해당 메시지가 Flow로 전달되어야 한다
```

### AC-NODE-001-36: BridgeOut 모드 (Flow -> Agent)

```gherkin
Given BridgeNode가 BridgeOut 모드로 설정되고, Mock Agent가 연결되어 있을 때
When Flow에서 BridgeNode의 입력 포트로 메시지가 전달되면
Then BridgeNode가 해당 메시지를 Agent로 전달해야 한다
```

### AC-NODE-001-37: BridgeInOut 모드 (양방향)

```gherkin
Given BridgeNode가 BridgeInOut 모드로 설정되어 있을 때
When Agent가 메시지 "request"를 전송하면
Then Flow 입력 포트로 "request"가 전달되어야 한다

When Flow 출력에서 메시지 "response"가 BridgeNode에 도착하면
Then Agent로 "response"가 전달되어야 한다
```

### AC-NODE-001-38: BridgeRequestReply 모드 - 성공

```gherkin
Given BridgeNode가 BridgeRequestReply 모드(타임아웃 5초)로 설정되어 있을 때
When Agent가 요청 메시지를 전송하면
Then Metadata에 _correlationID가 설정되어야 한다
And 요청이 Flow 입력 포트로 전달되어야 한다

When Flow에서 동일 _correlationID를 가진 응답 메시지가 돌아오면
Then Agent에게 응답이 전달되어야 한다
And sync.Map에서 해당 correlationID가 삭제되어야 한다
```

### AC-NODE-001-39: BridgeRequestReply 모드 - 타임아웃

```gherkin
Given BridgeNode가 BridgeRequestReply 모드(타임아웃 100ms)로 설정되어 있을 때
When Agent가 요청 메시지를 전송하고, 200ms 동안 응답이 없으면
Then ErrRequestTimeout 에러가 반환되어야 한다
And sync.Map에서 해당 correlationID가 삭제되어야 한다
```

### AC-NODE-001-40: BridgeRequestReply 동시 다중 요청

```gherkin
Given BridgeNode가 BridgeRequestReply 모드로 설정되어 있을 때
When 5개 goroutine에서 동시에 각각 다른 요청을 전송하면
Then 각 요청에 고유한 correlationID가 생성되어야 한다
And 각 응답이 올바른 요청자에게 전달되어야 한다
And race condition이 발생하지 않아야 한다 (go test -race 통과)
```

### AC-NODE-001-41: Bridge 초기화 - Agent 미발견

```gherkin
Given 존재하지 않는 AgentRef가 설정된 BridgeNode가 있을 때
When Init(ctx)를 호출하면
Then ErrAgentNotFound 에러를 반환해야 한다
And 상태가 StateError여야 한다
```

### AC-NODE-001-42: Bridge 종료 - Correlation 정리

```gherkin
Given BridgeRequestReply 모드에서 대기 중인 Correlation이 3개 있을 때
When Shutdown(ctx)를 호출하면
Then 3개 Correlation이 모두 타임아웃 처리되어야 한다
And sync.Map이 비어야 한다
```

---

## Module 9: Script Node - 스크립트 노드

### AC-NODE-001-43: 스크립트 실행 성공

```gherkin
Given ScriptNode에 "return {result = input.value * 2}" Lua 스크립트가 설정되어 있을 때
When Process(ctx, msg)를 호출하고, msg.Payload에 value: 5가 있으면
Then 1개의 메시지를 반환해야 한다
And 반환된 메시지의 Payload에 result: 10이 포함되어야 한다
```

### AC-NODE-001-44: 스크립트 컴파일 실패

```gherkin
Given ScriptNode에 문법 오류가 있는 Lua 스크립트가 설정되어 있을 때
When Init(ctx)를 호출하면
Then ErrScriptCompileFailed 에러를 반환해야 한다
And 상태가 StateError여야 한다
```

### AC-NODE-001-45: 스크립트 런타임 에러 격리

```gherkin
Given ScriptNode에 런타임 에러를 발생시키는 Lua 스크립트가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then 에러 메시지가 에러 출력 포트로 전달되어야 한다
And 노드 상태는 StateRunning을 유지해야 한다 (노드 생존)
And 다음 Process 호출이 정상 동작해야 한다
```

### AC-NODE-001-46: 스크립트 타임아웃

```gherkin
Given ScriptNode에 무한 루프 Lua 스크립트가 설정되어 있고, 타임아웃이 100ms일 때
When Process(ctx, msg)를 호출하면
Then 100ms 이내에 ErrScriptTimeout 에러가 에러 포트로 전달되어야 한다
And 노드 상태는 StateRunning을 유지해야 한다
```

### AC-NODE-001-47: 스크립트 Hot Reload

```gherkin
Given ScriptNode에 스크립트 A("return {v=1}")가 로드된 상태일 때
When Configure(map[string]any{"script": "return {v=2}"})를 호출하면
Then nil error를 반환해야 한다

When 다음 Process(ctx, msg)를 호출하면
Then 반환 메시지의 Payload에 v: 2가 포함되어야 한다 (스크립트 B 적용)
```

### AC-NODE-001-48: 스크립트 샌드박스

```gherkin
Given ScriptNode에 os.execute("rm -rf /") Lua 스크립트가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then 스크립트 실행 에러가 반환되어야 한다 (os 모듈 차단)
And 시스템에 영향이 없어야 한다
```

---

## Module 10: Debug Node - 디버그 노드

### AC-NODE-001-49: 메시지 로깅 및 Pass-Through

```gherkin
Given DebugNode에 로그 레벨 "info"가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then Logger에 info 레벨 로그가 기록되어야 한다
And 로그에 메시지 ID, Payload 요약, Metadata가 포함되어야 한다
And 1개의 메시지를 반환해야 한다
And 반환된 메시지가 원본 msg와 동일해야 한다 (pass-through)
```

### AC-NODE-001-50: 로그 레벨 설정

```gherkin
Given DebugNode가 초기화된 상태일 때
When Configure(map[string]any{"level": "warn"})를 호출하면
Then 다음 Process 호출 시 warn 레벨로 로그가 기록되어야 한다

Given Configure에 level 키가 없으면
Then 기본 로그 레벨 "debug"가 사용되어야 한다
```

---

## Module 11: Catch Node - 캐치 노드

### AC-NODE-001-51: 에러 메시지 수신 및 가공

```gherkin
Given CatchNode가 초기화된 상태일 때
When 에러 메시지(Metadata에 _error_node_id, _error_type, _error_message 포함)가 입력되면
Then 에러 정보를 추출하여 가공된 메시지를 출력 포트로 전달해야 한다
And 출력 메시지의 Payload에 에러 상세 정보가 포함되어야 한다
```

### AC-NODE-001-52: 에러 패턴 필터링

```gherkin
Given CatchNode에 catch_types: ["ErrScriptTimeout", "ErrScriptExecutionFailed"]가 설정되어 있을 때
When _error_type이 "ErrScriptTimeout"인 에러 메시지가 입력되면
Then 해당 메시지를 처리하여 출력 포트로 전달해야 한다

When _error_type이 "ErrInvalidConfig"인 에러 메시지가 입력되면
Then 해당 메시지를 그대로 통과시켜야 한다 (처리하지 않음)
```

### AC-NODE-001-53: 전체 에러 캐치

```gherkin
Given CatchNode에 catch_types 설정이 없거나 빈 배열일 때
When 어떤 에러 타입의 메시지가 입력되더라도
Then 모든 에러 메시지를 처리하여 출력 포트로 전달해야 한다
```

---

## Module 12: Status Node - 상태 노드

### AC-NODE-001-54: 상태 변경 이벤트 수신

```gherkin
Given StatusNode가 초기화되어 있고, 모니터링 대상이 전체 노드일 때
When Flow 내 노드 A의 상태가 StateRunning -> StatePaused로 변경되면
Then StatusNode의 출력에 상태 변경 이벤트 메시지가 전달되어야 한다
And 메시지 Payload에 node_id, previous_state, new_state, timestamp가 포함되어야 한다
```

### AC-NODE-001-55: 모니터링 대상 설정

```gherkin
Given StatusNode에 watch_nodes: ["node-A", "node-B"]가 설정되어 있을 때
When 노드 A의 상태가 변경되면
Then 상태 변경 이벤트가 출력되어야 한다

When 노드 C(모니터링 대상 아님)의 상태가 변경되면
Then 상태 변경 이벤트가 출력되지 않아야 한다
```

---

## Module 13: Dead Letter Node - 데드 레터 노드

### AC-NODE-001-56: TTL 만료 메시지 수신

```gherkin
Given DeadLetterNode가 초기화된 상태일 때
When Metadata에 _deadletter_reason: "ttl_expired"가 포함된 메시지가 입력되면
Then 메시지를 처리하여 출력 포트로 전달해야 한다
And 메시지 Payload에 원인 정보가 보강되어야 한다
And ttl_expired 메트릭 카운터가 1 증가해야 한다
```

### AC-NODE-001-57: 배달 불가 메시지 수신

```gherkin
Given DeadLetterNode가 초기화된 상태일 때
When Metadata에 _deadletter_reason: "undeliverable"가 포함된 메시지가 입력되면
Then undeliverable 메트릭 카운터가 1 증가해야 한다
```

### AC-NODE-001-58: 처리 전략 설정

```gherkin
Given DeadLetterNode에 strategy: "log"가 설정되어 있을 때
When 데드 레터 메시지가 입력되면
Then 로그에 메시지 상세 정보가 기록되어야 한다

Given DeadLetterNode에 strategy: "store"가 설정되어 있을 때
When 데드 레터 메시지가 입력되면
Then 스토어에 메시지가 저장되어야 한다

Given DeadLetterNode에 strategy: "forward"가 설정되어 있을 때
When 데드 레터 메시지가 입력되면
Then 설정된 대상 Flow로 메시지가 전달되어야 한다
```

---

## Module 14: Error Types - 에러 타입

### AC-NODE-001-59: Sentinel 에러 정의

```gherkin
Given errors 패키지가 임포트되어 있을 때
Then ErrNodeTypeAlreadyRegistered가 정의되어 있어야 한다
And ErrNodeTypeNotFound가 정의되어 있어야 한다
And ErrPortNotFound가 정의되어 있어야 한다
And ErrInvalidConfig가 정의되어 있어야 한다
And ErrAgentNotFound가 정의되어 있어야 한다
And ErrRequestTimeout이 정의되어 있어야 한다
And ErrScriptCompileFailed가 정의되어 있어야 한다
And ErrScriptExecutionFailed가 정의되어 있어야 한다
And ErrScriptTimeout이 정의되어 있어야 한다
And ErrNodeNotInitialized가 정의되어 있어야 한다
And ErrNodeAlreadyInitialized가 정의되어 있어야 한다
And ErrAggregateWindowInvalid가 정의되어 있어야 한다
```

### AC-NODE-001-60: errors.Is() 호환성

```gherkin
Given ErrNodeTypeNotFound 에러가 주어졌을 때
When fmt.Errorf("registry: %w", ErrNodeTypeNotFound)로 래핑한 후
Then errors.Is(wrappedErr, ErrNodeTypeNotFound)이 true를 반환해야 한다
```

### AC-NODE-001-61: NodeError 구조체

```gherkin
Given NodeError{NodeID: "node-1", NodeType: "filter", Err: ErrInvalidConfig}가 주어졌을 때
Then Error()가 노드 ID, 타입, 에러 메시지를 포함하는 문자열을 반환해야 한다
And Unwrap()이 ErrInvalidConfig를 반환해야 한다
And errors.Is(nodeErr, ErrInvalidConfig)이 true여야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-NODE-001-01 ~ 61) 테스트 통과
- [ ] `go test ./internal/node/...` 전체 통과
- [ ] `go test -race ./internal/node/...` 경쟁 상태 없음
- [ ] `go vet ./internal/node/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] 성능 목표 검증:
  - [ ] 노드 Process 단일 호출 지연시간: < 1ms (Filter/Transform/Switch/Debug)
  - [ ] Registry.Create 팩토리 호출: < 100us
  - [ ] Bridge RequestReply Correlation 매칭: < 1ms
  - [ ] Script Node Lua 실행: < 10ms (간단한 스크립트)
- [ ] goroutine 누수 없음 (Init/Shutdown 사이클 후 goroutine 수 원복 확인)
- [ ] Bridge RequestReply sync.Map 누수 없음 (Shutdown 후 빈 상태 확인)
- [ ] Script Node Lua VM 메모리 누수 없음 (Shutdown 후 해제 확인)
- [ ] 의존 패키지 인터페이스 mock 기반 독립 테스트 가능 확인

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go test -bench` | 벤치마크 테스트 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
| `runtime.NumGoroutine()` | goroutine 누수 검증 |
