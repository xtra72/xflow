# SPEC-FACILITY-DASHBOARD-001 — 인수 기준 (acceptance.md)

> 형식: Gherkin (Given-When-Then, 한국어). 각 시나리오는 spec.md의 REQ-FACDASH-001-* 요구사항에 대응한다.
> 소비 표면(AIRPURIFIER-001 exec, DASHBOARD-001 영속화)은 스텁/실표면 모두에서 검증 가능하도록 작성한다.

## Feature: 라인 패널 (Line panel)

### Scenario: 라인도에 호선의 역사가 순서대로 배치되고 역사별 상태 요약이 표시된다
- **Given** 대상 에이전트의 역사 레지스트리에 2호선 소속 역사 3개가 order 1,2,3으로 등록되어 있고
- **And** 각 역사에 online/offline·power on/off가 섞인 공기청정기 로스터가 있고
- **When** 사용자가 `facility-line` 패널을 2호선 대상으로 표시하면
- **Then** 라인도에 세 역사가 order 순으로 배치되고
- **And** 각 역사 노드에 online/offline 수·power on/off 수·결함 지시자 배지가 표시된다 (REQ-FACDASH-001-01-02)
- **And** 역사별 간략 상태 목록에 역사명·총 기기 수·online/offline·power on/off 요약이 표시된다 (REQ-FACDASH-001-01-03)

### Scenario: 라인 통계가 상태별 카운트로 집계된다
- **Given** 2호선 로스터에 총 10대(online 8/offline 2, power on 6/off 4, fan_speed 1:2·2:3·3:1)가 있을 때
- **When** 라인 패널이 통계 영역을 렌더링하면
- **Then** 총 기기 수 10, online 8/offline 2, power on 6/off 4, fan_speed 분포(1:2, 2:3, 3:1)가 카운트로 표시된다 (REQ-FACDASH-001-01-04, 05-02)

### Scenario: 라인 일괄 제어가 호선 전 기기에 fan-out되고 집계 결과가 표시된다
- **Given** 2호선에 기기 10대가 있고
- **When** 사용자가 라인 패널에서 "전체 전원 ON"을 실행하면
- **Then** 패널은 `execAgent(agentId, {command:"set_power", line:"line-2", params:{power:true}})`를 호출하고 (REQ-FACDASH-001-06-01)
- **And** 응답의 멤버별 결과(`ok`/`error`/`timeout`)를 성공 수·타임아웃 수 요약 + 실패/타임아웃 상세로 표시한다 (REQ-FACDASH-001-06-02)

### Scenario: 빈/미등록 호선은 안전하게 안내된다
- **Given** 대상 호선에 역사·기기가 하나도 없을 때
- **When** 라인 패널이 렌더링되면
- **Then** "표시할 역사/기기가 없습니다" 안내가 표시되고 일괄 제어가 비활성화되며 크래시가 발생하지 않는다 (REQ-FACDASH-001-01-06, 06-03)

## Feature: 역사 패널 (Station panel)

### Scenario: 역사 통계와 기기별 상태 목록이 표시된다
- **Given** 특정 역사에 공기청정기 4대(online 3/offline 1, power on 2/off 2)가 소속되어 있을 때
- **When** 사용자가 `facility-station` 패널을 그 역사 대상으로 표시하면
- **Then** 역사 통계(총 4, online 3/offline 1, power on 2/off 2, fan_speed 분포)가 표시되고 (REQ-FACDASH-001-02-02)
- **And** 각 기기의 상태(기기명/ID·online·power·fan_speed·place/index)가 목록으로 표시된다 (REQ-FACDASH-001-02-03)

### Scenario: 역사 일괄 제어가 역사 전 기기에 fan-out되고 집계 결과가 표시된다
- **Given** 대상 역사에 기기 4대가 있고
- **When** 사용자가 역사 패널에서 "전체 풍량 2단"을 실행하면
- **Then** 패널은 `execAgent(agentId, {command:"set_fan_speed", station:"ST-101", params:{fan_speed:2}})`를 호출하고 (REQ-FACDASH-001-02-04, 06-01)
- **And** 멤버별 `ok`/`error`/`timeout` 집계 결과가 표시된다 (REQ-FACDASH-001-06-02)

### Scenario: 미등록 역사는 안내된다
- **Given** 대상 역사가 역사 레지스트리에 없을 때
- **When** 역사 패널이 렌더링되면
- **Then** "미등록 역사" 안내가 표시되고 일괄 제어가 비활성화된다 (REQ-FACDASH-001-02-05)

## Feature: 기기 패널 (Device panel)

### Scenario: 단일 기기 상태가 표시된다
- **Given** 대상 기기가 online, power=on, fan_speed=2, station=ST-101, place=대합실, index=3일 때
- **When** 사용자가 `facility-device` 패널을 그 기기 대상으로 표시하면
- **Then** power/fan_speed/online/station/place/index가 표시된다 (REQ-FACDASH-001-03-01)

