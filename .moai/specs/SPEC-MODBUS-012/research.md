---
id: SPEC-MODBUS-012
title: "MODBUS Gateway 대시보드 패널 스위트 — 조사 노트"
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
tags: "modbus, gateway, dashboard, research, codebase-survey, exec-commands, reuse"
---

# SPEC-MODBUS-012 — 조사 노트 (research.md)

본 SPEC 작성 전 실제 코드 형상을 READ하여 확인한 근거이다. 모든 항목은 file:line로 검증되었다.

## 1. 대시보드 패널 프레임워크 (4단계 배선)

- **PanelType 유니온**: `web/src/stores/uiStore.ts:177-203`. 문자열 리터럴 유니온. 각 SPEC이 자기 타입을 주석으로 표기(예: `trigger-config` `:200-201`). → 6종 추가.
- **기본 크기**: `uiStore.ts:265-316` `panelDefaultSize(type)` switch, `default {w:5,h:4}`(`:313-315`). 차트 계열 크기는 SPEC-CHART-001 참조.
- **기본 config**: `uiStore.ts:319-446` `createDefaultPanel(type)`. agent 바인딩 패널은 `config:{agentId, ...}` 형태(`facility-*` `:408-434`가 선례). `addPanelWithConfig(type, config, title)`(`:872-895`)로 config+타이틀 주입.
- **다이얼로그 옵션**: `AddPanelDialog.tsx:112-156` `PANEL_OPTIONS_BY_CATEGORY`. 카테고리 `data|chart|content|control`(`:99-101`). 옵션 필드 `{type, icon, labelKey, descriptionKey, needsDevice?/needsFacility?/needsTriggerNode?/...}`(`:76-96`). → data 카테고리에 6종 + `needsAgent` 신규 플래그.
- **스텝 분기**: `AddPanelDialog.tsx:217-251` `handleSelect`. `needsFacility→setStep('facility')` 등 선례. → `needsAgent→setStep('agent')`.
- **에이전트 필터 선례**: `FacilityStep`(`:818-986`)이 `useAgents()`(`:833`) → `a.type === 'xsfm'`(`:835`) 필터. → `modbus-gateway` 미러.
- **타입→컴포넌트 스위치**: `renderDashboardPanel.tsx:83-266`. agent-바인딩 패널은 `{panelId, title, config, onConfigChange, onTitleChange}` props 전달(`facility-*` `:152-195`). → 6 case + import.

## 2. 에이전트 exec 데이터 소스 (agent.go Process switch :309-354)

각 명령의 응답 형상을 READ하여 확인:

| 명령 | 함수:라인 | 응답 형상 |
|---|---|---|
| `list_devices` | `processListDevices:1184-1206` | `{devices:[{unit_id, name, register_counts{coils,discrete_inputs,holding_registers,input_registers}, status:"active", stats{read_count,write_count,error_count}}], device_count}` |
| `list_clients` | `processListClients:1212-1229` | `{clients:[{remote_addr, connected_at(RFC3339), unit_ids:[]int, request_count, last_seen(RFC3339)}]}` — RTU는 `[]` |
| `get_status` | `processGetStatus:1163-1176` | `{listen_address, listen_port, unit_id, active_connections, max_connections, uptime_seconds}` — **throughput/CRC/시계열 없음** |
| `get_device_status` | `processGetDeviceStatus:1377-1407` | `{unit_id, name, register_counts, register_map(snapshot), stats{read_count,write_count,error_count,last_access}}` |
| `get_map` | `processGetMap:1098-1111` | `{register_map: snapshot, type_overlay?}`. `resolveRegisterMap`(`:1758-1776`)이 `params.unit_id=0`→공유 컨테이너(`SharedContainer`), 미지정→첫 디바이스 |
| `get_coils`/`get_discrete_inputs`/`get_holding_registers`/`get_input_registers` | `:820-958` | `{ok, address, quantity, values[]}` — 범위 읽기(그리드는 스냅샷 전체를 쓰므로 get_map/get_device_status 사용) |
| `get_register_defs` | `processGetRegisterDefs:1725-1738` | `{ok, register_defs:[{...current_value}]}` — 라벨/현재값(그리드 라벨 옵션) |

- **레지스터 스냅샷 형상**: `RegisterMap.GetSnapshot()`(`register_map.go:440+`) → 존재 영역만 `{coils:map[uint16]bool, discrete_inputs:map[uint16]bool, holding_registers:map[uint16]uint16, input_registers:map[uint16]uint16}`. `RegisterCounts()`(`:428-437`) → 영역별 정의 개수.

