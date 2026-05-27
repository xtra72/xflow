---
id: SPEC-INVENTORY-001
title: Inventory 노드 - 디바이스/에이전트/노드/플로우 인벤토리 스냅샷 emit
version: 0.3.0
status: planned
created: 2026-05-25
updated: 2026-05-25
author: xtra
priority: medium
tags: [node, inventory, snapshot, registry, processing]
related_spec: SPEC-NODE-001, SPEC-DEVICE-001, SPEC-AGENT-001, SPEC-FLOW-001, SPEC-ENGINE-001
---

# SPEC-INVENTORY-001: Inventory 노드 - 디바이스/에이전트/노드/플로우 인벤토리 스냅샷 emit

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-INVENTORY-001 |
| 제목 | Inventory 노드 - 디바이스/에이전트/노드/플로우 인벤토리 스냅샷 emit |
| 버전 | 0.2.0 |
| 상태 | planned |
| 작성일 | 2026-05-25 |
| 작성자 | xtra |
| 우선순위 | medium |
| 관련 SPEC | SPEC-NODE-001 (Node 시스템), SPEC-DEVICE-001 (Device Registry), SPEC-AGENT-001 (Agent Manager), SPEC-FLOW-001 (Flow 그래프), SPEC-ENGINE-001 (Flow Engine) |
| 도메인 | Node System / Inventory Snapshot / Registry Access |

---

## HISTORY

- **0.3.0** (2026-05-26): SPEC-DEVICE-IDENTITY-001 Phase B § B-T8 정합화 — `devices` source 의 payload 에서 `device_uuid` 키를 `uid` 로 정규화한다. UUID 1급 키는 `uid` 이며, v0.2.0 호환을 위해 `device_uuid` 도 alias 로 함께 emit 된다 (Deprecated, v0.4.0 또는 xflowd v1.0 에서 제거 예정). 두 키는 항상 동일한 UUID 값을 가진다. UUID 가 비어 있으면 두 키 모두 생략한다 (graceful degradation 유지). downstream 신규 소비자는 `uid` 사용 권장 (SPEC-DEVICE-IDENTITY-001 § M2 의 emit payload 키 통일성과 정렬).
- **0.2.0** (2026-05-25): `devices` source 의 payload 에 `device_uuid` 필드 추가 — `agent.ResolveDeviceID(ctx, agentName, localID)` 로 글로벌 UUID 를 노출. 기존 `id` (composite key `"agent_name:local_id"`) 는 그대로 유지되어 v0.1.0 와 후위 호환된다. UUID 가 비어 있으면 (저장소 미설정 / 매핑 부재) `device_uuid` 키 자체를 생략하여 graceful degradation 한다. 사용 사례: 에이전트 rename 에도 안정적인 시계열 tag 키 / MQTT topic 식별자. `agents`/`nodes`/`flows` source 는 변경 없음 (디바이스 전용 개념).
- **0.1.0** (2026-05-25): 최초 작성 — `inventory` 노드 타입 신설. 4종 source(devices/agents/nodes/flows) × 2종 emit_shape(array/per_item) × 선택적 DeviceFilter 재사용. NodeOption 4종(WithDeviceRegistry/WithAgentManager/WithFlowRegistry/WithNodeRegistry) 신규 도입.

| Version | Date       | Author | Change                                                                      |
| ------- | ---------- | ------ | --------------------------------------------------------------------------- |
| 0.3.0   | 2026-05-26 | xtra   | device_uuid → uid 정규화 (device_uuid alias 유지, SPEC-DEVICE-IDENTITY-001 § B-T8) |
| 0.2.0   | 2026-05-25 | xtra   | devices source 의 payload 에 `device_uuid` (UUID) 필드 추가 (v0.1.0 호환)   |
| 0.1.0   | 2026-05-25 | xtra   | 최초 작성 — inventory 노드 타입 신설, 4종 source/2종 emit_shape/filter 재사용 |

---

## 1. 개요 (Overview)

### 1.1 배경

xflow 는 노드 기반 플로우 엔진(Node-RED 스타일)으로 메시지를 처리하며, 디바이스 레지스트리(`device.DeviceRegistry`), 에이전트 매니저(`agent.Manager`), 플로우 엔진(`engine.Engine`), 노드 레지스트리(`node.Registry`)를 in-process 로 관리한다.

기존의 `trigger` 노드(SPEC-NODE-001 계열)는 스케줄(interval/cron/once/times)에 따라 정적/템플릿 페이로드를 emit 한다. 그러나 사용자가 주기적으로 "현재 디바이스 목록", "에이전트 목록", "노드 목록", "플로우 목록"의 스냅샷을 다른 노드(transform, mqtt-publisher, store-write, chart-emitter 등)로 흘려보내려면 별도의 fetch 메커니즘이 필요하다.

