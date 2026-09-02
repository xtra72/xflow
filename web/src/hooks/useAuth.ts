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
  const setUser = useAuthStore((s) => s.setUser);
  const setPermissionsError = useAuthStore((s) => s.setPermissionsError);
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

    // SPEC-AUTH-006 U1: 로그인 응답에는 permissions 가 없다(서버는 /auth/me 에서만
    // 제공한다). 토큰이 스토어에 들어간 뒤 이어서 조회해 권한 집합을 적재한다.
    // 조회 실패는 로그인 자체를 되돌리지 않는다 — 인증은 이미 성공했고, 권한만
    // '조회 실패'로 남겨 폴백 없이 권한 없음으로 처리한다.
    try {
      setUser(await authService.getCurrentUser());
    } catch {
      setPermissionsError();
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
