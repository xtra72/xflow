// dashboard_limits_test.go 는 편집기가 서버 상한을 읽어 가는 창구를 검증한다
// (@SPEC:SPEC-CANVAS-007 §결정 14).
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/config"
	"github.com/xtra/xflow/internal/storage"
)

// newLimitsRouter 는 주어진 config YAML 로 상한 조회 라우터를 세운다.
func newLimitsRouter(t *testing.T, yaml string) *api.Router {
	t.Helper()
	ctx := context.Background()

	path := filepath.Join(t.TempDir(), "xflow.yaml")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))
	cfg, err := config.Load(config.WithConfigFile(path))
	require.NoError(t, err)

	dbPath := filepath.Join(t.TempDir(), "limits.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	for _, u := range []struct{ name, role string }{{"root", "admin"}, {"vie", "viewer"}} {
		require.NoError(t, storage.InsertUser(ctx, db, u.name, "hash-"+u.name, u.role, 0, 0))
	}

	resolver := auth.NewSQLPermissionResolver(db)
	permCache := auth.NewPermissionCache(resolver).WithUserRoles(resolver)

	router := api.NewRouter()
	router.Use(api.Authorization(true, permCache, nil))
	g := router.Group("/api/v1")
	NewDashboardLimitsHandler(cfg, nil).RegisterRoutes(g)

	return router
}

// limitsOf 는 상한 조회 응답을 디코드해 두 수를 반환한다.
func limitsOf(t *testing.T, router *api.Router, user, role string) (int, int64) {
	t.Helper()
	rec := requestWithAuth(t, router, http.MethodGet, "/api/v1/dashboard-limits", user, role, nil, "")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var env struct {
		Data struct {
			MaxCanvasElements  int   `json:"max_canvas_elements"`
			PayloadBudgetBytes int64 `json:"payload_budget_bytes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return env.Data.MaxCanvasElements, env.Data.PayloadBudgetBytes
}

// TestDashboardLimits_ReportsConfiguredValue 는 설정한 수가 그대로 나오는지 본다.
func TestDashboardLimits_ReportsConfiguredValue(t *testing.T) {
	t.Run("기본값", func(t *testing.T) {
		router := newLimitsRouter(t, "server:\n  port: 8080\n")
		elements, budget := limitsOf(t, router, "root", "admin")
		assert.Equal(t, 1024, elements)
		assert.EqualValues(t, 1024*1024, budget)
	})

	t.Run("설정 파일의 값", func(t *testing.T) {
		router := newLimitsRouter(t, "dashboard:\n  max_canvas_elements: 3000\n")
		elements, budget := limitsOf(t, router, "root", "admin")
		assert.Equal(t, 3000, elements)
		assert.EqualValues(t, 3000*1024, budget)
	})

	t.Run("오타는 천장에서 죄여 보고된다", func(t *testing.T) {
		router := newLimitsRouter(t, "dashboard:\n  max_canvas_elements: 99999999\n")
		elements, budget := limitsOf(t, router, "root", "admin")
		assert.EqualValues(t, config.MaxCanvasElementsCeiling, elements,
			"편집기가 죄기 전 값을 받으면 받아 놓고 저장이 실패한다")
		assert.Equal(t, config.MaxDashboardPayloadBytes, budget)
	})
}

// TestDashboardLimits_ReadableByNonAdmin 은 **admin 이 아닌 편집자도** 같은 수를
// 읽는지 본다.
//
// admin 게이트를 두면 그들만 컴파일 기본값으로 떨어져, 운영자가 상한을 올려도 그들에게만
// 가져오기가 거절된다 — 이 창구가 없애려는 바로 그 조용한 갈라짐이다.
func TestDashboardLimits_ReadableByNonAdmin(t *testing.T) {
	router := newLimitsRouter(t, "dashboard:\n  max_canvas_elements: 2048\n")

	adminElements, adminBudget := limitsOf(t, router, "root", "admin")
	viewerElements, viewerBudget := limitsOf(t, router, "vie", "viewer")

	assert.Equal(t, adminElements, viewerElements, "viewer 도 같은 수를 읽는다")
	assert.Equal(t, adminBudget, viewerBudget)
	assert.Equal(t, 2048, viewerElements)
}
