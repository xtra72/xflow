# SPEC-SERIAL-001: 시리얼 포트 에이전트

## 메타데이터

| 항목 | 값 |
|------|-----|
| ID | SPEC-SERIAL-001 |
| 버전 | 2.4.0 |
| 상태 | Done |
| 생성일 | 2026-04-01 |
| 수정일 | 2026-07-28 |
| 작성자 | MoAI |
| 우선순위 | High |
| 관련 SPEC | SPEC-AGENT-001, SPEC-AGENT-006, SPEC-BRIDGE-001, SPEC-SOCKET-001, SPEC-ENGINE-001 |

---

## 변경 이력 (Change History)

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-04-01 | 1.0.0 ~ 2.1.0 | 초기 작성 ~ NASA 디바이스 연동 확장 (하단 확장 섹션 참조) |
| 2026-05-14 | 2.2.0 | xagent04 실배포 검증 hotfix 3종 반영. (1) **Serial 재시작 생명주기 보강** — `Stop()` 시 `stopCh` 재설정, bounded `Stop()`(5초 타임아웃), `Error` 상태에서의 회복 경로 추가 (REQ-SERIAL-003 amend, 커밋 `d0651aa`). (2) **SerialOut hex 인코딩 대칭성** — `data`/`raw` 필드가 hex 문자열일 때 `hex.DecodeString` 적용. 이전엔 `[]byte(str)` 로 ASCII 변질 (REQ-SERIAL-005 amend). (3) **Init-tolerance 패턴** — `serial-in`/`serial-out` 노드가 Init 시점에 agent 를 못 찾으면 hard-fail 대신 경고 로그 + Running 전이(deferred connection), agent 활성화 시 `ReinitNodesForAgent` 로 자동 연결. "resolver 미설정"(구성 오류)은 여전히 hard-fail (신규 REQ-SERIAL-016). 관련: SPEC-ENGINE-001 v1.3.0 Module 8, SPEC-AGENT-005 v1.1.0. |
| 2026-05-14 | 2.3.0 | develop 브랜치 serial/IO 결함 수정 4종 반영. (1) **포트 I/O 직렬화** — `Process()`(쓰기)와 `readLoop()`(읽기)가 동일 물리 포트에 동시 접근하던 경쟁 조건을 `portIOMu sync.Mutex` 로 직렬화. RS-485 반이중 정합성을 위해 필수 (REQ-SERIAL-003 보강, 커밋 `5c00731`). (2) **`effectiveReadTimeout` 클램프** — 0 이하의 `read_timeout` 을 200ms 로 클램프하여 `readLoop` 이 `portIOMu` 를 무한 점유해 쓰기를 기아 상태로 만드는 것을 방지 (REQ-SERIAL-002 보강). (3) **출력 디버그 로그** — `Process()` 가 쓰기 성공 후 `시리얼 포트 출력 port=... len=... hex=...` debug 로그를 남겨 OS 포트로 전달된 바이트를 운영자가 확인 가능 (REQ-SERIAL-005 보강). (4) **`serial-out` `input_encoding` 옵션** — 문자열 `data`/`raw` 페이로드의 바이트 변환 방식을 명시적으로 선택(`auto`/`hex`/`text`/`base64`, 기본 `auto`). `auto` 는 기존 동작(hex 시도 후 text 폴백)과 바이트 동일하며, 유효 hex 평문이 잘못 hex 디코딩되던 모호성 제거. `[]byte` 인 `raw` 는 인코딩을 우회 (신규 REQ-SERIAL-017, 커밋 `8cfd1b6`). (5) **`raw` 버퍼 aliasing 수정** — `SerialInNode` 의 `receiveLoop`/`rawReceiveLoop` 가 재사용 읽기 버퍼를 슬라이스 참조로 `raw` 에 저장하던 것을 방어적 복사로 수정 (REQ-SERIAL-004/012 보강, 커밋 `8cfd1b6`). (6) **시리얼 에이전트 `log_drops` 옵션** — `msgCh` 수신 버퍼 가득 참으로 메시지 드롭 시 per-drop WARN 로그를 `log_drops: true` 일 때만 출력(기본 `false`). 드롭은 항상 `stats.IncrDroppedMessages()` 로 계수되어 통계로 관측 가능. samsung-nasa `log_decode_errors` 패턴과 일관 (신규 REQ-SERIAL-018, 미커밋 작업). |
| 2026-07-28 | 2.4.0 | **half-duplex 쓰기 기아(write starvation) 근본 수정.** v2.3.0 의 포트 I/O 직렬화가 실배포(xagent03)에서 `serial-out: send failed: serial: context deadline exceeded` 를 반복 유발한 결함 수정. **근본 원인**: v2.3.0 은 `readLoop` 이 `a.reader.Read()` **한 번 호출 전체 동안** `portIOMu` 를 점유했고, "`effectiveReadTimeout` 클램프로 `Read` 가 bounded 되어 기아를 막는다"고 가정했으나 이 가정이 **틀렸다** — `SetReadTimeout` 은 개별 하위 read syscall 1회만 bound 할 뿐, 한 프레임을 조립하려 여러 하위 read 를 도는 프레이머의 전체 `Read()` 시간은 여전히 무한이다(`stream` 프레이머는 연속 스트림/`idle_timeout≤0` 에서 절대 반환 안 함; `length_prefix`/`frame`/`fixed_size` 는 `io.ReadFull` 로 부분 프레임에서 무한 블록). 그동안 `portIOMu` 를 놓지 못해 `Process()` 의 쓰기가 굶었고, 노드 레벨 `write_timeout`(커밋 `73dd0a2`, 기본 5s)이 이 무한 hang 을 5초 에러로 표면화했다. **수정**: `portIOReader` io.Reader 래퍼를 포트–프레이머 사이에 삽입해 락을 **프레임 조립 전체가 아니라 개별 하위 read syscall 단위**로만 획득/해제(readLoop 의 조악한 락 제거). 프레이머의 다중 read 루프 사이에 대기 중인 쓰기가 Go mutex 공정성(~1ms)으로 끼어들어 기아가 사라진다. half-duplex 직렬화(바이트 전송 순간의 배타성)는 보존. (2) **`half_duplex` 설정 옵션(기본 `true`)** — full-duplex 포트(TX/RX 물리 분리: RS-232/USB-serial/serial-to-ethernet 컨버터)에서 `false` 로 설정 시 `portIOLock` 이 no-op(`noopLocker`)이 되어 read/write 를 직렬화하지 않는다. 기본 `true` 는 RS-485 반이중 안전을 위한 기존 동작 보존 (신규 REQ-SERIAL-019). (3) **노드 레벨 `write_timeout`** — `serial-out` 이 blocking 한 시리얼 write 를 `write_timeout`(기본 5s) 내 반환하도록 상한, 상류 stall 방지 (REQ-SERIAL-005 보강, 커밋 `73dd0a2`). 특성 테스트: `agent_writestarve_test.go`(half_duplex=true 에서 연속 데이터 중 쓰기가 <1s 완료). |

