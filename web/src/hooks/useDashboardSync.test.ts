// SPEC-DASHBOARD-001 v0.2.0 — useDashboardSync 훅 테스트.
//
// 검증 시나리오:
//   - AC-3 / AC-1: boot 병렬 GET shared+mine 양쪽 200 또는 양쪽 404
//   - AC-16: 404 → 빌트인 기본 snapshot (version=0) 로 메모리 초기화
//   - AC-10: 충돌 시 1회 재PUT (last-write-wins)
//   - AC-4:  shared PUT 403 → 토스트 + 재PUT 안 함
//   - AC-15: localStorage 마이그레이션 1회 + 토스트 1회
//   - 500 ms debounce 동작
//   - 재마운트 시 토스트 미발화 (플래그 영속)
//
// @spec SPEC-DASHBOARD-001 v0.2.0

import { renderHook, act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// ─────────────────────────────────────────────────────────────────────
// Mocks
// ─────────────────────────────────────────────────────────────────────

const getSharedDashboardMock = vi.hoisted(() => vi.fn());
const getMyDashboardMock = vi.hoisted(() => vi.fn());
const putSharedDashboardMock = vi.hoisted(() => vi.fn());
const putMyDashboardMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/dashboardService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/dashboardService')>(
    '@/services/api/dashboardService',
  );
  return {
    ...actual,
    getSharedDashboard: getSharedDashboardMock,
    getMyDashboard: getMyDashboardMock,
    putSharedDashboard: putSharedDashboardMock,
    putMyDashboard: putMyDashboardMock,
  };
});

// ─────────────────────────────────────────────────────────────────────
// LocalStorage / SessionStorage helpers
// ─────────────────────────────────────────────────────────────────────

class InMemoryStorage implements Storage {
  private store: Record<string, string> = {};
  get length(): number {
    return Object.keys(this.store).length;
  }
  clear(): void {
    this.store = {};
  }
  getItem(key: string): string | null {
    return Object.prototype.hasOwnProperty.call(this.store, key) ? this.store[key]! : null;
  }
  key(index: number): string | null {
    return Object.keys(this.store)[index] ?? null;
  }
  removeItem(key: string): void {
    delete this.store[key];
  }
  setItem(key: string, value: string): void {
    this.store[key] = String(value);
  }
}

let testLocalStorage: InMemoryStorage;
let testSessionStorage: InMemoryStorage;

function resetStorage(): void {
  testLocalStorage = new InMemoryStorage();
  testSessionStorage = new InMemoryStorage();
  Object.defineProperty(globalThis, 'localStorage', {
    value: testLocalStorage,
    writable: true,
    configurable: true,
  });
  Object.defineProperty(globalThis, 'sessionStorage', {
    value: testSessionStorage,
    writable: true,
    configurable: true,
  });
}

// ─────────────────────────────────────────────────────────────────────
// Imports under test (after mocks)
// ─────────────────────────────────────────────────────────────────────

import {
  DashboardForbiddenError,
  DashboardUnauthorizedError,
} from '@/services/api/dashboardService';
import { useAuthStore } from '@/stores/authStore';
import {
  consumeMigrationToastFlag,
  runDashboardLocalStorageMigration,
  useUIStore,
} from '@/stores/uiStore';

import { useDashboardSync } from './useDashboardSync';
import type { DashboardSnapshot } from '@/types/dashboard';

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

function makeSnapshot(
  overrides: Partial<DashboardSnapshot> = {},
): DashboardSnapshot {
  return {
    scope: 'global',
    owner: null,
    version: 1,
    updatedAt: 1000,
    payload: {
      dashboardPages: [
        {
          id: 'page-1',
          name: 'P1',
          isDefault: true,
          panels: [],
          layout: [],
        },
      ],
      activeDashboardId: 'page-1',
      dashboardGridCols: 12,
      dashboardShowGridLines: false,
      dashboardRefreshInterval: 10,
      deviceGridLayout: {},
    },
    ...overrides,
  };
}

/**
 * 테스트 사이에 store 상태를 초기화한다. vi.resetModules 는 사용하지 않는다 —
 * 훅이 import 한 store 와 테스트가 import 한 store 의 모듈 인스턴스를 동일하게 유지해야
 * subscribe / setState 가 작동한다.
 *
 * 마이그레이션 모듈 플래그(`migrationToastPending`) 는 모듈 스코프 변수이므로
 * `runDashboardLocalStorageMigration()` 을 다시 호출하여 갱신한다.
 */
