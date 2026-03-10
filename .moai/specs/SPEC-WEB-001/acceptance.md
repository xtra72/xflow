---
id: SPEC-WEB-001
type: acceptance
version: "2.0.0"
status: planned
created: "2026-03-07"
updated: "2026-03-10"
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

## 8. Module 8: 동적 포트 시스템 (P0)

### AC-WEB-001-28: 브릿지 노드 방향별 포트 표시

```gherkin
기능: 브릿지 노드의 direction에 따른 포트 동적 표시

  시나리오: BridgeIn(agent->flow) 노드 포트 표시
    주어진 에디터에서 브릿지 노드를 생성하고 direction을 "in"으로 설정할 때
    그러면 해당 노드에 출력(output) 포트만 표시되어야 한다
    그리고 입력(input) 포트는 표시되지 않아야 한다

  시나리오: BridgeOut(flow->agent) 노드 포트 표시
    주어진 에디터에서 브릿지 노드를 생성하고 direction을 "out"으로 설정할 때
    그러면 해당 노드에 입력(input) 포트만 표시되어야 한다
    그리고 출력(output) 포트는 표시되지 않아야 한다

  시나리오: BridgeInOut 노드 포트 표시
    주어진 에디터에서 브릿지 노드를 생성하고 direction을 "inout"으로 설정할 때
    그러면 해당 노드에 입력(input)과 출력(output) 포트가 모두 표시되어야 한다

  시나리오: BridgeRequestReply 노드 포트 표시
    주어진 에디터에서 브릿지 노드를 생성하고 direction을 "request_reply"로 설정할 때
    그러면 해당 노드에 입력(input)과 출력(output) 포트가 모두 표시되어야 한다

  시나리오: direction 미설정 브릿지 노드
    주어진 에디터에서 브릿지 노드를 생성하고 direction을 설정하지 않을 때
    그러면 해당 노드에 기본값으로 입력(input)과 출력(output) 포트가 모두 표시되어야 한다
```

### AC-WEB-001-29: 스위치 노드 라우트 기반 동적 포트

```gherkin
기능: 스위치 노드의 라우트 설정에 따른 출력 포트 동적 생성

  시나리오: 라우트가 설정된 스위치 노드
    주어진 에디터에서 스위치 노드를 생성하고 routes에 ["route-a", "route-b"]를 설정할 때
    그러면 해당 노드에 입력 포트 "in" 1개가 표시되어야 한다
    그리고 출력 포트 "route-a", "route-b", "default" 3개가 표시되어야 한다

  시나리오: 라우트 추가 시 포트 동적 추가
    주어진 스위치 노드에 라우트 ["route-a"]가 설정된 상태일 때
    만약 라우트 설정에 "route-c"를 추가하면
    그러면 출력 포트에 "route-c"가 추가되어야 한다
    그리고 기존 "route-a"와 "default" 출력 포트는 유지되어야 한다

  시나리오: 라우트 삭제 시 포트 동적 제거
    주어진 스위치 노드에 라우트 ["route-a", "route-b"]가 설정된 상태일 때
    만약 라우트 설정에서 "route-b"를 삭제하면
    그러면 출력 포트 "route-b"가 제거되어야 한다
    그리고 "route-b" 포트에 연결된 엣지가 자동으로 제거되어야 한다
    그리고 "route-a"와 "default" 출력 포트는 유지되어야 한다

  시나리오: 라우트 미설정 스위치 노드
    주어진 에디터에서 스위치 노드를 생성하고 routes를 설정하지 않을 때
    그러면 기본 입출력 포트 "in"과 "out"이 표시되어야 한다
```

### AC-WEB-001-30: 에디터 설정 변경 시 포트 재계산

```gherkin
기능: 노드 설정 변경 시 포트 즉시 재계산

  시나리오: 브릿지 direction 변경 시 포트 업데이트
    주어진 브릿지 노드가 direction="inout"으로 input/output 포트가 모두 표시된 상태일 때
    만약 direction을 "in"으로 변경하면
    그러면 입력(input) 포트가 즉시 제거되어야 한다
    그리고 출력(output) 포트만 남아야 한다
    그리고 제거된 입력 포트에 연결된 엣지가 자동 삭제되어야 한다

  시나리오: direction 변경 후 다시 원복 시 포트 복원
    주어진 브릿지 노드가 direction="in"(output 포트만)인 상태일 때
    만약 direction을 "inout"으로 변경하면
    그러면 입력(input) 포트가 다시 추가되어야 한다
    그리고 출력(output) 포트도 유지되어야 한다

  시나리오: 기타 노드 타입의 설정 변경은 포트에 영향 없음
    주어진 "function" 타입 노드가 기본 in/out 포트를 가진 상태일 때
    만약 노드의 임의 설정(code, timeout 등)을 변경해도
    그러면 포트 구성이 변경되지 않아야 한다
```

### AC-WEB-001-31: 엣지 자동 정리

```gherkin
기능: 포트 제거 시 연결된 엣지 자동 삭제

  시나리오: 입력 포트 제거 시 인커밍 엣지 삭제
    주어진 브릿지 노드(direction="inout")의 입력 포트에 다른 노드로부터 엣지가 연결된 상태일 때
    만약 direction을 "in"(output만)으로 변경하면
    그러면 입력 포트에 연결된 인커밍 엣지가 자동 삭제되어야 한다
    그리고 출력 포트에 연결된 아웃고잉 엣지는 유지되어야 한다

  시나리오: 출력 포트 제거 시 아웃고잉 엣지 삭제
    주어진 브릿지 노드(direction="inout")의 출력 포트에 다른 노드로 엣지가 연결된 상태일 때
    만약 direction을 "out"(input만)으로 변경하면
    그러면 출력 포트에 연결된 아웃고잉 엣지가 자동 삭제되어야 한다
    그리고 입력 포트에 연결된 인커밍 엣지는 유지되어야 한다

  시나리오: 스위치 라우트 삭제 시 해당 포트 엣지 삭제
    주어진 스위치 노드의 "route-b" 출력 포트에 엣지가 연결된 상태일 때
    만약 routes에서 "route-b"를 삭제하면
    그러면 "route-b" 포트에 연결된 엣지가 자동 삭제되어야 한다
    그리고 "route-a" 및 "default" 포트의 엣지는 유지되어야 한다
```

### AC-WEB-001-32: 백엔드 NewNodeDef 브릿지 방향 인식

```gherkin
기능: 백엔드에서 브릿지 노드 생성 시 direction 기반 포트 설정

  시나리오: BridgeIn 노드 기본 포트
    주어진 BridgeIn 방향의 브릿지 노드를 NewNodeDef로 생성할 때
    그러면 Outputs에 "out" 포트만 포함되어야 한다
    그리고 Inputs는 비어 있어야 한다

  시나리오: BridgeOut 노드 기본 포트
    주어진 BridgeOut 방향의 브릿지 노드를 NewNodeDef로 생성할 때
    그러면 Inputs에 "in" 포트만 포함되어야 한다
    그리고 Outputs는 비어 있어야 한다

  시나리오: BridgeInOut 노드 기본 포트
    주어진 BridgeInOut 방향의 브릿지 노드를 NewNodeDef로 생성할 때
    그러면 Inputs에 "in" 포트가 포함되어야 한다
    그리고 Outputs에 "out" 포트가 포함되어야 한다

  시나리오: 플로우 저장 후 다시 로드 시 포트 유지
    주어진 BridgeIn 노드가 포함된 플로우를 저장한 후
    만약 해당 플로우를 에디터에서 다시 로드하면
    그러면 BridgeIn 노드에 출력(output) 포트만 표시되어야 한다
    그리고 flowToReactFlowConfig 변환이 올바른 포트를 반환해야 한다
```

