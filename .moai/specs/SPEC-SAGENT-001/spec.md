---
id: SPEC-SAGENT-001
version: "1.0.0"
status: draft
created: "2026-02-13"
updated: "2026-02-13"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-13 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-SAGENT-001: 표준 Agent 구현 - MQTT, HTTP, WebSocket, gRPC, Samsung NASA

## 1. Environment (환경)

### 1.1 시스템 개요

XFlow 플랫폼에서 표준 Agent는 사전 정의된 프로토콜(MQTT, HTTP, WebSocket, gRPC)을 통해 외부 시스템과 통신하는 구체 Agent 구현체이다. 모든 표준 Agent는 SPEC-AGENT-001에서 정의한 `Agent` 인터페이스를 구현하며 `BaseAgent`를 임베딩하여 공통 로직(생명주기, 상태 관리, Transport 래핑, 통계 수집)을 재사용한다.

본 SPEC은 Agent 시스템의 **구체 구현체**를 정의한다:

- **MQTT Client Agent**: Eclipse Paho Go 기반 MQTT 클라이언트 (QoS 0/1/2, 토픽 구독/발행)
- **HTTP Client/Server Agent**: HTTP 폴링 클라이언트 + 웹훅 수신 서버
- **WebSocket Client/Server Agent**: 양방향 실시간 통신 (자동 재연결, 핑/퐁 관리)
- **gRPC Client/Server Agent**: Protobuf 기반 고성능 서비스 간 통신 (Unary + Streaming)
- **Samsung NASA Manager Agent**: 삼성 시스템 에어컨 제어/모니터링 (RS-485/TCP, nasa.yaml 프로토콜 정의)
- **Custom Protocol Template**: 사용자 정의 프로토콜 Agent 생성 가이드 및 공통 유틸리티

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**:
  - `internal/agent/mqtt/` - MQTT Client Agent
  - `internal/agent/http/` - HTTP Client/Server Agent
  - `internal/agent/websocket/` - WebSocket Client/Server Agent
  - `internal/agent/grpc/` - gRPC Client/Server Agent
  - `internal/agent/samsung/` - Samsung NASA Manager Agent
- **Tier**: Tier 2 - 내부 실행 계층 (internal)
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-001): BaseAgent, Agent 인터페이스, Manager, Registry, TypeRegistry, AgentConfig, Transport, Protocol Definition Engine, AgentStats, AgentInfo, HealthStatus, Error Types
  - `pkg/lifecycle/` (SPEC-LIFE-001): State 인터페이스, 생명주기 상태 관리
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스 (Agent <-> Bridge 메시지 교환)
  - `pkg/xferr/` (SPEC-ERR-001): ErrorMessage, StatusEvent 타입