function resetStoreState(): void {
  // 이전 테스트에서 set 된 pending 마이그레이션 토스트 플래그를 먼저 소비해 모듈
  // 스코프 상태를 초기화한다.
  consumeMigrationToastFlag();
  // 그런 다음 현재 testLocalStorage 상태에 맞춰 마이그레이션을 재평가한다.
  runDashboardLocalStorageMigration();

  useUIStore.setState((state) => ({
    ...state,
    sharedSnapshot: null,
    mineSnapshot: null,
    activeDashboardScope: 'shared',
    notifications: [],
    dashboardPages: [
      {
        id: 'default',
        name: '대시보드',
        isDefault: true,
        panels: [],
        layout: [],
      },
    ],
    activeDashboardId: 'default',
    dashboardGridCols: 10,
    dashboardShowGridLines: true,
    dashboardRefreshInterval: 10,
    deviceGridLayout: {},
  }));
}

// ─────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────

beforeEach(() => {
  resetStorage();
  getSharedDashboardMock.mockReset();
  getMyDashboardMock.mockReset();
  putSharedDashboardMock.mockReset();
  putMyDashboardMock.mockReset();
  // 인증 store 초기화 — admin 기본.
  useAuthStore.setState({
    user: { name: 'tester', role: 'admin' },
    tokens: null,
    isAuthenticated: true,
    isLoading: false,
    authEnabled: true,
  });
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('useDashboardSync — boot phase', () => {
  it('양쪽 200: store 의 sharedSnapshot/mineSnapshot 모두 채워진다 (AC-3)', async () => {
    const shared = makeSnapshot({ scope: 'global', owner: null, version: 5 });
    const mine = makeSnapshot({ scope: 'user', owner: 'tester', version: 3 });
    getSharedDashboardMock.mockResolvedValue(shared);
    getMyDashboardMock.mockResolvedValue(mine);

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());

    await waitFor(() => expect(result.current.isLoading).toBe(false));

    const state = useUIStore.getState();
    expect(state.sharedSnapshot?.version).toBe(5);
    expect(state.mineSnapshot?.version).toBe(3);
  });

  it('양쪽 404 (null): version=0 의 빌트인 기본 snapshot 으로 메모리 초기화 (AC-1)', async () => {
    getSharedDashboardMock.mockResolvedValue(null);
    getMyDashboardMock.mockResolvedValue(null);

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    const state = useUIStore.getState();
    expect(state.sharedSnapshot).toBeTruthy();
    expect(state.sharedSnapshot?.version).toBe(0);
    expect(state.mineSnapshot).toBeTruthy();
    expect(state.mineSnapshot?.version).toBe(0);

    // 부팅 단계에서는 PUT 이 발생하지 않아야 한다 (사용자 변경 전).
    expect(putSharedDashboardMock).not.toHaveBeenCalled();
    expect(putMyDashboardMock).not.toHaveBeenCalled();
  });

  it('admin 사용자: 활성 스코프 기본값은 shared', async () => {
    getSharedDashboardMock.mockResolvedValue(makeSnapshot());
    getMyDashboardMock.mockResolvedValue(null);

    resetStoreState();
    useAuthStore.setState({ user: { name: 'admin', role: 'admin' } });

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(useUIStore.getState().activeDashboardScope).toBe('shared');
  });

  it('viewer 사용자 + mine 없음: 활성 스코프 기본값은 shared', async () => {
    getSharedDashboardMock.mockResolvedValue(makeSnapshot());
    getMyDashboardMock.mockResolvedValue(null);

    resetStoreState();
    useAuthStore.setState({ user: { name: 'tom', role: 'viewer' } });

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(useUIStore.getState().activeDashboardScope).toBe('shared');
  });

  it('editor 사용자 + mine 존재: 활성 스코프 기본값은 mine', async () => {
    getSharedDashboardMock.mockResolvedValue(null);
    getMyDashboardMock.mockResolvedValue(makeSnapshot({ scope: 'user', version: 2, owner: 'ed' }));

    resetStoreState();
    useAuthStore.setState({ user: { name: 'ed', role: 'editor' } });

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(useUIStore.getState().activeDashboardScope).toBe('mine');
  });

  it('sessionStorage 의 activeScope 가 있으면 그 값을 우선 사용', async () => {
    getSharedDashboardMock.mockResolvedValue(makeSnapshot());
    getMyDashboardMock.mockResolvedValue(makeSnapshot({ scope: 'user', owner: 'a', version: 7 }));

    // sessionStorage 에 mine 으로 기록.
    testSessionStorage.setItem('xflow-ui:active-dashboard-scope', 'mine');

    resetStoreState();
    useAuthStore.setState({ user: { name: 'a', role: 'admin' } });

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(useUIStore.getState().activeDashboardScope).toBe('mine');
  });
});

describe('useDashboardSync — mutation triggers PUT (debounced)', () => {
  it('활성 스코프에서 mutate 하면 500 ms 후 PUT 이 발생한다', async () => {
    vi.useFakeTimers();
    getSharedDashboardMock.mockResolvedValue(makeSnapshot({ version: 1 }));
    getMyDashboardMock.mockResolvedValue(null);
    putSharedDashboardMock.mockResolvedValue(makeSnapshot({ version: 2 }));

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    // boot 완료 대기 (실타이머 마이크로태스크 처리).
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    // mutate.
    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
    });

    expect(putSharedDashboardMock).not.toHaveBeenCalled();
    // 500ms debounce.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(1);
    // If-Match 헤더 검증 — version=1 전달.
    expect(putSharedDashboardMock).toHaveBeenCalledWith(expect.any(Object), 1);
  });
});

