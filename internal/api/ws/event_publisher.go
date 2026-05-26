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
	EventFlowUndeployed    = "flow_undeployed"
	EventFlowError         = "flow_error"
	EventAgentConnected    = "agent_connected"
	EventAgentDisconnected = "agent_disconnected"
	EventDeviceOnline      = "device_online"
	EventDeviceOffline     = "device_offline"
)

// eventMessages 는 이벤트 타입별 한국어 메시지 템플릿이다.
var eventMessages = map[string]string{
	EventFlowDeployed:      "플로우 '%s' 배포됨",
	EventFlowStarted:       "플로우 '%s' 시작됨",
	EventFlowStopped:       "플로우 '%s' 정지됨",
	EventFlowUndeployed:    "플로우 '%s' 배포 해제됨",
	EventFlowError:         "플로우 '%s' 오류 발생",
	EventAgentConnected:    "에이전트 '%s' 연결됨",
	EventAgentDisconnected: "에이전트 '%s' 연결 해제됨",
	EventDeviceOnline:      "에이전트 '%s' 디바이스 온라인",
	EventDeviceOffline:     "에이전트 '%s' 디바이스 오프라인",
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

// deviceStatusPayload 는 device.status 메시지의 페이로드 구조이다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (M3 / D-T3, xflowd v1.0 — Breaking):
//
//   - UID 는 글로벌 UUID v4 (Device.UID()) 의 유일한 식별 필드이다.
//   - 기존 `device_id` (composite alias) 필드는 Phase D 부터 완전 제거.
//     외부 클라이언트는 UID 1급 필드만 사용해야 한다 (Frontend 는 PR2 에서
//     migration 완료, M11 충족).
//   - graceful degradation: UID 가 빈 문자열인 경우 omitempty 로 키 자체를
//     생략한다 (downstream 이 키 존재 여부로 graceful degradation 판단 가능).
type deviceStatusPayload struct {
	EventType string `json:"event_type"`
	AgentName string `json:"agent_name"`
	UID       string `json:"uid,omitempty"` // SPEC-DEVICE-IDENTITY-001 — 1급 식별자 (UUID v4)
	Timestamp string `json:"timestamp"`
}

// PublishDeviceEvent 는 디바이스 상태 변경 이벤트를 브로드캐스트한다.
// eventType: EventDeviceOnline, EventDeviceOffline
// 연결된 클라이언트가 없으면 즉시 반환한다.
func (ep *EventPublisher) PublishDeviceEvent(eventType, agentName string) {
	if ep.hub.ClientCount() == 0 {
		return
	}

	payload := deviceStatusPayload{
		EventType: eventType,
		AgentName: agentName,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := ep.hub.BroadcastMessage(TypeDeviceStatus, payload); err != nil {
		ep.logger.Error("디바이스 이벤트 브로드캐스트 실패",
			"eventType", eventType,
			"agentName", agentName,
			"error", err,
		)
	}
}

// PublishDeviceStateChangedV2 는 SPEC-DEVICE-IDENTITY-001 의 디바이스 상태 변경
// 브로드캐스트 API 이다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (M3 / D-T3, xflowd v1.0 — Breaking):
// composite alias (`device_id`) 파라미터와 페이로드 필드가 완전 제거되었다.
// 함수 시그니처는 외부 호출자 (V2 callback wrapper) 의 호환성을 위해
// 두 번째 인자를 유지하나, 값은 무시된다 (페이로드에 반영되지 않음).
//
// 인자:
//   - deviceUID: 디바이스의 글로벌 UUID v4 (Device.UID()). 빈 문자열이면
//     "uid" 키가 omitempty 로 페이로드에서 생략된다 (graceful degradation).
//   - _: legacy composite key — Phase D 부터 무시됨. 추후 메이저에서 제거 예정.
//
// 페이로드 호환성:
//   - 외부 클라이언트는 web/src/hooks/useDevice.ts 의 onDeviceStatus 핸들러로
//     단순 refetch 트리거로 사용하므로 (payload: unknown), payload schema
//     축소는 비파괴적이다 (Frontend M11 마이그레이션 완료 — D-T20).
func (ep *EventPublisher) PublishDeviceStateChangedV2(deviceUID, _ string) {
	if ep.hub.ClientCount() == 0 {
		return
	}

	payload := deviceStatusPayload{
		EventType: "device_state_changed",
		UID:       deviceUID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := ep.hub.BroadcastMessage(TypeDeviceStatus, payload); err != nil {
		ep.logger.Error("디바이스 상태 변경 브로드캐스트 실패",
			"deviceUID", deviceUID,
			"error", err,
		)
	}
}
