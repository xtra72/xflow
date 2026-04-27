---
id: SPEC-AGENT-004
version: "2.0.0"
status: in-progress
created: "2026-04-07"
updated: "2026-04-07"
---

# SPEC-AGENT-004: Agent Statistics Enhancement - 인수 기준

---

## AC-R1: 외부/내부 메시지 통계 분리 — ✅ 통과

### AC-R1-01: 외부 메시지 수신 카운터 증가 ✅

```gherkin
Given 에이전트가 Running 상태이다
When 외부 트랜스포트를 통해 메시지 1건이 수신된다
Then ExternalMessagesReceived가 1 증가한다
And MessagesReceived가 1 증가한다 (합산 일관성)
```

검증: `IncrExternalMessagesReceived()`가 두 카운터를 동시에 증가. LGCP `captureLoop`에서 호출.

### AC-R1-02: 외부 메시지 송신 카운터 증가 ✅

```gherkin
Given 에이전트가 Running 상태이다
When 외부 트랜스포트를 통해 메시지 1건이 송신된다
Then ExternalMessagesSent가 1 증가한다
And MessagesSent가 1 증가한다 (합산 일관성)
```

검증: `IncrExternalMessagesSent()`가 두 카운터를 동시에 증가. LGCP `sendFrame`에서 호출.

### AC-R1-03: 외부 메시지 에러 카운터 증가 ✅

```gherkin
Given 에이전트가 Running 상태이다
When 외부 트랜스포트 메시지 처리 중 에러가 발생한다
Then ExternalMessagesErrored가 1 증가한다
And MessagesErrored가 1 증가한다 (합산 일관성)
```

### AC-R1-04: 내부 메시지 송신 카운터 증가 ✅

```gherkin
Given 에이전트가 Running 상태이다
When 내부 노드 참조를 통해 메시지가 전달된다
Then InternalMessagesSent가 전달 수만큼 증가한다
And MessagesSent가 전달 수만큼 증가한다 (합산 일관성)
```

검증: `processGetRecent` — `AddInternalMessagesSent(n)` (last_seq 필터 통과 프레임 수만).
검증: `ReceiveMessage` — `IncrInternalMessagesSent()` (bridge 전달 1건).
검증: `processControlCommand` — `IncrInternalMessagesSent()` (제어 응답 1건).

### AC-R1-05: 합산 일관성 검증 ✅

```gherkin
Given 에이전트가 외부 메시지 N건, 내부 메시지 M건을 처리했다
Then MessagesReceived == ExternalMessagesReceived (LGCP는 내부 수신 없음)
And MessagesSent == ExternalMessagesSent + InternalMessagesSent
```

검증: `Incr*External*`/`Incr*Internal*` 메서드가 총 카운터를 내부적으로 동시 증가.
주의: `IncrMessages*()` 와 동시 호출 시 이중 카운트 — 이 패턴은 코드에서 제거됨.

### AC-R1-06: 동시성 안전성 ✅

```gherkin
Given 에이전트에 다수의 고루틴이 동시에 메시지를 송수신한다
When go test -race 플래그로 테스트를 실행한다
Then data race가 발생하지 않는다
And 모든 카운터의 합산이 일치한다
```

검증: `go test -race ./internal/agent/lg/...` 통과.

---

## AC-R2: 신규 운영 카운터 — ✅ 통과

### AC-R2-01: DroppedMessages 카운터 ✅

```gherkin
Given 에이전트의 메시지 버퍼(msgCh)가 가득 찬 상태이다
When 새 메시지가 도착한다
Then 가장 오래된 메시지가 드롭되고 DroppedMessages가 1 증가한다
```

검증: `sendFrameEvent` 에서 msgCh 풀 시 oldest 드롭 + `IncrDroppedMessages()` 호출.
테스트: `TestLGCPAgent_MsgChDrop`.

### AC-R2-02: LoadTime 기록 ✅

```gherkin
Given 에이전트가 시작된 직후이다 (SetStartedAt 호출됨)
When 첫 번째 메시지가 수신된다
Then LoadTime이 time.Since(startedAt)으로 기록된다
And LoadTime은 0보다 크다
```

검증: `Start()`에서 `SetStartedAt(time.Now())`, `captureLoop`에서 `RecordFirstMessage()` 호출.

### AC-R2-03: LoadTime 중복 기록 방지 ✅

```gherkin
Given 에이전트의 LoadTime이 이미 기록되었다
When 추가 메시지가 수신된다
Then LoadTime은 변경되지 않는다 (최초 1회만 기록)
```

검증: `RecordFirstMessage()` 내부에서 `sync.Once` 패턴으로 1회만 실행.

---

## AC-R3: ConnectionStats — 미검증

### AC-R3-01: ConnectionStatsProvider 구현 에이전트

```gherkin
Given 에이전트가 ConnectionStatsProvider 인터페이스를 구현한다
When ConnectionStats()를 호출한다
Then 연결 단위별 통계 슬라이스가 반환된다
```

상태: 미구현

### AC-R3-02: ConnectionStatsProvider 미구현 에이전트

```gherkin
Given 에이전트가 ConnectionStatsProvider 인터페이스를 구현하지 않는다
When API를 통해 통계를 조회한다
Then connections 필드는 빈 배열 []이다
```

상태: 미구현

---

