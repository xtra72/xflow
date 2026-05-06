// @SPEC:SPEC-UPDATE-001 v0.1.0
// checker_test.go — Phase B 단위 테스트: GitHub Releases API 클라이언트.
//
// 테스트 전략:
//   - httptest.Server 로 GitHub API 모방 (실제 GitHub 호출 없음)
//   - HTTPS 강제 (HTTP 스킴은 NewChecker 단계에서 거부)
//   - 채널별 endpoint 라우팅 (stable: /releases/latest, beta/nightly: /releases)
//   - asset 매칭 (OS/arch + suffix .sig / checksum.txt)
//   - 다운그레이드 감지: latest < current → Available=false
package updater

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helperReleasePayload 는 GitHub Releases API 응답 JSON 을 생성한다.
//
// stable 채널은 단일 객체, beta/nightly 는 배열 반환을 흉내낸다.
func helperReleasePayload(tag, sigURL, binURL, checksumURL string, size int64) string {
	return `{
		"tag_name": "` + tag + `",
		"name": "xflowd ` + tag + `",
		"published_at": "2026-05-06T00:00:00Z",
		"prerelease": false,
		"html_url": "https://github.com/example/xflow/releases/tag/` + tag + `",
		"assets": [
			{"name": "xflowd-linux-amd64", "browser_download_url": "` + binURL + `", "size": ` + intToStr(size) + `},
			{"name": "xflowd-linux-amd64.sig", "browser_download_url": "` + sigURL + `", "size": 64},
			{"name": "checksum.txt", "browser_download_url": "` + checksumURL + `", "size": 200}
		]
	}`
}

// intToStr 는 의도적으로 fmt 미사용 (테스트 import 가지 단순화).
func intToStr(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

// TestNewChecker_HTTPSchemeRejected 는 HTTP URL 을 거부한다 (M1, M13).
func TestNewChecker_HTTPSchemeRejected(t *testing.T) {
	_, err := NewChecker("http://example.com/releases", ChannelStable)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid),
		"expected ErrUpdateChannelInvalid, got %v", err)
}

// TestNewChecker_InvalidChannelRejected 는 비-enum 채널을 거부한다.
func TestNewChecker_InvalidChannelRejected(t *testing.T) {
	_, err := NewChecker("https://example.com/releases", Channel("invalid"))
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
}

// TestNewChecker_EmptyURLRejected 은 빈 URL 을 거부한다.
func TestNewChecker_EmptyURLRejected(t *testing.T) {
	_, err := NewChecker("", ChannelStable)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
}

// TestNewChecker_MalformedURLRejected 는 파싱 실패 URL 을 거부한다.
func TestNewChecker_MalformedURLRejected(t *testing.T) {
	_, err := NewChecker("://broken", ChannelStable)
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateChannelInvalid))
}

// TestChecker_NewVersionAvailable_HappyPath 는 정상 신규 버전 발견을 검증한다.
//
// SPEC M2 / Acceptance Scenario 1.
func TestChecker_NewVersionAvailable_HappyPath(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Contains(t, r.URL.Path, "/releases/latest")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(helperReleasePayload("v0.4.0",
			srvURL(r)+"/dl/sig",
			srvURL(r)+"/dl/bin",
			srvURL(r)+"/dl/sum",
			12345)))
	}))
	defer srv.Close()

	c, err := NewChecker(srv.URL, ChannelStable)
	require.NoError(t, err)
	c.HTTPClient = srv.Client() // accept self-signed httptest cert

	res, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.True(t, res.Available, "expected Available=true (v0.3.0 → v0.4.0)")
	assert.Equal(t, Version("v0.3.0"), res.Current)
	assert.Equal(t, Version("v0.4.0"), res.Latest)
	require.NotNil(t, res.BinaryAsset)
	assert.Equal(t, "xflowd-linux-amd64", res.BinaryAsset.Name)
	assert.Equal(t, int64(12345), res.BinaryAsset.Size)
	require.NotNil(t, res.SignatureAsset)
	assert.Equal(t, "xflowd-linux-amd64.sig", res.SignatureAsset.Name)
	require.NotNil(t, res.ChecksumAsset)
	assert.Equal(t, "checksum.txt", res.ChecksumAsset.Name)
	assert.Contains(t, res.ReleaseURL, "v0.4.0")
}

// TestChecker_AlreadyLatest 는 동일 버전일 때 Available=false 를 검증한다.
func TestChecker_AlreadyLatest(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(helperReleasePayload("v0.4.0",
			"https://example.com/sig",
			"https://example.com/bin",
			"https://example.com/sum",
			100)))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.False(t, res.Available)
}

// TestChecker_DowngradeDetected 는 latest < current 인 경우 Available=false (M8 보호).
func TestChecker_DowngradeDetected(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(helperReleasePayload("v0.2.0",
			"https://example.com/sig",
			"https://example.com/bin",
			"https://example.com/sum",
			100)))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.False(t, res.Available, "downgrade must not surface as Available=true")
}

// TestChecker_404Response_NoReleasesYet 은 404 (releases 없음) 를 정상 종료로 처리한다.
func TestChecker_404Response_NoReleasesYet(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Not Found", http.StatusNotFound)
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err, "404 = no releases yet, should not be an error")
	assert.False(t, res.Available)
}

// TestChecker_500Response_PropagatesError 는 5xx 를 ErrUpdateDownloadFailed 로 변환한다.
func TestChecker_500Response_PropagatesError(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	_, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestChecker_MalformedJSON_Error 는 잘못된 JSON 을 거부한다.
func TestChecker_MalformedJSON_Error(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	_, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrUpdateDownloadFailed))
}

