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

// setupScheduleLogConfigRouter 는 temp dir 데이터 디렉토리를 쓰는 실제 config 로 핸들러를 등록한
// 라우터와 config 를 반환한다. storage.sqlite.path 를 temp dir 로 지정해 오버라이드 파일이
// temp 에 격리되게 한다(레포 오염 방지).
func setupScheduleLogConfigRouter(t *testing.T) *api.Router {
	t.Helper()
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "xflow.yaml")
	require.NoError(t, os.WriteFile(cfgFile,
		[]byte("storage:\n  sqlite:\n    path: "+filepath.Join(dir, "xflow.db")+"\n"), 0o644))
	cfg, err := config.Load(config.WithConfigFile(cfgFile))
	require.NoError(t, err)

	router := api.NewRouter()
	h := NewScheduleLogConfigHandler(cfg, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router
}

// TestScheduleLogConfig_Get_RequiresAdmin 는 admin 이 아니면 403 인지 검증한다.
func TestScheduleLogConfig_Get_RequiresAdmin(t *testing.T) {
	router := setupScheduleLogConfigRouter(t)
	rec := requestWithRole(t, router, http.MethodGet, "/api/v1/system/schedule-log-config", "viewer", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestScheduleLogConfig_Put_RequiresAdmin 는 PUT 도 admin 전용인지 검증한다.
func TestScheduleLogConfig_Put_RequiresAdmin(t *testing.T) {
	router := setupScheduleLogConfigRouter(t)
	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/schedule-log-config", "viewer",
		strings.NewReader(`{"storage_type":"memory"}`))
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestScheduleLogConfig_Get_Defaults 는 admin GET 이 기본값 "sqlite" 를 반환하는지 검증한다.
func TestScheduleLogConfig_Get_Defaults(t *testing.T) {
	router := setupScheduleLogConfigRouter(t)
	rec := requestWithRole(t, router, http.MethodGet, "/api/v1/system/schedule-log-config", "admin", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var resp dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &resp)
	assert.True(t, resp.Success)
	assert.Equal(t, "sqlite", resp.Data["storage_type"])
}

// TestScheduleLogConfig_Put_RoundTrips 는 PUT 이 값을 영속화하고(SetPersistent 가 이 키를 수용),
// 이후 GET 이 저장값을 반영하며, 이 키는 비-mutable 이므로 needs_restart=true 인지 검증한다.
// SetPersistent("storage.schedule_log.type", ...) 가 오버라이드 allowlist 에 포함되어야 통과한다.
func TestScheduleLogConfig_Put_RoundTrips(t *testing.T) {
	router := setupScheduleLogConfigRouter(t)

	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/schedule-log-config", "admin",
		strings.NewReader(`{"storage_type":"memory"}`))
	require.Equal(t, http.StatusOK, rec.Code)

	var put dto.APIResponse[map[string]any]
	decodeJSON(t, rec, &put)
	assert.True(t, put.Success)
	assert.Equal(t, "storage.schedule_log.type", put.Data["applied"])
	assert.Equal(t, true, put.Data["needs_restart"]) // 시작 설정 → 재시작 필요

	// GET 이 저장값을 반영해야 한다(런타임 viper 반영 확인 = put→get 라운드트립).
	rec2 := requestWithRole(t, router, http.MethodGet, "/api/v1/system/schedule-log-config", "admin", nil)
	require.Equal(t, http.StatusOK, rec2.Code)
	var get dto.APIResponse[map[string]any]
	decodeJSON(t, rec2, &get)
	assert.Equal(t, "memory", get.Data["storage_type"])
}

// TestScheduleLogConfig_Put_InvalidValue 는 허용 집합 밖 값을 422 로 거부하는지 검증한다.
func TestScheduleLogConfig_Put_InvalidValue(t *testing.T) {
	router := setupScheduleLogConfigRouter(t)
	rec := requestWithRole(t, router, http.MethodPut, "/api/v1/system/schedule-log-config", "admin",
		strings.NewReader(`{"storage_type":"redis"}`))
	assert.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}
