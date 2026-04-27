// 인증 상태 관리.
// 토큰은 페이지 새로고침 시에도 세션이 유지되도록 localStorage에 저장한다.

import { create } from 'zustand';

import * as authService from '@/services/api/authService';
import type { AuthTokens, User } from '@/types/auth';

/** localStorage 토큰 저장 키 */
const TOKENS_STORAGE_KEY = 'xflow_auth_tokens';

interface AuthState {
  user: User | null;
  tokens: AuthTokens | null;
  isAuthenticated: boolean;
  isLoading: boolean;
  /** 서버 인증 활성화 여부. null = 아직 확인 전 */
  authEnabled: boolean | null;
}

interface AuthActions {
  login: (user: User, tokens: AuthTokens) => void;
  logout: () => void;
  setTokens: (tokens: AuthTokens) => void;
  setLoading: (loading: boolean) => void;
  setAuthEnabled: (enabled: boolean) => void;
  /** 앱 시작 시 인증 상태 확인 및 토큰 복원 */
  initialize: () => Promise<void>;
}

/** localStorage에서 토큰을 읽는다. */
function loadTokens(): AuthTokens | null {
  try {
    const raw = localStorage.getItem(TOKENS_STORAGE_KEY);
    if (!raw) return null;
    return JSON.parse(raw) as AuthTokens;
  } catch {
    return null;
  }
}

/** localStorage에 토큰을 저장한다. */
function saveTokens(tokens: AuthTokens): void {
  try {
    localStorage.setItem(TOKENS_STORAGE_KEY, JSON.stringify(tokens));
  } catch {
    // localStorage 사용 불가 시 무시
  }
}

/** localStorage에서 토큰을 삭제한다. */
function clearTokens(): void {
  try {
    localStorage.removeItem(TOKENS_STORAGE_KEY);
  } catch {
    // localStorage 사용 불가 시 무시
  }
}

export const useAuthStore = create<AuthState & AuthActions>()((set, get) => ({
  // 상태
  user: null,
  tokens: null,
  isAuthenticated: false,
  isLoading: false,
  authEnabled: null,

  // 액션
  login: (user, tokens) => {
    saveTokens(tokens);
    set({
      user,
      tokens,
      isAuthenticated: true,
      isLoading: false,
    });
  },

  logout: () => {
    clearTokens();
    set({
      user: null,
      tokens: null,
      isAuthenticated: false,
      isLoading: false,
    });
  },

  setTokens: (tokens) => {
    saveTokens(tokens);
    set({ tokens });
  },

  setLoading: (loading) =>
    set({ isLoading: loading }),

  setAuthEnabled: (enabled) =>
    set({ authEnabled: enabled }),

  initialize: async () => {
    const state = get();
    // 이미 초기화됨 — 중복 호출 방지
    if (state.authEnabled !== null) return;

    set({ isLoading: true });

    try {
      // 1. 서버 인증 활성화 여부 확인
      const status = await authService.getAuthStatus();
      set({ authEnabled: status.auth_enabled });

      if (!status.auth_enabled) {
        // 인증 비활성화 — 모든 접근 허용
        set({ isLoading: false });
        return;
      }

      // 2. localStorage에서 기존 토큰 복원 시도
      const stored = loadTokens();
      if (!stored) {
        set({ isLoading: false });
        return;
      }

      // 3. 토큰 유효성 검증 (서버 호출)
      set({ tokens: stored });
      const user = await authService.getCurrentUser();
      set({
        user,
        isAuthenticated: true,
        isLoading: false,
      });
    } catch {
      // 토큰이 만료되었거나 유효하지 않음 — 정리
      clearTokens();
      set({
        user: null,
        tokens: null,
        isAuthenticated: false,
        isLoading: false,
      });
    }
  },
}));

/**
 * 비-React 컨텍스트(axios 인터셉터 등)에서 사용하는
 * 훅 없는 상태 접근자.
 */
export const getAuthState = () => useAuthStore.getState();
