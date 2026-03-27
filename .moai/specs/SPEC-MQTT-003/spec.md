---
id: SPEC-MQTT-003
version: "1.3.0"
status: completed
created: "2026-03-14"
updated: "2026-03-27"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-03-14 | 1.0.0 | 초기 SPEC 작성 |
| 2026-03-15 | 1.1.0 | 프론트엔드 스키마 동기화: agent_ref → agent_select, topics → string_list, StringListEditor 컴포넌트 추가, 에이전트 타입 필터링 추가, 예제 플로우 3종 추가 |
| 2026-03-16 | 1.2.0 | AgentRef 해석 수정: agent_ref를 AgentID+AgentName 이중 설정으로 UUID/이름 모두 검색 가능 |
| 2026-03-27 | 1.3.0 | 에이전트 타입 리네임 (mqtt → mqtt-client), Subscribe 브로커 미연결 허용 (토픽 선등록 + OnConnectHandler 자동 구독), PropertyPanel 스키마 우선순위 수정 |

---

# SPEC-MQTT-003: MQTT Subscriber / Publisher 노드 구현

## 1. Environment (환경)

### 1.1 시스템 개요

현재 xflow에서 MQTT 에이전트와의 통신은 Bridge 노드를 통해서만 가능하다. Bridge 노드는 범용 에이전트 통신 계층으로, MQTT 프로토콜 고유 기능(다중 토픽 구독, QoS 제어, Retained 메시지 등)을 직접 노출하기 어렵다.

본 SPEC은 MQTT 에이전트에 직접 연결하는 전용 노드 2종을 구현한다:

1. **mqtt-subscriber 노드**: SourceNode 인터페이스를 구현하여 MQTT 에이전트로부터 메시지를 수신. 설정된 토픽을 Init 시 자동 구독하고, 수신 메시지를 플로우 메시지로 변환하여 출력.
2. **mqtt-publisher 노드**: Process() 기반 노드로, 입력 메시지를 MQTT 에이전트를 통해 발행. 토픽/QoS/Retained 옵션을 설정 및 런타임 오버라이드 지원.

기존 Bridge 노드와의 차이점은 다음과 같다:
- **직접 연결**: Bridge의 BridgeAdapter 계층을 거치지 않고, MQTT 에이전트의 SubscriberAgent/MessagePublisher 인터페이스를 직접 사용
- **다중 토픽**: subscriber 노드에서 복수 토픽을 설정 단위로 구독 가능
- **전용 설정**: MQTT 프로토콜 전용 설정(QoS, Retained, 토픽 템플릿)을 노드 설정으로 직접 노출
- **NASA 노드 패턴 준수**: nasaNodeBase 패턴(AgentResolver, AgentAccessor, BaseNode 임베딩)과 동일한 구조 채택

### 1.2 기술 환경

- **언어**: Go 1.25+
- **패키지 경로**:
  - `internal/node/mqtt.go`: mqtt-subscriber, mqtt-publisher 노드 구현 (신규)
  - `internal/node/mqtt_test.go`: 노드 테스트 (신규)
  - `internal/node/registry.go`: 빌트인 노드 등록 (수정)
  - `web/src/config/nodeSchemas.ts`: 프론트엔드 설정 스키마 (수정)
  - `web/src/pages/nodes/nodeTypeMeta.ts`: 프론트엔드 노드 메타 (수정)
- **Tier**: Tier 2 - 내부 실행 계층 (internal)
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-001): SubscriberAgent, MessageReceiver, MessagePublisher 인터페이스
  - `internal/agent/system/mqtt_subscriber.go` (SPEC-SYSAGENT-001): MQTTSubscriberAgent 구현체
  - `internal/node/base.go` (SPEC-NODE-001): BaseNode, Node, SourceNode 인터페이스
  - `internal/node/bridge.go` (SPEC-BRIDGE-001): AgentResolver, AgentTransport, AgentAccessor
  - `internal/node/adapter/mqtt.go` (SPEC-BRIDGE-002): MQTTAdapter (TransformToFlow, TransformToAgent)
  - `pkg/flow/` (SPEC-FLOW-001): NodeDef, AgentRef, PortDirection
  - `pkg/message/` (SPEC-MSG-001): Message, Payload
  - `pkg/lifecycle/` (SPEC-LIFE-001): BaseLifecycle, 상태 전이
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**:
  - `sync.Once` (stopCh close 보호, subscriber)
  - `sync.RWMutex` (설정 보호)

