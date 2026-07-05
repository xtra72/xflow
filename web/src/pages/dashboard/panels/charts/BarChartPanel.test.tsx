// BarChartPanel 테스트.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen } from '@testing-library/react';

import type { ChartEntry } from './chartChannelTypes';

const mockResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

vi.mock('./useChartChannel', () => ({
  useChartChannel: () => mockResult.current,
}));

// useStoreChartData 가 내부에서 useAgents(React Query)를 호출하므로, QueryClient
// 없이 렌더 가능하도록 빈 목록으로 모킹한다. 목록이 비면 store 소스는 저장된
// 이름을 그대로 사용(fallback)해 기존 동작이 유지된다. (SPEC-WEB-006)
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

vi.mock('recharts', async () => await import('./__mocks__/rechartsStub'));

import BarChartPanel from './BarChartPanel';

describe('BarChartPanel', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  function readRows(): Array<{ label: string; value: number }> {
    const chart = screen.getByTestId('rc-bar-chart');
    const rowsAttr = chart.getAttribute('data-rows')!;
    return JSON.parse(rowsAttr) as Array<{ label: string; value: number }>;
  }

  it('category 모드: label 별 최신 값 그룹', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 10, labels: { name: 'A' } },
      { timestamp: 2, value: 20, labels: { name: 'B' } },
      { timestamp: 3, value: 12, labels: { name: 'A' } },
    ];
    render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category', label_field: 'labels.name' }}
      />,
    );
    const rows = readRows();
    expect(rows).toHaveLength(2);
    const a = rows.find((r) => r.label === 'A');
    const b = rows.find((r) => r.label === 'B');
    expect(a?.value).toBe(12); // 최신 값
    expect(b?.value).toBe(20);
  });

  it('time_bin 모드: bin_sec 60 초 기준 집계 (sum)', () => {
    // bin 경계에 맞춰 timestamp 를 명시적으로 선택 (60000 ms 경계)
    // bin1: [0, 60000), bin2: [60000, 120000)
    mockResult.current.entries = [
      { timestamp: 1000, value: 10 },
      { timestamp: 30_000, value: 20 }, // bin1
      { timestamp: 60_001, value: 5 }, // bin2
    ];
    render(
      <BarChartPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          mode: 'time_bin',
          bin_sec: 60,
          agg_func: 'sum',
        }}
      />,
    );
    const rows = readRows();
    expect(rows).toHaveLength(2);
    expect(rows[0]!.value).toBe(30);
    expect(rows[1]!.value).toBe(5);
  });

  it('time_bin + count 모드', () => {
    const t = 0;
    mockResult.current.entries = [
      { timestamp: t + 0, value: 1 },
      { timestamp: t + 1000, value: 2 },
      { timestamp: t + 60_001, value: 3 },
    ];
    render(
      <BarChartPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          mode: 'time_bin',
          bin_sec: 60,
          agg_func: 'count',
        }}
      />,
    );
    const rows = readRows();
    expect(rows[0]!.value).toBe(2);
    expect(rows[1]!.value).toBe(1);
  });

  it('recharts-wrapper 존재', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 1, labels: { name: 'A' } }];
    const { container } = render(
      <BarChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
    );
    expect(container.querySelector('.recharts-wrapper')).toBeTruthy();
  });

  it('closed overlay', () => {
    mockResult.current.status = 'closed' as unknown as 'connected';
    mockResult.current.closedReason = 'node_removed';
    render(<BarChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('bar-chart-overlay').textContent).toContain('node_removed');
  });

  it('연결 상태 아이콘 렌더', () => {
    render(<BarChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('chart-status-icon')).toBeInTheDocument();
  });
});
