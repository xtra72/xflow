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
      return storeHookResult.current;
    },
  };
});

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
    render(
      <PieChartPanel
        panelId="p1"
        config={{ channel_name: 'c', show_legend: false }}
      />,
    );
    expect(screen.queryByTestId('pie-chart-legend')).toBeNull();
  });

  it('show_legend=true (기본값) 이면 legend 렌더', () => {
    mockResult.current.entries = [{ timestamp: 1, value: 1, labels: { name: 'A' } }];
    render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    expect(screen.getByTestId('pie-chart-legend')).toBeTruthy();
  });

  describe('조각 라벨', () => {
    /** 렌더된 조각 라벨의 문구들. */
    const labels = (): string[] =>
      Array.from(document.querySelectorAll('[data-pie-slice-label]')).map(
        (e) => e.textContent ?? '',
      );

    beforeEach(() => {
      // 보고된 화면과 같은 구성: 96% / 3% / 1%.
      mockResult.current.entries = [
        { timestamp: 1, value: 96, labels: { name: 'en0' } },
        { timestamp: 2, value: 3, labels: { name: 'lo0' } },
        { timestamp: 3, value: 1, labels: { name: 'utun4' } },
      ];
    });

    it('기준(기본 5%) 미만 조각은 라벨을 적지 않는다', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      // 3% · 1% 는 접히고 96% 만 남는다 — 좁은 조각에 겹쳐 쌓이던 라벨이 사라진다.
      expect(labels()).toEqual(['96%']);
    });

    it('label_min_percent: 0 이면 모두 적는다 — 종전 동작으로 되돌리는 출구', () => {
      render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', label_min_percent: 0 }} />,
      );
      expect(labels()).toEqual(['96%', '3%', '1%']);
    });

    it('비율과 값을 함께 켜면 한 줄로 잇는다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{ channel_name: 'c', show_value: true, decimal_places: 0, unit: 'GB' }}
        />,
      );
      expect(labels()).toEqual(['96% 96GB']);
    });

    it('둘 다 끄면 라벨을 그리지 않는다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{ channel_name: 'c', show_percentage: false, show_value: false }}
        />,
      );
      expect(labels()).toEqual([]);
    });

    it('조각 안쪽에 놓는다 — 패널 경계에서 잘리지 않게', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      const el = document.querySelector('[data-pie-slice-label]')!;
      // 스텁 기하: cx=100, outerRadius=50, midAngle=0 → 반지름 60% 지점.
      expect(el.getAttribute('x')).toBe('130');
      expect(el.getAttribute('text-anchor')).toBe('middle');
    });

    it('바깥 배치는 recharts 가 준 자리를 그대로 쓴다 — 지시선 끝과 어긋나지 않게', () => {
      render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', label_position: 'outside' }} />,
      );
      // 스텁이 넘기는 바깥 좌표(x/y)는 안쪽 계산값(130)과 다르다.
      const el = document.querySelector('[data-pie-slice-label]')!;
      expect(el.getAttribute('x')).toBe('180');
    });

    it('글자색: 안쪽은 조각 색과 대비, 바깥은 테마색, 지정하면 그 색', () => {
      const { unmount } = render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      // 첫 조각은 팔레트 0번(#3b82f6, 어두움) → 흰 글자.
      expect(document.querySelector('[data-pie-slice-label]')!.getAttribute('fill')).toBe(
        '#ffffff',
      );
      unmount();

      const r2 = render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', label_position: 'outside' }} />,
      );
      expect(document.querySelector('[data-pie-slice-label]')!.getAttribute('fill')).toBe(
        'currentColor',
      );
      r2.unmount();

      render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', label_font_color: '#ff0000' }} />,
      );
      expect(document.querySelector('[data-pie-slice-label]')!.getAttribute('fill')).toBe(
        '#ff0000',
      );
    });

    it('글꼴은 지정했을 때만 싣는다 — 미지정은 상속', () => {
      const { unmount } = render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(
        document.querySelector('[data-pie-slice-label]')!.hasAttribute('font-family'),
      ).toBe(false);
      unmount();

      render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', label_font_family: 'mono' }} />,
      );
      expect(
        document.querySelector('[data-pie-slice-label]')!.getAttribute('font-family'),
      ).toContain('monospace');
    });

    it('글자 크기는 지정했을 때만 싣는다 — 미지정은 상속', () => {
      const { unmount } = render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />,
      );
      expect(document.querySelector('[data-pie-slice-label]')!.hasAttribute('font-size')).toBe(
        false,
      );
      unmount();

      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c', label_font_size: 18 }} />);
      expect(document.querySelector('[data-pie-slice-label]')!.getAttribute('font-size')).toBe(
        '18',
      );
    });
  });

  describe('파이 크기·위치', () => {
    const pie = () => screen.getByTestId('rc-pie');

    beforeEach(() => {
      mockResult.current.entries = [{ timestamp: 1, value: 1, labels: { name: 'A' } }];
    });

    it('기본은 한가운데 · 반지름 80%', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(pie().getAttribute('data-cx')).toBe('50%');
      expect(pie().getAttribute('data-cy')).toBe('50%');
      expect(pie().getAttribute('data-outer-radius')).toBe('80%');
    });

    it('바깥 라벨이면 자동으로 줄여 라벨 자리를 낸다', () => {
      render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', label_position: 'outside' }} />,
      );
      expect(pie().getAttribute('data-outer-radius')).toBe('62%');
    });

    it('라벨을 모두 끄면 바깥 배치라도 줄이지 않는다 — 낼 자리가 없다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            label_position: 'outside',
            show_percentage: false,
            show_value: false,
          }}
        />,
      );
      expect(pie().getAttribute('data-outer-radius')).toBe('80%');
    });

    it('지정 크기가 자동보다 이긴다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{ channel_name: 'c', label_position: 'outside', pie_size: 45 }}
        />,
      );
      expect(pie().getAttribute('data-outer-radius')).toBe('45%');
    });

    it('범위를 벗어난 크기는 무시하고 자동으로 돌아간다 — 파이가 사라지지 않게', () => {
      for (const bad of [0, -10, 150]) {
        const { unmount } = render(
          <PieChartPanel panelId="p1" config={{ channel_name: 'c', pie_size: bad }} />,
        );
        expect(pie().getAttribute('data-outer-radius')).toBe('80%');
        unmount();
      }
    });

    it('오프셋만큼 중심을 민다 — 백분율이라 패널 크기와 무관하다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{ channel_name: 'c', pie_offset_x: 10, pie_offset_y: -15 }}
        />,
      );
      expect(pie().getAttribute('data-cx')).toBe('60%');
      expect(pie().getAttribute('data-cy')).toBe('35%');
    });

    it('저장된 오프셋도 상한으로 죈다 — 손으로 고친 config 가 파이를 날리지 않게', () => {
      render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', pie_offset_x: 999 }} />,
      );
      expect(pie().getAttribute('data-cx')).toBe('90%');
    });

    it('드래그 레이어가 잡을 수 있도록 차트 영역에 표식을 남긴다', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(screen.getByTestId('pie-chart-container').hasAttribute('data-pie-chart-area')).toBe(
        true,
      );
    });
  });

  describe('범례(PieLegend) 배선', () => {
    beforeEach(() => {
      mockResult.current.entries = [
        { timestamp: 1, value: 3, labels: { name: 'A' } },
        { timestamp: 2, value: 1, labels: { name: 'B' } },
      ];
    });

    it('미지정이면 하단 배치 — 종전 동작과 같다', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      const legend = screen.getByTestId('pie-chart-legend');
      expect(legend.getAttribute('data-position')).toBe('bottom');
      // 하단은 가로로 흘러가므로 칸을 맞추지 않는다.
      expect(legend.getAttribute('data-aligned')).toBe('false');
    });

    it('left / right 는 세로 나열이라 칸을 맞춘다', () => {
      for (const position of ['left', 'right'] as const) {
        const { unmount } = render(
          <PieChartPanel panelId="p1" config={{ channel_name: 'c', legend_position: position }} />,
        );
        const legend = screen.getByTestId('pie-chart-legend');
        expect(legend.getAttribute('data-position')).toBe(position);
        expect(legend.getAttribute('data-aligned')).toBe('true');
        unmount();
      }
    });

    it('인식 불가 값은 하단으로 접는다 — 범례가 통째로 사라지지 않는다', () => {
      render(
        <PieChartPanel panelId="p1" config={{ channel_name: 'c', legend_position: 'top-left' }} />,
      );
      expect(screen.getByTestId('pie-chart-legend').getAttribute('data-position')).toBe('bottom');
    });

    it('기본은 이름만 — 비율·값 칸을 만들지 않는다', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      expect(screen.getAllByTestId('pie-legend-name')).toHaveLength(2);
      expect(screen.queryAllByTestId('pie-legend-percent')).toHaveLength(0);
      expect(screen.queryAllByTestId('pie-legend-value')).toHaveLength(0);
    });

    it('legend_show_percentage 는 조각 라벨과 같은 비율을 적는다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{ channel_name: 'c', legend_show_percentage: true }}
        />,
      );
      // 3 : 1 → 75% / 25%
      expect(screen.getAllByTestId('pie-legend-percent').map((e) => e.textContent)).toEqual([
        '75%',
        '25%',
      ]);
    });

    it('legend_show_value 는 자릿수·단위 규칙을 그대로 쓴다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{ channel_name: 'c', legend_show_value: true, decimal_places: 1, unit: 'kW' }}
        />,
      );
      expect(screen.getAllByTestId('pie-legend-value').map((e) => e.textContent)).toEqual([
        '3.0kW',
        '1.0kW',
      ]);
    });

    it('legend_font_size 와 끌어 옮긴 오프셋이 스타일로 실린다', () => {
      render(
        <PieChartPanel
          panelId="p1"
          config={{
            channel_name: 'c',
            legend_font_size: 18,
            legend_offset_x: 12,
            legend_offset_y: -4,
          }}
        />,
      );
      const style = screen.getByTestId('pie-chart-legend').getAttribute('style') ?? '';
      expect(style).toContain('font-size: 18px');
      // 하단 배치는 가운데 정렬 보정과 오프셋이 한 transform 에 합쳐진다.
      expect(style).toContain('translate(calc(-50% + 12px), -4px)');
    });

    it('오프셋이 0이어도 기준 자리 보정 transform 은 남는다', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
      const style = screen.getByTestId('pie-chart-legend').getAttribute('style') ?? '';
      // 가운데 정렬은 자기 폭의 절반을 되물려야 하므로 오프셋과 무관하게 필요하다.
      expect(style).toContain('translate(calc(-50% + 0px), 0px)');
    });

    it('파이 위에 겹쳐 뜬다 — 범례 위치를 바꿔도 파이가 따라 움직이지 않는다', () => {
      render(<PieChartPanel panelId="p1" config={{ channel_name: 'c', legend_position: 'right' }} />);
      expect(screen.getByTestId('pie-chart-legend').className).toContain('absolute');
      // 파이 기하는 범례 위치와 무관하다.
      expect(screen.getByTestId('rc-pie').getAttribute('data-cx')).toBe('50%');
      expect(screen.getByTestId('rc-pie').getAttribute('data-outer-radius')).toBe('80%');
    });
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
    render(<PieChartPanel panelId="p1" config={{ channel_name: 'c' }} />);
    // 범례는 더 이상 recharts 내장 `<Legend>` 가 아니라 PieLegend 로 그린다(끌어
    // 옮기기·칸 맞춤 때문). 기본 ON 이라는 특성 자체는 그대로 잠근다.
    expect(screen.getByTestId('pie-chart-legend')).toBeTruthy();
    // show_percentage 는 Pie 의 label 렌더러로만 전달되므로 스텁에서는 직접 확인하지
    // 않는다. 문구 규칙은 pieLabel 순수 모듈 테스트가 잠근다.
  });
});

