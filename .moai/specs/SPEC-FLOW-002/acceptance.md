---
id: SPEC-FLOW-002
type: acceptance
version: "1.0.0"
created: "2026-04-08"
updated: "2026-04-08"
---

# SPEC-FLOW-002 인수 기준

## Module 1: 동일 타입 에이전트 조회

### AC-FLOW-002-01: 동일 타입 에이전트 필터링

```gherkin
Scenario: 누락 에이전트와 동일 타입의 기존 에이전트가 존재
  Given 서버에 다음 에이전트가 등록되어 있을 때:
    | name          | type  |
    | lg-hvacr02-sensor-2 | lg_hvacr02 |
    | lg-hvacr02-sensor-3 | lg_hvacr02 |
    | mqtt-broker-1 | mqtt  |
  And 가져오기 플로우에 "lg-hvacr02-sensor-1" (type: lg_hvacr02)이 참조되어 있을 때
  When 플로우 파일을 가져오기하면
  Then "lg-hvacr02-sensor-1"은 누락 에이전트로 표시되어야 한다
  And 동일 타입 에이전트 목록에 "lg-hvacr02-sensor-2", "lg-hvacr02-sensor-3"이 표시되어야 한다
  And "mqtt-broker-1"은 동일 타입 목록에 포함되지 않아야 한다
```

### AC-FLOW-002-02: 타입 정보 없는 에이전트 처리

```gherkin
Scenario: 타입 정보가 없는 누락 에이전트
  Given 가져오기 플로우에 "unknown-agent" (type 없음)이 참조되어 있을 때
  When 플로우 파일을 가져오기하면
  Then "unknown-agent"은 누락 에이전트로 표시되어야 한다
  And 대체 선택 드롭다운이 표시되지 않아야 한다
  And "타입 정보 없음 - 수동 생성 필요" 메시지가 표시되어야 한다
```

### AC-FLOW-002-03: 동일 타입 에이전트가 없는 경우

```gherkin
Scenario: 동일 타입의 기존 에이전트가 없음
  Given 서버에 "lg_hvacr02" 타입 에이전트가 없을 때
  And 가져오기 플로우에 "lg-hvacr02-sensor-1" (type: lg_hvacr02)이 참조되어 있을 때
  When 플로우 파일을 가져오기하면
  Then "lg-hvacr02-sensor-1"은 누락 에이전트로 표시되어야 한다
  And "새로 생성" 옵션만 표시되어야 한다 (기존 동작)
  And 대체 선택 드롭다운에 기존 에이전트 옵션이 없어야 한다
```

---

## Module 2: 대체 에이전트 선택 UI

### AC-FLOW-002-04: 드롭다운 옵션 표시

```gherkin
Scenario: 동일 타입 에이전트가 있을 때 드롭다운 옵션
  Given 누락 에이전트 "lg-hvacr02-sensor-1" (type: lg_hvacr02)이 표시되어 있을 때
  And 서버에 "lg-hvacr02-sensor-2", "lg-hvacr02-sensor-3" (type: lg_hvacr02)이 존재할 때
  When 드롭다운을 클릭하면
  Then 다음 옵션이 표시되어야 한다:
    | 옵션 |
    | 새로 생성 |
    | lg-hvacr02-sensor-2 |
    | lg-hvacr02-sensor-3 |
    | 건너뛰기 |
```

### AC-FLOW-002-05: 기존 에이전트 선택 시 상태 변경

```gherkin
Scenario: 드롭다운에서 기존 에이전트 선택
  Given 누락 에이전트 "lg-hvacr02-sensor-1"의 드롭다운이 표시되어 있을 때
  When "lg-hvacr02-sensor-2"를 선택하면
  Then 해당 에이전트의 해결 방법이 "substitute"로 변경되어야 한다
  And 선택된 대체 에이전트가 "lg-hvacr02-sensor-2"로 기록되어야 한다
  And UI에 "lg-hvacr02-sensor-1 -> lg-hvacr02-sensor-2" 매핑이 표시되어야 한다
```

### AC-FLOW-002-06: "새로 생성" 선택 시 기존 동작

```gherkin
Scenario: 드롭다운에서 "새로 생성" 선택
  Given 누락 에이전트 "lg-hvacr02-sensor-1"의 드롭다운이 표시되어 있을 때
  When "새로 생성"을 선택하면
  Then 해당 에이전트의 해결 방법이 "create"로 변경되어야 한다
  And 기존과 동일하게 타입 정보가 표시되어야 한다
```

### AC-FLOW-002-07: "건너뛰기" 선택

```gherkin
Scenario: 드롭다운에서 "건너뛰기" 선택
  Given 누락 에이전트 "lg-hvacr02-sensor-1"의 드롭다운이 표시되어 있을 때
  When "건너뛰기"를 선택하면
  Then 해당 에이전트의 해결 방법이 "skip"으로 변경되어야 한다
  And 해당 항목이 비활성화된 스타일로 표시되어야 한다
```

### AC-FLOW-002-08: 혼합 해결 방법 표시

