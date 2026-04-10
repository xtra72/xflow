---
id: SPEC-NODE-003
version: "1.0.0"
status: draft
created: "2026-04-10"
updated: "2026-04-10"
author: xtra
priority: high
tags: [node, tcp, metadata, connection, framing]
related_spec: SPEC-NODE-002, SPEC-NODE-001, SPEC-SOCKET-001
---

# SPEC-NODE-003: Acceptance Criteria - TCP 소스 노드 connection_id 메타데이터 주입

본 문서는 SPEC-NODE-003 의 인수 기준을 Given-When-Then 형식으로 정의한다. 각 시나리오는 자동화된 Go 테스트로 검증 가능해야 한다.

---

## 1. M1: TCP 서버 모드 connection_id 주입

### AC1.1: 서버 모드에서 connection_id 메타데이터 존재

**Given** `TCPInNode` 가 `ConnAwareReceiver` 를 구현한 TCP 서버 에이전트와 연결된 상태
**And** 에이전트의 `ReceiveMessageFrom` 이 `remoteAddr="192.168.1.100:51234"` 를 반환
**When** receiveLoop 가 메시지를 수신하여 sourceCh 로 전달하면
**Then**:
- 메시지의 메타데이터에 `connection_id` 키가 존재해야 한다
- `connection_id` 의 값은 `"192.168.1.100:51234"` 이어야 한다

### AC1.2: connection_id 와 tcp.remote_addr 동일 값

**Given** AC1.1 과 동일한 설정
**When** 메시지를 수신하면
**Then**:
- `connection_id` 와 `tcp.remote_addr` 의 값이 동일해야 한다 (둘 다 `"192.168.1.100:51234"`)
- 두 키가 모두 메타데이터에 존재해야 한다

### AC1.3: 같은 클라이언트의 연속 메시지는 동일 connection_id

**Given** TCP 서버 에이전트가 동일 클라이언트 (`remoteAddr="10.0.0.1:9999"`) 로부터 3개의 메시지를 순차 수신
**When** receiveLoop 가 3개 메시지를 sourceCh 로 전달하면
**Then**:
- 3개 메시지 모두의 `connection_id` 가 `"10.0.0.1:9999"` 이어야 한다

### AC1.4: 서로 다른 클라이언트는 서로 다른 connection_id

**Given** TCP 서버 에이전트가 두 클라이언트로부터 교대로 메시지를 수신
- 클라이언트 A: `remoteAddr="10.0.0.1:9999"`
- 클라이언트 B: `remoteAddr="10.0.0.2:8888"`
**When** receiveLoop 가 메시지들을 sourceCh 로 전달하면
**Then**:
- 클라이언트 A 에서 온 메시지의 `connection_id` 는 `"10.0.0.1:9999"`
- 클라이언트 B 에서 온 메시지의 `connection_id` 는 `"10.0.0.2:8888"`
- 두 값은 서로 달라야 한다

### AC1.5: 기존 tcp.remote_addr 메타데이터 보존

**Given** AC1.1 과 동일한 설정
**When** 메시지를 수신하면
**Then**:
- `tcp.remote_addr` 이 여전히 `"192.168.1.100:51234"` 로 설정되어야 한다
- `tcp.node_id` 가 노드 ID 로 설정되어야 한다
- `tcp.agent_type` 이 에이전트 타입으로 설정되어야 한다
- 기존 메타데이터가 `connection_id` 추가로 인해 변경되어서는 안 된다

---

## 2. M2: TCP 클라이언트 모드 connection_id 주입

### AC2.1: 클라이언트 모드에서 connection_id 메타데이터 존재

**Given** `TCPInNode` 가 `MessageReceiver` 만 구현한 TCP 클라이언트 에이전트와 연결된 상태
**And** `ConnAwareReceiver` 는 미구현 (connReceiver == nil)
**When** receiveLoop 가 메시지를 수신하여 sourceCh 로 전달하면
**Then**:
- 메시지의 메타데이터에 `connection_id` 키가 존재해야 한다
- `connection_id` 의 값은 `n.ID()` (노드 ID) 와 동일해야 한다

### AC2.2: 클라이언트 모드에서 tcp.remote_addr 미설정 유지

**Given** AC2.1 과 동일한 설정
**When** 메시지를 수신하면
**Then**:
- `tcp.remote_addr` 메타데이터가 존재하지 않아야 한다 (또는 빈 문자열)
- 이는 기존 동작과 동일하다

### AC2.3: 클라이언트 모드 연속 메시지의 일관된 connection_id

