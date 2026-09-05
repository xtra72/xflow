// ChartPanelSections 단위 테스트.
// 각 섹션이 SPEC-CHART-001 §4.2.2 의 config 필드를 올바르게 렌더/편집하는지 검증.

import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';

// i18n 스텁 — 키를 그대로 반환하되, 보간 슬롯을 가진 키는 템플릿을 반환한다.
vi.mock('@/lib/i18n', () => {
  const templates: Record<string, string> = {
    'dashboard.chart.channelNameError': '유효한 채널 이름이 아닙니다',
    'dashboard.chart.channelOption': '{name} — flow {flow} ({count} subs)',
    'dashboard.chart.channelInactive': '{name} — (현재 선택, 비활성)',
    'dashboard.chart.channelInactiveShort': '{name} — (비활성)',
    // SPEC-CHART-002 M6.1/M6.2 — 보간 슬롯을 가진 안내 문구.
    'dashboard.chart.seriesReduceCombo':
      "현재 조합: {interval} 버킷을 '{agg}' 로 집계한 뒤, 구간 전체를 '{reduce}' 로 접습니다.",
    'dashboard.chart.seriesReduceBooleanHint':
      '불리언 시리즈가 선택되어 있습니다 — 이 대표값은 {meaning} 을(를) 뜻합니다.',
    'dashboard.chart.storeInfoSecUnit': '초',
  };
  return {
    useTranslation: () => ({ t: (k: string) => templates[k] ?? k }),
  };
});

// StoreSourceSection 은 store 에이전트 목록을 조회한다. QueryClient 없이 렌더 가능하도록
// 빈 목록으로 모킹한다(목록이 비면 저장된 agent_name 이 그대로 쓰인다 — SPEC-WEB-006).
vi.mock('@/hooks/useAgent', () => ({
  useAgents: () => ({ data: { data: [] } }),
}));

import {
  StoreSourceSection,
  StatChartSection,
  LineChartSection,
  BarChartSection,
  PieChartSection,
  TableChartSection,
} from './ChartPanelSections';
import type { PanelConfig } from '@/stores/uiStore';

function makePanel(type: PanelConfig['type'], config: Record<string, unknown>): PanelConfig {
  return { id: 'p1', type, title: '테스트', config };
}


describe('StatChartSection', () => {
  it('display_field / unit / decimal_places 편집', () => {
    const onConfigChange = vi.fn();
    render(
      <StatChartSection
        panel={makePanel('stat', {
          display_field: 'value',
          unit: '',
          decimal_places: 2,
        })}
        onConfigChange={onConfigChange}
      />,
    );
    // unit 은 목록에서 고른다 — 자유 텍스트 칸은 "직접 입력" 을 골랐을 때만 나타난다.
    fireEvent.change(screen.getByTestId('stat-unit'), { target: { value: 'kWh' } });
    expect(onConfigChange).toHaveBeenCalledWith({ unit: 'kWh' });
  });

  it('단위 "직접 입력" 을 고르면 입력칸이 나타나고 적은 값이 저장된다', () => {
    const onConfigChange = vi.fn();
    render(
      <StatChartSection panel={makePanel('stat', { unit: '' })} onConfigChange={onConfigChange} />,
    );
    // 처음에는 목록만 있다 — 종전처럼 늘 떠 있는 좁은 텍스트 칸은 없다.
    expect(screen.queryByTestId('stat-unit-custom')).not.toBeInTheDocument();

    fireEvent.change(screen.getByTestId('stat-unit'), { target: { value: '__custom__' } });
    const custom = screen.getByTestId('stat-unit-custom');
    fireEvent.change(custom, { target: { value: 'sccm' } });
    fireEvent.blur(custom);
    expect(onConfigChange).toHaveBeenCalledWith({ unit: 'sccm' });
  });

  it('목록에 없는 단위로 열리면 직접 입력 상태를 유지한다', () => {
    render(<StatChartSection panel={makePanel('stat', { unit: 'sccm' })} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('stat-unit-custom')).toHaveValue('sccm');
  });

  it('현재값 크기 배율을 편집한다', () => {
    const onConfigChange = vi.fn();
    render(<StatChartSection panel={makePanel('stat', {})} onConfigChange={onConfigChange} />);
    const slider = screen.getByTestId('stat-value-scale') as HTMLInputElement;
    // 미지정이면 1(기본 크기)에서 시작한다.
    expect(slider.value).toBe('1');

    fireEvent.change(slider, { target: { value: '1.5' } });
    expect(onConfigChange).toHaveBeenCalledWith({ value_scale: 1.5 });
  });

  it('초기화하면 config 에서 지운다 — 기본값을 저장해 두지 않는다', () => {
    const onConfigChange = vi.fn();
    render(
      <StatChartSection panel={makePanel('stat', { value_scale: 2 })} onConfigChange={onConfigChange} />,
    );
    expect((screen.getByTestId('stat-value-scale') as HTMLInputElement).value).toBe('2');

    fireEvent.click(screen.getByTestId('stat-value-scale-reset'));
    expect(onConfigChange).toHaveBeenCalledWith({ value_scale: undefined });
  });

  it('범위를 벗어난 저장값도 죄여서 보여준다', () => {
    render(<StatChartSection panel={makePanel('stat', { value_scale: 99 })} onConfigChange={vi.fn()} />);
    expect((screen.getByTestId('stat-value-scale') as HTMLInputElement).value).toBe('3');
  });

  it('threshold_color_rules 추가/제거', () => {
    const onConfigChange = vi.fn();
    render(
      <StatChartSection
        panel={makePanel('stat', { threshold_color_rules: [] })}
        onConfigChange={onConfigChange}
      />,
    );
    const addBtn = screen.getByRole('button', { name: /dashboard.chart.add/ });
    fireEvent.click(addBtn);
    expect(onConfigChange).toHaveBeenCalledWith({
      threshold_color_rules: [{ min: 0, color: '#3b82f6' }],
    });
  });
});

