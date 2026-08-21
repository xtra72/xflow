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

// 공용 recharts 스텁을 쓰되, Cell 의 fill 을 data-* 로 노출하도록 이 파일에서만
// 덮어쓴다(스텁 파일 자체는 건드리지 않는다 — M2 는 테스트 외 변경 금지).
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

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `series_reduce` 부재(레거시) 경로의 PieChartPanel 렌더 규칙을 잠근다.
// spec.md §2.9 [S1] / §1.2.4.
// ---------------------------------------------------------------------------

/** PieChartPanel.tsx 의 PIE_COLORS 와 같은 배열(특성화 대상 상수 사본). */
const PIE_COLORS_EXPECTED = [
  '#3b82f6',
  '#10b981',
  '#f59e0b',
  '#ef4444',
  '#8b5cf6',
  '#06b6d4',
  '#ec4899',
  '#84cc16',
  '#f97316',
  '#14b8a6',
];

describe('PieChartPanel 특성화 (SPEC-CHART-002 M2)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  function slices(): Array<{ name: string; value: number }> {
    return Array.from(document.querySelectorAll('[data-testid="rc-pie-slice"]')).map((el) => ({
      name: el.getAttribute('data-name') ?? '',
      value: Number(el.getAttribute('data-value')),
    }));
  }

  it('CH-08: aggregateByLabel + agg_func(sum/count/avg) 로 라벨 그룹을 집계한다', () => {
    const entries = [
      { timestamp: 1, value: 50, labels: { name: 'A' } },
      { timestamp: 2, value: 30, labels: { name: 'A' } },
      { timestamp: 3, value: 20, labels: { name: 'B' } },
    ];

    // 기본값 sum
    mockResult.current.entries = entries;
    const sum = render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(slices()).toEqual([
      { name: 'A', value: 80 },
      { name: 'B', value: 20 },
    ]);
    sum.unmount();

    // count = 그룹의 entry 수
    mockResult.current.entries = entries;
    const count = render(
      <PieChartPanel panelId="p1" config={{ channel_name: 'c', agg_func: 'count' }} />,
    );
    expect(slices()).toEqual([
      { name: 'A', value: 2 },
      { name: 'B', value: 1 },
    ]);
    count.unmount();

    // avg = sum / count
    mockResult.current.entries = entries;
    const avg = render(
      <PieChartPanel panelId="p1" config={{ channel_name: 'c', agg_func: 'avg' }} />,
    );
    expect(slices()).toEqual([
      { name: 'A', value: 40 },
      { name: 'B', value: 20 },
    ]);
    avg.unmount();
  });

  it('CH-09: 조각 색은 PIE_COLORS 를 인덱스 기준으로 순환 배정한다', () => {
    // 12그룹 → 10색 팔레트를 한 바퀴 돌고 앞 2색을 재사용한다.
    mockResult.current.entries = Array.from({ length: 12 }, (_, i) => ({
      timestamp: i + 1,
      value: i + 1,
      labels: { name: `L${i}` },
    }));
    render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);

    const fills = Array.from(document.querySelectorAll('[data-testid="rc-cell"]')).map((el) =>
      el.getAttribute('data-cell-fill'),
    );
    expect(fills).toHaveLength(12);
    expect(fills.slice(0, 10)).toEqual(PIE_COLORS_EXPECTED);
    // 순환(i % 10)
    expect(fills[10]).toBe(PIE_COLORS_EXPECTED[0]);
    expect(fills[11]).toBe(PIE_COLORS_EXPECTED[1]);
  });

  it('CH-10: max_points 는 그룹이 아니라 원시 entry 를 뒤에서 자른 뒤 집계한다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 50, labels: { name: 'A' } },
      { timestamp: 2, value: 30, labels: { name: 'A' } },
      { timestamp: 3, value: 20, labels: { name: 'B' } },
    ];
    render(<PieChartPanel panelId="p1" config={{ channel_name: 'c', max_points: 2 }} />);

    // 최근 2개 entry = [A=30, B=20] → A 의 첫 표본 50 은 집계에서 빠진다(80 이 아님).
    expect(slices()).toEqual([
      { name: 'A', value: 30 },
      { name: 'B', value: 20 },
    ]);
  });

  it('CH-10: max_points 트리밍으로 라벨 자체가 사라질 수 있다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 1, labels: { name: 'A' } },
      { timestamp: 2, value: 2, labels: { name: 'B' } },
      { timestamp: 3, value: 3, labels: { name: 'C' } },
    ];
    render(<PieChartPanel panelId="p1" config={{ channel_name: 'c', max_points: 2 }} />);
    expect(slices().map((s) => s.name)).toEqual(['B', 'C']);
  });

  it('CH-08: show_percentage 기본 ON / show_legend 기본 ON 이다', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 1, labels: { name: 'A' } }];
    const { container } = render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(container.querySelector('.recharts-legend-wrapper')).toBeTruthy();
    // show_percentage 는 Pie 의 label 렌더러로만 전달되므로 스텁에서는 직접 확인하지
    // 않는다. 여기서는 legend 기본 ON 만 잠근다(스텁 한계 — 특성화 범위 밖).
  });
});