현재 옵션은 다음과 같다:

1. `script` 노드에서 직접 in-process Go 객체에 접근 — 불가능 (script 노드는 메시지 변환만 담당, 레지스트리 접근 권한 없음)
2. `bridge` 노드로 system 에이전트와 통신 — 가능하나 표현 불편 (system 에이전트의 인벤토리 노출 API 부재)
3. HTTP 클라이언트 노드로 `/api/v1/devices` 등을 호출 — 외부 왕복 비용, 인증·네트워크 의존
4. **신규 in-process 인벤토리 노드** — 본 SPEC 의 접근

### 1.2 근본 원인 분석

**원인 1 - 인벤토리 데이터의 in-process 접근 경로 부재**

`internal/api/handler/device.go` 의 `DeviceHandler` 는 `device.DeviceRegistry.List(DeviceFilter)` 를 호출하여 디바이스 목록을 반환한다. 이 의존성은 HTTP 계층에만 노출되며, 노드 계층에는 노출되지 않는다. 같은 메커니즘으로 `agent.Manager.List()`, `engine.Engine.ListFlows()`, `node.Registry.AllTypeMeta()` 도 노드 레벨에서 접근 가능한 경로가 없다.

**원인 2 - 스케줄 기반 인벤토리 동기화의 표현력 부족**

플로우 그래프에서 "1분마다 모든 디바이스의 online 상태를 MQTT 로 발행"하려면 trigger 노드와 인벤토리 fetch 노드의 체이닝이 자연스럽다. 그러나 fetch 노드가 없으므로 사용자는 외부 스크립트 또는 별도 system 에이전트 확장에 의존해야 한다.

**원인 3 - 항목별 fan-out 처리 부재**

대시보드/스토리지 시나리오에서는 "디바이스마다 별도 메시지를 흘려서 downstream filter/transform 에 1개씩 처리" 하는 패턴이 일반적이다. trigger 노드는 배열 payload 를 단일 메시지로만 emit 하므로, 사용자가 별도의 `split`/`fan-out` 노드를 작성해야 한다.

### 1.3 제안 기능

1. `inventory` 라는 새로운 처리 노드를 `internal/node/` 에 도입한다. 이 노드는 임의의 입력 메시지(보통 trigger 노드의 tick)를 받으면 해당 시점의 in-process 레지스트리 스냅샷을 가져와 emit 한다.
2. `config.source` enum 으로 4종 인벤토리(`devices` | `agents` | `nodes` | `flows`) 중 하나를 선택한다.
3. `config.emit_shape` enum 으로 emit 형태를 선택한다: `array` (기본, 단일 메시지에 배열) 또는 `per_item` (항목별 N개 메시지 fan-out).
4. `config.filter` (선택) — `source=devices` 일 때만 적용. 기존 `device.DeviceFilter` (protocol/agent/type/online/group/tags) 를 재사용한다. 다른 source 에서는 무시되며 경고 로그가 남는다.
5. `config.include_metadata` (bool, default `true`) — payload 또는 items 내 각 항목에 풍부한 메타데이터(예: device.Metadata, agent.Info)를 포함할지 여부.
6. 의존성은 NodeOption 으로 주입한다: `WithDeviceRegistry`, `WithAgentManager`, `WithFlowRegistry`, `WithNodeRegistry`. 노드 생성 시점에는 4개 모두 옵셔널이지만, `Init`/`Process` 시 선택된 source 에 필요한 옵션이 미주입이면 명확한 에러를 반환한다.

### 1.4 핵심 원칙

1. **소스 무관성**: inventory 노드는 입력 메시지의 내용에 의존하지 않는다. 입력은 단지 "스냅샷을 찍어라"는 트리거이며, 페이로드/메타데이터는 무시된다(단, 메타데이터의 일부는 inventory 가 첨가하는 출력 메타데이터에 의해 override 되지 않는다 — 별도 keyspace `inventory.*` 사용).
2. **스냅샷 일관성**: 단일 `Process` 호출 내에서 source 데이터는 한 번만 조회된다. List 호출 사이에 레지스트리가 변동하더라도, 동일 `Process` 가 emit 하는 메시지들은 동일한 스냅샷에 기반한다.
3. **fan-out 격리**: `emit_shape=per_item` 일 때 N개 메시지는 각각 독립된 `message.Message` 인스턴스이며, payload 와 metadata 는 항목별로 분리된다. `inventory.index` 와 `inventory.total` 메타데이터로 순서·전체 크기를 알 수 있다.
4. **필터 재사용**: 기존 `device.DeviceFilter` 구조체와 `Matches` 메서드를 그대로 사용한다. inventory 노드 안에서 별도의 필터 코드를 새로 구현하지 않는다.
5. **명확한 의존성 에러**: source 가 `devices` 인데 `WithDeviceRegistry` 가 주입되지 않으면 `Init` 단계에서 명시적인 sentinel 에러(`ErrInventoryDeviceRegistryNotAvailable`)를 반환한다. 부분적·암묵적 동작을 허용하지 않는다.
6. **하위 호환성**: 기존 노드 타입과 NodeOption 인터페이스는 변경되지 않는다. 본 SPEC 은 추가만 수행하며, 기존 빌트인 노드의 동작에 영향이 없어야 한다.
7. **읽기 전용**: inventory 노드는 어떠한 레지스트리도 변경하지 않는다(read-only). 단순 List/Snapshot 호출만 사용한다.

