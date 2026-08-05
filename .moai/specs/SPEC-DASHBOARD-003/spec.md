---
id: SPEC-DASHBOARD-003
title: "Agent Status Panel — 메시지 흐름 다이어그램 + 타입별 부가 통계(백엔드 provider 확장)"
version: "0.1.0"
status: draft
created: 2026-08-05
updated: 2026-08-05
author: xtra
priority: P2
phase: plan
module: web/dashboard, internal/agent, internal/api
lifecycle: spec-anchored
tier: M
tags: [dashboard, panel, agent, stats, svg, diagram, backend, provider, frontend]
---

# SPEC-DASHBOARD-003 — 에이전트 상태 패널: 메시지 흐름 다이어그램 + 타입별 부가 통계

## 개요 (Overview)

SPEC-DASHBOARD-002 로 도입된 단일 에이전트 상태 패널(`agent-status`, `AgentStatusPanel.tsx`)에
두 가지 능력을 **추가**한다.

1. **타입별 부가 통계(백엔드 provider 확장)** — 에이전트가 자신의 타입에 유의미한 요약 카운트
   (예: xsfm 의 등록 장비 수·동작 중 장비 수)를 노출할 수 있는 **선택적 Go 인터페이스**를
   신설한다. 기존 옵셔널 인터페이스 패턴(`ConnectionStatsProvider`, `agent.go:94-99`)을 그대로
   미러링하며, API 어댑터가 타입 단언으로 채워 `/agents/{id}/stats` 응답(및 SSE)에 실어 보낸다.
   신규 엔드포인트는 없다.
2. **메시지 흐름 다이어그램 + 출력 형식 선택기** — 현재 타일 전용 패널에, External → 에이전트 →
   Internal 노드를 화살표로 잇고 in/out/error 카운트 및 dropped/buffer/uptime 을 표기하는
   **인라인 SVG 다이어그램 뷰**를 추가한다. `panel.config.viewMode`(diagram | tile) 로 뷰를
   전환하며, 기존 타일 뷰는 모드 중 하나로 보존한다.

두 능력은 SPEC-DASHBOARD-002 의 관측 전용·단일 바인딩·신규 엔드포인트 없음 원칙을 유지한다.
타입별 부가 통계는 첫 구현체로 `xsfm`("ui-line") 에이전트만 제공하며, 인터페이스는 이후 다른
에이전트 타입(modbus-gateway, hvac 계열, mqtt-client 등)이 각자의 타입 유의미 카운트를 구현할 수
있도록 범용적으로 설계한다.

## 환경 (Environment)

- 백엔드: Go 1.23+ (`internal/agent`, `internal/api`). 신규 엔드포인트·의존성 없음.
- 프론트엔드: React 19 + TypeScript 5.9 (Vite/Vitest). 신규 npm 의존성·차트 라이브러리 없음.
- 데이터 소스(기존, 신규 없음):
  - `GET /api/v1/agents/{id}/stats` → `AgentServiceAdapter.AgentStats` (`internal/api/service/agent_adapter.go:390-529`).
  - 실시간 훅 `useAgentStatsTarget(target, agentId)` (`web/src/hooks/useDetailTargets.ts:159`): 로컬 폴링 + 원격 SSE/폴백.
- 확장 지점(백엔드):
  - 옵셔널 인터페이스 선례: `ConnectionStatsProvider` (`internal/agent/agent.go:94-99`), `BufferInfoProvider`(:71-73), `InternalStatsRecorder`(:104-108).
  - 어댑터 조건부 채움 선례: `Connections`/`NodeRefs` 타입 단언 채움 (`agent_adapter.go:466-526`).
  - DTO: `AgentStatsInfo` (`internal/api/handler/agent.go:126-149`).
  - 첫 구현체 소스: `xsfm.XSFMAgent` — 장비 로스터 `ListDevices` (`internal/agent/xsfm/agent.go:949-957`, `len(a.devices)`), 온라인 필드 `Device.Online` (`internal/agent/xsfm/device.go:28`).
- 확장 지점(프론트):
  - DTO 미러: `AgentStatsInfo` (`web/src/types/agent.ts:86-109`).
  - 패널: `AgentStatusPanel.tsx` (현재 타일 전용, `StatCard` :31-38, EnhancedMessagesStats 조건부 렌더 :164-205).
  - 설정: `PanelSettingsDialog.tsx` `AgentStatusSettingsSection` (:1276-1312, agent-picker `<select>`).
  - 인라인 시각화 선례(차트 라이브러리 미사용·자족·data-testid 규약): `RegisterMapGrid.tsx` (`web/src/pages/dashboard/panels/modbus/RegisterMapGrid.tsx`).
  - i18n: `web/src/lib/i18n/{ko.json,en.json}` — `dashboard.agentStatus.*` (:230-236).
