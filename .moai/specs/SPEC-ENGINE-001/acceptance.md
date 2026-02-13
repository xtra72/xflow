---
id: SPEC-ENGINE-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-ENGINE-001
---

# SPEC-ENGINE-001 수락 기준

## Module 1: Engine Core - 엔진 코어

### AC-ENGINE-001-01: Engine 생성자

```gherkin
Given NewEngine()를 호출할 때
Then BaseLifecycle이 StateCreated 상태인 Engine 인스턴스를 반환해야 한다

Given NewEngine(WithShutdownTimeout(10 * time.Second))를 호출할 때
Then Shutdown 타임아웃이 10초로 설정된 Engine을 반환해야 한다
```

### AC-ENGINE-001-02: DeployFlow 성공

```gherkin
Given 유효한 Flow 정의(노드 2개, Wire 1개)가 주어졌을 때
When Engine.DeployFlow(ctx, flow)를 호출하면
Then nil error를 반환해야 한다
And 해당 Flow의 FlowState가 FlowLoaded여야 한다
And Engine.GetFlowStatus(flowID)가 성공해야 한다
And FlowStatus.NodeCount가 2여야 한다
And FlowStatus.WireCount가 1이어야 한다
```

### AC-ENGINE-001-03: DeployFlow 중복 거부

```gherkin
Given Flow A가 이미 배포되어 있을 때
When 동일한 Flow ID로 Engine.DeployFlow(ctx, flowA)를 재호출하면
Then ErrFlowAlreadyDeployed 에러를 반환해야 한다
```

### AC-ENGINE-001-04: DeployFlow 유효성 검증 실패

```gherkin
Given Wire의 SourceNodeID가 존재하지 않는 노드를 참조하는 Flow가 주어졌을 때
When Engine.DeployFlow(ctx, invalidFlow)를 호출하면
Then ErrFlowValidationFailed 에러를 반환해야 한다
```

### AC-ENGINE-001-05: StartFlow 성공

```gherkin
Given Flow가 FlowLoaded 상태로 배포되어 있을 때
When Engine.StartFlow(ctx, flowID)를 호출하면
Then nil error를 반환해야 한다
And 해당 Flow의 FlowState가 FlowRunning이어야 한다
And FlowStatus.ActiveNodes가 전체 노드 수와 같아야 한다
```

### AC-ENGINE-001-06: StartFlow 상태 검증

```gherkin
Given Flow가 FlowRunning 상태일 때
When Engine.StartFlow(ctx, flowID)를 호출하면
Then ErrFlowNotLoaded 에러를 반환해야 한다

Given Flow가 FlowStopped 상태일 때
When Engine.StartFlow(ctx, flowID)를 호출하면
Then ErrFlowNotLoaded 에러를 반환해야 한다
```

### AC-ENGINE-001-07: StartFlow 노드 시작 실패 롤백

```gherkin
Given 3개 노드를 가진 Flow가 배포되어 있고, 2번째 노드의 시작이 실패하도록 설정되어 있을 때
When Engine.StartFlow(ctx, flowID)를 호출하면
Then ErrNodeStartFailed 에러를 반환해야 한다
And FlowState가 FlowError여야 한다
And 이미 시작된 1번째 노드의 goroutine이 정리되어야 한다
```

### AC-ENGINE-001-08: StopFlow Graceful Shutdown

```gherkin
Given Flow가 FlowRunning 상태이고, 메시지가 처리 중일 때
When Engine.StopFlow(ctx, flowID)를 호출하면
Then nil error를 반환해야 한다
And FlowState가 FlowStopped여야 한다
And 진행 중이던 메시지가 완전히 처리된 후 종료되어야 한다
And 모든 goroutine이 종료되어야 한다 (goroutine 누수 없음)
```

### AC-ENGINE-001-09: StopFlow 타임아웃

