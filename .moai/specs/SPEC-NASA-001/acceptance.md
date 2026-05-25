---
id: SPEC-NASA-001
type: acceptance
version: "1.3.0"
created: "2026-02-24"
updated: "2026-03-12"
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
When Process({"command":"target_temperature","address":1,"params":{"temperature":24.0}})가 호출되면
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
When Process({"command":"target_temperature","address":1,"params":{"temperature":35.0}})가 호출되면
Then ErrTemperatureOutOfRange 에러가 반환되어야 한다

When Process({"command":"target_temperature","address":1,"params":{"temperature":10.0}})가 호출되면
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

## 11. Module 9: 설정 확장 (Config Extensions)

### Scenario 11.1: unsupported_msg_sets 16진수 파싱

```gherkin
Given NASAConfig에 unsupported_msg_sets: [0x4100, 0x4102, 0x4111]이 설정된 경우
When parseNASAConfig(opts)가 호출되면
Then cfg.UnsupportedMsgSets 맵에 0x4100, 0x4102, 0x4111 키가 포함되어야 한다
And 맵 크기가 3이어야 한다
```

**검증 테스트**: `TestParseNASAConfig_UnsupportedMsgSets` (config_test.go)

### Scenario 11.2: unsupported_msg_sets 빈 목록

```gherkin
Given NASAConfig에 unsupported_msg_sets가 설정되지 않은 경우
When parseNASAConfig(opts)가 호출되면
Then cfg.UnsupportedMsgSets가 nil이어야 한다
```

**검증 테스트**: `TestParseNASAConfig_UnsupportedMsgSets_Empty` (config_test.go)

### Scenario 11.3: unsupported_msg_sets JSON float64 역직렬화 호환성

```gherkin
Given JSON 역직렬화 후 unsupported_msg_sets 값이 float64 타입인 경우 (예: 1544.0 = 0x0608)
When parseNASAConfig(opts)가 호출되면
Then float64 값이 올바른 uint16 키로 변환되어야 한다
And cfg.UnsupportedMsgSets[0x0608]이 true를 반환해야 한다
```

**검증 테스트**: `TestParseNASAConfig_UnsupportedMsgSets_JSONRoundTrip` (config_test.go)

### Scenario 11.4: log_unsupported_msg_sets 파싱

```gherkin
Given NASAConfig에 log_unsupported_msg_sets: true가 설정된 경우
When parseNASAConfig(opts)가 호출되면
Then cfg.LogUnsupportedMsgSets가 true이어야 한다

Given log_unsupported_msg_sets가 설정되지 않은 경우
When parseNASAConfig(opts)가 호출되면
Then cfg.LogUnsupportedMsgSets 기본값이 false이어야 한다
```

### Scenario 11.5: include_raw_message_sets 파싱

```gherkin
Given NASAConfig에 include_raw_message_sets: false가 설정된 경우
When parseNASAConfig(opts)가 호출되면
Then cfg.IncludeRawMessageSets가 false이어야 한다

Given include_raw_message_sets가 설정되지 않은 경우
When parseNASAConfig(opts)가 호출되면
Then cfg.IncludeRawMessageSets 기본값이 true이어야 한다
```

---

## 12. Module 10: 메시지 셋 필터링 (Message Set Filtering)

### Scenario 12.1: unsupported 메시지 셋 필터링

```gherkin
Given NASAAgent의 unsupported_msg_sets에 0x4100이 포함된 경우
When 수신된 NASAMessageSet 목록에 인덱스 0x4100이 포함되어 있으면
Then filterMessageSets()가 해당 메시지 셋을 제거해야 한다
And UpdateFromMessageSets에 전달되는 목록에 0x4100이 포함되지 않아야 한다
```

**검증 테스트**: `TestFilterMessageSets` (agent_test.go)

### Scenario 12.2: supported 메시지 셋 보존

```gherkin
Given NASAAgent의 unsupported_msg_sets에 0x4100만 포함된 경우
When 수신된 NASAMessageSet 목록에 인덱스 0x4000, 0x4100, 0x4200이 포함되어 있으면
Then filterMessageSets()가 0x4000, 0x4200을 보존해야 한다
And 반환된 목록의 크기가 2이어야 한다
```

**검증 테스트**: `TestFilterMessageSets` (agent_test.go)

### Scenario 12.3: log_unsupported_msg_sets 로그 출력

