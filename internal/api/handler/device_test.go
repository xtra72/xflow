package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/device"
)

// --- Mock DeviceRegistry ---

type mockDeviceRegistry struct {
	listFn        func(filter device.DeviceFilter) []device.Device
	getFn         func(id string) (device.Device, error)
	countFn       func() int
	executeFn     func(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error)
	setMetadataFn func(id string, metadata device.DeviceMetadata) error
	getMetadataFn func(id string) (device.DeviceMetadata, error)
}

func (m *mockDeviceRegistry) List(filter device.DeviceFilter) []device.Device {
	if m.listFn != nil {
		return m.listFn(filter)
	}
	return nil
}

func (m *mockDeviceRegistry) Get(id string) (device.Device, error) {
	if m.getFn != nil {
		return m.getFn(id)
	}
	return nil, device.ErrDeviceNotFound
}

func (m *mockDeviceRegistry) Count() int {
	if m.countFn != nil {
		return m.countFn()
	}
	return 0
}

func (m *mockDeviceRegistry) Execute(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, id, command, params)
	}
	return nil, nil
}

func (m *mockDeviceRegistry) SetMetadata(id string, metadata device.DeviceMetadata) error {
	if m.setMetadataFn != nil {
		return m.setMetadataFn(id, metadata)
	}
	return nil
}

func (m *mockDeviceRegistry) GetMetadata(id string) (device.DeviceMetadata, error) {
	if m.getMetadataFn != nil {
		return m.getMetadataFn(id)
	}
	return device.DeviceMetadata{}, nil
}

// --- Mock MetadataRepository ---

type mockMetadataRepo struct {
	saveFn   func(ctx context.Context, deviceID string, metadata device.DeviceMetadata) error
	getFn    func(ctx context.Context, deviceID string) (device.DeviceMetadata, error)
	deleteFn func(ctx context.Context, deviceID string) error
}

func (m *mockMetadataRepo) Save(ctx context.Context, deviceID string, metadata device.DeviceMetadata) error {
	if m.saveFn != nil {
		return m.saveFn(ctx, deviceID, metadata)
	}
	return nil
}

func (m *mockMetadataRepo) Get(ctx context.Context, deviceID string) (device.DeviceMetadata, error) {
	if m.getFn != nil {
		return m.getFn(ctx, deviceID)
	}
	return device.DeviceMetadata{}, nil
}

func (m *mockMetadataRepo) Delete(ctx context.Context, deviceID string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, deviceID)
	}
	return nil
}

// --- Mock Device ---

type mockDevice struct {
	id           string
	name         string
	deviceType   device.DeviceType
	protocol     string
	agentName    string
	online       bool
	lastSeen     time.Time
	state        device.DeviceState
	metadata     device.DeviceMetadata
	capabilities []string
}

func (d *mockDevice) ID() string                  { return d.id }
func (d *mockDevice) Name() string                { return d.name }
func (d *mockDevice) Type() device.DeviceType     { return d.deviceType }
func (d *mockDevice) Protocol() string            { return d.protocol }
func (d *mockDevice) AgentName() string           { return d.agentName }
func (d *mockDevice) Online() bool                { return d.online }
func (d *mockDevice) LastSeen() time.Time         { return d.lastSeen }
func (d *mockDevice) State() device.DeviceState   { return d.state }
func (d *mockDevice) Metadata() device.DeviceMetadata { return d.metadata }
func (d *mockDevice) Source() string              { return "auto" }
func (d *mockDevice) Capabilities() []string      { return d.capabilities }

// --- Mock ControllableDevice ---

type mockControllableDevice struct {
	mockDevice
	executeFn func(ctx context.Context, command string, params map[string]any) (map[string]any, error)
	commands  []device.CommandSpec
}

func (d *mockControllableDevice) Execute(ctx context.Context, command string, params map[string]any) (map[string]any, error) {
	if d.executeFn != nil {
		return d.executeFn(ctx, command, params)
	}
	return nil, nil
}

func (d *mockControllableDevice) Commands() []device.CommandSpec {
	return d.commands
}

// --- Test Helper ---