---

## 1. 환경 (Environment)

### 1.1 시스템 컨텍스트

xflow는 IoT 데이터 스트림 처리를 위한 FBP 플랫폼이다. 현재 TCP/UDP 소켓 에이전트(SPEC-SOCKET-001)와 Samsung HVACR-01 에이전트(SPEC-SAMSUNG-HVACR-001, Samsung NASA 프로토콜)가 시리얼 통신을 지원하지만, 범용 시리얼 포트 에이전트가 부재하다. Samsung NASA와 LG HVACR-02 / LGAP 에이전트는 각각 자체적으로 `go.bug.st/serial` 라이브러리를 직접 사용하고 있어, 시리얼 통신의 공통 추상화가 없는 상태이다.

### 1.2 기술 스택

- 언어: Go 1.23+
- 시리얼 라이브러리: `go.bug.st/serial` v1.6+ (기존 의존성)
- 에이전트 프레임워크: `internal/agent/` (Agent 인터페이스, BaseLifecycle)
- 프레이밍: `internal/agent/socket/` 패키지의 Framer 인터페이스 참조
- 브릿지 노드: `internal/node/bridge.go` (BridgeAdapter 패턴)

### 1.3 기존 패턴

- **소켓 에이전트 패턴** (`internal/agent/socket/`): config.go, errors.go, framing.go, register.go 구조
- **어댑터 패턴** (`internal/node/adapter/`): SocketAdapter, NASAAdapter 등 BridgeAdapter 구현
- **시리얼 사용 사례**: Samsung NASA (RS-485, preamble), LG HVACR-02 / LGAP (RS-485, RWMutex)

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
| read_timeout | duration | 선택 | "100ms" | 읽기 타임아웃. 0 이하 값은 200ms로 클램프된다 (v2.3.0, REQ-SERIAL-003 참조) |
| log_drops | bool | 선택 | false | `msgCh` 버퍼 가득 참으로 메시지 드롭 시 per-drop WARN 로그 출력 여부 (v2.3.0, REQ-SERIAL-018) |
| buffer_size | int | 선택 | 4096 | 읽기 버퍼 크기(바이트) |
| framing | string | 선택 | "raw" | 프레이밍 타입 (raw, newline, length_prefix, fixed_size, stream, frame). 빈 문자열은 "raw"로 폴백 |
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

#### 재시작 생명주기 보강 (v2.2.0)

연속 Stop → Start (재시작) 경로의 신뢰성을 위해 다음을 **항상** 보장해야 한다:

- **stopCh 재설정**: `Stop()` 시 닫힌 `stopCh` 채널을 다음 `Start()` 가 재사용할 수 있도록 새 채널로 재설정한다. 닫힌 채널 재사용으로 인한 즉시 종료를 방지한다.
- **Bounded Stop**: `Stop()` 은 읽기 goroutine 종료를 무한정 대기하지 않고 5초 타임아웃을 적용한다. 타임아웃 시에도 포트를 닫고 상태 전이를 완료한다.
- **Error 상태 회복**: 에이전트가 `Error` 상태에 진입한 경우에도 `Start()`/`Restart()` 를 통해 정상 lifecycle 경로로 회복할 수 있어야 한다. `Error` 상태가 영구 정지 상태가 되어서는 안 된다.

