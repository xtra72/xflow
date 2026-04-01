# SPEC-SERIAL-001: 인수 기준

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SERIAL-001 |
| 관련 요구사항 | REQ-SERIAL-001 ~ REQ-SERIAL-010 |

---

## 1. 에이전트 등록 및 설정 파싱

### 시나리오 1.1: 시리얼 에이전트 타입 등록

```gherkin
Given Agent Manager가 초기화되어 있다
When RegisterSerialTypes(mgr)를 호출한다
Then "serial" 타입이 에이전트 팩토리에 등록된다
And 등록된 팩토리로 SerialAgent 인스턴스를 생성할 수 있다
```

### 시나리오 1.2: 필수 설정 누락 시 에러

```gherkin
Given 시리얼 에이전트 설정에 port 필드가 없다
When ParseSerialConfig(opts)를 호출한다
Then ErrPortRequired 에러가 반환된다
```

### 시나리오 1.3: 기본값을 사용한 최소 설정

```gherkin
Given 시리얼 에이전트 설정에 port="/dev/ttyUSB0"만 포함되어 있다
When ParseSerialConfig(opts)를 호출한다
Then SerialConfig의 BaudRate는 9600이다
And DataBits는 8이다
And StopBits는 1이다
And Parity는 "none"이다
And ReadTimeout은 100ms이다
And BufferSize는 4096이다
And Framing은 "raw"이다
```

### 시나리오 1.4: 전체 설정 파싱

```gherkin
Given 시리얼 에이전트 설정에 다음이 포함되어 있다:
  | port       | /dev/ttyUSB0 |
  | baud_rate  | 115200       |
  | data_bits  | 7            |
  | stop_bits  | 2            |
  | parity     | even         |
  | read_timeout | 200ms      |
  | buffer_size | 8192        |
  | framing    | newline      |
  | delimiter  | 13           |
When ParseSerialConfig(opts)를 호출한다
Then 모든 필드가 정상적으로 파싱된다
And Delimiter는 '\r' (13)이다
```

### 시나리오 1.5: 잘못된 baud_rate

```gherkin
Given 시리얼 에이전트 설정에 baud_rate=999가 포함되어 있다
When ParseSerialConfig(opts)를 호출한다
Then ErrInvalidBaudRate 에러가 반환된다
```

### 시나리오 1.6: 잘못된 parity 값

```gherkin
Given 시리얼 에이전트 설정에 parity="invalid"가 포함되어 있다
When ParseSerialConfig(opts)를 호출한다
Then ErrInvalidParity 에러가 반환된다
```

---

## 2. 생명주기 관리

### 시나리오 2.1: 정상 시작 및 정지

```gherkin
Given 유효한 설정으로 SerialAgent가 생성되었다
And mock 시리얼 포트가 준비되어 있다
When Init(ctx)를 호출한다
Then 에이전트 상태는 Initializing -> 이후 Start 준비 상태이다
When Start(ctx)를 호출한다
Then 시리얼 포트가 열린다
And 읽기 goroutine이 시작된다
And 에이전트 상태는 Running이다
When Stop(ctx)를 호출한다
Then 읽기 goroutine이 정지된다
And 시리얼 포트가 닫힌다
And 에이전트 상태는 Stopped이다
```

### 시나리오 2.2: Pause/Resume

```gherkin
Given SerialAgent가 Running 상태이다
When Pause(ctx)를 호출한다
Then 에이전트 상태는 Paused이다
And 시리얼 포트는 열린 상태로 유지된다
And 수신 데이터는 무시된다
When Resume(ctx)를 호출한다
Then 에이전트 상태는 Running이다
And 수신 데이터가 다시 msgCh로 전달된다
```

### 시나리오 2.3: 포트 열기 실패

```gherkin
Given 존재하지 않는 시리얼 포트 경로가 설정되어 있다
When Start(ctx)를 호출한다
Then 포트 열기가 실패한다
And 에러가 반환된다
And 에이전트 상태는 Error이다
```

---

## 3. 데이터 수신 (Agent -> Flow)

### 시나리오 3.1: raw 프레이밍으로 데이터 수신

```gherkin
Given SerialAgent가 Running 상태이고 framing="raw"이다
And 시리얼 포트로 "Hello World" 바이트가 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 바이트 데이터를 수신할 수 있다
And Stats의 MessagesReceived가 1 증가한다
And Stats의 BytesReceived가 11 증가한다
```

### 시나리오 3.2: newline 프레이밍으로 라인 수신

```gherkin
Given SerialAgent가 Running 상태이고 framing="newline"이다
And 시리얼 포트로 "line1\nline2\n" 바이트가 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 "line1" 바이트를 수신한다
And 이어서 "line2" 바이트를 수신한다
```

### 시나리오 3.3: length_prefix 프레이밍으로 수신

```gherkin
Given SerialAgent가 Running 상태이고 framing="length_prefix"이다
And 시리얼 포트로 [0x00, 0x00, 0x00, 0x05, 'H', 'e', 'l', 'l', 'o'] 바이트가 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 "Hello" (5바이트)를 수신한다
```

### 시나리오 3.4: fixed_size 프레이밍으로 수신

```gherkin
Given SerialAgent가 Running 상태이고 framing="fixed_size", fixed_size=4이다
And 시리얼 포트로 "ABCDEFGH" (8바이트)가 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 "ABCD" (4바이트)를 수신한다
And 이어서 "EFGH" (4바이트)를 수신한다
```