describe('useDashboardSync — 409 conflict (AC-10)', () => {
  it('409 발생 후 1회 재PUT 으로 사용자 변경 보존', async () => {
    vi.useFakeTimers();
    const initialShared = makeSnapshot({ version: 5 });
    const serverNewer = makeSnapshot({ version: 6 });
    const finalAccepted = makeSnapshot({ version: 7 });

    getSharedDashboardMock.mockResolvedValue(initialShared);
    getMyDashboardMock.mockResolvedValue(null);

    // 첫 PUT → 409, 두 번째 PUT → 200.
    putSharedDashboardMock
      .mockResolvedValueOnce({ conflict: true, serverSnapshot: serverNewer })
      .mockResolvedValueOnce(finalAccepted);

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    // 사용자 변경 — gridCols 변경 (페이로드가 server snapshot 과 달라짐).
    act(() => {
      useUIStore.getState().setDashboardGridCols(25);
    });
    // debounce 만료 + 첫 PUT(409) → 재PUT(200) 까지 모두 같은 microtask 흐름에서 처리.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      // 추가 마이크로태스크 처리 (재시도 promise 체인).
      await vi.runAllTimersAsync();
    });

    // 1차 PUT (If-Match: 5) + 2차 재PUT (If-Match: 6) 총 2회.
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(2);
    expect(putSharedDashboardMock.mock.calls[0]![1]).toBe(5);
    expect(putSharedDashboardMock.mock.calls[1]![1]).toBe(6);

    // 최종 적용 — version=7.
    expect(useUIStore.getState().sharedSnapshot?.version).toBe(7);
  });

  it('재PUT 도 409 (두 번째 충돌) 이면 server snapshot 으로 강제 동기 + 토스트', async () => {
    vi.useFakeTimers();
    const initialShared = makeSnapshot({ version: 5 });
    const serverV6 = makeSnapshot({ version: 6 });
    const serverV7 = makeSnapshot({ version: 7 });

    getSharedDashboardMock.mockResolvedValue(initialShared);
    getMyDashboardMock.mockResolvedValue(null);

    // 두 번 연속 409.
    putSharedDashboardMock
      .mockResolvedValueOnce({ conflict: true, serverSnapshot: serverV6 })
      .mockResolvedValueOnce({ conflict: true, serverSnapshot: serverV7 });

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(33);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await vi.runAllTimersAsync();
    });

    expect(putSharedDashboardMock).toHaveBeenCalledTimes(2);

    // 토스트 노출 확인.
    const notifications = useUIStore.getState().notifications;
    const conflictToast = notifications.find((n) =>
      n.message.includes('동기화'),
    );
    expect(conflictToast).toBeDefined();

    // 서버 v7 로 강제 동기.
    expect(useUIStore.getState().sharedSnapshot?.version).toBe(7);
  });
});

