---
id: SPEC-AGENT-003
version: "1.0.0"
status: completed
created: "2026-03-06"
updated: "2026-03-06"
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

### Scenario 5: CLI agent get 출력에 Buffer 행 표시

**Given** ModbusAgent(id: "modbus-001")가 Running 상태이고
**And** msgCh에 12개의 메시지가 대기 중이면

**When** `xflow agent get modbus-001` 명령을 실행하면

**Then** Stats 섹션에 `Buffer:       12/256` 형식의 행이 포함되어야 한다

---

### Scenario 6: CLI agent get에서 버퍼 없는 에이전트의 Buffer 행 생략

**Given** BufferInfoProvider를 구현하지 않는 에이전트(id: "timer-001")가 존재하면

**When** `xflow agent get timer-001` 명령을 실행하면

**Then** Stats 섹션에 Buffer 행이 포함되지 않아야 한다

---

### Scenario 7: CLI agent stats 출력에 버퍼 사용률 표시

**Given** InfluxDBAgent(id: "influx-001")가 Running 상태이고
**And** recvCh에 64개의 메시지가 대기 중이고 용량이 256이면

**When** `xflow agent stats influx-001` 명령을 실행하면

**Then** 출력에 `Buffer:      64/256 (25.0%)` 형식의 행이 포함되어야 한다

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

- [ ] `go build ./...` 성공 (컴파일 에러 0)
- [ ] `go vet ./internal/agent/...` 경고 0
- [ ] 기존 테스트 회귀 없음 (`go test ./internal/agent/... -count=1`)
- [ ] 기존 테스트 회귀 없음 (`go test ./internal/api/... -count=1`)
- [ ] 기존 테스트 회귀 없음 (`go test ./internal/cli/... -count=1`)

### 3.2 기능 검증

- [ ] 6개 에이전트 모두 `BufferInfoProvider` 컴파일 타임 체크 통과
- [ ] API 응답에 `buffer_pending`, `buffer_capacity` 필드 포함
- [ ] CLI 출력에 Buffer 행 조건부 표시

### 3.3 역호환성

- [ ] 기존 API 클라이언트가 새 필드 무시 가능 (추가 필드는 파싱 에러 없음)
- [ ] `BufferInfoProvider` 미구현 에이전트는 기존 동작 유지
- [ ] `StatsSnapshot` 필드 추가가 기존 JSON 직렬화/역직렬화에 영향 없음

### 3.4 Definition of Done

- [ ] 모든 REQ-AGENT-003-01 ~ 17 요구사항 구현 완료
- [ ] 위 11개 시나리오 중 최소 Scenario 1, 2, 3, 5, 6, 8 검증 완료
- [ ] `go test -race ./internal/agent/... ./internal/api/... ./internal/cli/...` 통과
- [ ] SPEC 문서(spec.md, plan.md, acceptance.md) 최신 상태로 동기화
