---
id: SPEC-NODE-003
version: "1.0.0"
status: completed
created: "2026-04-10"
updated: "2026-04-10"
author: xtra
priority: high
tags: [node, tcp, metadata, connection, framing]
related_spec: SPEC-NODE-002, SPEC-NODE-001, SPEC-SOCKET-001
---

# SPEC-NODE-003: TCP 소스 노드의 connection_id 메타데이터 주입

| 항목 | 내용 |
|------|------|
| SPEC ID | SPEC-NODE-003 |
| 제목 | TCP 소스 노드의 connection_id 메타데이터 주입 |
| 버전 | 1.0.0 |
| 상태 | draft |
| 작성일 | 2026-04-10 |
| 작성자 | xtra |
| 우선순위 | high |
| 관련 SPEC | SPEC-NODE-002 (Framer 노드), SPEC-NODE-001 (Node 시스템), SPEC-SOCKET-001 (Socket 에이전트) |
| 도메인 | Node System / TCP Transport / Metadata Injection |

---

## 1. Overview (개요)

### 1.1 배경

SPEC-NODE-002 는 바이트 스트림에서 프로토콜 프레임을 분리하는 `framer` 노드를 도입하였다. framer 노드는 메타데이터 기반 스트림 키 (기본: `connection_id`) 로 내부 버퍼를 분리하여 다중 연결 환경에서도 각 연결별 독립 프레이밍을 수행할 수 있다.

그러나 SPEC-NODE-002 결정 (f) 에서 전략 2 를 선택하여 TCP 서버 소스 노드의 `connection_id` 메타데이터 주입은 별도 SPEC 으로 분리하였다 (spec.md 9.2 항목 5 참조). 현재 상태에서 TCP 서버 소스 노드가 framer 노드와 결합되면, 모든 클라이언트의 바이트가 단일 공용 버퍼로 병합되어 연결별 프레이밍이 불가능하다.

### 1.2 근본 원인 분석

**원인 1 - tcp.remote_addr 와 connection_id 불일치**

현재 `TCPInNode` (internal/node/tcp_io.go) 의 receiveLoop 는 `ConnAwareReceiver` 인터페이스를 통해 `remoteAddr` (host:port) 를 수신하고, 이를 `tcp.remote_addr` 메타데이터로 설정한다 (라인 233-235). 그러나 framer 노드의 기본 `stream_key_metadata` 는 `connection_id` 이므로, `tcp.remote_addr` 이 존재해도 framer 노드는 이를 스트림 키로 인식하지 못한다. 메타데이터 키 이름의 불일치가 문제의 직접적 원인이다.

**원인 2 - 소스 노드의 connection_id 미주입**

`TCPInNode` 은 `tcp.node_id`, `tcp.remote_addr`, `tcp.agent_type` 세 가지 메타데이터만 설정한다. `connection_id` 라는 범용 식별자를 설정하지 않기 때문에 downstream 처리 노드 (framer 등) 가 소스 유형에 무관하게 연결을 식별할 수 없다.

**원인 3 - TCP 클라이언트 모드에서 connection_id 부재**

TCP 클라이언트 모드는 단일 연결이지만, 일관성을 위해 `connection_id` 를 설정하지 않는다. 이는 "serial-in → framer" 와 "tcp-in (client) → framer" 파이프라인 간의 메타데이터 규약이 다른 문제를 야기한다.

### 1.3 제안 기능

1. `TCPInNode` 의 receiveLoop 에서 매 메시지에 `connection_id` 메타데이터를 주입한다.
2. TCP 서버 모드 (ConnAwareReceiver): `connection_id` 값은 `remoteAddr` (host:port) 를 사용한다. 이는 한 클라이언트 연결의 수명 동안 고유하고, 사람이 읽을 수 있는 형태이다.
3. TCP 클라이언트 모드 (MessageReceiver): `connection_id` 값은 고정 문자열 (에이전트 타입 + 노드 ID 기반) 을 사용하여 일관된 메타데이터 구조를 제공한다.
4. 기존 `tcp.remote_addr` 메타데이터는 그대로 유지한다 (하위 호환).
5. `connection_id` 는 framer 노드의 기본 `stream_key_metadata` 와 일치하므로, 추가 설정 없이 "tcp-in (server) → framer" 파이프라인에서 연결별 프레이밍이 자동으로 동작한다.

