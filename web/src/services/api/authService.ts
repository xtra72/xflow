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
 * 현재 인증된 사용자 프로필을 조회한다.
 */
export async function getCurrentUser(): Promise<User> {
  return get<User>('/auth/me');
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
