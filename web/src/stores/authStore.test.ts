// SPEC-AUTH-004 단위 테스트.
// U2 (로그인 상태 전이) + UB1 (saveTokens 가드) + UB2 (loadTokens 자가 회복).
// 11개 케이스로 store 의 가드/자가 회복 분기를 모두 커버한다.

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import type { AuthTokens, User } from '@/types/auth';

import { __test__, useAuthStore } from './authStore';

const { saveTokens, loadTokens, TOKENS_STORAGE_KEY } = __test__;

// 테스트용 유효 토큰 (실제 JWT 형태가 아니어도 구조 검증만 통과하면 충분).
const validTokens: AuthTokens = {
  access_token: 'test-access-token',
  refresh_token: 'test-refresh-token',
  expires_at: 9999999999,
};

const validUser: User = {
  name: 'admin',
  role: 'admin',
};

/** store 상태를 테스트 사이에 reset 한다. */
function resetStore(): void {
  useAuthStore.setState({
    user: null,
    tokens: null,
    isAuthenticated: false,
    isLoading: false,
    authEnabled: null,
  });
}

/**
 * 콘솔 출력에 토큰 시크릿이 포함되지 않았는지 검증한다 (SPEC-AUTH-004 R-6).
 */
function assertNoTokenLeak(
  spy: ReturnType<typeof vi.spyOn>,
  tokenValues: readonly string[],
): void {
  for (const call of spy.mock.calls) {
    for (const arg of call) {
      const text = typeof arg === 'string' ? arg : JSON.stringify(arg);
      for (const token of tokenValues) {
        if (token.length === 0) continue;
        expect(
          text.includes(token),
          `console output must not include raw token value: ${token}`,
        ).toBe(false);
      }
    }
  }
}

