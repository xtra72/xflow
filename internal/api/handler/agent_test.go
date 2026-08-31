package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/agent"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
)

// --- Mock AgentManager ---

type mockAgentManager struct {
	listAgentsFn     func(ctx context.Context, opts dto.ListOptions) ([]AgentInfo, int64, error)
	getAgentFn       func(ctx context.Context, id string, detail string) (*AgentInfo, error)
	createAgentFn    func(ctx context.Context, req *dto.AgentCreateRequest) (*AgentInfo, error)
	updateAgentFn    func(ctx context.Context, id string, req *dto.AgentUpdateRequest) (*AgentInfo, error)
	deleteAgentFn    func(ctx context.Context, id string) error
	startAgentFn     func(ctx context.Context, id string) error
	stopAgentFn      func(ctx context.Context, id string) error
	restartAgentFn   func(ctx context.Context, id string) error
	configureAgentFn func(ctx context.Context, id string, cfg map[string]any) error
	agentStatsFn     func(ctx context.Context, id string) (*AgentStatsInfo, error)
	execAgentFn      func(ctx context.Context, id string, data []byte) (json.RawMessage, error)
	enableAgentFn    func(ctx context.Context, id string) (*AgentInfo, error)
	disableAgentFn   func(ctx context.Context, id string) (*AgentInfo, error)
}

func (m *mockAgentManager) ListAgents(ctx context.Context, opts dto.ListOptions) ([]AgentInfo, int64, error) {
	if m.listAgentsFn != nil {
		return m.listAgentsFn(ctx, opts)
	}
	return nil, 0, nil
}

