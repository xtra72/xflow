---
id: SPEC-MQTT-003
version: "1.1.0"
status: completed
created: "2026-03-14"
updated: "2026-03-15"
author: xtra
priority: high
---

# SPEC-MQTT-003 구현 계획: MQTT Subscriber / Publisher 노드

## 개요

MQTT 에이전트에 직접 연결하는 전용 노드 2종(mqtt-subscriber, mqtt-publisher)을 구현한다. NASA 노드 패턴을 따르며, 기존 MQTTAdapter를 재사용한다.

---

## 마일스톤

### M1: mqtt-subscriber 노드 구현 [Priority High]

**목표**: SourceNode 기반 MQTT 메시지 수신 노드 완성

**태스크**:

1. `internal/node/mqtt.go` 파일 생성
2. `MQTTSubscriberNodeConfig`, `TopicConfig` 구조체 정의
3. `MQTTSubscriberNode` 구조체 정의
4. `NewMQTTSubscriberNode` 팩토리 함수 구현
   - BaseNode 생성, config에서 AgentResolver 추출
5. `Configure()` 구현
   - agent_ref(필수), topics(필수) 검증
   - payload_format, buffer_size 기본값 설정
   - MQTTAdapter 인스턴스 생성
6. `Init()` 구현
   - AgentResolver를 통한 에이전트 해석
   - AgentAccessor를 통한 원본 Agent 접근
   - SubscriberAgent, MessageReceiver 인터페이스 확인
   - 설정 토픽 구독 (Subscribe 호출)
   - receiveLoop 고루틴 시작
7. `receiveLoop()` 구현
   - ReceiveMessage 블로킹 호출
   - MQTTAdapter.TransformToFlow 변환
   - sourceCh 전송 (overflow 시 드롭)
   - stopCh 감시
8. `Process()` 구현 (passthrough)
9. `Shutdown()` 구현
   - stopCh close (sync.Once)
   - Unsubscribe 호출
   - 상태 전이
10. `SourceCh()` 구현
11. 에러 변수 정의 (ErrMQTTMissingAgentRef, ErrMQTTMissingTopics 등)
12. 컴파일 타임 인터페이스 체크

**관련 요구사항**: REQ-MQTT-003-01-01 ~ 01-10

**의존성**: 없음 (신규 파일)

**참조 패턴**: `internal/node/nasa.go` (NASAStatusNode, nasaNodeBase)

---

### M2: mqtt-publisher 노드 구현 [Priority High]

**목표**: Process 기반 MQTT 메시지 발행 노드 완성

**태스크**:

1. `MQTTPublisherNodeConfig` 구조체 정의
2. `MQTTPublisherNode` 구조체 정의
3. `NewMQTTPublisherNode` 팩토리 함수 구현
4. `Configure()` 구현
   - agent_ref(필수) 검증
   - default_topic, default_qos(범위 0-2), default_retained 파싱
   - payload_format 기본값
   - MQTTAdapter 인스턴스 생성 (WithDefaultQoS, WithDefaultRetained, WithPublishTopic)
5. `Init()` 구현
   - AgentResolver를 통한 에이전트 해석
   - MessagePublisher 인터페이스 확인
6. `Process()` 구현
   - MQTTAdapter.TransformToAgent 변환
   - 토픽 결정 (adapter 결과 -> DefaultTopic -> 에러)
   - PublishMessage 호출
   - passthrough 출력
7. `Shutdown()` 구현 (상태 전이만)
8. 컴파일 타임 인터페이스 체크

**관련 요구사항**: REQ-MQTT-003-02-01 ~ 02-09

**의존성**: M1과 동일 파일(mqtt.go)에 구현, M1과 병렬 가능

**참조 패턴**: `internal/node/nasa.go` (NASAControlNode)

---

### M3: 노드 레지스트리 등록 [Priority High]

**목표**: 빌트인 노드 목록에 MQTT 노드 2종 등록

**태스크**:

1. `internal/node/registry.go`의 `registerBuiltins()` 함수에 추가:
   - `{"mqtt-subscriber", NewMQTTSubscriberNode, "io", "MQTT 에이전트에서 메시지를 구독 수신"}`
   - `{"mqtt-publisher", NewMQTTPublisherNode, "io", "MQTT 에이전트로 메시지를 발행"}`