### 1.4 핵심 원칙

1. **소스 무관 메타데이터 규약**: `connection_id` 는 소스 노드 유형 (serial, tcp, udp) 에 관계없이 "데이터 출처 식별자" 역할을 하는 범용 메타데이터 키이다. 본 SPEC 은 TCP 소스 노드에서 이를 첫 번째로 구현하며, 향후 다른 소스 노드로 확장될 수 있는 패턴을 수립한다.
2. **하위 호환성**: 기존 플로우와 downstream 노드는 `connection_id` 메타데이터가 추가되더라도 영향을 받지 않는다. 메타데이터 추가는 비파괴적 변경이다.
3. **framer 노드와의 즉시 통합**: 별도 설정 없이 framer 노드의 기본 `stream_key_metadata="connection_id"` 와 자동으로 연동되어야 한다.
4. **결정론적 식별자**: `connection_id` 값은 같은 연결에서 오는 모든 메시지에 대해 동일해야 하며, 서로 다른 연결 간에는 고유해야 한다.
5. **최소 변경**: `internal/node/tcp_io.go` 의 receiveLoop 에 메타데이터 설정 1줄 추가가 핵심 변경이다.

---

## 2. Environment (환경)

### 2.1 현재 아키텍처

**TCP 소스 노드 (`internal/node/tcp_io.go`)**

- `TCPInNode`: TCP 에이전트로부터 메시지를 수신하는 SourceNode
- 두 가지 수신 모드:
  - `ConnAwareReceiver` (TCP 서버): `ReceiveMessageFrom(ctx) (data, remoteAddr, err)` - 연결 정보 포함
  - `MessageReceiver` (TCP 클라이언트): `ReceiveMessage(ctx) (data, err)` - 연결 정보 없음
- 현재 설정하는 메타데이터:
  - `tcp.node_id`: 노드 ID
  - `tcp.remote_addr`: remoteAddr (서버 모드, ConnAwareReceiver 경유 시만)
  - `tcp.agent_type`: 에이전트 타입

**TCP 서버 에이전트 (`internal/agent/socket/tcp_server.go`)**

- `TCPServerAgent`: `agent.ConnAwareReceiver` 인터페이스 구현
- `ReceiveMessageFrom`: 내부 `msgCh` 에서 `connMessage{Data, RemoteAddr}` 를 읽어 반환
- `RemoteAddr` 형식: `"host:port"` (예: `"192.168.1.100:51234"`)

**framer 노드 (`internal/node/framer.go`)**

- `resolveStreamKey(msg)`: `msg.Metadata().Get(n.nodeOptions.StreamKeyMetadata)` 로 스트림 키 결정
- 기본 `StreamKeyMetadata`: `"connection_id"`
- 키가 없으면 빈 문자열 (단일 공용 버퍼)

**메시지 메타데이터 (`pkg/message/metadata.go`)**

- `Metadata` 인터페이스: `Get(key) (string, bool)`, `Set(key, value)`, `Has(key) bool`, `All() map[string]string`
- 값은 모두 `string` 타입

### 2.2 영향 범위

**수정 파일**

- `internal/node/tcp_io.go`: `TCPInNode.receiveLoop` 에 `connection_id` 메타데이터 주입 추가

**수정 없음 (Out of Scope)**

