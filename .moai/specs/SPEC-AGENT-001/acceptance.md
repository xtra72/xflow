---
id: SPEC-AGENT-001
type: acceptance
version: "1.1.0"
spec_ref: SPEC-AGENT-001
---

# SPEC-AGENT-001 수락 기준

## Module 1: Agent Interface & BaseAgent

### AC-AGENT-001-01: BaseAgent 초기화

```gherkin
Given 유효한 AgentConfig (ID: "agent-01", Name: "Test Agent", Type: "custom")가 주어졌을 때
When BaseAgent.Init(config)를 호출하면
Then nil error를 반환해야 한다
And Agent의 상태가 Initializing을 거쳐 Initialized가 되어야 한다
And Agent의 ID()가 "agent-01"이어야 한다
And Agent의 Name()이 "Test Agent"여야 한다
And Agent의 Type()이 "custom"이어야 한다
```

### AC-AGENT-001-02: BaseAgent 생명주기 전이

```gherkin
Given 초기화된 BaseAgent가 주어졌을 때
When Start(ctx)를 호출하면
Then Agent의 상태가 Running이 되어야 한다

When Pause(ctx)를 호출하면
Then Agent의 상태가 Paused가 되어야 한다

When Resume(ctx)를 호출하면
Then Agent의 상태가 Running이 되어야 한다

When Stop(ctx)를 호출하면
Then Agent의 상태가 Stopped가 되어야 한다
```

### AC-AGENT-001-03: BaseAgent 잘못된 상태 전이 거부

```gherkin
Given Stopped 상태인 BaseAgent가 주어졌을 때
When Pause(ctx)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidStateTransition)이 true여야 한다
```

### AC-AGENT-001-04: BaseAgent Paused 상태 버퍼링

```gherkin
Given Running 상태인 BaseAgent가 Pause(ctx)로 Paused가 된 후
When Transport에서 100바이트 데이터가 수신되면
Then 내부 버퍼에 100바이트가 축적되어야 한다
And Process()가 호출되지 않아야 한다 (처리 중단)
```

### AC-AGENT-001-05: BaseAgent Resume 시 버퍼 우선 처리

```gherkin
Given Paused 상태에서 100바이트가 버퍼에 축적된 BaseAgent가 주어졌을 때
When Resume(ctx)를 호출하면
Then 버퍼에 축적된 100바이트가 우선 Process() 처리되어야 한다
And 이후 새로 수신되는 데이터가 정상 처리되어야 한다
```

### AC-AGENT-001-06: BaseAgent 상태 전이 StatusEvent 발행

```gherkin
Given Running 상태인 BaseAgent가 주어졌을 때
When Stop(ctx)를 호출하면
Then StatusEvent가 생성되어야 한다
And StatusEvent의 PreviousState()가 Running이어야 한다
And StatusEvent의 NewState()가 Stopped여야 한다
And StatusEvent의 ComponentType()가 ComponentAgent여야 한다
```

---

## Module 2: Agent Manager

### AC-AGENT-001-07: Manager Agent 생성

```gherkin
Given DefaultManager가 주어졌을 때
When Create(validConfig)를 호출하면
Then nil error를 반환해야 한다
And 생성된 Agent의 ID()가 config.ID와 일치해야 한다
And Manager.Get(agentID)로 조회 가능해야 한다
```

### AC-AGENT-001-08: Manager 중복 ID 생성 거부

```gherkin
Given ID "agent-01"로 이미 Agent가 생성된 Manager가 주어졌을 때
When 동일 ID "agent-01"로 Create()를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrAgentAlreadyExists)가 true여야 한다
```

### AC-AGENT-001-09: Manager 존재하지 않는 Agent 조작 거부

```gherkin
Given Agent가 하나도 없는 Manager가 주어졌을 때
When Start(ctx, "nonexistent")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrAgentNotFound)가 true여야 한다

When Stop(ctx, "nonexistent")를 호출하면
Then errors.Is(err, ErrAgentNotFound)가 true여야 한다

When Restart(ctx, "nonexistent")를 호출하면
Then errors.Is(err, ErrAgentNotFound)가 true여야 한다

When Delete("nonexistent")를 호출하면
Then errors.Is(err, ErrAgentNotFound)가 true여야 한다
```

