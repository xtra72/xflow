// Sidebar 테스트 (SPEC-REMOTE-001).
//
// 범위: 원격 관리(admin) 그룹은 admin 권한 + 원격 관리 server 모드를 모두
// 충족할 때만 노출된다. 비-server 모드/로딩 중에는 숨긴다.

import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';
import type { RemoteMode } from '@/types/remote';
import type { User, UserRole } from '@/types/auth';

// ---- useAuth mock ----
const useAuthMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useAuth', () => ({ useAuth: useAuthMock }));

// ---- usePermission mock ----
// SPEC-AUTH-006 M3: 게이팅이 역할 이름이 아니라 권한 키로 판정한다.
// 기본은 전원 허용이고, 권한 부족 상황은 각 테스트가 denied 에 키를 넣어 만든다.
const permissionMock = vi.hoisted(() => ({ denied: new Set<string>() }));
vi.mock('@/hooks/usePermission', () => ({
  usePermission: () => ({
    hasPermission: (key: string) => !permissionMock.denied.has(key),
    hasAnyPermission: (keys: readonly string[]) =>
      keys.some((key) => !permissionMock.denied.has(key)),
    // SPEC-AUTH-006 E2: 메뉴 노출은 nav.* 만 본다.
    canSeeMenu: (navKey: string) => !permissionMock.denied.has(navKey),
    isPermissionUnavailable: false,
  }),
}));

// ---- useRemoteMode mock ----
const useRemoteModeMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useRemote', () => ({ useRemoteMode: useRemoteModeMock }));

// ---- uiStore mock (사이드바 접힘 상태) ----
const sidebarCollapsedMock = vi.hoisted(() => ({ value: false }));
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (
    selector?: (s: {
      sidebarCollapsed: boolean;
      toggleSidebar: () => void;
    }) => unknown,
  ) => {
    const state = {
      sidebarCollapsed: sidebarCollapsedMock.value,
      toggleSidebar: vi.fn(),
    };
    return selector ? selector(state) : state;
  },
}));

import Sidebar from './Sidebar';

function makeUser(role: UserRole): User {
  return { name: 'admin', role };
}

function renderSidebar() {
  return render(
    <MemoryRouter>
      <I18nProvider>
        <Sidebar />
      </I18nProvider>
    </MemoryRouter>,
  );
}

function renderSidebarAt(path: string) {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <I18nProvider>
        <Sidebar />
      </I18nProvider>
    </MemoryRouter>,
  );
}

beforeEach(() => {
  useAuthMock.mockReset();
  useRemoteModeMock.mockReset();
  sidebarCollapsedMock.value = false;
  permissionMock.denied = new Set<string>();
  useAuthMock.mockReturnValue({ user: makeUser('admin') });
  useRemoteModeMock.mockReturnValue({ data: { mode: 'server' } });
});

describe('Sidebar — 접힘 상태 그룹 하위 항목', () => {
  it('접힘 시 원격 그룹의 노드 관리/등록 관리/릴리스 저장소를 개별 아이콘으로 모두 노출한다', () => {
    sidebarCollapsedMock.value = true;
    renderSidebar();
    expect(screen.getByTitle('노드 관리')).toBeInTheDocument();
    expect(screen.getByTitle('등록 관리')).toBeInTheDocument();
    expect(screen.getByTitle('릴리스 저장소')).toBeInTheDocument();
  });
});

describe('Sidebar — 릴리스 저장소 하위 항목', () => {
  it('admin + server 모드이면 릴리스 저장소 링크를 end 매칭으로 노출한다', () => {
    renderSidebarAt('/admin/remote/releases');
    const link = screen.getByRole('link', { name: '릴리스 저장소' });
    expect(link).toHaveAttribute('href', '/admin/remote/releases');
    expect(link).toHaveAttribute('aria-current', 'page');
    // 형제 노드 관리(/admin/remote)는 end 매칭으로 비활성이어야 한다.
    expect(screen.getByRole('link', { name: '노드 관리' })).not.toHaveAttribute(
      'aria-current',
    );
  });
});

describe('Sidebar — 원격 관리 그룹 게이팅', () => {
  it('admin + server 모드이면 원격 관리 그룹을 표시한다', () => {
    renderSidebar();
    expect(screen.getByText('원격 관리')).toBeInTheDocument();
  });

  it.each(['disabled', 'client'] as const)(
    'admin 이어도 mode=%s 이면 원격 관리 그룹을 숨긴다',
    (mode: RemoteMode) => {
      useRemoteModeMock.mockReturnValue({ data: { mode } });
      renderSidebar();
      expect(screen.queryByText('원격 관리')).not.toBeInTheDocument();
    },
  );

  it('모드 미확정(로딩) 중에는 원격 관리 그룹을 숨긴다', () => {
    useRemoteModeMock.mockReturnValue({ data: undefined });
    renderSidebar();
    expect(screen.queryByText('원격 관리')).not.toBeInTheDocument();
  });

  it('server 모드여도 nav.remote 가 없으면 원격 관리 그룹을 숨긴다', () => {
    // SPEC-AUTH-006 M3: 판정 기준이 역할 이름에서 권한 키로 바뀌었다.
    // 커스텀 역할이 생기면 'viewer' 같은 이름 열거는 성립하지 않는다.
    permissionMock.denied = new Set(['nav.remote']);
    useAuthMock.mockReturnValue({ user: makeUser('viewer') });
    renderSidebar();
    expect(screen.queryByText('원격 관리')).not.toBeInTheDocument();
  });
});

describe('Sidebar — 하위 항목 활성 하이라이트', () => {
  // NavLink 는 활성 시 aria-current="page" 를 부여한다. 경로 접두사 매칭으로
  // 형제 메뉴가 동시에 활성되지 않아야 한다(end 매칭).
  it('그룹 관리 경로에서는 그룹 관리만 활성이고 노드 관리는 비활성이다', () => {
    renderSidebarAt('/admin/remote/groups');
    expect(screen.getByRole('link', { name: '그룹 관리' })).toHaveAttribute(
      'aria-current',
      'page',
    );
    expect(screen.getByRole('link', { name: '노드 관리' })).not.toHaveAttribute(
      'aria-current',
    );
  });

  it('등록 관리 경로에서는 노드 관리가 활성으로 표시되지 않는다', () => {
    renderSidebarAt('/admin/remote/enrollment');
    expect(screen.getByRole('link', { name: '노드 관리' })).not.toHaveAttribute(
      'aria-current',
    );
  });

  it('노드 관리 경로에서는 노드 관리만 활성이다', () => {
    renderSidebarAt('/admin/remote');
    expect(screen.getByRole('link', { name: '노드 관리' })).toHaveAttribute(
      'aria-current',
      'page',
    );
  });
});
