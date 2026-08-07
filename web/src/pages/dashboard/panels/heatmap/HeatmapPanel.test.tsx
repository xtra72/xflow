// HeatmapPanel 견고성/렌더 분기 테스트 (SPEC-HEATMAP-PANEL-001 T6/T8/T9).
// AC-E1(빈 상태), AC-E2(미배치 안내), AC-E3(폴링 실패 시 마지막 렌더 유지 + 오류 배지),
// 정상 경로(배치 센서 → canvas 렌더)를 컴포넌트 레벨에서 커버한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ChartEntry } from '../charts/chartChannelTypes';
import type { UseStoreChartDataResult } from '../charts/useStoreChartData';

// useStoreChartData 를 주입 가능한 mock 으로 대체한다(네트워크/폴링 없이 결정적 렌더).
const storeMock: { current: UseStoreChartDataResult } = {
  current: {
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  },
};
vi.mock('../charts/useStoreChartData', () => ({
  useStoreChartData: () => storeMock.current,
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import HeatmapPanel from './HeatmapPanel';

/** 최신값 1개짜리 시리즈 타임라인. */
function reading(value: number): ChartEntry[] {
  return [{ timestamp: 1, value }];
}

/** store 태그 모드 활성 config(isStore=true). sensor_positions 는 인자로 주입. */
function makeConfig(sensorPositions: Record<string, { x: number; y: number }>) {
  return {
    data_source: 'store',
    store_source: {
      agent_name: 'a',
      namespace: 'default',
      selection_mode: 'tag',
      tag_filters: { type: 'temperature' },
      series: [],
      time_window_ms: 1000,
      interval_ms: 1000,
      aggregation: 'last',
    },
    sensor_positions: sensorPositions,
    idw: { power: 2, grid_resolution: 8 },
  } as Record<string, unknown>;
}

function setStore(partial: Partial<UseStoreChartDataResult>) {
  storeMock.current = { ...storeMock.current, ...partial };
}

beforeEach(() => {
  storeMock.current = {
    entries: [],
    seriesEntries: new Map(),
    seriesStyles: new Map(),
    seriesNames: [],
    booleanSeries: new Set(),
    status: 'idle',
  };
});

describe('HeatmapPanel', () => {
  it('AC-E1: 센서 0개면 빈 상태 안내를 표시하고 canvas 를 렌더하지 않는다', () => {
    setStore({ seriesNames: [], seriesEntries: new Map(), status: 'connected' });
    render(<HeatmapPanel panelId="p" config={makeConfig({})} />);
    expect(screen.getByText('dashboard.heatmap.emptyState')).toBeInTheDocument();
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
  });

  it('AC-E2: 좌표 미지정 센서만 있으면 빈 상태 + 미배치 안내를 표시한다', () => {
    setStore({
      seriesNames: ['s1'],
      seriesEntries: new Map([['s1', reading(22)]]),
      status: 'connected',
    });
    render(<HeatmapPanel panelId="p" config={makeConfig({})} />);
    expect(screen.getByText('dashboard.heatmap.emptyState')).toBeInTheDocument();
    // 미배치 안내(키 반환 mock 이므로 키 문자열로 확인).
    expect(screen.getByText('dashboard.heatmap.unplaced')).toBeInTheDocument();
    expect(screen.queryByTestId('heatmap-canvas')).toBeNull();
  });

  it('배치된 센서가 있으면 HeatmapCanvas 를 렌더한다(정상 경로)', () => {
    setStore({
      seriesNames: ['s1', 's2'],
      seriesEntries: new Map([
        ['s1', reading(20)],
        ['s2', reading(26)],
      ]),
      status: 'connected',
    });
    render(
      <HeatmapPanel
        panelId="p"
        config={makeConfig({ s1: { x: 0.2, y: 0.2 }, s2: { x: 0.8, y: 0.8 } })}
      />,
    );
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    expect(screen.queryByText('dashboard.heatmap.emptyState')).toBeNull();
  });

  it('AC-E3: 폴링 실패(status=error)여도 마지막 온도장을 유지하고 오류 배지를 덧띄운다', () => {
    setStore({
      seriesNames: ['s1'],
      // 직전 폴링의 시리즈가 보존된 상태(useStoreChartData 가 prev 를 유지).
      seriesEntries: new Map([['s1', reading(24)]]),
      status: 'error',
      errorReason: 'network',
    });
    render(<HeatmapPanel panelId="p" config={makeConfig({ s1: { x: 0.5, y: 0.5 } })} />);
    // canvas(마지막 렌더)는 파괴되지 않는다.
    expect(screen.getByTestId('heatmap-canvas')).toBeInTheDocument();
    // 오류 배지가 함께 표시된다.
    expect(screen.getByTestId('heatmap-error')).toBeInTheDocument();
  });
});
