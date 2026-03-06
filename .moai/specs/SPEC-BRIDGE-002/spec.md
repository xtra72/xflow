---
id: SPEC-BRIDGE-002
version: "1.0.0"
status: completed
created: "2026-03-05"
updated: "2026-03-06"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-05 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-BRIDGE-002: Agent-Specific Dedicated Bridge Implementation - 에이전트별 전용 브릿지 어댑터

## 1. Environment (환경)

### 1.1 시스템 개요

SPEC-BRIDGE-001에서 구현한 범용 BridgeNode는 4가지 통신 모드(In, Out, InOut, Request-Reply)와 DefaultTransformer를 통해 Agent-Flow 간 기본적인 메시지 교환을 지원한다. 그러나 현재 구조는 에이전트 타입에 무관한 범용 변환(bytes <-> Message)만 수행하므로, 각 프로토콜의 고유한 특성을 활용하지 못한다.

본 SPEC-BRIDGE-002는 에이전트 타입별 전용 BridgeAdapter를 도입하여, 프로토콜 고유의 설정 검증, 메시지 변환, 제어 메시지 처리를 지원하는 확장 아키텍처를 정의한다.

**SPEC-BRIDGE-001과의 관계**: SPEC-BRIDGE-001이 정의한 BridgeNode, BridgeConfig, BridgeTransformer, CorrelationTracker는 그대로 유지한다. 본 SPEC은 기존 BridgeTransformer를 확장하는 BridgeAdapter 인터페이스와 Adapter Registry를 추가하여, 에이전트 타입을 인식하는 프로토콜 전용 변환 계층을 구축한다.

**기존 System Agent BridgeHandler와의 관계**: `store_bridge.go`, `timer_bridge.go`, `logger_bridge.go`에 구현된 BridgeHandler 패턴은 메타데이터 기반 연산 디스패치 방식이다. 본 SPEC의 BridgeAdapter는 이 패턴을 통합하여 일관된 어댑터 인터페이스로 제공한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **백엔드 패키지 경로**:
  - `internal/node/` (bridge_adapter.go, bridge_adapter_registry.go)
  - `internal/node/adapter/` (mqtt.go, modbus.go, http.go, system.go)
- **프론트엔드 경로**:
  - `web/src/config/nodeSchemas.ts` (에이전트 타입별 설정 스키마)
  - `web/src/components/property/` (에이전트별 설정 패널 컴포넌트)
- **프론트엔드 기술**: React 19, TypeScript 5.x, Tailwind CSS v4, React Flow
- **의존 패키지**:
  - `internal/node/` (SPEC-BRIDGE-001): BridgeNode, BridgeConfig, BridgeTransformer, CorrelationTracker
  - `internal/agent/mqtt/` (SPEC-MQTT-001): MQTTAgent, SubscriberAgent 인터페이스
  - `internal/agent/modbus/` (SPEC-MODBUS-001): MODBUSAgent, 레지스터 캐시
  - `internal/agent/modbusserver/` (SPEC-MODBUS-002): MODBUSServerAgent, 레지스터 맵
  - `internal/agent/http/`: HTTPAgent
  - `internal/agent/system/` (SPEC-STORE-001, SPEC-TIMER-001): BridgeHandler, TimerBridgeHandler
  - `internal/modbus/` (공유 타입 변환): MODBUS 데이터 타입 변환 유틸리티
  - `pkg/message/` (SPEC-MSG-001): Message, Payload, Metadata 인터페이스
  - `pkg/flow/`: AgentRef, BridgeDirection
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**: sync.RWMutex (Adapter Registry 보호)

### 1.3 설계 원칙

- **기존 호환성 보장**: BridgeAdapter가 등록되지 않은 에이전트 타입은 기존 DefaultTransformer로 폴백하여 하위 호환성을 유지한다
- **단일 책임 원칙**: 각 어댑터는 해당 프로토콜의 변환 로직만 담당하며, BridgeNode의 통신 모드 관리에는 관여하지 않는다
- **Registry 패턴**: 에이전트 타입 문자열 키로 어댑터를 등록/조회하는 레지스트리를 통해 확장성을 보장한다
- **System Agent 통합**: 기존 BridgeHandler 패턴(store_bridge.go, timer_bridge.go, logger_bridge.go)을 SystemAdapter로 통합하여 일관된 인터페이스를 제공한다

