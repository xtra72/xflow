// @SPEC:SPEC-DASHBOARD-004 (M4)
// dashboard_test.go — 대시보드 1급 엔티티 API 통합 테스트.
//
// 검증 범위 (acceptance.md):
//   - AC-03: viewer 생성 차단 / editor 생성 성공 / 403 이 권한 키를 노출하지 않음
//   - AC-04: private 대시보드의 타인 접근 차단 (403, 목록 제외, PUT 시 version 불변)
//   - AC-05: ACL 경유 조회 성공 (view → 읽기만, edit 로 승격 시 저장 가능)
//   - AC-06: ACL 미등재 차단 / 전량 치환으로 회수
//   - AC-07: role: subject 경유 접근 + 요청 시점 권한 조회 + 최대 레벨 승리
//   - AC-08: 소유자라도 dashboard.delete 없으면 403 / 관리자 삭제 시 ACL CASCADE
//   - AC-11: If-Match 낙관적 동시성 (409 시 서버 상태 불변)
//   - AC-12: 읽기 전용 shim 형상 + 리터럴 라우트 우선 + 쓰기 라우트 404
//   - AC-14/AC-21: 사용자 UI 상태 분리 + 존재하지 않는 활성 uid 정규화
//   - AC-16: 수용된 회귀와 완화책 (역할 권한 부여가 기존 토큰에 즉시 반영)
//   - AC-17: ACL 유효성 검증 표 전수
//   - AC-18: 인증 비활성 회귀 없음
//   - AC-19/AC-20: 잠금 방지 불변식 2종
//   - 엣지: 이름 규칙, 256KB 초과 413, 잘못된 JSON 400, 없는 uid 404

package handler

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
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

// -----------------------------------------------------------------------------
// Test fixture
// -----------------------------------------------------------------------------

// dashboardEnv 는 acceptance.md 공통 픽스처를 재현한 테스트 환경이다.
//
//	root : admin    (dashboard read/create/update/delete)
//	edi  : editor   (dashboard read/create/update)
//	vie  : viewer   (dashboard read)
//	ops  : operator (커스텀 — dashboard read/update)
type dashboardEnv struct {
	router *api.Router
	db     *sql.DB
	repo   storage.DashboardRepository
	acl    storage.DashboardACLRepository
	state  storage.DashboardUserStateRepository
	perms  *auth.PermissionCache
}

// newDashboardEnv 는 스키마·사용자·역할을 시드한 환경을 구성한다.
//
// api.Authorization 을 글로벌 미들웨어로 등록하는 이유는, 전역 RBAC(권한 미들웨어)와
// 대시보드 단위 인가(dashboardacl.Evaluate)가 **함께** 작동해야 acceptance.md 의
// 상태 코드가 재현되기 때문이다. 미들웨어를 빼면 viewer 의 403(AC-03)이 관측되지 않는다.
func newDashboardEnv(t *testing.T, authEnabled bool) *dashboardEnv {
	t.Helper()
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "dashboard.db")
	db, err := storage.OpenSQLiteDB(ctx, dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })

	// 커스텀 역할 operator — acceptance.md 공통 픽스처.
	require.NoError(t, storage.InsertRole(ctx, db, "operator", "테스트용 운영자",
		[]string{"dashboard.read", "dashboard.update"}))

	for _, u := range []struct{ name, role string }{
		{"root", "admin"}, {"edi", "editor"}, {"vie", "viewer"}, {"ops", "operator"},
	} {
		require.NoError(t, storage.InsertUser(ctx, db, u.name, "hash-"+u.name, u.role, 0, 0))
	}

	repo, err := storage.NewDashboardEntitySQLiteRepository(ctx, db)
	require.NoError(t, err)
	aclRepo, err := storage.NewDashboardACLSQLiteRepository(ctx, db)
	require.NoError(t, err)
	stateRepo, err := storage.NewDashboardUserStateSQLiteRepository(ctx, db)
	require.NoError(t, err)

	resolver := auth.NewSQLPermissionResolver(db)
	permCache := auth.NewPermissionCache(resolver).WithUserRoles(resolver)

	router := api.NewRouter()
	router.Use(api.Authorization(authEnabled, permCache, nil))
	g := router.Group("/api/v1")

	NewDashboardHandler(repo, aclRepo, stateRepo, nil, nil).
		WithPermissions(permCache).
		WithAuthEnabled(authEnabled).
		WithSubjectDB(db).
		RegisterRoutes(g)

	NewUserHandler(db, permCache, nil).RegisterRoutes(g)
	NewRoleHandler(db, permCache, nil).RegisterRoutes(g)

	return &dashboardEnv{router: router, db: db, repo: repo, acl: aclRepo, state: stateRepo, perms: permCache}
}

// 테스트 사용자 → 역할 매핑 (요청 시 컨텍스트 주입용).
var testUserRoles = map[string]string{
	"root": "admin", "edi": "editor", "vie": "viewer", "ops": "operator",
}

// do 는 픽스처 사용자로 요청을 보낸다.
func (e *dashboardEnv) do(t *testing.T, method, path, user string, body io.Reader) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithAuth(t, e.router, method, path, user, testUserRoles[user], body, "")
}

// doIfMatch 는 If-Match 헤더를 실어 요청을 보낸다.
func (e *dashboardEnv) doIfMatch(t *testing.T, method, path, user string, body io.Reader, ifMatch string) *httptest.ResponseRecorder {
	t.Helper()
	return requestWithAuth(t, e.router, method, path, user, testUserRoles[user], body, ifMatch)
}

// create 는 owner 소유의 대시보드를 만들고 uid 를 반환한다.
//
// visibility 는 생성 시 항상 private 이므로(spec.md §2.7), 다른 값이 필요하면
// 저장소를 통해 직접 조정한다 — PATCH 경로를 쓰면 테스트가 PATCH 인가에 의존한다.
func (e *dashboardEnv) create(t *testing.T, owner, name, visibility string) string {
	t.Helper()
	rec := e.do(t, http.MethodPost, "/api/v1/dashboards", owner,
		jsonBody(`{"name":`+strconv.Quote(name)+`}`))
	require.Equal(t, http.StatusCreated, rec.Code, "생성 실패: %s", rec.Body.String())

	var d dto.DashboardDetail
	decodeEnvelope(t, rec, &d)

	if visibility != "private" {
		_, err := e.repo.Update(context.Background(), d.UID,
			storage.DashboardUpdate{Visibility: &visibility}, -1)
		require.NoError(t, err)
	}
	return d.UID
}