### AC-AGENT-001-10: Manager Graceful Shutdown

```gherkin
Given 3개의 Running Agent가 있는 Manager가 주어졌을 때
When Shutdown(ctx)를 호출하면
Then 모든 Agent의 상태가 Stopped여야 한다
And nil error를 반환해야 한다
```

### AC-AGENT-001-11: Manager Shutdown context deadline

```gherkin
Given 1개의 Running Agent가 있고, Stop에 5초가 걸리는 Manager가 주어졌을 때
When 1초 deadline의 context로 Shutdown(ctx)를 호출하면
Then context.DeadlineExceeded 관련 error를 반환해야 한다
```

### AC-AGENT-001-12: Manager 동시성 안전

```gherkin
Given DefaultManager가 주어졌을 때
When 10개의 goroutine이 동시에 Create(), Start(), Stop(), Get(), List()를 호출하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
```

---

## Module 3: Agent Registry

### AC-AGENT-001-13: Registry 기본 등록 및 조회

```gherkin
Given DefaultRegistry가 주어졌을 때
When ID "agent-01", Name "Test", Type "custom"인 Agent를 Register()하면
Then nil error를 반환해야 한다
And Get("agent-01")이 (agent, true)를 반환해야 한다
And GetByName("Test")가 (agent, true)를 반환해야 한다
And GetByType("custom")이 해당 Agent를 포함하는 슬라이스를 반환해야 한다
And Count()가 1이어야 한다
```

### AC-AGENT-001-14: Registry 중복 등록 거부

```gherkin
Given ID "agent-01"로 이미 Agent가 등록된 Registry가 주어졌을 때
When 동일 ID "agent-01"로 Register()를 호출하면
Then errors.Is(err, ErrAgentAlreadyExists)가 true여야 한다
```

### AC-AGENT-001-15: Registry 등록 해제

```gherkin
Given ID "agent-01"로 Agent가 등록된 Registry가 주어졌을 때
When Unregister("agent-01")를 호출하면
Then nil error를 반환해야 한다
And Get("agent-01")이 (nil, false)를 반환해야 한다
And Count()가 0이어야 한다
```

### AC-AGENT-001-16: Registry 동시성 안전

```gherkin
Given DefaultRegistry가 주어졌을 때
When 10개의 goroutine이 동시에 Register(), Unregister(), Get(), List()를 호출하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
```

---

## Module 4: Agent Health Check

### AC-AGENT-001-17: HealthStatus 기본 값

```gherkin
Given 새로 생성된 Agent가 주어졌을 때
When Health()를 호출하면
Then HealthStatus.Status가 HealthHealthy여야 한다
And HealthStatus.ConsecutiveFailures가 0이어야 한다
```

### AC-AGENT-001-18: HealthChecker 주기적 체크

```gherkin
Given 100ms 주기로 설정된 HealthChecker가 주어졌을 때
When Start(ctx)를 호출하고 350ms를 기다리면
Then 최소 3회 이상 헬스 체크가 수행되어야 한다
```

### AC-AGENT-001-19: Unhealthy 시 자동 재시작

```gherkin
Given Transport.Available()이 false를 반환하는 Agent가 주어졌을 때
And HealthChecker가 Unhealthy 콜백이 등록되어 있을 때
When 헬스 체크에서 Unhealthy가 감지되면
Then Unhealthy 콜백이 호출되어야 한다
And 지수 백오프 기반으로 재시작이 시도되어야 한다
```

### AC-AGENT-001-20: 최대 재시작 횟수 초과

```gherkin
Given MaxRestarts가 3으로 설정된 Agent가 주어졌을 때
And Transport가 계속 실패하는 상황에서
When 3회 재시작이 모두 실패하면
Then ErrMaxRestartsExceeded 에러가 발행되어야 한다
And Agent 상태가 Stopped로 전환되어야 한다
And 추가 재시작이 시도되지 않아야 한다
```

### AC-AGENT-001-21: 지수 백오프 대기 시간

```gherkin
Given 초기 대기 1초, 배수 2, 최대 5분인 백오프 설정이 주어졌을 때
When 재시작 시도가 연속 실패하면
Then 1차 시도 후 약 1초 대기
And 2차 시도 후 약 2초 대기
And 3차 시도 후 약 4초 대기
And 대기 시간이 5분을 초과하지 않아야 한다
```