### 1.3 설계 원칙

- **NASA 노드 패턴 준수**: nasaNodeBase와 동일한 구조(BaseNode 임베딩, AgentResolver를 통한 에이전트 해석, AgentAccessor를 통한 원본 에이전트 접근)를 사용하여 코드베이스 일관성 유지
- **기존 어댑터 재사용**: MQTTAdapter의 TransformToFlow/TransformToAgent를 재사용하여 메시지 변환 로직 중복 방지
- **인터페이스 기반 확장**: SubscriberAgent, MessageReceiver, MessagePublisher 인터페이스를 통해 에이전트 타입에 비종속적 설계
- **Graceful Shutdown**: subscriber 노드는 Shutdown 시 구독 해제 후 수신 고루틴을 종료하여 리소스 누수 방지
- **최소 침습 원칙**: 기존 코드(bridge.go, mqtt_subscriber.go, adapter/mqtt.go)는 수정하지 않음. 새 파일(mqtt.go)만 추가

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `mqtt-subscriber` 노드 구현 (SourceNode 인터페이스)
- `mqtt-publisher` 노드 구현 (Process 기반)
- 노드 레지스트리 등록 (`internal/node/registry.go`)
- 프론트엔드 스키마 추가 (`nodeSchemas.ts`, `nodeTypeMeta.ts`)
- 단위 테스트 (`internal/node/mqtt_test.go`)

**OUT OF SCOPE (별도 SPEC)**:
- Bridge 노드 기능 변경
- MQTT 에이전트 구현 변경 (기존 인터페이스 그대로 사용)
- MQTT 5.0 Shared Subscription 지원
- 토픽 와일드카드 자동 분배 (fan-out)
- QoS별 토픽 그룹화 (모든 토픽에 동일 QoS 적용)
- 에이전트 상태 모니터링 (reconnect 감지)

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-MQTT-001 | 선행 | SubscriberAgent 인터페이스, Bridge 토픽 구독 |
| SPEC-MQTT-002 | 선행 | MQTT client_id UUID 자동 생성 |
| SPEC-AGENT-001 | 의존 | Agent, MessageReceiver, SubscriberAgent, MessagePublisher 인터페이스 |
| SPEC-SYSAGENT-001 | 의존 | MQTTSubscriberAgent 구현체 |
| SPEC-BRIDGE-001 | 참조 | AgentResolver, AgentTransport, AgentAccessor 인터페이스 |
| SPEC-BRIDGE-002 | 참조 | MQTTAdapter 메시지 변환 |
| SPEC-NODE-001 | 의존 | Node, SourceNode, BaseNode 인터페이스/구조체 |
| SPEC-NASA-001 | 참조 | NASA 노드 패턴 (nasaNodeBase, SourceNode 구현 참조) |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: SPEC-MQTT-001의 SubscriberAgent 인터페이스(`Subscribe`, `Unsubscribe`)가 구현되어 있다
- A2: SPEC-AGENT-001의 MessageReceiver 인터페이스(`ReceiveMessage`)와 MessagePublisher 인터페이스(`PublishMessage`)가 구현되어 있다
- A3: `MQTTSubscriberAgent`가 SubscriberAgent, MessageReceiver, MessagePublisher를 모두 구현하고 있다
- A4: `AgentResolver.ResolveAgent()`가 `AgentTransport`를 반환하고, `AgentAccessor.UnderlyingAgent()`를 통해 원본 Agent에 접근할 수 있다
- A5: `MQTTAdapter.TransformToFlow()`와 `TransformToAgent()`가 안정적으로 동작한다
- A6: `MQTTSubscriberAgent.ReceiveMessage()`는 blocking 호출로, 데이터 도착 또는 context 취소/done 시 반환된다
- A7: `MQTTSubscriberAgent.Subscribe()` 호출 시 해당 토픽의 메시지가 `ReceiveMessage()`를 통해 수신된다
- A8: `AgentMeta`의 Topic, QoS, Retained 필드가 MQTT 메타데이터를 전달할 수 있다

### 2.2 도메인 가정