- **외부 의존성**:
  - `github.com/eclipse/paho.mqtt.golang` v1.4+ (MQTT 클라이언트)
  - `github.com/gorilla/websocket` v1.5+ (WebSocket 통신)
  - `google.golang.org/grpc` v1.60+ (gRPC 프레임워크)
  - `google.golang.org/protobuf` v1.32+ (Protocol Buffers)
  - `go.bug.st/serial` v1.6+ (Samsung NASA RS-485 시리얼 통신)
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`

### 1.3 설계 원칙

- **BaseAgent 임베딩**: 모든 표준 Agent는 `BaseAgent`를 임베딩하여 공통 로직(생명주기, 통계, Transport)을 재사용
- **TypeRegistry 등록**: 각 표준 Agent는 `init()` 함수에서 `TypeRegistry.RegisterType()`으로 자동 등록
- **Transport 추상화 활용**: 표준 Agent는 프레임워크의 Transport 인터페이스 위에 프로토콜별 연결 관리를 추가
- **프로토콜별 설정**: 각 Agent는 `AgentConfig.Metadata` 또는 전용 Config 구조체로 프로토콜별 설정을 관리
- **연결 복원력**: 모든 네트워크 기반 Agent는 지수 백오프 기반 자동 재연결을 지원
- **동시성 안전**: 발행/구독, 요청/응답 등 모든 작업이 goroutine-safe
- **관찰성 내장**: 연결 상태, 메시지 처리 결과, QoS 통계 등 추적 가능

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- MQTT Client Agent 구현 (발행/구독, QoS 0/1/2, TLS, 와일드카드 토픽)
- HTTP Client Agent 구현 (폴링, 요청/응답)
- HTTP Server Agent 구현 (웹훅 수신 엔드포인트)
- WebSocket Client Agent 구현 (양방향, 자동 재연결, 핑/퐁)
- WebSocket Server Agent 구현 (다중 클라이언트 연결 수용)
- gRPC Client Agent 구현 (Unary + Streaming RPC)
- gRPC Server Agent 구현 (서비스 등록, 스트리밍)
- Samsung NASA Manager Agent 구현 (nasa.yaml 프로토콜 정의, 디바이스 모델, 제어 명령)
- Custom Protocol Agent 생성 유틸리티
- 표준 Agent 타입의 TypeRegistry 등록
- 표준 Agent 전용 Error Types

**OUT OF SCOPE (별도 SPEC)**:
- Agent 프레임워크 (BaseAgent, Manager, Registry 등) - SPEC-AGENT-001
- System Agent 구현 (Event, Logger, File, Timer, Store) - SPEC-SYSAGENT-001
- Bridge 노드 구현 - SPEC-NODE-001
- Transport 인터페이스 및 Serial/TCP/UDP 구현 - SPEC-AGENT-001
- Protocol Definition Engine - SPEC-AGENT-001
- 관찰성 시스템 통합 구현 - SPEC-OBS-001

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-AGENT-001 | 의존 (프레임워크) | BaseAgent, Agent 인터페이스, Manager, Registry, TypeRegistry, Transport, Protocol Definition Engine |
| SPEC-LIFE-001 | 의존 | State 인터페이스, 생명주기 상태 전이 |
| SPEC-MSG-001 | 의존 | Message 인터페이스 (Agent <-> Bridge 메시지 교환) |
| SPEC-ERR-001 | 의존 | ErrorMessage, StatusEvent 타입 |
| SPEC-CFG-001 | 참조 | 표준 Agent 설정을 전역 설정에서 로드 |
| SPEC-OBS-001 | 소비자 | 표준 Agent 관찰성 (연결 상태, QoS 통계, 메시지 처리 메트릭) |
| SPEC-BRIDGE-001 | 소비자 | Bridge 노드가 표준 Agent를 참조하여 메시지 교환 |
| SPEC-ENGINE-001 | 소비자 | 플로우 엔진이 Agent Manager를 통해 표준 Agent 참조 |
| SPEC-API-001 | 소비자 | REST API가 표준 Agent 정보 조회/제어 |
| SPEC-CLI-001 | 소비자 | CLI가 표준 Agent 목록/상태 확인 |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: SPEC-AGENT-001의 `BaseAgent`, `Agent` 인터페이스, `TypeRegistry`, `AgentConfig`, `Transport` 인터페이스가 구현 완료되어 있다
- A2: SPEC-AGENT-001의 `AgentStats`, `AgentInfo`, `HealthStatus` 구조체가 사용 가능하다
- A3: Eclipse Paho Go v1.4+가 MQTT 3.1.1/5.0 프로토콜을 안정적으로 지원한다
- A4: gorilla/websocket v1.5+가 WebSocket 프로토콜의 핑/퐁 및 자동 재연결 기반을 제공한다
- A5: google.golang.org/grpc v1.60+가 Unary 및 Streaming RPC를 안정적으로 지원한다
- A6: Samsung NASA 프로토콜 정의는 nasa.yaml 파일로 사전 정의되어 있으며, Protocol Definition Engine으로 로드/파싱 가능하다

### 2.2 도메인 가정

- A7: 각 표준 Agent는 하나의 프로토콜에 특화되며, 다중 프로토콜 지원은 여러 Agent 인스턴스로 구성한다
- A8: MQTT Agent는 하나의 브로커에 연결되며, 다중 브로커 연결은 여러 Agent 인스턴스로 구성한다
- A9: HTTP Server Agent는 단일 엔드포인트를 리스닝하며, 다중 경로는 라우팅 규칙으로 처리한다
- A10: gRPC Agent의 .proto 파일은 사전 컴파일되어 Go 코드로 생성되어 있다
- A11: Samsung NASA 프로토콜은 RS-485 기반 마스터-슬레이브 방식이며, 에이전트가 마스터 역할을 수행한다
- A12: 표준 Agent의 메시지 포맷은 `pkg/message/` Message 인터페이스를 준수하며, Bridge Node를 통해 플로우와 교환한다

---

## 3. Requirements (요구사항)

### Module 1: MQTT Client Agent (P0)

#### REQ-SAGENT-001-01-01 (Ubiquitous) MQTTAgent 구조체 정의

시스템은 **항상** `MQTTAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `client paho.Client` - Eclipse Paho MQTT 클라이언트 내부 관리
- `subscriptions map[string]byte` - 토픽별 QoS 레벨 관리
- `mu sync.RWMutex` - 구독 목록 동시성 보호

#### REQ-SAGENT-001-01-02 (Ubiquitous) MQTTConfig 구조체 정의

시스템은 **항상** `MQTTConfig` 구조체를 제공해야 한다:

- `BrokerURL string` - MQTT 브로커 주소 (예: `tcp://broker:1883`, `ssl://broker:8883`)
- `ClientID string` - 클라이언트 고유 ID
- `Username string` - 인증 사용자명 (선택)
- `Password string` - 인증 비밀번호 (선택)
- `QoS byte` - 기본 QoS 레벨 (0, 1, 2)
- `CleanSession bool` - 클린 세션 여부 (기본: true)
- `KeepAlive time.Duration` - 킵얼라이브 주기 (기본: 60초)
- `ConnectTimeout time.Duration` - 연결 타임아웃 (기본: 30초)
- `AutoReconnect bool` - 자동 재연결 여부 (기본: true)
- `MaxReconnectInterval time.Duration` - 최대 재연결 간격 (기본: 5분)
- `TLS *TLSConfig` - TLS 설정 (nil이면 미사용)
- `WillTopic string` - 유언 메시지 토픽 (선택)
- `WillPayload []byte` - 유언 메시지 페이로드 (선택)
- `WillQoS byte` - 유언 메시지 QoS (선택)
- `WillRetain bool` - 유언 메시지 리테인 여부 (선택)

#### REQ-SAGENT-001-01-03 (Ubiquitous) TLSConfig 구조체 정의

시스템은 **항상** `TLSConfig` 구조체를 제공해야 한다:

- `CACert string` - CA 인증서 파일 경로
- `ClientCert string` - 클라이언트 인증서 파일 경로 (mTLS 시)
- `ClientKey string` - 클라이언트 키 파일 경로 (mTLS 시)
- `InsecureSkipVerify bool` - 서버 인증서 검증 생략 여부

#### REQ-SAGENT-001-01-04 (Event-Driven) MQTT 연결 수립

