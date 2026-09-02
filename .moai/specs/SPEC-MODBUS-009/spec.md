---
id: SPEC-MODBUS-009
title: "MODBUS Client 디바이스 관리 UI 재구성 (설정탭 devices 숨김 + 장치탭 CRUD + 백엔드 list_devices/update_device)"
version: "0.1.0"
status: draft
created: 2026-08-04
updated: 2026-08-04
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "internal/agent/modbus"
lifecycle: spec-anchored
tags: "modbus, client, device-management, ui, list-devices, update-device, config-tab, devices-tab, i18n"
tier: M
---

# SPEC-MODBUS-009: MODBUS Client 디바이스 관리 UI 재구성

## HISTORY

| 버전  | 날짜       | 변경 내용 |
|-------|------------|-----------|
| 0.1.0 | 2026-08-04 | 초안 작성 (Draft) — `modbus-client`(type id, 패키지 `internal/agent/modbus`) 디바이스 관리 UX 재구성 명세. (1) 설정 탭에서 modbus-client의 `devices` 편집기 숨김, (2) 장치 탭에 modbus-client 전용 디바이스 관리(추가/제거/수정/목록) 섹션 추가, (3) 백엔드 `list_devices`/`update_device` 명령 추가. SPEC-MODBUS-008에서 구현된 런타임 `add_device`/`remove_device`를 활용·확장. 확정 설계 반영: 수정=신규 `update_device`(연결 유지 in-place), 설정탭 숨김=running-agent config 렌더에서만 제외, 장치탭=전용 섹션(`ModbusDevicesSection` 참조 패턴). REQ 5모듈(REQ-01~05), AC-01~09. 하위 호환은 HARD 게이트. |

---

## 1. 개요 (Overview)

xflow에는 MODBUS 클라이언트 에이전트(type id `modbus-client`, 패키지 `internal/agent/modbus/`)가 존재하며, SPEC-MODBUS-006/008을 통해 다중 디바이스 폴링, per-device transport(TCP/RTU), 세션 공유(`share_session`), 런타임 디바이스 등록(`add_device`/`remove_device`), 프레임 로그 관측성이 이미 구현되어 있다.

그러나 **디바이스 관리 UX**는 아직 미완성이다. 현재:

- **설정 탭**은 `modbus_devices` 스키마 필드(에디터)를 통해 디바이스 배열 전체를 편집하도록 노출하지만, 이는 런타임 add/remove/update 흐름과 이원화되어 사용자에게 혼란을 준다.
- **장치 탭**은 modbus-client에 대해 관리 기능 없이 read-only 실시간 목록만 제공한다(`canManageDevices` 분기에서 modbus-client는 fall-through). 반면 modbus-gateway·NASA·LGAP·LG-ICP는 장치 탭에서 관리(추가/삭제)가 가능하다.
- **백엔드**는 modbus-client `Process()` 명령에 `list_devices`·`update_device`가 없어(`add_device`/`remove_device`만 존재), UI가 목록 조회나 연결 유지형 수정을 수행할 수 없다.

본 SPEC은 modbus-client의 디바이스 관리 UX를 다음 3축으로 재구성한다.

- **F1 — 백엔드 `list_devices`**: 현재 디바이스 목록 스냅샷을 반환하는 신규 명령. modbus-gateway가 이미 제공하는 `list_devices`(`internal/agent/modbusserver/agent.go:325`)와 정렬된 표면.
- **F2 — 백엔드 `update_device`**: 연결을 유지한 채 기존 디바이스의 register_groups/unit_id/poll_interval/timeout/reconnect 등을 in-place로 변경하는 신규 명령. `set_config`의 기존 디바이스 재구성 패턴을 재사용한다(제거+재추가 아님).
- **F3 — 프론트엔드 재구성**: (a) 설정 탭에서 modbus-client의 `devices` 편집기를 숨기고, (b) 장치 탭에 modbus-client 전용 관리 섹션(추가/제거/수정/목록)을 추가한다. `ModbusDevicesSection`(modbus-gateway 전용)을 참조 패턴으로 삼는다.

본 SPEC은 문서만 산출하며 구현 코드를 포함하지 않는다. 기존 `modbus-client` 동작과 type id는 반드시 보존한다(하위 호환, DDD/hybrid).

