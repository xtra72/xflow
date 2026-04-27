# engine - XFlow FBP 런타임 엔진

`internal/engine` 패키지는 XFlow 플랫폼의 핵심 런타임 엔진이다. `pkg/flow/`에서 정의된 Flow 정의(NodeDef, Wire, FlowState)를 읽어 실제 런타임 인스턴스를 생성하고 실행하는 중앙 허브로서, Flow의 전체 생명주기(deploy, start, stop, pause, resume, undeploy)를 관리한다.

**SPEC**: SPEC-ENGINE-001

## 아키텍처 개요

```
    Engine 런타임 아키텍처

    +------------------------------------------+
    |              Engine                       |
    |  *lifecycle.BaseLifecycle 임베딩           |
    |  Flow 레지스트리, Scheduler, 옵션         |
    +------------------------------------------+
         |           |            |
    +----+----+ +----+-----+ +---+------+
    | Deploy  | | Start/   | | Status   |
    | Flow    | | Stop/    | | Query    |
    |         | | Pause/   | | (Get/    |
    |         | | Resume   | |  List)   |
    +---------+ +----------+ +----------+
         |           |
    +----+----+ +----+-----+
    |Scheduler| |  Node    |
    | (DAG    | | Goroutine|
    |  Kahn)  | |  Runner  |
    +---------+ +----------+
                     |
              +------+------+
              | RuntimeWire |
              | (channel-   |
              |  as-wire)   |
              +-------------+
```

**핵심 설계 원칙**:

1. **goroutine-per-node**: 각 노드는 독립 goroutine으로 실행되어 진정한 병렬 처리 달성
2. **channel-as-wire**: Go 채널이 Wire의 런타임 구현체이며, unbuffered(바이패스)/buffered(버퍼) 모드 제공
3. **상태 매핑 브릿지**: `pkg/flow/FlowState`(8상태)와 `pkg/lifecycle/State`(7상태) 사이의 양방향 매핑
4. **Graceful Shutdown**: 상류부터 하류 순서로 채널을 닫아 데이터 유실 없이 종료
5. **동시성 안전**: `sync.RWMutex`로 Flow 레지스트리 보호, `atomic` 타입으로 카운터/플래그 관리

## 핵심 타입 및 역할

### Engine (`engine.go`)

Flow의 배포, 실행, 관리를 담당하는 중앙 구조체이다.

```go
type Engine struct {
    *lifecycle.BaseLifecycle
    // 비공개 필드: flows, scheduler, logger, metrics, nodeRegistry, bpPolicy, config
}

func NewEngine(opts ...EngineOption) *Engine
func (e *Engine) DeployFlow(ctx context.Context, f flow.Flow) error
func (e *Engine) StartFlow(ctx context.Context, flowID string) error
func (e *Engine) StopFlow(ctx context.Context, flowID string) error
func (e *Engine) PauseFlow(ctx context.Context, flowID string) error
func (e *Engine) ResumeFlow(ctx context.Context, flowID string) error
func (e *Engine) UndeployFlow(ctx context.Context, flowID string) error
func (e *Engine) GetFlowStatus(flowID string) (FlowStatus, error)
func (e *Engine) ListFlows() []FlowStatus
func (e *Engine) Configure(ctx context.Context, cfg map[string]any) error
func (e *Engine) GetConfig() map[string]any
func (e *Engine) HealthCheck(ctx context.Context) lifecycle.HealthStatus
```

### Scheduler / DAGScheduler (`scheduler.go`)

Kahn 알고리즘 기반 DAG 위상 정렬로 노드 실행 순서를 결정한다.

```go
type Scheduler interface {
    Plan(f flow.Flow) (ExecutionPlan, error)
}

type ExecutionPlan struct {
    Levels [][]string  // 레벨별 노드 ID (같은 레벨은 병렬 실행 가능)
    Order  []string    // 전체 실행 순서 (토폴로지 정렬 결과)
}

type DAGScheduler struct{}
func NewDAGScheduler() *DAGScheduler
func (s *DAGScheduler) Plan(f flow.Flow) (ExecutionPlan, error)
```

### RuntimeWire (`wire.go`)

Wire 정의를 런타임 채널로 변환한 구조체이다. Fan-out(하나의 출력에서 여러 입력으로 복제)과 Fan-in(여러 출력에서 하나의 입력으로 합류)을 지원한다.

