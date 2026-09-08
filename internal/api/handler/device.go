package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/api/ws"
	"github.com/xtra/xflow/internal/device"
)

// DeviceRegistry 는 디바이스 레지스트리 인터페이스이다.
//
// SPEC-DEVICE-IDENTITY-001 Phase B (B-T1) 에서 GetByUID / GetByAgentName /
// ResolveDevice 가 도입되어 UUID / agent/name 기반 1급 lookup 경로를 제공한다.
// 핸들러는 ResolveDevice 를 우선 사용하며 composite key 는 Phase D (D-T12)
// 부터 dispatch 대상에서 제외된다 — UUID / agent/name 만 1급으로 수락한다.
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

// DeviceHistoryProvider 는 디바이스 수신 데이터 이력(주기 스냅샷) 조회 인터페이스이다.
//
// device.DeviceHistoryRecorder 가 이를 만족한다. nil 허용(이력 비활성 구성)이며,
// 이 경우 history 라우트는 등록되지 않는다.
type DeviceHistoryProvider interface {
	// History 는 deviceID(UUID)의 스냅샷을 최신순으로 최대 limit 개 반환한다.
	// 없으면 빈 슬라이스를 반환한다(에러 아님). limit clamp 는 구현이 담당한다.
	History(deviceID string, limit int) []device.HistorySnapshot
	// MaxEntries 는 링버퍼 상한이다(핸들러의 limit 기본/clamp 산출용).
	MaxEntries() int
}

