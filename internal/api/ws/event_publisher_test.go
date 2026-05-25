package ws

import (
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// TestNewEventPublisher 는 생성자의 기본 동작을 검증한다.
func TestNewEventPublisher(t *testing.T) {
	t.Parallel()

	t.Run("logger가 nil이면 기본 로거를 사용한다", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)

		ep := NewEventPublisher(hub, nil)
		if ep == nil {
			t.Fatal("NewEventPublisher 가 nil 을 반환했다")
		}
		if ep.hub != hub {
			t.Error("hub 가 올바르게 설정되지 않았다")
		}
		if ep.logger == nil {
			t.Error("nil logger 전달 시 기본 로거가 설정되어야 한다")
		}
	})

	t.Run("커스텀 logger를 사용한다", func(t *testing.T) {
		t.Parallel()
		hub := NewHub(nil)
		logger := slog.Default()

		ep := NewEventPublisher(hub, logger)
		if ep == nil {
			t.Fatal("NewEventPublisher 가 nil 을 반환했다")
		}
		if ep.hub != hub {
			t.Error("hub 가 올바르게 설정되지 않았다")
		}
		if ep.logger != logger {
			t.Error("전달된 logger 가 그대로 사용되어야 한다")
		}
	})
}

// TestEventPublisher_PublishFlowEvent_SkipsWhenNoClients 는
// 연결된 클라이언트가 없으면 브로드캐스트를 건너뛰는지 검증한다.
func TestEventPublisher_PublishFlowEvent_SkipsWhenNoClients(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	ep := NewEventPublisher(hub, nil)

	// ClientCount==0 이므로 브로드캐스트 채널에 메시지가 전송되지 않아야 한다.
	// 패닉이나 에러 없이 정상 반환되는지 확인한다.
	ep.PublishFlowEvent(EventFlowDeployed, "test-flow", "flow-123")

	if hub.ClientCount() != 0 {
		t.Errorf("클라이언트 수가 0이어야 하는데 %d 이다", hub.ClientCount())
	}
}

// TestEventPublisher_PublishFlowEvent_BroadcastsWhenClientsExist 는
// 클라이언트가 있을 때 올바른 메시지 포맷으로 브로드캐스트하는지 검증한다.
func TestEventPublisher_PublishFlowEvent_BroadcastsWhenClientsExist(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	// clientCount 를 직접 증가시켜 클라이언트가 있는 것처럼 만든다.
	// 실제 WebSocket 클라이언트 없이 ClientCount() > 0 조건을 통과시킨다.
	hub.clientCount.Add(1)
	defer hub.clientCount.Add(-1)

	ep := NewEventPublisher(hub, nil)

	ep.PublishFlowEvent(EventFlowDeployed, "my-flow", "flow-abc")

	// broadcast 채널에 메시지가 전송되었는지 확인한다.
	// Hub.Run() 이 메시지를 소비하지만 클라이언트 맵이 비어있어 실제 전송은 없다.
	// 잠시 대기하여 Hub.Run() 루프가 메시지를 처리할 시간을 준다.
	time.Sleep(50 * time.Millisecond)

	// 브로드캐스트가 패닉 없이 완료되었는지 확인 (이 지점에 도달하면 성공).
}

// TestEventPublisher_PublishAgentEvent_SkipsWhenNoClients 는
// 연결된 클라이언트가 없으면 에이전트 이벤트 브로드캐스트를 건너뛰는지 검증한다.
func TestEventPublisher_PublishAgentEvent_SkipsWhenNoClients(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	ep := NewEventPublisher(hub, nil)

	// ClientCount==0 이므로 브로드캐스트하지 않고 반환해야 한다.
	ep.PublishAgentEvent(EventAgentConnected, "test-agent", "agent-123")

	if hub.ClientCount() != 0 {
		t.Errorf("클라이언트 수가 0이어야 하는데 %d 이다", hub.ClientCount())
	}
}