```go
type RuntimeWire struct {
    ID, SourceNodeID, SourcePort, TargetNodeID, TargetPort string
    Mode       flow.WireMode
    BufferSize int
    TTL        time.Duration
    Ch         chan message.Message
}

func CreateRuntimeWires(wires []flow.Wire) ([]*RuntimeWire, error)
func (w *RuntimeWire) Send(ctx context.Context, msg message.Message) error
func (w *RuntimeWire) Close()
func (w *RuntimeWire) IsClosed() bool
```

- **바이패스 모드** (`WireBypass`): `make(chan message.Message)` - unbuffered, 즉시 전달
- **버퍼 모드** (`WireBuffer`): `make(chan message.Message, bufferSize)` - buffered, 버퍼링 전달
- **닫힌 채널 안전**: `Send()`는 `atomic.Bool` 확인 + `recover()`로 panic 격리

### FlowState 매핑 (`state.go`)

`flow.FlowState`(8상태)와 `lifecycle.State`(7상태) 간의 양방향 매핑을 제공한다.

```go
func MapFlowStateToLifecycleState(fs flow.FlowState) (lifecycle.State, error)
func MapLifecycleStateToFlowState(s lifecycle.State) flow.FlowState
```

| FlowState | lifecycle.State | 비고 |
|-----------|----------------|------|
| FlowStored | (매핑 없음) | 영속화 전용, ErrStateMapping 반환 |
| FlowLoaded | StateCreated | 메모리 로드 완료 |
| FlowInitializing | StateInitializing | 초기화 진행 중 |
| FlowRunning | StateRunning | 실행 중 |
| FlowPaused | StatePaused | 일시정지 |
| FlowStopping | StateStopping | 중지 진행 중 |
| FlowStopped | StateStopped | 중지 완료 |
| FlowError | StateError | 에러 |

### Backpressure (`backpressure.go`)

백프레셔 정책 타입 정의 및 런타임 적용 로직을 제공한다.

```go
type BackpressureStrategy string  // "block" | "drop"
type DropPolicy string            // "drop_newest" | "drop_oldest"

type BackpressurePolicy struct {
    Strategy            BackpressureStrategy
    BufferHighWaterMark float64
    DropPolicy          DropPolicy
}

type BackpressureResult struct {
    Dropped              bool  // 메시지가 드롭되었는지 여부
    HighWaterMarkReached bool  // 버퍼 사용량이 HighWaterMark를 초과했는지 여부
}

func DefaultBackpressurePolicy() BackpressurePolicy
// 기본값: Strategy=StrategyBlock, HighWaterMark=0.8, DropPolicy=DropNewest

func SendWithBackpressure(ctx context.Context, wire *RuntimeWire, msg message.Message, policy BackpressurePolicy) (BackpressureResult, error)
```

- **StrategyBlock**: Go 채널의 자연 백프레셔를 활용하여 버퍼에 여유가 생길 때까지 블로킹
- **StrategyDrop + DropNewest**: 버퍼가 가득 차면 새 메시지를 드롭 (논블로킹)
- **StrategyDrop + DropOldest**: 버퍼가 가득 차면 가장 오래된 메시지를 제거하고 새 메시지를 추가
- **고수위 마크 모니터링**: 버퍼 사용량이 `BufferHighWaterMark`를 초과하면 `BackpressureResult.HighWaterMarkReached`를 `true`로 설정

### FlowStatus / flowRuntime (`types.go`)

```go
// FlowStatus - 배포된 Flow의 런타임 상태 정보 (공개)
type FlowStatus struct {
    FlowID, FlowName string
    State        flow.FlowState
    NodeCount, ActiveNodes, WireCount int
    MessageCount, ErrorCount, DroppedCount int64
    StartedAt time.Time
    Uptime    time.Duration
}

// flowRuntime - 배포된 Flow의 내부 런타임 상태 (비공개)
// flow, nodes, wires, cancel, wg, paused, 카운터 등 관리
```

### TTL Management (`ttl.go`)

Wire 상 메시지 TTL 만료 검사, Dead Letter 라우팅, 주기적 버퍼 스캔을 제공한다.

