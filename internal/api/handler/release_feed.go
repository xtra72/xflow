// release_feed.go 는 관리 서버가 호스팅하는 프로그램 이미지를 GitHub-Releases 호환
// 형식으로 노드에 서빙하는 익명(인증 없음) 피드 핸들러이다. 노드 측 updater.Checker /
// Downloader 코드를 변경하지 않고도 동작하도록, 응답을 GitHub Releases API 응답 형태로
// 그대로 만든다(엔벨로프로 감싸지 않는다 — Checker 가 직접 unmarshal).
//
// 라우트(모두 raw 핸들러 — 인증 미들웨어 우회. 노드 Checker 는 토큰을 보내지 않으므로
// /api/v1/* Auth 게이트를 통과할 수 없다. 따라서 RegisterRawHandler 로 등록해 익명 접근을
// 보장한다):
//
//	GET /api/v1/updates/releases/latest                          → 최신 stable 단일 객체
//	GET /api/v1/updates/releases                                 → 전체 릴리즈 배열(semver desc)
//	GET /api/v1/updates/releases/download/{version}/{filename}   → 바이너리/.sig/checksum.txt 스트림
//
// 노드 update_url 예: https://{host}/api/v1/updates → Checker 가 /releases/latest 또는
// /releases 를 덧붙인다(updater.Checker.endpointURL 참조).
//
// 채널 매핑(Checker 호환): prerelease = (channel != "stable"). nightly 버전은 tag_name 에
// "-nightly" 식별자를 포함해야 한다(예: v1.2.3-nightly.20260620). Checker 의 채널 선택은
// beta = tag 에 "-beta" 포함 OR prerelease==true 첫 항목, nightly = tag 에 "-nightly"
// 포함 첫 항목이다.
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/storage"
)

// ghAsset 은 GitHub release asset 의 응답 표현이다(노드 Checker 의 githubAsset 과 필드명 일치).
type ghAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
	Size        int64  `json:"size"`
}

// ghRelease 는 GitHub release 의 응답 표현이다(노드 Checker 의 githubRelease 와 필드명 일치).
// published_at 은 RFC3339 문자열로 직렬화한다(Checker 가 time.Time 으로 파싱).
type ghRelease struct {
	TagName     string    `json:"tag_name"`
	Name        string    `json:"name"`
	PublishedAt string    `json:"published_at"`
	Prerelease  bool      `json:"prerelease"`
	HTMLURL     string    `json:"html_url"`
	Assets      []ghAsset `json:"assets"`
}

// ReleaseFeedHandler 는 익명 GitHub-호환 릴리즈 피드 + 다운로드를 처리한다.
type ReleaseFeedHandler struct {
	repo    *storage.ReleaseRepository
	pubBase string // 설정된 공개 base URL(예: https://mgmt.example.com). 빈 값이면 요청에서 유도.
	logger  *slog.Logger
}

// NewReleaseFeedHandler 는 ReleaseFeedHandler 를 생성한다. publicBaseURL 이 비어 있으면
// 다운로드 URL 의 base 를 들어오는 요청 Host(+X-Forwarded-*)에서 유도하되 scheme 은 항상
// https 로 강제한다(노드 Downloader 가 모든 browser_download_url 에 https 를 강제하므로).
func NewReleaseFeedHandler(repo *storage.ReleaseRepository, publicBaseURL string, logger *slog.Logger) *ReleaseFeedHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReleaseFeedHandler{
		repo:    repo,
		pubBase: strings.TrimRight(strings.TrimSpace(publicBaseURL), "/"),
		logger:  logger,
	}
}

// 라우트 패턴(raw 핸들러 등록용).
const (
	releaseFeedLatestPattern   = "GET /api/v1/updates/releases/latest"
	releaseFeedListPattern     = "GET /api/v1/updates/releases"
	releaseFeedDownloadPattern = "GET /api/v1/updates/releases/download/{version}/{filename}"
)

