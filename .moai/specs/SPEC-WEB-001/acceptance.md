---
id: SPEC-WEB-001
type: acceptance
version: "1.3.0"
status: completed
created: "2026-03-07"
updated: "2026-03-08"
author: xtra
---

# SPEC-WEB-001: 인수 테스트 기준 - 에이전트/노드 통계 및 디버깅 레벨 설정

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

## 4. Module 4: 플로우 노드 통계 및 로그 레벨 UI (P1)

### AC-WEB-001-12: 플로우 확장 패널 노드 목록

```gherkin
기능: 플로우 목록에서 노드 인스턴스 상세 표시

  시나리오: 플로우 행 확장으로 노드 목록 표시
    주어진 플로우 "modbus-flow"가 running 상태이고 3개 노드가 있을 때
    만약 플로우 행을 클릭하면
    그러면 확장 패널에 노드 인스턴스 테이블이 표시되어야 한다
    그리고 각 노드의 이름, 타입, 상태, In/Out 메시지 수(아이콘 분리)가 표시되어야 한다

  시나리오: 플로우 행 확장 토글
    주어진 플로우 확장 패널이 열린 상태일 때
    만약 같은 플로우 행을 다시 클릭하면
    그러면 확장 패널이 닫혀야 한다

  시나리오: 배포되지 않은 플로우의 노드 표시
    주어진 플로우 "test-flow"가 stored 상태이고 노드 정보가 없을 때
    만약 플로우 행을 클릭하면
    그러면 "노드 정보가 없습니다. 플로우를 배포하면 노드가 표시됩니다." 안내 메시지가 표시되어야 한다

  시나리오: 플로우명 클릭 시 에디터 이동
    주어진 플로우 목록이 표시된 상태일 때
    만약 플로우 이름 링크를 클릭하면
    그러면 에디터 페이지(/editor/{flowId})로 이동해야 한다
    그리고 확장 패널 토글이 발생하지 않아야 한다
```

### AC-WEB-001-13: 노드별 로그 레벨 제어

```gherkin
기능: 플로우 확장 패널에서 노드 로그 레벨 변경

  시나리오: 노드 로그 레벨 드롭다운 표시
    주어진 플로우 확장 패널이 열린 상태이고 노드 "modbus-reader"가 있을 때
    그러면 해당 노드 행에 로그 레벨 드롭다운이 표시되어야 한다
    그리고 드롭다운 옵션에 "기본값", "DEBUG", "INFO", "WARN", "ERROR"가 포함되어야 한다

  시나리오: 노드 로그 레벨 변경
    주어진 노드 "modbus-reader"의 로그 레벨 드롭다운에서 "DEBUG"를 선택하면
    그러면 PUT /monitor/loglevel/node.modbus-reader 요청이 body {"level": "debug"}로 전송되어야 한다
    그리고 성공 시 토스트 알림이 표시되어야 한다

  시나리오: 노드 로그 레벨을 기본값으로 리셋
    주어진 노드 "modbus-reader"의 로그 레벨이 "debug"로 오버라이드된 상태일 때
    만약 로그 레벨 드롭다운에서 "기본값"을 선택하면
    그러면 DELETE /monitor/loglevel/node.modbus-reader 요청이 전송되어야 한다
    그리고 성공 시 "기본값으로 리셋되었습니다" 토스트가 표시되어야 한다
```

### AC-WEB-001-14: 플로우 노드 통계 In/Out 분리 표시

```gherkin
기능: 플로우 확장 패널에서 In/Out 메시지 수 분리 표시

  시나리오: In/Out 메시지 수 아이콘 분리 표시
    주어진 노드 "sensor-mapper"에 input 포트 메시지 100건, output 포트 메시지 80건이 있을 때
    만약 플로우 확장 패널이 열리면
    그러면 "In / Out" 컬럼 헤더가 표시되어야 한다
    그리고 ArrowDownToLine 아이콘과 함께 입력 메시지 수 "100"이 표시되어야 한다
    그리고 ArrowUpFromLine 아이콘과 함께 출력 메시지 수 "80"이 표시되어야 한다

  시나리오: 포트가 없는 노드의 In/Out 표시
    주어진 노드에 포트 정보가 없을 때
    만약 플로우 확장 패널이 열리면
    그러면 In/Out 컬럼에 아무것도 표시되지 않아야 한다
```

