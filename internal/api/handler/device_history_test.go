// device_history_test.go 는 GET /devices/{id}/history 엔드포인트 통합 테스트이다.
//
// 검증 범위:
//   - 이력 제공자 미주입 시 history 라우트 미등록(404)
//   - happy path: UUID 로 조회 → 200 + 최신순 엔트리
//   - limit 파라미터 전달(clamp 는 제공자가 담당 — 전달값 검증)
//   - 빈 결과 → 200 + 빈 배열
//   - composite 참조 → 404(마이그레이션 안내)
//   - 잘못된 형식 → 400
//   - agent/name 해석 → UUID 키로 조회
package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/device"
)

// --- Mock DeviceHistoryProvider ---

type mockHistoryProvider struct {
	historyFn  func(deviceID string, limit int) []device.HistorySnapshot
	maxEntries int
	lastID     string
	lastLimit  int
}

func (m *mockHistoryProvider) History(deviceID string, limit int) []device.HistorySnapshot {
	m.lastID = deviceID
	m.lastLimit = limit
	if m.historyFn != nil {
		return m.historyFn(deviceID, limit)
	}
	return nil
}

func (m *mockHistoryProvider) MaxEntries() int {
	if m.maxEntries == 0 {
		return 100
	}
	return m.maxEntries
}

// setupDeviceHistoryRouter 는 history 제공자가 주입된 DeviceHandler 라우터를 만든다.
func setupDeviceHistoryRouter(reg *mockDeviceRegistry, hp DeviceHistoryProvider) *api.Router {
	router := api.NewRouter()
	opts := []DeviceHandlerOption{}
	if hp != nil {
		opts = append(opts, WithDeviceHistory(hp))
	}
	h := NewDeviceHandler(reg, &mockMetadataRepo{}, nil, opts...)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

const histUUID = "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d"

func doGet(router *api.Router, path string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// TestDeviceHistory_RouteNotRegisteredWhenNoProvider 는 제공자 미주입 시 라우트가
// 등록되지 않아 404 임을 검증한다.
func TestDeviceHistory_RouteNotRegisteredWhenNoProvider(t *testing.T) {
	router := setupDeviceHistoryRouter(&mockDeviceRegistry{}, nil)
	rec := doGet(router, "/api/v1/devices/"+histUUID+"/history")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestDeviceHistory_HappyPath(t *testing.T) {
	snaps := []device.HistorySnapshot{
		{Timestamp: 3000, Online: true, LastSeen: 2999, Properties: map[string]any{"n": 3}},
		{Timestamp: 2000, Online: true, LastSeen: 1999, Properties: map[string]any{"n": 2}},
		{Timestamp: 1000, Online: false, LastSeen: 999, Properties: map[string]any{"n": 1}},
	}
	hp := &mockHistoryProvider{
		historyFn: func(_ string, _ int) []device.HistorySnapshot { return snaps },
	}
	// UUID 해석이 성공하도록 레지스트리에 디바이스를 둔다.
	reg := &mockDeviceRegistry{
		getByUIDFn: func(uid string) (device.Device, error) {
			return &mockDevice{id: uid, uid: uid, online: true, lastSeen: time.UnixMilli(2999)}, nil
		},
	}
	router := setupDeviceHistoryRouter(reg, hp)

	rec := doGet(router, "/api/v1/devices/"+histUUID+"/history")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp dto.APIResponse[HistoryResponse]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, histUUID, resp.Data.DeviceID)
	assert.Equal(t, 3, resp.Data.Count)
	require.Len(t, resp.Data.Entries, 3)
	// 최신순 보장(제공자가 반환한 순서 그대로).
	assert.Equal(t, int64(3000), resp.Data.Entries[0].Timestamp)
	assert.Equal(t, histUUID, hp.lastID, "버퍼 키는 해석된 UUID 여야 한다")
}

func TestDeviceHistory_LimitParam(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		wantLimit int
	}{
		{"limit 지정", "?limit=5", 5},
		{"limit 미지정 → 0(제공자 clamp)", "", 0},
		{"limit 0 → 0", "?limit=0", 0},
		{"limit 음수 → 0", "?limit=-3", 0},
		{"limit 비정수 → 0", "?limit=abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hp := &mockHistoryProvider{
				historyFn: func(_ string, _ int) []device.HistorySnapshot {
					return []device.HistorySnapshot{}
				},
			}
			reg := &mockDeviceRegistry{
				getByUIDFn: func(uid string) (device.Device, error) {
					return &mockDevice{id: uid, uid: uid}, nil
				},
			}
			router := setupDeviceHistoryRouter(reg, hp)

			rec := doGet(router, "/api/v1/devices/"+histUUID+"/history"+tt.query)
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
			assert.Equal(t, tt.wantLimit, hp.lastLimit)
		})
	}
}

