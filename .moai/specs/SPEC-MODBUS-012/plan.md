---
id: SPEC-MODBUS-012
title: "MODBUS Gateway 대시보드 패널 스위트 — 구현 계획"
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
tags: "modbus, gateway, dashboard, panel, plan, milestones, reuse-map"
---

# SPEC-MODBUS-012 — 구현 계획 (plan.md)

## 1. 기술 접근 (Technical Approach)

- **프론트 중심**: 6종 패널 컴포넌트 + 에이전트 선택 스텝 + 레지스터 그리드 신규 컴포넌트. 기존 대시보드 4단계 배선 패턴을 그대로 따른다(신규 프레임워크 없음).
- **소규모 백엔드**: `backedStore`에 upstream 관측 카운터(atomic) 추가 + `deviceBacking`이 store 참조 보유 + `get_device_status`/`list_devices` 응답 확장. 레지스터 스냅샷 경로 불변.
- **값 기반 상태**: 셀 active/degraded 는 프론트 유틸(`registerCellState.ts`)로 판정. 레지스터 그리드는 공유 컴포넌트(`RegisterMapGrid`)로 REQ-04/05 재사용.
- **개발 방법론**: hybrid. 신규 프론트 컴포넌트/유틸·백엔드 카운터 = TDD(vitest / `go test -race`), 기존 exec·백킹 경로 수정 = DDD(특성화·행위 보존).

## 2. 재사용 맵 (Reuse Map, file:line)

### 프론트 — 패널 프레임워크 4단계
| 배선 단계 | 파일:라인 | 변경 |
|---|---|---|
| PanelType 유니온 | `web/src/stores/uiStore.ts:177-203` | 6종 추가 |
| 기본 크기 | `web/src/stores/uiStore.ts:265-316` `panelDefaultSize` | 6 case 추가 |
| 기본 config | `web/src/stores/uiStore.ts:319-446` `createDefaultPanel` | 6 case 추가(`{agentId}`, 가상 맵은 `{agentId,unitId}`) |
| 다이얼로그 옵션 | `web/src/pages/dashboard/AddPanelDialog.tsx:112-124` (data 카테고리) | 6 옵션 + `needsAgent` 플래그 |
| 에이전트 선택 스텝 | `AddPanelDialog.tsx:818-986` `FacilityStep`(미러 대상) | `AgentStep`(`type==='modbus-gateway'`) 신규 + unit 2차 스텝 |
| 타입 스위치 | `web/src/pages/dashboard/renderDashboardPanel.tsx:83-266` | 6 case + import |

### 프론트 — 데이터·패턴 재사용
| 용도 | 파일:라인 |
|---|---|
| exec 훅/서비스 | `web/src/hooks/useAgent.ts:145-154` `useExecAgent` · `web/src/services/api/agentService.ts:117-122` `execAgent` |
| 에이전트 목록 필터 | `AddPanelDialog.tsx:833-837`(xsfm 필터 → gateway 로 미러) · `web/src/hooks/useAgent.ts:11-17` `useAgents` |
| 폴링 주기 | `web/src/pages/dashboard/panels/AgentPanel.tsx:45` `dashboardRefreshInterval` |
| 미설정/빈 상태 패턴 | `web/src/pages/dashboard/panels/SingleDevicePanel.tsx:46-81` |
| 원격 graceful degrade | `AgentPanel.tsx:49-57`(TargetContext) · `AgentDetailPanel.tsx:3971-3978` |
| exec 응답 언랩 패턴 | `AgentDetailPanel.tsx:3950-3956`(`clients` / `result.clients`) |
| 레지스터 맵 테이블 참조 | `AgentDetailPanel.tsx:1164-1276` `RegisterMapTable`·`REGISTER_AREA_LABELS`(그리드 신규 작성 시 라벨/순서 재사용) |
| 응답 타입 참조 | `AgentDetailPanel.tsx:1126-1162` `ModbusDevice`/`ModbusDeviceDetail` |
| 차트 프리미티브 | `web/src/pages/dashboard/panels/charts/`(LineChart 등 미니차트 재사용) |