```gherkin
Given NASAAgent의 log_unsupported_msg_sets가 true이고 unsupported_msg_sets에 0x4100이 포함된 경우
When filterMessageSets()가 0x4100을 필터링하면
Then debug 수준의 로그가 출력되어야 한다
And 로그에 필터링된 메시지 셋 인덱스와 소스 주소가 포함되어야 한다
```

### Scenario 12.4: unsupported_msg_sets 빈 목록 시 필터링 건너뜀

```gherkin
Given NASAAgent의 unsupported_msg_sets가 nil 또는 빈 맵인 경우
When filterMessageSets()가 호출되면
Then 입력된 메시지 셋 목록이 그대로 반환되어야 한다
And 필터링 로직이 건너뛰어져야 한다
```

**검증 테스트**: `TestFilterMessageSets` (agent_test.go)

---

## 13. Module 11: HexKeyByteMap JSON 직렬화

### Scenario 13.1: MarshalJSON 16진수 키 출력

```gherkin
Given HexKeyByteMap에 uint16 키 0x0402와 값 []byte{0x01, 0x02}가 포함된 경우
When MarshalJSON()이 호출되면
Then JSON 출력에서 키가 "0x0402" 형식의 16진수 문자열이어야 한다
And 값이 올바른 바이트 배열로 직렬화되어야 한다
```

**검증 테스트**: `TestHexKeyByteMap_MarshalJSON` (device_test.go)

### Scenario 13.2: UnmarshalJSON 16진수 키 파싱

```gherkin
Given JSON 문자열에 "0x0402" 형식의 16진수 키가 포함된 경우
When UnmarshalJSON()이 호출되면
Then HexKeyByteMap에 uint16 키 0x0402가 생성되어야 한다
And 값이 올바르게 역직렬화되어야 한다
```

**검증 테스트**: `TestHexKeyByteMap_UnmarshalJSON_Hex` (device_test.go)

### Scenario 13.3: UnmarshalJSON 10진수 키 파싱

```gherkin
Given JSON 문자열에 "1026" 형식의 10진수 키가 포함된 경우
When UnmarshalJSON()이 호출되면
Then HexKeyByteMap에 uint16 키 1026 (= 0x0402)이 생성되어야 한다
And 16진수 키와 동일한 결과를 반환해야 한다
```

**검증 테스트**: `TestHexKeyByteMap_UnmarshalJSON_Decimal` (device_test.go)

### Scenario 13.4: 빈 HexKeyByteMap 직렬화/역직렬화

```gherkin
Given HexKeyByteMap이 빈 맵인 경우
When MarshalJSON()이 호출되면
Then 빈 JSON 객체 "{}"가 반환되어야 한다

Given 빈 JSON 객체 "{}"가 제공된 경우
When UnmarshalJSON()이 호출되면
Then 빈 HexKeyByteMap이 생성되어야 한다
```

---

## 14. Module 12: StateForJSON 조건부 출력

### Scenario 14.1: includeRaw=true 시 RawMessageSets 포함

```gherkin
Given NASADeviceState에 RawMessageSets 데이터가 존재하는 경우
When StateForJSON(true)가 호출되면
Then 반환된 맵에 "raw_message_sets" 필드가 포함되어야 한다
And RawMessageSets의 HexKeyByteMap 데이터가 정확히 포함되어야 한다
```

**검증 테스트**: `TestStateForJSON_IncludeRaw` (device_test.go)

### Scenario 14.2: includeRaw=false 시 RawMessageSets 제외

```gherkin
Given NASADeviceState에 RawMessageSets 데이터가 존재하는 경우
When StateForJSON(false)가 호출되면
Then 반환된 맵에 "raw_message_sets" 필드가 포함되지 않아야 한다
And 나머지 상태 필드(power, mode, target_temp 등)는 정상 포함되어야 한다
```

**검증 테스트**: `TestStateForJSON_ExcludeRaw` (device_test.go)

### Scenario 14.3: include_raw_message_sets 설정과 연동

```gherkin
Given NASAConfig의 include_raw_message_sets가 false로 설정된 경우
When 에이전트가 get_all_states 또는 get_state 응답을 생성하면
Then StateForJSON(false)가 호출되어야 한다
And 응답 JSON에 raw_message_sets가 제외되어야 한다

Given NASAConfig의 include_raw_message_sets가 true (기본값)로 설정된 경우
When 에이전트가 상태 응답을 생성하면
Then StateForJSON(true)가 호출되어야 한다
And 응답 JSON에 raw_message_sets가 포함되어야 한다
```

