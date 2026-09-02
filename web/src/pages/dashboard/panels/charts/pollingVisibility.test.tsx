// 차트 폴링이 실제로 가시성을 따르는지 — 훅 통합 검증.
//
// 유틸 자체는 visiblePolling.test.ts 가 본다. 여기서는 훅이 그 유틸을 **쓰고
// 있는지** 를 본다 — 유틸만 맞고 배선이 빠지면 아무것도 달라지지 않는다.

import { renderHook, act } from '@testing-library/react';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';

const mocks = vi.hoisted(() => ({ queryTsdbSourceMatrix: vi.fn(), fetchStoreKeys: vi.fn() }));

vi.mock('@/services/api/tsdbSource', async () => {
  const actual =
    await vi.importActual<typeof import('@/services/api/tsdbSource')>('@/services/api/tsdbSource');
  return { ...actual, queryTsdbSourceMatrix: mocks.queryTsdbSourceMatrix };
});
vi.mock('@/hooks/useAgent', () => ({ useAgents: () => ({ data: { data: [] } }) }));

import type { TsdbSourceConfig } from './chartChannelTypes';
import { useTsdbChartData } from './useTsdbChartData';

const source: TsdbSourceConfig = {
  backend: 'influxdb',
  agent_id: 'a-1',
  agent_name: 'ix',
  series: [{ key: 'cpu', field: 'usage' }],
  time_window_ms: 3_600_000,
  interval_ms: 60_000,
  aggregation: 'average',
  refresh_interval_ms: 1_000,
};

/** document.hidden 을 갈아끼우고 visibilitychange 를 쏜다. */
function setHidden(hidden: boolean) {
  Object.defineProperty(document, 'hidden', { configurable: true, get: () => hidden });
  document.dispatchEvent(new Event('visibilitychange'));
}

beforeEach(() => {
  vi.useFakeTimers();
  mocks.queryTsdbSourceMatrix.mockReset();
  mocks.queryTsdbSourceMatrix.mockResolvedValue({
    matrix: { columns: [], rows: [], columnOrigins: [], columnLabels: [] },
    failures: [],
  });
  setHidden(false);
});

afterEach(() => {
  vi.useRealTimers();
  setHidden(false);
});

describe('차트 폴링과 문서 가시성', () => {
  it('보이는 동안에는 주기마다 조회한다', () => {
    renderHook(() => useTsdbChartData(source, true));
    const initial = mocks.queryTsdbSourceMatrix.mock.calls.length;

    act(() => {
      vi.advanceTimersByTime(3_000);
    });
    expect(mocks.queryTsdbSourceMatrix.mock.calls.length).toBeGreaterThan(initial);
  });

  it('탭이 숨으면 조회가 멈춘다 — 아무도 보지 않는 질의를 내지 않는다', () => {
    renderHook(() => useTsdbChartData(source, true));

    act(() => {
      setHidden(true);
    });
    const afterHide = mocks.queryTsdbSourceMatrix.mock.calls.length;

    act(() => {
      vi.advanceTimersByTime(5_000);
    });
    expect(mocks.queryTsdbSourceMatrix.mock.calls.length).toBe(afterHide);
  });

  it('다시 보이면 즉시 조회한다 — 옛 값이 떠 있는 시간을 없앤다', () => {
    renderHook(() => useTsdbChartData(source, true));
    act(() => {
      setHidden(true);
    });
    const afterHide = mocks.queryTsdbSourceMatrix.mock.calls.length;

    act(() => {
      setHidden(false);
    });
    // 인터벌을 기다리지 않고 곧바로 한 번.
    expect(mocks.queryTsdbSourceMatrix.mock.calls.length).toBe(afterHide + 1);
  });
});