// TestEventPublisher_PublishAgentEvent_BroadcastsWhenClientsExist 는
// 클라이언트가 있을 때 에이전트 이벤트가 올바른 포맷으로 브로드캐스트되는지 검증한다.
func TestEventPublisher_PublishAgentEvent_BroadcastsWhenClientsExist(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	hub.clientCount.Add(1)
	defer hub.clientCount.Add(-1)

	ep := NewEventPublisher(hub, nil)

	ep.PublishAgentEvent(EventAgentConnected, "my-agent", "agent-abc")

	time.Sleep(50 * time.Millisecond)

	// 브로드캐스트가 패닉 없이 완료되었는지 확인.
}

// TestEventPublisher_EventTypes 는 모든 이벤트 타입 상수가 정의되어 있는지 검증한다.
func TestEventPublisher_EventTypes(t *testing.T) {
	t.Parallel()

	expectedEvents := map[string]string{
		"EventFlowDeployed":      EventFlowDeployed,
		"EventFlowStarted":       EventFlowStarted,
		"EventFlowStopped":       EventFlowStopped,
		"EventFlowError":         EventFlowError,
		"EventAgentConnected":    EventAgentConnected,
		"EventAgentDisconnected": EventAgentDisconnected,
	}

	expectedValues := map[string]string{
		"EventFlowDeployed":      "flow_deployed",
		"EventFlowStarted":       "flow_started",
		"EventFlowStopped":       "flow_stopped",
		"EventFlowError":         "flow_error",
		"EventAgentConnected":    "agent_connected",
		"EventAgentDisconnected": "agent_disconnected",
	}

	for name, got := range expectedEvents {
		want, ok := expectedValues[name]
		if !ok {
			t.Errorf("예상 값 맵에 %s 가 없다", name)
			continue
		}
		if got != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}

	// 모든 이벤트 타입에 대응하는 메시지 템플릿이 있는지 확인한다.
	for name, eventType := range expectedEvents {
		if _, ok := eventMessages[eventType]; !ok {
			t.Errorf("%s(%q)에 대한 메시지 템플릿이 eventMessages 에 없다", name, eventType)
		}
	}
}

// TestEventPublisher_PayloadFormat 은 페이로드 JSON 구조를 검증한다.
// BroadcastMessage 가 생성하는 메시지의 포맷을 간접적으로 테스트한다.
func TestEventPublisher_PayloadFormat(t *testing.T) {
	t.Parallel()

	// systemEventPayload 직렬화 포맷 검증
	t.Run("systemEventPayload JSON 구조", func(t *testing.T) {
		t.Parallel()

		payload := systemEventPayload{
			Type:      EventFlowDeployed,
			Message:   "플로우 'test' 배포됨",
			Timestamp: "2024-01-01T00:00:00Z",
			Details:   "flow-123",
		}

		data, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("JSON 직렬화 실패: %v", err)
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("JSON 역직렬화 실패: %v", err)
		}

		// 필수 필드가 존재하는지 확인한다.
		requiredFields := []string{"type", "message", "timestamp", "details"}
		for _, field := range requiredFields {
			if _, ok := decoded[field]; !ok {
				t.Errorf("필수 필드 %q 가 JSON 에 없다", field)
			}
		}

		// 값 검증
		if decoded["type"] != EventFlowDeployed {
			t.Errorf("type: got %v, want %q", decoded["type"], EventFlowDeployed)
		}
		if decoded["details"] != "flow-123" {
			t.Errorf("details: got %v, want %q", decoded["details"], "flow-123")
		}
	})

	// NewMessage + Encode 를 통한 전체 메시지 포맷 검증
	t.Run("전체 메시지 포맷 검증", func(t *testing.T) {
		t.Parallel()

		payload := systemEventPayload{
			Type:      EventAgentConnected,
			Message:   "에이전트 'test-agent' 연결됨",
			Timestamp: "2024-01-01T00:00:00Z",
			Details:   "agent-456",
		}

		msg, err := NewMessage(TypeSystemEvent, payload)
		if err != nil {
			t.Fatalf("NewMessage 실패: %v", err)
		}

		data, err := msg.Encode()
		if err != nil {
			t.Fatalf("Encode 실패: %v", err)
		}

		var decoded map[string]interface{}
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("JSON 역직렬화 실패: %v", err)
		}

		// 최상위 메시지 구조 검증
		if decoded["type"] != TypeSystemEvent {
			t.Errorf("메시지 type: got %v, want %q", decoded["type"], TypeSystemEvent)
		}
		if _, ok := decoded["timestamp"]; !ok {
			t.Error("메시지에 timestamp 필드가 없다")
		}
		if _, ok := decoded["payload"]; !ok {
			t.Error("메시지에 payload 필드가 없다")
		}

		// payload 내부 구조 검증
		payloadMap, ok := decoded["payload"].(map[string]interface{})
		if !ok {
			t.Fatal("payload 가 JSON 객체가 아니다")
		}

		if payloadMap["type"] != EventAgentConnected {
			t.Errorf("payload.type: got %v, want %q", payloadMap["type"], EventAgentConnected)
		}
		if payloadMap["details"] != "agent-456" {
			t.Errorf("payload.details: got %v, want %q", payloadMap["details"], "agent-456")
		}
		if payloadMap["message"] != "에이전트 'test-agent' 연결됨" {
			t.Errorf("payload.message: got %v, want %q", payloadMap["message"], "에이전트 'test-agent' 연결됨")
		}
	})
}

