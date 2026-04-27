package dto

// LoginRequest 는 로그인 요청 DTO이다.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse 는 로그인 응답 DTO이다.
type LoginResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

// RefreshRequest 는 토큰 갱신 요청 DTO이다.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// RefreshResponse 는 토큰 갱신 응답 DTO이다.
type RefreshResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

// ChangePasswordRequest 는 비밀번호 변경 요청 DTO이다.
type ChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UserInfoResponse 는 현재 사용자 정보 응답 DTO이다.
type UserInfoResponse struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}