---

## 2. Assumptions (가정)

### 2.1 프로토콜 가정

- MQTT 에이전트는 SubscriberAgent 인터페이스를 구현하며, 토픽 기반 메시지 라우팅을 지원한다
- Modbus 에이전트는 레지스터 맵 기반의 데이터 접근 모델을 사용하며, FC01-FC06/FC15-FC16 기능 코드를 지원한다
- HTTP 에이전트는 RESTful 패턴의 요청/응답 모델을 따른다
- System Agent는 메타데이터 접두사 기반 연산 디스패치 패턴을 사용한다

### 2.2 아키텍처 가정

- BridgeNode는 Init 시점에 연결된 에이전트의 타입을 식별할 수 있다 (AgentRef.AgentType 필드 사용)
- 하나의 BridgeNode는 하나의 에이전트 타입에만 바인딩된다
- Adapter Registry는 프로세스 전역 싱글턴으로 관리되며, 서버 시작 시 모든 어댑터가 등록된다
- 프론트엔드는 에이전트 타입 정보를 기반으로 동적 설정 스키마를 렌더링한다

### 2.3 성능 가정

- BridgeAdapter의 메시지 변환은 단일 메시지당 1ms 이내에 완료되어야 한다
- Adapter Registry 조회는 O(1) 시간 복잡도를 가져야 한다 (map 기반)
- 에이전트별 설정 검증은 BridgeNode Init 시점에 1회만 수행한다

---

## 3. Requirements (요구사항)

### Module 1: BridgeAdapter 인터페이스 및 Registry (P0)

#### REQ-BRIDGE-002-001: BridgeAdapter 인터페이스 정의

시스템은 **항상** 다음 메서드를 포함하는 BridgeAdapter 인터페이스를 제공해야 한다:

- `Validate(config BridgeConfig) error`: 에이전트 타입에 특화된 설정 유효성 검증
- `DefaultConfig() BridgeConfig`: 에이전트 타입에 적합한 기본 BridgeConfig 반환
- `TransformToFlow(data []byte, meta AgentMeta) (message.Message, error)`: 프로토콜 인식 에이전트->플로우 변환
- `TransformToAgent(msg message.Message) ([]byte, AgentMeta, error)`: 프로토콜 인식 플로우->에이전트 변환
- `HandleControl(msg message.Message) error`: 에이전트 고유 제어 메시지 처리

```
AgentMeta 구조체:
  - Topic string      // MQTT 토픽, HTTP URL 경로 등
  - QoS int           // MQTT QoS 레벨
  - Retained bool     // MQTT retained 플래그
  - Headers map[string]string  // HTTP 헤더
  - StatusCode int    // HTTP 상태 코드
  - ContentType string // HTTP Content-Type
  - UnitID uint8      // Modbus 디바이스 주소
  - FunctionCode uint8 // Modbus 기능 코드
  - RegisterAddr uint16 // Modbus 레지스터 시작 주소
  - RegisterCount uint16 // Modbus 레지스터 개수
```

#### REQ-BRIDGE-002-002: Adapter Registry

시스템은 **항상** 에이전트 타입 문자열을 키로 BridgeAdapter를 등록하고 조회할 수 있는 AdapterRegistry를 제공해야 한다:

- `RegisterAdapter(agentType string, adapter BridgeAdapter)`: 어댑터 등록
- `GetAdapter(agentType string) (BridgeAdapter, bool)`: 어댑터 조회
- 등록되지 않은 에이전트 타입에 대해서는 `(nil, false)`를 반환한다

#### REQ-BRIDGE-002-003: BridgeNode와 Adapter 통합

