package ws

import "time"

// DebugMessagePayload 는 debug.message WebSocket 메시지의 페이로드이다.
type DebugMessagePayload struct {
	NodeID    string `json:"node_id"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

// DebugSink 는 WebSocket Hub를 통해 디버그 메시지를 브로드캐스트하는 node.DebugSink 구현체이다.
type DebugSink struct {
	hub *Hub
}

// NewDebugSink 는 지정된 Hub로 메시지를 전송하는 DebugSink를 생성한다.
func NewDebugSink(hub *Hub) *DebugSink {
	return &DebugSink{hub: hub}
}

// SendDebug 는 nodeID와 메시지를 WebSocket으로 브로드캐스트한다.
func (s *DebugSink) SendDebug(nodeID string, message string) error {
	return s.hub.BroadcastMessage(TypeDebugMessage, DebugMessagePayload{
		NodeID:    nodeID,
		Message:   message,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}
