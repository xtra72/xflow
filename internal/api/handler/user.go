package handler

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/auth"
	"github.com/xtra/xflow/internal/storage"
)

// @SPEC:SPEC-AUTH-005 (M6) — 사용자 관리 API (spec.md §2.2 U2, §2.4 UB1).
//
// 웹 UI 없이 API 만으로 사용자 관리가 완결되어야 한다 (spec.md §6.5).

// RolePermissionService 는 역할→권한 조회와 캐시 무효화를 제공한다.
//
// *auth.PermissionCache 가 본 인터페이스를 만족한다. 핸들러가 구체 타입 대신
// 인터페이스에 의존하여 테스트에서 DB 없이 대체 구현을 주입할 수 있게 한다.
type RolePermissionService interface {
	// Permissions 는 역할의 권한 키 목록을 반환한다. 없는 역할은 빈 슬라이스이다.
	Permissions(ctx context.Context, role string) ([]string, error)
	// Invalidate 는 단일 역할의 캐시를 무효화한다.
	Invalidate(role string)
	// InvalidateAll 은 캐시 전체를 무효화한다.
	InvalidateAll()
}

// 잠금 방지 불변식(UB1) 위반 시 반환되는 409 에러들.
//
// acceptance.md AC-09 는 각 거부가 원인을 식별할 수 있는 에러 코드를 포함할 것을
// 요구한다. api.ErrConflict.WithMessage 는 Code 를 "CONFLICT" 로 유지하므로 원인별로
// 별도 APIError 를 정의한다.
var (
	// ErrLastAdminUser 는 관리 권한 보유자가 0 이 되는 삭제·강등을 거부한다.
	ErrLastAdminUser = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "LAST_ADMIN_USER",
		Message:  "관리 권한을 보유한 마지막 사용자는 삭제하거나 강등할 수 없습니다",
	}
	// ErrSelfDeletion 은 자기 자신의 삭제를 거부한다.
	ErrSelfDeletion = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "SELF_DELETION",
		Message:  "자기 자신은 삭제할 수 없습니다",
	}
	// ErrUserExists 는 중복 username 등록을 거부한다.
	ErrUserExists = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "USER_EXISTS",
		Message:  "이미 존재하는 사용자입니다",
	}
	// ErrLastDashboardDeleter 는 dashboard.delete 보유자가 0 이 되는 삭제·강등을
	// 거부한다 (@SPEC:SPEC-DASHBOARD-004 spec.md §2.13 UB1 #6, acceptance.md AC-19).
	//
	// 0 이 되면 아무도 대시보드를 정리할 수 없는 상태가 되고, 그 상태는 역할 편집으로
	// 만 빠져나올 수 있다 — 관리 API 접근 자체가 남아 있어도 대시보드는 영구히
	// 방치된다. ErrLastAdminUser 와 별도 코드로 두어 원인이 응답에서 식별된다.
	ErrLastDashboardDeleter = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "LAST_DASHBOARD_DELETER",
		Message:  "대시보드 삭제 권한을 보유한 마지막 사용자는 삭제하거나 강등할 수 없습니다",
	}
	// ErrDashboardOwnershipTransferUnavailable 은 소유권 승계 대상이 없어 대시보드가
	// 고아가 되는 사용자 삭제를 거부한다 (spec.md §2.13 UB1 #7, acceptance.md AC-20).
	ErrDashboardOwnershipTransferUnavailable = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "DASHBOARD_OWNERSHIP_TRANSFER_UNAVAILABLE",
		Message:  "삭제 대상이 소유한 대시보드를 승계할 관리자를 결정할 수 없습니다",
	}
)

// minPasswordLength 는 사용자 등록·재설정 시 요구되는 최소 비밀번호 길이이다
// (acceptance.md 엣지 케이스: 8자 미만 → 400).
//
// auth.HashPassword 의 하한(4자)보다 엄격하다. 기존 본인 비밀번호 변경 경로
// (PUT /auth/password)의 정책은 건드리지 않는다.
const minPasswordLength = 8

// 관리 권한 판정에 사용되는 권한 키 (spec.md §2.4 불변식 1).
const (
	permUserDelete = "user.delete"
	permRoleUpdate = "role.update"
	// permDashboardDelete 는 대시보드 정리 권한이다
	// (@SPEC:SPEC-DASHBOARD-004 spec.md §2.13 UB1 #6).
	permDashboardDelete = "dashboard.delete"
)

// UserHandler 는 사용자 관리 API 핸들러이다.
type UserHandler struct {
	db     *sql.DB
	perms  RolePermissionService
	logger *slog.Logger
}

// NewUserHandler 는 새 UserHandler 를 생성한다.
func NewUserHandler(db *sql.DB, perms RolePermissionService, logger *slog.Logger) *UserHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &UserHandler{db: db, perms: perms, logger: logger}
}