- A9: mqtt-subscriber 노드는 하나의 MQTT 에이전트에 연결되며, 해당 에이전트가 관리하는 모든 구독 토픽의 메시지를 수신한다
- A10: mqtt-subscriber 노드가 구독한 토픽은 노드 Shutdown 시 자동 해제되어야 한다
- A11: mqtt-publisher 노드의 발행 토픽은 설정 기본값과 런타임 오버라이드(payload `_mqtt.topic` 또는 metadata `mqtt.topic`)를 모두 지원한다
- A12: mqtt-publisher 노드는 passthrough 방식으로, 발행 후 원본 메시지를 다음 노드로 전달한다
- A13: ~~에이전트 연결이 끊어진 상태에서의 Subscribe/Publish 호출은 에러를 반환한다~~ **(v1.3.0 변경)** Subscribe는 브로커 미연결 시에도 토픽을 subscribedTopics에 저장하고, OnConnectHandler가 연결 후 자동 구독한다. Publish는 여전히 미연결 시 에러를 반환한다
- A14: ReceiveMessage에서 반환되는 데이터에는 MQTT 토픽, QoS 등의 메타데이터가 포함되지 않는다 (현재 MQTTSubscriberAgent.messageHandler가 payload만 recvCh에 전달)

---

## 3. Requirements (요구사항)

### 모듈 1: mqtt-subscriber 노드

#### REQ-MQTT-003-01-01 (Ubiquitous) MQTTSubscriberNode 구조체

시스템은 **항상** `internal/node/mqtt.go`에 다음 필드를 포함하는 `MQTTSubscriberNode` 구조체를 제공해야 한다:

- `*BaseNode` 임베딩
- `resolver AgentResolver`: 에이전트 해석기
- `transport AgentTransport`: 에이전트 통신 인터페이스
- `agent agent.Agent`: 원본 에이전트 객체
- `mqttCfg MQTTSubscriberNodeConfig`: MQTT 구독 노드 설정
- `adapter *adapter.MQTTAdapter`: 메시지 변환 어댑터
- `sourceCh chan message.Message`: SourceNode 출력 채널
- `stopCh chan struct{}`: 수신 고루틴 종료 시그널
- `stopOnce sync.Once`: stopCh close 보호
- `mu sync.RWMutex`: 설정 보호

#### REQ-MQTT-003-01-02 (Ubiquitous) MQTTSubscriberNodeConfig 설정 구조체

시스템은 **항상** 다음 필드를 포함하는 `MQTTSubscriberNodeConfig` 구조체를 제공해야 한다:

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `AgentRef` | `string` | Y | - | 연결할 MQTT 에이전트 이름/ID |
| `Topics` | `[]TopicConfig` | Y | - | 구독 토픽 목록 |
| `PayloadFormat` | `string` | N | `"json"` | 수신 페이로드 형식 (`"json"`, `"raw"`) |
| `BufferSize` | `int` | N | `64` | sourceCh 버퍼 크기 |

`TopicConfig` 구조체:

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `Topic` | `string` | Y | - | MQTT 토픽 문자열 |
| `QoS` | `int` | N | `0` | QoS 레벨 (0, 1, 2) |

#### REQ-MQTT-003-01-03 (Ubiquitous) Node/SourceNode 인터페이스 구현

시스템은 **항상** `MQTTSubscriberNode`가 `Node` 인터페이스와 `SourceNode` 인터페이스를 모두 구현해야 한다. 컴파일 타임 인터페이스 체크를 포함한다:

```
var _ Node = (*MQTTSubscriberNode)(nil)
var _ SourceNode = (*MQTTSubscriberNode)(nil)
```

#### REQ-MQTT-003-01-04 (Ubiquitous) Configure 메서드

시스템은 **항상** `Configure(config map[string]any) error` 메서드에서:

- `agent_ref` (필수) 검증: 빈 문자열이면 에러 반환
- `topics` (필수) 검증: 비어 있으면 에러 반환
- `payload_format` (선택) 파싱: 기본값 `"json"`
- `buffer_size` (선택) 파싱: 기본값 `64`
- MQTTAdapter 인스턴스 생성

#### REQ-MQTT-003-01-05 (Event-Driven) Init에서 토픽 자동 구독

**WHEN** `MQTTSubscriberNode.Init(ctx)` 호출 시, **THEN** 다음 순서로 초기화해야 한다:

1. `resolver.ResolveAgent(ctx, ref)`로 AgentTransport 획득
2. `AgentAccessor.UnderlyingAgent()`로 원본 Agent 획득
3. 원본 Agent가 `agent.SubscriberAgent`를 구현하는지 확인 (미구현 시 에러 반환)
4. 원본 Agent가 `agent.MessageReceiver`를 구현하는지 확인 (미구현 시 에러 반환)
5. 설정된 각 토픽에 대해 `SubscriberAgent.Subscribe(ctx, topicNames)` 호출
   > **v1.3.0 변경**: `MQTTAgent.Subscribe()`는 브로커 미연결 시에도 에러를 반환하지 않음. 토픽을 내부 `subscribedTopics`에 먼저 저장하고, 브로커 연결 상태이면 즉시 구독, 미연결 상태이면 `OnConnectHandler`가 연결 시점에 자동 구독함. 따라서 노드 Init 시 에이전트의 브로커 연결 여부와 무관하게 토픽 등록이 가능하며, 런타임 재연결 시에도 `OnConnectHandler`가 토픽을 자동 복원함.
6. 수신 고루틴(`receiveLoop`) 시작

#### REQ-MQTT-003-01-06 (Ubiquitous) receiveLoop 수신 고루틴

시스템은 **항상** `receiveLoop` 고루틴에서:

- `agent.(MessageReceiver).ReceiveMessage(ctx)` 블로킹 호출로 데이터 수신
- 수신 데이터를 `MQTTAdapter.TransformToFlow(data, meta)` 로 플로우 메시지로 변환
- 변환된 메시지에 `mqtt.node_id` 메타데이터 추가
- `sourceCh`에 메시지 전송 (채널 가득 시 드롭, 경고 로그)
- `stopCh` 닫힘 또는 `ReceiveMessage` 에러 시 종료

#### REQ-MQTT-003-01-07 (Event-Driven) Process 메서드 (입력 메시지 트리거)

**WHEN** `Process(ctx, msg)` 호출 시, **THEN** 입력 메시지를 그대로 출력으로 전달해야 한다 (passthrough). mqtt-subscriber 노드의 주 데이터 흐름은 SourceCh를 통한 비동기 수신이며, Process는 보조 트리거로 사용된다.

#### REQ-MQTT-003-01-08 (Event-Driven) Shutdown에서 토픽 구독 해제

**WHEN** `MQTTSubscriberNode.Shutdown(ctx)` 호출 시, **THEN**:

1. `stopCh`를 닫아 receiveLoop 종료 시그널 전송 (sync.Once 보호)
2. 원본 Agent가 SubscriberAgent를 구현하면, 설정된 토픽 목록에 대해 `Unsubscribe(ctx, topicNames)` 호출
3. Unsubscribe 에러 시 로그 기록 후 계속 진행

#### REQ-MQTT-003-01-09 (Ubiquitous) SourceCh 메서드

시스템은 **항상** `SourceCh() <-chan message.Message` 메서드에서 내부 `sourceCh` 채널을 반환해야 한다.

#### REQ-MQTT-003-01-10 (Ubiquitous) 팩토리 함수

시스템은 **항상** `NewMQTTSubscriberNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` 팩토리 함수를 제공해야 한다. `WithAgentResolver` 옵션에서 AgentResolver를 추출한다.

---

### 모듈 2: mqtt-publisher 노드

#### REQ-MQTT-003-02-01 (Ubiquitous) MQTTPublisherNode 구조체

시스템은 **항상** `internal/node/mqtt.go`에 다음 필드를 포함하는 `MQTTPublisherNode` 구조체를 제공해야 한다:

- `*BaseNode` 임베딩
- `resolver AgentResolver`: 에이전트 해석기
- `transport AgentTransport`: 에이전트 통신 인터페이스
- `agent agent.Agent`: 원본 에이전트 객체
- `mqttCfg MQTTPublisherNodeConfig`: MQTT 발행 노드 설정
- `adapter *adapter.MQTTAdapter`: 메시지 변환 어댑터
- `mu sync.RWMutex`: 설정 보호

#### REQ-MQTT-003-02-02 (Ubiquitous) MQTTPublisherNodeConfig 설정 구조체

시스템은 **항상** 다음 필드를 포함하는 `MQTTPublisherNodeConfig` 구조체를 제공해야 한다:

| 필드 | 타입 | 필수 | 기본값 | 설명 |
|------|------|------|--------|------|
| `AgentRef` | `string` | Y | - | 연결할 MQTT 에이전트 이름/ID |
| `DefaultTopic` | `string` | N | `""` | 기본 발행 토픽 |
| `DefaultQoS` | `int` | N | `0` | 기본 QoS 레벨 (0, 1, 2) |
| `DefaultRetained` | `bool` | N | `false` | 기본 Retained 플래그 |
| `PayloadFormat` | `string` | N | `"json"` | 발행 페이로드 형식 (`"json"`, `"raw"`) |

