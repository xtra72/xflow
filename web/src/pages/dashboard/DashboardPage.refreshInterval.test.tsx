// DashboardPage 헤더 갱신 주기 드롭다운 테스트.
//
// 검증:
//   - 일반(읽기) 모드 헤더에 갱신 주기 셀렉터가 렌더되고 현재 값(초)을 표시한다.
//   - 항목 선택 시 store 의 setDashboardRefreshInterval 이 호출되어 값이 반영된다.
//
// 무거운 데이터 훅(useFlows/useWebSocket/useDashboardSync)과 GridLayout/패널 렌더러는
// 스텁으로 대체하고, UI store 는 영속 싱글톤(useUIStore)을 실제로 사용한다
// (RemoteDashboardView.test.tsx 패턴 동일).

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { fireEvent, render, screen, within } from '@testing-library/react';
import type { PropsWithChildren } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

// 데이터 훅 스텁 — 결정적 반환값.
vi.mock('@/hooks', () => ({
  useFlows: () => ({ data: { data: [] }, isLoading: false, error: null }),
  useWebSocket: () => ({ state: 'connected', client: null }),
}));
vi.mock('@/hooks/useDashboardSync', () => ({
  useDashboardSync: () => ({ pendingSync: false }),
}));
vi.mock('@/services/api/monitorService', () => ({
  getMetrics: vi.fn().mockResolvedValue(null),
}));
// GridLayout + 패널 렌더러는 스텁(헤더만 검증하므로 본문은 불필요).
vi.mock('react-grid-layout', () => ({
  default: ({ children }: { children: React.ReactNode }) => (
    <div data-testid="grid">{children}</div>
  ),
}));
vi.mock('./renderDashboardPanel', () => ({
  renderDashboardPanel: () => null,
}));

// authStore — 일반 사용자(admin) 주입. selector/직접 호출 모두 지원.
const authState = { user: { name: 'admin', role: 'admin' as const } };
vi.mock('@/stores/authStore', () => ({
  useAuthStore: Object.assign(
    (selector?: (state: typeof authState) => unknown) =>
      selector ? selector(authState) : authState,
    { getState: () => authState },
  ),
}));

import { useUIStore } from '@/stores/uiStore';

import DashboardPage from './DashboardPage';

function renderPage() {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  function Wrapper({ children }: PropsWithChildren) {
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  }
  return render(<DashboardPage />, { wrapper: Wrapper });
}

beforeEach(() => {
  // 영속 store 싱글톤 — 각 테스트 전에 기본값으로 초기화.
  useUIStore.getState().setDashboardRefreshInterval(10);
  useUIStore.getState().setDashboardEditMode(false);
});

describe('DashboardPage 갱신 주기 드롭다운', () => {
  it('헤더에 현재 갱신 주기를 표시한다', () => {
    renderPage();
    expect(screen.getByRole('button', { name: '갱신 주기' })).toHaveTextContent('10초');
  });

  it('항목 선택 시 store 갱신 주기가 변경된다', () => {
    renderPage();

    fireEvent.click(screen.getByRole('button', { name: '갱신 주기' }));

    const listbox = screen.getByRole('listbox', { name: '갱신 주기 선택' });
    fireEvent.click(within(listbox).getByRole('option', { name: '30초' }));

    expect(useUIStore.getState().dashboardRefreshInterval).toBe(30);
    // 선택 후 드롭다운이 닫힌다.
    expect(screen.queryByRole('listbox', { name: '갱신 주기 선택' })).not.toBeInTheDocument();
  });
});
