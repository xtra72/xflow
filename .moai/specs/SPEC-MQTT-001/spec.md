---
id: SPEC-MQTT-001
version: "1.0.0"
status: planned
created: "2026-02-22"
updated: "2026-02-22"
author: xtra
priority: high
---

## HISTORY

| 날짜 | 버전 | 변경 내용 |
|------|------|----------|
| 2026-02-22 | 1.0.0 | 초기 SPEC 작성 |

---

# SPEC-MQTT-001: MQTT Bridge Topic Subscription Configuration - Bridge 노드를 통한 MQTT 토픽 구독 관리

## 1. Environment (환경)

### 1.1 시스템 개요

현재 MQTT 에이전트의 토픽 구독은 에이전트 설정(`MQTTSubscriberConfig.Topics`)을 통해서만 구성 가능하다. 본 SPEC은 Bridge 노드를 통해 두 가지 추가 구독 경로를 제공한다:

1. **Bridge 설정 토픽**: Bridge 설정에 정의된 토픽은 Bridge 초기화 시 자동 구독
2. **런타임 제어 메시지**: Bridge Process()를 통해 전달되는 제어 메시지로 동적 구독/해제

이를 위해 에이전트 패키지에 `SubscriberAgent` 인터페이스를 도입하여 동적 구독 관리를 추상화하고, `MQTTSubscriberAgent`가 이를 구현한다.

### 1.2 기술 환경

- **언어**: Go 1.23+
- **패키지 경로**:
  - `internal/agent/agent.go`: SubscriberAgent 인터페이스 정의
  - `internal/agent/system/mqtt_subscriber.go`: SubscriberAgent 구현
  - `internal/node/bridge.go`: Bridge Init/Process/Shutdown 확장
  - `internal/node/bridge_config.go`: BridgeConfig.Topics 필드 추가
- **Tier**: Tier 2 - 내부 실행 계층 (internal)
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-001): Agent 인터페이스, MessageReceiver 인터페이스
  - `internal/node/` (SPEC-BRIDGE-001): BridgeNode, BridgeConfig, AgentTransport
  - `internal/engine/` (SPEC-ENGINE-001): AgentManagerResolver, agentTransportAdapter
  - `pkg/flow/` (SPEC-FLOW-001): AgentRef, BridgeDirection
  - `pkg/message/` (SPEC-MSG-001): Message 인터페이스
  - `github.com/eclipse/paho.mqtt.golang`: MQTT 클라이언트 라이브러리
- **테스트 프레임워크**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **동시성 모델**:
  - `sync.RWMutex` (구독 토픽 목록 보호)
  - `sync.Mutex` (Bridge 내 bridge-added 토픽 추적)

### 1.3 설계 원칙

- **인터페이스 분리**: SubscriberAgent는 Agent 인터페이스와 독립적인 선택적(optional) 인터페이스로 정의하여, 구독 기능이 필요한 에이전트만 구현
- **소유권 추적**: Bridge가 추가한 토픽과 에이전트 자체 설정 토픽을 구분하여 Shutdown 시 Bridge 소유 토픽만 해제
- **호환성 유지**: 기존 `MQTTSubscriberConfig.Topics`를 통한 구독은 변경 없이 유지. SubscriberAgent를 구현하지 않는 에이전트에 연결된 Bridge는 토픽 구독 기능을 무시
- **제어 메시지 규약**: 런타임 토픽 구독/해제는 BridgeOut 방향의 Process()에서 특수 action 필드를 감지하여 처리

### 1.4 스코프 경계

**IN SCOPE (본 SPEC 범위)**:
- `SubscriberAgent` 인터페이스 정의 (`internal/agent/agent.go`)
- `MQTTSubscriberAgent`에 Subscribe/Unsubscribe 메서드 추가
- `BridgeConfig`에 `Topics []string` 필드 추가
- Bridge Init()에서 설정 토픽 자동 구독 로직
- Bridge Process()에서 런타임 제어 메시지 감지 및 구독/해제 처리
- Bridge Shutdown()에서 bridge-added 토픽 정리 로직
- 위 모든 기능에 대한 단위/통합 테스트

