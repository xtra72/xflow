---
id: SPEC-INVENTORY-001
title: Acceptance Criteria - Inventory 노드
version: 0.1.0
status: planned
created: 2026-05-25
updated: 2026-05-25
author: xtra
priority: medium
tags: [node, inventory, snapshot, registry, processing]
related_spec: SPEC-NODE-001, SPEC-DEVICE-001, SPEC-AGENT-001, SPEC-FLOW-001, SPEC-ENGINE-001
---

# SPEC-INVENTORY-001: Acceptance Criteria - Inventory 노드

본 문서는 SPEC-INVENTORY-001 의 인수 기준을 Given-When-Then 형식으로 정의한다. 각 시나리오는 자동화된 Go 테스트(`internal/node/inventory_test.go`) 또는 통합/UI 테스트로 검증 가능해야 한다. 테스트 파일 경로는 참고용이며, 최종 위치는 구현 단계에서 조정할 수 있다.

---

## 1. M1: 노드 정의와 Source Enum

### AC1.1: Registry 에 inventory 타입 등록

**Given** 빌트인이 모두 등록된 `internal/node.Registry` 인스턴스
**When** `Registry.Has("inventory")` 와 `Registry.TypeMeta("inventory")` 를 호출하면
**Then**:
- `Has` 는 `true` 를 반환해야 한다
- `TypeMeta` 는 `ok=true`, `Type="inventory"`, `Category="processing"`, `Source="builtin"` 인 `NodeTypeMeta` 를 반환해야 한다
- Description 은 한국어이며 "인벤토리" 또는 "스냅샷" 키워드를 포함해야 한다

### AC1.2: 기본 포트 정의

**Given** 유효한 옵션(`source=devices`)과 의존성이 주입된 inventory 노드 인스턴스
**When** 노드의 포트 정보를 조회하면
**Then**:
- `inputs` 는 `["in"]` 이어야 한다
- `outputs` 는 `["out"]` 을 포함해야 한다
- `error outputs` 는 `["error"]` 또는 기본 `_error` 포트를 포함해야 한다

### AC1.3: source 옵션 누락 시 팩토리 에러

**Given** `NodeDef.Options` 에 `source` 키가 없는 정의
**When** inventory 노드 팩토리를 호출하면
**Then**:
- nil 노드와 non-nil 에러를 반환해야 한다
- 에러는 `errors.Is(err, ErrInventoryInvalidSource)` 가 true 여야 한다
- 에러 메시지는 source 옵션 누락을 명확히 설명해야 한다

### AC1.4: 유효하지 않은 source 값

**Given** `source="unknown_kind"` 옵션
**When** 팩토리를 호출하면
**Then**:
- nil 노드와 non-nil 에러를 반환해야 한다
- 에러는 `errors.Is(err, ErrInventoryInvalidSource)` 가 true 여야 한다
- 에러 메시지는 허용 값 목록 (`devices`, `agents`, `nodes`, `flows`) 을 포함해야 한다

### AC1.5: 유효한 4종 source 모두 팩토리 통과

**Given** 4개의 NodeDef 가 각각 `source` 를 `devices`, `agents`, `nodes`, `flows` 로 설정
**When** 각 NodeDef 에 대해 팩토리를 호출하면
**Then**:
- 4개 모두 nil 에러로 노드 인스턴스를 반환해야 한다
- 각 노드의 internal source 필드가 입력 값과 정확히 일치해야 한다

---

## 2. M2: emit_shape 동작 (Array / Per-Item)

### AC2.1: 기본 emit_shape 는 array

**Given** `emit_shape` 가 명시되지 않은 inventory 노드 (`source=devices`)
**When** 노드 인스턴스를 검사하면
**Then**:
- 내부 emitShape 필드가 `"array"` 여야 한다

### AC2.2: array 모드 - 단일 메시지 emit

**Given** `source=devices`, `emit_shape=array` 의 inventory 노드, DeviceRegistry 에 2개 device 등록
**When** 임의 입력 메시지로 `Process` 를 호출하면
**Then**:
- 정확히 1개의 출력 메시지를 반환해야 한다
- 메시지 payload 의 `source` 키는 `"devices"` 여야 한다
- 메시지 payload 의 `count` 키는 `2` 여야 한다
- 메시지 payload 의 `items` 키는 길이 2의 배열이어야 한다