---

## Module 5: Shared Agent

### AC-AGENT-001-22: SharedRef 참조 카운팅

```gherkin
Given Agent "agent-01"에 대한 SharedRef가 주어졌을 때
When Acquire("flow-01")를 호출하면
Then RefCount()가 1이어야 한다
And Flows()에 "flow-01"이 포함되어야 한다

When Acquire("flow-02")를 호출하면
Then RefCount()가 2여야 한다

When Release("flow-01")를 호출하면
Then RefCount()가 1이어야 한다
And Flows()에 "flow-01"이 포함되지 않아야 한다
```

### AC-AGENT-001-23: 참조 카운트 0 시 StopOnZeroRef=true

```gherkin
Given StopOnZeroRef=true인 Agent의 SharedRef에서 RefCount가 1인 상태가 주어졌을 때
When Release("flow-01")로 RefCount가 0이 되면
Then Agent의 Stop이 요청되어야 한다
```

### AC-AGENT-001-24: 참조 카운트 0 시 StopOnZeroRef=false

```gherkin
Given StopOnZeroRef=false인 Agent의 SharedRef에서 RefCount가 1인 상태가 주어졌을 때
When Release("flow-01")로 RefCount가 0이 되면
Then Agent는 Running 상태를 유지해야 한다 (Stop 요청 없음)
```

### AC-AGENT-001-25: 중복 플로우 참조 거부

```gherkin
Given "flow-01"로 이미 Acquire된 SharedRef가 주어졌을 때
When 동일 "flow-01"로 Acquire()를 호출하면
Then errors.Is(err, ErrFlowAlreadyReferenced)가 true여야 한다
And RefCount()가 변하지 않아야 한다
```

### AC-AGENT-001-26: SharedRef 동시성 안전

```gherkin
Given SharedRef가 주어졌을 때
When 100개의 goroutine이 동시에 Acquire()와 Release()를 호출하면
Then data race가 발생하지 않아야 한다 (go test -race 통과)
And 최종 RefCount가 정확해야 한다
```

---

## Module 6: Transport Interface

### AC-AGENT-001-27: TransportFactory 타입 등록 및 생성

```gherkin
Given DefaultTransportFactory가 주어졌을 때
When Register("mock", mockCreator)를 호출하고 Create("mock")를 호출하면
Then nil error와 함께 Transport 인스턴스를 반환해야 한다
And Transport의 Available()이 false여야 한다 (아직 Open 전)
```

### AC-AGENT-001-28: TransportFactory 미등록 타입 거부

```gherkin
Given 어떤 Transport도 등록되지 않은 TransportFactory가 주어졌을 때
When Create("unknown")를 호출하면
Then errors.Is(err, ErrTransportNotAvailable)가 true여야 한다
```

### AC-AGENT-001-29: Transport Open/Close 생명주기

```gherkin
Given mock Transport가 주어졌을 때
When Open(config)를 호출하면
Then Available()이 true를 반환해야 한다

When Close()를 호출하면
Then Available()이 false를 반환해야 한다
```

### AC-AGENT-001-30: Transport 닫힌 상태 Read/Write 거부

```gherkin
Given Close()된 Transport가 주어졌을 때
When Read(buf)를 호출하면
Then errors.Is(err, ErrTransportClosed)가 true여야 한다

When Write(data)를 호출하면
Then errors.Is(err, ErrTransportClosed)가 true여야 한다
```

---

## Module 7: Serial Transport

### AC-AGENT-001-31: SerialTransport 설정

```gherkin
Given SerialTransport가 주어졌을 때
When Open(TransportConfig{Type: "serial", Options: map[string]any{
    "port": "/dev/ttyUSB0",
    "baudRate": 9600,
    "parity": "none",
    "stopBits": 1,
    "dataBits": 8,
}})를 호출하면
Then 시리얼 포트가 지정된 설정으로 열려야 한다
And Available()이 true를 반환해야 한다
```

### AC-AGENT-001-32: SerialTransport 자동 재연결

```gherkin
Given 열린 SerialTransport에서 연결이 끊긴 경우
When 자동 재연결 로직이 동작하면
Then 지수 백오프 기반으로 재연결을 시도해야 한다
And 재연결 성공 시 Available()이 true를 반환해야 한다
```

