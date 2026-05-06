// @SPEC:SPEC-UPDATE-001 v0.1.0
// coverage_extra_test.go — Phase B 보강 테스트: 분기 커버리지 90%+ 달성용.
//
// 추가 케이스:
//   - decodeRelease 의 nightly / 배열 비어있음 / prerelease fallback
//   - Download 의 fetchSmallFile 에러 경로
//   - DownloadManifest 의 signature 다운로드 실패
//   - normalizeTag 의 빈 입력
package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// osReadFile 는 os.ReadFile 의 별칭 (test 단순화용).
var osReadFile = os.ReadFile

// TestChecker_NightlyChannel 는 nightly 채널의 prerelease 매칭을 검증.
func TestChecker_NightlyChannel(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name": "v0.5.0-nightly.20260506", "published_at": "2026-05-06T00:00:00Z",
			 "prerelease": true, "html_url": "https://example.com/nightly",
			 "assets": [
				{"name": "xflowd-linux-amd64", "browser_download_url": "https://example.com/bin", "size": 100},
				{"name": "xflowd-linux-amd64.sig", "browser_download_url": "https://example.com/sig", "size": 64},
				{"name": "checksum.txt", "browser_download_url": "https://example.com/sum", "size": 200}
			]},
			{"tag_name": "v0.4.0-beta.1", "published_at": "2026-05-01T00:00:00Z",
			 "prerelease": true, "html_url": "https://example.com/beta",
			 "assets": []}
		]`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelNightly)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.True(t, res.Available)
	assert.Equal(t, Version("v0.5.0-nightly.20260506"), res.Latest)
}

// TestChecker_PrereleaseFallback_NoChannelMatch 는 채널 패턴이 매칭 안되면 prerelease=true 첫 항목 fallback.
func TestChecker_PrereleaseFallback_NoChannelMatch(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[
			{"tag_name": "v0.5.0", "published_at": "2026-05-06T00:00:00Z",
			 "prerelease": true, "html_url": "https://example.com/pre",
			 "assets": [
				{"name": "xflowd-linux-amd64", "browser_download_url": "https://example.com/bin", "size": 100}
			]},
			{"tag_name": "v0.4.0", "published_at": "2026-04-01T00:00:00Z",
			 "prerelease": false, "html_url": "https://example.com/stable",
			 "assets": []}
		]`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelBeta)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.Equal(t, Version("v0.5.0"), res.Latest, "fallback to first prerelease")
}

// TestChecker_EmptyArray 는 빈 배열 응답을 ErrUpdateDownloadFailed 로 처리.
func TestChecker_EmptyArray(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelBeta)
	c.HTTPClient = srv.Client()

	_, err := c.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestChecker_StableMissingTagName 는 tag_name 없는 stable 응답을 거부.
