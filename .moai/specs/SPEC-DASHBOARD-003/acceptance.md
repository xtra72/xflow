# SPEC-DASHBOARD-003 — 인수 기준 (acceptance.md)

Given/When/Then 시나리오. 각 AC 는 spec.md 의 REQ 에 매핑되며 기계적으로 검증 가능하다.

---

## REQ-01 — 타입별 부가 통계 provider 인터페이스

### AC-01-1 인터페이스 존재
- **Given** `internal/agent/agent.go` 에서
- **When** 옵셔널 인터페이스를 조회하면
- **Then** `SummaryStatsProvider`(또는 동등) 인터페이스가 요약 카운트 목록(`SummaryStat{Key, Value, Unit}`)을 반환하는 형태로 정의되어 있고, 기존 `ConnectionStatsProvider` 와 동일한 옵셔널 관례를 따른다.
- **Verify**: `grep -n "SummaryStatsProvider\|SummaryStat" internal/agent/agent.go`

### AC-01-2 미구현 에이전트 무영향
- **Given** `SummaryStatsProvider` 를 구현하지 않는 에이전트가 있을 때
- **When** 해당 에이전트의 통계를 조회하면
- **Then** 어떤 런타임 오류도 없고 기존 동작(빌드/테스트)이 그대로 통과한다.
- **Verify**: `go build ./...` && `go test ./internal/agent/...`

---

## REQ-02 — 어댑터 배선 및 DTO 전파

### AC-02-1 DTO 신규 옵셔널 필드
- **Given** `internal/api/handler/agent.go` 의 `AgentStatsInfo` 에서
- **When** 필드를 조회하면
- **Then** `SummaryStats []SummaryStatResponse` 가 `json:"summary_stats,omitempty"` 로 추가되어 있고 기존 flat/중첩 필드는 불변이다.
- **Verify**: `grep -n "summary_stats\|SummaryStatResponse" internal/api/handler/agent.go`

### AC-02-2 어댑터 조건부 채움
- **Given** 에이전트가 `SummaryStatsProvider` 를 구현할 때
- **When** `AgentServiceAdapter.AgentStats` 가 통계를 조립하면
- **Then** 타입 단언으로 요약 카운트가 `SummaryStats` 에 채워지고, 미구현 시 필드가 비어 응답에서 omitempty 로 생략된다(`Connections`/`NodeRefs` 채움과 동일 방식).
- **Verify**: `grep -n "SummaryStatsProvider" internal/api/service/agent_adapter.go`

### AC-02-3 신규 엔드포인트 없음
- **Given** 본 SPEC 변경 범위에서
- **When** 라우팅을 검토하면
- **Then** 신규 HTTP 엔드포인트가 추가되지 않았고, 데이터는 기존 `/api/v1/agents/{id}/stats` 응답(및 그 SSE)에만 실린다.

---

## REQ-03 — xsfm 첫 구현체

### AC-03-1 등록/동작 중 장비 수 노출
- **Given** xsfm 에이전트에 장비가 등록되어 있고 일부가 online 일 때
- **When** `SummaryStats()` 를 호출하면
- **Then** 최소한 `devicesTotal`(=`len(a.devices)`)와 `devicesOnline`(=`Device.Online==true` 개수) 요약 카운트가 반환된다.

### AC-03-2 lock 규약 준수
- **Given** xsfm 요약 집계 구현에서
- **When** 로스터를 읽으면
- **Then** `ListDevices()` 또는 동일 RLock 규약을 통해 접근하며, lock 보유 중 재진입(재귀 RLock)이 발생하지 않는다.
- **Verify**: `go test -race ./internal/agent/xsfm/...`

---

## REQ-04 — TS DTO 미러

### AC-04-1 TS 필드 정합
- **Given** `web/src/types/agent.ts` 의 `AgentStatsInfo` 에서
- **When** 타입을 조회하면
- **Then** `summary_stats?: AgentSummaryStat[]`(각 항목 `{ key: string; value: number; unit?: string }`)가 옵셔널로 미러링되어 백엔드 DTO 와 형상이 일치한다.
- **Verify**: `grep -n "summary_stats\|AgentSummaryStat" web/src/types/agent.ts`

---

## REQ-05 — 메시지 흐름 다이어그램(SVG) 뷰

### AC-05-1 SVG 다이어그램 렌더
- **Given** `viewMode === 'diagram'` 이고 유효한 stats.data 가 있을 때
- **When** 패널이 렌더되면
- **Then** External → 에이전트 → Internal 노드가 인라인 SVG 로 그려지고, 각 화살표에 in/out/error 카운트가 라벨로 표기된다(외부 차트 라이브러리 미사용).

### AC-05-2 보조 지표 표기
- **Given** 다이어그램 뷰에서
- **When** 렌더되면
- **Then** dropped_messages, buffer(pending/capacity), uptime 이 보조 지표로 함께 표기된다.