#### REQ-MQTT-003-02-03 (Ubiquitous) Node 인터페이스 구현

시스템은 **항상** `MQTTPublisherNode`가 `Node` 인터페이스를 구현해야 한다. SourceNode는 구현하지 않는다. 컴파일 타임 인터페이스 체크를 포함한다:

```
var _ Node = (*MQTTPublisherNode)(nil)
```

#### REQ-MQTT-003-02-04 (Ubiquitous) Configure 메서드

시스템은 **항상** `Configure(config map[string]any) error` 메서드에서:

- `agent_ref` (필수) 검증: 빈 문자열이면 에러 반환
- `default_topic` (선택) 파싱
- `default_qos` (선택) 파싱: 기본값 `0`, 범위 검증 (0-2)
- `default_retained` (선택) 파싱: 기본값 `false`
- `payload_format` (선택) 파싱: 기본값 `"json"`
- MQTTAdapter 인스턴스 생성 (WithDefaultQoS, WithDefaultRetained, WithPublishTopic 옵션 적용)

#### REQ-MQTT-003-02-05 (Event-Driven) Init에서 에이전트 해석

**WHEN** `MQTTPublisherNode.Init(ctx)` 호출 시, **THEN** 다음 순서로 초기화해야 한다:

1. `resolver.ResolveAgent(ctx, ref)`로 AgentTransport 획득
2. `AgentAccessor.UnderlyingAgent()`로 원본 Agent 획득
3. 원본 Agent가 `agent.MessagePublisher`를 구현하는지 확인 (미구현 시 에러 반환)

#### REQ-MQTT-003-02-06 (Event-Driven) Process에서 메시지 발행

**WHEN** `MQTTPublisherNode.Process(ctx, msg)` 호출 시, **THEN** 다음 순서로 처리해야 한다:

1. `MQTTAdapter.TransformToAgent(msg)`로 페이로드 및 메타데이터 변환
2. 토픽 결정: `AgentMeta.Topic`이 비어 있으면 `DefaultTopic` 사용
3. 토픽이 최종적으로 비어 있으면 에러 반환
4. `agent.(MessagePublisher).PublishMessage(topic, qos, retained, payload)` 호출
5. 발행 성공 시 입력 메시지에 `mqtt.published_topic` 메타데이터 추가 후 반환 (passthrough)

#### REQ-MQTT-003-02-07 (Unwanted) 빈 토픽 발행 금지

시스템은 발행 토픽이 결정되지 않은 상태에서 PublishMessage를 호출**하지 않아야 한다**. 토픽이 비어 있으면 에러를 반환해야 한다.

#### REQ-MQTT-003-02-08 (Event-Driven) Shutdown

**WHEN** `MQTTPublisherNode.Shutdown(ctx)` 호출 시, **THEN** 상태를 Stopping으로 전이해야 한다. 별도의 리소스 정리는 불필요하다 (에이전트 연결은 에이전트가 관리).

#### REQ-MQTT-003-02-09 (Ubiquitous) 팩토리 함수

시스템은 **항상** `NewMQTTPublisherNode(def flow.NodeDef, opts ...NodeOption) (Node, error)` 팩토리 함수를 제공해야 한다. `WithAgentResolver` 옵션에서 AgentResolver를 추출한다.

---

### 모듈 3: 프론트엔드 스키마

#### REQ-MQTT-003-03-01 (Ubiquitous) nodeSchemas.ts 스키마 추가

시스템은 **항상** `web/src/config/nodeSchemas.ts`에 `mqtt-subscriber`와 `mqtt-publisher` 노드의 설정 필드 스키마를 추가해야 한다.

mqtt-subscriber 필드:
- `agent_ref` (agent_select, 필수, options: ['mqtt-client']): MQTT 에이전트 선택 (에이전트 타입 필터링)
- `topics` (string_list, 필수): 구독 토픽 목록 (StringListEditor 컴포넌트로 편집)
- `payload_format` (select, 선택): json / raw
- `buffer_size` (number, 선택): sourceCh 버퍼 크기

mqtt-publisher 필드:
- `agent_ref` (agent_select, 필수, options: ['mqtt-client']): MQTT 에이전트 선택 (에이전트 타입 필터링)
- `default_topic` (string, 선택): 기본 발행 토픽
- `default_qos` (select, 선택): 0 / 1 / 2
- `default_retained` (boolean, 선택): Retained 플래그
- `payload_format` (select, 선택): json / raw

