---
id: SPEC-DASHBOARD-002
title: "Agent Status Panel — 타입 무관 실시간 통계·상태 대시보드 패널"
version: "0.1.0"
status: draft
created: 2026-08-05
updated: 2026-08-05
author: xtra
priority: P2
phase: plan
module: web/dashboard
lifecycle: spec-anchored
tier: M
tags: [dashboard, panel, agent, stats, realtime, frontend]
---

# SPEC-DASHBOARD-002 — 에이전트 상태 패널

## 개요 (Overview)

대시보드에 신규 패널 타입 `agent-status` 를 추가한다. 패널 생성 시 연결된 **임의의 에이전트 하나**를
에이전트 타입과 무관하게 선택하면, 그 에이전트가 보유한 **모든 타입에 공통인 통계·상태 정보**를
대시보드 새로고침 주기로 **실시간** 표출한다.

기존 `agents` 패널(전체 에이전트 목록 테이블, `AgentPanel.tsx`)과 구별되는, **단일 에이전트 상세
상태** 패널이다. 사실상 에이전트 상세 화면의 통계 탭(`AgentDetailPanel.tsx` §`StatsTab`, 363행)을
대시보드 패널로 이식한 것이며, 신규 백엔드/엔드포인트 없이 기존 프론트 훅과 API만 재사용한다.

## 환경 (Environment)

- 플랫폼: 웹 프론트엔드(React 19 + TypeScript 5.9, Vite/Vitest). 백엔드 변경 없음.
- 데이터 소스(기존, 신규 없음):
  - `GET /agents/{id}/stats` — `agentService.getAgentStats(id)` (`web/src/hooks/useAgent.ts:27` `useAgentStats`).
  - 실시간 타깃 훅 `useAgentStatsTarget(target, agentId)` (`web/src/hooks/useDetailTargets.ts:159`):
    로컬은 폴링, 원격은 SSE + 폴백 폴링(graceful degrade).
  - 에이전트 식별(name/type/enabled/status)용 `useAgentDetailTarget(target, agentId, detail)`
    (`web/src/hooks/useDetailTargets.ts:137`) → `AgentInfo` (`web/src/types/agent.ts:115`).
- 타입 정의: `AgentStatsInfo` (`web/src/types/agent.ts:86`), `EnhancedMessagesStats`
  (`web/src/types/agent.ts:44`), `AgentInfo` (`web/src/types/agent.ts:115`).
- 대시보드 패널 프레임워크: `web/src/stores/uiStore.ts`(PanelType/기본크기/기본config),
  `web/src/pages/dashboard/{AddPanelDialog,renderDashboardPanel,PanelSettingsDialog}.tsx`.
- 새로고침 주기: `useUIStore((s) => s.dashboardRefreshInterval)` (초 단위 → ms 변환, `AgentPanel.tsx:45` 선례).
- i18n: `web/src/lib/i18n/{ko.json,en.json}`.

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | 모든 에이전트 타입이 `AgentStatsInfo` 의 flat 공통 필드(status, messages_in/out, error_count, connected)를 제공한다 | 높음 | `web/src/types/agent.ts:86-96` 는 타입 무관 공통 스키마이며 `StatsTab`(`AgentDetailPanel.tsx:391`)이 타입 구분 없이 렌더 |
| A2 | `uptime`, `dropped_messages`, `messages`(EnhancedMessagesStats) 는 옵셔널이며 미제공 가능 | 높음 | `web/src/types/agent.ts:90,99,102` 옵셔널(`?`) 선언 |
| A3 | 에이전트 name/type 은 stats 응답에 없고 별도 detail 응답(`AgentInfo`)에서 얻는다 | 높음 | `AgentStatsInfo`(:86)에 name/type 없음, `AgentInfo`(:115)에 존재 |
| A4 | 신규 all-type 에이전트 picker 가 필요하다(기존 `needsAgent` 는 `a.type === 'modbus-gateway'` 로 필터) | 높음 | `AddPanelDialog.tsx:1245` `ModbusAgentStep` 은 gateway 타입만 필터; 본 패널은 전체 타입 대상 |
| A5 | 원격 타깃에서 name/type 등 일부 상세가 제한될 수 있다(graceful) | 중간 | `useAgentDetailTarget`(:137) 원격 분기; `SingleDevicePanel` 원격 graceful 선례 |

