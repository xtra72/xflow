package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/config"
)

// setupRemoteConfigRouter 는 temp dir 데이터 디렉토리를 쓰는 실제 config 로 핸들러를 등록한 라우터를 만든다.
// storage.sqlite.path 를 temp dir 로 지정해 오버라이드 파일이 temp 에 격리되게 한다(레포 오염 방지).
func setupRemoteConfigRouter(t *testing.T) *api.Router {
	t.Helper()
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "xflow.yaml")
	require.NoError(t, os.WriteFile(cfgFile,
		[]byte("storage:\n  sqlite:\n    path: "+filepath.Join(dir, "xflow.db")+"\n"), 0o644))
	cfg, err := config.Load(config.WithConfigFile(cfgFile))
	require.NoError(t, err)

	router := api.NewRouter()
	h := NewRemoteConfigHandler(cfg, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// TestRemoteConfig_Get_RequiresAdmin 는 admin 이 아니면 403 인지 검증한다.
func TestRemoteConfig_Get_RequiresAdmin(t *testing.T) {
	router := setupRemoteConfigRouter(t)
	rec := requestWithRole(t, router, http.MethodGet, "/api/v1/system/remote-config", "viewer", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestRemoteConfig_Get_Defaults 는 admin GET 이 기본 remote_management 설정을 반환하는지 검증한다.
func TestRemoteConfig_Get_Defaults(t *testing.T) {
	router := setupRemoteConfigRouter(t)
	rec := requestWithRole(t, router, http.MethodGet, "/api/v1/system/remote-config", "admin", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[dto.RemoteClientConfigResponse]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "disabled", resp.Data.Mode) // 기본값
	assert.False(t, resp.Data.EnrollmentTokenSet)
	assert.Contains(t, resp.Data.RestartRequiredFields, "remote_management.enrollment_token")
	assert.NotContains(t, resp.Data.RestartRequiredFields, "remote_management.mode") // mutable
}

// TestRemoteConfig_Put_AppliesAndPersists 는 PUT 이 값을 적용하고, 시크릿은 GET 에서 값 노출
// 없이 set 여부만 보이며, 비-mutable 키가 needs_restart 로 보고되는지 검증한다.
func TestRemoteConfig_Put_AppliesAndPersists(t *testing.T) {
	router := setupRemoteConfigRouter(t)

	body := `{"mode":"client","server_url":"wss://mgmt.example","enrollment_token":"tok-secret","auto_register":true}`
	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/remote-config", "admin", strings.NewReader(body))
	require.Equal(t, http.StatusOK, rec.Code)

	var put dto.APIResponse[dto.RemoteClientConfigUpdateResult]
	decodeJSON(t, rec, &put)
	assert.True(t, put.Success)
	assert.Contains(t, put.Data.Applied, "remote_management.mode")
	assert.Contains(t, put.Data.Applied, "remote_management.enrollment_token")
	// mode/server_url 은 mutable → needs_restart 아님. enrollment_token/auto_register 는 비-mutable.
	assert.NotContains(t, put.Data.NeedsRestart, "remote_management.mode")
	assert.Contains(t, put.Data.NeedsRestart, "remote_management.enrollment_token")
	assert.Contains(t, put.Data.NeedsRestart, "remote_management.auto_register")

	// GET 이 저장값을 반영하되 시크릿 값은 노출하지 않아야 한다.
	rec2 := requestWithRole(t, router, http.MethodGet, "/api/v1/system/remote-config", "admin", nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	var get dto.APIResponse[dto.RemoteClientConfigResponse]
	decodeJSON(t, rec2, &get)
	assert.Equal(t, "client", get.Data.Mode)
	assert.Equal(t, "wss://mgmt.example", get.Data.ServerURL)
	assert.True(t, get.Data.AutoRegister)
	assert.True(t, get.Data.EnrollmentTokenSet) // 설정됨 표시
	// 응답 어디에도 시크릿 원문이 없어야 한다.
	assert.NotContains(t, rec2.Body.String(), "tok-secret")
}

// TestRemoteConfig_Put_InvalidMode 는 잘못된 mode 를 422 로 거부하는지 검증한다.
func TestRemoteConfig_Put_InvalidMode(t *testing.T) {
	router := setupRemoteConfigRouter(t)
	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/remote-config", "admin",
		strings.NewReader(`{"mode":"bogus"}`))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