### AC-WEB-001-33: 기존 노드 타입 호환성

```gherkin
기능: Module 8 변경이 기존 노드 타입에 영향을 주지 않음

  시나리오: function 노드 포트 유지
    주어진 "function" 타입 노드를 에디터에서 생성할 때
    그러면 기존과 동일하게 입력 포트 "in"과 출력 포트 "out"이 표시되어야 한다

  시나리오: filter 노드 포트 유지
    주어진 "filter" 타입 노드를 에디터에서 생성할 때
    그러면 기존과 동일한 기본 포트가 표시되어야 한다

  시나리오: 기존 저장된 플로우의 bridge/switch 노드 표시
    주어진 Module 8 이전에 저장된 bridge 노드(direction 설정 포함)가 있는 플로우를 로드할 때
    그러면 백엔드가 반환하는 Inputs/Outputs 포트 데이터에 따라 올바르게 표시되어야 한다
    그리고 에디터에서 해당 노드의 direction을 변경하면 포트가 동적으로 재계산되어야 한다
```

---

## 9. Module 9: 에러 포트 타입 지원 (P0)

### AC-WEB-001-34: PortDef 및 NodeTypeDefinition 에러 포트 타입 지원

```gherkin
기능: 프론트엔드 포트 타입 시스템에서 'error' direction 지원

  시나리오: 백엔드에서 에러 포트가 포함된 노드 데이터 수신
    주어진 백엔드가 direction: "error"인 포트를 포함한 노드 정의를 반환할 때
    만약 프론트엔드가 해당 노드 데이터를 파싱하면
    그러면 PortDef 타입이 direction: 'error' 값을 허용해야 한다
    그리고 NodeTypeDefinition.ports[] 배열에 direction: 'error' 포트가 포함될 수 있어야 한다
    그리고 TypeScript 컴파일 에러가 발생하지 않아야 한다

  시나리오: 기존 input/output direction 하위 호환
    주어진 기존 노드 정의에 direction이 'input' 또는 'output'만 있을 때
    만약 해당 노드를 로드하면
    그러면 기존과 동일하게 포트가 올바르게 처리되어야 한다
    그리고 타입 확장으로 인한 동작 변화가 없어야 한다
```

### AC-WEB-001-35: CustomNode 에러 포트 렌더링

```gherkin
기능: CustomNode 컴포넌트에서 에러 포트를 시각적으로 구분하여 렌더링

  시나리오: 에러 포트가 있는 노드의 하단 배치
    주어진 노드에 direction: 'error'인 포트가 1개 있을 때
    만약 해당 노드가 캔버스에 렌더링되면
    그러면 에러 포트 핸들이 노드 하단(Bottom)에 배치되어야 한다
    그리고 에러 포트 핸들은 빨간색으로 표시되어야 한다
    그리고 입력 포트(좌측/상단)와 출력 포트(우측/하단)는 기존 위치를 유지해야 한다

  시나리오: 에러 포트가 없는 기존 노드의 정상 렌더링
    주어진 노드에 direction: 'error'인 포트가 없을 때
    만약 해당 노드가 캔버스에 렌더링되면
    그러면 노드 하단에 에러 포트 핸들이 표시되지 않아야 한다
    그리고 기존과 동일한 레이아웃으로 렌더링되어야 한다

  시나리오: 에러 포트와 출력 포트가 공존하는 노드
    주어진 노드에 output 포트 1개와 error 포트 1개가 있을 때
    만약 해당 노드가 캔버스에 렌더링되면
    그러면 출력 포트는 기존 위치(우측)에 초록색으로 표시되어야 한다
    그리고 에러 포트는 노드 하단에 빨간색으로 표시되어야 한다
    그리고 두 포트가 시각적으로 명확히 구분되어야 한다
```

### AC-WEB-001-36: NodeHandle 에러 포트 색상

```gherkin
기능: NodeHandle 컴포넌트에서 포트 타입별 색상 구분

  시나리오: 에러 포트 핸들 빨간색 표시
    주어진 에러 포트(direction: 'error')가 있는 노드가 렌더링될 때
    그러면 에러 포트 핸들이 빨간색(bg-red-500)으로 표시되어야 한다

  시나리오: 기존 포트 핸들 색상 유지
    주어진 입력 포트(direction: 'input')와 출력 포트(direction: 'output')가 있는 노드가 렌더링될 때
    그러면 입력 포트 핸들은 파란색(bg-blue-500)으로 표시되어야 한다
    그리고 출력 포트 핸들은 초록색(bg-green-500)으로 표시되어야 한다

  시나리오: 세 가지 포트 타입 동시 표시
    주어진 노드에 input, output, error 포트가 모두 있을 때
    만약 해당 노드가 캔버스에 렌더링되면
    그러면 입력 핸들은 파란색, 출력 핸들은 초록색, 에러 핸들은 빨간색으로 표시되어야 한다
    그리고 각 색상이 한눈에 구분 가능해야 한다
```

### AC-WEB-001-37: computePortsForNode 에러 포트 지원

```gherkin
기능: computePortsForNode 함수에서 에러 포트를 포함한 포트 배열 반환

  시나리오: 에러 포트를 지원하는 노드 타입의 포트 계산
    주어진 백엔드에서 에러 포트가 정의된 노드 타입의 데이터를 수신할 때
    만약 computePortsForNode(nodeType, config)를 호출하면
    그러면 반환되는 포트 배열에 direction: 'error'인 포트가 포함되어야 한다
    그리고 기존 input/output 포트도 함께 포함되어야 한다

  시나리오: 에러 포트가 없는 일반 노드 타입의 포트 계산
    주어진 에러 포트가 정의되지 않은 "function" 타입 노드일 때
    만약 computePortsForNode('function', config)를 호출하면
    그러면 반환되는 포트 배열에 direction: 'error'인 포트가 포함되지 않아야 한다
    그리고 기존과 동일한 input/output 포트만 반환되어야 한다
```

### AC-WEB-001-38: 에러 포트 없는 기존 노드 호환성

```gherkin
기능: 에러 포트가 없는 기존 노드의 정상 동작 보장

  시나리오: 기존 저장된 플로우 로드 시 호환성
    주어진 Module 9 이전에 저장된 플로우(에러 포트 없음)를 로드할 때
    그러면 모든 노드가 기존과 동일하게 렌더링되어야 한다
    그리고 에러 포트 관련 핸들이 표시되지 않아야 한다
    그리고 기존 엣지 연결이 모두 유지되어야 한다

  시나리오: 에러 포트 엣지 연결 및 저장
    주어진 에러 포트가 있는 노드에서 에러 포트에 엣지를 연결한 상태일 때
    만약 플로우를 저장한 후 다시 로드하면
    그러면 에러 포트에 연결된 엣지가 올바르게 복원되어야 한다
    그리고 에러 포트 핸들이 빨간색으로 표시되어야 한다

  시나리오: 에디터에서 에러 포트 사용 후 빌드 검증
    주어진 에러 포트가 연결된 플로우가 존재할 때
    만약 Vite 프로덕션 빌드를 실행하면
    그러면 빌드가 성공해야 한다
    그리고 TypeScript 컴파일 에러가 0건이어야 한다
```

---

## 10. Module 10: Handle ID 접두사 제거 (P0 - 리팩토링)

### AC-WEB-001-39: Handle ID에 접두사 미사용

```gherkin
기능: 포트 이름을 Handle ID로 직접 사용

  시나리오: CustomNode의 Handle ID
    주어진 브릿지 노드에 "in", "out", "error" 포트가 정의되어 있을 때
    만약 CustomNode가 해당 노드를 렌더링하면
    그러면 각 Handle의 id 속성은 포트 이름 그대로("in", "out", "error")여야 한다
    그리고 접두사("in-", "out-", "err-")가 포함되지 않아야 한다
```

