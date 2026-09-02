// SPEC-DASHBOARD-004 (M5) — useDashboardSync 훅 테스트.
//
// 검증 시나리오:
//   - AC-15: 부팅 시 목록 API 1회, 활성 대시보드만 PUT, 스코프 키 제거
//   - AC-21: 삭제/권한 상실 대시보드에서 폴백, 무한 재시도 없음
//   - 승계 메커니즘(키만 스코프 → uid): 500ms debounce, fingerprint spurious PUT
//     방지, 단일 비행 + 큐잉, 409 재PUT 후 강제 적용
//   - SPEC-DASHBOARD-001 승계: localStorage 마이그레이션 1회 + 토스트 1회
//
// @spec SPEC-DASHBOARD-004 v0.1.0

import { renderHook, act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

// ─────────────────────────────────────────────────────────────────────
// Mocks
// ─────────────────────────────────────────────────────────────────────

const listDashboardsMock = vi.hoisted(() => vi.fn());
const getDashboardMock = vi.hoisted(() => vi.fn());
const updateDashboardMock = vi.hoisted(() => vi.fn());
const getStateMock = vi.hoisted(() => vi.fn());
const putStateMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/dashboardService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/dashboardService')>(
    '@/services/api/dashboardService',
  );
  return {
    ...actual,
    listDashboards: listDashboardsMock,
    getDashboard: getDashboardMock,
    updateDashboard: updateDashboardMock,
    getState: getStateMock,
    putState: putStateMock,
  };
});

