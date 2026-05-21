# SPEC-LGCP-001: LG Internal Control Protocol Agent (v2.0.0 - Clean Transport Abstraction)

**Version**: 2.10.0
**Status**: Implemented
**Created**: 2026-03-24
**Updated**: 2026-05-21

## 1. Overview

LG Internal Control Protocol (LGCP) 에이전트의 전송 계층을 확장하여, 기존 시리얼 전송 외에 **TCP Client** 및 **TCP Server** 전송 모드를 지원한다. 기존 `LGAPTransport` 인터페이스에 새로운 구현체를 추가하는 방식이므로, LGCP 에이전트의 내부 구조(captureLoop, Frame Parser, Process, ReceiveMessage)는 변경하지 않는다.

### 1.1 Version History

| 버전 | 날짜 | 설명 |
|------|------|------|
| 1.0.0 | 2026-03-24 | 초기 SPEC: 직접 시리얼 캡처 에이전트 |
| 1.1.0 | 2026-03-24 | 제어 명령 지원 추가 (서모스탯 사칭 모드) |
| 2.0.0 | 2026-04-06 | Clean Transport Abstraction: TCP Client/Server 전송 모드 추가 |
| 2.6.0 | 2026-05-19 | **5 HVAC 옵션 명칭 통일 (notify→report)**. `notify_interval` → `report_interval` 등 5 에이전트 통일. JSON schema 슬림화 (timestamp_ms/seq/raw_hex/confirmation_status 제거). |
| 2.6.6 | 2026-05-20 | **event_temp_threshold 게이트 + 온도 통일**. 이벤트 보고 시 비온도 필드 변경 없이 온도(IndoorTempC+PipeTemp1C+PipeTemp2C) 만 max\|Δ\| < threshold 면 emit suppress. `nonTempFieldsChangedLGCP` + `maxTempDeltaLGCP` helper. 기본 1.0℃. |
| 2.6.8 | 2026-05-20 | **정기 보고 (`trigger=report`)** — LGCP 는 이미 `notifyLoop`/`sendDeviceNotifications` 보유. v0.7.0 에서 `device_state_report` → `device_state` (trigger="report") 로 schema 통일. |
| 2.7.0 | 2026-05-21 | **출력 schema 단일화 + change emit 추가**. `device_state_changed` / `device_state_report` 별도 event type → 단일 `type:"device_state"` (trigger 로 구분). LGCP 는 change 시 emit 이 없었으나 (콜백만 호출) — `emitDeviceStateLocked(dev, "change")` 추가하여 5 에이전트 통일. modeToCanonical: cooling/heating/dehumidify → cool/heat/dry. |
| 2.7.1 | 2026-05-21 | **폴링 명령 5 노드 통일**. `drain` → `get_recent + count=0` (deprecation alias 유지). |
| 2.7.2 | 2026-05-21 | **`processGetAll` 추가** (모든 device 즉시 snapshot 반환). |
| 2.7.3 | 2026-05-21 | **`processGetState` 추가** (address 기반 단일 device 조회). |
| 2.7.5 | 2026-05-21 | **mode/fan_speed 통일 ID (int) 출력**. `hvac.ModeFromName` / `hvac.FanSpeedFromName` 활용. Power=false 시 0 강제. OFF 상태 "-" 표기 폐기 → 0. |
| 2.7.6 | 2026-05-21 | **Manager.Restart lock holding 단축** (Restart 영향). |
| 2.7.7~2.7.8 | 2026-05-21 | **노드 pollSingle byte-equal dedup + normalizeForDedup** (last_seen_ms 제외). get_all/get_state 동일 snapshot 반복 emit 제거. |
| 2.7.14 | 2026-05-21 | **HVAC status payload 의 nested metadata 를 message metadata 로 promote**. `promotePayloadMetadata` 헬퍼. LGCP-Status / LGCP 의 모든 emit 사이트 적용. |
| 2.10.0 | 2026-05-21 | **BREAKING — `lgcp_source="request"` 제거**. message_type="device_state.response" 와 중복. Process 응답에서 lgcp_source 라인 삭제. lgcp_source="poll" / "poll_bulk" 는 유지. 다운스트림: `lgcp_source == "request"` → `message_type == "device_state.response"`. |
| 2.9.0 | 2026-05-21 | **BREAKING — payload.type 제거**. v0.8.0 message_type 계층형 분류로 인해 payload.type="device_state" 가 prefix 의 중복이 됨. `sendStatusEvent` 가 eventType="" 일 때 type 필드 주입 skip 하도록 변경 (transport_reconnecting 등 다른 이벤트는 type 유지). `emitDeviceStateLocked` 가 빈 eventType 으로 호출. 다운스트림: `$.payload.type` 검사 → `$.metadata.message_type` prefix 검사. |
| 2.8.0 | 2026-05-21 | **BREAKING — metadata.message_type 계층형 분류 + payload.trigger 제거**. 직교 분류 (`trigger` + `message_type="event\|response"`) 가 종속 관계라는 사용자 지적에 따라 단일 진실원천 통합. `applyDeviceStateMessageType(msg, payload, defaultSubType)` 헬퍼로 payload.trigger → `metadata.message_type="device_state.<trigger>"` 변환 + payload 에서 trigger 제거. 값 체계: `device_state.change` / `.report` / `.poll` (자발 emit) + `device_state.response` (Process 응답). LGCP 의 pollSingle / pollBulk / Process 모든 emit 사이트 적용. 다운스트림 필터 변경 필요. |

