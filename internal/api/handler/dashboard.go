// @SPEC:SPEC-DASHBOARD-004 (M4, spec.md §2.2, §2.3, §2.13)
// dashboard.go — 대시보드 1급 엔티티 REST 핸들러.
//
// 책임:
//   - 대시보드 CRUD (/dashboards, /dashboards/{uid})
//   - 사용자 UI 상태 (/dashboard-state — /dashboards/{uid} 와 충돌하지 않는 최상위 경로)
//   - 읽기 전용 호환 shim (GET /dashboards/{shared,mine} — 원격 프록시 무변경 유지)
//   - 인가는 **전부** internal/dashboardacl.Evaluate 로 위임한다. 경로마다 조건을
//     재작성하면 한 경로만 누락되어도 타인 대시보드가 노출되므로, 판정은 한 곳에만 둔다.
//   - 요청 본문의 owner · visibility · version · uid 는 어떤 경로에서도 신뢰하지 않는다
//     (spec.md §2.13 UB1 #4).
//
// 구 모델(SPEC-DASHBOARD-001)의 requireAdmin(= UserRole()=="admin")은 제거되었다.
// 커스텀 역할이 생기는 순간 역할 이름 열거가 성립하지 않기 때문이다(spec.md §4.2).

package handler

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/dashboardacl"
	"github.com/xtra/xflow/internal/storage"
)

// maxDashboardPayloadBytes 는 PUT 페이로드의 최대 크기이다 (spec.md §2.13 UB1 #9).
//
// 초과 시 413 Payload Too Large 로 거부하며 저장하지 않는다. 단위가 묶음에서
// 1장으로 줄었으므로 실효 상한은 완화되었다(spec.md §2.3).
const maxDashboardPayloadBytes = 256 * 1024

// maxDashboardNameRunes 는 대시보드 이름의 최대 길이이다 (spec.md §2.7).
const maxDashboardNameRunes = 64

// emptyDashboardPayload 는 payload 미지정 시 사용되는 기본 본문이다.
// 클라이언트는 panels / layout 을 항상 배열로 기대한다.
const emptyDashboardPayload = `{"panels":[],"layout":[]}`

// dashboardReservedUIDs 는 라우트 리터럴과 충돌하므로 uid 로 발급되지 않는 예약어다
// (spec.md §2.1). 서버가 uid 를 발급하므로 실제로 충돌할 수 없지만, 발급기가 바뀌어도
// 예약어가 새어나가지 않도록 발급 경로에서 명시적으로 걸러낸다.
var dashboardReservedUIDs = map[string]struct{}{
	"shared": {}, "mine": {}, "state": {},
}

// DashboardPermissionService 는 요청 시점의 유효 역할과 그 역할의 권한 집합을 조회한다.
//
// *auth.PermissionCache 가 본 인터페이스를 만족한다. 권한을 토큰이 아니라 요청 시점에
// 조회해야 역할 변경이 기존 토큰에도 즉시 반영된다(SPEC-AUTH-005 §4.3, AC-07).
type DashboardPermissionService interface {
	// EffectiveRole 은 인가 판정에 사용할 "현재" 역할을 반환한다.
	EffectiveRole(ctx context.Context, username, fallback string) (string, error)
	// Permissions 는 역할의 권한 키 목록을 반환한다. 없는 역할은 빈 슬라이스이다.
	Permissions(ctx context.Context, role string) ([]string, error)
}

// DashboardHandler 는 대시보드 1급 엔티티 REST 엔드포인트를 처리한다.
//
// jwtSvc 는 구 모델에서 승계한 필드로 현재 사용되지 않는다. 인증은 api.Auth
// 미들웨어가, 전역 권한은 api.RequirePermission 이 담당하고, 본 핸들러는
// (요청자, 대시보드) 인가만 판정한다.
type DashboardHandler struct {
	repo   storage.DashboardRepository
	acl    storage.DashboardACLRepository
	state  storage.DashboardUserStateRepository
	perms  DashboardPermissionService
	db     *sql.DB // ACL subject 실재 검증(사용자·역할 조회)용
	jwtSvc *auth.JWTService
	logger *slog.Logger

	// authEnabled 는 basic_auth.enabled 이다. false 이면 spec.md §2.10 S1 에 따라
	// 모든 인가 판정이 허용으로 처리되고 owner 는 빈 문자열이 된다.
	authEnabled bool
}

// NewDashboardHandler 는 새 DashboardHandler 를 생성한다.
//
// repo / acl / state 는 필수이며, perms 와 db 는 인증 비활성 배포나 테스트에서
// nil 일 수 있다(그 경우 권한 집합은 비어 있고 subject 실재 검증은 생략된다).
func NewDashboardHandler(
	repo storage.DashboardRepository,
	aclRepo storage.DashboardACLRepository,
	stateRepo storage.DashboardUserStateRepository,
	jwtSvc *auth.JWTService,
	logger *slog.Logger,
) *DashboardHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &DashboardHandler{
		repo:   repo,
		acl:    aclRepo,
		state:  stateRepo,
		jwtSvc: jwtSvc,
		logger: logger,
	}
}

