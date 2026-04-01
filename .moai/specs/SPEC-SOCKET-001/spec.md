# SPEC-SOCKET-001: TCP/UDP 소켓 통신 에이전트

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SOCKET-001 |
| 제목 | TCP/UDP 소켓 통신 에이전트 및 소켓 브릿지 노드 |
| 생성일 | 2026-04-01 |
| 상태 | Planned |
| 우선순위 | High |
| 담당 | expert-backend |
| 관련 SPEC | SPEC-AGENT-001, SPEC-BRIDGE-001, SPEC-NODE-001 |

---

## 1. 개요 및 동기

### 배경

xflow는 IoT 데이터 파이프라인 플랫폼으로, 다양한 프로토콜(MQTT, HTTP, WebSocket, MODBUS, Samsung NASA 등)을 에이전트로 지원한다. 산업 현장에서 TCP/UDP 소켓 기반의 raw 통신은 가장 기본적이고 범용적인 통신 방식으로, 커스텀 프로토콜 장비, PLC, 센서 게이트웨이 등과의 직접 통신에 필수적이다.

### 목적

TCP/UDP 소켓 통신이 가능한 에이전트를 서버/클라이언트 개별로 구현하여, xflow 플로우에서 임의의 TCP/UDP 기반 장비와 양방향 데이터 교환을 수행할 수 있도록 한다.

### 범위

- TCP Server / TCP Client / UDP Server / UDP Client 4종 에이전트
- 소켓 브릿지 노드 3종 (Input, Output, InputOutput)
- 클라이언트 자동 재연결 기능
- 서버 접속 수 제한 및 접속 관리(조회/차단)

---

## 2. 환경 (Environment)

### 기술 스택

- 언어: Go 1.23+
- 네트워크: Go 표준 라이브러리 `net` 패키지
- 에이전트 프레임워크: `internal/agent/` (BaseAgent, Agent 인터페이스)
- 노드 프레임워크: `internal/node/` (BaseNode, Node 인터페이스, BridgeNode 패턴)
- 생명주기: `pkg/lifecycle/` (BaseLifecycle)
- 메시지: `pkg/message/` (Message 인터페이스)
- 관찰성: `internal/observe/` (ComponentLogger, MetricsCollector)

### 의존성

- Go 표준 라이브러리 `net` 패키지 (외부 라이브러리 미사용)
- `pkg/lifecycle` - 공통 생명주기 관리
- `internal/agent` - Agent 프레임워크
- `internal/node` - Node 프레임워크 (BridgeNode 패턴 참조)
- `internal/observe` - 로깅 및 메트릭

### 제약 조건

- 외부 소켓 라이브러리를 사용하지 않고 Go 표준 `net` 패키지만 사용한다
- 기존 Agent 인터페이스 및 BaseAgent 구조를 준수한다
- 기존 BridgeNode 패턴과 호환되는 노드 구조를 따른다
- CGo 의존성 없이 순수 Go로 구현한다

---

## 3. 가정 (Assumptions)

- A1: 기존 Agent 인터페이스(Init, Start, Stop, Pause, Resume, Health, Process, Configure)를 그대로 활용한다
- A2: 기존 BridgeNode 및 BridgeAdapter 패턴을 활용하여 소켓 에이전트를 플로우에 연결한다
- A3: TCP는 스트림 기반이므로 메시지 프레이밍(구분자/길이 접두사/고정 크기)이 필요하다
- A4: UDP는 데이터그램 기반이므로 별도 프레이밍 없이 패킷 단위로 메시지를 처리한다
- A5: 서버 에이전트는 다중 클라이언트 접속을 동시에 처리해야 한다
- A6: 클라이언트 에이전트는 단일 서버에 대한 연결을 관리한다

---

## 4. 기능 요구사항 (Requirements)

### 4.1 TCP Server 에이전트

**REQ-TCPS-001** [유비쿼터스]
시스템은 **항상** TCP 서버 에이전트(`tcp-server`)를 에이전트 레지스트리에 등록해야 한다.

