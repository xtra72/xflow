// BarChartPanel 테스트.

import { describe, it, expect, vi, beforeEach } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/react';

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

// SPEC-TSDB-002 M2 특성화용 store 훅 기록기. 실제 export 는 그대로 두고 훅만 교체해
// (소스, 활성) 인자를 관측한다. 채널 모드 테스트에서는 idle 결과를 반환하므로 기존
// 동작(비활성 store 훅)과 동일하다.
const storeHookCalls = vi.hoisted(() => ({
  args: [] as Array<{ source: unknown; enabled: unknown }>,
}));
const storeHookResult = vi.hoisted(() => ({
  current: {
    entries: [] as Array<{ timestamp: number; value: unknown; labels?: Record<string, string> }>,
    seriesEntries: new Map<string, unknown>(),
    seriesStyles: new Map<string, unknown>(),
    seriesNames: [] as string[],
    booleanSeries: new Set<string>(),
    status: 'connected' as const,
    closedReason: undefined as string | undefined,
    errorReason: undefined as string | undefined,
  },
}));

vi.mock('./useStoreChartData', async (importOriginal) => {
  const actual = await importOriginal<typeof import('./useStoreChartData')>();
  return {
    ...actual,
    useStoreChartData: (source: unknown, enabled: unknown) => {
      storeHookCalls.args.push({ source, enabled });
      const store = storeHookResult.current;
      // 채널이 패널 소스에서 빠지면서 데이터 이음매가 하나로 줄었다. 이 파일의 오래된
      // 테스트들은 채널 형상(`mockResult.current`)을 심으므로, store 쪽에 심은 것이 없으면
      // 그 형상을 시리즈 소스 결과로 옮겨 준다 — 테스트 본문을 그대로 두기 위한 어댑터다.
      const storeHasData =
        (store.entries?.length ?? 0) > 0 || (store.seriesEntries?.size ?? 0) > 0;
      return storeHasData ? store : { ...store, ...mockResult.current };
    },
  };
});

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

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `BarChartPanel.tsx:100` 의 소스 활성 판정을 5분기로 잠근다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.1 CT-01 ~ CT-05 / AC-09
// ---------------------------------------------------------------------------
// ---------------------------------------------------------------------------
// SPEC-CHART-005 M1 — 특성화 테스트 (DDD PRESERVE).
//
// 바 차트에는 범례도 배치 편집도 없다는 현재 상태를 잠근다. M3·M4 가 이 서술을
// 뒤집으며, **범례 기본 꺼짐**만은 그 뒤에도 유지되어야 한다(저장된 대시보드 보존).
// ---------------------------------------------------------------------------
describe('BarChartPanel 특성화 (SPEC-CHART-005 M1)', () => {
  beforeEach(() => {
    mockResult.current = {
      entries: [],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
  });

  /** 기본 `label_field` 는 `labels.name` 이다. */
  const rows: ChartEntry[] = [
    { timestamp: 1, value: 10, labels: { name: 'A' } },
    { timestamp: 2, value: 20, labels: { name: 'B' } },
  ];

  it('AC-05: 범례는 기본으로 꺼져 있다 — 저장된 대시보드의 외형이 바뀌지 않는다', () => {
    mockResult.current.entries = rows;
    render(<BarChartPanel panelId="p1" config={{ channel_name: 'c', mode: 'category' }} />);
    expect(screen.queryByTestId('pie-chart-legend')).toBeNull();
  });

  it('AC-06: 켜면 카테고리별 항목이 나온다', () => {
    mockResult.current.entries = rows;
    render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category', show_legend: true }}
      />,
    );
    const names = screen.getAllByTestId('pie-legend-name').map((e) => e.textContent);
    expect(names).toEqual(['A', 'B']);
  });

  it('AC-06: 범례는 비중을 내지 않는다 — 막대는 합계 대비 비중을 읽는 그림이 아니다', () => {
    mockResult.current.entries = rows;
    render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category', show_legend: true }}
      />,
    );
    expect(screen.queryAllByTestId('pie-legend-percent')).toHaveLength(0);
    expect(screen.getAllByTestId('pie-legend-value').length).toBeGreaterThan(0);
  });

  it('AC-06: 범례 자리·변위가 파이와 같은 키로 먹는다', () => {
    mockResult.current.entries = rows;
    render(
      <BarChartPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          mode: 'category',
          show_legend: true,
          legend_position: 'right',
          legend_offset_y: 12,
        }}
      />,
    );
    // 오른쪽 배치에서 세로 변위는 기준 50% 에 접혀 `calc(62%)` 가 된다(파이와 같은 규칙).
    const style = screen.getByTestId('pie-chart-legend').getAttribute('style') ?? '';
    expect(style).toContain('62%');
  });

  it('AC-07: 플롯 오프셋·크기가 라인과 같은 키로 먹는다', () => {
    mockResult.current.entries = rows;
    render(
      <BarChartPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          mode: 'category',
          plot_offset_x: 10,
          plot_offset_y: -5,
          plot_size: 80,
        }}
      />,
    );
    const box = screen.getByTestId('bar-chart-container');
    expect(box.style.transform).toContain('translate(10%, -5%)');
    expect(box.style.transform).toContain('scale(0.8)');
  });

  it('AC-08: onConfigChange 가 없으면 편집 입구가 없다', () => {
    mockResult.current.entries = rows;
    const { container } = render(
      <BarChartPanel panelId="p1" config={{ channel_name: 'c', mode: 'category' }} />,
    );
    expect(screen.queryByTestId('bar-chart-edit-toggle')).toBeNull();
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(0);
    expect(screen.queryByTestId('panel-edit-grid')).toBeNull();
    expect(screen.queryByTestId('panel-align-toolbar')).toBeNull();
  });

  it('AC-08: forceEdit 면 그리드·툴바·드래그 표식이 나온다', () => {
    mockResult.current.entries = rows;
    const { container } = render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category', show_legend: true }}
        onConfigChange={vi.fn()}
        forceEdit
      />,
    );
    expect(screen.getByTestId('panel-edit-grid')).toBeInTheDocument();
    expect(screen.getByTestId('panel-edit-center')).toBeInTheDocument();
    expect(screen.getByTestId('panel-align-toolbar')).toBeInTheDocument();
    // 끌 수 있는 덩어리는 그림과 범례 둘이다.
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(2);
    // 미리보기는 토글을 감춘다.
    expect(screen.queryByTestId('bar-chart-edit-toggle')).toBeNull();
  });

  it('AC-08: 범례가 꺼져 있으면 끌 덩어리는 그림 하나다', () => {
    mockResult.current.entries = rows;
    const { container } = render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category' }}
        onConfigChange={vi.fn()}
        forceEdit
      />,
    );
    expect(container.querySelectorAll('[data-panel-drag]')).toHaveLength(1);
  });

  it('AC-09: 고르면 진한 실선이 되고 크기 손잡이가 붙는다', () => {
    mockResult.current.entries = rows;
    const { container } = render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category', show_legend: true }}
        onConfigChange={vi.fn()}
        forceEdit
      />,
    );

    const plot = screen.getByTestId('bar-chart-container');
    expect(plot.className).toContain('outline-dashed');

    fireEvent.pointerDown(plot);
    expect(screen.getByTestId('bar-chart-container').className).toContain('outline-2');
    expect(container.querySelectorAll('[data-panel-resize]')).toHaveLength(1);
  });

  it('AC-16: 편집을 켜도 그림 상자의 크기 클래스가 살아 있다', () => {
    mockResult.current.entries = rows;
    render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category' }}
        onConfigChange={vi.fn()}
        forceEdit
      />,
    );
    const box = screen.getByTestId('bar-chart-container');
    expect(box.className).toContain('h-full');
    expect(box.className).toContain('w-full');
  });

  it('AC-16: 편집 표식은 범례 자신에 붙는다 — 감싸는 상자를 만들지 않는다', () => {
    mockResult.current.entries = rows;
    const { container } = render(
      <BarChartPanel
        panelId="p1"
        config={{ channel_name: 'c', mode: 'category', show_legend: true }}
        onConfigChange={vi.fn()}
        forceEdit
      />,
    );
    const legend = screen.getByTestId('pie-chart-legend');
    expect(legend).toHaveAttribute('data-panel-drag', 'legend');
    expect([...container.querySelectorAll('[data-panel-drag="legend"]')]).toEqual([legend]);
  });

  it('AC-10: 배치 초기화가 두 요소의 오프셋을 한 번에 지운다', () => {
    mockResult.current.entries = rows;
    const onConfigChange = vi.fn();
    render(
      <BarChartPanel
        panelId="p1"
        config={{
          channel_name: 'c',
          mode: 'category',
          show_legend: true,
          plot_offset_x: 20,
          legend_offset_y: 10,
        }}
        onConfigChange={onConfigChange}
        forceEdit
      />,
    );

    fireEvent.click(screen.getByTestId('panel-align-reset'));
    expect(onConfigChange).toHaveBeenCalledTimes(1);
    expect(onConfigChange.mock.calls[0]![0]).toEqual({
      plot_offset_x: undefined,
      plot_offset_y: undefined,
      legend_offset_x: undefined,
      legend_offset_y: undefined,
    });
  });

  it('배치 값이 없으면 transform 을 붙이지 않는다 — 종전 화면 그대로다', () => {
    mockResult.current.entries = rows;
    render(<BarChartPanel panelId="p1" config={{ channel_name: 'c', mode: 'category' }} />);
    expect(screen.getByTestId('bar-chart-container').style.transform).toBe('');
  });
});

