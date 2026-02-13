---
id: SPEC-SAGENT-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-SAGENT-001
---

# SPEC-SAGENT-001 수락 기준

## Module 1: MQTT Client Agent

### AC-SAGENT-001-01: MQTTAgent 초기화

```gherkin
Given 유효한 MQTTConfig (BrokerURL: "tcp://localhost:1883", ClientID: "test-client", QoS: 1)가 주어졌을 때
When MQTTAgent.Init(config)를 호출하면
Then nil error를 반환해야 한다
And Agent의 Type()이 "mqtt"여야 한다
And Agent의 상태가 Initialized가 되어야 한다
```

### AC-SAGENT-001-02: MQTT 브로커 연결 수립

```gherkin
Given 초기화된 MQTTAgent가 주어졌을 때
When Start(ctx)를 호출하면
Then MQTT 브로커에 연결되어야 한다
And Agent의 상태가 Running이 되어야 한다
And Health().Status가 Healthy여야 한다
```

### AC-SAGENT-001-03: MQTT 토픽 구독 및 메시지 수신

```gherkin
Given Running 상태인 MQTTAgent가 브로커에 연결된 후
When Subscribe("sensor/temperature", 1)를 호출하면
Then nil error를 반환해야 한다

When 브로커에서 "sensor/temperature" 토픽에 "25.5" 메시지가 발행되면
Then 수신 콜백이 호출되어야 한다
And AgentStats.MessagesReceived가 1 증가해야 한다
And AgentStats.BytesRead가 4 증가해야 한다 (페이로드 크기)
And 수신 메시지가 Message 인터페이스로 변환되어야 한다
```

### AC-SAGENT-001-04: MQTT 와일드카드 토픽 구독

```gherkin
Given Running 상태인 MQTTAgent가 브로커에 연결된 후
When Subscribe("sensor/+/data", 0)를 호출하면
Then nil error를 반환해야 한다

When 브로커에서 "sensor/room1/data" 토픽에 메시지가 발행되면
Then 수신 콜백이 호출되어야 한다

When 브로커에서 "sensor/room2/data" 토픽에 메시지가 발행되면
Then 수신 콜백이 호출되어야 한다
```

### AC-SAGENT-001-05: MQTT 다중 토픽 구독

```gherkin
Given Running 상태인 MQTTAgent가 브로커에 연결된 후
When SubscribeMultiple(map[string]byte{"topic/a": 0, "topic/b": 1, "topic/c": 2})를 호출하면
Then nil error를 반환해야 한다
And 3개 토픽 모두 구독되어야 한다
```

### AC-SAGENT-001-06: MQTT 메시지 발행

```gherkin
Given Running 상태인 MQTTAgent가 브로커에 연결된 후
When Publish("control/led", 1, false, []byte("on"))를 호출하면
Then nil error를 반환해야 한다
And AgentStats.MessagesSent가 1 증가해야 한다
And AgentStats.BytesWritten이 2 증가해야 한다 (페이로드 크기)
```

### AC-SAGENT-001-07: MQTT QoS 레벨별 발행

```gherkin
Given Running 상태인 MQTTAgent가 브로커에 연결된 후

When Publish("topic", 0, false, []byte("data"))를 호출하면
Then nil error를 반환해야 한다 (QoS 0: fire-and-forget)

When Publish("topic", 1, false, []byte("data"))를 호출하면
Then nil error를 반환해야 한다 (QoS 1: at-least-once)

When Publish("topic", 2, false, []byte("data"))를 호출하면
Then nil error를 반환해야 한다 (QoS 2: exactly-once)
```

### AC-SAGENT-001-08: MQTT 잘못된 QoS 레벨 거부

```gherkin
Given Running 상태인 MQTTAgent가 주어졌을 때
When Subscribe("topic", 3)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidQoS)이 true여야 한다

When Publish("topic", 5, false, []byte("data"))를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidQoS)이 true여야 한다
```

### AC-SAGENT-001-09: MQTT 미연결 상태 발행 거부

