// dashboard_asset.go — /api/dashboard-assets REST 핸들러.
//
// 대시보드 패널이 참조하는 바이너리 자산(도면 이미지 등)을 snapshot 과 **분리**해 저장한다.
//
// 왜 분리하는가: 대시보드 snapshot PUT 은 256KB 상한이 걸려 있다(maxDashboardPayloadBytes).
// 도면 사진을 data-URL 로 패널 config 에 박으면 그 한 장이 전체 예산을 삼켜 대시보드 저장이
// 통째로 실패한다(다른 패널의 변경까지 함께 유실). 자산을 여기로 빼면 snapshot 에는 id 문자열만
// 남고, 이미지는 자기 상한(maxAssetPayloadBytes)만 지키면 된다.
//
// 전송 형식이 왜 JSON(data-URL)인가: 인증이 Authorization 헤더 기반이라 <img src> 직접 참조가
// 불가능하다(브라우저가 헤더를 싣지 않는다). 어차피 클라이언트가 인증된 요청으로 받아와야 하므로,
// 원시 바이트 스트리밍 대신 기존 JSON 규약을 그대로 쓰고 클라이언트가 data-URL 을 src 에 넣는다.
// 이 선택은 새 응답 writer 배관을 만들지 않는다.

package handler

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

// maxAssetPayloadBytes 는 자산 업로드 요청 본문의 상한이다.
//
// 이미지 자체 상한(웹 UI 8MB) + base64 인플레이션(약 1.34배) + JSON 봉투를 수용하도록 12MB 로
// 둔다. snapshot 상한(256KB)과 **의도적으로 다른 축**이다 — 그것이 이 분리의 목적이다.
const maxAssetPayloadBytes = 12 * 1024 * 1024

// 허용 MIME 화이트리스트. 임의 바이너리 보관소로 전용되지 않도록 이미지로 제한한다.
var allowedAssetMIME = map[string]bool{
	"image/png":     true,
	"image/jpeg":    true,
	"image/gif":     true,
	"image/webp":    true,
	"image/svg+xml": true,
	"image/bmp":     true,
}

// DashboardAssetHandler 는 대시보드 자산 REST 엔드포인트를 처리한다.
type DashboardAssetHandler struct {
	repo   storage.DashboardAssetRepository
	logger *slog.Logger
}

// NewDashboardAssetHandler 는 새 DashboardAssetHandler 를 생성한다.
func NewDashboardAssetHandler(repo storage.DashboardAssetRepository, logger *slog.Logger) *DashboardAssetHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DashboardAssetHandler{repo: repo, logger: logger}
}

// RegisterRoutes 는 자산 라우트를 등록한다.
//
//	POST /dashboard-assets      - 업로드(내용 해시 id 반환, 멱등)
//	GET  /dashboard-assets/{id} - 조회(data-URL)
func (h *DashboardAssetHandler) RegisterRoutes(g *api.RouteGroup) {
	g.POST("/dashboard-assets", h.upload)
	g.GET("/dashboard-assets/{id}", h.get)
}

// assetUploadRequest 는 업로드 요청 본문이다. data_url 은 `data:<mime>;base64,<payload>`.
type assetUploadRequest struct {
	DataURL string `json:"data_url"`
}

// assetMetaResponse 는 업로드 응답(메타데이터만 — 바이트는 되돌려 보내지 않는다).
type assetMetaResponse struct {
	ID   string `json:"id"`
	MIME string `json:"mime"`
	Size int64  `json:"size"`
}

// assetGetResponse 는 조회 응답이다. 클라이언트는 data_url 을 그대로 img src 에 넣는다.
type assetGetResponse struct {
	ID      string `json:"id"`
	MIME    string `json:"mime"`
	Size    int64  `json:"size"`
	DataURL string `json:"data_url"`
}

