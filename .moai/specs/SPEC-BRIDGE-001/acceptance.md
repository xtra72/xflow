---
id: SPEC-BRIDGE-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-BRIDGE-001
---

# SPEC-BRIDGE-001 수락 기준

## Module 1: BridgeNode Core - 브릿지 노드 코어

### AC-BRIDGE-001-01: NewBridgeNode 생성자

```gherkin
Given BridgeIn 모드, AgentRef.Name이 "sensor-agent"인 flow.NodeDef가 주어졌을 때
When NewBridgeNode(def)를 호출하면
Then Node 인터페이스를 구현한 BridgeNode를 반환해야 한다
And Type()이 "bridge"여야 한다
And BridgeConfig.Direction이 BridgeIn이어야 한다
And BridgeConfig.AgentRef.Name이 "sensor-agent"여야 한다
```

### AC-BRIDGE-001-02: NewBridgeNode 기본값 적용

```gherkin
Given flow.NodeDef에 RequestTimeout, ReconnectInterval이 설정되지 않았을 때
When NewBridgeNode(def)를 호출하면
Then BridgeConfig.RequestTimeout이 30초여야 한다
And BridgeConfig.ReconnectInterval이 5초여야 한다
And BridgeConfig.MaxReconnectAttempts가 10이어야 한다
And BridgeConfig.BufferSize가 256이어야 한다
And BridgeConfig.Transform.Mode가 "auto"여야 한다
```

### AC-BRIDGE-001-03: Bridge 초기화 성공

```gherkin
Given Mock Agent "sensor-agent"가 Running 상태로 Agent Registry에 등록되어 있을 때
And BridgeIn 모드의 BridgeNode가 생성되었을 때
When Init(ctx)를 호출하면
Then 에러가 nil이어야 한다
And Agent SharedRef.Acquire(flowID)가 호출되어야 한다
And 상태가 StateRunning이어야 한다
And 수신 루프 goroutine이 시작되어야 한다
```

### AC-BRIDGE-001-04: Bridge 초기화 - Agent 미발견

```gherkin
Given Agent Registry에 "unknown-agent"가 등록되어 있지 않을 때
And AgentRef.Name이 "unknown-agent"인 BridgeNode가 생성되었을 때
When Init(ctx)를 호출하면
Then ErrAgentNotFound 에러를 반환해야 한다
And 상태가 StateError여야 한다
```

### AC-BRIDGE-001-05: Bridge 종료

```gherkin
Given BridgeNode가 StateRunning 상태이고 Agent에 바인딩되어 있을 때
When Shutdown(ctx)를 호출하면
Then 수신 루프 goroutine이 중지되어야 한다
And Agent SharedRef.Release(flowID)가 호출되어야 한다
And Agent 바인딩이 해제되어야 한다
And 상태가 StateStopped여야 한다
```

### AC-BRIDGE-001-06: Bridge 종료 - goroutine 누수 없음

```gherkin
Given BridgeIn 모드로 Init()이 완료된 BridgeNode가 있을 때
And Init 전후의 goroutine 수를 기록했을 때
When Shutdown(ctx)를 호출하면
Then goroutine 수가 Init 전 수준으로 복원되어야 한다
```

---

## Module 2: Agent Binding - Agent 바인딩

### AC-BRIDGE-001-07: Agent 이름 기반 해상도

```gherkin
Given Agent Registry에 Name이 "temp-sensor"인 Agent가 등록되어 있을 때
And BridgeConfig.AgentRef.Name이 "temp-sensor"일 때
When Init(ctx)를 호출하면
Then Agent 해상도가 성공해야 한다
And 바인딩된 Agent의 Name()이 "temp-sensor"여야 한다
```

### AC-BRIDGE-001-08: Agent ID 기반 해상도

```gherkin
Given Agent Registry에 ID가 "agent-001"인 Agent가 등록되어 있을 때
And BridgeConfig.AgentRef.ID가 "agent-001"일 때
When Init(ctx)를 호출하면
Then Agent 해상도가 성공해야 한다
And 바인딩된 Agent의 ID()가 "agent-001"이어야 한다
```

### AC-BRIDGE-001-09: SharedRef 참조 획득 및 해제

```gherkin
Given Mock Agent의 SharedRef 참조 카운트가 0일 때
When BridgeNode.Init(ctx)를 호출하면
Then SharedRef.RefCount()가 1이어야 한다

When BridgeNode.Shutdown(ctx)를 호출하면
Then SharedRef.RefCount()가 0이어야 한다
```