### AC-WEB-001-15: 플로우 노드 데이터 실시간 갱신

```gherkin
기능: running 플로우의 노드 데이터 자동 갱신

  시나리오: running 플로우의 실시간 갱신
    주어진 플로우가 running 상태이고 확장 패널이 열린 상태일 때
    그러면 노드 인스턴스 데이터가 3초 간격으로 자동 갱신되어야 한다
    그리고 In/Out 메시지 수가 실시간으로 업데이트되어야 한다

  시나리오: stopped 플로우의 갱신 비활성화
    주어진 플로우가 stopped 상태이고 확장 패널이 열린 상태일 때
    그러면 노드 인스턴스 데이터 자동 갱신이 비활성화되어야 한다
    그리고 마지막 조회 데이터가 정적으로 표시되어야 한다
```

---

## 5. Module 5: 캔버스 런타임 통계 (P1)

### AC-WEB-001-16: 캔버스 CustomNode 런타임 상태 표시

```gherkin
기능: React Flow 캔버스에서 노드별 런타임 상태 표시

  시나리오: running 플로우의 노드 상태 표시등
    주어진 에디터에서 running 플로우가 열려 있고 노드 "sensor-mapper"가 running 상태일 때
    그러면 해당 CustomNode에 초록색 상태 표시등이 표시되어야 한다
    그리고 노드 라벨 아래에 In/Out 메시지 수가 아이콘과 함께 표시되어야 한다

  시나리오: error 상태 노드의 표시
    주어진 노드 "data-writer"가 error 상태일 때
    그러면 해당 CustomNode에 빨간색 상태 표시등이 표시되어야 한다
    그리고 In/Out 메시지 수가 계속 표시되어야 한다

  시나리오: stopped 플로우의 런타임 표시 숨김
    주어진 에디터에서 stopped 플로우가 열려 있을 때
    그러면 CustomNode에 런타임 통계가 표시되지 않아야 한다
    그리고 기본 노드 표시(라벨, 포트)만 보여야 한다
```

### AC-WEB-001-17: 런타임 통계 데이터 격리

```gherkin
기능: 런타임 통계가 에디터 상태에 영향을 주지 않음

  시나리오: 런타임 통계 갱신이 isDirty를 변경하지 않음
    주어진 에디터에서 running 플로우가 열려 있고 변경사항이 없는 상태일 때
    만약 런타임 통계가 3초마다 갱신되면
    그러면 isDirty 플래그가 false로 유지되어야 한다
    그리고 undo/redo 이력에 런타임 통계 업데이트가 추가되지 않아야 한다

  시나리오: RuntimeStatsContext를 통한 데이터 전달
    주어진 EditorPage에서 RuntimeStatsContext.Provider가 설정된 상태일 때
    만약 CustomNode에서 useNodeRuntimeStats(nodeId)를 호출하면
    그러면 해당 노드의 런타임 통계(inMessages, outMessages, state)를 반환해야 한다
    그리고 editorStore의 노드 데이터에는 런타임 정보가 포함되지 않아야 한다
```

---

## 6. 백엔드 버그 수정 (P0)

### AC-WEB-001-18: 포트 카운터 초기화

```gherkin
기능: 엔진 포트 카운터가 모든 포트에 대해 올바르게 초기화됨

  시나리오: 모든 포트 방향의 카운터 초기화
    주어진 노드에 input 포트 2개, output 포트 1개, error 포트 1개가 있을 때
    만약 엔진이 노드를 초기화하면
    그러면 모든 4개 포트의 메시지 카운터가 0으로 초기화되어야 한다
    그리고 메시지 처리 후 각 포트의 카운터가 정확히 증가해야 한다

  시나리오: 메시지 카운트 정확성
    주어진 노드가 running 상태이고 input 포트에 10개 메시지가 도착했을 때
    만약 GET /flows/{id}/nodes API를 호출하면
    그러면 해당 input 포트의 messages 값이 10이어야 한다
    그리고 output 포트의 messages 값도 실제 처리 수와 일치해야 한다
```

