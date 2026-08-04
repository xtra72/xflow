---
id: SPEC-MODBUS-011
title: "MODBUS Client·Gateway 장치 일괄 등록 (텍스트/CSV 붙여넣기)"
version: "0.1.0"
status: draft
created: 2026-08-04
updated: 2026-08-04
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "web/src/pages/agents, web/src/hooks"
lifecycle: spec-anchored
tags: "modbus, client, gateway, device-management, bulk-register, csv, paste, ui, i18n, best-effort"
tier: M
---

# SPEC-MODBUS-011: MODBUS Client·Gateway 장치 일괄 등록 (텍스트/CSV 붙여넣기)

## HISTORY

| 버전  | 날짜       | 변경 내용                                                                 |
| ----- | ---------- | ------------------------------------------------------------------------- |
| 0.1.0 | 2026-08-04 | 최초 작성. 두 장치탭(client·gateway) 텍스트/CSV 붙여넣기 일괄 등록 명세.   |

---

## 배경 (Environment)

modbus 장치탭에서 디바이스를 추가하려면 현재는 편집 다이얼로그로 **한 번에 하나씩** 등록해야 한다.
대상 탭은 두 곳이다:

- **modbus-client 장치탭**: `ModbusClientDevicesSection`
  (`web/src/pages/agents/AgentDetailPanel.tsx:1538`). 단일 추가는 런타임 exec
  `add_device`(`AgentDetailPanel.tsx:1583`, params=`toEmitDevice(row, agentTransport)`)로
  수행한다. 목록은 `list_devices`(F1) 응답의 `res.data` 배열이다(`AgentDetailPanel.tsx:1560`).
- **modbus-gateway 장치탭**: `ModbusDevicesSection`
  (`web/src/pages/agents/AgentDetailPanel.tsx:1272`, `DevicesTab` 3663 분기). 백엔드
  `add_device`(`internal/agent/modbusserver/agent.go:1232` `processAddDevice`)를 지원한다.

이미 xsfm 장치탭은 동일한 요구를 **텍스트/CSV 붙여넣기 표**로 해결하고 있다:
공용 UI 컴포넌트 `BulkRegisterPanel`(`web/src/pages/agents/BulkRegisterPanel.tsx`)과
행 파서 `parseDelimitedRows`·best-effort 등록 훅 `useBulkAddDevices`
(`web/src/hooks/useStation.ts:486`), 실패 타입 `BulkFailure`(`useStation.ts:63`)가 그것이다.
본 SPEC은 이 검증된 패턴을 두 modbus 장치탭에 이식한다.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 | 틀렸을 때 영향 |
| - | ---- | ------ | ---- | -------------- |
| A1 | 백엔드 `add_device` 명령을 행별로 반복 호출하면 일괄 등록을 신규 백엔드 명령 없이 구현할 수 있다 | 높음 | client `runtime_device.go:35` / gateway `agent.go:1232` 모두 `add_device` 존재. xsfm `useStation.ts:486`가 동일 패턴 검증 | 백엔드 명령 신설 필요 → 범위 확대 |
| A2 | client 디바이스는 `EmittedDevice`(`ModbusDevicesEditor.tsx:129`) 형상, gateway 디바이스는 `{unit_id, name?, register_map}`(`ModbusServerDevicesEditor.tsx:149`) 형상으로 방출하면 백엔드가 수용한다 | 높음 | 단일 add 흐름이 동일 형상 사용 | 파서 방출 형상 재설계 |
| A3 | per-device 오버라이드(transport/serial_port/share_session)와 backing/shared 세그먼트는 초기 붙여넣기 범위에서 제외해도 실용적이다(상속·순수 slave 기본값) | 중간 | 상속 시 미방출 → 하위 호환(`toEmitDevice` 조건부 방출, `ModbusDevicesEditor.tsx:342`) | 고급 필드도 붙여넣기 컬럼에 편입 필요 |
| A4 | 부분 성공(일부 행 실패·나머지 등록)이 원자적 전량 롤백보다 사용자에게 유용하다 | 높음 | xsfm best-effort 선례(`useStation.ts:452`) | 트랜잭션 경계 요구 시 재설계 |

## 요구사항 (Requirements, EARS)

### REQ-MODBUS-011-01 — Client 붙여넣기 파서 (Event-Driven)

**WHEN** 사용자가 modbus-client 일괄 등록 텍스트를 제출하면 **THEN** 시스템은 각 줄을
client 디바이스 행 포맷(§ 붙여넣기 포맷 · Client)으로 파싱하고 검증하여, 유효 행은
`EmittedDevice`로 변환하고 무효 행은 원본 줄 번호와 함께 `BulkFailure`로 수집해야 한다.

- 시스템은 `host`(트랜스포트 상속이 tcp일 때 필수)·`unit_id`(1-247 필수 정수) 누락/범위 초과 행을
  백엔드 미호출로 실패 집계해야 한다.
