package dto

// LoginRequest 는 로그인 요청 DTO이다.
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// TokenPair 는 JWT 토큰 쌍과 만료 시간을 담는 DTO이다.
// SPEC-AUTH-004 U1: LoginResponse 의 tokens 하위 객체로 사용된다.
type TokenPair struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresAt    int64  `json:"expires_at"`
	TokenType    string `json:"token_type"`
}

// LoginResponse 는 로그인 응답 DTO이다.
// SPEC-AUTH-004 U1: 클라이언트 LoginResponse (web/src/types/auth.ts) 와
// 정확히 일치하도록 {user, tokens} 중첩 구조로 정의한다.
type LoginResponse struct {
	User   UserInfoResponse `json:"user"`
	Tokens TokenPair        `json:"tokens"`
}

// RefreshRequest 는 토큰 갱신 요청 DTO이다.
type RefreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// O1 (SPEC-AUTH-004): LoginResponse 와 의도적으로 다른 flat 구조 — 리프레시는 user 정보를 반복 전송하지 않음.
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
