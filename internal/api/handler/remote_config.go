package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/config"
)

// RemoteConfigHandler 는 이 인스턴스(피관리 노드)의 원격 관리 "클라이언트" 설정을
// 조회/편집하는 엔드포인트를 제공한다. 값은 config 오버라이드 레이어에 영속화되며
// (원본 config 파일 보존), mutable 키는 즉시, 비-mutable 키는 재시작 후 완전 적용된다.
//
// @SPEC:SPEC-REMOTE-001 (원격 관리 클라이언트 설정 UI)
type RemoteConfigHandler struct {
	cfg    config.Config
	logger *slog.Logger
}

// NewRemoteConfigHandler 는 새 핸들러를 생성한다.
func NewRemoteConfigHandler(cfg config.Config, logger *slog.Logger) *RemoteConfigHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RemoteConfigHandler{cfg: cfg, logger: logger}
}

// RegisterRoutes 는 원격 설정 라우트를 등록한다(admin 전용).
//
//	GET /system/remote-config
//	PUT /system/remote-config
func (h *RemoteConfigHandler) RegisterRoutes(g *api.RouteGroup) {
	// @SPEC:SPEC-AUTH-005 (M5) — /system/* 하위의 서버 설정이므로 system.* 이다
	// (원격 "노드" 관리가 아니라 이 서버의 원격 클라이언트 설정이다).
	g.GETPerm("/system/remote-config", "system.read", h.Get)
	g.PUTPerm("/system/remote-config", "system.update", h.Put)
}

// remoteConfigKeys 는 UI 필드 ↔ config 키 매핑이다. restartRequiredKeys 로 재시작 필요 여부를 표시한다.
const (
	keyMode            = "remote_management.mode"
	keyServerURL       = "remote_management.server_url"
	keyInstanceID      = "remote_management.instance_id"
	keyAutoRegister    = "remote_management.auto_register"
	keyHeartbeat       = "remote_management.heartbeat_interval"
	keyEnrollmentToken = "remote_management.enrollment_token"
	keyBootstrapSecret = "remote_management.bootstrap_secret"
	keyExposureFlows   = "remote_management.exposure.flows"
	keyExposureAgents  = "remote_management.exposure.agents"
	keyExposureDevices = "remote_management.exposure.devices"
	keyRequireSecure   = "remote_management.require_secure"
	keyInsecureSkip    = "remote_management.insecure_skip_verify"
	keyDisplayWidth    = "remote_management.display.width"
	keyDisplayHeight   = "remote_management.display.height"
)

// restartRequiredKeys 는 편집 시 재시작이 필요한(비-mutable) 키 목록이다(UI 배지용).
func restartRequiredKeys() []string {
	all := []string{
		keyMode, keyServerURL, keyInstanceID, keyAutoRegister, keyHeartbeat,
		keyEnrollmentToken, keyBootstrapSecret, keyExposureFlows, keyExposureAgents,
		keyExposureDevices, keyRequireSecure, keyInsecureSkip, keyDisplayWidth, keyDisplayHeight,
	}
	var out []string
	for _, k := range all {
		if !config.IsMutable(k) {
			out = append(out, k)
		}
	}
	return out
}

// Get 은 현재 원격 클라이언트 설정을 반환한다(시크릿은 redaction). admin 전용.
func (h *RemoteConfigHandler) Get(ctx api.Context) error {
	if ctx.UserRole() != "admin" {
		return api.ErrForbidden.WithMessage("admin 권한이 필요합니다")
	}
	rm := h.cfg.RemoteManagement()
	resp := dto.RemoteClientConfigResponse{
		Mode:               rm.Mode,
		ServerURL:          rm.ServerURL,
		InstanceID:         rm.InstanceID,
		AutoRegister:       rm.AutoRegister,
		HeartbeatInterval:  rm.HeartbeatInterval.String(),
		EnrollmentTokenSet: rm.EnrollmentToken != "",
		BootstrapSecretSet: rm.BootstrapSecret != "",
		Exposure: dto.RemoteClientExposureDTO{
			Flows:   rm.Exposure.Flows,
			Agents:  rm.Exposure.Agents,
			Devices: rm.Exposure.Devices,
		},
		RequireSecure:         rm.RequireSecure,
		InsecureSkipVerify:    rm.InsecureSkipVerify,
		DisplayWidth:          rm.Display.Width,
		DisplayHeight:         rm.Display.Height,
		RestartRequiredFields: restartRequiredKeys(),
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(resp))
}