#### 포트 I/O 직렬화 (v2.3.0, v2.4.0 근본 수정)

`Process()`(쓰기 경로)와 `readLoop()`(읽기 경로)는 동일한 물리 시리얼 포트에 접근하므로, `half_duplex=true`(기본)일 때 시스템은 `portIOMu sync.Mutex` 로 `framer.Write` 와 물리 포트 `Read` 를 직렬화해야 한다.

- **RS-485 반이중 정합성**: RS-485 반이중 회선에서 송신과 수신이 겹치면 회선 충돌이 발생하므로 포트 I/O 직렬화가 필요하다.
- **`portIOMu` 는 기존 Write 직렬화용 `sync.Mutex` 와 별개의 잠금**으로, 읽기·쓰기 양방향을 모두 보호한다.
- **`half_duplex` 옵션 (v2.4.0, 기본 `true`)**: full-duplex 포트(TX/RX 물리 분리: RS-232 / USB-serial / serial-to-ethernet 컨버터)는 송수신을 동시에 수행할 수 있으므로 직렬화가 불필요하다. `half_duplex=false` 로 설정하면 `portIOLock` 이 `noopLocker`(no-op)로 대체되어 read 와 write 가 서로 배제하지 않는다. 기본 `true` 는 RS-485 안전을 위한 기존 동작 보존이다.
- **per-sub-read 락킹 (v2.4.0 근본 수정)**: v2.3.0 은 `readLoop` 이 `a.reader.Read()` **한 번 호출 전체 동안** `portIOMu` 를 점유했고, "`effectiveReadTimeout` 클램프로 `Read` 가 bounded 되므로 쓰기가 굶지 않는다"고 가정했다. **이 가정은 틀렸다**: `SetReadTimeout` 은 개별 하위 read syscall 1회만 bound 할 뿐, 한 프레임을 조립하려 여러 하위 read 를 루프하는 프레이머(`stream` 은 연속 스트림/`idle_timeout≤0` 에서 절대 반환 안 함; `length_prefix`/`frame`/`fixed_size` 는 `io.ReadFull` 로 부분 프레임에서 무한 블록)의 전체 `Read()` 시간은 여전히 무한이다. 그동안 `portIOMu` 를 놓지 못해 `Process()` 의 쓰기가 굶고, 노드 레벨 `write_timeout`(기본 5s)이 이를 `serial: context deadline exceeded` 로 표면화했다. **수정**: `portIOReader` io.Reader 래퍼를 물리 포트–프레이머 사이에 삽입하여 락을 **개별 하위 read syscall 단위**로만 획득/해제한다(`readLoop` 의 조악한 락 제거). 프레이머의 다중 read 루프 사이에 대기 중인 쓰기가 Go mutex 공정성(~1ms)으로 끼어들 수 있어 기아가 제거되며, half-duplex 직렬화(실제 바이트 전송 순간의 배타성)는 보존된다. `effectiveReadTimeout` 클램프(0 이하 → 200ms)는 각 하위 read 가 유한 시간에 반환되도록 여전히 유지한다.

### REQ-SERIAL-004: 데이터 수신 (Agent -> Flow)

**WHEN** 시리얼 포트에서 데이터가 수신될 때, **THEN** 시스템은 프레이머를 통해 데이터를 프레이밍하고, `msgCh` 채널로 전달하여 Bridge Node가 Flow에 메시지를 전달할 수 있도록 해야 한다.

- MessageReceiver 인터페이스 구현: `ReceiveMessage() <-chan []byte`
- 읽기 goroutine은 별도의 goroutine에서 실행된다.
- 에러 발생 시 로그를 남기고, 복구 가능하면 재시도한다.
- 프레이밍 에러(ETX 불일치, 체크섬 불일치, 프레임 크기 초과)는 해당 프레임만 폐기하고 다음 프레임 읽기를 계속한다. 에이전트가 Error 상태로 전이하지 않는다.

#### raw 버퍼 aliasing 방지 (v2.3.0)

**WHEN** `SerialInNode` 의 `receiveLoop`/`rawReceiveLoop` 가 수신 바이트를 메시지 페이로드의 `raw` 필드에 담을 때, **THEN** 시스템은 **항상** 재사용되는 읽기 버퍼에 대한 슬라이스 참조가 아니라 방어적 복사본(`make`+`copy`)을 저장해야 한다.

- v2.2.0 까지는 `raw` 가 재사용 읽기 버퍼를 직접 참조하여, 후속 `Read` 가 같은 backing array 를 덮어쓰면 이미 전달된 메시지의 `raw` 가 `data` 와 어긋나거나 다음 프레임 데이터로 오염되는 결함이 있었다.
- v2.3.0 부터는 `raw` 가 자신만의 backing array 를 소유하여 cross-frame 오염이 발생하지 않는다.

