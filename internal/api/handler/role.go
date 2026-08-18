package handler

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"

	"github.com/xtra/xflow/internal/api"
	"github.com/xtra/xflow/internal/api/dto"
	"github.com/xtra/xflow/internal/rbac"
	"github.com/xtra/xflow/internal/storage"
)

// @SPEC:SPEC-AUTH-005 (M6) — 역할 관리 API + 권한 카탈로그 (spec.md §2.1, §2.2, §2.4).

// 잠금 방지 불변식(UB1) 위반 시 반환되는 409 에러들 (역할 측).
var (
	// ErrBuiltinRoleImmutable 은 빌트인 역할(admin/editor/viewer)의 삭제를 거부한다.
	ErrBuiltinRoleImmutable = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "BUILTIN_ROLE_IMMUTABLE",
		Message:  "빌트인 역할은 삭제할 수 없습니다",
	}
	// ErrAdminRoleImmutable 은 admin 역할의 권한·이름 수정을 거부한다.
	//
	// 저장소의 시드는 부팅마다 admin 권한을 다시 채우므로 DB 계층에 맡기면 변경이
	// 조용히 되돌려질 뿐이다. API 계층에서 명시적으로 거부해야 원인이 드러난다.
	ErrAdminRoleImmutable = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "ADMIN_ROLE_IMMUTABLE",
		Message:  "admin 역할은 항상 전체 권한을 보유하므로 수정할 수 없습니다",
	}
	// ErrRoleInUse 는 한 명 이상의 사용자가 사용 중인 역할의 삭제를 거부한다.
	ErrRoleInUse = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "ROLE_IN_USE",
		Message:  "사용 중인 역할은 삭제할 수 없습니다",
	}
	// ErrRoleExists 는 중복 역할 이름 생성·변경을 거부한다.
	ErrRoleExists = &api.APIError{
		HTTPCode: http.StatusConflict,
		Code:     "ROLE_EXISTS",
		Message:  "이미 존재하는 역할입니다",
	}
)

// RoleHandler 는 역할 관리 및 권한 카탈로그 API 핸들러이다.
type RoleHandler struct {
	db     *sql.DB
	perms  RolePermissionService
	logger *slog.Logger
}

// NewRoleHandler 는 새 RoleHandler 를 생성한다.
func NewRoleHandler(db *sql.DB, perms RolePermissionService, logger *slog.Logger) *RoleHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &RoleHandler{db: db, perms: perms, logger: logger}
}

// RegisterRoutes 는 역할 관리 라우트를 등록한다.
//
//	GET    /roles         -> List   (role.read)
//	POST   /roles         -> Create (role.create)
//	PUT    /roles/{name}  -> Update (role.update)
//	DELETE /roles/{name}  -> Delete (role.delete)
//	GET    /permissions   -> Catalog (인증만 — 권한 미부착)
func (h *RoleHandler) RegisterRoutes(g *api.RouteGroup) {
	g.GETPerm("/roles", "role.read", h.List)
	g.POSTPerm("/roles", "role.create", h.Create)
	g.PUTPerm("/roles/{name}", "role.update", h.Update)
	g.DELETEPerm("/roles/{name}", "role.delete", h.Delete)

	// 권한 카탈로그는 코드 상수이므로 비밀이 아니며, 웹 UI 가 역할 편집 화면을 그리려면
	// role.* 권한이 없어도 읽을 수 있어야 한다 (spec.md §2.2 "인증만").
	g.GET("/permissions", h.Catalog)
}

// List 는 역할 목록을 권한과 함께 반환한다.
// GET /api/v1/roles
func (h *RoleHandler) List(ctx api.Context) error {
	rows, err := storage.ListRoles(ctx.Context(), h.db)
	if err != nil {
		h.logger.Error("역할 목록 조회 실패", "error", err)
		return api.ErrInternalServer.WithMessage("역할 목록 조회 실패")
	}

	out := make([]dto.RoleResponse, 0, len(rows))
	for _, r := range rows {
		out = append(out, toRoleResponse(r))
	}
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(out))
}

