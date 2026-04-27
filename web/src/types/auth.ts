// 웹 대시보드의 인증 및 권한 타입 정의.

/**
 * 접근 제어를 위한 사용자 역할.
 */
export type UserRole = 'admin' | 'editor' | 'viewer';

/**
 * 인증된 사용자 정보.
 * Basic Auth 방식이므로 email 필드 없이 name/role만 사용한다.
 */
export interface User {
  name: string;
  role: UserRole;
}

/**
 * JWT 토큰 쌍과 만료 시간.
 */
export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  expires_at: number;
}

/**
 * 로그인 요청 페이로드.
 */
export interface LoginRequest {
  username: string;
  password: string;
}

/**
 * 로그인 응답 — 사용자 정보와 토큰 포함.
 */
export interface LoginResponse {
  user: User;
  tokens: AuthTokens;
}

/**
 * 비밀번호 변경 요청 페이로드.
 */
export interface ChangePasswordRequest {
  current_password: string;
  new_password: string;
}

/**
 * 인증 상태 응답 — 서버에서 인증이 활성화되었는지 여부.
 */
export interface AuthStatusResponse {
  auth_enabled: boolean;
}