### REQ-SERIAL-005: 데이터 송신 (Flow -> Agent)

**WHEN** Flow에서 시리얼 에이전트로 메시지를 전송할 때, **THEN** 시스템은 `Process` 명령을 통해 바이트 데이터를 시리얼 포트에 쓸 수 있어야 한다.

- `Process(msg message.Message) error` 구현
- 페이로드에서 `raw` ([]byte) 또는 `data` (string) 필드를 추출하여 전송
- 동시 쓰기 보호: `sync.Mutex`로 Write 연산을 직렬화

#### hex 인코딩 대칭성 (v2.2.0)

**IF** `data` 또는 `raw` 필드가 hex 문자열로 제공되면, **THEN** SerialOutNode(및 SerialAdapter `TransformToAgent`)는 `hex.DecodeString` 으로 디코딩하여 실제 바이트를 전송해야 한다.

- v2.1.0 까지는 hex 문자열을 `[]byte(str)` 로 처리하여 ASCII 코드포인트로 변질되는 버그가 있었다 (예: `"02AA"` → 4바이트 ASCII `0x30 0x32 0x41 0x41` 전송).
- v2.2.0 부터는 serial-in 의 hex 출력과 serial-out 의 hex 입력이 대칭(round-trip)을 이룬다.

#### 출력 디버그 로그 (v2.3.0)

**WHEN** `Process()` 가 시리얼 포트 쓰기에 성공하면, **THEN** 시스템은 debug 수준 로그 `시리얼 포트 출력 port=... len=... hex=...` 를 남겨야 한다.

- 운영자가 OS 시리얼 포트로 실제 전달된 바이트(포트 경로, 길이, hex 덤프)를 저널에서 확인할 수 있도록 한다.
- debug 레벨이므로 기본 운영 환경에서는 출력되지 않으며, 디버깅 시에만 활성화된다.

### REQ-SERIAL-006: USB 디바이스 분리 감지

**IF** 시리얼 포트에서 USB 디바이스 분리(ENXIO, EIO 에러)가 발생하면, **THEN** 시스템은 에이전트를 Error 상태로 전이하고, 에러를 로그에 기록해야 한다.

- `syscall.ENXIO` (No such device or address): USB 디바이스 물리적 분리
- `syscall.EIO` (I/O error): 통신 중 디바이스 분리
- 플랫폼별 에러 코드 처리 (Samsung NASA, LG 에이전트의 `serial_opener.go` 패턴 참조)

### REQ-SERIAL-007: 시리얼 프레이밍

시스템은 **항상** 시리얼 데이터에 대해 6가지 프레이밍 방식을 지원해야 한다:

| 프레이밍 | 설명 |
|----------|------|
| raw | 버퍼 크기만큼 읽기 (기본) |
| newline | 구분자 기반 라인 읽기 |
| length_prefix | 4바이트 빅엔디안 길이 접두사 |
| fixed_size | 고정 크기 바이트 읽기 |
| stream | 유휴 타임아웃 기반 스트림 프레이밍 (idle_timeout 동안 추가 데이터 없으면 플러시, 기본 1ms) |
| frame | 프로토콜 수준 프레임 감지 (STX/길이/페이로드/ETX/체크섬) |

- 소켓 에이전트의 `Framer` 인터페이스는 `net.Conn`에 의존하므로, 시리얼 전용 `SerialFramer` 인터페이스를 정의한다.
- `SerialFramer`는 `io.ReadWriteCloser`를 대상으로 Read/Write를 수행한다.
- 구현 로직은 소켓 Framer와 동일하되, 인터페이스 시그니처만 다르다.

#### frame 프레이밍 모드

**WHEN** 사용자가 `framing="frame"`으로 시리얼 에이전트를 설정할 때, **THEN** 시스템은 프로토콜 수준의 프레임 감지를 수행해야 한다.

**프레임 감지 절차:**

1. STX 바이트를 대기한다
2. 길이 필드를 읽는다
3. `length_includes_header` 적용 후 `length_adjustment` 보정값을 더한다
4. 보정된 길이에 기반하여 나머지 바이트를 읽는다
5. (선택) ETX를 검증한다
6. (선택) 체크섬을 검증한다
7. STX부터 ETX(포함)까지 완전한 프레임을 단일 메시지로 반환한다

**설정 필드:**

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| stx | string | 필수 | - | 프레임 시작 마커 (hex 문자열, 예: "02", "AA55") |
| etx | string | 선택 | "" | 프레임 종료 마커 (hex 문자열, 예: "03"), 빈 문자열이면 ETX 검증 생략 |
| length_offset | int | 선택 | 1 | 프레임 시작부터 길이 필드까지의 오프셋 (바이트) |
| length_size | int | 선택 | 1 | 길이 필드 크기 (1 또는 2 바이트) |
| length_endian | string | 선택 | "big" | 길이 필드 엔디안 ("big" 또는 "little") |
| length_includes_header | bool | 선택 | false | 길이 값이 헤더 바이트를 포함하는지 여부 (false=페이로드만) |
| length_adjustment | int | 선택 | 0 | 디코딩된 길이에 더할 보정값. length_includes_header 적용 후 적용된다. 예: NASA 프로토콜은 -1 |
| checksum | string | 선택 | "none" | 체크섬 유형 ("none", "sum8", "xor") |