### AC-BRIDGE-001-10: Agent 상태 변경 - Paused

```gherkin
Given BridgeIn 모드의 BridgeNode가 Running 상태의 Agent에 바인딩되어 있을 때
When Agent 상태가 Paused로 변경되면
Then Bridge Node의 수신 루프가 일시 중지되어야 한다
And 새로운 메시지가 출력 포트로 전달되지 않아야 한다
```

### AC-BRIDGE-001-11: Agent 상태 변경 - Stopped 및 재바인딩

```gherkin
Given BridgeNode가 Running 상태의 Agent에 바인딩되어 있을 때
And MaxReconnectAttempts가 3, ReconnectInterval이 50ms로 설정되어 있을 때
When Agent 상태가 Stopped로 변경되면
Then 에러 포트로 ErrAgentDisconnected가 전달되어야 한다
And 50ms 간격으로 재바인딩을 시도해야 한다

When 재바인딩 시도 중 Agent가 다시 Running 상태로 복귀하면
Then 재바인딩이 성공하고 Bridge가 정상 동작을 재개해야 한다
```

### AC-BRIDGE-001-12: 재바인딩 최대 시도 초과

```gherkin
Given MaxReconnectAttempts가 3으로 설정되어 있을 때
And Agent가 3번의 재바인딩 시도 동안 복구되지 않으면
Then ErrMaxReconnectExceeded 에러가 에러 포트로 전달되어야 한다
And 재바인딩 시도가 중단되어야 한다
```

### AC-BRIDGE-001-13: 멀티 브릿지 연결

```gherkin
Given Agent "shared-agent"가 Running 상태일 때
When 2개의 BridgeNode(BridgeA, BridgeB)가 동일 Agent에 Init()를 호출하면
Then SharedRef.RefCount()가 2여야 한다
And 두 Bridge가 독립적으로 메시지를 수신해야 한다

When BridgeA.Shutdown()을 호출하면
Then SharedRef.RefCount()가 1이어야 한다
And BridgeB는 정상 동작을 유지해야 한다
```

---

## Module 3: Communication Modes - 통신 모드

### AC-BRIDGE-001-14: BridgeIn 모드 - Agent에서 Flow로

```gherkin
Given BridgeIn 모드의 BridgeNode가 Mock Agent에 바인딩되어 있을 때
And Agent가 []byte{0x01, 0x02, 0x03} 데이터를 반환하도록 설정되어 있을 때
When 수신 루프가 Agent.Process()를 호출하면
Then DefaultTransformer.AgentToFlow()로 변환된 Message가 생성되어야 한다
And 출력 포트로 Message가 전달되어야 한다
And MessagesFromAgent 카운터가 1 증가해야 한다
```

### AC-BRIDGE-001-15: BridgeOut 모드 - Flow에서 Agent로

```gherkin
Given BridgeOut 모드의 BridgeNode가 Mock Agent에 바인딩되어 있을 때
And Payload에 _raw: []byte{0x04, 0x05}가 포함된 Message가 주어졌을 때
When Process(ctx, msg)를 호출하면
Then DefaultTransformer.FlowToAgent()로 변환된 []byte가 Agent에 전달되어야 한다
And MessagesToAgent 카운터가 1 증가해야 한다
And MessagesRelayed 카운터가 1 증가해야 한다
```

### AC-BRIDGE-001-16: BridgeInOut 모드 - 양방향

```gherkin
Given BridgeInOut 모드의 BridgeNode가 Mock Agent에 바인딩되어 있을 때
When Flow에서 입력 메시지가 Process()로 전달되면
Then 메시지가 Agent로 변환되어 송신되어야 한다

When Agent에서 수신 데이터가 도착하면
Then 메시지가 출력 포트로 변환되어 전달되어야 한다
And 수신과 송신이 동시에 처리되어야 한다
```

### AC-BRIDGE-001-17: BridgeRequestReply 모드 - 성공

```gherkin
Given BridgeRequestReply 모드(타임아웃 5초)의 BridgeNode가 Mock Agent에 바인딩되어 있을 때
And Agent가 요청에 대해 즉시 응답을 반환하도록 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then 요청 메시지의 Metadata에 _correlationID가 설정되어야 한다
And Agent에 변환된 요청 데이터가 전달되어야 한다
And Correlation ID가 CorrelationTracker에 등록되어야 한다

When Agent로부터 동일 _correlationID의 응답이 도착하면
Then 응답 Message가 반환되어야 한다
And PendingCorrelations가 0이어야 한다
```

