// useTsdbChartData 훅 테스트 — 폴링/재구독/보존 규칙 + 부분 실패 신호 + 백엔드 불일치.
//
// queryTsdbFn 을 주입해 네트워크 없이 매트릭스를 시뮬레이션한다(useStoreChartData
// 테스트와 같은 패턴).
//
// @spec SPEC-TSDB-002 §2.14 (S2) · §2.18 (U11) · §2.19 (U12)

import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

/** 마이크로태스크 큐를 비워 resolved/rejected 프로미스 핸들러를 실행시킨다. */
async function flushMicrotasks(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

import type { SeriesMatrix } from '@/services/api/seriesDataSource';
import type { TsdbMatrixResult } from '@/services/api/tsdbSource';

import type { TsdbSourceConfig } from './chartChannelTypes';
import {
  useTsdbChartData,
  type QueryTsdbMatrixFn,
  type UseTsdbChartDataOptions,
  type UseTsdbChartDataResult,
} from './useTsdbChartData';

/** 현재 에이전트 목록. 각 테스트가 필요 시 바꿔 백엔드 파생을 제어한다. */
let mockAgents: Array<{ id: string; name: string; type: string }> = [];

/**
 * 에이전트 목록을 미리 심어 둔 QueryClient 로 감싼다.
 * `staleTime: Infinity` 라 네트워크 조회가 발생하지 않는다.
 */
function wrapper({ children }: { children: ReactNode }) {
  const client = new QueryClient({
    defaultOptions: { queries: { staleTime: Infinity, retry: false } },
  });
  client.setQueryData(['agents', undefined], {
    data: mockAgents,
    total: mockAgents.length,
  });
  return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
}

/** 훅을 Provider 안에서 렌더한다. */
function renderTsdb(
  config: TsdbSourceConfig | undefined,
  enabled: boolean,
  options: UseTsdbChartDataOptions,
) {
  return renderHook<UseTsdbChartDataResult, unknown>(
    () => useTsdbChartData(config, enabled, options),
    { wrapper },
  );
}

function makeConfig(overrides: Partial<TsdbSourceConfig> = {}): TsdbSourceConfig {
  return {
    backend: 'influxdb',
    agent_id: 'a-1',
    agent_name: 'ix',
    series: [{ key: 'cpu', field: 'usage', alias: 'CPU' }],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
    ...overrides,
  };
}

const matrix: SeriesMatrix = {
  columns: ['cpu'],
  rows: [
    { bucketStartMs: 1000, values: [21.5] },
    { bucketStartMs: 2000, values: [22.0] },
  ],
};

function ok(m: SeriesMatrix = matrix): TsdbMatrixResult {
  return { matrix: m, failures: [] };
}

beforeEach(() => {
  vi.useFakeTimers();
  mockAgents = [{ id: 'a-1', name: 'ix', type: 'influxdb' }];
});
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
});

describe('useTsdbChartData — 비활성/무효 설정은 조회하지 않는다', () => {
  it('enabled=false 이면 idle 을 유지한다', () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>();
    const { result } = renderTsdb(makeConfig(), false, { queryTsdbFn: queryFn });
    expect(result.current.status).toBe('idle');
    expect(queryFn).not.toHaveBeenCalled();
  });

  it('series 가 비어있으면 idle 을 유지한다', () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>();
    const { result } = renderTsdb(makeConfig({ series: [] }), true, { queryTsdbFn: queryFn });
    expect(result.current.status).toBe('idle');
    expect(queryFn).not.toHaveBeenCalled();
  });

  it('조회 창(time_window_ms · interval_ms)이 없으면 idle 을 유지한다', () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>();
    const partial = {
      backend: 'influxdb',
      agent_name: 'ix',
      series: [{ key: 'cpu', field: 'usage' }],
    } as unknown as TsdbSourceConfig;
    const { result } = renderTsdb(partial, true, { queryTsdbFn: queryFn });
    expect(result.current.status).toBe('idle');
    expect(queryFn).not.toHaveBeenCalled();
  });

  it('에이전트 이름이 비어 있으면 idle 을 유지한다', () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>();
    const { result } = renderTsdb(makeConfig({ agent_id: undefined, agent_name: '' }), true, {
        queryTsdbFn: queryFn,
      });
    expect(result.current.status).toBe('idle');
    expect(queryFn).not.toHaveBeenCalled();
  });
});