**REQ-TCPS-002** [이벤트 기반]
**WHEN** TCP 서버 에이전트가 Start되면 **THEN** 설정된 host:port에서 TCP 리스너를 시작하고 클라이언트 연결을 수신 대기해야 한다.

**REQ-TCPS-003** [상태 기반]
**IF** 최대 접속 수(`max_connections`)가 설정되어 있으면 **THEN** 해당 수를 초과하는 새 연결은 즉시 거절해야 한다.

**REQ-TCPS-004** [이벤트 기반]
**WHEN** 새 클라이언트가 연결되면 **THEN** 연결 정보(원격 주소, 연결 시각)를 내부 연결 맵에 등록하고 수신 고루틴을 시작해야 한다.

**REQ-TCPS-005** [이벤트 기반]
**WHEN** 클라이언트로부터 데이터가 수신되면 **THEN** 설정된 프레이밍 방식에 따라 메시지를 분리하고 내부 수신 채널로 전달해야 한다.

**REQ-TCPS-006** [이벤트 기반]
**WHEN** 특정 원격 주소에 대한 차단 요청이 수신되면 **THEN** 해당 연결을 즉시 종료하고 차단 목록에 추가해야 한다.

**REQ-TCPS-007** [비허용 동작]
시스템은 차단 목록에 포함된 원격 주소로부터의 새 연결을 **수락하지 않아야 한다**.

**REQ-TCPS-008** [이벤트 기반]
**WHEN** 연결 정보 조회 요청이 수신되면 **THEN** 현재 활성 연결 목록(원격 주소, 연결 시각, 전송/수신 바이트 수)을 반환해야 한다.

**REQ-TCPS-009** [이벤트 기반]
**WHEN** TCP 서버 에이전트가 Stop되면 **THEN** 모든 클라이언트 연결을 정상 종료(graceful close)하고 리스너를 닫아야 한다.

**REQ-TCPS-010** [상태 기반]
**IF** 에이전트가 Paused 상태이면 **THEN** 새 연결은 거절하되 기존 연결은 유지하고 수신 데이터를 버퍼링해야 한다.

### 4.2 TCP Client 에이전트

**REQ-TCPC-001** [유비쿼터스]
시스템은 **항상** TCP 클라이언트 에이전트(`tcp-client`)를 에이전트 레지스트리에 등록해야 한다.

**REQ-TCPC-002** [이벤트 기반]
**WHEN** TCP 클라이언트 에이전트가 Start되면 **THEN** 설정된 host:port로 TCP 연결을 시도해야 한다.

**REQ-TCPC-003** [이벤트 기반]
**WHEN** 연결이 끊어지면 **THEN** 설정된 재연결 간격(`reconnect_interval`)으로 자동 재연결을 시도해야 한다.

**REQ-TCPC-004** [상태 기반]
**IF** 최대 재시도 횟수(`max_retries`)가 설정되어 있고 해당 횟수를 초과하면 **THEN** 재연결을 중단하고 Error 상태로 전이해야 한다.

**REQ-TCPC-005** [상태 기반]
**IF** `max_retries`가 0(무한)으로 설정되면 **THEN** 연결이 복구될 때까지 무한 재시도해야 한다.

**REQ-TCPC-006** [이벤트 기반]
**WHEN** 서버로부터 데이터가 수신되면 **THEN** 설정된 프레이밍 방식에 따라 메시지를 분리하고 내부 수신 채널로 전달해야 한다.

**REQ-TCPC-007** [이벤트 기반]
**WHEN** 연결 타임아웃(`connect_timeout`)이 발생하면 **THEN** 연결 시도를 중단하고 재연결 로직을 트리거해야 한다.

**REQ-TCPC-008** [이벤트 기반]
**WHEN** TCP 클라이언트 에이전트가 Stop되면 **THEN** 재연결 루프를 중단하고 연결을 정상 종료해야 한다.

### 4.3 UDP Server 에이전트

