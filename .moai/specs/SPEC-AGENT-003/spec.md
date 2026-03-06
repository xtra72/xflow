---
id: SPEC-AGENT-003
version: "1.0.0"
status: completed
created: "2026-03-06"
updated: "2026-03-06"
author: xtra
priority: medium
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-06 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-AGENT-003: Agent Message Buffer Metrics Exposure - 내부 메시지 버퍼 메트릭 노출

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼의 에이전트들은 브릿지 노드와 비동기 통신을 위해 내부 메시지 채널(`msgCh`, `recvCh`)을 버퍼로 사용한다. 현재 `StatsSnapshot` 구조체는 누적 카운터(MessagesReceived, MessagesSent 등)만 제공하며, **실시간 버퍼 점유율**을 노출하지 않는다.

버퍼가 가득 차면(`cap` 도달) 메시지가 조용히 드롭되며 WARN 로그만 남기기 때문에, 운영자가 **드롭 발생 전에 버퍼 압력을 감지**할 방법이 없다. 본 SPEC은 선택적 인터페이스(`BufferInfoProvider`)를 통해 버퍼 깊이(pending)와 용량(capacity) 메트릭을 `StatsSnapshot`에 포함시키고, API 및 CLI 출력에 반영하는 기능을 정의한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**:
  - 코어: `internal/agent/info.go`, `internal/agent/agent.go`
  - 에이전트 구현체: `internal/agent/modbus/`, `internal/agent/modbusserver/`, `internal/agent/samsung/`, `internal/agent/system/`
  - API 레이어: `internal/api/handler/agent.go`, `internal/api/service/agent_adapter.go`
  - CLI 레이어: `internal/cli/agent.go`, `internal/cli/output.go`
- **관련 SPEC**: SPEC-AGENT-001 (Agent System 프레임워크), SPEC-AGENT-002 (Agent Import/Export + Detail View)

### 1.3 설계 원칙

- **선택적 인터페이스 패턴**: 기존 `StatefulAgent`, `MessageReceiver`, `PollingConfigurable`과 동일하게 Go 타입 어설션 기반 opt-in 패턴 적용
- **역호환성**: `StatsSnapshot` 필드 추가는 JSON 직렬화에서 `omitempty` 없이 기본값 0으로 안전하게 노출
- **최소 변경 원칙**: 기존 `Agent` 인터페이스 변경 없이, 별도 인터페이스로 기능 확장
- **실시간성**: `len(ch)`, `cap(ch)` 호출은 Go 런타임에서 O(1) 연산으로 성능 부담 없음

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `BufferInfoProvider` 인터페이스 정의
- `StatsSnapshot` 구조체에 `MsgBufferPending`, `MsgBufferCapacity` 필드 추가
- 버퍼를 보유한 5개 에이전트에 `BufferInfoProvider` 구현
- `BaseAgent.Info()`에서 `BufferInfoProvider` 감지 및 필드 채우기
- API `AgentStatsResponse` DTO에 버퍼 필드 추가
- CLI `agent get`, `agent stats` 출력에 버퍼 메트릭 포함

**OUT OF SCOPE (본 SPEC 범위 외)**:
- 버퍼 크기 동적 조절(auto-scaling) 기능
- 버퍼 압력 기반 알림/알람 시스템
- 히스토리 기반 버퍼 트렌드 분석
- 드롭 카운터 추가 (별도 SPEC으로 분리 가능)

---

## 2. Assumptions (가정)

### 2.1 선행 조건

- SPEC-AGENT-001의 Agent 시스템 프레임워크(`Agent` 인터페이스, `BaseAgent`, `AgentStats`, `StatsSnapshot`)가 구현되어 있다
- SPEC-AGENT-002 Module 6의 상세 조회(`detail=summary|full`) 및 `StatefulAgent` 인터페이스가 구현되어 있다
- 5개 에이전트 구현체가 각각 `msgCh` 또는 `recvCh` 채널을 내부 버퍼로 사용 중이다

### 2.2 기술 가정