**WHEN** `MQTTAgent.Start(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. `MQTTConfig`로부터 Eclipse Paho 클라이언트 옵션 구성
2. TLS 설정이 있으면 `tls.Config` 적용
3. 유언 메시지 설정이 있으면 적용
4. 브로커에 연결 시도
5. 연결 성공 시 사전 등록된 토픽 구독
6. `AgentStats.MessagesReceived` 카운터 시작

#### REQ-SAGENT-001-01-05 (Ubiquitous) MQTT 토픽 구독

시스템은 **항상** 다음 구독 메서드를 제공해야 한다:

- `Subscribe(topic string, qos byte) error` - 단일 토픽 구독
- `SubscribeMultiple(topics map[string]byte) error` - 다중 토픽 구독
- `Unsubscribe(topics ...string) error` - 구독 해제

와일드카드 토픽 (`+`, `#`)을 지원해야 한다.

#### REQ-SAGENT-001-01-06 (Ubiquitous) MQTT 메시지 발행

시스템은 **항상** 다음 발행 메서드를 제공해야 한다:

- `Publish(topic string, qos byte, retained bool, payload []byte) error` - 메시지 발행

발행 완료 시 `AgentStats.MessagesSent` 카운터를 증가시켜야 한다.

#### REQ-SAGENT-001-01-07 (Event-Driven) MQTT 메시지 수신 처리

**WHEN** 구독 중인 토픽에서 메시지가 수신되면, **THEN** 다음을 수행해야 한다:

1. `AgentStats.MessagesReceived` 카운터 증가
2. `AgentStats.BytesRead` 카운터 증가 (페이로드 크기만큼)
3. 수신 메시지를 `pkg/message/` Message로 변환
4. Bridge Node에게 메시지 전달

#### REQ-SAGENT-001-01-08 (Event-Driven) MQTT 연결 끊김 시 자동 재연결

**WHEN** MQTT 브로커 연결이 끊기면, **THEN** Eclipse Paho의 `AutoReconnect` 기능을 활용하여 지수 백오프 기반 자동 재연결을 수행해야 한다. 재연결 성공 시 이전 구독을 자동 복원해야 한다.

#### REQ-SAGENT-001-01-09 (Event-Driven) MQTT Agent 정상 종료

**WHEN** `MQTTAgent.Stop(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. 모든 토픽 구독 해제
2. 유언 메시지가 설정되어 있으면 발행 (선택적)
3. MQTT 클라이언트 연결 종료 (Disconnect, quiesce 대기)
4. BaseAgent.Stop() 호출

#### REQ-SAGENT-001-01-10 (Unwanted) 잘못된 QoS 레벨 거부

시스템은 0, 1, 2 이외의 QoS 레벨로 구독/발행을 수행**하지 않아야 한다**. `ErrInvalidQoS` 에러를 반환해야 한다.

#### REQ-SAGENT-001-01-11 (Unwanted) 연결되지 않은 상태에서 발행 거부

시스템은 브로커에 연결되지 않은 상태에서 `Publish`를 수행**하지 않아야 한다**. `ErrNotConnected` 에러를 반환해야 한다.

---

### Module 2: HTTP Client Agent (P0)

#### REQ-SAGENT-001-02-01 (Ubiquitous) HTTPClientAgent 구조체 정의

시스템은 **항상** `HTTPClientAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `httpClient *http.Client` - Go 표준 HTTP 클라이언트
- `pollTicker *time.Ticker` - 폴링 타이머 (nil이면 폴링 미사용)

#### REQ-SAGENT-001-02-02 (Ubiquitous) HTTPClientConfig 구조체 정의

시스템은 **항상** `HTTPClientConfig` 구조체를 제공해야 한다:

- `BaseURL string` - 기본 URL (예: `https://api.example.com`)
- `Method string` - HTTP 메서드 (기본: GET)
- `Headers map[string]string` - 기본 헤더 (Authorization 등)
- `PollInterval time.Duration` - 폴링 주기 (0이면 비활성)
- `Timeout time.Duration` - 요청 타임아웃 (기본: 30초)
- `RetryCount int` - 재시도 횟수 (기본: 3)
- `RetryBackoff time.Duration` - 재시도 간 대기 시간 (기본: 1초)
- `TLS *TLSConfig` - TLS 설정 (nil이면 기본 TLS)

#### REQ-SAGENT-001-02-03 (Event-Driven) HTTP 폴링 시작

**WHEN** `HTTPClientAgent.Start(ctx)` 호출 시, PollInterval > 0이면 **THEN** 주기적으로 `BaseURL`에 HTTP 요청을 보내고 응답을 Message로 변환하여 Bridge Node에 전달해야 한다.

#### REQ-SAGENT-001-02-04 (Ubiquitous) HTTP 요청/응답 메서드

시스템은 **항상** 다음 메서드를 제공해야 한다:

- `Request(ctx context.Context, method string, path string, body []byte) (*HTTPResponse, error)` - HTTP 요청 실행
- `Get(ctx context.Context, path string) (*HTTPResponse, error)` - GET 요청 편의 메서드
- `Post(ctx context.Context, path string, body []byte) (*HTTPResponse, error)` - POST 요청 편의 메서드

#### REQ-SAGENT-001-02-05 (Ubiquitous) HTTPResponse 구조체 정의

시스템은 **항상** `HTTPResponse` 구조체를 제공해야 한다:

- `StatusCode int` - HTTP 상태 코드
- `Headers http.Header` - 응답 헤더
- `Body []byte` - 응답 본문
- `Duration time.Duration` - 요청 소요 시간

#### REQ-SAGENT-001-02-06 (Event-Driven) HTTP 요청 재시도

**WHEN** HTTP 요청이 네트워크 에러 또는 5xx 응답을 반환하면, **THEN** `RetryCount`만큼 지수 백오프 기반으로 재시도해야 한다.

