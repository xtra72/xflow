// dashboard_limits.go — 대시보드 편집기가 서버의 상한을 읽어 가는 창구.
//
// **왜 필요한가.** 캔버스 SVG 가져오기의 요소 상한은 그동안 프론트엔드의 컴파일 상수였고
// (`svgImportTypes.ts` 의 `MAX_IMPORT_ELEMENTS`), 대시보드 저장 예산은 서버의 컴파일
// 상수였다. 둘은 서로 모르는 채 어긋났고, 그 어긋남이 **저장 시점의 413** 으로 나타났다
// (SPEC-CANVAS-007 §결정 13 이 그 위험을 이름으로 적어 두었다). 이제 운영자가 요소 수를
// 설정하고 서버가 그 수에서 예산을 유도하므로, **프론트엔드가 같은 수를 읽어야** 두 층이
// 다시 어긋나지 않는다.
//
// 이 창구가 하는 일은 그 하나다 — 읽기 전용이고, 쓰기 경로가 없다.
package handler

import (
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/config"
)

// DashboardLimitsHandler 는 대시보드/캔버스 상한을 조회하는 읽기 전용 엔드포인트다.
//
// ScheduleLogConfigHandler 의 형상(config.Config 주입 → 읽어서 응답)을 따르되 **admin
// 게이트를 두지 않는다**. 이 값은 비밀이 아니라 편집기가 동작하려면 알아야 하는 수이고,
// admin 으로 죄면 admin 이 아닌 편집자는 언제나 컴파일 기본값으로 떨어져 — 운영자가
// 상한을 올려도 그들에게만 가져오기가 거절된다. 그 조용한 갈라짐이 이 창구가 없애려는
// 바로 그 부류의 결함이다.
type DashboardLimitsHandler struct {
	cfg    config.Config
	logger *slog.Logger
}

// NewDashboardLimitsHandler 는 새 핸들러를 생성한다.
func NewDashboardLimitsHandler(cfg config.Config, logger *slog.Logger) *DashboardLimitsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DashboardLimitsHandler{cfg: cfg, logger: logger}
}

// RegisterRoutes 는 상한 조회 라우트를 등록한다.
//
//	GET /dashboard-limits   dashboard.read
//
// **최상위 경로인 것에 뜻이 있다.** `/dashboards/{uid}` 아래에 두면 uid 와 경로가 겹치고,
// 그 충돌을 피하려고 예약어를 하나 더 늘려야 한다 — `/dashboard-state` 가 같은 이유로
// 최상위에 선 그 선례를 따른다(dashboard.go §RegisterRoutes).
//
// 권한은 `dashboard.read` 다. 대시보드를 읽을 수 있는 사람이 곧 거기에 그림을 가져올
// 사람이므로 새 권한 키를 만들지 않는다 — 카탈로그에 없는 자원을 가장 가까운 키로 매핑한
// `chart.go` 의 선례와 같다.
func (h *DashboardLimitsHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GETPerm("/dashboard-limits", "dashboard.read", h.Get)
}

// Get 은 현재 적용 중인 상한을 반환한다.
//
// 응답의 두 수는 **같은 출처에서 유도된 한 쌍**이다(config.Dashboard). 프론트엔드는
// max_canvas_elements 를 가져오기 상한으로 쓰고, payload_budget_bytes 는 화면이
// "약 N KB / 예산의 M%" 를 말할 때의 분모다.
func (h *DashboardLimitsHandler) Get(ctx api.Context) error {
	dash := h.cfg.Dashboard()
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"max_canvas_elements":  dash.MaxCanvasElements,
		"payload_budget_bytes": dash.PayloadBudgetBytes,
	}))
}
