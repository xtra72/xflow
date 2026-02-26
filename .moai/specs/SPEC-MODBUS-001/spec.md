---
id: SPEC-MODBUS-001
version: "1.0.0"
status: approved
created: "2026-02-26"
updated: "2026-02-26"
author: xtra
priority: high
---

## HISTORY

| 버전 | 날짜 | 작성자 | 변경 내용 |
|------|------|--------|-----------|
| 1.0.0 | 2026-02-26 | xtra | 최초 작성 - 11개 모듈, 읽기/쓰기/동기화 통합 |

---

# SPEC-MODBUS-001: MODBUS/TCP Client Agent 구현

## 1. Environment (환경)

### 1.1 시스템 개요

xflow는 Go 기반 IoT FBP(Flow-Based Programming) 플랫폼이다. Agent 시스템은 Transport Interface(통신 인터페이스)와 Protocol Definition(프로토콜 정의)을 결합하여 외부 장비와 통신한다.

MODBUS/TCP Client Agent는 MODBUS/TCP 프로토콜(MBAP Header + PDU)을 통해 산업용 PLC, 센서, 액추에이터 등 MODBUS 디바이스와 TCP 소켓으로 통신하는 커스텀 에이전트이다. 다중 디바이스/주소 관리, 주기적 폴링(Interval Mode) 및 변경 감지(Event Mode)를 지원하며, 레지스터 읽기(FC01~FC04)와 쓰기(FC05/FC06/FC15/FC16)를 통한 양방향 제어를 제공한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**: `internal/agent/modbus/`
- **기존 인터페이스**:
  - `Agent` 인터페이스 (`internal/agent/agent.go`): Init, Start, Stop, Pause, Resume, Health, Process, Configure, ID, Name, Type, Info, Stats
  - `BaseAgent` 또는 `BaseLifecycle` 임베딩 패턴
  - `MessageReceiver` 인터페이스: ReceiveMessage(ctx) ([]byte, error)
  - `StatefulAgent` 인터페이스: State() map[string]any
  - `TypeRegistry`: RegisterType/CreateAgent
- **브릿지 노드** (`internal/node/bridge.go`):
  - `BridgeIn`: 에이전트 -> 플로우 (센서 데이터 수신)
  - `BridgeOut`: 플로우 -> 에이전트 (제어 명령 전송)
  - `BridgeInOut`: 양방향 (데이터 수신 + 제어 명령)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: `sync.RWMutex` (RegisterCache 보호)

### 1.3 설계 원칙

- **기존 패턴 준수**: MQTT Agent, Samsung NASA Agent의 설정 파싱, 생명주기 관리, Bridge 연동 패턴 활용
- **관심사 분리**: 에이전트 로직, 설정 파싱, 레지스터 캐시, MODBUS 프로토콜, TCP 연결 관리를 별도 파일로 분리
- **테스트 가능성**: 인터페이스 기반 설계로 TCP 연결을 mock 주입하여 단위 테스트 가능
- **다중 디바이스 격리**: 단일 에이전트 인스턴스에서 여러 MODBUS 디바이스를 독립적으로 관리하며, 한 디바이스의 오류가 다른 디바이스에 영향을 주지 않음
- **Write-Through 캐시**: 쓰기 작업은 디바이스 응답 성공 후에만 캐시를 갱신하여 데이터 정합성 보장

### 1.4 범위 경계

- **범위 내(In-Scope)**:
  - MODBUSAgent 구현 (Agent, MessageReceiver, StatefulAgent 인터페이스 준수)
  - MODBUS/TCP 프로토콜 (MBAP Header + PDU) 구현
  - Function Code FC01(Read Coils), FC02(Read Discrete Inputs), FC03(Read Holding Registers), FC04(Read Input Registers) 지원
  - Function Code FC05(Write Single Coil), FC06(Write Single Register), FC15(Write Multiple Coils), FC16(Write Multiple Registers) 지원
  - 다중 디바이스(host:port 또는 Unit ID 기반) 관리
  - RegisterCache 기반 레지스터 동기화
  - Cached/Direct 읽기 모드 지원
  - Interval Mode / Event Mode 지원
  - Bridge 연동 (BridgeIn, BridgeInOut 방향)
  - TCP 연결 관리 및 자동 재연결
  - Health 상태 관리 (Healthy, Degraded, Unhealthy)
  - 구조화 로깅 (slog)
  - 단위 테스트
- **범위 외(Out-of-Scope)**:
  - MODBUS RTU (시리얼 통신) 지원
  - MODBUS/TCP 서버(슬레이브) 구현
  - Bridge 노드 자체의 변경
  - 레지스터 주소 자동 탐색 (MODBUS 표준에 없음)
  - 실제 PLC 하드웨어 통합 테스트
  - MODBUS/TCP 보안 확장 (TLS)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-AGENT-001 | 의존 | Agent, MessageReceiver, StatefulAgent 인터페이스 |
| SPEC-BRIDGE-001 | 의존 | BridgeNode, BridgeConfig, AgentTransport |
| SPEC-ENGINE-001 | 의존 | AgentManagerResolver |
| SPEC-NASA-001 | 참조 | Samsung NASA Agent 구현 패턴 (유사 에이전트 아키텍처) |

---

## 2. Assumptions (가정)

### A-1. MODBUS/TCP 표준 준수