**REQ-UDPS-001** [유비쿼터스]
시스템은 **항상** UDP 서버 에이전트(`udp-server`)를 에이전트 레지스트리에 등록해야 한다.

**REQ-UDPS-002** [이벤트 기반]
**WHEN** UDP 서버 에이전트가 Start되면 **THEN** 설정된 host:port에서 UDP 소켓을 바인딩하고 데이터그램 수신을 시작해야 한다.

**REQ-UDPS-003** [이벤트 기반]
**WHEN** UDP 데이터그램이 수신되면 **THEN** 발신 주소 정보와 함께 메시지를 내부 수신 채널로 전달해야 한다.

**REQ-UDPS-004** [이벤트 기반]
**WHEN** 특정 원격 주소에 대한 차단 요청이 수신되면 **THEN** 해당 주소를 차단 목록에 추가해야 한다.

**REQ-UDPS-005** [비허용 동작]
시스템은 차단 목록에 포함된 원격 주소로부터의 데이터그램을 **처리하지 않아야 한다**.

**REQ-UDPS-006** [이벤트 기반]
**WHEN** 연결 정보 조회 요청이 수신되면 **THEN** 최근 통신한 원격 주소 목록(주소, 최종 수신 시각, 수신 바이트 수)을 반환해야 한다.

### 4.4 UDP Client 에이전트

**REQ-UDPC-001** [유비쿼터스]
시스템은 **항상** UDP 클라이언트 에이전트(`udp-client`)를 에이전트 레지스트리에 등록해야 한다.

**REQ-UDPC-002** [이벤트 기반]
**WHEN** UDP 클라이언트 에이전트가 Start되면 **THEN** 설정된 대상 host:port로 UDP 소켓을 준비해야 한다.

**REQ-UDPC-003** [이벤트 기반]
**WHEN** 송신 데이터가 전달되면 **THEN** 설정된 대상 주소로 UDP 데이터그램을 전송해야 한다.

**REQ-UDPC-004** [이벤트 기반]
**WHEN** 응답 데이터그램이 수신되면 **THEN** 내부 수신 채널로 전달해야 한다.

### 4.5 소켓 브릿지 노드

**REQ-NODE-001** [유비쿼터스]
시스템은 **항상** 소켓 입력 노드(`socket-input`), 소켓 출력 노드(`socket-output`), 소켓 양방향 노드(`socket-inout`)를 노드 레지스트리에 등록해야 한다.

**REQ-NODE-002** [이벤트 기반]
**WHEN** `socket-input` 노드가 초기화되면 **THEN** 연결된 소켓 에이전트로부터 데이터를 수신하여 플로우로 전달하는 BridgeIn 모드로 동작해야 한다.

**REQ-NODE-003** [이벤트 기반]
**WHEN** `socket-output` 노드에 메시지가 입력되면 **THEN** 연결된 소켓 에이전트를 통해 외부로 데이터를 전송하는 BridgeOut 모드로 동작해야 한다.

**REQ-NODE-004** [이벤트 기반]
**WHEN** `socket-inout` 노드가 초기화되면 **THEN** 양방향(수신+송신) BridgeInOut 모드로 동작해야 한다.

**REQ-NODE-005** [유비쿼터스]
소켓 브릿지 노드는 **항상** 기존 BridgeNode/BridgeAdapter 패턴을 활용하여 구현해야 한다.

### 4.6 메시지 프레이밍

**REQ-FRAME-001** [유비쿼터스]
TCP 에이전트는 **항상** 다음 프레이밍 방식 중 하나를 설정으로 선택할 수 있어야 한다:
- `raw`: 수신된 데이터를 그대로 전달 (버퍼 크기 단위)
- `newline`: 줄바꿈(`\n`) 구분자로 메시지 분리
- `length_prefix`: 4바이트 빅엔디안 길이 접두사로 메시지 분리
- `fixed_size`: 고정 크기로 메시지 분리

**REQ-FRAME-002** [유비쿼터스]
UDP 에이전트는 **항상** 데이터그램 단위로 메시지를 처리하며 별도 프레이밍 설정이 불필요해야 한다.

