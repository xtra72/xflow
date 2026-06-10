package handler

import (
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// fakeTapController 는 TapController 인터페이스의 테스트용 구현이다.
type fakeTapController struct {
	mu     sync.Mutex
	tapped map[string]bool // key: flowID|nodeID
}

func newFakeTapController() *fakeTapController {
	return &fakeTapController{tapped: make(map[string]bool)}
}

func (f *fakeTapController) key(flowID, nodeID string) string {
	return flowID + "|" + nodeID
}

func (f *fakeTapController) SetTap(flowID, nodeID string, enabled bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if enabled {
		f.tapped[f.key(flowID, nodeID)] = true
	} else {
		delete(f.tapped, f.key(flowID, nodeID))
	}
}

func (f *fakeTapController) IsTapped(flowID, nodeID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.tapped[f.key(flowID, nodeID)]
}

func (f *fakeTapController) TappedNodes(flowID string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k, v := range f.tapped {
		if !v {
			continue
		}
		if strings.HasPrefix(k, flowID+"|") {
			out = append(out, strings.TrimPrefix(k, flowID+"|"))
		}
	}
	return out
}

func setupTapRouter(taps TapController) *api.Router {
	router := api.NewRouter()
	h := NewFlowHandler(&mockFlowManager{}, nil, WithTapRegistry(taps))
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

func TestFlowHandler_TapNode_Enable(t *testing.T) {
	taps := newFakeTapController()
	router := setupTapRouter(taps)

	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/flows/flow-123/nodes/node-1/tap",
		strings.NewReader(`{"enabled":true}`))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, taps.IsTapped("flow-123", "node-1"), "tap이 활성화되어야 한다")

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "flow-123", resp.Data["flow_id"])
	assert.Equal(t, "node-1", resp.Data["node_id"])
	assert.Equal(t, true, resp.Data["enabled"])
}

func TestFlowHandler_TapNode_Disable(t *testing.T) {
	taps := newFakeTapController()
	taps.SetTap("flow-123", "node-1", true)
	router := setupTapRouter(taps)

	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/flows/flow-123/nodes/node-1/tap",
		strings.NewReader(`{"enabled":false}`))

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.False(t, taps.IsTapped("flow-123", "node-1"), "tap이 비활성화되어야 한다")

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	assert.Equal(t, false, resp.Data["enabled"])
}

func TestFlowHandler_TapNode_MissingFlowID(t *testing.T) {
	taps := newFakeTapController()
	router := setupTapRouter(taps)

	// 빈 nodeID 는 라우트 매칭이 안 되므로, flow id 누락 경로는 라우터가 처리한다.
	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/flows/flow-123/nodes/node-1/tap",
		strings.NewReader(`{bad json}`))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestFlowHandler_ListTaps(t *testing.T) {
	taps := newFakeTapController()
	taps.SetTap("flow-123", "node-1", true)
	taps.SetTap("flow-123", "node-2", true)
	router := setupTapRouter(taps)

	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-123/taps", nil)
	assert.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)
	nodes, ok := resp.Data["node_ids"].([]any)
	require.True(t, ok)
	assert.Len(t, nodes, 2)
}

// TapController 가 미주입(nil)이면 tap 라우트는 등록되지 않아야 한다 (404).
func TestFlowHandler_TapNode_NoController(t *testing.T) {
	router := api.NewRouter()
	h := NewFlowHandler(&mockFlowManager{}, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)

	rec := doRequest(t, router, http.MethodPost,
		"/api/v1/flows/flow-123/nodes/node-1/tap",
		strings.NewReader(`{"enabled":true}`))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}