```gherkin
Given Flow에 종료되지 않는 노드(무한 루프)가 포함되어 있을 때
When Engine.StopFlow(ctx, flowID)를 호출하면 (ShutdownTimeout 2초)
Then ErrShutdownTimeout 에러를 반환해야 한다
And FlowState가 FlowStopped여야 한다 (강제 종료)
And 모든 리소스가 정리되어야 한다
```

### AC-ENGINE-001-10: PauseFlow 성공

```gherkin
Given Flow가 FlowRunning 상태일 때
When Engine.PauseFlow(ctx, flowID)를 호출하면
Then nil error를 반환해야 한다
And FlowState가 FlowPaused여야 한다
And 모든 노드가 입력 채널 읽기를 중단해야 한다
And Wire 채널은 열린 상태로 유지되어야 한다
```

### AC-ENGINE-001-11: PauseFlow 상태 검증

```gherkin
Given Flow가 FlowPaused 상태일 때
When Engine.PauseFlow(ctx, flowID)를 재호출하면
Then ErrFlowNotRunning 에러를 반환해야 한다

Given Flow가 FlowStopped 상태일 때
When Engine.PauseFlow(ctx, flowID)를 호출하면
Then ErrFlowNotRunning 에러를 반환해야 한다
```

### AC-ENGINE-001-12: ResumeFlow 성공 및 큐 데이터 처리

```gherkin
Given Flow가 FlowPaused 상태이고, Wire 채널에 메시지 3개가 큐잉되어 있을 때
When Engine.ResumeFlow(ctx, flowID)를 호출하면
Then nil error를 반환해야 한다
And FlowState가 FlowRunning이어야 한다
And 큐잉된 3개 메시지가 순서대로 처리되어야 한다
```

### AC-ENGINE-001-13: ResumeFlow 상태 검증

```gherkin
Given Flow가 FlowRunning 상태일 때
When Engine.ResumeFlow(ctx, flowID)를 호출하면
Then ErrFlowNotPaused 에러를 반환해야 한다
```

### AC-ENGINE-001-14: UndeployFlow 성공

```gherkin
Given Flow가 FlowStopped 상태일 때
When Engine.UndeployFlow(ctx, flowID)를 호출하면
Then nil error를 반환해야 한다
And Engine.GetFlowStatus(flowID)가 ErrFlowNotFound를 반환해야 한다
```

### AC-ENGINE-001-15: UndeployFlow 상태 검증

```gherkin
Given Flow가 FlowRunning 상태일 때
When Engine.UndeployFlow(ctx, flowID)를 호출하면
Then ErrFlowNotStopped 에러를 반환해야 한다
```

### AC-ENGINE-001-16: GetFlowStatus 및 ListFlows

```gherkin
Given 3개의 Flow(A: Running, B: Paused, C: Stopped)가 배포되어 있을 때
When Engine.ListFlows()를 호출하면
Then 3개의 FlowStatus를 반환해야 한다

Given Flow A가 배포되어 있을 때
When Engine.GetFlowStatus(flowA_ID)를 호출하면
Then FlowStatus에 FlowID, FlowName, State, NodeCount, WireCount가 포함되어야 한다

Given 존재하지 않는 flowID가 주어졌을 때
When Engine.GetFlowStatus(unknownID)를 호출하면
Then ErrFlowNotFound 에러를 반환해야 한다
```

### AC-ENGINE-001-17: 전체 생명주기 통합

```gherkin
Given 유효한 Flow 정의가 주어졌을 때
When Deploy -> Start -> Pause -> Resume -> Stop -> Undeploy 전체 주기를 실행하면
Then 각 단계에서 FlowState가 올바르게 전이해야 한다
  | 단계 | 이전 상태 | 이후 상태 |
  | Deploy | - | FlowLoaded |
  | Start | FlowLoaded | FlowRunning |
  | Pause | FlowRunning | FlowPaused |
  | Resume | FlowPaused | FlowRunning |
  | Stop | FlowRunning | FlowStopped |
  | Undeploy | FlowStopped | (제거됨) |
And 모든 리소스가 정리되어야 한다
```

