---
id: SPEC-INVENTORY-001
title: Implementation Plan - Inventory 노드
version: 0.1.0
status: planned
created: 2026-05-25
updated: 2026-05-25
author: xtra
priority: medium
tags: [node, inventory, snapshot, registry, processing]
related_spec: SPEC-NODE-001, SPEC-DEVICE-001, SPEC-AGENT-001, SPEC-FLOW-001, SPEC-ENGINE-001
---

# SPEC-INVENTORY-001: Implementation Plan - Inventory 노드

## 1. 구현 개요

본 계획은 SPEC-INVENTORY-001 의 6개 EARS 모듈 (M1 ~ M6) 을 6개 Phase 로 분할한다. 모든 신규 코드는 TDD (RED-GREEN-REFACTOR) 로 진행하며, 기존 코드(`registry.go`, `main.go`, `nodeSchemas.ts`) 의 최소 침습 수정만 수행한다.

### 1.1 개발 방법론

- **Methodology**: TDD (Test-Driven Development) for new code (`inventory.go`, `inventory_test.go`)
- 기존 파일(`registry.go`, `main.go`, `nodeSchemas.ts`) 의 minimal edit 은 DDD (ANALYZE-PRESERVE-IMPROVE) 로 진행
- `.moai/config/sections/quality.yaml` 의 `development_mode: hybrid` 와 일치 (신규=TDD, 기존=DDD)
- **Test Coverage Target**: 신규 `inventory.go` ≥ 85%
- **참조 패턴**:
  - SPEC-NODE-002 의 "옵셔널 의존성은 NodeOption 으로 주입" 패턴
  - `bridge.go` 의 `WithAgentResolver` config 키 저장 패턴
  - `internal/device/filter.go` 의 `DeviceFilter` 재사용 (재구현 금지)

### 1.2 우선순위

| 우선순위 | Phase | 설명 |
|---------|-------|------|
| Primary Goal | Phase 0, 1 | 사전 설계 결정 + inventory.go 기본 골격 (devices source, array shape) |
| Secondary Goal | Phase 2, 3 | per_item / filter / 4종 source 전체 지원 |
| Tertiary Goal | Phase 4 | NodeOption 4종 + 의존성 부재 에러 처리 |
| Quaternary Goal | Phase 5 | Registry 등록 + main.go resolver 주입 |
| Final Goal | Phase 6 | Web UI nodeSchemas.ts + 문서/예제 |

> **시간 추정 금지**: 본 계획은 우선순위 기반이며, 각 Phase 의 절대 소요 시간은 명시하지 않는다. Phase 간 의존성은 명시한다.

---

## 2. 사전 설계 결정 (Pre-Implementation Decisions)

Plan 승인 단계에서 확정해야 할 설계 결정 4개를 아래에 정리한다.

### 2.1 결정 (a): FlowStatus 타입의 위치

**선택지 a1 - engine 패키지 유지 + 인터페이스 약화**:

`internal/node/inventory.go` 에 다음 인터페이스 정의:

```go
type FlowRegistry interface {
    ListFlows() []any   // 약한 타입
}
```

inventory 노드는 `reflect` 로 각 항목의 `ID`, `Name`, `State`, `NodeCount` 필드를 추출한다.

- 장점: 의존성 cycle 완전 회피
- 단점: 타입 안전성 손실, reflect 오버헤드

**선택지 a2 - FlowStatus 를 공용 pkg 로 추출**:

`pkg/flow` 또는 신규 `pkg/flowstatus` 패키지에 `FlowStatus` struct 정의. engine 과 node 모두 이를 import.

- 장점: 타입 안전성 유지
- 단점: SPEC 범위 외 변경(공용 패키지 추가)

**선택지 a3 - 노드 패키지에 별도 FlowSummary 정의**:

```go
// internal/node/inventory.go
type FlowSummary struct {
    ID, Name, State string
    NodeCount, WireCount int
    Extra map[string]any
}
type FlowRegistry interface {
    FlowSummaries() []FlowSummary
}
```

engine 에 `(e *Engine) FlowSummaries() []node.FlowSummary` 메서드 추가 (얇은 어댑터).