#### REQ-SAGENT-001-02-07 (Unwanted) 잘못된 HTTP 설정 거부

시스템은 BaseURL이 비어있거나 유효하지 않은 URL인 경우 Agent를 초기화**하지 않아야 한다**. `ErrInvalidConfig` 에러를 반환해야 한다.

---

### Module 3: HTTP Server Agent (P0)

#### REQ-SAGENT-001-03-01 (Ubiquitous) HTTPServerAgent 구조체 정의

시스템은 **항상** `HTTPServerAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `server *http.Server` - Go 표준 HTTP 서버
- `mux *http.ServeMux` - 라우팅 멀티플렉서

#### REQ-SAGENT-001-03-02 (Ubiquitous) HTTPServerConfig 구조체 정의

시스템은 **항상** `HTTPServerConfig` 구조체를 제공해야 한다:

- `ListenAddr string` - 리스닝 주소 (예: `:8080`)
- `Path string` - 웹훅 수신 경로 (기본: `/webhook`)
- `Methods []string` - 허용 HTTP 메서드 (기본: ["POST"])
- `MaxBodySize int64` - 최대 요청 본문 크기 (기본: 1MB)
- `TLS *TLSConfig` - TLS 설정 (nil이면 HTTP)
- `ReadTimeout time.Duration` - 읽기 타임아웃 (기본: 30초)
- `WriteTimeout time.Duration` - 쓰기 타임아웃 (기본: 30초)

#### REQ-SAGENT-001-03-03 (Event-Driven) HTTP 서버 시작

**WHEN** `HTTPServerAgent.Start(ctx)` 호출 시, **THEN** 지정된 주소와 경로에서 HTTP 요청을 수신하기 시작해야 한다. 수신된 요청의 본문을 Message로 변환하여 Bridge Node에 전달해야 한다.

#### REQ-SAGENT-001-03-04 (Event-Driven) HTTP 서버 정상 종료

**WHEN** `HTTPServerAgent.Stop(ctx)` 호출 시, **THEN** `http.Server.Shutdown(ctx)`을 통해 진행 중인 요청 처리를 완료한 후 서버를 종료해야 한다.

#### REQ-SAGENT-001-03-05 (Unwanted) 최대 본문 크기 초과 거부

시스템은 `MaxBodySize`를 초과하는 요청 본문을 수신**하지 않아야 한다**. HTTP 413 (Payload Too Large) 응답을 반환해야 한다.

#### REQ-SAGENT-001-03-06 (Unwanted) 허용되지 않은 HTTP 메서드 거부

시스템은 `Methods` 목록에 포함되지 않은 HTTP 메서드를 수용**하지 않아야 한다**. HTTP 405 (Method Not Allowed) 응답을 반환해야 한다.

---

### Module 4: WebSocket Client Agent (P1)

#### REQ-SAGENT-001-04-01 (Ubiquitous) WSClientAgent 구조체 정의

시스템은 **항상** `WSClientAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `conn *websocket.Conn` - gorilla/websocket 연결
- `mu sync.Mutex` - 쓰기 작업 동시성 보호

#### REQ-SAGENT-001-04-02 (Ubiquitous) WSClientConfig 구조체 정의

시스템은 **항상** `WSClientConfig` 구조체를 제공해야 한다:

- `URL string` - WebSocket 서버 URL (예: `ws://host:port/path`, `wss://host:port/path`)
- `Headers http.Header` - 핸드셰이크 헤더 (Authorization 등)
- `PingInterval time.Duration` - 핑 전송 주기 (기본: 30초)
- `PongTimeout time.Duration` - 퐁 응답 타임아웃 (기본: 10초)
- `WriteTimeout time.Duration` - 쓰기 타임아웃 (기본: 10초)
- `MaxMessageSize int64` - 최대 수신 메시지 크기 (기본: 512KB)
- `AutoReconnect bool` - 자동 재연결 여부 (기본: true)
- `MaxReconnectInterval time.Duration` - 최대 재연결 간격 (기본: 5분)
- `TLS *TLSConfig` - TLS 설정 (nil이면 기본)

#### REQ-SAGENT-001-04-03 (Event-Driven) WebSocket 연결 수립

**WHEN** `WSClientAgent.Start(ctx)` 호출 시, **THEN** 다음을 수행해야 한다:

1. gorilla/websocket `Dialer`로 서버에 연결
2. 핑/퐁 핸들러 설정
3. 읽기 goroutine 시작 (수신 메시지 대기)
4. 핑 전송 goroutine 시작

#### REQ-SAGENT-001-04-04 (Ubiquitous) WebSocket 메시지 송수신

시스템은 **항상** 다음 메서드를 제공해야 한다:

- `Send(messageType int, data []byte) error` - 메시지 전송 (TextMessage, BinaryMessage)
- `SendText(text string) error` - 텍스트 메시지 전송 편의 메서드
- `SendBinary(data []byte) error` - 바이너리 메시지 전송 편의 메서드

#### REQ-SAGENT-001-04-05 (Event-Driven) WebSocket 메시지 수신 처리

**WHEN** WebSocket 서버로부터 메시지가 수신되면, **THEN** 수신 메시지를 `pkg/message/` Message로 변환하여 Bridge Node에 전달해야 한다. `AgentStats` 카운터를 업데이트해야 한다.

#### REQ-SAGENT-001-04-06 (Event-Driven) WebSocket 연결 끊김 시 자동 재연결

**WHEN** WebSocket 연결이 끊기면, **THEN** 지수 백오프 기반으로 자동 재연결을 시도해야 한다. 재연결 성공 시 읽기/핑 goroutine을 재시작해야 한다.

