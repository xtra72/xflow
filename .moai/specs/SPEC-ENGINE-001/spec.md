---
id: SPEC-ENGINE-001
version: "1.1.0"
status: implemented
created: "2026-02-13"
updated: "2026-02-16"
author: xtra
priority: high
implementation_commit: f948296
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |
| 2026-02-16 | 1.1.0 | P0 구현 완료 (Module 1,2,3,4,5,7), 문서 동기화 |

---

# SPEC-ENGINE-001: Engine System - FBP 런타임 엔진, 플로우 실행, 스케줄링, Wire 메시지 전달, 백프레셔, 상태 관리

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 핵심 런타임 엔진을 정의한다. Engine은 `pkg/flow/`에서 정의된 Flow 정의(NodeDef, Wire, FlowState)를 읽어 실제 런타임 인스턴스를 생성하고 실행하는 중앙 허브이다. 각 Flow는 독립된 goroutine 집합(노드당 하나)으로 실행되며, Wire 채널이 goroutine 간 메시지 전달을 담당한다.

본 SPEC은 다음을 포함한다:

- **Engine Core** (`engine.go`): 플로우 실행 루프, 노드 goroutine 초기화, 플로우 전체 생명주기 조율
- **Scheduler** (`scheduler.go`): DAG 위상 정렬 기반 노드 실행 순서 결정, 병렬 실행 계획 수립
- **Wire System** (`wire.go`): 바이패스/버퍼 모드 채널, 수신 노드 상태별 전달 전략, Fan-out/Fan-in
- **Backpressure** (`backpressure.go`): Go 채널 버퍼 기반 백프레셔, 드롭 정책, 상류 속도 제한
- **State Management** (`state.go`): FlowState <-> lifecycle.State 매핑, 플로우 생명주기 상태 머신 실행
- **TTL Management** (`ttl.go`): Wire 상 메시지 TTL 만료 검사, Dead Letter 라우팅, 만료 메트릭

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/engine/`
- **Tier**: internal (비공개 패키지)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): `BaseLifecycle`, `Configurable`, `HealthChecker`, `State` 임베딩
  - `pkg/flow/` (SPEC-FLOW-001): `Flow`, `NodeDef`, `Wire`, `FlowState`, `Port`, `WireMode` 데이터 구조
  - `pkg/message/` (SPEC-MSG-001): `Message` 타입 (Wire 통과 메시지)
  - `internal/observe/` (SPEC-OBS-001): `Logger`, `Metrics`, `Trace` 관찰성 통합
  - `internal/config/` (SPEC-CFG-001): `Config`, `HotReload` 런타임 설정 변경
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: goroutine + channel (노드당 1 goroutine, Wire당 1 channel)

### 1.3 설계 원칙

- **goroutine-per-node**: 각 노드는 독립 goroutine으로 실행되어 진정한 병렬 처리 달성
- **channel-as-wire**: Go 채널이 Wire의 런타임 구현체이며, unbuffered(바이패스)/buffered(버퍼) 모드 제공
- **상태 매핑 브릿지**: Engine은 `pkg/flow/FlowState`(8상태)와 `pkg/lifecycle/State`(7상태) 사이의 매핑을 담당
- **Graceful Shutdown**: 모든 채널 데이터를 플러시한 후 goroutine 종료, 데이터 유실 방지
- **관찰성 내장**: 모든 상태 전이, 메시지 전달, 에러에 대해 로그/메트릭/트레이스 생성
- **동시성 안전**: 플로우 상태 관리는 `sync.Mutex` 보호, Wire 채널은 Go 런타임이 보장
- **정책 기반 설계**: 백프레셔 전략, 드롭 정책, TTL 처리를 정책 타입으로 정의하여 런타임 변경 지원

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Engine Core: 플로우 실행 루프, 노드 goroutine 관리, 플로우 시작/중지/일시정지/재개
- Scheduler: DAG 위상 정렬, 병렬 실행 레벨 계산
- Wire System: 런타임 채널 생성, 바이패스/버퍼 모드, 수신 노드 상태별 전달 전략, Fan-out/Fan-in
- Backpressure: 채널 버퍼 기반 자연적 백프레셔, 드롭 정책, 속도 제한
- State Management: FlowState <-> lifecycle.State 매핑 함수, 플로우 상태 전이 실행
- TTL Management: Wire 상 메시지 TTL 검사, Dead Letter 라우팅, 만료 버퍼 스캔

**OUT OF SCOPE (별도 SPEC)**:
- 노드 프로세스 실행 로직 (별도 SPEC: `internal/node/`)
- 노드 타입 레지스트리 (별도 SPEC: `internal/node/registry.go`)
- Agent 생명주기 관리 (별도 SPEC: `internal/agent/`)
- REST API 핸들러 (별도 SPEC: `internal/api/`)
- Flow 데이터 구조 정의 (SPEC-FLOW-001: `pkg/flow/`)
- 공통 생명주기 인터페이스 (SPEC-LIFE-001: `pkg/lifecycle/`)
- 메시지 구조 정의 (SPEC-MSG-001: `pkg/message/`)
- 관찰성 시스템 구현 (SPEC-OBS-001: `internal/observe/`)
- 설정 시스템 구현 (SPEC-CFG-001: `internal/config/`)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | `BaseLifecycle` 임베딩, `State` 타입, `Configurable`, `HealthChecker` 인터페이스 |
| SPEC-FLOW-001 | 의존 | `Flow`, `NodeDef`, `Wire`, `FlowState`, `Port`, `WireMode` 데이터 구조 소비 |
| SPEC-MSG-001 | 의존 | `Message` 타입 (Wire를 통해 노드 간 전달되는 데이터 단위) |
| SPEC-OBS-001 | 소비자 | Engine이 생성하는 로그, 메트릭, 트레이스를 관찰성 시스템에 전달 |
| SPEC-CFG-001 | 소비자 | Engine 런타임 설정(백프레셔 임계값, 실행 정책 등) 읽기 및 Hot Reload 수신 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: `pkg/flow/Flow` 인터페이스의 `Nodes()`, `Wires()` 메서드가 플로우 정의 데이터를 반환한다
- A2: Go 채널의 send/receive 연산은 Go 런타임에 의해 동시성 안전이 보장된다
- A3: unbuffered 채널(바이패스 모드)에서 sender는 receiver가 준비될 때까지 자연적으로 대기한다 (Go 채널 특성)
- A4: buffered 채널(버퍼 모드)에서 버퍼가 가득 차면 sender는 자연적으로 대기한다 (Go 채널 백프레셔)
- A5: `context.Context`를 통해 goroutine 취소 및 타임아웃을 전파한다
- A6: 각 플로우는 독립적으로 실행되며, 플로우 간 직접 goroutine/채널 공유는 없다
- A7: 동시에 100개 이상의 플로우를 실행할 수 있어야 하며, 메시지 처리율은 10,000+ msg/s를 목표로 한다
- A8: `internal/engine/`은 `pkg/lifecycle/`, `pkg/flow/`, `pkg/message/` 패키지에 의존하지만, 역방향 의존은 없다
- A9: 노드 구현체는 `internal/node/`에서 제공하며, Engine은 노드의 `Process()` 함수를 호출하는 인터페이스만 정의한다

### 2.2 도메인 가정

- A10: 하나의 Flow는 DAG(유향 비순환 그래프) 구조로, 순환이 있으면 유효성 검증에서 경고를 생성한다
- A11: 플로우 일시정지 시 모든 노드 goroutine이 입력 채널 읽기를 중단하지만, 채널과 goroutine은 유지된다
- A12: 플로우 재개 시 채널에 큐잉된 메시지부터 순서대로 처리를 재개한다
- A13: Wire의 TTL은 선택적(0이면 미적용)이며, 버퍼 모드에서만 의미가 있다 (바이패스는 즉시 전달)
- A14: Dead Letter 노드가 연결되지 않은 경우, TTL 만료 메시지는 자동 폐기되며 메트릭만 기록한다
- A15: 노드 추가/제거는 Hot Configuration으로 지원하지 않으며, 플로우 재시작이 필요하다
- A16: 백프레셔 임계값 및 실행 정책은 Hot Configuration으로 런타임 변경이 가능하다

---

## 3. Requirements (요구사항)

### Module 1: Engine Core - 엔진 코어 (P0, 구현 완료)

#### REQ-ENGINE-001-01-01 (Ubiquitous) Engine 구조체 정의

시스템은 **항상** 다음 기능을 제공하는 `Engine` 구조체를 제공해야 한다:

- `pkg/lifecycle/BaseLifecycle` 임베딩 (상태 관리 재사용)
- `pkg/lifecycle/Configurable` 인터페이스 구현 (런타임 설정 변경)
- `pkg/lifecycle/HealthChecker` 인터페이스 구현 (헬스 체크)
- 동시에 여러 Flow를 관리하는 내부 레지스트리
- Flow별 런타임 인스턴스(`flowRuntime`) 관리

#### REQ-ENGINE-001-01-02 (Ubiquitous) NewEngine() 생성자

시스템은 **항상** `NewEngine(opts ...EngineOption) *Engine` 생성자를 제공해야 한다:

- `BaseLifecycle` 초기 상태: `StateCreated`
- Options Pattern으로 Logger, Metrics, Config 주입
- 내부 레지스트리 초기화

#### REQ-ENGINE-001-01-03 (Event-Driven) DeployFlow 플로우 배포

**WHEN** `Engine.DeployFlow(ctx, flow Flow)` 호출 시 유효한 Flow 정의가 전달되면, **THEN** 다음을 수행해야 한다:

1. Flow 유효성 검증 (`pkg/flow/Validate()` 호출)
2. 각 NodeDef에 대해 런타임 노드 인스턴스 생성
3. 각 Wire에 대해 런타임 채널 생성 (WireMode에 따라 unbuffered/buffered)
4. 노드-채널 연결 완료
5. flowRuntime을 내부 레지스트리에 등록
6. FlowState를 `FlowLoaded`로 설정

#### REQ-ENGINE-001-01-04 (Event-Driven) StartFlow 플로우 시작

**WHEN** `Engine.StartFlow(ctx, flowID string)` 호출 시 해당 Flow가 `FlowLoaded` 상태이면, **THEN** 다음을 수행해야 한다:

1. FlowState를 `FlowInitializing`으로 전이
2. 스케줄러를 통해 노드 실행 순서 결정
3. 각 노드를 goroutine으로 시작 (실행 순서에 따라)
4. 모든 노드 시작 성공 시 FlowState를 `FlowRunning`으로 전이
5. 노드 시작 실패 시 FlowState를 `FlowError`로 전이 및 이미 시작된 노드 정리

#### REQ-ENGINE-001-01-05 (Event-Driven) StopFlow 플로우 중지

**WHEN** `Engine.StopFlow(ctx, flowID string)` 호출 시 해당 Flow가 `FlowRunning` 또는 `FlowPaused` 상태이면, **THEN** 다음을 수행해야 한다:

1. FlowState를 `FlowStopping`으로 전이
2. 모든 입력 채널 닫기 (상류부터 하류 순서)
3. 각 노드 goroutine의 완료 대기 (context 타임아웃 적용)
4. 모든 Wire 채널 닫기 및 잔여 메시지 드레인
5. FlowState를 `FlowStopped`로 전이
6. flowRuntime 리소스 정리

#### REQ-ENGINE-001-01-06 (Event-Driven) PauseFlow 플로우 일시정지

**WHEN** `Engine.PauseFlow(ctx, flowID string)` 호출 시 해당 Flow가 `FlowRunning` 상태이면, **THEN** 다음을 수행해야 한다:

1. FlowState를 `FlowPaused`로 전이
2. 모든 노드에 Pause 신호 전파 (각 노드가 입력 채널 읽기 중단)
3. 채널은 열린 상태 유지 (큐잉된 메시지 보존)

#### REQ-ENGINE-001-01-07 (Event-Driven) ResumeFlow 플로우 재개

**WHEN** `Engine.ResumeFlow(ctx, flowID string)` 호출 시 해당 Flow가 `FlowPaused` 상태이면, **THEN** 다음을 수행해야 한다:

1. FlowState를 `FlowRunning`으로 전이
2. 모든 노드에 Resume 신호 전파
3. 각 노드가 채널에 큐잉된 메시지부터 순서대로 처리 재개

#### REQ-ENGINE-001-01-08 (Event-Driven) UndeployFlow 플로우 배포 해제

**WHEN** `Engine.UndeployFlow(ctx, flowID string)` 호출 시 해당 Flow가 `FlowStopped` 상태이면, **THEN** 다음을 수행해야 한다:

1. flowRuntime을 내부 레지스트리에서 제거
2. 모든 런타임 리소스(채널, 노드 참조) 해제
3. FlowState를 `FlowStored`로 전이 (또는 레지스트리에서 완전 제거)

#### REQ-ENGINE-001-01-09 (Ubiquitous) GetFlowStatus 플로우 상태 조회

시스템은 **항상** `Engine.GetFlowStatus(flowID string) (FlowStatus, error)` 메서드를 제공하여 플로우의 현재 상태, 노드 수, 활성 goroutine 수, 메시지 처리량 등의 런타임 정보를 반환해야 한다.

#### REQ-ENGINE-001-01-10 (Ubiquitous) ListFlows 플로우 목록 조회

시스템은 **항상** `Engine.ListFlows() []FlowStatus` 메서드를 제공하여 현재 배포된 모든 플로우의 상태 목록을 반환해야 한다.

#### REQ-ENGINE-001-01-11 (Unwanted) 중복 플로우 배포 거부

시스템은 동일한 Flow ID로 이미 배포된 플로우가 존재할 때 재배포를 **허용하지 않아야 한다**. `ErrFlowAlreadyDeployed` 에러를 반환해야 한다.

#### REQ-ENGINE-001-01-12 (Unwanted) 유효하지 않은 Flow 배포 거부

시스템은 `pkg/flow/Validate()`에서 Error 심각도의 ValidationError가 발견된 Flow의 배포를 **허용하지 않아야 한다**. `ErrFlowValidationFailed` 에러를 반환해야 한다.

---

### Module 2: Scheduler - 노드 스케줄링 (P0, 구현 완료)

#### REQ-ENGINE-001-02-01 (Ubiquitous) Scheduler 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Scheduler` 인터페이스를 제공해야 한다:

- `Plan(flow Flow) (ExecutionPlan, error)` - 실행 계획 생성

#### REQ-ENGINE-001-02-02 (Ubiquitous) ExecutionPlan 구조체

시스템은 **항상** 다음 필드를 포함하는 `ExecutionPlan` 구조체를 제공해야 한다:

- `Levels [][]string` - 위상 정렬 레벨별 노드 ID 목록 (같은 레벨은 병렬 실행 가능)
- `Order []string` - 전체 실행 순서 (위상 정렬 결과)

#### REQ-ENGINE-001-02-03 (Event-Driven) 위상 정렬 실행 계획

**WHEN** `Scheduler.Plan(flow)` 호출 시 유효한 DAG 구조의 Flow가 전달되면, **THEN** Wire 연결 관계를 분석하여 위상 정렬된 `ExecutionPlan`을 반환해야 한다.

- 입력 Wire가 없는 노드(소스 노드)는 Level 0에 배치
- 모든 입력이 이전 레벨에서 공급되는 노드는 다음 레벨에 배치
- 같은 레벨의 노드는 병렬 시작 가능

#### REQ-ENGINE-001-02-04 (Event-Driven) 순환 그래프 감지

**WHEN** `Scheduler.Plan(flow)` 호출 시 Flow에 순환(cycle)이 존재하면, **THEN** `ErrCycleDetected` 에러를 반환해야 한다.