### 1.2 Protocol Summary (변경 없음)

- **Physical**: RS-485 (보레이트 설정 가능, 기본값 9600 bps)
- **Frame**: 가변 길이, STX=0x56 시작
- **Frame Structure**:
  ```
  [STX=0x56][LEN][DLEN][DA(N)][SLEN][SA(N)][CMD(2)][SEQ0#][PLEN][PAYLOAD(N)][SEQ1#][CRC(2)]
  ```
- **CRC**: CRC-16/CCITT-FALSE (poly=0x1021, init=0xFFFF) XOR 0x5C56, Big-endian
- **CRC Input**: frame[2:-2] (STX, LEN 제외, CRC 제외)

### 1.3 Key Design Decisions (v2.0.0)

- **기존 LGAPTransport 인터페이스 재사용**: LGCP 에이전트는 이미 `LGAPTransport` 인터페이스를 통해 전송 계층을 추상화하고 있다. 새로운 TCP 전송은 이 인터페이스의 추가 구현체로 만든다.
- **Bridge 노드 불필요**: TCP 전송은 LGCP 에이전트가 직접 소유한다. Samsung NASA 에이전트가 시리얼을 직접 소유하는 것과 동일한 패턴이다.
- **Process() 변경 없음**: 제어 명령은 기존과 동일하게 `Process(command_json)` → LGCP 프레임 빌드 → `transport.Send()` → 장치로 전달된다.
- **captureLoop 변경 없음**: `captureLoop()` 은 `transportReader` (io.Reader 어댑터)를 통해 transport에서 데이터를 읽으며, transport 구현체에 무관하게 동일하게 동작한다.
- **하위 호환성**: `transport_type` 미설정 시 기본값 `"serial"` 로 동작하며, 기존 설정 파일과 완전 호환된다.

### 1.4 Data Flow Architecture (v2.0.0)

```
=== 모든 전송 모드에서 동일한 내부 흐름 ===

[Transport: Serial / TCP Client / TCP Server]
     |
     v
[transportReader] (io.Reader adapter)
     |
     v
[captureLoop()] → Frame Parser → msgCh → ReceiveMessage() → Bridge(In) → Flow

[Flow → Bridge(Out)] → Process(command_json) → Build LGCP Frame → transport.Send() → [장치]


=== 전송 모드별 연결 방식 ===

Serial Mode (기존):
  lgapSerialTransport → RS-485 시리얼 포트 직접 연결

TCP Client Mode (신규):
  lgapTCPClientTransport → 원격 TCP 서버에 접속 (예: RS485-to-TCP 변환기)

TCP Server Mode (신규):
  lgapTCPServerTransport → TCP 포트에서 대기, 장치의 TCP 접속을 수락
```

