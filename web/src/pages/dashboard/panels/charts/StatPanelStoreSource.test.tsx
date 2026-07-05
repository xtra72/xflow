// StatPanel 의 data_source 분기 테스트(SPEC-WEB-005).
// useChartChannel / useStoreChartData 를 모두 모킹해, config.data_source 에 따라
// 올바른 소스가 활성화되는지 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ChartEntry } from './chartChannelTypes';

const channelResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

const storeResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    seriesEntries: new Map<string, ChartEntry[]>(),
    seriesNames: [] as string[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

// 어떤 인자로 호출됐는지 추적해 비활성 경로가 idle 로 호출되는지 확인한다.
const channelCalls = vi.hoisted(() => ({ args: [] as unknown[] }));
const storeCalls = vi.hoisted(() => ({ args: [] as unknown[] }));

vi.mock('./useChartChannel', () => ({
  useChartChannel: (channelName: string | undefined) => {
    channelCalls.args.push(channelName);
    return channelResult.current;
  },
}));

vi.mock('./useStoreChartData', () => ({
  useStoreChartData: (config: unknown, enabled: boolean) => {
    storeCalls.args.push({ config, enabled });
    return storeResult.current;
  },
}));

vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import StatPanel from './StatPanel';

describe('StatPanel data_source 분기', () => {
  beforeEach(() => {
    channelCalls.args = [];
    storeCalls.args = [];
    channelResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    storeResult.current = {
      entries: [],
      seriesEntries: new Map(),
      seriesNames: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('data_source 미지정이면 채널 경로를 사용한다(하위 호환)', () => {
    channelResult.current.entries = [
      { timestamp: 1, value: 10 },
      { timestamp: 2, value: 12 },
    ];
    render(<StatPanel panelId="p1" config={{ channel_name: 'c1' }} />);
    // 채널 데이터가 렌더된다.
    expect(screen.getByTestId('stat-value').textContent).toContain('12');
    // 채널 훅은 채널명으로, store 훅은 비활성(enabled=false)으로 호출된다.
    expect(channelCalls.args).toContain('c1');
    expect(storeCalls.args[0]).toMatchObject({ enabled: false });
  });

  it('data_source=store + series 있으면 Store 경로를 사용한다', () => {
    storeResult.current.entries = [
      { timestamp: 1, value: 99 },
      { timestamp: 2, value: 100 },
    ];
    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c1',
          data_source: 'store',
          store_source: {
            agent_name: 'store-1',
            series: [{ key: 'k1' }],
            time_window_ms: 60000,
            interval_ms: 10000,
            aggregation: 'average',
          },
        }}
      />,
    );
    // Store 데이터가 렌더된다.
    expect(screen.getByTestId('stat-value').textContent).toContain('100');
    // store 훅은 enabled=true, 채널 훅은 channelName=undefined(비활성)로 호출.
    expect(storeCalls.args[0]).toMatchObject({ enabled: true });
    expect(channelCalls.args).toContain(undefined);
  });

  it('data_source=store 이지만 series 가 비면 채널 경로로 폴백한다', () => {
    channelResult.current.entries = [{ timestamp: 1, value: 7 }];
    render(
      <StatPanel
        panelId="p1"
        config={{
          channel_name: 'c1',
          data_source: 'store',
          store_source: {
            agent_name: 'store-1',
            series: [],
            time_window_ms: 60000,
            interval_ms: 10000,
            aggregation: 'average',
          },
        }}
      />,
    );
    expect(screen.getByTestId('stat-value').textContent).toContain('7');
    expect(storeCalls.args[0]).toMatchObject({ enabled: false });
    expect(channelCalls.args).toContain('c1');
  });
});
