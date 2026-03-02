// Authentication hook wrapping authStore and authService.

import * as authService from '@/services/api/authService';
import { useAuthStore } from '@/stores/authStore';

/**
 * Provides authentication state and actions.
 * Login/logout orchestrate both the API call and local store update.
 * Errors are re-thrown so callers can handle them (e.g. display toast).
 */
export function useAuth() {
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const isLoading = useAuthStore((s) => s.isLoading);
  const storeLogin = useAuthStore((s) => s.login);
  const storeLogout = useAuthStore((s) => s.logout);
  const setLoading = useAuthStore((s) => s.setLoading);

  async function login(email: string, password: string): Promise<void> {
    setLoading(true);
    try {
      const response = await authService.login({ email, password });
      storeLogin(response.user, response.tokens);
    } catch (error) {
      setLoading(false);
      throw error;
    }
  }

  async function logout(): Promise<void> {
    try {
      await authService.logout();
    } finally {
      storeLogout();
    }
  }

  return { user, isAuthenticated, isLoading, login, logout };
}