---

## 15. Module 13: Bridge CommandPollAdapter 통합 (NASAAdapter)

### Scenario 15.1: NASAAdapter 인터페이스 구현

```gherkin
Given NASAAdapter 인스턴스가 생성된 경우
When node.BridgeAdapter 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ node.BridgeAdapter = (*NASAAdapter)(nil))

When node.CommandPollAdapter 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ node.CommandPollAdapter = (*NASAAdapter)(nil))
```

**검증 테스트**: nasa.go 컴파일 타임 인터페이스 검증

### Scenario 15.2: TransformToFlow 유효한 JSON 변환

```gherkin
Given 유효한 JSON 바이트 데이터와 AgentMeta가 제공된 경우
When TransformToFlow(data, meta)가 호출되면
Then flow.Message가 반환되어야 한다
And 메시지 Payload에 "agent_type": "samsung-nasa"가 포함되어야 한다
And JSON 데이터가 "data" 필드에 파싱되어 포함되어야 한다
```

**검증 테스트**: `TestNASAAdapter_TransformToFlow_JSON` (nasa_test.go)

### Scenario 15.3: TransformToFlow 비-JSON 데이터 (raw fallback)

```gherkin
Given 유효하지 않은 JSON 바이트 데이터가 제공된 경우
When TransformToFlow(data, meta)가 호출되면
Then flow.Message가 반환되어야 한다
And 원본 바이트가 "raw" 필드에 문자열로 포함되어야 한다
And 에러가 반환되지 않아야 한다
```

**검증 테스트**: `TestNASAAdapter_TransformToFlow_RawData` (nasa_test.go)

### Scenario 15.4: TransformToAgent 변환

```gherkin
Given flow.Message에 제어 명령 페이로드가 포함된 경우
When TransformToAgent(msg)가 호출되면
Then 에이전트가 처리할 수 있는 바이트 데이터가 반환되어야 한다
And AgentMeta.AgentType이 "samsung-nasa"이어야 한다
```

**검증 테스트**: `TestNASAAdapter_TransformToAgent` (nasa_test.go)

### Scenario 15.5: PollCommand 명령 반환

```gherkin
Given NASAAdapter 인스턴스가 생성된 경우
When PollCommand()가 호출되면
Then JSON 바이트가 반환되어야 한다
And JSON에 "command": "get_all_states"가 포함되어야 한다
```

**검증 테스트**: `TestNASAAdapter_PollCommand` (nasa_test.go)

### Scenario 15.6: AssemblePollMessage JSON 응답 변환

```gherkin
Given 에이전트로부터 유효한 JSON 폴링 응답이 수신된 경우
When AssemblePollMessage(response)가 호출되면
Then flow.Message가 반환되어야 한다
And 폴링 응답 데이터가 flow 메시지의 Payload에 포함되어야 한다
```

**검증 테스트**: `TestNASAAdapter_AssemblePollMessage` (nasa_test.go)

### Scenario 15.7: AssemblePollMessage 유효하지 않은 JSON 에러

```gherkin
Given 유효하지 않은 JSON 바이트가 폴링 응답으로 수신된 경우
When AssemblePollMessage(response)가 호출되면
Then 에러가 반환되어야 한다
```

**검증 테스트**: `TestNASAAdapter_AssemblePollMessage_InvalidJSON` (nasa_test.go)

### Scenario 15.8: 어댑터 레지스트리 등록

```gherkin
Given 어댑터 레지스트리에 init()이 실행된 경우
When "samsung-nasa" 키로 어댑터를 조회하면
Then NASAAdapter 인스턴스가 반환되어야 한다
```

**검증 코드**: `register.go` - `node.RegisterAdapter("samsung-nasa", NewNASAAdapter())`

---

## 17. Module 14: Transport 재연결 (v1.2.0)

### Scenario 17.1: 시작 시 연결 실패 시 재연결 루프 진입 (AC-1)

```gherkin
Given Agent의 Init()이 완료되고 transport가 사용 불가능한 상태인 경우
When Start(ctx)가 호출되면
Then Start()는 nil을 반환해야 한다 (에러 반환 금지)
And WARN 레벨 로그가 1회 출력되어야 한다
And reconnectLoop 고루틴이 시작되어야 한다
And transport_reconnecting 이벤트가 msgCh에 전달되어야 한다
And reconnecting 플래그가 true이어야 한다
```

