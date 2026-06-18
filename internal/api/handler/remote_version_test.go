// remote_version_test.go 는 버전 관리(Phase 1) REST 핸들러를 검증한다.
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/storage"
)

// memSettings 는 SettingsRepository 의 인메모리 테스트 구현이다.
type memSettings struct {
	mu sync.Mutex
	kv map[string]string
}

func newMemSettings() *memSettings { return &memSettings{kv: map[string]string{}} }

func (m *memSettings) GetSetting(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.kv[key]
	if !ok {
		return "", storage.ErrSettingNotFound
	}
	return v, nil
}

func (m *memSettings) SetSetting(_ context.Context, key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.kv[key] = value
	return nil
}

func (m *memSettings) Close() error { return nil }

// versionAdminRequest 는 admin role 이 주입된 요청 실행기를 만든다(settings 연결).
func versionAdminRequest(t *testing.T, svc NodeAdminService, settings storage.SettingsRepository) func(method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewRemoteAdminHandler(svc, nil).WithSettings(settings)
	return func(method, target, body string) *httptest.ResponseRecorder {
		router := api.NewRouter()
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, target, strings.NewReader(body))
		} else {
			req = httptest.NewRequest(method, target, nil)
		}
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), "admin")
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), "alice")
		req = req.WithContext(ctx)
		rec := httptest.NewRecorder()
		h.RegisterRoutes(router.Group("/api/v1"))
		router.Handler().ServeHTTP(rec, req)
		return rec
	}
}

// 응답 envelope 에서 data 를 추출한다.
func decodeData(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.True(t, env.Success, "응답 success=true 여야 한다: %s", rec.Body.String())
	require.NoError(t, json.Unmarshal(env.Data, dst))
}

func TestTargetVersion_SetAndGet(t *testing.T) {
	settings := newMemSettings()
	do := versionAdminRequest(t, newFakeNodeAdmin(), settings)

	// 초기: 빈 목표 버전.
	rec := do(http.MethodGet, "/api/v1/remote/target-version", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var got struct {
		Version string `json:"version"`
	}
	decodeData(t, rec, &got)
	assert.Equal(t, "", got.Version)

	// 설정.
	rec = do(http.MethodPut, "/api/v1/remote/target-version", `{"version":"v1.3.0"}`)
	require.Equal(t, http.StatusOK, rec.Code)

	// 재조회.
	rec = do(http.MethodGet, "/api/v1/remote/target-version", "")
	decodeData(t, rec, &got)
	assert.Equal(t, "v1.3.0", got.Version)
}

func TestTargetVersion_InvalidRejected(t *testing.T) {
	do := versionAdminRequest(t, newFakeNodeAdmin(), newMemSettings())
	rec := do(http.MethodPut, "/api/v1/remote/target-version", `{"version":"not-semver"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestTargetVersion_EmptyClears(t *testing.T) {
	settings := newMemSettings()
	do := versionAdminRequest(t, newFakeNodeAdmin(), settings)
	require.Equal(t, http.StatusOK, do(http.MethodPut, "/api/v1/remote/target-version", `{"version":"v1.3.0"}`).Code)
	// 빈 문자열 = 해제.
	require.Equal(t, http.StatusOK, do(http.MethodPut, "/api/v1/remote/target-version", `{"version":""}`).Code)
	rec := do(http.MethodGet, "/api/v1/remote/target-version", "")
	var got struct {
		Version string `json:"version"`
	}
	decodeData(t, rec, &got)
	assert.Equal(t, "", got.Version)
}

func TestListNodes_OutdatedFlag(t *testing.T) {
	settings := newMemSettings()
	svc := newFakeNodeAdmin()
	svc.nodes["old"] = storage.ManagedNode{InstanceID: "old", Version: "v1.0.0", Status: "approved"}
	svc.nodes["cur"] = storage.ManagedNode{InstanceID: "cur", Version: "v1.3.0", Status: "approved"}
	svc.nodes["dev"] = storage.ManagedNode{InstanceID: "dev", Version: "dev", Status: "approved"}
	do := versionAdminRequest(t, svc, settings)

	// 목표 버전 미설정 → 모두 outdated=false.
	rec := do(http.MethodGet, "/api/v1/remote/nodes", "")
	var nodes []ManagedNodeDTO
	decodeData(t, rec, &nodes)
	for _, n := range nodes {
		assert.False(t, n.Outdated, "목표 미설정 시 outdated=false: %s", n.InstanceID)
	}

	// 목표 v1.3.0 설정.
	require.Equal(t, http.StatusOK, do(http.MethodPut, "/api/v1/remote/target-version", `{"version":"v1.3.0"}`).Code)
	rec = do(http.MethodGet, "/api/v1/remote/nodes", "")
	decodeData(t, rec, &nodes)
	byID := map[string]ManagedNodeDTO{}
	for _, n := range nodes {
		byID[n.InstanceID] = n
	}
	assert.True(t, byID["old"].Outdated, "v1.0.0 < v1.3.0 → outdated")
	assert.False(t, byID["cur"].Outdated, "v1.3.0 == 목표 → not outdated")
	assert.False(t, byID["dev"].Outdated, "비-semver(dev) → not outdated(보수적)")
}

func TestVersionHistory_Endpoint(t *testing.T) {
	svc := newFakeNodeAdmin()
	svc.versionHistory = map[string][]storage.NodeVersionHistory{
		"n1": {
			{InstanceID: "n1", Version: "v1.2.0", ChangedAt: 3000},
			{InstanceID: "n1", Version: "v1.1.0", ChangedAt: 2000},
		},
	}
	do := versionAdminRequest(t, svc, newMemSettings())
	rec := do(http.MethodGet, "/api/v1/remote/nodes/n1/version-history", "")
	require.Equal(t, http.StatusOK, rec.Code)
	var hist []struct {
		Version   string `json:"version"`
		ChangedAt int64  `json:"changed_at"`
	}
	decodeData(t, rec, &hist)
	require.Len(t, hist, 2)
	assert.Equal(t, "v1.2.0", hist[0].Version)
	assert.Equal(t, int64(3000), hist[0].ChangedAt)
}

func TestVersionEndpoints_RequireAdmin(t *testing.T) {
	h := NewRemoteAdminHandler(newFakeNodeAdmin(), nil).WithSettings(newMemSettings())
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))
	// role 미주입(비-admin) → 403.
	for _, target := range []string{
		"/api/v1/remote/target-version",
		"/api/v1/remote/nodes/n1/version-history",
	} {
		req := httptest.NewRequest(http.MethodGet, target, nil)
		rec := httptest.NewRecorder()
		router.Handler().ServeHTTP(rec, req)
		assert.Equal(t, http.StatusForbidden, rec.Code, target)
	}
}