**OUT OF SCOPE (별도 SPEC)**:
- MQTT Publisher 에이전트 (별도 SPEC)
- 토픽 와일드카드 패턴 검증 (현재는 문자열 그대로 전달)
- 에이전트 간 토픽 구독 충돌 해소 (동일 토픽을 여러 Bridge가 구독하는 경우)
- QoS 레벨별 구독 관리 (현재는 에이전트 설정 QoS 사용)
- MQTT 5.0 Shared Subscription 지원

### 1.5 관련 SPEC

| SPEC ID | 관계 | 설명 |
|---------|------|------|
| SPEC-AGENT-001 | 의존 | Agent 인터페이스, MessageReceiver 인터페이스 |
| SPEC-SYSAGENT-001 | 확장 | MQTTSubscriberAgent 구현 |
| SPEC-BRIDGE-001 | 확장 | BridgeNode, BridgeConfig, 통신 모드 |
| SPEC-ENGINE-001 | 의존 | AgentManagerResolver, agentTransportAdapter |
| SPEC-MSG-001 | 의존 | Message 인터페이스 |
| SPEC-FLOW-001 | 의존 | AgentRef, BridgeDirection |

---

## 2. Assumptions (가정)

### 2.1 기술적 가정

- A1: SPEC-BRIDGE-001의 BridgeNode, BridgeConfig, AgentTransport, AgentResolver가 구현되어 있다
- A2: SPEC-AGENT-001의 Agent 인터페이스와 MessageReceiver 인터페이스가 구현되어 있다
- A3: `MQTTSubscriberAgent`가 `agent.Agent`와 `agent.MessageReceiver` 인터페이스를 구현하고, 내부에 `mqtt.Client`와 `subscribe(c mqtt.Client)` 메서드를 가지고 있다
- A4: `agentTransportAdapter`가 `agent.Agent`를 래핑하여 `node.AgentTransport` 인터페이스를 제공하며, 내부 `agent` 필드에 원본 Agent에 접근할 수 있다
- A5: Bridge의 BridgeOut 방향 Process()는 메시지를 `transport.Send()`로 에이전트에 전달한다
- A6: MQTT 클라이언트의 `Subscribe()` 및 `Unsubscribe()` 메서드는 동시 호출에 안전하다 (paho.mqtt.golang 보장)

### 2.2 도메인 가정

- A7: Bridge 설정에 정의된 토픽은 Bridge 생명주기에 종속되며, Bridge Shutdown 시 자동 해제되어야 한다
- A8: 런타임 제어 메시지로 추가된 토픽도 해당 Bridge 생명주기에 종속된다
- A9: 에이전트 자체 설정(`MQTTSubscriberConfig.Topics`)으로 구독된 토픽은 Bridge가 관여하지 않는다
- A10: 제어 메시지 형식은 `{"action": "subscribe", "topics": ["topic1", "topic2"]}` 또는 `{"action": "unsubscribe", "topics": ["topic1"]}` 이다
- A11: 제어 메시지는 BridgeOut 방향의 Process()를 통해 전달되며, action 필드가 없는 일반 메시지는 기존 로직대로 에이전트에 전달된다
- A12: 이미 구독 중인 토픽에 대해 중복 Subscribe를 호출하면, MQTT 클라이언트가 멱등적으로 처리한다

---

## 3. Requirements (요구사항)

### REQ-1: SubscriberAgent 인터페이스

#### REQ-MQTT-001-01-01 (Ubiquitous) SubscriberAgent 인터페이스 정의

시스템은 **항상** `internal/agent/agent.go`에 다음 메서드를 포함하는 `SubscriberAgent` 인터페이스를 제공해야 한다:

- `Subscribe(ctx context.Context, topics []string) error`: 지정된 토픽 목록을 동적으로 구독
- `Unsubscribe(ctx context.Context, topics []string) error`: 지정된 토픽 목록의 구독을 해제

#### REQ-MQTT-001-01-02 (Ubiquitous) 선택적 인터페이스

시스템은 **항상** `SubscriberAgent`를 `Agent` 인터페이스와 독립적인 선택적(optional) 인터페이스로 유지해야 한다. `SubscriberAgent`를 구현하지 않는 에이전트는 기존 동작에 영향을 받지 않아야 한다.

---

### REQ-2: MQTT Agent implements SubscriberAgent

#### REQ-MQTT-001-02-01 (Ubiquitous) MQTTSubscriberAgent Subscribe 구현

