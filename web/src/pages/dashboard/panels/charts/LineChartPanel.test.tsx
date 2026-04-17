// LineChartPanel 테스트.
// Recharts 는 jsdom 환경에서 SVG 를 정상 렌더하지 못하므로
// __mocks__/rechartsStub 으로 대체해 데이터 흐름만 검증한다.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, fireEvent, render, screen } from '@testing-library/react';

import type { ChartEntry } from './chartChannelTypes';

const mockResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

const multiMockResult = vi.hoisted(() => ({
  current: {
    channels: new Map<
      string,
      {
        entries: ChartEntry[];
        status: 'idle' | 'connecting' | 'connected' | 'closed' | 'error';
        closedReason?: string;
        errorReason?: string;
      }
    >(),
  },
}));

const csvMocks = vi.hoisted(() => ({
  downloadCsv: vi.fn(),
}));

vi.mock('./useChartChannel', () => ({
  useChartChannel: () => mockResult.current,
}));

vi.mock('./useChartChannels', () => ({
  useChartChannels: () => multiMockResult.current,
}));

vi.mock('./csvExport', async () => {
  const actual = await vi.importActual<typeof import('./csvExport')>('./csvExport');
  return { ...actual, downloadCsv: csvMocks.downloadCsv };
});

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
    multiMockResult.current = { channels: new Map() };
    csvMocks.downloadCsv.mockReset();
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

  // --- 일시정지/재개 ---
  describe('pause/resume', () => {
    it('일시정지 버튼이 헤더에 렌더', () => {
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(screen.getByTestId('line-chart-pause-button')).toBeInTheDocument();
    });

    it('일시정지 토글 시 새 entries 가 무시되고 스냅샷 유지', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 10 }];
      const { rerender } = render(
        <LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
      );
      expect(
        JSON.parse(screen.getByTestId('rc-line-chart').getAttribute('data-rows')!),
      ).toHaveLength(1);

      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-pause-button'));
      });

      // 새 entry 도착
      mockResult.current.entries = [
        { timestamp: 1000, value: 10 },
        { timestamp: 2000, value: 20 },
      ];
      rerender(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);

      // 여전히 스냅샷 (1개)
      expect(
        JSON.parse(screen.getByTestId('rc-line-chart').getAttribute('data-rows')!),
      ).toHaveLength(1);
    });

    it('재개 시 라이브 entries 로 복귀', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 10 }];
      const { rerender } = render(
        <LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
      );
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-pause-button'));
      });
      mockResult.current.entries = [
        { timestamp: 1000, value: 10 },
        { timestamp: 2000, value: 20 },
      ];
      rerender(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      // Resume
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-pause-button'));
      });
      expect(
        JSON.parse(screen.getByTestId('rc-line-chart').getAttribute('data-rows')!),
      ).toHaveLength(2);
    });

    it('일시정지 상태에서 시각 배지 노출', () => {
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(screen.queryByTestId('line-chart-pause-badge')).toBeNull();
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-pause-button'));
      });
      expect(screen.getByTestId('line-chart-pause-badge')).toBeInTheDocument();
    });

    it('recent 모드 일시정지 시 시간 윈도우(now) 도 정지', () => {
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
        // pause 직전 domain: [5000, 10000]
        act(() => {
          fireEvent.click(screen.getByTestId('line-chart-pause-button'));
        });
        // 시간 경과
        act(() => {
          vi.advanceTimersByTime(2000);
        });
        // 일시정지 중이므로 domain 변화 없음
        expect(
          JSON.parse(screen.getByTestId('rc-xaxis').getAttribute('data-domain')!),
        ).toEqual([5000, 10000]);
      } finally {
        vi.useRealTimers();
      }
    });
  });

  // --- Y축 임계선 ---
  describe('y_thresholds', () => {
    it('thresholds 미지정: ReferenceLine 미렌더', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 10 }];
      const { container } = render(
        <LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
      );
      expect(container.querySelectorAll('.recharts-reference-line')).toHaveLength(0);
    });

    it('thresholds 개수만큼 ReferenceLine 렌더', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 10 }];
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_thresholds: [
              { value: 80, severity: 'warning' },
              { value: 100, severity: 'critical' },
            ],
          }}
        />,
      );
      const lines = container.querySelectorAll('.recharts-reference-line');
      expect(lines).toHaveLength(2);
      expect(lines[0]!.getAttribute('data-ref-y')).toBe('80');
      expect(lines[1]!.getAttribute('data-ref-y')).toBe('100');
    });

    it('color 미지정 시 severity 기본 색상 적용', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 10 }];
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_thresholds: [
              { value: 80, severity: 'warning' },
              { value: 100, severity: 'critical' },
              { value: 50, severity: 'info' },
            ],
          }}
        />,
      );
      const lines = container.querySelectorAll('.recharts-reference-line');
      expect(lines[0]!.getAttribute('data-ref-stroke')).toBe('#f59e0b');
      expect(lines[1]!.getAttribute('data-ref-stroke')).toBe('#ef4444');
      expect(lines[2]!.getAttribute('data-ref-stroke')).toBe('#3b82f6');
    });

    it('명시적 color 가 severity 기본보다 우선', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 10 }];
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_thresholds: [{ value: 80, severity: 'warning', color: '#000000' }],
          }}
        />,
      );
      const line = container.querySelector('.recharts-reference-line')!;
      expect(line.getAttribute('data-ref-stroke')).toBe('#000000');
    });

    it('critical 임계 초과 시 alert 컨테이너 클래스 부여', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 50 },
        { timestamp: 2000, value: 120 }, // critical=100 초과
      ];
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_thresholds: [{ value: 100, severity: 'critical' }],
          }}
        />,
      );
      const container = screen.getByTestId('line-chart-container').parentElement!;
      expect(container.className).toMatch(/animate-pulse/);
    });

    it('critical 임계 미초과 시 alert 클래스 없음', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 50 }];
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_thresholds: [{ value: 100, severity: 'critical' }],
          }}
        />,
      );
      const container = screen.getByTestId('line-chart-container').parentElement!;
      expect(container.className).not.toMatch(/animate-pulse/);
    });

    it('warning 임계 초과는 깜빡이지 않음 (critical 만 깜빡임)', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 90 }];
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            y_thresholds: [{ value: 80, severity: 'warning' }],
          }}
        />,
      );
      const container = screen.getByTestId('line-chart-container').parentElement!;
      expect(container.className).not.toMatch(/animate-pulse/);
    });

    it('multi-series 에서도 시리즈 중 하나라도 critical 초과면 깜빡임', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 50, labels: { room: 'A' } },
        { timestamp: 1000, value: 110, labels: { room: 'B' } }, // critical 초과
      ];
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            multi_series_field: 'labels.room',
            y_thresholds: [{ value: 100, severity: 'critical' }],
          }}
        />,
      );
      const container = screen.getByTestId('line-chart-container').parentElement!;
      expect(container.className).toMatch(/animate-pulse/);
    });
  });

  // --- 다채널 비교 ---
  describe('multi-channel mode', () => {
    it('channels 미지정: 기존 단일 채널 동작 (회귀 방지)', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 10 },
        { timestamp: 2000, value: 20 },
      ];
      const { container } = render(
        <LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
      );
      const lines = container.querySelectorAll('.recharts-line');
      expect(lines.length).toBe(1);
      expect(lines[0]!.getAttribute('data-line-key')).toBe('value');
    });

    it('channels 지정: 채널마다 1개 라인', () => {
      multiMockResult.current.channels = new Map([
        [
          'temp_a',
          {
            entries: [
              { timestamp: 1000, value: 10 },
              { timestamp: 2000, value: 20 },
            ],
            status: 'connected',
          },
        ],
        [
          'temp_b',
          {
            entries: [
              { timestamp: 1000, value: 100 },
              { timestamp: 2000, value: 200 },
            ],
            status: 'connected',
          },
        ],
      ]);
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{
            channels: [{ name: 'temp_a' }, { name: 'temp_b' }],
          }}
        />,
      );
      const lines = container.querySelectorAll('.recharts-line');
      expect(lines.length).toBe(2);
      const keys = Array.from(lines).map((l) => l.getAttribute('data-line-key'));
      expect(keys).toContain('temp_a');
      expect(keys).toContain('temp_b');
    });

    it('channels alias 가 라인 키로 사용', () => {
      multiMockResult.current.channels = new Map([
        [
          'temp_a',
          {
            entries: [{ timestamp: 1000, value: 10 }],
            status: 'connected',
          },
        ],
      ]);
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{
            channels: [{ name: 'temp_a', alias: 'Living Room' }],
          }}
        />,
      );
      const lines = container.querySelectorAll('.recharts-line');
      expect(lines[0]!.getAttribute('data-line-key')).toBe('Living Room');
    });

    it('채널마다 다른 display_field 적용', () => {
      multiMockResult.current.channels = new Map([
        [
          'sensor_a',
          {
            entries: [{ timestamp: 1000, value: { temp: 10, humid: 50 } }],
            status: 'connected',
          },
        ],
        [
          'sensor_b',
          {
            entries: [{ timestamp: 1000, value: { temp: 20, humid: 60 } }],
            status: 'connected',
          },
        ],
      ]);
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channels: [
              { name: 'sensor_a', display_field: 'value.temp' },
              { name: 'sensor_b', display_field: 'value.humid' },
            ],
          }}
        />,
      );
      const rows = JSON.parse(
        screen.getByTestId('rc-line-chart').getAttribute('data-rows')!,
      ) as Array<Record<string, unknown>>;
      expect(rows[0]!.sensor_a).toBe(10);
      expect(rows[0]!.sensor_b).toBe(60);
    });

    it('channels × multi_series_field 조합: alias::label 키', () => {
      multiMockResult.current.channels = new Map([
        [
          'flow_a',
          {
            entries: [
              { timestamp: 1000, value: 10, labels: { room: 'X' } },
              { timestamp: 1000, value: 11, labels: { room: 'Y' } },
            ],
            status: 'connected',
          },
        ],
        [
          'flow_b',
          {
            entries: [
              { timestamp: 1000, value: 20, labels: { room: 'X' } },
            ],
            status: 'connected',
          },
        ],
      ]);
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{
            channels: [
              { name: 'flow_a', alias: 'A' },
              { name: 'flow_b', alias: 'B' },
            ],
            multi_series_field: 'labels.room',
          }}
        />,
      );
      const lines = container.querySelectorAll('.recharts-line');
      const keys = Array.from(lines)
        .map((l) => l.getAttribute('data-line-key'))
        .sort();
      expect(keys).toEqual(['A::X', 'A::Y', 'B::X']);
    });

    it('channels 지정 시 channel_name 무시', () => {
      // channel_name 으로는 데이터 있지만 channels 가 우선
      mockResult.current.entries = [{ timestamp: 1000, value: 999 }];
      multiMockResult.current.channels = new Map([
        [
          'a',
          { entries: [{ timestamp: 1000, value: 10 }], status: 'connected' },
        ],
      ]);
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{
            channel_name: 'IGNORED',
            channels: [{ name: 'a' }],
          }}
        />,
      );
      const lines = container.querySelectorAll('.recharts-line');
      const keys = Array.from(lines).map((l) => l.getAttribute('data-line-key'));
      expect(keys).toEqual(['a']);
      expect(keys).not.toContain('value');
    });

    it('빈 channels 배열: 단일 모드로 fallback', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 5 }];
      const { container } = render(
        <LineChartPanel
          panelId="p1"
          config={{ channel_name: 'fallback', channels: [] }}
        />,
      );
      const lines = container.querySelectorAll('.recharts-line');
      expect(lines.length).toBe(1);
      expect(lines[0]!.getAttribute('data-line-key')).toBe('value');
    });

    it('채널별 상태 배지가 채널 수만큼 렌더', () => {
      multiMockResult.current.channels = new Map([
        ['a', { entries: [], status: 'connected' }],
        ['b', { entries: [], status: 'closed', closedReason: 'flow_undeployed' }],
      ]);
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channels: [
              { name: 'a', alias: 'A' },
              { name: 'b', alias: 'B' },
            ],
          }}
        />,
      );
      const badges = screen.getAllByTestId(/^line-chart-channel-status-/);
      expect(badges).toHaveLength(2);
      expect(screen.getByTestId('line-chart-channel-status-a').textContent).toContain('A');
      expect(screen.getByTestId('line-chart-channel-status-b').textContent).toContain('B');
    });

    it('단일 채널 모드: 채널별 배지 미렌더', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 5 }];
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(screen.queryAllByTestId(/^line-chart-channel-status-/)).toHaveLength(0);
    });

    it('채널별 배지에 상태별 data-status 속성', () => {
      multiMockResult.current.channels = new Map([
        ['a', { entries: [], status: 'connected' }],
        ['b', { entries: [], status: 'error', errorReason: 'channel_not_found' }],
      ]);
      render(
        <LineChartPanel
          panelId="p1"
          config={{ channels: [{ name: 'a' }, { name: 'b' }] }}
        />,
      );
      expect(
        screen.getByTestId('line-chart-channel-status-a').getAttribute('data-status'),
      ).toBe('connected');
      expect(
        screen.getByTestId('line-chart-channel-status-b').getAttribute('data-status'),
      ).toBe('error');
    });

    it('한 채널이라도 critical 임계 초과면 깜빡임', () => {
      multiMockResult.current.channels = new Map([
        [
          'a',
          {
            entries: [{ timestamp: 1000, value: 10 }],
            status: 'connected',
          },
        ],
        [
          'b',
          {
            entries: [{ timestamp: 1000, value: 150 }], // critical 초과
            status: 'connected',
          },
        ],
      ]);
      render(
        <LineChartPanel
          panelId="p1"
          config={{
            channels: [{ name: 'a' }, { name: 'b' }],
            y_thresholds: [{ value: 100, severity: 'critical' }],
          }}
        />,
      );
      const wrapper = screen.getByTestId('line-chart-container').parentElement!;
      expect(wrapper.className).toMatch(/animate-pulse/);
    });
  });

  // --- CSV 내보내기 ---
  describe('csv export', () => {
    it('CSV 내보내기 버튼이 헤더에 렌더', () => {
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(screen.getByTestId('line-chart-csv-button')).toBeInTheDocument();
    });

    it('CSV 버튼 클릭 시 downloadCsv 가 채널명+timestamp 파일명으로 호출', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 10 },
        { timestamp: 2000, value: 20 },
      ];
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'temp_a' }} />);
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-csv-button'));
      });
      expect(csvMocks.downloadCsv).toHaveBeenCalledTimes(1);
      const [csv, filename] = csvMocks.downloadCsv.mock.calls[0]!;
      expect(csv).toContain('timestamp,iso,value');
      expect(csv).toContain('1000,');
      expect(csv).toContain('2000,');
      expect(filename).toMatch(/^temp_a-.*\.csv$/);
    });

    it('CSV 버튼: entries 비었을 때도 헤더만 포함된 CSV 다운로드', () => {
      mockResult.current.entries = [];
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-csv-button'));
      });
      const [csv] = csvMocks.downloadCsv.mock.calls[0]!;
      expect(csv.trim()).toBe('timestamp,iso,value');
    });

    it('multi_series 적용 시 시리즈 컬럼 모두 포함', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 10, labels: { room: 'A' } },
        { timestamp: 1000, value: 20, labels: { room: 'B' } },
      ];
      render(
        <LineChartPanel
          panelId="p1"
          config={{ channel_name: 'c', multi_series_field: 'labels.room' }}
        />,
      );
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-csv-button'));
      });
      const [csv] = csvMocks.downloadCsv.mock.calls[0]!;
      const header = csv.split('\n')[0]!;
      expect(header).toContain('A');
      expect(header).toContain('B');
    });
  });
});