// WithPermissions 는 요청 시점 권한 조회기를 주입한다.
func (h *DashboardHandler) WithPermissions(p DashboardPermissionService) *DashboardHandler {
	h.perms = p
	return h
}

// WithAuthEnabled 는 basic_auth 활성화 여부를 주입한다 (spec.md §2.10 S1).
func (h *DashboardHandler) WithAuthEnabled(enabled bool) *DashboardHandler {
	h.authEnabled = enabled
	return h
}

// WithSubjectDB 는 ACL subject 실재 검증에 쓰일 DB 핸들을 주입한다.
func (h *DashboardHandler) WithSubjectDB(db *sql.DB) *DashboardHandler {
	h.db = db
	return h
}

// RegisterRoutes 는 대시보드 라우트를 등록한다 (spec.md §2.3 라우트 표).
//
//	GET    /dashboards            dashboard.read   — view 가능 항목만
//	POST   /dashboards            dashboard.create
//	GET    /dashboards/shared     dashboard.read   — 읽기 전용 호환 shim
//	GET    /dashboards/mine       (권한 미부착)     — 본인 소유 private 만, shim
//	GET    /dashboards/{uid}/acl  dashboard.read   — grant 인가
//	PUT    /dashboards/{uid}/acl  dashboard.update — grant 인가
//	GET    /dashboards/{uid}      dashboard.read   — view 인가
//	PUT    /dashboards/{uid}      dashboard.update — edit 인가
//	PATCH  /dashboards/{uid}      dashboard.update — edit(이름) / grant(공개범위)
//	DELETE /dashboards/{uid}      dashboard.delete — delete 인가
//	GET    /dashboard-state       (권한 미부착)     — 본인 UI 상태
//	PUT    /dashboard-state       (권한 미부착)     — 본인 UI 상태
//
// 등록 순서는 user.go 의 `/users/{username}/password` 선례를 따라 리터럴 세그먼트를
// 파라미터 세그먼트보다 먼저 등록한다. net/http.ServeMux(Go 1.22+)는 패턴 특이성으로
// 우선순위를 결정하므로 등록 순서 자체가 의미를 갖지는 않지만, 읽는 사람이 우선순위를
// 코드 순서에서 바로 읽을 수 있어야 한다.
//
// 구 모델의 PUT/DELETE /dashboards/{shared,mine} 은 **등록하지 않는다**. 묶음 단위
// 쓰기는 새 모델에서 "어느 대시보드의 어느 version 에 대한 쓰기인가" 를 결정할 수
// 없어 낙관적 동시성이 성립하지 않는다(spec.md §2.3). 미등록이므로 {uid} 라우트가
// uid="shared" 로 받아 404 를 반환한다 — 405 가 아니다.
func (h *DashboardHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GETPerm("/dashboards", "dashboard.read", h.List)
	g.POSTPerm("/dashboards", "dashboard.create", h.Create)

	// 리터럴 세그먼트 우선 — 읽기 전용 호환 shim (spec.md §4.4).
	g.GETPerm("/dashboards/shared", "dashboard.read", h.GetSharedShim)
	// /dashboards/mine 은 본인 소유 리소스만 반환하므로 권한을 부착하지 않는다
	// (구 모델의 allowlist 사유를 그대로 승계한다 — spec.md §2.6 회귀 완화).
	g.GET("/dashboards/mine", h.GetMineShim)

	// 리터럴 /acl 세그먼트 — /dashboards/{uid} 보다 특이하다.
	g.GETPerm("/dashboards/{uid}/acl", "dashboard.read", h.GetACL)
	g.PUTPerm("/dashboards/{uid}/acl", "dashboard.update", h.PutACL)

	g.GETPerm("/dashboards/{uid}", "dashboard.read", h.Get)
	g.PUTPerm("/dashboards/{uid}", "dashboard.update", h.Put)
	g.PATCHPerm("/dashboards/{uid}", "dashboard.update", h.Patch)
	g.DELETEPerm("/dashboards/{uid}", "dashboard.delete", h.Delete)

	// 사용자 UI 상태는 /dashboards/{uid} 경로 매칭과 충돌하지 않도록 별도 최상위
	// 경로에 둔다(spec.md §2.3). 본인 것만 읽고 쓰므로 권한을 부착하지 않는다.
	g.GET("/dashboard-state", h.GetState)
	g.PUT("/dashboard-state", h.PutState)
}

// -----------------------------------------------------------------------------
// 대시보드 CRUD
// -----------------------------------------------------------------------------

// List 는 요청자가 view 가능한 대시보드 목록을 반환한다 (payload 제외).
// GET /api/v1/dashboards
func (h *DashboardHandler) List(ctx api.Context) error {
	subj, err := h.subject(ctx)
	if err != nil {
		return err
	}

	items, err := h.repo.List(ctx.Context(), false)
	if err != nil {
		h.logger.Error("대시보드 목록 조회 실패", "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 목록 조회 실패")
	}

	// 대시보드 행당 추가 질의 없이 판정하기 위해 요청자에게 매치될 수 있는 ACL 행만
	// 단일 질의로 가져온다(spec.md §5 — 질의 2회 이내).
	aclByDashboard, err := h.aclForSubject(ctx.Context(), subj)
	if err != nil {
		return err
	}

	out := make([]dto.DashboardMeta, 0, len(items))
	for i := range items {
		access := dashboardacl.Evaluate(subj, toACLDashboard(items[i]), aclByDashboard[items[i].ID])
		if !access.View {
			continue
		}
		out = append(out, toDashboardMeta(items[i], access))
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(out))
}