describe('LineChartSection', () => {

  it('LineChartSection 은 채널 편집기를 더 이상 렌더하지 않는다(전역 스타일만)', () => {
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { channels: [{ name: 'a' }] })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('line-chart-channels-editor')).toBeNull();
    expect(screen.queryByTestId('line-chart-add-channel')).toBeNull();
    // 전역 컨트롤(예: X축 범위)은 유지된다.
    expect(screen.getByTestId('line-chart-x-range-count')).toBeInTheDocument();
  });


  // --- 시간 윈도우 모드 ---
  it('구 config 없이 기본은 포인트 범위 — 기간 입력은 숨김', () => {
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('line-chart-x-range-count')).toBeInTheDocument();
    expect(screen.queryByTestId('line-chart-x-range-window')).toBeNull();
    expect(screen.queryByTestId('line-chart-x-range-start')).toBeNull();
  });

  it('구 time_window_mode=fixed 는 구간(absolute) 범위로 읽힌다', () => {
    render(
      <LineChartSection
        panel={makePanel('graph-chart', {
          time_window_mode: 'fixed',
          fixed_start_ms: 1000,
          fixed_end_ms: 2000,
        })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('line-chart-x-range-start')).toBeInTheDocument();
    expect(screen.getByTestId('line-chart-x-range-end')).toBeInTheDocument();
  });

  // --- Y축 모드 ---
  it('숫자형이면 최소/최대는 항상 노출되고, 비어 있으면 자동을 뜻한다', () => {
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={vi.fn()} />);
    const min = screen.getByTestId('line-chart-y-min') as HTMLInputElement;
    const max = screen.getByTestId('line-chart-y-max') as HTMLInputElement;
    expect(min.value).toBe('');
    expect(max.value).toBe('');
  });

  it('최소값을 넣으면 방식(manual)이 함께 저장된다', () => {
    const onConfigChange = vi.fn();
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={onConfigChange} />);
    fireEvent.change(screen.getByTestId('line-chart-y-min'), { target: { value: '3' } });
    expect(onConfigChange).toHaveBeenCalledWith({ y_min: 3, y_axis_mode: 'manual' });
  });

  it('최소·최대를 모두 비우면 방식이 자동으로 돌아간다', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { y_axis_mode: 'manual', y_min: 3 })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.change(screen.getByTestId('line-chart-y-min'), { target: { value: '' } });
    expect(onConfigChange).toHaveBeenCalledWith({ y_min: undefined, y_axis_mode: 'auto' });
  });

  it('자동 여백은 Y축 디자인 배지 안에 있다', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { y_axis_mode: 'auto_padded', y_axis_padding_pct: 5 })}
        onConfigChange={onConfigChange}
      />,
    );
    // 접혀 있다.
    expect(screen.queryByTestId('line-chart-y-design-pad-pct')).toBeNull();

    fireEvent.click(screen.getByTestId('line-chart-y-design-button'));
    const pad = screen.getByTestId('line-chart-y-design-pad-pct') as HTMLInputElement;
    expect(pad.value).toBe('5');

    fireEvent.change(pad, { target: { value: '10' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      y_axis_padding_pct: 10,
      y_axis_mode: 'auto_padded',
    });
  });

  it('자동 여백을 비우면 방식이 자동으로 돌아간다', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { y_axis_mode: 'auto_padded', y_axis_padding_pct: 5 })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('line-chart-y-design-button'));
    fireEvent.change(screen.getByTestId('line-chart-y-design-pad-pct'), { target: { value: '' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      y_axis_padding_pct: undefined,
      y_axis_mode: 'auto',
    });
  });

  it('X축 디자인 배지에는 여백 칸이 없다 — X축에는 없는 개념이다', () => {
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={vi.fn()} />);
    fireEvent.click(screen.getByTestId('line-chart-x-design-button'));
    expect(screen.getByTestId('line-chart-x-design-popover')).toBeInTheDocument();
    expect(screen.queryByTestId('line-chart-x-design-pad-pct')).toBeNull();
  });
});

describe('LineChartSection — 툴팁', () => {
  it('기본은 사용 켬 + 단일 값 끔', () => {
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={vi.fn()} />);
    expect((screen.getByTestId('line-chart-tooltip-enabled') as HTMLInputElement).checked).toBe(
      true,
    );
    expect((screen.getByTestId('line-chart-tooltip-single') as HTMLInputElement).checked).toBe(
      false,
    );
  });

  it('사용을 끄면 단일 값 칸이 사라진다 — 끈 툴팁의 표시 방식은 뜻이 없다', () => {
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { tooltip: { enabled: false } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('line-chart-tooltip-single')).toBeNull();
  });

  it('기본값으로 되돌아오면 tooltip 필드를 지운다', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { tooltip: { enabled: false } })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('line-chart-tooltip-enabled'));
    expect(onConfigChange).toHaveBeenCalledWith({ tooltip: undefined });
  });

  it('단일 값을 켜면 tooltip.single 로 저장된다', () => {
    const onConfigChange = vi.fn();
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('line-chart-tooltip-single'));
    expect(onConfigChange).toHaveBeenCalledWith({ tooltip: { single: true } });
  });
});

describe('LineChartSection — 라인 스타일 · 소수점', () => {
  it('패널 전역 곡선을 켜면 smooth 로 저장된다', () => {
    const onConfigChange = vi.fn();
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={onConfigChange} />);
    fireEvent.click(screen.getByTestId('line-chart-smooth'));
    expect(onConfigChange).toHaveBeenCalledWith({ smooth: true });
  });

  it('결측 점선은 라인 스타일 섹션에 있고 개수는 켰을 때만 뜬다', () => {
    const onConfigChange = vi.fn();
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={onConfigChange} />);
    expect(screen.queryByTestId('line-chart-gap-dash-threshold')).toBeNull();
    fireEvent.click(screen.getByTestId('line-chart-gap-dash'));
    // 켜면 기본 임계값이 함께 저장된다(끌 때는 undefined 로 지운다).
    expect(onConfigChange).toHaveBeenCalledWith(
      expect.objectContaining({ gap_dash_threshold: expect.any(Number) }),
    );
  });

  it('소수점 이하는 숫자형에서만 노출된다', () => {
    const { unmount } = render(
      <LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={vi.fn()} />,
    );
    expect(screen.getByTestId('line-chart-decimal-places')).toBeInTheDocument();
    unmount();

    render(
      <LineChartSection
        panel={makePanel('graph-chart', { y_axis_type: 'enum' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('line-chart-decimal-places')).toBeNull();
  });

  it('소수점 이하를 비우면 필드를 지운다(자동)', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { decimal_places: 2 })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.change(screen.getByTestId('line-chart-decimal-places'), { target: { value: '' } });
    expect(onConfigChange).toHaveBeenCalledWith({ decimal_places: undefined });
  });
});

