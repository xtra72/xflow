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
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/remote"
	"github.com/xtra/xflow/internal/storage"
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
	svc NodeGroupingService
}

// NewRemoteGroupingHandler 는 RemoteGroupingHandler 를 생성한다.
func NewRemoteGroupingHandler(svc NodeGroupingService) *RemoteGroupingHandler {
	return &RemoteGroupingHandler{svc: svc}
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