```gherkin
Given Initialized 상태인 (브로커 미연결) MQTTAgent가 주어졌을 때
When Publish("topic", 0, false, []byte("data"))를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrNotConnected)이 true여야 한다
```

### AC-SAGENT-001-10: MQTT 자동 재연결

```gherkin
Given Running 상태인 MQTTAgent가 브로커에 연결되고, Subscribe("sensor/#", 1)로 구독 중일 때
When 브로커 연결이 끊기면
Then Health().Status가 Unhealthy 또는 Degraded로 변경되어야 한다

When 브로커가 다시 사용 가능해지면
Then 자동 재연결이 수행되어야 한다
And 이전 구독 ("sensor/#", QoS 1)이 자동 복원되어야 한다
And Health().Status가 다시 Healthy로 변경되어야 한다
```

### AC-SAGENT-001-11: MQTT TLS 연결

```gherkin
Given TLSConfig (CACert: "ca.pem", InsecureSkipVerify: false)가 포함된 MQTTConfig가 주어졌을 때
When MQTTAgent.Start(ctx)를 호출하면
Then TLS로 브로커에 연결되어야 한다
And 인증서 검증이 수행되어야 한다
```

### AC-SAGENT-001-12: MQTT Agent 정상 종료

```gherkin
Given Running 상태인 MQTTAgent가 "sensor/#" 토픽을 구독 중일 때
When Stop(ctx)를 호출하면
Then 모든 구독이 해제되어야 한다
And MQTT 클라이언트가 정상 종료(Disconnect)되어야 한다
And Agent의 상태가 Stopped가 되어야 한다
```

### AC-SAGENT-001-13: MQTT TypeRegistry 등록 확인

```gherkin
Given MQTT 패키지가 import된 상태에서
When TypeRegistry.HasType("mqtt")를 호출하면
Then true를 반환해야 한다

When TypeRegistry.CreateAgent("mqtt", validConfig)를 호출하면
Then MQTTAgent 인스턴스가 생성되어야 한다
And nil error를 반환해야 한다
```

---

## Module 2: HTTP Client Agent

### AC-SAGENT-001-14: HTTPClientAgent 초기화

```gherkin
Given 유효한 HTTPClientConfig (BaseURL: "https://api.example.com", Method: "GET", Timeout: 30초)가 주어졌을 때
When HTTPClientAgent.Init(config)를 호출하면
Then nil error를 반환해야 한다
And Agent의 Type()이 "http-client"여야 한다
```

### AC-SAGENT-001-15: HTTP 폴링 시작

```gherkin
Given HTTPClientConfig에 PollInterval: 5초가 설정된 HTTPClientAgent가 초기화된 후
When Start(ctx)를 호출하면
Then 5초 간격으로 BaseURL에 HTTP GET 요청이 전송되어야 한다
And 각 응답이 Message로 변환되어 Bridge Node에 전달되어야 한다
And AgentStats.MessagesReceived가 응답 수만큼 증가해야 한다
```

### AC-SAGENT-001-16: HTTP 요청/응답

```gherkin
Given Running 상태인 HTTPClientAgent가 주어졌을 때
When Get(ctx, "/data")를 호출하면
Then HTTPResponse가 반환되어야 한다
And HTTPResponse.StatusCode가 200이어야 한다
And HTTPResponse.Body에 응답 본문이 포함되어야 한다
And HTTPResponse.Duration이 0보다 커야 한다
```

### AC-SAGENT-001-17: HTTP 요청 재시도

```gherkin
Given RetryCount: 3, RetryBackoff: 1초로 설정된 HTTPClientAgent가 주어졌을 때
When 서버가 503 응답을 반환하면
Then 최대 3회 재시도가 수행되어야 한다
And 재시도 간격이 1초, 2초, 4초 (지수 백오프)여야 한다

When 서버가 400 응답을 반환하면
Then 재시도 없이 즉시 에러를 반환해야 한다
```

### AC-SAGENT-001-18: HTTP 잘못된 URL 거부

```gherkin
Given BaseURL이 빈 문자열인 HTTPClientConfig가 주어졌을 때
When HTTPClientAgent.Init(config)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidConfig)이 true여야 한다
```

