import type {
  AuthStatusResponse,
  AuthTokens,
  ChangePasswordRequest,
  LoginRequest,
  LoginResponse,
  User,
} from '@/types/auth';

import { get, post, put } from './client';

/**
 * 사용자 자격 증명으로 인증한다.
 */
export async function login(req: LoginRequest): Promise<LoginResponse> {
  return post<LoginResponse>('/auth/login', req);
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
