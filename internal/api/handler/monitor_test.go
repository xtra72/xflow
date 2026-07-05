package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/observe"
)

// newMonitorTestRouter 는 MonitorHandler 를 등록한 라우터를 생성한다.
// levels=nil 이면 로그 레벨 기능은 비활성이지만 logstyle 은 observe 전역 상태를
// 사용하므로 독립적으로 동작한다.
func newMonitorTestRouter(t *testing.T) *api.Router {
	t.Helper()
	mgr := NewDefaultMonitorManager(nil, nil)
	h := NewMonitorHandler(mgr, nil)

	router := api.NewRouter()
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// doMonitorReq 는 라우터에 요청을 보내고 응답 레코더를 반환한다.
func doMonitorReq(t *testing.T, router *api.Router, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		bodyReader = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, bodyReader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	router.Handler().ServeHTTP(w, req)
	return w
}

func parseStyleResp(t *testing.T, w *httptest.ResponseRecorder) dto.APIResponse[logStyleResponse] {
	t.Helper()
	var resp dto.APIResponse[logStyleResponse]
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	return resp
}

// TestMonitorHandler_GetLogStyle - 현재 스타일 조회.
func TestMonitorHandler_GetLogStyle(t *testing.T) {
	t.Cleanup(func() { observe.SetLogIDStyle(observe.IDStyleBoth) })
	observe.SetLogIDStyle(observe.IDStyleBoth)

	router := newMonitorTestRouter(t)
	w := doMonitorReq(t, router, http.MethodGet, "/api/v1/monitor/logstyle", nil)

	require.Equal(t, http.StatusOK, w.Code)
	resp := parseStyleResp(t, w)
	assert.True(t, resp.Success)
	assert.Equal(t, "both", resp.Data.Style)
}

// TestMonitorHandler_SetLogStyle - 유효한 스타일 변경.
func TestMonitorHandler_SetLogStyle(t *testing.T) {
	t.Cleanup(func() { observe.SetLogIDStyle(observe.IDStyleBoth) })

	tests := []struct {
		name string
		body map[string]string
		want string
	}{
		{"name 모드", map[string]string{"style": "name"}, "name"},
		{"id 모드", map[string]string{"style": "id"}, "id"},
		{"both 모드", map[string]string{"style": "both"}, "both"},
		{"대문자 정규화", map[string]string{"style": "NAME"}, "name"},
		{"공백 트림", map[string]string{"style": "  id  "}, "id"},
	}

	router := newMonitorTestRouter(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := doMonitorReq(t, router, http.MethodPut, "/api/v1/monitor/logstyle", tt.body)

			require.Equal(t, http.StatusOK, w.Code)
			resp := parseStyleResp(t, w)
			assert.True(t, resp.Success)
			assert.Equal(t, tt.want, resp.Data.Style)
			// 전역 상태에도 실제 반영되었는지 확인.
			assert.Equal(t, tt.want, observe.GetLogIDStyle())
		})
	}
}

// TestMonitorHandler_SetLogStyle_Invalid - 유효하지 않은 스타일은 400 반환.
func TestMonitorHandler_SetLogStyle_Invalid(t *testing.T) {
	t.Cleanup(func() { observe.SetLogIDStyle(observe.IDStyleBoth) })
	observe.SetLogIDStyle(observe.IDStyleBoth)

	router := newMonitorTestRouter(t)
	w := doMonitorReq(t, router, http.MethodPut, "/api/v1/monitor/logstyle",
		map[string]string{"style": "garbage"})

	require.Equal(t, http.StatusBadRequest, w.Code)
	resp := parseStyleResp(t, w)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "INVALID_LOG_STYLE", resp.Error.Code)
	// 무효 요청은 전역 상태를 변경하지 않아야 한다.
	assert.Equal(t, "both", observe.GetLogIDStyle())
}

// TestMonitorHandler_SetLogStyle_BadBody - 파싱 불가 본문은 400 반환.
func TestMonitorHandler_SetLogStyle_BadBody(t *testing.T) {
	t.Cleanup(func() { observe.SetLogIDStyle(observe.IDStyleBoth) })

	router := newMonitorTestRouter(t)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/monitor/logstyle",
		bytes.NewReader([]byte("not-json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.Handler().ServeHTTP(w, req)

	require.Equal(t, http.StatusBadRequest, w.Code)
	resp := parseStyleResp(t, w)
	assert.False(t, resp.Success)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "INVALID_REQUEST", resp.Error.Code)
}

// TestDefaultMonitorManager_LogIDStyle - 매니저 계층의 Get/Set 직접 검증.
func TestDefaultMonitorManager_LogIDStyle(t *testing.T) {
	t.Cleanup(func() { observe.SetLogIDStyle(observe.IDStyleBoth) })

	mgr := NewDefaultMonitorManager(nil, nil)

	require.NoError(t, mgr.SetLogIDStyle(t.Context(), "name"))
	assert.Equal(t, "name", mgr.GetLogIDStyle(t.Context()))

	err := mgr.SetLogIDStyle(t.Context(), "garbage")
	require.Error(t, err)
	assert.Equal(t, "name", mgr.GetLogIDStyle(t.Context()), "무효 값은 반영되지 않아야 한다")
}
