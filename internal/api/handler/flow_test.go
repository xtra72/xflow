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
	listFlowsFn    func(ctx context.Context, opts dto.ListOptions) ([]FlowInfo, int64, error)
	getFlowFn      func(ctx context.Context, id string) (*FlowInfo, error)
	createFlowFn   func(ctx context.Context, req *dto.FlowCreateRequest) (*FlowInfo, error)
	updateFlowFn   func(ctx context.Context, id string, req *dto.FlowUpdateRequest) (*FlowInfo, error)
	deleteFlowFn   func(ctx context.Context, id string) error
	deployFlowFn   func(ctx context.Context, id string) error
	startFlowFn    func(ctx context.Context, id string) error
	stopFlowFn     func(ctx context.Context, id string) error
	restartFlowFn    func(ctx context.Context, id string) error
	undeployFlowFn   func(ctx context.Context, id string) error
	configureFlowFn  func(ctx context.Context, id string, cfg map[string]any) error
	flowStatusFn     func(ctx context.Context, id string) (*FlowStatusInfo, error)
	listFlowNodesFn  func(ctx context.Context, flowID string) ([]FlowNodeInfo, error)
	getFlowNodeFn    func(ctx context.Context, flowID, nodeID string) (*FlowNodeInfo, error)
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
		name           string
		url            string
		mock           *mockFlowManager
		expectedCode   int
		expectedTotal  int64
		expectedLen    int
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
			expectedCode:   http.StatusOK,
			expectedTotal:  2,
			expectedLen:    2,
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