MODBUS/TCP 프로토콜은 MODBUS Application Protocol Specification V1.1b3 및 MODBUS Messaging on TCP/IP Implementation Guide V1.0b 표준을 따른다. MBAP Header(Transaction ID 2 bytes + Protocol ID 2 bytes + Length 2 bytes + Unit ID 1 byte) + PDU(Function Code 1 byte + Data) 구조를 사용한다.

### A-2. 네트워크 접근성

대상 MODBUS 디바이스가 TCP/IP 네트워크를 통해 접근 가능하며, 기본 포트 502를 사용한다고 가정한다. 방화벽 설정은 사전에 완료되어 있다.

### A-3. 레지스터 주소 사전 설정

제어 대상 MODBUS 디바이스의 레지스터 주소, Function Code, 수량이 사용자에 의해 사전에 알려져 있다고 가정한다. MODBUS 표준에는 레지스터 자동 탐색 메커니즘이 없다.

### A-4. 기존 인터페이스 구현 완료

`Agent`, `MessageReceiver`, `StatefulAgent` 인터페이스와 `BridgeNode`, `AgentTransport` 인터페이스가 구현되어 있다고 가정한다.

### A-5. 단일 마스터 통신

MODBUS/TCP 통신에서 이 에이전트는 클라이언트(마스터) 역할만 수행하며, 디바이스는 서버(슬레이브)로 동작한다고 가정한다.

---

## 3. Requirements (요구사항)

### Module 1: Connection Management (연결 관리)

#### REQ-MODBUS-001-01-01 (Ubiquitous) MODBUS/TCP 프로토콜 통신

시스템은 **항상** MODBUS/TCP 프로토콜(MBAP Header + PDU)을 TCP 소켓을 통해 구현해야 한다.

**MBAP Header 구조:**

| 필드 | 크기 | 설명 |
|------|------|------|
| Transaction ID | 2 bytes (Big-Endian) | 요청-응답 매칭용 식별자 (에이전트가 순차 증가) |
| Protocol ID | 2 bytes (Big-Endian) | 항상 `0x0000` (MODBUS 프로토콜) |
| Length | 2 bytes (Big-Endian) | Unit ID + PDU 크기 |
| Unit ID | 1 byte | 디바이스 식별자 (0x00~0xFF) |

#### REQ-MODBUS-001-01-02 (Event-Driven) 자동 재연결 (설정 간격)

**WHEN** TCP 연결 시도가 실패하면 **THEN** 설정된 `reconnect_interval`(기본값: 10초)에 따라 재연결을 시도해야 한다. 재연결 시도마다 로그에 기록한다.

#### REQ-MODBUS-001-01-03 (Event-Driven) 연결 끊김 시 자동 재연결 및 Health 저하

**WHEN** 기존 TCP 연결이 끊어지면 **THEN** 자동으로 재연결을 시도하고, Health 상태를 `Degraded`로 보고해야 한다. 재연결 성공 시 Health를 `Healthy`로 복원한다.

#### REQ-MODBUS-001-01-04 (State-Driven) 연결 끊김 상태에서의 동작

**IF** TCP 연결이 끊어진 상태이면 **THEN** Health 상태를 `Unhealthy`로 보고하고, 폴링을 일시 중단해야 한다. 재연결 성공 시 폴링을 자동으로 재개한다.

#### REQ-MODBUS-001-01-05 (Unwanted) 무한 재시도 방지

시스템은 무한 재연결 루프를 **허용하지 않아야 한다**. 최대 재연결 시도 횟수(`max_reconnect_attempts`, 기본값: 10)를 초과하면 해당 디바이스의 재연결을 중지하고 `ErrMaxReconnectExceeded` 에러를 로그에 기록하며, Health를 `Unhealthy`로 설정한다.

---

### Module 2: Register Reading (레지스터 읽기)

#### REQ-MODBUS-001-02-01 (Ubiquitous) Function Code 지원

시스템은 **항상** 다음 MODBUS Function Code를 사용한 레지스터 읽기를 지원해야 한다:

| Function Code | 이름 | 대상 | 데이터 단위 |
|---------------|------|------|-------------|
| FC01 (0x01) | Read Coils | Coil (읽기/쓰기) | 1 bit |
| FC02 (0x02) | Read Discrete Inputs | Discrete Input (읽기 전용) | 1 bit |
| FC03 (0x03) | Read Holding Registers | Holding Register (읽기/쓰기) | 16 bits (2 bytes) |
| FC04 (0x04) | Read Input Registers | Input Register (읽기 전용) | 16 bits (2 bytes) |

#### REQ-MODBUS-001-02-02 (Event-Driven) 폴링 주기 레지스터 순회

**WHEN** 폴링 주기(`poll_interval`)가 도래하면 **THEN** 설정된 모든 디바이스 및 레지스터 그룹을 순회하여 읽기 요청을 전송해야 한다. 각 디바이스의 모든 레지스터 그룹을 순차적으로 읽는다.

#### REQ-MODBUS-001-02-03 (Ubiquitous) MBAP Header 생성 규칙

시스템은 **항상** 올바른 MBAP Header를 생성해야 한다:
- Transaction ID: 요청마다 순차 증가 (0x0000~0xFFFF 순환)
- Protocol ID: 항상 `0x0000`
- Length: Unit ID(1 byte) + PDU 크기
- Unit ID: 디바이스 설정에서 지정된 값

