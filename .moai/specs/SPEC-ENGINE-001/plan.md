---
id: SPEC-ENGINE-001
type: plan
version: "1.0.0"
spec_ref: SPEC-ENGINE-001
---

# SPEC-ENGINE-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 모든 파일이 신규 생성이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (goroutine/채널 기반 동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성**: goroutine + channel (노드 실행), `sync.Mutex` (상태 관리), `sync.RWMutex` (레지스트리)
- **컨텍스트**: `context.Context` (취소, 타임아웃 전파)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): BaseLifecycle, Configurable, HealthChecker
  - `pkg/flow/` (SPEC-FLOW-001): Flow, NodeDef, Wire, FlowState, WireMode
  - `pkg/message/` (SPEC-MSG-001): Message 타입
  - `internal/observe/` (SPEC-OBS-001): Logger, Metrics 인터페이스
  - `internal/config/` (SPEC-CFG-001): Config, HotReload 인터페이스

### 1.3 패키지 위치

- **경로**: `internal/engine/`
- **Tier**: internal (비공개 패키지)
- **소비자**: `internal/api/` (REST API), `cmd/xflowd/` (데몬 서버)

---

## 2. 마일스톤

### Primary Goal: Error Types + State Management + Scheduler + Wire System (P0)

**범위**: Module 2, 3, 5, 7

**작업 항목**:

1. `errors.go` + `errors_test.go` 작성
   - 12개 sentinel error 변수 정의
   - `errors.Is()` 호환성 테스트

2. `state.go` + `state_test.go` 작성
   - `MapFlowStateToLifecycleState()` 함수 구현
   - `MapLifecycleStateToFlowState()` 함수 구현
   - 8개 FlowState -> 7개 lifecycle.State 매핑 테이블 기반 테스트
   - FlowStored 매핑 시 에러 반환 테스트
   - 양방향 매핑 일관성 테스트

3. `scheduler.go` + `scheduler_test.go` 작성
   - `Scheduler` 인터페이스 정의
   - `ExecutionPlan` 구조체 정의
   - `DAGScheduler` 구현 (Kahn의 알고리즘 기반 위상 정렬)
   - 선형 체인 Flow 실행 계획 테스트
   - 병렬 분기 Flow 실행 계획 테스트 (레벨별 병렬 노드 확인)
   - Fan-out/Fan-in 토폴로지 테스트
   - 순환 그래프 감지 테스트 (`ErrCycleDetected`)
   - 빈 Flow 처리 테스트
   - 단일 노드 Flow 테스트

4. `wire.go` + `wire_test.go` 작성
   - `RuntimeWire` 구조체 정의
   - `CreateRuntimeWires()` 함수 구현
   - 바이패스 모드 채널 생성 테스트 (unbuffered)
   - 버퍼 모드 채널 생성 테스트 (buffered)
   - Fan-out 메시지 복제 테스트
   - Fan-in 메시지 합류 테스트
   - 수신 노드 상태별 전달 동작 테스트 (Running/Paused/Stopped)
   - 닫힌 채널 전송 방지 테스트

**산출물**: Engine의 기반 인프라 완성 (상태 매핑, 스케줄링, Wire 시스템)

---

### Secondary Goal: Engine Core (P0)

**범위**: Module 1

**작업 항목**:

1. `options.go` 작성
   - `EngineOption` 함수 타입
   - `WithLogger()`, `WithMetrics()`, `WithConfig()`, `WithScheduler()`, `WithBackpressurePolicy()`, `WithShutdownTimeout()` 옵션

2. `types.go` 작성
   - `FlowStatus` 구조체 정의
   - `flowRuntime` 내부 구조체 정의 (Flow 정의 + RuntimeWire + 노드 goroutine 관리)

3. `engine.go` + `engine_test.go` 작성
   - `Engine` 구조체 (`BaseLifecycle` 임베딩)
   - `NewEngine()` 생성자
   - `DeployFlow()` 구현 (유효성 검증 + 런타임 생성 + 레지스트리 등록)
   - `StartFlow()` 구현 (스케줄링 + 노드 goroutine 시작 + 상태 전이)
   - `StopFlow()` 구현 (Graceful Shutdown: 채널 닫기 + goroutine 대기 + 드레인)
   - `PauseFlow()` 구현 (Pause 전파)
   - `ResumeFlow()` 구현 (Resume 전파 + 큐 데이터 처리)
   - `UndeployFlow()` 구현 (레지스트리 제거 + 리소스 해제)
   - `GetFlowStatus()`, `ListFlows()` 구현
   - `Configure()`, `GetConfig()` 구현 (Configurable 인터페이스)
   - `HealthCheck()` 구현 (HealthChecker 인터페이스)
   - 플로우 전체 생명주기 통합 테스트 (Deploy -> Start -> Pause -> Resume -> Stop -> Undeploy)
   - 중복 배포 거부 테스트
   - 유효하지 않은 Flow 배포 거부 테스트
   - 동시 다중 플로우 실행 테스트
   - Graceful Shutdown 타임아웃 테스트
   - 노드 시작 실패 시 롤백 테스트