// ---------------------------------------------------------------------------
// SPEC-TSDB-002 M2 — 특성화 테스트 (DDD PRESERVE).
//
// `PieChartPanel.tsx:89` 의 소스 활성 판정을 5분기로 잠근다.
//
// @spec SPEC-TSDB-002 §2.3 (U3) · §2.4 (U4) — plan.md §3.1 CT-01 ~ CT-05 / AC-09
// ---------------------------------------------------------------------------
describe('PieChartPanel 소스 활성 판정 특성화 (SPEC-TSDB-002 M2, CT-01~CT-05)', () => {
  function lastStoreCall(): { source: unknown; enabled: unknown } {
    return storeHookCalls.args.at(-1)!;
  }
  /** 렌더된 슬라이스 라벨 목록 — 채널/store 어느 쪽 entries 를 소비했는지 드러낸다. */
  function sliceNames(container: HTMLElement): string[] {
    return Array.from(container.querySelectorAll('.recharts-pie-sector')).map(
      (s) => s.getAttribute('data-name') ?? '',
    );
  }

  beforeEach(() => {
    storeHookCalls.args = [];
    mockResult.current = {
      entries: [{ timestamp: 1, value: 11, labels: { name: 'CH' } }],
      status: 'connected',
      closedReason: undefined,
      errorReason: undefined,
    };
    storeHookResult.current = {
      ...storeHookResult.current,
      entries: [{ timestamp: 1, value: 99, labels: { name: 'STORE' } }],
      status: 'connected',
    };
  });

  const base = { channel_name: 'c1', label_field: 'labels.name', agg_func: 'sum' } as const;

  it('CT-01: config 가 비어 있으면 채널 경로다(store 훅은 idle)', () => {
    const { container } = render(<PieChartPanel panelId="p1" config={{ ...base }} />);
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(sliceNames(container)).toEqual(['CH']);
  });

  it("CT-02: data_source:'channel' 이면 채널 경로다", () => {
    const { container } = render(
      <PieChartPanel panelId="p1" config={{ ...base, data_source: 'channel' }} />,
    );
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(sliceNames(container)).toEqual(['CH']);
  });

  it("CT-03: data_source:'store' 인데 store_source 가 없으면 채널 경로로 폴백한다", () => {
    const { container } = render(
      <PieChartPanel panelId="p1" config={{ ...base, data_source: 'store' }} />,
    );
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(sliceNames(container)).toEqual(['CH']);
  });

  it("CT-04: data_source:'store' + 시리즈 0개면 채널 경로로 폴백한다", () => {
    const { container } = render(
      <PieChartPanel
        panelId="p1"
        config={{
          ...base,
          data_source: 'store',
          store_source: { agent_name: 'store-1', series: [] },
        }}
      />,
    );
    expect(lastStoreCall()).toEqual({ source: undefined, enabled: false });
    expect(sliceNames(container)).toEqual(['CH']);
  });

  it("CT-05: data_source:'store' + 시리즈 N개면 store 경로다", () => {
    const store_source = { agent_name: 'store-1', series: [{ key: 'k1' }] };
    const { container } = render(
      <PieChartPanel panelId="p1" config={{ ...base, data_source: 'store', store_source }} />,
    );
    expect(lastStoreCall()).toEqual({ source: store_source, enabled: true });
    expect(sliceNames(container)).toEqual(['STORE']);
  });
});