**검증 규칙:**

- **IF** `stx`가 빈 문자열이거나 유효하지 않은 hex이면, **THEN** `ErrInvalidSTX` 에러를 반환해야 한다.
- **IF** `length_size`가 1 또는 2가 아니면, **THEN** `ErrInvalidLengthSize` 에러를 반환해야 한다.
- **IF** `length_endian`이 "big" 또는 "little"이 아니면, **THEN** `ErrInvalidEndian` 에러를 반환해야 한다.
- **IF** `checksum`이 "none", "sum8", "xor" 중 하나가 아니면, **THEN** `ErrInvalidChecksum` 에러를 반환해야 한다.
- **IF** 수신된 프레임의 체크섬이 일치하지 않으면, **THEN** 프레임을 폐기하고 에러를 로그에 기록해야 한다. 다음 STX부터 재탐색한다.
- **IF** 수신된 프레임의 ETX가 기대값과 불일치하면, **THEN** 프레임을 폐기하고 다음 STX부터 재탐색한다.

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
| REQ-SERIAL-007 | framing.go | SerialFramer, rawFramer, newlineFramer, lengthPrefixFramer, fixedSizeFramer, streamFramer, frameFramer, FrameConfig |
| REQ-SERIAL-008 | bridge.go (기존) | BridgeNode 방향 처리 |
| REQ-SERIAL-009 | adapter/serial.go | SerialAdapter |
| REQ-SERIAL-010 | agent.go | Health, Stats, Info, TransportConnected |

---

## 5. 구현 노트 (Implementation Notes)

### 구현 일자
- 2026-04-01

### 구현 결과
- 전체 요구사항 REQ-SERIAL-001 ~ REQ-SERIAL-015 구현 완료
- 테스트 커버리지: serial 패키지 94.3%, adapter 89.4%
- `go test -race` 통과 (동시성 안전성 검증)

### 설계 결정 기록
- **프레이밍 전략**: Option B 채택 -- `io.Reader`/`io.Writer` 기반 별도 SerialFramer. 소켓 Framer(`net.Conn` 기반)와 독립
- **포트 추상화**: `serialPort` 인터페이스 + `serialOpener` 함수 타입으로 테스트 가능성 확보
- **동시성 모델**: Read는 별도 goroutine, Write는 `sync.Mutex` 직렬화, 상태는 `atomic.Bool`
- **length_adjustment**: NASA 프로토콜처럼 LEN 의미가 표준과 다른 경우를 위한 정수 보정값. `length_includes_header` 적용 후 최종 페이로드 길이에 더한다
- **프레이밍 에러 복원력**: ETX/체크섬/크기 오류는 해당 프레임만 폐기하고 readLoop 계속. USB 분리 등 물리적 에러만 Error 상태 전이
- **MultiSourceNode**: 엔진 레벨 멀티포트 라우팅 인터페이스. SerialInNode이 `raw_out` 채널을 엔진에 등록하면, 엔진이 포트별 goroutine으로 메시지를 라우팅

### 파일 목록
| 파일 | 역할 |
|------|------|
| internal/agent/serial/common.go | 상수 및 기본값 |
| internal/agent/serial/errors.go | 13개 센티널 에러 |
| internal/agent/serial/config.go | SerialConfig + ParseSerialConfig |
| internal/agent/serial/framing.go | SerialFramer 인터페이스 및 6종 구현체 (raw, newline, length_prefix, fixed_size, stream, frame) |
| internal/agent/serial/agent.go | SerialAgent (5개 인터페이스 구현) |
| internal/agent/serial/register.go | RegisterSerialTypes |
| internal/node/adapter/serial.go | SerialAdapter (BridgeAdapter, 기본 방향: inout) |
| cmd/xflowd/main.go | 시리얼 에이전트 등록 추가 |
| internal/node/base.go | MultiSourceNode 인터페이스 정의 |
| internal/engine/engine.go | groupWiresBySourcePort, MultiSourceNode 라우팅 |
| web/src/config/agentSchemas.ts | SERIAL_FIELDS (22개 ConfigField, frame 모드 visibleWhen) |

---

---

## v2.0.0 → v2.1.0 확장 (Extension)

> v2.0.0은 기존 완료된 v1.0.0의 확장이다. REQ-SERIAL-007에 stream/frame 프레이밍 모드를 통합하고, REQ-SERIAL-012(raw_out 포트)를 신규 추가한다.
> v2.1.0은 실제 NASA 프로토콜 디바이스 연동 과정에서 발견된 문제를 해결하며, length_adjustment 필드, 프레이밍 에러 복원력, Web UI 설정 스키마, MultiSourceNode 엔진 확장을 추가한다.

