---
id: SPEC-WEB-001
type: acceptance
version: "1.0.0"
status: draft
created: "2026-03-07"
updated: "2026-03-07"
author: xtra
---

# SPEC-WEB-001: 인수 테스트 기준 - 에이전트 통계 버그 수정 및 디버깅 레벨 설정

---

## 1. Module 1: 에이전트 목록 통계 버그 수정 (P0)

### AC-WEB-001-01: 에이전트 목록 API 호출 파라미터

```gherkin
기능: 에이전트 목록 API에 detail=summary 파라미터 전달

  시나리오: 에이전트 목록 페이지 로드 시 detail 파라미터 포함
    주어진 사용자가 인증된 상태일 때
    만약 에이전트 목록 페이지(/agents)에 접근하면
    그러면 GET /api/v1/agents 요청에 detail=summary 쿼리 파라미터가 포함되어야 한다
    그리고 API 응답에 각 에이전트의 stats, uptime, health 필드가 포함되어야 한다

  시나리오: 페이지네이션과 detail 파라미터 공존
    주어진 에이전트가 20개 이상 존재할 때
    만약 에이전트 목록의 2페이지에 접근하면
    그러면 GET /api/v1/agents?page=2&detail=summary 형태로 요청이 전송되어야 한다
    그리고 페이지네이션과 detail 파라미터가 모두 적용되어야 한다
```

### AC-WEB-001-02: 에이전트 통계 데이터 렌더링

```gherkin
기능: 에이전트 테이블에서 통계 데이터 표시

  시나리오: 통계 데이터가 있는 에이전트 표시
    주어진 에이전트 "modbus-001"이 running 상태이고 stats 데이터가 있을 때
    만약 에이전트 목록 페이지가 로드되면
    그러면 "modbus-001" 행의 "업타임" 컬럼에 uptime 값이 표시되어야 한다
    그리고 "메시지 (IN/OUT)" 컬럼에 "1,234 / 5,678" 형태로 messages_in/messages_out이 표시되어야 한다

  시나리오: 통계 데이터가 없는 에이전트 표시
    주어진 에이전트 "http-002"가 stopped 상태이고 stats 데이터가 null일 때
    만약 에이전트 목록 페이지가 로드되면
    그러면 "http-002" 행의 "업타임" 컬럼에 "-"가 표시되어야 한다
    그리고 "메시지 (IN/OUT)" 컬럼에 "-"가 표시되어야 한다

  시나리오: 혼합 상태 에이전트 목록 표시
    주어진 running 에이전트 3개와 stopped 에이전트 2개가 있을 때
    만약 에이전트 목록 페이지가 로드되면
    그러면 running 에이전트에는 실제 통계 값이 표시되어야 한다
    그리고 stopped 에이전트에는 "-" 기본값이 표시되어야 한다
```

---

## 2. Module 2: 백엔드 컴포넌트별 로그 레벨 API (P1)

### AC-WEB-001-03: 컴포넌트별 로그 레벨 설정

```gherkin
기능: PUT /monitor/loglevel/{component} 엔드포인트

  시나리오: 에이전트 로그 레벨 설정
    주어진 "agent.modbus-001" 컴포넌트가 기본 레벨(info)인 상태일 때
    만약 PUT /monitor/loglevel/agent.modbus-001 요청을 body {"level": "debug"}로 전송하면
    그러면 200 OK 응답을 반환해야 한다
    그리고 응답 body에 {"component": "agent.modbus-001", "level": "debug"}가 포함되어야 한다
    그리고 해당 컴포넌트의 로그 레벨이 debug로 변경되어야 한다

  시나리오: 노드 로그 레벨 설정
    주어진 "node.transform-1" 컴포넌트가 존재할 때
    만약 PUT /monitor/loglevel/node.transform-1 요청을 body {"level": "warn"}로 전송하면
    그러면 200 OK 응답을 반환해야 한다
    그리고 해당 노드의 로그 레벨이 warn으로 변경되어야 한다

  시나리오: 유효하지 않은 로그 레벨 거부
    주어진 임의의 컴포넌트가 존재할 때
    만약 PUT /monitor/loglevel/agent.mqtt-001 요청을 body {"level": "verbose"}로 전송하면
    그러면 400 Bad Request 응답을 반환해야 한다
    그리고 에러 메시지에 유효한 레벨 값(debug, info, warn, error)이 안내되어야 한다

  시나리오: 빈 body 거부
    만약 PUT /monitor/loglevel/agent.mqtt-001 요청을 빈 body로 전송하면
    그러면 400 Bad Request 응답을 반환해야 한다
```