**산출물**: Engine Core 기능 완성 (플로우 전체 생명주기 관리)

---

### Final Goal: Backpressure + TTL Management (P1)

**범위**: Module 4, 6

**작업 항목**:

1. `backpressure.go` + `backpressure_test.go` 작성
   - `BackpressurePolicy`, `BackpressureStrategy`, `DropPolicy` 타입 정의
   - `DefaultBackpressurePolicy()` 함수
   - Block 전략 테스트 (자연적 백프레셔, 채널 블로킹 확인)
   - Drop 전략 테스트 (DropNewest/DropOldest 동작 확인)
   - 고수위 마크 경고 로그 생성 테스트
   - 드롭 메트릭 기록 테스트

2. `ttl.go` + `ttl_test.go` 작성
   - `TTLScanner` 구조체
   - `NewTTLScanner()`, `Start()`, `Stop()`, `AddWire()`, `RemoveWire()`
   - TTL 만료 검사 로직 구현
   - Dead Letter 라우팅 구현
   - 주기적 버퍼 스캔 구현
   - 바이패스 모드 TTL 검사 생략 테스트
   - 버퍼 내 만료 메시지 제거 테스트
   - Dead Letter 노드 연결/미연결 시 처리 테스트
   - 스캔 goroutine 종료 테스트 (context 취소)
   - 만료 메트릭 기록 테스트

**산출물**: 백프레셔 제어 및 TTL 관리 기능 완성

---

### Optional Goal: 성능 최적화 및 고급 기능 (P2)

**범위**: 성능 벤치마크, 메모리 최적화

**작업 항목**:

1. 벤치마크 테스트 작성
   - 단일 플로우 메시지 처리 처리량 벤치마크 (목표: 10,000+ msg/s)
   - 다중 플로우(100+) 동시 실행 벤치마크
   - 메모리 사용량 프로파일링 (목표: 유휴 시 < 256MB)
   - P95 지연시간 측정 (목표: 노드당 < 10ms)

2. 메모리 최적화
   - 메시지 풀링 (sync.Pool) 도입 검토
   - Fan-out 시 메시지 복사 최소화

**산출물**: 성능 목표 달성 검증 및 최적화

---

## 3. 기술적 접근

### 3.1 Engine 아키텍처

```
Engine
├── registry (sync.RWMutex 보호)
│   └── flowRuntime 맵 (flowID -> flowRuntime)
├── scheduler (DAGScheduler)
├── config (런타임 설정)
├── logger, metrics (관찰성)
└── BaseLifecycle (Engine 자체 생명주기)

flowRuntime
├── flow (flow.Flow 정의)
├── wires ([]*RuntimeWire)
├── nodes (map[string]NodeRunner - 노드 ID -> 런타임 노드)
├── cancel (context.CancelFunc - 플로우 전체 취소)
├── wg (sync.WaitGroup - goroutine 대기)
├── status (FlowStatus - 런타임 통계)
└── ttlScanner (*TTLScanner - 해당 플로우의 TTL 스캐너)
```

### 3.2 goroutine-per-node 모델

각 노드는 독립 goroutine으로 실행된다:

```
goroutine (node-A):
  for msg := range inputCh {
      result := node.Process(ctx, msg)
      outputCh <- result
  }
```

- 입력 채널이 닫히면 `range` 루프가 자연 종료
- `context.Context`로 취소 전파 (타임아웃, 강제 종료)
- Pause 시 `select`로 pause 신호 채널을 먼저 확인

### 3.3 Graceful Shutdown 전략

1. 소스 노드(입력 Wire 없는 노드)의 context 취소
2. 소스 노드가 출력 채널을 닫으며 종료
3. 하류 노드가 입력 채널 고갈 후 자연 종료 (cascading close)
4. `sync.WaitGroup`으로 모든 goroutine 종료 대기
5. 타임아웃 초과 시 강제 종료 (`context.WithTimeout`)

### 3.4 DAGScheduler 알고리즘

Kahn의 알고리즘 기반 위상 정렬:

1. 각 노드의 in-degree(입력 Wire 수) 계산
2. in-degree가 0인 노드를 큐에 추가 (Level 0)
3. 큐에서 노드를 꺼내며 해당 노드의 출력 Wire가 연결된 노드의 in-degree 감소
4. in-degree가 0이 된 노드를 다음 레벨에 추가
5. 모든 노드가 처리될 때까지 반복
6. 처리되지 않은 노드가 남으면 순환 감지

### 3.5 FlowState <-> lifecycle.State 동기화

Engine은 두 상태를 동시에 관리한다:

- `flow.Flow.SetState()`: FlowState 변경 (Flow 도메인 상태)
- `BaseLifecycle.TransitionTo()`: lifecycle.State 변경 (공통 생명주기 상태)

매핑 함수를 통해 두 상태가 항상 일관성을 유지하도록 보장한다. FlowStored는 영속화 전용 상태이므로 Engine이 관리하는 lifecycle 범위에 포함되지 않는다.