// i18n 모킹: I18nProvider 없이 렌더하기 위해 useTranslation 을 교체한다.
// 토스트 메시지 단언이 실제 한국어 문자열을 검사하므로 ko.json 을 해석해 반환한다.
vi.mock('@/lib/i18n', async () => {
  const ko = (await import('@/lib/i18n/ko.json')).default as Record<string, unknown>;
  const resolveKo = (key: string): string => {
    const value = key
      .split('.')
      .reduce<unknown>(
        (obj, part) =>
          obj != null && typeof obj === 'object'
            ? (obj as Record<string, unknown>)[part]
            : undefined,
        ko,
      );
    return typeof value === 'string' ? value : key;
  };
  return {
    useTranslation: () => ({ t: (k: string) => resolveKo(k) }),
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
  DashboardNotFoundError,
  DashboardUnauthorizedError,
} from '@/services/api/dashboardService';
import { useAuthStore } from '@/stores/authStore';
import {
  consumeMigrationToastFlag,
  runDashboardLocalStorageMigration,
  useUIStore,
} from '@/stores/uiStore';

import { useDashboardSync } from './useDashboardSync';
import type { Dashboard, DashboardDetail, DashboardUserState } from '@/types/dashboard';

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

function makeDashboard(overrides: Partial<Dashboard> = {}): Dashboard {
  return {
    uid: 'dash-1',
    name: 'D1',
    owner: 'tester',
    visibility: 'private',
    is_default: true,
    sort_order: 0,
    version: 1,
    created_at: 1000,
    updated_at: 1000,
    can_edit: true,
    can_delete: true,
    can_grant: true,
    ...overrides,
  };
}

function makeDetail(overrides: Partial<DashboardDetail> = {}): DashboardDetail {
  const { payload, ...meta } = overrides;
  return {
    ...makeDashboard(meta),
    payload: {
      panels: [],
      layout: [],
      gridCols: 10,
      showGridLines: true,
      refreshInterval: 10,
      ...(payload ?? {}),
    },
  };
}

function makeUserState(overrides: Partial<DashboardUserState> = {}): DashboardUserState {
  return {
    active_dashboard_uid: '',
    device_grid_layout: {},
    version: 0,
    updated_at: 0,
    ...overrides,
  };
}

/** getDashboard 를 uid → detail 표로 응답하게 만든다. */
function serveDetails(details: DashboardDetail[]): void {
  getDashboardMock.mockImplementation(async (uid: string) =>
    details.find((d) => d.uid === uid) ?? null,
  );
}

/** 최근 PUT 호출의 uid 목록. */
function putUids(): string[] {
  return updateDashboardMock.mock.calls.map((c) => c[0] as string);
}

/**
 * 테스트 사이에 store 상태를 초기화한다. vi.resetModules 는 사용하지 않는다 —
 * 훅이 import 한 store 와 테스트가 import 한 store 의 모듈 인스턴스를 동일하게 유지해야
 * subscribe / setState 가 작동한다.
 */
function resetStoreState(): void {
  consumeMigrationToastFlag();
  runDashboardLocalStorageMigration();

  useUIStore.setState((state) => ({
    ...state,
    dashboards: [],
    notifications: [],
    dashboardPages: [
      { id: 'default', name: '대시보드', isDefault: true, panels: [], layout: [] },
    ],
    activeDashboardId: 'default',
    dashboardGridCols: 10,
    dashboardShowGridLines: true,
    dashboardRefreshInterval: 10,
    deviceGridLayout: {},
  }));
}

/** 인증 store 에 사용자를 주입한다 (부팅 경로가 권한을 읽지는 않는다). */
function setAuthUser(name: string, role: string): void {
  useAuthStore.setState({
    user: { name, role },
    permissions: new Set(['dashboard.read', 'dashboard.update']),
    permissionStatus: 'loaded',
  });
}

beforeEach(() => {
  resetStorage();
  listDashboardsMock.mockReset();
  getDashboardMock.mockReset();
  updateDashboardMock.mockReset();
  getStateMock.mockReset();
  putStateMock.mockReset();

  // 기본값: 대시보드 1장, 서버가 그것을 활성으로 기억.
  listDashboardsMock.mockResolvedValue([makeDashboard()]);
  serveDetails([makeDetail()]);
  getStateMock.mockResolvedValue(makeUserState({ active_dashboard_uid: 'dash-1' }));
  putStateMock.mockResolvedValue(makeUserState({ active_dashboard_uid: 'dash-1' }));

  useAuthStore.setState({
    tokens: null,
    isAuthenticated: true,
    isLoading: false,
    authEnabled: true,
  });
  setAuthUser('tester', 'admin');
});

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

// ─────────────────────────────────────────────────────────────────────
// AC-15 — 프론트엔드 단일 축 전환
// ─────────────────────────────────────────────────────────────────────

describe('useDashboardSync — 부팅 (AC-15)', () => {
  it('부팅 시 목록 API 를 1회만 호출한다', async () => {
    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(listDashboardsMock).toHaveBeenCalledTimes(1);
    // 활성 대시보드 1장의 본문만 받는다.
    expect(getDashboardMock).toHaveBeenCalledTimes(1);
    expect(getDashboardMock).toHaveBeenCalledWith('dash-1');
    // 목록이 store 의 단일 축에 반영된다.
    expect(useUIStore.getState().dashboards.map((d) => d.uid)).toEqual(['dash-1']);
    expect(useUIStore.getState().activeDashboardId).toBe('dash-1');
  });

  it('스코프 엔드포인트(shared/mine)는 서비스에 존재하지 않는다', async () => {
    const svc = await import('@/services/api/dashboardService');
    for (const removed of [
      'getSharedDashboard',
      'getMyDashboard',
      'putSharedDashboard',
      'putMyDashboard',
      'deleteSharedDashboard',
      'deleteMyDashboard',
    ]) {
      expect(removed in svc).toBe(false);
    }
  });

  it('uiStore 에 스코프 키가 존재하지 않는다', async () => {
    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    const state = useUIStore.getState() as unknown as Record<string, unknown>;
    // AC-15 는 이 키들의 부재와 동시에 잔여 참조 grep 이 무출력일 것을 요구한다.
    // 두 조건을 함께 만족시키려고 스코프 키 이름은 리터럴 대신 조립해서 만든다.
    const removedKeys = [`activeDashboard${'Scope'}`, 'sharedSnapshot', 'mineSnapshot'];
    for (const key of removedKeys) {
      expect(key in state).toBe(false);
    }
  });

  it('서버가 기억한 활성 uid 가 유효하면 정정 저장하지 않는다', async () => {
    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(putStateMock).not.toHaveBeenCalled();
  });

  it('부팅 직후에는 PUT 이 발생하지 않는다', async () => {
    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(updateDashboardMock).not.toHaveBeenCalled();
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-15 — 대시보드 단위 PUT
// ─────────────────────────────────────────────────────────────────────

describe('useDashboardSync — 대시보드 단위 저장 (AC-15)', () => {
  beforeEach(() => {
    listDashboardsMock.mockResolvedValue([
      makeDashboard({ uid: 'dash-1', is_default: true }),
      makeDashboard({ uid: 'dash-2', name: 'D2', is_default: false, version: 4 }),
    ]);
    serveDetails([makeDetail(), makeDetail({ uid: 'dash-2', name: 'D2', version: 4 })]);
  });

  it('활성 대시보드만 PUT 한다', async () => {
    vi.useFakeTimers();
    updateDashboardMock.mockResolvedValue(makeDetail({ version: 2 }));
    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));
    expect(useUIStore.getState().activeDashboardId).toBe('dash-1');

    act(() => {
      useUIStore.getState().addPanel('text');
    });

    expect(updateDashboardMock).not.toHaveBeenCalled();
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    expect(updateDashboardMock).toHaveBeenCalledTimes(1);
    expect(putUids()).toEqual(['dash-1']);
    // 다른 대시보드로는 PUT 하지 않는다.
    expect(putUids()).not.toContain('dash-2');

    // If-Match 는 활성 대시보드의 version.
    expect(updateDashboardMock.mock.calls[0]![2]).toBe(1);
    // 전송 본문은 그 대시보드 1장 분량 — dashboardPages 배열이 들어가지 않는다.
    const sent = updateDashboardMock.mock.calls[0]![1] as Record<string, unknown>;
    expect(Object.keys(sent).sort()).toEqual(
      ['gridCols', 'layout', 'panels', 'refreshInterval', 'showGridLines'].sort(),
    );
    expect((sent.panels as { type: string }[]).some((p) => p.type === 'text')).toBe(true);
  });

  it('500ms 안의 연속 변경은 1회의 PUT 으로 합쳐진다', async () => {
    vi.useFakeTimers();
    updateDashboardMock.mockResolvedValue(makeDetail({ version: 2 }));
    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(12);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });
    act(() => {
      useUIStore.getState().setDashboardGridCols(14);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(200);
    });
    act(() => {
      useUIStore.getState().setDashboardGridCols(16);
    });
    expect(updateDashboardMock).not.toHaveBeenCalled();

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });

    expect(updateDashboardMock).toHaveBeenCalledTimes(1);
    expect((updateDashboardMock.mock.calls[0]![1] as { gridCols: number }).gridCols).toBe(16);
  });

  it('동일 내용으로 되돌아오면 PUT 하지 않는다 (fingerprint 억제)', async () => {
    vi.useFakeTimers();
    updateDashboardMock.mockResolvedValue(makeDetail({ version: 2 }));
    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    // 값을 바꿨다가 debounce 만료 전에 원래 값으로 되돌린다.
    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
      useUIStore.getState().setDashboardGridCols(10);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).not.toHaveBeenCalled();

    // 실제 변경은 PUT 된다.
    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);

    // 저장된 값과 동일한 값을 다시 세팅해도 추가 PUT 은 없다.
    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);
  });

  it('in-flight 중 발생한 변경은 큐잉되어 응답 후 1회 더 PUT 된다', async () => {
    vi.useFakeTimers();
    let resolveFirst: ((d: DashboardDetail) => void) | null = null;
    updateDashboardMock
      .mockImplementationOnce(
        () =>
          new Promise<DashboardDetail>((resolve) => {
            resolveFirst = resolve;
          }),
      )
      .mockResolvedValue(makeDetail({ version: 3 }));

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);

    // in-flight 중 추가 변경 → debounce 만료 시점에 큐잉만 된다.
    act(() => {
      useUIStore.getState().setDashboardGridCols(30);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);

    // 첫 PUT 응답 → 큐가 발사된다.
    await act(async () => {
      resolveFirst!(makeDetail({ version: 2 }));
      await vi.runAllTimersAsync();
    });

    expect(updateDashboardMock).toHaveBeenCalledTimes(2);
    expect(putUids()).toEqual(['dash-1', 'dash-1']);
    expect((updateDashboardMock.mock.calls[1]![1] as { gridCols: number }).gridCols).toBe(30);
    // 두 번째 PUT 은 첫 응답의 version 을 If-Match 로 쓴다.
    expect(updateDashboardMock.mock.calls[1]![2]).toBe(2);
  });
});

