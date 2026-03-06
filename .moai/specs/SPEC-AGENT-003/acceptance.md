---
id: SPEC-AGENT-003
version: "1.1.0"
status: completed
created: "2026-03-06"
updated: "2026-03-07"
author: xtra
---

# SPEC-AGENT-003 Acceptance Criteria

## 1. 인수 기준 개요

SPEC-AGENT-003의 모든 요구사항(REQ-AGENT-003-01 ~ 17)이 충족되었음을 검증하기 위한 인수 테스트 시나리오를 정의한다.

---

## 2. 테스트 시나리오

### Scenario 1: 버퍼 보유 에이전트의 Stats에 버퍼 메트릭 포함

**Given** ModbusAgent가 `MsgChannelSize=256`으로 초기화되어 Running 상태이고
**And** msgCh에 10개의 메시지가 대기 중이면

**When** `agent.Stats()`를 호출하면

**Then** 반환된 `StatsSnapshot.MsgBufferPending`이 10이어야 한다
**And** 반환된 `StatsSnapshot.MsgBufferCapacity`가 256이어야 한다
**And** 기존 필드(MessagesReceived, MessagesSent 등)도 정상적으로 포함되어야 한다

---

### Scenario 2: 버퍼 미보유 에이전트의 기본값 반환

**Given** BufferInfoProvider를 구현하지 않는 에이전트(예: TimerAgent)가 Running 상태이면

**When** `agent.Stats()`를 호출하면

**Then** 반환된 `StatsSnapshot.MsgBufferPending`이 0이어야 한다
**And** 반환된 `StatsSnapshot.MsgBufferCapacity`가 0이어야 한다

---

### Scenario 3: API 상세 조회 응답에 버퍼 메트릭 포함

**Given** ModbusAgent(id: "modbus-001")가 Running 상태이고
**And** msgCh에 5개의 메시지가 대기 중이면

**When** `GET /api/v1/agents/modbus-001?detail=summary` 요청을 보내면

**Then** 응답 JSON의 `stats.buffer_pending`이 5이어야 한다
**And** 응답 JSON의 `stats.buffer_capacity`가 256이어야 한다

---

### Scenario 4: API agent stats 엔드포인트에 버퍼 메트릭 포함

**Given** NASAAgent(id: "nasa-001")가 Running 상태이고
**And** msgCh에 20개의 메시지가 대기 중이면

**When** `GET /api/v1/agents/nasa-001/stats` 요청을 보내면

**Then** 응답 JSON의 `buffer_pending`이 20이어야 한다
**And** 응답 JSON의 `buffer_capacity`가 256이어야 한다

---

### Scenario 5: CLI agent get 출력에 버퍼 메트릭 표시 (구현 방식 변경)

**Given** ModbusAgent(id: "modbus-001")가 Running 상태이고
**And** msgCh에 12개의 메시지가 대기 중이면

**When** `xflow agent get modbus-001` 명령을 실행하면

**Then** Stats 섹션에 `buffer_pending`과 `buffer_capacity` 필드가 DetailFormatter를 통해 자동 렌더링되어야 한다

> 구현 참고: 원래 `Buffer: 12/256` 커스텀 포맷이었으나, CLI의 DetailFormatter가 API DTO 필드를 자동 렌더링하므로 별도 포맷팅 불필요.

---

### Scenario 6: CLI agent get에서 버퍼 없는 에이전트의 기본값 표시

**Given** BufferInfoProvider를 구현하지 않는 에이전트(id: "timer-001")가 존재하면

**When** `xflow agent get timer-001` 명령을 실행하면

**Then** Stats 섹션에 `buffer_pending: 0`, `buffer_capacity: 0`이 표시되어야 한다

> 구현 참고: DetailFormatter는 0값 필드도 표시하므로, Buffer 행 생략 대신 기본값 0이 표시됨.

---

### Scenario 7: (삭제됨 - CLI agent stats 명령 미존재)

