---
id: SPEC-MQTT-001
type: plan
version: "1.0.0"
spec_ref: SPEC-MQTT-001
---

# SPEC-MQTT-001 구현 계획

## 1. 구현 전략 개요

### 1.1 개발 방법론

- **Hybrid 모드** (quality.yaml 설정 기반)
  - 신규 코드 (SubscriberAgent 인터페이스, Subscribe/Unsubscribe 메서드): TDD (RED-GREEN-REFACTOR)
  - 기존 파일 수정 (bridge.go, bridge_config.go, resolver.go): DDD (ANALYZE-PRESERVE-IMPROVE)
- 테스트 먼저 작성 후 구현
- 85%+ 테스트 커버리지 목표
- `go test -race` 필수 실행 (동시성 안전 검증)

### 1.2 기술 스택

- **언어**: Go 1.23+
- **테스트**: Go 표준 `testing` 패키지 + `github.com/stretchr/testify`
- **MQTT**: `github.com/eclipse/paho.mqtt.golang` (기존 의존성)
- **동시성**:
  - `sync.RWMutex` (MQTTSubscriberAgent 구독 토픽 목록 보호)
  - `sync.Mutex` (BridgeNode bridgeTopics 목록 보호)
- **의존 패키지**:
  - `internal/agent/` (SPEC-AGENT-001): Agent, MessageReceiver 인터페이스
  - `internal/node/` (SPEC-BRIDGE-001): BridgeNode, BridgeConfig
  - `internal/engine/` (SPEC-ENGINE-001): AgentManagerResolver, agentTransportAdapter
  - `pkg/message/` (SPEC-MSG-001): Message, Payload
  - `pkg/flow/` (SPEC-FLOW-001): AgentRef, BridgeDirection

### 1.3 패키지 위치

- `internal/agent/agent.go`: SubscriberAgent 인터페이스
- `internal/agent/system/mqtt_subscriber.go`: 구현
- `internal/node/bridge.go`, `bridge_config.go`: Bridge 확장
- `internal/engine/resolver.go`: AgentAccessor 지원

---

## 2. 마일스톤

### Primary Goal: SubscriberAgent 인터페이스 + MQTT 구현 (REQ-1, REQ-2)

**범위**: REQ-MQTT-001-01-01 ~ 02-04

**작업 항목**:

1. `internal/agent/agent.go` 수정
   - `SubscriberAgent` 인터페이스 정의 (Subscribe, Unsubscribe 메서드)
   - 기존 `Agent`, `MessageReceiver` 인터페이스와 동일한 스타일로 GoDoc 주석 작성

2. `internal/agent/system/mqtt_subscriber.go` 수정
   - `MQTTSubscriberAgent` 구조체에 `subscribedTopics []string`, `topicsMu sync.RWMutex` 필드 추가
   - `Subscribe(ctx, topics)` 메서드 구현:
     - MQTT 클라이언트 연결 상태 확인
     - `a.client.Subscribe(topic, qos, nil)` 호출
     - 성공한 토픽을 `subscribedTopics`에 추가
   - `Unsubscribe(ctx, topics)` 메서드 구현:
     - `a.client.Unsubscribe(topics...)` 호출
     - 해제된 토픽을 `subscribedTopics`에서 제거
   - `var _ agent.SubscriberAgent = (*MQTTSubscriberAgent)(nil)` 컴파일 타임 체크 추가
   - 기존 `subscribe(c)` 메서드에서 `subscribedTopics` 초기화 추가

3. `internal/agent/system/mqtt_subscriber_test.go` 수정
   - Subscribe 성공 테스트 (Mock MQTT Client)
   - Unsubscribe 성공 테스트
   - 연결되지 않은 상태에서 Subscribe 시 에러 테스트
   - 연결되지 않은 상태에서 Unsubscribe 시 에러 테스트
   - 동시성 안전 테스트 (`go test -race`)
   - subscribedTopics 목록 정확성 테스트

**산출물**: SubscriberAgent 인터페이스와 MQTTSubscriberAgent 구현 완성

---

### Secondary Goal: Bridge Config Topics + Init 구독 (REQ-3)

**범위**: REQ-MQTT-001-03-01 ~ 03-04

**작업 항목**:

1. `internal/node/bridge_config.go` 수정
   - `BridgeConfig` 구조체에 `Topics []string` 필드 추가
   - `DefaultBridgeConfig()` 수정: `Topics` 기본값 `nil` (빈 슬라이스)

2. `internal/engine/resolver.go` 수정
   - `agentTransportAdapter`에 `UnderlyingAgent() agent.Agent` 메서드 추가
   - `AgentAccessor` 인터페이스를 `internal/node/` 또는 `internal/engine/`에 정의

3. `internal/node/bridge.go` 수정
   - `BridgeNode` 구조체에 `bridgeTopics []string`, `topicsMu sync.Mutex` 필드 추가
   - `Init(ctx)` 확장:
     - transport에서 원본 Agent 획득 (`transport.(AgentAccessor).UnderlyingAgent()`)
     - `agent.(agent.SubscriberAgent)` 타입 확인
     - `SubscriberAgent`이고 `bridgeConfig.Topics` 비어있지 않으면 Subscribe 호출
     - 성공한 토픽을 `bridgeTopics`에 추가

4. 테스트 작성
   - `bridge_config_test.go`: Topics 필드가 있는 BridgeConfig 검증 테스트
   - `bridge_test.go`:
     - Init 시 SubscriberAgent에 대해 설정 토픽 자동 구독 테스트
     - SubscriberAgent 미구현 에이전트에서 Topics 설정 시 무시 테스트
     - Subscribe 실패 시 Init 에러 처리 테스트

**산출물**: Bridge 설정 기반 토픽 자동 구독 완성

---

### Tertiary Goal: Runtime Control Messages (REQ-4)

**범위**: REQ-MQTT-001-04-01 ~ 04-06

**작업 항목**:

1. `internal/node/bridge.go` 수정 - Process 확장
   - BridgeOut 방향의 `Process()` 내부에 제어 메시지 감지 로직 추가
   - 메시지 Payload에서 `"action"` 필드 확인
   - `"subscribe"` 액션 처리: topics 추출 -> SubscriberAgent.Subscribe() -> bridgeTopics 추가
   - `"unsubscribe"` 액션 처리: topics 추출 -> SubscriberAgent.Unsubscribe() -> bridgeTopics 제거
   - `"action"` 필드 없는 일반 메시지: 기존 BridgeOut 로직 유지
   - SubscriberAgent 미구현 시 에러 반환
   - 유효하지 않은 action 값 또는 topics 누락 시 에러 반환

2. 헬퍼 함수 작성
   - `isControlMessage(msg message.Message) bool`: action 필드 존재 여부 확인
   - `extractControlAction(msg message.Message) (action string, topics []string, err error)`: 제어 메시지 파싱

3. 테스트 작성
   - Subscribe 제어 메시지 처리 성공 테스트
   - Unsubscribe 제어 메시지 처리 성공 테스트
   - SubscriberAgent 미구현 에이전트에 제어 메시지 전달 시 에러 테스트
   - 유효하지 않은 action 값 에러 테스트
   - topics 필드 누락 에러 테스트
   - action 필드 없는 일반 메시지가 기존 로직으로 통과하는 테스트

**산출물**: 런타임 제어 메시지 기반 동적 토픽 관리 완성

---

### Final Goal: Cleanup on Shutdown + 통합 테스트 (REQ-5)

**범위**: REQ-MQTT-001-05-01 ~ 05-03, 통합

**작업 항목**:

1. `internal/node/bridge.go` 수정 - Shutdown 확장
   - Shutdown 시작 단계에서 `bridgeTopics`가 비어있지 않으면 Unsubscribe 호출
   - Unsubscribe 에러는 로그에 기록하되 Shutdown 진행
   - 기존 Shutdown 로직(수신 루프 취소, CorrelationTracker 닫기 등) 유지

2. 통합 테스트 작성
   - 전체 흐름: Init(Config Topics) -> Process(Subscribe Control) -> Process(Unsubscribe Control) -> Shutdown(Cleanup)
   - Shutdown 후 bridgeTopics가 모두 해제되었는지 검증
   - 에이전트 자체 설정 토픽이 Shutdown 후에도 유지되는지 검증
   - Unsubscribe 실패 시 Shutdown이 정상 완료되는지 검증
   - 동시성 안전 테스트 (`go test -race`)

3. 벤치마크 테스트 (선택적)
   - Subscribe/Unsubscribe 호출 지연시간
   - 제어 메시지 감지 및 파싱 지연시간