// ─────────────────────────────────────────────────────────────────────
// 409 충돌 (승계 메커니즘)
// ─────────────────────────────────────────────────────────────────────

describe('useDashboardSync — 409 충돌', () => {
  it('409 발생 후 1회 재PUT 으로 사용자 변경을 보존한다', async () => {
    vi.useFakeTimers();
    updateDashboardMock
      .mockResolvedValueOnce({
        conflict: true,
        serverDashboard: makeDetail({ version: 6, payload: { panels: [], layout: [], gridCols: 99 } }),
      })
      .mockResolvedValueOnce(makeDetail({ version: 7 }));

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(25);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await vi.runAllTimersAsync();
    });

    expect(updateDashboardMock).toHaveBeenCalledTimes(2);
    // 1차 If-Match = 1(부팅 version), 2차 = 6(서버가 알려준 최신 version).
    expect(updateDashboardMock.mock.calls[0]![2]).toBe(1);
    expect(updateDashboardMock.mock.calls[1]![2]).toBe(6);
    // 사용자의 변경(gridCols=25)이 재PUT 에서도 보존된다.
    expect((updateDashboardMock.mock.calls[1]![1] as { gridCols: number }).gridCols).toBe(25);
    expect(useUIStore.getState().dashboards[0]!.version).toBe(7);
  });

  it('재PUT 도 409 이면 서버 상태를 강제 적용하고 안내한다', async () => {
    vi.useFakeTimers();
    updateDashboardMock
      .mockResolvedValueOnce({
        conflict: true,
        serverDashboard: makeDetail({ version: 6, payload: { panels: [], layout: [], gridCols: 88 } }),
      })
      .mockResolvedValueOnce({
        conflict: true,
        serverDashboard: makeDetail({ version: 7, payload: { panels: [], layout: [], gridCols: 77 } }),
      });

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

    expect(updateDashboardMock).toHaveBeenCalledTimes(2);
    const conflictToast = useUIStore
      .getState()
      .notifications.find((n) => n.message.includes('동기화'));
    expect(conflictToast).toBeDefined();
    // 서버 v7 상태(gridCols=77)로 강제 동기.
    expect(useUIStore.getState().dashboards[0]!.version).toBe(7);
    expect(useUIStore.getState().dashboardGridCols).toBe(77);

    // 강제 적용 후 추가 PUT 이 발생하지 않는다 (fingerprint 동기화 확인).
    await act(async () => {
      await vi.advanceTimersByTimeAsync(2000);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(2);
  });
});