### AC2.3: array 모드 - 빈 레지스트리에서 count=0

**Given** `source=devices`, `emit_shape=array`, DeviceRegistry 가 비어있음
**When** `Process` 를 호출하면
**Then**:
- 정확히 1개의 메시지를 반환해야 한다 (에러 아님)
- payload 의 `count` 는 `0`
- payload 의 `items` 는 빈 배열 `[]` (nil 이 아님)

### AC2.4: per_item 모드 - N개 메시지 emit

**Given** `source=devices`, `emit_shape=per_item`, DeviceRegistry 에 3개 device 등록
**When** `Process` 를 호출하면
**Then**:
- 정확히 3개의 출력 메시지를 반환해야 한다
- 각 메시지 payload 는 단일 device 객체이며 wrapper(`source`/`count`/`items`)가 없어야 한다
- 각 메시지 payload 에는 `id`, `name`, `type`, `protocol`, `online` 등 device 필드가 직접 노출되어야 한다

### AC2.5: per_item 모드 - 빈 레지스트리에서 0개 메시지

**Given** `source=devices`, `emit_shape=per_item`, DeviceRegistry 가 비어있음
**When** `Process` 를 호출하면
**Then**:
- 빈 슬라이스 `[]` (또는 `nil`) 을 반환해야 한다
- 에러는 nil 이어야 한다
- 에러 포트로 메시지가 전달되지 않아야 한다

### AC2.6: per_item 모드 - 인덱스/전체 metadata 정확

**Given** `source=devices`, `emit_shape=per_item`, DeviceRegistry 에 3개 device
**When** `Process` 를 호출하여 3개 메시지를 받으면
**Then**:
- i번째(0-based) 메시지의 metadata 에 `inventory.index = "i"` 가 있어야 한다
- 모든 메시지의 metadata 에 `inventory.total = "3"` 가 있어야 한다
- 모든 메시지의 metadata 에 `inventory.count = "3"` 가 있어야 한다 (alias)
- 모든 메시지의 metadata 에 `inventory.source = "devices"` 가 있어야 한다

### AC2.7: 잘못된 emit_shape 값

**Given** `source=devices`, `emit_shape="batch"` (허용되지 않은 값)
**When** 팩토리를 호출하면
**Then**:
- nil 노드와 non-nil 에러를 반환해야 한다
- 에러는 `errors.Is(err, ErrInventoryInvalidEmitShape)` 가 true 여야 한다
- 에러 메시지는 허용 값 목록(`array`, `per_item`)을 포함해야 한다

---

## 3. M3: Filter 적용 (Devices 한정)

### AC3.1: filter 가 DeviceRegistry.List 에 전달됨

**Given** `source=devices`, `filter={protocol: "lgcnp", online: true}` 옵션
**When** `Process` 를 호출하면
**Then**:
- DeviceRegistry.List 가 정확히 1회 호출되어야 한다
- 호출 시 전달된 `DeviceFilter` 의 `Protocol` 필드가 `"lgcnp"` 여야 한다
- 호출 시 전달된 `DeviceFilter` 의 `Online` 필드가 `*bool(true)` 를 가리켜야 한다 (nil 아님)

### AC3.2: 필터 누락 시 빈 필터로 전체 조회

**Given** `source=devices`, `filter` 옵션 미제공
**When** `Process` 를 호출하면
**Then**:
- DeviceRegistry.List 에 zero-value `DeviceFilter{}` 가 전달되어야 한다
- 모든 디바이스가 emit 되어야 한다 (필터링 없음)

### AC3.3: DeviceFilter 의 모든 필드가 정확히 매핑됨

**Given** `source=devices`, `filter={protocol: "modbus", agent_name: "ag1", type: "HVACR.IDU", online: false, group: "prod", tags: ["critical", "v2"]}`
**When** Process 시 DeviceRegistry.List 에 전달된 필터를 검증하면
**Then**:
- `Protocol == "modbus"`
- `AgentName == "ag1"`
- `Type == "HVACR.IDU"`
- `Online != nil && *Online == false`
- `Group == "prod"`
- `Tags == ["critical", "v2"]` (순서 보존)

### AC3.4: 비-device source 의 filter 무시 + 경고