---

## Module 8: TCP Transport

### AC-AGENT-001-33: TCPTransport 클라이언트 모드

```gherkin
Given TCPTransport가 클라이언트 모드로 주어졌을 때
When Open(TransportConfig{Type: "tcp", Options: map[string]any{
    "address": "127.0.0.1:9999",
    "mode": "client",
}})를 호출하면
Then 지정된 주소로 TCP 연결이 수립되어야 한다
And Available()이 true를 반환해야 한다
```

### AC-AGENT-001-34: TCPTransport 자동 재연결

```gherkin
Given 열린 TCPTransport에서 연결이 끊긴 경우
When 자동 재연결 로직이 동작하면
Then 지수 백오프 기반으로 재연결을 시도해야 한다
```

### AC-AGENT-001-35: TCPTransport 타임아웃

```gherkin
Given 연결 타임아웃 2초로 설정된 TCPTransport가 주어졌을 때
When 존재하지 않는 주소로 Open()을 호출하면
Then 2초 이내에 error를 반환해야 한다
```

---

## Module 9: UDP Transport

### AC-AGENT-001-36: UDPTransport 유니캐스트

```gherkin
Given UDPTransport가 주어졌을 때
When Open(TransportConfig{Type: "udp", Options: map[string]any{
    "address": "127.0.0.1:9998",
}})를 호출하면
Then UDP 소켓이 열려야 한다
And Write(data)로 데이터 전송이 가능해야 한다
```

---

## Module 10: Protocol Definition Engine

### AC-AGENT-001-37: Parser 기본 파싱

```gherkin
Given 다음 ProtocolDefinition이 주어졌을 때:
  - Header: [{"name": "cmd", "type": "uint8", "size": 1}]
  - Payload: [{"name": "value", "type": "uint16_be", "size": 2}]
  - Trailer: [{"name": "end", "type": "uint8", "size": 1}]
When Parse([]byte{0x01, 0x00, 0x64, 0xFF})를 호출하면
Then ParsedMessage.Header["cmd"]가 uint8(1)이어야 한다
And ParsedMessage.Payload["value"]가 uint16(100)이어야 한다
And ParsedMessage.Trailer["end"]가 uint8(255)여야 한다
```

### AC-AGENT-001-38: Parser 조건부 파싱

```gherkin
Given ProtocolDefinition에 ConditionalPayload가 정의되어 있고:
  - FieldName: "cmd"
  - Conditions: {"1": [{"name": "temp", "type": "float32"}], "2": [{"name": "status", "type": "uint8"}]}
When 헤더의 "cmd" 값이 1인 데이터로 Parse()를 호출하면
Then ParsedMessage.Payload에 "temp" float32 필드가 파싱되어야 한다

When 헤더의 "cmd" 값이 2인 데이터로 Parse()를 호출하면
Then ParsedMessage.Payload에 "status" uint8 필드가 파싱되어야 한다
```

### AC-AGENT-001-39: Parser Serialize/Parse 왕복 테스트

```gherkin
Given 유효한 ParsedMessage가 주어졌을 때
When Serialize(msg)로 바이트 데이터를 생성하고, Parse(bytes)로 다시 파싱하면
Then 원본 ParsedMessage와 동일한 필드 값이어야 한다
```

### AC-AGENT-001-40: Parser 체크섬 검증 실패

```gherkin
Given 체크섬이 포함된 ProtocolDefinition이 주어졌을 때
When 체크섬이 변조된 데이터로 Parse()를 호출하면
Then errors.Is(err, ErrChecksumMismatch)가 true여야 한다
And ParsedMessage.ChecksumValid가 false여야 한다
```

### AC-AGENT-001-41: Parser 불충분한 데이터 거부

```gherkin
Given 최소 4바이트를 요구하는 ProtocolDefinition이 주어졌을 때
When 2바이트 데이터로 Parse()를 호출하면
Then errors.Is(err, ErrProtocolParseError)가 true여야 한다
```

---

## Module 11: Protocol Checksum

### AC-AGENT-001-42: CRC-16 계산

```gherkin
Given 알려진 CRC-16 테스트 벡터 데이터가 주어졌을 때
When Calculate(ChecksumCRC16, data)를 호출하면
Then 예상 CRC-16 값과 일치해야 한다
```

