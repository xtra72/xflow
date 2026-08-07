// HeatmapLegend 렌더 테스트 (색표 범례, additive).
//
// 그라디언트 막대(히트맵과 동일 color_table), 등간 눈금 라벨(값/개수/방향별 배치),
// 위치(4모서리)/크기(S/M/L), 퇴화(min==max) 방어, colorTable 폴백, 접근성(role/aria)을
// 컴포넌트 레벨에서 커버한다.

import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, cleanup, within } from '@testing-library/react';

// i18n 은 키를 그대로 반환하도록 모킹한다(I18nProvider 없이 렌더 가능).
vi.mock('@/lib/i18n', () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

import HeatmapLegend from './HeatmapLegend';
import type { ColorStop, LegendConfig } from './heatmapConfig';

afterEach(() => cleanup());

/** 기본 범례 config(테스트 오버라이드). */
function legendCfg(overrides: Partial<LegendConfig> = {}): LegendConfig {
  return {
    enabled: true,
    orientation: 'vertical',
    position: 'bottom-right',
    size: 'md',
    tick_count: 5,
    ...overrides,
  };
}

const TABLE: ColorStop[] = [
  { stop: 0, color: '#0000ff' },
  { stop: 1, color: '#ff0000' },
];

describe('HeatmapLegend — 구조/접근성', () => {
  it('role=img + aria-label 을 가진 범례 컨테이너를 렌더한다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 18, max: 26 }} colorTable={TABLE} legend={legendCfg()} />,
    );
    const el = getByTestId('heatmap-legend');
    expect(el.getAttribute('role')).toBe('img');
    expect(el.getAttribute('aria-label')).toBeTruthy();
    expect(el.className).toContain('pointer-events-none');
  });

  it('tick_count 개수만큼 눈금 라벨을 렌더하고 양 끝은 min/max 값이다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 18, max: 26 }} colorTable={TABLE} legend={legendCfg({ tick_count: 5 })} />,
    );
    const legend = getByTestId('heatmap-legend');
    const ticks = within(legend).getAllByTestId(/heatmap-legend-tick-/);
    expect(ticks.length).toBe(5);
    // i=0(min)=18.0, i=last(max)=26.0. 소수 1자리 포맷.
    expect(ticks[0]!.textContent).toBe('18.0');
    expect(ticks[4]!.textContent).toBe('26.0');
    // 중앙(i=2) = 22.0.
    expect(ticks[2]!.textContent).toBe('22.0');
  });
});

describe('HeatmapLegend — 방향/그라디언트', () => {
  it('세로는 flex-row 배치 + 그라디언트 to top', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 10 }} colorTable={TABLE} legend={legendCfg({ orientation: 'vertical' })} />,
    );
    const legend = getByTestId('heatmap-legend');
    expect(legend.className).toContain('flex-row');
    // 막대(첫 자식 div)의 그라디언트 방향.
    const bar = legend.querySelector('div')!;
    expect(bar.style.background).toContain('to top');
  });

  it('가로는 flex-col 배치 + 그라디언트 to right', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 10 }} colorTable={TABLE} legend={legendCfg({ orientation: 'horizontal' })} />,
    );
    const legend = getByTestId('heatmap-legend');
    expect(legend.className).toContain('flex-col');
    const bar = legend.querySelector('div')!;
    expect(bar.style.background).toContain('to right');
  });
});

describe('HeatmapLegend — 위치/크기', () => {
  it('position 에 따라 모서리 배치 클래스를 적용한다', () => {
    const { getByTestId, rerender } = render(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ position: 'top-left' })} />,
    );
    expect(getByTestId('heatmap-legend').className).toContain('top-2');
    expect(getByTestId('heatmap-legend').className).toContain('left-2');

    rerender(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ position: 'bottom-right' })} />,
    );
    expect(getByTestId('heatmap-legend').className).toContain('bottom-2');
    expect(getByTestId('heatmap-legend').className).toContain('right-2');
  });

  it('크기 프리셋에 따라 막대 길이가 달라진다(sm < lg)', () => {
    const { getByTestId, rerender } = render(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ size: 'sm' })} />,
    );
    const smBar = getByTestId('heatmap-legend').querySelector('div')!;
    const smHeight = smBar.style.height;

    rerender(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={TABLE} legend={legendCfg({ size: 'lg' })} />,
    );
    const lgBar = getByTestId('heatmap-legend').querySelector('div')!;
    expect(parseInt(lgBar.style.height, 10)).toBeGreaterThan(parseInt(smHeight, 10));
  });
});

describe('HeatmapLegend — 견고성', () => {
  it('min==max(퇴화) 이면 단일 라벨만 렌더한다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 21, max: 21 }} colorTable={TABLE} legend={legendCfg({ tick_count: 5 })} />,
    );
    const ticks = within(getByTestId('heatmap-legend')).getAllByTestId(/heatmap-legend-tick-/);
    expect(ticks.length).toBe(1);
    expect(ticks[0]!.textContent).toBe('21.0');
  });

  it('colorTable 이 비면 DEFAULT_COLOR_TABLE 로 폴백해 그라디언트를 렌더한다', () => {
    const { getByTestId } = render(
      <HeatmapLegend bounds={{ min: 0, max: 1 }} colorTable={[]} legend={legendCfg()} />,
    );
    const bar = getByTestId('heatmap-legend').querySelector('div')!;
    // 기본 gradient 의 첫 색(#2166ac)이 포함된다.
    expect(bar.style.background).toContain('#2166ac');
  });
});
