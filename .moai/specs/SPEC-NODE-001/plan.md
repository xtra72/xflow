---
id: SPEC-NODE-001
type: plan
version: "1.0.0"
spec_ref: SPEC-NODE-001
---

# SPEC-NODE-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 모든 파일이 신규 생성이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: `sync.Mutex` (노드 내부 상태 보호), `sync.RWMutex` (Registry), `sync.Map` (Bridge Correlation)
- **컨텍스트**: `context.Context` (취소, 타임아웃 전파)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): BaseLifecycle, Configurable, State
  - `pkg/flow/` (SPEC-FLOW-001): NodeDef, Port, PortDirection, AgentRef, BridgeDirection
  - `pkg/message/` (SPEC-MSG-001): Message, Payload, Metadata
  - `internal/observe/` (SPEC-OBS-001): Logger, Metrics 인터페이스
  - `internal/config/` (SPEC-CFG-001): Config, HotReload 인터페이스
  - `internal/script/` (별도 SPEC): Lua VM 인터페이스

### 1.3 패키지 위치

- **경로**: `internal/node/`
- **Tier**: internal (비공개 패키지)
- **소비자**: `internal/engine/` (Engine이 Node 인터페이스 소비), `internal/api/` (REST API)

---

## 2. 마일스톤

### Primary Goal: Error Types + Node Interface & BaseNode + Port System + Registry (P0)

**범위**: Module 14, 1, 3, 2

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 12개 sentinel error 변수 정의
   - `NodeError` 구조체 (`Error()`, `Unwrap()` 메서드)
   - `errors.Is()` 호환성 테스트
   - `NodeError` 래핑/언래핑 테스트

2. `base.go` + `base_test.go` 작성 (Node Interface & BaseNode & Port System)
   - `Node` 인터페이스 정의
   - `PortDirection` 타입 및 상수 정의
   - `Port` 구조체 정의
   - `BaseNode` 구조체 (`BaseLifecycle` 임베딩)
   - `NewBaseNode(def, opts...)` 생성자
   - `NodeOption` 타입 및 `WithNodeLogger()`, `WithNodeMetrics()` 옵션
   - `ID()`, `Name()`, `Type()` 메서드
   - `Configure()`, `GetConfig()` 메서드 (mutex 보호)
   - `Ports()` 메서드
   - 기본 에러 포트 자동 생성 테스트
   - 포트 바인딩 테스트 (flow.NodeDef 기반)
   - Hot Configuration 테스트 (유효/무효 설정)
   - Options Pattern 테스트

3. `registry.go` + `registry_test.go` 작성
   - `NodeFactory` 타입 정의
   - `Registry` 구조체 (`sync.RWMutex` 보호)
   - `NewRegistry()` 생성자 (내장 타입 자동 등록)
   - `Register()` 메서드 (중복 등록 거부 테스트)
   - `Create()` 메서드 (미등록 타입 거부 테스트)
   - `Types()` 메서드
   - `Has()` 메서드
   - `WithoutBuiltins()` 옵션 테스트
   - 동시 등록/생성 race condition 테스트

**산출물**: Node 시스템의 기반 인프라 완성 (인터페이스, 기반 구현체, 포트, 레지스트리)

---

### Secondary Goal: Filter + Transform + Switch + Catch Node (P1 기본)

**범위**: Module 4, 5, 6, 11

**작업 항목**:

1. `filter.go` + `filter_test.go` 작성
   - `FilterNode` 구조체 (`BaseNode` 임베딩)
   - `FilterCondition` 함수 타입
   - `NewFilterNode()` 팩토리 함수
   - `Init()`, `Process()`, `Shutdown()` 구현
   - 조건 통과/미통과 메시지 테스트
   - Configure로 조건 변경 테스트
   - 에러 발생 시 에러 포트 전달 테스트

2. `transform.go` + `transform_test.go` 작성
   - `TransformNode` 구조체, `TransformRule` 구조체
   - `NewTransformNode()` 팩토리 함수
   - `Init()`, `Process()`, `Shutdown()` 구현
   - Payload 변환 성공 테스트
   - 변환 실패 시 에러 포트 전달 테스트
   - 다중 규칙 순차 적용 테스트

3. `switch.go` + `switch_test.go` 작성
   - `SwitchNode` 구조체, `SwitchRoute` 구조체
   - `NewSwitchNode()` 팩토리 함수
   - `Init()`, `Process()`, `Shutdown()` 구현
   - First-match 라우팅 테스트
   - 기본 라우트 테스트
   - 매칭 실패 시 폐기 테스트
   - 다중 출력 포트 테스트

