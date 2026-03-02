import type { AuthTokens, LoginRequest, LoginResponse, User } from '@/types/auth';

import { get, post } from './client';

/**
 * Authenticate user with credentials.
 */
export async function login(req: LoginRequest): Promise<LoginResponse> {
  return post<LoginResponse>('/auth/login', req);
}

/**
 * Invalidate the current session on the server.
 */
export async function logout(): Promise<void> {
  await post<void>('/auth/logout');
}

/**
 * Exchange a refresh token for new access/refresh token pair.
 */
export async function refreshToken(token: string): Promise<AuthTokens> {
  return post<AuthTokens>('/auth/refresh', { refresh_token: token });
}

/**
 * Retrieve the currently authenticated user profile.
 */
export async function getCurrentUser(): Promise<User> {
  return get<User>('/auth/me');
}