func (m *mockAgentManager) GetAgent(ctx context.Context, id string, detail string) (*AgentInfo, error) {
	if m.getAgentFn != nil {
		return m.getAgentFn(ctx, id, detail)
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

func (m *mockAgentManager) ExecAgent(ctx context.Context, id string, data []byte) (json.RawMessage, error) {
	if m.execAgentFn != nil {
		return m.execAgentFn(ctx, id, data)
	}
	return nil, nil
}

func (m *mockAgentManager) EnableAgent(ctx context.Context, id string) (*AgentInfo, error) {
	if m.enableAgentFn != nil {
		return m.enableAgentFn(ctx, id)
	}
	return nil, nil
}

func (m *mockAgentManager) DisableAgent(ctx context.Context, id string) (*AgentInfo, error) {
	if m.disableAgentFn != nil {
		return m.disableAgentFn(ctx, id)
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
	// 16개 라우트 등록 확인 (기존 13 + Enable + Disable: SPEC-AGENT-005, + Query)
	assert.Equal(t, 16, router.RouteCount())
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
						{ID: "a1", Name: "agent1", Type: "mqtt-client", Status: "active"},
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
				getAgentFn: func(_ context.Context, id string, _ string) (*AgentInfo, error) {
					assert.Equal(t, "agent-123", id)
					return &AgentInfo{ID: "agent-123", Name: "test-agent", Type: "mqtt-client", Status: "active"}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 도메인 에러",
			url:  "/api/v1/agents/not-found",
			mock: &mockAgentManager{
				getAgentFn: func(_ context.Context, _ string, _ string) (*AgentInfo, error) {
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
				assert.Equal(t, "mqtt-client", resp.Data.Type)
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
			body: `{"name":"new-agent","type":"mqtt-client","config":{"host":"localhost"}}`,
			mock: &mockAgentManager{
				createAgentFn: func(_ context.Context, req *dto.AgentCreateRequest) (*AgentInfo, error) {
					assert.Equal(t, "new-agent", req.Name)
					assert.Equal(t, "mqtt-client", req.Type)
					return &AgentInfo{ID: "new-id", Name: "new-agent", Type: "mqtt-client", Status: "created"}, nil
				},
			},
			expectedCode: http.StatusCreated,
		},
		{
			name:         "에러: 유효성 검증 실패 (이름 누락)",
			body:         `{"type":"mqtt-client"}`,
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
			body: `{"name":"new-agent","type":"mqtt-client"}`,
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
				getAgentFn: func(_ context.Context, id string, _ string) (*AgentInfo, error) {
					return &AgentInfo{ID: "agent-123", Name: "old-name", Type: "mqtt-client", Status: "active"}, nil
				},
				updateAgentFn: func(_ context.Context, id string, req *dto.AgentUpdateRequest) (*AgentInfo, error) {
					assert.Equal(t, "agent-123", id)
					require.NotNil(t, req.Name)
					assert.Equal(t, "updated-name", *req.Name)
					return &AgentInfo{ID: "agent-123", Name: "updated-name", Type: "mqtt-client", Status: "active"}, nil
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
				getAgentFn: func(_ context.Context, _ string, _ string) (*AgentInfo, error) {
					return &AgentInfo{ID: "agent-123", Name: "old-name"}, nil
				},
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

// --- Export 테스트 ---

func TestAgentHandler_Export(t *testing.T) {
	mock := &mockAgentManager{
		getAgentFn: func(_ context.Context, id string, _ string) (*AgentInfo, error) {
			assert.Equal(t, "agent-01", id)
			return &AgentInfo{
				ID:     "agent-01",
				Name:   "test-agent",
				Type:   "mqtt-client",
				Status: "active",
				Config: map[string]any{"host": "localhost", "port": 1883},
			}, nil
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/agents/agent-01/export", nil)

	// HTTP 200 확인
	assert.Equal(t, http.StatusOK, rec.Code)

	// 엔벨로프 응답 파싱
	var envelope map[string]any
	err := json.NewDecoder(rec.Body).Decode(&envelope)
	require.NoError(t, err, "응답이 유효한 JSON 이어야 합니다")
	assert.Equal(t, true, envelope["success"], "success 필드가 true 여야 합니다")

	result, ok := envelope["data"].(map[string]any)
	require.True(t, ok, "data 필드가 객체여야 합니다")

	// 에이전트 데이터 포함 확인
	assert.Equal(t, "test-agent", result["name"],
		"name 필드가 포함되어야 합니다")
	assert.Equal(t, "mqtt-client", result["type"],
		"type 필드가 포함되어야 합니다")
	assert.NotNil(t, result["config"],
		"config 필드가 포함되어야 합니다")

	// 런타임 필드(id, status) 가 제거되었는지 확인
	_, hasID := result["id"]
	_, hasStatus := result["status"]
	assert.False(t, hasID, "런타임 필드 id 가 제거되어야 합니다")
	assert.False(t, hasStatus, "런타임 필드 status 가 제거되어야 합니다")
}

// TestAgentHandler_Export_NotFound - 존재하지 않는 에이전트 내보내기
func TestAgentHandler_Export_NotFound(t *testing.T) {
	mock := &mockAgentManager{
		getAgentFn: func(_ context.Context, _ string, _ string) (*AgentInfo, error) {
			return nil, fmt.Errorf("get agent: %w", agent.ErrAgentNotFound)
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/agents/nonexistent/export", nil)

	// HTTP 404 확인
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// --- ExportAll 테스트 ---

func TestAgentHandler_ExportAll(t *testing.T) {
	mock := &mockAgentManager{
		listAgentsFn: func(_ context.Context, _ dto.ListOptions) ([]AgentInfo, int64, error) {
			return []AgentInfo{
				{
					ID:     "a1",
					Name:   "agent-1",
					Type:   "mqtt-client",
					Status: "active",
					Config: map[string]any{"host": "host-1"},
				},
				{
					ID:     "a2",
					Name:   "agent-2",
					Type:   "http",
					Status: "stopped",
					Config: nil,
				},
			}, 2, nil
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/agents/export", nil)

	// HTTP 200 확인
	assert.Equal(t, http.StatusOK, rec.Code)

	// 엔벨로프 응답 파싱
	var envelope map[string]any
	err := json.NewDecoder(rec.Body).Decode(&envelope)
	require.NoError(t, err, "응답이 유효한 JSON 이어야 합니다")
	assert.Equal(t, true, envelope["success"], "success 필드가 true 여야 합니다")

	dataRaw, ok := envelope["data"].([]any)
	require.True(t, ok, "data 필드가 배열이어야 합니다")
	assert.Len(t, dataRaw, 2, "2개의 에이전트가 반환되어야 합니다")

	// []any → []map[string]any 변환
	result := make([]map[string]any, len(dataRaw))
	for i, raw := range dataRaw {
		result[i], ok = raw.(map[string]any)
		require.True(t, ok, "요소 %d: map 타입이어야 합니다", i)
	}

	// 각 요소에 name, type 이 있고 id, status 가 없는지 확인
	for i, item := range result {
		assert.NotEmpty(t, item["name"],
			"요소 %d: name 필드가 포함되어야 합니다", i)
		assert.NotEmpty(t, item["type"],
			"요소 %d: type 필드가 포함되어야 합니다", i)

		_, hasID := item["id"]
		_, hasStatus := item["status"]
		assert.False(t, hasID,
			"요소 %d: 런타임 필드 id 가 제거되어야 합니다", i)
		assert.False(t, hasStatus,
			"요소 %d: 런타임 필드 status 가 제거되어야 합니다", i)
	}

	// 첫 번째 에이전트에 config 가 있는지 확인
	assert.NotNil(t, result[0]["config"],
		"config 이 있는 에이전트는 config 필드를 포함해야 합니다")

	// 두 번째 에이전트에 config 가 없는지 확인 (nil Config 는 생략)
	_, hasConfig := result[1]["config"]
	assert.False(t, hasConfig,
		"config 이 nil 인 에이전트는 config 필드를 포함하지 않아야 합니다")
}

// --- SPEC-AGENT-005 Phase 3: Enable/Disable 핸들러 테스트 ---

// TestAgentHandler_Enable_Success_200 은 POST /agents/{id}/enable 이 성공 시
// 200 OK 와 enabled=true 인 AgentInfo 를 반환하는지 검증한다.
func TestAgentHandler_Enable_Success_200(t *testing.T) {
	mock := &mockAgentManager{
		enableAgentFn: func(_ context.Context, id string) (*AgentInfo, error) {
			assert.Equal(t, "agent-abc", id)
			return &AgentInfo{
				ID:      "agent-abc",
				Name:    "test-agent",
				Type:    "mqtt-client",
				Status:  "running",
				Enabled: true,
			}, nil
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/agents/agent-abc/enable", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	// JSON 본문에 "enabled":true 문자열이 포함되어야 한다 (decode 전에 검사)
	bodyStr := rec.Body.String()
	assert.Contains(t, bodyStr, `"enabled":true`)

	var resp dto.APIResponse[*AgentInfo]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "agent-abc", resp.Data.ID)
	assert.True(t, resp.Data.Enabled, "Enable 성공 시 응답 AgentInfo.Enabled 는 true 여야 함")
}

// TestAgentHandler_Disable_Success_200 은 POST /agents/{id}/disable 이 성공 시
// 200 OK 와 enabled=false 인 AgentInfo 를 반환하는지 검증한다.
func TestAgentHandler_Disable_Success_200(t *testing.T) {
	mock := &mockAgentManager{
		disableAgentFn: func(_ context.Context, id string) (*AgentInfo, error) {
			assert.Equal(t, "agent-abc", id)
			return &AgentInfo{
				ID:      "agent-abc",
				Name:    "test-agent",
				Type:    "mqtt-client",
				Status:  "running", // Disable 은 실행 상태에 영향을 주지 않는다 (R3.7)
				Enabled: false,
			}, nil
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/agents/agent-abc/disable", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	// JSON 본문에 "enabled":false 문자열이 포함되어야 한다 (decode 전에 검사)
	bodyStr := rec.Body.String()
	assert.Contains(t, bodyStr, `"enabled":false`)

	var resp dto.APIResponse[*AgentInfo]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "agent-abc", resp.Data.ID)
	assert.False(t, resp.Data.Enabled, "Disable 성공 시 응답 AgentInfo.Enabled 는 false 여야 함")
}

// TestAgentHandler_Enable_NotFound_404 는 존재하지 않는 에이전트에 대한 Enable 호출 시
// 404 Not Found 를 반환하는지 검증한다.
func TestAgentHandler_Enable_NotFound_404(t *testing.T) {
	mock := &mockAgentManager{
		enableAgentFn: func(_ context.Context, _ string) (*AgentInfo, error) {
			return nil, fmt.Errorf("enable agent: %w", agent.ErrAgentNotFound)
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/agents/nonexistent/enable", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestAgentHandler_Disable_NotFound_404 는 존재하지 않는 에이전트에 대한 Disable 호출 시
// 404 Not Found 를 반환하는지 검증한다.
func TestAgentHandler_Disable_NotFound_404(t *testing.T) {
	mock := &mockAgentManager{
		disableAgentFn: func(_ context.Context, _ string) (*AgentInfo, error) {
			return nil, fmt.Errorf("disable agent: %w", agent.ErrAgentNotFound)
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodPost, "/api/v1/agents/nonexistent/disable", nil)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestAgentHandler_AgentInfoSerialization_IncludesEnabledField 는 GET /agents/{id}
// 응답 JSON 에 enabled 필드가 항상 포함되는지 검증한다.
func TestAgentHandler_AgentInfoSerialization_IncludesEnabledField(t *testing.T) {
	mock := &mockAgentManager{
		getAgentFn: func(_ context.Context, _ string, _ string) (*AgentInfo, error) {
			return &AgentInfo{
				ID:      "agent-abc",
				Name:    "test-agent",
				Type:    "mqtt-client",
				Status:  "running",
				Enabled: true,
			}, nil
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/agents/agent-abc", nil)

	assert.Equal(t, http.StatusOK, rec.Code)

	// JSON 본문에 "enabled" 필드가 포함되어야 한다 (omitempty 없이 항상 출력)
	body := rec.Body.String()
	assert.Contains(t, body, `"enabled"`, "AgentInfo 응답에 enabled 필드가 포함되어야 함")
}

// TestAgentHandler_AgentInfoSerialization_EnabledFalseIncluded 는 Enabled=false 일 때도
// enabled 필드가 JSON 에 포함되는지 검증한다 (omitempty 없음).
func TestAgentHandler_AgentInfoSerialization_EnabledFalseIncluded(t *testing.T) {
	mock := &mockAgentManager{
		getAgentFn: func(_ context.Context, _ string, _ string) (*AgentInfo, error) {
			return &AgentInfo{
				ID:      "agent-abc",
				Name:    "test-agent",
				Type:    "mqtt-client",
				Status:  "running",
				Enabled: false,
			}, nil
		},
	}

	router := setupAgentRouter(mock)
	rec := doRequest(t, router, http.MethodGet, "/api/v1/agents/agent-abc", nil)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), `"enabled":false`, "Enabled=false 일 때도 필드가 포함되어야 함")
}

// --- Query 테스트 (읽기 전용 커맨드 화이트리스트) ---

// queryReadCommands 는 대시보드 패널이 조회에 사용하는 읽기 전용 커맨드 전체이다.
// queryReadOnlyCommands 화이트리스트와 1:1 로 대응해야 한다.
var queryReadCommands = []string{
	"list_devices", "list_clients", "list_stations", "list_lines", "list_groups",
	"list_gateways", "list_connections", "list_models", "get_status", "get_map", "get_device_status",
	// get_history 는 sysmetrics 에이전트의 표본 이력 조회이다 — 메모리 버퍼를 읽기만 한다.
	"get_history",
}

// queryWriteCommands 는 상태를 변경하므로 /query 로는 절대 통과해서는 안 되는 커맨드이다.
var queryWriteCommands = []string{
	"add_device", "add_group", "add_line", "add_place", "add_station",
	"remove_device", "remove_group", "remove_line", "remove_place", "remove_station",
	"set_device", "set_fan_speed", "set_group", "set_multiple", "set_power",
}

// TestAgentHandler_QueryAllowlistIsExplicit 는 화이트리스트의 내용을 고정한다.
//
// 항목 추가는 권한 결정이므로 조용히 늘어나서는 안 된다. 이 단언이 깨진다는 것은
// 누군가 /query 로 열리는 커맨드 집합을 바꿨다는 뜻이며, 리뷰 대상이라는 신호이다.
func TestAgentHandler_QueryAllowlistIsExplicit(t *testing.T) {
	got := make([]string, 0, len(queryReadOnlyCommands))
	for cmd := range queryReadOnlyCommands {
		got = append(got, cmd)
	}
	assert.ElementsMatch(t, queryReadCommands, got)
}

// TestAgentHandler_QueryAcceptsEveryReadCommand 는 읽기 커맨드 12종이 모두 에이전트에
// 도달함을 검증한다.
func TestAgentHandler_QueryAcceptsEveryReadCommand(t *testing.T) {
	for _, cmd := range queryReadCommands {
		t.Run(cmd, func(t *testing.T) {
			var gotCommand string
			mock := &mockAgentManager{
				execAgentFn: func(_ context.Context, id string, data []byte) (json.RawMessage, error) {
					assert.Equal(t, "agent-1", id)
					var req dto.AgentExecRequest
					require.NoError(t, json.Unmarshal(data, &req))
					gotCommand = req.Command
					return json.RawMessage(`{"items":[]}`), nil
				},
			}
			router := setupAgentRouter(mock)

			rec := doRequest(t, router, http.MethodPost, "/api/v1/agents/agent-1/query",
				strings.NewReader(`{"command":"`+cmd+`"}`))

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, cmd, gotCommand, "커맨드가 에이전트에 전달되지 않았다")
		})
	}
}

// TestAgentHandler_QueryRejectsUnlistedCommand 는 미등재 커맨드가 400 으로 거부되고
// 에이전트가 호출되지 않음을 검증한다.
//
// 상태 코드만 확인하면 "에이전트가 실행된 뒤 응답만 400 으로 덮인" 경우를 놓친다.
// 따라서 ExecAgent 호출 자체가 0 회임을 단언한다.
func TestAgentHandler_QueryRejectsUnlistedCommand(t *testing.T) {
	unlisted := append([]string{}, queryWriteCommands...)
	unlisted = append(unlisted,
		"",                  // 빈 커맨드
		"list_dir",          // 접두사만 읽기처럼 보이는 미등재 커맨드
		"list_peers",        // 위와 동일
		"totally_bogus_cmd", // 알 수 없는 커맨드
		"LIST_DEVICES",      // 대소문자 우회 시도
		"list_devices ",     // 공백 패딩 우회 시도
	)

	for _, cmd := range unlisted {
		t.Run("거부: "+cmd, func(t *testing.T) {
			execCalls := 0
			mock := &mockAgentManager{
				execAgentFn: func(_ context.Context, _ string, _ []byte) (json.RawMessage, error) {
					execCalls++
					return json.RawMessage(`{"ok":true}`), nil
				},
			}
			router := setupAgentRouter(mock)

			body, err := json.Marshal(dto.AgentExecRequest{Command: cmd})
			require.NoError(t, err)
			rec := doRequest(t, router, http.MethodPost, "/api/v1/agents/agent-1/query",
				strings.NewReader(string(body)))

			assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Zero(t, execCalls, "거부된 커맨드가 에이전트까지 전달되었다")
		})
	}
}

// TestAgentHandler_ExecStillAcceptsWriteCommands 는 /exec 의 수용 커맨드 집합이
// 좁아지지 않았음을 검증한다 (화이트리스트가 /exec 에 새어 들어가지 않는다).
func TestAgentHandler_ExecStillAcceptsWriteCommands(t *testing.T) {
	for _, cmd := range queryWriteCommands {
		t.Run(cmd, func(t *testing.T) {
			called := false
			mock := &mockAgentManager{
				execAgentFn: func(_ context.Context, _ string, _ []byte) (json.RawMessage, error) {
					called = true
					return json.RawMessage(`{"ok":true}`), nil
				},
			}
			router := setupAgentRouter(mock)

			rec := doRequest(t, router, http.MethodPost, "/api/v1/agents/agent-1/exec",
				strings.NewReader(`{"command":"`+cmd+`"}`))

			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.True(t, called, "/exec 이 쓰기 커맨드를 에이전트로 전달하지 않았다")
		})
	}
}

// TestAgentHandler_QueryAndExecShareResponseShape 는 동일한 읽기 커맨드에 대해 두
// 엔드포인트의 응답이 바이트 단위로 같음을 검증한다.
//
// 프론트가 읽기 커맨드를 /exec 에서 /query 로 옮길 때 응답 파싱을 바꿀 필요가
// 없어야 한다는 계약이다.
func TestAgentHandler_QueryAndExecShareResponseShape(t *testing.T) {
	newMock := func(seen *[]byte) *mockAgentManager {
		return &mockAgentManager{
			execAgentFn: func(_ context.Context, _ string, data []byte) (json.RawMessage, error) {
				*seen = append([]byte(nil), data...)
				return json.RawMessage(`{"stations":[{"id":"s1"}],"total":1}`), nil
			},
		}
	}

	body := `{"command":"list_stations","params":{"line":"L1"}}`

	var execPayload []byte
	execRec := doRequest(t, setupAgentRouter(newMock(&execPayload)),
		http.MethodPost, "/api/v1/agents/agent-1/exec", strings.NewReader(body))

	var queryPayload []byte
	queryRec := doRequest(t, setupAgentRouter(newMock(&queryPayload)),
		http.MethodPost, "/api/v1/agents/agent-1/query", strings.NewReader(body))

	// 에이전트에 전달되는 요청 바이트가 같다 (동일 DTO · 동일 직렬화 경로).
	assert.Equal(t, string(execPayload), string(queryPayload))

	// 응답 상태·헤더·본문이 모두 같다 (동일 엔벨로프).
	assert.Equal(t, execRec.Code, queryRec.Code)
	assert.Equal(t, execRec.Header().Get("Content-Type"), queryRec.Header().Get("Content-Type"))
	assert.Equal(t, execRec.Body.String(), queryRec.Body.String())
	assert.Contains(t, queryRec.Body.String(), `"stations"`)
}