### 2.3 추가 가정

- A8: 프레임 모드(`frame`)의 STX/ETX 바이트 패턴은 프로토콜마다 다르므로, 사용자가 hex 문자열로 직접 설정한다.
- A9: 프레임 길이 필드는 최대 2바이트(65535)까지 지원하며, 이 범위를 초과하는 프레임은 에러로 처리한다.
- A10: `raw_out` 포트는 디버깅/로깅 목적이며, 연결된 노드가 없을 경우 성능에 영향을 주지 않아야 한다.

---

## 3-EXT. 요구사항 v2.0.0 (Requirements Extension)

> REQ-SERIAL-007에 stream/frame 프레이밍 모드가 통합되었다. 아래는 신규 요구사항만 기술한다.

### REQ-SERIAL-012: serial-in 노드의 raw_out 출력 포트

**WHEN** serial-in 노드가 시리얼 데이터를 수신할 때, **THEN** 시스템은 두 개의 출력 포트로 데이터를 전달해야 한다:

- **`out`**: 기존 프레이밍이 적용된 메시지 (변경 없음)
- **`raw_out`**: 프레이밍 적용 **이전**의 원시(raw) 바이트 청크

**구현 조건:**

- `raw_out`은 시리얼 포트의 `Read()` 호출에서 직접 얻은 바이트를 전달한다.
- `raw_out`에 연결된 노드가 없을 경우, 원시 데이터 전달 로직은 생략되어 성능 오버헤드가 없어야 한다.
- 시리얼 에이전트에 `rawCh` 채널을 추가하여, 프레이머 처리 **이전**에 원시 바이트를 전달한다.
- `RawMessageReceiver` 인터페이스: `ReceiveRawMessage() <-chan []byte`

### REQ-SERIAL-013: 프레이밍 에러 복원력

**WHEN** readLoop에서 프레이밍 에러(ETX 불일치, 체크섬 불일치, 프레임 크기 초과)가 발생할 때, **THEN** 시스템은 해당 프레임만 폐기하고 다음 프레임 읽기를 계속해야 한다.

- 프레이밍 에러는 `isFramingError()` 헬퍼로 식별한다 (`ErrETXMismatch`, `ErrChecksumMismatch`, `ErrFrameTooLarge`)
- WARN 레벨 로그를 기록하고 readLoop을 계속한다
- 에이전트가 Error 상태로 전이하지 않는다
- USB 분리(ENXIO/EIO), EOF 등 물리적 에러만 Error 상태로 전이한다

### REQ-SERIAL-014: Web UI 시리얼 에이전트 설정 폼

시스템은 **항상** Web UI에서 시리얼 에이전트의 모든 설정 필드를 구성할 수 있어야 한다.

- `agentSchemas.ts`에 `SERIAL_FIELDS` 배열로 22개 ConfigField 등록
- `framing` 필드의 값에 따라 관련 설정 필드가 `visibleWhen` 조건으로 표시/숨김
- Web UI select/boolean 필드에서 전달되는 문자열 값을 `toInt`/`toBool` 헬퍼로 변환
- `nodeSchemas.ts`의 serial-in 노드에 `raw_out` 출력 포트 등록

### REQ-SERIAL-015: MultiSourceNode 엔진 확장

**WHEN** SourceNode가 `MultiSourceNode` 인터페이스를 구현할 때, **THEN** 엔진은 추가 출력 채널을 포트 이름별로 라우팅해야 한다.

- `MultiSourceNode` 인터페이스: `ExtraSourceChannels() map[string]<-chan message.Message`
- 엔진의 `groupWiresBySourcePort` 헬퍼로 와이어를 포트별 그룹화
- 포트별 독립 goroutine으로 메시지를 라우팅
- `SerialInNode`이 `MultiSourceNode`를 구현하여 `raw_out` 채널을 등록

### REQ-SERIAL-016: 노드 Init-tolerance (deferred connection) (v2.2.0)

**WHEN** `serial-in` 또는 `serial-out` 노드의 `Init()` 이 호출되어 `AgentResolver` 를 통해 `agent_ref` 에이전트를 resolve 하려 했으나 에이전트를 찾을 수 없으면(disabled 또는 미등록), **THEN** 시스템은 다음을 수행해야 한다:

1. **hard-fail 하지 않는다** — `Init()` 은 에러를 반환하지 않는다
2. 경고(WARNING) 로그를 남긴다 (에이전트 참조, 노드 ID 포함)
3. 노드를 `Running` 상태로 전이시킨다 — 단, 에이전트 연결은 보류(deferred connection)된다
4. 이후 해당 에이전트가 활성화되면 SPEC-ENGINE-001 `ReinitNodesForAgent` 경로를 통해 노드가 자동으로 재초기화·재연결된다

**IF** `AgentResolver` 자체가 설정되지 않은 경우(구성 오류)이면, **THEN** 시스템은 기존대로 `Init()` 에서 hard-fail(에러 반환)해야 한다. resolver 미설정은 deferred connection 으로 회복 불가능한 구성 오류이기 때문이다.

