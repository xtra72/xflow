---
id: SPEC-MQTT-001
type: acceptance
version: "1.0.0"
spec_ref: SPEC-MQTT-001
---

# SPEC-MQTT-001 수락 기준

## REQ-1: SubscriberAgent 인터페이스

### AC-MQTT-001-01: SubscriberAgent 인터페이스 정의

```gherkin
Given internal/agent/agent.go 파일이 주어졌을 때
Then SubscriberAgent 인터페이스가 정의되어 있어야 한다
And Subscribe(ctx context.Context, topics []string) error 메서드가 포함되어야 한다
And Unsubscribe(ctx context.Context, topics []string) error 메서드가 포함되어야 한다
```

### AC-MQTT-001-02: SubscriberAgent는 Agent와 독립적

```gherkin
Given SubscriberAgent 인터페이스를 구현하지 않는 Mock Agent가 주어졌을 때
When 해당 Agent를 BridgeNode에 바인딩하면
Then 기존 Bridge 기능(BridgeOut/In/InOut/RequestReply)이 정상 동작해야 한다
And 토픽 구독 관련 에러가 발생하지 않아야 한다
```

---

## REQ-2: MQTT Agent implements SubscriberAgent

### AC-MQTT-001-03: MQTTSubscriberAgent 컴파일 타임 체크

```gherkin
Given internal/agent/system/mqtt_subscriber.go 파일이 주어졌을 때
Then var _ agent.SubscriberAgent = (*MQTTSubscriberAgent)(nil) 컴파일 타임 체크가 포함되어야 한다
And 컴파일이 성공해야 한다
```

### AC-MQTT-001-04: Subscribe 성공

```gherkin
Given MQTTSubscriberAgent가 MQTT 브로커에 연결된 상태일 때
When Subscribe(ctx, []string{"sensor/temperature", "sensor/humidity"})를 호출하면
Then nil 에러를 반환해야 한다
And MQTT 클라이언트의 Subscribe가 각 토픽에 대해 호출되어야 한다
And subscribedTopics에 "sensor/temperature"와 "sensor/humidity"가 포함되어야 한다
```

### AC-MQTT-001-05: Subscribe 연결 끊김 시 에러

```gherkin
Given MQTTSubscriberAgent가 MQTT 브로커에 연결되지 않은 상태일 때
When Subscribe(ctx, []string{"sensor/temperature"})를 호출하면
Then 에러를 반환해야 한다
And subscribedTopics가 변경되지 않아야 한다
```

### AC-MQTT-001-06: Unsubscribe 성공

```gherkin
Given MQTTSubscriberAgent가 MQTT 브로커에 연결된 상태이고
And "sensor/temperature"와 "sensor/humidity" 토픽이 구독 중일 때
When Unsubscribe(ctx, []string{"sensor/humidity"})를 호출하면
Then nil 에러를 반환해야 한다
And MQTT 클라이언트의 Unsubscribe가 "sensor/humidity"에 대해 호출되어야 한다
And subscribedTopics에 "sensor/temperature"만 남아야 한다
And subscribedTopics에 "sensor/humidity"가 없어야 한다
```

### AC-MQTT-001-07: Unsubscribe 연결 끊김 시 에러

```gherkin
Given MQTTSubscriberAgent가 MQTT 브로커에 연결되지 않은 상태일 때
When Unsubscribe(ctx, []string{"sensor/temperature"})를 호출하면
Then 에러를 반환해야 한다
And subscribedTopics가 변경되지 않아야 한다
```

### AC-MQTT-001-08: Subscribe/Unsubscribe 동시성 안전

```gherkin
Given MQTTSubscriberAgent가 MQTT 브로커에 연결된 상태일 때
When 10개 goroutine에서 동시에 Subscribe와 Unsubscribe를 호출하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And subscribedTopics 목록이 일관성을 유지해야 한다
```

### AC-MQTT-001-09: 재연결 시 구독 토픽 복구

