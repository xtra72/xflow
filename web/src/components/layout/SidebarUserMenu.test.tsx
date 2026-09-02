// SidebarUserMenu 단위 테스트 (SPEC-AUTH-006).
//
// 핵심 회귀: 메뉴가 대시보드 하나만 남은 사용자는 로그아웃할 방법이 없었다.
// Header 가 대시보드 라우트에서 null 을 반환하고 사용자 메뉴가 거기에만 있었기
// 때문이다. 사이드바는 어떤 권한 조합에서도 렌더되므로 여기로 옮겼다.

import { fireEvent, render, screen } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

import { I18nProvider } from '@/lib/i18n';

const logoutMock = vi.hoisted(() => vi.fn());
const authMock = vi.hoisted(() => ({
  user: { name: 'kim', role: 'viewer' } as { name: string; role: string } | null,
  authEnabled: true as boolean | null,
}));
vi.mock('@/hooks/useAuth', () => ({
  useAuth: () => ({ ...authMock, logout: logoutMock }),
}));
vi.mock('@/pages/auth/ChangePasswordDialog', () => ({
  default: ({ open }: { open: boolean }) =>
    open ? <div data-testid="password-dialog" /> : null,
}));

import SidebarUserMenu from './SidebarUserMenu';

function renderMenu(collapsed = false) {
  return render(
    <I18nProvider>
      <SidebarUserMenu collapsed={collapsed} />
    </I18nProvider>,
  );
}

beforeEach(() => {
  logoutMock.mockReset();
  authMock.user = { name: 'kim', role: 'viewer' };
  authMock.authEnabled = true;
});

describe('SidebarUserMenu — 로그아웃 경로', () => {
  it('대시보드만 보이는 사용자도 사이드바에서 로그아웃할 수 있다', () => {
    renderMenu();
    fireEvent.click(screen.getByTestId('sidebar-user-menu-trigger'));
    fireEvent.click(screen.getByText('로그아웃'));
    expect(logoutMock).toHaveBeenCalledTimes(1);
  });

  it('비밀번호 변경도 같은 메뉴에서 연다', () => {
    renderMenu();
    fireEvent.click(screen.getByTestId('sidebar-user-menu-trigger'));
    fireEvent.click(screen.getByText('비밀번호 변경'));
    expect(screen.getByTestId('password-dialog')).toBeInTheDocument();
  });

  it('사이드바가 접혀도 트리거는 남는다 — 접힘 상태에서 로그아웃이 막히면 안 된다', () => {
    renderMenu(true);
    expect(screen.getByTestId('sidebar-user-menu-trigger')).toBeInTheDocument();
  });

  it('인증 비활성 배포에서는 로그아웃 대신 사용자 정보만 표시한다', () => {
    authMock.authEnabled = false;
    renderMenu();
    expect(screen.getByTestId('sidebar-user-static')).toBeInTheDocument();
    expect(screen.queryByTestId('sidebar-user-menu-trigger')).toBeNull();
  });

  it('로그인 사용자가 없으면 아무것도 렌더하지 않는다', () => {
    authMock.user = null;
    const { container } = renderMenu();
    expect(container).toBeEmptyDOMElement();
  });
});