### AC-ENGINE-001-18: 동시 다중 플로우 실행

```gherkin
Given 10개의 독립적인 Flow 정의가 주어졌을 때
When 10개 모두 Deploy -> Start를 수행하면
Then 10개 Flow가 동시에 FlowRunning 상태여야 한다
And ListFlows()가 10개의 FlowStatus를 반환해야 한다
And race condition이 발생하지 않아야 한다 (go test -race 통과)
```

---

## Module 2: Scheduler - 노드 스케줄링

### AC-ENGINE-001-19: 선형 체인 위상 정렬

```gherkin
Given A -> B -> C 순서의 선형 체인 Flow가 주어졌을 때
When Scheduler.Plan(flow)를 호출하면
Then ExecutionPlan.Order가 [A, B, C] 순서여야 한다
And ExecutionPlan.Levels가 [[A], [B], [C]]여야 한다 (각 레벨 1개 노드)
```

### AC-ENGINE-001-20: 병렬 분기 위상 정렬

```gherkin
Given A -> (B, C) -> D 구조의 Flow가 주어졌을 때 (B와 C는 A의 출력을 각각 받아 D에 전달)
When Scheduler.Plan(flow)를 호출하면
Then ExecutionPlan.Levels[0]이 [A]여야 한다
And ExecutionPlan.Levels[1]이 [B, C]를 포함해야 한다 (순서 무관)
And ExecutionPlan.Levels[2]가 [D]여야 한다
```

### AC-ENGINE-001-21: 순환 그래프 감지

```gherkin
Given A -> B -> C -> A 순환이 있는 Flow가 주어졌을 때
When Scheduler.Plan(flow)를 호출하면
Then ErrCycleDetected 에러를 반환해야 한다
```

### AC-ENGINE-001-22: 빈 Flow 처리

```gherkin
Given 노드가 0개인 빈 Flow가 주어졌을 때
When Scheduler.Plan(flow)를 호출하면
Then 빈 ExecutionPlan(Levels: [], Order: [])을 반환해야 한다
And error는 nil이어야 한다
```

### AC-ENGINE-001-23: 단일 노드 Flow

```gherkin
Given 노드 1개, Wire 0개인 Flow가 주어졌을 때
When Scheduler.Plan(flow)를 호출하면
Then ExecutionPlan.Levels가 [[nodeID]]여야 한다
And ExecutionPlan.Order가 [nodeID]여야 한다
```

### AC-ENGINE-001-24: Fan-out/Fan-in 토폴로지

```gherkin
Given A -> (B, C, D) -> E 구조의 Fan-out/Fan-in Flow가 주어졌을 때
When Scheduler.Plan(flow)를 호출하면
Then Level 0: [A], Level 1: [B, C, D], Level 2: [E]로 분류해야 한다
```

---

## Module 3: Wire System - Wire 런타임

### AC-ENGINE-001-25: 바이패스 모드 Wire 생성

```gherkin
Given WireMode가 WireBypass인 Wire 정의가 주어졌을 때
When CreateRuntimeWires()를 호출하면
Then RuntimeWire.Ch이 unbuffered 채널이어야 한다 (cap(ch) == 0)
```

### AC-ENGINE-001-26: 버퍼 모드 Wire 생성

```gherkin
Given WireMode가 WireBuffer이고 BufferSize가 100인 Wire 정의가 주어졌을 때
When CreateRuntimeWires()를 호출하면
Then RuntimeWire.Ch이 buffered 채널이어야 한다 (cap(ch) == 100)
```

### AC-ENGINE-001-27: 수신 노드 Running 상태 시 즉시 전달

```gherkin
Given RuntimeWire가 생성되어 있고, 수신 노드가 Running 상태일 때
When 송신 노드가 메시지를 Wire에 전송하면
Then 수신 노드가 해당 메시지를 받아야 한다
And 메시지 내용이 변경되지 않아야 한다
```

