// remote_grouping.go 는 v1.4(M9, 그룹 K)의 노드 그룹핑 REST + 노드 상세(시스템 정보 +
// uptime + 운영 요약) API 를 제공한다(@SPEC:SPEC-REMOTE-001 M9, REQ-K02~K06/K08/K10).
//
// 모든 엔드포인트는 admin 권한을 강제한다(REQ-K06/F04). 그룹은 서버 운영 메타데이터
// 이므로(A13) 배정/해제는 managed_nodes.group_name 갱신만 수행하고 노드로 명령을
// 전파하지 않는다(그룹 D 비경유).
//
// 라우트(api/v1 그룹 하위, remote_admin.go 의 GET 목록과 공존):
//
//	PUT    /remote/nodes/{instance_id}/group  — 그룹 배정/변경(REQ-K02) {group_name}
//	DELETE /remote/nodes/{instance_id}/group  — 그룹 해제("전체" 환원, REQ-K02/K05) → 204
//	GET    /remote/groups                     — distinct 그룹 + 카운트(항상 "전체" 포함, REQ-K03)
//	GET    /remote/nodes/{instance_id}        — 노드 상세(메타+시스템 정보+uptime+운영 요약, REQ-K08/K10)
//
// GET /remote/nodes/{instance_id} 는 단일 세그먼트 패턴이므로 GET /remote/nodes/pending
// (리터럴) 및 GET /remote/nodes/{instance_id}/flows 등(더 긴 path)과 충돌하지 않는다
// (Go 1.22 ServeMux 가 리터럴·더 구체적 패턴을 우선).
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
	"github.com/xtra/xflow/internal/updater"
)

// NodeGroupingService 는 노드 그룹핑 + 상세 오케스트레이션을 추상화한다(*remote.Server
// 가 만족). 핸들러 테스트에서 fake 로 대체 가능하도록 인터페이스로 분리한다.
type NodeGroupingService interface {
	// SetNodeGroup 은 노드의 그룹 라벨을 배정/변경한다(REQ-K02). 빈 문자열은 해제이다.
	// 미존재 시 storage.ErrManagedNodeNotFound.
	SetNodeGroup(ctx context.Context, instanceID, groupName string) error
	// ClearNodeGroup 은 노드의 그룹을 해제하여 "전체"로 환원한다(REQ-K02/K05).
	ClearNodeGroup(ctx context.Context, instanceID string) error
	// ListGroups 는 distinct 그룹 + 카운트를 반환한다(항상 "전체" 포함 — REQ-K03).
	ListGroups(ctx context.Context) ([]storage.NodeGroupCount, error)
	// NodeDetail 은 노드 메타 + 시스템 정보 + uptime + 운영 요약을 반환한다(REQ-K08/K10).
	// 미존재 시 storage.ErrManagedNodeNotFound.
	NodeDetail(ctx context.Context, instanceID string) (remote.NodeDetail, error)
	// SetNodeDisplayOverride 는 관리자 해상도 오버라이드를 설정한다(v1.6 M11 확장).
	// width/height 가 양수여야 하며(핸들러가 검증), 그룹 배정과 동일하게 노드로 명령을
	// 전파하지 않는다(A13). 미존재 시 storage.ErrManagedNodeNotFound.
	SetNodeDisplayOverride(ctx context.Context, instanceID string, width, height int) error
	// ClearNodeDisplayOverride 는 해상도 오버라이드를 해제한다(effective=노드 보고값 폴백).
	// 미존재 시 storage.ErrManagedNodeNotFound.
	ClearNodeDisplayOverride(ctx context.Context, instanceID string) error

	// --- 그룹 관리(일괄) ---

	// RenameGroup 은 oldName 그룹의 모든 노드를 newName 으로 일괄 이름변경한다(영향 노드 수).
	RenameGroup(ctx context.Context, oldName, newName string) (int, error)
	// DeleteGroup 은 groupName 그룹을 삭제하여 멤버를 "전체"로 이동한다(영향 노드 수).
	DeleteGroup(ctx context.Context, groupName string) (int, error)
	// DispatchGroup 은 그룹 내 승인 노드 전체에 명령을 디스패치하고 노드별 결과를 모은다.
	DispatchGroup(ctx context.Context, groupName, domain, action string, args json.RawMessage) ([]remote.GroupDispatchResult, error)
	// DispatchGroupUpdate 은 그룹 내 승인 노드에 아키텍처-aware 한 system/update 를 디스패치한다.
	// 각 노드의 보고된 OS/Arch 로 plan.VersionByArch 를 조회해 노드별 TargetVersion 을 해석한다.
	DispatchGroupUpdate(ctx context.Context, groupName string, plan remote.GroupUpdatePlan) ([]remote.GroupDispatchResult, error)
}