```gherkin
Given MQTTSubscriberAgent가 MQTT 브로커에 연결되어 있고
And 초기 토픽 ["config/topic"]과 동적 추가 토픽 ["dynamic/topic"]이 구독 중일 때
When MQTT 브로커 연결이 끊겼다가 재연결되면
Then onConnect 핸들러에서 ["config/topic", "dynamic/topic"] 모두 재구독되어야 한다
```

---

## REQ-3: Bridge Node Config Topics

### AC-MQTT-001-10: BridgeConfig Topics 필드

```gherkin
Given BridgeConfig 구조체가 주어졌을 때
Then Topics []string 필드가 존재해야 한다

Given DefaultBridgeConfig(agentRef)를 호출하면
Then Topics가 nil이어야 한다
```

### AC-MQTT-001-11: Bridge Init 토픽 자동 구독 - SubscriberAgent 구현

```gherkin
Given SubscriberAgent를 구현하는 Mock Agent가 AgentResolver에 등록되어 있을 때
And BridgeConfig.Topics가 ["bridge/topic1", "bridge/topic2"]로 설정되어 있을 때
When BridgeNode.Init(ctx)를 호출하면
Then Mock Agent의 Subscribe(ctx, ["bridge/topic1", "bridge/topic2"])가 호출되어야 한다
And BridgeNode의 bridgeTopics에 ["bridge/topic1", "bridge/topic2"]가 포함되어야 한다
And Init이 nil 에러를 반환해야 한다
```

### AC-MQTT-001-12: Bridge Init 토픽 자동 구독 - SubscriberAgent 미구현

```gherkin
Given SubscriberAgent를 구현하지 않는 Mock Agent가 AgentResolver에 등록되어 있을 때
And BridgeConfig.Topics가 ["bridge/topic1"]로 설정되어 있을 때
When BridgeNode.Init(ctx)를 호출하면
Then Subscribe가 호출되지 않아야 한다
And Init이 nil 에러를 반환해야 한다 (무시하고 진행)
And 경고 로그가 출력되어야 한다
```

### AC-MQTT-001-13: Bridge Init - Topics가 비어 있을 때

```gherkin
Given SubscriberAgent를 구현하는 Mock Agent가 AgentResolver에 등록되어 있을 때
And BridgeConfig.Topics가 nil 또는 빈 슬라이스일 때
When BridgeNode.Init(ctx)를 호출하면
Then Subscribe가 호출되지 않아야 한다
And Init이 정상 완료되어야 한다
```

### AC-MQTT-001-14: Bridge Init - Subscribe 실패 시

```gherkin
Given SubscriberAgent를 구현하는 Mock Agent가 Subscribe에서 에러를 반환하도록 설정되어 있을 때
And BridgeConfig.Topics가 ["fail/topic"]로 설정되어 있을 때
When BridgeNode.Init(ctx)를 호출하면
Then Subscribe 에러가 반환되어야 한다
```

---

## REQ-4: Runtime Control Messages via Bridge Process

### AC-MQTT-001-15: Subscribe 제어 메시지 처리

```gherkin
Given BridgeOut 방향의 BridgeNode가 SubscriberAgent를 구현하는 Agent에 바인딩되어 있을 때
And 메시지 Payload에 {"action": "subscribe", "topics": ["runtime/topic1", "runtime/topic2"]}가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then Agent의 Subscribe(ctx, ["runtime/topic1", "runtime/topic2"])가 호출되어야 한다
And bridgeTopics에 ["runtime/topic1", "runtime/topic2"]가 추가되어야 한다
And nil 에러를 반환해야 한다
```

### AC-MQTT-001-16: Unsubscribe 제어 메시지 처리

```gherkin
Given BridgeOut 방향의 BridgeNode가 SubscriberAgent를 구현하는 Agent에 바인딩되어 있을 때
And bridgeTopics에 "runtime/topic1"이 포함되어 있을 때
And 메시지 Payload에 {"action": "unsubscribe", "topics": ["runtime/topic1"]}가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then Agent의 Unsubscribe(ctx, ["runtime/topic1"])가 호출되어야 한다
And bridgeTopics에서 "runtime/topic1"이 제거되어야 한다
And nil 에러를 반환해야 한다
```

