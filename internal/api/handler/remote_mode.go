// remote_mode.go 는 구성된 원격 관리 모드를 보고하는 경량 엔드포인트를 제공한다
// (@SPEC:SPEC-REMOTE-001).
//
// remote_admin.go 의 admin 라우트가 server 모드에서만 등록되는 것과 달리, 본
// 엔드포인트는 모든 모드(server/client/disabled)에서 무조건 등록된다. 이를 통해
// Web UI 가 인스턴스가 관리 서버인지 감지하여 server 전용 엔드포인트(예:
// /remote/nodes)의 불필요한 호출을 회피할 수 있다.
//
// 라우트(api/v1 그룹 하위):
//
//	GET /remote/mode — 구성된 원격 관리 모드 보고
//
// admin 권한을 요구하지 않는다(로그인 사용자 누구나 모드를 조회 가능). 표준
// /api/v1 인증 미들웨어만 적용된다.
package handler

import (
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// RemoteModeDTO 는 원격 관리 모드 응답 표현이다.
type RemoteModeDTO struct {
	Mode string `json:"mode"` // "server" | "client" | "disabled"
}

// RemoteModeHandler 는 구성된 원격 관리 모드를 보고한다. 전체 원격 서버가 아니라
// 모드 문자열만 주입받으므로 client/disabled(remoteServer 가 nil) 모드에서도 동작한다.
type RemoteModeHandler struct {
	mode string
}

// NewRemoteModeHandler 는 주입된 모드 문자열로 RemoteModeHandler 를 생성한다.
// 빈 모드(미설정)는 "disabled" 로 정규화한다(기본값 안전).
func NewRemoteModeHandler(mode string) *RemoteModeHandler {
	if mode == "" {
		mode = "disabled"
	}
	return &RemoteModeHandler{mode: mode}
}

// RegisterRoutes 는 모드 조회 라우트를 그룹에 등록한다.
func (h *RemoteModeHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — 원격 하위 API 는 remote.* 단일 키로만 다룬다
	// (spec.md §1.3 비범위: 원격 노드 하위 API 의 세분 권한).
	g.GETPerm("/remote/mode", "remote.read", h.Mode)
}

// Mode 는 구성된 원격 관리 모드를 반환한다. GET /remote/mode
func (h *RemoteModeHandler) Mode(ctx api.Context) error {
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(RemoteModeDTO{Mode: h.mode}))
}