### AC-BRIDGE-001-18: BridgeRequestReply 모드 - 타임아웃

```gherkin
Given BridgeRequestReply 모드(타임아웃 100ms)의 BridgeNode가 Mock Agent에 바인딩되어 있을 때
And Agent가 응답을 반환하지 않도록 설정되어 있을 때
When Process(ctx, msg)를 호출하고 200ms를 대기하면
Then ErrRequestTimeout 에러가 반환되어야 한다
And CorrelationTimeouts 카운터가 1 증가해야 한다
And PendingCorrelations가 0이어야 한다
```

### AC-BRIDGE-001-19: BridgeRequestReply 동시 다중 요청

```gherkin
Given BridgeRequestReply 모드의 BridgeNode가 Mock Agent에 바인딩되어 있을 때
When 5개 goroutine에서 동시에 각각 다른 요청 메시지로 Process()를 호출하면
Then 각 요청에 고유한 _correlationID가 생성되어야 한다
And 각 응답이 올바른 요청자에게 전달되어야 한다
And race condition이 발생하지 않아야 한다 (go test -race 통과)
```

### AC-BRIDGE-001-20: 수신 버퍼 초과

```gherkin
Given BridgeIn 모드의 BridgeNode에 BufferSize가 2로 설정되어 있을 때
And 수신 버퍼에 이미 2개의 메시지가 차 있을 때
When 수신 루프에서 3번째 메시지를 버퍼에 넣으려 하면
Then ErrBufferFull 에러가 에러 포트로 전달되어야 한다
And 3번째 메시지는 폐기되어야 한다
```

---

## Module 4: Message Transformation - 메시지 변환

### AC-BRIDGE-001-21: DefaultTransformer - AgentToFlow

```gherkin
Given DefaultTransformer가 생성되어 있을 때
When AgentToFlow([]byte{0x01, 0x02, 0x03})를 호출하면
Then Message가 반환되어야 한다
And Message.Payload의 "_raw" 키에 []byte{0x01, 0x02, 0x03}이 설정되어야 한다
```

### AC-BRIDGE-001-22: DefaultTransformer - FlowToAgent

```gherkin
Given DefaultTransformer가 생성되어 있을 때
And Message.Payload에 "_raw": []byte{0x04, 0x05, 0x06}이 설정되어 있을 때
When FlowToAgent(msg)를 호출하면
Then []byte{0x04, 0x05, 0x06}이 반환되어야 한다
```

### AC-BRIDGE-001-23: DefaultTransformer - 왕복 변환

```gherkin
Given 원본 데이터 []byte{0x10, 0x20, 0x30}이 주어졌을 때
When AgentToFlow(data)로 변환 후 FlowToAgent(msg)로 다시 변환하면
Then 결과가 원본 데이터 []byte{0x10, 0x20, 0x30}과 동일해야 한다
```

### AC-BRIDGE-001-24: DefaultTransformer - _raw 키 누락

```gherkin
Given DefaultTransformer가 생성되어 있을 때
And Message.Payload에 "_raw" 키가 없을 때
When FlowToAgent(msg)를 호출하면
Then ErrTransformFailed 에러를 반환해야 한다
```

### AC-BRIDGE-001-25: LuaTransformer 생성

```gherkin
Given TransformConfig에 Mode: "lua", LuaScript: "transform.lua"가 설정되어 있을 때
And "transform.lua" 파일에 유효한 Lua 변환 스크립트가 있을 때
When NewLuaTransformer(config)를 호출하면
Then LuaTransformer가 에러 없이 생성되어야 한다
```

### AC-BRIDGE-001-26: 변환 에러 처리

```gherkin
Given BridgeOut 모드에서 FlowToAgent 변환이 실패하도록 Mock Transformer가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then ErrTransformFailed 에러가 에러 포트로 전달되어야 한다
And TransformErrors 카운터가 1 증가해야 한다
```

### AC-BRIDGE-001-27: 변환기 Hot Reload

```gherkin
Given BridgeNode가 DefaultTransformer로 동작 중일 때
When Configure(map[string]any{"transform": TransformConfig{Mode: "auto"}})를 호출하면
Then nil 에러를 반환해야 한다
And 다음 Process 호출부터 새 Transformer가 적용되어야 한다
```