### AC-AGENT-001-43: CRC-32 계산

```gherkin
Given 알려진 CRC-32 테스트 벡터 데이터가 주어졌을 때
When Calculate(ChecksumCRC32, data)를 호출하면
Then 예상 CRC-32 값과 일치해야 한다
```

### AC-AGENT-001-44: XOR 체크섬 계산

```gherkin
Given 데이터 []byte{0x01, 0x02, 0x03}이 주어졌을 때
When Calculate(ChecksumXOR, data)를 호출하면
Then 결과가 []byte{0x00}이어야 한다 (0x01 XOR 0x02 XOR 0x03 = 0x00)
```

### AC-AGENT-001-45: Modbus CRC 계산

```gherkin
Given 알려진 Modbus CRC 테스트 벡터 데이터가 주어졌을 때
When Calculate(ChecksumModbusCRC, data)를 호출하면
Then 예상 Modbus CRC 값과 일치해야 한다
```

### AC-AGENT-001-46: Verify 함수

```gherkin
Given Calculate()로 계산된 체크섬이 주어졌을 때
When Verify(algorithm, data, checksum)를 호출하면
Then true를 반환해야 한다

Given 변조된 체크섬이 주어졌을 때
When Verify(algorithm, data, wrongChecksum)를 호출하면
Then false를 반환해야 한다
```

---

## Module 12: Protocol Loader

### AC-AGENT-001-47: YAML 파일 로드

```gherkin
Given 유효한 프로토콜 정의 YAML 파일이 주어졌을 때
When LoadFromFile("protocol.yaml")를 호출하면
Then nil error와 함께 *ProtocolDefinition을 반환해야 한다
And ProtocolDefinition.Name이 YAML에 정의된 값과 일치해야 한다
```

### AC-AGENT-001-48: JSON 파일 로드

```gherkin
Given 유효한 프로토콜 정의 JSON 파일이 주어졌을 때
When LoadFromFile("protocol.json")를 호출하면
Then nil error와 함께 *ProtocolDefinition을 반환해야 한다
```

### AC-AGENT-001-49: 유효성 검증 실패

```gherkin
Given Name이 비어있는 프로토콜 정의가 주어졌을 때
When Validate(def)를 호출하면
Then errors.Is(err, ErrInvalidConfig)가 true여야 한다

Given 중복 필드 이름이 있는 프로토콜 정의가 주어졌을 때
When Validate(def)를 호출하면
Then errors.Is(err, ErrInvalidConfig)가 true여야 한다

Given 유효하지 않은 FieldType이 있는 프로토콜 정의가 주어졌을 때
When Validate(def)를 호출하면
Then errors.Is(err, ErrInvalidConfig)가 true여야 한다
```

---

## Module 13: Agent Configuration

### AC-AGENT-001-50: AgentConfig 유효성 검증 성공

```gherkin
Given 유효한 AgentConfig (ID: "a1", Name: "Agent1", HealthCheckInterval: 30s, MaxRestarts: 10, BufferSize: 1024)가 주어졌을 때
When Validate()를 호출하면
Then nil error를 반환해야 한다
```

### AC-AGENT-001-51: AgentConfig 유효성 검증 실패

```gherkin
Given ID가 빈 문자열인 AgentConfig가 주어졌을 때
When Validate()를 호출하면
Then error가 nil이 아니어야 한다

Given HealthCheckInterval이 0인 AgentConfig가 주어졌을 때
When Validate()를 호출하면
Then error가 nil이 아니어야 한다

Given BufferSize가 0인 AgentConfig가 주어졌을 때
When Validate()를 호출하면
Then error가 nil이 아니어야 한다
```

### AC-AGENT-001-52: 불변 설정 런타임 변경 거부

```gherkin
Given Running 상태인 Agent가 주어졌을 때
When Transport 타입을 변경하는 Configure(config)를 호출하면
Then errors.Is(err, ErrConfigImmutable)가 true여야 한다
```

---

## Module 14: System Agent Interface

### AC-AGENT-001-53: SystemAgent 인터페이스 준수

```gherkin
Given SystemAgent 인터페이스를 구현하는 구현체가 주어졌을 때
When IsSystem()를 호출하면
Then true를 반환해야 한다

When RequiresTransport()를 호출하면
Then false를 반환해야 한다
```