- 새로고침 주기: `useUIStore((s) => s.dashboardRefreshInterval)` (기존 패널 계약, DASHBOARD-002 재사용).

## 가정 (Assumptions)

| # | 가정 | 신뢰도 | 근거 / 검증 |
|---|------|--------|-------------|
| A1 | 옵셔널 인터페이스 + 어댑터 타입 단언 패턴으로 기존 flat/중첩 DTO 를 깨지 않고 필드를 추가할 수 있다 | 높음 | `Connections`/`NodeRefs` 가 동일 방식으로 이미 조건부 채움(`agent_adapter.go:466-526`) |
| A2 | 미구현 에이전트는 새 필드가 빈 배열/omitempty 로 반환되어 프론트가 graceful 하게 생략할 수 있다 | 높음 | `EnhancedMessagesStats`(옵셔널) 및 `Connections`(빈 배열) 선례; 패널 조건부 렌더(`AgentStatusPanel.tsx:165`) |
| A3 | xsfm 는 lock 하에 `len(a.devices)` 와 `Device.Online` 집계로 등록/동작 중 장비 수를 산출할 수 있다 | 높음 | `ListDevices`(RLock, `xsfm/agent.go:949-957`), `Device.Online`(`device.go:28`) |
| A4 | 다이어그램에 필요한 in/out/error·dropped·buffer·uptime 은 기존 DTO 공통 필드로 충분하다 | 높음 | `messages_in/out`, `error_count`, `messages`(external/internal), `dropped_messages`, `buffer`, `uptime` 모두 `agent.ts:86-109` 존재 |
| A5 | 인라인 SVG 로 노드·화살표 다이어그램을 외부 라이브러리 없이 그릴 수 있다 | 높음 | `RegisterMapGrid` 가 자족 시각화 컴포넌트로 동작(차트 라이브러리 미사용) |
| A6 | `viewMode` 를 `panel.config` 에 저장해도 기존 config(agentId) 및 패널 프레임워크 계약을 깨지 않는다 | 높음 | `config` 는 `Record<string, unknown>`; MODBUS 패널이 `unitId` 등 추가 키를 이미 저장 |
| A7 | 타입별 부가 통계의 라벨은 안정적 key 로 전달하고 프론트에서 i18n 매핑(미매핑 시 key fallback)한다 | 중간 | 에이전트 타입별 key 집합은 가변 → key 기반 + defaultValue fallback 이 확장 안전 |

## 요구사항 (Requirements — EARS)

### REQ-01 — 타입별 부가 통계 provider 인터페이스 (Ubiquitous)
시스템은 **항상** `internal/agent/agent.go` 에 선택적 인터페이스(예: `SummaryStatsProvider`)를 제공해야
하며, 이는 에이전트 타입에 유의미한 요약 카운트 목록(안정적 key + 정수 value + 선택 unit)을 반환한다.
이 인터페이스는 기존 옵셔널 인터페이스(`ConnectionStatsProvider` 등)와 동일한 관례를 따르고, 구현하지
않는 에이전트에 대해서는 어떤 동작 변화도 강제하지 **않아야** 한다.

### REQ-02 — 어댑터 배선 및 DTO 전파 (Event-Driven)
**WHEN** `AgentServiceAdapter.AgentStats` 가 통계를 조립하면 **THEN** 시스템은 에이전트가 REQ-01
인터페이스를 구현하는 경우에만 타입 단언으로 요약 카운트를 취득하여 `AgentStatsInfo` DTO 의 신규
옵셔널 필드(예: `summary_stats`, `json:"summary_stats,omitempty"`)에 채워야 하며, 미구현 시 필드를
비우거나 생략(omitempty)해야 한다. 이 데이터는 기존 `/agents/{id}/stats` 응답 및 그 SSE 스트림에만
실려야 하고 신규 엔드포인트를 추가하지 **않아야** 한다.

### REQ-03 — xsfm 첫 구현체 (State-Driven)
**IF** 에이전트가 `xsfm` 타입이면 **THEN** 시스템은 REQ-01 인터페이스를 구현하여 최소한 **등록 장비
수**(`len(a.devices)`)와 **동작 중 장비 수**(`Device.Online == true` 개수)를 요약 카운트로 노출해야
하며, 선택적으로 line/station/group 카운트를 포함할 수 있다. 집계는 로스터 lock 규약(`ListDevices`
RLock 패턴)을 준수해야 한다.

### REQ-04 — TS DTO 미러 (Ubiquitous)
시스템은 **항상** `web/src/types/agent.ts` 의 `AgentStatsInfo` 에 신규 필드를 옵셔널로 미러링하여
백엔드 DTO 와 형상을 일치시켜야 하며, 각 요약 항목은 key/value(및 선택 unit)를 표현해야 한다.

