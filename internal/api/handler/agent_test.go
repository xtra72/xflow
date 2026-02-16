package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// --- Mock AgentManager ---

type mockAgentManager struct {
	listAgentsFn    func(ctx context.Context, opts dto.ListOptions) ([]AgentInfo, int64, error)
	getAgentFn      func(ctx context.Context, id string) (*AgentInfo, error)
	createAgentFn   func(ctx context.Context, req *dto.AgentCreateRequest) (*AgentInfo, error)
	updateAgentFn   func(ctx context.Context, id string, req *dto.AgentUpdateRequest) (*AgentInfo, error)
	deleteAgentFn   func(ctx context.Context, id string) error
	startAgentFn    func(ctx context.Context, id string) error
	stopAgentFn     func(ctx context.Context, id string) error
	restartAgentFn  func(ctx context.Context, id string) error
	configureAgentFn func(ctx context.Context, id string, cfg map[string]any) error
	agentStatsFn    func(ctx context.Context, id string) (*AgentStatsInfo, error)
}

func (m *mockAgentManager) ListAgents(ctx context.Context, opts dto.ListOptions) ([]AgentInfo, int64, error) {
	if m.listAgentsFn != nil {
		return m.listAgentsFn(ctx, opts)
	}
	return nil, 0, nil
}

