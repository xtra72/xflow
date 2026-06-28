// release_admin_test.go 는 릴리즈 admin 관리 API + multipart 업로드를 검증한다
// (admin 게이팅, 생성, 업로드, 삭제).
package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/storage"
)

// newAdminTestRepo 는 임시 SQLite + releases 디렉토리로 ReleaseRepository 를 만든다.
func newAdminTestRepo(t *testing.T) *storage.ReleaseRepository {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.OpenSQLiteDB(context.Background(), filepath.Join(dir, "xflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo, err := storage.NewReleaseRepository(db, filepath.Join(dir, "releases"))
	require.NoError(t, err)
	return repo
}

// adminJSONRequest 는 JSON admin 라우트를 role 주입과 함께 실행한다(RouteGroup 경로).
// role="" 이면 역할 미주입(비-admin) → 403 검증용.
func adminJSONRequest(t *testing.T, repo *storage.ReleaseRepository) func(method, target, body, role string) *httptest.ResponseRecorder {
	t.Helper()
	h := NewReleaseAdminHandler(repo, nil, nil) // jwtSvc nil — JSON 라우트는 컨텍스트 role 사용.
	return func(method, target, body, role string) *httptest.ResponseRecorder {
		router := api.NewRouter()
		var req *http.Request
		if body != "" {
			req = httptest.NewRequest(method, target, strings.NewReader(body))
		} else {
			req = httptest.NewRequest(method, target, nil)
		}
		if role != "" {
			ctx := context.WithValue(req.Context(), api.ContextKeyUserRole(), role)
			ctx = context.WithValue(ctx, api.ContextKeyUserID(), "alice")
			req = req.WithContext(ctx)
		}
		rec := httptest.NewRecorder()
		h.RegisterRoutes(router.Group("/api/v1"))
		router.Handler().ServeHTTP(rec, req)
		return rec
	}
}

// uploadMultipart 는 multipart 업로드 raw 핸들러를 호출한다. role="admin" 이면 테스트
// 우회 모드(jwtSvc nil)에서 admin 컨텍스트를 주입한다.
func uploadMultipart(t *testing.T, repo *storage.ReleaseRepository, version, goos, arch string, bin, sig []byte, role string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(t, mw.WriteField("os", goos))
	require.NoError(t, mw.WriteField("arch", arch))
	bw, err := mw.CreateFormFile("binary", "xflowd")
	require.NoError(t, err)
	_, _ = bw.Write(bin)
	if sig != nil {
		sw, serr := mw.CreateFormFile("signature", "xflowd.sig")
		require.NoError(t, serr)
		_, _ = sw.Write(sig)
	}
	require.NoError(t, mw.Close())

	h := NewReleaseAdminHandler(repo, nil, nil) // jwtSvc nil → 테스트 인증 우회.
	mux := http.NewServeMux()
	h.RegisterRawHandlers(func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, handler)
	})

	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/remote/releases/"+version+"/assets", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if role != "" {
		req = req.WithContext(context.WithValue(req.Context(), testRoleKey{}, role))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestReleaseAdmin_RequiresAdmin(t *testing.T) {
	repo := newAdminTestRepo(t)
	do := adminJSONRequest(t, repo)

	// 비-admin(역할 미주입) → 403.
	rec := do(http.MethodGet, "/api/v1/remote/releases", "", "")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// viewer 역할 → 403.
	rec = do(http.MethodPost, "/api/v1/remote/releases", `{"version":"v1.0.0"}`, "viewer")
	assert.Equal(t, http.StatusForbidden, rec.Code)

	// 업로드 raw 핸들러도 비-admin 거부.
	upRec := uploadMultipart(t, repo, "v1.0.0", "linux", "amd64", []byte("bin"), []byte("sig"), "")
	assert.Equal(t, http.StatusForbidden, upRec.Code)
}

func TestReleaseAdmin_CreateAndList(t *testing.T) {
	repo := newAdminTestRepo(t)
	do := adminJSONRequest(t, repo)

	// 생성(channel 생략 → stable).
	rec := do(http.MethodPost, "/api/v1/remote/releases", `{"version":"v1.0.0","notes":"first"}`, "admin")
	require.Equal(t, http.StatusOK, rec.Code)

	var created struct {
		Success bool `json:"success"`
		Data    struct {
			Version string `json:"version"`
			Channel string `json:"channel"`
			Notes   string `json:"notes"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.True(t, created.Success)
	assert.Equal(t, "v1.0.0", created.Data.Version)
	assert.Equal(t, "stable", created.Data.Channel)
	assert.Equal(t, "first", created.Data.Notes)

	// 잘못된 semver 거부.
	rec = do(http.MethodPost, "/api/v1/remote/releases", `{"version":"1.0.0"}`, "admin")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 잘못된 채널 거부.
	rec = do(http.MethodPost, "/api/v1/remote/releases", `{"version":"v2.0.0","channel":"edge"}`, "admin")
	assert.Equal(t, http.StatusBadRequest, rec.Code)

	// 목록 조회.
	rec = do(http.MethodGet, "/api/v1/remote/releases", "", "admin")
	require.Equal(t, http.StatusOK, rec.Code)
	var listed struct {
		Data struct {
			Releases []struct {
				Version string `json:"version"`
			} `json:"releases"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &listed))
	require.Len(t, listed.Data.Releases, 1)
	assert.Equal(t, "v1.0.0", listed.Data.Releases[0].Version)
}

func TestReleaseAdmin_UploadHappyPath(t *testing.T) {
	repo := newAdminTestRepo(t)

	rec := uploadMultipart(t, repo, "v1.0.0", "linux", "arm", []byte("rpi-binary"), []byte("ed25519-sig"), "admin")
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			OS       string `json:"os"`
			Arch     string `json:"arch"`
			Filename string `json:"filename"`
			SHA256   string `json:"sha256"`
			HasSig   bool   `json:"has_sig"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	require.True(t, resp.Success)
	assert.Equal(t, "linux", resp.Data.OS)
	assert.Equal(t, "arm", resp.Data.Arch)
	assert.Equal(t, "xflowd-linux-arm", resp.Data.Filename)
	assert.NotEmpty(t, resp.Data.SHA256)
	assert.True(t, resp.Data.HasSig)

	// 저장소에 반영되었는지(릴리즈 자동 생성 + asset).
	relRec, err := repo.GetRelease(context.Background(), "v1.0.0")
	require.NoError(t, err)
	require.Len(t, relRec.Assets, 1)
	assert.Equal(t, "arm", relRec.Assets[0].Arch)

	// 잘못된 semver version 거부.
	bad := uploadMultipart(t, repo, "1.0.0", "linux", "amd64", []byte("bin"), []byte("sig"), "admin")
	assert.Equal(t, http.StatusBadRequest, bad.Code)
}

func TestReleaseAdmin_DeleteReleaseAndAsset(t *testing.T) {
	repo := newAdminTestRepo(t)
	do := adminJSONRequest(t, repo)

	// 업로드로 릴리즈 + 2개 asset 생성.
	require.Equal(t, http.StatusOK,
		uploadMultipart(t, repo, "v1.0.0", "linux", "amd64", []byte("a"), []byte("s1"), "admin").Code)
	require.Equal(t, http.StatusOK,
		uploadMultipart(t, repo, "v1.0.0", "linux", "arm64", []byte("b"), []byte("s2"), "admin").Code)

	// asset 1개 삭제.
	rec := do(http.MethodDelete, "/api/v1/remote/releases/v1.0.0/assets/linux/amd64", "", "admin")
	require.Equal(t, http.StatusOK, rec.Code)
	relRec, err := repo.GetRelease(context.Background(), "v1.0.0")
	require.NoError(t, err)
	require.Len(t, relRec.Assets, 1)
	assert.Equal(t, "arm64", relRec.Assets[0].Arch)

	// 릴리즈 전체 삭제.
	rec = do(http.MethodDelete, "/api/v1/remote/releases/v1.0.0", "", "admin")
	require.Equal(t, http.StatusOK, rec.Code)
	_, gerr := repo.GetRelease(context.Background(), "v1.0.0")
	require.ErrorIs(t, gerr, storage.ErrReleaseNotFound)
}