```go
type DeadLetterEntry struct {
    Message   message.Message
    Reason    string
    WireID    string
    ExpiredAt time.Time
}

type DeadLetterRouter interface {
    Route(entry DeadLetterEntry) error
}

func IsExpired(msg message.Message, ttl time.Duration) bool
func ShouldCheckTTL(wire *RuntimeWire) bool

type TTLScannerOption func(*TTLScanner)
func WithDeadLetterRouter(dlr DeadLetterRouter) TTLScannerOption
func WithScanInterval(d time.Duration) TTLScannerOption

type TTLScanner struct { /* 비공개 필드 */ }
func NewTTLScanner(opts ...TTLScannerOption) *TTLScanner
func (s *TTLScanner) Start(ctx context.Context)
func (s *TTLScanner) Wait()
func (s *TTLScanner) ScanOnce() int
func (s *TTLScanner) AddWire(wire *RuntimeWire)
func (s *TTLScanner) RemoveWire(wireID string)
func (s *TTLScanner) HandleExpiredMessage(msg message.Message, wireID string)
func (s *TTLScanner) ExpiredCount() int64
func (s *TTLScanner) DiscardedCount() int64
func (s *TTLScanner) WireCount() int
func (s *TTLScanner) ScanInterval() time.Duration
```

- **IsExpired**: 메시지 생성 시각 + TTL과 현재 시각을 비교하여 만료 여부 판정 (TTL이 0이면 만료 없음)
- **ShouldCheckTTL**: 바이패스 모드이거나 TTL이 0인 Wire는 검사 생략
- **TTLScanner**: 등록된 Wire 버퍼를 주기적으로 스캔하여 만료 메시지를 제거하는 goroutine 관리
- **Dead Letter 라우팅**: `DeadLetterRouter`가 설정되면 만료 메시지를 해당 노드로 라우팅, 없으면 자동 폐기
- **스캔 주기**: 기본 1초, `WithScanInterval` 옵션으로 변경 가능
- **메트릭**: `ExpiredCount()` (만료 메시지 수), `DiscardedCount()` (Dead Letter 없이 폐기된 메시지 수)

### EngineOption (`options.go`)

```go
type EngineOption func(*Engine)

func WithLogger(logger observe.ComponentLogger) EngineOption
func WithMetrics(metrics observe.MetricsCollector) EngineOption
func WithScheduler(s Scheduler) EngineOption
func WithNodeRegistry(r *node.Registry) EngineOption
func WithShutdownTimeout(d time.Duration) EngineOption
func WithBackpressurePolicy(p BackpressurePolicy) EngineOption
```

### Sentinel 에러 (`errors.go`)

| 에러 변수 | 용도 |
|-----------|------|
| `ErrFlowAlreadyDeployed` | 동일 ID로 이미 배포된 플로우 존재 시 |
| `ErrFlowNotFound` | 지정된 ID의 플로우를 찾을 수 없을 때 |
| `ErrFlowNotRunning` | Running 상태가 아닌 플로우에 Pause/Stop 시도 시 |
| `ErrFlowNotPaused` | Paused 상태가 아닌 플로우에 Resume 시도 시 |
| `ErrFlowNotStopped` | Stopped 상태가 아닌 플로우에 Undeploy 시도 시 |
| `ErrFlowNotLoaded` | Loaded 상태가 아닌 플로우에 Start 시도 시 |
| `ErrFlowValidationFailed` | 유효성 검증 실패한 플로우 배포 시도 시 |
| `ErrCycleDetected` | DAG에 순환이 감지되었을 때 |
| `ErrStateMapping` | FlowState-lifecycle.State 매핑 실패 시 |
| `ErrChannelClosed` | 닫힌 채널에 전송 시도 시 |
| `ErrNodeStartFailed` | 노드 시작 실패 시 |
| `ErrShutdownTimeout` | Graceful Shutdown 타임아웃 시 |

모든 에러는 `errors.Is()` 및 `errors.As()`와 호환되는 sentinel error 패턴을 사용한다.

## 모듈 구현 상태

| 모듈 | 파일 | 우선순위 | 상태 |
|------|------|---------|------|
| Engine Core | engine.go, options.go, types.go | P0 | 구현 완료 |
| Scheduler | scheduler.go | P0 | 구현 완료 |
| Wire System | wire.go | P0 | 구현 완료 |
| Backpressure (타입) | backpressure.go | P0 | 구현 완료 |
| Backpressure (로직) | backpressure.go | P1 | 구현 완료 (SendWithBackpressure, Drop 전략, 고수위 마크 모니터링) |
| State Management | state.go | P0 | 구현 완료 |
| TTL Management | ttl.go | P1 | 구현 완료 (TTLScanner, IsExpired, ShouldCheckTTL, DeadLetterRouter) |
| Error Types | errors.go | P0 | 구현 완료 |

## 파일 구조