describe('BarChartSection (REQ-M4-06 2 modes)', () => {
  it('mode=category 선택 시 label_field 필드 노출', () => {
    const onConfigChange = vi.fn();
    render(
      <BarChartSection
        panel={makePanel('bar-chart', { mode: 'category', label_field: 'labels.name' })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.getByText(/dashboard\.chart\.labelField$/)).toBeInTheDocument();
    expect(screen.queryByText(/dashboard.chart.binSec/)).toBeNull();
  });

  it('mode=time_bin 선택 시 bin_sec 필드 노출', () => {
    const onConfigChange = vi.fn();
    render(
      <BarChartSection
        panel={makePanel('bar-chart', { mode: 'time_bin', bin_sec: 60 })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.getByText(/dashboard.chart.binSec/)).toBeInTheDocument();
  });

  it('agg_func 변경 → onConfigChange', () => {
    const onConfigChange = vi.fn();
    render(
      <BarChartSection
        panel={makePanel('bar-chart', { agg_func: 'avg' })}
        onConfigChange={onConfigChange}
      />,
    );
    const selects = screen.getAllByRole('combobox');
    // 마지막 select 가 agg_func 으로 추정 (mode, agg_func 순)
    const aggSelect = selects.find((s) => (s as HTMLSelectElement).value === 'avg')!;
    fireEvent.change(aggSelect, { target: { value: 'sum' } });
    expect(onConfigChange).toHaveBeenCalledWith({ agg_func: 'sum' });
  });
});

describe('PieChartSection', () => {
  it('show_legend 토글', () => {
    const onConfigChange = vi.fn();
    render(
      <PieChartSection
        panel={makePanel('pie-chart', { show_legend: true, show_percentage: true })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('pie-chart-show-legend'));
    expect(onConfigChange).toHaveBeenCalledWith({ show_legend: false });
  });

  describe('파이 크기·위치', () => {
    it('크기를 바꾸면 pie_size 로 저장한다', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={onConfigChange} />,
      );
      fireEvent.change(screen.getByTestId('pie-chart-size'), { target: { value: '45' } });
      expect(onConfigChange).toHaveBeenCalledWith({ pie_size: 45 });
    });

    it('미지정이면 자동값을 보여 주고 초기화 버튼은 숨긴다', () => {
      const { unmount } = render(
        <PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={vi.fn()} />,
      );
      // 안쪽 라벨 기본 → 80. 슬라이더가 왼쪽 끝에 붙어 있지 않아야 한다.
      expect((screen.getByTestId('pie-chart-size') as HTMLInputElement).value).toBe('80');
      expect(screen.queryByTestId('pie-chart-size-reset')).toBeNull();
      unmount();

      // 바깥 라벨이면 자동값이 62 로 바뀐다 — 패널이 실제로 그리는 값과 같다.
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { label_position: 'outside' })}
          onConfigChange={vi.fn()}
        />,
      );
      expect((screen.getByTestId('pie-chart-size') as HTMLInputElement).value).toBe('62');
    });

    it('자동으로 되돌리면 pie_size 를 지운다 — 슬라이더에는 미지정 자리가 없다', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { pie_size: 45 })}
          onConfigChange={onConfigChange}
        />,
      );
      fireEvent.click(screen.getByTestId('pie-chart-size-reset'));
      expect(onConfigChange).toHaveBeenCalledWith({ pie_size: undefined });
    });

    it('끌어 옮긴 자리가 있을 때만 초기화 버튼이 나온다', () => {
      const onConfigChange = vi.fn();
      const { unmount } = render(
        <PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={vi.fn()} />,
      );
      expect(screen.queryByTestId('pie-chart-reset-offset')).toBeNull();
      unmount();

      render(
        <PieChartSection
          panel={makePanel('pie-chart', { pie_offset_x: 12 })}
          onConfigChange={onConfigChange}
        />,
      );
      fireEvent.click(screen.getByTestId('pie-chart-reset-offset'));
      expect(onConfigChange).toHaveBeenCalledWith({
        pie_offset_x: undefined,
        pie_offset_y: undefined,
      });
    });
  });

  describe('조각 라벨', () => {
    it('값 표시는 기본 꺼짐이며 켜면 show_value 로 저장한다', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={onConfigChange} />,
      );
      const box = screen.getByTestId('pie-chart-show-value') as HTMLInputElement;
      expect(box.checked).toBe(false);
      fireEvent.click(box);
      expect(onConfigChange).toHaveBeenCalledWith({ show_value: true });
    });

    it('글자 크기를 비우면 상속으로 되돌린다', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { label_font_size: 20 })}
          onConfigChange={onConfigChange}
        />,
      );
      fireEvent.change(screen.getByTestId('pie-chart-label-font-size'), {
        target: { value: '' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ label_font_size: undefined });
    });

    it('라벨 위치를 고른다 — 기본은 조각 안쪽', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={onConfigChange} />,
      );
      const select = screen.getByTestId('pie-chart-label-position') as HTMLSelectElement;
      expect(select.value).toBe('inside');
      fireEvent.change(select, { target: { value: 'outside' } });
      expect(onConfigChange).toHaveBeenCalledWith({ label_position: 'outside' });
    });

    it('글꼴·색을 저장하고, 색은 초기화로 상속으로 되돌린다', () => {
      const onConfigChange = vi.fn();
      const { unmount } = render(
        <PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={onConfigChange} />,
      );
      fireEvent.change(screen.getByTestId('pie-chart-label-font-family'), {
        target: { value: 'mono' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ label_font_family: 'mono' });
      fireEvent.change(screen.getByTestId('pie-chart-label-font-color'), {
        target: { value: '#ff0000' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ label_font_color: '#ff0000' });
      // 색을 지정하지 않았으면 초기화 버튼이 없다 — 되돌릴 것이 없다.
      expect(screen.queryByTestId('pie-chart-label-font-color-reset')).toBeNull();
      unmount();

      const onConfigChange2 = vi.fn();
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { label_font_color: '#ff0000' })}
          onConfigChange={onConfigChange2}
        />,
      );
      fireEvent.click(screen.getByTestId('pie-chart-label-font-color-reset'));
      expect(onConfigChange2).toHaveBeenCalledWith({ label_font_color: undefined });
    });

    it('비율·값을 모두 끄면 글자 크기·최소 비중 칸을 내린다', () => {
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { show_percentage: false, show_value: false })}
          onConfigChange={vi.fn()}
        />,
      );
      expect(screen.queryByTestId('pie-chart-label-font-size')).toBeNull();
      expect(screen.queryByTestId('pie-chart-label-min-percent')).toBeNull();
    });

    it('라벨 표시 최소 비중을 저장한다 — 0 은 "모두 표시" 로 저장된다', () => {
      const onConfigChange = vi.fn();
      const { unmount } = render(
        <PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={onConfigChange} />,
      );
      const input = screen.getByTestId('pie-chart-label-min-percent') as HTMLInputElement;
      // 미지정이면 기본값을 placeholder 로 알린다 — 빈 칸이 "0" 으로 읽히지 않도록.
      expect(input.placeholder).toBe('5');
      fireEvent.change(input, { target: { value: '0' } });
      expect(onConfigChange).toHaveBeenCalledWith({ label_min_percent: 0 });
      unmount();
    });

    it('비우면 기본값으로 되돌린다', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { label_min_percent: 10 })}
          onConfigChange={onConfigChange}
        />,
      );
      fireEvent.change(screen.getByTestId('pie-chart-label-min-percent'), {
        target: { value: '' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ label_min_percent: undefined });
    });
  });

  describe('범례', () => {
    it('위치를 고르면 legend_position 으로 저장한다', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { show_legend: true })}
          onConfigChange={onConfigChange}
        />,
      );
      fireEvent.change(screen.getByTestId('pie-chart-legend-position'), {
        target: { value: 'right' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ legend_position: 'right' });
    });

    it('미지정이면 위치 기본 선택은 하단', () => {
      render(<PieChartSection panel={makePanel('pie-chart', {})} onConfigChange={vi.fn()} />);
      expect((screen.getByTestId('pie-chart-legend-position') as HTMLSelectElement).value).toBe(
        'bottom',
      );
    });

    it('비율·값 표시와 글자 크기를 저장한다', () => {
      const onConfigChange = vi.fn();
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { show_legend: true })}
          onConfigChange={onConfigChange}
        />,
      );
      fireEvent.click(screen.getByTestId('pie-chart-legend-show-percentage'));
      expect(onConfigChange).toHaveBeenCalledWith({ legend_show_percentage: true });
      fireEvent.click(screen.getByTestId('pie-chart-legend-show-value'));
      expect(onConfigChange).toHaveBeenCalledWith({ legend_show_value: true });
      fireEvent.change(screen.getByTestId('pie-chart-legend-font-size'), {
        target: { value: '16' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ legend_font_size: 16 });
      fireEvent.change(screen.getByTestId('pie-chart-legend-font-family'), {
        target: { value: 'serif' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ legend_font_family: 'serif' });
      fireEvent.change(screen.getByTestId('pie-chart-legend-font-color'), {
        target: { value: '#00ff00' },
      });
      expect(onConfigChange).toHaveBeenCalledWith({ legend_font_color: '#00ff00' });
    });

    it('범례를 끄면 하위 설정을 모두 내린다 — 적용되지 않는 설정을 남기지 않는다', () => {
      render(
        <PieChartSection
          panel={makePanel('pie-chart', { show_legend: false })}
          onConfigChange={vi.fn()}
        />,
      );
      expect(screen.queryByTestId('pie-chart-legend-position')).toBeNull();
      expect(screen.queryByTestId('pie-chart-legend-show-percentage')).toBeNull();
      expect(screen.queryByTestId('pie-chart-legend-font-size')).toBeNull();
      expect(screen.queryByTestId('pie-chart-legend-font-family')).toBeNull();
    });

    it('끌어 옮긴 자리가 있을 때만 초기화 버튼이 나오고, 누르면 오프셋을 지운다', () => {
      const onConfigChange = vi.fn();
      const { unmount } = render(
        <PieChartSection
          panel={makePanel('pie-chart', { show_legend: true })}
          onConfigChange={vi.fn()}
        />,
      );
      // 끌어 옮기지 않았으면 되돌릴 것이 없다.
      expect(screen.queryByTestId('pie-chart-legend-reset-offset')).toBeNull();
      unmount();

      render(
        <PieChartSection
          panel={makePanel('pie-chart', { show_legend: true, legend_offset_x: 12 })}
          onConfigChange={onConfigChange}
        />,
      );
      fireEvent.click(screen.getByTestId('pie-chart-legend-reset-offset'));
      expect(onConfigChange).toHaveBeenCalledWith({
        legend_offset_x: undefined,
        legend_offset_y: undefined,
      });
    });
  });

  it('표시 필드·라벨 필드·집계 함수·최대 포인트는 설정에 노출하지 않는다', () => {
    // 넷 다 구간 대표값(series_reduce) 경로에서 무시되고 채널 모드에서만 쓰인다.
    // 설정 화면에서 걷어냈으므로 렌더 결과에 남아 있으면 안 된다.
    render(
      <PieChartSection
        panel={makePanel('pie-chart', {
          display_field: 'value',
          label_field: 'labels.name',
          agg_func: 'count',
          max_points: 5,
        })}
        onConfigChange={vi.fn()}
      />,
    );
    for (const key of ['displayField', 'labelField', 'aggFunc', 'maxPoints']) {
      expect(screen.queryByText(`dashboard.chart.${key}`)).toBeNull();
    }
  });
});