### AC-WEB-001-40: 백엔드 Edge 매핑

```gherkin
기능: 백엔드 Wire↔Edge 매핑에서 접두사 변환 없음

  시나리오: flowToReactFlowConfig Edge 생성
    주어진 Wire의 SourcePort가 "out"이고 TargetPort가 "in"일 때
    만약 flowToReactFlowConfig가 Edge를 생성하면
    그러면 sourceHandle은 "out"이어야 한다
    그리고 targetHandle은 "in"이어야 한다
    그리고 접두사 추가/제거 로직이 없어야 한다

  시나리오: normalizeReactFlowDefinition Edge 역변환
    주어진 Edge의 sourceHandle이 "out"이고 targetHandle이 "in"일 때
    만약 normalizeReactFlowDefinition이 Wire를 생성하면
    그러면 source_port는 "out"이어야 한다
    그리고 target_port는 "in"이어야 한다
    그리고 TrimPrefix 등의 변환이 없어야 한다
```

### AC-WEB-001-41: PropertyPanel Port 타입 에러 지원

```gherkin
기능: PropertyPanel에서 에러 포트 direction 지원

  시나리오: Port 타입의 direction 범위
    주어진 PropertyPanel이 노드의 포트를 관리할 때
    그러면 Port 타입은 'input', 'output', 'error' 세 가지 direction을 모두 지원해야 한다

  시나리오: 포트 삭제 시 엣지 정리
    주어진 포트 이름이 "error"인 에러 포트가 삭제되었을 때
    만약 PropertyPanel이 엣지를 정리하면
    그러면 포트 이름 "error"로 직접 비교하여 연결된 엣지를 삭제해야 한다
```

---

## 11. Module 11: 리스트 정렬 기능 (P1 - 신규 기능)

### AC-WEB-001-42: 기본 정렬

```gherkin
기능: 리스트 페이지 기본 정렬

  시나리오: FlowListPage 기본 정렬
    주어진 사용자가 플로우 목록 페이지에 접근할 때
    그러면 플로우 목록은 이름(name) 기준 오름차순으로 정렬되어야 한다

  시나리오: AgentListPage 기본 정렬
    주어진 사용자가 에이전트 목록 페이지에 접근할 때
    그러면 에이전트 목록은 이름(name) 기준 오름차순으로 정렬되어야 한다

  시나리오: FlowDetailPanel 노드 인스턴스 기본 정렬
    주어진 사용자가 플로우 행을 확장하여 노드 인스턴스 목록을 볼 때
    그러면 노드 목록은 이름(name) 기준 오름차순으로 정렬되어야 한다
```

### AC-WEB-001-43: 정렬 가능 컬럼 헤더

```gherkin
기능: 정렬 가능한 컬럼 헤더 UI

  시나리오: FlowListPage 정렬 가능 컬럼
    주어진 플로우 목록 페이지가 표시될 때
    그러면 이름, 상태, 생성일, 수정일 컬럼 헤더에 정렬 인디케이터가 표시되어야 한다
    그리고 노드, 액션 컬럼은 정렬 불가여야 한다

  시나리오: AgentListPage 정렬 가능 컬럼
    주어진 에이전트 목록 페이지가 표시될 때
    그러면 이름, 타입, 상태 컬럼 헤더에 정렬 인디케이터가 표시되어야 한다
    그리고 업타임, 메시지, 액션 컬럼은 정렬 불가여야 한다

  시나리오: FlowDetailPanel 정렬 가능 컬럼
    주어진 플로우 노드 인스턴스 목록이 표시될 때
    그러면 이름, 타입, 상태 컬럼 헤더에 정렬 인디케이터가 표시되어야 한다
    그리고 In/Out, 로그 레벨 컬럼은 정렬 불가여야 한다
```

### AC-WEB-001-44: 정렬 토글 동작

```gherkin
기능: 컬럼 헤더 클릭 시 정렬 토글

  시나리오: 같은 컬럼 클릭 시 방향 토글
    주어진 현재 이름 컬럼이 오름차순(▲)으로 정렬 중일 때
    만약 이름 컬럼 헤더를 클릭하면
    그러면 내림차순(▼)으로 변경되어야 한다
    그리고 정렬 인디케이터가 ▼로 변경되어야 한다

  시나리오: 다른 컬럼 클릭 시 오름차순 설정
    주어진 현재 이름 컬럼이 내림차순으로 정렬 중일 때
    만약 상태 컬럼 헤더를 클릭하면
    그러면 상태 컬럼 기준 오름차순으로 변경되어야 한다
```

### AC-WEB-001-45: 백엔드 정렬 처리

```gherkin
기능: 백엔드 sort 파라미터 처리

  시나리오: parseSortParam 기본값
    주어진 sort 파라미터가 비어 있을 때
    그러면 기본값 ("name", true/asc)를 반환해야 한다

  시나리오: FlowServiceAdapter 정렬
    주어진 sort 파라미터가 "name:desc"일 때
    만약 ListFlows가 호출되면
    그러면 결과는 이름 내림차순으로 정렬되어야 한다
    그리고 정렬은 페이지네이션 적용 전에 수행되어야 한다

  시나리오: AgentServiceAdapter 정렬
    주어진 sort 파라미터가 "type:asc"일 때
    만약 ListAgents가 호출되면
    그러면 결과는 타입 오름차순으로 정렬되어야 한다
```

---

## 12. Module 12: 대시보드 패널 재구성 (P1 - 리팩토링)

### AC-WEB-001-46: FlowPanel 상태 요약 표시

```gherkin
기능: FlowPanel 상단 영역에 플로우 상태별 건수 요약 표시

  시나리오: 플로우 상태별 건수 표시
    주어진 플로우가 running 3개, stopped 2개, error 1개, stored 1개, loaded 1개 존재할 때
    만약 대시보드 페이지에 접근하면
    그러면 FlowPanel 상단에 각 상태별 건수가 FlowStatusBadge와 함께 표시되어야 한다
    그리고 건수는 실제 useFlows() 데이터에서 계산된 값이어야 한다

  시나리오: 플로우가 없는 경우 빈 상태 표시
    주어진 등록된 플로우가 없을 때
    만약 대시보드 페이지에 접근하면
    그러면 FlowPanel 상단에 모든 상태 건수가 0으로 표시되어야 한다
    그리고 하단 리스트 영역에 빈 상태 안내 메시지가 표시되어야 한다
```

### AC-WEB-001-47: FlowPanel 리스트 테이블

```gherkin
기능: FlowPanel 하단 영역에 플로우 리스트 테이블 표시

  시나리오: 플로우 리스트 기본 표시
    주어진 플로우 5개가 존재할 때
    만약 대시보드 페이지에 접근하면
    그러면 FlowPanel 하단에 테이블이 표시되어야 한다
    그리고 컬럼은 이름, 상태, 노드 수, 동작 시간, 액션 순서로 구성되어야 한다
    그리고 각 행에 플로우 정보가 올바르게 표시되어야 한다

  시나리오: 플로우 이름 클릭 시 에디터 이동
    주어진 FlowPanel 리스트에 플로우 "modbus-flow"가 표시될 때
    만약 이름 "modbus-flow"를 클릭하면
    그러면 에디터 페이지(/editor/{flowId})로 이동해야 한다

  시나리오: 상태 배지 표시
    주어진 running 상태의 플로우가 리스트에 있을 때
    그러면 상태 컬럼에 FlowStatusBadge가 올바른 색상으로 표시되어야 한다

  시나리오: 동작 시간 표시
    주어진 running 상태의 플로우가 updated_at 값을 가지고 있을 때
    그러면 동작 시간 컬럼에 상대적 시간("3분 전", "1시간 전" 등)이 표시되어야 한다

  시나리오: 노드 수 표시
    주어진 플로우에 nodes 배열이 5개 요소를 가질 때
    그러면 노드 수 컬럼에 "5"가 표시되어야 한다
```