- 장점: 타입 안전 + cycle 없음 + 노드 패키지가 필요 데이터만 정확히 정의
- 단점: engine 에 메서드 1개 추가 필요

**결정**: **a3 선택 (권장)**. import cycle 회피와 타입 안전성을 동시에 달성하며, engine 의 변경은 메서드 1개 추가로 최소 침습이다. inventory 노드가 정말 필요한 필드만 인터페이스에 노출되므로 향후 engine 의 내부 구조 변경에도 안정적이다.

### 2.2 결정 (b): NodeOption 의 config 키 충돌 방지

기존 패턴: `bridge.go` 는 `b.config["_agent_resolver"]` 처럼 underscore 접두 키 사용.

**선택지 b1 - 동일 패턴 (`_device_registry`, `_agent_manager`, `_flow_registry`, `_node_registry`)**:
- 장점: 기존 코드와 일관성
- 단점: 키 충돌 위험 (모든 노드가 같은 `b.config` 공유)

**선택지 b2 - 네임스페이스 prefix (`_inventory.device_registry` 등)**:
- 장점: 충돌 위험 제로
- 단점: 다른 노드가 같은 의존성을 쓰고 싶을 때(예: 향후 `device-control` 노드) 일관성 결여

**결정**: **b1 + 일반화된 키 이름** 선택. underscore prefix 패턴을 유지하되, 키 이름은 의존성 자체를 표현(`_device_registry`)하여 향후 다른 노드도 같은 키를 공유할 수 있게 한다. 키 충돌은 NodeOption 으로만 설정되므로 실제 위험은 낮다.

### 2.3 결정 (c): NodeRegistry 자기 참조 방식

`WithNodeRegistry(*Registry)` 는 `node.Registry` 가 자기 자신을 inventory 노드에 주입하는 형태이다.

**선택지 c1 - Engine 이 Registry 를 만든 후 옵션 주입**:

```go
// cmd/xflowd/main.go
nodeReg := node.NewRegistry()
eng := engine.NewEngine(
    engine.WithNodeRegistry(nodeReg),
    engine.WithNodeOptions(
        node.WithDeviceRegistry(devReg),
        node.WithAgentManager(agtMgr),
        node.WithFlowRegistry(eng),       // ← 순환! eng 아직 생성 안 됨
        node.WithNodeRegistry(nodeReg),
    ),
)
```

이 패턴은 `eng` 가 자기 자신을 가리키는 순환이 발생한다.

**선택지 c2 - 지연 주입 (post-construction)**:

```go
nodeReg := node.NewRegistry()
eng := engine.NewEngine(...)
eng.SetNodeOptions(
    node.WithFlowRegistry(eng),
    node.WithNodeRegistry(nodeReg),
    node.WithDeviceRegistry(devReg),
    node.WithAgentManager(agtMgr),
)
```

`engine.Engine` 에 `SetNodeOptions(opts ...node.NodeOption)` 메서드 추가하여 생성 후 주입.

**선택지 c3 - resolver 함수 패턴**:

```go
node.WithFlowRegistryFunc(func() node.FlowRegistry { return eng })
node.WithNodeRegistryFunc(func() *node.Registry { return nodeReg })
```

함수형 lazy resolver. inventory 노드의 Init 시점에 함수 호출하여 실제 인스턴스 획득.

**결정**: **c3 선택 (권장)**. 함수형 resolver 는 순환 참조와 초기화 순서 문제를 완전히 회피한다. 또한 inventory 노드 인스턴스화 시점이 engine 생성 이후이므로 함수 호출 시점에는 모든 의존성이 준비되어 있다.

API 보강안:
```go
func WithDeviceRegistryFunc(fn func() device.DeviceRegistry) NodeOption
func WithAgentManagerFunc(fn func() agent.Manager) NodeOption
func WithFlowRegistryFunc(fn func() FlowRegistry) NodeOption
func WithNodeRegistryFunc(fn func() *Registry) NodeOption
```