### 백엔드 — 백킹 메트릭
| 용도 | 파일:라인 | 변경 |
|---|---|---|
| exec 스위치 | `internal/agent/modbusserver/agent.go:309-354` | 없음(기존 명령 재사용) |
| get_device_status 응답 | `agent.go:1377-1407` `processGetDeviceStatus` | `backing` 서브객체 추가 |
| list_devices 응답 | `agent.go:1184-1207` `processListDevices` | `backed`/`mode` 필드 추가 |
| backedStore 상태 | `internal/agent/modbusserver/backed_store.go:35-42` | atomic 카운터 추가(`reqCount`,`errCount`,`latencyNs`,`lastOK` 재사용) |
| upstream 송신 계측점 | `backed_store.go:223-227` `sendUpstream` | 요청/에러/레이턴시 기록 |
| 폴 성공 계측점 | `poller.go:107-142` `pollOnce` · `backed_store.go:209-211` `recordPollSuccess` | 성공/에러 카운트 반영 |
| backing 참조 보유 | `backing_lifecycle.go:21-24` `deviceBacking` | `store *backedStore` 필드 추가 |
| 디바이스 stats(서빙측, 구분) | `device_manager.go:27-58` `DeviceStats` | 변경 없음(백킹 카운터와 별개) |

## 3. 마일스톤 (우선순위 기반, 시간 예측 없음)

- **M1 (Priority High) — 프레임워크 통합 (REQ-01)**: 6종 `PanelType` 등록(uiStore/AddPanelDialog/renderDashboardPanel), `needsAgent` 스텝(gateway 필터), 미설정/빈 상태. i18n ko/en 스켈레톤. 플레이스홀더 패널 렌더로 배선 검증.
- **M2 (Priority High) — 목록 패널 2종 (REQ-02 프론트 골격, REQ-03)**: 가상 디바이스 목록(`list_devices`), 실제 디바이스 목록 골격(백엔드 메트릭 전이라 status/mode 표시). 폴링·정렬·배지.
- **M3 (Priority High) — 백엔드 백킹 메트릭 (REQ-06-01/02)**: `backedStore` atomic 카운터 + `deviceBacking.store` + `get_device_status.backing` + `list_devices.backed/mode`. `go test -race`. REQ-02 실측 표시 활성화.
- **M4 (Priority Medium) — 레지스터 맵 그리드 2종 (REQ-04, REQ-05)**: 공유 `RegisterMapGrid` 컴포넌트 + `registerCellState.ts` 값 기반 판정 유틸. 공유 맵(`get_map` unit 0) + 가상 맵(`get_device_status` unit N). POINTS/ACTIVE/DEGRADED 집계 + 셀 색상.
- **M5 (Priority Medium) — 버스/종합 통계 (REQ-06-03/04)**: 종합 통계 바(`get_status`+`list_devices`+`list_clients`), 버스 통계 미니차트(프론트 델타 누적, 차트 프리미티브 재사용).
- **M6 (Priority Low, Final Goal) — 회귀·품질·문서 (REQ-07)**: i18n 정합, 기존 패널/게이트웨이 회귀 테스트, tsc/vitest/`go test -race` 클린, 커버리지 ≥85%, README/도움말 갱신.

의존: M2는 M1 후. M3는 M2와 병행 가능하나 REQ-02 실측은 M3 완료 후. M4/M5는 M1 후 독립. M6는 전체 후.

## 4. 위험 및 대응 (Risks)

| 위험 | 영향 | 대응 |
|---|---|---|
| `get_status` 시계열 부재 | 버스 미니차트 데이터 없음 | 프론트 델타 누적(REQ-06-04). 세션-로컬 링버퍼. |
| CRC/frame 오류 미노출 | 목업 "CRC errors" 미충족 | Non-Goal 처리; error_count 델타를 "errors/min"으로 대체 표시. |
| backedStore↔agent 참조 경로 | 백킹 메트릭 노출 배선 복잡 | `deviceBacking`에 `store` 참조 추가(최소 변경), atomic 읽기. |
| 폴러/서빙 카운터 경합 | 데이터 레이스 | 모든 카운터 `atomic.Int64`. `go test -race`로 검증. |
| 원격 대시보드 exec 부재 | 원격에서 오동작 | TargetContext 감지 → 안내 문구(기존 패턴). |
| 대량 레지스터 그리드 렌더 | 성능 저하 | 영역별 가상화/페이지네이션(`RegisterMapTable` expand 패턴 참조). |

## 5. 완료 정의 (Definition of Done)

- 6종 패널이 생성→에이전트 선택→실시간 표출까지 동작(AC 전부 통과).
- 백엔드 `backing` 메트릭이 `get_device_status`로 노출되고 `go test -race` 클린.
- 값 기반 셀 상태 유틸이 공유되어 REQ-04/05가 동일 규칙 사용.
- 기존 대시보드 패널·게이트웨이 동작·type id 회귀 0, npm/go.mod 신규 의존성 0.
- i18n ko/en 정합, tsc/vitest/커버리지 게이트 통과.