### AC-WEB-001-48: FlowPanel 이름 정렬

```gherkin
기능: FlowPanel 리스트에서 이름 컬럼 정렬

  시나리오: 기본 정렬 (이름 오름차순)
    주어진 플로우 "bravo", "alpha", "charlie"가 존재할 때
    만약 대시보드 페이지에 접근하면
    그러면 FlowPanel 리스트는 이름 오름차순("alpha", "bravo", "charlie")으로 정렬되어야 한다

  시나리오: 이름 헤더 클릭 시 정렬 토글
    주어진 FlowPanel 리스트가 이름 오름차순으로 정렬된 상태일 때
    만약 이름 컬럼 헤더(SortableHeader)를 클릭하면
    그러면 이름 내림차순("charlie", "bravo", "alpha")으로 재정렬되어야 한다
    그리고 정렬 인디케이터가 변경되어야 한다
```

### AC-WEB-001-49: FlowPanel 최대 10행 및 "더 보기" 링크

```gherkin
기능: FlowPanel 리스트 최대 10행 표시 및 더 보기 링크

  시나리오: 10개 이하 플로우 표시
    주어진 플로우가 8개 존재할 때
    만약 대시보드 페이지에 접근하면
    그러면 FlowPanel 리스트에 8개 행이 모두 표시되어야 한다
    그리고 "더 보기" 링크가 표시되지 않아야 한다

  시나리오: 11개 이상 플로우 표시
    주어진 플로우가 15개 존재할 때
    만약 대시보드 페이지에 접근하면
    그러면 FlowPanel 리스트에 10개 행만 표시되어야 한다
    그리고 리스트 하단에 "더 보기" 링크가 표시되어야 한다

  시나리오: "더 보기" 링크 클릭 시 플로우 목록 페이지 이동
    주어진 "더 보기" 링크가 표시된 상태일 때
    만약 "더 보기" 링크를 클릭하면
    그러면 플로우 목록 페이지(/flows)로 이동해야 한다
```

### AC-WEB-001-50: FlowPanel 액션 버튼

```gherkin
기능: FlowPanel 리스트에서 플로우 시작/정지 액션

  시나리오: running 플로우 정지 버튼
    주어진 FlowPanel 리스트에 running 상태의 플로우가 표시될 때
    그러면 해당 행의 액션 컬럼에 Pause 아이콘 버튼이 표시되어야 한다
    만약 Pause 버튼을 클릭하면
    그러면 flowService.stopFlow(id) API가 호출되어야 한다
    그리고 성공 시 플로우 쿼리가 무효화되어 목록이 갱신되어야 한다

  시나리오: stopped 플로우 시작 버튼
    주어진 FlowPanel 리스트에 stopped 상태의 플로우가 표시될 때
    그러면 해당 행의 액션 컬럼에 Play 아이콘 버튼이 표시되어야 한다
    만약 Play 버튼을 클릭하면
    그러면 flowService.startFlow(id) API가 호출되어야 한다
    그리고 성공 시 플로우 쿼리가 무효화되어 목록이 갱신되어야 한다

  시나리오: error 플로우 재시작 버튼
    주어진 FlowPanel 리스트에 error 상태의 플로우가 표시될 때
    그러면 해당 행의 액션 컬럼에 RotateCcw 아이콘 버튼이 표시되어야 한다

  시나리오: 액션 실패 시 토스트 피드백
    주어진 FlowPanel 리스트에서 액션 버튼을 클릭했을 때
    만약 API 호출이 실패하면
    그러면 에러 토스트가 표시되어야 한다
    그리고 플로우 상태는 변경되지 않아야 한다
```

### AC-WEB-001-51: AgentPanel 상태 요약 표시

```gherkin
기능: AgentPanel 상단 영역에 에이전트 상태별 건수 요약 표시

  시나리오: 에이전트 상태별 건수 표시
    주어진 에이전트가 running 5개, stopped 3개, error 2개 존재할 때
    만약 대시보드 페이지에 접근하면
    그러면 AgentPanel 상단에 total(10), active(5), inactive(5) 건수가 표시되어야 한다
    그리고 AgentStatusBadge와 함께 각 카테고리가 구분 표시되어야 한다

  시나리오: 에이전트가 없는 경우 빈 상태 표시
    주어진 등록된 에이전트가 없을 때
    만약 대시보드 페이지에 접근하면
    그러면 AgentPanel 상단에 total(0), active(0), inactive(0)이 표시되어야 한다
```

### AC-WEB-001-52: AgentPanel 리스트 테이블

```gherkin
기능: AgentPanel 하단 영역에 에이전트 리스트 테이블 표시

  시나리오: 에이전트 리스트 기본 표시
    주어진 에이전트 5개가 detail=summary 데이터와 함께 로드될 때
    만약 대시보드 페이지에 접근하면
    그러면 AgentPanel 하단에 테이블이 표시되어야 한다
    그리고 컬럼은 이름, 타입, 상태, 업타임, 메시지 IN/OUT, 액션 순서로 구성되어야 한다

  시나리오: 에이전트 업타임 표시
    주어진 running 에이전트에 uptime 값이 있을 때
    그러면 업타임 컬럼에 해당 값이 표시되어야 한다

  시나리오: 에이전트 업타임 값 없는 경우
    주어진 stopped 에이전트에 uptime 값이 null일 때
    그러면 업타임 컬럼에 "-"가 표시되어야 한다

  시나리오: 에이전트 메시지 IN/OUT 표시
    주어진 에이전트의 stats.messages_in이 100이고 stats.messages_out이 50일 때
    그러면 메시지 IN/OUT 컬럼에 "100 / 50"이 표시되어야 한다

  시나리오: 에이전트 이름 정렬 및 최대 10행
    주어진 에이전트가 12개 존재할 때
    만약 대시보드 페이지에 접근하면
    그러면 AgentPanel 리스트에 이름 오름차순으로 10행만 표시되어야 한다
    그리고 "더 보기" 링크가 표시되어 /agents로 이동할 수 있어야 한다
```

### AC-WEB-001-53: DashboardPage 레이아웃 변경

```gherkin
기능: 대시보드 페이지 레이아웃이 3패널 구조로 재구성됨

  시나리오: 데스크톱 레이아웃 (md 이상)
    주어진 브라우저 너비가 768px 이상일 때
    만약 대시보드 페이지에 접근하면
    그러면 상단에 FlowPanel과 AgentPanel이 나란히 2열로 배치되어야 한다
    그리고 하단에 ResourceWidget이 전체 너비로 배치되어야 한다

  시나리오: 모바일 레이아웃 (md 미만)
    주어진 브라우저 너비가 768px 미만일 때
    만약 대시보드 페이지에 접근하면
    그러면 FlowPanel, AgentPanel, ResourceWidget이 세로 스택으로 배치되어야 한다
    그리고 FlowPanel이 최상단에 표시되어야 한다

  시나리오: ResourceWidget 기존 동작 유지
    주어진 대시보드 페이지에 접근할 때
    그러면 ResourceWidget에 CPU 사용률과 메모리 사용률이 게이지/차트로 표시되어야 한다
    그리고 기존 모니터링 API(monitor/metrics)를 동일하게 호출해야 한다
```

### AC-WEB-001-54: 기존 위젯 삭제 및 데이터 무결성

