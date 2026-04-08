---
id: SPEC-AGENT-004
version: "2.0.0"
status: in-progress
created: "2026-04-07"
updated: "2026-04-07"
author: xtra
priority: high
---

# SPEC-AGENT-004: Agent Statistics Enhancement - 에이전트 통계 고도화

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-AGENT-004 |
| 제목 | Agent 통계 고도화 |
| 버전 | 2.0.0 |
| 상태 | in-progress |
| 작성일 | 2026-04-07 |
| 관련 SPEC | SPEC-AGENT-001 (Agent System Framework), SPEC-AGENT-003 (Buffer Metrics) |
| 도메인 | Observability / Agent Statistics |

---

## 1. Environment (환경)

### 1.1 현재 아키텍처

XFlow는 IoT 플로우 엔진으로, Agent가 외부 장치와의 통신을 담당하는 트랜스포트 계층 컴포넌트이다.

**현재 통계 구조 (`StatsSnapshot` in `internal/agent/info.go`):**

- `MessagesReceived` / `MessagesSent` / `MessagesErrored`: 메시지 수 카운터
- `ExternalMessagesReceived` / `ExternalMessagesSent` / `ExternalMessagesErrored`: 외부 메시지 카운터
- `InternalMessagesReceived` / `InternalMessagesSent` / `InternalMessagesErrored`: 내부 메시지 카운터
- `BytesRead` / `BytesWritten`: 바이트 카운터
- `DroppedMessages`: 드롭된 메시지 수
- `LoadTime`: 첫 메시지 수신까지 소요 시간
- `LastActivityAt`: 마지막 활동 시간
- `AvgProcessingLatency`: 평균 처리 지연
- `RestartCount`: 재시작 횟수
- `MsgBufferPending` / `MsgBufferCapacity`: 메시지 버퍼 상태 (SPEC-AGENT-003에서 추가)
- `Extra`: 에이전트별 추가 통계 (`map[string]any`)

**LGCP 에이전트 메시지 전달 모델:**

LGCP 에이전트는 두 가지 메시지 전달 경로를 가진다:

1. **Bridge 경로 (Push)**: `ReceiveMessage()` → `msgCh` 채널 → I/O 노드 (serial_io, mqtt, tcp_io)
   - `bridgeActive` atomic guard로 소비자 없을 때 채널 적재 방지
   - `msgCh` 버퍼 오버플로우 시 oldest 메시지 드롭

2. **Poll 경로 (Pull)**: `Process()` → `get_recent`/`drain` → 링 버퍼 → 폴링 노드 (lgcp-status, lgcp)
   - `last_seq` 서버사이드 필터링으로 신규 프레임만 반환
   - 멀티 노드 독립 소비 지원 (각 노드가 자신의 `lastSeq` 관리)

### 1.2 현재 한계 (v1.0.0 대비 해결된 항목 표시)

1. ~~외부 연결(트랜스포트)과 내부 연결(노드 참조) 메시지를 구분하지 않음~~ → **해결 (R1)**
2. ~~드롭된 메시지, 로드 시간 등 운영 핵심 지표 부재~~ → **해결 (R2)**
3. 에이전트 타입별 외부 연결 단위 통계 없음 (예: MQTT 토픽별, TCP 클라이언트별) → **미구현 (R3)**
4. ~~노드 참조별 내부 통계 없음~~ → **해결 (R4, LGCP 에이전트)**
5. API 응답이 flat 구조로 상세 분석 불가 → **미구현 (R5)**

### 1.3 에이전트 타입별 외부 연결 단위

| 에이전트 타입 | 연결 단위 | 식별자 |
|--------------|----------|--------|
| mqtt-client | 토픽 (Topic) | topic name |
| samsung-nasa | 장치 (Device) | device address |
| serial | 포트 (Port) | serial port path |
| tcp-server | 클라이언트 (Client) | client remote address |
| tcp-client | 서버 (Server) | server address |
| modbus-tcp | 장치 (Device) | unit ID |
| modbus-tcp-server | 클라이언트 (Client) | client remote address |
| lgap | 실내기 (Indoor Unit) | unit address |
| lgcp | 실내기 (Indoor Unit) | unit address |

### 1.4 내부 연결 구조

노드는 `agent_ref` 또는 `AgentRef` 구조체를 통해 에이전트를 참조한다. 하나의 에이전트에 여러 노드가 연결될 수 있으며, 노드별 메시지 송수신 통계가 필요하다.

