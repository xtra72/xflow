// @SPEC:SPEC-UPDATE-001 v0.1.0
// checker.go — GitHub Releases API 를 통한 버전 확인 (M2).
//
// 보안 critical:
//   - HTTPS 전용 (NewChecker 단계에서 http:// 거부; M1, M13)
//   - 채널 enum 검증 (stable/beta/nightly 만 허용)
//   - 다운그레이드 자동 미수행 (Available=false 로 처리; M8)
//
// 디자인:
//   - stable 채널: GET ${BaseURL}/releases/latest (단일 객체)
//   - beta/nightly 채널: GET ${BaseURL}/releases (배열, prerelease 우선)
//   - 5xx 또는 JSON 파싱 실패 → ErrUpdateDownloadFailed wrap
//   - 404 → no releases yet (Available=false, no error)
//   - asset 매칭: AssetName(binary, os, arch) 와 정확 일치
//   - signature asset: <binaryName>.sig
//   - checksum asset: "checksum.txt"
package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CheckResult 는 버전 확인 결과를 담는다.
//
// SPEC M2 응답 페이로드: current_version, latest_version, update_available, release_notes_url, published_at.
type CheckResult struct {
	Available      bool          // 새 버전 적용 가능 (latest > current AND binary asset 존재)
	Current        Version       // 입력된 현재 버전
	Latest         Version       // 채널의 최신 버전 (정규화됨)
	BinaryAsset    *ReleaseAsset // 플랫폼 매칭 바이너리 asset (없으면 nil)
	SignatureAsset *ReleaseAsset // <binary>.sig (없으면 nil)
	ChecksumAsset  *ReleaseAsset // checksum.txt (없으면 nil)
	ReleaseURL     string        // GitHub release 페이지 URL (release_notes_url)
	PublishedAt    time.Time     // release 게시 시각 (replay 방어 + 표시용)
	IsPrerelease   bool          // beta/nightly 여부
}

// Checker 는 채널에서 최신 release 메타데이터를 조회한다.
//
// HTTPClient 는 외부에서 주입 가능 (테스트용 httptest.Server 클라이언트 또는
// production 의 Timeout/InsecureSkipVerify 설정 적용 클라이언트).
type Checker struct {
	BaseURL    *url.URL
	Channel    Channel
	HTTPClient *http.Client
}

// NewChecker 는 Checker 인스턴스를 생성한다.
//
// 입력 검증 (fail-fast):
//   - rawURL 이 빈 문자열 → ErrUpdateChannelInvalid
//   - URL 파싱 실패 → ErrUpdateChannelInvalid
//   - 스킴이 "https" 가 아님 → ErrUpdateChannelInvalid (M1, M13: HTTPS 강제)
//   - 채널이 enum 멤버가 아님 → ErrUpdateChannelInvalid
//
// 보안: HTTP 거부는 NewChecker 단계 + Download 단계 양쪽에서 수행 (depth-in-defense).
func NewChecker(rawURL string, channel Channel) (*Checker, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("%w: empty URL", ErrUpdateChannelInvalid)
	}
	if !channel.IsValid() {
		return nil, fmt.Errorf("%w: channel %q not in {stable,beta,nightly}",
			ErrUpdateChannelInvalid, channel)
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("%w: parse URL: %v", ErrUpdateChannelInvalid, err)
	}
	if u.Scheme != "https" {
		return nil, fmt.Errorf("%w: update_url must use https:// scheme; got %s://%s",
			ErrUpdateChannelInvalid, u.Scheme, u.Host)
	}
	return &Checker{
		BaseURL: u,
		Channel: channel,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}, nil
}

// githubRelease 는 GitHub Releases API 응답의 부분 매핑이다 (필요 필드만).
type githubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	PublishedAt time.Time     `json:"published_at"`
	Prerelease  bool          `json:"prerelease"`
	HTMLURL     string        `json:"html_url"`
	Assets      []githubAsset `json:"assets"`
}

type githubAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// Check 는 채널에서 최신 release 를 조회하고 currentVersion 과 비교한다.
//
// 인자:
//   - ctx: 호출자 timeout/cancel 제어
//   - currentVersion: 현재 실행 중인 바이너리 버전 (semver)
//   - goos / goarch: 플랫폼 매칭 (예: "linux", "amd64")
//   - binary: asset prefix (예: "xflowd")
//
// 반환:
//   - 정상 신규 버전 발견: CheckResult{Available: true, ...}, nil
//   - 동일 버전 / 다운그레이드: CheckResult{Available: false, ...}, nil
//   - 404 (releases 없음): CheckResult{Available: false}, nil
//   - 5xx / 네트워크 오류 / JSON 실패: ErrUpdateDownloadFailed wrap
//   - ctx cancel: 표준 ctx 에러 (errors.Is(err, context.Canceled) 식별 가능)
func (c *Checker) Check(ctx context.Context, currentVersion Version, goos, goarch, binary string) (CheckResult, error) {
	endpoint := c.endpointURL()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return CheckResult{}, fmt.Errorf("%w: build request: %v", ErrUpdateDownloadFailed, err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "xflow-updater/1.0")

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		// ctx cancel 은 그대로 전파 (호출자가 식별 가능)
		if ctx.Err() != nil {
			return CheckResult{}, ctx.Err()
		}
		return CheckResult{}, fmt.Errorf("%w: do request: %v", ErrUpdateDownloadFailed, err)
	}
	defer func() { _ = resp.Body.Close() }()

	// 404: 채널에 release 가 아직 없음 → 정상 종료
	if resp.StatusCode == http.StatusNotFound {
		return CheckResult{
			Current:   currentVersion,
			Available: false,
		}, nil
	}
	if resp.StatusCode >= 400 {
		// 4xx (404 외) / 5xx 모두 실패로 처리
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return CheckResult{}, fmt.Errorf("%w: HTTP %d: %s",
			ErrUpdateDownloadFailed, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	rel, err := decodeRelease(resp.Body, c.Channel)
	if err != nil {
		return CheckResult{}, fmt.Errorf("%w: decode: %v", ErrUpdateDownloadFailed, err)
	}

	latest := normalizeTag(rel.TagName)
	res := CheckResult{
		Current:      currentVersion,
		Latest:       latest,
		ReleaseURL:   rel.HTMLURL,
		PublishedAt:  rel.PublishedAt,
		IsPrerelease: rel.Prerelease,
	}

	// asset 매칭
	binAssetName := AssetName(binary, goos, goarch)
	sigAssetName := binAssetName + ".sig"
	for i := range rel.Assets {
		a := rel.Assets[i]
		switch a.Name {
		case binAssetName:
			res.BinaryAsset = &ReleaseAsset{Name: a.Name, DownloadURL: a.DownloadURL, Size: a.Size}
		case sigAssetName:
			res.SignatureAsset = &ReleaseAsset{Name: a.Name, DownloadURL: a.DownloadURL, Size: a.Size}
		case "checksum.txt", "checksums.txt":
			res.ChecksumAsset = &ReleaseAsset{Name: a.Name, DownloadURL: a.DownloadURL, Size: a.Size}
		}
	}

	// Available 판정:
	//   1. binary asset 매칭 성공
	//   2. latest > current (다운그레이드 자동 회피)
	if res.BinaryAsset != nil && latest.Compare(currentVersion) > 0 {
		res.Available = true
	}
	return res, nil
}

// endpointURL 은 채널에 따라 GitHub Releases API endpoint 를 구성한다.
//
// stable: ${BaseURL}/releases/latest (단일 객체)
// beta/nightly: ${BaseURL}/releases (배열, 첫 prerelease 항목)
func (c *Checker) endpointURL() string {
	base := strings.TrimRight(c.BaseURL.String(), "/")
	if c.Channel == ChannelStable {
		return base + "/releases/latest"
	}
	return base + "/releases"
}

// decodeRelease 는 채널에 맞춰 응답 본문을 디코딩한다.
//
// stable: 단일 객체.
// beta/nightly: 배열. 입력된 채널과 매칭하는 첫 prerelease 를 선택.
//   - beta: tag 에 "-beta" 포함하거나 prerelease=true 첫 항목
//   - nightly: tag 에 "-nightly" 포함
//   - 매칭 없으면 첫 항목 사용 (defensive default)
func decodeRelease(body io.Reader, channel Channel) (githubRelease, error) {
	if channel == ChannelStable {
		var single githubRelease
		if err := json.NewDecoder(body).Decode(&single); err != nil {
			return githubRelease{}, err
		}
		if single.TagName == "" {
			return githubRelease{}, errors.New("missing tag_name")
		}
		return single, nil
	}

	var arr []githubRelease
	if err := json.NewDecoder(body).Decode(&arr); err != nil {
		return githubRelease{}, err
	}
	if len(arr) == 0 {
		return githubRelease{}, errors.New("no releases in array")
	}

	// 채널-매칭 prerelease 선택
	want := "-beta"
	if channel == ChannelNightly {
		want = "-nightly"
	}
	for _, r := range arr {
		if strings.Contains(r.TagName, want) {
			return r, nil
		}
	}
	// 매칭 없으면 prerelease=true 첫 항목
	for _, r := range arr {
		if r.Prerelease {
			return r, nil
		}
	}
	return arr[0], nil
}

// normalizeTag 는 tag 문자열에 v 접두사를 강제한다.
//
// GitHub release 에 따라 "v0.4.0" 또는 "0.4.0" 형식이 혼재하므로 정규화 필요.
// 빈 문자열은 그대로 반환 (caller 가 IsValid 로 검증).
func normalizeTag(tag string) Version {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return Version("")
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return Version(tag)
}