**Given** `source=agents`, `filter={protocol: "lgcnp"}` (의미 없는 필터)
**When** 팩토리/Init/Process 를 호출하면
**Then**:
- 팩토리는 성공해야 한다 (에러 없음)
- Process 는 모든 agent 를 정상 emit 해야 한다 (필터 무시)
- 로그에 `"filter ignored for non-device source"` 메시지가 최소 1회 출력되어야 한다 (전체 lifecycle 동안 1회만 출력해도 됨, sync.Once 정책 허용)

### AC3.5: 잘못된 filter 타입

**Given** `source=devices`, `filter={tags: [1, 2, 3]}` (tags 가 숫자 배열)
**When** 팩토리를 호출하면
**Then**:
- nil 노드와 non-nil 에러를 반환해야 한다
- 에러는 `errors.Is(err, ErrInventoryInvalidFilter)` 가 true 여야 한다

### AC3.6: filter.online 의 타입 검증

**Given** `source=devices`, `filter={online: "yes"}` (string 이지 bool 아님)
**When** 팩토리를 호출하면
**Then**:
- nil 노드와 non-nil 에러를 반환해야 한다
- 에러는 `errors.Is(err, ErrInventoryInvalidFilter)` 가 true 여야 한다

---

## 4. M4: 의존성 주입 및 부재 시 동작

### AC4.1: WithDeviceRegistry 미주입 시 Init 에러

**Given** `source=devices` 의 inventory 노드, `WithDeviceRegistryFunc` 옵션 미제공
**When** 노드 `Init(ctx)` 를 호출하면
**Then**:
- non-nil 에러를 반환해야 한다
- 에러는 `errors.Is(err, ErrInventoryDeviceRegistryNotAvailable)` 가 true 여야 한다
- 노드는 Running 상태로 진입하지 않아야 한다

### AC4.2: WithAgentManager 미주입 시 Init 에러

**Given** `source=agents` 의 inventory 노드, `WithAgentManagerFunc` 미제공
**When** `Init(ctx)` 를 호출하면
**Then**:
- 에러는 `errors.Is(err, ErrInventoryAgentManagerNotAvailable)` 가 true 여야 한다

### AC4.3: WithFlowRegistry 미주입 시 Init 에러

**Given** `source=flows` 의 inventory 노드, `WithFlowRegistryFunc` 미제공
**When** `Init(ctx)` 를 호출하면
**Then**:
- 에러는 `errors.Is(err, ErrInventoryFlowRegistryNotAvailable)` 가 true 여야 한다

### AC4.4: WithNodeRegistry 미주입 시 Init 에러

**Given** `source=nodes` 의 inventory 노드, `WithNodeRegistryFunc` 미제공
**When** `Init(ctx)` 를 호출하면
**Then**:
- 에러는 `errors.Is(err, ErrInventoryNodeRegistryNotAvailable)` 가 true 여야 한다

### AC4.5: 다른 source 의 옵션 미주입은 무관

**Given** `source=devices` 의 inventory 노드, `WithDeviceRegistryFunc` 만 주입, 다른 3종 옵션은 미주입
**When** `Init(ctx)` 와 `Process` 를 호출하면
**Then**:
- `Init` 에러 없음, `Process` 정상 동작 (devices 만 필요하므로)

### AC4.6: 모든 옵션 주입 시 4종 source 모두 정상 동작

**Given** 4종 NodeOption (`WithDeviceRegistryFunc`, `WithAgentManagerFunc`, `WithFlowRegistryFunc`, `WithNodeRegistryFunc`) 가 모두 주입된 환경
**When** 4종 source 각각으로 inventory 노드를 생성하고 `Init` + `Process` 를 호출하면
**Then**:
- 4개 노드 모두 에러 없이 정상 Process 출력을 생성해야 한다

---

## 5. M5: 메시지 스키마 및 Metadata 키

### AC5.1: array 모드 payload 의 필수 키

**Given** `source=devices`, `emit_shape=array`, DeviceRegistry 에 1개 device
**When** Process 출력 메시지를 검사하면
**Then**:
- payload 에 정확히 다음 키들이 존재해야 한다: `source`, `count`, `items`
- payload 에 다른 wrapping 키(`data`, `metadata` 등)는 없어야 한다