**WHEN** BridgeNode가 Init()을 수행할 때 **THEN** 연결된 에이전트의 타입으로 AdapterRegistry에서 어댑터를 조회하고, 어댑터가 존재하면 해당 어댑터의 Validate와 TransformToFlow/TransformToAgent를 BridgeTransformer 대신 사용해야 한다.

**IF** 에이전트 타입에 대한 어댑터가 등록되어 있지 않다면 **THEN** 시스템은 기존 DefaultTransformer를 사용하여 하위 호환성을 유지해야 한다.

#### REQ-BRIDGE-002-004: DefaultAdapter

시스템은 **항상** 기존 DefaultTransformer와 동일한 동작을 수행하는 DefaultAdapter를 제공해야 한다. DefaultAdapter는 BridgeAdapter 인터페이스를 구현하되, 프로토콜 특화 로직 없이 범용 bytes <-> Message 변환을 수행한다.

---

### Module 2: MQTT 전용 어댑터 (P0)

#### REQ-BRIDGE-002-010: MQTT 토픽 라우팅

**WHEN** MQTT 에이전트로부터 메시지를 수신할 때 **THEN** MQTTAdapter는 수신 토픽을 Message의 Metadata에 `mqtt.topic` 키로 저장하고, 발행 시에는 Metadata의 `mqtt.topic` 값을 발행 토픽으로 사용해야 한다.

#### REQ-BRIDGE-002-011: MQTT QoS 레벨 관리

**WHEN** MQTT 메시지를 변환할 때 **THEN** MQTTAdapter는 수신 QoS를 `mqtt.qos` 메타데이터에 저장하고, 발행 시에는 설정된 QoS 또는 메시지별 `mqtt.qos` 메타데이터 값을 사용해야 한다.

#### REQ-BRIDGE-002-012: MQTT Retained 메시지 지원

**WHEN** MQTT 메시지가 retained 플래그를 가질 때 **THEN** MQTTAdapter는 `mqtt.retained` 메타데이터에 "true"를 저장해야 하며, 발행 시 `mqtt.retained` 메타데이터가 "true"이면 retained 플래그를 설정해야 한다.

#### REQ-BRIDGE-002-013: MQTT 토픽 템플릿

**가능하면** MQTTAdapter는 발행 토픽에 메시지 필드 보간을 지원해야 한다. 예: `devices/{device_id}/data` 형식에서 `{device_id}`를 Payload의 `device_id` 값으로 치환한다.

#### REQ-BRIDGE-002-014: MQTT 와일드카드 구독

**WHEN** BridgeConfig의 Topics에 MQTT 와일드카드(`#`, `+`)가 포함된 토픽이 있을 때 **THEN** MQTTAdapter는 와일드카드 구독을 에이전트에 위임하고, 수신된 각 메시지에 실제 토픽을 메타데이터에 기록해야 한다.

#### REQ-BRIDGE-002-015: MQTT 설정 검증

**WHEN** MQTTAdapter.Validate()가 호출될 때 **THEN** 다음을 검증해야 한다:
- Topics 필드에 최소 1개 이상의 토픽이 지정되어 있는지 (In/InOut 방향일 때)
- QoS 값이 0, 1, 2 중 하나인지
- 토픽 문자열이 유효한 MQTT 토픽 형식인지

---

### Module 3: Modbus 전용 어댑터 (P0)

#### REQ-BRIDGE-002-020: Modbus 레지스터 맵 변환

**WHEN** Modbus 에이전트로부터 레지스터 데이터를 수신할 때 **THEN** ModbusAdapter는 레지스터 맵 설정(주소, 개수, 데이터 타입, 바이트 오더)에 따라 uint16 레지스터 값을 Go 네이티브 타입(int16, float32, uint32 등)으로 변환하여 Message Payload에 저장해야 한다.

#### REQ-BRIDGE-002-021: Modbus 폴링 인터벌 지원

**WHILE** Modbus Bridge가 활성화되어 있을 때 **THEN** ModbusAdapter는 설정된 폴링 인터벌에 따라 에이전트에 레지스터 읽기를 주기적으로 요청하고, 변환된 데이터를 플로우에 전달해야 한다.