// getDashboard 는 저장소에서 대시보드를 직접 읽는다 (서버 상태 불변 검증용).
func (e *dashboardEnv) getDashboard(t *testing.T, uid string) *storage.Dashboard {
	t.Helper()
	d, err := e.repo.Get(context.Background(), uid)
	require.NoError(t, err)
	return d
}

// -----------------------------------------------------------------------------
// Request helpers
// -----------------------------------------------------------------------------

// requestWithAuth 는 미들웨어를 우회하여 user_id / user_role 컨텍스트를 직접 주입한 후
// 라우터로 요청을 보낸다.
//
// username 이 빈 문자열이면 미인증 (UserID() 가 "" 을 반환).
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

// jsonBody 는 문자열 본문을 reader 로 감싼다.
func jsonBody(s string) io.Reader { return bytes.NewBufferString(s) }

// decodeEnvelope 는 표준 APIResponse envelope 의 data 를 dst 로 디코딩한다.
func decodeEnvelope(t *testing.T, rec *httptest.ResponseRecorder, dst any) {
	t.Helper()
	var env struct {
		Success bool            `json:"success"`
		Data    json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env),
		"envelope 디코딩 실패: %s", rec.Body.String())
	require.NoError(t, json.Unmarshal(env.Data, dst),
		"data 디코딩 실패: %s", rec.Body.String())
}

// -----------------------------------------------------------------------------
// AC-03 — 생성 권한
// -----------------------------------------------------------------------------

func TestDashboardCreate_ViewerForbidden(t *testing.T) {
	env := newDashboardEnv(t, true)

	rec := env.do(t, http.MethodPost, "/api/v1/dashboards", "vie", jsonBody(`{"name":"내 대시보드"}`))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 부족한 권한 키를 응답에 노출하지 않는다 (spec.md §5 보안).
	assert.NotContains(t, rec.Body.String(), "dashboard.create")

	items, err := env.repo.List(context.Background(), false)
	require.NoError(t, err)
	assert.Empty(t, items, "거부된 요청이 행을 만들면 안 된다")
}

func TestDashboardCreate_EditorSucceeds(t *testing.T) {
	env := newDashboardEnv(t, true)

	rec := env.do(t, http.MethodPost, "/api/v1/dashboards", "edi", jsonBody(`{"name":"내 대시보드"}`))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var d dto.DashboardDetail
	decodeEnvelope(t, rec, &d)
	assert.Equal(t, "edi", d.Owner, "owner 는 JWT username 으로 결정된다")
	assert.Equal(t, "private", d.Visibility)
	assert.EqualValues(t, 1, d.Version)
	assert.NotEmpty(t, d.UID)
}

// TestDashboardCreate_IgnoresPrivilegedBodyFields 는 UB1 #4 를 고정한다.
func TestDashboardCreate_IgnoresPrivilegedBodyFields(t *testing.T) {
	env := newDashboardEnv(t, true)

	rec := env.do(t, http.MethodPost, "/api/v1/dashboards", "edi", jsonBody(
		`{"name":"탈취","owner":"root","visibility":"shared","version":99,"uid":"shared"}`))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var d dto.DashboardDetail
	decodeEnvelope(t, rec, &d)
	assert.Equal(t, "edi", d.Owner, "본문 owner 는 무시된다")
	assert.Equal(t, "private", d.Visibility, "본문 visibility 는 무시된다")
	assert.EqualValues(t, 1, d.Version, "본문 version 은 무시된다")
	assert.NotEqual(t, "shared", d.UID, "본문 uid 는 무시되고 예약어는 발급되지 않는다")
}

func TestDashboardCreate_NameValidation(t *testing.T) {
	env := newDashboardEnv(t, true)

	cases := []struct {
		name string
		body string
		want int
	}{
		{"공백만", `{"name":"   "}`, http.StatusBadRequest},
		{"빈 문자열", `{"name":""}`, http.StatusBadRequest},
		{"65자", `{"name":"` + strings.Repeat("가", 65) + `"}`, http.StatusBadRequest},
		{"64자", `{"name":"` + strings.Repeat("가", 64) + `"}`, http.StatusCreated},
		{"잘못된 JSON", `{"name":`, http.StatusBadRequest},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec := env.do(t, http.MethodPost, "/api/v1/dashboards", "root", jsonBody(c.body))
			assert.Equal(t, c.want, rec.Code, rec.Body.String())
		})
	}
}

// TestDashboardCreate_DuplicateNameAllowed 는 식별자가 uid 임을 고정한다.
func TestDashboardCreate_DuplicateNameAllowed(t *testing.T) {
	env := newDashboardEnv(t, true)
	a := env.create(t, "edi", "같은 이름", "private")
	b := env.create(t, "edi", "같은 이름", "private")
	assert.NotEqual(t, a, b)
}

// -----------------------------------------------------------------------------
// AC-04 — private 대시보드의 타인 접근 차단
// -----------------------------------------------------------------------------

func TestDashboardPrivate_OtherUserForbidden(t *testing.T) {
	env := newDashboardEnv(t, true)
	d1 := env.create(t, "edi", "edi 개인", "private")

	// 조회 → 403 (404 가 아니다 — spec.md §2.13 각주).
	rec := env.do(t, http.MethodGet, "/api/v1/dashboards/"+d1, "ops", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())

	// 목록에서 제외.
	rec = env.do(t, http.MethodGet, "/api/v1/dashboards", "ops", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var list []dto.DashboardMeta
	decodeEnvelope(t, rec, &list)
	for _, it := range list {
		assert.NotEqual(t, d1, it.UID, "타인의 private 대시보드가 목록에 노출되면 안 된다")
	}

	// 저장 시도 → 403, version 불변.
	before := env.getDashboard(t, d1).Version
	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+d1, "ops",
		jsonBody(`{"payload":{"panels":[1]}}`))
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Equal(t, before, env.getDashboard(t, d1).Version, "거부된 저장이 version 을 올리면 안 된다")
}

