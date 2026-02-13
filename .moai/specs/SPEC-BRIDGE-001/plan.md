---
id: SPEC-BRIDGE-001
type: plan
version: "1.0.0"
spec_ref: SPEC-BRIDGE-001
---

# SPEC-BRIDGE-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반)
  - 신규 파일 (bridge_transform.go, bridge_correlation.go, bridge_config.go, bridge_info.go, bridge_errors.go): TDD (RED-GREEN-REFACTOR)
  - 기존 파일 수정 (bridge.go): DDD (ANALYZE-PRESERVE-IMPROVE) - SPEC-NODE-001에서 기본 구조 정의됨
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**:
  - `sync.Map` (CorrelationTracker 내부 매핑)
  - `sync.RWMutex` (BridgeNode Agent 바인딩 상태 보호)
  - `sync/atomic` (BridgeStats 카운터)
- **UUID**: `github.com/google/uuid` (Correlation ID 생성)
- **컨텍스트**: `context.Context` (수신 루프, 정리 루프 취소 전파)
- **의존 패키지**:
  - `internal/node/` (SPEC-NODE-001): BaseNode, Node, NodeOption, Registry
  - `internal/agent/` (SPEC-AGENT-001): Agent, Registry, SharedRef
  - `pkg/message/` (SPEC-MSG-001): Message, Payload, Metadata
  - `pkg/flow/` (SPEC-FLOW-001): NodeDef, AgentRef, BridgeDirection
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent
  - `pkg/lifecycle/` (SPEC-LIFE-001): State
  - `internal/script/` (별도 SPEC): Lua VM (LuaTransformer용)

### 1.3 패키지 위치

- **경로**: `internal/node/` (bridge_*.go 파일군)
- **Tier**: internal (비공개 패키지)
- **소비자**: `internal/engine/` (Engine이 BridgeNode를 goroutine으로 실행)

---

## 2. 마일스톤

### Primary Goal: Error Types + BridgeConfig + BridgeNode Core 골격 (P0)

**범위**: Module 8, 6, 1 (기반)

**작업 항목**:

1. `bridge_errors.go` + `bridge_errors_test.go` 작성
   - 9개 sentinel error 변수 정의
   - `errors.Is()` 호환성 테스트
   - `fmt.Errorf("%w", ...)` 래핑/언래핑 테스트

2. `bridge_config.go` + `bridge_config_test.go` 작성
   - `BridgeConfig` 구조체 정의
   - `TransformConfig` 구조체 정의
   - `BridgeConfig.Validate()` 메서드 구현
   - 유효한 설정 검증 테스트
   - AgentRef 누락 시 에러 테스트
   - 유효하지 않은 Direction 에러 테스트
   - Request-Reply 모드 RequestTimeout 검증 테스트
   - Transform.Mode "lua"에서 LuaScript 누락 시 에러 테스트
   - BufferSize 범위 검증 테스트

3. `bridge.go` 확장 (BridgeNode 구조체 골격)
   - SPEC-NODE-001의 BridgeNode 기본 구조에 새 필드 추가
   - `NewBridgeNode()` 생성자에 BridgeConfig 초기화 및 검증 로직 추가
   - Agent 바인딩 없이 설정 기반 초기 구조 테스트

**산출물**: Bridge 에러 타입, 설정 구조체, BridgeNode 골격 완성

---

### Secondary Goal: Agent Binding + Communication Modes (P0)

**범위**: Module 2, 3

**작업 항목**:

1. Agent 바인딩 구현 (bridge.go 확장)
   - Agent 이름 기반 해상도 (Agent Registry 조회)
   - Agent ID 기반 해상도
   - SharedRef.Acquire/Release 연동
   - Agent 미발견 시 ErrAgentNotFound 반환 테스트
   - Agent Not Running 시 ErrAgentNotRunning 반환 테스트
   - SharedRef 참조 카운트 증가/감소 테스트
   - 멀티 브릿지 동시 연결 테스트

2. BridgeIn 모드 구현
   - 수신 루프 goroutine 구현
   - Agent.Process() 폴링 및 Message 변환
   - 수신 버퍼 채널 관리
   - 수신 데이터 -> 출력 포트 전달 테스트
   - 컨텍스트 취소 시 goroutine 정상 종료 테스트

3. BridgeOut 모드 구현
   - Process()에서 입력 메시지 -> Agent 전달
   - Message -> Agent 프로토콜 데이터 변환
   - 송신 성공/실패 테스트

4. BridgeInOut 모드 구현
   - 수신 루프 + Process 송신 동시 동작
   - 양방향 메시지 전달 테스트