**관련 요구사항**: REQ-MQTT-003-04-01

**의존성**: M1, M2 완료 후

---

### M4: 프론트엔드 스키마 [Priority Medium]

**목표**: 웹 UI에서 MQTT 노드 설정 가능

**태스크**:

1. `web/src/config/nodeSchemas.ts`에 mqtt-subscriber 설정 필드 추가
   - agent_ref (agent_select, required, options: ['mqtt'])
   - topics (string_list, required)
   - payload_format (select: json/raw)
   - buffer_size (number)
2. `web/src/config/nodeSchemas.ts`에 mqtt-publisher 설정 필드 추가
   - agent_ref (agent_select, required, options: ['mqtt'])
   - default_topic (string)
   - default_qos (select: 0/1/2)
   - default_retained (boolean)
   - payload_format (select: json/raw)
3. `web/src/pages/nodes/nodeTypeMeta.ts`에 mqtt-subscriber 메타 추가
   - description, ports, configFields, configExample
4. `web/src/pages/nodes/nodeTypeMeta.ts`에 mqtt-publisher 메타 추가
   - description, ports, configFields, configExample
5. `web/src/components/property/StringListEditor.tsx` 생성
   - string[] 편집 컴포넌트 (추가/삭제, 빈 행 보존)
   - useState 기반 내부 상태 관리
6. `web/src/components/property/FormField.tsx` 수정
   - string_list 타입 → StringListEditor 연동
   - AgentSelectInput에 agentTypes 필터 추가 (field.options 기반)
7. `web/src/types/node.ts` 수정
   - ConfigField.type 유니온에 'string_list' 추가

**관련 요구사항**: REQ-MQTT-003-03-01 ~ 03-04

**의존성**: 독립적 (백엔드와 병렬 가능)

---

### M5: 테스트 [Priority High]

**목표**: 85%+ 코드 커버리지 달성

**태스크**:

1. `internal/node/mqtt_test.go` 파일 생성
2. Mock 구조체 정의:
   - `mockMQTTAgent`: SubscriberAgent + MessageReceiver + MessagePublisher 구현
   - `mockMQTTResolver`: AgentResolver 구현
   - `mockMQTTTransport`: AgentTransport + AgentAccessor 구현
3. mqtt-subscriber 테스트:
   - `TestNewMQTTSubscriberNode_Basic`: 팩토리 함수 기본 생성
   - `TestMQTTSubscriberNode_Configure_Valid`: 유효한 설정
   - `TestMQTTSubscriberNode_Configure_MissingAgentRef`: agent_ref 누락 에러
   - `TestMQTTSubscriberNode_Configure_MissingTopics`: topics 누락 에러
   - `TestMQTTSubscriberNode_Init_Success`: 정상 초기화 + 구독
   - `TestMQTTSubscriberNode_Init_NoResolver`: resolver 미설정 에러
   - `TestMQTTSubscriberNode_Init_AgentNotSubscriber`: 인터페이스 미구현 에러
   - `TestMQTTSubscriberNode_ReceiveLoop`: 메시지 수신 -> sourceCh 전달
   - `TestMQTTSubscriberNode_Shutdown`: 구독 해제 + 고루틴 종료
4. mqtt-publisher 테스트:
   - `TestNewMQTTPublisherNode_Basic`: 팩토리 함수 기본 생성
   - `TestMQTTPublisherNode_Configure_Valid`: 유효한 설정
   - `TestMQTTPublisherNode_Configure_MissingAgentRef`: agent_ref 누락 에러
   - `TestMQTTPublisherNode_Configure_InvalidQoS`: QoS 범위 초과 에러
   - `TestMQTTPublisherNode_Init_Success`: 정상 초기화
   - `TestMQTTPublisherNode_Process_Success`: 정상 발행 + passthrough
   - `TestMQTTPublisherNode_Process_NoTopic`: 토픽 미지정 에러
   - `TestMQTTPublisherNode_Process_TopicOverride`: _mqtt.topic 오버라이드
   - `TestMQTTPublisherNode_Shutdown`: 정상 종료

**관련 요구사항**: 모든 REQ

**의존성**: M1, M2 완료 후

---

### M6: 예제 플로우 [Priority Low]

**목표**: MQTT 노드 활용 예제 플로우 YAML 작성

