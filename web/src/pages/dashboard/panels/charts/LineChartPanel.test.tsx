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

const csvMocks = vi.hoisted(() => ({
  downloadCsv: vi.fn(),
}));

// SPEC-WEB-005: store 소스 훅 모킹. seriesEntries/seriesStyles 를 주입해
// store 모드 라인 렌더 + per-line 스타일 적용을 검증한다.
const storeMockResult = vi.hoisted(() => ({
  current: {
    entries: [] as ChartEntry[],
    seriesEntries: new Map<string, ChartEntry[]>(),
    seriesStyles: new Map<
      string,
      {
        color?: string;
        stroke_style?: 'solid' | 'dashed' | 'dotted';
        stroke_width?: number;
        smooth?: boolean;
      }
    >(),
    seriesNames: [] as string[],
    booleanSeries: new Set<string>(),
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

// SPEC-TSDB-002 M2 특성화용 호출 인자 기록기. 반환값은 종전과 동일하므로 기존
// 테스트의 동작은 바뀌지 않고, "어느 훅이 활성으로 호출됐는가" 만 관측 가능해진다.
const hookCalls = vi.hoisted(() => ({
  store: [] as Array<{ source: unknown; enabled: unknown }>,
}));

// 채널이 패널 소스에서 빠진 뒤로 데이터 이음매는 **하나**다(`usePanelSeriesData`).
//
// 이 파일의 오래된 테스트들은 단일 타임라인(`mockResult.current.entries`)을 심어 왔다.
// 그 형상을 시리즈 하나로 옮겨 주는 어댑터를 여기 둔다 — 테스트 30여 개의 본문을 고치는
// 대신, "채널 시절의 한 줄" 을 "시리즈 소스의 한 줄" 로 읽는다. 여러 줄을 보는 테스트는
// `storeMockResult` 를 직접 심으며, 그때는 그쪽이 이긴다.
vi.mock('./usePanelSeriesData', () => ({
  usePanelSeriesData: (source?: unknown, enabled?: unknown) => {
    hookCalls.store.push({ source, enabled });
    const store = storeMockResult.current;
    if (store.seriesEntries.size > 0 || store.seriesNames.length > 0) return store;
    const entries = mockResult.current.entries;
    return {
      ...store,
      entries,
      seriesEntries: entries.length > 0 ? new Map([['value', entries]]) : new Map(),
      seriesNames: entries.length > 0 ? ['value'] : [],
      status: mockResult.current.status,
      closedReason: mockResult.current.closedReason,
      errorReason: mockResult.current.errorReason,
    };
  },
  // 이 파일은 렌더·데이터 흐름을 본다. 소스 바인딩 판정은 panelDataSource 테스트가 본다.
  isPanelSeriesSource: () => true,
}));

// 활성 판정도 같은 사유로 참으로 고정한다 — 이 파일의 config 들은 소스 블록을 갖추지 않고
// 훅 결과를 직접 심으므로, 실제 판정을 태우면 패널이 "고른 시리즈 없음" 으로 비어 버린다.
// 나머지 export 는 원본을 그대로 쓴다(축 창·종류 판정은 실제 규칙이 돌아야 한다).
vi.mock('./panelDataSource', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./panelDataSource')>()),
  isPanelSeriesActive: () => true,
}));

// 캔들은 조회 계층(react-query)을 쓴다 — QueryClient 없이 렌더하려고 비활성으로 흉내 낸다.
vi.mock('./useCandleSeriesData', () => ({
  useCandleSeriesData: () => new Map(),
}));

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
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
    storeMockResult.current = {
      entries: [],
      seriesEntries: new Map(),
      seriesStyles: new Map(),
      seriesNames: [],
      booleanSeries: new Set(),
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
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

  // --- 값 타입 처리: 스트링 제외 / int·float 혼합 / boolean true-false ---
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
      const container = screen.getByTestId('line-chart-root');
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
      const container = screen.getByTestId('line-chart-root');
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
      const container = screen.getByTestId('line-chart-root');
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
      const container = screen.getByTestId('line-chart-root');
      expect(container.className).toMatch(/animate-pulse/);
    });
  });

  // --- 다채널 비교 ---
  // --- CSV 내보내기 ---
  describe('csv export', () => {
    it('CSV 내보내기 버튼이 헤더에 렌더', () => {
      render(<LineChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(screen.getByTestId('line-chart-csv-button')).toBeInTheDocument();
    });

    // 파일 이름의 출처가 바뀌었다 — 채널 이름이 사라졌으므로 패널 제목을 쓴다.
    it('CSV 버튼 클릭 시 downloadCsv 가 패널 제목+timestamp 파일명으로 호출', () => {
      mockResult.current.entries = [
        { timestamp: 1000, value: 10 },
        { timestamp: 2000, value: 20 },
      ];
      render(<LineChartPanel panelId="p1" title="temp_a" config={{}} />);
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

    it('제목이 없으면 기본 파일 이름으로 떨어진다', () => {
      mockResult.current.entries = [{ timestamp: 1000, value: 10 }];
      render(<LineChartPanel panelId="p1" config={{}} />);
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-csv-button'));
      });
      const [, filename] = csvMocks.downloadCsv.mock.calls[0]!;
      expect(filename).toMatch(/^chart-.*\.csv$/);
    });

    // 컬럼은 **그려진 시리즈**가 정한다. 채널 시절에는 줄이 없어도 `value` 한 칸이
    // 고정으로 있었지만, 시리즈 소스는 이름을 결과에서 얻으므로 빈 결과면 칸도 없다.
    it('CSV 버튼: 그릴 시리즈가 없으면 시간 컬럼만 나온다', () => {
      mockResult.current.entries = [];
      render(<LineChartPanel panelId="p1" config={{}} />);
      act(() => {
        fireEvent.click(screen.getByTestId('line-chart-csv-button'));
      });
      const [csv] = csvMocks.downloadCsv.mock.calls[0]!;
      expect(csv.trim()).toBe('timestamp,iso');
    });
  });

  // SPEC-WEB-005: store 데이터 소스 모드 — 시리즈별 라인 + per-line 스타일.
  describe('store 데이터 소스 모드', () => {
    const storeConfig = {
      data_source: 'store',
      store_source: {
        agent_name: 'store-1',
        series: [
          { key: 'room:1:temp', alias: 'Temp' },
          { key: 'room:2:temp', alias: 'Hum' },
        ],
        time_window_ms: 60000,
        interval_ms: 10000,
        aggregation: 'average',
      },
    };

    it('store 시리즈가 각각 라인으로 렌더된다(시리즈 표시 이름 기준)', () => {
      storeMockResult.current.seriesEntries = new Map([
        ['Temp', [{ timestamp: 1000, value: 21 }, { timestamp: 2000, value: 22 }]],
        ['Hum', [{ timestamp: 1000, value: 40 }, { timestamp: 2000, value: 41 }]],
      ]);
      render(<LineChartPanel panelId="p1" config={storeConfig} />);
      const lines = screen.getAllByTestId('rc-line');
      const keys = lines.map((l) => l.getAttribute('data-line-key'));
      expect(keys).toContain('Temp');
      expect(keys).toContain('Hum');
    });

    it('X축을 설정된 time_window_ms 범위로 고정한다(데이터 실제 범위와 무관)', () => {
      storeMockResult.current.seriesEntries = new Map([
        ['Temp', [{ timestamp: 1000, value: 21 }, { timestamp: 2000, value: 22 }]],
      ]);
      render(<LineChartPanel panelId="p1" config={storeConfig} />);
      const domain = JSON.parse(screen.getByTestId('rc-xaxis').getAttribute('data-domain')!);
      expect(typeof domain[0]).toBe('number');
      expect(typeof domain[1]).toBe('number');
      // 설정 윈도우(60000ms)로 고정 — 데이터 실제 범위(1000~2000)가 아니라 [now-60000, now].
      expect(domain[1] - domain[0]).toBe(60000);
    });

    it('태그 모드(series 빈, tag_filters 존재)에서도 store 라인이 활성화되어 렌더된다', () => {
      storeMockResult.current.seriesEntries = new Map([
        ['temp/room1/a', [{ timestamp: 1000, value: 21 }, { timestamp: 2000, value: 22 }]],
      ]);
      const tagConfig = {
        data_source: 'store',
        store_source: {
          agent_name: 'store-1',
          series: [],
          selection_mode: 'tag',
          tag_filters: { room: '1' },
          time_window_ms: 60000,
          interval_ms: 10000,
          aggregation: 'average',
        },
      };
      render(<LineChartPanel panelId="p1" config={tagConfig} />);
      const keys = screen.getAllByTestId('rc-line').map((l) => l.getAttribute('data-line-key'));
      expect(keys).toContain('temp/room1/a');
    });

    it('booleanSeries(store data_type=boolean)는 Y축을 false/true 로 표시한다', () => {
      // store 값은 이미 1/0 로 변환되어 도달하며, booleanSeries 로 표시 대상을 판별한다.
      storeMockResult.current.seriesEntries = new Map([
        ['Power', [{ timestamp: 1000, value: 1 }, { timestamp: 2000, value: 0 }]],
      ]);
      storeMockResult.current.booleanSeries = new Set(['Power']);
      render(<LineChartPanel panelId="p1" config={storeConfig} />);
      expect(
        JSON.parse(screen.getByTestId('rc-yaxis').getAttribute('data-domain')!),
      ).toEqual([-0.1, 1.1]);
    });

    it('seriesStyles 의 per-line 스타일(색/두께/대시/곡선)이 라인에 적용된다', () => {
      storeMockResult.current.seriesEntries = new Map([
        ['Temp', [{ timestamp: 1000, value: 21 }, { timestamp: 2000, value: 22 }]],
      ]);
      storeMockResult.current.seriesStyles = new Map([
        ['Temp', { color: '#ff0000', stroke_style: 'dashed', stroke_width: 4, smooth: true }],
      ]);
      render(<LineChartPanel panelId="p1" config={storeConfig} />);
      const line = screen
        .getAllByTestId('rc-line')
        .find((l) => l.getAttribute('data-line-key') === 'Temp')!;
      expect(line.getAttribute('data-line-stroke')).toBe('#ff0000');
      expect(line.getAttribute('data-line-width')).toBe('4');
      // dashed → STROKE_DASHARRAY['dashed'] = '8 4'
      expect(line.getAttribute('data-line-dash')).toBe('8 4');
      // smooth=true → type 'monotone'
      expect(line.getAttribute('data-line-type')).toBe('monotone');
    });

    it('스타일 미지정 시 기본값(팔레트 색/두께 2/실선/linear)을 사용한다', () => {
      storeMockResult.current.seriesEntries = new Map([
        ['Temp', [{ timestamp: 1000, value: 21 }]],
      ]);
      // seriesStyles 비어있음.
      render(<LineChartPanel panelId="p1" config={storeConfig} />);
      const line = screen
        .getAllByTestId('rc-line')
        .find((l) => l.getAttribute('data-line-key') === 'Temp')!;
      expect(line.getAttribute('data-line-width')).toBe('2');
      expect(line.getAttribute('data-line-dash')).toBe('');
      expect(line.getAttribute('data-line-type')).toBe('linear');
    });
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `LineChartPanel.tsx:245` 의 소스 활성 판정(`config.data_source === 'store' && ...`)
// 현재 동작을 잠근다. M3 에서 이 지점이 `resolvePanelSourceBinding(config).active` +
// `usePanelSeriesData(config)` 로 치환되어도 아래 5분기는 **동일한 훅 활성 조합**을
// 내야 한다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.1 CT-01 ~ CT-05 / AC-09
// ---------------------------------------------------------------------------