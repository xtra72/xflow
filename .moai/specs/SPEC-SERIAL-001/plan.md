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

---

## v2.0.0 확장 구현 계획

### 관련 요구사항

| 요구사항 | 설명 |
|----------|------|
| REQ-SERIAL-007 | 프레임 프레이밍 모드 (frame framing mode) |
| REQ-SERIAL-012 | serial-in 노드 raw_out 출력 포트 |

---

### 6. 기술적 접근 (v2.0.0)

#### 6.1 프레임 프레이머 설계

`frameFramer`는 기존 `SerialFramer` 인터페이스를 구현하며, 바이트 스트림에서 프로토콜 프레임을 감지한다.

**핵심 알고리즘 (Read):**

1. **STX 탐색**: 바이트 단위로 읽으며 STX 패턴과 매칭. 멀티바이트 STX (예: 0xAA55)를 지원하기 위해 상태 머신 방식으로 탐색한다.
2. **헤더 읽기**: STX 이후 `length_offset - len(stx)` 바이트를 추가로 읽어 헤더를 완성한다.
3. **길이 필드 디코딩**: `length_size`와 `length_endian`에 따라 `binary.BigEndian` 또는 `binary.LittleEndian`으로 디코딩한다.
4. **페이로드 읽기**: 계산된 잔여 바이트 수만큼 `io.ReadFull`로 읽는다.
   - `length_includes_header=false`: 잔여 = length - (이미 읽은 헤더 잔여분) + etx_len + checksum_len
   - `length_includes_header=true`: 잔여 = length - (지금까지 읽은 바이트 수) + etx_len + checksum_len (헤더 미포함 부분)
5. **체크섬 검증** (선택): `sum8` 또는 `xor` 계산 후 프레임 내 체크섬 바이트와 비교.
6. **ETX 검증** (선택): 프레임 끝부분이 ETX 패턴과 일치하는지 확인.

**Write 구현**: 프레임 구성은 호출자(Flow) 책임이므로, `Write`는 데이터를 그대로 시리얼 포트에 쓴다.

#### 6.2 raw_out 구현 전략

기존 `readLoop`은 `SerialConnReader`를 통해 포트에서 읽은 데이터를 프레이머에 전달한다. raw_out을 구현하기 위해:

1. **SerialConnReader 수정**: `Read()` 호출마다 읽은 원시 바이트를 `rawCh`로 복사 전송.
2. **rawCh 채널**: `make(chan []byte, 64)` -- 버퍼 크기는 기존 `msgCh`와 동일.
3. **SerialInNode 수정**: 에이전트가 `RawMessageReceiver` 인터페이스를 구현하는지 확인하고, 구현 시 `ReceiveRawMessage()` 채널을 select 루프에 추가하여 `raw_out` 포트로 전달.
4. **성능 최적화**: `raw_out` 포트에 연결된 엣지가 없으면 rawCh 전송을 생략한다. 또는 rawCh에 non-blocking send를 사용하여 수신자 없을 때 드롭한다.

#### 6.3 설정 파싱 확장

`ParseSerialConfig`에 `framing="frame"` 분기를 추가:

- `stx` hex 문자열 -> `[]byte` 변환 (`hex.DecodeString`)
- `etx` hex 문자열 -> `[]byte` 변환 (빈 문자열이면 nil)
- `length_offset`, `length_size`, `length_endian`, `length_includes_header`, `checksum` 파싱
- 유효성 검증: `length_size` ∈ {1, 2}, `length_endian` ∈ {"big", "little"}, `checksum` ∈ {"none", "sum8", "xor"}

---

### 7. 마일스톤 (v2.0.0)

#### Primary Goal: 프레임 프레이밍 모드 (REQ-SERIAL-007)

**작업 목록:**

1. `internal/agent/serial/errors.go` 수정
   - 7개 신규 에러 추가: ErrInvalidSTX, ErrInvalidLengthSize, ErrInvalidEndian, ErrInvalidChecksum, ErrChecksumMismatch, ErrETXMismatch, ErrFrameTooLarge

2. `internal/agent/serial/common.go` 수정
   - `FramingFrame` 상수 추가 ("frame")
   - `validChecksumTypes` 맵 추가

3. `internal/agent/serial/config.go` 수정
   - `SerialConfig`에 FrameConfig 관련 7개 필드 추가
   - `ParseSerialConfig`에 frame 설정 파싱 로직 추가
   - hex 문자열 파싱 유틸리티 함수

4. `internal/agent/serial/framing.go` 수정
   - `FrameConfig` 구조체 정의
   - `frameFramer` 구현 (Read: STX 탐색 + 길이 디코딩 + 페이로드 읽기 + 체크섬/ETX 검증)
   - `NewSerialFramer` 팩토리에 "frame" 케이스 추가