#### REQ-BRIDGE-002-022: Modbus 쓰기 연산

**WHEN** 플로우에서 Modbus Bridge로 쓰기 메시지를 전달할 때 **THEN** ModbusAdapter는 Message Payload의 값을 레지스터 맵 설정에 따라 uint16 레지스터 값으로 역변환하고, 적절한 기능 코드(FC05, FC06, FC15, FC16)로 에이전트에 쓰기 명령을 전달해야 한다.

#### REQ-BRIDGE-002-023: Modbus 디바이스 주소 관리

시스템은 **항상** ModbusAdapter 설정에 디바이스 주소(Unit ID)를 포함해야 하며, 에이전트와의 통신 시 해당 Unit ID를 사용해야 한다.

#### REQ-BRIDGE-002-024: Modbus 기능 코드와 Bridge 방향 매핑

시스템은 **항상** Bridge Direction과 Modbus 기능 코드를 다음과 같이 매핑해야 한다:

| Bridge Direction | Modbus 기능 코드 | 설명 |
|---|---|---|
| In | FC01-FC04 (Read) | 레지스터 읽기 (폴링) |
| Out | FC05, FC06, FC15, FC16 (Write) | 레지스터/코일 쓰기 |
| InOut | FC01-FC06, FC15-FC16 | 읽기 + 쓰기 |
| RequestReply | FC03/FC04 요청 -> 응답 | 동기 읽기 |

#### REQ-BRIDGE-002-025: Modbus 설정 검증

**WHEN** ModbusAdapter.Validate()가 호출될 때 **THEN** 다음을 검증해야 한다:
- Unit ID가 1-247 범위인지
- 레지스터 맵에 최소 1개 이상의 레지스터 정의가 있는지
- 각 레지스터의 데이터 타입이 유효한지 (int16, uint16, float32, uint32, int32)
- 레지스터 주소 범위가 0-65535인지
- 폴링 인터벌이 100ms 이상인지 (In/InOut 방향일 때)

#### REQ-BRIDGE-002-026: Modbus 레지스터 정의 구조

시스템은 **항상** 다음 필드를 포함하는 레지스터 정의를 지원해야 한다:

```
RegisterDef 구조체:
  - Name string         // 레지스터 이름 (Payload 키로 사용)
  - Address uint16      // 시작 주소
  - Count uint16        // 레지스터 개수
  - DataType string     // 데이터 타입 (int16, uint16, float32, uint32, int32)
  - ByteOrder string    // 바이트 오더 (big, little) - 기본값: big
  - Scale float64       // 스케일 팩터 (기본값: 1.0)
  - Offset float64      // 오프셋 값 (기본값: 0.0)
  - ReadOnly bool       // 읽기 전용 여부
```

---

### Module 4: HTTP 전용 어댑터 (P1)

#### REQ-BRIDGE-002-030: HTTP Content-Type 처리

**WHEN** HTTP 에이전트로부터 응답을 수신할 때 **THEN** HTTPAdapter는 Content-Type에 따라 변환 전략을 적용해야 한다:
- `application/json`: JSON 파싱하여 Payload에 저장
- `application/x-www-form-urlencoded`: 폼 데이터를 key-value로 파싱
- `multipart/form-data`: 각 파트를 개별 키로 파싱
- 기타: 원본 데이터를 `_raw` 키에 저장

#### REQ-BRIDGE-002-031: HTTP 상태 코드 매핑

**WHEN** HTTP 응답을 Message로 변환할 때 **THEN** HTTPAdapter는 상태 코드를 `http.status_code` 메타데이터에 저장하고, 4xx/5xx 응답은 에러 포트로 전달해야 한다.

#### REQ-BRIDGE-002-032: HTTP 헤더 전달

시스템은 **항상** HTTP 응답/요청 헤더를 `http.header.{name}` 형식의 메타데이터로 변환하여 전달해야 한다.

#### REQ-BRIDGE-002-033: HTTP URL 경로 템플릿