---

## 2. 환경 (Environment)

### 2.1 현재 아키텍처

**Device Registry**

- `internal/device/device.go`: `Device` 인터페이스 (ID, Name, Type, Protocol, AgentName, Online, LastSeen, State, Metadata, Source, Capabilities)
- `internal/device/registry.go`: `DeviceRegistry` 인터페이스, `List(filter DeviceFilter) []Device`, `Get(id) (Device, error)`, `Count() int`
- `internal/device/filter.go`: `DeviceFilter` 구조체 (Protocol/AgentName/Type/Online/Tags/Group), `Matches(d Device) bool`

**Agent Manager**

- `internal/agent/manager.go`: `Manager` 인터페이스, `List() []Agent`, `Get(id) (Agent, error)`, `Summary() ManagerSummary`
- `internal/agent/agent.go`: `Agent` 인터페이스, `Info() AgentInfo`, `Stats() StatsSnapshot`

**Flow Engine**

- `internal/engine/engine.go`: `Engine` 구조체, `ListFlows() []FlowStatus`, `GetFlowStatus(flowID) (FlowStatus, error)`

**Node Registry**

- `internal/node/registry.go`: `node.Registry` 구조체, `AllTypeMeta() []NodeTypeMeta`, `Types() []string`
- `NodeTypeMeta`: Type, Category, Description, Source

**Node 계층 NodeOption 패턴**

- `internal/node/base.go`: `type NodeOption func(*BaseNode)` — 모든 노드 팩토리 시그니처 `func(def flow.NodeDef, opts ...NodeOption) (Node, error)`
- 옵션은 `BaseNode.config["_xxx_resolver"]` 또는 유사한 패턴으로 저장됨 (참고: `WithAgentResolver` in `internal/node/bridge.go`)

### 2.2 구현 경로

| 영역 | 파일 | 변경 유형 |
|------|------|----------|
| 노드 구현 | `internal/node/inventory.go` | 신규 |
| 노드 테스트 | `internal/node/inventory_test.go` | 신규 |
| NodeOption | `internal/node/inventory.go` (또는 `base.go` 보강) | 신규 |
| Registry 등록 | `internal/node/registry.go` | 1줄 추가 |
| 의존성 주입 | `cmd/xflowd/main.go` | engine NodeOptions 에 4종 resolver 주입 |
| Web UI 스키마 | `web/src/config/nodeSchemas.ts` | inventory 노드 스키마 추가 |

### 2.3 SDD 2025 Constitution 정합성

- Go 1.23+ 기존 기술 스택 (신규 의존성 없음)
- `device.DeviceFilter`, `device.DeviceRegistry`, `agent.Manager`, `engine.Engine`, `node.Registry` 모두 기존 in-process 객체 재사용
- TypeScript 5.9+ Web UI: 기존 `ConfigField` / `ConfigSchema` 타입만 사용 (Zod 등 신규 의존성 없음)
- 읽기 전용 노드이므로 보안 측면에서 새로운 위협 표면 추가 없음

---

## 3. EARS 요구사항 (EARS Requirements)

본 SPEC 은 6개 EARS 모듈로 구성된다.

---

### M1: 노드 정의와 Source Enum

- **Ubiquitous**: 시스템은 `inventory` 타입의 노드를 `internal/node.Registry` 의 빌트인 노드 목록에 등록해야 한다. 카테고리는 `"processing"` 이며 설명은 한국어로 명시한다.
- **Ubiquitous**: inventory 노드는 `config.source` 필드를 enum 으로 지원해야 한다. 허용 값: `"devices"`, `"agents"`, `"nodes"`, `"flows"`. 기본값은 존재하지 않으며 필수 필드이다.
- **Unwanted**: WHEN `config.source` 가 누락되거나 4종 enum 외의 임의 문자열이면, THEN 노드 팩토리는 `ErrInventoryInvalidSource` 를 반환해야 한다.
- **Ubiquitous**: inventory 노드의 기본 포트는 입력 `["in"]`, 출력 `["out"]`, 에러 `["error"]` 이다. 입력이 들어오면 항상 1개 이상의 출력 또는 에러를 emit 한다.

---

### M2: emit_shape 동작 (Array / Per-Item)

