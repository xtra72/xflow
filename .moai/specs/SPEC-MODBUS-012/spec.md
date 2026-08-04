---
id: SPEC-MODBUS-012
title: "MODBUS Gateway 대시보드 패널 스위트 (실제/가상 디바이스·공유/가상 레지스터 맵·버스/종합 통계)"
version: "0.1.0"
status: draft
created: 2026-08-04
updated: 2026-08-04
author: xtra
priority: P2
phase: "v0.1.0 target"
module: "web/src/pages/dashboard + internal/agent/modbusserver"
lifecycle: spec-anchored
tier: L
tags: "modbus, gateway, dashboard, panel, register-map, grid, bus-stats, backing-metrics, observability, i18n"
---

# SPEC-MODBUS-012: MODBUS Gateway 대시보드 패널 스위트

## HISTORY

| 버전  | 날짜       | 변경 내용                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
|-------|------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 0.1.0 | 2026-08-04 | 초안 작성 (Draft, Tier L 5-산출물). MODBUS Gateway(`modbus-gateway`, 패키지 `internal/agent/modbusserver`)를 위한 **대시보드 패널 스위트 6종** 추가. 확정 설계 반영: (1) 신규 SPEC Tier L 전체(6종 패널 + 레지스터 그리드 시각화 + 백엔드 메트릭 노출)를 마일스톤 분할로, (2) 실제 디바이스 메트릭 = SPEC-010 백킹에 요청·에러·레이턴시 카운터 추가 후 `get_device_status` 노출(SPEC-010 M7 optional 완성), (3) 레지스터 맵 active/degraded 상태 = 값 기반 프론트 판정(레지스터 스냅샷 백엔드 미변경). REQ-01~07, AC-01~22. |

---

## 1. 개요 (Overview)

xflow의 대시보드는 사용자가 패널을 추가하고 대상(디바이스/에이전트/플로우 노드)을 선택하면 실시간 정보를 표출하는 **컴포넌트-기반 위젯 시스템**이다. 패널 추가는 4단계 배선으로 이루어진다: (1) `PanelType` 유니온 등록(`web/src/stores/uiStore.ts:177-203`), (2) 기본 설정/크기 정의(`createDefaultPanel` `:319-446`, `panelDefaultSize` `:265-316`), (3) 추가 다이얼로그 옵션 등록(`web/src/pages/dashboard/AddPanelDialog.tsx:112-156`), (4) 타입→컴포넌트 스위치(`web/src/pages/dashboard/renderDashboardPanel.tsx:83-266`).

본 SPEC은 MODBUS Gateway 에이전트(type id `modbus-gateway`)를 대상으로 하는 **6종 대시보드 패널**을 추가한다. 사용자가 패널을 생성하고 modbus-gateway 에이전트를 선택하면 해당 에이전트의 실시간 정보를 표출한다.

1. **실제 연결 디바이스 목록** — 백킹(upstream) 실제 디바이스의 연결 상태 + 메트릭(slave id, latency, err rate)
2. **가상 디바이스 목록** — `list_devices` 기반 U01~ 목록(unit_id, name, 영역 배지, active/stale)
3. **공유 레지스터 맵** — unit 0 공유 컨테이너 레지스터 맵 그리드(4영역, POINTS/ACTIVE/DEGRADED 집계 + 셀 색상)
4. **가상 디바이스 레지스터 맵** — 선택 가상 디바이스의 레지스터 맵 그리드
5. **버스 통계** — reads·writes/min, latency, 활성 커넥션 미니 차트
6. **종합 통계** — poll cycle/throughput/clients/errors/uptime 요약 바

참조 목업은 다크 테마 대시보드 이미지(상단 종합 통계 바 + 좌측 실제 디바이스 목록 + 중앙 레지스터 맵 그리드(Coils/Discrete Inputs/Input/Holding, POINTS/ACTIVE/DEGRADED + 색상 셀) + 우측 가상 디바이스 목록(U01~ active/stale) + 하단 버스 통계 미니 차트)이다.

