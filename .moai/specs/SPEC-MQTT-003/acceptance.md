---
id: SPEC-MQTT-003
version: "1.1.0"
status: completed
created: "2026-03-14"
updated: "2026-03-15"
author: xtra
priority: high
---

# SPEC-MQTT-003 수락 기준: MQTT Subscriber / Publisher 노드

## 1. mqtt-subscriber 노드

### 시나리오 1.1: 정상 초기화 및 토픽 구독

```gherkin
Given MQTT 에이전트 "mqtt-agent-1"이 실행 중이고
  And mqtt-subscriber 노드의 설정이 다음과 같을 때:
    | 필드           | 값                                              |
    | agent_ref      | mqtt-agent-1                                    |
    | topics         | [{"topic": "sensor/temp", "qos": 0}, {"topic": "sensor/humidity", "qos": 1}] |
    | payload_format | json                                            |
When 노드 Init(ctx)이 호출되면
Then AgentResolver를 통해 "mqtt-agent-1" 에이전트가 해석되고
  And 원본 에이전트가 SubscriberAgent와 MessageReceiver 인터페이스를 구현함이 확인되고
  And Subscribe(ctx, ["sensor/temp", "sensor/humidity"])가 호출되고
  And receiveLoop 고루틴이 시작되고
  And 노드 상태가 Running으로 전이된다
```

### 시나리오 1.2: 메시지 수신 및 SourceCh 전달

```gherkin
Given mqtt-subscriber 노드가 Running 상태이고
  And "sensor/temp" 토픽을 구독 중일 때
When MQTT 에이전트가 {"temperature": 25.5, "unit": "celsius"} 데이터를 수신하면
Then receiveLoop가 ReceiveMessage()를 통해 데이터를 수신하고
  And MQTTAdapter.TransformToFlow()를 통해 플로우 메시지로 변환되고
  And 변환된 메시지의 payload에 "temperature"=25.5, "unit"="celsius"가 포함되고
  And 메시지 메타데이터에 "mqtt.node_id"가 노드 ID로 설정되고
  And SourceCh()에 메시지가 전달된다
```

### 시나리오 1.3: Shutdown 시 토픽 구독 해제

```gherkin
Given mqtt-subscriber 노드가 Running 상태이고
  And ["sensor/temp", "sensor/humidity"] 토픽을 구독 중일 때
When Shutdown(ctx)이 호출되면
Then stopCh가 닫히고 receiveLoop 고루틴이 종료되고
  And Unsubscribe(ctx, ["sensor/temp", "sensor/humidity"])가 호출되고
  And 노드 상태가 Stopping으로 전이된다
```

### 시나리오 1.4: agent_ref 누락 시 Configure 에러

```gherkin
Given mqtt-subscriber 노드의 설정에 agent_ref가 없을 때
When Configure(config)가 호출되면
Then ErrMQTTMissingAgentRef 에러가 반환된다
```

### 시나리오 1.5: topics 누락 시 Configure 에러

```gherkin
Given mqtt-subscriber 노드의 설정이 다음과 같을 때:
    | 필드      | 값           |
    | agent_ref | mqtt-agent-1 |
    | topics    | []           |
When Configure(config)가 호출되면
Then ErrMQTTMissingTopics 에러가 반환된다
```

### 시나리오 1.6: AgentResolver 미설정 시 Init 에러

```gherkin
Given mqtt-subscriber 노드에 AgentResolver가 설정되지 않았을 때
When Init(ctx)이 호출되면
Then ErrMQTTNoResolver 에러가 반환된다
```

### 시나리오 1.7: 에이전트가 SubscriberAgent 미구현 시 Init 에러

```gherkin
Given AgentResolver가 SubscriberAgent를 구현하지 않는 에이전트를 반환할 때
When Init(ctx)이 호출되면
Then ErrMQTTAgentNotMQTT 에러가 반환되고
  And 에러 메시지에 SubscriberAgent 인터페이스 관련 설명이 포함된다
```

### 시나리오 1.8: sourceCh 버퍼 오버플로우 시 메시지 드롭

```gherkin
Given mqtt-subscriber 노드의 buffer_size가 2이고
  And sourceCh에 이미 2개의 메시지가 대기 중일 때
When 새로운 메시지가 수신되면
Then 해당 메시지는 드롭되고
  And 경고 로그가 출력된다
```

### 시나리오 1.9: Subscribe 부분 실패 시 롤백

```gherkin
Given mqtt-subscriber 노드의 topics가 ["topic-a", "topic-b", "topic-c"]이고
  And "topic-b" 구독이 실패할 때
When Init(ctx)이 호출되면
Then Init은 에러를 반환하고
  And 노드는 Running 상태로 전이되지 않는다
```

---

## 2. mqtt-publisher 노드

### 시나리오 2.1: 정상 초기화