func TestDeviceHistory_EmptyResult(t *testing.T) {
	hp := &mockHistoryProvider{
		historyFn: func(_ string, _ int) []device.HistorySnapshot { return nil },
	}
	reg := &mockDeviceRegistry{
		getByUIDFn: func(uid string) (device.Device, error) {
			return &mockDevice{id: uid, uid: uid}, nil
		},
	}
	router := setupDeviceHistoryRouter(reg, hp)

	rec := doGet(router, "/api/v1/devices/"+histUUID+"/history")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp dto.APIResponse[HistoryResponse]
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.Equal(t, 0, resp.Data.Count)
	assert.NotNil(t, resp.Data.Entries)
	assert.Empty(t, resp.Data.Entries)
}

// TestDeviceHistory_UnresolvedUUIDStillQueries 는 레지스트리에 없는(오프라인/제거)
// UUID 도 그대로 버퍼 키로 조회됨을 검증한다(GC 전 이력 보존).
func TestDeviceHistory_UnresolvedUUIDStillQueries(t *testing.T) {
	hp := &mockHistoryProvider{
		historyFn: func(_ string, _ int) []device.HistorySnapshot {
			return []device.HistorySnapshot{{Timestamp: 1}}
		},
	}
	// getByUIDFn 미설정 → ResolveDevice 가 ErrDeviceNotFound. id 를 그대로 사용.
	reg := &mockDeviceRegistry{}
	router := setupDeviceHistoryRouter(reg, hp)

	rec := doGet(router, "/api/v1/devices/"+histUUID+"/history")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, histUUID, hp.lastID)
}

func TestDeviceHistory_CompositeRejected(t *testing.T) {
	hp := &mockHistoryProvider{}
	router := setupDeviceHistoryRouter(&mockDeviceRegistry{}, hp)

	// composite "agent:local_id" → 404 마이그레이션 안내.
	rec := doGet(router, "/api/v1/devices/lg_icp01:81/history")
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestDeviceHistory_AgentNameResolved 는 단일 path 세그먼트로 전달된 agent/name
// (슬래시 URL 인코딩)이 UUID 로 해석되어 그 키로 조회됨을 검증한다.
//
// 주의: history 라우트는 "/devices/{id}/history" 로 {id} 가 단일 세그먼트이므로,
// agent/name 의 슬래시는 %2F 로 인코딩되어야 한다(클라이언트는 UUID 사용을 권장).
func TestDeviceHistory_AgentNameResolved(t *testing.T) {
	hp := &mockHistoryProvider{
		historyFn: func(_ string, _ int) []device.HistorySnapshot {
			return []device.HistorySnapshot{{Timestamp: 1}}
		},
	}
	reg := &mockDeviceRegistry{
		getByAgentNameFn: func(agent, name string) (device.Device, error) {
			return &mockDevice{id: histUUID, uid: histUUID, agentName: agent, name: name}, nil
		},
	}
	router := setupDeviceHistoryRouter(reg, hp)

	// 단일 세그먼트로 인코딩된 agent/name → ResolveDevice 가 UUID 로 해석.
	rec := doGet(router, "/api/v1/devices/agent1%2Fsensor7/history")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, histUUID, hp.lastID)
}