### AC-WEB-001-19: 브릿지 노드 버그 수정

```gherkin
기능: Bridge 노드의 메시지 처리 정상 동작

  시나리오: BridgeIn 노드 메시지 수신
    주어진 BridgeIn 노드가 초기화된 상태일 때
    만약 외부에서 메시지가 전달되면
    그러면 msgCh 채널을 통해 메시지가 정상적으로 수신되어야 한다
    그리고 nil 패닉이 발생하지 않아야 한다

  시나리오: BridgeOut 노드 Process 반환값
    주어진 BridgeOut 노드가 메시지를 처리할 때
    만약 정상적으로 메시지를 전달하면
    그러면 Process() 메서드가 nil을 반환해야 한다
    그리고 메시지 처리 파이프라인이 중단되지 않아야 한다
```

---

## 7. Module 7: 로그 뷰어 컴포넌트/소스 필터링 (P1)

### AC-WEB-001-22: LogEntry 인터페이스 확장 및 handleLog 페이로드 추출

```gherkin
기능: WebSocket log.entry 메시지에서 component/source 필드 추출

  시나리오: component와 source가 포함된 log.entry 수신
    주어진 WebSocket 연결이 활성화된 상태이고
    그리고 백엔드에서 log.entry 메시지를 전송할 때
      | level | message          | timestamp            | component            | source |
      | INFO  | 읽기 완료        | 2026-03-08T10:00:00Z | agent.modbus-reader  | agent  |
    만약 MonitoringPage의 handleLog가 해당 메시지를 수신하면
    그러면 LogEntry 객체에 component 값 "agent.modbus-reader"가 포함되어야 한다
    그리고 LogEntry 객체에 source 값 "agent"가 포함되어야 한다

  시나리오: component/source가 누락된 log.entry 수신 (하위 호환)
    주어진 WebSocket 연결이 활성화된 상태이고
    그리고 백엔드에서 component/source 필드가 없는 log.entry 메시지를 전송할 때
      | level | message    | timestamp            |
      | WARN  | 연결 실패  | 2026-03-08T10:01:00Z |
    만약 MonitoringPage의 handleLog가 해당 메시지를 수신하면
    그러면 LogEntry 객체의 component는 undefined여야 한다
    그리고 LogEntry 객체의 source는 undefined여야 한다
    그리고 기존 level, message, timestamp 필드는 정상 처리되어야 한다
```

### AC-WEB-001-23: 로그 행에 소스 배지 및 컴포넌트 이름 표시

```gherkin
기능: 로그 뷰어 각 행에 source 배지와 component 이름 표시

  시나리오: source별 색상 배지 표시
    주어진 다음 로그 항목들이 수신된 상태일 때
      | source  | component           |
      | agent   | agent.modbus-reader |
      | node    | node.transform-1    |
      | flow    | flow.data-pipeline  |
      | api     | api.agents          |
      | engine  | engine.scheduler    |
      | system  | xflowd              |
    만약 로그 탭을 확인하면
    그러면 각 행에 source 값에 해당하는 색상 배지가 표시되어야 한다
    그리고 "agent" 배지는 보라색 계열이어야 한다
    그리고 "node" 배지는 틸색 계열이어야 한다
    그리고 "flow" 배지는 초록색 계열이어야 한다
    그리고 "api" 배지는 주황색 계열이어야 한다
    그리고 "engine" 배지는 남색 계열이어야 한다
    그리고 "system" 배지는 회색 계열이어야 한다

  시나리오: component 이름 표시
    주어진 component가 "agent.modbus-reader"인 로그 항목일 때
    만약 로그 탭에서 해당 행을 확인하면
    그러면 source 배지 옆에 컴포넌트 이름 "agent.modbus-reader"가 표시되어야 한다
    그리고 컴포넌트 이름이 140px 영역 내에서 말줄임(truncate) 처리되어야 한다

  시나리오: source/component가 없는 로그 행 표시
    주어진 component와 source가 undefined인 로그 항목일 때
    만약 로그 탭에서 해당 행을 확인하면
    그러면 source 배지 영역이 비어 있어야 한다
    그리고 컴포넌트 이름 영역이 비어 있어야 한다
    그리고 기존 timestamp, level, message 컬럼은 정상 표시되어야 한다
```

