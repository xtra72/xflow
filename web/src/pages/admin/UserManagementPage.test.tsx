// 사용자 관리 화면(사용자/역할 탭 컨테이너) 테스트.
//
// 검증 대상은 컨테이너의 책임뿐이다 — 탭 노출(nav.role 메뉴 축), 탭 전환,
// URL 딥링크, 본문 제목 부재, 폭 제약 부재. 각 탭의 내용은 UsersPanel /
// RolesPanel 전용 테스트가 검증하므로 여기서는 가벼운 스텁으로 대체한다.
//
// @spec SPEC-AUTH-006 v0.1.0 (M2.2, M2.3)

import { render, screen } from '@testing-library/react';
import { fireEvent } from '@testing-library/react';
import { createMemoryRouter, RouterProvider } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

// ---- 권한 스텁 (메뉴 축) ----

const granted = vi.hoisted(() => ({ keys: new Set<string>() }));

vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => granted.keys.has(key),
    hasAnyPermission: (keys: readonly string[]) => keys.some((k) => granted.keys.has(k)),
    canSeeMenu: (key: string) => granted.keys.has(key),
    isPermissionUnavailable: false,
  }),
}));

function grant(...keys: string[]): void {
  granted.keys = new Set(keys);
}

// ---- 패널 스텁 ----
// 컨테이너 테스트는 어느 패널이 걸렸는지만 알면 된다.

vi.mock('./UsersPanel', () => ({
  default: () => <div data-testid="stub-users-panel">users-panel</div>,
}));
vi.mock('./RolesPanel', () => ({
  default: () => <div data-testid="stub-roles-panel">roles-panel</div>,
}));

import UserManagementPage from './UserManagementPage';

/** 컨테이너를 memory router 위에 마운트한다(useSearchParams 사용). */
function renderPage(initialPath = '/admin/users') {
  const router = createMemoryRouter(
    [{ path: '/admin/users', element: <UserManagementPage /> }],
    { initialEntries: [initialPath] },
  );
  const utils = render(
    <I18nProvider>
      <RouterProvider router={router} />
    </I18nProvider>,
  );
  return { ...utils, router };
}

beforeEach(() => {
  grant('nav.user', 'nav.role');
});

describe('UserManagementPage 탭 노출 (메뉴 축)', () => {
  it('nav.role 이 있으면 역할 탭을 렌더한다', () => {
    renderPage();

    expect(screen.getByRole('tab', { name: '사용자' })).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '역할' })).toBeInTheDocument();
  });

  it('nav.role 이 없으면 역할 탭을 아예 렌더하지 않는다 (비활성이 아니다)', () => {
    grant('nav.user');
    renderPage();

    expect(screen.getByRole('tab', { name: '사용자' })).toBeInTheDocument();
    expect(screen.queryByRole('tab', { name: '역할' })).not.toBeInTheDocument();
    expect(screen.getAllByRole('tab')).toHaveLength(1);
  });

  it('nav.role 없이 ?tab=roles 로 진입해도 역할 본문을 그리지 않는다', () => {
    grant('nav.user');
    renderPage('/admin/users?tab=roles');

    expect(screen.getByTestId('stub-users-panel')).toBeInTheDocument();
    expect(screen.queryByTestId('stub-roles-panel')).not.toBeInTheDocument();
  });
});

describe('UserManagementPage 탭 전환과 딥링크', () => {
  it('기본은 사용자 탭이다', () => {
    renderPage();

    expect(screen.getByTestId('stub-users-panel')).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '사용자' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('역할 탭을 누르면 본문이 바뀌고 URL 에 ?tab=roles 가 남는다', () => {
    const { router } = renderPage();

    fireEvent.click(screen.getByRole('tab', { name: '역할' }));

    expect(screen.getByTestId('stub-roles-panel')).toBeInTheDocument();
    expect(screen.queryByTestId('stub-users-panel')).not.toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '역할' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
    expect(router.state.location.search).toBe('?tab=roles');
  });

  it('?tab=roles 로 직접 진입하면 역할 탭이 활성이다 (딥링크)', () => {
    renderPage('/admin/users?tab=roles');

    expect(screen.getByTestId('stub-roles-panel')).toBeInTheDocument();
    expect(screen.getByRole('tab', { name: '역할' })).toHaveAttribute(
      'aria-selected',
      'true',
    );
  });

  it('사용자 탭으로 돌아오면 기본 탭이므로 파라미터를 지운다', () => {
    const { router } = renderPage('/admin/users?tab=roles');

    fireEvent.click(screen.getByRole('tab', { name: '사용자' }));

    expect(screen.getByTestId('stub-users-panel')).toBeInTheDocument();
    expect(router.state.location.search).toBe('');
  });

  it('알 수 없는 tab 값은 사용자 탭으로 수렴한다', () => {
    renderPage('/admin/users?tab=bogus');

    expect(screen.getByTestId('stub-users-panel')).toBeInTheDocument();
  });

  it('좌우 화살표로 탭을 이동한다 (role=tablist 키보드 계약)', () => {
    renderPage();

    fireEvent.keyDown(screen.getByRole('tab', { name: '사용자' }), { key: 'ArrowRight' });
    expect(screen.getByTestId('stub-roles-panel')).toBeInTheDocument();

    fireEvent.keyDown(screen.getByRole('tab', { name: '역할' }), { key: 'ArrowLeft' });
    expect(screen.getByTestId('stub-users-panel')).toBeInTheDocument();
  });

  it('활성 탭만 탭 순서에 남는다 (roving tabindex)', () => {
    renderPage();

    expect(screen.getByRole('tab', { name: '사용자' })).toHaveAttribute('tabindex', '0');
    expect(screen.getByRole('tab', { name: '역할' })).toHaveAttribute('tabindex', '-1');
  });

  it('다른 쿼리 파라미터는 탭 전환에서 보존된다', () => {
    const { router } = renderPage('/admin/users?keep=1');

    fireEvent.click(screen.getByRole('tab', { name: '역할' }));

    expect(router.state.location.search).toContain('keep=1');
    expect(router.state.location.search).toContain('tab=roles');
  });
});

describe('UserManagementPage 레이아웃', () => {
  it('본문에 화면 제목을 그리지 않는다 — 앱 헤더가 표시한다', () => {
    renderPage();

    expect(screen.queryByRole('heading', { level: 1 })).not.toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: '사용자 관리' })).not.toBeInTheDocument();
  });

  it('최대 폭 제약 없이 페이지 전체 폭을 쓴다', () => {
    const { container } = renderPage();

    const root = screen.getByTestId('admin-user-management-page');
    expect(root.className).not.toMatch(/max-w-/);
    expect(root.className).not.toMatch(/mx-auto/);
    for (const el of container.querySelectorAll('div')) {
      expect(el.className).not.toMatch(/max-w-(?:sm|md|lg|xl|\d)/);
    }
  });
});