5. BridgeRequestReply 모드 기본 구현
   - Correlation ID 생성 및 Metadata 설정
   - 요청 -> Agent 전달
   - 응답 수신 및 반환
   - 기본 타임아웃 처리 테스트

6. Agent 상태 변경 감지 구현
   - Running -> Paused 시 수신/송신 일시 중지
   - Paused -> Running 시 재개
   - Stopped 시 에러 포트 전달 및 재바인딩 시도

7. Agent 재바인딩 구현
   - ReconnectInterval 간격으로 재시도
   - MaxReconnectAttempts 초과 시 중단
   - 재바인딩 성공/실패 테스트

8. Init/Shutdown 완성
   - Init: Agent 해상도, SharedRef.Acquire, 수신 루프 시작
   - Shutdown: 수신 루프 중지, SharedRef.Release, Agent 해제
   - goroutine 누수 없음 검증 테스트

**산출물**: Agent 연결 관리 및 4개 통신 모드 기본 구현 완성

---

### Tertiary Goal: Message Transformation + Correlation Tracker (P1)

**범위**: Module 4, 5

**작업 항목**:

1. `bridge_transform.go` + `bridge_transform_test.go` 작성
   - `BridgeTransformer` 인터페이스 정의
   - `DefaultTransformer` 구현
     - AgentToFlow: []byte -> Message (Payload `_raw` 키)
     - FlowToAgent: Message -> []byte (Payload `_raw` 키 추출)
     - 양방향 변환 왕복 테스트
     - `_raw` 키 누락 시 에러 테스트
   - `LuaTransformer` 구현 (Optional)
     - Lua VM 참조 초기화
     - AgentToFlow: Lua 스크립트 실행으로 변환
     - FlowToAgent: Lua 스크립트 실행으로 변환
     - 스크립트 로드 실패 테스트
     - 스크립트 실행 에러 격리 테스트
   - 변환 에러 시 ErrTransformFailed 반환 테스트
   - 변환기 Hot Reload 테스트

2. `bridge_correlation.go` + `bridge_correlation_test.go` 작성
   - `CorrelationTracker` 구조체 구현
   - `NewCorrelationTracker(timeout)` 생성자
   - `Track(correlationID, responseCh)` 구현
   - `Resolve(correlationID, response)` 구현
     - 성공 매칭 시 responseCh에 응답 전달 테스트
     - 미등록 ID 시 ErrCorrelationNotFound 반환 테스트
   - `Cleanup()` 만료 항목 정리 구현
     - 타임아웃 경과 항목 정리 테스트
   - `PendingCount()` 구현
   - `Close()` 전체 정리 구현
   - `StartCleanupLoop(ctx)` 주기적 정리 goroutine
   - sync.Map 동시성 안전 테스트 (`go test -race`)
   - 동시 다중 Track/Resolve 테스트
   - Close 후 모든 항목 정리 확인 테스트

**산출물**: 메시지 변환 레이어 및 Correlation ID 관리 시스템 완성

---

### Final Goal: Bridge Info & Stats + 통합 테스트 (P1+)

**범위**: Module 7, 통합

**작업 항목**:

1. `bridge_info.go` + `bridge_info_test.go` 작성
   - `BridgeInfo` 구조체 정의
   - `BridgeStats` 구조체 정의
   - `Info()` 메서드 구현 (Agent 바인딩 정보 스냅샷)
   - `Stats()` 메서드 구현 (atomic 카운터 읽기)
   - Info() 연결 상태 반영 테스트
   - Stats() 메트릭 정확성 테스트
   - atomic 카운터 동시 업데이트 테스트 (`go test -race`)

2. 통합 테스트 작성
   - BridgeIn: Mock Agent -> BridgeNode -> 출력 메시지 검증
   - BridgeOut: 입력 메시지 -> BridgeNode -> Mock Agent 수신 검증
   - BridgeInOut: 양방향 메시지 교환 검증
   - BridgeRequestReply: 요청-응답 Correlation 매칭 검증
   - Agent 재바인딩: Agent 중단 -> 재시작 -> Bridge 재연결 검증
   - 수신 버퍼 초과: ErrBufferFull 에러 포트 전달 검증
   - Correlation 타임아웃: 응답 지연 시 타임아웃 처리 검증
   - 멀티 브릿지: 동일 Agent에 2개 Bridge 동시 연결 검증

3. 벤치마크 테스트
   - Bridge Process 단일 호출 지연시간 (BridgeOut 모드, 목표: < 1ms)
   - DefaultTransformer 양방향 변환 지연시간 (목표: < 100us)
   - CorrelationTracker Track/Resolve 지연시간 (목표: < 50us)
   - 동시 다중 Request-Reply 처리량 (목표: 10,000 req/s)

**산출물**: Bridge 상태/통계 조회 및 전체 통합 검증 완성