- `internal/node/framer.go`: framer 노드 자체는 변경하지 않음 (기본 `stream_key_metadata="connection_id"` 로 이미 준비됨)
- `internal/agent/socket/tcp_server.go`: TCP 서버 에이전트 자체는 변경하지 않음 (이미 `ConnAwareReceiver` 구현)
- `internal/agent/socket/tcp_client.go`: TCP 클라이언트 에이전트 자체는 변경하지 않음
- `internal/node/serial_io.go`: 시리얼 노드의 `connection_id` 주입은 본 SPEC 의 범위 외 (단일 연결이므로 framer 노드가 이미 단일 공용 버퍼로 정상 동작)
- Web UI
- 에이전트 설정 스키마
- `pkg/framing`

### 2.3 현재 코드 분석 (`TCPInNode.receiveLoop`)

```go
// receiveLoop 현재 구현 (tcp_io.go:195-247)
func (n *TCPInNode) receiveLoop() {
    for {
        // ... select stopCh ...
        var data []byte
        var remoteAddr string
        var err error

        if n.connReceiver != nil {
            data, remoteAddr, err = n.connReceiver.ReceiveMessageFrom(ctx)
        } else {
            data, err = n.receiver.ReceiveMessage(ctx)
        }
        // ... error handling ...

        msg := message.New()
        msg.Payload().Set("raw", data)
        msg.Payload().Set("data", hex.EncodeToString(data))
        msg.Metadata().Set("tcp.node_id", n.ID())

        if remoteAddr != "" {
            msg.Metadata().Set("tcp.remote_addr", remoteAddr)
        }
        // *** connection_id 는 여기에 없다 ***

        if n.agent != nil {
            msg.Metadata().Set("tcp.agent_type", n.agent.Type())
        }
        // ... send to sourceCh ...
    }
}
```

변경 포인트: `tcp.remote_addr` 설정 직후에 `connection_id` 를 설정하면 된다.

---

## 3. Assumptions (가정)

1. TCP 서버 에이전트의 `ReceiveMessageFrom` 이 반환하는 `remoteAddr` 는 한 클라이언트 연결의 수명 동안 일정하다 (TCP 커넥션의 원격 주소는 변경되지 않음).
2. 서로 다른 클라이언트 연결은 서로 다른 `remoteAddr` (host:port) 를 가진다. 단, NAT 뒤에서 포트가 재사용되는 경우 이전 연결과 동일한 `remoteAddr` 가 나타날 수 있으나, 이전 연결은 이미 종료된 상태이므로 framer 노드의 idle timeout 이 처리한다.
3. TCP 클라이언트 모드에서는 단일 연결만 존재하므로 `connection_id` 값의 고유성은 노드 인스턴스 내에서만 요구된다.
4. `connection_id` 메타데이터가 추가되어도 기존 downstream 노드 (output, transform, filter, debug 등) 는 이를 무시하고 정상 동작한다.
5. framer 노드의 기본 `stream_key_metadata` 는 `"connection_id"` 이며 이는 SPEC-NODE-002 에서 확정되었고 변경되지 않는다.

---

## 4. Requirements (요구사항 - EARS Format)

### M1: TCP 서버 모드 connection_id 주입

**R1.1 (Event-Driven)**: WHEN `TCPInNode` 가 `ConnAwareReceiver` 인터페이스를 통해 데이터와 `remoteAddr` 를 수신하면, THEN 시스템은 생성되는 메시지의 메타데이터에 `connection_id` 키로 `remoteAddr` 값을 설정해야 한다.

**R1.2 (Ubiquitous)**: 시스템은 항상 `connection_id` 의 값이 `ConnAwareReceiver.ReceiveMessageFrom` 이 반환한 `remoteAddr` 와 정확히 동일한 문자열이어야 한다 (예: `"192.168.1.100:51234"`).

**R1.3 (Ubiquitous)**: 시스템은 항상 같은 클라이언트 연결에서 오는 모든 메시지에 대해 동일한 `connection_id` 값을 설정해야 한다.

**R1.4 (Ubiquitous)**: 시스템은 항상 서로 다른 클라이언트 연결에서 오는 메시지에 대해 서로 다른 `connection_id` 값을 설정해야 한다.

