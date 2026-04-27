# SPEC-SERIAL-001: 인수 기준

## 메타데이터

| 항목 | 값 |
|------|-----|
| SPEC ID | SPEC-SERIAL-001 |
| 관련 요구사항 | REQ-SERIAL-001 ~ REQ-SERIAL-015 |

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

---

## v2.0.0 확장 인수 기준

### 관련 요구사항

| 요구사항 | 설명 |
|----------|------|
| REQ-SERIAL-007 | 프레임 프레이밍 모드 |
| REQ-SERIAL-012 | serial-in 노드 raw_out 출력 포트 |

---

## 9. 프레임 프레이밍 설정 파싱 (REQ-SERIAL-007)

### 시나리오 9.1: 유효한 frame 설정 파싱

```gherkin
Given 시리얼 에이전트 설정에 다음이 포함되어 있다:
  | framing         | frame  |
  | stx             | 02     |
  | etx             | 03     |
  | length_offset   | 1      |
  | length_size     | 1      |
  | length_endian   | big    |
  | length_includes_header | false |
  | checksum        | sum8   |
When ParseSerialConfig(opts)를 호출한다
Then SerialConfig의 Framing은 "frame"이다
And STX는 []byte{0x02}이다
And ETX는 []byte{0x03}이다
And LengthOffset은 1이다
And LengthSize는 1이다
And LengthEndian은 "big"이다
And LengthIncludesHeader는 false이다
And Checksum은 "sum8"이다
```

### 시나리오 9.2: 멀티바이트 STX 파싱

```gherkin
Given 시리얼 에이전트 설정에 stx="AA55"가 포함되어 있다
And framing="frame"이다
When ParseSerialConfig(opts)를 호출한다
Then STX는 []byte{0xAA, 0x55}이다
```

### 시나리오 9.3: STX 누락 시 에러

```gherkin
Given 시리얼 에이전트 설정에 framing="frame"이 있다
And stx 필드가 없다
When ParseSerialConfig(opts)를 호출한다
Then ErrInvalidSTX 에러가 반환된다
```

### 시나리오 9.4: 잘못된 STX hex 문자열

```gherkin
Given 시리얼 에이전트 설정에 stx="ZZ"가 포함되어 있다
When ParseSerialConfig(opts)를 호출한다
Then ErrInvalidSTX 에러가 반환된다
```

### 시나리오 9.5: 잘못된 length_size

```gherkin
Given 시리얼 에이전트 설정에 framing="frame", stx="02", length_size=3이 포함되어 있다
When ParseSerialConfig(opts)를 호출한다
Then ErrInvalidLengthSize 에러가 반환된다
```

### 시나리오 9.6: 잘못된 length_endian

```gherkin
Given 시리얼 에이전트 설정에 framing="frame", stx="02", length_endian="middle"이 포함되어 있다
When ParseSerialConfig(opts)를 호출한다
Then ErrInvalidEndian 에러가 반환된다
```

### 시나리오 9.7: 잘못된 checksum 타입

```gherkin
Given 시리얼 에이전트 설정에 framing="frame", stx="02", checksum="crc16"이 포함되어 있다
When ParseSerialConfig(opts)를 호출한다
Then ErrInvalidChecksum 에러가 반환된다
```

### 시나리오 9.8: ETX 생략 시 기본값

```gherkin
Given 시리얼 에이전트 설정에 framing="frame", stx="02"가 포함되어 있다
And etx 필드가 없다
When ParseSerialConfig(opts)를 호출한다
Then ETX는 빈 슬라이스(nil)이다
And ETX 검증은 비활성화된다
```

---

## 10. 프레임 프레이밍 데이터 수신 (REQ-SERIAL-007)

### 시나리오 10.1: 단일바이트 STX 프레임 수신