### REQ-05 — 메시지 흐름 다이어그램(SVG) 뷰 (Event-Driven)
**WHEN** 패널의 `viewMode` 가 `diagram` 이고 유효한 통계가 존재하면 **THEN** 시스템은 External →
에이전트 → Internal 노드를 화살표로 연결하고, 각 화살표에 방향별 in/out/error 카운트를, 보조로
dropped/buffer/uptime 을 표기하는 **인라인 SVG** 다이어그램을 렌더해야 한다. 렌더는 외부 차트
라이브러리 없이 자족 컴포넌트로 구현해야 한다.

### REQ-06 — 출력 형식 선택기 viewMode (State-Driven)
**IF** 사용자가 `AgentStatusSettingsSection` 에서 출력 형식을 선택하면 **THEN** 시스템은 선택값을
`panel.config.viewMode`(diagram | tile) 로 저장하고, 패널이 이를 읽어 뷰를 전환해야 한다. `viewMode`
미설정/미인식 값은 기본값으로 폴백해야 하며(예: 기본 `tile` 로 하위호환), 기존 타일 뷰가 두 모드 중
하나로 보존되어야 한다.

### REQ-07 — 두 뷰 공통의 타입별 부가 통계 표출 (State-Driven)
**IF** stats 응답에 REQ-02 요약 카운트가 존재하면 **THEN** 시스템은 이를 **다이어그램·타일 두 뷰
모두에서** 표시해야 하며, 존재하지 않으면(미구현/원격 제약) 오류 없이 해당 영역을 생략(graceful)해야
한다(기존 `EnhancedMessagesStats` 조건부 렌더와 동일한 부재 처리).

### REQ-08 — i18n (Ubiquitous)
시스템은 **항상** 신규 사용자 노출 문자열(뷰 모드 라벨, 다이어그램 노드/화살표 라벨, 요약 카운트
라벨 등)을 `dashboard.agentStatus.*` 하위 키로 `ko.json` 과 `en.json` 양쪽에 정합하게 제공해야 한다.

### REQ-09 — 하위 호환·비범위·품질 (Unwanted / cross-cutting)
시스템은 다음을 하지 **않아야** 한다:
- 기존 `agent-status` 패널 및 DASHBOARD-002 동작을 회귀시키기(타일 뷰·상태 매트릭스·설정 재선택 보존).
- 신규 엔드포인트/신규 npm 의존성/차트 라이브러리 추가.
- 기존 타일을 토글 추가 이상으로 재설계하기.
- `AgentDetailPanel` 이 사용하는 `exec(list_devices)` 경로를 변경/의존하기(본 SPEC 은 stats 응답만 사용).
- 에이전트 제어(start/stop) 또는 다중 에이전트 집계 노출(관측 전용·단일 바인딩 유지).
결과물은 `go build`/`go test`(백엔드), `tsc --noEmit`/`vitest`(프론트) 를 통과하고 ko/en i18n 키가
정합해야 한다.

## 명세 (Specifications)

### §S1. provider 인터페이스 및 DTO 형상 (REQ-01, REQ-02, REQ-04)

백엔드(`internal/agent/agent.go`, `ConnectionStatsProvider` 미러):

| 요소 | 형상(안) | 비고 |
|------|----------|------|
| 요약 항목 struct | `SummaryStat{ Key string; Value int64; Unit string }` | `Key` 는 안정적 식별자(i18n 매핑용), `Unit` 옵셔널 |
| 옵셔널 인터페이스 | `SummaryStatsProvider interface { SummaryStats() []SummaryStat }` | 미구현 에이전트는 무영향(A1/A2) |

DTO(`internal/api/handler/agent.go`):

| 요소 | 형상(안) | 비고 |
|------|----------|------|
| 응답 항목 | `SummaryStatResponse{ Key string \`json:"key"\`; Value int64 \`json:"value"\`; Unit string \`json:"unit,omitempty"\` }` | |
| `AgentStatsInfo` 신규 필드 | `SummaryStats []SummaryStatResponse \`json:"summary_stats,omitempty"\`` | omitempty → 미구현 시 생략 |

어댑터(`agent_adapter.go`, `Connections` 채움 미러):
- `if sp, ok := ag.(agent.SummaryStatsProvider); ok { result.SummaryStats = map(sp.SummaryStats()) }`.

TS 미러(`web/src/types/agent.ts`):
- `interface AgentSummaryStat { key: string; value: number; unit?: string }`
- `AgentStatsInfo.summary_stats?: AgentSummaryStat[]`.

> 인터페이스/필드 명칭은 확정 아님(구현 시 확정). 요구는 "옵셔널·타입 단언·omitempty·범용 key/value" 계약이다.

### §S2. xsfm 첫 구현체 요약 카운트 (REQ-03)