#### REQ-ENGINE-001-02-05 (Ubiquitous) DAGScheduler 기본 구현체

시스템은 **항상** `Scheduler` 인터페이스의 기본 구현체인 `DAGScheduler`를 제공해야 한다. Kahn의 알고리즘 기반 위상 정렬을 사용한다.

---

### Module 3: Wire System - Wire 런타임 (P0, 구현 완료)

#### REQ-ENGINE-001-03-01 (Ubiquitous) RuntimeWire 구조체

시스템은 **항상** Wire 정의를 런타임 채널로 변환한 `RuntimeWire` 구조체를 제공해야 한다:

- 바이패스 모드: `make(chan message.Message)` (unbuffered)
- 버퍼 모드: `make(chan message.Message, bufferSize)` (buffered)
- 소스 노드 ID, 포트 이름
- 타겟 노드 ID, 포트 이름
- TTL 설정값

#### REQ-ENGINE-001-03-02 (Ubiquitous) CreateRuntimeWires 함수

시스템은 **항상** `CreateRuntimeWires(wires []flow.Wire) ([]*RuntimeWire, error)` 함수를 제공하여, Flow 정의의 Wire 목록을 RuntimeWire 목록으로 변환해야 한다.

#### REQ-ENGINE-001-03-03 (State-Driven) 수신 노드 Running 상태 시 메시지 전달

