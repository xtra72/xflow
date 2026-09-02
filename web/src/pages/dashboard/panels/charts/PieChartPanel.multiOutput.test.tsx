// PieChartPanel 다중 출력(구간 대표값) 동작 테스트 — SPEC-CHART-002 M3.4.
//
// 레거시 경로(aggregateByLabel / PIE_COLORS / max_points 트리밍)는 PieChartPanel.test.tsx
// 의 특성화 CH-08~CH-10 이 지킨다. 이 파일은 `series_reduce` 가 **지정된** 경우만 다룬다.
//
// @spec SPEC-CHART-002 AC-13 / AC-16 / §4.4(음수 조각 생략 + 사유 안내)

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';

import type { SeriesMatrix } from '@/services/api/seriesDataSource';

const NOW = 5_000;

const query = vi.hoisted(() => ({
  fn: undefined as unknown as ReturnType<typeof vi.fn>,
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

vi.mock('./useChartChannel', () => ({
  useChartChannel: () => ({
    entries: [],
    status: 'idle',
    closedReason: undefined,
    errorReason: undefined,
  }),
}));

vi.mock('./useStoreChartData', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useStoreChartData')>();
  return {
    ...actual,
    useStoreChartData: (config: Parameters<typeof actual.useStoreChartData>[0], enabled: boolean) =>
      actual.useStoreChartData(config, enabled, {
        queryMatrixFn: (agentName, q, signal) => query.fn(agentName, q, signal),
        nowFn: () => NOW,
      }),
  };
});

vi.mock('recharts', async () => {
  const React = await import('react');
  const stub = await import('./__mocks__/rechartsStub');
  return {
    ...stub,
    Cell: (props: { fill?: string }) =>
      React.createElement('div', {
        'data-testid': 'rc-cell',
        'data-cell-fill': props.fill ?? '',
      }),
  };
});

import type { StoreSourceConfig } from './chartChannelTypes';
import { pickSeriesColor } from './chartChannelTypes';
import PieChartPanel from './PieChartPanel';

/** F1 — 3시리즈 × 5버킷. room3 은 전 버킷 null. */
const F1_MATRIX: SeriesMatrix = {
  columns: ['k.room1', 'k.room2', 'k.room3'],
  rows: [
    { bucketStartMs: 1000, values: [20, 18, null] },
    { bucketStartMs: 2000, values: [22, null, null] },
    { bucketStartMs: 3000, values: [26, 19, null] },
    { bucketStartMs: 4000, values: [24, 19, null] },
    { bucketStartMs: 5000, values: [21, 23, null] },
  ],
};

/** room1 은 감소 구간(delta < 0), room2 는 증가 구간(delta > 0). */
const DELTA_MATRIX: SeriesMatrix = {
  columns: ['k.room1', 'k.room2', 'k.room3'],
  rows: [
    { bucketStartMs: 1000, values: [30, 10, null] },
    { bucketStartMs: 2000, values: [20, 15, null] },
    { bucketStartMs: 3000, values: [10, 25, null] },
  ],
};

function f1Config(overrides: Partial<StoreSourceConfig> = {}): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    namespace: 'default',
    selection_mode: 'keys',
    series: [
      { key: 'k.room1', alias: 'temp.room1' },
      { key: 'k.room2', alias: 'temp.room2' },
      { key: 'k.room3', alias: 'temp.room3' },
    ],
    time_window_ms: 60_000,
    interval_ms: 1_000,
    aggregation: 'average',
    ...overrides,
  };
}

function panelConfig(extra: Record<string, unknown> = {}, store?: StoreSourceConfig) {
  return {
    channel_name: 'c1',
    data_source: 'store',
    store_source: store ?? f1Config(),
    ...extra,
  };
}

async function renderPanel(config: Record<string, unknown>) {
  await act(async () => {
    render(<PieChartPanel panelId="p1" config={config} />);
  });
}

function slices(): Array<{ name: string; value: number }> {
  return screen.queryAllByTestId('rc-pie-slice').map((el) => ({
    name: el.getAttribute('data-name') ?? '',
    value: Number(el.getAttribute('data-value')),
  }));
}

function cellFills(): string[] {
  return screen
    .queryAllByTestId('rc-cell')
    .map((el) => el.getAttribute('data-cell-fill') ?? '');
}