// setGroupRequest 는 그룹 배정 요청 본문이다(PUT .../group).
type setGroupRequest struct {
	GroupName string `json:"group_name"`
}

// setDisplayOverrideRequest 는 해상도 오버라이드 설정 요청 본문이다(PUT .../display).
// width/height 는 양수여야 한다(핸들러가 검증). 해제는 DELETE .../display 로 한다.
type setDisplayOverrideRequest struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// NodeGroupDTO 는 distinct 그룹 응답 표현이다(REQ-K03). GroupName 이 빈 문자열이면
// 가상 "전체" 버킷(그룹 미지정 노드)을 의미한다(프론트엔드가 "전체"로 표시).
type NodeGroupDTO struct {
	GroupName string `json:"group_name"`
	NodeCount int    `json:"node_count"`
}

// NodeSummaryDTO 는 노드 운영 요약 응답 표현이다(미러 파생 — REQ-K10).
type NodeSummaryDTO struct {
	Flows struct {
		Total   int `json:"total"`
		Running int `json:"running"`
		Stopped int `json:"stopped"`
	} `json:"flows"`
	Agents struct {
		Total     int `json:"total"`
		Connected int `json:"connected"`
	} `json:"agents"`
	Devices struct {
		Total  int `json:"total"`
		Online int `json:"online"`
	} `json:"devices"`
}

// NodeDetailDTO 는 노드 상세 응답 표현이다(메타+시스템 정보+uptime+운영 요약 — REQ-K08/K10).
//
// Uptime 은 started_at>0 일 때만 채워진다(서버 파생 — REQ-K08). started_at==0(미보고)
// 이면 Uptime=null, StartedAt=0 으로 표현된다(프론트엔드가 uptime 미표시 — 하위 호환).
// 토큰 식별자 등 시크릿은 노출하지 않는다(REQ-F06).
type NodeDetailDTO struct {
	InstanceID string `json:"instance_id"`
	Hostname   string `json:"hostname"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Online     bool   `json:"online"`
	GroupName  string `json:"group_name"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	StartedAt  int64  `json:"started_at"` // epoch ms (0=미보고)
	Uptime     *int64 `json:"uptime"`     // ms (started_at>0 일 때만, 아니면 null)
	// DisplayWidth/DisplayHeight 는 EFFECTIVE 노드 해상도이다(px, v1.6 M11/M11 확장).
	// 우선순위: 관리자 오버라이드(둘 다 양수) > 노드 보고값 > 0(둘 다 없음). 프론트엔드
	// 고정 캔버스 스케일러(FixedCanvasScaler)는 이 effective 값을 변경 없이 사용하며,
	// 0,0 이면 합리적 기본 해상도(1920×1080)/컨테이너 크기로 폴백한다(REQ-M03).
	DisplayWidth  int `json:"display_width"`  // EFFECTIVE 가로 px (오버라이드>보고값>0)
	DisplayHeight int `json:"display_height"` // EFFECTIVE 세로 px (오버라이드>보고값>0)
	// DisplayOverride* 는 관리자가 서버에서 설정한 해상도 오버라이드이다(v1.6 M11 확장).
	// 0 은 오버라이드 없음을 의미한다(이때 effective=보고값). UI 가 "서버 오버라이드" 소스
	// 표시 + 입력 폼 프리필에 사용한다.
	DisplayOverrideWidth  int `json:"display_override_width"`  // 관리자 오버라이드 가로 px (0=없음)
	DisplayOverrideHeight int `json:"display_override_height"` // 관리자 오버라이드 세로 px (0=없음)
	// DisplayReported* 는 노드가 보고한 원본 해상도이다(v1.6 M11). 0 은 미보고이다.
	// UI 가 "노드 보고" 소스 표시 + 오버라이드와의 비교에 사용한다.
	DisplayReportedWidth  int            `json:"display_reported_width"`  // 노드 보고 가로 px (0=미보고)
	DisplayReportedHeight int            `json:"display_reported_height"` // 노드 보고 세로 px (0=미보고)
	LastSeen              int64          `json:"last_seen"`               // epoch ms
	Summary               NodeSummaryDTO `json:"summary"`                 // 운영 요약(미러 파생)
}

