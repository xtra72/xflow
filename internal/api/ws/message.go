package ws

import (
	"encoding/json"
	"time"
)

// 프론트엔드에서 사용하는 WebSocket 메시지 타입 상수.
const (
	// TypeFlowStatus 는 플로우 상태 업데이트 메시지 타입이다.
	TypeFlowStatus = "flow.status"
	// TypeFlowMetrics 는 플로우 성능 메트릭 메시지 타입이다.
	TypeFlowMetrics = "flow.metrics"
	// TypeNodeStats 는 노드별 통계 메시지 타입이다.
	TypeNodeStats = "node.stats"
	// TypeAgentStatus 는 에이전트 연결 상태 메시지 타입이다.
	TypeAgentStatus = "agent.status"
	// TypeLogEntry 는 실시간 로그 항목 메시지 타입이다.
	TypeLogEntry = "log.entry"
	// TypeSystemEvent 는 시스템 이벤트 메시지 타입이다.
	TypeSystemEvent = "system.event"

	// TypePing 은 클라이언트가 보내는 핑 메시지 타입이다.
	TypePing = "ping"
	// TypePong 은 서버가 응답하는 퐁 메시지 타입이다.
	TypePong = "pong"
)

// Message 는 WebSocket 을 통해 전송되는 JSON 메시지 구조체이다.
// 프론트엔드 클라이언트와 동일한 포맷을 사용한다.
type Message struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp string          `json:"timestamp"`
}

// NewMessage 는 주어진 타입과 페이로드로 새 Message를 생성한다.
// 타임스탬프는 현재 시각의 ISO 8601 포맷으로 설정된다.
func NewMessage(msgType string, payload any) (*Message, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &Message{
		Type:      msgType,
		Payload:   data,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// Encode 는 Message를 JSON 바이트로 직렬화한다.
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage 는 JSON 바이트를 Message로 역직렬화한다.
func DecodeMessage(data []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}