#### REQ-MODBUS-001-02-04 (Event-Driven) MODBUS Exception 응답 처리

**WHEN** MODBUS Exception 응답(Function Code의 MSB가 1)이 수신되면 **THEN** Exception Code를 로그에 기록하고, 해당 디바이스/레지스터 그룹의 에러 카운터를 증가시켜야 한다.

**MODBUS Exception Code 정의:**

| Exception Code | 이름 | 설명 |
|---------------|------|------|
| 0x01 | Illegal Function | 지원하지 않는 Function Code |
| 0x02 | Illegal Data Address | 유효하지 않은 레지스터 주소 |
| 0x03 | Illegal Data Value | 유효하지 않은 데이터 값 |
| 0x04 | Server Device Failure | 디바이스 내부 오류 |

#### REQ-MODBUS-001-02-05 (Unwanted) 유효하지 않은 요청 방지

시스템은 유효하지 않은 Function Code 또는 주소 범위(0~65535 외)를 가진 MODBUS 요청을 **전송하지 않아야 한다**. 유효하지 않은 요청 시 `ErrInvalidFunctionCode` 또는 `ErrAddressOutOfRange` 에러를 반환한다.

---

### Module 3: Read Mode (읽기 모드)

#### REQ-MODBUS-001-03-01 (Ubiquitous) Cached 읽기 모드

시스템은 **항상** `read_mode: "cached"` (기본값) 모드에서 주기적 폴링을 수행하여 읽은 값을 `RegisterCache`에 저장해야 한다. 읽기 요청 시 캐시된 값을 반환한다. Interval Mode와 Event Mode는 캐시 데이터를 기반으로 동작한다.

#### REQ-MODBUS-001-03-02 (Ubiquitous) Direct 읽기 모드

시스템은 **항상** `read_mode: "direct"` 모드에서 주기적 폴링을 수행하지 않아야 한다. 각 읽기 요청마다 디바이스에 직접 쿼리하여 결과를 즉시 반환한다. Interval Mode와 Event Mode는 이 모드에서 적용되지 않는다.

#### REQ-MODBUS-001-03-03 (Event-Driven) Cached 모드 강제 읽기

**WHEN** `read_mode: "cached"` 모드에서 `force: true` 파라미터가 포함된 읽기 요청이 수신되면 **THEN** 캐시를 무시하고 디바이스에 직접 쿼리한 후, 결과를 캐시에 갱신하고 반환해야 한다.

#### REQ-MODBUS-001-03-04 (Event-Driven) Direct 모드 읽기 명령 처리

**WHEN** `read_mode: "direct"` 모드에서 `read_registers` 명령이 `Process()`를 통해 수신되면 **THEN** 디바이스에 직접 쿼리하여 결과를 즉시 반환해야 한다.

#### REQ-MODBUS-001-03-05 (Ubiquitous) 기본 읽기 모드

시스템은 **항상** `read_mode`의 기본값을 `"cached"`로 설정하여 하위 호환성을 유지해야 한다.

---

### Module 4: Multi-Device/Address (다중 디바이스/주소)

#### REQ-MODBUS-001-04-01 (Ubiquitous) 다중 디바이스 관리

시스템은 **항상** 단일 에이전트 인스턴스에서 여러 MODBUS 디바이스(서로 다른 host:port 또는 동일 host:port의 서로 다른 Unit ID)를 관리할 수 있어야 한다. 각 디바이스는 고유한 `device_id`로 식별된다.

#### REQ-MODBUS-001-04-02 (Ubiquitous) 디바이스별 독립 레지스터 설정

시스템은 **항상** 각 디바이스에 대해 독립적인 레지스터 주소 목록을 설정할 수 있어야 한다. 각 레지스터 그룹은 Function Code, 시작 주소, 수량을 포함한다.

#### REQ-MODBUS-001-04-03 (Unwanted) 디바이스 간 오류 전파 방지

시스템은 하나의 디바이스에서 발생한 통신 오류가 다른 디바이스의 폴링에 영향을 **주지 않아야 한다**. 오류가 발생한 디바이스를 건너뛰고 다음 디바이스의 폴링을 계속한다.

#### REQ-MODBUS-001-04-04 (Optional) 디바이스별 독립 폴링 간격

**가능하면** 각 디바이스에 대해 독립적인 폴링 간격(`poll_interval`)을 설정할 수 있도록 제공한다. 설정되지 않은 경우 에이전트 전역 `poll_interval`을 사용한다.

---

### Module 5: Interval Mode (인터벌 모드)

#### REQ-MODBUS-001-05-01 (Complex) 인터벌 모드 데이터 전송

**IF** `mode`가 `"interval"`이고 **AND** `read_mode`가 `"cached"`이면 **THEN** 매 폴링 주기마다 수집된 전체 레지스터 데이터를 JSON Message로 변환하여 `msgCh`에 즉시 전달해야 한다.

#### REQ-MODBUS-001-05-02 (Ubiquitous) JSON Message 변환 및 전달

시스템은 **항상** 수집된 레지스터 데이터를 JSON Message(Payload + Metadata)로 변환하여 `msgCh` 채널을 통해 Bridge에 즉시 전달해야 한다.

---

### Module 6: Event Mode (이벤트 모드)

#### REQ-MODBUS-001-06-01 (Complex) 이벤트 모드 변경 감지 전송