### 1.5 LGAPTransport Interface (기존, 변경 없음)

```go
type LGAPTransport interface {
    Open() error
    Close() error
    Send(data []byte) error
    Receive(buf []byte) (int, error)
    Available() bool
    Write(data []byte) (int, error)
}
```

현재 구현체:
- `lgapSerialTransport` (기존)

신규 구현체 (v2.0.0):
- `lgapTCPClientTransport` - 원격 TCP 서버에 접속하는 클라이언트
- `lgapTCPServerTransport` - TCP 접속을 수락하는 서버

### 1.6 LGAP vs LGCP 비교 (변경 없음)

| 항목 | LGAP (기존) | LGCP (신규) |
|------|------------|------------|
| STX | 0x10 | 0x56 |
| 프레임 길이 | 고정 (요청 8B, 응답 16B) | 가변 (LEN 바이트로 지정) |
| 통신 모델 | Master/Slave 폴링 | 패시브 캡처 |
| CRC | (sum % 256) XOR 0x55 | CRC-16/CCITT XOR 0x5C56 |
| 주소 체계 | Zone (1 byte) | DA(NB) + SA(NB) |
| 데이터 출력 | 상태 이벤트 -> msgCh | 프레임 이벤트 -> msgCh |

---

## 2. Requirements

### 기존 요구사항 (v1.1.0 - 유지)

아래 요구사항은 v1.1.0에서 정의되었으며 v2.0.0에서도 유효하다:

- **REQ-LGCP-001-01**: Agent Type Registration (`"lgcp"` 타입 등록)
- **REQ-LGCP-001-02**: Serial Transport Configuration (시리얼 포트 설정)
- **REQ-LGCP-001-03**: Frame Parser (STX+LEN Framing)
- **REQ-LGCP-001-04**: CRC-16 Verification
- **REQ-LGCP-001-05**: Frame Header Parsing
- **REQ-LGCP-001-06**: Frame Event Output via msgCh
- **REQ-LGCP-001-07**: Agent Lifecycle
- **REQ-LGCP-001-08**: Capture Statistics
- **REQ-LGCP-001-09**: Agent Configuration
- **REQ-LGCP-001-10**: Passive Operation (serial 모드 한정)
- **REQ-LGCP-001-11**: Web UI Schema
- **REQ-LGCP-001-12**: Example Configuration

### 신규 요구사항 (v2.0.0)

### REQ-LGCP-001-13: Transport Type Configuration

시스템은 **항상** LGCP 에이전트의 전송 유형을 `transport_type` 설정으로 관리해야 한다.

- 지원 유형: `"serial"` (기본), `"tcp-client"`, `"tcp-server"`
- `LGCPConfig` 구조체에 `TransportType string` 필드 추가 (기본값: `"serial"`)
- `transport_type` 미설정 시 기본값 `"serial"` 로 동작 (기존 설정 파일과 완전 호환)
- 알 수 없는 `transport_type` 값에 대해 에러를 반환

### REQ-LGCP-001-14: TCP Client Transport

**WHEN** `transport_type` 이 `"tcp-client"` 로 설정되면 **THEN** 시스템은 `lgapTCPClientTransport` 를 생성하여 원격 TCP 서버에 접속해야 한다.

- `tcp_host` 와 `tcp_port` 설정을 사용하여 원격 호스트에 TCP 연결
- `Open()`: `net.DialTimeout()` 으로 TCP 연결 수립, 설정된 타임아웃 적용
- `Close()`: TCP 연결 종료
- `Send(data)`: TCP 연결로 데이터 전송, write deadline 적용
- `Receive(buf)`: TCP 연결에서 데이터 수신, read deadline 적용
- `Available()`: TCP 연결 상태를 atomic bool로 추적
- `Write(data)`: `Send()` 와 동일
- 연결 상태를 atomic bool로 관리하여 thread-safe하게 추적
- 동시 Read/Write를 `sync.Mutex` 로 보호

### REQ-LGCP-001-15: TCP Server Transport

