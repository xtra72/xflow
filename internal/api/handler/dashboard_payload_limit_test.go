// dashboard_payload_limit_test.go 는 설정에서 유도된 PUT 페이로드 상한이 **핸들러까지
// 실제로 닿는지** 검증한다 (@SPEC:SPEC-CANVAS-007 §결정 14).
//
// **순수 함수 시험(config 패키지)만으로는 부족한 이유**: 유도가 옳아도 핸들러가 그 값을
// 쓰지 않으면 — 주입을 빠뜨렸거나 상수를 그대로 읽으면 — 설정은 조용히 무시된다. 그
// 침묵이 이 변경이 없애려는 결함(저장 시점의 413)과 정확히 같은 모양이므로, 경계를
// **HTTP 를 지나** 친다.
package handler

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/storage"
)

// limitEnv 는 페이로드 상한을 주입한 대시보드 환경이다.
type limitEnv struct {
	router *api.Router
	db     *sql.DB
	repo   storage.DashboardRepository
}

// newLimitEnv 는 payloadLimit 을 주입한 라우터를 세운다.
//
// newDashboardEnv 를 재사용하지 않는 것은 그 헬퍼가 상한을 주입하지 않기 때문이고,
// 그 헬퍼를 고치면 **주입 없는 경로(폴백)를 재는 기존 시험**이 사라진다.
func newLimitEnv(t *testing.T, payloadLimit int64) *limitEnv {
	t.Helper()
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "dashboard-limit.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	require.NoError(t, storage.InsertUser(ctx, db, "root", "hash-root", "admin", 0, 0))

	repo, err := storage.NewDashboardEntitySQLiteRepository(ctx, db)
	require.NoError(t, err)
	aclRepo, err := storage.NewDashboardACLSQLiteRepository(ctx, db)
	require.NoError(t, err)
	stateRepo, err := storage.NewDashboardUserStateSQLiteRepository(ctx, db)
	require.NoError(t, err)

	resolver := auth.NewSQLPermissionResolver(db)
	permCache := auth.NewPermissionCache(resolver).WithUserRoles(resolver)

	router := api.NewRouter()
	router.Use(api.Authorization(true, permCache, nil))
	g := router.Group("/api/v1")

	NewDashboardHandler(repo, aclRepo, stateRepo, nil, nil).
		WithPermissions(permCache).
		WithAuthEnabled(true).
		WithSubjectDB(db).
		WithPayloadLimit(payloadLimit).
		RegisterRoutes(g)

	return &limitEnv{router: router, db: db, repo: repo}
}

// createForLimitEnv 는 대시보드를 하나 만들고 uid 를 반환한다.
func createForLimitEnv(t *testing.T, env *limitEnv, owner, name string) string {
	t.Helper()
	rec := requestWithAuth(t, env.router, http.MethodPost, "/api/v1/dashboards",
		owner, "admin", jsonBody(`{"name":`+strconv.Quote(name)+`}`), "")
	require.Equal(t, http.StatusCreated, rec.Code, "생성 실패: %s", rec.Body.String())

	var d dto.DashboardDetail
	decodeEnvelope(t, rec, &d)
	return d.UID
}

// getVersionForLimitEnv 는 저장소에서 직접 version 을 읽는다(서버 상태 불변 검증용).
func getVersionForLimitEnv(t *testing.T, env *limitEnv, uid string) int64 {
	t.Helper()
	d, err := env.repo.Get(context.Background(), uid)
	require.NoError(t, err)
	return d.Version
}

// putForLimitEnv 는 본문을 그대로 실어 PUT 한다.
func putForLimitEnv(t *testing.T, env *limitEnv, uid, body string) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithAuth(t, env.router, http.MethodPut, "/api/v1/dashboards/"+uid,
		"root", "admin", jsonBody(body), "")
}

