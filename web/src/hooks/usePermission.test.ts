// usePermission 단위 테스트 (SPEC-AUTH-006 M1.4 — AC-01 / AC-02).
//
// 범위: 폴백 규칙(인증 비활성 · 구버전 서버 · 조회 실패)과 집합 조회 판정,
//       로그인 경로와 세션 복원 경로에서 동일한 결과가 나오는지.
//
// 스토어를 모킹하지 않고 실제 authStore 를 구동한다. 판정은 스토어 상태에서
// 파생되므로, 스토어를 우회하면 "로그인/복원 후에도 동일" 을 검증할 수 없다.

import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { useAuthStore } from '@/stores/authStore';
import type { AuthTokens, User } from '@/types/auth';

import { hasPermissionOf, usePermission } from './usePermission';

// ---- authService mock (로그인/세션 복원 경로 구동용) ----
const getAuthStatusMock = vi.hoisted(() => vi.fn());
const getCurrentUserMock = vi.hoisted(() => vi.fn());
vi.mock('@/services/api/authService', () => ({
  getAuthStatus: getAuthStatusMock,
  getCurrentUser: getCurrentUserMock,
}));

const TOKENS: AuthTokens = {
  access_token: 'access-token',
  refresh_token: 'refresh-token',
  expires_at: Date.now() + 3_600_000,
};

/** AC-01 이 지정한 권한 3종. */
const THREE_PERMISSIONS = ['agent.read', 'agent.execute', 'device.read'];

function makeUser(overrides: Partial<User> = {}): User {
  return { name: 'alice', role: 'operator', ...overrides };
}

/** 스토어를 초기 상태로 되돌린다 (zustand 스토어는 모듈 수명 동안 공유된다). */
function resetStore() {
  useAuthStore.setState({
    user: null,
    tokens: null,
    isAuthenticated: false,
    isLoading: false,
    authEnabled: null,
    permissions: new Set<string>(),
    permissionStatus: 'unknown',
  });
}

beforeEach(() => {
  localStorage.clear();
  getAuthStatusMock.mockReset();
  getCurrentUserMock.mockReset();
  resetStore();
});

// ─────────────────────────────────────────────────────────────────────
// AC-01 — 권한 컨텍스트 적재
// ─────────────────────────────────────────────────────────────────────

describe('AC-01 — 권한 컨텍스트 적재', () => {
  it('로그인 시 /auth/me 의 permissions 3종이 스토어 집합과 일치한다', async () => {
    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      // 로그인 응답에는 permissions 가 없다 — /auth/me 가 채운다.
      useAuthStore.getState().login(makeUser(), TOKENS);
      useAuthStore
        .getState()
        .setUser(makeUser({ permissions: THREE_PERMISSIONS }));
    });

    expect([...useAuthStore.getState().permissions].sort()).toEqual(
      [...THREE_PERMISSIONS].sort(),
    );
    expect(useAuthStore.getState().permissionStatus).toBe('loaded');
  });

  it('보유 권한은 true, 미보유 권한은 false 를 반환한다', async () => {
    const { result } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      useAuthStore
        .getState()
        .setUser(makeUser({ permissions: THREE_PERMISSIONS }));
    });

    expect(result.current.hasPermission('agent.read')).toBe(true);
    expect(result.current.hasPermission('agent.delete')).toBe(false);
  });

  it('세션 복원(initialize) 후에도 동일한 판정이 유지된다', async () => {
    // 새로고침 상황 재현: localStorage 에 토큰이 남아 있고 /auth/me 가 권한을 준다.
    localStorage.setItem('xflow_auth_tokens', JSON.stringify(TOKENS));
    getAuthStatusMock.mockResolvedValue({ auth_enabled: true });
    getCurrentUserMock.mockResolvedValue(
      makeUser({ permissions: THREE_PERMISSIONS }),
    );

    const { result } = renderHook(() => usePermission());

    await act(async () => {
      await useAuthStore.getState().initialize();
    });

    expect(useAuthStore.getState().isAuthenticated).toBe(true);
    expect([...useAuthStore.getState().permissions].sort()).toEqual(
      [...THREE_PERMISSIONS].sort(),
    );
    expect(result.current.hasPermission('agent.read')).toBe(true);
    expect(result.current.hasPermission('agent.delete')).toBe(false);
  });

  it('세션 복원 시 user.name 이 서버의 username 으로 채워진다', async () => {
    // getCurrentUser 가 도메인 타입(User)을 돌려주므로 name 이 살아 있어야 한다.
    localStorage.setItem('xflow_auth_tokens', JSON.stringify(TOKENS));
    getAuthStatusMock.mockResolvedValue({ auth_enabled: true });
    getCurrentUserMock.mockResolvedValue(
      makeUser({ name: 'alice', permissions: THREE_PERMISSIONS }),
    );

    await act(async () => {
      await useAuthStore.getState().initialize();
    });

    expect(useAuthStore.getState().user?.name).toBe('alice');
  });

  it('권한 0개 사용자는 모든 판정이 false 다', async () => {
    const { result } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      useAuthStore.getState().setUser(makeUser({ permissions: [] }));
    });

    expect(result.current.hasPermission('agent.read')).toBe(false);
    expect(result.current.hasAnyPermission(['agent.read', 'flow.read'])).toBe(
      false,
    );
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-02 — 폴백 규칙
// ─────────────────────────────────────────────────────────────────────