**WHEN** `transport_type` 이 `"tcp-server"` 로 설정되면 **THEN** 시스템은 `lgapTCPServerTransport` 를 생성하여 TCP 접속을 수락해야 한다.

- `tcp_host` 와 `tcp_port` 설정을 사용하여 TCP 리스너 시작
- `Open()`: `net.Listen("tcp", host:port)` 로 리스닝 시작, 별도 고루틴에서 Accept 루프 실행
- `Close()`: 리스너와 현재 연결 모두 종료
- `Send(data)`: 현재 활성 연결로 데이터 전송 (연결이 없으면 에러)
- `Receive(buf)`: 현재 활성 연결에서 데이터 수신
- `Available()`: 활성 연결 존재 여부를 atomic bool로 추적
- `Write(data)`: `Send()` 와 동일

### REQ-LGCP-001-16: TCP Reconnection and Connection Management

시스템은 **항상** TCP 전송의 재연결 및 연결 관리를 안전하게 처리해야 한다.

- **TCP Client 재연결**: 연결이 끊어지면 LGCP 에이전트의 기존 `reconnectLoop()` 이 `transport.Open()` 을 호출하여 재연결 시도. 기존 시리얼 재연결과 동일한 패턴.
- **TCP Server 연결 교체**: 새 연결이 들어오면 기존 연결을 닫고 새 연결로 교체 (최신 연결 우선). 교체 시 `sync.Mutex` 로 보호.
- **TCP Client/Server 공통**: 연결 끊김 시 `Available()` 이 `false` 를 반환하도록 atomic bool 업데이트.

### REQ-LGCP-001-17: TCP Connection Health Monitoring

시스템은 **항상** TCP 연결 상태를 모니터링해야 한다.

- `Available()` 메서드가 현재 TCP 연결 상태를 정확하게 반환
- Read/Write 에러 발생 시 `Available()` 을 `false` 로 전환
- 연결 성공 시 `Available()` 을 `true` 로 전환
- 기존 `reconnectLoop()` 이 `Available()` 을 확인하여 재연결 필요 여부를 판단 (기존 시리얼과 동일한 패턴)

### REQ-LGCP-001-18: Serial Port Conditional Requirement

**IF** `transport_type` 이 `"serial"` 이면 **THEN** `serial_port` 설정이 필수이다.
**IF** `transport_type` 이 `"tcp-client"` 또는 `"tcp-server"` 이면 **THEN** `serial_port` 설정이 필수가 아니다.

- `tcp-client` 모드: `tcp_host` 와 `tcp_port` 가 필수
- `tcp-server` 모드: `tcp_port` 가 필수, `tcp_host` 는 선택 (기본값: `"0.0.0.0"`)
- `serial` 모드: 기존과 동일하게 `serial_port` 가 필수 (`ErrLGCPSerialPortRequired`)

### REQ-LGCP-001-19: Transport Factory

시스템은 **항상** 설정의 `transport_type` 에 따라 올바른 전송 구현체를 생성해야 한다.

- `"serial"` (기본): `lgapSerialTransport` 생성 (기존 동작)
- `"tcp-client"`: `lgapTCPClientTransport` 생성
- `"tcp-server"`: `lgapTCPServerTransport` 생성
- 알 수 없는 값: 에러 반환
- Transport Factory 는 `NewLGCPAgent()` 내에서 실행
- 생성된 transport 는 기존과 동일하게 `agent.transport` 필드에 할당

### REQ-LGCP-001-20: TCP Read/Write Timeout Configuration

시스템은 **항상** TCP 전송의 읽기/쓰기 타임아웃을 설정할 수 있어야 한다.

- `LGCPConfig` 에 TCP 관련 필드 추가:
  - `TCPHost string`: TCP 호스트 (기본값: `"0.0.0.0"`)
  - `TCPPort int`: TCP 포트 (필수, tcp-client/tcp-server 모드)
  - `TCPReadTimeout time.Duration`: TCP 읽기 타임아웃 (기본값: 500ms, 기존 `ReadTimeout` 과 동일)
  - `TCPWriteTimeout time.Duration`: TCP 쓰기 타임아웃 (기본값: 1s)
  - `TCPConnectTimeout time.Duration`: TCP 연결 타임아웃 (기본값: 5s, tcp-client 전용)