```gherkin
기능: 기존 위젯 삭제 후 데이터 무결성 검증

  시나리오: SystemStatusWidget 데이터 포함 확인
    주어진 기존 SystemStatusWidget이 표시하던 플로우 상태 정보(running/stopped/error 건수)가 있을 때
    만약 대시보드 페이지에서 FlowPanel을 확인하면
    그러면 FlowPanel 상태 요약에 동일한 건수가 표시되어야 한다
    그리고 기존 대비 누락된 정보가 없어야 한다

  시나리오: RecentFlowsWidget 데이터 포함 확인
    주어진 기존 RecentFlowsWidget이 표시하던 최근 플로우 목록이 있을 때
    만약 대시보드 페이지에서 FlowPanel 리스트를 확인하면
    그러면 기존 위젯에서 제공하던 플로우 이름, 상태 정보가 모두 포함되어야 한다

  시나리오: AgentStatusWidget 데이터 포함 확인
    주어진 기존 AgentStatusWidget이 표시하던 에이전트 상태 정보(total/active/inactive)가 있을 때
    만약 대시보드 페이지에서 AgentPanel을 확인하면
    그러면 AgentPanel 상태 요약에 동일한 건수가 표시되어야 한다

  시나리오: 삭제된 위젯 파일 참조 없음
    주어진 SystemStatusWidget.tsx, RecentFlowsWidget.tsx, AgentStatusWidget.tsx가 삭제된 상태일 때
    만약 TypeScript 컴파일을 실행하면
    그러면 삭제된 파일에 대한 import 에러가 0건이어야 한다
    그리고 Vite 프로덕션 빌드가 성공해야 한다
```

---

## 13. Module 13: Import/Export 기능 (P2 - 신규 기능)

### AC-WEB-001-55: 플로우 단일 Export

```gherkin
기능: 플로우를 JSON 파일로 내보내기

  시나리오: 플로우 내보내기 버튼 클릭
    주어진 플로우 "modbus-flow"가 존재하고 FlowActionMenu가 열린 상태일 때
    만약 "내보내기" 메뉴 항목을 클릭하면
    그러면 flowService.exportFlow(id) API가 호출되어야 한다
    그리고 "modbus-flow.json" 파일이 브라우저에서 다운로드되어야 한다
    그리고 파일 내용에 "name", "definition" 필드가 포함되어야 한다
    그리고 "id", "status", "stats", "created_at", "updated_at" 필드가 포함되지 않아야 한다

  시나리오: Export 파일 구조 검증
    주어진 플로우 "data-pipeline"에 설명(description)이 있고 노드 3개, 와이어 2개가 있을 때
    만약 해당 플로우를 내보내면
    그러면 JSON 파일에 "name": "data-pipeline"이 포함되어야 한다
    그리고 "description" 필드가 포함되어야 한다
    그리고 "definition.nodes" 배열에 3개 항목이 있어야 한다
    그리고 "definition.wires" 배열에 2개 항목이 있어야 한다

  시나리오: 설명이 없는 플로우 Export
    주어진 플로우 "simple-flow"에 설명(description)이 없을 때
    만약 해당 플로우를 내보내면
    그러면 JSON 파일에 "description" 필드가 없거나 null이어야 한다
    그리고 "name"과 "definition" 필드는 정상 포함되어야 한다
```

### AC-WEB-001-56: 플로우 전체 Export

```gherkin
기능: 모든 플로우를 JSON 파일로 한 번에 내보내기

  시나리오: 전체 내보내기 버튼 클릭
    주어진 플로우가 5개 존재하고 FlowListPage 툴바가 표시될 때
    만약 "전체 내보내기" 버튼을 클릭하면
    그러면 flowService.exportAllFlows() API가 호출되어야 한다
    그리고 "flows.json" 파일이 브라우저에서 다운로드되어야 한다
    그리고 파일 내용이 5개 플로우 Export 객체의 배열이어야 한다

  시나리오: 플로우가 없는 상태에서 전체 내보내기
    주어진 등록된 플로우가 없을 때
    만약 "전체 내보내기" 버튼을 클릭하면
    그러면 빈 배열([])을 포함하는 "flows.json" 파일이 다운로드되어야 한다
```

### AC-WEB-001-57: 에이전트 단일 Export

```gherkin
기능: 에이전트를 JSON 파일로 내보내기

  시나리오: 에이전트 내보내기 버튼 클릭
    주어진 에이전트 "modbus-001"이 존재할 때
    만약 에이전트의 내보내기 버튼을 클릭하면
    그러면 agentService.exportAgent(id) API가 호출되어야 한다
    그리고 "modbus-001.json" 파일이 브라우저에서 다운로드되어야 한다
    그리고 파일 내용에 "name", "type" 필드가 포함되어야 한다
    그리고 "id", "status", "stats", "uptime", "health" 필드가 포함되지 않아야 한다

  시나리오: config가 있는 에이전트 Export
    주어진 에이전트 "mqtt-bridge"에 config 설정이 있을 때
    만약 해당 에이전트를 내보내면
    그러면 JSON 파일에 "config" 필드가 포함되어야 한다
    그리고 config 내용이 원본과 동일해야 한다
```

### AC-WEB-001-58: 에이전트 전체 Export

```gherkin
기능: 모든 에이전트를 JSON 파일로 한 번에 내보내기

  시나리오: 전체 내보내기 버튼 클릭
    주어진 에이전트가 3개 존재하고 AgentListPage 툴바가 표시될 때
    만약 "전체 내보내기" 버튼을 클릭하면
    그러면 agentService.exportAllAgents() API가 호출되어야 한다
    그리고 "agents.json" 파일이 브라우저에서 다운로드되어야 한다
    그리고 파일 내용이 3개 에이전트 Export 객체의 배열이어야 한다
```

### AC-WEB-001-59: Runtime 필드 제거 검증

```gherkin
기능: Export 데이터에서 런타임 전용 필드가 제거됨

  시나리오: 플로우 Export 런타임 필드 제거
    주어진 running 상태인 플로우가 id, status, stats, created_at, updated_at를 가지고 있을 때
    만약 해당 플로우를 내보내면
    그러면 Export 파일에 다음 필드가 포함되지 않아야 한다: id, status, stats, created_at, updated_at
    그리고 name, definition 필드만 포함되어야 한다

  시나리오: 에이전트 Export 런타임 필드 제거
    주어진 running 상태인 에이전트가 id, status, stats, uptime, health를 가지고 있을 때
    만약 해당 에이전트를 내보내면
    그러면 Export 파일에 다음 필드가 포함되지 않아야 한다: id, status, stats, uptime, health, created_at, updated_at
    그리고 name, type, config 필드만 포함되어야 한다
```

### AC-WEB-001-60: ImportDialog 파일 선택 및 드래그 앤 드롭

```gherkin
기능: ImportDialog에서 파일 선택 및 드래그 앤 드롭

  시나리오: 가져오기 버튼 클릭 시 ImportDialog 열기
    주어진 FlowListPage가 표시된 상태일 때
    만약 "가져오기" 버튼을 클릭하면
    그러면 ImportDialog 모달이 열려야 한다
    그리고 파일 선택기(file picker)가 표시되어야 한다
    그리고 드래그 앤 드롭 영역이 표시되어야 한다

  시나리오: 파일 선택기에서 파일 선택
    주어진 ImportDialog가 열린 상태일 때
    만약 파일 선택기에서 "modbus-flow.json" 파일을 선택하면
    그러면 파일이 파싱되어야 한다
    그리고 미리보기 영역에 파싱 결과가 표시되어야 한다

  시나리오: 드래그 앤 드롭으로 파일 추가
    주어진 ImportDialog가 열린 상태일 때
    만약 "flows.json" 파일을 드래그 앤 드롭 영역에 드롭하면
    그러면 파일이 파싱되어야 한다
    그리고 미리보기 영역에 파싱 결과가 표시되어야 한다

  시나리오: 지원하지 않는 확장자 파일 거부
    주어진 ImportDialog가 열린 상태일 때
    만약 ".txt" 확장자 파일을 선택하면
    그러면 파일 선택기 필터에 의해 거부되어야 한다
    그리고 ".json", ".yaml", ".yml" 확장자만 허용되어야 한다
```