> 본 요구사항은 "에이전트 가용성에 무관하게 flow 를 배포·운영" 하기 위한 것으로,
> SPEC-AGENT-005 v1.1.0 (R5.7~R5.9), SPEC-ENGINE-001 v1.3.0 Module 8 과 연계된다.

### REQ-SERIAL-017: serial-out 노드 input_encoding 옵션 (v2.3.0)

**WHEN** `serial-out` 노드가 문자열 형태의 `data` 또는 `raw` 페이로드를 시리얼 바이트로 변환할 때, **THEN** 시스템은 노드 설정의 `input_encoding` 옵션에 따라 변환 방식을 결정해야 한다.

| 값 | 동작 |
|----|------|
| `auto` (기본값) | hex 디코딩을 먼저 시도하고 실패 시 text(ASCII)로 폴백. v2.2.0 이전 동작과 바이트 동일하다. |
| `hex` | 항상 `hex.DecodeString` 으로 디코딩. 디코딩 실패 시 에러. |
| `text` | 항상 평문 텍스트 바이트(`[]byte(str)`)로 변환. |
| `base64` | 항상 base64 로 디코딩. |

**검증/동작 규칙:**

- **IF** `raw` 페이로드가 이미 `[]byte` 타입이면, **THEN** 시스템은 `input_encoding` 을 적용하지 않고 바이트를 그대로 전송해야 한다 (인코딩 우회).
- `auto` 모드는 유효한 hex 문자열로 보이는 평문(예: `"abcdef"`)이 의도치 않게 hex 디코딩되는 모호성을 가진다. `hex`/`text`/`base64` 명시 선택으로 이 모호성을 제거할 수 있다.
- 변환 로직은 `decodeSerialPayloadString` 헬퍼로 일원화한다.

### REQ-SERIAL-018: 시리얼 에이전트 메시지 드롭 로그 제어 (log_drops) (v2.3.0)

**WHEN** 시리얼 에이전트의 수신 버퍼(`msgCh`)가 가득 차서 인바운드 메시지를 드롭할 때, **THEN** 시스템은 `log_drops` 설정이 `true` 인 경우에만 per-drop WARN 로그 `시리얼 에이전트 메시지 버퍼 가득 참, 드롭` 을 출력해야 한다.

- `log_drops` 기본값은 `false` 이며, 운영 환경에서 드롭 WARN 로그가 저널을 범람시키는 것을 억제한다.
- **드롭 발생 시 `log_drops` 값과 무관하게 항상 `stats.IncrDroppedMessages()` 로 드롭을 계수해야 한다.** 로그를 끄더라도 드롭은 에이전트 통계를 통해 관측 가능해야 한다.
- 본 패턴은 samsung_hvacr01 에이전트의 `log_decode_errors` 옵션 패턴과 일관된다.

---

## 4-EXT. 명세 v2.0.0 (Specifications Extension)

### 4.7 프레임 프레이머 (frameFramer) — REQ-SERIAL-007 확장

```go
// FrameConfig 는 frame 프레이밍의 설정을 담는다.
type FrameConfig struct {
    STX                 []byte // 프레임 시작 마커
    ETX                 []byte // 프레임 종료 마커 (빈 슬라이스면 검증 생략)
    LengthOffset        int    // STX부터 길이 필드까지 오프셋
    LengthSize          int    // 길이 필드 크기 (1 또는 2)
    LengthEndian        string // "big" 또는 "little"
    LengthIncludesHeader bool  // true면 길이 = 헤더+페이로드, false면 길이 = 페이로드만
    LengthAdjustment     int   // 디코딩된 길이에 더할 보정값 (length_includes_header 후 적용)
    Checksum            string // "none", "sum8", "xor"
}
```

`frameFramer`는 `SerialFramer` 인터페이스를 구현한다:

- `Read(r io.Reader) ([]byte, error)`: STX 탐색 -> 길이 필드 읽기 -> 페이로드 읽기 -> (선택) ETX 검증 -> (선택) 체크섬 검증 -> 완전한 프레임 반환
- `Write(w io.Writer, data []byte) error`: 데이터를 그대로 쓴다 (프레임 구성은 호출자 책임)

**체크섬 알고리즘:**

- `sum8`: STX부터 체크섬 바이트 직전까지의 모든 바이트 합을 `& 0xFF`
- `xor`: STX부터 체크섬 바이트 직전까지의 모든 바이트를 XOR

### 4.8 SerialConfig 확장 필드 — REQ-SERIAL-007 확장

기존 `SerialConfig` 구조체에 다음 필드를 추가한다:

| 필드 | 타입 | 설명 |
|------|------|------|
| STX | []byte | 프레임 시작 마커 |
| ETX | []byte | 프레임 종료 마커 |
| LengthOffset | int | 길이 필드 오프셋 |
| LengthSize | int | 길이 필드 크기 |
| LengthEndian | string | 길이 필드 엔디안 |
| LengthIncludesHeader | bool | 길이에 헤더 포함 여부 |
| LengthAdjustment | int | 길이 보정값 (length_includes_header 후 적용) |
| Checksum | string | 체크섬 유형 |