// RemoteGroupingHandler 는 노드 그룹핑 + 상세 엔드포인트를 처리한다.
type RemoteGroupingHandler struct {
	svc      NodeGroupingService
	settings storage.SettingsRepository // 업데이트 소스(update_url/채널) 주입용(선택).
	releases *storage.ReleaseRepository // 아키텍처-aware 그룹 업데이트의 버전 해석용(선택).
}

// NewRemoteGroupingHandler 는 RemoteGroupingHandler 를 생성한다.
func NewRemoteGroupingHandler(svc NodeGroupingService) *RemoteGroupingHandler {
	return &RemoteGroupingHandler{svc: svc}
}

// WithSettings 는 전역 설정 저장소를 연결한다(그룹 일괄 업데이트에 서버 저장 update_url/
// 채널을 주입). nil 이면 노드 로컬 설정으로 폴백한다.
func (h *RemoteGroupingHandler) WithSettings(settings storage.SettingsRepository) *RemoteGroupingHandler {
	h.settings = settings
	return h
}

// WithReleases 는 릴리즈 저장소를 연결한다(strategy=latest/per_arch 의 노드별 버전 해석에
// 사용). nil 이면 strategy=latest/per_arch 요청이 400 으로 거부된다(strategy=pin 은 무관).
func (h *RemoteGroupingHandler) WithReleases(releases *storage.ReleaseRepository) *RemoteGroupingHandler {
	h.releases = releases
	return h
}

// RegisterRoutes 는 그룹핑 + 상세 라우트를 그룹에 등록한다(remote_admin 의 라우트와 공존).
func (h *RemoteGroupingHandler) RegisterRoutes(g *api.RouteGroup) {
	g.PUT("/remote/nodes/{instance_id}/group", h.SetGroup)
	g.DELETE("/remote/nodes/{instance_id}/group", h.ClearGroup)
	g.GET("/remote/groups", h.ListGroups)
	g.GET("/remote/nodes/{instance_id}", h.NodeDetail)
	// v1.6(M11 확장) 노드 해상도 서버-측 오버라이드(관리자 전용). group 라우트와 동일한
	// 2-세그먼트 패턴이라 GET .../{id}(단일 세그먼트)·.../flows 등과 충돌하지 않는다.
	g.PUT("/remote/nodes/{instance_id}/display", h.SetDisplayOverride)
	g.DELETE("/remote/nodes/{instance_id}/display", h.ClearDisplayOverride)

	// 그룹 관리(일괄): 이름변경/삭제 + 그룹 단위 업데이트/명령. {group_name} 단일 세그먼트는
	// 리터럴 GET /remote/groups 와 충돌하지 않는다.
	g.PUT("/remote/groups/{group_name}", h.RenameGroup)
	g.DELETE("/remote/groups/{group_name}", h.DeleteGroup)
	g.POST("/remote/groups/{group_name}/update", h.UpdateGroup)
	g.POST("/remote/groups/{group_name}/command", h.CommandGroup)
}