본 SPEC은 **프론트엔드 중심 + 소규모 백엔드 확장**이다. 백엔드 변경은 SPEC-010 백킹(upstream 실제 디바이스)의 관측 메트릭 노출로 **한정**한다(요청/에러/레이턴시 카운터 추가 + `get_device_status` 노출). 레지스터 맵 스냅샷 백엔드는 미변경이며, active/degraded 상태는 프론트에서 값 기반 규칙으로 판정한다. 기존 대시보드 패널·게이트웨이 동작·type id는 반드시 보존한다(하위 호환 HARD, hybrid DDD/TDD).

> **주의(패키지·타입 혼동 금지)**: 대상은 서버 `internal/agent/modbusserver`(type id `modbus-gateway`)이다. 클라이언트 `internal/agent/modbus`(type id `modbus-client`)는 본 SPEC의 패널 대상이 아니다(§8 Non-Goals). type id `modbus-gateway`/`modbus-client`는 불변이다.

---

## 2. 환경 (Environment)

- **프론트엔드**: TypeScript 5.9, React 19, Zustand(`uiStore`), TanStack Query(`useExecAgent`/`useAgents`), Vitest. 기존 대시보드 차트/그리드 프리미티브 재사용(신규 npm 의존성 금지).
- **백엔드**: Go 1.23, 패키지 `internal/agent/modbusserver`. exec 명령은 `agent.go` `Process` 스위치(`:309-354`)로 처리. 백킹 런타임은 `backed_store.go`/`poller.go`/`backing_lifecycle.go`.
- **에이전트 바인딩**: 패널은 `config.agentId`(선택된 modbus-gateway 에이전트) + (레지스터 맵/가상 맵은) `config.unitId` 를 저장한다. 데이터는 `execAgent(agentId, {command, params})` 로 취득한다(`web/src/services/api/agentService.ts:117-122`).
- **폴링**: 패널은 `useUIStore(s => s.dashboardRefreshInterval)` 주기로 exec 폴링한다(기존 패널 패턴, `AgentPanel.tsx:45`).
- **i18n**: `web/src/lib/i18n/{ko,en}.json`. `dashboard.panelTypes.*`, `dashboard.addPanel.descriptions.*` 키 확장 + `dashboard.modbus.*` 신규 네임스페이스.

---

## 3. 가정 (Assumptions)

- 대상 에이전트는 `type === 'modbus-gateway'` 인 실행 중 에이전트이다. 정지/오프라인 에이전트는 exec 실패 시 빈 상태/에러 배지로 graceful degrade 한다(ClientsTab 패턴, `AgentDetailPanel.tsx:3958-3963`).
- 가상 디바이스 목록·레지스터 맵·상태는 기존 exec 명령(`list_devices`/`get_device_status`/`get_map`/`get_*`)으로 취득 가능하다(스냅샷 백엔드 미변경).
- 실제(upstream) 디바이스 메트릭(요청 수·에러 수·레이턴시)은 현재 미노출이며(SPEC-010 M7 optional 스킵, `SPEC-MODBUS-010/plan.md:60`), 본 SPEC이 백엔드 카운터를 추가하여 `get_device_status` 로 노출한다.
- 레지스터 셀의 active/degraded 는 **레지스터 값**으로 프론트에서 판정한다(값 ≠ 0 → active, 정의된 맵 범위 밖/조회 에러 → degraded). 임계·규칙은 프론트 상수로 관리한다.
- 버스 통계의 시계열(reads/writes per min)은 `get_status` 에 히스토리가 없으므로(현재 응답 `:1163-1176`) 프론트에서 폴링 델타로 누적한다.
- 원격 대시보드 타깃(SPEC-REMOTE-001)은 exec 프록시가 없으므로 본 패널들은 원격에서 안내 문구로 graceful degrade 한다(회귀 0).

---

## 4. 요구사항 (Requirements, EARS)

### REQ-01 — 패널 프레임워크 통합 (Ubiquitous)

시스템은 **항상** 6종 신규 `PanelType`(`modbus-real-devices`, `modbus-virtual-devices`, `modbus-shared-registers`, `modbus-device-registers`, `modbus-bus-stats`, `modbus-summary-stats`)을 대시보드 패널 프레임워크에 등록해야 한다: `uiStore` 유니온·기본설정·기본크기, `AddPanelDialog` 옵션(data 카테고리), `renderDashboardPanel` 스위치, i18n(ko/en) 라벨/설명.