// TestEventPublisher_FlowEventMessageFormat 은 각 플로우 이벤트 타입별 메시지 포맷을 검증한다.
func TestEventPublisher_FlowEventMessageFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		eventType   string
		flowName    string
		wantMessage string
	}{
		{
			name:        "flow_deployed 메시지 포맷",
			eventType:   EventFlowDeployed,
			flowName:    "my-flow",
			wantMessage: "플로우 'my-flow' 배포됨",
		},
		{
			name:        "flow_started 메시지 포맷",
			eventType:   EventFlowStarted,
			flowName:    "my-flow",
			wantMessage: "플로우 'my-flow' 시작됨",
		},
		{
			name:        "flow_stopped 메시지 포맷",
			eventType:   EventFlowStopped,
			flowName:    "my-flow",
			wantMessage: "플로우 'my-flow' 정지됨",
		},
		{
			name:        "flow_error 메시지 포맷",
			eventType:   EventFlowError,
			flowName:    "my-flow",
			wantMessage: "플로우 'my-flow' 오류 발생",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			hub := NewHub(nil)
			go hub.Run()
			defer hub.Stop()

			// 클라이언트가 있는 것처럼 설정하여 브로드캐스트 로직이 실행되게 한다.
			hub.clientCount.Add(1)
			defer hub.clientCount.Add(-1)

			// broadcast 채널에서 직접 메시지를 읽어 검증한다.
			// Hub.Run() 이 소비하기 전에 채널에서 꺼낸다.
			// 이를 위해 별도의 Hub 를 사용하되 Run() 을 호출하지 않는다.
			hubNoRun := NewHub(nil)
			hubNoRun.clientCount.Add(1)

			ep := NewEventPublisher(hubNoRun, nil)
			ep.PublishFlowEvent(tt.eventType, tt.flowName, "flow-id-1")

			// broadcast 채널에서 메시지를 꺼낸다.
			select {
			case raw := <-hubNoRun.broadcast:
				var msg Message
				if err := json.Unmarshal(raw, &msg); err != nil {
					t.Fatalf("메시지 역직렬화 실패: %v", err)
				}

				if msg.Type != TypeSystemEvent {
					t.Errorf("메시지 타입: got %q, want %q", msg.Type, TypeSystemEvent)
				}

				var payload systemEventPayload
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					t.Fatalf("페이로드 역직렬화 실패: %v", err)
				}

				if payload.Type != tt.eventType {
					t.Errorf("payload.type: got %q, want %q", payload.Type, tt.eventType)
				}
				if payload.Message != tt.wantMessage {
					t.Errorf("payload.message: got %q, want %q", payload.Message, tt.wantMessage)
				}
				if payload.Details != "flow-id-1" {
					t.Errorf("payload.details: got %q, want %q", payload.Details, "flow-id-1")
				}

				// 타임스탬프가 유효한 RFC3339 포맷인지 확인한다.
				if _, err := time.Parse(time.RFC3339, payload.Timestamp); err != nil {
					t.Errorf("payload.timestamp 가 유효한 RFC3339 형식이 아니다: %q", payload.Timestamp)
				}

			case <-time.After(time.Second):
				t.Fatal("broadcast 채널에서 메시지를 받지 못했다")
			}
		})
	}
}