// Put 은 원격 클라이언트 설정을 부분 업데이트한다(오버라이드 영속화). admin 전용.
func (h *RemoteConfigHandler) Put(ctx api.Context) error {
	if ctx.UserRole() != "admin" {
		return api.ErrForbidden.WithMessage("admin 권한이 필요합니다")
	}
	var req dto.RemoteClientConfigUpdate
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}

	// 값 검증 (mode / heartbeat).
	if req.Mode != nil {
		switch *req.Mode {
		case "disabled", "client", "server":
		default:
			return api.ErrValidationFailed.WithMessage("mode 는 disabled | client | server 중 하나여야 합니다")
		}
	}
	if req.HeartbeatInterval != nil && *req.HeartbeatInterval != "" {
		if _, err := time.ParseDuration(*req.HeartbeatInterval); err != nil {
			return api.ErrValidationFailed.WithMessage("heartbeat_interval 은 Go duration 형식이어야 합니다 (예: 30s)")
		}
	}

	// 필드 → 키 매핑을 순회하며 non-nil 값만 영속화한다.
	// 시크릿은 nil=유지이므로 이미 non-nil 여부로 처리된다(빈 문자열=해제).
	type kv struct {
		key string
		val any
		set bool
	}
	items := []kv{
		{keyMode, deref(req.Mode), req.Mode != nil},
		{keyServerURL, deref(req.ServerURL), req.ServerURL != nil},
		{keyInstanceID, deref(req.InstanceID), req.InstanceID != nil},
		{keyAutoRegister, derefBool(req.AutoRegister), req.AutoRegister != nil},
		{keyHeartbeat, deref(req.HeartbeatInterval), req.HeartbeatInterval != nil},
		{keyEnrollmentToken, deref(req.EnrollmentToken), req.EnrollmentToken != nil},
		{keyBootstrapSecret, deref(req.BootstrapSecret), req.BootstrapSecret != nil},
		{keyExposureFlows, deref(req.ExposureFlows), req.ExposureFlows != nil},
		{keyExposureAgents, deref(req.ExposureAgents), req.ExposureAgents != nil},
		{keyExposureDevices, deref(req.ExposureDevices), req.ExposureDevices != nil},
		{keyRequireSecure, derefBool(req.RequireSecure), req.RequireSecure != nil},
		{keyInsecureSkip, derefBool(req.InsecureSkipVerify), req.InsecureSkipVerify != nil},
		{keyDisplayWidth, derefInt(req.DisplayWidth), req.DisplayWidth != nil},
		{keyDisplayHeight, derefInt(req.DisplayHeight), req.DisplayHeight != nil},
	}

	result := dto.RemoteClientConfigUpdateResult{}
	for _, it := range items {
		if !it.set {
			continue
		}
		if err := h.cfg.SetPersistent(it.key, it.val); err != nil {
			h.logger.Error("remote-config: 설정 영속화 실패", "key", it.key, "error", err)
			// admin 전용 엔드포인트이므로 원인 파악을 위해 실제 에러를 함께 노출한다.
			return api.ErrInternalServer.WithMessage("설정 저장 실패: " + it.key + ": " + err.Error())
		}
		result.Applied = append(result.Applied, it.key)
		if !config.IsMutable(it.key) {
			result.NeedsRestart = append(result.NeedsRestart, it.key)
		}
	}

	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(result))
}

func deref(p *string) any {
	if p == nil {
		return ""
	}
	return *p
}
func derefBool(p *bool) any {
	if p == nil {
		return false
	}
	return *p
}
func derefInt(p *int) any {
	if p == nil {
		return 0
	}
	return *p
}