**요구사항**: REQ-NASA-001-01-05, REQ-NASA-001-01-09, REQ-NASA-001-07-03

### Scenario 17.2: 재연결 성공 시 정상 운영 재개 (AC-2)

```gherkin
Given Agent가 reconnectLoop 상태이고 3번째 시도에서 transport가 사용 가능해진 경우
When transport.Open()이 성공하면
Then INFO 레벨 "재연결 성공" 로그가 출력되어야 한다
And pollLoop와 receiveLoop 고루틴이 시작되어야 한다
And transport_reconnected 이벤트가 msgCh에 전달되어야 한다
And 이벤트에 attempt_count=3이 포함되어야 한다
And 이벤트에 downtime_seconds가 0보다 큰 값으로 포함되어야 한다
And reconnecting 플래그가 false로 리셋되어야 한다
And 백오프 카운터가 리셋되어야 한다
```

**요구사항**: REQ-NASA-001-01-09, REQ-NASA-001-07-03

### Scenario 17.3: 재연결 시도 시 에러 로그 억제 (AC-3)

```gherkin
Given Agent가 reconnectLoop 상태이고 transport가 계속 사용 불가능한 경우
When 2번째 이후 재연결 시도가 실패하면
Then WARN 또는 ERROR 레벨 로그가 출력되지 않아야 한다
And DEBUG 레벨 로그만 출력되어야 한다
```

**요구사항**: REQ-NASA-001-01-09

### Scenario 17.4: 운영 중 연결 끊김 시 재연결 루프 진입 (AC-4)

```gherkin
Given Agent가 정상 운영 중 (pollLoop, receiveLoop 활성) 상태인 경우
When receiveLoop에서 연결 끊김이 감지되면 (transport.Receive() 실패 + Available()==false)
Then transport_disconnected 이벤트가 msgCh에 전달되어야 한다
And receiveLoop가 종료되어야 한다
And disconnectCh를 통해 pollLoop에 중지 신호가 전달되어야 한다
And pollLoop가 종료되어야 한다
And reconnectLoop 고루틴이 시작되어야 한다
```

**요구사항**: REQ-NASA-001-01-10, REQ-NASA-001-01-11, REQ-NASA-001-07-03

### Scenario 17.5: 지수 백오프 적용 (AC-5)

```gherkin
Given reconnect_interval="5s"이고 max_reconnect_backoff="5m"인 경우
When 반복적인 재연결 실패가 발생하면
Then 재연결 간격이 다음 순서로 증가해야 한다: 5s -> 10s -> 20s -> 40s -> 80s -> 160s -> 300s(cap)
And max_reconnect_backoff(5m=300s)를 초과하지 않아야 한다
```

**요구사항**: REQ-NASA-001-01-09

### Scenario 17.6: Stop() 호출 시 재연결 루프 종료 (AC-6)

```gherkin
Given Agent가 reconnectLoop 상태인 경우
When Stop(ctx)가 호출되면
Then reconnectLoop가 즉시 종료되어야 한다
And 모든 리소스가 정리되어야 한다
And 에이전트 상태가 Stopped로 전이되어야 한다
```

**요구사항**: REQ-NASA-001-01-09, REQ-NASA-001-01-06

### Scenario 17.7: Available() 상태 정확성 (AC-7)

```gherkin
Given TCP transport가 연결된 상태인 경우
When Receive()에서 io.EOF 에러가 발생하면
Then Available()이 false를 반환해야 한다

Given TCP transport가 연결된 상태인 경우
When Receive()에서 타임아웃 에러가 발생하면 (net.Error.Timeout()==true)
Then Available()이 여전히 true를 반환해야 한다 (타임아웃은 연결 끊김이 아님)
```

**요구사항**: REQ-NASA-001-02-01, REQ-NASA-001-02-05

### Scenario 17.8: 초기 연결 실패와 운영 중 끊김의 동일 동작 (AC-8)

```gherkin
Given 두 가지 시나리오가 있는 경우:
  (A) Start() 시 transport.Open() 실패
  (B) 운영 중 receiveLoop에서 연결 끊김 감지
When 각각 reconnectLoop에 진입하면
Then 동일한 reconnectLoop 로직이 사용되어야 한다
And 동일한 지수 백오프가 적용되어야 한다
And 동일한 로그 억제 정책이 적용되어야 한다 (첫 시도만 WARN, 이후 DEBUG)
And 재연결 성공 시 동일하게 pollLoop/receiveLoop가 시작되어야 한다
```