// Create 는 대시보드를 생성한다 (spec.md §2.7 E1).
// POST /api/v1/dashboards
func (h *DashboardHandler) Create(ctx api.Context) error {
	subj, err := h.subject(ctx)
	if err != nil {
		return err
	}

	body, err := h.readBody(ctx)
	if err != nil {
		return err
	}
	var req dto.DashboardCreateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}

	name, err := validateDashboardName(req.Name)
	if err != nil {
		return err
	}
	payload, err := normalizeDashboardPayload(req.Payload)
	if err != nil {
		return err
	}

	created, err := h.repo.Create(ctx.Context(), storage.Dashboard{
		UID:  newDashboardUID(),
		Name: name,
		// owner 는 JWT username 으로만 결정한다(spec.md §2.13 UB1 #4).
		// 인증 비활성 배포에서는 빈 문자열이다(spec.md §2.10 S1).
		Owner:      subj.Username,
		Visibility: dashboardacl.VisibilityPrivate,
		Payload:    payload,
		// SortOrder 는 저장소가 소유자별 시퀀스로 부여한다(같은 owner 의 최댓값 + 1).
		// 여기서 읽고-쓰면 동시 생성 시 두 요청이 같은 값을 받으므로 저장소 트랜잭션
		// 안에서 결정한다.
	})
	if err != nil {
		if errors.Is(err, storage.ErrDashboardUIDExists) {
			// 서버가 uuid 로 발급하므로 사실상 도달하지 않는다.
			return api.ErrConflict.WithMessage("dashboard uid already exists")
		}
		h.logger.Error("대시보드 생성 실패", "name", name, "owner", subj.Username, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 생성 실패")
	}

	h.logger.Info("대시보드 생성", "uid", created.UID, "owner", created.Owner, "actor", ctx.UserID())
	access := dashboardacl.Evaluate(subj, toACLDashboard(*created), nil)
	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(toDashboardDetail(*created, access)))
}

// Get 은 단건 대시보드를 payload 와 함께 반환한다.
// GET /api/v1/dashboards/{uid}
func (h *DashboardHandler) Get(ctx api.Context) error {
	_, d, access, err := h.load(ctx)
	if err != nil {
		return err
	}
	if !access.View {
		// 존재 노출을 감수하고 403 으로 통일한다 — 404 를 쓰면 클라이언트 폴백 로직이
		// "권한 없음" 과 "삭제됨" 을 구분하지 못한다(spec.md §2.13 UB1 #1/#2 각주).
		return forbiddenDashboard()
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toDashboardDetail(*d, access)))
}

// Put 은 대시보드 본문(payload)을 저장한다.
// PUT /api/v1/dashboards/{uid}
func (h *DashboardHandler) Put(ctx api.Context) error {
	// 413 판정은 본문을 읽기 전에도 Content-Length 로 선행되며, 인가보다 먼저
	// 수행하면 권한 없는 사용자가 413/403 차이로 존재를 탐지할 수 있다. 따라서
	// 인가 → 본문 순서를 지킨다.
	subj, d, access, err := h.load(ctx)
	if err != nil {
		return err
	}
	if !access.View {
		return forbiddenDashboard()
	}
	if !access.Edit {
		// view 만 부여된 사용자의 저장 — 서버 상태 불변(spec.md §2.13 UB1 #3).
		return forbiddenDashboard()
	}

	body, err := h.readBody(ctx)
	if err != nil {
		return err
	}
	var req dto.DashboardSaveRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}
	payload, err := normalizeDashboardPayload(req.Payload)
	if err != nil {
		return err
	}

	updated, err := h.repo.Update(ctx.Context(), d.UID,
		storage.DashboardUpdate{Payload: payload}, parseIfMatch(ctx.GetHeader("If-Match")))
	if err != nil {
		return h.updateError(ctx, err, d.UID, subj)
	}

	h.logger.Info("대시보드 저장", "uid", updated.UID, "version", updated.Version, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toDashboardDetail(*updated, access)))
}