describe('TableChartSection (REQ-M4-08)', () => {
  it('columns 추가', () => {
    const onConfigChange = vi.fn();
    const initialColumns = [
      { field: 'timestamp', header: '시간', format: 'datetime' as const },
      { field: 'value', header: '값', format: 'number' as const },
    ];
    render(
      <TableChartSection
        panel={makePanel('table', { columns: initialColumns })}
        onConfigChange={onConfigChange}
      />,
    );
    const addBtn = screen.getByRole('button', { name: /dashboard.chart.add/ });
    fireEvent.click(addBtn);
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [...initialColumns, { field: 'value', header: 'dashboard.chart.newColumn' }],
    });
  });

  it('태그 열 추가 — 소스 설정의 태그를 골라 $.tags.<키> 열을 만든다', () => {
    const onConfigChange = vi.fn();
    const initialColumns = [{ field: 'value', header: '값', format: 'number' as const }];
    render(
      <TableChartSection
        panel={makePanel('table', {
          columns: initialColumns,
          store_source: {
            agent_name: 'a',
            series: [{ key: 'LAI', tags: { room: '1', floor: '3' } }],
          },
        })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.change(screen.getByTestId('table-add-tag-column'), { target: { value: 'room' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [...initialColumns, { field: '$.tags.room', header: 'room', format: 'string' }],
    });
  });

  it('이미 열로 쓰는 태그는 후보에서 빠진다', () => {
    render(
      <TableChartSection
        panel={makePanel('table', {
          columns: [{ field: '$.tags.room', header: 'room' }],
          store_source: {
            agent_name: 'a',
            series: [{ key: 'LAI', tags: { room: '1', floor: '3' } }],
          },
        })}
        onConfigChange={vi.fn()}
      />,
    );
    const select = screen.getByTestId('table-add-tag-column');
    const values = Array.from(select.querySelectorAll('option')).map((o) => o.getAttribute('value'));
    // 안내 항목('') + 남은 후보만 남는다.
    expect(values).toEqual(['', 'floor']);
  });

  it('태그 후보가 없으면 셀렉트 자체를 내린다', () => {
    render(
      <TableChartSection
        panel={makePanel('table', { columns: [{ field: 'value', header: '값' }] })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('table-add-tag-column')).not.toBeInTheDocument();
  });

  it('행 기준을 시각 기준으로 바꾸면 열 편집기 대신 피벗 설정이 나온다', () => {
    const onConfigChange = vi.fn();
    const { rerender } = render(
      <TableChartSection
        panel={makePanel('table', { columns: [{ field: 'value', header: '값' }] })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.queryByTestId('table-pivot-settings')).not.toBeInTheDocument();

    fireEvent.change(screen.getByTestId('table-row-mode'), { target: { value: 'timestamp' } });
    expect(onConfigChange).toHaveBeenCalledWith({ row_mode: 'timestamp' });

    rerender(
      <TableChartSection
        panel={makePanel('table', {
          columns: [{ field: 'value', header: '값' }],
          row_mode: 'timestamp',
        })}
        onConfigChange={onConfigChange}
      />,
    );
    expect(screen.getByTestId('table-pivot-settings')).toBeInTheDocument();
    // 파생 열을 목록으로 띄우지 않는다 — 고쳐도 다음 렌더에 되돌아가기 때문이다.
    expect(screen.queryByTestId('table-column-row-0')).not.toBeInTheDocument();
  });

  it('엔트리별로 되돌리면 기본값이라 config 에 남기지 않는다', () => {
    const onConfigChange = vi.fn();
    render(
      <TableChartSection
        panel={makePanel('table', { columns: [{ field: 'value', header: '값' }], row_mode: 'timestamp' })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.change(screen.getByTestId('table-row-mode'), { target: { value: 'entry' } });
    expect(onConfigChange).toHaveBeenCalledWith({ row_mode: undefined });
  });

  it('시각 기준 행의 시각 열 이름과 공통 단위를 편집한다', () => {
    const onConfigChange = vi.fn();
    render(
      <TableChartSection
        panel={makePanel('table', { columns: [{ field: 'value', header: '값' }], row_mode: 'timestamp' })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.change(screen.getByTestId('table-pivot-time-header'), {
      target: { value: '측정 시각' },
    });
    expect(onConfigChange).toHaveBeenCalledWith({ pivot_time_header: '측정 시각' });

    fireEvent.change(screen.getByTestId('table-pivot-unit'), { target: { value: '°C' } });
    expect(onConfigChange).toHaveBeenCalledWith({ pivot_unit: '°C' });
  });

  it('columns 수정 (field 변경)', () => {
    const onConfigChange = vi.fn();
    render(
      <TableChartSection
        panel={makePanel('table', {
          columns: [{ field: 'timestamp', header: '시간' }],
        })}
        onConfigChange={onConfigChange}
      />,
    );
    const fieldInput = screen.getByDisplayValue('timestamp') as HTMLInputElement;
    fireEvent.change(fieldInput, { target: { value: 'ts' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [{ field: 'ts', header: '시간' }],
    });
  });

  it('단일 컬럼은 삭제 버튼 비활성', () => {
    const onConfigChange = vi.fn();
    render(
      <TableChartSection
        panel={makePanel('table', {
          columns: [{ field: 'value', header: '값' }],
        })}
        onConfigChange={onConfigChange}
      />,
    );
    const delBtn = screen.getByLabelText('dashboard.chart.deleteColumnAria') as HTMLButtonElement;
    expect(delBtn.disabled).toBe(true);
  });

  // ---- 열 폭 비율 / 순서 / 정렬·필터 허용 ----

  const THREE_COLUMNS = [
    { field: 'timestamp', header: '시간', format: 'datetime' as const },
    { field: 'value', header: '값', format: 'number' as const },
    { field: 'name', header: '이름', format: 'string' as const },
  ];

  function renderTable(columns: unknown[] = THREE_COLUMNS) {
    const onConfigChange = vi.fn();
    render(
      <TableChartSection
        panel={makePanel('table', { columns })}
        onConfigChange={onConfigChange}
      />,
    );
    return onConfigChange;
  }

  it('폭 비율을 입력하면 해당 열의 width 만 갱신한다', () => {
    const onConfigChange = renderTable();
    const widthInputs = screen.getAllByLabelText(/dashboard.chart.columnWidth/);
    fireEvent.change(widthInputs[1]!, { target: { value: '3' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [THREE_COLUMNS[0], { ...THREE_COLUMNS[1], width: 3 }, THREE_COLUMNS[2]],
    });
  });

  it('폭을 비우면 width 를 지워 자동으로 되돌린다', () => {
    const cols = [{ field: 'a', header: 'A', width: 5 }, { field: 'b', header: 'B' }];
    const onConfigChange = renderTable(cols);
    const widthInputs = screen.getAllByLabelText(/dashboard.chart.columnWidth/);
    fireEvent.change(widthInputs[0]!, { target: { value: '' } });
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [{ field: 'a', header: 'A', width: undefined }, { field: 'b', header: 'B' }],
    });
  });

  it('0 이하 폭은 무시한다 (열이 사라지지 않게)', () => {
    const onConfigChange = renderTable();
    const widthInputs = screen.getAllByLabelText(/dashboard.chart.columnWidth/);
    fireEvent.change(widthInputs[0]!, { target: { value: '0' } });
    expect(onConfigChange).not.toHaveBeenCalled();
  });

  it('아래로 이동 버튼이 인접 열과 자리를 바꾼다', () => {
    const onConfigChange = renderTable();
    const downs = screen.getAllByLabelText('dashboard.chart.moveColumnDownAria');
    fireEvent.click(downs[0]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [THREE_COLUMNS[1], THREE_COLUMNS[0], THREE_COLUMNS[2]],
    });
  });

  it('위로 이동 버튼이 인접 열과 자리를 바꾼다', () => {
    const onConfigChange = renderTable();
    const ups = screen.getAllByLabelText('dashboard.chart.moveColumnUpAria');
    fireEvent.click(ups[2]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [THREE_COLUMNS[0], THREE_COLUMNS[2], THREE_COLUMNS[1]],
    });
  });

  it('첫 열의 위로 · 마지막 열의 아래로 버튼은 비활성이다', () => {
    renderTable();
    const ups = screen.getAllByLabelText('dashboard.chart.moveColumnUpAria') as HTMLButtonElement[];
    const downs = screen.getAllByLabelText('dashboard.chart.moveColumnDownAria') as HTMLButtonElement[];
    expect(ups[0]!.disabled).toBe(true);
    expect(ups[2]!.disabled).toBe(false);
    expect(downs[2]!.disabled).toBe(true);
    expect(downs[0]!.disabled).toBe(false);
  });

  it('정렬 허용 체크박스는 기본 켜짐이고 끄면 sortable:false 를 쓴다', () => {
    const onConfigChange = renderTable();
    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[];
    // 열마다 [정렬, 필터] 두 개 — 첫 열의 정렬은 index 0.
    expect(boxes[0]!.checked).toBe(true);
    fireEvent.click(boxes[0]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [{ ...THREE_COLUMNS[0], sortable: false }, THREE_COLUMNS[1], THREE_COLUMNS[2]],
    });
  });

  it('필터 허용 체크박스는 기본 꺼짐이고 켜면 filterable:true 를 쓴다', () => {
    const onConfigChange = renderTable();
    const boxes = screen.getAllByRole('checkbox') as HTMLInputElement[];
    expect(boxes[1]!.checked).toBe(false);
    fireEvent.click(boxes[1]!);
    expect(onConfigChange).toHaveBeenCalledWith({
      columns: [{ ...THREE_COLUMNS[0], filterable: true }, THREE_COLUMNS[1], THREE_COLUMNS[2]],
    });
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M3.5 / M3.6 — 구간 대표값 선택기.
//
// 노출 조건은 두 개의 AND 다: Store 모드(§2.10 [S2]) × REDUCE_PANEL_TYPES(§2.3 [U3]).
// 특성화 CH-20 이 반대 방향(line-chart/table/heatmap 미노출)을 이미 잠그고 있으므로,
// 여기서는 노출되는 쪽과 선택지 구성/편집 결과를 잠근다.
// ---------------------------------------------------------------------------

const REDUCE_TESTID = 'chart-series-reduce';

/** Store 모드 config — 선택기가 노출될 수 있는 유일한 조건. */
function storeModeConfig(extra: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    data_source: 'store',
    store_source: {
      agent_name: 'store-1',
      namespace: 'default',
      selection_mode: 'keys',
      series: [{ key: 'k1' }],
      time_window_ms: 3_600_000,
      interval_ms: 60_000,
      aggregation: 'average',
    },
    ...extra,
  };
}

describe('대표값 선택기 (SPEC-CHART-002 U3)', () => {
  it('stat/gauge/bar-chart/pie-chart + Store 모드에서 노출된다', () => {
    for (const type of ['stat', 'gauge', 'bar-chart', 'pie-chart'] as const) {
      const view = render(
        <StoreSourceSection panel={makePanel(type, storeModeConfig())} onConfigChange={vi.fn()} />,
      );
      expect(screen.getByTestId(REDUCE_TESTID)).toBeInTheDocument();
      view.unmount();
    }
  });

  it('line-chart/table/heatmap 에서는 노출되지 않는다', () => {
    for (const type of ['graph-chart', 'table', 'heatmap'] as const) {
      const view = render(
        <StoreSourceSection panel={makePanel(type, storeModeConfig())} onConfigChange={vi.fn()} />,
      );
      expect(screen.queryByTestId(REDUCE_TESTID)).toBeNull();
      view.unmount();
    }
  });

  it('선택지는 max/avg/min/last/sum/count/delta 7개이며 first 는 없다', () => {
    render(
      <StoreSourceSection panel={makePanel('stat', storeModeConfig())} onConfigChange={vi.fn()} />,
    );
    const select = screen.getByTestId(REDUCE_TESTID) as HTMLSelectElement;
    const values = Array.from(select.options).map((o) => o.value);
    // 첫 항목은 "미지정"(빈 값) — 부재가 곧 레거시 경로이기 때문이다(§2.9).
    expect(values).toEqual(['', 'max', 'avg', 'min', 'last', 'sum', 'count', 'delta']);
    expect(values).not.toContain('first');
    expect(select.value).toBe('');
  });

  it('선택하면 series_reduce 를 기록하고 미지정으로 되돌리면 undefined 로 지운다', () => {
    const onConfigChange = vi.fn();
    const view = render(
      <StoreSourceSection panel={makePanel('stat', storeModeConfig())} onConfigChange={onConfigChange} />,
    );
    fireEvent.change(screen.getByTestId(REDUCE_TESTID), { target: { value: 'avg' } });
    expect(onConfigChange).toHaveBeenCalledWith({ series_reduce: 'avg' });
    view.unmount();

    const onConfigChange2 = vi.fn();
    render(
      <StoreSourceSection
        panel={makePanel('stat', storeModeConfig({ series_reduce: 'avg' }))}
        onConfigChange={onConfigChange2}
      />,
    );
    fireEvent.change(screen.getByTestId(REDUCE_TESTID), { target: { value: '' } });
    expect(onConfigChange2).toHaveBeenCalledWith({ series_reduce: undefined });
  });

  it('pie-chart + 음수 가능 대표값(delta/min/sum) 조합에 경고를 표시한다', () => {
    for (const fn of ['delta', 'min', 'sum'] as const) {
      const view = render(
        <StoreSourceSection
          panel={makePanel('pie-chart', storeModeConfig({ series_reduce: fn }))}
          onConfigChange={vi.fn()}
        />,
      );
      expect(screen.getByTestId('chart-series-reduce-pie-warning')).toBeInTheDocument();
      view.unmount();
    }
  });

  it('pie-chart 라도 음수가 나지 않는 대표값에는 경고가 없고, 같은 조합이라도 bar 는 경고하지 않는다', () => {
    const view = render(
      <StoreSourceSection
        panel={makePanel('pie-chart', storeModeConfig({ series_reduce: 'max' }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('chart-series-reduce-pie-warning')).toBeNull();
    view.unmount();

    render(
      <StoreSourceSection
        panel={makePanel('bar-chart', storeModeConfig({ series_reduce: 'delta' }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('chart-series-reduce-pie-warning')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-002 M6.1 / M6.2 — 대표값 선택기의 안내 문구 두 종.
//
// 6.1 조합 설명: `store_source.aggregation`(버킷 집계) × `series_reduce`(윈도우 대표값)는
//     서로 다른 축인데(§2.1 [U1]) 두 셀렉트가 같은 화면에 있어 가장 혼동하기 쉽다.
// 6.2 불리언 안내: 계산에는 분기가 없지만(1/0 정규화) 읽는 의미가 전혀 다르다(§2.2 표).
// ---------------------------------------------------------------------------

const COMBO_TESTID = 'chart-series-reduce-combo';
const BOOLEAN_TESTID = 'chart-series-reduce-boolean-hint';

/** Store 모드 config — store_source 를 부분 재정의할 수 있게 한 변형. */
function storeModeConfigWith(
  store: Record<string, unknown>,
  extra: Record<string, unknown> = {},
): Record<string, unknown> {
  const base = storeModeConfig() as {
    store_source: Record<string, unknown>;
  } & Record<string, unknown>;
  return { ...base, store_source: { ...base.store_source, ...store }, ...extra };
}

describe('대표값 조합 설명 (SPEC-CHART-002 M6.1)', () => {
  it('대표값 미지정이면 조합 설명을 표시하지 않는다', () => {
    render(
      <StoreSourceSection panel={makePanel('stat', storeModeConfig())} onConfigChange={vi.fn()} />,
    );
    expect(screen.queryByTestId(COMBO_TESTID)).toBeNull();
  });

  it('대표값을 고르면 집계 · 버킷 간격 · 대표값을 한 줄로 설명한다', () => {
    render(
      <StoreSourceSection
        panel={makePanel('stat', storeModeConfig({ series_reduce: 'max' }))}
        onConfigChange={vi.fn()}
      />,
    );
    const line = screen.getByTestId(COMBO_TESTID);
    // aggregation:'average' + interval_ms:60_000 + series_reduce:'max'
    expect(line).toHaveTextContent('60초');
    expect(line).toHaveTextContent('tsdb.aggAverage');
    expect(line).toHaveTextContent('dashboard.chart.seriesReduceMax');
    // 보간 슬롯이 남아 있으면 치환에 실패한 것이다.
    expect(line.textContent).not.toContain('{');
  });

  it('집계나 대표값을 바꾸면 설명도 함께 바뀐다(두 축이 각각 반영된다)', () => {
    const view = render(
      <StoreSourceSection
        panel={makePanel(
          'stat',
          storeModeConfigWith({ aggregation: 'max', interval_ms: 5_000 }, { series_reduce: 'avg' }),
        )}
        onConfigChange={vi.fn()}
      />,
    );
    const line = screen.getByTestId(COMBO_TESTID);
    expect(line).toHaveTextContent('5초');
    expect(line).toHaveTextContent('tsdb.aggMax');
    expect(line).toHaveTextContent('dashboard.chart.seriesReduceAvg');
    view.unmount();
  });

  it('대표값 대상 4종 패널 모두에서 설명이 나온다', () => {
    for (const type of ['stat', 'gauge', 'bar-chart', 'pie-chart'] as const) {
      const view = render(
        <StoreSourceSection
          panel={makePanel(type, storeModeConfig({ series_reduce: 'last' }))}
          onConfigChange={vi.fn()}
        />,
      );
      expect(screen.getByTestId(COMBO_TESTID)).toBeInTheDocument();
      view.unmount();
    }
  });
});

describe('불리언 시리즈 안내 (SPEC-CHART-002 M6.2)', () => {
  it('불리언 시리즈가 없으면 안내하지 않는다', () => {
    render(
      <StoreSourceSection
        panel={makePanel('stat', storeModeConfig({ series_reduce: 'avg' }))}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId(BOOLEAN_TESTID)).toBeNull();
  });

  it('불리언 시리즈가 있어도 대표값 미지정이면 안내하지 않는다', () => {
    render(
      <StoreSourceSection
        panel={makePanel(
          'stat',
          storeModeConfigWith({ series: [{ key: 'b1', data_type: 'boolean' }] }),
        )}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId(BOOLEAN_TESTID)).toBeNull();
  });

  it('불리언 시리즈 + 대표값 선택 시 해당 대표값의 불리언 의미를 안내한다', () => {
    const view = render(
      <StoreSourceSection
        panel={makePanel(
          'stat',
          storeModeConfigWith(
            { series: [{ key: 'b1', data_type: 'boolean' }] },
            { series_reduce: 'avg' },
          ),
        )}
        onConfigChange={vi.fn()}
      />,
    );
    const hint = screen.getByTestId(BOOLEAN_TESTID);
    expect(hint).toHaveTextContent('dashboard.chart.seriesReduceBoolAvg');
    expect(hint.textContent).not.toContain('{');
    view.unmount();
  });

  it('7종 대표값 각각에 서로 다른 불리언 의미 문구가 대응된다', () => {
    const seen = new Set<string>();
    for (const fn of ['max', 'avg', 'min', 'last', 'sum', 'count', 'delta'] as const) {
      const view = render(
        <StoreSourceSection
          panel={makePanel(
            'stat',
            storeModeConfigWith(
              { series: [{ key: 'b1', data_type: 'boolean' }] },
              { series_reduce: fn },
            ),
          )}
          onConfigChange={vi.fn()}
        />,
      );
      const text = screen.getByTestId(BOOLEAN_TESTID).textContent ?? '';
      expect(text).not.toBe('');
      seen.add(text);
      view.unmount();
    }
    expect(seen.size).toBe(7);
  });

  it('수치 시리즈와 섞여 있어도 불리언이 하나라도 있으면 안내한다', () => {
    render(
      <StoreSourceSection
        panel={makePanel(
          'stat',
          storeModeConfigWith(
            {
              series: [
                { key: 'n1', data_type: 'float' },
                { key: 'b1', data_type: 'boolean' },
              ],
            },
            { series_reduce: 'sum' },
          ),
        )}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId(BOOLEAN_TESTID)).toHaveTextContent(
      'dashboard.chart.seriesReduceBoolSum',
    );
  });

  it('tag 모드는 설정 시점에 data_type 을 알 수 없으므로 안내하지 않는다', () => {
    // 추측해서 틀린 안내를 하는 것보다 침묵이 낫다 — 키는 폴링 시점에 해석된다.
    render(
      <StoreSourceSection
        panel={makePanel(
          'stat',
          storeModeConfigWith(
            { selection_mode: 'tag', tag_filters: { room: '1' }, series: [] },
            { series_reduce: 'max' },
          ),
        )}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId(BOOLEAN_TESTID)).toBeNull();
    // 조합 설명은 tag 모드에서도 유효하다(두 축은 선택 방식과 무관하다).
    expect(screen.getByTestId(COMBO_TESTID)).toBeInTheDocument();
  });
});

// ===== 결측 구간 점선 표기 (SPEC-TSDB-004 §2.19) =====

describe('LineChartSection — 결측 구간 점선', () => {
  it('기본은 꺼짐이며 임계 입력이 숨어 있다', () => {
    render(
      <LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={() => {}} />,
    );
    const toggle = screen.getByTestId('line-chart-gap-dash') as HTMLInputElement;
    expect(toggle.checked).toBe(false);
    expect(screen.queryByTestId('line-chart-gap-dash-threshold')).toBeNull();
  });

  it('켜면 기본 임계로 저장한다', () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <LineChartSection
        panel={makePanel('graph-chart', {})}
        onConfigChange={(p) => patches.push(p)}
      />,
    );
    fireEvent.click(screen.getByTestId('line-chart-gap-dash'));
    expect(patches.at(-1)?.gap_dash_threshold).toBe(2);
  });

  it('끄면 값을 지운다 — 0 을 남기면 "켜져 있는데 임계 0" 처럼 읽힌다', () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { gap_dash_threshold: 3 })}
        onConfigChange={(p) => patches.push(p)}
      />,
    );
    fireEvent.click(screen.getByTestId('line-chart-gap-dash'));
    expect(patches.at(-1)?.gap_dash_threshold).toBeUndefined();
  });

  it('켜져 있으면 임계를 고칠 수 있다', () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { gap_dash_threshold: 2 })}
        onConfigChange={(p) => patches.push(p)}
      />,
    );
    const input = screen.getByTestId('line-chart-gap-dash-threshold') as HTMLInputElement;
    expect(input.value).toBe('2');
    fireEvent.change(input, { target: { value: '5' } });
    expect(patches.at(-1)?.gap_dash_threshold).toBe(5);
  });

  it('1 미만은 반영하지 않는다', () => {
    const patches: Array<Record<string, unknown>> = [];
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { gap_dash_threshold: 2 })}
        onConfigChange={(p) => patches.push(p)}
      />,
    );
    const before = patches.length;
    fireEvent.change(screen.getByTestId('line-chart-gap-dash-threshold'), {
      target: { value: '0' },
    });
    // 0 은 "끄기" 와 같은 뜻인데 토글은 켜져 있다 — 상태가 모순된다.
    expect(patches.length).toBe(before);
  });
});

describe('LineChartSection — 그래프 스타일', () => {
  it('기본은 라인 — 저장된 패널의 동작', () => {
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={vi.fn()} />);
    expect((screen.getByTestId('line-chart-graph-style') as HTMLSelectElement).value).toBe('line');
  });

  it('스타일을 고르면 graph_style 로 저장된다', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={onConfigChange} />,
    );
    fireEvent.change(screen.getByTestId('line-chart-graph-style'), { target: { value: 'bar' } });
    expect(onConfigChange).toHaveBeenCalledWith({ graph_style: 'bar' });
  });

  it('라인에서는 스택킹을 묻지 않는다 — 쌓아도 누적으로 읽히지 않는다', () => {
    render(<LineChartSection panel={makePanel('graph-chart', {})} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('line-chart-stacked')).toBeNull();
  });

  it('영역·바에서만 스택킹이 나타난다', () => {
    for (const style of ['area', 'bar']) {
      const { unmount } = render(
        <LineChartSection
          panel={makePanel('graph-chart', { graph_style: style })}
          onConfigChange={vi.fn()}
        />,
      );
      expect(screen.getByTestId('line-chart-stacked')).toBeInTheDocument();
      unmount();
    }
  });

  it('캔들도 스택킹을 묻지 않는다 — 네 값이 한 덩어리다', () => {
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { graph_style: 'candle' })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('line-chart-stacked')).toBeNull();
  });

  it('스택킹을 끄면 필드를 지운다', () => {
    const onConfigChange = vi.fn();
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { graph_style: 'bar', stacked: true })}
        onConfigChange={onConfigChange}
      />,
    );
    fireEvent.click(screen.getByTestId('line-chart-stacked'));
    expect(onConfigChange).toHaveBeenCalledWith({ stacked: undefined });
  });

  it.each(['line', 'area'])('%s 에서는 곡선·결측 점선을 묻는다', (style) => {
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { graph_style: style })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('line-chart-smooth')).toBeInTheDocument();
    expect(screen.getByTestId('line-chart-gap-dash')).toBeInTheDocument();
  });

  it.each(['bar', 'candle'])('%s 에서는 곡선·결측 점선을 묻지 않는다', (style) => {
    // 구부릴 선이 없고, 막대가 서지 않아 결측이 이미 눈에 보인다 — 골라도 그림이
    // 바뀌지 않는 설정을 내면 "켰는데 왜 안 되지" 를 찾아 헤매게 된다.
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { graph_style: style })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('line-chart-smooth')).toBeNull();
    expect(screen.queryByTestId('line-chart-gap-dash')).toBeNull();
  });

  it('바에서는 저장된 결측 임계값이 있어도 입력·힌트를 내지 않는다', () => {
    // 스타일을 바꾸며 남은 값이다. 쓰이지 않는 설정을 계속 보여 주면 그림에
    // 반영되고 있다고 읽힌다.
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { graph_style: 'bar', gap_dash_threshold: 3 })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('line-chart-gap-dash-threshold')).toBeNull();
    expect(screen.queryByTestId('line-chart-gap-dash-hint')).toBeNull();
  });

  it('캔들에서는 스타일 옵션이 하나도 뜨지 않는다', () => {
    render(
      <LineChartSection
        panel={makePanel('graph-chart', { graph_style: 'candle' })}
        onConfigChange={vi.fn()}
      />,
    );
    for (const id of ['line-chart-stacked', 'line-chart-smooth', 'line-chart-gap-dash']) {
      expect(screen.queryByTestId(id)).toBeNull();
    }
  });
});