**테스트**: framing_test.go에 frame 프레이머 테스트 추가
- STX 탐색 (단일바이트, 멀티바이트)
- 길이 필드 디코딩 (1바이트/2바이트, big/little endian)
- ETX 검증 (있을 때/없을 때)
- 체크섬 검증 (sum8, xor, none)
- 에러 케이스 (체크섬 불일치, ETX 불일치, 프레임 초과)
- 노이즈 바이트 후 STX 탐색

#### Secondary Goal: raw_out 포트 (REQ-SERIAL-012)

**작업 목록:**

1. `internal/agent/serial/agent.go` 수정
   - `rawCh chan []byte` 필드 추가
   - `ReceiveRawMessage() <-chan []byte` 메서드 추가
   - `SerialConnReader` 수정: Read 시 rawCh로 복사 전송
   - `Start`에서 rawCh 초기화, `Stop`에서 rawCh close

2. `internal/node/serial_io.go` 수정
   - `SerialInNode`에 `raw_out` 포트 추가
   - select 루프에서 rawCh 수신 -> raw_out 포트 전달

3. `web/src/config/nodeSchemas.ts` 수정
   - serial-in 노드 스키마에 `raw_out` 출력 포트 추가

**테스트**: agent_test.go, serial_io_test.go에 raw_out 관련 테스트 추가

#### Tertiary Goal: UI 업데이트

**작업 목록:**

1. `web/src/config/nodeSchemas.ts` 수정
   - serial-in/serial-out 노드의 framing 옵션에 "frame" 추가
   - frame 모드 전용 설정 필드 (stx, etx, length_offset 등) UI 스키마 추가

#### Optional Goal: 통합 테스트

**작업 목록:**

1. 프레임 프레이머 + mock 시리얼 포트 통합 테스트
2. raw_out + framed out 동시 수신 시나리오 테스트
3. 프레임 에러 복구 시나리오 (체크섬 불일치 후 다음 프레임 정상 수신)

---

### 8. 아키텍처 설계 방향 (v2.0.0)

#### 8.1 파일 변경 범위

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| internal/agent/serial/common.go | 수정 | FramingFrame 상수, validChecksumTypes 추가 |
| internal/agent/serial/errors.go | 수정 | 7개 신규 에러 추가 |
| internal/agent/serial/config.go | 수정 | FrameConfig 필드 및 파싱 로직 |
| internal/agent/serial/framing.go | 수정 | frameFramer 구현체 추가 |
| internal/agent/serial/agent.go | 수정 | rawCh, ReceiveRawMessage, SerialConnReader 수정 |
| internal/node/serial_io.go | 수정 | raw_out 포트 추가 |
| web/src/config/nodeSchemas.ts | 수정 | frame 옵션, raw_out 포트 스키마 |

#### 8.2 인터페이스 확장

```go
// RawMessageReceiver 는 프레이밍 이전 원시 바이트를 수신하는 인터페이스이다.
type RawMessageReceiver interface {
    ReceiveRawMessage() <-chan []byte
}
```

SerialInNode는 에이전트가 `RawMessageReceiver`를 구현하는지 타입 어서션으로 확인하고, 구현 시에만 raw_out 채널을 활성화한다. 이 방식은 기존 에이전트와의 하위 호환성을 보장한다.

---

### 9. 리스크 및 대응 (v2.0.0)

| 리스크 | 영향도 | 대응 방안 |
|--------|--------|----------|
| 프레임 경계 오탐지 (STX 패턴이 페이로드 내에 존재) | Medium | 길이 필드 기반으로 프레임 경계를 확정. STX만으로 판단하지 않음 |
| raw_out 채널 블로킹으로 읽기 지연 | Medium | non-blocking send 또는 충분한 버퍼(64)로 드롭 방지 |
| 길이 필드 디코딩 오류 (잘못된 프로토콜 설정) | Low | 최대 메시지 크기(max_message_size) 제한으로 과도한 메모리 할당 방지 |
| 멀티바이트 STX와 바이트 경계 정렬 문제 | Low | 바이트 단위 상태 머신으로 정확한 매칭 |

---

### 10. 전문가 상담 권장 (v2.0.0)

| 영역 | 에이전트 | 이유 |
|------|---------|------|
| 백엔드 구현 | expert-backend | frameFramer 구현, SerialConnReader 설계, 동시성 패턴 |
| 프론트엔드 | expert-frontend | frame 모드 UI 스키마, raw_out 포트 시각화 |

---