**IF** 수신 노드가 `Running` 상태일 때, **THEN** Wire는 메시지를 즉시 전달해야 한다 (바이패스: 즉시, 버퍼: 버퍼에 추가).

#### REQ-ENGINE-001-03-04 (State-Driven) 수신 노드 Paused 상태 시 메시지 처리

**IF** 수신 노드가 `Paused` 상태일 때, **THEN** 바이패스 모드 Wire에서는 송신 노드가 대기(블로킹)하고, 버퍼 모드 Wire에서는 메시지가 버퍼에 축적되며 버퍼가 가득 차면 송신 노드가 대기해야 한다.

#### REQ-ENGINE-001-03-05 (State-Driven) 수신 노드 Stopped 상태 시 에러 포트 라우팅

**IF** 수신 노드가 `Stopped` 상태일 때, **THEN** Wire는 메시지를 에러 포트로 라우팅해야 한다. 에러 포트가 없으면 메시지를 폐기하고 메트릭을 기록한다.

#### REQ-ENGINE-001-03-06 (Ubiquitous) Fan-out 지원

시스템은 **항상** 하나의 출력 포트에서 여러 입력 포트로의 메시지 복제(fan-out)를 지원해야 한다. 각 대상 Wire에 메시지 사본을 전달한다.

#### REQ-ENGINE-001-03-07 (Ubiquitous) Fan-in 지원

시스템은 **항상** 여러 출력 포트에서 하나의 입력 포트로의 메시지 합류(fan-in)를 지원해야 한다. 메시지는 도착 순서대로 처리된다.

#### REQ-ENGINE-001-03-08 (Unwanted) 닫힌 채널 전송 방지

