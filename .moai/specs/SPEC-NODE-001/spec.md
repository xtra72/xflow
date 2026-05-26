---
id: SPEC-NODE-001
version: "1.5.0"
status: completed
created: "2026-02-13"
updated: "2026-05-26"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-05-26 | 1.5.0 | **BREAKING — store-read `entries_field` 배치 모드 객체 배열 지원**. (1) `processBatch` 가 배열 요소를 `fmt.Sprint(elem)` 으로 string 화하던 동작 제거 — primitive 든 객체든 원본 그대로 `entries_var` payload 키에 주입. 객체일 때 `{entries_var.field}` 또는 `{entries_var.nested.field}` dot notation 으로 nested 접근 가능. (2) `resolveTemplateExpr` legacy 분기 (`$.` prefix 없는 form) 확장 — 단일 segment 는 기존 flat lookup 유지, dot 포함 시 `lookupPayloadPath` 로 traverse. (3) **BREAKING**: 결과 map (`output_key`) 의 키를 elemStr (e.g., `"room1"`) 에서 resolved store 키 (e.g., `"device.room1.temp"`) 로 변경. primitive·객체 모든 케이스에서 일관. 다운스트림 (chart-emitter 등) 은 map 키를 임의로 사용하므로 영향 없음. 사용자 코드가 `result["room1"]` 형태로 직접 접근하던 케이스는 `result["device.room1.temp"]` 로 마이그레이션 필요. (4) 신규 테스트 3건: `PrimitiveStrings`, `ObjectElements`, `ObjectNestedField`. 기존 `TestStoreReadNode_EntriesField_ObjectElements` 는 사실상 primitive 만 검증하던 misnamed 테스트였음 — `PrimitiveStrings` 로 분리하고 진짜 객체 테스트 신설. |
| 2026-05-23 | 1.4.0 | **deduplicate 노드 강화**. (1) `compare_fields` 가 array (table) 형식 지원 — `[{name: "X", tolerance: 0.5}, {name: "Y"}]`. 레거시 string ("X:0.5, Y") 형식 호환 유지. `parseCompareFieldsArray` 헬퍼 신설. (2) `missing_field_as_different: bool` (기본 false) 옵션 신설 — 활성 시 신규 메시지의 비교 필드 중 하나라도 부재하면 즉시 "다름" 으로 판정하여 통과 (중복 폐기 안 함). `extractValuesWithMissing` 헬퍼로 부재 필드 감지. (3) Web 측 `CompareFieldsEditor` 컴포넌트 신설 — 필드명 / 허용오차 2-칼럼 테이블 편집, 행 추가/삭제, 레거시 string 자동 파싱 + array 양방향 변환. `FormField` 에 `compare_fields` 타입 추가. 단위테스트 3건 추가 (array 형식 / missing=true / missing=false). |
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |
| 2026-03-17 | 1.1.0 | output 노드 타입 추가: Go text/template 기반 메시지 포맷팅 출력 (pass-through). 카테고리: debug |
| 2026-03-30 | 1.3.0 | NodeDef.Enabled 필드 추가: nil=활성(기본), false=비활성. IsEnabled() 메서드, WithEnabled() 옵션. Engine runNode()에서 비활성 노드 메시지 드레인(SourceNode/ProcessNode 모두 처리). flowRuntime.disabledNodes set. React Flow normalizeReactFlowDefinition enabled 필드 양방향 변환. pkg/flow/node_test.go, internal/engine/engine_test.go 테스트 추가 |
| 2026-03-30 | 1.2.0 | store-write, store-read 노드 타입 추가: Store Agent 연동 키-값 저장소 읽기/쓰기 노드. store-write(key_template, namespace, ttl 설정, agent_ref로 StoreAgent 참조), store-read(key 또는 key_pattern, namespace 설정). 카테고리: storage. NodeRegistry RegisterDefaults()에 등록 |

---

# SPEC-NODE-001: Node System - FBP 노드 인터페이스, 기반 구현체, 레지스트리, 포트 시스템, 내장 노드 타입

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 핵심 실행 단위인 Node 시스템을 정의한다. Node는 Flow 내에서 데이터를 수신, 처리, 출력하는 독립 실행 컴포넌트이며, 각 노드는 Engine에 의해 별도 goroutine으로 실행된다. 본 SPEC은 모든 노드가 공유하는 공통 인터페이스, 기반 구현체, 노드 타입 레지스트리, 포트 시스템, 그리고 내장 노드 타입 구현을 다룬다.

본 SPEC은 다음을 포함한다:

- **Node Interface & BaseNode** (`base.go`): 모든 노드가 구현하는 `Node` 인터페이스와 공통 기능을 제공하는 `BaseNode` 기반 구조체
- **Node Registry** (`registry.go`): 노드 타입 이름으로 팩토리 함수를 등록/조회하는 중앙 레지스트리
- **Port System** (`base.go` 내 포트 관리): 입력/출력/에러 포트 런타임 관리, `pkg/flow/Port` 데이터 구조 소비
- **Filter Node** (`filter.go`): 조건 기반 메시지 필터링
- **Transform Node** (`transform.go`): 메시지 Payload 변환
- **Switch Node** (`switch.go`): 조건별 라우팅 (다중 출력 포트)
- **Aggregate Node** (`aggregate.go`): 다중 메시지 집계
- **Bridge Node** (`bridge.go`): Agent-Flow 양방향 연결 (In/Out/InOut/Request-Reply 4모드)
- **Script Node** (`script.go`): Lua 스크립트 동적 실행 (`internal/script/` 연동)
- **Debug Node** (`debug.go`): 메시지 내용 로깅 및 디버깅
- **Catch Node** (`catch.go`): 에러 포트 메시지 수신 및 에러 처리
- **Status Node** (`status.go`): 플로우/노드 상태 이벤트 모니터링
- **Dead Letter Node** (`deadletter.go`): TTL 만료/배달 불가 메시지 수집
- **Error Types** (`errors.go`): 노드 패키지 전용 sentinel 에러 정의

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/node/`
- **Tier**: internal (비공개 패키지)
- **의존 패키지**:
  - `pkg/lifecycle/` (SPEC-LIFE-001): `Lifecycle`, `BaseLifecycle`, `Configurable`, `State` 임베딩
  - `pkg/flow/` (SPEC-FLOW-001): `NodeDef`, `Port`, `PortDirection`, `AgentRef`, `BridgeDirection` 데이터 구조
  - `pkg/message/` (SPEC-MSG-001): `Message`, `Payload`, `Metadata` 인터페이스 (노드 입출력 데이터)
  - `internal/observe/` (SPEC-OBS-001): `Logger`, `Metrics`, `Trace` 관찰성 통합
  - `internal/config/` (SPEC-CFG-001): `Config`, `HotReload` 런타임 설정 변경
  - `internal/engine/` (SPEC-ENGINE-001): Engine이 Node 인터페이스를 소비하여 goroutine 실행
  - `internal/script/` (별도 SPEC 예정): Script Node의 Lua 실행 엔진 (GopherLua)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: 노드당 1 goroutine (Engine이 관리), 노드 내부 상태는 `sync.Mutex` 보호

### 1.3 설계 원칙

- **인터페이스 우선**: `Node` 인터페이스로 모든 노드 타입의 공통 계약을 정의하며, Engine은 이 인터페이스만 의존
- **임베딩 패턴**: `BaseNode`가 `pkg/lifecycle/BaseLifecycle`을 임베딩하여 공통 생명주기 로직 재사용
- **팩토리 레지스트리**: Node Registry가 노드 타입 이름 -> 팩토리 함수 매핑을 관리하여 런타임 노드 인스턴스 생성
- **에러 출력 포트**: 모든 노드는 표준 출력 포트 외에 에러 출력 포트를 가지며, Process 실패 시 에러 메시지를 에러 포트로 전달
- **Mutable Message**: 노드는 수신 메시지의 Payload를 Add/Set/Delete/Get으로 변형 가능
- **Hot Configuration**: 노드 설정을 런타임에 변경 가능 (재시작 없이)
- **관찰성 내장**: 모든 노드의 Process 호출에 대해 로그/메트릭/트레이스 자동 생성
- **동시성 안전**: 노드 내부 상태(설정, 포트 매핑)는 `sync.Mutex`로 보호

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- Node 인터페이스 정의 (Init, Process, Pause, Resume, Shutdown, Configure, Ports)
- BaseNode 기반 구조체 (공통 생명주기, 포트 관리, 에러 포트 전달)
- Node Registry (팩토리 등록, 조회, 노드 인스턴스 생성)
- Port 런타임 관리 (입력/출력/에러 포트 바인딩)
- 내장 노드 타입 10종 (Filter, Transform, Switch, Aggregate, Bridge, Script, Debug, Catch, Status, Dead Letter)
- 노드 패키지 전용 에러 타입

**OUT OF SCOPE (별도 SPEC)**:
- 노드 goroutine 실행 루프 (SPEC-ENGINE-001: `internal/engine/`)
- Wire 채널 생성 및 메시지 전달 (SPEC-ENGINE-001: Wire System)
- Flow 데이터 구조 정의 (SPEC-FLOW-001: `pkg/flow/`)
- Message 데이터 구조 정의 (SPEC-MSG-001: `pkg/message/`)
- 공통 생명주기 인터페이스 (SPEC-LIFE-001: `pkg/lifecycle/`)
- Lua 스크립트 엔진 구현 (별도 SPEC: `internal/script/`)
- Agent 생명주기 관리 (별도 SPEC: `internal/agent/`)
- 관찰성 시스템 구현 (SPEC-OBS-001: `internal/observe/`)
- 설정 시스템 구현 (SPEC-CFG-001: `internal/config/`)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-LIFE-001 | 의존 | `Lifecycle`, `BaseLifecycle`, `Configurable`, `State` 임베딩 및 구현 |
| SPEC-FLOW-001 | 의존 | `NodeDef`, `Port`, `PortDirection`, `AgentRef`, `BridgeDirection` 데이터 구조 소비 |
| SPEC-MSG-001 | 의존 | `Message`, `Payload`, `Metadata` 인터페이스 (노드 Process 입출력) |
| SPEC-ENGINE-001 | 소비자 | Engine이 `Node` 인터페이스를 통해 goroutine 실행, Process 호출 |
| SPEC-OBS-001 | 소비자 | 노드가 생성하는 로그, 메트릭, 트레이스를 관찰성 시스템에 전달 |
| SPEC-CFG-001 | 소비자 | 노드 런타임 설정 읽기 및 Hot Reload 수신 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: Engine이 각 노드를 별도 goroutine으로 실행하며, 노드의 `Process()` 메서드는 단일 goroutine에서 순차 호출된다
- A2: 노드 간 메시지 전달은 Engine의 Wire 시스템이 담당하며, 노드는 Wire 채널에 직접 접근하지 않는다
- A3: `pkg/lifecycle/BaseLifecycle`의 상태 전이 로직은 thread-safe하다 (SPEC-LIFE-001 보장)
- A4: `pkg/message/Message`의 Payload 변형 연산(Add/Set/Delete)은 단일 goroutine에서 호출된다 (SPEC-MSG-001 가정)
- A5: `pkg/flow/NodeDef`에 정의된 Port 목록이 노드의 런타임 포트 구성의 원천이다
- A6: `internal/script/` 패키지가 Lua 스크립트 실행 환경(GopherLua)을 제공하며, Script Node는 이를 소비한다
- A7: 노드 설정은 `map[string]any` 형태로 전달되며, 각 노드 타입이 자체적으로 타입 변환 및 검증을 수행한다
- A8: Bridge Node가 참조하는 Agent는 `internal/agent/` 패키지에서 관리되며, 이름 또는 ID로 조회 가능하다

### 2.2 도메인 가정

- A9: 모든 노드는 최소 하나의 출력 포트(표준 출력 또는 에러 출력)를 가진다
- A10: 에러 출력 포트에 Wire가 연결되지 않은 경우, 에러 메시지는 자동 폐기되며 로그와 메트릭만 기록한다
- A11: Bridge Node의 Request-Reply 모드에서 Correlation ID는 UUID 기반이며, `sync.Map`으로 관리한다
- A12: Bridge Node의 Request-Reply 타임아웃은 기본 30초이며, 노드 설정으로 변경 가능하다
- A13: Script Node의 Lua 스크립트는 Hot Reload를 지원하며, 스크립트 변경 시 다음 Process 호출부터 적용한다
- A14: Aggregate Node는 윈도우 기반(시간 또는 카운트)으로 메시지를 모은 후 집계 결과를 출력한다
- A15: Switch Node는 첫 번째 매칭 라우트로 전달하며(first-match), 기본 라우트(default)를 지원한다
- A16: Dead Letter Node는 TTL 만료, 배달 불가, 최대 재시도 초과 메시지를 수집하며, 원인 정보를 Metadata에 첨부한다

---

## 3. Requirements (요구사항)

### Module 1: Node Interface & BaseNode - 노드 인터페이스 및 기반 구현체 (P0)

#### REQ-NODE-001-01-01 (Ubiquitous) Node 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 `Node` 인터페이스를 제공해야 한다:

- `ID() string`: 노드 고유 ID 반환
- `Name() string`: 노드 표시 이름 반환
- `Type() string`: 노드 타입 이름 반환 (예: "filter", "transform", "bridge")
- `Init(ctx context.Context) error`: 노드 초기화 (리소스 할당, 포트 바인딩)
- `Process(ctx context.Context, msg message.Message) ([]message.Message, error)`: 메시지 처리 및 결과 반환
- `Shutdown(ctx context.Context) error`: 노드 종료 (리소스 해제)
- `Configure(config map[string]any) error`: 런타임 설정 변경 (Hot Configuration)
- `Ports() []Port`: 노드에 바인딩된 포트 목록 반환

#### REQ-NODE-001-01-02 (Ubiquitous) BaseNode 기반 구조체

시스템은 **항상** 다음 기능을 제공하는 `BaseNode` 구조체를 제공해야 한다:

- `pkg/lifecycle/BaseLifecycle` 임베딩 (상태 관리 재사용)
- 노드 ID, Name, Type 필드 보유
- 입력 포트, 출력 포트, 에러 포트 맵 관리
- 공통 Configure/GetConfig 구현 (`sync.Mutex` 보호)
- 공통 Ports() 구현 (바인딩된 포트 목록 반환)
- Logger, Metrics 참조 보유 (관찰성 통합)

#### REQ-NODE-001-01-03 (Ubiquitous) NewBaseNode() 생성자

시스템은 **항상** `NewBaseNode(def flow.NodeDef, opts ...NodeOption) *BaseNode` 생성자를 제공해야 한다:

- `flow.NodeDef`에서 ID, Name, Type, Config, Ports 정보를 추출하여 초기화
- `BaseLifecycle` 초기 상태: `StateCreated`
- Options Pattern으로 Logger, Metrics 주입
- 입력/출력/에러 포트를 `flow.NodeDef.Inputs`, `flow.NodeDef.Outputs`, `flow.NodeDef.ErrorPort`에서 바인딩

#### REQ-NODE-001-01-04 (Event-Driven) 에러 출력 포트 전달

**WHEN** `Node.Process()` 실행 중 에러가 발생하면, **THEN** 다음을 수행해야 한다:

1. 에러 정보를 포함한 에러 메시지 생성 (원본 메시지 ID, 에러 내용, 노드 ID를 Metadata에 첨부)
2. 에러 출력 포트로 에러 메시지 전달
3. 에러 메트릭 카운터 증가
4. 에러 로그 기록

#### REQ-NODE-001-01-05 (Event-Driven) Hot Configuration 적용

**WHEN** `Node.Configure(config map[string]any)` 호출 시 유효한 설정이 전달되면, **THEN** 다음을 수행해야 한다:

1. 설정 값 타입 검증
2. `sync.Mutex`로 기존 설정 교체
3. 다음 Process 호출부터 새 설정 적용
4. 설정 변경 로그 기록

#### REQ-NODE-001-01-06 (Unwanted) 잘못된 설정 거부

시스템은 `Node.Configure()`에 유효하지 않은 설정(필수 키 누락, 타입 불일치)이 전달되면 **에러를 반환하고 기존 설정을 유지해야 한다**.

#### REQ-NODE-001-01-07 (Event-Driven) 노드 초기화

**WHEN** `Node.Init(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 상태를 `StateInitializing`으로 전이
2. 노드 타입별 리소스 초기화 (Script Node: Lua VM, Bridge Node: Agent 연결 등)
3. 포트 바인딩 검증
4. 초기화 성공 시 상태를 `StateRunning`으로 전이
5. 초기화 실패 시 상태를 `StateError`로 전이 및 에러 반환

#### REQ-NODE-001-01-08 (Event-Driven) 노드 종료

**WHEN** `Node.Shutdown(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 상태를 `StateStopping`으로 전이
2. 노드 타입별 리소스 해제 (Script Node: Lua VM 종료, Bridge Node: Agent 연결 해제 등)
3. 포트 바인딩 해제
4. 상태를 `StateStopped`으로 전이

---

### Module 2: Node Registry - 노드 레지스트리 (P0)

#### REQ-NODE-001-02-01 (Ubiquitous) Registry 구조체 정의

시스템은 **항상** 노드 타입 이름(`string`) -> 팩토리 함수(`NodeFactory`) 매핑을 관리하는 `Registry` 구조체를 제공해야 한다:

- `sync.RWMutex` 기반 동시성 안전 보장
- 내부 맵으로 팩토리 함수 저장

#### REQ-NODE-001-02-02 (Ubiquitous) NodeFactory 타입 정의

시스템은 **항상** `type NodeFactory func(def flow.NodeDef, opts ...NodeOption) (Node, error)` 팩토리 함수 타입을 제공해야 한다.

#### REQ-NODE-001-02-03 (Event-Driven) 노드 타입 등록

**WHEN** `Registry.Register(typeName string, factory NodeFactory)` 호출 시, **THEN** 해당 타입 이름으로 팩토리 함수를 등록해야 한다.

#### REQ-NODE-001-02-04 (Unwanted) 중복 타입 등록 거부

시스템은 이미 등록된 타입 이름으로 `Registry.Register()`를 재호출하면 `ErrNodeTypeAlreadyRegistered` 에러를 반환하고 **기존 등록을 유지해야 한다**.

#### REQ-NODE-001-02-05 (Event-Driven) 노드 인스턴스 생성

**WHEN** `Registry.Create(def flow.NodeDef, opts ...NodeOption) (Node, error)` 호출 시 해당 타입이 등록되어 있으면, **THEN** 팩토리 함수를 호출하여 노드 인스턴스를 반환해야 한다.

#### REQ-NODE-001-02-06 (Unwanted) 미등록 타입 생성 거부

시스템은 등록되지 않은 타입 이름으로 `Registry.Create()`를 호출하면 `ErrNodeTypeNotFound` 에러를 반환**해야 한다**.

#### REQ-NODE-001-02-07 (Ubiquitous) NewRegistry() 생성자 및 내장 타입 자동 등록

시스템은 **항상** `NewRegistry(opts ...RegistryOption) *Registry` 생성자를 제공하며, 기본 옵션으로 내장 노드 타입 10종(filter, transform, switch, aggregate, bridge, script, debug, catch, status, deadletter)을 자동 등록해야 한다.

#### REQ-NODE-001-02-08 (Ubiquitous) 등록된 타입 목록 조회

시스템은 **항상** `Registry.Types() []string` 메서드를 제공하여 등록된 모든 노드 타입 이름 목록을 반환해야 한다.

---

### Module 3: Port System - 포트 시스템 (P0)

#### REQ-NODE-001-03-01 (Ubiquitous) Port 런타임 구조체

시스템은 **항상** 노드의 런타임 포트를 관리하는 `Port` 구조체를 제공해야 한다:

- `ID string`: 포트 고유 ID
- `Name string`: 포트 표시 이름
- `Direction PortDirection`: 방향 (Input/Output/Error)
- `Connected bool`: Wire 연결 상태

#### REQ-NODE-001-03-02 (Ubiquitous) PortDirection 타입

시스템은 **항상** `PortDirection` 타입과 다음 상수를 제공해야 한다:

- `PortInput`: 입력 포트
- `PortOutput`: 출력 포트
- `PortError`: 에러 출력 포트

#### REQ-NODE-001-03-03 (Event-Driven) 포트 바인딩

**WHEN** `BaseNode.Init()` 실행 시, **THEN** `flow.NodeDef`의 Inputs/Outputs/ErrorPort 정보를 기반으로 런타임 `Port` 인스턴스를 생성하고 내부 맵에 바인딩해야 한다.

#### REQ-NODE-001-03-04 (Ubiquitous) 기본 에러 포트 보장

시스템은 **항상** 모든 노드에 최소 하나의 에러 출력 포트(`_error`)를 보장해야 한다. `flow.NodeDef.ErrorPort`가 nil인 경우에도 기본 에러 포트를 자동 생성한다.

#### REQ-NODE-001-03-05 (Event-Driven) 포트 검증

**WHEN** 노드가 Process 결과를 특정 출력 포트로 전달하려 할 때, 해당 포트가 존재하지 않으면, **THEN** `ErrPortNotFound` 에러를 반환해야 한다.

---

### Module 4: Filter Node - 필터 노드 (P1)

#### REQ-NODE-001-04-01 (Ubiquitous) FilterNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 조건 기반 메시지 필터링을 수행하는 `FilterNode` 구조체를 제공해야 한다.

#### REQ-NODE-001-04-02 (Event-Driven) 필터 조건 통과

**WHEN** `FilterNode.Process(ctx, msg)` 호출 시 메시지가 필터 조건을 만족하면, **THEN** 메시지를 출력 포트로 전달해야 한다.

#### REQ-NODE-001-04-03 (Event-Driven) 필터 조건 미통과

**WHEN** `FilterNode.Process(ctx, msg)` 호출 시 메시지가 필터 조건을 만족하지 않으면, **THEN** 빈 결과를 반환해야 한다 (메시지 폐기, 에러 아님).

#### REQ-NODE-001-04-04 (Event-Driven) 필터 조건 설정

**WHEN** `FilterNode.Configure(config)` 호출 시, **THEN** `"condition"` 키로 필터 조건(JSONPath 표현식 또는 함수)을 설정해야 한다.

---

### Module 5: Transform Node - 변환 노드 (P1)

#### REQ-NODE-001-05-01 (Ubiquitous) TransformNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 메시지 Payload 변환을 수행하는 `TransformNode` 구조체를 제공해야 한다.

#### REQ-NODE-001-05-02 (Event-Driven) 메시지 변환 실행

**WHEN** `TransformNode.Process(ctx, msg)` 호출 시, **THEN** 설정된 변환 규칙에 따라 메시지의 Payload를 변형하고 결과를 출력 포트로 전달해야 한다.

#### REQ-NODE-001-05-03 (Event-Driven) 변환 실패 시 에러 포트 전달

**WHEN** `TransformNode.Process()` 실행 중 변환 규칙 적용에 실패하면, **THEN** 에러 메시지를 에러 출력 포트로 전달해야 한다.

#### REQ-NODE-001-05-04 (Event-Driven) 변환 규칙 설정

**WHEN** `TransformNode.Configure(config)` 호출 시, **THEN** `"rules"` 키로 변환 규칙 목록(소스 필드, 대상 필드, 변환 함수)을 설정해야 한다.

---

### Module 6: Switch Node - 스위치 노드 (P1)

#### REQ-NODE-001-06-01 (Ubiquitous) SwitchNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 조건별 라우팅을 수행하는 `SwitchNode` 구조체를 제공해야 한다. 다중 출력 포트를 지원한다.

#### REQ-NODE-001-06-02 (Event-Driven) 조건 매칭 라우팅 (First-Match)

**WHEN** `SwitchNode.Process(ctx, msg)` 호출 시, **THEN** 설정된 라우팅 규칙을 순서대로 평가하여 첫 번째 매칭 조건의 출력 포트로 메시지를 전달해야 한다.

#### REQ-NODE-001-06-03 (Event-Driven) 기본 라우트 (Default)

**WHEN** `SwitchNode.Process()` 실행 시 어떤 조건도 매칭되지 않고 기본 라우트가 설정되어 있으면, **THEN** 기본 라우트의 출력 포트로 메시지를 전달해야 한다.

#### REQ-NODE-001-06-04 (Event-Driven) 매칭 실패 시 폐기

**WHEN** `SwitchNode.Process()` 실행 시 어떤 조건도 매칭되지 않고 기본 라우트가 없으면, **THEN** 메시지를 폐기하고 메트릭(드롭 카운터)을 증가시켜야 한다.

#### REQ-NODE-001-06-05 (Event-Driven) 라우팅 규칙 설정

**WHEN** `SwitchNode.Configure(config)` 호출 시, **THEN** `"routes"` 키로 라우팅 규칙 목록(조건 표현식, 대상 포트 이름)을 설정해야 한다.

---

### Module 7: Aggregate Node - 집계 노드 (P2)

#### REQ-NODE-001-07-01 (Ubiquitous) AggregateNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 다중 메시지 집계를 수행하는 `AggregateNode` 구조체를 제공해야 한다.

#### REQ-NODE-001-07-02 (Event-Driven) 카운트 기반 윈도우

**WHEN** `AggregateNode.Process(ctx, msg)` 호출 시 집계 윈도우 방식이 `count`이면, **THEN** 설정된 카운트만큼 메시지가 모일 때까지 내부 버퍼에 축적하고, 카운트 도달 시 집계 함수를 실행하여 결과를 출력해야 한다.

#### REQ-NODE-001-07-03 (Event-Driven) 시간 기반 윈도우

**WHEN** `AggregateNode` 집계 윈도우 방식이 `time`이면, **THEN** 설정된 시간 간격이 경과할 때 내부 버퍼에 축적된 메시지에 대해 집계 함수를 실행하여 결과를 출력해야 한다.

#### REQ-NODE-001-07-04 (Event-Driven) 집계 윈도우 설정

**WHEN** `AggregateNode.Configure(config)` 호출 시, **THEN** `"window_type"` (count/time), `"window_size"` (카운트 또는 시간 간격), `"aggregate_fn"` (집계 함수 이름: sum, avg, count, min, max, first, last) 키로 집계 설정을 구성해야 한다.

#### REQ-NODE-001-07-05 (Event-Driven) 종료 시 잔여 버퍼 플러시

**WHEN** `AggregateNode.Shutdown(ctx)` 호출 시 내부 버퍼에 메시지가 남아 있으면, **THEN** 잔여 메시지에 대해 집계 함수를 실행하여 결과를 출력한 후 종료해야 한다.

---

### Module 8: Bridge Node - 브릿지 노드 (P1)

#### REQ-NODE-001-08-01 (Ubiquitous) BridgeNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 Agent-Flow 양방향 연결을 수행하는 `BridgeNode` 구조체를 제공해야 한다:

- `flow.AgentRef` 기반 Agent 참조 (AgentID 또는 AgentName)
- `flow.BridgeDirection` 기반 4가지 통신 모드 지원

#### REQ-NODE-001-08-02 (State-Driven) BridgeIn 모드

**IF** `BridgeDirection`이 `BridgeIn`이면, **THEN** BridgeNode는 Agent로부터 메시지를 수신하여 Flow 입력 포트로 전달해야 한다 (Agent -> Flow).

#### REQ-NODE-001-08-03 (State-Driven) BridgeOut 모드

**IF** `BridgeDirection`이 `BridgeOut`이면, **THEN** BridgeNode는 Flow로부터 메시지를 수신하여 Agent로 전달해야 한다 (Flow -> Agent).

#### REQ-NODE-001-08-04 (State-Driven) BridgeInOut 모드

**IF** `BridgeDirection`이 `BridgeInOut`이면, **THEN** BridgeNode는 양방향 메시지 전달을 수행해야 한다 (Agent <-> Flow). Agent에서 수신한 메시지는 Flow 입력으로, Flow 출력은 Agent로 전달한다.

#### REQ-NODE-001-08-05 (State-Driven) BridgeRequestReply 모드

**IF** `BridgeDirection`이 `BridgeRequestReply`이면, **THEN** BridgeNode는 다음을 수행해야 한다:

1. Agent로부터 요청 메시지 수신
2. Correlation ID(UUID) 생성 및 Metadata의 `_correlationID`에 설정
3. 요청을 Flow 입력 포트로 전달
4. Correlation ID를 `sync.Map`에 등록하여 응답 대기
5. Flow에서 동일 Correlation ID를 가진 응답 메시지를 수신하면 Agent로 응답 전달
6. 타임아웃(기본 30초) 초과 시 타임아웃 에러 응답

#### REQ-NODE-001-08-06 (Event-Driven) Bridge 초기화

**WHEN** `BridgeNode.Init(ctx)` 호출 시, **THEN** `flow.NodeDef.AgentRef`에서 Agent 참조를 추출하고, Agent 연결을 설정해야 한다. Agent를 찾을 수 없으면 `ErrAgentNotFound` 에러를 반환한다.

#### REQ-NODE-001-08-07 (Event-Driven) Bridge 종료

**WHEN** `BridgeNode.Shutdown(ctx)` 호출 시, **THEN** Agent 연결을 해제하고, Request-Reply 모드의 경우 대기 중인 모든 Correlation ID를 타임아웃 처리해야 한다.

#### REQ-NODE-001-08-08 (Unwanted) Correlation ID 타임아웃

시스템은 Request-Reply 모드에서 설정된 타임아웃 내에 응답이 오지 않으면 `ErrRequestTimeout` 에러를 생성하고 **대기 중인 Correlation 항목을 정리해야 한다**.

---

### Module 9: Script Node - 스크립트 노드 (P1)

#### REQ-NODE-001-09-01 (Ubiquitous) ScriptNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 Lua 스크립트 실행을 수행하는 `ScriptNode` 구조체를 제공해야 한다:

- `internal/script/` 패키지의 Lua VM 참조
- 스크립트 소스 코드 또는 파일 경로 보유
- 샌드박스 환경(위험 함수 차단)

#### REQ-NODE-001-09-02 (Event-Driven) 스크립트 실행

**WHEN** `ScriptNode.Process(ctx, msg)` 호출 시, **THEN** Lua VM에서 설정된 스크립트를 실행하며, 입력 메시지의 Payload를 Lua 테이블로 변환하여 전달하고, 스크립트 반환값을 Go Message로 변환하여 출력해야 한다.

#### REQ-NODE-001-09-03 (Event-Driven) 스크립트 Hot Reload

**WHEN** `ScriptNode.Configure(config)` 호출 시 `"script"` 키에 새 스크립트 소스가 전달되면, **THEN** Lua VM을 재초기화하고 새 스크립트를 로드해야 한다. 다음 Process 호출부터 새 스크립트가 적용된다.

#### REQ-NODE-001-09-04 (Unwanted) 스크립트 실행 에러 격리

시스템은 Lua 스크립트 실행 중 에러(문법 오류, 런타임 에러)가 발생하면 **노드를 종료시키지 않고** 에러 메시지를 에러 출력 포트로 전달해야 한다.

#### REQ-NODE-001-09-05 (Unwanted) 스크립트 실행 타임아웃

시스템은 Lua 스크립트 실행이 설정된 타임아웃(기본 5초)을 초과하면 **실행을 강제 중단하고** 타임아웃 에러를 에러 출력 포트로 전달해야 한다.

#### REQ-NODE-001-09-06 (Event-Driven) 스크립트 초기화

**WHEN** `ScriptNode.Init(ctx)` 호출 시, **THEN** Lua VM을 생성하고, 초기 스크립트를 컴파일하며, 샌드박스 환경을 구성해야 한다. 스크립트 컴파일 실패 시 `ErrScriptCompileFailed` 에러를 반환한다.

---

### Module 10: Debug Node - 디버그 노드 (P2)

#### REQ-NODE-001-10-01 (Ubiquitous) DebugNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 메시지 내용을 로깅하는 `DebugNode` 구조체를 제공해야 한다.

#### REQ-NODE-001-10-02 (Event-Driven) 메시지 로깅

**WHEN** `DebugNode.Process(ctx, msg)` 호출 시, **THEN** 메시지의 ID, Payload, Metadata를 설정된 로그 레벨(debug/info/warn)로 출력하고, 메시지를 그대로 출력 포트로 전달해야 한다 (pass-through).

#### REQ-NODE-001-10-03 (Event-Driven) 로그 레벨 설정

**WHEN** `DebugNode.Configure(config)` 호출 시, **THEN** `"level"` 키로 로그 출력 레벨(debug, info, warn)을 설정해야 한다. 기본값은 `debug`이다.

---

### Module 11: Catch Node - 캐치 노드 (P1)

#### REQ-NODE-001-11-01 (Ubiquitous) CatchNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 에러 포트 메시지를 수신하여 처리하는 `CatchNode` 구조체를 제공해야 한다.

#### REQ-NODE-001-11-02 (Event-Driven) 에러 메시지 수신 및 가공

**WHEN** `CatchNode.Process(ctx, msg)` 호출 시, **THEN** 에러 메시지의 Metadata에서 에러 정보(원인 노드 ID, 에러 타입, 에러 메시지)를 추출하고, 설정된 처리 로직(로깅, 변환, 재시도 정보 첨부)을 수행한 후 출력 포트로 전달해야 한다.

#### REQ-NODE-001-11-03 (Event-Driven) 에러 패턴 필터링

**WHEN** `CatchNode.Configure(config)` 호출 시 `"catch_types"` 키가 설정되어 있으면, **THEN** 지정된 에러 타입만 처리하고 나머지는 통과시켜야 한다. 설정이 없으면 모든 에러를 처리한다.

#### REQ-NODE-001-11-04 (Ubiquitous) 캐치 노드 연결 패턴

시스템은 **항상** CatchNode가 다른 노드의 에러 출력 포트에 Wire로 연결되는 패턴을 지원해야 한다. CatchNode의 입력 포트는 에러 포트 타입의 메시지를 수신할 수 있어야 한다.

---

### Module 12: Status Node - 상태 노드 (P2)

#### REQ-NODE-001-12-01 (Ubiquitous) StatusNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 플로우/노드 상태 변경 이벤트를 모니터링하는 `StatusNode` 구조체를 제공해야 한다.

#### REQ-NODE-001-12-02 (Event-Driven) 상태 이벤트 수신

**WHEN** Flow 내 노드의 생명주기 상태가 변경되면 (예: Running -> Paused), **THEN** StatusNode는 상태 변경 이벤트를 수신하고, 이벤트 정보(노드 ID, 이전 상태, 새 상태, 타임스탬프)를 메시지로 변환하여 출력 포트로 전달해야 한다.

#### REQ-NODE-001-12-03 (Event-Driven) 모니터링 대상 설정

**WHEN** `StatusNode.Configure(config)` 호출 시, **THEN** `"watch_nodes"` 키로 모니터링 대상 노드 ID 목록을 설정해야 한다. 빈 목록이면 Flow 내 모든 노드를 모니터링한다.

---

### Module 13: Dead Letter Node - 데드 레터 노드 (P2)

#### REQ-NODE-001-13-01 (Ubiquitous) DeadLetterNode 구조체

시스템은 **항상** `BaseNode`를 임베딩하고 TTL 만료/배달 불가/최대 재시도 초과 메시지를 수집하는 `DeadLetterNode` 구조체를 제공해야 한다.

#### REQ-NODE-001-13-02 (Event-Driven) 데드 레터 메시지 수신

**WHEN** `DeadLetterNode.Process(ctx, msg)` 호출 시, **THEN** 메시지의 Metadata에서 데드 레터 사유(`_deadletter_reason`: ttl_expired, undeliverable, max_retries)를 확인하고, 원인 정보를 보강하여 출력 포트로 전달해야 한다.

#### REQ-NODE-001-13-03 (Event-Driven) 데드 레터 메트릭 기록

**WHEN** DeadLetterNode가 메시지를 수신하면, **THEN** 사유별 메트릭 카운터(ttl_expired, undeliverable, max_retries)를 증가시켜야 한다.

#### REQ-NODE-001-13-04 (Event-Driven) 데드 레터 저장 전략 설정

**WHEN** `DeadLetterNode.Configure(config)` 호출 시, **THEN** `"strategy"` 키로 처리 전략(log: 로깅만, store: 스토어 저장, forward: 다른 Flow로 전달)을 설정해야 한다. 기본값은 `log`이다.

---

### Module 14: Error Types - 에러 타입 (P0)

#### REQ-NODE-001-14-01 (Ubiquitous) Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러를 정의해야 한다:

- `ErrNodeTypeAlreadyRegistered`: 이미 등록된 노드 타입 중복 등록 시도
- `ErrNodeTypeNotFound`: 미등록 노드 타입 조회
- `ErrPortNotFound`: 존재하지 않는 포트 접근
- `ErrInvalidConfig`: 유효하지 않은 노드 설정
- `ErrAgentNotFound`: Bridge Node에서 Agent 조회 실패
- `ErrRequestTimeout`: Bridge Request-Reply 모드 타임아웃
- `ErrScriptCompileFailed`: Script Node 스크립트 컴파일 실패
- `ErrScriptExecutionFailed`: Script Node 스크립트 실행 실패
- `ErrScriptTimeout`: Script Node 스크립트 실행 타임아웃
- `ErrNodeNotInitialized`: 초기화되지 않은 노드에서 Process 호출
- `ErrNodeAlreadyInitialized`: 이미 초기화된 노드에서 Init 재호출
- `ErrAggregateWindowInvalid`: 유효하지 않은 집계 윈도우 설정

#### REQ-NODE-001-14-02 (Ubiquitous) errors.Is() 호환성

시스템은 **항상** 모든 sentinel 에러가 `errors.Is()` 및 `fmt.Errorf("%w", ...)` 래핑과 호환되어야 한다.

#### REQ-NODE-001-14-03 (Ubiquitous) NodeError 구조체

시스템은 **항상** 노드 ID, 노드 타입, 에러 원인을 포함하는 `NodeError` 구조체를 제공하며, `error` 인터페이스와 `Unwrap() error` 메서드를 구현해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/node/
├── base.go           // Node 인터페이스, BaseNode 구조체, Port 타입, NodeOption
├── registry.go       // Registry 구조체, NodeFactory 타입, 등록/생성/조회
├── filter.go         // FilterNode 구현
├── transform.go      // TransformNode 구현
├── switch.go         // SwitchNode 구현
├── aggregate.go      // AggregateNode 구현
├── bridge.go         // BridgeNode 구현 (4모드)
├── script.go         // ScriptNode 구현 (Lua 연동)
├── debug.go          // DebugNode 구현
├── catch.go          // CatchNode 구현
├── status.go         // StatusNode 구현
├── deadletter.go     // DeadLetterNode 구현
├── errors.go         // Sentinel 에러, NodeError 구조체
└── node_test.go      // 전체 테스트 (또는 타입별 *_test.go 분리)
```

### 4.2 타입 시그니처

```go
package node

import (
    "context"
    "sync"
    "time"

    "xflow/pkg/flow"
    "xflow/pkg/lifecycle"
    "xflow/pkg/message"
    "xflow/internal/observe"
)

// ── Node Interface ──

type Node interface {
    ID() string
    Name() string
    Type() string
    Init(ctx context.Context) error
    Process(ctx context.Context, msg message.Message) ([]message.Message, error)
    Shutdown(ctx context.Context) error
    Configure(config map[string]any) error
    Ports() []Port
}

// ── Port System ──

type PortDirection int

const (
    PortInput  PortDirection = iota
    PortOutput
    PortError
)

type Port struct {
    ID        string
    Name      string
    Direction PortDirection
    Connected bool
}

// ── BaseNode ──

type BaseNode struct {
    lifecycle.BaseLifecycle
    id       string
    name     string
    nodeType string
    config   map[string]any
    mu       sync.Mutex
    inputs   map[string]*Port
    outputs  map[string]*Port
    errorPort *Port
    logger   observe.Logger
    metrics  observe.Metrics
}

func NewBaseNode(def flow.NodeDef, opts ...NodeOption) *BaseNode

func (n *BaseNode) ID() string
func (n *BaseNode) Name() string
func (n *BaseNode) Type() string
func (n *BaseNode) Configure(config map[string]any) error
func (n *BaseNode) GetConfig() map[string]any
func (n *BaseNode) Ports() []Port

// ── NodeOption ──

type NodeOption func(*BaseNode)
func WithNodeLogger(logger observe.Logger) NodeOption
func WithNodeMetrics(metrics observe.Metrics) NodeOption

// ── Registry ──

type NodeFactory func(def flow.NodeDef, opts ...NodeOption) (Node, error)

type Registry struct {
    mu        sync.RWMutex
    factories map[string]NodeFactory
}

type RegistryOption func(*Registry)

func NewRegistry(opts ...RegistryOption) *Registry
func WithoutBuiltins() RegistryOption  // 내장 타입 자동 등록 생략

func (r *Registry) Register(typeName string, factory NodeFactory) error
func (r *Registry) Create(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (r *Registry) Types() []string
func (r *Registry) Has(typeName string) bool

// ── FilterNode ──

type FilterNode struct {
    *BaseNode
    condition FilterCondition
}

type FilterCondition func(msg message.Message) bool

func NewFilterNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *FilterNode) Init(ctx context.Context) error
func (n *FilterNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *FilterNode) Shutdown(ctx context.Context) error

// ── TransformNode ──

type TransformNode struct {
    *BaseNode
    rules []TransformRule
}

type TransformRule struct {
    SourceField string
    TargetField string
    TransformFn func(value any) (any, error)
}

func NewTransformNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *TransformNode) Init(ctx context.Context) error
func (n *TransformNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *TransformNode) Shutdown(ctx context.Context) error

// ── SwitchNode ──

type SwitchNode struct {
    *BaseNode
    routes       []SwitchRoute
    defaultPort  string
}

type SwitchRoute struct {
    Condition  func(msg message.Message) bool
    TargetPort string
}

func NewSwitchNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *SwitchNode) Init(ctx context.Context) error
func (n *SwitchNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *SwitchNode) Shutdown(ctx context.Context) error

// ── AggregateNode ──

type WindowType string

const (
    WindowCount WindowType = "count"
    WindowTime  WindowType = "time"
)

type AggregateFn string

const (
    AggregateSum   AggregateFn = "sum"
    AggregateAvg   AggregateFn = "avg"
    AggregateCount AggregateFn = "count"
    AggregateMin   AggregateFn = "min"
    AggregateMax   AggregateFn = "max"
    AggregateFirst AggregateFn = "first"
    AggregateLast  AggregateFn = "last"
)

type AggregateNode struct {
    *BaseNode
    windowType  WindowType
    windowSize  int
    windowDur   time.Duration
    aggregateFn AggregateFn
    buffer      []message.Message
    mu          sync.Mutex
    timer       *time.Timer
}

func NewAggregateNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *AggregateNode) Init(ctx context.Context) error
func (n *AggregateNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *AggregateNode) Shutdown(ctx context.Context) error

// ── BridgeNode ──

type BridgeNode struct {
    *BaseNode
    agentRef      flow.AgentRef
    direction     flow.BridgeDirection
    correlations  sync.Map            // Request-Reply: correlationID -> chan message.Message
    replyTimeout  time.Duration
}

func NewBridgeNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *BridgeNode) Init(ctx context.Context) error
func (n *BridgeNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *BridgeNode) Shutdown(ctx context.Context) error

// ── ScriptNode ──

type ScriptNode struct {
    *BaseNode
    scriptSource  string
    scriptTimeout time.Duration
    // vm 참조는 internal/script 패키지 타입 (별도 SPEC)
}

func NewScriptNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *ScriptNode) Init(ctx context.Context) error
func (n *ScriptNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *ScriptNode) Shutdown(ctx context.Context) error

// ── DebugNode ──

type DebugNode struct {
    *BaseNode
    logLevel string // "debug", "info", "warn"
}

func NewDebugNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *DebugNode) Init(ctx context.Context) error
func (n *DebugNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *DebugNode) Shutdown(ctx context.Context) error

// ── CatchNode ──

type CatchNode struct {
    *BaseNode
    catchTypes []string // 처리 대상 에러 타입 (빈 배열이면 모든 에러)
}

func NewCatchNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *CatchNode) Init(ctx context.Context) error
func (n *CatchNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *CatchNode) Shutdown(ctx context.Context) error

// ── StatusNode ──

type StatusNode struct {
    *BaseNode
    watchNodes []string // 모니터링 대상 노드 ID (빈 배열이면 전체)
}

func NewStatusNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *StatusNode) Init(ctx context.Context) error
func (n *StatusNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *StatusNode) Shutdown(ctx context.Context) error

// ── DeadLetterNode ──

type DeadLetterStrategy string

const (
    DeadLetterLog     DeadLetterStrategy = "log"
    DeadLetterStore   DeadLetterStrategy = "store"
    DeadLetterForward DeadLetterStrategy = "forward"
)

type DeadLetterNode struct {
    *BaseNode
    strategy DeadLetterStrategy
}

func NewDeadLetterNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func (n *DeadLetterNode) Init(ctx context.Context) error
func (n *DeadLetterNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *DeadLetterNode) Shutdown(ctx context.Context) error

// ── NodeError ──

type NodeError struct {
    NodeID   string
    NodeType string
    Err      error
}

func (e *NodeError) Error() string
func (e *NodeError) Unwrap() error

// ── Errors ──

var (
    ErrNodeTypeAlreadyRegistered = errors.New("node: type already registered")
    ErrNodeTypeNotFound          = errors.New("node: type not found")
    ErrPortNotFound              = errors.New("node: port not found")
    ErrInvalidConfig             = errors.New("node: invalid configuration")
    ErrAgentNotFound             = errors.New("node: agent not found")
    ErrRequestTimeout            = errors.New("node: request-reply timeout")
    ErrScriptCompileFailed       = errors.New("node: script compile failed")
    ErrScriptExecutionFailed     = errors.New("node: script execution failed")
    ErrScriptTimeout             = errors.New("node: script execution timeout")
    ErrNodeNotInitialized        = errors.New("node: not initialized")
    ErrNodeAlreadyInitialized    = errors.New("node: already initialized")
    ErrAggregateWindowInvalid    = errors.New("node: invalid aggregate window")
)
```

### 4.3 Node Interface - Engine 통합 다이어그램

```
internal/engine/ (SPEC-ENGINE-001)          internal/node/ (본 SPEC)
================================          =========================
Engine.DeployFlow()                       Registry.Create(nodeDef)
  └── Registry.Create(nodeDef) ---------> NodeFactory(def, opts) -> Node

Engine.StartFlow()                        Node.Init(ctx)
  └── node.Init(ctx) -------------------> BaseNode.Init() + 타입별 초기화

Engine goroutine loop:                    Node.Process(ctx, msg)
  for msg := range inputCh {             ├── 메시지 처리
      results, err := node.Process(...)   ├── 결과 메시지 반환 ([]Message)
      if err != nil {                     └── 에러 시 에러 메시지 반환
          errPort <- errMsg
      }
      for _, out := range results {
          outputCh <- out
      }
  }

Engine.StopFlow()                         Node.Shutdown(ctx)
  └── node.Shutdown(ctx) ----------------> BaseNode.Shutdown() + 타입별 정리
```

### 4.4 Bridge Node 통신 모드 다이어그램

```
BridgeIn (Agent -> Flow):
  Agent ──[메시지]──> BridgeNode ──[입력포트]──> Flow 내 다음 노드

BridgeOut (Flow -> Agent):
  Flow 내 이전 노드 ──[출력포트]──> BridgeNode ──[메시지]──> Agent

BridgeInOut (Agent <-> Flow):
  Agent ──[메시지]──> BridgeNode ──[입력포트]──> Flow
  Flow ──[출력포트]──> BridgeNode ──[메시지]──> Agent

BridgeRequestReply (Agent <-> Flow, Correlation ID 기반):
  Agent ──[요청]──> BridgeNode ──[correlationID 생성]──> Flow
                        │                                   │
                        │<──── sync.Map[correlationID] ─────│
                        │                                   │
                  [타임아웃 감시]                     [응답 correlationID 매칭]
                        │                                   │
  Agent <──[응답]──── BridgeNode <──[응답]──────────────── Flow
```

### 4.5 Node 생명주기 상태 전이

```
pkg/lifecycle/State (SPEC-LIFE-001):

  StateCreated ──[Init()]──> StateInitializing ──[성공]──> StateRunning
       │                          │                          │    │
       │                     [실패]│                    [Pause]│  [Process]
       │                          ↓                          ↓    │
       │                     StateError              StatePaused   │
       │                                                 │        │
       │                                           [Resume]│      │
       │                                                 ↓        │
       │                                            StateRunning──│
       │                                                          │
       └──────────────────────[Shutdown()]──> StateStopping ──> StateStopped

노드는 BaseLifecycle을 임베딩하여 위 상태 전이를 자동 관리한다.
Engine이 상태 전이를 호출하며, 노드는 각 전이에 대한 타입별 초기화/정리 훅을 구현한다.
```

---

## 5. Traceability (추적성)

| 요구사항 ID | 모듈 | 파일 | 우선순위 |
|------------|------|------|---------|
| REQ-NODE-001-01-01 ~ 01-08 | Node Interface & BaseNode | base.go | P0 |
| REQ-NODE-001-02-01 ~ 02-08 | Node Registry | registry.go | P0 |
| REQ-NODE-001-03-01 ~ 03-05 | Port System | base.go | P0 |
| REQ-NODE-001-04-01 ~ 04-04 | Filter Node | filter.go | P1 |
| REQ-NODE-001-05-01 ~ 05-04 | Transform Node | transform.go | P1 |
| REQ-NODE-001-06-01 ~ 06-05 | Switch Node | switch.go | P1 |
| REQ-NODE-001-07-01 ~ 07-05 | Aggregate Node | aggregate.go | P2 |
| REQ-NODE-001-08-01 ~ 08-08 | Bridge Node | bridge.go | P1 |
| REQ-NODE-001-09-01 ~ 09-06 | Script Node | script.go | P1 |
| REQ-NODE-001-10-01 ~ 10-03 | Debug Node | debug.go | P2 |
| REQ-NODE-001-11-01 ~ 11-04 | Catch Node | catch.go | P1 |
| REQ-NODE-001-12-01 ~ 12-03 | Status Node | status.go | P2 |
| REQ-NODE-001-13-01 ~ 13-04 | Dead Letter Node | deadletter.go | P2 |
| REQ-NODE-001-14-01 ~ 14-03 | Error Types | errors.go | P0 |

---

## Implementation Notes

- **구현 일자**: 2026-02-15 (P0+P1), 2026-02-16 (P2)
- **커밋**: `9b28ea5` (P0+P1), `cbd354a` (P2)
- **패키지**: `internal/node/`
- **파일 수**: 28개 (14 구현 + 14 테스트)
- **테스트 커버리지**: 92.8% (전체), 약 159개 테스트
- **구현 범위**: 14/14 모듈 전체 완료
  - Module 1+3: Node 인터페이스 + BaseNode + NodePort 런타임 포트 시스템
  - Module 2: Registry (NodeFactory, 10개 내장 타입 자동 등록)
  - Module 4: FilterNode (조건 기반 메시지 필터링)
  - Module 5: TransformNode (메시지 Payload 변환)
  - Module 6: SwitchNode (조건별 라우팅, First-Match, 기본 라우트)
  - Module 7: AggregateNode (count/time 윈도우 집계, 7개 집계 함수)
  - Module 8: BridgeNode (Agent-Flow 4모드: In/Out/InOut/RequestReply)
  - Module 9: ScriptNode (ScriptEngine 인터페이스 기반, Lua 이연)
  - Module 10: DebugNode (메시지 로깅 pass-through, 3단계 로그 레벨)
  - Module 11: CatchNode (에러 포트 메시지 수신/필터링)
  - Module 12: StatusNode (생명주기 상태 변경 모니터링, watchNodes 필터링)
  - Module 13: DeadLetterNode (폐기 메시지 수집, 3가지 처리 전략)
  - Module 14: 12개 sentinel 에러 + NodeError 구조체
- **설계 결정**:
  - BaseLifecycle 포인터 임베딩 (*lifecycle.BaseLifecycle)
  - NodePort 런타임 포트 (flow.Port 데이터 구조와 분리)
  - AgentResolver/AgentTransport 인터페이스 (Bridge, 실제 Agent 의존성 분리)
  - ScriptEngine 인터페이스 (Lua 구현은 SPEC-SCRIPT-001로 이연)
  - sync.RWMutex 기반 설정/상태 동시성 보호
  - AggregateNode: sync.Mutex 보호 버퍼 + time.Timer 기반 시간 윈도우
  - DeadLetterNode: 사유별 메트릭 카운터 (ttl_expired/undeliverable/max_retries)
  - StatusNode: StatusCallback 함수형 인터페이스로 상태 변경 이벤트 수신