- 기존 `ReadTimeout` 은 시리얼 모드 전용으로 유지

### REQ-LGCP-001-21: TCP Server Multi-Connection Handling

**WHEN** TCP Server 모드에서 새로운 TCP 연결이 수락되면 **THEN** 시스템은 기존 연결을 닫고 새 연결로 교체해야 한다.

- 단일 활성 연결만 유지 (LGCP 프로토콜은 point-to-point 통신)
- 새 연결 수락 시: 기존 연결 Close → 새 연결을 활성 연결로 설정 → `Available()` = `true`
- 연결 교체는 `sync.Mutex` 로 보호하여 thread-safe 하게 처리
- 리스너 Accept 루프는 `Close()` 호출 시 종료

---

## 3. Architecture (v2.0.0)

### 3.1 Package Structure (변경/추가 파일)

```
internal/agent/lg/
  # --- 기존 파일 (변경 없음) ---
  transport.go              - LGAPTransport 인터페이스, lgapSerialTransport (변경 없음)
  lgcp_agent.go             - captureLoop, Process, ReceiveMessage (변경 최소화)
  lgcp_frame.go             - LGCPFrameParser (변경 없음)

  # --- 수정 파일 ---
  lgcp_config.go            - transport_type, tcp_host, tcp_port 필드 추가, serial_port 조건부 필수
  lgcp_agent.go             - NewLGCPAgent() 에 Transport Factory 추가, reconnectLoop 미세 조정

  # --- 신규 파일 ---
  transport_tcp.go          - lgapTCPClientTransport, lgapTCPServerTransport
  transport_tcp_test.go     - TCP Transport 단위 테스트

  # --- 수정 테스트 ---
  lgcp_config_test.go       - transport_type, tcp_host, tcp_port 파싱 테스트
  lgcp_agent_test.go        - TCP 전송 모드 통합 테스트

web/src/config/agentSchemas.ts  - transport_type, tcp_host, tcp_port 필드 추가
web/src/pages/agents/agentTypeMeta.ts  - Transport 유형별 필드 메타데이터
```

### 3.2 Component Diagram (v2.0.0)

```
                ┌───────────────────────────────────────────────┐
                │                 LGCPAgent                     │
                │                                               │
                │   ┌──────────────────────────────────┐        │
                │   │        LGAPTransport             │        │
                │   │  (interface, 변경 없음)            │        │
                │   └──────────┬───────────────────────┘        │
                │              │                                │
                │     ┌────────┼────────────┐                   │
                │     │        │            │                   │
                │     v        v            v                   │
                │ ┌────────┐ ┌──────────┐ ┌──────────┐         │
                │ │ Serial │ │TCP Client│ │TCP Server│         │
                │ │(기존)   │ │(신규)     │ │(신규)     │         │
                │ └────┬───┘ └─────┬────┘ └─────┬────┘         │
                │      │           │            │               │
                │      v           v            v               │
                │   RS-485     net.Conn      net.Listener       │
                │   Port       (Dial)        (Accept)           │
                │                                               │
                │   ┌──────────────────────────────────┐        │
                │   │   transportReader (io.Reader)    │        │
                │   │   transport.Receive() 래핑        │        │
                │   └──────────┬───────────────────────┘        │
                │              │                                │
                │              v                                │
                │   ┌──────────────────────────────────┐        │
                │   │       captureLoop()              │        │
                │   │       (변경 없음)                  │        │
                │   └──────────┬───────────────────────┘        │
                │              │                                │
                │              v                                │
                │   ┌──────────────────────────────────┐        │
                │   │    LGCPFrameParser (변경 없음)     │        │
                │   └──────────┬───────────────────────┘        │
                │              │                                │
                │              v                                │
                │           msgCh (파싱된 이벤트)                 │
                └──────────────┬────────────────────────────────┘
                               │
                               v
                      ReceiveMessage()
                               │
                               v
                        [Bridge Node]
                               │
                               v
                      [다운스트림 노드]
```