| 요약 항목 | key(안) | 산출 | 소스(file:line) |
|-----------|---------|------|-----------------|
| 등록 장비 수 | `devicesTotal` | `len(a.devices)`(RLock) | `xsfm/agent.go:949-957` (ListDevices 규약) |
| 동작 중 장비 수 | `devicesOnline` | `count(Device.Online == true)` | `xsfm/device.go:28` |
| (선택) 라인/스테이션/그룹 수 | `lines`/`stations`/`groups` | 로스터 파생 집계 | 초기 선택, 미포함 가능 |

- 집계 함수는 `ListDevices()` 를 통하거나 동일한 RLock 규약을 준수한다(로스터 경합 방지, HVAC lock 트랩 회피).

### §S3. 다이어그램(SVG) 매핑 (REQ-05)

| 시각 요소 | 소스 필드 | 표기 |
|-----------|-----------|------|
| 노드 External ↔ Agent 화살표(수신) | `messages.external.received` (없으면 `messages_in`) | 화살표 라벨 in |
| 노드 Agent → External 화살표(송신) | `messages.external.sent` (없으면 `messages_out`) | 화살표 라벨 out |
| 외부 오류 | `messages.external.errored` (없으면 `error_count`) | 화살표 라벨 err |
| 노드 Agent ↔ Internal 화살표 | `messages.internal.received` / `.sent` / `.errored` | 화살표 라벨 in/out/err |
| 보조 지표 | `dropped_messages`, `buffer`(pending/capacity), `uptime` | 노드 하단/여백 텍스트 |
| 상태 색상 | `status`/`connected` | 에이전트 노드 색상(running/stopped/error), `AgentPanel` 배지 팔레트 재사용 |

- 인라인 SVG(`<svg><line/rect/text .../>`)로 구성, data-testid 규약(예: `agent-status-diagram`, `agent-status-arrow-external-in`)을 부여한다(RegisterMapGrid 규약 참조).
- `messages`(EnhancedMessagesStats) 부재 시 flat 필드(`messages_in/out`, `error_count`)로 폴백해 단일 External↔Agent 흐름만 표기(graceful).

### §S4. viewMode 선택기 및 두 뷰 표출 (REQ-06, REQ-07)

- 저장: `panel.config.viewMode: 'diagram' | 'tile'`. 설정 UI 는 `AgentStatusSettingsSection` 에 agent-picker `<select>`(:1296-1308) 를 미러링한 `<select>` 로 추가(data-testid 예: `agent-status-viewmode-select`).
- 읽기: `AgentStatusPanel` 이 `config.viewMode` 를 읽어 뷰 분기. 미설정/미인식 → 기본 `tile`(하위호환, DASHBOARD-002 동작 보존).
- 두 뷰 공통: REQ-02 `summary_stats` 존재 시 요약 카운트 영역을 diagram·tile 모두에 표출(부재 시 생략).
- 타일 뷰: 기존 렌더(:106-216) 유지 + 요약 카운트 영역만 추가(그 외 재설계 금지, REQ-09).

### §S5. 비범위 (Non-Goals)

- 타입별 특수 통계의 상세 테이블(MODBUS 레지스터 맵, 연결/노드 참조 테이블) — 전용 패널 소관, 제외(단, 범용 요약 카운트는 포함).
- `exec(list_devices)` 제어 경로 사용/변경 — 제외(stats 응답만 사용, `AgentDetailPanel` 무영향).
- 에이전트 제어(start/stop/restart), 다중 에이전트 집계/비교 — 제외(관측 전용·단일 바인딩).
- xsfm 외 타입의 provider 구현 — 인터페이스만 범용 설계, 실제 구현은 후속 SPEC.
- 기존 타일 레이아웃 재설계 — 토글·요약 카운트 추가 외 변경 금지.

## 추적성 (Traceability)

| REQ | 명세 절 | AC (acceptance.md) |
|-----|---------|--------------------|
| REQ-01 provider 인터페이스 | §S1 | AC-01-x |
| REQ-02 어댑터·DTO 전파 | §S1 | AC-02-x |
| REQ-03 xsfm 구현체 | §S2 | AC-03-x |
| REQ-04 TS DTO 미러 | §S1 | AC-04-x |
| REQ-05 SVG 다이어그램 | §S3 | AC-05-x |
| REQ-06 viewMode 선택기 | §S4 | AC-06-x |
| REQ-07 두 뷰 공통 요약 | §S4 | AC-07-x |
| REQ-08 i18n | §S1~S4 | AC-08-x |
| REQ-09 하위호환·비범위·품질 | §S5 | AC-09-x |

- 구현 계획 및 reuse map(file:line): `plan.md`
- 인수 기준(Given/When/Then): `acceptance.md`
