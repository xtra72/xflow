package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// --- Mock FlowManager ---

type mockFlowManager struct {
	listFlowsFn     func(ctx context.Context, opts dto.ListOptions) ([]FlowInfo, int64, error)
	getFlowFn       func(ctx context.Context, id string) (*FlowInfo, error)
	createFlowFn    func(ctx context.Context, req *dto.FlowCreateRequest) (*FlowInfo, error)
	updateFlowFn    func(ctx context.Context, id string, req *dto.FlowUpdateRequest) (*FlowInfo, error)
	deleteFlowFn    func(ctx context.Context, id string) error
	deployFlowFn    func(ctx context.Context, id string) error
	startFlowFn     func(ctx context.Context, id string) error
	stopFlowFn      func(ctx context.Context, id string) error
	restartFlowFn   func(ctx context.Context, id string) error
	undeployFlowFn  func(ctx context.Context, id string) error
	configureFlowFn func(ctx context.Context, id string, cfg map[string]any) error
	flowStatusFn    func(ctx context.Context, id string) (*FlowStatusInfo, error)
	listFlowNodesFn func(ctx context.Context, flowID string) ([]FlowNodeInfo, error)
	getFlowNodeFn   func(ctx context.Context, flowID, nodeID string) (*FlowNodeInfo, error)
}

func (m *mockFlowManager) ListFlows(ctx context.Context, opts dto.ListOptions) ([]FlowInfo, int64, error) {
	if m.listFlowsFn != nil {
		return m.listFlowsFn(ctx, opts)
	}
	return nil, 0, nil
}

func (m *mockFlowManager) GetFlow(ctx context.Context, id string) (*FlowInfo, error) {
	if m.getFlowFn != nil {
		return m.getFlowFn(ctx, id)
	}
	return nil, nil
}

func (m *mockFlowManager) CreateFlow(ctx context.Context, req *dto.FlowCreateRequest) (*FlowInfo, error) {
	if m.createFlowFn != nil {
		return m.createFlowFn(ctx, req)
	}
	return nil, nil
}

func (m *mockFlowManager) UpdateFlow(ctx context.Context, id string, req *dto.FlowUpdateRequest) (*FlowInfo, error) {
	if m.updateFlowFn != nil {
		return m.updateFlowFn(ctx, id, req)
	}
	return nil, nil
}

func (m *mockFlowManager) DeleteFlow(ctx context.Context, id string) error {
	if m.deleteFlowFn != nil {
		return m.deleteFlowFn(ctx, id)
	}
	return nil
}

func (m *mockFlowManager) DeployFlow(ctx context.Context, id string) error {
	if m.deployFlowFn != nil {
		return m.deployFlowFn(ctx, id)
	}
	return nil
}

func (m *mockFlowManager) StartFlow(ctx context.Context, id string) error {
	if m.startFlowFn != nil {
		return m.startFlowFn(ctx, id)
	}
	return nil
}

func (m *mockFlowManager) StopFlow(ctx context.Context, id string) error {
	if m.stopFlowFn != nil {
		return m.stopFlowFn(ctx, id)
	}
	return nil
}

func (m *mockFlowManager) RestartFlow(ctx context.Context, id string) error {
	if m.restartFlowFn != nil {
		return m.restartFlowFn(ctx, id)
	}
	return nil
}

func (m *mockFlowManager) UndeployFlow(ctx context.Context, id string) error {
	if m.undeployFlowFn != nil {
		return m.undeployFlowFn(ctx, id)
	}
	return nil
}

func (m *mockFlowManager) ConfigureFlow(ctx context.Context, id string, cfg map[string]any) error {
	if m.configureFlowFn != nil {
		return m.configureFlowFn(ctx, id, cfg)
	}
	return nil
}

func (m *mockFlowManager) FlowStatus(ctx context.Context, id string) (*FlowStatusInfo, error) {
	if m.flowStatusFn != nil {
		return m.flowStatusFn(ctx, id)
	}
	return nil, nil
}

func (m *mockFlowManager) ListFlowNodes(ctx context.Context, flowID string) ([]FlowNodeInfo, error) {
	if m.listFlowNodesFn != nil {
		return m.listFlowNodesFn(ctx, flowID)
	}
	return nil, nil
}