> 원래 `xflow agent stats influx-001` 명령에 대한 시나리오였으나, 해당 CLI 명령이 존재하지 않으므로 삭제됨. API `GET /api/v1/agents/{id}/stats` 엔드포인트로 버퍼 메트릭 확인 가능 (Scenario 4 참조).

---

### Scenario 8: 모든 대상 에이전트의 컴파일 타임 인터페이스 체크

**Given** 프로젝트 소스 코드가 빌드 가능한 상태이면

**When** `go build ./...`을 실행하면

**Then** 다음 6개 에이전트의 `BufferInfoProvider` 컴파일 타임 체크가 통과해야 한다:
- `ModbusAgent`
- `ModbusServerAgent`
- `NASAAgent`
- `HTTPReceiverAgent`
- `InfluxDBAgent`
- `MQTTSubscriberAgent`

---

### Scenario 9: 빈 버퍼 상태에서의 메트릭 정확성

**Given** ModbusAgent가 초기화 직후(메시지 미수신) 상태이면

**When** `agent.Stats()`를 호출하면

**Then** `MsgBufferPending`이 0이어야 한다
**And** `MsgBufferCapacity`가 256(기본값)이어야 한다

---

### Scenario 10: 버퍼 가득 찬 상태에서의 메트릭 정확성

**Given** NASAAgent의 msgCh 용량이 256이고
**And** 256개의 메시지가 모두 채워진 상태이면

**When** `agent.Stats()`를 호출하면

**Then** `MsgBufferPending`이 256이어야 한다
**And** `MsgBufferCapacity`가 256이어야 한다
**And** pending == capacity 조건이 성립해야 한다 (100% 사용률)

---

### Scenario 11: Info() 호출 시 버퍼 메트릭 일관성

**Given** ModbusAgent가 Running 상태이고
**And** msgCh에 30개의 메시지가 대기 중이면

**When** `agent.Info()`를 호출하면

**Then** `AgentInfo.Stats.MsgBufferPending`이 30이어야 한다
**And** `AgentInfo.Stats.MsgBufferCapacity`가 256이어야 한다
**And** `agent.Stats()`와 `agent.Info().Stats`의 버퍼 관련 필드가 일관되어야 한다

---

## 3. Quality Gate 기준

### 3.1 코드 품질

- [x] `go build ./...` 성공 (컴파일 에러 0)
- [x] `go vet ./internal/agent/...` 경고 0
- [x] 기존 테스트 회귀 없음 (`go test ./internal/agent/... -count=1`)
- [x] 기존 테스트 회귀 없음 (`go test ./internal/api/... -count=1`)
- [x] 기존 테스트 회귀 없음 (`go test ./internal/cli/... -count=1`)

### 3.2 기능 검증

- [x] 6개 에이전트 모두 `BufferInfoProvider` 컴파일 타임 체크 통과
- [x] API 응답에 `buffer_pending`, `buffer_capacity` 필드 포함
- [x] CLI 출력에 DetailFormatter를 통한 버퍼 필드 자동 렌더링

### 3.3 역호환성

- [x] 기존 API 클라이언트가 새 필드 무시 가능 (추가 필드는 파싱 에러 없음)
- [x] `BufferInfoProvider` 미구현 에이전트는 기존 동작 유지
- [x] `StatsSnapshot` 필드 추가가 기존 JSON 직렬화/역직렬화에 영향 없음

### 3.4 Definition of Done

- [x] REQ-AGENT-003-01 ~ 15, 17 구현 완료 (REQ-16은 CLI 명령 미존재로 삭제)
- [x] Scenario 1, 2, 3, 4, 5, 6, 8, 9, 10, 11 검증 완료 (Scenario 7 삭제)
- [x] `go test ./internal/agent/... ./internal/api/... ./internal/cli/...` 통과
- [x] SPEC 문서(spec.md, plan.md, acceptance.md) v1.1.0으로 동기화