---

## 3. 기술적 접근

### 3.1 BridgeNode 아키텍처

```
BridgeNode
├── *BaseNode                     // SPEC-NODE-001 임베딩
├── agent agent.Agent             // 바인딩된 Agent
├── sharedRef *agent.SharedRef    // 참조 카운팅
├── config BridgeConfig           // Bridge 설정
├── transformer BridgeTransformer // 변환기 (인터페이스)
│   ├── DefaultTransformer        // 기본: []byte <-> Message._raw
│   └── LuaTransformer            // Lua: 스크립트 기반 변환
├── correlation *CorrelationTracker // Request-Reply 전용
├── stats BridgeStats             // 처리 통계 (atomic)
├── recvCh chan message.Message   // 수신 버퍼 채널
├── cancelFn context.CancelFunc   // goroutine 취소
├── mu sync.RWMutex               // Agent 바인딩 보호
└── flowID string                 // Flow ID (SharedRef용)
```

### 3.2 수신 루프 전략 (BridgeIn / BridgeInOut)

```
go func(ctx context.Context) {
    for {
        select {
        case <-ctx.Done():
            return
        default:
            data, err := agent.Process(nil) // Agent에서 수신 데이터 폴링
            if err != nil {
                // 에러 포트 전달
                continue
            }
            if data == nil {
                time.Sleep(10 * time.Millisecond) // 폴링 간격
                continue
            }
            msg, err := transformer.AgentToFlow(data)
            if err != nil {
                // ErrTransformFailed 에러 포트 전달
                continue
            }
            select {
            case recvCh <- msg:
                atomic.AddInt64(&stats.MessagesFromAgent, 1)
            default:
                // ErrBufferFull 에러 포트 전달
            }
        }
    }
}(ctx)
```

### 3.3 Request-Reply Correlation 전략

```
Process(ctx, msg) [BridgeRequestReply 모드]:
  1. correlationID := uuid.New().String()
  2. msg.Metadata().Set("_correlationID", correlationID)
  3. responseCh := make(chan message.Message, 1)
  4. tracker.Track(correlationID, responseCh)
  5. data, err := transformer.FlowToAgent(msg)
  6. agent.Process(data) // Agent에 요청 전달
  7. select {
       case resp := <-responseCh:
           return [resp], nil
       case <-time.After(config.RequestTimeout):
           tracker.entries.Delete(correlationID)
           return nil, ErrRequestTimeout
       case <-ctx.Done():
           tracker.entries.Delete(correlationID)
           return nil, ctx.Err()
     }
```

### 3.4 Agent 재바인딩 전략

```
go func(ctx context.Context) {
    for attempt := 0; attempt < config.MaxReconnectAttempts; attempt++ {
        select {
        case <-ctx.Done():
            return
        case <-time.After(config.ReconnectInterval):
            newAgent := agentRegistry.Get(config.AgentRef.Name)
            if newAgent != nil && newAgent.Health().State == lifecycle.Running {
                mu.Lock()
                agent = newAgent
                sharedRef = newAgent.SharedRef()
                sharedRef.Acquire(flowID)
                mu.Unlock()
                // 수신 루프 재시작
                return
            }
        }
    }
    // 모든 시도 실패
    // ErrMaxReconnectExceeded 에러 포트 전달
}(ctx)
```

### 3.5 CorrelationTracker 정리 루프

```
func (t *CorrelationTracker) StartCleanupLoop(ctx context.Context) {
    ticker := time.NewTicker(t.timeout / 2)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            t.Cleanup() // 만료 항목 정리
        }
    }
}

func (t *CorrelationTracker) Cleanup() {
    now := time.Now()
    t.entries.Range(func(key, value any) bool {
        entry := value.(*correlationEntry)
        if now.Sub(entry.createdAt) > entry.timeout {
            close(entry.responseCh)
            t.entries.Delete(key)
            t.timeoutCnt.Add(1)
        }
        return true
    })
}
```

### 3.6 Mock Agent 전략

의존 패키지(SPEC-AGENT-001)가 미완성일 경우, Agent 인터페이스를 local interface로 재정의하여 mock 기반 테스트를 수행한다:

```go
// bridge_test.go
type mockAgent struct {
    id     string
    name   string
    health agent.HealthStatus
    data   []byte
}

func (m *mockAgent) ID() string          { return m.id }
func (m *mockAgent) Name() string        { return m.name }
func (m *mockAgent) Health() agent.HealthStatus { return m.health }
func (m *mockAgent) Process(data []byte) ([]byte, error) { return m.data, nil }
```

---

## 4. 리스크 및 대응

### Risk 1: Agent 패키지 미완성

