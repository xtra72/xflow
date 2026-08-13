// 패널 추가/설정 라우트 왕복 시 대시보드 변경이 유지되는지 검증하는 통합 테스트.
//
// 회귀 배경:
//   `/`(DashboardPage), `/panels/new`, `/panels/:panelId/settings` 는 모두 AppLayout
//   의 형제 자식 라우트다. 동기화 훅(useDashboardSync)이 DashboardPage 안에 있으면
//   패널 추가 라우트로 이동하는 순간 훅이 언마운트되어
//     (1) 저장 PUT 을 발사하는 store 구독 + debounce 타이머가 사라지고,
//     (2) 대시보드 복귀 시 부팅 GET 이 재실행되어 서버 snapshot 이 로컬 변경을
//         덮어쓴다 (dashboardPages 는 persist 대상이 아니라 로컬 fallback 도 없다).
//   결과: "패널 추가가 아무 동작도 하지 않는" 것처럼 보인다.
//
// 이 테스트가 기존 테스트와 다른 점:
//   PanelPages.test.tsx 는 PanelCreatePage 를 단독 렌더하므로 AppLayout/동기화 훅이
//   트리에 없어 왕복 자체를 검증하지 못한다. 여기서는 실제 라우트 트리(appRoutes)를
//   memory router 로 마운트하고, store 는 실제 싱글톤을, 모킹은 네트워크 경계
//   (dashboardService)에서만 수행한다.
//
// 스텁 범위(결함과 무관한 부분만):
//   - AddPanelDialog / PanelSettingsDialog 본문: 위저드 UI 는 각 전용 테스트가 검증한다.
//     여기서는 실제 store 액션(addPanel / updatePanelTitle)만 호출하는 스텁으로 대체 —
//     실제 다이얼로그가 하는 것과 동일한 호출(`addPanel(type); onClose();`)이다.
//   - Header / Sidebar / 시스템 버전 폴링 / 대시보드 데이터 훅: 앱 크롬 및 무관 데이터.
//
// @spec SPEC-DASHBOARD-001 v0.2.0

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import type { DashboardPayload, DashboardSnapshot } from '@/types/dashboard';

// ─────────────────────────────────────────────────────────────────────
// Mocks — 네트워크 경계(dashboardService)와 무관한 주변 모듈만.
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

// 인증: AuthGuard 통과용. authEnabled=false 로 role 검사까지 우회한다.
const initializeMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({
    isAuthenticated: true,
    authEnabled: false,
    isLoading: false,
    initialize: initializeMock,
    user: { name: 'tester', role: 'admin' as const },
  }),
}));

// 앱 크롬(라우팅 왕복과 무관) — 자체 데이터 훅을 가지므로 스텁.
vi.mock('@/components/layout/Header', () => ({ default: () => null }));
vi.mock('@/components/layout/Sidebar', () => ({ default: () => null }));
vi.mock('@/hooks/useUpdateAvailableNotification', () => ({
  useUpdateAvailableNotification: () => {},
}));

// 대시보드 데이터 훅 / 렌더러 — 결정적 스텁.
vi.mock('@/hooks', () => ({
  useFlows: () => ({ data: { data: [] }, isLoading: false, error: null }),
  useWebSocket: () => ({ state: 'connected', client: null }),
}));
vi.mock('@/services/api/monitorService', () => ({
  getMetrics: vi.fn().mockResolvedValue(null),
}));
vi.mock('react-grid-layout', () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="grid">{children}</div>
  ),
}));
vi.mock('./renderDashboardPanel', () => ({ renderDashboardPanel: () => null }));

// i18n 스텁 — 키를 그대로 반환한다(버튼/인디케이터 조회에 사용).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

// 다이얼로그 본문 스텁 — 실제 store 액션만 호출한다(모킹된 store 아님).
vi.mock('./AddPanelDialog', async () => {
  const { useUIStore } = await import('@/stores/uiStore');
  return {
    default: ({ onClose }: { open: boolean; onClose: () => void }) => (
      <button
        type="button"
        onClick={() => {
          // 실제 AddPanelDialog 가 단순 타입 선택 시 수행하는 것과 동일한 호출.
          useUIStore.getState().addPanel('text');
          onClose();
        }}
      >
        add-text-panel
      </button>
    ),
  };
});
vi.mock('./PanelSettingsDialog', async () => {
  const { useUIStore } = await import('@/stores/uiStore');
  return {
    default: ({ panelId, onClose }: { panelId: string; onClose: () => void }) => (
      <button
        type="button"
        onClick={() => {
          useUIStore.getState().updatePanelTitle(panelId, 'renamed-panel');
          onClose();
        }}
      >
        rename-panel
      </button>
    ),
  };
});