**요구사항**: REQ-NASA-001-01-05, REQ-NASA-001-01-09, REQ-NASA-001-01-10

---

## 18. Module 15: NASAStatusNode (v1.3.0)

### Scenario 18.1: NASAStatusNode 인터페이스 구현

```gherkin
Given NASAStatusNode 인스턴스가 생성된 경우
When Node 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ Node = (*NASAStatusNode)(nil))

When SourceNode 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ SourceNode = (*NASAStatusNode)(nil))
```

**요구사항**: REQ-NASA-001-09-21

### Scenario 18.2: NASAStatusNode Configure — agent_ref 필수

```gherkin
Given agent_ref가 빈 문자열인 config가 제공된 경우
When Configure(config)가 호출되면
Then ErrNASAMissingAgentRef 에러가 반환되어야 한다
```

**요구사항**: REQ-NASA-001-09-03

### Scenario 18.3: NASAStatusNode Configure — 기본값 적용

```gherkin
Given agent_ref만 설정된 최소 config가 제공된 경우
When Configure(config)가 호출되면
Then timeout 기본값 "5s"가 적용되어야 한다
And poll_interval 기본값 "30s"가 적용되어야 한다
And include_raw 기본값 false가 적용되어야 한다
```

**요구사항**: REQ-NASA-001-09-01, REQ-NASA-001-09-03

### Scenario 18.4: NASAStatusNode Init — 비-NASA Agent 거부

```gherkin
Given AgentResolver가 MODBUS Agent를 반환하도록 설정된 경우
When Init(ctx)가 호출되면
Then ErrNASAAgentNotNASA 에러가 반환되어야 한다
```

**요구사항**: REQ-NASA-001-09-04

### Scenario 18.5: NASAStatusNode Init — AgentResolver 미설정

```gherkin
Given AgentResolver가 nil인 경우
When Init(ctx)가 호출되면
Then ErrNASANoResolver 에러가 반환되어야 한다
```

**요구사항**: REQ-NASA-001-09-04

### Scenario 18.6: NASAStatusNode Init — 정상 초기화

```gherkin
Given AgentResolver가 *samsung.NASAAgent를 반환하도록 설정된 경우
When Init(ctx)가 호출되면
Then 에러 없이 성공해야 한다
And 노드 상태가 Running으로 전이되어야 한다
And agent 필드에 *samsung.NASAAgent가 저장되어야 한다
```

**요구사항**: REQ-NASA-001-09-04

### Scenario 18.7: NASAStatusNode Process — device_id로 상태 조회

```gherkin
Given NASAStatusNode가 초기화되고 device_id가 "living-room"으로 설정된 경우
When Process(ctx, msg)가 호출되면 (msg.Payload는 빈 상태)
Then Agent에 {"command":"get_state","device_id":"living-room"} JSON이 전달되어야 한다
And 출력 메시지 Payload에 상태 응답이 포함되어야 한다
And 메타데이터에 nasa.source="node", nasa.node_type="nasa-status"가 설정되어야 한다
```

**요구사항**: REQ-NASA-001-09-05

### Scenario 18.8: NASAStatusNode Process — 전체 상태 조회

```gherkin
Given NASAStatusNode가 초기화되고 device_address/device_id가 모두 비어있는 경우
When Process(ctx, msg)가 호출되면
Then Agent에 {"command":"get_all_states"} JSON이 전달되어야 한다
```

**요구사항**: REQ-NASA-001-09-05

### Scenario 18.9: NASAStatusNode Process — 런타임 오버라이드

```gherkin
Given NASAStatusNode가 device_id="living-room"으로 설정된 경우
When msg.Payload에 "device_id": "bedroom-1"이 포함된 메시지로 Process가 호출되면
Then Agent에 {"command":"get_state","device_id":"bedroom-1"} JSON이 전달되어야 한다 (오버라이드 적용)
```

**요구사항**: REQ-NASA-001-09-20

### Scenario 18.10: NASAStatusNode SourceNode 폴링

```gherkin
Given NASAStatusNode가 poll_interval="100ms"로 초기화된 경우
When 200ms 경과 후 SourceCh()에서 읽으면
Then 최소 1개의 상태 메시지가 수신되어야 한다
And 메시지 Payload에 Agent 상태 응답이 포함되어야 한다
```