describe('useTsdbChartData — 폴링과 변환', () => {
  it('활성화 시 즉시 1회 조회하고 connected 로 entries 를 채운다', async () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue(ok());
    const { result } = renderTsdb(makeConfig(), true, {
        queryTsdbFn: queryFn,
        nowFn: () => 100_000,
      });

    expect(queryFn).toHaveBeenCalledTimes(1);
    const call = queryFn.mock.calls[0]!;
    expect(call[0]).toMatchObject({ agent_id: 'a-1', agent_name: 'ix' });
    expect(call[2]).toMatchObject({
      keys: ['cpu'],
      seriesFilters: [{ fieldName: 'usage' }],
      startMs: 40_000,
      endMs: 100_000,
      intervalMs: 10_000,
      aggregation: 'average',
    });

    await flushMicrotasks();
    expect(result.current.status).toBe('connected');
    expect(result.current.seriesNames).toEqual(['CPU']);
    expect(result.current.entries.map((e) => e.value)).toEqual([21.5, 22.0]);
    expect(result.current.partialFailureCount).toBe(0);
  });

  it('bucket 을 조회 지시자에 실어 전달한다', async () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue(ok());
    renderTsdb(makeConfig({ bucket: 'metrics' }), true, {
        queryTsdbFn: queryFn,
        nowFn: () => 100_000,
      });
    await flushMicrotasks();
    expect(queryFn.mock.calls[0]![0]).toMatchObject({ bucket: 'metrics' });
  });

  it('refresh_interval_ms 주기로 폴링한다', async () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue(ok());
    renderTsdb(makeConfig({ refresh_interval_ms: 5_000 }), true, {
        queryTsdbFn: queryFn,
      });
    await flushMicrotasks();
    expect(queryFn).toHaveBeenCalledTimes(1);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    await flushMicrotasks();
    expect(queryFn).toHaveBeenCalledTimes(2);
  });

  it('언마운트 시 폴링을 멈춘다', async () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue(ok());
    const { unmount } = renderTsdb(makeConfig(), true, { queryTsdbFn: queryFn });
    await flushMicrotasks();
    unmount();
    await act(async () => {
      vi.advanceTimersByTime(20_000);
    });
    expect(queryFn).toHaveBeenCalledTimes(1);
  });
});

describe('useTsdbChartData — 부분 실패 신호 (AC-39 · AC-40)', () => {
  it('일부 컬럼이 실패하면 성공 시리즈를 렌더하고 실패 개수를 보고한다', async () => {
    const queryFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue({
      matrix,
      failures: [{ index: 1, error: new Error('boom') }],
    });
    const { result } = renderTsdb(makeConfig(), true, { queryTsdbFn: queryFn });
    await flushMicrotasks();

    // 부분 실패는 오류가 아니라 별도 신호다(§2.19).
    expect(result.current.status).toBe('connected');
    expect(result.current.partialFailureCount).toBe(1);
    expect(result.current.entries.length).toBe(2);
  });

  it('부분 실패 복구 시 배지가 제거된다', async () => {
    const queryFn = vi
      .fn<QueryTsdbMatrixFn>()
      .mockResolvedValueOnce({ matrix, failures: [{ index: 0, error: new Error('x') }] })
      .mockResolvedValue(ok());
    const { result } = renderTsdb(makeConfig({ refresh_interval_ms: 5_000 }), true, {
        queryTsdbFn: queryFn,
      });
    await flushMicrotasks();
    expect(result.current.partialFailureCount).toBe(1);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    await flushMicrotasks();
    expect(result.current.partialFailureCount).toBe(0);
  });
});