### AC-WEB-001-24: 소스 타입 필터 (멀티 셀렉트)

```gherkin
기능: source 유형별 로그 필터링 (멀티 셀렉트 토글)

  시나리오: 초기 상태 - 모든 소스 활성화
    주어진 로그 탭이 열린 상태일 때
    그러면 소스 필터 영역에 6개 토글 버튼이 표시되어야 한다
      | 버튼     |
      | agent    |
      | node     |
      | flow     |
      | api      |
      | engine   |
      | system   |
    그리고 모든 토글이 활성(선택) 상태여야 한다
    그리고 모든 소스의 로그가 표시되어야 한다

  시나리오: 단일 소스 비활성화
    주어진 모든 소스 필터가 활성 상태일 때
    만약 "agent" 토글 버튼을 클릭하면
    그러면 "agent" 토글이 비활성 상태로 변경되어야 한다
    그리고 source가 "agent"인 로그 항목이 목록에서 숨겨져야 한다
    그리고 나머지 5개 소스의 로그는 계속 표시되어야 한다

  시나리오: 복수 소스 비활성화
    주어진 모든 소스 필터가 활성 상태일 때
    만약 "agent" 토글과 "system" 토글을 순차적으로 클릭하면
    그러면 source가 "agent"이거나 "system"인 로그 항목이 숨겨져야 한다
    그리고 node, flow, api, engine 소스의 로그만 표시되어야 한다

  시나리오: 비활성 소스 재활성화
    주어진 "agent" 소스 필터가 비활성 상태일 때
    만약 "agent" 토글 버튼을 다시 클릭하면
    그러면 "agent" 토글이 활성 상태로 변경되어야 한다
    그리고 source가 "agent"인 로그 항목이 다시 목록에 표시되어야 한다

  시나리오: source가 없는 로그 항목의 필터 동작
    주어진 source가 undefined인 로그 항목이 존재할 때
    만약 일부 소스 필터를 비활성화해도
    그러면 source가 undefined인 로그 항목은 항상 표시되어야 한다
```

### AC-WEB-001-25: 컴포넌트 이름 검색 필터

```gherkin
기능: 컴포넌트 이름 텍스트 검색 필터

  시나리오: 검색어 입력으로 필터링
    주어진 다음 로그 항목들이 존재할 때
      | component            |
      | agent.modbus-reader  |
      | agent.modbus-writer  |
      | node.transform-1     |
      | flow.data-pipeline   |
    만약 컴포넌트 검색 입력란에 "modbus"를 입력하면
    그러면 300ms 디바운스 후 필터가 적용되어야 한다
    그리고 component에 "modbus"가 포함된 로그 항목만 표시되어야 한다
    그리고 "node.transform-1"과 "flow.data-pipeline" 항목은 숨겨져야 한다

  시나리오: 대소문자 무시 검색
    주어진 component가 "Agent.Modbus-Reader"인 로그 항목이 존재할 때
    만약 컴포넌트 검색 입력란에 "modbus"를 입력하면
    그러면 해당 로그 항목이 표시되어야 한다 (대소문자 무시)

  시나리오: 검색어 제거로 필터 해제
    주어진 컴포넌트 검색란에 "modbus"가 입력된 상태일 때
    만약 검색란의 텍스트를 모두 삭제하면
    그러면 300ms 디바운스 후 모든 로그 항목이 다시 표시되어야 한다

  시나리오: 디바운스 동작 확인
    주어진 빈 검색란 상태일 때
    만약 "m", "o", "d", "b", "u", "s"를 빠르게 연속 입력하면
    그러면 마지막 입력 후 300ms가 경과할 때까지 필터가 적용되지 않아야 한다
    그리고 300ms 경과 후 "modbus"로 한 번만 필터링이 수행되어야 한다
```