// renameGroupRequest 는 그룹 이름변경 요청 본문이다(PUT /remote/groups/{name}).
type renameGroupRequest struct {
	NewName string `json:"new_name"`
}

// groupUpdateRequest 는 그룹 일괄 업데이트 요청 본문이다(POST /remote/groups/{name}/update).
//
// 하위 호환: strategy 가 비면 기존 동작(pin — 단일 version 을 전 멤버에 동일 적용,
// version 이 비면 채널 최신)을 유지한다. strategy 로 아키텍처-aware 해석을 선택할 수 있다.
type groupUpdateRequest struct {
	Version       string            `json:"version"`            // 단일 버전 고정(기존). strategy 빈 값일 때 사용.
	Strategy      string            `json:"strategy,omitempty"` // "latest" | "pin" | "per_arch"; 빈 값 = pin(기존 동작)
	Channel       string            `json:"channel,omitempty"`
	Restart       bool              `json:"restart,omitempty"`
	VersionByArch map[string]string `json:"version_by_arch,omitempty"` // per_arch 모드: "os/arch"→version
}

// groupCommandRequest 는 그룹 일괄 명령 요청 본문이다(POST /remote/groups/{name}/command).
type groupCommandRequest struct {
	Domain string          `json:"domain"`
	Action string          `json:"action"`
	Args   json.RawMessage `json:"args,omitempty"`
}

// RenameGroup 은 그룹을 일괄 이름변경한다. PUT /remote/groups/{group_name} {new_name}
func (h *RemoteGroupingHandler) RenameGroup(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	old := strings.TrimSpace(ctx.Param("group_name"))
	if old == "" {
		return api.ErrBadRequest.WithMessage("그룹 이름이 필요합니다")
	}
	var req renameGroupRequest
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("new_name 본문이 올바르지 않습니다")
	}
	newName := strings.TrimSpace(req.NewName)
	if newName == "" {
		return api.ErrBadRequest.WithMessage("new_name 은 비어 있을 수 없습니다")
	}
	moved, err := h.svc.RenameGroup(ctx.Context(), old, newName)
	if err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"group_name": newName,
		"moved":      moved,
	}))
}

// DeleteGroup 은 그룹을 삭제하여 멤버를 "전체"로 이동한다. DELETE /remote/groups/{group_name}
func (h *RemoteGroupingHandler) DeleteGroup(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	name := strings.TrimSpace(ctx.Param("group_name"))
	if name == "" {
		return api.ErrBadRequest.WithMessage("그룹 이름이 필요합니다")
	}
	moved, err := h.svc.DeleteGroup(ctx.Context(), name)
	if err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{"moved": moved}))
}

// UpdateGroup 은 그룹 내 승인·온라인 노드를 일괄 원격 업데이트한다(아키텍처/OS-aware 그룹 확장).
// POST /remote/groups/{group_name}/update {version?, strategy?, channel?, restart?, version_by_arch?}
//
// strategy 로 노드별 타깃 버전 해석 정책을 선택한다(하위 호환 — 빈 값 = pin):
//   - ""|"pin": 단일 version 을 전 멤버에 동일 적용(version 이 비면 채널 최신). 기존 동작.
//   - "latest": 릴리즈 저장소에서 채널별 (os,arch) 슬롯 최신 버전을 해석해 노드별 적용.
//   - "per_arch": 요청의 version_by_arch("os/arch"→version) 맵을 그대로 적용(각 값 검증).
//
// latest/per_arch 는 미매핑 아키텍처를 건너뛴다(RequireMapping=true — 부분 성공).
func (h *RemoteGroupingHandler) UpdateGroup(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	name := strings.TrimSpace(ctx.Param("group_name"))
	if name == "" {
		return api.ErrBadRequest.WithMessage("그룹 이름이 필요합니다")
	}
	var req groupUpdateRequest
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("요청 본문 파싱 실패")
	}
	// 서버 저장 소스(update_url/채널)를 주입한다(요청이 명시하면 우선).
	srcURL, srcChannel := resolveUpdateSource(ctx.Context(), h.settings)
	channel := req.Channel
	if channel == "" {
		channel = srcChannel
	}

	plan, err := h.buildGroupUpdatePlan(ctx.Context(), req, channel, srcURL)
	if err != nil {
		return err // 이미 api.ErrBadRequest 등으로 매핑됨.
	}

	dctx := remote.ContextWithActor(ctx.Context(), ctx.UserID())
	results, err := h.svc.DispatchGroupUpdate(dctx, name, plan)
	if err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"group_name": name,
		"results":    results,
	}))
}

