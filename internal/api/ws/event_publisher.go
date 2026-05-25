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
// SPEC-DEVICE-IDENTITY-001 Phase B (M3 / B-T3):
//
//   - UID 는 글로벌 UUID v4 (Device.UID()) 의 1급 식별 필드이다. Phase B 부터
//     모든 WebSocket 디바이스 이벤트의 1순위 식별자이며 v1.0 (Phase D) 까지
//     유지된다.
//   - DeviceID 는 v0.x 의 composite key ("agent_name:local_id" — 예: "lgcnp:81")
//     로 외부 클라이언트의 호환 alias 이다. Phase D 에서 제거 예정. omitempty
//     로 빈 값은 키 자체를 생략한다.
//   - graceful degradation: UID 가 빈 문자열인 경우 ("DeviceIDRepository" 미설정
//     / 매핑 부재) "uid" 키를 생략하고 DeviceID 만 페이로드에 남긴다.
//   - 기존 외부 클라이언트는 device.status 메시지 수신 시 payload 의 특정
//     필드에 결합하지 않고 단순히 refetch 트리거로 사용하므로, 새 필드 추가는
//     비파괴적이다 (web/src/services/ws/wsHandlers.ts 의 unknown 페이로드).
type deviceStatusPayload struct {
	EventType string `json:"event_type"`
	AgentName string `json:"agent_name"`
	UID       string `json:"uid,omitempty"`       // SPEC-DEVICE-IDENTITY-001 Phase B — 1급 식별자
	DeviceID  string `json:"device_id,omitempty"` // composite alias (Phase D 에서 제거)
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

// PublishDeviceStateChanged 는 디바이스 속성 변경 이벤트를 브로드캐스트한다.
// 커맨드 실행 후 프론트엔드가 최신 상태를 다시 가져오도록 알린다.
//
// Deprecated: SPEC-DEVICE-IDENTITY-001 Phase B (M3 / B-T3) 부터
// PublishDeviceStateChangedV2 가 1급 API 이다. 본 함수는 호출자가 UUID 를
// 제공할 수 없는 경로 (예: 기존 v1 콜백 어댑터) 에서만 사용되며 v1.0
// (Phase D) 에서 제거 예정이다. 신규 호출자는 V2 를 사용하라.
//
// deviceID 는 composite key ("agent_name:local_id") 로 가정한다.
func (ep *EventPublisher) PublishDeviceStateChanged(deviceID string) {
	// V2 경로로 위임: UID 는 빈 문자열 (graceful degradation), DeviceID 는 composite 로 그대로.
	ep.PublishDeviceStateChangedV2("", deviceID)
}

// PublishDeviceStateChangedV2 는 SPEC-DEVICE-IDENTITY-001 Phase B (M3 / B-T3)
// 의 1급 디바이스 상태 변경 브로드캐스트 API 이다.
//
// 인자:
//   - deviceUID: 디바이스의 글로벌 UUID v4 (Device.UID()). 빈 문자열이면
//     "uid" 키가 omitempty 로 페이로드에서 생략된다 (graceful degradation).
//   - deviceCompositeID: v0.x 호환 composite key ("agent_name:local_id"). 빈
//     문자열이면 "device_id" 키가 omitempty 로 생략된다.
//
// 두 인자가 모두 빈 문자열이어도 호출자는 panic 없이 안전하게 호출 가능하며,
// 페이로드는 EventType / Timestamp / AgentName 만 포함한다 (디바이스 식별
// 정보 없는 일반 알림). 운영상 두 값 모두 빈 케이스는 드물지만 callback 등
// 호출 사이트가 항상 양쪽 모두를 보유한다고 가정하지 않는다 (TRUST 5: Tested).
//
// 페이로드 호환성:
//   - 외부 클라이언트는 web/src/hooks/useDevice.ts 의 onDeviceStatus 핸들러로
//     단순 refetch 트리거로 사용하므로 (payload: unknown), 새 "uid" 필드 추가는
//     비파괴적이다.
//   - 기존 "device_id" 필드는 Phase D 까지 alias 로 유지된다.
func (ep *EventPublisher) PublishDeviceStateChangedV2(deviceUID, deviceCompositeID string) {
	if ep.hub.ClientCount() == 0 {
		return
	}

	payload := deviceStatusPayload{
		EventType: "device_state_changed",
		UID:       deviceUID,
		DeviceID:  deviceCompositeID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	if err := ep.hub.BroadcastMessage(TypeDeviceStatus, payload); err != nil {
		ep.logger.Error("디바이스 상태 변경 브로드캐스트 실패",
			"deviceUID", deviceUID,
			"deviceID", deviceCompositeID,
			"error", err,
		)
	}
}