시스템은 이미 닫힌 채널에 메시지를 전송하려는 시도를 **수행하지 않아야 한다**. 전송 전 채널 상태를 확인하거나 recover로 panic을 격리해야 한다.

---

### Module 4: Backpressure - 백프레셔 타입 정의 (P0 타입 구현 완료, P1 로직 미구현)

#### REQ-ENGINE-001-04-01 (Ubiquitous) BackpressurePolicy 구조체

시스템은 **항상** 다음 필드를 포함하는 `BackpressurePolicy` 구조체를 제공해야 한다:

- `Strategy BackpressureStrategy` - 백프레셔 전략
- `BufferHighWaterMark float64` - 버퍼 고수위 마크 (0.0~1.0, 이 비율 초과 시 경고)
- `DropPolicy DropPolicy` - 버퍼 가득 참 시 드롭 정책

#### REQ-ENGINE-001-04-02 (Ubiquitous) BackpressureStrategy 타입

시스템은 **항상** `BackpressureStrategy` 타입과 다음 상수를 제공해야 한다:

- `StrategyBlock` - 블로킹 (기본값, Go 채널 자연적 백프레셔)
- `StrategyDrop` - 드롭 (DropPolicy에 따라 메시지 폐기)

#### REQ-ENGINE-001-04-03 (Ubiquitous) DropPolicy 타입

시스템은 **항상** `DropPolicy` 타입과 다음 상수를 제공해야 한다:

- `DropNewest` - 최신 메시지 폐기 (새로 도착한 메시지를 버림)
- `DropOldest` - 최고령 메시지 폐기 (버퍼에서 가장 오래된 메시지를 버림)

#### REQ-ENGINE-001-04-04 (State-Driven) Block 전략 시 자연적 백프레셔

**IF** 백프레셔 전략이 `StrategyBlock`일 때, **THEN** Go 채널의 자연적 블로킹으로 상류 노드의 출력 속도를 자동 제한해야 한다.

#### REQ-ENGINE-001-04-05 (State-Driven) Drop 전략 시 메시지 폐기

**IF** 백프레셔 전략이 `StrategyDrop`이고 버퍼가 가득 찬 상태일 때, **THEN** DropPolicy에 따라 메시지를 폐기하고, 폐기된 메시지 수를 메트릭에 기록해야 한다.

#### REQ-ENGINE-001-04-06 (Event-Driven) 고수위 마크 경고

**WHEN** Wire 버퍼 사용률이 `BufferHighWaterMark`를 초과하면, **THEN** 경고 로그를 생성하고 백프레셔 메트릭을 갱신해야 한다.

#### REQ-ENGINE-001-04-07 (Ubiquitous) DefaultBackpressurePolicy 함수

시스템은 **항상** `DefaultBackpressurePolicy() BackpressurePolicy` 함수를 제공하여 합리적인 기본값(Strategy: StrategyBlock, HighWaterMark: 0.8, DropPolicy: DropNewest)을 반환해야 한다.

---

### Module 5: State Management - 플로우 상태 관리 (P0, 구현 완료)

#### REQ-ENGINE-001-05-01 (Ubiquitous) FlowState-State 매핑 함수

시스템은 **항상** `MapFlowStateToLifecycleState(fs flow.FlowState) (lifecycle.State, error)` 함수를 제공하여 FlowState를 lifecycle.State로 매핑해야 한다:

| FlowState | lifecycle.State | 비고 |
|-----------|----------------|------|
| FlowStored | - | 영속화 상태, lifecycle 대응 없음 |
| FlowLoaded | StateCreated | 메모리에 로드됨, 초기화 준비 |
| FlowInitializing | StateInitializing | 초기화 진행 중 |
| FlowRunning | StateRunning | 실행 중 |
| FlowPaused | StatePaused | 일시정지 |
| FlowStopping | StateStopping | 중지 진행 중 |
| FlowStopped | StateStopped | 중지 완료 |
| FlowError | StateError | 에러 |

`FlowStored`는 lifecycle에 대응하는 상태가 없으므로 에러를 반환한다.

#### REQ-ENGINE-001-05-02 (Ubiquitous) LifecycleState-FlowState 역매핑 함수

시스템은 **항상** `MapLifecycleStateToFlowState(s lifecycle.State) flow.FlowState` 함수를 제공하여 lifecycle.State를 FlowState로 역매핑해야 한다.

#### REQ-ENGINE-001-05-03 (Event-Driven) 플로우 상태 전이 실행

**WHEN** Engine이 플로우의 상태를 전이할 때, **THEN** 다음 순서로 수행해야 한다:

1. `flow.IsValidTransition(currentFlowState, newFlowState)` 검증
2. 유효하면 `flow.SetState(newFlowState)` 호출
3. 대응하는 lifecycle.State가 있으면 `BaseLifecycle.TransitionTo()` 호출
4. `StateChangeCallback`을 통해 관찰성 시스템에 알림