**태스크**:

1. `examples/flows/mqtt-subscriber-node.yaml` 작성
   - mqtt-subscriber → filter → console 파이프라인
2. `examples/flows/mqtt-pub-sub-node.yaml` 작성
   - mqtt-subscriber → transform → mqtt-publisher 파이프라인
3. `examples/flows/mqtt-to-modbus-v4.yaml` 작성
   - Path A: mqtt-subscriber → modbus write (메시지 오버라이드)
   - Path B: modbus polling → mqtt-publisher (주기적 읽기 → 발행)

**의존성**: M1 ~ M4 완료 후

---

## 기술 접근 방식

### 아키텍처

```
                         +-------------------+
                         |  MQTT Broker      |
                         +--------+----------+
                                  |
                         +--------v----------+
                         | MQTTSubscriber    |
                         | Agent             |
                         | (Subscribe,       |
                         |  ReceiveMessage,  |
                         |  PublishMessage)  |
                         +---+-----------+---+
                             |           |
                    +--------v---+  +----v----------+
                    | mqtt-      |  | mqtt-         |
                    | subscriber |  | publisher     |
                    | (SourceNode)|  | (Process)     |
                    +--------+---+  +----+----------+
                             |           ^
                    sourceCh v           | Process(msg)
                         +---v-----------+---+
                         |   Flow Engine      |
                         +-------------------+
```

### 핵심 설계 결정

1. **NASA 노드 패턴 채택**: nasaNodeBase 패턴(AgentResolver 기반 에이전트 해석, AgentAccessor를 통한 원본 접근)을 따라 코드베이스 일관성 유지. 단, nasaNodeBase를 직접 임베딩하지 않고 독립적으로 구현 (MQTT 전용 설정이 NASA와 다르기 때문)

2. **MQTTAdapter 재사용**: `adapter.MQTTAdapter`의 `TransformToFlow()`와 `TransformToAgent()`를 재사용하여 메시지 변환 로직 중복 제거. 어댑터의 토픽/QoS/Retained 우선순위 로직을 그대로 활용

3. **ReceiveMessage 기반 수신**: subscriber 노드는 `ReceiveMessage()` 블로킹 호출로 메시지를 수신. 이 방식은 Bridge 노드의 BridgeIn과 동일한 패턴

4. **토픽별 QoS 미지원 (A14 기반)**: 현재 `MQTTSubscriberAgent.Subscribe(topic, qos)`는 에이전트 설정의 단일 QoS를 사용. TopicConfig에 QoS 필드를 예비로 정의하되, 실제 구독 시에는 에이전트 기본 QoS 적용

---

## 리스크 분석

| 리스크 | 영향도 | 발생 확률 | 대응 방안 |
|--------|--------|----------|----------|
| ReceiveMessage에 토픽 메타데이터 부재 (A14) | 중간 | 높음 | TransformToFlow에 빈 AgentMeta 전달. 토픽 정보는 메시지에 포함되지 않음. 추후 에이전트 개선 시 메타데이터 추가 가능 |
| 다중 토픽 수신 시 토픽 구분 불가 | 중간 | 높음 | 현재 MQTTSubscriberAgent의 messageHandler가 payload만 전달. 토픽 구분이 필요하면 에이전트 측 개선 필요 (별도 SPEC) |
| receiveLoop 고루틴 누수 | 높음 | 낮음 | stopCh + context 취소 이중 안전장치. ReceiveMessage의 context 취소 응답성에 의존 |
| Subscribe 실패 시 부분 구독 | 중간 | 낮음 | 하나의 토픽 구독 실패 시 전체 Init 실패. 이미 구독된 토픽 해제 로직 포함 |
| 에이전트 reconnect 중 메시지 손실 | 낮음 | 중간 | MQTT 클라이언트 레벨에서 자동 재연결 처리. 노드 레벨에서는 감지하지 않음 |

---

## 관련 SPEC

| SPEC ID | 관계 | 상태 |
|---------|------|------|
| SPEC-MQTT-001 | 선행 | completed |
| SPEC-MQTT-002 | 선행 | planned |
| SPEC-SAMSUNG-HVACR-001 | 참조 패턴 | completed |
| SPEC-BRIDGE-001 | 참조 | completed |
| SPEC-BRIDGE-002 | 참조 | completed |