### 4.7 설정 (Configuration)

**REQ-CFG-001** [유비쿼터스]
모든 소켓 에이전트는 **항상** 다음 공통 설정을 지원해야 한다:
- `host`: 바인딩/대상 호스트 (기본: `0.0.0.0` for server, `localhost` for client)
- `port`: 포트 번호 (필수)
- `buffer_size`: 수신 버퍼 크기 (기본: 4096 바이트)

**REQ-CFG-002** [유비쿼터스]
TCP 에이전트는 **항상** 추가로 다음 설정을 지원해야 한다:
- `framing`: 프레이밍 방식 (기본: `raw`)
- `delimiter`: 커스텀 구분자 바이트 (framing이 `newline`일 때 오버라이드 가능)
- `fixed_size`: 고정 크기 값 (framing이 `fixed_size`일 때 필수)

**REQ-CFG-003** [유비쿼터스]
TCP 서버 에이전트는 **항상** 추가로 다음 설정을 지원해야 한다:
- `max_connections`: 최대 동시 접속 수 (기본: 0 = 무제한)

**REQ-CFG-004** [유비쿼터스]
TCP 클라이언트 에이전트는 **항상** 추가로 다음 설정을 지원해야 한다:
- `reconnect_interval`: 재연결 간격 (기본: 5s)
- `max_retries`: 최대 재시도 횟수 (기본: 0 = 무한)
- `connect_timeout`: 연결 타임아웃 (기본: 10s)

---

## 5. 비기능 요구사항

### 5.1 성능

- **NFR-PERF-001**: TCP 서버는 최소 100개 동시 접속을 지원해야 한다
- **NFR-PERF-002**: 메시지 수신 지연은 p95 기준 5ms 이내여야 한다
- **NFR-PERF-003**: 에이전트 메모리 사용량은 유휴 상태에서 10MB 이하여야 한다

### 5.2 신뢰성

- **NFR-REL-001**: 클라이언트 재연결은 지수 백오프를 적용해야 한다
- **NFR-REL-002**: 서버 에이전트 Stop 시 모든 연결이 graceful close되어야 한다
- **NFR-REL-003**: 에이전트 Pause/Resume 시 데이터 유실이 없어야 한다

### 5.3 관찰성

- **NFR-OBS-001**: 연결/해제 이벤트를 slog 기반 구조화된 로그로 기록해야 한다
- **NFR-OBS-002**: Prometheus 메트릭(활성 연결 수, 수신/송신 바이트, 에러 수)을 노출해야 한다
- **NFR-OBS-003**: 에이전트 상태(연결 목록, 차단 목록)를 StatefulAgent 인터페이스로 제공해야 한다

### 5.4 보안

- **NFR-SEC-001**: 서버 에이전트는 IP 기반 접속 차단을 지원해야 한다
- **NFR-SEC-002**: 버퍼 오버플로우를 방지하기 위해 최대 메시지 크기 제한을 설정할 수 있어야 한다

---

## 6. 기술 접근 / 아키텍처

### 6.1 에이전트 구조

```
internal/agent/socket/
├── common.go           # 공통 타입, 상수, 에러 정의
├── config.go           # SocketConfig 설정 파싱 및 검증
├── errors.go           # 센티널 에러 정의
├── framing.go          # 메시지 프레이밍 (raw, newline, length_prefix, fixed_size)
├── connection.go       # ConnectionInfo, ConnectionManager (연결 관리/차단)
├── tcp_server.go       # TCPServerAgent 구현
├── tcp_client.go       # TCPClientAgent 구현
├── udp_server.go       # UDPServerAgent 구현
├── udp_client.go       # UDPClientAgent 구현
├── register.go         # 에이전트 타입 등록 (tcp-server, tcp-client, udp-server, udp-client)
├── common_test.go      # 공통 테스트
├── config_test.go      # 설정 테스트
├── framing_test.go     # 프레이밍 테스트
├── connection_test.go  # 연결 관리 테스트
├── tcp_server_test.go  # TCP 서버 테스트
├── tcp_client_test.go  # TCP 클라이언트 테스트
├── udp_server_test.go  # UDP 서버 테스트
└── udp_client_test.go  # UDP 클라이언트 테스트
```