**IF** `mode`가 `"event"`이고 **AND** `read_mode`가 `"cached"`이면 **THEN** 폴링 주기마다 현재 읽은 값을 `RegisterCache`의 이전 값과 비교하여, 변경된 데이터만 `msgCh`에 전달해야 한다.

#### REQ-MODBUS-001-06-02 (Event-Driven) 변경 데이터 전달

**WHEN** 레지스터 값이 이전 캐시와 다른 것이 감지되면 **THEN** 변경된 레지스터 데이터만 JSON 형식으로 `msgCh`에 전달해야 한다.

#### REQ-MODBUS-001-06-03 (Event-Driven) Heartbeat 전체 상태 전송

**WHEN** Event Mode에서 heartbeat 타이머(`heartbeat_interval`)가 만료되면 **THEN** 변경 여부와 무관하게 전체 레지스터 상태를 `msgCh`에 전달해야 한다.

#### REQ-MODBUS-001-06-04 (Ubiquitous) RegisterCache 기반 이전 값 관리

시스템은 **항상** `RegisterCache`를 통해 디바이스/레지스터별 이전 값(`lastStates`)을 유지하여 Event Mode의 변경 감지에 활용해야 한다.

---

### Module 7: Bridge Integration (브릿지 통합)

#### REQ-MODBUS-001-07-01 (Ubiquitous) MessageReceiver 인터페이스 구현

시스템은 **항상** `MessageReceiver` 인터페이스를 구현하여 BridgeIn 방향으로 레지스터 데이터를 전달해야 한다. `ReceiveMessage(ctx)` 메서드는 내부 `msgCh` 채널과 `ctx.Done()` 채널을 `select`로 대기한다.

#### REQ-MODBUS-001-07-02 (Ubiquitous) JSON Message 포맷

시스템은 **항상** Bridge를 통해 전달되는 메시지를 JSON 포맷(Payload + Metadata)으로 구성해야 한다.

**Interval Mode 메시지 예시:**
```json
{
  "type": "register_data",
  "device_id": "plc-01",
  "unit_id": 1,
  "mode": "interval",
  "registers": {
    "holding_registers": {
      "40001": 1234,
      "40002": 5678
    },
    "coils": {
      "00001": true,
      "00002": false
    }
  },
  "timestamp": "2026-02-26T10:30:00Z"
}
```

**Event Mode 메시지 예시:**
```json
{
  "type": "register_changed",
  "device_id": "plc-01",
  "unit_id": 1,
  "mode": "event",
  "changed_registers": {
    "holding_registers": {
      "40001": {"old": 1234, "new": 1235}
    }
  },
  "timestamp": "2026-02-26T10:30:05Z"
}
```

#### REQ-MODBUS-001-07-03 (Ubiquitous) Metadata 포함

시스템은 **항상** 메시지 Metadata에 다음 정보를 포함해야 한다:
- `device_id`: 디바이스 식별자
- `unit_id`: MODBUS Unit ID
- `timestamp`: ISO 8601 형식 타임스탬프
- `mode`: 동작 모드 (`"interval"` 또는 `"event"`)

#### REQ-MODBUS-001-07-04 (Ubiquitous) BridgeInOut 양방향 지원

시스템은 **항상** `BridgeInOut` 방향을 지원하여 레지스터 데이터 수신(BridgeIn)과 쓰기/읽기 명령 전송(BridgeOut)을 동시에 처리할 수 있어야 한다.

#### REQ-MODBUS-001-07-05 (Event-Driven) Process() 명령 처리

**WHEN** Bridge를 통해 `Process(data []byte)` 가 호출되면 **THEN** JSON 데이터를 파싱하여 다음 명령을 처리해야 한다:

**쓰기 명령:**
- `write_coil`: FC05 단일 코일 쓰기
- `write_register`: FC06 단일 레지스터 쓰기
- `write_coils`: FC15 다중 코일 쓰기
- `write_registers`: FC16 다중 레지스터 쓰기

**읽기 명령:**
- `read_registers`: 지정된 레지스터 직접 읽기
- `get_cache`: 특정 디바이스/레지스터 그룹의 캐시 조회
- `get_all_caches`: 전체 캐시 조회
- `refresh_cache`: 특정 디바이스의 캐시 강제 갱신

---

### Module 8: Lifecycle (라이프사이클)

#### REQ-MODBUS-001-08-01 (Event-Driven) Init 생명주기

**WHEN** `Init(config)` 호출 시 **THEN**:
1. `config.Transport.Options`에서 MODBUS 전용 설정을 파싱한다 (`parseModbusConfig`)
2. 설정된 디바이스 목록으로 `deviceCaches` 맵을 초기화한다
3. 각 디바이스에 대해 `RegisterCache`를 초기 생성한다
4. `msgCh` 채널을 생성한다
5. 설정 유효성을 검증한다 (필수 필드, 주소 범위 등)

#### REQ-MODBUS-001-08-02 (Event-Driven) Stop 생명주기

**WHEN** `Stop(ctx)` 호출 시 **THEN**:
1. 폴링 고루틴을 중지한다 (`stopCh` 채널 닫기)
2. 모든 TCP 연결을 닫는다
3. `msgCh` 채널을 정리한다
4. 상태를 `Stopped`로 전이한다

#### REQ-MODBUS-001-08-03 (Event-Driven) Pause 생명주기

**WHEN** `Pause(ctx)` 호출 시 **THEN** 폴링을 일시 중지하되 TCP 연결은 유지해야 한다.