### AC-SAGENT-001-19: HTTP TypeRegistry 등록 확인

```gherkin
Given HTTP 패키지가 import된 상태에서
When TypeRegistry.HasType("http-client")를 호출하면
Then true를 반환해야 한다

When TypeRegistry.HasType("http-server")를 호출하면
Then true를 반환해야 한다
```

---

## Module 3: HTTP Server Agent

### AC-SAGENT-001-20: HTTPServerAgent 시작

```gherkin
Given 유효한 HTTPServerConfig (ListenAddr: ":0", Path: "/webhook", Methods: ["POST"])가 주어졌을 때
When HTTPServerAgent.Start(ctx)를 호출하면
Then 지정된 주소에서 HTTP 요청 수신이 시작되어야 한다
And Agent의 상태가 Running이 되어야 한다
```

### AC-SAGENT-001-21: 웹훅 수신 처리

```gherkin
Given Running 상태인 HTTPServerAgent가 "/webhook" 경로를 수신 중일 때
When POST /webhook 요청이 {"temperature": 25.5} 본문으로 수신되면
Then HTTP 200 응답이 반환되어야 한다
And 요청 본문이 Message로 변환되어 Bridge Node에 전달되어야 한다
And AgentStats.MessagesReceived가 1 증가해야 한다
```

### AC-SAGENT-001-22: HTTP 최대 본문 크기 초과 거부

```gherkin
Given MaxBodySize: 1024 (1KB)로 설정된 HTTPServerAgent가 수신 중일 때
When 2048 바이트 본문의 POST 요청이 수신되면
Then HTTP 413 (Payload Too Large) 응답이 반환되어야 한다
And AgentStats.MessagesErrored가 1 증가해야 한다
```

### AC-SAGENT-001-23: HTTP 허용되지 않은 메서드 거부

```gherkin
Given Methods: ["POST"]로 설정된 HTTPServerAgent가 수신 중일 때
When GET /webhook 요청이 수신되면
Then HTTP 405 (Method Not Allowed) 응답이 반환되어야 한다
```

### AC-SAGENT-001-24: HTTP Server Graceful Shutdown

```gherkin
Given Running 상태인 HTTPServerAgent가 요청 처리 중일 때
When Stop(ctx)를 호출하면
Then 진행 중인 요청 처리가 완료되어야 한다
And 새로운 요청 수신이 중지되어야 한다
And Agent의 상태가 Stopped가 되어야 한다
```

---

## Module 4: WebSocket Client Agent

### AC-SAGENT-001-25: WSClientAgent 연결 수립

```gherkin
Given 유효한 WSClientConfig (URL: "ws://localhost:9090/ws", PingInterval: 30초)가 주어졌을 때
When WSClientAgent.Start(ctx)를 호출하면
Then WebSocket 서버에 연결되어야 한다
And Agent의 상태가 Running이 되어야 한다
And 핑 전송 goroutine이 시작되어야 한다
And 읽기 goroutine이 시작되어야 한다
```

### AC-SAGENT-001-26: WebSocket 메시지 송수신

```gherkin
Given Running 상태인 WSClientAgent가 서버에 연결된 후
When SendText("hello")를 호출하면
Then nil error를 반환해야 한다
And AgentStats.MessagesSent가 1 증가해야 한다

When 서버에서 텍스트 메시지 "world"가 수신되면
Then 수신 메시지가 Message로 변환되어 Bridge Node에 전달되어야 한다
And AgentStats.MessagesReceived가 1 증가해야 한다
```

### AC-SAGENT-001-27: WebSocket 자동 재연결

```gherkin
Given Running 상태인 WSClientAgent가 AutoReconnect: true로 설정되어 서버에 연결 중일 때
When WebSocket 연결이 끊기면
Then 지수 백오프 기반으로 재연결이 시도되어야 한다

When 서버가 다시 사용 가능해지면
Then 재연결이 성공해야 한다
And 읽기/핑 goroutine이 재시작되어야 한다
And Health().Status가 Healthy로 복원되어야 한다
```