### AC-WEB-001-26: 복합 필터 AND 조합

```gherkin
기능: 레벨 + 소스 + 컴포넌트 검색 필터 AND 조합

  시나리오: 레벨 필터와 소스 필터 AND 조합
    주어진 다음 로그 항목들이 존재할 때
      | level | source | component           |
      | INFO  | agent  | agent.modbus-reader |
      | ERROR | agent  | agent.modbus-writer |
      | INFO  | node   | node.transform-1    |
      | ERROR | node   | node.filter-1       |
    만약 레벨 필터를 "ERROR"로 설정하고
    그리고 소스 필터에서 "agent"만 활성화하면
    그러면 level이 "ERROR"이고 source가 "agent"인 항목만 표시되어야 한다
    그리고 "agent.modbus-writer" 1건만 표시되어야 한다

  시나리오: 3개 필터 모두 적용
    주어진 다음 로그 항목들이 존재할 때
      | level | source | component            |
      | ERROR | agent  | agent.modbus-reader  |
      | ERROR | agent  | agent.http-writer    |
      | INFO  | agent  | agent.modbus-writer  |
      | ERROR | node   | node.modbus-filter   |
    만약 레벨 필터를 "ERROR"로 설정하고
    그리고 소스 필터에서 "agent"만 활성화하고
    그리고 컴포넌트 검색란에 "modbus"를 입력하면
    그러면 "agent.modbus-reader" 1건만 표시되어야 한다

  시나리오: 필터 해제 시 전체 복원
    주어진 레벨="ERROR", 소스="agent만", 검색="modbus" 필터가 적용된 상태일 때
    만약 레벨 필터를 "ALL"로 변경하고
    그리고 모든 소스 토글을 활성화하고
    그리고 검색란을 비우면
    그러면 모든 로그 항목이 표시되어야 한다
```

### AC-WEB-001-27: 필터링 성능

```gherkin
기능: 대량 로그에서 필터 성능

  시나리오: 10,000건 로그에서 필터 반응 시간
    주어진 로그 항목이 10,000건 누적된 상태일 때
    만약 소스 필터 토글을 클릭하면
    그러면 필터링된 목록이 100ms 이내에 갱신되어야 한다
    그리고 스크롤이 끊김 없이 동작해야 한다

  시나리오: 10,000건 로그에서 컴포넌트 검색 성능
    주어진 로그 항목이 10,000건 누적된 상태일 때
    만약 컴포넌트 검색란에 "modbus"를 입력하면
    그러면 디바운스(300ms) 후 100ms 이내에 필터링 결과가 표시되어야 한다
    그리고 가상 스크롤 동작에 프레임 드롭이 없어야 한다

  시나리오: 필터 상태에서 신규 로그 추가 성능
    주어진 소스 필터에서 "agent"만 활성화된 상태이고
    그리고 로그 항목이 5,000건 누적된 상태일 때
    만약 새로운 source="agent" 로그가 수신되면
    그러면 필터링된 목록에 즉시 추가되어야 한다
    그리고 source="node" 로그가 수신되면 목록에 추가되지 않아야 한다
```

---

## 8. 비기능 요구사항

### AC-WEB-001-20: API 성능

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

### AC-WEB-001-21: 에러 복원력

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

## 9. Quality Gate 체크리스트