**Given** TCP 클라이언트 에이전트가 5개 메시지를 순차 수신
**When** receiveLoop 가 5개 메시지를 sourceCh 로 전달하면
**Then**:
- 5개 메시지 모두의 `connection_id` 가 동일한 값 (`n.ID()`) 이어야 한다

---

## 3. M3: 하위 호환성 및 비파괴성

### AC3.1: 기존 메타데이터 키 불변 (서버 모드)

**Given** connection_id 주입 변경 적용 후 상태
**And** ConnAwareReceiver mock 에이전트 (`remoteAddr="1.2.3.4:5678"`)
**When** TCPInNode 가 메시지를 수신하면
**Then**:
- `tcp.node_id` == 노드 ID (기존과 동일)
- `tcp.remote_addr` == `"1.2.3.4:5678"` (기존과 동일)
- `tcp.agent_type` == 에이전트 타입 (기존과 동일)
- 위 세 키의 값은 connection_id 추가 전후로 변경되지 않아야 한다

### AC3.2: 기존 메타데이터 키 불변 (클라이언트 모드)

**Given** connection_id 주입 변경 적용 후 상태
**And** MessageReceiver mock 에이전트 (클라이언트 모드)
**When** TCPInNode 가 메시지를 수신하면
**Then**:
- `tcp.node_id` == 노드 ID (기존과 동일)
- `tcp.agent_type` == 에이전트 타입 (기존과 동일)
- `tcp.remote_addr` 는 설정되지 않음 (기존과 동일)

### AC3.3: 페이로드 불변

**Given** 에이전트가 `data = []byte{0x01, 0x02, 0x03}` 을 반환
**When** TCPInNode 가 메시지를 생성하면
**Then**:
- `raw` 페이로드는 `[]byte{0x01, 0x02, 0x03}` 이어야 한다
- `data` 페이로드는 `"010203"` (hex) 이어야 한다
- connection_id 추가로 인해 페이로드가 변경되어서는 안 된다

### AC3.4: TCPOutNode 응답 라우팅 불변

**Given** `TCPOutNode` 가 메시지의 `tcp.remote_addr` 를 읽어 특정 클라이언트로 응답 라우팅
**And** 메시지에 `tcp.remote_addr="1.2.3.4:5678"` 과 `connection_id="1.2.3.4:5678"` 이 모두 존재
**When** `TCPOutNode.Process` 를 호출하면
**Then**:
- `tcp.remote_addr` 값으로 라우팅이 수행되어야 한다 (기존 동작)
- `connection_id` 는 무시되어야 한다
- 라우팅 동작이 connection_id 추가 전후로 동일해야 한다

---

## 4. M4: framer 노드와의 통합

### AC4.1: TCP 서버 다중 클라이언트 → framer 연결별 프레이밍

**Given** 플로우: TCPInNode (서버 모드) → FramerNode (`framing=newline`, `stream_key_metadata="connection_id"` 기본값)
**And** 두 클라이언트가 각각 교대로 부분 데이터를 전송:
- 클라이언트 A (remoteAddr="A:1"): msg1=`"hel"`, msg3=`"lo\n"`
- 클라이언트 B (remoteAddr="B:2"): msg2=`"wor"`, msg4=`"ld\n"`
**When** TCPInNode 가 msg1, msg2, msg3, msg4 를 순차 수신하고 framer 노드에 전달하면
**Then**:
- msg1 처리: connection_id="A:1" → 스트림 A 에 "hel" 버퍼링 → 출력 없음
- msg2 처리: connection_id="B:2" → 스트림 B 에 "wor" 버퍼링 → 출력 없음
- msg3 처리: connection_id="A:1" → 스트림 A 에 "lo\n" 추가 → "hello" 프레임 출력
- msg4 처리: connection_id="B:2" → 스트림 B 에 "ld\n" 추가 → "world" 프레임 출력
- 출력 프레임의 `frame.stream_key` 가 각각 `"A:1"`, `"B:2"` 이어야 한다
- 두 스트림의 바이트가 혼합되지 않아야 한다

### AC4.2: TCP 서버 다중 클라이언트 → framer 독립 frame.index

**Given** AC4.1 과 유사하되 각 클라이언트가 여러 프레임을 완성
- 클라이언트 A: `"a1\na2\n"` (2 프레임)
- 클라이언트 B: `"b1\n"` (1 프레임)
**When** framer 노드가 처리하면
**Then**:
- 클라이언트 A 의 두 프레임: `frame.index=0`, `frame.index=1`
- 클라이언트 B 의 한 프레임: `frame.index=0`
- frame.index 가 스트림별 독립이어야 한다

