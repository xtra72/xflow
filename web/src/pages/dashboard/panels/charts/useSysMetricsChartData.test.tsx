// useSysMetricsChartData 테스트 — 이력 질의 모델.
//
// 에이전트가 이력을 들고 있으므로 이 훅은 **누적기가 아니라 질의자**다. 잠그는 것:
//
//   - config 를 Store 형상으로 옮기는 변환(`toStoreShapedConfig`)이 어휘를 보존한다
//   - 조회·버킷팅·이름 짓기는 `useStoreChartData` 에 위임한다(사본을 만들지 않는다)
//   - 에이전트를 고르기 전에는 조회하지 않는다
//
// 조회 자체는 `usePanelSeriesData` 가 Store 훅 하나로 처리하므로, 위임은 그 진입점으로
// 확인한다 — 이 모듈에 훅 사본을 두면 두 경로가 갈라진다.

import { describe, expect, it, vi } from 'vitest';
import { renderHook } from '@testing-library/react';

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [{ id: 'a1', name: 'host-1', type: 'sysmetrics' }] } }),
}));

import type { SeriesMatrix, SeriesMatrixQuery } from '@/services/api/seriesDataSource';
import type { SysmetricsSourceConfig } from './chartChannelTypes';
import { toStoreShapedConfig } from './useSysMetricsChartData';
import { usePanelSeriesData } from './usePanelSeriesData';
import type { QueryMatrixFn } from './useStoreChartData';

function source(over: Partial<SysmetricsSourceConfig> = {}): SysmetricsSourceConfig {
  return {
    agent_id: 'a1',
    agent_name: 'host-1',
    series: [{ key: 'cpu.usage_percent' }],
    time_window_ms: 60 * 60_000,
    interval_ms: 60_000,
    aggregation: 'average',
    ...over,
  };
}

describe('toStoreShapedConfig — 어휘를 보존한 채 자리만 옮긴다', () => {
  it('measurement 는 key 로, 분류·대상은 tags 로 간다', () => {
    const out = toStoreShapedConfig(
      source({ series: [{ key: 'network.bytes_recv', target: 'en0' }] }),
    );
    expect(out!.series).toEqual([
      expect.objectContaining({
        key: 'bytes_recv',
        tags: { category: 'network', interface: 'en0' },
      }),
    ]);
  });

  it('대상이 없으면 분류 태그만 실린다', () => {
    const out = toStoreShapedConfig(source({ series: [{ key: 'cpu.usage_percent' }] }));
    expect(out!.series[0]).toMatchObject({
      key: 'usage_percent',
      tags: { category: 'cpu' },
    });
  });

  it('이름·색·선 모양을 그대로 물려준다', () => {
    // 여기서 빠뜨리면 조회는 되는데 스타일이 엉뚱한 줄에 붙는다.
    const out = toStoreShapedConfig(
      source({
        series: [
          {
            key: 'cpu.usage_percent',
            alias: 'CPU',
            color: '#abc',
            stroke_style: 'dashed',
            stroke_width: 3,
            smooth: true,
          },
        ],
      }),
    );
    expect(out!.series[0]).toMatchObject({
      alias: 'CPU',
      color: '#abc',
      stroke_style: 'dashed',
      stroke_width: 3,
      smooth: true,
    });
  });

  it('조회 창 필드를 Store 형상 그대로 옮긴다', () => {
    const out = toStoreShapedConfig(
      source({ time_window_ms: 123, interval_ms: 45, aggregation: 'max', refresh_interval_ms: 7 }),
    );
    expect(out).toMatchObject({
      time_window_ms: 123,
      interval_ms: 45,
      aggregation: 'max',
      refresh_interval_ms: 7,
    });
  });

  it('조회 창 필드가 없는 옛 config 도 기본값으로 조회된다', () => {
    // 이 필드들은 "에이전트가 이력을 보관" 으로 바뀌면서 생겼다. 그 전에 저장된
    // 패널에는 없으므로, 메우지 않으면 창을 해석하지 못해 조회가 아예 나가지 않는다
    // — 화면에는 그냥 빈 차트로 보인다.
    const legacy = {
      agent_id: 'a1',
      agent_name: 'host-1',
      series: [{ key: 'cpu.usage_percent' }],
    } as unknown as SysmetricsSourceConfig;

    expect(toStoreShapedConfig(legacy)).toMatchObject({
      time_window_ms: 60 * 60_000,
      interval_ms: 60_000,
      aggregation: 'average',
    });
  });

  it('카탈로그에 없는 키는 옮기지 않는다', () => {
    const out = toStoreShapedConfig(source({ series: [{ key: 'nope.nothing' }] }));
    expect(out!.series).toEqual([]);
  });

  it('소스가 없으면 undefined 다', () => {
    expect(toStoreShapedConfig(undefined)).toBeUndefined();
  });
});

describe('패널 진입점 — sysmetrics 조회 위임', () => {
  const matrix: SeriesMatrix = {
    columns: ['usage_percent · {category=cpu}'],
    rows: [
      { bucketStartMs: 1_000, values: [42] },
      { bucketStartMs: 2_000, values: [43] },
    ],
  };

  /** sysmetrics 소스를 고른 패널 config. */
  function panelConfig(over: Partial<SysmetricsSourceConfig> = {}): Record<string, unknown> {
    return { data_source: 'sysmetrics', sysmetrics_source: source(over) };
  }

  it('에이전트를 고르기 전에는 조회하지 않는다', () => {
    const queryMatrixFn = vi.fn();
    renderHook(() =>
      usePanelSeriesData(panelConfig({ agent_id: undefined }), {
        sysmetricsOptions: { queryMatrixFn: queryMatrixFn as unknown as QueryMatrixFn },
      }),
    );
    expect(queryMatrixFn).not.toHaveBeenCalled();
  });

  it('다른 소스를 고르면 sysmetrics 를 조회하지 않는다', () => {
    const queryMatrixFn = vi.fn();
    renderHook(() =>
      usePanelSeriesData(
        { ...panelConfig(), data_source: 'channel' },
        { sysmetricsOptions: { queryMatrixFn: queryMatrixFn as unknown as QueryMatrixFn } },
      ),
    );
    expect(queryMatrixFn).not.toHaveBeenCalled();
  });

  it('활성이면 Store 훅을 통해 매트릭스를 조회한다', () => {
    const queryMatrixFn = vi.fn(
      async (_agent: string, _params: SeriesMatrixQuery, _signal: AbortSignal) => matrix,
    );
    renderHook(() =>
      usePanelSeriesData(panelConfig(), {
        sysmetricsOptions: {
          queryMatrixFn: queryMatrixFn as unknown as QueryMatrixFn,
          nowFn: () => 10_000,
        },
      }),
    );

    expect(queryMatrixFn).toHaveBeenCalled();
    const params = queryMatrixFn.mock.calls[0]![1] as unknown as SeriesMatrixQuery;
    // 요청 키·태그가 Store 어휘 그대로 나간다 — 이력 시리즈와 짝짓는 축이다.
    expect(params.keys).toEqual(['usage_percent']);
    expect(params.seriesFilters?.[0]?.tags).toEqual({ category: 'cpu' });
    expect(params.intervalMs).toBe(60_000);
    expect(params.aggregation).toBe('average');
  });
});
