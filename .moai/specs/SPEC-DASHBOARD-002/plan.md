# SPEC-DASHBOARD-002 — 구현 계획 (plan.md)

## 관련 문서
- 명세: `spec.md`
- 인수 기준: `acceptance.md`

## 기술 접근 (Technical Approach)

프론트 전용. 기존 에이전트 상세 통계 탭(`AgentDetailPanel.tsx` §`StatsTab`)의 렌더 로직을 대시보드
단일-에이전트 패널로 이식한다. 데이터는 전부 기존 훅(`useAgentStatsTarget` 실시간 통계 +
`useAgentDetailTarget` 식별 정보)으로 취득하며, 신규 백엔드/엔드포인트/의존성은 없다.

핵심 차별점(신규 작업): 기존 `needsAgent` 에이전트 선택 스텝(`ModbusAgentStep`)은 `a.type ===
'modbus-gateway'` 로 타입을 필터하지만, 본 패널은 **전체 타입** 에이전트를 대상으로 하므로 타입
필터가 없는 all-type picker 스텝/섹션을 추가한다.

개발 방법론: hybrid — 신규 컴포넌트(`AgentStatusPanel`)와 신규 picker 는 TDD(신규 코드 85% 목표),
기존 프레임워크 4파일 수정은 DDD(회귀 방지 특성화 관점).

## 재사용 맵 (Reuse Map — file:line)

| 목적 | 재사용/참조 대상 | 파일:라인 |
|------|------------------|-----------|
| 실시간 통계(로컬 폴링 + 원격 SSE/폴백) | `useAgentStatsTarget(target, agentId)` | `web/src/hooks/useDetailTargets.ts:159` |
| 에이전트 식별(name/type/enabled/status) | `useAgentDetailTarget(target, agentId, detail)` | `web/src/hooks/useDetailTargets.ts:137` |
| 통계 타일 렌더 패턴(그대로 이식) | `StatsTab` (StatCard 그리드, external/internal, dropped 등) | `web/src/pages/agents/AgentDetailPanel.tsx:363` |
| 로컬 폴링 통계 훅 | `useAgentStats(id)` (refetchInterval 5000) | `web/src/hooks/useAgent.ts:27` |
| 공통 통계 타입 | `AgentStatsInfo` / `EnhancedMessagesStats` | `web/src/types/agent.ts:86` / `:44` |
| 에이전트 식별 타입 | `AgentInfo` (name/type/enabled) | `web/src/types/agent.ts:115` |
| 새로고침 주기(초→ms) | `useUIStore((s) => s.dashboardRefreshInterval)` | `web/src/pages/dashboard/panels/AgentPanel.tsx:45` |
| 원격/로컬 분기 + target context | `isRemoteTarget`, `useTargetContext` | `AgentPanel.tsx:24-25,49-50` |
| 미설정/로딩/미발견 상태 UI 패턴 | `SingleDevicePanel` (notConfigured/loading/notFound) | `web/src/pages/dashboard/panels/SingleDevicePanel.tsx:46,55,74` |
| status 배지/connected 아이콘 패턴 | `AgentPanel` status 렌더 | `AgentPanel.tsx:359-380` |
| PanelType 유니온/기본크기/기본config | `PanelType`, `panelDefaultSize`, `createDefaultPanel` | `web/src/stores/uiStore.ts:177,273,339` |
| 패널 옵션 + 에이전트 선택 스텝(미러 대상) | `PanelOption(needsAgent)`, `ModbusAgentStep` | `AddPanelDialog.tsx:95,131-136,1228` |
| switch 디스패치 지점 | `renderDashboardPanel` | `web/src/pages/dashboard/renderDashboardPanel.tsx:89` |
| 설정 편집 섹션(에이전트 재선택 패턴) | `MODBUS_PANEL_TYPES` 섹션 / `ModbusSettingsSection` | `PanelSettingsDialog.tsx:510-524` |
| i18n 키 파일 | `ko.json` / `en.json` | `web/src/lib/i18n/` |

