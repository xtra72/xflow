package handler

import (
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/config"
)

// ScheduleLogConfigHandler 는 스케줄(예약) 실행 로그 저장소 백엔드 타입을 조회/편집하는
// 엔드포인트를 제공한다(설정 UI 용). 값은 config 오버라이드 레이어에 영속화되며
// (원본 config 파일 보존), 이 키는 시작 설정이므로 재시작 후 완전 적용된다(비-mutable).
//
// RemoteConfigHandler 의 영속화 패턴(admin-gate → SetPersistent → IsMutable 재시작 신호)을 미러링한다.
type ScheduleLogConfigHandler struct {
	cfg    config.Config
	logger *slog.Logger
}

// NewScheduleLogConfigHandler 는 새 핸들러를 생성한다.
func NewScheduleLogConfigHandler(cfg config.Config, logger *slog.Logger) *ScheduleLogConfigHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &ScheduleLogConfigHandler{cfg: cfg, logger: logger}
}

// keyScheduleLogType 는 스케줄 로그 저장소 백엔드를 선택하는 config 키이다.
const keyScheduleLogType = "storage.schedule_log.type"

// allowedScheduleLogTypes 는 storage 팩토리 NewScheduleLogRepository 가 수용하는 값 집합이다.
//   - "sqlite"/"database"/"db" → SQLite(영속, 기본)
//   - "memory"/"mem"           → 인메모리(비영속)
//   - "file"/"jsonl"           → JSON-Lines 파일(append-only)
var allowedScheduleLogTypes = map[string]bool{
	"sqlite": true, "database": true, "db": true,
	"memory": true, "mem": true,
	"file": true, "jsonl": true,
}

// scheduleLogConfigUpdate 는 PUT 요청 본문이다.
type scheduleLogConfigUpdate struct {
	StorageType string `json:"storage_type"`
}

// RegisterRoutes 는 스케줄 로그 설정 라우트를 등록한다(admin 전용).
//
//	GET /system/schedule-log-config
//	PUT /system/schedule-log-config
func (h *ScheduleLogConfigHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — /system/* 하위 설정이므로 system.* 로 매핑한다
	// (스케줄 로그 "데이터" 가 아니라 서버 저장소 백엔드 "설정" 이다).
	g.GETPerm("/system/schedule-log-config", "system.read", h.Get)
	g.PUTPerm("/system/schedule-log-config", "system.update", h.Put)
}

// Get 은 현재 스케줄 로그 저장소 타입을 반환한다(빈 값이면 "sqlite"). admin 전용.
func (h *ScheduleLogConfigHandler) Get(ctx api.Context) error {
	if ctx.UserRole() != "admin" {
		return api.ErrForbidden.WithMessage("admin 권한이 필요합니다")
	}
	storageType := h.cfg.Storage().ScheduleLogType
	if storageType == "" {
		storageType = "sqlite"
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"storage_type": storageType,
	}))
}

// Put 은 스케줄 로그 저장소 타입을 변경한다(오버라이드 영속화). admin 전용.
// 이 설정은 시작 설정이므로 재시작 후 완전 적용된다(needs_restart=true 기대).
func (h *ScheduleLogConfigHandler) Put(ctx api.Context) error {
	if ctx.UserRole() != "admin" {
		return api.ErrForbidden.WithMessage("admin 권한이 필요합니다")
	}
	var req scheduleLogConfigUpdate
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}

	// storage 팩토리가 수용하는 별칭 집합만 허용한다.
	if !allowedScheduleLogTypes[req.StorageType] {
		return api.ErrValidationFailed.WithMessage(
			"storage_type 은 sqlite | database | db | memory | mem | file | jsonl 중 하나여야 합니다")
	}

	if err := h.cfg.SetPersistent(keyScheduleLogType, req.StorageType); err != nil {
		h.logger.Error("schedule-log-config: 설정 영속화 실패", "key", keyScheduleLogType, "error", err)
		// admin 전용 엔드포인트이므로 원인 파악을 위해 실제 에러를 함께 노출한다.
		return api.ErrInternalServer.WithMessage("설정 저장 실패: " + keyScheduleLogType + ": " + err.Error())
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"applied":       keyScheduleLogType,
		"needs_restart": !config.IsMutable(keyScheduleLogType),
	}))
}
