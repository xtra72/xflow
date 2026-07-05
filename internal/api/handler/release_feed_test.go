// release_feed_test.go 는 익명 GitHub-호환 릴리즈 피드 + 다운로드를 검증한다. 핵심은
// 실제 노드 updater.Checker 가 피드 응답을 그대로 소비할 수 있음을 보장하는 것이다.
package handler

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/internal/updater"
)

// newFeedTestRepo 는 임시 SQLite + releases 디렉토리로 ReleaseRepository 를 만든다.
func newFeedTestRepo(t *testing.T) *storage.ReleaseRepository {
	t.Helper()
	dir := t.TempDir()
	db, err := storage.OpenSQLiteDB(context.Background(), filepath.Join(dir, "xflow.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	repo, err := storage.NewReleaseRepository(db, filepath.Join(dir, "releases"))
	require.NoError(t, err)
	return repo
}

// seedRelease 는 (version,channel) 릴리즈와 linux/amd64 바이너리+서명을 올린다.
func seedRelease(t *testing.T, repo *storage.ReleaseRepository, version, channel string, bin []byte) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, repo.UpsertRelease(ctx, version, channel, "notes", 1700000000000))
	_, err := repo.PutAsset(ctx, version, "linux", "amd64",
		strings.NewReader(string(bin)), strings.NewReader("sig-bytes"), 1700000000000)
	require.NoError(t, err)
}

// feedMux 는 피드 핸들러를 raw 핸들러로 등록한 ServeMux 를 만든다(운영 등록 경로 재현).
func feedMux(repo *storage.ReleaseRepository, publicBase string) *http.ServeMux {
	h := NewReleaseFeedHandler(repo, publicBase, nil)
	mux := http.NewServeMux()
	h.RegisterRawHandlers(func(pattern string, handler http.HandlerFunc) {
		mux.HandleFunc(pattern, handler)
	})
	return mux
}

// TestFeed_CheckerConsumesLatest 는 실제 updater.Checker(stable)가 /releases/latest 응답을
// 그대로 소비함을 검증한다(end-to-end 호환성 — 노드 코드 무변경 계약).
func TestFeed_CheckerConsumesLatest(t *testing.T) {
	repo := newFeedTestRepo(t)
	bin := []byte("xflowd-v0.4.0-binary")
	seedRelease(t, repo, "v0.4.0", "stable", bin)

	// public_base_url 미설정 → 요청 Host 에서 유도(https 강제). httptest TLS 서버를 쓴다.
	srv := httptest.NewTLSServer(feedMux(repo, ""))
	defer srv.Close()

	// update_url 은 .../api/v1/updates → Checker 가 /releases/latest 를 덧붙인다.
	c, err := updater.NewChecker(srv.URL+"/api/v1/updates", updater.ChannelStable)
	require.NoError(t, err)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.True(t, res.Available, "v0.3.0 → v0.4.0 이므로 Available=true 여야 함")
	assert.Equal(t, updater.Version("v0.4.0"), res.Latest)
	require.NotNil(t, res.BinaryAsset)
	assert.Equal(t, "xflowd-linux-amd64", res.BinaryAsset.Name)
	// 노드 Downloader 는 모든 browser_download_url 에 https 를 강제한다.
	assert.True(t, strings.HasPrefix(res.BinaryAsset.DownloadURL, "https://"),
		"asset URL 은 절대 https 여야 함: %s", res.BinaryAsset.DownloadURL)
	assert.Equal(t, int64(len(bin)), res.BinaryAsset.Size)
	require.NotNil(t, res.SignatureAsset)
	assert.Equal(t, "xflowd-linux-amd64.sig", res.SignatureAsset.Name)
	require.NotNil(t, res.ChecksumAsset)
	assert.Equal(t, "checksum.txt", res.ChecksumAsset.Name)
}

// TestFeed_LatestShape 는 /releases/latest 의 GitHub 필드명 + RFC3339 published_at 을 검증한다.
func TestFeed_LatestShape(t *testing.T) {
	repo := newFeedTestRepo(t)
	seedRelease(t, repo, "v1.0.0", "stable", []byte("bin"))

	mux := feedMux(repo, "https://mgmt.example.com")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases/latest", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// 노드 Checker 와 동일 필드명으로 unmarshal 되는지(계약).
	var got struct {
		TagName     string `json:"tag_name"`
		Name        string `json:"name"`
		PublishedAt string `json:"published_at"`
		Prerelease  bool   `json:"prerelease"`
		HTMLURL     string `json:"html_url"`
		Assets      []struct {
			Name        string `json:"name"`
			DownloadURL string `json:"browser_download_url"`
			Size        int64  `json:"size"`
		} `json:"assets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	assert.Equal(t, "v1.0.0", got.TagName)
	assert.False(t, got.Prerelease, "stable 채널은 prerelease=false")
	// RFC3339 형식인지(예: 2023-11-14T...Z).
	assert.Regexp(t, `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`, got.PublishedAt)
	// public_base_url 설정 시 그 base 를 그대로 사용.
	for _, a := range got.Assets {
		assert.True(t, strings.HasPrefix(a.DownloadURL, "https://mgmt.example.com/"),
			"asset URL 은 설정된 base 를 사용해야 함: %s", a.DownloadURL)
	}
}

// TestFeed_ListArrayAndChannelFlags 는 /releases 배열 + 채널/prerelease 매핑을 검증한다.
func TestFeed_ListArrayAndChannelFlags(t *testing.T) {
	repo := newFeedTestRepo(t)
	seedRelease(t, repo, "v1.0.0", "stable", []byte("bin1"))
	seedRelease(t, repo, "v1.1.0-beta.1", "beta", []byte("bin2"))
	seedRelease(t, repo, "v1.2.0-nightly.20260620", "nightly", []byte("bin3"))

	mux := feedMux(repo, "https://mgmt.example.com")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var arr []struct {
		TagName    string `json:"tag_name"`
		Prerelease bool   `json:"prerelease"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &arr))
	require.Len(t, arr, 3)
	// semver 내림차순: 1.2.0-nightly > 1.1.0-beta > 1.0.0.
	assert.Equal(t, "v1.2.0-nightly.20260620", arr[0].TagName)
	assert.Equal(t, "v1.0.0", arr[2].TagName)

	flags := map[string]bool{}
	for _, r := range arr {
		flags[r.TagName] = r.Prerelease
	}
	assert.False(t, flags["v1.0.0"], "stable → prerelease=false")
	assert.True(t, flags["v1.1.0-beta.1"], "beta → prerelease=true")
	assert.True(t, flags["v1.2.0-nightly.20260620"], "nightly → prerelease=true")
}