- **Ubiquitous**: 시스템은 `config.emit_shape` 필드를 enum 으로 지원해야 한다. 허용 값: `"array"`, `"per_item"`. 기본값은 `"array"`.
- **State-driven**: WHILE `emit_shape == "array"`, 입력 메시지 1개당 정확히 1개의 출력 메시지를 emit 해야 한다. 출력 payload 는 다음 형태를 가진다:
  ```
  {
    "source": "<source-enum>",
    "count": <int>,
    "items": [<item>, <item>, ...]
  }
  ```
  단, 스냅샷 시점에 항목이 0개여도 정상 emit 하며 `count=0`, `items=[]` 이다.
- **State-driven**: WHILE `emit_shape == "per_item"`, 입력 메시지 1개당 스냅샷 항목 수만큼(N개)의 출력 메시지를 emit 해야 한다. 각 출력 메시지의 payload 는 단일 항목 객체 자체이며 (위 `items` 배열 원소와 동일 스키마), 별도 wrapper 없이 직접 노출된다.
- **State-driven**: WHILE `emit_shape == "per_item"` AND 스냅샷 항목이 0개, THEN 어떤 출력 메시지도 emit 하지 않으며 에러도 발생하지 않아야 한다. (downstream 은 이 경우를 자연스럽게 처리할 수 있어야 한다.)
- **Unwanted**: WHEN `config.emit_shape` 가 2종 enum 외의 임의 문자열이면, THEN 노드 팩토리는 `ErrInventoryInvalidEmitShape` 를 반환해야 한다.

---

### M3: Filter 적용 (Devices 한정)

- **Optional**: WHERE `config.source == "devices"`, 시스템은 `config.filter` 필드를 옵셔널로 지원하며, 이 필드는 기존 `device.DeviceFilter` 의 yaml/json 표현을 그대로 사용한다 (`protocol`, `agent_name`, `type`, `online`, `tags`, `group`).
- **State-driven**: IF `config.source == "devices"` AND `config.filter` 가 제공되면, THEN inventory 노드는 `DeviceRegistry.List(filter)` 를 호출하여 필터가 적용된 디바이스 목록만 스냅샷에 포함해야 한다.
- **State-driven**: IF `config.source == "devices"` AND `config.filter` 가 없으면, THEN inventory 노드는 `DeviceRegistry.List(DeviceFilter{})` (빈 필터) 를 호출하여 모든 디바이스를 반환해야 한다.
- **Unwanted**: WHEN `config.source != "devices"` AND `config.filter` 가 제공되면, THEN 노드는 필터를 무시하고 경고 로그(`"inventory: filter ignored for non-device source"`)를 1회 출력해야 한다. 팩토리 또는 Process 에러는 발생하지 않는다.
- **Unwanted**: WHEN `config.filter.tags` 가 비-string 슬라이스(예: `[1, 2]`)이거나 `online` 이 bool 외 타입이면, THEN 노드 팩토리는 `ErrInventoryInvalidFilter` 를 반환해야 한다.

---

### M4: 의존성 주입 및 부재 시 동작

- **Ubiquitous**: 시스템은 다음 4종 `NodeOption` 함수를 신규 도입해야 한다. 모두 `internal/node` 패키지의 외부 API 이다.
  - `WithDeviceRegistry(reg device.DeviceRegistry) NodeOption`
  - `WithAgentManager(mgr agent.Manager) NodeOption`
  - `WithFlowRegistry(eng FlowRegistry) NodeOption` — `FlowRegistry` 인터페이스는 `ListFlows() []FlowStatus` 메서드만 요구하는 작은 인터페이스이며, `*engine.Engine` 이 이를 만족한다.
  - `WithNodeRegistry(reg *node.Registry) NodeOption`
- **State-driven**: IF `cmd/xflowd/main.go` 에서 engine 의 NodeOptions 에 위 4종 resolver 가 모두 주입되면, THEN 모든 source 옵션에 대해 inventory 노드가 정상 동작해야 한다.
- **Unwanted**: WHEN inventory 노드의 `Init` 시점에 선택된 `source` 에 필요한 옵션이 미주입이면, THEN `Init` 은 다음 sentinel 에러를 반환해야 한다:
  - `source=devices` → `ErrInventoryDeviceRegistryNotAvailable`
  - `source=agents` → `ErrInventoryAgentManagerNotAvailable`
  - `source=flows` → `ErrInventoryFlowRegistryNotAvailable`
  - `source=nodes` → `ErrInventoryNodeRegistryNotAvailable`
- **Unwanted**: inventory 노드는 의존성 부재를 부분적으로 우회하거나 빈 결과로 대체하지 않아야 한다. 즉 옵션 미주입 시 명시적으로 실패해야 한다.

---

### M5: 메시지 스키마 및 Metadata 키