### 3.3 Transport Implementations

```go
// lgapTCPClientTransport 는 원격 TCP 서버에 접속하는 전송 구현체이다.
type lgapTCPClientTransport struct {
    host           string
    port           int
    conn           net.Conn
    mu             sync.Mutex
    connected      atomic.Bool
    readTimeout    time.Duration
    writeTimeout   time.Duration
    connectTimeout time.Duration
}

// lgapTCPServerTransport 는 TCP 접속을 수락하는 전송 구현체이다.
type lgapTCPServerTransport struct {
    host        string
    port        int
    listener    net.Listener
    conn        net.Conn        // 현재 활성 연결 (단일)
    mu          sync.Mutex
    connected   atomic.Bool
    readTimeout time.Duration
    writeTimeout time.Duration
    closeCh     chan struct{}
}
```

### 3.4 YAML Configuration Examples (v2.0.0)

```yaml
# Serial 모드 (기존, 변경 없음)
name: lgcp-serial-capture
type: lgcp
transport:
  type: serial
  options:
    serial_port: /dev/ttyUSB1
    baud_rate: 9600
    verify_crc: true

---

# TCP Client 모드 (신규) - RS485-to-TCP 변환기에 접속
name: lgcp-tcp-client
type: lgcp
transport:
  type: serial           # 레거시 호환을 위해 유지
  options:
    transport_type: tcp-client
    tcp_host: "192.168.1.100"
    tcp_port: 8899
    tcp_read_timeout: "500ms"
    tcp_write_timeout: "1s"
    tcp_connect_timeout: "5s"
    verify_crc: true
    control_enabled: true

---

# TCP Server 모드 (신규) - 장치가 xflow에 접속
name: lgcp-tcp-server
type: lgcp
transport:
  type: serial           # 레거시 호환을 위해 유지
  options:
    transport_type: tcp-server
    tcp_host: "0.0.0.0"
    tcp_port: 9900
    tcp_read_timeout: "500ms"
    tcp_write_timeout: "1s"
    verify_crc: true
    msg_channel_size: 256
```

---

## 4. Constraints

### 4.1 기술 제약

- Go 표준 라이브러리만 사용 (외부 라이브러리 미사용)
- `LGAPTransport` 인터페이스를 변경하지 않음 (기존 구현체와의 호환성 유지)
- TCP 전송은 기존 `lgapSerialTransport` 와 동일한 수준의 thread-safety 를 보장해야 함
- LGCP 에이전트 내부 구조 (`captureLoop`, `transportReader`, `LGCPFrameParser`)를 변경하지 않음

### 4.2 하위 호환성 제약

- 기존 YAML 설정 파일이 변경 없이 동작해야 한다 (`transport_type` 미설정 = `"serial"`)
- 기존 REST API 응답 형식이 변경되지 않아야 한다
- 기존 단위 테스트가 전부 통과해야 한다
- `lgapSerialTransport` 코드를 수정하지 않는다

---

## 5. Traceability

| TAG | 요구사항 | 관련 파일 |
|-----|---------|----------|
| REQ-LGCP-001-13 | Transport Type Configuration | lgcp_config.go |
| REQ-LGCP-001-14 | TCP Client Transport | transport_tcp.go |
| REQ-LGCP-001-15 | TCP Server Transport | transport_tcp.go |
| REQ-LGCP-001-16 | TCP Reconnection and Connection Management | transport_tcp.go, lgcp_agent.go |
| REQ-LGCP-001-17 | TCP Connection Health Monitoring | transport_tcp.go |
| REQ-LGCP-001-18 | Serial Port Conditional Requirement | lgcp_config.go |
| REQ-LGCP-001-19 | Transport Factory | lgcp_agent.go |
| REQ-LGCP-001-20 | TCP Read/Write Timeout Configuration | lgcp_config.go, transport_tcp.go |
| REQ-LGCP-001-21 | TCP Server Multi-Connection Handling | transport_tcp.go |