> **v1.1.0 변경**: agent_ref 타입을 `string` → `agent_select`로 변경하여 등록된 MQTT 에이전트만 드롭다운에 표시. topics 타입을 `array` → `string_list`로 변경하여 전용 StringListEditor 컴포넌트로 토픽 추가/삭제 지원.

> **v1.3.0 변경**: 에이전트 타입 필터 `options`를 `['mqtt']` → `['mqtt-client']`로 변경. MQTTAgent의 `Type()` 반환값이 `"mqtt"` → `"mqtt-client"`로 리네임됨에 따라 TypeRegistry 등록 키, 노드 어댑터(`adapter/mqtt.go`) `TransformToAgent` 호출, 프론트엔드 `nodeSchemas.ts` 에이전트 타입 필터 모두 `'mqtt-client'`로 동기화.

> **v1.3.0 변경 (PropertyPanel)**: `PropertyPanel`의 스키마 결정 로직을 정적 스키마(`getConfigSchema`) 우선, 스냅샷 스키마(`draft.config_schema`) 폴백으로 변경. 기존에는 노드 생성 시 캐시된 구 스키마가 최신 정적 스키마를 덮어쓰는 문제가 있었으며, 이를 정적 스키마 우선 적용으로 해결.

#### REQ-MQTT-003-03-03 (Ubiquitous) StringListEditor 컴포넌트

시스템은 **항상** `web/src/components/property/StringListEditor.tsx`에 `string[]` 형태의 데이터를 행별로 편집하는 컴포넌트를 제공해야 한다:

- 빈 행 추가/삭제 지원 (추가 버튼, 삭제 버튼)
- 내부 상태(`useState`)로 빈 행 보존, 외부 onChange에는 빈 문자열 제외하여 전달
- `internalUpdate` ref로 자기 업데이트와 외부 value 동기화를 구분
- readOnly 모드 지원
- ConfigField의 `type: 'string_list'`와 연동

#### REQ-MQTT-003-03-04 (Ubiquitous) agent_select 에이전트 타입 필터링

시스템은 **항상** `web/src/components/property/FormField.tsx`의 `AgentSelectInput` 컴포넌트에서 `field.options` 배열이 제공되면 해당 에이전트 타입만 드롭다운에 표시해야 한다:

- `options: ['mqtt']` → MQTT 타입 에이전트만 표시
- `options` 미지정 시 전체 에이전트 표시 (기존 동작 유지)

#### REQ-MQTT-003-03-02 (Ubiquitous) nodeTypeMeta.ts 메타 추가

시스템은 **항상** `web/src/pages/nodes/nodeTypeMeta.ts`에 `mqtt-subscriber`와 `mqtt-publisher`의 설명, 포트 정보, 설정 필드 목록, 설정 예제를 추가해야 한다.

mqtt-subscriber 포트:
- `out` (output): MQTT 에이전트에서 수신한 메시지 출력

mqtt-publisher 포트:
- `in` (input): 발행할 메시지 입력
- `out` (output): 발행 후 원본 메시지 passthrough 출력

---

### 모듈 4: 노드 레지스트리 등록

#### REQ-MQTT-003-04-01 (Ubiquitous) registerBuiltins 등록

시스템은 **항상** `internal/node/registry.go`의 `registerBuiltins()` 함수에 다음 노드를 등록해야 한다:

- `"mqtt-subscriber"`: `NewMQTTSubscriberNode`, category `"io"`, description `"MQTT 에이전트에서 메시지를 구독 수신"`
- `"mqtt-publisher"`: `NewMQTTPublisherNode`, category `"io"`, description `"MQTT 에이전트로 메시지를 발행"`

---

## 4. Specifications (명세)

### 4.1 파일 구조