- **Ubiquitous**: `emit_shape == "array"` 의 출력 메시지 payload 는 다음 키를 포함해야 한다:
  - `source` (string) — 4종 enum 중 하나
  - `count` (int) — `items` 의 길이와 일치
  - `items` (array of object) — source 별 항목 스키마 (M5 보조 절 참조)
- **Ubiquitous**: `emit_shape == "per_item"` 의 출력 메시지 payload 는 source 별 항목 객체 자체이다 (위 `items` 원소와 동일 스키마, wrapper 없음).
- **Ubiquitous**: 모든 출력 메시지의 metadata 에는 다음 키가 추가되어야 한다 (모든 값은 string):
  - `inventory.source` — source enum 값
  - `inventory.count` — 스냅샷 전체 항목 수 (per_item 모드에서도 전체 N 동일)
- **Ubiquitous**: `emit_shape == "per_item"` 의 metadata 에는 추가로 다음 키가 포함되어야 한다:
  - `inventory.index` — 0-based 인덱스 (string 표현)
  - `inventory.total` — `inventory.count` 와 동일 (편의 alias)
- **State-driven**: IF `config.include_metadata == true` (기본), THEN 항목 객체는 풍부한 메타데이터를 포함한다 (M5 보조 절 참조).
- **State-driven**: IF `config.include_metadata == false`, THEN 항목 객체는 핵심 식별 필드(id, name 등)만 포함하고 metadata/state/properties 등은 생략된다.
- **Unwanted**: inventory 노드는 입력 메시지의 기존 metadata 키를 `inventory.*` 접두사 외의 영역에서 변경하지 않아야 한다. 입력 metadata 의 다른 키는 보존되거나 (per_item 모드에서 각 출력에 복제) 무시된다 (구현 정책으로 결정, 본 SPEC 은 `inventory.*` 접두사 충돌 방지만 강제).

**M5 보조 절 — Source 별 항목 스키마**

`source=devices` 항목 스키마:
- `id` (string) — `Device.ID()` — composite key (`"agent_name:local_id"` 형식, address 역할)
- `uid` (string, optional, v0.3.0+) — `Device.UID()` 의 결과인 글로벌 UUID (identity 역할). 1급 키. 매핑이 존재할 때만 포함되며, 미설정 / 매핑 부재 시 키 자체를 생략한다 (graceful degradation). 에이전트 rename 에도 안정적이므로 시계열 tag 키 / MQTT topic 식별자로 권장된다.
- `device_uuid` (string, optional, v0.2.0+, Deprecated v0.3.0+) — `uid` 의 호환 alias. 항상 `uid` 와 동일한 값을 가지며 (둘 다 포함되거나 둘 다 생략됨), v0.4.0 또는 xflowd v1.0 에서 제거 예정. 신규 소비자는 `uid` 사용 권장.
- `name` (string) — `Device.Name()`
- `type` (string) — `Device.Type()`
- `protocol` (string) — `Device.Protocol()`
- `agent_name` (string) — `Device.AgentName()`
- `online` (bool) — `Device.Online()`
- `last_seen` (string, RFC3339) — `Device.LastSeen()`
- `source` (string) — `Device.Source()` (config/auto)
- `capabilities` (array of string) — `Device.Capabilities()`
- `metadata` (object, optional) — `include_metadata=true` 일 때만 포함 (`name`, `tags`, `location`, `group`, `labels`, `pinned`)
- `state` (object, optional) — `include_metadata=true` 일 때만 포함 (`online`, `ready`, `last_seen`, `error_count`, `properties`)

> v0.3.0 도입 — `uid` (1급) + `device_uuid` (alias) 와 `id` 의 역할 구분:
> - `id` 는 composite key (address) 로, 에이전트 내부에서 디바이스를 가리킨다. 에이전트 rename 시 변경된다.
> - `uid` 는 글로벌 UUID (identity) 로, 디바이스의 영구 식별자다. 에이전트 rename 에도 불변이다. SPEC-DEVICE-IDENTITY-001 의 emit/REST/inventory 전반에서 1급 키로 사용된다.
> - `device_uuid` 는 v0.2.0 alias 로 호환 유지. 신규 소비자는 `uid` 사용 권장.
> - downstream 노드는 사용 목적에 따라 골라 쓸 수 있다:
>   - 시계열 DB tag 키 / MQTT topic: `uid` 권장 (rename 안전, 1급 키)
>   - 에이전트 컨텍스트 디버깅: `id` 권장 (사람이 읽기 쉬움)

`source=agents` 항목 스키마:
- `id` (string) — agent ID
- `name` (string) — agent name
- `type` (string) — agent type (e.g., `serial`, `socket`, `mqtt`)
- `state` (string) — lifecycle state (e.g., `running`, `stopped`)
- `info` (object, optional) — `include_metadata=true` 일 때만 포함 (`AgentInfo` 직렬화)
- `stats` (object, optional) — `include_metadata=true` 일 때만 포함 (`StatsSnapshot` 의 안전한 일부 필드)