```gherkin
Given SerialAgent가 Running 상태이고 framing="frame"이다
And stx=0x02, etx=0x03, length_offset=1, length_size=1, checksum="none"
And 시리얼 포트로 [0x02, 0x05, 0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x03] 바이트가 도착한다
  (STX=0x02, Length=5, Payload="Hello", ETX=0x03)
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 [0x02, 0x05, 0x48, 0x65, 0x6C, 0x6C, 0x6F, 0x03] 완전한 프레임을 수신한다
```

### 시나리오 10.2: 멀티바이트 STX 프레임 수신

```gherkin
Given framing="frame", stx="AA55", length_offset=2, length_size=2, length_endian="big"
And 시리얼 포트로 [0xAA, 0x55, 0x00, 0x03, 0x01, 0x02, 0x03] 바이트가 도착한다
  (STX=0xAA55, Length=3, Payload=[0x01, 0x02, 0x03])
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 [0xAA, 0x55, 0x00, 0x03, 0x01, 0x02, 0x03] 프레임을 수신한다
```

### 시나리오 10.3: little-endian 길이 필드

```gherkin
Given framing="frame", stx="02", length_offset=1, length_size=2, length_endian="little"
And 시리얼 포트로 [0x02, 0x03, 0x00, 0x41, 0x42, 0x43, 0x03] 바이트가 도착한다
  (STX=0x02, Length=0x0003 little-endian, Payload="ABC", ETX=0x03)
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 완전한 프레임을 수신한다
And 페이로드는 "ABC" (3바이트)이다
```

### 시나리오 10.4: sum8 체크섬 검증 성공

```gherkin
Given framing="frame", stx="02", etx="03", checksum="sum8"
And 체크섬은 STX부터 체크섬 바이트 직전까지의 합 & 0xFF이다
And 올바른 체크섬이 포함된 프레임이 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 완전한 프레임을 수신한다
```

### 시나리오 10.5: xor 체크섬 검증 성공

```gherkin
Given framing="frame", stx="02", etx="03", checksum="xor"
And 올바른 XOR 체크섬이 포함된 프레임이 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ReceiveMessage() 채널에서 완전한 프레임을 수신한다
```

### 시나리오 10.6: 체크섬 불일치 시 프레임 폐기

```gherkin
Given framing="frame", stx="02", etx="03", checksum="sum8"
And 잘못된 체크섬이 포함된 프레임이 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then 프레임이 폐기된다
And 에러 로그가 기록된다
And 다음 STX부터 재탐색을 시작한다
```

### 시나리오 10.7: ETX 불일치 시 프레임 폐기

```gherkin
Given framing="frame", stx="02", etx="03"
And 프레임의 ETX 위치에 0x04가 있다 (기대값 0x03과 불일치)
When 읽기 goroutine이 데이터를 읽는다
Then 프레임이 폐기된다
And 다음 STX부터 재탐색을 시작한다
```

### 시나리오 10.8: 노이즈 바이트 후 정상 프레임 수신

```gherkin
Given framing="frame", stx="02", etx="03"
And 시리얼 포트로 [0xFF, 0xFE, 0x02, 0x03, 0x41, 0x42, 0x43, 0x03] 바이트가 도착한다
  (노이즈 2바이트 + 정상 프레임)
When 읽기 goroutine이 데이터를 읽는다
Then 노이즈 바이트는 무시된다
And [0x02, 0x03, 0x41, 0x42, 0x43, 0x03] 프레임을 수신한다
```

### 시나리오 10.9: max_message_size 초과 프레임

```gherkin
Given framing="frame", stx="02", max_message_size=1024
And 길이 필드가 2048을 나타내는 프레임이 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ErrFrameTooLarge로 프레임이 폐기된다
And 다음 STX부터 재탐색을 시작한다
```

### 시나리오 10.10: length_includes_header=true