SPEC 본문에서는 `With...` 패턴만 명시하였으나, 구현에서는 `With...Func` 가 일관성·안전성 측면에서 우월하다. spec.md 의 API 절은 plan 결정에 따라 `Func` 변형을 채택한 것으로 간주한다 (외형은 동일, 시그니처만 함수형).

### 2.4 결정 (d): per_item 모드의 metadata 복제 정책

입력 메시지가 `inventory.foo=bar` 같은 비-inventory metadata 를 가지고 들어왔을 때, per_item 으로 fan-out 된 N개 출력 메시지는 이를 어떻게 처리할 것인가?

**선택지 d1 - 입력 metadata 전부 복제**:
- N개 메시지가 모두 동일한 base metadata 를 가지고, 그 위에 `inventory.*` 키가 추가됨
- 장점: 입력 컨텍스트 보존
- 단점: 메모리 N배 증가, 입력 metadata 가 큰 경우 비효율

**선택지 d2 - inventory.* 키만 새로 설정**:
- 출력 메시지의 metadata 는 비어있고 `inventory.*` 키만 채워짐
- 장점: 메모리 효율
- 단점: 입력 트리거의 context 손실 (예: trigger.schedule_id 등이 사라짐)

**선택지 d3 - 입력 metadata 의 shallow copy + inventory.* 추가**:
- 표준 Go map copy (얕은 복사), `inventory.*` 키 덮어쓰기 또는 신규 추가
- 장점: 컨텍스트 보존 + 메모리 효율 (얕은 복사)
- 단점: 없음 (Go map 의 표준 패턴)

**결정**: **d3 선택**. 입력 trigger 의 메타데이터를 보존하면 downstream 노드가 trigger context 를 활용할 수 있다 (예: `trigger.schedule_id` 로 어떤 스케줄이 emit 했는지 추적). 얕은 복사이므로 N=수백 단위까지 무리 없다.

---

## 3. Phase 0: 사전 검증 (DDD - ANALYZE)

**목표**: 본격 구현 전 의존성 구조와 기존 API 호환성을 확인한다.

**작업**:

1. `internal/device/registry.go` 의 `DeviceRegistry.List(filter DeviceFilter) []Device` 시그니처 확인 (이미 확인됨)
2. `internal/agent/manager.go` 의 `Manager.List() []Agent` 시그니처 확인 (이미 확인됨)
3. `internal/engine/engine.go` 의 `ListFlows() []FlowStatus` 시그니처 및 `FlowStatus` 구조 확인
4. `internal/node/registry.go` 의 `AllTypeMeta() []NodeTypeMeta` 시그니처 확인 (이미 확인됨)
5. `engine.Engine.NodeOptions` 주입 경로 확인 — `engine.NewEngine(WithNodeOptions(...))` 가 존재하는지, 노드 생성 시 NodeOption 이 전파되는지
6. `internal/api/handler/device.go` 의 `DeviceHandler` 생성 시 `device.DeviceRegistry` 가 어떻게 주입되는지 확인 → 같은 인스턴스를 노드에도 주입할 수 있도록 main.go 의 단일 출처(single source) 확인

**산출물**: 결정 (a) 의 `FlowStatus` 노출 방식, 결정 (c) 의 resolver 함수 패턴 적합성 최종 확인 메모. plan.md 본문 업데이트 가능.

**테스트 변경**: 없음 (기존 테스트 영향 없음)

**완료 조건**:
- Phase 0 검증 메모가 plan.md 또는 별도 노트에 기록됨
- 기존 NodeOption 패턴(예: `WithAgentResolver`)이 inventory 시나리오에 그대로 적용 가능함이 확인됨

---

## 4. Phase 1: inventory.go 기본 골격 + devices/array (TDD)

**목표**: M1 + M2(array) + M3 + M5(devices) 의 가장 기본 시나리오를 통과하는 inventory 노드 구현.

**의존성**: Phase 0 완료

**RED 단계** (`internal/node/inventory_test.go`):

