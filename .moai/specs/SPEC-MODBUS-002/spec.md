---
id: SPEC-MODBUS-002
version: "1.0.0"
status: completed
created: "2026-02-26"
updated: "2026-02-27"
author: xtra
priority: high
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 0.1.0 | 2026-02-26 | xtra | 최초 작성 - MODBUS/TCP Server Agent 8개 모듈 |
| 1.0.0 | 2026-02-27 | xtra | 구현 완료 - 전체 9개 모듈, 커버리지 89.5%, 14개 파일 |

---

# SPEC-MODBUS-002: MODBUS/TCP Server Agent 구현

## 1. Environment (환경)

### 1.1 시스템 개요

xflow는 Go 기반 IoT FBP(Flow-Based Programming) 플랫폼이다. SPEC-MODBUS-001에서 구현한 MODBUS/TCP Client Agent는 외부 MODBUS 디바이스에 능동적으로 연결하여 레지스터를 읽고 쓰는 **마스터(클라이언트)** 역할을 수행한다.

MODBUS/TCP Server Agent는 이와 반대로, TCP 포트를 열어 외부 MODBUS 클라이언트(HMI, SCADA, PLC 등)의 연결을 수락하고 레지스터 맵에 대한 읽기/쓰기 요청을 처리하는 **슬레이브(서버)** 역할을 수행한다. 에이전트 설정(YAML)으로 레지스터 맵(Coil, Discrete Input, Holding Register, Input Register)을 정의하고, Bridge를 통해 내부 플로우에서 레지스터 값을 동적으로 변경하거나 외부 클라이언트의 쓰기에 의한 값 변경을 플로우에 알릴 수 있다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/modbusserver/`
- **기존 인터페이스**:
  - `Agent` 인터페이스 (`internal/agent/agent.go`): Init, Start, Stop, Pause, Resume, Health, Process, Configure, ID, Name, Type, Info, Stats
  - `BaseLifecycle` 임베딩 패턴 (`pkg/lifecycle`)
  - `MessageReceiver` 인터페이스: ReceiveMessage(ctx) ([]byte, error)
  - `StatefulAgent` 인터페이스: State() map[string]any
  - `TypeRegistry`: RegisterType/CreateAgent
- **재사용 가능한 기존 코드** (`internal/agent/modbus/`):
  - `protocol.go`: MBAP 상수, 프레임 빌드/파싱, FC 상수, encode/decode 함수
  - `cache.go`: RegisterCache 구조체 (Coils, DiscreteInputs, HoldingRegisters, InputRegisters 맵)
  - `errors.go`: 공통 에러 (ErrFrameTooShort, ErrInvalidFunctionCode 등)
  - `config.go`: toInt, toByte, toUint16 헬퍼 함수
- **브릿지 노드** (`internal/node/bridge.go`):
  - `BridgeIn`: 에이전트 -> 플로우 (레지스터 변경 알림)
  - `BridgeOut`: 플로우 -> 에이전트 (레지스터 값 설정 명령)
  - `BridgeInOut`: 양방향
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: `sync.RWMutex` (RegisterMap 보호), 다중 클라이언트 연결 병렬 처리

### 1.3 설계 원칙

- **기존 패턴 준수**: MODBUS Client Agent(SPEC-MODBUS-001)의 설정 파싱, 생명주기 관리, Bridge 연동 패턴을 최대한 활용
- **코드 재사용**: `protocol.go`, `cache.go`, `errors.go`의 공통 구성 요소를 재사용하되, 서버 전용 코드는 별도 패키지(`modbusserver/`)에 위치
- **관심사 분리**: 서버 설정, TCP 리스너, 연결 핸들러, 요청 핸들러, 레지스터 맵, Bridge 연동, 에이전트 생명주기를 별도 파일로 분리
- **테스트 가능성**: TCP 리스너를 인터페이스로 추상화하여 테스트에서 `net.Pipe()` 또는 로컬 바인딩으로 대체 가능
- **다중 클라이언트 지원**: 연결마다 독립적인 goroutine으로 처리하며, 공유 RegisterMap에 대한 동시 접근은 `sync.RWMutex`로 보호

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - ModbusServerAgent 구현 (Agent, MessageReceiver, StatefulAgent 인터페이스 준수)
  - TCP 리스너로 외부 MODBUS 클라이언트 연결 수락
  - 다중 동시 클라이언트 연결 지원 (최대 연결 수 제한)
  - YAML 설정 기반 레지스터 맵 초기화 (Coil, Discrete Input, Holding Register, Input Register)
  - FC01~FC04 읽기 요청 처리 (서버 측)
  - FC05/FC06/FC15/FC16 쓰기 요청 처리 (서버 측)
  - MODBUS Exception 응답 생성 (Illegal Function, Illegal Data Address, Illegal Data Value)
  - Bridge Process() 명령을 통한 레지스터 맵 동적 변경
  - Bridge ReceiveMessage를 통한 레지스터 변경 알림 전송
  - Transaction ID 매칭 (요청의 Transaction ID를 응답에 에코)
  - 유휴 클라이언트 타임아웃
  - Health 상태 관리
  - 구조화 로깅 (slog)
  - 단위 테스트 (85%+ 커버리지)
  - TypeRegistry 등록 (`modbus-tcp-server`)
- **범위 외(Out-of-Scope)**:
  - MODBUS RTU 서버 (시리얼 통신)
  - TLS 암호화
  - MODBUS 진단 기능 코드 (FC07, FC08, FC17 등)
  - 파일 레코드 접근 (FC20, FC21)
  - Bridge 노드 자체의 변경
  - 클러스터/분산 레지스터 맵

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-MODBUS-001 | 선행 | MODBUS/TCP Client Agent (protocol.go, cache.go 재사용) |
| SPEC-AGENT-001 | 의존 | Agent, MessageReceiver, StatefulAgent 인터페이스 |
| SPEC-BRIDGE-001 | 의존 | BridgeNode, BridgeConfig, AgentTransport |
| SPEC-ENGINE-001 | 의존 | AgentManagerResolver |

---

## 2. Assumptions (가정)

### A-1. MODBUS/TCP 표준 준수

MODBUS/TCP 프로토콜은 MODBUS Application Protocol Specification V1.1b3 표준을 따른다. 서버는 MBAP Header를 파싱하고 PDU를 처리하여 표준 응답을 생성한다.

### A-2. 기존 protocol.go 재사용 가능

`internal/agent/modbus/protocol.go`에 정의된 MBAP 상수, 프레임 빌드/파싱 함수, Function Code 상수, encode/decode 유틸리티를 서버 에이전트에서 직접 import하여 재사용할 수 있다.

### A-3. 레지스터 맵 사전 정의

서버의 레지스터 맵은 에이전트 YAML 설정에서 사전에 정의된다. 주소 범위와 초기값이 설정 시점에 결정되며, 런타임에 Bridge Process() 명령으로 값만 변경 가능하다 (맵 구조 자체는 변경 불가).

### A-4. 단일 서버 인스턴스

하나의 ModbusServerAgent 인스턴스는 하나의 TCP 포트에서 리스닝한다. 여러 포트가 필요하면 별도의 에이전트 인스턴스를 설정한다.

### A-5. 클라이언트 인증 불필요

MODBUS/TCP 프로토콜 표준에는 인증 메커니즘이 없다. 네트워크 레벨 보안(방화벽, VLAN)으로 접근을 제어한다고 가정한다.

### A-6. Unit ID 단일

서버 에이전트는 설정에서 지정된 단일 Unit ID에 대한 요청만 처리한다. 다른 Unit ID의 요청은 무시(응답 없음)하거나 Exception 응답을 반환한다.

---

## 3. Requirements (요구사항)

### Module 1: Server Configuration (서버 설정)

#### REQ-MODBUS-002-01-01 (Ubiquitous) 서버 설정 파싱

시스템은 **항상** `Transport.Options` (`map[string]any`)에서 다음 서버 전용 설정을 파싱해야 한다:

| 필드 | 키 | 타입 | 기본값 | 필수 | 설명 |
|------|-----|------|--------|------|------|
| ListenAddress | `listen_address` | `string` | `"0.0.0.0"` | No | 리스닝 IP 주소 |
| ListenPort | `listen_port` | `int` | `502` | No | 리스닝 TCP 포트 |
| UnitID | `unit_id` | `byte` | `1` | No | 서버 Unit ID (0~247) |
| MaxConnections | `max_connections` | `int` | `10` | No | 최대 동시 연결 수 |
| IdleTimeout | `idle_timeout` | `string` | `"60s"` | No | 유휴 클라이언트 타임아웃 (time.Duration) |
| MsgChannelSize | `msg_channel_size` | `int` | `256` | No | 메시지 채널 버퍼 크기 |
| RegisterMap | `register_map` | `RegisterMapConfig` | - | Yes | 레지스터 맵 정의 |

#### REQ-MODBUS-002-01-02 (Ubiquitous) 레지스터 맵 설정 구조

시스템은 **항상** 다음 구조로 레지스터 맵을 설정에서 파싱해야 한다:

**RegisterMapConfig 구조:**

| 필드 | 키 | 타입 | 기본값 | 필수 | 설명 |
|------|-----|------|--------|------|------|
| Coils | `coils` | `RegisterAreaConfig` | - | No | FC01/FC05/FC15 코일 영역 |
| DiscreteInputs | `discrete_inputs` | `RegisterAreaConfig` | - | No | FC02 이산 입력 영역 |
| HoldingRegisters | `holding_registers` | `RegisterAreaConfig` | - | No | FC03/FC06/FC16 보유 레지스터 영역 |
| InputRegisters | `input_registers` | `RegisterAreaConfig` | - | No | FC04 입력 레지스터 영역 |

**RegisterAreaConfig 구조:**

| 필드 | 키 | 타입 | 기본값 | 필수 | 설명 |
|------|-----|------|--------|------|------|
| StartAddress | `start_address` | `uint16` | `0` | No | 시작 주소 |
| Count | `count` | `uint16` | - | Yes | 레지스터/코일 수 |
| InitialValues | `initial_values` | `[]any` | 모두 0/false | No | 초기값 배열 |

#### REQ-MODBUS-002-01-03 (Event-Driven) 필수 필드 검증

**WHEN** 설정 파싱 시 `register_map` 필드가 누락되거나 모든 영역의 `count`가 0이면 **THEN** 명확한 에러 메시지와 함께 설정 오류를 반환해야 한다.

#### REQ-MODBUS-002-01-04 (Unwanted) 유효하지 않은 설정 거부

시스템은 다음 유효하지 않은 설정을 **수락하지 않아야 한다**:
- `listen_port` 범위 초과 (1~65535)
- `unit_id` 범위 초과 (0~247)
- `max_connections` 0 이하
- 개별 영역의 `count`가 최대 수량 초과 (Coils: 65536, Registers: 65536)
- `initial_values` 배열 길이가 `count`를 초과

**YAML 설정 예시:**
```yaml
agents:
  - id: "modbus-server-01"
    name: "MODBUS Register Server"
    type: "modbus-tcp-server"
    transport:
      type: "custom"
      options:
        listen_address: "0.0.0.0"
        listen_port: 502
        unit_id: 1
        max_connections: 10
        idle_timeout: "60s"
        msg_channel_size: 256
        register_map:
          coils:
            start_address: 0
            count: 100
            initial_values: [false, true, false, false]
          discrete_inputs:
            start_address: 0
            count: 50
          holding_registers:
            start_address: 0
            count: 100
            initial_values: [0, 100, 200, 300]
          input_registers:
            start_address: 0
            count: 50
            initial_values: [1000, 2000, 3000]
