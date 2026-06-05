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

// ---- useRemoteMode mock ----
const useRemoteModeMock = vi.hoisted(() => vi.fn());
vi.mock('@/hooks/useRemote', () => ({ useRemoteMode: useRemoteModeMock }));

// ---- uiStore mock (사이드바 접힘 상태) ----
vi.mock('@/stores/uiStore', () => ({
  useUIStore: (
    selector?: (s: {
      sidebarCollapsed: boolean;
      toggleSidebar: () => void;
    }) => unknown,
  ) => {
    const state = { sidebarCollapsed: false, toggleSidebar: vi.fn() };
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

beforeEach(() => {
  useAuthMock.mockReset();
  useRemoteModeMock.mockReset();
  useAuthMock.mockReturnValue({ user: makeUser('admin') });
  useRemoteModeMock.mockReturnValue({ data: { mode: 'server' } });
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

  it('server 모드여도 비-admin 사용자에게는 원격 관리 그룹을 숨긴다', () => {
    useAuthMock.mockReturnValue({ user: makeUser('viewer') });
    renderSidebar();
    expect(screen.queryByText('원격 관리')).not.toBeInTheDocument();
  });
});