// Patch 는 대시보드 메타(이름·공개범위·기본 여부·정렬)를 변경한다.
// PATCH /api/v1/dashboards/{uid}
//
// name 은 edit 인가, visibility · is_default 는 grant 인가를 요구한다
// (spec.md §2.3 라우트 표). sort_order 는 표시 순서일 뿐이므로 edit 로 취급한다.
func (h *DashboardHandler) Patch(ctx api.Context) error {
	subj, d, access, err := h.load(ctx)
	if err != nil {
		return err
	}
	if !access.View {
		return forbiddenDashboard()
	}

	body, err := h.readBody(ctx)
	if err != nil {
		return err
	}
	var req dto.DashboardPatchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}
	if req.Name == nil && req.Visibility == nil && req.IsDefault == nil && req.SortOrder == nil {
		return api.ErrBadRequest.WithMessage("변경할 필드가 없습니다")
	}

	var upd storage.DashboardUpdate

	if req.Name != nil || req.SortOrder != nil {
		if !access.Edit {
			return forbiddenDashboard()
		}
		if req.Name != nil {
			name, err := validateDashboardName(*req.Name)
			if err != nil {
				return err
			}
			upd.Name = &name
		}
		upd.SortOrder = req.SortOrder
	}

	if req.Visibility != nil || req.IsDefault != nil {
		if !access.Grant {
			return forbiddenDashboard()
		}
		if req.Visibility != nil {
			if !dashboardacl.IsValidVisibility(*req.Visibility) {
				return api.ErrBadRequest.WithMessage("visibility 는 private / shared / acl 중 하나여야 합니다")
			}
			upd.Visibility = req.Visibility
		}
		upd.IsDefault = req.IsDefault
	}

	updated, err := h.repo.Update(ctx.Context(), d.UID, upd, parseIfMatch(ctx.GetHeader("If-Match")))
	if err != nil {
		return h.updateError(ctx, err, d.UID, subj)
	}

	// 공개범위·소유권이 바뀌면 요청자 자신의 인가도 달라질 수 있으므로 재판정한다.
	acl, err := h.aclFor(ctx.Context(), updated.ID)
	if err != nil {
		return err
	}
	newAccess := dashboardacl.Evaluate(subj, toACLDashboard(*updated), acl)

	h.logger.Info("대시보드 메타 변경", "uid", updated.UID, "version", updated.Version, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toDashboardDetail(*updated, newAccess)))
}

// Delete 는 대시보드와 그에 딸린 ACL 을 삭제한다 → 204.
// DELETE /api/v1/dashboards/{uid}
func (h *DashboardHandler) Delete(ctx api.Context) error {
	_, d, access, err := h.load(ctx)
	if err != nil {
		return err
	}
	if !access.View {
		return forbiddenDashboard()
	}
	if !access.Delete {
		return forbiddenDashboard()
	}

	if err := h.repo.Delete(ctx.Context(), d.UID); err != nil {
		h.logger.Error("대시보드 삭제 실패", "uid", d.UID, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 삭제 실패")
	}
	h.logger.Info("대시보드 삭제", "uid", d.UID, "actor", ctx.UserID())
	return ctx.NoContent(http.StatusNoContent)
}

// -----------------------------------------------------------------------------
// 사용자 UI 상태 (/dashboard-state)
// -----------------------------------------------------------------------------

// GetState 는 요청자의 대시보드 UI 상태를 반환한다.
// GET /api/v1/dashboard-state
//
// 행이 없으면 404 가 아니라 기본값(version 0)을 반환한다 — 최초 로그인 사용자가
// 404 폴백 분기를 타야 할 이유가 없다.
func (h *DashboardHandler) GetState(ctx api.Context) error {
	username, err := h.stateUsername(ctx)
	if err != nil {
		return err
	}

	st, err := h.state.Get(ctx.Context(), username)
	if errors.Is(err, storage.ErrDashboardUserStateNotFound) {
		return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.DashboardUserStateResponse{
			DeviceGridLayout: json.RawMessage(`{}`),
		}))
	}
	if err != nil {
		h.logger.Error("대시보드 UI 상태 조회 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 UI 상태 조회 실패")
	}

	// 삭제되었거나 접근 권한을 잃은 대시보드를 가리키면 빈 문자열로 정규화한다
	// (spec.md §2.13 UB1 #11 — 400 이 아니다).
	activeUID, err := h.normalizeActiveUID(ctx, st.ActiveDashboardUID)
	if err != nil {
		return err
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toUserStateResponse(*st, activeUID)))
}

// PutState 는 요청자의 대시보드 UI 상태를 저장한다.
// PUT /api/v1/dashboard-state
func (h *DashboardHandler) PutState(ctx api.Context) error {
	username, err := h.stateUsername(ctx)
	if err != nil {
		return err
	}

	body, err := h.readBody(ctx)
	if err != nil {
		return err
	}
	var req dto.DashboardUserStateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return api.ErrBadRequest.WithMessage("invalid request body: " + err.Error())
	}

	layout := []byte(req.DeviceGridLayout)
	if len(layout) == 0 || string(layout) == "null" {
		layout = []byte(`{}`)
	} else if !json.Valid(layout) {
		return api.ErrBadRequest.WithMessage("device_grid_layout 이 유효한 JSON 이 아닙니다")
	}

	// 존재하지 않는 uid 는 저장 시점에도 빈 문자열로 정규화한다. 400 으로 거부하면
	// 클라이언트가 정정 저장(폴백)조차 할 수 없다(spec.md §2.13 UB1 #11).
	activeUID, err := h.normalizeActiveUID(ctx, req.ActiveDashboardUID)
	if err != nil {
		return err
	}

	st, err := h.state.Put(ctx.Context(), storage.DashboardUserState{
		Username:           username,
		ActiveDashboardUID: activeUID,
		DeviceGridLayout:   layout,
	}, parseIfMatch(ctx.GetHeader("If-Match")))
	if err != nil {
		if errors.Is(err, storage.ErrDashboardVersionMismatch) {
			return api.ErrConflict.WithMessage("dashboard state version mismatch")
		}
		h.logger.Error("대시보드 UI 상태 저장 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 UI 상태 저장 실패")
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toUserStateResponse(*st, st.ActiveDashboardUID)))
}

