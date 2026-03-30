// 인증 훅 — authStore와 authService를 래핑한다.

import * as authService from '@/services/api/authService';
import { useAuthStore } from '@/stores/authStore';

/**
 * 인증 상태와 액션을 제공한다.
 * login/logout은 API 호출과 로컬 스토어 업데이트를 함께 수행한다.
 * 에러는 호출자가 처리할 수 있도록 재throw한다 (예: 토스트 표시).
 */
export function useAuth() {
  const user = useAuthStore((s) => s.user);
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated);
  const isLoading = useAuthStore((s) => s.isLoading);
  const authEnabled = useAuthStore((s) => s.authEnabled);
  const storeLogin = useAuthStore((s) => s.login);
  const storeLogout = useAuthStore((s) => s.logout);
  const setLoading = useAuthStore((s) => s.setLoading);
  const initialize = useAuthStore((s) => s.initialize);

  async function login(username: string, password: string): Promise<void> {
    setLoading(true);
    try {
      const response = await authService.login({ username, password });
      storeLogin(response.user, response.tokens);
    } catch (error) {
      setLoading(false);
      throw error;
    }
  }

  async function logout(): Promise<void> {
    try {
      await authService.logout();
    } catch {
      // 서버 로그아웃 실패해도 로컬 상태는 정리
    } finally {
      storeLogout();
    }
  }

  return { user, isAuthenticated, isLoading, authEnabled, login, logout, initialize };
}