describe('useDashboardSync — 403 on shared PUT (AC-4)', () => {
  it('shared PUT 시 403: 토스트 + 재시도 안 함', async () => {
    vi.useFakeTimers();
    getSharedDashboardMock.mockResolvedValue(makeSnapshot({ version: 1 }));
    getMyDashboardMock.mockResolvedValue(null);
    putSharedDashboardMock.mockRejectedValue(new DashboardForbiddenError());

    resetStoreState();
    useAuthStore.setState({ user: { name: 'ed', role: 'editor' } });

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(30);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(1);

    // 토스트 확인.
    const notifications = useUIStore.getState().notifications;
    const forbiddenToast = notifications.find((n) =>
      n.message.includes('공유 대시보드 편집 권한이 없습니다'),
    );
    expect(forbiddenToast).toBeDefined();
    expect(forbiddenToast?.type).toBe('warning');

    // 동일 fingerprint 로는 재시도하지 않는다 — 추가 PUT 호출 없음.
    act(() => {
      useUIStore.getState().setDashboardGridCols(30); // 동일 값
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(1);
  });
});

describe('useDashboardSync — blank slate migration (AC-15)', () => {
  it('첫 부팅 시 legacy localStorage 키가 제거되고 토스트가 1회 노출된다', async () => {
    // v0.1 시절의 localStorage 상태 모의.
    const legacyState = {
      state: {
        sidebarCollapsed: false,
        theme: 'system',
        customThemeTokens: {},
        dashboardRefreshInterval: 30,
        dashboardPages: [{ id: 'p1', name: 'Legacy', isDefault: true, panels: [], layout: [] }],
        activeDashboardId: 'p1',
        dashboardGridCols: 8,
        dashboardShowGridLines: true,
        deviceGridLayout: { devA: { i: 'devA', x: 0, y: 0, w: 1, h: 1 } },
      },
      version: 4,
    };
    testLocalStorage.setItem('xflow-ui', JSON.stringify(legacyState));
    // 마이그레이션 플래그는 없음.
    expect(testLocalStorage.getItem('xflow-ui:dashboard-migrated-v0.2')).toBeNull();

    getSharedDashboardMock.mockResolvedValue(null);
    getMyDashboardMock.mockResolvedValue(null);

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    // 마이그레이션 플래그 set 확인.
    expect(testLocalStorage.getItem('xflow-ui:dashboard-migrated-v0.2')).toBe('1');

    // 6 개 키가 localStorage 의 persist 페이로드에서 제거되었는지 확인.
    const after = JSON.parse(testLocalStorage.getItem('xflow-ui')!);
    const afterState = (after.state ?? after) as Record<string, unknown>;
    for (const key of [
      'dashboardPages',
      'activeDashboardId',
      'dashboardGridCols',
      'dashboardShowGridLines',
      'dashboardRefreshInterval',
      'deviceGridLayout',
    ]) {
      expect(afterState[key]).toBeUndefined();
    }
    // theme/customThemeTokens/sidebarCollapsed 는 유지.
    expect(afterState.theme).toBe('system');
    expect(afterState.sidebarCollapsed).toBe(false);

    // 토스트 1회 발화.
    const notifications = useUIStore.getState().notifications;
    const migrationToast = notifications.find((n) =>
      n.message.includes('대시보드 구성이 서버 저장으로 전환'),
    );
    expect(migrationToast).toBeDefined();
    expect(migrationToast?.type).toBe('info');
  });

  it('마이그레이션 플래그가 이미 set 인 상태에서 재부팅: 토스트 발화 안 함, LS 미변경', async () => {
    testLocalStorage.setItem('xflow-ui:dashboard-migrated-v0.2', '1');
    // dashboard 키가 들어있는 잔여 LS 가 있어도 손대지 않는다 (이미 마이그레이션 완료).
    testLocalStorage.setItem(
      'xflow-ui',
      JSON.stringify({
        state: {
          theme: 'night',
          sidebarCollapsed: true,
          customThemeTokens: {},
          // 잔여 key (실제론 없어야 하지만 사용자 임의 수정 등 케이스):
          dashboardPages: [],
        },
        version: 4,
      }),
    );

    getSharedDashboardMock.mockResolvedValue(null);
    getMyDashboardMock.mockResolvedValue(null);

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    // 플래그 set 유지.
    expect(testLocalStorage.getItem('xflow-ui:dashboard-migrated-v0.2')).toBe('1');

    // 토스트 발화 없음 (마이그레이션 안내).
    const notifications = useUIStore.getState().notifications;
    const migrationToast = notifications.find((n) =>
      n.message.includes('대시보드 구성이 서버 저장으로 전환'),
    );
    expect(migrationToast).toBeUndefined();

    // 기존 LS 내용은 그대로 유지된다 (재마이그레이션 안 함).
    const afterParsed = JSON.parse(testLocalStorage.getItem('xflow-ui')!);
    const afterState = afterParsed.state ?? afterParsed;
    expect(afterState.theme).toBe('night');
  });
});

describe('useDashboardSync — boot error (AC-8 보조)', () => {
  it('GET 401 이면 setError 가 호출되지 않고 interceptor 흐름에 위임 (default 적용 안 됨)', async () => {
    getSharedDashboardMock.mockRejectedValue(new DashboardUnauthorizedError());
    getMyDashboardMock.mockRejectedValue(new DashboardUnauthorizedError());

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    // 401 은 interceptor 가 처리하므로 hook 의 error state 는 null 유지.
    expect(result.current.error).toBeNull();
    // unauthorized 인 경우 default snapshot 도 적용하지 않는다 (재로그인 흐름에 위임).
    expect(useUIStore.getState().sharedSnapshot).toBeNull();
  });
});

describe('useDashboardSync — error path infinite-loop guard (regression)', () => {
  /**
   * 회귀 방지: PUT 이 401/500/네트워크 에러로 실패할 때, catch 블록에서
   * lastSyncedFingerprintRef 를 갱신하지 않으면 다음과 같은 무한 루프가 발생한다.
   *
   *   1) 사용자 변경 → fingerprint 변경 → schedulePut
   *   2) PUT → 401/500
   *   3) (선택) showToast → addNotification → store 변경
   *   4) subscribe 가 다시 fire → lastFp != currentFp (갱신 안 됐으므로) → schedulePut
   *   5) (1) 로 돌아감 — 영원히 PUT 반복
   *
   * 본 테스트는 PUT 이 401 또는 500 으로 한 번 실패한 뒤, 동일 fingerprint 에 대해
   * 더 이상 자동 재시도가 발생하지 않음을 확인한다.
   */
  it('PUT 401: 한 번만 호출되고 같은 fingerprint 로 무한 재시도하지 않는다', async () => {
    vi.useFakeTimers();
    getSharedDashboardMock.mockResolvedValue(makeSnapshot({ version: 1 }));
    getMyDashboardMock.mockResolvedValue(null);
    putSharedDashboardMock.mockRejectedValue(new DashboardUnauthorizedError());

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(30);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(1);

    // 추가로 1초 더 흘려도 spurious 재시도가 발생하지 않아야 한다.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(1);

    // 사용자가 새 변경을 가하면 정상적으로 다음 PUT 이 트리거된다 (재시도 자체는 가능).
    act(() => {
      useUIStore.getState().setDashboardGridCols(40);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(2);
  });

  it('PUT 500: 토스트 1회 + 동일 fingerprint 로 무한 재시도 없음', async () => {
    vi.useFakeTimers();
    getSharedDashboardMock.mockResolvedValue(makeSnapshot({ version: 1 }));
    getMyDashboardMock.mockResolvedValue(null);
    // DashboardServerError 가 아닌 generic Error 로도 같은 가드가 동작해야 한다.
    putSharedDashboardMock.mockRejectedValue(new Error('server boom'));

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(30);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(1);

    // 토스트는 1회.
    const errorToasts = useUIStore.getState().notifications.filter((n) =>
      n.message.includes('대시보드 저장에 실패했습니다'),
    );
    expect(errorToasts.length).toBe(1);

    // 추가 1초가 흘러도 재시도 없음.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(putSharedDashboardMock).toHaveBeenCalledTimes(1);
  });
});
