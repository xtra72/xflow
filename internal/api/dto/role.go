package dto

// @SPEC:SPEC-AUTH-005 (M6) — 역할 관리 API DTO (spec.md §2.1, §2.2).

// RoleResponse 는 역할 목록·단건 응답 DTO 이다.
type RoleResponse struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Builtin     bool     `json:"builtin"`
	Permissions []string `json:"permissions"`
	CreatedAt   int64    `json:"created_at"` // epoch ms
	UpdatedAt   int64    `json:"updated_at"` // epoch ms
}

// CreateRoleRequest 는 역할 생성 요청 DTO 이다 (POST /roles).
type CreateRoleRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// UpdateRoleRequest 는 역할 수정 요청 DTO 이다 (PUT /roles/{name}).
//
// 두 필드 모두 선택적이다 (포인터). Bind 는 DisallowUnknownFields 이므로 생략된
// 필드는 nil 로 남고 해당 항목은 변경되지 않는다.
//   - Name        : 역할 이름 변경. 해당 역할을 쓰던 users.role 도 함께 갱신된다.
//   - Permissions : 권한 집합 전체 교체.
type UpdateRoleRequest struct {
	Name        *string   `json:"name,omitempty"`
	Permissions *[]string `json:"permissions,omitempty"`
}

// PermissionCatalogResponse 는 권한 키 카탈로그 응답 DTO 이다
// (GET /permissions — 인증만 요구).
type PermissionCatalogResponse struct {
	Permissions []string `json:"permissions"`
}