> **주의(패키지 혼동 금지)**: 본 SPEC의 대상은 클라이언트 `internal/agent/modbus`(type id `modbus-client`)이다. 서버(slave)인 `internal/agent/modbusserver`(type id `modbus-gateway`)는 변경 대상이 아니며, F3의 장치 탭 섹션·백엔드 `list_devices` 표면에서 **참조 패턴**으로만 활용한다.

### 1.1 확정 설계 결정 (Locked — 사용자 확정)

1. **수정(edit) = 신규 `update_device` 명령**: 백엔드에 `update_device`를 추가해 **연결을 유지한 채** register_groups/unit_id/poll_interval/request_timeout/reconnect_interval 등을 in-place 변경한다. `processSetConfig`(set_config.go:33)의 "기존 디바이스/그룹 재구성" 패턴을 재사용한다. **제거+재추가 방식은 미채택**(연결 재설정 회피).
2. **설정 탭 devices 제거 = 실행 중 설정탭에서만 숨김**: 이미 생성된 에이전트의 `ConfigTab`(AgentDetailPanel.tsx:769)에서 modbus-client의 `devices` 필드(`modbus_devices` 에디터, agentSchemas.ts:106)만 조건부로 숨긴다. 에이전트 생성(CreateAgentModal) 시 초기 devices 부트스트랩 유지 여부는 구현 시 판단하되, **스키마 자체는 유지**하고 running-agent config 렌더에서만 제외하는 방향을 기본으로 한다.
3. **장치 탭 modbus-client 전용 섹션**: 기존 `ModbusDevicesSection`(modbus-gateway 전용, AgentDetailPanel.tsx:1255, `DevicesTab`에서 3663라인 분기)을 참조 패턴으로 삼아 modbus-client 전용 관리 섹션을 추가한다. **신규 React 컴포넌트는 허용**한다(장치 탭은 스키마 주도가 아니라 전용 섹션 방식). 디바이스 편집 폼은 `ModbusDevicesEditor`(web/src/components/property/ModbusDevicesEditor.tsx)의 modbus 필드를 재사용한다.
4. **하위 호환 = HARD 게이트**: 기존 `modbus-client` 동작·type id·기존 명령(read/write/set_config/add_device/remove_device)은 모두 오늘과 동일하게 동작해야 한다. 특성화/행위 보존 테스트로 강제한다.

---

## 2. 환경 (Environment)

