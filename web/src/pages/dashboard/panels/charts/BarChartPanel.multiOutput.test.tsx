// BarChartPanel 다중 출력(구간 대표값) 동작 테스트 — SPEC-CHART-002 M3.3.
//
// 레거시 경로(category / time_bin / 하드코딩 채움색)는 BarChartPanel.test.tsx 의 특성화
// CH-05~CH-07 이 지킨다. 이 파일은 `series_reduce` 가 **지정된** 경우만 다룬다.
//
// @spec SPEC-CHART-002 AC-12 / AC-15 / AC-16

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, act } from '@testing-library/react';

import type { SeriesMatrix, SeriesMatrixQuery } from '@/services/api/seriesDataSource';

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

// 채널 훅은 비활성(idle) 형상만 있으면 된다.
vi.mock('./useChartChannel', () => ({
  useChartChannel: () => ({
    entries: [],
    status: 'idle',
    closedReason: undefined,
    errorReason: undefined,
  }),
}));

// Store 훅은 실제 구현 + 테스트 주입점.
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

// Bar 는 children(Cell)을 렌더해야 시리즈별 채움색을 검증할 수 있다.
vi.mock('recharts', async () => {
  const React = await import('react');
  const stub = await import('./__mocks__/rechartsStub');
  return {
    ...stub,
    Bar: (props: { dataKey?: string | number; fill?: string; children?: React.ReactNode }) =>
      React.createElement(
        'div',
        {
          'data-testid': 'rc-bar',
          'data-bar-key': String(props.dataKey),
          'data-bar-fill': props.fill ?? '',
          className: 'recharts-bar',
        },
        props.children,
      ),
    Cell: (props: { fill?: string }) =>
      React.createElement('div', {
        'data-testid': 'rc-cell',
        'data-cell-fill': props.fill ?? '',
      }),
  };
});

import type { StoreSourceConfig } from './chartChannelTypes';
import { pickSeriesColor, storeSeriesLabel } from './chartChannelTypes';
import BarChartPanel from './BarChartPanel';

/** F1 — 3시리즈 × 5버킷. room3 은 전 버킷 null → 대표값 undefined. */
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

async function renderPanel(config: Record<string, unknown>, title?: string) {
  await act(async () => {
    render(<BarChartPanel panelId="p1" title={title} config={config} />);
  });
}

/** 차트에 전달된 행에서 라벨/값만 뽑는다(채움색은 Cell 로 따로 검증한다). */
function rows(): Array<{ label: string; value: number }> {
  const chart = screen.getByTestId('rc-bar-chart');
  const parsed = JSON.parse(chart.getAttribute('data-rows')!) as Array<{
    label: string;
    value: number;
  }>;
  return parsed.map((r) => ({ label: r.label, value: r.value }));
}

function cellFills(): string[] {
  return screen
    .queryAllByTestId('rc-cell')
    .map((el) => el.getAttribute('data-cell-fill') ?? '');
}

describe('BarChartPanel 헤더 표기', () => {
  beforeEach(() => {
    query.fn = vi.fn(async () => F1_MATRIX);
  });

  it('타이틀이 있으면 채널 이름 대신 타이틀을 표시한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'max' }), '실외기 온도');
    expect(screen.getByText('실외기 온도')).toBeInTheDocument();
  });

  it('시리즈 소스 패널의 부제에는 "채널 미지정" 을 붙이지 않는다', async () => {
    // store/tsdb 패널은 채널을 쓰지 않는다 — 채널 이름 자리를 비워 두면 설정이 빠진 것처럼
    // 읽히는 잘못된 안내가 된다.
    await renderPanel(panelConfig({ series_reduce: 'max' }), '실외기 온도');
    expect(screen.queryByText(/채널 미지정/)).toBeNull();
  });

  it('채널 모드에서는 부제에 채널 이름을 그대로 붙인다(종전 동작)', async () => {
    await renderPanel({ channel_name: 'c1', data_source: 'channel', mode: 'category' }, '바');
    expect(screen.getByText(/c1 ·/)).toBeInTheDocument();
  });
});