- **위험**: `internal/agent/` 패키지의 Agent 인터페이스, SharedRef가 구현되지 않은 상태에서 Bridge 개발
- **대응**: Agent 인터페이스를 local interface로 정의하여 mock 기반 테스트. Agent 패키지 완성 후 통합. SharedRef도 mock 구현으로 테스트

### Risk 2: Correlation ID 누수

- **위험**: 타임아웃 처리 실패 또는 goroutine 종료 문제로 sync.Map에 Correlation 항목이 무한 누적
- **대응**: 이중 안전장치 적용
  1. 요청별 타임아웃 (select + time.After)
  2. 주기적 Cleanup goroutine (timeout / 2 간격)
  3. Shutdown 시 Close()로 전체 정리
  4. PendingCount() 메트릭으로 누수 감지

### Risk 3: 수신 루프 goroutine 누수

- **위험**: BridgeIn/BridgeInOut 모드의 수신 루프 goroutine이 Shutdown 후에도 잔존
- **대응**: context.WithCancel으로 생성하고, Shutdown에서 cancelFn() 호출. 테스트에서 runtime.NumGoroutine()으로 goroutine 수 원복 확인

### Risk 4: Agent 상태 변경 경쟁 상태

- **위험**: Agent 상태 변경 감지와 수신/송신 동작 간 race condition
- **대응**: sync.RWMutex로 Agent 바인딩 상태 보호. 수신 루프에서 Agent 접근 시 RLock 사용. 상태 변경 처리 시 WLock 사용. `go test -race` 필수

### Risk 5: DefaultTransformer 데이터 손실

- **위험**: Agent의 프로토콜 데이터가 복잡한 구조인 경우, 단순 []byte 매핑으로 정보 손실
- **대응**: DefaultTransformer는 패스스루 용도로 설계. 복잡한 변환이 필요한 경우 LuaTransformer 사용을 권장. 문서에 명확히 기술

### Risk 6: Lua VM 안정성 (LuaTransformer)

- **위험**: Lua 스크립트의 메모리 누수, 무한 루프, panic으로 Bridge 전체 중단
- **대응**: Script Node와 동일한 전략 적용 - recover() 래퍼, 타임아웃 제한, 샌드박스. LuaTransformer는 Optional로 분류하여 핵심 기능에 영향 없음

### Risk 7: 의존 SPEC 미완성

- **위험**: SPEC-NODE-001의 BaseNode, SPEC-FLOW-001의 AgentRef/BridgeDirection이 미완성
- **대응**: 인터페이스 기반 의존으로 mock 테스트 가능. 의존 패키지의 타입을 local 정의로 대체하여 독립 테스트

---

## 5. 의존성 그래프

```
internal/node/bridge_*.go (본 SPEC)
  ├── 동일 패키지: internal/node/base.go    (SPEC-NODE-001: BaseNode, Node, NodeOption)
  ├── 의존: internal/agent/                  (SPEC-AGENT-001: Agent, Registry, SharedRef)
  ├── 의존: pkg/flow/                        (SPEC-FLOW-001: NodeDef, AgentRef, BridgeDirection)
  ├── 의존: pkg/message/                     (SPEC-MSG-001: Message, Payload, Metadata)
  ├── 의존: pkg/xferr/                       (SPEC-ERR-001: ErrorMessage, StatusEvent)
  ├── 의존: pkg/lifecycle/                   (SPEC-LIFE-001: State)
  ├── 의존(Optional): internal/script/       (별도 SPEC: Lua VM - LuaTransformer)
  ├── 의존: github.com/google/uuid          (Correlation ID 생성)
  ├── 의존: 표준 라이브러리                    (context, sync, sync/atomic, time, errors, fmt)
  ├── 소비자: internal/engine/               (SPEC-ENGINE-001: Engine이 BridgeNode를 실행)
  └── 소비자: internal/api/                  (REST API에서 Bridge 정보 조회)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | bridge_errors.go | Sentinel 에러 9개 정의 | 없음 |
| 2 | bridge_config.go | BridgeConfig, TransformConfig, Validate() | bridge_errors.go, pkg/flow/ |
| 3 | bridge_transform.go | BridgeTransformer 인터페이스, DefaultTransformer | pkg/message/ |
| 4 | bridge_correlation.go | CorrelationTracker 구조체 | pkg/message/, bridge_errors.go |
| 5 | bridge_info.go | BridgeInfo, BridgeStats 구조체 | pkg/flow/ |
| 6 | bridge.go (확장) | BridgeNode 상세 구현 (바인딩, 4모드, Init, Shutdown) | 1~5 전체, internal/agent/ |

모든 파일에 대해 Hybrid 방식(신규 TDD, 수정 DDD)으로 테스트 파일(`*_test.go`)을 작성한다.