4. `catch.go` + `catch_test.go` 작성
   - `CatchNode` 구조체
   - `NewCatchNode()` 팩토리 함수
   - `Init()`, `Process()`, `Shutdown()` 구현
   - 에러 메시지 수신 및 가공 테스트
   - 에러 패턴 필터링 테스트 (catch_types 설정)
   - 전체 에러 캐치 테스트 (설정 없음)

**산출물**: 기본 데이터 처리 노드 및 에러 처리 노드 완성

---

### Tertiary Goal: Bridge + Script Node (P1 고급)

**범위**: Module 8, 9

**작업 항목**:

1. `bridge.go` + `bridge_test.go` 작성
   - `BridgeNode` 구조체
   - `NewBridgeNode()` 팩토리 함수
   - `Init()` 구현 (Agent 연결 설정, AgentRef 검증)
   - BridgeIn 모드: Agent -> Flow 메시지 전달 테스트
   - BridgeOut 모드: Flow -> Agent 메시지 전달 테스트
   - BridgeInOut 모드: 양방향 전달 테스트
   - BridgeRequestReply 모드:
     - Correlation ID 생성 및 매칭 테스트
     - sync.Map 기반 응답 대기 테스트
     - 타임아웃 처리 테스트 (기본 30초, 설정 변경)
     - 동시 다중 요청 테스트
   - `Shutdown()` 구현 (Agent 연결 해제, 대기 Correlation 정리)
   - Agent 미발견 시 에러 테스트

2. `script.go` + `script_test.go` 작성
   - `ScriptNode` 구조체
   - `NewScriptNode()` 팩토리 함수
   - `Init()` 구현 (Lua VM 생성, 스크립트 컴파일, 샌드박스)
   - `Process()` 구현 (Payload -> Lua 테이블 -> Payload 변환)
   - 스크립트 실행 성공 테스트
   - 스크립트 컴파일 실패 테스트
   - 스크립트 런타임 에러 격리 테스트 (노드 생존)
   - 스크립트 타임아웃 테스트 (기본 5초)
   - Hot Reload 테스트 (Configure로 스크립트 교체)
   - `Shutdown()` 구현 (Lua VM 종료)

**산출물**: Agent-Flow 연결 및 동적 스크립트 실행 기능 완성

---

### Final Goal: Aggregate + Debug + Status + Dead Letter Node (P2)

**범위**: Module 7, 10, 12, 13

**작업 항목**:

1. `aggregate.go` + `aggregate_test.go` 작성
   - `AggregateNode` 구조체, `WindowType`/`AggregateFn` 타입
   - `NewAggregateNode()` 팩토리 함수
   - 카운트 기반 윈도우 테스트 (5개 모인 후 집계)
   - 시간 기반 윈도우 테스트 (1초 간격 집계)
   - 집계 함수별 테스트 (sum, avg, count, min, max, first, last)
   - 종료 시 잔여 버퍼 플러시 테스트
   - 유효하지 않은 윈도우 설정 거부 테스트

2. `debug.go` + `debug_test.go` 작성
   - `DebugNode` 구조체
   - `NewDebugNode()` 팩토리 함수
   - 메시지 로깅 + pass-through 테스트
   - 로그 레벨 설정 테스트 (debug/info/warn)

3. `status.go` + `status_test.go` 작성
   - `StatusNode` 구조체
   - `NewStatusNode()` 팩토리 함수
   - 상태 변경 이벤트 수신 테스트
   - 모니터링 대상 설정 테스트 (특정 노드/전체)
   - 이벤트 -> 메시지 변환 테스트

4. `deadletter.go` + `deadletter_test.go` 작성
   - `DeadLetterNode` 구조체, `DeadLetterStrategy` 타입
   - `NewDeadLetterNode()` 팩토리 함수
   - 사유별 메시지 수신 테스트 (ttl_expired, undeliverable, max_retries)
   - 사유별 메트릭 카운터 테스트
   - 처리 전략 설정 테스트 (log/store/forward)

**산출물**: 고급 데이터 처리 및 운영 모니터링 노드 완성

---

### Optional Goal: 통합 테스트 및 벤치마크 (P2+)

**범위**: 전체 통합, 성능 검증

**작업 항목**:

1. 통합 테스트 작성
   - Registry에서 모든 내장 타입 생성 확인
   - Filter -> Transform -> Switch 파이프라인 테스트
   - Bridge Node <-> Mock Agent 통합 테스트
   - Catch Node <-> 에러 포트 연결 통합 테스트
   - Dead Letter Node 수신 통합 테스트