시스템은 **항상** `MQTTSubscriberAgent`에 `Subscribe(ctx context.Context, topics []string) error` 메서드를 제공해야 한다:

- MQTT 클라이언트가 연결된 상태에서 각 토픽을 에이전트의 QoS 레벨로 구독
- 구독 성공한 토픽을 내부 구독 토픽 목록에 추가
- 클라이언트가 연결되지 않은 상태이면 에러 반환

#### REQ-MQTT-001-02-02 (Ubiquitous) MQTTSubscriberAgent Unsubscribe 구현

시스템은 **항상** `MQTTSubscriberAgent`에 `Unsubscribe(ctx context.Context, topics []string) error` 메서드를 제공해야 한다:

- MQTT 클라이언트의 `Unsubscribe()` 메서드를 호출하여 지정 토픽 구독 해제
- 해제된 토픽을 내부 구독 토픽 목록에서 제거
- 클라이언트가 연결되지 않은 상태이면 에러 반환

#### REQ-MQTT-001-02-03 (Ubiquitous) 구독 토픽 추적

시스템은 **항상** `MQTTSubscriberAgent` 내부에 현재 구독 중인 토픽 목록을 관리해야 한다. 이 목록은 `sync.RWMutex`로 동시성 보호되어야 한다.

#### REQ-MQTT-001-02-04 (Ubiquitous) 컴파일 타임 인터페이스 체크

시스템은 **항상** `var _ agent.SubscriberAgent = (*MQTTSubscriberAgent)(nil)` 컴파일 타임 체크를 포함해야 한다.

---

### REQ-3: Bridge Node Config Topics

#### REQ-MQTT-001-03-01 (Ubiquitous) BridgeConfig Topics 필드

시스템은 **항상** `BridgeConfig` 구조체에 `Topics []string` 필드를 포함해야 한다. 이 필드는 Bridge 초기화 시 자동 구독할 토픽 목록을 정의한다.

#### REQ-MQTT-001-03-02 (Event-Driven) Bridge Init 토픽 자동 구독

**WHEN** `BridgeNode.Init(ctx)` 호출 시, 해석된 에이전트가 `SubscriberAgent` 인터페이스를 구현하고 `BridgeConfig.Topics`가 비어 있지 않으면, **THEN** `SubscriberAgent.Subscribe(ctx, topics)` 를 호출하여 설정된 토픽을 구독해야 한다.

#### REQ-MQTT-001-03-03 (State-Driven) SubscriberAgent 미구현 시 무시

**IF** 해석된 에이전트가 `SubscriberAgent` 인터페이스를 구현하지 않으면, **THEN** `BridgeConfig.Topics`가 설정되어 있더라도 토픽 구독을 시도하지 않고, 경고 로그를 출력해야 한다.

#### REQ-MQTT-001-03-04 (Ubiquitous) Bridge-added 토픽 추적

시스템은 **항상** BridgeNode 내부에 Bridge가 추가한 토픽 목록(`bridgeTopics`)을 관리해야 한다. Config 토픽과 런타임 제어 메시지로 추가된 토픽을 모두 포함한다.

---

### REQ-4: Runtime Control Messages via Bridge Process

#### REQ-MQTT-001-04-01 (Event-Driven) 제어 메시지 감지

**WHEN** BridgeOut 방향의 `Process(ctx, msg)` 호출 시, 메시지 Payload에 `"action"` 필드가 존재하면, **THEN** 제어 메시지로 인식하고 일반 송신 대신 제어 로직을 실행해야 한다.

#### REQ-MQTT-001-04-02 (Event-Driven) Subscribe 제어 메시지 처리

**WHEN** 제어 메시지의 `"action"` 값이 `"subscribe"`이고, `"topics"` 필드에 문자열 배열이 있으면, **THEN** 해석된 에이전트가 `SubscriberAgent`를 구현하는 경우 `Subscribe(ctx, topics)`를 호출하고, 성공한 토픽을 `bridgeTopics`에 추가해야 한다.

#### REQ-MQTT-001-04-03 (Event-Driven) Unsubscribe 제어 메시지 처리

**WHEN** 제어 메시지의 `"action"` 값이 `"unsubscribe"`이고, `"topics"` 필드에 문자열 배열이 있으면, **THEN** 해석된 에이전트가 `SubscriberAgent`를 구현하는 경우 `Unsubscribe(ctx, topics)`를 호출하고, 해당 토픽을 `bridgeTopics`에서 제거해야 한다.