**산출물**: Bridge Shutdown 토픽 정리 및 전체 통합 검증 완성

---

## 3. 기술적 접근

### 3.1 SubscriberAgent 인터페이스 설계

```
agent.Agent          (기존: 모든 에이전트의 기본 인터페이스)
agent.MessageReceiver (기존: 비동기 수신 에이전트)
agent.SubscriberAgent (신규: 동적 구독 관리 에이전트)

MQTTSubscriberAgent implements:
  - agent.Agent
  - agent.MessageReceiver
  - agent.SubscriberAgent  <-- 신규
```

Go의 인터페이스 합성을 활용하여, SubscriberAgent는 Agent와 독립적으로 정의한다. Bridge에서는 타입 단언(`agent.(SubscriberAgent)`)으로 지원 여부를 확인한다.

### 3.2 Agent 접근 경로

Bridge는 `AgentTransport` 인터페이스를 통해 에이전트와 통신한다. SubscriberAgent 접근을 위해 transport에서 원본 Agent를 획득하는 경로가 필요하다:

```
BridgeNode
  -> transport (AgentTransport 인터페이스)
  -> transport.(AgentAccessor).UnderlyingAgent() (원본 Agent)
  -> agent.(SubscriberAgent) (타입 단언)
```

`agentTransportAdapter`가 `AgentAccessor` 인터페이스를 구현하여 이 경로를 제공한다.

### 3.3 BridgeOut Process 제어 메시지 분기

```
Process(ctx, msg) [BridgeOut]:
  1. action := msg.Payload().Get("action")
  2. IF action 존재:
     - topics := msg.Payload().Get("topics") -> []string 변환
     - subscriber := transport.(AgentAccessor).UnderlyingAgent().(SubscriberAgent)
     - IF action == "subscribe":
         subscriber.Subscribe(ctx, topics)
         topicsMu.Lock()
         bridgeTopics = append(bridgeTopics, topics...)
         topicsMu.Unlock()
     - IF action == "unsubscribe":
         subscriber.Unsubscribe(ctx, topics)
         topicsMu.Lock()
         bridgeTopics = removeTopic(bridgeTopics, topics)
         topicsMu.Unlock()
     - ELSE: 에러 (unknown action)
  3. ELSE (일반 메시지):
     - 기존 BridgeOut 로직 (transform + send)
```

### 3.4 토픽 소유권 추적

```
에이전트 자체 토픽: MQTTSubscriberConfig.Topics -> subscribe(c) 에서 구독
                   에이전트 Stop()에서 해제
                   Bridge가 관여하지 않음

Bridge 설정 토픽:  BridgeConfig.Topics -> Init()에서 구독
                   -> bridgeTopics에 추가
                   -> Shutdown()에서 해제

Runtime 토픽:      Process() 제어 메시지 -> 동적 구독/해제
                   -> bridgeTopics에 추가/제거
                   -> Shutdown()에서 남은 토픽 해제
```

### 3.5 MQTTSubscriberAgent Subscribe 구현 전략

기존 `subscribe(c mqtt.Client)` 메서드는 onConnect 핸들러에서 호출되어 초기 설정 토픽을 구독한다. 새로운 `Subscribe(ctx, topics)` 메서드는 이와 유사하지만:

- 임의 시점에 호출 가능 (런타임 동적 구독)
- MQTT 클라이언트의 현재 연결 상태를 확인
- `subscribedTopics` 목록을 갱신하여 추적
- `ctx` 를 통한 취소 지원 (토큰 대기 타임아웃)

```
Subscribe(ctx, topics):
  IF !a.client.IsConnected():
    return error("not connected")
  FOR each topic in topics:
    token := a.client.Subscribe(topic, a.mqttConfig.QoS, nil)
    token.Wait()
    IF token.Error():
      log error, continue (partial failure 허용)
    ELSE:
      topicsMu.Lock()
      subscribedTopics = append(subscribedTopics, topic)
      topicsMu.Unlock()
  return nil (또는 부분 실패 시 에러 집계)
```

---

## 4. 리스크 및 대응

### Risk 1: MQTT 클라이언트 동시 접근

- **위험**: Subscribe/Unsubscribe가 여러 Bridge에서 동시에 호출될 수 있음
- **대응**: paho.mqtt.golang의 `Subscribe()`/`Unsubscribe()` 메서드는 내부적으로 동시성 안전. `subscribedTopics` 목록은 `sync.RWMutex`로 보호. `go test -race` 필수 실행