**가능하면** HTTPAdapter는 요청 URL에 경로 변수 보간을 지원해야 한다. 예: `/api/devices/{id}/status`에서 `{id}`를 Payload 값으로 치환한다.

#### REQ-BRIDGE-002-034: HTTP 설정 검증

**WHEN** HTTPAdapter.Validate()가 호출될 때 **THEN** 다음을 검증해야 한다:
- Content-Type이 지원되는 형식인지
- URL 경로 템플릿 구문이 유효한지
- 타임아웃 설정이 양수인지

---

### Module 5: 프론트엔드 에이전트별 설정 UI (P1)

#### REQ-BRIDGE-002-040: 에이전트 타입별 동적 설정 패널

**WHEN** 사용자가 Bridge Node를 선택하고 에이전트 타입을 지정할 때 **THEN** PropertyPanel은 해당 에이전트 타입에 맞는 설정 필드를 동적으로 렌더링해야 한다.

#### REQ-BRIDGE-002-041: MQTT 설정 패널

**WHEN** 에이전트 타입이 'mqtt'일 때 **THEN** 다음 설정 필드를 표시해야 한다:
- 구독 토픽 입력 (다중 입력)
- 발행 토픽 입력
- QoS 레벨 선택 (0, 1, 2 드롭다운)
- Retained 메시지 토글
- 토픽 템플릿 입력 (선택)

#### REQ-BRIDGE-002-042: Modbus 설정 패널

**WHEN** 에이전트 타입이 'modbus-tcp' 또는 'modbus-rtu'일 때 **THEN** 다음 설정 필드를 표시해야 한다:
- 디바이스 주소 (Unit ID) 입력
- 폴링 인터벌 입력 (ms 단위)
- 레지스터 맵 테이블 (기존 RegisterMapEditor 컴포넌트 재사용)
  - 이름, 주소, 개수, 데이터 타입, 바이트 오더, 스케일, 오프셋, 읽기 전용

#### REQ-BRIDGE-002-043: HTTP 설정 패널

**WHEN** 에이전트 타입이 'http'일 때 **THEN** 다음 설정 필드를 표시해야 한다:
- URL 경로 입력
- Content-Type 선택 (JSON, Form, Multipart)
- 커스텀 헤더 key-value 편집기
- 타임아웃 입력

#### REQ-BRIDGE-002-044: 설정 스키마 확장

시스템은 **항상** `nodeSchemas.ts`의 `BRIDGE_AGENT_DEFAULTS` 객체를 확장하여, 에이전트 타입별 추가 설정 필드 정의를 포함해야 한다. 각 에이전트 타입은 `configFields` 배열을 통해 동적 필드 목록을 제공한다.

#### REQ-BRIDGE-002-045: 설정 유효성 검증 UI

**WHEN** 사용자가 Bridge Node 설정을 저장하려고 할 때 **THEN** 프론트엔드는 에이전트 타입별 검증 규칙을 적용하여 잘못된 값에 대해 인라인 에러 메시지를 표시해야 한다.

---

### Module 6: System Agent 통합 어댑터 (P0)

#### REQ-BRIDGE-002-050: SystemAdapter 통합

시스템은 **항상** 기존 BridgeHandler 패턴(store_bridge.go, timer_bridge.go, logger_bridge.go)을 BridgeAdapter 인터페이스로 감싸는 SystemAdapter를 제공해야 한다.

#### REQ-BRIDGE-002-051: System Agent 메타데이터 접두사

**WHEN** System Agent Bridge를 통해 메시지를 처리할 때 **THEN** SystemAdapter는 에이전트 서브타입(store, timer, logger, event, file)에 따라 적절한 메타데이터 접두사(`store.operation`, `timer.operation` 등)를 사용하여 기존 BridgeHandler에 위임해야 한다.

#### REQ-BRIDGE-002-052: System Agent 설정 검증

**WHEN** SystemAdapter.Validate()가 호출될 때 **THEN** 에이전트 서브타입이 유효한 System Agent 타입(store, timer, logger, event, file) 중 하나인지 검증해야 한다.