#### REQ-MQTT-001-04-04 (State-Driven) SubscriberAgent 미구현 시 제어 메시지 거부

**IF** 해석된 에이전트가 `SubscriberAgent` 인터페이스를 구현하지 않으면, **THEN** subscribe/unsubscribe 제어 메시지에 대해 에러를 반환해야 한다.

#### REQ-MQTT-001-04-05 (Unwanted) 유효하지 않은 제어 메시지

시스템은 `"action"` 값이 `"subscribe"` 또는 `"unsubscribe"`가 아니거나 `"topics"` 필드가 없는 제어 메시지에 대해 에러를 반환**해야 한다**.

#### REQ-MQTT-001-04-06 (State-Driven) 일반 메시지 통과

**IF** 메시지 Payload에 `"action"` 필드가 존재하지 않으면, **THEN** 기존 BridgeOut 처리 로직(변환 후 에이전트 전송)을 그대로 수행해야 한다.

---

### REQ-5: Cleanup on Bridge Shutdown

#### REQ-MQTT-001-05-01 (Event-Driven) Bridge Shutdown 토픽 정리

**WHEN** `BridgeNode.Shutdown(ctx)` 호출 시, `bridgeTopics` 목록이 비어 있지 않고 에이전트가 `SubscriberAgent`를 구현하면, **THEN** `Unsubscribe(ctx, bridgeTopics)`를 호출하여 Bridge가 추가한 토픽만 구독 해제해야 한다.

#### REQ-MQTT-001-05-02 (Unwanted) 에이전트 자체 토픽 보호

시스템은 Bridge Shutdown 시 에이전트 자체 설정(`MQTTSubscriberConfig.Topics`)으로 구독된 토픽을 구독 해제**하지 않아야 한다**.

#### REQ-MQTT-001-05-03 (Event-Driven) Shutdown 에러 처리

**WHEN** Shutdown 중 `Unsubscribe()` 호출이 실패하면, **THEN** 에러를 로그에 기록하되, Bridge Shutdown 프로세스는 계속 진행해야 한다.

---

## 4. Specifications (명세)

### 4.1 파일 구조 (변경 대상)

```
internal/agent/
├── agent.go                    // [수정] SubscriberAgent 인터페이스 추가
└── system/
    ├── mqtt_subscriber.go      // [수정] Subscribe, Unsubscribe 메서드 추가, 구독 토픽 추적
    └── mqtt_subscriber_test.go // [수정] SubscriberAgent 구현 테스트 추가

internal/node/
├── bridge.go                   // [수정] Init/Process/Shutdown 확장
├── bridge_config.go            // [수정] Topics 필드 추가
├── bridge_test.go              // [수정] 토픽 구독 관련 테스트 추가
└── bridge_config_test.go       // [수정] Topics 필드 검증 테스트 추가

internal/engine/
└── resolver.go                 // [수정] agentTransportAdapter에 SubscriberAgent 접근 지원
```

### 4.2 타입 시그니처

```go
// ── internal/agent/agent.go ──

// SubscriberAgent 는 동적 토픽 구독/해제를 지원하는 에이전트의 선택적 인터페이스이다.
// MQTT Subscriber 등 메시지 브로커 기반 에이전트가 구현한다.
// Bridge Node에서 이 인터페이스 존재 여부를 확인하여 토픽 관리에 사용한다.
type SubscriberAgent interface {
    Subscribe(ctx context.Context, topics []string) error
    Unsubscribe(ctx context.Context, topics []string) error
}
```

```go
// ── internal/agent/system/mqtt_subscriber.go ──

// MQTTSubscriberAgent 확장 필드
type MQTTSubscriberAgent struct {
    // ... 기존 필드 ...
    subscribedTopics []string       // 현재 구독 중인 전체 토픽 목록
    topicsMu         sync.RWMutex   // 토픽 목록 동시성 보호
}

// 컴파일 타임 인터페이스 체크
var _ agent.SubscriberAgent = (*MQTTSubscriberAgent)(nil)

func (a *MQTTSubscriberAgent) Subscribe(ctx context.Context, topics []string) error
func (a *MQTTSubscriberAgent) Unsubscribe(ctx context.Context, topics []string) error
```