// normalizeActiveUID 는 존재하지 않는 uid 를 빈 문자열로 바꾼다.
func (h *DashboardHandler) normalizeActiveUID(ctx api.Context, uid string) (string, error) {
	if uid == "" {
		return "", nil
	}
	_, err := h.repo.Get(ctx.Context(), uid)
	if errors.Is(err, storage.ErrDashboardNotFound) {
		return "", nil
	}
	if err != nil {
		h.logger.Error("활성 대시보드 확인 실패", "uid", uid, "error", err)
		return "", api.ErrInternalServer.WithMessage("활성 대시보드 확인 실패")
	}
	return uid, nil
}

// -----------------------------------------------------------------------------
// 읽기 전용 호환 shim (spec.md §4.4)
// -----------------------------------------------------------------------------

// GetSharedShim 은 요청자가 view 가능한 `visibility != 'private'` 대시보드를 구
// 스냅샷 형상으로 합성해 반환한다.
// GET /api/v1/dashboards/shared
func (h *DashboardHandler) GetSharedShim(ctx api.Context) error {
	subj, err := h.subject(ctx)
	if err != nil {
		return err
	}
	items, err := h.repo.List(ctx.Context(), true)
	if err != nil {
		h.logger.Error("대시보드 목록 조회 실패(shim)", "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 목록 조회 실패")
	}
	aclByDashboard, err := h.aclForSubject(ctx.Context(), subj)
	if err != nil {
		return err
	}

	included := make([]storage.Dashboard, 0, len(items))
	for i := range items {
		if items[i].Visibility == dashboardacl.VisibilityPrivate {
			continue
		}
		if !dashboardacl.Evaluate(subj, toACLDashboard(items[i]), aclByDashboard[items[i].ID]).View {
			continue
		}
		included = append(included, items[i])
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(
		toDTO(SynthesizeDashboardSnapshot("global", "", included))))
}

// GetMineShim 은 요청자가 소유한 `visibility = 'private'` 대시보드를 구 스냅샷
// 형상으로 합성해 반환한다.
// GET /api/v1/dashboards/mine
func (h *DashboardHandler) GetMineShim(ctx api.Context) error {
	subj, err := h.subject(ctx)
	if err != nil {
		return err
	}
	items, err := h.repo.List(ctx.Context(), true)
	if err != nil {
		h.logger.Error("대시보드 목록 조회 실패(shim)", "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 목록 조회 실패")
	}

	included := make([]storage.Dashboard, 0, len(items))
	for i := range items {
		if items[i].Visibility != dashboardacl.VisibilityPrivate {
			continue
		}
		if items[i].Owner != subj.Username {
			continue
		}
		included = append(included, items[i])
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(
		toDTO(SynthesizeDashboardSnapshot("user", subj.Username, included))))
}

// -----------------------------------------------------------------------------
// 공통 로직
// -----------------------------------------------------------------------------

// load 는 {uid} 라우트의 공통 선행 절차이다: 요청자 판정 → 대시보드 조회 → 인가 판정.
//
// 존재하지 않는 uid 는 404 이다. 인가 실패는 호출자가 403 으로 매핑한다 — 두 코드를
// 여기서 함께 결정하면 경로마다 다른 의미를 부여하기 어렵다.
func (h *DashboardHandler) load(ctx api.Context) (dashboardacl.Subject, *storage.Dashboard, dashboardacl.Access, error) {
	var zero dashboardacl.Subject
	subj, err := h.subject(ctx)
	if err != nil {
		return zero, nil, dashboardacl.Access{}, err
	}

	uid := ctx.Param("uid")
	if uid == "" {
		return zero, nil, dashboardacl.Access{}, api.ErrBadRequest.WithMessage("uid 가 필요합니다")
	}

	d, err := h.repo.Get(ctx.Context(), uid)
	if errors.Is(err, storage.ErrDashboardNotFound) {
		return zero, nil, dashboardacl.Access{}, api.ErrNotFound.WithMessage("대시보드를 찾을 수 없습니다")
	}
	if err != nil {
		h.logger.Error("대시보드 조회 실패", "uid", uid, "error", err)
		return zero, nil, dashboardacl.Access{}, api.ErrInternalServer.WithMessage("대시보드 조회 실패")
	}

	acl, err := h.aclFor(ctx.Context(), d.ID)
	if err != nil {
		return zero, nil, dashboardacl.Access{}, err
	}
	return subj, d, dashboardacl.Evaluate(subj, toACLDashboard(*d), acl), nil
}