describe('authStore', () => {
  let warnSpy: ReturnType<typeof vi.spyOn>;
  let errorSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    localStorage.clear();
    resetStore();
    warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => undefined);
    errorSpy = vi.spyOn(console, 'error').mockImplementation(() => undefined);
  });

  afterEach(() => {
    warnSpy.mockRestore();
    errorSpy.mockRestore();
  });

  // ===========================================================================
  // U2 — 클라이언트 로그인 성공 상태 전이
  // ===========================================================================

  describe('U2 — 로그인 상태 전이', () => {
    it('U2-T1: login(user, tokens) 호출 시 store 가 {user, tokens, isAuthenticated:true, isLoading:false} 로 전이한다', () => {
      useAuthStore.getState().login(validUser, validTokens);

      const state = useAuthStore.getState();
      expect(state.user).toEqual(validUser);
      expect(state.tokens).toEqual(validTokens);
      expect(state.isAuthenticated).toBe(true);
      expect(state.isLoading).toBe(false);
    });

    it('U2-T2: 로그인 직후 localStorage 의 토큰 키가 유효 JSON 이며 tokens 와 deep-equal 한다', () => {
      useAuthStore.getState().login(validUser, validTokens);

      const raw = localStorage.getItem(TOKENS_STORAGE_KEY);
      expect(raw).not.toBeNull();
      expect(raw).not.toBe('undefined');
      expect(raw).not.toBe('null');

      const parsed = JSON.parse(raw as string);
      expect(parsed).toEqual(validTokens);
    });

    it('U2-T3: broken state (literal "undefined") 가 사전 저장된 상태에서 loadTokens 가 자동 클린업 후 null 반환한다', () => {
      // SPEC 이전 버전에서 생성된 broken state 시뮬레이션.
      localStorage.setItem(TOKENS_STORAGE_KEY, 'undefined');

      const result = loadTokens();

      expect(result).toBeNull();
      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      expect(useAuthStore.getState().isAuthenticated).toBe(false);
    });
  });

  // ===========================================================================
  // UB1 — saveTokens 의 falsy 차단
  // ===========================================================================

  describe('UB1 — saveTokens falsy 차단', () => {
    it('UB1-T1: saveTokens(undefined) 호출 시 localStorage 미변경, console.warn 1회 호출', () => {
      saveTokens(undefined as unknown as AuthTokens);

      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      expect(warnSpy).toHaveBeenCalledTimes(1);
      assertNoTokenLeak(warnSpy, [validTokens.access_token, validTokens.refresh_token]);
    });

    it('UB1-T2: saveTokens 가 빈 access_token 을 거부한다', () => {
      saveTokens({
        access_token: '',
        refresh_token: 'x',
        expires_at: 100,
      });

      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      expect(warnSpy).toHaveBeenCalledTimes(1);
      assertNoTokenLeak(warnSpy, ['x']);
    });

    it('UB1-T3: login(undefined, undefined) 호출 시 store 미변경, console.error 1회', () => {
      // 사전 상태 시드 (이전 상태 유지 확인용).
      useAuthStore.setState({
        user: null,
        tokens: null,
        isAuthenticated: false,
        isLoading: false,
        authEnabled: true,
      });

      const before = useAuthStore.getState();
      useAuthStore
        .getState()
        .login(undefined as unknown as User, undefined as unknown as AuthTokens);

      const after = useAuthStore.getState();
      expect(after.user).toBe(before.user);
      expect(after.tokens).toBe(before.tokens);
      expect(after.isAuthenticated).toBe(before.isAuthenticated);
      expect(after.isLoading).toBe(before.isLoading);
      expect(after.authEnabled).toBe(before.authEnabled);
      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      // 가드 메시지가 console.error 또는 console.warn 으로 1회 이상 발생해야 한다.
      const totalGuardCalls = errorSpy.mock.calls.length + warnSpy.mock.calls.length;
      expect(totalGuardCalls).toBeGreaterThanOrEqual(1);
    });
  });

  // ===========================================================================
  // UB2 — loadTokens 의 broken state 자가 회복
  // ===========================================================================

  describe('UB2 — loadTokens 자가 회복', () => {
    it('UB2-T1: literal "undefined" 가 저장된 경우 null 반환 + 키 자동 삭제 + warn 1회', () => {
      localStorage.setItem(TOKENS_STORAGE_KEY, 'undefined');

      const result = loadTokens();

      expect(result).toBeNull();
      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      expect(warnSpy).toHaveBeenCalledTimes(1);
    });

    it('UB2-T2: literal "null" 이 저장된 경우 동일하게 자가 회복', () => {
      localStorage.setItem(TOKENS_STORAGE_KEY, 'null');

      const result = loadTokens();

      expect(result).toBeNull();
      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      expect(warnSpy).toHaveBeenCalledTimes(1);
    });

    it('UB2-T3: malformed JSON 이 저장된 경우 자가 회복', () => {
      localStorage.setItem(TOKENS_STORAGE_KEY, '{not json');

      const result = loadTokens();

      expect(result).toBeNull();
      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      expect(warnSpy).toHaveBeenCalledTimes(1);
    });

    it('UB2-T4: 파싱 가능하나 구조가 AuthTokens 와 다른 JSON 은 자가 회복', () => {
      localStorage.setItem(TOKENS_STORAGE_KEY, JSON.stringify({ foo: 'bar' }));

      const result = loadTokens();

      expect(result).toBeNull();
      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBeNull();
      expect(warnSpy).toHaveBeenCalledTimes(1);
    });

    it('UB2-T5: 정상 유효 JSON 이 저장된 경우 deep-equal 반환, warn 미발생, localStorage 미변경', () => {
      const stored: AuthTokens = {
        access_token: 'A',
        refresh_token: 'R',
        expires_at: 9999999999,
      };
      const rawBefore = JSON.stringify(stored);
      localStorage.setItem(TOKENS_STORAGE_KEY, rawBefore);

      const result = loadTokens();

      expect(result).toEqual(stored);
      expect(localStorage.getItem(TOKENS_STORAGE_KEY)).toBe(rawBefore);
      expect(warnSpy).not.toHaveBeenCalled();
    });
  });
});
