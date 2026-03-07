package ws

import (
	"fmt"
	"log/slog"
	"time"
)

// 플로우 라이프사이클 이벤트 타입 상수.
const (
	EventFlowDeployed      = "flow_deployed"
	EventFlowStarted       = "flow_started"
	EventFlowStopped       = "flow_stopped"
	EventFlowError         = "flow_error"
	EventAgentConnected    = "agent_connected"
	EventAgentDisconnected = "agent_disconnected"
)

// eventMessages 는 이벤트 타입별 한국어 메시지 템플릿이다.
var eventMessages = map[string]string{
	EventFlowDeployed:      "플로우 '%s' 배포됨",
	EventFlowStarted:       "플로우 '%s' 시작됨",
	EventFlowStopped:       "플로우 '%s' 정지됨",
	EventFlowError:         "플로우 '%s' 오류 발생",
	EventAgentConnected:    "에이전트 '%s' 연결됨",
	EventAgentDisconnected: "에이전트 '%s' 연결 해제됨",
}

// systemEventPayload 는 system.event 메시지의 페이로드 구조이다.
type systemEventPayload struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
	Details   string `json:"details"`
}

// EventPublisher 는 시스템 이벤트를 WebSocket 으로 브로드캐스트한다.
type EventPublisher struct {
	hub    *Hub
	logger *slog.Logger
}

// NewEventPublisher 는 새 EventPublisher 를 생성한다.
func NewEventPublisher(hub *Hub, logger *slog.Logger) *EventPublisher {
	if logger == nil {
		logger = slog.Default()
	}
	return &EventPublisher{
		hub:    hub,
		logger: logger,
	}
}

// PublishFlowEvent 는 플로우 라이프사이클 이벤트를 브로드캐스트한다.
// eventType: EventFlowDeployed, EventFlowStarted, EventFlowStopped, EventFlowError
// 연결된 클라이언트가 없으면 즉시 반환한다.
// 브로드캐스트 실패 시 로그만 남기고 에러를 반환하지 않는다.
func (ep *EventPublisher) PublishFlowEvent(eventType, flowName, flowID string) {
	if ep.hub.ClientCount() == 0 {
		return
	}

	msg := fmt.Sprintf(eventMessages[eventType], flowName)
	if msg == "" {
		msg = fmt.Sprintf("플로우 '%s' 이벤트: %s", flowName, eventType)
	}

	payload := systemEventPayload{
		Type:      eventType,
		Message:   msg,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Details:   flowID,
	}

	if err := ep.hub.BroadcastMessage(TypeSystemEvent, payload); err != nil {
		ep.logger.Error("플로우 이벤트 브로드캐스트 실패",
			"eventType", eventType,
			"flowID", flowID,
			"error", err,
		)
	}
}

// PublishAgentEvent 는 에이전트 연결 이벤트를 브로드캐스트한다.
// eventType: EventAgentConnected, EventAgentDisconnected
// 연결된 클라이언트가 없으면 즉시 반환한다.
// 브로드캐스트 실패 시 로그만 남기고 에러를 반환하지 않는다.
func (ep *EventPublisher) PublishAgentEvent(eventType, agentName, agentID string) {
	if ep.hub.ClientCount() == 0 {
		return
	}

	msg := fmt.Sprintf(eventMessages[eventType], agentName)
	if msg == "" {
		msg = fmt.Sprintf("에이전트 '%s' 이벤트: %s", agentName, eventType)
	}

	payload := systemEventPayload{
		Type:      eventType,
		Message:   msg,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Details:   agentID,
	}

	if err := ep.hub.BroadcastMessage(TypeSystemEvent, payload); err != nil {
		ep.logger.Error("에이전트 이벤트 브로드캐스트 실패",
			"eventType", eventType,
			"agentID", agentID,
			"error", err,
		)
	}
}
