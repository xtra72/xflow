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

// 공용 recharts 스텁을 쓰되, Bar 의 fill 을 data-* 로 노출하도록 이 파일에서만
// 덮어쓴다(스텁 파일 자체는 건드리지 않는다 — M2 는 테스트 외 변경 금지).
// JSX 대신 createElement 를 쓰는 이유: vi.mock 팩토리는 호이스팅되므로 파일 상단
// 바인딩에 의존하지 않도록 팩토리 안에서 react 를 동적 import 한다.
vi.mock('recharts', async () => {
  const React = await import('react');
  const stub = await import('./__mocks__/rechartsStub');
  return {
    ...stub,
    Bar: (props: { dataKey?: string | number; fill?: string }) =>
      React.createElement('div', {
        'data-testid': 'rc-bar',
        'data-bar-key': String(props.dataKey),
        'data-bar-fill': props.fill ?? '',
        className: 'recharts-bar',
      }),
  };
});

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

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `series_reduce` 부재(레거시) 경로의 BarChartPanel 렌더 규칙을 잠근다.
// spec.md §2.9 [S1] / §1.2.4.
// ---------------------------------------------------------------------------
describe('BarChartPanel 특성화 (SPEC-CHART-002 M2)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  function rows(): Array<{ label: string; value: number }> {
    const chart = screen.getByTestId('rc-bar-chart');
    return JSON.parse(chart.getAttribute('data-rows')!) as Array<{ label: string; value: number }>;
  }

  it('CH-05: category 모드는 labels.name 그룹의 최신값을 막대로 낸다', () => {
    // Store 소스 entry 형상(labels.name = 시리즈 표시 이름)을 그대로 사용한다.
    mockResult.current.entries = [
      { timestamp: 1, value: 20, labels: { name: 'temp.room1' } },
      { timestamp: 1, value: 18, labels: { name: 'temp.room2' } },
      { timestamp: 2, value: 26, labels: { name: 'temp.room1' } },
      { timestamp: 2, value: 19, labels: { name: 'temp.room2' } },
      { timestamp: 3, value: 21, labels: { name: 'temp.room1' } },
      { timestamp: 3, value: 23, labels: { name: 'temp.room2' } },
    ];
    render(<BarChartPanel panelId="p1" config={{ channel_name: 'c', mode: 'category' }} />);

    // 그룹 수 = 라벨 종류 수. 값은 각 라벨의 "최신"(마지막으로 본) 값이며
    // max/avg 가 아니다. 순서는 라벨이 처음 등장한 순서(Map 삽입 순서)를 따른다.
    expect(rows()).toEqual([
      { label: 'temp.room1', value: 21 },
      { label: 'temp.room2', value: 23 },
    ]);
  });

  it('CH-05: label_field 값이 없으면 unknown 그룹으로 묶고 max_points 는 뒤에서 자른다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 1 },
      { timestamp: 2, value: 2, labels: { name: 'B' } },
      { timestamp: 3, value: 3, labels: { name: 'C' } },
    ];
    render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category', max_points: 2 }}
      />,
    );
    // 3그룹(unknown/B/C) → 최근 2개만 남는다(slice(-2)).
    expect(rows()).toEqual([
      { label: 'B', value: 2 },
      { label: 'C', value: 3 },
    ]);
  });

  it('CH-06: time_bin 모드는 bin_sec 버킷 × agg_func(sum/count/avg) 로 접는다', () => {
    // bin1: [0, 60000) 에 10, 20 / bin2: [60000, 120000) 에 5
    const entries = [
      { timestamp: 1_000, value: 10 },
      { timestamp: 30_000, value: 20 },
      { timestamp: 60_001, value: 5 },
    ];

    mockResult.current.entries = entries;
    const sum = render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'time_bin', bin_sec: 60, agg_func: 'sum' }}
      />,
    );
    expect(rows().map((r) => r.value)).toEqual([30, 5]);
    sum.unmount();

    mockResult.current.entries = entries;
    const count = render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'time_bin', bin_sec: 60, agg_func: 'count' }}
      />,
    );
    expect(rows().map((r) => r.value)).toEqual([2, 1]);
    count.unmount();

    mockResult.current.entries = entries;
    const avg = render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'time_bin', bin_sec: 60, agg_func: 'avg' }}
      />,
    );
    expect(rows().map((r) => r.value)).toEqual([15, 5]);
    avg.unmount();
  });

  it('CH-07: 막대 채움색은 시리즈 색과 무관하게 하드코딩 #3b82f6 이다', () => {
    mockResult.current.entries = [
      { timestamp: 1, value: 10, labels: { name: 'A' } },
      { timestamp: 2, value: 20, labels: { name: 'B' } },
    ];
    render(<BarChartPanel panelId="p1" config={{ channel_name: 'c', mode: 'category' }} />);

    // Bar 는 카테고리 수와 무관하게 1개이며(dataKey='value'), 채움색은 단일 상수다.
    const bars = screen.getAllByTestId('rc-bar');
    expect(bars).toHaveLength(1);
    expect(bars[0]!.getAttribute('data-bar-key')).toBe('value');
    expect(bars[0]!.getAttribute('data-bar-fill')).toBe('#3b82f6');
  });
});