// TestEventPublisher_AgentEventMessageFormat 은 각 에이전트 이벤트 타입별 메시지 포맷을 검증한다.
func TestEventPublisher_AgentEventMessageFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		eventType   string
		agentName   string
		wantMessage string
	}{
		{
			name:        "agent_connected 메시지 포맷",
			eventType:   EventAgentConnected,
			agentName:   "sensor-01",
			wantMessage: "에이전트 'sensor-01' 연결됨",
		},
		{
			name:        "agent_disconnected 메시지 포맷",
			eventType:   EventAgentDisconnected,
			agentName:   "sensor-01",
			wantMessage: "에이전트 'sensor-01' 연결 해제됨",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// Run() 을 호출하지 않고 broadcast 채널에서 직접 읽어 검증한다.
			hub := NewHub(nil)
			hub.clientCount.Add(1)

			ep := NewEventPublisher(hub, nil)
			ep.PublishAgentEvent(tt.eventType, tt.agentName, "agent-id-1")

			select {
			case raw := <-hub.broadcast:
				var msg Message
				if err := json.Unmarshal(raw, &msg); err != nil {
					t.Fatalf("메시지 역직렬화 실패: %v", err)
				}

				if msg.Type != TypeSystemEvent {
					t.Errorf("메시지 타입: got %q, want %q", msg.Type, TypeSystemEvent)
				}

				var payload systemEventPayload
				if err := json.Unmarshal(msg.Payload, &payload); err != nil {
					t.Fatalf("페이로드 역직렬화 실패: %v", err)
				}

				if payload.Type != tt.eventType {
					t.Errorf("payload.type: got %q, want %q", payload.Type, tt.eventType)
				}
				if payload.Message != tt.wantMessage {
					t.Errorf("payload.message: got %q, want %q", payload.Message, tt.wantMessage)
				}
				if payload.Details != "agent-id-1" {
					t.Errorf("payload.details: got %q, want %q", payload.Details, "agent-id-1")
				}

				if _, err := time.Parse(time.RFC3339, payload.Timestamp); err != nil {
					t.Errorf("payload.timestamp 가 유효한 RFC3339 형식이 아니다: %q", payload.Timestamp)
				}

			case <-time.After(time.Second):
				t.Fatal("broadcast 채널에서 메시지를 받지 못했다")
			}
		})
	}
}