// ─────────────────────────────────────────────────────────────────────
// Imports under test (after mocks)
// ─────────────────────────────────────────────────────────────────────

import { appRoutes } from '@/router';
import { useAuthStore } from '@/stores/authStore';
import { useUIStore } from '@/stores/uiStore';

// ─────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────

/** 서버가 돌려주는 shared snapshot (패널 목록만 파라미터화). */
function makeSnapshot(
  panels: { id: string; type: string; title: string }[] = [],
  version = 1,
): DashboardSnapshot {
  return {
    scope: 'global',
    owner: null,
    version,
    updatedAt: 1000,
    payload: {
      dashboardPages: [
        {
          id: 'page-1',
          name: 'P1',
          isDefault: true,
          panels: panels as never,
          layout: [],
        },
      ],
      activeDashboardId: 'page-1',
      dashboardGridCols: 10,
      dashboardShowGridLines: true,
      dashboardRefreshInterval: 10,
      deviceGridLayout: {},
    },
  };
}

/** 현재 활성 대시보드의 패널 목록 (실제 store 조회). */
function activePanels(): { id: string; type: string; title: string }[] {
  const state = useUIStore.getState();
  const page = state.dashboardPages.find((p) => p.id === state.activeDashboardId);
  return (page?.panels ?? []) as { id: string; type: string; title: string }[];
}

/** 실제 라우트 트리(appRoutes)를 memory router 로 마운트한다. */
function renderApp(initialPath = '/') {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const router = createMemoryRouter(appRoutes, { initialEntries: [initialPath] });
  render(
    <QueryClientProvider client={client}>
      <RouterProvider router={router} />
    </QueryClientProvider>,
  );
  return router;
}

/** 부팅 GET 이 store 에 반영될 때까지 대기 + 대시보드 렌더 대기. */
async function waitForBoot(version = 1): Promise<void> {
  await waitFor(() =>
    expect(useUIStore.getState().sharedSnapshot?.version).toBe(version),
  );
  await screen.findByRole('tablist', { name: 'dashboard.scope.aria' });
}

beforeEach(() => {
  getSharedDashboardMock.mockReset();
  getMyDashboardMock.mockReset();
  putSharedDashboardMock.mockReset();
  putMyDashboardMock.mockReset();

  // 부팅 GET 기본값: shared 는 서버 snapshot, mine 은 404(null).
  getSharedDashboardMock.mockResolvedValue(makeSnapshot());
  getMyDashboardMock.mockResolvedValue(null);
  // PUT 기본값: 받은 payload 를 그대로 승인하고 version 을 올린다.
  putSharedDashboardMock.mockImplementation(async (payload: DashboardPayload) => ({
    ...makeSnapshot([], 2),
    payload,
  }));

  useAuthStore.setState({
    user: { name: 'tester', role: 'admin' },
    tokens: null,
    isAuthenticated: true,
    isLoading: false,
    authEnabled: true,
  });

  // 실제 store 싱글톤 초기화 (모킹하지 않는다 — 덮어쓰기 여부가 검증 대상).
  useUIStore.setState((state) => ({
    ...state,
    sharedSnapshot: null,
    mineSnapshot: null,
    activeDashboardScope: 'shared',
    notifications: [],
    dashboardEditMode: false,
    dashboardPages: [
      { id: 'default', name: '대시보드', isDefault: true, panels: [], layout: [] },
    ],
    activeDashboardId: 'default',
    dashboardGridCols: 10,
    dashboardShowGridLines: true,
    dashboardRefreshInterval: 10,
    deviceGridLayout: {},
  }));
});

// ─────────────────────────────────────────────────────────────────────
// Tests
// ─────────────────────────────────────────────────────────────────────