### AC-SAGENT-001-28: WebSocket 핑/퐁 타임아웃

```gherkin
Given WSClientAgent가 PingInterval: 5초, PongTimeout: 3초로 연결된 상태에서
When 5초 후 핑이 전송되었지만 3초 내에 퐁 응답이 없으면
Then 연결이 닫혀야 한다
And 자동 재연결이 시작되어야 한다
```

### AC-SAGENT-001-29: WebSocket 최대 메시지 크기 초과 거부

```gherkin
Given MaxMessageSize: 1024로 설정된 WSClientAgent가 연결된 상태에서
When 서버에서 2048 바이트 메시지가 수신되면
Then 연결이 정상적으로 닫혀야 한다
And AgentStats.MessagesErrored가 1 증가해야 한다
```

---

## Module 5: WebSocket Server Agent

### AC-SAGENT-001-30: WSServerAgent 시작

```gherkin
Given 유효한 WSServerConfig (ListenAddr: ":0", Path: "/ws", MaxConnections: 10)가 주어졌을 때
When WSServerAgent.Start(ctx)를 호출하면
Then 지정된 주소에서 WebSocket 업그레이드 요청 수신이 시작되어야 한다
And Agent의 상태가 Running이 되어야 한다
```

### AC-SAGENT-001-31: WebSocket 다중 클라이언트 연결

```gherkin
Given Running 상태인 WSServerAgent가 수신 중일 때
When 3개의 WebSocket 클라이언트가 연결되면
Then ActiveConnections()가 3을 반환해야 한다
And 각 연결에 대해 개별 읽기 goroutine이 생성되어야 한다
```

### AC-SAGENT-001-32: WebSocket 브로드캐스트

```gherkin
Given 3개의 클라이언트가 연결된 WSServerAgent가 주어졌을 때
When Broadcast(TextMessage, []byte("notification"))를 호출하면
Then nil error를 반환해야 한다
And 3개 클라이언트 모두 "notification" 메시지를 수신해야 한다
```

### AC-SAGENT-001-33: WebSocket 특정 연결 메시지 전송

```gherkin
Given 3개의 클라이언트 (connA, connB, connC)가 연결된 WSServerAgent가 주어졌을 때
When SendTo(connB, TextMessage, []byte("private"))를 호출하면
Then connB만 "private" 메시지를 수신해야 한다
And connA, connC는 메시지를 수신하지 않아야 한다
```

### AC-SAGENT-001-34: WebSocket 최대 연결 수 초과 거부

```gherkin
Given MaxConnections: 2로 설정된 WSServerAgent에 2개 클라이언트가 연결된 상태에서
When 3번째 클라이언트가 연결을 시도하면
Then HTTP 503 (Service Unavailable) 응답이 반환되어야 한다
And ActiveConnections()가 2를 유지해야 한다
```

### AC-SAGENT-001-35: WebSocket 연결/해제 이벤트

```gherkin
Given Running 상태인 WSServerAgent가 주어졌을 때
When 새 클라이언트가 연결되면
Then StatusEvent가 발행되어야 한다
And ActiveConnections()가 1 증가해야 한다

When 연결된 클라이언트가 해제되면
Then StatusEvent가 발행되어야 한다
And ActiveConnections()가 1 감소해야 한다
```

---

## Module 6: gRPC Client Agent

### AC-SAGENT-001-36: GRPCClientAgent 연결 수립

```gherkin
Given 유효한 GRPCClientConfig (Target: "localhost:50051", TLS: nil)가 주어졌을 때
When GRPCClientAgent.Start(ctx)를 호출하면
Then gRPC 서버에 연결되어야 한다
And Agent의 상태가 Running이 되어야 한다
```

### AC-SAGENT-001-37: gRPC Unary RPC 호출

```gherkin
Given Running 상태인 GRPCClientAgent가 서버에 연결된 후
When Invoke(ctx, "/service/Method", request, &response)를 호출하면
Then nil error를 반환해야 한다
And response에 서버 응답이 채워져야 한다
And AgentStats.MessagesSent가 1 증가해야 한다
And AgentStats.MessagesReceived가 1 증가해야 한다
```