```gherkin
Given MQTT 에이전트 "mqtt-agent-1"이 실행 중이고
  And mqtt-publisher 노드의 설정이 다음과 같을 때:
    | 필드             | 값               |
    | agent_ref        | mqtt-agent-1     |
    | default_topic    | device/command   |
    | default_qos      | 1                |
    | default_retained | false            |
When 노드 Init(ctx)이 호출되면
Then AgentResolver를 통해 "mqtt-agent-1" 에이전트가 해석되고
  And 원본 에이전트가 MessagePublisher 인터페이스를 구현함이 확인되고
  And 노드 상태가 Running으로 전이된다
```

### 시나리오 2.2: 기본 토픽으로 메시지 발행

```gherkin
Given mqtt-publisher 노드가 Running 상태이고
  And default_topic이 "device/command", default_qos가 1일 때
When Process(ctx, msg)가 호출되고
  And msg.payload가 {"action": "turn_on", "target": "light-1"}일 때
Then MQTTAdapter.TransformToAgent(msg)로 페이로드가 변환되고
  And PublishMessage("device/command", 1, false, payload)가 호출되고
  And 출력 메시지의 메타데이터에 "mqtt.published_topic"="device/command"가 설정되고
  And 원본 메시지가 passthrough로 출력된다
```

### 시나리오 2.3: _mqtt 객체를 통한 토픽 오버라이드

```gherkin
Given mqtt-publisher 노드가 Running 상태이고
  And default_topic이 "device/command"일 때
When Process(ctx, msg)가 호출되고
  And msg.payload에 _mqtt 객체가 포함될 때:
    | 필드     | 값                  |
    | topic    | custom/override    |
    | qos      | 2                  |
    | retained | true               |
Then PublishMessage("custom/override", 2, true, payload)가 호출되고
  And 발행 페이로드에서 _mqtt 객체는 제거되고
  And 출력 메시지의 "mqtt.published_topic"="custom/override"가 설정된다
```

### 시나리오 2.4: 메타데이터를 통한 토픽 오버라이드

```gherkin
Given mqtt-publisher 노드가 Running 상태이고
  And default_topic이 "device/command"일 때
When Process(ctx, msg)가 호출되고
  And msg.metadata에 "mqtt.topic"="meta/override"가 설정되어 있을 때
Then PublishMessage("meta/override", ...)가 호출되고
  And 메타데이터의 mqtt.topic이 default_topic보다 우선 적용된다
```

### 시나리오 2.5: 토픽 미지정 시 에러

```gherkin
Given mqtt-publisher 노드가 Running 상태이고
  And default_topic이 빈 문자열이고
  And 메시지에 토픽 오버라이드가 없을 때
When Process(ctx, msg)가 호출되면
Then ErrMQTTNoTopic 에러가 반환되고
  And PublishMessage는 호출되지 않는다
```

### 시나리오 2.6: agent_ref 누락 시 Configure 에러

```gherkin
Given mqtt-publisher 노드의 설정에 agent_ref가 없을 때
When Configure(config)가 호출되면
Then ErrMQTTMissingAgentRef 에러가 반환된다
```

### 시나리오 2.7: QoS 범위 초과 시 Configure 에러

```gherkin
Given mqtt-publisher 노드의 설정에 default_qos가 3일 때
When Configure(config)가 호출되면
Then 에러가 반환되고
  And 에러 메시지에 QoS 범위(0-2)가 명시된다
```

### 시나리오 2.8: 에이전트가 MessagePublisher 미구현 시 Init 에러

```gherkin
Given AgentResolver가 MessagePublisher를 구현하지 않는 에이전트를 반환할 때
When Init(ctx)이 호출되면
Then ErrMQTTAgentNotMQTT 에러가 반환된다
```

### 시나리오 2.9: PublishMessage 실패 시 에러 전파

```gherkin
Given mqtt-publisher 노드가 Running 상태이고
  And 에이전트의 PublishMessage가 에러를 반환할 때
When Process(ctx, msg)가 호출되면
Then Process는 에러를 반환하고
  And 출력 메시지는 nil이다
```

---

## 3. 노드 레지스트리

### 시나리오 3.1: 빌트인 등록 확인

```gherkin
Given 기본 노드 레지스트리가 초기화될 때
When registerBuiltins()가 호출되면
Then "mqtt-subscriber" 타입이 카테고리 "io"로 등록되고
  And "mqtt-publisher" 타입이 카테고리 "io"로 등록되고
  And 각각의 팩토리 함수가 정상적으로 노드를 생성할 수 있다
```

---

## 4. 프론트엔드 스키마

### 시나리오 4.1: mqtt-subscriber 스키마 존재 확인

```gherkin
Given nodeSchemas.ts가 로드될 때
When "mqtt-subscriber" 타입의 스키마를 조회하면
Then agent_ref (agent_select, required, options: ['mqtt']) 필드가 존재하고
  And topics (string_list, required) 필드가 존재하고
  And payload_format (select) 필드가 존재하고
  And buffer_size (number) 필드가 존재한다
```

### 시나리오 4.2: mqtt-publisher 스키마 존재 확인

```gherkin
Given nodeSchemas.ts가 로드될 때
When "mqtt-publisher" 타입의 스키마를 조회하면
Then agent_ref (agent_select, required, options: ['mqtt']) 필드가 존재하고
  And default_topic (string) 필드가 존재하고
  And default_qos (select: 0/1/2) 필드가 존재하고
  And default_retained (boolean) 필드가 존재하고
  And payload_format (select) 필드가 존재한다
```