describe('PieChartPanel 다중 출력 (SPEC-CHART-002 M3)', () => {
  beforeEach(() => {
    query.fn = vi.fn(async () => F1_MATRIX);
  });

  it('series_reduce 지정 시 시리즈당 조각 1개를 렌더한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'max' }));
    expect(slices()).toEqual([
      { name: 'temp.room1', value: 26 },
      { name: 'temp.room2', value: 23 },
    ]);
  });

  it('undefined 대표값 시리즈는 조각을 생략한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'avg' }));
    // room3 은 표본 0개 → 조각 없음. 사유 안내는 음수 전용이므로 나타나지 않는다.
    expect(slices().map((s) => s.name)).toEqual(['temp.room1', 'temp.room2']);
    expect(screen.queryByTestId('pie-negative-omitted')).toBeNull();
  });

  it('음수 대표값 시리즈는 조각을 생략하고 사유를 안내한다', async () => {
    query.fn = vi.fn(async () => DELTA_MATRIX);
    await renderPanel(panelConfig({ series_reduce: 'delta' }));

    // room1 delta = 10 - 30 = -20 → 생략. room2 delta = 25 - 10 = +15 → 유지.
    expect(slices()).toEqual([{ name: 'temp.room2', value: 15 }]);
    // 절댓값(20)도 0 clamp 도 아니다 — 조각이 사라지고 이유가 화면에 남는다.
    const notice = screen.getByTestId('pie-negative-omitted');
    expect(notice.textContent).toContain('dashboard.chart.pieNegativeOmitted');
  });

  it('0 은 음수가 아니므로 조각을 생략하지 않는다', async () => {
    query.fn = vi.fn(async () => ({
      columns: ['k.room1'],
      rows: [
        { bucketStartMs: 1000, values: [7] },
        { bucketStartMs: 2000, values: [7] },
      ],
    }));
    await renderPanel(
      panelConfig({ series_reduce: 'delta' }, f1Config({
        series: [{ key: 'k.room1', alias: 'temp.room1' }],
      })),
    );
    expect(slices()).toEqual([{ name: 'temp.room1', value: 0 }]);
    expect(screen.queryByTestId('pie-negative-omitted')).toBeNull();
  });

  it('series_reduce 지정 시 label_field/agg_func/max_points 를 무시한다', async () => {
    await renderPanel(
      panelConfig({
        series_reduce: 'max',
        label_field: 'labels.nonexistent',
        agg_func: 'count',
        max_points: 1,
      }),
    );
    expect(slices()).toEqual([
      { name: 'temp.room1', value: 26 },
      { name: 'temp.room2', value: 23 },
    ]);
  });

  it('시리즈 color 가 조각 채움색으로 적용되고 미지정 시 pickSeriesColor 팔레트를 쓴다', async () => {
    const store = f1Config({
      series: [
        { key: 'k.room1', alias: 'temp.room1', color: '#ff0000' },
        { key: 'k.room2', alias: 'temp.room2' },
        { key: 'k.room3', alias: 'temp.room3' },
      ],
    });
    await renderPanel(panelConfig({ series_reduce: 'max' }, store));
    // 지정 색 우선, 미지정은 필터 전 시리즈 인덱스 기준 팔레트.
    expect(cellFills()).toEqual(['#ff0000', pickSeriesColor(1)]);
  });

  it('레거시 모드에서는 기존 PIE_COLORS 순환 배정이 유지된다', async () => {
    // 같은 store config 에서 series_reduce 만 없애면 레거시 경로다.
    await renderPanel(panelConfig({}));
    // 평탄화 entries 를 labels.name 으로 그룹화 → 시리즈 이름이 그룹이 되지만 색은
    // 시리즈 색이 아니라 하드코딩 팔레트다.
    expect(cellFills()).toEqual(['#3b82f6', '#10b981']);
  });

  it('조각이 상한을 넘으면 12개만 그리고 +K 표기를 노출한다', async () => {
    const columns = Array.from({ length: 20 }, (_, i) => `k.s${i}`);
    query.fn = vi.fn(async () => ({
      columns,
      rows: [{ bucketStartMs: 1000, values: columns.map((_, i) => i + 1) }],
    }));
    await renderPanel(
      panelConfig({ series_reduce: 'last' }, f1Config({
        series: columns.map((key, i) => ({ key, alias: `s${i}` })),
      })),
    );

    expect(slices()).toHaveLength(12);
    expect(screen.getByTestId('series-tile-truncation').textContent).toContain('+8');
  });
});