// subject 는 요청자의 인가 판정 입력을 만든다.
//
// 권한은 토큰이 아니라 요청 시점에 조회한다(SPEC-AUTH-005 §4.3). 그래서 역할 변경이
// 기존 토큰에도 즉시 반영된다(acceptance.md AC-07, AC-16).
func (h *DashboardHandler) subject(ctx api.Context) (dashboardacl.Subject, error) {
	if !h.authEnabled {
		// 인증 비활성 배포에서 대시보드만 잠그면 인증 도입 이전 배포가 사용 불가가
		// 된다(spec.md §2.10 S1). owner 도 빈 문자열이다.
		return dashboardacl.Subject{AuthEnabled: false}, nil
	}

	username := ctx.UserID()
	if username == "" {
		return dashboardacl.Subject{}, api.ErrUnauthorized.WithMessage("authentication required")
	}

	role := ctx.UserRole()
	perms := map[string]struct{}{}
	if h.perms != nil {
		effective, err := h.perms.EffectiveRole(ctx.Context(), username, role)
		if err != nil {
			h.logger.Error("유효 역할 조회 실패", "username", username, "error", err)
			return dashboardacl.Subject{}, api.ErrInternalServer.WithMessage("권한 조회 실패")
		}
		role = effective

		list, err := h.perms.Permissions(ctx.Context(), role)
		if err != nil {
			h.logger.Error("역할 권한 조회 실패", "role", role, "error", err)
			return dashboardacl.Subject{}, api.ErrInternalServer.WithMessage("권한 조회 실패")
		}
		for _, p := range list {
			perms[p] = struct{}{}
		}
	}

	return dashboardacl.Subject{
		Username:    username,
		Role:        role,
		Permissions: perms,
		AuthEnabled: true,
	}, nil
}

// aclFor 는 단일 대시보드의 ACL 행 전체를 조회한다.
//
// 단건 경로는 대시보드가 하나뿐이므로 subject 로 좁히지 않고 전량을 넘긴다 —
// 매치 판정은 dashboardacl.Evaluate 가 수행한다. 필터를 여기서도 하면 판정 규칙이
// 두 곳에 생긴다.
func (h *DashboardHandler) aclFor(ctx context.Context, dashboardID int64) ([]dashboardacl.ACLEntry, error) {
	if h.acl == nil {
		return nil, nil
	}
	rows, err := h.acl.ListByDashboard(ctx, dashboardID)
	if err != nil {
		h.logger.Error("대시보드 ACL 조회 실패", "dashboard_id", dashboardID, "error", err)
		return nil, api.ErrInternalServer.WithMessage("대시보드 권한 조회 실패")
	}
	return toACLEntries(rows), nil
}

// aclForSubject 는 목록 판정용으로 요청자에게 매치될 수 있는 ACL 행 전체를 단일
// 질의로 가져와 dashboard_id 별로 묶는다 (spec.md §5 — 행당 추가 질의 없음).
func (h *DashboardHandler) aclForSubject(ctx context.Context, subj dashboardacl.Subject) (map[int64][]dashboardacl.ACLEntry, error) {
	out := map[int64][]dashboardacl.ACLEntry{}
	if h.acl == nil {
		return out, nil
	}
	subjects := dashboardacl.Subjects(subj)
	if len(subjects) == 0 {
		return out, nil
	}
	rows, err := h.acl.ListBySubjects(ctx, subjects)
	if err != nil {
		h.logger.Error("대시보드 ACL 목록 조회 실패", "error", err)
		return nil, api.ErrInternalServer.WithMessage("대시보드 권한 조회 실패")
	}
	for _, r := range rows {
		out[r.DashboardID] = append(out[r.DashboardID], dashboardacl.ACLEntry{Subject: r.Subject, Level: r.Level})
	}
	return out, nil
}

// stateUsername 은 /dashboard-state 의 대상 사용자를 세션 사용자로 고정한다.
func (h *DashboardHandler) stateUsername(ctx api.Context) (string, error) {
	if !h.authEnabled {
		// 인증 비활성 배포에서는 사용자 구분이 없으므로 단일 행을 쓴다.
		return "", nil
	}
	username := ctx.UserID()
	if username == "" {
		return "", api.ErrUnauthorized.WithMessage("authentication required")
	}
	return username, nil
}

// updateError 는 저장소 Update 오류를 HTTP 응답으로 매핑한다.
//
// If-Match 불일치는 409 이며 서버 상태는 변하지 않는다(spec.md §2.13 UB1 #10).
// 409 본문에는 서버측 최신 상태를 담아 클라이언트가 재PUT 을 결정할 수 있게 한다.
func (h *DashboardHandler) updateError(ctx api.Context, err error, uid string, subj dashboardacl.Subject) error {
	if errors.Is(err, storage.ErrDashboardNotFound) {
		return api.ErrNotFound.WithMessage("대시보드를 찾을 수 없습니다")
	}
	if errors.Is(err, storage.ErrDashboardVersionMismatch) {
		latest, getErr := h.repo.Get(ctx.Context(), uid)
		if getErr != nil {
			return api.ErrConflict.WithMessage("version mismatch (latest unavailable)")
		}
		acl, aclErr := h.aclFor(ctx.Context(), latest.ID)
		if aclErr != nil {
			return aclErr
		}
		access := dashboardacl.Evaluate(subj, toACLDashboard(*latest), acl)
		return ctx.JSON(http.StatusConflict, dto.NewSuccessResponse(toDashboardDetail(*latest, access)))
	}
	h.logger.Error("대시보드 저장 실패", "uid", uid, "error", err)
	return api.ErrInternalServer.WithMessage("대시보드 저장 실패")
}