```
internal/node/
├── mqtt.go                    // [신규] MQTTSubscriberNode, MQTTPublisherNode 구현
├── mqtt_test.go               // [신규] 단위 테스트
└── registry.go                // [수정] registerBuiltins에 mqtt-subscriber, mqtt-publisher 추가

internal/agent/system/
├── mqtt_agent.go              // [수정] v1.3.0 Type() "mqtt" → "mqtt-client" 리네임, Subscribe 브로커 미연결 허용
└── mqtt_register.go           // [수정] v1.3.0 TypeRegistry 등록 키 "mqtt-client"로 변경

internal/node/adapter/
└── mqtt.go                    // [수정] v1.3.0 TransformToAgent AgentType "mqtt-client" 사용

web/src/
├── components/property/
│   ├── FormField.tsx          // [수정] string_list 타입 추가, agent_select 에이전트 타입 필터링 추가
│   ├── PropertyPanel.tsx      // [수정] v1.3.0 스키마 우선순위 변경 (정적 스키마 우선, 스냅샷 폴백)
│   └── StringListEditor.tsx   // [신규] 문자열 목록 에디터 컴포넌트
├── config/
│   └── nodeSchemas.ts         // [수정] mqtt-subscriber, mqtt-publisher 설정 스키마 추가
├── pages/nodes/
│   └── nodeTypeMeta.ts        // [수정] mqtt-subscriber, mqtt-publisher 노드 메타 추가
└── types/
    └── node.ts                // [수정] ConfigField.type에 'string_list' 추가

examples/flows/
├── mqtt-subscriber-node.yaml  // [신규] mqtt-subscriber 기본 예제
├── mqtt-pub-sub-node.yaml     // [신규] mqtt-subscriber + mqtt-publisher 파이프라인 예제
└── mqtt-to-modbus-v4.yaml     // [신규] mqtt-subscriber + modbus + mqtt-publisher 양방향 IoT 게이트웨이 예제
```

### 4.2 타입 시그니처

```go
// ── internal/node/mqtt.go ──

// MQTTSubscriberNodeConfig 는 mqtt-subscriber 노드 설정이다.
type MQTTSubscriberNodeConfig struct {
    AgentRef      string        `json:"agent_ref"`
    Topics        []TopicConfig `json:"topics"`
    PayloadFormat string        `json:"payload_format"`
    BufferSize    int           `json:"buffer_size"`
}

// TopicConfig 는 개별 토픽 설정이다.
type TopicConfig struct {
    Topic string `json:"topic"`
    QoS   int    `json:"qos"`
}

// MQTTPublisherNodeConfig 는 mqtt-publisher 노드 설정이다.
type MQTTPublisherNodeConfig struct {
    AgentRef        string `json:"agent_ref"`
    DefaultTopic    string `json:"default_topic"`
    DefaultQoS      int    `json:"default_qos"`
    DefaultRetained bool   `json:"default_retained"`
    PayloadFormat   string `json:"payload_format"`
}

// MQTTSubscriberNode 는 MQTT 에이전트에서 메시지를 구독 수신하는 SourceNode이다.
type MQTTSubscriberNode struct {
    *BaseNode
    resolver  AgentResolver
    transport AgentTransport
    agent     agent.Agent
    mqttCfg   MQTTSubscriberNodeConfig
    adapter   *adapter.MQTTAdapter
    sourceCh  chan message.Message
    stopCh    chan struct{}
    stopOnce  sync.Once
    mu        sync.RWMutex
}

// MQTTPublisherNode 는 MQTT 에이전트로 메시지를 발행하는 노드이다.
type MQTTPublisherNode struct {
    *BaseNode
    resolver  AgentResolver
    transport AgentTransport
    agent     agent.Agent
    mqttCfg   MQTTPublisherNodeConfig
    adapter   *adapter.MQTTAdapter
    mu        sync.RWMutex
}

// 컴파일 타임 인터페이스 체크
var (
    _ Node       = (*MQTTSubscriberNode)(nil)
    _ SourceNode = (*MQTTSubscriberNode)(nil)
    _ Node       = (*MQTTPublisherNode)(nil)
)

// 팩토리 함수
func NewMQTTSubscriberNode(def flow.NodeDef, opts ...NodeOption) (Node, error)
func NewMQTTPublisherNode(def flow.NodeDef, opts ...NodeOption) (Node, error)

// MQTTSubscriberNode 메서드
func (n *MQTTSubscriberNode) Configure(config map[string]any) error
func (n *MQTTSubscriberNode) Init(ctx context.Context) error
func (n *MQTTSubscriberNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *MQTTSubscriberNode) Shutdown(ctx context.Context) error
func (n *MQTTSubscriberNode) SourceCh() <-chan message.Message

// MQTTPublisherNode 메서드
func (n *MQTTPublisherNode) Configure(config map[string]any) error
func (n *MQTTPublisherNode) Init(ctx context.Context) error
func (n *MQTTPublisherNode) Process(ctx context.Context, msg message.Message) ([]message.Message, error)
func (n *MQTTPublisherNode) Shutdown(ctx context.Context) error
```