```
internal/engine/
  engine.go              # Engine 구조체, NewEngine(), DeployFlow/Start/Stop/Pause/Resume/Undeploy
  scheduler.go           # Scheduler 인터페이스, DAGScheduler (Kahn 알고리즘), ExecutionPlan
  wire.go                # RuntimeWire, CreateRuntimeWires(), Send/Close/IsClosed
  backpressure.go        # BackpressureStrategy, DropPolicy, BackpressurePolicy, SendWithBackpressure
  state.go               # MapFlowStateToLifecycleState(), MapLifecycleStateToFlowState()
  ttl.go                 # TTLScanner, IsExpired, ShouldCheckTTL, DeadLetterRouter
  errors.go              # 12개 sentinel 에러 변수
  options.go             # EngineOption 함수형 옵션 (6종)
  types.go               # FlowStatus (공개), flowRuntime (비공개)

  engine_test.go         # Engine 통합 테스트 (생명주기, 메시지 흐름, Fan-out)
  scheduler_test.go      # DAG 위상 정렬, 순환 감지, 빈 그래프 테스트
  wire_test.go           # RuntimeWire 생성, 모드별 전달, Close 안전성 테스트
  backpressure_test.go   # 백프레셔 전략, 드롭 정책, SendWithBackpressure 테스트
  state_test.go          # FlowState-lifecycle.State 양방향 매핑 테스트
  ttl_test.go            # TTL 만료, Dead Letter 라우팅, TTLScanner 스캔 테스트
  errors_test.go         # sentinel 에러 존재 및 래핑 호환성 테스트
```

## 의존성

- **표준 라이브러리**: `context`, `sync`, `sync/atomic`, `time`, `errors`, `fmt`
- **내부 의존성**:
  - `pkg/lifecycle` (SPEC-LIFE-001) - `BaseLifecycle` 임베딩, `State` 타입
  - `pkg/flow` (SPEC-FLOW-001) - `Flow`, `NodeDef`, `Wire`, `FlowState`, `WireMode`
  - `pkg/message` (SPEC-MSG-001) - `Message` 인터페이스 (Wire 통과 데이터)
  - `internal/node` (SPEC-NODE-001) - `Node` 인터페이스, `Registry`
  - `internal/observe` (SPEC-OBS-001) - `ComponentLogger`, `MetricsCollector`

## 사용 예시

### Engine 생성 및 Flow 배포

```go
// Engine 생성
engine := engine.NewEngine(
    engine.WithLogger(logger),
    engine.WithMetrics(metrics),
    engine.WithNodeRegistry(registry),
    engine.WithShutdownTimeout(30 * time.Second),
)

// Flow 배포 (유효성 검사 -> 노드 생성 -> 채널 생성 -> 등록)
err := engine.DeployFlow(ctx, myFlow)

// Flow 시작 (스케줄링 -> goroutine 실행)
err = engine.StartFlow(ctx, "flow-1")

// Flow 상태 조회
status, err := engine.GetFlowStatus("flow-1")

// Flow 일시정지/재개
err = engine.PauseFlow(ctx, "flow-1")
err = engine.ResumeFlow(ctx, "flow-1")

// Flow 중지 -> 배포 해제
err = engine.StopFlow(ctx, "flow-1")
err = engine.UndeployFlow(ctx, "flow-1")
```

### 커스텀 Scheduler 사용

```go
engine := engine.NewEngine(
    engine.WithScheduler(myCustomScheduler),
)
```

## 테스트

```bash
# 전체 테스트 실행
go test ./internal/engine/...

# Race Detector 포함 테스트
go test -race ./internal/engine/...

# 커버리지 확인
go test -cover ./internal/engine/

# 상세 커버리지 리포트
go test -coverprofile=cover.out ./internal/engine/
go tool cover -html=cover.out
```

### 테스트 결과

- 테스트 수: 84개
- 커버리지: 90.7%
- Race Detector: 이상 없음 (go test -race)

## 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-ENGINE-001 | 본 SPEC | Engine 시스템 전체 명세 |
| SPEC-LIFE-001 | 의존 | `BaseLifecycle` 상태 관리 (임베딩) |
| SPEC-FLOW-001 | 의존 | `Flow`, `NodeDef`, `Wire`, `FlowState` 데이터 구조 |
| SPEC-MSG-001 | 의존 | `Message` 인터페이스 (Wire 통과 데이터) |
| SPEC-NODE-001 | 의존 | `Node` 인터페이스, `Registry` (노드 생성/관리) |
| SPEC-OBS-001 | 소비자 | `ComponentLogger`, `MetricsCollector` 관측 인터페이스 |