### AC4.3: TCP 클라이언트 → framer 단일 스트림

**Given** 플로우: TCPInNode (클라이언트 모드) → FramerNode (`framing=newline`)
**And** 에이전트가 `"hello\nworld\n"` 바이트 반환
**When** framer 노드가 처리하면
**Then**:
- 2개 프레임 출력 (`"hello"`, `"world"`)
- `frame.stream_key` 가 `n.ID()` (노드 ID) 이어야 한다
- 단일 스트림으로 프레이밍

### AC4.4: framer 의 stream_key_metadata 기본값과 자동 연동

**Given** FramerNode 를 `stream_key_metadata` 옵션 없이 생성 (기본값 `"connection_id"` 사용)
**And** TCPInNode (서버 모드) 가 `connection_id` 메타데이터를 설정
**When** 두 노드를 파이프라인으로 연결하면
**Then**:
- 추가 설정 없이 framer 노드가 `connection_id` 를 스트림 키로 사용해야 한다
- 별도 `stream_key_metadata="tcp.remote_addr"` 지정이 불필요해야 한다

### AC4.5: 바이트 스트림 혼합 방지

**Given** TCP 서버가 3개 클라이언트 (A, B, C) 를 동시 수용
**And** 각 클라이언트가 length_prefix 프레임을 부분적으로 전송 (바이트가 교차 도착)
**When** framer 노드 (`framing=length_prefix`) 가 처리하면
**Then**:
- 각 클라이언트의 프레임이 독립적으로 올바르게 조립되어야 한다
- 한 클라이언트의 length 필드가 다른 클라이언트의 데이터를 참조해서는 안 된다

---

## 5. 비기능 요구사항 검증

### AC5.1: Race condition 없음

**Given** TCPInNode 가 빠르게 메시지를 수신하는 상황
**When** `go test -race ./internal/node/...` 를 실행하면
**Then**:
- data race 가 탐지되지 않아야 한다

### AC5.2: 성능 오버헤드 무시 가능

**Given** connection_id 주입이 활성화된 TCPInNode
**When** 10,000 개의 메시지를 수신하면
**Then**:
- connection_id 미주입 대비 처리 시간 차이가 5% 이내여야 한다
- (메타데이터 Set 한 줄 추가이므로 실질적으로 측정 불가능한 수준)

### AC5.3: 기존 테스트 회귀 없음

**Given** connection_id 변경 적용 후 상태
**When** `go test ./...` 를 실행하면
**Then**:
- 모든 기존 테스트가 통과해야 한다
- 특히 framer 노드 테스트 (`internal/node/framer_test.go`) 의 기존 connection_id 관련 테스트가 통과해야 한다

---

## 6. 엣지 케이스

### AC6.1: remoteAddr 가 빈 문자열인 서버 모드

**Given** ConnAwareReceiver 가 빈 문자열 remoteAddr 를 반환하는 비정상 상황
**When** TCPInNode 가 메시지를 수신하면
**Then**:
- `tcp.remote_addr` 가 설정되지 않아야 한다 (기존 동작: `remoteAddr != ""` 조건)
- `connection_id` 는 클라이언트 모드 폴백으로 `n.ID()` 가 설정되어야 한다

### AC6.2: nil 데이터 수신

**Given** 에이전트가 nil 데이터를 반환
**When** receiveLoop 가 데이터를 수신하면
**Then**:
- 메시지가 sourceCh 로 전달되지 않아야 한다 (기존 동작: `data == nil` 시 continue)
- connection_id 설정 로직이 실행되지 않아야 한다

### AC6.3: 에이전트 타입이 nil

**Given** TCPInNode 의 `n.agent` 가 nil (AgentAccessor 미구현)
**When** 메시지를 수신하면
**Then**:
- `tcp.agent_type` 이 설정되지 않아야 한다 (기존 동작)
- `connection_id` 는 정상적으로 설정되어야 한다
- `tcp.node_id` 도 정상적으로 설정되어야 한다

---

## 7. 완료 검증 체크리스트

- [ ] AC1.x ~ AC6.x 의 모든 시나리오가 자동화된 Go 테스트로 구현되었다
- [ ] 모든 테스트가 `go test -race ./...` 에서 통과한다
- [ ] 기존 framer 노드 테스트 (`framer_test.go`) 가 회귀 없이 통과한다
- [ ] 기존 TCP 노드 테스트 (`tcp_io_test.go`) 가 회귀 없이 통과한다
- [ ] `TCPOutNode` 의 라우팅 동작이 불변이다
- [ ] `go vet ./...` 통과
- [ ] `gofmt -l .` 출력 없음
