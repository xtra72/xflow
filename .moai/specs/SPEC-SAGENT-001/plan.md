---
id: SPEC-SAGENT-001
type: plan
version: "1.0.0"
spec_ref: SPEC-SAGENT-001
---

# SPEC-SAGENT-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반): 모든 파일이 신규 생성이므로 TDD(RED-GREEN-REFACTOR) 적용
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-001): BaseAgent, Agent 인터페이스, TypeRegistry, Transport
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 인터페이스
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent 타입
- **외부 의존성**:
  - `github.com/eclipse/paho.mqtt.golang` v1.4+ (MQTT)
  - `github.com/gorilla/websocket` v1.5+ (WebSocket)
  - `google.golang.org/grpc` v1.60+ (gRPC)
  - `google.golang.org/protobuf` v1.32+ (Protobuf)
  - `go.bug.st/serial` v1.6+ (Samsung NASA RS-485)

### 1.3 패키지 위치

- **MQTT**: `internal/agent/mqtt/`
- **HTTP**: `internal/agent/http/`
- **WebSocket**: `internal/agent/websocket/`
- **gRPC**: `internal/agent/grpc/`
- **Samsung NASA**: `internal/agent/samsung/`
- **Tier**: Tier 2 - 내부 실행 계층

---

## 2. 마일스톤

### Primary Goal: MQTT Client Agent + HTTP Client/Server Agent + Error Types + TypeRegistry 등록 (P0)

**범위**: Module 1, 2, 3, 10, 11

**작업 항목**:

1. `mqtt/agent.go` + `mqtt/mqtt_test.go` 작성 (에러 정의 포함)
   - MQTT 전용 sentinel 에러 정의 (ErrInvalidQoS, ErrNotConnected, ErrSubscriptionFailed, ErrPublishFailed, ErrBrokerConnectionFailed)
   - `MQTTAgent` 구조체 정의 (BaseAgent 임베딩)
   - `MQTTConfig` 구조체 정의 및 유효성 검증
   - `TLSConfig` 구조체 정의
   - `init()` 함수에서 TypeRegistry.RegisterType("mqtt", factory) 등록
   - Init, Start, Stop 생명주기 테스트
   - 잘못된 QoS 거부 테스트
   - 미연결 상태 발행 거부 테스트
   - TypeRegistry 등록 확인 테스트

2. `mqtt/client.go` + `mqtt/mqtt_test.go` 확장
   - Eclipse Paho 클라이언트 래퍼 구현
   - 연결/해제 로직
   - TLS 설정 적용 로직
   - 유언 메시지 설정 로직
   - 자동 재연결 설정
   - 연결 콜백 (OnConnect, OnConnectionLost) 구현