// RegisterRoutes 는 사용자 관리 라우트를 등록한다.
//
//	GET    /users                     -> List     (user.read)
//	POST   /users                     -> Create   (user.create)
//	PUT    /users/{username}          -> Update   (user.update)
//	PUT    /users/{username}/password -> ResetPassword (user.update)
//	DELETE /users/{username}          -> Delete   (user.delete)
func (h *UserHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GETPerm("/users", "user.read", h.List)
	g.POSTPerm("/users", "user.create", h.Create)
	// 리터럴 /password 세그먼트가 있어 PUT /users/{username} 과 충돌하지 않는다.
	g.PUTPerm("/users/{username}/password", "user.update", h.ResetPassword)
	g.PUTPerm("/users/{username}", "user.update", h.Update)
	g.DELETEPerm("/users/{username}", "user.delete", h.Delete)
}

// List 는 사용자 목록을 반환한다 (비밀번호 해시 제외).
// GET /api/v1/users
func (h *UserHandler) List(ctx api.Context) error {
	rows, err := storage.ListUsers(ctx.Context(), h.db)
	if err != nil {
		h.logger.Error("사용자 목록 조회 실패", "error", err)
		return api.ErrInternalServer.WithMessage("사용자 목록 조회 실패")
	}

	out := make([]dto.UserResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, toUserResponse(r))
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(out))
}

// Create 는 사용자를 등록한다.
// POST /api/v1/users
func (h *UserHandler) Create(ctx api.Context) error {
	var req dto.CreateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	if req.Username == "" {
		return api.ErrBadRequest.WithMessage("username 은 필수입니다")
	}
	if len(req.Password) < minPasswordLength {
		return api.ErrBadRequest.WithMessage("password 는 최소 8자 이상이어야 합니다")
	}
	if req.Role == "" {
		return api.ErrBadRequest.WithMessage("role 은 필수입니다")
	}
	// 존재하지 않는 역할 부여는 400 이며 사용자는 생성되지 않는다.
	if err := h.requireRoleExists(ctx.Context(), req.Role); err != nil {
		return err
	}

	// 중복 username 은 409 이다.
	if _, err := storage.GetUserByUsername(ctx.Context(), h.db, req.Username); err == nil {
		return ErrUserExists
	} else if !errors.Is(err, storage.ErrUserNotFound) {
		h.logger.Error("사용자 조회 실패", "username", req.Username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 조회 실패")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("비밀번호 해싱 실패", "error", err)
		return api.ErrInternalServer.WithMessage("비밀번호 해싱 실패")
	}
	if err := storage.InsertUser(ctx.Context(), h.db, req.Username, hash, req.Role, 0, 0); err != nil {
		h.logger.Error("사용자 생성 실패", "username", req.Username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 생성 실패")
	}

	row, err := storage.GetUserByUsername(ctx.Context(), h.db, req.Username)
	if err != nil {
		h.logger.Error("생성된 사용자 조회 실패", "username", req.Username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 조회 실패")
	}

	h.logger.Info("사용자 생성", "username", req.Username, "role", req.Role, "actor", ctx.UserID())
	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(toUserResponse(*row)))
}

// Update 는 사용자의 역할을 변경한다.
// PUT /api/v1/users/{username}
func (h *UserHandler) Update(ctx api.Context) error {
	username := ctx.Param("username")
	if username == "" {
		return api.ErrBadRequest.WithMessage("username 이 필요합니다")
	}

	var req dto.UpdateUserRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.Role == "" {
		return api.ErrBadRequest.WithMessage("role 은 필수입니다")
	}

	current, err := storage.GetUserByUsername(ctx.Context(), h.db, username)
	if errors.Is(err, storage.ErrUserNotFound) {
		return api.ErrNotFound.WithMessage("사용자를 찾을 수 없습니다")
	}
	if err != nil {
		h.logger.Error("사용자 조회 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 조회 실패")
	}

	// 존재하지 않는 역할 부여는 400 이며 사용자 상태는 불변이다.
	if err := h.requireRoleExists(ctx.Context(), req.Role); err != nil {
		return err
	}

	// UB1-1: 관리 권한 보유자가 0 이 되는 강등을 차단한다.
	if current.Role != req.Role {
		if err := h.guardAdminCapacity(ctx.Context(), "", username, req.Role); err != nil {
			return err
		}
	}

	if err := storage.UpdateUserRole(ctx.Context(), h.db, username, req.Role); err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return api.ErrNotFound.WithMessage("사용자를 찾을 수 없습니다")
		}
		h.logger.Error("사용자 역할 변경 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 역할 변경 실패")
	}

	row, err := storage.GetUserByUsername(ctx.Context(), h.db, username)
	if err != nil {
		h.logger.Error("변경된 사용자 조회 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 조회 실패")
	}

	h.logger.Info("사용자 역할 변경",
		"username", username, "from", current.Role, "to", req.Role, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toUserResponse(*row)))
}

