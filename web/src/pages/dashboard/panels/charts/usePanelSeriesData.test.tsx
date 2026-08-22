// usePanelSeriesData — 소스 종류 키잉 시리즈 조회 훅 테스트.
//
// queryMatrixFn 을 주입해 네트워크 없이 store 조회를 시뮬레이션한다. TSDB 분기는
// M3 시점에 idle 스텁이므로 "조회가 일어나지 않는다" 를 잠근다(M6.8 배선 지점).
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4)

import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';

/** 마이크로태스크 큐를 비워 resolved 프로미스 핸들러를 실행시킨다. */
async function flushMicrotasks(): Promise<void> {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'store-1', name: 'store-1', type: 'store' }] } }),
}));

import type { SeriesMatrix } from '@/services/api/seriesDataSource';
import type { StoreSourceConfig, TsdbSourceConfig } from './chartChannelTypes';
import type { QueryMatrixFn, ResolveKeysFn } from './useStoreChartData';
import type { QueryTsdbMatrixFn } from './useTsdbChartData';
import { isPanelSeriesSource, usePanelSeriesData } from './usePanelSeriesData';
import { resolvePanelSourceBinding } from './panelDataSource';
import { resolvePanelSeriesDisplay } from './panelSeriesStatus';

const matrix: SeriesMatrix = {
  columns: ['room:temp'],
  rows: [
    { bucketStartMs: 1000, values: [21.5] },
    { bucketStartMs: 2000, values: [22.0] },
  ],
};

let queryFn: QueryMatrixFn;
let keysFn: ResolveKeysFn;

beforeEach(() => {
  queryFn = vi.fn(async () => matrix);
  keysFn = vi.fn(async () => ['room:temp']);
});

function storeBlock(overrides: Partial<StoreSourceConfig> = {}): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    namespace: 'default',
    series: [{ key: 'room:temp', alias: 'Temp' }],
    time_window_ms: 60_000,
    interval_ms: 10_000,
    aggregation: 'average',
    refresh_interval_ms: 5_000,
    ...overrides,
  };
}

function render(config: Record<string, unknown>, storeSourceOverride?: StoreSourceConfig) {
  return renderHook(() =>
    usePanelSeriesData(config, {
      storeSourceOverride,
      storeOptions: { queryMatrixFn: queryFn, resolveKeysFn: keysFn },
    }),
  );
}

describe('usePanelSeriesData — 반환 형상', () => {
  it('UseStoreChartDataResult 형상을 그대로 반환한다(패널 소비 코드 무변경의 근거)', () => {
    const { result } = render({});
    expect(Object.keys(result.current).sort()).toEqual(
      ['booleanSeries', 'entries', 'seriesEntries', 'seriesNames', 'seriesStyles', 'status'].sort(),
    );
    expect(result.current.seriesEntries).toBeInstanceOf(Map);
    expect(result.current.seriesStyles).toBeInstanceOf(Map);
    expect(result.current.booleanSeries).toBeInstanceOf(Set);
  });
});

describe('usePanelSeriesData — 비활성 경로는 idle 이며 조회하지 않는다', () => {
  it.each([
    ['config 부재', {}],
    ['data_source:channel', { data_source: 'channel' }],
    ['store + store_source 부재', { data_source: 'store' }],
    ['store + series 0', { data_source: 'store', store_source: storeBlock({ series: [] }) }],
    ['인식 불가 문자열', { data_source: 'wat', store_source: storeBlock() }],
  ])('%s → idle', async (_label, config) => {
    const { result } = render(config as Record<string, unknown>);
    await flushMicrotasks();
    expect(result.current.status).toBe('idle');
    expect(result.current.entries).toEqual([]);
    expect(queryFn).not.toHaveBeenCalled();
  });
});

