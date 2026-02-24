---
id: SPEC-NASA-001
type: acceptance
version: "0.1.0"
created: "2026-02-24"
updated: "2026-02-24"
author: xtra
---

# SPEC-NASA-001 인수 기준: Samsung NASA Agent 구현

## 1. Module 1: NASAAgent Core (에이전트 코어)

### Scenario 1.1: 에이전트 생성 및 초기화

```gherkin
Given NASAConfig에 transport_type "serial", serial_port "/dev/ttyUSB0", device_addresses [1, 2, 3]이 설정된 경우
When NASAAgent.Init(config)이 호출되면
Then 에이전트 상태가 Running으로 전이되어야 한다
And devices 맵에 주소 1, 2, 3에 대한 NASADevice 엔트리가 생성되어야 한다
And 각 디바이스의 Online 상태가 false이어야 한다 (아직 폴링 전)
```

### Scenario 1.2: Agent 인터페이스 준수

```gherkin
Given NASAAgent 인스턴스가 생성된 경우
When agent.Agent 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ agent.Agent = (*NASAAgent)(nil))
```

### Scenario 1.3: MessageReceiver 인터페이스 준수

```gherkin
Given NASAAgent 인스턴스가 생성된 경우
When agent.MessageReceiver 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ agent.MessageReceiver = (*NASAAgent)(nil))
```

### Scenario 1.4: Stop 후 리소스 정리

```gherkin
Given NASAAgent가 Running 상태인 경우
When Stop(ctx)이 호출되면
Then 폴링 고루틴이 종료되어야 한다
And 트랜스포트 연결이 닫혀야 한다
And 에이전트 상태가 Stopped로 전이되어야 한다
```

### Scenario 1.5: Pause/Resume 동작

```gherkin
Given NASAAgent가 Running 상태인 경우
When Pause(ctx)가 호출되면
Then 폴링이 일시 중지되어야 한다
And 트랜스포트 연결은 유지되어야 한다
And 에이전트 상태가 Paused로 전이되어야 한다

When Resume(ctx)가 호출되면
Then 폴링이 재개되어야 한다
And 에이전트 상태가 Running으로 전이되어야 한다
```

---

## 2. Module 2: Transport Layer (트랜스포트 레이어)

### Scenario 2.1: Serial 트랜스포트 설정 파싱

```gherkin
Given transport_type이 "serial"이고 serial_port가 "/dev/ttyUSB0"인 경우
When NewNASATransport(config)가 호출되면
Then NASASerialTransport 인스턴스가 생성되어야 한다
And BaudRate 기본값 9600이 적용되어야 한다
And Parity 기본값 "even"이 적용되어야 한다
```

### Scenario 2.2: TCP 트랜스포트 설정 파싱

```gherkin
Given transport_type이 "tcp"이고 tcp_address가 "192.168.1.100:4196"인 경우
When NewNASATransport(config)가 호출되면
Then NASATCPTransport 인스턴스가 생성되어야 한다
And ConnectTimeout 기본값 5s가 적용되어야 한다
And ReadTimeout 기본값 3s가 적용되어야 한다
```

### Scenario 2.3: 유효하지 않은 트랜스포트 유형

```gherkin
Given transport_type이 "udp"인 경우
When NewNASATransport(config)가 호출되면
Then ErrInvalidTransportType 에러가 반환되어야 한다
```

### Scenario 2.4: Serial 포트 누락 에러

```gherkin
Given transport_type이 "serial"이고 serial_port가 빈 문자열인 경우
When parseNASAConfig(opts)가 호출되면
Then ErrSerialPortRequired 에러가 반환되어야 한다
```

### Scenario 2.5: TCP 주소 누락 에러

```gherkin
Given transport_type이 "tcp"이고 tcp_address가 빈 문자열인 경우
When parseNASAConfig(opts)가 호출되면
Then ErrTCPAddressRequired 에러가 반환되어야 한다
```

---

## 3. Module 3: NASA Protocol (프로토콜 처리)

### Scenario 3.1: 상태 조회 메시지 인코딩

```gherkin
Given NASAProtocol 인스턴스가 초기화된 경우
When BuildStatusQuery(addr=0x01)가 호출되면
Then 유효한 NASA 프로토콜 바이트 배열이 반환되어야 한다
And 첫 바이트는 SOF(시작 바이트)이어야 한다
And 마지막 바이트는 올바른 체크섬이어야 한다
And DestAddr 필드가 0x01이어야 한다
```

### Scenario 3.2: 응답 메시지 디코딩

```gherkin
Given 유효한 NASA 프로토콜 응답 바이트 배열이 제공된 경우
When Decode(data)가 호출되면
Then NASAMessage 구조체가 올바르게 파싱되어야 한다
And SourceAddr, DestAddr, CommandCode, Payload가 정확히 추출되어야 한다
And Checksum 검증이 통과해야 한다
```