```gherkin
Given framing="frame", stx="02", length_offset=1, length_size=1, length_includes_header=true
And 시리얼 포트로 [0x02, 0x07, 0x48, 0x65, 0x6C, 0x6C, 0x6F] 바이트가 도착한다
  (STX=0x02, Length=7=전체길이, Payload="Hello")
When 읽기 goroutine이 데이터를 읽는다
Then 길이 7에서 헤더(STX+length 필드=2바이트)를 뺀 5바이트를 페이로드로 읽는다
And 완전한 프레임을 수신한다
```

---

## 11. serial-in 노드 raw_out 포트 (REQ-SERIAL-012)

### 시나리오 11.1: raw_out으로 원시 바이트 수신

```gherkin
Given SerialAgent가 Running 상태이고 framing="newline"이다
And serial-in 노드의 raw_out 포트에 노드가 연결되어 있다
And 시리얼 포트에서 "Hello\nWorld\n" 바이트가 한 번의 Read()로 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then raw_out 포트에서 "Hello\nWorld\n" 원시 바이트 청크를 수신한다
And out 포트에서 "Hello"와 "World" 두 개의 프레이밍된 메시지를 수신한다
```

### 시나리오 11.2: raw_out 미연결 시 성능 오버헤드 없음

```gherkin
Given SerialAgent가 Running 상태이다
And serial-in 노드의 raw_out 포트에 연결된 노드가 없다
When 시리얼 포트에서 데이터가 도착한다
Then raw_out 관련 채널 전송이 생략된다
And out 포트는 정상적으로 프레이밍된 메시지를 전달한다
And 메모리/CPU 오버헤드가 없다
```

### 시나리오 11.3: raw_out과 out 동시 수신

```gherkin
Given SerialAgent가 Running 상태이고 framing="frame"이다
And stx=0x02, etx=0x03
And serial-in 노드의 raw_out과 out 포트 모두에 노드가 연결되어 있다
And 시리얼 포트로 [0xFF, 0x02, 0x02, 0x41, 0x42, 0x03] 바이트가 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then raw_out 포트에서 [0xFF, 0x02, 0x02, 0x41, 0x42, 0x03] 원시 바이트를 수신한다
And out 포트에서 [0x02, 0x02, 0x41, 0x42, 0x03] 프레이밍된 메시지를 수신한다
```

### 시나리오 11.4: ReceiveRawMessage 인터페이스

```gherkin
Given SerialAgent가 생성되어 있다
When ReceiveRawMessage()를 호출한다
Then nil이 아닌 <-chan []byte 채널이 반환된다
```

### 시나리오 11.5: nodeSchemas에 raw_out 포트 등록

```gherkin
Given serial-in 노드 스키마가 정의되어 있다
When 노드 스키마의 outputs를 확인한다
Then "out"과 "raw_out"과 "error" 3개의 출력 포트가 정의되어 있다
```

---

## 12. 품질 게이트 (v2.0.0 확장)

### 12.1 Definition of Done

- [ ] REQ-SERIAL-007: frameFramer가 SerialFramer 인터페이스를 구현한다
- [ ] REQ-SERIAL-007: STX 탐색, 길이 디코딩, ETX/체크섬 검증이 동작한다
- [ ] REQ-SERIAL-007: 6가지 설정 검증 에러가 올바르게 반환된다
- [ ] REQ-SERIAL-012: ReceiveRawMessage() 채널로 원시 바이트가 전달된다
- [ ] REQ-SERIAL-012: serial-in 노드의 raw_out 포트가 동작한다
- [ ] REQ-SERIAL-012: raw_out 미연결 시 성능 오버헤드가 없다
- [ ] 단위 테스트 커버리지 85% 이상 (신규 코드)
- [ ] `go test -race ./internal/agent/serial/...` 통과
- [ ] `go vet ./internal/agent/serial/...` 경고 없음
- [ ] nodeSchemas.ts에 frame 모드 UI 스키마가 추가되었다

### 12.2 검증 방법