### 3.6 Wire Fan-out 전략

하나의 출력 포트에서 여러 Wire가 연결된 경우:

- 메시지를 각 Wire에 복사하여 전달 (독립적 메시지 인스턴스)
- 메시지 Payload가 `[]byte`이므로 얕은 복사로 충분 (변경 불가능 관례)
- 모든 Wire에 전달 성공해야 다음 메시지 처리 (또는 백프레셔 정책에 따라 드롭)

---

## 4. 리스크 및 대응

### Risk 1: goroutine 누수

- **위험**: 노드 goroutine이 종료되지 않고 누적되어 메모리 누수 발생
- **대응**: `context.Context` 기반 취소 전파 + `sync.WaitGroup` 기반 종료 확인. StopFlow 시 타임아웃(기본 30초) 후 강제 종료. goroutine 수 메트릭 추적

### Risk 2: 채널 교착 상태(deadlock)

- **위험**: 순환 의존 또는 버퍼 부족으로 채널 교착 발생
- **대응**: DAGScheduler가 순환을 사전 감지하여 배포 차단. 바이패스 모드에서의 교착은 Go 런타임이 감지. 버퍼 모드에서는 BackpressurePolicy.DropPolicy로 해소

### Risk 3: 닫힌 채널 panic

- **위험**: 닫힌 채널에 메시지 전송 시 Go 런타임 panic 발생
- **대응**: 전송 함수에 `recover()` 래퍼 적용. 채널 상태 플래그(`atomic.Bool`)로 전송 전 확인. 에러 로그 기록 후 메시지 폐기

### Risk 4: FlowState-lifecycle.State 불일치

- **위험**: 한쪽만 전이 성공하고 다른 쪽이 실패하여 상태 불일치 발생
- **대응**: 매핑 함수에서 양쪽 전이를 원자적으로 수행. FlowState 전이 실패 시 lifecycle 전이를 롤백. 불일치 감지 시 `ErrStateMapping` 에러 + 복구 로직

### Risk 5: 대규모 플로우에서의 Shutdown 지연

- **위험**: 수백 개 노드를 가진 플로우의 Graceful Shutdown이 오래 걸림
- **대응**: Cascading close 패턴으로 자연스러운 전파. `ShutdownTimeout` 옵션으로 최대 대기 시간 설정. 타임아웃 초과 시 강제 종료 + 리소스 정리

### Risk 6: 의존 SPEC 미완성

- **위험**: SPEC-LIFE-001, SPEC-FLOW-001, SPEC-MSG-001의 구현이 완료되지 않은 상태에서 개발 시작
- **대응**: 인터페이스 기반 의존으로 mock 객체를 사용한 독립 테스트 가능. 의존 패키지의 인터페이스를 local interface로 재정의하여 테스트 격리

---

## 5. 의존성 그래프

```
internal/engine/ (본 SPEC)
  ├── 의존: pkg/lifecycle/          (SPEC-LIFE-001: BaseLifecycle, Configurable, HealthChecker, State)
  ├── 의존: pkg/flow/              (SPEC-FLOW-001: Flow, NodeDef, Wire, FlowState, WireMode, Port)
  ├── 의존: pkg/message/           (SPEC-MSG-001: Message 타입)
  ├── 의존: internal/observe/      (SPEC-OBS-001: Logger, Metrics, Trace 인터페이스)
  ├── 의존: internal/config/       (SPEC-CFG-001: Config, HotReload 인터페이스)
  ├── 의존: 표준 라이브러리        (context, sync, time, errors, fmt, sort)
  ├── 소비자: internal/api/        (REST API에서 Engine 메서드 호출)
  ├── 소비자: cmd/xflowd/          (데몬 서버에서 Engine 초기화)
  └── 협력: internal/node/         (Node 인터페이스의 런타임 인스턴스 실행)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | errors.go | Sentinel 에러 정의 | 없음 |
| 2 | state.go | FlowState <-> lifecycle.State 매핑 | errors.go, pkg/lifecycle/, pkg/flow/ |
| 3 | scheduler.go | DAGScheduler, 위상 정렬, 순환 감지 | pkg/flow/ |
| 4 | wire.go | RuntimeWire, 채널 생성, Fan-out/Fan-in | pkg/flow/, pkg/message/ |
| 5 | backpressure.go | BackpressurePolicy, Strategy, DropPolicy | 없음 |
| 6 | ttl.go | TTLScanner, 만료 검사, Dead Letter | wire.go, pkg/message/ |
| 7 | options.go | EngineOption, With*() 함수 | 전체 타입 참조 |
| 8 | types.go | FlowStatus, flowRuntime 내부 타입 | wire.go, pkg/flow/ |
| 9 | engine.go | Engine 구조체, 전체 플로우 생명주기 | 전체 (1-8) + pkg/lifecycle/ |

모든 파일에 대해 TDD 방식으로 테스트 파일(`*_test.go`)을 먼저 작성한다.