`source=flows` 항목 스키마:
- `id` (string) — flow ID
- `name` (string) — flow name
- `state` (string) — flow lifecycle state
- `node_count` (int) — flow 내 노드 수
- `wire_count` (int) — flow 내 와이어 수
- `extra` (object, optional) — `include_metadata=true` 일 때만 포함 (`FlowStatus.Extra` 등)

`source=nodes` 항목 스키마:
- `type` (string) — `NodeTypeMeta.Type`
- `category` (string) — `NodeTypeMeta.Category`
- `description` (string) — `NodeTypeMeta.Description`
- `origin` (string) — `NodeTypeMeta.Source` (예: `"builtin"`)

> 직렬화 안정성: 모든 timestamp 필드는 RFC3339 문자열로 직렬화되며, nil 슬라이스/맵은 빈 배열/객체로 직렬화되어 downstream JSON 사용자가 nil 체크를 하지 않아도 되도록 한다.

---

### M6: Web UI 통합 (nodeSchemas.ts)

- **Ubiquitous**: 시스템은 `web/src/config/nodeSchemas.ts` 의 노드 스키마 맵에 `inventory` 키를 추가해야 한다. 스키마는 기존 `ConfigSchema` / `ConfigField` 타입을 사용한다.
- **Ubiquitous**: inventory 스키마의 `configSchema.fields` 는 다음을 포함해야 한다:
  - `source` (select, required) — `['devices', 'agents', 'nodes', 'flows']`
  - `emit_shape` (select, default `'array'`) — `['array', 'per_item']`
  - `include_metadata` (boolean, default `true`)
  - `filter` (object, optional, `visibleWhen: { field: 'source', value: 'devices' }`) — 하위 필드 `protocol` (string), `agent_name` (string), `type` (string), `online` (boolean), `group` (string), `tags` (string array)
- **Ubiquitous**: inventory 스키마의 `defaultPorts` 는 `[{ name: 'in', direction: 'input' }, { name: 'out', direction: 'output' }]` 이어야 한다.
- **Ubiquitous**: `description`, `inputDesc`, `outputDesc` 는 한국어로 작성되며, 출력 metadata 키(`inventory.source`, `inventory.count`, `inventory.index`, `inventory.total`) 를 명시한다.
- **Optional**: WHERE Web UI 가 `visibleWhen` 조건 분기를 지원하는 한, `filter` 필드는 `source=devices` 일 때만 노출되어야 한다 (다른 source 에서는 hidden).

---

## 4. 명세 (Specifications)

### 4.1 노드 Config 스키마

```yaml
# flow yaml 예시 - trigger → inventory → mqtt-publisher 체이닝
nodes:
  - id: tick-1m
    type: trigger
    config:
      schedules:
        - type: interval
          value: "1m"

  - id: dump-devices
    type: inventory
    config:
      source: "devices"           # 필수: devices | agents | nodes | flows
      emit_shape: "per_item"      # 기본 "array"
      include_metadata: true       # 기본 true
      filter:                      # source=devices 한정, 옵셔널
        protocol: "lg_icp01"
        online: true
        group: "production"
        tags: ["critical"]

  - id: publish
    type: mqtt-publisher
    config:
      topic_template: "inventory/devices/{id}"

wires:
  - from: { node: tick-1m, port: out }
    to:   { node: dump-devices, port: in }
  - from: { node: dump-devices, port: out }
    to:   { node: publish, port: in }
```

### 4.2 출력 메시지 예시

**emit_shape=array (source=devices, 2개 매칭):**

```json
{
  "payload": {
    "source": "devices",
    "count": 2,
    "items": [
      {
        "id": "lg-hvacr01-agent-1:0.0.16",
        "name": "Indoor Unit A",
        "type": "HVACR.IDU",
        "protocol": "lg_icp01",
        "agent_name": "lg-hvacr01-agent-1",
        "online": true,
        "last_seen": "2026-05-25T10:30:00Z",
        "source": "auto",
        "capabilities": ["status", "control"],
        "metadata": {
          "name": "Room A",
          "tags": ["critical", "production"],
          "location": "Floor 1",
          "group": "production",
          "labels": {"zone": "north"},
          "pinned": true
        },
        "state": {
          "online": true,
          "ready": true,
          "last_seen": "2026-05-25T10:30:00Z",
          "error_count": 0,
          "properties": {"temperature": 22.5}
        }
      },
      {
        "id": "lg-hvacr01-agent-1:0.0.17",
        "name": "Indoor Unit B",
        "type": "HVACR.IDU",
        "protocol": "lg_icp01",
        "agent_name": "lg-hvacr01-agent-1",
        "online": true,
        "last_seen": "2026-05-25T10:30:00Z",
        "source": "auto",
        "capabilities": ["status", "control"],
        "metadata": { "...": "..." },
        "state": { "...": "..." }
      }
    ]
  },
  "metadata": {
    "inventory.source": "devices",
    "inventory.count": "2"
  }
}
```

