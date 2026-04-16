// LineChartPanel 테스트.
// Recharts 는 jsdom 환경에서 SVG 를 정상 렌더하지 못하므로
// __mocks__/rechartsStub 으로 대체해 데이터 흐름만 검증한다.

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

vi.mock('recharts', async () => await import('./__mocks__/rechartsStub'));

import LineChartPanel from './LineChartPanel';

describe('LineChartPanel', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('recharts-wrapper (LineChart) 가 렌더', () => {
    mockResult.current.entries = [
      { timestamp: 1000, value: 1 },
      { timestamp: 2000, value: 2 },
    ];
    const { container } = render(
      <LineChartPanel panelId="p1" config={{ channel_name: 'test' }} />,
    );
    expect(container.querySelector('.recharts-wrapper')).toBeTruthy();
  });

  it('entries 비어있을 때도 크래시 없이 렌더', () => {
    mockResult.current.entries = [];
    expect(() =>
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'test' }} />),
    ).not.toThrow();
  });

  it('단일 시리즈: 1개 Line 렌더', () => {
    mockResult.current.entries = [
      { timestamp: 1000, value: 10 },
      { timestamp: 2000, value: 20 },
    ];
    const { container } = render(
      <LineChartPanel panelId="p1" config={{ channel_name: 'test' }} />,
    );
    const lines = container.querySelectorAll('.recharts-line');
    expect(lines.length).toBe(1);
    expect(lines[0]!.getAttribute('data-line-key')).toBe('value');
  });

  it('multi_series_field 지정 시 시리즈 수만큼 Line 분리', () => {
    mockResult.current.entries = [
      { timestamp: 1000, value: 10, labels: { room: 'A' } },
      { timestamp: 1000, value: 20, labels: { room: 'B' } },
      { timestamp: 2000, value: 11, labels: { room: 'A' } },
      { timestamp: 2000, value: 21, labels: { room: 'B' } },
    ];
    const { container } = render(
      <LineChartPanel
        panelId="p1"
        config={{ channel_name: 'test', multi_series_field: 'labels.room' }}
      />,
    );
    const lines = container.querySelectorAll('.recharts-line');
    expect(lines.length).toBe(2);
    const keys = Array.from(lines).map((l) => l.getAttribute('data-line-key'));
    expect(keys).toContain('A');
    expect(keys).toContain('B');
  });

  it('closed 상태에서 overlay 표시', () => {
    mockResult.current.status = 'closed' as unknown as 'connected';
    mockResult.current.closedReason = 'flow_undeployed';
    render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('line-chart-overlay').textContent).toContain('flow_undeployed');
  });

  it('연결 상태 아이콘 렌더', () => {
    render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('chart-status-icon')).toBeInTheDocument();
  });

  it('LineChart 에 전달된 data 가 단일 시리즈 timestamp/value 쌍', () => {
    mockResult.current.entries = [
      { timestamp: 1000, value: 10 },
      { timestamp: 2000, value: 20 },
    ];
    render(<LineChartPanel panelId="p1" config={{ channel_name: 'test' }} />);
    const chart = screen.getByTestId('rc-line-chart');
    const rowsAttr = chart.getAttribute('data-rows')!;
    const rows = JSON.parse(rowsAttr) as Array<{ timestamp: number; value: number }>;
    expect(rows).toEqual([
      { timestamp: 1000, value: 10 },
      { timestamp: 2000, value: 20 },
    ]);
  });
});