### AC-SAGENT-001-38: gRPC 서버 스트리밍 RPC

```gherkin
Given Running 상태인 GRPCClientAgent가 서버에 연결된 후
When ServerStream(ctx, "/service/StreamMethod", request)를 호출하면
Then GRPCStream이 반환되어야 한다

When stream.Recv()를 반복 호출하면
Then 서버에서 전송하는 메시지를 순차적으로 수신해야 한다
And 스트림 종료 시 io.EOF를 반환해야 한다
```

### AC-SAGENT-001-39: gRPC 연결 상태 모니터링

```gherkin
Given Running 상태인 GRPCClientAgent가 서버에 연결된 상태에서
When gRPC 서버가 중지되면
Then Health().Status가 Unhealthy로 변경되어야 한다
And StatusEvent가 발행되어야 한다

When gRPC 서버가 다시 시작되면
Then 연결이 복원되어야 한다
And Health().Status가 Healthy로 변경되어야 한다
```

### AC-SAGENT-001-40: gRPC TypeRegistry 등록 확인

```gherkin
Given gRPC 패키지가 import된 상태에서
When TypeRegistry.HasType("grpc-client")를 호출하면
Then true를 반환해야 한다

When TypeRegistry.HasType("grpc-server")를 호출하면
Then true를 반환해야 한다
```

---

## Module 7: gRPC Server Agent

### AC-SAGENT-001-41: GRPCServerAgent 시작

```gherkin
Given 유효한 GRPCServerConfig (ListenAddr: ":0", TLS: nil)가 주어졌을 때
When GRPCServerAgent.Start(ctx)를 호출하면
Then 지정된 주소에서 gRPC 요청 수신이 시작되어야 한다
And Agent의 상태가 Running이 되어야 한다
```

### AC-SAGENT-001-42: gRPC 서비스 핸들러 등록

```gherkin
Given 초기화된 GRPCServerAgent가 주어졌을 때
When RegisterService(serviceDesc, serviceImpl)를 호출하면
Then 서비스 핸들러가 gRPC 서버에 등록되어야 한다

When 클라이언트가 등록된 서비스의 메서드를 호출하면
Then 핸들러가 실행되어야 한다
And 수신된 요청이 Message로 변환되어 Bridge Node에 전달되어야 한다
```

### AC-SAGENT-001-43: gRPC Server Graceful Shutdown

```gherkin
Given Running 상태인 GRPCServerAgent가 RPC 처리 중일 때
When Stop(ctx)를 호출하면
Then 진행 중인 RPC 처리가 완료되어야 한다 (GracefulStop)
And 새로운 연결 수신이 중지되어야 한다
And Agent의 상태가 Stopped가 되어야 한다

When context deadline을 초과하면
Then 강제 종료 (Stop)가 수행되어야 한다
```

---

## Module 8: Samsung NASA Manager Agent

### AC-SAGENT-001-44: NASAAgent 초기화

```gherkin
Given 유효한 NASAConfig (TransportType: "tcp", TCPAddr: "192.168.1.100:4196", ProtocolFile: "nasa.yaml")가 주어졌을 때
When NASAAgent.Init(config)를 호출하면
Then nil error를 반환해야 한다
And Agent의 Type()이 "nasa"여야 한다
And nasa.yaml 프로토콜 정의가 로드되어야 한다
```

### AC-SAGENT-001-45: NASA 디바이스 상태 폴링

```gherkin
Given Running 상태인 NASAAgent가 PollInterval: 5초로 설정되고, DeviceAddresses: [0x01, 0x02]가 등록된 후
When 5초 폴링 주기에 도달하면
Then 디바이스 0x01에 상태 조회 명령이 전송되어야 한다
And 디바이스 0x02에 상태 조회 명령이 전송되어야 한다
And 응답이 파싱되어 NASADevice.State가 업데이트되어야 한다
```

### AC-SAGENT-001-46: NASA 제어 명령 - 전원/온도/모드/풍량