#### REQ-MODBUS-001-08-04 (Event-Driven) Resume 생명주기

**WHEN** `Resume(ctx)` 호출 시 **THEN** 중지된 폴링을 재개해야 한다.

#### REQ-MODBUS-001-08-05 (Ubiquitous) StatefulAgent 인터페이스 구현

시스템은 **항상** `StatefulAgent` 인터페이스를 구현하여 `State()` 메서드를 통해 다음 정보를 노출해야 한다:

```json
{
  "device_count": 2,
  "read_mode": "cached",
  "mode": "interval",
  "devices": {
    "plc-01": {
      "unit_id": 1,
      "host": "192.168.1.100",
      "port": 502,
      "online": true,
      "register_groups": [
        {
          "function_code": 3,
          "start_address": 0,
          "quantity": 10,
          "last_update": "2026-02-26T10:30:00Z",
          "stale": false
        }
      ]
    }
  }
}
```

---

### Module 9: Register Write (레지스터 쓰기)

#### REQ-MODBUS-001-09-01 (Ubiquitous) 쓰기 Function Code 지원

시스템은 **항상** 다음 MODBUS Function Code를 사용한 레지스터 쓰기를 지원해야 한다:

| Function Code | 이름 | 대상 | 설명 |
|---------------|------|------|------|
| FC05 (0x05) | Write Single Coil | 단일 Coil | 값: `0xFF00`(ON) 또는 `0x0000`(OFF) |
| FC06 (0x06) | Write Single Register | 단일 Holding Register | 값: 0x0000~0xFFFF |
| FC15 (0x0F) | Write Multiple Coils | 다중 Coil | 최대 1968 coils |
| FC16 (0x10) | Write Multiple Registers | 다중 Holding Register | 최대 123 registers |

#### REQ-MODBUS-001-09-02 (Event-Driven) Process() 쓰기 명령 수신

**WHEN** `Process(data []byte)`를 통해 쓰기 명령이 수신되면 **THEN** JSON을 파싱하여 해당 Function Code로 MODBUS 요청을 전송해야 한다.

**쓰기 명령 JSON 포맷:**
```json
{
  "command": "write_register",
  "device_id": "plc-01",
  "params": {
    "address": 40001,
    "value": 1234
  }
}
```

```json
{
  "command": "write_registers",
  "device_id": "plc-01",
  "params": {
    "address": 40001,
    "values": [1234, 5678, 9012]
  }
}
```

```json
{
  "command": "write_coil",
  "device_id": "plc-01",
  "params": {
    "address": 1,
    "value": true
  }
}
```

```json
{
  "command": "write_coils",
  "device_id": "plc-01",
  "params": {
    "address": 1,
    "values": [true, false, true, true]
  }
}
```

#### REQ-MODBUS-001-09-03 (Ubiquitous) 쓰기 명령 JSON 포맷

시스템은 **항상** 쓰기 명령을 다음 필드를 포함하는 JSON 형식으로 수신해야 한다:
- `command`: 명령 유형 (`write_coil`, `write_register`, `write_coils`, `write_registers`)
- `device_id`: 대상 디바이스 식별자
- `params`: 명령 파라미터 (address, value/values)

#### REQ-MODBUS-001-09-04 (Ubiquitous) 쓰기 응답 JSON 포맷

시스템은 **항상** 쓰기 명령의 결과를 다음 JSON 형식으로 반환해야 한다:

**성공 응답:**
```json
{
  "status": "ok",
  "device_id": "plc-01",
  "unit_id": 1,
  "command": "write_register",
  "address": 40001,
  "quantity": 1,
  "timestamp": "2026-02-26T10:30:00Z"
}
```

**실패 응답:**
```json
{
  "status": "error",
  "device_id": "plc-01",
  "unit_id": 1,
  "command": "write_register",
  "error": "modbus exception: illegal data address (0x02)",
  "exception_code": 2,
  "timestamp": "2026-02-26T10:30:00Z"
}
```

#### REQ-MODBUS-001-09-05 (Unwanted) 읽기 전용 레지스터 쓰기 거부

시스템은 읽기 전용 레지스터(Input Register FC04, Discrete Input FC02)에 대한 쓰기 요청을 **수락하지 않아야 한다**. `ErrReadOnlyRegister` 에러를 반환한다.

#### REQ-MODBUS-001-09-06 (Unwanted) 주소 범위 초과 거부

시스템은 0~65535 범위를 벗어나는 레지스터 주소를 **수락하지 않아야 한다**. `ErrAddressOutOfRange` 에러를 반환한다.

#### REQ-MODBUS-001-09-07 (Unwanted) 수량 제한 초과 거부

시스템은 FC15의 경우 1968개, FC16의 경우 123개를 초과하는 수량의 쓰기 요청을 **수락하지 않아야 한다**. `ErrQuantityExceeded` 에러를 반환한다.

#### REQ-MODBUS-001-09-08 (Event-Driven) MODBUS Exception 응답 처리

**WHEN** 쓰기 요청에 대해 MODBUS Exception 응답이 수신되면 **THEN** `exception_code`를 포함한 에러 응답을 반환하고, `ErrModbusException` 에러를 로그에 기록해야 한다.

#### REQ-MODBUS-001-09-09 (Ubiquitous) Write-Through 캐시 갱신