// readBody 는 256KB 상한을 적용해 요청 본문을 읽는다 (spec.md §2.13 UB1 #9).
func (h *DashboardHandler) readBody(ctx api.Context) ([]byte, error) {
	// Content-Length 헤더 기반 사전 거부 (정확하지 않을 수 있으므로 reader 단계도 보강).
	if cl := ctx.GetHeader("Content-Length"); cl != "" {
		if n, err := strconv.ParseInt(cl, 10, 64); err == nil && n > maxDashboardPayloadBytes {
			return nil, errPayloadTooLargeAPI()
		}
	}
	body, err := readLimitedBody(ctx, maxDashboardPayloadBytes)
	if err != nil {
		if errors.Is(err, errPayloadTooLarge) {
			return nil, errPayloadTooLargeAPI()
		}
		return nil, api.ErrBadRequest.WithMessage("read body: " + err.Error())
	}
	if len(body) == 0 {
		body = []byte(`{}`)
	}
	return body, nil
}

// forbiddenDashboard 는 인가 실패 응답이다.
//
// 부족한 권한 키를 본문에 노출하지 않는다(spec.md §5 보안 — acceptance.md AC-03).
func forbiddenDashboard() error {
	return api.ErrForbidden.WithMessage("대시보드에 접근할 권한이 없습니다")
}

// validateDashboardName 은 이름 규칙(1~64자, 공백만 불가)을 검증하고 trim 한다.
func validateDashboardName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", api.ErrBadRequest.WithMessage("name 은 공백만으로 구성될 수 없습니다")
	}
	if utf8.RuneCountInString(name) > maxDashboardNameRunes {
		return "", api.ErrBadRequest.WithMessage(
			fmt.Sprintf("name 은 %d자 이하여야 합니다", maxDashboardNameRunes))
	}
	return name, nil
}

// normalizeDashboardPayload 는 payload 를 검증하고 기본값을 채운다.
func normalizeDashboardPayload(raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []byte(emptyDashboardPayload), nil
	}
	if !json.Valid(raw) {
		return nil, api.ErrBadRequest.WithMessage("payload 가 유효한 JSON 이 아닙니다")
	}
	return []byte(raw), nil
}

// newDashboardUID 는 서버가 대시보드 식별자를 발급한다.
//
// 요청 본문의 uid 는 무시된다(spec.md §2.13 UB1 #4). 예약어(shared/mine/state)는
// uuid 형식상 생성될 수 없지만, 발급기를 바꾸더라도 예약어가 새어나가지 않도록
// 명시적으로 재발급한다(spec.md §2.1).
func newDashboardUID() string {
	for {
		uid := uuid.NewString()
		if _, reserved := dashboardReservedUIDs[uid]; !reserved {
			return uid
		}
	}
}

// -----------------------------------------------------------------------------
// Body / header parsing
// -----------------------------------------------------------------------------

// errPayloadTooLarge 는 readLimitedBody 의 내부 sentinel.
var errPayloadTooLarge = errors.New("payload too large")

// errPayloadTooLargeAPI 는 256KB 초과 시 반환되는 APIError 를 생성한다 (HTTP 413).
//
// api 패키지의 사전 정의 sentinel 에는 413 이 없으므로 핸들러 레벨에서 직접 생성.
// router.handleError 가 APIError.HTTPCode 를 그대로 사용하여 응답 코드를 결정한다.
func errPayloadTooLargeAPI() *api.APIError {
	return &api.APIError{
		HTTPCode: http.StatusRequestEntityTooLarge,
		Code:     "PAYLOAD_TOO_LARGE",
		Message:  fmt.Sprintf("payload exceeds %d bytes", maxDashboardPayloadBytes),
	}
}

// readLimitedBody 는 ctx 의 요청 본문을 maxBytes 까지만 읽어 반환한다.
//
// LimitReader 로 maxBytes+1 만큼 읽어 정확히 한도 검출이 가능하다. maxBytes 초과 시
// errPayloadTooLarge 반환.
//
// 본 함수는 api.Context 인터페이스의 hidden interface assertion 으로 *http.Request
// 를 얻는다. api.httpContext 가 그 인터페이스를 구현하지 않으면 fallback 으로
// Bind 후 marshalled 바이트를 사용 (테스트에서는 fallback 경로 사용).
func readLimitedBody(ctx api.Context, maxBytes int64) ([]byte, error) {
	// httpContext 의 raw request 에 접근하는 우회 인터페이스.
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
			return nil, errPayloadTooLarge
		}
		return data, nil
	}
	// Fallback: ctx.Bind 가 io.EOF 를 반환하면 빈 body 로 처리.
	var raw json.RawMessage
	if err := ctx.Bind(&raw); err != nil {
		// "request body is empty" → 빈 객체로 처리. 그 외 에러 전파.
		if errors.Is(err, io.EOF) {
			return []byte(`{}`), nil
		}
		// api.ErrBadRequest 에서 "request body is empty" 메시지로 들어오는 경우.
		return nil, err
	}
	if int64(len(raw)) > maxBytes {
		return nil, errPayloadTooLarge
	}
	return raw, nil
}

