// remote_enrollment_test.go 는 수동 enrollment(그룹 H) REST API 의 권한·동작을
// 검증한다(@SPEC:SPEC-REMOTE-001 v1.1, REQ-REMOTE-H01/H03/H04/H06).
//
// 검증 항목:
//   - POST /remote/nodes (사전 등록 생성 201, 중복 409)
//   - DELETE /remote/nodes/{instance_id} (삭제 204)
//   - POST /remote/enrollment-tokens (발급 — 원본 토큰은 생성 응답에서 1회만 노출)
//   - GET /remote/enrollment-tokens (목록 — 토큰/해시 미노출)
//   - DELETE /remote/enrollment-tokens/{id} (폐기 204)
//   - 모든 엔드포인트 admin 게이트(node-role 403).
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
)

// hashRawForTest 는 원본 토큰의 SHA-256 16진 해시를 계산한다(구현 규약과 동일).
func hashRawForTest(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// remoteManagedNodeExistsErr 는 중복 사전 등록 에러를 반환한다(409 매핑 검증용).
func remoteManagedNodeExistsErr() error { return remote.ErrManagedNodeExists }

// newEnrollTokenRepo 는 핸들러 테스트용 enrollment 저장소를 SQLite(임시 파일)로 만든다.
func newEnrollTokenRepo(t *testing.T) storage.EnrollmentTokenRepository {
	t.Helper()
	repo, err := storage.NewEnrollmentTokenSQLiteRepository(context.Background(),
		filepath.Join(t.TempDir(), "enroll.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = repo.Close() })
	return repo
}

// fakePreReg 는 PreRegistrationService 의 테스트 구현이다.
type fakePreReg struct {
	created   map[string]string // instance_id -> name
	removed   []string
	createErr error
	removeErr error
}

func newFakePreReg() *fakePreReg { return &fakePreReg{created: make(map[string]string)} }

func (f *fakePreReg) PreRegister(_ context.Context, instanceID, name string) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.created[instanceID] = name
	return nil
}

func (f *fakePreReg) RemoveNode(_ context.Context, instanceID string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.removed = append(f.removed, instanceID)
	return nil
}

// newEnrollHandler 는 SQLite enrollment 저장소 + fakePreReg 로 핸들러를 만든다.
func newEnrollHandler(t *testing.T, preReg PreRegistrationService) (*RemoteEnrollmentHandler, storage.EnrollmentTokenRepository) {
	t.Helper()
	repo := newEnrollTokenRepo(t)
	h := NewRemoteEnrollmentHandler(preReg, NewEnrollmentTokenService(repo), nil)
	return h, repo
}

// doEnroll 은 role 컨텍스트와 (선택) JSON 본문으로 요청을 보낸다.
func doEnroll(t *testing.T, h *RemoteEnrollmentHandler, role, method, target, body string) *httptest.ResponseRecorder {
	t.Helper()
	router := api.NewRouter()
	h.RegisterRoutes(router.Group("/api/v1"))

	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	if role != "" {
		ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
		ctx = context.WithValue(ctx, api.ContextKeyUserID(), "admin-user")
		req = req.WithContext(ctx)
	}
	rec := httptest.NewRecorder()
	router.Handler().ServeHTTP(rec, req)
	return rec
}

// --- 사전 등록 -----------------------------------------------------------------

func TestEnroll_CreateNode(t *testing.T) {
	preReg := newFakePreReg()
	h, _ := newEnrollHandler(t, preReg)

	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/nodes",
		`{"instance_id":"n1","name":"Edge A"}`)

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "Edge A", preReg.created["n1"])
	assert.Contains(t, rec.Body.String(), "n1")
}

func TestEnroll_CreateNode_Duplicate409(t *testing.T) {
	preReg := newFakePreReg()
	preReg.createErr = remoteManagedNodeExistsErr()
	h, _ := newEnrollHandler(t, preReg)

	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/nodes",
		`{"instance_id":"dup"}`)
	assert.Equal(t, http.StatusConflict, rec.Code)
}

