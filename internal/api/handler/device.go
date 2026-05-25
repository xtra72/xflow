package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/device"
	"github.com/xtra/xflow/internal/observe"
)

// compositeAliasSunsetHTTPDate 는 RFC 8594 Sunset 헤더 값이다.
//
// SPEC-DEVICE-IDENTITY-001 Phase B (M4 / B-T4): composite alias 사용은
// xflowd v1.0 (Phase D) 에서 제거 예정이며 plan.md § 2.4 에 따라 Phase B/C
// 완료 후 최소 6개월 호환 기간을 확보한다. 본 날짜는 보수적 placeholder 로,
// 실제 v1.0 릴리스 일정에 맞춰 단일 지점에서 갱신 가능하다.
const compositeAliasSunsetHTTPDate = "Sun, 31 Dec 2026 23:59:59 GMT"

// DeviceRegistry 는 디바이스 레지스트리 인터페이스이다.
//
// SPEC-DEVICE-IDENTITY-001 Phase B (B-T1) 에서 GetByUID / GetByAgentName /
// ResolveDevice 가 도입되어 UUID / agent/name 기반 1급 lookup 경로를 제공한다.
// 핸들러는 ResolveDevice 를 우선 사용하며 composite key 는 호환 alias 로
// dispatch 되어 Deprecation 헤더 + 메트릭 카운터가 부착된다 (B-T4).
type DeviceRegistry interface {
	List(filter device.DeviceFilter) []device.Device
	Get(id string) (device.Device, error)
	GetByUID(uid string) (device.Device, error)
	GetByAgentName(agentName, name string) (device.Device, error)
	ResolveDevice(ref string) (device.Device, device.DeviceRefKind, error)
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
//
// SPEC-DEVICE-IDENTITY-001 Phase A (M2): UID 필드를 1급으로 노출한다.
// 기존 ID (composite "agent:local_id") 는 그대로 유지하여 외부 클라이언트
// 하위 호환을 보장한다 (Phase A 비파괴). UID 가 빈 문자열인 경우 (UUID
// 발급 저장소 미설정 / 매핑 부재) omitempty 로 키 자체를 생략하여
// downstream 이 키 존재 여부로 graceful degradation 을 판단할 수 있다.
type DeviceResponse struct {
	ID           string                 `json:"id"`
	UID          string                 `json:"uid,omitempty"` // SPEC-DEVICE-IDENTITY-001 Phase A
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
// Routes (SPEC-DEVICE-IDENTITY-001 Phase B):
//
//	GET    /devices                  -> List
//	GET    /devices:resolve          -> ResolveByAgentName  (B-T5, M4/B-AC4)
//	GET    /devices/{ref}            -> Get (UUID / composite, B-T4)
//	GET    /devices/{agent}/{name}   -> GetByAgentName (B-T4, M4/B-AC3)
//	POST   /devices/{id}/execute     -> Execute
//	PUT    /devices/{id}/metadata    -> UpdateMetadata
//	DELETE /devices/{id}/metadata    -> DeleteMetadata
//
// Go 1.22+ ServeMux 패턴 정밀도:
//   - "/devices:resolve" 는 단일 경로 세그먼트 ("devices:resolve") 로
//     "/devices/{ref}" 와 분리된다 (슬래시 부재).
//   - "/devices/{ref}" 는 1 세그먼트 캡처로 UUID / composite ("agent:local_id")
//     모두 매칭된다.
//   - "/devices/{agent}/{name}" 는 2 세그먼트 캡처로 위 패턴과 disjoint 하다.
//   - {id}/{ref}/{agent}/{name} 은 모두 ServeMux 의 path-value 캡처이며
//     execute/metadata 등 더 긴 패턴이 우선한다 (Go 1.22 spec).
func (h *DeviceHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GET("/devices", h.List)
	g.GET("/devices:resolve", h.ResolveByAgentName) // B-T5
	g.GET("/devices/{ref}", h.Get)
	g.GET("/devices/{agent}/{name}", h.GetByAgentName) // B-T4 — 2 세그먼트 dispatch
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
		resp := deviceToResponse(d)

		// 레지스트리 메타데이터 병합 (사용자 정의 이름 등)
		if meta, err := h.registry.GetMetadata(d.ID()); err == nil {
			if meta.Name != "" || meta.Location != "" || len(meta.Tags) > 0 || meta.Group != "" || len(meta.Labels) > 0 || meta.Pinned != nil {
				resp.Metadata = &meta
				if meta.Name != "" {
					resp.Name = meta.Name
				}
			}
		}

		responses = append(responses, resp)
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(responses))
}

// Get 은 reference 로 단일 디바이스 상세 정보를 반환한다.
// GET /devices/{ref}
//
// SPEC-DEVICE-IDENTITY-001 Phase B (M4 / B-T4 / B-AC3, B-AC5):
//
//	{ref} 는 다음 3 가지 형식 중 하나로 dispatch 된다:
//	  - UUID v4 (예: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d")
//	    → registry.GetByUID, 1급 식별자, Deprecation 헤더 없음.
//	  - composite ("agent:local_id" — 예: "lgcnp:81")
//	    → registry.Get, 호환 alias, Deprecation: true + Sunset 헤더 부착 +
//	      observe.IncDeviceCompositeUse(CompositeUseSourceRESTURL) 카운터 증가.
//	  - 그 외 (UUID/composite/agent-name 어느 것도 아님)
//	    → HTTP 404 + 명시적 에러 메시지 (B-AC5).
//
//	agent/name 형식 ("lgcnp/indoor-1") 은 별도 라우트 GET /devices/{agent}/{name}
//	(GetByAgentName 핸들러) 가 처리하므로 본 핸들러에서는 도달하지 않는다.
func (h *DeviceHandler) Get(ctx api.Context) error {
	ref := ctx.Param("ref")
	if ref == "" {
		return api.ErrBadRequest.WithMessage("device reference is required")
	}

	d, kind, err := h.registry.ResolveDevice(ref)
	if err != nil {
		// kind 가 Unknown 이면 명시적 에러 메시지로 404 반환 (B-AC5).
		if kind == device.DeviceRefUnknown {
			return api.ErrNotFound.WithMessage(fmt.Sprintf(
				"device reference %q not found; expected UUID, agent/name, or legacy agent:local_id",
				ref,
			))
		}
		return mapDeviceError(err)
	}

	// composite alias 사용 — Deprecation 헤더 + 메트릭 카운터 부착 (B-T4).
	if kind == device.DeviceRefComposite {
		h.attachCompositeDeprecation(ctx)
	}

	return h.respondWithDeviceDetail(ctx, d)
}

// GetByAgentName 은 (agent, name) 쌍으로 디바이스를 조회한다 (Phase B 신규).
// GET /devices/{agent}/{name}
//
// SPEC-DEVICE-IDENTITY-001 Phase B (M4 / B-T4 / B-AC3):
//
//	agent/name 은 사람이 읽기 좋은 1급 reference 이다. composite 와 달리
//	Deprecation 헤더가 부착되지 않으며, Phase D 이후에도 유지된다.
//	매칭 실패 시 404, agent 또는 name 미지정 시 400.
func (h *DeviceHandler) GetByAgentName(ctx api.Context) error {
	agentName := ctx.Param("agent")
	name := ctx.Param("name")
	if agentName == "" || name == "" {
		return api.ErrBadRequest.WithMessage("agent and name are required")
	}

	d, err := h.registry.GetByAgentName(agentName, name)
	if err != nil {
		if errors.Is(err, device.ErrDeviceNotFound) {
			return api.ErrNotFound.WithMessage(fmt.Sprintf(
				"device %s/%s not found", agentName, name,
			))
		}
		return mapDeviceError(err)
	}

	return h.respondWithDeviceDetail(ctx, d)
}

// respondWithDeviceDetail 은 Get / GetByAgentName / ResolveByAgentName 공통으로
// 디바이스 상세 응답을 빌드하여 반환한다.
func (h *DeviceHandler) respondWithDeviceDetail(ctx api.Context, d device.Device) error {
	id := d.ID()

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

// attachCompositeDeprecation 은 composite alias 응답에 RFC 8594 Sunset / RFC 8594
// Deprecation 헤더를 부착하고 메트릭 카운터를 증가시킨다 (B-T4).
//
// 호출자: composite key 로 dispatch 된 모든 응답 경로 (현재는 Get 만).
func (h *DeviceHandler) attachCompositeDeprecation(ctx api.Context) {
	ctx.SetHeader("Deprecation", "true")
	ctx.SetHeader("Sunset", compositeAliasSunsetHTTPDate)
	ctx.SetHeader("Link", `</docs/migration/device-identity>; rel="deprecation"`)
	observe.IncDeviceCompositeUse(observe.CompositeUseSourceRESTURL)
}

// ResolveByAgentName 은 query parameter agent / name 으로 디바이스를 조회한다.
// GET /devices:resolve?agent=X&name=Y
//
// SPEC-DEVICE-IDENTITY-001 Phase B (M4 / B-T5 / B-AC4):
//
//	본 엔드포인트는 path-based agent/name dispatch (/devices/{agent}/{name}) 와
//	동일한 결과를 반환하되, 명시적 명명 (query parameter) 으로 호출자가 의도를
//	선언할 수 있게 한다. 운영 도구·스크립트가 path encoding 부담 없이 사용하기
//	편리하다.
//
//	- agent 또는 name 미제공: 400 Bad Request.
//	- 매칭 없음: 404 Not Found + 명시적 메시지.
//	- 정상 매칭: 200 OK + DeviceDetailResponse JSON.
//
// 경로 ("/devices:resolve") 의 콜론은 Go 1.22+ ServeMux 의 path 세그먼트
// 매칭 규칙상 단일 리터럴 세그먼트로 처리되어 "/devices/{ref}" 와 disjoint
// 하다 (슬래시가 없으므로 패턴 충돌 없음).
func (h *DeviceHandler) ResolveByAgentName(ctx api.Context) error {
	agentName := ctx.Query("agent")
	name := ctx.Query("name")
	if agentName == "" || name == "" {
		return api.ErrBadRequest.WithMessage(
			"both 'agent' and 'name' query parameters are required",
		)
	}

	d, err := h.registry.GetByAgentName(agentName, name)
	if err != nil {
		if errors.Is(err, device.ErrDeviceNotFound) {
			return api.ErrNotFound.WithMessage(fmt.Sprintf(
				"device with agent=%q name=%q not found", agentName, name,
			))
		}
		return mapDeviceError(err)
	}

	return h.respondWithDeviceDetail(ctx, d)
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

	h.logger.Info("device execute 요청", "id", id, "command", req.Command, "params", req.Params)

	result, err := h.registry.Execute(ctx.Context(), id, req.Command, req.Params)
	if err != nil {
		h.logger.Error("device execute 실패", "id", id, "command", req.Command, "error", err)
		return mapDeviceError(err)
	}

	if h.events != nil {
		// SPEC-DEVICE-IDENTITY-001 Phase B (M3 / B-T3): 가능하면 UUID 를 함께
		// 전달하여 V2 페이로드 (uid 1급) 를 emit 한다. id 를 ResolveDevice 로
		// 역조회하여 UID 를 추출하되, 조회 실패 시 v1 폴백 (composite 만 emit).
		var deviceUID string
		if d, _, resolveErr := h.registry.ResolveDevice(id); resolveErr == nil && d != nil {
			deviceUID = d.UID()
		}
		h.events.PublishDeviceStateChangedV2(deviceUID, id)
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
//
// SPEC-DEVICE-IDENTITY-001 Phase A (M2): UID 필드는 d.UID() 가 비어 있지
// 않으면 함께 채운다. 빈 문자열인 경우 omitempty 로 키 자체가 생략된다
// (graceful degradation — DeviceIDRepository 미설정 시 정상 동작).
func deviceToResponse(d device.Device) DeviceResponse {
	meta := d.Metadata()
	resp := DeviceResponse{
		ID:           d.ID(),
		UID:          d.UID(),
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