### Scenario 3.3: 체크섬 불일치 거부

```gherkin
Given NASA 프로토콜 바이트 배열의 체크섬이 변조된 경우
When Decode(data)가 호출되면
Then ErrChecksumMismatch 에러가 반환되어야 한다
```

### Scenario 3.4: 불완전한 패킷 처리

```gherkin
Given SOF 바이트만 포함된 불완전한 바이트 배열이 제공된 경우
When Decode(data)가 호출되면
Then ErrProtocolParseFailed 에러가 반환되어야 한다
```

### Scenario 3.5: 제어 명령 인코딩

```gherkin
Given NASAProtocol 인스턴스가 초기화된 경우
When BuildControlCommand(addr=0x01, cmd=SetPowerOn)가 호출되면
Then 유효한 제어 명령 바이트 배열이 반환되어야 한다
And CommandCode가 전원 제어 코드와 일치해야 한다
And Payload에 전원 켜기 파라미터가 포함되어야 한다
```

---

## 4. Module 4: Device Management (디바이스 관리)

### Scenario 4.1: 디바이스 상태 폴링

```gherkin
Given NASAAgent가 Running 상태이고 device_addresses [1, 2]가 설정된 경우
When PollInterval이 도래하면
Then 주소 1과 2에 상태 조회 메시지가 전송되어야 한다
And 응답을 수신하면 해당 디바이스의 NASADeviceState가 업데이트되어야 한다
And LastSeen 타임스탬프가 갱신되어야 한다
```

### Scenario 4.2: 상태 변경 감지 및 알림

```gherkin
Given 디바이스 주소 1의 현재 모드가 "cool"인 경우
When 폴링 응답에서 모드가 "heat"로 변경된 것이 확인되면
Then NASADeviceState.Mode가 "heat"로 업데이트되어야 한다
And msgCh 채널에 JSON 형식의 상태 변경 메시지가 전달되어야 한다
And 메시지 type은 "device_state"이어야 한다
```

### Scenario 4.3: 디바이스 오프라인 감지

```gherkin
Given offline_threshold가 3이고 디바이스 주소 1이 온라인 상태인 경우
When 3회 연속 폴링에 응답이 없으면
Then 디바이스 Online 상태가 false로 변경되어야 한다
And msgCh 채널에 "device_offline" 타입의 이벤트 메시지가 전달되어야 한다
```

### Scenario 4.4: 오프라인 디바이스 복구

```gherkin
Given 디바이스 주소 1이 오프라인(Online=false) 상태인 경우
When 폴링에 다시 응답하면
Then 디바이스 Online 상태가 true로 복원되어야 한다
And ErrorCount가 0으로 초기화되어야 한다
And msgCh 채널에 "device_online" 타입의 이벤트 메시지가 전달되어야 한다
```

### Scenario 4.5: 디바이스 목록 조회

```gherkin
Given device_addresses [1, 2, 3]으로 초기화된 NASAAgent가 있는 경우
When ListDevices()가 호출되면
Then 3개의 NASADevice가 반환되어야 한다
And 각 디바이스의 Address가 1, 2, 3이어야 한다
```

### Scenario 4.6: 미등록 디바이스 상태 조회

```gherkin
Given device_addresses [1, 2]로 초기화된 NASAAgent가 있는 경우
When GetDeviceState(addr=99)가 호출되면
Then ErrDeviceNotFound 에러가 반환되어야 한다
```

---

## 5. Module 5: Control Commands (제어 명령)

### Scenario 5.1: 전원 제어 명령

```gherkin
Given 디바이스 주소 1이 온라인 상태인 경우
When Process({"command":"set_power","address":1,"params":{"power":true}})가 호출되면
Then 전원 켜기 NASA 프로토콜 메시지가 트랜스포트로 전송되어야 한다
And 응답 JSON에 "status":"ok"가 포함되어야 한다
```

### Scenario 5.2: 모드 설정 명령

```gherkin
Given 디바이스 주소 1이 온라인 상태인 경우
When Process({"command":"set_mode","address":1,"params":{"mode":"heat"}})가 호출되면
Then 모드 변경 NASA 프로토콜 메시지가 트랜스포트로 전송되어야 한다
And 응답 JSON에 "status":"ok"가 포함되어야 한다
```

### Scenario 5.3: 온도 설정 명령

```gherkin
Given 디바이스 주소 1이 온라인 상태인 경우
When Process({"command":"set_temperature","address":1,"params":{"temperature":24.0}})가 호출되면
Then 온도 설정 NASA 프로토콜 메시지가 트랜스포트로 전송되어야 한다
And 응답 JSON에 설정 결과가 포함되어야 한다
```