**R1.5 (Ubiquitous)**: 시스템은 항상 기존 `tcp.remote_addr` 메타데이터도 동시에 설정해야 한다 (하위 호환).

---

### M2: TCP 클라이언트 모드 connection_id 주입

**R2.1 (Event-Driven)**: WHEN `TCPInNode` 가 `MessageReceiver` 인터페이스를 통해 데이터를 수신하면 (TCP 클라이언트 모드), THEN 시스템은 생성되는 메시지의 메타데이터에 `connection_id` 키로 고정 식별자를 설정해야 한다.

**R2.2 (Ubiquitous)**: 시스템은 항상 TCP 클라이언트 모드의 `connection_id` 값을 노드 ID 기반 고정 문자열 (예: `n.ID()`) 로 설정해야 한다. 이는 한 노드 인스턴스의 수명 동안 일정하다.

**R2.3 (Ubiquitous)**: 시스템은 항상 TCP 클라이언트 모드에서 `tcp.remote_addr` 메타데이터가 설정되지 않는 기존 동작을 유지해야 한다 (MessageReceiver 는 remoteAddr 를 반환하지 않음).

---

### M3: 하위 호환성 및 비파괴성

**R3.1 (Ubiquitous)**: 시스템은 항상 기존 메타데이터 키 (`tcp.node_id`, `tcp.remote_addr`, `tcp.agent_type`) 를 변경 없이 유지해야 한다.

**R3.2 (Unwanted)**: 시스템은 `connection_id` 메타데이터 추가로 인해 기존 플로우의 관측 가능한 동작 (메시지 페이로드, 기존 메타데이터 값, 라우팅) 을 변경해서는 안 된다.

**R3.3 (Ubiquitous)**: 시스템은 항상 `connection_id` 메타데이터를 무조건 설정해야 한다. 사용자가 이를 비활성화하는 옵션은 제공하지 않는다 (메타데이터 추가는 비파괴적이며, 사용하지 않는 노드는 단순히 무시하면 된다).

---

### M4: framer 노드와의 통합

**R4.1 (State-Driven)**: IF `TCPInNode` (서버 모드) 의 출력이 framer 노드의 입력으로 연결되고 framer 노드의 `stream_key_metadata` 가 기본값 `"connection_id"` 이면, THEN framer 노드는 각 TCP 클라이언트 연결별로 독립된 스트림 버퍼를 유지해야 한다 (추가 설정 불필요).

**R4.2 (Event-Driven)**: WHEN 두 개 이상의 TCP 클라이언트가 동시에 데이터를 전송하면, THEN framer 노드는 각 클라이언트의 바이트 스트림을 독립적으로 프레이밍하여 올바른 프레임을 생성해야 한다.

**R4.3 (Unwanted)**: 시스템은 서로 다른 TCP 클라이언트의 바이트 스트림을 혼합하여 프레이밍해서는 안 된다.

---

## 5. Out of Scope (범위 외)

1. **시리얼 소스 노드의 connection_id 주입**: `SerialInNode` 은 단일 연결이므로 framer 노드가 단일 공용 버퍼로 정상 동작한다. 일관성을 위한 주입은 후속 작업으로 분리한다.
2. **UDP 소스 노드**: `internal/node/udp_io.go` 는 아직 존재하지 않으므로 본 SPEC 의 범위 외이다. UDP 소스 노드 구현 시 `connection_id` (또는 `source_addr`) 패턴을 참고하면 된다.
3. **framer 노드의 stream_key_metadata 기본값 변경**: framer 노드는 이미 `"connection_id"` 를 기본값으로 사용하므로 변경 불필요.
4. **TCP 에이전트 자체의 변경**: `TCPServerAgent`, `TCPClientAgent` 의 내부 구현은 변경하지 않는다. 메타데이터 주입은 노드 계층에서 수행한다.
5. **connection_id 형식 커스터마이징**: `connection_id` 의 형식 (현재: `remoteAddr` 그대로) 을 사용자가 커스터마이징하는 기능은 제공하지 않는다.
6. **연결 라이프사이클 이벤트**: 클라이언트 연결/해제 이벤트를 별도 메시지로 발행하는 기능은 본 SPEC 의 범위 외이다.
7. **Web UI 변경**: 없음.

