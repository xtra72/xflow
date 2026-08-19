// 대시보드 관리 라우트 가드 테스트 (SPEC-DASHBOARD-004 M7 7.4, acceptance.md AC-10).
//
// 메뉴를 숨기는 것만으로는 부족하다 — 라우트를 열어두면 주소창에 경로를 직접
// 입력해 화면에 도달한다. AC-10 은 두 가지를 함께 요구한다:
//   (1) `nav.dashboard` 없는 사용자에게 메뉴 미노출 (Sidebar.test.tsx)
//   (2) 같은 사용자의 직접 진입 차단 → 대시보드로 리다이렉트 (이 파일)
//
// 실제 라우트 트리(appRoutes)를 memory router 로 마운트한다. 가드가 라우터에
// **등록되어 있는지**가 검증 대상이므로, 가드 컴포넌트를 따로 렌더하면 의미가 없다
// (PanelRouteSync.test.tsx 와 동일한 방침).
//
// @spec SPEC-DASHBOARD-004 v0.1.0 (§2.5, AC-10)

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

// ---- 인증 ----

const authMock = vi.hoisted(() => ({
  isAuthenticated: true,
  authEnabled: true as boolean | null,
  isLoading: false,
}));
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({ ...authMock, initialize: vi.fn(), user: { name: 't', role: 'viewer' } }),
}));

// ---- 권한 ----

const menuMock = vi.hoisted(() => ({ allowed: new Set<string>() }));
vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => menuMock.allowed.has(key),
    hasAnyPermission: (keys: readonly string[]) => keys.some((k) => menuMock.allowed.has(k)),
    canSeeMenu: (key: string) => menuMock.allowed.has(key),
    isPermissionUnavailable: false,
  }),
}));

// ---- 앱 크롬 / 무관 훅 ----

vi.mock('@/components/layout/Header', () => ({ default: () => null }));
vi.mock('@/components/layout/Sidebar', () => ({ default: () => null }));
vi.mock('@/hooks/useUpdateAvailableNotification', () => ({
  useUpdateAvailableNotification: () => {},
}));
// useDashboardSync 는 AppLayout 이, pickFallbackUid 는 useDashboardMutations 가 쓴다.
vi.mock('@/hooks/useDashboardSync', () => ({
  useDashboardSync: () => ({ isLoading: false, error: null, pendingSync: false }),
  pickFallbackUid: () => '',
}));
vi.mock('@/pages/dashboard/DashboardPage', () => ({
  default: () => <div data-testid="dashboard-view">대시보드</div>,
}));

import { appRoutes } from '@/router';

function renderAt(path: string) {
  const router = createMemoryRouter(appRoutes, { initialEntries: [path] });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

beforeEach(() => {
  authMock.isAuthenticated = true;
  authMock.authEnabled = true;
  authMock.isLoading = false;
  menuMock.allowed = new Set<string>();
});

describe('/dashboards/admin 라우트 가드 (AC-10)', () => {
  it('nav.dashboard 없이 직접 진입하면 대시보드로 되돌려 보낸다', async () => {
    renderAt('/dashboards/admin');

    await waitFor(() => expect(screen.getByTestId('dashboard-view')).toBeInTheDocument());
    expect(screen.queryByTestId('dashboard-admin-page')).not.toBeInTheDocument();
  });

  it('nav.dashboard 를 보유하면 관리 화면이 렌더된다', async () => {
    menuMock.allowed = new Set(['nav.dashboard']);
    renderAt('/dashboards/admin');

    await waitFor(() =>
      expect(screen.getByTestId('dashboard-admin-page')).toBeInTheDocument(),
    );
  });

  it('인증 비활성 배포에서는 권한 검사 없이 진입한다 (spec.md §2.10 S1)', async () => {
    authMock.authEnabled = false;
    renderAt('/dashboards/admin');

    await waitFor(() =>
      expect(screen.getByTestId('dashboard-admin-page')).toBeInTheDocument(),
    );
  });
});