// DeviceResponse 는 디바이스 목록 응답 DTO이다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (v1.0): Device.ID() 자체가 UUID 를 반환하므로
// 별도 UID 필드는 중복. ID 단일 필드만 노출 (외부 클라이언트는 ID 를 UUID 로
// 사용).
type DeviceResponse struct {
	ID            string                 `json:"id"`
	Name          string                 `json:"name"`
	Type          string                 `json:"type"`
	Protocol      string                 `json:"protocol"`
	AgentName     string                 `json:"agent_name"`
	Online        bool                   `json:"online"`
	ReportEnabled bool                   `json:"report_enabled"`
	LastSeen      time.Time              `json:"last_seen"`
	Source        string                 `json:"source"`
	Capabilities  []string               `json:"capabilities"`
	Metadata      *device.DeviceMetadata `json:"metadata,omitempty"`
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
	history      DeviceHistoryProvider // nil 허용 (이력 비활성 시 history 라우트 미등록)
	events       *ws.EventPublisher    // nil 허용
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

// WithDeviceHistory 는 DeviceHandler 에 이력 제공자를 설정한다.
// nil 을 전달하면 history 라우트가 등록되지 않는다(이력 비활성 구성 호환).
func WithDeviceHistory(p DeviceHistoryProvider) DeviceHandlerOption {
	return func(h *DeviceHandler) {
		h.history = p
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
	// @SPEC:SPEC-AUTH-005 (M5) — device.* 권한 부착.
	// metadata 는 디바이스의 하위 속성이므로 PUT/DELETE 모두 device.update 로 매핑한다
	// (device.delete 는 디바이스 자체의 삭제를 뜻한다 — 본 핸들러에는 해당 라우트가 없다).
	g.GETPerm("/devices", "device.read", h.List)
	g.GETPerm("/devices:resolve", "device.read", h.ResolveByAgentName) // B-T5
	g.GETPerm("/devices/{ref}", "device.read", h.Get)
	g.GETPerm("/devices/{agent}/{name}", "device.read", h.GetByAgentName) // B-T4 — 2 세그먼트 dispatch
	g.POSTPerm("/devices/{id}/execute", "device.execute", h.Execute)
	g.PUTPerm("/devices/{id}/metadata", "device.update", h.UpdateMetadata)
	g.DELETEPerm("/devices/{id}/metadata", "device.update", h.DeleteMetadata)

	// 디바이스 수신 데이터 이력(주기 스냅샷). 이력 제공자가 주입된 경우에만 등록한다.
	// "/devices/{id}/history" 는 execute/metadata 와 동일하게 2 세그먼트 + 액션
	// 리터럴이라 "/devices/{ref}" 보다 우선한다(Go 1.22 ServeMux precedence).
	if h.history != nil {
		g.GETPerm("/devices/{id}/history", "device.read", h.History)
	}
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
			if hasMetadata(meta) {
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
// SPEC-DEVICE-IDENTITY-001 Phase D (M9 / D-T2 / D-T12):
//
//	{ref} 는 UUID v4 형식만 수락한다 (agent/name 형식은 별도 라우트
//	GET /devices/{agent}/{name} 가 처리).
//
//	  - UUID v4 (예: "a58ba668-5741-4b3c-9d2e-7f3c8a1b2c3d")
//	    → registry.ResolveDevice → GetByUID, 1급 식별자.
//	  - composite ("agent:local_id" — 예: "lg_icp01:81")
//	    → ClassifyDeviceRef 가 DeviceRefUnknown 으로 분류 → HTTP 404
//	      (D-T2 / D-AC2 — 마이그레이션 안내 메시지 포함).
//	  - 그 외 (UUID/agent-name 어느 것도 아님)
//	    → HTTP 404 + 명시적 에러 메시지.
func (h *DeviceHandler) Get(ctx api.Context) error {
	ref := ctx.Param("ref")
	if ref == "" {
		return api.ErrBadRequest.WithMessage("device reference is required")
	}

	// composite 패턴 ("agent:local_id") 명시 검출 — actionable 마이그레이션 안내.
	// D-AC2: "composite reference is removed; use UUID or agent/name".
	if looksLikeComposite(ref) {
		return api.ErrNotFound.WithMessage(fmt.Sprintf(
			"device reference %q not found: composite reference is removed in xflowd v1.0; use UUID or agent/name",
			ref,
		))
	}

	d, kind, err := h.registry.ResolveDevice(ref)
	if err != nil {
		if kind == device.DeviceRefUnknown {
			return api.ErrNotFound.WithMessage(fmt.Sprintf(
				"device reference %q not found; expected UUID or agent/name",
				ref,
			))
		}
		return mapDeviceError(err)
	}

	return h.respondWithDeviceDetail(ctx, d)
}

// looksLikeComposite 는 "agent:local_id" 형식의 legacy composite 참조를 감지한다.
// SPEC-DEVICE-IDENTITY-001 Phase D (D-T2): UUID 도 슬래시도 아니면서 콜론을
// 포함하는 ref 는 v0.x composite 로 간주하고 명시적 404 + 마이그레이션 안내.
func looksLikeComposite(ref string) bool {
	if ref == "" {
		return false
	}
	if device.ClassifyDeviceRef(ref) != device.DeviceRefUnknown {
		return false
	}
	return strings.Contains(ref, ":")
}

// HistoryResponse 는 디바이스 이력 조회 응답 DTO 이다.
type HistoryResponse struct {
	DeviceID string                   `json:"device_id"`
	Count    int                      `json:"count"`
	Entries  []device.HistorySnapshot `json:"entries"`
}

// History 는 디바이스 수신 데이터 이력(주기 스냅샷)을 최신순으로 반환한다.
// GET /devices/{id}/history?limit=N
//
// 식별자 해석은 기존 Get 과 동일하게 ResolveDevice(UUID/agent-name)를 재사용한다.
// 디바이스가 없거나 이력이 없으면 빈 배열(200)을 반환한다(이력은 best-effort 관측
// 데이터이므로 미존재를 404 로 다루지 않는다). limit 기본값은 MaxEntries 이며 상한도
// MaxEntries 로 clamp 된다(레코더가 최종 clamp 수행).
func (h *DeviceHandler) History(ctx api.Context) error {
	if h.history == nil {
		return api.ErrNotFound.WithMessage("device history is not enabled")
	}

	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("device id is required")
	}

	// limit 파싱(미지정/무효 → 0 → 레코더가 MaxEntries 로 clamp).
	limit := 0
	if limitStr := ctx.Query("limit"); limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// 식별자를 UUID 로 해석하여 레코더의 버퍼 키(UUID)와 일치시킨다.
	// composite 는 더 이상 1급이 아니므로 Get 과 동일하게 명시 안내한다.
	if looksLikeComposite(id) {
		return api.ErrNotFound.WithMessage(fmt.Sprintf(
			"device reference %q not found: composite reference is removed in xflowd v1.0; use UUID or agent/name",
			id,
		))
	}

	deviceID := id
	if d, kind, err := h.registry.ResolveDevice(id); err == nil && d != nil {
		// 해석 성공 시 UUID 를 버퍼 키로 사용한다.
		deviceID = d.ID()
	} else if kind == device.DeviceRefUnknown {
		// 형식 자체가 UUID/agent-name 어느 것도 아니면 400.
		return api.ErrBadRequest.WithMessage(fmt.Sprintf(
			"device reference %q is invalid; expected UUID or agent/name", id,
		))
	}
	// 디바이스가 현재 레지스트리에 없어도(오프라인/제거) UUID 형태면 그대로 조회 —
	// 버퍼가 GC 되기 전이면 이력이 남아 있을 수 있다(graceful).

	entries := h.history.History(deviceID, limit)
	if entries == nil {
		entries = []device.HistorySnapshot{}
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(HistoryResponse{
		DeviceID: deviceID,
		Count:    len(entries),
		Entries:  entries,
	}))
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
	if hasMetadata(meta) {
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
// hasMetadata 는 응답에 실어 보낼 만한 메타데이터가 하나라도 있는지 본다.
//
// 세 곳(목록·상세·해석)이 같은 판정을 하므로 한곳에 둔다. 종전에는 같은 긴 조건식이
// 세 벌 있어, 필드를 늘릴 때 한 곳만 고치면 그 화면에서만 메타데이터가 사라졌다.
func hasMetadata(meta device.DeviceMetadata) bool {
	return meta.Name != "" ||
		meta.Location != "" ||
		len(meta.Tags) > 0 ||
		meta.Group != "" ||
		len(meta.Labels) > 0 ||
		meta.Pinned != nil ||
		meta.StaleAfterSec != nil
}

// deviceLocalID 는 에이전트 내부 식별자를 얻는다.
//
// LocalID() 는 선택적 확장 인터페이스다. 없으면 Name() 으로 떨어진다 — 대부분의
// 어댑터가 라벨과 내부 식별자를 같게 쓰므로 복원에 충분하고, 부팅 경로도 같은
// 폴백을 쓴다(cmd/xflowd/main.go).
func deviceLocalID(dev device.Device) string {
	type localIDProvider interface{ LocalID() string }
	if lp, ok := dev.(localIDProvider); ok {
		if id := lp.LocalID(); id != "" {
			return id
		}
	}
	return dev.Name()
}

func (h *DeviceHandler) UpdateMetadata(ctx api.Context) error {
	id := ctx.Param("id")
	if id == "" {
		return api.ErrBadRequest.WithMessage("device id is required")
	}

	var meta device.DeviceMetadata
	if err := ctx.Bind(&meta); err != nil {
		return err
	}

	// 소유 정보(에이전트 이름 · 로컬 식별자)를 함께 적는다.
	//
	// 고정 설치 디바이스를 재시작 후 **다시 발견되기 전에** 복원하려면 어느
	// 에이전트의 어떤 로컬 ID 인지 알아야 한다. 그 정보는 디바이스가 살아 있는
	// 지금만 알 수 있으므로 여기서 채운다(DeviceMetadata 주석 참조).
	//
	// 요청 본문의 값은 신뢰하지 않는다 — 레지스트리가 정본이다.
	meta.AgentName = ""
	meta.LocalID = ""
	if dev, err := h.registry.Get(id); err == nil && dev != nil {
		meta.AgentName = dev.AgentName()
		meta.LocalID = deviceLocalID(dev)
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

// reportEnabledCarrier 는 디바이스별 상태 전송 on/off 를 노출하는 optional 인터페이스이다.
// samsung/lgap 어댑터(SamsungNasaDeviceAdapter) 만 구현하며, 미구현 어댑터(modbus/
// lg_icp/century 등)는 기본 true 로 안전하게 처리된다. device.Device 인터페이스를 건드리지
// 않아 모든 구현체 파급을 피한다.
type reportEnabledCarrier interface {
	ReportEnabled() bool
}

// deviceToResponse 는 device.Device를 DeviceResponse로 변환한다.
//
// SPEC-DEVICE-IDENTITY-001 Phase D (v1.0): ID 가 곧 UUID 이므로 별도 UID 필드는
// 제거됨.
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

	// report_enabled: optional-interface 로 채운다. 미구현 어댑터는 기본 true(안전).
	resp.ReportEnabled = true
	if rc, ok := d.(reportEnabledCarrier); ok {
		resp.ReportEnabled = rc.ReportEnabled()
	}

	if hasMetadata(meta) {
		resp.Metadata = &meta
	}

	return resp
}
