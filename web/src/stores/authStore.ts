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

/**
 * AuthTokens 구조 검증 (런타임 가드).
 *
 * SPEC-AUTH-004 UB1/UB2 공통 검증 로직. 다음 조건을 모두 만족할 때만 true 를 반환한다:
 * - 객체이며 null/undefined 아님
 * - access_token / refresh_token 이 비어있지 않은 문자열
 * - expires_at 이 양의 정수
 */
function isValidAuthTokens(value: unknown): value is AuthTokens {
  if (!value || typeof value !== 'object') return false;
  const t = value as Partial<AuthTokens>;
  return (
    typeof t.access_token === 'string' &&
    t.access_token.length > 0 &&
    typeof t.refresh_token === 'string' &&
    t.refresh_token.length > 0 &&
    typeof t.expires_at === 'number' &&
    t.expires_at > 0
  );
}

/**
 * localStorage에서 토큰을 읽는다.
 *
 * SPEC-AUTH-004 UB2: broken state (literal "undefined"/"null", malformed JSON,
 * 구조 불일치 JSON) 가 발견되면 자동으로 해당 키를 삭제 후 null 반환.
 * 토큰 시크릿은 로그에 노출하지 않는다 (R-6).
 */
function loadTokens(): AuthTokens | null {
  // 1단계: localStorage 접근. 예외 시 silent null.
  let raw: string | null;
  try {
    raw = localStorage.getItem(TOKENS_STORAGE_KEY);
  } catch {
    return null;
  }
  if (!raw) return null;

  // 2단계: literal "undefined" / "null" 자가 회복.
  if (raw === 'undefined' || raw === 'null') {
    console.warn('[authStore] loadTokens: literal non-JSON detected, cleaning up');
    try {
      localStorage.removeItem(TOKENS_STORAGE_KEY);
    } catch {
      // ignore
    }
    return null;
  }

  // 3단계: JSON 파싱. 실패 시 자가 회복.
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    console.warn('[authStore] loadTokens: malformed JSON, cleaning up');
    try {
      localStorage.removeItem(TOKENS_STORAGE_KEY);
    } catch {
      // ignore
    }
    return null;
  }

  // 4단계: 구조적 유효성 검증. 실패 시 자가 회복.
  if (!isValidAuthTokens(parsed)) {
    console.warn('[authStore] loadTokens: invalid shape, cleaning up');
    try {
      localStorage.removeItem(TOKENS_STORAGE_KEY);
    } catch {
      // ignore
    }
    return null;
  }

  // 5단계: 정상 케이스. 파싱 결과 반환.
  return parsed;
}

/**
 * localStorage에 토큰을 저장한다.
 *
 * SPEC-AUTH-004 UB1: falsy 또는 구조 불일치 인자에 대해 저장을 거부한다.
 * 토큰 시크릿은 로그에 노출하지 않는다 (R-6).
 */
function saveTokens(tokens: AuthTokens): void {
  // UB1 런타임 가드: TypeScript 시그니처 우회 시에도 falsy / 비정상 구조 차단.
  if (!isValidAuthTokens(tokens)) {
    console.warn('[authStore] saveTokens: invalid tokens, skipping persist');
    return;
  }

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
    // SPEC-AUTH-004 UB1: user 또는 tokens 가 falsy 인 경우 진입 거부.
    // 토큰 시크릿은 로그에 노출하지 않는다 (R-6).
    if (!user || !tokens) {
      console.error('[authStore] login: rejected (user or tokens missing)');
      return;
    }
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

/**
 * SPEC-AUTH-004 단위 테스트 전용 export.
 *
 * 프로덕션 코드에서는 사용하지 말 것. authStore.test.ts 가 saveTokens / loadTokens
 * 의 가드/자가 회복 분기를 직접 검증하기 위해 필요한 test seam.
 * @internal
 */
export const __test__ = {
  saveTokens,
  loadTokens,
  TOKENS_STORAGE_KEY,
};