### Scenario 5.4: 유효하지 않은 모드 값 거부

```gherkin
Given 디바이스 주소 1이 온라인 상태인 경우
When Process({"command":"set_mode","address":1,"params":{"mode":"turbo_mode"}})가 호출되면
Then ErrInvalidMode 에러가 반환되어야 한다
And NASA 프로토콜 메시지가 트랜스포트로 전송되지 않아야 한다
```

### Scenario 5.5: 유효하지 않은 풍량 값 거부

```gherkin
Given 디바이스 주소 1이 온라인 상태인 경우
When Process({"command":"set_fan_speed","address":1,"params":{"fan_speed":"hurricane"}})가 호출되면
Then ErrInvalidFanSpeed 에러가 반환되어야 한다
```

### Scenario 5.6: 온도 범위 초과 거부

```gherkin
Given 디바이스 주소 1이 온라인 상태인 경우
When Process({"command":"set_temperature","address":1,"params":{"temperature":35.0}})가 호출되면
Then ErrTemperatureOutOfRange 에러가 반환되어야 한다

When Process({"command":"set_temperature","address":1,"params":{"temperature":10.0}})가 호출되면
Then ErrTemperatureOutOfRange 에러가 반환되어야 한다
```

### Scenario 5.7: 미등록 디바이스 제어 거부

```gherkin
Given device_addresses [1, 2]로 초기화된 NASAAgent가 있는 경우
When Process({"command":"set_power","address":99,"params":{"power":true}})가 호출되면
Then ErrDeviceNotFound 에러가 반환되어야 한다
```

### Scenario 5.8: 오프라인 디바이스 제어 거부

```gherkin
Given 디바이스 주소 1이 오프라인(Online=false) 상태인 경우
When Process({"command":"set_power","address":1,"params":{"power":true}})가 호출되면
Then ErrDeviceOffline 에러가 반환되어야 한다
```

---

## 6. Module 6: Bridge Integration (Bridge 연동)

### Scenario 6.1: ReceiveMessage 채널 기반 수신

```gherkin
Given NASAAgent가 Running 상태이고 msgCh에 메시지가 있는 경우
When ReceiveMessage(ctx)가 호출되면
Then msgCh에서 JSON 바이트 메시지가 반환되어야 한다
And 메시지는 유효한 JSON 형식이어야 한다
```

### Scenario 6.2: ReceiveMessage 컨텍스트 취소

```gherkin
Given NASAAgent가 Running 상태이고 msgCh가 비어있는 경우
When 취소된 context로 ReceiveMessage(ctx)가 호출되면
Then context.Canceled 에러가 즉시 반환되어야 한다
And 블로킹 없이 반환되어야 한다
```

### Scenario 6.3: 상태 변경 메시지 JSON 포맷

```gherkin
Given 디바이스 상태 변경이 감지된 경우
When msgCh에 메시지가 전달되면
Then 메시지는 "type", "address", "device_type", "online", "state", "timestamp" 필드를 포함해야 한다
And "state" 객체는 "power", "mode", "target_temp", "current_temp", "fan_speed", "error_code" 필드를 포함해야 한다
And "timestamp"는 RFC 3339 형식이어야 한다
```

---

## 7. Module 7: Error Handling (에러 처리)

### Scenario 7.1: 센티널 에러 정의 확인

```gherkin
Given samsung 패키지가 로드된 경우
When 12개의 센티널 에러 변수를 확인하면
Then 모든 에러가 정의되어 있어야 한다
And 각 에러는 errors.Is()로 비교 가능해야 한다
And 각 에러는 고유한 에러 메시지를 가져야 한다
```

### Scenario 7.2: errors.Is() 호환성

```gherkin
Given fmt.Errorf("%w: detail", ErrDeviceNotFound) 형태로 래핑된 에러가 있는 경우
When errors.Is(err, ErrDeviceNotFound)로 검사하면
Then true가 반환되어야 한다
```

---

## 8. Module 8: TypeRegistry Registration (타입 등록)

### Scenario 8.1: 에이전트 타입 등록

```gherkin
Given 빈 TypeRegistry가 있는 경우
When RegisterSamsungNASATypes(registry)가 호출되면
Then registry.HasType("samsung-nasa")가 true를 반환해야 한다
And registry.ListTypes()에 "samsung-nasa"가 포함되어야 한다
```

### Scenario 8.2: 팩토리를 통한 에이전트 생성

```gherkin
Given "samsung-nasa" 타입이 등록된 TypeRegistry가 있는 경우
When registry.CreateAgent("samsung-nasa", validConfig)가 호출되면
Then NASAAgent 인스턴스가 반환되어야 한다
And 에이전트 Type()이 "samsung-nasa"를 반환해야 한다
```

