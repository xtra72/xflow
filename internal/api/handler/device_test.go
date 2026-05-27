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

// mockDeviceRegistry 는 핸들러 테스트용 DeviceRegistry mock 이다.
//
// SPEC-DEVICE-IDENTITY-001 Phase B (B-T1, B-T4): GetByUID / GetByAgentName /
// ResolveDevice 함수 포인터가 추가되었다. 미설정 시 getFn 으로 fallback 하여
// Phase A 시점의 테스트 (composite id 만 사용) 가 회귀 없이 동작한다.
type mockDeviceRegistry struct {
	listFn           func(filter device.DeviceFilter) []device.Device
	getFn            func(id string) (device.Device, error)
	getByUIDFn       func(uid string) (device.Device, error)
	getByAgentNameFn func(agent, name string) (device.Device, error)
	resolveDeviceFn  func(ref string) (device.Device, device.DeviceRefKind, error)
	countFn          func() int
	executeFn        func(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error)
	setMetadataFn    func(id string, metadata device.DeviceMetadata) error
	getMetadataFn    func(id string) (device.DeviceMetadata, error)
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

func (m *mockDeviceRegistry) GetByUID(uid string) (device.Device, error) {
	if m.getByUIDFn != nil {
		return m.getByUIDFn(uid)
	}
	return nil, device.ErrDeviceNotFound
}

func (m *mockDeviceRegistry) GetByAgentName(agentName, name string) (device.Device, error) {
	if m.getByAgentNameFn != nil {
		return m.getByAgentNameFn(agentName, name)
	}
	return nil, device.ErrDeviceNotFound
}

// ResolveDevice 는 ref 형식을 분류하여 적절한 lookup 으로 dispatch 한다.
// resolveDeviceFn 이 설정되어 있으면 그것을 사용하고, 아니면 device 패키지의
// ClassifyDeviceRef 와 동일한 dispatch 로직을 mock 인터페이스 위에서 재현한다.
func (m *mockDeviceRegistry) ResolveDevice(ref string) (device.Device, device.DeviceRefKind, error) {
	if m.resolveDeviceFn != nil {
		return m.resolveDeviceFn(ref)
	}
	kind := device.ClassifyDeviceRef(ref)
	switch kind {
	case device.DeviceRefUUID:
		dev, err := m.GetByUID(ref)
		return dev, kind, err
	case device.DeviceRefAgentName:
		agent, name, ok := device.SplitAgentName(ref)
		if !ok {
			return nil, kind, device.ErrDeviceNotFound
		}
		dev, err := m.GetByAgentName(agent, name)
		return dev, kind, err
	case device.DeviceRefComposite:
		dev, err := m.Get(ref)
		return dev, kind, err
	default:
		return nil, kind, device.ErrDeviceNotFound
	}
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
	uid          string // SPEC-DEVICE-IDENTITY-001 Phase A: UUID v4 (may be empty)
	deviceType   device.DeviceType
	protocol     string
	agentName    string
	online       bool
	lastSeen     time.Time
	state        device.DeviceState
	metadata     device.DeviceMetadata
	capabilities []string
}

func (d *mockDevice) ID() string                      { return d.id }
func (d *mockDevice) UID() string                     { return d.uid }
func (d *mockDevice) Name() string                    { return d.name }
func (d *mockDevice) Type() device.DeviceType         { return d.deviceType }
func (d *mockDevice) Protocol() string                { return d.protocol }
func (d *mockDevice) AgentName() string               { return d.agentName }
func (d *mockDevice) Online() bool                    { return d.online }
func (d *mockDevice) LastSeen() time.Time             { return d.lastSeen }
func (d *mockDevice) State() device.DeviceState       { return d.state }
func (d *mockDevice) Metadata() device.DeviceMetadata { return d.metadata }
func (d *mockDevice) Source() string                  { return "auto" }
func (d *mockDevice) Capabilities() []string          { return d.capabilities }

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
	// 7개 라우트 (SPEC-DEVICE-IDENTITY-001 Phase B § B-T4, B-T5 추가):
	//   GET    /devices
	//   GET    /devices:resolve                  (B-T5 신규)
	//   GET    /devices/{ref}
	//   GET    /devices/{agent}/{name}           (B-T4 신규)
	//   POST   /devices/{id}/execute
	//   PUT    /devices/{id}/metadata
	//   DELETE /devices/{id}/metadata
	assert.Equal(t, 7, router.RouteCount())
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
			// Phase D (D-T12): URL 은 agent/name 형식 (별도 라우트로 dispatch).
			name: "성공: agent/name 형식 디바이스 조회",
			url:  "/api/v1/devices/agent1/Indoor%20Unit%201",
			registry: &mockDeviceRegistry{
				getByAgentNameFn: func(agent, name string) (device.Device, error) {
					assert.Equal(t, "agent1", agent)
					assert.Equal(t, "Indoor Unit 1", name)
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
				assert.Equal(t, "Indoor Unit 1", resp.Name)
				assert.Equal(t, "nasa", resp.Protocol)
				assert.True(t, resp.Online)
				assert.NotNil(t, resp.State)
				assert.Nil(t, resp.Commands)
				assert.Nil(t, resp.Metadata)
			},
		},
		{
			// Phase D (D-T12): URL 은 agent/name 형식.
			name: "성공: ControllableDevice 커맨드 포함 (agent/name)",
			url:  "/api/v1/devices/agent1/HVAC%20Unit",
			registry: &mockDeviceRegistry{
				getByAgentNameFn: func(agent, name string) (device.Device, error) {
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

// ---------------------------------------------------------------------------
// SPEC-DEVICE-IDENTITY-001 Phase B — B-T4, B-T5 (B-AC3, B-AC4, B-AC5)
//
// REST URL resolver 의 3 가지 dispatch (UUID / agent/name / composite alias) 와
// 잘못된 reference 의 404 처리, name 기반 resolver 엔드포인트 (B-T5) 를 검증.
// ---------------------------------------------------------------------------

// helperLgcnpDevice 는 B-T4/B-T5 테스트에서 공유되는 fixture 디바이스이다.
// Phase D (v1.0): Device.ID() 가 UUID 반환하므로 mock 의 id == uid.
func helperLgcnpDevice(now time.Time) *mockDevice {
	const uid = "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"
	return &mockDevice{
		id:         uid,
		uid:        uid,
		name:       "indoor-1",
		deviceType: device.DeviceTypeIndoor,
		protocol:   "lg_icp01",
		agentName:  "lg_hvacr01",
		online:     true,
		lastSeen:   now,
		state:      device.DeviceState{Online: true},
	}
}

// B-AC3 / B-T4: UUID URL 형식이 GetByUID 로 dispatch 되며 Deprecation 헤더가
// 부착되지 않아야 한다 (1급 식별자).
func TestDeviceHandler_Get_UUIDDispatchesToGetByUID(t *testing.T) {
	now := time.Now()
	const uid = "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"

	registry := &mockDeviceRegistry{
		getByUIDFn: func(got string) (device.Device, error) {
			assert.Equal(t, uid, got)
			return helperLgcnpDevice(now), nil
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet, "/api/v1/devices/"+uid, nil)

	require.Equal(t, http.StatusOK, rec.Code)
	// UUID 경로는 deprecated 가 아니므로 헤더가 없어야 한다.
	assert.Empty(t, rec.Header().Get("Deprecation"), "UUID 1급 경로에 Deprecation 헤더가 붙으면 안 된다")
	assert.Empty(t, rec.Header().Get("Sunset"), "UUID 1급 경로에 Sunset 헤더가 붙으면 안 된다")

	var resp dto.APIResponse[DeviceDetailResponse]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)
	assert.Equal(t, uid, resp.Data.ID, "Phase D: id 가 UUID 반환")
}

// B-AC3 / B-T4: agent/name 2 세그먼트 URL 이 GetByAgentName 으로 dispatch.
func TestDeviceHandler_GetByAgentName_TwoSegmentDispatch(t *testing.T) {
	now := time.Now()

	registry := &mockDeviceRegistry{
		getByAgentNameFn: func(agentName, name string) (device.Device, error) {
			assert.Equal(t, "lg_hvacr01", agentName)
			assert.Equal(t, "indoor-1", name)
			return helperLgcnpDevice(now), nil
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet, "/api/v1/devices/lg_hvacr01/indoor-1", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	// agent/name 은 Phase D 이후에도 유지되는 1급 reference 이므로 Deprecation 없음.
	assert.Empty(t, rec.Header().Get("Deprecation"))

	var resp dto.APIResponse[DeviceDetailResponse]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)
	assert.Equal(t, "indoor-1", resp.Data.Name)
}

// D-AC10 / D-T12: composite URL 형식은 Phase D 부터 alias 제거되어 404 반환.
func TestDeviceHandler_Get_CompositeReturns404(t *testing.T) {
	now := time.Now()

	// getFn 이 호출되더라도 핸들러에서 composite kind 차단 → 404 응답.
	// resolveDeviceFn 미설정 시 mock 의 fallback 동작 (ClassifyDeviceRef + getFn) 사용.
	registry := &mockDeviceRegistry{
		getFn: func(id string) (device.Device, error) {
			return helperLgcnpDevice(now), nil
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet, "/api/v1/devices/lg_icp01:81", nil)

	require.Equal(t, http.StatusNotFound, rec.Code,
		"composite reference 는 Phase D 부터 alias 제거되어 404 반환")
	assert.Empty(t, rec.Header().Get("Deprecation"),
		"composite alias 가 제거되었으므로 Deprecation 헤더 부착 없음")

	body := rec.Body.String()
	assert.Contains(t, body, "lg_icp01:81",
		"에러 메시지는 입력 reference 를 echo 해야 한다")
	assert.NotContains(t, body, "legacy agent:local_id",
		"에러 메시지는 Phase D 부터 composite 형식을 옵션으로 안내하지 않아야 한다")
}

// B-AC5 / B-T4: 어떤 형식에도 매칭되지 않는 reference 는 404 + 명시적 메시지.
func TestDeviceHandler_Get_UnknownReferenceReturns404(t *testing.T) {
	registry := &mockDeviceRegistry{}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet, "/api/v1/devices/invalid-xyz-format", nil)

	require.Equal(t, http.StatusNotFound, rec.Code)

	body := rec.Body.String()
	assert.Contains(t, body, "invalid-xyz-format",
		"에러 메시지는 입력 reference 를 echo 해야 한다")
	assert.Contains(t, body, "UUID",
		"에러 메시지는 허용 형식 (UUID) 안내를 포함해야 한다")
	assert.Contains(t, body, "agent/name",
		"에러 메시지는 허용 형식 (agent/name) 안내를 포함해야 한다")
}

// B-AC5 (보조): composite 형식인데 매핑이 없으면 404 (composite resolver 가
// ErrDeviceNotFound 반환). Deprecation 헤더는 dispatch 가 composite 로 분류된
// 시점에 부착되므로 NOT-FOUND 응답에도 헤더가 남을 수 있다 (운영자 가시성).
// 본 테스트는 404 동작만 검증하며 헤더 부착 여부는 의도된 동작 범위에서
// 허용한다.
func TestDeviceHandler_Get_CompositeMissingReturns404(t *testing.T) {
	registry := &mockDeviceRegistry{
		getFn: func(_ string) (device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet, "/api/v1/devices/lg_icp01:nonexistent", nil)

	require.Equal(t, http.StatusNotFound, rec.Code)
}

// B-AC3 (회귀): GET /devices/{agent}/{name} 의 미매칭 케이스 — 404.
func TestDeviceHandler_GetByAgentName_NotFound(t *testing.T) {
	registry := &mockDeviceRegistry{
		getByAgentNameFn: func(_, _ string) (device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet,
		"/api/v1/devices/lg_hvacr01/nonexistent", nil)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "lg_hvacr01")
	assert.Contains(t, body, "nonexistent")
}

// ---------------------------------------------------------------------------
// SPEC-DEVICE-IDENTITY-001 Phase B — B-T5 (B-AC4)
//
// GET /api/v1/devices:resolve?agent=X&name=Y 신규 엔드포인트 검증.
//   - 정상 매칭: 200 + DeviceDetailResponse JSON (B-AC4)
//   - 매칭 없음: 404 + 명시적 메시지 (B-AC4 / B-AC5)
//   - agent 또는 name 미제공: 400 (B-AC4)
// ---------------------------------------------------------------------------

// B-AC4 / B-T5: name 기반 명시 resolver 엔드포인트의 정상 매칭.
func TestDeviceHandler_ResolveByAgentName_Success(t *testing.T) {
	now := time.Now()

	registry := &mockDeviceRegistry{
		getByAgentNameFn: func(agentName, name string) (device.Device, error) {
			assert.Equal(t, "lg_hvacr01", agentName)
			assert.Equal(t, "indoor-1", name)
			return helperLgcnpDevice(now), nil
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet,
		"/api/v1/devices:resolve?agent=lg_hvacr01&name=indoor-1", nil)

	require.Equal(t, http.StatusOK, rec.Code)
	// agent/name 1급 reference 이므로 Deprecation 헤더 부재.
	assert.Empty(t, rec.Header().Get("Deprecation"))

	var resp dto.APIResponse[DeviceDetailResponse]
	decodeJSON(t, rec, &resp)
	require.True(t, resp.Success)
	assert.Equal(t, "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d", resp.Data.ID, "Phase D: id 가 UUID 반환")
	assert.Equal(t, "indoor-1", resp.Data.Name)
}

// B-AC4 / B-T5: 잘못된 query parameter 조합 — 400.
func TestDeviceHandler_ResolveByAgentName_MissingParams(t *testing.T) {
	registry := &mockDeviceRegistry{}
	router := setupDeviceRouter(registry, &mockMetadataRepo{})

	t.Run("agent 누락", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet,
			"/api/v1/devices:resolve?name=indoor-1", nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("name 누락", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet,
			"/api/v1/devices:resolve?agent=lg_hvacr01", nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("둘 다 누락", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet,
			"/api/v1/devices:resolve", nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("agent 빈 문자열", func(t *testing.T) {
		rec := doRequest(t, router, http.MethodGet,
			"/api/v1/devices:resolve?agent=&name=indoor-1", nil)
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})
}

// B-AC4 / B-T5: name 매칭 없음 → 404 + 명시적 메시지.
func TestDeviceHandler_ResolveByAgentName_NotFound(t *testing.T) {
	registry := &mockDeviceRegistry{
		getByAgentNameFn: func(_, _ string) (device.Device, error) {
			return nil, device.ErrDeviceNotFound
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet,
		"/api/v1/devices:resolve?agent=lg_hvacr01&name=missing-device", nil)

	require.Equal(t, http.StatusNotFound, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "lg_hvacr01",
		"404 메시지는 입력 agent 를 echo 해야 한다")
	assert.Contains(t, body, "missing-device",
		"404 메시지는 입력 name 을 echo 해야 한다")
}

// B-AC4 / B-T5 (회귀): /devices:resolve 와 /devices/{ref} 가 disjoint 한지
// TestDeviceHandler_ResolveRoute_CompositeReturns404 은 Phase D 부터 composite
// path 형식이 alias 제거되어 404 를 반환함을 검증한다. ":resolve" 라우트와의
// path-matching disjoint 성은 유지된다 (별도 라우트는 정상 동작).
func TestDeviceHandler_ResolveRoute_CompositeReturns404(t *testing.T) {
	registry := &mockDeviceRegistry{
		getFn: func(id string) (device.Device, error) {
			// composite path 가 ResolveDevice 의 default 분기로 들어와도
			// 핸들러가 kind 를 composite 로 판단하여 404 반환.
			return helperLgcnpDevice(time.Now()), nil
		},
	}

	router := setupDeviceRouter(registry, &mockMetadataRepo{})
	rec := doRequest(t, router, http.MethodGet, "/api/v1/devices/lg_icp01:81", nil)

	require.Equal(t, http.StatusNotFound, rec.Code,
		"/devices/{composite} 는 Phase D 부터 alias 제거되어 404 반환")
	assert.Empty(t, rec.Header().Get("Deprecation"),
		"Deprecation 헤더는 더 이상 부착되지 않는다")
}