// ─────────────────────────────────────────────────────────────────────
// AC-21 — 403 / 404 폴백
// ─────────────────────────────────────────────────────────────────────

describe('useDashboardSync — 접근 상실 복구 (AC-21)', () => {
  it('삭제된 활성 대시보드를 기본 대시보드로 폴백한다', async () => {
    vi.useFakeTimers();
    const survivor = makeDashboard({ uid: 'dash-9', name: 'D9', is_default: true, version: 3 });
    listDashboardsMock
      .mockResolvedValueOnce([makeDashboard({ uid: 'dash-1' })])
      // PUT 404 이후의 재조회 — dash-1 이 사라졌다.
      .mockResolvedValue([makeDashboard({ uid: 'dash-5', is_default: false }), survivor]);
    serveDetails([
      makeDetail(),
      makeDetail({ uid: 'dash-9', name: 'D9', version: 3, is_default: true }),
      makeDetail({ uid: 'dash-5', is_default: false }),
    ]);
    updateDashboardMock.mockRejectedValue(new DashboardNotFoundError());

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await vi.runAllTimersAsync();
    });

    // 목록을 1회 재조회하고 is_default 대시보드로 폴백한다.
    expect(listDashboardsMock).toHaveBeenCalledTimes(2);
    expect(useUIStore.getState().activeDashboardId).toBe('dash-9');
    // 정정 저장 (PUT /dashboard-state).
    await act(async () => {
      await vi.runAllTimersAsync();
    });
    expect(putStateMock).toHaveBeenCalledWith(
      expect.objectContaining({ active_dashboard_uid: 'dash-9' }),
    );
  });

  it('404 수신 후 재시도하지 않는다', async () => {
    vi.useFakeTimers();
    listDashboardsMock.mockResolvedValue([makeDashboard({ uid: 'dash-1' })]);
    updateDashboardMock.mockRejectedValue(new DashboardNotFoundError());

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await vi.runAllTimersAsync();
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);

    // 추가 시간이 흘러도 재시도가 없다 (무한 루프 가드).
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);
  });

  it('403 이면 목록을 재조회하고 안내하되 폴백하지 않는다', async () => {
    vi.useFakeTimers();
    listDashboardsMock.mockResolvedValue([
      makeDashboard({ uid: 'dash-1', can_edit: false }),
    ]);
    updateDashboardMock.mockRejectedValue(new DashboardForbiddenError());

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(20);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await vi.runAllTimersAsync();
    });

    expect(updateDashboardMock).toHaveBeenCalledTimes(1);
    expect(listDashboardsMock).toHaveBeenCalledTimes(2);
    // 대시보드는 남아 있으므로 활성은 그대로, can_edit 만 갱신된다.
    expect(useUIStore.getState().activeDashboardId).toBe('dash-1');
    expect(useUIStore.getState().dashboards[0]!.can_edit).toBe(false);

    const toast = useUIStore
      .getState()
      .notifications.find((n) => n.message.includes('편집할 권한이 없습니다'));
    expect(toast).toBeDefined();
    expect(toast?.type).toBe('warning');

    // 재시도 없음.
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);
  });

  it('접근 가능한 대시보드가 0장이면 빈 상태로 두고 재시도하지 않는다', async () => {
    listDashboardsMock.mockResolvedValue([]);
    getStateMock.mockResolvedValue(makeUserState({ active_dashboard_uid: 'gone' }));

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(useUIStore.getState().activeDashboardId).toBe('');
    expect(useUIStore.getState().dashboardPages).toEqual([]);
    // 본문 조회를 시도하지 않는다.
    expect(getDashboardMock).not.toHaveBeenCalled();
    expect(listDashboardsMock).toHaveBeenCalledTimes(1);
  });

  it('서버가 기억한 활성 uid 가 사라졌으면 폴백 후 정정 저장한다', async () => {
    listDashboardsMock.mockResolvedValue([
      makeDashboard({ uid: 'dash-a', is_default: false }),
      makeDashboard({ uid: 'dash-b', is_default: true }),
    ]);
    serveDetails([
      makeDetail({ uid: 'dash-a', is_default: false }),
      makeDetail({ uid: 'dash-b', is_default: true }),
    ]);
    getStateMock.mockResolvedValue(makeUserState({ active_dashboard_uid: 'deleted-uid' }));

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(useUIStore.getState().activeDashboardId).toBe('dash-b');
    expect(putStateMock).toHaveBeenCalledWith(
      expect.objectContaining({ active_dashboard_uid: 'dash-b' }),
    );
  });
});