// buildGroupUpdatePlan 은 요청의 strategy 에 따라 remote.GroupUpdatePlan 을 구성한다.
// channel/updateURL 은 호출자가 이미 서버 소스 폴백을 해석해 전달한다(요청 우선).
// 검증 실패는 api.ErrBadRequest 류로 즉시 반환한다(디스패치 전 차단).
func (h *RemoteGroupingHandler) buildGroupUpdatePlan(
	ctx context.Context,
	req groupUpdateRequest,
	channel, updateURL string,
) (remote.GroupUpdatePlan, error) {
	base := remote.GroupUpdatePlan{
		Channel:   channel,
		UpdateURL: updateURL,
		Restart:   req.Restart,
	}

	switch req.Strategy {
	case "", "pin":
		// 기존 동작: 단일 version 을 전 멤버에 동일 적용(version 이 비면 채널 최신).
		if req.Version != "" && !updater.Version(req.Version).IsValid() {
			return remote.GroupUpdatePlan{}, api.ErrBadRequest.WithMessage("version 은 vMAJOR.MINOR.PATCH 형식이어야 합니다")
		}
		base.DefaultVersion = req.Version
		base.RequireMapping = false
		return base, nil

	case "latest":
		// 채널별 (os,arch) 슬롯 최신 버전을 릴리즈 저장소에서 해석한다.
		if h.releases == nil {
			return remote.GroupUpdatePlan{}, api.ErrBadRequest.WithMessage("릴리스 저장소가 구성되지 않았습니다")
		}
		// 해석 채널: 요청/소스 채널 우선, 둘 다 비면 stable.
		resolveChannel := channel
		if resolveChannel == "" {
			resolveChannel = "stable"
		}
		vmap, err := h.releases.LatestVersionByArch(ctx, resolveChannel)
		if err != nil {
			return remote.GroupUpdatePlan{}, mapRemoteAdminError(err)
		}
		base.VersionByArch = vmap
		base.RequireMapping = true
		return base, nil

	case "per_arch":
		// 요청의 명시 맵을 적용한다. 각 값은 유효 semver 이며 그 버전이 해당 os/arch
		// asset 을 실제로 보유해야 한다(없으면 잘못된 키로 400).
		if h.releases == nil {
			return remote.GroupUpdatePlan{}, api.ErrBadRequest.WithMessage("릴리스 저장소가 구성되지 않았습니다")
		}
		if len(req.VersionByArch) == 0 {
			return remote.GroupUpdatePlan{}, api.ErrBadRequest.WithMessage("per_arch 전략은 version_by_arch 가 비어 있을 수 없습니다")
		}
		for key, version := range req.VersionByArch {
			if !updater.Version(version).IsValid() {
				return remote.GroupUpdatePlan{}, api.ErrBadRequest.WithMessage("version_by_arch[" + key + "] 은 vMAJOR.MINOR.PATCH 형식이어야 합니다")
			}
			if !h.assetExistsForKey(ctx, version, key) {
				return remote.GroupUpdatePlan{}, api.ErrBadRequest.WithMessage("version_by_arch[" + key + "]: 버전 " + version + " 에 해당 아키텍처 asset 이 없습니다")
			}
		}
		base.VersionByArch = req.VersionByArch
		base.RequireMapping = true
		return base, nil

	default:
		return remote.GroupUpdatePlan{}, api.ErrBadRequest.WithMessage("알 수 없는 strategy 입니다(pin|latest|per_arch)")
	}
}