describe('BarChartPanel 다중 출력 (SPEC-CHART-002 M3)', () => {
  beforeEach(() => {
    query.fn = vi.fn(async () => F1_MATRIX);
  });

  it('series_reduce 지정 시 시리즈당 막대 1개를 렌더한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'avg' }));
    // 카테고리 라벨 = 시리즈 표시 이름. 값 = 시리즈 대표값.
    expect(rows()).toEqual([
      { label: 'temp.room1', value: 22.6 },
      { label: 'temp.room2', value: 19.75 },
    ]);
  });

  it('대표값이 undefined 인 시리즈는 막대를 생략한다', async () => {
    await renderPanel(panelConfig({ series_reduce: 'max' }));
    const labels = rows().map((r) => r.label);
    // room3 은 표본 0개 → 막대 자체가 없다(0 과 구분되지 않는 막대를 그리지 않는다).
    expect(labels).toEqual(['temp.room1', 'temp.room2']);
    expect(labels).not.toContain('temp.room3');
  });

  it('series_reduce 지정 시 mode/label_field/agg_func 를 무시한다', async () => {
    await renderPanel(
      panelConfig({
        series_reduce: 'max',
        mode: 'time_bin',
        bin_sec: 1,
        agg_func: 'count',
        label_field: 'labels.nonexistent',
        max_points: 1,
      }),
    );
    // time_bin/count/label_field/max_points 중 무엇도 결과에 영향을 주지 않는다.
    expect(rows()).toEqual([
      { label: 'temp.room1', value: 26 },
      { label: 'temp.room2', value: 23 },
    ]);
  });

  it('시리즈 color 가 막대 채움색으로 적용되고 미지정 시 pickSeriesColor 팔레트를 쓴다', async () => {
    const store = f1Config({
      series: [
        { key: 'k.room1', alias: 'temp.room1', color: '#ff0000' },
        { key: 'k.room2', alias: 'temp.room2' },
        { key: 'k.room3', alias: 'temp.room3' },
      ],
    });
    await renderPanel(panelConfig({ series_reduce: 'max' }, store));

    // 지정 색 우선, 미지정은 **필터 전 시리즈 인덱스** 기준 팔레트(색이 밀려나지 않는다).
    expect(cellFills()).toEqual(['#ff0000', pickSeriesColor(1)]);
    // Bar 레벨 하드코딩 색은 다중 출력 경로에서 비운다(§2.6).
    expect(screen.getByTestId('rc-bar').getAttribute('data-bar-fill')).toBe('');
  });

  it('레거시 모드에서는 기존 하드코딩 색상(#3b82f6)이 유지된다', async () => {
    // 같은 store config 에서 series_reduce 만 없애면 레거시 경로다.
    await renderPanel(panelConfig({}));
    expect(screen.getByTestId('rc-bar').getAttribute('data-bar-fill')).toBe('#3b82f6');
    expect(cellFills()).toEqual([]);
  });

  it('출력 라벨은 storeSeriesLabel 규칙(alias > series_name_format > 서술 표기)을 따른다', async () => {
    const store = f1Config({
      series_name_format: '{$.measurement}-fmt',
      series: [
        { key: 'k.room1', alias: 'aliasA' },
        { key: 'k.room2' },
        { key: 'k.room3', field: 'gauge', tags: { room: '3' } },
      ],
    });
    await renderPanel(panelConfig({ series_reduce: 'last' }, store));

    const expected = store.series
      .slice(0, 2) // room3 은 값이 없어 막대가 생략된다
      .map((ref) => storeSeriesLabel(ref, store.series_name_format));
    expect(rows().map((r) => r.label)).toEqual(expected);
    expect(expected[0]).toBe('aliasA');
    expect(expected[1]).toBe('k.room2-fmt');
  });

  it('막대가 상한을 넘으면 12개만 그리고 +K 표기를 노출한다', async () => {
    const columns = Array.from({ length: 20 }, (_, i) => `k.s${i}`);
    query.fn = vi.fn(async (_a: string, _q: SeriesMatrixQuery) => ({
      columns,
      rows: [{ bucketStartMs: 1000, values: columns.map((_, i) => i + 1) }],
    }));
    await renderPanel(
      panelConfig({ series_reduce: 'last' }, f1Config({
        series: columns.map((key, i) => ({ key, alias: `s${i}` })),
      })),
    );

    expect(rows()).toHaveLength(12);
    expect(screen.getByTestId('series-tile-truncation').textContent).toContain('+8');
  });
});