1. `TestInventoryNode_FactoryWithMissingSource_ReturnsError`
2. `TestInventoryNode_FactoryWithInvalidSource_ReturnsError`
3. `TestInventoryNode_FactoryWithInvalidEmitShape_ReturnsError`
4. `TestInventoryNode_DevicesArrayShape_EmitsSingleMessage` — fake DeviceRegistry 와 2개 디바이스
5. `TestInventoryNode_DevicesArrayShape_EmptyRegistry_EmitsCountZero`
6. `TestInventoryNode_Devices_WithFilter_AppliesDeviceFilter` — filter 가 DeviceRegistry.List 에 전달됨

**GREEN 단계** (`internal/node/inventory.go`):

```go
package node

import (
    "context"
    "encoding/hex"  // unused?
    "fmt"
    "log/slog"

    "github.com/xtra/xflow/internal/device"
    "github.com/xtra/xflow/pkg/flow"
    "github.com/xtra/xflow/pkg/message"
)

const (
    InventorySourceDevices = "devices"
    InventorySourceAgents  = "agents"
    InventorySourceNodes   = "nodes"
    InventorySourceFlows   = "flows"

    InventoryShapeArray   = "array"
    InventoryShapePerItem = "per_item"
)

var (
    ErrInventoryInvalidSource    = fmt.Errorf("inventory: %w: invalid source", ErrInvalidConfig)
    ErrInventoryInvalidEmitShape = fmt.Errorf("inventory: %w: invalid emit_shape", ErrInvalidConfig)
    ErrInventoryInvalidFilter    = fmt.Errorf("inventory: %w: invalid filter", ErrInvalidConfig)
    // ... 의존성 부재 에러는 Phase 4 에서 추가
)

type InventoryNode struct {
    *BaseNode
    source          string
    emitShape       string
    filter          device.DeviceFilter
    filterSpecified bool
    includeMetadata bool

    // 의존성 resolver (Phase 4 에서 채움)
    deviceRegistryFn func() device.DeviceRegistry
}

func NewInventoryNode(def flow.NodeDef, opts ...NodeOption) (Node, error) { /* ... */ }
func (n *InventoryNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error) { /* ... */ }
```

**REFACTOR 단계**:
- source 별 List 함수를 method-table 패턴으로 분리 (`listDevices`, `listAgents` 등) → Phase 2 준비
- item serializer 도 함수 분리 (`deviceToItem(d Device, includeMeta bool) map[string]any`)

**테스트 변경**: 신규 파일만 추가, 기존 테스트 영향 없음

**완료 조건**:
- inventory_test.go 의 Phase 1 테스트 6개 PASS
- `go test ./internal/node/...` 전체 PASS (regression 없음)
- coverage ≥ 85% (Phase 1 범위)

---

## 5. Phase 2: per_item 모드 + 4종 source 완성 (TDD)

**목표**: M2(per_item) + M5(4종 source) 완성.

**의존성**: Phase 1 완료

**RED 단계** (테스트 추가):

1. `TestInventoryNode_DevicesPerItem_EmitsNMessages`
2. `TestInventoryNode_DevicesPerItem_EmptyRegistry_EmitsZeroMessages`
3. `TestInventoryNode_DevicesPerItem_IndexMetadataCorrect`
4. `TestInventoryNode_AgentsArray_EmitsAgentList`
5. `TestInventoryNode_NodesArray_EmitsNodeTypeMeta`
6. `TestInventoryNode_FlowsArray_EmitsFlowSummary`
7. `TestInventoryNode_IncludeMetadataFalse_OmitsRichFields` — devices/agents 양쪽
8. `TestInventoryNode_PerItem_PreservesInputMetadata` — 결정 (d) 검증

**GREEN 단계**:

- `agent.Manager`, `FlowRegistry`, `*Registry` 의존성을 위한 fake/mock 구현 (테스트 helper)
- source 별 item 변환 함수: `agentToItem`, `flowSummaryToItem`, `nodeTypeMetaToItem`
- per_item emit 로직: `for i, item := range items { outMsg := buildPerItemMessage(item, i, total, baseMeta) }`

**REFACTOR 단계**:
- 공통 emit 로직을 `emitInventory(source string, items []map[string]any, msg message.Message)` 로 추출
- includeMetadata=false 일 때 omit 필드를 source 별 helper 안에서 분기

