package handler

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/xtra/xflow/internal/api/ws"
)

// websocket.Upgrader 설정.
// ReadBufferSize, WriteBufferSize: 1024 바이트.
// CheckOrigin: 개발 환경에서 모든 오리진 허용 (프로덕션에서는 제한 필요).
var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// TODO: 프로덕션 환경에서는 허용된 오리진만 허용하도록 변경
		return true
	},
}

// WebSocketHandler 는 WebSocket 업그레이드 요청을 처리하는 핸들러이다.
type WebSocketHandler struct {
	hub    *ws.Hub
	logger *slog.Logger
}

// NewWebSocketHandler 는 새 WebSocketHandler를 생성한다.
func NewWebSocketHandler(hub *ws.Hub, logger *slog.Logger) *WebSocketHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &WebSocketHandler{
		hub:    hub,
		logger: logger,
	}
}

// HandleUpgrade 는 HTTP 연결을 WebSocket으로 업그레이드하는 http.HandlerFunc 이다.
// Router의 HandleFunc를 통해 직접 등록된다 (Context 래퍼 바이패스).
func (h *WebSocketHandler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("WebSocket 업그레이드 실패",
			"error", err,
			"remote_addr", r.RemoteAddr,
		)
		return
	}

	client := ws.NewClient(h.hub, conn, h.logger)
	h.hub.Register(client)

	// 읽기/쓰기 고루틴 시작
	go client.WritePump()
	go client.ReadPump()

	h.logger.Debug("WebSocket 연결 성공",
		"remote_addr", r.RemoteAddr,
	)
}
