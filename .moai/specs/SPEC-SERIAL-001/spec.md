# SPEC-SERIAL-001: 시리얼 포트 에이전트

## 메타데이터

| 항목 | 값 |
|------|-----|
| ID | SPEC-SERIAL-001 |
| 버전 | 1.0.0 |
| 상태 | Planned |
| 생성일 | 2026-04-01 |
| 작성자 | MoAI |
| 우선순위 | High |
| 관련 SPEC | SPEC-AGENT-001, SPEC-BRIDGE-001, SPEC-SOCKET-001 |

---

## 1. 환경 (Environment)

### 1.1 시스템 컨텍스트

xflow는 IoT 데이터 스트림 처리를 위한 FBP 플랫폼이다. 현재 TCP/UDP 소켓 에이전트(SPEC-SOCKET-001)와 삼성 NASA 에이전트(SPEC-NASA-001)가 시리얼 통신을 지원하지만, 범용 시리얼 포트 에이전트가 부재하다. Samsung NASA와 LG LGCP/LGAP 에이전트는 각각 자체적으로 `go.bug.st/serial` 라이브러리를 직접 사용하고 있어, 시리얼 통신의 공통 추상화가 없는 상태이다.

### 1.2 기술 스택

- 언어: Go 1.23+
- 시리얼 라이브러리: `go.bug.st/serial` v1.6+ (기존 의존성)
- 에이전트 프레임워크: `internal/agent/` (Agent 인터페이스, BaseLifecycle)
- 프레이밍: `internal/agent/socket/` 패키지의 Framer 인터페이스 참조
- 브릿지 노드: `internal/node/bridge.go` (BridgeAdapter 패턴)

### 1.3 기존 패턴

- **소켓 에이전트 패턴** (`internal/agent/socket/`): config.go, errors.go, framing.go, register.go 구조
- **어댑터 패턴** (`internal/node/adapter/`): SocketAdapter, NASAAdapter 등 BridgeAdapter 구현
- **시리얼 사용 사례**: Samsung NASA (RS-485, preamble), LG LGCP/LGAP (RS-485, RWMutex)

---

## 2. 가정 (Assumptions)

### 2.1 기술적 가정

- A1: `go.bug.st/serial` 라이브러리는 Linux, macOS, Windows에서 시리얼 포트 접근을 지원한다.
- A2: 시리얼 포트는 한 번에 하나의 에이전트만 열 수 있다. 동일 포트에 대한 다중 에이전트 생성은 OS 수준에서 차단된다.
- A3: USB-to-Serial 디바이스의 핫플러그(연결/해제)가 런타임에 발생할 수 있다.
- A4: 시리얼 데이터는 비구조화된 바이트 스트림이다. 프레이밍은 사용자가 선택한다.

### 2.2 설계적 가정

- A5: 소켓 에이전트의 Framer 인터페이스(`net.Conn` 기반)는 시리얼 포트(`io.ReadWriteCloser`)와 직접 호환되지 않는다. 어댑터 또는 별도 구현이 필요하다.
- A6: 하나의 시리얼 에이전트 인스턴스를 여러 플로우에서 Input, Output, InputOutput 방향의 Bridge Node로 공유할 수 있다.
- A7: Pause 시 포트 연결은 유지하되, 읽기 goroutine이 데이터를 무시한다. Resume 시 읽기를 재개한다.

---

## 3. 요구사항 (Requirements)

### REQ-SERIAL-001: 시리얼 포트 에이전트 타입 등록

시스템은 **항상** "serial" 타입의 에이전트 팩토리를 Agent Manager에 등록해야 한다.

- 등록 함수: `RegisterSerialTypes(mgr *agent.DefaultManager) error`
- 에이전트 팩토리: `func(config AgentConfig) (Agent, error)`

### REQ-SERIAL-002: 시리얼 포트 설정 파싱

**WHEN** 사용자가 시리얼 에이전트를 생성할 때, **THEN** 시스템은 `AgentConfig.Transport.Options` 맵에서 다음 설정을 파싱해야 한다:

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| port | string | 필수 | - | 시리얼 포트 경로 (예: /dev/ttyUSB0, COM3) |
| baud_rate | int | 선택 | 9600 | 전송 속도 |
| data_bits | int | 선택 | 8 | 데이터 비트 (5, 6, 7, 8) |
| stop_bits | int | 선택 | 1 | 스톱 비트 (1, 2) |
| parity | string | 선택 | "none" | 패리티 (none, even, odd, mark, space) |
| read_timeout | duration | 선택 | "100ms" | 읽기 타임아웃 |
| buffer_size | int | 선택 | 4096 | 읽기 버퍼 크기(바이트) |
| framing | string | 선택 | "raw" | 프레이밍 타입 (raw, newline, length_prefix, fixed_size) |
| delimiter | byte | 선택 | '\n' | 구분자 (framing=newline 시) |
| fixed_size | int | 선택 | 0 | 고정 크기 (framing=fixed_size 시 필수) |
| max_message_size | int | 선택 | 0 | 최대 메시지 크기 (0=무제한) |