describe('패널 추가 라우트 왕복 (/ → /panels/new → /)', () => {
  it('추가한 패널이 유지되고 서버 PUT 이 발사된다', async () => {
    const router = renderApp('/');
    await waitForBoot();

    // 편집 모드 진입 — "패널 추가" 버튼은 편집 모드에서만 노출된다.
    // (dashboardEditMode 는 서버 payload 에 포함되지 않으므로 PUT 을 유발하지 않는다.)
    act(() => {
      useUIStore.getState().setDashboardEditMode(true);
    });

    // 실제 "패널 추가" 버튼 클릭 → navigate('/panels/new').
    fireEvent.click(
      await screen.findByRole('button', { name: /dashboard\.addPanel/ }),
    );
    await waitFor(() => expect(router.state.location.pathname).toBe('/panels/new'));

    // 패널 추가 후 대시보드로 복귀.
    fireEvent.click(await screen.findByText('add-text-panel'));
    await waitFor(() => expect(router.state.location.pathname).toBe('/'));

    // (1) 저장 PUT 이 발사되어야 한다 — 500 ms debounce.
    await waitFor(() => expect(putSharedDashboardMock).toHaveBeenCalledTimes(1), {
      timeout: 3000,
    });
    const sentPayload = putSharedDashboardMock.mock.calls[0]![0] as DashboardPayload;
    expect(
      sentPayload.dashboardPages.flatMap((p) => p.panels).some((p) => p.type === 'text'),
    ).toBe(true);

    // (2) 복귀 후에도 패널이 살아있어야 한다 — 재부팅 GET 이 덮어쓰면 사라진다.
    expect(activePanels().some((p) => p.type === 'text')).toBe(true);

    // (3) 부팅 GET 은 앱 셸 마운트당 1회 — 라우트 전환으로 재실행되지 않는다.
    expect(getSharedDashboardMock).toHaveBeenCalledTimes(1);
    expect(getMyDashboardMock).toHaveBeenCalledTimes(1);
  });

  it('라우트를 여러 번 왕복해도 부팅 GET 은 1회만 실행된다', async () => {
    const router = renderApp('/');
    await waitForBoot();

    for (const path of ['/panels/new', '/', '/panels/new', '/']) {
      await waitFor(async () => {
        await router.navigate(path);
        expect(router.state.location.pathname).toBe(path);
      });
    }
    await screen.findByRole('tablist', { name: 'dashboard.scope.aria' });

    expect(getSharedDashboardMock).toHaveBeenCalledTimes(1);
    expect(getMyDashboardMock).toHaveBeenCalledTimes(1);
  });

  it('pendingSync 가 대시보드까지 전달된다 (동기화 인디케이터)', async () => {
    // PUT 을 보류시켜 pendingSync=true 상태를 관측 가능하게 만든다.
    let resolvePut: ((snapshot: DashboardSnapshot) => void) | null = null;
    putSharedDashboardMock.mockImplementation(
      (payload: DashboardPayload) =>
        new Promise<DashboardSnapshot>((resolve) => {
          resolvePut = () => resolve({ ...makeSnapshot([], 2), payload });
        }),
    );

    const router = renderApp('/');
    await waitForBoot();

    // 인디케이터는 초기에 노출되지 않는다.
    expect(screen.queryByText('dashboard.scope.syncing')).toBeNull();

    await waitFor(async () => {
      await router.navigate('/panels/new');
      expect(router.state.location.pathname).toBe('/panels/new');
    });
    fireEvent.click(await screen.findByText('add-text-panel'));
    await waitFor(() => expect(router.state.location.pathname).toBe('/'));

    // AppLayout 이 소유한 훅의 pendingSync 가 컨텍스트로 DashboardPage 에 도달한다.
    await screen.findByText('dashboard.scope.syncing');

    // PUT 완료 후 인디케이터가 사라진다.
    await waitFor(() => expect(resolvePut).not.toBeNull(), { timeout: 3000 });
    resolvePut!({} as DashboardSnapshot);
    await waitFor(() =>
      expect(screen.queryByText('dashboard.scope.syncing')).toBeNull(),
    );
  });
});

describe('패널 설정 라우트 왕복 (/ → /panels/:panelId/settings → /)', () => {
  it('변경한 패널 제목이 유지되고 서버 PUT 이 발사된다', async () => {
    // 서버 snapshot 에 패널 1개가 있는 상태로 부팅.
    getSharedDashboardMock.mockResolvedValue(
      makeSnapshot([{ id: 'p1', type: 'text', title: 'before' }]),
    );

    const router = renderApp('/');
    await waitForBoot();
    expect(activePanels()[0]?.title).toBe('before');

    await waitFor(async () => {
      await router.navigate('/panels/p1/settings');
      expect(router.state.location.pathname).toBe('/panels/p1/settings');
    });

    fireEvent.click(await screen.findByText('rename-panel'));
    await waitFor(() => expect(router.state.location.pathname).toBe('/'));

    await waitFor(() => expect(putSharedDashboardMock).toHaveBeenCalledTimes(1), {
      timeout: 3000,
    });
    const sentPayload = putSharedDashboardMock.mock.calls[0]![0] as DashboardPayload;
    expect(
      sentPayload.dashboardPages
        .flatMap((p) => p.panels)
        .some((p) => p.title === 'renamed-panel'),
    ).toBe(true);

    expect(activePanels()[0]?.title).toBe('renamed-panel');
    expect(getSharedDashboardMock).toHaveBeenCalledTimes(1);
  });
});
