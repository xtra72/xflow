// release_admin.go 는 관리 서버가 호스팅하는 프로그램 이미지의 관리자(admin) 관리 API 를
// 제공한다(@SPEC:SPEC-REMOTE-001 버전 관리 — 서버 호스팅 이미지). 관리자가 릴리즈를
// 생성/삭제하고 아키텍처별 바이너리 + Ed25519 서명을 업로드한다. 노드-측 익명 피드
// (release_feed.go)가 본 저장소를 출처로 GitHub-호환 응답을 만든다.
//
// 라우트:
//
//	GET    /remote/releases                              — 전체 릴리즈 admin 뷰(asset 상세 포함)
//	POST   /remote/releases                              — 릴리즈 생성/갱신(version, channel?, notes?)
//	DELETE /remote/releases/{version}                    — 릴리즈 삭제(행 + 디스크 디렉토리)
//	DELETE /remote/releases/{version}/assets/{os}/{arch} — asset 삭제(행 + 파일)
//
// 업로드(raw 핸들러 — api.Context 는 multipart 접근을 노출하지 않으므로):
//
//	POST /api/v1/remote/releases/{version}/assets        — multipart/form-data 바이너리+서명 업로드
//
// 인증: JSON 라우트는 RouteGroup 의 Auth 미들웨어 + requireAdmin() 으로 admin 을 강제한다.
// raw 업로드 핸들러는 Auth 미들웨어를 우회하므로, remote_stream 의 authorizeAdmin 패턴을
// 준용해 JWT 를 직접 검증한다(Bearer 토큰 → role=admin).
package handler

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/internal/updater"
)

// maxUploadBytes 는 multipart 업로드 본문의 상한이다(200 MB). 바이너리 본체는 디스크로
// 스트리밍되지만, 과대 업로드로 인한 자원 고갈을 막기 위해 요청 본문 크기를 제한한다.
const maxUploadBytes = 200 << 20 // 200 MiB

// ReleaseAdminHandler 는 릴리즈 admin 관리 + multipart 업로드를 처리한다.
type ReleaseAdminHandler struct {
	repo   *storage.ReleaseRepository
	jwtSvc *auth.JWTService // raw 업로드 핸들러의 admin JWT 직접 검증용(nil=테스트 우회)
	logger *slog.Logger
}

// NewReleaseAdminHandler 는 ReleaseAdminHandler 를 생성한다. jwtSvc 가 nil 이면 raw 업로드
// 핸들러는 인증 우회 모드이다(테스트 전용 — 운영에서는 항상 jwtSvc 를 주입한다).
func NewReleaseAdminHandler(repo *storage.ReleaseRepository, jwtSvc *auth.JWTService, logger *slog.Logger) *ReleaseAdminHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ReleaseAdminHandler{repo: repo, jwtSvc: jwtSvc, logger: logger}
}

// releaseAssetDTO 는 admin 뷰의 asset 표현이다.
type releaseAssetDTO struct {
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Filename     string `json:"filename"`
	Size         int64  `json:"size"`
	SHA256       string `json:"sha256"`
	HasSig       bool   `json:"has_sig"`
	UploadedAtMs int64  `json:"uploaded_at_ms"`
}

// releaseDTO 는 admin 뷰의 릴리즈 표현이다(버전 + 채널 + 노트 + asset 상세).
type releaseDTO struct {
	Version       string            `json:"version"`
	Channel       string            `json:"channel"`
	Notes         string            `json:"notes"`
	PublishedAtMs int64             `json:"published_at_ms"`
	Assets        []releaseAssetDTO `json:"assets"`
}