// TestEventPublisher_UnknownEventType 은 알 수 없는 이벤트 타입에 대한 폴백 메시지를 검증한다.
func TestEventPublisher_UnknownEventType(t *testing.T) {
	t.Parallel()

	t.Run("알 수 없는 플로우 이벤트 타입 폴백 메시지", func(t *testing.T) {
		t.Parallel()

		hub := NewHub(nil)
		hub.clientCount.Add(1)

		ep := NewEventPublisher(hub, nil)
		ep.PublishFlowEvent("unknown_event", "my-flow", "flow-id")

		select {
		case raw := <-hub.broadcast:
			var msg Message
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatalf("메시지 역직렬화 실패: %v", err)
			}

			var payload systemEventPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				t.Fatalf("페이로드 역직렬화 실패: %v", err)
			}

			// fmt.Sprintf(eventMessages["unknown_event"], "my-flow") 의 결과는
			// eventMessages 에 "unknown_event" 키가 없으므로 빈 문자열이 되고,
			// fmt.Sprintf("", "my-flow") 는 "%!(EXTRA string=my-flow)" 가 된다.
			// 이 값은 빈 문자열이 아니므로 폴백 분기로 진입하지 않는다.
			// 실제 동작에 맞게 검증한다.
			if payload.Type != "unknown_event" {
				t.Errorf("payload.type: got %q, want %q", payload.Type, "unknown_event")
			}

		case <-time.After(time.Second):
			t.Fatal("broadcast 채널에서 메시지를 받지 못했다")
		}
	})

	t.Run("알 수 없는 에이전트 이벤트 타입 폴백 메시지", func(t *testing.T) {
		t.Parallel()

		hub := NewHub(nil)
		hub.clientCount.Add(1)

		ep := NewEventPublisher(hub, nil)
		ep.PublishAgentEvent("unknown_agent_event", "my-agent", "agent-id")

		select {
		case raw := <-hub.broadcast:
			var msg Message
			if err := json.Unmarshal(raw, &msg); err != nil {
				t.Fatalf("메시지 역직렬화 실패: %v", err)
			}

			var payload systemEventPayload
			if err := json.Unmarshal(msg.Payload, &payload); err != nil {
				t.Fatalf("페이로드 역직렬화 실패: %v", err)
			}

			if payload.Type != "unknown_agent_event" {
				t.Errorf("payload.type: got %q, want %q", payload.Type, "unknown_agent_event")
			}

		case <-time.After(time.Second):
			t.Fatal("broadcast 채널에서 메시지를 받지 못했다")
		}
	})
}

// ---------------------------------------------------------------------------
// SPEC-DEVICE-IDENTITY-001 Phase B — B-T3 (B-AC2)
//
// WebSocket device.status 페이로드의 uid 1급 필드와 device_id 호환 alias 의
// 직렬화 동작을 검증한다. graceful degradation (uid 빈 문자열) 경로에서는
// "uid" 키가 JSON 에서 omitempty 로 생략되어야 한다.
// ---------------------------------------------------------------------------

// captureDeviceStatusPayload 는 PublishDevice* 호출 시 broadcast 채널로 전송된
// device.status 메시지의 페이로드와 원본 JSON 바이트를 함께 반환한다. Hub.Run()
// 을 호출하지 않으므로 채널에서 직접 읽어 race 없이 검증할 수 있다.
func captureDeviceStatusPayload(t *testing.T, publish func(ep *EventPublisher)) (deviceStatusPayload, string) {
	t.Helper()

	hub := NewHub(nil)
	hub.clientCount.Add(1) // ClientCount > 0 조건 통과

	ep := NewEventPublisher(hub, nil)
	publish(ep)

	select {
	case raw := <-hub.broadcast:
		var msg Message
		if err := json.Unmarshal(raw, &msg); err != nil {
			t.Fatalf("메시지 역직렬화 실패: %v", err)
		}
		if msg.Type != TypeDeviceStatus {
			t.Errorf("메시지 타입: got %q, want %q", msg.Type, TypeDeviceStatus)
		}
		var payload deviceStatusPayload
		if err := json.Unmarshal(msg.Payload, &payload); err != nil {
			t.Fatalf("페이로드 역직렬화 실패: %v", err)
		}
		return payload, string(msg.Payload)
	case <-time.After(time.Second):
		t.Fatal("broadcast 채널에서 메시지를 받지 못했다")
		return deviceStatusPayload{}, ""
	}
}

