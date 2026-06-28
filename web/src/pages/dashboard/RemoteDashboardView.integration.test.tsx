// RemoteDashboardView 통합 테스트 (SPEC-REMOTE-001 M10, 그룹 L) — 응답 형태 robust.
//
// RemoteDashboardView.test.tsx 가 useDashboardConfigTarget 훅을 mock 하는 것과 달리,
// 본 테스트는 *서비스 레이어*(getRemoteDashboard)만 mock 하고 실제 훅(정규화 포함)을
// 통과시켜, 노드 프록시가 RAW base64 형태를 반환할 때도 패널이 실제로 렌더되는지
// 검증한다. useTargetGating 은 노드 ready 로 고정한다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { render, screen, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getRemoteDashboardMock = vi.hoisted(() => vi.fn());
const useTargetGatingMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/remoteService', () => ({
  getRemoteDashboard: getRemoteDashboardMock,
}));
vi.mock('@/hooks/useTargetGating', () => ({
  useTargetGating: useTargetGatingMock,
}));
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));
// GridLayout + 패널 렌더러는 스텁(패널 셀만 검증).
vi.mock('react-grid-layout', () => ({
  default: ({ children }: { children: ReactNode }) => (
    <div data-testid="grid">{children}</div>
  ),
}));
vi.mock('./renderDashboardPanel', () => ({
  renderDashboardPanel: (panel: { id: string; type: string }) => (
    <div data-testid={`panel-${panel.id}`} data-type={panel.type} />
  ),
}));

import RemoteDashboardView from './RemoteDashboardView';

const TARGET = { type: 'remote' as const, instanceId: 'node-1' };

/** 노드 ready 게이팅 고정값. */
const READY_GATING = {
  isRemote: true,
  nodeReady: true,
  nodeLabel: 'host-a',
  canControl: () => true,
};

/** 패널 1개를 가진 대시보드 페이로드. */
const PAYLOAD = {
  dashboardPages: [
    {
      id: 'p1',
      name: 'P1',
      isDefault: true,
      panels: [{ id: 'flows-1', type: 'flows', title: 'F', config: {} }],
      layout: [{ i: 'flows-1', x: 0, y: 0, w: 4, h: 3 }],
    },
  ],
  activeDashboardId: 'p1',
  dashboardGridCols: 10,
  dashboardShowGridLines: false,
  dashboardRefreshInterval: 5,
  deviceGridLayout: {},
};

/** 유니코드 안전 base64 인코딩(노드 RAW 형태 시뮬레이션). */
function toBase64Json(value: unknown): string {
  const json = JSON.stringify(value);
  const bytes = new TextEncoder().encode(json);
  let binary = '';
  for (const b of bytes) binary += String.fromCharCode(b);
  return btoa(binary);
}

function renderWithClient(): void {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <RemoteDashboardView target={TARGET} />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  getRemoteDashboardMock.mockReset();
  useTargetGatingMock.mockReset();
  useTargetGatingMock.mockReturnValue(READY_GATING);
});

describe('RemoteDashboardView — 노드 응답 형태 robust 렌더', () => {
  it('노드가 RAW base64 형태({ Payload })를 반환해도 패널을 렌더한다', async () => {
    getRemoteDashboardMock.mockResolvedValue({
      Scope: 'global',
      Owner: '',
      Version: 1,
      UpdatedAt: 0,
      Payload: toBase64Json(PAYLOAD),
    });

    renderWithClient();

    // 정규화를 거쳐 패널이 실제로 렌더된다(빈 상태 아님).
    await waitFor(() =>
      expect(screen.getByTestId('panel-flows-1')).toBeInTheDocument(),
    );
    expect(screen.queryByTestId('remote-dashboard-empty')).not.toBeInTheDocument();
  });

  it('노드가 DTO 형태({ payload: object })를 반환해도 패널을 렌더한다', async () => {
    getRemoteDashboardMock.mockResolvedValue({
      scope: 'global',
      owner: null,
      version: 1,
      updatedAt: 0,
      payload: PAYLOAD,
    });

    renderWithClient();

    await waitFor(() =>
      expect(screen.getByTestId('panel-flows-1')).toBeInTheDocument(),
    );
  });

  it('노드가 garbage 페이로드를 반환하면 크래시 없이 빈 상태를 표시한다', async () => {
    getRemoteDashboardMock.mockResolvedValue({ Payload: '@@garbage@@' });

    renderWithClient();

    await waitFor(() =>
      expect(screen.getByTestId('remote-dashboard-empty')).toBeInTheDocument(),
    );
    expect(screen.queryByTestId('panel-flows-1')).not.toBeInTheDocument();
    expect(screen.queryByTestId('remote-dashboard-error')).not.toBeInTheDocument();
  });

  it('노드가 truly-null payload(빈 대시보드)를 반환하면 빈 상태를 표시한다', async () => {
    getRemoteDashboardMock.mockResolvedValue({
      scope: 'global',
      owner: null,
      version: 0,
      updatedAt: 0,
      payload: null,
    });

    renderWithClient();

    await waitFor(() =>
      expect(screen.getByTestId('remote-dashboard-empty')).toBeInTheDocument(),
    );
  });
});