### AC5.2: per_item 모드 payload 의 노출 형태

**Given** `source=devices`, `emit_shape=per_item`, DeviceRegistry 에 1개 device
**When** Process 출력 메시지를 검사하면
**Then**:
- 1개의 메시지가 emit 되어야 한다
- 메시지 payload 는 device item 객체 자체이며, `source` / `count` / `items` 키가 **없어야** 한다
- payload 에는 `id`, `name` 등 item 필드가 직접 노출되어야 한다

### AC5.3: devices 항목 스키마 검증 (include_metadata=true)

**Given** `source=devices`, `include_metadata=true` (기본), 1개 device(id="ag1:0.0.16", protocol="lgcnp", online=true, capabilities=["status","control"], metadata.group="prod", state.error_count=2)
**When** Process 출력의 첫 번째 item 을 검사하면
**Then**:
- `id == "ag1:0.0.16"`, `protocol == "lgcnp"`, `online == true`
- `capabilities == ["status","control"]`
- `metadata.group == "prod"` (metadata 객체 포함됨)
- `state.error_count == 2` (state 객체 포함됨)
- `last_seen` 는 RFC3339 형식의 string 이어야 한다

### AC5.4: include_metadata=false 시 풍부 필드 생략

**Given** 위와 동일한 device, `include_metadata=false`
**When** Process 출력의 첫 번째 item 을 검사하면
**Then**:
- 핵심 필드(`id`, `name`, `type`, `protocol`, `agent_name`, `online`, `last_seen`, `source`, `capabilities`)는 **포함**되어야 한다
- `metadata` 키와 `state` 키는 **포함되지 않거나** 명시적으로 빈 객체 `{}` 여야 한다 (구현 정책 일관성 유지)

### AC5.5: agents 항목 스키마 검증

**Given** `source=agents`, agent.Manager 에 1개 agent (id="a1", name="serial-agent", type="serial", state="running")
**When** Process 출력의 첫 번째 item 을 검사하면
**Then**:
- `id == "a1"`, `name == "serial-agent"`, `type == "serial"`, `state == "running"`
- `include_metadata=true` 일 때 `info` 객체가 포함되어야 한다 (AgentInfo 직렬화)

### AC5.6: flows 항목 스키마 검증

**Given** `source=flows`, FlowRegistry 가 1개 flow 반환 (id="f1", name="my-flow", state="running", node_count=5, wire_count=4)
**When** Process 출력의 첫 번째 item 을 검사하면
**Then**:
- `id == "f1"`, `name == "my-flow"`, `state == "running"`
- `node_count == 5`, `wire_count == 4`

### AC5.7: nodes 항목 스키마 검증

**Given** `source=nodes`, node.Registry 에 빌트인 노드 등록 완료
**When** Process 출력 items 를 검사하면
**Then**:
- 항목 수는 `node.Registry.AllTypeMeta()` 의 길이와 동일해야 한다
- 각 항목은 `type`, `category`, `description`, `origin` 필드를 가져야 한다
- `inventory` 노드 자기 자신도 항목에 포함되어야 한다 (`type == "inventory"`)

### AC5.8: metadata 키 표준

**Given** 임의의 valid inventory 노드 + Process 호출
**When** 출력 메시지의 metadata 를 검사하면
**Then**:
- `inventory.source` 키가 존재하고 값이 source enum 문자열이어야 한다
- `inventory.count` 키가 존재하고 값이 전체 항목 수의 string 표현이어야 한다 (예: `"5"`)
- 모든 metadata 값은 string 타입이어야 한다 (다른 노드와의 일관성)

### AC5.9: per_item 모드의 index/total metadata

**Given** `source=devices`, `emit_shape=per_item`, 3개 device
**When** 3개 출력 메시지의 metadata 를 검사하면
**Then**:
- 메시지[0]: `inventory.index == "0"`, `inventory.total == "3"`
- 메시지[1]: `inventory.index == "1"`, `inventory.total == "3"`
- 메시지[2]: `inventory.index == "2"`, `inventory.total == "3"`
- array 모드 출력에는 `inventory.index` 와 `inventory.total` 가 **없어야** 한다

### AC5.10: 입력 metadata 보존 (per_item 모드)