#### REQ-SAGENT-001-04-07 (Event-Driven) WebSocket 핑/퐁 관리

**WHEN** PingInterval 주기에 도달하면, **THEN** 핑 프레임을 전송해야 한다. PongTimeout 내에 퐁 응답이 없으면 연결을 끊고 자동 재연결을 시작해야 한다.

#### REQ-SAGENT-001-04-08 (Unwanted) 최대 메시지 크기 초과 거부

시스템은 `MaxMessageSize`를 초과하는 수신 메시지를 처리**하지 않아야 한다**. 연결을 정상적으로 닫아야 한다.

---

### Module 5: WebSocket Server Agent (P1)

#### REQ-SAGENT-001-05-01 (Ubiquitous) WSServerAgent 구조체 정의

시스템은 **항상** `WSServerAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `upgrader websocket.Upgrader` - HTTP -> WebSocket 업그레이드
- `connections map[string]*websocket.Conn` - 활성 연결 관리
- `mu sync.RWMutex` - 연결 목록 동시성 보호

#### REQ-SAGENT-001-05-02 (Ubiquitous) WSServerConfig 구조체 정의

시스템은 **항상** `WSServerConfig` 구조체를 제공해야 한다:

- `ListenAddr string` - 리스닝 주소 (예: `:9090`)
- `Path string` - WebSocket 엔드포인트 경로 (기본: `/ws`)
- `MaxConnections int` - 최대 동시 연결 수 (기본: 100)
- `MaxMessageSize int64` - 최대 수신 메시지 크기 (기본: 512KB)
- `ReadBufferSize int` - 읽기 버퍼 크기 (기본: 1024)
- `WriteBufferSize int` - 쓰기 버퍼 크기 (기본: 1024)
- `CheckOrigin func(r *http.Request) bool` - Origin 검증 함수 (nil이면 모든 Origin 허용)

#### REQ-SAGENT-001-05-03 (Event-Driven) WebSocket 서버 시작

**WHEN** `WSServerAgent.Start(ctx)` 호출 시, **THEN** 지정된 주소에서 WebSocket 업그레이드 요청을 수신하기 시작해야 한다. 각 연결별로 읽기 goroutine을 생성해야 한다.

#### REQ-SAGENT-001-05-04 (Ubiquitous) WebSocket 서버 브로드캐스트

시스템은 **항상** 다음 메서드를 제공해야 한다:

- `Broadcast(messageType int, data []byte) error` - 모든 연결에 메시지 전송
- `SendTo(connID string, messageType int, data []byte) error` - 특정 연결에 메시지 전송
- `ActiveConnections() int` - 현재 활성 연결 수

#### REQ-SAGENT-001-05-05 (Event-Driven) WebSocket 클라이언트 연결/해제 이벤트

**WHEN** 새 클라이언트가 연결되거나 기존 클라이언트가 해제되면, **THEN** 연결 관리 맵을 업데이트하고 StatusEvent를 발행해야 한다.

#### REQ-SAGENT-001-05-06 (Unwanted) 최대 연결 수 초과 거부

시스템은 `MaxConnections`를 초과하는 새 연결을 수용**하지 않아야 한다**. HTTP 503 (Service Unavailable) 응답을 반환해야 한다.

---

### Module 6: gRPC Client Agent (P1)

#### REQ-SAGENT-001-06-01 (Ubiquitous) GRPCClientAgent 구조체 정의

시스템은 **항상** `GRPCClientAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `conn *grpc.ClientConn` - gRPC 클라이언트 연결
- `dialOpts []grpc.DialOption` - gRPC 다이얼 옵션

#### REQ-SAGENT-001-06-02 (Ubiquitous) GRPCClientConfig 구조체 정의

시스템은 **항상** `GRPCClientConfig` 구조체를 제공해야 한다:

- `Target string` - gRPC 서버 주소 (예: `localhost:50051`)
- `ServiceName string` - 대상 서비스 이름
- `TLS *TLSConfig` - TLS 설정 (nil이면 insecure)
- `MaxRetries int` - 최대 재시도 횟수 (기본: 3)
- `Timeout time.Duration` - 요청 타임아웃 (기본: 30초)
- `KeepAlive *GRPCKeepAliveConfig` - 킵얼라이브 설정

#### REQ-SAGENT-001-06-03 (Ubiquitous) GRPCKeepAliveConfig 구조체 정의

시스템은 **항상** `GRPCKeepAliveConfig` 구조체를 제공해야 한다:

- `Time time.Duration` - 킵얼라이브 핑 간격 (기본: 30초)
- `Timeout time.Duration` - 킵얼라이브 타임아웃 (기본: 10초)
- `PermitWithoutStream bool` - 스트림 없이도 킵얼라이브 허용 (기본: true)

#### REQ-SAGENT-001-06-04 (Event-Driven) gRPC 연결 수립

**WHEN** `GRPCClientAgent.Start(ctx)` 호출 시, **THEN** gRPC 서버에 연결을 수립해야 한다. TLS 설정, 킵얼라이브, 인터셉터 등을 적용해야 한다.

#### REQ-SAGENT-001-06-05 (Ubiquitous) gRPC Unary RPC 호출

시스템은 **항상** 다음 메서드를 제공해야 한다:

- `Invoke(ctx context.Context, method string, req any, resp any) error` - Unary RPC 호출
- `InvokeRaw(ctx context.Context, method string, payload []byte) ([]byte, error)` - 원시 바이트 기반 Unary RPC

#### REQ-SAGENT-001-06-06 (Ubiquitous) gRPC Streaming RPC 지원

시스템은 **항상** 다음 스트리밍 메서드를 제공해야 한다:

- `ServerStream(ctx context.Context, method string, req any) (GRPCStream, error)` - 서버 스트리밍 RPC
- `ClientStream(ctx context.Context, method string) (GRPCStream, error)` - 클라이언트 스트리밍 RPC
- `BidiStream(ctx context.Context, method string) (GRPCStream, error)` - 양방향 스트리밍 RPC

#### REQ-SAGENT-001-06-07 (Ubiquitous) GRPCStream 인터페이스 정의

시스템은 **항상** `GRPCStream` 인터페이스를 제공해야 한다:

- `Send(msg any) error` - 메시지 전송
- `Recv() (any, error)` - 메시지 수신
- `Close() error` - 스트림 종료

#### REQ-SAGENT-001-06-08 (Event-Driven) gRPC 연결 상태 모니터링

**WHEN** gRPC 연결 상태가 변경되면 (CONNECTING, READY, IDLE, TRANSIENT_FAILURE, SHUTDOWN), **THEN** `HealthStatus`를 업데이트하고 StatusEvent를 발행해야 한다.

---

### Module 7: gRPC Server Agent (P1)

#### REQ-SAGENT-001-07-01 (Ubiquitous) GRPCServerAgent 구조체 정의

시스템은 **항상** `GRPCServerAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `server *grpc.Server` - gRPC 서버
- `listener net.Listener` - TCP 리스너

#### REQ-SAGENT-001-07-02 (Ubiquitous) GRPCServerConfig 구조체 정의

시스템은 **항상** `GRPCServerConfig` 구조체를 제공해야 한다:

- `ListenAddr string` - 리스닝 주소 (예: `:50051`)
- `TLS *TLSConfig` - TLS 설정 (nil이면 insecure)
- `MaxRecvMsgSize int` - 최대 수신 메시지 크기 (기본: 4MB)
- `MaxSendMsgSize int` - 최대 송신 메시지 크기 (기본: 4MB)
- `MaxConcurrentStreams uint32` - 최대 동시 스트림 수 (기본: 100)

#### REQ-SAGENT-001-07-03 (Event-Driven) gRPC 서버 시작

**WHEN** `GRPCServerAgent.Start(ctx)` 호출 시, **THEN** 지정된 주소에서 gRPC 요청을 수신하기 시작해야 한다. 등록된 서비스 핸들러를 통해 수신된 RPC 요청을 Message로 변환하여 Bridge Node에 전달해야 한다.

#### REQ-SAGENT-001-07-04 (Event-Driven) gRPC 서버 정상 종료

**WHEN** `GRPCServerAgent.Stop(ctx)` 호출 시, **THEN** `grpc.Server.GracefulStop()`을 통해 진행 중인 RPC 처리를 완료한 후 서버를 종료해야 한다. Context deadline 초과 시 `grpc.Server.Stop()`으로 강제 종료해야 한다.

#### REQ-SAGENT-001-07-05 (Ubiquitous) gRPC 서비스 핸들러 등록

시스템은 **항상** 다음 메서드를 제공해야 한다:

- `RegisterService(desc *grpc.ServiceDesc, impl any)` - gRPC 서비스 핸들러 등록

---

### Module 8: Samsung NASA Manager Agent (P2)

#### REQ-SAGENT-001-08-01 (Ubiquitous) NASAAgent 구조체 정의

시스템은 **항상** `NASAAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- `devices map[string]*NASADevice` - 디바이스 목록 관리
- `protocol *protocol.ProtocolDefinition` - nasa.yaml 기반 프로토콜 정의
- `mu sync.RWMutex` - 디바이스 목록 동시성 보호

#### REQ-SAGENT-001-08-02 (Ubiquitous) NASAConfig 구조체 정의

시스템은 **항상** `NASAConfig` 구조체를 제공해야 한다:

- `TransportType string` - 통신 방식 ("serial" 또는 "tcp")
- `SerialPort string` - 시리얼 포트 (예: `/dev/ttyUSB0`) - TransportType이 serial일 때
- `BaudRate int` - 보레이트 (기본: 9600)
- `TCPAddr string` - TCP 주소 (예: `192.168.1.100:4196`) - TransportType이 tcp일 때
- `ProtocolFile string` - NASA 프로토콜 정의 파일 경로 (기본: 내장 nasa.yaml)
- `PollInterval time.Duration` - 디바이스 상태 폴링 주기 (기본: 30초)
- `DeviceAddresses []byte` - 모니터링 대상 디바이스 주소 목록

#### REQ-SAGENT-001-08-03 (Ubiquitous) NASADevice 구조체 정의

시스템은 **항상** `NASADevice` 구조체를 제공해야 한다:

- `Address byte` - 디바이스 RS-485 주소
- `Type string` - 디바이스 타입 ("indoor", "outdoor", "controller")
- `Online bool` - 온라인 상태
- `LastSeen time.Time` - 마지막 응답 시각
- `State *NASADeviceState` - 현재 디바이스 상태

#### REQ-SAGENT-001-08-04 (Ubiquitous) NASADeviceState 구조체 정의

시스템은 **항상** `NASADeviceState` 구조체를 제공해야 한다:

- `Power bool` - 전원 상태
- `Mode string` - 운전 모드 ("cool", "heat", "dry", "fan", "auto")
- `TargetTemp float32` - 설정 온도
- `CurrentTemp float32` - 현재 실내 온도
- `FanSpeed string` - 풍량 ("auto", "low", "medium", "high", "turbo")
- `ErrorCode int` - 에러 코드 (0이면 정상)

#### REQ-SAGENT-001-08-05 (Event-Driven) NASA 디바이스 상태 폴링