- Go의 `len(ch)`는 현재 채널에 대기 중인 요소 수를, `cap(ch)`는 채널 용량을 반환하며, 둘 다 O(1) 연산이고 스레드 안전하다
- 버퍼 메트릭은 조회 시점의 스냅샷이며, 호출 간 값이 변할 수 있다 (eventually consistent)
- `BufferInfoProvider`를 구현하지 않는 에이전트(예: System Agent 중 `timer`, `logger`)는 `MsgBufferPending=0`, `MsgBufferCapacity=0`으로 표시된다
- 기본 버퍼 크기는 256이며, 에이전트별 설정(`MsgChannelSize`, `BufferSize`)으로 변경 가능하다

---

## 3. Requirements (요구사항)

### Module 1: BufferInfoProvider 인터페이스 및 StatsSnapshot 확장 - P0

#### REQ-AGENT-003-01: BufferInfoProvider 인터페이스 정의

시스템은 **항상** `internal/agent/agent.go`에 다음 선택적 인터페이스를 정의해야 한다:

```go
// BufferInfoProvider 는 내부 메시지 버퍼 상태를 노출하는 에이전트의 선택적 인터페이스이다.
// msgCh/recvCh 등 비동기 메시지 채널을 버퍼로 사용하는 에이전트가 구현한다.
type BufferInfoProvider interface {
    BufferInfo() (pending int, capacity int)
}
```

#### REQ-AGENT-003-02: StatsSnapshot 필드 확장

시스템은 **항상** `StatsSnapshot` 구조체에 다음 필드를 포함해야 한다:

| 필드 | 타입 | 설명 |
|------|------|------|
| MsgBufferPending | int | 현재 버퍼에 대기 중인 메시지 수 |
| MsgBufferCapacity | int | 버퍼 최대 용량 |

#### REQ-AGENT-003-03: BaseAgent.Info()에서 BufferInfoProvider 감지

**IF** 에이전트가 `BufferInfoProvider` 인터페이스를 구현하면
**THEN** `BaseAgent.Info()` 호출 시 `BufferInfo()`를 호출하여 `StatsSnapshot.MsgBufferPending`과 `StatsSnapshot.MsgBufferCapacity` 필드를 채워야 한다

**IF** 에이전트가 `BufferInfoProvider` 인터페이스를 구현하지 않으면
**THEN** `MsgBufferPending`과 `MsgBufferCapacity`는 기본값 0을 유지해야 한다

#### REQ-AGENT-003-04: Info() 메서드 확장 패턴

**WHEN** `BaseAgent.Info()`가 `StatsSnapshot`을 생성하면
**THEN** `BufferInfoProvider` 타입 어설션을 통해 구현 여부를 검사하고, 구현된 경우 반환된 값으로 스냅샷 필드를 갱신해야 한다

> 참고: `BaseAgent`는 직접 `BufferInfoProvider`를 알 수 없으므로, 각 에이전트 구현체가 자체 `Info()` 또는 `Stats()` 메서드를 오버라이드하여 `BufferInfo()` 결과를 포함하는 방식도 허용한다.

### Module 2: 에이전트 구현체 BufferInfoProvider 적용 - P0

#### REQ-AGENT-003-05: Modbus 에이전트 구현

**WHEN** `ModbusAgent`가 초기화되면
**THEN** `BufferInfoProvider` 인터페이스를 구현하여 `len(a.msgCh), cap(a.msgCh)`를 반환해야 한다

#### REQ-AGENT-003-06: ModbusServer 에이전트 구현

**WHEN** `ModbusServerAgent`가 초기화되면
**THEN** `BufferInfoProvider` 인터페이스를 구현하여 `len(a.msgCh), cap(a.msgCh)`를 반환해야 한다

#### REQ-AGENT-003-07: Samsung NASA 에이전트 구현

**WHEN** `NASAAgent`가 초기화되면
**THEN** `BufferInfoProvider` 인터페이스를 구현하여 `len(a.msgCh), cap(a.msgCh)`를 반환해야 한다

#### REQ-AGENT-003-08: HTTP Receiver 에이전트 구현

