// PieChartPanel 테스트.

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

import PieChartPanel from './PieChartPanel';

describe('PieChartPanel', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  it('3개 카테고리 기준 슬라이스가 3개 렌더', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 50, labels: { name: 'A' } },
      { timestamp: 2, value: 30, labels: { name: 'B' } },
      { timestamp: 3, value: 20, labels: { name: 'C' } },
    ];
    const { container } = render(
      <PieChartPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          label_field: 'labels.name',
          agg_func: 'sum',
        }}
      />,
    );
    const sectors = container.querySelectorAll('.recharts-pie-sector');
    expect(sectors.length).toBe(3);
    const names = Array.from(sectors).map((s) => s.getAttribute('data-name'));
    expect(names).toEqual(expect.arrayContaining(['A', 'B', 'C']));
  });

  it('agg_func=count 이면 카테고리별 entry 수로 집계', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 50, labels: { name: 'A' } },
      { timestamp: 2, value: 30, labels: { name: 'A' } },
      { timestamp: 3, value: 20, labels: { name: 'B' } },
    ];
    const { container } = render(
      <PieChartPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          label_field: 'labels.name',
          agg_func: 'count',
        }}
      />,
    );
    const sectors = container.querySelectorAll('.recharts-pie-sector');
    expect(sectors.length).toBe(2);
    const map = new Map<string, number>();
    sectors.forEach((s) => {
      map.set(s.getAttribute('data-name') ?? '', Number(s.getAttribute('data-value')));
    });
    expect(map.get('A')).toBe(2);
    expect(map.get('B')).toBe(1);
  });

  it('recharts-wrapper 존재', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 10, labels: { name: 'A' } }];
    const { container } = render(
      <PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
    );
    expect(container.querySelector('.recharts-wrapper')).toBeTruthy();
  });

  it('show_legend=false 면 legend 미렌더', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 1, labels: { name: 'A' } }];
    const { container } = render(
      <PieChartPanel
        panelId="p1"
        config={{ channel_name: 'c', show_legend: false }}
      />,
    );
    expect(container.querySelector('.recharts-legend-wrapper')).toBeNull();
  });

  it('show_legend=true (기본값) 이면 legend 렌더', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 1, labels: { name: 'A' } }];
    const { container } = render(
      <PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
    );
    expect(container.querySelector('.recharts-legend-wrapper')).toBeTruthy();
  });

  it('closed overlay', () => {
    mockResult.current.status = 'closed' as unknown as 'connected';
    mockResult.current.closedReason = 'flow_undeployed';
    render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('pie-chart-overlay').textContent).toContain('flow_undeployed');
  });

  it('연결 상태 아이콘 렌더', () => {
    render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('chart-status-icon')).toBeInTheDocument();
  });
});
