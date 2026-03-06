package ws

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait 은 메시지 쓰기 타임아웃이다.
	writeWait = 10 * time.Second

	// pongWait 은 클라이언트로부터 pong 응답을 기다리는 시간이다.
	pongWait = 60 * time.Second

	// pingPeriod 는 서버가 핑을 보내는 주기이다.
	// pongWait 보다 짧아야 한다 (약 90%).
	pingPeriod = 54 * time.Second

	// maxMessageSize 는 클라이언트가 보내는 메시지의 최대 크기(바이트)이다.
	// 클라이언트 메시지는 주로 핑이므로 512바이트면 충분하다.
	maxMessageSize = 512

	// sendBufferSize 는 클라이언트 전송 채널의 버퍼 크기이다.
	sendBufferSize = 256
)

// Client 는 WebSocket 연결을 래핑하는 클라이언트 구조체이다.
// 각 클라이언트는 읽기/쓰기 고루틴을 가진다.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	logger *slog.Logger
	once   sync.Once
}

// NewClient 는 새 WebSocket 클라이언트를 생성한다.
func NewClient(hub *Hub, conn *websocket.Conn, logger *slog.Logger) *Client {
	if logger == nil {
		logger = slog.Default()
	}
	return &Client{
		hub:    hub,
		conn:   conn,
		send:   make(chan []byte, sendBufferSize),
		logger: logger,
	}
}

// Send 는 클라이언트에게 메시지를 전송한다.
// 채널이 가득 찬 경우 메시지를 버린다 (느린 클라이언트 방지).
func (c *Client) Send(data []byte) {
	select {
	case c.send <- data:
	default:
		c.logger.Warn("클라이언트 전송 채널 가득 참, 메시지 버림")
	}
}

// ReadPump 은 WebSocket 연결에서 메시지를 읽는 고루틴이다.
// 클라이언트가 보내는 ping 메시지를 처리하고, 연결 종료 시 정리한다.
// 반드시 별도 고루틴에서 실행되어야 한다.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	if err := c.conn.SetReadDeadline(time.Now().Add(pongWait)); err != nil {
		c.logger.Error("ReadDeadline 설정 실패", "error", err)
		return
	}
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
			) {
				// pong timeout은 브라우저 탭 비활성화 등 일반적 상황이므로 Debug 레벨
				if strings.Contains(err.Error(), "pong timeout") {
					c.logger.Debug("WebSocket pong timeout으로 종료", "error", err)
				} else {
					c.logger.Warn("WebSocket 비정상 종료", "error", err)
				}
			}
			return
		}

		// 클라이언트 메시지 처리 (ping -> pong 응답)
		c.handleMessage(message)
	}
}

// WritePump 은 send 채널의 메시지를 WebSocket 연결에 쓰는 고루틴이다.
// 주기적으로 핑 메시지를 보내 연결 유지를 확인한다.
// 반드시 별도 고루틴에서 실행되어야 한다.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Error("WriteDeadline 설정 실패", "error", err)
				return
			}
			if !ok {
				// Hub 가 채널을 닫음 -> 클라이언트 연결 종료 메시지 전송
				_ = c.conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}

			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				c.logger.Error("WebSocket 쓰기 실패", "error", err)
				return
			}

		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				c.logger.Error("WriteDeadline 설정 실패", "error", err)
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// handleMessage 는 클라이언트가 보낸 메시지를 처리한다.
// 현재는 ping 메시지에 대한 pong 응답만 처리한다.
func (c *Client) handleMessage(data []byte) {
	msg, err := DecodeMessage(data)
	if err != nil {
		c.logger.Debug("잘못된 메시지 포맷", "error", err)
		return
	}

	switch msg.Type {
	case TypePing:
		pong, err := NewMessage(TypePong, nil)
		if err != nil {
			c.logger.Error("pong 메시지 생성 실패", "error", err)
			return
		}
		encoded, err := pong.Encode()
		if err != nil {
			c.logger.Error("pong 메시지 직렬화 실패", "error", err)
			return
		}
		c.Send(encoded)

	default:
		c.logger.Debug("알 수 없는 메시지 타입", "type", msg.Type)
	}
}

// close 는 WebSocket 연결을 닫는다. 한 번만 실행된다.
func (c *Client) close() {
	c.once.Do(func() {
		_ = c.conn.Close()
	})
}