---

## v2.1.0 확장 구현 계획

### 관련 요구사항

| 요구사항 | 설명 |
|----------|------|
| REQ-SERIAL-007 (확장) | length_adjustment 길이 보정값 |
| REQ-SERIAL-013 | 프레이밍 에러 복원력 |
| REQ-SERIAL-014 | Web UI 시리얼 에이전트 설정 폼 |
| REQ-SERIAL-015 | MultiSourceNode 엔진 확장 |

### 11. 기술적 접근 (v2.1.0)

#### 11.1 length_adjustment 설계

NASA 프로토콜의 LEN 필드는 LEN 자체(2B) + Body + CRC(2B)를 포함하되 STX/ETX는 미포함한다. 기존 `length_includes_header` boolean으로는 이 의미를 표현할 수 없다.

**해결**: `LengthAdjustment int` 필드를 추가하여 `payloadLen += lengthAdjustment`로 최종 보정한다.

NASA 프로토콜 예시:
- `length_includes_header: false`, `length_adjustment: -1`
- LEN=6 → payloadLen = 6 + (-1) = 5 = body(2) + CRC(2) + ETX(1)

#### 11.2 프레이밍 에러 복원력

readLoop에서 프레이밍 에러 발생 시 에이전트 전체가 Error 상태로 전이하는 문제를 발견.
`isFramingError()` 헬퍼로 재시도 가능한 프레이밍 에러를 식별하여 WARN 로그만 남기고 계속.

#### 11.3 Web UI 설정 스키마

`agentSchemas.ts`에 `SERIAL_FIELDS` 배열로 22개 필드를 정의. `visibleWhen` 조건으로 framing 모드별 필드 표시/숨김. Web UI의 select/boolean 필드가 문자열로 전달되므로 `toInt`/`toBool` 헬퍼를 강화.

#### 11.4 MultiSourceNode 엔진 확장

`internal/node/base.go`에 `MultiSourceNode` 인터페이스를 추가하고, `internal/engine/engine.go`의 SourceNode 처리에서 `groupWiresBySourcePort` 헬퍼로 포트별 라우팅을 구현.

### 12. 마일스톤 (v2.1.0)

#### 작업 완료 목록

1. `internal/agent/serial/config.go` — `LengthAdjustment` 필드, `toInt` string 처리, `toBool` 헬퍼, 빈 framing 폴백
2. `internal/agent/serial/framing.go` — `FramerOptions.LengthAdjustment`, `frameFramer.lengthAdjustment`, Read() 보정 로직
3. `internal/agent/serial/agent.go` — `FramerOptions.LengthAdjustment` 전달, `isFramingError` 헬퍼, readLoop 프레이밍 에러 재시도
4. `internal/node/base.go` — `MultiSourceNode` 인터페이스
5. `internal/engine/engine.go` — `groupWiresBySourcePort`, MultiSourceNode 라우팅 goroutine
6. `internal/node/serial_io.go` — `SerialInNode` raw_out 포트, `ExtraSourceChannels()`
7. `web/src/config/agentSchemas.ts` — `SERIAL_FIELDS` 22개 필드 + `length_adjustment` 필드
8. `web/src/config/nodeSchemas.ts` — serial-in raw_out 포트
9. 테스트: config_test.go, framing_test.go, serial_io_test.go, engine_test.go

### 13. 파일 변경 범위 (v2.1.0)

| 파일 | 변경 유형 | 설명 |
|------|----------|------|
| internal/agent/serial/config.go | 수정 | LengthAdjustment, toInt string, toBool, 빈 framing 폴백 |
| internal/agent/serial/framing.go | 수정 | FramerOptions/frameFramer에 lengthAdjustment 추가, Read() 보정 |
| internal/agent/serial/agent.go | 수정 | isFramingError 헬퍼, readLoop 프레이밍 에러 재시도, LengthAdjustment 전달 |
| internal/node/base.go | 수정 | MultiSourceNode 인터페이스 추가 |
| internal/engine/engine.go | 수정 | groupWiresBySourcePort, MultiSourceNode 라우팅 |
| internal/node/serial_io.go | 신규 | SerialInNode (raw_out, ExtraSourceChannels) |
| web/src/config/agentSchemas.ts | 수정 | SERIAL_FIELDS + length_adjustment 필드 |
| web/src/config/nodeSchemas.ts | 수정 | serial-in raw_out 포트 |

---

*계획 버전: 2.1.0*
*v1.0.0 생성일: 2026-04-01*
*v2.0.0 확장일: 2026-04-01*
*v2.1.0 확장일: 2026-04-01*
*작성: MoAI SPEC Builder*