### 시나리오 4.3: nodeTypeMeta 메타 확인

```gherkin
Given nodeTypeMeta.ts가 로드될 때
When "mqtt-subscriber" 타입의 메타를 조회하면
Then description에 MQTT 구독 관련 설명이 포함되고
  And ports에 out(output) 포트가 정의되어 있고
  And configExample에 agent_ref와 topics 예제가 포함된다

When "mqtt-publisher" 타입의 메타를 조회하면
Then description에 MQTT 발행 관련 설명이 포함되고
  And ports에 in(input)과 out(output) 포트가 정의되어 있고
  And configExample에 agent_ref와 default_topic 예제가 포함된다
```

### 시나리오 4.4: StringListEditor 컴포넌트 동작

```gherkin
Given FormField 컴포넌트가 string_list 타입 필드를 렌더링할 때
When 사용자가 "추가" 버튼을 클릭하면
Then 빈 입력 필드가 추가되고
  And 사용자가 값을 입력하면 onChange에 빈 문자열이 제외된 string[] 배열이 전달된다

When 사용자가 삭제 버튼을 클릭하면
Then 해당 행이 제거되고
  And onChange에 업데이트된 배열이 전달된다
```

### 시나리오 4.5: agent_select 에이전트 타입 필터링

```gherkin
Given FormField 컴포넌트가 agent_select 타입 필드를 렌더링하고
  And field.options가 ['mqtt']일 때
When 에이전트 드롭다운이 열리면
Then type이 'mqtt'인 에이전트만 목록에 표시되고
  And 다른 타입의 에이전트는 표시되지 않는다
```

---

## 5. 엣지 케이스

### 시나리오 5.1: 빈 페이로드 수신

```gherkin
Given mqtt-subscriber 노드가 Running 상태일 때
When MQTT 에이전트가 빈 바이트(nil 또는 []) 데이터를 수신하면
Then TransformToFlow가 빈 메시지를 생성하고
  And 메시지가 정상적으로 sourceCh에 전달된다
```

### 시나리오 5.2: 비 JSON 페이로드 수신

```gherkin
Given mqtt-subscriber 노드가 Running 상태이고 payload_format이 "json"일 때
When MQTT 에이전트가 "hello world" (비 JSON) 데이터를 수신하면
Then TransformToFlow가 "_raw" 키에 원본 바이트를 저장하는 메시지를 생성하고
  And 메시지가 정상적으로 sourceCh에 전달된다
```

### 시나리오 5.3: 대용량 메시지 연속 수신

```gherkin
Given mqtt-subscriber 노드가 Running 상태이고 buffer_size가 64일 때
When 1초 동안 100개의 메시지가 연속 수신되면
Then 최대 64개의 메시지가 sourceCh에 대기하고
  And 초과 메시지는 드롭되며 경고 로그가 출력된다
```

### 시나리오 5.4: Shutdown 후 재 Shutdown 호출

```gherkin
Given mqtt-subscriber 노드가 Shutdown 완료 상태일 때
When Shutdown(ctx)이 다시 호출되면
Then sync.Once에 의해 stopCh close가 중복 실행되지 않고
  And 패닉이 발생하지 않는다
```

### 시나리오 5.5: Publisher 발행 토픽 우선순위

```gherkin
Given mqtt-publisher 노드가 Running 상태이고
  And default_topic이 "default/topic"일 때
When Process(ctx, msg)가 호출되고
  And msg.metadata에 "mqtt.topic"="meta-topic"이 있고
  And msg.payload에 _mqtt.topic="payload-topic"이 있을 때
Then 토픽 우선순위에 따라 "meta-topic"이 사용된다
  (우선순위: metadata > _mqtt payload > adapter publishTopic > default_topic)
```

---

## 6. Quality Gate 기준

### 코드 품질

- 테스트 커버리지: internal/node/mqtt.go 기준 85% 이상
- 컴파일 에러: 0건
- go vet: 경고 0건
- race condition: `go test -race` 통과

### 기능 완성도

- mqtt-subscriber 노드: Init/Configure/Process/Shutdown/SourceCh 모두 구현
- mqtt-publisher 노드: Init/Configure/Process/Shutdown 모두 구현
- 노드 레지스트리에 2종 등록
- 프론트엔드 스키마 2종 추가

### Definition of Done

- [x] M1~M5 모든 마일스톤 태스크 완료
- [x] 모든 수락 기준 시나리오 통과
- [x] `go test -race ./internal/node/...` 통과 (22개 테스트)
- [x] `go vet ./internal/node/...` 경고 0건
- [x] 프론트엔드 빌드 에러 0건
- [x] SPEC 상태를 `completed`로 업데이트
- [x] 프론트엔드 StringListEditor + agent_select 필터링 구현 (v1.1.0)
- [x] 예제 플로우 3종 작성 (mqtt-subscriber-node, mqtt-pub-sub-node, mqtt-to-modbus-v4)