describe('AC-02 — 폴백 규칙', () => {
  it('인증 비활성이면 임의의 권한이 항상 true 다', async () => {
    const { result } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(false);
    });

    expect(result.current.hasPermission('agent.read')).toBe(true);
    expect(result.current.hasPermission('user.delete')).toBe(true);
    expect(result.current.hasPermission('아무거나')).toBe(true);
  });

  it('인증 비활성이면 권한 집합이 비어 있어도 true 다', async () => {
    const { result } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(false);
      useAuthStore.getState().setUser(makeUser({ permissions: [] }));
    });

    expect(result.current.hasPermission('agent.read')).toBe(true);
  });

  it('permissions 필드가 없는 구버전 서버 응답이면 항상 true 다', async () => {
    const { result } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      // permissions 미포함 — 구버전 서버.
      useAuthStore.getState().setUser(makeUser());
    });

    expect(useAuthStore.getState().permissionStatus).toBe('absent');
    expect(result.current.hasPermission('agent.read')).toBe(true);
    expect(result.current.hasPermission('user.delete')).toBe(true);
  });

  it('권한 미확정 상태(초기)에서는 전원 허용으로 폴백한다', () => {
    const { result } = renderHook(() => usePermission());

    expect(useAuthStore.getState().permissionStatus).toBe('unknown');
    expect(result.current.hasPermission('agent.read')).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 엣지 케이스 — 조회 실패는 폴백하지 않는다 (구버전 서버와 구분)
// ─────────────────────────────────────────────────────────────────────

describe('권한 조회 실패', () => {
  it('조회 실패는 전원 허용으로 폴백하지 않고 권한 없음으로 처리한다', async () => {
    const { result } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      useAuthStore.getState().setPermissionsError();
    });

    expect(result.current.hasPermission('agent.read')).toBe(false);
    expect(result.current.hasAnyPermission(['agent.read', 'flow.read'])).toBe(
      false,
    );
  });

  it('조회 실패는 재시도 안내를 위해 구버전 서버 폴백과 구분된다', async () => {
    const { result, rerender } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      useAuthStore.getState().setUser(makeUser()); // 구버전 서버
    });
    rerender();
    expect(result.current.isPermissionUnavailable).toBe(false);

    await act(async () => {
      useAuthStore.getState().setPermissionsError();
    });
    rerender();
    expect(result.current.isPermissionUnavailable).toBe(true);
  });

  it('인증 비활성 상태에서는 조회 실패여도 재시도 안내를 띄우지 않는다', async () => {
    const { result } = renderHook(() => usePermission());

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(false);
      useAuthStore.getState().setPermissionsError();
    });

    expect(result.current.isPermissionUnavailable).toBe(false);
    expect(result.current.hasPermission('agent.read')).toBe(true);
  });

  it('세션 복원이 실패하면 권한 상태가 error 가 된다', async () => {
    localStorage.setItem('xflow_auth_tokens', JSON.stringify(TOKENS));
    getAuthStatusMock.mockResolvedValue({ auth_enabled: true });
    getCurrentUserMock.mockRejectedValue(new Error('401'));

    await act(async () => {
      await useAuthStore.getState().initialize();
    });

    expect(useAuthStore.getState().permissionStatus).toBe('error');
    expect(hasPermissionOf('agent.read')).toBe(false);
  });
});

// ─────────────────────────────────────────────────────────────────────
// hasAnyPermission / 비-React 판정
// ─────────────────────────────────────────────────────────────────────