// assetExistsForKey 는 version 릴리즈가 "os/arch" 키에 해당하는 asset 을 보유하는지 확인한다.
// 릴리즈/asset 미존재는 false(잘못된 매핑)로 취급한다.
func (h *RemoteGroupingHandler) assetExistsForKey(ctx context.Context, version, key string) bool {
	osArch := strings.SplitN(key, "/", 2)
	if len(osArch) != 2 || osArch[0] == "" || osArch[1] == "" {
		return false
	}
	rec, err := h.releases.GetRelease(ctx, version)
	if err != nil {
		return false
	}
	for _, a := range rec.Assets {
		if a.OS == osArch[0] && a.Arch == osArch[1] {
			return true
		}
	}
	return false
}

// CommandGroup 은 그룹 내 승인·온라인 노드에 임의 명령을 일괄 디스패치한다.
// POST /remote/groups/{group_name}/command {domain, action, args?}
func (h *RemoteGroupingHandler) CommandGroup(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	name := strings.TrimSpace(ctx.Param("group_name"))
	if name == "" {
		return api.ErrBadRequest.WithMessage("그룹 이름이 필요합니다")
	}
	var req groupCommandRequest
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("요청 본문 파싱 실패")
	}
	if req.Domain == "" || req.Action == "" {
		return api.ErrBadRequest.WithMessage("domain 과 action 은 필수입니다")
	}
	dctx := remote.ContextWithActor(ctx.Context(), ctx.UserID())
	results, err := h.svc.DispatchGroup(dctx, name, req.Domain, req.Action, req.Args)
	if err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]any{
		"group_name": name,
		"results":    results,
	}))
}

// SetGroup 은 노드의 그룹을 배정/변경한다(REQ-K02). PUT /remote/nodes/{instance_id}/group
//
// 본문 {group_name}. 빈 문자열도 허용되며(해제와 동일 의미 — REQ-K05) 200 으로 응답한다.
func (h *RemoteGroupingHandler) SetGroup(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	var req setGroupRequest
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("group_name 본문이 올바르지 않습니다")
	}
	if err := h.svc.SetNodeGroup(ctx.Context(), id, req.GroupName); err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"instance_id": id,
		"group_name":  req.GroupName,
	}))
}

// ClearGroup 은 노드의 그룹을 해제하여 "전체"로 환원한다(REQ-K02/K05).
// DELETE /remote/nodes/{instance_id}/group → 204
func (h *RemoteGroupingHandler) ClearGroup(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	if err := h.svc.ClearNodeGroup(ctx.Context(), id); err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

// SetDisplayOverride 는 노드 해상도 서버-측 오버라이드를 설정한다(v1.6 M11 확장).
// PUT /remote/nodes/{instance_id}/display 본문 {width, height}.
//
// width/height 는 양수여야 한다(검증 실패 → 400). 해제는 DELETE .../display 로 한다
// (해제 의미가 PUT 의 비양수와 섞이지 않도록 분리). admin 게이팅(REQ-K06/F04 일관).
func (h *RemoteGroupingHandler) SetDisplayOverride(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	var req setDisplayOverrideRequest
	if err := ctx.Bind(&req); err != nil {
		return api.ErrBadRequest.WithMessage("width/height 본문이 올바르지 않습니다")
	}
	if req.Width <= 0 || req.Height <= 0 {
		return api.ErrBadRequest.WithMessage("width 와 height 는 양의 정수여야 합니다(해제는 DELETE 사용)")
	}
	if err := h.svc.SetNodeDisplayOverride(ctx.Context(), id, req.Width, req.Height); err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]int{
		"display_override_width":  req.Width,
		"display_override_height": req.Height,
	}))
}