### AC-AGENT-001-54: SystemAgentType 열거

```gherkin
Given SystemAgentEvent, SystemAgentLogger, SystemAgentFile, SystemAgentTimer, SystemAgentStore가 주어졌을 때
Then 모든 타입이 유효한 문자열 값을 가져야 한다
And 중복되는 값이 없어야 한다
```

---

## Module 15: Agent Types Registry

### AC-AGENT-001-55: TypeRegistry 타입 등록 및 생성

```gherkin
Given DefaultTypeRegistry가 주어졌을 때
When RegisterType("test", testFactory)를 호출하고 CreateAgent("test", config)를 호출하면
Then nil error와 함께 Agent 인스턴스를 반환해야 한다
```

### AC-AGENT-001-56: TypeRegistry 기본 타입 자동 등록

```gherkin
Given 새로 생성된 DefaultTypeRegistry가 주어졌을 때
Then HasType("custom")이 true여야 한다
And HasType("event")가 true여야 한다
And HasType("logger")가 true여야 한다
And HasType("file")이 true여야 한다
And HasType("timer")가 true여야 한다
And HasType("store")가 true여야 한다
And ListTypes()에 최소 6개 타입이 포함되어야 한다
```

### AC-AGENT-001-57: TypeRegistry 중복 타입 등록 거부

```gherkin
Given "custom" 타입이 이미 등록된 TypeRegistry가 주어졌을 때
When RegisterType("custom", anotherFactory)를 호출하면
Then error가 nil이 아니어야 한다
```

---

## Module 16: Agent Info & Stats

### AC-AGENT-001-58: AgentInfo 종합 조회

```gherkin
Given Running 상태인 BaseAgent가 주어졌을 때
And Transport가 연결된 상태이고
And Protocol Definition이 로드된 상태이면
When Info()를 호출하면
Then AgentInfo.ID가 Agent.ID()와 일치해야 한다
And AgentInfo.Name이 Agent.Name()과 일치해야 한다
And AgentInfo.Type이 Agent.Type()과 일치해야 한다
And AgentInfo.State가 Running이어야 한다
And AgentInfo.Health가 Agent.Health()와 일치해야 한다
And AgentInfo.Transport.Connected가 true여야 한다
And AgentInfo.Protocol.Name이 로드된 프로토콜 이름과 일치해야 한다
And AgentInfo.Config가 현재 설정 스냅샷이어야 한다
And AgentInfo.Stats가 Agent.Stats()와 일치해야 한다
And AgentInfo.CreatedAt이 zero가 아니어야 한다
And AgentInfo.StartedAt이 zero가 아니어야 한다
And AgentInfo.Uptime이 0보다 커야 한다
```

### AC-AGENT-001-59: AgentStats 통계 증가

```gherkin
Given Running 상태인 BaseAgent가 주어졌을 때
And 초기 Stats().MessagesReceived가 0인 상태에서
When 메시지 3건을 수신 처리하면
Then Stats().MessagesReceived가 3이어야 한다
And Stats().BytesRead가 수신된 총 바이트 수와 일치해야 한다
And Stats().LastActivityAt이 마지막 수신 시각과 근사해야 한다
```

### AC-AGENT-001-60: AgentStats 재시작 보존

```gherkin
Given Running 상태에서 MessagesReceived가 100, BytesRead가 5000인 BaseAgent가 주어졌을 때
When Agent를 Stop 후 다시 Start하면 (재시작)
Then Stats().MessagesReceived가 100이어야 한다 (초기화되지 않음)
And Stats().BytesRead가 5000이어야 한다 (초기화되지 않음)
And Stats().RestartCount가 1 증가해야 한다
```

### AC-AGENT-001-61: ManagerSummary 집계

```gherkin
Given 3개의 Agent가 있는 Manager가 주어졌을 때:
  - agent-01: Running, Healthy, MessagesReceived=50
  - agent-02: Paused, Healthy, MessagesReceived=30
  - agent-03: Stopped, Unhealthy, MessagesErrored=5
When Summary()를 호출하면
Then ManagerSummary.TotalAgents가 3이어야 한다
And ManagerSummary.RunningAgents가 1이어야 한다
And ManagerSummary.PausedAgents가 1이어야 한다
And ManagerSummary.StoppedAgents가 1이어야 한다
And ManagerSummary.HealthyAgents가 2여야 한다
And ManagerSummary.UnhealthyAgents가 1이어야 한다
And ManagerSummary.TotalMessagesProcessed가 80이어야 한다 (50+30)
And ManagerSummary.TotalErrors가 5여야 한다
And ManagerSummary.Agents의 길이가 3이어야 한다
```

