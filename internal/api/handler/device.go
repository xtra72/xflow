package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/device"
)

// DeviceRegistry 는 디바이스 레지스트리 인터페이스이다.
type DeviceRegistry interface {
	List(filter device.DeviceFilter) []device.Device
	Get(id string) (device.Device, error)
	Count() int
	Execute(ctx context.Context, id string, command string, params map[string]any) (map[string]any, error)
	SetMetadata(id string, metadata device.DeviceMetadata) error
	GetMetadata(id string) (device.DeviceMetadata, error)
}

// MetadataRepository 는 디바이스 메타데이터 영속화 인터페이스이다.
type MetadataRepository interface {
	Save(ctx context.Context, deviceID string, metadata device.DeviceMetadata) error
	Get(ctx context.Context, deviceID string) (device.DeviceMetadata, error)
	Delete(ctx context.Context, deviceID string) error
}

// DeviceResponse 는 디바이스 목록 응답 DTO이다.
type DeviceResponse struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Type         string                 `json:"type"`
	Protocol     string                 `json:"protocol"`
	AgentName    string                 `json:"agent_name"`
	Online       bool                   `json:"online"`
	LastSeen     time.Time              `json:"last_seen"`
	Source       string                 `json:"source"`
	Capabilities []string               `json:"capabilities"`
	Metadata     *device.DeviceMetadata `json:"metadata,omitempty"`
}

// DeviceDetailResponse 는 디바이스 상세 조회 응답 DTO이다.
type DeviceDetailResponse struct {
	DeviceResponse
	State    *device.DeviceState  `json:"state,omitempty"`
	Commands []device.CommandSpec `json:"commands,omitempty"`
}

// ExecuteRequest 는 디바이스 커맨드 실행 요청이다.
type ExecuteRequest struct {
	Command string         `json:"command"`
	Params  map[string]any `json:"params,omitempty"`
}

// DeviceHandler 는 디바이스 관련 API 엔드포인트를 처리한다.
type DeviceHandler struct {
	registry     DeviceRegistry
	metadataRepo MetadataRepository
	events       *ws.EventPublisher // nil 허용
	logger       *slog.Logger
}

// DeviceHandlerOption 은 DeviceHandler 의 선택적 설정 함수이다.
type DeviceHandlerOption func(*DeviceHandler)

// WithDeviceEventPublisher 는 DeviceHandler 에 EventPublisher 를 설정한다.
func WithDeviceEventPublisher(ep *ws.EventPublisher) DeviceHandlerOption {
	return func(h *DeviceHandler) {
		h.events = ep
	}
}

