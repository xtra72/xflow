// BarChartPanel 시리즈 색 적용 테스트.
//
// 회귀 대상: 시리즈마다 색을 골라 두어도 막대가 전부 하드코딩 파란색으로 그려지던
// 문제. 다중 출력(`series_reduce`) 경로만 시리즈 색을 입히고 있었고, 카테고리
// 경로는 라벨이 곧 시리즈 이름인데도 색을 쓰지 않았다 — 화면에서는 범례와 막대가
// 어긋나 어느 막대가 어느 시리즈인지 색으로 읽을 수 없었다.

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

// Bar 는 children(Cell)을 렌더해야 막대별 채움색을 검증할 수 있다.
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
import BarChartPanel from './BarChartPanel';

/** 2시리즈 × 2버킷 — 카테고리 경로에서 시리즈당 막대 하나가 된다. */
const MATRIX: SeriesMatrix = {
  columns: ['k.room1', 'k.room2'],
  rows: [
    { bucketStartMs: 1000, values: [20, 18] },
    { bucketStartMs: 2000, values: [22, 19] },
  ],
};

function storeConfig(series: StoreSourceConfig['series']): StoreSourceConfig {
  return {
    agent_name: 'store-1',
    namespace: 'default',
    selection_mode: 'keys',
    series,
    time_window_ms: 60_000,
    interval_ms: 1_000,
    aggregation: 'average',
  };
}

async function renderPanel(store: StoreSourceConfig, extra: Record<string, unknown> = {}) {
  await act(async () => {
    render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c1', data_source: 'store', store_source: store, ...extra }}
      />,
    );
  });
}

function cellFills(): string[] {
  return screen.queryAllByTestId('rc-cell').map((el) => el.getAttribute('data-cell-fill') ?? '');
}

function barFill(): string {
  return screen.getByTestId('rc-bar').getAttribute('data-bar-fill') ?? '';
}

describe('BarChartPanel 시리즈 색 (카테고리 경로)', () => {
  beforeEach(() => {
    query.fn = vi.fn(async () => MATRIX);
  });

  it('시리즈에 고른 색을 막대에 입힌다', async () => {
    await renderPanel(
      storeConfig([
        { key: 'k.room1', alias: 'temp.room1', color: '#ff8800' },
        { key: 'k.room2', alias: 'temp.room2', color: '#ee0033' },
      ]),
    );
    expect(cellFills()).toEqual(['#ff8800', '#ee0033']);
    // 막대별 색을 쓸 때는 Bar 레벨 fill 을 비운다 — 두 축이 섞이면 어느 쪽이 이기는지
    // 렌더러 구현에 달리게 된다.
    expect(barFill()).toBe('');
  });

  it('색을 고르지 않으면 종전 하드코딩 색을 그대로 쓴다', async () => {
    // 무회귀: 색 축을 쓰지 않는 패널의 그림은 바뀌지 않아야 한다.
    await renderPanel(
      storeConfig([
        { key: 'k.room1', alias: 'temp.room1' },
        { key: 'k.room2', alias: 'temp.room2' },
      ]),
    );
    expect(cellFills()).toEqual([]);
    expect(barFill()).toBe('#3b82f6');
  });

  it('일부만 색을 고르면 나머지는 종전 색으로 남는다', async () => {
    await renderPanel(
      storeConfig([
        { key: 'k.room1', alias: 'temp.room1', color: '#ff8800' },
        { key: 'k.room2', alias: 'temp.room2' },
      ]),
    );
    expect(cellFills()).toEqual(['#ff8800', '#3b82f6']);
  });
});
