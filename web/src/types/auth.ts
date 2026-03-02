// Authentication and authorization types for the web dashboard.

/**
 * User role levels for access control.
 */
export type UserRole = 'admin' | 'editor' | 'viewer';

/**
 * Authenticated user information.
 */
export interface User {
  id: string;
  name: string;
  email: string;
  role: UserRole;
}

/**
 * JWT token pair with expiration.
 */
export interface AuthTokens {
  access_token: string;
  refresh_token: string;
  expires_at: number;
}

/**
 * Login request payload.
 */
export interface LoginRequest {
  email: string;
  password: string;
}

/**
 * Login response containing user info and tokens.
 */
export interface LoginResponse {
  user: User;
  tokens: AuthTokens;
}