---

## Module 5: Request-Reply Correlation - 요청-응답 Correlation 관리

### AC-BRIDGE-001-28: CorrelationTracker - Track 및 Resolve

```gherkin
Given NewCorrelationTracker(30초)로 생성된 Tracker가 있을 때
And responseCh := make(chan message.Message, 1)이 주어졌을 때
When Track("corr-001", responseCh)를 호출하면
Then PendingCount()가 1이어야 한다

When Resolve("corr-001", responseMsg)를 호출하면
Then true를 반환해야 한다
And responseCh에서 responseMsg를 수신할 수 있어야 한다
And PendingCount()가 0이어야 한다
```

### AC-BRIDGE-001-29: CorrelationTracker - 미등록 ID Resolve

```gherkin
Given CorrelationTracker에 "corr-001"이 등록되어 있지 않을 때
When Resolve("corr-unknown", responseMsg)를 호출하면
Then false를 반환해야 한다
```

### AC-BRIDGE-001-30: CorrelationTracker - Cleanup 만료 항목 정리

```gherkin
Given CorrelationTracker에 타임아웃 100ms가 설정되어 있을 때
And Track("corr-expired", responseCh)가 호출된 후 200ms가 경과했을 때
When Cleanup()를 호출하면
Then "corr-expired" 항목이 삭제되어야 한다
And responseCh가 닫혀야 한다
And PendingCount()가 0이어야 한다
```

### AC-BRIDGE-001-31: CorrelationTracker - Close 전체 정리

```gherkin
Given CorrelationTracker에 3개 항목("corr-1", "corr-2", "corr-3")이 등록되어 있을 때
When Close()를 호출하면
Then 3개 항목의 responseCh가 모두 닫혀야 한다
And PendingCount()가 0이어야 한다
```

### AC-BRIDGE-001-32: CorrelationTracker - 동시성 안전

```gherkin
Given CorrelationTracker가 초기화된 상태일 때
When 10개 goroutine에서 동시에 Track()과 Resolve()를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 Track/Resolve 결과가 정확해야 한다
```

### AC-BRIDGE-001-33: StartCleanupLoop 주기적 정리

```gherkin
Given CorrelationTracker에 타임아웃 100ms가 설정되어 있을 때
And StartCleanupLoop(ctx)가 시작되었을 때
When Track("auto-cleanup", responseCh)를 호출한 후 150ms를 대기하면
Then 자동 정리 루프가 "auto-cleanup" 항목을 삭제해야 한다
And PendingCount()가 0이어야 한다
```

---

## Module 6: Bridge Configuration - 브릿지 설정

### AC-BRIDGE-001-34: BridgeConfig 유효한 설정 검증

```gherkin
Given BridgeConfig에 AgentRef.Name: "agent1", Direction: BridgeIn, Transform.Mode: "auto", BufferSize: 256이 설정되어 있을 때
When Validate()를 호출하면
Then nil 에러를 반환해야 한다
```

### AC-BRIDGE-001-35: BridgeConfig - AgentRef 누락

```gherkin
Given BridgeConfig에 AgentRef.Name과 AgentRef.ID가 모두 비어 있을 때
When Validate()를 호출하면
Then 에러를 반환해야 한다
```

### AC-BRIDGE-001-36: BridgeConfig - 유효하지 않은 Direction

```gherkin
Given BridgeConfig에 Direction이 유효하지 않은 값(예: 99)으로 설정되어 있을 때
When Validate()를 호출하면
Then ErrInvalidDirection 에러를 반환해야 한다
```

### AC-BRIDGE-001-37: BridgeConfig - Lua 모드에서 스크립트 누락

```gherkin
Given BridgeConfig에 Transform.Mode: "lua", Transform.LuaScript: ""가 설정되어 있을 때
When Validate()를 호출하면
Then 에러를 반환해야 한다
```

### AC-BRIDGE-001-38: BridgeConfig - BufferSize 범위 검증

```gherkin
Given BridgeConfig에 BufferSize: 0이 설정되어 있을 때
When Validate()를 호출하면
Then 에러를 반환해야 한다

Given BridgeConfig에 BufferSize: 1이 설정되어 있을 때
When Validate()를 호출하면
Then nil 에러를 반환해야 한다
```

### AC-BRIDGE-001-39: BridgeConfig - RequestReply 모드 타임아웃 검증