**완료 조건**:
- Phase 2 테스트 8개 PASS
- emit_shape × source 의 8개 조합(4 source × 2 shape) 모두 검증됨

---

## 6. Phase 3: Filter 검증 강화 + 비-devices source 의 filter 무시 (TDD)

**목표**: M3 의 unwanted 절 완성.

**의존성**: Phase 2 완료

**RED 단계**:

1. `TestInventoryNode_FilterWithInvalidTagsType_FactoryError`
2. `TestInventoryNode_FilterWithInvalidOnlineType_FactoryError`
3. `TestInventoryNode_FilterOnNonDeviceSource_LoggedAndIgnored` — `source=agents` + `filter` 가 있어도 정상 동작, 경고 로그 1회

**GREEN 단계**:
- `parseDeviceFilter(raw map[string]any) (device.DeviceFilter, error)` helper 작성
- non-device source 일 때 `slog.Warn("inventory: filter ignored for non-device source", "source", n.source)` 1회 로깅 (sync.Once 또는 init 시점)

**완료 조건**:
- Phase 3 테스트 3개 PASS
- yaml/json 의 filter 표현이 `device.DeviceFilter` 의 json tag (`protocol`, `agent_name`, `type`, `online`, `tags`, `group`) 와 정확히 매핑됨

---

## 7. Phase 4: NodeOption 4종 + 의존성 부재 에러 (TDD + DDD 일부)

**목표**: M4 완성. 결정 (c) 의 resolver-func 패턴 적용.

**의존성**: Phase 3 완료

**RED 단계**:

1. `TestInventoryNode_DevicesSource_WithoutRegistryOption_InitError`
2. `TestInventoryNode_AgentsSource_WithoutManagerOption_InitError`
3. `TestInventoryNode_FlowsSource_WithoutFlowRegistryOption_InitError`
4. `TestInventoryNode_NodesSource_WithoutNodeRegistryOption_InitError`
5. `TestInventoryNode_AllSources_WithAllOptionsInjected_ProcessSucceeds`

**GREEN 단계**:

```go
// internal/node/inventory.go (또는 별도 options 파일)

// FlowSummary 는 inventory 노드가 flow 목록을 표현할 때 사용하는 작은 DTO 이다.
// engine.Engine 이 FlowSummaries() 메서드를 통해 이를 만족한다.
type FlowSummary struct {
    ID        string
    Name      string
    State     string
    NodeCount int
    WireCount int
    Extra     map[string]any
}

type FlowRegistry interface {
    FlowSummaries() []FlowSummary
}

func WithDeviceRegistryFunc(fn func() device.DeviceRegistry) NodeOption {
    return func(b *BaseNode) {
        if b.config == nil { b.config = map[string]any{} }
        b.config["_device_registry_fn"] = fn
    }
}
// 동일 패턴으로 WithAgentManagerFunc, WithFlowRegistryFunc, WithNodeRegistryFunc

// Init 에서 source 별 함수 호출
func (n *InventoryNode) Init(ctx context.Context) error {
    cfg := n.GetConfig()
    switch n.source {
    case InventorySourceDevices:
        fn, ok := cfg["_device_registry_fn"].(func() device.DeviceRegistry)
        if !ok || fn == nil { return ErrInventoryDeviceRegistryNotAvailable }
        n.deviceRegistryFn = fn
    // ... agents, flows, nodes 동일 패턴
    }
    return n.BaseNode.Init(ctx)  // or super
}
```

**DDD 일부**: `engine.Engine` 에 `FlowSummaries() []node.FlowSummary` 메서드 추가 (1개 메서드 추가, 기존 동작 보존). 기존 `ListFlows() []FlowStatus` 는 그대로 유지.

**완료 조건**:
- Phase 4 테스트 5개 PASS
- `engine.Engine.FlowSummaries()` 가 `engine_test.go` 의 기존 테스트와 상호작용하지 않음 (회귀 없음)

---

## 8. Phase 5: Registry 등록 + main.go resolver 주입 (DDD - PRESERVE-IMPROVE)