- REQ-01-01: **WHEN** 사용자가 패널 추가 다이얼로그에서 6종 중 하나를 선택 **THEN** 시스템은 에이전트 선택 스텝(modbus-gateway 필터)을 제시하고, 완료 시 `config.agentId`(+ 가상 맵은 `unitId`)를 저장한 패널을 활성 대시보드에 추가해야 한다.
- REQ-01-02: 시스템은 각 패널 타입에 목업에 부합하는 기본 그리드 크기를 부여해야 한다(레지스터 맵 그리드는 넓게, 통계 바/미니차트는 낮게).
- REQ-01-03: 시스템은 `config.agentId` 미설정 시 "에이전트 미설정" 안내를, 대상 에이전트 부재/정지 시 빈 상태를 표시해야 한다(`SingleDevicePanel.tsx:46-53` 패턴).

### REQ-02 — 실제 연결 디바이스 목록 패널 (State-Driven)

**IF** 대상 게이트웨이에 백킹(upstream) 디바이스가 존재 **THEN** 시스템은 각 실제 디바이스의 연결 상태(online/stale/error)와 메트릭(slave id=upstream unit id, latency, err rate, 모드 direct/indirect)을 목록으로 표출해야 한다. 본 REQ은 REQ-06의 백엔드 메트릭 노출에 의존한다.

- REQ-02-01: 시스템은 `list_devices` 로 디바이스를 열거하고 `backed=true` 인 항목만 실제 디바이스로 표시해야 한다.
- REQ-02-02: 시스템은 각 실제 디바이스에 대해 `get_device_status` 의 `backing` 서브객체(mode, connected, request_count, error_count, avg_latency_ms, last_ok)를 표시해야 한다.
- REQ-02-03: 시스템은 `connected=false` 또는 stale(indirect, `last_ok` 초과) 디바이스를 시각적으로 degraded 로 구분해야 한다.

### REQ-03 — 가상 디바이스 목록 패널 (Event-Driven)

**WHEN** 패널이 폴링 주기에 도달 **THEN** 시스템은 `list_devices` 응답으로 가상 디바이스(U01~) 목록을 unit_id, name, 영역 배지(CO/DI/IR/HR), active/stale 상태와 함께 표출해야 한다.

- REQ-03-01: 시스템은 각 디바이스의 `register_counts`(coils/discrete_inputs/holding_registers/input_registers)를 영역 배지로 표시해야 한다.
- REQ-03-02: 시스템은 `stats`(read_count/write_count/error_count) 델타로 active(최근 접근) / stale(무접근) 상태를 판정·표시해야 한다.

### REQ-04 — 공유 레지스터 맵 패널 (State-Driven)

**IF** 대상 게이트웨이에 unit 0 공유 컨테이너가 구성 **THEN** 시스템은 공유 컨테이너 레지스터 맵을 4영역(Coils/Discrete Inputs/Input Registers/Holding Registers) 그리드로 표출하고, 각 영역의 POINTS(정의 개수)/ACTIVE/DEGRADED 집계와 셀 색상을 표시해야 한다.

- REQ-04-01: 시스템은 `get_map`(`params.unit_id=0`)의 `register_map` 스냅샷으로 그리드를 렌더해야 한다(공유 컨테이너 미구성 시 빈/안내 상태).
- REQ-04-02: 시스템은 각 셀의 active/degraded 를 **값 기반 규칙**으로 프론트 판정해야 한다(값 ≠ 0 → active, 정의된 맵 범위 밖/조회 에러 → degraded).
- REQ-04-03: 시스템은 영역별 POINTS/ACTIVE/DEGRADED 집계를 헤더에 표시해야 한다(목업 부합).

### REQ-05 — 가상 디바이스 레지스터 맵 패널 (State-Driven)

**IF** `config.unitId` 로 지정된 가상 디바이스가 존재 **THEN** 시스템은 해당 디바이스의 레지스터 맵을 REQ-04와 동일한 4영역 그리드(POINTS/ACTIVE/DEGRADED + 셀 색상)로 표출해야 한다.

- REQ-05-01: 시스템은 `get_device_status`(`params.unit_id=N`)의 `register_map` 스냅샷으로 그리드를 렌더해야 한다.
- REQ-05-02: 시스템은 REQ-04-02와 동일한 값 기반 판정 규칙을 재사용해야 한다(공유 로직).