#### REQ-ENGINE-001-05-04 (Unwanted) FlowState-lifecycle.State 불일치 방지

시스템은 FlowState와 대응하는 lifecycle.State가 불일치하는 상태를 **허용하지 않아야 한다**. 매핑 실패 시 `ErrStateMapping` 에러를 반환하고 상태 전이를 중단해야 한다.

#### REQ-ENGINE-001-05-05 (Event-Driven) Pause 전파

**WHEN** Engine이 `PauseFlow()`를 실행하면, **THEN** 해당 플로우의 모든 노드에 Pause 신호를 전파하여 각 노드가 입력 채널 읽기를 중단하도록 해야 한다.

#### REQ-ENGINE-001-05-06 (Event-Driven) Resume 후 큐 데이터 처리

**WHEN** Engine이 `ResumeFlow()`를 실행하면, **THEN** 각 노드는 채널에 큐잉된 메시지부터 순서대로 처리를 재개해야 한다.

---

### Module 6: TTL Management - 메시지 TTL 관리 (P1, 미구현)

#### REQ-ENGINE-001-06-01 (Event-Driven) Wire 전달 시 TTL 검사

**WHEN** Wire가 메시지를 전달하려 할 때 해당 Wire에 TTL이 설정되어 있으면, **THEN** 메시지의 생성 시각 + TTL과 현재 시각을 비교하여 만료 여부를 판정해야 한다.

#### REQ-ENGINE-001-06-02 (Event-Driven) 만료 메시지 Dead Letter 라우팅

**WHEN** TTL이 만료된 메시지가 발견되면, **THEN** 다음 순서로 처리해야 한다:

1. Dead Letter 노드가 연결되어 있으면: 만료 사유와 함께 해당 노드로 전달
2. Dead Letter 노드가 없으면: 메시지를 자동 폐기
3. 항상: 만료 메트릭 기록 (드롭 카운터, 로그)

#### REQ-ENGINE-001-06-03 (Ubiquitous) 버퍼 내 만료 메시지 주기적 스캔

시스템은 **항상** 버퍼 모드 Wire의 버퍼에서 만료된 메시지를 주기적으로 스캔하여 제거하는 별도 goroutine을 실행해야 한다.

- 스캔 주기: 설정 가능 (기본 1초)
- `context.Context` 기반 goroutine 종료 제어
- 플로우 정지 시 스캔 goroutine도 종료

#### REQ-ENGINE-001-06-04 (State-Driven) 바이패스 모드에서 TTL 검사 생략

**IF** Wire가 바이패스 모드(unbuffered)일 때, **THEN** TTL 검사를 수행하지 않아야 한다. 바이패스 모드는 즉시 전달이므로 TTL이 의미가 없다.

#### REQ-ENGINE-001-06-05 (Ubiquitous) TTLScanner 구조체

시스템은 **항상** 다음 기능을 포함하는 `TTLScanner` 구조체를 제공해야 한다:

- 스캔 대상 RuntimeWire 목록 관리
- 스캔 주기 설정 (기본 1초)
- Dead Letter 라우터 참조
- 메트릭 카운터 (만료 메시지 수, 폐기 메시지 수)

---

### Module 7: Error Types - 에러 타입 (P0, 구현 완료)

#### REQ-ENGINE-001-07-01 (Ubiquitous) 표준 에러 변수

시스템은 **항상** 다음 에러 변수를 제공해야 한다:

| 에러 변수 | 용도 |
|-----------|------|
| `ErrFlowAlreadyDeployed` | 동일 ID로 이미 배포된 플로우 존재 시 |
| `ErrFlowNotFound` | 지정된 ID의 플로우를 찾을 수 없을 때 |
| `ErrFlowNotRunning` | Running 상태가 아닌 플로우에 Pause 시도 시 |
| `ErrFlowNotPaused` | Paused 상태가 아닌 플로우에 Resume 시도 시 |
| `ErrFlowNotStopped` | Stopped 상태가 아닌 플로우에 Undeploy 시도 시 |
| `ErrFlowNotLoaded` | Loaded 상태가 아닌 플로우에 Start 시도 시 |
| `ErrFlowValidationFailed` | 유효성 검증 실패한 플로우 배포 시도 시 |
| `ErrCycleDetected` | DAG에 순환이 감지되었을 때 |
| `ErrStateMapping` | FlowState-lifecycle.State 매핑 실패 시 |
| `ErrChannelClosed` | 닫힌 채널에 전송 시도 시 |
| `ErrNodeStartFailed` | 노드 시작 실패 시 |
| `ErrShutdownTimeout` | Graceful Shutdown 타임아웃 시 |