// RegisterRawHandlers 는 익명 피드/다운로드 라우트를 raw 핸들러로 등록한다.
// raw 등록 이유: 노드 Checker/Downloader 는 인증 토큰을 보내지 않으므로 /api/v1/* Auth
// 미들웨어를 우회해야 하고, 다운로드는 octet-stream 바이트를 직접 스트리밍해야 한다.
func (h *ReleaseFeedHandler) RegisterRawHandlers(register func(pattern string, handler http.HandlerFunc)) {
	register(releaseFeedLatestPattern, h.Latest)
	register(releaseFeedListPattern, h.List)
	register(releaseFeedDownloadPattern, h.Download)
}

// Latest 는 최신 stable 릴리즈를 GitHub 형식 단일 객체로 반환한다. 없으면 404.
// GET /api/v1/updates/releases/latest
func (h *ReleaseFeedHandler) Latest(w http.ResponseWriter, r *http.Request) {
	rec, ok, err := h.repo.ResolveLatestStable(r.Context())
	if err != nil {
		h.writeJSONError(w, http.StatusInternalServerError, "릴리즈 조회 실패")
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	base := h.resolveBase(r)
	h.writeJSON(w, http.StatusOK, h.toGitHub(rec, base))
}

// List 는 전체 릴리즈를 GitHub 형식 배열(semver 내림차순)로 반환한다. 비어 있으면 빈 배열.
// GET /api/v1/updates/releases
func (h *ReleaseFeedHandler) List(w http.ResponseWriter, r *http.Request) {
	recs, err := h.repo.ListReleases(r.Context())
	if err != nil {
		h.writeJSONError(w, http.StatusInternalServerError, "릴리즈 목록 조회 실패")
		return
	}
	base := h.resolveBase(r)
	out := make([]ghRelease, 0, len(recs))
	for _, rec := range recs {
		out = append(out, h.toGitHub(rec, base))
	}
	h.writeJSON(w, http.StatusOK, out)
}

// Download 는 저장된 바이너리/.sig/checksum.txt 를 raw octet-stream 으로 스트리밍한다.
// filename=checksum.txt 는 ChecksumFile() 로 즉석 생성한다. 그 외에는 OpenAsset() 으로
// 스트리밍한다. 경로 순회는 거부한다(저장소가 검증).
// GET /api/v1/updates/releases/download/{version}/{filename}
func (h *ReleaseFeedHandler) Download(w http.ResponseWriter, r *http.Request) {
	version := r.PathValue("version")
	filename := r.PathValue("filename")
	if version == "" || filename == "" {
		http.NotFound(w, r)
		return
	}

	// checksum.txt 는 저장 파일이 아니라 sha256 으로부터 즉석 생성한다.
	if filename == "checksum.txt" {
		data, err := h.repo.ChecksumFile(r.Context(), version)
		if err != nil {
			if errors.Is(err, storage.ErrReleaseNotFound) {
				http.NotFound(w, r)
				return
			}
			h.writeJSONError(w, http.StatusInternalServerError, "체크섬 생성 실패")
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
		return
	}

	rc, size, err := h.repo.OpenAsset(r.Context(), version, filename)
	if err != nil {
		if errors.Is(err, storage.ErrReleaseAssetNotFound) {
			http.NotFound(w, r)
			return
		}
		if errors.Is(err, storage.ErrInvalidAssetFilename) {
			http.Error(w, "invalid filename", http.StatusBadRequest)
			return
		}
		h.writeJSONError(w, http.StatusInternalServerError, "에셋 스트리밍 실패")
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.WriteHeader(http.StatusOK)
	if _, werr := io.Copy(w, rc); werr != nil {
		// 헤더가 이미 전송된 뒤이므로 상태 코드를 바꿀 수 없다 — 로그만 남긴다.
		h.logger.Warn("에셋 스트리밍 중 오류", "version", version, "filename", filename, "error", werr)
	}
}

// toGitHub 은 ReleaseRecord 를 GitHub 형식 응답으로 변환한다. asset 목록에는 각 바이너리
// (browser_download_url, .sig 동반 시 별도 항목) + checksum.txt 항목을 포함한다. 모든
// download URL 은 절대 https URL 이다(노드 Downloader 의 https 강제 충족).
func (h *ReleaseFeedHandler) toGitHub(rec storage.ReleaseRecord, base string) ghRelease {
	assets := make([]ghAsset, 0, len(rec.Assets)*2+1)
	hasAnyBinary := false
	for _, a := range rec.Assets {
		assets = append(assets, ghAsset{
			Name:        a.Filename,
			DownloadURL: downloadURL(base, rec.Version, a.Filename),
			Size:        a.Size,
		})
		hasAnyBinary = true
		if a.HasSig {
			sigName := a.Filename + ".sig"
			assets = append(assets, ghAsset{
				Name:        sigName,
				DownloadURL: downloadURL(base, rec.Version, sigName),
				// .sig 크기는 메타에 저장하지 않으므로 0(노드 Downloader 는 size 를 진행률
				// 계산에만 쓰고 검증에는 쓰지 않는다).
				Size: 0,
			})
		}
	}
	// checksum.txt 는 바이너리 asset 이 하나라도 있을 때만 노출한다(즉석 생성).
	if hasAnyBinary {
		assets = append(assets, ghAsset{
			Name:        "checksum.txt",
			DownloadURL: downloadURL(base, rec.Version, "checksum.txt"),
			Size:        0,
		})
	}

	return ghRelease{
		TagName:     rec.Version,
		Name:        rec.Version,
		PublishedAt: time.UnixMilli(rec.PublishedAtMs).UTC().Format(time.RFC3339),
		// 채널 매핑: stable 만 정식 릴리즈. beta/nightly 는 prerelease=true.
		Prerelease: rec.Channel != "stable",
		HTMLURL:    base + "/api/v1/updates/releases/" + rec.Version,
		Assets:     assets,
	}
}

// resolveBase 는 다운로드 URL 의 public base 를 결정한다. 설정값(pubBase)이 있으면 우선
// 사용하고, 없으면 요청 Host 에서 유도한다(X-Forwarded-Host 우선). scheme 은 항상 https 로
// 강제한다(노드 Downloader 가 모든 browser_download_url 에 https 를 강제하므로 — X-Forwarded-Proto
// 가 http 여도 https 로 고정).
func (h *ReleaseFeedHandler) resolveBase(r *http.Request) string {
	if h.pubBase != "" {
		return h.pubBase
	}
	host := r.Header.Get("X-Forwarded-Host")
	if host == "" {
		host = r.Host
	}
	host = strings.TrimSpace(host)
	return "https://" + strings.TrimRight(host, "/")
}

// writeJSON 은 GitHub 형식 응답을 엔벨로프 없이 직접 직렬화한다(노드 Checker 가 응답을
// githubRelease 로 직접 unmarshal 하므로 dto 엔벨로프로 감싸지 않는다).
func (h *ReleaseFeedHandler) writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.logger.Warn("릴리즈 피드 JSON 인코딩 실패", "error", err)
	}
}

// writeJSONError 는 익명 피드용 간단한 JSON 에러를 쓴다(엔벨로프 비사용 — 노드 Checker 는
// 에러 바디를 GitHub 응답으로 파싱하지 않고 status code 만 본다).
func (h *ReleaseFeedHandler) writeJSONError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_, _ = w.Write([]byte(`{"message":` + strconv.Quote(msg) + `}`))
}

// downloadURL 은 절대 https 다운로드 URL 을 구성한다.
func downloadURL(base, version, filename string) string {
	return base + "/api/v1/updates/releases/download/" + version + "/" + filename
}