**WHEN** PollInterval 주기에 도달하면, **THEN** 등록된 모든 디바이스 주소에 상태 조회 명령을 전송하고, 응답을 파싱하여 `NASADevice.State`를 업데이트해야 한다.

#### REQ-SAGENT-001-08-06 (Ubiquitous) NASA 제어 명령

시스템은 **항상** 다음 제어 메서드를 제공해야 한다:

- `SetPower(address byte, on bool) error` - 전원 제어
- `SetMode(address byte, mode string) error` - 운전 모드 변경
- `SetTemperature(address byte, temp float32) error` - 목표 온도 설정
- `SetFanSpeed(address byte, speed string) error` - 풍량 변경
- `GetDeviceState(address byte) (*NASADeviceState, error)` - 디바이스 상태 조회
- `ListDevices() []*NASADevice` - 전체 디바이스 목록

#### REQ-SAGENT-001-08-07 (Event-Driven) NASA 프로토콜 메시지 파싱

**WHEN** Transport에서 데이터가 수신되면, **THEN** Protocol Definition Engine을 통해 NASA 프로토콜 메시지를 파싱하고, 대상 디바이스 상태를 업데이트해야 한다.

#### REQ-SAGENT-001-08-08 (Unwanted) 유효하지 않은 디바이스 주소 거부

시스템은 등록되지 않은 디바이스 주소에 대한 제어 명령을 수행**하지 않아야 한다**. `ErrDeviceNotFound` 에러를 반환해야 한다.

#### REQ-SAGENT-001-08-09 (Unwanted) 유효하지 않은 운전 모드/풍량 거부

시스템은 유효하지 않은 운전 모드 또는 풍량 값으로 제어 명령을 수행**하지 않아야 한다**. `ErrInvalidCommand` 에러를 반환해야 한다.

---

### Module 9: Custom Protocol Template (P2)

#### REQ-SAGENT-001-09-01 (Ubiquitous) CustomAgent 구조체 정의

시스템은 **항상** `CustomAgent` 구조체를 제공해야 한다:

- `BaseAgent` 임베딩 (SPEC-AGENT-001)
- Protocol Definition Engine과 Transport를 조합한 범용 Agent

#### REQ-SAGENT-001-09-02 (Ubiquitous) CustomAgent 생성 유틸리티

시스템은 **항상** 다음 팩토리 함수를 제공해야 한다:

- `NewCustomAgent(config AgentConfig) (*CustomAgent, error)` - AgentConfig의 Transport + ProtocolFile 조합으로 CustomAgent 생성

#### REQ-SAGENT-001-09-03 (Event-Driven) CustomAgent 데이터 수신 처리

**WHEN** Transport에서 데이터가 수신되면, **THEN** Protocol Definition Engine을 통해 데이터를 파싱하고, 결과를 Message로 변환하여 Bridge Node에 전달해야 한다.

#### REQ-SAGENT-001-09-04 (Event-Driven) CustomAgent 데이터 송신 처리

**WHEN** Bridge Node로부터 Message가 수신되면, **THEN** Protocol Definition Engine을 통해 메시지를 바이트 데이터로 직렬화하고, Transport를 통해 전송해야 한다.

---

### Module 10: 표준 Agent TypeRegistry 등록 (P0)

#### REQ-SAGENT-001-10-01 (Ubiquitous) 표준 Agent 타입 자동 등록

시스템은 **항상** 각 표준 Agent 패키지의 `init()` 함수에서 `TypeRegistry.RegisterType()`을 호출하여 다음 타입을 자동 등록해야 한다:

- `"mqtt"` - MQTTAgent 팩토리
- `"http-client"` - HTTPClientAgent 팩토리
- `"http-server"` - HTTPServerAgent 팩토리
- `"websocket-client"` - WSClientAgent 팩토리
- `"websocket-server"` - WSServerAgent 팩토리
- `"grpc-client"` - GRPCClientAgent 팩토리
- `"grpc-server"` - GRPCServerAgent 팩토리
- `"nasa"` - NASAAgent 팩토리

#### REQ-SAGENT-001-10-02 (Ubiquitous) AgentFactory 구현

시스템은 **항상** 각 표준 Agent 타입에 대해 `AgentFactory` 함수를 제공해야 한다:

- AgentConfig에서 프로토콜별 설정을 파싱
- 해당 Agent 인스턴스를 생성 및 초기화
- 에러 시 descriptive 에러 메시지 반환

---

### Module 11: 표준 Agent Error Types (P0)

#### REQ-SAGENT-001-11-01 (Ubiquitous) 표준 Agent Sentinel 에러 정의

시스템은 **항상** 다음 sentinel 에러 변수를 제공해야 한다:

**MQTT 에러**:
- `ErrInvalidQoS` - 유효하지 않은 QoS 레벨 (0, 1, 2 외)
- `ErrNotConnected` - 브로커에 연결되지 않은 상태
- `ErrSubscriptionFailed` - 토픽 구독 실패
- `ErrPublishFailed` - 메시지 발행 실패
- `ErrBrokerConnectionFailed` - 브로커 연결 실패

**HTTP 에러**:
- `ErrHTTPRequestFailed` - HTTP 요청 실패
- `ErrHTTPServerStartFailed` - HTTP 서버 시작 실패
- `ErrPayloadTooLarge` - 요청 본문 크기 초과
- `ErrMethodNotAllowed` - 허용되지 않은 HTTP 메서드

**WebSocket 에러**:
- `ErrWSConnectionFailed` - WebSocket 연결 실패
- `ErrWSWriteFailed` - WebSocket 쓰기 실패
- `ErrWSMaxConnections` - 최대 연결 수 초과
- `ErrWSMessageTooLarge` - 메시지 크기 초과