### AC-WEB-001-04: 전체 로그 레벨 조회

```gherkin
기능: GET /monitor/loglevel 엔드포인트

  시나리오: 컴포넌트별 오버라이드가 없는 상태에서 조회
    주어진 글로벌 기본 레벨이 "info"이고 컴포넌트별 오버라이드가 없을 때
    만약 GET /monitor/loglevel 요청을 전송하면
    그러면 200 OK 응답을 반환해야 한다
    그리고 응답에 default_level이 "info"여야 한다
    그리고 응답의 components 맵이 비어 있거나 기본 레벨만 포함해야 한다

  시나리오: 컴포넌트별 오버라이드가 있는 상태에서 조회
    주어진 "agent.modbus-001"이 debug, "node.filter-1"이 error로 오버라이드된 상태일 때
    만약 GET /monitor/loglevel 요청을 전송하면
    그러면 200 OK 응답을 반환해야 한다
    그리고 응답 components에 "agent.modbus-001": "debug"가 포함되어야 한다
    그리고 응답 components에 "node.filter-1": "error"가 포함되어야 한다
```

### AC-WEB-001-05: 컴포넌트 로그 레벨 리셋

```gherkin
기능: DELETE /monitor/loglevel/{component} 엔드포인트

  시나리오: 오버라이드된 컴포넌트 리셋
    주어진 "agent.modbus-001"이 debug로 오버라이드된 상태일 때
    만약 DELETE /monitor/loglevel/agent.modbus-001 요청을 전송하면
    그러면 200 OK 응답을 반환해야 한다
    그리고 "agent.modbus-001"의 로그 레벨이 글로벌 기본 레벨로 복원되어야 한다
    그리고 GET /monitor/loglevel 응답의 components에서 해당 항목이 제거되어야 한다

  시나리오: 오버라이드되지 않은 컴포넌트 리셋
    주어진 "agent.http-002"가 오버라이드되지 않은 상태일 때
    만약 DELETE /monitor/loglevel/agent.http-002 요청을 전송하면
    그러면 200 OK 응답을 반환해야 한다
    그리고 기존 동작에 영향이 없어야 한다
```

### AC-WEB-001-06: 글로벌 로그 레벨 하위 호환성

```gherkin
기능: 기존 PUT /monitor/loglevel 글로벌 API 유지

  시나리오: 글로벌 로그 레벨 변경 (기존 동작 유지)
    주어진 글로벌 기본 레벨이 "info"인 상태일 때
    만약 PUT /monitor/loglevel 요청을 body {"level": "debug"}로 전송하면
    그러면 200 OK 응답을 반환해야 한다
    그리고 글로벌 기본 레벨이 "debug"로 변경되어야 한다
    그리고 기존 API 형식과 동일한 응답을 반환해야 한다

  시나리오: 글로벌 레벨 변경이 컴포넌트 오버라이드에 영향 없음
    주어진 "agent.modbus-001"이 debug로 오버라이드되고 글로벌 레벨이 info인 상태일 때
    만약 PUT /monitor/loglevel 요청으로 글로벌 레벨을 warn으로 변경하면
    그러면 "agent.modbus-001"의 로그 레벨은 debug로 유지되어야 한다
    그리고 오버라이드가 없는 다른 컴포넌트는 warn으로 변경되어야 한다
```

---

## 3. Module 3: 프론트엔드 컴포넌트별 로그 레벨 UI (P1)

### AC-WEB-001-07: 에이전트 상세 패널 로그 레벨 컨트롤

