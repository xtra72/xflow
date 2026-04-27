package handler

import (
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/xtra/xflow/internal/auth"
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
	hub         *ws.Hub
	logger      *slog.Logger
	authEnabled bool
	jwtSvc      *auth.JWTService
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

// WithWebSocketAuth 는 WebSocket 핸들러에 JWT 인증을 설정한다.
func (h *WebSocketHandler) WithWebSocketAuth(jwtSvc *auth.JWTService) *WebSocketHandler {
	h.authEnabled = true
	h.jwtSvc = jwtSvc
	return h
}

// HandleUpgrade 는 HTTP 연결을 WebSocket으로 업그레이드하는 http.HandlerFunc 이다.
// Router의 HandleFunc를 통해 직접 등록된다 (Context 래퍼 바이패스).
// basic_auth 활성화 시 Authorization 헤더 또는 ?token= 쿼리 파라미터에서 JWT를 검증한다.
func (h *WebSocketHandler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	// JWT 인증 검증 (활성화된 경우)
	if h.authEnabled && h.jwtSvc != nil {
		token := ""

		// 1) Authorization 헤더에서 Bearer 토큰 추출
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			token = authHeader[7:]
		}

		// 2) 쿼리 파라미터에서 토큰 추출 (WebSocket 클라이언트용 폴백)
		if token == "" {
			token = r.URL.Query().Get("token")
		}

		if token == "" {
			h.logger.Warn("WebSocket 인증 실패: 토큰 없음",
				"remote_addr", r.RemoteAddr,
			)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 블랙리스트 확인
		if h.jwtSvc.IsBlacklisted(token) {
			h.logger.Warn("WebSocket 인증 실패: 블랙리스트 토큰",
				"remote_addr", r.RemoteAddr,
			)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// 토큰 검증
		if _, err := h.jwtSvc.ValidateToken(token); err != nil {
			h.logger.Warn("WebSocket 인증 실패: 유효하지 않은 토큰",
				"error", err,
				"remote_addr", r.RemoteAddr,
			)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}

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