---

## 6. Non-Functional Requirements (비기능 요구사항)

### 6.1 하위 호환성 (Backward Compatibility)

- **NFR1**: 본 SPEC 적용 후에도 framer 노드 없이 tcp-in 을 사용하는 기존 플로우는 동일하게 동작해야 한다. `connection_id` 메타데이터는 downstream 에서 무시되면 영향 없다.
- **NFR2**: 기존 `tcp.remote_addr` 기반으로 응답 라우팅하는 `TCPOutNode` 의 동작은 불변이어야 한다.

### 6.2 동시성 안전 (Concurrency Safety)

- **NFR3**: `connection_id` 설정은 `receiveLoop` 의 단일 고루틴에서 수행되므로 추가 동시성 문제는 없다. `go test -race ./internal/node/...` 가 통과해야 한다.

### 6.3 성능 (Performance)

- **NFR4**: `connection_id` 메타데이터 설정은 `msg.Metadata().Set(key, value)` 한 줄이므로 오버헤드는 무시할 수 있다.

### 6.4 테스트 커버리지 (Test Coverage)

- **NFR5**: `internal/node/tcp_io.go` 의 변경된 코드 (connection_id 주입 로직) 는 단위 테스트로 커버되어야 한다.
- **NFR6**: "tcp-in (서버) → framer" 통합 시나리오의 테스트가 포함되어야 한다.

---

## 7. Traceability

| 요구사항 | 구현 모듈 | 테스트 | 비고 |
|----------|-----------|--------|------|
| M1 (R1.1~R1.5) | `internal/node/tcp_io.go` (receiveLoop) | `tcp_io_test.go` (서버 모드 connection_id) | TCP 서버 connection_id 주입 |
| M2 (R2.1~R2.3) | `internal/node/tcp_io.go` (receiveLoop) | `tcp_io_test.go` (클라이언트 모드 connection_id) | TCP 클라이언트 connection_id 주입 |
| M3 (R3.1~R3.3) | `internal/node/tcp_io.go` | `tcp_io_test.go` (기존 메타데이터 보존) | 하위 호환성 |
| M4 (R4.1~R4.3) | 교차 의존 (tcp_io.go + framer.go) | `tcp_framer_integration_test.go` | framer 통합 시나리오 |

---

## 8. References (참조)

- **핵심 파일**:
  - `internal/node/tcp_io.go` (390라인): TCPInNode 구현, receiveLoop (라인 195-247)
  - `internal/agent/receiver.go`: `ConnAwareReceiver` 인터페이스 정의
  - `internal/agent/socket/tcp_server.go`: `TCPServerAgent.ReceiveMessageFrom` 구현
  - `internal/node/framer.go`: framer 노드의 `resolveStreamKey`, `defaultStreamKeyMetadata = "connection_id"`
- **메시지 계층**:
  - `pkg/message/message.go`: `Message` 인터페이스
  - `pkg/message/metadata.go`: `Metadata` 인터페이스 (`Get`, `Set`, `Has`, `All`, `Clone`)
- **관련 SPEC**:
  - SPEC-NODE-002: Framer 노드 (본 SPEC 의 전제 조건, 결정 (f) 참조)
  - SPEC-NODE-001: Node 시스템 기본 설계
  - SPEC-SOCKET-001: Socket 에이전트 기본 설계
- **SPEC-NODE-002 참조 포인트**:
  - spec.md 2.3: TCP 서버 connection_id 메타데이터 의존성 설명
  - spec.md 5 항목 6: Out of Scope 에서 본 SPEC 분리 명시
  - plan.md 2.6: 결정 (f) - 전략 2 (별도 SPEC 분리) 선택
  - spec.md 9.2 항목 5: 구현 메모에서 SPEC-NODE-003 후속 SPEC 언급