// Create 는 커스텀 역할을 생성한다.
// POST /api/v1/roles
func (h *RoleHandler) Create(ctx api.Context) error {
	var req dto.CreateRoleRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}

	// 이름 규칙(소문자·숫자·하이픈, 1~32자) 위반은 400 이다.
	if !rbac.IsValidRoleName(req.Name) {
		return api.ErrBadRequest.WithMessage("역할 이름은 소문자·숫자·하이픈 1~32자여야 합니다")
	}
	// 카탈로그에 없는 권한 키가 하나라도 있으면 400 이며 역할은 생성되지 않는다.
	if bad := firstInvalidPermission(req.Permissions); bad != "" {
		return api.ErrBadRequest.WithMessage("정의되지 않은 권한 키입니다: " + bad)
	}

	err := storage.InsertRole(ctx.Context(), h.db, req.Name, req.Description, req.Permissions)
	switch {
	case errors.Is(err, storage.ErrRoleExists):
		return ErrRoleExists
	case errors.Is(err, storage.ErrInvalidRoleName):
		return api.ErrBadRequest.WithMessage("역할 이름은 소문자·숫자·하이픈 1~32자여야 합니다")
	case errors.Is(err, storage.ErrInvalidPermission):
		return api.ErrBadRequest.WithMessage("정의되지 않은 권한 키입니다")
	case err != nil:
		h.logger.Error("역할 생성 실패", "role", req.Name, "error", err)
		return api.ErrInternalServer.WithMessage("역할 생성 실패")
	}

	// 이전에 "없는 역할" 로 음성 캐시되었을 수 있으므로 무효화한다.
	h.invalidate(req.Name)

	row, err := storage.GetRoleByName(ctx.Context(), h.db, req.Name)
	if err != nil {
		h.logger.Error("생성된 역할 조회 실패", "role", req.Name, "error", err)
		return api.ErrInternalServer.WithMessage("역할 조회 실패")
	}

	h.logger.Info("역할 생성", "role", req.Name, "actor", ctx.UserID())
	return ctx.JSON(http.StatusCreated, dto.NewSuccessResponse(toRoleResponse(*row)))
}

// Update 는 역할의 권한 집합을 교체하거나 이름을 변경한다.
// PUT /api/v1/roles/{name}
func (h *RoleHandler) Update(ctx api.Context) error {
	name := ctx.Param("name")
	if name == "" {
		return api.ErrBadRequest.WithMessage("역할 이름이 필요합니다")
	}

	var req dto.UpdateRoleRequest
	if err := ctx.Bind(&req); err != nil {
		return err
	}
	if req.Name == nil && req.Permissions == nil {
		return api.ErrBadRequest.WithMessage("name 또는 permissions 중 하나는 필요합니다")
	}

	// UB1-4: admin 역할은 항상 전체 권한을 유지한다.
	if name == rbac.RoleAdmin {
		return ErrAdminRoleImmutable
	}

	if _, err := storage.GetRoleByName(ctx.Context(), h.db, name); err != nil {
		if errors.Is(err, storage.ErrRoleNotFound) {
			return api.ErrNotFound.WithMessage("역할을 찾을 수 없습니다")
		}
		h.logger.Error("역할 조회 실패", "role", name, "error", err)
		return api.ErrInternalServer.WithMessage("역할 조회 실패")
	}

	if req.Permissions != nil {
		if bad := firstInvalidPermission(*req.Permissions); bad != "" {
			return api.ErrBadRequest.WithMessage("정의되지 않은 권한 키입니다: " + bad)
		}
		err := storage.UpdateRolePermissions(ctx.Context(), h.db, name, *req.Permissions)
		switch {
		case errors.Is(err, storage.ErrRoleNotFound):
			return api.ErrNotFound.WithMessage("역할을 찾을 수 없습니다")
		case errors.Is(err, storage.ErrInvalidPermission):
			return api.ErrBadRequest.WithMessage("정의되지 않은 권한 키입니다")
		case err != nil:
			h.logger.Error("역할 권한 수정 실패", "role", name, "error", err)
			return api.ErrInternalServer.WithMessage("역할 권한 수정 실패")
		}
		h.invalidate(name)
	}

	finalName := name
	if req.Name != nil && *req.Name != name {
		newName := *req.Name
		if !rbac.IsValidRoleName(newName) {
			return api.ErrBadRequest.WithMessage("역할 이름은 소문자·숫자·하이픈 1~32자여야 합니다")
		}
		// 빌트인 역할 이름은 users.role 및 코드 상수와 결합되어 있어 변경하지 않는다.
		if rbac.IsBuiltinRole(name) {
			return ErrBuiltinRoleImmutable
		}
		err := storage.UpdateRoleName(ctx.Context(), h.db, name, newName)
		switch {
		case errors.Is(err, storage.ErrRoleNotFound):
			return api.ErrNotFound.WithMessage("역할을 찾을 수 없습니다")
		case errors.Is(err, storage.ErrRoleExists):
			return ErrRoleExists
		case errors.Is(err, storage.ErrInvalidRoleName):
			return api.ErrBadRequest.WithMessage("역할 이름은 소문자·숫자·하이픈 1~32자여야 합니다")
		case err != nil:
			h.logger.Error("역할 이름 변경 실패", "role", name, "error", err)
			return api.ErrInternalServer.WithMessage("역할 이름 변경 실패")
		}
		// 이름 변경은 users.role 까지 함께 갱신하므로 전체를 무효화한다.
		h.invalidateAll()
		finalName = newName
	}

	row, err := storage.GetRoleByName(ctx.Context(), h.db, finalName)
	if err != nil {
		h.logger.Error("수정된 역할 조회 실패", "role", finalName, "error", err)
		return api.ErrInternalServer.WithMessage("역할 조회 실패")
	}

	h.logger.Info("역할 수정", "role", name, "final_name", finalName, "actor", ctx.UserID())
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(toRoleResponse(*row)))
}