시스템은 **항상** 쓰기 작업 성공 응답을 수신한 **이후에만** `RegisterCache`를 갱신해야 한다 (Write-Through 전략). 쓰기 실패 시 캐시는 변경되지 않는다 (롤백 불필요 — 실패 시 캐시가 원래 값을 유지함).

#### REQ-MODBUS-001-09-10 (Unwanted) 오프라인 디바이스 쓰기 거부

시스템은 오프라인 상태의 디바이스에 대한 쓰기 요청을 **수락하지 않아야 한다**. `ErrDeviceOffline` 에러를 반환한다.

---

### Module 10: Register Synchronization (레지스터 동기화)

#### REQ-MODBUS-001-10-01 (Ubiquitous) RegisterCache 구조체

시스템은 **항상** 디바이스별 `RegisterCache` 구조체를 다음 필드로 정의해야 한다:

| 필드 | 타입 | 설명 |
|------|------|------|
| `Coils` | `map[uint16]bool` | FC01 Coil 값 캐시 |
| `DiscreteInputs` | `map[uint16]bool` | FC02 Discrete Input 값 캐시 |
| `HoldingRegisters` | `map[uint16]uint16` | FC03 Holding Register 값 캐시 |
| `InputRegisters` | `map[uint16]uint16` | FC04 Input Register 값 캐시 |
| `LastUpdateTime` | `map[string]time.Time` | 레지스터 그룹별 마지막 갱신 시각 (키: "FC{code}_{startAddr}") |
| `mu` | `sync.RWMutex` | 동시성 보호 |

#### REQ-MODBUS-001-10-02 (Ubiquitous) deviceCaches 맵 관리

시스템은 **항상** `deviceCaches`를 `map[uint8]*RegisterCache` (Unit ID 기반)로 관리해야 한다. 각 Unit ID에 대해 독립적인 `RegisterCache` 인스턴스를 유지한다.

#### REQ-MODBUS-001-10-03 (Event-Driven) 읽기 성공 시 캐시 갱신

**WHEN** 폴링을 통한 레지스터 읽기가 성공하면 **THEN** 해당 레지스터 그룹의 `RegisterCache` 값과 `LastUpdateTime`을 갱신해야 한다.

#### REQ-MODBUS-001-10-04 (Ubiquitous) 쓰기 성공 후 캐시 갱신 (Write-Through)

시스템은 **항상** 쓰기 작업 성공 응답을 수신한 이후에만 `RegisterCache`를 갱신해야 한다. 실패 시 캐시는 변경되지 않는다 (롤백 불필요).

#### REQ-MODBUS-001-10-05 (Ubiquitous) StatefulAgent.State() 캐시 상태 노출

시스템은 **항상** `StatefulAgent.State()` 메서드를 통해 다음 캐시 상태 정보를 노출해야 한다:
- `device_count`: 관리 중인 디바이스 수
- `devices`: 디바이스별 상태 (unit_id, host, port, online, register_groups)
- `register_groups`: 레지스터 그룹별 function_code, start_address, quantity, last_update, stale 여부

#### REQ-MODBUS-001-10-06 (Event-Driven) Staleness 감지

**WHEN** 특정 레지스터 그룹의 `LastUpdateTime`이 현재 시각에서 `stale_threshold`(기본값: `poll_interval * 3`)를 초과하면 **THEN** 해당 레지스터 그룹을 `stale` 상태로 표시해야 한다.

#### REQ-MODBUS-001-10-07 (Ubiquitous) Event Mode 변경 감지

시스템은 **항상** Event Mode에서 `RegisterCache`에 저장된 이전 값과 현재 폴링 값을 비교하여 변경을 감지해야 한다.

#### REQ-MODBUS-001-10-08 (Event-Driven) Stale 경고 이벤트

**WHEN** 레지스터 그룹이 `stale` 상태가 되면 **THEN** `register_group_stale` 타입의 경고 메시지를 `msgCh`에 전달해야 한다.

**Stale 경고 메시지:**
```json
{
  "type": "register_group_stale",
  "device_id": "plc-01",
  "unit_id": 1,
  "register_group": "FC03_0",
  "last_update": "2026-02-26T10:25:00Z",
  "stale_threshold": "15s",
  "timestamp": "2026-02-26T10:30:00Z"
}
```

#### REQ-MODBUS-001-10-09 (Event-Driven) 온디맨드 캐시 조회

**WHEN** `get_cache` 또는 `get_all_caches` 명령이 `Process()`를 통해 수신되면 **THEN** 현재 `RegisterCache` 상태를 JSON으로 반환해야 한다.

**get_cache 명령:**
```json
{
  "command": "get_cache",
  "device_id": "plc-01"
}
```

**get_cache 응답:**
```json
{
  "status": "ok",
  "device_id": "plc-01",
  "unit_id": 1,
  "cache": {
    "coils": {"1": true, "2": false},
    "discrete_inputs": {},
    "holding_registers": {"0": 1234, "1": 5678},
    "input_registers": {"0": 100}
  },
  "last_update_times": {
    "FC01_0": "2026-02-26T10:30:00Z",
    "FC03_0": "2026-02-26T10:30:00Z"
  },
  "timestamp": "2026-02-26T10:30:05Z"
}
```

#### REQ-MODBUS-001-10-10 (Optional) 강제 캐시 갱신