**Given** 입력 메시지 metadata 에 `trigger.schedule_id = "sched-1"`, `trigger.tick_count = "42"` 가 있고, `emit_shape=per_item`, 3개 device
**When** 3개 출력 메시지의 metadata 를 검사하면
**Then**:
- 각 출력 메시지의 metadata 에 `trigger.schedule_id == "sched-1"` 가 포함되어야 한다
- 각 출력 메시지의 metadata 에 `trigger.tick_count == "42"` 가 포함되어야 한다
- 출력 메시지의 `inventory.*` 키는 입력 metadata 의 `inventory.*` 키를 덮어쓴다 (충돌 시 inventory 우선)

### AC5.11: 입력 metadata 의 inventory.* 키 충돌 처리

**Given** 입력 메시지 metadata 에 `inventory.source = "should_be_overwritten"` 가 존재
**When** Process 출력을 검사하면
**Then**:
- 출력 메시지의 `inventory.source` 는 노드 설정의 source 값으로 덮어써져야 한다
- 입력의 위조된 값은 보존되지 않아야 한다

---

## 6. M6: Web UI 통합 (nodeSchemas.ts)

### AC6.1: nodeSchemas 에 inventory 키 존재

**Given** `web/src/config/nodeSchemas.ts` 파일
**When** 노드 스키마 맵을 검사하면
**Then**:
- `inventory` 키가 존재해야 한다
- 값은 `description`, `inputDesc`, `outputDesc`, `configSchema`, `defaultPorts` 필드를 포함해야 한다

### AC6.2: source 필드는 4종 select

**Given** inventory 스키마의 `configSchema.fields`
**When** `source` 필드를 검사하면
**Then**:
- `type == 'select'`, `required == true`
- `options` 가 `['devices', 'agents', 'nodes', 'flows']` 와 정확히 일치해야 한다

### AC6.3: emit_shape 필드의 기본값

**Given** inventory 스키마의 `configSchema.fields`
**When** `emit_shape` 필드를 검사하면
**Then**:
- `type == 'select'`, `default == 'array'`
- `options` 가 `['array', 'per_item']` 과 정확히 일치해야 한다

### AC6.4: include_metadata 필드의 기본값

**Given** inventory 스키마
**When** `include_metadata` 필드를 검사하면
**Then**:
- `type == 'boolean'`, `default == true`

### AC6.5: filter 필드는 source=devices 일 때만 노출

**Given** inventory 스키마
**When** `filter` 필드의 `visibleWhen` 을 검사하면
**Then**:
- `visibleWhen.field == 'source'`
- `visibleWhen.value == 'devices'`

### AC6.6: defaultPorts 검증

**Given** inventory 스키마
**When** `defaultPorts` 를 검사하면
**Then**:
- 길이 2 의 배열이며
- `[0] == { name: 'in', direction: 'input' }`
- `[1] == { name: 'out', direction: 'output' }`

### AC6.7: TypeScript 빌드 통과

**Given** 수정된 `nodeSchemas.ts`
**When** `pnpm build` 또는 `tsc --noEmit` 을 실행하면
**Then**:
- 타입 에러 없이 빌드가 성공해야 한다

### AC6.8: outputDesc 가 metadata 키를 명시

**Given** inventory 스키마의 `outputDesc` 문자열
**When** 내용을 검사하면
**Then**:
- `inventory.source` 키워드가 포함되어야 한다
- `inventory.count` 키워드가 포함되어야 한다
- per_item 모드 설명에 `inventory.index`, `inventory.total` 키워드가 포함되어야 한다

---

## 7. 통합 시나리오 (End-to-End)

### AC7.1: trigger → inventory(devices, array) → 검증

**Given** Flow 정의:
- node A: trigger (interval=100ms)
- node B: inventory (source=devices, emit_shape=array)
- wire: A.out → B.in
- DeviceRegistry 에 2개 device 사전 등록

**When** Flow 를 deploy + start 후 500ms 대기하면
**Then**:
- B 노드의 out 포트로 최소 1개 이상의 메시지가 emit 되어야 한다
- 각 출력의 payload.count == 2

### AC7.2: trigger → inventory(devices, per_item) → 검증