**WHEN** `HTTPReceiverAgent`가 초기화되면
**THEN** `BufferInfoProvider` 인터페이스를 구현하여 `len(a.recvCh), cap(a.recvCh)`를 반환해야 한다

#### REQ-AGENT-003-09: InfluxDB 에이전트 구현

**WHEN** `InfluxDBAgent`가 초기화되면
**THEN** `BufferInfoProvider` 인터페이스를 구현하여 `len(a.recvCh), cap(a.recvCh)`를 반환해야 한다

#### REQ-AGENT-003-10: MQTT Subscriber 에이전트 구현

**WHEN** `MQTTSubscriberAgent`가 초기화되면
**THEN** `BufferInfoProvider` 인터페이스를 구현하여 `len(a.recvCh), cap(a.recvCh)`를 반환해야 한다

#### REQ-AGENT-003-11: 컴파일 타임 인터페이스 체크

시스템은 **항상** 각 에이전트 구현체에 다음 컴파일 타임 체크를 포함해야 한다:

```go
var _ agent.BufferInfoProvider = (*에이전트타입)(nil)
```

### Module 3: API/CLI 출력 반영 - P0

#### REQ-AGENT-003-12: AgentStatsResponse 필드 추가

시스템은 **항상** `handler.AgentStatsResponse` 구조체에 다음 필드를 포함해야 한다:

| 필드 | 타입 | JSON 태그 | 설명 |
|------|------|-----------|------|
| BufferPending | int | `json:"buffer_pending"` | 현재 대기 중인 메시지 수 |
| BufferCapacity | int | `json:"buffer_capacity"` | 버퍼 최대 용량 |

#### REQ-AGENT-003-13: agentToHandlerInfo 변환 반영

**WHEN** `agentToHandlerInfo()` 함수가 `detail=summary` 또는 `detail=full`로 호출되면
**THEN** `StatsSnapshot.MsgBufferPending`과 `StatsSnapshot.MsgBufferCapacity` 값을 `AgentStatsResponse.BufferPending`과 `AgentStatsResponse.BufferCapacity`에 매핑해야 한다

#### REQ-AGENT-003-14: AgentStatsInfo 필드 추가

시스템은 **항상** `handler.AgentStatsInfo` 구조체에 다음 필드를 포함해야 한다:

| 필드 | 타입 | JSON 태그 | 설명 |
|------|------|-----------|------|
| BufferPending | int | `json:"buffer_pending"` | 현재 대기 중인 메시지 수 |
| BufferCapacity | int | `json:"buffer_capacity"` | 버퍼 최대 용량 |

#### REQ-AGENT-003-15: CLI agent get 출력 반영

**WHEN** 사용자가 `xflow agent get <id>` 명령을 실행하고 에이전트가 `BufferInfoProvider`를 구현하면
**THEN** Stats 섹션에 `Buffer: {pending}/{capacity}` 형식으로 버퍼 점유율을 출력해야 한다

**IF** 에이전트가 `BufferInfoProvider`를 구현하지 않으면 (capacity=0)
**THEN** Buffer 항목을 출력하지 않아야 한다

#### REQ-AGENT-003-16: CLI agent stats 출력 반영

**WHEN** 사용자가 `xflow agent stats <id>` 명령을 실행하고 에이전트가 버퍼를 보유하면
**THEN** `Buffer: {pending}/{capacity} ({사용률%})` 형식으로 버퍼 상태를 출력해야 한다

#### REQ-AGENT-003-17: API GET /api/v1/agents/{id}/stats 응답 반영

**WHEN** `GET /api/v1/agents/{id}/stats` 요청이 수신되면
**THEN** 응답에 `buffer_pending`과 `buffer_capacity` 필드를 포함해야 한다

---

## 4. Specifications (명세)

### 4.1 Module 1: BufferInfoProvider 인터페이스 및 StatsSnapshot 확장

#### 4.1.1 인터페이스 정의 위치

- **파일**: `internal/agent/agent.go`
- **위치**: 기존 선택적 인터페이스(`MessageReceiver`, `SubscriberAgent`, `StatefulAgent`, `PollingConfigurable`) 이후에 추가