### AC-WEB-001-61: JSON/YAML 파일 파싱 및 자동 감지

```gherkin
기능: 파일 확장자 기반 JSON/YAML 자동 감지 및 파싱

  시나리오: JSON 파일 파싱
    주어진 ".json" 확장자의 유효한 플로우 Export 파일이 있을 때
    만약 ImportDialog에서 해당 파일을 선택하면
    그러면 JSON.parse로 파싱되어야 한다
    그리고 미리보기에 플로우 이름과 설명이 표시되어야 한다

  시나리오: YAML 파일 파싱
    주어진 ".yaml" 확장자의 유효한 에이전트 Export 파일이 있을 때
    만약 ImportDialog에서 해당 파일을 선택하면
    그러면 js-yaml의 yaml.load로 파싱되어야 한다
    그리고 미리보기에 에이전트 이름과 타입이 표시되어야 한다

  시나리오: .yml 확장자 파일 파싱
    주어진 ".yml" 확장자의 유효한 플로우 Export 파일이 있을 때
    만약 ImportDialog에서 해당 파일을 선택하면
    그러면 YAML로 파싱되어야 한다
    그리고 미리보기가 정상 표시되어야 한다

  시나리오: 잘못된 JSON 파일 에러 표시
    주어진 구문 오류가 있는 JSON 파일이 있을 때
    만약 ImportDialog에서 해당 파일을 선택하면
    그러면 파싱 에러 메시지가 표시되어야 한다
    그리고 확인 버튼이 비활성화되어야 한다
```

### AC-WEB-001-62: ImportDialog 미리보기 및 이름 편집

```gherkin
기능: ImportDialog에서 파싱 결과 미리보기 및 이름 편집

  시나리오: 단일 플로우 파일 미리보기
    주어진 단일 플로우 Export JSON 파일을 ImportDialog에서 선택했을 때
    그러면 미리보기 영역에 플로우 이름이 표시되어야 한다
    그리고 설명(description)이 있으면 표시되어야 한다
    그리고 편집 가능한 이름 입력 필드가 표시되어야 한다

  시나리오: 배열 파일 리스트 미리보기
    주어진 3개 플로우를 포함하는 배열 JSON 파일을 ImportDialog에서 선택했을 때
    그러면 미리보기 영역에 3개 항목이 리스트로 표시되어야 한다
    그리고 각 항목에 편집 가능한 이름 입력 필드가 표시되어야 한다

  시나리오: 이름 편집 후 가져오기
    주어진 플로우 이름이 "original-name"인 Export 파일의 미리보기가 표시될 때
    만약 이름을 "new-name"으로 편집하고 확인 버튼을 클릭하면
    그러면 POST /flows API가 이름 "new-name"으로 호출되어야 한다

  시나리오: 에이전트 파일 미리보기
    주어진 단일 에이전트 Export JSON 파일을 ImportDialog에서 선택했을 때
    그러면 미리보기에 에이전트 이름과 타입이 표시되어야 한다
    그리고 편집 가능한 이름 입력 필드가 표시되어야 한다
```

### AC-WEB-001-63: Import 유효성 검사

```gherkin
기능: Import 파일 유효성 검사

  시나리오: 플로우 필수 필드 누락 시 에러 표시
    주어진 "definition" 필드가 없는 플로우 JSON 파일을 ImportDialog에서 선택했을 때
    그러면 유효성 에러 메시지가 표시되어야 한다
    그리고 "필수 필드 누락: definition" 형태의 메시지가 포함되어야 한다
    그리고 확인 버튼이 비활성화되어야 한다

  시나리오: 에이전트 필수 필드 누락 시 에러 표시
    주어진 "type" 필드가 없는 에이전트 JSON 파일을 ImportDialog에서 선택했을 때
    그러면 유효성 에러 메시지가 표시되어야 한다
    그리고 "필수 필드 누락: type" 형태의 메시지가 포함되어야 한다
    그리고 확인 버튼이 비활성화되어야 한다

  시나리오: 배열 파일에서 일부 항목 유효성 에러
    주어진 3개 항목 중 2번째 항목에 "name" 필드가 없는 배열 JSON 파일을 선택했을 때
    그러면 2번째 항목에 유효성 에러가 표시되어야 한다
    그리고 1번째와 3번째 항목은 정상 미리보기가 표시되어야 한다
    그리고 유효한 항목만 가져오기 가능해야 한다

  시나리오: 유효한 파일의 확인 버튼 활성화
    주어진 필수 필드가 모두 포함된 플로우 JSON 파일을 ImportDialog에서 선택했을 때
    그러면 유효성 에러가 표시되지 않아야 한다
    그리고 확인 버튼이 활성화되어야 한다
```

### AC-WEB-001-64: Import 확인 및 Create API 호출

```gherkin
기능: Import 확인 시 Create API 호출

  시나리오: 단일 플로우 가져오기
    주어진 유효한 플로우 Export 파일의 미리보기가 표시된 상태일 때
    만약 확인 버튼을 클릭하면
    그러면 POST /flows API가 플로우 데이터와 함께 호출되어야 한다
    그리고 성공 시 "플로우를 가져왔습니다" 토스트가 표시되어야 한다
    그리고 ImportDialog가 닫혀야 한다
    그리고 플로우 목록이 갱신되어야 한다

  시나리오: 배열 플로우 가져오기
    주어진 3개 플로우를 포함하는 배열 파일의 미리보기가 표시된 상태일 때
    만약 확인 버튼을 클릭하면
    그러면 POST /flows API가 각 플로우에 대해 3번 호출되어야 한다
    그리고 성공 시 "3개 플로우를 가져왔습니다" 토스트가 표시되어야 한다

  시나리오: 에이전트 가져오기
    주어진 유효한 에이전트 Export 파일의 미리보기가 표시된 상태일 때
    만약 확인 버튼을 클릭하면
    그러면 POST /agents API가 에이전트 데이터와 함께 호출되어야 한다
    그리고 성공 시 "에이전트를 가져왔습니다" 토스트가 표시되어야 한다
    그리고 에이전트 목록이 갱신되어야 한다

  시나리오: 가져오기 중 로딩 상태 표시
    주어진 확인 버튼을 클릭한 직후
    그러면 확인 버튼에 로딩 스피너가 표시되어야 한다
    그리고 확인 버튼과 취소 버튼이 비활성화되어야 한다
    그리고 파일 선택기가 비활성화되어야 한다
```

### AC-WEB-001-65: Import 이름 충돌 방지

```gherkin
기능: Import 시 이름 충돌 방지

  시나리오: 자동 덮어쓰기 없음
    주어진 "existing-flow"라는 이름의 플로우가 이미 존재할 때
    만약 동일한 이름의 플로우 Export 파일을 가져오기 하면
    그러면 시스템이 기존 플로우를 자동으로 덮어쓰지 않아야 한다
    그리고 API가 중복 이름 에러를 반환해야 한다
    그리고 ImportDialog에 에러 메시지가 표시되어야 한다

  시나리오: 이름 편집 후 재시도
    주어진 중복 이름 에러가 ImportDialog에 표시된 상태일 때
    만약 이름을 "existing-flow-copy"로 편집하고 확인 버튼을 다시 클릭하면
    그러면 POST /flows API가 새 이름으로 호출되어야 한다
    그리고 성공 시 ImportDialog가 닫혀야 한다
```

### AC-WEB-001-66: Import API 에러 처리

