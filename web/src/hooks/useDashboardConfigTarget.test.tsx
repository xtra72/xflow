// useDashboardConfigTarget / useMetricsTarget 테스트 (SPEC-REMOTE-001 M10, 그룹 L).
//
// 검증:
//   - 로컬 target: 원격 config 를 fetch 하지 않는다(payload undefined, sync 없음).
//   - 원격 target: getRemoteDashboard(scope) 로 READ-ONLY config 를 취득한다.
//   - useMetricsTarget: 로컬은 getMetrics, 원격은 getRemoteMetrics 를 소스로 쓴다.

import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { renderHook, waitFor } from '@testing-library/react';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getRemoteDashboardMock = vi.hoisted(() => vi.fn());
const getRemoteMetricsMock = vi.hoisted(() => vi.fn());
const getMetricsMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/remoteService', () => ({
  getRemoteDashboard: getRemoteDashboardMock,
  getRemoteMetrics: getRemoteMetricsMock,
}));
vi.mock('@/services/api/monitorService', () => ({
  getMetrics: getMetricsMock,
}));

import { LOCAL_TARGET } from '@/lib/remote/target';

import { useDashboardConfigTarget } from './useDashboardConfigTarget';
import { useMetricsTarget } from './useMetricsTarget';

function wrapper() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  );
}

beforeEach(() => {
  getRemoteDashboardMock.mockReset();
  getRemoteMetricsMock.mockReset();
  getMetricsMock.mockReset();
});

describe('useDashboardConfigTarget', () => {
  it('로컬 target 이면 원격 config 를 fetch 하지 않는다(READ-ONLY 없음)', () => {
    const { result } = renderHook(
      () => useDashboardConfigTarget(LOCAL_TARGET, 'shared'),
      { wrapper: wrapper() },
    );
    expect(getRemoteDashboardMock).not.toHaveBeenCalled();
    expect(result.current.payload).toBeUndefined();
  });

  it('원격 target 이면 getRemoteDashboard(scope) 로 config 를 READ-ONLY 취득한다', async () => {
    const payload = { dashboardPages: [], activeDashboardId: '', dashboardGridCols: 10 };
    getRemoteDashboardMock.mockResolvedValueOnce({ payload });

    const { result } = renderHook(
      () =>
        useDashboardConfigTarget({ type: 'remote', instanceId: 'node-1' }, 'mine', true),
      { wrapper: wrapper() },
    );

    await waitFor(() => expect(result.current.payload).toEqual(payload));
    expect(getRemoteDashboardMock).toHaveBeenCalledWith('node-1', 'mine');
  });

  it('enabled=false 면 원격 config 를 fetch 하지 않는다(게이팅)', () => {
    renderHook(
      () =>
        useDashboardConfigTarget({ type: 'remote', instanceId: 'node-1' }, 'shared', false),
      { wrapper: wrapper() },
    );
    expect(getRemoteDashboardMock).not.toHaveBeenCalled();
  });
});

describe('useMetricsTarget', () => {
  it('로컬 target 이면 getMetrics 를 소스로 쓴다(getRemoteMetrics 미호출)', async () => {
    getMetricsMock.mockResolvedValueOnce({ cpu: 5 });
    const { result } = renderHook(() => useMetricsTarget(LOCAL_TARGET, 1000), {
      wrapper: wrapper(),
    });
    await waitFor(() => expect(result.current.metrics).toEqual({ cpu: 5 }));
    expect(getRemoteMetricsMock).not.toHaveBeenCalled();
  });

  it('원격 target 이면 getRemoteMetrics 를 소스로 쓴다(getMetrics 미호출)', async () => {
    getRemoteMetricsMock.mockResolvedValueOnce({ cpu: 9 });
    const { result } = renderHook(
      () => useMetricsTarget({ type: 'remote', instanceId: 'node-1' }, 1000, true),
      { wrapper: wrapper() },
    );
    await waitFor(() => expect(result.current.metrics).toEqual({ cpu: 9 }));
    expect(getRemoteMetricsMock).toHaveBeenCalledWith('node-1');
    expect(getMetricsMock).not.toHaveBeenCalled();
  });
});
