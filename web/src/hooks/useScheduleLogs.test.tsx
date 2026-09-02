// SPEC-SCHEDULE-VIEW-001 M6 — useScheduleLogs / useClearScheduleLogs 훅 테스트.
//
// useScheduleLogs: 기본 limit(25) 을 병합해 getScheduleLogs 를 호출하고 { items, total }
// 결과를 노출하는지 검증한다. useClearScheduleLogs: clearScheduleLogs 를 호출하고 성공 시
// ['schedule-logs'] 쿼리를 invalidate 하는지 검증한다.

import { renderHook, waitFor } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactNode } from 'react';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const getScheduleLogsMock = vi.hoisted(() => vi.fn());
const clearScheduleLogsMock = vi.hoisted(() => vi.fn());

vi.mock('@/services/api/scheduleLogService', () => ({
  getScheduleLogs: getScheduleLogsMock,
  clearScheduleLogs: clearScheduleLogsMock,
}));

import {
  SCHEDULE_LOGS_DEFAULT_LIMIT,
  useClearScheduleLogs,
  useScheduleLogs,
} from './useScheduleLogs';

let queryClient: QueryClient;

function wrapper({ children }: { children: ReactNode }) {
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
}

beforeEach(() => {
  getScheduleLogsMock.mockReset();
  clearScheduleLogsMock.mockReset();
  queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
});

describe('useScheduleLogs', () => {
  it('기본 limit 을 병합해 { items, total } 을 노출한다', async () => {
    getScheduleLogsMock.mockResolvedValueOnce({ items: [{ id: 1 }], total: 3 });

    const { result } = renderHook(() => useScheduleLogs({ scheduleId: 'sch-1' }), { wrapper });

    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(getScheduleLogsMock).toHaveBeenCalledWith({
      limit: SCHEDULE_LOGS_DEFAULT_LIMIT,
      scheduleId: 'sch-1',
    });
    expect(result.current.data).toEqual({ items: [{ id: 1 }], total: 3 });
  });
});

describe('useClearScheduleLogs', () => {
  it('clearScheduleLogs 를 호출하고 성공 시 로그 쿼리를 invalidate 한다', async () => {
    clearScheduleLogsMock.mockResolvedValueOnce(undefined);
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries');

    const { result } = renderHook(() => useClearScheduleLogs(), { wrapper });

    await result.current.mutateAsync();

    expect(clearScheduleLogsMock).toHaveBeenCalledTimes(1);
    await waitFor(() =>
      expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['schedule-logs'] }),
    );
  });
});