**요구사항**: REQ-NASA-001-09-06

### Scenario 18.11: NASAStatusNode Shutdown — 폴링 중지

```gherkin
Given NASAStatusNode가 SourceNode 폴링 중인 경우
When Shutdown(ctx)가 호출되면
Then 폴링 고루틴이 종료되어야 한다
And SourceCh() 채널이 더 이상 메시지를 수신하지 않아야 한다
And 노드 상태가 Stopping으로 전이되어야 한다
```

**요구사항**: REQ-NASA-001-09-19

---

## 19. Module 16: NASAControlNode (v1.3.0)

### Scenario 19.1: NASAControlNode 인터페이스 구현

```gherkin
Given NASAControlNode 인스턴스가 생성된 경우
When Node 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ Node = (*NASAControlNode)(nil))

When SourceNode 인터페이스로 캐스팅하면
Then NASAControlNode는 SourceNode를 구현하지 않아야 한다
```

**요구사항**: REQ-NASA-001-09-07, REQ-NASA-001-09-21

### Scenario 19.2: NASAControlNode Process — 직접 명령 형식 (set_power)

```gherkin
Given NASAControlNode가 초기화되고 device_id="living-room"으로 설정된 경우
When msg.Payload에 {"command":"set_power","params":{"power":true}}가 포함된 메시지로 Process가 호출되면
Then Agent에 {"command":"set_power","device_id":"living-room","params":{"power":true}} JSON이 전달되어야 한다
And 출력 메시지에 제어 응답이 포함되어야 한다
And 메타데이터에 nasa.node_type="nasa-control"이 설정되어야 한다
```

**요구사항**: REQ-NASA-001-09-10

### Scenario 19.3: NASAControlNode Process — 간소화 형식 (자동 set_multiple 변환)

```gherkin
Given NASAControlNode가 초기화되고 device_id="living-room"으로 설정된 경우
When msg.Payload에 {"power":true,"mode":"cool","target_temp":24.0}가 포함된 메시지로 Process가 호출되면
Then Agent에 {"command":"set_multiple","device_id":"living-room","params":{"power":true,"mode":"cool","target_temp":24.0}} JSON이 전달되어야 한다
```

**요구사항**: REQ-NASA-001-09-11

### Scenario 19.4: NASAControlNode Process — device_address 런타임 오버라이드

```gherkin
Given NASAControlNode가 device_id="living-room"으로 설정된 경우
When msg.Payload에 {"command":"set_power","device_address":"200001","params":{"power":false}}가 포함된 메시지로 Process가 호출되면
Then Agent에 device_address="200001"이 전달되어야 한다 (노드 설정의 device_id 대신 오버라이드)
```

**요구사항**: REQ-NASA-001-09-20

### Scenario 19.5: NASAControlNode Process — set_multiple 복합 제어

```gherkin
Given NASAControlNode가 초기화된 경우
When msg.Payload에 {"command":"set_multiple","device_id":"bedroom-1","params":{"power":true,"mode":"heat","target_temp":22.0,"fan_speed":"low"}}가 포함된 메시지로 Process가 호출되면
Then Agent에 동일한 set_multiple 명령이 전달되어야 한다
And 출력 메시지에 제어 응답이 포함되어야 한다
```

**요구사항**: REQ-NASA-001-09-10

### Scenario 19.6: NASAControlNode Process — 타임아웃

```gherkin
Given NASAControlNode가 timeout="50ms"로 초기화되고 Agent Process()가 100ms 이상 지연되는 경우
When Process(ctx, msg)가 호출되면
Then context.DeadlineExceeded를 래핑한 에러가 반환되어야 한다
```

**요구사항**: REQ-NASA-001-09-18

---

## 20. Module 17: NASANode 복합 (v1.3.0)

### Scenario 20.1: NASANode 인터페이스 구현

```gherkin
Given NASANode 인스턴스가 생성된 경우
When Node 인터페이스로 캐스팅하면
Then 컴파일 타임에 성공해야 한다 (var _ Node = (*NASANode)(nil))
```

**요구사항**: REQ-NASA-001-09-21

### Scenario 20.2: NASANode Process — 자동 감지: command 키로 제어