**멀티 노드 연결 지원**: 동일 에이전트에 다수의 lgcp-status 노드가 연결될 수 있다. 각 노드는 독립적으로 메시지를 소비하며, `last_seq` 기반 서버사이드 필터링으로 중복 없이 신규 프레임만 수신한다.

---

## 2. Assumptions (가정)

1. **하위 호환성**: 기존 `StatsSnapshot` 필드는 유지하며 새 필드를 추가한다. 기존 API 소비자는 영향받지 않는다.
2. **atomic 카운터**: 모든 새 카운터는 `sync/atomic` 패키지를 사용하여 스레드 안전성을 보장한다.
3. **인터페이스 패턴**: 에이전트 타입별 연결 통계는 `ConnectionStatsProvider` 인터페이스를 통해 선택적으로 구현한다.
4. **성능 제약**: 통계 수집은 메시지 처리 경로의 지연을 1ms 이내로 유지해야 한다.
5. **프론트엔드 통합**: React 19 + TypeScript 5.x + Zustand 기반 프론트엔드에서 새 통계 구조를 표시한다.
6. **합산 일관성**: `Incr*External*` / `Incr*Internal*` 메서드가 내부적으로 총 카운터도 함께 증가시켜 이중 호출에 의한 불일치를 방지한다.

---

## 3. Requirements (요구사항)

### R1: 외부/내부 메시지 통계 분리 — ✅ 구현 완료

**WHEN** 에이전트가 외부 트랜스포트를 통해 메시지를 송수신할 때, **THEN** 시스템은 `ExternalMessagesReceived`, `ExternalMessagesSent`, `ExternalMessagesErrored` 카운터를 각각 증가시켜야 한다.

**WHEN** 에이전트가 내부 노드 참조를 통해 메시지를 송수신할 때, **THEN** 시스템은 `InternalMessagesReceived`, `InternalMessagesSent`, `InternalMessagesErrored` 카운터를 각각 증가시켜야 한다.

시스템은 **항상** 기존 `MessagesReceived`, `MessagesSent`, `MessagesErrored` 값을 외부 + 내부 합산으로 유지해야 한다.

**구현 노트:**
- `IncrExternalMessagesReceived()`는 `externalMessagesReceived`와 `messagesReceived`를 동시에 증가
- `IncrInternalMessagesSent()`는 `internalMessagesSent`와 `messagesSent`를 동시에 증가
- `AddInternalMessagesSent(n)`는 벌크 카운트용 (get_recent의 신규 프레임 수)
- 개별 `IncrMessagesReceived()`/`IncrMessagesSent()` 와 함께 호출하면 이중 카운트 발생 — 반드시 한쪽만 사용

### R2: 신규 운영 카운터 추가 — ✅ 구현 완료

시스템은 **항상** 다음 카운터를 추적해야 한다:

- `DroppedMessages`: 버퍼 오버플로우 등으로 드롭된 메시지 수
- `LoadTime`: 에이전트 초기화부터 첫 메시지 수신까지의 소요 시간

**WHEN** 메시지가 버퍼 가득참으로 드롭될 때, **THEN** 시스템은 `DroppedMessages` 카운터를 증가시켜야 한다.

**WHEN** 에이전트가 시작 후 첫 메시지를 수신할 때, **THEN** 시스템은 `LoadTime`을 `time.Since(startedAt)`으로 기록해야 한다.

**구현 노트:**
- `SetStartedAt(time.Now())`를 `Start()`에서 호출
- `RecordFirstMessage()`를 첫 프레임 캡처 시 호출 (이미 기록되었으면 무시)
- LGCP의 `sendFrameEvent`에서 msgCh 오버플로우 시 `IncrDroppedMessages()` 호출

### R3: 에이전트 타입별 외부 연결 통계 (ConnectionStats) — 미구현

**WHERE** 에이전트가 `ConnectionStatsProvider` 인터페이스를 구현하는 경우, 시스템은 연결 단위별 `ConnectionStats`를 제공해야 한다.

각 `ConnectionStats`는 다음을 포함해야 한다:

- `ID`: 연결 식별자 (토픽명, 클라이언트 주소, 장치 주소 등)
- `MessagesReceived` / `MessagesSent` / `MessagesErrored`: 연결별 메시지 카운터
- `BytesRead` / `BytesWritten`: 연결별 바이트 카운터
- `ConnectedAt`: 연결 시작 시각
- `LastActivityAt`: 마지막 활동 시각