- 시스템은 `register_groups` 서브 필드(`fc:start:qty[:data_type[:poll]]`)를 파싱해 유효하지 않은
  `fc`·`start`·`qty`·`data_type`을 행 실패로 집계해야 한다.

### REQ-MODBUS-011-02 — Gateway 붙여넣기 파서 (Event-Driven)

**WHEN** 사용자가 modbus-gateway 일괄 등록 텍스트를 제출하면 **THEN** 시스템은 각 줄을
gateway 디바이스 행 포맷(§ 붙여넣기 포맷 · Gateway)으로 파싱하고 검증하여, 유효 행은
`{unit_id, name?, register_map}` 파라미터로 변환하고 무효 행은 `BulkFailure`로 수집해야 한다.

- 시스템은 `unit_id`(1-247 필수)와 **최소 1개 세그먼트**(`register_map` 백엔드 필수,
  `agent.go:1245`) 누락 행을 백엔드 미호출로 실패 집계해야 한다.
- 시스템은 세그먼트 서브 필드(`area:start:count[:data_type]`)의 무효 `area`·`start`·`count`를
  행 실패로 집계해야 한다.
- **가능하면(Where)** backing·shared 세그먼트는 초기 범위에서 제외한다(순수 slave local 세그먼트만).

### REQ-MODBUS-011-03 — 일괄 등록 실행 (State-Driven, best-effort)

**IF** 파서가 1개 이상의 유효 행을 산출하면 **THEN** 시스템은 각 유효 행마다 `add_device` exec를
순차 호출하고, 행별 성공/실패를 수집하여 `BulkResult`(`total`/`ok`/`failed`) 요약을 사용자에게
표시해야 한다.

- 시스템은 개별 행 실패(중복 ID·백엔드 검증 실패 등)에도 **중단하지 않고** 나머지 행을 계속
  등록해야 한다(부분 성공 허용).
- 시스템은 모든 행 처리 후 **한 번만** 관련 목록 쿼리를 갱신(client: `list_devices` refetch,
  gateway: 목록 무효화)해야 한다.
- 시스템은 전량 성공 시 성공 요약을, 부분 실패 시 `{ok}개 등록, {failed}개 실패`와 실패 행 사유를
  표시해야 한다.

### REQ-MODBUS-011-04 — UI 통합 (Event-Driven)

**WHEN** 사용자가 두 장치탭(`ModbusClientDevicesSection`·`ModbusDevicesSection`) 중 하나에서
일괄 등록 진입점(토글 버튼)을 누르면 **THEN** 시스템은 `BulkRegisterPanel`을 표시하고, 붙여넣기 →
제출 → 실패 인라인 표시 흐름을 제공해야 한다.

- 시스템은 기존 단일 add/update/remove 흐름을 **변경 없이** 유지해야 한다.
- 시스템은 모든 문자열을 `agents.detail.devices.*` i18n 키로 표기해야 한다(하드코딩 금지, ko/en 정합).

### REQ-MODBUS-011-05 — 품질·하위 호환 (Ubiquitous, cross-cutting)

시스템은 **항상** 다음을 만족해야 한다:

- 백엔드 Go 코드 **미변경**(기존 `add_device` 재사용만). 에이전트 type id
  `modbus-client`/`modbus-gateway` 불변.
- 신규 npm 의존성 **금지**. 코드 주석 한국어.
- 프론트엔드 `tsc`(타입) 및 `vitest`(단위 테스트) 클린. 신규 파서는 순수 함수로 단위 테스트 대상.
- **금지(Unwanted)**: 시스템은 일괄 등록을 위한 신규 백엔드 bulk 명령을 도입하지 **않아야 한다**.
  시스템은 원자적 전량 롤백을 수행하지 **않아야 한다**(부분 성공 유지).

## 명세 (Specifications)

### 붙여넣기 포맷 · Client (`parseModbusClientBulk`)

한 줄 = 디바이스 하나. 셀 구분자는 **콤마 또는 탭**(줄마다 자동 감지, 탭 우선 —
`parseDelimitedRows` 규칙), 각 셀 trim, 빈 줄 무시. 컬럼(위치 고정, 후행 컬럼 선택):

| # | 컬럼 | 필수 | 설명 |
| - | ---- | ---- | ---- |
| 1 | `host` | TCP 필수 / RTU 무시 | 대상 호스트. 트랜스포트 상속이 tcp면 필수. rtu 상속이면 공란 허용. |
| 2 | `port` | 선택 | TCP 포트. 공란이면 502(기본). |
| 3 | `unit_id` | 필수 | 1-247 정수. 공란·비정수·범위 밖이면 행 실패. |
| 4 | `id` | 선택(후행) | 디바이스 ID. 공란이면 방출에서 생략(백엔드 자동 부여). |
| 5 | `groups` | 선택(후행) | register_groups. 그룹을 `;`로 구분, 각 그룹 = `fc:start:qty[:data_type[:poll]]`. 공란이면 빈 배열로 생성(이후 개별 편집). |