func TestDashboardGet_UnknownUIDNotFound(t *testing.T) {
	env := newDashboardEnv(t, true)
	rec := env.do(t, http.MethodGet, "/api/v1/dashboards/no-such-uid", "root", nil)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// -----------------------------------------------------------------------------
// AC-05 / AC-06 — ACL 경유 접근
// -----------------------------------------------------------------------------

func TestDashboardACL_ViewThenEdit(t *testing.T) {
	env := newDashboardEnv(t, true)
	d2 := env.create(t, "edi", "acl 대시보드", "acl")

	// view 부여.
	rec := env.do(t, http.MethodPut, "/api/v1/dashboards/"+d2+"/acl", "edi",
		jsonBody(`[{"subject":"user:ops","level":"view"}]`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// ops 조회 성공 + payload 포함.
	rec = env.do(t, http.MethodGet, "/api/v1/dashboards/"+d2, "ops", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var detail dto.DashboardDetail
	decodeEnvelope(t, rec, &detail)
	assert.NotEmpty(t, detail.Payload, "단건 응답은 payload 를 포함한다")

	// 목록의 판정 플래그.
	rec = env.do(t, http.MethodGet, "/api/v1/dashboards", "ops", nil)
	var list []dto.DashboardMeta
	decodeEnvelope(t, rec, &list)
	require.Len(t, list, 1)
	assert.False(t, list[0].CanEdit)
	assert.False(t, list[0].CanDelete)
	assert.False(t, list[0].CanGrant)
	assert.Empty(t, listPayloadKeys(t, rec), "목록 응답은 payload 를 포함하지 않는다")

	// view 만 있으면 저장 불가.
	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+d2, "ops", jsonBody(`{"payload":{}}`))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// edit 으로 승격하면 저장 가능 (ops 는 dashboard.update 보유).
	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+d2+"/acl", "edi",
		jsonBody(`[{"subject":"user:ops","level":"edit"}]`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+d2, "ops", jsonBody(`{"payload":{"panels":[]}}`))
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// granted_by 는 요청자로 기록된다.
	rows, err := env.acl.ListByDashboard(context.Background(), env.getDashboard(t, d2).ID)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "user:ops", rows[0].Subject)
	assert.Equal(t, "edit", rows[0].Level)
	assert.Equal(t, "edi", rows[0].GrantedBy)
}

func TestDashboardACL_NotListedForbiddenAndRevocable(t *testing.T) {
	env := newDashboardEnv(t, true)
	d2 := env.create(t, "edi", "acl 대시보드", "acl")

	rec := env.do(t, http.MethodPut, "/api/v1/dashboards/"+d2+"/acl", "edi",
		jsonBody(`[{"subject":"user:ops","level":"view"}]`))
	require.Equal(t, http.StatusOK, rec.Code)

	// 미등재 사용자 → 403.
	rec = env.do(t, http.MethodGet, "/api/v1/dashboards/"+d2, "vie", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 전량 치환으로 회수하면 기존 등재자도 403.
	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+d2+"/acl", "edi", jsonBody(`[]`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = env.do(t, http.MethodGet, "/api/v1/dashboards/"+d2, "ops", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)
}

// TestDashboardACL_RoleSubject 는 role: subject 경유 접근과 요청 시점 권한 조회를
// 고정한다 (AC-07).
func TestDashboardACL_RoleSubject(t *testing.T) {
	env := newDashboardEnv(t, true)
	d3 := env.create(t, "edi", "role acl", "acl")

	rec := env.do(t, http.MethodPut, "/api/v1/dashboards/"+d3+"/acl", "edi",
		jsonBody(`[{"subject":"role:operator","level":"edit"}]`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// operator 역할 보유자는 접근 가능하고 can_edit 이다.
	rec = env.do(t, http.MethodGet, "/api/v1/dashboards", "ops", nil)
	var list []dto.DashboardMeta
	decodeEnvelope(t, rec, &list)
	require.Len(t, list, 1)
	assert.True(t, list[0].CanEdit)

	// 역할이 다른 사용자는 403.
	rec = env.do(t, http.MethodGet, "/api/v1/dashboards/"+d3, "vie", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 관리자가 ops 의 역할을 viewer 로 바꾸면 **기존 토큰 그대로** 403 이 된다
	// (권한은 요청 시점 조회 — SPEC-AUTH-005 §4.3).
	rec = env.do(t, http.MethodPut, "/api/v1/users/ops", "root", jsonBody(`{"role":"viewer"}`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = requestWithAuth(t, env.router, http.MethodGet, "/api/v1/dashboards/"+d3, "ops", "operator", nil, "")
	assert.Equal(t, http.StatusForbidden, rec.Code, "토큰의 옛 역할을 신뢰하면 안 된다")
}

// TestDashboardACL_MaxLevelWins 는 user: 와 role: 이 동시에 매치되면 높은 레벨이
// 이김을 고정한다 (AC-07 마지막 절).
func TestDashboardACL_MaxLevelWins(t *testing.T) {
	env := newDashboardEnv(t, true)
	d := env.create(t, "edi", "max level", "acl")

	rec := env.do(t, http.MethodPut, "/api/v1/dashboards/"+d+"/acl", "edi", jsonBody(
		`[{"subject":"user:ops","level":"view"},{"subject":"role:operator","level":"edit"}]`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+d, "ops", jsonBody(`{"payload":{}}`))
	assert.Equal(t, http.StatusOK, rec.Code, "edit 이 view 를 이겨야 한다: %s", rec.Body.String())
}

// -----------------------------------------------------------------------------
// AC-17 — ACL 유효성 검증
// -----------------------------------------------------------------------------

func TestDashboardACL_Validation(t *testing.T) {
	env := newDashboardEnv(t, true)
	d9 := env.create(t, "root", "acl 검증", "acl")
	path := "/api/v1/dashboards/" + d9 + "/acl"

	cases := []struct {
		name string
		body string
		want int
	}{
		{"user 정상", `[{"subject":"user:edi","level":"view"}]`, http.StatusOK},
		{"role 정상", `[{"subject":"role:operator","level":"edit"}]`, http.StatusOK},
		{"접두사 없음", `[{"subject":"edi","level":"view"}]`, http.StatusBadRequest},
		{"미지원 접두사", `[{"subject":"group:dev","level":"view"}]`, http.StatusBadRequest},
		{"없는 사용자", `[{"subject":"user:nobody","level":"view"}]`, http.StatusBadRequest},
		{"없는 역할", `[{"subject":"role:nosuchrole","level":"view"}]`, http.StatusBadRequest},
		{"미지원 레벨", `[{"subject":"user:edi","level":"manage"}]`, http.StatusBadRequest},
		{"소유자 자신", `[{"subject":"user:root","level":"view"}]`, http.StatusBadRequest},
		{"중복 subject", `[{"subject":"user:edi","level":"view"},{"subject":"user:edi","level":"edit"}]`, http.StatusBadRequest},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// 실패 시 ACL 이 전혀 변경되지 않아야 하므로 사전 상태를 고정한다.
			require.Equal(t, http.StatusOK,
				env.do(t, http.MethodPut, path, "root", jsonBody(`[{"subject":"user:ops","level":"view"}]`)).Code)

			rec := env.do(t, http.MethodPut, path, "root", jsonBody(c.body))
			assert.Equal(t, c.want, rec.Code, rec.Body.String())

			if c.want != http.StatusOK {
				rows, err := env.acl.ListByDashboard(context.Background(), env.getDashboard(t, d9).ID)
				require.NoError(t, err)
				require.Len(t, rows, 1, "거부된 요청이 ACL 을 바꾸면 안 된다")
				assert.Equal(t, "user:ops", rows[0].Subject)
			}
		})
	}
}

// TestDashboardACL_NonOwnerForbidden 은 grant 인가 없는 사용자의 ACL 접근을 고정한다.
func TestDashboardACL_NonOwnerForbidden(t *testing.T) {
	env := newDashboardEnv(t, true)
	d9 := env.create(t, "root", "acl 검증", "shared")

	rec := env.do(t, http.MethodPut, "/api/v1/dashboards/"+d9+"/acl", "edi",
		jsonBody(`[{"subject":"user:ops","level":"view"}]`))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	rec = env.do(t, http.MethodGet, "/api/v1/dashboards/"+d9+"/acl", "edi", nil)
	assert.Equal(t, http.StatusForbidden, rec.Code, "GET /acl 도 grant 인가를 요구한다")

	rec = env.do(t, http.MethodGet, "/api/v1/dashboards/"+d9+"/acl", "root", nil)
	assert.Equal(t, http.StatusOK, rec.Code)
}

// -----------------------------------------------------------------------------
// AC-08 — 삭제
// -----------------------------------------------------------------------------

func TestDashboardDelete(t *testing.T) {
	env := newDashboardEnv(t, true)

	t.Run("소유자라도 dashboard.delete 가 없으면 403", func(t *testing.T) {
		d4 := env.create(t, "edi", "edi 소유", "private")
		rec := env.do(t, http.MethodDelete, "/api/v1/dashboards/"+d4, "edi", nil)
		assert.Equal(t, http.StatusForbidden, rec.Code, "전역 권한이 상한이다(spec.md §4.3)")
		_, err := env.repo.Get(context.Background(), d4)
		assert.NoError(t, err, "거부된 삭제가 행을 지우면 안 된다")
	})

	t.Run("소유자 + dashboard.delete → 204 와 ACL CASCADE", func(t *testing.T) {
		d5 := env.create(t, "root", "root 소유", "acl")
		require.Equal(t, http.StatusOK, env.do(t, http.MethodPut,
			"/api/v1/dashboards/"+d5+"/acl", "root",
			jsonBody(`[{"subject":"user:ops","level":"view"}]`)).Code)
		id := env.getDashboard(t, d5).ID

		rec := env.do(t, http.MethodDelete, "/api/v1/dashboards/"+d5, "root", nil)
		require.Equal(t, http.StatusNoContent, rec.Code)

		_, err := env.repo.Get(context.Background(), d5)
		assert.ErrorIs(t, err, storage.ErrDashboardNotFound)

		rows, err := env.acl.ListByDashboard(context.Background(), id)
		require.NoError(t, err)
		assert.Empty(t, rows, "ACL 도 함께 사라져야 한다")
	})

	t.Run("shared 대시보드도 비소유자는 삭제 불가", func(t *testing.T) {
		d6 := env.create(t, "root", "공유", "shared")
		for _, u := range []string{"edi", "ops"} {
			rec := env.do(t, http.MethodDelete, "/api/v1/dashboards/"+d6, u, nil)
			assert.Equal(t, http.StatusForbidden, rec.Code, "user=%s", u)
		}
		_, err := env.repo.Get(context.Background(), d6)
		assert.NoError(t, err)
	})
}

// -----------------------------------------------------------------------------
// AC-11 — If-Match 낙관적 동시성
// -----------------------------------------------------------------------------

func TestDashboardPut_IfMatch(t *testing.T) {
	env := newDashboardEnv(t, true)
	d7 := env.create(t, "root", "동시성", "private")
	path := "/api/v1/dashboards/" + d7

	// version 을 5 로 올린다 (생성 시 1).
	for env.getDashboard(t, d7).Version < 5 {
		require.Equal(t, http.StatusOK,
			env.do(t, http.MethodPut, path, "root", jsonBody(`{"payload":{"panels":[]}}`)).Code)
	}
	require.EqualValues(t, 5, env.getDashboard(t, d7).Version)

	// 불일치 → 409, 서버 상태 불변.
	rec := env.doIfMatch(t, http.MethodPut, path, "root", jsonBody(`{"payload":{"panels":[1]}}`), "4")
	assert.Equal(t, http.StatusConflict, rec.Code)
	assert.EqualValues(t, 5, env.getDashboard(t, d7).Version, "409 는 서버 상태를 바꾸지 않는다")

	// 409 본문에 서버측 최신 상태가 실려야 한다.
	var latest dto.DashboardDetail
	decodeEnvelope(t, rec, &latest)
	assert.EqualValues(t, 5, latest.Version)

	// 일치 → 200, version 6.
	rec = env.doIfMatch(t, http.MethodPut, path, "root", jsonBody(`{"payload":{"panels":[1]}}`), "5")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.EqualValues(t, 6, env.getDashboard(t, d7).Version)

	// 헤더 없음 → 무조건 저장 (기존 정책 승계).
	rec = env.do(t, http.MethodPut, path, "root", jsonBody(`{"payload":{}}`))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.EqualValues(t, 7, env.getDashboard(t, d7).Version)

	// 숫자가 아닌 If-Match → unconditional (기존 parseIfMatch 동작 승계).
	rec = env.doIfMatch(t, http.MethodPut, path, "root", jsonBody(`{"payload":{}}`), "not-a-number")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.EqualValues(t, 8, env.getDashboard(t, d7).Version)
}

// TestDashboardPut_PayloadTooLarge 는 UB1 #9 를 고정한다.
func TestDashboardPut_PayloadTooLarge(t *testing.T) {
	env := newDashboardEnv(t, true)
	uid := env.create(t, "root", "크기 상한", "private")
	before := env.getDashboard(t, uid).Version

	huge := `{"payload":{"blob":"` + strings.Repeat("x", 256*1024) + `"}}`
	rec := env.do(t, http.MethodPut, "/api/v1/dashboards/"+uid, "root", jsonBody(huge))
	assert.Equal(t, http.StatusRequestEntityTooLarge, rec.Code)
	assert.Equal(t, before, env.getDashboard(t, uid).Version, "413 은 저장하지 않는다")
}

// -----------------------------------------------------------------------------
// PATCH — 이름(edit) / 공개범위·기본(grant)
// -----------------------------------------------------------------------------

func TestDashboardPatch(t *testing.T) {
	env := newDashboardEnv(t, true)

	t.Run("소유자는 이름과 공개범위를 바꾼다", func(t *testing.T) {
		uid := env.create(t, "root", "이전 이름", "private")
		rec := env.do(t, http.MethodPatch, "/api/v1/dashboards/"+uid, "root",
			jsonBody(`{"name":"새 이름","visibility":"shared","is_default":true}`))
		require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

		var d dto.DashboardDetail
		decodeEnvelope(t, rec, &d)
		assert.Equal(t, "새 이름", d.Name)
		assert.Equal(t, "shared", d.Visibility)
		assert.True(t, d.IsDefault)
	})

	t.Run("edit 만 가진 사용자는 공개범위를 바꿀 수 없다", func(t *testing.T) {
		uid := env.create(t, "edi", "acl 대상", "acl")
		require.Equal(t, http.StatusOK, env.do(t, http.MethodPut,
			"/api/v1/dashboards/"+uid+"/acl", "edi",
			jsonBody(`[{"subject":"user:ops","level":"edit"}]`)).Code)

		rec := env.do(t, http.MethodPatch, "/api/v1/dashboards/"+uid, "ops", jsonBody(`{"name":"ops 가 바꿈"}`))
		assert.Equal(t, http.StatusOK, rec.Code, "이름 변경은 edit 인가로 충분하다: %s", rec.Body.String())

		rec = env.do(t, http.MethodPatch, "/api/v1/dashboards/"+uid, "ops", jsonBody(`{"visibility":"shared"}`))
		assert.Equal(t, http.StatusForbidden, rec.Code, "공개범위 변경은 grant 인가를 요구한다")
		assert.Equal(t, "acl", env.getDashboard(t, uid).Visibility)
	})

	t.Run("잘못된 visibility 는 400", func(t *testing.T) {
		uid := env.create(t, "root", "검증", "private")
		rec := env.do(t, http.MethodPatch, "/api/v1/dashboards/"+uid, "root", jsonBody(`{"visibility":"public"}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("변경 필드가 없으면 400", func(t *testing.T) {
		uid := env.create(t, "root", "검증", "private")
		rec := env.do(t, http.MethodPatch, "/api/v1/dashboards/"+uid, "root", jsonBody(`{}`))
		assert.Equal(t, http.StatusBadRequest, rec.Code)
	})

	t.Run("If-Match 불일치는 409", func(t *testing.T) {
		uid := env.create(t, "root", "검증", "private")
		rec := env.doIfMatch(t, http.MethodPatch, "/api/v1/dashboards/"+uid, "root",
			jsonBody(`{"name":"바뀜"}`), "99")
		assert.Equal(t, http.StatusConflict, rec.Code)
		assert.Equal(t, "검증", env.getDashboard(t, uid).Name)
	})
}

// -----------------------------------------------------------------------------
// AC-12 — 읽기 전용 shim
// -----------------------------------------------------------------------------

func TestDashboardSharedShim_LegacyShape(t *testing.T) {
	env := newDashboardEnv(t, true)
	env.create(t, "root", "공유 1", "shared")
	env.create(t, "root", "공유 2", "shared")
	env.create(t, "edi", "개인", "private")

	rec := env.do(t, http.MethodGet, "/api/v1/dashboards/shared", "root", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 레거시 DTO 로 그대로 역직렬화되어야 한다 (원격 프록시 소비 형상).
	var snap dto.DashboardSnapshot
	decodeEnvelope(t, rec, &snap)
	assert.Equal(t, "global", snap.Scope)
	assert.Nil(t, snap.Owner, "global 은 owner=null")
	assert.NotZero(t, snap.UpdatedAt)

	// 필드 존재 자체를 raw 로도 확인한다 (키 이름 회귀 방지).
	raw := rawData(t, rec)
	for _, key := range []string{"scope", "owner", "version", "updatedAt", "payload"} {
		assert.Contains(t, raw, key, "레거시 키 %s 가 사라지면 원격 프록시가 깨진다", key)
	}

	var payload struct {
		DashboardPages []struct {
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			IsDefault bool            `json:"isDefault"`
			Panels    json.RawMessage `json:"panels"`
			Layout    json.RawMessage `json:"layout"`
		} `json:"dashboardPages"`
		ActiveDashboardID string          `json:"activeDashboardId"`
		DeviceGridLayout  json.RawMessage `json:"deviceGridLayout"`
	}
	require.NoError(t, json.Unmarshal(snap.Payload, &payload))
	require.Len(t, payload.DashboardPages, 2, "private 대시보드는 shared shim 에 포함되지 않는다")
	// 생성 시 소유자별 sort_order 가 부여되므로 생성 순서가 그대로 유지된다
	// (uid 는 UUID 라 sort_order 가 전부 0 이면 순서가 임의로 뒤섞인다).
	assert.Equal(t, []string{"공유 1", "공유 2"},
		[]string{payload.DashboardPages[0].Name, payload.DashboardPages[1].Name})
	assert.JSONEq(t, `[]`, string(payload.DashboardPages[0].Panels))
	assert.JSONEq(t, `{}`, string(payload.DeviceGridLayout))
}

func TestDashboardMineShim(t *testing.T) {
	env := newDashboardEnv(t, true)
	env.create(t, "edi", "edi 개인", "private")
	env.create(t, "root", "root 개인", "private")
	env.create(t, "root", "공유", "shared")

	rec := env.do(t, http.MethodGet, "/api/v1/dashboards/mine", "edi", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var snap dto.DashboardSnapshot
	decodeEnvelope(t, rec, &snap)
	assert.Equal(t, "user", snap.Scope)
	require.NotNil(t, snap.Owner)
	assert.Equal(t, "edi", *snap.Owner)

	var payload struct {
		DashboardPages []struct {
			Name string `json:"name"`
		} `json:"dashboardPages"`
	}
	require.NoError(t, json.Unmarshal(snap.Payload, &payload))
	require.Len(t, payload.DashboardPages, 1, "본인 소유 private 만 담긴다")
	assert.Equal(t, "edi 개인", payload.DashboardPages[0].Name)
}

// TestDashboardShim_VersionIsMaxOfIncluded 는 version/updatedAt 이 포함된 행들의
// 최댓값임을 고정한다 (spec.md §2.3 shim 표).
func TestDashboardShim_VersionIsMaxOfIncluded(t *testing.T) {
	env := newDashboardEnv(t, true)
	a := env.create(t, "root", "A", "shared")
	env.create(t, "root", "B", "shared")

	for i := 0; i < 3; i++ {
		require.Equal(t, http.StatusOK, env.do(t, http.MethodPut,
			"/api/v1/dashboards/"+a, "root", jsonBody(`{"payload":{}}`)).Code)
	}
	want := env.getDashboard(t, a).Version

	rec := env.do(t, http.MethodGet, "/api/v1/dashboards/shared", "root", nil)
	var snap dto.DashboardSnapshot
	decodeEnvelope(t, rec, &snap)
	assert.Equal(t, want, snap.Version)
}

// TestDashboardShim_WriteRoutesAreGone 은 PUT/DELETE 가 405 가 아니라 404 임을
// 고정한다 (라우트 미등록 — spec.md §2.3).
func TestDashboardShim_WriteRoutesAreGone(t *testing.T) {
	env := newDashboardEnv(t, true)

	cases := []struct{ method, path string }{
		{http.MethodPut, "/api/v1/dashboards/shared"},
		{http.MethodDelete, "/api/v1/dashboards/shared"},
		{http.MethodPut, "/api/v1/dashboards/mine"},
		{http.MethodDelete, "/api/v1/dashboards/mine"},
	}
	for _, c := range cases {
		t.Run(c.method+" "+c.path, func(t *testing.T) {
			rec := env.do(t, c.method, c.path, "root", jsonBody(`{}`))
			assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
		})
	}
}

// TestDashboardShim_LiteralRoutePrecedence 는 리터럴 세그먼트가 {uid} 보다 먼저
// 매칭됨을 고정한다. 이 순서가 깨지면 관리 API 가 shim 으로 라우팅된다.
func TestDashboardShim_LiteralRoutePrecedence(t *testing.T) {
	env := newDashboardEnv(t, true)
	env.create(t, "root", "공유", "shared")

	// {uid} 핸들러였다면 uid="shared" 를 찾지 못해 404 였을 것이다.
	rec := env.do(t, http.MethodGet, "/api/v1/dashboards/shared", "root", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var snap dto.DashboardSnapshot
	decodeEnvelope(t, rec, &snap)
	assert.Equal(t, "global", snap.Scope, "shim 핸들러가 응답해야 한다")

	rec = env.do(t, http.MethodGet, "/api/v1/dashboards/mine", "root", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	decodeEnvelope(t, rec, &snap)
	assert.Equal(t, "user", snap.Scope)
}

// -----------------------------------------------------------------------------
// AC-14 / AC-21 — 사용자 UI 상태
// -----------------------------------------------------------------------------

func TestDashboardState(t *testing.T) {
	env := newDashboardEnv(t, true)
	uid := env.create(t, "root", "활성", "private")

	// 최초 조회는 404 가 아니라 기본값이다.
	rec := env.do(t, http.MethodGet, "/api/v1/dashboard-state", "root", nil)
	require.Equal(t, http.StatusOK, rec.Code)
	var st dto.DashboardUserStateResponse
	decodeEnvelope(t, rec, &st)
	assert.Empty(t, st.ActiveDashboardUID)
	assert.EqualValues(t, 0, st.Version)

	// 저장.
	rec = env.do(t, http.MethodPut, "/api/v1/dashboard-state", "root",
		jsonBody(`{"active_dashboard_uid":"`+uid+`","device_grid_layout":{"dev-1":{"x":1}}}`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	decodeEnvelope(t, rec, &st)
	assert.Equal(t, uid, st.ActiveDashboardUID)
	assert.EqualValues(t, 1, st.Version)

	// 사용자별로 분리된다.
	rec = env.do(t, http.MethodGet, "/api/v1/dashboard-state", "edi", nil)
	decodeEnvelope(t, rec, &st)
	assert.Empty(t, st.ActiveDashboardUID, "타 사용자의 활성 대시보드가 새어나가면 안 된다")
}

// TestDashboardState_NormalizesDeletedActiveUID 는 UB1 #11 을 고정한다.
func TestDashboardState_NormalizesDeletedActiveUID(t *testing.T) {
	env := newDashboardEnv(t, true)
	uid := env.create(t, "root", "곧 삭제", "private")

	require.Equal(t, http.StatusOK, env.do(t, http.MethodPut, "/api/v1/dashboard-state", "root",
		jsonBody(`{"active_dashboard_uid":"`+uid+`"}`)).Code)
	require.Equal(t, http.StatusNoContent,
		env.do(t, http.MethodDelete, "/api/v1/dashboards/"+uid, "root", nil).Code)

	rec := env.do(t, http.MethodGet, "/api/v1/dashboard-state", "root", nil)
	require.Equal(t, http.StatusOK, rec.Code, "400 이 아니라 정규화된 200 이어야 한다")
	var st dto.DashboardUserStateResponse
	decodeEnvelope(t, rec, &st)
	assert.Empty(t, st.ActiveDashboardUID)

	// 저장 시에도 존재하지 않는 uid 는 빈 문자열로 정규화된다.
	rec = env.do(t, http.MethodPut, "/api/v1/dashboard-state", "root",
		jsonBody(`{"active_dashboard_uid":"ghost"}`))
	require.Equal(t, http.StatusOK, rec.Code)
	decodeEnvelope(t, rec, &st)
	assert.Empty(t, st.ActiveDashboardUID)
}

// -----------------------------------------------------------------------------
// AC-16 — 수용된 회귀와 완화책
// -----------------------------------------------------------------------------

func TestDashboardViewerRegressionAndMitigation(t *testing.T) {
	env := newDashboardEnv(t, true)

	// viewer 소유 대시보드는 마이그레이션으로만 생길 수 있으므로 저장소로 직접 만든다.
	created, err := env.repo.Create(context.Background(), storage.Dashboard{
		UID: "D8", Name: "viewer 개인", Owner: "vie", Visibility: "private",
		Payload: []byte(`{"panels":[],"layout":[]}`),
	})
	require.NoError(t, err)

	// 조회는 가능하다 (데이터는 삭제되지 않는다).
	rec := env.do(t, http.MethodGet, "/api/v1/dashboards/"+created.UID, "vie", nil)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// 저장은 불가 (dashboard.update 미보유 — 전역 권한이 상한).
	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+created.UID, "vie", jsonBody(`{"payload":{}}`))
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 완화책: 관리자가 viewer 역할에 권한을 부여하면 **기존 토큰 그대로** 저장된다.
	rec = env.do(t, http.MethodPut, "/api/v1/roles/viewer", "root", jsonBody(
		`{"permissions":["dashboard.read","dashboard.create","dashboard.update"]}`))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	rec = env.do(t, http.MethodPut, "/api/v1/dashboards/"+created.UID, "vie", jsonBody(`{"payload":{}}`))
	assert.Equal(t, http.StatusOK, rec.Code, "재로그인 없이 즉시 반영되어야 한다: %s", rec.Body.String())
}

// -----------------------------------------------------------------------------
// AC-18 — 인증 비활성
// -----------------------------------------------------------------------------

func TestDashboardAuthDisabled(t *testing.T) {
	env := newDashboardEnv(t, false)

	// 토큰 없이 생성.
	rec := requestWithAuth(t, env.router, http.MethodPost, "/api/v1/dashboards", "", "",
		jsonBody(`{"name":"무인증"}`), "")
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var d dto.DashboardDetail
	decodeEnvelope(t, rec, &d)
	assert.Empty(t, d.Owner, "인증 비활성 배포의 owner 는 빈 문자열이다")

	// 목록의 모든 항목이 4종 허용.
	rec = requestWithAuth(t, env.router, http.MethodGet, "/api/v1/dashboards", "", "", nil, "")
	require.Equal(t, http.StatusOK, rec.Code)
	var list []dto.DashboardMeta
	decodeEnvelope(t, rec, &list)
	require.Len(t, list, 1)
	assert.True(t, list[0].CanEdit)
	assert.True(t, list[0].CanDelete)
	assert.True(t, list[0].CanGrant)

	// 삭제도 통과.
	rec = requestWithAuth(t, env.router, http.MethodDelete, "/api/v1/dashboards/"+d.UID, "", "", nil, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
}

// TestDashboardRequiresAuthentication 은 인증 활성 상태에서 미인증 요청이 통과하지
// 않음을 고정한다.
//
// 운영에서는 api.Auth 미들웨어가 먼저 401 을 낸다. 본 테스트는 그 미들웨어를 우회해
// 컨텍스트를 직접 주입하므로, 권한 미들웨어(RequirePermission)가 먼저 403 으로 막는다.
// 어느 쪽이든 미인증 요청이 데이터에 닿지 않는다는 점이 고정 대상이다.
func TestDashboardRequiresAuthentication(t *testing.T) {
	env := newDashboardEnv(t, true)
	rec := requestWithAuth(t, env.router, http.MethodGet, "/api/v1/dashboards", "", "", nil, "")
	assert.Contains(t, []int{http.StatusUnauthorized, http.StatusForbidden}, rec.Code)

	// 권한 미들웨어가 없는 경로(/dashboard-state)는 핸들러가 직접 401 을 낸다.
	rec = requestWithAuth(t, env.router, http.MethodGet, "/api/v1/dashboard-state", "", "", nil, "")
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// -----------------------------------------------------------------------------
// AC-19 / AC-20 — 잠금 방지 불변식
// -----------------------------------------------------------------------------

// TestLockout_LastDashboardDeleter 는 dashboard.delete 보유자가 0 이 되는 변경을
// 거부함을 고정한다 (UB1 #6).
func TestLockout_LastDashboardDeleter(t *testing.T) {
	env := newDashboardEnv(t, true)

	// root 만 dashboard.delete 를 보유한다 (admin).
	t.Run("역할 강등", func(t *testing.T) {
		rec := env.do(t, http.MethodPut, "/api/v1/users/root", "root", jsonBody(`{"role":"editor"}`))
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	})

	t.Run("사용자 삭제 — 관리 권한 보유자 소실", func(t *testing.T) {
		rec := env.do(t, http.MethodDelete, "/api/v1/users/root", "root", nil)
		assert.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	})

	t.Run("사용자 삭제 — dashboard.delete 만 0 이 되는 경우", func(t *testing.T) {
		// ops 에게 관리 권한은 주되 dashboard.delete 는 주지 않는다. 그러면 user.delete /
		// role.update 보유자는 남지만 dashboard.delete 보유자만 0 이 되어, 신규 불변식이
		// 단독으로 관측된다.
		require.Equal(t, http.StatusOK, env.do(t, http.MethodPut, "/api/v1/roles/operator", "root",
			jsonBody(`{"permissions":["dashboard.read","dashboard.update","user.read","user.delete","role.update"]}`)).Code)

		rec := env.do(t, http.MethodDelete, "/api/v1/users/root", "ops", nil)
		require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
		assert.Contains(t, rec.Body.String(), "LAST_DASHBOARD_DELETER")

		// 원상 복구 — 후속 하위 테스트가 영향을 받지 않도록.
		require.Equal(t, http.StatusOK, env.do(t, http.MethodPut, "/api/v1/roles/operator", "root",
			jsonBody(`{"permissions":["dashboard.read","dashboard.update"]}`)).Code)
	})

	t.Run("역할에서 권한 제거", func(t *testing.T) {
		// admin 역할 자체는 수정 불가이므로, operator 에게 delete 를 준 뒤 회수해 본다.
		require.Equal(t, http.StatusOK, env.do(t, http.MethodPut, "/api/v1/roles/operator", "root",
			jsonBody(`{"permissions":["dashboard.read","dashboard.update","dashboard.delete"]}`)).Code)
		// 이제 root 를 editor 로 강등해도 ops 가 남으므로 통과한다.
		require.Equal(t, http.StatusOK, env.do(t, http.MethodPut, "/api/v1/users/root", "root",
			jsonBody(`{"role":"admin"}`)).Code)

		// ops 가 유일한 보유자가 되도록 admin 사용자를 제외하기는 어려우므로,
		// operator 에서 delete 를 빼는 변경만으로는 root 가 남아 성공해야 한다.
		rec := env.do(t, http.MethodPut, "/api/v1/roles/operator", "root",
			jsonBody(`{"permissions":["dashboard.read","dashboard.update"]}`))
		assert.Equal(t, http.StatusOK, rec.Code, "다른 보유자가 남으면 통과한다: %s", rec.Body.String())
	})

	t.Run("admin 역할 수정은 항상 거부", func(t *testing.T) {
		rec := env.do(t, http.MethodPut, "/api/v1/roles/admin", "root",
			jsonBody(`{"permissions":["dashboard.read"]}`))
		assert.Equal(t, http.StatusConflict, rec.Code)
	})
}

// TestLockout_LastDashboardDeleterAllowsWhenAnotherHolderExists 는 보유자가 둘이면
// 강등이 통과함을 고정한다 (AC-19 마지막 절).
func TestLockout_LastDashboardDeleterAllowsWhenAnotherHolderExists(t *testing.T) {
	env := newDashboardEnv(t, true)

	// ops 에게 관리 권한 3종(user.delete / role.update / dashboard.delete)을 부여한다.
	require.Equal(t, http.StatusOK, env.do(t, http.MethodPut, "/api/v1/roles/operator", "root",
		jsonBody(`{"permissions":["dashboard.read","dashboard.update","dashboard.delete","user.delete","role.update","user.read"]}`)).Code)

	rec := env.do(t, http.MethodDelete, "/api/v1/users/root", "ops", nil)
	assert.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// TestUserDelete_TransfersDashboardOwnership 는 UB1 #7 을 고정한다 (AC-20).
func TestUserDelete_TransfersDashboardOwnership(t *testing.T) {
	env := newDashboardEnv(t, true)
	d10 := env.create(t, "edi", "승계 대상", "shared")

	// edi 를 대상으로 하는 ACL 행도 만들어 둔다.
	other := env.create(t, "root", "acl 보유", "acl")
	require.Equal(t, http.StatusOK, env.do(t, http.MethodPut,
		"/api/v1/dashboards/"+other+"/acl", "root",
		jsonBody(`[{"subject":"user:edi","level":"view"}]`)).Code)

	rec := env.do(t, http.MethodDelete, "/api/v1/users/edi", "root", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())

	assert.Equal(t, "root", env.getDashboard(t, d10).Owner, "소유권이 삭제 실행자에게 승계된다")

	rows, err := env.acl.ListByDashboard(context.Background(), env.getDashboard(t, other).ID)
	require.NoError(t, err)
	assert.Empty(t, rows, "삭제된 사용자를 대상으로 하던 ACL 행이 제거된다")

	// 고아 대시보드가 없다.
	items, err := env.repo.List(context.Background(), false)
	require.NoError(t, err)
	users, err := storage.ListUsers(context.Background(), env.db)
	require.NoError(t, err)
	known := map[string]bool{"": true}
	for _, u := range users {
		known[u.Username] = true
	}
	for _, it := range items {
		assert.True(t, known[it.Owner], "고아 대시보드: uid=%s owner=%s", it.UID, it.Owner)
	}
}

// TestDashboardList_PreservesCreationOrder 는 목록이 생성 순서를 유지함을 고정한다.
//
// uid 가 UUID 이므로 sort_order 가 전부 0 이면 정렬이 uid 순으로 떨어져 새로 만든
// 대시보드끼리 임의로 뒤섞인다. 저장소가 소유자별 sort_order 를 부여해 이를 막는다.
func TestDashboardList_PreservesCreationOrder(t *testing.T) {
	env := newDashboardEnv(t, true)

	want := []string{"첫째", "둘째", "셋째", "넷째"}
	for _, name := range want {
		env.create(t, "root", name, "private")
	}

	rec := env.do(t, http.MethodGet, "/api/v1/dashboards", "root", nil)
	require.Equal(t, http.StatusOK, rec.Code)

	var list []dto.DashboardMeta
	decodeEnvelope(t, rec, &list)
	require.Len(t, list, len(want))

	got := make([]string, 0, len(list))
	for i, it := range list {
		got = append(got, it.Name)
		assert.EqualValues(t, i, it.SortOrder, "%s 의 sort_order", it.Name)
	}
	assert.Equal(t, want, got)
}

// TestDashboardList_SortOrderIsPerOwner 는 소유자별 시퀀스가 서로 독립임을 고정한다.
func TestDashboardList_SortOrderIsPerOwner(t *testing.T) {
	env := newDashboardEnv(t, true)

	env.create(t, "root", "root A", "shared")
	env.create(t, "edi", "edi A", "shared")
	env.create(t, "root", "root B", "shared")
	env.create(t, "edi", "edi B", "shared")

	rec := env.do(t, http.MethodGet, "/api/v1/dashboards", "root", nil)
	var list []dto.DashboardMeta
	decodeEnvelope(t, rec, &list)

	byName := map[string]int64{}
	for _, it := range list {
		byName[it.Name] = it.SortOrder
	}
	assert.EqualValues(t, 0, byName["root A"])
	assert.EqualValues(t, 1, byName["root B"])
	assert.EqualValues(t, 0, byName["edi A"], "소유자별로 0 에서 시작한다")
	assert.EqualValues(t, 1, byName["edi B"])
}

// -----------------------------------------------------------------------------
// helpers
// -----------------------------------------------------------------------------

// rawData 는 envelope 의 data 를 문자열로 반환한다 (키 존재 검증용).
func rawData(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	return string(env.Data)
}

// listPayloadKeys 는 목록 응답에 payload 키가 들어 있으면 그 조각을 반환한다.
func listPayloadKeys(t *testing.T, rec *httptest.ResponseRecorder) []string {
	t.Helper()
	var env struct {
		Data []map[string]json.RawMessage `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &env))
	var found []string
	for _, item := range env.Data {
		if _, ok := item["payload"]; ok {
			found = append(found, "payload")
		}
	}
	return found
}