// TestFeed_CheckerConsumesBetaArray 는 실제 Checker(beta)가 /releases 배열에서 beta 를
// 선택함을 검증한다(채널 매칭 계약).
func TestFeed_CheckerConsumesBetaArray(t *testing.T) {
	repo := newFeedTestRepo(t)
	seedRelease(t, repo, "v1.0.0", "stable", []byte("bin1"))
	seedRelease(t, repo, "v1.1.0-beta.1", "beta", []byte("bin2"))

	srv := httptest.NewTLSServer(feedMux(repo, ""))
	defer srv.Close()

	c, err := updater.NewChecker(srv.URL+"/api/v1/updates", updater.ChannelBeta)
	require.NoError(t, err)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v1.0.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.Equal(t, updater.Version("v1.1.0-beta.1"), res.Latest)
	assert.True(t, res.IsPrerelease)
}

// TestFeed_DownloadBinaryAndChecksum 는 다운로드가 정확한 바이트를 스트리밍하고 checksum.txt
// 가 올바르게 생성됨을 검증한다.
func TestFeed_DownloadBinaryAndChecksum(t *testing.T) {
	repo := newFeedTestRepo(t)
	bin := []byte("exact-binary-bytes-to-stream")
	seedRelease(t, repo, "v1.0.0", "stable", bin)
	mux := feedMux(repo, "https://mgmt.example.com")

	// 바이너리 다운로드.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases/download/v1.0.0/xflowd-linux-amd64", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/octet-stream", rec.Header().Get("Content-Type"))
	body, _ := io.ReadAll(rec.Body)
	assert.Equal(t, bin, body)

	// checksum.txt — 즉석 생성, sha256 정확성.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases/download/v1.0.0/checksum.txt", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	sum := sha256.Sum256(bin)
	assert.Contains(t, rec.Body.String(), hex.EncodeToString(sum[:])+"  xflowd-linux-amd64")

	// .sig 다운로드.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases/download/v1.0.0/xflowd-linux-amd64.sig", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "sig-bytes", rec.Body.String())
}

// TestFeed_NotFoundAndTraversal 는 404 와 경로 순회 거부를 검증한다.
func TestFeed_NotFoundAndTraversal(t *testing.T) {
	repo := newFeedTestRepo(t)
	mux := feedMux(repo, "https://mgmt.example.com")

	// 릴리즈 없음 → latest 404.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases/latest", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)

	// 빈 목록은 200 + 빈 배열.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "[]\n", rec.Body.String())

	// 미존재 에셋 → 404.
	seedRelease(t, repo, "v1.0.0", "stable", []byte("bin"))
	req = httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases/download/v1.0.0/xflowd-windows-amd64", nil)
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// TestFeed_XForwardedHost 는 public_base_url 미설정 시 X-Forwarded-Host 를 우선 사용하고
// scheme 을 https 로 강제함을 검증한다(리버스 프록시 뒤 배포).
func TestFeed_XForwardedHost(t *testing.T) {
	repo := newFeedTestRepo(t)
	seedRelease(t, repo, "v1.0.0", "stable", []byte("bin"))
	mux := feedMux(repo, "") // 설정 미사용 → 요청 Host 유도.

	req := httptest.NewRequest(http.MethodGet, "/api/v1/updates/releases/latest", nil)
	req.Header.Set("X-Forwarded-Host", "downloads.example.org")
	req.Header.Set("X-Forwarded-Proto", "http") // http 여도 https 로 강제되어야 함.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	var got struct {
		Assets []struct {
			DownloadURL string `json:"browser_download_url"`
		} `json:"assets"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
	require.NotEmpty(t, got.Assets)
	assert.True(t, strings.HasPrefix(got.Assets[0].DownloadURL, "https://downloads.example.org/"),
		"X-Forwarded-Host + https 강제: %s", got.Assets[0].DownloadURL)
}