그룹 서브 토큰 `fc:start:qty[:data_type[:poll]]`:
- `fc` ∈ 1|2|3|4 (coils/discrete_inputs/holding_registers/input_registers). 그 외 → 행 실패.
- `start` 정수 ≥0, `qty` 정수 ≥1. 위반 → 행 실패.
- `data_type` 선택: `MODBUS_DATA_TYPE_OPTIONS` 중 하나. 공란이면 uint16(기본).
- `poll` 선택: Go duration 자유 텍스트(검증 안 함).

방출 형상: `EmittedDevice = { id?, host?, port?, unit_id, register_groups: EmittedGroup[] }`
(`ModbusDevicesEditor.tsx:129`). per-device transport/serial_port/share_session 오버라이드는
붙여넣기 범위 밖(상속 → 미방출).

예시:
```
host,port,unit_id,id,groups
192.168.0.10,502,1,,3:0:10:uint16;4:100:4
192.168.0.11,,2
```
- 1행: 헤더(첫 셀 `host`가 비정수 → 헤더로 스킵).
- 2행: host=192.168.0.10, port=502, unit_id=1, id 생략, 그룹 2개(holding 0..10, input 100..4).
- 3행: host=192.168.0.11, port 공란(→502), unit_id=2, 그룹 없음(빈 배열).

### 붙여넣기 포맷 · Gateway (`parseModbusGatewayBulk`)

한 줄 = 디바이스 하나. 구분자·빈 줄 규칙 동일. 컬럼:

| # | 컬럼 | 필수 | 설명 |
| - | ---- | ---- | ---- |
| 1 | `unit_id` | 필수 | 1-247 정수. 공란·비정수·범위 밖이면 행 실패. |
| 2 | `name` | 선택 | 디바이스 이름. 공란이면 방출에서 생략(백엔드가 `device-{unit_id}` 자동, `agent.go:1260`). |
| 3 | `segments` | 필수 | register_map. 세그먼트를 `;`로 구분, 각 세그먼트 = `area:start:count[:data_type]`. **최소 1개 필수**(백엔드 register_map 필수). 공란이면 행 실패. |

세그먼트 서브 토큰 `area:start:count[:data_type]`:
- `area` ∈ 1|2|3|4 또는 `coils|discrete_inputs|holding_registers|input_registers`. 그 외 → 행 실패.
- `start` 정수 ≥0, `count` 정수 ≥1. 위반 → 행 실패.
- `data_type` 선택: local 세그먼트, 공란이면 uint16.
- backing·shared 세그먼트(`shared_address`)는 초기 범위 제외(local 세그먼트만 방출).

방출 형상: `{ unit_id, name?, register_map: { <area>: [ {address, count, data_type} ] } }`
(백엔드 `processAddDevice` params, `agent.go:1232`; 에디터 형상 `ModbusServerDevicesEditor.tsx:149`).

예시:
```
unit_id,name,segments
1,Meter-A,holding_registers:0:10:uint16;input_registers:100:4
2,,coils:0:8
```
- 2행: unit_id=1, name=Meter-A, register_map={holding_registers:[{0,10,uint16}], input_registers:[{100,4,uint16}]}.
- 3행: unit_id=2, name 생략, register_map={coils:[{0,8,uint16}]}.

### 실행 및 결과 요약

- 실행 훅(신규): client `useModbusClientBulkAdd(agentId)`, gateway `useModbusGatewayBulkAdd(agentId)`
  — xsfm `useBulkAddDevices`(`useStation.ts:486`) best-effort 구조를 재사용. 각 유효 행마다
  `agentService.execAgent(agentId, { command: 'add_device', params })` 순차 호출.
- 결과: `BulkResult { total, ok, failed: BulkFailure[] }`. 실패 사유는 백엔드 오류 메시지 원문 또는
  파서 sentinel(i18n 매핑). 전량 성공 → 성공 토스트, 부분 실패 → `{ok}/{failed}` 토스트 + 패널 인라인 실패 목록.
- 갱신: client는 `fetchDevices()`(list_devices) 재호출, gateway는 관련 목록 무효화. 모두 처리 후 1회만.

## Traceability

| 요구사항 | 명세 절 | 수용 기준 |
| -------- | ------- | --------- |
| REQ-MODBUS-011-01 | 붙여넣기 포맷 · Client | AC-01, AC-02 |
| REQ-MODBUS-011-02 | 붙여넣기 포맷 · Gateway | AC-03, AC-04 |
| REQ-MODBUS-011-03 | 실행 및 결과 요약 | AC-05, AC-06, AC-07 |
| REQ-MODBUS-011-04 | UI 통합 | AC-08, AC-09 |
| REQ-MODBUS-011-05 | 품질·하위 호환 | AC-10, AC-11, AC-12 |

상세 재사용 지도(file:line), 마일스톤은 `plan.md`. Given/When/Then 수용 기준은 `acceptance.md`.