---

## 4. Specifications (명세)

### 4.1 패키지 구조

```
internal/node/
  bridge_adapter.go          // BridgeAdapter 인터페이스, AgentMeta 구조체
  bridge_adapter_registry.go // AdapterRegistry, DefaultAdapter
  adapter/
    mqtt.go                  // MQTTAdapter
    mqtt_test.go
    modbus.go                // ModbusAdapter
    modbus_test.go
    http.go                  // HTTPAdapter
    http_test.go
    system.go                // SystemAdapter
    system_test.go
    register.go              // init() 기반 자동 등록

web/src/config/
  nodeSchemas.ts             // BRIDGE_AGENT_DEFAULTS 확장
  bridgeAdapterSchemas.ts    // 에이전트 타입별 설정 스키마 정의

web/src/components/property/
  BridgeMqttConfig.tsx       // MQTT 전용 설정 패널
  BridgeModbusConfig.tsx     // Modbus 전용 설정 패널
  BridgeHttpConfig.tsx       // HTTP 전용 설정 패널
```

### 4.2 인터페이스 설계

```go
// BridgeAdapter 는 에이전트 타입별 전용 브릿지 어댑터 인터페이스이다.
type BridgeAdapter interface {
    // Validate 는 에이전트 타입에 특화된 설정 검증을 수행한다.
    Validate(config BridgeConfig) error

    // DefaultConfig 는 에이전트 타입에 적합한 기본 BridgeConfig를 반환한다.
    DefaultConfig() BridgeConfig

    // TransformToFlow 는 에이전트 데이터를 프로토콜 인식 방식으로 Flow Message로 변환한다.
    TransformToFlow(data []byte, meta AgentMeta) (message.Message, error)

    // TransformToAgent 는 Flow Message를 프로토콜 인식 방식으로 에이전트 데이터로 변환한다.
    TransformToAgent(msg message.Message) ([]byte, AgentMeta, error)

    // HandleControl 는 에이전트 고유 제어 메시지를 처리한다.
    HandleControl(msg message.Message) error
}

// AgentMeta 는 프로토콜별 메타 정보를 담는 구조체이다.
type AgentMeta struct {
    // 공통
    AgentType string

    // MQTT
    Topic    string
    QoS      int
    Retained bool

    // HTTP
    Headers     map[string]string
    StatusCode  int
    ContentType string
    URLPath     string

    // Modbus
    UnitID        uint8
    FunctionCode  uint8
    RegisterAddr  uint16
    RegisterCount uint16
}
```

### 4.3 Registry 설계

```go
// AdapterRegistry 는 에이전트 타입별 BridgeAdapter를 관리하는 레지스트리이다.
type AdapterRegistry struct {
    mu       sync.RWMutex
    adapters map[string]BridgeAdapter
}

// 전역 레지스트리 인스턴스
var globalAdapterRegistry = &AdapterRegistry{
    adapters: make(map[string]BridgeAdapter),
}
```

### 4.4 BridgeNode 통합 흐름

```
BridgeNode.Init()
  |
  +-> AgentRef에서 agentType 추출
  |
  +-> AdapterRegistry.GetAdapter(agentType)
  |     |
  |     +-> 어댑터 존재: adapter.Validate(config) 실행
  |     |     +-> 성공: adapter를 transformer로 설정
  |     |     +-> 실패: 에러 반환
  |     |
  |     +-> 어댑터 없음: DefaultAdapter 사용 (기존 동작)
  |
  +-> BridgeNode 초기화 계속...

BridgeNode.receiveLoop() (In/InOut 모드)
  |
  +-> transport.Receive(ctx) -> []byte + AgentMeta
  |
  +-> adapter.TransformToFlow(data, meta) -> Message
  |
  +-> outputCh <- Message

BridgeNode.Process() (Out 모드)
  |
  +-> 입력 Message 수신
  |
  +-> adapter.TransformToAgent(msg) -> []byte + AgentMeta
  |
  +-> transport.Send(ctx, data, meta)
```