| 항목 | 현재 상태 / 제약 (file:line 근거) |
|------|-----------------------------------|
| 백엔드 언어 | Go 1.23+ |
| 클라이언트 패키지 | `internal/agent/modbus/` (type id `modbus-client`) |
| 서버 패키지(구분) | `internal/agent/modbusserver/` (type id `modbus-gateway`) — 본 SPEC 변경 대상 아님, 참조 패턴만 |
| 클라이언트 Process 명령 | `ModbusAgent.Process`(agent.go:946) 문자열 스위치(agent.go:953-975): read_registers/get_status/get_cache/get_all_caches/write_coil/write_register/write_coils/write_registers/read_raw/set_config/**add_device**(973)/**remove_device**(975). **`list_devices`·`update_device`·`get_device_status` 없음** → F1/F2 신규 추가 대상 |
| 서버 Process 명령(참조) | `modbusserver/agent.go`에 `list_devices`(325)·`add_device`(329)·`remove_device`(331) 존재 → F1 `list_devices` 응답 표면의 정렬 대상 |
| 런타임 add/remove(SPEC-008) | `processAddDevice`(runtime_device.go:35), `processRemoveDevice`(runtime_device.go:140). `findDeviceLocked`(runtime_device.go:216), `buildRuntimeDeviceLocked`(runtime_device.go:232), copy-on-write clone 헬퍼(cloneCacheMap/cloneCounterMap 등, runtime_device.go:317~) — F2 update_device의 정합 대상 |
| set_config 재구성 패턴 | `processSetConfig`(set_config.go:33): register_groups/unit_id/poll_interval/request_timeout/reconnect_interval을 **기존 디바이스에** in-place 변경. `rejectInitOnlyFields`(set_config.go:204)로 transport 전환·RTU 하드웨어 파라미터 거부. `parseSetConfigRegisterGroups`(set_config.go:223) — F2 update_device가 재사용할 핵심 패턴 |
| 디바이스 설정 | `DeviceConfig`(config.go:50): ID, Host, Port, UnitID, RegisterGroups(55), per-device Transport(58), per-device Serial(60), per-device ShareSession *bool(65) — SPEC-008에서 확장됨 |
| 동시성 보호 | `a.mu`(RWMutex). devices/caches는 add/remove가 copy-on-write로 통째 교체(agent.go:647 주석, runtime_device.go clone 헬퍼) → F1 list_devices는 RLock 스냅샷, F2 update_device는 Lock 하 copy-on-write |
| 오류 규약 | `ErrMissingDeviceID`·`ErrDuplicateDevice`·`ErrDeviceNotFound`(runtime_device.go) — F2 미존재 ID 거부에 재사용 |
| 프론트 탭 구조 | `AgentDetailPanel.tsx`(web/src/pages/agents/): Tab 타입(118), `NO_DEVICES_TAB`(131 — modbus-client 미포함 → 장치 탭 이미 노출), `showDevices`(178) |
| 프론트 설정 탭 | `ConfigTab`(AgentDetailPanel.tsx:769) — 스키마 주도 폼 렌더. modbus-client `devices` 필드는 `agentSchemas.ts:106`(`modbus_devices` 타입)에서 오며 `FormField.tsx:337`이 `ModbusDevicesEditor`로 렌더 → F3(a) 숨김 대상 |
| 프론트 장치 탭 | `DevicesTab`(AgentDetailPanel.tsx:3570). 분기: modbus-gateway→`ModbusDevicesSection`(3663), xsfm→`XsfmDevicesTab`(3669), NASA/LGAP/LG-ICP→`canManageDevices`(3601, isNasa\|\|isLgap\|\|isLgIcp). **modbus-client은 fall-through → 관리 불가(read-only)** → F3(b) 신규 섹션 대상 |
| 프론트 exec 메커니즘 | `useExecAgent`(hooks/useAgent.ts:145) → `agentService.execAgent(id,{command,params})` → POST `/agents/{id}/exec`. `useDevicesRealtime`(hooks/useDevice.ts:70), `useDeleteDevice`(hooks/useDevice.ts:239, `remove_device` 호출:247) |
| list_devices 기존 사용 | 프론트는 `canManageDevices` 에이전트(AgentDetailPanel.tsx:3617)와 modbus-gateway `ModbusDevicesSection`에서 이미 `list_devices` 명령을 호출 중 → modbus-client에도 백엔드 `list_devices`가 추가되면 동일 패턴 재사용 가능 |
| 디바이스 편집 폼 | `ModbusDevicesEditor`(web/src/components/property/ModbusDevicesEditor.tsx:1248) — id/host/port/unit_id/register_groups/per-device transport·serial/share_session 필드 정의. F3(b) 섹션이 재사용 |
| i18n | `agents.detail.devices.*` 계열 키(ko.json) — F3 섹션이 확장·정합 |
| 개발 방법론 | hybrid — 신규 코드는 TDD, 기존 패키지 편집은 DDD(특성화 테스트). 커버리지 목표 85%, `code_comments: ko` |

### 2.1 현재 아키텍처 (ANALYZE)

- **디바이스 목록 조회 부재**: modbus-client `Process()`에는 `list_devices`가 없다(agent.go:953-975). 프론트가 modbus-client 디바이스 목록을 exec 경로로 얻을 방법이 없다. modbus-gateway는 이미 이를 제공한다(modbusserver/agent.go:325) → F1이 클라이언트에 동형 명령을 추가.
- **연결 유지형 수정 부재**: 기존 `set_config`(set_config.go:33)는 register_groups/unit_id/poll_interval/timeout/reconnect를 기존 디바이스에 변경하나, "디바이스 단위 수정" 명령 표면(`update_device`)은 없다. F2는 `set_config`의 재구성 로직을 디바이스-스코프 단일 명령으로 표면화한다.
- **UI 이원화**: 설정 탭(스키마 에디터, 전체 배열 편집)과 런타임 명령(add/remove)이 이원화되어 있다. F3은 설정 탭에서 편집기를 걷어내고, 장치 탭을 modbus-client의 단일 디바이스 관리 지점으로 통합한다.
- **참조 패턴**: modbus-gateway `ModbusDevicesSection`(AgentDetailPanel.tsx:1255)은 list_devices 실시간 목록 + 관리 UI 패턴을 이미 구현한다 → F3(b)의 구조적 참조.

---

## 3. 가정 (Assumptions)

| ID | 가정 사항 |
|----|-----------|
| A-1 | 기존 `modbus-client` type id와 동작(read/write/set_config/add_device/remove_device)은 그대로 유지되며, 본 SPEC은 동일 패키지 내 확장으로만 수행한다(하위 호환 HARD) |
| A-2 | `list_devices`는 부작용 없는 조회이며 `a.mu` RLock 하 스냅샷으로 devices/설정을 안전하게 읽는다(폴링 goroutine과 경합 없음) |
| A-3 | `update_device`는 대상 디바이스의 연결/트랜스포트를 재생성하지 않고 in-place로 register_groups/unit_id/케이던스만 변경한다(제거+재추가 아님) |
| A-4 | `update_device`가 변경할 수 없는 init 전용 필드(transport 전환·RTU 하드웨어 파라미터 등)는 `rejectInitOnlyFields`(set_config.go:204) 규칙에 준하여 거부한다 |
| A-5 | 존재하지 않는 디바이스 ID에 대한 `update_device`는 `ErrDeviceNotFound`로 원자적 거부하며 부분 적용하지 않는다 |
| A-6 | 설정 탭에서 modbus-client `devices` 필드를 숨겨도 다른 agentType(modbus-gateway 등)의 설정 폼과 기존 스키마 파이프라인에는 영향이 없다 |
| A-7 | 장치 탭 modbus-client 전용 섹션은 exec 경로(useExecAgent)로 add_device/remove_device/update_device/list_devices를 호출하며, 디바이스 편집 필드는 `ModbusDevicesEditor`의 modbus 필드를 재사용한다 |
| A-8 | 프론트 신규 섹션은 `agents.detail.devices.*` 계열 i18n 키를 사용하며, 하드코딩 문자열을 도입하지 않는다 |
| A-9 | `get_device_status`(디바이스 단위 상세 상태)는 선택 사항이며, F1 list_devices만으로 목록·기본 상태가 충족되면 도입하지 않는다 |

---

## 4. 요구사항 (Requirements — EARS)

EARS 형식. 요구사항 모듈은 **5개**(REQ-01~05). 구현 의존 관계: F1 백엔드 list_devices → F2 백엔드 update_device → F3 프론트(설정탭 숨김 + 장치탭 섹션). REQ-05는 cross-cutting(하위 호환·품질).

### REQ-MODBUS-009-01 — 백엔드 `list_devices` (디바이스 목록 조회)

- **Ubiquitous**: 시스템은 항상 modbus-client `Process()` 명령 경로로 현재 디바이스 목록을 조회할 수 있는 `list_devices` 명령을 제공해야 하며, 응답은 각 디바이스의 식별·연결·폴링 메타데이터(id, host, port, unit_id, transport, register_groups, share_session 등)를 포함해야 한다.
- **Event-driven**: WHEN `list_devices` 명령이 발행될 때 THEN 시스템은 `a.mu` RLock 하에서 현재 devices 스냅샷을 구성하여 반환해야 한다(폴링 goroutine과 경합 없이).
- **State-driven (0-device)**: IF 에이전트에 디바이스가 없으면 THEN 시스템은 빈 목록을 오류 없이 반환해야 한다.
- **State-driven (표면 정렬)**: IF list_devices 응답 형태가 정의되면 THEN 프론트가 이미 소비 중인 표면(modbus-gateway `list_devices`, AgentDetailPanel.tsx:3617/`ModbusDevicesSection`)과 정렬되어야 한다.
- **Unwanted**: 시스템은 `list_devices` 처리 중 공유 상태(devices, 트랜스포트, 통계 맵)를 변형하지 않아야 한다(read-only).

### REQ-MODBUS-009-02 — 백엔드 `update_device` (연결 유지 in-place 수정)

- **Ubiquitous**: 시스템은 항상 modbus-client `Process()` 명령 경로로 기존 디바이스의 설정(register_groups/unit_id/poll_interval/request_timeout/reconnect_interval)을 **연결을 유지한 채** in-place로 변경하는 `update_device` 명령을 제공해야 하며, `set_config`(set_config.go:33)의 기존 디바이스 재구성 패턴을 재사용해야 한다.
- **Event-driven**: WHEN 유효한 `update_device` 명령이 발행될 때 THEN 시스템은 대상 디바이스를 `a.mu` Lock 하에서 찾아(`findDeviceLocked`) 지정 필드를 변경하고, 필요한 경우 그룹 스케줄러/통계를 copy-on-write로 갱신하되 트랜스포트/연결은 재생성하지 않아야 한다.
- **State-driven (미존재 거부)**: IF 대상 디바이스 ID가 존재하지 않으면 THEN 시스템은 `ErrDeviceNotFound`로 원자적으로 거부하고 직전 상태로 계속 동작해야 한다(부분 적용 없음).
- **State-driven (init 전용 거부)**: IF `update_device`가 transport 전환·RTU 하드웨어 파라미터 등 init 전용 필드를 변경하려 하면 THEN 시스템은 `rejectInitOnlyFields`(set_config.go:204) 규칙에 준하여 거부해야 한다.
- **Unwanted**: 시스템은 `update_device`를 제거+재추가로 구현하지 않아야 하며(연결 재설정 회피), 변경 적용 중 공유 상태를 뮤텍스 보호 없이 수정하지 않아야 한다.

### REQ-MODBUS-009-03 — 프론트엔드 설정 탭: modbus-client `devices` 편집기 숨김

- **Ubiquitous**: 시스템은 항상 실행 중 에이전트의 설정 탭(`ConfigTab`, AgentDetailPanel.tsx:769)에서 modbus-client의 `devices` 필드(`modbus_devices` 에디터, agentSchemas.ts:106/FormField.tsx:337)를 렌더에서 제외해야 한다.
- **State-driven (타입 스코프)**: IF agentType이 modbus-client이면 THEN `devices` 필드를 숨기고, 그 외 agentType(예: modbus-gateway)의 설정 폼과 스키마 파이프라인에는 영향이 없어야 한다.
- **Unwanted**: 시스템은 설정 탭에서 modbus-client `devices`를 숨기더라도 다른 필드(transport/serial/케이던스 등)의 렌더나 저장 동작을 변형하지 않아야 한다.

### REQ-MODBUS-009-04 — 프론트엔드 장치 탭: modbus-client 전용 디바이스 관리 섹션

- **Ubiquitous**: 시스템은 항상 장치 탭(`DevicesTab`, AgentDetailPanel.tsx:3570)에서 agentType이 modbus-client일 때 전용 디바이스 관리 섹션을 렌더해야 하며, 이 섹션은 추가(add_device)·제거(remove_device)·수정(update_device)·목록(list_devices)을 제공해야 한다.
- **Event-driven (CRUD)**: WHEN 사용자가 섹션에서 디바이스 추가/제거/수정을 수행할 때 THEN 시스템은 `useExecAgent`(hooks/useAgent.ts:145) 경로로 각각 add_device/remove_device/update_device 명령을 발행하고, list_devices로 목록을 갱신해야 한다.
- **State-driven (편집 필드 재사용)**: IF 디바이스 추가/수정 폼이 렌더되면 THEN 시스템은 `ModbusDevicesEditor`(ModbusDevicesEditor.tsx)의 modbus 필드(id/host/port/unit_id/register_groups/per-device transport·serial/share_session)를 재사용해야 한다.
- **State-driven (참조 패턴)**: IF 섹션이 구현되면 THEN `ModbusDevicesSection`(modbus-gateway 전용, AgentDetailPanel.tsx:1255/3663)을 구조적 참조 패턴으로 삼아야 하며, 신규 전용 React 컴포넌트 도입은 허용된다.
- **Unwanted**: 시스템은 modbus-client 장치 탭 섹션에서 i18n 키(`agents.detail.devices.*`) 대신 하드코딩 문자열을 사용하지 않아야 하며, modbus-gateway·xsfm·NASA/LGAP/LG-ICP의 기존 장치 탭 분기를 변경하지 않아야 한다.

### REQ-MODBUS-009-05 — 하위 호환 · 품질 (cross-cutting)

- **Ubiquitous (하위 호환 HARD)**: 시스템은 항상 기존 `modbus-client` type id·동작·기존 명령(read/write/set_config/add_device/remove_device) 및 기존 폴링/트랜스포트/세션 공유 동작을 오늘과 동일하게 유지해야 한다.
- **Ubiquitous (명령 표면 일관성)**: 시스템은 항상 `list_devices`/`update_device`를 기존 `Process` 문자열 명령 스위치(agent.go:953-975)와 일관된 방식(1 동사 = 1 case + 핸들러)으로 노출해야 한다(신규 병렬 메커니즘 금지).
- **State-driven (품질 게이트)**: IF 신규 백엔드 코드가 추가되면 THEN `go test -race`가 클린이어야 하고 커버리지 목표(≥85%)를 만족해야 하며, 프론트 변경은 `tsc`(타입 체크)와 `vitest`(단위 테스트)를 통과해야 한다.
- **Unwanted**: 시스템은 신규 외부 modbus 라이브러리를 `go.mod`에 추가하지 않아야 하고, `ModbusTransport.SendAndReceive` 시그니처를 변경하지 않아야 하며, i18n 키 누락/불일치를 도입하지 않아야 한다.

---

## 5. 명세 (Specifications)

### 5.1 F1 — 백엔드 `list_devices`

- `Process` 스위치(agent.go:946-975)에 `case "list_devices"` + `processListDevices` 핸들러 추가(1 동사 = 1 case, 기존 스타일 준수).
- `a.mu` RLock 하에서 현재 devices를 순회하여 각 디바이스의 id/host/port/unit_id/transport/register_groups/share_session 등 메타데이터 배열을 JSON으로 반환. 부작용 없음.
- 응답 형태는 modbus-gateway `list_devices`(modbusserver/agent.go:325) 및 프론트 기존 소비 형태(AgentDetailPanel.tsx:3617에서 `data` 배열의 device_id/address/source 키 참조)와 정렬. modbus-client 고유 필드(register_groups 등)는 확장 필드로 추가.
- 신규 파일 배치 판단: `runtime_device.go`(런타임 디바이스 라이프사이클) 확장 또는 신규 `list_devices.go` — 구현 시 결정(plan.md reuse map 참조).

### 5.2 F2 — 백엔드 `update_device`

- `Process` 스위치에 `case "update_device"` + `processUpdateDevice` 핸들러 추가.
- 로직: params에서 대상 `device_id` + 변경 필드 파싱 → `a.mu` Lock → `findDeviceLocked`(runtime_device.go:216)로 대상 조회(미존재 시 `ErrDeviceNotFound` 거부) → `rejectInitOnlyFields`(set_config.go:204) 규칙으로 금지 필드 거부 → `processSetConfig`(set_config.go:33)의 register_groups/unit_id/poll_interval/request_timeout/reconnect_interval 재구성 로직 재사용(가능하면 공통 헬퍼로 추출) → copy-on-write clone 헬퍼(runtime_device.go:317~)로 통계/캐시 정합.
- 트랜스포트/연결은 재생성하지 않는다(연결 유지). 그룹 변경 시 스케줄러만 갱신(set_config.go:153 패턴).
- `set_config`와의 관계: `set_config`는 에이전트 기본 케이던스 + 디바이스 스코프 미세조정을 겸하는 반면, `update_device`는 디바이스 단위 명시적 수정 표면을 제공한다. 공통 재구성 로직은 헬퍼로 공유하여 중복을 피한다.

### 5.3 F3(a) — 프론트 설정 탭 숨김

- `ConfigTab`(AgentDetailPanel.tsx:769)의 필드 렌더 파이프라인에서 agentType === 'modbus-client'이고 필드가 `devices`(type `modbus_devices`)일 때 렌더 스킵.
- 구현 지점 후보: `ConfigTab`의 필드 필터, 또는 `FormField.tsx`(337)의 `modbus_devices` 렌더 게이팅, 또는 스키마 `visibleWhen` 계열 메커니즘 — 구현 시 최소 침습 지점 선택. 다른 agentType·다른 필드에 영향이 없어야 한다.
- 스키마(agentSchemas.ts:106) 자체는 유지(생성 시 초기 부트스트랩 판단은 구현 시). running-agent config 렌더에서만 제외.

### 5.4 F3(b) — 프론트 장치 탭 modbus-client 전용 섹션

- `DevicesTab`(AgentDetailPanel.tsx:3570)에 modbus-client 분기 추가: `if (agentType === 'modbus-client') return <ModbusClientDevicesSection agentId={agentId} />;`(modbus-gateway 3663 / xsfm 3669 분기와 동일 스타일). 기존 분기는 불변.
- 신규 컴포넌트 `ModbusClientDevicesSection`(신규 허용): `ModbusDevicesSection`(1255) 구조 참조. list_devices 실시간 목록 + 추가/제거/수정 UI.
  - 추가: `ModbusDevicesEditor` 필드로 신규 디바이스 폼 → `add_device` exec.
  - 제거: 목록 항목 삭제 → `remove_device` exec(useDeleteDevice 패턴, useDevice.ts:247 참조).
  - 수정: 항목 편집 폼(ModbusDevicesEditor 필드) → `update_device` exec.
  - 목록/갱신: `list_devices` exec(useExecAgent) → CRUD 후 refetch.
- i18n: `agents.detail.devices.*` 계열 키 확장(add/edit/remove/labels). 하드코딩 금지.

### 5.5 통계·동시성 정합

- `update_device`의 그룹 변경은 SPEC-008이 확립한 copy-on-write 패턴(runtime_device.go clone 헬퍼, agent.go:647 주석)을 준수하여 폴링 goroutine과의 경합을 유발하지 않아야 한다.
- `list_devices`는 RLock 스냅샷으로 충분(변형 없음).

---

## 6. 추적성 (Traceability)

| 요구사항 모듈 | 명세 절 | 수용 시나리오(acceptance.md) | 주요 파일(reuse map은 plan.md 참조) |
|---------------|---------|------------------------------|--------------------------------------|
| REQ-MODBUS-009-01 (F1 list_devices) | 5.1 | AC-01, AC-02 | agent.go(Process 스위치), runtime_device.go 또는 신규 list_devices.go, modbusserver/agent.go(참조) |
| REQ-MODBUS-009-02 (F2 update_device) | 5.2, 5.5 | AC-03, AC-04 | agent.go(Process 스위치), set_config.go(재구성 재사용), runtime_device.go(findDeviceLocked·clone) |
| REQ-MODBUS-009-03 (F3a 설정탭 숨김) | 5.3 | AC-05 | AgentDetailPanel.tsx(ConfigTab), FormField.tsx, agentSchemas.ts |
| REQ-MODBUS-009-04 (F3b 장치탭 섹션) | 5.4 | AC-06, AC-07 | AgentDetailPanel.tsx(DevicesTab), 신규 ModbusClientDevicesSection, ModbusDevicesEditor.tsx, hooks/useAgent.ts·useDevice.ts, ko.json(i18n) |
| REQ-MODBUS-009-05 (cross-cutting) | 5.1~5.5 | AC-08, AC-09 | agent.go, config.go, go.mod, AgentDetailPanel.tsx, ko.json |

---

## 7. 범위 밖 (Non-Goals)

- MODBUS 게이트웨이(`internal/agent/modbusserver/`, type id `modbus-gateway`) 동작 변경 — 참조 패턴만 차용.
- `get_device_status`(디바이스 단위 상세 상태 명령) — 선택 사항. F1 list_devices만으로 목록·기본 상태가 충족되면 도입하지 않는다(필요 시 향후 REQ로 optional 추가).
- 신규 외부 modbus 라이브러리 도입, `modbus-client` type id 변경, 신규 패키지 복제.
- `ModbusTransport.SendAndReceive` 시그니처 변경.
- 실제 하드웨어 RTU 타이밍 튜닝(잔여 위험으로 plan.md에 기록).
- 에이전트 생성 마법사(CreateAgentModal)의 디바이스 부트스트랩 흐름 전면 재설계(설정 탭 숨김 범위에 한정, 초기 부트스트랩 유지 여부는 구현 시 판단).