```gherkin
Given NASANode가 초기화된 경우
When msg.Payload에 {"command":"set_power","device_id":"living-room","params":{"power":true}}가 포함된 메시지로 Process가 호출되면
Then 제어 로직이 실행되어야 한다 (NASAControlNode Process와 동일)
And Agent에 set_power 명령이 전달되어야 한다
```

**요구사항**: REQ-NASA-001-09-14

### Scenario 20.3: NASANode Process — 자동 감지: 제어 키로 간소화 제어

```gherkin
Given NASANode가 device_id="living-room"으로 초기화된 경우
When msg.Payload에 {"power":true,"mode":"cool"}가 포함된 메시지로 Process가 호출되면
Then 간소화 제어 로직이 실행되어야 한다
And Agent에 set_multiple 명령으로 자동 변환되어 전달되어야 한다
```

**요구사항**: REQ-NASA-001-09-14

### Scenario 20.4: NASANode Process — 자동 감지: 상태 조회

```gherkin
Given NASANode가 device_id="living-room"으로 초기화된 경우
When msg.Payload에 제어 키도 command 키도 없는 빈 메시지로 Process가 호출되면
Then 상태 조회 로직이 실행되어야 한다 (NASAStatusNode Process와 동일)
And Agent에 get_state 명령이 전달되어야 한다
```

**요구사항**: REQ-NASA-001-09-14

### Scenario 20.5: NASANode SourceNode 폴링 (선택적)

```gherkin
Given NASANode가 poll_interval="100ms"로 초기화된 경우
When SourceCh()를 호출하면
Then nil이 아닌 채널이 반환되어야 한다
And 200ms 경과 후 상태 메시지가 수신되어야 한다

Given NASANode가 poll_interval이 설정되지 않은 경우
When SourceCh()를 호출하면
Then nil 채널이 반환되어야 한다
```

**요구사항**: REQ-NASA-001-09-12

### Scenario 20.6: NASANode Process — 자동 감지 우선순위

```gherkin
Given NASANode가 초기화된 경우
When msg.Payload에 {"command":"get_state","power":true}가 포함된 메시지로 Process가 호출되면
Then command 키가 우선되어 get_state 상태 조회가 실행되어야 한다 (power 키는 무시)
```

**요구사항**: REQ-NASA-001-09-14

---

## 21. Module 18: NASA 노드 레지스트리 및 공통 (v1.3.0)

### Scenario 21.1: 노드 레지스트리 등록 확인

```gherkin
Given NewRegistry()로 기본 레지스트리가 생성된 경우
When registry.Types()를 호출하면
Then 15개의 빌트인 타입이 반환되어야 한다
And "nasa-status", "nasa-control", "nasa" 타입이 포함되어야 한다
And registry.Has("nasa-status")가 true를 반환해야 한다
And registry.Has("nasa-control")가 true를 반환해야 한다
And registry.Has("nasa")가 true를 반환해야 한다
```

**요구사항**: REQ-NASA-001-09-16

### Scenario 21.2: 팩토리를 통한 노드 생성

```gherkin
Given "nasa-status" 타입이 등록된 레지스트리가 있는 경우
When registry.Create(flow.NodeDef{Type:"nasa-status", ID:"n1", Name:"test"})가 호출되면
Then NASAStatusNode 인스턴스가 반환되어야 한다
And 노드의 Type()이 "nasa-status"를 반환해야 한다

Given "nasa-control" 타입이 등록된 레지스트리가 있는 경우
When registry.Create(flow.NodeDef{Type:"nasa-control", ID:"n2", Name:"test"})가 호출되면
Then NASAControlNode 인스턴스가 반환되어야 한다

Given "nasa" 타입이 등록된 레지스트리가 있는 경우
When registry.Create(flow.NodeDef{Type:"nasa", ID:"n3", Name:"test"})가 호출되면
Then NASANode 인스턴스가 반환되어야 한다
```

**요구사항**: REQ-NASA-001-09-16

### Scenario 21.3: callAgentProcess 타임아웃

```gherkin
Given timeout이 50ms로 설정되고 Agent Process()가 100ms 이상 소요되는 경우
When callAgentProcess(ctx, agent, cmdBytes, 50ms)가 호출되면
Then context.DeadlineExceeded를 래핑한 에러가 반환되어야 한다
And Agent Process() 고루틴이 백그라운드에서 완료되더라도 안전해야 한다
```

**요구사항**: REQ-NASA-001-09-18

---

## 16. Quality Gate 기준