### AC-AGENT-001-62: TransportInfo 연결 정보

```gherkin
Given TCPTransport로 "127.0.0.1:9999"에 연결된 Agent가 주어졌을 때
When Info()를 호출하면
Then AgentInfo.Transport.Type이 "tcp"여야 한다
And AgentInfo.Transport.Connected가 true여야 한다
And AgentInfo.Transport.RemoteAddr이 "127.0.0.1:9999"여야 한다
And AgentInfo.Transport.ConnectedAt이 zero가 아니어야 한다

Given Transport가 연결 해제된 경우
When Info()를 호출하면
Then AgentInfo.Transport.Connected가 false여야 한다
```

### AC-AGENT-001-63: Stats 리셋

```gherkin
Given MessagesReceived=100, BytesRead=5000인 BaseAgent가 주어졌을 때
When ResetStats()를 호출하면
Then Stats().MessagesReceived가 0이어야 한다
And Stats().MessagesSent가 0이어야 한다
And Stats().MessagesErrored가 0이어야 한다
And Stats().BytesRead가 0이어야 한다
And Stats().BytesWritten이 0이어야 한다
And Stats().RestartCount가 0이어야 한다
And Stats().AvgProcessingLatency가 0이어야 한다
```

### AC-AGENT-001-64: Stats 동시성 안전

```gherkin
Given BaseAgent가 주어졌을 때
When 100개의 goroutine이 동시에 Stats 카운터를 업데이트하면 (MessagesReceived++, BytesRead+=n)
Then data race가 발생하지 않아야 한다 (go test -race 통과)
And 최종 MessagesReceived가 100이어야 한다
And Stats()로 읽은 값이 정확해야 한다
```

---

## Module 17: Error Types

### AC-AGENT-001-65: Sentinel 에러 errors.Is() 호환성

```gherkin
Given ErrAgentNotFound, ErrAgentAlreadyExists, ErrAgentNotRunning, ErrAgentAlreadyStopped,
    ErrTransportNotAvailable, ErrTransportClosed, ErrProtocolParseError, ErrChecksumMismatch,
    ErrHealthCheckFailed, ErrMaxRestartsExceeded, ErrInvalidConfig, ErrConfigImmutable,
    ErrInvalidStateTransition, ErrFlowAlreadyReferenced 각각에 대해
When errors.Is(err, sentinel)를 호출하면
Then 동일한 sentinel 에러와 비교 시 true를 반환해야 한다
And 다른 sentinel 에러와 비교 시 false를 반환해야 한다
```

### AC-AGENT-001-66: Sentinel 에러 fmt.Errorf 래핑 호환성

```gherkin
Given ErrAgentNotFound를 fmt.Errorf("wrapper: %w", ErrAgentNotFound)로 래핑했을 때
When errors.Is(wrappedErr, ErrAgentNotFound)를 호출하면
Then true를 반환해야 한다
```

---

## 품질 게이트

### QG-AGENT-001-01: 테스트 커버리지

```gherkin
Given internal/agent/ 패키지의 모든 소스 파일에 대해
When go test -cover ./internal/agent/...를 실행하면
Then 테스트 커버리지가 85% 이상이어야 한다
```

### QG-AGENT-001-02: Race Condition

```gherkin
Given internal/agent/ 패키지의 모든 테스트에 대해
When go test -race ./internal/agent/...를 실행하면
Then data race가 감지되지 않아야 한다
```

### QG-AGENT-001-03: 린트

```gherkin
Given internal/agent/ 패키지의 모든 소스 파일에 대해
When golangci-lint run ./internal/agent/...를 실행하면
Then 린트 에러가 0개여야 한다
```

### QG-AGENT-001-04: vet

```gherkin
Given internal/agent/ 패키지의 모든 소스 파일에 대해
When go vet ./internal/agent/...를 실행하면
Then 경고가 0개여야 한다
```