// TestChecker_NoMatchingAsset_ForOSArch 는 플랫폼 asset 부재 시 BinaryAsset=nil 처리.
func TestChecker_NoMatchingAsset_ForOSArch(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.4.0",
			"published_at": "2026-05-06T00:00:00Z",
			"html_url": "https://example.com/v0.4.0",
			"assets": [
				{"name": "xflowd-windows-arm64", "browser_download_url": "https://example.com/win", "size": 100}
			]
		}`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	// 플랫폼 asset 이 없으면 Available=false (다운로드 불가)
	assert.False(t, res.Available, "no matching asset → not available")
	assert.Nil(t, res.BinaryAsset)
}

// TestChecker_PicksRightAssetByPlatform 은 다중 플랫폼 중 정확한 asset 선택을 검증.
func TestChecker_PicksRightAssetByPlatform(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"tag_name": "v0.4.0",
			"published_at": "2026-05-06T00:00:00Z",
			"html_url": "https://example.com/v0.4.0",
			"assets": [
				{"name": "xflowd-linux-amd64",       "browser_download_url": "https://example.com/lin64", "size": 100},
				{"name": "xflowd-linux-arm64",       "browser_download_url": "https://example.com/linarm", "size": 200},
				{"name": "xflowd-darwin-amd64",      "browser_download_url": "https://example.com/dar64", "size": 300},
				{"name": "xflowd-linux-amd64.sig",   "browser_download_url": "https://example.com/sig", "size": 64},
				{"name": "checksum.txt",             "browser_download_url": "https://example.com/sum", "size": 200}
			]
		}`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.3.0", "darwin", "amd64", "xflowd")
	require.NoError(t, err)
	require.NotNil(t, res.BinaryAsset)
	assert.Equal(t, "xflowd-darwin-amd64", res.BinaryAsset.Name)
	assert.Equal(t, int64(300), res.BinaryAsset.Size)
}

// TestChecker_PrereleaseChannel_Beta 는 beta 채널 endpoint 라우팅을 검증.
//
// stable 은 /releases/latest, prerelease 채널은 /releases (배열) 사용.
func TestChecker_PrereleaseChannel_Beta(t *testing.T) {
	visitedPath := ""
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		visitedPath = r.URL.Path
		// beta: 배열 응답
		_, _ = w.Write([]byte(`[
			{
				"tag_name": "v0.5.0-beta.1",
				"published_at": "2026-05-06T00:00:00Z",
				"prerelease": true,
				"html_url": "https://example.com/beta",
				"assets": [
					{"name": "xflowd-linux-amd64",     "browser_download_url": "https://example.com/bin", "size": 100},
					{"name": "xflowd-linux-amd64.sig", "browser_download_url": "https://example.com/sig", "size": 64},
					{"name": "checksum.txt",           "browser_download_url": "https://example.com/sum", "size": 200}
				]
			},
			{
				"tag_name": "v0.4.0",
				"published_at": "2026-04-01T00:00:00Z",
				"prerelease": false,
				"html_url": "https://example.com/stable",
				"assets": []
			}
		]`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelBeta)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.4.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.True(t, res.Available)
	assert.Equal(t, Version("v0.5.0-beta.1"), res.Latest)
	assert.NotContains(t, visitedPath, "/latest", "beta channel must not call /releases/latest")
}

// TestChecker_ContextCancellation 은 ctx cancel 전파를 검증.
func TestChecker_ContextCancellation(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 의도적으로 응답을 지연시키는 동안 ctx 취소
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 즉시 취소

	_, err := c.Check(ctx, "v0.3.0", "linux", "amd64", "xflowd")
	require.Error(t, err)
}

// TestChecker_PublishedAtPropagated 는 published_at 시간 전파를 검증.
func TestChecker_PublishedAtPropagated(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(helperReleasePayload("v0.4.0",
			"https://example.com/sig",
			"https://example.com/bin",
			"https://example.com/sum",
			100)))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	want, _ := time.Parse(time.RFC3339, "2026-05-06T00:00:00Z")
	assert.True(t, res.PublishedAt.Equal(want),
		"published_at mismatch: want %v, got %v", want, res.PublishedAt)
}

// TestChecker_TagWithoutVPrefix 는 v 접두사 없는 tag 도 정규화 처리한다.
//
// 일부 GitHub release 는 "0.4.0" 형식을 사용; SPEC 은 "v0.4.0" 만 valid 이므로 정규화 필요.
func TestChecker_TagWithoutVPrefix(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"tag_name": "0.4.0",
			"published_at": "2026-05-06T00:00:00Z",
			"html_url": "https://example.com/v0.4.0",
			"assets": [
				{"name": "xflowd-linux-amd64",     "browser_download_url": "https://example.com/bin", "size": 100},
				{"name": "xflowd-linux-amd64.sig", "browser_download_url": "https://example.com/sig", "size": 64},
				{"name": "checksum.txt",           "browser_download_url": "https://example.com/sum", "size": 200}
			]
		}`))
	}))
	defer srv.Close()

	c, _ := NewChecker(srv.URL, ChannelStable)
	c.HTTPClient = srv.Client()

	res, err := c.Check(context.Background(), "v0.3.0", "linux", "amd64", "xflowd")
	require.NoError(t, err)
	assert.True(t, res.Available)
	assert.Equal(t, Version("v0.4.0"), res.Latest, "tag must be normalized with v prefix")
}

// srvURL 은 r 의 호스트로부터 base URL 을 재구성한다 (httptest 동적 포트 대응).
func srvURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		scheme = "http"
	}
	u := &url.URL{Scheme: scheme, Host: r.Host}
	return strings.TrimRight(u.String(), "/")
}
