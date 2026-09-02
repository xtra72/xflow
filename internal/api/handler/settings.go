// settings.go 는 전역(앱 전체 공유) key-value 설정 API 핸들러이다.
//
// 디바이스 컬럼 구성 등 "전역 1벌"로 공유되는 UI/서버 설정을 저장/조회한다.
// 키 네임스페이스는 프론트엔드가 정한다(예: "device-list-columns").
//
// 엔드포인트(범용 — 추후 다른 UI 설정도 재사용):
//
//	GET /settings/{key}  -> 저장된 value(JSON) 반환. 없으면 404.
//	PUT /settings/{key}  -> body(JSON value)를 불투명 문자열로 저장(UPSERT).
//
// 서버는 value 의 스키마를 강제하지 않는다. 단, 본문이 유효한 JSON 인지만 검증해
// 손상된 페이로드 저장을 막는다(저장은 정규화 없는 원본 보존).
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/storage"
)

// maxSettingsValueBytes 는 설정 value(JSON)의 최대 크기이다(256KB). UI 설정 용도로
// 충분히 크면서 무제한 저장을 막는 안전 상한이다.
const maxSettingsValueBytes = 256 * 1024

// SettingsRepository 는 전역 설정 영속화 인터페이스이다(storage.SettingsRepository 준용).
type SettingsRepository interface {
	GetSetting(ctx context.Context, key string) (string, error)
	SetSetting(ctx context.Context, key, value string) error
}

// SettingsResponse 는 설정 조회/저장 응답 DTO 이다.
//
// Value 는 저장된 불투명 JSON 을 그대로 담기 위해 json.RawMessage 로 노출한다(이중
// 인코딩 방지 — 클라이언트가 받은 그대로 재파싱 가능).
type SettingsResponse struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

// SettingsHandler 는 전역 설정 API 엔드포인트를 처리한다.
type SettingsHandler struct {
	repo   SettingsRepository
	logger *slog.Logger
}

// NewSettingsHandler 는 새 SettingsHandler 를 생성한다.
func NewSettingsHandler(repo SettingsRepository, logger *slog.Logger) *SettingsHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &SettingsHandler{repo: repo, logger: logger}
}

// RegisterRoutes 는 설정 라우트를 등록한다.
//
//	GET /settings/{key}
//	PUT /settings/{key}
func (h *SettingsHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — 전역 서버/UI 설정이므로 system.* 로 매핑한다.
	g.GETPerm("/settings/{key}", "system.read", h.Get)
	g.PUTPerm("/settings/{key}", "system.update", h.Put)
}

// Get 은 key 에 해당하는 설정 value 를 반환한다.
// GET /settings/{key}
func (h *SettingsHandler) Get(ctx api.Context) error {
	key := ctx.Param("key")
	if key == "" {
		return api.ErrBadRequest.WithMessage("setting key is required")
	}

	value, err := h.repo.GetSetting(ctx.Context(), key)
	if err != nil {
		if errors.Is(err, storage.ErrSettingNotFound) {
			return api.ErrNotFound.WithMessage(fmt.Sprintf("setting %q not found", key))
		}
		h.logger.Error("설정 조회 실패", "key", key, "error", err)
		return api.ErrInternalServer.WithMessage("failed to read setting")
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(SettingsResponse{
		Key:   key,
		Value: json.RawMessage(value),
	}))
}

// Put 은 key 에 body(JSON value)를 저장한다(UPSERT).
// PUT /settings/{key}
//
// 본문은 불투명 JSON value 로 저장된다(서버는 스키마 강제 안 함). 유효한 JSON 인지만
// 검증하며, 256KB 초과 시 413 을 반환한다.
func (h *SettingsHandler) Put(ctx api.Context) error {
	key := ctx.Param("key")
	if key == "" {
		return api.ErrBadRequest.WithMessage("setting key is required")
	}

	body, err := readSettingsBody(ctx, maxSettingsValueBytes)
	if err != nil {
		if errors.Is(err, errSettingsTooLarge) {
			return &api.APIError{
				HTTPCode: http.StatusRequestEntityTooLarge,
				Code:     "PAYLOAD_TOO_LARGE",
				Message:  fmt.Sprintf("value exceeds %d bytes", maxSettingsValueBytes),
			}
		}
		return api.ErrBadRequest.WithMessage("failed to read request body")
	}

	// 유효한 JSON 인지만 검증 — 스키마는 강제하지 않는다(불투명 저장).
	if !json.Valid(body) {
		return api.ErrBadRequest.WithMessage("value must be valid JSON")
	}

	if err := h.repo.SetSetting(ctx.Context(), key, string(body)); err != nil {
		h.logger.Error("설정 저장 실패", "key", key, "error", err)
		return api.ErrInternalServer.WithMessage("failed to persist setting")
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(SettingsResponse{
		Key:   key,
		Value: json.RawMessage(body),
	}))
}

// errSettingsTooLarge 는 readSettingsBody 의 내부 sentinel(413 매핑용)이다.
var errSettingsTooLarge = errors.New("settings value too large")

// readSettingsBody 는 요청 본문을 maxBytes 까지만 읽어 반환한다.
//
// LimitReader 로 maxBytes+1 만큼 읽어 정확히 한도 초과를 검출한다. httpContext 의
// raw request 에 접근하는 우회 인터페이스를 우선 사용하고, 없으면 Bind 폴백을 쓴다
// (테스트 환경 호환 — dashboard.readLimitedBody 와 동일 전략).
func readSettingsBody(ctx api.Context, maxBytes int64) ([]byte, error) {
	type requester interface {
		Request() *http.Request
	}
	if req, ok := ctx.(requester); ok && req.Request() != nil && req.Request().Body != nil {
		r := io.LimitReader(req.Request().Body, maxBytes+1)
		data, err := io.ReadAll(r)
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > maxBytes {
			return nil, errSettingsTooLarge
		}
		return data, nil
	}

	// Fallback: 빈 body 는 빈 슬라이스로 처리(이후 json.Valid 가 거부).
	var raw json.RawMessage
	if err := ctx.Bind(&raw); err != nil {
		if errors.Is(err, io.EOF) {
			return []byte{}, nil
		}
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, errSettingsTooLarge
	}
	return raw, nil
}
