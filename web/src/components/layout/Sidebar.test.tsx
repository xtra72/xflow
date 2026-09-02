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
// `denyAll` 은 권한이 **하나도 없는** 사용자를 표현한다. 부정 목록만으로는
// "카탈로그에 있는 모든 키를 나열했는가" 에 의존하게 되어, 키가 늘어나면 조용히
// 허용으로 바뀐다. SPEC-DASHBOARD-004 AC-10 의 회귀 가드가 이 상태를 요구한다.
const permissionMock = vi.hoisted(() => ({
  denied: new Set<string>(),
  denyAll: false,
}));
vi.mock('@/hooks/usePermission', () => {
  const allow = (key: string) => !permissionMock.denyAll && !permissionMock.denied.has(key);
  return {
    usePermission: () => ({
      hasPermission: allow,
      hasAnyPermission: (keys: readonly string[]) => keys.some(allow),
      // SPEC-AUTH-006 E2: 메뉴 노출은 nav.* 만 본다.
      canSeeMenu: allow,
      isPermissionUnavailable: false,
    }),
  };
});

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
  permissionMock.denyAll = false;
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

describe('Sidebar — 대시보드 메뉴 (SPEC-DASHBOARD-004)', () => {
  // 별도 '대시보드 관리' 항목은 제거되었다(M7 재작업). 관리 어포던스는 대시보드
  // 편집(설정) 모드 안으로 들어갔고 `nav.dashboard` 가 거기서 노출을 판정한다
  // (DashboardSettingsSelector.test.tsx). 사이드바에는 그 항목이 다시 생기면
  // 안 된다 — 두 곳에 관리 진입점이 생기면 게이팅 판정 사본이 둘이 된다.
  it('대시보드 관리 항목은 사이드바에 존재하지 않는다', () => {
    renderSidebar();
    expect(
      screen.queryByRole('link', { name: '대시보드 관리' }),
    ).not.toBeInTheDocument();
  });

  // spec.md §2.5 회귀 가드 — 대시보드를 *보는* 것은 인증만 요구한다.
  // 기존 대시보드 항목에 permission/navPermission 이 붙는 순간 권한 0개 사용자가
  // 빈 사이드바를 보게 되고, 그것이 catalog.go 가 금지한 상태다.
  it('권한이 0개인 사용자에게도 대시보드(보기) 항목은 남는다', () => {
    permissionMock.denyAll = true;
    renderSidebar();
    const link = screen.getByRole('link', { name: '대시보드' });
    expect(link).toHaveAttribute('href', '/');
  });

  // nav.dashboard 를 거부해도 보기 항목은 영향을 받지 않는다 — 보기 항목에는
  // 애초에 키가 붙어 있지 않다. 두 축이 섞이지 않았음을 고정한다.
  it('nav.dashboard 가 없어도 대시보드(보기) 항목은 남는다', () => {
    permissionMock.denied = new Set(['nav.dashboard']);
    renderSidebar();
    expect(screen.getByRole('link', { name: '대시보드' })).toHaveAttribute('href', '/');
  });

  it('권한이 0개여도 사이드바가 비지 않는다 — 링크가 최소 1개 남는다', () => {
    permissionMock.denyAll = true;
    renderSidebar();
    expect(screen.getAllByRole('link').length).toBeGreaterThanOrEqual(1);
  });
});