### REQ-06 — 버스/종합 통계 패널 + 백엔드 실제 디바이스 메트릭 노출 (Event-Driven + Ubiquitous)

**WHEN** 패널이 폴링 주기에 도달 **THEN** 시스템은 종합 통계(active_connections, clients, uptime, 총 디바이스, 총 reads/writes/errors)와 버스 통계(reads·writes per min, avg latency, 활성 커넥션 추이 미니차트)를 표출해야 한다. 아울러 시스템은 **항상** SPEC-010 백킹의 upstream 요청 수·에러 수·레이턴시 카운터를 백엔드에 추가하고 `get_device_status`로 노출해야 한다.

- REQ-06-01: 백엔드 — 시스템은 `backedStore`에 upstream 요청 수(`request_count`)·에러 수(`error_count`)·누적 레이턴시(→ `avg_latency_ms`)·마지막 성공 시각(`last_ok`) 카운터를 원자적으로 기록해야 한다(폴러·서빙 경합 안전).
- REQ-06-02: 백엔드 — 시스템은 `get_device_status` 응답에 `backing` 서브객체(mode, connected, request_count, error_count, avg_latency_ms, last_ok)를 추가하고, 비백킹 디바이스는 `backing:null`을 반환해야 한다. `list_devices` 항목에 `backed`(bool)·`mode`를 추가해야 한다.
- REQ-06-03: 프론트 — 시스템은 종합 통계 바를 `get_status` + `list_devices`(디바이스/합계) + `list_clients`(clients 수)로 구성해야 한다.
- REQ-06-04: 프론트 — 시스템은 버스 통계 시계열을 `list_devices` stats 델타로 프론트 누적하여 미니차트로 표시해야 한다(기존 차트 프리미티브 재사용).

### REQ-07 — 하위 호환·품질 (Unwanted + Ubiquitous, cross-cutting)

시스템은 기존 대시보드 패널·게이트웨이 동작·type id를 변경**하지 않아야 한다**. 시스템은 신규 라이브러리(npm modbus/차트, go.mod modbus 모듈)를 도입**하지 않아야 한다**.

- REQ-07-01: 시스템은 프론트 tsc/vitest 클린, 백엔드 `go test -race` 클린을 유지해야 한다.
- REQ-07-02: 시스템은 i18n ko/en 키 정합을 유지해야 한다(신규 키 양 로케일 동시 추가).
- REQ-07-03: 시스템은 레지스터 값 쓰기를 제공**하지 않아야 한다**(대시보드는 관측 전용).
- REQ-07-04: 백엔드 카운터 추가는 백킹 관측 메트릭으로 **한정**하며, 레지스터 스냅샷 경로(`get_map`/`get_*`)를 변경**하지 않아야 한다**.

---

## 5. 명세 (Specifications) — 패널별 데이터 소스 매핑

### 5.1 패널 → exec 명령 → 응답 형상 표

| # | 패널 (PanelType)                    | exec 명령 (agent.go)                                   | 응답 형상 (근거)                                                                                                          | 백엔드 변경        |
|---|-------------------------------------|-------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------|--------------------|
| 1 | `modbus-real-devices`               | `list_devices`(`:342`) + `get_device_status`(`:350`)  | `list_devices`: `{devices:[{unit_id,name,register_counts,status,stats{read/write/error_count}, backed*, mode*}], device_count}` (`:1184-1206`) · `get_device_status.backing*`: `{mode,connected,request_count,error_count,avg_latency_ms,last_ok}` | YES (REQ-06-01/02) |
| 2 | `modbus-virtual-devices`            | `list_devices`(`:342`)                                | 위 `list_devices` (backed 무관 전체)                                                                                       | none               |
| 3 | `modbus-shared-registers`           | `get_map`(`:334`, `params.unit_id=0`)                 | `{register_map:{coils,discrete_inputs,holding_registers,input_registers}, type_overlay?}` (`:1098-1111`, snapshot `register_map.go:440`) | none               |
| 4 | `modbus-device-registers`           | `get_device_status`(`:350`, `params.unit_id=N`)       | `{unit_id,name,register_counts,register_map{...},stats{...,last_access}}` (`:1394-1406`)                                   | none               |
| 5 | `modbus-bus-stats`                  | `get_status`(`:338`) + `list_devices` 델타             | `get_status`: `{listen_address,listen_port,unit_id,active_connections,max_connections,uptime_seconds}` (`:1163-1176`)      | none (프론트 누적) |
| 6 | `modbus-summary-stats`              | `get_status`(`:338`) + `list_devices` + `list_clients`(`:344`) | `list_clients`: `{clients:[{remote_addr,connected_at,unit_ids[],request_count,last_seen}]}` (`:1212-1230`)           | none               |