**Given** 위 시나리오에서 inventory 의 `emit_shape=per_item`, 3개 device
**When** Flow 1회 tick 후
**Then**:
- B 노드의 out 포트로 정확히 3개 메시지가 emit 되어야 한다 (한 tick 당)
- 3개 메시지의 `inventory.index` 가 `0`, `1`, `2` 를 정확히 한 번씩 포함해야 한다

### AC7.3: registry 변경의 즉시 반영

**Given** 위 시나리오 진행 중, trigger 가 1초 주기로 tick
**When** 도중에 새 device 가 DeviceRegistry 에 추가되면
**Then**:
- 추가 이후의 다음 tick 에서 emit 되는 메시지의 count 가 새 device 를 반영해야 한다
- 이미 emit 된 메시지는 변경되지 않는다

### AC7.4: xflowd 시작 후 inventory 노드 사용 가능

**Given** main.go 가 Phase 5 의 resolver 주입을 완료한 xflowd 바이너리
**When** xflowd 를 시작하고 API `GET /api/v1/nodes/types` 를 호출하면 (또는 동등한 internal 호출)
**Then**:
- 응답 목록에 `type=inventory` 항목이 포함되어야 한다
- category 는 `processing` 이어야 한다

---

## 8. 회귀 및 비기능 (Regression and Non-Functional)

### AC8.1: 기존 빌트인 노드 회귀 없음

**Given** Phase 5 완료 후 빌드된 xflowd
**When** `node.Registry.Types()` 를 호출하면
**Then**:
- 기존 빌트인 노드(`trigger`, `filter`, `transform`, `bridge`, `framer` 등)가 모두 포함되어야 한다
- 누락된 노드 타입이 없어야 한다

### AC8.2: inventory 노드는 read-only 동작

**Given** inventory 노드 1000회 반복 Process
**When** 각 Process 후 DeviceRegistry/AgentManager/FlowRegistry/NodeRegistry 의 상태를 검사하면
**Then**:
- 어떤 레지스트리도 inventory Process 호출로 인해 변경되지 않아야 한다
- device/agent 의 추가, 삭제, 상태 변경이 발생하지 않아야 한다

### AC8.3: 동시성 안전

**Given** 단일 inventory 노드 인스턴스
**When** 10개의 goroutine 이 동시에 `Process` 를 1000회씩 호출하면
**Then**:
- panic 없이 완료되어야 한다
- 각 Process 의 출력 payload 는 일관된 스키마를 가져야 한다 (Race detector PASS: `go test -race`)

### AC8.4: 큰 인벤토리 처리

**Given** DeviceRegistry 에 1,000개 device 등록, `emit_shape=per_item`
**When** Process 를 호출하면
**Then**:
- 1,000개 메시지를 정상 emit 해야 한다
- Process 호출은 합리적 시간 내(예: 1초 미만) 완료되어야 한다 (정확한 임계는 환경 의존)

### AC8.5: lint 및 coverage

**Given** Phase 6 완료된 코드베이스
**When** `golangci-lint run ./internal/node/...` 및 `go test -cover ./internal/node/...` 실행
**Then**:
- lint 신규 경고가 0 개여야 한다
- inventory 관련 코드의 coverage 가 85% 이상이어야 한다

---

## 9. Definition of Done 체크리스트

본 SPEC 이 완료되었다고 선언하기 위한 최종 체크리스트:

- [ ] **M1 인수**: AC1.1 ~ AC1.5 모두 PASS
- [ ] **M2 인수**: AC2.1 ~ AC2.7 모두 PASS
- [ ] **M3 인수**: AC3.1 ~ AC3.6 모두 PASS
- [ ] **M4 인수**: AC4.1 ~ AC4.6 모두 PASS
- [ ] **M5 인수**: AC5.1 ~ AC5.11 모두 PASS
- [ ] **M6 인수**: AC6.1 ~ AC6.8 모두 PASS
- [ ] **통합 인수**: AC7.1 ~ AC7.4 모두 PASS
- [ ] **비기능 인수**: AC8.1 ~ AC8.5 모두 PASS
- [ ] `go test ./...` 전체 PASS, regression 0
- [ ] `golangci-lint run` 신규 경고 0
- [ ] Web UI `pnpm build` 성공
- [ ] PR 본문에 본 acceptance.md 의 체크박스 체크 결과 첨부