```gherkin
Given BridgeConfig에 Direction: BridgeRequestReply, RequestTimeout: 0이 설정되어 있을 때
When Validate()를 호출하면
Then 에러를 반환해야 한다

Given BridgeConfig에 Direction: BridgeRequestReply, RequestTimeout: 30초가 설정되어 있을 때
When Validate()를 호출하면
Then nil 에러를 반환해야 한다
```

---

## Module 7: Bridge Info & Stats - 브릿지 정보 및 통계

### AC-BRIDGE-001-40: Info() 연결 상태 반영

```gherkin
Given BridgeNode가 Agent "sensor-001"에 바인딩되어 있을 때
When Info()를 호출하면
Then AgentID가 "sensor-001"의 ID와 일치해야 한다
And AgentName이 "sensor-001"의 Name과 일치해야 한다
And Direction이 설정된 BridgeDirection과 일치해야 한다
And Connected가 true여야 한다
```

### AC-BRIDGE-001-41: Info() 연결 끊김 상태

```gherkin
Given BridgeNode의 Agent 연결이 끊긴 상태일 때
When Info()를 호출하면
Then Connected가 false여야 한다
```

### AC-BRIDGE-001-42: Stats() 메트릭 정확성

```gherkin
Given BridgeOut 모드의 BridgeNode가 5개 메시지를 Agent로 전달했을 때
When Stats()를 호출하면
Then MessagesToAgent가 5여야 한다
And MessagesRelayed가 5여야 한다
And LastActivityAt이 마지막 메시지 전달 시각 이후여야 한다
```

### AC-BRIDGE-001-43: Stats() 변환 에러 카운팅

```gherkin
Given BridgeNode에서 변환 에러가 2회 발생했을 때
When Stats()를 호출하면
Then TransformErrors가 2여야 한다
```

### AC-BRIDGE-001-44: Stats() 원자적 업데이트

```gherkin
Given BridgeNode가 동작 중일 때
When 10개 goroutine에서 동시에 메시지를 처리하며 Stats()를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And 모든 카운터가 정확한 값을 반환해야 한다
```

---

## Module 8: Error Types - 에러 타입

### AC-BRIDGE-001-45: Sentinel 에러 정의

```gherkin
Given bridge_errors.go 패키지가 로드되어 있을 때
Then ErrAgentNotFound가 정의되어 있어야 한다
And ErrAgentNotRunning이 정의되어 있어야 한다
And ErrAgentDisconnected가 정의되어 있어야 한다
And ErrRequestTimeout이 정의되어 있어야 한다
And ErrCorrelationNotFound가 정의되어 있어야 한다
And ErrTransformFailed가 정의되어 있어야 한다
And ErrInvalidDirection이 정의되어 있어야 한다
And ErrMaxReconnectExceeded가 정의되어 있어야 한다
And ErrBufferFull이 정의되어 있어야 한다
```

### AC-BRIDGE-001-46: errors.Is() 호환성

```gherkin
Given ErrAgentNotFound 에러가 주어졌을 때
When fmt.Errorf("init: %w", ErrAgentNotFound)로 래핑한 후
Then errors.Is(wrappedErr, ErrAgentNotFound)이 true를 반환해야 한다
```

### AC-BRIDGE-001-47: 모든 에러 고유성

```gherkin
Given 9개의 Bridge sentinel 에러가 정의되어 있을 때
Then 모든 에러의 Error() 문자열이 서로 다른 값이어야 한다
And 모든 에러가 "bridge:" 접두사를 포함해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-BRIDGE-001-01 ~ 47) 테스트 통과
- [ ] `go test ./internal/node/...` 전체 통과
- [ ] `go test -race ./internal/node/...` 경쟁 상태 없음
- [ ] `go vet ./internal/node/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] 성능 목표 검증:
  - [ ] Bridge Process 단일 호출 (BridgeOut): < 1ms
  - [ ] DefaultTransformer 양방향 변환: < 100us
  - [ ] CorrelationTracker Track/Resolve: < 50us
  - [ ] 동시 Request-Reply 처리량: 10,000 req/s
- [ ] goroutine 누수 없음 (Init/Shutdown 사이클 후 goroutine 수 원복 확인)
- [ ] CorrelationTracker sync.Map 누수 없음 (Shutdown 후 PendingCount == 0 확인)
- [ ] Agent SharedRef 참조 카운트 누수 없음 (Shutdown 후 RefCount 감소 확인)

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