**목표**: 빌트인 등록과 의존성 주입의 최종 와이어링.

**의존성**: Phase 4 완료

**작업**:

### 5.1 `internal/node/registry.go` 수정

```go
// registerBuiltins 의 builtins 배열에 1줄 추가
{"inventory", NewInventoryNode, "processing", "in-process 디바이스/에이전트/노드/플로우 인벤토리 스냅샷을 emit"},
```

기존 라인은 변경하지 않는다. 등록 순서는 알파벳순 또는 카테고리순 정책 유지.

### 5.2 `cmd/xflowd/main.go` 수정

main.go 의 engine 생성부에서 다음과 같이 NodeOption 4종 주입:

```go
// 기존: engine := engine.NewEngine(...)
// 변경: NodeOptions 에 inventory 의존성 추가

nodeReg := node.NewRegistry()
devReg := device.NewRegistry()
agtMgr := agent.NewDefaultManager(...)
// flow engine 자체를 FlowRegistry 로 활용

eng := engine.NewEngine(
    // 기존 옵션 보존
    engine.WithNodeRegistry(nodeReg),
    engine.WithAgentManager(agtMgr),
    engine.WithNodeOptions(
        // 기존 옵션 (예: WithAgentResolver) 보존
        node.WithDeviceRegistryFunc(func() device.DeviceRegistry { return devReg }),
        node.WithAgentManagerFunc(func() agent.Manager { return agtMgr }),
        node.WithNodeRegistryFunc(func() *node.Registry { return nodeReg }),
        // FlowRegistry 는 self-reference, lazy 함수로 회피
    ),
)

// engine 생성 후 자기 자신을 FlowRegistry 로 주입
eng.AddNodeOption(node.WithFlowRegistryFunc(func() node.FlowRegistry { return eng }))
```

> **단서**: `engine.AddNodeOption` 또는 `SetNodeOptions` 메서드가 기존에 존재하지 않으면 신규 추가 필요. Phase 0 에서 확인할 항목. 만약 NodeOption 이 engine 생성 시점에만 전달 가능하다면, `WithFlowRegistryFunc` 의 함수가 외부 변수 `var eng *engine.Engine` 을 capture 하는 방식으로 우회 가능 (Go 클로저).

### 5.3 회귀 테스트

`cmd/xflowd/main_test.go` 또는 통합 테스트에서:
- 기존 빌트인 노드(`filter`, `transform`, `bridge` 등)가 모두 정상 생성됨을 확인
- 신규 inventory 노드가 정상 등록됨 확인 (`registry.Has("inventory") == true`)

**완료 조건**:
- `inventory` 가 `node.Registry.Types()` 출력에 포함됨
- xflowd 바이너리가 정상 빌드/실행됨
- 기존 통합 테스트 모두 PASS

---

## 9. Phase 6: Web UI 통합 + 문서/예제 (DDD - IMPROVE)

**목표**: M6 완성. 사용자 노출.

**의존성**: Phase 5 완료

### 6.1 `web/src/config/nodeSchemas.ts` 수정

기존 `trigger` 키 근처(또는 `processing` 카테고리 인근)에 inventory 키 추가:

```typescript
inventory: {
  description: '디바이스/에이전트/노드/플로우 인벤토리 스냅샷을 emit. trigger 와 체이닝하여 주기적 상태 동기화에 사용.',
  inputDesc: '임의의 트리거 메시지 (페이로드 무시). 보통 trigger 노드의 출력을 입력으로 사용.',
  outputDesc:
    'array 모드: payload { source, count, items[] }. per_item 모드: payload 가 단일 item 객체. ' +
    'metadata: inventory.source, inventory.count. per_item 모드에서 추가로 inventory.index, inventory.total.',
  configSchema: {
    fields: [
      {
        name: 'source', type: 'select', label: '소스', required: true,
        options: ['devices', 'agents', 'nodes', 'flows'],
        description: '스냅샷 대상 인벤토리 종류',
      },
      {
        name: 'emit_shape', type: 'select', label: 'emit 형태',
        options: ['array', 'per_item'], default: 'array',
        description: 'array: 단일 메시지에 배열 / per_item: 항목별 N개 메시지 fan-out',
      },
      {
        name: 'include_metadata', type: 'boolean', label: '메타데이터 포함',
        default: true,
        description: 'true: 풍부한 메타데이터(state, properties 등) 포함 / false: 핵심 필드만',
      },
      {
        name: 'filter', type: 'object', label: '디바이스 필터',
        description: 'source=devices 일 때만 적용. DeviceFilter 와 동일 스키마.',
        visibleWhen: { field: 'source', value: 'devices' },
        // 하위 필드: protocol, agent_name, type, online, group, tags
      },
    ],
  },
  defaultPorts: [
    { name: 'in', direction: 'input' as const },
    { name: 'out', direction: 'output' as const },
  ],
},
```