### R4: 노드 참조별 내부 통계 (NodeRefStats) — ✅ 구현 완료 (LGCP)

**WHEN** 노드가 에이전트에 메시지를 전송할 때, **THEN** 시스템은 해당 노드 ID를 키로 `NodeRefStats`를 업데이트해야 한다.

각 `NodeRefStats`는 다음을 포함해야 한다:

- `NodeID`: 노드 식별자
- `FlowID`: 플로우 식별자
- `MessagesReceived` / `MessagesSent` / `MessagesErrored`: 노드별 메시지 카운터
- `LastActivityAt`: 마지막 활동 시각

**구현 노트:**
- `IncrNodeRefSent(nodeID, flowID)`: 노드가 에이전트로부터 프레임을 가져갈 때 호출
- LGCP 노드는 `pollRecentBulk`에서 `node_id`와 `last_seq`를 요청에 포함
- 에이전트의 `processGetRecent`에서 신규 프레임 반환 시 `IncrNodeRefSent` 호출

### R5: 향상된 통계 API 응답 구조 — 미구현

**WHEN** 클라이언트가 `GET /api/v1/agents/:id/stats` 엔드포인트를 호출할 때, **THEN** 시스템은 다음 JSON 구조로 응답해야 한다:

```json
{
  "id": "agent-uuid",
  "status": "running",
  "uptime": "2h30m15s",
  "connected": true,
  "messages": {
    "total": { "received": 1000, "sent": 800, "errored": 5 },
    "external": { "received": 600, "sent": 500, "errored": 3 },
    "internal": { "received": 400, "sent": 300, "errored": 2 }
  },
  "bytes": { "read": 102400, "written": 81920 },
  "buffer": { "pending": 5, "capacity": 100 },
  "dropped_messages": 2,
  "load_time": "1.234s",
  "avg_processing_latency": "0.5ms",
  "restart_count": 1,
  "last_activity_at": "2026-04-07T10:30:00Z",
  "connections": [],
  "node_refs": []
}
```

**IF** `connections`가 빈 배열이면(에이전트가 `ConnectionStatsProvider`를 구현하지 않는 경우), **THEN** 시스템은 빈 배열 `[]`을 반환해야 한다.

### R6: 프론트엔드 Agent Detail Panel 업데이트 — 미구현

**WHEN** 사용자가 Agent Detail Panel을 조회할 때, **THEN** 시스템은 다음 정보를 표시해야 한다:

- 외부/내부 메시지 통계 분리 표시
- 드롭된 메시지 수 및 로드 시간
- 연결 단위별 상세 통계 테이블 (connections)
- 노드 참조별 통계 테이블 (node_refs)

### R7: LGCP 멀티 노드 지원 — ✅ 구현 완료 (신규)

**WHEN** 동일 LGCP 에이전트에 다수의 lgcp-status 노드가 연결될 때, **THEN** 각 노드는 독립적으로 신규 프레임만 수신해야 한다.

**구현 방식:**
- 기본 `poll_command`를 `drain`에서 `get_recent`로 변경 (drain은 버퍼를 리셋하여 멀티 노드 비호환)
- 각 노드가 자신의 `lastSeq`를 `last_seq` 필드로 에이전트에 전달
- 에이전트가 `seq > last_seq`인 프레임만 반환 (서버사이드 필터링)
- 노드 쪽에서도 `lastSeq` 기반 방어적 필터링 유지

### R8: Bridge 소비자 관리 — ✅ 구현 완료 (신규)

**WHEN** bridge 소비자(ReceiveMessage 호출자)가 없을 때, **THEN** 에이전트는 msgCh 채널에 메시지를 적재하지 않아야 한다.

**구현 방식:**
- `bridgeActive atomic.Bool` 필드: `ReceiveMessage()` 최초 호출 시 true로 설정
- `handleCapturedFrame`: `bridgeActive`가 false이면 `sendFrameEvent` 생략
- `sendStatusEvent`: `bridgeActive`가 false이면 즉시 반환

---

## 4. Specifications (명세)

### S1: StatsSnapshot 구조체 확장 — ✅ 완료

`internal/agent/info.go`의 `StatsSnapshot`에 다음 필드를 추가한다:

- `ExternalMessagesReceived int64`
- `ExternalMessagesSent int64`
- `ExternalMessagesErrored int64`
- `InternalMessagesReceived int64`
- `InternalMessagesSent int64`
- `InternalMessagesErrored int64`
- `DroppedMessages int64`
- `LoadTime time.Duration`

`AgentStats` 구조체에 대응하는 `atomic.Int64` 필드를 추가하고, `Incr*` / `Add*` 메서드를 제공한다.

**합산 보장 패턴:**
- `IncrExternal*()` 메서드는 내부적으로 총 카운터도 증가
- `IncrInternal*()` 메서드는 내부적으로 총 카운터도 증가
- 별도의 `IncrMessages*()` 호출과 동시 사용 금지 (이중 카운트 방지)

### S2: ConnectionStatsProvider 인터페이스 — 미구현

새 인터페이스 `ConnectionStatsProvider`를 `internal/agent/` 패키지에 정의한다:

- `ConnectionStats` 구조체: ID, MessagesReceived, MessagesSent, MessagesErrored, BytesRead, BytesWritten, ConnectedAt, LastActivityAt
- `ConnectionStatsProvider` 인터페이스: `ConnectionStats() []ConnectionStats` 메서드 1개

에이전트 타입별로 선택적 구현. `agentToHandlerInfo()` 및 `AgentStats()` 함수에서 타입 어설션으로 확인한다.

### S3: NodeRefStats 수집 — ✅ 완료

- `NodeRefStats` 구조체: NodeID, FlowID, MessagesReceived, MessagesSent, MessagesErrored, LastActivityAt
- `AgentStats`에 `sync.RWMutex` + `map[string]*nodeRefStatsEntry`로 노드별 통계 관리
- 키는 `nodeID` 사용
- `IncrNodeRefReceived(nodeID, flowID)` / `IncrNodeRefSent(nodeID, flowID)` / `IncrNodeRefErrored(nodeID, flowID)` 메서드 제공
- `NodeRefStatsSnapshot() []NodeRefStats` 메서드로 읽기 전용 스냅샷 반환

### S4: API 응답 구조 변환 — 미구현

`internal/api/handler/agent.go`에 다음 중첩 구조체를 추가한다:

- `MessageCounters`: received, sent, errored
- `EnhancedMessagesStats`: total, external, internal (각각 `MessageCounters`)
- `BytesStats`: read, written
- `BufferStats`: pending, capacity
- `ConnectionStatsResponse`: id, messages_received, messages_sent 등
- `NodeRefStatsResponse`: node_id, flow_id, messages_received 등

`AgentStatsInfo`를 확장하여 위 구조체를 포함한다.

`internal/api/service/agent_adapter.go`의 `AgentStats()` 메서드에서 새 필드를 매핑한다.

### S5: 성능 고려사항

- 모든 카운터는 `sync/atomic` 사용 (lock-free)
- `ConnectionStats` 조회는 읽기 전용 스냅샷 반환
- `NodeRefStats`는 `sync.RWMutex`로 읽기 병행성 확보
- 통계 수집 오버헤드: 메시지 처리 경로에서 1ms 이내

### S6: LGCP last_seq 서버사이드 필터링 — ✅ 완료 (신규)

**구조 변경:**
- `lgcpFrameRecord`에 `Seq int64` 필드 추가 (링 버퍼에 seq 저장)
- `lgcpProcessRequest`에 `LastSeq int64` 필드 추가
- `pushRecentFrame(eventJSON, ts, seq)`: seq를 링 버퍼에 저장
- `processGetRecent(count, lastSeq, nodeID, flowID)`: `rec.Seq > lastSeq` 필터링
- 노드의 `pollRecentBulk`에서 `last_seq: n.lastSeq` 전송

**카운트 정확성:**
- `processGetRecent`: `lastSeq` 필터 통과한 신규 프레임 수만 `AddInternalMessagesSent(n)` 호출
- `processDrain`: 반환된 전체 프레임 수 카운트 (drain은 소비 모드이므로 중복 없음)
- `ReceiveMessage`: `IncrInternalMessagesSent()` 1회만 호출 (`IncrMessagesSent()` 이중 호출 제거)
- `processControlCommand`: 제어 명령 응답 시 `IncrInternalMessagesSent()` 1회

### S7: bridgeActive Guard — ✅ 완료 (신규)