- [x] Module 1: `agentService.ts`의 `getAgents` 호출에 `detail=summary` 파라미터가 추가됨
- [x] Module 1: 백엔드 `ListOptions.Detail` 파라미터 전달이 수정됨
- [x] Module 1: 에이전트 목록 테이블에서 stats 데이터가 올바르게 렌더링됨
- [x] Module 2: `GET /monitor/loglevel` 엔드포인트가 전체 레벨 맵을 반환함
- [x] Module 2: `PUT /monitor/loglevel/{component}` 엔드포인트가 개별 레벨을 설정함
- [x] Module 2: `DELETE /monitor/loglevel/{component}` 엔드포인트가 오버라이드를 리셋함
- [x] Module 2: 유효하지 않은 레벨 값에 대해 400 응답을 반환함
- [x] Module 2: 기존 `PUT /monitor/loglevel` 글로벌 API가 정상 동작함
- [x] Module 3: 에이전트 상세 패널에 로그 레벨 드롭다운이 표시됨
- [x] Module 3: 드롭다운 변경 시 올바른 API가 호출됨
- [x] Module 3: 설정 페이지에 컴포넌트별 오버라이드 목록이 표시됨
- [x] Module 4: 플로우 확장 패널에 노드 인스턴스 테이블이 표시됨
- [x] Module 4: In/Out 메시지 분리 표시 (ArrowDownToLine/ArrowUpFromLine 아이콘)
- [x] Module 4: 노드별 로그 레벨 드롭다운이 동작함
- [x] Module 4: running 플로우에서 3초 자동 갱신 동작
- [x] Module 4: 빈 노드 상태에 안내 메시지가 표시됨
- [x] Module 5: RuntimeStatsContext가 editorStore와 분리됨
- [x] Module 5: EditorPage에서 런타임 통계 폴링 및 Provider 래핑
- [x] Module 5: CustomNode에 런타임 상태 표시등 + In/Out 표시
- [x] Module 5: 런타임 통계 갱신이 isDirty/undo에 영향 없음
- [x] BF: engine.go 포트 카운터가 모든 포트에 대해 초기화됨
- [x] BF: bridge.go msgCh 초기화 및 Process() 반환값 수정됨
- [x] BF: bridge_test.go 테스트 통과
- [x] TypeScript strict 모드에서 타입 에러 0건
- [x] Module 7: LogEntry 인터페이스에 component?, source? 필드가 추가됨
- [x] Module 7: MonitoringPage handleLog에서 component/source 추출이 동작함
- [x] Module 7: 로그 행에 source 배지가 SOURCE_STYLES 색상으로 표시됨
- [x] Module 7: 로그 행에 component 이름이 truncate로 표시됨
- [x] Module 7: 소스 필터 6개 토글 버튼이 멀티 셀렉트로 동작함
- [x] Module 7: 컴포넌트 검색 입력란이 300ms 디바운스로 동작함
- [x] Module 7: 레벨 + 소스 + 컴포넌트 검색 AND 복합 필터가 동작함
- [x] Module 7: source/component 미포함 로그의 하위 호환성 유지됨
- [x] Module 7: 10,000건 로그에서 필터 전환 100ms 이내 동작
- [ ] ESLint 경고 0건

---

## 10. Definition of Done

- [x] Module 1 (P0): 에이전트 목록 페이지에서 stats 데이터가 정상 표시됨 (백엔드 + 프론트엔드 수정)
- [x] Module 2 (P1): 3개 API 엔드포인트가 구현되고 테스트 통과
- [x] Module 3 (P1): 에이전트 상세 패널 + 설정 페이지 UI 구현 완료
- [x] Module 4 (P1): 플로우 확장 패널 + In/Out 분리 표시 + 실시간 갱신 + 로그 레벨 UI 구현 완료
- [x] Module 5 (P1): 캔버스 RuntimeStatsContext + CustomNode 런타임 상태/In/Out 표시 구현 완료
- [x] BF (P0): engine.go 포트 카운터 초기화 + bridge.go 버그 수정 완료
- [x] Module 7 (P1): LogEntry 확장 + source 배지/component 표시 + 소스 필터(멀티 셀렉트) + 컴포넌트 검색(디바운스) 구현 완료
- [x] 기존 글로벌 로그 레벨 기능이 정상 동작 (회귀 없음)
- [x] 모든 신규 백엔드 핸들러에 단위 테스트 존재
- [x] 프론트엔드 TypeScript 컴파일 에러 없음
- [x] Vite 프로덕션 빌드 성공

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.3.0*
*상태: planned*
*최종 수정: 2026-03-08*