`*` = 본 SPEC 백엔드 확장으로 추가되는 필드.

### 5.2 값 기반 셀 상태 판정 규칙 (REQ-04-02/05-02, 공유 로직)

- 입력: 영역별 스냅샷 맵(`coils`/`discrete_inputs`: `addr→bool`, `holding_registers`/`input_registers`: `addr→uint16`) + `register_counts`(정의 개수).
- 판정:
  - **active**: bool 영역은 `true`, 숫자 영역은 값 `!= 0`.
  - **inactive**: 정의된 주소이나 값이 0/false.
  - **degraded**: 정의된 맵 범위 밖 접근 또는 조회 에러(스냅샷에 주소 부재이나 count>0 로 기대되는 경우, 또는 exec 실패).
- 집계: 영역별 POINTS = `register_counts[area]`, ACTIVE = active 셀 수, DEGRADED = degraded 셀 수.
- 규칙은 프론트 상수/유틸(`registerCellState.ts`)로 관리하며 백엔드 미변경(locked decision #3).

### 5.3 에이전트 선택 스텝

`AddPanelDialog` 에 `needsAgent`(신규) 옵션 플래그를 도입한다. `FacilityStep`(`:818-986`, `a.type === 'xsfm'` 필터)을 미러링하여 `a.type === 'modbus-gateway'` 필터의 에이전트 셀렉트 스텝을 추가한다. 가상 디바이스 레지스터 맵(`modbus-device-registers`)은 에이전트 선택 후 `list_devices` 로 unit 셀렉트 2차 스텝을 제시하고 `config.unitId` 를 저장한다.

---

## 6. 추적성 (Traceability)

| REQ    | 명세 절            | AC                     | 패널/대상                             |
|--------|--------------------|------------------------|---------------------------------------|
| REQ-01 | §5.1, §5.3         | AC-01, AC-02, AC-03    | 프레임워크(6종 등록)                  |
| REQ-02 | §5.1(#1)           | AC-04, AC-05, AC-06    | 실제 디바이스 목록                    |
| REQ-03 | §5.1(#2)           | AC-07, AC-08           | 가상 디바이스 목록                    |
| REQ-04 | §5.1(#3), §5.2     | AC-09, AC-10, AC-11    | 공유 레지스터 맵                      |
| REQ-05 | §5.1(#4), §5.2     | AC-12, AC-13           | 가상 디바이스 레지스터 맵             |
| REQ-06 | §5.1(#5,#6)        | AC-14~AC-18            | 버스/종합 통계 + 백엔드 메트릭        |
| REQ-07 | §1, §5.2           | AC-19~AC-22            | 하위 호환·품질(cross-cutting)         |

---

## 7. 마일스톤 (우선순위 기반, 시간 예측 없음)

plan.md 참조. 요약: M1 프레임워크 → M2 목록 패널 2종 → M3 백엔드 백킹 메트릭 → M4 레지스터 맵 그리드 2종 → M5 버스/종합 통계 → M6 i18n·회귀·품질.

---

## 8. 비목표 (Non-Goals)

- 레지스터 값 **쓰기**(대시보드는 관측 전용, REQ-07-03).
- 실 하드웨어 타이밍/CRC 프레임 오류 계측(현재 백엔드 카운터 부재; 백엔드 변경은 백킹 메트릭으로 한정 — 버스 "errors"는 error_count 델타 집계로 대체).
- 대시보드 프레임워크 자체 개편(그리드 엔진/스냅샷 모델 변경 없음).
- `modbus-client` 대시보드 패널(본 SPEC은 gateway 전용).
- 레지스터 맵 스냅샷 백엔드 변경(`get_map`/`get_*` 경로 불변).