### Scenario 8.3: 중복 등록 방지

```gherkin
Given "samsung-nasa" 타입이 이미 등록된 TypeRegistry가 있는 경우
When RegisterSamsungNASATypes(registry)가 다시 호출되면
Then 에러가 반환되어야 한다
```

---

## 9. 에지 케이스 테스트

### Scenario 9.1: 빈 device_addresses

```gherkin
Given device_addresses가 빈 배열 []인 경우
When parseNASAConfig(opts)가 호출되면
Then 에러가 반환되어야 한다 (최소 1개 디바이스 주소 필요)
```

### Scenario 9.2: 중복 디바이스 주소

```gherkin
Given device_addresses가 [1, 2, 1, 3]인 경우
When parseNASAConfig(opts)가 호출되면
Then 중복이 제거되어 [1, 2, 3]으로 처리되어야 한다
```

### Scenario 9.3: 디바이스 주소 범위 초과

```gherkin
Given device_addresses에 256이 포함된 경우
When parseNASAConfig(opts)가 호출되면
Then 에러가 반환되어야 한다 (주소 범위: 0~255)
```

### Scenario 9.4: PollInterval 최소값 검증

```gherkin
Given poll_interval이 "100ms"로 설정된 경우
When parseNASAConfig(opts)가 호출되면
Then 에러가 반환되어야 한다 (최소 1s 이상)
```

### Scenario 9.5: 동시 Process 호출

```gherkin
Given NASAAgent가 Running 상태인 경우
When 2개의 고루틴에서 동시에 Process()가 호출되면
Then 데이터 레이스 없이 순차적으로 처리되어야 한다
And go test -race에서 에러가 발생하지 않아야 한다
```

### Scenario 9.6: 유효하지 않은 JSON 명령

```gherkin
Given NASAAgent가 Running 상태인 경우
When Process([]byte("invalid json"))가 호출되면
Then ErrInvalidCommand 에러가 반환되어야 한다
```

---

## 10. 성능 기준

### Scenario 10.1: 폴링 지연시간

```gherkin
Given 5대의 디바이스가 등록된 NASAAgent가 있는 경우
When 전체 디바이스 폴링 사이클이 실행되면
Then 모든 디바이스 상태 조회 및 응답 처리가 PollInterval 내에 완료되어야 한다
```

### Scenario 10.2: 메시지 처리 지연

```gherkin
Given NASAAgent가 Running 상태인 경우
When Process()를 통해 제어 명령이 전달되면
Then 명령 인코딩 + 전송 + 응답 수신 + 디코딩이 ReadTimeout 내에 완료되어야 한다
```

### Scenario 10.3: 메시지 채널 처리량

```gherkin
Given msg_channel_size가 256으로 설정된 경우
When 256개의 상태 변경 이벤트가 연속 발생하면
Then 채널이 가득 차더라도 폴링 고루틴이 블로킹되지 않아야 한다 (비블로킹 전송 또는 드롭)
And 채널 오버플로우 시 로그에 경고가 기록되어야 한다
```

---

## 11. Quality Gate 기준

### 11.1 테스트 커버리지

| 파일 | 최소 커버리지 |
|------|-------------|
| `errors.go` | 100% |
| `config.go` | 90%+ |
| `device.go` | 85%+ |
| `message.go` | 85%+ |
| `protocol.go` | 85%+ |
| `transport.go` | 80%+ (목 기반) |
| `agent.go` | 85%+ |
| `register.go` | 100% |
| **전체 패키지** | **85%+** |

### 11.2 코드 품질

- `go vet ./internal/agent/samsung/...` 경고 0건
- `go test -race ./internal/agent/samsung/...` 데이터 레이스 0건
- 모든 exported 타입 및 함수에 GoDoc 주석 포함
- 센티널 에러는 `errors.Is()` 호환

### 11.3 Definition of Done

- [ ] 모든 요구사항(REQ-NASA-001-*)에 대한 테스트 시나리오 존재
- [ ] `go test -race -cover ./internal/agent/samsung/...` 통과 (85%+ 커버리지)
- [ ] `go vet ./internal/agent/samsung/...` 경고 0건
- [ ] TypeRegistry에 "samsung-nasa" 타입 등록 완료
- [ ] cmd/xflowd/main.go에서 RegisterSamsungNASATypes 호출 추가
- [ ] 예제 설정 YAML 파일 작성
- [ ] SPEC-NASA-001 문서와 구현 코드 간 추적성(traceability) 확인

---

*SPEC-NASA-001 Acceptance v0.1.0*
*작성자: xtra*
*날짜: 2026-02-24*