```

---

### Module 2: TCP Listener (TCP 리스너)

#### REQ-MODBUS-002-02-01 (Event-Driven) TCP 리스닝 시작

**WHEN** `Start(ctx)` 호출 시 **THEN** 설정된 `listen_address:listen_port`에서 TCP 리스닝을 시작해야 한다. 리스닝 시작 후 로그에 주소와 포트를 기록한다.

#### REQ-MODBUS-002-02-02 (Event-Driven) 클라이언트 연결 수락

**WHEN** 외부 MODBUS 클라이언트가 TCP 연결을 요청하면 **THEN** 연결을 수락하고, 해당 클라이언트 전용 goroutine을 시작하여 MBAP 프레임을 처리해야 한다.

#### REQ-MODBUS-002-02-03 (State-Driven) 최대 연결 수 제한

**IF** 현재 연결 수가 `max_connections`에 도달한 상태이면 **THEN** 신규 연결 수락을 거부하고 즉시 TCP 연결을 닫아야 한다. 거부 사실을 로그에 기록한다.

#### REQ-MODBUS-002-02-04 (Event-Driven) 클라이언트 연결/해제 알림

**WHEN** 클라이언트가 연결되거나 연결이 해제되면 **THEN** `client_connected` 또는 `client_disconnected` 이벤트를 `msgCh`에 전달해야 한다.

```json
{
  "type": "client_connected",
  "remote_addr": "192.168.1.50:54321",
  "active_connections": 3,
  "timestamp": "2026-02-26T10:30:00Z"
}
```

#### REQ-MODBUS-002-02-05 (Event-Driven) Graceful Shutdown

**WHEN** `Stop(ctx)` 호출 시 **THEN** TCP 리스너를 닫고, 모든 활성 클라이언트 연결을 gracefully 종료해야 한다. context 타임아웃 내에 완료되지 않으면 강제 종료한다.

#### REQ-MODBUS-002-02-06 (Event-Driven) 유휴 클라이언트 타임아웃

**WHEN** 클라이언트가 `idle_timeout` 동안 어떤 요청도 보내지 않으면 **THEN** 해당 연결을 닫고 `client_disconnected` 이벤트를 전송해야 한다.

---

### Module 3: Connection Handler (연결 핸들러)

#### REQ-MODBUS-002-03-01 (Ubiquitous) MBAP 프레임 읽기

시스템은 **항상** TCP 스트림에서 MBAP Header(7 bytes)를 먼저 읽고, Length 필드에 따라 나머지 PDU를 읽어 완전한 MODBUS 요청을 조립해야 한다.

#### REQ-MODBUS-002-03-02 (Event-Driven) Unit ID 불일치 처리

**WHEN** 요청의 Unit ID가 서버 설정의 Unit ID와 일치하지 않으면 **THEN** 해당 요청을 무시(응답 없음)해야 한다. 불일치 사실을 Debug 레벨 로그에 기록한다.

#### REQ-MODBUS-002-03-03 (Ubiquitous) Transaction ID 에코

시스템은 **항상** 요청의 Transaction ID를 응답의 Transaction ID에 그대로 복사해야 한다. 이를 통해 클라이언트가 요청-응답을 매칭할 수 있다.

#### REQ-MODBUS-002-03-04 (Event-Driven) 불완전한 프레임 처리

**WHEN** TCP 읽기 중 MBAP Header가 불완전하거나 Length 필드가 비정상적이면 **THEN** 해당 연결을 닫고 에러를 로그에 기록해야 한다.

#### REQ-MODBUS-002-03-05 (Unwanted) 버퍼 오버플로우 방지

시스템은 Length 필드가 비정상적으로 큰 값(> 253 bytes)을 가진 요청을 **수락하지 않아야 한다**. 연결을 닫고 로그에 기록한다.

---

### Module 4: Request Handler (요청 핸들러)

#### REQ-MODBUS-002-04-01 (Ubiquitous) 읽기 Function Code 처리

시스템은 **항상** 다음 읽기 Function Code를 처리하여 레지스터 맵에서 값을 읽어 응답해야 한다:

| Function Code | 이름 | 대상 영역 | 데이터 단위 |
|---------------|------|----------|-------------|
| FC01 (0x01) | Read Coils | Coils | 1 bit |
| FC02 (0x02) | Read Discrete Inputs | Discrete Inputs | 1 bit |
| FC03 (0x03) | Read Holding Registers | Holding Registers | 16 bits |
| FC04 (0x04) | Read Input Registers | Input Registers | 16 bits |

#### REQ-MODBUS-002-04-02 (Ubiquitous) 쓰기 Function Code 처리

시스템은 **항상** 다음 쓰기 Function Code를 처리하여 레지스터 맵에 값을 쓰고 응답해야 한다:

| Function Code | 이름 | 대상 영역 | 설명 |
|---------------|------|----------|------|
| FC05 (0x05) | Write Single Coil | Coils | 값: 0xFF00(ON) 또는 0x0000(OFF) |
| FC06 (0x06) | Write Single Register | Holding Registers | 값: 0x0000~0xFFFF |
| FC15 (0x0F) | Write Multiple Coils | Coils | 최대 1968 coils |
| FC16 (0x10) | Write Multiple Registers | Holding Registers | 최대 123 registers |

#### REQ-MODBUS-002-04-03 (Event-Driven) 지원하지 않는 Function Code

**WHEN** 지원하지 않는 Function Code가 포함된 요청을 수신하면 **THEN** Exception Code 0x01 (Illegal Function)을 포함한 예외 응답을 반환해야 한다.

#### REQ-MODBUS-002-04-04 (Event-Driven) 주소 범위 초과

**WHEN** 요청의 시작 주소 + 수량이 설정된 레지스터 맵 범위를 초과하면 **THEN** Exception Code 0x02 (Illegal Data Address)를 포함한 예외 응답을 반환해야 한다.

#### REQ-MODBUS-002-04-05 (Event-Driven) 유효하지 않은 데이터 값

**WHEN** FC05 요청에서 값이 0xFF00 또는 0x0000이 아니면 **THEN** Exception Code 0x03 (Illegal Data Value)을 포함한 예외 응답을 반환해야 한다.

#### REQ-MODBUS-002-04-06 (Unwanted) 읽기 전용 영역 쓰기 거부

시스템은 Discrete Inputs(FC02 영역)와 Input Registers(FC04 영역)에 대한 외부 클라이언트의 쓰기 요청을 **수락하지 않아야 한다**. Exception Code 0x01 (Illegal Function)을 반환한다.

#### REQ-MODBUS-002-04-07 (Ubiquitous) 수량 제한 검증

시스템은 **항상** 다음 수량 제한을 검증해야 한다:
- 읽기: FC01/FC02 최대 2000개 coils, FC03/FC04 최대 125개 registers
- 쓰기: FC15 최대 1968개 coils, FC16 최대 123개 registers

수량이 0이거나 최대값을 초과하면 Exception Code 0x03 (Illegal Data Value)을 반환한다.

---

### Module 5: Register Map (레지스터 맵)

#### REQ-MODBUS-002-05-01 (Ubiquitous) 레지스터 맵 자료구조

시스템은 **항상** 다음 구조의 공유 레지스터 맵을 유지해야 한다:

| 필드 | 타입 | 설명 |
|------|------|------|
| Coils | `map[uint16]bool` | 코일 영역 (읽기/쓰기) |
| DiscreteInputs | `map[uint16]bool` | 이산 입력 영역 (외부 클라이언트 읽기 전용) |
| HoldingRegisters | `map[uint16]uint16` | 보유 레지스터 (읽기/쓰기) |
| InputRegisters | `map[uint16]uint16` | 입력 레지스터 (외부 클라이언트 읽기 전용) |
| mu | `sync.RWMutex` | 동시성 보호 |

#### REQ-MODBUS-002-05-02 (Event-Driven) 설정 기반 초기화

**WHEN** 에이전트가 초기화되면 **THEN** `register_map` 설정에 따라 레지스터 맵을 초기화해야 한다. `initial_values`가 지정되면 해당 값으로, 지정되지 않으면 0/false로 초기화한다.

#### REQ-MODBUS-002-05-03 (Ubiquitous) 동시성 안전 접근

시스템은 **항상** 레지스터 맵에 대한 모든 읽기/쓰기 접근을 `sync.RWMutex`로 보호해야 한다. 읽기 요청은 `RLock`, 쓰기 요청과 Bridge Process() 명령은 `Lock`을 사용한다.

#### REQ-MODBUS-002-05-04 (Event-Driven) 변경 추적

**WHEN** 레지스터 맵의 값이 변경되면(외부 클라이언트 쓰기 또는 내부 Process() 명령) **THEN** 변경된 영역, 주소, 이전 값, 새 값을 추적하여 이벤트를 생성해야 한다.

#### REQ-MODBUS-002-05-05 (Ubiquitous) 스냅샷 조회

시스템은 **항상** 레지스터 맵의 전체 또는 특정 영역에 대한 읽기 잠금 스냅샷을 반환할 수 있어야 한다.

---

### Module 6: Bridge Integration (Bridge 연동)

#### REQ-MODBUS-002-06-01 (Ubiquitous) Process() 명령 지원

시스템은 **항상** Bridge `Process(data []byte)` 를 통해 다음 JSON 명령을 처리해야 한다:

| 명령 | 설명 | 대상 영역 |
|------|------|----------|
| `set_coil` | 코일 값 설정 | Coils |
| `set_coils` | 다수 코일 값 설정 | Coils |
| `set_register` | 보유 레지스터 값 설정 | Holding Registers |
| `set_registers` | 다수 보유 레지스터 값 설정 | Holding Registers |
| `set_input` | 입력 레지스터/이산 입력 값 설정 (내부 전용) | Input Registers / Discrete Inputs |
| `set_inputs` | 다수 입력 레지스터/이산 입력 값 설정 (내부 전용) | Input Registers / Discrete Inputs |
| `get_map` | 전체 레지스터 맵 스냅샷 조회 | 전체 |
| `get_status` | 서버 상태 조회 (연결 수, 가동 시간 등) | - |

#### REQ-MODBUS-002-06-02 (Event-Driven) set_coil / set_coils 명령 처리

**WHEN** `set_coil` 명령이 수신되면 **THEN** 지정된 주소의 코일 값을 변경하고 `register_updated` 이벤트를 `msgCh`에 전달해야 한다.

```json
{
  "command": "set_coil",
  "params": {
    "address": 0,
    "value": true
  }
}
```

```json
{
  "command": "set_coils",
  "params": {
    "address": 0,
    "values": [true, false, true]
  }
}
```

#### REQ-MODBUS-002-06-03 (Event-Driven) set_register / set_registers 명령 처리

**WHEN** `set_register` 명령이 수신되면 **THEN** 지정된 주소의 보유 레지스터 값을 변경하고 `register_updated` 이벤트를 `msgCh`에 전달해야 한다.

```json
{
  "command": "set_register",
  "params": {
    "address": 0,
    "value": 1234
  }
}
```

```json
{
  "command": "set_registers",
  "params": {
    "address": 0,
    "values": [1234, 5678, 9012]
  }
}
```

#### REQ-MODBUS-002-06-04 (Event-Driven) set_input / set_inputs 명령 처리

**WHEN** `set_input` 명령이 수신되면 **THEN** 지정된 주소의 Input Register 또는 Discrete Input 값을 변경하고 `register_updated` 이벤트를 `msgCh`에 전달해야 한다. 외부 MODBUS 클라이언트는 이 영역에 쓸 수 없으므로 내부 Process() 전용 기능이다.

```json
{
  "command": "set_input",
  "params": {
    "area": "input_registers",
    "address": 0,
    "value": 5000
  }
}
```

```json
{
  "command": "set_inputs",
  "params": {
    "area": "discrete_inputs",
    "address": 0,
    "values": [true, false, true]
  }
}
```

#### REQ-MODBUS-002-06-05 (Event-Driven) get_map 명령 처리

**WHEN** `get_map` 명령이 수신되면 **THEN** 전체 레지스터 맵의 스냅샷을 JSON으로 반환해야 한다.

#### REQ-MODBUS-002-06-06 (Event-Driven) get_status 명령 처리

**WHEN** `get_status` 명령이 수신되면 **THEN** 서버 상태(리스닝 주소, 활성 연결 수, 가동 시간, 처리된 요청 수)를 JSON으로 반환해야 한다.

#### REQ-MODBUS-002-06-07 (Ubiquitous) ReceiveMessage 구현

시스템은 **항상** `MessageReceiver` 인터페이스를 구현하여 `ReceiveMessage(ctx)` 메서드를 통해 `msgCh` 채널에서 이벤트를 전달해야 한다.

#### REQ-MODBUS-002-06-08 (Event-Driven) 외부 쓰기 알림 (register_changed)

**WHEN** 외부 MODBUS 클라이언트가 FC05/FC06/FC15/FC16으로 레지스터에 쓰기를 수행하면 **THEN** `register_changed` 이벤트를 `msgCh`에 전달해야 한다.

```json
{
  "type": "register_changed",
  "source": "external",
  "remote_addr": "192.168.1.50:54321",
  "function_code": 6,
  "area": "holding_registers",
  "address": 10,
  "quantity": 1,
  "values": [1234],
  "timestamp": "2026-02-26T10:30:00Z"
}
```

#### REQ-MODBUS-002-06-09 (Event-Driven) 내부 설정 알림 (register_updated)

**WHEN** Bridge Process() 명령으로 레지스터 값이 변경되면 **THEN** `register_updated` 이벤트를 `msgCh`에 전달해야 한다.

```json
{
  "type": "register_updated",
  "source": "internal",
  "command": "set_register",
  "area": "holding_registers",
  "address": 10,
  "quantity": 1,
  "values": [1234],
  "timestamp": "2026-02-26T10:30:00Z"
}
```

---

### Module 7: Agent Lifecycle (에이전트 생명주기)

#### REQ-MODBUS-002-07-01 (Event-Driven) Init 생명주기

**WHEN** `Init(config)` 호출 시 **THEN**:
1. `config.Transport.Options`에서 서버 설정을 파싱한다
2. RegisterMap을 초기화한다 (설정의 initial_values 적용)
3. `msgCh` 채널을 생성한다
4. 상태를 `Running`으로 전이한다

#### REQ-MODBUS-002-07-02 (Event-Driven) Start 생명주기

**WHEN** `Start(ctx)` 호출 시 **THEN**:
1. TCP 리스너를 시작한다
2. 연결 수락 goroutine을 시작한다

#### REQ-MODBUS-002-07-03 (Event-Driven) Stop 생명주기

**WHEN** `Stop(ctx)` 호출 시 **THEN**:
1. TCP 리스너를 닫는다
2. 모든 클라이언트 연결을 종료한다
3. `msgCh`를 드레인한다
4. 상태를 `Stopped`로 전이한다

#### REQ-MODBUS-002-07-04 (Event-Driven) Pause / Resume

**WHEN** `Pause(ctx)` 호출 시 **THEN** 리스너는 유지하되 새로운 요청 처리를 일시 중단해야 한다 (기존 연결은 유지).

**WHEN** `Resume(ctx)` 호출 시 **THEN** 요청 처리를 재개해야 한다.

#### REQ-MODBUS-002-07-05 (Ubiquitous) Health 상태

시스템은 **항상** 다음 기준으로 Health 상태를 보고해야 한다:
- `Healthy`: Running 상태, TCP 리스너 활성
- `Degraded`: Paused 상태
- `Unhealthy`: Stopped 또는 리스너 실패

#### REQ-MODBUS-002-07-06 (Ubiquitous) StatefulAgent.State() 구현

시스템은 **항상** `StatefulAgent` 인터페이스를 구현하여 다음 정보를 노출해야 한다:
- `listen_address`: 리스닝 주소
- `listen_port`: 리스닝 포트
- `unit_id`: 서버 Unit ID
- `active_connections`: 현재 활성 연결 수
- `max_connections`: 최대 연결 수
- `total_requests`: 처리된 총 요청 수
- `register_map`: 현재 레지스터 맵 스냅샷

#### REQ-MODBUS-002-07-07 (Ubiquitous) 에이전트 정보

시스템은 **항상** `ID()`, `Name()`, `Type()`, `Info()`, `Stats()` 메서드를 구현해야 한다. `Type()`은 `"modbus-tcp-server"`를 반환한다.

---

### Module 8: Registration & Examples (타입 등록 및 예제)

#### REQ-MODBUS-002-08-01 (Ubiquitous) TypeRegistry 등록

시스템은 **항상** `RegisterModbusServerTypes(registry)` 함수로 `"modbus-tcp-server"` 타입을 에이전트 팩토리에 등록해야 한다.

#### REQ-MODBUS-002-08-02 (Event-Driven) main.go 등록

**WHEN** xflowd 서버가 시작되면 **THEN** `cmd/xflowd/main.go`에서 `RegisterModbusServerTypes`를 호출하여 타입을 등록해야 한다.

#### REQ-MODBUS-002-08-03 (Optional) 예제 YAML 파일

**가능하면** 다음 예제 파일을 제공한다:
- `examples/agents/modbus-server.yaml`: 서버 에이전트 설정 예시
- `examples/flows/modbus-server-bridge.yaml`: Bridge 연동 플로우 예시

---

### Module 9: Error Handling (에러 처리)

#### REQ-MODBUS-002-09-01 (Ubiquitous) 서버 전용 센티널 에러

시스템은 **항상** 다음 서버 전용 센티널 에러를 정의해야 한다:

| 에러 변수 | 설명 |
|-----------|------|
| `ErrServerAlreadyRunning` | 서버가 이미 리스닝 중 |
| `ErrListenFailed` | TCP 리스닝 시작 실패 |
| `ErrMaxConnectionsReached` | 최대 연결 수 도달 |
| `ErrInvalidRegisterMap` | 유효하지 않은 레지스터 맵 설정 |
| `ErrAddressNotMapped` | 맵에 정의되지 않은 주소 접근 |
| `ErrReadOnlyArea` | 읽기 전용 영역(DI/IR)에 외부 쓰기 시도 |

모든 에러는 `errors.New()`로 정의하며, `errors.Is()`로 비교 가능해야 한다.

기존 `modbus` 패키지의 공통 에러(`ErrFrameTooShort`, `ErrInvalidCommand` 등)도 재사용한다.

#### REQ-MODBUS-002-09-02 (Ubiquitous) 구조화 로깅

시스템은 **항상** `slog` 패키지를 사용한 구조화 로깅을 적용해야 한다. 로그 메시지에는 다음 필드를 포함한다:
- `remote_addr`: 클라이언트 주소
- `function_code`: MODBUS Function Code
- `address`: 레지스터 주소
- `unit_id`: 요청의 Unit ID
- `error`: 에러 메시지 (오류 발생 시)

#### REQ-MODBUS-002-09-03 (Ubiquitous) MODBUS Exception 응답 생성

시스템은 **항상** 에러 발생 시 표준 MODBUS Exception 응답을 생성해야 한다:
- Function Code: 원래 FC + 0x80
- Exception Code: 1 byte (0x01~0x04)

---

## 4. Specifications (상세 명세)

### 4.1 파일 구조

```
internal/agent/modbusserver/
├── config.go          # ModbusServerConfig, RegisterMapConfig 파싱 및 검증
├── register_map.go    # RegisterMap 구조체 (공유 레지스터 저장소, 변경 추적)
├── listener.go        # TCP 리스너, 연결 수락, 연결 관리
├── handler.go         # 연결별 MBAP 프레임 읽기 및 FC 디스패치
├── request.go         # FC01~FC04 읽기 핸들러, FC05/FC06/FC15/FC16 쓰기 핸들러
├── agent.go           # ModbusServerAgent (Init, Start, Stop, Pause, Resume, Process, ReceiveMessage, State)
├── register.go        # RegisterModbusServerTypes 타입 등록 함수
├── errors.go          # 서버 전용 센티널 에러 정의
├── config_test.go     # 설정 파싱 테스트
├── register_map_test.go # RegisterMap 테스트
├── handler_test.go    # 핸들러 테스트 (net.Pipe 기반)
├── request_test.go    # 요청 핸들러 테스트
├── agent_test.go      # 에이전트 통합 테스트
└── listener_test.go   # TCP 리스너 테스트
```

### 4.2 타입 등록

`RegisterModbusServerTypes(mgr)` 함수로 `"modbus-tcp-server"` 타입을 에이전트 팩토리에 등록한다.

### 4.3 재사용 코드 (import)

`internal/agent/modbus` 패키지에서 다음을 import하여 사용한다:
- `MBAPHeaderSize`, `MBAPProtocolID` 상수
- `FC01ReadCoils` ~ `FC16WriteMultipleRegisters` Function Code 상수
- `ExceptionIllegalFunction` ~ `ExceptionSlaveDeviceFailure` Exception Code 상수
- `MaxCoilsRead`, `MaxRegistersRead`, `MaxCoilsWrite`, `MaxRegistersWrite` 수량 상수
- `MBAPHeader` 구조체
- `decodeCoils()`, `decodeRegisters()` 디코딩 유틸리티
- `ModbusException` 에러 타입

### 4.4 Traceability (추적성)

| 요구사항 그룹 | Module | 파일 |
|-------------|--------|------|
| REQ-MODBUS-002-01-* | Module 1: Server Configuration | config.go |
| REQ-MODBUS-002-02-* | Module 2: TCP Listener | listener.go |
| REQ-MODBUS-002-03-* | Module 3: Connection Handler | handler.go |
| REQ-MODBUS-002-04-* | Module 4: Request Handler | request.go |
| REQ-MODBUS-002-05-* | Module 5: Register Map | register_map.go |
| REQ-MODBUS-002-06-* | Module 6: Bridge Integration | agent.go |
| REQ-MODBUS-002-07-* | Module 7: Agent Lifecycle | agent.go |
| REQ-MODBUS-002-08-* | Module 8: Registration & Examples | register.go, cmd/xflowd/main.go |
| REQ-MODBUS-002-09-* | Module 9: Error Handling | errors.go |

---

---

## 5. Implementation Notes (구현 노트)

> 이 섹션은 구현 완료 후 추가되었습니다. (Level 1: spec-first)

### 5.1 구현 요약

SPEC-MODBUS-002에서 정의한 MODBUS/TCP Server Agent가 전체 9개 모듈에 걸쳐 완전히 구현되었다.

- **패키지 경로**: `internal/agent/modbusserver/`
- **타입 이름**: `modbus-tcp-server`
- **구현 인터페이스**: Agent, MessageReceiver, StatefulAgent

### 5.2 파일 목록

**소스 파일 (8개)**:

| 파일 | 모듈 | 설명 |
|------|------|------|
| `errors.go` | Module 9 | 7개 센티널 에러 (ErrServerAlreadyRunning, ErrListenFailed, ErrMaxConnectionsReached, ErrInvalidRegisterMap, ErrAddressNotMapped, ErrReadOnlyArea, ErrInvalidCommand) |
| `config.go` | Module 1 | ModbusServerConfig 파싱 및 전체 검증 (listen_address, listen_port, unit_id, max_connections, idle_timeout, msg_channel_size, register_map) |
| `register_map.go` | Module 5 | RegisterMap (4개 영역: Coils, DiscreteInputs, HoldingRegisters, InputRegisters), CRUD, ChangeSet 추적, GetSnapshot |
| `request.go` | Module 4 | RequestHandler (FC01-FC04 읽기, FC05/FC06/FC15/FC16 쓰기), encode/decode 헬퍼, Exception PDU 빌더 |
| `handler.go` | Module 3 | ModbusHandler (MBAP 프레임 파싱), ConnectionHandler 인터페이스, msgCh 변경 알림 |
| `listener.go` | Module 2 | TCP Listener (연결 제한 maxConns, 유휴 타임아웃, Graceful Shutdown, Addr() 테스트 지원) |
| `agent.go` | Module 6, 7 | ModbusServerAgent (Agent + MessageReceiver + StatefulAgent), Process() 명령 (set_coil, set_coils, set_register, set_registers, set_input, set_inputs, get_map, get_status), ReceiveMessage |
| `register.go` | Module 8 | RegisterModbusServerTypes 타입 등록 (`modbus-tcp-server`) |

**테스트 파일 (6개)**:

| 파일 | 테스트 수 | 대상 |
|------|-----------|------|
| `config_test.go` | 14 | 설정 파싱 및 검증 |
| `register_map_test.go` | 16 | RegisterMap CRUD, 동시성, 변경 추적 |
| `request_test.go` | - | FC 핸들러 + 인코딩 테스트 |
| `handler_test.go` | 6 | MBAP 프레임 파싱, Unit ID 매칭 |
| `listener_test.go` | 6 | TCP 리스너, 연결 제한 |
| `agent_test.go` | 27 | 에이전트 통합 테스트 (TCP 포함) |

### 5.3 수정된 기존 파일

| 파일 | 변경 내용 |
|------|-----------|
| `cmd/xflowd/main.go` | modbusserver import 추가, RegisterModbusServerTypes 호출 |

### 5.4 예제 파일

| 파일 | 설명 |
|------|------|
| `examples/agents/modbus-server.yaml` | 서버 에이전트 설정 예시 |
| `examples/flows/modbus-server-bridge.yaml` | Bridge 연동 플로우 예시 (레지스터 변경 이벤트) |

### 5.5 품질 지표

| 항목 | 결과 |
|------|------|
| 테스트 커버리지 | 89.5% (목표 85% 초과) |
| Race Detection | Clean (데이터 레이스 없음) |
| go vet | Clean (경고 0건) |
| go build | Success |

### 5.6 상태 변경

- **이전 상태**: approved (v0.1.0)
- **현재 상태**: completed (v1.0.0)
- **완료 날짜**: 2026-02-27

---

*SPEC-MODBUS-002 v1.0.0*
*작성자: xtra*
*날짜: 2026-02-27*