### AC-ENGINE-001-28: 수신 노드 Paused 상태 시 블로킹/버퍼링

```gherkin
Given 바이패스 모드 Wire가 있고, 수신 노드가 Paused 상태일 때
When 송신 노드가 메시지를 Wire에 전송하면
Then 송신 노드가 블로킹(대기)되어야 한다

Given 버퍼 모드 Wire(크기 10)가 있고, 수신 노드가 Paused 상태일 때
When 송신 노드가 메시지 5개를 Wire에 전송하면
Then 5개 메시지가 버퍼에 축적되어야 한다
And 송신 노드가 블로킹되지 않아야 한다 (버퍼 미충족)

When 송신 노드가 추가로 메시지 5개를 전송하여 버퍼가 가득 차면
Then 다음 메시지 전송 시 송신 노드가 블로킹되어야 한다
```

### AC-ENGINE-001-29: 수신 노드 Stopped 상태 시 에러 포트 라우팅

```gherkin
Given 수신 노드가 Stopped 상태이고, 에러 포트가 연결되어 있을 때
When 송신 노드가 메시지를 Wire에 전송하면
Then 메시지가 에러 포트로 라우팅되어야 한다

Given 수신 노드가 Stopped 상태이고, 에러 포트가 없을 때
When 송신 노드가 메시지를 Wire에 전송하면
Then 메시지가 폐기되어야 한다
And 메트릭(드롭 카운터)이 증가해야 한다
```

### AC-ENGINE-001-30: Fan-out 메시지 복제

```gherkin
Given 노드 A의 출력 포트에 Wire 3개(-> B, -> C, -> D)가 연결되어 있을 때
When 노드 A가 메시지 1개를 출력하면
Then B, C, D 각각이 동일한 내용의 메시지를 수신해야 한다
And 각 메시지는 독립적인 인스턴스여야 한다 (메모리 주소 다름)
```

### AC-ENGINE-001-31: Fan-in 메시지 합류

```gherkin
Given 노드 B와 C의 출력이 노드 D의 입력에 연결되어 있을 때
When B가 메시지 "b1"을, C가 메시지 "c1"을 전송하면
Then D가 "b1"과 "c1"을 모두 수신해야 한다
And 도착 순서대로 처리되어야 한다
```

### AC-ENGINE-001-32: 닫힌 채널 전송 방지

```gherkin
Given RuntimeWire의 채널이 닫힌 상태일 때
When 해당 Wire에 메시지를 전송 시도하면
Then panic이 발생하지 않아야 한다
And ErrChannelClosed 에러가 반환되거나, 에러 로그가 기록되어야 한다
```

---

## Module 4: Backpressure - 백프레셔 제어

### AC-ENGINE-001-33: DefaultBackpressurePolicy 기본값

```gherkin
Given DefaultBackpressurePolicy()를 호출할 때
Then Strategy가 StrategyBlock이어야 한다
And BufferHighWaterMark가 0.8이어야 한다
And DropPolicy가 DropNewest여야 한다
```

### AC-ENGINE-001-34: Block 전략 자연적 백프레셔

```gherkin
Given 버퍼 크기 5인 Wire에 StrategyBlock 정책이 적용되어 있을 때
When 수신 노드가 메시지를 소비하지 않고, 송신 노드가 메시지 6개를 전송하면
Then 6번째 메시지 전송 시 송신 노드가 블로킹되어야 한다
And 수신 노드가 메시지 1개를 소비하면 6번째 메시지가 전달되어야 한다
```

### AC-ENGINE-001-35: Drop 전략 - DropNewest

```gherkin
Given 버퍼 크기 5인 Wire에 StrategyDrop + DropNewest 정책이 적용되어 있을 때
When 버퍼가 가득 찬 상태에서 새 메시지를 전송하면
Then 새 메시지가 폐기되어야 한다
And 버퍼 내 기존 메시지는 유지되어야 한다
And 드롭 메트릭이 1 증가해야 한다
```