| 항목 | 방법 |
|------|------|
| 프레임 파싱 | bytes.Buffer에 구성된 프레임 바이트로 frameFramer.Read 테스트 |
| 체크섬 검증 | sum8/xor 계산 결과 비교 테스트 |
| 에러 복구 | 잘못된 프레임 후 정상 프레임 수신 테스트 |
| raw_out 채널 | mock 시리얼 포트 + ReceiveRawMessage 채널 수신 테스트 |
| 성능 (raw_out 미연결) | 벤치마크 테스트 비교 |
| UI 스키마 | nodeSchemas.ts 직접 검증 |

---

---

## v2.1.0 확장 인수 기준

### 관련 요구사항

| 요구사항 | 설명 |
|----------|------|
| REQ-SERIAL-013 | 프레이밍 에러 복원력 |
| REQ-SERIAL-014 | Web UI 시리얼 에이전트 설정 폼 |
| REQ-SERIAL-015 | MultiSourceNode 엔진 확장 |

---

## 13. length_adjustment 길이 보정 (REQ-SERIAL-007 확장)

### 시나리오 13.1: 양수 length_adjustment 보정

```gherkin
Given framing="frame", stx="02", length_offset=1, length_size=1, length_adjustment=2
And 시리얼 포트로 [0x02, 0x01, 'a', 'b', 'c'] 바이트가 도착한다
  (STX=0x02, Length=1, payload=1+2보정=3바이트)
When 읽기 goroutine이 데이터를 읽는다
Then 완전한 프레임 [0x02, 0x01, 'a', 'b', 'c']를 수신한다
```

### 시나리오 13.2: 음수 length_adjustment 보정

```gherkin
Given framing="frame", stx="02", length_offset=1, length_size=1, length_adjustment=-1
And 시리얼 포트로 [0x02, 0x03, 'a', 'b'] 바이트가 도착한다
  (STX=0x02, Length=3, payload=3-1보정=2바이트)
When 읽기 goroutine이 데이터를 읽는다
Then 완전한 프레임 [0x02, 0x03, 'a', 'b']를 수신한다
```

### 시나리오 13.3: NASA 프로토콜 length_adjustment=-1 + ETX

```gherkin
Given framing="frame", stx="32", etx="34", length_size=2, length_endian="big", length_adjustment=-1
And NASA 프로토콜 프레임: STX(0x32) + LEN(0x00,0x06) + body(2B) + CRC(2B) + ETX(0x34)
  LEN=6은 LEN(2)+body(2)+CRC(2) 포함, STX/ETX 미포함
When 읽기 goroutine이 데이터를 읽는다
Then payloadLen = 6 + (-1) = 5 = body(2) + CRC(2) + ETX(1)
And 완전한 프레임을 수신하고 ETX 검증을 통과한다
```

---

## 14. 프레이밍 에러 복원력 (REQ-SERIAL-013)

### 시나리오 14.1: ETX 불일치 시 재시도

```gherkin
Given SerialAgent가 Running 상태이고 framing="frame"이다
And 시리얼 포트로 ETX가 불일치하는 프레임이 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then WARN 로그 "시리얼 프레이밍 오류 (재시도)"가 기록된다
And 에이전트 상태는 Running을 유지한다
And 다음 프레임 읽기를 계속한다
```

### 시나리오 14.2: 체크섬 불일치 시 재시도

```gherkin
Given SerialAgent가 Running 상태이고 framing="frame", checksum="sum8"이다
And 시리얼 포트로 체크섬이 잘못된 프레임이 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then WARN 로그가 기록되고 에이전트는 Running 상태를 유지한다
```

### 시나리오 14.3: 프레임 크기 초과 시 재시도

```gherkin
Given SerialAgent가 Running 상태이고 max_message_size=1024이다
And 시리얼 포트로 길이 필드가 2048인 프레임이 도착한다
When 읽기 goroutine이 데이터를 읽는다
Then ErrFrameTooLarge로 해당 프레임만 폐기하고 다음 프레임을 시도한다
```

### 시나리오 14.4: USB 분리는 Error 상태 전이