```gherkin
기능: 에이전트 상세 패널에서 로그 레벨 변경

  시나리오: 로그 레벨 드롭다운 표시
    주어진 에이전트 "modbus-001"의 현재 로그 레벨이 "info"일 때
    만약 에이전트 행을 클릭하여 상세 패널을 열면
    그러면 통계 탭에 로그 레벨 드롭다운이 표시되어야 한다
    그리고 드롭다운 옵션에 "기본값", "DEBUG", "INFO", "WARN", "ERROR"가 포함되어야 한다

  시나리오: 에이전트 로그 레벨 변경
    주어진 에이전트 상세 패널이 열린 상태이고 로그 레벨이 "info"일 때
    만약 로그 레벨 드롭다운에서 "DEBUG"를 선택하면
    그러면 PUT /monitor/loglevel/agent.modbus-001 요청이 body {"level": "debug"}로 전송되어야 한다
    그리고 성공 시 "로그 레벨이 변경되었습니다" 토스트가 표시되어야 한다
    그리고 드롭다운 값이 "DEBUG"로 업데이트되어야 한다

  시나리오: 에이전트 로그 레벨을 기본값으로 리셋
    주어진 에이전트 로그 레벨이 "debug"로 오버라이드된 상태일 때
    만약 로그 레벨 드롭다운에서 "기본값"을 선택하면
    그러면 DELETE /monitor/loglevel/agent.modbus-001 요청이 전송되어야 한다
    그리고 성공 시 "로그 레벨이 기본값으로 리셋되었습니다" 토스트가 표시되어야 한다

  시나리오: 로그 레벨 변경 실패 처리
    주어진 에이전트 상세 패널이 열린 상태일 때
    만약 로그 레벨 변경 API가 서버 오류를 반환하면
    그러면 "로그 레벨 변경에 실패했습니다" 에러 토스트가 표시되어야 한다
    그리고 드롭다운 값이 이전 레벨로 유지되어야 한다
```

### AC-WEB-001-08: 설정 페이지 컴포넌트별 오버라이드 관리

```gherkin
기능: 설정 페이지에서 컴포넌트별 로그 레벨 오버라이드 관리

  시나리오: 컴포넌트별 오버라이드 목록 로드
    주어진 "agent.modbus-001"이 debug, "node.filter-1"이 error로 오버라이드된 상태일 때
    만약 설정 페이지의 시스템 탭에 접근하면
    그러면 GET /monitor/loglevel API가 호출되어야 한다
    그리고 글로벌 로그 레벨 카드 아래에 "컴포넌트별 로그 레벨" 섹션이 표시되어야 한다
    그리고 테이블에 "agent.modbus-001 | DEBUG | 리셋" 행이 표시되어야 한다
    그리고 테이블에 "node.filter-1 | ERROR | 리셋" 행이 표시되어야 한다

  시나리오: 오버라이드가 없는 경우 빈 상태 표시
    주어진 컴포넌트별 오버라이드가 없는 상태일 때
    만약 설정 페이지의 시스템 탭에 접근하면
    그러면 "컴포넌트별 로그 레벨 오버라이드가 없습니다" 안내 메시지가 표시되어야 한다

  시나리오: 오버라이드 리셋
    주어진 "agent.modbus-001"이 debug로 오버라이드된 상태이고 목록에 표시될 때
    만약 해당 행의 "리셋" 버튼을 클릭하면
    그러면 DELETE /monitor/loglevel/agent.modbus-001 요청이 전송되어야 한다
    그리고 성공 시 해당 행이 목록에서 제거되어야 한다
    그리고 "로그 레벨이 기본값으로 리셋되었습니다" 토스트가 표시되어야 한다
```

### AC-WEB-001-09: RBAC 접근 제어

```gherkin
기능: 역할에 따른 로그 레벨 제어 접근 제한

  시나리오: Admin 역할의 로그 레벨 제어
    주어진 Admin 역할의 사용자가 로그인한 상태일 때
    만약 에이전트 상세 패널의 로그 레벨 드롭다운에 접근하면
    그러면 드롭다운이 활성화(enabled)되어야 한다
    그리고 값을 변경할 수 있어야 한다

  시나리오: Viewer 역할의 로그 레벨 제어 제한
    주어진 Viewer 역할의 사용자가 로그인한 상태일 때
    만약 에이전트 상세 패널의 로그 레벨 드롭다운에 접근하면
    그러면 드롭다운이 비활성화(disabled)되어야 한다
    만약 설정 페이지의 시스템 탭에서 오버라이드 목록을 볼 때
    그러면 리셋 버튼이 비활성화(disabled)되어야 한다
```

