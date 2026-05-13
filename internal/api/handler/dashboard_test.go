// @SPEC:SPEC-DASHBOARD-001 v0.2.0 (M-7)
// dashboard_test.go — /api/dashboards/{shared,mine} 핸들러 통합 테스트.
//
// 검증 범위 (acceptance.md):
//   - AC-3:  admin shared PUT happy path
//   - AC-4:  editor → shared PUT/DELETE → 403
//   - AC-5:  cross-user 격리 (alice vs bob)
//   - AC-6:  owner spoofing 차단 (body owner 무시)
//   - AC-7:  URL vs body scope 불일치 → 400
//   - AC-8:  비인증 → 401
//   - AC-10: 409 + 서버측 최신 snapshot (If-Match 충돌)
//   - AC-11: 잘못된 JSON → 400
//   - AC-12: 256KB 초과 → 413
//   - AC-17: DELETE mine → 204, 이후 GET → 404

package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

// -----------------------------------------------------------------------------
// Test helpers
// -----------------------------------------------------------------------------

// setupDashboardRouter 는 DashboardHandler 가 등록된 라우터 + repo 를 반환한다.
func setupDashboardRouter(t *testing.T) (*api.Router, storage.DashboardRepository) {
	t.Helper()
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "dashboard.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	repo, err := storage.NewDashboardSQLiteRepository(ctx, db)
	require.NoError(t, err)

	router := api.NewRouter()
	h := NewDashboardHandler(repo, nil, nil)
	g := router.Group("/api/v1")
	h.RegisterRoutes(g)
	return router, repo
}

// requestWithAuth 는 미들웨어를 우회하여 user_id / user_role 컨텍스트를 직접 주입한 후
// 라우터로 요청을 보낸다.
//
// username 이 빈 문자열이면 미인증 (UserID() 가 "" 을 반환).
// role 이 빈 문자열이면 어떤 admin 검사도 통과하지 않음.
// ifMatch 가 빈 문자열이 아니면 If-Match 헤더로 추가.
func requestWithAuth(
	t *testing.T,
	router *api.Router,
	method, path, username, role string,
	body io.Reader,
	ifMatch string,
) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, body)
	req.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		req.Header.Set("If-Match", ifMatch)
	}

	ctx := req.Context()
	if username != "" {
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), username)
	}
	if role != "" {
		ctx = context.WithValue(ctx, api.ContextKeyUserRole(), role)
	}
	req = req.WithContext(ctx)

	router.Handler().ServeHTTP(rec, req)
	return rec
}

// decodeSnapshot 은 응답 body 에서 DashboardSnapshot 을 디코딩한다.
//
// v0.2.1 hotfix: 응답이 표준 APIResponse envelope (`{"success":true,"data":{...}}`)
// 으로 래핑되므로 envelope 을 먼저 unwrap 한 뒤 data 를 반환한다.
func decodeSnapshot(t *testing.T, rec *httptest.ResponseRecorder) dto.DashboardSnapshot {
	t.Helper()
	var env dto.APIResponse[dto.DashboardSnapshot]
	err := json.NewDecoder(rec.Body).Decode(&env)
	require.NoError(t, err, "응답 디코딩 실패: %s", rec.Body.String())
	require.True(t, env.Success, "envelope.success 는 true 여야 함: %s", rec.Body.String())
	return env.Data
}