#### REQ-ENGINE-001-07-02 (Ubiquitous) 에러 래핑 지원

시스템은 **항상** 모든 에러 변수가 `errors.Is()` 및 `errors.As()`와 호환되도록 sentinel error 패턴을 사용해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/engine/
  engine.go            # Engine 구조체, NewEngine(), DeployFlow(), StartFlow(), StopFlow(),
                       #   PauseFlow(), ResumeFlow(), UndeployFlow(), GetFlowStatus(), ListFlows()
  scheduler.go         # Scheduler 인터페이스, DAGScheduler, ExecutionPlan, Plan()
  wire.go              # RuntimeWire, CreateRuntimeWires(), Fan-out/Fan-in, 수신 노드 상태별 전달
  backpressure.go      # BackpressurePolicy, Strategy, DropPolicy, DefaultBackpressurePolicy()
  state.go             # MapFlowStateToLifecycleState(), MapLifecycleStateToFlowState(), 상태 전이 실행
  ttl.go               # TTLScanner, 만료 검사, Dead Letter 라우팅, 주기적 버퍼 스캔
  errors.go            # 에러 변수 정의
  options.go           # EngineOption, WithLogger(), WithMetrics(), WithConfig() 등
  types.go             # FlowStatus, flowRuntime 등 내부 타입

  engine_test.go       # Engine 통합 테스트 (Deploy/Start/Stop/Pause/Resume)
  scheduler_test.go    # DAG 위상 정렬, 순환 감지 테스트
  wire_test.go         # RuntimeWire 생성, 모드별 전달, Fan-out/Fan-in 테스트
  backpressure_test.go # 백프레셔 전략, 드롭 정책 테스트
  state_test.go        # FlowState-lifecycle.State 매핑 테스트
  ttl_test.go          # TTL 만료, Dead Letter 라우팅, 버퍼 스캔 테스트