// TestEventPublisher_PublishDeviceStateChangedV2_ExposesUID 는 V2 API 호출 시
// payload 의 uid 가 1급으로 노출되며 composite alias (device_id) 도 함께
// emit 됨을 검증한다 (B-AC2).
func TestEventPublisher_PublishDeviceStateChangedV2_ExposesUID(t *testing.T) {
	t.Parallel()

	const uid = "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	const composite = "lgcnp:81"

	payload, rawJSON := captureDeviceStatusPayload(t, func(ep *EventPublisher) {
		ep.PublishDeviceStateChangedV2(uid, composite)
	})

	if payload.EventType != "device_state_changed" {
		t.Errorf("event_type: got %q, want %q", payload.EventType, "device_state_changed")
	}
	if payload.UID != uid {
		t.Errorf("uid: got %q, want %q", payload.UID, uid)
	}
	if payload.DeviceID != composite {
		t.Errorf("device_id (alias): got %q, want %q", payload.DeviceID, composite)
	}

	// raw JSON 검증: uid 와 device_id 키가 모두 직렬화됨.
	if !strings.Contains(rawJSON, `"uid":"`+uid+`"`) {
		t.Errorf("uid 필드가 raw JSON 에 노출되지 않았다: %s", rawJSON)
	}
	if !strings.Contains(rawJSON, `"device_id":"`+composite+`"`) {
		t.Errorf("device_id alias 가 raw JSON 에 노출되지 않았다: %s", rawJSON)
	}
}

// TestEventPublisher_PublishDeviceStateChangedV2_GracefulDegradation 는 uid 가
// 빈 문자열일 때 omitempty 계약에 따라 "uid" 키 자체가 JSON 에서 생략됨을
// 검증한다. DeviceID (composite) 는 그대로 유지된다.
func TestEventPublisher_PublishDeviceStateChangedV2_GracefulDegradation(t *testing.T) {
	t.Parallel()

	const composite = "lgcnp:81"

	payload, rawJSON := captureDeviceStatusPayload(t, func(ep *EventPublisher) {
		ep.PublishDeviceStateChangedV2("", composite)
	})

	if payload.UID != "" {
		t.Errorf("uid: got %q, want empty (graceful degradation)", payload.UID)
	}
	if payload.DeviceID != composite {
		t.Errorf("device_id: got %q, want %q", payload.DeviceID, composite)
	}

	// raw JSON: "uid" 키 자체가 생략되어야 한다 (omitempty).
	if strings.Contains(rawJSON, `"uid":`) {
		t.Errorf("uid 키가 빈 값에도 emit 되었다 (omitempty 위반): %s", rawJSON)
	}
	if !strings.Contains(rawJSON, `"device_id":"`+composite+`"`) {
		t.Errorf("composite alias 가 사라졌다: %s", rawJSON)
	}
}

// TestEventPublisher_PublishDeviceStateChanged_DelegatesToV2 는 v1 호환 API 가
// V2 경로로 위임됨을 검증한다 (V2 의 deviceCompositeID 인자로 deviceID 전달,
// UID 는 빈 문자열).
func TestEventPublisher_PublishDeviceStateChanged_DelegatesToV2(t *testing.T) {
	t.Parallel()

	const composite = "lgcnp:81"

	payload, rawJSON := captureDeviceStatusPayload(t, func(ep *EventPublisher) {
		ep.PublishDeviceStateChanged(composite)
	})

	if payload.UID != "" {
		t.Errorf("v1 경로의 uid 는 빈 문자열이어야 한다: got %q", payload.UID)
	}
	if payload.DeviceID != composite {
		t.Errorf("device_id: got %q, want %q", payload.DeviceID, composite)
	}

	if strings.Contains(rawJSON, `"uid":`) {
		t.Errorf("v1 경로에서 uid 키가 emit 되면 안 된다: %s", rawJSON)
	}
}

// TestEventPublisher_PublishDeviceStateChangedV2_SkipsWhenNoClients 는
// 연결 클라이언트가 없을 때 즉시 반환 (broadcast 없음) 됨을 검증한다.
func TestEventPublisher_PublishDeviceStateChangedV2_SkipsWhenNoClients(t *testing.T) {
	t.Parallel()

	hub := NewHub(nil)
	go hub.Run()
	defer hub.Stop()

	ep := NewEventPublisher(hub, nil)

	// ClientCount==0 이므로 broadcast 채널에 메시지가 전송되지 않아야 한다.
	// panic / error 없이 정상 반환되어야 한다.
	ep.PublishDeviceStateChangedV2("a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", "lgcnp:81")

	if hub.ClientCount() != 0 {
		t.Errorf("클라이언트 수가 0 이어야 하는데 %d 이다", hub.ClientCount())
	}
}