func TestEnroll_CreateNode_MissingID400(t *testing.T) {
	h, _ := newEnrollHandler(t, newFakePreReg())
	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/nodes", `{"name":"x"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEnroll_CreateNode_NodeRoleForbidden(t *testing.T) {
	preReg := newFakePreReg()
	h, _ := newEnrollHandler(t, preReg)
	rec := doEnroll(t, h, "node", http.MethodPost, "/api/v1/remote/nodes", `{"instance_id":"n1"}`)
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, preReg.created)
}

func TestEnroll_DeleteNode(t *testing.T) {
	preReg := newFakePreReg()
	h, _ := newEnrollHandler(t, preReg)
	rec := doEnroll(t, h, "admin", http.MethodDelete, "/api/v1/remote/nodes/n1", "")
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, []string{"n1"}, preReg.removed)
}

func TestEnroll_DeleteNode_NotFound404(t *testing.T) {
	preReg := newFakePreReg()
	preReg.removeErr = storage.ErrManagedNodeNotFound
	h, _ := newEnrollHandler(t, preReg)
	rec := doEnroll(t, h, "admin", http.MethodDelete, "/api/v1/remote/nodes/missing", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestEnroll_DeleteNode_NodeRoleForbidden(t *testing.T) {
	preReg := newFakePreReg()
	h, _ := newEnrollHandler(t, preReg)
	rec := doEnroll(t, h, "node", http.MethodDelete, "/api/v1/remote/nodes/n1", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, preReg.removed)
}

// --- enrollment 토큰 -----------------------------------------------------------

func TestEnroll_CreateToken_ShowsRawOnce(t *testing.T) {
	h, repo := newEnrollHandler(t, newFakePreReg())

	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/enrollment-tokens",
		`{"label":"fleet","max_uses":5}`)
	require.Equal(t, http.StatusCreated, rec.Code)

	var resp struct {
		Data struct {
			ID        string `json:"id"`
			Token     string `json:"token"`
			Label     string `json:"label"`
			MaxUses   *int   `json:"max_uses"`
			ExpiresAt *int64 `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.NotEmpty(t, resp.Data.ID)
	assert.NotEmpty(t, resp.Data.Token, "생성 응답은 원본 토큰을 1회 노출해야 한다")
	assert.Equal(t, "fleet", resp.Data.Label)
	require.NotNil(t, resp.Data.MaxUses)
	assert.Equal(t, 5, *resp.Data.MaxUses)

	// 토큰은 256비트(>=43 base64url chars) 이상의 엔트로피를 가져야 한다.
	assert.GreaterOrEqual(t, len(resp.Data.Token), 43)

	// 저장소에는 해시만 있고 원본 토큰은 없다(REQ-H06).
	stored, err := repo.GetByHash(context.Background(), hashRawForTest(resp.Data.Token))
	require.NoError(t, err)
	assert.Equal(t, resp.Data.ID, stored.ID)
	assert.NotEqual(t, resp.Data.Token, stored.TokenHash, "저장은 원본 토큰이 아닌 해시여야 한다")
}

func TestEnroll_CreateToken_WithExpiry(t *testing.T) {
	h, _ := newEnrollHandler(t, newFakePreReg())
	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/enrollment-tokens",
		`{"expires_in":"1h"}`)
	require.Equal(t, http.StatusCreated, rec.Code)
	var resp struct {
		Data struct {
			ExpiresAt *int64 `json:"expires_at"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.NotNil(t, resp.Data.ExpiresAt)
	assert.Greater(t, *resp.Data.ExpiresAt, int64(0))
}

func TestEnroll_CreateToken_BadDuration400(t *testing.T) {
	h, _ := newEnrollHandler(t, newFakePreReg())
	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/enrollment-tokens",
		`{"expires_in":"not-a-duration"}`)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestEnroll_ListTokens_HidesSecret(t *testing.T) {
	h, _ := newEnrollHandler(t, newFakePreReg())
	create := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/enrollment-tokens", `{"label":"a"}`)
	require.Equal(t, http.StatusCreated, create.Code)
	var created struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &created))

	rec := doEnroll(t, h, "admin", http.MethodGet, "/api/v1/remote/enrollment-tokens", "")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, created.Data.Token, "목록은 원본 토큰을 노출하면 안 된다(REQ-H06)")
	assert.NotContains(t, body, "token_hash", "목록은 해시 필드를 노출하면 안 된다")
	assert.NotContains(t, strings.ToLower(body), "\"token\"", "목록 항목에 token 필드가 없어야 한다")
	assert.Contains(t, body, "\"label\":\"a\"")
}