## 3. 백엔드 백킹 확장 대상 (SPEC-010 자산)

- **backedStore**: `backed_store.go:35-42`. 현재 상태 필드 = `inner, transport, upUnitID, mode, timeout, lastOK(atomic.Int64)`. **upstream 요청/에러/레이턴시 카운터 없음** → 추가 대상.
- **upstream 단일 계측점**: `sendUpstream:223-227`(모든 read/write 헬퍼 `:229-271`가 경유). → 여기서 req/err/latency 기록.
- **폴러**: `poller.go:107-142` `pollOnce`가 `bs.upstreamRead*` 호출 후 `recordPollSuccess:209-211`(lastOK). 폴 성공/실패가 카운터에 반영됨(sendUpstream 경유).
- **deviceBacking**: `backing_lifecycle.go:21-24` = `{transport, poller}`. **backedStore 참조 없음** → `store *backedStore` 필드 추가 필요(메트릭 조회 경로).
- **backings 맵 접근 규약**: `agent.go:1357-1360`(remove) / `:1286-1288`(add) — `a.mu` 보호. get_device_status에서 동일 규약으로 조회.
- **SPEC-010 M7(optional) 스킵 확인**: `SPEC-MODBUS-010/plan.md:60` — "백킹 조회/폴 성공·실패 카운터를 get_device_status에 노출(스키마 확장). 필요 시에만." → 본 SPEC이 완성.
- **DeviceStats vs backing 카운터 구분**: `device_manager.go:27-58` `DeviceStats{ReadCount,WriteCount,ErrorCount,LastAccess}`는 마스터→게이트웨이 **서빙** 통계(RequestHandler 기록)로, 게이트웨이→upstream 통신 통계와 별개. 실제 디바이스 latency/err는 backing 카운터를 써야 함(혼동 방지).

## 4. 프론트 소비 패턴 선례

- **exec 훅/서비스**: `useAgent.ts:145-154` `useExecAgent`(mutation), `agentService.ts:117-122` `execAgent(id,{command,params})` → `POST /agents/{id}/exec`, 반환 `{result}`(`types/agent.ts:169-170`).
- **기존 게이트웨이 관측 UI**: `AgentDetailPanel.tsx`에 `ClientsTab`(`:3931`, `list_clients` 5초 폴링, 언랩 `raw.clients ?? raw.result.clients` `:3950-3956`), `ModbusDevicesSection`(`:1347`, `list_devices`/`get_device_status` exec), `RegisterMapTable`(`:1176`), 타입 `ModbusDevice`/`ModbusDeviceDetail`(`:1126-1162`), 라벨 `REGISTER_AREA_LABELS`(`:1164-1170`)/순서(`:1173`). → 패널에서 형상·라벨 재사용.
- **agent-바인딩 패널 선례**: `AgentPanel.tsx`(자체 `useAgents` 폴링 `:45,51`), `SingleDevicePanel.tsx`(config.deviceId 바인딩, 미설정/부재/로딩 상태 `:46-81`).
- **원격 graceful**: `TargetContext`/`isRemoteTarget`(`AgentPanel.tsx:24,49`), 원격 exec 부재 안내(`AgentDetailPanel.tsx:3971-3978`).

## 5. 미해결/결정 사항 (open items → 설계 반영)

1. **버스 통계 시계열**: `get_status`에 히스토리 없음(확인). → 프론트 델타 누적(REQ-06-04, design §2.5).
2. **CRC errors**: 백엔드 전용 카운터 없음 + 제약(백엔드 변경 = 백킹 메트릭 한정). → Non-Goal, errors/min 대체(design §4).
3. **connected 판정**: `ModbusTransport`에 연결 상태 getter 유무 미확인. → indirect는 `!isStale()`, direct는 마지막 요청 성공/실패로 간이 판정(구현 시 트랜스포트 인터페이스 재확인 필요).
4. **레지스터 그리드 성능**: 대량 주소 시 `RegisterMapTable`의 expand/영역 토글 패턴(`:1176-1195`) 참조하여 가상화/페이지네이션 고려.

## 6. 산업 표준/참조

- EARS(Rolls-Royce Mavin 2009) 요구사항 패턴 적용(Ubiquitous/Event/State/Unwanted).
- 대시보드 패널 패턴은 xflow 자체 컨벤션(SPEC-DASHBOARD-001, SPEC-CHART-001, SPEC-FACILITY-DASHBOARD-001, SPEC-TRIGGER-PANEL-001)을 따름 — 프레임워크 개편 없음.
- MODBUS 예외/게이트웨이 시맨틱은 SPEC-MODBUS-010(백킹) 자산 재사용.