```gherkin
Scenario: 여러 누락 에이전트에 대해 서로 다른 해결 방법 선택
  Given 다음 누락 에이전트가 있을 때:
    | name          | type   |
    | lg-hvacr02-sensor-1 | lg_hvacr02  |
    | mqtt-pub-1    | mqtt   |
    | serial-dev-1  | serial |
  And "lg_hvacr02" 타입에 기존 에이전트 "lg-hvacr02-sensor-2"가 존재할 때
  And "mqtt" 타입에 기존 에이전트가 없을 때
  And "serial" 타입에 기존 에이전트 "serial-dev-2"가 존재할 때
  When 사용자가 다음과 같이 선택하면:
    | name          | 해결 방법          |
    | lg-hvacr02-sensor-1 | lg-hvacr02-sensor-2 대체 |
    | mqtt-pub-1    | 새로 생성          |
    | serial-dev-1  | 건너뛰기           |
  Then 각 항목이 선택한 해결 방법에 맞게 표시되어야 한다
```

---

## Module 3: 에이전트 이름 리매핑

### AC-FLOW-002-09: 단일 노드 에이전트 이름 리매핑

```gherkin
Scenario: 하나의 노드에서 에이전트 이름 치환
  Given 플로우 정의에 다음 노드가 있을 때:
    | node_name    | agent_ref.agent_name |
    | lg_hvacr02-bridge  | lg-hvacr02-sensor-1        |
  And remapTable이 { "lg-hvacr02-sensor-1": "lg-hvacr02-sensor-2" }일 때
  When remapAgentNames(definition, remapTable)을 호출하면
  Then 결과의 nodes[0].agent_ref.agent_name이 "lg-hvacr02-sensor-2"여야 한다
  And 결과의 nodes[0].agent_ref.agent_id가 빈 문자열이어야 한다
```

### AC-FLOW-002-10: 다중 노드 동일 에이전트 리매핑

```gherkin
Scenario: 여러 노드가 같은 에이전트를 참조할 때 일괄 치환
  Given 플로우 정의에 다음 노드가 있을 때:
    | node_name     | agent_ref.agent_name |
    | lg_hvacr02-input    | lg-hvacr02-sensor-1        |
    | lg_hvacr02-output   | lg-hvacr02-sensor-1        |
    | mqtt-bridge   | mqtt-broker-1        |
  And remapTable이 { "lg-hvacr02-sensor-1": "lg-hvacr02-sensor-2" }일 때
  When remapAgentNames(definition, remapTable)을 호출하면
  Then nodes[0].agent_ref.agent_name이 "lg-hvacr02-sensor-2"여야 한다
  And nodes[1].agent_ref.agent_name이 "lg-hvacr02-sensor-2"여야 한다
  And nodes[2].agent_ref.agent_name이 "mqtt-broker-1"이어야 한다 (변경 없음)
```

### AC-FLOW-002-11: 원본 데이터 불변성 보장

```gherkin
Scenario: 리매핑이 원본 데이터를 변경하지 않음
  Given 원본 플로우 정의에 agent_ref.agent_name이 "lg-hvacr02-sensor-1"인 노드가 있을 때
  And remapTable이 { "lg-hvacr02-sensor-1": "lg-hvacr02-sensor-2" }일 때
  When remapAgentNames(definition, remapTable)을 호출하면
  Then 원본 definition의 nodes[0].agent_ref.agent_name은 여전히 "lg-hvacr02-sensor-1"이어야 한다
  And 반환된 결과는 새로운 객체여야 한다
```

### AC-FLOW-002-12: agent_ref가 없는 노드 처리

```gherkin
Scenario: agent_ref가 없는 노드는 건너뜀
  Given 플로우 정의에 다음 노드가 있을 때:
    | node_name   | type   | agent_ref |
    | filter-1    | filter | null      |
    | lg_hvacr02-bridge | bridge | { agent_name: "lg-hvacr02-sensor-1" } |
  And remapTable이 { "lg-hvacr02-sensor-1": "lg-hvacr02-sensor-2" }일 때
  When remapAgentNames(definition, remapTable)을 호출하면
  Then nodes[0]은 변경 없이 유지되어야 한다 (filter 노드)
  And nodes[1].agent_ref.agent_name이 "lg-hvacr02-sensor-2"여야 한다
```

### AC-FLOW-002-13: 빈 remapTable 처리

```gherkin
Scenario: remapTable이 비어있으면 변경 없음
  Given 플로우 정의에 노드가 있을 때
  And remapTable이 {}일 때
  When remapAgentNames(definition, remapTable)을 호출하면
  Then 결과가 원본과 동일한 내용이어야 한다
```

---

## Module 4: 가져오기 실행 통합

### AC-FLOW-002-14: "새로 생성" 선택 시 에이전트 생성 후 플로우 생성

```gherkin
Scenario: "새로 생성"으로 해결된 에이전트 처리
  Given 누락 에이전트 "lg-hvacr02-sensor-1" (type: lg_hvacr02)이 "새로 생성"으로 선택되었을 때
  When 가져오기를 실행하면
  Then createAgent API가 { name: "lg-hvacr02-sensor-1", type: "lg_hvacr02" }로 호출되어야 한다
  And 이후 createFlow API가 원본 definition으로 호출되어야 한다
```