### 4.3 mqtt-subscriber 데이터 흐름

```
Init():
  1. BaseNode.TransitionTo(Initializing)
  2. resolver.ResolveAgent(ctx, AgentRef) -> transport
  3. transport.(AgentAccessor).UnderlyingAgent() -> agent
  4. agent.(SubscriberAgent) 타입 확인 (실패 시 에러)
  5. agent.(MessageReceiver) 타입 확인 (실패 시 에러)
  6. SubscriberAgent.Subscribe(ctx, topicNames) 호출
  7. go receiveLoop() 시작
  8. BaseNode.TransitionTo(Running)

receiveLoop():
  1. ctx, cancel := context.WithCancel(context.Background())
  2. for {
       select {
       case <-stopCh: cancel(); return
       default:
         data, err := agent.(MessageReceiver).ReceiveMessage(ctx)
         if err != nil: 에러 처리 (stopCh 확인 후 continue 또는 return)
         msg, err := adapter.TransformToFlow(data, AgentMeta{AgentType: "mqtt-client"})
         if err != nil: continue
         msg.Metadata().Set("mqtt.node_id", n.ID())
         sourceCh <- msg (가득 차면 드롭)
       }
     }

Shutdown():
  1. stopOnce.Do(func() { close(stopCh) })
  2. agent.(SubscriberAgent).Unsubscribe(ctx, topicNames)
  3. BaseNode.TransitionTo(Stopping)
```

### 4.4 mqtt-publisher 데이터 흐름

```
Init():
  1. BaseNode.TransitionTo(Initializing)
  2. resolver.ResolveAgent(ctx, AgentRef) -> transport
  3. transport.(AgentAccessor).UnderlyingAgent() -> agent
  4. agent.(MessagePublisher) 타입 확인 (실패 시 에러)
  5. BaseNode.TransitionTo(Running)

Process(ctx, msg):
  1. adapter.TransformToAgent(msg) -> payload, agentMeta, err
  2. topic := agentMeta.Topic
  3. IF topic == "": topic = mqttCfg.DefaultTopic
  4. IF topic == "": return error ("publish topic not specified")
  5. qos := byte(agentMeta.QoS)
  6. retained := agentMeta.Retained
  7. agent.(MessagePublisher).PublishMessage(topic, qos, retained, payload)
  8. out := msg.Clone()
  9. out.Metadata().Set("mqtt.published_topic", topic)
  10. return [out], nil

Shutdown():
  1. BaseNode.TransitionTo(Stopping)
```

### 4.5 에러 변수

```go
var (
    ErrMQTTMissingAgentRef = fmt.Errorf("mqtt: %w: agent_ref is required", ErrInvalidConfig)
    ErrMQTTMissingTopics   = fmt.Errorf("mqtt-subscriber: %w: topics is required", ErrInvalidConfig)
    ErrMQTTNoResolver      = fmt.Errorf("mqtt: %w: agent resolver not configured", ErrNodeNotInitialized)
    ErrMQTTAgentNotMQTT    = fmt.Errorf("mqtt: %w: agent does not implement required interface", ErrInvalidConfig)
    ErrMQTTNoTopic         = fmt.Errorf("mqtt-publisher: publish topic not specified")
    ErrMQTTPublishFailed   = fmt.Errorf("mqtt-publisher: publish failed")
)
```

---

## 5. Traceability (추적성)

| 요구사항 ID | REQ | 파일 | 우선순위 |
|------------|-----|------|---------|
| REQ-MQTT-003-01-01 ~ 01-10 | 모듈 1: mqtt-subscriber 노드 | internal/node/mqtt.go | P0 |
| REQ-MQTT-003-02-01 ~ 02-09 | 모듈 2: mqtt-publisher 노드 | internal/node/mqtt.go | P0 |
| REQ-MQTT-003-03-01 ~ 03-02 | 모듈 3: 프론트엔드 스키마 | web/src/config/nodeSchemas.ts, web/src/pages/nodes/nodeTypeMeta.ts | P1 |
| REQ-MQTT-003-03-03 | 모듈 3: StringListEditor 컴포넌트 | web/src/components/property/StringListEditor.tsx | P1 |
| REQ-MQTT-003-03-04 | 모듈 3: agent_select 타입 필터링 | web/src/components/property/FormField.tsx | P1 |
| REQ-MQTT-003-04-01 | 모듈 4: 노드 레지스트리 등록 | internal/node/registry.go | P0 |