// parseIfMatch 는 If-Match 헤더를 int64 로 파싱한다.
//
// 헤더가 없거나 잘못된 형식이면 -1 (unconditional).
//
// 표준 If-Match 는 ETag 문자열을 사용하지만, 본 SPEC 은 단순 정수 version 을 사용
// 한다. 따옴표/W/ 접두사가 있을 경우 무시한다.
func parseIfMatch(header string) int64 {
	if header == "" {
		return -1
	}
	// 따옴표 제거, W/ 접두사 제거 (방어적)
	s := header
	if len(s) >= 2 && s[0] == 'W' && s[1] == '/' {
		s = s[2:]
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return -1
	}
	if n < 0 {
		return -1
	}
	return n
}

// -----------------------------------------------------------------------------
// DTO conversion
// -----------------------------------------------------------------------------

// toACLDashboard 는 저장소 행을 판정 입력 값 타입으로 좁힌다.
func toACLDashboard(d storage.Dashboard) dashboardacl.Dashboard {
	return dashboardacl.Dashboard{UID: d.UID, Owner: d.Owner, Visibility: d.Visibility}
}

// toACLEntries 는 저장소 ACL 행을 판정 입력 값 타입으로 좁힌다.
func toACLEntries(rows []storage.DashboardACLEntry) []dashboardacl.ACLEntry {
	out := make([]dashboardacl.ACLEntry, 0, len(rows))
	for _, r := range rows {
		out = append(out, dashboardacl.ACLEntry{Subject: r.Subject, Level: r.Level})
	}
	return out
}

// toDashboardMeta 는 저장소 행 + 인가 판정을 목록 응답 DTO 로 변환한다.
func toDashboardMeta(d storage.Dashboard, a dashboardacl.Access) dto.DashboardMeta {
	return dto.DashboardMeta{
		UID:        d.UID,
		Name:       d.Name,
		Owner:      d.Owner,
		Visibility: d.Visibility,
		IsDefault:  d.IsDefault,
		SortOrder:  d.SortOrder,
		Version:    d.Version,
		CreatedAt:  d.CreatedAt,
		UpdatedAt:  d.UpdatedAt,
		CanEdit:    a.Edit,
		CanDelete:  a.Delete,
		CanGrant:   a.Grant,
	}
}

// toDashboardDetail 은 payload 를 포함한 단건 응답 DTO 로 변환한다.
func toDashboardDetail(d storage.Dashboard, a dashboardacl.Access) dto.DashboardDetail {
	payload := json.RawMessage(d.Payload)
	if len(payload) == 0 {
		payload = json.RawMessage(emptyDashboardPayload)
	}
	return dto.DashboardDetail{DashboardMeta: toDashboardMeta(d, a), Payload: payload}
}

// toUserStateResponse 는 사용자 UI 상태를 응답 DTO 로 변환한다.
func toUserStateResponse(st storage.DashboardUserState, activeUID string) dto.DashboardUserStateResponse {
	layout := json.RawMessage(st.DeviceGridLayout)
	if len(layout) == 0 {
		layout = json.RawMessage(`{}`)
	}
	return dto.DashboardUserStateResponse{
		ActiveDashboardUID: activeUID,
		DeviceGridLayout:   layout,
		Version:            st.Version,
		UpdatedAt:          st.UpdatedAt,
	}
}

// toDTO 는 storage.DashboardSnapshot → dto.DashboardSnapshot 변환.
//
// scope=global 인 경우 Owner 필드를 nil 로 만들어 JSON null 직렬화를 보장한다.
// scope=user 인 경우 *string 으로 username 을 감싼다.
//
// Payload 는 저장된 JSON 원본을 그대로 통과시킨다 (re-encoding 없음, byte-stable).
func toDTO(s *storage.DashboardSnapshot) dto.DashboardSnapshot {
	var owner *string
	if s.Scope == "user" {
		v := s.Owner
		owner = &v
	}
	return dto.DashboardSnapshot{
		Scope:     s.Scope,
		Owner:     owner,
		Version:   s.Version,
		UpdatedAt: s.UpdatedAt,
		Payload:   s.Payload,
	}
}

// DashboardSnapshotToDTO 는 toDTO 의 exported 래퍼이다.
//
// 원격 관리 query 프록시(cmd/xflowd)가 노드-로컬 GET /dashboards/{shared,mine} 와
// IDENTICAL 한 응답 형상(소문자 키, payload=raw JSON object, global 시 owner=null)을
// 재사용하도록 변환을 공개한다(SPEC-REMOTE-001 그룹 L — 프런트 호환). 로컬 핸들러는
// 계속 toDTO 를 직접 사용한다(SoT 동일).
//
// @SPEC:SPEC-DASHBOARD-004 (M4) — 시그니처는 고정이다. 원격 프록시 3개 파일이 본
// 함수에 의존하므로, shim 이 신규 모델에서 레거시 스냅샷을 합성해 넘긴다
// (SynthesizeDashboardSnapshot 참조).
func DashboardSnapshotToDTO(s *storage.DashboardSnapshot) dto.DashboardSnapshot {
	return toDTO(s)
}