2. 벤치마크 테스트
   - 노드 Process 단일 호출 지연시간 벤치마크 (목표: < 1ms)
   - Registry.Create 팩토리 호출 벤치마크
   - Bridge RequestReply Correlation 매칭 벤치마크
   - AggregateNode 대량 메시지 버퍼링 벤치마크

**산출물**: 전체 통합 검증 및 성능 목표 달성

---

## 3. 기술적 접근

### 3.1 Node 아키텍처

```
Node Interface (contract)
├── ID(), Name(), Type()         // 식별
├── Init(ctx)                    // 초기화 (리소스 할당)
├── Process(ctx, msg)            // 메시지 처리 (핵심 로직)
├── Shutdown(ctx)                // 종료 (리소스 해제)
├── Configure(config)            // Hot Configuration
└── Ports()                      // 포트 목록

BaseNode (공통 기반 구현체)
├── lifecycle.BaseLifecycle      // 임베딩 (상태 전이 자동 관리)
├── id, name, nodeType           // 식별 정보
├── config (sync.Mutex 보호)     // 런타임 설정
├── inputs, outputs, errorPort   // 포트 맵
└── logger, metrics              // 관찰성

ConcreteNode (타입별 구현체)
├── *BaseNode                    // 임베딩
├── 타입별 필드                   // 예: FilterCondition, TransformRule
├── Init()                       // 타입별 초기화 오버라이드
├── Process()                    // 타입별 처리 로직
└── Shutdown()                   // 타입별 정리 오버라이드
```

### 3.2 Registry 팩토리 패턴

```
NewRegistry()
  └── 내장 타입 자동 등록:
      ├── "filter"      -> NewFilterNode
      ├── "transform"   -> NewTransformNode
      ├── "switch"      -> NewSwitchNode
      ├── "aggregate"   -> NewAggregateNode
      ├── "bridge"      -> NewBridgeNode
      ├── "script"      -> NewScriptNode
      ├── "debug"       -> NewDebugNode
      ├── "catch"       -> NewCatchNode
      ├── "status"      -> NewStatusNode
      └── "deadletter"  -> NewDeadLetterNode

Engine.DeployFlow():
  for _, nodeDef := range flow.Nodes() {
      node, err := registry.Create(nodeDef, opts...)
      // node는 Node 인터페이스
  }
```

### 3.3 에러 출력 포트 전략

모든 노드의 Process() 메서드에서 에러 발생 시:

1. BaseNode의 에러 포트 참조 확인
2. 에러 메시지 생성 (원본 메시지 ID + 에러 정보 + 노드 ID를 Metadata에 첨부)
3. 에러 메시지를 반환 결과에 에러 포트 대상으로 포함
4. Engine이 반환된 에러 메시지를 에러 포트 Wire로 전달
5. Catch Node가 에러 포트 Wire를 통해 에러 메시지를 수신

### 3.4 Bridge Node Request-Reply 전략

```
1. Agent 요청 수신
2. correlationID := uuid.New()
3. replyCh := make(chan message.Message, 1)
4. correlations.Store(correlationID, replyCh)
5. msg.Metadata().Set("_correlationID", correlationID)
6. 요청 메시지를 Flow 입력 포트로 전달
7. select {
   case reply := <-replyCh:
       correlations.Delete(correlationID)
       return reply (Agent로 응답)
   case <-time.After(replyTimeout):
       correlations.Delete(correlationID)
       return ErrRequestTimeout
   case <-ctx.Done():
       correlations.Delete(correlationID)
       return ctx.Err()
   }
```

### 3.5 Script Node 샌드박스 전략

- GopherLua VM 인스턴스 생성 시 위험 모듈 제거 (os, io, debug)
- 실행 타임아웃을 context.WithTimeout으로 제어
- 메모리 제한은 GopherLua의 기본 제한 활용
- Payload <-> Lua 테이블 양방향 변환 유틸리티 구현

### 3.6 AggregateNode 윈도우 전략

- Count 윈도우: 내부 `[]message.Message` 버퍼에 축적, len(buffer) >= windowSize 시 집계
- Time 윈도우: `time.AfterFunc` 또는 `time.Timer`로 주기적 플러시
- 종료 시: `Shutdown()`에서 잔여 버퍼 강제 플러시
- 동시성 안전: buffer 접근 시 `sync.Mutex` 보호