```gherkin
기능: Import API 호출 실패 시 에러 처리

  시나리오: API 중복 이름 에러
    주어진 Import 확인 시 API가 409 Conflict(중복 이름)를 반환할 때
    그러면 ImportDialog에 API 응답의 에러 메시지가 표시되어야 한다
    그리고 ImportDialog가 열린 상태를 유지해야 한다
    그리고 사용자가 이름을 수정하고 재시도할 수 있어야 한다

  시나리오: API 유효성 에러
    주어진 Import 확인 시 API가 400 Bad Request(유효성 에러)를 반환할 때
    그러면 ImportDialog에 API 응답의 에러 메시지가 표시되어야 한다
    그리고 ImportDialog가 열린 상태를 유지해야 한다

  시나리오: API 서버 에러
    주어진 Import 확인 시 API가 500 Internal Server Error를 반환할 때
    그러면 ImportDialog에 "서버 오류가 발생했습니다" 에러 메시지가 표시되어야 한다
    그리고 ImportDialog가 열린 상태를 유지해야 한다
    그리고 재시도가 가능해야 한다

  시나리오: 배열 Import에서 부분 실패
    주어진 3개 플로우 배열 Import 중 2번째 항목이 API 에러를 반환할 때
    그러면 1번째 항목은 성공적으로 생성되어야 한다
    그리고 2번째 항목의 에러 메시지가 표시되어야 한다
    그리고 3번째 항목은 계속 시도되어야 한다
```

### AC-WEB-001-67: CLI 호환성

```gherkin
기능: CLI Export 파일과 웹 Import 상호 호환

  시나리오: CLI에서 내보낸 플로우 파일 웹에서 가져오기
    주어진 CLI "xflowd flow export" 명령으로 생성된 JSON 파일이 있을 때
    만약 웹 UI의 ImportDialog에서 해당 파일을 선택하면
    그러면 파일이 정상적으로 파싱되어야 한다
    그리고 미리보기에 플로우 이름과 정보가 표시되어야 한다
    그리고 확인 시 플로우가 정상 생성되어야 한다

  시나리오: 웹에서 내보낸 플로우 파일 CLI에서 가져오기 호환
    주어진 웹 UI에서 내보낸 플로우 JSON 파일이 있을 때
    그러면 파일 구조가 CLI "xflowd flow import" 명령이 기대하는 형식과 동일해야 한다
    그리고 필드 이름이 pkg/flow/serialize.go 출력과 일치해야 한다

  시나리오: CLI에서 내보낸 에이전트 파일 웹에서 가져오기
    주어진 CLI "xflowd agent export" 명령으로 생성된 JSON 파일이 있을 때
    만약 웹 UI의 ImportDialog에서 해당 파일을 선택하면
    그러면 파일이 정상적으로 파싱되어야 한다
    그리고 확인 시 에이전트가 정상 생성되어야 한다
```

### AC-WEB-001-68: FlowListPage/AgentListPage 툴바 UI

```gherkin
기능: 목록 페이지 툴바에 가져오기/내보내기 버튼 표시

  시나리오: FlowListPage 툴바 버튼 표시
    주어진 FlowListPage가 로드된 상태일 때
    그러면 툴바에 "가져오기" 버튼(Upload 아이콘)이 표시되어야 한다
    그리고 "전체 내보내기" 버튼(Download 아이콘)이 표시되어야 한다

  시나리오: AgentListPage 툴바 버튼 표시
    주어진 AgentListPage가 로드된 상태일 때
    그러면 툴바에 "가져오기" 버튼(Upload 아이콘)이 표시되어야 한다
    그리고 "전체 내보내기" 버튼(Download 아이콘)이 표시되어야 한다

  시나리오: FlowActionMenu 내보내기 항목 표시
    주어진 플로우 행의 FlowActionMenu가 열린 상태일 때
    그러면 "내보내기" 메뉴 항목(Download 아이콘)이 표시되어야 한다

  시나리오: Import 성공 후 목록 갱신
    주어진 FlowListPage에서 Import가 성공적으로 완료된 후
    그러면 플로우 목록 쿼리가 무효화(invalidate)되어야 한다
    그리고 새로 가져온 플로우가 목록에 표시되어야 한다
```

---

## 14. 비기능 요구사항

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