describe('useTsdbChartData — 오류와 백엔드 불일치', () => {
  it('404 수신 시 마지막 성공 렌더를 보존하고 폴링 주기를 초과 호출하지 않는다', async () => {
    const queryFn = vi
      .fn<QueryTsdbMatrixFn>()
      .mockResolvedValueOnce(ok())
      .mockRejectedValue(new Error('agent_not_found'));
    const { result } = renderTsdb(makeConfig({ refresh_interval_ms: 5_000 }), true, {
        queryTsdbFn: queryFn,
      });
    await flushMicrotasks();
    expect(result.current.entries.length).toBe(2);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    await flushMicrotasks();

    expect(result.current.status).toBe('error');
    expect(result.current.errorReason).toContain('agent_not_found');
    // 마지막 성공 렌더는 파괴되지 않는다(§2.14).
    expect(result.current.entries.length).toBe(2);

    // 무한 재시도하지 않는다 — 폴링 주기당 정확히 1회다(§2.17-3).
    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    await flushMicrotasks();
    expect(queryFn).toHaveBeenCalledTimes(3);
  });

  it('참조된 에이전트가 지원 백엔드가 아니면 불일치 오류를 표시하고 질의하지 않는다', async () => {
    mockAgents = [{ id: 'a-1', name: 'ix', type: 'store' }];
    const queryFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue(ok());
    const { result } = renderTsdb(makeConfig(), true, { queryTsdbFn: queryFn });
    await flushMicrotasks();

    expect(result.current.status).toBe('error');
    expect(result.current.backendMismatch).toBe(true);
    expect(queryFn).not.toHaveBeenCalled();

    // 폴링 타이머도 돌지 않는다 — 조용한 재질의가 없다.
    await act(async () => {
      vi.advanceTimersByTime(20_000);
    });
    expect(queryFn).not.toHaveBeenCalled();
  });
});

describe('useTsdbChartData — group by 시리즈 렌더 (버그 재현)', () => {
  it('group by 항목 1개가 그룹 2개로 펼쳐지면 시리즈 2개가 나와야 한다', async () => {
    // 실제 경로와 같은 형상: 컬럼 2개 · 출처 인덱스는 둘 다 0 · 라벨에 그룹 태그.
    const grouped: SeriesMatrix = {
      columns: ['cpu{host=a}', 'cpu{host=b}'],
      rows: [
        { bucketStartMs: 1000, values: [1, 2] },
        { bucketStartMs: 2000, values: [3, 4] },
      ],
      columnOrigins: [0, 0],
      columnLabels: [
        { __field__: 'usage', host: 'a' },
        { __field__: 'usage', host: 'b' },
      ],
    };
    const queryFn = vi.fn(async () => ok(grouped));
    const { result } = renderTsdb(
      makeConfig({ series: [{ key: 'cpu', field: 'usage', group_by: ['host'] }] }),
      true,
      { queryTsdbFn: queryFn },
    );

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(result.current.status).toBe('connected');
    // 그룹 2개가 각각 독립 시리즈로 나와야 한다.
    expect(result.current.seriesNames).toHaveLength(2);
    expect(new Set(result.current.seriesNames).size).toBe(2);
    expect(result.current.entries.length).toBeGreaterThan(0);
  });

  it('group_by 가 요청 필터로 전달된다', async () => {
    const queryFn = vi.fn(async () => ok());
    renderTsdb(
      makeConfig({ series: [{ key: 'cpu', field: 'usage', group_by: ['host'] }] }),
      true,
      { queryTsdbFn: queryFn },
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    const call = queryFn.mock.calls[0] as unknown as [
      unknown,
      unknown,
      { seriesFilters?: Array<Record<string, unknown>> },
    ];
    expect(call[2].seriesFilters?.[0]?.groupBy).toEqual(['host']);
  });
});

describe('useTsdbChartData — group_by 변경이 재조회를 촉발해야 한다 (버그 재현)', () => {
  it('group_by 를 나중에 걸면 다시 질의한다', async () => {
    const queryFn = vi.fn(async () => ok());
    const { rerender } = renderHook<UseTsdbChartDataResult, { cfg: TsdbSourceConfig }>(
      ({ cfg }) => useTsdbChartData(cfg, true, { queryTsdbFn: queryFn }),
      {
        wrapper,
        initialProps: { cfg: makeConfig({ series: [{ key: 'cpu', field: 'usage' }] }) },
      },
    );
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    const before = queryFn.mock.calls.length;
    expect(before).toBeGreaterThan(0);

    // 그룹 축만 바꾼다 — 사용자가 설정 화면에서 그룹 기준을 체크한 상황.
    rerender({
      cfg: makeConfig({ series: [{ key: 'cpu', field: 'usage', group_by: ['host'] }] }),
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    expect(queryFn.mock.calls.length).toBeGreaterThan(before);
    const last = queryFn.mock.calls[queryFn.mock.calls.length - 1] as unknown as [
      unknown,
      unknown,
      { seriesFilters?: Array<Record<string, unknown>> },
    ];
    expect(last[2].seriesFilters?.[0]?.groupBy).toEqual(['host']);
  });
});