**가능하면** `refresh_cache` 명령을 통해 특정 디바이스의 모든 레지스터를 즉시 재읽기하여 캐시를 강제 갱신할 수 있도록 제공한다.

**refresh_cache 명령:**
```json
{
  "command": "refresh_cache",
  "device_id": "plc-01"
}
```

---

### Module 11: Configuration (설정)

#### REQ-MODBUS-001-11-01 (Ubiquitous) Transport.Options 기반 설정 파싱

시스템은 **항상** `Transport.Options` (`map[string]any`)에서 다음 설정을 파싱해야 한다:

| 필드 | 키 | 타입 | 기본값 | 필수 | 설명 |
|------|-----|------|--------|------|------|
| Devices | `devices` | `[]DeviceConfig` | - | Yes | 디바이스 설정 목록 |
| PollInterval | `poll_interval` | `string` | `"5s"` | No | 전역 폴링 주기 (time.Duration) |
| Mode | `mode` | `string` | `"interval"` | No | 동작 모드: `"interval"` 또는 `"event"` |
| ReadMode | `read_mode` | `string` | `"cached"` | No | 읽기 모드: `"cached"` 또는 `"direct"` |
| HeartbeatInterval | `heartbeat_interval` | `string` | `"60s"` | No | Event Mode heartbeat 주기 (time.Duration) |
| StaleThreshold | `stale_threshold` | `string` | `""` | No | Staleness 감지 임계값 (비어있으면 poll_interval * 3) |
| WriteTimeout | `write_timeout` | `string` | `"5s"` | No | 쓰기 작업 타임아웃 (time.Duration) |
| EnableWriteEvents | `enable_write_events` | `bool` | `false` | No | 쓰기 성공 시 msgCh에 이벤트 전달 여부 |
| MsgChannelSize | `msg_channel_size` | `int` | 256 | No | 메시지 채널 버퍼 크기 |

**DeviceConfig 구조:**

| 필드 | 키 | 타입 | 기본값 | 필수 | 설명 |
|------|-----|------|--------|------|------|
| DeviceID | `device_id` | `string` | - | Yes | 디바이스 식별자 |
| Host | `host` | `string` | - | Yes | MODBUS 디바이스 IP 주소 |
| Port | `port` | `int` | 502 | No | MODBUS TCP 포트 |
| UnitID | `unit_id` | `uint8` | 1 | No | MODBUS Unit ID (0~247) |
| PollInterval | `poll_interval` | `string` | (전역값) | No | 디바이스별 폴링 주기 |
| ConnectTimeout | `connect_timeout` | `string` | `"5s"` | No | TCP 연결 타임아웃 |
| ReadTimeout | `read_timeout` | `string` | `"3s"` | No | 읽기 타임아웃 |
| ReconnectInterval | `reconnect_interval` | `string` | `"10s"` | No | 재연결 간격 |
| MaxReconnectAttempts | `max_reconnect_attempts` | `int` | 10 | No | 최대 재연결 시도 횟수 |
| Registers | `registers` | `[]RegisterGroupConfig` | - | Yes | 레지스터 그룹 목록 |

**RegisterGroupConfig 구조:**

| 필드 | 키 | 타입 | 기본값 | 필수 | 설명 |
|------|-----|------|--------|------|------|
| FunctionCode | `function_code` | `int` | - | Yes | MODBUS Function Code (1, 2, 3, 4) |
| StartAddress | `start_address` | `uint16` | - | Yes | 시작 레지스터 주소 |
| Quantity | `quantity` | `uint16` | - | Yes | 읽을 레지스터 수량 |
| Name | `name` | `string` | `""` | No | 레지스터 그룹 이름 (로깅/디스플레이용) |

#### REQ-MODBUS-001-11-02 (Event-Driven) 필수 필드 검증

**WHEN** 설정 파싱 시 `devices` 필드가 누락되거나 비어있으면 **THEN** 명확한 에러 메시지와 함께 설정 오류를 반환해야 한다.

#### REQ-MODBUS-001-11-03 (Ubiquitous) 기본값 제공

시스템은 **항상** 다음 기본값을 제공해야 한다:
- `poll_interval`: `"5s"`
- `mode`: `"interval"`
- `read_mode`: `"cached"`
- `port`: 502
- `unit_id`: 1
- `connect_timeout`: `"5s"`
- `read_timeout`: `"3s"`
- `reconnect_interval`: `"10s"`
- `max_reconnect_attempts`: 10
- `heartbeat_interval`: `"60s"`
- `write_timeout`: `"5s"`
- `msg_channel_size`: 256

**YAML 설정 예시:**
```yaml
agents:
  - id: "modbus-client-01"
    name: "MODBUS PLC Controller"
    type: "modbus-tcp"
    transport:
      type: "custom"
      options:
        poll_interval: "5s"
        mode: "interval"
        read_mode: "cached"
        stale_threshold: "15s"
        write_timeout: "5s"
        enable_write_events: true
        devices:
          - device_id: "plc-01"
            host: "192.168.1.100"
            port: 502
            unit_id: 1
            reconnect_interval: "10s"
            max_reconnect_attempts: 10
            registers:
              - function_code: 3
                start_address: 0
                quantity: 10
                name: "holding-regs-0-9"
              - function_code: 1
                start_address: 0
                quantity: 16
                name: "coils-0-15"
              - function_code: 4
                start_address: 0
                quantity: 5
                name: "input-regs-0-4"
          - device_id: "sensor-01"
            host: "192.168.1.101"
            port: 502
            unit_id: 2
            poll_interval: "10s"
            registers:
              - function_code: 4
                start_address: 0
                quantity: 8
                name: "sensor-inputs"
```