> 하위 필드 구조는 기존 nodeSchemas.ts 의 `object` 필드 표현 방식에 맞춰 조정 (필요 시 nested `fields` 또는 별도 sub-schema).

### 6.2 README / docs 업데이트

- `internal/node/README.md` 의 빌트인 노드 표에 inventory 추가
- 예제 플로우 yaml 1개 작성 (`docs/examples/inventory-snapshot.yaml` 또는 유사 위치)

### 6.3 회귀 테스트

- Web UI 빌드 (`pnpm build`) 성공
- 노드 팔레트에서 inventory 노드 검색/추가 가능

**완료 조건**:
- nodeSchemas.ts 에 inventory 키 추가, 타입 에러 없음
- README/예제 문서 업데이트
- PR 가능한 상태

---

## 10. 기술적 접근 (Technical Approach)

### 10.1 사용 라이브러리 (기존 활용)

- `github.com/xtra/xflow/internal/device` — DeviceRegistry, DeviceFilter, Device, DeviceMetadata
- `github.com/xtra/xflow/internal/agent` — Manager, Agent, AgentInfo, StatsSnapshot
- `github.com/xtra/xflow/internal/engine` — Engine, FlowStatus (FlowSummary 신규 분리)
- `github.com/xtra/xflow/internal/node` — Registry, NodeTypeMeta, NodeOption, BaseNode
- `github.com/xtra/xflow/pkg/flow` — NodeDef
- `github.com/xtra/xflow/pkg/message` — Message, Payload, Metadata
- 표준 라이브러리: `log/slog`, `fmt`, `context`, `strconv` (index 직렬화)

> **신규 의존성**: 없음. 모든 기능은 기존 in-process 객체만으로 구현 가능.

### 10.2 핵심 알고리즘 (의사 코드)

```
function Process(ctx, inMsg):
    items = []
    switch n.source:
        case "devices":
            reg = n.deviceRegistryFn()
            devices = reg.List(n.filter)
            for d in devices:
                items.append(deviceToItem(d, n.includeMetadata))
        case "agents":
            mgr = n.agentManagerFn()
            for a in mgr.List():
                items.append(agentToItem(a, n.includeMetadata))
        case "flows":
            reg = n.flowRegistryFn()
            for f in reg.FlowSummaries():
                items.append(flowSummaryToItem(f, n.includeMetadata))
        case "nodes":
            reg = n.nodeRegistryFn()
            for m in reg.AllTypeMeta():
                items.append(nodeTypeMetaToItem(m))

    if n.emitShape == "array":
        out = newMessage(payload={source, count: len(items), items}, meta={inventory.source, inventory.count})
        out.metadata.merge(inMsg.metadata)  // 결정 (d): 입력 메타 보존
        return [out]
    else:  // per_item
        results = []
        for i, item in enumerate(items):
            out = newMessage(payload=item, meta={inventory.source, inventory.count, inventory.index=i, inventory.total=len(items)})
            out.metadata.merge(inMsg.metadata)
            results.append(out)
        return results
```

### 10.3 동시성

- inventory 노드는 stateless 한 read-only 변환이므로 `BaseNode.mu` 외 추가 lock 불필요
- `DeviceRegistry.List`, `agent.Manager.List` 등은 이미 내부 lock 으로 보호됨 (기존 SPEC-DEVICE-001, SPEC-AGENT-001 보장)
- per_item 모드의 N개 메시지 생성은 단일 goroutine 에서 순차 실행. fan-out 은 engine 의 wire dispatcher 가 담당.