### 4.5 추적성 태그

| 요구사항 ID | 모듈 | 우선순위 |
|---|---|---|
| REQ-BRIDGE-002-001 ~ 004 | Module 1: BridgeAdapter 인터페이스 및 Registry | P0 |
| REQ-BRIDGE-002-010 ~ 015 | Module 2: MQTT 전용 어댑터 | P0 |
| REQ-BRIDGE-002-020 ~ 026 | Module 3: Modbus 전용 어댑터 | P0 |
| REQ-BRIDGE-002-030 ~ 034 | Module 4: HTTP 전용 어댑터 | P1 |
| REQ-BRIDGE-002-040 ~ 045 | Module 5: 프론트엔드 설정 UI | P1 |
| REQ-BRIDGE-002-050 ~ 052 | Module 6: System Agent 통합 | P0 |

---

## 5. Implementation Notes (구현 노트)

### 5.1 구현 완료 상태

모든 P0 모듈(Module 1, 2, 3, 6)과 P1 모듈(Module 4, 5)이 구현 완료되었다.

| 모듈 | 상태 | 비고 |
|------|------|------|
| Module 1: BridgeAdapter 인터페이스 및 Registry | 완료 | bridge_adapter.go, bridge_adapter_registry.go |
| Module 2: MQTT 전용 어댑터 | 완료 | adapter/mqtt.go — 토픽 라우팅, QoS, Retained, JSON 파싱 |
| Module 3: Modbus 전용 어댑터 | 완료 | adapter/modbus.go — RegisterDef 기반 자동 변환, PollableAdapter |
| Module 4: HTTP 전용 어댑터 | 완료 | adapter/http.go — Content-Type, 상태 코드, 헤더, URL 템플릿 |
| Module 5: 프론트엔드 설정 UI | 완료 | BridgeMqttConfig, BridgeModbusConfig, BridgeHttpConfig |
| Module 6: System Agent 통합 | 완료 | adapter/system.go — store/timer/logger/event/file 서브타입 |

### 5.2 범위 확장: PollableAdapter (브릿지 주도 폴링)

원래 SPEC에는 포함되지 않았으나, Modbus 폴링을 에이전트가 아닌 브릿지에서 주도하는 아키텍처로 확장하였다.

**추가된 인터페이스**:
- `PollableAdapter`: ReadSpecs() + AssembleMessage() 메서드
- `ReadSpec`: 단일 읽기 단위 정의 (FunctionCode, StartAddr, Quantity, UnitID)
- `ReadResult`: 에이전트로부터 받은 원시 읽기 결과

**추가된 구현**:
- `ModbusAdapter.ReadSpecs()`: RegisterDef 배열을 ReadSpec 목록으로 변환
- `ModbusAdapter.AssembleMessage()`: ReadResult 바이트를 조합하여 플로우 Message 생성
- `BridgeNode.startBridgePollLoop()`: 브릿지가 자체 타이머로 에이전트에 read_raw 명령 전송
- `ModbusAgent.processReadRaw()`: Modbus TCP 클라이언트 에이전트의 read_raw 처리
- `ModbusServerAgent.processReadRaw()`: Modbus TCP 서버 에이전트의 read_raw 처리 (로컬 레지스터 스토어에서 읽기)

**이점**: 하나의 에이전트에 여러 브릿지가 연결될 때, 각 브릿지가 독립적인 레지스터 맵과 폴링 간격으로 읽기 가능.

### 5.3 추가 버그 수정 및 개선

- `getPollingIntervalOverride()`: `polling_interval` 키 폴백 추가 (기존에는 `polling_interval_ms`만 인식)
- WebSocket 클라이언트 재연결 안정성 개선
- Transform 노드 `strip_nulls` 옵션 추가
- API 서비스 어댑터 (에이전트/플로우) 개선 및 테스트 추가
- 엔진 라이프사이클 관리 개선

---

*문서 버전: 1.0.0*
*최종 수정: 2026-03-06*
*작성: MoAI SPEC Builder*
