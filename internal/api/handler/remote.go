// remote.go 는 관리 WS 엔드포인트(GET /api/remote/ws)의 업그레이드 핸들러이다
// (@SPEC:SPEC-REMOTE-001 M1, REQ-REMOTE-B01/B02/N02).
//
// 이 엔드포인트는 웹 UI 모니터링 WS(/ws)와 반드시 분리된다(REQ-N02). 노드(관리
// 클라이언트)가 이 엔드포인트로 dial 하면, 핸들러는 연결을 업그레이드하여
// remote.Server 로 넘긴다. Server 가 hello/heartbeat 를 읽어 online/offline 을
// 추적한다.
//
// M2 seam: 인증 핸드셰이크(노드 토큰/부트스트랩 시크릿 검증)는 현재 websocket.go
// 의 Bearer/?token= 패턴을 준용하되, M1 은 인증을 강제하지 않는다(remote.Server
// 의 Authenticator seam 이 수락). M2 에서 토큰 검증·등록 승인 게이팅을 추가한다.
package handler

import (
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/remote"
)

// RemoteWSPattern 은 관리 WS 엔드포인트의 라우트 패턴이다. 모니터링 /ws 와
// 분리된 별도 경로이다(REQ-REMOTE-N02).
const RemoteWSPattern = "GET /api/remote/ws"

// RemoteHandler 는 관리 WS 업그레이드 요청을 처리한다.
type RemoteHandler struct {
	server *remote.Server
	logger *slog.Logger
}

// NewRemoteHandler 는 RemoteHandler 를 생성한다.
func NewRemoteHandler(server *remote.Server, logger *slog.Logger) *RemoteHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteHandler{
		server: server,
		logger: logger,
	}
}

// HandleUpgrade 는 HTTP 연결을 관리 WS 로 업그레이드하고 remote.Server 에 위임한다.
// 모니터링 WS 와 동일한 upgrader(websocket.go)를 재사용하되, 별도 엔드포인트에서
// 동작한다.
//
// 각 연결은 자체 고루틴(이 핸들러 호출 자체)에서 처리되며, 요청 컨텍스트가
// 취소되면(연결 종료/서버 종료) Server.HandleConnection 이 정리된다.
func (h *RemoteHandler) HandleUpgrade(w http.ResponseWriter, r *http.Request) {
	// M2 seam: 여기서 Bearer/?token= 토큰 검증을 추가한다(websocket.go 패턴 준용).
	// M1 은 업그레이드만 수행하고 hello 기반 식별은 remote.Server 가 담당한다.

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.logger.Error("관리 WS 업그레이드 실패",
			"error", err, "remote_addr", r.RemoteAddr)
		return
	}

	h.logger.Debug("관리 WS 연결 수립", "remote_addr", r.RemoteAddr)

	// 연결 수명은 요청 컨텍스트에 종속된다. 연결 종료 시 ReadMessage 가 에러를
	// 반환하여 HandleConnection 이 반환한다.
	ctx := r.Context()
	if err := h.server.HandleConnection(ctx, remote.NewGorillaConn(conn)); err != nil {
		h.logger.Debug("관리 WS 연결 종료", "error", err, "remote_addr", r.RemoteAddr)
	}
}

// 컴파일 타임: HandleUpgrade 가 http.HandlerFunc 와 호환됨을 보장.
var _ http.HandlerFunc = (*RemoteHandler)(nil).HandleUpgrade