describe('usePanelSeriesData — store 분기', () => {
  it('series N개면 store 훅으로 조회하고 결과를 그대로 흘린다', async () => {
    const { result } = render({ data_source: 'store', store_source: storeBlock() });
    await flushMicrotasks();
    expect(queryFn).toHaveBeenCalledTimes(1);
    expect(result.current.status).toBe('connected');
    expect(result.current.seriesNames).toEqual(['Temp']);
    expect(result.current.entries.length).toBe(2);
  });

  it('tag 모드(series 0 + tag_filters N)도 활성이다', async () => {
    const { result } = render({
      data_source: 'store',
      store_source: storeBlock({
        series: [],
        selection_mode: 'tag',
        tag_filters: { room: '1' },
      }),
    });
    await flushMicrotasks();
    expect(keysFn).toHaveBeenCalledTimes(1);
    await flushMicrotasks();
    expect(result.current.status).not.toBe('idle');
  });

  it('storeSourceOverride 를 주면 config.store_source 대신 그것으로 조회한다(히트맵 경로)', async () => {
    const override = storeBlock({ series: [{ key: 'room:temp', alias: 'SENSOR#1' }] });
    const { result } = render(
      { data_source: 'store', store_source: storeBlock({ series: [] }) },
      override,
    );
    await flushMicrotasks();
    // config.store_source 만 봤다면 series 0 이라 비활성이었을 것이다.
    expect(queryFn).toHaveBeenCalledTimes(1);
    expect(result.current.seriesNames).toEqual(['SENSOR#1']);
  });
});

describe('usePanelSeriesData — tsdb 분기는 M3 에서 idle 스텁이다', () => {
  it('tsdb 활성 config 라도 store 훅으로 조회하지 않고 idle 을 반환한다(M6.8 배선 지점)', async () => {
    const { result } = render({
      data_source: 'tsdb',
      tsdb_source: {
        backend: 'influxdb',
        agent_name: 'influx-1',
        series: [{ measurement: 'm', field: 'f' }],
      },
      // store 블록이 함께 있어도 tsdb 를 고른 이상 store 로 새지 않는다.
      store_source: storeBlock(),
    });
    await flushMicrotasks();
    expect(result.current.status).toBe('idle');
    expect(queryFn).not.toHaveBeenCalled();
  });

  it('tsdb 비활성(시리즈 0)도 idle 이며 channel 로 조용히 폴백하지 않는다(UB2-1)', async () => {
    const { result } = render({
      data_source: 'tsdb',
      tsdb_source: { backend: 'influxdb', agent_name: 'influx-1', series: [] },
    });
    await flushMicrotasks();
    expect(result.current.status).toBe('idle');
    expect(queryFn).not.toHaveBeenCalled();
  });
});

describe('usePanelSeriesData — 소스 전환 시 훅 순서가 안정적이다', () => {
  it('channel → store → tsdb 로 바꿔 재렌더해도 예외 없이 결과가 따라간다', async () => {
    const { result, rerender } = renderHook(
      ({ config }: { config: Record<string, unknown> }) =>
        usePanelSeriesData(config, {
          storeOptions: { queryMatrixFn: queryFn, resolveKeysFn: keysFn },
        }),
      { initialProps: { config: { data_source: 'channel' } as Record<string, unknown> } },
    );
    expect(result.current.status).toBe('idle');

    rerender({ config: { data_source: 'store', store_source: storeBlock() } });
    await flushMicrotasks();
    expect(result.current.status).toBe('connected');

    rerender({
      config: {
        data_source: 'tsdb',
        tsdb_source: {
          backend: 'influxdb',
          agent_name: 'influx-1',
          series: [{ measurement: 'm', field: 'f' }],
        },
      },
    });
    await flushMicrotasks();
    expect(result.current.status).toBe('idle');
  });
});