describe('LineChartSection — 캔들 게이팅', () => {
  it('시리즈 소스(TSDB)에서는 캔들을 고를 수 있다', () => {
    render(
      <LineChartSection
        panel={makePanel('graph-chart', {
          data_source: 'tsdb',
          tsdb_source: {
            backend: 'influxdb',
            agent_name: 'ix',
            series: [{ key: 'cpu', field: 'usage' }],
            time_window_ms: 3_600_000,
            interval_ms: 60_000,
            aggregation: 'average',
          },
        })}
        onConfigChange={vi.fn()}
      />,
    );
    const select = screen.getByTestId('line-chart-graph-style') as HTMLSelectElement;
    expect([...select.options].find((o) => o.value === 'candle')!.disabled).toBe(false);
    expect(screen.queryByTestId('line-chart-candle-unsupported')).toBeNull();
  });
});

// ---------------------------------------------------------------------------
// SPEC-CHART-003 M6 — stat 보조 표기 설정 UI.
//
// @spec SPEC-CHART-003 AC-18 / AC-23 / AC-25 (U5-1 · U5-2)
// ---------------------------------------------------------------------------
describe('StatChartSection 보조 표기 설정 (SPEC-CHART-003)', () => {
  const statPanel = (config: Record<string, unknown> = {}) =>
    makePanel('stat', { channel_name: 'c', ...config });

  it('채널 모드에서 변화량 체크박스는 켜진 상태로 시작한다(레거시 경로 기본값)', () => {
    render(<StatChartSection panel={statPanel()} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('stat-delta-enabled')).toBeChecked();
  });

  it('체크박스를 끄면 delta_display.enabled=false 가 명시적으로 저장된다', () => {
    const onChange = vi.fn();
    render(<StatChartSection panel={statPanel()} onConfigChange={onChange} />);
    fireEvent.click(screen.getByTestId('stat-delta-enabled'));
    expect(onChange).toHaveBeenCalledWith({ delta_display: { enabled: false } });
  });

  it('U5-2: 변화량이 꺼져 있으면 색 컨트롤을 내지 않는다', () => {
    render(
      <StatChartSection
        panel={statPanel({ delta_display: { enabled: false } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('stat-delta-colors')).toBeNull();
    expect(screen.queryByTestId('stat-delta-up-color')).toBeNull();
  });

  it('변화량이 켜져 있으면 증가·감소·변화없음 색을 각각 편집할 수 있다', () => {
    const onChange = vi.fn();
    render(<StatChartSection panel={statPanel()} onConfigChange={onChange} />);
    expect(screen.getByTestId('stat-delta-colors')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('stat-delta-up-color'), {
      target: { value: '#123456' },
    });
    expect(onChange).toHaveBeenCalledWith({ delta_display: { up_color: '#123456' } });
  });

  it('색 입력의 기본값은 현행 하드코딩 색이다(빈 값이 아니다)', () => {
    render(<StatChartSection panel={statPanel()} onConfigChange={vi.fn()} />);
    expect(screen.getByTestId('stat-delta-up-color')).toHaveValue('#10b981');
    expect(screen.getByTestId('stat-delta-down-color')).toHaveValue('#f43f5e');
  });

  it('구간 통계 체크박스 3개가 config 를 토글한다', () => {
    const onChange = vi.fn();
    render(<StatChartSection panel={statPanel()} onConfigChange={onChange} />);

    for (const kind of ['avg', 'max', 'min'] as const) {
      expect(screen.getByTestId(`stat-window-${kind}`)).not.toBeChecked();
    }

    fireEvent.click(screen.getByTestId('stat-window-max'));
    expect(onChange).toHaveBeenCalledWith({ window_stats: { max: true } });
  });

  it('이미 켠 항목을 다시 누르면 꺼진다(다른 항목은 유지)', () => {
    const onChange = vi.fn();
    render(
      <StatChartSection
        panel={statPanel({ window_stats: { avg: true, max: true } })}
        onConfigChange={onChange}
      />,
    );
    expect(screen.getByTestId('stat-window-avg')).toBeChecked();

    fireEvent.click(screen.getByTestId('stat-window-avg'));
    expect(onChange).toHaveBeenCalledWith({ window_stats: { avg: false, max: true } });
  });

  it('AC-23: 채널 모드에서 구간 통계를 켜면 max_points 안내가 뜬다', () => {
    render(
      <StatChartSection
        panel={statPanel({ window_stats: { avg: true } })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('stat-window-max-points-hint')).toBeInTheDocument();
  });

  it('AC-23: 구간 통계가 꺼져 있으면 안내를 내지 않는다', () => {
    render(<StatChartSection panel={statPanel()} onConfigChange={vi.fn()} />);
    expect(screen.queryByTestId('stat-window-max-points-hint')).toBeNull();
  });

  it('AC-23: 시리즈 소스 패널에서는 안내를 내지 않는다(조회 윈도우가 구간을 정한다)', () => {
    render(
      <StatChartSection
        panel={statPanel({
          window_stats: { avg: true },
          data_source: 'store',
          store_source: {
            agent_name: 'store-1',
            namespace: 'default',
            selection_mode: 'keys',
            series: [{ key: 'k1' }],
            time_window_ms: 60_000,
            interval_ms: 1_000,
            aggregation: 'average',
          },
        })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.queryByTestId('stat-window-max-points-hint')).toBeNull();
  });

  it('보조 줄 크기 배율을 편집하고 초기화할 수 있다', () => {
    const onChange = vi.fn();
    render(
      <StatChartSection panel={statPanel({ sub_value_scale: 2 })} onConfigChange={onChange} />,
    );
    expect(screen.getByTestId('stat-sub-value-scale')).toHaveValue('2');

    fireEvent.click(screen.getByTestId('stat-sub-value-scale-reset'));
    expect(onChange).toHaveBeenCalledWith({ sub_value_scale: undefined });
  });

  it('보조 줄 배율은 본값 배율(value_scale)과 별개 슬라이더다', () => {
    render(
      <StatChartSection
        panel={statPanel({ value_scale: 3, sub_value_scale: 1.5 })}
        onConfigChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId('stat-value-scale')).toHaveValue('3');
    expect(screen.getByTestId('stat-sub-value-scale')).toHaveValue('1.5');
  });
});