### AC-05-3 messages 부재 시 flat 폴백
- **Given** stats 응답에 `messages`(external/internal) 가 없을 때
- **When** 다이어그램 뷰가 렌더되면
- **Then** flat 필드(`messages_in/out`, `error_count`)로 단일 External↔Agent 흐름만 오류 없이 표기된다.

---

## REQ-06 — 출력 형식 선택기 viewMode

### AC-06-1 선택기 UI
- **Given** PanelSettingsDialog 에서 `agent-status` 패널을 편집할 때
- **When** `AgentStatusSettingsSection` 을 열면
- **Then** diagram/tile 을 선택하는 `<select>`(agent-picker 미러링) 컨트롤이 표시된다.
- **Verify**: `grep -n "viewmode\|viewMode" web/src/pages/dashboard/PanelSettingsDialog.tsx`

### AC-06-2 config 저장·반영
- **Given** 사용자가 출력 형식을 변경했을 때
- **When** 변경이 적용되면
- **Then** `onConfigChange({ viewMode })` 로 `panel.config.viewMode` 가 저장되고, 패널이 해당 뷰로 전환된다.

### AC-06-3 기본값 폴백(하위호환)
- **Given** `config.viewMode` 가 미설정이거나 인식 불가 값일 때
- **When** 패널이 렌더되면
- **Then** 기본 `tile` 뷰(DASHBOARD-002 기존 동작)로 폴백되어 렌더된다.

---

## REQ-07 — 두 뷰 공통의 타입별 부가 통계 표출

### AC-07-1 요약 카운트 diagram·tile 공통 표출
- **Given** stats 응답에 `summary_stats` 가 존재할 때
- **When** diagram 뷰와 tile 뷰 각각을 렌더하면
- **Then** 요약 카운트(예: 등록/동작 중 장비 수)가 두 뷰 모두에서 표시된다.

### AC-07-2 부재 시 graceful 생략
- **Given** `summary_stats` 가 없을 때(미구현/원격 제약)
- **When** 패널이 렌더되면
- **Then** 요약 카운트 영역이 오류 없이 생략되고 나머지 표출은 유지된다(`EnhancedMessagesStats` 조건부 렌더와 동일).

---

## REQ-08 — i18n

### AC-08-1 ko/en 키 정합
- **Given** 신규 라벨(뷰 모드, 다이어그램 노드/화살표, 요약 카운트)을 추가했을 때
- **When** `dashboard.agentStatus.*` 하위 키 집합을 비교하면
- **Then** ko.json 과 en.json 의 신규 키 집합이 일치하고 누락이 없으며, 사용자 노출 문자열에 하드코딩이 없다.

---

## REQ-09 — 하위 호환·비범위·품질 (Definition of Done)

### AC-09-1 DASHBOARD-002 무회귀
- **Given** DASHBOARD-002 의 타일 뷰·상태 매트릭스·설정 재선택이 있을 때
- **When** 본 SPEC 변경 후 패널을 사용하면
- **Then** 기존 동작(미설정/로딩/에러/정상 렌더, 에이전트 재선택)이 이전과 동일하게 작동한다(기존 테스트 그린).

### AC-09-2 신규 엔드포인트/의존성 없음
- **Given** 본 SPEC 구현 범위에서
- **When** 변경 사항을 검토하면
- **Then** 신규 백엔드 엔드포인트, 신규 npm 의존성, 차트 라이브러리가 추가되지 않았다.

### AC-09-3 exec 경로 및 타일 재설계 미침범
- **Given** `AgentDetailPanel` 의 `exec(list_devices)` 경로가 있을 때
- **When** 변경 사항을 검토하면
- **Then** 해당 경로는 변경/의존되지 않고, 기존 타일 뷰는 토글·요약 카운트 추가 외에 재설계되지 않는다.

### AC-09-4 관측 전용·단일 바인딩 유지
- **Given** `agent-status` 패널에서
- **When** UI 를 확인하면
- **Then** 에이전트 제어(start/stop) 버튼과 다중 에이전트 집계가 노출되지 않는다.

### AC-09-5 품질 게이트
- **Given** 구현이 완료되었을 때
- **When** `go build ./...`, `go test ./internal/...`, `tsc --noEmit`, `vitest` 를 실행하면
- **Then** 모두 통과(타입 에러 0, 테스트 그린)하고 신규 코드 커버리지 85% 목표를 만족한다.

---

## 완료 정의 (Definition of Done)
- REQ-01~09 의 모든 AC 통과.
- 백엔드 `go build`/`go test`(+`-race` xsfm) 그린, 프론트 `tsc`/`vitest` 그린.
- 신규 엔드포인트/npm 의존성/차트 라이브러리 0, `exec(list_devices)` 경로 미변경.
- ko/en i18n 키 정합, DASHBOARD-002 회귀 없음, 관측 전용·단일 바인딩 유지.
