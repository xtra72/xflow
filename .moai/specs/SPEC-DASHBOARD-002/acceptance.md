# SPEC-DASHBOARD-002 — 인수 기준 (acceptance.md)

Given/When/Then 시나리오. 각 AC 는 spec.md 의 REQ 에 매핑된다.

---

## REQ-01 — 패널 프레임워크 통합

### AC-01-1 PanelType 등록
- **Given** 대시보드 패널 프레임워크(`uiStore.ts`)가 있고
- **When** `agent-status` 타입을 추가하면
- **Then** `PanelType` 유니온에 `'agent-status'` 가 포함되고, `panelDefaultSize('agent-status')` 가
  유효 크기(예: `{w:4,h:5,minW:3,minH:3}`)를, `createDefaultPanel('agent-status')` 가
  `{ type:'agent-status', title, config:{ agentId:'' } }` 를 반환한다.

### AC-01-2 패널 추가 옵션 노출
- **Given** AddPanelDialog 가 열려 있고
- **When** 패널 유형 목록을 조회하면
- **Then** `agent-status` 옵션이 i18n 라벨과 함께 표시되고, 선택 시 에이전트 선택 스텝으로 진입한다.

### AC-01-3 all-type 에이전트 선택
- **Given** 에이전트 선택 스텝에 진입했고 서로 다른 타입(예: modbus-gateway, xsfm, 기타)의 에이전트가 연결되어 있을 때
- **When** 목록을 조회하면
- **Then** **타입 필터 없이 전체 연결 에이전트**가 제시되고, 하나를 선택해 완료하면 `{ agentId }` 가 패널 config 로 저장된다.

### AC-01-4 디스패치 + i18n
- **Given** `agent-status` 패널이 대시보드에 존재하고
- **When** `renderDashboardPanel` 이 호출되면
- **Then** `AgentStatusPanel` 이 렌더되며, 모든 사용자 노출 문자열이 ko/en i18n 키로 제공된다(하드코딩 문자열 없음).

---

## REQ-02 — 실시간 상태·통계 표출

### AC-02-1 공통 통계 타일 표시
- **Given** 유효한 `config.agentId` 가 설정된 정상 에이전트가 있을 때
- **When** 패널이 렌더되면
- **Then** status, enabled, connected, uptime, messages_in, messages_out, error_count,
  dropped_messages 가 카드/타일로 표시되고, 수치는 `toLocaleString()` 로 포맷된다.

### AC-02-2 EnhancedMessagesStats 요약
- **Given** stats 응답의 `messages`(external/internal) 가 존재할 때
- **When** 패널이 렌더되면
- **Then** external/internal 의 received/sent/errored 요약 그리드가 표시된다.
- **And** `messages` 가 없으면 해당 요약 그리드는 생략된다(오류 없이).

### AC-02-3 주기적 실시간 갱신(로컬)
- **Given** 로컬 타깃이고 `dashboardRefreshInterval` 주기가 설정되어 있을 때
- **When** 주기가 도래하면
- **Then** `useAgentStatsTarget` 를 통해 최신 통계로 타일이 갱신된다.

### AC-02-4 원격 실시간 경로
- **Given** 원격 타깃일 때
- **When** 패널이 실시간 통계를 취득하면
- **Then** SSE 우선 + 폴백 폴링 경로(`useAgentStatsTarget` 원격 분기)가 사용되어 표시가 갱신된다.

---

## REQ-03 — 상태 처리 (blank 금지)

### AC-03-1 미설정
- **Given** `config.agentId` 가 비어 있을 때
- **When** 패널이 렌더되면
- **Then** 빈 화면 대신 미설정 안내(아이콘 + `dashboard.agentStatus.notConfigured`)가 표시된다.

### AC-03-2 로딩
- **Given** stats/detail 이 로딩 중일 때
- **When** 패널이 렌더되면
- **Then** 스피너 또는 스켈레톤이 표시된다.

### AC-03-3 무응답/에러
- **Given** 에이전트가 미실행이거나 stats 취득이 실패(`!data` 또는 `error`)일 때
- **When** 패널이 렌더되면
- **Then** 로드 불가 안내(`dashboard.agentStatus.cannotLoad`)가 표시되고 앱이 깨지지 않는다.

### AC-03-4 헤더 name/type
- **Given** 정상 에이전트가 바인딩되어 있을 때
- **When** 패널이 렌더되면
- **Then** 헤더에 에이전트 name 과 type 이 표시된다.

### AC-03-5 원격 제약 graceful
- **Given** 원격 타깃에서 detail(name/type) 이 제한될 때
- **When** 패널이 렌더되면
- **Then** name→agentId, type→"-" 등 대체 표기로 graceful 하게 표시되고, 통계 타일 표출은 유지된다.

---

## REQ-04 — 설정에서 에이전트 재선택

### AC-04-1 재선택 UI
- **Given** PanelSettingsDialog 에서 `agent-status` 패널을 편집할 때
- **When** 편집 섹션을 열면
- **Then** 전체 타입 연결 에이전트 목록에서 에이전트를 재선택하는 컨트롤이 표시된다.

### AC-04-2 config 반영
- **Given** 에이전트 재선택 컨트롤에서 다른 에이전트를 선택했을 때
- **When** 변경이 적용되면
- **Then** `onConfigChange({ agentId })` 로 `config.agentId` 가 갱신되고, 패널이 새 대상의 통계를 표시한다.

---

## REQ-05 — 하위 호환·품질 (Definition of Done)

### AC-05-1 기존 패널 무회귀
- **Given** 기존 `agents` 패널 및 기타 대시보드 패널이 있을 때
- **When** 신규 `agent-status` 를 추가한 뒤 대시보드를 사용하면
- **Then** 기존 패널 추가/렌더/설정 동작이 이전과 동일하게 작동한다(기존 테스트 그린).

### AC-05-2 신규 백엔드/의존성 없음
- **Given** 본 SPEC 구현 범위에서
- **When** 변경 사항을 검토하면
- **Then** 신규 백엔드 코드/엔드포인트/npm 의존성이 추가되지 않았고, 데이터는 기존 `/agents/{id}/stats` 재사용이다.

### AC-05-3 비범위 준수
- **Given** `agent-status` 패널에서
- **When** UI 를 확인하면
- **Then** 타입별 특수 통계(MODBUS 레지스터 등), 에이전트 제어(start/stop) 버튼, 다중 에이전트 집계가 노출되지 않는다(관측 전용, 단일 바인딩).

### AC-05-4 품질 게이트
- **Given** 구현이 완료되었을 때
- **When** `tsc --noEmit` 와 `vitest` 를 실행하면
- **Then** 타입 에러 0, 테스트 전부 그린이며, 신규 코드 커버리지 85% 목표를 만족한다.

### AC-05-5 i18n 정합
- **Given** ko.json 과 en.json 에 신규 라벨을 추가했을 때
- **When** 키 집합을 비교하면
- **Then** 두 로케일의 신규 키 집합이 일치하고 누락이 없다.

---

## 완료 정의 (Definition of Done)
- REQ-01~05 의 모든 AC 통과.
- `tsc` 타입 체크 통과, `vitest` 그린.
- 신규 백엔드/엔드포인트/의존성 0.
- ko/en i18n 키 정합.
- 기존 대시보드/`agents` 패널 회귀 없음.