### 6.2 노드 구조

소켓 브릿지 노드는 기존 BridgeNode를 활용한다. 별도의 소켓 전용 노드 파일이 필요한 경우:

```
internal/node/
├── socket_bridge.go       # SocketBridgeAdapter 구현 (BridgeAdapter 인터페이스)
└── socket_bridge_test.go  # 소켓 브릿지 어댑터 테스트
```

또는 기존 BridgeNode가 소켓 에이전트 타입을 자동 감지하여 처리하는 방식으로 구현할 수 있다 (MODBUS RW Node 패턴 참조).

### 6.3 에이전트 클래스 구조

```
TCPServerAgent
├── BaseAgent (임베딩)
├── listener      net.Listener
├── connections   ConnectionManager
├── framer        Framer
├── recvCh        chan []byte
└── config        TCPServerConfig

TCPClientAgent
├── BaseAgent (임베딩)
├── conn          net.Conn
├── framer        Framer
├── recvCh        chan []byte
├── reconnector   Reconnector
└── config        TCPClientConfig

UDPServerAgent
├── BaseAgent (임베딩)
├── conn          *net.UDPConn
├── peers         ConnectionManager
├── recvCh        chan []byte
└── config        UDPServerConfig

UDPClientAgent
├── BaseAgent (임베딩)
├── conn          *net.UDPConn
├── recvCh        chan []byte
└── config        UDPClientConfig
```

### 6.4 프레이밍 인터페이스

```go
type Framer interface {
    // Read reads one complete message from the connection.
    Read(conn net.Conn) ([]byte, error)
    // Write writes one complete message to the connection.
    Write(conn net.Conn, data []byte) error
}
```

프레이밍 구현체: `RawFramer`, `NewlineFramer`, `LengthPrefixFramer`, `FixedSizeFramer`

### 6.5 ConnectionManager 인터페이스

```go
type ConnectionInfo struct {
    RemoteAddr    string
    ConnectedAt   time.Time
    BytesSent     int64
    BytesReceived int64
}

type ConnectionManager interface {
    Add(conn net.Conn) error
    Remove(remoteAddr string) error
    Block(remoteAddr string) error
    Unblock(remoteAddr string) error
    IsBlocked(remoteAddr string) bool
    List() []ConnectionInfo
    BlockedList() []string
    Count() int
    CloseAll() error
}
```

### 6.6 선택적 인터페이스 구현

| 에이전트 | Agent | MessageReceiver | StatefulAgent | TransportChecker | BufferInfoProvider |
|---------|-------|----------------|---------------|-----------------|-------------------|
| TCPServerAgent | O | O | O | O | O |
| TCPClientAgent | O | O | O | O | O |
| UDPServerAgent | O | O | O | - | O |
| UDPClientAgent | O | O | O | - | O |

---

## 7. 추적성 (Traceability)

| 요구사항 | 파일 | 테스트 |
|---------|------|--------|
| REQ-TCPS-001~010 | internal/agent/socket/tcp_server.go | tcp_server_test.go |
| REQ-TCPC-001~008 | internal/agent/socket/tcp_client.go | tcp_client_test.go |
| REQ-UDPS-001~006 | internal/agent/socket/udp_server.go | udp_server_test.go |
| REQ-UDPC-001~004 | internal/agent/socket/udp_client.go | udp_client_test.go |
| REQ-NODE-001~005 | internal/node/socket_bridge.go | socket_bridge_test.go |
| REQ-FRAME-001~002 | internal/agent/socket/framing.go | framing_test.go |
| REQ-CFG-001~004 | internal/agent/socket/config.go | config_test.go |

---

*문서 버전: 1.0.0*
*최종 수정: 2026-04-01*
*작성: MoAI SPEC Builder (manager-spec)*