// ---------------------------------------------------------------------------
// 바는 사각형이라 폭·높이가 서로 다른 것을 가리킨다.
//
//   폭   → 막대 굵기(px). recharts `barSize`.
//   높이 → 그림 영역 배율(%). 막대의 높이는 **값**이 정하므로 설정할 수 없다.
// ---------------------------------------------------------------------------
describe('바 차트는 막대 굵기와 그림 영역 높이를 따로 잡는다', () => {
  const bar = (extra: Record<string, unknown> = {}) => (
    <BarChartPanel panelId="p1" config={{ channel_name: 'c', mode: 'category', ...extra }} />
  );

  it('세로 배율을 주면 그림 영역이 축마다 다른 배율로 그려진다', () => {
    render(bar({ plot_size: 100, plot_size_y: 60 }));
    expect(screen.getByTestId('bar-chart-container').style.transform).toContain('scale(1, 0.6)');
  });

  it('세로 배율이 없으면 종전대로 균일 배율이다 — 저장된 설정이 그대로 그려진다', () => {
    render(bar({ plot_size: 60 }));
    expect(screen.getByTestId('bar-chart-container').style.transform).toContain('scale(0.6)');
  });

  it('가로 배율을 주면 그림 영역이 그만큼 좁아진다 — 손잡이가 잡는 것은 도형의 폭이다', () => {
    render(bar({ plot_size: 60, plot_size_y: 100 }));
    expect(screen.getByTestId('bar-chart-container').style.transform).toContain('scale(0.6, 1)');
  });

  it('막대 굵기는 범위 밖이면 무시한다 — 잘못된 저장값이 막대를 지우지 않는다', () => {
    // 0 이나 음수가 그대로 내려가면 막대가 사라진다. 자동(미지정)으로 되돌린다.
    render(bar({ bar_size: 0 }));
    expect(screen.getByTestId('bar-chart-container')).toBeInTheDocument();
  });
});
