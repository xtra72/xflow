package dto

// @SPEC:SPEC-AUTH-005 (M6) — 사용자 관리 API DTO (spec.md §2.2).
//
// 보안: 어떤 응답 DTO 에도 password_hash 필드를 두지 않는다. UserResponse 는
// storage.UserRow 를 그대로 노출하지 않고 필요한 필드만 선별해 담는다.

// UserResponse 는 사용자 목록·단건 응답 DTO 이다.
//
// 비밀번호 해시는 의도적으로 포함하지 않는다 (spec.md §5 비기능 요구사항).
type UserResponse struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt int64  `json:"created_at"` // epoch ms
	UpdatedAt int64  `json:"updated_at"` // epoch ms
}

// CreateUserRequest 는 사용자 등록 요청 DTO 이다 (POST /users).
type CreateUserRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// UpdateUserRequest 는 사용자 역할 변경 요청 DTO 이다 (PUT /users/{username}).
type UpdateUserRequest struct {
	Role string `json:"role"`
}

// ResetPasswordRequest 는 관리자 비밀번호 재설정 요청 DTO 이다
// (PUT /users/{username}/password).
//
// 본인 비밀번호 변경(PUT /auth/password)과 달리 현재 비밀번호를 요구하지 않는다.
type ResetPasswordRequest struct {
	Password string `json:"password"`
}