```gherkin
Given SerialAgent가 Running 상태이다
When 시리얼 포트 읽기에서 syscall.ENXIO 에러가 발생한다
Then isFramingError가 false를 반환한다
And isDisconnectError가 true를 반환한다
And 에이전트 상태가 Error로 전이된다
```

---

## 15. Web UI 시리얼 에이전트 설정 (REQ-SERIAL-014)

### 시나리오 15.1: agentSchemas에 serial 타입 등록

```gherkin
Given agentSchemas.ts가 로드되어 있다
When AGENT_CONFIG_SCHEMAS를 확인한다
Then 'serial' 키에 SERIAL_FIELDS 배열이 등록되어 있다
And 22개 ConfigField가 정의되어 있다
```

### 시나리오 15.2: frame 모드 필드 조건부 표시

```gherkin
Given serial 에이전트 설정 폼이 열려 있다
When framing 필드를 "frame"으로 선택한다
Then stx, etx, length_offset, length_size, length_endian, length_includes_header, length_adjustment, checksum 필드가 표시된다
When framing 필드를 "raw"로 변경한다
Then frame 전용 필드가 숨겨진다
```

### 시나리오 15.3: Web UI select 필드 문자열 변환

```gherkin
Given Web UI에서 length_size="2" (문자열)로 전달된다
When ParseSerialConfig(opts)를 호출한다
Then toInt가 문자열 "2"를 정수 2로 변환한다
And LengthSize는 2이다
```

### 시나리오 15.4: 빈 framing 문자열 폴백

```gherkin
Given 저장된 에이전트 설정에 framing="" (빈 문자열)이 있다
When ParseSerialConfig(opts)를 호출한다
Then Framing은 기본값 "raw"로 설정된다
And 에러가 반환되지 않는다
```

---

## 16. MultiSourceNode 엔진 확장 (REQ-SERIAL-015)

### 시나리오 16.1: SerialInNode이 MultiSourceNode 구현

```gherkin
Given SerialInNode이 RawMessageReceiver를 지원하는 에이전트로 초기화되었다
When ExtraSourceChannels()를 호출한다
Then "raw_out" 키에 message.Message 채널이 반환된다
```

### 시나리오 16.2: ExtraSourceChannels 비활성 시

```gherkin
Given SerialInNode이 RawMessageReceiver를 지원하지 않는 에이전트로 초기화되었다
When ExtraSourceChannels()를 호출한다
Then nil이 반환된다
```

### 시나리오 16.3: 엔진 포트별 와이어 그룹화

```gherkin
Given RuntimeWire 3개가 있다: out(2개), raw_out(1개)
When groupWiresBySourcePort(wires)를 호출한다
Then "out" 키에 2개, "raw_out" 키에 1개의 와이어가 그룹화된다
```

---

## 17. 품질 게이트 (v2.1.0 확장)

### 17.1 Definition of Done

- [x] REQ-SERIAL-007: length_adjustment 필드 추가 및 frameFramer 적용
- [x] REQ-SERIAL-013: 프레이밍 에러 시 재시도 (isFramingError)
- [x] REQ-SERIAL-014: agentSchemas.ts SERIAL_FIELDS 22개 필드, visibleWhen 조건
- [x] REQ-SERIAL-014: toInt/toBool 문자열 변환, 빈 framing 폴백
- [x] REQ-SERIAL-015: MultiSourceNode 인터페이스, groupWiresBySourcePort
- [x] 단위 테스트: length_adjustment 양수/음수/NASA 시나리오, 빈 framing 폴백
- [x] `go test -race ./internal/agent/serial/...` 통과
- [x] `go test -race ./internal/engine/...` 통과
- [x] `go test -race ./internal/node/...` 통과

---

*인수 기준 버전: 2.1.0*
*v1.0.0 생성일: 2026-04-01*
*v2.0.0 확장일: 2026-04-01*
*v2.1.0 확장일: 2026-04-01*
*작성: MoAI SPEC Builder*