### REQ-SERIAL-003: 시리얼 포트 생명주기

시리얼 에이전트는 **항상** `lifecycle.BaseLifecycle`을 임베딩하여 7개 상태 머신을 따라야 한다.

- **Init**: 설정 파싱 및 유효성 검증. 포트를 열지 않는다.
- **Start**: 시리얼 포트를 열고, 읽기 goroutine을 시작한다. 프레이머를 초기화한다.
- **Pause**: 읽기 goroutine을 정지한다. 포트 연결은 유지한다.
- **Resume**: 읽기 goroutine을 재시작한다.
- **Stop**: 읽기 goroutine을 정지하고, 시리얼 포트를 닫는다.

### REQ-SERIAL-004: 데이터 수신 (Agent -> Flow)

**WHEN** 시리얼 포트에서 데이터가 수신될 때, **THEN** 시스템은 프레이머를 통해 데이터를 프레이밍하고, `msgCh` 채널로 전달하여 Bridge Node가 Flow에 메시지를 전달할 수 있도록 해야 한다.

- MessageReceiver 인터페이스 구현: `ReceiveMessage() <-chan []byte`
- 읽기 goroutine은 별도의 goroutine에서 실행된다.
- 에러 발생 시 로그를 남기고, 복구 가능하면 재시도한다.

### REQ-SERIAL-005: 데이터 송신 (Flow -> Agent)

**WHEN** Flow에서 시리얼 에이전트로 메시지를 전송할 때, **THEN** 시스템은 `Process` 명령을 통해 바이트 데이터를 시리얼 포트에 쓸 수 있어야 한다.

- `Process(msg message.Message) error` 구현
- 페이로드에서 `raw` ([]byte) 또는 `data` (string) 필드를 추출하여 전송
- 동시 쓰기 보호: `sync.Mutex`로 Write 연산을 직렬화

### REQ-SERIAL-006: USB 디바이스 분리 감지

**IF** 시리얼 포트에서 USB 디바이스 분리(ENXIO, EIO 에러)가 발생하면, **THEN** 시스템은 에이전트를 Error 상태로 전이하고, 에러를 로그에 기록해야 한다.

- `syscall.ENXIO` (No such device or address): USB 디바이스 물리적 분리
- `syscall.EIO` (I/O error): 통신 중 디바이스 분리
- 플랫폼별 에러 코드 처리 (Samsung NASA, LG 에이전트의 `serial_opener.go` 패턴 참조)

### REQ-SERIAL-007: 시리얼 프레이밍

시스템은 **항상** 시리얼 데이터에 대해 4가지 프레이밍 방식을 지원해야 한다:

| 프레이밍 | 설명 |
|----------|------|
| raw | 버퍼 크기만큼 읽기 (기본) |
| newline | 구분자 기반 라인 읽기 |
| length_prefix | 4바이트 빅엔디안 길이 접두사 |
| fixed_size | 고정 크기 바이트 읽기 |

- 소켓 에이전트의 `Framer` 인터페이스는 `net.Conn`에 의존하므로, 시리얼 전용 `SerialFramer` 인터페이스를 정의한다.
- `SerialFramer`는 `io.ReadWriteCloser`를 대상으로 Read/Write를 수행한다.
- 구현 로직은 소켓 Framer와 동일하되, 인터페이스 시그니처만 다르다.

### REQ-SERIAL-008: Bridge Node를 통한 공유 접근

**WHEN** 여러 플로우에서 동일 시리얼 에이전트를 참조할 때, **THEN** 각 플로우는 Bridge Node의 방향(In, Out, InOut)에 따라 독립적으로 데이터를 주고받을 수 있어야 한다.

- BridgeIn (Input): Agent -> Flow 방향만 (시리얼 수신 데이터를 플로우로)
- BridgeOut (Output): Flow -> Agent 방향만 (플로우 데이터를 시리얼로)
- BridgeInOut (InputOutput): 양방향
- 에이전트의 `StopOnZeroRef` 설정에 따라 참조 0일 때 자동 정지

### REQ-SERIAL-009: 시리얼 어댑터

시스템은 **항상** "serial" 타입에 대한 BridgeAdapter를 등록해야 한다.

- `SerialAdapter` 구현: `internal/node/adapter/serial.go`
- `TransformToFlow`: 수신 바이트 -> Message (raw: []byte, data: string)
- `TransformToAgent`: Message -> 바이트 (raw 필드 우선, 폴백으로 JSON 직렬화)
- `DefaultConfig`: direction=inout (기본 양방향)
- `HandleControl`: 제어 메시지 처리 (확장 포인트)