### AC-FLOW-002-15: "기존 에이전트 대체" 선택 시 리매핑 후 플로우 생성

```gherkin
Scenario: 대체 에이전트로 해결된 에이전트 처리
  Given 누락 에이전트 "lg-hvacr02-sensor-1"이 "lg-hvacr02-sensor-2"로 대체 선택되었을 때
  When 가져오기를 실행하면
  Then createAgent API가 호출되지 않아야 한다 (해당 에이전트에 대해)
  And createFlow API가 리매핑된 definition으로 호출되어야 한다
  And 리매핑된 definition 내 "lg-hvacr02-sensor-1" 참조가 "lg-hvacr02-sensor-2"로 변경되어야 한다
```

### AC-FLOW-002-16: "건너뛰기" 선택 시 에이전트 무시

```gherkin
Scenario: "건너뛰기"로 해결된 에이전트 처리
  Given 누락 에이전트 "lg-hvacr02-sensor-1"이 "건너뛰기"로 선택되었을 때
  When 가져오기를 실행하면
  Then createAgent API가 호출되지 않아야 한다 (해당 에이전트에 대해)
  And createFlow API가 원본 definition으로 호출되어야 한다 (리매핑 없음)
```

### AC-FLOW-002-17: 혼합 해결 방법으로 가져오기 실행

```gherkin
Scenario: 여러 해결 방법이 혼합된 가져오기
  Given 다음 누락 에이전트 해결 상태가 있을 때:
    | name          | 해결 방법                  |
    | lg-hvacr02-sensor-1 | lg-hvacr02-sensor-2로 대체       |
    | mqtt-pub-1    | 새로 생성                  |
    | serial-dev-1  | 건너뛰기                   |
  When 가져오기를 실행하면
  Then createAgent API가 { name: "mqtt-pub-1", type: "mqtt" }로 1회 호출되어야 한다
  And createFlow API가 호출되어야 한다
  And 전송된 definition에서 "lg-hvacr02-sensor-1" 참조는 "lg-hvacr02-sensor-2"로 변경되어야 한다
  And 전송된 definition에서 "mqtt-pub-1" 참조는 변경되지 않아야 한다
  And 전송된 definition에서 "serial-dev-1" 참조는 변경되지 않아야 한다
```

### AC-FLOW-002-18: 가져오기 성공 후 모달 닫기

```gherkin
Scenario: 성공적인 가져오기 후 UI 처리
  Given 모든 누락 에이전트가 해결된 상태에서
  When 가져오기를 실행하고 모든 API 호출이 성공하면
  Then 성공 메시지가 표시되어야 한다
  And onImportSuccess 콜백이 호출되어야 한다
  And 모달이 닫혀야 한다
```

---

## 기존 동작 회귀 테스트

### AC-FLOW-002-19: 누락 에이전트 없이 플로우 가져오기 (기존 동작 보존)

```gherkin
Scenario: 모든 에이전트가 존재하는 플로우 가져오기
  Given 플로우가 참조하는 모든 에이전트가 서버에 존재할 때
  When 플로우 파일을 가져오기하면
  Then 누락 에이전트 섹션이 표시되지 않아야 한다
  And 즉시 플로우 가져오기가 가능해야 한다
```

### AC-FLOW-002-20: 에이전트 파일 드래그 앤 드롭 (기존 동작 보존)

```gherkin
Scenario: 에이전트 파일 드래그 앤 드롭으로 누락 에이전트 보완
  Given 누락 에이전트 "lg-hvacr02-sensor-1"이 표시되어 있을 때
  When "lg-hvacr02-sensor-1" 에이전트 정의 파일을 드래그 앤 드롭하면
  Then 해당 에이전트의 타입과 설정이 보완되어야 한다
  And 해당 에이전트의 해결 방법이 "새로 생성"으로 설정되어야 한다
```

---

## Quality Gate

### Definition of Done

- [ ] Module 1-4의 모든 요구사항이 구현됨
- [ ] 모든 acceptance criteria 시나리오가 검증됨
- [ ] `remapAgentNames` 함수에 대한 단위 테스트 작성 (importParser.test.ts)
- [ ] 기존 가져오기 동작에 대한 회귀 테스트 통과
- [ ] TypeScript 타입 체크 오류 없음 (`npx tsc --noEmit`)
- [ ] ESLint 경고 없음 (`npx eslint`)
- [ ] 다음 시나리오에 대한 수동 검증:
  - 동일 타입 에이전트가 있는 경우 드롭다운 표시
  - 동일 타입 에이전트가 없는 경우 기존 UI 유지
  - 타입 정보 없는 에이전트의 기존 동작 유지
  - 대체 선택 후 가져오기 실행 시 리매핑 정상 동작
  - 혼합 해결 방법 (생성 + 대체 + 건너뛰기) 정상 동작
  - 에이전트 파일 드래그 앤 드롭 기존 동작 유지