- `LGCPAgent` 구조체에 `bridgeActive atomic.Bool` 필드
- `ReceiveMessage()` 최초 호출 시 `bridgeActive.Store(true)`
- `handleCapturedFrame`: `bridgeActive.Load()` 체크 후 `sendFrameEvent` 호출
- `sendStatusEvent`: `bridgeActive.Load()` 체크 후 조기 반환

---

## 5. Traceability (추적성)

| 요구사항 | 명세 | 상태 | 주요 영향 파일 |
|---------|------|------|--------------|
| R1 (외부/내부 분리) | S1 | ✅ 완료 | `internal/agent/info.go`, `internal/agent/lg/lgcp_agent.go` |
| R2 (신규 카운터) | S1 | ✅ 완료 | `internal/agent/info.go`, `internal/agent/lg/lgcp_agent.go` |
| R3 (ConnectionStats) | S2 | 미구현 | `internal/agent/agent.go`, 에이전트 타입별 파일 |
| R4 (NodeRefStats) | S3 | ✅ 완료 | `internal/agent/info.go`, `internal/agent/lg/lgcp_agent.go` |
| R5 (API 응답) | S4 | 미구현 | `internal/api/handler/agent.go`, `internal/api/service/agent_adapter.go` |
| R6 (프론트엔드) | - | 미구현 | `web/src/pages/agents/AgentDetailPanel.tsx` |
| R7 (멀티 노드) | S6 | ✅ 완료 | `internal/agent/lg/lgcp_agent.go`, `internal/node/lgcp.go` |
| R8 (Bridge Guard) | S7 | ✅ 완료 | `internal/agent/lg/lgcp_agent.go` |

---

## 6. 구현 이력

### v2.0.0 (2026-04-07)

**LGCP 에이전트 통계 고도화 구현:**

1. `AgentStats` 코어 확장 (R1, R2)
   - 외부/내부 메시지 카운터 분리 (`IncrExternal*`, `IncrInternal*`, `AddInternal*`)
   - `DroppedMessages`, `LoadTime`, `StartedAt` 카운터 추가
   - 합산 일관성 보장 패턴 (`Incr*` 메서드가 총 카운터도 동시 증가)

2. NodeRefStats (R4)
   - `IncrNodeRefSent(nodeID, flowID)` per-node 통계 추적
   - `processGetRecent`에서 신규 프레임 반환 시 호출

3. LGCP 에이전트 통계 연동
   - `captureLoop`: `IncrExternalMessagesReceived()`, `AddBytesRead()`, `UpdateLastActivity()`, `RecordFirstMessage()`
   - `sendFrame`: `IncrExternalMessagesSent()`, `AddBytesWritten()`
   - `processGetRecent`: `AddInternalMessagesSent(n)` (신규 프레임 수만)
   - `processDrain`: `AddInternalMessagesSent(n)` (소비된 프레임 수)
   - `ReceiveMessage`: `IncrInternalMessagesSent()` (bridge 전달)
   - `processControlCommand`: `IncrInternalMessagesSent()` (제어 응답)
   - `sendFrameEvent` drop 시: `IncrDroppedMessages()`
   - `Process` 에러 시: `IncrMessagesErrored()`
   - `Stats()`: `Extra` 필드에 LGCP 고유 통계 (frames_captured, frames_valid, frames_invalid, frames_dropped, bytes_received, transport_connected)

4. 멀티 노드 지원 (R7)
   - 기본 `poll_command`를 `drain` → `get_recent`로 변경
   - `lgcpFrameRecord`에 `Seq` 필드 추가
   - `lgcpProcessRequest`에 `LastSeq` 필드 추가
   - `processGetRecent`에서 `seq > lastSeq` 서버사이드 필터링
   - 노드의 `pollRecentBulk`에서 `last_seq` 전송

5. Bridge Guard (R8)
   - `bridgeActive atomic.Bool` 가드
   - bridge 소비자 없으면 `sendFrameEvent`/`sendStatusEvent` 생략

**수정된 파일:**
- `internal/agent/info.go` — `AddMessagesSent`, `AddInternalMessagesSent` 메서드 추가
- `internal/agent/lg/lgcp_agent.go` — 통계 연동, bridgeActive, last_seq 필터링, 멀티노드
- `internal/agent/lg/lgcp_agent_test.go` — bridgeActive 테스트, 통계 검증, pushRecentFrame seq
- `internal/node/lgcp.go` — 기본 poll_command 변경, last_seq/node_id 전송
- `internal/node/lgcp_test.go` — 기본값 assertion 업데이트