### AC-MQTT-001-17: SubscriberAgent 미구현 에이전트에 제어 메시지

```gherkin
Given BridgeOut 방향의 BridgeNode가 SubscriberAgent를 구현하지 않는 Agent에 바인딩되어 있을 때
And 메시지 Payload에 {"action": "subscribe", "topics": ["topic1"]}가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then 에러를 반환해야 한다
And 토픽 구독이 시도되지 않아야 한다
```

### AC-MQTT-001-18: 유효하지 않은 action 값

```gherkin
Given BridgeOut 방향의 BridgeNode가 SubscriberAgent를 구현하는 Agent에 바인딩되어 있을 때
And 메시지 Payload에 {"action": "invalid_action", "topics": ["topic1"]}가 설정되어 있을 때
When Process(ctx, msg)를 호출하면
Then 에러를 반환해야 한다
```

### AC-MQTT-001-19: topics 필드 누락

```gherkin
Given BridgeOut 방향의 BridgeNode가 SubscriberAgent를 구현하는 Agent에 바인딩되어 있을 때
And 메시지 Payload에 {"action": "subscribe"}만 설정되어 있을 때 (topics 필드 없음)
When Process(ctx, msg)를 호출하면
Then 에러를 반환해야 한다
```

### AC-MQTT-001-20: 일반 메시지 통과

```gherkin
Given BridgeOut 방향의 BridgeNode가 Agent에 바인딩되어 있을 때
And 메시지 Payload에 {"temperature": 25.5, "unit": "celsius"}가 설정되어 있을 때 (action 필드 없음)
When Process(ctx, msg)를 호출하면
Then 기존 BridgeOut 로직이 실행되어야 한다 (변환 후 transport.Send 호출)
And Subscribe/Unsubscribe가 호출되지 않아야 한다
```

### AC-MQTT-001-21: action 필드는 있지만 topics가 없는 일반 메시지 구분

```gherkin
Given BridgeOut 방향의 BridgeNode가 Agent에 바인딩되어 있을 때
And 메시지 Payload에 {"action": "do_something", "data": 123}가 설정되어 있을 때
And action 값이 "subscribe"도 "unsubscribe"도 아닐 때
When Process(ctx, msg)를 호출하면
Then 에러를 반환해야 한다
```

---

## REQ-5: Cleanup on Bridge Shutdown

### AC-MQTT-001-22: Shutdown 시 bridge-added 토픽 정리

```gherkin
Given BridgeNode가 Init에서 ["config/topic1"]을 구독하고
And Process에서 런타임으로 ["runtime/topic1"]을 추가 구독한 상태일 때
And bridgeTopics가 ["config/topic1", "runtime/topic1"]일 때
When Shutdown(ctx)를 호출하면
Then Agent의 Unsubscribe(ctx, ["config/topic1", "runtime/topic1"])가 호출되어야 한다
And Shutdown이 nil 에러를 반환해야 한다
```

### AC-MQTT-001-23: Shutdown 시 에이전트 자체 토픽 보호

```gherkin
Given MQTTSubscriberAgent가 자체 설정으로 ["agent/topic1", "agent/topic2"]를 구독 중이고
And BridgeNode가 ["bridge/topic1"]을 추가 구독한 상태일 때
When BridgeNode.Shutdown(ctx)를 호출하면
Then Unsubscribe는 ["bridge/topic1"]에 대해서만 호출되어야 한다
And "agent/topic1"과 "agent/topic2"는 여전히 구독 중이어야 한다
```

### AC-MQTT-001-24: Shutdown - bridgeTopics가 비어 있을 때

```gherkin
Given BridgeNode에 bridgeTopics가 비어 있을 때
When Shutdown(ctx)를 호출하면
Then Unsubscribe가 호출되지 않아야 한다
And Shutdown이 nil 에러를 반환해야 한다 (기존 로직만 실행)
```