### Risk 2: 에이전트 재연결 시 토픽 유실

- **위험**: MQTT 재연결 시 기존 `onConnect` 핸들러가 `MQTTSubscriberConfig.Topics`만 재구독하여, Bridge가 추가한 토픽이 유실될 수 있음
- **대응**: `subscribedTopics` 전체 목록을 `onConnect` 핸들러에서 재구독하도록 `subscribe(c)` 메서드를 확장. 이렇게 하면 재연결 시 Bridge 추가 토픽도 자동 복구

### Risk 3: 제어 메시지와 일반 메시지 구분 모호성

- **위험**: 일반 에이전트 데이터에 우연히 `"action"` 필드가 포함된 경우 제어 메시지로 오인
- **대응**: 제어 메시지 판별 시 `"action"` 필드의 값이 `"subscribe"` 또는 `"unsubscribe"`이고 `"topics"` 필드가 동시에 존재하는 경우에만 제어 메시지로 처리. 하나라도 누락이면 일반 메시지로 통과

### Risk 4: Shutdown 시 에이전트 이미 연결 해제

- **위험**: Bridge Shutdown 시점에 MQTT 에이전트가 이미 Stop되어 Unsubscribe 불가
- **대응**: Unsubscribe 실패를 로그에 기록하되 Shutdown 프로세스 진행. 에이전트 Stop 시 자체적으로 모든 구독이 해제되므로 실질적 문제 없음

### Risk 5: AgentTransport에서 원본 Agent 접근 불가

- **위험**: `agentTransportAdapter` 이외의 `AgentTransport` 구현체에서 `UnderlyingAgent()` 미구현
- **대응**: `AgentAccessor` 인터페이스를 타입 단언으로 확인. 미구현 시 SubscriberAgent 기능을 무시하고 경고 로그 출력. 핵심 Bridge 기능에 영향 없음

---

## 5. 의존성 그래프

```
internal/agent/agent.go (SubscriberAgent 인터페이스)
  └── 소비자: internal/agent/system/mqtt_subscriber.go (구현)
  └── 소비자: internal/node/bridge.go (타입 단언으로 사용)

internal/agent/system/mqtt_subscriber.go (Subscribe, Unsubscribe 메서드)
  ├── 의존: internal/agent/agent.go (SubscriberAgent 인터페이스)
  ├── 의존: github.com/eclipse/paho.mqtt.golang (MQTT 클라이언트)
  └── 소비자: internal/node/bridge.go (Bridge Init/Process/Shutdown에서 호출)

internal/node/bridge_config.go (Topics 필드)
  ├── 의존: pkg/flow/ (AgentRef)
  └── 소비자: internal/node/bridge.go (Init에서 참조)

internal/node/bridge.go (Bridge 확장)
  ├── 의존: internal/agent/agent.go (SubscriberAgent 인터페이스)
  ├── 의존: internal/node/bridge_config.go (Topics 필드)
  ├── 의존: internal/engine/resolver.go (AgentAccessor)
  └── 의존: pkg/message/ (Message Payload)

internal/engine/resolver.go (AgentAccessor 지원)
  ├── 의존: internal/agent/agent.go (Agent 인터페이스)
  └── 소비자: internal/node/bridge.go (UnderlyingAgent 호출)
```

---

## 6. 구현 순서 (파일별)

| 순서 | 파일 | 설명 | 의존성 |
|------|------|------|--------|
| 1 | internal/agent/agent.go | SubscriberAgent 인터페이스 정의 | 없음 (기존 파일에 추가) |
| 2 | internal/agent/system/mqtt_subscriber.go | Subscribe, Unsubscribe 구현 | agent.go (SubscriberAgent) |
| 3 | internal/engine/resolver.go | UnderlyingAgent() 메서드 추가 | agent.go (Agent) |
| 4 | internal/node/bridge_config.go | Topics 필드 추가 | 없음 (기존 파일에 추가) |
| 5 | internal/node/bridge.go | Init/Process/Shutdown 확장 | 1~4 전체 |

모든 파일에 대해 Hybrid 방식(신규 TDD, 수정 DDD)으로 테스트 파일(`*_test.go`)을 작성한다.