```gherkin
Given Running 상태인 NASAAgent가 디바이스 0x01이 등록된 후

When SetPower(0x01, true)를 호출하면
Then nil error를 반환해야 한다
And 전원 ON 명령 패킷이 Transport로 전송되어야 한다

When SetTemperature(0x01, 24.0)를 호출하면
Then nil error를 반환해야 한다
And 온도 설정 명령 패킷이 Transport로 전송되어야 한다

When SetMode(0x01, "cool")를 호출하면
Then nil error를 반환해야 한다
And 운전 모드 변경 명령 패킷이 Transport로 전송되어야 한다

When SetFanSpeed(0x01, "high")를 호출하면
Then nil error를 반환해야 한다
And 풍량 변경 명령 패킷이 Transport로 전송되어야 한다
```

### AC-SAGENT-001-47: NASA 디바이스 상태 조회

```gherkin
Given NASAAgent에 디바이스 0x01 (Power: true, Mode: "cool", TargetTemp: 24.0, CurrentTemp: 25.5)이 등록된 후
When GetDeviceState(0x01)를 호출하면
Then NASADeviceState가 반환되어야 한다
And Power가 true여야 한다
And Mode가 "cool"이어야 한다
And TargetTemp가 24.0이어야 한다
And CurrentTemp가 25.5여야 한다

When ListDevices()를 호출하면
Then 1개의 NASADevice가 반환되어야 한다
And NASADevice.Online이 true여야 한다
```

### AC-SAGENT-001-48: NASA 프로토콜 메시지 파싱

```gherkin
Given nasa.yaml 프로토콜 정의가 로드된 NASAAgent가 주어졌을 때
When Transport에서 유효한 NASA 프로토콜 패킷이 수신되면
Then Protocol Definition Engine이 패킷을 파싱해야 한다
And 파싱된 데이터에서 대상 디바이스 주소를 추출해야 한다
And 해당 NASADevice의 State를 업데이트해야 한다
And AgentStats.MessagesReceived가 1 증가해야 한다
```

### AC-SAGENT-001-49: NASA 유효하지 않은 디바이스 주소 거부

```gherkin
Given NASAAgent에 디바이스 0x01만 등록된 상태에서
When SetPower(0xFF, true)를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrDeviceNotFound)이 true여야 한다
```

### AC-SAGENT-001-50: NASA 유효하지 않은 운전 모드 거부

```gherkin
Given Running 상태인 NASAAgent가 디바이스 0x01이 등록된 후
When SetMode(0x01, "invalid-mode")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidCommand)이 true여야 한다

When SetFanSpeed(0x01, "ultra")를 호출하면
Then error가 nil이 아니어야 한다
And errors.Is(err, ErrInvalidCommand)이 true여야 한다
```

### AC-SAGENT-001-51: NASA TypeRegistry 등록 확인

```gherkin
Given Samsung 패키지가 import된 상태에서
When TypeRegistry.HasType("nasa")를 호출하면
Then true를 반환해야 한다
```

---

## Module 9: Custom Protocol Template

### AC-SAGENT-001-52: CustomAgent 생성

```gherkin
Given Transport: TCP, ProtocolFile: "custom.yaml"이 포함된 AgentConfig가 주어졌을 때
When NewCustomAgent(config)를 호출하면
Then CustomAgent 인스턴스가 생성되어야 한다
And nil error를 반환해야 한다
And custom.yaml 프로토콜 정의가 로드되어야 한다
```

### AC-SAGENT-001-53: CustomAgent 데이터 수신 및 파싱

```gherkin
Given Running 상태인 CustomAgent가 TCP Transport에 연결된 후
When Transport에서 프로토콜 정의에 맞는 바이트 데이터가 수신되면
Then Protocol Definition Engine이 데이터를 파싱해야 한다
And 파싱 결과가 Message로 변환되어 Bridge Node에 전달되어야 한다
And AgentStats.MessagesReceived가 1 증가해야 한다
```

### AC-SAGENT-001-54: CustomAgent 데이터 송신 및 직렬화