// createReleaseRequest 는 릴리즈 생성/갱신 요청 본문이다.
type createReleaseRequest struct {
	Version string `json:"version"`
	Channel string `json:"channel,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

// RegisterRoutes 는 JSON admin 라우트를 RouteGroup 에 등록한다(Auth 미들웨어 + requireAdmin).
func (h *ReleaseAdminHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — 원격 하위 API 는 remote.* 단일 키로만 다룬다
	// (spec.md §1.3 비범위: 원격 노드 하위 API 의 세분 권한). 조회는 remote.read,
	// 그 외 모든 변경·명령은 remote.update 이다.
	g.GETPerm("/remote/releases", "remote.read", h.ListReleases)
	g.POSTPerm("/remote/releases", "remote.update", h.CreateRelease)
	g.DELETEPerm("/remote/releases/{version}", "remote.update", h.DeleteRelease)
	g.DELETEPerm("/remote/releases/{version}/assets/{os}/{arch}", "remote.update", h.DeleteAsset)
}

// uploadPattern 은 raw multipart 업로드 라우트 패턴이다(RegisterRawHandler 등록용).
const uploadPattern = "POST /api/v1/remote/releases/{version}/assets"

// RegisterRawHandlers 는 multipart 업로드 라우트를 raw 핸들러로 등록한다. raw 등록 이유:
// api.Context 는 multipart/form-data 접근을 노출하지 않으므로 *http.Request 에 직접
// 접근해야 한다.
func (h *ReleaseAdminHandler) RegisterRawHandlers(register func(pattern string, handler http.HandlerFunc)) {
	register(uploadPattern, h.UploadAsset)
}

// ListReleases 는 전체 릴리즈를 admin 뷰(asset 상세 포함, semver 내림차순)로 반환한다.
// GET /remote/releases
func (h *ReleaseAdminHandler) ListReleases(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	recs, err := h.repo.ListReleases(ctx.Context())
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	out := make([]releaseDTO, 0, len(recs))
	for _, rec := range recs {
		out = append(out, toReleaseDTO(rec))
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{"releases": out}))
}

// CreateRelease 는 릴리즈를 생성/갱신한다. published_at 은 핸들러가 now(epoch ms)로 부여한다.
// POST /remote/releases {version, channel?, notes?}
func (h *ReleaseAdminHandler) CreateRelease(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	var req createReleaseRequest
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("요청 본문 파싱 실패")
	}
	if !updater.Version(req.Version).IsValid() {
		return api.ErrBadRequest.WithMessage("version 은 vMAJOR.MINOR.PATCH 형식이어야 합니다")
	}
	channel := req.Channel
	if channel == "" {
		channel = "stable"
	}
	nowMs := time.Now().UnixMilli()
	if err := h.repo.UpsertRelease(ctx.Context(), req.Version, channel, req.Notes, nowMs); err != nil {
		if errors.Is(err, storage.ErrInvalidReleaseChannel) {
			return api.ErrBadRequest.WithMessage("channel 은 stable, beta, nightly 중 하나여야 합니다")
		}
		if errors.Is(err, storage.ErrInvalidReleaseVersion) {
			return api.ErrBadRequest.WithMessage("version 은 vMAJOR.MINOR.PATCH 형식이어야 합니다")
		}
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	h.logger.Info("릴리즈 생성/갱신", "version", req.Version, "channel", channel, "actor", ctx.UserID())
	rec, err := h.repo.GetRelease(ctx.Context(), req.Version)
	if err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toReleaseDTO(rec)))
}

// DeleteRelease 는 릴리즈와 그 디스크 디렉토리를 삭제한다(멱등).
// DELETE /remote/releases/{version}
func (h *ReleaseAdminHandler) DeleteRelease(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	version := ctx.Param("version")
	if err := h.repo.DeleteRelease(ctx.Context(), version); err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	h.logger.Info("릴리즈 삭제", "version", version, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{"version": version, "deleted": true}))
}

// DeleteAsset 은 (version,os,arch) asset 행과 디스크 파일을 삭제한다(멱등).
// DELETE /remote/releases/{version}/assets/{os}/{arch}
func (h *ReleaseAdminHandler) DeleteAsset(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	version := ctx.Param("version")
	goos := ctx.Param("os")
	arch := ctx.Param("arch")
	if err := h.repo.DeleteAsset(ctx.Context(), version, goos, arch); err != nil {
		return api.ErrInternalServer.WithMessage(err.Error())
	}
	h.logger.Info("릴리즈 에셋 삭제", "version", version, "os", goos, "arch", arch, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"version": version, "os": goos, "arch": arch, "deleted": true,
	}))
}

// UploadAsset 은 multipart/form-data 로 바이너리 + 서명을 업로드한다(raw 핸들러). 폼 필드:
// os, arch(텍스트), binary(필수 파일), signature(선택 파일). admin JWT 를 직접 검증한다.
// POST /api/v1/remote/releases/{version}/assets
func (h *ReleaseAdminHandler) UploadAsset(w http.ResponseWriter, r *http.Request) {
	if !h.authorizeAdmin(r) {
		http.Error(w, "admin 권한이 필요합니다", http.StatusForbidden)
		return
	}

	version := r.PathValue("version")
	if !updater.Version(version).IsValid() {
		http.Error(w, "version 은 vMAJOR.MINOR.PATCH 형식이어야 합니다", http.StatusBadRequest)
		return
	}

	// 요청 본문 상한을 적용한다(과대 업로드 자원 고갈 방지).
	r.Body = http.MaxBytesReader(w, r.Body, maxUploadBytes)
	// 파일은 디스크로 스트리밍되므로 메모리 버퍼는 작게(32 MiB) 둔다.
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "multipart/form-data 파싱 실패: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() {
		if r.MultipartForm != nil {
			_ = r.MultipartForm.RemoveAll()
		}
	}()

	goos := r.FormValue("os")
	arch := r.FormValue("arch")
	if goos == "" || arch == "" {
		http.Error(w, "os 와 arch 폼 필드가 필요합니다", http.StatusBadRequest)
		return
	}

	binFile, _, err := r.FormFile("binary")
	if err != nil {
		http.Error(w, "binary 파일이 필요합니다: "+err.Error(), http.StatusBadRequest)
		return
	}
	defer func() { _ = binFile.Close() }()

	// signature 는 선택이지만, 사전 서명 모델상 거의 항상 제공된다. 미제공 시 nil 로 두면
	// PutAsset 이 .sig 를 기록하지 않는다(has_sig=false).
	var sigReader io.Reader
	if sigFile, _, serr := r.FormFile("signature"); serr == nil {
		defer func() { _ = sigFile.Close() }()
		sigReader = sigFile
	}

	nowMs := time.Now().UnixMilli()
	rec, perr := h.repo.PutAsset(r.Context(), version, goos, arch, binFile, sigReader, nowMs)
	if perr != nil {
		if errors.Is(perr, storage.ErrInvalidReleaseVersion) {
			http.Error(w, "version 은 vMAJOR.MINOR.PATCH 형식이어야 합니다", http.StatusBadRequest)
			return
		}
		h.logger.Error("릴리즈 에셋 업로드 실패", "version", version, "os", goos, "arch", arch, "error", perr)
		http.Error(w, "에셋 저장 실패: "+perr.Error(), http.StatusInternalServerError)
		return
	}

	h.logger.Info("릴리즈 에셋 업로드",
		"version", version, "os", goos, "arch", arch,
		"size", rec.Size, "sha256", rec.SHA256, "has_sig", rec.HasSig)

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(dto.NewSuccessResponse(toReleaseAssetDTO(rec))); err != nil {
		// 헤더가 이미 전송된 뒤이므로 상태 코드를 바꿀 수 없다 — 로그만 남긴다.
		h.logger.Warn("릴리즈 admin 업로드 응답 인코딩 실패", "error", err)
	}
}

// authorizeAdmin 은 raw 업로드 요청이 admin 권한인지 확인한다. jwtSvc 설정 시 Bearer
// 토큰을 검증해 role=admin 을 강제하고, 미설정(테스트) 시 컨텍스트 역할을 사용한다
// (remote_stream.authorizeAdmin 패턴 준용).
func (h *ReleaseAdminHandler) authorizeAdmin(r *http.Request) bool {
	if h.jwtSvc == nil {
		return testRoleFromContext(r.Context()) == "admin"
	}
	token := bearerOrQueryToken(r)
	if token == "" || h.jwtSvc.IsBlacklisted(token) {
		return false
	}
	claims, err := h.jwtSvc.ValidateToken(token)
	if err != nil {
		return false
	}
	return claims.Role == "admin"
}

// toReleaseDTO 는 storage.ReleaseRecord 를 admin DTO 로 변환한다.
func toReleaseDTO(rec storage.ReleaseRecord) releaseDTO {
	assets := make([]releaseAssetDTO, 0, len(rec.Assets))
	for _, a := range rec.Assets {
		assets = append(assets, toReleaseAssetDTO(a))
	}
	return releaseDTO{
		Version:       rec.Version,
		Channel:       rec.Channel,
		Notes:         rec.Notes,
		PublishedAtMs: rec.PublishedAtMs,
		Assets:        assets,
	}
}

// toReleaseAssetDTO 는 storage.ReleaseAssetRecord 를 admin DTO 로 변환한다.
func toReleaseAssetDTO(a storage.ReleaseAssetRecord) releaseAssetDTO {
	return releaseAssetDTO{
		OS:           a.OS,
		Arch:         a.Arch,
		Filename:     a.Filename,
		Size:         a.Size,
		SHA256:       a.SHA256,
		HasSig:       a.HasSig,
		UploadedAtMs: a.UploadedAtMs,
	}
}