### AC-ENGINE-001-36: Drop 전략 - DropOldest

```gherkin
Given 버퍼 크기 5인 Wire에 StrategyDrop + DropOldest 정책이 적용되어 있을 때
When 버퍼가 가득 찬 상태에서 새 메시지를 전송하면
Then 버퍼에서 가장 오래된 메시지가 폐기되어야 한다
And 새 메시지가 버퍼에 추가되어야 한다
And 드롭 메트릭이 1 증가해야 한다
```

### AC-ENGINE-001-37: 고수위 마크 경고

```gherkin
Given BufferHighWaterMark가 0.8이고, 버퍼 크기가 10인 Wire가 있을 때
When 버퍼에 8개째 메시지가 추가되면 (사용률 80%)
Then 경고 로그가 생성되어야 한다
And 백프레셔 메트릭이 갱신되어야 한다
```

---

## Module 5: State Management - 플로우 상태 관리

### AC-ENGINE-001-38: FlowState -> lifecycle.State 매핑

```gherkin
Given FlowLoaded 상태가 주어졌을 때
When MapFlowStateToLifecycleState(FlowLoaded)를 호출하면
Then StateCreated를 반환해야 한다

Given FlowRunning 상태가 주어졌을 때
When MapFlowStateToLifecycleState(FlowRunning)를 호출하면
Then StateRunning을 반환해야 한다

Given FlowPaused 상태가 주어졌을 때
When MapFlowStateToLifecycleState(FlowPaused)를 호출하면
Then StatePaused를 반환해야 한다

Given FlowStored 상태가 주어졌을 때
When MapFlowStateToLifecycleState(FlowStored)를 호출하면
Then ErrStateMapping 에러를 반환해야 한다 (lifecycle 대응 없음)
```

### AC-ENGINE-001-39: lifecycle.State -> FlowState 역매핑

```gherkin
Given StateCreated 상태가 주어졌을 때
When MapLifecycleStateToFlowState(StateCreated)를 호출하면
Then FlowLoaded를 반환해야 한다

Given StateRunning 상태가 주어졌을 때
When MapLifecycleStateToFlowState(StateRunning)를 호출하면
Then FlowRunning을 반환해야 한다
```

### AC-ENGINE-001-40: 양방향 매핑 일관성

```gherkin
Given FlowInitializing, FlowRunning, FlowPaused, FlowStopping, FlowStopped, FlowError 각각에 대해
When MapFlowStateToLifecycleState() 후 MapLifecycleStateToFlowState()를 호출하면
Then 원래 FlowState로 복원되어야 한다 (왕복 일관성)
```

### AC-ENGINE-001-41: Pause 전파

```gherkin
Given 3개 노드(A -> B -> C)로 구성된 Running Flow가 있을 때
When Engine.PauseFlow(ctx, flowID)를 호출하면
Then 3개 노드 모두가 Paused 상태여야 한다
And 채널은 열린 상태로 유지되어야 한다
And PauseFlow 후 Wire에 메시지를 보내면 버퍼/블로킹 동작이 수행되어야 한다
```

### AC-ENGINE-001-42: Resume 후 큐 데이터 처리

```gherkin
Given Flow가 Paused 상태이고, Wire 채널에 메시지 ["m1", "m2", "m3"]이 큐잉되어 있을 때
When Engine.ResumeFlow(ctx, flowID)를 호출하면
Then 노드가 "m1", "m2", "m3" 순서대로 처리해야 한다
And 큐 처리 완료 후 새 메시지도 정상 처리되어야 한다
```

---

## Module 6: TTL Management - 메시지 TTL 관리

### AC-ENGINE-001-43: TTL 만료 메시지 감지