**emit_shape=per_item (동일 스냅샷, 2개 메시지):**

```json
// 메시지 1
{
  "payload": {
    "id": "lg-hvacr01-agent-1:0.0.16",
    "name": "Indoor Unit A",
    "type": "HVACR.IDU",
    "protocol": "lg_icp01",
    "agent_name": "lg-hvacr01-agent-1",
    "online": true,
    "...": "..."
  },
  "metadata": {
    "inventory.source": "devices",
    "inventory.count": "2",
    "inventory.index": "0",
    "inventory.total": "2"
  }
}

// 메시지 2
{
  "payload": { "id": "lg-hvacr01-agent-1:0.0.17", "...": "..." },
  "metadata": {
    "inventory.source": "devices",
    "inventory.count": "2",
    "inventory.index": "1",
    "inventory.total": "2"
  }
}
```

### 4.3 NodeOption API 추가

```go
// internal/node/inventory.go (또는 base.go 보강)

// WithDeviceRegistry 는 inventory 노드에 DeviceRegistry 를 주입하는 옵션이다.
func WithDeviceRegistry(reg device.DeviceRegistry) NodeOption

// WithAgentManager 는 inventory 노드에 agent.Manager 를 주입하는 옵션이다.
func WithAgentManager(mgr agent.Manager) NodeOption

// FlowRegistry 는 inventory 노드가 요구하는 최소 인터페이스이다.
// engine.Engine 이 이를 만족한다. 별도 인터페이스로 추출하여 노드 패키지가
// engine 패키지에 의존하지 않도록 한다 (import cycle 방지).
type FlowRegistry interface {
    ListFlows() []FlowStatus // FlowStatus 는 노드 패키지에서 재정의 또는 engine 의 타입 재사용
}

// WithFlowRegistry 는 inventory 노드에 FlowRegistry 를 주입하는 옵션이다.
func WithFlowRegistry(reg FlowRegistry) NodeOption

// WithNodeRegistry 는 inventory 노드에 *node.Registry 를 주입하는 옵션이다.
// 자기 자신을 가리키므로 약한 참조 또는 lazy 주입 패턴이 필요할 수 있다.
func WithNodeRegistry(reg *Registry) NodeOption
```

> **구현 메모**: `FlowRegistry` 인터페이스를 노드 패키지에 두면 engine → node 단방향 의존성이 유지된다. 만약 `FlowStatus` 가 engine 패키지 타입이라면 노드 패키지는 `engine.FlowStatus` 를 import 해야 하므로, 필요시 `FlowStatus` 도 노드 패키지(또는 공용 pkg)로 추출하거나, 노드 인터페이스를 `ListFlows() []any` 로 약화하고 inventory 가 reflection 으로 처리하는 방안을 plan.md 에서 검토한다.

### 4.4 에러 모델

inventory 노드 전용 sentinel 에러 (`internal/node/inventory.go`):

- `ErrInventoryInvalidSource` — `config.source` 누락 또는 4종 enum 외
- `ErrInventoryInvalidEmitShape` — `config.emit_shape` 가 2종 enum 외
- `ErrInventoryInvalidFilter` — `config.filter` 의 타입 불일치 (예: tags 가 string 슬라이스 아님)
- `ErrInventoryDeviceRegistryNotAvailable` — `source=devices` 인데 `WithDeviceRegistry` 미주입
- `ErrInventoryAgentManagerNotAvailable` — `source=agents` 인데 `WithAgentManager` 미주입
- `ErrInventoryFlowRegistryNotAvailable` — `source=flows` 인데 `WithFlowRegistry` 미주입
- `ErrInventoryNodeRegistryNotAvailable` — `source=nodes` 인데 `WithNodeRegistry` 미주입

모든 에러는 `fmt.Errorf("inventory: %w: <detail>", ErrInvalidConfig)` 또는 유사 패턴으로 wrapping 하여, 기존 노드 에러 분류 체계(`ErrInvalidConfig`, `ErrNodeNotInitialized` 등)와 일관성을 유지한다.

---

## 5. 관련 SPEC (Related SPECs)