---

## 4. 비기능 요구사항

### AC-WEB-001-10: API 성능

```gherkin
기능: API 응답 성능

  시나리오: detail=summary 응답 시간
    주어진 에이전트가 50개 등록된 상태일 때
    만약 GET /api/v1/agents?detail=summary 요청을 전송하면
    그러면 응답 시간이 500ms 이내여야 한다

  시나리오: 컴포넌트 로그 레벨 조회 응답 시간
    주어진 100개의 컴포넌트 오버라이드가 등록된 상태일 때
    만약 GET /monitor/loglevel 요청을 전송하면
    그러면 응답 시간이 100ms 이내여야 한다
```

### AC-WEB-001-11: 에러 복원력

```gherkin
기능: API 오류 시 UI 복원력

  시나리오: 로그 레벨 API 오류 시 에이전트 목록 정상 동작
    주어진 /monitor/loglevel API가 일시적으로 오류를 반환할 때
    만약 에이전트 상세 패널을 열면
    그러면 통계 데이터는 정상적으로 표시되어야 한다
    그리고 로그 레벨 드롭다운에 "불러오기 실패" 또는 기본값이 표시되어야 한다
    그리고 전체 패널이 오류 상태로 전환되지 않아야 한다

  시나리오: detail=summary API 오류 시 기본 에이전트 목록 표시
    주어진 detail=summary 파라미터가 서버에서 미지원인 경우
    만약 에이전트 목록 페이지가 로드되면
    그러면 기본 에이전트 정보(이름, 타입, 상태)는 정상 표시되어야 한다
    그리고 통계 컬럼에 "-" 기본값이 표시되어야 한다
```

---

## 5. Quality Gate 체크리스트

- [ ] Module 1: `agentService.ts`의 `getAgents` 호출에 `detail=summary` 파라미터가 추가됨
- [ ] Module 1: 에이전트 목록 테이블에서 stats 데이터가 올바르게 렌더링됨
- [ ] Module 2: `GET /monitor/loglevel` 엔드포인트가 전체 레벨 맵을 반환함
- [ ] Module 2: `PUT /monitor/loglevel/{component}` 엔드포인트가 개별 레벨을 설정함
- [ ] Module 2: `DELETE /monitor/loglevel/{component}` 엔드포인트가 오버라이드를 리셋함
- [ ] Module 2: 유효하지 않은 레벨 값에 대해 400 응답을 반환함
- [ ] Module 2: 기존 `PUT /monitor/loglevel` 글로벌 API가 정상 동작함
- [ ] Module 2: 백엔드 테스트 커버리지 85% 이상
- [ ] Module 3: 에이전트 상세 패널에 로그 레벨 드롭다운이 표시됨
- [ ] Module 3: 드롭다운 변경 시 올바른 API가 호출됨
- [ ] Module 3: 설정 페이지에 컴포넌트별 오버라이드 목록이 표시됨
- [ ] Module 3: Viewer 역할에서 변경 컨트롤이 비활성화됨
- [ ] TypeScript strict 모드에서 타입 에러 0건
- [ ] ESLint 경고 0건

---

## 6. Definition of Done

- [ ] Module 1 (P0): 에이전트 목록 페이지에서 stats 데이터가 정상 표시됨
- [ ] Module 2 (P1): 3개 API 엔드포인트가 구현되고 테스트 통과
- [ ] Module 3 (P1): 에이전트 상세 패널 + 설정 페이지 UI 구현 완료
- [ ] 기존 글로벌 로그 레벨 기능이 정상 동작 (회귀 없음)
- [ ] RBAC 접근 제어가 적용됨 (Admin: 전체 제어, Viewer: 조회만)
- [ ] 모든 신규 백엔드 핸들러에 단위 테스트 존재
- [ ] 프론트엔드 TypeScript 컴파일 에러 없음
- [ ] ESLint 경고 없음

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.0.0*
*상태: draft*
*최종 수정: 2026-03-07*