func TestEnroll_RevokeToken(t *testing.T) {
	h, repo := newEnrollHandler(t, newFakePreReg())
	create := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/enrollment-tokens", `{}`)
	var created struct {
		Data struct {
			ID    string `json:"id"`
			Token string `json:"token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(create.Body.Bytes(), &created))

	rec := doEnroll(t, h, "admin", http.MethodDelete, "/api/v1/remote/enrollment-tokens/"+created.Data.ID, "")
	assert.Equal(t, http.StatusNoContent, rec.Code)

	got, err := repo.GetByHash(context.Background(), hashRawForTest(created.Data.Token))
	require.NoError(t, err)
	assert.True(t, got.Revoked)
}

func TestEnroll_RevokeToken_NotFound404(t *testing.T) {
	h, _ := newEnrollHandler(t, newFakePreReg())
	rec := doEnroll(t, h, "admin", http.MethodDelete, "/api/v1/remote/enrollment-tokens/missing", "")
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// errEnrollRepo 는 Create/List 가 실패하는 저장소(500 매핑 검증용)이다.
type errEnrollRepo struct{ err error }

func (e *errEnrollRepo) Create(context.Context, storage.EnrollmentToken) error { return e.err }
func (e *errEnrollRepo) GetByHash(context.Context, string) (storage.EnrollmentToken, error) {
	return storage.EnrollmentToken{}, e.err
}
func (e *errEnrollRepo) List(context.Context) ([]storage.EnrollmentToken, error) { return nil, e.err }
func (e *errEnrollRepo) Revoke(context.Context, string) error                    { return e.err }
func (e *errEnrollRepo) IncrementUses(context.Context, string) error             { return e.err }
func (e *errEnrollRepo) Delete(context.Context, string) error                    { return e.err }
func (e *errEnrollRepo) Close() error                                            { return nil }

func TestEnroll_CreateToken_RepoError500(t *testing.T) {
	h := NewRemoteEnrollmentHandler(newFakePreReg(),
		NewEnrollmentTokenService(&errEnrollRepo{err: errors.New("disk full")}), nil)
	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/enrollment-tokens", `{}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestEnroll_ListTokens_RepoError500(t *testing.T) {
	h := NewRemoteEnrollmentHandler(newFakePreReg(),
		NewEnrollmentTokenService(&errEnrollRepo{err: errors.New("boom")}), nil)
	rec := doEnroll(t, h, "admin", http.MethodGet, "/api/v1/remote/enrollment-tokens", "")
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

// TestEnroll_CreateNode_InternalError500 은 사전 등록의 일반 에러가 500 으로 매핑되는지
// 검증한다(mapEnrollmentError default).
func TestEnroll_CreateNode_InternalError500(t *testing.T) {
	preReg := newFakePreReg()
	preReg.createErr = errors.New("unexpected")
	h, _ := newEnrollHandler(t, preReg)
	rec := doEnroll(t, h, "admin", http.MethodPost, "/api/v1/remote/nodes", `{"instance_id":"n1"}`)
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
}

func TestEnroll_Tokens_NodeRoleForbidden(t *testing.T) {
	h, _ := newEnrollHandler(t, newFakePreReg())
	for _, tc := range []struct {
		method, target string
	}{
		{http.MethodPost, "/api/v1/remote/enrollment-tokens"},
		{http.MethodGet, "/api/v1/remote/enrollment-tokens"},
		{http.MethodDelete, "/api/v1/remote/enrollment-tokens/x"},
	} {
		body := ""
		if tc.method == http.MethodPost {
			body = "{}"
		}
		rec := doEnroll(t, h, "node", tc.method, tc.target, body)
		assert.Equal(t, http.StatusForbidden, rec.Code, tc.method+" "+tc.target)
	}
}