---

## 4. 데이터 송신 (Flow -> Agent)

### 시나리오 4.1: raw 바이트 전송

```gherkin
Given SerialAgent가 Running 상태이다
And Message의 payload에 raw=[]byte{0x01, 0x02, 0x03}이 포함되어 있다
When Process(msg)를 호출한다
Then 시리얼 포트에 [0x01, 0x02, 0x03] 바이트가 쓰여진다
And Stats의 MessagesSent가 1 증가한다
And Stats의 BytesSent가 3 증가한다
```

### 시나리오 4.2: data 문자열 전송 (raw 없을 때)

```gherkin
Given SerialAgent가 Running 상태이다
And Message의 payload에 data="Hello"가 포함되어 있고 raw 필드는 없다
When Process(msg)를 호출한다
Then 시리얼 포트에 "Hello" 바이트가 쓰여진다
```

### 시나리오 4.3: Running 아닌 상태에서 전송 시도

```gherkin
Given SerialAgent가 Paused 상태이다
When Process(msg)를 호출한다
Then ErrNotRunning 에러가 반환된다
```

---

## 5. USB 디바이스 분리 감지

### 시나리오 5.1: 읽기 중 디바이스 분리

```gherkin
Given SerialAgent가 Running 상태이다
When 시리얼 포트 읽기에서 syscall.ENXIO 에러가 발생한다
Then isDisconnectError가 true를 반환한다
And 에이전트 상태가 Error로 전이된다
And 에러 로그가 기록된다
```

### 시나리오 5.2: 쓰기 중 I/O 에러

```gherkin
Given SerialAgent가 Running 상태이다
When Process(msg) 실행 중 시리얼 포트 쓰기에서 syscall.EIO 에러가 발생한다
Then isDisconnectError가 true를 반환한다
And ErrDeviceDisconnected가 반환된다
```

---

## 6. Bridge Node 어댑터

### 시나리오 6.1: TransformToFlow 변환

```gherkin
Given SerialAdapter가 생성되어 있다
And 시리얼에서 수신한 바이트 데이터 [0x48, 0x65, 0x6C, 0x6C, 0x6F]가 있다
When TransformToFlow(data, meta)를 호출한다
Then Message의 payload.raw는 [0x48, 0x65, 0x6C, 0x6C, 0x6F]이다
And payload.data는 "Hello"이다
And metadata.agent.type이 설정되어 있다
```

### 시나리오 6.2: TransformToAgent 변환 (raw 필드 사용)

```gherkin
Given SerialAdapter가 생성되어 있다
And Message의 payload에 raw=[]byte{0x01, 0x02}가 있다
When TransformToAgent(msg)를 호출한다
Then 반환된 바이트는 [0x01, 0x02]이다
```

### 시나리오 6.3: TransformToAgent 변환 (JSON 폴백)

```gherkin
Given SerialAdapter가 생성되어 있다
And Message의 payload에 raw 필드가 없고 key="value"만 있다
When TransformToAgent(msg)를 호출한다
Then 반환된 바이트는 payload의 JSON 직렬화 결과이다
```

### 시나리오 6.4: DefaultConfig 기본값

```gherkin
Given SerialAdapter가 생성되어 있다
When DefaultConfig()를 호출한다
Then Direction은 "inout"이다
And BufferSize는 1024이다
```

### 시나리오 6.5: Validate 방향 검증

```gherkin
Given SerialAdapter가 생성되어 있다
When Validate(config)를 호출하되 direction="in"이다
Then 에러가 nil이다
When Validate(config)를 호출하되 direction="invalid"이다
Then 에러가 반환된다
```

---

## 7. 상태 정보 제공

### 시나리오 7.1: Health 정보

```gherkin
Given SerialAgent가 Running 상태이고 포트가 열려 있다
When Health()를 호출한다
Then HealthInfo의 Connected는 true이다
```

### 시나리오 7.2: TransportConnected

```gherkin
Given SerialAgent가 Running 상태이다
When TransportConnected()를 호출한다
Then true가 반환된다
Given SerialAgent가 Stopped 상태이다
When TransportConnected()를 호출한다
Then false가 반환된다
```

---

## 8. 품질 게이트

### 8.1 Definition of Done

- [ ] 모든 요구사항(REQ-SERIAL-001 ~ REQ-SERIAL-010)이 구현되었다
- [ ] 단위 테스트 커버리지 85% 이상이다
- [ ] `go test -race ./internal/agent/serial/...` 통과한다
- [ ] `go vet ./internal/agent/serial/...` 경고가 없다
- [ ] `golangci-lint run ./internal/agent/serial/...` 에러가 없다
- [ ] 소켓 에이전트 패턴과 일관된 파일 구조이다
- [ ] 어댑터가 `internal/node/adapter/register.go`에 등록되었다
- [ ] 에이전트가 `RegisterSerialTypes`를 통해 등록 가능하다

### 8.2 검증 방법

| 항목 | 방법 |
|------|------|
| 기능 검증 | 단위 테스트 + mock 시리얼 포트 |
| 동시성 안전 | `go test -race` |
| 코드 품질 | golangci-lint |
| 커버리지 | `go test -coverprofile` |
| 프레이밍 | bytes.Buffer 기반 프레이머 테스트 |
| USB 분리 | syscall.ENXIO/EIO mock 에러 주입 |

---

*인수 기준 버전: 1.0.0*
*생성일: 2026-04-01*
*작성: MoAI SPEC Builder*
