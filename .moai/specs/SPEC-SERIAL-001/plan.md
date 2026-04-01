# SPEC-SERIAL-001: 구현 계획

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SERIAL-001 |
| 관련 요구사항 | REQ-SERIAL-001 ~ REQ-SERIAL-010 |

---

## 1. 기술적 접근

### 1.1 프레이밍 전략: io.ReadWriteCloser 기반 별도 인터페이스

소켓 Framer는 `net.Conn`에 의존하므로 시리얼 포트(`io.ReadWriteCloser`)와 직접 호환되지 않는다. 세 가지 옵션을 검토한 결과:

- **Option A**: `net.Conn` wrapper 생성 -- 불필요한 복잡성 (net.Conn의 Addr, Deadline 등 불필요)
- **Option B**: `io.Reader`/`io.Writer` 기반 별도 SerialFramer -- **채택**
- **Option C**: 소켓 Framer를 `io.ReadWriteCloser`로 리팩토링 -- 기존 코드 변경 (breaking change)

Option B를 채택한다. 구현 로직은 소켓 Framer와 동일하되 인터페이스 시그니처만 `io.Reader`/`io.Writer`를 사용한다. 향후 필요 시 공통 인터페이스로 통합할 수 있다.

### 1.2 에이전트 패턴: 소켓 에이전트 구조 준수

소켓 에이전트의 파일 구조와 패턴을 그대로 따른다:

- `common.go`: 상수, 기본값
- `errors.go`: 센티널 에러
- `config.go`: 타입 안전한 설정 파싱 (`map[string]any` -> 구조체)
- `framing.go`: SerialFramer 인터페이스 + 4가지 구현체
- `agent.go`: SerialAgent (Agent + MessageReceiver 인터페이스)
- `register.go`: RegisterSerialTypes 팩토리 등록

### 1.3 동시성 모델

- **읽기**: 별도 goroutine에서 `framer.Read(port)` 루프. `stopCh` 채널로 정지 신호.
- **쓰기**: `Process()` 메서드에서 `sync.Mutex`로 직렬화. `framer.Write(port, data)`.
- **Pause/Resume**: `atomic.Bool` 플래그로 읽기 goroutine이 데이터를 무시/처리.

### 1.4 USB 분리 감지

기존 Samsung NASA, LG 에이전트의 `serial_opener.go`에서 검증된 패턴을 재사용한다:

```go
func isDisconnectError(err error) bool {
    var errno syscall.Errno
    if errors.As(err, &errno) {
        return errno == syscall.ENXIO || errno == syscall.EIO
    }
    return false
}
```

---

## 2. 마일스톤

### Primary Goal: 핵심 에이전트 구현

**범위**: REQ-SERIAL-001, REQ-SERIAL-002, REQ-SERIAL-003, REQ-SERIAL-005, REQ-SERIAL-010

**작업 목록**:

1. `internal/agent/serial/common.go` 생성
   - 상수 정의: DefaultBaudRate, DefaultDataBits, DefaultStopBits, DefaultParity, DefaultReadTimeout, DefaultBufferSize
   - 프레이밍 타입 상수: FramingRaw, FramingNewline, FramingLengthPrefix, FramingFixedSize
   - 유효값 맵: validBaudRates, validDataBits, validStopBits, validParities

2. `internal/agent/serial/errors.go` 생성
   - 12개 센티널 에러 정의

3. `internal/agent/serial/config.go` 생성
   - `SerialConfig` 구조체
   - `ParseSerialConfig(opts map[string]any) (SerialConfig, error)` 함수
   - go.bug.st/serial의 `serial.Mode` 변환 메서드

4. `internal/agent/serial/agent.go` 생성
   - `SerialAgent` 구조체 (BaseLifecycle 임베딩)
   - Init, Start, Stop, Pause, Resume 구현
   - Process 구현 (데이터 송신)
   - Health, Stats, Info, TransportConnected 구현

5. `internal/agent/serial/register.go` 생성
   - `RegisterSerialTypes(mgr *agent.DefaultManager) error`

**테스트**: config_test.go, agent_test.go, register_test.go

### Secondary Goal: 프레이밍 시스템

**범위**: REQ-SERIAL-007

**작업 목록**:

1. `internal/agent/serial/framing.go` 생성
   - `SerialFramer` 인터페이스 (`Read(io.Reader)`, `Write(io.Writer)`)
   - `NewSerialFramer(framingType string, opts FramerOptions) (SerialFramer, error)` 팩토리
   - `rawFramer` 구현 (io.Read 래핑)
   - `newlineFramer` 구현 (bufio.Scanner + 구분자)
   - `lengthPrefixFramer` 구현 (4바이트 빅엔디안 + io.ReadFull)
   - `fixedSizeFramer` 구현 (io.ReadFull)

**테스트**: framing_test.go -- 각 프레이머별 정상/에러 케이스

### Tertiary Goal: 수신 및 어댑터

**범위**: REQ-SERIAL-004, REQ-SERIAL-006, REQ-SERIAL-008, REQ-SERIAL-009

**작업 목록**:

1. `internal/agent/serial/agent.go`에 readLoop 추가
   - `ReceiveMessage() <-chan []byte` 구현
   - 읽기 goroutine: framer.Read -> msgCh
   - USB 분리 감지: `isDisconnectError` -> Error 상태 전이

2. `internal/node/adapter/serial.go` 생성
   - `SerialAdapter` 구조체
   - `Validate`, `DefaultConfig`, `TransformToFlow`, `TransformToAgent`, `HandleControl`
   - 소켓 어댑터와 동일 패턴, direction 기본값만 inout

3. `internal/node/adapter/register.go` 수정
   - `RegisterAdapter("serial", NewSerialAdapter())` 추가

**테스트**: serial_test.go (어댑터), agent_test.go (readLoop 관련 추가)

### Optional Goal: 통합 테스트

**범위**: 전체 통합 검증

**작업 목록**:

1. 시리얼 포트 목(mock) 구현 (`io.ReadWriteCloser` 인터페이스)
2. 에이전트 + Bridge Node + Flow 통합 시나리오 테스트
3. Pause/Resume 시나리오 검증
4. 동시 쓰기 경합 테스트 (`-race` 플래그)

---

## 3. 아키텍처 설계 방향

### 3.1 패키지 의존성

```
internal/agent/serial/
  └── go.bug.st/serial       (시리얼 포트 I/O)
  └── pkg/lifecycle           (BaseLifecycle)
  └── internal/agent          (Agent 인터페이스, AgentConfig, AgentStats)
  └── pkg/message             (Message 인터페이스)

internal/node/adapter/serial.go
  └── internal/node           (BridgeAdapter, BridgeConfig, AgentMeta)
  └── pkg/flow                (BridgeDirection 상수)
  └── pkg/message             (Message 인터페이스)
```

### 3.2 테스트 전략

- **단위 테스트**: `serial.Port`를 인터페이스로 추상화하여 mock 주입
- **프레이밍 테스트**: `bytes.Buffer`를 `io.ReadWriteCloser`로 활용
- **동시성 테스트**: `go test -race` 필수
- **커버리지 목표**: 85% 이상

### 3.3 시리얼 포트 추상화

테스트 용이성을 위해 `go.bug.st/serial.Port`를 래핑하는 인터페이스를 정의한다:

```go
// serialPort 는 테스트를 위한 시리얼 포트 추상화이다.
type serialPort interface {
    io.ReadWriteCloser
}

// serialOpener 는 테스트를 위한 시리얼 포트 열기 추상화이다.
type serialOpener func(portName string, mode *serial.Mode) (serial.Port, error)
```

프로덕션에서는 `serial.Open`을 사용하고, 테스트에서는 mock을 주입한다.

---

## 4. 리스크 및 대응

| 리스크 | 영향도 | 대응 방안 |
|--------|--------|----------|
| 플랫폼별 시리얼 포트 동작 차이 | Medium | go.bug.st/serial이 추상화. 플랫폼 특화 에러만 별도 처리 |
| USB 핫플러그 시 goroutine 누수 | High | stopCh + context 기반 graceful shutdown |
| 동시 Read/Write 경합 | Medium | Write는 Mutex, Read는 별도 goroutine으로 분리 |
| 소켓 Framer와의 코드 중복 | Low | 향후 공통 인터페이스 추출 가능. 현재는 독립 구현으로 안정성 우선 |

---

## 5. 전문가 상담 권장

| 영역 | 에이전트 | 이유 |
|------|---------|------|
| 백엔드 구현 | expert-backend | Agent 인터페이스 구현, 동시성 패턴, 테스트 전략 |
| 프론트엔드 (선택) | expert-frontend | Web Dashboard에 시리얼 에이전트 설정 UI 추가 시 |

---

*계획 버전: 1.0.0*
*생성일: 2026-04-01*
*작성: MoAI SPEC Builder*
