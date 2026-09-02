// 웹 대시보드의 인증 및 권한 타입 정의.

/**
 * 사용자 역할 이름.
 *
 * SPEC-AUTH-006 U1: SPEC-AUTH-005 가 역할 CRUD 를 도입해 `operator` 같은 커스텀
 * 역할이 생기므로 3종 고정 유니온에서 문자열로 완화한다. 역할 이름은 더 이상
 * 접근 판정에 쓰이지 않으며(권한 키로 판정한다) 표시 용도로만 남는다.
 */
export type UserRole = string;

/**
 * 인증된 사용자 정보.
 * Basic Auth 방식이므로 email 필드 없이 name/role만 사용한다.
 */
export interface User {
  name: string;
  role: UserRole;
  /**
   * `<resource>.<action>` 형식의 권한 키 목록 (SPEC-AUTH-006 U1).
   *
   * `GET /auth/me` 응답에서만 제공된다. 로그인 응답(`POST /auth/login`)에는
   * 없으며, permissions 를 제공하지 않는 구버전 서버에서도 undefined 이다.
   * undefined 는 "권한 정보 없음"이며 폴백 규칙상 전원 허용으로 처리된다
   * (판정 규칙은 `hooks/usePermission.ts` 한 곳에만 있다).
   */
  permissions?: string[];
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