---

## 4. 리스크 및 대응

### Risk 1: Bridge Node Agent 의존성

- **위험**: `internal/agent/` 패키지가 미완성 상태에서 Bridge Node 개발 불가
- **대응**: Agent 인터페이스를 local interface로 정의하여 mock 기반 테스트. Agent 패키지 완성 후 통합

### Risk 2: Script Node Lua VM 안정성

- **위험**: GopherLua의 메모리 누수 또는 panic으로 노드 전체 중단
- **대응**: recover() 래퍼로 panic 격리. 타임아웃으로 무한 루프 방지. VM 인스턴스를 Process 단위로 생성 또는 풀링

### Risk 3: AggregateNode 시간 윈도우 정밀도

- **위험**: time.Timer 기반 윈도우의 정밀도가 환경에 따라 달라질 수 있음
- **대응**: 테스트에서 충분한 마진(tolerance) 적용. 정밀도 요구사항이 높은 경우 벤치마크로 검증

### Risk 4: Registry 동시성 경쟁

- **위험**: 동시 Register/Create 호출 시 race condition
- **대응**: `sync.RWMutex`로 보호. Register는 Write Lock, Create/Types/Has는 Read Lock. `go test -race` 필수 실행

### Risk 5: Bridge RequestReply Correlation 누수

- **위험**: 타임아웃 처리 실패로 sync.Map에 Correlation 항목이 누적
- **대응**: 타임아웃 시 자동 삭제. Shutdown 시 잔여 항목 전체 정리. 주기적 정리 goroutine 검토

### Risk 6: 의존 SPEC 미완성

- **위험**: SPEC-LIFE-001, SPEC-FLOW-001, SPEC-MSG-001의 구현이 완료되지 않은 상태에서 개발 시작
- **대응**: 인터페이스 기반 의존으로 mock 객체를 사용한 독립 테스트 가능. 의존 패키지의 인터페이스를 local interface로 재정의하여 테스트 격리

---

## 5. 의존성 그래프

```
internal/node/ (본 SPEC)
  ├── 의존: pkg/lifecycle/          (SPEC-LIFE-001: Lifecycle, BaseLifecycle, Configurable, State)
  ├── 의존: pkg/flow/              (SPEC-FLOW-001: NodeDef, Port, PortDirection, AgentRef, BridgeDirection)
  ├── 의존: pkg/message/           (SPEC-MSG-001: Message, Payload, Metadata)
  ├── 의존: internal/observe/      (SPEC-OBS-001: Logger, Metrics 인터페이스)
  ├── 의존: internal/config/       (SPEC-CFG-001: Config, HotReload 인터페이스)
  ├── 의존: internal/script/       (별도 SPEC: Lua VM 인터페이스)
  ├── 의존: 표준 라이브러리        (context, sync, time, errors, fmt)
  ├── 소비자: internal/engine/     (SPEC-ENGINE-001: Engine이 Node 인터페이스를 goroutine으로 실행)
  ├── 소비자: internal/api/        (REST API에서 노드 정보 조회)
  └── 협력: internal/agent/        (Bridge Node가 Agent 참조)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | errors.go | Sentinel 에러, NodeError 구조체 | 없음 |
| 2 | base.go | Node 인터페이스, Port 타입, BaseNode, NodeOption | errors.go, pkg/lifecycle/, pkg/flow/ |
| 3 | registry.go | Registry, NodeFactory, 등록/생성/조회 | base.go, pkg/flow/ |
| 4 | filter.go | FilterNode 구현 | base.go, pkg/message/ |
| 5 | transform.go | TransformNode 구현 | base.go, pkg/message/ |
| 6 | switch.go | SwitchNode 구현 | base.go, pkg/message/ |
| 7 | catch.go | CatchNode 구현 | base.go, pkg/message/ |
| 8 | bridge.go | BridgeNode 구현 (4모드) | base.go, pkg/flow/, pkg/message/, internal/agent/ |
| 9 | script.go | ScriptNode 구현 (Lua) | base.go, pkg/message/, internal/script/ |
| 10 | aggregate.go | AggregateNode 구현 | base.go, pkg/message/ |
| 11 | debug.go | DebugNode 구현 | base.go, pkg/message/ |
| 12 | status.go | StatusNode 구현 | base.go, pkg/message/ |
| 13 | deadletter.go | DeadLetterNode 구현 | base.go, pkg/message/ |

모든 파일에 대해 TDD 방식으로 테스트 파일(`*_test.go`)을 먼저 작성한다.