// upload 는 data-URL 을 디코드해 자산으로 저장하고 id 를 반환한다.
//
// 같은 이미지를 다시 올리면 같은 id 가 반환된다(내용 주소화 — 저장소가 멱등).
func (h *DashboardAssetHandler) upload(ctx api.Context) error {
	if ctx.UserID() == "" {
		return api.ErrUnauthorized
	}

	body, err := readLimitedBody(ctx, maxAssetPayloadBytes)
	if err != nil {
		if errors.Is(err, errPayloadTooLarge) {
			return errPayloadTooLargeAPI()
		}
		return api.ErrBadRequest.WithMessage("read body: " + err.Error())
	}

	var req assetUploadRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}

	mime, data, derr := decodeDataURL(req.DataURL)
	if derr != nil {
		return api.ErrBadRequest.WithMessage(derr.Error())
	}
	if !allowedAssetMIME[mime] {
		return api.ErrBadRequest.WithMessage("지원하지 않는 이미지 형식입니다: " + mime)
	}

	rec, perr := h.repo.Put(ctx.Context(), mime, data, time.Now().UnixMilli())
	if perr != nil {
		h.logger.Error("대시보드 자산 저장 실패", "error", perr)
		return api.ErrInternalServer.WithMessage("자산 저장 실패")
	}
	h.logger.Info("대시보드 자산 저장", "id", rec.ID, "mime", rec.MIME, "size", rec.Size)

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(assetMetaResponse{
		ID:   rec.ID,
		MIME: rec.MIME,
		Size: rec.Size,
	}))
}

// get 은 id 로 자산을 조회해 data-URL 로 반환한다.
func (h *DashboardAssetHandler) get(ctx api.Context) error {
	if ctx.UserID() == "" {
		return api.ErrUnauthorized
	}
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("id 가 필요합니다")
	}

	rec, err := h.repo.Get(ctx.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrDashboardAssetNotFound) {
			return api.ErrNotFound.WithMessage("자산을 찾을 수 없습니다")
		}
		h.logger.Error("대시보드 자산 조회 실패", "id", id, "error", err)
		return api.ErrInternalServer.WithMessage("자산 조회 실패")
	}

	// 자산은 내용 주소화(id = 내용 해시)라 절대 변하지 않는다 → 장기 캐시가 안전하다.
	ctx.SetHeader("Cache-Control", "private, max-age=31536000, immutable")

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(assetGetResponse{
		ID:      rec.ID,
		MIME:    rec.MIME,
		Size:    rec.Size,
		DataURL: "data:" + rec.MIME + ";base64," + base64.StdEncoding.EncodeToString(rec.Data),
	}))
}

// decodeDataURL 은 `data:<mime>;base64,<payload>` 를 (mime, bytes) 로 분해한다.
//
// base64 이외의 인코딩(퍼센트 인코딩 data-URL)은 지원하지 않는다 — 브라우저 FileReader
// 의 readAsDataURL 이 항상 base64 를 만들기 때문이고, 지원 범위를 좁혀 파싱 표면을 줄인다.
func decodeDataURL(s string) (string, []byte, error) {
	if !strings.HasPrefix(s, "data:") {
		return "", nil, errors.New("data_url 은 data: 로 시작해야 합니다")
	}
	comma := strings.IndexByte(s, ',')
	if comma < 0 {
		return "", nil, errors.New("data_url 형식이 올바르지 않습니다")
	}
	meta := s[len("data:"):comma]
	payload := s[comma+1:]
	if !strings.HasSuffix(meta, ";base64") {
		return "", nil, errors.New("data_url 은 base64 인코딩이어야 합니다")
	}
	mime := strings.TrimSuffix(meta, ";base64")
	if mime == "" {
		return "", nil, errors.New("data_url 에 MIME 타입이 없습니다")
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return "", nil, errors.New("data_url base64 디코드 실패")
	}
	if len(data) == 0 {
		return "", nil, errors.New("data_url 이 비어 있습니다")
	}
	return mime, data, nil
}