// bodyOfExactly 는 정확히 total 바이트인 대시보드 PUT 본문을 만든다.
//
// 상한이 `<=` 인지 `<` 인지를 재려면 **정확한 바이트 수**가 필요하다 — 어림한 본문으로는
// 경계에서 한 칸 어긋나도 시험이 통과한다.
func bodyOfExactly(t *testing.T, total int) string {
	t.Helper()
	const prefix = `{"payload":{"blob":"`
	const suffix = `"}}`
	fill := total - len(prefix) - len(suffix)
	require.Greater(t, fill, 0, "총 바이트가 봉투보다 커야 한다")
	body := prefix + strings.Repeat("x", fill) + suffix
	require.Len(t, body, total, "본문이 정확히 요청한 바이트여야 한다")
	return body
}

// TestDashboardPut_DerivedLimit_JustUnderAndOver 는 유도된 상한의 **양쪽**을 친다.
//
// 요소 300개 → 300 × 1KiB = 307,200 B. 바닥(256KB)보다 크므로 유도값이 실제로 쓰이는지가
// 관측된다 — 바닥 구간에서 재면 상수 폴백과 구별되지 않는다.
func TestDashboardPut_DerivedLimit_JustUnderAndOver(t *testing.T) {
	const limit = 300 * 1024 // config.DeriveDashboardPayloadBytes(300)
	require.Greater(t, int64(limit), int64(256*1024),
		"바닥보다 커야 폴백(256KB)과 구별된다")

	env := newLimitEnv(t, limit)
	uid := createForLimitEnv(t, env, "root", "상한 경계")
	before := getVersionForLimitEnv(t, env, uid)

	t.Run("상한과 정확히 같으면 저장된다", func(t *testing.T) {
		rec := putForLimitEnv(t, env, uid, bodyOfExactly(t, limit))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
		assert.Equal(t, before+1, getVersionForLimitEnv(t, env, uid), "저장되어 version 이 오른다")
	})

	t.Run("한 바이트 넘으면 413 이고 저장되지 않는다", func(t *testing.T) {
		v := getVersionForLimitEnv(t, env, uid)
		rec := putForLimitEnv(t, env, uid, bodyOfExactly(t, limit+1))
		assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		assert.Equal(t, v, getVersionForLimitEnv(t, env, uid), "413 은 저장하지 않는다")
	})

	t.Run("413 메시지가 **적용된** 상한을 말한다", func(t *testing.T) {
		rec := putForLimitEnv(t, env, uid, bodyOfExactly(t, limit+1))
		require.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
		assert.Contains(t, rec.Body.String(), "307200",
			"상수 262144 를 말하면 사용자가 틀린 수를 읽는다")
	})
}

// TestDashboardPut_NoInjection_FallsBackToConst 는 주입이 없을 때 컴파일 기본값이
// 남는지 본다 — 구형 배선과 설정을 모르는 테스트가 그대로 살아 있어야 한다.
func TestDashboardPut_NoInjection_FallsBackToConst(t *testing.T) {
	env := newLimitEnv(t, 0) // 주입 없음
	uid := createForLimitEnv(t, env, "root", "폴백")

	rec := putForLimitEnv(t, env, uid, bodyOfExactly(t, maxDashboardPayloadBytes))
	require.Equal(t, http.StatusOK, rec.Code, "기본값과 같은 크기는 통과한다")

	rec = putForLimitEnv(t, env, uid, bodyOfExactly(t, maxDashboardPayloadBytes+1))
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code, "기본값을 넘으면 413")
}

// TestWithPayloadLimit_IgnoresNonPositive 는 주입이 상한을 **줄이지 못함**을 본다.
// 0/음수 주입이 상한을 0 으로 만들면 모든 저장이 413 이 된다.
func TestWithPayloadLimit_IgnoresNonPositive(t *testing.T) {
	h := NewDashboardHandler(nil, nil, nil, nil, nil)
	assert.EqualValues(t, maxDashboardPayloadBytes, h.maxPayloadBytes(), "주입 전")

	h.WithPayloadLimit(0)
	assert.EqualValues(t, maxDashboardPayloadBytes, h.maxPayloadBytes(), "0 은 무시된다")

	h.WithPayloadLimit(-1)
	assert.EqualValues(t, maxDashboardPayloadBytes, h.maxPayloadBytes(), "음수는 무시된다")

	h.WithPayloadLimit(999)
	assert.EqualValues(t, 999, h.maxPayloadBytes(), "양수는 적용된다")
}