describe('hasAnyPermission', () => {
  beforeEach(async () => {
    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      useAuthStore
        .getState()
        .setUser(makeUser({ permissions: THREE_PERMISSIONS }));
    });
  });

  it('하나라도 보유하면 true 다', () => {
    const { result } = renderHook(() => usePermission());
    expect(
      result.current.hasAnyPermission(['flow.read', 'device.read']),
    ).toBe(true);
  });

  it('전부 미보유면 false 다', () => {
    const { result } = renderHook(() => usePermission());
    expect(result.current.hasAnyPermission(['flow.read', 'user.read'])).toBe(
      false,
    );
  });

  it('빈 배열은 false 다', () => {
    const { result } = renderHook(() => usePermission());
    expect(result.current.hasAnyPermission([])).toBe(false);
  });
});

describe('hasPermissionOf (비-React 경로)', () => {
  it('훅과 동일한 폴백 규칙을 적용한다', async () => {
    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      useAuthStore
        .getState()
        .setUser(makeUser({ permissions: THREE_PERMISSIONS }));
    });

    expect(hasPermissionOf('device.read')).toBe(true);
    expect(hasPermissionOf('device.delete')).toBe(false);

    await act(async () => {
      useAuthStore.getState().setAuthEnabled(false);
    });
    expect(hasPermissionOf('device.delete')).toBe(true);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 로그아웃
// ─────────────────────────────────────────────────────────────────────

describe('로그아웃', () => {
  it('로그아웃하면 권한 집합과 상태가 초기화된다', async () => {
    await act(async () => {
      useAuthStore.getState().setAuthEnabled(true);
      useAuthStore.getState().login(makeUser(), TOKENS);
      useAuthStore
        .getState()
        .setUser(makeUser({ permissions: THREE_PERMISSIONS }));
    });
    expect(useAuthStore.getState().permissions.size).toBe(3);

    await act(async () => {
      useAuthStore.getState().logout();
    });

    expect(useAuthStore.getState().permissions.size).toBe(0);
    expect(useAuthStore.getState().permissionStatus).toBe('unknown');
  });
});

// ─────────────────────────────────────────────────────────────────────
// 메뉴 축 (SPEC-AUTH-006 E2)
//
// 메뉴 노출과 데이터 읽기를 분리한 이유는 "대시보드만 보이는 역할" 때문이다.
// 대시보드 패널이 에이전트·장치·플로우를 읽으므로 read 는 줘야 하는데, 같은 키로
// 메뉴까지 판정하면 그 역할을 만들 수 없다.
// ─────────────────────────────────────────────────────────────────────

/** 스토어를 지정한 권한 상태로 세팅한다. */
function seedPermissions(perms: string[], authEnabled: boolean | null = true) {
  useAuthStore.setState({
    user: makeUser(),
    tokens: TOKENS,
    isAuthenticated: true,
    isLoading: false,
    authEnabled,
    permissions: new Set(perms),
    permissionStatus: 'loaded',
  });
}

describe('usePermission — 메뉴 축', () => {
  it('nav.* 보유 역할은 메뉴 축으로 판정한다 — read 가 있어도 nav 가 없으면 숨긴다', () => {
    seedPermissions([
      'agent.read', 'flow.read', 'device.read', 'dashboard.read',
      'nav.monitoring',
    ]);
    const { result } = renderHook(() => usePermission());

    // 데이터는 읽을 수 있어야 대시보드 패널이 동작한다.
    expect(result.current.hasPermission('agent.read')).toBe(true);
    // 그러나 nav.agent 가 없으므로 에이전트 메뉴는 숨긴다.
    expect(result.current.canSeeMenu('nav.agent')).toBe(false);
    expect(result.current.canSeeMenu('nav.monitoring')).toBe(true);
  });

  it('nav.* 를 전부 끄면 메뉴가 실제로 전부 숨는다 — 데이터 read 가 있어도', () => {
    // 이것이 "대시보드 전용 역할" 의 정의다. 키 개수로 축 사용 여부를 추론하면
    // 이 상태가 "이관 이전 역할" 로 오인되어 메뉴가 도로 보인다.
    seedPermissions(['agent.read', 'flow.read', 'device.read', 'dashboard.read']);
    const { result } = renderHook(() => usePermission());

    expect(result.current.hasPermission('agent.read')).toBe(true);
    expect(result.current.canSeeMenu('nav.agent')).toBe(false);
    expect(result.current.canSeeMenu('nav.flow')).toBe(false);
  });

  it('인증 비활성이면 메뉴 판정도 전원 허용이다', () => {
    seedPermissions([], false);
    const { result } = renderHook(() => usePermission());
    expect(result.current.canSeeMenu('nav.agent')).toBe(true);
  });
});