```

### 4.2 타입 시그니처

```go
// Engine Core
type Engine struct {
    *lifecycle.BaseLifecycle // 임베딩
    // unexported fields: registry, scheduler, logger, metrics, config
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

// Configurable 구현
func (e *Engine) Configure(ctx context.Context, cfg map[string]any) error
func (e *Engine) GetConfig() map[string]any

// HealthChecker 구현
func (e *Engine) HealthCheck(ctx context.Context) lifecycle.HealthStatus

// Scheduler
type Scheduler interface {
    Plan(f flow.Flow) (ExecutionPlan, error)
}

type ExecutionPlan struct {
    Levels [][]string // 레벨별 노드 ID 목록
    Order  []string   // 전체 실행 순서
}

type DAGScheduler struct{}
func NewDAGScheduler() *DAGScheduler
func (s *DAGScheduler) Plan(f flow.Flow) (ExecutionPlan, error)

// Wire System
type RuntimeWire struct {
    ID           string
    SourceNodeID string
    SourcePort   string
    TargetNodeID string
    TargetPort   string
    Mode         flow.WireMode
    BufferSize   int
    TTL          time.Duration
    Ch           chan message.Message
}

func CreateRuntimeWires(wires []flow.Wire) ([]*RuntimeWire, error)

// Backpressure
type BackpressureStrategy string
const (
    StrategyBlock BackpressureStrategy = "block"
    StrategyDrop  BackpressureStrategy = "drop"
)

type DropPolicy string
const (
    DropNewest DropPolicy = "drop_newest"
    DropOldest DropPolicy = "drop_oldest"
)

type BackpressurePolicy struct {
    Strategy           BackpressureStrategy
    BufferHighWaterMark float64
    DropPolicy         DropPolicy
}

func DefaultBackpressurePolicy() BackpressurePolicy

// State Management
func MapFlowStateToLifecycleState(fs flow.FlowState) (lifecycle.State, error)
func MapLifecycleStateToFlowState(s lifecycle.State) flow.FlowState

// TTL
type TTLScanner struct {
    // unexported fields
}

func NewTTLScanner(interval time.Duration, opts ...TTLScannerOption) *TTLScanner
func (s *TTLScanner) Start(ctx context.Context)
func (s *TTLScanner) Stop()
func (s *TTLScanner) AddWire(w *RuntimeWire)
func (s *TTLScanner) RemoveWire(wireID string)

// FlowStatus
type FlowStatus struct {
    FlowID        string
    FlowName      string
    State         flow.FlowState
    NodeCount     int
    ActiveNodes   int
    WireCount     int
    MessageCount  int64  // 누적 처리 메시지 수
    ErrorCount    int64  // 누적 에러 수
    DroppedCount  int64  // 폐기된 메시지 수
    StartedAt     time.Time
    Uptime        time.Duration
}

// Options
type EngineOption func(*Engine)
func WithLogger(logger observe.Logger) EngineOption
func WithMetrics(metrics observe.Metrics) EngineOption
func WithConfig(cfg config.Config) EngineOption
func WithScheduler(s Scheduler) EngineOption
func WithBackpressurePolicy(p BackpressurePolicy) EngineOption
func WithShutdownTimeout(d time.Duration) EngineOption

// 에러
var (
    ErrFlowAlreadyDeployed  = errors.New("engine: flow already deployed")
    ErrFlowNotFound         = errors.New("engine: flow not found")
    ErrFlowNotRunning       = errors.New("engine: flow not running")
    ErrFlowNotPaused        = errors.New("engine: flow not paused")
    ErrFlowNotStopped       = errors.New("engine: flow not stopped")
    ErrFlowNotLoaded        = errors.New("engine: flow not loaded")
    ErrFlowValidationFailed = errors.New("engine: flow validation failed")
    ErrCycleDetected        = errors.New("engine: cycle detected in flow graph")
    ErrStateMapping         = errors.New("engine: flow state to lifecycle state mapping failed")
    ErrChannelClosed        = errors.New("engine: channel closed")
    ErrNodeStartFailed      = errors.New("engine: node start failed")
    ErrShutdownTimeout      = errors.New("engine: shutdown timeout exceeded")
)
```

### 4.3 FlowState <-> lifecycle.State 매핑 다이어그램

```
pkg/flow/FlowState (8상태)          internal/engine/          pkg/lifecycle/State (7상태)
========================          =================          ==========================
FlowStored --------- (매핑 없음, 영속화 전용)
FlowLoaded ----------> MapFlowStateToLifecycleState() --> StateCreated
FlowInitializing ----> MapFlowStateToLifecycleState() --> StateInitializing
FlowRunning ---------> MapFlowStateToLifecycleState() --> StateRunning
FlowPaused ----------> MapFlowStateToLifecycleState() --> StatePaused
FlowStopping --------> MapFlowStateToLifecycleState() --> StateStopping
FlowStopped ---------> MapFlowStateToLifecycleState() --> StateStopped
FlowError -----------> MapFlowStateToLifecycleState() --> StateError
```

### 4.4 수신 노드 상태별 Wire 전달 동작

| 수신 노드 상태 | 바이패스 Wire (unbuffered) | 버퍼 Wire (buffered) |
|---------------|--------------------------|---------------------|
| Running | 즉시 전달 (receiver 준비 시) | 즉시 전달 또는 버퍼에 추가 |
| Paused | 송신 노드 블로킹 (대기) | 버퍼에 축적, 가득 차면 블로킹 |
| Stopped | 에러 포트로 라우팅 | 에러 포트로 라우팅 |

### 4.5 Graceful Shutdown 순서

```
1. FlowState -> FlowStopping 전이
2. 소스 노드(입력 Wire 없는 노드)의 입력 채널 닫기
3. 각 노드가 입력 채널 고갈(drain) 후 자연 종료
4. 종료 전파: 상류 -> 하류 순서로 채널 닫힘 전파
5. 모든 노드 goroutine 종료 대기 (context 타임아웃)
6. 잔여 Wire 채널 닫기 및 드레인
7. TTLScanner 종료
8. FlowState -> FlowStopped 전이
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 | 상태 |
|------------|------|------|---------|------|
| REQ-ENGINE-001-01-01 ~ 01-12 | Engine Core | engine.go, options.go, types.go | P0 | 구현 완료 |
| REQ-ENGINE-001-02-01 ~ 02-05 | Scheduler | scheduler.go | P0 | 구현 완료 |
| REQ-ENGINE-001-03-01 ~ 03-08 | Wire System | wire.go | P0 | 구현 완료 |
| REQ-ENGINE-001-04-01 ~ 04-07 | Backpressure | backpressure.go | P0(타입)/P1(로직) | 타입 정의 완료, 런타임 로직 미구현 |
| REQ-ENGINE-001-05-01 ~ 05-06 | State Management | state.go | P0 | 구현 완료 |
| REQ-ENGINE-001-06-01 ~ 06-05 | TTL Management | ttl.go | P1 | 미구현 |
| REQ-ENGINE-001-07-01 ~ 07-02 | Error Types | errors.go | P0 | 구현 완료 |

### 구현 참조

- **구현 커밋**: `f948296`
- **구현 일자**: 2026-02-16
- **P0 구현 범위**: Module 1 (Engine Core), Module 2 (Scheduler), Module 3 (Wire System), Module 4 (Backpressure 타입), Module 5 (State Management), Module 7 (Error Types)
- **P1 미구현**: Module 4 (Backpressure 런타임 로직 - Drop 전략, 고수위 마크 모니터링), Module 6 (TTL Management - TTLScanner, Dead Letter 라우팅)