## AC-R4: NodeRefStats — ✅ 통과 (LGCP)

### AC-R4-01: 노드별 송신 카운터 ✅

```gherkin
Given 에이전트에 node-A와 node-B가 연결되어 있다
When node-A가 get_recent로 프레임 5건을 수신한다
And node-B가 get_recent로 프레임 3건을 수신한다
Then node-A의 NodeRefStats에 송신 기록이 있다
And node-B의 NodeRefStats에 송신 기록이 있다
```

검증: `processGetRecent`에서 신규 프레임 반환 시 `IncrNodeRefSent(nodeID, flowID)` 호출.

### AC-R4-02: NodeRefStats 동시성 안전성 ✅

```gherkin
Given 다수 노드가 동시에 에이전트에 메시지를 요청한다
When go test -race 플래그로 테스트를 실행한다
Then data race가 발생하지 않는다
```

검증: `sync.RWMutex` 기반 `nodeRefStats` map 보호. `-race` 테스트 통과.

---

## AC-R5: 향상된 통계 API 응답 — 미검증

상태: 미구현

---

## AC-R6: 프론트엔드 Agent Detail Panel — 미검증

상태: 미구현

---

## AC-R7: LGCP 멀티 노드 지원 — ✅ 통과 (신규)

### AC-R7-01: 독립 소비 ✅

```gherkin
Given 동일 LGCP 에이전트에 lgcp-status 노드 2개가 연결되어 있다
When 에이전트가 프레임 10건을 캡처한다
Then 각 노드가 독립적으로 10건의 프레임을 수신할 수 있다
And 한 노드의 소비가 다른 노드에 영향을 주지 않는다
```

검증: 기본 `poll_command`가 `get_recent` (버퍼 비소비). 각 노드가 자신의 `lastSeq` 전달.

### AC-R7-02: last_seq 서버사이드 필터링 ✅

```gherkin
Given 노드가 last_seq=42로 get_recent를 요청한다
When 에이전트 링 버퍼에 seq 40~50 프레임이 있다
Then seq 43~50 프레임만 반환된다
And InternalMessagesSent가 8 증가한다 (반환된 신규 프레임 수)
```

검증: `processGetRecent`에서 `rec.Seq > lastSeq` 조건 필터링 + `AddInternalMessagesSent(n)`.

### AC-R7-03: 빈 폴링 응답 카운트 제외 ✅

```gherkin
Given 노드의 last_seq가 에이전트의 최신 seq와 동일하다
When get_recent를 요청한다
Then 빈 결과가 반환된다
And InternalMessagesSent는 증가하지 않는다
```

검증: `len(result) > 0` 조건에서만 `AddInternalMessagesSent` 호출.

---

## AC-R8: Bridge Guard — ✅ 통과 (신규)

### AC-R8-01: Bridge 소비자 없을 때 msgCh 적재 방지 ✅

```gherkin
Given bridgeActive가 false이다 (ReceiveMessage 미호출)
When 에이전트가 프레임을 캡처한다
Then msgCh에 메시지가 적재되지 않는다
And recentFrames 링 버퍼에는 정상 저장된다
```

검증: 테스트 `TestLGCPAgent_NoBridge_SkipsMsgCh`.

### AC-R8-02: Bridge 소비자 활성화 후 정상 전달 ✅

```gherkin
Given ReceiveMessage가 호출되어 bridgeActive가 true이다
When 에이전트가 프레임을 캡처한다
Then msgCh에 메시지가 정상 적재된다
```

검증: `TestLGCPAgent_CaptureLoop` 등 기존 테스트에서 `bridgeActive.Store(true)` 설정 후 검증.

### AC-R8-03: 상태 이벤트도 Bridge Guard 적용 ✅

```gherkin
Given bridgeActive가 false이다
When 에이전트가 재연결하여 상태 이벤트를 발생시킨다
Then sendStatusEvent가 즉시 반환되어 msgCh에 적재되지 않는다
```

검증: `sendStatusEvent` 첫 줄에서 `bridgeActive.Load()` 체크.

---

## 성능 인수 기준

### AC-PERF-01: 통계 수집 오버헤드

```gherkin
Given 에이전트가 메시지를 처리하는 hot path에서
When 통계 카운터 증가 작업이 수행된다
Then 추가 지연은 1ms 이내이다
```

검증: 모든 카운터가 `sync/atomic` lock-free 연산. 벤치마크는 미실행.

### AC-PERF-02: NodeRefStats 조회 성능

```gherkin
Given 에이전트에 다수의 노드 참조가 있다
When NodeRefStatsSnapshot()를 호출한다
Then 응답 시간은 5ms 이내이다
```

검증: `sync.RWMutex` 읽기 락으로 병행성 확보. 벤치마크는 미실행.

---

## Quality Gate (품질 게이트)

- [x] `go test -race` 통과 (agent, node 패키지)
- [x] 기존 테스트 회귀 없음
- [x] 합산 일관성: ExternalReceived == MessagesReceived (LGCP)
- [x] 합산 일관성: ExternalSent + InternalSent == MessagesSent
- [x] 이중 카운트 제거 검증
- [ ] 전체 테스트 커버리지: 85% 이상 (미측정)
- [ ] `go vet` 경고 없음 (미실행)
- [ ] API 하위 호환성 유지 (미구현)