// ResetPassword 는 관리자가 다른 사용자의 비밀번호를 재설정한다.
// PUT /api/v1/users/{username}/password
func (h *UserHandler) ResetPassword(ctx api.Context) error {
	username := ctx.Param("username")
	if username == "" {
		return api.ErrBadRequest.WithMessage("username 이 필요합니다")
	}

	var req dto.ResetPasswordRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if len(req.Password) < minPasswordLength {
		return api.ErrBadRequest.WithMessage("password 는 최소 8자 이상이어야 합니다")
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		h.logger.Error("비밀번호 해싱 실패", "error", err)
		return api.ErrInternalServer.WithMessage("비밀번호 해싱 실패")
	}

	if err := storage.UpdatePasswordHash(ctx.Context(), h.db, username, hash); err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return api.ErrNotFound.WithMessage("사용자를 찾을 수 없습니다")
		}
		h.logger.Error("비밀번호 재설정 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("비밀번호 재설정 실패")
	}

	h.logger.Info("비밀번호 재설정", "username", username, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(map[string]string{
		"message": "비밀번호가 재설정되었습니다",
	}))
}

// Delete 는 사용자를 삭제한다.
// DELETE /api/v1/users/{username}
func (h *UserHandler) Delete(ctx api.Context) error {
	username := ctx.Param("username")
	if username == "" {
		return api.ErrBadRequest.WithMessage("username 이 필요합니다")
	}

	if _, err := storage.GetUserByUsername(ctx.Context(), h.db, username); err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return api.ErrNotFound.WithMessage("사용자를 찾을 수 없습니다")
		}
		h.logger.Error("사용자 조회 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 조회 실패")
	}

	// 검사 순서: UB1-1(마지막 관리자) → UB1-2(자기 자신).
	//
	// 자기 자신을 먼저 검사하면 "관리자가 1명뿐일 때 자기 삭제" 가 항상 자기-삭제로
	// 판정되어 마지막-관리자 불변식을 독립적으로 관측할 수 없다. 두 불변식이 각각
	// 검증 가능하도록 관리 정원 검사를 앞에 둔다.
	if err := h.guardAdminCapacity(ctx.Context(), username, "", ""); err != nil {
		return err
	}
	if username == ctx.UserID() {
		return ErrSelfDeletion
	}

	// @SPEC:SPEC-DASHBOARD-004 (spec.md §2.13 UB1 #7, acceptance.md AC-20)
	// 소유자가 사라져 대시보드가 고아가 되는 상태를 만들지 않는다. 승계 대상은
	// 삭제를 실행한 관리자다(spec.md §6 가정 5 — 다른 사용자로의 지정 승계는 범위 밖).
	// 승계가 불가능하면 삭제 자체를 409 로 거부한다 — 대시보드를 조용히 잃는 것보다
	// 삭제를 막는 편이 복구 가능하다.
	if err := h.transferDashboardOwnership(ctx, username); err != nil {
		return err
	}

	if err := storage.DeleteUser(ctx.Context(), h.db, username); err != nil {
		if errors.Is(err, storage.ErrUserNotFound) {
			return api.ErrNotFound.WithMessage("사용자를 찾을 수 없습니다")
		}
		h.logger.Error("사용자 삭제 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("사용자 삭제 실패")
	}

	h.logger.Info("사용자 삭제", "username", username, "actor", ctx.UserID())
	return ctx.NoContent(http.StatusNoContent)
}

// requireRoleExists 는 역할이 존재하지 않으면 400 을 반환한다.
//
// 존재하지 않는 역할 부여는 사용자 상태를 바꾸지 않은 채 400 이어야 한다
// (acceptance.md 엣지 케이스).
func (h *UserHandler) requireRoleExists(ctx context.Context, role string) error {
	if _, err := storage.GetRoleByName(ctx, h.db, role); err != nil {
		if errors.Is(err, storage.ErrRoleNotFound) {
			return api.ErrBadRequest.WithMessage("존재하지 않는 역할입니다")
		}
		h.logger.Error("역할 조회 실패", "role", role, "error", err)
		return api.ErrInternalServer.WithMessage("역할 조회 실패")
	}
	return nil
}