```go
// ── internal/node/bridge_config.go ──

type BridgeConfig struct {
    // ... 기존 필드 ...
    Topics []string  // Bridge 초기화 시 자동 구독할 토픽 목록
}
```

```go
// ── internal/node/bridge.go ──

type BridgeNode struct {
    // ... 기존 필드 ...
    bridgeTopics []string    // Bridge가 추가한 토픽 목록 (config + runtime)
    topicsMu     sync.Mutex  // bridgeTopics 동시성 보호
}

// Init 확장: SubscriberAgent 검사 + 설정 토픽 구독
// Process 확장: 제어 메시지 감지 및 처리
// Shutdown 확장: bridgeTopics 구독 해제
```

```go
// ── internal/engine/resolver.go ──

// agentTransportAdapter 확장 메서드
// UnderlyingAgent 는 래핑된 원본 Agent를 반환한다.
func (t *agentTransportAdapter) UnderlyingAgent() agent.Agent
```

### 4.3 제어 메시지 형식

```json
// Subscribe 제어 메시지
{
    "action": "subscribe",
    "topics": ["sensor/temperature", "sensor/humidity"]
}

// Unsubscribe 제어 메시지
{
    "action": "unsubscribe",
    "topics": ["sensor/humidity"]
}
```

### 4.4 Bridge 토픽 관리 흐름

```
Bridge Init():
  1. resolver.ResolveAgent(ctx, agentRef) -> transport
  2. transport.(AgentAccessor).UnderlyingAgent() -> agent
  3. agent.(SubscriberAgent) 타입 확인
  4. IF SubscriberAgent AND config.Topics 비어있지 않음:
     - agent.Subscribe(ctx, config.Topics)
     - bridgeTopics = append(bridgeTopics, config.Topics...)
  5. 기존 Init 로직 계속

Bridge Process() [BridgeOut, 제어 메시지]:
  1. msg.Payload에서 "action" 필드 확인
  2. IF action == "subscribe":
     - topics := msg.Payload["topics"].([]string)
     - agent.(SubscriberAgent).Subscribe(ctx, topics)
     - bridgeTopics = append(bridgeTopics, topics...)
  3. IF action == "unsubscribe":
     - topics := msg.Payload["topics"].([]string)
     - agent.(SubscriberAgent).Unsubscribe(ctx, topics)
     - bridgeTopics에서 해당 토픽 제거
  4. IF action 없음:
     - 기존 BridgeOut 로직 (transport.Send)

Bridge Shutdown():
  1. IF bridgeTopics 비어있지 않음 AND agent가 SubscriberAgent 구현:
     - agent.(SubscriberAgent).Unsubscribe(ctx, bridgeTopics)
  2. 기존 Shutdown 로직 계속
```

### 4.5 AgentAccessor 인터페이스

Bridge에서 `AgentTransport`를 통해 원본 `Agent`에 접근하기 위한 인터페이스:

```go
// internal/node/bridge.go 또는 internal/engine/resolver.go

// AgentAccessor 는 AgentTransport에서 원본 Agent에 접근하기 위한 인터페이스이다.
type AgentAccessor interface {
    UnderlyingAgent() agent.Agent
}
```

`agentTransportAdapter`가 이 인터페이스를 구현하여, Bridge가 transport에서 원본 Agent를 꺼내 SubscriberAgent 타입 확인을 수행할 수 있다.

---

## 5. Traceability (추적성)

| 요구사항 ID | REQ | 파일 | 우선순위 |
|------------|-----|------|---------|
| REQ-MQTT-001-01-01 ~ 01-02 | REQ-1: SubscriberAgent Interface | internal/agent/agent.go | P0 |
| REQ-MQTT-001-02-01 ~ 02-04 | REQ-2: MQTT Agent implements SubscriberAgent | internal/agent/system/mqtt_subscriber.go | P0 |
| REQ-MQTT-001-03-01 ~ 03-04 | REQ-3: Bridge Node Config Topics | internal/node/bridge_config.go, bridge.go | P0 |
| REQ-MQTT-001-04-01 ~ 04-06 | REQ-4: Runtime Control Messages | internal/node/bridge.go | P1 |
| REQ-MQTT-001-05-01 ~ 05-03 | REQ-5: Cleanup on Bridge Shutdown | internal/node/bridge.go | P0 |