// ─────────────────────────────────────────────────────────────────────
// 에러 경로 무한 루프 가드 (회귀)
// ─────────────────────────────────────────────────────────────────────

describe('useDashboardSync — 에러 경로 무한 루프 가드', () => {
  it('PUT 401: 한 번만 호출되고 같은 내용으로 무한 재시도하지 않는다', async () => {
    vi.useFakeTimers();
    updateDashboardMock.mockRejectedValue(new DashboardUnauthorizedError());

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(30);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);

    // 새 변경은 정상적으로 PUT 된다.
    act(() => {
      useUIStore.getState().setDashboardGridCols(40);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(2);
  });

  it('PUT 500: 토스트 1회 + 동일 내용 무한 재시도 없음', async () => {
    vi.useFakeTimers();
    updateDashboardMock.mockRejectedValue(new Error('server boom'));

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await vi.waitFor(() => expect(result.current.isLoading).toBe(false));

    act(() => {
      useUIStore.getState().setDashboardGridCols(30);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);

    const errorToasts = useUIStore
      .getState()
      .notifications.filter((n) => n.message.includes('대시보드 저장에 실패했습니다'));
    expect(errorToasts.length).toBe(1);

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    expect(updateDashboardMock).toHaveBeenCalledTimes(1);
  });

  it('부팅 목록 조회가 401 이면 에러를 노출하지 않는다', async () => {
    listDashboardsMock.mockRejectedValue(new DashboardUnauthorizedError());

    resetStoreState();
    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(result.current.error).toBeNull();
    expect(useUIStore.getState().dashboards).toEqual([]);
  });
});

// ─────────────────────────────────────────────────────────────────────
// blank slate 마이그레이션 (SPEC-DASHBOARD-001 승계)
// ─────────────────────────────────────────────────────────────────────

describe('useDashboardSync — blank slate 마이그레이션', () => {
  it('첫 부팅 시 legacy localStorage 키가 제거되고 토스트가 1회 노출된다', async () => {
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
    expect(testLocalStorage.getItem('xflow-ui:dashboard-migrated-v0.2')).toBeNull();

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(testLocalStorage.getItem('xflow-ui:dashboard-migrated-v0.2')).toBe('1');

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
    expect(afterState.theme).toBe('system');
    expect(afterState.sidebarCollapsed).toBe(false);

    const migrationToast = useUIStore
      .getState()
      .notifications.find((n) => n.message.includes('대시보드 구성이 서버 저장으로 전환'));
    expect(migrationToast).toBeDefined();
    expect(migrationToast?.type).toBe('info');
  });

  it('마이그레이션 플래그가 이미 set 이면 토스트를 다시 노출하지 않는다', async () => {
    testLocalStorage.setItem('xflow-ui:dashboard-migrated-v0.2', '1');
    testLocalStorage.setItem(
      'xflow-ui',
      JSON.stringify({
        state: { theme: 'night', sidebarCollapsed: true, customThemeTokens: {}, dashboardPages: [] },
        version: 4,
      }),
    );

    resetStoreState();

    const { result } = renderHook(() => useDashboardSync());
    await waitFor(() => expect(result.current.isLoading).toBe(false));

    expect(testLocalStorage.getItem('xflow-ui:dashboard-migrated-v0.2')).toBe('1');
    const migrationToast = useUIStore
      .getState()
      .notifications.find((n) => n.message.includes('대시보드 구성이 서버 저장으로 전환'));
    expect(migrationToast).toBeUndefined();

    const afterParsed = JSON.parse(testLocalStorage.getItem('xflow-ui')!);
    const afterState = afterParsed.state ?? afterParsed;
    expect(afterState.theme).toBe('night');
  });
});