// setupDeviceRouter 는 DeviceHandler가 등록된 라우터를 생성한다.
func setupDeviceRouter(reg *mockDeviceRegistry, repo *mockMetadataRepo) *api.Router {
	router := api.NewRouter()
	h := NewDeviceHandler(reg, repo, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// --- DeviceHandler 생성 테스트 ---

func TestNewDeviceHandler(t *testing.T) {
	reg := &mockDeviceRegistry{}
	repo := &mockMetadataRepo{}

	t.Run("with nil logger", func(t *testing.T) {
		h := NewDeviceHandler(reg, repo, nil)
		require.NotNil(t, h)
		assert.NotNil(t, h.logger)
		assert.Equal(t, reg, h.registry)
		assert.Equal(t, repo, h.metadataRepo)
	})
}

// --- RegisterRoutes 테스트 ---

func TestDeviceHandler_RegisterRoutes(t *testing.T) {
	router := setupDeviceRouter(&mockDeviceRegistry{}, &mockMetadataRepo{})
	// 5개 라우트: GET /devices, GET /devices/{id}, POST /devices/{id}/execute,
	// PUT /devices/{id}/metadata, DELETE /devices/{id}/metadata
	assert.Equal(t, 5, router.RouteCount())
}

// --- List 테스트 ---

func TestDeviceHandler_List(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		url          string
		registry     *mockDeviceRegistry
		expectedCode int
		expectedLen  int
	}{
		{
			name: "성공: 빈 목록",
			url:  "/api/v1/devices",
			registry: &mockDeviceRegistry{
				listFn: func(_ device.DeviceFilter) []device.Device {
					return nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  0,
		},
		{
			name: "성공: 디바이스 목록",
			url:  "/api/v1/devices",
			registry: &mockDeviceRegistry{
				listFn: func(_ device.DeviceFilter) []device.Device {
					return []device.Device{
						&mockDevice{id: "agent1:dev1", name: "Device 1", protocol: "nasa", online: true, lastSeen: now},
						&mockDevice{id: "agent1:dev2", name: "Device 2", protocol: "modbus", online: false, lastSeen: now},
					}
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  2,
		},
		{
			name: "성공: protocol 필터",
			url:  "/api/v1/devices?protocol=nasa",
			registry: &mockDeviceRegistry{
				listFn: func(filter device.DeviceFilter) []device.Device {
					assert.Equal(t, "nasa", filter.Protocol)
					return []device.Device{
						&mockDevice{id: "agent1:dev1", name: "Device 1", protocol: "nasa", online: true, lastSeen: now},
					}
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  1,
		},
		{
			name: "성공: online 필터",
			url:  "/api/v1/devices?online=true",
			registry: &mockDeviceRegistry{
				listFn: func(filter device.DeviceFilter) []device.Device {
					require.NotNil(t, filter.Online)
					assert.True(t, *filter.Online)
					return []device.Device{
						&mockDevice{id: "agent1:dev1", name: "Device 1", online: true, lastSeen: now},
					}
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  1,
		},
		{
			name: "성공: tags 필터",
			url:  "/api/v1/devices?tags=hvac,indoor",
			registry: &mockDeviceRegistry{
				listFn: func(filter device.DeviceFilter) []device.Device {
					assert.Equal(t, []string{"hvac", "indoor"}, filter.Tags)
					return nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  0,
		},
		{
			name: "성공: 복합 필터",
			url:  "/api/v1/devices?protocol=nasa&agent=agent1&type=indoor&group=1f",
			registry: &mockDeviceRegistry{
				listFn: func(filter device.DeviceFilter) []device.Device {
					assert.Equal(t, "nasa", filter.Protocol)
					assert.Equal(t, "agent1", filter.AgentName)
					assert.Equal(t, "indoor", filter.Type)
					assert.Equal(t, "1f", filter.Group)
					return nil
				},
			},
			expectedCode: http.StatusOK,
			expectedLen:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupDeviceRouter(tt.registry, &mockMetadataRepo{})
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK {
				var resp dto.APIResponse[[]DeviceResponse]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				assert.Len(t, resp.Data, tt.expectedLen)
			}
		})
	}
}

// --- Get 테스트 ---

func TestDeviceHandler_Get(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name         string
		url          string
		registry     *mockDeviceRegistry
		expectedCode int
		checkResp    func(t *testing.T, resp DeviceDetailResponse)
	}{
		{
			name: "성공: 기본 디바이스 조회",
			url:  "/api/v1/devices/agent1:dev1",
			registry: &mockDeviceRegistry{
				getFn: func(id string) (device.Device, error) {
					assert.Equal(t, "agent1:dev1", id)
					return &mockDevice{
						id:         "agent1:dev1",
						name:       "Indoor Unit 1",
						deviceType: device.DeviceTypeIndoor,
						protocol:   "nasa",
						agentName:  "agent1",
						online:     true,
						lastSeen:   now,
						state: device.DeviceState{
							Online: true,
							Ready:  true,
						},
					}, nil
				},
				getMetadataFn: func(id string) (device.DeviceMetadata, error) {
					return device.DeviceMetadata{}, nil
				},
			},
			expectedCode: http.StatusOK,
			checkResp: func(t *testing.T, resp DeviceDetailResponse) {
				assert.Equal(t, "agent1:dev1", resp.ID)
				assert.Equal(t, "Indoor Unit 1", resp.Name)
				assert.Equal(t, "nasa", resp.Protocol)
				assert.True(t, resp.Online)
				assert.NotNil(t, resp.State)
				assert.Nil(t, resp.Commands)
				assert.Nil(t, resp.Metadata)
			},
		},
		{
			name: "성공: ControllableDevice 커맨드 포함",
			url:  "/api/v1/devices/agent1:dev2",
			registry: &mockDeviceRegistry{
				getFn: func(id string) (device.Device, error) {
					return &mockControllableDevice{
						mockDevice: mockDevice{
							id:         "agent1:dev2",
							name:       "HVAC Unit",
							deviceType: device.DeviceTypeIndoor,
							protocol:   "nasa",
							agentName:  "agent1",
							online:     true,
							lastSeen:   now,
							state:      device.DeviceState{Online: true},
						},
						commands: []device.CommandSpec{
							{Name: "target_temperature", Description: "Set temperature"},
						},
					}, nil
				},
				getMetadataFn: func(id string) (device.DeviceMetadata, error) {
					return device.DeviceMetadata{
						Location: "1F Lobby",
						Tags:     []string{"hvac"},
					}, nil
				},
			},
			expectedCode: http.StatusOK,
			checkResp: func(t *testing.T, resp DeviceDetailResponse) {
				assert.Equal(t, "agent1:dev2", resp.ID)
				assert.NotNil(t, resp.Commands)
				assert.Len(t, resp.Commands, 1)
				assert.Equal(t, "target_temperature", resp.Commands[0].Name)
				assert.NotNil(t, resp.Metadata)
				assert.Equal(t, "1F Lobby", resp.Metadata.Location)
			},
		},
		{
			name: "에러: 디바이스 없음",
			url:  "/api/v1/devices/not-exist",
			registry: &mockDeviceRegistry{
				getFn: func(id string) (device.Device, error) {
					return nil, device.ErrDeviceNotFound
				},
			},
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupDeviceRouter(tt.registry, &mockMetadataRepo{})
			rec := doRequest(t, router, http.MethodGet, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)

			if tt.expectedCode == http.StatusOK && tt.checkResp != nil {
				var resp dto.APIResponse[DeviceDetailResponse]
				decodeJSON(t, rec, &resp)
				assert.True(t, resp.Success)
				tt.checkResp(t, resp.Data)
			}
		})
	}
}

// --- Execute 테스트 ---

func TestDeviceHandler_Execute(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		body         string
		registry     *mockDeviceRegistry
		expectedCode int
	}{
		{
			name: "성공: 커맨드 실행",
			url:  "/api/v1/devices/agent1:dev1/execute",
			body: `{"command":"target_temperature","params":{"value":24}}`,
			registry: &mockDeviceRegistry{
				executeFn: func(_ context.Context, id string, command string, params map[string]any) (map[string]any, error) {
					assert.Equal(t, "agent1:dev1", id)
					assert.Equal(t, "target_temperature", command)
					return map[string]any{"status": "ok"}, nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name:         "에러: command 누락",
			url:          "/api/v1/devices/agent1:dev1/execute",
			body:         `{"params":{"value":24}}`,
			registry:     &mockDeviceRegistry{},
			expectedCode: http.StatusBadRequest,
		},
		{
			name: "에러: 디바이스 없음",
			url:  "/api/v1/devices/not-exist/execute",
			body: `{"command":"target_temperature"}`,
			registry: &mockDeviceRegistry{
				executeFn: func(_ context.Context, _ string, _ string, _ map[string]any) (map[string]any, error) {
					return nil, device.ErrDeviceNotFound
				},
			},
			expectedCode: http.StatusNotFound,
		},
		{
			name: "에러: 제어 불가능",
			url:  "/api/v1/devices/agent1:sensor1/execute",
			body: `{"command":"target_temperature"}`,
			registry: &mockDeviceRegistry{
				executeFn: func(_ context.Context, _ string, _ string, _ map[string]any) (map[string]any, error) {
					return nil, device.ErrNotControllable
				},
			},
			expectedCode: http.StatusUnprocessableEntity,
		},
		{
			name: "에러: 에이전트 정지",
			url:  "/api/v1/devices/agent1:dev1/execute",
			body: `{"command":"target_temperature"}`,
			registry: &mockDeviceRegistry{
				executeFn: func(_ context.Context, _ string, _ string, _ map[string]any) (map[string]any, error) {
					return nil, device.ErrAgentStopped
				},
			},
			expectedCode: http.StatusServiceUnavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupDeviceRouter(tt.registry, &mockMetadataRepo{})
			rec := doRequest(t, router, http.MethodPost, tt.url, strings.NewReader(tt.body))
			assert.Equal(t, tt.expectedCode, rec.Code)
		})
	}
}

// --- UpdateMetadata 테스트 ---

func TestDeviceHandler_UpdateMetadata(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		body         string
		registry     *mockDeviceRegistry
		repo         *mockMetadataRepo
		expectedCode int
	}{
		{
			name: "성공: 메타데이터 업데이트",
			url:  "/api/v1/devices/agent1:dev1/metadata",
			body: `{"tags":["hvac","indoor"],"location":"1F Lobby","group":"1f","labels":{"zone":"a"}}`,
			registry: &mockDeviceRegistry{
				setMetadataFn: func(id string, meta device.DeviceMetadata) error {
					assert.Equal(t, "agent1:dev1", id)
					assert.Equal(t, "1F Lobby", meta.Location)
					assert.Equal(t, []string{"hvac", "indoor"}, meta.Tags)
					return nil
				},
			},
			repo: &mockMetadataRepo{
				saveFn: func(_ context.Context, deviceID string, meta device.DeviceMetadata) error {
					assert.Equal(t, "agent1:dev1", deviceID)
					assert.Equal(t, "1F Lobby", meta.Location)
					return nil
				},
			},
			expectedCode: http.StatusOK,
		},
		{
			name: "에러: 디바이스 없음",
			url:  "/api/v1/devices/not-exist/metadata",
			body: `{"tags":["test"]}`,
			registry: &mockDeviceRegistry{
				setMetadataFn: func(_ string, _ device.DeviceMetadata) error {
					return device.ErrDeviceNotFound
				},
			},
			repo:         &mockMetadataRepo{},
			expectedCode: http.StatusNotFound,
		},
		{
			name: "에러: 영속화 실패",
			url:  "/api/v1/devices/agent1:dev1/metadata",
			body: `{"tags":["test"]}`,
			registry: &mockDeviceRegistry{
				setMetadataFn: func(_ string, _ device.DeviceMetadata) error {
					return nil
				},
			},
			repo: &mockMetadataRepo{
				saveFn: func(_ context.Context, _ string, _ device.DeviceMetadata) error {
					return errors.New("disk full")
				},
			},
			expectedCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupDeviceRouter(tt.registry, tt.repo)
			rec := doRequest(t, router, http.MethodPut, tt.url, strings.NewReader(tt.body))
			assert.Equal(t, tt.expectedCode, rec.Code)
		})
	}
}

// --- DeleteMetadata 테스트 ---

func TestDeviceHandler_DeleteMetadata(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		registry     *mockDeviceRegistry
		repo         *mockMetadataRepo
		expectedCode int
	}{
		{
			name: "성공: 메타데이터 삭제",
			url:  "/api/v1/devices/agent1:dev1/metadata",
			registry: &mockDeviceRegistry{
				setMetadataFn: func(id string, meta device.DeviceMetadata) error {
					assert.Equal(t, "agent1:dev1", id)
					assert.Empty(t, meta.Tags)
					assert.Empty(t, meta.Location)
					return nil
				},
			},
			repo: &mockMetadataRepo{
				deleteFn: func(_ context.Context, deviceID string) error {
					assert.Equal(t, "agent1:dev1", deviceID)
					return nil
				},
			},
			expectedCode: http.StatusNoContent,
		},
		{
			name: "에러: 디바이스 없음 (SetMetadata 실패)",
			url:  "/api/v1/devices/not-exist/metadata",
			registry: &mockDeviceRegistry{
				setMetadataFn: func(_ string, _ device.DeviceMetadata) error {
					return device.ErrDeviceNotFound
				},
			},
			repo:         &mockMetadataRepo{},
			expectedCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := setupDeviceRouter(tt.registry, tt.repo)
			rec := doRequest(t, router, http.MethodDelete, tt.url, nil)
			assert.Equal(t, tt.expectedCode, rec.Code)
		})
	}
}