---

### Module 12: Error Handling (오류 처리)

#### REQ-MODBUS-001-12-01 (Event-Driven) Health Degraded 전환

**WHEN** 특정 디바이스의 연속 통신 에러 횟수가 임계값(기본값: 3)을 초과하면 **THEN** Health 상태를 `Degraded`로 보고해야 한다. 모든 디바이스의 통신이 정상이면 `Healthy`로 복원한다.

#### REQ-MODBUS-001-12-02 (Event-Driven) 타임아웃 처리

**WHEN** 읽기 또는 쓰기 요청이 타임아웃되면 **THEN** 에러를 로그에 기록하고, 해당 디바이스/레지스터를 건너뛰고 다음 디바이스/레지스터의 처리를 계속해야 한다.

#### REQ-MODBUS-001-12-03 (Ubiquitous) 구조화 로깅

시스템은 **항상** `slog` 패키지를 사용한 구조화 로깅을 적용해야 한다. 로그 메시지에는 다음 필드를 포함한다:
- `device_id`: 디바이스 식별자
- `function_code`: MODBUS Function Code
- `address`: 레지스터 주소
- `error`: 에러 메시지 (오류 발생 시)

#### REQ-MODBUS-001-12-04 (Ubiquitous) 센티널 에러 정의

다음 센티널 에러가 **항상** 정의되어야 한다:

| 에러 변수 | 설명 |
|-----------|------|
| `ErrReadOnlyRegister` | 읽기 전용 레지스터에 대한 쓰기 시도 |
| `ErrAddressOutOfRange` | 레지스터 주소 범위 초과 (0~65535) |
| `ErrQuantityExceeded` | 쓰기 수량 제한 초과 (FC15: 1968, FC16: 123) |
| `ErrWriteTimeout` | 쓰기 작업 타임아웃 |
| `ErrModbusException` | MODBUS Exception 응답 수신 |
| `ErrDeviceOffline` | 오프라인 디바이스에 대한 명령 |
| `ErrInvalidFunctionCode` | 유효하지 않은 Function Code |
| `ErrInvalidCommand` | 유효하지 않은 명령 형식 |
| `ErrDeviceNotFound` | 미등록 디바이스 ID |
| `ErrMaxReconnectExceeded` | 최대 재연결 횟수 초과 |
| `ErrConnectionFailed` | TCP 연결 실패 |
| `ErrProtocolError` | MODBUS 프로토콜 오류 (MBAP Header 불일치 등) |

모든 에러는 `errors.New()`로 정의하며, `errors.Is()`로 비교 가능해야 한다.

---

## 4. Specifications (상세 명세)

### 4.1 파일 구조

```
internal/agent/modbus/
├── agent.go           # MODBUSAgent 구현 (Init, Start, Stop, Pause, Resume, Process, ReceiveMessage, State)
├── config.go          # ModbusConfig, DeviceConfig, RegisterGroupConfig 파싱 및 검증
├── cache.go           # RegisterCache 구조체 및 캐시 관리 로직
├── client.go          # MODBUS/TCP 클라이언트 (MBAP Header 생성, PDU 조립, TCP 통신)
├── protocol.go        # MODBUS 프로토콜 상수, Function Code, Exception Code 정의
├── register.go        # RegisterSamsungModbusTypes 타입 등록 함수
├── errors.go          # 센티널 에러 정의
├── agent_test.go      # 에이전트 단위 테스트
├── config_test.go     # 설정 파싱 테스트
├── cache_test.go      # RegisterCache 테스트
├── client_test.go     # MODBUS 클라이언트 테스트
└── protocol_test.go   # 프로토콜 상수/인코딩 테스트
```

### 4.2 타입 등록

`RegisterModbusTCPTypes(registry)` 함수로 `"modbus-tcp"` 타입을 에이전트 팩토리에 등록한다.

### 4.3 Traceability (추적성)

| 요구사항 그룹 | Module | 파일 |
|-------------|--------|------|
| REQ-MODBUS-001-01-* | Module 1: Connection Management | client.go |
| REQ-MODBUS-001-02-* | Module 2: Register Reading | agent.go, client.go |
| REQ-MODBUS-001-03-* | Module 3: Read Mode | agent.go |
| REQ-MODBUS-001-04-* | Module 4: Multi-Device | agent.go, config.go |
| REQ-MODBUS-001-05-* | Module 5: Interval Mode | agent.go |
| REQ-MODBUS-001-06-* | Module 6: Event Mode | agent.go, cache.go |
| REQ-MODBUS-001-07-* | Module 7: Bridge Integration | agent.go |
| REQ-MODBUS-001-08-* | Module 8: Lifecycle | agent.go |
| REQ-MODBUS-001-09-* | Module 9: Register Write | agent.go, client.go |
| REQ-MODBUS-001-10-* | Module 10: Register Synchronization | cache.go |
| REQ-MODBUS-001-11-* | Module 11: Configuration | config.go |
| REQ-MODBUS-001-12-* | Module 12: Error Handling | errors.go, agent.go |

---

*SPEC-MODBUS-001 v1.0.0*
*작성자: xtra*
*날짜: 2026-02-26*
