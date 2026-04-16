// LineChartPanel 테스트.
// Recharts 는 jsdom 환경에서 SVG 를 정상 렌더하지 못하므로
// __mocks__/rechartsStub 으로 대체해 데이터 흐름만 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, render, screen } from '@testing-library/react';

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

  // --- Y축 모드 ---
  describe('y_axis_mode', () => {
    it('기본(auto) 이면 YAxis domain = ["auto","auto"]', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 10 },
        { timestamp: 2000, value: 20 },
      ];
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      const y = screen.getByTestId('rc-yaxis');
      expect(JSON.parse(y.getAttribute('data-domain')!)).toEqual(['auto', 'auto']);
    });

    it('manual 이면 YAxis domain 에 y_min/y_max 반영', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 15 }];
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_axis_mode: 'manual',
            y_min: 0,
            y_max: 100,
          }}
        />,
      );
      const y = screen.getByTestId('rc-yaxis');
      expect(JSON.parse(y.getAttribute('data-domain')!)).toEqual([0, 100]);
    });

    it('auto_padded 이면 데이터 [min,max] 에 padding_pct 를 적용한 범위', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 10 },
        { timestamp: 2000, value: 30 },
      ];
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_axis_mode: 'auto_padded',
            y_axis_padding_pct: 10,
          }}
        />,
      );
      const y = screen.getByTestId('rc-yaxis');
      const dom = JSON.parse(y.getAttribute('data-domain')!) as [number, number];
      // range=20, pad=2 → [8, 32]
      expect(dom[0]).toBeCloseTo(8);
      expect(dom[1]).toBeCloseTo(32);
    });

    it('auto_padded + 유효 숫자 없음 → fallback ["auto","auto"]', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 'not-a-number' },
      ];
      render(
        <LineChartPanel
          panelId="p1"
          config={{ channel_name: 'c', y_axis_mode: 'auto_padded' }}
        />,
      );
      const y = screen.getByTestId('rc-yaxis');
      expect(JSON.parse(y.getAttribute('data-domain')!)).toEqual(['auto', 'auto']);
    });
  });

  // --- 시간 윈도우 모드 ---
  describe('time_window_mode', () => {
    it('기본(points) 이면 XAxis domain = ["dataMin","dataMax"], 필터 없음', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 1 },
        { timestamp: 2000, value: 2 },
      ];
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      const x = screen.getByTestId('rc-xaxis');
      expect(JSON.parse(x.getAttribute('data-domain')!)).toEqual(['dataMin', 'dataMax']);
      const rows = JSON.parse(screen.getByTestId('rc-line-chart').getAttribute('data-rows')!);
      expect(rows).toHaveLength(2);
    });

    it('fixed 모드: 범위 밖 entries 는 필터링, XAxis domain=[start,end]', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 1 }, // 범위 밖
        { timestamp: 1500, value: 2 },
        { timestamp: 2500, value: 3 },
        { timestamp: 3500, value: 4 }, // 범위 밖
      ];
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            time_window_mode: 'fixed',
            fixed_start_ms: 1500,
            fixed_end_ms: 2500,
          }}
        />,
      );
      const x = screen.getByTestId('rc-xaxis');
      expect(JSON.parse(x.getAttribute('data-domain')!)).toEqual([1500, 2500]);
      const rows = JSON.parse(
        screen.getByTestId('rc-line-chart').getAttribute('data-rows')!,
      ) as Array<{ timestamp: number }>;
      expect(rows.map((r) => r.timestamp)).toEqual([1500, 2500]);
    });

    it('recent 모드: 현재 시각 기준 [now-window, now] 범위 필터', () => {
      vi.useFakeTimers();
      try {
        const now = 10_000;
        vi.setSystemTime(now);
        mockResult.current.entries = [
          { timestamp: 3000, value: 1 }, // 범위 밖 (7초 전보다 오래됨)
          { timestamp: 5000, value: 2 },
          { timestamp: 8000, value: 3 },
          { timestamp: 9500, value: 4 },
        ];
        render(
          <LineChartPanel
            panelId="p1"
            config={{
              channel_name: 'c',
              time_window_mode: 'recent',
              recent_window_sec: 5, // window = 5초 → start = 5000
            }}
          />,
        );
        const x = screen.getByTestId('rc-xaxis');
        expect(JSON.parse(x.getAttribute('data-domain')!)).toEqual([5000, 10000]);
        const rows = JSON.parse(
          screen.getByTestId('rc-line-chart').getAttribute('data-rows')!,
        ) as Array<{ timestamp: number }>;
        expect(rows.map((r) => r.timestamp)).toEqual([5000, 8000, 9500]);
      } finally {
        vi.useRealTimers();
      }
    });

    it('recent 모드: time_window_refresh_ms 주기로 현재 시각 갱신', () => {
      vi.useFakeTimers();
      try {
        vi.setSystemTime(10_000);
        mockResult.current.entries = [{ timestamp: 8000, value: 1 }];
        render(
          <LineChartPanel
            panelId="p1"
            config={{
              channel_name: 'c',
              time_window_mode: 'recent',
              recent_window_sec: 5,
              time_window_refresh_ms: 500,
            }}
          />,
        );
        // 초기 domain: [5000, 10000]
        let x = screen.getByTestId('rc-xaxis');
        expect(JSON.parse(x.getAttribute('data-domain')!)).toEqual([5000, 10000]);

        // 500ms 경과 → advanceTimersByTime 이 mocked Date 도 전진시키므로 now=10500
        act(() => {
          vi.advanceTimersByTime(500);
        });
        x = screen.getByTestId('rc-xaxis');
        expect(JSON.parse(x.getAttribute('data-domain')!)).toEqual([5500, 10500]);
      } finally {
        vi.useRealTimers();
      }
    });
  });
});
