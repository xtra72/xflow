import type {
  AuthStatusResponse,
  AuthTokens,
  ChangePasswordRequest,
  LoginRequest,
  LoginResponse,
  User,
  UserRole,
} from '@/types/auth';

import { get, post, put } from './client';

/**
 * 서버 측 LoginResponse 의 원본 형태.
 *
 * SPEC-AUTH-004 U1: 서버는 envelope 언래핑 후 다음 구조로 응답한다.
 * - user.username → 클라이언트 User.name 으로 매핑
 * - user.role → 클라이언트 User.role (UserRole) 로 매핑
 * - tokens 의 access_token / refresh_token / expires_at 은 동일 키로 직접 전달
 * - tokens.token_type 은 클라이언트가 보관하지 않으므로 무시
 */
interface ServerLoginResponse {
  user: {
    username: string;
    role: string;
  };
  tokens: {
    access_token: string;
    refresh_token: string;
    expires_at: number;
    token_type: string;
  };
}

/**
 * 사용자 자격 증명으로 인증한다.
 *
 * SPEC-AUTH-004 U2: 서버 응답 {user: {username, role}, tokens: {...}} 을
 * 클라이언트 도메인 타입 LoginResponse {user: User, tokens: AuthTokens} 으로
 * 한 곳에서 변환한다. 호출자(useAuth.login) 는 도메인 타입만 받는다.
 */
export async function login(req: LoginRequest): Promise<LoginResponse> {
  const serverData = await post<ServerLoginResponse>('/auth/login', req);

  const user: User = {
    name: serverData.user.username,
    role: serverData.user.role as UserRole,
  };
  const tokens: AuthTokens = {
    access_token: serverData.tokens.access_token,
    refresh_token: serverData.tokens.refresh_token,
    expires_at: serverData.tokens.expires_at,
  };

  return { user, tokens };
}

/**
 * 현재 세션을 서버에서 무효화한다.
 */
export async function logout(): Promise<void> {
  await post<void>('/auth/logout');
}

/**
 * 리프레시 토큰으로 새 액세스/리프레시 토큰 쌍을 발급받는다.
 */
export async function refreshToken(token: string): Promise<AuthTokens> {
  return post<AuthTokens>('/auth/refresh', { refresh_token: token });
}

/**
 * 서버 측 `GET /auth/me` 응답의 원본 형태.
 *
 * SPEC-AUTH-005 M6: 기존 필드(username, role)의 위치·형식을 그대로 유지하고
 * permissions 배열만 뒤에 덧붙인다. permissions 를 제공하지 않는 구버전 서버
 * 에서는 필드 자체가 없다 (하위 호환 폴백 대상).
 */
interface ServerMeResponse {
  username: string;
  role: string;
  permissions?: string[];
}

/**
 * 현재 인증된 사용자 프로필을 조회한다.
 *
 * SPEC-AUTH-006 M1.3: 이전 구현은 `get<User>('/auth/me')` 로 서버 응답을 그대로
 * 단언했다. 서버는 `username` 을 보내는데 클라이언트 도메인 타입은 `name` 이고
 * client.ts 의 get 은 키를 변환하지 않으므로, 세션 복원 경로에서 `user.name` 이
 * undefined 가 되어 헤더 사용자명·설정 계정 패널·`useDashboardSync` 의 `mine`
 * 스코프 owner 가 함께 깨졌다. login() 과 동일하게 username → name 으로 매핑한다.
 */
export async function getCurrentUser(): Promise<User> {
  const data = await get<ServerMeResponse>('/auth/me');

  return {
    name: data.username,
    role: data.role as UserRole,
    // 구버전 서버에서는 undefined 로 남는다 — 폴백 판단의 입력이다.
    permissions: data.permissions,
  };
}

/**
 * 비밀번호를 변경한다. Bearer 토큰 필요.
 */
export async function changePassword(req: ChangePasswordRequest): Promise<void> {
  await put<void>('/auth/password', req);
}

/**
 * 서버의 인증 활성화 상태를 조회한다. 인증 불필요.
 */
export async function getAuthStatus(): Promise<AuthStatusResponse> {
  return get<AuthStatusResponse>('/auth/status');
}