### Scenario: 제어 성공(상태 에코 반영)이 사용자에게 표시된다
- **Given** 기기 패널이 표시되어 있고 대상 기기의 응답 대기가 활성일 때
- **When** 사용자가 "전원 ON"을 실행하고 타임아웃 내에 상태 에코가 반영되면
- **Then** 패널은 `executeCommand(deviceId, {command:"set_power", params:{power:true}})`를 호출하고
- **And** 결과를 성공(`ok`, 상태 에코 반영)으로 명시적으로 표시한다 (REQ-FACDASH-001-03-02)

### Scenario: 제어 타임아웃이 사용자에게 표시된다
- **Given** 기기 패널이 표시되어 있고
- **When** 사용자가 제어를 실행했으나 `control_response_timeout` 내 상태 에코가 도착하지 않으면
- **Then** 패널은 결과를 타임아웃(`timeout`/`ErrControlTimeout`)으로 명시적으로 표시한다 — 소리 없는 실패가 아니다 (REQ-FACDASH-001-03-02, UB-002)

### Scenario: 전원 OFF에서 풍량 제어 UX 의존성이 반영된다
- **Given** 대상 기기의 전원이 OFF일 때
- **When** 기기 패널이 렌더링되면
- **Then** 풍량 컨트롤이 비활성화되거나, 실행 시 `ErrPowerOff` 거부 결과가 명시적으로 안내된다 (REQ-FACDASH-001-03-03, UB-005)

## Feature: 패널 등록/와이어링 및 영속화

### Scenario: 신규 패널 타입 3종을 대시보드에 추가할 수 있다
- **Given** 대시보드 페이지가 열려 있고
- **When** 사용자가 "패널 추가" 다이얼로그를 열면
- **Then** `facility-line`·`facility-station`·`facility-device` 3종이 목록에 표시되고 (REQ-FACDASH-001-04-02)
- **And** 라인/역사는 대상(line/station) 선택 스텝, 기기는 기기 선택 스텝을 거쳐 패널이 추가된다

### Scenario: 추가한 패널이 저장 후 새로고침에도 복원된다
- **Given** 사용자가 3종 패널을 추가하고 배치·설정을 마쳤을 때
- **When** 대시보드 구성이 저장되고 (SPEC-DASHBOARD-001 영속 흐름) 페이지를 새로고침하면
- **Then** 3종 패널이 `type` + `config`(agentId/line/station/deviceId/표시옵션) 그대로 복원되고 (REQ-FACDASH-001-04-05)
- **And** 스냅샷 스키마에 신규 최상위 필드가 추가되지 않는다 (UB-003)

### Scenario: 신규 패널 렌더가 타입별로 디스패치된다
- **Given** 저장된 대시보드에 3종 패널이 포함되어 있을 때
- **When** 대시보드가 렌더링되면
- **Then** `renderDashboardPanel`이 각 `type`에 대해 해당 컴포넌트로 디스패치한다 (REQ-FACDASH-001-04-03)

## Feature: 데이터/집계

### Scenario: 미등록 station의 기기는 line 집계에서 제외되고 별도 표기된다
- **Given** 로스터에 레지스트리에 없는 station을 참조하는 기기 2대가 있을 때
- **When** 라인 통계가 집계되면
- **Then** 그 2대는 line 집계에서 제외되고 "미분류 2대"로 표기된다 (REQ-FACDASH-001-05-03, UB-004)

### Scenario: refresh 주기에 상태/통계가 갱신된다
- **Given** 시설물 패널이 표시되어 있을 때
- **When** 대시보드 refresh 주기가 도래하거나 사용자가 수동 새로고침하면
- **Then** `list_devices`/`list_stations`가 재조회되어 상태·통계가 갱신된다 (REQ-FACDASH-001-05-04)

## Definition of Done

- [ ] REQ-FACDASH-001-01-*~06-* 전 요구사항이 대응 테스트로 검증됨
- [ ] 3종 패널이 기존 대시보드 패널 시스템에 등록/렌더/설정/영속됨(4-지점 + 스냅샷 정합)
- [ ] 라인/역사 일괄 제어가 AIRPURIFIER-001 셀렉터 fan-out을 호출하고 멤버별 응답 대기 집계를 표시함
- [ ] 개별 제어의 응답 대기 성공/타임아웃이 사용자에게 명시적으로 표시됨(소리 없는 실패 없음)
- [ ] 빈/미등록 station·라인, 미분류 기기가 크래시 없이 안전 degrade됨
- [ ] station→line 매핑·fan-out·응답 판정을 자체 재구현하지 않고 AIRPURIFIER-001 표면만 소비함(UB-001)
- [ ] i18n 키가 ko/en에 추가되고 하드코딩 텍스트가 없음
- [ ] Vitest + RTL 테스트 통과, TRUST 5 품질 게이트 충족