`ParseSerialConfig`에 `framing="frame"` 시 추가 필드 파싱 로직을 추가한다.

### 4.9 SerialAgent raw 채널 확장

```go
type SerialAgent struct {
    // ... 기존 필드 ...
    rawCh     chan []byte        // 원시 바이트 채널 (raw_out 포트용)
}
```

- `ReceiveRawMessage() <-chan []byte`: rawCh 반환
- `readLoop` 수정: `SerialConnReader`를 도입하여, `Read()` 시 원시 바이트를 `rawCh`로 전달한 후 프레이머에 데이터를 공급한다.

### 4.10 serial-in 노드 raw_out 포트

```
SerialInNode 포트:
  - out:     프레이밍 적용된 메시지 (기존)
  - raw_out: 원시 바이트 청크 (신규)
  - error:   에러 메시지 (기존)
```

`SerialInNode`에서 에이전트의 `ReceiveRawMessage()` 채널을 감시하여, 수신된 원시 바이트를 `raw_out` 포트로 전달한다.

### 4.11 nodeSchemas.ts 확장

`serial-in` 노드 스키마에 `raw_out` 출력 포트를 추가한다.

### 4.12 추가 에러 정의

```go
var (
    ErrInvalidSTX        = errors.New("serial: invalid stx (must be non-empty hex string)")
    ErrInvalidLengthSize = errors.New("serial: invalid length_size (must be 1 or 2)")
    ErrInvalidEndian     = errors.New("serial: invalid length_endian (must be 'big' or 'little')")
    ErrInvalidChecksum   = errors.New("serial: invalid checksum type (must be 'none', 'sum8', or 'xor')")
    ErrChecksumMismatch  = errors.New("serial: frame checksum mismatch")
    ErrETXMismatch       = errors.New("serial: frame ETX mismatch")
    ErrFrameTooLarge     = errors.New("serial: frame exceeds max_message_size")
)
```

### 4.13 추적성 태그 (v2.0.0 확장)

| 요구사항 | 파일 | 함수/구조체 |
|----------|------|------------|
| REQ-SERIAL-007 | framing.go | frameFramer, FrameConfig (v2.0.0 추가) |
| REQ-SERIAL-007 | config.go | ParseSerialConfig (frame/stream 설정 파싱, v2.0.0 추가) |
| REQ-SERIAL-007 | errors.go | ErrInvalidSTX, ErrInvalidLengthSize, ErrInvalidEndian, ErrInvalidChecksum, ErrChecksumMismatch, ErrETXMismatch |
| REQ-SERIAL-012 | agent.go | rawCh, ReceiveRawMessage, SerialConnReader |
| REQ-SERIAL-012 | serial_io.go | SerialInNode raw_out 포트 |
| REQ-SERIAL-012 | nodeSchemas.ts | serial-in raw_out 포트 스키마 |
| REQ-SERIAL-013 | agent.go | isFramingError, readLoop 프레이밍 에러 복원력 |
| REQ-SERIAL-014 | agentSchemas.ts | SERIAL_FIELDS (22개 ConfigField), toInt/toBool 문자열 변환 |
| REQ-SERIAL-015 | base.go, engine.go | MultiSourceNode 인터페이스, groupWiresBySourcePort |
| REQ-SERIAL-003 (v2.2.0 보강) | agent.go | stopCh 재설정, bounded Stop(5초), Error 상태 회복 |
| REQ-SERIAL-005 (v2.2.0 보강) | agent.go, adapter/serial.go | hex.DecodeString 대칭 인코딩 |
| REQ-SERIAL-016 | serial_io.go | serial-in/serial-out 노드 Init-tolerance, deferred connection |
| REQ-SERIAL-003 (v2.3.0 보강) | agent.go | portIOMu 포트 I/O 직렬화, effectiveReadTimeout 클램프 |
| REQ-SERIAL-004 (v2.3.0 보강) | serial_io.go | receiveLoop/rawReceiveLoop raw 필드 방어적 복사 |
| REQ-SERIAL-005 (v2.3.0 보강) | agent.go | Process 출력 debug 로그 (시리얼 포트 출력 ...) |
| REQ-SERIAL-017 | serial_io.go, nodeSchemas.ts | serial-out input_encoding 옵션, decodeSerialPayloadString |
| REQ-SERIAL-018 | config.go, agent.go, agentSchemas.ts | 시리얼 에이전트 log_drops 옵션, 드롭 통계 계수 유지 |

---

*SPEC 버전: 2.3.0*
*v1.0.0 생성일: 2026-04-01*
*v2.0.0 확장일: 2026-04-01*
*v2.1.0 확장일: 2026-04-01*
*v2.2.0 확장일: 2026-05-14*
*v2.3.0 확장일: 2026-05-14*
*작성: MoAI SPEC Builder*