// Delete 는 역할을 삭제한다.
// DELETE /api/v1/roles/{name}
func (h *RoleHandler) Delete(ctx api.Context) error {
	name := ctx.Param("name")
	if name == "" {
		return api.ErrBadRequest.WithMessage("역할 이름이 필요합니다")
	}

	// UB1-3: 빌트인 역할은 삭제할 수 없다.
	if rbac.IsBuiltinRole(name) {
		return ErrBuiltinRoleImmutable
	}

	if _, err := storage.GetRoleByName(ctx.Context(), h.db, name); err != nil {
		if errors.Is(err, storage.ErrRoleNotFound) {
			return api.ErrNotFound.WithMessage("역할을 찾을 수 없습니다")
		}
		h.logger.Error("역할 조회 실패", "role", name, "error", err)
		return api.ErrInternalServer.WithMessage("역할 조회 실패")
	}

	// UB1-5: 한 명 이상의 사용자가 사용 중인 역할은 삭제할 수 없다.
	// (삭제하면 그 사용자가 존재하지 않는 역할을 가리켜 모든 요청이 403 이 된다.)
	inUse, err := storage.CountUsersByRole(ctx.Context(), h.db, name)
	if err != nil {
		h.logger.Error("역할 사용자 수 조회 실패", "role", name, "error", err)
		return api.ErrInternalServer.WithMessage("역할 사용자 수 조회 실패")
	}
	if inUse > 0 {
		return ErrRoleInUse
	}

	if err := storage.DeleteRole(ctx.Context(), h.db, name); err != nil {
		if errors.Is(err, storage.ErrRoleNotFound) {
			return api.ErrNotFound.WithMessage("역할을 찾을 수 없습니다")
		}
		h.logger.Error("역할 삭제 실패", "role", name, "error", err)
		return api.ErrInternalServer.WithMessage("역할 삭제 실패")
	}
	h.invalidate(name)

	h.logger.Info("역할 삭제", "role", name, "actor", ctx.UserID())
	return ctx.NoContent(http.StatusNoContent)
}

// Catalog 는 권한 키 카탈로그를 반환한다 (인증만 요구).
// GET /api/v1/permissions
func (h *RoleHandler) Catalog(ctx api.Context) error {
	return ctx.JSON(http.StatusOK, dto.NewSuccessResponse(dto.PermissionCatalogResponse{
		Permissions: rbac.Permissions(),
	}))
}

// invalidate 는 단일 역할의 권한 캐시를 무효화한다 (주입되지 않았으면 no-op).
func (h *RoleHandler) invalidate(role string) {
	if h.perms != nil {
		h.perms.Invalidate(role)
	}
}

// invalidateAll 은 권한 캐시 전체를 무효화한다 (주입되지 않았으면 no-op).
func (h *RoleHandler) invalidateAll() {
	if h.perms != nil {
		h.perms.InvalidateAll()
	}
}

// firstInvalidPermission 은 카탈로그에 없는 첫 권한 키를 반환한다. 모두 유효하면 "".
func firstInvalidPermission(permissions []string) string {
	for _, p := range permissions {
		if !rbac.IsValidPermission(p) {
			return p
		}
	}
	return ""
}

// toRoleResponse 는 storage.RoleRow 를 응답 DTO 로 변환한다.
// 권한이 없는 역할도 null 이 아닌 빈 배열로 직렬화한다 (클라이언트 분기 단순화).
func toRoleResponse(r storage.RoleRow) dto.RoleResponse {
	perms := r.Permissions
	if perms == nil {
		perms = []string{}
	}
	return dto.RoleResponse{
		Name:        r.Name,
		Description: r.Description,
		Builtin:     r.Builtin,
		Permissions: perms,
		CreatedAt:   r.CreatedAt,
		UpdatedAt:   r.UpdatedAt,
	}
}