**gRPC 에러**:
- `ErrGRPCDialFailed` - gRPC 서버 연결 실패
- `ErrGRPCInvokeFailed` - gRPC RPC 호출 실패
- `ErrGRPCStreamFailed` - gRPC 스트리밍 실패
- `ErrGRPCServerStartFailed` - gRPC 서버 시작 실패

**Samsung NASA 에러**:
- `ErrDeviceNotFound` - 디바이스 주소 미발견
- `ErrInvalidCommand` - 유효하지 않은 제어 명령
- `ErrDeviceOffline` - 디바이스 오프라인
- `ErrNASAProtocolError` - NASA 프로토콜 파싱 에러

**공통 에러**:
- `ErrInvalidConfig` - 유효하지 않은 Agent 설정
- `ErrTLSConfigFailed` - TLS 설정 실패

#### REQ-SAGENT-001-11-02 (Ubiquitous) 에러 호환성

시스템은 **항상** 모든 sentinel 에러가 `errors.Is()` 비교와 호환되어야 한다.

---

## 4. Specifications (사양)

### 4.1 파일 구조

```
internal/agent/
├── mqtt/
│   ├── agent.go          # MQTTAgent 구현 (Agent 인터페이스)
│   ├── client.go         # MQTT 클라이언트 래퍼 (Eclipse Paho)
│   ├── subscriber.go     # MQTT 구독 관리
│   ├── publisher.go      # MQTT 발행 관리
│   └── mqtt_test.go      # MQTT Agent 테스트
│
├── http/
│   ├── agent.go          # HTTP Agent 구현 (Client + Server)
│   ├── client.go         # HTTP 클라이언트 (폴링/요청-응답)
│   ├── server.go         # HTTP 수신 엔드포인트 (웹훅)
│   └── http_test.go      # HTTP Agent 테스트
│
├── websocket/
│   ├── agent.go          # WebSocket Agent 구현 (Client + Server)
│   ├── client.go         # WebSocket 클라이언트
│   ├── server.go         # WebSocket 서버
│   └── ws_test.go        # WebSocket Agent 테스트
│
├── grpc/
│   ├── agent.go          # gRPC Agent 구현 (Client + Server)
│   ├── client.go         # gRPC 클라이언트
│   ├── server.go         # gRPC 서버
│   └── grpc_test.go      # gRPC Agent 테스트
│
└── samsung/
    ├── agent.go          # Samsung NASA Agent 구현
    ├── nasa.yaml         # NASA 프로토콜 정의 (설정 파일)
    ├── device.go         # 실내기/실외기 디바이스 모델
    ├── command.go        # 제어 명령 (온도, 모드, 풍량 등)
    └── samsung_test.go   # Samsung NASA Agent 테스트
```

### 4.2 BaseAgent 임베딩 패턴

모든 표준 Agent는 다음 패턴을 따른다:

```go
type MQTTAgent struct {
    *agent.BaseAgent  // SPEC-AGENT-001 BaseAgent 임베딩
    // 프로토콜별 필드
}
```

이로써 Init, Start, Stop, Pause, Resume, Health, ID, Name, Type, Info, Stats 등의 공통 메서드를 BaseAgent에서 상속받고, 프로토콜별로 Process(), Configure() 등을 오버라이드한다.

### 4.3 TypeRegistry 등록 패턴

```go
func init() {
    agent.DefaultTypeRegistry.RegisterType("mqtt", func(config agent.AgentConfig) (agent.Agent, error) {
        a := &MQTTAgent{}
        if err := a.Init(config); err != nil {
            return nil, err
        }
        return a, nil
    })
}
```

### 4.4 메시지 변환 규칙

표준 Agent가 외부에서 수신한 프로토콜 데이터를 `pkg/message/` Message로 변환할 때:

- MQTT: `msg.Payload` -> Message.Payload, `msg.Topic` -> Message.Metadata["mqtt.topic"]
- HTTP: `resp.Body` -> Message.Payload, `resp.StatusCode` -> Message.Metadata["http.status"]
- WebSocket: `message.Data` -> Message.Payload, `message.Type` -> Message.Metadata["ws.type"]
- gRPC: `response bytes` -> Message.Payload, `method` -> Message.Metadata["grpc.method"]
- NASA: `ParsedMessage` -> Message.Payload, `device.Address` -> Message.Metadata["nasa.device"]

---

## 5. Traceability (추적성)

| REQ ID | Module | 파일 |
|--------|--------|------|
| REQ-SAGENT-001-01-* | MQTT Client Agent | internal/agent/mqtt/ |
| REQ-SAGENT-001-02-* | HTTP Client Agent | internal/agent/http/client.go |
| REQ-SAGENT-001-03-* | HTTP Server Agent | internal/agent/http/server.go |
| REQ-SAGENT-001-04-* | WebSocket Client Agent | internal/agent/websocket/client.go |
| REQ-SAGENT-001-05-* | WebSocket Server Agent | internal/agent/websocket/server.go |
| REQ-SAGENT-001-06-* | gRPC Client Agent | internal/agent/grpc/client.go |
| REQ-SAGENT-001-07-* | gRPC Server Agent | internal/agent/grpc/server.go |
| REQ-SAGENT-001-08-* | Samsung NASA Agent | internal/agent/samsung/ |
| REQ-SAGENT-001-09-* | Custom Protocol Template | internal/agent/ (BaseAgent 활용) |
| REQ-SAGENT-001-10-* | TypeRegistry 등록 | 각 Agent 패키지의 init() |
| REQ-SAGENT-001-11-* | Error Types | 각 Agent 패키지의 errors.go |
