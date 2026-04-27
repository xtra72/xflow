package ws

import (
	"log/slog"
	"sync/atomic"
)

// Hub 는 모든 WebSocket 클라이언트를 관리하는 중앙 허브이다.
// 클라이언트 등록/해제와 메시지 브로드캐스트를 처리한다.
type Hub struct {
	// clients 는 등록된 클라이언트 집합이다.
	clients map[*Client]struct{}

	// register 는 클라이언트 등록 채널이다.
	register chan *Client

	// unregister 는 클라이언트 해제 채널이다.
	unregister chan *Client

	// broadcast 는 모든 클라이언트에 보낼 메시지 채널이다.
	broadcast chan []byte

	// done 은 Hub 종료를 알리는 채널이다.
	done chan struct{}

	// clientCount 는 현재 연결된 클라이언트 수 (원자적 접근).
	clientCount atomic.Int64

	logger *slog.Logger
}

// NewHub 는 새 Hub를 생성한다.
func NewHub(logger *slog.Logger) *Hub {
	if logger == nil {
		logger = slog.Default()
	}
	return &Hub{
		clients:    make(map[*Client]struct{}),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 256),
		done:       make(chan struct{}),
		logger:     logger,
	}
}

// Run 은 Hub의 메인 이벤트 루프를 실행한다.
// 클라이언트 등록/해제 및 브로드캐스트를 처리한다.
// 반드시 별도 고루틴에서 실행되어야 한다.
// Stop()이 호출되면 루프가 종료된다.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.clients[client] = struct{}{}
			h.clientCount.Add(1)
			h.logger.Info("WebSocket 클라이언트 연결",
				"total", h.clientCount.Load(),
			)

		case client := <-h.unregister:
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
				h.clientCount.Add(-1)
				h.logger.Info("WebSocket 클라이언트 해제",
					"total", h.clientCount.Load(),
				)
			}

		case message := <-h.broadcast:
			for client := range h.clients {
				client.Send(message)
			}

		case <-h.done:
			// 모든 클라이언트 정리 후 종료
			for client := range h.clients {
				close(client.send)
				delete(h.clients, client)
			}
			h.clientCount.Store(0)
			h.logger.Info("WebSocket 허브 종료")
			return
		}
	}
}

// Stop 은 Hub의 이벤트 루프를 종료한다.
// 모든 연결된 클라이언트가 정리된다.
func (h *Hub) Stop() {
	close(h.done)
}

// Register 는 클라이언트를 허브에 등록한다.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister 는 클라이언트를 허브에서 해제한다.
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast 는 모든 연결된 클라이언트에게 원시 메시지를 전송한다.
func (h *Hub) Broadcast(data []byte) {
	h.broadcast <- data
}

// BroadcastMessage 는 주어진 타입과 페이로드로 메시지를 생성하여
// 모든 연결된 클라이언트에게 전송한다.
func (h *Hub) BroadcastMessage(msgType string, payload any) error {
	msg, err := NewMessage(msgType, payload)
	if err != nil {
		return err
	}
	data, err := msg.Encode()
	if err != nil {
		return err
	}
	h.Broadcast(data)
	return nil
}

// ClientCount 는 현재 연결된 클라이언트 수를 반환한다.
func (h *Hub) ClientCount() int64 {
	return h.clientCount.Load()
}
