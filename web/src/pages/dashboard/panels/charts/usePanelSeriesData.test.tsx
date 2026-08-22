// usePanelSeriesData — 소스 종류 키잉 시리즈 조회 훅 테스트.
//
// queryMatrixFn 을 주입해 네트워크 없이 store 조회를 시뮬레이션한다. TSDB 분기는
// M3 시점에 idle 스텁이므로 "조회가 일어나지 않는다" 를 잠근다(M6.8 배선 지점).
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4)

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, renderHook } from '@testing-library/react';

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
import type { StoreSourceConfig } from './chartChannelTypes';
import type { QueryMatrixFn, ResolveKeysFn } from './useStoreChartData';
import { isPanelSeriesSource, usePanelSeriesData } from './usePanelSeriesData';
import { resolvePanelSourceBinding } from './panelDataSource';

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