// guardAdminCapacity 는 변경 후에도 관리 권한 보유자가 남는지 검사한다 (UB1-1).
//
// deleted    : 삭제될 사용자 (없으면 "")
// changed    : 역할이 바뀔 사용자 (없으면 "")
// changedRole: changed 의 새 역할
//
// user.delete 또는 role.update 보유자가 0 이 되면 ErrLastAdminUser 를 반환한다.
// 둘 중 하나라도 0 이 되면 관리 API 접근이 영구히 소실되므로 각각 검사한다.
func (h *UserHandler) guardAdminCapacity(ctx context.Context, deleted, changed, changedRole string) error {
	if h.perms == nil {
		// 인가가 비활성인 배포에서는 잠금 상태 자체가 성립하지 않는다.
		return nil
	}

	users, err := storage.ListUsers(ctx, h.db)
	if err != nil {
		h.logger.Error("사용자 목록 조회 실패", "error", err)
		return api.ErrInternalServer.WithMessage("사용자 목록 조회 실패")
	}

	// 역할별 권한 집합은 사용자 수만큼 반복 조회되므로 요청 단위로 메모한다.
	holds := make(map[string]map[string]bool, 4)
	rolePerms := func(role string) (map[string]bool, error) {
		if m, ok := holds[role]; ok {
			return m, nil
		}
		perms, err := h.perms.Permissions(ctx, role)
		if err != nil {
			return nil, err
		}
		m := make(map[string]bool, len(perms))
		for _, p := range perms {
			m[p] = true
		}
		holds[role] = m
		return m, nil
	}

	userDeleteHolders, roleUpdateHolders, dashboardDeleteHolders := 0, 0, 0
	for _, u := range users {
		if u.Username == deleted {
			continue
		}
		role := u.Role
		if u.Username == changed {
			role = changedRole
		}
		granted, err := rolePerms(role)
		if err != nil {
			h.logger.Error("역할 권한 조회 실패", "role", role, "error", err)
			return api.ErrInternalServer.WithMessage("역할 권한 조회 실패")
		}
		if granted[permUserDelete] {
			userDeleteHolders++
		}
		if granted[permRoleUpdate] {
			roleUpdateHolders++
		}
		if granted[permDashboardDelete] {
			dashboardDeleteHolders++
		}
	}

	if userDeleteHolders == 0 || roleUpdateHolders == 0 {
		return ErrLastAdminUser
	}
	// @SPEC:SPEC-DASHBOARD-004 (spec.md §2.13 UB1 #6, acceptance.md AC-19)
	// 관리 API 접근이 남아 있어도 대시보드 정리 권한이 0 이면 대시보드는 영구히
	// 방치된다. 따라서 별도 불변식으로 검사하고 별도 코드로 거부한다.
	if dashboardDeleteHolders == 0 {
		return ErrLastDashboardDeleter
	}
	return nil
}

// transferDashboardOwnership 은 삭제 대상이 소유한 대시보드를 삭제 실행자에게
// 승계하고, 삭제 대상을 향하던 ACL 행(`user:<username>`)을 제거한다 (UB1 #7).
//
// 소유 대시보드가 0장이면 승계 대상 검증 없이 통과한다 — 승계할 것이 없는데
// 삭제를 막으면 인증 비활성 배포나 스크립트 삭제 경로가 이유 없이 실패한다.
// 다만 `user:<username>` ACL 잔여 행은 그 경우에도 정리한다.
func (h *UserHandler) transferDashboardOwnership(ctx api.Context, username string) error {
	owned, err := storage.CountDashboardsByOwner(ctx.Context(), h.db, username)
	if err != nil {
		h.logger.Error("대시보드 소유 수 조회 실패", "username", username, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 소유 수 조회 실패")
	}

	actor := ctx.UserID()
	if owned > 0 {
		// 승계 대상은 삭제를 실행한 관리자다. 실행자를 알 수 없거나(인증 비활성이
		// 아닌데 컨텍스트가 비어 있음) 실행자 자신이 삭제 대상이면 승계가 성립하지 않는다.
		if actor == "" || actor == username {
			return ErrDashboardOwnershipTransferUnavailable
		}
		if _, err := storage.GetUserByUsername(ctx.Context(), h.db, actor); err != nil {
			// 실행자가 users 에 없으면(원격 노드 토큰 등) 승계 대상이 될 수 없다.
			return ErrDashboardOwnershipTransferUnavailable
		}
	}

	moved, err := storage.TransferDashboardOwnership(ctx.Context(), h.db, username, actor)
	if err != nil {
		h.logger.Error("대시보드 소유권 승계 실패", "from", username, "to", actor, "error", err)
		return api.ErrInternalServer.WithMessage("대시보드 소유권 승계 실패")
	}
	if moved > 0 {
		h.logger.Info("대시보드 소유권 승계", "from", username, "to", actor, "count", moved)
	}
	return nil
}

// toUserResponse 는 storage.UserRow 를 응답 DTO 로 변환한다 (해시 제외).
func toUserResponse(u storage.UserRow) dto.UserResponse {
	return dto.UserResponse{
		Username:  u.Username,
		Role:      u.Role,
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
	}
}