### 10.4 관측성

- `n.logger.Info("inventory: emitted snapshot", "source", n.source, "shape", n.emitShape, "count", len(items))` — Process 마다 1회
- `n.logger.Warn("inventory: filter ignored for non-device source", "source", n.source)` — 비-device source 에서 filter 가 있을 때 1회 (init 시점)
- 메트릭: 기존 `BaseNode.metrics` 의 표준 카운터(processed_messages, errors) 활용

---

## 11. 마일스톤 (Milestones)

| 마일스톤 | Phase | 산출물 |
|---------|-------|--------|
| M0: 사전 설계 합의 | Phase 0 | 결정 (a)~(d) 확정, plan 본문 final |
| M1: 기본 골격 | Phase 1 | inventory.go 초안, devices/array PASS |
| M2: shape × source 매트릭스 | Phase 2 | 4 × 2 = 8 조합 모두 PASS |
| M3: 필터 강화 | Phase 3 | DeviceFilter 재사용 검증 완료 |
| M4: 의존성 주입 | Phase 4 | NodeOption 4종 + Init 에러 5개 |
| M5: 와이어링 | Phase 5 | registry 등록, main.go 주입 완료, xflowd 빌드 OK |
| M6: UI/문서 | Phase 6 | nodeSchemas.ts, README, 예제 yaml |

각 마일스톤은 독립적으로 PR 분할 가능하다 (권장: M0-M2 / M3-M4 / M5-M6 의 3 PR).

---

## 12. 리스크 및 대응 (Risks and Mitigations)

| 리스크 | 영향도 | 대응 |
|--------|--------|------|
| FlowStatus 의 노드 패키지 노출이 engine 패키지 import cycle 유발 | High | 결정 (a3) 의 `node.FlowSummary` 별도 정의로 회피 |
| NodeRegistry 자기 참조 초기화 순서 문제 | Medium | 결정 (c3) 의 resolver-func 패턴으로 회피 |
| trigger → inventory(per_item, N=수천) 시 메모리 압박 | Low | 본 SPEC 에서는 미최적화. 운영 사례 수집 후 후속 SPEC 으로 메시지 풀링 검토 |
| DeviceFilter 의 yaml 표현(`tags: [...]`) 파싱 실패 | Medium | `parseDeviceFilter` 에서 명시적 타입 검증 + ErrInventoryInvalidFilter 반환 |
| Web UI 의 `visibleWhen` 이 nested object 필드를 지원 안 함 | Low | filter 를 평면 필드 6개로 분해 또는 별도 컴포넌트 사용 |
| agent.Agent.Stats() 가 비용 큰 호출(예: 통계 계산) | Low | `include_metadata=false` 일 때 Stats 호출 생략 |
| inventory 노드가 다른 모든 노드 옵션을 받아도 무시하는지 검증 부족 | Low | Phase 1 의 TestInventoryNode_FactoryWithMissingSource_ReturnsError 등에서 함께 검증 |

---

## 13. 완료 조건 (Definition of Done)

본 SPEC 의 구현이 완료되었다고 선언하기 위한 조건:

- [ ] Phase 0 ~ Phase 6 모두 완료
- [ ] `go test ./internal/node/... -run Inventory` 전체 PASS, 신규 코드 coverage ≥ 85%
- [ ] `go test ./...` 전체 PASS (regression 0)
- [ ] `golangci-lint run` 신규 경고 0
- [ ] xflowd 바이너리가 inventory 노드 포함하여 빌드/실행됨
- [ ] Web UI 빌드(`pnpm build`) 성공, inventory 노드가 팔레트에 노출됨
- [ ] acceptance.md 의 모든 AC 시나리오 PASS
- [ ] PR 본문에 SPEC-INVENTORY-001 링크와 acceptance.md 체크리스트 포함
- [ ] CHANGELOG 에 신규 노드 추가 항목 기재