## 산출/수정 파일

신규:
- `web/src/pages/dashboard/panels/AgentStatusPanel.tsx` — 단일 에이전트 상태·통계 패널 컴포넌트.
- `web/src/pages/dashboard/panels/AgentStatusPanel.test.tsx` — 상태 매트릭스/렌더 테스트.
- (신규 picker 를 별도 파일로 뺄 경우) all-type 에이전트 선택 스텝 — 초기엔 `AddPanelDialog.tsx` 내부 컴포넌트로 추가 가능.

수정:
- `web/src/stores/uiStore.ts` — PanelType 유니온 + panelDefaultSize + createDefaultPanel.
- `web/src/pages/dashboard/AddPanelDialog.tsx` — 옵션 항목 + all-type 에이전트 선택 스텝/핸들러.
- `web/src/pages/dashboard/renderDashboardPanel.tsx` — `case 'agent-status'`.
- `web/src/pages/dashboard/PanelSettingsDialog.tsx` — `agent-status` 편집 섹션(에이전트 재선택).
- `web/src/lib/i18n/ko.json`, `web/src/lib/i18n/en.json` — 신규 라벨 키.

## 아키텍처 방향

- 컴포넌트 인터페이스는 기존 패널 규약을 따른다: `{ panelId, title, config, onConfigChange?, onTitleChange? }`
  (`SingleDevicePanel` 시그니처, `renderDashboardPanel` 디스패치 계약과 일치).
- 데이터 획득은 컴포넌트 내부에서 `useAgentDetailTarget` + `useAgentStatsTarget` 병행 사용.
  `dashboardRefreshInterval` 로 로컬 폴링, 원격은 훅 내부 SSE/폴백에 위임(추가 로직 불필요).
- 렌더는 `StatsTab` 의 StatCard 그리드 구조를 재사용하되, 상단에 name/type 헤더 + status/enabled/
  connected 배지 행을 추가한다.

## 마일스톤 (우선순위 기반, 시간 예측 없음)

- Primary Goal (Priority High): 프레임워크 통합(REQ-01) + `AgentStatusPanel` 정상 상태 실시간 표출(REQ-02). 로컬 타깃 기준 동작.
- Secondary Goal (Priority High): 상태 매트릭스 전부(미설정/로딩/에러/원격 제약, REQ-03) + 설정 재선택(REQ-04).
- Final Goal (Priority Medium): i18n ko/en 정합 + 하위호환 회귀 확인 + tsc/vitest 그린(REQ-05).
- Optional Goal (Priority Low): 표시 필드 토글/컬럼 수 커스터마이즈(향후 여지, 본 SPEC 범위 밖).

## 위험 및 대응

| 위험 | 영향 | 대응 |
|------|------|------|
| stats 응답에 name/type 부재로 헤더 공백 | 헤더 blank | `useAgentDetailTarget` 병행 취득; 원격 제약 시 agentId/"-" 대체(REQ-03 graceful) |
| 원격 타깃 SSE 미지원/지연 | 실시간 표출 저하 | 훅 내부 폴백 폴링에 위임(기존 계약); 로딩/에러 상태 명시 |
| 기존 `needsAgent`(gateway 필터) 재사용 시 타입 잘못 필터 | 특정 타입만 노출 | 신규 all-type 스텝 별도 추가(기존 `ModbusAgentStep` 미변경, 회귀 방지) |
| createDefaultPanel/panelDefaultSize 누락 | 패널 추가 실패/기본크기 오류 | 4지점 통합 체크리스트로 커버(AC-01) |
| i18n 키 누락(ko/en 불일치) | 라벨 미표기 | 두 파일 동시 추가 + 키 정합 검사(REQ-05) |

## 품질 게이트

- `tsc --noEmit` 타입 에러 0.
- `vitest` 신규/기존 테스트 그린, 신규 코드 커버리지 85% 목표.
- 신규 백엔드/엔드포인트/npm 의존성 0.
- 코드 주석 한국어(프로젝트 `code_comments: ko`).