describe('isPanelSeriesSource — 채널 이외의 활성 소스만 참', () => {
  it.each([
    ['config 부재', {}, false],
    ['channel', { data_source: 'channel' }, false],
    ['인식 불가(channel 폴백)', { data_source: 'wat' }, false],
    ['store 비활성', { data_source: 'store' }, false],
    ['store 활성', { data_source: 'store', store_source: storeBlock() }, true],
    [
      'tsdb 활성',
      {
        data_source: 'tsdb',
        tsdb_source: {
          backend: 'influxdb',
          agent_name: 'influx-1',
          series: [{ measurement: 'm', field: 'f' }],
        },
      },
      true,
    ],
    ['tsdb 비활성', { data_source: 'tsdb' }, false],
  ])('%s → %s', (_label, config, expected) => {
    expect(isPanelSeriesSource(resolvePanelSourceBinding(config as Record<string, unknown>))).toBe(
      expected,
    );
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 §2.14 [S2] — 네 가지 상태가 서로 구분 가능하다 (AC-39 · AC-40)
//
// 판정의 정본은 `panelSeriesStatus.resolvePanelSeriesDisplay` 이며 여기서는 훅 결과를
// 그대로 먹여 네 상태가 실제로 갈리는지 확인한다. "빈 결과"와 "전체 실패"가 같은 값으로
// 접히면 사용자가 센서 단선을 정상으로 오독한다(§2.14).
// ---------------------------------------------------------------------------

describe('usePanelSeriesData — 상태 4종 (§2.14)', () => {
  /** 에이전트 목록을 미리 심어 둔 QueryClient. 백엔드 파생이 네트워크를 타지 않는다. */
  function tsdbWrapper({ children }: { children: ReactNode }) {
    const client = new QueryClient({
      defaultOptions: { queries: { staleTime: Infinity, retry: false } },
    });
    client.setQueryData(['agents', undefined], {
      data: [{ id: 'a-1', name: 'ix', type: 'influxdb' }],
      total: 1,
    });
    return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
  }

  function tsdbBlock(overrides: Partial<TsdbSourceConfig> = {}): TsdbSourceConfig {
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

  const tsdbMatrix: SeriesMatrix = {
    columns: ['cpu'],
    rows: [
      { bucketStartMs: 1000, values: [21.5] },
      { bucketStartMs: 2000, values: [22.0] },
    ],
  };

  /** 훅을 렌더하고 표시 상태까지 파생해 돌려준다. */
  function renderDisplay(
    config: Record<string, unknown>,
    queryTsdbFn?: QueryTsdbMatrixFn,
  ) {
    return renderHook(
      () => {
        const result = usePanelSeriesData(config, {
          storeOptions: { queryMatrixFn: queryFn, resolveKeysFn: keysFn },
          ...(queryTsdbFn ? { tsdbOptions: { queryTsdbFn } } : {}),
        });
        return {
          result,
          display: resolvePanelSeriesDisplay(resolvePanelSourceBinding(config), result),
        };
      },
      { wrapper: tsdbWrapper },
    );
  }

  beforeEach(() => {
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('시리즈 0개 → 빈 선택 안내', async () => {
    const tsdbFn = vi.fn<QueryTsdbMatrixFn>();
    const { result } = renderDisplay(
      { data_source: 'tsdb', tsdb_source: tsdbBlock({ series: [] }) },
      tsdbFn,
    );
    await flushMicrotasks();
    expect(result.current.display.state).toBe('empty-selection');
    // 빈 선택은 조회 이전 상태다 — 요청이 나가지 않는다.
    expect(tsdbFn).not.toHaveBeenCalled();
  });

  it('에이전트 미선택 → 빈 선택 안내', async () => {
    const tsdbFn = vi.fn<QueryTsdbMatrixFn>();
    const { result } = renderDisplay(
      {
        data_source: 'tsdb',
        tsdb_source: tsdbBlock({ agent_id: undefined, agent_name: '' }),
      },
      tsdbFn,
    );
    await flushMicrotasks();
    // 시리즈는 골랐지만 어느 외부 DB 인지 모른다(§2.18) — 같은 "빈 선택" 이다.
    expect(result.current.display.state).toBe('empty-selection');
    expect(tsdbFn).not.toHaveBeenCalled();
  });

  it('시리즈 N개 + 매트릭스 0행 → 빈 차트(오류 아님)', async () => {
    const tsdbFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue({
      matrix: { columns: ['cpu'], rows: [] },
      failures: [],
    });
    const { result } = renderDisplay(
      { data_source: 'tsdb', tsdb_source: tsdbBlock() },
      tsdbFn,
    );
    await flushMicrotasks();

    expect(result.current.result.status).toBe('connected');
    expect(result.current.result.entries).toEqual([]);
    // 여기가 §2.14 의 하중 지지점이다 — 데이터가 없는 것은 조회 실패가 아니다.
    expect(result.current.display.state).toBe('empty-result');
    expect(result.current.display.state).not.toBe('error');
  });

  it('일부 컬럼 실패 → 성공 시리즈 렌더 + 실패 개수 배지', async () => {
    const tsdbFn = vi.fn<QueryTsdbMatrixFn>().mockResolvedValue({
      matrix: tsdbMatrix,
      failures: [{ index: 1, error: new Error('boom') }],
    });
    const { result } = renderDisplay(
      { data_source: 'tsdb', tsdb_source: tsdbBlock() },
      tsdbFn,
    );
    await flushMicrotasks();

    expect(result.current.display.state).toBe('partial-failure');
    expect(result.current.display.failureCount).toBe(1);
    // 성공 시리즈는 그대로 렌더된다 — 1개 실패로 패널을 비우지 않는다(UB1-6).
    expect(result.current.result.entries.length).toBe(2);
    expect(result.current.result.status).toBe('connected');
  });

  it('조회 전체 실패 → 오류 오버레이 + 마지막 성공 렌더 보존', async () => {
    // 전 시리즈 실패는 부분 실패가 아니라 전체 실패로 승격되어 어댑터가 첫 오류를
    // 던진다(§2.19). 던지는 형태가 "마지막 성공 렌더 보존"과 양립하는지가 이 테스트의
    // 실제 질문이다 — 훅의 catch 가 이전 상태에 병합하므로 entries 는 살아남는다.
    const tsdbFn = vi
      .fn<QueryTsdbMatrixFn>()
      .mockResolvedValueOnce({ matrix: tsdbMatrix, failures: [] })
      .mockRejectedValue(new Error('all_series_failed'));
    const { result } = renderDisplay(
      { data_source: 'tsdb', tsdb_source: tsdbBlock() },
      tsdbFn,
    );
    await flushMicrotasks();
    expect(result.current.display.state).toBe('ok');
    expect(result.current.result.entries.length).toBe(2);

    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    await flushMicrotasks();

    expect(result.current.display.state).toBe('error');
    expect(result.current.result.errorReason).toContain('all_series_failed');
    // 오버레이는 뜨되 마지막 성공 렌더는 파괴되지 않는다(§2.14).
    expect(result.current.result.entries.length).toBe(2);
  });

  it('부분 실패 복구 시 배지가 제거된다', async () => {
    const tsdbFn = vi
      .fn<QueryTsdbMatrixFn>()
      .mockResolvedValueOnce({
        matrix: tsdbMatrix,
        failures: [{ index: 0, error: new Error('x') }],
      })
      .mockResolvedValue({ matrix: tsdbMatrix, failures: [] });
    const { result } = renderDisplay(
      { data_source: 'tsdb', tsdb_source: tsdbBlock() },
      tsdbFn,
    );
    await flushMicrotasks();
    expect(result.current.display.state).toBe('partial-failure');

    await act(async () => {
      vi.advanceTimersByTime(5_000);
    });
    await flushMicrotasks();

    expect(result.current.display.state).toBe('ok');
    expect(result.current.display.failureCount).toBe(0);
  });

  it('백엔드 불일치는 오류이며 사유가 구분된다 (§2.18 · AC-54)', async () => {
    // store 에이전트를 가리키는 tsdb_source. 질의 자체가 나가지 않아야 한다.
    function mismatchWrapper({ children }: { children: ReactNode }) {
      const client = new QueryClient({
        defaultOptions: { queries: { staleTime: Infinity, retry: false } },
      });
      client.setQueryData(['agents', undefined], {
        data: [{ id: 'a-1', name: 'ix', type: 'store' }],
        total: 1,
      });
      return <QueryClientProvider client={client}>{children}</QueryClientProvider>;
    }
    const tsdbFn = vi.fn<QueryTsdbMatrixFn>();
    const config = { data_source: 'tsdb', tsdb_source: tsdbBlock() };
    const { result } = renderHook(
      () => {
        const r = usePanelSeriesData(config, { tsdbOptions: { queryTsdbFn: tsdbFn } });
        return resolvePanelSeriesDisplay(resolvePanelSourceBinding(config), r);
      },
      { wrapper: mismatchWrapper },
    );
    await flushMicrotasks();

    expect(result.current.state).toBe('error');
    expect(result.current.backendMismatch).toBe(true);
    expect(tsdbFn).not.toHaveBeenCalled();
  });
});