## 14. Quality Gate 체크리스트

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
- [ ] Module 8: `computePortsForNode(nodeType, config)` 함수가 nodeSchemas.ts에 추가됨
- [ ] Module 8: 브릿지 노드 direction별 포트가 올바르게 계산됨 (in→output만, out→input만, inout/request_reply→양방향)
- [ ] Module 8: 스위치 노드 라우트 기반 동적 출력 포트가 올바르게 계산됨
- [ ] Module 8: 에디터에서 노드 생성 시 `computePortsForNode`가 호출됨
- [ ] Module 8: 에디터에서 설정 변경 시 포트가 즉시 재계산됨
- [ ] Module 8: 삭제된 포트에 연결된 엣지가 자동 제거됨
- [ ] Module 8: 백엔드 NewNodeDef에서 브릿지 direction 기반 포트 생성됨
- [ ] Module 8: 기존 노드 타입(bridge/switch 외)의 포트가 변경 없이 유지됨
- [ ] Module 8: 기존 저장된 플로우 로드 시 호환성 유지됨
- [ ] Module 9: `PortDef.direction` 타입이 `'input' | 'output' | 'error'`로 확장됨
- [ ] Module 9: `NodeTypeDefinition.ports[].direction` 타입이 `'input' | 'output' | 'error'`로 확장됨
- [ ] Module 9: CustomNode에서 에러 포트가 노드 하단에 빨간색으로 렌더링됨
- [ ] Module 9: NodeHandle에서 에러 포트 핸들이 `bg-red-500`으로 표시됨
- [ ] Module 9: `computePortsForNode()`가 에러 포트를 반환 배열에 포함함
- [ ] Module 9: 에러 포트가 없는 기존 노드가 변경 없이 정상 렌더링됨
- [ ] Module 9: 에러 포트에 엣지 연결 후 저장/로드 시 엣지가 유지됨
- [ ] Module 10: CustomNode Handle의 id 속성이 포트 이름 그대로 사용됨 (접두사 없음)
- [ ] Module 10: `flowToReactFlowConfig`에서 sourceHandle/targetHandle에 접두사 추가 로직이 없음
- [ ] Module 10: `normalizeReactFlowDefinition`에서 TrimPrefix 등 접두사 제거 로직이 없음
- [ ] Module 10: PropertyPanel Port 타입이 'input', 'output', 'error' direction을 모두 지원함
- [ ] Module 10: PropertyPanel 포트 삭제 시 포트 이름으로 직접 비교하여 엣지 정리됨
- [ ] Module 11: FlowListPage 기본 정렬이 이름 오름차순으로 적용됨
- [ ] Module 11: AgentListPage 기본 정렬이 이름 오름차순으로 적용됨
- [ ] Module 11: FlowDetailPanel 노드 인스턴스 기본 정렬이 이름 오름차순으로 적용됨
- [ ] Module 11: 정렬 가능 컬럼 헤더에 정렬 인디케이터가 표시됨
- [ ] Module 11: 같은 컬럼 클릭 시 오름차순/내림차순 토글이 동작함
- [ ] Module 11: 다른 컬럼 클릭 시 해당 컬럼 오름차순으로 변경됨
- [ ] Module 11: 백엔드 `parseSortParam` 기본값이 ("name", asc)를 반환함
- [ ] Module 11: FlowServiceAdapter/AgentServiceAdapter에서 sort 파라미터 기반 정렬이 동작함
- [x] Module 12: FlowPanel 상단에 플로우 상태별 건수(FlowStatusBadge)가 표시됨
- [x] Module 12: FlowPanel 하단에 플로우 리스트 테이블(이름, 상태, 노드 수, 동작 시간, 액션)이 표시됨
- [x] Module 12: FlowPanel 이름 컬럼에 SortableHeader가 적용되고 정렬 토글이 동작함
- [x] Module 12: FlowPanel 이름 클릭 시 에디터 페이지로 이동함
- [x] Module 12: FlowPanel 액션 버튼(Play/Pause)이 flowService API를 호출함
- [x] Module 12: FlowPanel 최대 10행 표시 및 11개 이상 시 "더 보기" 링크가 /flows로 이동함
- [x] Module 12: AgentPanel 상단에 에이전트 상태별 건수(AgentStatusBadge)가 표시됨
- [x] Module 12: AgentPanel 하단에 에이전트 리스트 테이블(이름, 타입, 상태, 업타임, 메시지 IN/OUT, 액션)이 표시됨
- [x] Module 12: AgentPanel 이름 정렬 및 최대 10행 + "더 보기" 링크가 /agents로 이동함
- [x] Module 12: DashboardPage 레이아웃이 데스크톱 2열(FlowPanel + AgentPanel) + ResourceWidget 구조임
- [x] Module 12: DashboardPage 모바일에서 세로 스택 레이아웃으로 전환됨
- [x] Module 12: SystemStatusWidget, RecentFlowsWidget, AgentStatusWidget이 삭제되고 import 참조가 없음
- [x] Module 12: 기존 위젯이 표시하던 모든 정보가 새 패널에서 누락 없이 표시됨
- [x] Module 12: ResourceWidget이 기존과 동일하게 CPU/메모리 게이지를 표시함
- [x] Module 13: 백엔드 `GET /flows/{id}/export` 엔드포인트가 런타임 필드를 제거한 플로우 정의를 반환함
- [x] Module 13: 백엔드 `GET /flows/export` 엔드포인트가 전체 플로우 배열을 반환함
- [x] Module 13: `downloadJSON(data, filename)` 유틸리티가 Blob + URL.createObjectURL로 JSON 파일을 다운로드함
- [x] Module 13: `parseImportFile(file)` 유틸리티가 확장자 기반 JSON/YAML 자동 감지로 파싱함
- [x] Module 13: `validateFlowImport(data)` 유틸리티가 name/definition 필수 필드를 검증함
- [x] Module 13: `validateAgentImport(data)` 유틸리티가 name/type 필수 필드를 검증함
- [x] Module 13: ImportDialog에서 파일 선택기와 드래그 앤 드롭이 동작함
- [x] Module 13: ImportDialog에서 파싱 결과 미리보기와 이름 편집이 동작함
- [x] Module 13: ImportDialog에서 유효성 에러 시 확인 버튼이 비활성화됨
- [x] Module 13: ImportDialog에서 Import 성공 시 토스트 + 목록 갱신이 동작함
- [x] Module 13: ImportDialog에서 API 에러 시 에러 메시지 표시 + 재시도 가능함
- [x] Module 13: `flowService.exportFlow(id)` / `exportAllFlows()` 함수가 Export API를 호출함
- [x] Module 13: `agentService.exportAgent(id)` / `exportAllAgents()` 함수가 Export API를 호출함
- [x] Module 13: FlowActionMenu에 "내보내기" 메뉴 항목이 추가됨
- [x] Module 13: FlowListPage 툴바에 "가져오기"/"전체 내보내기" 버튼이 표시됨
- [x] Module 13: AgentListPage 툴바에 "가져오기"/"전체 내보내기" 버튼이 표시됨
- [x] Module 13: Export 파일에서 런타임 필드(id, status, stats, timestamps)가 제거됨
- [x] Module 13: CLI에서 내보낸 파일을 웹 ImportDialog에서 가져오기 가능함
- [x] Module 13: `js-yaml` 의존성이 package.json에 추가됨
- [ ] ESLint 경고 0건

---

## 15. Definition of Done

- [x] Module 1 (P0): 에이전트 목록 페이지에서 stats 데이터가 정상 표시됨 (백엔드 + 프론트엔드 수정)
- [x] Module 2 (P1): 3개 API 엔드포인트가 구현되고 테스트 통과
- [x] Module 3 (P1): 에이전트 상세 패널 + 설정 페이지 UI 구현 완료
- [x] Module 4 (P1): 플로우 확장 패널 + In/Out 분리 표시 + 실시간 갱신 + 로그 레벨 UI 구현 완료
- [x] Module 5 (P1): 캔버스 RuntimeStatsContext + CustomNode 런타임 상태/In/Out 표시 구현 완료
- [x] BF (P0): engine.go 포트 카운터 초기화 + bridge.go 버그 수정 완료
- [x] Module 7 (P1): LogEntry 확장 + source 배지/component 표시 + 소스 필터(멀티 셀렉트) + 컴포넌트 검색(디바운스) 구현 완료
- [ ] Module 8 (P0): 동적 포트 시스템 구현 완료 — computePortsForNode 함수 + 브릿지 direction 포트 + 스위치 라우트 포트 + 설정 변경 시 포트 재계산 + 엣지 자동 정리 + 백엔드 NewNodeDef 브릿지 방향 인식
- [ ] Module 9 (P0): 에러 포트 타입 지원 구현 완료 — PortDef/NodeTypeDefinition 타입 확장 + CustomNode 에러 포트 하단 빨간색 렌더링 + NodeHandle 에러 포트 색상 + computePortsForNode 에러 포트 포함 + 기존 노드 호환성 유지
- [ ] Module 10 (P0): Handle ID 접두사 제거 구현 완료 — CustomNode Handle ID 포트 이름 직접 사용 + flowToReactFlowConfig/normalizeReactFlowDefinition 접두사 변환 제거 + PropertyPanel 에러 포트 direction 지원 + 포트 삭제 시 포트 이름 직접 비교 엣지 정리
- [ ] Module 11 (P1): 리스트 정렬 기능 구현 완료 — FlowListPage/AgentListPage/FlowDetailPanel 기본 이름 오름차순 정렬 + 정렬 가능 컬럼 헤더 인디케이터 + 정렬 토글 동작 + 백엔드 parseSortParam 및 ServiceAdapter 정렬 처리
- [x] Module 12 (P1): 대시보드 패널 재구성 구현 완료 — FlowPanel(상태 요약 + 리스트 테이블 + 이름 정렬 + Start/Stop 액션 + 더 보기 링크) + AgentPanel(상태 요약 + 리스트 테이블 + 이름 정렬 + 더 보기 링크) + DashboardPage 3패널 반응형 레이아웃 + 기존 3개 위젯 삭제 + 데이터 무결성 검증
- [ ] Module 13 (P2): Import/Export 기능 구현 완료 — 백엔드 플로우 Export API(GET /flows/{id}/export, GET /flows/export) + downloadJSON 유틸 + importParser(JSON/YAML 자동 감지, 유효성 검사) + ImportDialog 공용 모달(파일 선택, 드래그 앤 드롭, 미리보기, 이름 편집, 유효성 에러, 로딩 상태, API 에러 처리) + flowService/agentService Export 함수 + FlowActionMenu 내보내기 항목 + FlowListPage/AgentListPage 가져오기/전체 내보내기 툴바 버튼 + js-yaml 의존성 + CLI 호환 포맷
- [x] 기존 글로벌 로그 레벨 기능이 정상 동작 (회귀 없음)
- [x] 모든 신규 백엔드 핸들러에 단위 테스트 존재
- [x] 프론트엔드 TypeScript 컴파일 에러 없음
- [x] Vite 프로덕션 빌드 성공

---

*SPEC ID: SPEC-WEB-001*
*버전: 1.9.0*
*상태: in_progress*
*최종 수정: 2026-03-10*
