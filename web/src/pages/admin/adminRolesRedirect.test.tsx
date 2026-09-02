// `/admin/roles` 딥링크 보존 통합 테스트.
//
// 역할 관리를 사용자 관리 화면의 탭으로 옮기면서 `/admin/roles` 화면은 사라졌다.
// 기존 북마크가 죽지 않도록 그 경로는 사용자 관리의 역할 탭으로 리다이렉트한다
// (`/admin/remote/control` → `/admin/remote` 와 같은 방식).
//
// 라우트 트리를 부분적으로 흉내 내면 실제 중첩(AuthGuard → AppLayout → 리다이렉트)
// 에서만 드러나는 결함을 놓치므로, 실제 appRoutes 를 memory router 로 마운트하고
// 네트워크 경계(userService)와 앱 크롬만 스텁한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

// 인증: AuthGuard 통과용. authEnabled=false 로 두면 usePermission 폴백이
// 전원 허용이므로(구 동작 유지 규칙) nav.role 탭도 노출된다.
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({
    isAuthenticated: true,
    authEnabled: false,
    isLoading: false,
    initialize: vi.fn(),
    user: { name: 'tester', role: 'admin' as const },
  }),
}));

// 앱 크롬 — 리다이렉트와 무관하고 자체 데이터 훅을 가진다.
vi.mock('@/components/layout/Header', () => ({ default: () => null }));
vi.mock('@/components/layout/Sidebar', () => ({ default: () => null }));
vi.mock('@/hooks/useUpdateAvailableNotification', () => ({
  useUpdateAvailableNotification: () => {},
}));
vi.mock('@/hooks/useDashboardSync', () => ({
  useDashboardSync: () => ({ isLoading: false, error: null, pendingSync: false }),
}));

// 네트워크 경계 — 역할/권한 카탈로그.
const getRoles = vi.hoisted(() => vi.fn());
const getPermissionCatalog = vi.hoisted(() => vi.fn());
const getUsers = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/userService', async () => {
  const actual = await vi.importActual<typeof import('@/services/api/userService')>(
    '@/services/api/userService',
  );
  return { ...actual, getRoles, getPermissionCatalog, getUsers };
});

import { appRoutes } from '@/router';
import { useAuthStore } from '@/stores/authStore';

function renderApp(initialPath: string) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const router = createMemoryRouter(appRoutes, { initialEntries: [initialPath] });
  render(
    <I18nProvider>
      <QueryClientProvider client={client}>
        <RouterProvider router={router} />
      </QueryClientProvider>
    </I18nProvider>,
  );
  return router;
}

beforeEach(() => {
  vi.clearAllMocks();
  useAuthStore.setState({ authEnabled: false, user: null });
  getRoles.mockResolvedValue([]);
  getPermissionCatalog.mockResolvedValue([]);
  getUsers.mockResolvedValue([]);
});

describe('/admin/roles 딥링크 보존', () => {
  it('사용자 관리 화면의 역할 탭으로 리다이렉트한다', async () => {
    const router = renderApp('/admin/roles');

    await waitFor(() => {
      expect(router.state.location.pathname).toBe('/admin/users');
    });
    expect(router.state.location.search).toBe('?tab=roles');
  });

  it('리다이렉트 후 역할 탭이 활성이고 역할 본문이 보인다', async () => {
    renderApp('/admin/roles');

    const rolesTab = await screen.findByRole('tab', { name: '역할' });
    expect(rolesTab).toHaveAttribute('aria-selected', 'true');
    // 역할 패널 본문(빈 목록 안내)이 실제로 렌더된다.
    expect(await screen.findByTestId('admin-roles-panel')).toBeInTheDocument();
    expect(screen.queryByTestId('admin-users-panel')).not.toBeInTheDocument();
  });

  it('히스토리를 오염시키지 않는다 — replace 리다이렉트다', async () => {
    const router = renderApp('/admin/roles');

    await waitFor(() => expect(router.state.location.pathname).toBe('/admin/users'));
    // replace 이므로 되돌아갈 /admin/roles 항목이 남지 않는다.
    expect(router.state.historyAction).toBe('REPLACE');
  });

  it('`/admin/roles` 는 더 이상 자체 화면을 렌더하지 않는다', async () => {
    renderApp('/admin/roles');

    await screen.findByTestId('admin-user-management-page');
    // 구 화면의 컨테이너 testid 는 어디에도 없다.
    expect(screen.queryByTestId('admin-roles-page')).not.toBeInTheDocument();
  });
});