```gherkin
Given Running 상태인 CustomAgent가 TCP Transport에 연결된 후
When Bridge Node로부터 Message가 수신되면
Then Protocol Definition Engine이 Message를 바이트 데이터로 직렬화해야 한다
And Transport를 통해 직렬화된 데이터가 전송되어야 한다
And AgentStats.MessagesSent가 1 증가해야 한다
```

---

## Module 10: 표준 Agent TypeRegistry 등록

### AC-SAGENT-001-55: 전체 표준 Agent 타입 등록 확인

```gherkin
Given 모든 표준 Agent 패키지가 import된 상태에서
When TypeRegistry.ListTypes()를 호출하면
Then 반환된 목록에 다음이 포함되어야 한다:
  - "mqtt"
  - "http-client"
  - "http-server"
  - "websocket-client"
  - "websocket-server"
  - "grpc-client"
  - "grpc-server"
  - "nasa"
```

### AC-SAGENT-001-56: AgentFactory 정상 동작

```gherkin
Given TypeRegistry에 "mqtt" 타입이 등록된 상태에서
When TypeRegistry.CreateAgent("mqtt", validMQTTConfig)를 호출하면
Then MQTTAgent 인스턴스가 생성되어야 한다
And Agent.Type()이 "mqtt"여야 한다
And nil error를 반환해야 한다

When TypeRegistry.CreateAgent("mqtt", invalidConfig)를 호출하면
Then Agent가 nil이어야 한다
And error가 nil이 아니어야 한다
```

---

## Module 11: 표준 Agent Error Types

### AC-SAGENT-001-57: Sentinel 에러 errors.Is() 호환

```gherkin
Given 모든 표준 Agent sentinel 에러가 정의된 상태에서
When errors.Is(ErrInvalidQoS, ErrInvalidQoS)를 호출하면
Then true를 반환해야 한다

When errors.Is(ErrNotConnected, ErrNotConnected)를 호출하면
Then true를 반환해야 한다

When errors.Is(ErrDeviceNotFound, ErrDeviceNotFound)를 호출하면
Then true를 반환해야 한다

When errors.Is(ErrInvalidQoS, ErrNotConnected)를 호출하면
Then false를 반환해야 한다
```

### AC-SAGENT-001-58: 에러 메시지 명확성

```gherkin
Given 모든 표준 Agent sentinel 에러가 정의된 상태에서
When ErrInvalidQoS.Error()를 호출하면
Then 빈 문자열이 아닌 설명적 에러 메시지를 반환해야 한다

When ErrDeviceNotFound.Error()를 호출하면
Then 빈 문자열이 아닌 설명적 에러 메시지를 반환해야 한다
```

---

## Quality Gates

### QG-1: 테스트 커버리지

```gherkin
Given 모든 표준 Agent 코드가 작성된 상태에서
When go test -coverprofile=coverage.out ./internal/agent/mqtt/ ./internal/agent/http/ ./internal/agent/websocket/ ./internal/agent/grpc/ ./internal/agent/samsung/을 실행하면
Then 각 패키지의 테스트 커버리지가 85% 이상이어야 한다
```

### QG-2: Race Condition 검사

```gherkin
Given 모든 표준 Agent 테스트가 작성된 상태에서
When go test -race ./internal/agent/...을 실행하면
Then data race가 감지되지 않아야 한다
```

### QG-3: 정적 분석

```gherkin
Given 모든 표준 Agent 코드가 작성된 상태에서
When golangci-lint run ./internal/agent/...을 실행하면
Then lint 에러가 0개여야 한다

When go vet ./internal/agent/...을 실행하면
Then vet 에러가 0개여야 한다
```

### QG-4: 벤치마크 성능 기준

```gherkin
Given 벤치마크 테스트가 작성된 상태에서
When go test -bench=. ./internal/agent/...을 실행하면
Then BenchmarkMQTTPublish가 합리적인 성능을 보여야 한다
And BenchmarkHTTPPoll이 합리적인 성능을 보여야 한다
And BenchmarkWSBroadcast가 합리적인 성능을 보여야 한다
And BenchmarkGRPCUnary가 합리적인 성능을 보여야 한다
```