- **SPEC-NODE-001**: Node 시스템 기본 설계 (전제) — `node.Registry`, `NodeFactory`, `NodeOption` 패턴 제공
- **SPEC-DEVICE-001**: Device Registry (전제) — `device.DeviceRegistry`, `DeviceFilter`, `Device` 인터페이스 재사용
- **SPEC-AGENT-001 ~ SPEC-AGENT-006**: Agent Manager 와 lifecycle (전제) — `agent.Manager.List()`, `Agent.Info()`, `Agent.Stats()`
- **SPEC-FLOW-001**: Flow 그래프 (전제) — flow 정의 및 wire 라우팅
- **SPEC-ENGINE-001**: Flow Engine (전제) — `Engine.ListFlows()`, `FlowStatus`
- **SPEC-DASHBOARD-001** (예정/연관): 대시보드에서 inventory 노드 활용 예시 (디바이스 카드 그리드 자동 갱신 등) — 본 SPEC 의 후행 사용처

---

## 6. TAG Traceability

- @SPEC:SPEC-INVENTORY-001 → spec.md (이 문서)
- @PLAN:SPEC-INVENTORY-001 → plan.md
- @ACCEPTANCE:SPEC-INVENTORY-001 → acceptance.md
- 구현 경로 (예정):
  - `internal/node/inventory.go` (신규 — 노드 구현, NodeOption 4종, sentinel 에러)
  - `internal/node/inventory_test.go` (신규 — 단위 테스트)
  - `internal/node/registry.go` (registerBuiltins 에 `inventory` 1줄 추가)
  - `cmd/xflowd/main.go` (engine NodeOptions 에 4종 resolver 주입)
  - `web/src/config/nodeSchemas.ts` (inventory 스키마 추가)

---

## 7. Implementation Notes

### 7.1 의존성 주입 시점 결정

inventory 노드의 의존성 4종은 모두 in-process 객체이며 노드 생성 시점에 이미 존재한다. 따라서 NodeOption 으로 직접 주입하는 패턴이 자연스럽다. `bridge` 노드의 `WithAgentResolver` 처럼 `BaseNode.config` 의 private 키(`_device_registry` 등)에 저장한 뒤, inventory 팩토리가 type assertion 으로 꺼내는 방식을 따른다.

이렇게 하면 다음 두 가지가 동시에 달성된다:
- (a) `NodeFactory` 시그니처 `func(def flow.NodeDef, opts ...NodeOption) (Node, error)` 가 그대로 유지됨
- (b) inventory 노드만 신규 옵션을 인지하고, 다른 노드는 옵션을 받아도 무시 (안전)

### 7.2 NodeRegistry 자기 참조 처리

`WithNodeRegistry(*Registry)` 는 자기 자신을 노드에 주입하는 형태이다. `engine` 이 `node.Registry` 를 만든 후 inventory 노드 옵션으로 `&registry` 를 넘기는 패턴이 자연스럽다. 다만 noderegistry 자기 참조에 의한 GC 사이클은 신경 쓰지 않아도 된다(엔진 생명주기 종료 시 함께 해제).

### 7.3 FlowRegistry 인터페이스 vs 직접 의존

`*engine.Engine` 을 노드 패키지에서 직접 import 하면 import cycle 위험이 있다 (`engine` 이 `node` 를 import 하므로). 따라서 inventory 노드는 작은 인터페이스 `FlowRegistry { ListFlows() []FlowStatus }` 를 노드 패키지에 정의하고, `*engine.Engine` 이 자연스럽게 이를 만족하도록 한다. `FlowStatus` 타입의 위치는 plan.md 에서 추가 검토한다 (공용 패키지 추출 vs 노드 패키지 재정의 vs `any` 약화 중 택1).

### 7.4 read-only 보장

inventory 노드는 어떠한 setter/writer 도 호출하지 않는다:
- `DeviceRegistry.List()` / `Get()` — read-only
- `agent.Manager.List()` / `Get()` — read-only (Start/Stop/Restart 등은 호출 금지)
- `engine.Engine.ListFlows()` / `GetFlowStatus()` — read-only
- `node.Registry.AllTypeMeta()` / `Types()` — read-only

linter rule 또는 코드 리뷰 체크리스트에서 이를 강제한다.

### 7.5 Performance 고려

- `DeviceRegistry.List(filter)` 는 in-memory 순회이며 디바이스 수에 비례한다. 수천 단위까지 무난.
- `emit_shape=per_item` 일 때 N개 메시지를 생성하는 비용은 메시지 객체 할당이 지배적이다. trigger 의 schedule 주기가 1초 미만이고 디바이스가 수백 개 이상이면 GC 부담 증가 가능. plan.md 에서 메시지 풀링 도입 여부를 검토.
- 본 SPEC v0.1.0 범위에서는 풀링/최적화 없이 단순 구현. 후속 SPEC 에서 필요 시 최적화.

### 7.6 Status: planned

본 SPEC 은 작성 단계(`planned`)이며 구현이 시작되지 않았다. 구현 착수 시 status 를 `in_progress` 로 전환하고, 인수 기준 통과 시 `completed` 로 회귀한다.