## 요구사항 (Requirements — EARS)

### REQ-01 — 패널 프레임워크 통합 (Ubiquitous)
시스템은 **항상** 신규 PanelType `agent-status` 를 대시보드 패널 프레임워크 4지점(uiStore PanelType
유니온 + `panelDefaultSize` + `createDefaultPanel`, AddPanelDialog 옵션/에이전트 선택 스텝,
renderDashboardPanel switch case, PanelSettingsDialog 편집 섹션)에 등록해야 하며, 단일 에이전트를
`config.agentId` 로 바인딩하고 모든 라벨을 i18n(ko/en) 으로 제공해야 한다.

### REQ-02 — 실시간 상태·통계 표출 (Event-Driven)
**WHEN** 패널에 유효한 `config.agentId` 가 설정되어 있고 대시보드 새로고침 주기가 도래하면
**THEN** 시스템은 `useAgentStatsTarget` 로 최신 통계를 취득하여 타입 무관 공통 필드
(status, enabled, connected, uptime, messages_in, messages_out, error_count, dropped_messages,
그리고 존재 시 EnhancedMessagesStats 요약)를 카드/타일로 갱신 표시해야 하며, 원격 타깃은 SSE +
폴백 폴링 경로를 사용해야 한다.

### REQ-03 — 상태 처리 (State-Driven)
**IF** 패널이 다음 상태 중 하나이면 **THEN** 시스템은 각 상태를 명확히 구분하여 표시해야 한다
(빈 화면/blank 렌더 금지):
- 미설정(agentId 미선택): 안내 문구 + 아이콘.
- 로딩: 스피너/스켈레톤.
- 에러 또는 무응답(에이전트 미실행/stats 취득 실패): 로드 불가 안내.
- 정상: 에이전트 name/type 헤더 + 상태/통계 타일.
또한 원격 제약으로 name/type 등이 제한될 경우 대체 표기(예: agentId, "-")로 graceful 하게 표시해야 한다.

### REQ-04 — 설정에서 에이전트 재선택 (State-Driven)
**IF** 사용자가 PanelSettingsDialog 에서 `agent-status` 패널을 편집하면 **THEN** 시스템은 전체
타입의 연결 에이전트 목록에서 에이전트를 재선택할 수 있게 하고, 변경 시 `config.agentId` 를
갱신하며 패널을 새 대상으로 즉시 반영해야 한다.

### REQ-05 — 하위 호환·품질 (Unwanted / cross-cutting)
시스템은 기존 `agents` 패널 및 기존 대시보드 패널·설정 동작을 회귀시키지 **않아야 하며**, 신규
백엔드/엔드포인트/npm 의존성을 추가하지 **않아야 한다**. 또한 타입별 특수 통계(예: MODBUS 레지스터)
및 에이전트 제어(start/stop) 를 이 패널에서 노출하지 **않아야 한다**(관측 전용). 결과물은 `tsc`
타입 체크와 `vitest` 를 통과해야 하고 ko/en i18n 키가 정합해야 한다.

## 명세 (Specifications)

### §S1. 표시 대상 공통 통계·상태 필드 매트릭스

