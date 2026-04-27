package handler

import (
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// ChartHandler 는 SPEC-CHART-001 M5 에서 프런트엔드가 활성 chart-emitter
// 채널을 조회할 때 사용하는 핸들러이다.
//
// 라우트:
//
//	GET /charts/channels -> ListChannels
//
// 내부적으로는 프로세스 전역 singleton 인 system.DefaultChartChannelRegistry()
// 를 참조한다. 레지스트리가 설정되지 않은 경우(예: 초기화 이전/테스트)에는
// HTTP 500 을 반환한다.
type ChartHandler struct {
	logger *slog.Logger
}

// NewChartHandler 는 새 ChartHandler 를 생성한다.
// logger 가 nil 이면 slog.Default() 가 사용된다.
func NewChartHandler(logger *slog.Logger) *ChartHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ChartHandler{logger: logger}
}

// RegisterRoutes 는 chart 관련 라우트를 등록한다.
func (h *ChartHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/charts/channels", h.ListChannels)
}

// chartChannelsResponse 는 채널 목록 응답 데이터 구조이다.
// SPEC-CHART-001 REQ-M5-02 형식을 따른다.
type chartChannelsResponse struct {
	Channels []system.ChartChannelInfo `json:"channels"`
}

// ListChannels 는 현재 활성화된 모든 chart-emitter 채널의 요약 정보를 반환한다.
// GET /charts/channels
func (h *ChartHandler) ListChannels(ctx api.Context) error {
	reg := system.DefaultChartChannelRegistry()
	if reg == nil {
		h.logger.Warn("chart channel registry is not initialized")
		return api.ErrInternalServer.WithMessage("chart channel registry is not initialized")
	}

	channels := reg.List()
	if channels == nil {
		channels = []system.ChartChannelInfo{}
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(chartChannelsResponse{Channels: channels}))
}