### AC-MQTT-001-25: Shutdown - Unsubscribe 실패 시 계속 진행

```gherkin
Given BridgeNode에 bridgeTopics가 ["fail/topic"]이고
And Agent의 Unsubscribe가 에러를 반환하도록 설정되어 있을 때
When Shutdown(ctx)를 호출하면
Then 에러가 로그에 기록되어야 한다
And Shutdown 프로세스가 중단되지 않고 완료되어야 한다
And BridgeNode 상태가 정상적으로 전이되어야 한다
```

### AC-MQTT-001-26: Shutdown - SubscriberAgent 미구현 에이전트

```gherkin
Given BridgeNode가 SubscriberAgent를 구현하지 않는 Agent에 바인딩되어 있고
And bridgeTopics가 비어 있을 때
When Shutdown(ctx)를 호출하면
Then Unsubscribe 시도 없이 기존 Shutdown 로직이 실행되어야 한다
And Shutdown이 정상 완료되어야 한다
```

---

## 통합 테스트

### AC-MQTT-001-27: 전체 흐름 통합 테스트

```gherkin
Given SubscriberAgent를 구현하는 Mock Agent가 등록되어 있고
And BridgeConfig.Topics가 ["init/topic"]이고
And BridgeOut 모드의 BridgeNode가 생성되었을 때

When Init(ctx)를 호출하면
Then ["init/topic"]이 구독되어야 한다
And bridgeTopics가 ["init/topic"]이어야 한다

When Process(ctx, subscribeMsg)를 호출하면 (action: "subscribe", topics: ["runtime/topic"])
Then ["runtime/topic"]이 추가 구독되어야 한다
And bridgeTopics가 ["init/topic", "runtime/topic"]이어야 한다

When Process(ctx, unsubscribeMsg)를 호출하면 (action: "unsubscribe", topics: ["init/topic"])
Then ["init/topic"]이 구독 해제되어야 한다
And bridgeTopics가 ["runtime/topic"]이어야 한다

When Shutdown(ctx)를 호출하면
Then ["runtime/topic"]이 구독 해제되어야 한다
And bridgeTopics가 비어 있어야 한다
```

### AC-MQTT-001-28: 동시성 안전 통합 테스트

```gherkin
Given BridgeNode가 SubscriberAgent를 구현하는 Agent에 바인딩되어 Init()이 완료된 상태일 때
When 5개 goroutine에서 동시에 subscribe/unsubscribe 제어 메시지를 Process()로 전달하면
Then race condition이 발생하지 않아야 한다 (go test -race 통과)
And bridgeTopics 목록이 일관성을 유지해야 한다
And 모든 Process 호출이 에러 없이 완료되어야 한다
```

---

## 품질 게이트

### Definition of Done

- [ ] 모든 수락 기준(AC-MQTT-001-01 ~ 28) 테스트 통과
- [ ] `go test ./internal/agent/...` 전체 통과
- [ ] `go test ./internal/node/...` 전체 통과
- [ ] `go test ./internal/engine/...` 전체 통과
- [ ] `go test -race ./internal/agent/... ./internal/node/... ./internal/engine/...` 경쟁 상태 없음
- [ ] `go vet ./internal/agent/... ./internal/node/... ./internal/engine/...` 경고 없음
- [ ] 테스트 커버리지 85% 이상 (`go test -cover`)
- [ ] GoDoc 주석 작성 완료 (SubscriberAgent, Subscribe, Unsubscribe, BridgeConfig.Topics)
- [ ] 기존 테스트 회귀 없음 (기존 BridgeNode 테스트 전체 통과)
- [ ] 기존 MQTT 에이전트 테스트 회귀 없음

### 검증 도구

| 도구 | 용도 |
|------|------|
| `go test` | 단위/통합 테스트 실행 |
| `go test -race` | 경쟁 상태 검출 |
| `go test -cover` | 커버리지 측정 |
| `go vet` | 정적 분석 |
| `golangci-lint` | 코드 품질 린팅 |