| 표시 항목 | 소스 필드 (file:line) | 표기 | 비고 |
|-----------|----------------------|------|------|
| 상태 status | `AgentInfo.status` (`agent.ts:119`) / `AgentStatsInfo.status` (`agent.ts:89`) | 배지(running/stopped/error) | `AgentPanel` status 배지 패턴 재사용 |
| 활성화 enabled | `AgentInfo.enabled?` (`agent.ts:126`) | 배지/토글 표기(읽기전용) | 옵셔널(구버전 서버 누락 가능) |
| 연결 connected | `AgentStatsInfo.connected` (`agent.ts:94`) | 아이콘(Activity/CircleStop) | `AgentPanel:369` 선례 |
| 가동시간 uptime | `AgentStatsInfo.uptime?` (`agent.ts:90`) | 텍스트, 미제공 시 "-" | |
| 수신 messages_in | `AgentStatsInfo.messages_in` (`agent.ts:91`) | `toLocaleString()` | `StatsTab:395` |
| 송신 messages_out | `AgentStatsInfo.messages_out` (`agent.ts:92`) | `toLocaleString()` | `StatsTab:396` |
| 오류 error_count | `AgentStatsInfo.error_count` (`agent.ts:93`) | `toLocaleString()` | `StatsTab:397` |
| 드롭 dropped_messages | `AgentStatsInfo.dropped_messages?` (`agent.ts:102`) | 미제공 시 0 | `StatsTab:448` |
| 메시지 상세(요약) messages | `AgentStatsInfo.messages?` (`agent.ts:99`) = `EnhancedMessagesStats`(external/internal received/sent/errored, `agent.ts:44`) | 존재 시 external/internal 요약 그리드 | `StatsTab:402-442` |
| 헤더 name/type | `AgentInfo.name`/`AgentInfo.type` (`agent.ts:116-118`) | 헤더 타이틀 + 타입 라벨 | detail 훅 필요(A3) |

> 초기 범위: 위 공통 필드를 기본 표시. 표시 필드 토글/컬럼 수 등 커스터마이즈는 향후 여지(REQ-04 주석 참조, 초기 미포함).

### §S2. 패널 상태 매트릭스 (REQ-03)

| 상태 | 조건 | 렌더 |
|------|------|------|
| 미설정 | `!config.agentId` | Bot 아이콘 + `dashboard.agentStatus.notConfigured` 안내 (`SingleDevicePanel:46` 패턴) |
| 로딩 | stats/detail `isLoading` | 스피너/스켈레톤 (`StatsTab:370`, `SingleDevicePanel:55` 패턴) |
| 무응답/에러 | stats `!data` 또는 `error` | `dashboard.agentStatus.cannotLoad` 안내 |
| 정상 | stats `data` 존재 | 헤더(name/type) + 상태 배지 + 통계 타일 |
| 원격 제약 | 원격 타깃에서 detail 제한 | name→agentId, type→"-" 등 대체 표기(graceful) |

### §S3. 프레임워크 통합 지점 (REQ-01)

1. `uiStore.ts`: `PanelType` 유니온에 `'agent-status'` 추가; `panelDefaultSize` 케이스(권장 `{w:4,h:5,minW:3,minH:3}` — `modbus-*-devices` 선례 참조); `createDefaultPanel` 케이스 `{ type, title, config: { agentId: '' } }`.
2. `AddPanelDialog.tsx`: 패널 옵션 배열에 `agent-status` 항목 추가 + **신규 all-type 에이전트 선택 스텝**(`ModbusAgentStep`(:1228) 구조를 미러링하되 `a.type === '...'` 타입 필터 제거 → 전체 연결 에이전트 제시). 완료 시 `{ agentId }` config 저장.
3. `renderDashboardPanel.tsx`: `case 'agent-status':` 에서 신규 `AgentStatusPanel` 디스패치(panelId/title/config/handlers 전달).
4. `PanelSettingsDialog.tsx`: `panel.type === 'agent-status'` 편집 섹션에서 전체 타입 에이전트 재선택 → `onConfigChange({ agentId })`.

### §S4. 비범위 (Non-Goals)

- 타입별 특수 통계(MODBUS 레지스터 맵, 연결/노드 참조 상세 테이블 등)는 전용 패널 소관 — 본 패널 제외(단, EnhancedMessagesStats 요약은 타입 무관 공통이므로 포함).
- 에이전트 제어(start/stop/restart 버튼) — 관측 전용, 제외.
- 다중 에이전트 집계/비교 — 단일 바인딩만, 제외.

## 추적성 (Traceability)

| REQ | 명세 절 | AC (acceptance.md) |
|-----|---------|--------------------|
| REQ-01 패널 통합 | §S3 | AC-01-x |
| REQ-02 실시간 표출 | §S1, §S2(정상) | AC-02-x |
| REQ-03 상태 처리 | §S2 | AC-03-x |
| REQ-04 설정 재선택 | §S3-4 | AC-04-x |
| REQ-05 하위호환·품질 | §S4 | AC-05-x |

- 구현 계획 및 reuse map(file:line): `plan.md`
- 인수 기준(Given/When/Then): `acceptance.md`