// ClearDisplayOverride 는 노드 해상도 오버라이드를 해제한다(effective=노드 보고값 폴백).
// DELETE /remote/nodes/{instance_id}/display → 204. admin 게이팅(REQ-K06/F04 일관).
func (h *RemoteGroupingHandler) ClearDisplayOverride(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	if err := h.svc.ClearNodeDisplayOverride(ctx.Context(), id); err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.NoContent(http.StatusNoContent)
}

// ListGroups 는 distinct 그룹 + 카운트를 반환한다(항상 "전체" 포함 — REQ-K03).
// GET /remote/groups
func (h *RemoteGroupingHandler) ListGroups(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	groups, err := h.svc.ListGroups(ctx.Context())
	if err != nil {
		return mapRemoteAdminError(err)
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toNodeGroupDTOs(groups)))
}

// NodeDetail 은 노드 메타 + 시스템 정보 + uptime + 운영 요약을 반환한다(REQ-K08/K10).
// GET /remote/nodes/{instance_id}
func (h *RemoteGroupingHandler) NodeDetail(ctx api.Context) error {
	if err := requireAdmin(ctx); err != nil {
		return err
	}
	id := ctx.Param("instance_id")
	detail, err := h.svc.NodeDetail(ctx.Context(), id)
	if err != nil {
		return mapRemoteAdminError(err) // ErrManagedNodeNotFound → 404.
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toNodeDetailDTO(detail)))
}

// toNodeGroupDTOs 는 저장소 그룹 카운트를 응답 DTO 로 변환한다(REQ-K03).
func toNodeGroupDTOs(groups []storage.NodeGroupCount) []NodeGroupDTO {
	out := make([]NodeGroupDTO, 0, len(groups))
	for _, g := range groups {
		out = append(out, NodeGroupDTO{GroupName: g.GroupName, NodeCount: g.NodeCount})
	}
	return out
}

// toNodeDetailDTO 는 노드 상세를 응답 DTO 로 변환한다(시크릿 토큰 제외 — REQ-F06).
//
// uptime 은 HasUptime 일 때만 채우고(started_at>0), 아니면 null(미표시 — REQ-K08).
func toNodeDetailDTO(d remote.NodeDetail) NodeDetailDTO {
	dto := NodeDetailDTO{
		InstanceID: d.Node.InstanceID,
		Hostname:   d.Node.Hostname,
		Version:    d.Node.Version,
		Status:     d.Node.Status,
		Online:     d.Online,
		GroupName:  d.Node.GroupName,
		OS:         d.Node.OS,
		Arch:       d.Node.Arch,
		StartedAt:  d.Node.StartedAt,
		// EFFECTIVE 해상도(오버라이드>보고값>0) — remote.NodeDetail 이 파생(v1.6 M11 확장).
		DisplayWidth:  d.EffectiveWidth,
		DisplayHeight: d.EffectiveHeight,
		// 관리자 오버라이드 + 노드 보고 원본을 별도 노출(UI 소스 표시·폼 프리필).
		DisplayOverrideWidth:  d.Node.DisplayOverrideWidth,
		DisplayOverrideHeight: d.Node.DisplayOverrideHeight,
		DisplayReportedWidth:  d.Node.DisplayWidth,
		DisplayReportedHeight: d.Node.DisplayHeight,
		LastSeen:              d.Node.LastSeen,
	}
	if d.HasUptime {
		up := d.UptimeMs
		dto.Uptime = &up
	}
	dto.Summary.Flows.Total = d.Summary.Flows.Total
	dto.Summary.Flows.Running = d.Summary.Flows.Running
	dto.Summary.Flows.Stopped = d.Summary.Flows.Stopped
	dto.Summary.Agents.Total = d.Summary.Agents.Total
	dto.Summary.Agents.Connected = d.Summary.Agents.Connected
	dto.Summary.Devices.Total = d.Summary.Devices.Total
	dto.Summary.Devices.Online = d.Summary.Devices.Online
	return dto
}