```gherkin
Given TTL이 100ms인 버퍼 Wire에 메시지가 있을 때
When 150ms가 경과한 후 해당 메시지를 전달하려 하면
Then 메시지가 만료된 것으로 판정되어야 한다
And 만료 메트릭(드롭 카운터)이 증가해야 한다
```

### AC-ENGINE-001-44: Dead Letter 노드 라우팅

```gherkin
Given TTL 만료 메시지가 발견되고, Dead Letter 노드가 연결되어 있을 때
Then 만료 사유와 함께 Dead Letter 노드로 전달되어야 한다

Given TTL 만료 메시지가 발견되고, Dead Letter 노드가 없을 때
Then 메시지가 자동 폐기되어야 한다
And 로그에 폐기 사유가 기록되어야 한다
```

### AC-ENGINE-001-45: 주기적 버퍼 스캔

```gherkin
Given TTLScanner가 1초 주기로 실행되고, 버퍼에 TTL 만료 메시지 3개가 있을 때
When 스캔 주기가 도래하면
Then 3개의 만료 메시지가 모두 제거되어야 한다
And 각 메시지에 대해 Dead Letter 처리 또는 폐기가 수행되어야 한다
```

### AC-ENGINE-001-46: 바이패스 모드 TTL 검사 생략

```gherkin
Given 바이패스 모드(unbuffered) Wire에 TTL이 설정되어 있을 때
When 메시지를 전달하면
Then TTL 검사를 수행하지 않아야 한다 (즉시 전달)
```

### AC-ENGINE-001-47: TTLScanner 생명주기

```gherkin
Given TTLScanner가 Start(ctx)로 시작되었을 때
When ctx가 취소되면
Then 스캔 goroutine이 종료되어야 한다 (goroutine 누수 없음)

Given TTLScanner.Stop()을 호출할 때
Then 스캔 goroutine이 종료되어야 한다
```

---

## Module 7: Error Types - 에러 타입

### AC-ENGINE-001-48: Sentinel 에러 정의

```gherkin
Given errors 패키지가 임포트되어 있을 때
Then ErrFlowAlreadyDeployed가 정의되어 있어야 한다
And ErrFlowNotFound가 정의되어 있어야 한다
And ErrFlowNotRunning이 정의되어 있어야 한다
And ErrFlowNotPaused가 정의되어 있어야 한다
And ErrFlowNotStopped가 정의되어 있어야 한다
And ErrFlowNotLoaded가 정의되어 있어야 한다
And ErrFlowValidationFailed가 정의되어 있어야 한다
And ErrCycleDetected가 정의되어 있어야 한다
And ErrStateMapping이 정의되어 있어야 한다
And ErrChannelClosed가 정의되어 있어야 한다
And ErrNodeStartFailed가 정의되어 있어야 한다
And ErrShutdownTimeout이 정의되어 있어야 한다
```

### AC-ENGINE-001-49: errors.Is() 호환성

```gherkin
Given ErrFlowNotFound 에러가 주어졌을 때
When fmt.Errorf("wrap: %w", ErrFlowNotFound)로 래핑한 후
Then errors.Is(wrappedErr, ErrFlowNotFound)이 true를 반환해야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-ENGINE-001-01 ~ 49) 테스트 통과
- [ ] `go test ./internal/engine/...` 전체 통과
- [ ] `go test -race ./internal/engine/...` 경쟁 상태 없음
- [ ] `go vet ./internal/engine/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (모든 exported 타입/함수/메서드)
- [ ] 성능 목표 검증:
  - [ ] 메시지 처리 지연시간(P95): < 10ms per node
  - [ ] 동시 플로우: 100+ 실행 가능
  - [ ] 메시지 처리량: 10,000+ msg/s
  - [ ] 유휴 메모리: < 256MB
- [ ] goroutine 누수 없음 (Start/Stop 사이클 후 goroutine 수 원복 확인)
- [ ] Graceful Shutdown 시 메시지 유실 없음 확인
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
