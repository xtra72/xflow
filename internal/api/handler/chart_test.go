package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent/system"
	"github.com/xtra/xflow/internal/api"
)

// setupChartRouter 는 ChartHandler 를 탑재한 테스트 라우터를 만든다.
// registry 가 nil 이면 DefaultChartChannelRegistry() 가 사용된다 (방어적 가드 테스트용).
func setupChartRouter(t *testing.T, reg *system.ChartChannelRegistry) *api.Router {
	t.Helper()
	// 전역 레지스트리를 테스트 스코프에서 잠시 교체한다.
	prev := system.DefaultChartChannelRegistry()
	system.SetDefaultChartChannelRegistry(reg)
	t.Cleanup(func() { system.SetDefaultChartChannelRegistry(prev) })

	router := api.NewRouter()
	h := NewChartHandler(nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

func TestChartHandler_RegisterRoutes(t *testing.T) {
	t.Parallel()
	reg := system.NewChartChannelRegistry()
	t.Cleanup(reg.Close)

	router := api.NewRouter()
	h := NewChartHandler(nil)
	g := router.Group("/api/v1")

	before := router.RouteCount()
	h.RegisterRoutes(g)
	after := router.RouteCount()

	// GET /charts/channels 만 등록된다
	assert.Equal(t, 1, after-before)
}

func TestChartHandler_ListChannels_빈레지스트리(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	t.Cleanup(reg.Close)
	router := setupChartRouter(t, reg)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/charts/channels", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Channels []system.ChartChannelInfo `json:"channels"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.True(t, resp.Success)
	assert.NotNil(t, resp.Data.Channels)
	assert.Len(t, resp.Data.Channels, 0)
}

func TestChartHandler_ListChannels_두채널(t *testing.T) {
	reg := system.NewChartChannelRegistry()
	t.Cleanup(reg.Close)
	_, err := reg.Register("alpha", "flow-a", "node-1", 50, 600)
	require.NoError(t, err)
	_, err = reg.Register("beta", "flow-b", "node-2", 200, 3600)
	require.NoError(t, err)

	router := setupChartRouter(t, reg)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/charts/channels", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			Channels []system.ChartChannelInfo `json:"channels"`
		} `json:"data"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	require.Len(t, resp.Data.Channels, 2)
	// List() 는 이름 오름차순으로 반환한다.
	assert.Equal(t, "alpha", resp.Data.Channels[0].Name)
	assert.Equal(t, "flow-a", resp.Data.Channels[0].FlowID)
	assert.Equal(t, "node-1", resp.Data.Channels[0].NodeID)
	assert.Equal(t, 50, resp.Data.Channels[0].BufferSize)
	assert.Equal(t, 600, resp.Data.Channels[0].RetentionSec)

	assert.Equal(t, "beta", resp.Data.Channels[1].Name)
	assert.Equal(t, 200, resp.Data.Channels[1].BufferSize)
	assert.Equal(t, 3600, resp.Data.Channels[1].RetentionSec)
}

func TestChartHandler_ListChannels_nil레지스트리_500(t *testing.T) {
	// DefaultChartChannelRegistry 가 설정되지 않으면 500 을 반환해야 한다.
	router := setupChartRouter(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/charts/channels", nil)
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	var resp struct {
		Success bool `json:"success"`
		Error   struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&resp))
	assert.False(t, resp.Success)
	assert.NotEmpty(t, resp.Error.Code)
}