#### 4.1.2 StatsSnapshot 확장

- **파일**: `internal/agent/info.go`
- `StatsSnapshot` 구조체 끝에 2개 필드 추가:

```go
type StatsSnapshot struct {
    // ... 기존 필드 ...
    MsgBufferPending  int // 현재 메시지 버퍼에 대기 중인 메시지 수
    MsgBufferCapacity int // 메시지 버퍼 최대 용량
}
```

#### 4.1.3 Info() 확장 전략

`BaseAgent.Info()`는 구체 에이전트 타입을 알 수 없으므로, 각 에이전트가 자체 `Info()` 메서드에서 `BaseAgent.Info()` 결과를 받은 뒤 `BufferInfo()` 값으로 보강하는 패턴을 사용한다.

또는, `agentToHandlerInfo()` 어댑터 레이어에서 `BufferInfoProvider` 타입 어설션으로 감지하여 DTO에 직접 매핑하는 방법도 가능하다. 구현 시 두 패턴 중 코드 중복이 적은 방안을 선택한다.

**권장 패턴**: 각 에이전트의 `Stats()` 메서드에서 `AgentStats.Snapshot()` 결과에 `BufferInfo()` 값을 추가하여 반환. 이 방식이 `Info()` 내부에서도 `Stats()` 호출로 일관되게 동작한다.

### 4.2 Module 2: 에이전트 구현체 적용

#### 4.2.1 대상 에이전트 및 채널 매핑

| 에이전트 | 파일 | 채널 필드 | 채널 타입 | 기본 용량 |
|----------|------|-----------|-----------|-----------|
| ModbusAgent | `internal/agent/modbus/agent.go` | `msgCh` | `chan []byte` | 256 (MsgChannelSize) |
| ModbusServerAgent | `internal/agent/modbusserver/agent.go` | `msgCh` | `chan map[string]any` | 256 (MsgChannelSize) |
| NASAAgent | `internal/agent/samsung/agent.go` | `msgCh` | `chan []byte` | 256 (MsgChannelSize) |
| HTTPReceiverAgent | `internal/agent/system/http_receiver.go` | `recvCh` | `chan []byte` | 256 (BufferSize) |
| InfluxDBAgent | `internal/agent/system/influxdb_agent.go` | `recvCh` | `chan []byte` | 256 (BufferSize) |
| MQTTSubscriberAgent | `internal/agent/system/mqtt_subscriber.go` | `recvCh` | `chan []byte` | 256 (BufferSize) |

#### 4.2.2 구현 패턴 (각 에이전트 공통)

```go
// BufferInfo 는 내부 메시지 버퍼의 현재 상태를 반환한다.
func (a *XxxAgent) BufferInfo() (pending int, capacity int) {
    return len(a.msgCh), cap(a.msgCh)
}
```

각 에이전트의 `Stats()` 메서드를 오버라이드하여 버퍼 정보를 포함:

```go
func (a *XxxAgent) Stats() agent.StatsSnapshot {
    snap := a.stats.Snapshot()
    snap.MsgBufferPending, snap.MsgBufferCapacity = a.BufferInfo()
    return snap
}
```

#### 4.2.3 컴파일 타임 체크

각 에이전트 파일의 기존 인터페이스 체크 블록에 추가:

```go
var _ agent.BufferInfoProvider = (*XxxAgent)(nil)
```

### 4.3 Module 3: API/CLI 출력 반영

#### 4.3.1 handler.AgentStatsResponse 확장

- **파일**: `internal/api/handler/agent.go`

```go
type AgentStatsResponse struct {
    MessagesIn     int64 `json:"messages_in"`
    MessagesOut    int64 `json:"messages_out"`
    Errors         int64 `json:"errors"`
    BufferPending  int   `json:"buffer_pending"`
    BufferCapacity int   `json:"buffer_capacity"`
}
```

#### 4.3.2 handler.AgentStatsInfo 확장

```go
type AgentStatsInfo struct {
    // ... 기존 필드 ...
    BufferPending  int `json:"buffer_pending"`
    BufferCapacity int `json:"buffer_capacity"`
}
```