func (m *mockAgentManager) GetAgent(ctx context.Context, id string) (*AgentInfo, error) {
	if m.getAgentFn != nil {
		return m.getAgentFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAgentManager) CreateAgent(ctx context.Context, req *dto.AgentCreateRequest) (*AgentInfo, error) {
	if m.createAgentFn != nil {
		return m.createAgentFn(ctx, req)
	}
	return nil, nil
}

func (m *mockAgentManager) UpdateAgent(ctx context.Context, id string, req *dto.AgentUpdateRequest) (*AgentInfo, error) {
	if m.updateAgentFn != nil {
		return m.updateAgentFn(ctx, id, req)
	}
	return nil, nil
}

func (m *mockAgentManager) DeleteAgent(ctx context.Context, id string) error {
	if m.deleteAgentFn != nil {
		return m.deleteAgentFn(ctx, id)
	}
	return nil
}

func (m *mockAgentManager) StartAgent(ctx context.Context, id string) error {
	if m.startAgentFn != nil {
		return m.startAgentFn(ctx, id)
	}
	return nil
}

func (m *mockAgentManager) StopAgent(ctx context.Context, id string) error {
	if m.stopAgentFn != nil {
		return m.stopAgentFn(ctx, id)
	}
	return nil
}

func (m *mockAgentManager) RestartAgent(ctx context.Context, id string) error {
	if m.restartAgentFn != nil {
		return m.restartAgentFn(ctx, id)
	}
	return nil
}

func (m *mockAgentManager) ConfigureAgent(ctx context.Context, id string, cfg map[string]any) error {
	if m.configureAgentFn != nil {
		return m.configureAgentFn(ctx, id, cfg)
	}
	return nil
}

func (m *mockAgentManager) AgentStats(ctx context.Context, id string) (*AgentStatsInfo, error) {
	if m.agentStatsFn != nil {
		return m.agentStatsFn(ctx, id)
	}
	return nil, nil
}

// --- Test Helper ---

// setupAgentRouter 는 AgentHandler가 등록된 라우터를 생성한다.
func setupAgentRouter(mock *mockAgentManager) *api.Router {
	router := api.NewRouter()
	h := NewAgentHandler(mock, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// --- AgentHandler 생성 테스트 ---

func TestNewAgentHandler(t *testing.T) {
	mock := &mockAgentManager{}

	t.Run("with nil logger", func(t *testing.T) {
		h := NewAgentHandler(mock, nil)
		require.NotNil(t, h)
		assert.NotNil(t, h.logger)
		assert.Equal(t, mock, h.agents)
	})
}

// --- RegisterRoutes 테스트 ---

func TestAgentHandler_RegisterRoutes(t *testing.T) {
	router := setupAgentRouter(&mockAgentManager{})
	// 10개 라우트 등록 확인
	assert.Equal(t, 10, router.RouteCount())
}

// --- List 테스트 ---

func TestAgentHandler_List(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockAgentManager
		expectedCode int
		expectedLen  int
	}{
		{
			name: "성공: 빈 목록",
			url:  "/api/v1/agents",
			mock: &mockAgentManager{
				listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
					return []AgentInfo{}, 0, nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  0,
		},
		{
			name: "성공: 에이전트 목록",
			url:  "/api/v1/agents?page=1&size=10&status=active",
			mock: &mockAgentManager{
				listAgentsFn: func(_ context.Context, opts dto.ListOptions) ([]AgentInfo, int64, error) {
					assert.Equal(t, "active", opts.Status)
					return []AgentInfo{
						{ID: "a1", Name: "agent1", Type: "mqtt", Status: "active"},
						{ID: "a2", Name: "agent2", Type: "http", Status: "active"},
					}, 2, nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  2,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/agents",
			mock: &mockAgentManager{
				listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
					return nil, 0, errors.New("internal error")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[[]AgentInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Len(t, resp.Data, tt.expectedLen)
			}
		})
	}
}

// --- Get 테스트 ---

func TestAgentHandler_Get(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 에이전트 조회",
			url:  "/api/v1/agents/agent-123",
			mock: &mockAgentManager{
				getAgentFn: func(_ context.Context, id string) (*AgentInfo, error) {
					assert.Equal(t, "agent-123", id)
					return &AgentInfo{ID: "agent-123", Name: "test-agent", Type: "mqtt", Status: "active"}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/agents/not-found",
			mock: &mockAgentManager{
				getAgentFn: func(_ context.Context, _ string) (*AgentInfo, error) {
					return nil, errors.New("not found")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[*AgentInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "agent-123", resp.Data.ID)
				assert.Equal(t, "test-agent", resp.Data.Name)
				assert.Equal(t, "mqtt", resp.Data.Type)
			}
		})
	}
}

// --- Create 테스트 ---

func TestAgentHandler_Create(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 에이전트 생성",
			body: `{"name":"new-agent","type":"mqtt","config":{"host":"localhost"}}`,
			mock: &mockAgentManager{
				createAgentFn: func(_ context.Context, req *dto.AgentCreateRequest) (*AgentInfo, error) {
					assert.Equal(t, "new-agent", req.Name)
					assert.Equal(t, "mqtt", req.Type)
					return &AgentInfo{ID: "new-id", Name: "new-agent", Type: "mqtt", Status: "created"}, nil
				},
			},
			expectedCode: http.StatusCreated,
		},
		{
			name:         "에러: 유효성 검증 실패 (이름 누락)",
			body:         `{"type":"mqtt"}`,
			mock:         &mockAgentManager{},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name:         "에러: 유효성 검증 실패 (타입 누락)",
			body:         `{"name":"test"}`,
			mock:         &mockAgentManager{},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name:         "에러: 잘못된 JSON",
			body:         `invalid json`,
			mock:         &mockAgentManager{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "에러: 도메인 에러",
			body: `{"name":"new-agent","type":"mqtt"}`,
			mock: &mockAgentManager{
				createAgentFn: func(_ context.Context, _ *dto.AgentCreateRequest) (*AgentInfo, error) {
					return nil, errors.New("create failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPost, "/api/v1/agents", strings.NewReader(tt.body))
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusCreated {
				var resp dto.APIResponse[*AgentInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "new-id", resp.Data.ID)
			}
		})
	}
}

// --- Update 테스트 ---

func TestAgentHandler_Update(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		body         string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 에이전트 업데이트",
			url:  "/api/v1/agents/agent-123",
			body: `{"name":"updated-name"}`,
			mock: &mockAgentManager{
				updateAgentFn: func(_ context.Context, id string, req *dto.AgentUpdateRequest) (*AgentInfo, error) {
					assert.Equal(t, "agent-123", id)
					require.NotNil(t, req.Name)
					assert.Equal(t, "updated-name", *req.Name)
					return &AgentInfo{ID: "agent-123", Name: "updated-name", Type: "mqtt", Status: "active"}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "에러: 잘못된 JSON",
			url:          "/api/v1/agents/agent-123",
			body:         `{invalid}`,
			mock:         &mockAgentManager{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/agents/agent-123",
			body: `{"name":"updated"}`,
			mock: &mockAgentManager{
				updateAgentFn: func(_ context.Context, _ string, _ *dto.AgentUpdateRequest) (*AgentInfo, error) {
					return nil, errors.New("update failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
			rec := doRequest(t, router, http.MethodPut, tt.url, strings.NewReader(tt.body))
			assert.Equal(t, tt.expectedCode, rec.Code)
		})
	}
}

// --- Delete 테스트 ---

func TestAgentHandler_Delete(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 에이전트 삭제",
			url:  "/api/v1/agents/agent-123",
			mock: &mockAgentManager{
				deleteAgentFn: func(_ context.Context, id string) error {
					assert.Equal(t, "agent-123", id)
					return nil
				},
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "에러: 삭제 실패",
			url:  "/api/v1/agents/agent-123",
			mock: &mockAgentManager{
				deleteAgentFn: func(_ context.Context, _ string) error {
					return errors.New("delete failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
			rec := doRequest(t, router, http.MethodDelete, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusNoContent {
				assert.Empty(t, rec.Body.String())
			}
		})
	}
}

// --- Start 테스트 ---

func TestAgentHandler_Start(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 에이전트 시작",
			url:  "/api/v1/agents/agent-123/start",
			mock: &mockAgentManager{
				startAgentFn: func(_ context.Context, id string) error {
					assert.Equal(t, "agent-123", id)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 시작 실패",
			url:  "/api/v1/agents/agent-123/start",
			mock: &mockAgentManager{
				startAgentFn: func(_ context.Context, _ string) error {
					return errors.New("start failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
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

func TestAgentHandler_Stop(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 에이전트 정지",
			url:  "/api/v1/agents/agent-123/stop",
			mock: &mockAgentManager{
				stopAgentFn: func(_ context.Context, id string) error {
					assert.Equal(t, "agent-123", id)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 정지 실패",
			url:  "/api/v1/agents/agent-123/stop",
			mock: &mockAgentManager{
				stopAgentFn: func(_ context.Context, _ string) error {
					return errors.New("stop failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
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

func TestAgentHandler_Restart(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 에이전트 재시작",
			url:  "/api/v1/agents/agent-123/restart",
			mock: &mockAgentManager{
				restartAgentFn: func(_ context.Context, id string) error {
					assert.Equal(t, "agent-123", id)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 재시작 실패",
			url:  "/api/v1/agents/agent-123/restart",
			mock: &mockAgentManager{
				restartAgentFn: func(_ context.Context, _ string) error {
					return errors.New("restart failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
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

func TestAgentHandler_Configure(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		body         string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 설정 업데이트",
			url:  "/api/v1/agents/agent-123/config",
			body: `{"config":{"host":"localhost","port":1883}}`,
			mock: &mockAgentManager{
				configureAgentFn: func(_ context.Context, id string, cfg map[string]any) error {
					assert.Equal(t, "agent-123", id)
					assert.Equal(t, "localhost", cfg["host"])
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "에러: 유효성 검증 실패 (config 누락)",
			url:          "/api/v1/agents/agent-123/config",
			body:         `{}`,
			mock:         &mockAgentManager{},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name:         "에러: 잘못된 JSON",
			url:          "/api/v1/agents/agent-123/config",
			body:         `{invalid}`,
			mock:         &mockAgentManager{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/agents/agent-123/config",
			body: `{"config":{"key":"value"}}`,
			mock: &mockAgentManager{
				configureAgentFn: func(_ context.Context, _ string, _ map[string]any) error {
					return errors.New("configure failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
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

// --- Stats 테스트 ---

func TestAgentHandler_Stats(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		mock         *mockAgentManager
		expectedCode int
	}{
		{
			name: "성공: 통계 조회",
			url:  "/api/v1/agents/agent-123/stats",
			mock: &mockAgentManager{
				agentStatsFn: func(_ context.Context, id string) (*AgentStatsInfo, error) {
					assert.Equal(t, "agent-123", id)
					return &AgentStatsInfo{
						ID:          "agent-123",
						Status:      "active",
						Uptime:      "2h15m",
						MessagesIn:  5000,
						MessagesOut: 4500,
						ErrorCount:  10,
						Connected:   true,
					}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 통계 조회 실패",
			url:  "/api/v1/agents/agent-123/stats",
			mock: &mockAgentManager{
				agentStatsFn: func(_ context.Context, _ string) (*AgentStatsInfo, error) {
					return nil, errors.New("stats failed")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupAgentRouter(tt.mock)
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[*AgentStatsInfo]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Equal(t, "agent-123", resp.Data.ID)
				assert.Equal(t, "active", resp.Data.Status)
				assert.Equal(t, int64(5000), resp.Data.MessagesIn)
				assert.Equal(t, int64(4500), resp.Data.MessagesOut)
				assert.True(t, resp.Data.Connected)
			}
		})
	}
}