// NewDeviceHandler 는 새 DeviceHandler를 생성한다.
func NewDeviceHandler(registry DeviceRegistry, metadataRepo MetadataRepository, logger *slog.Logger, opts ...DeviceHandlerOption) *DeviceHandler {
	if logger == nil {
		logger = slog.Default()
	}
	h := &DeviceHandler{
		registry:     registry,
		metadataRepo: metadataRepo,
		logger:       logger,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// RegisterRoutes 는 디바이스 라우트를 등록한다.
//
// Routes:
//
//	GET    /devices              -> List
//	GET    /devices/{id}         -> Get
//	POST   /devices/{id}/execute -> Execute
//	PUT    /devices/{id}/metadata -> UpdateMetadata
//	DELETE /devices/{id}/metadata -> DeleteMetadata
func (h *DeviceHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/devices", h.List)
	g.GET("/devices/{id}", h.Get)
	g.POST("/devices/{id}/execute", h.Execute)
	g.PUT("/devices/{id}/metadata", h.UpdateMetadata)
	g.DELETE("/devices/{id}/metadata", h.DeleteMetadata)
}

// List 는 필터를 적용하여 디바이스 목록을 반환한다.
// GET /devices?protocol=nasa&agent=agent1&type=indoor&online=true&group=1f&tags=tag1,tag2
func (h *DeviceHandler) List(ctx api.Context) error {
	filter := device.DeviceFilter{
		Protocol:  ctx.Query("protocol"),
		AgentName: ctx.Query("agent"),
		Type:      ctx.Query("type"),
		Group:     ctx.Query("group"),
	}

	if onlineStr := ctx.Query("online"); onlineStr != "" {
		online := onlineStr == "true"
		filter.Online = &online
	}

	if tagsStr := ctx.Query("tags"); tagsStr != "" {
		filter.Tags = strings.Split(tagsStr, ",")
	}

	devices := h.registry.List(filter)

	responses := make([]DeviceResponse, 0, len(devices))
	for _, d := range devices {
		responses = append(responses, deviceToResponse(d))
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(responses))
}

// Get 은 ID로 단일 디바이스 상세 정보를 반환한다.
// GET /devices/{id}
func (h *DeviceHandler) Get(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("device id is required")
	}

	d, err := h.registry.Get(id)
	if err != nil {
		return mapDeviceError(err)
	}

	meta, err := h.registry.GetMetadata(id)
	if err != nil {
		return mapDeviceError(err)
	}

	resp := DeviceDetailResponse{
		DeviceResponse: deviceToResponse(d),
	}

	// 메타데이터가 비어 있지 않으면 설정
	if meta.Name != "" || meta.Location != "" || len(meta.Tags) > 0 || meta.Group != "" || len(meta.Labels) > 0 || meta.Pinned != nil {
		resp.Metadata = &meta
		// 메타데이터의 Name이 설정되면 응답의 name을 오버라이드
		if meta.Name != "" {
			resp.DeviceResponse.Name = meta.Name
		}
	}

	// 상태 정보 포함
	state := d.State()
	resp.State = &state

	// ControllableDevice 인 경우 커맨드 목록 포함
	if controllable, ok := d.(device.ControllableDevice); ok {
		resp.Commands = controllable.Commands()
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// Execute 는 디바이스에 커맨드를 실행한다.
// POST /devices/{id}/execute
func (h *DeviceHandler) Execute(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("device id is required")
	}

	var req ExecuteRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if req.Command == "" {
		return api.ErrBadRequest.WithMessage("command is required")
	}

	result, err := h.registry.Execute(ctx.Context(), id, req.Command, req.Params)
	if err != nil {
		return mapDeviceError(err)
	}

	if h.events != nil {
		h.events.PublishDeviceStateChanged(id)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(result))
}

// UpdateMetadata 는 디바이스 메타데이터를 업데이트한다.
// PUT /devices/{id}/metadata
func (h *DeviceHandler) UpdateMetadata(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("device id is required")
	}

	var meta device.DeviceMetadata
	if err := ctx.Bind(&meta); err != nil {
		return err
	}

	// 레지스트리의 인메모리 메타데이터 업데이트
	if err := h.registry.SetMetadata(id, meta); err != nil {
		return mapDeviceError(err)
	}

	// 영속 저장소에 저장
	if err := h.metadataRepo.Save(ctx.Context(), id, meta); err != nil {
		h.logger.Error("디바이스 메타데이터 영속화 실패", "device_id", id, "error", err)
		return api.ErrInternalServer.WithMessage("failed to persist metadata")
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(meta))
}

// DeleteMetadata 는 디바이스 메타데이터를 삭제한다.
// DELETE /devices/{id}/metadata
func (h *DeviceHandler) DeleteMetadata(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("device id is required")
	}

	// 레지스트리의 인메모리 메타데이터를 빈 값으로 설정
	if err := h.registry.SetMetadata(id, device.DeviceMetadata{}); err != nil {
		return mapDeviceError(err)
	}

	// 영속 저장소에서 삭제
	if err := h.metadataRepo.Delete(ctx.Context(), id); err != nil {
		h.logger.Error("디바이스 메타데이터 삭제 실패", "device_id", id, "error", err)
		return api.ErrInternalServer.WithMessage("failed to delete metadata")
	}

	return ctx.NoContent(http.StatusNoContent)
}

// mapDeviceError 는 디바이스 도메인 에러를 APIError로 매핑한다.
func mapDeviceError(err error) *api.APIError {
	switch {
	case errors.Is(err, device.ErrDeviceNotFound):
		return api.ErrNotFound.WithMessage(err.Error())
	case errors.Is(err, device.ErrNotControllable):
		return api.ErrValidationFailed.WithMessage(err.Error())
	case errors.Is(err, device.ErrAgentStopped):
		return api.ErrServiceUnavailable.WithMessage(err.Error())
	case errors.Is(err, device.ErrCommandNotFound):
		return api.ErrBadRequest.WithMessage(err.Error())
	case errors.Is(err, device.ErrInvalidParams):
		return api.ErrValidationFailed.WithMessage(err.Error())
	default:
		return api.ErrInternalServer.WithMessage(err.Error())
	}
}

// deviceToResponse 는 device.Device를 DeviceResponse로 변환한다.
func deviceToResponse(d device.Device) DeviceResponse {
	meta := d.Metadata()
	resp := DeviceResponse{
		ID:           d.ID(),
		Name:         d.Name(),
		Type:         string(d.Type()),
		Protocol:     d.Protocol(),
		AgentName:    d.AgentName(),
		Online:       d.Online(),
		LastSeen:     d.LastSeen(),
		Source:       d.Source(),
		Capabilities: d.Capabilities(),
	}

	if meta.Name != "" || meta.Location != "" || len(meta.Tags) > 0 || meta.Group != "" || len(meta.Labels) > 0 || meta.Pinned != nil {
		resp.Metadata = &meta
	}

	return resp
}