// decodeSnapshotRaw 는 envelope 의 data 필드를 raw JSON 으로 반환한다.
// owner JSON null vs string 직렬화 등 필드 단위 검증용.
func decodeSnapshotRaw(t *testing.T, rec *httptest.ResponseRecorder) map[string]json.RawMessage {
	t.Helper()
	var env struct {
		Success bool                       `json:"success"`
		Data    map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	require.True(t, env.Success)
	return env.Data
}

// -----------------------------------------------------------------------------
// AC-8: 401 - 비인증 요청 거부 (UR-004)
// -----------------------------------------------------------------------------

func TestDashboardHandler_Unauthenticated_Returns401(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	endpoints := []struct {
		method, path string
	}{
		{"GET", "/api/v1/dashboards/shared"},
		{"PUT", "/api/v1/dashboards/shared"},
		{"DELETE", "/api/v1/dashboards/shared"},
		{"GET", "/api/v1/dashboards/mine"},
		{"PUT", "/api/v1/dashboards/mine"},
		{"DELETE", "/api/v1/dashboards/mine"},
	}
	for _, e := range endpoints {
		t.Run(e.method+" "+e.path, func(t *testing.T) {
			rec := requestWithAuth(t, router, e.method, e.path, "", "", nil, "")
			assert.Equal(t, http.StatusUnauthorized, rec.Code)
		})
	}
}

// -----------------------------------------------------------------------------
// AC-3: admin shared GET/PUT happy path
// -----------------------------------------------------------------------------

func TestDashboardHandler_SharedPut_AdminHappyPath(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// 1. 최초 PUT (admin) → 200, version=1, owner=null
	body := strings.NewReader(`{"payload": {"pages": [{"id": "p1"}]}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/shared", "admin", "admin", body, "")
	require.Equal(t, http.StatusOK, rec.Code, "응답: %s", rec.Body.String())

	snap := decodeSnapshot(t, rec)
	assert.Equal(t, "global", snap.Scope)
	assert.Nil(t, snap.Owner, "scope=global 시 owner 는 JSON null")
	assert.EqualValues(t, 1, snap.Version)
	assert.Positive(t, snap.UpdatedAt, "updatedAt 은 서버 부여")

	// 2. 다른 사용자가 GET → 같은 snapshot 반환 (AC-3)
	rec2 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/shared", "alice", "editor", nil, "")
	require.Equal(t, http.StatusOK, rec2.Code)
	snap2 := decodeSnapshot(t, rec2)
	assert.EqualValues(t, 1, snap2.Version)
	assert.Contains(t, string(snap2.Payload), "p1")

	// 3. 두 번째 PUT (admin) → version=2
	body2 := strings.NewReader(`{"payload": {"pages": [{"id": "p2"}]}}`)
	rec3 := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/shared", "admin", "admin", body2, "")
	require.Equal(t, http.StatusOK, rec3.Code)
	snap3 := decodeSnapshot(t, rec3)
	assert.EqualValues(t, 2, snap3.Version, "두번째 PUT 은 version=2")
}

// -----------------------------------------------------------------------------
// AC-4: editor → shared PUT/DELETE → 403 Forbidden (UB-004)
// -----------------------------------------------------------------------------

func TestDashboardHandler_SharedPut_NonAdmin_Returns403(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	for _, role := range []string{"editor", "viewer"} {
		t.Run("role="+role, func(t *testing.T) {
			body := strings.NewReader(`{"payload": {}}`)
			rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/shared", "alice", role, body, "")
			assert.Equal(t, http.StatusForbidden, rec.Code)
		})
	}
}

func TestDashboardHandler_SharedDelete_NonAdmin_Returns403(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	rec := requestWithAuth(t, router, "DELETE", "/api/v1/dashboards/shared", "alice", "editor", nil, "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// AC-3 보강: editor 가 GET shared 는 허용됨 (읽기 전용).
func TestDashboardHandler_SharedGet_EditorAllowed(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// 먼저 admin 으로 PUT
	body := strings.NewReader(`{"payload": {"x": 1}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/shared", "admin", "admin", body, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// editor 가 GET → 200
	rec2 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/shared", "alice", "editor", nil, "")
	assert.Equal(t, http.StatusOK, rec2.Code)

	// viewer 도 GET 허용
	rec3 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/shared", "bob", "viewer", nil, "")
	assert.Equal(t, http.StatusOK, rec3.Code)
}

// -----------------------------------------------------------------------------
// AC-5: cross-user 격리 — alice 의 PUT 이 bob 의 mine 에 영향 없음
// -----------------------------------------------------------------------------

func TestDashboardHandler_MineCrossUserIsolation(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// alice 가 PUT
	body := strings.NewReader(`{"payload": {"alice": true}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	require.Equal(t, http.StatusOK, rec.Code)
	snap := decodeSnapshot(t, rec)
	require.NotNil(t, snap.Owner)
	assert.Equal(t, "alice", *snap.Owner, "owner 는 JWT username 으로 결정")

	// bob 이 GET → 404 (자신의 snapshot 없음)
	rec2 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/mine", "bob", "editor", nil, "")
	assert.Equal(t, http.StatusNotFound, rec2.Code)

	// bob 이 자신의 PUT
	body2 := strings.NewReader(`{"payload": {"bob": true}}`)
	rec3 := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "bob", "editor", body2, "")
	require.Equal(t, http.StatusOK, rec3.Code)
	snap3 := decodeSnapshot(t, rec3)
	require.NotNil(t, snap3.Owner)
	assert.Equal(t, "bob", *snap3.Owner)
	assert.EqualValues(t, 1, snap3.Version)

	// alice 의 snapshot 은 영향 없음 — version 1 유지, payload alice:true
	rec4 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/mine", "alice", "editor", nil, "")
	require.Equal(t, http.StatusOK, rec4.Code)
	snap4 := decodeSnapshot(t, rec4)
	assert.EqualValues(t, 1, snap4.Version)
	assert.Contains(t, string(snap4.Payload), `"alice":true`)
}

// -----------------------------------------------------------------------------
// AC-6: owner spoofing 차단 (UB-003) — body 의 owner 무시
// -----------------------------------------------------------------------------

func TestDashboardHandler_OwnerSpoofingIgnored(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// alice 가 PUT mine 에 owner=bob 을 시도
	body := strings.NewReader(`{"owner": "bob", "payload": {"data": 1}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	require.Equal(t, http.StatusOK, rec.Code)

	snap := decodeSnapshot(t, rec)
	require.NotNil(t, snap.Owner)
	assert.Equal(t, "alice", *snap.Owner, "JWT username 으로 결정되며 body 의 owner 는 무시됨")

	// bob 이 GET mine → 404 (alice 의 PUT 이 bob 에 영향 주지 않음)
	rec2 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/mine", "bob", "editor", nil, "")
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

// -----------------------------------------------------------------------------
// AC-7: URL vs body scope 불일치 → 400 (UB-006)
// -----------------------------------------------------------------------------

func TestDashboardHandler_ScopeMismatch_Returns400(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// mine URL 에 scope=global 명시
	body := strings.NewReader(`{"scope": "global", "payload": {}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "scope")

	// shared URL 에 scope=user 명시
	body2 := strings.NewReader(`{"scope": "user", "payload": {}}`)
	rec2 := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/shared", "admin", "admin", body2, "")
	assert.Equal(t, http.StatusBadRequest, rec2.Code)
}

// scope 가 URL 과 일치하면 통과 (긍정 케이스)
func TestDashboardHandler_ScopeMatch_Allows(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	body := strings.NewReader(`{"scope": "user", "payload": {"x": 1}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// -----------------------------------------------------------------------------
// AC-11: 잘못된 JSON → 400 (UR-003)
// -----------------------------------------------------------------------------

func TestDashboardHandler_InvalidJSON_Returns400(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	body := strings.NewReader(`{this is not valid json`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

// -----------------------------------------------------------------------------
// AC-12: 256KB+ payload → 413
// -----------------------------------------------------------------------------

func TestDashboardHandler_PayloadTooLarge_Returns413(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// 257KB JSON payload 생성
	bigStr := strings.Repeat("a", 257*1024)
	bodyJSON := fmt.Sprintf(`{"payload": {"big": "%s"}}`, bigStr)
	body := strings.NewReader(bodyJSON)

	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code,
		"256KB 초과 페이로드는 413 Payload Too Large 로 거부")
}

// -----------------------------------------------------------------------------
// AC-10: 409 + 서버측 최신 snapshot (If-Match 충돌)
// -----------------------------------------------------------------------------

func TestDashboardHandler_VersionMismatch_Returns409WithLatest(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// 1. PUT version=1
	body1 := strings.NewReader(`{"payload": {"v": 1}}`)
	rec1 := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body1, "")
	require.Equal(t, http.StatusOK, rec1.Code)
	snap1 := decodeSnapshot(t, rec1)
	assert.EqualValues(t, 1, snap1.Version)

	// 2. PUT (If-Match: 1) → version=2
	body2 := strings.NewReader(`{"payload": {"v": 2}}`)
	rec2 := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body2, "1")
	require.Equal(t, http.StatusOK, rec2.Code)
	snap2 := decodeSnapshot(t, rec2)
	assert.EqualValues(t, 2, snap2.Version)

	// 3. PUT (If-Match: 1, stale) → 409 + 서버측 최신 (version=2)
	body3 := strings.NewReader(`{"payload": {"v": "stale"}}`)
	rec3 := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body3, "1")
	require.Equal(t, http.StatusConflict, rec3.Code)
	snap3 := decodeSnapshot(t, rec3)
	assert.EqualValues(t, 2, snap3.Version, "409 응답 body 에 서버측 최신 snapshot 포함")
	assert.Contains(t, string(snap3.Payload), `"v":2`, "stale 페이로드가 아닌 최신 페이로드가 반환됨")

	// 4. 충돌 후 클라이언트가 If-Match: 2 로 재PUT → 200
	body4 := strings.NewReader(`{"payload": {"v": "after-conflict"}}`)
	rec4 := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body4, "2")
	require.Equal(t, http.StatusOK, rec4.Code)
	snap4 := decodeSnapshot(t, rec4)
	assert.EqualValues(t, 3, snap4.Version)
}

// PUT without If-Match (unconditional) 는 항상 성공
func TestDashboardHandler_PutWithoutIfMatch_AlwaysSucceeds(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	for i := 0; i < 3; i++ {
		body := strings.NewReader(fmt.Sprintf(`{"payload": {"v": %d}}`, i))
		rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
		require.Equal(t, http.StatusOK, rec.Code)
	}
}

// -----------------------------------------------------------------------------
// AC-16: GET 404 → 빌트인 기본값 시나리오 (서버 측 동작만 검증)
// -----------------------------------------------------------------------------

func TestDashboardHandler_Get_Empty_Returns404(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	rec := requestWithAuth(t, router, "GET", "/api/v1/dashboards/mine", "alice", "editor", nil, "")
	assert.Equal(t, http.StatusNotFound, rec.Code)

	rec2 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/shared", "admin", "admin", nil, "")
	assert.Equal(t, http.StatusNotFound, rec2.Code)
}

// -----------------------------------------------------------------------------
// AC-17: DELETE mine → 204, 이후 GET → 404
// -----------------------------------------------------------------------------

func TestDashboardHandler_DeleteMine_204ThenGet404(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// PUT 으로 snapshot 생성
	body := strings.NewReader(`{"payload": {}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// DELETE → 204
	rec2 := requestWithAuth(t, router, "DELETE", "/api/v1/dashboards/mine", "alice", "editor", nil, "")
	assert.Equal(t, http.StatusNoContent, rec2.Code)

	// 이후 GET → 404
	rec3 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/mine", "alice", "editor", nil, "")
	assert.Equal(t, http.StatusNotFound, rec3.Code)
}

// DELETE 멱등 — 존재하지 않더라도 204
func TestDashboardHandler_DeleteMine_Idempotent(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// DELETE 전에 snapshot 없는 상태
	rec := requestWithAuth(t, router, "DELETE", "/api/v1/dashboards/mine", "alice", "editor", nil, "")
	assert.Equal(t, http.StatusNoContent, rec.Code, "snapshot 없어도 204 (멱등)")
}

// DELETE shared admin 만 → 204
func TestDashboardHandler_DeleteShared_AdminSucceeds(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	// 먼저 PUT
	body := strings.NewReader(`{"payload": {}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/shared", "admin", "admin", body, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// admin DELETE → 204
	rec2 := requestWithAuth(t, router, "DELETE", "/api/v1/dashboards/shared", "admin", "admin", nil, "")
	assert.Equal(t, http.StatusNoContent, rec2.Code)

	// 이후 GET → 404
	rec3 := requestWithAuth(t, router, "GET", "/api/v1/dashboards/shared", "admin", "admin", nil, "")
	assert.Equal(t, http.StatusNotFound, rec3.Code)
}

// -----------------------------------------------------------------------------
// 응답 JSON 구조 검증 (UR-002)
// -----------------------------------------------------------------------------

// scope=global 시 owner 가 JSON null 로 직렬화되는지 검증
func TestDashboardHandler_GlobalOwnerSerializesAsNull(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	body := strings.NewReader(`{"payload": {}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/shared", "admin", "admin", body, "")
	require.Equal(t, http.StatusOK, rec.Code)

	// raw body 검사 — envelope 의 data.owner 가 JSON null
	m := decodeSnapshotRaw(t, rec)
	assert.JSONEq(t, "null", string(m["owner"]), "scope=global 시 owner 는 JSON null")
}

// scope=user 시 owner 가 string 으로 직렬화되는지 검증
func TestDashboardHandler_UserOwnerSerializesAsString(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	body := strings.NewReader(`{"payload": {}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	require.Equal(t, http.StatusOK, rec.Code)

	m := decodeSnapshotRaw(t, rec)
	assert.JSONEq(t, `"alice"`, string(m["owner"]))
}

// 빈 payload 객체도 허용 (reset 시나리오)
func TestDashboardHandler_EmptyPayloadAllowed(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	body := bytes.NewBufferString(`{"payload": {}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

// If-Match 헤더 잘못된 형식이면 unconditional 으로 fallback
func TestDashboardHandler_InvalidIfMatch_FallsBackUnconditional(t *testing.T) {
	router, _ := setupDashboardRouter(t)

	body := strings.NewReader(`{"payload": {"v": 1}}`)
	rec := requestWithAuth(t, router, "PUT", "/api/v1/dashboards/mine", "alice", "editor", body, "not-a-number")
	assert.Equal(t, http.StatusOK, rec.Code, "잘못된 If-Match 는 unconditional 로 처리")
}