func (m *mockFlowManager) GetFlowNode(ctx context.Context, flowID, nodeID string) (*FlowNodeInfo, error) {
	if m.getFlowNodeFn != nil {
		return m.getFlowNodeFn(ctx, flowID, nodeID)
	}
	return nil, nil
}

func (m *mockFlowManager) RenameAgentInFlows(_ context.Context, _, _ string) (int, error) {
	return 0, nil
}

// --- Test Helpers ---

// doRequest 는 HTTP 요청을 생성하고 라우터를 통해 처리한다.
func doRequest(t *testing.T, router *api.Router, method, path string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// setupFlowRouter 는 FlowHandler가 등록된 라우터를 생성한다.
func setupFlowRouter(mock *mockFlowManager) *api.Router {
	router := api.NewRouter()
	h := NewFlowHandler(mock, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// decodeJSON 은 응답 본문을 JSON으로 디코딩한다.
func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	err := json.NewDecoder(rec.Body).Decode(v)
	require.NoError(t, err)
}

// --- FlowHandler 생성 테스트 ---

func TestNewFlowHandler(t *testing.T) {
	mock := &mockFlowManager{}

	t.Run("with logger", func(t *testing.T) {
		h := NewFlowHandler(mock, nil)
		require.NotNil(t, h)
		assert.NotNil(t, h.logger)
		assert.Equal(t, mock, h.flows)
	})
}

// --- RegisterRoutes 테스트 ---

func TestFlowHandler_RegisterRoutes(t *testing.T) {
	router := setupFlowRouter(&mockFlowManager{})
	// 16개 라우트 등록 확인 (기존 11 + ListNodes, GetNode + Export, ExportAll + Undeploy)
	assert.Equal(t, 16, router.RouteCount())
}

// --- List 테스트 ---

func TestFlowHandler_List(t *testing.T) {
	tests := []struct {
		name            string
		url             string
		mock            *mockFlowManager
		expectedCode    int
		expectedTotal   int64
		expectedLen     int
		checkPagination bool
	}{
		{
			name: "성공: 빈 목록",
			url:  "/api/v1/flows",
			mock: &mockFlowManager{
				listFlowsFn: func(_ context.Context, _ dto.ListOptions) ([]FlowInfo, int64, error) {
					return []FlowInfo{}, 0, nil
				},
			},
			expectedCode:  http.StatusOK,
			expectedTotal: 0,
			expectedLen:   0,
		},
		{
			name: "성공: 플로우 목록",
			url:  "/api/v1/flows?page=1&size=10&status=running",
			mock: &mockFlowManager{
				listFlowsFn: func(_ context.Context, opts dto.ListOptions) ([]FlowInfo, int64, error) {
					assert.Equal(t, 1, opts.Page)
					assert.Equal(t, 10, opts.Size)
					assert.Equal(t, "running", opts.Status)
					return []FlowInfo{
						{ID: "f1", Name: "flow1", Status: "running"},
						{ID: "f2", Name: "flow2", Status: "running"},
					}, 2, nil
				},
			},
			expectedCode:    http.StatusOK,
			expectedTotal:   2,
			expectedLen:     2,
			checkPagination: true,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/flows",
			mock: &mockFlowManager{
				listFlowsFn: func(_ context.Context, _ dto.ListOptions) ([]FlowInfo, int64, error) {
					return nil, 0, errors.New("internal error")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[[]FlowInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Len(t, resp.Data, tt.expectedLen)

				if tt.checkPagination {
					require.NotNil(t, resp.Meta)
					require.NotNil(t, resp.Meta.Pagination)
					assert.Equal(t, tt.expectedTotal, resp.Meta.Pagination.Total)
					assert.Equal(t, 1, resp.Meta.Pagination.Page)
					assert.Equal(t, 10, resp.Meta.Pagination.Size)
				}
			}
		})
	}
}

// --- Get 테스트 ---

func TestFlowHandler_Get(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 조회",
			url:  "/api/v1/flows/flow-123",
			mock: &mockFlowManager{
				getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
					assert.Equal(t, "flow-123", id)
					return &FlowInfo{ID: "flow-123", Name: "test-flow", Status: "running"}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 도메인 에러 (not found 매핑)",
			url:  "/api/v1/flows/not-found",
			mock: &mockFlowManager{
				getFlowFn: func(_ context.Context, _ string) (*FlowInfo, error) {
					return nil, errors.New("not found")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[*FlowInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "flow-123", resp.Data.ID)
				assert.Equal(t, "test-flow", resp.Data.Name)
			}
		})
	}
}

// --- Create 테스트 ---

func TestFlowHandler_Create(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 생성",
			body: `{"name":"new-flow","definition":{"nodes":[]}}`,
			mock: &mockFlowManager{
				createFlowFn: func(_ context.Context, req *dto.FlowCreateRequest) (*FlowInfo, error) {
					assert.Equal(t, "new-flow", req.Name)
					return &FlowInfo{ID: "new-id", Name: "new-flow", Status: "created"}, nil
				},
			},
			expectedCode: http.StatusCreated,
		},
		{
			name:         "에러: 유효성 검증 실패 (이름 누락)",
			body:         `{"definition":{"nodes":[]}}`,
			mock:         &mockFlowManager{},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name:         "에러: 유효성 검증 실패 (definition 누락)",
			body:         `{"name":"test"}`,
			mock:         &mockFlowManager{},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name:         "에러: 잘못된 JSON",
			body:         `invalid json`,
			mock:         &mockFlowManager{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "에러: 도메인 에러",
			body: `{"name":"new-flow","definition":{"nodes":[]}}`,
			mock: &mockFlowManager{
				createFlowFn: func(_ context.Context, _ *dto.FlowCreateRequest) (*FlowInfo, error) {
					return nil, errors.New("create failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPost, "/api/v1/flows", strings.NewReader(tt.body))
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusCreated {
				var resp dto.APIResponse[*FlowInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "new-id", resp.Data.ID)
			}
		})
	}
}

// --- Update 테스트 ---

func TestFlowHandler_Update(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		body         string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 업데이트",
			url:  "/api/v1/flows/flow-123",
			body: `{"name":"updated-name"}`,
			mock: &mockFlowManager{
				updateFlowFn: func(_ context.Context, id string, req *dto.FlowUpdateRequest) (*FlowInfo, error) {
					assert.Equal(t, "flow-123", id)
					require.NotNil(t, req.Name)
					assert.Equal(t, "updated-name", *req.Name)
					return &FlowInfo{ID: "flow-123", Name: "updated-name", Status: "running"}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "에러: 잘못된 JSON",
			url:          "/api/v1/flows/flow-123",
			body:         `{invalid}`,
			mock:         &mockFlowManager{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/flows/flow-123",
			body: `{"name":"updated"}`,
			mock: &mockFlowManager{
				updateFlowFn: func(_ context.Context, _ string, _ *dto.FlowUpdateRequest) (*FlowInfo, error) {
					return nil, errors.New("update failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPut, tt.url, strings.NewReader(tt.body))
			assert.Equal(t, tt.expectedCode, rec.Code)
		})
	}
}

// --- Delete 테스트 ---

func TestFlowHandler_Delete(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 삭제",
			url:  "/api/v1/flows/flow-123",
			mock: &mockFlowManager{
				deleteFlowFn: func(_ context.Context, id string) error {
					assert.Equal(t, "flow-123", id)
					return nil
				},
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "에러: 삭제 실패",
			url:  "/api/v1/flows/flow-123",
			mock: &mockFlowManager{
				deleteFlowFn: func(_ context.Context, _ string) error {
					return errors.New("delete failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodDelete, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusNoContent {
				assert.Empty(t, rec.Body.String())
			}
		})
	}
}

// --- Deploy 테스트 ---

func TestFlowHandler_Deploy(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 배포",
			url:  "/api/v1/flows/flow-123/deploy",
			mock: &mockFlowManager{
				deployFlowFn: func(_ context.Context, id string) error {
					assert.Equal(t, "flow-123", id)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 배포 실패",
			url:  "/api/v1/flows/flow-123/deploy",
			mock: &mockFlowManager{
				deployFlowFn: func(_ context.Context, _ string) error {
					return errors.New("deploy failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPost, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[map[string]string]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "deployed", resp.Data["status"])
			}
		})
	}
}

// --- Start 테스트 ---

func TestFlowHandler_Start(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 시작",
			url:  "/api/v1/flows/flow-123/start",
			mock: &mockFlowManager{
				startFlowFn: func(_ context.Context, id string) error {
					assert.Equal(t, "flow-123", id)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 시작 실패",
			url:  "/api/v1/flows/flow-123/start",
			mock: &mockFlowManager{
				startFlowFn: func(_ context.Context, _ string) error {
					return errors.New("start failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPost, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[map[string]string]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "started", resp.Data["status"])
			}
		})
	}
}

// --- Stop 테스트 ---

func TestFlowHandler_Stop(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 정지",
			url:  "/api/v1/flows/flow-123/stop",
			mock: &mockFlowManager{
				stopFlowFn: func(_ context.Context, id string) error {
					assert.Equal(t, "flow-123", id)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 정지 실패",
			url:  "/api/v1/flows/flow-123/stop",
			mock: &mockFlowManager{
				stopFlowFn: func(_ context.Context, _ string) error {
					return errors.New("stop failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPost, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[map[string]string]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "stopped", resp.Data["status"])
			}
		})
	}
}

// --- Restart 테스트 ---

func TestFlowHandler_Restart(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 플로우 재시작",
			url:  "/api/v1/flows/flow-123/restart",
			mock: &mockFlowManager{
				restartFlowFn: func(_ context.Context, id string) error {
					assert.Equal(t, "flow-123", id)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 재시작 실패",
			url:  "/api/v1/flows/flow-123/restart",
			mock: &mockFlowManager{
				restartFlowFn: func(_ context.Context, _ string) error {
					return errors.New("restart failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPost, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[map[string]string]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "restarted", resp.Data["status"])
			}
		})
	}
}

// --- Configure 테스트 ---

func TestFlowHandler_Configure(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		body         string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 설정 업데이트",
			url:  "/api/v1/flows/flow-123/config",
			body: `{"config":{"key":"value"}}`,
			mock: &mockFlowManager{
				configureFlowFn: func(_ context.Context, id string, cfg map[string]any) error {
					assert.Equal(t, "flow-123", id)
					assert.Equal(t, "value", cfg["key"])
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "에러: 유효성 검증 실패 (config 누락)",
			url:          "/api/v1/flows/flow-123/config",
			body:         `{}`,
			mock:         &mockFlowManager{},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name:         "에러: 잘못된 JSON",
			url:          "/api/v1/flows/flow-123/config",
			body:         `{invalid}`,
			mock:         &mockFlowManager{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/flows/flow-123/config",
			body: `{"config":{"key":"value"}}`,
			mock: &mockFlowManager{
				configureFlowFn: func(_ context.Context, _ string, _ map[string]any) error {
					return errors.New("configure failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPut, tt.url, strings.NewReader(tt.body))
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[map[string]string]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "configured", resp.Data["status"])
			}
		})
	}
}

// --- Status 테스트 ---

func TestFlowHandler_Status(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name: "성공: 상태 조회",
			url:  "/api/v1/flows/flow-123/status",
			mock: &mockFlowManager{
				flowStatusFn: func(_ context.Context, id string) (*FlowStatusInfo, error) {
					assert.Equal(t, "flow-123", id)
					return &FlowStatusInfo{
						ID:           "flow-123",
						Status:       "running",
						Uptime:       "1h30m",
						MessageCount: 1000,
						ErrorCount:   5,
						NodeStats: []NodeStatInfo{
							{NodeID: "n1", NodeType: "source", Processed: 500, Errors: 2},
						},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 상태 조회 실패",
			url:  "/api/v1/flows/flow-123/status",
			mock: &mockFlowManager{
				flowStatusFn: func(_ context.Context, _ string) (*FlowStatusInfo, error) {
					return nil, errors.New("status failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[*FlowStatusInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "flow-123", resp.Data.ID)
				assert.Equal(t, "running", resp.Data.Status)
				assert.Equal(t, int64(1000), resp.Data.MessageCount)
				assert.Len(t, resp.Data.NodeStats, 1)
			}
		})
	}
}

// --- parsePagination 테스트 ---

func TestParsePagination(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		expectedPage int
		expectedSize int
	}{
		{
			name:         "기본값",
			url:          "/test",
			expectedPage: 1,
			expectedSize: 20,
		},
		{
			name:         "사용자 지정 값",
			url:          "/test?page=3&size=50",
			expectedPage: 3,
			expectedSize: 50,
		},
		{
			name:         "0 이하 페이지는 1로 정규화",
			url:          "/test?page=0&size=10",
			expectedPage: 1,
			expectedSize: 10,
		},
		{
			name:         "100 초과 크기는 100으로 정규화",
			url:          "/test?page=1&size=200",
			expectedPage: 1,
			expectedSize: 100,
		},
		{
			name:         "음수 크기는 기본값으로",
			url:          "/test?page=1&size=-5",
			expectedPage: 1,
			expectedSize: 20,
		},
		{
			name:         "비숫자 값은 기본값으로",
			url:          "/test?page=abc&size=xyz",
			expectedPage: 1,
			expectedSize: 20,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := api.NewRouter()
			var gotPage, gotSize int

			router.GET("/test", func(ctx api.Context) error {
				p := parsePagination(ctx)
				gotPage = p.Page
				gotSize = p.Size
				return ctx.NoContent(http.StatusOK)
			})

			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Equal(t, tt.expectedPage, gotPage)
			assert.Equal(t, tt.expectedSize, gotSize)
		})
	}
}

// --- parseListOptions 테스트 ---

func TestParseListOptions(t *testing.T) {
	router := api.NewRouter()
	var opts dto.ListOptions

	router.GET("/test", func(ctx api.Context) error {
		opts = parseListOptions(ctx)
		return ctx.NoContent(http.StatusOK)
	})

	rec := doRequest(t, router, http.MethodGet, "/test?page=2&size=15&sort=name&filter=active&status=running", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 2, opts.Page)
	assert.Equal(t, 15, opts.Size)
	assert.Equal(t, "name", opts.Sort)
	assert.Equal(t, "active", opts.Filter)
	assert.Equal(t, "running", opts.Status)
}

// --- ListNodes 테스트 ---

func TestFlowHandler_ListNodes(t *testing.T) {
	tests := []struct {
		name         string
		flowID       string
		mock         *mockFlowManager
		expectedCode int
		expectedLen  int
	}{
		{
			name:   "성공: 노드 목록 반환",
			flowID: "flow-001",
			mock: &mockFlowManager{
				listFlowNodesFn: func(_ context.Context, flowID string) ([]FlowNodeInfo, error) {
					return []FlowNodeInfo{
						{NodeID: "n1", Name: "filter-1", Type: "filter", State: "running"},
						{NodeID: "n2", Name: "transform-1", Type: "transform", State: "running"},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  2,
		},
		{
			name:   "성공: 빈 노드 목록",
			flowID: "flow-002",
			mock: &mockFlowManager{
				listFlowNodesFn: func(_ context.Context, flowID string) ([]FlowNodeInfo, error) {
					return []FlowNodeInfo{}, nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  0,
		},
		{
			name:   "실패: 플로우 없음",
			flowID: "nonexistent",
			mock: &mockFlowManager{
				listFlowNodesFn: func(_ context.Context, flowID string) ([]FlowNodeInfo, error) {
					return nil, errors.New("flow not found")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			url := "/api/v1/flows/" + tt.flowID + "/nodes"
			rec := doRequest(t, router, http.MethodGet, url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[[]FlowNodeInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Len(t, resp.Data, tt.expectedLen)
			}
		})
	}
}

// --- GetNode 테스트 ---

func TestFlowHandler_GetNode(t *testing.T) {
	tests := []struct {
		name         string
		flowID       string
		nodeID       string
		mock         *mockFlowManager
		expectedCode int
	}{
		{
			name:   "성공: 노드 상세 조회",
			flowID: "flow-001",
			nodeID: "n1",
			mock: &mockFlowManager{
				getFlowNodeFn: func(_ context.Context, flowID, nodeID string) (*FlowNodeInfo, error) {
					return &FlowNodeInfo{
						NodeID: "n1",
						Name:   "filter-1",
						Type:   "filter",
						State:  "running",
						Config: map[string]any{"condition": "x > 0"},
						Ports: []PortInfo{
							{ID: "in", Name: "in", Direction: "input", Connected: true},
							{ID: "out", Name: "out", Direction: "output", Connected: true},
						},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:   "실패: 노드 없음",
			flowID: "flow-001",
			nodeID: "nonexistent",
			mock: &mockFlowManager{
				getFlowNodeFn: func(_ context.Context, flowID, nodeID string) (*FlowNodeInfo, error) {
					return nil, errors.New("node not found")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupFlowRouter(tt.mock)
			url := "/api/v1/flows/" + tt.flowID + "/nodes/" + tt.nodeID
			rec := doRequest(t, router, http.MethodGet, url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[FlowNodeInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, tt.nodeID, resp.Data.NodeID)
				assert.NotEmpty(t, resp.Data.Ports)
			}
		})
	}
}

// --- Export / ExportAll required_agents 회귀 테스트 ---
//
// 배경: flowToReactFlowConfig 는 노드의 agent 정보를 data.agent_id / data.agent_name
// 의 flat 형태로 직렬화하지만 extractAgentNames 는 XFlow 표준인 agent_ref 중첩 객체를
// 기대한다. 따라서 Export 가 info.Config 를 그대로 extractAgentNames 에 넘기면
// 항상 빈 슬라이스가 반환되어 required_agents 가 누락된다.
// 본 테스트는 separateLayoutFields 변환 결과를 사용해야 한다는 계약을 고정한다.

// setupFlowRouterWithAgents 는 FlowHandler 를 AgentManager 와 함께 구성한 라우터를 생성한다.
// mockAgentManager 는 agent_test.go 에서 정의된 것을 재사용한다.
func setupFlowRouterWithAgents(flowMock *mockFlowManager, agentsMock AgentManager) *api.Router {
	router := api.NewRouter()
	h := NewFlowHandler(flowMock, nil, WithAgentManager(agentsMock))
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// reactFlowNode 는 flowToReactFlowConfig 가 생성하는 노드 형태를 모사한다.
//   - type: "custom"
//   - data: { label, nodeType, agent_id, agent_name, ... }  (FLAT)
func reactFlowNode(id, label, agentName string) map[string]any {
	data := map[string]any{
		"label":    label,
		"nodeType": label,
		"category": "agent",
		"status":   "draft",
		"enabled":  true,
	}
	if agentName != "" {
		data["agent_id"] = "agent-" + agentName
		data["agent_name"] = agentName
		data["direction"] = "external"
		data["agent_type"] = ""
	}
	return map[string]any{
		"id":   id,
		"type": "custom",
		"position": map[string]any{
			"x": 0.0,
			"y": 0.0,
		},
		"data": data,
	}
}

// reactFlowConfig 는 flowToReactFlowConfig 의 결과 형태(노드 slice 가 []map[string]any)를
// 그대로 모사하여 Export 가 처리하는 실제 입력을 재현한다.
func reactFlowConfig(nodes ...map[string]any) map[string]any {
	return map[string]any{
		"nodes": nodes,
		"edges": []map[string]any{},
	}
}

func TestExport_IncludesRequiredAgentsWhenAgentRefPresent(t *testing.T) {
	// 4개 노드 중 3개가 서로 다른 agent_name 을 참조한다.
	cfg := reactFlowConfig(
		reactFlowNode("n1", "lgcnp-status", "lgcnp"),
		reactFlowNode("n2", "withio-pub", "data.withio.net"),
		reactFlowNode("n3", "tsdb-writer", "tsdb"),
		reactFlowNode("n4", "filter", ""), // 에이전트 미참조
	)

	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "flow-A", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{
				{ID: "1", Name: "lgcnp", Type: "lgcnp", Config: map[string]any{"host": "x"}},
				{ID: "2", Name: "data.withio.net", Type: "mqtt"},
				// "tsdb" 는 일부러 등록하지 않아 이름만 노출되는 경우를 동시에 확인한다.
			}, 2, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-A/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)

	raw, ok := resp.Data["required_agents"]
	require.True(t, ok, "required_agents 키가 응답에 존재해야 한다")

	agents, ok := raw.([]any)
	require.True(t, ok, "required_agents 는 배열이어야 한다")
	require.Len(t, agents, 3, "참조된 distinct agent_name 개수와 일치해야 한다")

	// 순서는 노드 탐색 순서를 따른다 (insertion order).
	names := make([]string, 0, len(agents))
	for _, a := range agents {
		entry, ok := a.(map[string]any)
		require.True(t, ok)
		name, _ := entry["name"].(string)
		names = append(names, name)
	}
	assert.Equal(t, []string{"lgcnp", "data.withio.net", "tsdb"}, names)

	// resolve 가능한 에이전트는 type / config 가 채워져야 한다.
	lgcnp := agents[0].(map[string]any)
	assert.Equal(t, "lgcnp", lgcnp["type"])
	assert.NotNil(t, lgcnp["config"])

	withio := agents[1].(map[string]any)
	assert.Equal(t, "mqtt", withio["type"])

	// resolve 실패한 이름은 name 만 있고 type/config 가 없어야 한다.
	tsdb := agents[2].(map[string]any)
	assert.Equal(t, "tsdb", tsdb["name"])
	_, hasType := tsdb["type"]
	assert.False(t, hasType)
	_, hasCfg := tsdb["config"]
	assert.False(t, hasCfg)
}

func TestExport_NoRequiredAgentsWhenNoAgentRef(t *testing.T) {
	// 에이전트 참조가 전혀 없는 플로우는 required_agents 키 자체가 없어야 한다.
	cfg := reactFlowConfig(
		reactFlowNode("n1", "filter", ""),
		reactFlowNode("n2", "router", ""),
	)
	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "flow-empty", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{}, 0, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-empty/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)

	_, hasRequired := resp.Data["required_agents"]
	assert.False(t, hasRequired, "에이전트 미참조 플로우는 required_agents 키가 없어야 한다")
}

func TestExportAll_IncludesRequiredAgentsPerFlow(t *testing.T) {
	// flow-A: 2개 distinct agent_name. flow-B: 0개.
	cfgA := reactFlowConfig(
		reactFlowNode("a1", "lgcnp-status", "lgcnp"),
		reactFlowNode("a2", "influx-writer", "influxdb"),
	)
	cfgB := reactFlowConfig(
		reactFlowNode("b1", "filter", ""),
	)

	flows := []FlowInfo{
		{ID: "flow-A", Name: "flow-A", Status: "draft"},
		{ID: "flow-B", Name: "flow-B", Status: "draft"},
	}
	flowMock := &mockFlowManager{
		listFlowsFn: func(_ context.Context, _ dto.ListOptions) ([]FlowInfo, int64, error) {
			return flows, int64(len(flows)), nil
		},
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			switch id {
			case "flow-A":
				return &FlowInfo{ID: id, Name: "flow-A", Status: "draft", Config: cfgA}, nil
			case "flow-B":
				return &FlowInfo{ID: id, Name: "flow-B", Status: "draft", Config: cfgB}, nil
			}
			return nil, errors.New("unknown flow")
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{
				{ID: "1", Name: "lgcnp", Type: "lgcnp"},
				{ID: "2", Name: "influxdb", Type: "influxdb"},
			}, 2, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[[]map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)
	require.Len(t, resp.Data, 2)

	// 응답에서 flow-A / flow-B 를 이름으로 찾는다 (순서는 ListFlows 가 보장).
	itemA := resp.Data[0]
	itemB := resp.Data[1]
	require.Equal(t, "flow-A", itemA["name"])
	require.Equal(t, "flow-B", itemB["name"])

	// flow-A 는 required_agents 가 있어야 하고 길이는 2 이다.
	rawA, ok := itemA["required_agents"]
	require.True(t, ok, "flow-A 는 required_agents 가 있어야 한다")
	agentsA, ok := rawA.([]any)
	require.True(t, ok)
	require.Len(t, agentsA, 2)
	namesA := []string{
		agentsA[0].(map[string]any)["name"].(string),
		agentsA[1].(map[string]any)["name"].(string),
	}
	assert.Equal(t, []string{"lgcnp", "influxdb"}, namesA)

	// flow-B 는 required_agents 키가 없어야 한다.
	_, hasRequiredB := itemB["required_agents"]
	assert.False(t, hasRequiredB, "flow-B 는 required_agents 키가 없어야 한다")
}

func TestExport_AgentNameWithoutMatchingAgentRecord(t *testing.T) {
	// 에이전트 서비스가 "ghost" 를 등록하지 않더라도 required_agents 에는
	// 이름만 채워진 엔트리가 포함되어야 한다 (resolveAgentExports 의 기존 의미론).
	cfg := reactFlowConfig(
		reactFlowNode("n1", "ghost-node", "ghost"),
	)
	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "flow-ghost", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{}, 0, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-ghost/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)

	raw, ok := resp.Data["required_agents"]
	require.True(t, ok, "이름이 매칭되지 않더라도 required_agents 자체는 존재해야 한다")
	agents, ok := raw.([]any)
	require.True(t, ok)
	require.Len(t, agents, 1)

	entry := agents[0].(map[string]any)
	assert.Equal(t, "ghost", entry["name"])
	_, hasType := entry["type"]
	assert.False(t, hasType, "매칭되지 않은 에이전트는 type 이 없어야 한다")
	_, hasCfg := entry["config"]
	assert.False(t, hasCfg, "매칭되지 않은 에이전트는 config 가 없어야 한다")
}

func TestExport_AgentNameCaseInsensitiveMatch(t *testing.T) {
	// agent_id 는 재프로비저닝 시 변하므로 환경 간 이식성이 없다.
	// 매칭은 (name, type) 으로만 수행하되, name 비교는 대소문자 무시(case-insensitive)
	// 로 한다. 단, 응답의 name 필드는 flow 가 참조한 원본 케이스를 그대로 보존해야
	// 다운스트림 매칭(예: UI ImportDialog)이 깨지지 않는다.
	cfg := reactFlowConfig(
		reactFlowNode("n1", "lgcnp-status", "lgcnp"),
		reactFlowNode("n2", "tsdb-writer", "tsdb"),
		reactFlowNode("n3", "influx-writer", "influxdb"),
	)

	flowMock := &mockFlowManager{
		getFlowFn: func(_ context.Context, id string) (*FlowInfo, error) {
			return &FlowInfo{ID: id, Name: "flow-mixedcase", Status: "draft", Config: cfg}, nil
		},
	}
	agentsMock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			// 등록 측은 혼합 케이스(LGCNP, TSDB, Influxdb) — flow 측은 소문자 참조.
			return []AgentInfo{
				{ID: "1", Name: "LGCNP", Type: "lgcnp", Config: map[string]any{"host": "x"}},
				{ID: "2", Name: "TSDB", Type: "tsdb"},
				{ID: "3", Name: "Influxdb", Type: "influxdb"},
			}, 3, nil
		},
	}

	router := setupFlowRouterWithAgents(flowMock, agentsMock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/flows/flow-mixedcase/export", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)

	raw, ok := resp.Data["required_agents"]
	require.True(t, ok, "required_agents 키가 응답에 존재해야 한다")
	agents, ok := raw.([]any)
	require.True(t, ok, "required_agents 는 배열이어야 한다")
	require.Len(t, agents, 3, "참조된 distinct agent_name 개수와 일치해야 한다")

	// name 은 flow 참조 원본(소문자) 을 보존, type 은 등록 레코드에서 채워진다.
	lgcnp := agents[0].(map[string]any)
	assert.Equal(t, "lgcnp", lgcnp["name"], "name 은 flow 참조 원본 케이스를 보존해야 한다")
	assert.Equal(t, "lgcnp", lgcnp["type"], "type 은 대소문자 무시 매칭으로 채워져야 한다")
	assert.NotNil(t, lgcnp["config"], "config 도 함께 채워져야 한다")

	tsdb := agents[1].(map[string]any)
	assert.Equal(t, "tsdb", tsdb["name"])
	assert.Equal(t, "tsdb", tsdb["type"])

	influx := agents[2].(map[string]any)
	assert.Equal(t, "influxdb", influx["name"])
	assert.Equal(t, "influxdb", influx["type"])
}