### REQ-SERIAL-010: 에이전트 상태 정보 제공

시스템은 **항상** 시리얼 에이전트의 상태 정보를 제공해야 한다.

- `Health() agent.HealthInfo`: 포트 연결 상태, 마지막 수신/송신 시각
- `Stats() agent.AgentStats`: 수신/송신 메시지 수, 바이트 수 (atomic 카운터)
- `Info() agent.AgentInfo`: 에이전트 타입, 포트 경로, baud_rate 등 설정 정보
- `TransportConnected() bool`: 포트 열림 여부

---

## 4. 명세 (Specifications)

### 4.1 파일 구조

```
internal/agent/serial/
  common.go          # 상수 및 기본값 정의
  errors.go          # 센티널 에러 정의
  config.go          # SerialConfig, ParseSerialConfig
  framing.go         # SerialFramer 인터페이스 및 4가지 구현체
  agent.go           # SerialAgent 구현 (Agent + MessageReceiver)
  register.go        # RegisterSerialTypes
  *_test.go          # 각 파일별 테스트

internal/node/adapter/
  serial.go          # SerialAdapter (BridgeAdapter)
  serial_test.go     # 어댑터 테스트
```

### 4.2 SerialFramer 인터페이스

```go
// SerialFramer 는 시리얼 포트에서 메시지 프레이밍을 담당한다.
type SerialFramer interface {
    Read(r io.Reader) ([]byte, error)
    Write(w io.Writer, data []byte) error
}
```

소켓 Framer(`net.Conn` 기반)와 구분하여 `io.Reader`/`io.Writer` 기반으로 설계한다. 4가지 구현체:

- `rawFramer`: 버퍼 크기만큼 읽기
- `newlineFramer`: 구분자 기반 `bufio.Scanner` 읽기
- `lengthPrefixFramer`: 4바이트 빅엔디안 길이 접두사
- `fixedSizeFramer`: `io.ReadFull`로 고정 크기 읽기

### 4.3 SerialAgent 구조체

```go
type SerialAgent struct {
    lifecycle.BaseLifecycle
    config    SerialConfig
    port      serial.Port        // go.bug.st/serial
    framer    SerialFramer
    msgCh     chan []byte
    mu        sync.Mutex         // Write 직렬화
    stats     agent.AgentStats   // atomic 카운터
    stopCh    chan struct{}       // 읽기 goroutine 정지 신호
    logger    *slog.Logger
}
```

### 4.4 에러 정의

```go
var (
    ErrPortRequired     = errors.New("serial: port is required")
    ErrInvalidBaudRate  = errors.New("serial: invalid baud_rate")
    ErrInvalidDataBits  = errors.New("serial: invalid data_bits")
    ErrInvalidStopBits  = errors.New("serial: invalid stop_bits")
    ErrInvalidParity    = errors.New("serial: invalid parity")
    ErrInvalidFraming   = errors.New("serial: invalid framing type")
    ErrFixedSizeRequired = errors.New("serial: fixed_size is required for fixed_size framing")
    ErrPortNotFound     = errors.New("serial: port not found")
    ErrPermissionDenied = errors.New("serial: permission denied")
    ErrDeviceDisconnected = errors.New("serial: device disconnected")
    ErrPortClosed       = errors.New("serial: port is closed")
    ErrNotRunning       = errors.New("serial: agent is not running")
)
```

### 4.5 어댑터 등록

```go
// internal/node/adapter/register.go 에 추가
node.RegisterAdapter("serial", NewSerialAdapter())
```

### 4.6 추적성 태그

| 요구사항 | 파일 | 함수/구조체 |
|----------|------|------------|
| REQ-SERIAL-001 | register.go | RegisterSerialTypes |
| REQ-SERIAL-002 | config.go | ParseSerialConfig |
| REQ-SERIAL-003 | agent.go | SerialAgent (Init/Start/Pause/Resume/Stop) |
| REQ-SERIAL-004 | agent.go | readLoop, ReceiveMessage |
| REQ-SERIAL-005 | agent.go | Process |
| REQ-SERIAL-006 | agent.go | isDisconnectError |
| REQ-SERIAL-007 | framing.go | SerialFramer, rawFramer, newlineFramer, lengthPrefixFramer, fixedSizeFramer |
| REQ-SERIAL-008 | bridge.go (기존) | BridgeNode 방향 처리 |
| REQ-SERIAL-009 | adapter/serial.go | SerialAdapter |
| REQ-SERIAL-010 | agent.go | Health, Stats, Info, TransportConnected |

---

*SPEC 버전: 1.0.0*
*생성일: 2026-04-01*
*작성: MoAI SPEC Builder*