func TestChecker_StableMissingTagName(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"published_at": "2026-05-06T00:00:00Z", "assets": []}`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	_, err := c.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestChecker_NetworkError_Propagates 는 네트워크 dial 실패 처리.
//
// 서버를 띄우지 않고 Closed URL 사용.
func TestChecker_NetworkError_Propagates(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 즉시 닫음

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	_, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestChecker_403Response_PropagatesError 는 403 (rate-limit 등) 도 에러로 처리.
func TestChecker_403Response_PropagatesError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limit", http.StatusForbidden)
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	_, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestDownloader_FetchSmallFile_ServerError 는 메타데이터 다운로드 5xx 처리.
func TestDownloader_FetchSmallFile_ServerError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	checksumAsset := ReleaseAsset{DownloadURL: srv.URL + "/checksum.txt", Size: 200}
	sigAsset := ReleaseAsset{DownloadURL: srv.URL + "/sig", Size: 64}

	_, err := d.DownloadManifest(context.Background(), checksumAsset, sigAsset, "xflowd-linux-amd64", "v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestDownloader_FetchSmallFile_TooLarge 는 응답이 limit 초과 시 거부.
//
// fetchSmallFile 의 internal limit 가 1MB 이므로, 더 큰 응답을 흘려 거부 검증.
func TestDownloader_FetchSmallFile_TooLarge(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 2MB → checksum 캡 1MB 초과
		large := strings.Repeat("a", 2*1024*1024)
		_, _ = w.Write([]byte(large))
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	checksumAsset := ReleaseAsset{DownloadURL: srv.URL + "/checksum.txt", Size: 200}
	sigAsset := ReleaseAsset{DownloadURL: srv.URL + "/sig", Size: 64}

	_, err := d.DownloadManifest(context.Background(), checksumAsset, sigAsset, "xflowd-linux-amd64", "v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestDownloader_DownloadManifest_HTTPRejected_Checksum 는 checksum URL 의 http:// 거부.
func TestDownloader_DownloadManifest_HTTPRejected_Checksum(t *testing.T) {
	d := NewDownloader()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	_, err := d.DownloadManifest(context.Background(),
		ReleaseAsset{DownloadURL: "http://example.com/sum"},
		ReleaseAsset{DownloadURL: "https://example.com/sig"},
		"xflowd-linux-amd64", "v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
}

// TestDownloader_DownloadManifest_HTTPRejected_Signature 는 signature URL 의 http:// 거부.
func TestDownloader_DownloadManifest_HTTPRejected_Signature(t *testing.T) {
	d := NewDownloader()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	_, err := d.DownloadManifest(context.Background(),
		ReleaseAsset{DownloadURL: "https://example.com/sum"},
		ReleaseAsset{DownloadURL: "http://example.com/sig"},
		"xflowd-linux-amd64", "v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
}

// TestDownloader_OpenTmpFails_Wrapped 는 destPath 디렉토리가 없을 때 tmp 생성 실패 처리.
func TestDownloader_OpenTmpFails_Wrapped(t *testing.T) {
	d := NewDownloader()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	asset := ReleaseAsset{
		DownloadURL: "https://example.com/bin",
		Size:        100,
	}
	// 존재하지 않는 디렉토리 → OpenFile 실패
	dest := filepath.Join("/nonexistent/path/that/does/not/exist", "binary")
	err := d.Download(context.Background(), asset, dest, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestDownloader_AssetSizeZero_SkipsDiskCheck 는 size=0 일 때 디스크 검사를 생략한다.
func TestDownloader_AssetSizeZero_SkipsDiskCheck(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 0} // 0이지만 size=0 이므로 검사 skip

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: 0}

	require.NoError(t, d.Download(context.Background(), asset, dest, nil))
	got, _ := readFileBytes(t, dest)
	assert.Equal(t, []byte("ok"), got)
}

// TestNormalizeTag_Empty 는 normalizeTag 의 빈 문자열 분기 검증.
func TestNormalizeTag_Empty(t *testing.T) {
	assert.Equal(t, Version(""), normalizeTag(""))
	assert.Equal(t, Version(""), normalizeTag("   \n  "))
}

// TestDownloader_Download_NetworkDialFails 는 dial 실패 (서버 닫힘) 처리.
func TestDownloader_Download_NetworkDialFails(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // 즉시 닫음

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: 100}

	err := d.Download(context.Background(), asset, dest, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
	// .tmp 파일도 정리됐는지 확인
	_, statErr := os.Stat(dest + ".tmp")
	assert.True(t, os.IsNotExist(statErr))
}

// TestDownloader_DownloadManifest_ChecksumWithComments 는 # 주석 행을 무시한다.
func TestDownloader_DownloadManifest_ChecksumWithComments(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/checksum.txt"):
			_, _ = w.Write([]byte("# this is a comment\n\n" +
				"abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789  xflowd-linux-amd64\n"))
		case strings.HasSuffix(r.URL.Path, "/sig"):
			_, _ = w.Write(make([]byte, 64))
		}
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	man, err := d.DownloadManifest(context.Background(),
		ReleaseAsset{DownloadURL: srv.URL + "/checksum.txt", Size: 200},
		ReleaseAsset{DownloadURL: srv.URL + "/sig", Size: 64},
		"xflowd-linux-amd64", "v0.4.0")
	require.NoError(t, err)
	assert.Equal(t, "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789", man.SHA256)
}

// TestDownloader_DownloadManifest_SignatureNetworkError 는 signature 다운로드 단계 에러 처리.
func TestDownloader_DownloadManifest_SignatureNetworkError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// checksum 은 정상 응답
		if strings.HasSuffix(r.URL.Path, "/checksum.txt") {
			_, _ = w.Write([]byte("aabbcc  xflowd-linux-amd64\n"))
			return
		}
		// signature 는 5xx
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	_, err := d.DownloadManifest(context.Background(),
		ReleaseAsset{DownloadURL: srv.URL + "/checksum.txt", Size: 200},
		ReleaseAsset{DownloadURL: srv.URL + "/sig", Size: 64},
		"xflowd-linux-amd64", "v0.4.0")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestDownloader_DownloadManifest_NilProgress 는 progress=nil 이어도 정상 동작.
func TestDownloader_DownloadManifest_NilProgress(t *testing.T) {
	binPayload := strings.Repeat("X", 200*1024) // 200KB
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(binPayload))
	}))
	defer srv.Close()

	d := NewDownloader()
	d.HTTPClient = srv.Client()
	d.disk = &fakeDiskInspector{avail: 1 << 30}

	dest := filepath.Join(t.TempDir(), "binary")
	asset := ReleaseAsset{DownloadURL: srv.URL + "/bin", Size: int64(len(binPayload))}

	require.NoError(t, d.Download(context.Background(), asset, dest, nil))
}

// readFileBytes 는 read helper.
func readFileBytes(t *testing.T, path string) ([]byte, error) {
	t.Helper()
	return osReadFile(path)
}