3. `mqtt/subscriber.go` + `mqtt/mqtt_test.go` 확장
   - Subscribe, SubscribeMultiple, Unsubscribe 메서드
   - 와일드카드 토픽 (+, #) 지원 테스트
   - 수신 메시지 -> Message 변환 테스트
   - AgentStats 카운터 업데이트 테스트
   - QoS 0/1/2 구독 테스트

4. `mqtt/publisher.go` + `mqtt/mqtt_test.go` 확장
   - Publish 메서드
   - QoS 0/1/2 발행 테스트
   - 리테인 메시지 테스트
   - AgentStats.MessagesSent 카운터 테스트

5. `http/agent.go` + `http/http_test.go` 작성 (에러 정의 포함)
   - HTTP 전용 sentinel 에러 정의 (ErrHTTPRequestFailed, ErrHTTPServerStartFailed, ErrPayloadTooLarge, ErrMethodNotAllowed)
   - `HTTPClientAgent`, `HTTPServerAgent` 구조체 정의 (BaseAgent 임베딩)
   - `HTTPClientConfig`, `HTTPServerConfig` 구조체 정의
   - `HTTPResponse` 구조체 정의
   - `init()` 함수에서 TypeRegistry.RegisterType("http-client", factory), RegisterType("http-server", factory) 등록
   - TypeRegistry 등록 확인 테스트

6. `http/client.go` + `http/http_test.go` 확장
   - Request, Get, Post 메서드 구현
   - 폴링 goroutine 구현 (PollInterval > 0일 때)
   - 재시도 로직 (지수 백오프)
   - AgentStats 카운터 업데이트
   - 폴링 시작/정지 테스트
   - 재시도 로직 테스트
   - 잘못된 URL 거부 테스트

7. `http/server.go` + `http/http_test.go` 확장
   - HTTP 서버 시작/종료
   - 웹훅 수신 핸들러
   - MaxBodySize 검증
   - 허용 HTTP 메서드 검증
   - Graceful Shutdown 테스트
   - 최대 본문 크기 초과 거부 테스트 (HTTP 413)
   - 허용되지 않은 메서드 거부 테스트 (HTTP 405)
   - 수신 메시지 -> Message 변환 테스트

**산출물**: MQTT + HTTP Agent 완전 동작, TypeRegistry 등록 완료

---

### Secondary Goal: WebSocket Client/Server + gRPC Client/Server Agent (P1)

**범위**: Module 4, 5, 6, 7

**작업 항목**:

1. `websocket/agent.go` + `websocket/ws_test.go` 작성 (에러 정의 포함)
   - WebSocket 전용 sentinel 에러 정의 (ErrWSConnectionFailed, ErrWSWriteFailed, ErrWSMaxConnections, ErrWSMessageTooLarge)
   - `WSClientAgent`, `WSServerAgent` 구조체 정의 (BaseAgent 임베딩)
   - `WSClientConfig`, `WSServerConfig` 구조체 정의
   - `init()` 함수에서 TypeRegistry 등록 ("websocket-client", "websocket-server")
   - TypeRegistry 등록 확인 테스트

2. `websocket/client.go` + `websocket/ws_test.go` 확장
   - gorilla/websocket Dialer 기반 연결 수립
   - 핑/퐁 핸들러 구현
   - Send, SendText, SendBinary 메서드 구현
   - 읽기 goroutine 구현 (수신 메시지 대기)
   - 핑 전송 goroutine 구현
   - 자동 재연결 로직 (지수 백오프)
   - 연결/해제/재연결 테스트
   - 핑/퐁 타임아웃 테스트
   - 최대 메시지 크기 초과 거부 테스트
   - AgentStats 카운터 테스트

3. `websocket/server.go` + `websocket/ws_test.go` 확장
   - HTTP -> WebSocket 업그레이드 핸들러
   - 다중 클라이언트 연결 관리
   - Broadcast, SendTo 메서드 구현
   - ActiveConnections 메서드 구현
   - 연결/해제 이벤트 StatusEvent 발행
   - 최대 연결 수 초과 거부 테스트 (HTTP 503)
   - 브로드캐스트 테스트
   - 특정 연결 메시지 전송 테스트
   - 동시성 안전 테스트

4. `grpc/agent.go` + `grpc/grpc_test.go` 작성 (에러 정의 포함)
   - gRPC 전용 sentinel 에러 정의 (ErrGRPCDialFailed, ErrGRPCInvokeFailed, ErrGRPCStreamFailed, ErrGRPCServerStartFailed)
   - `GRPCClientAgent`, `GRPCServerAgent` 구조체 정의 (BaseAgent 임베딩)
   - `GRPCClientConfig`, `GRPCServerConfig`, `GRPCKeepAliveConfig` 구조체 정의
   - `GRPCStream` 인터페이스 정의
   - `init()` 함수에서 TypeRegistry 등록 ("grpc-client", "grpc-server")
   - TypeRegistry 등록 확인 테스트

5. `grpc/client.go` + `grpc/grpc_test.go` 확장
   - gRPC 연결 수립 (DialOption 구성)
   - Invoke, InvokeRaw 메서드 구현
   - ServerStream, ClientStream, BidiStream 메서드 구현
   - 연결 상태 모니터링 (connectivity.State)
   - 킵얼라이브 설정 테스트
   - Unary RPC 호출 테스트
   - 스트리밍 RPC 테스트
   - 연결 상태 변경 이벤트 테스트

6. `grpc/server.go` + `grpc/grpc_test.go` 확장
   - gRPC 서버 시작 (net.Listener)
   - RegisterService 메서드 구현
   - GracefulStop/Stop 종료 로직
   - 서버 시작/종료 테스트
   - 서비스 핸들러 등록 테스트
   - 최대 메시지 크기 검증 테스트
   - Graceful Shutdown 테스트

**산출물**: WebSocket + gRPC Agent 완전 동작

---

### Tertiary Goal: Samsung NASA Manager Agent + Custom Protocol Template (P2)

**범위**: Module 8, 9

**작업 항목**:

1. `samsung/agent.go` + `samsung/samsung_test.go` 작성 (에러 정의 포함)
   - NASA 전용 sentinel 에러 정의 (ErrDeviceNotFound, ErrInvalidCommand, ErrDeviceOffline, ErrNASAProtocolError)
   - `NASAAgent` 구조체 정의 (BaseAgent 임베딩)
   - `NASAConfig` 구조체 정의 및 유효성 검증
   - `init()` 함수에서 TypeRegistry.RegisterType("nasa", factory) 등록
   - Init, Start, Stop 생명주기 테스트
   - TypeRegistry 등록 확인 테스트

2. `samsung/device.go` + `samsung/samsung_test.go` 확장
   - `NASADevice` 구조체 정의
   - `NASADeviceState` 구조체 정의
   - 디바이스 목록 관리 (등록, 조회, 상태 업데이트)
   - ListDevices, GetDeviceState 메서드 구현
   - 디바이스 온라인/오프라인 상태 관리 테스트
   - 디바이스 상태 업데이트 테스트

3. `samsung/command.go` + `samsung/samsung_test.go` 확장
   - SetPower, SetMode, SetTemperature, SetFanSpeed 메서드 구현
   - NASA 프로토콜 명령 패킷 생성 로직
   - Protocol Definition Engine과 연동하여 명령 직렬화
   - 유효하지 않은 디바이스 주소 거부 테스트
   - 유효하지 않은 운전 모드/풍량 거부 테스트
   - 명령 패킷 직렬화 정확성 테스트

4. `samsung/nasa.yaml` 작성
   - NASA 프로토콜 정의 파일 (Protocol Definition Engine 포맷)
   - 헤더: STX, 소스주소, 대상주소, 명령코드, 데이터길이
   - 페이로드: 명령별 데이터 필드 정의
   - 트레일러: 체크섬, ETX
   - 조건부 파싱: 명령코드에 따른 페이로드 분기

5. CustomAgent 유틸리티 (BaseAgent 활용)
   - `NewCustomAgent()` 팩토리 함수 구현
   - Transport + Protocol Definition 조합 테스트
   - 데이터 수신 -> 파싱 -> Message 변환 테스트
   - Message -> 직렬화 -> Transport 전송 테스트

**산출물**: Samsung NASA + Custom Protocol Agent 완전 동작

---

### Optional Goal: 패키지 문서화

**범위**: 각 패키지 doc.go

**작업 항목**:

1. 각 표준 Agent 패키지에 `doc.go` 작성
   - 패키지 개요 문서
   - 주요 Config 구조체 필드 설명
   - 사용 예시 (Agent 생성, 설정, 시작)
   - Bridge Node 연결 예시

**산출물**: GoDoc 문서 완성

---

## 3. 기술적 접근 방식

### 3.1 BaseAgent 임베딩 전략

모든 표준 Agent는 `BaseAgent`를 포인터로 임베딩하여 공통 로직을 재사용한다:

- 생명주기 관리 (Init, Start, Stop, Pause, Resume)
- 상태 전이 및 StatusEvent 발행
- AgentStats 통계 수집
- Health Check 연동
- Shared Agent 참조 카운팅

프로토콜별 로직은 다음 메서드를 오버라이드하여 구현한다:

- `Start(ctx)`: 프로토콜별 연결 수립 후 BaseAgent.Start() 호출
- `Stop(ctx)`: 프로토콜별 정리 후 BaseAgent.Stop() 호출
- `Process(data)`: 프로토콜별 데이터 처리
- `Configure(config)`: 프로토콜별 런타임 설정 변경

### 3.2 MQTT 구현 전략

Eclipse Paho Go 클라이언트를 래핑하여 MQTT Agent를 구현한다:

- `paho.ClientOptions`를 `MQTTConfig`로부터 빌드
- 연결 콜백(`OnConnect`)에서 이전 구독 자동 복원
- 연결 끊김 콜백(`OnConnectionLost`)에서 HealthStatus 업데이트
- QoS 레벨별 메시지 처리 흐름:
  - QoS 0: Fire-and-forget
  - QoS 1: At-least-once (Paho 내부 재전송)
  - QoS 2: Exactly-once (Paho 내부 핸드셰이크)

### 3.3 HTTP 폴링/웹훅 전략

HTTP Agent는 두 가지 모드로 동작한다:

- **Client (폴링 모드)**: `time.Ticker` 기반으로 주기적 HTTP 요청, context 기반 종료
- **Server (웹훅 모드)**: `http.Server`로 수신 요청 처리, `Shutdown(ctx)` 기반 Graceful Shutdown

HTTP Client의 재시도 로직:
- 네트워크 에러 또는 5xx 응답 시 재시도
- 지수 백오프: `retryBackoff * 2^(attempt-1)`
- 4xx 응답은 재시도하지 않음 (클라이언트 에러)

### 3.4 WebSocket 연결 관리 전략

gorilla/websocket을 활용한 WebSocket Agent 구현:

- **Client**: Dialer로 연결 후 읽기 goroutine + 핑 goroutine 분리
- **Server**: http.Handler 기반 업그레이드, 연결별 읽기 goroutine 생성
- 쓰기 작업은 `sync.Mutex`로 보호 (WebSocket은 동시 쓰기 불가)
- 핑/퐁:
  - 클라이언트: PingInterval마다 핑 전송, PongTimeout 초과 시 재연결
  - 서버: 수신 핑에 자동 퐁 응답 (gorilla/websocket 기본 동작)

### 3.5 gRPC 클라이언트/서버 전략

google.golang.org/grpc를 활용한 gRPC Agent 구현:

- **Client**: `grpc.NewClient()`로 연결, `grpc.WithKeepaliveParams()` 설정
- **Server**: `grpc.NewServer()` + `net.Listener` 기반
- 연결 상태 모니터링: `conn.GetState()` 및 `conn.WaitForStateChange()` 활용
- 스트리밍: `grpc.Stream` 인터페이스 래핑으로 Send/Recv/Close 제공

### 3.6 Samsung NASA 프로토콜 전략

Samsung NASA 프로토콜은 Protocol Definition Engine(SPEC-AGENT-001)을 활용한다:

- nasa.yaml 파일에 프로토콜 구조 정의 (헤더, 페이로드, 체크섬)
- 조건부 파싱: 명령코드 필드 값에 따라 페이로드 구조 분기
- 마스터-슬레이브 통신: Agent가 마스터로 디바이스에 명령 전송
- 디바이스 상태는 메모리 내 `map[byte]*NASADevice`로 관리

### 3.7 테스트에서의 Mock 전략

외부 서비스 의존성을 mock으로 대체한다:

- **MQTT**: Eclipse Paho 인터페이스를 래핑하여 mock 클라이언트 제공
- **HTTP**: `httptest.Server`를 활용한 테스트 서버
- **WebSocket**: `httptest.Server` + gorilla/websocket 테스트 연결
- **gRPC**: `grpc.NewServer()` 기반 인메모리 테스트 서버 (`bufconn` 패키지)
- **Samsung NASA**: mock Transport (SPEC-AGENT-001의 Transport 인터페이스)

---

## 4. 리스크 및 대응

### R1: SPEC-AGENT-001 프레임워크 미완성

- **리스크**: BaseAgent, TypeRegistry 등 프레임워크 코드가 아직 구현되지 않아 표준 Agent 컴파일 불가
- **대응**: 인터페이스 기반 mock 사용, 프레임워크 완성 후 통합 테스트 실행
- **심각도**: 높음 (프레임워크 의존도 100%)

### R2: Eclipse Paho Go의 API 안정성

- **리스크**: Eclipse Paho Go의 API가 변경되면 MQTT Agent 코드 수정 필요
- **대응**: Paho 클라이언트를 인터페이스로 래핑하여 의존성 분리, 버전 고정 (go.mod)
- **심각도**: 낮음 (v1.4+는 안정 버전)

### R3: WebSocket 동시 쓰기 경합

- **리스크**: 다중 goroutine에서 WebSocket에 동시 쓰기 시 panic 발생
- **대응**: `sync.Mutex`로 쓰기 작업 직렬화, `go test -race` 필수 실행
- **심각도**: 중간 (알려진 gorilla/websocket 제약)

### R4: gRPC Streaming 리소스 누수

- **리스크**: 스트림이 적절히 종료되지 않으면 goroutine/메모리 누수 발생
- **대응**: context.Context 기반 종료, defer stream.Close(), `goleak` 테스트
- **심각도**: 중간 (context 기반 종료로 방지 가능)

### R5: Samsung NASA 프로토콜 검증

- **리스크**: 실제 삼성 시스템 에어컨 없이는 프로토콜 동작 검증 불가
- **대응**: 프로토콜 스펙 기반 mock 데이터로 파싱/직렬화 테스트, 실 환경 검증은 통합 테스트에서 수행
- **심각도**: 중간 (프로토콜 정의 기반 테스트 가능)

### R6: HTTP Server 포트 충돌

- **리스크**: 테스트 시 포트 충돌로 HTTP/WebSocket/gRPC 서버 시작 실패
- **대응**: `httptest.NewServer()` 사용, 또는 0 포트 바인딩으로 OS가 자동 할당
- **심각도**: 낮음 (테스트 패턴으로 해결)

---

## 5. 의존성 순서

```
구현 순서 (의존성 방향):

1. 각 Agent 패키지의 에러 정의            (의존성 없음)
2. 각 Agent 패키지의 Config 구조체 정의     (의존성 없음)
3. mqtt/client.go                       (Paho 래퍼, Config 의존)
4. mqtt/subscriber.go                   (client.go 의존)
5. mqtt/publisher.go                    (client.go 의존)
6. mqtt/agent.go                        (client.go, subscriber.go, publisher.go 의존)
7. http/client.go                       (Config 의존)
8. http/server.go                       (Config 의존)
9. http/agent.go                        (client.go, server.go 의존)
10. websocket/client.go                 (gorilla/websocket, Config 의존)
11. websocket/server.go                 (gorilla/websocket, Config 의존)
12. websocket/agent.go                  (client.go, server.go 의존)
13. grpc/client.go                      (grpc 패키지, Config 의존)
14. grpc/server.go                      (grpc 패키지, Config 의존)
15. grpc/agent.go                       (client.go, server.go 의존)
16. samsung/device.go                   (의존성 없음 - 모델 정의)
17. samsung/command.go                  (device.go, protocol 패키지 의존)
18. samsung/nasa.yaml                   (프로토콜 정의 파일)
19. samsung/agent.go                    (device.go, command.go, protocol 의존)

공통 의존성: 모든 agent.go는 internal/agent/ (SPEC-AGENT-001)에 의존
```

---

## 6. 테스트 전략

### 6.1 단위 테스트 구조

- 모든 테스트는 테이블 드리븐 방식으로 작성
- 외부 서비스 mock을 테스트 파일 내에 정의
- `testify/assert` 및 `testify/require` 사용
- 각 모듈의 테스트 파일은 해당 소스 파일과 같은 디렉토리에 위치

### 6.2 테스트 카테고리

| 카테고리 | 대상 | 검증 항목 |
|---------|------|---------|
| 생명주기 테스트 | 모든 Agent | Init, Start, Stop, Pause, Resume 상태 전이 |
| 연결 테스트 | MQTT, WebSocket, gRPC | 연결 수립, 끊김 감지, 자동 재연결 |
| 메시지 처리 테스트 | 모든 Agent | 수신/송신 메시지 변환, AgentStats 업데이트 |
| 설정 검증 테스트 | 모든 Agent | Config 유효성, 기본값, 잘못된 설정 거부 |
| 에러 처리 테스트 | 모든 Agent | sentinel 에러, errors.Is() 호환, 에러 시나리오 |
| 동시성 테스트 | WebSocket Server, gRPC | 다중 연결 관리, 쓰기 직렬화, data race 없음 |
| TypeRegistry 테스트 | 모든 Agent | init() 자동 등록, 팩토리 함수 동작 |
| TLS 테스트 | MQTT, HTTP, WebSocket, gRPC | TLS 연결, mTLS, 인증서 검증 |
| Samsung NASA 테스트 | NASA Agent | 프로토콜 파싱, 명령 직렬화, 디바이스 상태 관리 |

### 6.3 Mock 전략 상세

| Agent | Mock 대상 | Mock 방식 |
|-------|----------|----------|
| MQTT | MQTT 브로커 | Eclipse Paho 인터페이스 래핑 mock |
| HTTP Client | HTTP 서버 | `httptest.Server` |
| HTTP Server | HTTP 클라이언트 | `net/http` 표준 클라이언트 |
| WebSocket Client | WebSocket 서버 | `httptest.Server` + gorilla 업그레이드 |
| WebSocket Server | WebSocket 클라이언트 | gorilla `websocket.Dial()` |
| gRPC Client | gRPC 서버 | `bufconn` 패키지 인메모리 서버 |
| gRPC Server | gRPC 클라이언트 | `bufconn` 패키지 인메모리 클라이언트 |
| Samsung NASA | Transport | SPEC-AGENT-001 Transport 인터페이스 mock |

### 6.4 벤치마크 테스트

- `BenchmarkMQTTPublish` - MQTT 메시지 발행 성능
- `BenchmarkHTTPPoll` - HTTP 폴링 성능
- `BenchmarkWSBroadcast` - WebSocket 브로드캐스트 성능 (N개 연결)
- `BenchmarkGRPCUnary` - gRPC Unary RPC 성능
- `BenchmarkNASAParse` - NASA 프로토콜 파싱 성능

### 6.5 통합 테스트

통합 테스트는 `test/integration/` 디렉토리에서 수행한다:

- MQTT Agent: 실제 MQTT 브로커 (Docker 기반 Eclipse Mosquitto)에 연결하여 발행/구독 테스트
- HTTP Agent: 실제 HTTP 서버/클라이언트 통합 테스트
- WebSocket Agent: 실제 WebSocket 서버/클라이언트 통합 테스트
- gRPC Agent: 실제 gRPC 서버/클라이언트 통합 테스트
- Samsung NASA Agent: mock Transport 기반 프로토콜 통합 테스트
- Agent 전체 생명주기: TypeRegistry -> Create -> Init -> Start -> Process -> Stop -> Delete