#### 4.3.3 agentToHandlerInfo 변환 로직

- **파일**: `internal/api/service/agent_adapter.go`
- `detail=summary` 또는 `detail=full` 분기 내 `result.Stats` 설정 시 버퍼 필드 매핑 추가:

```go
result.Stats = &handler.AgentStatsResponse{
    MessagesIn:     info.Stats.MessagesReceived,
    MessagesOut:    info.Stats.MessagesSent,
    Errors:         info.Stats.MessagesErrored,
    BufferPending:  info.Stats.MsgBufferPending,
    BufferCapacity: info.Stats.MsgBufferCapacity,
}
```

#### 4.3.4 AgentStats() 서비스 메서드 반영

- `AgentServiceAdapter.AgentStats()` 메서드에서 `AgentStatsInfo` 생성 시 버퍼 필드 매핑 추가

#### 4.3.5 CLI 출력 형식

`agent get` (detail=summary) Stats 섹션:
```
Stats:
  Messages In:  1,234
  Messages Out: 1,200
  Errors:       5
  Buffer:       12/256
```

`agent stats` 출력:
```
ID:          abc-123
Status:      running
Uptime:      2h30m15s
Messages In: 1,234
Messages Out:1,200
Errors:      5
Connected:   true
Buffer:      12/256 (4.7%)
```

버퍼가 없는 에이전트(capacity=0)는 Buffer 행을 생략한다.

#### 4.3.6 API 응답 예시

```json
{
  "id": "modbus-001",
  "status": "running",
  "uptime": "2h30m15s",
  "messages_in": 1234,
  "messages_out": 1200,
  "error_count": 5,
  "connected": true,
  "buffer_pending": 12,
  "buffer_capacity": 256
}
```

---

## 5. Traceability (추적성)

### 5.1 SPEC 간 참조

| 참조 SPEC | 관계 |
|-----------|------|
| SPEC-AGENT-001 | Agent System 프레임워크 (Agent, BaseAgent, StatsSnapshot 정의) |
| SPEC-AGENT-002 | Agent Detail View (detail=summary/full, StatefulAgent 패턴 참조) |

### 5.2 파일-모듈 매핑

| 파일 | 모듈 | 변경 유형 |
|------|------|----------|
| `internal/agent/agent.go` | Module 1 | `BufferInfoProvider` 인터페이스 추가 |
| `internal/agent/info.go` | Module 1 | `StatsSnapshot` 필드 추가 |
| `internal/agent/modbus/agent.go` | Module 2 | `BufferInfoProvider` 구현, `Stats()` 오버라이드 |
| `internal/agent/modbusserver/agent.go` | Module 2 | `BufferInfoProvider` 구현, `Stats()` 오버라이드 |
| `internal/agent/samsung/agent.go` | Module 2 | `BufferInfoProvider` 구현, `Stats()` 오버라이드 |
| `internal/agent/system/http_receiver.go` | Module 2 | `BufferInfoProvider` 구현, `Stats()` 오버라이드 |
| `internal/agent/system/influxdb_agent.go` | Module 2 | `BufferInfoProvider` 구현, `Stats()` 오버라이드 |
| `internal/agent/system/mqtt_subscriber.go` | Module 2 | `BufferInfoProvider` 구현, `Stats()` 오버라이드 |
| `internal/api/handler/agent.go` | Module 3 | DTO 필드 추가 |
| `internal/api/service/agent_adapter.go` | Module 3 | 변환 로직 확장 |
| `internal/cli/agent.go` | Module 3 | CLI 출력 반영 |

### 5.3 요구사항-모듈 매핑

| 요구사항 ID | 모듈 | 우선순위 |
|------------|------|---------|
| REQ-AGENT-003-01 ~ 04 | Module 1: BufferInfoProvider 인터페이스 및 StatsSnapshot 확장 | P0 |
| REQ-AGENT-003-05 ~ 11 | Module 2: 에이전트 구현체 BufferInfoProvider 적용 | P0 |
| REQ-AGENT-003-12 ~ 17 | Module 3: API/CLI 출력 반영 | P0 |