### 16.1 테스트 커버리지

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
| `internal/node/adapter/nasa.go` | 85%+ |
| `internal/node/nasa.go` | 85%+ (v1.3.0) |
| **전체 패키지** | **85%+** |

### 16.2 코드 품질

- `go vet ./internal/agent/samsung/...` 경고 0건
- `go vet ./internal/node/adapter/...` 경고 0건
- `go vet ./internal/node/...` 경고 0건 (v1.3.0 nasa.go 포함)
- `go test -race ./internal/agent/samsung/...` 데이터 레이스 0건
- `go test -race ./internal/node/adapter/...` 데이터 레이스 0건
- `go test -race ./internal/node/...` 데이터 레이스 0건 (v1.3.0 nasa_test.go 포함)
- 모든 exported 타입 및 함수에 GoDoc 주석 포함
- 센티널 에러는 `errors.Is()` 호환

### 16.3 Definition of Done

- [x] 모든 요구사항(REQ-NASA-001-*)에 대한 테스트 시나리오 존재
- [x] `go test -race -cover ./internal/agent/samsung/...` 통과 (85%+ 커버리지)
- [x] `go vet ./internal/agent/samsung/...` 경고 0건
- [x] TypeRegistry에 "samsung-nasa" 타입 등록 완료
- [x] cmd/xflowd/main.go에서 RegisterSamsungNASATypes 호출 추가
- [x] 예제 설정 YAML 파일 작성
- [x] SPEC-NASA-001 문서와 구현 코드 간 추적성(traceability) 확인
- [x] unsupported_msg_sets 설정 파싱 및 hex/float64 변환 테스트 (TS-12)
- [x] filterMessageSets 필터링 로직 및 로그 출력 테스트 (TS-13)
- [x] HexKeyByteMap MarshalJSON/UnmarshalJSON 직렬화 테스트 (TS-14)
- [x] StateForJSON 조건부 RawMessageSets 포함/제외 테스트 (TS-15)
- [x] NASAAdapter BridgeAdapter + CommandPollAdapter 통합 테스트 (TS-16)
- [x] 어댑터 레지스트리에 "samsung-nasa"로 등록 확인
- [ ] v1.2.0: ReconnectInterval/MaxReconnectBackoff config 파싱 테스트 (TS-R1)
- [ ] v1.2.0: Transport Available() I/O 에러 시 상태 갱신 테스트 (TS-R2)
- [ ] v1.2.0: reconnectLoop 지수 백오프 및 로그 억제 테스트 (TS-R3)
- [ ] v1.2.0: receiveLoop 연결 끊김 감지 테스트 (TS-R4)
- [ ] v1.2.0: pollLoop disconnectCh 중지 테스트 (TS-R5)
- [ ] v1.2.0: Start() 연결 실패 시 재연결 루프 진입 테스트 (TS-R6)
- [ ] v1.2.0: State() 확장 필드 (transport_connected, reconnecting, reconnect_attempts) 테스트 (TS-R7)
- [ ] v1.2.0: 재연결 이벤트 메시지 (transport_disconnected/reconnecting/reconnected) 테스트 (TS-R8)
- [ ] v1.3.0: NASANodeConfig 파싱 및 기본값 테스트 (TS-N1)
- [ ] v1.3.0: NASAStatusNode Configure/Init/Process 테스트 (TS-N2)
- [ ] v1.3.0: NASAStatusNode SourceNode 폴링 테스트 (TS-N3)
- [ ] v1.3.0: NASAControlNode 직접 명령 + 간소화 형식 테스트 (TS-N4)
- [ ] v1.3.0: NASANode 복합 자동 감지 테스트 (TS-N5)
- [ ] v1.3.0: 센티널 에러 정의 및 errors.Is() 호환 테스트 (TS-N6)
- [ ] v1.3.0: 노드 레지스트리 등록 15개 빌트인 테스트 (TS-N7)
- [ ] v1.3.0: 런타임 메시지 오버라이드 테스트 (TS-N8)
- [ ] v1.3.0: callAgentProcess 타임아웃 테스트 (TS-N9)
- [ ] v1.3.0: 프론트엔드 nodeSchemas.ts/nodeTypeMeta.ts 업데이트 확인 (TS-N10)

---

*SPEC-NASA-001 Acceptance v1.3.0*
*작성자: xtra*
*최초 작성: 2026-02-24*
*최종 수정: 2026-03-12*
